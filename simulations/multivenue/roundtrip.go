package multivenue

import (
	"context"
	"math/rand"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// RoundTripTraderConfig describes a participant whose demand mean-reverts in
// quantity: it opens a position and closes it again after a holding period.
//
// Random-side noise flow mean-reverts in *price* but not in quantity — each
// order is a fresh coin flip, so the participant's net position is a random
// walk. Market-maker inventory is the mirror of everyone else's net position,
// so a population of pure random-side takers leaves makers holding a position
// that never returns. A round trip returns it by construction.
type RoundTripTraderConfig struct {
	Symbol        string        `json:"symbol"`
	BasePrecision int64         `json:"base_precision"`
	LotQty        int64         `json:"lot_qty"`
	Interval      time.Duration `json:"interval"`
	// HoldDuration is how long a position is carried before it is unwound.
	HoldDuration time.Duration `json:"hold_duration"`
	// OpenProbability is the chance of opening a position on an idle tick.
	OpenProbability float64 `json:"open_probability"`
	Seed            int64   `json:"seed"`
}

// RoundTripTrader opens a position, holds it, then closes it.
type RoundTripTrader struct {
	*actor.BaseActor
	cfg  RoundTripTraderConfig
	rng  *rand.Rand
	book struct{ bid, ask, bidQty, askQty int64 }

	position   int64
	openedAt   int64
	pending    bool
	unwinding  bool
	subscribed bool
	roundTrips int
}

func NewRoundTripTrader(id uint64, gw actor.Gateway, cfg RoundTripTraderConfig) *RoundTripTrader {
	t := &RoundTripTrader{
		BaseActor: actor.NewBaseActor(id, gw),
		cfg:       cfg,
		rng:       rand.New(rand.NewSource(cfg.Seed)),
	}
	t.SetHandler(t)
	t.AddTicker(cfg.Interval, t.onTick)
	return t
}

// RoundTrips is the number of completed open-and-close cycles.
func (t *RoundTripTrader) RoundTrips() int { return t.roundTrips }

// Position is the current signed base position.
func (t *RoundTripTrader) Position() int64 { return t.position }

func (t *RoundTripTrader) HandleEvent(_ context.Context, evt *actor.Event) {
	switch evt.Type {
	case actor.EventBookSnapshot:
		e := evt.Data.(actor.BookSnapshotEvent)
		if e.Symbol != t.cfg.Symbol || e.Snapshot == nil {
			return
		}
		t.book.bid, t.book.bidQty, t.book.ask, t.book.askQty = 0, 0, 0, 0
		if len(e.Snapshot.Bids) > 0 {
			t.book.bid, t.book.bidQty = e.Snapshot.Bids[0].Price, e.Snapshot.Bids[0].VisibleQty
		}
		if len(e.Snapshot.Asks) > 0 {
			t.book.ask, t.book.askQty = e.Snapshot.Asks[0].Price, e.Snapshot.Asks[0].VisibleQty
		}
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		e := evt.Data.(actor.OrderFillEvent)
		if e.Symbol != t.cfg.Symbol {
			return
		}
		if e.Side == exchange.Buy {
			t.position += e.Qty
		} else {
			t.position -= e.Qty
		}
		if e.IsFull {
			t.pending = false
		}
		// Fills arrive after the order is sent, so a completed round trip can
		// only be recognised here, not at submission time.
		if t.position == 0 && t.unwinding {
			t.unwinding = false
			t.roundTrips++
		}
	case actor.EventOrderCancelled, actor.EventOrderRejected:
		t.pending = false
	}
}

func (t *RoundTripTrader) onTick(now time.Time) {
	if !t.subscribed {
		t.Subscribe(t.cfg.Symbol, exchange.MDSnapshot)
		t.subscribed = true
		return
	}
	if t.pending || t.cfg.LotQty <= 0 || t.book.bid <= 0 || t.book.ask <= 0 {
		return
	}
	timestamp := now.UnixNano()
	if t.position == 0 {
		if t.rng.Float64() < t.cfg.OpenProbability {
			side := exchange.Buy
			if t.rng.Intn(2) == 0 {
				side = exchange.Sell
			}
			if t.cross(side, t.cfg.LotQty) {
				t.openedAt = timestamp
			}
		}
		return
	}
	if timestamp-t.openedAt < int64(t.cfg.HoldDuration) {
		return
	}
	// Unwind whatever is actually held, not whatever was intended.
	side, quantity := exchange.Sell, t.position
	if quantity < 0 {
		side, quantity = exchange.Buy, -quantity
	}
	if t.cross(side, quantity) {
		t.unwinding = true
	}
}

// cross sends a marketable order bounded by the visible size at the touch.
func (t *RoundTripTrader) cross(side exchange.Side, quantity int64) bool {
	price, available := t.book.ask, t.book.askQty
	if side == exchange.Sell {
		price, available = t.book.bid, t.book.bidQty
	}
	if available > 0 && quantity > available {
		quantity = available
	}
	if quantity <= 0 {
		return false
	}
	t.SubmitOrderWithTimeInForce(t.cfg.Symbol, side, exchange.LimitOrder, price, quantity, exchange.IOC)
	t.pending = true
	return true
}

// roundTripBalances sizes a round-trip desk's starting balances so it can open
// on either side.
//
// Opening a long costs quote currency. A desk funded like a noise trader holds
// far less quote than one lot is worth, so every buy is rejected for
// INSUFFICIENT_BALANCE while every sell succeeds. The desk then stops being the
// symmetric flow it exists to provide and becomes a persistent seller, which
// biases the price path and leaves makers systematically long — the opposite of
// the inventory mean reversion it was added for.
//
// Lots is how many lots of headroom to fund on each side, so the desk can carry
// several positions at once without its holding period being cut short by its
// own balance.
func roundTripBalances(lotQty, price, basePrecision int64, lots int64, extraAssets []string) map[string]int64 {
	if lots < 1 {
		lots = 1
	}
	base := lotQty * lots
	quote := lotQty * price / basePrecision * lots
	balances := map[string]int64{"ABC": base, "USD": quote}
	for _, asset := range extraAssets {
		balances[asset] = base
	}
	return balances
}
