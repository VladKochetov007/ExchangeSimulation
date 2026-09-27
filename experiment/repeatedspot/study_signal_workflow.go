package repeatedspot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"exchange_sim/experiment/executionpilot"
	"exchange_sim/simulation"
)

type ME016RunManifest struct {
	SchemaVersion      int                     `json:"schema_version"`
	Cell               ME016Cell               `json:"cell"`
	PlanRawSHA256      string                  `json:"plan_raw_sha256"`
	TypedPlanSHA256    string                  `json:"typed_plan_sha256"`
	Identity           executionpilot.Identity `json:"identity"`
	ContractSHA256     string                  `json:"contract_sha256"`
	Evidence           EvidenceIdentity        `json:"evidence"`
	EvidenceFileSHA256 string                  `json:"evidence_file_sha256"`
	SidecarSHA256      map[string]string       `json:"sidecar_sha256"`
}

type ME016AnalyzedResult struct {
	SchemaVersion          int                     `json:"schema_version"`
	Cell                   ME016Cell               `json:"cell"`
	Identity               executionpilot.Identity `json:"identity"`
	ManifestFileSHA256     string                  `json:"manifest_file_sha256"`
	EvidenceFileSHA256     string                  `json:"evidence_file_sha256"`
	EconomicReconstruction *EconomicReplay         `json:"economic_reconstruction"`
}

func WriteME016Plan(repositoryDir, analyzerBinary, path string, cell ME016Cell) error {
	if err := requireE0ExternalOutput(repositoryDir, path); err != nil {
		return err
	}
	identity, err := executionpilot.RuntimeIdentity(repositoryDir, analyzerBinary, SignalEvidenceSchemaID)
	if err != nil {
		return err
	}
	plan, err := LockME016Plan(cell, identity)
	if err != nil {
		return err
	}
	return writeE0ExclusiveJSON(path, plan)
}

func readME016Plan(path string) (ME016LockedPlan, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ME016LockedPlan{}, "", err
	}
	return DecodeME016Plan(raw)
}

func RunME016Plan(ctx context.Context, repositoryDir, analyzerBinary, planPath, outputDir string) (ME016RunManifest, error) {
	if err := requireE0ExternalOutput(repositoryDir, outputDir); err != nil {
		return ME016RunManifest{}, err
	}
	plan, rawDigest, err := readME016Plan(planPath)
	if err != nil {
		return ME016RunManifest{}, err
	}
	identity, err := executionpilot.RuntimeIdentity(repositoryDir, analyzerBinary, SignalEvidenceSchemaID)
	if err != nil {
		return ME016RunManifest{}, err
	}
	world, err := VerifyME016Plan(plan, identity)
	if err != nil {
		return ME016RunManifest{}, err
	}
	defer world.Close()
	if err := os.Mkdir(outputDir, 0700); err != nil {
		return ME016RunManifest{}, fmt.Errorf("repeated spot: fresh ME-016 run directory required: %w", err)
	}
	receipts, err := simulation.NewMarketDataReceiptRecorder(outputDir)
	if err != nil {
		return ME016RunManifest{}, err
	}
	evidencePath := filepath.Join(outputDir, e0EvidenceFilename)
	evidenceFile, err := os.OpenFile(evidencePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return ME016RunManifest{}, err
	}
	capture, err := NewCapture(world, evidenceFile, receipts)
	if err != nil {
		_ = evidenceFile.Close()
		return ME016RunManifest{}, err
	}
	evidenceIdentity, runErr := capture.Run(ctx)
	syncErr := evidenceFile.Sync()
	closeErr := evidenceFile.Close()
	if err := errors.Join(runErr, syncErr, closeErr); err != nil {
		return ME016RunManifest{}, fmt.Errorf("repeated spot: incomplete ME-016 world; retain failed attempt: %w", err)
	}
	evidenceDigest, err := hashE0File(evidencePath)
	if err != nil {
		return ME016RunManifest{}, err
	}
	sidecars, err := hashE0Sidecars(outputDir)
	if err != nil {
		return ME016RunManifest{}, err
	}
	manifest := ME016RunManifest{SchemaVersion: 1, Cell: plan.Cell, PlanRawSHA256: rawDigest,
		TypedPlanSHA256: plan.TypedPlanSHA256, Identity: identity,
		ContractSHA256: world.ContractSHA256(), Evidence: evidenceIdentity,
		EvidenceFileSHA256: evidenceDigest, SidecarSHA256: sidecars}
	if err := writeE0ExclusiveJSON(filepath.Join(outputDir, e0ManifestFilename), manifest); err != nil {
		return ME016RunManifest{}, err
	}
	return manifest, nil
}

func AnalyzeME016Run(repositoryDir, simulatorBinary, planPath, runDir, resultPath string) (ME016AnalyzedResult, error) {
	if err := requireE0ExternalOutput(repositoryDir, resultPath); err != nil {
		return ME016AnalyzedResult{}, err
	}
	plan, rawDigest, err := readME016Plan(planPath)
	if err != nil {
		return ME016AnalyzedResult{}, err
	}
	analyzerBinary, err := os.Executable()
	if err != nil {
		return ME016AnalyzedResult{}, err
	}
	identity, err := executionpilot.ToolIdentity(repositoryDir, simulatorBinary, analyzerBinary, SignalEvidenceSchemaID)
	if err != nil {
		return ME016AnalyzedResult{}, err
	}
	world, err := VerifyME016Plan(plan, identity)
	if err != nil {
		return ME016AnalyzedResult{}, err
	}
	world.Close()
	manifestPath := filepath.Join(runDir, e0ManifestFilename)
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		return ME016AnalyzedResult{}, err
	}
	var manifest ME016RunManifest
	if err := decodeE0Strict(manifestRaw, &manifest); err != nil {
		return ME016AnalyzedResult{}, err
	}
	if err := verifyME016Manifest(plan, rawDigest, identity, manifest); err != nil {
		return ME016AnalyzedResult{}, err
	}
	evidencePath := filepath.Join(runDir, e0EvidenceFilename)
	evidenceDigest, err := hashE0File(evidencePath)
	if err != nil || evidenceDigest != manifest.EvidenceFileSHA256 {
		return ME016AnalyzedResult{}, errors.New("repeated spot: ME-016 evidence file digest mismatch")
	}
	sidecars, err := hashE0Sidecars(runDir)
	if err != nil {
		return ME016AnalyzedResult{}, err
	}
	for name, digest := range sidecars {
		if manifest.SidecarSHA256[name] != digest {
			return ME016AnalyzedResult{}, fmt.Errorf("repeated spot: ME-016 sidecar %s digest mismatch", name)
		}
	}
	evidenceFile, err := os.Open(evidencePath)
	if err != nil {
		return ME016AnalyzedResult{}, err
	}
	defer evidenceFile.Close()
	reconstruction, err := ReplayWindow(plan.EffectiveWorld, evidenceFile, manifest.Evidence, runDir, plan.Window)
	if err != nil {
		return ME016AnalyzedResult{}, err
	}
	if reconstruction == nil || reconstruction.SignalAudit == nil {
		return ME016AnalyzedResult{}, errors.New("repeated spot: incomplete ME-016 signal reconstruction")
	}
	manifestDigest := sha256.Sum256(manifestRaw)
	result := ME016AnalyzedResult{SchemaVersion: 1, Cell: plan.Cell, Identity: identity,
		ManifestFileSHA256: hex.EncodeToString(manifestDigest[:]), EvidenceFileSHA256: evidenceDigest,
		EconomicReconstruction: reconstruction}
	if err := writeE0ExclusiveJSON(resultPath, result); err != nil {
		return ME016AnalyzedResult{}, err
	}
	return result, nil
}

func verifyME016Manifest(plan ME016LockedPlan, rawDigest string, identity executionpilot.Identity, manifest ME016RunManifest) error {
	contractHash := sha256.Sum256(plan.EffectiveWorld)
	if manifest.SchemaVersion != 1 || manifest.Cell != plan.Cell || manifest.Identity != identity ||
		manifest.PlanRawSHA256 != rawDigest || manifest.TypedPlanSHA256 != plan.TypedPlanSHA256 ||
		manifest.ContractSHA256 != hex.EncodeToString(contractHash[:]) ||
		manifest.Evidence.SchemaID != SignalEvidenceSchemaID || len(manifest.SidecarSHA256) != len(e0Sidecars) {
		return errors.New("repeated spot: ME-016 manifest differs from locked plan")
	}
	for _, name := range e0Sidecars {
		if manifest.SidecarSHA256[name] == "" {
			return fmt.Errorf("repeated spot: ME-016 manifest omitted %s", name)
		}
	}
	return nil
}
