package actor

import "exchange_sim/exchange"

type EventType uint8

const (
	EventOrderAccepted EventType = iota
	EventOrderRejected
	EventOrderPartialFill
	EventOrderFilled
	EventOrderCancelled
	EventOrderCancelRejected
	EventTrade
	EventBookDelta
	EventBookSnapshot
	EventFundingUpdate
)

type Event struct {
	Type EventType
	Data any
}

type OrderAcceptedEvent struct {
	OrderID   uint64
	RequestID uint64
}

type OrderRejectedEvent struct {
	RequestID uint64
	Reason    exchange.RejectReason
}

type OrderFillEvent struct {
	OrderID   uint64
	Qty       int64
	Price     int64
	Side      exchange.Side
	IsFull    bool
	TradeID   uint64
	FeeAmount int64
	FeeAsset  string
}

type OrderCancelledEvent struct {
	OrderID      uint64
	RequestID    uint64
	RemainingQty int64
}

type OrderCancelRejectedEvent struct {
	OrderID   uint64
	RequestID uint64
	Reason    exchange.RejectReason
}

type TradeEvent struct {
	Symbol    string
	Trade     *exchange.Trade
	Timestamp int64
}

type BookDeltaEvent struct {
	Symbol    string
	Delta     *exchange.BookDelta
	Timestamp int64
}

type BookSnapshotEvent struct {
	Symbol    string
	Snapshot  *exchange.BookSnapshot
	Timestamp int64
}

type FundingUpdateEvent struct {
	Symbol      string
	FundingRate *exchange.FundingRate
	Timestamp   int64
}
