package types

import "time"

type PriceOracle interface {
	GetPrice(symbol string) int64
}

// Logger is the event logging interface for the exchange.
type Logger interface {
	LogEvent(simTime int64, clientID uint64, eventName string, event any)
}

// FeeModel calculates trading fees for each execution.
type FeeModel interface {
	CalculateFee(exec *Execution, side Side, isMaker bool, baseAsset, quoteAsset string, precision int64) Fee
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
