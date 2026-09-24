package executionpilot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
)

type CellResult struct {
	ID                    string               `json:"id"`
	Cell                  Cell                 `json:"cell"`
	PlanRawSHA256         string               `json:"plan_raw_sha256"`
	TypedPlanSHA256       string               `json:"typed_plan_sha256"`
	EvidenceExecutionHash string               `json:"evidence_execution_hash"`
	EvidenceFileSHA256    string               `json:"evidence_file_sha256"`
	EvidenceBytes         int64                `json:"evidence_bytes"`
	Classification        string               `json:"classification"`
	Admissibility         string               `json:"admissibility"`
	Outcome               ReconstructedOutcome `json:"outcome"`
}

type GroupResult struct {
	Arm                string `json:"arm"`
	TargetQty          int64  `json:"target_qty"`
	AdmissibleCount    int    `json:"admissible_count"`
	FailedMandateCount int    `json:"failed_mandate_count"`
	NotAssessableCount int    `json:"not_assessable_count"`
	NoOpportunityCount int    `json:"no_opportunity_count"`
}

type PairedContrast struct {
	Arm                          string    `json:"arm"`
	TargetQty                    int64     `json:"target_qty"`
	Seeds                        []int64   `json:"seeds"`
	DepthDefined                 bool      `json:"depth_defined"`
	DepthDifferences             []int64   `json:"depth_differences,omitempty"`
	MedianDepthDifference        int64     `json:"median_depth_difference,omitempty"`
	OpportunitySeparated         bool      `json:"opportunity_separated"`
	ShortfallDefined             bool      `json:"shortfall_defined"`
	ShortfallDifferencesBps      []float64 `json:"shortfall_differences_bps,omitempty"`
	MedianShortfallDifferenceBps float64   `json:"median_shortfall_difference_bps,omitempty"`
	MinShortfallDifferenceBps    float64   `json:"min_shortfall_difference_bps,omitempty"`
	MaxShortfallDifferenceBps    float64   `json:"max_shortfall_difference_bps,omitempty"`
}

type ResponseSurface struct {
	SchemaVersion      int              `json:"schema_version"`
	ExecutionIdentity  Identity         `json:"execution_identity"`
	AggregationCommit  string           `json:"aggregation_commit"`
	AssignedWorlds     int              `json:"assigned_worlds"`
	TotalEvidenceBytes int64            `json:"total_evidence_bytes"`
	Cells              []CellResult     `json:"cells"`
	Groups             []GroupResult    `json:"groups"`
	Contrasts          []PairedContrast `json:"contrasts"`
}

func cellID(cell Cell) (string, error) {
	arm := ""
	switch cell.MakerCount {
	case 4:
		arm = "C0"
	case 6:
		arm = "Cp"
	case 2:
		arm = "Cm"
	default:
		return "", errors.New("execution pilot: unregistered maker count")
	}
	scale := ""
	switch cell.TargetQty {
	case 50_000_000:
		scale = "S1"
	case 200_000_000:
		scale = "S2"
	case 500_000_000:
		scale = "S3"
	default:
		return "", errors.New("execution pilot: unregistered target quantity")
	}
	return fmt.Sprintf("%s-%s-%d", arm, scale, cell.Seed), nil
}

func decodeStrictFile[T any](path string) (T, error) {
	var value T
	raw, err := os.ReadFile(path)
	if err != nil {
		return value, err
	}
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return value, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return value, errors.New("execution pilot: trailing surface input content")
	}
	return value, nil
}

func regularFileSize(path string) (int64, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() {
		return 0, fmt.Errorf("execution pilot: non-regular surface input %s", path)
	}
	return info.Size(), nil
}

func loadCellResult(root string, expected Cell, identity Identity) (CellResult, error) {
	id, err := cellID(expected)
	if err != nil {
		return CellResult{}, err
	}
	planPath := filepath.Join(root, "plans", id+".json")
	runDir := filepath.Join(root, "runs", id)
	resultPath := filepath.Join(root, "analysis", id+".json")
	for _, path := range []string{planPath, filepath.Join(runDir, ManifestFilename), resultPath,
		filepath.Join(runDir, EvidenceFilename), filepath.Join(runDir, ActorFilename)} {
		if _, err := regularFileSize(path); err != nil {
			return CellResult{}, fmt.Errorf("%s: %w", id, err)
		}
	}
	plan, rawDigest, err := readLockedPlan(planPath)
	if err != nil {
		return CellResult{}, err
	}
	if plan.Cell != expected || plan.Identity != identity {
		return CellResult{}, fmt.Errorf("%s: plan cell/source identity mismatch", id)
	}
	if _, err := Verify(plan, identity); err != nil {
		return CellResult{}, fmt.Errorf("%s: %w", id, err)
	}
	manifest, err := decodeStrictFile[RunManifest](filepath.Join(runDir, ManifestFilename))
	if err != nil {
		return CellResult{}, err
	}
	if manifest.SchemaVersion != 1 || manifest.PlanRawSHA256 != rawDigest ||
		manifest.TypedPlanSHA256 != plan.TypedPlanSHA256 || manifest.Identity != identity ||
		manifest.Evidence.SchemaID != EvidenceSchemaID {
		return CellResult{}, fmt.Errorf("%s: run manifest/plan mismatch", id)
	}
	evidencePath := filepath.Join(runDir, EvidenceFilename)
	evidenceDigest, err := fileSHA256(evidencePath)
	if err != nil || evidenceDigest != manifest.EvidenceFileSHA256 {
		return CellResult{}, fmt.Errorf("%s: evidence file hash mismatch", id)
	}
	actorPath := filepath.Join(runDir, ActorFilename)
	actorDigest, err := fileSHA256(actorPath)
	if err != nil || actorDigest != manifest.ActorFileSHA256 {
		return CellResult{}, fmt.Errorf("%s: actor report file hash mismatch", id)
	}
	result, err := decodeStrictFile[struct {
		SchemaVersion       int                  `json:"schema_version"`
		ManifestTypedSHA256 string               `json:"manifest_typed_sha256"`
		Outcome             ReconstructedOutcome `json:"outcome"`
	}](resultPath)
	if err != nil {
		return CellResult{}, err
	}
	manifestRaw, err := json.Marshal(manifest)
	if err != nil {
		return CellResult{}, err
	}
	manifestHash := sha256.Sum256(manifestRaw)
	if result.SchemaVersion != 1 || result.ManifestTypedSHA256 != hex.EncodeToString(manifestHash[:]) {
		return CellResult{}, fmt.Errorf("%s: reconstructed result/manifest mismatch", id)
	}
	evidenceFile, err := os.Open(evidencePath)
	if err != nil {
		return CellResult{}, err
	}
	reconstructed, reconstructionErr := Reconstruct(evidenceFile, manifest.Evidence, plan)
	closeErr := evidenceFile.Close()
	if reconstructionErr != nil || closeErr != nil {
		return CellResult{}, fmt.Errorf("%s: independent evidence replay: %v / close: %v", id, reconstructionErr, closeErr)
	}
	if reconstructed != result.Outcome {
		return CellResult{}, fmt.Errorf("%s: stored outcome differs from evidence replay", id)
	}
	actorRaw, err := os.ReadFile(actorPath)
	if err != nil {
		return CellResult{}, err
	}
	if err := rejectDuplicateJSONKeys(actorRaw); err != nil {
		return CellResult{}, err
	}
	var actorReport actorSummary
	if err := json.Unmarshal(actorRaw, &actorReport); err != nil {
		return CellResult{}, err
	}
	if err := compareActorReport(reconstructed, actorReport); err != nil {
		return CellResult{}, err
	}
	evidenceBytes, err := regularFileSize(evidencePath)
	if err != nil {
		return CellResult{}, err
	}
	classification, admissibility := classifyOutcome(reconstructed)
	return CellResult{
		ID: id, Cell: expected, PlanRawSHA256: rawDigest,
		TypedPlanSHA256:       plan.TypedPlanSHA256,
		EvidenceExecutionHash: manifest.Evidence.ExecutionHash,
		EvidenceFileSHA256:    evidenceDigest, EvidenceBytes: evidenceBytes,
		Classification: classification, Admissibility: admissibility, Outcome: reconstructed,
	}, nil
}

func classifyOutcome(outcome ReconstructedOutcome) (string, string) {
	if outcome.Status == OutcomeNoObservedOpportunity {
		return "NO_OPPORTUNITY", "NOT_ASSESSABLE"
	}
	if !outcome.TargetShortfallDefined {
		return "VALID_ACTIVE", "NOT_ASSESSABLE"
	}
	if outcome.Status == OutcomeFullyFilled && outcome.TargetShortfallBps <= 10 {
		return "VALID_ACTIVE", "PASS"
	}
	return "ECONOMIC_FAILURE", "FAIL"
}

func AggregateRuns(root, repositoryDir, expectedSource string, seeds []int64) (ResponseSurface, error) {
	if !hexDigest(expectedSource, 20) {
		return ResponseSurface{}, errors.New("execution pilot: invalid expected source commit")
	}
	aggregationCommit, err := gitValue(repositoryDir, "rev-parse", "HEAD")
	if err != nil {
		return ResponseSurface{}, err
	}
	status, err := gitValue(repositoryDir, "status", "--porcelain")
	if err != nil || status != "" {
		return ResponseSurface{}, errors.New("execution pilot: aggregate from a clean source checkout")
	}
	cells, err := Cells(seeds)
	if err != nil {
		return ResponseSurface{}, err
	}
	var surface ResponseSurface
	surface.SchemaVersion = 1
	surface.AggregationCommit = aggregationCommit
	surface.AssignedWorlds = len(cells)
	for index, cell := range cells {
		id, err := cellID(cell)
		if err != nil {
			return ResponseSurface{}, err
		}
		plan, _, err := readLockedPlan(filepath.Join(root, "plans", id+".json"))
		if err != nil {
			return ResponseSurface{}, err
		}
		if index == 0 {
			surface.ExecutionIdentity = plan.Identity
			if surface.ExecutionIdentity.SourceCommit != expectedSource {
				return ResponseSurface{}, errors.New("execution pilot: source commit differs from registered execution")
			}
		}
		result, err := loadCellResult(root, cell, surface.ExecutionIdentity)
		if err != nil {
			return ResponseSurface{}, err
		}
		surface.Cells = append(surface.Cells, result)
		surface.TotalEvidenceBytes += result.EvidenceBytes
	}
	surface.Groups, surface.Contrasts, err = summarizeCells(surface.Cells, seeds)
	return surface, err
}

func summarizeCells(cells []CellResult, seeds []int64) ([]GroupResult, []PairedContrast, error) {
	if len(cells) != 27 || len(seeds) != 3 {
		return nil, nil, errors.New("execution pilot: incomplete registered response surface")
	}
	byID := make(map[string]CellResult, len(cells))
	for _, cell := range cells {
		if byID[cell.ID].ID != "" {
			return nil, nil, errors.New("execution pilot: duplicate response surface cell")
		}
		byID[cell.ID] = cell
	}
	var groups []GroupResult
	var contrasts []PairedContrast
	for _, target := range []int64{50_000_000, 200_000_000, 500_000_000} {
		for _, arm := range []string{"C0", "Cp", "Cm"} {
			group := GroupResult{Arm: arm, TargetQty: target}
			for _, seed := range seeds {
				id, err := cellID(Cell{MakerCount: map[string]int{"C0": 4, "Cp": 6, "Cm": 2}[arm], TargetQty: target, Seed: seed})
				if err != nil {
					return nil, nil, err
				}
				cell, present := byID[id]
				if !present || cell.Cell.TargetQty != target || cell.Cell.Seed != seed {
					return nil, nil, fmt.Errorf("execution pilot: missing registered cell %s", id)
				}
				switch cell.Admissibility {
				case "PASS":
					group.AdmissibleCount++
				case "FAIL":
					group.FailedMandateCount++
				case "NOT_ASSESSABLE":
					group.NotAssessableCount++
				default:
					return nil, nil, fmt.Errorf("execution pilot: unknown admissibility for %s", id)
				}
				if cell.Classification == "NO_OPPORTUNITY" {
					group.NoOpportunityCount++
				}
			}
			groups = append(groups, group)
		}
		for _, arm := range []string{"Cp", "Cm"} {
			contrast := PairedContrast{Arm: arm, TargetQty: target, Seeds: slices.Clone(seeds), DepthDefined: true, ShortfallDefined: true}
			for _, seed := range seeds {
				baselineID, _ := cellID(Cell{MakerCount: 4, TargetQty: target, Seed: seed})
				makerCount := 6
				if arm == "Cm" {
					makerCount = 2
				}
				treatmentID, _ := cellID(Cell{MakerCount: makerCount, TargetQty: target, Seed: seed})
				baseline, treatment := byID[baselineID], byID[treatmentID]
				if baseline.Classification == "NO_OPPORTUNITY" || treatment.Classification == "NO_OPPORTUNITY" {
					contrast.DepthDefined = false
				} else {
					contrast.DepthDifferences = append(contrast.DepthDifferences,
						treatment.Outcome.DeliveredFiveAskQty-baseline.Outcome.DeliveredFiveAskQty)
				}
				if !baseline.Outcome.TargetShortfallDefined || !treatment.Outcome.TargetShortfallDefined {
					contrast.ShortfallDefined = false
				} else {
					contrast.ShortfallDifferencesBps = append(contrast.ShortfallDifferencesBps,
						treatment.Outcome.TargetShortfallBps-baseline.Outcome.TargetShortfallBps)
				}
			}
			if contrast.DepthDefined {
				depth := slices.Clone(contrast.DepthDifferences)
				slices.Sort(depth)
				contrast.MedianDepthDifference = depth[1]
				contrast.OpportunitySeparated = arm == "Cp" && depth[1] > 0 || arm == "Cm" && depth[1] < 0
			} else {
				contrast.DepthDifferences = nil
			}
			if contrast.ShortfallDefined {
				shortfall := slices.Clone(contrast.ShortfallDifferencesBps)
				slices.Sort(shortfall)
				contrast.MinShortfallDifferenceBps = shortfall[0]
				contrast.MedianShortfallDifferenceBps = shortfall[1]
				contrast.MaxShortfallDifferenceBps = shortfall[2]
			} else {
				contrast.ShortfallDifferencesBps = nil
			}
			contrasts = append(contrasts, contrast)
		}
	}
	return groups, contrasts, nil
}

func WriteSurface(path string, surface ResponseSurface) error {
	return writeExclusiveJSON(path, surface)
}
