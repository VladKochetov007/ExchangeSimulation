package repeatedspot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"exchange_sim/experiment/executionpilot"
	"exchange_sim/simulation"
)

const (
	e0EvidenceFilename = "evidence.evs"
	e0ManifestFilename = "run-manifest.json"
)

var e0Sidecars = [...]string{
	"market-data-evidence-v2.json", "market-data-schedules-v2.bin",
	"market-data-receipts-v2.bin", "market-data-decisions-v2.bin",
}

type E0RunManifest struct {
	SchemaVersion      int                     `json:"schema_version"`
	Cell               E0Cell                  `json:"cell"`
	PlanRawSHA256      string                  `json:"plan_raw_sha256"`
	TypedPlanSHA256    string                  `json:"typed_plan_sha256"`
	Identity           executionpilot.Identity `json:"identity"`
	ContractSHA256     string                  `json:"contract_sha256"`
	Evidence           EvidenceIdentity        `json:"evidence"`
	EvidenceFileSHA256 string                  `json:"evidence_file_sha256"`
	SidecarSHA256      map[string]string       `json:"sidecar_sha256"`
}

type E0AnalyzedResult struct {
	SchemaVersion          int                     `json:"schema_version"`
	Cell                   E0Cell                  `json:"cell"`
	Identity               executionpilot.Identity `json:"identity"`
	ManifestFileSHA256     string                  `json:"manifest_file_sha256"`
	EvidenceFileSHA256     string                  `json:"evidence_file_sha256"`
	EconomicReconstruction *EconomicReplay         `json:"economic_reconstruction"`
}

func WriteE0Plan(repositoryDir, analyzerBinary, path string, cell E0Cell) error {
	if err := requireE0ExternalOutput(repositoryDir, path); err != nil {
		return err
	}
	identity, err := executionpilot.RuntimeIdentity(repositoryDir, analyzerBinary, EvidenceSchemaID)
	if err != nil {
		return err
	}
	plan, err := LockE0Plan(cell, identity)
	if err != nil {
		return err
	}
	return writeE0ExclusiveJSON(path, plan)
}

func readE0Plan(path string) (E0LockedPlan, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return E0LockedPlan{}, "", err
	}
	return DecodeE0Plan(raw)
}

func RunE0Plan(ctx context.Context, repositoryDir, analyzerBinary, planPath, outputDir string) (E0RunManifest, error) {
	if err := requireE0ExternalOutput(repositoryDir, outputDir); err != nil {
		return E0RunManifest{}, err
	}
	plan, rawDigest, err := readE0Plan(planPath)
	if err != nil {
		return E0RunManifest{}, err
	}
	identity, err := executionpilot.RuntimeIdentity(repositoryDir, analyzerBinary, EvidenceSchemaID)
	if err != nil {
		return E0RunManifest{}, err
	}
	world, err := VerifyE0Plan(plan, identity)
	if err != nil {
		return E0RunManifest{}, err
	}
	defer world.Close()
	if err := os.Mkdir(outputDir, 0700); err != nil {
		return E0RunManifest{}, fmt.Errorf("repeated spot: fresh run directory required: %w", err)
	}
	receipts, err := simulation.NewMarketDataReceiptRecorder(outputDir)
	if err != nil {
		return E0RunManifest{}, err
	}
	evidencePath := filepath.Join(outputDir, e0EvidenceFilename)
	evidenceFile, err := os.OpenFile(evidencePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return E0RunManifest{}, err
	}
	capture, err := NewCapture(world, evidenceFile, receipts)
	if err != nil {
		_ = evidenceFile.Close()
		return E0RunManifest{}, err
	}
	evidenceIdentity, runErr := capture.Run(ctx)
	syncErr := evidenceFile.Sync()
	closeErr := evidenceFile.Close()
	if err := errors.Join(runErr, syncErr, closeErr); err != nil {
		return E0RunManifest{}, fmt.Errorf("repeated spot: incomplete E0 world; retain failed attempt: %w", err)
	}
	evidenceDigest, err := hashE0File(evidencePath)
	if err != nil {
		return E0RunManifest{}, err
	}
	sidecars, err := hashE0Sidecars(outputDir)
	if err != nil {
		return E0RunManifest{}, err
	}
	manifest := E0RunManifest{SchemaVersion: 1, Cell: plan.Cell, PlanRawSHA256: rawDigest,
		TypedPlanSHA256: plan.TypedPlanSHA256, Identity: identity, ContractSHA256: world.ContractSHA256(),
		Evidence: evidenceIdentity, EvidenceFileSHA256: evidenceDigest, SidecarSHA256: sidecars}
	if err := writeE0ExclusiveJSON(filepath.Join(outputDir, e0ManifestFilename), manifest); err != nil {
		return E0RunManifest{}, err
	}
	return manifest, nil
}

func AnalyzeE0Run(repositoryDir, simulatorBinary, planPath, runDir, resultPath string) (E0AnalyzedResult, error) {
	if err := requireE0ExternalOutput(repositoryDir, resultPath); err != nil {
		return E0AnalyzedResult{}, err
	}
	plan, rawDigest, err := readE0Plan(planPath)
	if err != nil {
		return E0AnalyzedResult{}, err
	}
	analyzerBinary, err := os.Executable()
	if err != nil {
		return E0AnalyzedResult{}, err
	}
	identity, err := executionpilot.ToolIdentity(repositoryDir, simulatorBinary, analyzerBinary, EvidenceSchemaID)
	if err != nil {
		return E0AnalyzedResult{}, err
	}
	world, err := VerifyE0Plan(plan, identity)
	if err != nil {
		return E0AnalyzedResult{}, err
	}
	world.Close()
	manifestPath := filepath.Join(runDir, e0ManifestFilename)
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		return E0AnalyzedResult{}, err
	}
	var manifest E0RunManifest
	if err := decodeE0Strict(manifestRaw, &manifest); err != nil {
		return E0AnalyzedResult{}, err
	}
	if err := verifyE0Manifest(plan, rawDigest, identity, manifest); err != nil {
		return E0AnalyzedResult{}, err
	}
	evidencePath := filepath.Join(runDir, e0EvidenceFilename)
	evidenceDigest, err := hashE0File(evidencePath)
	if err != nil || evidenceDigest != manifest.EvidenceFileSHA256 {
		return E0AnalyzedResult{}, errors.New("repeated spot: E0 evidence file digest mismatch")
	}
	sidecars, err := hashE0Sidecars(runDir)
	if err != nil {
		return E0AnalyzedResult{}, err
	}
	for name, digest := range sidecars {
		if manifest.SidecarSHA256[name] != digest {
			return E0AnalyzedResult{}, fmt.Errorf("repeated spot: E0 sidecar %s digest mismatch", name)
		}
	}
	evidenceFile, err := os.Open(evidencePath)
	if err != nil {
		return E0AnalyzedResult{}, err
	}
	defer evidenceFile.Close()
	reconstruction, err := ReplayWindow(plan.EffectiveWorld, evidenceFile, manifest.Evidence, runDir, plan.Window)
	if err != nil {
		return E0AnalyzedResult{}, err
	}
	manifestDigest := sha256.Sum256(manifestRaw)
	result := E0AnalyzedResult{SchemaVersion: 1, Cell: plan.Cell, Identity: identity,
		ManifestFileSHA256: hex.EncodeToString(manifestDigest[:]), EvidenceFileSHA256: evidenceDigest,
		EconomicReconstruction: reconstruction}
	if err := writeE0ExclusiveJSON(resultPath, result); err != nil {
		return E0AnalyzedResult{}, err
	}
	return result, nil
}

func verifyE0Manifest(plan E0LockedPlan, rawDigest string, identity executionpilot.Identity, manifest E0RunManifest) error {
	contractHash := sha256.Sum256(plan.EffectiveWorld)
	if manifest.SchemaVersion != 1 || manifest.Cell != plan.Cell || manifest.Identity != identity ||
		manifest.PlanRawSHA256 != rawDigest || manifest.TypedPlanSHA256 != plan.TypedPlanSHA256 ||
		manifest.ContractSHA256 != hex.EncodeToString(contractHash[:]) ||
		manifest.Evidence.SchemaID != EvidenceSchemaID || len(manifest.SidecarSHA256) != len(e0Sidecars) {
		return errors.New("repeated spot: E0 manifest differs from locked plan")
	}
	for _, name := range e0Sidecars {
		if manifest.SidecarSHA256[name] == "" {
			return fmt.Errorf("repeated spot: E0 manifest omitted %s", name)
		}
	}
	return nil
}

func hashE0Sidecars(directory string) (map[string]string, error) {
	digests := make(map[string]string, len(e0Sidecars))
	for _, name := range e0Sidecars {
		digest, err := hashE0File(filepath.Join(directory, name))
		if err != nil {
			return nil, fmt.Errorf("repeated spot: hash %s: %w", name, err)
		}
		digests[name] = digest
	}
	return digests, nil
}

func hashE0File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func decodeE0Strict(raw []byte, value any) error {
	if err := executionpilot.ValidateStrictJSON(raw); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("repeated spot: trailing JSON content")
	}
	return nil
}

func writeE0ExclusiveJSON(path string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func requireE0ExternalOutput(repositoryDir, outputPath string) error {
	repository, err := filepath.EvalSymlinks(repositoryDir)
	if err != nil {
		return err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(outputPath))
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(repository, parent)
	if err != nil {
		return err
	}
	if relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("repeated spot: plans, runs and analyses must be outside the clean candidate checkout")
	}
	return nil
}
