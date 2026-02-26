package clock

import (
	"time"

	etypes "exchange_sim/types"
)

type RealClock struct{}

func (c *RealClock) NowUnixNano() int64 { return time.Now().UnixNano() }
func (c *RealClock) NowUnix() int64     { return time.Now().Unix() }

// RealTickerFactory creates real-time tickers for production use.
type RealTickerFactory struct{}

func (f *RealTickerFactory) NewTicker(d time.Duration) etypes.Ticker {
	return &realTicker{ticker: time.NewTicker(d)}
}

type realTicker struct {
	ticker *time.Ticker
}

func (t *realTicker) C() <-chan time.Time { return t.ticker.C }
func (t *realTicker) Stop()               { t.ticker.Stop() }
