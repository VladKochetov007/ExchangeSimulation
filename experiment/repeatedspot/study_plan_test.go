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

func syntheticE0Identity() executionpilot.Identity {
	return executionpilot.Identity{SourceCommit: strings.Repeat("a", 40), SourceTree: strings.Repeat("b", 40),
		SimulatorSHA256: strings.Repeat("c", 64), AnalyzerSHA256: strings.Repeat("d", 64),
		Toolchain: "go1.27.0", EvidenceSchemaID: EvidenceSchemaID}
}

func TestE0PublishedPlanRoundTripsIntoRunVerifier(t *testing.T) {
	cell := E0Cell{Composition: "P", QuoteQty: e0QuoteSmall, Seed: e0DevelopmentSeeds[0]}
	identity := syntheticE0Identity()
	plan, err := LockE0Plan(cell, identity)
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
	decoded, rawDigest, err := DecodeE0Plan(raw)
	if err != nil || rawDigest == "" || decoded.TypedPlanSHA256 != plan.TypedPlanSHA256 {
		t.Fatalf("published plan lost its identities: %v", err)
	}
	world, err := VerifyE0Plan(decoded, identity)
	if err != nil {
		t.Fatalf("writer-produced plan could not pass the runner's verifier: %v", err)
	}
	if !bytes.Equal(decoded.EffectiveWorld, world.ContractJSON()) {
		t.Fatal("decoded plan retained noncanonical contract bytes for replay")
	}
	manifest := E0RunManifest{SchemaVersion: 1, Cell: cell, PlanRawSHA256: rawDigest,
		TypedPlanSHA256: decoded.TypedPlanSHA256, Identity: identity,
		ContractSHA256: world.ContractSHA256(), Evidence: EvidenceIdentity{SchemaID: EvidenceSchemaID},
		SidecarSHA256: make(map[string]string)}
	for _, name := range e0Sidecars {
		manifest.SidecarSHA256[name] = strings.Repeat("a", 64)
	}
	if err := verifyE0Manifest(decoded, rawDigest, identity, manifest); err != nil {
		t.Fatalf("writer-produced plan could not verify the runner's canonical manifest: %v", err)
	}
	world.Close()
}

func TestE0PlanBindsCellWorldWindowAndToolchain(t *testing.T) {
	cell := E0Cell{Composition: "M1", QuoteQty: e0QuoteSmall, Seed: e0DevelopmentSeeds[0]}
	identity := syntheticE0Identity()
	plan, err := LockE0Plan(cell, identity)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	decoded, rawDigest, err := DecodeE0Plan(raw)
	if err != nil || rawDigest == "" {
		t.Fatalf("valid locked E0 plan failed to decode: %v", err)
	}
	world, err := VerifyE0Plan(decoded, identity)
	if err != nil {
		t.Fatal(err)
	}
	world.Close()
	for name, mutate := range map[string]func(*E0LockedPlan){
		"different seed":   func(plan *E0LockedPlan) { plan.Cell.Seed = e0DevelopmentSeeds[1] },
		"different source": func(plan *E0LockedPlan) { plan.Identity.SourceCommit = strings.Repeat("e", 40) },
		"different window": func(plan *E0LockedPlan) { plan.Window.StartAt++ },
		"different effective world": func(plan *E0LockedPlan) {
			plan.EffectiveWorld = append([]byte(nil), plan.EffectiveWorld...)
			plan.EffectiveWorld[0] = '['
		},
		"different typed digest": func(plan *E0LockedPlan) { plan.TypedPlanSHA256 = strings.Repeat("0", 64) },
	} {
		t.Run(name, func(t *testing.T) {
			changed := plan
			mutate(&changed)
			if world, err := VerifyE0Plan(changed, identity); err == nil {
				world.Close()
				t.Fatal("mutated E0 plan was accepted")
			}
		})
	}
	withDuplicateKey := append([]byte(`{"schema_version":1,`), raw[1:]...)
	if _, _, err := DecodeE0Plan(withDuplicateKey); err == nil {
		t.Fatal("duplicate E0 plan field was accepted")
	}
}
