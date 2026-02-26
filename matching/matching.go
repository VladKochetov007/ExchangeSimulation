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
