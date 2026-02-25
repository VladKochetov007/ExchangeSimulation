package matching

import (
	ebook "exchange_sim/exchange/book"
	eclock "exchange_sim/exchange/clock"
	etypes "exchange_sim/exchange/types"
	"sync"
)

// Execution pool — only used within this package.
var executionPool = sync.Pool{
	New: func() any { return &etypes.Execution{} },
}

func getExecution() *etypes.Execution {
	return executionPool.Get().(*etypes.Execution)
}

// GetExecution retrieves an Execution from the pool.
func GetExecution() *etypes.Execution {
	return executionPool.Get().(*etypes.Execution)
}

// PutExecution returns an execution to the pool.
func PutExecution(e *etypes.Execution) {
	e.TakerOrderID = 0
	e.MakerOrderID = 0
	e.TakerClientID = 0
	e.MakerClientID = 0
	e.Price = 0
	e.Qty = 0
	e.Timestamp = 0
	executionPool.Put(e)
}

// MatchResult holds the output of a single matching pass.
type MatchResult struct {
	Executions  []*etypes.Execution
	FullyFilled bool
}

// MatchingEngine is the matching algorithm interface.
type MatchingEngine interface {
	Match(bidBook, askBook *ebook.Book, incomingOrder *etypes.Order) *MatchResult
}

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
