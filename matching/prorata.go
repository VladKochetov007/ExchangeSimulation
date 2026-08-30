package matching

import (
	ebook "exchange_sim/book"
	eclock "exchange_sim/clock"
	etypes "exchange_sim/types"
)

// ProRataMatcher distributes fills proportionally to resting order size at each level.
// Used by CME Globex and Euronext for liquid futures contracts.
// Time priority is used only as a tiebreaker when quantities are equal.
type ProRataMatcher struct {
	clock etypes.Clock
}

func NewProRataMatcher(clock etypes.Clock) *ProRataMatcher {
	if clock == nil {
		clock = &eclock.RealClock{}
	}
	return &ProRataMatcher{clock: clock}
}

func (m *ProRataMatcher) Match(bidBook, askBook *ebook.Book, incomingOrder *etypes.Order) *MatchResult {
	executions := make([]*etypes.Execution, 0, 8)
	var book *ebook.Book
	if incomingOrder.Side == etypes.Buy {
		book = askBook
	} else {
		book = bidBook
	}

	limit := book.Best
	for incomingOrder.FilledQty < incomingOrder.Qty && limit != nil {
		if !m.CanMatch(incomingOrder, limit) {
			break
		}
		nextLimit := limit.Next
		// Rescan after iceberg refreshes: fresh tranches are new liquidity
		// behind the queue. Terminates — each refresh pass consumed qty.
		blocked := false
		for {
			remaining := incomingOrder.Qty - incomingOrder.FilledQty
			if remaining == 0 {
				break
			}

			// Collect eligible resting orders and displayed qty at this level.
			// Icebergs participate with their display tranche only.
			type candidate struct {
				order     *etypes.Order
				available int64
			}
			var candidates []candidate
			totalQty := int64(0)
			for o := limit.Head; o != nil; o = o.Next {
				if o.FilledQty < o.Qty && o.ClientID != incomingOrder.ClientID {
					available := makerAvailable(o)
					if available <= 0 {
						continue
					}
					var ok bool
					totalQty, ok = etypes.TryAdd(totalQty, available)
					if !ok {
						blocked = true
						break
					}
					candidates = append(candidates, candidate{order: o, available: available})
				}
			}
			if blocked {
				// An unrepresentable level cannot receive a correct pro-rata
				// allocation. Do not mutate it or bypass its price priority.
				break
			}
			if totalQty == 0 {
				// Level holds only the incoming client's own orders; skip past it
				// instead of blockading deeper liquidity.
				break
			}

			// Distribute fills proportionally. Each maker gets floor(remaining * share),
			// capped at available so remaining > totalQty never over-fills a maker.
			// Leftovers are assigned in resting order (FIFO tiebreaker) until exhausted.
			filled := int64(0)
			shares := make([]int64, len(candidates))
			for i, c := range candidates {
				shares[i] = min(etypes.MulDiv(remaining, c.available, totalQty), c.available)
				filled += shares[i]
			}
			leftover := min(remaining-filled, totalQty)
			for i := range candidates {
				if leftover == 0 {
					break
				}
				extra := min(candidates[i].available-shares[i], leftover)
				shares[i] += extra
				leftover -= extra
			}

			// Emit one execution per maker with a non-zero share.
			now := m.clock.NowUnixNano()
			refreshed := false
			for i, c := range candidates {
				if shares[i] == 0 {
					continue
				}
				execQty := shares[i]
				incomingOrder.FilledQty += execQty
				c.order.FilledQty += execQty
				if c.order.Visibility == etypes.Iceberg && c.order.DisplayRemaining > 0 {
					c.order.DisplayRemaining -= execQty
				}
				if c.order.Parent != nil {
					c.order.Parent.TotalQty -= execQty
				}

				exec := getExecution()
				exec.TakerOrderID = incomingOrder.ID
				exec.MakerOrderID = c.order.ID
				exec.TakerClientID = incomingOrder.ClientID
				exec.MakerClientID = c.order.ClientID
				exec.Price = limit.Price
				exec.Qty = execQty
				exec.Timestamp = now
				exec.TakerFilledQty = incomingOrder.FilledQty
				exec.MakerFilledQty = c.order.FilledQty
				exec.MakerTotalQty = c.order.Qty
				exec.MakerSide = c.order.Side
				exec.MakerPosSide = c.order.PositionSide
				executions = append(executions, exec)

				if c.order.FilledQty >= c.order.Qty {
					c.order.Status = etypes.Filled
					// Keep the book.Orders index entry for settlement; the exchange
					// removes it in removeMakerOrders.
					ebook.UnlinkOrder(c.order)
				} else {
					c.order.Status = etypes.PartialFill
					if c.order.Visibility == etypes.Iceberg && c.order.DisplayRemaining == 0 {
						refreshIcebergTranche(limit, c.order)
						refreshed = true
					}
				}
			}
			if !refreshed {
				break
			}
		}
		if blocked {
			break
		}

		if ebook.IsEmpty(limit) {
			book.RemoveLimit(limit)
		}
		limit = nextLimit
	}

	fullyFilled := incomingOrder.FilledQty >= incomingOrder.Qty
	if fullyFilled {
		incomingOrder.Status = etypes.Filled
	} else if incomingOrder.FilledQty > 0 {
		incomingOrder.Status = etypes.PartialFill
	}

	return &MatchResult{Executions: executions, FullyFilled: fullyFilled}
}

func (m *ProRataMatcher) CanMatch(incoming *etypes.Order, limit *etypes.Limit) bool {
	if incoming.Type == etypes.Market {
		return true
	}
	if incoming.Side == etypes.Buy {
		return incoming.Price >= limit.Price
	}
	return incoming.Price <= limit.Price
}

// MatchesOnlyCrossingPrices reports that this matcher gates every execution on
// CanMatch, which compares the incoming price against the level price, and
// breaks out of its walk at the first level that fails it — so it neither
// executes against nor reads a level the order does not cross.
func (m *ProRataMatcher) MatchesOnlyCrossingPrices() bool { return true }
