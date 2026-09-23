package executionpilot

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/simulations/executionlab"
)

func TestEvidenceFixtureHasIndependentCausalBoundaries(t *testing.T) {
	world, err := NewWorld(Cell{MakerCount: 4, RandomTakerCount: 8, TargetQty: 200_000_000, Seed: 42})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	recorder := NewRecorder(&output)
	world.SetEvidenceObserver(recorder.Record)
	reports, err := world.RunMany(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	identity, err := recorder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	if err := WalkEvidence(bytes.NewReader(output.Bytes()), identity, func(event RecordedEvent) error {
		counts[event.Source+"/"+event.Name]++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"actor/book_snapshot_receipt", "actor/decision_tick", "actor/order_send",
		"exchange/OrderAccepted", "exchange/OrderFill", "exchange/balance_change",
		"exchange/terminal_book",
	} {
		if counts[name] == 0 {
			t.Fatalf("missing %s; counts=%v", name, counts)
		}
	}
	plan, err := Lock(Cell{MakerCount: 4, RandomTakerCount: 8, TargetQty: 200_000_000, Seed: 42}, fixtureIdentity())
	if err != nil {
		t.Fatal(err)
	}
	reconstructed, err := Reconstruct(bytes.NewReader(output.Bytes()), identity, plan)
	if err != nil {
		t.Fatal(err)
	}
	if reconstructed.Status != OutcomeFullyFilled || reconstructed.FilledQty != reports[0].FilledQty ||
		reconstructed.TargetShortfall != reports[0].TargetShortfall || reconstructed.DecisionMid != reports[0].DecisionMid ||
		reconstructed.Notional != reports[0].Notional || reconstructed.QuoteFees != reports[0].QuoteFees {
		t.Fatalf("independent reconstruction diverged: got=%+v actor=%+v", reconstructed, reports[0])
	}
}

func TestPartialFillFixtureReconstructsCancellation(t *testing.T) {
	cell := Cell{MakerCount: 2, RandomTakerCount: 10, TargetQty: 500_000_000, Seed: 42}
	world, err := NewWorld(cell)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	recorder := NewRecorder(&output)
	world.SetEvidenceObserver(recorder.Record)
	reports, err := world.RunMany(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	identity, err := recorder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	plan, err := Lock(cell, fixtureIdentity())
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := Reconstruct(bytes.NewReader(output.Bytes()), identity, plan)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != OutcomePartiallyFilled || outcome.FilledQty != reports[0].FilledQty ||
		outcome.CancelledResidual != reports[0].UnfilledQty || outcome.TargetShortfall != reports[0].TargetShortfall {
		t.Fatalf("partial-fill fixture divergence: outcome=%+v report=%+v", outcome, reports[0])
	}
	var delayedReceipts []RecordedEvent
	var cancellationReceiptAt int64
	if err := WalkEvidence(bytes.NewReader(output.Bytes()), identity, func(event RecordedEvent) error {
		if event.ClientID == 13 && event.Source == "actor" {
			if event.Name == "order_fill_receipt" {
				delayedReceipts = append(delayedReceipts, event)
			}
			if event.Name == "order_cancelled_receipt" {
				cancellationReceiptAt = event.Timestamp
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if cancellationReceiptAt == 0 || len(delayedReceipts) == 0 {
		t.Fatal("fixture did not exercise cancelled residual and earlier fills")
	}
	var reordered bytes.Buffer
	reorderedRecorder := NewRecorder(&reordered)
	if err := WalkEvidence(bytes.NewReader(output.Bytes()), identity, func(event RecordedEvent) error {
		if event.ClientID == 13 && event.Source == "actor" && event.Name == "order_fill_receipt" {
			return nil
		}
		reorderedRecorder.Record(executionlab.EvidenceObservation{
			Timestamp: event.Timestamp, ClientID: event.ClientID,
			Source: event.Source, Name: event.Name, Payload: event.Payload,
		})
		if event.ClientID == 13 && event.Source == "actor" && event.Name == "order_cancelled_receipt" {
			for _, fill := range delayedReceipts {
				reorderedRecorder.Record(executionlab.EvidenceObservation{
					Timestamp: cancellationReceiptAt, ClientID: fill.ClientID,
					Source: fill.Source, Name: fill.Name, Payload: fill.Payload,
				})
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	reorderedIdentity, err := reorderedRecorder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	delayedOutcome, err := Reconstruct(bytes.NewReader(reordered.Bytes()), reorderedIdentity, plan)
	if err != nil {
		t.Fatal(err)
	}
	if delayedOutcome.Status != outcome.Status || delayedOutcome.FilledQty != outcome.FilledQty {
		t.Fatalf("queued earlier fills after cancellation changed execution: %+v", delayedOutcome)
	}
	var unmarked bytes.Buffer
	unmarkedRecorder := NewRecorder(&unmarked)
	if err := WalkEvidence(bytes.NewReader(output.Bytes()), identity, func(event RecordedEvent) error {
		payload := any(event.Payload)
		if event.Name == "terminal_book" {
			payload = executionlab.TerminalBook{Symbol: "ABC/USD", Valid: false}
		}
		unmarkedRecorder.Record(executionlab.EvidenceObservation{
			Timestamp: event.Timestamp, ClientID: event.ClientID,
			Source: event.Source, Name: event.Name, Payload: payload,
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	unmarkedIdentity, err := unmarkedRecorder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	unmarkedOutcome, err := Reconstruct(bytes.NewReader(unmarked.Bytes()), unmarkedIdentity, plan)
	if err != nil {
		t.Fatal(err)
	}
	if unmarkedOutcome.Status != OutcomePartiallyFilled || unmarkedOutcome.TargetShortfallDefined {
		t.Fatalf("unavailable terminal mark was hidden or imputed: %+v", unmarkedOutcome)
	}
}

func TestEvidenceObserverDoesNotChangeFixtureOutcome(t *testing.T) {
	cell := Cell{MakerCount: 2, RandomTakerCount: 10, TargetQty: 500_000_000, Seed: 42}
	plain, err := NewWorld(cell)
	if err != nil {
		t.Fatal(err)
	}
	plainReport, err := plain.RunMany(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	instrumented, err := NewWorld(cell)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	recorder := NewRecorder(&output)
	instrumented.SetEvidenceObserver(recorder.Record)
	instrumentedReport, err := instrumented.RunMany(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := recorder.Finish(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plainReport, instrumentedReport) {
		t.Fatalf("evidence observer changed actor outcome: plain=%+v observed=%+v", plainReport, instrumentedReport)
	}
}

func TestNoTradeIsNotEvidenceFailure(t *testing.T) {
	cell := Cell{MakerCount: 4, RandomTakerCount: 8, TargetQty: 200_000_000, Seed: 42}
	plan, err := Lock(cell, fixtureIdentity())
	if err != nil {
		t.Fatal(err)
	}
	for _, caseName := range []string{"no observed opportunity", "observed but inactive"} {
		t.Run(caseName, func(t *testing.T) {
			var stream bytes.Buffer
			recorder := NewRecorder(&stream)
			if caseName == "observed but inactive" {
				recorder.Record(executionlab.EvidenceObservation{
					Timestamp: 500_000_000, ClientID: 13, Source: "actor", Name: "book_snapshot_receipt",
					Payload: actor.BookSnapshotEvent{
						Symbol: "ABC/USD", Timestamp: 499_000_000, SeqNum: 1,
						Snapshot: &exchange.BookSnapshot{
							Bids: []exchange.PriceLevel{{Price: 4_999_000_000, VisibleQty: 100_000_000}},
							Asks: []exchange.PriceLevel{{Price: 5_001_000_000, VisibleQty: 100_000_000}},
						},
					},
				})
			}
			tick := executionlab.DecisionTick{}
			if caseName == "observed but inactive" {
				tick.BestBid, tick.BestAsk = 4_999_000_000, 5_001_000_000
			}
			recorder.Record(executionlab.EvidenceObservation{
				Timestamp: 1_000_000_000, ClientID: 13, Source: "actor", Name: "decision_tick", Payload: tick,
			})
			recorder.Record(executionlab.EvidenceObservation{
				Timestamp: 4_000_000_000, ClientID: 13, Source: "exchange", Name: "balance_snapshot",
				Payload: exchange.BalanceSnapshot{ClientID: 13, SpotBalances: []exchange.AssetBalance{
					{Asset: "ABC", Free: 100_000 * 100_000_000},
					{Asset: "USD", Free: 100_000_000 * 100_000},
				}},
			})
			recorder.Record(executionlab.EvidenceObservation{
				Timestamp: 4_000_000_000, Source: "exchange", Name: "terminal_book",
				Payload: executionlab.TerminalBook{Symbol: "ABC/USD", Valid: false},
			})
			identity, err := recorder.Finish()
			if err != nil {
				t.Fatal(err)
			}
			outcome, err := Reconstruct(bytes.NewReader(stream.Bytes()), identity, plan)
			if err != nil {
				t.Fatal(err)
			}
			expected := OutcomeNoObservedOpportunity
			if caseName == "observed but inactive" {
				expected = OutcomeOpportunityNoAction
			}
			if outcome.Status != expected {
				t.Fatalf("status=%s expected=%s", outcome.Status, expected)
			}
		})
	}
}

func TestAcceptedZeroFillAfterFacingDepthDisappears(t *testing.T) {
	cell := Cell{MakerCount: 4, RandomTakerCount: 8, TargetQty: 200_000_000, Seed: 42}
	plan, err := Lock(cell, fixtureIdentity())
	if err != nil {
		t.Fatal(err)
	}
	var stream bytes.Buffer
	recorder := NewRecorder(&stream)
	emit := func(timestamp int64, clientID uint64, source, name string, payload any) {
		recorder.Record(executionlab.EvidenceObservation{
			Timestamp: timestamp, ClientID: clientID, Source: source, Name: name, Payload: payload,
		})
	}
	emit(500_000_000, 13, "actor", "book_snapshot_receipt", actor.BookSnapshotEvent{
		Symbol: "ABC/USD", Timestamp: 499_000_000, SeqNum: 1,
		Snapshot: &exchange.BookSnapshot{
			Bids: []exchange.PriceLevel{{Price: 4_999_000_000, VisibleQty: 100_000_000}},
			Asks: []exchange.PriceLevel{{Price: 5_001_000_000, VisibleQty: 100_000_000}},
		},
	})
	emit(999_000_000, 13, "actor", "book_snapshot_receipt", actor.BookSnapshotEvent{
		Symbol: "ABC/USD", Timestamp: 998_000_000, SeqNum: 2,
		Snapshot: &exchange.BookSnapshot{
			Bids: []exchange.PriceLevel{{Price: 4_999_000_000, VisibleQty: 100_000_000}},
			Asks: nil,
		},
	})
	emit(1_000_000_000, 13, "actor", "decision_tick", executionlab.DecisionTick{
		BestBid: 4_999_000_000, BestAsk: 5_001_000_000,
	})
	emit(1_000_000_000, 13, "actor", "order_send", exchange.Request{
		Type: exchange.ReqPlaceOrder,
		OrderReq: &exchange.OrderRequest{
			RequestID: 2, Side: exchange.Buy, Type: exchange.Market,
			Qty: 200_000_000, Symbol: "ABC/USD",
		},
	})
	emit(1_001_000_000, 13, "exchange", "OrderAccepted", map[string]any{
		"request_id": uint64(2), "order_id": uint64(412), "client_id": uint64(13),
		"side": "BUY", "type": "MARKET", "qty": int64(200_000_000), "timestamp": int64(1_001_000_000),
		"time_in_force": "GTC", "visibility": "NORMAL",
	})
	emit(1_001_000_000, 13, "exchange", "OrderCancelled", map[string]any{
		"request_id": uint64(2), "order_id": uint64(412),
		"remaining_qty": int64(200_000_000), "reason": "NO_LIQUIDITY",
	})
	emit(1_002_000_000, 13, "actor", "order_accepted_receipt", actor.OrderAcceptedEvent{
		RequestID: 2, OrderID: 412,
	})
	emit(1_002_000_000, 13, "actor", "order_cancelled_receipt", actor.OrderCancelledEvent{
		RequestID: 2, OrderID: 412, RemainingQty: 200_000_000,
	})
	emit(4_000_000_000, 13, "exchange", "balance_snapshot", exchange.BalanceSnapshot{
		ClientID: 13, SpotBalances: []exchange.AssetBalance{
			{Asset: "ABC", Free: 100_000 * 100_000_000},
			{Asset: "USD", Free: 100_000_000 * 100_000},
		},
	})
	emit(4_000_000_000, 0, "exchange", "terminal_book", executionlab.TerminalBook{
		Symbol: "ABC/USD", Valid: true,
		Bid: exchange.TopLevel{Price: 4_999_000_000, TotalQty: 100_000_000},
		Ask: exchange.TopLevel{Price: 5_001_000_000, TotalQty: 100_000_000},
	})
	identity, err := recorder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := Reconstruct(bytes.NewReader(stream.Bytes()), identity, plan)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != OutcomeAcceptedUnfilled || outcome.FilledQty != 0 || outcome.CancelledResidual != cell.TargetQty ||
		!outcome.TargetShortfallDefined || !outcome.RetainedAfterOneSided || outcome.LatestMessageTwoSided {
		t.Fatalf("zero-fill accepted request misclassified: %+v", outcome)
	}
	var rejectedStream bytes.Buffer
	rejectedRecorder := NewRecorder(&rejectedStream)
	if err := WalkEvidence(bytes.NewReader(stream.Bytes()), identity, func(event RecordedEvent) error {
		payload := any(event.Payload)
		name := event.Name
		switch event.Name {
		case "OrderAccepted":
			name = "OrderRejected"
			payload = map[string]any{
				"request_id": uint64(2), "error": exchange.RejectInvalidQty,
				"symbol": "ABC/USD", "side": "BUY", "type": "MARKET", "qty": cell.TargetQty,
				"time_in_force": "GTC",
			}
		case "OrderCancelled", "order_cancelled_receipt":
			return nil
		case "order_accepted_receipt":
			name = "order_rejected_receipt"
			payload = actor.OrderRejectedEvent{RequestID: 2, Reason: exchange.RejectInvalidQty}
		}
		rejectedRecorder.Record(executionlab.EvidenceObservation{
			Timestamp: event.Timestamp, ClientID: event.ClientID,
			Source: event.Source, Name: name, Payload: payload,
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	rejectedIdentity, err := rejectedRecorder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	rejectedOutcome, err := Reconstruct(bytes.NewReader(rejectedStream.Bytes()), rejectedIdentity, plan)
	if err != nil {
		t.Fatal(err)
	}
	if rejectedOutcome.Status != OutcomeRejected || rejectedOutcome.RejectReason != exchange.RejectInvalidQty || rejectedOutcome.FilledQty != 0 {
		t.Fatalf("rejected request misclassified: %+v", rejectedOutcome)
	}
}

func TestEvidenceMutationsFailClosed(t *testing.T) {
	cell := Cell{MakerCount: 4, RandomTakerCount: 8, TargetQty: 200_000_000, Seed: 42}
	world, err := NewWorld(cell)
	if err != nil {
		t.Fatal(err)
	}
	var original bytes.Buffer
	recorder := NewRecorder(&original)
	world.SetEvidenceObserver(recorder.Record)
	if _, err := world.RunMany(context.Background()); err != nil {
		t.Fatal(err)
	}
	identity, err := recorder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	plan, err := Lock(cell, fixtureIdentity())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Reconstruct(bytes.NewReader(original.Bytes()[:original.Len()-1]), identity, plan); err == nil {
		t.Fatal("truncated evidence accepted")
	}
	for _, mutation := range []struct {
		name       string
		source     string
		event      string
		field      string
		value      any
		remove     bool
		atDecision bool
	}{
		{name: "intent quantity", source: "actor", event: "order_send", field: "OrderReq", value: "invalid"},
		{name: "venue fill price", source: "exchange", event: "OrderFill", field: "price", value: int64(1)},
		{name: "unanchored receipt time", source: "actor", event: "order_fill_receipt", field: "timestamp", value: int64(1_999_000_000)},
		{name: "ledger delta", source: "exchange", event: "balance_change", field: "changes", value: "invalid"},
		{name: "terminal mark", source: "exchange", event: "terminal_book", field: "bid", value: "invalid"},
		{name: "missing terminal snapshot", source: "exchange", event: "balance_snapshot", remove: true},
		{name: "missing eligible tick", source: "actor", event: "decision_tick", remove: true, atDecision: true},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			var changed bytes.Buffer
			mutated := NewRecorder(&changed)
			didChange := false
			err := WalkEvidence(bytes.NewReader(original.Bytes()), identity, func(event RecordedEvent) error {
				if !didChange && event.Source == mutation.source && event.Name == mutation.event &&
					(event.ClientID == 13 || event.Name == "terminal_book") &&
					(!mutation.atDecision || event.Timestamp == 1_000_000_000) {
					didChange = true
					if mutation.remove {
						return nil
					}
					var value map[string]any
					if err := json.Unmarshal(event.Payload, &value); err != nil {
						return err
					}
					value[mutation.field] = mutation.value
					encoded, err := json.Marshal(value)
					if err != nil {
						return err
					}
					event.Payload = encoded
				}
				mutated.Record(executionlab.EvidenceObservation{
					Timestamp: event.Timestamp, ClientID: event.ClientID,
					Source: event.Source, Name: event.Name, Payload: event.Payload,
				})
				return nil
			})
			if err != nil || !didChange {
				t.Fatalf("rewrite failed: changed=%t err=%v", didChange, err)
			}
			changedIdentity, err := mutated.Finish()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Reconstruct(bytes.NewReader(changed.Bytes()), changedIdentity, plan); err == nil {
				t.Fatal("semantically malformed but rehashed evidence accepted")
			}
		})
	}
}
