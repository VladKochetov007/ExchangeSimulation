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
	ConnectNewClient(clientID uint64, balances map[string]int64, feePlan FeeModel) Gateway
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

// MarginLedger is optionally implemented by PositionStore backends to track
// position margin exactly. When absent, settlement falls back to recomputing
// margin from entry price, which can leave rounding dust in reservations.
type MarginLedger interface {
	// AddPositionMargin increases the tracked margin for a position (on open).
	AddPositionMargin(clientID uint64, symbol string, side PositionSide, amount int64)
	// ReleasePositionMargin removes and returns the margin share for closing
	// closedQty out of a position previously sized oldSize. A full close
	// returns the entire remainder, so nothing accrues across the lifecycle.
	ReleasePositionMargin(clientID uint64, symbol string, side PositionSide, closedQty, oldSize int64) int64
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

// FeeModel calculates trading fees for each execution. Implementations must be
// pure and deterministic for an identical FillContext: the exchange may quote
// a cloned execution before matching and settle using that frozen quote.
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

// OrderMarginer is implemented by instruments whose order reservation depends
// on the order side (e.g. options: buyers pay the premium in full, sellers
// post a margin formula). Reservations are taken from the perp wallet in the
// quote asset. Checked before Margined.
type OrderMarginer interface {
	MarginForOrder(side Side, qty, price, precision int64) int64
	MarginForMarketOrder(side Side, qty, refPrice, precision int64) int64
}

// PositionMarginer is implemented by non-perp instruments whose OPEN POSITIONS
// contribute mark-to-market and maintenance margin to the cross-margin account
// profile (e.g. options: a short must post maintenance against the underlying,
// and premium marks move account equity). Without it a position is invisible
// to the risk engine and can never trigger liquidation.
type PositionMarginer interface {
	// PositionMark is the price positions are marked at for unrealized PnL
	// (zero when no mark has been set yet — the position then marks neutral).
	PositionMark() int64
	// MaintenanceForPosition returns the quote-asset maintenance requirement
	// for a signed position of the given size.
	MaintenanceForPosition(size, precision int64) int64
}

// Expirable is implemented by instruments with a finite life (dated futures,
// options). The exchange's automation observes settlement inputs while the
// instrument trades and cash-settles all positions at expiry.
type Expirable interface {
	// ExpiryNano is the expiry timestamp in unix nanoseconds.
	ExpiryNano() int64
	// ObserveSettlement records an underlying index/mark sample used to build
	// the settlement price (venues use a TWAP window before expiry).
	ObserveSettlement(price, tsNano int64)
	// SettlementPrice returns the final settlement price once expired.
	SettlementPrice() int64
	// ExpiryCashFlow returns the signed quote-asset cash delta credited to a
	// position of size (signed, base units) entered at entryPrice when the
	// instrument settles at settlementPrice.
	ExpiryCashFlow(size, entryPrice, settlementPrice, basePrecision int64) int64
	// DeliveryFee returns the quote-asset fee charged per position at expiry
	// (0 for none). size is signed; fee must be non-negative.
	DeliveryFee(size, settlementPrice, basePrecision int64) int64
}

// UnderlyingRef is implemented by derivatives that reference another listed
// symbol (used by the exchange to source settlement observations and marks).
type UnderlyingRef interface {
	UnderlyingSymbol() string
}

// ListingPolicy generates new instruments on a schedule (an exchange listing
// calendar: dated futures tenors, option chains around spot). The exchange
// polls policies from its automation loop and lists whatever is returned.
// Implementations must self-deduplicate (return each instrument once).
type ListingPolicy interface {
	PendingListings(nowNano int64, prices PriceSource) []Instrument
}

// SettlementContext carries all state an instrument needs to settle one execution.
// Account mutation callbacks are closures that capture the exchange's internal client map,
// so the instrument never needs to import the exchange package.
type SettlementContext struct {
	Exec         *Execution
	TakerOrder   *Order
	MakerOrder   *Order       // nil if the matcher removed the filled maker from the book index
	MakerPosSide PositionSide // resolved by the exchange before calling Settle
	TakerFee     Fee
	MakerFee     Fee
	Positions    PositionStore

	// Account mutation callbacks.
	PerpBalance       func(clientID uint64, asset string) int64
	MutatePerpBalance func(clientID uint64, asset string, delta int64)
	// ReservePerp earmarks post-trade margin. The exchange-provided closure
	// force-reserves: the fill already happened, so the margin is owed even if
	// it pushes available below zero (the liquidation sweep resolves shortfalls).
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
