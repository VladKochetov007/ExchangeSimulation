package repeatedspot

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"exchange_sim/experiment/executionpilot"
)

func syntheticME016Identity() executionpilot.Identity {
	identity := syntheticE0Identity()
	identity.EvidenceSchemaID = SignalEvidenceSchemaID
	return identity
}

func TestME016PlanAndManifestBindTheExactV5World(t *testing.T) {
	cell := ME016DevelopmentCells()[0]
	identity := syntheticME016Identity()
	plan, err := LockME016Plan(cell, identity)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "plan.json")
	if err := writeE0ExclusiveJSON(path, plan); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	decoded, rawDigest, err := DecodeME016Plan(raw)
	if err != nil || rawDigest == "" || decoded.TypedPlanSHA256 != plan.TypedPlanSHA256 {
		t.Fatalf("ME-016 plan did not round-trip: %v", err)
	}
	world, err := VerifyME016Plan(decoded, identity)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded.EffectiveWorld, world.ContractJSON()) {
		t.Fatal("decoded ME-016 contract is not the builder's canonical world")
	}
	manifest := ME016RunManifest{SchemaVersion: 1, Cell: cell, PlanRawSHA256: rawDigest,
		TypedPlanSHA256: plan.TypedPlanSHA256, Identity: identity,
		ContractSHA256: world.ContractSHA256(), Evidence: EvidenceIdentity{
			SchemaID: SignalEvidenceSchemaID, ExecutionHash: strings.Repeat("e", 64), FrameCount: 1},
		EvidenceFileSHA256: strings.Repeat("f", 64), SidecarSHA256: make(map[string]string)}
	world.Close()
	for _, name := range e0Sidecars {
		manifest.SidecarSHA256[name] = strings.Repeat("a", 64)
	}
	if err := verifyME016Manifest(plan, rawDigest, identity, manifest); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*ME016LockedPlan){
		"seed":     func(value *ME016LockedPlan) { value.Cell.Seed = 619 },
		"gain":     func(value *ME016LockedPlan) { value.Cell.SignalGainBps = 2 },
		"window":   func(value *ME016LockedPlan) { value.Window.StartAt++ },
		"source":   func(value *ME016LockedPlan) { value.Identity.SourceCommit = strings.Repeat("0", 40) },
		"contract": func(value *ME016LockedPlan) { value.EffectiveWorld = json.RawMessage(`{}`) },
		"digest":   func(value *ME016LockedPlan) { value.TypedPlanSHA256 = strings.Repeat("0", 64) },
	} {
		t.Run(name, func(t *testing.T) {
			changed := plan
			mutate(&changed)
			if opened, err := VerifyME016Plan(changed, identity); err == nil {
				opened.Close()
				t.Fatal("changed ME-016 plan passed source/world verification")
			}
		})
	}
	for name, mutate := range map[string]func(*ME016RunManifest){
		"raw plan": func(value *ME016RunManifest) { value.PlanRawSHA256 = strings.Repeat("0", 64) },
		"typed plan": func(value *ME016RunManifest) {
			value.TypedPlanSHA256 = strings.Repeat("0", 64)
		},
		"source": func(value *ME016RunManifest) { value.Identity.SourceCommit = strings.Repeat("0", 40) },
		"contract": func(value *ME016RunManifest) {
			value.ContractSHA256 = strings.Repeat("0", 64)
		},
		"schema":  func(value *ME016RunManifest) { value.Evidence.SchemaID = EvidenceSchemaID },
		"sidecar": func(value *ME016RunManifest) { value.SidecarSHA256 = map[string]string{} },
	} {
		t.Run(name, func(t *testing.T) {
			changed := manifest
			mutate(&changed)
			if err := verifyME016Manifest(plan, rawDigest, identity, changed); err == nil {
				t.Fatal("changed ME-016 run manifest passed verification")
			}
		})
	}
	for _, malformed := range [][]byte{
		append([]byte(`{"schema_version":1,`), raw[1:]...),
		[]byte(`{"schema_version":1,"extra":1}`),
	} {
		if _, _, err := DecodeME016Plan(malformed); err == nil {
			t.Fatal("duplicate or unknown ME-016 plan field accepted")
		}
	}
}

func TestME016WrongSchemaOrOutputRootFailsBeforeWorldStartup(t *testing.T) {
	cell := ME016DevelopmentCells()[0]
	if _, err := LockME016Plan(cell, syntheticE0Identity()); err == nil {
		t.Fatal("legacy evidence identity accepted for signal world")
	}
	identity := syntheticME016Identity()
	plan, err := LockME016Plan(cell, identity)
	if err != nil {
		t.Fatal(err)
	}
	plan.Identity.AnalyzerSHA256 = strings.Repeat("0", 64)
	if opened, err := VerifyME016Plan(plan, identity); err == nil {
		opened.Close()
		t.Fatal("different analysis binary accepted before startup")
	}
	repository := t.TempDir()
	if err := requireE0ExternalOutput(repository, filepath.Join(repository, "me016-run")); err == nil {
		t.Fatal("ME-016 output inside pinned checkout accepted")
	}
}
