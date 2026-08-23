package multivenue

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/simulation"
	etypes "exchange_sim/types"
)

// CrossVenueArbConfig defines an explicitly prefunded, non-atomic spot route.
// It never shares collateral between venues and each market leg is FOK, so a
// failed counterpart remains visible as residual venue-local inventory.
type CrossVenueArbConfig struct {
	Symbol                      string
	LotQty                      int64
	BasePrecision               int64
	TakerFeeBps                 int64
	MaxAttempts                 int
	RequireCompleteFeedFrontier bool
	DecisionObserver            func(CrossVenueArbDecision)
}

// CrossVenueArbDecision is the observation-only evidence boundary for one
// submitted router leg. Components are the complete three-venue local public
// feed frontier that made the route eligible; they are not an actor input.
type CrossVenueArbDecision struct {
	ActorID       uint64
	ClientID      uint64
	TradingLinkID uint32
	Request       exchange.Request
	Components    []simulation.DecisionFrontierComponent
}

// CrossVenueArbLegConfig binds one router endpoint to one independently
// funded venue account. Actor IDs are global runner-order IDs; client IDs are
// local to their venue and therefore deliberately separate.
type CrossVenueArbLegConfig struct {
	VenueID  string
	ClientID uint64
	ActorID  uint64
	Gateway  actor.Gateway
}

// CrossVenueArb is one venue-qualified, non-atomic two-leg arbitrage router.
// It is not a transfer model: a completed buy on one venue and sell on another
// leaves offsetting local inventories that can only be moved by an explicit
// future transfer/rebalance policy.
type CrossVenueArb struct {
	tier   float64
	cfg    CrossVenueArbConfig
	legs   []*crossVenueArbLeg
	groups []*crossVenueArbGroup

	quoteGeneration       uint64
	lastAttemptGeneration uint64
	inFlight              *crossVenueArbGroup
	report                CrossVenueArbReport
}

// CrossVenueArbReport distinguishes observed quoted opportunities, submitted
// attempts, fully completed conversions, failures, and non-atomic residual.
// It is execution telemetry, not a marked-PnL claim.
type CrossVenueArbReport struct {
	Tier              float64                 `json:"tier"`
	RouterID          uint64                  `json:"router_id"`
	ExecutableSignals int                     `json:"executable_signals"`
	SubmittedGroups   int                     `json:"submitted_groups"`
	CompletedGroups   int                     `json:"completed_groups"`
	FailedGroups      int                     `json:"failed_groups"`
	PendingGroups     int                     `json:"pending_groups"`
	BuyFilledQty      int64                   `json:"buy_filled_qty"`
	SellFilledQty     int64                   `json:"sell_filled_qty"`
	BuyNotional       int64                   `json:"buy_notional"`
	SellNotional      int64                   `json:"sell_notional"`
	QuoteFees         int64                   `json:"quote_fees"`
	UnpricedFeeCount  int                     `json:"unpriced_fee_count"`
	CompletedCashflow int64                   `json:"completed_quote_cashflow"`
	CashflowGroups    int                     `json:"completed_cashflow_groups"`
	ResidualBaseQty   int64                   `json:"residual_base_qty"`
	Groups            []CrossVenueGroupReport `json:"groups"`
}

// CrossVenueGroupReport retains each two-leg outcome so an aggregate cannot
// hide a filled leg whose remote counterpart rejected.
type CrossVenueGroupReport struct {
	ID                 uint64              `json:"id"`
	BuyVenue           string              `json:"buy_venue"`
	SellVenue          string              `json:"sell_venue"`
	QuotedEdge         int64               `json:"quoted_edge"`
	Buy                CrossVenueLegReport `json:"buy"`
	Sell               CrossVenueLegReport `json:"sell"`
	Complete           bool                `json:"complete"`
	Failed             bool                `json:"failed"`
	QuoteCashflowValid bool                `json:"quote_cashflow_valid"`
	QuoteCashflow      int64               `json:"quote_cashflow"`
	ResidualBaseQty    int64               `json:"residual_base_qty"`
}

type CrossVenueLegReport struct {
	VenueID          string                `json:"venue_id"`
	ClientID         uint64                `json:"client_id"`
	Side             exchange.Side         `json:"side"`
	RequestID        uint64                `json:"request_id"`
	OrderID          uint64                `json:"order_id"`
	FilledQty        int64                 `json:"filled_qty"`
	Notional         int64                 `json:"notional"`
	QuoteFees        int64                 `json:"quote_fees"`
	UnpricedFeeCount int                   `json:"unpriced_fee_count"`
	Rejected         bool                  `json:"rejected"`
	RejectReason     exchange.RejectReason `json:"reject_reason"`
	Cancelled        bool                  `json:"cancelled"`
}

type crossVenueArbGroup struct {
	id         uint64
	quotedEdge int64
	buy        *crossVenueArbOrder
	sell       *crossVenueArbOrder
	frontiers  []simulation.DecisionFrontierComponent
	complete   bool
	failed     bool
}

type crossVenueArbOrder struct {
	leg *crossVenueArbLeg
	CrossVenueLegReport
}

func (o *crossVenueArbOrder) terminal(target int64) bool {
	return o.FilledQty >= target || o.Rejected || o.Cancelled
}

type crossVenueArbLeg struct {
	*actor.BaseActor
	owner    *CrossVenueArb
	venueID  string
	clientID uint64
	book     crossVenueQuoteBook
	frontier func() simulation.MarketDataFrontier
}

// NewCrossVenueArb creates one three-endpoint router. Each leg has one
// gateway/account and therefore cannot silently net or transfer balances
// between venues.
func NewCrossVenueArb(tier float64, cfg CrossVenueArbConfig, legs []CrossVenueArbLegConfig) (*CrossVenueArb, error) {
	if tier <= 0 || cfg.Symbol == "" || cfg.LotQty <= 0 || cfg.BasePrecision <= 0 || cfg.TakerFeeBps < 0 || cfg.MaxAttempts <= 0 {
		return nil, fmt.Errorf("multivenue: invalid cross-venue router config")
	}
	if len(legs) != 3 {
		return nil, fmt.Errorf("multivenue: cross-venue router requires exactly three venue legs")
	}
	router := &CrossVenueArb{tier: tier, cfg: cfg, report: CrossVenueArbReport{Tier: tier}}
	seenVenues := make(map[string]struct{}, len(legs))
	for _, spec := range legs {
		if spec.VenueID == "" || spec.ClientID == 0 || spec.ActorID == 0 || spec.Gateway == nil {
			return nil, fmt.Errorf("multivenue: incomplete cross-venue router leg")
		}
		if _, exists := seenVenues[spec.VenueID]; exists {
			return nil, fmt.Errorf("multivenue: duplicate router venue %q", spec.VenueID)
		}
		seenVenues[spec.VenueID] = struct{}{}
		leg := &crossVenueArbLeg{
			BaseActor: actor.NewBaseActor(spec.ActorID, spec.Gateway),
			owner:     router, venueID: spec.VenueID, clientID: spec.ClientID,
		}
		if source, ok := spec.Gateway.(interface {
			MarketDataFrontier() simulation.MarketDataFrontier
		}); ok {
			leg.frontier = source.MarketDataFrontier
		}
		if cfg.RequireCompleteFeedFrontier && leg.frontier == nil {
			return nil, fmt.Errorf("multivenue: cross-venue router leg %s lacks an auditable delayed feed frontier", spec.VenueID)
		}
		leg.SetHandler(leg)
		boundLeg := leg
		leg.SetOrderDecisionObserver(func(request exchange.Request) {
			router.observeDecision(boundLeg, request)
		})
		router.legs = append(router.legs, leg)
	}
	router.report.RouterID = router.legs[0].ID()
	return router, nil
}

// Actors exposes the independent venue endpoints for registration with the
// phase runner in deterministic venue/configuration order.
func (r *CrossVenueArb) Actors() []actor.Actor {
	actors := make([]actor.Actor, len(r.legs))
	for index, leg := range r.legs {
		actors[index] = leg
	}
	return actors
}

func (r *CrossVenueArb) SetTickerFactory(factory exchange.TickerFactory) {
	for _, leg := range r.legs {
		leg.SetTickerFactory(factory)
	}
}

func (r *CrossVenueArb) Tier() float64 { return r.tier }

func (r *CrossVenueArb) Report() CrossVenueArbReport {
	report := r.report
	report.Groups = make([]CrossVenueGroupReport, 0, len(r.groups))
	var residual int64
	for _, group := range r.groups {
		row := group.report()
		report.Groups = append(report.Groups, row)
		residual = checkedAdd(residual, row.ResidualBaseQty, "cross-venue residual")
		if row.Complete && row.QuoteCashflowValid {
			report.CompletedCashflow = checkedAdd(report.CompletedCashflow, row.QuoteCashflow, "cross-venue completed cashflow")
			report.CashflowGroups++
		}
		if !row.Complete && !row.Failed {
			report.PendingGroups++
		}
	}
	report.ResidualBaseQty = residual
	return report
}

func (g *crossVenueArbGroup) report() CrossVenueGroupReport {
	buy := g.buy.CrossVenueLegReport
	sell := g.sell.CrossVenueLegReport
	fees := checkedAdd(buy.QuoteFees, sell.QuoteFees, "cross-venue group quote fees")
	cashflow := checkedSub(checkedSub(sell.Notional, buy.Notional, "cross-venue group notional"), fees, "cross-venue group cashflow")
	return CrossVenueGroupReport{
		ID: g.id, BuyVenue: g.buy.leg.venueID, SellVenue: g.sell.leg.venueID,
		QuotedEdge: g.quotedEdge, Buy: buy, Sell: sell, Complete: g.complete,
		Failed: g.failed, QuoteCashflowValid: buy.UnpricedFeeCount == 0 && sell.UnpricedFeeCount == 0,
		QuoteCashflow: cashflow, ResidualBaseQty: checkedSub(buy.FilledQty, sell.FilledQty, "cross-venue group residual"),
	}
}

func (l *crossVenueArbLeg) Start(ctx context.Context) error {
	// Subscribe before entering the deterministic actor phase. Requests still
	// traverse this leg's configured, phase-owned network delay.
	l.Subscribe(l.owner.cfg.Symbol, exchange.MDSnapshot, exchange.MDDelta)
	return l.BaseActor.Start(ctx)
}

func (l *crossVenueArbLeg) HandleEvent(_ context.Context, event *actor.Event) {
	switch event.Type {
	case actor.EventBookSnapshot:
		e := event.Data.(actor.BookSnapshotEvent)
		if e.Symbol != l.owner.cfg.Symbol {
			return
		}
		l.book.reset(e.Snapshot)
		l.owner.onQuote(l)
	case actor.EventBookDelta:
		e := event.Data.(actor.BookDeltaEvent)
		if e.Symbol != l.owner.cfg.Symbol {
			return
		}
		l.book.apply(e.Delta)
		l.owner.onQuote(l)
	case actor.EventOrderAccepted:
		l.owner.onAccepted(l, event.Data.(actor.OrderAcceptedEvent))
	case actor.EventOrderRejected:
		l.owner.onRejected(l, event.Data.(actor.OrderRejectedEvent))
	case actor.EventOrderCancelled:
		l.owner.onCancelled(l, event.Data.(actor.OrderCancelledEvent))
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		l.owner.onFill(l, event.Data.(actor.OrderFillEvent))
	}
}

func (r *CrossVenueArb) onQuote(_ *crossVenueArbLeg) {
	r.quoteGeneration++
	if r.inFlight != nil || len(r.groups) >= r.cfg.MaxAttempts || r.quoteGeneration == r.lastAttemptGeneration {
		return
	}
	if _, ok := r.completeFeedFrontier(); !ok {
		return
	}
	buy, sell, edge, ok := r.bestOpportunity()
	if !ok {
		return
	}
	r.lastAttemptGeneration = r.quoteGeneration
	r.report.ExecutableSignals++
	r.openGroup(buy, sell, edge)
}

func (r *CrossVenueArb) bestOpportunity() (buy, sell *crossVenueArbLeg, edge int64, ok bool) {
	ordered := append([]*crossVenueArbLeg(nil), r.legs...)
	slices.SortFunc(ordered, func(left, right *crossVenueArbLeg) int { return strings.Compare(left.venueID, right.venueID) })
	for _, candidateBuy := range ordered {
		_, _, ask, askQty, quoteOK := candidateBuy.book.best()
		if !quoteOK || askQty < r.cfg.LotQty {
			continue
		}
		for _, candidateSell := range ordered {
			if candidateSell == candidateBuy {
				continue
			}
			bid, bidQty, _, _, quoteOK := candidateSell.book.best()
			if !quoteOK || bidQty < r.cfg.LotQty {
				continue
			}
			candidateEdge, edgeOK := r.executableEdge(bid, ask)
			if !edgeOK || candidateEdge <= 0 {
				continue
			}
			if !ok || candidateEdge > edge || (candidateEdge == edge && (candidateBuy.venueID < buy.venueID || (candidateBuy.venueID == buy.venueID && candidateSell.venueID < sell.venueID))) {
				buy, sell, edge, ok = candidateBuy, candidateSell, candidateEdge, true
			}
		}
	}
	return buy, sell, edge, ok
}

func (r *CrossVenueArb) executableEdge(sellBid, buyAsk int64) (int64, bool) {
	if sellBid <= 0 || buyAsk <= 0 {
		return 0, false
	}
	sellNotional, ok := etypes.TryMulDiv(r.cfg.LotQty, sellBid, r.cfg.BasePrecision)
	if !ok {
		return 0, false
	}
	buyNotional, ok := etypes.TryMulDiv(r.cfg.LotQty, buyAsk, r.cfg.BasePrecision)
	if !ok {
		return 0, false
	}
	sellFee, ok := etypes.TryMulBps(sellNotional, r.cfg.TakerFeeBps)
	if !ok {
		return 0, false
	}
	buyFee, ok := etypes.TryMulBps(buyNotional, r.cfg.TakerFeeBps)
	if !ok {
		return 0, false
	}
	proceeds, ok := etypes.TrySub(sellNotional, sellFee)
	if !ok {
		return 0, false
	}
	cost, ok := etypes.TryAdd(buyNotional, buyFee)
	if !ok {
		return 0, false
	}
	return etypes.TrySub(proceeds, cost)
}

func (r *CrossVenueArb) openGroup(buy, sell *crossVenueArbLeg, edge int64) {
	frontiers, ok := r.completeFeedFrontier()
	if !ok {
		return
	}
	group := &crossVenueArbGroup{
		id: uint64(len(r.groups) + 1), quotedEdge: edge,
		buy:       &crossVenueArbOrder{leg: buy, CrossVenueLegReport: CrossVenueLegReport{VenueID: buy.venueID, ClientID: buy.clientID, Side: exchange.Buy}},
		sell:      &crossVenueArbOrder{leg: sell, CrossVenueLegReport: CrossVenueLegReport{VenueID: sell.venueID, ClientID: sell.clientID, Side: exchange.Sell}},
		frontiers: frontiers,
	}
	// Install the in-flight group before either gateway sees a request: the
	// decision observer runs immediately before Send and must bind each leg to
	// the same comparison frontier. No actor receives this metadata.
	r.groups = append(r.groups, group)
	r.inFlight = group
	group.buy.RequestID = buy.SubmitOrderWithTimeInForce(r.cfg.Symbol, exchange.Buy, exchange.Market, 0, r.cfg.LotQty, exchange.FOK)
	group.sell.RequestID = sell.SubmitOrderWithTimeInForce(r.cfg.Symbol, exchange.Sell, exchange.Market, 0, r.cfg.LotQty, exchange.FOK)
	r.report.SubmittedGroups++
}

// completeFeedFrontier returns the exact frontiers used by the fixed
// three-venue comparison. In ordinary compatibility mode it has no effect on
// routing. An instrumented V2 router requires a nonempty prefix from every
// declared venue before it may turn quote state into a pair of order requests.
func (r *CrossVenueArb) completeFeedFrontier() ([]simulation.DecisionFrontierComponent, bool) {
	if !r.cfg.RequireCompleteFeedFrontier {
		return nil, true
	}
	components := make([]simulation.DecisionFrontierComponent, 0, len(r.legs))
	for _, leg := range r.legs {
		if leg.frontier == nil {
			return nil, false
		}
		frontier := leg.frontier()
		if frontier.LinkID == 0 || frontier.Ordinal == 0 || frontier.DeliveredAt == 0 || frontier.Digest == ([16]byte{}) {
			return nil, false
		}
		components = append(components, simulation.DecisionFrontierComponent{ClientID: leg.clientID, Frontier: frontier})
	}
	slices.SortFunc(components, func(left, right simulation.DecisionFrontierComponent) int {
		if left.ClientID != right.ClientID {
			return cmp.Compare(left.ClientID, right.ClientID)
		}
		return cmp.Compare(left.Frontier.LinkID, right.Frontier.LinkID)
	})
	return components, true
}

func (r *CrossVenueArb) observeDecision(leg *crossVenueArbLeg, request exchange.Request) {
	if r.cfg.DecisionObserver == nil || request.Type != exchange.ReqPlaceOrder || request.OrderReq == nil || r.inFlight == nil {
		return
	}
	if leg.frontier == nil {
		panic("multivenue: instrumented cross-venue route lacks trading-link frontier")
	}
	tradingFrontier := leg.frontier()
	for _, component := range r.inFlight.frontiers {
		// Client IDs are venue-local, so the same number can legitimately
		// identify all three router accounts. Match both account and link; a
		// client-only match would bind a sell decision to another venue's
		// scalar gateway record.
		if component.ClientID != leg.clientID || component.Frontier.LinkID != tradingFrontier.LinkID {
			continue
		}
		decision := CrossVenueArbDecision{
			ActorID: leg.ID(), ClientID: leg.clientID, TradingLinkID: component.Frontier.LinkID,
			Request: request, Components: append([]simulation.DecisionFrontierComponent(nil), r.inFlight.frontiers...),
		}
		r.cfg.DecisionObserver(decision)
		return
	}
	panic("multivenue: instrumented cross-venue route missing trading-link frontier")
}

func (r *CrossVenueArb) findOrder(leg *crossVenueArbLeg, requestID, orderID uint64) *crossVenueArbOrder {
	for _, group := range r.groups {
		for _, candidate := range []*crossVenueArbOrder{group.buy, group.sell} {
			if candidate.leg != leg {
				continue
			}
			if requestID != 0 && candidate.RequestID == requestID {
				return candidate
			}
			if orderID != 0 && candidate.OrderID == orderID {
				return candidate
			}
		}
	}
	return nil
}

func (r *CrossVenueArb) onAccepted(leg *crossVenueArbLeg, event actor.OrderAcceptedEvent) {
	if order := r.findOrder(leg, event.RequestID, 0); order != nil {
		order.OrderID = event.OrderID
	}
}

func (r *CrossVenueArb) onRejected(leg *crossVenueArbLeg, event actor.OrderRejectedEvent) {
	if order := r.findOrder(leg, event.RequestID, 0); order != nil {
		order.Rejected, order.RejectReason = true, event.Reason
		r.refreshGroup(order)
	}
}

func (r *CrossVenueArb) onCancelled(leg *crossVenueArbLeg, event actor.OrderCancelledEvent) {
	if order := r.findOrder(leg, event.RequestID, event.OrderID); order != nil {
		order.Cancelled = true
		r.refreshGroup(order)
	}
}

func (r *CrossVenueArb) onFill(leg *crossVenueArbLeg, event actor.OrderFillEvent) {
	order := r.findOrder(leg, 0, event.OrderID)
	if order == nil {
		return
	}
	notional, ok := etypes.TryMulDiv(event.Qty, event.Price, r.cfg.BasePrecision)
	if !ok {
		panic("multivenue: cross-venue fill notional overflows")
	}
	order.FilledQty = checkedAdd(order.FilledQty, event.Qty, "cross-venue leg fill")
	order.Notional = checkedAdd(order.Notional, notional, "cross-venue leg notional")
	if event.FeeAmount != 0 {
		if event.FeeAsset == "USD" {
			order.QuoteFees = checkedAdd(order.QuoteFees, event.FeeAmount, "cross-venue quote fee")
			r.report.QuoteFees = checkedAdd(r.report.QuoteFees, event.FeeAmount, "cross-venue aggregate quote fee")
		} else {
			order.UnpricedFeeCount++
			r.report.UnpricedFeeCount++
		}
	}
	if order.Side == exchange.Buy {
		r.report.BuyFilledQty = checkedAdd(r.report.BuyFilledQty, event.Qty, "cross-venue aggregate buy fill")
		r.report.BuyNotional = checkedAdd(r.report.BuyNotional, notional, "cross-venue aggregate buy notional")
	} else {
		r.report.SellFilledQty = checkedAdd(r.report.SellFilledQty, event.Qty, "cross-venue aggregate sell fill")
		r.report.SellNotional = checkedAdd(r.report.SellNotional, notional, "cross-venue aggregate sell notional")
	}
	r.refreshGroup(order)
}

func (r *CrossVenueArb) refreshGroup(order *crossVenueArbOrder) {
	var group *crossVenueArbGroup
	for _, candidate := range r.groups {
		if candidate.buy == order || candidate.sell == order {
			group = candidate
			break
		}
	}
	if group == nil || group.complete || group.failed || !group.buy.terminal(r.cfg.LotQty) || !group.sell.terminal(r.cfg.LotQty) {
		return
	}
	if group.buy.FilledQty == r.cfg.LotQty && group.sell.FilledQty == r.cfg.LotQty {
		group.complete = true
		r.report.CompletedGroups++
	} else {
		group.failed = true
		r.report.FailedGroups++
	}
	if r.inFlight == group {
		r.inFlight = nil
	}
}

type crossVenueQuoteBook struct {
	bids map[int64]int64
	asks map[int64]int64
}

func (b *crossVenueQuoteBook) reset(snapshot *exchange.BookSnapshot) {
	b.bids = make(map[int64]int64, len(snapshot.Bids))
	b.asks = make(map[int64]int64, len(snapshot.Asks))
	for _, level := range snapshot.Bids {
		if level.VisibleQty > 0 {
			b.bids[level.Price] = level.VisibleQty
		}
	}
	for _, level := range snapshot.Asks {
		if level.VisibleQty > 0 {
			b.asks[level.Price] = level.VisibleQty
		}
	}
}

func (b *crossVenueQuoteBook) apply(delta *exchange.BookDelta) {
	levels := b.bids
	if delta.Side == exchange.Sell {
		levels = b.asks
	}
	if levels == nil {
		return
	}
	if delta.VisibleQty <= 0 {
		delete(levels, delta.Price)
		return
	}
	levels[delta.Price] = delta.VisibleQty
}

func (b *crossVenueQuoteBook) best() (bid, bidQty, ask, askQty int64, ok bool) {
	for price, qty := range b.bids {
		if qty > 0 && price > bid {
			bid, bidQty = price, qty
		}
	}
	for price, qty := range b.asks {
		if qty > 0 && (ask == 0 || price < ask) {
			ask, askQty = price, qty
		}
	}
	return bid, bidQty, ask, askQty, bid > 0 && ask > 0
}

func checkedAdd(left, right int64, field string) int64 {
	value, ok := etypes.TryAdd(left, right)
	if !ok {
		panic("multivenue: " + field + " overflows")
	}
	return value
}

func checkedSub(left, right int64, field string) int64 {
	value, ok := etypes.TrySub(left, right)
	if !ok {
		panic("multivenue: " + field + " overflows")
	}
	return value
}
