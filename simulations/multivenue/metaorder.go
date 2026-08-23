package multivenue

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	etypes "exchange_sim/types"
)

// MetaorderTraderConfig describes a participant that executes large parent
// orders by splitting them into children over time.
//
// This is the agent the market impact literature is about. A single large
// order tells you about book depletion; a *split* order executed at a
// controlled participation rate is what produces the square-root impact law
// and, through splitting, the long memory of order-flow signs.
//
// The sign of each parent is drawn independently of the price path
// process on purpose. An informed metaorder would measure the trader's alpha
// rather than the mechanical impact of its own execution, which is the single
// most common way this measurement goes wrong.
type MetaorderTraderConfig struct {
	Symbol        string `json:"symbol"`
	BasePrecision int64  `json:"base_precision"`
	TickSize      int64  `json:"tick_size"`

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
	// MinOrderSize is the venue's minimum order size. A parent whose residual
	// falls below it can never be completed: the exchange rejects every such
	// child for INVALID_QTY, and the agent would resubmit until its horizon
	// expired, spending its whole request budget on orders that cannot fill.
	// Zero leaves the residual to be worked, which is only correct on a venue
	// that has no minimum.
	MinOrderSize int64 `json:"min_order_size"`

	// RestInterval separates one parent from the next, so impact measurements
	// do not overlap.
	RestInterval time.Duration `json:"rest_interval"`
	// MaxSlippageBps prices each child through the touch so it can walk more
	// than one level. Capping a child at the size displayed at the best price
	// makes realised participation a property of the makers' quote size rather
	// than of the configured rate, which stops participation being a treatment
	// at all.
	MaxSlippageBps int64 `json:"max_slippage_bps"`
	// MaxDuration abandons a parent that cannot complete, recording what it did
	// fill. Without a horizon a parent whose side of the book is empty waits
	// forever, and the agent stops producing measurements entirely.
	MaxDuration time.Duration `json:"max_duration"`
	Seed        int64         `json:"seed"`
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
	case c.MinOrderSize < 0:
		return errors.New("multivenue: metaorder min_order_size must not be negative")
	case c.MinChildQty <= 0 && c.ParticipationRate <= 0:
		return errors.New("multivenue: metaorder needs min_child_qty or participation_rate")
	}
	return nil
}

// MetaorderRecord is one completed or abandoned parent order.
type MetaorderRecord struct {
	ID int `json:"id"`
	// TraderID identifies which desk produced the record. IDs restart at one
	// per desk, so without it records from several desks on a venue cannot be
	// told apart, and anything sequential — the gap between one parent ending
	// and the next beginning — is meaningless when read from the merged list.
	TraderID  uint64 `json:"trader_id"`
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
	childVolume  int64
	ownVolume    int64
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
		m.ownVolume += e.Qty
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
		if timestamp < m.nextStartTS {
			return
		}
		if _, available := twoSidedMidpoint(m.bestBid, m.bestAsk); !available {
			return
		}
		m.begin(timestamp)
		return
	}
	m.executeChild(timestamp)
}

// begin starts a parent order. Size is Pareto and sign is a fair coin, both
// independent of the price path.
func (m *MetaorderTrader) begin(timestamp int64) {
	mid, available := twoSidedMidpoint(m.bestBid, m.bestAsk)
	if !available {
		return
	}
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
	m.startTS, m.startMid = timestamp, mid
	m.startVolume, m.childVolume = m.marketVolume, m.externalVolume()
	m.ownVolume = 0
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
	// The venue cannot accept a child this small, so no further child can
	// reduce the residual. Retire the parent rather than resubmitting until
	// the horizon expires.
	if m.cfg.MinOrderSize > 0 && remaining < m.cfg.MinOrderSize {
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
	// Marketable limit priced through the touch: aggressive enough to trade
	// now and to walk several levels, still bounded so a thin book cannot
	// execute the child at an arbitrary price.
	price, available := m.bestAsk, m.askQty
	if m.side == exchange.Sell {
		price, available = m.bestBid, m.bidQty
	}
	if price <= 0 {
		return
	}
	if m.cfg.MaxSlippageBps > 0 {
		if room, ok := etypes.TryMulBps(price, m.cfg.MaxSlippageBps); ok {
			if m.side == exchange.Buy {
				price += room
			} else {
				price -= room
			}
		}
	} else if available > 0 && child > available {
		// Without a slippage allowance the child can only take what is
		// displayed at the best price.
		child = available
	}
	if price <= 0 || child <= 0 {
		return
	}
	if tick := m.cfg.TickSize; tick > 0 {
		if m.side == exchange.Buy {
			price = (price + tick - 1) / tick * tick
		} else {
			price = price / tick * tick
		}
	}
	m.childVolume = m.externalVolume()
	m.SubmitOrderWithTimeInForce(m.cfg.Symbol, m.side, exchange.LimitOrder, price, child, exchange.IOC)
	m.pendingChild = true
	m.childCount++
}

// childQty tracks market activity rather than the clock: each child is a
// configured fraction of the volume traded since the previous child.
//
// Measuring from the start of the parent instead would let the allowance grow
// with the whole parent's history, so the realised participation rate ran far
// above the configured one — 0.66 against a configured 0.02 — and the agent
// became most of the market it was supposed to be measuring.
func (m *MetaorderTrader) childQty(remaining int64) int64 {
	child := m.cfg.MinChildQty
	if m.cfg.ParticipationRate > 0 {
		// Pace against volume traded by everyone else. The trade feed includes
		// this agent's own fills, so pacing against total volume is
		// self-feeding: each child enlarges the allowance for the next one and
		// the realised participation runs to one regardless of the setting.
		recent := m.externalVolume() - m.childVolume
		if paced := int64(float64(recent) * m.cfg.ParticipationRate); paced > child {
			child = paced
		}
	}
	if child > remaining {
		child = remaining
	}
	return child
}

// externalVolume is market volume excluding this agent's own executions.
func (m *MetaorderTrader) externalVolume() int64 {
	if external := m.marketVolume - m.ownVolume; external > 0 {
		return external
	}
	return 0
}

func (m *MetaorderTrader) finish(timestamp int64, completed bool) {
	endMid := m.startMid
	if current, available := twoSidedMidpoint(m.bestBid, m.bestAsk); available {
		endMid = current
	}
	record := MetaorderRecord{
		ID: len(m.records) + 1, TraderID: m.ID(), VenueID: m.venueID, Side: m.side.String(),
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
	// Participation is measured against volume traded by everyone else, so a
	// dominant agent shows a rate above one instead of being flattered toward
	// one by counting its own fills in the denominator.
	if external := record.MarketVolume - m.filledQty; external > 0 {
		record.RealizedParticipation = float64(m.filledQty) / float64(external)
	}
	m.records = append(m.records, record)
	m.active = false
	m.nextStartTS = timestamp + int64(m.cfg.RestInterval)
}
