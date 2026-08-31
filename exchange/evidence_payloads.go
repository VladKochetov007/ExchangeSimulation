package exchange

// These payload types are the typed boundary shared by the JSON evidence
// logger and the successor binary evidence stream. Keeping the declarations
// separate from order admission and settlement makes the representation change
// auditable: adding a codec does not change matching or account state.

// bookSnapshotEvidence is the complete venue-side snapshot. Nil and empty
// sides are intentionally distinct in the binary schema, matching JSON null
// versus [].
type bookSnapshotEvidence struct {
	Asks []PriceLevel `json:"asks"`
	Bids []PriceLevel `json:"bids"`
}

// bookDeltaEvidence contains both public and hidden quantities. The public
// market-data path only exposes VisibleQty; the evidence path retains the
// complete level state for reconstruction.
type bookDeltaEvidence struct {
	HiddenQty  int64  `json:"hidden_qty"`
	Price      int64  `json:"price"`
	Side       string `json:"side"`
	TotalQty   int64  `json:"total_qty"`
	VisibleQty int64  `json:"visible_qty"`
}

// fillEvidence is the persisted OrderFill payload. It deliberately mirrors
// the established JSON field names so the successor can compare decoded
// binary events against retained JSON runs without changing economic state.
type fillEvidence struct {
	FeeAmount     int64  `json:"fee_amount"`
	FeeAsset      string `json:"fee_asset"`
	FilledQty     int64  `json:"filled_qty"`
	IsFull        bool   `json:"is_full"`
	NewEntryPrice int64  `json:"new_entry_price"`
	NewSize       int64  `json:"new_size"`
	OrderID       uint64 `json:"order_id"`
	PositionSide  string `json:"position_side"`
	Price         int64  `json:"price"`
	Qty           int64  `json:"qty"`
	RealizedPnL   int64  `json:"realized_pnl"`
	RemainingQty  int64  `json:"remaining_qty"`
	Role          string `json:"role"`
	Side          string `json:"side"`
	Symbol        string `json:"symbol"`
	TradeID       uint64 `json:"trade_id"`
}
