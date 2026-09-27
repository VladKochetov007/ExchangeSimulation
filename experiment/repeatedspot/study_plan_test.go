package repeatedspot

import (
	"encoding/json"
	"strings"
	"testing"

	"exchange_sim/experiment/executionpilot"
)

func syntheticE0Identity() executionpilot.Identity {
	return executionpilot.Identity{SourceCommit: strings.Repeat("a", 40), SourceTree: strings.Repeat("b", 40),
		SimulatorSHA256: strings.Repeat("c", 64), AnalyzerSHA256: strings.Repeat("d", 64),
		Toolchain: "go1.27.0", EvidenceSchemaID: EvidenceSchemaID}
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
