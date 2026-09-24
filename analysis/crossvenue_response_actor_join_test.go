package analysis

import "testing"

func TestCrossVenueResponseActorsBindToSubmittedLegAndNoActionEvaluation(t *testing.T) {
	venues := map[string]uint64{"north": 7, "south": 8}
	evaluations, decisions, placements := crossVenueSubmissionFixture(t)
	evaluations[0].Payload.TriggerVenueID = "north"
	evaluations[0].Payload.TriggerClientID = 7
	evaluations[0].Payload.TriggerActorID = 101
	groups, err := BindCrossVenueSubmissionOutcomes(evaluations, decisions, placements, "ABC/USD", 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	receipts := []CrossVenueResponseReceiptRecord{
		{Payload: CrossVenueResponseReceiptPayload{VenueID: "north", ClientID: 7, ActorID: 101}},
		{Payload: CrossVenueResponseReceiptPayload{VenueID: "south", ClientID: 8, ActorID: 102}},
	}
	if err := VerifyCrossVenueResponseActors(evaluations, groups, receipts, venues); err != nil {
		t.Fatal(err)
	}
	receipts[0].Payload.ActorID = 999
	if err := VerifyCrossVenueResponseActors(evaluations, groups, receipts, venues); err == nil {
		t.Fatal("different nonzero actor accepted on submitted buy leg")
	}
	receipts[0].Payload.ActorID = 101
	receipts[1].Payload.ActorID = 999
	if err := VerifyCrossVenueResponseActors(evaluations, groups, receipts, venues); err == nil {
		t.Fatal("different nonzero actor accepted on submitted sell leg")
	}
	receipts[1].Payload.ActorID = 102
	if err := VerifyCrossVenueResponseActors(evaluations, nil, receipts[:1], venues); err != nil {
		t.Fatalf("no-attempt evaluated venue rejected: %v", err)
	}
	receipts[0].Payload.ActorID = 999
	if err := VerifyCrossVenueResponseActors(evaluations, nil, receipts[:1], venues); err == nil {
		t.Fatal("different nonzero actor accepted on no-attempt evaluated venue")
	}
}

func TestCrossVenueResponseActorsRejectInconsistentUnobservedVenueReceipts(t *testing.T) {
	clients := map[string]uint64{"north": 7, "south": 8}
	receipts := []CrossVenueResponseReceiptRecord{
		{Payload: CrossVenueResponseReceiptPayload{VenueID: "south", ClientID: 8, ActorID: 102}},
		{Payload: CrossVenueResponseReceiptPayload{VenueID: "south", ClientID: 8, ActorID: 103}},
	}
	if err := VerifyCrossVenueResponseActors(nil, nil, receipts, clients); err == nil {
		t.Fatal("unobserved venue changed response actor")
	}
}
