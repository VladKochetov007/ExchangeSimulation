package executionpilot

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func fixtureIdentity() Identity {
	return Identity{
		SourceCommit: strings.Repeat("a", 40), SourceTree: strings.Repeat("b", 40),
		SimulatorSHA256: strings.Repeat("c", 64), AnalyzerSHA256: strings.Repeat("d", 64),
		Toolchain: RequiredToolchain, EvidenceSchemaID: EvidenceSchemaID,
	}
}

func TestProspectiveMatrixPlansDistinct(t *testing.T) {
	cells, err := Cells([]int64{1009, 1013, 1019})
	if err != nil || len(cells) != 27 {
		t.Fatalf("cells=%d err=%v", len(cells), err)
	}
	seen := map[string]bool{}
	for _, cell := range cells {
		plan, err := Lock(cell, fixtureIdentity())
		if err != nil {
			t.Fatal(err)
		}
		if seen[plan.TypedPlanSHA256] {
			t.Fatalf("duplicate typed plan for %#v", cell)
		}
		seen[plan.TypedPlanSHA256] = true
		if _, err := Verify(plan, fixtureIdentity()); err != nil {
			t.Fatalf("verify cell %#v: %v", cell, err)
		}
	}
}

func TestPlanSeparatesRawAndTypedDigest(t *testing.T) {
	plan, err := Lock(Cell{MakerCount: 4, RandomTakerCount: 8, TargetQty: 200_000_000, Seed: 1009}, fixtureIdentity())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	decoded, rawDigest, err := DecodePlan(raw)
	if err != nil || rawDigest == plan.TypedPlanSHA256 {
		t.Fatalf("raw/typed digests mixed: %v", err)
	}
	pretty := &bytes.Buffer{}
	if err := json.Indent(pretty, raw, "", "  "); err != nil {
		t.Fatal(err)
	}
	prettyPlan, prettyDigest, err := DecodePlan(pretty.Bytes())
	if err != nil || prettyDigest == rawDigest || prettyPlan.TypedPlanSHA256 != plan.TypedPlanSHA256 {
		t.Fatalf("formatting changed typed identity: %v", err)
	}
	if _, err := Verify(prettyPlan, fixtureIdentity()); err != nil {
		t.Fatalf("pretty plan should verify: %v", err)
	}
	if _, err := Verify(decoded, fixtureIdentity()); err != nil {
		t.Fatal(err)
	}

	changed := plan
	changed.Cell.TargetQty = 500_000_000
	if _, err := Verify(changed, fixtureIdentity()); err == nil {
		t.Fatal("stale copied contract accepted after quantity change")
	}
	changed = plan
	changed.Identity.SourceCommit = strings.Repeat("e", 40)
	if _, err := Verify(changed, fixtureIdentity()); err == nil {
		t.Fatal("source identity mismatch accepted")
	}
	changed = plan
	changed.Identity.AnalyzerSHA256 = strings.Repeat("e", 64)
	if _, err := Verify(changed, fixtureIdentity()); err == nil {
		t.Fatal("analyzer identity mismatch accepted")
	}
	changed = plan
	changed.Identity.SourceTree = strings.Repeat("e", 40)
	if _, err := Verify(changed, fixtureIdentity()); err == nil {
		t.Fatal("source tree mismatch accepted")
	}
	changed = plan
	changed.Identity.Toolchain = "go0.0"
	if _, err := Verify(changed, fixtureIdentity()); err == nil {
		t.Fatal("toolchain mismatch accepted")
	}
	changed = plan
	changed.EffectiveWorld = []byte(`{"config":{"Seed":0}}`)
	if _, err := Verify(changed, fixtureIdentity()); err == nil {
		t.Fatal("stale effective world accepted")
	}
}

func TestPlanRejectsMalformedInput(t *testing.T) {
	valid, err := Lock(Cell{MakerCount: 2, RandomTakerCount: 10, TargetQty: 50_000_000, Seed: 1009}, fixtureIdentity())
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(valid)
	for _, malformed := range [][]byte{
		bytes.Replace(raw, []byte(`"seed":1009`), []byte(`"seed":0`), 1),
		bytes.Replace(raw, []byte(`"maker_count":2`), []byte(`"maker_count":-2`), 1),
		bytes.Replace(raw, []byte(`"source_tree":"`), []byte(`"source_tree":"bad`), 1),
		bytes.Replace(raw, []byte(`"schema_version":1`), []byte(`"schema_version":1,"surprise":true`), 1),
		bytes.Replace(raw, []byte(`"seed":1009`), []byte(`"seed":1009,"seed":1009`), 1),
		append(append([]byte(nil), raw...), []byte(`{}`)...),
	} {
		plan, _, decodeErr := DecodePlan(malformed)
		if decodeErr == nil {
			_, decodeErr = Verify(plan, fixtureIdentity())
		}
		if decodeErr == nil {
			t.Fatalf("malformed plan accepted: %s", malformed)
		}
	}
	if _, err := Cells([]int64{1009, 1009, 1019}); err == nil {
		t.Fatal("duplicate development seeds accepted")
	}
}
