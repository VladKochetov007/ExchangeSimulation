package analysis

import "testing"

func TestCrossVenuePublicStagesNestedDirectionalEpisodes(t *testing.T) {
	venues := [2]string{"north", "south"}
	transitions := []CrossVenuePublicTransition{
		{SimTS: 10, GlobalSequence: 1, VenueID: "north", Touch: CrossVenueTouch{Bid: 100, Ask: 101, BidQty: 10, AskQty: 10, HasBid: true, HasAsk: true}},
		{SimTS: 20, GlobalSequence: 2, VenueID: "south", Touch: CrossVenueTouch{Bid: 105, Ask: 106, BidQty: 10, AskQty: 10, HasBid: true, HasAsk: true}},
		{SimTS: 30, GlobalSequence: 3, VenueID: "south", Touch: CrossVenueTouch{Bid: 101, Ask: 106, BidQty: 10, AskQty: 10, HasBid: true, HasAsk: true}},
		{SimTS: 40, GlobalSequence: 4, VenueID: "north", Touch: CrossVenueTouch{Ask: 101, AskQty: 10, HasAsk: true}},
	}
	stages, err := DiagnoseCrossVenuePublicStages(venues, transitions, 50, 5, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if stages.EvaluatedTransitions != 3 || stages.TwoSidedLegPrices.DirectionalEpisodes != 2 ||
		stages.TwoSidedLegPrices.PositiveNanos != 40 || stages.GrossCrossing.DirectionalEpisodes != 1 ||
		stages.GrossCrossing.PositiveNanos != 10 || stages.OneLotDepth.PositiveNanos != 10 ||
		stages.FeePositive.PositiveNanos != 10 {
		t.Fatalf("wrong nested stages: %+v", stages)
	}
	feeBlocked, err := DiagnoseCrossVenuePublicStages(venues, transitions, 50, 5, 1, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	if feeBlocked.GrossCrossing.PositiveNanos != 10 || feeBlocked.OneLotDepth.PositiveNanos != 10 ||
		feeBlocked.FeePositive.DirectionalEpisodes != 0 {
		t.Fatalf("fee stage did not distinguish gross edge: %+v", feeBlocked)
	}
	shallow := append([]CrossVenuePublicTransition(nil), transitions...)
	shallow[1].Touch.BidQty = 4
	depthBlocked, err := DiagnoseCrossVenuePublicStages(venues, shallow, 50, 5, 1, 0)
	if err != nil || depthBlocked.GrossCrossing.PositiveNanos != 10 || depthBlocked.OneLotDepth.DirectionalEpisodes != 0 {
		t.Fatalf("depth stage did not distinguish gross edge: %+v %v", depthBlocked, err)
	}
}

func TestCrossVenuePublicStagesFailClosedOnIdentityAndZeroDuration(t *testing.T) {
	venues := [2]string{"north", "south"}
	transitions := []CrossVenuePublicTransition{
		{SimTS: 10, GlobalSequence: 1, VenueID: "north", Touch: CrossVenueTouch{Bid: 100, Ask: 101, BidQty: 10, AskQty: 10, HasBid: true, HasAsk: true}},
		{SimTS: 10, GlobalSequence: 2, VenueID: "south", Touch: CrossVenueTouch{Bid: 105, Ask: 106, BidQty: 10, AskQty: 10, HasBid: true, HasAsk: true}},
		{SimTS: 10, GlobalSequence: 3, VenueID: "south", Touch: CrossVenueTouch{Bid: 101, Ask: 106, BidQty: 10, AskQty: 10, HasBid: true, HasAsk: true}},
	}
	stages, err := DiagnoseCrossVenuePublicStages(venues, transitions, 20, 5, 1, 0)
	if err != nil || stages.GrossCrossing.DirectionalEpisodes != 1 ||
		stages.GrossCrossing.PositiveNanos != 0 || stages.GrossCrossing.ZeroDurationEpisodes != 1 {
		t.Fatalf("same-time episode lost: %+v %v", stages, err)
	}
	for _, mutation := range []func([]CrossVenuePublicTransition){
		func(rows []CrossVenuePublicTransition) { rows[2].GlobalSequence = rows[1].GlobalSequence },
		func(rows []CrossVenuePublicTransition) { rows[2].SimTS = 9 },
		func(rows []CrossVenuePublicTransition) { rows[2].VenueID = "unknown" },
		func(rows []CrossVenuePublicTransition) { rows[2].Touch.BidQty = -1 },
	} {
		changed := append([]CrossVenuePublicTransition(nil), transitions...)
		mutation(changed)
		if _, err := DiagnoseCrossVenuePublicStages(venues, changed, 20, 5, 1, 0); err == nil {
			t.Fatalf("malformed transition accepted: %+v", changed[2])
		}
	}
}
