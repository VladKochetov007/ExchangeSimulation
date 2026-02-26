package types

type Execution struct {
	TakerOrderID   uint64 `json:"taker_order_id"`
	MakerOrderID   uint64 `json:"maker_order_id"`
	TakerClientID  uint64 `json:"taker_client_id"`
	MakerClientID  uint64 `json:"maker_client_id"`
	Price          int64  `json:"price"`
	Qty            int64  `json:"qty"`
	Timestamp      int64  `json:"timestamp"`
	MakerFilledQty int64  `json:"maker_filled_qty"`
	MakerTotalQty  int64  `json:"maker_total_qty"`
	MakerSide      Side   `json:"maker_side"`
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
