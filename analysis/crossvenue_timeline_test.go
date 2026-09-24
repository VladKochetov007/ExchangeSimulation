package analysis

import "testing"

func crossVenueTimelineEvaluation(frame uint64, at int64, generation uint64, reason string, northBid, northAsk, southBid, southAsk int64) CrossVenueEvaluationRecord {
	return CrossVenueEvaluationRecord{
		Event: Event{GlobalSequence: frame, SimTS: at},
		Payload: CrossVenueEvaluationPayload{
			Generation: generation, Reason: reason, TriggerVenueID: "north", TriggerSequence: generation,
			TriggerPublishedAt: 10,
			Books: []CrossVenueEvaluationBook{
				{VenueID: "north", Bid: northBid, BidQty: 1, HasBid: true, Ask: northAsk, AskQty: 1, HasAsk: true},
				{VenueID: "south", Bid: southBid, BidQty: 1, HasBid: true, Ask: southAsk, AskQty: 1, HasAsk: true},
			},
		},
	}
}

func crossVenueTimelineSource(evaluation CrossVenueEvaluationRecord, publicationFrame uint64) CrossVenueConsumedSource {
	return CrossVenueConsumedSource{
		Generation: evaluation.Payload.Generation, VenueID: evaluation.Payload.TriggerVenueID,
		SourceSequence: evaluation.Payload.TriggerSequence, PublicationFrame: publicationFrame,
		PublishedAt: evaluation.Payload.TriggerPublishedAt, EvaluatedAt: evaluation.Event.SimTS,
		Reason: evaluation.Payload.Reason,
	}
}

func TestCrossVenueOpportunityTimelineUsesHalfOpenFrameOrder(t *testing.T) {
	episode := CrossVenueEdgeEpisode{StartTS: 10, StartSequence: 20, EndTS: 10, EndSequence: 30, BuyVenue: "north", SellVenue: "south", InitialEdge: 10}
	inside := crossVenueTimelineEvaluation(25, 10, 1, "SUBMIT", 99, 100, 110, 111)
	outside := crossVenueTimelineEvaluation(31, 10, 2, "ATTEMPT_LIMIT", 99, 100, 110, 111)
	timeline, err := joinCrossVenueOpportunityTimeline(
		[]CrossVenueEvaluationRecord{inside, outside},
		[]CrossVenueConsumedSource{crossVenueTimelineSource(inside, 19), crossVenueTimelineSource(outside, 29)},
		[]CrossVenueEdgeEpisode{episode}, [2]string{"north", "south"}, 40, 1, 1, 0,
	)
	if err != nil || timeline.Episodes[0].Evaluations != 1 || timeline.Episodes[0].Submissions != 1 ||
		timeline.Episodes[0].AlignedEvaluations != 1 || timeline.Evaluations[0].Relation != "PUBLIC_ACTIVE_LOCAL_ALIGNED" ||
		timeline.Evaluations[1].Relation != "LOCAL_POSITIVE_PUBLIC_INACTIVE" || timeline.Evaluations[1].PublicEpisodeIndex != -1 ||
		timeline.Evaluations[0].SourceFrameByVenue["north"] != 19 || timeline.Evaluations[1].SourceFrameByVenue["north"] != 29 {
		t.Fatalf("same-time public/evaluation ordering = %#v, %v", timeline, err)
	}
}

func TestCrossVenueOpportunityTimelineRetainsUnseenAndLocalInactiveEpisodes(t *testing.T) {
	episodes := []CrossVenueEdgeEpisode{
		{StartTS: 10, StartSequence: 20, EndTS: 15, EndSequence: 30, BuyVenue: "north", SellVenue: "south", InitialEdge: 10},
		{StartTS: 20, StartSequence: 40, EndTS: 40, Censored: true, BuyVenue: "north", SellVenue: "south", InitialEdge: 10},
	}
	evaluation := crossVenueTimelineEvaluation(25, 12, 1, "NO_POSITIVE_POLICY_EDGE", 99, 100, 100, 111)
	timeline, err := joinCrossVenueOpportunityTimeline(
		[]CrossVenueEvaluationRecord{evaluation}, []CrossVenueConsumedSource{crossVenueTimelineSource(evaluation, 19)},
		episodes, [2]string{"north", "south"}, 40, 1, 1, 0,
	)
	if err != nil || timeline.Episodes[0].Evaluations != 1 || timeline.Episodes[0].AlignedEvaluations != 0 ||
		timeline.Episodes[1].Evaluations != 0 || timeline.Evaluations[0].Relation != "PUBLIC_ACTIVE_LOCAL_INACTIVE" {
		t.Fatalf("public episodes and delayed local state = %#v, %v", timeline, err)
	}
}

func TestCrossVenueOpportunityTimelineRejectsMissingOrContradictorySources(t *testing.T) {
	evaluation := crossVenueTimelineEvaluation(25, 12, 1, "SUBMIT", 99, 100, 110, 111)
	source := crossVenueTimelineSource(evaluation, 19)
	for _, test := range []struct {
		name    string
		sources []CrossVenueConsumedSource
	}{
		{"missing", nil},
		{"future-publication", []CrossVenueConsumedSource{func() CrossVenueConsumedSource { row := source; row.PublicationFrame = 25; return row }()}},
		{"wrong-generation", []CrossVenueConsumedSource{func() CrossVenueConsumedSource { row := source; row.Generation++; return row }()}},
		{"wrong-trigger-venue", []CrossVenueConsumedSource{func() CrossVenueConsumedSource { row := source; row.VenueID = "south"; return row }()}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := joinCrossVenueOpportunityTimeline([]CrossVenueEvaluationRecord{evaluation}, test.sources, nil, [2]string{"north", "south"}, 40, 1, 1, 0); err == nil {
				t.Fatal("unverified opportunity source accepted")
			}
		})
	}
}
