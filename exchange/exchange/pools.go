package exchange

import (
	"sync"

	ebook "exchange_sim/exchange/book"
)

var orderPool = sync.Pool{
	New: func() any {
		return &Order{}
	},
}

func getOrder() *Order {
	return orderPool.Get().(*Order)
}

func putOrder(o *Order) {
	ebook.ResetOrder(o)
	orderPool.Put(o)
}

// GetOrder retrieves an Order from the pool. Used by tests and external tooling.
func GetOrder() *Order { return getOrder() }

// PutOrder returns an Order to the pool, resetting its fields first.
func PutOrder(o *Order) { putOrder(o) }
