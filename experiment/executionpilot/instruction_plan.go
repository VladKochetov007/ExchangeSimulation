package executionpilot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"exchange_sim/exchange"
	"exchange_sim/simulations/executionlab"
)

const InstructionPlanSchemaVersion = 1
const InstructionLimitPrice = int64(5_010_000_000)

type InstructionCell struct {
	TimeInForce string `json:"time_in_force"`
	TargetQty   int64  `json:"target_qty"`
	Seed        int64  `json:"seed"`
}

type InstructionLockedPlan struct {
	SchemaVersion   int             `json:"schema_version"`
	Cell            InstructionCell `json:"cell"`
	Identity        Identity        `json:"identity"`
	EffectiveWorld  json.RawMessage `json:"effective_world"`
	TypedPlanSHA256 string          `json:"typed_plan_sha256"`
}

func ValidateInstructionCell(cell InstructionCell) error {
	if cell.TimeInForce != "IOC" && cell.TimeInForce != "FOK" ||
		!memberInt64(cell.TargetQty, 50_000_000, 500_000_000) ||
		!memberInt64(cell.Seed, 14001, 14011, 14017) {
		return errors.New("instruction pilot: cell outside prospective ME-003 development matrix")
	}
	return nil
}

func NewInstructionWorld(cell InstructionCell) (*executionlab.Sim, error) {
	if err := ValidateInstructionCell(cell); err != nil {
		return nil, err
	}
	config := executionlab.DefaultSimConfig(executionlab.Immediate)
	config.Seed = cell.Seed
	config.Parent.TargetQty = cell.TargetQty
	config.RecordSnapshotProjectionEvidence = true
	timeInForce := exchange.IOC
	if cell.TimeInForce == "FOK" {
		timeInForce = exchange.FOK
	}
	config.Parent.Instruction = &executionlab.ChildInstruction{
		OrderType: exchange.LimitOrder, TimeInForce: timeInForce, LimitPrice: InstructionLimitPrice,
	}
	return executionlab.NewSim(config)
}

func LockInstruction(cell InstructionCell, identity Identity) (InstructionLockedPlan, error) {
	if err := ValidateIdentityForSchema(identity, InstructionEvidenceSchemaID); err != nil {
		return InstructionLockedPlan{}, err
	}
	world, err := NewInstructionWorld(cell)
	if err != nil {
		return InstructionLockedPlan{}, err
	}
	effectiveWorld, err := json.Marshal(world.WorldContract())
	if err != nil {
		return InstructionLockedPlan{}, err
	}
	plan := InstructionLockedPlan{SchemaVersion: InstructionPlanSchemaVersion,
		Cell: cell, Identity: identity, EffectiveWorld: effectiveWorld}
	plan.TypedPlanSHA256, err = instructionPlanDigest(plan)
	return plan, err
}

func VerifyInstruction(plan InstructionLockedPlan, actual Identity) (*executionlab.Sim, error) {
	if plan.SchemaVersion != InstructionPlanSchemaVersion || plan.Identity != actual {
		return nil, errors.New("instruction pilot: plan version or runtime identity mismatch")
	}
	if err := ValidateIdentityForSchema(actual, InstructionEvidenceSchemaID); err != nil {
		return nil, err
	}
	world, err := NewInstructionWorld(plan.Cell)
	if err != nil {
		return nil, err
	}
	consumed, err := json.Marshal(world.WorldContract())
	if err != nil {
		return nil, err
	}
	var supplied bytes.Buffer
	if err := json.Compact(&supplied, plan.EffectiveWorld); err != nil {
		return nil, err
	}
	if !bytes.Equal(supplied.Bytes(), consumed) {
		return nil, errors.New("instruction pilot: effective world differs from constructed world")
	}
	digest, err := instructionPlanDigest(plan)
	if err != nil || digest != plan.TypedPlanSHA256 {
		return nil, errors.New("instruction pilot: typed plan digest mismatch")
	}
	return world, nil
}

func instructionPlanDigest(plan InstructionLockedPlan) (string, error) {
	canonical, err := json.Marshal(struct {
		SchemaVersion  int             `json:"schema_version"`
		Cell           InstructionCell `json:"cell"`
		Identity       Identity        `json:"identity"`
		EffectiveWorld json.RawMessage `json:"effective_world"`
	}{plan.SchemaVersion, plan.Cell, plan.Identity, plan.EffectiveWorld})
	if err != nil {
		return "", fmt.Errorf("instruction pilot: encode plan: %w", err)
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

func DecodeInstructionPlan(raw []byte) (InstructionLockedPlan, string, error) {
	var plan InstructionLockedPlan
	if err := ValidateStrictJSON(raw); err != nil {
		return plan, "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return plan, "", err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return plan, "", errors.New("instruction pilot: trailing plan content")
	}
	if plan.SchemaVersion != InstructionPlanSchemaVersion || len(plan.EffectiveWorld) == 0 || plan.TypedPlanSHA256 == "" {
		return plan, "", errors.New("instruction pilot: incomplete plan")
	}
	digest := sha256.Sum256(raw)
	return plan, hex.EncodeToString(digest[:]), nil
}
