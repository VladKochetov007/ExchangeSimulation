package executionpilot

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"exchange_sim/simulations/executionlab"
)

func cadenceFixture(t *testing.T, cell CadenceCell) ([]byte, EvidenceIdentity, json.RawMessage, executionlab.ExecutionReport) {
	t.Helper()
	world, err := NewCadenceWorld(cell)
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

func TestCadencePlanIsSeparateFromLatencyMatrix(t *testing.T) {
	identity := fixtureIdentity()
	identity.EvidenceSchemaID = LatencyEvidenceSchemaID
	for _, network := range []int64{1_000_000, 90_000_000} {
		for _, poll := range []int64{1_000_000, 80_000_000} {
			cell := CadenceCell{NetworkLatencyNanos: network, PollIntervalNanos: poll, TargetQty: 500_000_000, Seed: 13001}
			plan, err := LockCadence(cell, identity)
			if err != nil {
				t.Fatal(err)
			}
			world, err := VerifyCadence(plan, identity)
			if err != nil {
				t.Fatal(err)
			}
			contract := world.WorldContract()
			if int64(contract.Parents[0].Config.PollInterval) != poll ||
				int64(contract.Parents[0].Deployment.ProcessingDelay) != 0 ||
				int64(contract.Parents[0].Deployment.RequestLatency) != network {
				t.Fatalf("wrong effective contract: %+v", contract.Parents[0])
			}
			plan.Cell.Seed++
			if _, err := VerifyCadence(plan, identity); err == nil {
				t.Fatal("changed seed accepted")
			}
		}
	}
	for _, bad := range []CadenceCell{
		{1_000_000, 80_000_000, 500_000_000, 619},
		{1_000_000, 40_000_000, 500_000_000, 13001},
		{1_000_000, 80_000_000, 50_000_000, 13001},
	} {
		if _, err := LockCadence(bad, identity); err == nil {
			t.Fatalf("out-of-matrix cell accepted: %+v", bad)
		}
	}
	if _, _, err := DecodeCadencePlan([]byte(`{"schema_version":1,"schema_version":1}`)); err == nil {
		t.Fatal("duplicate plan key accepted")
	}
}

func TestCadenceFirstActionPhaseAndEvidence(t *testing.T) {
	for _, test := range []struct {
		poll, expectedDecision, expectedPollWait int64
	}{
		{1_000_000, 1_000_000_000, 0},
		{80_000_000, 1_040_000_000, 40_000_000},
	} {
		cell := CadenceCell{NetworkLatencyNanos: 1_000_000, PollIntervalNanos: test.poll, TargetQty: 500_000_000, Seed: 13001}
		raw, identity, effective, actorReport := cadenceFixture(t, cell)
		got, err := ReconstructCadence(bytes.NewReader(raw), identity, effective, cell)
		if err != nil {
			t.Fatal(err)
		}
		if got.Outcome.DecisionAt != test.expectedDecision ||
			got.Timing.ScheduledFirstGateTickNanos != test.expectedDecision ||
			got.Timing.FirstUsableQuoteAtNanos == nil ||
			got.Timing.GateAdjustedPollWaitNanos == nil ||
			*got.Timing.GateAdjustedPollWaitNanos != test.expectedPollWait ||
			got.Timing.SelectedQuoteAgeNanos == nil || *got.Timing.SelectedQuoteAgeNanos < 0 {
			t.Fatalf("wrong first-action phase: %+v", got)
		}
		actorRaw, err := json.Marshal(actorReport)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := decodeLatencyActorReport(actorRaw)
		if err != nil || compareActorReport(got.Outcome, decoded) != nil {
			t.Fatalf("actor and replay disagree: %v", err)
		}
		if test.poll == 1_000_000 {
			legacy, err := ReconstructLatency(bytes.NewReader(raw), identity, effective, cell.TargetQty)
			if err != nil || !reflect.DeepEqual(legacy, got.Outcome) {
				t.Fatalf("1-ms control diverged from ME-002 replay: %v", err)
			}
		}
	}
}

func TestCadenceTickAndOrderMutationFailClosed(t *testing.T) {
	cell := CadenceCell{NetworkLatencyNanos: 1_000_000, PollIntervalNanos: 80_000_000, TargetQty: 500_000_000, Seed: 13001}
	raw, identity, effective, _ := cadenceFixture(t, cell)
	var original []RecordedEvent
	if err := WalkLatencyEvidence(bytes.NewReader(raw), identity, func(event RecordedEvent) error {
		original = append(original, event)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"drop_tick", "duplicate_tick", "send_before_tick", "wrong_tick_time", "future_snapshot"} {
		t.Run(mutation, func(t *testing.T) {
			events := append([]RecordedEvent(nil), original...)
			tickPosition, sendPosition := -1, -1
			for index, event := range events {
				if event.ClientID == 13 && event.Timestamp == 1_040_000_000 {
					if event.Name == "decision_tick" {
						tickPosition = index
					}
					if event.Name == "order_send" {
						sendPosition = index
					}
				}
			}
			if tickPosition < 0 || sendPosition < 0 || tickPosition >= sendPosition {
				t.Fatal("fixture lacks ordered tick and send")
			}
			switch mutation {
			case "drop_tick":
				events = append(events[:tickPosition], events[tickPosition+1:]...)
			case "duplicate_tick":
				events = append(events[:tickPosition+1], append([]RecordedEvent{events[tickPosition]}, events[tickPosition+1:]...)...)
			case "send_before_tick":
				events[tickPosition], events[sendPosition] = events[sendPosition], events[tickPosition]
			case "wrong_tick_time":
				events[tickPosition].Timestamp++
			case "future_snapshot":
				for index := range events {
					if events[index].ClientID == 13 && events[index].Name == "book_snapshot_receipt" {
						var payload map[string]any
						if err := json.Unmarshal(events[index].Payload, &payload); err != nil {
							t.Fatal(err)
						}
						payload["Timestamp"] = float64(events[index].Timestamp + 1)
						events[index].Payload, _ = json.Marshal(payload)
						break
					}
				}
			}
			var changed bytes.Buffer
			recorder := NewLatencyRecorder(&changed)
			for _, event := range events {
				recorder.Record(executionlab.EvidenceObservation{Timestamp: event.Timestamp, ClientID: event.ClientID,
					Source: event.Source, Name: event.Name, Route: event.Route, Payload: event.Payload})
			}
			changedIdentity, err := recorder.Finish()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ReconstructCadence(bytes.NewReader(changed.Bytes()), changedIdentity, effective, cell); err == nil {
				t.Fatal("validly rehashed cadence mutation accepted")
			}
		})
	}
}

func TestCadenceNeighboringGatePhaseAndEvidenceNeutrality(t *testing.T) {
	cell := CadenceCell{NetworkLatencyNanos: 90_000_000, PollIntervalNanos: 80_000_000, TargetQty: 500_000_000, Seed: 13011}
	for _, test := range []struct {
		gate, firstTick int64
	}{
		{960_000_000, 960_000_000},
		{1_000_000_000, 1_040_000_000},
		{1_040_000_000, 1_040_000_000},
	} {
		config := executionlab.DefaultSimConfig(executionlab.Immediate)
		config.Seed = cell.Seed
		config.Parent.TargetQty = cell.TargetQty
		config.Parent.PollInterval = 80_000_000
		config.Parent.DecisionAfter = time.Duration(test.gate)
		config.ExecutionLatency = 0
		config.ParentDeployment = &executionlab.ParentDeployment{
			MarketDataLatency: 90_000_000, RequestLatency: 90_000_000, ResponseLatency: 90_000_000,
		}
		world, err := executionlab.NewSim(config)
		if err != nil {
			t.Fatal(err)
		}
		report, err := world.Run(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if report.DecisionAt != test.firstTick {
			t.Fatalf("gate %d acted at %d, want %d", test.gate, report.DecisionAt, test.firstTick)
		}
	}
	raw, identity, effective, observed := cadenceFixture(t, cell)
	reconstructed, err := ReconstructCadence(bytes.NewReader(raw), identity, effective, cell)
	if err != nil {
		t.Fatal(err)
	}
	if reconstructed.Outcome.DecisionAt != 1_040_000_000 {
		t.Fatalf("slow-network fixture missed gate phase: %+v", reconstructed.Timing)
	}
	plain, err := NewCadenceWorld(cell)
	if err != nil {
		t.Fatal(err)
	}
	unobserved, err := plain.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(observed, unobserved) {
		t.Fatal("evidence observer changed actor report")
	}
}
