package simulation

import (
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// boundaryGateway is deliberately only a message source. The test exercises
// the courier boundary itself, where an accidental negative latency would make
// a market observation available to an actor before its publication instant.
type boundaryGateway struct {
	id   uint64
	resp chan exchange.Response
	md   chan *exchange.MarketDataMsg
}

var _ actor.Gateway = (*boundaryGateway)(nil)

func (g *boundaryGateway) ID() uint64                                   { return g.id }
func (g *boundaryGateway) Send(exchange.Request)                        {}
func (g *boundaryGateway) Responses() <-chan exchange.Response          { return g.resp }
func (g *boundaryGateway) MarketDataCh() <-chan *exchange.MarketDataMsg { return g.md }
func (g *boundaryGateway) IsRunning() bool                              { return true }

// TestMarketDataCannotArriveBeforePublicationPlusLatency is a direct
// information-boundary invariant. It does not trust a later actor decision:
// the observed message must be absent before its publication time plus the
// modeled courier latency, then appear at that exact delivery boundary.
func TestMarketDataCannotArriveBeforePublicationPlusLatency(t *testing.T) {
	const delay = 10 * time.Millisecond
	clock := NewSimulatedClock(int64(time.Second))
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	stats := NewLatencyStats()
	inner := &boundaryGateway{
		id: 7, resp: make(chan exchange.Response, 1), md: make(chan *exchange.MarketDataMsg, 1),
	}
	delayed := NewDelayedGateway(inner, nil, nil, NewConstantLatency(delay))
	delayed.UseScheduler(scheduler, clock)
	delayed.SetLatencyTelemetry(stats, "boundary-test")
	if err := delayed.EnableDeterministicPhases(); err != nil {
		t.Fatal(err)
	}
	delayed.Start()
	defer delayed.Stop()

	publishedAt := clock.NowUnixNano()
	inner.md <- &exchange.MarketDataMsg{Symbol: "ABC-USD", Timestamp: publishedAt}
	if !delayed.PumpDeterministicPhase() {
		t.Fatal("market-data publication was not scheduled")
	}
	if delayed.DrainDeterministicPhaseEgress() {
		t.Fatal("market data reached the actor at its publication instant")
	}
	select {
	case got := <-delayed.MarketDataCh():
		t.Fatalf("future information delivered at %d: %+v", clock.NowUnixNano(), got)
	default:
	}

	clock.Advance(delay - time.Nanosecond)
	if delayed.DrainDeterministicPhaseEgress() {
		t.Fatal("market data reached the actor before publication plus latency")
	}
	clock.Advance(time.Nanosecond)
	if !delayed.DrainDeterministicPhaseEgress() {
		t.Fatal("market data was not delivered at publication plus latency")
	}
	select {
	case got := <-delayed.MarketDataCh():
		if got == nil || clock.NowUnixNano() < publishedAt+delay.Nanoseconds() {
			t.Fatalf("invalid delayed delivery at %d: %+v", clock.NowUnixNano(), got)
		}
	default:
		t.Fatal("actor inbox missing due market data")
	}

	rows := stats.Summary().Rows
	if len(rows) != 1 || rows[0].Scheduled != 1 || rows[0].Delivered != 1 || rows[0].MeanDeliveryNanoseconds < float64(delay) {
		t.Fatalf("latency evidence does not support the boundary: %+v", rows)
	}
}

// Zero delay is still an explicit modeled courier path when a link is
// configured with a latency provider. The telemetry must retain that observed
// zero rather than silently dropping the link and making an accidental direct
// connection indistinguishable from missing evidence.
func TestScheduledZeroLatencyProducesZeroTelemetry(t *testing.T) {
	clock := NewSimulatedClock(int64(time.Second))
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	stats := NewLatencyStats()
	inner := &boundaryGateway{
		id: 8, resp: make(chan exchange.Response, 1), md: make(chan *exchange.MarketDataMsg, 1),
	}
	delayed := NewDelayedGateway(inner, nil, nil, NewConstantLatency(0))
	delayed.UseScheduler(scheduler, clock)
	delayed.SetLatencyTelemetry(stats, "zero-boundary-test")
	if err := delayed.EnableDeterministicPhases(); err != nil {
		t.Fatal(err)
	}
	delayed.Start()
	defer delayed.Stop()

	inner.md <- &exchange.MarketDataMsg{Symbol: "ABC-USD", Timestamp: clock.NowUnixNano()}
	if !delayed.PumpDeterministicPhase() {
		t.Fatal("zero-delay market data was not scheduled")
	}
	if !delayed.DrainDeterministicPhaseEgress() {
		t.Fatal("zero-delay market data was not delivered at publication")
	}
	select {
	case <-delayed.MarketDataCh():
	default:
		t.Fatal("actor inbox missing zero-delay market data")
	}

	rows := stats.Summary().Rows
	if len(rows) != 1 || rows[0].Scheduled != 1 || rows[0].Delivered != 1 ||
		rows[0].MeanDrawnNanoseconds != 0 || rows[0].MeanDeliveryNanoseconds != 0 {
		t.Fatalf("zero-delay telemetry is incomplete or nonzero: %+v", rows)
	}
}
