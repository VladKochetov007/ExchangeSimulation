package derivsim

import (
	"context"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	eprice "exchange_sim/price"
)

// OptionValueTakerConfig drives a participant that trades an option when the
// market's premium disagrees with its own valuation.
//
// The population's existing option flow picks a contract at random and takes
// whatever is quoted. That produces volume without producing a market: nothing
// about the price influences whether it trades, so a chain of eleven strikes
// gets flow spread thinly over all of them regardless of where the dealers are
// wrong. This participant reads the quotes and only trades where it thinks the
// dealer is mispriced, which is what makes a strike that is badly quoted the
// strike that trades.
type OptionValueTakerConfig struct {
	Underlying string
	// VolModel is this taker's own view of volatility. Disagreement with the
	// dealer's model is the entire source of its trades.
	VolModel eprice.VolatilityModel
	// EdgeBps is the premium mispricing required before it trades, in basis
	// points of the underlying. Below it the taker holds: crossing the spread
	// for less than the spread is how a participant funds everyone else.
	EdgeBps int64
	// LotQty is the size of one trade and MaxPosition caps net contracts per
	// contract symbol, so the taker cannot accumulate without bound on a view
	// that never converges.
	LotQty      int64
	MaxPosition int64
	Interval    time.Duration
	// BasePrecision converts the underlying price into the units EdgeBps is
	// measured against.
	BasePrecision int64
}

// OptionValueTaker trades options it believes are mispriced.
type OptionValueTaker struct {
	*actor.BaseActor
	cfg        OptionValueTakerConfig
	set        *contractSet
	spotMid    int64
	quotes     map[string]optionTouch
	positions  map[string]int64
	subscribed bool
	// trades counts executed decisions, for reporting.
	trades int
}

type optionTouch struct {
	bid, ask int64
}

// NewOptionValueTaker builds a taker that prices with its own model.
func NewOptionValueTaker(id uint64, gw actor.Gateway, cfg OptionValueTakerConfig) *OptionValueTaker {
	taker := &OptionValueTaker{
		BaseActor: actor.NewBaseActor(id, gw),
		cfg:       cfg,
		set:       newContractSet(cfg.Underlying),
		quotes:    make(map[string]optionTouch),
		positions: make(map[string]int64),
	}
	taker.set.onList = func(c *Contract) {
		if c.Type == "OPTION" {
			taker.Subscribe(c.Symbol, exchange.MDSnapshot)
		}
	}
	taker.set.onSettle = func(c *Contract, _ int64) {
		delete(taker.quotes, c.Symbol)
		delete(taker.positions, c.Symbol)
	}
	taker.set.onFill = taker.onFill
	taker.SetHandler(taker)
	taker.AddTicker(cfg.Interval, taker.onTick)
	return taker
}

// Trades reports how many mispricing decisions reached the book.
func (t *OptionValueTaker) Trades() int { return t.trades }

// Position reports the taker's signed inventory in one contract.
func (t *OptionValueTaker) Position(symbol string) int64 { return t.positions[symbol] }

func (t *OptionValueTaker) HandleEvent(_ context.Context, evt *actor.Event) {
	if evt.Type == actor.EventBookSnapshot {
		e := evt.Data.(actor.BookSnapshotEvent)
		if e.Symbol == t.cfg.Underlying {
			if len(e.Snapshot.Bids) > 0 && len(e.Snapshot.Asks) > 0 {
				t.spotMid = (e.Snapshot.Bids[0].Price + e.Snapshot.Asks[0].Price) / 2
				if observer, ok := t.cfg.VolModel.(eprice.PriceObserver); ok {
					observer.Observe(t.spotMid, e.Timestamp)
				}
			}
			return
		}
		touch := optionTouch{}
		if len(e.Snapshot.Bids) > 0 {
			touch.bid = e.Snapshot.Bids[0].Price
		}
		if len(e.Snapshot.Asks) > 0 {
			touch.ask = e.Snapshot.Asks[0].Price
		}
		t.quotes[e.Symbol] = touch
		return
	}
	t.set.handle(evt)
}

func (t *OptionValueTaker) onFill(sym string, e actor.OrderFillEvent) {
	t.positions[sym] += signedQty(e)
}

// onTick values every listed option and takes the one furthest beyond its edge
// requirement. Trading only the best opportunity per tick is what keeps the
// taker from emptying an entire chain on one volatility view.
func (t *OptionValueTaker) onTick(tick time.Time) {
	if !t.subscribed {
		t.Subscribe(exchange.InstrumentFeedSymbol, exchange.MDInstrument)
		t.Subscribe(t.cfg.Underlying, exchange.MDSnapshot)
		t.subscribed = true
		return
	}
	if t.spotMid <= 0 || t.cfg.LotQty <= 0 || t.cfg.BasePrecision <= 0 {
		return
	}
	now := tick.UnixNano()
	threshold := t.cfg.EdgeBps * t.spotMid / 10_000
	bestSymbol, bestSide, bestEdge := "", exchange.Buy, threshold
	for _, c := range t.set.orderedContracts() {
		if c.Type != "OPTION" {
			continue
		}
		touch, quoted := t.quotes[c.Symbol]
		if !quoted {
			continue
		}
		yearsLeft := float64(c.ExpiryNano-now) / float64(365*24*time.Hour)
		if yearsLeft <= 0 {
			continue
		}
		vol := 0.0
		if t.cfg.VolModel != nil {
			vol = t.cfg.VolModel.Volatility(t.spotMid, c.Strike, yearsLeft, c.IsCall)
		}
		if vol <= 0 {
			continue
		}
		fair := eprice.Black76Premium(t.spotMid, c.Strike, vol, yearsLeft, c.IsCall)
		position := t.positions[c.Symbol]
		// An offer below the taker's value is cheap, and a bid above it is
		// rich. Both are only tradable if the resulting position stays inside
		// the cap, which is checked here rather than after the order is sent.
		if touch.ask > 0 && fair-touch.ask > bestEdge && position+t.cfg.LotQty <= t.cfg.MaxPosition {
			bestSymbol, bestSide, bestEdge = c.Symbol, exchange.Buy, fair-touch.ask
		}
		if touch.bid > 0 && touch.bid-fair > bestEdge && position-t.cfg.LotQty >= -t.cfg.MaxPosition {
			bestSymbol, bestSide, bestEdge = c.Symbol, exchange.Sell, touch.bid-fair
		}
	}
	if bestSymbol == "" {
		return
	}
	reqID := t.SubmitOrder(bestSymbol, bestSide, exchange.Market, 0, t.cfg.LotQty)
	t.set.trackRequest(reqID, bestSymbol)
	t.trades++
}
