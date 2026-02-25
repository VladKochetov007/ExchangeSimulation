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
