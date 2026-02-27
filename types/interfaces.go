package types

import "time"

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
