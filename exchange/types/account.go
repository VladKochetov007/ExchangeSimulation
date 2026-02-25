package types

type BalanceSnapshot struct {
	Timestamp    int64            `json:"timestamp"`
	ClientID     uint64           `json:"client_id"`
	SpotBalances []AssetBalance   `json:"spot_balances"`
	PerpBalances []AssetBalance   `json:"perp_balances"`
	Borrowed     map[string]int64 `json:"borrowed"`
}

type AssetBalance struct {
	Asset     string `json:"asset"`
	Total     int64  `json:"total"`
	Available int64  `json:"available"`
	Reserved  int64  `json:"reserved"`
}

type Position struct {
	ClientID   uint64 `json:"client_id"`
	Symbol     string `json:"symbol"`
	Size       int64  `json:"size"`
	EntryPrice int64  `json:"entry_price"`
	Margin     int64  `json:"margin"`
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

	PriceOracle PriceOracle
}
