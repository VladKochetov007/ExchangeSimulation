package executionpilot

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"exchange_sim/exchange"
	"exchange_sim/simulations/executionlab"
)

func instructionFixture(t *testing.T, timeInForce exchange.TimeInForce, targetQty, limitPrice int64) ([]byte, EvidenceIdentity, json.RawMessage, executionlab.ExecutionReport) {
	t.Helper()
	config := executionlab.DefaultSimConfig(executionlab.Immediate)
	config.Seed = 42
	config.RecordSnapshotProjectionEvidence = true
	config.Parent.TargetQty = targetQty
	config.Parent.Instruction = &executionlab.ChildInstruction{
		OrderType: exchange.LimitOrder, TimeInForce: timeInForce, LimitPrice: limitPrice,
	}
	world, err := executionlab.NewSim(config)
	if err != nil {
		t.Fatal(err)
	}
	effectiveWorld, err := json.Marshal(world.WorldContract())
	if err != nil {
		t.Fatal(err)
	}
	var stream bytes.Buffer
	recorder := NewInstructionRecorder(&stream)
	world.SetEvidenceObserver(recorder.Record)
	report, err := world.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	identity, err := recorder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	return stream.Bytes(), identity, effectiveWorld, report
}

func TestInstructionEvidenceReconstructsIOCAndFOK(t *testing.T) {
	for _, targetQty := range []int64{50_000_000, 500_000_000} {
		for _, timeInForce := range []exchange.TimeInForce{exchange.IOC, exchange.FOK} {
			name := timeInForce.String()
			if targetQty == 500_000_000 {
				name += "_five_ABC"
			}
			t.Run(name, func(t *testing.T) {
				raw, identity, effectiveWorld, report := instructionFixture(t, timeInForce, targetQty, 5_010_000_000)
				outcome, err := ReconstructInstruction(bytes.NewReader(raw), identity, effectiveWorld, targetQty)
				if err != nil {
					t.Fatal(err)
				}
				if outcome.CapBoundedAskQty == nil || outcome.FilledQty != report.FilledQty ||
					outcome.UnfilledQty != report.UnfilledQty || outcome.QuoteFees != report.QuoteFees ||
					outcome.TerminalABC != outcome.InitialABC+outcome.FilledQty {
					t.Fatalf("independent instruction reconstruction differs from actor report: outcome=%+v report=%+v", outcome, report)
				}
				if timeInForce == exchange.FOK && outcome.FilledQty != 0 && outcome.FilledQty != targetQty {
					t.Fatalf("accepted FOK partially filled: %+v", outcome)
				}
				if timeInForce == exchange.IOC && outcome.CancelledResidual > 0 && outcome.CancelReason != "IOC_EXPIRED" {
					t.Fatalf("partial IOC cancellation reason = %q", outcome.CancelReason)
				}
			})
		}
	}
}

func TestInstructionEvidencePartialIOCAndRejectedFOKFixture(t *testing.T) {
	const fixtureCap = int64(5_002_000_000)
	const targetQty = int64(500_000_000)
	for _, timeInForce := range []exchange.TimeInForce{exchange.IOC, exchange.FOK} {
		t.Run(timeInForce.String(), func(t *testing.T) {
			raw, identity, effectiveWorld, report := instructionFixture(t, timeInForce, targetQty, fixtureCap)
			outcome, err := ReconstructInstruction(bytes.NewReader(raw), identity, effectiveWorld, targetQty)
			if err != nil {
				t.Fatal(err)
			}
			if outcome.FilledQty != report.FilledQty || outcome.UnfilledQty != report.UnfilledQty {
				t.Fatalf("reconstruction differs from actor report: outcome=%+v report=%+v", outcome, report)
			}
			if outcome.CapBoundedAskQty == nil || *outcome.CapBoundedAskQty <= 0 || *outcome.CapBoundedAskQty >= targetQty {
				t.Fatalf("fixture lacks a sampled partial-depth opportunity: %+v", outcome)
			}
			switch timeInForce {
			case exchange.IOC:
				if outcome.Status != OutcomePartiallyFilled || outcome.FilledQty != 100_000_000 ||
					outcome.CancelledResidual != targetQty-outcome.FilledQty || outcome.CancelReason != "IOC_EXPIRED" {
					t.Fatalf("IOC partial/cancel path was not independently reconstructed: %+v", outcome)
				}
			case exchange.FOK:
				if outcome.Status != OutcomeRejected || outcome.FilledQty != 0 ||
					outcome.RejectReason != "FOK_NOT_FILLED" || outcome.CancelledResidual != 0 ||
					outcome.TerminalABC != outcome.InitialABC || outcome.TerminalUSD != outcome.InitialUSD {
					t.Fatalf("FOK rejection was not non-mutating: %+v", outcome)
				}
			}
		})
	}
}

func TestInstructionSelectedAskCapBoundariesAndNoReachableDepth(t *testing.T) {
	const targetQty = int64(50_000_000)
	raw, identity, world, _ := instructionFixture(t, exchange.IOC, targetQty, InstructionLimitPrice)
	baseline, err := ReconstructInstruction(bytes.NewReader(raw), identity, world, targetQty)
	if err != nil {
		t.Fatal(err)
	}
	var selectedAsk int64
	if err := WalkInstructionEvidence(bytes.NewReader(raw), identity, func(event RecordedEvent) error {
		if event.Source != "actor" || event.Name != "book_snapshot_receipt" || event.ClientID != 13 {
			return nil
		}
		var snapshot snapshotWire
		if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
			return err
		}
		if snapshot.SeqNum == baseline.DeliveredSnapshotSeq {
			selectedAsk = snapshot.Snapshot.Asks[0].Price
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if selectedAsk <= 1_000_000 {
		t.Fatalf("selected ask missing or below venue tick: %d", selectedAsk)
	}
	for _, fixture := range []struct {
		name      string
		cap       int64
		wantDepth bool
	}{
		{"at_selected_best_ask", selectedAsk, true},
		{"one_tick_below_selected_best_ask", selectedAsk - 1_000_000, false},
		{"far_below_any_reachable_ask", 1_000_000, false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			for _, timeInForce := range []exchange.TimeInForce{exchange.IOC, exchange.FOK} {
				raw, identity, world, _ := instructionFixture(t, timeInForce, targetQty, fixture.cap)
				outcome, err := ReconstructInstruction(bytes.NewReader(raw), identity, world, targetQty)
				if err != nil {
					t.Fatal(err)
				}
				if outcome.CapBoundedAskQty == nil || (*outcome.CapBoundedAskQty > 0) != fixture.wantDepth {
					t.Fatalf("cap-bounded selected public quantity differs: %+v", outcome)
				}
				if outcome.Status == OutcomeFullyFilled && outcome.FilledQty != targetQty {
					t.Fatalf("contradictory completion: %+v", outcome)
				}
				if fixture.name == "far_below_any_reachable_ask" {
					if outcome.FilledQty != 0 {
						t.Fatalf("far-below-cap instruction unexpectedly filled: %+v", outcome)
					}
					if timeInForce == exchange.IOC && (outcome.Status != OutcomeAcceptedUnfilled ||
						outcome.CancelledResidual != targetQty) {
						t.Fatalf("unfilled IOC did not cancel its residual: %+v", outcome)
					}
					if timeInForce == exchange.FOK && (outcome.Status != OutcomeRejected ||
						outcome.RejectReason != "FOK_NOT_FILLED") {
						t.Fatalf("unfillable FOK did not reject: %+v", outcome)
					}
				}
			}
		})
	}
}

func TestInstructionEvidenceMutationsFailClosed(t *testing.T) {
	const fixtureCap = int64(5_002_000_000)
	const targetQty = int64(500_000_000)
	for _, test := range []struct {
		name        string
		timeInForce exchange.TimeInForce
		source      string
		eventName   string
		field       string
		value       any
		omit        bool
	}{
		{"IOC_wrong_tif", exchange.IOC, "actor", "order_send", "OrderReq.time_in_force", "FOK", false},
		{"IOC_wrong_request_id", exchange.IOC, "actor", "order_send", "OrderReq.request_id", float64(999), false},
		{"IOC_wrong_cap", exchange.IOC, "actor", "order_send", "OrderReq.price", float64(fixtureCap + 1_000_000), false},
		{"IOC_wrong_acceptance_tif", exchange.IOC, "exchange", "OrderAccepted", "time_in_force", "FOK", false},
		{"IOC_wrong_acceptance_order_id", exchange.IOC, "exchange", "OrderAccepted", "order_id", float64(999), false},
		{"IOC_wrong_cancel_reason", exchange.IOC, "exchange", "OrderCancelled", "reason", "NO_LIQUIDITY", false},
		{"IOC_wrong_cancel_qty", exchange.IOC, "exchange", "OrderCancelled", "remaining_qty", float64(1), false},
		{"IOC_wrong_fill_fee", exchange.IOC, "exchange", "OrderFill", "fee_amount", float64(1), false},
		{"IOC_above_cap_fill", exchange.IOC, "exchange", "OrderFill", "price", float64(fixtureCap + 1), false},
		{"IOC_wrong_fill_order_id", exchange.IOC, "exchange", "OrderFill", "order_id", float64(999), false},
		{"IOC_wrong_ledger_delta", exchange.IOC, "exchange", "balance_change", "changes[0].delta", float64(999), false},
		{"IOC_invalid_terminal_mark", exchange.IOC, "exchange", "terminal_book", "valid", false, false},
		{"IOC_missing_cancel_receipt", exchange.IOC, "actor", "order_cancelled_receipt", "", nil, true},
		{"FOK_wrong_rejection_tif", exchange.FOK, "exchange", "OrderRejected", "time_in_force", "IOC", false},
		{"FOK_wrong_rejection_receipt", exchange.FOK, "actor", "order_rejected_receipt", "reason", "BAD", false},
		{"FOK_missing_rejection_receipt", exchange.FOK, "actor", "order_rejected_receipt", "", nil, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw, identity, effectiveWorld, _ := instructionFixture(t, test.timeInForce, targetQty, fixtureCap)
			changedRaw, changedIdentity := rehashInstructionMutation(t, raw, identity, test.source, test.eventName, test.field, test.value, test.omit)
			if _, err := ReconstructInstruction(bytes.NewReader(changedRaw), changedIdentity, effectiveWorld, targetQty); err == nil {
				t.Fatal("validly rehashed instruction evidence mutation was accepted")
			}
		})
	}
}

func rehashInstructionMutation(t *testing.T, raw []byte, identity EvidenceIdentity,
	source, eventName, field string, value any, omit bool) ([]byte, EvidenceIdentity) {
	t.Helper()
	var changed bytes.Buffer
	recorder := NewInstructionRecorder(&changed)
	modified := false
	if err := WalkInstructionEvidence(bytes.NewReader(raw), identity, func(event RecordedEvent) error {
		if !modified && event.Source == source && event.Name == eventName &&
			(event.ClientID == 13 || event.Name == "terminal_book") {
			modified = true
			if omit {
				return nil
			}
			var payload map[string]any
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				return err
			}
			if field == "OrderReq.time_in_force" || field == "OrderReq.price" || field == "OrderReq.request_id" {
				payload["OrderReq"].(map[string]any)[field[len("OrderReq."):]] = value
			} else if field == "changes[0].delta" {
				payload["changes"].([]any)[0].(map[string]any)["delta"] = value
			} else {
				payload[field] = value
			}
			encoded, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			event.Payload = encoded
		}
		recorder.Record(executionlab.EvidenceObservation{Timestamp: event.Timestamp, ClientID: event.ClientID,
			Source: event.Source, Name: event.Name, Route: event.Route, Payload: event.Payload})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !modified {
		t.Fatalf("fixture lacks %s/%s mutation target", source, eventName)
	}
	changedIdentity, err := recorder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	return changed.Bytes(), changedIdentity
}

func TestInstructionEvidenceRejectsDuplicateAndMisorderedEvents(t *testing.T) {
	raw, identity, effectiveWorld, _ := instructionFixture(t, exchange.IOC, 500_000_000, 5_002_000_000)
	var original []RecordedEvent
	if err := WalkInstructionEvidence(bytes.NewReader(raw), identity, func(event RecordedEvent) error {
		original = append(original, event)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"duplicate_send", "fill_before_admission"} {
		t.Run(mutation, func(t *testing.T) {
			events := append([]RecordedEvent(nil), original...)
			sendIndex, acceptanceIndex, fillIndex := -1, -1, -1
			for index, event := range events {
				if event.ClientID != 13 {
					continue
				}
				switch event.Name {
				case "order_send":
					if sendIndex == -1 {
						sendIndex = index
					}
				case "OrderAccepted":
					if acceptanceIndex == -1 {
						acceptanceIndex = index
					}
				case "OrderFill":
					if fillIndex == -1 {
						fillIndex = index
					}
				}
			}
			if sendIndex < 0 || acceptanceIndex < 0 || fillIndex <= acceptanceIndex {
				t.Fatal("fixture lacks ordered send/admission/fill")
			}
			switch mutation {
			case "duplicate_send":
				events = append(events[:sendIndex+1], append([]RecordedEvent{events[sendIndex]}, events[sendIndex+1:]...)...)
			case "fill_before_admission":
				fill := events[fillIndex]
				copy(events[acceptanceIndex+1:fillIndex+1], events[acceptanceIndex:fillIndex])
				events[acceptanceIndex] = fill
			}
			var changed bytes.Buffer
			recorder := NewInstructionRecorder(&changed)
			for _, event := range events {
				recorder.Record(executionlab.EvidenceObservation{Timestamp: event.Timestamp, ClientID: event.ClientID,
					Source: event.Source, Name: event.Name, Route: event.Route, Payload: event.Payload})
			}
			changedIdentity, err := recorder.Finish()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ReconstructInstruction(bytes.NewReader(changed.Bytes()), changedIdentity, effectiveWorld, 500_000_000); err == nil {
				t.Fatal("validly rehashed duplicate or misordered instruction event was accepted")
			}
		})
	}
}

func TestInstructionVenueInvariantsRejectAboveCapAndDelayedExecution(t *testing.T) {
	config := executionlab.DefaultSimConfig(executionlab.Immediate)
	config.Parent.Instruction = &executionlab.ChildInstruction{
		OrderType: exchange.LimitOrder, TimeInForce: exchange.IOC, LimitPrice: InstructionLimitPrice,
	}
	world, err := executionlab.NewSim(config)
	if err != nil {
		t.Fatal(err)
	}
	contractRaw, err := json.Marshal(world.WorldContract())
	if err != nil {
		t.Fatal(err)
	}
	var contract analysisContract
	if err := json.Unmarshal(contractRaw, &contract); err != nil {
		t.Fatal(err)
	}
	const arrival = int64(1_001_000_000)
	newState := func() *reconstructionState {
		state := newReconstructionState(contract, 13, 500_000_000)
		state.instructionEvidence = true
		state.expectedOrderType = "LIMIT"
		state.expectedTimeInForce = "IOC"
		state.expectedLimitPrice = InstructionLimitPrice
		state.wasAccepted = true
		state.acceptedSeen = true
		state.result.OrderID = 7
		state.result.RequestID = 3
		state.result.VenueArrivalAt = arrival
		return state
	}
	for _, test := range []struct {
		name      string
		eventName string
		timestamp int64
		price     int64
	}{
		{"above_cap_trade", "Trade", arrival, InstructionLimitPrice + 1},
		{"delayed_trade", "Trade", arrival + 1, InstructionLimitPrice},
		{"above_cap_fill", "OrderFill", arrival, InstructionLimitPrice + 1},
		{"delayed_fill", "OrderFill", arrival + 1, InstructionLimitPrice},
		{"delayed_cancel", "OrderCancelled", arrival + 1, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			var payload any
			switch test.eventName {
			case "Trade":
				payload = tradeWire{TradeID: 11, TakerOrderID: 7, Qty: 100_000_000, Price: test.price}
			case "OrderFill":
				payload = fillWire{OrderID: 7, Symbol: "ABC/USD", Side: "BUY", TradeID: 11,
					Qty: 100_000_000, Price: test.price, FilledQty: 100_000_000,
					RemainingQty: 400_000_000, FeeAsset: "USD", FeeAmount: test.price * 5 / 10_000}
			case "OrderCancelled":
				payload = map[string]any{"order_id": 7, "request_id": 3,
					"remaining_qty": 500_000_000, "reason": "IOC_EXPIRED"}
			}
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			if err := newState().consumeExchange(RecordedEvent{Timestamp: test.timestamp,
				ClientID: 13, Source: "exchange", Route: "ABC/USD", Name: test.eventName,
				Payload: encoded}); err == nil {
				t.Fatal("venue action violating locked limit instruction was accepted")
			}
		})
	}
}

func TestInstructionUndefinedTerminalMarkDoesNotInvalidateFilledFraction(t *testing.T) {
	const targetQty = int64(500_000_000)
	raw, identity, effectiveWorld, _ := instructionFixture(t, exchange.IOC, targetQty, 5_002_000_000)
	var changed bytes.Buffer
	recorder := NewInstructionRecorder(&changed)
	changedTerminal := false
	if err := WalkInstructionEvidence(bytes.NewReader(raw), identity, func(event RecordedEvent) error {
		payload := any(event.Payload)
		if event.Source == "exchange" && event.Name == "terminal_book" {
			changedTerminal = true
			payload = executionlab.TerminalBook{Symbol: "ABC/USD"}
		}
		recorder.Record(executionlab.EvidenceObservation{Timestamp: event.Timestamp, ClientID: event.ClientID,
			Source: event.Source, Name: event.Name, Route: event.Route, Payload: payload})
		return nil
	}); err != nil || !changedTerminal {
		t.Fatalf("could not construct unavailable-terminal synthetic fixture: changed=%t err=%v", changedTerminal, err)
	}
	changedIdentity, err := recorder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := ReconstructInstruction(bytes.NewReader(changed.Bytes()), changedIdentity, effectiveWorld, targetQty)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.FilledQty != 100_000_000 || outcome.TerminalMarkAvailable || outcome.TargetShortfallDefined {
		t.Fatalf("unavailable terminal mark was imputed or erased valid fills: %+v", outcome)
	}
}

func TestInstructionQueuedFillReceiptAfterCancellationReceipt(t *testing.T) {
	const targetQty = int64(500_000_000)
	raw, identity, effectiveWorld, _ := instructionFixture(t, exchange.IOC, targetQty, 5_002_000_000)
	var queuedFills []RecordedEvent
	if err := WalkInstructionEvidence(bytes.NewReader(raw), identity, func(event RecordedEvent) error {
		if event.ClientID == 13 && event.Source == "actor" && event.Name == "order_fill_receipt" {
			queuedFills = append(queuedFills, event)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(queuedFills) == 0 {
		t.Fatal("partial-IOC fixture lacks a venue-anchored fill receipt")
	}
	var changed bytes.Buffer
	recorder := NewInstructionRecorder(&changed)
	inserted := false
	if err := WalkInstructionEvidence(bytes.NewReader(raw), identity, func(event RecordedEvent) error {
		if event.ClientID == 13 && event.Source == "actor" && event.Name == "order_fill_receipt" {
			return nil
		}
		recorder.Record(executionlab.EvidenceObservation{Timestamp: event.Timestamp, ClientID: event.ClientID,
			Source: event.Source, Name: event.Name, Route: event.Route, Payload: event.Payload})
		if event.ClientID == 13 && event.Source == "actor" && event.Name == "order_cancelled_receipt" {
			inserted = true
			for _, fill := range queuedFills {
				recorder.Record(executionlab.EvidenceObservation{Timestamp: event.Timestamp,
					ClientID: fill.ClientID, Source: fill.Source, Name: fill.Name,
					Route: fill.Route, Payload: fill.Payload})
			}
		}
		return nil
	}); err != nil || !inserted {
		t.Fatalf("could not reorder response queue: inserted=%t err=%v", inserted, err)
	}
	changedIdentity, err := recorder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := ReconstructInstruction(bytes.NewReader(changed.Bytes()), changedIdentity, effectiveWorld, targetQty)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != OutcomePartiallyFilled || outcome.FilledQty != 100_000_000 {
		t.Fatalf("later receipt was mistaken for a later execution: %+v", outcome)
	}
}
