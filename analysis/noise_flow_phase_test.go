package analysis

import "testing"

func TestNoiseFlowPhaseReplayRejectsTimingMutations(t *testing.T) {
	phase := int64(1_000_000_000)
	participant := Participant{VenueID: "north", ClientID: 7}
	roster := map[Participant]string{participant: "noise_flow_1"}
	validRows := []noiseFlowPhaseDecision{
		{VenueID: "north", Role: "noise_flow_1", ClientID: 7, DecisionTime: noiseFlowPhaseSimulationStart + 3_000_000_000, DecisionPhaseOffset: &phase, Action: "SUBSCRIBE"},
		{VenueID: "north", Role: "noise_flow_1", ClientID: 7, DecisionTime: noiseFlowPhaseSimulationStart + 5_000_000_000, DecisionPhaseOffset: &phase, Action: "EVALUATE", SubmittedRequestCount: 1},
		{VenueID: "north", Role: "noise_flow_1", ClientID: 7, DecisionTime: noiseFlowPhaseSimulationStart + 7_000_000_000, DecisionPhaseOffset: &phase, Action: "EVALUATE"},
	}
	check := func(rows []noiseFlowPhaseDecision) *NoiseFlowPhaseAudit {
		result := &NoiseFlowPhaseAudit{
			DecisionPhaseOffsetNanos: phase, NoiseIntervalNanos: 2_000_000_000,
			TerminalAt: noiseFlowPhaseSimulationStart + 7_000_000_000,
		}
		validateNoiseFlowPhaseRows(result, roster, map[Participant][]noiseFlowPhaseDecision{participant: rows})
		return result
	}
	valid := check(validRows)
	if valid.MissingTicks != 0 || valid.DuplicateTicks != 0 || valid.OffPhaseTicks != 0 || valid.PhaseMismatches != 0 || valid.InvalidRecords != 0 || valid.ActionMismatches != 0 || valid.Decisions != 3 || valid.SubmittedRequests != 1 {
		t.Fatalf("valid timing rows failed replay: %+v", valid)
	}
	mutations := map[string]func([]noiseFlowPhaseDecision) []noiseFlowPhaseDecision{
		"dropped":   func(rows []noiseFlowPhaseDecision) []noiseFlowPhaseDecision { return rows[:2] },
		"duplicate": func(rows []noiseFlowPhaseDecision) []noiseFlowPhaseDecision { return append(rows, rows[2]) },
		"off phase": func(rows []noiseFlowPhaseDecision) []noiseFlowPhaseDecision { rows[1].DecisionTime++; return rows },
		"phase mismatch": func(rows []noiseFlowPhaseDecision) []noiseFlowPhaseDecision {
			wrong := phase - 1
			rows[1].DecisionPhaseOffset = &wrong
			return rows
		},
		"phase omitted": func(rows []noiseFlowPhaseDecision) []noiseFlowPhaseDecision {
			rows[1].DecisionPhaseOffset = nil
			return rows
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			rows := append([]noiseFlowPhaseDecision(nil), validRows...)
			result := check(mutate(rows))
			if result.MissingTicks == 0 && result.DuplicateTicks == 0 && result.OffPhaseTicks == 0 && result.PhaseMismatches == 0 && result.InvalidRecords == 0 {
				t.Fatalf("mutation survived independent timing replay: %+v", result)
			}
		})
	}
}
