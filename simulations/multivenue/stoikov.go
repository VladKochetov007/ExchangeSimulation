// Package multivenue builds deterministic, venue-local market ecologies for
// experiments that later add explicit cross-venue routing and execution risk.
package multivenue

import (
	"context"
	"math"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	etypes "exchange_sim/types"
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
	// QuoteSizeVolElasticity shrinks quoted size as the maker's own volatility
	// estimate rises above the level it started from. Zero quotes a constant
	// size in every state, which is what the reference population did.
	//
	// Constant depth denies the market the feedback that produces volatility
	// clustering: a burst of trading meets the same depth as a quiet period, so
	// a large move cannot make the next one more likely. Withdrawing size under
	// stress lets an order walk further exactly when the estimate is already
	// high, which is the loop real books have.
	// ForwardHalfLife smooths the maker's view of the reference book instead of
	// taking its instantaneous midpoint.
	//
	// Zero means the maker's forward is whatever the book last printed, so a
	// sweep moves its view permanently and it requotes around the new level.
	// Impact is then permanent by construction, which is why every parameter
	// that widens the return tails also makes the level slide: the only way to
	// make a return large is to move the level for good. A maker that forms its
	// view over time quotes back toward where it believed the price was, so a
	// sweep decays as it replenishes.
	ForwardHalfLife time.Duration

	QuoteSizeVolElasticity float64
	// MinQuoteSizeFraction floors the withdrawal, so a volatility spike thins
	// the book rather than emptying it. Zero means a tenth.
	MinQuoteSizeFraction float64
	QuoteInterval        time.Duration

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
	// MaxLogVarianceMultiple caps the estimate at this multiple of its initial
	// value. The Avellaneda-Stoikov derivation treats volatility as exogenous;
	// a maker measuring a book it dominates does not have an exogenous
	// estimate, so the cap bounds how far a feedback episode can travel before
	// the estimate mean-reverts.
	MaxLogVarianceMultiple float64
	// VolatilitySampleInterval is the minimum spacing between the trade prices
	// used for the variance estimate. Consecutive prints alternate between the
	// bid and the ask, so sampling every print measures the maker's own spread
	// rather than the volatility of the asset: in a 90-minute run, one-second
	// sampling gave 1.57e-2 against 4.15e-3 for the midpoint, while the two
	// agreed by 30-second sampling. Because a wider estimate widens the quote,
	// that bias is another feedback path.
	VolatilitySampleInterval time.Duration
	InventoryHorizon         time.Duration
	RelativeRiskAversion     float64
	RelativeFillDecay        float64
	MinHalfSpreadTicks       int64
	// AnchorToIndex quotes around the venue's published index instead of the
	// maker's own book midpoint. Quoting around its own midpoint makes the
	// midpoint reproduce itself: the price becomes a self-referential random
	// walk with no restoring force, and it wanders arbitrarily far from value
	// once informed participants reach their inventory bounds.
	// InventoryLimit is the position the maker treats as its full risk budget.
	// Inventory enters the control as a fraction of it, clamped to one, so the
	// skew is bounded and calibratable: risk aversion sets the shift at the
	// limit rather than a shift per unit that an unbounded position multiplies
	// into an arbitrary number.
	InventoryLimit int64
	// InventorySkewBps, when positive, sets the reservation-price shift at the
	// full inventory limit directly, in basis points, instead of deriving it
	// from risk aversion times variance times horizon.
	//
	// The textbook term couples the skew to the volatility estimate, and in
	// this market that estimate moves by two and a half orders of magnitude
	// between its floor and its cap. The skew is then either negligible — 0.6
	// basis points at 100 units of inventory, so makers accumulate without
	// limit — or large enough to move the price itself and diverge. Setting the
	// shift at the limit makes the control calibratable and bounded.
	InventorySkewBps int64
	// HedgeInterval, when positive, gives the hedge its own cadence instead of
	// running it inside the quote cycle.
	//
	// Coupling the two is not neutral in either direction. Hedging only when
	// the maker requotes means a calm market stops its risk management while it
	// is still being filled. Hedging on every tick removes the rate limit the
	// quote cycle was providing, and the maker's own marketable hedges become
	// the dominant flow on the hedge instrument: measured over eight hours that
	// took the median basis from 2.1 to 830 basis points. The interval is the
	// dial between those two failures and has to be chosen, not inherited.
	HedgeInterval time.Duration
	// HedgeSymbol, when set, is where the maker offsets the inventory it takes
	// on in Symbol. A real market maker does not run flat by holding spot: it
	// quotes one instrument and moves the resulting delta to another, which is
	// why its quoting is not limited by how much of the asset it owns.
	HedgeSymbol string
	// HedgeBandQty is how far net delta may drift before it is offset.
	HedgeBandQty int64
	// HedgeTickSize is the hedge instrument's tick. A price that is not a
	// multiple of it is rejected outright, and pricing through the touch is
	// exactly what knocks a price off the grid.
	HedgeTickSize int64
	// HedgeSlippageBps prices the hedge through the touch the maker last saw.
	// Quoting exactly at a remembered touch does not cross: the hedge venue
	// requotes between the snapshot and the order's arrival, so the order rests
	// behind the market and expires unfilled.
	HedgeSlippageBps int64
	AnchorToIndex    bool
	// IndexWeight blends the index with the book midpoint, 1 meaning the index
	// alone. A partial weight lets the book discover price while still being
	// tethered.
	IndexWeight float64
	// UseLocalReferenceCache makes this maker quote from its own copied public
	// book observation. It is the first V2-1 single-feed slice; it never reads
	// an exchange book or a shared composite. Cross-venue composites require a
	// later explicit multi-frontier evidence extension.
	UseLocalReferenceCache    bool
	LocalReferenceSourceVenue string
	// RequoteBps suppresses a replacement until the target has moved this far
	// from what is already resting. Without it a maker replaces its quotes on
	// essentially every step, which synchronises the whole population: measured
	// at 97.6 percent of steps having every maker cancel at once. A threshold
	// lets each maker's own inventory and volatility state decide when it moves,
	// so the population desynchronises.
	RequoteBps int64

	// SubmitBeforeCancel replaces quotes without ever leaving the book empty.
	// The exchange cancels a client's own crossing quotes on rest, so the
	// momentary overlap cannot self-trade.
	SubmitBeforeCancel bool
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
	indexPrice         int64
	localReference     *LocalBookCache
	forwardAt          int64
	lastForward        int64
	lastForwardTS      int64
	logVariancePerSec  float64
	inventory          int64
	bidID, askID       uint64
	bidPrice, askPrice int64
	hedgePosition      int64
	hedgePending       bool
	hedgeRequest       uint64
	hedgeAttempts      int
	hedgeFills         int
	hedgeRejects       int
	hedgeLastReject    exchange.RejectReason
	hedgeLastQty       int64
	hedgeBid, hedgeAsk int64
	hedgeBidQty        int64
	hedgeAskQty        int64
	hedgeBookSeen      int
	hedgeBookTwoSided  int
	pending            map[uint64]quoteSide
	subscribed         bool
}

func NewStoikovMarketMaker(id uint64, gw actor.Gateway, cfg StoikovMMConfig) *StoikovMarketMaker {
	mm := &StoikovMarketMaker{
		BaseActor:         actor.NewBaseActor(id, gw),
		cfg:               cfg,
		forward:           cfg.BootstrapPrice,
		logVariancePerSec: cfg.InitialLogVariancePerSec,
		pending:           make(map[uint64]quoteSide),
	}
	if cfg.UseLocalReferenceCache {
		mm.localReference = NewLocalBookCache(cfg.LocalReferenceSourceVenue, cfg.ReferenceSymbol)
	}
	mm.SetHandler(mm)
	mm.AddTicker(cfg.QuoteInterval, mm.onTick)
	if cfg.HedgeInterval > 0 {
		mm.AddTicker(cfg.HedgeInterval, mm.onHedgeTick)
	}
	return mm
}

func (mm *StoikovMarketMaker) Inventory() int64              { return mm.inventory }
func (mm *StoikovMarketMaker) LogVariancePerSecond() float64 { return mm.logVariancePerSec }

// LocalReferenceView exposes a value copy for scenario activation checks. It
// never returns a mutable exchange/book object to the controller or actor.
func (mm *StoikovMarketMaker) LocalReferenceView() (LocalBookView, bool) {
	if mm == nil {
		return LocalBookView{}, false
	}
	return mm.localReference.View()
}

func (mm *StoikovMarketMaker) HandleEvent(_ context.Context, evt *actor.Event) {
	switch evt.Type {
	case actor.EventBookSnapshot:
		mm.onSnapshot(evt.Data.(actor.BookSnapshotEvent))
	case actor.EventTrade:
		mm.onTrade(evt.Data.(actor.TradeEvent))
	case actor.EventIndex:
		e := evt.Data.(actor.IndexEvent)
		if e.Symbol == mm.cfg.ReferenceSymbol && e.Price > 0 {
			mm.indexPrice = e.Price
		}
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
	if mm.cfg.HedgeSymbol != "" && e.Symbol == mm.cfg.HedgeSymbol {
		mm.hedgeBid, mm.hedgeBidQty, mm.hedgeAsk, mm.hedgeAskQty = 0, 0, 0, 0
		if e.Snapshot != nil {
			if len(e.Snapshot.Bids) > 0 {
				mm.hedgeBid, mm.hedgeBidQty = e.Snapshot.Bids[0].Price, e.Snapshot.Bids[0].VisibleQty
			}
			if len(e.Snapshot.Asks) > 0 {
				mm.hedgeAsk, mm.hedgeAskQty = e.Snapshot.Asks[0].Price, e.Snapshot.Asks[0].VisibleQty
			}
		}
		mm.hedgeBookSeen++
		if mm.hedgeBid > 0 && mm.hedgeAsk > 0 {
			mm.hedgeBookTwoSided++
		}
		return
	}
	if e.Symbol != mm.cfg.ReferenceSymbol || len(e.Snapshot.Bids) == 0 || len(e.Snapshot.Asks) == 0 {
		return
	}
	mid := (e.Snapshot.Bids[0].Price + e.Snapshot.Asks[0].Price) / 2
	if mm.localReference != nil {
		mm.localReference.ObserveSnapshot(e)
		var ok bool
		mid, ok = mm.localReference.Mid()
		if !ok {
			return
		}
	}
	if mid <= 0 {
		return
	}
	mm.forward = mm.blendForward(mid, e.Timestamp)
}

// blendForward moves the maker's view toward the observed midpoint at the
// configured half-life, leaving it unchanged when none is set.
func (mm *StoikovMarketMaker) blendForward(mid, timestamp int64) int64 {
	if mm.cfg.ForwardHalfLife <= 0 || mm.forward <= 0 {
		mm.forwardAt = timestamp
		return mid
	}
	elapsed := float64(timestamp-mm.forwardAt) / 1e9
	mm.forwardAt = timestamp
	if elapsed <= 0 {
		return mm.forward
	}
	alpha := 1 - math.Exp(-math.Ln2*elapsed/mm.cfg.ForwardHalfLife.Seconds())
	blended := float64(mm.forward) + alpha*(float64(mid)-float64(mm.forward))
	if !finite(blended) || blended <= 0 {
		return mid
	}
	return int64(blended)
}

// onTrade estimates volatility from executed prices rather than from the
// maker's own quoted midpoint.
//
// A midpoint estimate is self-referential: widening the quote moves the mid,
// which raises the estimate, which widens the quote again. Trade prints are
// the prices at which flow actually crossed, so an untraded quote excursion
// cannot inflate the estimate on its own.
func (mm *StoikovMarketMaker) onTrade(e actor.TradeEvent) {
	if e.Symbol != mm.cfg.ReferenceSymbol || e.Trade == nil || e.Trade.Price <= 0 {
		return
	}
	price := e.Trade.Price
	if minSpacing := int64(mm.cfg.VolatilitySampleInterval); minSpacing > 0 && e.Timestamp-mm.lastForwardTS < minSpacing {
		return
	}
	if mm.lastForward > 0 && e.Timestamp > mm.lastForwardTS {
		dt := float64(e.Timestamp-mm.lastForwardTS) / float64(time.Second)
		logReturn := math.Log(float64(price) / float64(mm.lastForward))
		if dt > 0 && finite(logReturn) {
			instantVariance := logReturn * logReturn / dt
			alpha := ewmaAlpha(dt, mm.cfg.VolatilityHalfLife)
			mm.logVariancePerSec = (1-alpha)*mm.logVariancePerSec + alpha*instantVariance
			if cap := mm.maxLogVariance(); cap > 0 && mm.logVariancePerSec > cap {
				mm.logVariancePerSec = cap
			}
		}
	}
	mm.lastForward = price
	mm.lastForwardTS = e.Timestamp
}

func (mm *StoikovMarketMaker) maxLogVariance() float64 {
	if mm.cfg.MaxLogVarianceMultiple <= 0 || mm.cfg.InitialLogVariancePerSec <= 0 {
		return 0
	}
	return mm.cfg.InitialLogVariancePerSec * mm.cfg.MaxLogVarianceMultiple
}

func (mm *StoikovMarketMaker) onAccepted(e actor.OrderAcceptedEvent) {
	if mm.hedgeRequest != 0 && e.RequestID == mm.hedgeRequest {
		// A hedge is not a quote. Cancelling every unknown acknowledgement
		// would cancel the maker's own hedge before it could execute.
		mm.hedgeRequest = 0
		return
	}
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
		// A rejected hedge must release the in-flight flag, otherwise one
		// rejection stops the maker hedging for the rest of the run.
		mm.hedgeRejects++
		mm.hedgeLastReject = e.Reason
		mm.hedgePending, mm.hedgeRequest = false, 0
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
	if mm.cfg.HedgeSymbol != "" && e.Symbol == mm.cfg.HedgeSymbol {
		if e.Side == exchange.Buy {
			mm.hedgePosition += e.Qty
		} else {
			mm.hedgePosition -= e.Qty
		}
		mm.hedgeFills++
		if e.IsFull {
			mm.hedgePending = false
		}
		return
	}
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
	// An immediate-or-cancel hedge that partly filled is cancelled for its
	// remainder; the flag must clear on that too.
	mm.hedgePending = false
	if e.OrderID == mm.bidID {
		mm.bidID, mm.bidPrice = 0, 0
	}
	if e.OrderID == mm.askID {
		mm.askID, mm.askPrice = 0, 0
	}
}

// onHedgeTick runs the hedge on its own cadence when one is configured.
func (mm *StoikovMarketMaker) onHedgeTick(_ time.Time) {
	if !mm.subscribed || mm.cfg.HedgeInterval <= 0 {
		return
	}
	mm.hedgeDelta()
}

func (mm *StoikovMarketMaker) onTick(_ time.Time) {
	if !mm.subscribed {
		mm.Subscribe(mm.cfg.ReferenceSymbol, exchange.MDSnapshot)
		mm.Subscribe(mm.cfg.ReferenceSymbol, exchange.MDTrade)
		if mm.cfg.AnchorToIndex {
			mm.Subscribe(mm.cfg.ReferenceSymbol, exchange.MDIndex)
		}
		if mm.cfg.HedgeSymbol != "" {
			mm.Subscribe(mm.cfg.HedgeSymbol, exchange.MDSnapshot)
		}
		if mm.cfg.ReferenceSymbol != mm.cfg.Symbol {
			mm.Subscribe(mm.cfg.Symbol, exchange.MDSnapshot)
		}
		mm.subscribed = true
		return
	}
	if len(mm.pending) != 0 || mm.cfg.BasePrecision <= 0 || mm.cfg.QuotePrecision <= 0 || mm.cfg.TickSize <= 0 || mm.cfg.QuoteQty <= 0 {
		return
	}
	forward := mm.referencePrice()
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
	inventory := mm.inventoryFraction()
	quote, ok := CalculateStoikovQuote(StoikovInputs{
		Forward:           forwardPrice,
		Inventory:         inventory,
		VariancePerSecond: mm.logVariancePerSec * forwardPrice * forwardPrice,
		RiskAversion:      mm.cfg.RelativeRiskAversion / forwardPrice,
		FillDecay:         mm.cfg.RelativeFillDecay / forwardPrice,
		InventoryHorizon:  mm.cfg.InventoryHorizon,
		MinHalfSpread:     float64(mm.cfg.MinHalfSpreadTicks*mm.cfg.TickSize) / float64(mm.cfg.QuotePrecision),
	})
	if !ok {
		return
	}
	if mm.cfg.InventorySkewBps > 0 {
		// Replace the variance-derived skew with the configured one, keeping
		// the spread the control law produced.
		shift := forwardPrice * float64(mm.cfg.InventorySkewBps) / 10_000 * inventory
		quote.Reservation = forwardPrice - shift
		quote.Bid = quote.Reservation - quote.HalfSpread
		quote.Ask = quote.Reservation + quote.HalfSpread
		if quote.Bid <= 0 || quote.Ask <= quote.Bid {
			return
		}
	}
	bid, okBid := quoteToBidTicks(quote.Bid, mm.cfg.QuotePrecision, mm.cfg.TickSize)
	ask, okAsk := quoteToAskTicks(quote.Ask, mm.cfg.QuotePrecision, mm.cfg.TickSize)
	if !okBid || !okAsk || bid <= 0 || ask <= bid {
		return
	}
	if bid == mm.bidPrice && ask == mm.askPrice && mm.bidID != 0 && mm.askID != 0 {
		return
	}
	if mm.cfg.RequoteBps > 0 && mm.bidID != 0 && mm.askID != 0 {
		moved := maxInt64(absInt64(bid-mm.bidPrice), absInt64(ask-mm.askPrice))
		if reference := (mm.bidPrice + mm.askPrice) / 2; reference > 0 && moved*10000 < mm.cfg.RequoteBps*reference {
			return
		}
	}
	previousBid, previousAsk := mm.bidID, mm.askID
	if !mm.cfg.SubmitBeforeCancel {
		mm.cancelResting(previousBid, previousAsk)
	}
	mm.bidID, mm.askID = 0, 0
	// With a cadence configured the hedge runs on its own timer instead.
	if mm.cfg.HedgeInterval <= 0 {
		mm.hedgeDelta()
	}
	mm.bidPrice, mm.askPrice = bid, ask
	size := mm.quoteSize()
	bidRequest := mm.SubmitOrder(mm.cfg.Symbol, exchange.Buy, exchange.LimitOrder, bid, size)
	mm.pending[bidRequest] = stoikovBid
	askRequest := mm.SubmitOrder(mm.cfg.Symbol, exchange.Sell, exchange.LimitOrder, ask, size)
	mm.pending[askRequest] = stoikovAsk
	if mm.cfg.SubmitBeforeCancel {
		// Cancelling only after the replacements are submitted keeps depth
		// resting continuously. Cancelling first empties the book for the rest
		// of the phase, which every actor scheduled behind the maker then meets.
		mm.cancelResting(previousBid, previousAsk)
	}
}

func (mm *StoikovMarketMaker) cancelResting(bidID, askID uint64) {
	if bidID != 0 {
		mm.CancelOrder(bidID)
	}
	if askID != 0 {
		mm.CancelOrder(askID)
	}
}

// NetDelta is the maker's exposure after its hedge.
func (mm *StoikovMarketMaker) NetDelta() int64 { return mm.inventory + mm.hedgePosition }

// hedgeDelta offsets accumulated inventory on the hedge instrument, so the
// maker's risk is bounded by its hedging capacity rather than by how much of
// the asset it holds.
// quoteSize is the depth the maker shows, after any volatility withdrawal.
func (mm *StoikovMarketMaker) quoteSize() int64 {
	if mm.cfg.QuoteSizeVolElasticity <= 0 || mm.cfg.InitialLogVariancePerSec <= 0 {
		return mm.cfg.QuoteQty
	}
	floor := mm.cfg.MinQuoteSizeFraction
	if floor <= 0 {
		floor = 0.1
	}
	ratio := mm.cfg.InitialLogVariancePerSec / mm.logVariancePerSec
	if !finite(ratio) || ratio <= 0 {
		return int64(float64(mm.cfg.QuoteQty) * floor)
	}
	scale := math.Pow(ratio, mm.cfg.QuoteSizeVolElasticity)
	if scale > 1 {
		scale = 1
	}
	if scale < floor {
		scale = floor
	}
	return int64(float64(mm.cfg.QuoteQty) * scale)
}

func (mm *StoikovMarketMaker) hedgeDelta() {
	if mm.cfg.HedgeSymbol == "" || mm.hedgePending || mm.cfg.HedgeBandQty <= 0 {
		return
	}
	delta := mm.NetDelta()
	if delta > -mm.cfg.HedgeBandQty && delta < mm.cfg.HedgeBandQty {
		return
	}
	// Hedge against liquidity the maker can actually see, at a bounded price.
	// A blind market order is not a hedge: it can be sent into a book that has
	// nothing on the other side and simply disappear.
	side, quantity := exchange.Sell, delta
	price, available := mm.hedgeBid, mm.hedgeBidQty
	if delta < 0 {
		side, quantity = exchange.Buy, -delta
		price, available = mm.hedgeAsk, mm.hedgeAskQty
	}
	if price <= 0 {
		return
	}
	if mm.cfg.HedgeSlippageBps > 0 {
		if bumped, ok := etypes.TryMulBps(price, mm.cfg.HedgeSlippageBps); ok {
			if side == exchange.Buy {
				price += bumped
			} else {
				price -= bumped
			}
		}
		if price <= 0 {
			return
		}
	}
	if tick := mm.cfg.HedgeTickSize; tick > 0 {
		// Round outward so the order stays marketable after alignment.
		if side == exchange.Buy {
			price = (price + tick - 1) / tick * tick
		} else {
			price = price / tick * tick
		}
		if price <= 0 {
			return
		}
	}
	if available > 0 && quantity > available {
		quantity = available
	}
	if quantity <= 0 {
		return
	}
	mm.hedgeAttempts++
	mm.hedgeLastQty = quantity
	mm.hedgeRequest = mm.SubmitOrderWithTimeInForce(mm.cfg.HedgeSymbol, side, exchange.LimitOrder, price, quantity, exchange.IOC)
	mm.hedgePending = true
}

// inventoryFraction is the signed position as a fraction of the risk budget,
// clamped so a position beyond the budget cannot skew the quote without bound.
func (mm *StoikovMarketMaker) inventoryFraction() float64 {
	scale := mm.cfg.InventoryLimit
	if scale <= 0 {
		scale = mm.cfg.BasePrecision
	}
	position := mm.inventory
	if mm.cfg.HedgeSymbol != "" {
		// Skew against the risk actually carried, not the raw spot position.
		position = mm.NetDelta()
	}
	fraction := float64(position) / float64(scale)
	if fraction > 1 {
		return 1
	}
	if fraction < -1 {
		return -1
	}
	return fraction
}

// referencePrice is what the maker quotes around.
func (mm *StoikovMarketMaker) referencePrice() int64 {
	book := mm.forward
	if book <= 0 {
		book = mm.cfg.BootstrapPrice
	}
	if !mm.cfg.AnchorToIndex || mm.indexPrice <= 0 {
		return book
	}
	weight := mm.cfg.IndexWeight
	if weight <= 0 || weight > 1 {
		weight = 1
	}
	blended := weight*float64(mm.indexPrice) + (1-weight)*float64(book)
	if !finite(blended) || blended <= 0 {
		return book
	}
	return int64(blended)
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

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
