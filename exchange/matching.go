package exchange

type MatchingEngine interface {
	Match(bidBook, askBook *Book, incomingOrder *Order) []*Execution
	Priority() Priority
}

type DefaultMatcher struct {
	priority Priority
	clock    Clock
}

func NewDefaultMatcher() *DefaultMatcher {
	return &DefaultMatcher{
		priority: Priority{
			Primary:   PriorityPrice,
			Secondary: PriorityVisibility,
			Tertiary:  PriorityTime,
		},
		clock: &RealClock{},
	}
}

func (m *DefaultMatcher) Priority() Priority {
	return m.priority
}

func (m *DefaultMatcher) Match(bidBook, askBook *Book, incomingOrder *Order) []*Execution {
	executions := make([]*Execution, 0, 8)
	var book *Book
	if incomingOrder.Side == Buy {
		book = askBook
	} else {
		book = bidBook
	}

	for incomingOrder.FilledQty < incomingOrder.Qty && book.Best != nil {
		if !m.canMatch(incomingOrder, book.Best) {
			break
		}

		limit := book.Best
		matched := false
		for order := limit.Head; order != nil && incomingOrder.FilledQty < incomingOrder.Qty; {
			next := order.Next
			if m.shouldMatch(incomingOrder, order) {
				exec := m.execute(incomingOrder, order)
				executions = append(executions, exec)
				matched = true

				if order.FilledQty >= order.Qty {
					order.Status = Filled
					unlinkOrder(order)
					delete(book.Orders, order.ID)
					putOrder(order)
				} else {
					order.Status = PartialFill
				}
			}
			order = next
		}

		if isEmpty(limit) {
			book.removeLimit(limit)
		} else if !matched {
			break
		}
	}

	if incomingOrder.FilledQty >= incomingOrder.Qty {
		incomingOrder.Status = Filled
	} else if incomingOrder.FilledQty > 0 {
		incomingOrder.Status = PartialFill
	}

	return executions
}

func (m *DefaultMatcher) canMatch(incoming *Order, limit *Limit) bool {
	if incoming.Type == Market {
		return true
	}
	if incoming.Side == Buy {
		return incoming.Price >= limit.Price
	}
	return incoming.Price <= limit.Price
}

func (m *DefaultMatcher) shouldMatch(incoming, resting *Order) bool {
	if incoming.ClientID == resting.ClientID {
		return false
	}
	return true
}

func (m *DefaultMatcher) execute(taker, maker *Order) *Execution {
	execQty := min(taker.Qty-taker.FilledQty, maker.Qty-maker.FilledQty)
	taker.FilledQty += execQty
	maker.FilledQty += execQty

	exec := getExecution()
	exec.TakerOrderID = taker.ID
	exec.MakerOrderID = maker.ID
	exec.TakerClientID = taker.ClientID
	exec.MakerClientID = maker.ClientID
	exec.Price = maker.Price
	exec.Qty = execQty
	exec.Timestamp = m.clock.NowUnixNano()
	return exec
}
