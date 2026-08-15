// Package multivenue builds deterministic, venue-local market ecologies for
// experiments that later add explicit cross-venue routing and execution risk.
package multivenue

import (
	"context"
	"math"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// StoikovInputs are expressed in human units rather than fixed-point units.
// Variance is price^2 per second, inventory is base units, risk aversion has
// reciprocal quote-price units per base unit, and FillDecay has reciprocal
// quote-price units. The finite InventoryHorizon is a rolling risk horizon,
// not a claim that perpetual trading has a terminal time.
type StoikovInputs struct {
	Forward           float64
	Inventory         float64
	VariancePerSecond float64
	RiskAversion      float64
	FillDecay         float64
	InventoryHorizon  time.Duration
	MinHalfSpread     float64
}

// StoikovQuote is the unrounded reservation price and one-level quote pair.
type StoikovQuote struct {
	Reservation float64
	HalfSpread  float64
	Bid         float64
	Ask         float64
}

// CalculateStoikovQuote implements the finite-horizon Avellaneda-Stoikov
// control used for a linear instrument:
//
//	r = F - q * gamma * sigma^2 * tau
//	spread = gamma * sigma^2 * tau + 2/gamma * log(1 + gamma/kappa)
//
// The exchange matcher supplies actual queueing and fills; the exponential
// fill-distance curve is therefore a control approximation that must be
// calibrated and tested, not an assertion about the simulator.
func CalculateStoikovQuote(in StoikovInputs) (StoikovQuote, bool) {
	if !finite(in.Forward) || !finite(in.Inventory) || !finite(in.VariancePerSecond) ||
		!finite(in.RiskAversion) || !finite(in.FillDecay) || !finite(in.MinHalfSpread) ||
		in.Forward <= 0 || in.VariancePerSecond < 0 || in.RiskAversion <= 0 ||
		in.FillDecay <= 0 || in.InventoryHorizon <= 0 || in.MinHalfSpread < 0 {
		return StoikovQuote{}, false
	}
	tau := in.InventoryHorizon.Seconds()
	riskTerm := in.RiskAversion * in.VariancePerSecond * tau
	reservation := in.Forward - in.Inventory*riskTerm
	spread := riskTerm + 2/in.RiskAversion*math.Log1p(in.RiskAversion/in.FillDecay)
	halfSpread := math.Max(spread/2, in.MinHalfSpread)
	quote := StoikovQuote{
		Reservation: reservation,
		HalfSpread:  halfSpread,
		Bid:         reservation - halfSpread,
		Ask:         reservation + halfSpread,
	}
	if !finite(quote.Reservation) || !finite(quote.HalfSpread) || !finite(quote.Bid) || !finite(quote.Ask) || quote.Bid <= 0 || quote.Ask <= quote.Bid {
		return StoikovQuote{}, false
	}
	return quote, true
}

// StoikovMMConfig describes one one-level linear market maker. ReferenceSymbol
// may differ from Symbol, which lets a perp or dated future quote against the
// local spot book. For spot it is normally the same symbol.
type StoikovMMConfig struct {
	Symbol          string
	ReferenceSymbol string
	BootstrapPrice  int64
	BasePrecision   int64
	QuotePrecision  int64
	TickSize        int64
	QuoteQty        int64
	QuoteInterval   time.Duration

	// The control parameters are relative (scale free): variance is of log
	// returns, and risk aversion and fill decay are dimensionless. The maker
	// converts them to the absolute quote units the Avellaneda-Stoikov formula
	// expects using the current forward, so the same parameters describe the
	// same behaviour on any book regardless of its price scale or quote
	// currency.
	//
	// Absolute-price variance estimated from the maker's own reference mid is
	// not usable here: a price move raises the variance estimate, which widens
	// the quote, which produces a larger move. That loop diverged
	// superexponentially in long runs.
	VolatilityHalfLife       time.Duration
	InitialLogVariancePerSec float64
	InventoryHorizon         time.Duration
	RelativeRiskAversion     float64
	RelativeFillDecay        float64
	MinHalfSpreadTicks       int64
}

type quoteSide bool

const (
	stoikovBid quoteSide = true
	stoikovAsk quoteSide = false
)

// StoikovMarketMaker executes the control law against the exchange's actual
// book. It measures realised variance from reference-book midpoint changes;
// it does not fabricate Poisson fills or a queue-priority advantage.
type StoikovMarketMaker struct {
	*actor.BaseActor
	cfg StoikovMMConfig

	forward            int64
	lastForward        int64
	lastForwardTS      int64
	logVariancePerSec  float64
	inventory          int64
	bidID, askID       uint64
	bidPrice, askPrice int64
	pending            map[uint64]quoteSide
	subscribed         bool
}

func NewStoikovMarketMaker(id uint64, gw actor.Gateway, cfg StoikovMMConfig) *StoikovMarketMaker {
	mm := &StoikovMarketMaker{
		BaseActor:      actor.NewBaseActor(id, gw),
		cfg:            cfg,
		forward:        cfg.BootstrapPrice,
		logVariancePerSec: cfg.InitialLogVariancePerSec,
		pending:        make(map[uint64]quoteSide),
	}
	mm.SetHandler(mm)
	mm.AddTicker(cfg.QuoteInterval, mm.onTick)
	return mm
}

func (mm *StoikovMarketMaker) Inventory() int64           { return mm.inventory }
func (mm *StoikovMarketMaker) LogVariancePerSecond() float64 { return mm.logVariancePerSec }

func (mm *StoikovMarketMaker) HandleEvent(_ context.Context, evt *actor.Event) {
	switch evt.Type {
	case actor.EventBookSnapshot:
		mm.onSnapshot(evt.Data.(actor.BookSnapshotEvent))
	case actor.EventOrderAccepted:
		mm.onAccepted(evt.Data.(actor.OrderAcceptedEvent))
	case actor.EventOrderRejected:
		mm.onRejected(evt.Data.(actor.OrderRejectedEvent))
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		mm.onFill(evt.Data.(actor.OrderFillEvent))
	case actor.EventOrderCancelled:
		mm.onCancelled(evt.Data.(actor.OrderCancelledEvent))
	}
}

func (mm *StoikovMarketMaker) onSnapshot(e actor.BookSnapshotEvent) {
	if e.Symbol != mm.cfg.ReferenceSymbol || len(e.Snapshot.Bids) == 0 || len(e.Snapshot.Asks) == 0 {
		return
	}
	mid := (e.Snapshot.Bids[0].Price + e.Snapshot.Asks[0].Price) / 2
	if mid <= 0 {
		return
	}
	if mm.lastForward > 0 && e.Timestamp > mm.lastForwardTS {
		dt := float64(e.Timestamp-mm.lastForwardTS) / float64(time.Second)
		// Log returns keep the variance estimate scale free, so it measures how
		// volatile the book is rather than how large its prices are.
		logReturn := math.Log(float64(mid) / float64(mm.lastForward))
		if dt > 0 && finite(logReturn) {
			instantVariance := logReturn * logReturn / dt
			alpha := ewmaAlpha(dt, mm.cfg.VolatilityHalfLife)
			mm.logVariancePerSec = (1-alpha)*mm.logVariancePerSec + alpha*instantVariance
		}
	}
	mm.forward = mid
	mm.lastForward = mid
	mm.lastForwardTS = e.Timestamp
}

func (mm *StoikovMarketMaker) onAccepted(e actor.OrderAcceptedEvent) {
	side, ok := mm.pending[e.RequestID]
	if !ok {
		mm.CancelOrder(e.OrderID)
		return
	}
	delete(mm.pending, e.RequestID)
	if side == stoikovBid {
		mm.bidID = e.OrderID
	} else {
		mm.askID = e.OrderID
	}
}

func (mm *StoikovMarketMaker) onRejected(e actor.OrderRejectedEvent) {
	side, ok := mm.pending[e.RequestID]
	if !ok {
		return
	}
	delete(mm.pending, e.RequestID)
	if side == stoikovBid {
		mm.bidPrice = 0
	} else {
		mm.askPrice = 0
	}
}

func (mm *StoikovMarketMaker) onFill(e actor.OrderFillEvent) {
	if e.Symbol != mm.cfg.Symbol {
		return
	}
	if e.Side == exchange.Buy {
		mm.inventory += e.Qty
	} else {
		mm.inventory -= e.Qty
	}
	if !e.IsFull {
		return
	}
	if e.OrderID == mm.bidID {
		mm.bidID, mm.bidPrice = 0, 0
	}
	if e.OrderID == mm.askID {
		mm.askID, mm.askPrice = 0, 0
	}
}

func (mm *StoikovMarketMaker) onCancelled(e actor.OrderCancelledEvent) {
	if e.OrderID == mm.bidID {
		mm.bidID, mm.bidPrice = 0, 0
	}
	if e.OrderID == mm.askID {
		mm.askID, mm.askPrice = 0, 0
	}
}

func (mm *StoikovMarketMaker) onTick(_ time.Time) {
	if !mm.subscribed {
		mm.Subscribe(mm.cfg.ReferenceSymbol, exchange.MDSnapshot)
		if mm.cfg.ReferenceSymbol != mm.cfg.Symbol {
			mm.Subscribe(mm.cfg.Symbol, exchange.MDSnapshot)
		}
		mm.subscribed = true
		return
	}
	if len(mm.pending) != 0 || mm.cfg.BasePrecision <= 0 || mm.cfg.QuotePrecision <= 0 || mm.cfg.TickSize <= 0 || mm.cfg.QuoteQty <= 0 {
		return
	}
	forward := mm.forward
	if forward <= 0 {
		forward = mm.cfg.BootstrapPrice
	}
	// Convert the relative parameters into the absolute quote units the
	// formula expects. Variance is quote-price^2 and both risk aversion and
	// fill decay are reciprocal quote-price, so with forward F:
	//   sigma^2 = logVariance * F^2, gamma = gammaRel / F, kappa = kappaRel / F.
	// Every price-scaled factor then cancels and the resulting quote is a fixed
	// fraction of F for a given inventory fraction.
	forwardPrice := float64(forward) / float64(mm.cfg.QuotePrecision)
	if !finite(forwardPrice) || forwardPrice <= 0 {
		return
	}
	quote, ok := CalculateStoikovQuote(StoikovInputs{
		Forward:           forwardPrice,
		Inventory:         float64(mm.inventory) / float64(mm.cfg.BasePrecision),
		VariancePerSecond: mm.logVariancePerSec * forwardPrice * forwardPrice,
		RiskAversion:      mm.cfg.RelativeRiskAversion / forwardPrice,
		FillDecay:         mm.cfg.RelativeFillDecay / forwardPrice,
		InventoryHorizon:  mm.cfg.InventoryHorizon,
		MinHalfSpread:     float64(mm.cfg.MinHalfSpreadTicks*mm.cfg.TickSize) / float64(mm.cfg.QuotePrecision),
	})
	if !ok {
		return
	}
	bid, okBid := quoteToBidTicks(quote.Bid, mm.cfg.QuotePrecision, mm.cfg.TickSize)
	ask, okAsk := quoteToAskTicks(quote.Ask, mm.cfg.QuotePrecision, mm.cfg.TickSize)
	if !okBid || !okAsk || bid <= 0 || ask <= bid {
		return
	}
	if bid == mm.bidPrice && ask == mm.askPrice && mm.bidID != 0 && mm.askID != 0 {
		return
	}
	if mm.bidID != 0 {
		mm.CancelOrder(mm.bidID)
		mm.bidID = 0
	}
	if mm.askID != 0 {
		mm.CancelOrder(mm.askID)
		mm.askID = 0
	}
	mm.bidPrice, mm.askPrice = bid, ask
	bidRequest := mm.SubmitOrder(mm.cfg.Symbol, exchange.Buy, exchange.LimitOrder, bid, mm.cfg.QuoteQty)
	mm.pending[bidRequest] = stoikovBid
	askRequest := mm.SubmitOrder(mm.cfg.Symbol, exchange.Sell, exchange.LimitOrder, ask, mm.cfg.QuoteQty)
	mm.pending[askRequest] = stoikovAsk
}

func ewmaAlpha(dt float64, halfLife time.Duration) float64 {
	if halfLife <= 0 {
		return 1
	}
	return 1 - math.Exp(-math.Ln2*dt/halfLife.Seconds())
}

func quoteToBidTicks(price float64, precision, tick int64) (int64, bool) {
	if !finite(price) || precision <= 0 || tick <= 0 || price <= 0 || price > float64(math.MaxInt64)/float64(precision) {
		return 0, false
	}
	raw := int64(math.Floor(price * float64(precision)))
	return raw / tick * tick, true
}

func quoteToAskTicks(price float64, precision, tick int64) (int64, bool) {
	if !finite(price) || precision <= 0 || tick <= 0 || price <= 0 || price > float64(math.MaxInt64)/float64(precision) {
		return 0, false
	}
	raw := int64(math.Ceil(price * float64(precision)))
	if raw > math.MaxInt64-(tick-1) {
		return 0, false
	}
	return ((raw + tick - 1) / tick) * tick, true
}

func finite(x float64) bool {
	return !math.IsNaN(x) && !math.IsInf(x, 0)
}
