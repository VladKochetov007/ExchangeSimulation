package multivenue

import (
	"context"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	etypes "exchange_sim/types"
)

// CarryArbitrageurConfig describes a participant that is delta neutral by
// construction and carries the basis instead.
//
// Every intervention so far has moved directional risk rather than absorbing
// it: skewing a maker's quotes moves the price the anchor is built from,
// round-trip flow promotes the next accumulator, and hedging concentrates risk
// in whoever does not hedge. A cash-and-carry participant is different because
// it holds offsetting positions in the spot and the perpetual, so it can take
// the other side of directional flow without accumulating direction itself.
type CarryArbitrageurConfig struct {
	SpotSymbol    string        `json:"spot_symbol"`
	PerpSymbol    string        `json:"perp_symbol"`
	BasePrecision int64         `json:"base_precision"`
	Interval      time.Duration `json:"interval"`
	// EntryBps is the basis, in basis points of the spot price, at which the
	// participant starts carrying. ExitBps is where it unwinds.
	EntryBps int64 `json:"entry_bps"`
	ExitBps  int64 `json:"exit_bps"`
	// MaxPosition bounds the carried size, and LotQty the size added per tick.
	MaxPosition int64 `json:"max_position"`
	LotQty      int64 `json:"lot_qty"`
	SpotTick    int64 `json:"spot_tick"`
	PerpTick    int64 `json:"perp_tick"`
}

// CarryArbitrageur holds matched spot and perpetual positions.
type CarryArbitrageur struct {
	*actor.BaseActor
	cfg CarryArbitrageurConfig

	spot, perp bookTouch
	spotPos    int64
	perpPos    int64
	pending    bool
	attempts   int
	rejects    int
	lastReject exchange.RejectReason
	spotSeen   int
	perpSeen   int
	subscribed bool
}

type bookTouch struct{ bid, ask, bidQty, askQty int64 }

func (b bookTouch) mid() int64 {
	if b.bid <= 0 || b.ask <= 0 {
		return 0
	}
	return (b.bid + b.ask) / 2
}

func NewCarryArbitrageur(id uint64, gw actor.Gateway, cfg CarryArbitrageurConfig) *CarryArbitrageur {
	c := &CarryArbitrageur{BaseActor: actor.NewBaseActor(id, gw), cfg: cfg}
	c.SetHandler(c)
	c.AddTicker(cfg.Interval, c.onTick)
	return c
}

// SpotPosition and PerpPosition expose the two legs; their sum is the
// participant's directional exposure and should stay near zero.
func (c *CarryArbitrageur) SpotPosition() int64 { return c.spotPos }
func (c *CarryArbitrageur) PerpPosition() int64 { return c.perpPos }

func (c *CarryArbitrageur) HandleEvent(_ context.Context, evt *actor.Event) {
	switch evt.Type {
	case actor.EventBookSnapshot:
		e := evt.Data.(actor.BookSnapshotEvent)
		touch := bookTouch{}
		if e.Snapshot != nil {
			if len(e.Snapshot.Bids) > 0 {
				touch.bid, touch.bidQty = e.Snapshot.Bids[0].Price, e.Snapshot.Bids[0].VisibleQty
			}
			if len(e.Snapshot.Asks) > 0 {
				touch.ask, touch.askQty = e.Snapshot.Asks[0].Price, e.Snapshot.Asks[0].VisibleQty
			}
		}
		switch e.Symbol {
		case c.cfg.SpotSymbol:
			c.spot, c.spotSeen = touch, c.spotSeen+1
		case c.cfg.PerpSymbol:
			c.perp, c.perpSeen = touch, c.perpSeen+1
		}
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		e := evt.Data.(actor.OrderFillEvent)
		signed := e.Qty
		if e.Side == exchange.Sell {
			signed = -signed
		}
		switch e.Symbol {
		case c.cfg.SpotSymbol:
			c.spotPos += signed
		case c.cfg.PerpSymbol:
			c.perpPos += signed
		}
		if e.IsFull {
			c.pending = false
		}
	case actor.EventOrderCancelled:
		c.pending = false
	case actor.EventOrderRejected:
		c.rejects++
		c.lastReject = evt.Data.(actor.OrderRejectedEvent).Reason
		c.pending = false
	}
}

// targetCarry is the spot leg the participant wants. A positive value means
// long spot and short perpetual, which is the trade when the perpetual is rich.
func (c *CarryArbitrageur) targetCarry() int64 {
	spotMid, perpMid := c.spot.mid(), c.perp.mid()
	if spotMid <= 0 || perpMid <= 0 {
		return c.spotPos
	}
	entry, entryOK := etypes.TryMulBps(spotMid, c.cfg.EntryBps)
	exit, exitOK := etypes.TryMulBps(spotMid, c.cfg.ExitBps)
	if !entryOK || !exitOK {
		return c.spotPos
	}
	basis := perpMid - spotMid
	switch {
	case basis > entry:
		return c.cfg.MaxPosition
	case basis < -entry:
		return -c.cfg.MaxPosition
	case basis < exit && basis > -exit:
		// The basis has closed: carry no position rather than holding one that
		// no longer earns anything.
		return 0
	}
	return c.spotPos
}

func (c *CarryArbitrageur) onTick(time.Time) {
	if !c.subscribed {
		c.Subscribe(c.cfg.SpotSymbol, exchange.MDSnapshot)
		c.Subscribe(c.cfg.PerpSymbol, exchange.MDSnapshot)
		c.subscribed = true
		return
	}
	if c.pending || c.cfg.LotQty <= 0 {
		return
	}

	// Keep the legs matched first: an unmatched leg is directional risk, which
	// is precisely what this participant exists not to hold.
	if mismatch := c.spotPos + c.perpPos; mismatch != 0 {
		c.tradePerp(-mismatch)
		return
	}
	if gap := c.targetCarry() - c.spotPos; gap != 0 {
		c.tradeSpot(gap)
	}
}

func (c *CarryArbitrageur) tradeSpot(gap int64) {
	side, quantity, price, available := exchange.Buy, gap, c.spot.ask, c.spot.askQty
	if gap < 0 {
		side, quantity, price, available = exchange.Sell, -gap, c.spot.bid, c.spot.bidQty
	}
	c.submit(c.cfg.SpotSymbol, side, price, quantity, available, c.cfg.SpotTick)
}

func (c *CarryArbitrageur) tradePerp(gap int64) {
	side, quantity, price, available := exchange.Buy, gap, c.perp.ask, c.perp.askQty
	if gap < 0 {
		side, quantity, price, available = exchange.Sell, -gap, c.perp.bid, c.perp.bidQty
	}
	c.submit(c.cfg.PerpSymbol, side, price, quantity, available, c.cfg.PerpTick)
}

func (c *CarryArbitrageur) submit(symbol string, side exchange.Side, price, quantity, available, tick int64) {
	if price <= 0 || quantity <= 0 {
		return
	}
	if quantity > c.cfg.LotQty {
		quantity = c.cfg.LotQty
	}
	if available > 0 && quantity > available {
		quantity = available
	}
	if quantity <= 0 {
		return
	}
	// Prices must sit on the instrument's grid or the venue rejects them
	// outright, which is silent because a rejection is not a fill.
	if tick > 0 {
		if side == exchange.Buy {
			price = (price + tick - 1) / tick * tick
		} else {
			price = price / tick * tick
		}
		if price <= 0 {
			return
		}
	}
	c.attempts++
	c.SubmitOrderWithTimeInForce(symbol, side, exchange.LimitOrder, price, quantity, exchange.IOC)
	c.pending = true
}
