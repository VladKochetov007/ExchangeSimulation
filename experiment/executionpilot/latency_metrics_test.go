package executionpilot

import (
	"math"
	"testing"

	"exchange_sim/exchange"
)

func TestSelectedSampledOpportunityRequiresObservedBothBoundaries(t *testing.T) {
	publication := func(quantity int64) publicationWire {
		return publicationWire{PublicBids: []exchange.PriceLevel{{Price: 99, VisibleQty: 10}},
			PublicAsks: []exchange.PriceLevel{{Price: 101, VisibleQty: quantity}}}
	}
	makeState := func() *reconstructionState {
		return &reconstructionState{
			result: ReconstructedOutcome{TargetQty: 5, DeliveredSnapshotSeq: 2,
				PublishedSnapshotAt: 100, DeliveredSnapshotAt: 101, ProcessedSnapshotAt: 121,
				DecisionAt: 150, VenueArrivalAt: 151},
			admissionReceiptAt: 152, marketDataLatency: 1, processingDelay: 20,
			requestLatency: 1, responseLatency: 1,
			publications: map[uint64]publishedSnapshot{
				1: {Timestamp: 0, Payload: publication(1)},
				2: {Timestamp: 100, Payload: publication(5)},
				3: {Timestamp: 150, Payload: publication(5)},
				4: {Timestamp: 200, Payload: publication(1)},
			},
		}
	}
	complete := makeState()
	if err := complete.finishActionTiming(); err != nil {
		t.Fatal(err)
	}
	window := complete.result.SelectedOpportunity
	if window == nil || !window.Complete || window.StartAt != 100 || window.EndAt != 200 ||
		window.DurationNanos != 100 || window.ActionDelayOverDuration == nil ||
		math.Abs(*window.ActionDelayOverDuration-0.51) > 1e-12 {
		t.Fatalf("complete sampled episode = %+v", window)
	}
	right := makeState()
	delete(right.publications, 4)
	if err := right.finishActionTiming(); err != nil {
		t.Fatal(err)
	}
	if !right.result.SelectedOpportunity.RightCensored || right.result.SelectedOpportunity.ActionDelayOverDuration != nil {
		t.Fatalf("right-censored sampled episode was treated as complete: %+v", right.result.SelectedOpportunity)
	}
	left := makeState()
	delete(left.publications, 1)
	if err := left.finishActionTiming(); err != nil {
		t.Fatal(err)
	}
	if !left.result.SelectedOpportunity.LeftCensored || left.result.SelectedOpportunity.Complete ||
		left.result.SelectedOpportunity.ActionDelayOverDuration != nil {
		t.Fatalf("left-censored sampled episode was treated as complete: %+v", left.result.SelectedOpportunity)
	}
}
