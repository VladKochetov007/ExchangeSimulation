package analysis

import (
	"os"
	"path/filepath"
	"testing"

	"exchange_sim/exchange"
	"exchange_sim/simulation"
)

func TestMakerInventoryRebalanceAuditJoinsLocalPolicyReceiptAndFill(t *testing.T) {
	run := p2TestRun(t, p2Fixture{})
	result, err := run.MeasureMakerInventoryRebalance()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.Decisions != 1 || result.Submitted != 1 || result.Accepted != 1 || result.Fills != 1 || result.FilledQty != 500_000_000 || result.ReceiptMatches != 1 || result.ActionCounts["SUBMIT_IOC"] != 1 || result.CancelledIOC != 0 || len(result.Checks) != 0 {
		t.Fatalf("valid P2 evidence audit = %+v", result)
	}
}

func TestMakerInventoryRebalanceAuditDisabledControlDoesNotRequireUnusedSnapshot(t *testing.T) {
	run := p2TestRun(t, p2Fixture{Disabled: true})
	result, err := run.MeasureMakerInventoryRebalance()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.Decisions != 1 || result.DisabledDecisions != 1 || result.Submitted != 0 || result.ReceiptMatches != 0 || result.ActionCounts["POLICY_DISABLED"] != 1 || len(result.Checks) != 0 {
		t.Fatalf("disabled P2 control audit = %+v", result)
	}
}

func TestMakerInventoryRebalanceAuditCatchesDeclaredMutations(t *testing.T) {
	tests := []struct {
		name  string
		setup p2Fixture
		check func(*MakerInventoryRebalanceAudit) bool
	}{
		{
			name: "side reversal", setup: p2Fixture{Decision: map[string]any{"side": "BUY"}},
			check: func(result *MakerInventoryRebalanceAudit) bool { return result.DecisionFieldMismatches > 0 },
		},
		{
			name: "fake in-band deferral", setup: p2Fixture{Decision: map[string]any{"action_or_defer_reason": "IN_BAND", "request_id": uint64(0), "requested_qty": int64(0), "cooldown_until": int64(0)}},
			check: func(result *MakerInventoryRebalanceAudit) bool { return result.DecisionFieldMismatches > 0 },
		},
		{
			name: "omitted participation cap", setup: p2Fixture{Decision: map[string]any{"participation_cap": int64(0)}},
			check: func(result *MakerInventoryRebalanceAudit) bool { return result.DecisionFieldMismatches > 0 },
		},
		{
			name: "zero fee", setup: p2Fixture{Fill: map[string]any{"fee_amount": int64(0)}, FillEvidence: map[string]any{"fee_amount": int64(0)}},
			check: func(result *MakerInventoryRebalanceAudit) bool {
				return result.NonPositiveFees > 0 && result.FeeMismatches > 0
			},
		},
		{
			name: "fake fill evidence", setup: p2Fixture{FillEvidence: map[string]any{"qty": int64(400_000_000)}},
			check: func(result *MakerInventoryRebalanceAudit) bool {
				return result.MissingFillEvidence > 0 && result.UnexpectedFillEvidence > 0
			},
		},
		{
			name: "non-reducing local inventory transition", setup: p2Fixture{FillEvidence: map[string]any{"post_inventory": int64(15_000_000_000)}},
			check: func(result *MakerInventoryRebalanceAudit) bool { return result.NonReducingFills > 0 },
		},
		{
			name: "self counterparty", setup: p2Fixture{CounterpartyClient: 7},
			check: func(result *MakerInventoryRebalanceAudit) bool { return result.SelfFills > 0 },
		},
		{
			name: "duplicate request", setup: p2Fixture{DuplicateDecision: true},
			check: func(result *MakerInventoryRebalanceAudit) bool { return result.DuplicateDecisions > 0 },
		},
		{
			name: "dropped IOC cancellation", setup: p2Fixture{PartialFill: true, DropCancel: true},
			check: func(result *MakerInventoryRebalanceAudit) bool { return result.MissingIOCTerminals > 0 },
		},
		{
			name: "future-injected receipt", setup: p2Fixture{ReceiptAt: 21_000_000_000, Decision: map[string]any{"last_book_received_time": int64(21_000_000_000)}},
			check: func(result *MakerInventoryRebalanceAudit) bool { return result.FutureReceiptUse > 0 },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			run := p2TestRun(t, tc.setup)
			result, err := run.MeasureMakerInventoryRebalance()
			if err != nil {
				t.Fatal(err)
			}
			if result.Valid || !tc.check(result) {
				t.Fatalf("P2 mutation survived: %+v", result)
			}
		})
	}
}

type p2Fixture struct {
	Decision           map[string]any
	Fill               map[string]any
	FillEvidence       map[string]any
	DuplicateDecision  bool
	PartialFill        bool
	DropCancel         bool
	Disabled           bool
	CounterpartyClient uint64
	ReceiptAt          int64
}

func p2TestRun(t *testing.T, fixture p2Fixture) *Run {
	t.Helper()
	dir := t.TempDir()
	if fixture.ReceiptAt == 0 {
		fixture.ReceiptAt = 11_000_000_000
	}
	writeP2Receipt(t, dir, fixture.ReceiptAt)
	decision := map[string]any{
		"venue_id": "north", "maker": "cdf_spot_maker_1", "client_id": uint64(7), "symbol": "CDF/USD", "decision_time": int64(20_000_000_000), "enabled": true, "subscribed": true, "request_pending": false, "action_or_defer_reason": "SUBMIT_IOC",
		"inventory": int64(15_000_000_000), "risk_band_qty": int64(10_000_000_000), "target_band_qty": int64(5_000_000_000), "last_book_source_time": int64(10_000_000_000), "last_book_received_time": fixture.ReceiptAt, "last_book_sequence": uint64(7),
		"bid_price": int64(300_000_000), "bid_visible_qty": int64(50_000_000_000), "ask_price": int64(300_100_000), "ask_visible_qty": int64(50_000_000_000), "side": "SELL", "desired_reduction": int64(10_000_000_000), "participation_cap": int64(5_000_000_000),
		"max_request_qty": int64(500_000_000), "participation_bps": int64(1_000), "slippage_bps": int64(50), "evaluation_interval": int64(10_000_000_000), "cooldown": int64(30_000_000_000), "limit_price": int64(298_500_000), "requested_qty": int64(500_000_000), "taker_fee_bps": int64(5),
		"request_id": uint64(42), "cooldown_until": int64(50_000_000_000), "outcome_expectation": "VENUE_OUTCOME_REQUIRED",
	}
	if fixture.Disabled {
		decision["enabled"] = false
		decision["subscribed"] = false
		decision["action_or_defer_reason"] = "POLICY_DISABLED"
		decision["request_id"] = uint64(0)
		decision["requested_qty"] = int64(0)
		decision["cooldown_until"] = int64(0)
	}
	counterpartyClient := uint64(8)
	if fixture.CounterpartyClient != 0 {
		counterpartyClient = fixture.CounterpartyClient
	}
	for field, value := range fixture.Decision {
		decision[field] = value
	}
	if fixture.Disabled {
		path := filepath.Join(dir, "general.jsonl")
		if err := os.WriteFile(path, []byte(logLine(20_000_000_000, 7, "maker_inventory_rebalance_decision", decision)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return &Run{Dir: dir, files: []string{path}, roles: map[Participant]string{{VenueID: "north", ClientID: 7}: "cdf_spot_maker"}}
	}
	order := map[string]any{"order_id": uint64(70), "client_id": uint64(7), "request_id": uint64(42), "symbol": "CDF/USD", "side": "SELL", "type": "LIMIT", "time_in_force": "IOC", "post_only": false, "price": int64(298_500_000), "qty": int64(500_000_000)}
	qty := int64(500_000_000)
	if fixture.PartialFill {
		qty = 400_000_000
	}
	fee := int64(746_250)
	if fixture.PartialFill {
		fee = 597_000
	}
	fill := map[string]any{"order_id": uint64(70), "trade_id": uint64(9), "symbol": "CDF/USD", "side": "SELL", "qty": qty, "price": int64(298_500_000), "fee_amount": fee, "fee_asset": "USD", "role": "taker"}
	for field, value := range fixture.Fill {
		fill[field] = value
	}
	fillEvidence := map[string]any{"venue_id": "north", "maker": "cdf_spot_maker_1", "client_id": uint64(7), "symbol": "CDF/USD", "timestamp": int64(21_000_000_000), "order_id": uint64(70), "trade_id": uint64(9), "side": "SELL", "qty": qty, "price": int64(298_500_000), "fee_amount": fee, "fee_asset": "USD", "pre_inventory": int64(15_000_000_000), "post_inventory": int64(15_000_000_000 - qty)}
	for field, value := range fixture.FillEvidence {
		fillEvidence[field] = value
	}
	lines := []string{
		logLine(20_000_000_000, 7, "maker_inventory_rebalance_decision", decision),
		logLine(21_000_000_000, 7, "OrderAccepted", order),
		logLine(21_000_000_000, counterpartyClient, "OrderAccepted", map[string]any{"order_id": uint64(80), "client_id": counterpartyClient, "request_id": uint64(43), "symbol": "CDF/USD", "side": "BUY", "type": "LIMIT", "time_in_force": "GTC", "post_only": false, "price": int64(298_500_000), "qty": qty}),
		logLine(21_000_000_000, 0, "Trade", map[string]any{"trade_id": uint64(9), "price": int64(298_500_000), "qty": qty, "taker_order_id": uint64(70), "maker_order_id": uint64(80)}),
		logLine(21_000_000_000, 7, "OrderFill", fill),
		logLine(21_000_000_000, 7, "maker_inventory_rebalance_fill", fillEvidence),
	}
	if fixture.PartialFill && !fixture.DropCancel {
		lines = append(lines, logLine(21_000_000_000, 7, "OrderCancelled", map[string]any{"order_id": uint64(70), "remaining_qty": int64(500_000_000 - qty)}))
	}
	if fixture.DuplicateDecision {
		lines = append(lines, logLine(20_000_000_000, 7, "maker_inventory_rebalance_decision", decision))
	}
	path := filepath.Join(dir, "general.jsonl")
	if err := os.WriteFile(path, []byte(joinLines(lines)), 0o644); err != nil {
		t.Fatal(err)
	}
	roles := map[Participant]string{{VenueID: "north", ClientID: 7}: "cdf_spot_maker"}
	if counterpartyClient != 7 {
		roles[Participant{VenueID: "north", ClientID: counterpartyClient}] = "noise_flow"
	}
	return &Run{Dir: dir, files: []string{path}, roles: roles}
}

func writeP2Receipt(t *testing.T, dir string, deliveredAt int64) {
	t.Helper()
	recorder, err := simulation.NewMarketDataReceiptRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	schedule := simulation.MarketDataSchedule{ClientID: 7, SourceVenue: "north", Link: "north/cdf_spot_maker/client/7", Symbol: "CDF/USD", Type: exchange.MDSnapshot, Sequence: 7, Fingerprint: [16]byte{1}, PublishedAt: 10_000_000_000, ScheduledAt: 10_000_000_000, LinkOrdinal: 1}
	recorder.RegisterLink("north", schedule.Link, "cdf_spot_maker")
	recorder.RecordSchedule(schedule)
	recorder.RecordReceipt(simulation.MarketDataReceipt{MarketDataSchedule: schedule, DeliveredAt: deliveredAt})
	if err := recorder.Finalize(100_000_000_000); err != nil {
		t.Fatal(err)
	}
}

func joinLines(lines []string) string {
	output := ""
	for _, line := range lines {
		output += line + "\n"
	}
	return output
}
