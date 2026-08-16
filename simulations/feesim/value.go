package feesim

import (
	"context"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// ValueTraderConfig drives an ABIDES-style value agent: it holds its own opinion
// of what the asset is worth and trades against deviations from it, supplying the
// mean-reversion force that pins the price level in an otherwise
// random-flow ecology.
type ValueTraderConfig struct {
	Symbol        string
	BelievedValue int64         // this agent's own opinion of fair value; nothing validates it
	BandBps       int64         // act when |mid - believed value| exceeds this
	LotQty        int64         // market order size per action
	MaxPosition   int64         // absolute position cap in base units
	Interval      time.Duration // decision cadence
}

type ValueTrader struct {
	*actor.BaseActor
	cfg        ValueTraderConfig
	mid        int64
	position   int64
	subscribed bool
}

func NewValueTrader(id uint64, gw actor.Gateway, cfg ValueTraderConfig) *ValueTrader {
	vt := &ValueTrader{
		BaseActor: actor.NewBaseActor(id, gw),
		cfg:       cfg,
	}
	vt.SetHandler(vt)
	vt.AddTicker(cfg.Interval, vt.onTick)
	return vt
}

func (vt *ValueTrader) Position() int64 { return vt.position }

func (vt *ValueTrader) HandleEvent(_ context.Context, evt *actor.Event) {
	switch evt.Type {
	case actor.EventBookSnapshot:
		e := evt.Data.(actor.BookSnapshotEvent)
		if e.Symbol != vt.cfg.Symbol {
			return
		}
		snap := e.Snapshot
		if len(snap.Bids) > 0 && len(snap.Asks) > 0 {
			vt.mid = (snap.Bids[0].Price + snap.Asks[0].Price) / 2
		}
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		e := evt.Data.(actor.OrderFillEvent)
		if e.Side == exchange.Buy {
			vt.position += e.Qty
		} else {
			vt.position -= e.Qty
		}
	}
}

func (vt *ValueTrader) onTick(_ time.Time) {
	if !vt.subscribed {
		vt.Subscribe(vt.cfg.Symbol, exchange.MDSnapshot)
		vt.subscribed = true
	}
	if vt.mid == 0 {
		return
	}
	deviationBps := (vt.mid - vt.cfg.BelievedValue) * 10000 / vt.cfg.BelievedValue
	switch {
	case deviationBps < -vt.cfg.BandBps && vt.position < vt.cfg.MaxPosition:
		vt.SubmitOrder(vt.cfg.Symbol, exchange.Buy, exchange.Market, 0, vt.cfg.LotQty)
	case deviationBps > vt.cfg.BandBps && vt.position > -vt.cfg.MaxPosition:
		vt.SubmitOrder(vt.cfg.Symbol, exchange.Sell, exchange.Market, 0, vt.cfg.LotQty)
	}
}
