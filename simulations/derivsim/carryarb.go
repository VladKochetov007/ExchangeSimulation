package derivsim

import (
	"context"
	"math"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// CarryArbConfig drives a cash-and-carry arbitrageur across every dated
// future on Underlying: when a future trades rich to spot beyond EdgeBps it
// sells the future and buys spot; cheap, the reverse. Because the contract
// cash-settles to the spot TWAP, the arb is the force that drags the basis
// to zero into expiry — the H-F1 emergent-convergence hypothesis.
type CarryArbConfig struct {
	Underlying    string
	EdgeBps       int64
	LotQty        int64
	MaxPosPerSym  int64
	CheckInterval time.Duration
	// TenorNano scales the edge with carry risk when set: effective edge =
	// EdgeBps × sqrt(timeToExpiry/TenorNano). A rational carry desk demands
	// less edge as settlement risk shrinks, which is what produces the
	// square-root convergence envelope into expiry.
	TenorNano int64
	// TickSize and SlippageBps switch the legs from unbounded market orders to
	// tick-aligned IOC limits that cross by at most SlippageBps. A market order
	// against a momentarily empty book is accepted and then force-cancelled, so
	// a desk that counts intent rather than fills silently ends up with one leg
	// on and no hedge. Leave TickSize zero to keep market orders.
	TickSize    int64
	SlippageBps int64
}

// BasisSample records one basis observation for post-run analysis.
type BasisSample struct {
	Symbol       string
	TimeToExpiry time.Duration
	BasisBps     float64
}

type carryPos struct {
	position int64 // signed intent counter (submits), used to gate signals
	filled   int64 // signed futures position from actual fills
	bestBid  int64
	bestAsk  int64
}

// CashCarryArb trades future-vs-spot basis and records the basis series.
type CashCarryArb struct {
	*actor.BaseActor
	cfg        CarryArbConfig
	set        *contractSet
	state      map[string]*carryPos
	spotMid    int64
	spotBid    int64
	spotAsk    int64
	samples    []BasisSample
	subscribed bool
}

func NewCashCarryArb(id uint64, gw actor.Gateway, cfg CarryArbConfig) *CashCarryArb {
	a := &CashCarryArb{
		BaseActor: actor.NewBaseActor(id, gw),
		cfg:       cfg,
		set:       newContractSet(cfg.Underlying),
		state:     make(map[string]*carryPos),
	}
	a.set.onList = func(c *Contract) {
		if c.Type == "FUTURE" {
			a.state[c.Symbol] = &carryPos{}
			a.Subscribe(c.Symbol, exchange.MDSnapshot)
		}
	}
	a.set.onSettle = func(c *Contract, _ int64) {
		// Futures leg cash-settled by the exchange; unwind the spot hedge
		// sized by what actually FILLED, not by the intent counter — a
		// rejected or partial leg must not leave residual spot exposure.
		if st, ok := a.state[c.Symbol]; ok && st.filled != 0 {
			qty := st.filled
			side := exchange.Buy // long futures were hedged short spot: buy it back
			if qty < 0 {
				side = exchange.Sell
				qty = -qty
			}
			reqID := a.SubmitOrder(a.cfg.Underlying, side, exchange.Market, 0, qty)
			a.set.trackRequest(reqID, a.cfg.Underlying)
		}
		delete(a.state, c.Symbol)
	}
	a.set.onFill = func(sym string, e actor.OrderFillEvent) {
		if st, ok := a.state[sym]; ok {
			st.filled += signedQty(e)
		}
	}
	a.SetHandler(a)
	a.AddTicker(cfg.CheckInterval, a.onTick)
	return a
}

// BasisSeries returns the recorded basis observations.
func (a *CashCarryArb) BasisSeries() []BasisSample { return a.samples }

func (a *CashCarryArb) HandleEvent(_ context.Context, evt *actor.Event) {
	if evt.Type == actor.EventBookSnapshot {
		e := evt.Data.(actor.BookSnapshotEvent)
		if e.Symbol == a.cfg.Underlying {
			if len(e.Snapshot.Bids) > 0 && len(e.Snapshot.Asks) > 0 {
				a.spotBid, a.spotAsk = e.Snapshot.Bids[0].Price, e.Snapshot.Asks[0].Price
				a.spotMid = (a.spotBid + a.spotAsk) / 2
			} else {
				a.spotBid, a.spotAsk = 0, 0
			}
			return
		}
		if st, ok := a.state[e.Symbol]; ok {
			if len(e.Snapshot.Bids) > 0 {
				st.bestBid = e.Snapshot.Bids[0].Price
			}
			if len(e.Snapshot.Asks) > 0 {
				st.bestAsk = e.Snapshot.Asks[0].Price
			}
		}
		return
	}
	a.set.handle(evt)
}

func (a *CashCarryArb) onTick(t time.Time) {
	if !a.subscribed {
		a.Subscribe(exchange.InstrumentFeedSymbol, exchange.MDInstrument)
		a.Subscribe(a.cfg.Underlying, exchange.MDSnapshot)
		a.subscribed = true
		return
	}
	if a.spotMid == 0 || a.spotBid == 0 || a.spotAsk == 0 {
		// Both legs or neither: firing the futures leg while spot is uncrossable
		// converts a delta-neutral trade into naked futures exposure.
		return
	}
	now := t.UnixNano()
	for _, c := range a.set.orderedContracts() {
		if c.Type != "FUTURE" {
			continue
		}
		sym := c.Symbol
		st := a.state[sym]
		if st == nil || st.bestBid == 0 || st.bestAsk == 0 {
			continue
		}
		futMid := (st.bestBid + st.bestAsk) / 2
		basisBps := float64(futMid-a.spotMid) / float64(a.spotMid) * 10000
		a.samples = append(a.samples, BasisSample{
			Symbol:       sym,
			TimeToExpiry: time.Duration(c.ExpiryNano - now),
			BasisBps:     basisBps,
		})

		edge := a.spotMid * a.cfg.EdgeBps / 10000
		if a.cfg.TenorNano > 0 {
			frac := float64(c.ExpiryNano-now) / float64(a.cfg.TenorNano)
			if frac < 0 {
				frac = 0
			}
			edge = int64(float64(edge) * math.Sqrt(frac))
		}
		switch {
		case futMid-a.spotMid > edge && st.position > -a.cfg.MaxPosPerSym:
			// Future rich: sell future, buy spot.
			a.legAt(sym, exchange.Sell, st.bestBid)
			a.legAt(a.cfg.Underlying, exchange.Buy, a.spotAsk)
			st.position -= a.cfg.LotQty
		case a.spotMid-futMid > edge && st.position < a.cfg.MaxPosPerSym:
			// Future cheap: buy future, sell spot.
			a.legAt(sym, exchange.Buy, st.bestAsk)
			a.legAt(a.cfg.Underlying, exchange.Sell, a.spotBid)
			st.position += a.cfg.LotQty
		}
	}
}

func (a *CashCarryArb) leg(symbol string, side exchange.Side) {
	a.legAt(symbol, side, 0)
}

// legAt crosses with a bounded IOC limit when a tick size is configured, and
// falls back to an unbounded market order otherwise.
func (a *CashCarryArb) legAt(symbol string, side exchange.Side, touch int64) {
	if a.cfg.TickSize <= 0 || touch <= 0 {
		reqID := a.SubmitOrder(symbol, side, exchange.Market, 0, a.cfg.LotQty)
		a.set.trackRequest(reqID, symbol)
		return
	}
	limit := touch + touch*a.cfg.SlippageBps/10000
	limit = (limit / a.cfg.TickSize) * a.cfg.TickSize
	if side == exchange.Sell {
		limit = touch - touch*a.cfg.SlippageBps/10000
		limit = ((limit + a.cfg.TickSize - 1) / a.cfg.TickSize) * a.cfg.TickSize
	}
	if limit <= 0 {
		return
	}
	reqID := a.SubmitOrderWithTimeInForce(symbol, side, exchange.LimitOrder, limit, a.cfg.LotQty, exchange.IOC)
	a.set.trackRequest(reqID, symbol)
}
