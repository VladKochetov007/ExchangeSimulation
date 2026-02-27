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
	Symbol           string       `json:"symbol"`
	PositionSide     PositionSide `json:"position_side"`
	Size             int64        `json:"size"`
	EntryPrice       int64        `json:"entry_price"`
	MarkPrice        int64        `json:"mark_price"`
	UnrealizedPnL    int64        `json:"unrealized_pnl"`
	MarginType       MarginMode   `json:"margin_type"`
	IsolatedMargin   int64        `json:"isolated_margin"`
	Leverage         int64        `json:"leverage"`
	LiquidationPrice int64        `json:"liquidation_price"`
}

type AccountSnapshot struct {
	BalanceSnapshot
	Positions []PositionSnapshot `json:"positions"`
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

	PriceSource PriceSource
}
