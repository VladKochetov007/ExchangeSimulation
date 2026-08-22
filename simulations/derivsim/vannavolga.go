package derivsim

import (
	"context"
	"math"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	eprice "exchange_sim/price"
)

// VannaVolgaHedgerConfig drives a desk that hedges an option book in options
// rather than only in the underlying.
//
// A delta hedge leaves a book exposed to everything that is not delta. The two
// exposures that cost a real desk money are vanna, which is how its delta
// moves when volatility moves, and volga, which is how its vega moves when
// volatility moves. Neither can be hedged in the underlying at any size,
// because the underlying has no vega. This desk buys and sells options to flatten
// them, which is the same hedge a vanna-volga pricer assumes is available.
type VannaVolgaHedgerConfig struct {
	Underlying string
	// VolModel prices the risk being hedged. A desk hedging with the wrong
	// volatility flattens the wrong number, so this is the desk's own view
	// rather than the venue's.
	VolModel eprice.VolatilityModel
	// VegaTolerance, VannaTolerance and VolgaTolerance are the exposures the
	// desk tolerates before trading. A desk with zero tolerance trades on every
	// tick and pays the spread for exposures it would have carried harmlessly.
	VegaTolerance  float64
	VannaTolerance float64
	VolgaTolerance float64
	// LotQty is the size of one hedging trade and MaxContracts caps the
	// position the desk may build in any one contract while hedging.
	LotQty       int64
	MaxContracts int64
	Interval     time.Duration
	// BasePrecision converts contract quantities into the units the greeks are
	// expressed in.
	BasePrecision int64
	// Exposure reports the book being hedged. It is a callback because the
	// risk belongs to whoever owns the option inventory, which may be another
	// participant entirely: a desk can be hired to hedge a book it does not own.
	Exposure func() []ContractExposure
}

// ContractExposure is one option position the hedger has to neutralise.
type ContractExposure struct {
	Symbol     string
	Strike     int64
	IsCall     bool
	ExpiryNano int64
	// Position is signed, in base units.
	Position int64
}

// VannaVolgaHedger flattens vega, vanna and volga by trading options.
type VannaVolgaHedger struct {
	*actor.BaseActor
	cfg        VannaVolgaHedgerConfig
	set        *contractSet
	spotMid    int64
	positions  map[string]int64
	subscribed bool
	hedges     int
}

// NewVannaVolgaHedger builds the desk.
func NewVannaVolgaHedger(id uint64, gw actor.Gateway, cfg VannaVolgaHedgerConfig) *VannaVolgaHedger {
	hedger := &VannaVolgaHedger{
		BaseActor: actor.NewBaseActor(id, gw),
		cfg:       cfg,
		set:       newContractSet(cfg.Underlying),
		positions: make(map[string]int64),
	}
	hedger.set.onFill = hedger.onFill
	hedger.set.onSettle = func(c *Contract, _ int64) { delete(hedger.positions, c.Symbol) }
	hedger.SetHandler(hedger)
	hedger.AddTicker(cfg.Interval, hedger.onTick)
	return hedger
}

// onFill records the desk's own inventory, which counts against the risk it
// is flattening: a hedge already traded is risk already removed.
func (h *VannaVolgaHedger) onFill(symbol string, e actor.OrderFillEvent) {
	h.positions[symbol] += signedQty(e)
}

// Hedges reports how many hedging trades the desk has sent.
func (h *VannaVolgaHedger) Hedges() int { return h.hedges }

// Position reports the desk's own signed inventory in a contract.
func (h *VannaVolgaHedger) Position(symbol string) int64 { return h.positions[symbol] }

func (h *VannaVolgaHedger) HandleEvent(_ context.Context, evt *actor.Event) {
	if evt.Type == actor.EventBookSnapshot {
		e := evt.Data.(actor.BookSnapshotEvent)
		if e.Symbol == h.cfg.Underlying && len(e.Snapshot.Bids) > 0 && len(e.Snapshot.Asks) > 0 {
			h.spotMid = (e.Snapshot.Bids[0].Price + e.Snapshot.Asks[0].Price) / 2
			if observer, ok := h.cfg.VolModel.(eprice.PriceObserver); ok {
				observer.Observe(h.spotMid, e.Timestamp)
			}
		}
		return
	}
	h.set.handle(evt)
}

// BookRisk is the aggregate second-order exposure of an option book.
type BookRisk struct {
	Vega  float64
	Vanna float64
	Volga float64
}

// exceeds reports whether any exposure is outside its tolerance.
func (r BookRisk) exceeds(cfg VannaVolgaHedgerConfig) bool {
	return math.Abs(r.Vega) > cfg.VegaTolerance ||
		math.Abs(r.Vanna) > cfg.VannaTolerance ||
		math.Abs(r.Volga) > cfg.VolgaTolerance
}

// MeasureRisk aggregates the book's vega, vanna and volga at one instant,
// including whatever the desk has already traded to hedge it.
func (h *VannaVolgaHedger) MeasureRisk(nano int64) BookRisk {
	risk := BookRisk{}
	if h.spotMid <= 0 || h.cfg.BasePrecision <= 0 {
		return risk
	}
	add := func(strike int64, isCall bool, expiryNano, position int64) {
		if position == 0 {
			return
		}
		years := float64(expiryNano-nano) / float64(365*24*time.Hour)
		if years <= 0 {
			return
		}
		vol := 0.0
		if h.cfg.VolModel != nil {
			vol = h.cfg.VolModel.Volatility(h.spotMid, strike, years, isCall)
		}
		if vol <= 0 {
			return
		}
		contracts := float64(position) / float64(h.cfg.BasePrecision)
		risk.Vega += contracts * eprice.Black76Vega(h.spotMid, strike, vol, years)
		risk.Vanna += contracts * eprice.Black76Vanna(h.spotMid, strike, vol, years)
		risk.Volga += contracts * eprice.Black76Volga(h.spotMid, strike, vol, years)
	}
	if h.cfg.Exposure != nil {
		for _, exposure := range h.cfg.Exposure() {
			add(exposure.Strike, exposure.IsCall, exposure.ExpiryNano, exposure.Position)
		}
	}
	// Symbol order, not map order. Vega, vanna and volga accumulate in
	// float64, and floating-point addition is not associative: summing the
	// same positions in a different order gives a slightly different risk,
	// which changes which contract this desk decides to trade. That is a
	// nondeterministic input to the model, and it grows with the size of the
	// chain -- runs of one seed agreed for hours and then diverged here.
	for _, c := range h.set.orderedContracts() {
		if c.Type != "OPTION" {
			continue
		}
		if position, held := h.positions[c.Symbol]; held {
			add(c.Strike, c.IsCall, c.ExpiryNano, position)
		}
	}
	return risk
}

// onTick trades the one contract that removes most of the book's excess risk.
//
// Hedging one contract per tick rather than solving the whole system is the
// honest version of what a desk does: it improves the worst exposure with the
// instrument that moves it most per unit of premium paid, and revisits.
func (h *VannaVolgaHedger) onTick(tick time.Time) {
	if !h.subscribed {
		h.Subscribe(exchange.InstrumentFeedSymbol, exchange.MDInstrument)
		h.Subscribe(h.cfg.Underlying, exchange.MDSnapshot)
		h.subscribed = true
		return
	}
	if h.spotMid <= 0 || h.cfg.LotQty <= 0 || h.cfg.BasePrecision <= 0 {
		return
	}
	now := tick.UnixNano()
	risk := h.MeasureRisk(now)
	if !risk.exceeds(h.cfg) {
		return
	}
	bestSymbol, bestSide, bestImprovement := "", exchange.Buy, 0.0
	lots := float64(h.cfg.LotQty) / float64(h.cfg.BasePrecision)
	for _, c := range h.set.orderedContracts() {
		if c.Type != "OPTION" {
			continue
		}
		years := float64(c.ExpiryNano-now) / float64(365*24*time.Hour)
		if years <= 0 {
			continue
		}
		vol := 0.0
		if h.cfg.VolModel != nil {
			vol = h.cfg.VolModel.Volatility(h.spotMid, c.Strike, years, c.IsCall)
		}
		if vol <= 0 {
			continue
		}
		unit := BookRisk{
			Vega:  lots * eprice.Black76Vega(h.spotMid, c.Strike, vol, years),
			Vanna: lots * eprice.Black76Vanna(h.spotMid, c.Strike, vol, years),
			Volga: lots * eprice.Black76Volga(h.spotMid, c.Strike, vol, years),
		}
		for _, side := range []exchange.Side{exchange.Buy, exchange.Sell} {
			sign := 1.0
			signedLot := h.cfg.LotQty
			if side == exchange.Sell {
				sign, signedLot = -1, -h.cfg.LotQty
			}
			if position := h.positions[c.Symbol] + signedLot; position > h.cfg.MaxContracts || position < -h.cfg.MaxContracts {
				continue
			}
			after := BookRisk{
				Vega:  risk.Vega + sign*unit.Vega,
				Vanna: risk.Vanna + sign*unit.Vanna,
				Volga: risk.Volga + sign*unit.Volga,
			}
			if improvement := h.severity(risk) - h.severity(after); improvement > bestImprovement {
				bestSymbol, bestSide, bestImprovement = c.Symbol, side, improvement
			}
		}
	}
	if bestSymbol == "" {
		return
	}
	reqID := h.SubmitOrder(bestSymbol, bestSide, exchange.Market, 0, h.cfg.LotQty)
	h.set.trackRequest(reqID, bestSymbol)
	h.hedges++
}

// severity scores a risk state in units of its own tolerances, so that three
// exposures measured in incomparable units can be traded off against each
// other. A desk that summed them raw would chase whichever happens to be
// numerically largest.
func (h *VannaVolgaHedger) severity(risk BookRisk) float64 {
	score := 0.0
	if h.cfg.VegaTolerance > 0 {
		score += math.Abs(risk.Vega) / h.cfg.VegaTolerance
	}
	if h.cfg.VannaTolerance > 0 {
		score += math.Abs(risk.Vanna) / h.cfg.VannaTolerance
	}
	if h.cfg.VolgaTolerance > 0 {
		score += math.Abs(risk.Volga) / h.cfg.VolgaTolerance
	}
	return score
}
