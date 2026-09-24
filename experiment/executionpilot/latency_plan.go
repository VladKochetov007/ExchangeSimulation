package executionpilot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"exchange_sim/simulations/executionlab"
)

const LatencyPlanSchemaVersion = 1

type LatencyCell struct {
	NetworkLatencyNanos  int64 `json:"network_latency_nanos"`
	ProcessingDelayNanos int64 `json:"processing_delay_nanos"`
	TargetQty            int64 `json:"target_qty"`
	Seed                 int64 `json:"seed"`
}

type LatencyLockedPlan struct {
	SchemaVersion   int             `json:"schema_version"`
	Cell            LatencyCell     `json:"cell"`
	Identity        Identity        `json:"identity"`
	EffectiveWorld  json.RawMessage `json:"effective_world"`
	TypedPlanSHA256 string          `json:"typed_plan_sha256"`
}

func ValidateLatencyCell(cell LatencyCell) error {
	if !memberInt64(cell.NetworkLatencyNanos, 1_000_000, 90_000_000) ||
		!memberInt64(cell.ProcessingDelayNanos, 0, 120_000_000) ||
		!memberInt64(cell.TargetQty, 50_000_000, 500_000_000) ||
		!memberInt64(cell.Seed, 12001, 12011, 12017) {
		return errors.New("latency pilot: cell outside prospective ME-002 development matrix")
	}
	return nil
}

func NewLatencyWorld(cell LatencyCell) (*executionlab.Sim, error) {
	if err := ValidateLatencyCell(cell); err != nil {
		return nil, err
	}
	config := executionlab.DefaultSimConfig(executionlab.Immediate)
	config.Seed = cell.Seed
	config.Parent.TargetQty = cell.TargetQty
	config.RecordSnapshotProjectionEvidence = true
	config.ExecutionLatency = 0
	config.ParentDeployment = &executionlab.ParentDeployment{
		MarketDataLatency: time.Duration(cell.NetworkLatencyNanos),
		RequestLatency:    time.Duration(cell.NetworkLatencyNanos),
		ResponseLatency:   time.Duration(cell.NetworkLatencyNanos),
		ProcessingDelay:   time.Duration(cell.ProcessingDelayNanos),
	}
	return executionlab.NewSim(config)
}

func LockLatency(cell LatencyCell, identity Identity) (LatencyLockedPlan, error) {
	if err := ValidateIdentityForSchema(identity, LatencyEvidenceSchemaID); err != nil {
		return LatencyLockedPlan{}, err
	}
	world, err := NewLatencyWorld(cell)
	if err != nil {
		return LatencyLockedPlan{}, err
	}
	effective, err := json.Marshal(world.WorldContract())
	if err != nil {
		return LatencyLockedPlan{}, err
	}
	plan := LatencyLockedPlan{SchemaVersion: LatencyPlanSchemaVersion, Cell: cell, Identity: identity, EffectiveWorld: effective}
	plan.TypedPlanSHA256, err = latencyPlanDigest(plan)
	return plan, err
}

func VerifyLatency(plan LatencyLockedPlan, actual Identity) (*executionlab.Sim, error) {
	if plan.SchemaVersion != LatencyPlanSchemaVersion || plan.Identity != actual {
		return nil, errors.New("latency pilot: plan version or runtime identity mismatch")
	}
	if err := ValidateIdentityForSchema(actual, LatencyEvidenceSchemaID); err != nil {
		return nil, err
	}
	world, err := NewLatencyWorld(plan.Cell)
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
		return nil, errors.New("latency pilot: effective world differs from constructed world")
	}
	digest, err := latencyPlanDigest(plan)
	if err != nil || digest != plan.TypedPlanSHA256 {
		return nil, errors.New("latency pilot: typed plan digest mismatch")
	}
	return world, nil
}

func latencyPlanDigest(plan LatencyLockedPlan) (string, error) {
	canonical, err := json.Marshal(struct {
		SchemaVersion  int             `json:"schema_version"`
		Cell           LatencyCell     `json:"cell"`
		Identity       Identity        `json:"identity"`
		EffectiveWorld json.RawMessage `json:"effective_world"`
	}{plan.SchemaVersion, plan.Cell, plan.Identity, plan.EffectiveWorld})
	if err != nil {
		return "", fmt.Errorf("latency pilot: encode plan: %w", err)
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

func DecodeLatencyPlan(raw []byte) (LatencyLockedPlan, string, error) {
	var plan LatencyLockedPlan
	if err := ValidateStrictJSON(raw); err != nil {
		return plan, "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return plan, "", err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return plan, "", errors.New("latency pilot: trailing plan content")
	}
	if plan.SchemaVersion != LatencyPlanSchemaVersion || len(plan.EffectiveWorld) == 0 || plan.TypedPlanSHA256 == "" {
		return plan, "", errors.New("latency pilot: incomplete plan")
	}
	digest := sha256.Sum256(raw)
	return plan, hex.EncodeToString(digest[:]), nil
}
