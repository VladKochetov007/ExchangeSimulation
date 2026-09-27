package repeatedspot

import (
	"encoding/json"
	"testing"
	"time"

	"exchange_sim/exchange"
	worldspot "exchange_sim/simulations/repeatedspot"
)

func TestSignalShadowDecisionLinksBlockedSentAdmittedRejectedAndFilledPaths(t *testing.T) {
	cell := ME016Cell{Composition: "M1", SignalGainBps: 2, Seed: 18_101}
	world, err := BuildME016World(cell)
	if err != nil {
		t.Fatal(err)
	}
	var contract replayContract
	if err := json.Unmarshal(world.ContractJSON(), &contract); err != nil {
		t.Fatal(err)
	}
	world.Close()
	parameters, err := parseMakerParameters(contract.Participants[2])
	if err != nil {
		t.Fatal(err)
	}
	clientID := contract.Participants[2].ClientID
	for _, outcome := range []string{"blocked", "accepted", "rejected"} {
		t.Run(outcome, func(t *testing.T) {
			window := MeasurementWindow{StartAt: 0, EndAt: int64(20 * time.Second)}
			account := &accountState{clientID: clientID, maker: parameters,
				envelope: newMakerEnvelopeReplay(0, window, parameters.workingLimit)}
			bidQty, askQty := int64(3*e0BasePrecision), int64(e0BasePrecision)
			latest := worldspot.MakerObservation{SourceAt: int64(4 * time.Second),
				BestBid: 49_990 * e0QuotePrecision, BestAsk: 50_010 * e0QuotePrecision,
				TopBidVisibleQty: &bidQty, TopAskVisibleQty: &askQty}
			state := &replayState{contract: contract, accounts: map[uint64]*accountState{clientID: account},
				sentRequests:        make(map[requestKey]*sentOrder),
				latestMakerSnapshot: map[uint64]worldspot.MakerObservation{clientID: latest},
				lastTwoSidedMaker:   map[uint64]worldspot.MakerObservation{clientID: latest},
				resting:             &restingBook{orders: make(map[uint64]*restingOrder)},
				market:              newPublicBookSeries(0, e0QuotePrecision), trades: newTradeAudit(contract.Instrument)}
			decision := worldspot.MakerDecision{DecisionAt: int64(5 * time.Second),
				LatestBookSourceAt: latest.SourceAt, BestBid: latest.BestBid, BestAsk: latest.BestAsk,
				TopBidVisibleQty: &bidQty, TopAskVisibleQty: &askQty,
				LogVariancePerSecond: parameters.initialVariance, Action: "evaluate_placements"}
			mid := latest.BestBid + (latest.BestAsk-latest.BestBid)/2
			decision.TargetBid, decision.TargetAsk, _ = expectedMakerQuote(parameters, decision, mid)
			if outcome == "blocked" {
				decision.Action, decision.TargetBid, decision.TargetAsk = "await_response", 0, 0
			} else {
				request := sentRequest{Type: exchange.ReqPlaceOrder, OrderReq: &sentPlacement{
					RequestID: 10, Side: "BUY", Type: "LIMIT", Price: decision.TargetBid,
					Qty: e0QuoteSmall, Symbol: contract.Instrument.Symbol, TimeInForce: "GTC",
					Visibility: "NORMAL", PostOnly: true}}
				sent := &sentOrder{request: request, at: decision.DecisionAt}
				account.pendingSends = []*sentOrder{sent}
				state.sentRequests[requestKey{clientID: clientID, requestID: 10}] = sent
			}
			audit := newME016SignalAudit(window, 0)
			if err := audit.makerDecision(state, account, decision); err != nil {
				t.Fatal(err)
			}
			trace := audit.Decisions[0]
			if !trace.ShadowGain2Changes || (outcome == "blocked") != (len(trace.Requests) == 0) {
				t.Fatalf("shadow opportunity or request linkage lost: %+v", trace)
			}
			if outcome == "blocked" {
				if err := audit.finishRequests(state.sentRequests); err != nil ||
					len(audit.Makers) != 1 || audit.Makers[0].ShadowPlaceSent != 0 {
					t.Fatalf("blocked shadow opportunity counted as order: %+v, %v", audit.Makers, err)
				}
				return
			}
			if trace.BidRiskFeasible == nil || !*trace.BidRiskFeasible {
				t.Fatalf("sent bid was not feasible under pending-inclusive cap: %+v", trace)
			}
			if outcome == "accepted" {
				accepted := acceptedOrder{RequestID: 10, OrderID: 55, ClientID: clientID,
					Side: "BUY", Price: decision.TargetBid, Qty: e0QuoteSmall,
					Type: "LIMIT", TimeInForce: "GTC", Visibility: "NORMAL",
					PostOnly: true, Status: uint8(exchange.Open), Timestamp: int64(6 * time.Second)}
				payload, _ := json.Marshal(accepted)
				event := Event{ClientID: clientID, Timestamp: accepted.Timestamp, Name: "OrderAccepted", Payload: payload}
				if err := state.checkOrderOutcome(event); err != nil {
					t.Fatal(err)
				}
				state.resting.orders[55] = &restingOrder{clientID: clientID, price: accepted.Price,
					side: "BUY", qty: accepted.Qty}
				if err := audit.exchangeEvent(event, state); err != nil {
					t.Fatal(err)
				}
				state.market.bids[accepted.Price] = accepted.Qty
				if err := audit.exchangeEvent(Event{Name: "BookSnapshot"}, state); err != nil {
					t.Fatal(err)
				}
				fill, _ := json.Marshal(recordedFill{OrderID: 55, Side: "BUY", Qty: e0QuoteSmall / 2})
				if err := audit.exchangeEvent(Event{Name: "OrderFill", Payload: fill}, state); err != nil {
					t.Fatal(err)
				}
				request := trace.Requests[0]
				if request.VenueOutcome != "OrderAccepted" || request.OrderID != 55 ||
					!request.ObservedRestingSource || !request.ObservedAtBestSource ||
					request.FilledBaseUnits != e0QuoteSmall/2 || request.FillCount != 1 {
					t.Fatalf("accepted order lost resting/fill provenance: %+v", request)
				}
			} else {
				rejected := map[string]any{"request_id": uint64(10), "error": "INSUFFICIENT_BALANCE",
					"symbol": contract.Instrument.Symbol, "qty": e0QuoteSmall, "side": "BUY",
					"type": "LIMIT", "time_in_force": "GTC", "post_only": true, "price": decision.TargetBid}
				payload, _ := json.Marshal(rejected)
				event := Event{ClientID: clientID, Timestamp: int64(6 * time.Second), Name: "OrderRejected", Payload: payload}
				if err := state.checkOrderOutcome(event); err != nil {
					t.Fatal(err)
				}
				if err := audit.exchangeEvent(event, state); err != nil {
					t.Fatal(err)
				}
				if trace.Requests[0].VenueOutcome != "OrderRejected" ||
					trace.Requests[0].RejectionReason != "INSUFFICIENT_BALANCE" ||
					trace.Requests[0].ObservedRestingSource || trace.Requests[0].FilledBaseUnits != 0 {
					t.Fatalf("rejected order incorrectly appeared as live/fill: %+v", trace.Requests[0])
				}
			}
			if err := audit.finishRequests(state.sentRequests); err != nil {
				t.Fatal(err)
			}
			funnel := audit.Makers[0]
			if funnel.ShadowPlaceSent != 1 || funnel.ShadowRiskFeasible != 1 ||
				outcome == "accepted" && (funnel.ShadowAccepted != 1 || funnel.ShadowRestingObserved != 1 ||
					funnel.ShadowBestObserved != 1 || funnel.ShadowFilled != 1) ||
				outcome == "rejected" && (funnel.ShadowRejected != 1 || funnel.ShadowAccepted != 0 || funnel.ShadowFilled != 0) {
				t.Fatalf("shadow-to-venue opportunity funnel lost outcome: %+v", funnel)
			}
		})
	}
}
