package types

type Order struct {
	ID           uint64       `json:"order_id"`
	ClientID     uint64       `json:"client_id"`
	Side         Side         `json:"side"`
	PositionSide PositionSide `json:"position_side"`
	Type         OrderType    `json:"type"`
	TimeInForce  TimeInForce  `json:"time_in_force"`
	Price        int64        `json:"price"`
	Qty          int64        `json:"qty"`
	FilledQty    int64        `json:"filled_qty"`
	Visibility   Visibility   `json:"visibility"`
	IcebergQty   int64        `json:"iceberg_qty"`
	// DisplayRemaining is the unfilled portion of an iceberg's current display
	// tranche. Matchers cap fills at this and re-queue the order at the back
	// of its level when the tranche is exhausted (venue refresh semantics).
	DisplayRemaining int64       `json:"-"`
	Status           OrderStatus `json:"status"`
	Timestamp        int64       `json:"timestamp"`
	// Reserved is the remaining amount locked for this order (quote units for
	// buys and margined orders, base units for spot sells). The exchange is the
	// single writer; releases are computed as deltas against this ledger so
	// price improvement and fee headroom never leak reserved funds.
	Reserved int64 `json:"-"`
	// FeeReserved backs fees charged in assets outside the order's trade-leg
	// reservation, keyed by asset. It is released or recomputed as fills reduce
	// the order, preventing several resting orders from spending one fee balance.
	FeeReserved map[string]int64 `json:"-"`

	Prev   *Order `json:"-"`
	Next   *Order `json:"-"`
	Parent *Limit `json:"-"`
}

type Limit struct {
	Price    int64
	TotalQty int64
	OrderCnt int32

	Head *Order
	Tail *Order

	Prev *Limit
	Next *Limit
}
