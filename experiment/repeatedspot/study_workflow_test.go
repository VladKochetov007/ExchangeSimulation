package repeatedspot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestE0ManifestBindingAndExclusivePublication(t *testing.T) {
	cell := E0Cell{Composition: "P", QuoteQty: e0QuoteSmall, Seed: e0DevelopmentSeeds[0]}
	identity := syntheticE0Identity()
	plan, err := LockE0Plan(cell, identity)
	if err != nil {
		t.Fatal(err)
	}
	contractHash := sha256.Sum256(plan.EffectiveWorld)
	manifest := E0RunManifest{SchemaVersion: 1, Cell: cell, PlanRawSHA256: strings.Repeat("a", 64),
		TypedPlanSHA256: plan.TypedPlanSHA256, Identity: identity,
		ContractSHA256:     hex.EncodeToString(contractHash[:]),
		Evidence:           EvidenceIdentity{SchemaID: EvidenceSchemaID, ExecutionHash: strings.Repeat("b", 64), FrameCount: 1},
		EvidenceFileSHA256: strings.Repeat("c", 64), SidecarSHA256: make(map[string]string)}
	for _, name := range e0Sidecars {
		manifest.SidecarSHA256[name] = strings.Repeat("d", 64)
	}
	if err := verifyE0Manifest(plan, manifest.PlanRawSHA256, identity, manifest); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*E0RunManifest){
		"cell":        func(value *E0RunManifest) { value.Cell.Seed = e0DevelopmentSeeds[1] },
		"plan digest": func(value *E0RunManifest) { value.PlanRawSHA256 = strings.Repeat("0", 64) },
		"source": func(value *E0RunManifest) {
			value.Identity.SourceCommit = strings.Repeat("e", 40)
		},
		"contract": func(value *E0RunManifest) { value.ContractSHA256 = strings.Repeat("0", 64) },
		"evidence schema": func(value *E0RunManifest) {
			value.Evidence.SchemaID = "repeated-spot-opaque-v2"
		},
		"missing sidecar": func(value *E0RunManifest) {
			value.SidecarSHA256 = make(map[string]string)
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := manifest
			mutate(&changed)
			if err := verifyE0Manifest(plan, manifest.PlanRawSHA256, identity, changed); err == nil {
				t.Fatal("mutated E0 run manifest was accepted")
			}
		})
	}
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := writeE0ExclusiveJSON(path, manifest); err != nil {
		t.Fatal(err)
	}
	if err := writeE0ExclusiveJSON(path, manifest); err == nil {
		t.Fatal("existing E0 manifest was overwritten")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded E0RunManifest
	if err := decodeE0Strict(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Cell != cell {
		t.Fatal("exclusive E0 manifest did not round-trip")
	}
	for _, malformed := range [][]byte{
		append([]byte(`{"schema_version":1,`), raw[1:]...),
		[]byte(`{"schema_version":1,"unexpected":1}`),
	} {
		if err := decodeE0Strict(malformed, &E0RunManifest{}); err == nil {
			t.Fatal("malformed or unknown E0 manifest field accepted")
		}
	}
	if _, err := json.Marshal(decoded); err != nil {
		t.Fatal(err)
	}
}

func TestE0OutputsCannotDirtyPinnedSourceCheckout(t *testing.T) {
	repository := t.TempDir()
	outside := t.TempDir()
	if err := requireE0ExternalOutput(repository, filepath.Join(repository, "plan.json")); err == nil {
		t.Fatal("plan inside pinned checkout was accepted")
	}
	if err := requireE0ExternalOutput(repository, filepath.Join(outside, "plan.json")); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(outside, "source-alias")
	if err := os.Symlink(repository, alias); err != nil {
		t.Fatal(err)
	}
	if err := requireE0ExternalOutput(repository, filepath.Join(alias, "plan.json")); err == nil {
		t.Fatal("symlink into pinned checkout was accepted")
	}
}
