package analysis

import (
	"testing"

	"exchange_sim/exchange"
)

func crossVenueSubmissionFixture(t *testing.T) ([]CrossVenueEvaluationRecord, []SelectedDecisionFrontierVector, []CrossVenuePlacement) {
	t.Helper()
	north := CrossVenueEvaluationFeed{VenueID: "north", ClientID: 7, Frontier: CrossVenueEvaluationFrontier{LinkID: 11, Ordinal: 1, DeliveredAt: 8, Digest: [16]byte{1}}}
	south := CrossVenueEvaluationFeed{VenueID: "south", ClientID: 8, Frontier: CrossVenueEvaluationFrontier{LinkID: 12, Ordinal: 1, DeliveredAt: 9, Digest: [16]byte{2}}}
	evaluation := CrossVenueEvaluationRecord{
		Event: Event{SimTS: 10, GlobalSequence: 1},
		Payload: CrossVenueEvaluationPayload{
			Generation: 3, Reason: "SUBMIT", SelectedBuy: "north", SelectedSell: "south", QuotedEdge: 50,
			Feeds: []CrossVenueEvaluationFeed{north, south},
		},
	}
	components := []SelectedDecisionFrontierComponent{
		{ClientID: 7, LinkID: 11, Ordinal: 1, DeliveredAt: 8, Digest: [16]byte{1}},
		{ClientID: 8, LinkID: 12, Ordinal: 1, DeliveredAt: 9, Digest: [16]byte{2}},
	}
	buy := SelectedDecisionFrontierVector{
		DecisionID: 1, ActorID: 101, ClientID: 7, RequestID: 1, TradingLinkID: 11, Symbol: "ABC/USD",
		Side: exchange.Buy, OrderType: exchange.Market, TimeInForce: exchange.FOK, Qty: 5, DecisionAt: 10,
		ComponentCount: 2, Components: append([]SelectedDecisionFrontierComponent(nil), components...),
	}
	sell := SelectedDecisionFrontierVector{
		DecisionID: 2, ActorID: 102, ClientID: 8, RequestID: 1, TradingLinkID: 12, Symbol: "ABC/USD",
		Side: exchange.Sell, OrderType: exchange.Market, TimeInForce: exchange.FOK, Qty: 5, DecisionAt: 10,
		ComponentCount: 2, Components: append([]SelectedDecisionFrontierComponent(nil), components...),
	}
	placements := []CrossVenuePlacement{
		{Event: Event{VenueID: "north", ClientID: 7, SimTS: 11, GlobalSequence: 2}, Kind: "ACCEPTED", RequestID: 1, OrderID: 21, Side: "BUY", Qty: 5},
		{Event: Event{VenueID: "south", ClientID: 8, SimTS: 12, GlobalSequence: 3}, Kind: "REJECTED", RequestID: 1, Side: "SELL", Qty: 5, Reason: "FOK_NOT_FILLED"},
	}
	return []CrossVenueEvaluationRecord{evaluation}, []SelectedDecisionFrontierVector{buy, sell}, placements
}

func TestCrossVenueSubmissionJoinBindsTwoLinksAndBothOutcomes(t *testing.T) {
	evaluations, vectors, placements := crossVenueSubmissionFixture(t)
	groups, err := BindCrossVenueSubmissionOutcomes(evaluations, vectors, placements, "ABC/USD", 5, 1)
	if err != nil || len(groups) != 1 || groups[0].Generation != 3 || groups[0].Buy.Placement.OrderID != 21 || groups[0].Sell.Placement.Kind != "REJECTED" {
		t.Fatalf("submission/outcome bind = %#v, %v", groups, err)
	}
	groups, err = BindCrossVenueSubmissionOutcomes(nil, nil, nil, "ABC/USD", 5, 1)
	if err != nil || len(groups) != 0 {
		t.Fatalf("zero-attempt join = %#v, %v", groups, err)
	}
}

func TestCrossVenueSubmissionJoinRejectsDroppedOrMutatedEvidence(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*[]CrossVenueEvaluationRecord, *[]SelectedDecisionFrontierVector, *[]CrossVenuePlacement)
	}{
		{"missing-vector", func(_ *[]CrossVenueEvaluationRecord, vectors *[]SelectedDecisionFrontierVector, _ *[]CrossVenuePlacement) {
			*vectors = (*vectors)[:1]
		}},
		{"missing-outcome", func(_ *[]CrossVenueEvaluationRecord, _ *[]SelectedDecisionFrontierVector, placements *[]CrossVenuePlacement) {
			*placements = (*placements)[:1]
		}},
		{"wrong-frontier", func(_ *[]CrossVenueEvaluationRecord, vectors *[]SelectedDecisionFrontierVector, _ *[]CrossVenuePlacement) {
			(*vectors)[0].Components[1].Digest[0]++
		}},
		{"wrong-side", func(_ *[]CrossVenueEvaluationRecord, vectors *[]SelectedDecisionFrontierVector, _ *[]CrossVenuePlacement) {
			(*vectors)[0].Side = exchange.Sell
		}},
		{"wrong-request", func(_ *[]CrossVenueEvaluationRecord, vectors *[]SelectedDecisionFrontierVector, _ *[]CrossVenuePlacement) {
			(*vectors)[1].RequestID++
		}},
		{"future-information", func(_ *[]CrossVenueEvaluationRecord, vectors *[]SelectedDecisionFrontierVector, _ *[]CrossVenuePlacement) {
			(*vectors)[0].Components[0].DeliveredAt = 12
		}},
		{"early-outcome", func(_ *[]CrossVenueEvaluationRecord, _ *[]SelectedDecisionFrontierVector, placements *[]CrossVenuePlacement) {
			(*placements)[0].Event.GlobalSequence = 1
		}},
		{"no-submit-with-order", func(evaluations *[]CrossVenueEvaluationRecord, _ *[]SelectedDecisionFrontierVector, _ *[]CrossVenuePlacement) {
			(*evaluations)[0].Payload.Reason = "NO_POSITIVE_POLICY_EDGE"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			evaluations, vectors, placements := crossVenueSubmissionFixture(t)
			test.mutate(&evaluations, &vectors, &placements)
			if _, err := BindCrossVenueSubmissionOutcomes(evaluations, vectors, placements, "ABC/USD", 5, 1); err == nil {
				t.Fatal("corrupt submission or outcome join accepted")
			}
		})
	}
}
