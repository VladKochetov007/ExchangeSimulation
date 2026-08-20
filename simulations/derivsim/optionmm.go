package derivsim

import (
	"context"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	eprice "exchange_sim/price"
)

// OptionMMConfig drives a market-opening option dealer: it discovers every
// listed option on Underlying from the instrument feed, quotes around the
// Black-76 theoretical value with a simple linear inventory skew, and
// delta-hedges its aggregate option inventory in the underlying.
//
// This is deliberately not labelled Avellaneda-Stoikov: it has no calibrated
// fill-intensity curve, risk-aversion term, volatility estimate, or finite
// horizon reservation price. It is the baseline against which that model will
// be evaluated.
type OptionMMConfig struct {
	Underlying string
	IV         float64
	// VolModel supplies the volatility this dealer prices and hedges with.
	// Nil prices every contract at IV, which is what the population did before
	// dealers were allowed to disagree.
	VolModel eprice.VolatilityModel
	// SpreadBps is the half-spread around theo, in bps of the underlying price.
	SpreadBps int64
	// SkewPerLotBps shifts both quotes against inventory, in bps of the
	// underlying price per lot of net contract inventory.
	SkewPerLotBps int64
	QuoteQty      int64         // per side per contract
	LotQty        int64         // inventory normalization unit
	PremiumTick   int64         // option book tick
	QuoteInterval time.Duration // requote cadence

	// HedgeEnabled turns on delta hedging in the underlying.
	HedgeEnabled  bool
	HedgeInterval time.Duration
	// HedgeBandQty: rebalance only when |target − current| exceeds this.
	HedgeBandQty int64
	// HedgePolicy decides the hedge a dealer holds against its option book.
	// Nil keeps the running delta hedge, which is what HedgeEnabled turned on
	// before the policy was separable. The two differ in what they leave
	// exposed: a hedge set once at the trade carries the whole gamma, and a
	// hedge rebalanced on a band pays the spread repeatedly to shed it.
	HedgePolicy HedgePolicy

	// GreekInterval samples the live option-book exposure after the actor has
	// consumed the ordered exchange feed. Zero disables telemetry for callers
	// that only need quoting behavior.
	GreekInterval time.Duration

	BasePrecision int64
}

// GreekProfile is the dealer's Black-76 sensitivity snapshot. ModelForward is
// currently the spot-mid proxy used by the baseline simulator; ForwardSource
// makes that approximation visible to downstream analysis. Delta is in
// underlying base units, gamma in base units per quote-price unit, and vega in
// quote units per one-unit annualized-volatility move. HedgeDelta is the spot
// hedge inventory; NetDelta is option delta plus that hedge.
type GreekProfile struct {
	Timestamp         int64   `json:"timestamp"`
	Phase             string  `json:"phase"`
	SpotMid           int64   `json:"spot_mid"`
	ModelForward      int64   `json:"model_forward"`
	ForwardSource     string  `json:"forward_source"`
	ImpliedVolatility float64 `json:"implied_volatility"`
	OptionDelta       float64 `json:"option_delta"`
	HedgeDelta        float64 `json:"hedge_delta"`
	NetDelta          float64 `json:"net_delta"`
	Gamma             float64 `json:"gamma"`
	Vega              float64 `json:"vega"`
	Contracts         int     `json:"contracts"`
}

// GreekPosition is one non-zero option position at a sampling point. It keeps
// contract identity and tenor intact so that a final aggregate containing long
// options cannot be misread as the terminal state of a short-expiry board.
type GreekPosition struct {
	Timestamp         int64   `json:"timestamp"`
	Phase             string  `json:"phase"`
	Symbol            string  `json:"symbol"`
	Underlying        string  `json:"underlying"`
	ListedNano        int64   `json:"listed_nano"`
	ExpiryNano        int64   `json:"expiry_nano"`
	Strike            int64   `json:"strike"`
	IsCall            bool    `json:"is_call"`
	Position          int64   `json:"position"`
	TimeToExpiryNano  int64   `json:"time_to_expiry_nano"`
	SpotMid           int64   `json:"spot_mid"`
	ModelForward      int64   `json:"model_forward"`
	ForwardSource     string  `json:"forward_source"`
	ImpliedVolatility float64 `json:"implied_volatility"`
	Delta             float64 `json:"delta"`
	Gamma             float64 `json:"gamma"`
	Vega              float64 `json:"vega"`
}

type optionQuotes struct {
	bidID, askID       uint64
	bidPrice, askPrice int64
	inventory          int64 // signed contracts in base units
	// pendingBid/pendingAsk gate requoting while a submit is in flight:
	// re-submitting before the accept lands would orphan the unacked order
	// as an uncancellable zombie quote.
	pendingBid, pendingAsk bool
}

// quoteRef ties an in-flight quote request to its contract and book side so
// the accept can be routed back into per-contract quote state.
type quoteRef struct {
	sym   string
	isBid bool
}

// OptionMarketMaker opens and quotes every option listed on its underlying.
type OptionMarketMaker struct {
	*actor.BaseActor
	cfg OptionMMConfig

	set      *contractSet
	quotes   map[string]*optionQuotes
	pending  map[uint64]quoteRef
	spotMid  int64
	hedgePos int64 // underlying inventory from hedge fills
	// hedgePending is signed submitted quantity not yet resolved by fills,
	// cancellation, or rejection. Including it in the next target prevents a
	// delayed hedge acknowledgement from causing repeated over-correction.
	hedgePending  int64
	hedgeRequests map[uint64]int64 // request ID -> signed requested quantity
	hedgeOrders   map[uint64]int64 // order ID -> signed unresolved quantity
	hedgeQty      int64            // cumulative |hedge| traded, for reporting
	// unhedgedDelta is option delta taken on since the last hedge, which is
	// what a per-trade hedge covers. lastHedgeNano is when that hedge went out.
	unhedgedDelta float64
	lastHedgeNano int64
	profiles      []GreekProfile
	positions     []GreekPosition
	subscribed    bool
}

func NewOptionMarketMaker(id uint64, gw actor.Gateway, cfg OptionMMConfig) *OptionMarketMaker {
	mm := &OptionMarketMaker{
		BaseActor:     actor.NewBaseActor(id, gw),
		cfg:           cfg,
		set:           newContractSet(cfg.Underlying),
		quotes:        make(map[string]*optionQuotes),
		pending:       make(map[uint64]quoteRef),
		hedgeRequests: make(map[uint64]int64),
		hedgeOrders:   make(map[uint64]int64),
	}
	mm.set.onList = func(c *Contract) {
		if c.Type == "OPTION" {
			mm.quotes[c.Symbol] = &optionQuotes{}
		}
	}
	mm.set.onSettle = func(c *Contract, _ int64) { delete(mm.quotes, c.Symbol) }
	mm.set.onFill = mm.onFill
	mm.set.onAccept = mm.onQuoteAccepted
	mm.set.onReject = mm.onQuoteRejected
	mm.SetHandler(mm)
	mm.AddTicker(cfg.QuoteInterval, mm.onQuoteTick)
	if cfg.HedgeEnabled {
		mm.AddTicker(cfg.HedgeInterval, mm.onHedgeTick)
	}
	if cfg.GreekInterval > 0 {
		mm.AddTicker(cfg.GreekInterval, mm.onGreekTick)
	}
	return mm
}

// HedgeTraded reports cumulative hedge volume (base units) for diagnostics.
func (mm *OptionMarketMaker) HedgeTraded() int64 { return mm.hedgeQty }
func (mm *OptionMarketMaker) HedgePosition() int64 {
	return mm.hedgePos
}

// GreekProfiles returns a stable copy so post-run analysis cannot mutate
// telemetry retained by the actor.
func (mm *OptionMarketMaker) GreekProfiles() []GreekProfile {
	return append([]GreekProfile(nil), mm.profiles...)
}

// GreekPositionProfiles returns immutable per-contract rows for expiry/strike
// attribution. It intentionally excludes resting orders, which are desired
// exposure rather than executed portfolio risk.
func (mm *OptionMarketMaker) GreekPositionProfiles() []GreekPosition {
	return append([]GreekPosition(nil), mm.positions...)
}

func (mm *OptionMarketMaker) HandleEvent(_ context.Context, evt *actor.Event) {
	if evt.Type == actor.EventBookSnapshot {
		e := evt.Data.(actor.BookSnapshotEvent)
		if e.Symbol == mm.cfg.Underlying {
			if len(e.Snapshot.Bids) > 0 && len(e.Snapshot.Asks) > 0 {
				mm.spotMid = (e.Snapshot.Bids[0].Price + e.Snapshot.Asks[0].Price) / 2
				mm.observeUnderlying(e.Timestamp)
			}
		}
		return
	}
	if mm.handleHedgeEvent(evt) {
		return
	}
	mm.set.handle(evt)
}

func (mm *OptionMarketMaker) handleHedgeEvent(evt *actor.Event) bool {
	switch evt.Type {
	case actor.EventOrderAccepted:
		e := evt.Data.(actor.OrderAcceptedEvent)
		pending, ok := mm.hedgeRequests[e.RequestID]
		if !ok {
			return false
		}
		delete(mm.hedgeRequests, e.RequestID)
		mm.hedgeOrders[e.OrderID] = pending
		return true
	case actor.EventOrderRejected:
		e := evt.Data.(actor.OrderRejectedEvent)
		pending, ok := mm.hedgeRequests[e.RequestID]
		if !ok {
			return false
		}
		delete(mm.hedgeRequests, e.RequestID)
		mm.hedgePending -= pending
		return true
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		e := evt.Data.(actor.OrderFillEvent)
		if _, ok := mm.hedgeOrders[e.OrderID]; !ok {
			return false
		}
		filled := signedQty(e)
		mm.hedgePos += filled
		mm.hedgePending -= filled
		mm.hedgeQty += e.Qty
		if e.IsFull {
			delete(mm.hedgeOrders, e.OrderID)
		}
		return true
	case actor.EventOrderCancelled:
		e := evt.Data.(actor.OrderCancelledEvent)
		pending, ok := mm.hedgeOrders[e.OrderID]
		if !ok {
			return false
		}
		if pending > 0 {
			mm.hedgePending -= e.RemainingQty
		} else {
			mm.hedgePending += e.RemainingQty
		}
		delete(mm.hedgeOrders, e.OrderID)
		return true
	}
	return false
}

// onQuoteAccepted records the live order ID for a quote so it can be
// cancelled on requote. An accept for a contract that settled in flight is
// orphan-cancelled immediately.
func (mm *OptionMarketMaker) onQuoteAccepted(_ string, reqID, orderID uint64) {
	ref, ok := mm.pending[reqID]
	if !ok {
		return // hedge order, tracked by fills only
	}
	delete(mm.pending, reqID)
	q := mm.quotes[ref.sym]
	if q == nil {
		mm.CancelOrder(orderID)
		return
	}
	if ref.isBid {
		q.bidID = orderID
		q.pendingBid = false
	} else {
		q.askID = orderID
		q.pendingAsk = false
	}
}

func (mm *OptionMarketMaker) onQuoteRejected(reqID uint64) {
	ref, ok := mm.pending[reqID]
	if !ok {
		return
	}
	delete(mm.pending, reqID)
	if q := mm.quotes[ref.sym]; q != nil {
		if ref.isBid {
			q.pendingBid = false
		} else {
			q.pendingAsk = false
		}
	}
}

func (mm *OptionMarketMaker) onFill(sym string, e actor.OrderFillEvent) {
	if q, ok := mm.quotes[sym]; ok {
		filled := signedQty(e)
		q.inventory += filled
		mm.unhedgedDelta += mm.contractDelta(sym, e.Timestamp) * float64(filled)
		if e.IsFull {
			if e.Side == exchange.Buy {
				q.bidID = 0
			} else {
				q.askID = 0
			}
		}
	}
}

// contractDelta is the delta of one unit of a contract at the moment of a
// trade, which is what a per-trade hedge is sized from.
func (mm *OptionMarketMaker) contractDelta(symbol string, nano int64) float64 {
	c := mm.set.contracts[symbol]
	if c == nil || c.Type != "OPTION" || mm.spotMid <= 0 {
		return 0
	}
	yearsLeft := float64(c.ExpiryNano-nano) / float64(365*24*time.Hour)
	if yearsLeft <= 0 {
		return 0
	}
	return eprice.Black76Delta(mm.spotMid, c.Strike, mm.volatility(c.Strike, yearsLeft, c.IsCall), yearsLeft, c.IsCall)
}

func (mm *OptionMarketMaker) onQuoteTick(t time.Time) {
	if !mm.subscribed {
		mm.Subscribe(exchange.InstrumentFeedSymbol, exchange.MDInstrument)
		mm.Subscribe(mm.cfg.Underlying, exchange.MDSnapshot)
		mm.subscribed = true
		return
	}
	if mm.spotMid == 0 {
		return
	}
	now := t.UnixNano()
	for _, c := range mm.set.orderedContracts() {
		if c.Type != "OPTION" {
			continue
		}
		q := mm.quotes[c.Symbol]
		if q == nil {
			continue
		}
		mm.requoteContract(c.Symbol, c, q, now)
	}
}

func (mm *OptionMarketMaker) requoteContract(sym string, c *Contract, q *optionQuotes, now int64) {
	yearsLeft := float64(c.ExpiryNano-now) / float64(365*24*time.Hour)
	if yearsLeft <= 0 {
		return
	}
	theo := eprice.Black76Premium(mm.spotMid, c.Strike, mm.volatility(c.Strike, yearsLeft, c.IsCall), yearsLeft, c.IsCall)
	half := mm.spotMid * mm.cfg.SpreadBps / 10000
	skew := int64(0)
	if mm.cfg.LotQty > 0 {
		skew = mm.spotMid * mm.cfg.SkewPerLotBps / 10000 * q.inventory / mm.cfg.LotQty
	}
	tick := mm.cfg.PremiumTick
	bid := alignDown(theo-half-skew, tick)
	ask := alignUp(theo+half-skew, tick)
	if ask <= 0 {
		return
	}
	if q.pendingBid || q.pendingAsk {
		return
	}
	if bid == q.bidPrice && ask == q.askPrice && q.bidID != 0 && q.askID != 0 {
		return
	}
	if q.bidID != 0 {
		mm.CancelOrder(q.bidID)
		q.bidID = 0
	}
	if q.askID != 0 {
		mm.CancelOrder(q.askID)
		q.askID = 0
	}
	if bid > 0 {
		reqID := mm.SubmitOrder(sym, exchange.Buy, exchange.LimitOrder, bid, mm.cfg.QuoteQty)
		mm.set.trackRequest(reqID, sym)
		mm.pending[reqID] = quoteRef{sym: sym, isBid: true}
		q.pendingBid = true
		q.bidPrice = bid
	}
	reqID := mm.SubmitOrder(sym, exchange.Sell, exchange.LimitOrder, ask, mm.cfg.QuoteQty)
	mm.set.trackRequest(reqID, sym)
	mm.pending[reqID] = quoteRef{sym: sym, isBid: false}
	q.pendingAsk = true
	q.askPrice = ask
}

// observeUnderlying feeds the dealer's volatility model the price path it
// estimates from. A model that does not estimate ignores it.
func (mm *OptionMarketMaker) observeUnderlying(nano int64) {
	if observer, ok := mm.cfg.VolModel.(eprice.PriceObserver); ok {
		observer.Observe(mm.spotMid, nano)
	}
}

// volatility is the volatility this dealer prices one contract at. A model
// that declines to price falls back to the configured level, so a dealer whose
// estimator has not warmed up still quotes rather than leaving the book empty.
func (mm *OptionMarketMaker) volatility(strike int64, yearsLeft float64, isCall bool) float64 {
	if mm.cfg.VolModel != nil {
		if vol := mm.cfg.VolModel.Volatility(mm.spotMid, strike, yearsLeft, isCall); vol > 0 {
			return vol
		}
	}
	return mm.cfg.IV
}

// Exposures reports every non-zero option position the dealer holds, in the
// form a hedging desk consumes. A desk that lays off a dealer's wings needs
// the positions themselves, not their aggregate greeks, because it prices them
// with its own volatility rather than the dealer's.
func (mm *OptionMarketMaker) Exposures() []ContractExposure {
	var exposures []ContractExposure
	for _, c := range mm.set.orderedContracts() {
		if c.Type != "OPTION" {
			continue
		}
		q := mm.quotes[c.Symbol]
		if q == nil || q.inventory == 0 {
			continue
		}
		exposures = append(exposures, ContractExposure{
			Symbol: c.Symbol, Strike: c.Strike, IsCall: c.IsCall,
			ExpiryNano: c.ExpiryNano, Position: q.inventory,
		})
	}
	return exposures
}

// PricingVolatility exposes the volatility this dealer would price a contract
// at, so a report about the dealer's book can be marked with the dealer's own
// view rather than the venue's.
func (mm *OptionMarketMaker) PricingVolatility(strike int64, yearsLeft float64, isCall bool) float64 {
	return mm.volatility(strike, yearsLeft, isCall)
}

// OptionInventory reports the dealer's signed position in one contract, in
// base units. It is what an inventory-sensitive volatility model reads.
func (mm *OptionMarketMaker) OptionInventory(symbol string) int64 {
	if q := mm.quotes[symbol]; q != nil {
		return q.inventory
	}
	return 0
}

// onHedgeTick trades the underlying toward delta neutrality of the whole book.
func (mm *OptionMarketMaker) onHedgeTick(t time.Time) {
	if mm.spotMid == 0 {
		return
	}
	now := t.UnixNano()
	var netDelta float64
	for _, c := range mm.set.orderedContracts() {
		if c.Type != "OPTION" {
			continue
		}
		q := mm.quotes[c.Symbol]
		if q == nil || q.inventory == 0 {
			continue
		}
		yearsLeft := float64(c.ExpiryNano-now) / float64(365*24*time.Hour)
		delta := eprice.Black76Delta(mm.spotMid, c.Strike, mm.volatility(c.Strike, yearsLeft, c.IsCall), yearsLeft, c.IsCall)
		netDelta += delta * float64(q.inventory)
	}
	policy := mm.cfg.HedgePolicy
	if policy == nil {
		policy = BandedDeltaHedge{}
	}
	gap := policy.Hedge(HedgeState{
		NetDelta:      netDelta,
		HedgePosition: mm.hedgePos,
		HedgePending:  mm.hedgePending,
		TradedDelta:   mm.unhedgedDelta,
		SpotMid:       mm.spotMid,
		Nano:          now,
		BandQty:       mm.cfg.HedgeBandQty,
		LastNano:      mm.lastHedgeNano,
	})
	if gap == 0 {
		return
	}
	side := exchange.Buy
	qty := gap
	if gap < 0 {
		side, qty = exchange.Sell, -gap
	}
	reqID := mm.SubmitOrder(mm.cfg.Underlying, side, exchange.Market, 0, qty)
	mm.hedgeRequests[reqID] = gap
	mm.hedgePending += gap
	mm.lastHedgeNano = now
	// A policy that hedges each trade once has now covered everything it took
	// on; one that targets the whole book never accumulates this in the first
	// place, since it reads NetDelta.
	mm.unhedgedDelta = 0
}

func (mm *OptionMarketMaker) onGreekTick(t time.Time) {
	positions := mm.GreekPositions(t)
	profile := mm.aggregateGreekProfile(t, positions)
	if profile.SpotMid != 0 {
		mm.profiles = append(mm.profiles, profile)
		mm.positions = append(mm.positions, positions...)
	}
}

// GreekProfile computes a mark-time sensitivity snapshot from the dealer's
// filled option inventory and underlying hedge. It intentionally does not
// include resting orders: Greeks are exposures, not desired exposure.
func (mm *OptionMarketMaker) GreekProfile(t time.Time) GreekProfile {
	return mm.aggregateGreekProfile(t, mm.GreekPositions(t))
}

// GreekPositions computes signed per-contract sensitivities from filled
// inventory. The current forward is explicitly tagged as the spot-mid proxy;
// a future maturity-matched forward provider can replace that input without
// changing the report schema.
func (mm *OptionMarketMaker) GreekPositions(t time.Time) []GreekPosition {
	if mm.spotMid <= 0 || mm.cfg.BasePrecision <= 0 {
		return nil
	}
	now := t.UnixNano()
	positions := make([]GreekPosition, 0)
	for _, c := range mm.set.orderedContracts() {
		if c.Type != "OPTION" {
			continue
		}
		q := mm.quotes[c.Symbol]
		if q == nil || q.inventory == 0 {
			continue
		}
		timeToExpiry := c.ExpiryNano - now
		yearsLeft := float64(timeToExpiry) / float64(365*24*time.Hour)
		if yearsLeft <= 0 {
			continue
		}
		contractVol := mm.volatility(c.Strike, yearsLeft, c.IsCall)
		sensitivity, ok := eprice.Black76Sensitivities(mm.spotMid, c.Strike, contractVol, yearsLeft, c.IsCall)
		if !ok {
			continue
		}
		contracts := float64(q.inventory) / float64(mm.cfg.BasePrecision)
		positions = append(positions, GreekPosition{
			Timestamp:         now,
			Phase:             "post_quote_pre_hedge_fill",
			Symbol:            c.Symbol,
			Underlying:        mm.cfg.Underlying,
			ListedNano:        c.ListedNano,
			ExpiryNano:        c.ExpiryNano,
			Strike:            c.Strike,
			IsCall:            c.IsCall,
			Position:          q.inventory,
			TimeToExpiryNano:  timeToExpiry,
			SpotMid:           mm.spotMid,
			ModelForward:      mm.spotMid,
			ForwardSource:     "spot_mid_proxy",
			ImpliedVolatility: contractVol,
			Delta:             contracts * sensitivity.Delta,
			Gamma:             contracts * sensitivity.Gamma,
			Vega:              contracts * sensitivity.Vega,
		})
	}
	return positions
}

func (mm *OptionMarketMaker) aggregateGreekProfile(t time.Time, positions []GreekPosition) GreekProfile {
	profile := GreekProfile{
		Timestamp:         t.UnixNano(),
		Phase:             "post_quote_pre_hedge_fill",
		SpotMid:           mm.spotMid,
		ModelForward:      mm.spotMid,
		ForwardSource:     "spot_mid_proxy",
		ImpliedVolatility: mm.volatility(0, 0, true),
	}
	for _, position := range positions {
		profile.OptionDelta += position.Delta
		profile.Gamma += position.Gamma
		profile.Vega += position.Vega
		profile.Contracts++
	}
	profile.HedgeDelta = float64(mm.hedgePos) / float64(mm.cfg.BasePrecision)
	profile.NetDelta = profile.OptionDelta + profile.HedgeDelta
	return profile
}

func alignDown(price, tick int64) int64 {
	if price <= 0 {
		return 0
	}
	return (price / tick) * tick
}

func alignUp(price, tick int64) int64 {
	if price <= 0 {
		return tick
	}
	return ((price + tick - 1) / tick) * tick
}
