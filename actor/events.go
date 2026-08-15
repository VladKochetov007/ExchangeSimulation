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
	// EventInstrument carries listing/settlement announcements from the
	// exchange's reference-data feed (subscribe to InstrumentFeedSymbol).
	EventInstrument
	// EventIndex carries the venue's published index price.
	EventIndex
	EventBalanceUpdate
	EventAccountUpdate
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
	OrderID uint64 `json:"order_id"`
	// Symbol identifies the filled leg. Multi-leg strategies (basis, triangle,
	// parity) cannot reconcile intent against reality without it: market
	// orders fill before their accept response arrives, so an order-ID lookup
	// is not yet populated when the first fill lands.
	Symbol    string        `json:"symbol"`
	Qty       int64         `json:"qty"`
	Price     int64         `json:"price"`
	Side      exchange.Side `json:"side"`
	IsFull    bool          `json:"is_full"`
	TradeID   uint64        `json:"trade_id"`
	FeeAmount int64         `json:"fee_amount"`
	FeeAsset  string        `json:"fee_asset"`
	// Timestamp is the exchange-side execution time, not actor receipt time.
	Timestamp int64 `json:"timestamp"`
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

// IndexEvent is the venue's published reference price for a symbol.
type IndexEvent struct {
	Symbol    string
	Price     int64
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

type InstrumentEvent struct {
	Announcement *exchange.InstrumentAnnouncement
	Timestamp    int64
}

type BalanceUpdateEvent struct {
	Snapshot *exchange.BalanceSnapshot
}

type AccountUpdateEvent struct {
	Snapshot *exchange.AccountSnapshot
}
