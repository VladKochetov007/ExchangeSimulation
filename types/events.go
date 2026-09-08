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
	// Reason distinguishes fund flows now that the fund is two-directional:
	// "liquidation_deficit" (debit) or "clearance_fee" (credit).
	Reason string `json:"reason,omitempty"`
}

type MarginInterestEvent struct {
	Timestamp int64  `json:"timestamp"`
	ClientID  uint64 `json:"client_id"`
	Asset     string `json:"asset"`
	// Wallet binds the charge to the wallet that funded it; a total-only event
	// cannot distinguish a spot/perp attribution error.
	Wallet string `json:"wallet"`
	Amount int64  `json:"amount"`
}

// MarginInterestAccrualEvent records the fixed-point state transition behind
// one declared interest interval. Interest may be zero while the bounded
// remainder is nonzero; retaining that state makes repeated small debts
// auditable rather than silently free of financing cost.
type MarginInterestAccrualEvent struct {
	Timestamp       int64  `json:"timestamp"`
	ClientID        uint64 `json:"client_id"`
	Asset           string `json:"asset"`
	IntervalSeconds int64  `json:"interval_seconds"`
	Principal       int64  `json:"principal"`
	RateBps         int64  `json:"rate_bps"`
	Interest        int64  `json:"interest"`
	SpotInterest    int64  `json:"spot_interest"`
	PerpInterest    int64  `json:"perp_interest"`
	RemainderBefore int64  `json:"remainder_before"`
	RemainderAfter  int64  `json:"remainder_after"`
	Denominator     int64  `json:"denominator"`
}

// MarginInterestRemainderClosedEvent records the explicit terminal policy for
// sub-unit interest when a debt is fully repaid or liquidated. The current
// fixed-point contract writes the fraction off because it cannot be posted as
// an integer asset unit; it must never disappear without an audit record.
type MarginInterestRemainderClosedEvent struct {
	Timestamp       int64  `json:"timestamp"`
	ClientID        uint64 `json:"client_id"`
	Asset           string `json:"asset"`
	RemainderBefore int64  `json:"remainder_before"`
	RemainderAfter  int64  `json:"remainder_after"`
	Denominator     int64  `json:"denominator"`
	DebtBefore      int64  `json:"debt_before"`
	DebtAfter       int64  `json:"debt_after"`
	Reason          string `json:"reason"`
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
	Timestamp    int64          `json:"timestamp"`
	ClientID     uint64         `json:"client_id"`
	Symbol       string         `json:"symbol"`
	PositionSide string         `json:"position_side,omitempty"`
	Reason       string         `json:"reason"`
	Changes      []BalanceDelta `json:"changes"`
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
	// Reason distinguishes a client repayment from an internal compensating
	// repayment. It is omitted for historical events, so adding it is
	// JSONL-schema compatible with existing log consumers.
	Reason string `json:"reason,omitempty"`
}

type PositionUpdateEvent struct {
	Timestamp     int64  `json:"timestamp"`
	ClientID      uint64 `json:"client_id"`
	Symbol        string `json:"symbol"`
	PositionSide  string `json:"position_side,omitempty"`
	BasePrecision int64  `json:"base_precision,omitempty"`
	OldSize       int64  `json:"old_size"`
	OldEntryPrice int64  `json:"old_entry_price"`
	NewSize       int64  `json:"new_size"`
	NewEntryPrice int64  `json:"new_entry_price"`
	TradeQty      int64  `json:"trade_qty"`
	TradePrice    int64  `json:"trade_price"`
	TradeSide     string `json:"trade_side"`
	Reason        string `json:"reason"`
}

type PositionRoundingEvent struct {
	Timestamp          int64  `json:"timestamp"`
	ClientID           uint64 `json:"client_id"`
	Symbol             string `json:"symbol"`
	Asset              string `json:"asset"`
	CashAdjustment     int64  `json:"cash_adjustment"`
	RemainderNumerator int64  `json:"remainder_numerator"`
	Precision          int64  `json:"precision"`
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

// PriceUnavailableEvent records an explicitly deferred internal operation.
// It is deliberately separate from a numeric price event: a missing reference
// must not look like a price of zero in scientific evidence or in an actor's
// reconstruction of venue state.
type PriceUnavailableEvent struct {
	Timestamp int64  `json:"timestamp"`
	Symbol    string `json:"symbol"`
	Operation string `json:"operation"`
	Reason    string `json:"reason"`
}

// ExpirySettlementPendingEvent records a contract that reached expiry but is
// still waiting on its declared reference price. It has no settlement-price
// field by design: price absence is a lifecycle state, never numeric zero.
type ExpirySettlementPendingEvent struct {
	Timestamp       int64  `json:"timestamp"`
	Symbol          string `json:"symbol"`
	State           string `json:"state"`
	Policy          string `json:"policy"`
	Attempts        uint64 `json:"attempts"`
	ExpiryReachedAt int64  `json:"expiry_reached_at"`
	Reason          string `json:"reason"`
}

// OptionExpiryAccountingEvent records the aggregate cash contract for one
// successfully settled option book. Position-level payouts are integer
// rounded, so an independent audit needs the net book, gross payout, explicit
// residual, delivery fees, and the venue movement that closes that residual.
type OptionExpiryAccountingEvent struct {
	Timestamp          int64  `json:"timestamp"`
	Symbol             string `json:"symbol"`
	QuoteAsset         string `json:"quote_asset"`
	BasePrecision      int64  `json:"base_precision"`
	SettlementPrice    int64  `json:"settlement_price"`
	PositionCount      int    `json:"position_count"`
	NetPositionSize    int64  `json:"net_position_size"`
	GrossCashFlow      int64  `json:"gross_cash_flow"`
	ExpectedCashFlow   int64  `json:"expected_cash_flow"`
	RoundingResidual   int64  `json:"rounding_residual"`
	VenueRoundingDelta int64  `json:"venue_rounding_delta"`
	DeliveryFeeTotal   int64  `json:"delivery_fee_total"`
}

// FundingRateUpdateEvent logs funding rate changes for perpetual futures
type FundingRateUpdateEvent struct {
	Timestamp   int64  `json:"timestamp"`
	Symbol      string `json:"symbol"`
	Rate        int64  `json:"rate"`
	NextFunding int64  `json:"next_funding"`
	Interval    int64  `json:"interval"`
}

// FundingSettlementEvent is emitted even when integer funding rounds to zero.
// A balance-change stream alone cannot distinguish a valid zero-cash
// settlement from a missed settlement deadline.
type FundingSettlementEvent struct {
	Timestamp     int64  `json:"timestamp"`
	Symbol        string `json:"symbol"`
	Rate          int64  `json:"rate"`
	NextFunding   int64  `json:"next_funding"`
	Interval      int64  `json:"interval"`
	MarkPrice     int64  `json:"mark_price"`
	BasePrecision int64  `json:"base_precision"`
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
