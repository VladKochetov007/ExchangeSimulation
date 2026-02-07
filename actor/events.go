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
	EventOpenInterest
)

type Event struct {
	Type EventType
	Data any
}

type OrderAcceptedEvent struct {
	OrderID   uint64 `json:"order_id"`
	RequestID uint64 `json:"request_id"`
}

type OrderRejectedEvent struct {
	RequestID uint64                `json:"request_id"`
	Reason    exchange.RejectReason `json:"reason"`
}

type OrderFillEvent struct {
	OrderID   uint64        `json:"order_id"`
	Qty       int64         `json:"qty"`
	Price     int64         `json:"price"`
	Side      exchange.Side `json:"side"`
	IsFull    bool          `json:"is_full"`
	TradeID   uint64        `json:"trade_id"`
	FeeAmount int64         `json:"fee_amount"`
	FeeAsset  string        `json:"fee_asset"`
}

type OrderCancelledEvent struct {
	OrderID      uint64 `json:"order_id"`
	RequestID    uint64 `json:"request_id"`
	RemainingQty int64  `json:"remaining_qty"`
}

type OrderCancelRejectedEvent struct {
	OrderID   uint64                `json:"order_id"`
	RequestID uint64                `json:"request_id"`
	Reason    exchange.RejectReason `json:"reason"`
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
	SeqNum    uint64
}

type BookSnapshotEvent struct {
	Symbol    string
	Snapshot  *exchange.BookSnapshot
	Timestamp int64
	SeqNum    uint64
}

type FundingUpdateEvent struct {
	Symbol      string
	FundingRate *exchange.FundingRate
	Timestamp   int64
}

type OpenInterestEvent struct {
	Symbol       string
	OpenInterest *exchange.OpenInterest
	Timestamp    int64
}
