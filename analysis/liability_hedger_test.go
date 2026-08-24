package analysis

import (
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"exchange_sim/exchange"
	"exchange_sim/simulation"
)

func TestLiabilityHedgerAuditReplaysLocalStateReceiptAndFill(t *testing.T) {
	run := l0TestRun(t, l0Fixture{})
	result, err := run.MeasureLiabilityHedger()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.Decisions != 2 || result.StateUpdates != 1 || result.Submitted != 1 || result.Accepted != 1 || result.Fills != 1 || result.ReceiptMatches != 1 || result.ActionCounts["SUBMIT_IOC"] != 1 || len(result.Hedgers) != 1 || result.Hedgers[0].Accepted != 1 || result.Hedgers[0].Fills != 1 || len(result.Checks) != 0 {
		t.Fatalf("valid L0 evidence audit = %+v", result)
	}
}

func TestLiabilityHedgerAuditDisabledControlEvolvesButDoesNotAct(t *testing.T) {
	run := l0TestRun(t, l0Fixture{Disabled: true})
	result, err := run.MeasureLiabilityHedger()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.Decisions != 2 || result.DisabledDecisions != 2 || result.StateUpdates != 1 || result.Submitted != 0 || result.Accepted != 0 || result.Fills != 0 || result.ActionCounts["NOT_SUBSCRIBED"] != 1 || result.ActionCounts["POLICY_DISABLED"] != 1 || len(result.Checks) != 0 {
		t.Fatalf("disabled L0 evidence audit = %+v", result)
	}
}

func TestLiabilityHedgerAuditReplaysRandomSideControlAndReportsItsGapDirection(t *testing.T) {
	run := l0TestRun(t, l0Fixture{PolicyMode: liabilityHedgerPolicyRandom})
	result, err := run.MeasureLiabilityHedger()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.PolicyMode != liabilityHedgerPolicyRandom || result.RandomControlFills != 1 || result.RandomControlReducing+result.RandomControlNonReducing != 1 || result.NonReducingFills != 0 {
		t.Fatalf("valid L1 random-control audit = %+v", result)
	}
}

func TestLiabilityHedgerAuditAcceptsExplicitUnavailableTouchWithoutRequest(t *testing.T) {
	run := l0TestRun(t, l0Fixture{PolicyMode: liabilityHedgerPolicyRandom, NoExecutableTouch: true})
	result, err := run.MeasureLiabilityHedger()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.Deferred != 2 || result.Submitted != 0 || result.DecisionFieldMismatches != 0 || result.ActionCounts["LOCAL_EXECUTABLE_PRICE_UNAVAILABLE"] != 1 || len(result.Checks) != 0 {
		t.Fatalf("unavailable-touch random-control defer = %+v", result)
	}

	// The deferred policy may expose its intended quantity, but it must not
	// smuggle an actual venue request across the decision boundary.
	mutated := l0TestRun(t, l0Fixture{
		PolicyMode:        liabilityHedgerPolicyRandom,
		NoExecutableTouch: true,
		Decision:          map[string]any{"request_id": uint64(42)},
	})
	result, err = mutated.MeasureLiabilityHedger()
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid || result.DecisionFieldMismatches == 0 {
		t.Fatalf("unavailable-touch request-id mutation survived: %+v", result)
	}
}

func TestLiabilityHedgerAuditAcceptsOnlyARealTailCensor(t *testing.T) {
	run := l0TestRun(t, l0Fixture{TailCensored: true})
	result, err := run.MeasureLiabilityHedger()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.HorizonCensored != 1 || result.Submitted != 0 || result.ActionCounts["SIMULATION_HORIZON_CENSORED"] != 1 {
		t.Fatalf("terminal L0 defer = %+v", result)
	}
}

func TestLiabilityHedgerAuditCatchesDeclaredMutations(t *testing.T) {
	tests := []struct {
		name  string
		setup l0Fixture
		check func(*LiabilityHedgerAudit) bool
	}{
		{
			name: "dropped decision", setup: l0Fixture{DropInitialDecision: true},
			check: func(result *LiabilityHedgerAudit) bool { return result.DecisionFieldMismatches > 0 },
		},
		{
			name: "duplicated decision", setup: l0Fixture{DuplicateDecision: true},
			check: func(result *LiabilityHedgerAudit) bool { return result.DuplicateDecisions > 0 },
		},
		{
			name: "stripped obligation update", setup: l0Fixture{Decision: map[string]any{"obligation_step": int64(0), "obligation_after": int64(0)}},
			check: func(result *LiabilityHedgerAudit) bool { return result.DecisionFieldMismatches > 0 },
		},
		{
			name: "reversed hedge side", setup: l0Fixture{ReverseSide: true},
			check: func(result *LiabilityHedgerAudit) bool { return result.DecisionFieldMismatches > 0 },
		},
		{
			name: "future injected receipt", setup: l0Fixture{ReceiptAt: 13_000_000_000},
			check: func(result *LiabilityHedgerAudit) bool { return result.FutureReceiptUse > 0 },
		},
		{
			name: "dropped gateway decision", setup: l0Fixture{DropGatewayDecision: true},
			check: func(result *LiabilityHedgerAudit) bool { return result.MissingGatewayDecisions > 0 },
		},
		{
			name: "missing ask treated as touch", setup: l0Fixture{Decision: map[string]any{"has_ask": false}},
			check: func(result *LiabilityHedgerAudit) bool { return result.DecisionFieldMismatches > 0 },
		},
		{
			name: "request cap violation", setup: l0Fixture{Decision: map[string]any{"requested_qty": int64(100_000_001)}},
			check: func(result *LiabilityHedgerAudit) bool { return result.DecisionFieldMismatches > 0 },
		},
		{
			name: "dropped IOC cancellation", setup: l0Fixture{PartialFill: true, DropCancel: true},
			check: func(result *LiabilityHedgerAudit) bool { return result.MissingIOCTerminals > 0 },
		},
		{
			name: "self fill", setup: l0Fixture{CounterpartyClient: 7},
			check: func(result *LiabilityHedgerAudit) bool { return result.SelfFills > 0 },
		},
		{
			name: "fee free fill", setup: l0Fixture{Fill: map[string]any{"fee_amount": int64(0)}, FillEvidence: map[string]any{"fee_amount": int64(0)}},
			check: func(result *LiabilityHedgerAudit) bool { return result.NonPositiveFees > 0 && result.FeeMismatches > 0 },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			run := l0TestRun(t, tc.setup)
			result, err := run.MeasureLiabilityHedger()
			if err != nil {
				t.Fatal(err)
			}
			if result.Valid || !tc.check(result) {
				t.Fatalf("L0 mutation survived: %+v", result)
			}
		})
	}
}

func TestLiabilityHedgerAuditCatchesRandomControlModeAndSideMutations(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup l0Fixture
	}{
		{name: "omitted random policy mode", setup: l0Fixture{PolicyMode: liabilityHedgerPolicyRandom, Decision: map[string]any{"policy_mode": ""}}},
		{name: "reversed random policy side", setup: l0Fixture{PolicyMode: liabilityHedgerPolicyRandom, ReverseSide: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := l0TestRun(t, tc.setup).MeasureLiabilityHedger()
			if err != nil {
				t.Fatal(err)
			}
			if result.Valid || result.DecisionFieldMismatches == 0 {
				t.Fatalf("random-control mutation survived: %+v", result)
			}
		})
	}
}

func TestLiabilityHedgerBookEvidenceSeparatesPresenceFromPrice(t *testing.T) {
	decision := liabilityHedgerDecision{HasSnapshot: true, HasAsk: true, AskPrice: 0}
	if !validLiabilityHedgerBookEvidence(decision) {
		t.Fatal("present zero-valued ask was treated as missing")
	}
	decision.HasAsk = false
	decision.AskPrice = 1
	if validLiabilityHedgerBookEvidence(decision) {
		t.Fatal("missing ask retained a numeric price field")
	}
}

func TestLiabilityHedgerAuditClassifiesRandomControlGapIncreaseWithoutRejectingIt(t *testing.T) {
	state := &liabilityHedgerReplayState{seenFirst: true, obligation: 100, position: 0}
	valid, reduces := validateLiabilityHedgerStateFill(liabilityHedgerFillEvidence{
		PolicyMode: liabilityHedgerPolicyRandom, Side: "SELL", Qty: 100,
		PrePosition: 0, PostPosition: -100,
	}, state, liabilityHedgerPolicyRandom)
	if !valid || reduces || state.position != -100 {
		t.Fatalf("random-control gap-increasing fill classification = valid=%t reduces=%t state=%+v", valid, reduces, state)
	}
}

type l0Fixture struct {
	Disabled            bool
	PolicyMode          string
	DropInitialDecision bool
	DuplicateDecision   bool
	DropGatewayDecision bool
	TailCensored        bool
	NoExecutableTouch   bool
	ReverseSide         bool
	PartialFill         bool
	DropCancel          bool
	CounterpartyClient  uint64
	ReceiptAt           int64
	TerminalAt          int64
	Decision            map[string]any
	Fill                map[string]any
	FillEvidence        map[string]any
}

func l0TestRun(t *testing.T, fixture l0Fixture) *Run {
	t.Helper()
	dir := t.TempDir()
	if fixture.ReceiptAt == 0 {
		fixture.ReceiptAt = 11_000_000_000
	}
	if fixture.TerminalAt == 0 {
		fixture.TerminalAt = 100_000_000_000
	}
	if fixture.TailCensored {
		fixture.TerminalAt = 13_000_000_000
	}
	writeL0Config(t, dir, fixture.Disabled, fixture.PolicyMode)

	step := l0FixtureStep()
	side, price := "SELL", int64(300_000_000)
	orderSide := exchange.Sell
	if fixture.PolicyMode == liabilityHedgerPolicyRandom {
		control := rand.New(rand.NewSource(liabilityHedgerFlowSeed(101, 0, 0, 15)))
		if control.Intn(2) == 0 {
			side, price, orderSide = "BUY", 300_100_000, exchange.Buy
		}
	} else if step > 0 {
		side, price, orderSide = "BUY", 300_100_000, exchange.Buy
	}
	submitted := !fixture.Disabled && !fixture.TailCensored && !fixture.NoExecutableTouch && !fixture.DropGatewayDecision
	writeL0Evidence(t, dir, fixture.ReceiptAt, fixture.TerminalAt, submitted, orderSide, price)
	active := map[string]any{
		"venue_id": "north", "hedger": "liability_hedger_1", "client_id": uint64(7), "symbol": "CDF/USD",
		"decision_time": int64(12_000_000_000), "enabled": !fixture.Disabled, "subscribed": true, "request_pending": false,
		"action_or_defer_reason": "SUBMIT_IOC", "obligation_before": int64(0), "obligation_after": step, "obligation_step": step,
		"obligation_limit": int64(2_000_000_000), "position_before": int64(0), "hedge_gap": step,
		"decision_interval": int64(2_000_000_000), "obligation_interval": int64(10_000_000_000),
		"last_book_source_time": int64(10_000_000_000), "last_book_received_time": fixture.ReceiptAt, "last_book_sequence": uint64(7),
		"has_snapshot": true, "has_bid": true, "bid_price": int64(300_000_000), "bid_visible_qty": int64(1_000_000_000),
		"has_ask": true, "ask_price": int64(300_100_000), "ask_visible_qty": int64(1_000_000_000),
		"side": side, "limit_price": price, "requested_qty": int64(100_000_000), "request_id": uint64(42), "taker_fee_bps": int64(5),
		"outcome_expectation": "VENUE_OUTCOME_REQUIRED",
	}
	if fixture.PolicyMode != "" {
		active["policy_mode"] = fixture.PolicyMode
	}
	if fixture.Disabled {
		active["action_or_defer_reason"] = "POLICY_DISABLED"
		active["side"] = ""
		active["limit_price"] = int64(0)
		active["requested_qty"] = int64(0)
		active["request_id"] = uint64(0)
		active["outcome_expectation"] = ""
	}
	if fixture.TailCensored {
		active["action_or_defer_reason"] = "SIMULATION_HORIZON_CENSORED"
		active["side"] = ""
		active["limit_price"] = int64(0)
		active["requested_qty"] = int64(0)
		active["request_id"] = uint64(0)
		active["outcome_expectation"] = "SIMULATION_HORIZON_CENSORED"
		active["censor_reason"] = "terminal_horizon_before_round_trip"
	}
	if fixture.NoExecutableTouch {
		active["action_or_defer_reason"] = "LOCAL_EXECUTABLE_PRICE_UNAVAILABLE"
		active["limit_price"] = int64(0)
		active["request_id"] = uint64(0)
		active["outcome_expectation"] = ""
		if side == "BUY" {
			active["has_ask"] = false
			active["ask_price"] = int64(0)
			active["ask_visible_qty"] = int64(0)
		} else {
			active["has_bid"] = false
			active["bid_price"] = int64(0)
			active["bid_visible_qty"] = int64(0)
		}
	}
	if fixture.ReverseSide {
		if side == "BUY" {
			active["side"] = "SELL"
		} else {
			active["side"] = "BUY"
		}
	}
	for field, value := range fixture.Decision {
		active[field] = value
	}
	initial := map[string]any{
		"venue_id": "north", "hedger": "liability_hedger_1", "client_id": uint64(7), "symbol": "CDF/USD",
		"decision_time": int64(10_000_000_000), "enabled": !fixture.Disabled, "subscribed": false, "request_pending": false,
		"action_or_defer_reason": "NOT_SUBSCRIBED", "obligation_before": int64(0), "obligation_after": int64(0), "obligation_step": int64(0),
		"obligation_limit": int64(2_000_000_000), "position_before": int64(0), "hedge_gap": int64(0),
		"decision_interval": int64(2_000_000_000), "obligation_interval": int64(10_000_000_000),
		"last_book_source_time": int64(0), "last_book_received_time": int64(0), "last_book_sequence": uint64(0),
		"has_snapshot": false, "has_bid": false, "bid_price": int64(0), "bid_visible_qty": int64(0),
		"has_ask": false, "ask_price": int64(0), "ask_visible_qty": int64(0),
		"side": "", "limit_price": int64(0), "requested_qty": int64(0), "request_id": uint64(0), "taker_fee_bps": int64(5),
	}
	if fixture.PolicyMode != "" {
		initial["policy_mode"] = fixture.PolicyMode
	}

	lines := make([]string, 0, 8)
	if !fixture.DropInitialDecision {
		lines = append(lines, logLine(10_000_000_000, 7, "liability_hedger_decision", initial))
	}
	lines = append(lines, logLine(12_000_000_000, 7, "liability_hedger_decision", active))
	if fixture.DuplicateDecision {
		lines = append(lines, logLine(12_000_000_000, 7, "liability_hedger_decision", active))
	}
	if submitted {
		quantity := int64(100_000_000)
		if fixture.PartialFill {
			quantity = 50_000_000
		}
		fee, ok := liabilityHedgerFee(quantity, price, 5)
		if !ok {
			t.Fatal("fixture fee overflow")
		}
		postPosition := quantity
		counterSide := "SELL"
		if side == "SELL" {
			postPosition = -quantity
			counterSide = "BUY"
		}
		counterpartyClient := uint64(8)
		if fixture.CounterpartyClient != 0 {
			counterpartyClient = fixture.CounterpartyClient
		}
		order := map[string]any{"order_id": uint64(70), "client_id": uint64(7), "request_id": uint64(42), "symbol": "CDF/USD", "side": side, "type": "LIMIT", "time_in_force": "IOC", "post_only": false, "price": price, "qty": int64(100_000_000)}
		counter := map[string]any{"order_id": uint64(80), "client_id": counterpartyClient, "request_id": uint64(43), "symbol": "CDF/USD", "side": counterSide, "type": "LIMIT", "time_in_force": "GTC", "post_only": false, "price": price, "qty": quantity}
		fill := map[string]any{"order_id": uint64(70), "trade_id": uint64(9), "symbol": "CDF/USD", "side": side, "qty": quantity, "price": price, "fee_amount": fee, "fee_asset": "USD", "role": "taker"}
		fillEvidence := map[string]any{"venue_id": "north", "hedger": "liability_hedger_1", "client_id": uint64(7), "symbol": "CDF/USD", "timestamp": int64(13_000_000_000), "order_id": uint64(70), "trade_id": uint64(9), "side": side, "qty": quantity, "price": price, "fee_amount": fee, "fee_asset": "USD", "pre_position": int64(0), "post_position": postPosition}
		if fixture.PolicyMode != "" {
			fillEvidence["policy_mode"] = fixture.PolicyMode
		}
		for field, value := range fixture.Fill {
			fill[field] = value
		}
		for field, value := range fixture.FillEvidence {
			fillEvidence[field] = value
		}
		lines = append(lines,
			logLine(13_000_000_000, 7, "OrderAccepted", order),
			logLine(13_000_000_000, counterpartyClient, "OrderAccepted", counter),
			logLine(13_000_000_000, 0, "Trade", map[string]any{"trade_id": uint64(9), "price": price, "qty": quantity, "taker_order_id": uint64(70), "maker_order_id": uint64(80)}),
			logLine(13_000_000_000, 7, "OrderFill", fill),
			logLine(13_000_000_000, 7, "liability_hedger_fill", fillEvidence),
		)
		if fixture.PartialFill && !fixture.DropCancel {
			lines = append(lines, logLine(13_000_000_000, 7, "OrderCancelled", map[string]any{"order_id": uint64(70), "remaining_qty": int64(50_000_000)}))
		}
	}
	path := filepath.Join(dir, "general.jsonl")
	if err := os.WriteFile(path, []byte(joinLines(lines)), 0o644); err != nil {
		t.Fatal(err)
	}
	return &Run{Dir: dir, files: []string{path}, roles: map[Participant]string{{VenueID: "north", ClientID: 7}: "liability_hedger", {VenueID: "north", ClientID: 8}: "noise_flow"}}
}

func writeL0Config(t *testing.T, dir string, disabled bool, policyMode string) {
	t.Helper()
	policy := map[string]any{
		"enabled": !disabled, "symbol": "CDF/USD", "decision_interval": int64(2_000_000_000), "obligation_interval": int64(10_000_000_000),
		"obligation_step_qty": int64(200_000_000), "max_abs_obligation_qty": int64(2_000_000_000), "max_request_qty": int64(100_000_000),
	}
	if policyMode != "" {
		policy["policy_mode"] = policyMode
	}
	raw, err := json.Marshal(map[string]any{
		"seed": int64(101), "venue_ids": []string{"north", "central", "south"},
		"cdf_liability_hedger": policy,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "run-config.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeL0Evidence(t *testing.T, dir string, deliveredAt, terminalAt int64, submitted bool, side exchange.Side, price int64) {
	t.Helper()
	recorder, err := simulation.NewMarketDataReceiptRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	link := "north/liability_hedger/client/7"
	recorder.RegisterLink("north", link, "liability_hedger")
	schedule := simulation.MarketDataSchedule{ClientID: 7, SourceVenue: "north", Link: link, Symbol: "CDF/USD", Type: exchange.MDSnapshot, Sequence: 7, Fingerprint: [16]byte{1}, PublishedAt: 10_000_000_000, ScheduledAt: 10_000_000_000, LinkOrdinal: 1}
	recorder.RecordSchedule(schedule)
	frontier := recorder.RecordReceipt(simulation.MarketDataReceipt{MarketDataSchedule: schedule, DeliveredAt: deliveredAt})
	if submitted {
		recorder.RecordDecision(simulation.MarketDataDecision{
			ClientID: 7, SourceVenue: "north", Link: link, Symbol: "CDF/USD", RequestID: 42,
			Side: side, OrderType: exchange.LimitOrder, TimeInForce: exchange.IOC,
			Price: price, Qty: 100_000_000, DecisionAt: 12_000_000_000, Frontier: frontier,
		})
	}
	if err := recorder.Finalize(terminalAt); err != nil {
		t.Fatal(err)
	}
}

func l0FixtureStep() int64 {
	state := &liabilityHedgerReplayState{rng: rand.New(rand.NewSource(liabilityHedgerFlowSeed(101, 0, 0, 14)))}
	step, _, ok := liabilityHedgerNextStep(state)
	if !ok {
		panic("fixture L0 obligation transition")
	}
	return step
}
