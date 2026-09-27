package repeatedspot

import (
	"errors"
	"fmt"
	"math/big"
	"strings"

	"exchange_sim/experiment/executionpilot"
)

type E0CellArtifacts struct {
	Result           E0AnalyzedResult
	Diagnostic       E0LiquidityDiagnostic
	ResultSHA256     string
	DiagnosticSHA256 string
}

type E0DevelopmentCellSummary struct {
	Cell                      E0Cell             `json:"cell"`
	ResultSHA256              string             `json:"result_sha256"`
	DiagnosticSHA256          string             `json:"diagnostic_sha256"`
	ManifestSHA256            string             `json:"manifest_sha256"`
	EvidenceFileSHA256        string             `json:"evidence_file_sha256"`
	ExecutionHash             string             `json:"execution_hash"`
	TerminalMarkStatus        string             `json:"terminal_mark_status"`
	MakerGainSumQuoteAtoms    *string            `json:"maker_gain_sum_quote_atoms"`
	BookDurations             BookStateDurations `json:"book_durations"`
	FirstPermanentEmptyNanos  *int64             `json:"first_permanent_empty_ns"`
	LastTwoSidedEndNanos      *int64             `json:"last_two_sided_end_ns"`
	TradeCount                int                `json:"trade_count"`
	VenueFeeRevenueQuoteAtoms int64              `json:"venue_fee_revenue_quote_atoms"`
	MakerPlacementEvaluations int64              `json:"maker_placement_evaluations_in_window"`
	MakerNoUsableDecisions    int64              `json:"maker_no_usable_decisions_in_window"`
	MakerRestingSideNanos     int64              `json:"maker_resting_side_actor_nanos_in_window"`
	RunResourceSHA256         string             `json:"run_resource_sha256,omitempty"`
	AnalysisResourceSHA256    string             `json:"analysis_resource_sha256,omitempty"`
	PeakRunAllocatedBytes     uint64             `json:"peak_run_allocated_bytes,omitempty"`
	PeakRunCgroupMemoryBytes  uint64             `json:"peak_run_cgroup_memory_bytes,omitempty"`
	PeakAnalysisCgroupBytes   uint64             `json:"peak_analysis_cgroup_memory_bytes,omitempty"`
}

type E0PrimaryPair struct {
	Seed                       int64   `json:"seed"`
	PureTerminalMarkStatus     string  `json:"pure_terminal_mark_status"`
	ASTerminalMarkStatus       string  `json:"as_terminal_mark_status"`
	DeltaNumeratorQuoteAtoms   *string `json:"delta_numerator_quote_atoms"`
	DeltaDenominatorMakerCount int     `json:"delta_denominator_maker_count"`
}

type E0TechnicalControlSummary struct {
	Workers                 int    `json:"go_workers"`
	ResultSHA256            string `json:"result_sha256"`
	ManifestSHA256          string `json:"manifest_sha256"`
	RunResourceSHA256       string `json:"run_resource_sha256"`
	AnalysisResourceSHA256  string `json:"analysis_resource_sha256"`
	BitIdenticalToReference bool   `json:"bit_identical_to_reference"`
}

type E0DevelopmentSurface struct {
	SchemaVersion                        int                         `json:"schema_version"`
	StudyID                              string                      `json:"study_id"`
	StudyStage                           string                      `json:"study_stage"`
	ExecutionIdentity                    executionpilot.Identity     `json:"execution_identity"`
	DiagnosticSourceCommit               string                      `json:"diagnostic_source_commit"`
	DiagnosticBinarySHA256               string                      `json:"diagnostic_binary_sha256"`
	ExpectedEconomicCells                int                         `json:"expected_economic_cells"`
	ValidEconomicCells                   int                         `json:"valid_economic_cells"`
	TechnicalControls                    []E0TechnicalControlSummary `json:"technical_controls"`
	PrimaryContrastStatus                string                      `json:"primary_contrast_status"`
	PrimaryAvailablePairs                int                         `json:"primary_available_pairs"`
	PrimaryPairs                         []E0PrimaryPair             `json:"primary_pairs"`
	AllCellsTerminalMarkUnavailable      bool                        `json:"all_cells_terminal_mark_unavailable"`
	AllCellsPermanentlyEmptyBeforeWindow bool                        `json:"all_cells_permanently_empty_before_window"`
	Cells                                []E0DevelopmentCellSummary  `json:"cells"`
}

// SummarizeE0Development applies the registered missing-mark rule to the
// complete 24-cell matrix. Economic failures remain cells, not exclusions.
func SummarizeE0Development(inputs []E0CellArtifacts) (E0DevelopmentSurface, error) {
	registered := E0DevelopmentCells()
	if len(inputs) != len(registered) {
		return E0DevelopmentSurface{}, fmt.Errorf("repeated spot: expected %d development cells, found %d", len(registered), len(inputs))
	}
	byCell := make(map[E0Cell]E0CellArtifacts, len(inputs))
	for _, input := range inputs {
		if _, duplicate := byCell[input.Result.Cell]; duplicate {
			return E0DevelopmentSurface{}, errors.New("repeated spot: duplicate development cell")
		}
		byCell[input.Result.Cell] = input
	}
	surface := E0DevelopmentSurface{SchemaVersion: 1, StudyID: "ME-013", StudyStage: "DEVELOPMENT",
		ExpectedEconomicCells: len(registered), AllCellsTerminalMarkUnavailable: true,
		AllCellsPermanentlyEmptyBeforeWindow: true}
	classGains := make(map[E0Cell]*big.Int, len(registered))
	for _, cell := range registered {
		input, present := byCell[cell]
		if !present || input.Result.EconomicReconstruction == nil {
			return E0DevelopmentSurface{}, fmt.Errorf("repeated spot: missing accepted result for %s", cell.ID())
		}
		result := input.Result
		replay := result.EconomicReconstruction
		if surface.ExecutionIdentity == (executionpilot.Identity{}) {
			surface.ExecutionIdentity = result.Identity
		}
		if result.Identity != surface.ExecutionIdentity || result.Cell != cell ||
			result.EvidenceFileSHA256 == "" || result.ManifestFileSHA256 == "" ||
			input.ResultSHA256 == "" || input.DiagnosticSHA256 == "" ||
			replay.Evidence != input.Diagnostic.Evidence ||
			replay.InformationAudit == nil || !replay.InformationAudit.Valid ||
			replay.MeasurementWindow != E0MeasurementWindow() ||
			input.Diagnostic.MeasurementStartNanos != replay.MeasurementWindow.StartAt ||
			input.Diagnostic.WorldEndNanos != replay.MeasurementWindow.EndAt ||
			input.Diagnostic.TradeCount != int64(replay.TradeCount) ||
			!sameBookDurations(input.Diagnostic.BookDurations, replay.Market) {
			return E0DevelopmentSurface{}, fmt.Errorf("repeated spot: inconsistent accepted evidence for %s", cell.ID())
		}
		summary := E0DevelopmentCellSummary{Cell: cell, ResultSHA256: input.ResultSHA256,
			DiagnosticSHA256: input.DiagnosticSHA256, ManifestSHA256: result.ManifestFileSHA256,
			EvidenceFileSHA256: result.EvidenceFileSHA256, ExecutionHash: replay.Evidence.ExecutionHash,
			TerminalMarkStatus: replay.TerminalMarkStatus, BookDurations: input.Diagnostic.BookDurations,
			FirstPermanentEmptyNanos: input.Diagnostic.FirstPermanentEmptyNanos,
			LastTwoSidedEndNanos:     input.Diagnostic.LastTwoSidedEndNanos,
			TradeCount:               replay.TradeCount, VenueFeeRevenueQuoteAtoms: replay.VenueFeeRevenue["USD"]}
		gainSum := new(big.Int)
		makers := make(map[uint64]struct{}, 4)
		gainAvailable := true
		anyGain := false
		for _, account := range replay.Accounts {
			if !strings.HasPrefix(account.Role, "maker_slot_") {
				continue
			}
			makers[account.ActorID] = struct{}{}
			if account.MakerEnvelope == nil || account.MakerEnvelope.WindowNanos != replay.MeasurementWindow.EndAt-replay.MeasurementWindow.StartAt ||
				account.RestingDepth.WindowNanos != replay.MeasurementWindow.EndAt-replay.MeasurementWindow.StartAt {
				return E0DevelopmentSurface{}, fmt.Errorf("repeated spot: maker window unavailable for %s", cell.ID())
			}
			summary.MakerPlacementEvaluations += account.MakerEnvelope.PlacementEvaluationDecisions
			summary.MakerRestingSideNanos += account.RestingDepth.BidPresentNanos + account.RestingDepth.AskPresentNanos
			if account.BenchmarkGain == nil {
				gainAvailable = false
			} else {
				anyGain = true
				gainSum.Add(gainSum, big.NewInt(*account.BenchmarkGain))
			}
		}
		if len(makers) != 4 || len(input.Diagnostic.Makers) != 4 {
			return E0DevelopmentSurface{}, fmt.Errorf("repeated spot: incomplete maker roster for %s", cell.ID())
		}
		diagnosticMakers := make(map[uint64]struct{}, 4)
		for _, maker := range input.Diagnostic.Makers {
			if _, present := makers[maker.ActorID]; !present {
				return E0DevelopmentSurface{}, fmt.Errorf("repeated spot: diagnostic maker mismatch for %s", cell.ID())
			}
			if _, duplicate := diagnosticMakers[maker.ActorID]; duplicate {
				return E0DevelopmentSurface{}, fmt.Errorf("repeated spot: duplicate diagnostic maker for %s", cell.ID())
			}
			diagnosticMakers[maker.ActorID] = struct{}{}
			summary.MakerNoUsableDecisions += maker.DuringMeasurement["no_usable_quote"]
		}
		switch replay.TerminalMarkStatus {
		case "AVAILABLE_TWO_SIDED_MID":
			if replay.TerminalMidQuote == nil || !gainAvailable || input.Diagnostic.FirstPermanentEmptyNanos != nil {
				return E0DevelopmentSurface{}, fmt.Errorf("repeated spot: available mark lacks maker gain for %s", cell.ID())
			}
			value := gainSum.String()
			summary.MakerGainSumQuoteAtoms = &value
			classGains[cell] = gainSum
			surface.AllCellsTerminalMarkUnavailable = false
		case "UNAVAILABLE_ONE_SIDED_OR_EMPTY":
			if replay.TerminalMidQuote != nil || anyGain {
				return E0DevelopmentSurface{}, fmt.Errorf("repeated spot: unavailable mark has maker gain for %s", cell.ID())
			}
		default:
			return E0DevelopmentSurface{}, fmt.Errorf("repeated spot: unsupported terminal mark status for %s", cell.ID())
		}
		if input.Diagnostic.FirstPermanentEmptyNanos == nil ||
			*input.Diagnostic.FirstPermanentEmptyNanos >= replay.MeasurementWindow.StartAt {
			surface.AllCellsPermanentlyEmptyBeforeWindow = false
		}
		surface.Cells = append(surface.Cells, summary)
	}
	for _, seed := range e0DevelopmentSeeds {
		pureCell := E0Cell{Composition: "P", QuoteQty: e0QuoteSmall, Seed: seed}
		asCell := E0Cell{Composition: "A", QuoteQty: e0QuoteSmall, Seed: seed}
		pure := byCell[pureCell].Result.EconomicReconstruction
		as := byCell[asCell].Result.EconomicReconstruction
		pair := E0PrimaryPair{Seed: seed, PureTerminalMarkStatus: pure.TerminalMarkStatus,
			ASTerminalMarkStatus: as.TerminalMarkStatus, DeltaDenominatorMakerCount: 4}
		if pureGain, present := classGains[pureCell]; present {
			if asGain, present := classGains[asCell]; present {
				value := new(big.Int).Sub(asGain, pureGain).String()
				pair.DeltaNumeratorQuoteAtoms = &value
				surface.PrimaryAvailablePairs++
			}
		}
		surface.PrimaryPairs = append(surface.PrimaryPairs, pair)
	}
	surface.PrimaryContrastStatus = "NOT_IDENTIFIED"
	if surface.PrimaryAvailablePairs == len(e0DevelopmentSeeds) {
		surface.PrimaryContrastStatus = "ESTIMATED_DEVELOPMENT_ONLY"
	}
	surface.ValidEconomicCells = len(surface.Cells)
	return surface, nil
}

func sameBookDurations(actual BookStateDurations, expected MarketSummary) bool {
	return actual.HorizonNanos == expected.HorizonNanos &&
		actual.TwoSidedNanos == expected.TwoSidedNanos && actual.BidOnlyNanos == expected.BidOnlyNanos &&
		actual.AskOnlyNanos == expected.AskOnlyNanos && actual.EmptyNanos == expected.EmptyNanos &&
		actual.TwoSidedNanos+actual.BidOnlyNanos+actual.AskOnlyNanos+actual.EmptyNanos == actual.HorizonNanos
}
