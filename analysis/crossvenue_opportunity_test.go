package analysis

import (
	"math"
	"testing"
)

func TestCrossVenueOneLotEdgeSeparatesPolicyFromLegSideOpportunity(t *testing.T) {
	venues := [2]string{"north", "south"}
	books := [2]CrossVenueTouch{
		{Ask: 100, AskQty: 2, HasAsk: true},
		{Bid: 105, BidQty: 2, HasBid: true},
	}
	policy := EvaluateCrossVenueOneLotEdge(venues, books, 1, 1, 0, true)
	broad := EvaluateCrossVenueOneLotEdge(venues, books, 1, 1, 0, false)
	if policy.Status != "MISSING_SIDE" || broad.Status != "POSITIVE_EDGE" || broad.Edge != 5 || broad.BuyVenue != "north" || broad.SellVenue != "south" {
		t.Fatalf("policy = %#v, broader diagnostic = %#v", policy, broad)
	}
	books[0].Bid, books[0].BidQty, books[0].HasBid = 99, 1, true
	books[1].Ask, books[1].AskQty, books[1].HasAsk = 106, 1, true
	policy = EvaluateCrossVenueOneLotEdge(venues, books, 1, 1, 100, true)
	if policy.Status != "POSITIVE_EDGE" || policy.Edge != 3 {
		t.Fatalf("costed edge = %#v, want 3", policy)
	}
	books[1].Bid = 102
	if got := EvaluateCrossVenueOneLotEdge(venues, books, 1, 1, 100, true); got.Status != "NONPOSITIVE_EDGE" {
		t.Fatalf("zero after fees = %#v", got)
	}
	books[1].Bid = 99
	if got := EvaluateCrossVenueOneLotEdge(venues, books, 1, 1, 0, true); got.Status != "NONPOSITIVE_EDGE" {
		t.Fatalf("negative edge = %#v", got)
	}
	books[1].BidQty = 0
	if got := EvaluateCrossVenueOneLotEdge(venues, books, 1, 1, 0, true); got.Directions[0].Status != "INSUFFICIENT_DEPTH" {
		t.Fatalf("insufficient depth = %#v", got)
	}
	books[1].BidQty, books[1].Bid = 1, -1
	if got := EvaluateCrossVenueOneLotEdge(venues, books, 1, 1, 0, true); got.Directions[0].Status != "OUT_OF_DOMAIN" {
		t.Fatalf("signed spot touch = %#v", got)
	}
	books[1].Bid = math.MaxInt64
	if got := EvaluateCrossVenueOneLotEdge(venues, books, 2, 1, 0, true); got.Directions[0].Status != "INSUFFICIENT_DEPTH" {
		t.Fatalf("lot gate = %#v", got)
	}
	books[0].AskQty, books[1].BidQty = 2, 2
	if got := EvaluateCrossVenueOneLotEdge(venues, books, 2, 1, 0, true); got.Directions[0].Status != "ARITHMETIC_OVERFLOW" {
		t.Fatalf("overflow = %#v", got)
	}
}

func TestCrossVenueEpisodesRespectSameTimestampGlobalOrderAndHorizon(t *testing.T) {
	venues := [2]string{"north", "south"}
	north := CrossVenueTouch{Bid: 99, BidQty: 1, HasBid: true, Ask: 100, AskQty: 1, HasAsk: true}
	southPositive := CrossVenueTouch{Bid: 105, BidQty: 1, HasBid: true, Ask: 106, AskQty: 1, HasAsk: true}
	southFlat := CrossVenueTouch{Bid: 100, BidQty: 1, HasBid: true, Ask: 101, AskQty: 1, HasAsk: true}
	transitions := []CrossVenuePublicTransition{
		{SimTS: 10, GlobalSequence: 1, VenueID: "north", Touch: north},
		{SimTS: 10, GlobalSequence: 2, VenueID: "south", Touch: southPositive},
		{SimTS: 10, GlobalSequence: 3, VenueID: "south", Touch: southFlat},
		{SimTS: 12, GlobalSequence: 4, VenueID: "south", Touch: southPositive},
	}
	episodes, err := ReconstructCrossVenueEdgeEpisodes(venues, transitions, 20, 1, 1, 0)
	if err != nil || len(episodes) != 2 {
		t.Fatalf("episodes = %#v, %v", episodes, err)
	}
	if episodes[0].StartSequence != 2 || episodes[0].EndSequence != 3 || episodes[0].StartTS != 10 || episodes[0].EndTS != 10 || episodes[0].Censored {
		t.Fatalf("same-time positive episode lost: %#v", episodes[0])
	}
	if episodes[1].StartTS != 12 || episodes[1].EndTS != 20 || !episodes[1].Censored {
		t.Fatalf("open episode was not censored: %#v", episodes[1])
	}
	transitions[2].GlobalSequence = 2
	if _, err := ReconstructCrossVenueEdgeEpisodes(venues, transitions, 20, 1, 1, 0); err == nil {
		t.Fatal("duplicate global sequence accepted")
	}
}

func TestCrossVenueOneLotEdgeIsVenueLabelInvariant(t *testing.T) {
	books := [2]CrossVenueTouch{
		{Bid: 99, BidQty: 1, HasBid: true, Ask: 100, AskQty: 1, HasAsk: true},
		{Bid: 105, BidQty: 1, HasBid: true, Ask: 106, AskQty: 1, HasAsk: true},
	}
	forward := EvaluateCrossVenueOneLotEdge([2]string{"north", "south"}, books, 1, 1, 0, true)
	reversed := EvaluateCrossVenueOneLotEdge([2]string{"south", "north"}, [2]CrossVenueTouch{books[1], books[0]}, 1, 1, 0, true)
	if forward.Status != reversed.Status || forward.Edge != reversed.Edge || forward.BuyVenue != reversed.BuyVenue || forward.SellVenue != reversed.SellVenue {
		t.Fatalf("venue swap changed opportunity: forward %#v, reversed %#v", forward, reversed)
	}
}

func TestCrossVenueEventTimeEpisodeIsNotLostToPeriodicSnapshotAliasing(t *testing.T) {
	venues := [2]string{"north", "south"}
	north := CrossVenueTouch{Bid: 99, BidQty: 1, HasBid: true, Ask: 100, AskQty: 1, HasAsk: true}
	southFlat := CrossVenueTouch{Bid: 100, BidQty: 1, HasBid: true, Ask: 101, AskQty: 1, HasAsk: true}
	southPositive := CrossVenueTouch{Bid: 105, BidQty: 1, HasBid: true, Ask: 106, AskQty: 1, HasAsk: true}
	transitions := []CrossVenuePublicTransition{
		{SimTS: 0, GlobalSequence: 1, VenueID: "north", Touch: north},
		{SimTS: 0, GlobalSequence: 2, VenueID: "south", Touch: southFlat},
		{SimTS: 5, GlobalSequence: 3, VenueID: "south", Touch: southPositive},
		{SimTS: 10, GlobalSequence: 4, VenueID: "south", Touch: southFlat},
	}
	episodes, err := ReconstructCrossVenueEdgeEpisodes(venues, transitions, 20, 1, 1, 0)
	if err != nil || len(episodes) != 1 || episodes[0].StartTS != 5 || episodes[0].EndTS != 10 {
		t.Fatalf("event-time edge was lost: %#v, %v", episodes, err)
	}
	for _, sampled := range [][2]CrossVenueTouch{{north, southFlat}, {north, southFlat}} {
		if edge := EvaluateCrossVenueOneLotEdge(venues, sampled, 1, 1, 0, false); edge.Status == "POSITIVE_EDGE" {
			t.Fatalf("periodic endpoint unexpectedly observed the intervening edge: %#v", edge)
		}
	}
}
