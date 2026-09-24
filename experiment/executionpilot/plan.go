package executionpilot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"exchange_sim/simulations/executionlab"
)

const PlanSchemaVersion = 1
const RequiredToolchain = "go1.27.0"

type Cell struct {
	MakerCount       int   `json:"maker_count"`
	RandomTakerCount int   `json:"random_taker_count"`
	TargetQty        int64 `json:"target_qty"`
	Seed             int64 `json:"seed"`
}

type Identity struct {
	SourceCommit     string `json:"source_commit"`
	SourceTree       string `json:"source_tree"`
	SimulatorSHA256  string `json:"simulator_sha256"`
	AnalyzerSHA256   string `json:"analyzer_sha256"`
	Toolchain        string `json:"toolchain"`
	EvidenceSchemaID string `json:"evidence_schema_id"`
}

type LockedPlan struct {
	SchemaVersion   int             `json:"schema_version"`
	Cell            Cell            `json:"cell"`
	Identity        Identity        `json:"identity"`
	EffectiveWorld  json.RawMessage `json:"effective_world"`
	TypedPlanSHA256 string          `json:"typed_plan_sha256"`
}

func ValidateCell(cell Cell) error {
	if cell.MakerCount+cell.RandomTakerCount != 12 ||
		!memberInt(cell.MakerCount, 2, 4, 6) ||
		!memberInt(cell.RandomTakerCount, 10, 8, 6) ||
		!memberInt64(cell.TargetQty, 50_000_000, 200_000_000, 500_000_000) ||
		cell.Seed <= 0 {
		return fmt.Errorf("execution pilot: cell outside the prospective quantity/composition grid")
	}
	return nil
}

func memberInt(value int, choices ...int) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}

func memberInt64(value int64, choices ...int64) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}

func ValidateIdentity(identity Identity) error {
	return ValidateIdentityForSchema(identity, EvidenceSchemaID)
}

func ValidateIdentityForSchema(identity Identity, schemaID string) error {
	if !hexDigest(identity.SourceCommit, 20) || !hexDigest(identity.SourceTree, 20) ||
		!hexDigest(identity.SimulatorSHA256, 32) || !hexDigest(identity.AnalyzerSHA256, 32) ||
		identity.Toolchain != RequiredToolchain || identity.EvidenceSchemaID != schemaID {
		return fmt.Errorf("execution pilot: incomplete source, binary, toolchain or evidence identity")
	}
	return nil
}

func hexDigest(value string, bytes int) bool {
	if len(value) != bytes*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == bytes && value == strings.ToLower(value)
}

func NewWorld(cell Cell) (*executionlab.Sim, error) {
	if err := ValidateCell(cell); err != nil {
		return nil, err
	}
	config := executionlab.DefaultSimConfig(executionlab.Immediate)
	config.Seed = cell.Seed
	config.MMCount = cell.MakerCount
	config.NoiseTraderCount = cell.RandomTakerCount
	config.RecordSnapshotProjectionEvidence = true
	config.Parent.TargetQty = cell.TargetQty
	return executionlab.NewSim(config)
}

func Lock(cell Cell, identity Identity) (LockedPlan, error) {
	if err := ValidateIdentity(identity); err != nil {
		return LockedPlan{}, err
	}
	world, err := NewWorld(cell)
	if err != nil {
		return LockedPlan{}, err
	}
	contract, err := json.Marshal(world.WorldContract())
	if err != nil {
		return LockedPlan{}, fmt.Errorf("encode effective world: %w", err)
	}
	plan := LockedPlan{
		SchemaVersion: PlanSchemaVersion, Cell: cell, Identity: identity,
		EffectiveWorld: contract,
	}
	plan.TypedPlanSHA256, err = planDigest(plan)
	return plan, err
}

func Verify(plan LockedPlan, actual Identity) (*executionlab.Sim, error) {
	if plan.SchemaVersion != PlanSchemaVersion {
		return nil, fmt.Errorf("execution pilot: unsupported plan schema %d", plan.SchemaVersion)
	}
	if err := ValidateIdentity(actual); err != nil {
		return nil, err
	}
	if plan.Identity != actual {
		return nil, errors.New("execution pilot: plan source/tool/binary/evidence identity mismatch")
	}
	world, err := NewWorld(plan.Cell)
	if err != nil {
		return nil, err
	}
	consumed, err := json.Marshal(world.WorldContract())
	if err != nil {
		return nil, fmt.Errorf("encode consumed world: %w", err)
	}
	var supplied bytes.Buffer
	if err := json.Compact(&supplied, plan.EffectiveWorld); err != nil {
		return nil, fmt.Errorf("execution pilot: invalid effective world: %w", err)
	}
	if !bytes.Equal(supplied.Bytes(), consumed) {
		return nil, errors.New("execution pilot: locked effective world differs from constructed world")
	}
	digest, err := planDigest(plan)
	if err != nil {
		return nil, err
	}
	if plan.TypedPlanSHA256 != digest {
		return nil, errors.New("execution pilot: typed plan digest mismatch")
	}
	return world, nil
}

func planDigest(plan LockedPlan) (string, error) {
	canonical, err := json.Marshal(struct {
		SchemaVersion  int             `json:"schema_version"`
		Cell           Cell            `json:"cell"`
		Identity       Identity        `json:"identity"`
		EffectiveWorld json.RawMessage `json:"effective_world"`
	}{plan.SchemaVersion, plan.Cell, plan.Identity, plan.EffectiveWorld})
	if err != nil {
		return "", fmt.Errorf("encode typed plan: %w", err)
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

func DecodePlan(raw []byte) (LockedPlan, string, error) {
	var plan LockedPlan
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return LockedPlan{}, "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return LockedPlan{}, "", fmt.Errorf("decode plan: %w", err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return LockedPlan{}, "", errors.New("execution pilot: trailing plan content")
	}
	if plan.SchemaVersion == 0 || len(plan.EffectiveWorld) == 0 || plan.TypedPlanSHA256 == "" {
		return LockedPlan{}, "", errors.New("execution pilot: missing required plan fields")
	}
	digest := sha256.Sum256(raw)
	return plan, hex.EncodeToString(digest[:]), nil
}

func Cells(seeds []int64) ([]Cell, error) {
	if len(seeds) != 3 || seeds[0] <= 0 || seeds[1] <= 0 || seeds[2] <= 0 ||
		seeds[0] == seeds[1] || seeds[0] == seeds[2] || seeds[1] == seeds[2] {
		return nil, errors.New("execution pilot: exactly three distinct positive development seeds required")
	}
	var cells []Cell
	for _, composition := range [][2]int{{4, 8}, {6, 6}, {2, 10}} {
		for _, target := range []int64{50_000_000, 200_000_000, 500_000_000} {
			for _, seed := range seeds {
				cells = append(cells, Cell{composition[0], composition[1], target, seed})
			}
		}
	}
	return cells, nil
}
