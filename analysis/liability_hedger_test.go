package analysis

import (
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"exchange_sim/exchange"
	"exchange_sim/simulation"
)

func TestLiabilityHedgerAuditReplaysLocalStateReceiptAndFill(t *testing.T) {
	run := l0TestRun(t, l0Fixture{})
	result, err := run.MeasureLiabilityHedger()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.Decisions != 6 || result.StateUpdates != 3 || result.Submitted != 3 || result.Accepted != 3 || result.Fills != 3 || result.ReceiptMatches != 3 || result.ActionCounts["SUBMIT_IOC"] != 3 || len(result.Hedgers) != 3 || result.Hedgers[0].Accepted != 1 || result.Hedgers[0].Fills != 1 || len(result.Checks) != 0 {
		t.Fatalf("valid L0 evidence audit = %+v", result)
	}
}

func TestLiabilityHedgerAuditAcceptsZeroAsFirstTradeIdentity(t *testing.T) {
	result, err := l0TestRun(t, l0Fixture{ZeroTradeID: true}).MeasureLiabilityHedger()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.DuplicateTradeIdentities != 0 || result.DuplicateFillIdentities != 0 || result.LifecycleIdentityMismatches != 0 {
		t.Fatalf("zero-valued first trade identity was rejected: %+v", result)
	}
}

func TestLiabilityHedgerEventFilesExcludeUnrelatedBooks(t *testing.T) {
	run := &Run{Dir: "/run", files: []string{
		"/run/venues/north/general.jsonl",
		"/run/venues/north/spot/CDF-USD.jsonl",
		"/run/venues/north/spot/ABC-USD.jsonl",
		"/run/venues/north/derivatives.jsonl",
		"/run/venues/south/general.jsonl",
	}}
	got := liabilityHedgerEventFiles(run, []string{"north"})
	want := []string{
		"/run/venues/north/general.jsonl",
		"/run/venues/north/spot/CDF-USD.jsonl",
	}
	if len(got) != len(want) {
		t.Fatalf("liability event files = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("liability event files = %v, want %v", got, want)
		}
	}
}

func TestLiabilityHedgerAuditRejectsAdversarialLifecycleJoins(t *testing.T) {
	tests := []struct {
		name  string
		setup l0Fixture
		check func(*LiabilityHedgerAudit) bool
	}{
		{
			name:  "counterparty payload identity spoof",
			setup: l0Fixture{CounterpartyPayloadClient: 7},
			check: func(result *LiabilityHedgerAudit) bool { return result.CounterpartyIdentityMismatches > 0 },
		},
		{
			name:  "wrong role fill envelope",
			setup: l0Fixture{FillEnvelopeClient: 8},
			check: func(result *LiabilityHedgerAudit) bool { return result.LifecycleIdentityMismatches > 0 },
		},
		{
			name:  "trade precedes order acceptance",
			setup: l0Fixture{TradeBeforeOrder: true},
			check: func(result *LiabilityHedgerAudit) bool { return result.CausalOrderMismatches > 0 },
		},
		{
			name:  "duplicate order acceptance",
			setup: l0Fixture{DuplicateOrder: true},
			check: func(result *LiabilityHedgerAudit) bool {
				return result.DuplicateOrderIdentities > 0 || result.DuplicateOutcomes > 0
			},
		},
		{
			name:  "trade without liability fill",
			setup: l0Fixture{DropOrderFill: true},
			check: func(result *LiabilityHedgerAudit) bool { return result.MissingTradeFills > 0 },
		},
		{
			name:  "duplicate trade identity",
			setup: l0Fixture{DuplicateTrade: true},
			check: func(result *LiabilityHedgerAudit) bool { return result.DuplicateTradeIdentities > 0 },
		},
		{
			name:  "duplicate fill identity",
			setup: l0Fixture{DuplicateFill: true},
			check: func(result *LiabilityHedgerAudit) bool { return result.DuplicateFillIdentities > 0 },
		},
		{
			name:  "fill precedes trade",
			setup: l0Fixture{FillBeforeTrade: true},
			check: func(result *LiabilityHedgerAudit) bool { return result.CausalOrderMismatches > 0 },
		},
		{
			name:  "counterparty symbol mutation",
			setup: l0Fixture{CounterpartySymbol: "ABC/USD"},
			check: func(result *LiabilityHedgerAudit) bool { return result.CounterpartyFieldMismatches > 0 },
		},
		{
			name:  "counterparty quantity mutation",
			setup: l0Fixture{CounterpartyQty: 1},
			check: func(result *LiabilityHedgerAudit) bool { return result.CounterpartyFieldMismatches > 0 },
		},
		{
			name:  "cancellation precedes acceptance",
			setup: l0Fixture{PartialFill: true, CancelBeforeOrder: true},
			check: func(result *LiabilityHedgerAudit) bool { return result.CausalOrderMismatches > 0 },
		},
		{
			name:  "swapped trade maker and taker",
			setup: l0Fixture{SwapTradeOrders: true},
			check: func(result *LiabilityHedgerAudit) bool { return result.TradeRoleMismatches > 0 },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := l0TestRun(t, tc.setup).MeasureLiabilityHedger()
			if err != nil {
				t.Fatal(err)
			}
			if result.Valid || !tc.check(result) {
				t.Fatalf("adversarial lifecycle mutation survived: %+v", result)
			}
		})
	}
}

func TestLiabilityHedgerAuditRejectsActorEvidenceInBookFile(t *testing.T) {
	run := l0TestRun(t, l0Fixture{})
	actorPath := filepath.Join(run.Dir, "venues", "north", "general.jsonl")
	bookPath := filepath.Join(run.Dir, "venues", "north", "spot", "CDF-USD.jsonl")
	actorEvidence, err := os.ReadFile(actorPath)
	if err != nil {
		t.Fatal(err)
	}
	book, err := os.OpenFile(bookPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := book.Write(actorEvidence); err != nil {
		_ = book.Close()
		t.Fatal(err)
	}
	if err := book.Close(); err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureLiabilityHedger()
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid || result.WrongEvidenceFiles == 0 {
		t.Fatalf("wrong-file actor evidence survived: %+v", result)
	}
}

func TestLiabilityHedgerAuditRejectsMissingConfiguredParticipant(t *testing.T) {
	run := l0TestRun(t, l0Fixture{})
	missingActorPath := filepath.Join(run.Dir, "venues", "central", "general.jsonl")
	missingBookPath := filepath.Join(run.Dir, "venues", "central", "spot", "CDF-USD.jsonl")
	retainedFiles := make([]string, 0, len(run.files)-2)
	for _, path := range run.files {
		if path == missingActorPath || path == missingBookPath {
			continue
		}
		retainedFiles = append(retainedFiles, path)
	}
	run.files = retainedFiles
	result, err := run.MeasureLiabilityHedger()
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid || result.ConfiguredParticipantMismatches == 0 {
		t.Fatalf("missing configured participant survived: %+v", result)
	}
}

func TestLiabilityHedgerAuditRejectsMissingConfiguredBookOnly(t *testing.T) {
	run := l0TestRun(t, l0Fixture{})
	missingBookPath := filepath.Join(run.Dir, "venues", "central", "spot", "CDF-USD.jsonl")
	retainedFiles := make([]string, 0, len(run.files)-1)
	for _, path := range run.files {
		if path == missingBookPath {
			continue
		}
		retainedFiles = append(retainedFiles, path)
	}
	run.files = retainedFiles
	result, err := run.MeasureLiabilityHedger()
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid || result.ConfiguredParticipantMismatches == 0 {
		t.Fatalf("missing configured book survived: %+v", result)
	}
}

func TestLiabilityHedgerAuditRejectsRelocatedBookFile(t *testing.T) {
	run := l0TestRun(t, l0Fixture{})
	canonicalPath := filepath.Join(run.Dir, "venues", "central", "spot", "CDF-USD.jsonl")
	relocatedPath := filepath.Join(run.Dir, "venues", "central", "archive", "CDF-USD.jsonl")
	raw, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(relocatedPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(relocatedPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	retainedFiles := make([]string, 0, len(run.files))
	for _, path := range run.files {
		if path == canonicalPath {
			retainedFiles = append(retainedFiles, relocatedPath)
			continue
		}
		retainedFiles = append(retainedFiles, path)
	}
	run.files = retainedFiles
	result, err := run.MeasureLiabilityHedger()
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid || result.ConfiguredParticipantMismatches == 0 {
		t.Fatalf("relocated configured book survived: %+v", result)
	}
}

func TestLiabilityHedgerAuditDisabledControlEvolvesButDoesNotAct(t *testing.T) {
	run := l0TestRun(t, l0Fixture{Disabled: true})
	result, err := run.MeasureLiabilityHedger()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.Decisions != 6 || result.DisabledDecisions != 6 || result.StateUpdates != 3 || result.Submitted != 0 || result.Accepted != 0 || result.Fills != 0 || result.ActionCounts["NOT_SUBSCRIBED"] != 3 || result.ActionCounts["POLICY_DISABLED"] != 3 || len(result.Checks) != 0 {
		t.Fatalf("disabled L0 evidence audit = %+v", result)
	}
}

func TestLiabilityHedgerAuditReplaysRandomSideControlAndReportsItsGapDirection(t *testing.T) {
	run := l0TestRun(t, l0Fixture{PolicyMode: liabilityHedgerPolicyRandom})
	result, err := run.MeasureLiabilityHedger()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.PolicyMode != liabilityHedgerPolicyRandom || result.RandomControlFills != 3 || result.RandomControlReducing+result.RandomControlNonReducing != 3 || result.NonReducingFills != 0 {
		t.Fatalf("valid L1 random-control audit = %+v", result)
	}
}

func TestLiabilityHedgerAuditAcceptsExplicitUnavailableTouchWithoutRequest(t *testing.T) {
	run := l0TestRun(t, l0Fixture{PolicyMode: liabilityHedgerPolicyRandom, NoExecutableTouch: true})
	result, err := run.MeasureLiabilityHedger()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.Deferred != 6 || result.Submitted != 0 || result.DecisionFieldMismatches != 0 || result.ActionCounts["LOCAL_EXECUTABLE_PRICE_UNAVAILABLE"] != 3 || len(result.Checks) != 0 {
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

func TestLiabilityHedgerAuditAcceptsExactZeroFeeRounding(t *testing.T) {
	run := l0TestRun(t, l0Fixture{TinyPartialFill: true})
	result, err := run.MeasureLiabilityHedger()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.Fills != 3 || result.FeeMismatches != 0 || result.NonPositiveFees != 0 || len(result.Checks) != 0 {
		t.Fatalf("exact zero-fee partial fill = %+v", result)
	}

	// The zero-fee representation is exact: assigning it a quote asset is as
	// invalid as omitting the asset of a positive fee.
	mutated := l0TestRun(t, l0Fixture{
		TinyPartialFill: true,
		Fill:            map[string]any{"fee_asset": "USD"},
		FillEvidence:    map[string]any{"fee_asset": "USD"},
	})
	result, err = mutated.MeasureLiabilityHedger()
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid || result.FeeMismatches == 0 {
		t.Fatalf("zero-fee asset mutation survived: %+v", result)
	}
}

func TestLiabilityHedgerAuditAcceptsOnlyARealTailCensor(t *testing.T) {
	run := l0TestRun(t, l0Fixture{TailCensored: true})
	result, err := run.MeasureLiabilityHedger()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.HorizonCensored != 3 || result.Submitted != 0 || result.ActionCounts["SIMULATION_HORIZON_CENSORED"] != 3 {
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

func TestLiabilityHedgerDecisionPhaseEvidenceRejectsMissingMismatchedAndOffPhaseRows(t *testing.T) {
	offset := int64(time.Second)
	d := liabilityHedgerDecision{
		DecisionTime:        liabilityHedgerSimulationStart + liabilityHedgerDecisionInterval + offset,
		DecisionPhaseOffset: &offset,
	}
	if !liabilityHedgerDecisionMatchesPhase(d, offset, true) {
		t.Fatal("configured 1s phase decision was rejected")
	}
	d.DecisionPhaseOffset = nil
	if liabilityHedgerDecisionMatchesPhase(d, offset, true) {
		t.Fatal("missing explicit configured phase survived")
	}
	wrong := int64(0)
	d.DecisionPhaseOffset = &wrong
	if liabilityHedgerDecisionMatchesPhase(d, offset, true) {
		t.Fatal("mismatched phase field survived")
	}
	d.DecisionPhaseOffset = &offset
	d.DecisionTime--
	if liabilityHedgerDecisionMatchesPhase(d, offset, true) {
		t.Fatal("off-phase decision timestamp survived")
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
	Disabled                  bool
	PolicyMode                string
	DropInitialDecision       bool
	DuplicateDecision         bool
	DropGatewayDecision       bool
	TailCensored              bool
	NoExecutableTouch         bool
	ReverseSide               bool
	PartialFill               bool
	TinyPartialFill           bool
	DropCancel                bool
	DropOrderFill             bool
	DuplicateOrder            bool
	DuplicateTrade            bool
	DuplicateFill             bool
	SwapTradeOrders           bool
	TradeBeforeOrder          bool
	FillBeforeTrade           bool
	CancelBeforeOrder         bool
	CounterpartyClient        uint64
	CounterpartyPayloadClient uint64
	CounterpartySymbol        string
	CounterpartyQty           int64
	FillEnvelopeClient        uint64
	ReceiptAt                 int64
	TerminalAt                int64
	Decision                  map[string]any
	Fill                      map[string]any
	FillEvidence              map[string]any
	ZeroTradeID               bool
}

func liabilityHedgerLogLine(venue string, ts int64, clientID uint64, event string, payload map[string]any) string {
	raw, err := json.Marshal(map[string]any{
		"client_id": clientID, "event": event, "sim_ts": ts,
		"data": map[string]any{"venue_id": venue, "payload": payload},
	})
	if err != nil {
		panic(err)
	}
	return string(raw)
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
	if fixture.PolicyMode != liabilityHedgerPolicyRandom && step > 0 {
		side, price, orderSide = "BUY", 300_100_000, exchange.Buy
	}
	submitted := !fixture.Disabled && !fixture.TailCensored && !fixture.NoExecutableTouch && !fixture.DropGatewayDecision
	writeL0Evidence(t, dir, fixture.ReceiptAt, fixture.TerminalAt, submitted, fixture.PolicyMode, side, orderSide, price)
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

	venueIDs := []string{"north", "central", "south"}
	actorLines := make([]string, 0, 9)
	bookLines := make([]string, 0, 18)
	actorRanges := make(map[string][2]int, len(venueIDs))
	bookRanges := make(map[string][2]int, len(venueIDs))
	for venueIndex, venueID := range venueIDs {
		actorStart := len(actorLines)
		bookStart := len(bookLines)
		venueSide, venuePrice := side, price
		if fixture.PolicyMode == liabilityHedgerPolicyRandom {
			control := rand.New(rand.NewSource(liabilityHedgerFlowSeed(101, venueIndex, 0, 15)))
			if control.Intn(2) == 0 {
				venueSide, venuePrice = "BUY", 300_100_000
			}
		}
		if fixture.ReverseSide {
			if venueSide == "BUY" {
				venueSide = "SELL"
			} else {
				venueSide = "BUY"
			}
		}
		active["venue_id"] = venueID
		initial["venue_id"] = venueID
		active["side"] = venueSide
		active["limit_price"] = venuePrice
		if fixture.Disabled || fixture.TailCensored {
			active["side"] = ""
			active["limit_price"] = int64(0)
		}
		if fixture.NoExecutableTouch {
			active["limit_price"] = int64(0)
		}
		active["has_bid"] = true
		active["bid_price"] = int64(300_000_000)
		active["bid_visible_qty"] = int64(1_000_000_000)
		active["has_ask"] = true
		active["ask_price"] = int64(300_100_000)
		active["ask_visible_qty"] = int64(1_000_000_000)
		if fixture.NoExecutableTouch {
			if venueSide == "BUY" {
				active["has_ask"] = false
				active["ask_price"] = int64(0)
				active["ask_visible_qty"] = int64(0)
			} else {
				active["has_bid"] = false
				active["bid_price"] = int64(0)
				active["bid_visible_qty"] = int64(0)
			}
		}
		for field, value := range fixture.Decision {
			active[field] = value
		}
		if !fixture.DropInitialDecision {
			actorLines = append(actorLines, liabilityHedgerLogLine(venueID, 10_000_000_000, 7, "liability_hedger_decision", initial))
		}
		actorLines = append(actorLines, liabilityHedgerLogLine(venueID, 12_000_000_000, 7, "liability_hedger_decision", active))
		if fixture.DuplicateDecision {
			actorLines = append(actorLines, liabilityHedgerLogLine(venueID, 12_000_000_000, 7, "liability_hedger_decision", active))
		}
		if submitted {
			quantity := int64(100_000_000)
			if fixture.PartialFill {
				quantity = 50_000_000
			}
			if fixture.TinyPartialFill {
				quantity = 1
			}
			fee, ok := liabilityHedgerFee(quantity, venuePrice, 5)
			if !ok {
				t.Fatal("fixture fee overflow")
			}
			postPosition := quantity
			counterSide := "SELL"
			if venueSide == "SELL" {
				postPosition = -quantity
				counterSide = "BUY"
			}
			counterpartyClient := uint64(8)
			if fixture.CounterpartyClient != 0 {
				counterpartyClient = fixture.CounterpartyClient
			}
			counterpartyPayloadClient := counterpartyClient
			if fixture.CounterpartyPayloadClient != 0 {
				counterpartyPayloadClient = fixture.CounterpartyPayloadClient
			}
			counterpartySymbol := "CDF/USD"
			if fixture.CounterpartySymbol != "" {
				counterpartySymbol = fixture.CounterpartySymbol
			}
			counterpartyQty := quantity
			if fixture.CounterpartyQty != 0 {
				counterpartyQty = fixture.CounterpartyQty
			}
			fillEnvelopeClient := uint64(7)
			if fixture.FillEnvelopeClient != 0 {
				fillEnvelopeClient = fixture.FillEnvelopeClient
			}
			order := map[string]any{"order_id": uint64(70), "client_id": uint64(7), "request_id": uint64(42), "symbol": "CDF/USD", "side": venueSide, "type": "LIMIT", "time_in_force": "IOC", "post_only": false, "price": venuePrice, "qty": int64(100_000_000)}
			counter := map[string]any{"order_id": uint64(80), "client_id": counterpartyPayloadClient, "request_id": uint64(43), "symbol": counterpartySymbol, "side": counterSide, "type": "LIMIT", "time_in_force": "GTC", "post_only": false, "price": venuePrice, "qty": counterpartyQty}
			feeAsset := "USD"
			if fee == 0 {
				feeAsset = ""
			}
			fill := map[string]any{"order_id": uint64(70), "trade_id": uint64(9), "symbol": "CDF/USD", "side": venueSide, "qty": quantity, "price": venuePrice, "fee_amount": fee, "fee_asset": feeAsset, "role": "taker"}
			tradeID := uint64(9)
			if fixture.ZeroTradeID {
				tradeID = 0
			}
			fill["trade_id"] = tradeID
			fillEvidence := map[string]any{"venue_id": venueID, "hedger": "liability_hedger_1", "client_id": uint64(7), "symbol": "CDF/USD", "timestamp": int64(13_000_000_000), "order_id": uint64(70), "trade_id": uint64(9), "side": side, "qty": quantity, "price": price, "fee_amount": fee, "fee_asset": feeAsset, "pre_position": int64(0), "post_position": postPosition}
			fillEvidence["trade_id"] = tradeID
			fillEvidence["side"] = venueSide
			fillEvidence["price"] = venuePrice
			if fixture.PolicyMode != "" {
				fillEvidence["policy_mode"] = fixture.PolicyMode
			}
			for field, value := range fixture.Fill {
				fill[field] = value
			}
			for field, value := range fixture.FillEvidence {
				fillEvidence[field] = value
			}
			orderAcceptedLine := liabilityHedgerLogLine(venueID, 13_000_000_000, 7, "OrderAccepted", order)
			counterpartyAcceptedLine := liabilityHedgerLogLine(venueID, 13_000_000_000, counterpartyClient, "OrderAccepted", counter)
			takerOrderID, makerOrderID := uint64(70), uint64(80)
			if fixture.SwapTradeOrders {
				takerOrderID, makerOrderID = makerOrderID, takerOrderID
			}
			tradeLine := liabilityHedgerLogLine(venueID, 13_000_000_000, 0, "Trade", map[string]any{"trade_id": tradeID, "price": venuePrice, "qty": quantity, "taker_order_id": takerOrderID, "maker_order_id": makerOrderID})
			fillLine := liabilityHedgerLogLine(venueID, 13_000_000_000, fillEnvelopeClient, "OrderFill", fill)
			cancelLine := liabilityHedgerLogLine(venueID, 13_000_000_000, 7, "OrderCancelled", map[string]any{"order_id": uint64(70), "remaining_qty": int64(100_000_000) - quantity})
			if fixture.CancelBeforeOrder && (fixture.PartialFill || fixture.TinyPartialFill) && !fixture.DropCancel {
				bookLines = append(bookLines, cancelLine)
			}
			if fixture.TradeBeforeOrder {
				bookLines = append(bookLines, tradeLine, orderAcceptedLine, counterpartyAcceptedLine)
			} else {
				bookLines = append(bookLines, orderAcceptedLine, counterpartyAcceptedLine)
				if fixture.FillBeforeTrade && !fixture.DropOrderFill {
					bookLines = append(bookLines, fillLine)
				}
				bookLines = append(bookLines, tradeLine)
			}
			if !fixture.DropOrderFill && (!fixture.FillBeforeTrade || fixture.TradeBeforeOrder) {
				bookLines = append(bookLines, fillLine)
			}
			if fixture.DuplicateFill && !fixture.DropOrderFill {
				bookLines = append(bookLines, fillLine)
			}
			if fixture.DuplicateTrade {
				bookLines = append(bookLines, tradeLine)
			}
			actorLines = append(actorLines, liabilityHedgerLogLine(venueID, 13_000_000_000, 7, "liability_hedger_fill", fillEvidence))
			if fixture.DuplicateOrder {
				bookLines = append(bookLines, orderAcceptedLine)
			}
			if (fixture.PartialFill || fixture.TinyPartialFill) && !fixture.DropCancel && !fixture.CancelBeforeOrder {
				bookLines = append(bookLines, cancelLine)
			}
		}
		actorRanges[venueID] = [2]int{actorStart, len(actorLines)}
		bookRanges[venueID] = [2]int{bookStart, len(bookLines)}
	}
	roles := make(map[Participant]string, len(venueIDs)*2)
	files := make([]string, 0, len(venueIDs)*2)
	for _, venueID := range venueIDs {
		actorPath := filepath.Join(dir, "venues", venueID, "general.jsonl")
		bookPath := filepath.Join(dir, "venues", venueID, "spot", "CDF-USD.jsonl")
		if err := os.MkdirAll(filepath.Dir(bookPath), 0o755); err != nil {
			t.Fatal(err)
		}
		actorRange := actorRanges[venueID]
		bookRange := bookRanges[venueID]
		if err := os.WriteFile(actorPath, []byte(joinLines(actorLines[actorRange[0]:actorRange[1]])), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(bookPath, []byte(joinLines(bookLines[bookRange[0]:bookRange[1]])), 0o644); err != nil {
			t.Fatal(err)
		}
		roles[Participant{VenueID: venueID, ClientID: 7}] = "liability_hedger"
		roles[Participant{VenueID: venueID, ClientID: 8}] = "noise_flow"
		files = append(files, actorPath, bookPath)
	}
	return &Run{Dir: dir, files: files, roles: roles}
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

func writeL0Evidence(t *testing.T, dir string, deliveredAt, terminalAt int64, submitted bool, policyMode, side string, orderSide exchange.Side, price int64) {
	t.Helper()
	recorder, err := simulation.NewMarketDataReceiptRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	for venueIndex, venueID := range []string{"north", "central", "south"} {
		link := venueID + "/liability_hedger/client/7"
		recorder.RegisterLink(venueID, link, "liability_hedger")
		schedule := simulation.MarketDataSchedule{ClientID: 7, SourceVenue: venueID, Link: link, Symbol: "CDF/USD", Type: exchange.MDSnapshot, Sequence: 7, Fingerprint: [16]byte{1}, PublishedAt: 10_000_000_000, ScheduledAt: 10_000_000_000, LinkOrdinal: 1}
		recorder.RecordSchedule(schedule)
		frontier := recorder.RecordReceipt(simulation.MarketDataReceipt{MarketDataSchedule: schedule, DeliveredAt: deliveredAt})
		if submitted {
			venuePrice, venueOrderSide := price, orderSide
			if policyMode == liabilityHedgerPolicyRandom {
				control := rand.New(rand.NewSource(liabilityHedgerFlowSeed(101, venueIndex, 0, 15)))
				if control.Intn(2) == 0 {
					venuePrice, venueOrderSide = 300_100_000, exchange.Buy
				}
			}
			recorder.RecordDecision(simulation.MarketDataDecision{
				ClientID: 7, SourceVenue: venueID, Link: link, Symbol: "CDF/USD", RequestID: 42,
				Side: venueOrderSide, OrderType: exchange.LimitOrder, TimeInForce: exchange.IOC,
				Price: venuePrice, Qty: 100_000_000, DecisionAt: 12_000_000_000, Frontier: frontier,
			})
		}
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
