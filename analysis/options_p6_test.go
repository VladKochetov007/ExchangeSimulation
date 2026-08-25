package analysis

import (
	"fmt"
	"testing"
)

func p6LiabilityDecisionLine(ts int64, venue string, client, request uint64, receivedAt int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"option_liability_user_decision","data":{"venue_id":%q,"payload":{"client_id":%d,"decision_time":%d,"action":"SUBMIT_PUT_IOC","reason":"SUBMIT_PUT_IOC","target_qty":10,"position_before":0,"option_symbol":"OPT-P","has_ask":true,"ask_price":100,"ask_source_time":%d,"ask_received_at":%d,"underlying_source_time":%d,"underlying_received_at":%d,"request_id":%d,"requested_qty":10,"side":"BUY","order_type":"LIMIT","time_in_force":"IOC"}}}`,
		ts, client, venue, client, ts, ts-1, receivedAt, ts-1, receivedAt, request)
}

func p6LiabilityRejectedLine(ts int64, venue string, client, request uint64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"OrderRejected","data":{"venue_id":%q,"payload":{"symbol":"OPT-P","payload":{"request_id":%d,"success":false,"error":"INSUFFICIENT_BALANCE"}}}}`, ts, client, venue, request)
}

func p6LiabilityReport() Report {
	return Report{TerminalAccounts: []AccountRow{
		{VenueID: "north", ClientID: 7, Role: "option_liability_user_1"},
		{VenueID: "south", ClientID: 7, Role: "option_liability_user_1"},
	}}
}

// Request identifiers are actor-local and restart at the same value on each
// venue. The audit must include venue and client identity or it will falsely
// call independent outcomes duplicates/missing.
func TestOptionLiabilityAuditScopesRequestIdentityByVenue(t *testing.T) {
	const client, request = uint64(7), uint64(1)
	dir := writeRun(t, p6LiabilityReport(), map[string][]string{
		"north/general.jsonl": {
			p6LiabilityDecisionLine(100, "north", client, request, 100),
		},
		"north/derivatives.jsonl": {
			p6LiabilityRejectedLine(101, "north", client, request),
		},
		"south/general.jsonl": {
			p6LiabilityDecisionLine(100, "south", client, request, 100),
		},
		"south/derivatives.jsonl": {
			p6LiabilityRejectedLine(101, "south", client, request),
		},
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	audit, err := run.MeasureOptionLiability()
	if err != nil {
		t.Fatalf("MeasureOptionLiability: %v", err)
	}
	if audit.Participants != 2 || audit.Decisions != 2 || audit.SubmitDecisions != 2 || audit.Rejected != 2 {
		t.Fatalf("audit counts = participants %d decisions %d submits %d rejects %d, want 2/2/2/2", audit.Participants, audit.Decisions, audit.SubmitDecisions, audit.Rejected)
	}
	if audit.MissingOutcomes != 0 || audit.DuplicateOutcomes != 0 || audit.OrphanOutcomes != 0 || !audit.Valid {
		t.Fatalf("independent venue outcomes were not joined exactly: %+v", audit)
	}
	if audit.ParticipantsByVenue["north"] != 1 || audit.ParticipantsByVenue["south"] != 1 || audit.DecisionsByVenue["north"] != 1 || audit.DecisionsByVenue["south"] != 1 {
		t.Fatalf("per-venue activation counts = participants=%v decisions=%v", audit.ParticipantsByVenue, audit.DecisionsByVenue)
	}
}

func TestOptionLiabilityAuditCatchesOutcomeAndFrontierMutations(t *testing.T) {
	tests := []struct {
		name       string
		decision   string
		outcomes   []string
		wantDup    int
		wantFuture int
	}{
		{
			name:       "duplicate rejection",
			decision:   p6LiabilityDecisionLine(100, "north", 7, 1, 100),
			outcomes:   []string{p6LiabilityRejectedLine(101, "north", 7, 1), p6LiabilityRejectedLine(102, "north", 7, 1)},
			wantDup:    1,
			wantFuture: 0,
		},
		{
			name:       "future delivered frontier",
			decision:   p6LiabilityDecisionLine(100, "north", 7, 1, 101),
			outcomes:   []string{p6LiabilityRejectedLine(101, "north", 7, 1)},
			wantDup:    0,
			wantFuture: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeRun(t, Report{TerminalAccounts: []AccountRow{{VenueID: "north", ClientID: 7, Role: "option_liability_user_1"}}}, map[string][]string{
				"north/general.jsonl":     {tc.decision},
				"north/derivatives.jsonl": tc.outcomes,
			})
			run, err := Open(dir)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			audit, err := run.MeasureOptionLiability()
			if err != nil {
				t.Fatalf("MeasureOptionLiability: %v", err)
			}
			if audit.DuplicateOutcomes != tc.wantDup || audit.FutureObservationUse != tc.wantFuture || audit.Valid {
				t.Fatalf("mutation audit = %+v, want duplicate=%d future=%d invalid", audit, tc.wantDup, tc.wantFuture)
			}
		})
	}
}
