package executionpilot

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"exchange_sim/simulations/executionlab"
)

func latencyFixture(t *testing.T, network, processing time.Duration) ([]byte, EvidenceIdentity, json.RawMessage, executionlab.ExecutionReport) {
	t.Helper()
	config := executionlab.DefaultSimConfig(executionlab.Immediate)
	config.Seed = 42
	config.ExecutionLatency = 0
	config.RecordSnapshotProjectionEvidence = true
	config.Parent.TargetQty = 500_000_000
	config.ParentDeployment = &executionlab.ParentDeployment{
		MarketDataLatency: network, RequestLatency: network, ResponseLatency: network,
		ProcessingDelay: processing,
	}
	world, err := executionlab.NewSim(config)
	if err != nil {
		t.Fatal(err)
	}
	effective, err := json.Marshal(world.WorldContract())
	if err != nil {
		t.Fatal(err)
	}
	var stream bytes.Buffer
	recorder := NewLatencyRecorder(&stream)
	world.SetEvidenceObserver(recorder.Record)
	report, err := world.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	identity, err := recorder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	return stream.Bytes(), identity, effective, report
}

func TestLatencyProcessingEvidenceFailsClosed(t *testing.T) {
	raw, identity, world, _ := latencyFixture(t, time.Millisecond, 120*time.Millisecond)
	tests := []struct {
		name   string
		mutate func(RecordedEvent) (RecordedEvent, bool)
	}{
		{"omit", func(event RecordedEvent) (RecordedEvent, bool) {
			return event, false
		}},
		{"wrong_sequence", func(event RecordedEvent) (RecordedEvent, bool) {
			if event.Name == "snapshot_processing_complete" {
				var payload map[string]any
				if err := json.Unmarshal(event.Payload, &payload); err != nil {
					t.Fatal(err)
				}
				payload["seq_num"] = float64(999999)
				event.Payload, _ = json.Marshal(payload)
			}
			return event, true
		}},
		{"wrong_receipt_time", func(event RecordedEvent) (RecordedEvent, bool) {
			if event.Name == "snapshot_processing_complete" {
				var payload map[string]any
				if err := json.Unmarshal(event.Payload, &payload); err != nil {
					t.Fatal(err)
				}
				payload["received_at"] = payload["received_at"].(float64) + 1
				event.Payload, _ = json.Marshal(payload)
			}
			return event, true
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var changed bytes.Buffer
			recorder := NewLatencyRecorder(&changed)
			modified := false
			err := WalkLatencyEvidence(bytes.NewReader(raw), identity, func(event RecordedEvent) error {
				if !modified && event.Name == "snapshot_processing_complete" {
					event, keep := test.mutate(event)
					modified = true
					if !keep {
						return nil
					}
					recorder.Record(executionlab.EvidenceObservation{Timestamp: event.Timestamp, ClientID: event.ClientID, Source: event.Source, Name: event.Name, Route: event.Route, Payload: event.Payload})
					return nil
				}
				recorder.Record(executionlab.EvidenceObservation{Timestamp: event.Timestamp, ClientID: event.ClientID, Source: event.Source, Name: event.Name, Route: event.Route, Payload: event.Payload})
				return nil
			})
			if err != nil || !modified {
				t.Fatalf("mutation setup: modified=%v err=%v", modified, err)
			}
			changedIdentity, err := recorder.Finish()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ReconstructLatency(bytes.NewReader(changed.Bytes()), changedIdentity, world, 500_000_000); err == nil {
				t.Fatal("validly rehashed processing-evidence mutation was accepted")
			}
		})
	}
	if _, err := ReconstructLatency(bytes.NewReader(raw), EvidenceIdentity{SchemaID: EvidenceSchemaID, ExecutionHash: identity.ExecutionHash, FrameCount: identity.FrameCount}, world, 500_000_000); err == nil {
		t.Fatal("v3 schema ID was accepted for v4 evidence")
	}
	var contract map[string]any
	if err := json.Unmarshal(world, &contract); err != nil {
		t.Fatal(err)
	}
	parents := contract["parents"].([]any)
	deployment := parents[0].(map[string]any)["deployment"].(map[string]any)
	deployment["processing_delay_nanos"] = float64(119_000_000)
	changedWorld, err := json.Marshal(contract)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReconstructLatency(bytes.NewReader(raw), identity, changedWorld, 500_000_000); err == nil {
		t.Fatal("changed effective deployment was accepted")
	}
}

func TestLatencyZeroProcessingCompletionIsRequired(t *testing.T) {
	raw, identity, world, _ := latencyFixture(t, time.Millisecond, 0)
	var changed bytes.Buffer
	recorder := NewLatencyRecorder(&changed)
	removed := false
	if err := WalkLatencyEvidence(bytes.NewReader(raw), identity, func(event RecordedEvent) error {
		if event.Name == "snapshot_processing_complete" && !removed {
			removed = true
			return nil
		}
		recorder.Record(executionlab.EvidenceObservation{Timestamp: event.Timestamp, ClientID: event.ClientID,
			Source: event.Source, Name: event.Name, Route: event.Route, Payload: event.Payload})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("fixture has no processing completion")
	}
	changedIdentity, err := recorder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReconstructLatency(bytes.NewReader(changed.Bytes()), changedIdentity, world, 500_000_000); err == nil {
		t.Fatal("zero-delay arm accepted missing processing completion")
	}
}

func TestLatencyEvidenceReconstructsFourDirectedDeployments(t *testing.T) {
	for _, test := range []struct {
		name                string
		network, processing time.Duration
	}{
		{"fast-fast", time.Millisecond, 0},
		{"fast-slow", time.Millisecond, 120 * time.Millisecond},
		{"slow-fast", 90 * time.Millisecond, 0},
		{"slow-slow", 90 * time.Millisecond, 120 * time.Millisecond},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw, identity, world, report := latencyFixture(t, test.network, test.processing)
			outcome, err := ReconstructLatency(bytes.NewReader(raw), identity, world, 500_000_000)
			if err != nil {
				t.Fatal(err)
			}
			if outcome.FilledQty != report.FilledQty || outcome.Notional != report.Notional || outcome.QuoteFees != report.QuoteFees ||
				outcome.DecisionAt != report.DecisionAt || outcome.DecisionMid != report.DecisionMid || outcome.TargetShortfall != report.TargetShortfall {
				t.Fatalf("independent reconstruction differs from actor report: %+v versus %+v", outcome, report)
			}
			if test.processing > 0 && outcome.ProcessedSnapshotAt < outcome.DeliveredSnapshotAt+int64(test.processing) {
				t.Fatalf("processed snapshot earlier than configured actor delay: %+v", outcome)
			}
			funnel := outcome.LatencyFunnel
			if funnel == nil || funnel.Published == 0 || funnel.Published != funnel.Enqueued+funnel.NotEnqueued ||
				funnel.Processed > funnel.Received || funnel.Received > funnel.Enqueued ||
				funnel.ReceivedUnprocessed != funnel.Received-funnel.Processed ||
				funnel.QualifyingProcessed > funnel.QualifyingReceived || funnel.QualifyingReceived > funnel.QualifyingEnqueued ||
				funnel.QualifyingEnqueued > funnel.QualifyingPublished {
				t.Fatalf("invalid reconstructed opportunity funnel: %+v", funnel)
			}
		})
	}
}
