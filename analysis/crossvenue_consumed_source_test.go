package analysis

import (
	"strings"
	"testing"

	etypes "exchange_sim/types"
)

func TestCrossVenueConsumedSourceReconstructsLocalNotGlobalBooks(t *testing.T) {
	northSnapshot := crossVenueSnapshotEvidence{
		Bids: []etypes.PriceLevel{{Price: 99, VisibleQty: 1}}, Asks: []etypes.PriceLevel{{Price: 101, VisibleQty: 1}},
		PublicBids: []etypes.PriceLevel{{Price: 99, VisibleQty: 1}}, PublicAsks: []etypes.PriceLevel{{Price: 101, VisibleQty: 1}}, SourceSequence: 1,
	}
	southSnapshot := crossVenueSnapshotEvidence{
		Bids: []etypes.PriceLevel{{Price: 105, VisibleQty: 1}}, Asks: []etypes.PriceLevel{{Price: 106, VisibleQty: 1}},
		PublicBids: []etypes.PriceLevel{{Price: 105, VisibleQty: 1}}, PublicAsks: []etypes.PriceLevel{{Price: 106, VisibleQty: 1}}, SourceSequence: 1,
	}
	sources := []Event{
		crossVenueReplayEvent(t, "BookSnapshot", "north", 1, 10, northSnapshot),
		crossVenueReplayEvent(t, "BookSnapshot", "south", 2, 10, southSnapshot),
	}
	northMessage, err := crossVenueSourceMessage(sources[0], "ABC/USD", 1)
	if err != nil {
		t.Fatal(err)
	}
	northDigest, err := etypes.MarketDataFingerprint(northMessage)
	if err != nil {
		t.Fatal(err)
	}
	southMessage, err := crossVenueSourceMessage(sources[1], "ABC/USD", 1)
	if err != nil {
		t.Fatal(err)
	}
	southDigest, err := etypes.MarketDataFingerprint(southMessage)
	if err != nil {
		t.Fatal(err)
	}
	first := validCrossVenueEvaluationRecord(t)
	first.Payload.TriggerDigest = northDigest
	second := first
	second.Event = crossVenueReplayEvent(t, "cross_venue_arb_evaluation", "south", 4, 21, nil)
	second.Event.ClientID = 12
	second.Payload.Books = append([]CrossVenueEvaluationBook(nil), first.Payload.Books...)
	second.Payload.Feeds = append([]CrossVenueEvaluationFeed(nil), first.Payload.Feeds...)
	second.Payload.TriggerVenueID, second.Payload.TriggerClientID, second.Payload.TriggerActorID = "south", 12, 6
	second.Payload.TriggerDigest, second.Payload.Generation = southDigest, 2
	second.Payload.Reason, second.Payload.SelectedBuy, second.Payload.SelectedSell, second.Payload.QuotedEdge = "SUBMIT", "north", "south", 4
	second.Payload.Books[1] = CrossVenueEvaluationBook{VenueID: "south", ClientID: 12, Bid: 105, BidQty: 1, HasBid: true, Ask: 106, AskQty: 1, HasAsk: true}
	second.Payload.Feeds[1].Frontier = CrossVenueEvaluationFrontier{LinkID: 2, Ordinal: 1, DeliveredAt: 20, Digest: [16]byte{6}, Fingerprint: southDigest}

	matched, err := MatchCrossVenueEvaluationSources([]CrossVenueEvaluationRecord{first, second}, sources, "ABC/USD", 1, 1, 0)
	if err != nil || len(matched) != 2 || matched[0].PublicationFrame != 1 || matched[1].PublicationFrame != 2 {
		t.Fatalf("matched sources = %#v, %v", matched, err)
	}
	wrongLocalBook := second
	wrongLocalBook.Payload.Books = append([]CrossVenueEvaluationBook(nil), second.Payload.Books...)
	wrongLocalBook.Payload.Books[1].Bid = 106
	if _, err := MatchCrossVenueEvaluationSources([]CrossVenueEvaluationRecord{first, wrongLocalBook}, sources, "ABC/USD", 1, 1, 0); err == nil || !strings.Contains(err.Error(), "actor-local book") {
		t.Fatalf("false local book error = %v", err)
	}
	wrongSource := second
	wrongSource.Payload.TriggerDigest = [16]byte{9}
	if _, err := MatchCrossVenueEvaluationSources([]CrossVenueEvaluationRecord{first, wrongSource}, sources, "ABC/USD", 1, 1, 0); err == nil || !strings.Contains(err.Error(), "no earlier matching publication") {
		t.Fatalf("unanchored source error = %v", err)
	}
	if _, err := MatchCrossVenueEvaluationSources([]CrossVenueEvaluationRecord{first, second}, sources[:1], "ABC/USD", 1, 1, 0); err == nil {
		t.Fatal("missing southern publication was accepted")
	}
}

func TestCrossVenueConsumedSourceRejectsIndistinguishableSameTimeDeltas(t *testing.T) {
	delta := crossVenueDeltaEvidence{Side: "BUY", Price: 99, VisibleQty: 1, TotalQty: 1}
	first := crossVenueReplayEvent(t, "BookDelta", "north", 1, 10, delta)
	duplicate := crossVenueReplayEvent(t, "BookDelta", "north", 2, 10, delta)
	message, err := crossVenueSourceMessage(first, "ABC/USD", 2)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := etypes.MarketDataFingerprint(message)
	if err != nil {
		t.Fatal(err)
	}
	evaluation := validCrossVenueEvaluationRecord(t)
	evaluation.Payload.TriggerType = etypes.MDDelta
	evaluation.Payload.TriggerSequence = 2
	evaluation.Payload.TriggerDigest = digest
	if _, err := MatchCrossVenueEvaluationSources([]CrossVenueEvaluationRecord{evaluation}, []Event{first, duplicate}, "ABC/USD", 1, 1, 0); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("indistinguishable delta publications accepted: %v", err)
	}
}
