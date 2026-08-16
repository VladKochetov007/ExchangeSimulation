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
	symbol    string
	side      exchange.Side
	price     int64
	qty       int64
	resting   bool
	requote   bool
	restingID uint64
	fills     int64
	accepted  int
	rejected  int
	ticks     int
	crossAt   int
}

func (c *crossingActor) HandleEvent(_ context.Context, evt *actor.Event) {
	switch evt.Type {
	case actor.EventOrderAccepted:
		c.accepted++
		c.restingID = evt.Data.(actor.OrderAcceptedEvent).OrderID
	case actor.EventOrderRejected:
		c.rejected++
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		c.fills += evt.Data.(actor.OrderFillEvent).Qty
	}
}

func (c *crossingActor) onTick(time.Time) {
	c.ticks++
	if c.resting {
		if c.requote {
			// Cancel and replace every tick, the way a market maker refreshes
			// its quote.
			if c.restingID != 0 {
				c.CancelOrder(c.restingID)
				c.restingID = 0
			}
			c.SubmitOrder(c.symbol, c.side, exchange.LimitOrder, c.price, c.qty)
			return
		}
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

// The real market maker, with hedging configured, must offset an inventory it
// already holds against a perpetual book that has liquidity. This is the next
// step out from the minimal crossing test toward the full scenario.
func TestMarketMakerHedgesAgainstAvailableLiquidity(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano()
	clock := simulation.NewSimulatedClock(start)
	scheduler := simulation.NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	timers := simulation.NewSimTimerFactory(scheduler)
	runner := simulation.NewRunner(clock, simulation.RunnerConfig{
		Iterations: 30, Step: time.Second, Quiesce: true, DeterministicPhases: true,
	})
	runner.AddIdler(timers)

	ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		ID: "probe", EstimatedClients: 4, Clock: clock, TickerFactory: timers,
		DeterministicIngress: true, DeterministicPhases: true, SnapshotInterval: time.Second,
	})
	tick := int64(10 * mvQuotePrecision)
	ex.AddInstrument(exchange.NewSpotInstrument("ABC/USD", "ABC", "USD", mvBasePrecision, mvQuotePrecision, tick, mvBasePrecision/1_000))
	ex.AddInstrument(exchange.NewPerpFutures("ABC-PERP", "ABC", "USD", mvBasePrecision, mvQuotePrecision, tick, mvBasePrecision/1_000))
	mount := simulation.NewMount(ex, simulation.LatencyConfig{})
	runner.AddMount(mount)

	connect := func(id uint64) actor.Gateway {
		gw := mount.ConnectNewClient(id, map[string]int64{
			"ABC": 10_000 * mvBasePrecision, "USD": 100_000_000 * mvQuotePrecision,
		}, &exchange.FixedFee{})
		ex.AddPerpBalance(id, "USD", 100_000_000*mvQuotePrecision)
		return gw
	}

	price := int64(50_000) * mvQuotePrecision
	maker := NewStoikovMarketMaker(1, connect(1), StoikovMMConfig{
		Symbol: "ABC/USD", ReferenceSymbol: "ABC/USD", BootstrapPrice: price,
		BasePrecision: mvBasePrecision, QuotePrecision: mvQuotePrecision, TickSize: tick,
		QuoteQty: mvBasePrecision / 5, QuoteInterval: time.Second,
		VolatilityHalfLife: time.Minute, InitialLogVariancePerSec: 1e-8,
		InventoryHorizon: 10 * time.Minute, RelativeRiskAversion: 0.1, RelativeFillDecay: 25_000,
		MinHalfSpreadTicks: 1, InventoryLimit: 100 * mvBasePrecision,
		HedgeSymbol: "ABC-PERP", HedgeBandQty: mvBasePrecision / 10, HedgeSlippageBps: 50,
	})
	maker.SetTickerFactory(timers)
	// The maker is already short spot, which is what it must offset.
	maker.inventory = -2 * mvBasePrecision

	perpLiquidity := &crossingActor{symbol: "ABC-PERP", side: exchange.Sell, price: price, qty: 5 * mvBasePrecision, resting: true}
	perpLiquidity.BaseActor = actor.NewBaseActor(2, connect(2))
	perpLiquidity.SetHandler(perpLiquidity)
	perpLiquidity.AddTicker(time.Second, perpLiquidity.onTick)
	perpLiquidity.SetTickerFactory(timers)

	runner.AddActor(maker)
	runner.AddActor(perpLiquidity)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	t.Logf("hedge attempts=%d fills=%d position=%d; hedge book seen=%d two-sided=%d",
		maker.hedgeAttempts, maker.hedgeFills, maker.hedgePosition, maker.hedgeBookSeen, maker.hedgeBookTwoSided)
	if maker.hedgeAttempts == 0 {
		t.Fatal("maker never attempted to hedge a two-unit short")
	}
	if maker.hedgePosition == 0 {
		t.Fatal("maker hedged nothing despite five units resting on the perpetual")
	}
}

// A maker that refreshes its quote every tick must still be reachable by a
// participant whose orders are drained earlier in the same phase. Deterministic
// ingress processes clients in ascending order, so a hedger with a lower client
// identifier arrives after the quote was cancelled and before its replacement.
func TestHedgeReachesAQuoteThatRefreshesEveryTick(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano()
	clock := simulation.NewSimulatedClock(start)
	scheduler := simulation.NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	timers := simulation.NewSimTimerFactory(scheduler)
	runner := simulation.NewRunner(clock, simulation.RunnerConfig{
		Iterations: 30, Step: time.Second, Quiesce: true, DeterministicPhases: true,
	})
	runner.AddIdler(timers)

	ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		ID: "probe", EstimatedClients: 4, Clock: clock, TickerFactory: timers,
		DeterministicIngress: true, DeterministicPhases: true, SnapshotInterval: time.Second,
	})
	tick := int64(10 * mvQuotePrecision)
	ex.AddInstrument(exchange.NewSpotInstrument("ABC/USD", "ABC", "USD", mvBasePrecision, mvQuotePrecision, tick, mvBasePrecision/1_000))
	ex.AddInstrument(exchange.NewPerpFutures("ABC-PERP", "ABC", "USD", mvBasePrecision, mvQuotePrecision, tick, mvBasePrecision/1_000))
	mount := simulation.NewMount(ex, simulation.LatencyConfig{})
	runner.AddMount(mount)

	connect := func(id uint64) actor.Gateway {
		gw := mount.ConnectNewClient(id, map[string]int64{
			"ABC": 10_000 * mvBasePrecision, "USD": 100_000_000 * mvQuotePrecision,
		}, &exchange.FixedFee{})
		ex.AddPerpBalance(id, "USD", 100_000_000*mvQuotePrecision)
		return gw
	}

	price := int64(50_000) * mvQuotePrecision
	maker := NewStoikovMarketMaker(1, connect(1), StoikovMMConfig{
		Symbol: "ABC/USD", ReferenceSymbol: "ABC/USD", BootstrapPrice: price,
		BasePrecision: mvBasePrecision, QuotePrecision: mvQuotePrecision, TickSize: tick,
		QuoteQty: mvBasePrecision / 5, QuoteInterval: time.Second,
		VolatilityHalfLife: time.Minute, InitialLogVariancePerSec: 1e-8,
		InventoryHorizon: 10 * time.Minute, RelativeRiskAversion: 0.1, RelativeFillDecay: 25_000,
		MinHalfSpreadTicks: 1, InventoryLimit: 100 * mvBasePrecision,
		HedgeSymbol: "ABC-PERP", HedgeBandQty: mvBasePrecision / 10, HedgeSlippageBps: 50,
	})
	maker.SetTickerFactory(timers)
	maker.inventory = -2 * mvBasePrecision

	// Higher client identifier, and it refreshes its quote every tick.
	perpMaker := &crossingActor{symbol: "ABC-PERP", side: exchange.Sell, price: price, qty: 5 * mvBasePrecision, resting: true, requote: true}
	perpMaker.BaseActor = actor.NewBaseActor(2, connect(7))
	perpMaker.SetHandler(perpMaker)
	perpMaker.AddTicker(time.Second, perpMaker.onTick)
	perpMaker.SetTickerFactory(timers)

	runner.AddActor(maker)
	runner.AddActor(perpMaker)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	t.Logf("hedge attempts=%d fills=%d position=%d", maker.hedgeAttempts, maker.hedgeFills, maker.hedgePosition)
	if maker.hedgeAttempts == 0 {
		t.Fatal("maker never attempted to hedge")
	}
	if maker.hedgePosition == 0 {
		t.Fatalf("maker hedged nothing across %d attempts against a continuously refreshed quote", maker.hedgeAttempts)
	}
}

// Characterisation: the perpetual currently cannot price away from spot, so
// the market has no basis.
//
// Both makers anchor to the same published index, and neither prices its
// inventory into its quote (FFA-16), so they produce the same quote and the
// two midpoints coincide to the unit. The consequence is measured elsewhere:
// carry arbitrageurs placed no order at any entry threshold down to one basis
// point, because there was never a basis to trade.
//
// This test asserts the limitation rather than the desired behaviour, so that
// it fails and must be revisited once makers price inventory. Publishing an
// index for the perpetual and giving it its own reference book, both done
// here, are necessary but not sufficient.
func TestPerpetualCurrentlyMirrorsSpot(t *testing.T) {
	sim, err := NewSim(30*time.Minute, Config{
		LogDir: t.TempDir(), LogMode: "none", Seed: 91,
		MakerAnchor: "fundamental", CarryArbitrageurCount: 2, CarryEntryBps: 1, CarryExitBps: 1,
	})
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	defer sim.Close()
	if err := sim.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	identical, observed, attempts := 0, 0, 0
	for _, venue := range sim.Venues {
		for _, arb := range venue.CarryArbs {
			attempts += arb.attempts
			spot, perp := arb.spot.mid(), arb.perp.mid()
			if spot <= 0 || perp <= 0 {
				continue
			}
			observed++
			if spot == perp {
				identical++
			}
		}
	}
	if observed == 0 {
		t.Fatal("no carry participant ever saw both books")
	}
	if identical != observed {
		t.Fatalf("the perpetual has started pricing away from spot (%d of %d midpoints differ): "+
			"a basis now exists, so this characterisation is stale and the carry participant should be re-measured",
			observed-identical, observed)
	}
	if attempts != 0 {
		t.Fatalf("carry participants traded %d times without a basis, which contradicts the measurement", attempts)
	}
}
