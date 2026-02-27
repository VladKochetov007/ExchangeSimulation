package exchange_test

import (
	. "exchange_sim/exchange"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

type TestLogger struct {
	events []map[string]any
}

func (t *TestLogger) LogEvent(simTime int64, clientID uint64, eventName string, event any) {
	entry := map[string]any{
		"sim_time":  simTime,
		"client_id": clientID,
		"event":     eventName,
	}
	if event != nil {
		eventBytes, _ := json.Marshal(event)
		var eventFields map[string]any
		json.Unmarshal(eventBytes, &eventFields)
		for k, v := range eventFields {
			entry[k] = v
		}
	}
	t.events = append(t.events, entry)
}

// SimulatedClock for testing - allows manual time advancement
type SimulatedClock struct {
	time    int64
	tickers []*simTestTicker
	mu      sync.RWMutex
}

func (c *SimulatedClock) NowUnixNano() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.time
}

func (c *SimulatedClock) NowUnix() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.time / 1_000_000_000
}

func (c *SimulatedClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.time += int64(d)
	// Notify all tickers about time advancement
	for _, ticker := range c.tickers {
		ticker.checkAndFire(c.time)
	}
	c.mu.Unlock()
}

func (c *SimulatedClock) registerTicker(t *simTestTicker) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tickers == nil {
		c.tickers = make([]*simTestTicker, 0)
	}
	c.tickers = append(c.tickers, t)
}

// SimulatedTickerFactory for testing with SimulatedClock
type SimulatedTickerFactory struct {
	clock *SimulatedClock
}

func (f *SimulatedTickerFactory) NewTicker(d time.Duration) Ticker {
	t := &simTestTicker{
		clock:      f.clock,
		interval:   int64(d),
		ch:         make(chan time.Time, 10), // Buffered
		nextFire:   f.clock.NowUnixNano() + int64(d),
	}
	f.clock.registerTicker(t)
	return t
}

type simTestTicker struct {
	clock    *SimulatedClock
	interval int64
	ch       chan time.Time
	nextFire int64
	stopped  bool
}

func (t *simTestTicker) C() <-chan time.Time {
	return t.ch
}

func (t *simTestTicker) Stop() {
	if !t.stopped {
		t.stopped = true
		close(t.ch)
	}
}

func (t *simTestTicker) checkAndFire(now int64) {
	if t.stopped {
		return
	}
	for now >= t.nextFire {
		select {
		case t.ch <- time.Unix(0, now):
		default:
			// Channel full, skip
		}
		t.nextFire += t.interval
	}
}

func TestPeriodicSnapshots(t *testing.T) {
	// Setup exchange with real clock
	clock := &RealClock{}
	ex := NewExchange(10, clock)

	// Add instrument
	inst := NewSpotInstrument("BTCUSD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(inst)

	// Connect client with balances
	balances := map[string]int64{
		"BTC": 100 * BTC_PRECISION,
		"USD": 1000000 * USD_PRECISION,
	}
	gw := ex.ConnectClient(1, balances, &PercentageFee{})

	// Place a buy order to make the snapshot interesting
	gw.RequestCh <- Request{
		Type: ReqPlaceOrder,
		OrderReq: &OrderRequest{
			Symbol:      "BTCUSD",
			Side:        Buy,
			Type:        LimitOrder,
			Price:       PriceUSD(50000, DOLLAR_TICK),
			Qty:         BTCAmount(1),
			TimeInForce: GTC,
			Visibility:  Normal,
		},
	}
	<-gw.ResponseCh

	// Subscribe to market data - this should trigger periodic snapshots
	gw.RequestCh <- Request{
		Type: ReqSubscribe,
		QueryReq: &QueryRequest{
			RequestID: 2,
			Symbol:    "BTCUSD",
		},
	}
	<-gw.ResponseCh

	// Collect snapshots from the market data stream
	snapshots := make([]*BookSnapshot, 0)
	timeout := time.After(350 * time.Millisecond)
	done := false

	for !done {
		select {
		case msg := <-gw.MarketData:
			if msg.Type == MDSnapshot {
				snapshot := msg.Data.(*BookSnapshot)
				snapshots = append(snapshots, snapshot)
			}
		case <-timeout:
			done = true
		}
	}

	ex.Shutdown()

	// Should have received multiple snapshots (initial + periodic)
	// With 100ms interval over 350ms, expect at least 3-4 snapshots
	if len(snapshots) < 3 {
		t.Errorf("expected at least 3 snapshots, got %d", len(snapshots))
	}

	// Verify snapshot content - all should have the bid we placed
	for i, snapshot := range snapshots {
		if len(snapshot.Bids) != 1 {
			t.Errorf("snapshot %d: expected 1 bid level, got %d", i, len(snapshot.Bids))
			continue
		}

		expectedPrice := PriceUSD(50000, DOLLAR_TICK)
		if snapshot.Bids[0].Price != expectedPrice {
			t.Errorf("snapshot %d: expected bid price %d, got %d", i, expectedPrice, snapshot.Bids[0].Price)
		}
	}
}

func TestPeriodicSnapshotsWithSimulatedClock(t *testing.T) {
	// Test that periodic snapshots work with simulation time jumping ahead
	clock := &SimulatedClock{time: 0}
	tickerFactory := &SimulatedTickerFactory{clock: clock}

	ex := NewExchangeWithConfig(ExchangeConfig{
		EstimatedClients: 10,
		Clock:            clock,
		TickerFactory:    tickerFactory,
	})

	inst := NewSpotInstrument("BTCUSD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(inst)

	balances := map[string]int64{
		"BTC": 100 * BTC_PRECISION,
		"USD": 1000000 * USD_PRECISION,
	}
	gw := ex.ConnectClient(1, balances, &PercentageFee{})

	// Subscribe
	gw.RequestCh <- Request{
		Type: ReqSubscribe,
		QueryReq: &QueryRequest{
			RequestID: 1,
			Symbol:    "BTCUSD",
		},
	}
	<-gw.ResponseCh

	// Collect initial snapshot
	initialMsg := <-gw.MarketData
	if initialMsg.Type != MDSnapshot {
		t.Fatalf("Expected initial snapshot, got %v", initialMsg.Type)
	}

	// Advance simulation time by 250ms (should trigger 2 more snapshots)
	clock.Advance(250 * time.Millisecond)

	// Give the loop time to process (real-time)
	time.Sleep(50 * time.Millisecond)

	// Collect snapshots
	snapshots := make([]*BookSnapshot, 0)
	timeout := time.After(10 * time.Millisecond)
	done := false

	for !done {
		select {
		case msg := <-gw.MarketData:
			if msg.Type == MDSnapshot {
				snapshot := msg.Data.(*BookSnapshot)
				snapshots = append(snapshots, snapshot)
			}
		case <-timeout:
			done = true
		}
	}

	ex.Shutdown()

	// Should have received at least 2 snapshots (at t=100ms and t=200ms)
	if len(snapshots) < 2 {
		t.Errorf("expected at least 2 snapshots after 250ms sim time, got %d", len(snapshots))
	}
}

func TestMultipleSubscribersReceiveSnapshots(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(10, clock)

	inst := NewSpotInstrument("BTCUSD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(inst)

	balances := map[string]int64{
		"BTC": 100 * BTC_PRECISION,
		"USD": 1000000 * USD_PRECISION,
	}

	// Connect two clients
	gw1 := ex.ConnectClient(1, balances, &PercentageFee{})
	gw2 := ex.ConnectClient(2, balances, &PercentageFee{})

	// Both subscribe to same symbol
	for _, gw := range []*ClientGateway{gw1, gw2} {
		gw.RequestCh <- Request{
			Type: ReqSubscribe,
			QueryReq: &QueryRequest{
				RequestID: 1,
				Symbol:    "BTCUSD",
			},
		}
		<-gw.ResponseCh
	}

	// Collect snapshots from both clients
	time.Sleep(250 * time.Millisecond)

	count1 := 0
	count2 := 0

	timeout := time.After(10 * time.Millisecond)

	// Drain gw1
	for {
		select {
		case msg := <-gw1.MarketData:
			if msg.Type == MDSnapshot {
				count1++
			}
		case <-timeout:
			goto drainGw2
		}
	}

drainGw2:
	timeout = time.After(10 * time.Millisecond)
	for {
		select {
		case msg := <-gw2.MarketData:
			if msg.Type == MDSnapshot {
				count2++
			}
		case <-timeout:
			goto checkCounts
		}
	}

checkCounts:
	ex.Shutdown()

	// Both clients should have received snapshots
	if count1 < 2 {
		t.Errorf("client 1: expected at least 2 snapshots, got %d", count1)
	}
	if count2 < 2 {
		t.Errorf("client 2: expected at least 2 snapshots, got %d", count2)
	}
}

func TestDeltasInterleavedWithSnapshots(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(10, clock)

	inst := NewSpotInstrument("BTCUSD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(inst)

	balances := map[string]int64{
		"BTC": 100 * BTC_PRECISION,
		"USD": 1000000 * USD_PRECISION,
	}

	gw := ex.ConnectClient(1, balances, &PercentageFee{})

	// Subscribe
	gw.RequestCh <- Request{
		Type: ReqSubscribe,
		QueryReq: &QueryRequest{
			RequestID: 1,
			Symbol:    "BTCUSD",
		},
	}
	<-gw.ResponseCh

	// Wait a bit
	time.Sleep(50 * time.Millisecond)

	// Place an order - should trigger delta
	gw.RequestCh <- Request{
		Type: ReqPlaceOrder,
		OrderReq: &OrderRequest{
			Symbol:      "BTCUSD",
			Side:        Buy,
			Type:        LimitOrder,
			Price:       PriceUSD(50000, DOLLAR_TICK),
			Qty:         BTCAmount(1),
			TimeInForce: GTC,
			Visibility:  Normal,
		},
	}
	<-gw.ResponseCh

	// Collect messages for 250ms
	time.Sleep(250 * time.Millisecond)

	snapshotCount := 0
	deltaCount := 0

	timeout := time.After(10 * time.Millisecond)
	for {
		select {
		case msg := <-gw.MarketData:
			switch msg.Type {
			case MDSnapshot:
				snapshotCount++
			case MDDelta:
				deltaCount++
			}
		case <-timeout:
			goto checkResults
		}
	}

checkResults:
	ex.Shutdown()

	// Should have received both snapshots and deltas
	if snapshotCount < 2 {
		t.Errorf("expected at least 2 snapshots, got %d", snapshotCount)
	}
	if deltaCount < 1 {
		t.Errorf("expected at least 1 delta (from order placement), got %d", deltaCount)
	}
}
