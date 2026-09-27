package repeatedspot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"exchange_sim/experiment/executionpilot"
	worldspot "exchange_sim/simulations/repeatedspot"
)

const e0PlanSchemaVersion = 1

type E0LockedPlan struct {
	SchemaVersion   int                     `json:"schema_version"`
	Cell            E0Cell                  `json:"cell"`
	Identity        executionpilot.Identity `json:"identity"`
	EffectiveWorld  json.RawMessage         `json:"effective_world"`
	Window          MeasurementWindow       `json:"measurement_window"`
	TypedPlanSHA256 string                  `json:"typed_plan_sha256"`
}

func LockE0Plan(cell E0Cell, identity executionpilot.Identity) (E0LockedPlan, error) {
	if err := executionpilot.ValidateIdentityForSchema(identity, EvidenceSchemaID); err != nil {
		return E0LockedPlan{}, err
	}
	world, err := buildRegisteredE0World(cell)
	if err != nil {
		return E0LockedPlan{}, err
	}
	defer world.Close()
	plan := E0LockedPlan{SchemaVersion: e0PlanSchemaVersion, Cell: cell, Identity: identity,
		EffectiveWorld: world.ContractJSON(), Window: E0MeasurementWindow()}
	plan.TypedPlanSHA256, err = digestE0Plan(plan)
	return plan, err
}

func VerifyE0Plan(plan E0LockedPlan, actual executionpilot.Identity) (*worldspot.World, error) {
	if plan.SchemaVersion != e0PlanSchemaVersion || plan.Identity != actual ||
		plan.Window != E0MeasurementWindow() {
		return nil, errors.New("repeated spot: E0 plan schema, identity or measurement window changed")
	}
	if err := executionpilot.ValidateIdentityForSchema(actual, EvidenceSchemaID); err != nil {
		return nil, err
	}
	world, err := buildRegisteredE0World(plan.Cell)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(plan.EffectiveWorld, world.ContractJSON()) {
		world.Close()
		return nil, errors.New("repeated spot: E0 plan differs from effective world")
	}
	digest, err := digestE0Plan(plan)
	if err != nil || digest != plan.TypedPlanSHA256 {
		world.Close()
		return nil, errors.New("repeated spot: E0 typed plan digest mismatch")
	}
	return world, nil
}

func DecodeE0Plan(raw []byte) (E0LockedPlan, string, error) {
	if err := executionpilot.ValidateStrictJSON(raw); err != nil {
		return E0LockedPlan{}, "", err
	}
	var plan E0LockedPlan
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return E0LockedPlan{}, "", err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return E0LockedPlan{}, "", errors.New("repeated spot: trailing E0 plan content")
	}
	if len(plan.EffectiveWorld) == 0 || plan.TypedPlanSHA256 == "" {
		return E0LockedPlan{}, "", errors.New("repeated spot: incomplete E0 plan")
	}
	// The plan writer may indent RawMessage; downstream contract hashes and
	// replay must use the world's canonical bytes, not presentation whitespace.
	var canonicalWorld bytes.Buffer
	if err := json.Compact(&canonicalWorld, plan.EffectiveWorld); err != nil {
		return E0LockedPlan{}, "", fmt.Errorf("repeated spot: invalid effective world JSON: %w", err)
	}
	plan.EffectiveWorld = canonicalWorld.Bytes()
	hash := sha256.Sum256(raw)
	return plan, hex.EncodeToString(hash[:]), nil
}

func digestE0Plan(plan E0LockedPlan) (string, error) {
	encoded, err := json.Marshal(struct {
		SchemaVersion  int                     `json:"schema_version"`
		Cell           E0Cell                  `json:"cell"`
		Identity       executionpilot.Identity `json:"identity"`
		EffectiveWorld json.RawMessage         `json:"effective_world"`
		Window         MeasurementWindow       `json:"measurement_window"`
	}{plan.SchemaVersion, plan.Cell, plan.Identity, plan.EffectiveWorld, plan.Window})
	if err != nil {
		return "", fmt.Errorf("repeated spot: encode E0 plan: %w", err)
	}
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:]), nil
}
