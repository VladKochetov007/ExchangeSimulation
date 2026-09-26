package executionpilot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"

	"exchange_sim/analysis"
)

type CadenceCellResult struct {
	ID                     string                `json:"id"`
	Cell                   CadenceCell           `json:"cell"`
	PlanRawSHA256          string                `json:"plan_raw_sha256"`
	TypedPlanSHA256        string                `json:"typed_plan_sha256"`
	EvidenceExecutionHash  string                `json:"evidence_execution_hash"`
	EvidenceFileSHA256     string                `json:"evidence_file_sha256"`
	EvidenceBytes          int64                 `json:"evidence_bytes"`
	ResultFileSHA256       string                `json:"result_file_sha256"`
	RunResourceSHA256      string                `json:"run_resource_sha256"`
	AnalysisResourceSHA256 string                `json:"analysis_resource_sha256"`
	WorldWallSeconds       float64               `json:"world_wall_seconds"`
	PeakCgroupMemoryBytes  uint64                `json:"peak_cgroup_memory_bytes"`
	FilledFraction         float64               `json:"filled_fraction"`
	Reconstruction         CadenceReconstruction `json:"reconstruction"`
	SelectedSampledQuote   *CadenceSampledQuote  `json:"selected_sampled_quote,omitempty"`
	OpportunityLifetime    string                `json:"opportunity_lifetime_status"`
}

type CadenceSampledQuote struct {
	SnapshotSeq             uint64 `json:"snapshot_seq"`
	QualifyingAtPublication bool   `json:"qualifying_at_publication"`
}

type CadencePairedContrast struct {
	Seed         int64   `json:"seed"`
	P1N1         float64 `json:"p1_n1"`
	P80N1        float64 `json:"p80_n1"`
	P1N90        float64 `json:"p1_n90"`
	P80N90       float64 `json:"p80_n90"`
	PollAtN1     float64 `json:"poll_effect_n1"`
	PollAtN90    float64 `json:"poll_effect_n90"`
	NetworkAtP1  float64 `json:"network_effect_p1"`
	NetworkAtP80 float64 `json:"network_effect_p80"`
	Interaction  float64 `json:"interaction"`
}

type CadenceContrastRange struct {
	Effect string  `json:"effect"`
	Median float64 `json:"median"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
}

type CadenceSurface struct {
	SchemaVersion             int                     `json:"schema_version"`
	AnalysisCorrection        string                  `json:"analysis_correction"`
	ExecutionIdentity         Identity                `json:"execution_identity"`
	AggregationCommit         string                  `json:"aggregation_commit"`
	AssignedWorlds            int                     `json:"assigned_worlds"`
	ValidWorlds               int                     `json:"valid_worlds"`
	TotalEvidenceBytes        int64                   `json:"total_evidence_bytes"`
	TotalWorldWallSeconds     float64                 `json:"total_world_wall_seconds"`
	MaxWorldWallSeconds       float64                 `json:"max_world_wall_seconds"`
	MaxPeakCgroupMemoryBytes  uint64                  `json:"max_peak_cgroup_memory_bytes"`
	ControlEvidenceFileSHA256 string                  `json:"control_evidence_file_sha256"`
	ControlResultFileSHA256   string                  `json:"control_result_file_sha256"`
	Cells                     []CadenceCellResult     `json:"cells"`
	PairedContrasts           []CadencePairedContrast `json:"paired_contrasts"`
	ContrastRanges            []CadenceContrastRange  `json:"contrast_ranges"`
}

const cadenceLifetimeUnavailable = "NOT_RECONSTRUCTIBLE_FROM_SAMPLED_SNAPSHOTS"

type cadenceStoredResult struct {
	SchemaVersion       int                   `json:"schema_version"`
	Cell                CadenceCell           `json:"cell"`
	ManifestTypedSHA256 string                `json:"manifest_typed_sha256"`
	FilledFraction      float64               `json:"filled_fraction"`
	Reconstruction      CadenceReconstruction `json:"reconstruction"`
}

func validateCadenceStoredResult(stored cadenceStoredResult, cell CadenceCell,
	manifestDigest string, reconstruction CadenceReconstruction) error {
	if stored.SchemaVersion != 1 || stored.Cell != cell ||
		stored.ManifestTypedSHA256 != manifestDigest ||
		!reflect.DeepEqual(stored.Reconstruction, reconstruction) ||
		reconstruction.Outcome.FilledQty < 0 || reconstruction.Outcome.FilledQty > cell.TargetQty ||
		stored.FilledFraction != float64(reconstruction.Outcome.FilledQty)/float64(cell.TargetQty) {
		return errors.New("cadence surface: stored result differs from bound raw replay")
	}
	return nil
}

func CadenceCells() []CadenceCell {
	cells := make([]CadenceCell, 0, 12)
	for _, arm := range [][2]int64{{1_000_000, 1_000_000}, {80_000_000, 1_000_000},
		{1_000_000, 90_000_000}, {80_000_000, 90_000_000}} {
		for _, seed := range []int64{13001, 13011, 13017} {
			cells = append(cells, CadenceCell{NetworkLatencyNanos: arm[1],
				PollIntervalNanos: arm[0], TargetQty: 500_000_000, Seed: seed})
		}
	}
	return cells
}

func cadenceCellID(cell CadenceCell) (string, error) {
	if err := ValidateCadenceCell(cell); err != nil {
		return "", err
	}
	return fmt.Sprintf("P%d-N%d-s%d", cell.PollIntervalNanos/1_000_000,
		cell.NetworkLatencyNanos/1_000_000, cell.Seed), nil
}

func readCadenceResource(path string) (analysis.SV1DResourceMeasurement, string, error) {
	measurement, err := decodeStrictFile[analysis.SV1DResourceMeasurement](path)
	if err != nil {
		return measurement, "", err
	}
	if err := analysis.ValidateSV1DResourceMeasurement(measurement, true); err != nil {
		return measurement, "", err
	}
	if measurement.CgroupMemoryLimitBytes != 4<<30 ||
		measurement.MinimumHostMemAvailableBytes < 8<<30 ||
		measurement.MaximumSwapUsedBytes != 0 {
		return measurement, "", errors.New("cadence surface: resource envelope differs from protocol")
	}
	digest, err := fileSHA256(path)
	return measurement, digest, err
}

func cadenceWorldSeconds(measurement analysis.SV1DResourceMeasurement) (float64, error) {
	if len(measurement.Samples) < 2 {
		return 0, errors.New("cadence surface: missing resource samples")
	}
	duration := float64(measurement.Samples[len(measurement.Samples)-1].ObservedAtUnixNano-
		measurement.Samples[0].ObservedAtUnixNano) / 1e9
	if duration <= 0 || duration > 120 {
		return 0, errors.New("cadence surface: world exceeded registered time envelope")
	}
	return duration, nil
}

// The pinned replay calls a bounded run of publication snapshots a complete
// opportunity. P2 does not identify a continuous executable lifetime from
// those snapshots, so the versioned ME-002-B report exposes only the sampled
// selected state and deliberately omits the old duration and delay ratio.
func cadenceReportedReconstruction(reconstruction CadenceReconstruction) (CadenceReconstruction, *CadenceSampledQuote) {
	var selected *CadenceSampledQuote
	if opportunity := reconstruction.Outcome.SelectedOpportunity; opportunity != nil {
		selected = &CadenceSampledQuote{SnapshotSeq: opportunity.SelectedSnapshotSeq,
			QualifyingAtPublication: opportunity.QualifyingAtPublication}
	}
	reconstruction.Outcome.SelectedOpportunity = nil
	return reconstruction, selected
}

func verifyCadenceControls(root string) (string, string, error) {
	controls := filepath.Join(root, "controls")
	var evidenceHash, resultHash string
	for _, suffix := range []string{"g1", "g7"} {
		for _, name := range []string{"control-" + suffix + ".json",
			"control-" + suffix + "-analysis.json"} {
			if _, _, err := readCadenceResource(filepath.Join(root, "measurements", name)); err != nil {
				return "", "", fmt.Errorf("control %s: %w", name, err)
			}
		}
		evidence, err := fileSHA256(filepath.Join(controls, suffix, EvidenceFilename))
		if err != nil {
			return "", "", err
		}
		result, err := fileSHA256(filepath.Join(controls, suffix+"-result.json"))
		if err != nil {
			return "", "", err
		}
		if evidenceHash != "" && (evidence != evidenceHash || result != resultHash) {
			return "", "", errors.New("cadence surface: fresh-process controls differ")
		}
		evidenceHash, resultHash = evidence, result
	}
	return evidenceHash, resultHash, nil
}

func loadCadenceCell(root string, cell CadenceCell, expectedSource string) (CadenceCellResult, Identity, error) {
	id, err := cadenceCellID(cell)
	if err != nil {
		return CadenceCellResult{}, Identity{}, err
	}
	planPath := filepath.Join(root, "plans", id+".json")
	runDir := filepath.Join(root, "economics", id)
	resultPath := filepath.Join(root, "results", id+".json")
	plan, rawPlanDigest, err := readCadencePlan(planPath)
	if err != nil || plan.Cell != cell || plan.Identity.SourceCommit != expectedSource {
		return CadenceCellResult{}, Identity{}, fmt.Errorf("%s: plan or source identity mismatch: %w", id, err)
	}
	if _, err := VerifyCadence(plan, plan.Identity); err != nil {
		return CadenceCellResult{}, Identity{}, fmt.Errorf("%s: %w", id, err)
	}
	manifest, err := decodeStrictFile[RunManifest](filepath.Join(runDir, ManifestFilename))
	if err != nil {
		return CadenceCellResult{}, Identity{}, err
	}
	if manifest.SchemaVersion != 2 || manifest.PlanRawSHA256 != rawPlanDigest ||
		manifest.TypedPlanSHA256 != plan.TypedPlanSHA256 || manifest.Identity != plan.Identity ||
		manifest.Evidence.SchemaID != LatencyEvidenceSchemaID {
		return CadenceCellResult{}, Identity{}, fmt.Errorf("%s: run manifest differs from locked plan", id)
	}
	evidencePath := filepath.Join(runDir, EvidenceFilename)
	evidenceDigest, err := fileSHA256(evidencePath)
	if err != nil || evidenceDigest != manifest.EvidenceFileSHA256 {
		return CadenceCellResult{}, Identity{}, fmt.Errorf("%s: evidence digest mismatch: %w", id, err)
	}
	actorPath := filepath.Join(runDir, ActorFilename)
	actorDigest, err := fileSHA256(actorPath)
	if err != nil || actorDigest != manifest.ActorFileSHA256 {
		return CadenceCellResult{}, Identity{}, fmt.Errorf("%s: actor digest mismatch: %w", id, err)
	}
	stored, err := decodeStrictFile[cadenceStoredResult](resultPath)
	if err != nil {
		return CadenceCellResult{}, Identity{}, err
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return CadenceCellResult{}, Identity{}, err
	}
	manifestDigest := sha256.Sum256(manifestBytes)
	evidence, err := os.Open(evidencePath)
	if err != nil {
		return CadenceCellResult{}, Identity{}, err
	}
	reconstruction, replayErr := ReconstructCadence(evidence, manifest.Evidence, plan.EffectiveWorld, cell)
	closeErr := evidence.Close()
	if replayErr != nil || closeErr != nil {
		return CadenceCellResult{}, Identity{}, fmt.Errorf("%s: replay %v / close %v", id, replayErr, closeErr)
	}
	if err := validateCadenceStoredResult(stored, cell,
		hex.EncodeToString(manifestDigest[:]), reconstruction); err != nil {
		return CadenceCellResult{}, Identity{}, fmt.Errorf("%s: %w", id, err)
	}
	actorRaw, err := os.ReadFile(actorPath)
	if err != nil {
		return CadenceCellResult{}, Identity{}, err
	}
	actorReport, err := decodeLatencyActorReport(actorRaw)
	if err != nil {
		return CadenceCellResult{}, Identity{}, fmt.Errorf("%s: decode actor report: %w", id, err)
	}
	if err := compareActorReport(reconstruction.Outcome, actorReport); err != nil {
		return CadenceCellResult{}, Identity{}, fmt.Errorf("%s: actor/replay mismatch: %w", id, err)
	}
	runResource, runResourceDigest, err := readCadenceResource(filepath.Join(root, "measurements", id+"-run.json"))
	if err != nil {
		return CadenceCellResult{}, Identity{}, err
	}
	_, analysisResourceDigest, err := readCadenceResource(filepath.Join(root, "measurements", id+"-analysis.json"))
	if err != nil {
		return CadenceCellResult{}, Identity{}, err
	}
	seconds, err := cadenceWorldSeconds(runResource)
	if err != nil {
		return CadenceCellResult{}, Identity{}, err
	}
	bytes, err := regularFileSize(evidencePath)
	if err != nil {
		return CadenceCellResult{}, Identity{}, err
	}
	resultDigest, err := fileSHA256(resultPath)
	if err != nil {
		return CadenceCellResult{}, Identity{}, err
	}
	reported, selected := cadenceReportedReconstruction(reconstruction)
	return CadenceCellResult{ID: id, Cell: cell, PlanRawSHA256: rawPlanDigest,
		TypedPlanSHA256: plan.TypedPlanSHA256, EvidenceExecutionHash: manifest.Evidence.ExecutionHash,
		EvidenceFileSHA256: evidenceDigest, EvidenceBytes: bytes, ResultFileSHA256: resultDigest,
		RunResourceSHA256: runResourceDigest, AnalysisResourceSHA256: analysisResourceDigest,
		WorldWallSeconds: seconds, PeakCgroupMemoryBytes: runResource.PeakCgroupMemoryBytes,
		FilledFraction: stored.FilledFraction, Reconstruction: reported,
		SelectedSampledQuote: selected, OpportunityLifetime: cadenceLifetimeUnavailable}, plan.Identity, nil
}

func cadenceContrast(seed int64, values [4]float64) CadencePairedContrast {
	return CadencePairedContrast{Seed: seed, P1N1: values[0], P80N1: values[1],
		P1N90: values[2], P80N90: values[3],
		PollAtN1: values[1] - values[0], PollAtN90: values[3] - values[2],
		NetworkAtP1: values[2] - values[0], NetworkAtP80: values[3] - values[1],
		Interaction: (values[3] - values[2]) - (values[1] - values[0])}
}

func cadenceContrasts(cells []CadenceCellResult) ([]CadencePairedContrast, []CadenceContrastRange, error) {
	byID := make(map[string]CadenceCellResult, len(cells))
	for _, cell := range cells {
		if _, duplicate := byID[cell.ID]; duplicate {
			return nil, nil, errors.New("cadence surface: duplicate cell")
		}
		byID[cell.ID] = cell
	}
	if len(byID) != 12 {
		return nil, nil, errors.New("cadence surface: incomplete assigned matrix")
	}
	contrasts := make([]CadencePairedContrast, 0, 3)
	for _, seed := range []int64{13001, 13011, 13017} {
		var values [4]float64
		for index, arm := range []string{"P1-N1", "P80-N1", "P1-N90", "P80-N90"} {
			cell, found := byID[fmt.Sprintf("%s-s%d", arm, seed)]
			if !found {
				return nil, nil, errors.New("cadence surface: missing matched arm")
			}
			values[index] = cell.FilledFraction
		}
		contrasts = append(contrasts, cadenceContrast(seed, values))
	}
	ranges := make([]CadenceContrastRange, 0, 5)
	for _, effect := range []struct {
		name        string
		selectValue func(CadencePairedContrast) float64
	}{
		{"poll_n1", func(c CadencePairedContrast) float64 { return c.PollAtN1 }},
		{"poll_n90", func(c CadencePairedContrast) float64 { return c.PollAtN90 }},
		{"network_p1", func(c CadencePairedContrast) float64 { return c.NetworkAtP1 }},
		{"network_p80", func(c CadencePairedContrast) float64 { return c.NetworkAtP80 }},
		{"interaction", func(c CadencePairedContrast) float64 { return c.Interaction }},
	} {
		values := []float64{effect.selectValue(contrasts[0]),
			effect.selectValue(contrasts[1]), effect.selectValue(contrasts[2])}
		slices.Sort(values)
		ranges = append(ranges, CadenceContrastRange{Effect: effect.name,
			Median: values[1], Min: values[0], Max: values[2]})
	}
	return contrasts, ranges, nil
}

func AggregateCadenceRuns(root, repositoryDir, expectedSource string) (CadenceSurface, error) {
	if !hexDigest(expectedSource, 20) {
		return CadenceSurface{}, errors.New("cadence surface: invalid execution source")
	}
	aggregationCommit, err := gitValue(repositoryDir, "rev-parse", "HEAD")
	if err != nil {
		return CadenceSurface{}, err
	}
	status, err := gitValue(repositoryDir, "status", "--porcelain")
	if err != nil || status != "" {
		return CadenceSurface{}, errors.New("cadence surface: aggregation checkout must be clean")
	}
	controlEvidence, controlResult, err := verifyCadenceControls(root)
	if err != nil {
		return CadenceSurface{}, err
	}
	surface := CadenceSurface{SchemaVersion: 2, AggregationCommit: aggregationCommit,
		AnalysisCorrection: "Pinned result v1's sampled-episode completion and action-delay/duration ratio are not continuous executable opportunity lifetimes; corrected view omits those fields for every cell.",
		AssignedWorlds:     12, ControlEvidenceFileSHA256: controlEvidence,
		ControlResultFileSHA256: controlResult}
	for _, cell := range CadenceCells() {
		result, identity, err := loadCadenceCell(root, cell, expectedSource)
		if err != nil {
			return CadenceSurface{}, err
		}
		if surface.ExecutionIdentity == (Identity{}) {
			surface.ExecutionIdentity = identity
		} else if surface.ExecutionIdentity != identity {
			return CadenceSurface{}, errors.New("cadence surface: mixed execution identities")
		}
		surface.Cells = append(surface.Cells, result)
		surface.TotalEvidenceBytes += result.EvidenceBytes
		surface.TotalWorldWallSeconds += result.WorldWallSeconds
		surface.MaxWorldWallSeconds = max(surface.MaxWorldWallSeconds, result.WorldWallSeconds)
		surface.MaxPeakCgroupMemoryBytes = max(surface.MaxPeakCgroupMemoryBytes, result.PeakCgroupMemoryBytes)
	}
	surface.ValidWorlds = len(surface.Cells)
	surface.PairedContrasts, surface.ContrastRanges, err = cadenceContrasts(surface.Cells)
	if err != nil {
		return CadenceSurface{}, err
	}
	if surface.TotalEvidenceBytes > 20<<30 || surface.TotalWorldWallSeconds > 3600 {
		return CadenceSurface{}, errors.New("cadence surface: batch exceeded registered resource cap")
	}
	return surface, nil
}

func WriteCadenceSurface(path string, surface CadenceSurface) error {
	if surface.SchemaVersion != 2 || surface.ValidWorlds != 12 ||
		len(surface.PairedContrasts) != 3 || len(surface.ContrastRanges) != 5 {
		return errors.New("cadence surface: incomplete result")
	}
	return writeExclusiveJSON(path, surface)
}
