package multivenue

import (
	"context"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	etypes "exchange_sim/types"
)

// ValueTraderConfig describes one informed participant.
//
// The trader knows the exogenous fundamental value and trades the visible book
// against it. This is the only participant with a view: market makers quote
// around their own inventory and noise traders pick sides at random, so without
// informed flow nothing opposes a drift in the quoted price.
type ValueTraderConfig struct {
	Symbol        string
	BasePrecision int64
	TickSize      int64
	// EdgeBps is the deviation from fundamental value required before trading,
	// which keeps the trader from paying the spread on noise.
	EdgeBps int64
	// LotQty is the base quantity of one order.
	LotQty int64
	// MaxInventory bounds the signed base position, so an informed trader
	// cannot become an unbounded directional bet on its own signal.
	MaxInventory int64
	// ExitBps closes the position back toward flat once the price has come
	// within this distance of fundamental value.
	//
	// Without it an informed trader only ever reverses: it buys below value and
	// sells above, and holds everything in between. Under an anchor that keeps
	// the price near value it therefore never unwinds, so its position ratchets
	// with the fundamental's cumulative drift and market makers hold the
	// mirror of it indefinitely.
	ExitBps       int64
	TradeInterval time.Duration
}

// ValueTrader buys visible offers below fundamental value and sells visible
// bids above it, in bounded size.
type ValueTrader struct {
	*actor.BaseActor
	cfg        ValueTraderConfig
	value      *FundamentalValue
	bestBid    int64
	bestAsk    int64
	bidQty     int64
	askQty     int64
	inventory  int64
	subscribed bool
}

func NewValueTrader(id uint64, gw actor.Gateway, value *FundamentalValue, cfg ValueTraderConfig) *ValueTrader {
	vt := &ValueTrader{
		BaseActor: actor.NewBaseActor(id, gw),
		cfg:       cfg,
		value:     value,
	}
	vt.SetHandler(vt)
	vt.AddTicker(cfg.TradeInterval, vt.onTick)
	return vt
}

func (vt *ValueTrader) Inventory() int64 { return vt.inventory }

func (vt *ValueTrader) HandleEvent(_ context.Context, evt *actor.Event) {
	switch evt.Type {
	case actor.EventBookSnapshot:
		e := evt.Data.(actor.BookSnapshotEvent)
		if e.Symbol != vt.cfg.Symbol || e.Snapshot == nil {
			return
		}
		vt.bestBid, vt.bidQty, vt.bestAsk, vt.askQty = 0, 0, 0, 0
		if len(e.Snapshot.Bids) > 0 {
			vt.bestBid, vt.bidQty = e.Snapshot.Bids[0].Price, e.Snapshot.Bids[0].VisibleQty
		}
		if len(e.Snapshot.Asks) > 0 {
			vt.bestAsk, vt.askQty = e.Snapshot.Asks[0].Price, e.Snapshot.Asks[0].VisibleQty
		}
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		e := evt.Data.(actor.OrderFillEvent)
		if e.Symbol != vt.cfg.Symbol {
			return
		}
		if e.Side == exchange.Buy {
			vt.inventory += e.Qty
		} else {
			vt.inventory -= e.Qty
		}
	}
}

func (vt *ValueTrader) onTick(now time.Time) {
	if !vt.subscribed {
		vt.Subscribe(vt.cfg.Symbol, exchange.MDSnapshot)
		vt.subscribed = true
		return
	}
	if vt.cfg.LotQty <= 0 || vt.cfg.TickSize <= 0 {
		return
	}
	fair := vt.value.Value(now.UnixNano())
	if fair <= 0 {
		return
	}
	band, ok := etypes.TryMulBps(fair, vt.cfg.EdgeBps)
	if !ok {
		return
	}

	if vt.exitTowardFlat(fair) {
		return
	}

	// Only trade against liquidity that is actually displayed, and never take
	// more than the visible size of the level being hit.
	if vt.bestAsk > 0 && vt.bestAsk < fair-band && vt.inventory < vt.cfg.MaxInventory {
		if qty := vt.sizeFor(vt.askQty, vt.cfg.MaxInventory-vt.inventory); qty > 0 {
			vt.SubmitOrderWithTimeInForce(vt.cfg.Symbol, exchange.Buy, exchange.LimitOrder, vt.bestAsk, qty, exchange.IOC)
			return
		}
	}
	if vt.bestBid > 0 && vt.bestBid > fair+band && vt.inventory > -vt.cfg.MaxInventory {
		if qty := vt.sizeFor(vt.bidQty, vt.cfg.MaxInventory+vt.inventory); qty > 0 {
			vt.SubmitOrderWithTimeInForce(vt.cfg.Symbol, exchange.Sell, exchange.LimitOrder, vt.bestBid, qty, exchange.IOC)
		}
	}
}

// exitTowardFlat unwinds an existing position once the price has returned to
// within ExitBps of fundamental value. It reports whether it acted.
func (vt *ValueTrader) exitTowardFlat(fair int64) bool {
	if vt.cfg.ExitBps <= 0 || vt.inventory == 0 {
		return false
	}
	band, ok := etypes.TryMulBps(fair, vt.cfg.ExitBps)
	if !ok {
		return false
	}
	if vt.inventory > 0 {
		// Long: sell back into the bid, but only once it is no longer cheap.
		if vt.bestBid <= 0 || vt.bestBid < fair-band {
			return false
		}
		if qty := vt.sizeFor(vt.bidQty, vt.inventory); qty > 0 {
			vt.SubmitOrderWithTimeInForce(vt.cfg.Symbol, exchange.Sell, exchange.LimitOrder, vt.bestBid, qty, exchange.IOC)
			return true
		}
		return false
	}
	if vt.bestAsk <= 0 || vt.bestAsk > fair+band {
		return false
	}
	if qty := vt.sizeFor(vt.askQty, -vt.inventory); qty > 0 {
		vt.SubmitOrderWithTimeInForce(vt.cfg.Symbol, exchange.Buy, exchange.LimitOrder, vt.bestAsk, qty, exchange.IOC)
		return true
	}
	return false
}

func (vt *ValueTrader) sizeFor(visible, headroom int64) int64 {
	qty := vt.cfg.LotQty
	if visible > 0 && visible < qty {
		qty = visible
	}
	if headroom < qty {
		qty = headroom
	}
	if qty <= 0 {
		return 0
	}
	return qty
}
