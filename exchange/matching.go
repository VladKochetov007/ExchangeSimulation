package exchange

type MatchResult struct {
	Executions  []*Execution
	FullyFilled bool
}

type MatchingEngine interface {
	Match(bidBook, askBook *Book, incomingOrder *Order) *MatchResult
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

func (m *DefaultMatcher) Match(bidBook, askBook *Book, incomingOrder *Order) *MatchResult {
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
			if order.FilledQty < order.Qty && m.shouldMatch(incomingOrder, order) {
				exec := m.execute(incomingOrder, order)
				executions = append(executions, exec)
				matched = true

				if order.FilledQty >= order.Qty {
					order.Status = Filled
					// CRITICAL: Remove filled order from book so matching can continue to next level
					unlinkOrder(order)
					delete(book.Orders, order.ID)
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

	fullyFilled := incomingOrder.FilledQty >= incomingOrder.Qty
	if fullyFilled {
		incomingOrder.Status = Filled
	} else if incomingOrder.FilledQty > 0 {
		incomingOrder.Status = PartialFill
	}

	return &MatchResult{
		Executions:  executions,
		FullyFilled: fullyFilled,
	}
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

// ProRataMatcher matches at each price level by distributing fills proportionally
// to resting order size rather than by arrival time (FIFO).
// Used by CME Globex and Euronext for liquid futures contracts.
// Time priority is used only as a tiebreaker when quantities are equal.
type ProRataMatcher struct {
	clock Clock
}

func NewProRataMatcher() *ProRataMatcher {
	return &ProRataMatcher{clock: &RealClock{}}
}

func (m *ProRataMatcher) Priority() Priority {
	return Priority{Primary: PriorityPrice}
}

func (m *ProRataMatcher) Match(bidBook, askBook *Book, incomingOrder *Order) *MatchResult {
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
		remaining := incomingOrder.Qty - incomingOrder.FilledQty

		// Collect eligible resting orders and total available qty at this level.
		type candidate struct{ order *Order }
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
		leftover := remaining - filled
		if leftover > totalQty {
			leftover = totalQty
		}
		for i := range candidates {
			if leftover == 0 {
				break
			}
			available := candidates[i].order.Qty - candidates[i].order.FilledQty
			extra := available - shares[i]
			if extra > leftover {
				extra = leftover
			}
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
				c.order.Status = Filled
				unlinkOrder(c.order)
				delete(book.Orders, c.order.ID)
			} else {
				c.order.Status = PartialFill
			}
		}

		if isEmpty(limit) {
			book.removeLimit(limit)
		}
	}

	fullyFilled := incomingOrder.FilledQty >= incomingOrder.Qty
	if fullyFilled {
		incomingOrder.Status = Filled
	} else if incomingOrder.FilledQty > 0 {
		incomingOrder.Status = PartialFill
	}

	return &MatchResult{Executions: executions, FullyFilled: fullyFilled}
}

func (m *ProRataMatcher) canMatch(incoming *Order, limit *Limit) bool {
	if incoming.Type == Market {
		return true
	}
	if incoming.Side == Buy {
		return incoming.Price >= limit.Price
	}
	return incoming.Price <= limit.Price
}
