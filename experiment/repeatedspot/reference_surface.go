package repeatedspot

import (
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"

	"exchange_sim/experiment/executionpilot"
)

type E0ReferenceCellSummary struct {
	Cell                        E0Cell              `json:"cell"`
	ResultSHA256                string              `json:"result_sha256"`
	DiagnosticSHA256            string              `json:"diagnostic_sha256"`
	ManifestSHA256              string              `json:"manifest_sha256"`
	EvidenceFileSHA256          string              `json:"evidence_file_sha256"`
	ExecutionHash               string              `json:"execution_hash"`
	TerminalMarkStatus          string              `json:"terminal_mark_status"`
	MakerGainSumQuoteAtoms      *string             `json:"maker_gain_sum_quote_atoms"`
	MeasurementBookDurations    BookStateDurations  `json:"measurement_book_durations"`
	PublicRestingDepth          RestingDepthSummary `json:"public_resting_depth"`
	MakerOnlyRestingDepth       RestingDepthSummary `json:"maker_only_resting_depth"`
	WindowSpreadPriceUnitNanos  string              `json:"window_spread_price_unit_ns"`
	FirstWindowTwoSidedMidPrice *int64              `json:"first_window_two_sided_mid_price_units"`
	LastWindowTwoSidedMidPrice  *int64              `json:"last_window_two_sided_mid_price_units"`
	WindowMidDriftPriceUnits    *int64              `json:"window_two_sided_mid_drift_price_units"`
	TradeCount                  int                 `json:"trade_count"`
	ShadowEligibleDecisions     int64               `json:"shadow_eligible_decisions"`
	ShadowWaitingDecisions      int64               `json:"shadow_waiting_decisions"`
	ShadowPlacementEvaluations  int64               `json:"shadow_placement_evaluations"`
	CachedReferenceDecisions    int64               `json:"cached_reference_decisions"`
	CachedReferenceAgeSumNanos  int64               `json:"cached_reference_age_sum_ns"`
	CachedReferenceAgeMaxNanos  int64               `json:"cached_reference_age_max_ns"`
	RunResourceSHA256           string              `json:"run_resource_sha256,omitempty"`
	AnalysisResourceSHA256      string              `json:"analysis_resource_sha256,omitempty"`
	PeakRunAllocatedBytes       uint64              `json:"peak_run_allocated_bytes,omitempty"`
	PeakRunCgroupMemoryBytes    uint64              `json:"peak_run_cgroup_memory_bytes,omitempty"`
	PeakAnalysisCgroupBytes     uint64              `json:"peak_analysis_cgroup_memory_bytes,omitempty"`
}

type E0ReferenceSeedPair struct {
	Seed                   int64 `json:"seed"`
	OnTwoSidedNanos        int64 `json:"on_two_sided_ns"`
	OffTwoSidedNanos       int64 `json:"off_two_sided_ns"`
	OnMinusOffNanos        int64 `json:"on_minus_off_ns"`
	WindowDenominatorNanos int64 `json:"window_denominator_ns"`
}

type E0ReferenceSurface struct {
	SchemaVersion            int                         `json:"schema_version"`
	StudyID                  string                      `json:"study_id"`
	StudyStage               string                      `json:"study_stage"`
	ExecutionIdentity        executionpilot.Identity     `json:"execution_identity"`
	DiagnosticSourceCommit   string                      `json:"diagnostic_source_commit"`
	DiagnosticBinarySHA256   string                      `json:"diagnostic_binary_sha256"`
	ExpectedEconomicCells    int                         `json:"expected_economic_cells"`
	ValidEconomicCells       int                         `json:"valid_economic_cells"`
	TechnicalControls        []E0TechnicalControlSummary `json:"technical_controls"`
	PrimaryContrastStatus    string                      `json:"primary_contrast_status"`
	PrimaryPairs             []E0ReferenceSeedPair       `json:"primary_pairs"`
	PrimaryMedianDeltaNanos  int64                       `json:"primary_median_delta_ns"`
	PrimaryMinimumDeltaNanos int64                       `json:"primary_minimum_delta_ns"`
	PrimaryMaximumDeltaNanos int64                       `json:"primary_maximum_delta_ns"`
	TreatmentPAGainStatus    string                      `json:"treatment_p_vs_a_gain_status"`
	TreatmentPAGainPairs     []E0PrimaryPair             `json:"treatment_p_vs_a_gain_pairs"`
	ControlPAGainStatus      string                      `json:"control_p_vs_a_gain_status"`
	ControlPAGainPairs       []E0PrimaryPair             `json:"control_p_vs_a_gain_pairs"`
	Cells                    []E0ReferenceCellSummary    `json:"cells"`
}

// SummarizeE0LocalReference requires the complete assigned matrix. It never
// substitutes the actor's cached quote reference for a missing terminal mark.
func SummarizeE0LocalReference(inputs []E0CellArtifacts) (E0ReferenceSurface, error) {
	registered := E0LocalReferenceDevelopmentCells()
	if len(inputs) != len(registered) {
		return E0ReferenceSurface{}, fmt.Errorf("repeated spot: expected %d ME-015 cells, found %d", len(registered), len(inputs))
	}
	byCell := make(map[E0Cell]E0CellArtifacts, len(inputs))
	for _, input := range inputs {
		if _, duplicate := byCell[input.Result.Cell]; duplicate {
			return E0ReferenceSurface{}, errors.New("repeated spot: duplicate ME-015 cell")
		}
		byCell[input.Result.Cell] = input
	}
	window := E0MeasurementWindow()
	surface := E0ReferenceSurface{SchemaVersion: 1, StudyID: "ME-015", StudyStage: "DEVELOPMENT",
		ExpectedEconomicCells: len(registered), PrimaryContrastStatus: "ESTIMATED_DEVELOPMENT_ONLY",
		TreatmentPAGainStatus: "NOT_IDENTIFIED", ControlPAGainStatus: "NOT_IDENTIFIED"}
	gains := make(map[E0Cell]*big.Int, len(registered))
	for _, cell := range registered {
		input, present := byCell[cell]
		if !present || input.Result.EconomicReconstruction == nil {
			return E0ReferenceSurface{}, fmt.Errorf("repeated spot: missing strict result for %s", cell.ID())
		}
		result, diagnostic := input.Result, input.Diagnostic
		replay := result.EconomicReconstruction
		if surface.ExecutionIdentity == (executionpilot.Identity{}) {
			surface.ExecutionIdentity = result.Identity
		}
		if result.Identity != surface.ExecutionIdentity || result.Cell != cell ||
			result.ManifestFileSHA256 == "" || result.EvidenceFileSHA256 == "" ||
			input.ResultSHA256 == "" || input.DiagnosticSHA256 == "" ||
			replay.Evidence != diagnostic.Evidence || replay.InformationAudit == nil || !replay.InformationAudit.Valid ||
			replay.MeasurementWindow != window || diagnostic.MeasurementStartNanos != window.StartAt ||
			diagnostic.MeasurementEndNanos != window.EndAt || diagnostic.WorldEndNanos != window.EndAt ||
			diagnostic.ShadowReferenceMaxAgeNanos != int64(E0LocalReferenceMaxAge) ||
			diagnostic.TradeCount != int64(replay.TradeCount) ||
			!sameBookDurations(diagnostic.BookDurations, replay.Market) ||
			!sameWindowedPublicDepth(diagnostic.MeasurementBookDurations, replay.PublicWindowDepth) ||
			!validWindowPriceSummary(diagnostic) ||
			replay.MakerWindowDepth.WindowNanos != window.EndAt-window.StartAt ||
			replay.MakerWindowDepth.BidPresentNanos < 0 || replay.MakerWindowDepth.AskPresentNanos < 0 ||
			replay.MakerWindowDepth.TwoSidedNanos < 0 ||
			replay.MakerWindowDepth.BidPresentNanos > replay.PublicWindowDepth.BidPresentNanos ||
			replay.MakerWindowDepth.AskPresentNanos > replay.PublicWindowDepth.AskPresentNanos ||
			replay.MakerWindowDepth.TwoSidedNanos > replay.PublicWindowDepth.TwoSidedNanos {
			return E0ReferenceSurface{}, fmt.Errorf("repeated spot: inconsistent ME-015 evidence for %s", cell.ID())
		}
		summary := E0ReferenceCellSummary{Cell: cell, ResultSHA256: input.ResultSHA256,
			DiagnosticSHA256: input.DiagnosticSHA256, ManifestSHA256: result.ManifestFileSHA256,
			EvidenceFileSHA256: result.EvidenceFileSHA256, ExecutionHash: replay.Evidence.ExecutionHash,
			TerminalMarkStatus: replay.TerminalMarkStatus, MeasurementBookDurations: diagnostic.MeasurementBookDurations,
			PublicRestingDepth: replay.PublicWindowDepth, MakerOnlyRestingDepth: replay.MakerWindowDepth,
			WindowSpreadPriceUnitNanos:  diagnostic.MeasurementSpreadPriceUnitNanos,
			FirstWindowTwoSidedMidPrice: diagnostic.FirstWindowTwoSidedMidPrice,
			LastWindowTwoSidedMidPrice:  diagnostic.LastWindowTwoSidedMidPrice,
			TradeCount:                  replay.TradeCount}
		if diagnostic.FirstWindowTwoSidedMidPrice != nil {
			drift := *diagnostic.LastWindowTwoSidedMidPrice - *diagnostic.FirstWindowTwoSidedMidPrice
			summary.WindowMidDriftPriceUnits = &drift
		}
		makerIDs := make(map[uint64]struct{}, 4)
		gainSum := new(big.Int)
		anyGain, everyGain := false, true
		for _, account := range replay.Accounts {
			if !strings.HasPrefix(account.Role, "maker_slot_") {
				continue
			}
			if _, duplicate := makerIDs[account.ActorID]; duplicate || account.MakerEnvelope == nil ||
				account.MakerEnvelope.WindowNanos != window.EndAt-window.StartAt ||
				account.RestingDepth.WindowNanos != window.EndAt-window.StartAt {
				return E0ReferenceSurface{}, fmt.Errorf("repeated spot: incomplete ME-015 maker account for %s", cell.ID())
			}
			makerIDs[account.ActorID] = struct{}{}
			if account.BenchmarkGain == nil {
				everyGain = false
			} else {
				anyGain = true
				gainSum.Add(gainSum, big.NewInt(*account.BenchmarkGain))
			}
		}
		if len(makerIDs) != 4 || len(diagnostic.Makers) != 4 {
			return E0ReferenceSurface{}, fmt.Errorf("repeated spot: incomplete ME-015 maker roster for %s", cell.ID())
		}
		seenDiagnostic := make(map[uint64]struct{}, 4)
		for _, maker := range diagnostic.Makers {
			if _, present := makerIDs[maker.ActorID]; !present {
				return E0ReferenceSurface{}, fmt.Errorf("repeated spot: ME-015 diagnostic maker mismatch for %s", cell.ID())
			}
			if _, duplicate := seenDiagnostic[maker.ActorID]; duplicate {
				return E0ReferenceSurface{}, fmt.Errorf("repeated spot: duplicate ME-015 diagnostic maker for %s", cell.ID())
			}
			seenDiagnostic[maker.ActorID] = struct{}{}
			summary.ShadowEligibleDecisions += maker.ShadowEligibleDecisions
			summary.ShadowWaitingDecisions += maker.ShadowWaitingDecisions
			summary.ShadowPlacementEvaluations += maker.ShadowEvaluations
			summary.CachedReferenceDecisions += maker.CachedReferenceDecisions
			summary.CachedReferenceAgeSumNanos += maker.CachedReferenceAgeSum
			summary.CachedReferenceAgeMaxNanos = max(summary.CachedReferenceAgeMaxNanos, maker.CachedReferenceAgeMax)
		}
		if summary.ShadowWaitingDecisions+summary.ShadowPlacementEvaluations > summary.ShadowEligibleDecisions ||
			cell.ReferenceMode == "OFF" && summary.CachedReferenceDecisions != 0 ||
			summary.CachedReferenceDecisions > summary.ShadowEligibleDecisions {
			return E0ReferenceSurface{}, fmt.Errorf("repeated spot: invalid ME-015 opportunity counts for %s", cell.ID())
		}
		switch replay.TerminalMarkStatus {
		case "AVAILABLE_TWO_SIDED_MID":
			if replay.TerminalMidQuote == nil || !everyGain || *replay.TerminalMidQuote <= 0 {
				return E0ReferenceSurface{}, fmt.Errorf("repeated spot: ME-015 available mark lacks maker gains for %s", cell.ID())
			}
			value := gainSum.String()
			summary.MakerGainSumQuoteAtoms = &value
			gains[cell] = gainSum
		case "UNAVAILABLE_ONE_SIDED_OR_EMPTY":
			if replay.TerminalMidQuote != nil || anyGain {
				return E0ReferenceSurface{}, fmt.Errorf("repeated spot: ME-015 unavailable mark has maker gain for %s", cell.ID())
			}
		default:
			return E0ReferenceSurface{}, fmt.Errorf("repeated spot: unsupported ME-015 mark status for %s", cell.ID())
		}
		surface.Cells = append(surface.Cells, summary)
	}
	for _, seed := range e0LocalReferenceSeeds {
		on := byCell[E0Cell{Composition: "P", QuoteQty: e0QuoteSmall, Seed: seed, ReferenceMode: "ON"}].Diagnostic.MeasurementBookDurations.TwoSidedNanos
		off := byCell[E0Cell{Composition: "P", QuoteQty: e0QuoteSmall, Seed: seed, ReferenceMode: "OFF"}].Diagnostic.MeasurementBookDurations.TwoSidedNanos
		surface.PrimaryPairs = append(surface.PrimaryPairs, E0ReferenceSeedPair{Seed: seed, OnTwoSidedNanos: on,
			OffTwoSidedNanos: off, OnMinusOffNanos: on - off, WindowDenominatorNanos: window.EndAt - window.StartAt})
	}
	deltas := []int64{surface.PrimaryPairs[0].OnMinusOffNanos, surface.PrimaryPairs[1].OnMinusOffNanos,
		surface.PrimaryPairs[2].OnMinusOffNanos}
	slices.Sort(deltas)
	surface.PrimaryMinimumDeltaNanos, surface.PrimaryMedianDeltaNanos, surface.PrimaryMaximumDeltaNanos = deltas[0], deltas[1], deltas[2]
	treatmentPairs, availableTreatmentPairs := referenceMakerGainPairs("ON", byCell, gains)
	surface.TreatmentPAGainPairs = treatmentPairs
	if availableTreatmentPairs == len(e0LocalReferenceSeeds) {
		surface.TreatmentPAGainStatus = "ESTIMATED_DEVELOPMENT_ONLY"
	}
	controlPairs, availableControlPairs := referenceMakerGainPairs("OFF", byCell, gains)
	surface.ControlPAGainPairs = controlPairs
	if availableControlPairs == len(e0LocalReferenceSeeds) {
		surface.ControlPAGainStatus = "ESTIMATED_DEVELOPMENT_ONLY"
	}
	surface.ValidEconomicCells = len(surface.Cells)
	return surface, nil
}

func referenceMakerGainPairs(mode string, byCell map[E0Cell]E0CellArtifacts,
	gains map[E0Cell]*big.Int) ([]E0PrimaryPair, int) {
	pairs := make([]E0PrimaryPair, 0, len(e0LocalReferenceSeeds))
	available := 0
	for _, seed := range e0LocalReferenceSeeds {
		pureCell := E0Cell{Composition: "P", QuoteQty: e0QuoteSmall, Seed: seed, ReferenceMode: mode}
		asCell := E0Cell{Composition: "A", QuoteQty: e0QuoteSmall, Seed: seed, ReferenceMode: mode}
		pure, as := byCell[pureCell].Result.EconomicReconstruction, byCell[asCell].Result.EconomicReconstruction
		pair := E0PrimaryPair{Seed: seed, PureTerminalMarkStatus: pure.TerminalMarkStatus,
			ASTerminalMarkStatus: as.TerminalMarkStatus, DeltaDenominatorMakerCount: 4}
		if pureGain, present := gains[pureCell]; present {
			if asGain, present := gains[asCell]; present {
				value := new(big.Int).Sub(asGain, pureGain).String()
				pair.DeltaNumeratorQuoteAtoms = &value
				available++
			}
		}
		pairs = append(pairs, pair)
	}
	return pairs, available
}

func validWindowPriceSummary(diagnostic E0LiquidityDiagnostic) bool {
	spread, parsed := new(big.Int).SetString(diagnostic.MeasurementSpreadPriceUnitNanos, 10)
	if !parsed || spread.Sign() < 0 {
		return false
	}
	if diagnostic.MeasurementBookDurations.TwoSidedNanos == 0 {
		return spread.Sign() == 0 && diagnostic.FirstWindowTwoSidedMidPrice == nil &&
			diagnostic.LastWindowTwoSidedMidPrice == nil
	}
	return spread.Sign() > 0 && diagnostic.FirstWindowTwoSidedMidPrice != nil &&
		diagnostic.LastWindowTwoSidedMidPrice != nil && *diagnostic.FirstWindowTwoSidedMidPrice > 0 &&
		*diagnostic.LastWindowTwoSidedMidPrice > 0
}

func sameWindowedPublicDepth(book BookStateDurations, depth RestingDepthSummary) bool {
	return book.HorizonNanos == depth.WindowNanos && book.HorizonNanos == E0MeasurementWindow().EndAt-E0MeasurementWindow().StartAt &&
		book.TwoSidedNanos >= 0 && book.BidOnlyNanos >= 0 && book.AskOnlyNanos >= 0 && book.EmptyNanos >= 0 &&
		depth.BidPresentNanos >= 0 && depth.AskPresentNanos >= 0 && depth.TwoSidedNanos >= 0 &&
		book.TwoSidedNanos == depth.TwoSidedNanos &&
		book.TwoSidedNanos+book.BidOnlyNanos == depth.BidPresentNanos &&
		book.TwoSidedNanos+book.AskOnlyNanos == depth.AskPresentNanos &&
		book.TwoSidedNanos+book.BidOnlyNanos+book.AskOnlyNanos+book.EmptyNanos == book.HorizonNanos
}
