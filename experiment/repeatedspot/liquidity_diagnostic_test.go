package repeatedspot

import (
	"bytes"
	"testing"

	worldspot "exchange_sim/simulations/repeatedspot"
)

func TestDiagnoseE0LiquidityEvidenceTracksFinalEmptyStreakAndMakerGate(t *testing.T) {
	var stream bytes.Buffer
	recorder := NewRecorder(&stream)
	bookDelta := func(at int64, side string, visible int64) {
		price := int64(100)
		if side == "SELL" {
			price = 102
		}
		recorder.Record(at, 0, "exchange", "BookDelta", "ABC/USD", struct {
			Side       string `json:"side"`
			Price      int64  `json:"price"`
			VisibleQty int64  `json:"visible_qty"`
			HiddenQty  int64  `json:"hidden_qty"`
			TotalQty   int64  `json:"total_qty"`
		}{side, price, visible, 0, visible})
	}
	decision := func(at int64, action string, bid, ask int64) {
		recorder.Record(at, 2, "actor", "maker_decision", "ABC/USD", worldspot.MakerDecision{
			ActorID: 2, DecisionAt: at, LatestBookSourceAt: at - 1,
			BestBid: bid, BestAsk: ask, Action: action,
		})
	}
	recorder.Record(0, 0, "control", "world_begin", "", struct{}{})
	bookDelta(1, "BUY", 1)
	bookDelta(2, "SELL", 1)
	decision(3, "evaluate_placements", 100, 102)
	recorder.Record(4, 0, "exchange", "Trade", "ABC/USD", struct{}{})
	bookDelta(5, "SELL", 0)
	decision(6, "cancel_quotes", 100, 0)
	bookDelta(7, "BUY", 0)
	decision(8, "no_usable_quote", 0, 0)
	decision(10, "no_usable_quote", 0, 0)
	recorder.Record(10, 0, "control", "world_run_end", "", struct{}{})
	identity, err := recorder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	window := MeasurementWindow{StartAt: 6, EndAt: 10}
	diagnostic, err := DiagnoseE0LiquidityEvidence(bytes.NewReader(stream.Bytes()), identity, window, 1)
	if err != nil {
		t.Fatal(err)
	}
	if diagnostic.MeasurementEndNanos != 10 ||
		diagnostic.BookDurations != (BookStateDurations{HorizonNanos: 10, TwoSidedNanos: 3, BidOnlyNanos: 3, EmptyNanos: 4}) ||
		diagnostic.MeasurementBookDurations != (BookStateDurations{HorizonNanos: 4, BidOnlyNanos: 1, EmptyNanos: 3}) ||
		diagnostic.FirstPermanentEmptyNanos == nil || *diagnostic.FirstPermanentEmptyNanos != 7 ||
		diagnostic.LastTwoSidedEndNanos == nil || *diagnostic.LastTwoSidedEndNanos != 5 ||
		diagnostic.TradeCount != 1 || diagnostic.LastTradeNanos == nil || *diagnostic.LastTradeNanos != 4 {
		t.Fatalf("unexpected diagnostic timeline: %+v", diagnostic)
	}
	if len(diagnostic.Makers) != 1 {
		t.Fatalf("maker count: %+v", diagnostic.Makers)
	}
	maker := diagnostic.Makers[0]
	if maker.BeforeMeasurement["evaluate_placements"] != 1 ||
		maker.DuringMeasurement["cancel_quotes"] != 1 || maker.DuringMeasurement["no_usable_quote"] != 1 ||
		maker.AfterMeasurement["no_usable_quote"] != 1 ||
		maker.FirstUnusableCancel == nil || maker.FirstUnusableCancel.DecisionAtNanos != 6 ||
		maker.FirstNoUsableQuote == nil || maker.FirstNoUsableQuote.DecisionAtNanos != 8 ||
		maker.LastPlacementEvaluation == nil || maker.LastPlacementEvaluation.DecisionAtNanos != 3 {
		t.Fatalf("unexpected maker actions: %+v", maker)
	}
	changed := identity
	changed.FrameCount++
	if _, err := DiagnoseE0LiquidityEvidence(bytes.NewReader(stream.Bytes()), changed, window, 1); err == nil {
		t.Fatal("incorrect evidence count accepted")
	}
}

func TestDiagnoseShadowReferenceEligibilityInBothPolicyArms(t *testing.T) {
	var stream bytes.Buffer
	recorder := NewRecorder(&stream)
	recorder.Record(0, 0, "control", "world_begin", "", struct{}{})
	for _, delta := range []struct {
		at      int64
		side    string
		price   int64
		visible int64
	}{
		{1, "BUY", 99, 1}, {1, "SELL", 101, 1}, {3, "SELL", 101, 0},
	} {
		recorder.Record(delta.at, 0, "exchange", "BookDelta", "ABC/USD", struct {
			Side       string `json:"side"`
			Price      int64  `json:"price"`
			VisibleQty int64  `json:"visible_qty"`
			HiddenQty  int64  `json:"hidden_qty"`
			TotalQty   int64  `json:"total_qty"`
		}{delta.side, delta.price, delta.visible, 0, delta.visible})
		if delta.side == "SELL" && delta.visible == 1 {
			recorder.Record(2, 2, "actor", "maker_observation", "ABC/USD", worldspot.MakerObservation{
				ActorID: 2, Kind: "snapshot", ProcessedAt: 2, SourceAt: 1,
				SourceSequence: 11, Symbol: "ABC/USD", BestBid: 99, BestAsk: 101})
		}
	}
	recorder.Record(4, 2, "actor", "maker_observation", "ABC/USD", worldspot.MakerObservation{
		ActorID: 2, Kind: "snapshot", ProcessedAt: 4, SourceAt: 3,
		SourceSequence: 12, Symbol: "ABC/USD", BestBid: 99})
	for _, decision := range []worldspot.MakerDecision{
		{ActorID: 2, DecisionAt: 5, BestBid: 99, Action: "evaluate_placements", ReferenceMode: "cached_two_sided"},
		{ActorID: 2, DecisionAt: 6, BestBid: 99, Action: "await_response"},
		{ActorID: 2, DecisionAt: 16, BestBid: 99, Action: "no_usable_quote"},
	} {
		recorder.Record(decision.DecisionAt, 2, "actor", "maker_decision", "ABC/USD", decision)
	}
	recorder.Record(20, 0, "control", "world_run_end", "", struct{}{})
	identity, err := recorder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	result, err := DiagnoseE0LiquidityEvidenceWithReference(bytes.NewReader(stream.Bytes()), identity,
		MeasurementWindow{StartAt: 4, EndAt: 20}, 1, 15)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Makers) != 1 || result.Makers[0].ShadowEligibleDecisions != 2 ||
		result.Makers[0].ShadowWaitingDecisions != 1 || result.Makers[0].ShadowEvaluations != 1 ||
		result.Makers[0].CachedReferenceDecisions != 1 || result.MeasurementBookDurations.BidOnlyNanos != 16 {
		t.Fatalf("shadow denominator confused activity with eligibility or counted expiry: %+v", result)
	}
}
