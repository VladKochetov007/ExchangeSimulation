package multivenue

import (
	"context"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/simulation"
)

// crossingActor rests a quote on its first tick and then crosses it from a
// second account, which is the minimal version of a market maker hedging into
// another book.
type crossingActor struct {
	*actor.BaseActor
	symbol   string
	side     exchange.Side
	price    int64
	qty      int64
	resting  bool
	fills    int64
	accepted int
	rejected int
	ticks    int
	crossAt  int
}

func (c *crossingActor) HandleEvent(_ context.Context, evt *actor.Event) {
	switch evt.Type {
	case actor.EventOrderAccepted:
		c.accepted++
	case actor.EventOrderRejected:
		c.rejected++
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		c.fills += evt.Data.(actor.OrderFillEvent).Qty
	}
}

func (c *crossingActor) onTick(time.Time) {
	c.ticks++
	if c.resting {
		if c.ticks == 1 {
			c.SubmitOrder(c.symbol, c.side, exchange.LimitOrder, c.price, c.qty)
		}
		return
	}
	if c.ticks == c.crossAt {
		c.SubmitOrderWithTimeInForce(c.symbol, c.side, exchange.LimitOrder, c.price, c.qty, exchange.IOC)
	}
}

// A marketable order sent through the deterministic phase runtime must match
// resting liquidity. FFA-28 observed a market maker's hedge accepted and never
// matched while priced 5.2% through the visible touch, so this pins down
// whether the runtime itself can carry a crossing order.
func TestPhaseRuntimeMatchesACrossingOrder(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano()
	clock := simulation.NewSimulatedClock(start)
	scheduler := simulation.NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	timers := simulation.NewSimTimerFactory(scheduler)
	runner := simulation.NewRunner(clock, simulation.RunnerConfig{
		Iterations: 20, Step: time.Second, Quiesce: true, DeterministicPhases: true,
	})
	runner.AddIdler(timers)

	ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		ID: "probe", EstimatedClients: 4, Clock: clock, TickerFactory: timers,
		DeterministicIngress: true, DeterministicPhases: true,
		SnapshotInterval: time.Second,
	})
	ex.AddInstrument(exchange.NewPerpFutures("ABC-PERP", "ABC", "USD", mvBasePrecision, mvQuotePrecision, int64(10*mvQuotePrecision), mvBasePrecision/1_000))
	mount := simulation.NewMount(ex, simulation.LatencyConfig{})
	runner.AddMount(mount)

	connect := func(id uint64) actor.Gateway {
		gw := mount.ConnectNewClient(id, map[string]int64{"USD": 100_000_000 * mvQuotePrecision}, &exchange.FixedFee{})
		ex.AddPerpBalance(id, "USD", 100_000_000*mvQuotePrecision)
		return gw
	}

	price := int64(50_000) * mvQuotePrecision
	maker := &crossingActor{symbol: "ABC-PERP", side: exchange.Sell, price: price, qty: mvBasePrecision / 5, resting: true}
	maker.BaseActor = actor.NewBaseActor(1, connect(1))
	maker.SetHandler(maker)
	maker.AddTicker(time.Second, maker.onTick)
	maker.SetTickerFactory(timers)

	// Buy well through the resting ask, exactly as a hedge does.
	taker := &crossingActor{symbol: "ABC-PERP", side: exchange.Buy, price: price + 100*mvQuotePrecision, qty: mvBasePrecision / 10, crossAt: 5}
	taker.BaseActor = actor.NewBaseActor(2, connect(2))
	taker.SetHandler(taker)
	taker.AddTicker(time.Second, taker.onTick)
	taker.SetTickerFactory(timers)

	runner.AddActor(maker)
	runner.AddActor(taker)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	t.Logf("taker accepted=%d rejected=%d fills=%d; maker fills=%d", taker.accepted, taker.rejected, taker.fills, maker.fills)
	if taker.fills == 0 {
		t.Fatalf("a crossing order priced through the touch did not match under the phase runtime")
	}
	if taker.fills != mvBasePrecision/10 {
		t.Fatalf("crossing order filled %d, want %d", taker.fills, mvBasePrecision/10)
	}
}
