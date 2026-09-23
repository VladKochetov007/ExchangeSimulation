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
			Source: event.Source, Name: event.Name, Route: event.Route, Payload: event.Payload,
		})
		if event.ClientID == 13 && event.Source == "actor" && event.Name == "order_cancelled_receipt" {
			for _, fill := range delayedReceipts {
				reorderedRecorder.Record(executionlab.EvidenceObservation{
					Timestamp: cancellationReceiptAt, ClientID: fill.ClientID,
					Source: fill.Source, Name: fill.Name, Route: fill.Route, Payload: fill.Payload,
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
	for _, mutation := range []struct {
		name      string
		duplicate bool
	}{
		{name: "missing cancellation receipt"},
		{name: "duplicate cancellation receipt", duplicate: true},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			var changed bytes.Buffer
			changedRecorder := NewRecorder(&changed)
			changedReceipt := false
			if err := WalkEvidence(bytes.NewReader(output.Bytes()), identity, func(event RecordedEvent) error {
				observation := executionlab.EvidenceObservation{
					Timestamp: event.Timestamp, ClientID: event.ClientID,
					Source: event.Source, Name: event.Name, Route: event.Route, Payload: event.Payload,
				}
				if event.ClientID == 13 && event.Source == "actor" && event.Name == "order_cancelled_receipt" {
					changedReceipt = true
					if !mutation.duplicate {
						return nil
					}
					changedRecorder.Record(observation)
				}
				changedRecorder.Record(observation)
				return nil
			}); err != nil || !changedReceipt {
				t.Fatalf("cancellation rewrite failed: changed=%t err=%v", changedReceipt, err)
			}
			changedIdentity, err := changedRecorder.Finish()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Reconstruct(bytes.NewReader(changed.Bytes()), changedIdentity, plan); err == nil {
				t.Fatal("rehashed missing or duplicate cancellation receipt accepted")
			}
		})
	}
	for _, mutation := range []struct {
		name          string
		source        string
		event         string
		removeRequest bool
	}{
		{name: "wrong venue cancellation request", source: "exchange", event: "OrderCancelled"},
		{name: "wrong forced cancellation receipt request", source: "actor", event: "order_cancelled_receipt"},
		{name: "missing venue cancellation request", source: "exchange", event: "OrderCancelled", removeRequest: true},
		{name: "missing forced cancellation receipt request", source: "actor", event: "order_cancelled_receipt", removeRequest: true},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			var changed bytes.Buffer
			changedRecorder := NewRecorder(&changed)
			changedRequest := false
			if err := WalkEvidence(bytes.NewReader(output.Bytes()), identity, func(event RecordedEvent) error {
				payload := any(event.Payload)
				if event.ClientID == 13 && event.Source == mutation.source && event.Name == mutation.event {
					changedRequest = true
					var fields map[string]any
					if err := json.Unmarshal(event.Payload, &fields); err != nil {
						return err
					}
					if mutation.removeRequest {
						delete(fields, "request_id")
					} else {
						fields["request_id"] = uint64(999_999)
					}
					payload = fields
				}
				changedRecorder.Record(executionlab.EvidenceObservation{
					Timestamp: event.Timestamp, ClientID: event.ClientID,
					Source: event.Source, Name: event.Name, Route: event.Route, Payload: payload,
				})
				return nil
			}); err != nil || !changedRequest {
				t.Fatalf("cancellation request rewrite failed: changed=%t err=%v", changedRequest, err)
			}
			changedIdentity, err := changedRecorder.Finish()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Reconstruct(bytes.NewReader(changed.Bytes()), changedIdentity, plan); err == nil {
				t.Fatal("rehashed wrong cancellation request ID accepted")
			}
		})
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
			Source: event.Source, Name: event.Name, Route: event.Route, Payload: payload,
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
			for tickIndex := int64(1); tickIndex <= 4000; tickIndex++ {
				timestamp := tickIndex * 1_000_000
				if caseName == "observed but inactive" && timestamp == 500_000_000 {
					bids := []exchange.PriceLevel{{Price: 4_999_000_000, VisibleQty: 100_000_000}}
					asks := []exchange.PriceLevel{{Price: 5_001_000_000, VisibleQty: 100_000_000}}
					recorder.Record(executionlab.EvidenceObservation{
						Timestamp: 499_000_000, Source: "exchange", Name: "BookSnapshot", Route: "ABC/USD",
						Payload: map[string]any{"source_sequence": uint64(1), "public_bids": bids, "public_asks": asks},
					})
					recorder.Record(executionlab.EvidenceObservation{
						Timestamp: 500_000_000, ClientID: 13, Source: "actor", Name: "book_snapshot_receipt",
						Payload: actor.BookSnapshotEvent{
							Symbol: "ABC/USD", Timestamp: 499_000_000, SeqNum: 1,
							Snapshot: &exchange.BookSnapshot{Bids: bids, Asks: asks},
						},
					})
				}
				tick := executionlab.DecisionTick{}
				if caseName == "observed but inactive" && timestamp >= 500_000_000 {
					tick.BestBid, tick.BestAsk = 4_999_000_000, 5_001_000_000
				}
				recorder.Record(executionlab.EvidenceObservation{
					Timestamp: timestamp, ClientID: 13, Source: "actor", Name: "decision_tick", Payload: tick,
				})
			}
			recorder.Record(executionlab.EvidenceObservation{
				Timestamp: 4_000_000_000, ClientID: 13, Source: "exchange", Name: "balance_snapshot", Route: "_global",
				Payload: exchange.BalanceSnapshot{ClientID: 13, SpotBalances: []exchange.AssetBalance{
					{Asset: "ABC", Free: 100_000 * 100_000_000},
					{Asset: "USD", Free: 100_000_000 * 100_000},
				}},
			})
			recorder.Record(executionlab.EvidenceObservation{
				Timestamp: 4_000_000_000, Source: "exchange", Name: "terminal_book", Route: "ABC/USD",
				Payload: executionlab.TerminalBook{Symbol: "ABC/USD", Valid: false},
			})
			identity, err := recorder.Finish()
			if err != nil {
				t.Fatal(err)
			}
			outcome, err := Reconstruct(bytes.NewReader(stream.Bytes()), identity, plan)
			if caseName == "observed but inactive" {
				if err == nil {
					t.Fatal("omitted immediate send after eligible tick was accepted")
				}
				return
			}
			if err != nil || outcome.Status != OutcomeNoObservedOpportunity {
				t.Fatalf("complete no-opportunity clock misclassified: status=%s err=%v", outcome.Status, err)
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
		route := ""
		if source == "exchange" {
			route = "ABC/USD"
			if name == "balance_snapshot" {
				route = "_global"
			}
		}
		recorder.Record(executionlab.EvidenceObservation{
			Timestamp: timestamp, ClientID: clientID, Source: source, Name: name, Route: route, Payload: payload,
		})
	}
	bids := []exchange.PriceLevel{{Price: 4_999_000_000, VisibleQty: 100_000_000}}
	asks := []exchange.PriceLevel{{Price: 5_001_000_000, VisibleQty: 100_000_000}}
	for tickIndex := int64(1); tickIndex <= 4000; tickIndex++ {
		timestamp := tickIndex * 1_000_000
		switch timestamp {
		case 500_000_000:
			emit(499_000_000, 0, "exchange", "BookSnapshot", map[string]any{
				"source_sequence": uint64(1), "public_bids": bids, "public_asks": asks,
			})
			emit(timestamp, 13, "actor", "book_snapshot_receipt", actor.BookSnapshotEvent{
				Symbol: "ABC/USD", Timestamp: 499_000_000, SeqNum: 1,
				Snapshot: &exchange.BookSnapshot{Bids: bids, Asks: asks},
			})
		case 999_000_000:
			emit(998_000_000, 0, "exchange", "BookSnapshot", map[string]any{
				"source_sequence": uint64(2), "public_bids": bids, "public_asks": []exchange.PriceLevel(nil),
			})
			emit(timestamp, 13, "actor", "book_snapshot_receipt", actor.BookSnapshotEvent{
				Symbol: "ABC/USD", Timestamp: 998_000_000, SeqNum: 2,
				Snapshot: &exchange.BookSnapshot{Bids: bids, Asks: nil},
			})
		case 1_001_000_000:
			emit(timestamp, 13, "exchange", "OrderAccepted", map[string]any{
				"request_id": uint64(2), "order_id": uint64(412), "client_id": uint64(13),
				"side": "BUY", "type": "MARKET", "qty": cell.TargetQty, "timestamp": timestamp,
				"time_in_force": "GTC", "visibility": "NORMAL",
			})
			emit(timestamp, 13, "exchange", "OrderCancelled", map[string]any{
				"request_id": uint64(2), "order_id": uint64(412),
				"remaining_qty": cell.TargetQty, "reason": "NO_LIQUIDITY",
			})
		case 1_002_000_000:
			emit(timestamp, 13, "actor", "order_accepted_receipt", actor.OrderAcceptedEvent{
				RequestID: 2, OrderID: 412,
			})
			emit(timestamp, 13, "actor", "order_cancelled_receipt", actor.OrderCancelledEvent{
				OrderID: 412, RemainingQty: cell.TargetQty,
			})
		}
		tick := executionlab.DecisionTick{AlreadyDecided: timestamp > 1_000_000_000}
		if timestamp >= 500_000_000 {
			tick.BestBid, tick.BestAsk = bids[0].Price, asks[0].Price
		}
		emit(timestamp, 13, "actor", "decision_tick", tick)
		if timestamp == 1_000_000_000 {
			emit(timestamp, 13, "actor", "order_send", exchange.Request{
				Type: exchange.ReqPlaceOrder,
				OrderReq: &exchange.OrderRequest{
					RequestID: 2, Side: exchange.Buy, Type: exchange.Market,
					Qty: cell.TargetQty, Symbol: "ABC/USD",
				},
			})
		}
	}
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
			Source: event.Source, Name: name, Route: event.Route, Payload: payload,
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
		name           string
		source         string
		event          string
		field          string
		value          any
		remove         bool
		atDecision     bool
		timestampDelta int64
	}{
		{name: "intent quantity", source: "actor", event: "order_send", field: "OrderReq", value: "invalid"},
		{name: "missing order intent", source: "actor", event: "order_send", remove: true},
		{name: "unpublished snapshot sequence", source: "actor", event: "book_snapshot_receipt", field: "SeqNum", value: uint64(999_999)},
		{name: "altered delivered depth", source: "actor", event: "book_snapshot_receipt", field: "Snapshot", value: map[string]any{
			"bids": []exchange.PriceLevel{{Price: 1, VisibleQty: 1}}, "asks": []exchange.PriceLevel{{Price: 2, VisibleQty: 1}},
		}},
		{name: "delayed market data", source: "actor", event: "book_snapshot_receipt", timestampDelta: 1_000_000},
		{name: "delayed venue arrival", source: "exchange", event: "OrderAccepted", timestampDelta: 1_000_000},
		{name: "delayed admission receipt", source: "actor", event: "order_accepted_receipt", timestampDelta: 1_000_000},
		{name: "delayed fill receipt", source: "actor", event: "order_fill_receipt", timestampDelta: 1_000_000},
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
					if mutation.timestampDelta != 0 {
						event.Timestamp += mutation.timestampDelta
					} else {
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
				}
				mutated.Record(executionlab.EvidenceObservation{
					Timestamp: event.Timestamp, ClientID: event.ClientID,
					Source: event.Source, Name: event.Name, Route: event.Route, Payload: event.Payload,
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
