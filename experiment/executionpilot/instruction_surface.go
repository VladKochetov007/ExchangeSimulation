package executionpilot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

type InstructionCellResult struct {
	ID                    string               `json:"id"`
	Cell                  InstructionCell      `json:"cell"`
	PlanRawSHA256         string               `json:"plan_raw_sha256"`
	TypedPlanSHA256       string               `json:"typed_plan_sha256"`
	EvidenceExecutionHash string               `json:"evidence_execution_hash"`
	EvidenceFileSHA256    string               `json:"evidence_file_sha256"`
	EvidenceBytes         int64                `json:"evidence_bytes"`
	WallSeconds           float64              `json:"wall_seconds"`
	PeakRSSKiB            int64                `json:"peak_rss_kib"`
	FilledFraction        float64              `json:"filled_fraction"`
	Outcome               ReconstructedOutcome `json:"outcome"`
}

type InstructionPairedContrast struct {
	TargetQty  int64   `json:"target_qty"`
	Seed       int64   `json:"seed"`
	IOC        float64 `json:"ioc_filled_fraction"`
	FOK        float64 `json:"fok_filled_fraction"`
	Difference float64 `json:"ioc_minus_fok"`
}

type InstructionContrastRange struct {
	TargetQty int64   `json:"target_qty"`
	Median    float64 `json:"median"`
	Min       float64 `json:"min"`
	Max       float64 `json:"max"`
}

type InstructionSurface struct {
	SchemaVersion             int                         `json:"schema_version"`
	ExecutionIdentity         Identity                    `json:"execution_identity"`
	AggregationCommit         string                      `json:"aggregation_commit"`
	AssignedWorlds            int                         `json:"assigned_worlds"`
	ValidWorlds               int                         `json:"valid_worlds"`
	TotalEvidenceBytes        int64                       `json:"total_evidence_bytes"`
	TotalWorldWallSeconds     float64                     `json:"total_world_wall_seconds"`
	MaxWorldWallSeconds       float64                     `json:"max_world_wall_seconds"`
	MaxPeakRSSKiB             int64                       `json:"max_peak_rss_kib"`
	ControlEvidenceFileSHA256 string                      `json:"control_evidence_file_sha256"`
	ControlResultFileSHA256   string                      `json:"control_result_file_sha256"`
	Cells                     []InstructionCellResult     `json:"cells"`
	PairedContrasts           []InstructionPairedContrast `json:"paired_contrasts"`
	ContrastRanges            []InstructionContrastRange  `json:"contrast_ranges"`
}

func InstructionCells() []InstructionCell {
	cells := make([]InstructionCell, 0, 12)
	for _, target := range []int64{50_000_000, 500_000_000} {
		for _, timeInForce := range []string{"IOC", "FOK"} {
			for _, seed := range []int64{14001, 14011, 14017} {
				cells = append(cells, InstructionCell{TimeInForce: timeInForce, TargetQty: target, Seed: seed})
			}
		}
	}
	return cells
}

func instructionCellID(cell InstructionCell) (string, error) {
	if err := ValidateInstructionCell(cell); err != nil {
		return "", err
	}
	return fmt.Sprintf("dev-%s-%d-%d", strings.ToLower(cell.TimeInForce), cell.TargetQty, cell.Seed), nil
}

func readInstructionResource(path string) (float64, int64, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}
	var wallText, rssText, exitText string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Elapsed (wall clock) time (h:mm:ss or m:ss): "):
			wallText = strings.TrimPrefix(line, "Elapsed (wall clock) time (h:mm:ss or m:ss): ")
		case strings.HasPrefix(line, "Maximum resident set size (kbytes): "):
			rssText = strings.TrimPrefix(line, "Maximum resident set size (kbytes): ")
		case strings.HasPrefix(line, "Exit status: "):
			exitText = strings.TrimPrefix(line, "Exit status: ")
		}
	}
	if wallText == "" || rssText == "" || exitText != "0" {
		return 0, 0, errors.New("instruction pilot: missing or failed resource measurement")
	}
	parts := strings.Split(wallText, ":")
	if len(parts) != 2 && len(parts) != 3 {
		return 0, 0, errors.New("instruction pilot: invalid elapsed-time format")
	}
	seconds, err := strconv.ParseFloat(parts[len(parts)-1], 64)
	if err != nil || seconds < 0 || seconds >= 60 {
		return 0, 0, errors.New("instruction pilot: invalid elapsed seconds")
	}
	minutes, err := strconv.ParseInt(parts[len(parts)-2], 10, 64)
	if err != nil || minutes < 0 || (len(parts) == 3 && minutes >= 60) {
		return 0, 0, errors.New("instruction pilot: invalid elapsed minutes")
	}
	wall := float64(minutes)*60 + seconds
	if len(parts) == 3 {
		hours, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || hours < 0 {
			return 0, 0, errors.New("instruction pilot: invalid elapsed hours")
		}
		wall += float64(hours) * 3600
	}
	rss, err := strconv.ParseInt(rssText, 10, 64)
	if err != nil || math.IsNaN(wall) || math.IsInf(wall, 0) || wall > 30 || rss <= 0 || rss > 4*1024*1024 {
		return 0, 0, errors.New("instruction pilot: measured world exceeded resource contract")
	}
	return wall, rss, nil
}

func verifyInstructionControls(root string) (string, string, error) {
	var evidenceHash, resultHash, actorHash, manifestHash string
	for index, label := range []string{"control-g1", "control-g7"} {
		evidencePath := filepath.Join(root, label, EvidenceFilename)
		resultPath := filepath.Join(root, label+"-result.json")
		actorPath := filepath.Join(root, label, ActorFilename)
		manifestPath := filepath.Join(root, label, ManifestFilename)
		if _, _, err := readInstructionResource(filepath.Join(root, label+"-resource.txt")); err != nil {
			return "", "", err
		}
		for _, path := range []string{evidencePath, resultPath, actorPath, manifestPath} {
			if _, err := regularFileSize(path); err != nil {
				return "", "", err
			}
		}
		currentEvidence, err := fileSHA256(evidencePath)
		if err != nil {
			return "", "", err
		}
		currentResult, err := fileSHA256(resultPath)
		if err != nil {
			return "", "", err
		}
		currentActor, err := fileSHA256(actorPath)
		if err != nil {
			return "", "", err
		}
		currentManifest, err := fileSHA256(manifestPath)
		if err != nil {
			return "", "", err
		}
		if index == 0 {
			evidenceHash, resultHash = currentEvidence, currentResult
			actorHash, manifestHash = currentActor, currentManifest
		} else if evidenceHash != currentEvidence || resultHash != currentResult ||
			actorHash != currentActor || manifestHash != currentManifest {
			return "", "", errors.New("instruction pilot: fresh-process control mismatch")
		}
	}
	return evidenceHash, resultHash, nil
}

func loadInstructionCell(root string, cell InstructionCell, expectedSource string) (InstructionCellResult, Identity, error) {
	id, err := instructionCellID(cell)
	if err != nil {
		return InstructionCellResult{}, Identity{}, err
	}
	planPath := filepath.Join(root, id+"-plan.json")
	runDir := filepath.Join(root, id)
	resultPath := filepath.Join(root, id+"-result.json")
	for _, path := range []string{planPath, resultPath, filepath.Join(root, id+"-resource.txt"),
		filepath.Join(runDir, ManifestFilename), filepath.Join(runDir, EvidenceFilename), filepath.Join(runDir, ActorFilename)} {
		if _, err := regularFileSize(path); err != nil {
			return InstructionCellResult{}, Identity{}, fmt.Errorf("%s: %w", id, err)
		}
	}
	plan, planRawSHA, err := readInstructionPlan(planPath)
	if err != nil || plan.Cell != cell || plan.Identity.SourceCommit != expectedSource {
		return InstructionCellResult{}, Identity{}, fmt.Errorf("%s: wrong cell or source identity: %w", id, err)
	}
	if _, err := VerifyInstruction(plan, plan.Identity); err != nil {
		return InstructionCellResult{}, Identity{}, err
	}
	manifest, err := decodeStrictFile[RunManifest](filepath.Join(runDir, ManifestFilename))
	if err != nil {
		return InstructionCellResult{}, Identity{}, err
	}
	if manifest.SchemaVersion != 3 || manifest.PlanRawSHA256 != planRawSHA ||
		manifest.TypedPlanSHA256 != plan.TypedPlanSHA256 || manifest.Identity != plan.Identity ||
		manifest.Evidence.SchemaID != InstructionEvidenceSchemaID {
		return InstructionCellResult{}, Identity{}, fmt.Errorf("%s: manifest/plan identity mismatch", id)
	}
	evidencePath := filepath.Join(runDir, EvidenceFilename)
	evidenceSHA, err := fileSHA256(evidencePath)
	if err != nil || evidenceSHA != manifest.EvidenceFileSHA256 {
		return InstructionCellResult{}, Identity{}, fmt.Errorf("%s: evidence digest mismatch", id)
	}
	actorPath := filepath.Join(runDir, ActorFilename)
	actorSHA, err := fileSHA256(actorPath)
	if err != nil || actorSHA != manifest.ActorFileSHA256 {
		return InstructionCellResult{}, Identity{}, fmt.Errorf("%s: actor digest mismatch", id)
	}
	stored, err := decodeStrictFile[struct {
		SchemaVersion       int                  `json:"schema_version"`
		Cell                InstructionCell      `json:"cell"`
		ManifestTypedSHA256 string               `json:"manifest_typed_sha256"`
		Outcome             ReconstructedOutcome `json:"outcome"`
	}](resultPath)
	if err != nil {
		return InstructionCellResult{}, Identity{}, err
	}
	manifestCanonical, err := json.Marshal(manifest)
	if err != nil {
		return InstructionCellResult{}, Identity{}, err
	}
	manifestDigest := sha256.Sum256(manifestCanonical)
	if stored.SchemaVersion != 3 || stored.Cell != cell ||
		stored.ManifestTypedSHA256 != hex.EncodeToString(manifestDigest[:]) {
		return InstructionCellResult{}, Identity{}, fmt.Errorf("%s: result/manifest mismatch", id)
	}
	evidence, err := os.Open(evidencePath)
	if err != nil {
		return InstructionCellResult{}, Identity{}, err
	}
	reconstructed, replayErr := ReconstructInstruction(evidence, manifest.Evidence, plan.EffectiveWorld, cell.TargetQty)
	closeErr := evidence.Close()
	if replayErr != nil || closeErr != nil {
		return InstructionCellResult{}, Identity{}, fmt.Errorf("%s: replay %v / close %v", id, replayErr, closeErr)
	}
	if !reflect.DeepEqual(stored.Outcome, reconstructed) {
		return InstructionCellResult{}, Identity{}, fmt.Errorf("%s: stored result differs from raw replay", id)
	}
	actorRaw, err := os.ReadFile(actorPath)
	if err != nil {
		return InstructionCellResult{}, Identity{}, err
	}
	if err := compareInstructionActorReport(actorRaw, reconstructed); err != nil {
		return InstructionCellResult{}, Identity{}, fmt.Errorf("%s: %w", id, err)
	}
	if reconstructed.FilledQty < 0 || reconstructed.FilledQty > cell.TargetQty ||
		!reconstructed.BalanceSnapshotSeen {
		return InstructionCellResult{}, Identity{}, fmt.Errorf("%s: invalid primary endpoint", id)
	}
	wall, rss, err := readInstructionResource(filepath.Join(root, id+"-resource.txt"))
	if err != nil {
		return InstructionCellResult{}, Identity{}, fmt.Errorf("%s: %w", id, err)
	}
	bytes, err := regularFileSize(evidencePath)
	if err != nil {
		return InstructionCellResult{}, Identity{}, err
	}
	return InstructionCellResult{ID: id, Cell: cell, PlanRawSHA256: planRawSHA,
			TypedPlanSHA256: plan.TypedPlanSHA256, EvidenceExecutionHash: manifest.Evidence.ExecutionHash,
			EvidenceFileSHA256: evidenceSHA, EvidenceBytes: bytes, WallSeconds: wall, PeakRSSKiB: rss,
			FilledFraction: float64(reconstructed.FilledQty) / float64(cell.TargetQty), Outcome: reconstructed},
		plan.Identity, nil
}

func AggregateInstructionRuns(root, repositoryDir, expectedSource string) (InstructionSurface, error) {
	if !hexDigest(expectedSource, 20) {
		return InstructionSurface{}, errors.New("instruction pilot: invalid execution source")
	}
	aggregationCommit, err := gitValue(repositoryDir, "rev-parse", "HEAD")
	if err != nil {
		return InstructionSurface{}, err
	}
	status, err := gitValue(repositoryDir, "status", "--porcelain")
	if err != nil || status != "" {
		return InstructionSurface{}, errors.New("instruction pilot: aggregation checkout must be clean")
	}
	controlEvidence, controlResult, err := verifyInstructionControls(root)
	if err != nil {
		return InstructionSurface{}, err
	}
	surface := InstructionSurface{SchemaVersion: 1, AggregationCommit: aggregationCommit,
		AssignedWorlds: 12, ControlEvidenceFileSHA256: controlEvidence, ControlResultFileSHA256: controlResult}
	for _, cell := range InstructionCells() {
		result, identity, err := loadInstructionCell(root, cell, expectedSource)
		if err != nil {
			return InstructionSurface{}, err
		}
		if surface.ExecutionIdentity == (Identity{}) {
			surface.ExecutionIdentity = identity
		} else if surface.ExecutionIdentity != identity {
			return InstructionSurface{}, errors.New("instruction pilot: mixed execution identities")
		}
		surface.Cells = append(surface.Cells, result)
		surface.TotalEvidenceBytes += result.EvidenceBytes
		surface.TotalWorldWallSeconds += result.WallSeconds
		surface.MaxWorldWallSeconds = max(surface.MaxWorldWallSeconds, result.WallSeconds)
		surface.MaxPeakRSSKiB = max(surface.MaxPeakRSSKiB, result.PeakRSSKiB)
	}
	if len(surface.Cells) != 12 || surface.TotalEvidenceBytes > 1<<30 ||
		surface.TotalWorldWallSeconds > 900 {
		return InstructionSurface{}, errors.New("instruction pilot: incomplete or over-budget development matrix")
	}
	surface.ValidWorlds = len(surface.Cells)
	for _, target := range []int64{50_000_000, 500_000_000} {
		var differences []float64
		for _, seed := range []int64{14001, 14011, 14017} {
			values := map[string]float64{}
			for _, cell := range surface.Cells {
				if cell.Cell.TargetQty == target && cell.Cell.Seed == seed {
					if _, present := values[cell.Cell.TimeInForce]; present {
						return InstructionSurface{}, errors.New("instruction pilot: duplicate matched arm")
					}
					values[cell.Cell.TimeInForce] = cell.FilledFraction
				}
			}
			if len(values) != 2 {
				return InstructionSurface{}, errors.New("instruction pilot: missing matched arm")
			}
			difference := values["IOC"] - values["FOK"]
			surface.PairedContrasts = append(surface.PairedContrasts, InstructionPairedContrast{
				TargetQty: target, Seed: seed, IOC: values["IOC"], FOK: values["FOK"], Difference: difference})
			differences = append(differences, difference)
		}
		slices.Sort(differences)
		surface.ContrastRanges = append(surface.ContrastRanges, InstructionContrastRange{
			TargetQty: target, Median: differences[1], Min: differences[0], Max: differences[2]})
	}
	return surface, nil
}

func WriteInstructionSurface(path string, surface InstructionSurface) error {
	if surface.SchemaVersion != 1 || surface.ValidWorlds != 12 || len(surface.PairedContrasts) != 6 ||
		len(surface.ContrastRanges) != 2 {
		return errors.New("instruction pilot: incomplete response surface")
	}
	return writeExclusiveJSON(path, surface)
}
