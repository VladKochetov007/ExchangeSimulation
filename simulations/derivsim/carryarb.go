package derivsim

import (
	"context"
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
}

// BasisSample records one basis observation for post-run analysis.
type BasisSample struct {
	Symbol       string
	TimeToExpiry time.Duration
	BasisBps     float64
}

type carryPos struct {
	position int64 // signed futures position (spot hedge is the mirror)
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
		// Futures leg cash-settled by the exchange; unwind the spot hedge.
		if st, ok := a.state[c.Symbol]; ok && st.position != 0 {
			qty := st.position
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
				a.spotMid = (e.Snapshot.Bids[0].Price + e.Snapshot.Asks[0].Price) / 2
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
	if a.spotMid == 0 {
		return
	}
	now := t.UnixNano()
	for sym, c := range a.set.contracts {
		if c.Type != "FUTURE" {
			continue
		}
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
		switch {
		case futMid-a.spotMid > edge && st.position > -a.cfg.MaxPosPerSym:
			// Future rich: sell future, buy spot.
			a.leg(sym, exchange.Sell)
			a.leg(a.cfg.Underlying, exchange.Buy)
			st.position -= a.cfg.LotQty
		case a.spotMid-futMid > edge && st.position < a.cfg.MaxPosPerSym:
			// Future cheap: buy future, sell spot.
			a.leg(sym, exchange.Buy)
			a.leg(a.cfg.Underlying, exchange.Sell)
			st.position += a.cfg.LotQty
		}
	}
}

func (a *CashCarryArb) leg(symbol string, side exchange.Side) {
	reqID := a.SubmitOrder(symbol, side, exchange.Market, 0, a.cfg.LotQty)
	a.set.trackRequest(reqID, symbol)
}
