package types

type MarginCallEvent struct {
	Timestamp        int64  `json:"timestamp"`
	ClientID         uint64 `json:"client_id"`
	Symbol           string `json:"symbol"`
	MarginRatioBps   int64  `json:"margin_ratio_bps"`
	LiquidationPrice int64  `json:"liquidation_price"`
}

type LiquidationEvent struct {
	Timestamp     int64  `json:"timestamp"`
	ClientID      uint64 `json:"client_id"`
	Symbol        string `json:"symbol"`
	PositionSize  int64  `json:"position_size"`
	FillPrice     int64  `json:"fill_price"`
	RemainingDebt int64  `json:"remaining_debt"`
}

type InsuranceFundEvent struct {
	Timestamp int64  `json:"timestamp"`
	Symbol    string `json:"symbol"`
	Delta     int64  `json:"delta"`
	Balance   int64  `json:"balance"`
}

type MarginInterestEvent struct {
	Timestamp int64  `json:"timestamp"`
	ClientID  uint64 `json:"client_id"`
	Asset     string `json:"asset"`
	Amount    int64  `json:"amount"`
}

type TransferEvent struct {
	Timestamp  int64  `json:"timestamp"`
	ClientID   uint64 `json:"client_id"`
	FromWallet string `json:"from_wallet"`
	ToWallet   string `json:"to_wallet"`
	Asset      string `json:"asset"`
	Amount     int64  `json:"amount"`
}

type BalanceChangeEvent struct {
	Timestamp int64          `json:"timestamp"`
	ClientID  uint64         `json:"client_id"`
	Symbol    string         `json:"symbol"`
	Reason    string         `json:"reason"`
	Changes   []BalanceDelta `json:"changes"`
}

type BalanceDelta struct {
	Asset      string `json:"asset"`
	Wallet     string `json:"wallet"`
	OldBalance int64  `json:"old_balance"`
	NewBalance int64  `json:"new_balance"`
	Delta      int64  `json:"delta"`
}

type BorrowEvent struct {
	Timestamp      int64  `json:"timestamp"`
	ClientID       uint64 `json:"client_id"`
	Asset          string `json:"asset"`
	Amount         int64  `json:"amount"`
	Reason         string `json:"reason"`
	MarginMode     string `json:"margin_mode"`
	InterestRate   int64  `json:"interest_rate_bps"`
	CollateralUsed int64  `json:"collateral_used"`
}

type RepayEvent struct {
	Timestamp     int64  `json:"timestamp"`
	ClientID      uint64 `json:"client_id"`
	Asset         string `json:"asset"`
	Principal     int64  `json:"principal"`
	Interest      int64  `json:"interest"`
	RemainingDebt int64  `json:"remaining_debt"`
}

type PositionUpdateEvent struct {
	Timestamp     int64  `json:"timestamp"`
	ClientID      uint64 `json:"client_id"`
	Symbol        string `json:"symbol"`
	OldSize       int64  `json:"old_size"`
	OldEntryPrice int64  `json:"old_entry_price"`
	NewSize       int64  `json:"new_size"`
	NewEntryPrice int64  `json:"new_entry_price"`
	TradeQty      int64  `json:"trade_qty"`
	TradePrice    int64  `json:"trade_price"`
	TradeSide     string `json:"trade_side"`
	Reason        string `json:"reason"`
}

type RealizedPnLEvent struct {
	Timestamp  int64  `json:"timestamp"`
	ClientID   uint64 `json:"client_id"`
	Symbol     string `json:"symbol"`
	TradeID    uint64 `json:"trade_id"`
	ClosedQty  int64  `json:"closed_qty"`
	EntryPrice int64  `json:"entry_price"`
	ExitPrice  int64  `json:"exit_price"`
	PnL        int64  `json:"pnl"`
	Side       string `json:"side"`
}

type MarkPriceUpdateEvent struct {
	Timestamp  int64  `json:"timestamp"`
	Symbol     string `json:"symbol"`
	MarkPrice  int64  `json:"mark_price"`
	IndexPrice int64  `json:"index_price"`
}

// FundingRateUpdateEvent logs funding rate changes for perpetual futures
type FundingRateUpdateEvent struct {
	Timestamp   int64  `json:"timestamp"`
	Symbol      string `json:"symbol"`
	Rate        int64  `json:"rate"`
	NextFunding int64  `json:"next_funding"`
}

// OpenInterestEvent logs total open interest for a symbol
type OpenInterestEvent struct {
	Timestamp    int64  `json:"timestamp"`
	Symbol       string `json:"symbol"`
	OpenInterest int64  `json:"open_interest"`
}

// FeeRevenueEvent logs exchange fee revenue per trade
type FeeRevenueEvent struct {
	Timestamp int64  `json:"timestamp"`
	Symbol    string `json:"symbol"`
	TradeID   uint64 `json:"trade_id"`
	TakerFee  int64  `json:"taker_fee"`
	MakerFee  int64  `json:"maker_fee"`
	Asset     string `json:"asset"`
}
