package analysis

import "testing"

func TestCrossVenueFirstAttemptWorldRejectsIncompleteContract(t *testing.T) {
	run := &Run{}
	if _, err := run.ReconstructCrossVenueFirstAttempt(CrossVenueFirstAttemptSpec{}); err == nil {
		t.Fatal("incomplete registered world contract accepted")
	}
}

func TestCrossVenueFirstAttemptWorldRejectsFeedAccountMismatch(t *testing.T) {
	spec := CrossVenueFirstAttemptSpec{
		Venues: [2]string{"north", "south"}, Clients: map[string]uint64{"north": 7, "south": 8},
	}
	evaluations := []CrossVenueEvaluationRecord{{Payload: CrossVenueEvaluationPayload{
		Feeds: []CrossVenueEvaluationFeed{{VenueID: "north", ClientID: 9, Frontier: CrossVenueEvaluationFrontier{LinkID: 1}}},
	}}}
	if _, err := selectCrossVenueFirstAttemptDecisions(spec, evaluations); err == nil {
		t.Fatal("actor feed was attributed to the wrong venue account")
	}
}
