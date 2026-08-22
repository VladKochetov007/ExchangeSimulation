package main

import (
	"strings"
	"testing"
)

func TestArmForRecognizesFrozenPhase2BaselinesAsControls(t *testing.T) {
	manifest := &Manifest{Arms: map[string]ArmContract{"control": {}}}

	for _, run := range []string{"f2_baseline_101", "f2_baseline_102", "f2_baseline_103"} {
		if got := armFor(run, manifest); got != "control" {
			t.Errorf("armFor(%q) = %q, want control", run, got)
		}
	}
}

func TestCheckRequirementChecksTheDeclaredMetricRatherThanAnyNonzeroValue(t *testing.T) {
	payload := map[string]any{
		"result": map[string]any{
			"risk_samples": float64(0),
			"books":        float64(9),
			"transmission": []any{},
		},
	}
	if err := checkRequirement(payload, "result.risk_samples > 0"); err == nil {
		t.Fatal("zero primary metric passed because an unrelated field was nonzero")
	}
	if err := checkRequirement(payload, "result.risk_samples > 0 && result.transmission non-empty"); err == nil || !strings.Contains(err.Error(), "risk_samples") {
		t.Fatalf("wrong failure for compound primary requirement: %v", err)
	}
	payload["result"].(map[string]any)["risk_samples"] = float64(4)
	if err := checkRequirement(payload, "result.risk_samples > 0 && result.transmission non-empty"); err == nil || !strings.Contains(err.Error(), "transmission") {
		t.Fatalf("empty required collection passed or wrong failure: %v", err)
	}
	payload["result"].(map[string]any)["transmission"] = []any{map[string]any{"n": float64(1)}}
	if err := checkRequirement(payload, "result.risk_samples > 0 && result.transmission non-empty"); err != nil {
		t.Fatalf("valid declared measurements rejected: %v", err)
	}
}

func TestCheckRequirementPresentAllowsAZeroValuedObservedField(t *testing.T) {
	payload := map[string]any{"result": map[string]any{"mismatched": float64(0)}}
	if err := checkRequirement(payload, "result.mismatched present"); err != nil {
		t.Fatalf("observed zero field should satisfy present: %v", err)
	}
}
