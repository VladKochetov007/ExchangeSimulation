package matching

import (
	ebook "exchange_sim/book"
	eclock "exchange_sim/clock"
	etypes "exchange_sim/types"
)

// DefaultMatcher implements price-time priority (FIFO) matching.
type DefaultMatcher struct {
	clock etypes.Clock
}

func NewDefaultMatcher(clock etypes.Clock) *DefaultMatcher {
	if clock == nil {
		clock = &eclock.RealClock{}
	}
	return &DefaultMatcher{clock: clock}
}

func (m *DefaultMatcher) Match(bidBook, askBook *ebook.Book, incomingOrder *etypes.Order) *MatchResult {
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
		matched := false
		for order := limit.Head; order != nil && incomingOrder.FilledQty < incomingOrder.Qty; {
			next := order.Next
			if order.FilledQty < order.Qty && m.shouldMatch(incomingOrder, order) {
				exec := m.execute(incomingOrder, order)
				executions = append(executions, exec)
				matched = true

				if order.FilledQty >= order.Qty {
					order.Status = etypes.Filled
					// CRITICAL: Remove filled order from book so matching can continue to next level
					ebook.UnlinkOrder(order)
					delete(book.Orders, order.ID)
				} else {
					order.Status = etypes.PartialFill
				}
			}
			order = next
		}

		if ebook.IsEmpty(limit) {
			book.RemoveLimit(limit)
		} else if !matched {
			break
		}
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

func (m *DefaultMatcher) CanMatch(incoming *etypes.Order, limit *etypes.Limit) bool {
	if incoming.Type == etypes.Market {
		return true
	}
	if incoming.Side == etypes.Buy {
		return incoming.Price >= limit.Price
	}
	return incoming.Price <= limit.Price
}

func (m *DefaultMatcher) shouldMatch(incoming, resting *etypes.Order) bool {
	return incoming.ClientID != resting.ClientID
}

func (m *DefaultMatcher) execute(taker, maker *etypes.Order) *etypes.Execution {
	execQty := min(taker.Qty-taker.FilledQty, maker.Qty-maker.FilledQty)
	if execQty <= 0 {
		panic("matching engine bug: attempted zero-quantity execution")
	}

	taker.FilledQty += execQty
	maker.FilledQty += execQty
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
	exec.MakerFilledQty = maker.FilledQty
	exec.MakerTotalQty = maker.Qty
	exec.MakerSide = maker.Side
	return exec
}
