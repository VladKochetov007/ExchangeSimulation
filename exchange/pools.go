package exchange

import "sync"

var orderPool = sync.Pool{
	New: func() interface{} {
		return &Order{}
	},
}

var limitPool = sync.Pool{
	New: func() interface{} {
		return &Limit{}
	},
}

var executionPool = sync.Pool{
	New: func() interface{} {
		return &Execution{}
	},
}

var mdMsgPool = sync.Pool{
	New: func() interface{} {
		return &MarketDataMsg{}
	},
}

func getOrder() *Order {
	return orderPool.Get().(*Order)
}

func putOrder(o *Order) {
	resetOrder(o)
	orderPool.Put(o)
}

func getLimit(price int64) *Limit {
	l := limitPool.Get().(*Limit)
	l.Price = price
	return l
}

func putLimit(l *Limit) {
	resetLimit(l)
	limitPool.Put(l)
}

func getExecution() *Execution {
	return executionPool.Get().(*Execution)
}

func putExecution(e *Execution) {
	e.TakerOrderID = 0
	e.MakerOrderID = 0
	e.Price = 0
	e.Qty = 0
	e.Timestamp = 0
	executionPool.Put(e)
}

func getMDMsg() *MarketDataMsg {
	return mdMsgPool.Get().(*MarketDataMsg)
}

func putMDMsg(m *MarketDataMsg) {
	m.Type = MDSnapshot
	m.Symbol = ""
	m.SeqNum = 0
	m.Timestamp = 0
	m.Data = nil
	mdMsgPool.Put(m)
}
