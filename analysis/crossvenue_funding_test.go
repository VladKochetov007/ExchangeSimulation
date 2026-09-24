package analysis

import "testing"

func crossVenueFirstFundingFixture() ([]CrossVenueEvaluationRecord, *CrossVenueOpportunityTimeline, map[string]CrossVenueRouterMovementAccount) {
	evaluation := crossVenueTimelineEvaluation(25, 12, 1, "SUBMIT", 99, 100, 110, 111)
	evaluation.Payload.SelectedBuy, evaluation.Payload.SelectedSell = "north", "south"
	timeline := &CrossVenueOpportunityTimeline{Evaluations: []CrossVenueOpportunityObservation{{
		Generation: 1, EvaluationFrame: 25, Reason: "SUBMIT", LocalStatus: "POSITIVE_EDGE",
		LocalBuyVenue: "north", LocalSellVenue: "south",
	}}}
	accounts := map[string]CrossVenueRouterMovementAccount{
		"north": {VenueID: "north", ClientID: 7, InitialBase: 5, InitialQuote: 101, DepositFrame: 1, FirstSettlementFrame: 30},
		"south": {VenueID: "south", ClientID: 8, InitialBase: 5, InitialQuote: 1, DepositFrame: 2, FirstSettlementFrame: 31},
	}
	return []CrossVenueEvaluationRecord{evaluation}, timeline, accounts
}

func TestCrossVenueFirstAttemptFundingSeparatesQuoteSufficiencyFromArrival(t *testing.T) {
	evaluations, timeline, accounts := crossVenueFirstFundingFixture()
	funding, err := AssessCrossVenueFirstAttemptFunding(evaluations, timeline, accounts, [2]string{"north", "south"}, 1, 1, 100)
	if err != nil || funding == nil || !funding.SufficientAtDecision || funding.BuyQuoteRequired != 101 || funding.BuyQuoteAvailable != 101 || funding.SellBaseAvailable != 5 {
		t.Fatalf("first attempt quoted funding = %#v, %v", funding, err)
	}
	account := accounts["north"]
	account.InitialQuote = 99
	accounts["north"] = account
	funding, err = AssessCrossVenueFirstAttemptFunding(evaluations, timeline, accounts, [2]string{"north", "south"}, 1, 1, 100)
	if err != nil || funding == nil || funding.SufficientAtDecision {
		t.Fatalf("insufficient quoted funding misclassified = %#v, %v", funding, err)
	}
	evaluations[0].Payload.Reason = "NO_POSITIVE_POLICY_EDGE"
	timeline.Evaluations[0].Reason = "NO_POSITIVE_POLICY_EDGE"
	funding, err = AssessCrossVenueFirstAttemptFunding(evaluations, timeline, accounts, [2]string{"north", "south"}, 1, 1, 100)
	if err != nil || funding != nil {
		t.Fatalf("no attempt acquired a funding verdict = %#v, %v", funding, err)
	}
}

func TestCrossVenueFirstAttemptFundingRejectsUnanchoredOrRepeatedAttempts(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*[]CrossVenueEvaluationRecord, **CrossVenueOpportunityTimeline, *map[string]CrossVenueRouterMovementAccount)
	}{
		{"prior-settlement", func(_ *[]CrossVenueEvaluationRecord, _ **CrossVenueOpportunityTimeline, accounts *map[string]CrossVenueRouterMovementAccount) {
			account := (*accounts)["north"]
			account.FirstSettlementFrame = 20
			(*accounts)["north"] = account
		}},
		{"unanchored-deposit", func(_ *[]CrossVenueEvaluationRecord, _ **CrossVenueOpportunityTimeline, accounts *map[string]CrossVenueRouterMovementAccount) {
			account := (*accounts)["south"]
			account.DepositFrame = 0
			(*accounts)["south"] = account
		}},
		{"later-attempt", func(evaluations *[]CrossVenueEvaluationRecord, _ **CrossVenueOpportunityTimeline, _ *map[string]CrossVenueRouterMovementAccount) {
			(*evaluations)[0].Payload.AttemptsUsed = 1
		}},
		{"timeline-mismatch", func(_ *[]CrossVenueEvaluationRecord, timeline **CrossVenueOpportunityTimeline, _ *map[string]CrossVenueRouterMovementAccount) {
			(*timeline).Evaluations[0].EvaluationFrame++
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			evaluations, timeline, accounts := crossVenueFirstFundingFixture()
			test.mutate(&evaluations, &timeline, &accounts)
			if _, err := AssessCrossVenueFirstAttemptFunding(evaluations, timeline, accounts, [2]string{"north", "south"}, 1, 1, 100); err == nil {
				t.Fatal("unanchored first-attempt funding accepted")
			}
		})
	}
}
