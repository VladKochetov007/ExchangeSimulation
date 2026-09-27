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

const me016PlanSchemaVersion = 1

type ME016LockedPlan struct {
	SchemaVersion   int                     `json:"schema_version"`
	Cell            ME016Cell               `json:"cell"`
	Identity        executionpilot.Identity `json:"identity"`
	EffectiveWorld  json.RawMessage         `json:"effective_world"`
	Window          MeasurementWindow       `json:"measurement_window"`
	TypedPlanSHA256 string                  `json:"typed_plan_sha256"`
}

func LockME016Plan(cell ME016Cell, identity executionpilot.Identity) (ME016LockedPlan, error) {
	if err := executionpilot.ValidateIdentityForSchema(identity, SignalEvidenceSchemaID); err != nil {
		return ME016LockedPlan{}, err
	}
	world, err := BuildME016World(cell)
	if err != nil {
		return ME016LockedPlan{}, err
	}
	defer world.Close()
	plan := ME016LockedPlan{SchemaVersion: me016PlanSchemaVersion, Cell: cell,
		Identity: identity, EffectiveWorld: world.ContractJSON(), Window: E0MeasurementWindow()}
	plan.TypedPlanSHA256, err = digestME016Plan(plan)
	return plan, err
}

func VerifyME016Plan(plan ME016LockedPlan, actual executionpilot.Identity) (*worldspot.World, error) {
	if plan.SchemaVersion != me016PlanSchemaVersion || plan.Identity != actual ||
		plan.Window != E0MeasurementWindow() {
		return nil, errors.New("repeated spot: ME-016 plan schema, identity or measurement window changed")
	}
	if err := executionpilot.ValidateIdentityForSchema(actual, SignalEvidenceSchemaID); err != nil {
		return nil, err
	}
	world, err := BuildME016World(plan.Cell)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(plan.EffectiveWorld, world.ContractJSON()) {
		world.Close()
		return nil, errors.New("repeated spot: ME-016 plan differs from effective world")
	}
	digest, err := digestME016Plan(plan)
	if err != nil || digest != plan.TypedPlanSHA256 {
		world.Close()
		return nil, errors.New("repeated spot: ME-016 typed plan digest mismatch")
	}
	return world, nil
}

func DecodeME016Plan(raw []byte) (ME016LockedPlan, string, error) {
	if err := executionpilot.ValidateStrictJSON(raw); err != nil {
		return ME016LockedPlan{}, "", err
	}
	var plan ME016LockedPlan
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return ME016LockedPlan{}, "", err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return ME016LockedPlan{}, "", errors.New("repeated spot: trailing ME-016 plan content")
	}
	if len(plan.EffectiveWorld) == 0 || plan.TypedPlanSHA256 == "" {
		return ME016LockedPlan{}, "", errors.New("repeated spot: incomplete ME-016 plan")
	}
	var canonicalWorld bytes.Buffer
	if err := json.Compact(&canonicalWorld, plan.EffectiveWorld); err != nil {
		return ME016LockedPlan{}, "", fmt.Errorf("repeated spot: invalid ME-016 effective world: %w", err)
	}
	plan.EffectiveWorld = canonicalWorld.Bytes()
	hash := sha256.Sum256(raw)
	return plan, hex.EncodeToString(hash[:]), nil
}

func digestME016Plan(plan ME016LockedPlan) (string, error) {
	encoded, err := json.Marshal(struct {
		SchemaVersion  int                     `json:"schema_version"`
		Cell           ME016Cell               `json:"cell"`
		Identity       executionpilot.Identity `json:"identity"`
		EffectiveWorld json.RawMessage         `json:"effective_world"`
		Window         MeasurementWindow       `json:"measurement_window"`
	}{plan.SchemaVersion, plan.Cell, plan.Identity, plan.EffectiveWorld, plan.Window})
	if err != nil {
		return "", fmt.Errorf("repeated spot: encode ME-016 plan: %w", err)
	}
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:]), nil
}
