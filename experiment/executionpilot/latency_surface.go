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

type LatencyCellResult struct {
	ID                    string               `json:"id"`
	Cell                  LatencyCell          `json:"cell"`
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

type LatencyPairedContrast struct {
	TargetQty        int64   `json:"target_qty"`
	Seed             int64   `json:"seed"`
	FastFast         float64 `json:"fast_fast"`
	FastSlow         float64 `json:"fast_slow"`
	SlowFast         float64 `json:"slow_fast"`
	SlowSlow         float64 `json:"slow_slow"`
	NetworkEffect    float64 `json:"network_effect"`
	ProcessingEffect float64 `json:"processing_effect"`
	Interaction      float64 `json:"interaction"`
}

type LatencyContrastRange struct {
	TargetQty int64   `json:"target_qty"`
	Effect    string  `json:"effect"`
	Median    float64 `json:"median"`
	Min       float64 `json:"min"`
	Max       float64 `json:"max"`
}

type LatencySurface struct {
	SchemaVersion             int                     `json:"schema_version"`
	ExecutionIdentity         Identity                `json:"execution_identity"`
	AggregationCommit         string                  `json:"aggregation_commit"`
	AssignedWorlds            int                     `json:"assigned_worlds"`
	ValidWorlds               int                     `json:"valid_worlds"`
	TotalEvidenceBytes        int64                   `json:"total_evidence_bytes"`
	TotalWorldWallSeconds     float64                 `json:"total_world_wall_seconds"`
	MaxWorldWallSeconds       float64                 `json:"max_world_wall_seconds"`
	MaxPeakRSSKiB             int64                   `json:"max_peak_rss_kib"`
	ControlEvidenceFileSHA256 string                  `json:"control_evidence_file_sha256"`
	ControlResultFileSHA256   string                  `json:"control_result_file_sha256"`
	Cells                     []LatencyCellResult     `json:"cells"`
	PairedContrasts           []LatencyPairedContrast `json:"paired_contrasts"`
	ContrastRanges            []LatencyContrastRange  `json:"contrast_ranges"`
}

func LatencyCells() []LatencyCell {
	cells := make([]LatencyCell, 0, 24)
	for _, target := range []int64{50_000_000, 500_000_000} {
		for _, deployment := range [][2]int64{{1_000_000, 0}, {1_000_000, 120_000_000},
			{90_000_000, 0}, {90_000_000, 120_000_000}} {
			for _, seed := range []int64{12001, 12011, 12017} {
				cells = append(cells, LatencyCell{deployment[0], deployment[1], target, seed})
			}
		}
	}
	return cells
}

func latencyCellID(cell LatencyCell) (string, error) {
	if err := ValidateLatencyCell(cell); err != nil {
		return "", err
	}
	arm := ""
	switch [2]int64{cell.NetworkLatencyNanos, cell.ProcessingDelayNanos} {
	case [2]int64{1_000_000, 0}:
		arm = "F-F"
	case [2]int64{1_000_000, 120_000_000}:
		arm = "F-S"
	case [2]int64{90_000_000, 0}:
		arm = "S-F"
	case [2]int64{90_000_000, 120_000_000}:
		arm = "S-S"
	}
	scale := "q0p5"
	if cell.TargetQty == 500_000_000 {
		scale = "q5"
	}
	return fmt.Sprintf("%s-%s-s%d", arm, scale, cell.Seed), nil
}

func readResourceStamp(path string) (float64, int64, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}
	fields := strings.Fields(string(raw))
	if len(fields) != 3 || fields[2] != "exit_status=0" ||
		!strings.HasPrefix(fields[0], "wall_seconds=") || !strings.HasPrefix(fields[1], "peak_rss_kib=") {
		return 0, 0, errors.New("latency pilot: malformed or failed resource stamp")
	}
	wall, err := strconv.ParseFloat(strings.TrimPrefix(fields[0], "wall_seconds="), 64)
	if err != nil || math.IsNaN(wall) || math.IsInf(wall, 0) || wall < 0 || wall > 30 {
		return 0, 0, errors.New("latency pilot: invalid per-world wall time")
	}
	rss, err := strconv.ParseInt(strings.TrimPrefix(fields[1], "peak_rss_kib="), 10, 64)
	if err != nil || rss <= 0 || rss > 4*1024*1024 {
		return 0, 0, errors.New("latency pilot: invalid per-world peak RSS")
	}
	return wall, rss, nil
}

func loadLatencyCell(root string, cell LatencyCell, expectedSource string) (LatencyCellResult, Identity, error) {
	id, err := latencyCellID(cell)
	if err != nil {
		return LatencyCellResult{}, Identity{}, err
	}
	planPath := filepath.Join(root, "plans", id+".json")
	runDir := filepath.Join(root, "runs", "dev-"+id)
	resultPath := filepath.Join(root, "analysis", "dev-"+id+".json")
	timePath := filepath.Join(root, "analysis", "dev-"+id+".time.txt")
	for _, path := range []string{planPath, resultPath, timePath, filepath.Join(runDir, ManifestFilename),
		filepath.Join(runDir, EvidenceFilename), filepath.Join(runDir, ActorFilename)} {
		if _, err := regularFileSize(path); err != nil {
			return LatencyCellResult{}, Identity{}, fmt.Errorf("%s: %w", id, err)
		}
	}
	plan, planRawSHA, err := readLatencyPlan(planPath)
	if err != nil || plan.Cell != cell || plan.Identity.SourceCommit != expectedSource {
		return LatencyCellResult{}, Identity{}, fmt.Errorf("%s: wrong cell or source identity: %w", id, err)
	}
	if _, err := VerifyLatency(plan, plan.Identity); err != nil {
		return LatencyCellResult{}, Identity{}, fmt.Errorf("%s: %w", id, err)
	}
	manifest, err := decodeStrictFile[RunManifest](filepath.Join(runDir, ManifestFilename))
	if err != nil {
		return LatencyCellResult{}, Identity{}, err
	}
	if manifest.SchemaVersion != 2 || manifest.PlanRawSHA256 != planRawSHA ||
		manifest.TypedPlanSHA256 != plan.TypedPlanSHA256 || manifest.Identity != plan.Identity ||
		manifest.Evidence.SchemaID != LatencyEvidenceSchemaID {
		return LatencyCellResult{}, Identity{}, fmt.Errorf("%s: manifest/plan identity mismatch", id)
	}
	evidencePath := filepath.Join(runDir, EvidenceFilename)
	evidenceSHA, err := fileSHA256(evidencePath)
	if err != nil || evidenceSHA != manifest.EvidenceFileSHA256 {
		return LatencyCellResult{}, Identity{}, fmt.Errorf("%s: evidence digest mismatch", id)
	}
	actorPath := filepath.Join(runDir, ActorFilename)
	actorSHA, err := fileSHA256(actorPath)
	if err != nil || actorSHA != manifest.ActorFileSHA256 {
		return LatencyCellResult{}, Identity{}, fmt.Errorf("%s: actor digest mismatch", id)
	}
	stored, err := decodeStrictFile[struct {
		SchemaVersion       int                  `json:"schema_version"`
		Cell                LatencyCell          `json:"cell"`
		ManifestTypedSHA256 string               `json:"manifest_typed_sha256"`
		Outcome             ReconstructedOutcome `json:"outcome"`
	}](resultPath)
	if err != nil {
		return LatencyCellResult{}, Identity{}, err
	}
	manifestCanonical, err := json.Marshal(manifest)
	if err != nil {
		return LatencyCellResult{}, Identity{}, err
	}
	manifestDigest := sha256.Sum256(manifestCanonical)
	if stored.SchemaVersion != 2 || stored.Cell != cell || stored.ManifestTypedSHA256 != hex.EncodeToString(manifestDigest[:]) {
		return LatencyCellResult{}, Identity{}, fmt.Errorf("%s: result/manifest mismatch", id)
	}
	evidence, err := os.Open(evidencePath)
	if err != nil {
		return LatencyCellResult{}, Identity{}, err
	}
	reconstructed, replayErr := ReconstructLatency(evidence, manifest.Evidence, plan.EffectiveWorld, cell.TargetQty)
	closeErr := evidence.Close()
	if replayErr != nil || closeErr != nil {
		return LatencyCellResult{}, Identity{}, fmt.Errorf("%s: replay %v / close %v", id, replayErr, closeErr)
	}
	if !reflect.DeepEqual(stored.Outcome, reconstructed) {
		return LatencyCellResult{}, Identity{}, fmt.Errorf("%s: stored outcome differs from raw replay", id)
	}
	actorRaw, err := os.ReadFile(actorPath)
	if err != nil {
		return LatencyCellResult{}, Identity{}, err
	}
	actorReport, err := decodeLatencyActorReport(actorRaw)
	if err != nil {
		return LatencyCellResult{}, Identity{}, err
	}
	if err := compareActorReport(reconstructed, actorReport); err != nil {
		return LatencyCellResult{}, Identity{}, err
	}
	if reconstructed.FilledQty < 0 || reconstructed.FilledQty > cell.TargetQty || reconstructed.LatencyFunnel == nil ||
		!reconstructed.BalanceSnapshotSeen {
		return LatencyCellResult{}, Identity{}, fmt.Errorf("%s: invalid reconstructed endpoint", id)
	}
	wall, rss, err := readResourceStamp(timePath)
	if err != nil {
		return LatencyCellResult{}, Identity{}, err
	}
	bytes, err := regularFileSize(evidencePath)
	if err != nil {
		return LatencyCellResult{}, Identity{}, err
	}
	return LatencyCellResult{ID: id, Cell: cell, PlanRawSHA256: planRawSHA, TypedPlanSHA256: plan.TypedPlanSHA256,
		EvidenceExecutionHash: manifest.Evidence.ExecutionHash, EvidenceFileSHA256: evidenceSHA,
		EvidenceBytes: bytes, WallSeconds: wall, PeakRSSKiB: rss,
		FilledFraction: float64(reconstructed.FilledQty) / float64(cell.TargetQty), Outcome: reconstructed}, plan.Identity, nil
}

func verifyLatencyControls(root string) (string, string, error) {
	first := "control-F-F-q0p5-s12001-g1"
	second := "control-F-F-q0p5-s12001-g7"
	evidenceA := filepath.Join(root, "runs", first, EvidenceFilename)
	evidenceB := filepath.Join(root, "runs", second, EvidenceFilename)
	resultA := filepath.Join(root, "analysis", first+".json")
	resultB := filepath.Join(root, "analysis", second+".json")
	for _, path := range []string{evidenceA, evidenceB, resultA, resultB} {
		if _, err := regularFileSize(path); err != nil {
			return "", "", err
		}
	}
	evidenceHashA, err := fileSHA256(evidenceA)
	if err != nil {
		return "", "", err
	}
	evidenceHashB, err := fileSHA256(evidenceB)
	if err != nil || evidenceHashA != evidenceHashB {
		return "", "", errors.New("latency pilot: control evidence differs across fresh processes")
	}
	resultHashA, err := fileSHA256(resultA)
	if err != nil {
		return "", "", err
	}
	resultHashB, err := fileSHA256(resultB)
	if err != nil || resultHashA != resultHashB {
		return "", "", errors.New("latency pilot: control results differ across fresh processes")
	}
	return evidenceHashA, resultHashA, nil
}

func AggregateLatencyRuns(root, repositoryDir, expectedSource string) (LatencySurface, error) {
	if !hexDigest(expectedSource, 20) {
		return LatencySurface{}, errors.New("latency pilot: invalid execution source")
	}
	aggregationCommit, err := gitValue(repositoryDir, "rev-parse", "HEAD")
	if err != nil {
		return LatencySurface{}, err
	}
	status, err := gitValue(repositoryDir, "status", "--porcelain")
	if err != nil || status != "" {
		return LatencySurface{}, errors.New("latency pilot: aggregation checkout must be clean")
	}
	controlEvidence, controlResult, err := verifyLatencyControls(root)
	if err != nil {
		return LatencySurface{}, err
	}
	surface := LatencySurface{SchemaVersion: 1, AggregationCommit: aggregationCommit,
		AssignedWorlds: 24, ControlEvidenceFileSHA256: controlEvidence, ControlResultFileSHA256: controlResult}
	for _, cell := range LatencyCells() {
		result, identity, err := loadLatencyCell(root, cell, expectedSource)
		if err != nil {
			return LatencySurface{}, err
		}
		if surface.ExecutionIdentity == (Identity{}) {
			surface.ExecutionIdentity = identity
		} else if surface.ExecutionIdentity != identity {
			return LatencySurface{}, errors.New("latency pilot: mixed execution identities")
		}
		surface.Cells = append(surface.Cells, result)
		surface.TotalEvidenceBytes += result.EvidenceBytes
		surface.TotalWorldWallSeconds += result.WallSeconds
		surface.MaxWorldWallSeconds = max(surface.MaxWorldWallSeconds, result.WallSeconds)
		surface.MaxPeakRSSKiB = max(surface.MaxPeakRSSKiB, result.PeakRSSKiB)
	}
	if len(surface.Cells) != 24 || surface.TotalEvidenceBytes > 1<<30 || surface.TotalWorldWallSeconds > 900 {
		return LatencySurface{}, errors.New("latency pilot: incomplete matrix or exceeded batch resource cap")
	}
	surface.ValidWorlds = len(surface.Cells)
	for _, target := range []int64{50_000_000, 500_000_000} {
		var network, processing, interaction []float64
		for _, seed := range []int64{12001, 12011, 12017} {
			var values [4]float64
			found := [4]bool{}
			for _, result := range surface.Cells {
				if result.Cell.TargetQty != target || result.Cell.Seed != seed {
					continue
				}
				index := -1
				switch [2]int64{result.Cell.NetworkLatencyNanos, result.Cell.ProcessingDelayNanos} {
				case [2]int64{1_000_000, 0}:
					index = 0
				case [2]int64{1_000_000, 120_000_000}:
					index = 1
				case [2]int64{90_000_000, 0}:
					index = 2
				case [2]int64{90_000_000, 120_000_000}:
					index = 3
				}
				if index < 0 || found[index] {
					return LatencySurface{}, errors.New("latency pilot: duplicate or unknown matched arm")
				}
				values[index], found[index] = result.FilledFraction, true
			}
			for _, present := range found {
				if !present {
					return LatencySurface{}, errors.New("latency pilot: missing matched arm")
				}
			}
			contrast := latencyPairedContrast(target, seed, values)
			surface.PairedContrasts = append(surface.PairedContrasts, contrast)
			network = append(network, contrast.NetworkEffect)
			processing = append(processing, contrast.ProcessingEffect)
			interaction = append(interaction, contrast.Interaction)
		}
		for _, effect := range []struct {
			name   string
			values []float64
		}{{"network", network}, {"processing", processing}, {"interaction", interaction}} {
			slices.Sort(effect.values)
			surface.ContrastRanges = append(surface.ContrastRanges, LatencyContrastRange{TargetQty: target, Effect: effect.name,
				Median: effect.values[1], Min: effect.values[0], Max: effect.values[2]})
		}
	}
	return surface, nil
}

func latencyPairedContrast(target, seed int64, values [4]float64) LatencyPairedContrast {
	return LatencyPairedContrast{TargetQty: target, Seed: seed,
		FastFast: values[0], FastSlow: values[1], SlowFast: values[2], SlowSlow: values[3],
		NetworkEffect:    ((values[2] + values[3]) - (values[0] + values[1])) / 2,
		ProcessingEffect: ((values[1] + values[3]) - (values[0] + values[2])) / 2,
		Interaction:      values[3] - values[2] - values[1] + values[0]}
}

func WriteLatencySurface(path string, surface LatencySurface) error {
	if surface.SchemaVersion != 1 || surface.ValidWorlds != 24 || len(surface.PairedContrasts) != 6 {
		return errors.New("latency pilot: incomplete surface")
	}
	return writeExclusiveJSON(path, surface)
}
