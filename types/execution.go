package types

type Execution struct {
	TakerOrderID  uint64 `json:"taker_order_id"`
	MakerOrderID  uint64 `json:"maker_order_id"`
	TakerClientID uint64 `json:"taker_client_id"`
	MakerClientID uint64 `json:"maker_client_id"`
	Price         int64  `json:"price"`
	Qty           int64  `json:"qty"`
	Timestamp     int64  `json:"timestamp"`
	// TakerFilledQty is the taker's cumulative fill AFTER this execution,
	// captured at match time so per-execution notifications don't report the
	// taker's final state on every intermediate fill.
	TakerFilledQty int64 `json:"taker_filled_qty"`
	MakerFilledQty int64 `json:"maker_filled_qty"`
	MakerTotalQty  int64 `json:"maker_total_qty"`
	MakerSide      Side  `json:"maker_side"`
	// MakerPosSide is captured at match time because fully filled maker orders
	// are removed from the book before settlement can look them up.
	MakerPosSide PositionSide `json:"maker_pos_side"`
}

type Fee struct {
	Asset  string `json:"asset"`
	Amount int64  `json:"amount"`
}

// PositionDelta contains position state before and after an update.
type PositionDelta struct {
	OldSize       int64
	OldEntryPrice int64
	NewSize       int64
	NewEntryPrice int64
}
