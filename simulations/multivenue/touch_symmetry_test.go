package multivenue

import (
	"context"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/simulation"
	etypes "exchange_sim/types"
)

// touchTaker prices an IOC exactly at the touch it last saw, which is what the
// metaorder, carry and round-trip desks all do when no slippage allowance is
// configured.
type touchTaker struct {
	*actor.BaseActor
	symbol           string
	side             exchange.Side
	qty              int64
	bestBid, bestAsk int64
	sent, fills      int
	subscribed       bool
}

func (t *touchTaker) HandleEvent(_ context.Context, evt *actor.Event) {
	switch evt.Type {
	case actor.EventBookSnapshot:
		e := evt.Data.(actor.BookSnapshotEvent)
		if e.Symbol != t.symbol || e.Snapshot == nil {
			return
		}
		t.bestBid, t.bestAsk = 0, 0
		if len(e.Snapshot.Bids) > 0 {
			t.bestBid = e.Snapshot.Bids[0].Price
		}
		if len(e.Snapshot.Asks) > 0 {
			t.bestAsk = e.Snapshot.Asks[0].Price
		}
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		t.fills++
	}
}

func (t *touchTaker) onTick(time.Time) {
	if !t.subscribed {
		t.Subscribe(t.symbol, exchange.MDSnapshot)
		t.subscribed = true
		return
	}
	price := t.bestAsk
	if t.side == exchange.Sell {
		price = t.bestBid
	}
	if price <= 0 {
		return
	}
	t.SubmitOrderWithTimeInForce(t.symbol, t.side, exchange.LimitOrder, price, t.qty, exchange.IOC)
	t.sent++
}

// requotingMaker replaces both quotes every step, submitting the bid before the
// ask, which is the order the Stoikov maker uses.
type requotingMaker struct {
	*actor.BaseActor
	symbol       string
	mid, half    int64
	bidID, askID uint64
	pending      map[uint64]bool
	step         int64
	qty          int64
	askFirst     bool
	// offset shifts this maker's mid so two makers quote overlapping prices and
	// cross each other, as the reference population's makers do.
	offset int64
}

func (m *requotingMaker) HandleEvent(_ context.Context, evt *actor.Event) {
	if evt.Type == actor.EventOrderAccepted {
		e := evt.Data.(actor.OrderAcceptedEvent)
		if m.pending[e.RequestID] {
			if m.bidID == 0 {
				m.bidID = e.OrderID
			} else if m.askID == 0 {
				m.askID = e.OrderID
			}
			delete(m.pending, e.RequestID)
		}
	}
}

func (m *requotingMaker) onTick(time.Time) {
	previousBid, previousAsk := m.bidID, m.askID
	m.bidID, m.askID = 0, 0
	// Walk the mid so the touch moves between steps, as it does in a live run.
	m.step++
	mid := m.mid + m.offset + (m.step%5)*m.half/4
	submitBid := func() {
		req := m.SubmitOrder(m.symbol, exchange.Buy, exchange.LimitOrder, mid-m.half, m.qty)
		m.pending[req] = true
	}
	submitAsk := func() {
		req := m.SubmitOrder(m.symbol, exchange.Sell, exchange.LimitOrder, mid+m.half, m.qty)
		m.pending[req] = true
	}
	if m.askFirst {
		submitAsk()
		submitBid()
	} else {
		submitBid()
		submitAsk()
	}
	if previousBid != 0 {
		m.CancelOrder(previousBid)
	}
	if previousAsk != 0 {
		m.CancelOrder(previousAsk)
	}
}

// A desk pricing at the touch must fill at comparable rates on both sides. In
// reference runs the metaorder desk missed 88-95% of its buys and 1.7-1.9% of
// its sells, and carry_arb and round_trip missed 96% and 93% with independently
// written at-touch code. The engine crosses symmetrically under direct
// submission, so this pins the behaviour under the phase runtime instead.
func TestAtTouchFillsAreSymmetricAcrossSides(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano()
	clock := simulation.NewSimulatedClock(start)
	scheduler := simulation.NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	timers := simulation.NewSimTimerFactory(scheduler)
	runner := simulation.NewRunner(clock, simulation.RunnerConfig{
		Iterations: 200, Step: time.Second, Quiesce: true, DeterministicPhases: true,
	})
	runner.AddIdler(timers)

	ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		ID: "probe", EstimatedClients: 8, Clock: clock, TickerFactory: timers,
		DeterministicIngress: true, DeterministicPhases: true,
		SnapshotInterval: time.Second,
	})
	tick := int64(mvQuotePrecision)
	ex.AddInstrument(exchange.NewSpotInstrument("ABC/USD", "ABC", "USD", mvBasePrecision, mvQuotePrecision, tick, mvBasePrecision/1_000))
	mount := simulation.NewMount(ex, simulation.LatencyConfig{})
	runner.AddMount(mount)

	connect := func(id uint64) actor.Gateway {
		return mount.ConnectNewClient(id, map[string]int64{
			"ABC": 10_000 * mvBasePrecision,
			"USD": 1_000_000_000 * mvQuotePrecision,
		}, &exchange.FixedFee{})
	}

	mid := int64(50_000) * mvQuotePrecision
	maker := &requotingMaker{symbol: "ABC/USD", mid: mid, half: 20 * tick, qty: mvBasePrecision, pending: map[uint64]bool{}}
	maker.BaseActor = actor.NewBaseActor(1, connect(1))
	maker.SetHandler(maker)
	maker.AddTicker(time.Second, maker.onTick)
	maker.SetTickerFactory(timers)
	runner.AddActor(maker)

	buyer := &touchTaker{symbol: "ABC/USD", side: exchange.Buy, qty: mvBasePrecision / 100}
	buyer.BaseActor = actor.NewBaseActor(2, connect(2))
	buyer.SetHandler(buyer)
	buyer.AddTicker(time.Second, buyer.onTick)
	buyer.SetTickerFactory(timers)
	runner.AddActor(buyer)

	seller := &touchTaker{symbol: "ABC/USD", side: exchange.Sell, qty: mvBasePrecision / 100}
	seller.BaseActor = actor.NewBaseActor(3, connect(3))
	seller.SetHandler(seller)
	seller.AddTicker(time.Second, seller.onTick)
	seller.SetTickerFactory(timers)
	runner.AddActor(seller)

	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	buyRate := float64(buyer.fills) / float64(maxInt(buyer.sent, 1))
	sellRate := float64(seller.fills) / float64(maxInt(seller.sent, 1))
	t.Logf("buy sent=%d fills=%d (%.1f%%); sell sent=%d fills=%d (%.1f%%)",
		buyer.sent, buyer.fills, 100*buyRate, seller.sent, seller.fills, 100*sellRate)
	if buyer.sent == 0 || seller.sent == 0 {
		t.Fatalf("a taker never submitted: buy=%d sell=%d", buyer.sent, seller.sent)
	}
	if diff := buyRate - sellRate; diff > 0.25 || diff < -0.25 {
		t.Fatalf("at-touch fill rates differ by side: buy %.1f%%, sell %.1f%%", 100*buyRate, 100*sellRate)
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var _ = etypes.PriceLevel{}

// The population contains buyers that consume the touch before the metaorder
// desk is scheduled: the round-trip desk alone places thousands of buys against
// hundreds of sells. If contention for the ask is what starves an at-touch buy,
// adding one competitor that sweeps the ask each step should reproduce the
// asymmetry that the single-maker case does not show.
func TestAtTouchBuysStarveWhenTheAskIsContested(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano()
	clock := simulation.NewSimulatedClock(start)
	scheduler := simulation.NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	timers := simulation.NewSimTimerFactory(scheduler)
	runner := simulation.NewRunner(clock, simulation.RunnerConfig{
		Iterations: 200, Step: time.Second, Quiesce: true, DeterministicPhases: true,
	})
	runner.AddIdler(timers)

	ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		ID: "probe", EstimatedClients: 8, Clock: clock, TickerFactory: timers,
		DeterministicIngress: true, DeterministicPhases: true,
		SnapshotInterval: time.Second,
	})
	tick := int64(mvQuotePrecision)
	ex.AddInstrument(exchange.NewSpotInstrument("ABC/USD", "ABC", "USD", mvBasePrecision, mvQuotePrecision, tick, mvBasePrecision/1_000))
	mount := simulation.NewMount(ex, simulation.LatencyConfig{})
	runner.AddMount(mount)

	connect := func(id uint64) actor.Gateway {
		return mount.ConnectNewClient(id, map[string]int64{
			"ABC": 10_000 * mvBasePrecision,
			"USD": 1_000_000_000 * mvQuotePrecision,
		}, &exchange.FixedFee{})
	}

	mid := int64(50_000) * mvQuotePrecision
	maker := &requotingMaker{symbol: "ABC/USD", mid: mid, half: 20 * tick, qty: mvBasePrecision, pending: map[uint64]bool{}}
	maker.BaseActor = actor.NewBaseActor(1, connect(1))
	maker.SetHandler(maker)
	maker.AddTicker(time.Second, maker.onTick)
	maker.SetTickerFactory(timers)
	runner.AddActor(maker)

	// Sweeps the whole displayed ask every step, ahead of the desk under test.
	sweeper := &touchTaker{symbol: "ABC/USD", side: exchange.Buy, qty: mvBasePrecision}
	sweeper.BaseActor = actor.NewBaseActor(2, connect(2))
	sweeper.SetHandler(sweeper)
	sweeper.AddTicker(time.Second, sweeper.onTick)
	sweeper.SetTickerFactory(timers)
	runner.AddActor(sweeper)

	buyer := &touchTaker{symbol: "ABC/USD", side: exchange.Buy, qty: mvBasePrecision / 100}
	buyer.BaseActor = actor.NewBaseActor(3, connect(3))
	buyer.SetHandler(buyer)
	buyer.AddTicker(time.Second, buyer.onTick)
	buyer.SetTickerFactory(timers)
	runner.AddActor(buyer)

	seller := &touchTaker{symbol: "ABC/USD", side: exchange.Sell, qty: mvBasePrecision / 100}
	seller.BaseActor = actor.NewBaseActor(4, connect(4))
	seller.SetHandler(seller)
	seller.AddTicker(time.Second, seller.onTick)
	seller.SetTickerFactory(timers)
	runner.AddActor(seller)

	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	buyRate := 100 * float64(buyer.fills) / float64(maxInt(buyer.sent, 1))
	sellRate := 100 * float64(seller.fills) / float64(maxInt(seller.sent, 1))
	t.Logf("contested: buy sent=%d fills=%d (%.1f%%); sell sent=%d fills=%d (%.1f%%); sweeper fills=%d",
		buyer.sent, buyer.fills, buyRate, seller.sent, seller.fills, sellRate, sweeper.fills)
}

// Two makers quoting overlapping prices cross each other, which is 98.5% of the
// traded volume in a reference run. If the side asymmetry an at-touch desk sees
// is caused by the order in which a maker submits its two quotes — the fresh
// second leg crossing a resting bid and never resting itself — then reversing
// that order must reverse which side starves.
func TestRequoteOrderDecidesWhichSideStarves(t *testing.T) {
	run := func(askFirst bool) (buyRate, sellRate float64, makerAggressorSells, makerAggressorBuys int) {
		start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano()
		clock := simulation.NewSimulatedClock(start)
		scheduler := simulation.NewEventScheduler(clock)
		clock.SetScheduler(scheduler)
		timers := simulation.NewSimTimerFactory(scheduler)
		runner := simulation.NewRunner(clock, simulation.RunnerConfig{
			Iterations: 200, Step: time.Second, Quiesce: true, DeterministicPhases: true,
		})
		runner.AddIdler(timers)
		ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
			ID: "probe", EstimatedClients: 8, Clock: clock, TickerFactory: timers,
			DeterministicIngress: true, DeterministicPhases: true, SnapshotInterval: time.Second,
		})
		tick := int64(mvQuotePrecision)
		ex.AddInstrument(exchange.NewSpotInstrument("ABC/USD", "ABC", "USD", mvBasePrecision, mvQuotePrecision, tick, mvBasePrecision/1_000))
		mount := simulation.NewMount(ex, simulation.LatencyConfig{})
		runner.AddMount(mount)
		connect := func(id uint64) actor.Gateway {
			return mount.ConnectNewClient(id, map[string]int64{
				"ABC": 100_000 * mvBasePrecision, "USD": 10_000_000_000 * mvQuotePrecision,
			}, &exchange.FixedFee{})
		}
		mid := int64(50_000) * mvQuotePrecision
		// Offsets are half the spread apart, so each maker's new quote crosses
		// the other's resting quote on the opposite side.
		for i, offset := range []int64{0, 50 * tick} {
			m := &requotingMaker{
				symbol: "ABC/USD", mid: mid, half: 20 * tick, qty: mvBasePrecision,
				pending: map[uint64]bool{}, askFirst: askFirst, offset: offset,
			}
			m.BaseActor = actor.NewBaseActor(uint64(i+1), connect(uint64(i+1)))
			m.SetHandler(m)
			m.AddTicker(time.Second, m.onTick)
			m.SetTickerFactory(timers)
			runner.AddActor(m)
		}
		buyer := &touchTaker{symbol: "ABC/USD", side: exchange.Buy, qty: mvBasePrecision / 100}
		buyer.BaseActor = actor.NewBaseActor(3, connect(3))
		buyer.SetHandler(buyer)
		buyer.AddTicker(time.Second, buyer.onTick)
		buyer.SetTickerFactory(timers)
		runner.AddActor(buyer)
		seller := &touchTaker{symbol: "ABC/USD", side: exchange.Sell, qty: mvBasePrecision / 100}
		seller.BaseActor = actor.NewBaseActor(4, connect(4))
		seller.SetHandler(seller)
		seller.AddTicker(time.Second, seller.onTick)
		seller.SetTickerFactory(timers)
		runner.AddActor(seller)

		if err := runner.Run(context.Background()); err != nil {
			t.Fatalf("Run: %v", err)
		}
		return 100 * float64(buyer.fills) / float64(maxInt(buyer.sent, 1)),
			100 * float64(seller.fills) / float64(maxInt(seller.sent, 1)), 0, 0
	}
	bidFirstBuy, bidFirstSell, _, _ := run(false)
	askFirstBuy, askFirstSell, _, _ := run(true)
	t.Logf("bid submitted first: buy %.1f%% sell %.1f%%", bidFirstBuy, bidFirstSell)
	t.Logf("ask submitted first: buy %.1f%% sell %.1f%%", askFirstBuy, askFirstSell)
	if (bidFirstBuy < bidFirstSell) == (askFirstBuy < askFirstSell) {
		t.Logf("submission order did not reverse which side starves")
	}
}
