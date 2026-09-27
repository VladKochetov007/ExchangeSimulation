package repeatedspot

import (
	"errors"
	"math/big"

	worldspot "exchange_sim/simulations/repeatedspot"
	"exchange_sim/types"
)

type SignalSnapshotOwnership struct {
	ClientID       uint64 `json:"client_id"`
	SourceSequence uint64 `json:"source_sequence"`
	SourceAt       int64  `json:"source_at_ns"`
	DeliveredAt    int64  `json:"delivered_at_ns"`
	BestBid        int64  `json:"best_bid"`
	BestAsk        int64  `json:"best_ask"`
	PublicBidQty   int64  `json:"public_bid_base_units"`
	PublicAskQty   int64  `json:"public_ask_base_units"`
	OwnBidQty      int64  `json:"own_bid_base_units"`
	OwnAskQty      int64  `json:"own_ask_base_units"`
	OwnOnlyBid     bool   `json:"own_only_bid"`
	OwnOnlyAsk     bool   `json:"own_only_ask"`
}

type SignalMakerFunnel struct {
	ClientID               uint64 `json:"client_id"`
	ScheduledDecisions     int64  `json:"scheduled_decisions"`
	ProcessedBookDecisions int64  `json:"processed_book_decisions"`
	FreshTwoSidedDepth     int64  `json:"fresh_two_sided_depth_decisions"`
	UnequalTopDepth        int64  `json:"unequal_top_depth_decisions"`
	UsableLocalReference   int64  `json:"usable_local_reference_decisions"`
	BaseQuoteUsable        int64  `json:"base_quote_usable_decisions"`
	ShadowGain2Changes     int64  `json:"shadow_gain2_changes_decisions"`
	PendingResponse        int64  `json:"pending_response_decisions"`
	QuoteEvaluations       int64  `json:"quote_evaluations"`
	PlacementEvaluations   int64  `json:"placement_evaluations"`
	ActualShiftedTargets   int64  `json:"actual_shifted_target_decisions"`
	ShadowRiskFeasible     int64  `json:"shadow_decisions_with_risk_feasible_side"`
	ShadowPlaceSent        int64  `json:"shadow_decisions_with_place_send"`
	ShadowAccepted         int64  `json:"shadow_decisions_with_accepted_place"`
	ShadowRejected         int64  `json:"shadow_decisions_with_rejected_place"`
	ShadowRestingObserved  int64  `json:"shadow_decisions_with_source_snapshot_resting"`
	ShadowBestObserved     int64  `json:"shadow_decisions_with_source_snapshot_best"`
	ShadowFilled           int64  `json:"shadow_decisions_with_fill"`
	ShadowUnresolved       int64  `json:"shadow_decisions_with_unresolved_send"`
}

type SignalRequestTrace struct {
	RequestID             uint64 `json:"request_id"`
	Kind                  string `json:"kind"`
	Side                  string `json:"side,omitempty"`
	Price                 int64  `json:"price,omitempty"`
	Qty                   int64  `json:"qty,omitempty"`
	SentAt                int64  `json:"sent_at_ns"`
	VenueOutcome          string `json:"venue_outcome,omitempty"`
	VenueAt               int64  `json:"venue_at_ns,omitempty"`
	OrderID               uint64 `json:"order_id,omitempty"`
	RejectionReason       string `json:"rejection_reason,omitempty"`
	ObservedRestingSource bool   `json:"observed_resting_in_public_source_snapshot"`
	ObservedAtBestSource  bool   `json:"observed_at_public_best_in_source_snapshot"`
	FillCount             int64  `json:"fill_count"`
	FilledBaseUnits       int64  `json:"filled_base_units"`
}

type SignalDecisionTrace struct {
	ClientID            uint64                `json:"client_id"`
	DecisionAt          int64                 `json:"decision_at_ns"`
	LocalSourceSequence uint64                `json:"local_source_sequence"`
	LocalSourceAt       int64                 `json:"local_source_at_ns"`
	Action              string                `json:"action"`
	FreshTwoSidedDepth  bool                  `json:"fresh_two_sided_depth"`
	ShadowGain2Changes  bool                  `json:"shadow_gain2_changes_quote"`
	BidRiskFeasible     *bool                 `json:"bid_risk_feasible,omitempty"`
	AskRiskFeasible     *bool                 `json:"ask_risk_feasible,omitempty"`
	TargetBid           int64                 `json:"target_bid"`
	TargetAsk           int64                 `json:"target_ask"`
	Requests            []*SignalRequestTrace `json:"requests"`
}

type SignalWindowBookSummary struct {
	WindowNanos               int64  `json:"window_ns"`
	BidPresentNanos           int64  `json:"bid_present_ns"`
	AskPresentNanos           int64  `json:"ask_present_ns"`
	TwoSidedNanos             int64  `json:"two_sided_ns"`
	BestBidBaseUnitNanos      string `json:"best_bid_base_unit_ns"`
	BestAskBaseUnitNanos      string `json:"best_ask_base_unit_ns"`
	SpreadPriceUnitNanos      string `json:"two_sided_spread_price_unit_ns"`
	MakerBestBidBaseUnitNanos string `json:"maker_best_bid_base_unit_ns"`
	MakerBestAskBaseUnitNanos string `json:"maker_best_ask_base_unit_ns"`
	SeedBestBidBaseUnitNanos  string `json:"seed_best_bid_base_unit_ns"`
	SeedBestAskBaseUnitNanos  string `json:"seed_best_ask_base_unit_ns"`
	OtherBestBidBaseUnitNanos string `json:"other_best_bid_base_unit_ns"`
	OtherBestAskBaseUnitNanos string `json:"other_best_ask_base_unit_ns"`
}

type ME016SignalAudit struct {
	Window         SignalWindowBookSummary   `json:"window_book"`
	Snapshots      []SignalSnapshotOwnership `json:"delivered_snapshot_ownership"`
	Makers         []SignalMakerFunnel       `json:"maker_decision_funnels"`
	Decisions      []*SignalDecisionTrace    `json:"decision_request_traces"`
	window         MeasurementWindow
	lastAt         int64
	bestBid        big.Int
	bestAsk        big.Int
	spread         big.Int
	restAt         int64
	restBid        big.Int
	restAsk        big.Int
	restSpr        big.Int
	restBidNs      int64
	restAskNs      int64
	restBoth       int64
	maker          map[uint64]*SignalMakerFunnel
	clientCategory map[uint64]string
	makerBestBid   big.Int
	makerBestAsk   big.Int
	seedBestBid    big.Int
	seedBestAsk    big.Int
	otherBestBid   big.Int
	otherBestAsk   big.Int
	requests       map[requestKey]*SignalRequestTrace
	orders         map[uint64]*SignalRequestTrace
}

type signalOwnedBest struct {
	bid int64
	ask int64
}

func newME016SignalAudit(window MeasurementWindow, startAt int64) *ME016SignalAudit {
	return &ME016SignalAudit{window: window, lastAt: startAt, restAt: startAt,
		maker: make(map[uint64]*SignalMakerFunnel), Snapshots: make([]SignalSnapshotOwnership, 0),
		clientCategory: make(map[uint64]string),
		requests:       make(map[requestKey]*SignalRequestTrace), orders: make(map[uint64]*SignalRequestTrace)}
}

func (book *restingBook) ownedBest(bids, asks []types.PriceLevel) (map[uint64]signalOwnedBest, error) {
	bestBid, bestAsk := firstLevelPrice(bids), firstLevelPrice(asks)
	owned := make(map[uint64]signalOwnedBest)
	for _, order := range book.orders {
		if order.qty <= 0 {
			return nil, errors.New("repeated spot: nonpositive source-time resting order")
		}
		quantity := owned[order.clientID]
		var next int64
		var ok bool
		switch {
		case bestBid > 0 && order.side == "BUY" && order.price == bestBid:
			next, ok = checkedAdd(quantity.bid, order.qty)
			quantity.bid = next
		case bestAsk > 0 && order.side == "SELL" && order.price == bestAsk:
			next, ok = checkedAdd(quantity.ask, order.qty)
			quantity.ask = next
		default:
			continue
		}
		if !ok {
			return nil, errors.New("repeated spot: source-time owned best depth overflows")
		}
		owned[order.clientID] = quantity
	}
	var sumBid, sumAsk int64
	for _, quantity := range owned {
		var bidOK, askOK bool
		sumBid, bidOK = checkedAdd(sumBid, quantity.bid)
		sumAsk, askOK = checkedAdd(sumAsk, quantity.ask)
		if !bidOK || !askOK {
			return nil, errors.New("repeated spot: source-time best depth overflows")
		}
	}
	if sumBid != firstLevelVisibleQty(bids) || sumAsk != firstLevelVisibleQty(asks) {
		return nil, errors.New("repeated spot: source-time owned depth disagrees with public best")
	}
	return owned, nil
}

func (audit *ME016SignalAudit) makerSnapshot(event Event, observation worldspot.MakerObservation, source publicSnapshot) error {
	own := source.ownBest[event.ClientID]
	publicBid, publicAsk := firstLevelVisibleQty(source.bids), firstLevelVisibleQty(source.asks)
	if own.bid < 0 || own.ask < 0 || own.bid > publicBid || own.ask > publicAsk {
		return errors.New("repeated spot: invalid delivered source-time order ownership")
	}
	audit.Snapshots = append(audit.Snapshots, SignalSnapshotOwnership{
		ClientID: event.ClientID, SourceSequence: observation.SourceSequence,
		SourceAt: observation.SourceAt, DeliveredAt: event.Timestamp,
		BestBid: observation.BestBid, BestAsk: observation.BestAsk,
		PublicBidQty: publicBid, PublicAskQty: publicAsk,
		OwnBidQty: own.bid, OwnAskQty: own.ask,
		OwnOnlyBid: publicBid > 0 && own.bid == publicBid,
		OwnOnlyAsk: publicAsk > 0 && own.ask == publicAsk})
	return nil
}

func (audit *ME016SignalAudit) funnel(clientID uint64) *SignalMakerFunnel {
	funnel := audit.maker[clientID]
	if funnel == nil {
		funnel = &SignalMakerFunnel{ClientID: clientID}
		audit.maker[clientID] = funnel
	}
	return funnel
}

func (audit *ME016SignalAudit) makerDecision(state *replayState, account *accountState, decision worldspot.MakerDecision) error {
	if decision.DecisionAt < audit.window.StartAt || decision.DecisionAt >= audit.window.EndAt {
		return nil
	}
	funnel := audit.funnel(account.clientID)
	trace := &SignalDecisionTrace{ClientID: account.clientID, DecisionAt: decision.DecisionAt,
		LocalSourceSequence: decision.LatestBookSequence, LocalSourceAt: decision.LatestBookSourceAt,
		Action: decision.Action, TargetBid: decision.TargetBid, TargetAsk: decision.TargetAsk,
		Requests: make([]*SignalRequestTrace, 0, len(account.pendingSends))}
	if err := audit.attachDecisionRequests(state, account, decision, trace); err != nil {
		return err
	}
	audit.Decisions = append(audit.Decisions, trace)
	funnel.ScheduledDecisions++
	if decision.Action == "await_response" {
		funnel.PendingResponse++
	}
	if decision.Action == "evaluate_placements" {
		funnel.PlacementEvaluations++
	}
	if decision.Action == "keep_quotes" || decision.Action == "cancel_quotes" ||
		decision.Action == "evaluate_placements" || decision.Action == "no_usable_quote" {
		funnel.QuoteEvaluations++
	}
	latest, seen := state.latestMakerSnapshot[account.clientID]
	if !seen {
		return nil
	}
	funnel.ProcessedBookDecisions++
	if latest.BestBid <= 0 || latest.BestAsk <= latest.BestBid ||
		latest.TopBidVisibleQty == nil || latest.TopAskVisibleQty == nil ||
		*latest.TopBidVisibleQty <= 0 || *latest.TopAskVisibleQty <= 0 ||
		decision.DecisionAt < latest.SourceAt || decision.DecisionAt-latest.SourceAt >= account.maker.maxSignalAge {
		return nil
	}
	funnel.FreshTwoSidedDepth++
	trace.FreshTwoSidedDepth = true
	if *latest.TopBidVisibleQty == *latest.TopAskVisibleQty {
		return nil
	}
	funnel.UnequalTopDepth++
	_, mid, _, _ := state.makerReferenceAt(account, decision.DecisionAt)
	if mid <= 0 {
		return nil
	}
	funnel.UsableLocalReference++
	base := *account.maker
	base.signalGainBps = 0
	baseBid, baseAsk, baseOK := expectedMakerQuote(&base, decision, mid)
	if !baseOK {
		return nil
	}
	funnel.BaseQuoteUsable++
	shadow := base
	shadow.signalGainBps = 2
	shadowBid, shadowAsk, shadowOK := expectedMakerQuote(&shadow, decision, mid)
	if !shadowOK || (shadowBid == baseBid && shadowAsk == baseAsk) {
		return nil
	}
	funnel.ShadowGain2Changes++
	trace.ShadowGain2Changes = true
	if decision.Action == "keep_quotes" || decision.Action == "cancel_quotes" || decision.Action == "evaluate_placements" {
		if decision.TargetBid != baseBid || decision.TargetAsk != baseAsk {
			funnel.ActualShiftedTargets++
		}
	}
	return nil
}

func (audit *ME016SignalAudit) accrueBook(at int64, book *publicBookSeries) error {
	if at < audit.lastAt {
		return errors.New("repeated spot: signal window book time regressed")
	}
	from := max(audit.lastAt, audit.window.StartAt)
	until := min(at, audit.window.EndAt)
	audit.lastAt = at
	if until <= from {
		return nil
	}
	duration := until - from
	bid, bidQty := bestVisible(book.bids, true)
	ask, askQty := bestVisible(book.asks, false)
	if bid > 0 {
		audit.Window.BidPresentNanos += duration
	}
	if ask > 0 {
		audit.Window.AskPresentNanos += duration
	}
	if bid > 0 && ask > 0 {
		if bid >= ask {
			return errors.New("repeated spot: crossed signal window book")
		}
		audit.Window.TwoSidedNanos += duration
		audit.spread.Add(&audit.spread, new(big.Int).Mul(big.NewInt(ask-bid), big.NewInt(duration)))
	}
	audit.bestBid.Add(&audit.bestBid, new(big.Int).Mul(big.NewInt(bidQty), big.NewInt(duration)))
	audit.bestAsk.Add(&audit.bestAsk, new(big.Int).Mul(big.NewInt(askQty), big.NewInt(duration)))
	return nil
}

func (audit *ME016SignalAudit) accrueResting(at int64, book *restingBook) error {
	if at < audit.restAt {
		return errors.New("repeated spot: source-order window time regressed")
	}
	from := max(audit.restAt, audit.window.StartAt)
	until := min(at, audit.window.EndAt)
	audit.restAt = at
	if until <= from {
		return nil
	}
	bids, asks := make(map[int64]int64), make(map[int64]int64)
	for _, order := range book.orders {
		if order.qty <= 0 || order.price <= 0 {
			return errors.New("repeated spot: invalid resting order in independent best series")
		}
		var side map[int64]int64
		switch order.side {
		case "BUY":
			side = bids
		case "SELL":
			side = asks
		default:
			return errors.New("repeated spot: invalid resting side in independent best series")
		}
		updated, ok := checkedAdd(side[order.price], order.qty)
		if !ok {
			return errors.New("repeated spot: independent best-level depth overflows")
		}
		side[order.price] = updated
	}
	duration := until - from
	bid, bidQty := bestVisible(bids, true)
	ask, askQty := bestVisible(asks, false)
	if bid > 0 {
		audit.restBidNs += duration
	}
	if ask > 0 {
		audit.restAskNs += duration
	}
	if bid > 0 && ask > 0 {
		if bid >= ask {
			return errors.New("repeated spot: crossed independently reconstructed resting book")
		}
		audit.restBoth += duration
		audit.restSpr.Add(&audit.restSpr, new(big.Int).Mul(big.NewInt(ask-bid), big.NewInt(duration)))
	}
	audit.restBid.Add(&audit.restBid, new(big.Int).Mul(big.NewInt(bidQty), big.NewInt(duration)))
	audit.restAsk.Add(&audit.restAsk, new(big.Int).Mul(big.NewInt(askQty), big.NewInt(duration)))
	for _, order := range book.orders {
		var target *big.Int
		switch audit.clientCategory[order.clientID] {
		case "maker":
			if order.side == "BUY" && order.price == bid {
				target = &audit.makerBestBid
			}
			if order.side == "SELL" && order.price == ask {
				target = &audit.makerBestAsk
			}
		case "seed":
			if order.side == "BUY" && order.price == bid {
				target = &audit.seedBestBid
			}
			if order.side == "SELL" && order.price == ask {
				target = &audit.seedBestAsk
			}
		default:
			if order.side == "BUY" && order.price == bid {
				target = &audit.otherBestBid
			}
			if order.side == "SELL" && order.price == ask {
				target = &audit.otherBestAsk
			}
		}
		if target != nil {
			target.Add(target, new(big.Int).Mul(big.NewInt(order.qty), big.NewInt(duration)))
		}
	}
	return nil
}

func (audit *ME016SignalAudit) finish(terminalAt int64, book *publicBookSeries, restingBook *restingBook, resting RestingDepthSummary) error {
	if err := audit.accrueBook(terminalAt, book); err != nil {
		return err
	}
	if err := audit.accrueResting(terminalAt, restingBook); err != nil {
		return err
	}
	audit.Window.WindowNanos = audit.window.EndAt - audit.window.StartAt
	audit.Window.BestBidBaseUnitNanos = audit.bestBid.String()
	audit.Window.BestAskBaseUnitNanos = audit.bestAsk.String()
	audit.Window.SpreadPriceUnitNanos = audit.spread.String()
	audit.Window.MakerBestBidBaseUnitNanos, audit.Window.MakerBestAskBaseUnitNanos = audit.makerBestBid.String(), audit.makerBestAsk.String()
	audit.Window.SeedBestBidBaseUnitNanos, audit.Window.SeedBestAskBaseUnitNanos = audit.seedBestBid.String(), audit.seedBestAsk.String()
	audit.Window.OtherBestBidBaseUnitNanos, audit.Window.OtherBestAskBaseUnitNanos = audit.otherBestBid.String(), audit.otherBestAsk.String()
	categoryBid := new(big.Int).Add(&audit.makerBestBid, &audit.seedBestBid)
	categoryBid.Add(categoryBid, &audit.otherBestBid)
	categoryAsk := new(big.Int).Add(&audit.makerBestAsk, &audit.seedBestAsk)
	categoryAsk.Add(categoryAsk, &audit.otherBestAsk)
	if audit.Window.TwoSidedNanos != resting.TwoSidedNanos ||
		audit.Window.BidPresentNanos != resting.BidPresentNanos ||
		audit.Window.AskPresentNanos != resting.AskPresentNanos ||
		audit.restBidNs != audit.Window.BidPresentNanos ||
		audit.restAskNs != audit.Window.AskPresentNanos ||
		audit.restBoth != audit.Window.TwoSidedNanos ||
		audit.restBid.Cmp(&audit.bestBid) != 0 || audit.restAsk.Cmp(&audit.bestAsk) != 0 ||
		audit.restSpr.Cmp(&audit.spread) != 0 ||
		categoryBid.Cmp(&audit.bestBid) != 0 || categoryAsk.Cmp(&audit.bestAsk) != 0 {
		return errors.New("repeated spot: public-best and independent resting-order window paths differ")
	}
	var restingBid, restingAsk big.Int
	if _, ok := restingBid.SetString(resting.BidDepthBaseUnitNanos, 10); !ok ||
		audit.bestBid.Cmp(&restingBid) > 0 {
		return errors.New("repeated spot: public best bid depth exceeds resting depth")
	}
	if _, ok := restingAsk.SetString(resting.AskDepthBaseUnitNanos, 10); !ok ||
		audit.bestAsk.Cmp(&restingAsk) > 0 {
		return errors.New("repeated spot: public best ask depth exceeds resting depth")
	}
	return nil
}
