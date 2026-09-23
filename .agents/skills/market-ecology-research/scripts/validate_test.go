package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func projectRoot(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("source location unavailable")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(source), "../../../.."))
}

func templateResult(t *testing.T) Result {
	t.Helper()
	var result Result
	if err := readJSON(filepath.Join(projectRoot(t), skillPath, "assets/result-template.json"), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestRepositoryMetadata(t *testing.T) {
	if err := validate(projectRoot(t)); err != nil {
		t.Fatal(err)
	}
	var registry Registry
	if err := readJSON(filepath.Join(projectRoot(t), "research/program/registry.json"), &registry); err != nil {
		t.Fatal(err)
	}
	if len(registry.Studies) < 13 || len(registry.Policies) < 24 {
		t.Fatal("initial queue or catalogue missing")
	}
	for _, study := range registry.Studies {
		if study.Authorization.Prepare || study.Authorization.Run {
			t.Fatalf("authoring fixture cannot authorize study %s", study.ID)
		}
	}
}

func TestRegistryMutations(t *testing.T) {
	cases := map[string]func(*Registry){
		"duplicate ID":          func(r *Registry) { r.Studies = append(r.Studies, r.Studies[0]) },
		"dangling dependency":   func(r *Registry) { r.Studies[0].Dependencies = []string{"ME-999"} },
		"cycle":                 func(r *Registry) { r.Studies[0].Dependencies = []string{"ME-001"} },
		"missing policy":        func(r *Registry) { r.Studies[0].PolicyIDs = []string{"S99"} },
		"missing file":          func(r *Registry) { r.Studies[0].Idea = "research/program/absent.md" },
		"path escape":           func(r *Registry) { r.Studies[0].Idea = "../outside.md" },
		"unsupported status":    func(r *Registry) { r.Studies[0].Stage = "VALIDATED_FOREVER" },
		"unsupported claim":     func(r *Registry) { r.Studies[0].ClaimType = "REALISM_CERTIFIED" },
		"run without protocol":  func(r *Registry) { r.Studies[0].Authorization.Run = true },
		"unknown source status": func(r *Registry) { r.Policies[0].SourceStatus = "PROFITABLE" },
		"empty source claim":    func(r *Registry) { r.Policies[0].Sources = nil },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			var registry Registry
			if err := readJSON(filepath.Join(projectRoot(t), "research/program/registry.json"), &registry); err != nil {
				t.Fatal(err)
			}
			mutate(&registry)
			if validateRegistry(projectRoot(t), registry) == nil {
				t.Fatal("invalid metadata accepted")
			}
		})
	}
}

func TestResultMutations(t *testing.T) {
	cases := map[string]func(*Result){
		"invalid evidence economic verdict": func(r *Result) { r.EvidenceValidity = "INVALID"; r.ScientificVerdict = "NOT_SUPPORTED" },
		"service error acceptance":          func(r *Result) { r.Review.Execution = "UNAVAILABLE"; r.Review.Verdict = "ACCEPT" },
		"substantive review no scope":       func(r *Result) { r.Review.Execution = "COMPLETED"; r.Review.Verdict = "REJECT" },
		"no run evidence":                   func(r *Result) { r.EvidenceValidity = "VALID" },
		"unknown enum":                      func(r *Result) { r.PolicyActivity = "PROFITABLE" },
		"missing identity":                  func(r *Result) { delete(r.Identities, "analyzer") },
		"dangling claim": func(r *Result) {
			r.ProcessStatus = "COMPLETED"
			r.EvidenceValidity = "VALID"
			r.Claims = []Claim{{ID: "C1", Type: "MECHANICAL", Text: "fixture", EvidenceIDs: []string{"missing"}, Limitations: []string{"synthetic"}}}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			result := templateResult(t)
			mutate(&result)
			if validateResult(result) == nil {
				t.Fatal("contradictory result accepted")
			}
		})
	}
}

func TestValidNoTradeAndUnavailableReview(t *testing.T) {
	result := templateResult(t)
	result.ProcessStatus = "COMPLETED"
	result.EvidenceValidity = "VALID"
	result.OpportunityPresence = "PRESENT"
	result.PolicyActivity = "INACTIVE"
	result.RegisteredActivation = "NOT_APPLICABLE"
	result.ScientificVerdict = "NOT_ISSUED"
	result.Review.Execution = "UNAVAILABLE"
	result.Reasons = []string{"Synthetic case: policy rationally abstained; service unavailable is not a verdict."}
	if err := validateResult(result); err != nil {
		t.Fatal(err)
	}
}

func TestStrictDecode(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(projectRoot(t), skillPath, "assets/result-template.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	delete(fields, "registered_activation")
	missing, _ := json.Marshal(fields)
	for _, invalid := range [][]byte{
		missing,
		[]byte(strings.Replace(string(data), `"schema_version": 1`, `"schema_version": 1, "unknown": true`, 1)),
		[]byte(strings.Replace(string(data), `"run": false`, `"run": null`, 1)),
		append(append([]byte{}, data...), []byte("{}")...),
	} {
		var result Result
		if decode(invalid, &result) == nil {
			t.Fatal("malformed/unknown/omitted/trailing input accepted")
		}
	}
}
