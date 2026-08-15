package multivenue

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// MetaorderTraderConfig describes a participant that executes large parent
// orders by splitting them into children over time.
//
// This is the agent the market impact literature is about. A single large
// order tells you about book depletion; a *split* order executed at a
// controlled participation rate is what produces the square-root impact law
// and, through splitting, the long memory of order-flow signs.
//
// The sign of each parent is drawn independently of the fundamental value
// process on purpose. An informed metaorder would measure the trader's alpha
// rather than the mechanical impact of its own execution, which is the single
// most common way this measurement goes wrong.
type MetaorderTraderConfig struct {
	Symbol        string `json:"symbol"`
	BasePrecision int64  `json:"base_precision"`

	// Parent sizes are drawn from a Pareto tail, which is what metaorder sizes
	// empirically follow. The exponent matters: the splitting explanation of
	// order-flow memory predicts a flow autocorrelation exponent of Alpha - 1.
	MinQty      int64   `json:"min_qty"`
	MaxQty      int64   `json:"max_qty"`
	ParetoAlpha float64 `json:"pareto_alpha"`

	// ChildInterval is the spacing between child orders. ParticipationRate
	// caps each child at that fraction of the market volume observed since the
	// previous child, so the agent tracks activity rather than a wall clock.
	ChildInterval     time.Duration `json:"child_interval"`
	ParticipationRate float64       `json:"participation_rate"`
	MinChildQty       int64         `json:"min_child_qty"`

	// RestInterval separates one parent from the next, so impact measurements
	// do not overlap.
	RestInterval time.Duration `json:"rest_interval"`
	// MaxDuration abandons a parent that cannot complete, recording what it did
	// fill. Without a horizon a parent whose side of the book is empty waits
	// forever, and the agent stops producing measurements entirely.
	MaxDuration time.Duration `json:"max_duration"`
	Seed         int64         `json:"seed"`
}

// validate rejects a configuration that cannot execute, rather than letting a
// zero interval reach the timer factory as a panic.
func (c MetaorderTraderConfig) validate() error {
	switch {
	case c.ChildInterval <= 0:
		return errors.New("multivenue: metaorder child_interval must be positive")
	case c.RestInterval < 0:
		return errors.New("multivenue: metaorder rest_interval must not be negative")
	case c.MinQty <= 0:
		return errors.New("multivenue: metaorder min_qty must be positive")
	case c.MaxQty > 0 && c.MaxQty < c.MinQty:
		return errors.New("multivenue: metaorder max_qty must not be below min_qty")
	case c.ParetoAlpha <= 0:
		return errors.New("multivenue: metaorder pareto_alpha must be positive")
	case c.ParticipationRate < 0:
		return errors.New("multivenue: metaorder participation_rate must not be negative")
	case c.MaxDuration < 0:
		return errors.New("multivenue: metaorder max_duration must not be negative")
	case c.MinChildQty <= 0 && c.ParticipationRate <= 0:
		return errors.New("multivenue: metaorder needs min_child_qty or participation_rate")
	}
	return nil
}

// MetaorderRecord is one completed or abandoned parent order.
type MetaorderRecord struct {
	ID        int    `json:"id"`
	VenueID   string `json:"venue_id"`
	Side      string `json:"side"`
	ParentQty int64  `json:"parent_qty"`
	FilledQty int64  `json:"filled_qty"`

	StartTimestamp int64 `json:"start_timestamp"`
	EndTimestamp   int64 `json:"end_timestamp"`
	StartMid       int64 `json:"start_mid"`
	EndMid         int64 `json:"end_mid"`
	VWAP           int64 `json:"vwap"`

	ChildCount   int   `json:"child_count"`
	MarketVolume int64 `json:"market_volume_during_execution"`

	// SignedImpact is (end mid - start mid) / start mid, signed so that a
	// positive value means the price moved in the direction of the trade.
	SignedImpact float64 `json:"signed_impact"`
	// RealizedParticipation is filled quantity over market volume during
	// execution, the empirical analogue of the configured rate.
	RealizedParticipation float64 `json:"realized_participation"`
	Completed             bool    `json:"completed"`
}

// MetaorderTrader executes a sequence of independent parent orders.
type MetaorderTrader struct {
	*actor.BaseActor
	cfg     MetaorderTraderConfig
	venueID string
	rng     *rand.Rand

	bestBid, bestAsk int64
	askQty, bidQty   int64

	active       bool
	side         exchange.Side
	parentQty    int64
	filledQty    int64
	notional     int64
	childCount   int
	startTS      int64
	startMid     int64
	startVolume  int64
	marketVolume int64
	nextStartTS  int64
	records      []MetaorderRecord
	subscribed   bool
	pendingChild bool
}

func NewMetaorderTrader(id uint64, gw actor.Gateway, venueID string, cfg MetaorderTraderConfig) *MetaorderTrader {
	m := &MetaorderTrader{
		BaseActor: actor.NewBaseActor(id, gw),
		cfg:       cfg,
		venueID:   venueID,
		rng:       rand.New(rand.NewSource(cfg.Seed)),
	}
	m.SetHandler(m)
	m.AddTicker(cfg.ChildInterval, m.onTick)
	return m
}

// Records returns the completed parent orders in execution order.
func (m *MetaorderTrader) Records() []MetaorderRecord { return m.records }

func (m *MetaorderTrader) HandleEvent(_ context.Context, evt *actor.Event) {
	switch evt.Type {
	case actor.EventBookSnapshot:
		e := evt.Data.(actor.BookSnapshotEvent)
		if e.Symbol != m.cfg.Symbol || e.Snapshot == nil {
			return
		}
		m.bestBid, m.bidQty, m.bestAsk, m.askQty = 0, 0, 0, 0
		if len(e.Snapshot.Bids) > 0 {
			m.bestBid, m.bidQty = e.Snapshot.Bids[0].Price, e.Snapshot.Bids[0].VisibleQty
		}
		if len(e.Snapshot.Asks) > 0 {
			m.bestAsk, m.askQty = e.Snapshot.Asks[0].Price, e.Snapshot.Asks[0].VisibleQty
		}
	case actor.EventTrade:
		e := evt.Data.(actor.TradeEvent)
		if e.Symbol == m.cfg.Symbol && e.Trade != nil {
			m.marketVolume += e.Trade.Qty
		}
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		e := evt.Data.(actor.OrderFillEvent)
		if e.Symbol != m.cfg.Symbol {
			return
		}
		m.filledQty += e.Qty
		m.notional += e.Qty * e.Price / m.cfg.BasePrecision
		if e.IsFull {
			m.pendingChild = false
		}
	case actor.EventOrderCancelled:
		// An immediate-or-cancel child that only partly filled is force
		// cancelled for its remainder. Without this the agent would wait
		// forever for a completion event that never comes.
		m.pendingChild = false
	case actor.EventOrderRejected:
		m.pendingChild = false
	}
}

func (m *MetaorderTrader) onTick(now time.Time) {
	if !m.subscribed {
		m.Subscribe(m.cfg.Symbol, exchange.MDSnapshot)
		m.Subscribe(m.cfg.Symbol, exchange.MDTrade)
		m.subscribed = true
		return
	}
	timestamp := now.UnixNano()
	if !m.active {
		if timestamp < m.nextStartTS || m.bestBid <= 0 || m.bestAsk <= 0 {
			return
		}
		m.begin(timestamp)
		return
	}
	m.executeChild(timestamp)
}

// begin starts a parent order. Size is Pareto and sign is a fair coin, both
// independent of the fundamental value.
func (m *MetaorderTrader) begin(timestamp int64) {
	quantity := m.drawParentQty()
	if quantity <= 0 {
		return
	}
	m.active = true
	m.side = exchange.Buy
	if m.rng.Intn(2) == 0 {
		m.side = exchange.Sell
	}
	m.parentQty, m.filledQty, m.notional, m.childCount = quantity, 0, 0, 0
	m.startTS, m.startMid = timestamp, (m.bestBid+m.bestAsk)/2
	m.startVolume = m.marketVolume
}

func (m *MetaorderTrader) drawParentQty() int64 {
	if m.cfg.MinQty <= 0 || m.cfg.ParetoAlpha <= 0 {
		return 0
	}
	// Inverse-transform sampling of a Pareto tail.
	u := m.rng.Float64()
	if u <= 0 || u >= 1 {
		u = 0.5
	}
	scaled := float64(m.cfg.MinQty) * math.Pow(1-u, -1/m.cfg.ParetoAlpha)
	if !finite(scaled) {
		return m.cfg.MinQty
	}
	quantity := int64(scaled)
	if m.cfg.MaxQty > 0 && quantity > m.cfg.MaxQty {
		quantity = m.cfg.MaxQty
	}
	return quantity
}

func (m *MetaorderTrader) executeChild(timestamp int64) {
	if m.pendingChild {
		return
	}
	remaining := m.parentQty - m.filledQty
	if remaining <= 0 {
		m.finish(timestamp, true)
		return
	}
	if m.cfg.MaxDuration > 0 && timestamp-m.startTS >= int64(m.cfg.MaxDuration) {
		m.finish(timestamp, false)
		return
	}
	if m.bestBid <= 0 || m.bestAsk <= 0 {
		return
	}
	child := m.childQty(remaining)
	if child <= 0 {
		return
	}
	// Marketable limit at the touch: aggressive enough to trade now, bounded so
	// a thin book cannot execute the child at an arbitrary price.
	price, available := m.bestAsk, m.askQty
	if m.side == exchange.Sell {
		price, available = m.bestBid, m.bidQty
	}
	if available > 0 && child > available {
		child = available
	}
	if child <= 0 {
		return
	}
	m.SubmitOrderWithTimeInForce(m.cfg.Symbol, m.side, exchange.LimitOrder, price, child, exchange.IOC)
	m.pendingChild = true
	m.childCount++
}

// childQty tracks market activity rather than the clock: each child is a
// configured fraction of the volume traded since the previous one.
func (m *MetaorderTrader) childQty(remaining int64) int64 {
	child := m.cfg.MinChildQty
	if m.cfg.ParticipationRate > 0 {
		recent := m.marketVolume - m.startVolume
		if paced := int64(float64(recent) * m.cfg.ParticipationRate); paced > child {
			child = paced
		}
	}
	if child > remaining {
		child = remaining
	}
	return child
}

func (m *MetaorderTrader) finish(timestamp int64, completed bool) {
	endMid := m.startMid
	if m.bestBid > 0 && m.bestAsk > 0 {
		endMid = (m.bestBid + m.bestAsk) / 2
	}
	record := MetaorderRecord{
		ID: len(m.records) + 1, VenueID: m.venueID, Side: m.side.String(),
		ParentQty: m.parentQty, FilledQty: m.filledQty,
		StartTimestamp: m.startTS, EndTimestamp: timestamp,
		StartMid: m.startMid, EndMid: endMid,
		ChildCount: m.childCount, MarketVolume: m.marketVolume - m.startVolume,
		Completed: completed,
	}
	if m.filledQty > 0 {
		record.VWAP = m.notional * m.cfg.BasePrecision / m.filledQty
	}
	if m.startMid > 0 {
		signed := float64(endMid-m.startMid) / float64(m.startMid)
		if m.side == exchange.Sell {
			signed = -signed
		}
		record.SignedImpact = signed
	}
	if record.MarketVolume > 0 {
		record.RealizedParticipation = float64(m.filledQty) / float64(record.MarketVolume)
	}
	m.records = append(m.records, record)
	m.active = false
	m.nextStartTS = timestamp + int64(m.cfg.RestInterval)
}
