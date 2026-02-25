package exchange

import "time"

type Clock interface {
	NowUnixNano() int64
	NowUnix() int64
}

// Ticker interface matches the relevant parts of time.Ticker
type Ticker interface {
	C() <-chan time.Time
	Stop()
}

// TickerFactory creates tickers that work with either real-time or simulation time
type TickerFactory interface {
	NewTicker(d time.Duration) Ticker
}

type RealClock struct{}

func (c *RealClock) NowUnixNano() int64 { return time.Now().UnixNano() }
func (c *RealClock) NowUnix() int64     { return time.Now().Unix() }

// RealTickerFactory creates real-time tickers for production use
type RealTickerFactory struct{}

func (f *RealTickerFactory) NewTicker(d time.Duration) Ticker {
	return &realTicker{ticker: time.NewTicker(d)}
}

type realTicker struct {
	ticker *time.Ticker
}

func (t *realTicker) C() <-chan time.Time { return t.ticker.C }
func (t *realTicker) Stop()               { t.ticker.Stop() }
