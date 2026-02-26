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

	for incomingOrder.FilledQty < incomingOrder.Qty && book.Best != nil {
		if !m.CanMatch(incomingOrder, book.Best) {
			break
		}

		limit := book.Best
		remaining := incomingOrder.Qty - incomingOrder.FilledQty

		// Collect eligible resting orders and total available qty at this level.
		type candidate struct{ order *etypes.Order }
		var candidates []candidate
		totalQty := int64(0)
		for o := limit.Head; o != nil; o = o.Next {
			if o.FilledQty < o.Qty && o.ClientID != incomingOrder.ClientID {
				candidates = append(candidates, candidate{o})
				totalQty += o.Qty - o.FilledQty
			}
		}
		if totalQty == 0 {
			break
		}

		// Distribute fills proportionally. Each maker gets floor(remaining * share),
		// capped at available so remaining > totalQty never over-fills a maker.
		// Leftovers are assigned in resting order (FIFO tiebreaker) until exhausted.
		filled := int64(0)
		shares := make([]int64, len(candidates))
		for i, c := range candidates {
			available := c.order.Qty - c.order.FilledQty
			shares[i] = min(remaining*available/totalQty, available)
			filled += shares[i]
		}
		leftover := min(remaining-filled, totalQty)
		for i := range candidates {
			if leftover == 0 {
				break
			}
			available := candidates[i].order.Qty - candidates[i].order.FilledQty
			extra := min(available-shares[i], leftover)
			shares[i] += extra
			leftover -= extra
		}

		// Emit one execution per maker with a non-zero share.
		now := m.clock.NowUnixNano()
		for i, c := range candidates {
			if shares[i] == 0 {
				continue
			}
			execQty := shares[i]
			incomingOrder.FilledQty += execQty
			c.order.FilledQty += execQty
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
			exec.MakerFilledQty = c.order.FilledQty
			exec.MakerTotalQty = c.order.Qty
			exec.MakerSide = c.order.Side
			executions = append(executions, exec)

			if c.order.FilledQty >= c.order.Qty {
				c.order.Status = etypes.Filled
				ebook.UnlinkOrder(c.order)
				delete(book.Orders, c.order.ID)
			} else {
				c.order.Status = etypes.PartialFill
			}
		}

		if ebook.IsEmpty(limit) {
			book.RemoveLimit(limit)
		}
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
