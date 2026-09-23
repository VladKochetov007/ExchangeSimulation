package executionpilot

import (
	"encoding/json"
	"testing"
)

func TestLatencyPlanPinsEachDeploymentAndRejectsMutations(t *testing.T) {
	identity := fixtureIdentity()
	identity.EvidenceSchemaID = LatencyEvidenceSchemaID
	for _, network := range []int64{1_000_000, 90_000_000} {
		for _, processing := range []int64{0, 120_000_000} {
			cell := LatencyCell{NetworkLatencyNanos: network, ProcessingDelayNanos: processing, TargetQty: 50_000_000, Seed: 12001}
			plan, err := LockLatency(cell, identity)
			if err != nil {
				t.Fatal(err)
			}
			world, err := VerifyLatency(plan, identity)
			if err != nil {
				t.Fatal(err)
			}
			contract := world.WorldContract()
			deployment := contract.Parents[0].Deployment
			if deployment == nil || int64(deployment.MarketDataLatency) != network ||
				int64(deployment.RequestLatency) != network || int64(deployment.ResponseLatency) != network ||
				int64(deployment.ProcessingDelay) != processing || contract.Parents[0].ClientID != 13 {
				t.Fatalf("effective deployment does not match cell: %+v", contract.Parents[0])
			}
			raw, err := json.Marshal(plan)
			if err != nil {
				t.Fatal(err)
			}
			decoded, _, err := DecodeLatencyPlan(raw)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := VerifyLatency(decoded, identity); err != nil {
				t.Fatal(err)
			}
			decoded.Cell.NetworkLatencyNanos++
			if _, err := VerifyLatency(decoded, identity); err == nil {
				t.Fatal("mutated cell accepted")
			}
		}
	}
	if _, err := LockLatency(LatencyCell{NetworkLatencyNanos: 1_000_000, ProcessingDelayNanos: 0, TargetQty: 50_000_000, Seed: 619}, identity); err == nil {
		t.Fatal("historical holdout label accepted")
	}
	if _, _, err := DecodeLatencyPlan([]byte(`{"schema_version":1,"schema_version":1}`)); err == nil {
		t.Fatal("duplicate plan field accepted")
	}
	identity.EvidenceSchemaID = EvidenceSchemaID
	if _, err := LockLatency(LatencyCell{NetworkLatencyNanos: 1_000_000, ProcessingDelayNanos: 0, TargetQty: 50_000_000, Seed: 12001}, identity); err == nil {
		t.Fatal("ME-001 evidence identity accepted for ME-002")
	}
}
