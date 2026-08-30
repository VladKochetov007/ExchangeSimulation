package matching

import (
	ebook "exchange_sim/book"
	eclock "exchange_sim/clock"
	etypes "exchange_sim/types"
)

// PriceTimeMatcher implements price-time priority (FIFO) matching.
type PriceTimeMatcher struct {
	clock etypes.Clock
}

func NewPriceTimeMatcher(clock etypes.Clock) *PriceTimeMatcher {
	if clock == nil {
		clock = &eclock.RealClock{}
	}
	return &PriceTimeMatcher{clock: clock}
}

func (m *PriceTimeMatcher) Match(bidBook, askBook *ebook.Book, incomingOrder *etypes.Order) *MatchResult {
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
		// Rescan the level after iceberg refreshes: a refreshed tranche is new
		// liquidity behind the queue, and an aggressive order keeps eating it.
		// Terminates because every pass with a refresh strictly consumed qty.
		for {
			refreshed := false
			for order := limit.Head; order != nil && incomingOrder.FilledQty < incomingOrder.Qty; {
				next := order.Next
				if order.FilledQty < order.Qty && m.shouldMatch(incomingOrder, order) {
					exec := m.execute(incomingOrder, order)
					executions = append(executions, exec)

					if order.FilledQty >= order.Qty {
						order.Status = etypes.Filled
						// Unlink from the price level so matching continues, but keep
						// the book.Orders index entry: the exchange settles against the
						// order (reservation ledger, position side) and removes it in
						// removeMakerOrders after settlement.
						ebook.UnlinkOrder(order)
					} else {
						order.Status = etypes.PartialFill
						if order.Visibility == etypes.Iceberg && order.DisplayRemaining == 0 {
							// Exhausted tranche re-queues at the level tail with a
							// fresh display, losing time priority (venue refresh).
							refreshIcebergTranche(limit, order)
							refreshed = true
						}
					}
				}
				order = next
			}
			if !refreshed || incomingOrder.FilledQty >= incomingOrder.Qty {
				break
			}
		}

		if ebook.IsEmpty(limit) {
			book.RemoveLimit(limit)
		}
		// Advance even when nothing matched here: a level holding only the
		// incoming client's own orders must not blockade deeper liquidity.
		limit = nextLimit
	}

	fullyFilled := incomingOrder.FilledQty >= incomingOrder.Qty
	if fullyFilled {
		incomingOrder.Status = etypes.Filled
	} else if incomingOrder.FilledQty > 0 {
		incomingOrder.Status = etypes.PartialFill
	}

	return &MatchResult{
		Executions:  executions,
		FullyFilled: fullyFilled,
	}
}

func (m *PriceTimeMatcher) CanMatch(incoming *etypes.Order, limit *etypes.Limit) bool {
	if incoming.Type == etypes.Market {
		return true
	}
	if incoming.Side == etypes.Buy {
		return incoming.Price >= limit.Price
	}
	return incoming.Price <= limit.Price
}

func (m *PriceTimeMatcher) shouldMatch(incoming, resting *etypes.Order) bool {
	return incoming.ClientID != resting.ClientID
}

func (m *PriceTimeMatcher) execute(taker, maker *etypes.Order) *etypes.Execution {
	execQty := min(taker.Qty-taker.FilledQty, makerAvailable(maker))
	if execQty <= 0 {
		panic("matching engine bug: attempted zero-quantity execution")
	}

	taker.FilledQty += execQty
	maker.FilledQty += execQty
	if maker.Visibility == etypes.Iceberg && maker.DisplayRemaining > 0 {
		maker.DisplayRemaining -= execQty
	}
	if maker.Parent != nil {
		maker.Parent.TotalQty -= execQty
	}

	exec := getExecution()
	exec.TakerOrderID = taker.ID
	exec.MakerOrderID = maker.ID
	exec.TakerClientID = taker.ClientID
	exec.MakerClientID = maker.ClientID
	exec.Price = maker.Price
	exec.Qty = execQty
	exec.Timestamp = m.clock.NowUnixNano()
	exec.TakerFilledQty = taker.FilledQty
	exec.MakerFilledQty = maker.FilledQty
	exec.MakerTotalQty = maker.Qty
	exec.MakerSide = maker.Side
	exec.MakerPosSide = maker.PositionSide
	return exec
}

// MatchesOnlyCrossingPrices reports that this matcher gates every execution on
// CanMatch, which compares the incoming price against the level price, and
// breaks out of its walk at the first level that fails it — so it neither
// executes against nor reads a level the order does not cross.
func (m *PriceTimeMatcher) MatchesOnlyCrossingPrices() bool { return true }
