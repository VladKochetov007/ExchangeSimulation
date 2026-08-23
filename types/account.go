package types

type BalanceSnapshot struct {
	Timestamp    int64            `json:"timestamp"`
	ClientID     uint64           `json:"client_id"`
	SpotBalances []AssetBalance   `json:"spot_balances"`
	PerpBalances []AssetBalance   `json:"perp_balances"`
	Borrowed     map[string]int64 `json:"borrowed"`
}

type AssetBalance struct {
	Asset    string `json:"asset"`
	Free     int64  `json:"free"`
	Locked   int64  `json:"locked"`
	Borrowed int64  `json:"borrowed"`
	Interest int64  `json:"interest"`
	NetAsset int64  `json:"net_asset"`
}

type Position struct {
	ClientID     uint64       `json:"client_id"`
	Symbol       string       `json:"symbol"`
	PositionSide PositionSide `json:"position_side"`
	Size         int64        `json:"size"`
	EntryPrice   int64        `json:"entry_price"`
	Margin       int64        `json:"margin"`
}

type PositionSnapshot struct {
	Symbol       string       `json:"symbol"`
	PositionSide PositionSide `json:"position_side"`
	Size         int64        `json:"size"`
	EntryPrice   int64        `json:"entry_price"`
	// MarkPrice is nil when the display reference is unavailable. A zero
	// premium can be a valid option mark, so a pointer carries absence without
	// turning it into the numeric price zero.
	MarkPrice             *int64     `json:"mark_price,omitempty"`
	MarkUnavailableReason string     `json:"mark_unavailable_reason,omitempty"`
	UnrealizedPnL         int64      `json:"unrealized_pnl"`
	MarginType            MarginMode `json:"margin_type"`
	IsolatedMargin        int64      `json:"isolated_margin"`
	Leverage              int64      `json:"leverage"`
	// LiquidationPrice is nil when no estimate has been calculated for this
	// display snapshot. A computed zero can be a legitimate estimate for a
	// sufficiently collateralized long, so absence cannot be encoded as 0.
	LiquidationPrice *int64 `json:"liquidation_price,omitempty"`
}

type AccountSnapshot struct {
	BalanceSnapshot
	Positions []PositionSnapshot `json:"positions"`
}

// AssetValuationMark converts an asset's fixed-point balance into one
// reporting asset. Price is reporting-asset units per whole Asset; Precision
// is the number of fixed-point units in one whole Asset.
//
// Example: valuing ABC and USD in USD precision, an ABC/USD mid of 50,000 USD
// is expressed as Price=5_000_000_000 and Precision=100_000_000 for ABC;
// USD itself uses Price=100_000 and Precision=100_000.
type AssetValuationMark struct {
	Price     int64 `json:"price"`
	Precision int64 `json:"precision"`
}

// AccountValuationSpec makes the numeraire and every asset conversion
// explicit. Marked account reports fail rather than silently drop a non-zero
// asset or derivative quote currency without a valid mark.
type AccountValuationSpec struct {
	ReportAsset     string                        `json:"report_asset"`
	ReportPrecision int64                         `json:"report_precision"`
	AssetMarks      map[string]AssetValuationMark `json:"asset_marks"`
}

// MarkedAccountSnapshot is a synchronous mark-to-market account report in the
// requested reporting asset. Wallet balances include locked cash and subtract
// attributed debt exactly once. Futures-style positions contribute entry-to-mark
// PnL; cash-premium options contribute their signed current market value because
// their entry premium was already transferred through the wallet at each fill.
type MarkedAccountSnapshot struct {
	AccountSnapshot
	ReportAsset          string `json:"report_asset"`
	ReportPrecision      int64  `json:"report_precision"`
	SpotEquity           int64  `json:"spot_equity"`
	PerpCashEquity       int64  `json:"perp_cash_equity"`
	IsolatedEquity       int64  `json:"isolated_equity"`
	DerivativeUnrealized int64  `json:"derivative_unrealized"`
	OptionMarketValue    int64  `json:"option_market_value"`
	Maintenance          int64  `json:"maintenance"`
	Equity               int64  `json:"equity"`
}

type IsolatedPosition struct {
	Symbol     string
	Collateral map[string]int64
	Borrowed   map[string]int64
}

type BorrowingConfig struct {
	Enabled           bool
	AutoBorrowSpot    bool
	AutoBorrowPerp    bool
	DefaultMarginMode MarginMode

	BorrowRates       map[string]int64
	CollateralFactors map[string]float64
	MaxBorrowPerAsset map[string]int64

	// AssetPrecisions maps asset → units per whole asset for collateral
	// valuation (e.g. "USD" → 100_000). Assets absent from the map fall back
	// to BTC precision (1e8).
	AssetPrecisions map[string]int64

	PriceSource PriceSource
}
