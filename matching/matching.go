package matching

import (
	"sync"

	ebook "exchange_sim/book"
	etypes "exchange_sim/types"
)

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
	e.TakerFilledQty = 0
	e.MakerFilledQty = 0
	e.MakerTotalQty = 0
	e.MakerSide = etypes.Buy
	e.MakerPosSide = etypes.PositionBoth
	executionPool.Put(e)
}

// makerAvailable returns how much of a resting order the incoming taker may
// consume right now: full remainder for normal/hidden orders, the current
// display tranche for icebergs (reserve fills only after a refresh re-queues
// the order behind existing liquidity).
func makerAvailable(order *etypes.Order) int64 {
	remaining := order.Qty - order.FilledQty
	if order.Visibility != etypes.Iceberg {
		return remaining
	}
	display := order.DisplayRemaining
	if display <= 0 {
		return 0
	}
	if display < remaining {
		return display
	}
	return remaining
}

// refreshIcebergTranche re-queues an iceberg whose display tranche was fully
// consumed: the order moves to the back of its price level with a fresh
// tranche, losing time priority exactly like a venue refresh. No-op unless
// the order is a resting iceberg with quantity left and an exhausted tranche.
func refreshIcebergTranche(limit *etypes.Limit, order *etypes.Order) {
	if order.Visibility != etypes.Iceberg || order.Parent == nil {
		return
	}
	remaining := order.Qty - order.FilledQty
	if remaining <= 0 || order.DisplayRemaining > 0 {
		return
	}
	ebook.UnlinkOrder(order)
	tranche := order.IcebergQty
	if tranche > remaining {
		tranche = remaining
	}
	order.DisplayRemaining = tranche
	// The order was just unlinked from this valid level, so re-linking its
	// positive remaining tranche must be representable. If it is not, leave it
	// unlinked rather than corrupting the level aggregate.
	if !ebook.LinkOrder(limit, order) {
		order.Status = etypes.Cancelled
	}
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

// PriceCrossingMatcher is an optional promise a matcher may make: that it never
// produces an execution for an order whose price does not cross the opposite
// best, so an order that cannot cross yields no executions and no change to the
// books.
//
// A caller that only needs to know the outcome may then skip building the
// detached book a preview would otherwise match against. It is opt-in because
// the promise is not implied by MatchingEngine: a venue that crosses at a
// midpoint, or auctions, would match orders that never cross the touch, and
// must not implement this.
type PriceCrossingMatcher interface {
	MatchingEngine
	// MatchesOnlyCrossingPrices reports the promise above. Implementations
	// return a constant; it exists as a method so the promise travels with the
	// matcher rather than with a type assertion on a concrete type.
	MatchesOnlyCrossingPrices() bool
}
