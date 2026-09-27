package repeatedspot

import (
	"encoding/json"
	"math"
	"testing"

	worldspot "exchange_sim/simulations/repeatedspot"
)

func makerPlacement(requestID uint64, side string, quantity int64) sentRequest {
	return sentRequest{OrderReq: &sentPlacement{RequestID: requestID, Side: side, Qty: quantity}}
}

func TestMakerEnvelopeKeepsCancelPendingExposureAndMeasuresCapacity(t *testing.T) {
	window := MeasurementWindow{StartAt: 5, EndAt: 18}
	replay := newMakerEnvelopeReplay(0, window, 2)
	parameters := &makerParameters{quoteQty: 2, minQuoteQty: 1}
	first := makerPlacement(1, "BUY", 2)
	if err := replay.sent(2, first); err != nil {
		t.Fatal(err)
	}
	if err := replay.decision(2, worldspot.MakerDecision{Action: "evaluate_placements", TargetBid: 100,
		WorkingLower: 0, WorkingUpper: 2}, []*sentOrder{{request: first}}, parameters); err != nil {
		t.Fatal(err)
	}
	if err := replay.processed(3, worldspot.MakerProcessedResponse{Kind: "accepted", RequestID: 1, OrderID: 10}); err != nil {
		t.Fatal(err)
	}
	if err := replay.processed(8, worldspot.MakerProcessedResponse{Kind: "fill", OrderID: 10,
		Side: "BUY", Qty: 1}); err != nil {
		t.Fatal(err)
	}
	cancel := sentRequest{CancelReq: &sentCancellation{RequestID: 2, OrderID: 10}}
	if err := replay.sent(9, cancel); err != nil {
		t.Fatal(err)
	}
	if err := replay.decision(9, worldspot.MakerDecision{Action: "cancel_quotes", FilledInventory: 1,
		WorkingLower: 1, WorkingUpper: 2}, []*sentOrder{{request: cancel}}, parameters); err != nil {
		t.Fatal(err)
	}
	if err := replay.processed(12, worldspot.MakerProcessedResponse{Kind: "cancelled", OrderID: 10,
		RemainingQty: 1}); err != nil {
		t.Fatal(err)
	}
	second := makerPlacement(3, "BUY", 1)
	if err := replay.sent(13, second); err != nil {
		t.Fatal(err)
	}
	if err := replay.decision(13, worldspot.MakerDecision{Action: "evaluate_placements", TargetBid: 100,
		FilledInventory: 1, WorkingLower: 1, WorkingUpper: 2}, []*sentOrder{{request: second}}, parameters); err != nil {
		t.Fatal(err)
	}
	if err := replay.processed(13, worldspot.MakerProcessedResponse{Kind: "accepted", RequestID: 3, OrderID: 11}); err != nil {
		t.Fatal(err)
	}
	if err := replay.processed(14, worldspot.MakerProcessedResponse{Kind: "fill", OrderID: 11,
		Side: "BUY", Qty: 1, IsFull: true}); err != nil {
		t.Fatal(err)
	}
	if err := replay.decision(16, worldspot.MakerDecision{Action: "evaluate_placements", TargetBid: 100,
		FilledInventory: 2, WorkingLower: 2, WorkingUpper: 2}, nil, parameters); err != nil {
		t.Fatal(err)
	}
	summary, err := replay.finish(20)
	if err != nil {
		t.Fatal(err)
	}
	if summary.WindowNanos != 13 || summary.UpperAtLimitNanos != 12 || summary.LowerAtLimitNanos != 0 ||
		summary.EitherAtLimitNanos != 12 || summary.PlacementEvaluationDecisions != 2 ||
		summary.BidTargetDecisions != 2 || summary.AskTargetDecisions != 0 ||
		summary.BidCapClippedDecisions != 2 || summary.BidResourceDeniedDecisions != 1 ||
		summary.TerminalLowerBaseUnits != 2 || summary.TerminalUpperBaseUnits != 2 {
		t.Fatalf("incorrect pending-inclusive maker envelope: %+v", summary)
	}
}

func TestMakerEnvelopeRejectsInvalidReservationsAndReports(t *testing.T) {
	window := MeasurementWindow{StartAt: 0, EndAt: 10}
	for _, test := range []struct {
		name string
		fail func(*makerEnvelopeReplay) error
	}{
		{"over-limit pending order", func(replay *makerEnvelopeReplay) error {
			return replay.sent(1, makerPlacement(1, "BUY", 3))
		}},
		{"cancel of absent local order", func(replay *makerEnvelopeReplay) error {
			return replay.sent(1, sentRequest{CancelReq: &sentCancellation{RequestID: 2, OrderID: 10}})
		}},
		{"zero exchange order identity", func(replay *makerEnvelopeReplay) error {
			replay.byRequest[1] = &envelopeOrder{requestID: 1, side: "BUY", remaining: 1}
			return replay.processed(1, worldspot.MakerProcessedResponse{Kind: "accepted", RequestID: 1})
		}},
		{"forged decision bound", func(replay *makerEnvelopeReplay) error {
			return replay.decision(1, worldspot.MakerDecision{Action: "await_response", WorkingUpper: 1}, nil,
				&makerParameters{quoteQty: 2, minQuoteQty: 1})
		}},
		{"under-sized order despite capacity", func(replay *makerEnvelopeReplay) error {
			order := makerPlacement(1, "BUY", 1)
			if err := replay.sent(1, order); err != nil {
				return err
			}
			return replay.decision(1, worldspot.MakerDecision{Action: "evaluate_placements", TargetBid: 100,
				WorkingUpper: 1}, []*sentOrder{{request: order}}, &makerParameters{quoteQty: 2, minQuoteQty: 1})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.fail(newMakerEnvelopeReplay(0, window, 2)); err == nil {
				t.Fatal("invalid maker envelope evidence was accepted")
			}
		})
	}
}

func TestMakerEnvelopeRejectsCapacityArithmeticOverflow(t *testing.T) {
	for _, position := range []int64{-math.MaxInt64, math.MaxInt64} {
		replay := newMakerEnvelopeReplay(0, MeasurementWindow{StartAt: 0, EndAt: 10}, math.MaxInt64)
		replay.filled = position
		if err := replay.decision(1, worldspot.MakerDecision{Action: "evaluate_placements",
			FilledInventory: position, WorkingLower: position, WorkingUpper: position}, nil,
			&makerParameters{quoteQty: 1, minQuoteQty: 1}); err == nil {
			t.Fatalf("capacity overflow at position %d was accepted", position)
		}
	}
}

func TestMakerEnvelopeSellSideRejectionAndCancelRace(t *testing.T) {
	replay := newMakerEnvelopeReplay(0, MeasurementWindow{StartAt: 0, EndAt: 10}, 2)
	parameters := &makerParameters{quoteQty: 2, minQuoteQty: 1}
	first := makerPlacement(1, "SELL", 2)
	if err := replay.sent(1, first); err != nil {
		t.Fatal(err)
	}
	if err := replay.decision(1, worldspot.MakerDecision{Action: "evaluate_placements", TargetAsk: 110,
		WorkingLower: -2}, []*sentOrder{{request: first}}, parameters); err != nil {
		t.Fatal(err)
	}
	if err := replay.processed(2, worldspot.MakerProcessedResponse{Kind: "order_rejected", RequestID: 1}); err != nil {
		t.Fatal(err)
	}
	if lower, upper, err := replay.bounds(); err != nil || lower != 0 || upper != 0 {
		t.Fatalf("rejected placement retained exposure: %d/%d, %v", lower, upper, err)
	}
	second := makerPlacement(2, "SELL", 2)
	if err := replay.sent(3, second); err != nil {
		t.Fatal(err)
	}
	if err := replay.decision(3, worldspot.MakerDecision{Action: "evaluate_placements", TargetAsk: 110,
		WorkingLower: -2}, []*sentOrder{{request: second}}, parameters); err != nil {
		t.Fatal(err)
	}
	if err := replay.processed(4, worldspot.MakerProcessedResponse{Kind: "accepted", RequestID: 2, OrderID: 10}); err != nil {
		t.Fatal(err)
	}
	if err := replay.processed(5, worldspot.MakerProcessedResponse{Kind: "fill", OrderID: 10,
		Side: "SELL", Qty: 1}); err != nil {
		t.Fatal(err)
	}
	cancel := sentRequest{CancelReq: &sentCancellation{RequestID: 3, OrderID: 10}}
	if err := replay.sent(6, cancel); err != nil {
		t.Fatal(err)
	}
	if err := replay.decision(6, worldspot.MakerDecision{Action: "cancel_quotes", FilledInventory: -1,
		WorkingLower: -2, WorkingUpper: -1}, []*sentOrder{{request: cancel}}, parameters); err != nil {
		t.Fatal(err)
	}
	if err := replay.processed(7, worldspot.MakerProcessedResponse{Kind: "cancel_rejected", OrderID: 10}); err != nil {
		t.Fatal(err)
	}
	if lower, upper, err := replay.bounds(); err != nil || lower != -2 || upper != -1 {
		t.Fatalf("cancel rejection released exposure: %d/%d, %v", lower, upper, err)
	}
	if err := replay.processed(8, worldspot.MakerProcessedResponse{Kind: "forced_cancel", OrderID: 10,
		RemainingQty: 1}); err != nil {
		t.Fatal(err)
	}
	summary, err := replay.finish(10)
	if err != nil {
		t.Fatal(err)
	}
	if summary.LowerAtLimitNanos != 6 || summary.UpperAtLimitNanos != 0 ||
		summary.TerminalLowerBaseUnits != -1 || summary.TerminalUpperBaseUnits != -1 ||
		summary.AskTargetDecisions != 2 || summary.BidTargetDecisions != 0 {
		t.Fatalf("sell-side pending exposure or release was mismeasured: %+v", summary)
	}
}

func TestReplayRejectsRehashedMakerEnvelopeCorruption(t *testing.T) {
	contract, events, directory := capturedFixture(t)
	var changed bool
	for index := range events {
		if events[index].Source != "actor" || events[index].Name != "maker_decision" {
			continue
		}
		var decision worldspot.MakerDecision
		if err := json.Unmarshal(events[index].Payload, &decision); err != nil {
			t.Fatal(err)
		}
		if decision.WorkingUpper > decision.FilledInventory {
			replacePayloadField(t, &events[index], "working_upper", decision.WorkingUpper-1)
			changed = true
			break
		}
	}
	if !changed {
		t.Fatal("fixture lacks pending or live maker buy exposure")
	}
	if _, err := replayMutated(t, contract, events, directory); err == nil {
		t.Fatal("rehashed false working envelope was accepted")
	}
}
