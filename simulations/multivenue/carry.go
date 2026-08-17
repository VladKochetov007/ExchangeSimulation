package multivenue

import (
	"context"
	"sort"
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
	// MinOrderSize is the venue minimum. Clamping to the visible touch size can
	// land below it, and those orders are rejected rather than filled.
	MinOrderSize int64 `json:"min_order_size"`
	SpotTick     int64 `json:"spot_tick"`
	PerpTick     int64 `json:"perp_tick"`
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
	fills      int
	filledQty  int64
	feesPaid   int64
	notional   int64
	entryBasis []float64
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

// CarryActivity summarises what a carry participant actually did, which
// separates a strategy starved of opportunities from one earning thinner
// margins on the same activity.
type CarryActivity struct {
	VenueID          string  `json:"venue_id"`
	Attempts         int     `json:"attempts"`
	Rejects          int     `json:"rejects"`
	Fills            int     `json:"fills"`
	FilledQty        int64   `json:"filled_qty"`
	Entries          int     `json:"entries"`
	MedianEntryBasis float64 `json:"median_entry_basis_bps"`
	FeesPaid         int64   `json:"fees_paid"`
	Notional         int64   `json:"notional"`
	// FeeCostBps is fees as a fraction of the notional traded, which is the
	// cost the basis has to beat.
	FeeCostBps float64 `json:"fee_cost_bps"`
}

// Activity reports this participant's execution record.
func (c *CarryArbitrageur) Activity(venueID string) CarryActivity {
	median := 0.0
	if len(c.entryBasis) > 0 {
		sorted := append([]float64(nil), c.entryBasis...)
		sort.Float64s(sorted)
		median = sorted[len(sorted)/2]
	}
	feeCost := 0.0
	if c.notional > 0 {
		feeCost = 1e4 * float64(c.feesPaid) / float64(c.notional)
	}
	return CarryActivity{
		VenueID: venueID, Attempts: c.attempts, Rejects: c.rejects, Fills: c.fills,
		FilledQty: c.filledQty, Entries: len(c.entryBasis), MedianEntryBasis: median,
		FeesPaid: c.feesPaid, Notional: c.notional, FeeCostBps: feeCost,
	}
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
		c.fills++
		c.filledQty += e.Qty
		c.feesPaid += e.FeeAmount
		// Notional in quote units, so fees and edge are comparable.
		c.notional += e.Qty * e.Price / c.cfg.BasePrecision
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
		// Record the basis being entered on, so a fall in results can be
		// attributed either to fewer opportunities or to thinner ones.
		if spotMid, perpMid := c.spot.mid(), c.perp.mid(); spotMid > 0 && perpMid > 0 {
			c.entryBasis = append(c.entryBasis, 1e4*float64(perpMid-spotMid)/float64(spotMid))
		}
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
	sized, ok := venueSizedQty(quantity, available, c.cfg.MinOrderSize)
	if !ok {
		return
	}
	quantity = sized
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
