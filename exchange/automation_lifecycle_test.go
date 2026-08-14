package exchange

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"
)

type recordingTickerFactory struct {
	mu        sync.Mutex
	intervals []time.Duration
}

func (f *recordingTickerFactory) NewTicker(interval time.Duration) Ticker {
	f.mu.Lock()
	f.intervals = append(f.intervals, interval)
	f.mu.Unlock()
	return &recordingTicker{ch: make(chan time.Time)}
}

func (f *recordingTickerFactory) snapshot() []time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]time.Duration(nil), f.intervals...)
}

type recordingTicker struct{ ch chan time.Time }

func (t *recordingTicker) C() <-chan time.Time { return t.ch }
func (t *recordingTicker) Stop()               {}

func TestStartAutomationRegistersTickersSynchronouslyInFixedOrder(t *testing.T) {
	factory := &recordingTickerFactory{}
	ex := NewExchangeWithConfig(ExchangeConfig{Clock: &RealClock{}, TickerFactory: factory})
	ex.ConfigureAutomation(AutomationConfig{PriceUpdateInterval: 3 * time.Second})
	ex.StartAutomation(context.Background())
	defer ex.StopAutomation()

	if got, want := factory.snapshot(), []time.Duration{3 * time.Second, time.Second, time.Minute, time.Second}; !slices.Equal(got, want) {
		t.Fatalf("automation ticker registration = %v, want %v", got, want)
	}
}
