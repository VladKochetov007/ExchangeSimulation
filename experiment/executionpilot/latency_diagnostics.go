package executionpilot

import (
	"errors"
	"fmt"
	"slices"
)

type LatencyArmSummary struct {
	Arm                                     string    `json:"arm"`
	TargetQty                               int64     `json:"target_qty"`
	Seeds                                   []int64   `json:"seeds"`
	FilledFractions                         []float64 `json:"filled_fractions"`
	MedianFilledFraction                    float64   `json:"median_filled_fraction"`
	MinFilledFraction                       float64   `json:"min_filled_fraction"`
	MaxFilledFraction                       float64   `json:"max_filled_fraction"`
	FullCompletionCount                     int       `json:"full_completion_count"`
	TerminalMarkDefinedCount                int       `json:"terminal_mark_defined_count"`
	SelectedQualifyingCount                 int       `json:"selected_qualifying_count"`
	CompleteSampledOpportunityCount         int       `json:"complete_sampled_opportunity_count"`
	ActionTimingObservedCount               int       `json:"action_timing_observed_count"`
	MedianSelectedPublicationToArrivalNanos int64     `json:"median_selected_publication_to_arrival_nanos,omitempty"`
	MedianDecisionToArrivalNanos            int64     `json:"median_decision_to_arrival_nanos,omitempty"`
}

type LatencyTimingDiagnostics struct {
	SchemaVersion                     int                 `json:"schema_version"`
	SurfaceFileSHA256                 string              `json:"surface_file_sha256"`
	ExecutionSource                   string              `json:"execution_source"`
	AnalysisSource                    string              `json:"analysis_source"`
	AllDecisionsAtOneSecond           bool                `json:"all_decisions_at_one_second"`
	ProcessingMatchedPairs            int                 `json:"processing_matched_pairs"`
	ProcessingChangedOrderArrival     int                 `json:"processing_changed_order_arrival"`
	ProcessingChangedSelectedSnapshot int                 `json:"processing_changed_selected_snapshot"`
	ProcessingChangedFilledQty        int                 `json:"processing_changed_filled_qty"`
	NetworkMatchedPairs               int                 `json:"network_matched_pairs"`
	NetworkChangedOrderArrival        int                 `json:"network_changed_order_arrival"`
	NetworkChangedFilledQty           int                 `json:"network_changed_filled_qty"`
	ArmSummaries                      []LatencyArmSummary `json:"arm_summaries"`
}

func DiagnoseLatencySurface(surfacePath string) (LatencyTimingDiagnostics, error) {
	surface, err := decodeStrictFile[LatencySurface](surfacePath)
	if err != nil {
		return LatencyTimingDiagnostics{}, err
	}
	if surface.SchemaVersion != 1 || surface.AssignedWorlds != 24 || surface.ValidWorlds != 24 || len(surface.Cells) != 24 ||
		len(surface.PairedContrasts) != 6 {
		return LatencyTimingDiagnostics{}, errors.New("latency pilot: incomplete input surface")
	}
	digest, err := fileSHA256(surfacePath)
	if err != nil {
		return LatencyTimingDiagnostics{}, err
	}
	diagnostic := LatencyTimingDiagnostics{SchemaVersion: 1, SurfaceFileSHA256: digest,
		ExecutionSource: surface.ExecutionIdentity.SourceCommit, AnalysisSource: surface.AggregationCommit,
		AllDecisionsAtOneSecond: true}
	byID := map[string]LatencyCellResult{}
	for _, result := range surface.Cells {
		expectedID, err := latencyCellID(result.Cell)
		if err != nil || expectedID != result.ID || result.Cell.TargetQty <= 0 ||
			result.Outcome.FilledQty < 0 || result.Outcome.FilledQty > result.Cell.TargetQty ||
			result.FilledFraction != float64(result.Outcome.FilledQty)/float64(result.Cell.TargetQty) {
			return LatencyTimingDiagnostics{}, errors.New("latency pilot: inconsistent diagnostic cell and filled fraction")
		}
		if _, exists := byID[result.ID]; exists {
			return LatencyTimingDiagnostics{}, errors.New("latency pilot: duplicate diagnostic cell")
		}
		byID[result.ID] = result
		if result.Outcome.DecisionAt != 1_000_000_000 {
			diagnostic.AllDecisionsAtOneSecond = false
		}
	}
	for _, target := range []int64{50_000_000, 500_000_000} {
		for _, deployment := range []struct {
			arm                           string
			networkNanos, processingNanos int64
		}{
			{"F-F", 1_000_000, 0}, {"F-S", 1_000_000, 120_000_000},
			{"S-F", 90_000_000, 0}, {"S-S", 90_000_000, 120_000_000},
		} {
			summary := LatencyArmSummary{Arm: deployment.arm, TargetQty: target}
			var selectedAges, outbound []int64
			for _, seed := range []int64{12001, 12011, 12017} {
				cell := LatencyCell{NetworkLatencyNanos: deployment.networkNanos, ProcessingDelayNanos: deployment.processingNanos, TargetQty: target, Seed: seed}
				id, _ := latencyCellID(cell)
				result, present := byID[id]
				if !present {
					return LatencyTimingDiagnostics{}, fmt.Errorf("latency pilot: diagnostic cell %s missing", id)
				}
				summary.Seeds = append(summary.Seeds, seed)
				summary.FilledFractions = append(summary.FilledFractions, result.FilledFraction)
				if result.Outcome.Status == OutcomeFullyFilled {
					summary.FullCompletionCount++
				}
				if result.Outcome.TerminalMarkAvailable {
					summary.TerminalMarkDefinedCount++
				}
				if result.Outcome.SelectedOpportunity != nil {
					if result.Outcome.SelectedOpportunity.QualifyingAtPublication {
						summary.SelectedQualifyingCount++
					}
					if result.Outcome.SelectedOpportunity.Complete {
						summary.CompleteSampledOpportunityCount++
					}
				}
				if result.Outcome.ActionTiming != nil {
					summary.ActionTimingObservedCount++
					selectedAges = append(selectedAges, result.Outcome.ActionTiming.PublicationToVenueArrivalNanos)
					outbound = append(outbound, result.Outcome.ActionTiming.DecisionToVenueArrivalNanos)
				}
			}
			orderedFractions := slices.Clone(summary.FilledFractions)
			slices.Sort(orderedFractions)
			summary.MinFilledFraction, summary.MedianFilledFraction, summary.MaxFilledFraction = orderedFractions[0], orderedFractions[1], orderedFractions[2]
			if len(selectedAges) > 0 {
				slices.Sort(selectedAges)
				slices.Sort(outbound)
				summary.MedianSelectedPublicationToArrivalNanos = selectedAges[len(selectedAges)/2]
				summary.MedianDecisionToArrivalNanos = outbound[len(outbound)/2]
			}
			diagnostic.ArmSummaries = append(diagnostic.ArmSummaries, summary)
		}
		for _, seed := range []int64{12001, 12011, 12017} {
			get := func(network, processing int64) (LatencyCellResult, error) {
				id, _ := latencyCellID(LatencyCell{network, processing, target, seed})
				result, present := byID[id]
				if !present {
					return LatencyCellResult{}, fmt.Errorf("latency pilot: missing pair member %s", id)
				}
				return result, nil
			}
			ff, err := get(1_000_000, 0)
			if err != nil {
				return LatencyTimingDiagnostics{}, err
			}
			fs, err := get(1_000_000, 120_000_000)
			if err != nil {
				return LatencyTimingDiagnostics{}, err
			}
			sf, err := get(90_000_000, 0)
			if err != nil {
				return LatencyTimingDiagnostics{}, err
			}
			ss, err := get(90_000_000, 120_000_000)
			if err != nil {
				return LatencyTimingDiagnostics{}, err
			}
			derived := latencyPairedContrast(target, seed, [4]float64{ff.FilledFraction, fs.FilledFraction, sf.FilledFraction, ss.FilledFraction})
			foundContrast := false
			for _, stored := range surface.PairedContrasts {
				if stored.TargetQty == target && stored.Seed == seed {
					if foundContrast || stored != derived {
						return LatencyTimingDiagnostics{}, errors.New("latency pilot: paired contrast differs from cell outcomes")
					}
					foundContrast = true
				}
			}
			if !foundContrast {
				return LatencyTimingDiagnostics{}, errors.New("latency pilot: paired contrast absent")
			}
			for _, pair := range [][2]LatencyCellResult{{ff, fs}, {sf, ss}} {
				diagnostic.ProcessingMatchedPairs++
				if pair[0].Outcome.VenueArrivalAt != pair[1].Outcome.VenueArrivalAt {
					diagnostic.ProcessingChangedOrderArrival++
				}
				if pair[0].Outcome.DeliveredSnapshotSeq != pair[1].Outcome.DeliveredSnapshotSeq {
					diagnostic.ProcessingChangedSelectedSnapshot++
				}
				if pair[0].Outcome.FilledQty != pair[1].Outcome.FilledQty {
					diagnostic.ProcessingChangedFilledQty++
				}
			}
			for _, pair := range [][2]LatencyCellResult{{ff, sf}, {fs, ss}} {
				diagnostic.NetworkMatchedPairs++
				if pair[0].Outcome.VenueArrivalAt != pair[1].Outcome.VenueArrivalAt {
					diagnostic.NetworkChangedOrderArrival++
				}
				if pair[0].Outcome.FilledQty != pair[1].Outcome.FilledQty {
					diagnostic.NetworkChangedFilledQty++
				}
			}
		}
	}
	return diagnostic, nil
}

func WriteLatencyDiagnostics(path string, diagnostics LatencyTimingDiagnostics) error {
	if diagnostics.SchemaVersion != 1 || diagnostics.ProcessingMatchedPairs != 12 || diagnostics.NetworkMatchedPairs != 12 {
		return errors.New("latency pilot: incomplete timing diagnostics")
	}
	return writeExclusiveJSON(path, diagnostics)
}
