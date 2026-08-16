package multivenue

import (
	"context"
	"math"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// ElasticSupplierConfig describes a participant with a downward-sloping demand
// curve: it holds the asset and wants to hold less of it as the price rises.
//
// Every other participant here is either flat in expectation or takes the side
// of the price drift, which leaves market makers holding the mirror of
// the cumulative drift. A supply curve is what absorbs that: as the price
// rises, this participant sells, and it buys back when the price falls. Its
// target depends on the price level itself rather than on the deviation from
// the prevailing price, so it supplies into a drift instead of chasing it.
type ElasticSupplierConfig struct {
	Symbol        string        `json:"symbol"`
	BasePrecision int64         `json:"base_precision"`
	Interval      time.Duration `json:"interval"`
	// ReferencePrice is the price at which the participant wants to hold
	// exactly BaseHolding.
	ReferencePrice int64 `json:"reference_price"`
	BaseHolding    int64 `json:"base_holding"`
	// ElasticityPerPercent is how many base units the target position falls for
	// each percent the price rises above the reference.
	ElasticityPerPercent int64 `json:"elasticity_per_percent"`
	// MaxPosition bounds the signed position in either direction.
	MaxPosition int64 `json:"max_position"`
	// RebalanceLot is the largest quantity traded on one tick, so the
	// participant supplies gradually rather than in one block.
	RebalanceLot int64 `json:"rebalance_lot"`
}

// ElasticSupplier trades toward a price-dependent target position.
type ElasticSupplier struct {
	*actor.BaseActor
	cfg      ElasticSupplierConfig
	bestBid  int64
	bestAsk  int64
	bidQty   int64
	askQty   int64
	position int64

	pending    bool
	subscribed bool
}

func NewElasticSupplier(id uint64, gw actor.Gateway, cfg ElasticSupplierConfig) *ElasticSupplier {
	s := &ElasticSupplier{BaseActor: actor.NewBaseActor(id, gw), cfg: cfg}
	s.SetHandler(s)
	s.AddTicker(cfg.Interval, s.onTick)
	return s
}

// Position is the current signed base position relative to its start.
func (s *ElasticSupplier) Position() int64 { return s.position }

// TargetPosition is the holding the participant wants at a given price.
func (s *ElasticSupplier) TargetPosition(price int64) int64 {
	if price <= 0 || s.cfg.ReferencePrice <= 0 {
		return s.cfg.BaseHolding
	}
	percentAbove := (float64(price)/float64(s.cfg.ReferencePrice) - 1) * 100
	target := float64(s.cfg.BaseHolding) - percentAbove*float64(s.cfg.ElasticityPerPercent)
	if !finite(target) {
		return s.cfg.BaseHolding
	}
	limit := float64(s.cfg.MaxPosition)
	return int64(math.Max(-limit, math.Min(limit, target)))
}

func (s *ElasticSupplier) HandleEvent(_ context.Context, evt *actor.Event) {
	switch evt.Type {
	case actor.EventBookSnapshot:
		e := evt.Data.(actor.BookSnapshotEvent)
		if e.Symbol != s.cfg.Symbol || e.Snapshot == nil {
			return
		}
		s.bestBid, s.bidQty, s.bestAsk, s.askQty = 0, 0, 0, 0
		if len(e.Snapshot.Bids) > 0 {
			s.bestBid, s.bidQty = e.Snapshot.Bids[0].Price, e.Snapshot.Bids[0].VisibleQty
		}
		if len(e.Snapshot.Asks) > 0 {
			s.bestAsk, s.askQty = e.Snapshot.Asks[0].Price, e.Snapshot.Asks[0].VisibleQty
		}
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		e := evt.Data.(actor.OrderFillEvent)
		if e.Symbol != s.cfg.Symbol {
			return
		}
		if e.Side == exchange.Buy {
			s.position += e.Qty
		} else {
			s.position -= e.Qty
		}
		if e.IsFull {
			s.pending = false
		}
	case actor.EventOrderCancelled, actor.EventOrderRejected:
		s.pending = false
	}
}

func (s *ElasticSupplier) onTick(time.Time) {
	if !s.subscribed {
		s.Subscribe(s.cfg.Symbol, exchange.MDSnapshot)
		s.subscribed = true
		return
	}
	if s.pending || s.bestBid <= 0 || s.bestAsk <= 0 || s.cfg.RebalanceLot <= 0 {
		return
	}
	mid := (s.bestBid + s.bestAsk) / 2
	gap := s.TargetPosition(mid) - s.position
	if gap == 0 {
		return
	}
	side, quantity, price, available := exchange.Buy, gap, s.bestAsk, s.askQty
	if gap < 0 {
		side, quantity, price, available = exchange.Sell, -gap, s.bestBid, s.bidQty
	}
	if quantity > s.cfg.RebalanceLot {
		quantity = s.cfg.RebalanceLot
	}
	if available > 0 && quantity > available {
		quantity = available
	}
	if quantity <= 0 {
		return
	}
	s.SubmitOrderWithTimeInForce(s.cfg.Symbol, side, exchange.LimitOrder, price, quantity, exchange.IOC)
	s.pending = true
}
