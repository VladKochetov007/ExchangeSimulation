// Package multivenue builds deterministic, venue-local market ecologies for
// experiments that later add explicit cross-venue routing and execution risk.
package multivenue

import (
	"context"
	"fmt"
	"math"
	"math/bits"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/simulation"
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
	// InventorySizeSkewBps redistributes the existing volatility-adjusted quote
	// size across bid and ask as the maker's carried risk moves away from zero.
	// It is separate from InventorySkewBps: that field moves reservation price,
	// while this field changes only displayed quantity. V2-3 P1 configures this
	// only for selected spot makers; zero retains symmetric sizes.
	InventorySizeSkewBps int64
	// RestingQuoteReplenishmentBelowBps refreshes an acknowledged passive side
	// only after its known remaining quantity falls strictly below this fraction
	// of the current target. Zero retains the historical target-only refresh
	// policy. The multivenue V2-3 P3 wiring scopes the field to ABC-PERP; the
	// actor itself neither assumes an exchange book nor creates a new cadence.
	RestingQuoteReplenishmentBelowBps int64
	// InventoryRebalance is an explicit, separately scheduled aggressive
	// risk-transfer action. It is nil for the passive P0/P1 population; unlike
	// passive requotes it can take locally visible liquidity and pay taker fees.
	InventoryRebalance *InventoryRebalanceConfig
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
	// PostOnly requires every quote refresh to rest. It is distinct from hedge
	// and rebalance policies, which remain explicit aggressive actions.
	PostOnly bool
	// PostOnlyCancelBeforeReplace separates the replacement-ordering treatment
	// from the post-only admission treatment. With false, the legacy actor sends
	// replacements before cancellation; with true, it sends cancellations first.
	// In both cases the venue checks post-only at actual arrival.
	PostOnlyCancelBeforeReplace bool

	// QuoteSizeDecisionObserver is an evidence-only P1 hook. It runs before
	// quote requests enter the gateway and has no return path into strategy,
	// exchange, scheduler, or matching state.
	QuoteSizeDecisionObserver func(MakerQuoteSizeDecision)
	QuoteSizeDecisionMaker    string
	QuoteSizeDecisionClient   uint64
	// QuoteSizeDecisionTerminalNano declares the run boundary for optional P1
	// evidence only. A quote decision at that instant cannot reach venue
	// ingress before the registered horizon closes.
	QuoteSizeDecisionTerminalNano int64
	// QuoteReplenishmentDecisionObserver records V2-3 P3's local own-order
	// lifecycle decision before any ordinary refresh request is submitted. It
	// has no return path into actor, gateway, scheduler, or matching state.
	QuoteReplenishmentDecisionObserver     func(PerpQuoteReplenishmentDecision)
	QuoteReplenishmentDecisionVenue        string
	QuoteReplenishmentDecisionMaker        string
	QuoteReplenishmentDecisionClient       uint64
	QuoteReplenishmentDecisionTerminalNano int64
	// InventoryRebalanceDecisionObserver is the P2 counterpart to the P1
	// quantity observer. It is write-only evidence and never returns state to
	// the actor, exchange, scheduler, or policy.
	InventoryRebalanceDecisionObserver     func(MakerInventoryRebalanceDecision)
	InventoryRebalanceDecisionVenue        string
	InventoryRebalanceDecisionMaker        string
	InventoryRebalanceDecisionClient       uint64
	InventoryRebalanceDecisionTerminalNano int64
	InventoryRebalanceTakerFeeBps          int64
	InventoryRebalanceFillObserver         func(MakerInventoryRebalanceFill)
}

// InventoryRebalanceConfig is the local, rate-limited P2 action contract. It
// is deliberately separate from a maker's passive quote policy: the action is
// a normal IOC order with its own cadence, participation cap, and cooldown.
type InventoryRebalanceConfig struct {
	Enabled          bool          `json:"enabled"`
	Interval         time.Duration `json:"interval"`
	Cooldown         time.Duration `json:"cooldown"`
	RiskBandQty      int64         `json:"risk_band_qty"`
	TargetBandQty    int64         `json:"target_band_qty"`
	MaxRequestQty    int64         `json:"max_request_qty"`
	ParticipationBps int64         `json:"participation_bps"`
	SlippageBps      int64         `json:"slippage_bps"`
}

func (c InventoryRebalanceConfig) validate() error {
	if c.Interval <= 0 || c.Cooldown <= 0 {
		return fmt.Errorf("interval and cooldown must be positive")
	}
	if c.RiskBandQty <= 0 || c.TargetBandQty < 0 || c.TargetBandQty >= c.RiskBandQty {
		return fmt.Errorf("require 0 <= target band < positive risk band")
	}
	if c.MaxRequestQty <= 0 {
		return fmt.Errorf("maximum request quantity must be positive")
	}
	if c.ParticipationBps <= 0 || c.ParticipationBps > 10_000 {
		return fmt.Errorf("participation bps must be in [1,10000]")
	}
	if c.SlippageBps < 0 || c.SlippageBps > 10_000 {
		return fmt.Errorf("slippage bps must be in [0,10000]")
	}
	return nil
}

// MakerInventoryRebalanceDecision is P2's persisted, pre-ingress evidence.
// It records an evaluation even when the declared local policy defers. The
// book fields come only from the actor's locally delivered CDF/USD snapshot;
// price zero remains a numeric field, never an availability sentinel.
type MakerInventoryRebalanceDecision struct {
	VenueID              string `json:"venue_id"`
	Maker                string `json:"maker"`
	ClientID             uint64 `json:"client_id"`
	Symbol               string `json:"symbol"`
	DecisionTime         int64  `json:"decision_time"`
	Enabled              bool   `json:"enabled"`
	Subscribed           bool   `json:"subscribed"`
	RequestPending       bool   `json:"request_pending"`
	ActionOrDeferReason  string `json:"action_or_defer_reason"`
	Inventory            int64  `json:"inventory"`
	RiskBandQty          int64  `json:"risk_band_qty"`
	TargetBandQty        int64  `json:"target_band_qty"`
	LastBookSourceTime   int64  `json:"last_book_source_time"`
	LastBookReceivedTime int64  `json:"last_book_received_time"`
	LastBookSequence     uint64 `json:"last_book_sequence"`
	BidPrice             int64  `json:"bid_price"`
	BidVisibleQty        int64  `json:"bid_visible_qty"`
	AskPrice             int64  `json:"ask_price"`
	AskVisibleQty        int64  `json:"ask_visible_qty"`
	// Side remains the execution representation. SideEvidence is deliberately
	// separate because Buy is the zero-valued enum and therefore cannot use
	// omitempty without turning a real BUY decision into absent evidence.
	Side               exchange.Side `json:"-"`
	SideEvidence       string        `json:"side,omitempty"`
	DesiredReduction   int64         `json:"desired_reduction"`
	ParticipationCap   int64         `json:"participation_cap"`
	MaxRequestQty      int64         `json:"max_request_qty"`
	ParticipationBps   int64         `json:"participation_bps"`
	SlippageBps        int64         `json:"slippage_bps"`
	EvaluationInterval int64         `json:"evaluation_interval"`
	Cooldown           int64         `json:"cooldown"`
	LimitPrice         int64         `json:"limit_price"`
	RequestedQty       int64         `json:"requested_qty"`
	TakerFeeBps        int64         `json:"taker_fee_bps"`
	RequestID          uint64        `json:"request_id,omitempty"`
	CooldownUntil      int64         `json:"cooldown_until"`
	OutcomeExpectation string        `json:"outcome_expectation"`
	CensorReason       string        `json:"censor_reason,omitempty"`
}

// MakerInventoryRebalanceFill attests the submitting maker's actor-local
// inventory immediately around an exchange-confirmed P2 fill. The exchange
// fill remains the source for execution and fee semantics; this companion
// record makes the claimed individual risk reduction independently testable.
type MakerInventoryRebalanceFill struct {
	VenueID       string        `json:"venue_id"`
	Maker         string        `json:"maker"`
	ClientID      uint64        `json:"client_id"`
	Symbol        string        `json:"symbol"`
	Timestamp     int64         `json:"timestamp"`
	OrderID       uint64        `json:"order_id"`
	TradeID       uint64        `json:"trade_id"`
	Side          exchange.Side `json:"side"`
	Qty           int64         `json:"qty"`
	Price         int64         `json:"price"`
	FeeAmount     int64         `json:"fee_amount"`
	FeeAsset      string        `json:"fee_asset"`
	PreInventory  int64         `json:"pre_inventory"`
	PostInventory int64         `json:"post_inventory"`
}

// MakerQuoteSizeDecision records a P1 quantity decision before either quote
// request is submitted. Venue acceptance/rejection evidence independently
// records what subsequently reached the matching engine.
type MakerQuoteSizeDecision struct {
	Maker               string `json:"maker"`
	ClientID            uint64 `json:"client_id"`
	Symbol              string `json:"symbol"`
	DecisionTime        int64  `json:"decision_time"`
	BidRequestID        uint64 `json:"bid_request_id"`
	AskRequestID        uint64 `json:"ask_request_id"`
	BaseVolatilitySize  int64  `json:"base_volatility_size"`
	RiskPosition        int64  `json:"risk_position"`
	InventoryLimit      int64  `json:"inventory_limit"`
	SizeSkewBps         int64  `json:"size_skew_bps"`
	FullAdjustment      int64  `json:"full_adjustment"`
	Adjustment          int64  `json:"adjustment"`
	BidPrice            int64  `json:"bid_price"`
	AskPrice            int64  `json:"ask_price"`
	BidQty              int64  `json:"bid_qty"`
	AskQty              int64  `json:"ask_qty"`
	PostOnly            bool   `json:"post_only"`
	CancelBeforeReplace bool   `json:"cancel_before_replace"`
	OutcomeExpectation  string `json:"outcome_expectation"`
	CensorReason        string `json:"censor_reason,omitempty"`
}

// PerpQuoteReplenishmentDecision is V2-3 P3 evidence for a maker's ordinary
// quote-tick decision after an own confirmed partial fill. Target quantities
// and known resting quantities are intentionally distinct: a maker_state
// target is not a claim about exchange residual depth.
type PerpQuoteReplenishmentDecision struct {
	VenueID             string `json:"venue_id"`
	Maker               string `json:"maker"`
	ClientID            uint64 `json:"client_id"`
	Symbol              string `json:"symbol"`
	DecisionTime        int64  `json:"decision_time"`
	Enabled             bool   `json:"enabled"`
	ThresholdBps        int64  `json:"threshold_bps"`
	BidOrderID          uint64 `json:"bid_order_id"`
	AskOrderID          uint64 `json:"ask_order_id"`
	BidTargetQty        int64  `json:"bid_target_qty"`
	AskTargetQty        int64  `json:"ask_target_qty"`
	BidKnownRestingQty  int64  `json:"bid_known_resting_qty"`
	AskKnownRestingQty  int64  `json:"ask_known_resting_qty"`
	BidReplenishmentDue bool   `json:"bid_replenishment_due"`
	AskReplenishmentDue bool   `json:"ask_replenishment_due"`
	RefreshDue          bool   `json:"refresh_due"`
	Reason              string `json:"reason"`
	BidPrice            int64  `json:"bid_price"`
	AskPrice            int64  `json:"ask_price"`
	BidRequestID        uint64 `json:"bid_request_id"`
	AskRequestID        uint64 `json:"ask_request_id"`
	OutcomeExpectation  string `json:"outcome_expectation"`
	CensorReason        string `json:"censor_reason,omitempty"`
}

// quoteReplenishmentState is the actor-local pre-cancel state attested by a
// P3 decision. The ordinary refresh clears current IDs before it allocates
// request IDs, so recording this value copy prevents evidence from confusing a
// deliberately cancelled prior quote with an absent prior quote.
type quoteReplenishmentState struct {
	bidDue, askDue               bool
	refreshDue                   bool
	bidOrderID, askOrderID       uint64
	bidTargetQty, askTargetQty   int64
	bidRestingQty, askRestingQty int64
	bidPrice, askPrice           int64
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

	forward                      int64
	indexPrice                   int64
	localReference               *LocalBookCache
	remoteReference              *LocalBookCache
	remoteWeight                 float64
	remoteConfidence             float64
	remoteMaxAge                 time.Duration
	forwardAt                    int64
	lastForward                  int64
	lastForwardTS                int64
	logVariancePerSec            float64
	inventory                    int64
	bidID, askID                 uint64
	bidPrice, askPrice           int64
	bidQty, askQty               int64
	bidRestingQty, askRestingQty int64
	hedgePosition                int64
	hedgePending                 bool
	hedgeRequest                 uint64
	hedgeAttempts                int
	hedgeFills                   int
	hedgeRejects                 int
	hedgeLastReject              exchange.RejectReason
	hedgeLastQty                 int64
	hedgeBid, hedgeAsk           int64
	hedgeBidQty                  int64
	hedgeAskQty                  int64
	hedgeBookSeen                int
	hedgeBookTwoSided            int
	rebalanceBook                localRebalanceBook
	rebalancePending             bool
	rebalanceRequest             uint64
	rebalanceOrderIDs            map[uint64]struct{}
	rebalanceCooldownAt          int64
	pending                      map[uint64]quoteSide
	subscribed                   bool
}

// localRebalanceBook is the last actor-local CDF/USD snapshot. It stores both
// the source publication and observed inbox delivery boundaries so P2 can be
// independently joined back to V2-0 receipts without reading an exchange book.
type localRebalanceBook struct {
	SourceTime   int64
	ReceivedTime int64
	Sequence     uint64
	BidPrice     int64
	BidQty       int64
	AskPrice     int64
	AskQty       int64
}

func NewStoikovMarketMaker(id uint64, gw actor.Gateway, cfg StoikovMMConfig) *StoikovMarketMaker {
	mm := &StoikovMarketMaker{
		BaseActor:         actor.NewBaseActor(id, gw),
		cfg:               cfg,
		forward:           cfg.BootstrapPrice,
		logVariancePerSec: cfg.InitialLogVariancePerSec,
		pending:           make(map[uint64]quoteSide),
		rebalanceOrderIDs: make(map[uint64]struct{}),
	}
	if cfg.UseLocalReferenceCache {
		mm.localReference = NewLocalBookCache(cfg.LocalReferenceSourceVenue, cfg.ReferenceSymbol)
	}
	mm.SetHandler(mm)
	mm.AddTicker(cfg.QuoteInterval, mm.onTick)
	if cfg.HedgeInterval > 0 {
		mm.AddTicker(cfg.HedgeInterval, mm.onHedgeTick)
	}
	if cfg.InventoryRebalance != nil && cfg.InventoryRebalance.Interval > 0 {
		mm.AddTicker(cfg.InventoryRebalance.Interval, mm.onRebalanceTick)
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

// RemoteReferenceView is controller-only activation evidence for the one
// remote-feed V2-1 smoke world. It returns a copy, never an exchange object.
func (mm *StoikovMarketMaker) RemoteReferenceView() (LocalBookView, bool) {
	if mm == nil {
		return LocalBookView{}, false
	}
	return mm.remoteReference.View()
}

// AttachRemoteReferenceFeed attaches one feed-only delayed public session to
// this maker. Weight and confidence form the remote composite component;
// maxAge bounds it by source publication time when positive. The reference is
// withheld until both actor-owned caches have usable observations; there is no
// global-index fallback.
func (mm *StoikovMarketMaker) AttachRemoteReferenceFeed(feed actor.Gateway, sourceVenue string, weight, confidence float64, maxAge time.Duration) error {
	if mm == nil || feed == nil || sourceVenue == "" || weight <= 0 || weight > 1 || confidence <= 0 || confidence > 1 || maxAge < 0 || weight*confidence > 1 {
		return fmt.Errorf("multivenue: invalid remote reference feed")
	}
	if mm.localReference == nil {
		return fmt.Errorf("multivenue: remote reference needs a local cache")
	}
	if mm.remoteReference != nil {
		return fmt.Errorf("multivenue: remote reference already attached")
	}
	cache := NewLocalBookCache(sourceVenue, mm.cfg.ReferenceSymbol)
	if err := mm.AddMarketDataFeed(feed, func(message *exchange.MarketDataMsg) {
		cache.ObserveMarketData(message)
	}); err != nil {
		return err
	}
	if err := mm.SubscribeMarketDataFeed(feed, mm.cfg.ReferenceSymbol, exchange.MDSnapshot); err != nil {
		return err
	}
	mm.remoteReference, mm.remoteWeight, mm.remoteConfidence, mm.remoteMaxAge = cache, weight, confidence, maxAge
	return nil
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
	if mm.cfg.InventoryRebalance != nil && e.Symbol == mm.cfg.Symbol {
		mm.observeRebalanceBook(e)
	}
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
	if e.Symbol != mm.cfg.ReferenceSymbol || e.Snapshot == nil || len(e.Snapshot.Bids) == 0 || len(e.Snapshot.Asks) == 0 {
		return
	}
	mid, available := positiveDomainTwoSidedMidpoint(e.Snapshot.Bids[0].Price, e.Snapshot.Asks[0].Price)
	if !available {
		return
	}
	if mm.localReference != nil {
		mm.localReference.ObserveSnapshot(e)
		var ok bool
		mid, ok = mm.localReference.Mid()
		if !ok {
			return
		}
	}
	mm.forward = mm.blendForward(mid, e.Timestamp)
}

// observeRebalanceBook copies one actor-local public snapshot for the P2
// policy. The optional receipt frontier is evidence metadata only: the policy
// never branches on it. In deterministic phases any snapshot dispatched here
// was inserted into the inbox at that delivery time; the offline P2 auditor
// additionally verifies this source timestamp/sequence against the V2-0
// receipt ledger.
func (mm *StoikovMarketMaker) observeRebalanceBook(e actor.BookSnapshotEvent) {
	book := localRebalanceBook{SourceTime: e.Timestamp, Sequence: e.SeqNum}
	if e.Snapshot != nil {
		if len(e.Snapshot.Bids) > 0 {
			book.BidPrice, book.BidQty = e.Snapshot.Bids[0].Price, e.Snapshot.Bids[0].VisibleQty
		}
		if len(e.Snapshot.Asks) > 0 {
			book.AskPrice, book.AskQty = e.Snapshot.Asks[0].Price, e.Snapshot.Asks[0].VisibleQty
		}
	}
	if gateway, ok := mm.Gateway().(interface {
		MarketDataFrontier() simulation.MarketDataFrontier
	}); ok {
		book.ReceivedTime = gateway.MarketDataFrontier().DeliveredAt
	}
	mm.rebalanceBook = book
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
	if mm.rebalanceRequest != 0 && e.RequestID == mm.rebalanceRequest {
		// An accepted P2 IOC must remain live long enough to receive its normal
		// fill/cancel notifications. It is not a passive quote and must never be
		// cancelled by the unknown-acknowledgement guard below.
		mm.rebalancePending, mm.rebalanceRequest = false, 0
		mm.rebalanceOrderIDs[e.OrderID] = struct{}{}
		return
	}
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
		mm.bidRestingQty = mm.bidQty
	} else {
		mm.askID = e.OrderID
		mm.askRestingQty = mm.askQty
	}
}

func (mm *StoikovMarketMaker) onRejected(e actor.OrderRejectedEvent) {
	if mm.rebalanceRequest != 0 && e.RequestID == mm.rebalanceRequest {
		mm.rebalancePending, mm.rebalanceRequest = false, 0
		return
	}
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
		mm.bidPrice, mm.bidQty, mm.bidRestingQty = 0, 0, 0
	} else {
		mm.askPrice, mm.askQty, mm.askRestingQty = 0, 0, 0
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
	_, rebalanceFill := mm.rebalanceOrderIDs[e.OrderID]
	preInventory := mm.inventory
	if e.Side == exchange.Buy {
		mm.inventory += e.Qty
	} else {
		mm.inventory -= e.Qty
	}
	if rebalanceFill && mm.cfg.InventoryRebalanceFillObserver != nil {
		mm.cfg.InventoryRebalanceFillObserver(MakerInventoryRebalanceFill{
			VenueID:       mm.cfg.InventoryRebalanceDecisionVenue,
			Maker:         mm.cfg.InventoryRebalanceDecisionMaker,
			ClientID:      mm.cfg.InventoryRebalanceDecisionClient,
			Symbol:        e.Symbol,
			Timestamp:     e.Timestamp,
			OrderID:       e.OrderID,
			TradeID:       e.TradeID,
			Side:          e.Side,
			Qty:           e.Qty,
			Price:         e.Price,
			FeeAmount:     e.FeeAmount,
			FeeAsset:      e.FeeAsset,
			PreInventory:  preInventory,
			PostInventory: mm.inventory,
		})
	}
	if e.OrderID == mm.bidID {
		mm.bidRestingQty = decrementKnownResting(mm.bidRestingQty, e.Qty)
	}
	if e.OrderID == mm.askID {
		mm.askRestingQty = decrementKnownResting(mm.askRestingQty, e.Qty)
	}
	if !e.IsFull {
		return
	}
	delete(mm.rebalanceOrderIDs, e.OrderID)
	if e.OrderID == mm.bidID {
		mm.bidID, mm.bidPrice, mm.bidQty, mm.bidRestingQty = 0, 0, 0, 0
	}
	if e.OrderID == mm.askID {
		mm.askID, mm.askPrice, mm.askQty, mm.askRestingQty = 0, 0, 0, 0
	}
}

func (mm *StoikovMarketMaker) onCancelled(e actor.OrderCancelledEvent) {
	delete(mm.rebalanceOrderIDs, e.OrderID)
	// An immediate-or-cancel hedge that partly filled is cancelled for its
	// remainder; the flag must clear on that too.
	mm.hedgePending = false
	if e.OrderID == mm.bidID {
		mm.bidID, mm.bidPrice, mm.bidQty, mm.bidRestingQty = 0, 0, 0, 0
	}
	if e.OrderID == mm.askID {
		mm.askID, mm.askPrice, mm.askQty, mm.askRestingQty = 0, 0, 0, 0
	}
}

// onHedgeTick runs the hedge on its own cadence when one is configured.
func (mm *StoikovMarketMaker) onHedgeTick(_ time.Time) {
	if !mm.subscribed || mm.cfg.HedgeInterval <= 0 {
		return
	}
	mm.hedgeDelta()
}

// onRebalanceTick runs P2's explicit CDF/USD risk-transfer policy on its
// independently declared cadence. It shares neither quote refreshes nor hedge
// decisions: a passive maker must make any aggressive inventory reduction
// observable, costly, rate-limited, and non-guaranteed.
func (mm *StoikovMarketMaker) onRebalanceTick(now time.Time) {
	policy := mm.cfg.InventoryRebalance
	if policy == nil || policy.Interval <= 0 {
		return
	}
	decision := mm.rebalanceDecision(now)
	if decision.ActionOrDeferReason != "SUBMIT_IOC" {
		mm.emitInventoryRebalanceDecision(decision)
		return
	}
	// Cooldown begins at the attempted action boundary, not at a later fill or
	// acceptance. A rejected or unfilled IOC therefore cannot hammer the book.
	mm.rebalancePending = true
	mm.rebalanceRequest = decision.RequestID
	mm.rebalanceCooldownAt = decision.CooldownUntil
	mm.emitInventoryRebalanceDecision(decision)
	mm.SubmitOrderWithTimeInForce(mm.cfg.Symbol, decision.Side, exchange.LimitOrder, decision.LimitPrice, decision.RequestedQty, exchange.IOC)
}

// rebalanceDecision is deliberately pure with respect to venue state: it reads
// only the maker's inventory and the last locally delivered public snapshot.
// Its explicit reason is persisted for every timer evaluation, including the
// disabled control arm.
func (mm *StoikovMarketMaker) rebalanceDecision(now time.Time) MakerInventoryRebalanceDecision {
	policy := mm.cfg.InventoryRebalance
	book := mm.rebalanceBook
	decision := MakerInventoryRebalanceDecision{
		VenueID:              mm.cfg.InventoryRebalanceDecisionVenue,
		Maker:                mm.cfg.InventoryRebalanceDecisionMaker,
		ClientID:             mm.cfg.InventoryRebalanceDecisionClient,
		Symbol:               mm.cfg.Symbol,
		DecisionTime:         now.UnixNano(),
		Subscribed:           mm.subscribed,
		RequestPending:       mm.rebalancePending,
		Inventory:            mm.inventory,
		LastBookSourceTime:   book.SourceTime,
		LastBookReceivedTime: book.ReceivedTime,
		LastBookSequence:     book.Sequence,
		BidPrice:             book.BidPrice,
		BidVisibleQty:        book.BidQty,
		AskPrice:             book.AskPrice,
		AskVisibleQty:        book.AskQty,
	}
	if policy == nil {
		decision.ActionOrDeferReason = "POLICY_UNCONFIGURED"
		return decision
	}
	decision.Enabled = policy.Enabled
	decision.RiskBandQty = policy.RiskBandQty
	decision.TargetBandQty = policy.TargetBandQty
	decision.MaxRequestQty = policy.MaxRequestQty
	decision.ParticipationBps = policy.ParticipationBps
	decision.SlippageBps = policy.SlippageBps
	decision.EvaluationInterval = int64(policy.Interval)
	decision.Cooldown = int64(policy.Cooldown)
	decision.TakerFeeBps = mm.cfg.InventoryRebalanceTakerFeeBps
	if !policy.Enabled {
		decision.ActionOrDeferReason = "POLICY_DISABLED"
		return decision
	}
	if !mm.subscribed {
		decision.ActionOrDeferReason = "NOT_SUBSCRIBED"
		return decision
	}
	if mm.rebalancePending {
		decision.ActionOrDeferReason = "REQUEST_PENDING"
		return decision
	}
	if now.UnixNano() < mm.rebalanceCooldownAt {
		decision.ActionOrDeferReason = "COOLDOWN"
		decision.CooldownUntil = mm.rebalanceCooldownAt
		return decision
	}
	magnitude := clampAbsInventory(mm.inventory, math.MaxInt64)
	if magnitude < policy.RiskBandQty {
		decision.ActionOrDeferReason = "IN_BAND"
		return decision
	}
	if book.SourceTime == 0 {
		decision.ActionOrDeferReason = "LOCAL_BOOK_UNAVAILABLE"
		return decision
	}
	// P2 has no free staleness parameter: the actor-visible source publication
	// is stale exactly once it is older than the already declared evaluation
	// interval. Receipt delivery time remains evidence-only, so turning V2-0
	// instrumentation on cannot alter the simulated action.
	age := now.UnixNano() - book.SourceTime
	if age < 0 {
		decision.ActionOrDeferReason = "LOCAL_BOOK_SOURCE_FUTURE"
		return decision
	}
	if age > int64(policy.Interval) {
		decision.ActionOrDeferReason = "LOCAL_BOOK_STALE"
		return decision
	}
	if magnitude <= policy.TargetBandQty {
		decision.ActionOrDeferReason = "IN_BAND"
		return decision
	}
	desired := magnitude - policy.TargetBandQty
	decision.DesiredReduction = desired
	if mm.inventory > 0 {
		decision.Side = exchange.Sell
		decision.ParticipationCap = rebalanceParticipationCap(book.BidQty, policy.ParticipationBps)
		decision.LimitPrice = rebalanceOutwardLimit(book.BidPrice, policy.SlippageBps, mm.cfg.TickSize, exchange.Sell)
	} else {
		decision.Side = exchange.Buy
		decision.ParticipationCap = rebalanceParticipationCap(book.AskQty, policy.ParticipationBps)
		decision.LimitPrice = rebalanceOutwardLimit(book.AskPrice, policy.SlippageBps, mm.cfg.TickSize, exchange.Buy)
	}
	decision.SideEvidence = decision.Side.String()
	if decision.ParticipationCap <= 0 {
		decision.ActionOrDeferReason = "LOCAL_CONTRA_TOUCH_UNAVAILABLE"
		return decision
	}
	if decision.LimitPrice <= 0 {
		decision.ActionOrDeferReason = "INVALID_OUTWARD_LIMIT"
		return decision
	}
	decision.RequestedQty = minInt64(desired, policy.MaxRequestQty, decision.ParticipationCap)
	if decision.RequestedQty <= 0 {
		decision.ActionOrDeferReason = "REQUEST_QUANTITY_UNAVAILABLE"
		return decision
	}
	cooldownUntil, ok := etypes.TryAdd(now.UnixNano(), int64(policy.Cooldown))
	if !ok {
		decision.ActionOrDeferReason = "COOLDOWN_OVERFLOW"
		return decision
	}
	decision.RequestID = mm.PeekNextRequestID()
	decision.CooldownUntil = cooldownUntil
	decision.ActionOrDeferReason = "SUBMIT_IOC"
	decision.OutcomeExpectation = "VENUE_OUTCOME_REQUIRED"
	if mm.cfg.InventoryRebalanceDecisionTerminalNano != 0 && now.UnixNano() >= mm.cfg.InventoryRebalanceDecisionTerminalNano {
		decision.OutcomeExpectation = "SIMULATION_HORIZON_CENSORED"
		decision.CensorReason = "terminal_horizon_before_venue_ingress"
	}
	return decision
}

func rebalanceParticipationCap(visibleQty, bps int64) int64 {
	if visibleQty <= 0 || bps <= 0 {
		return 0
	}
	cap, ok := etypes.TryMulBps(visibleQty, bps)
	if !ok || cap <= 0 {
		return 0
	}
	return cap
}

func rebalanceOutwardLimit(touch, slippageBps, tick int64, side exchange.Side) int64 {
	if touch <= 0 || slippageBps < 0 || tick <= 0 {
		return 0
	}
	concession, ok := etypes.TryMulBps(touch, slippageBps)
	if !ok || concession < 0 {
		return 0
	}
	price := touch
	if side == exchange.Buy {
		price, ok = etypes.TryAdd(touch, concession)
	} else {
		price, ok = etypes.TryAdd(touch, -concession)
	}
	if !ok || price <= 0 {
		return 0
	}
	remainder := price % tick
	if remainder == 0 {
		return price
	}
	if side == exchange.Buy {
		price, ok = etypes.TryAdd(price, tick-remainder)
		if !ok {
			return 0
		}
		return price
	}
	return price - remainder
}

func minInt64(values ...int64) int64 {
	if len(values) == 0 {
		return 0
	}
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

func (mm *StoikovMarketMaker) emitInventoryRebalanceDecision(decision MakerInventoryRebalanceDecision) {
	if mm.cfg.InventoryRebalanceDecisionObserver != nil {
		mm.cfg.InventoryRebalanceDecisionObserver(decision)
	}
}

func (mm *StoikovMarketMaker) onTick(now time.Time) {
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
	forward, available := mm.referencePriceAt(now)
	if !available {
		// The maker's declared information frontier has no usable reference.
		// This is an explicit local-policy deferral, not a quote around 0.
		return
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
	sizes, ok := mm.quoteSizePlan()
	if !ok {
		return
	}
	sizeUnchanged := sizes.BidQty == mm.bidQty && sizes.AskQty == mm.askQty
	quoteUnchanged := bid == mm.bidPrice && ask == mm.askPrice && sizeUnchanged && mm.bidID != 0 && mm.askID != 0
	if quoteUnchanged {
		bidDue, askDue := mm.replenishmentDue()
		refreshDue := bidDue || askDue
		replenishment := mm.quoteReplenishmentState(bidDue, askDue, refreshDue)
		if !refreshDue {
			mm.emitQuoteReplenishmentDecision(now, replenishment, 0, 0)
			return
		}
		mm.refreshQuotePair(now, bid, ask, sizes, replenishment)
		return
	}
	if mm.cfg.RequoteBps > 0 && sizeUnchanged && mm.bidID != 0 && mm.askID != 0 {
		moved := maxInt64(absInt64(bid-mm.bidPrice), absInt64(ask-mm.askPrice))
		if reference, available := positiveDomainTwoSidedMidpoint(mm.bidPrice, mm.askPrice); available && moved*10000 < mm.cfg.RequoteBps*reference {
			return
		}
	}
	mm.refreshQuotePair(now, bid, ask, sizes, quoteReplenishmentState{})
}

func (mm *StoikovMarketMaker) refreshQuotePair(now time.Time, bid, ask int64, sizes quoteSizePlan, replenishment quoteReplenishmentState) {
	previousBid, previousAsk := mm.bidID, mm.askID
	if !mm.cfg.SubmitBeforeCancel || (mm.cfg.PostOnly && mm.cfg.PostOnlyCancelBeforeReplace) {
		mm.cancelResting(previousBid, previousAsk)
	}
	mm.bidID, mm.askID = 0, 0
	mm.bidRestingQty, mm.askRestingQty = 0, 0
	// With a cadence configured the hedge runs on its own timer instead.
	if mm.cfg.HedgeInterval <= 0 {
		mm.hedgeDelta()
	}
	mm.bidPrice, mm.askPrice = bid, ask
	mm.bidQty, mm.askQty = sizes.BidQty, sizes.AskQty
	bidRequestID := mm.PeekNextRequestID()
	askRequestID := bidRequestID + 1
	if replenishment.refreshDue {
		mm.emitQuoteReplenishmentDecision(now, replenishment, bidRequestID, askRequestID)
	}
	mm.emitQuoteSizeDecision(now, sizes, bid, ask, bidRequestID, askRequestID)
	bidRequest := mm.submitQuote(exchange.Buy, bid, sizes.BidQty)
	mm.pending[bidRequest] = stoikovBid
	askRequest := mm.submitQuote(exchange.Sell, ask, sizes.AskQty)
	mm.pending[askRequest] = stoikovAsk
	if mm.cfg.SubmitBeforeCancel && !(mm.cfg.PostOnly && mm.cfg.PostOnlyCancelBeforeReplace) {
		// Cancelling only after the replacements are submitted keeps depth
		// resting continuously. Cancelling first empties the book for the rest
		// of the phase, which every actor scheduled behind the maker then meets.
		mm.cancelResting(previousBid, previousAsk)
	}
}

// decrementKnownResting updates only the maker's locally confirmed quantity.
// Exchange fill quantities are positive by admission invariant. A malformed or
// duplicate fill cannot manufacture residual liquidity: it leaves the local
// known amount at zero and the independent P3 replay reports the broken raw
// lifecycle relation.
func decrementKnownResting(resting, fill int64) int64 {
	if resting <= 0 || fill <= 0 || fill >= resting {
		return 0
	}
	return resting - fill
}

// restingBelowFraction compares resting*10,000 < target*bps with 128-bit
// unsigned products. Inputs are non-negative quantities and bps is validated
// in [0,10,000], so this is exact for the complete int64 quantity domain and
// never relies on a rounded threshold quantity.
func restingBelowFraction(resting, target, bps int64) bool {
	if resting < 0 || target <= 0 || bps <= 0 {
		return false
	}
	leftHi, leftLo := bits.Mul64(uint64(resting), 10_000)
	rightHi, rightLo := bits.Mul64(uint64(target), uint64(bps))
	return leftHi < rightHi || (leftHi == rightHi && leftLo < rightLo)
}

func (mm *StoikovMarketMaker) replenishmentDue() (bidDue, askDue bool) {
	bps := mm.cfg.RestingQuoteReplenishmentBelowBps
	if bps <= 0 || mm.bidID == 0 || mm.askID == 0 {
		return false, false
	}
	return restingBelowFraction(mm.bidRestingQty, mm.bidQty, bps), restingBelowFraction(mm.askRestingQty, mm.askQty, bps)
}

func (mm *StoikovMarketMaker) quoteReplenishmentState(bidDue, askDue, refreshDue bool) quoteReplenishmentState {
	return quoteReplenishmentState{
		bidDue: bidDue, askDue: askDue, refreshDue: refreshDue,
		bidOrderID: mm.bidID, askOrderID: mm.askID,
		bidTargetQty: mm.bidQty, askTargetQty: mm.askQty,
		bidRestingQty: mm.bidRestingQty, askRestingQty: mm.askRestingQty,
		bidPrice: mm.bidPrice, askPrice: mm.askPrice,
	}
}

func (mm *StoikovMarketMaker) emitQuoteReplenishmentDecision(now time.Time, state quoteReplenishmentState, bidRequestID, askRequestID uint64) {
	if mm.cfg.QuoteReplenishmentDecisionObserver == nil {
		return
	}
	reason := "POLICY_DISABLED"
	if state.refreshDue {
		switch {
		case state.bidDue && state.askDue:
			reason = "BOTH_BELOW_THRESHOLD"
		case state.bidDue:
			reason = "BID_BELOW_THRESHOLD"
		default:
			reason = "ASK_BELOW_THRESHOLD"
		}
	} else if mm.cfg.RestingQuoteReplenishmentBelowBps > 0 {
		reason = "ABOVE_THRESHOLD"
	}
	expectation, censorReason := "NO_VENUE_REQUEST", ""
	if state.refreshDue {
		expectation = "VENUE_OUTCOME_REQUIRED"
		if mm.cfg.QuoteReplenishmentDecisionTerminalNano != 0 && now.UnixNano() >= mm.cfg.QuoteReplenishmentDecisionTerminalNano {
			expectation = "SIMULATION_HORIZON_CENSORED"
			censorReason = "terminal_horizon_before_venue_ingress"
		}
	}
	mm.cfg.QuoteReplenishmentDecisionObserver(PerpQuoteReplenishmentDecision{
		VenueID:             mm.cfg.QuoteReplenishmentDecisionVenue,
		Maker:               mm.cfg.QuoteReplenishmentDecisionMaker,
		ClientID:            mm.cfg.QuoteReplenishmentDecisionClient,
		Symbol:              mm.cfg.Symbol,
		DecisionTime:        now.UnixNano(),
		Enabled:             mm.cfg.RestingQuoteReplenishmentBelowBps > 0,
		ThresholdBps:        mm.cfg.RestingQuoteReplenishmentBelowBps,
		BidOrderID:          state.bidOrderID,
		AskOrderID:          state.askOrderID,
		BidTargetQty:        state.bidTargetQty,
		AskTargetQty:        state.askTargetQty,
		BidKnownRestingQty:  state.bidRestingQty,
		AskKnownRestingQty:  state.askRestingQty,
		BidReplenishmentDue: state.bidDue,
		AskReplenishmentDue: state.askDue,
		RefreshDue:          state.refreshDue,
		Reason:              reason,
		BidPrice:            state.bidPrice,
		AskPrice:            state.askPrice,
		BidRequestID:        bidRequestID,
		AskRequestID:        askRequestID,
		OutcomeExpectation:  expectation,
		CensorReason:        censorReason,
	})
}

func (mm *StoikovMarketMaker) submitQuote(side exchange.Side, price, qty int64) uint64 {
	if mm.cfg.PostOnly {
		return mm.SubmitPostOnlyOrder(mm.cfg.Symbol, side, price, qty)
	}
	return mm.SubmitOrder(mm.cfg.Symbol, side, exchange.LimitOrder, price, qty)
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

type quoteSizePlan struct {
	BaseVolatilitySize int64
	RiskPosition       int64
	InventoryLimit     int64
	SizeSkewBps        int64
	FullAdjustment     int64
	Adjustment         int64
	BidQty             int64
	AskQty             int64
}

// quoteSizePlan evaluates the V2-3 P1 quantity policy without changing any
// price or exchange state. Checked fixed-point arithmetic prevents a large
// configuration value from wrapping into a superficially valid quote.
func (mm *StoikovMarketMaker) quoteSizePlan() (quoteSizePlan, bool) {
	base := mm.quoteSize()
	limit := mm.cfg.InventoryLimit
	if limit <= 0 {
		// Keep direct-maker fixtures and the existing inventory-fraction
		// contract aligned with scenario-normalized configurations.
		limit = mm.cfg.BasePrecision
	}
	bps := mm.cfg.InventorySizeSkewBps
	if base <= 0 || limit <= 0 || bps < 0 || bps > 5_000 {
		return quoteSizePlan{}, false
	}
	plan := quoteSizePlan{
		BaseVolatilitySize: base,
		RiskPosition:       mm.riskPosition(),
		InventoryLimit:     limit,
		SizeSkewBps:        bps,
		BidQty:             base,
		AskQty:             base,
	}
	full, ok := etypes.TryMulBps(base, bps)
	if !ok {
		return quoteSizePlan{}, false
	}
	plan.FullAdjustment = full
	positionMagnitude := clampAbsInventory(plan.RiskPosition, limit)
	adjustment, ok := etypes.TryMulDiv(full, positionMagnitude, limit)
	if !ok || adjustment < 0 || adjustment > base {
		return quoteSizePlan{}, false
	}
	plan.Adjustment = adjustment
	if adjustment == 0 || plan.RiskPosition == 0 {
		return plan, true
	}
	if plan.RiskPosition > 0 {
		plan.BidQty -= adjustment
		plan.AskQty, ok = etypes.TryAdd(base, adjustment)
	} else {
		plan.AskQty -= adjustment
		plan.BidQty, ok = etypes.TryAdd(base, adjustment)
	}
	if !ok || plan.BidQty <= 0 || plan.AskQty <= 0 {
		return quoteSizePlan{}, false
	}
	return plan, true
}

// riskPosition is the same carried-risk state used by inventoryFraction: a
// hedged maker sizes against net delta, while an unhedged maker sizes against
// the spot inventory it actually carries.
func (mm *StoikovMarketMaker) riskPosition() int64 {
	if mm.cfg.HedgeSymbol != "" {
		return mm.NetDelta()
	}
	return mm.inventory
}

// clampAbsInventory returns min(abs(position), limit) without negating
// math.MinInt64. limit is a validated positive quantity bound.
func clampAbsInventory(position, limit int64) int64 {
	if limit <= 0 {
		return 0
	}
	if position >= 0 {
		if position > limit {
			return limit
		}
		return position
	}
	if position == math.MinInt64 || -position > limit {
		return limit
	}
	return -position
}

func (mm *StoikovMarketMaker) emitQuoteSizeDecision(now time.Time, plan quoteSizePlan, bidPrice, askPrice int64, bidRequestID, askRequestID uint64) {
	if mm.cfg.QuoteSizeDecisionObserver == nil {
		return
	}
	expectation, reason := "VENUE_OUTCOME_REQUIRED", ""
	if mm.cfg.QuoteSizeDecisionTerminalNano != 0 && now.UnixNano() >= mm.cfg.QuoteSizeDecisionTerminalNano {
		expectation = "SIMULATION_HORIZON_CENSORED"
		reason = "terminal_horizon_before_venue_ingress"
	}
	mm.cfg.QuoteSizeDecisionObserver(MakerQuoteSizeDecision{
		Maker:               mm.cfg.QuoteSizeDecisionMaker,
		ClientID:            mm.cfg.QuoteSizeDecisionClient,
		Symbol:              mm.cfg.Symbol,
		DecisionTime:        now.UnixNano(),
		BidRequestID:        bidRequestID,
		AskRequestID:        askRequestID,
		BaseVolatilitySize:  plan.BaseVolatilitySize,
		RiskPosition:        plan.RiskPosition,
		InventoryLimit:      plan.InventoryLimit,
		SizeSkewBps:         plan.SizeSkewBps,
		FullAdjustment:      plan.FullAdjustment,
		Adjustment:          plan.Adjustment,
		BidPrice:            bidPrice,
		AskPrice:            askPrice,
		BidQty:              plan.BidQty,
		AskQty:              plan.AskQty,
		PostOnly:            mm.cfg.PostOnly,
		CancelBeforeReplace: mm.cfg.PostOnlyCancelBeforeReplace,
		OutcomeExpectation:  expectation,
		CensorReason:        reason,
	})
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
	position := mm.riskPosition()
	fraction := float64(position) / float64(scale)
	if fraction > 1 {
		return 1
	}
	if fraction < -1 {
		return -1
	}
	return fraction
}

// referencePrice is what the maker quotes around. The boolean distinguishes
// a valid price from a missing/stale local observation; callers must defer
// instead of treating an int64 zero as a quoteable reference.
func (mm *StoikovMarketMaker) referencePrice() (int64, bool) {
	return mm.referencePriceAt(time.Time{})
}

func (mm *StoikovMarketMaker) referencePriceAt(now time.Time) (int64, bool) {
	book := mm.forward
	if book <= 0 {
		book = mm.cfg.BootstrapPrice
	}
	if book <= 0 {
		return 0, false
	}
	if !mm.cfg.AnchorToIndex || mm.indexPrice <= 0 {
		if mm.remoteReference != nil {
			localView, localOK := mm.localReference.View()
			remoteView, remoteOK := mm.remoteReference.View()
			if !localOK || !remoteOK {
				return 0, false
			}
			if mm.remoteMaxAge > 0 && !now.IsZero() {
				if now.UnixNano()-localView.PublishedAt > mm.remoteMaxAge.Nanoseconds() || now.UnixNano()-remoteView.PublishedAt > mm.remoteMaxAge.Nanoseconds() {
					return 0, false
				}
			}
			local, localOK := mm.localReference.Mid()
			remote, remoteOK := mm.remoteReference.Mid()
			if !localOK || !remoteOK {
				return 0, false
			}
			weight := mm.remoteWeight * mm.remoteConfidence
			composite := (1-weight)*float64(local) + weight*float64(remote)
			if !finite(composite) || composite <= 0 {
				return 0, false
			}
			return int64(composite), true
		}
		return book, true
	}
	weight := mm.cfg.IndexWeight
	if weight <= 0 || weight > 1 {
		weight = 1
	}
	blended := weight*float64(mm.indexPrice) + (1-weight)*float64(book)
	if !finite(blended) || blended <= 0 {
		return book, true
	}
	return int64(blended), true
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
