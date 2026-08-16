package multivenue

import (
	"context"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// BootstrapDepthConfig describes a passive depth ladder anchored to the venue
// index. Unlike a market maker it never cancels a resting level to reprice: it
// only replaces levels the market has consumed. That separates two properties
// that quoting makers hold together — whether depth exists at all, and whether
// it exists at the instant a given actor is scheduled.
type BootstrapDepthConfig struct {
	Symbol      string        `json:"symbol"`
	Levels      int           `json:"levels"`
	QtyPerLevel int64         `json:"qty_per_level"`
	SpacingBps  int64         `json:"spacing_bps"`
	Interval    time.Duration `json:"interval"`
	// Withdraw stops refilling after this much simulated time, so a run can ask
	// whether the ecology still functions once the scaffold is removed. Zero
	// leaves the ladder in place for the whole run.
	Withdraw time.Duration `json:"withdraw"`
	TickSize int64         `json:"-"`
}

type depthLevel struct {
	side    exchange.Side
	offset  int64
	orderID uint64
	pending bool
}

// BootstrapDepth rests a static ladder and refills only what gets consumed.
type BootstrapDepth struct {
	*actor.BaseActor
	cfg        BootstrapDepthConfig
	levels     []*depthLevel
	pending    map[uint64]*depthLevel
	indexPrice int64
	started    int64
	now        int64
	subscribed bool
	withdrawn  bool
}

func NewBootstrapDepth(id uint64, gw actor.Gateway, cfg BootstrapDepthConfig) *BootstrapDepth {
	d := &BootstrapDepth{
		BaseActor: actor.NewBaseActor(id, gw),
		cfg:       cfg,
		pending:   make(map[uint64]*depthLevel),
	}
	for level := 1; level <= cfg.Levels; level++ {
		offset := int64(level) * cfg.SpacingBps
		d.levels = append(d.levels,
			&depthLevel{side: exchange.Buy, offset: offset},
			&depthLevel{side: exchange.Sell, offset: offset},
		)
	}
	d.SetHandler(d)
	d.AddTicker(cfg.Interval, d.onTick)
	return d
}

// Withdrawn reports whether the scaffold has stopped refilling.
func (d *BootstrapDepth) Withdrawn() bool { return d.withdrawn }

func (d *BootstrapDepth) HandleEvent(_ context.Context, evt *actor.Event) {
	switch evt.Type {
	case actor.EventIndex:
		e := evt.Data.(actor.IndexEvent)
		if e.Symbol == d.cfg.Symbol && e.Price > 0 {
			d.indexPrice = e.Price
		}
	case actor.EventOrderAccepted:
		e := evt.Data.(actor.OrderAcceptedEvent)
		level, ok := d.pending[e.RequestID]
		if !ok {
			// Accept for a level already retired: cancel rather than leak depth
			// this actor no longer tracks.
			d.CancelOrder(e.OrderID)
			return
		}
		delete(d.pending, e.RequestID)
		level.pending, level.orderID = false, e.OrderID
	case actor.EventOrderRejected:
		e := evt.Data.(actor.OrderRejectedEvent)
		if level, ok := d.pending[e.RequestID]; ok {
			delete(d.pending, e.RequestID)
			level.pending = false
		}
	case actor.EventOrderFilled:
		d.releaseFilled(evt.Data.(actor.OrderFillEvent))
	case actor.EventOrderCancelled:
		e := evt.Data.(actor.OrderCancelledEvent)
		d.release(e.OrderID)
	}
}

func (d *BootstrapDepth) releaseFilled(e actor.OrderFillEvent) {
	if e.IsFull {
		d.release(e.OrderID)
	}
}

func (d *BootstrapDepth) release(orderID uint64) {
	for _, level := range d.levels {
		if level.orderID == orderID {
			level.orderID = 0
			return
		}
	}
}

func (d *BootstrapDepth) onTick(t time.Time) {
	d.now = t.UnixNano()
	if d.started == 0 {
		d.started = d.now
	}
	if !d.subscribed {
		d.Subscribe(d.cfg.Symbol, exchange.MDIndex)
		d.subscribed = true
		return
	}
	if d.cfg.Withdraw > 0 && d.now-d.started >= d.cfg.Withdraw.Nanoseconds() {
		d.withdrawn = true
		return
	}
	if d.indexPrice <= 0 || d.cfg.TickSize <= 0 || d.cfg.QtyPerLevel <= 0 {
		return
	}
	for _, level := range d.levels {
		if level.orderID != 0 || level.pending {
			continue
		}
		price := d.levelPrice(level)
		if price <= 0 {
			continue
		}
		reqID := d.SubmitOrder(d.cfg.Symbol, level.side, exchange.LimitOrder, price, d.cfg.QtyPerLevel)
		d.pending[reqID] = level
		level.pending = true
	}
}

func (d *BootstrapDepth) levelPrice(level *depthLevel) int64 {
	offset := d.indexPrice * level.offset / 10000
	if level.side == exchange.Buy {
		return ((d.indexPrice - offset) / d.cfg.TickSize) * d.cfg.TickSize
	}
	price := d.indexPrice + offset
	return ((price + d.cfg.TickSize - 1) / d.cfg.TickSize) * d.cfg.TickSize
}
