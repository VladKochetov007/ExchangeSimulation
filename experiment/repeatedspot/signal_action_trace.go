package repeatedspot

import (
	"encoding/json"
	"errors"
	"slices"

	worldspot "exchange_sim/simulations/repeatedspot"
)

func (audit *ME016SignalAudit) attachDecisionRequests(state *replayState, account *accountState,
	decision worldspot.MakerDecision, trace *SignalDecisionTrace) error {
	if decision.Action == "evaluate_placements" {
		if account.envelope == nil {
			return errors.New("repeated spot: signal maker has no risk envelope")
		}
		lower, upper, err := account.envelope.bounds()
		if err != nil {
			return err
		}
		if account.envelope.preCaptured {
			lower, upper = account.envelope.preLower, account.envelope.preUpper
		}
		buyCapacity, buyOK := checkedAdd(account.maker.workingLimit, -upper)
		sellCapacity, sellOK := checkedAdd(account.maker.workingLimit, lower)
		if !buyOK || !sellOK {
			return errors.New("repeated spot: signal maker risk capacity overflows")
		}
		bidFeasible := decision.TargetBid > 0 && min(account.maker.quoteQty, buyCapacity) >= account.maker.minQuoteQty
		askFeasible := decision.TargetAsk > 0 && min(account.maker.quoteQty, sellCapacity) >= account.maker.minQuoteQty
		trace.BidRiskFeasible, trace.AskRiskFeasible = &bidFeasible, &askFeasible
	}
	for _, sent := range account.pendingSends {
		request := &SignalRequestTrace{SentAt: sent.at}
		switch {
		case sent.request.OrderReq != nil:
			order := sent.request.OrderReq
			request.RequestID, request.Kind, request.Side = order.RequestID, "place", order.Side
			request.Price, request.Qty = order.Price, order.Qty
		case sent.request.CancelReq != nil:
			request.RequestID, request.Kind = sent.request.CancelReq.RequestID, "cancel"
			request.OrderID = sent.request.CancelReq.OrderID
		default:
			return errors.New("repeated spot: unsupported signal maker request")
		}
		key := requestKey{clientID: account.clientID, requestID: request.RequestID}
		if audit.requests[key] != nil {
			return errors.New("repeated spot: duplicate signal decision request")
		}
		if sent.resolved {
			request.VenueOutcome, request.VenueAt, request.OrderID = sent.outcome, sent.outcomeAt, sent.outcomeOrderID
			request.RejectionReason = sent.outcomeReason
			if request.Kind == "place" && request.VenueOutcome == "OrderAccepted" {
				request.FilledBaseUnits = state.trades.filledByOrder[request.OrderID]
				audit.orders[request.OrderID] = request
			}
		}
		audit.requests[key] = request
		trace.Requests = append(trace.Requests, request)
	}
	return nil
}

func (audit *ME016SignalAudit) exchangeEvent(event Event, state *replayState) error {
	switch event.Name {
	case "OrderAccepted", "OrderRejected", "OrderCancelled", "OrderCancelRejected":
		requestID, actorRequest, err := signalOutcomeRequestID(event)
		if err != nil || !actorRequest {
			return err
		}
		key := requestKey{clientID: event.ClientID, requestID: requestID}
		request := audit.requests[key]
		if request == nil {
			return nil
		}
		sent := state.sentRequests[key]
		if sent == nil || !sent.resolved || sent.outcome != event.Name || request.VenueOutcome != "" {
			return errors.New("repeated spot: signal decision outcome lacks its exact sent request")
		}
		request.VenueOutcome, request.VenueAt, request.OrderID = sent.outcome, sent.outcomeAt, sent.outcomeOrderID
		request.RejectionReason = sent.outcomeReason
		if request.Kind == "place" && event.Name == "OrderAccepted" {
			if audit.orders[request.OrderID] != nil || state.resting.orders[request.OrderID] == nil {
				return errors.New("repeated spot: signal accepted request lacks a live resting order")
			}
			audit.orders[request.OrderID] = request
		}
	case "OrderFill":
		var fill recordedFill
		if err := json.Unmarshal(event.Payload, &fill); err != nil {
			return err
		}
		request := audit.orders[fill.OrderID]
		if request == nil {
			return nil
		}
		updated, ok := checkedAdd(request.FilledBaseUnits, fill.Qty)
		if !ok || updated > request.Qty || fill.Qty <= 0 || request.Side != fill.Side {
			return errors.New("repeated spot: signal decision fill exceeds its accepted request")
		}
		request.FilledBaseUnits = updated
		request.FillCount++
	case "BookSnapshot":
		audit.observeRestingSnapshot(state)
	}
	return nil
}

func signalOutcomeRequestID(event Event) (uint64, bool, error) {
	if event.Name == "OrderCancelled" {
		var cancelled struct {
			RequestID *uint64 `json:"request_id"`
		}
		if err := json.Unmarshal(event.Payload, &cancelled); err != nil {
			return 0, false, err
		}
		if cancelled.RequestID == nil {
			return 0, false, nil
		}
		return *cancelled.RequestID, true, nil
	}
	var outcome struct {
		RequestID uint64 `json:"request_id"`
	}
	if err := json.Unmarshal(event.Payload, &outcome); err != nil {
		return 0, false, err
	}
	return outcome.RequestID, true, nil
}

func (audit *ME016SignalAudit) observeRestingSnapshot(state *replayState) {
	bestBid, _ := bestVisible(state.market.bids, true)
	bestAsk, _ := bestVisible(state.market.asks, false)
	for orderID, request := range audit.orders {
		order := state.resting.orders[orderID]
		if order == nil {
			continue
		}
		request.ObservedRestingSource = true
		if order.side == "BUY" && order.price == bestBid ||
			order.side == "SELL" && order.price == bestAsk {
			request.ObservedAtBestSource = true
		}
	}
}

func (audit *ME016SignalAudit) finishRequests(sentRequests map[requestKey]*sentOrder) error {
	for key, request := range audit.requests {
		sent := sentRequests[key]
		if sent == nil || request.RequestID != key.requestID || request.SentAt != sent.at {
			return errors.New("repeated spot: signal decision request lost its canonical send")
		}
		if sent.resolved {
			if request.VenueOutcome != sent.outcome || request.VenueAt != sent.outcomeAt ||
				request.OrderID != sent.outcomeOrderID {
				return errors.New("repeated spot: signal request outcome disagrees with venue")
			}
		} else if request.VenueOutcome == "" {
			request.VenueOutcome = "UNRESOLVED_AT_END"
		} else {
			return errors.New("repeated spot: signal request has outcome for unresolved venue send")
		}
	}
	for _, trace := range audit.Decisions {
		if !trace.ShadowGain2Changes {
			continue
		}
		funnel := audit.maker[trace.ClientID]
		if funnel == nil {
			return errors.New("repeated spot: shadow trace lost its signal maker")
		}
		if trace.BidRiskFeasible != nil && *trace.BidRiskFeasible ||
			trace.AskRiskFeasible != nil && *trace.AskRiskFeasible {
			funnel.ShadowRiskFeasible++
		}
		var sent, accepted, rejected, resting, best, filled, unresolved bool
		for _, request := range trace.Requests {
			if request.Kind != "place" {
				continue
			}
			sent = true
			accepted = accepted || request.VenueOutcome == "OrderAccepted"
			rejected = rejected || request.VenueOutcome == "OrderRejected"
			resting = resting || request.ObservedRestingSource
			best = best || request.ObservedAtBestSource
			filled = filled || request.FilledBaseUnits > 0
			unresolved = unresolved || request.VenueOutcome == "UNRESOLVED_AT_END"
		}
		if sent {
			funnel.ShadowPlaceSent++
		}
		if accepted {
			funnel.ShadowAccepted++
		}
		if rejected {
			funnel.ShadowRejected++
		}
		if resting {
			funnel.ShadowRestingObserved++
		}
		if best {
			funnel.ShadowBestObserved++
		}
		if filled {
			funnel.ShadowFilled++
		}
		if unresolved {
			funnel.ShadowUnresolved++
		}
	}
	ids := make([]uint64, 0, len(audit.maker))
	for clientID := range audit.maker {
		ids = append(ids, clientID)
	}
	slices.Sort(ids)
	for _, clientID := range ids {
		audit.Makers = append(audit.Makers, *audit.maker[clientID])
	}
	return nil
}
