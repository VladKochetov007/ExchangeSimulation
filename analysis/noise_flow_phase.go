package analysis

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// NoiseFlowPhaseAudit independently verifies the V2-4 L1-P2 broad-flow clock
// treatment. It reads the immutable run configuration, terminal participant
// roster, receipt-sidecar terminal boundary, and persisted timing rows; it
// never imports the actor or scheduler implementation.
type NoiseFlowPhaseAudit struct {
	DecisionPhaseOffsetNanos int64 `json:"decision_phase_offset_nanos"`
	NoiseIntervalNanos       int64 `json:"noise_interval_nanos"`
	TerminalAt               int64 `json:"terminal_at"`

	ExpectedParticipants int64 `json:"expected_participants"`
	ExpectedTicks        int64 `json:"expected_ticks_per_participant"`
	Decisions            int64 `json:"decisions"`
	SubscribeDecisions   int64 `json:"subscribe_decisions"`
	EvaluateDecisions    int64 `json:"evaluate_decisions"`
	SubmittedRequests    int64 `json:"submitted_requests"`

	ReceiptAuditValid     bool  `json:"receipt_audit_valid"`
	ReceiptEvidenceErrors int64 `json:"receipt_evidence_errors"`
	InvalidRecords        int64 `json:"invalid_records"`
	UnknownParticipants   int64 `json:"unknown_participants"`
	PhaseMismatches       int64 `json:"phase_mismatches"`
	OffPhaseTicks         int64 `json:"off_phase_ticks"`
	MissingTicks          int64 `json:"missing_ticks"`
	DuplicateTicks        int64 `json:"duplicate_ticks"`
	ActionMismatches      int64 `json:"action_mismatches"`
	ExtraTicks            int64 `json:"extra_ticks"`

	Participants []NoiseFlowPhaseBucket `json:"participants,omitempty"`
	Checks       []NoiseFlowPhaseCheck  `json:"checks,omitempty"`
	Valid        bool                   `json:"valid"`
}

// NoiseFlowPhaseBucket keeps every venue-local noise account visible instead
// of averaging timing errors across the population.
type NoiseFlowPhaseBucket struct {
	VenueID          string `json:"venue_id"`
	ClientID         uint64 `json:"client_id"`
	Role             string `json:"role"`
	Decisions        int64  `json:"decisions"`
	Subscribe        int64  `json:"subscribe"`
	Evaluate         int64  `json:"evaluate"`
	SubmittedRequest int64  `json:"submitted_requests"`
}

// NoiseFlowPhaseCheck identifies one independent replay failure.
type NoiseFlowPhaseCheck struct {
	VenueID  string `json:"venue_id"`
	ClientID uint64 `json:"client_id"`
	Time     int64  `json:"decision_time"`
	Failure  string `json:"failure"`
}

type noiseFlowPhaseRunConfig struct {
	NoiseInterval                int64  `json:"noise_interval"`
	NoiseFlowDecisionPhaseOffset *int64 `json:"noise_flow_decision_phase_offset"`
	NoiseTraderCount             int    `json:"noise_trader_count"`
}

type noiseFlowPhaseDecision struct {
	VenueID               string `json:"venue_id"`
	Role                  string `json:"role"`
	ClientID              uint64 `json:"client_id"`
	DecisionTime          int64  `json:"decision_time"`
	DecisionPhaseOffset   *int64 `json:"decision_phase_offset_nanos"`
	Action                string `json:"action"`
	SubmittedRequestCount int64  `json:"submitted_request_count"`
}

const noiseFlowPhaseSimulationStart = int64(1_735_689_600_000_000_000)

// MeasureNoiseFlowPhase proves that every configured noise-flow actor emitted
// exactly one persisted tick on every declared phase-lattice point through the
// receipt-sidecar terminal boundary. A zero submitted count is valid evidence
// of an evaluated no-order decision, never a proxy for a missing row.
func (r *Run) MeasureNoiseFlowPhase() (*NoiseFlowPhaseAudit, error) {
	config, err := loadNoiseFlowPhaseRunConfig(r.Dir)
	if err != nil {
		return nil, err
	}
	if err := validNoiseFlowPhaseRunConfig(config); err != nil {
		return nil, err
	}
	receiptAudit, receiptErr := AuditMarketDataReceipts(r.Dir)
	if receiptErr != nil {
		return nil, fmt.Errorf("noise-flow phase: audit receipt evidence: %w", receiptErr)
	}
	result := &NoiseFlowPhaseAudit{
		DecisionPhaseOffsetNanos: *config.NoiseFlowDecisionPhaseOffset,
		NoiseIntervalNanos:       config.NoiseInterval,
		TerminalAt:               receiptAudit.TerminalAt,
		ReceiptAuditValid:        receiptAudit.Valid,
	}
	if !receiptAudit.Valid || receiptAudit.TerminalAt <= noiseFlowPhaseSimulationStart {
		result.ReceiptEvidenceErrors++
	}
	roster := noiseFlowPhaseRoster(r)
	result.ExpectedParticipants = int64(len(roster))
	if len(roster) != config.NoiseTraderCount*countNoiseFlowVenues(roster) {
		result.InvalidRecords++
		result.Checks = append(result.Checks, NoiseFlowPhaseCheck{Failure: "terminal_noise_flow_roster_does_not_match_config"})
	}
	rows := make(map[Participant][]noiseFlowPhaseDecision, len(roster))
	err = r.Scan(ScanOptions{Events: []string{"noise_flow_phase_decision"}, Workers: 1}, func(event Event) {
		var row noiseFlowPhaseDecision
		if event.Decode(&row) != nil || row.VenueID != event.VenueID || row.ClientID != event.ClientID ||
			row.Role == "" || row.DecisionTime != event.SimTS || row.DecisionPhaseOffset == nil ||
			row.SubmittedRequestCount < 0 || (row.Action != "SUBSCRIBE" && row.Action != "EVALUATE") {
			result.InvalidRecords++
			result.Checks = append(result.Checks, NoiseFlowPhaseCheck{VenueID: event.VenueID, ClientID: event.ClientID, Time: event.SimTS, Failure: "invalid_noise_flow_phase_record"})
			return
		}
		participant := Participant{VenueID: event.VenueID, ClientID: event.ClientID}
		role, known := roster[participant]
		if !known || role != row.Role {
			result.UnknownParticipants++
			result.Checks = append(result.Checks, NoiseFlowPhaseCheck{VenueID: event.VenueID, ClientID: event.ClientID, Time: event.SimTS, Failure: "noise_flow_record_not_in_terminal_roster"})
			return
		}
		rows[participant] = append(rows[participant], row)
	})
	if err != nil {
		return nil, err
	}
	validateNoiseFlowPhaseRows(result, roster, rows)
	result.Valid = result.ReceiptEvidenceErrors == 0 && result.InvalidRecords == 0 && result.UnknownParticipants == 0 &&
		result.PhaseMismatches == 0 && result.OffPhaseTicks == 0 && result.MissingTicks == 0 && result.DuplicateTicks == 0 &&
		result.ActionMismatches == 0 && result.ExtraTicks == 0 && result.Decisions == result.ExpectedParticipants*result.ExpectedTicks
	return result, nil
}

func loadNoiseFlowPhaseRunConfig(dir string) (noiseFlowPhaseRunConfig, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "run-config.json"))
	if err != nil {
		return noiseFlowPhaseRunConfig{}, fmt.Errorf("noise-flow phase: read run configuration: %w", err)
	}
	var config noiseFlowPhaseRunConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return noiseFlowPhaseRunConfig{}, fmt.Errorf("noise-flow phase: decode run configuration: %w", err)
	}
	return config, nil
}

func validNoiseFlowPhaseRunConfig(config noiseFlowPhaseRunConfig) error {
	if config.NoiseInterval <= 0 || config.NoiseTraderCount <= 0 {
		return fmt.Errorf("noise-flow phase: invalid interval %d or trader count %d", config.NoiseInterval, config.NoiseTraderCount)
	}
	if config.NoiseFlowDecisionPhaseOffset == nil {
		return fmt.Errorf("noise-flow phase: run configuration omits explicit phase offset")
	}
	if *config.NoiseFlowDecisionPhaseOffset < 0 || *config.NoiseFlowDecisionPhaseOffset >= config.NoiseInterval {
		return fmt.Errorf("noise-flow phase: phase %d is outside [0,%d)", *config.NoiseFlowDecisionPhaseOffset, config.NoiseInterval)
	}
	return nil
}

func noiseFlowPhaseRoster(r *Run) map[Participant]string {
	roster := make(map[Participant]string)
	for _, account := range r.Report.TerminalAccounts {
		if strings.HasPrefix(account.Role, "noise_flow_") {
			roster[Participant{VenueID: account.VenueID, ClientID: account.ClientID}] = account.Role
		}
	}
	return roster
}

func countNoiseFlowVenues(roster map[Participant]string) int {
	venues := make(map[string]struct{})
	for participant := range roster {
		venues[participant.VenueID] = struct{}{}
	}
	return len(venues)
}

func validateNoiseFlowPhaseRows(result *NoiseFlowPhaseAudit, roster map[Participant]string, rows map[Participant][]noiseFlowPhaseDecision) {
	if result.TerminalAt <= noiseFlowPhaseSimulationStart || result.NoiseIntervalNanos <= 0 {
		return
	}
	phase := result.DecisionPhaseOffsetNanos
	first := noiseFlowPhaseSimulationStart + result.NoiseIntervalNanos + phase
	for at := first; at <= result.TerminalAt; at += result.NoiseIntervalNanos {
		result.ExpectedTicks++
	}
	participants := make([]Participant, 0, len(roster))
	for participant := range roster {
		participants = append(participants, participant)
	}
	sort.Slice(participants, func(i, j int) bool {
		if participants[i].VenueID != participants[j].VenueID {
			return participants[i].VenueID < participants[j].VenueID
		}
		return participants[i].ClientID < participants[j].ClientID
	})
	for _, participant := range participants {
		bucket := NoiseFlowPhaseBucket{VenueID: participant.VenueID, ClientID: participant.ClientID, Role: roster[participant]}
		byTime := make(map[int64][]noiseFlowPhaseDecision)
		for _, row := range rows[participant] {
			result.Decisions++
			bucket.Decisions++
			bucket.SubmittedRequest += row.SubmittedRequestCount
			result.SubmittedRequests += row.SubmittedRequestCount
			if row.Action == "SUBSCRIBE" {
				result.SubscribeDecisions++
				bucket.Subscribe++
			} else {
				result.EvaluateDecisions++
				bucket.Evaluate++
			}
			if row.DecisionPhaseOffset == nil {
				result.InvalidRecords++
				result.Checks = append(result.Checks, NoiseFlowPhaseCheck{VenueID: participant.VenueID, ClientID: participant.ClientID, Time: row.DecisionTime, Failure: "missing_decision_phase_field"})
			} else if *row.DecisionPhaseOffset != phase {
				result.PhaseMismatches++
				result.Checks = append(result.Checks, NoiseFlowPhaseCheck{VenueID: participant.VenueID, ClientID: participant.ClientID, Time: row.DecisionTime, Failure: "decision_phase_field_mismatch"})
			}
			if row.DecisionTime < first || row.DecisionTime > result.TerminalAt || (row.DecisionTime-first)%result.NoiseIntervalNanos != 0 {
				result.OffPhaseTicks++
				result.Checks = append(result.Checks, NoiseFlowPhaseCheck{VenueID: participant.VenueID, ClientID: participant.ClientID, Time: row.DecisionTime, Failure: "decision_time_off_declared_phase_lattice"})
			}
			byTime[row.DecisionTime] = append(byTime[row.DecisionTime], row)
		}
		for at := first; at <= result.TerminalAt; at += result.NoiseIntervalNanos {
			matching := byTime[at]
			switch len(matching) {
			case 0:
				result.MissingTicks++
				result.Checks = append(result.Checks, NoiseFlowPhaseCheck{VenueID: participant.VenueID, ClientID: participant.ClientID, Time: at, Failure: "missing_declared_noise_flow_tick"})
				continue
			case 1:
			default:
				result.DuplicateTicks += int64(len(matching) - 1)
				result.Checks = append(result.Checks, NoiseFlowPhaseCheck{VenueID: participant.VenueID, ClientID: participant.ClientID, Time: at, Failure: "duplicate_noise_flow_tick"})
			}
			wantAction := "EVALUATE"
			if at == first {
				wantAction = "SUBSCRIBE"
			}
			if matching[0].Action != wantAction || (wantAction == "SUBSCRIBE" && matching[0].SubmittedRequestCount != 0) {
				result.ActionMismatches++
				result.Checks = append(result.Checks, NoiseFlowPhaseCheck{VenueID: participant.VenueID, ClientID: participant.ClientID, Time: at, Failure: "noise_flow_tick_action_mismatch"})
			}
		}
		for at, matching := range byTime {
			if at < first || at > result.TerminalAt || (at-first)%result.NoiseIntervalNanos != 0 {
				result.ExtraTicks += int64(len(matching))
			}
		}
		result.Participants = append(result.Participants, bucket)
	}
}
