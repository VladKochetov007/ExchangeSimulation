package repeatedspot

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"exchange_sim/analysis"
	"exchange_sim/experiment/executionpilot"
)

type ME016SurfaceCell struct {
	Cell                     ME016Cell               `json:"cell"`
	PlanRawSHA256            string                  `json:"plan_raw_sha256"`
	TypedPlanSHA256          string                  `json:"typed_plan_sha256"`
	ManifestSHA256           string                  `json:"manifest_sha256"`
	ResultSHA256             string                  `json:"result_sha256"`
	EvidenceFileSHA256       string                  `json:"evidence_file_sha256"`
	ExecutionHash            string                  `json:"execution_hash"`
	RunResourceSHA256        string                  `json:"run_resource_sha256"`
	AnalysisResourceSHA256   string                  `json:"analysis_resource_sha256"`
	PeakRunAllocatedBytes    uint64                  `json:"peak_run_allocated_bytes"`
	PeakRunCgroupMemoryBytes uint64                  `json:"peak_run_cgroup_memory_bytes"`
	PeakAnalysisCgroupBytes  uint64                  `json:"peak_analysis_cgroup_memory_bytes"`
	RunWorkerCountAttested   bool                    `json:"run_worker_count_attested"`
	WindowPublicDepth        RestingDepthSummary     `json:"window_public_depth"`
	WindowBestBook           SignalWindowBookSummary `json:"window_best_book"`
	TerminalMarkStatus       string                  `json:"terminal_mark_status"`
	TradeCount               int                     `json:"trade_count"`
	SignalMakerFunnels       []SignalMakerFunnel     `json:"signal_maker_funnels"`
	SignalSnapshotOwnCounts  []ME016OwnSnapshotCount `json:"signal_snapshot_own_counts"`
	MakerAccounts            []AccountResult         `json:"maker_accounts"`
}

type ME016OwnSnapshotCount struct {
	ClientID           uint64 `json:"client_id"`
	DeliveredSnapshots int64  `json:"delivered_snapshots"`
	OwnBestBidPresent  int64  `json:"own_best_bid_present"`
	OwnBestAskPresent  int64  `json:"own_best_ask_present"`
	OwnOnlyBestBid     int64  `json:"own_only_best_bid"`
	OwnOnlyBestAsk     int64  `json:"own_only_best_ask"`
}

type ME016SeedPair struct {
	Composition            string `json:"composition"`
	Seed                   int64  `json:"seed"`
	GainZeroTwoSidedNanos  int64  `json:"gain_zero_two_sided_ns"`
	GainTwoTwoSidedNanos   int64  `json:"gain_two_two_sided_ns"`
	GainTwoMinusZeroNanos  int64  `json:"gain_two_minus_zero_ns"`
	WindowDenominatorNanos int64  `json:"window_denominator_ns"`
}

type ME016TechnicalControl struct {
	Cell                    ME016Cell `json:"cell"`
	ResultSHA256            string    `json:"result_sha256"`
	ManifestSHA256          string    `json:"manifest_sha256"`
	EvidenceFileSHA256      string    `json:"evidence_file_sha256"`
	RunResourceSHA256       string    `json:"run_resource_sha256"`
	AnalysisResourceSHA256  string    `json:"analysis_resource_sha256"`
	WorkerOneCommandBound   bool      `json:"worker_one_command_bound"`
	BitIdenticalToReference bool      `json:"bit_identical_to_reference"`
}

type ME016DevelopmentSurface struct {
	SchemaVersion            int                     `json:"schema_version"`
	StudyID                  string                  `json:"study_id"`
	StudyStage               string                  `json:"study_stage"`
	ExecutionIdentity        executionpilot.Identity `json:"execution_identity"`
	ExpectedEconomicCells    int                     `json:"expected_economic_cells"`
	ValidEconomicCells       int                     `json:"valid_economic_cells"`
	WindowDenominatorNanos   int64                   `json:"window_denominator_ns"`
	PrimaryContrastStatus    string                  `json:"primary_contrast_status"`
	PrimaryM1Pairs           []ME016SeedPair         `json:"primary_m1_pairs"`
	SensitivityM2Pairs       []ME016SeedPair         `json:"sensitivity_m2_pairs"`
	PrimaryMedianDeltaNanos  int64                   `json:"primary_median_delta_ns"`
	PrimaryMinimumDeltaNanos int64                   `json:"primary_minimum_delta_ns"`
	PrimaryMaximumDeltaNanos int64                   `json:"primary_maximum_delta_ns"`
	TerminalMarksAvailable   int                     `json:"terminal_marks_available"`
	FailedAnalysisAttemptSHA string                  `json:"failed_first_analysis_attempt_sha256"`
	TechnicalControl         *ME016TechnicalControl  `json:"technical_control,omitempty"`
	Cells                    []ME016SurfaceCell      `json:"cells"`
}

type me016SurfaceArtifact struct {
	plan             ME016LockedPlan
	manifest         ME016RunManifest
	result           ME016AnalyzedResult
	planHash         string
	manifestHash     string
	resultHash       string
	runResourceHash  string
	analysisHash     string
	runResource      analysis.SV1DResourceMeasurement
	analysisResource analysis.SV1DResourceMeasurement
}

// BuildME016DevelopmentSurface reads only retained plans, manifests, results and
// resource records. The source-pinned strict analyzer, not this summary, replays
// every event. The override identifies a retained successful retry, never an
// overwritten or discarded analysis attempt.
func BuildME016DevelopmentSurface(root string, analysisResourceOverrides map[string]string) (ME016DevelopmentSurface, error) {
	registered := ME016DevelopmentCells()
	if err := requireME016RunInventory(root, registered); err != nil {
		return ME016DevelopmentSurface{}, err
	}
	artifacts := make(map[ME016Cell]me016SurfaceArtifact, len(registered))
	for _, cell := range registered {
		id := cell.ID()
		resourcePath := filepath.Join(root, "resources", id+"-analysis-resource.json")
		if replacement, ok := analysisResourceOverrides[id]; ok {
			resourcePath = replacement
		}
		artifact, err := loadME016SurfaceArtifact(root, id, cell,
			filepath.Join(root, "plans", id+"-plan.json"), resourcePath)
		if err != nil {
			return ME016DevelopmentSurface{}, fmt.Errorf("%s: %w", id, err)
		}
		artifacts[cell] = artifact
	}
	if len(analysisResourceOverrides) > 0 {
		for id := range analysisResourceOverrides {
			found := false
			for _, cell := range registered {
				found = found || cell.ID() == id
			}
			if !found {
				return ME016DevelopmentSurface{}, fmt.Errorf("unregistered analysis resource override %s", id)
			}
		}
	}
	surface, err := summarizeME016Artifacts(registered, artifacts)
	if err != nil {
		return ME016DevelopmentSurface{}, err
	}
	first := registered[0]
	failedHash, failedAttempt, err := readME016StrictFile[analysis.SV1DResourceMeasurement](
		filepath.Join(root, "resources", first.ID()+"-analysis-resource.json"))
	if err != nil || failedAttempt.Complete || failedAttempt.ExitStatus == 0 ||
		failedAttempt.MeasurementRoot != filepath.Join(root, "analysis", first.ID()) {
		return ME016DevelopmentSurface{}, errors.New("missing retained first analysis failure record")
	}
	surface.FailedAnalysisAttemptSHA = failedHash
	controlID := first.ID() + "-g1-control"
	control, err := loadME016SurfaceArtifact(root, controlID, first,
		filepath.Join(root, "plans", first.ID()+"-plan.json"),
		filepath.Join(root, "resources", controlID+"-analysis-resource.json"))
	if err != nil {
		return ME016DevelopmentSurface{}, fmt.Errorf("technical control: %w", err)
	}
	reference := artifacts[first]
	if control.planHash != reference.planHash || control.manifestHash != reference.manifestHash ||
		control.resultHash != reference.resultHash || control.manifest.EvidenceFileSHA256 != reference.manifest.EvidenceFileSHA256 ||
		!slices.Equal(control.runResource.Command[:min(2, len(control.runResource.Command))], []string{"/usr/bin/env", "GOMAXPROCS=1"}) {
		return ME016DevelopmentSurface{}, errors.New("ME-016 technical control is not byte-identical or worker-one command is unbound")
	}
	surface.TechnicalControl = &ME016TechnicalControl{Cell: first, ResultSHA256: control.resultHash,
		ManifestSHA256: control.manifestHash, EvidenceFileSHA256: control.manifest.EvidenceFileSHA256,
		RunResourceSHA256: control.runResourceHash, AnalysisResourceSHA256: control.analysisHash,
		WorkerOneCommandBound: true, BitIdenticalToReference: true}
	return surface, nil
}

func requireME016RunInventory(root string, registered []ME016Cell) error {
	expected := make(map[string]bool, len(registered)+1)
	for _, cell := range registered {
		expected[cell.ID()+"-run"] = true
	}
	expected[registered[0].ID()+"-g1-control-run"] = true
	entries, err := os.ReadDir(filepath.Join(root, "cells"))
	if err != nil {
		return err
	}
	if len(entries) != len(expected) {
		return errors.New("ME-016 retained run inventory differs from registered attempts")
	}
	for _, entry := range entries {
		if !entry.IsDir() || !expected[entry.Name()] {
			return fmt.Errorf("unregistered ME-016 retained run: %s", entry.Name())
		}
	}
	return nil
}

func loadME016SurfaceArtifact(root, id string, cell ME016Cell, planPath, analysisResourcePath string) (me016SurfaceArtifact, error) {
	plan, planHash, err := readME016Plan(planPath)
	if err != nil {
		return me016SurfaceArtifact{}, err
	}
	runDir := filepath.Join(root, "cells", id+"-run")
	manifestPath := filepath.Join(runDir, e0ManifestFilename)
	manifestHash, manifest, err := readME016StrictFile[ME016RunManifest](manifestPath)
	if err != nil {
		return me016SurfaceArtifact{}, err
	}
	resultHash, result, err := readME016StrictFile[ME016AnalyzedResult](filepath.Join(root, "analysis", id, "result.json"))
	if err != nil {
		return me016SurfaceArtifact{}, err
	}
	if plan.Cell != cell || result.Cell != cell || result.SchemaVersion != 1 || result.Identity != plan.Identity ||
		result.ManifestFileSHA256 != manifestHash || result.EvidenceFileSHA256 != manifest.EvidenceFileSHA256 {
		return me016SurfaceArtifact{}, errors.New("plan, manifest or result identity mismatch")
	}
	world, err := VerifyME016Plan(plan, result.Identity)
	if err != nil {
		return me016SurfaceArtifact{}, err
	}
	world.Close()
	if err := verifyME016Manifest(plan, planHash, result.Identity, manifest); err != nil {
		return me016SurfaceArtifact{}, err
	}
	evidenceHash, err := hashE0File(filepath.Join(runDir, e0EvidenceFilename))
	if err != nil || evidenceHash != manifest.EvidenceFileSHA256 {
		return me016SurfaceArtifact{}, errors.New("retained canonical evidence hash mismatch")
	}
	sidecars, err := hashE0Sidecars(runDir)
	if err != nil {
		return me016SurfaceArtifact{}, err
	}
	for name, hash := range sidecars {
		if manifest.SidecarSHA256[name] != hash {
			return me016SurfaceArtifact{}, fmt.Errorf("retained %s hash mismatch", name)
		}
	}
	runHash, runResource, err := readME016Resource(filepath.Join(root, "resources", id+"-resource.json"), runDir)
	if err != nil {
		return me016SurfaceArtifact{}, fmt.Errorf("run resource: %w", err)
	}
	analysisHash, analysisResource, err := readME016Resource(analysisResourcePath, filepath.Join(root, "analysis", id))
	if err != nil {
		return me016SurfaceArtifact{}, fmt.Errorf("analysis resource: %w", err)
	}
	resultPath := filepath.Join(root, "analysis", id, "result.json")
	if !slices.Contains(runResource.Command, runDir) || !slices.Contains(runResource.Command, planPath) ||
		!slices.Contains(analysisResource.Command, runDir) || !slices.Contains(analysisResource.Command, resultPath) ||
		!slices.Contains(analysisResource.Command, planPath) {
		return me016SurfaceArtifact{}, errors.New("resource command lacks its exact plan/run/result binding")
	}
	return me016SurfaceArtifact{plan: plan, manifest: manifest, result: result, planHash: planHash,
		manifestHash: manifestHash, resultHash: resultHash, runResourceHash: runHash,
		analysisHash: analysisHash, runResource: runResource, analysisResource: analysisResource}, nil
}

func readME016StrictFile[T any](path string) (string, T, error) {
	var value T
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", value, err
	}
	if err := decodeE0Strict(raw, &value); err != nil {
		return "", value, err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), value, nil
}

func readME016Resource(path, expectedRoot string) (string, analysis.SV1DResourceMeasurement, error) {
	hash, resource, err := readME016StrictFile[analysis.SV1DResourceMeasurement](path)
	if err != nil {
		return "", resource, err
	}
	if err := analysis.ValidateSV1DResourceMeasurement(resource, true); err != nil {
		return "", resource, err
	}
	if resource.MeasurementRoot != expectedRoot || resource.OutputParent != filepath.Dir(filepath.Dir(expectedRoot)) ||
		resource.CgroupMemoryLimitBytes > 8<<30 ||
		resource.MinimumAvailableBytes < 10<<30 || resource.PeakAllocatedBytes > 3<<29 ||
		resource.MaximumSwapUsedBytes != 0 || resource.CgroupOOMEventsDelta != 0 ||
		resource.CgroupOOMKillEventsDelta != 0 || resource.CgroupLocalOOMDelta != 0 ||
		resource.CgroupLocalOOMKillDelta != 0 {
		return "", resource, errors.New("resource record exceeds ME-016 envelope")
	}
	return hash, resource, nil
}

func summarizeME016Artifacts(registered []ME016Cell, artifacts map[ME016Cell]me016SurfaceArtifact) (ME016DevelopmentSurface, error) {
	window := E0MeasurementWindow()
	denominator := window.EndAt - window.StartAt
	surface := ME016DevelopmentSurface{SchemaVersion: 1, StudyID: "ME-016", StudyStage: "DEVELOPMENT",
		ExpectedEconomicCells: len(registered), WindowDenominatorNanos: denominator,
		PrimaryContrastStatus: "ESTIMATED_DEVELOPMENT_ONLY"}
	for _, cell := range registered {
		artifact, present := artifacts[cell]
		if !present || artifact.result.EconomicReconstruction == nil {
			return ME016DevelopmentSurface{}, fmt.Errorf("missing strict ME-016 result for %s", cell.ID())
		}
		result, replay := artifact.result, artifact.result.EconomicReconstruction
		if surface.ExecutionIdentity == (executionpilot.Identity{}) {
			surface.ExecutionIdentity = result.Identity
		}
		if result.Identity != surface.ExecutionIdentity || replay.InformationAudit == nil ||
			!replay.InformationAudit.Valid || replay.SignalAudit == nil || replay.MeasurementWindow != window ||
			replay.ContractSHA256 != artifact.manifest.ContractSHA256 || replay.Evidence != artifact.manifest.Evidence ||
			replay.PublicWindowDepth.WindowNanos != denominator || replay.SignalAudit.Window.WindowNanos != denominator ||
			replay.PublicWindowDepth.TwoSidedNanos != replay.SignalAudit.Window.TwoSidedNanos ||
			replay.PublicWindowDepth.BidPresentNanos != replay.SignalAudit.Window.BidPresentNanos ||
			replay.PublicWindowDepth.AskPresentNanos != replay.SignalAudit.Window.AskPresentNanos {
			return ME016DevelopmentSurface{}, fmt.Errorf("inconsistent ME-016 replay for %s", cell.ID())
		}
		if err := validateME016BestBook(replay.SignalAudit.Window, replay.PublicWindowDepth); err != nil {
			return ME016DevelopmentSurface{}, fmt.Errorf("%s: %w", cell.ID(), err)
		}
		makerAccounts, err := me016MakerAccounts(cell, replay)
		if err != nil {
			return ME016DevelopmentSurface{}, err
		}
		ownCounts, err := me016SnapshotCounts(replay.SignalAudit)
		if err != nil {
			return ME016DevelopmentSurface{}, err
		}
		if err := validateME016Funnel(cell, replay.SignalAudit.Makers); err != nil {
			return ME016DevelopmentSurface{}, err
		}
		if replay.TerminalMarkStatus == "AVAILABLE_TWO_SIDED_MID" {
			surface.TerminalMarksAvailable++
			if replay.TerminalMidQuote == nil {
				return ME016DevelopmentSurface{}, errors.New("available terminal mark lacks midpoint")
			}
		} else if replay.TerminalMarkStatus != "UNAVAILABLE_ONE_SIDED_OR_EMPTY" || replay.TerminalMidQuote != nil {
			return ME016DevelopmentSurface{}, errors.New("ME-016 terminal mark status is inconsistent")
		}
		for _, account := range makerAccounts {
			if (replay.TerminalMidQuote == nil) != (account.BenchmarkGain == nil) {
				return ME016DevelopmentSurface{}, errors.New("ME-016 maker gain contradicts terminal mark")
			}
		}
		workerCountAttested := len(artifact.runResource.Command) >= 2 &&
			artifact.runResource.Command[0] == "/usr/bin/env" && artifact.runResource.Command[1] == "GOMAXPROCS=4"
		surface.Cells = append(surface.Cells, ME016SurfaceCell{Cell: cell, PlanRawSHA256: artifact.planHash,
			TypedPlanSHA256: artifact.plan.TypedPlanSHA256, ManifestSHA256: artifact.manifestHash,
			ResultSHA256: artifact.resultHash, EvidenceFileSHA256: result.EvidenceFileSHA256,
			ExecutionHash: replay.Evidence.ExecutionHash, RunResourceSHA256: artifact.runResourceHash,
			AnalysisResourceSHA256:   artifact.analysisHash,
			PeakRunAllocatedBytes:    artifact.runResource.PeakAllocatedBytes,
			PeakRunCgroupMemoryBytes: artifact.runResource.PeakCgroupMemoryBytes,
			PeakAnalysisCgroupBytes:  artifact.analysisResource.PeakCgroupMemoryBytes,
			RunWorkerCountAttested:   workerCountAttested, WindowPublicDepth: replay.PublicWindowDepth,
			WindowBestBook: replay.SignalAudit.Window, TerminalMarkStatus: replay.TerminalMarkStatus,
			TradeCount: replay.TradeCount, SignalMakerFunnels: replay.SignalAudit.Makers,
			SignalSnapshotOwnCounts: ownCounts, MakerAccounts: makerAccounts})
	}
	surface.ValidEconomicCells = len(surface.Cells)
	for _, composition := range [...]string{"M1", "M2"} {
		for _, seed := range me016DevelopmentSeeds {
			zero := artifacts[ME016Cell{Composition: composition, SignalGainBps: 0, Seed: seed}]
			two := artifacts[ME016Cell{Composition: composition, SignalGainBps: 2, Seed: seed}]
			zeroNanos := zero.result.EconomicReconstruction.PublicWindowDepth.TwoSidedNanos
			twoNanos := two.result.EconomicReconstruction.PublicWindowDepth.TwoSidedNanos
			pair := ME016SeedPair{Composition: composition, Seed: seed, GainZeroTwoSidedNanos: zeroNanos,
				GainTwoTwoSidedNanos: twoNanos, GainTwoMinusZeroNanos: twoNanos - zeroNanos,
				WindowDenominatorNanos: denominator}
			if composition == "M1" {
				surface.PrimaryM1Pairs = append(surface.PrimaryM1Pairs, pair)
			} else {
				surface.SensitivityM2Pairs = append(surface.SensitivityM2Pairs, pair)
			}
		}
	}
	deltas := []int64{surface.PrimaryM1Pairs[0].GainTwoMinusZeroNanos,
		surface.PrimaryM1Pairs[1].GainTwoMinusZeroNanos, surface.PrimaryM1Pairs[2].GainTwoMinusZeroNanos}
	slices.Sort(deltas)
	surface.PrimaryMinimumDeltaNanos, surface.PrimaryMedianDeltaNanos, surface.PrimaryMaximumDeltaNanos = deltas[0], deltas[1], deltas[2]
	return surface, nil
}

func validateME016BestBook(book SignalWindowBookSummary, public RestingDepthSummary) error {
	if book.TwoSidedNanos < 0 || book.TwoSidedNanos > book.WindowNanos ||
		book.BidPresentNanos < book.TwoSidedNanos || book.AskPresentNanos < book.TwoSidedNanos ||
		book.BidPresentNanos > book.WindowNanos || book.AskPresentNanos > book.WindowNanos {
		return errors.New("invalid ME-016 book-duration bounds")
	}
	for _, side := range [][4]string{
		{book.BestBidBaseUnitNanos, book.MakerBestBidBaseUnitNanos, book.SeedBestBidBaseUnitNanos, book.OtherBestBidBaseUnitNanos},
		{book.BestAskBaseUnitNanos, book.MakerBestAskBaseUnitNanos, book.SeedBestAskBaseUnitNanos, book.OtherBestAskBaseUnitNanos},
	} {
		total, err := me016NonnegativeInteger(side[0])
		if err != nil {
			return err
		}
		parts := new(big.Int)
		for _, raw := range side[1:] {
			part, err := me016NonnegativeInteger(raw)
			if err != nil {
				return err
			}
			parts.Add(parts, part)
		}
		if parts.Cmp(total) != 0 {
			return errors.New("ME-016 best depth does not reconcile to owners")
		}
	}
	spread, err := me016NonnegativeInteger(book.SpreadPriceUnitNanos)
	if err != nil || (book.TwoSidedNanos == 0) != (spread.Sign() == 0) {
		return errors.New("ME-016 spread lacks its two-sided denominator")
	}
	for _, pair := range [][2]string{{book.BestBidBaseUnitNanos, public.BidDepthBaseUnitNanos},
		{book.BestAskBaseUnitNanos, public.AskDepthBaseUnitNanos}} {
		best, err := me016NonnegativeInteger(pair[0])
		if err != nil {
			return err
		}
		all, err := me016NonnegativeInteger(pair[1])
		if err != nil || best.Cmp(all) > 0 {
			return errors.New("ME-016 best depth exceeds all-level public depth")
		}
	}
	return nil
}

func me016NonnegativeInteger(raw string) (*big.Int, error) {
	value, ok := new(big.Int).SetString(raw, 10)
	if !ok || value.Sign() < 0 {
		return nil, errors.New("invalid ME-016 nonnegative integer")
	}
	return value, nil
}

func me016MakerAccounts(cell ME016Cell, replay *EconomicReplay) ([]AccountResult, error) {
	accounts := make([]AccountResult, 0, 4)
	seen := make(map[uint64]bool, 4)
	for _, account := range replay.Accounts {
		if strings.HasPrefix(account.Role, "maker_slot_") {
			if account.ActorID < 2 || account.ActorID > 5 || account.ClientID != account.ActorID ||
				account.Role != fmt.Sprintf("maker_slot_%d", account.ActorID-1) || seen[account.ActorID] ||
				account.MakerEnvelope == nil ||
				account.MakerEnvelope.WindowNanos != E0MeasurementWindow().EndAt-E0MeasurementWindow().StartAt {
				return nil, fmt.Errorf("%s maker risk evidence incomplete", cell.ID())
			}
			seen[account.ActorID] = true
			accounts = append(accounts, account)
		}
	}
	if len(accounts) != 4 {
		return nil, fmt.Errorf("%s maker roster incomplete", cell.ID())
	}
	return accounts, nil
}

func me016SnapshotCounts(audit *ME016SignalAudit) ([]ME016OwnSnapshotCount, error) {
	byClient := make(map[uint64]*ME016OwnSnapshotCount, len(audit.Makers))
	for _, maker := range audit.Makers {
		if byClient[maker.ClientID] != nil {
			return nil, errors.New("duplicate ME-016 signal maker")
		}
		byClient[maker.ClientID] = &ME016OwnSnapshotCount{ClientID: maker.ClientID}
	}
	for _, snapshot := range audit.Snapshots {
		count := byClient[snapshot.ClientID]
		if count == nil || snapshot.OwnBidQty < 0 || snapshot.OwnAskQty < 0 ||
			snapshot.OwnBidQty > snapshot.PublicBidQty || snapshot.OwnAskQty > snapshot.PublicAskQty ||
			snapshot.OwnOnlyBid != (snapshot.PublicBidQty > 0 && snapshot.OwnBidQty == snapshot.PublicBidQty) ||
			snapshot.OwnOnlyAsk != (snapshot.PublicAskQty > 0 && snapshot.OwnAskQty == snapshot.PublicAskQty) {
			return nil, errors.New("ME-016 delivered ownership is inconsistent")
		}
		count.DeliveredSnapshots++
		if snapshot.OwnBidQty > 0 {
			count.OwnBestBidPresent++
		}
		if snapshot.OwnAskQty > 0 {
			count.OwnBestAskPresent++
		}
		if snapshot.OwnOnlyBid {
			count.OwnOnlyBestBid++
		}
		if snapshot.OwnOnlyAsk {
			count.OwnOnlyBestAsk++
		}
	}
	counts := make([]ME016OwnSnapshotCount, 0, len(byClient))
	for _, count := range byClient {
		counts = append(counts, *count)
	}
	slices.SortFunc(counts, func(left, right ME016OwnSnapshotCount) int { return cmp.Compare(left.ClientID, right.ClientID) })
	return counts, nil
}

func WriteME016Surface(path string, surface ME016DevelopmentSurface) error {
	if surface.StudyID != "ME-016" || surface.ValidEconomicCells != len(ME016DevelopmentCells()) {
		return errors.New("incomplete ME-016 surface")
	}
	return writeE0ExclusiveJSON(path, surface)
}

func validateME016Funnel(cell ME016Cell, funnels []SignalMakerFunnel) error {
	if len(funnels) != 2 {
		return fmt.Errorf("%s signal-maker funnel count is not two", cell.ID())
	}
	expected := map[uint64]bool{3: true, 5: true}
	if cell.Composition == "M2" {
		expected = map[uint64]bool{2: true, 4: true}
	}
	for _, funnel := range funnels {
		if !expected[funnel.ClientID] || funnel.ScheduledDecisions < 0 ||
			funnel.ShadowGain2Changes < 0 || funnel.ActualShiftedTargets < 0 ||
			funnel.ShadowRiskFeasible < 0 || funnel.ShadowPlaceSent < 0 ||
			funnel.ShadowAccepted < 0 || funnel.ShadowRejected < 0 ||
			funnel.ShadowRestingObserved < 0 || funnel.ShadowBestObserved < 0 ||
			funnel.ShadowFilled < 0 || funnel.ShadowGain2Changes > funnel.BaseQuoteUsable ||
			funnel.ShadowRiskFeasible > funnel.ShadowGain2Changes ||
			funnel.ShadowPlaceSent > funnel.ShadowRiskFeasible ||
			funnel.ShadowRestingObserved > funnel.ShadowAccepted ||
			funnel.ShadowBestObserved > funnel.ShadowRestingObserved ||
			funnel.ShadowFilled > funnel.ShadowAccepted ||
			cell.SignalGainBps == 0 && funnel.ActualShiftedTargets != 0 ||
			cell.SignalGainBps == 2 && funnel.ActualShiftedTargets > funnel.ShadowGain2Changes {
			return fmt.Errorf("%s signal funnel is inconsistent", cell.ID())
		}
		delete(expected, funnel.ClientID)
	}
	if len(expected) != 0 {
		return fmt.Errorf("%s signal-maker identity is incomplete", cell.ID())
	}
	return nil
}
