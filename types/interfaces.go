package types

import "time"

// Gateway is the actor-facing contract for any trading venue.
type Gateway interface {
	ID() uint64
	Send(req Request)
	Responses() <-chan Response
	MarketDataCh() <-chan *MarketDataMsg
	IsRunning() bool
}

// Venue is the minimal contract any trading venue must satisfy.
type Venue interface {
	ConnectClient(clientID uint64, balances map[string]int64, feePlan FeeModel) Gateway
	Shutdown()
	IsRunning() bool
}

type PriceSource interface {
	Price(symbol string) int64
}

// PositionStore is the minimal interface for position tracking.
// Implement this to substitute custom position persistence (e.g. database-backed).
type PositionStore interface {
	// UpdatePosition applies a trade delta and returns old/new state.
	// Logging is the caller's responsibility.
	UpdatePosition(clientID uint64, symbol string, qty, price int64, tradeSide Side, posSide PositionSide) PositionDelta

	GetPosition(clientID uint64, symbol string) *Position
	GetPositionBySide(clientID uint64, symbol string, posSide PositionSide) *Position

	// HasOpenPositions returns true if the client has any non-zero positions.
	HasOpenPositions(clientID uint64) bool

	// CalculateOpenInterest returns the sum of absolute position sizes for symbol.
	CalculateOpenInterest(symbol string) int64

	// PositionsForFunding calls fn for every non-zero position for symbol.
	// fn receives a value copy — do not store the pointer.
	PositionsForFunding(symbol string, fn func(clientID uint64, pos Position))

	// GetAllPositions returns a snapshot of all non-zero positions for clientID.
	GetAllPositions(clientID uint64) []Position
}

// Logger is the event logging interface for the exchange.
type Logger interface {
	LogEvent(simTime int64, clientID uint64, eventName string, event any)
}

// FillContext is passed to FeeModel.CalculateFee per execution.
type FillContext struct {
	Exec       *Execution
	IsMaker    bool
	BaseAsset  string
	QuoteAsset string
	Precision  int64
}

// FeeModel calculates trading fees for each execution.
type FeeModel interface {
	CalculateFee(ctx FillContext) Fee
}

// Instrument describes a tradeable asset pair.
type Instrument interface {
	Symbol() string
	BaseAsset() string
	QuoteAsset() string
	BasePrecision() int64
	QuotePrecision() int64
	TickSize() int64
	MinOrderSize() int64
	ValidatePrice(price int64) bool
	ValidateQty(qty int64) bool
	IsPerp() bool
	InstrumentType() string
}

// Instrumentable is implemented by venues that manage tradeable instruments.
type Instrumentable interface {
	AddInstrument(instrument Instrument)
	ListInstruments(baseFilter, quoteFilter string) []Instrument
}

// ClientLifecycle covers client-side lifecycle management.
type ClientLifecycle interface {
	CancelAllClientOrders(clientID uint64) int
	DisconnectClient(clientID uint64)
	SetLogger(symbol string, log Logger)
}

// MarginLending adds collateral borrowing for leveraged trading.
type MarginLending interface {
	EnableBorrowing(config BorrowingConfig) error
	BorrowMargin(clientID uint64, asset string, amount int64, reason string) error
	RepayMargin(clientID uint64, asset string, amount int64) error
}

// PerpWallet manages the perp account and cross-wallet transfers.
type PerpWallet interface {
	AddPerpBalance(clientID uint64, asset string, amount int64)
	Transfer(clientID uint64, fromWallet, toWallet, asset string, amount int64) error
}

// SpotExchange is the management API for a spot/margin trading venue.
type SpotExchange interface {
	Venue
	Instrumentable
	ClientLifecycle
	MarginLending
}

// PerpExchange is the management API for a perpetual futures venue.
type PerpExchange interface {
	Venue
	Instrumentable
	ClientLifecycle
	PerpWallet
}

// Margined is implemented by instruments that use margin-based fund reservation.
// The exchange calls these instead of the IsPerp()+type-assert path.
type Margined interface {
	MarginRequired(qty, price, precision int64) int64
	MarginForMarket(qty, refPrice, precision int64) int64
	MarginOnCancel(remainingQty, orderPrice, precision int64) int64
}

// SettlementContext carries all state an instrument needs to settle one execution.
// Account mutation callbacks are closures that capture the exchange's internal client map,
// so the instrument never needs to import the exchange package.
type SettlementContext struct {
	Exec         *Execution
	TakerOrder   *Order       // nil on force-close (liquidation) path
	MakerPosSide PositionSide // resolved by the exchange before calling Settle
	TakerFee     Fee
	MakerFee     Fee
	Positions    PositionStore

	// Account mutation callbacks.
	PerpBalance      func(clientID uint64, asset string) int64
	MutatePerpBalance func(clientID uint64, asset string, delta int64)
	ReservePerp      func(clientID uint64, asset string, amount int64) bool
	ReleasePerp      func(clientID uint64, asset string, amount int64)
	RecordFeeRevenue func(asset string, takerAmt, makerAmt int64)
	LogBalanceChange func(clientID uint64, symbol, reason string, deltas []BalanceDelta)

	Log       Logger // symbol-scoped logger (may be nil)
	GlobalLog Logger // _global logger (may be nil)

	BasePrecision int64
	Timestamp     int64
	BookSymbol    string
	BookSeqNum    uint64
}

// SettlementResult carries position deltas and realized PnL for fill notifications.
type SettlementResult struct {
	TakerDelta PositionDelta
	MakerDelta PositionDelta
	TakerPnL   int64
	MakerPnL   int64
}

// Settleable is implemented by instruments with custom post-match settlement logic.
// Instruments that do not implement Settleable receive default spot settlement.
type Settleable interface {
	Settle(ctx SettlementContext) SettlementResult
}

// Clock is the time abstraction used throughout the exchange.
type Clock interface {
	NowUnixNano() int64
	NowUnix() int64
}

// Ticker matches the relevant parts of time.Ticker.
type Ticker interface {
	C() <-chan time.Time
	Stop()
}

// TickerFactory creates tickers that work with either real-time or simulation time.
type TickerFactory interface {
	NewTicker(d time.Duration) Ticker
}
