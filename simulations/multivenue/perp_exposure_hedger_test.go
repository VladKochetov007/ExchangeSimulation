package multivenue

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/analysis"
	"exchange_sim/exchange"
	"exchange_sim/simulation"
)

func perpExposureTestConfig() PerpExposureHedgerConfig {
	return PerpExposureHedgerConfig{
		Enabled: true, Symbol: "ABC-PERP", DecisionInterval: time.Second, ExposureInterval: 10 * time.Second,
		ExposureStepQty: 10, MaxAbsExposure: 50, MaxRequestQty: 10, TickSize: 1,
		InitialQuoteBalance: 100_000, InitialMargin: 50_000, Seed: 17,
		VenueID: "north", Hedger: "perp_exposure_hedger_1", ClientID: 12, TakerFeeBps: 5,
	}
}

func observePerpExposureBook(h *PerpExposureHedger, gateway *fundingCarryStubGateway, now time.Time, bid, ask int64) {
	gateway.frontier = simulation.MarketDataFrontier{LinkID: 9, Ordinal: 4, DeliveredAt: now.UnixNano(), Digest: [16]byte{9}}
	h.HandleEvent(context.Background(), &actor.Event{Type: actor.EventBookSnapshot, Data: actor.BookSnapshotEvent{
		Symbol: "ABC-PERP", Timestamp: now.UnixNano(), SeqNum: 41,
		Snapshot: &exchange.BookSnapshot{Bids: []exchange.PriceLevel{{Price: bid, VisibleQty: 100}}, Asks: []exchange.PriceLevel{{Price: ask, VisibleQty: 100}}},
	}})
}

func TestPerpExposureHedgerTargetsOppositePhysicalExposureAtLocalTouch(t *testing.T) {
	for _, tc := range []struct {
		name         string
		physical     int64
		wantSide     exchange.Side
		wantPrice    int64
		wantPosition int64
	}{
		{"producer physical long shorts perp", 10, exchange.Sell, 100, -10},
		{"consumer physical short buys perp", -10, exchange.Buy, 101, 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gateway := newFundingCarryStubGateway()
			var decisions []PerpExposureHedgerDecision
			var fills []PerpExposureHedgerFill
			cfg := perpExposureTestConfig()
			cfg.DecisionObserver = func(decision PerpExposureHedgerDecision) { decisions = append(decisions, decision) }
			cfg.FillObserver = func(fill PerpExposureHedgerFill) { fills = append(fills, fill) }
			h := NewPerpExposureHedger(1, gateway, cfg)
			now := time.Unix(10, 0)
			h.subscribed, h.lastUpdate, h.physicalExposure = true, now.UnixNano(), tc.physical
			observePerpExposureBook(h, gateway, now, 100, 101)

			h.onTick(now.Add(time.Second))
			if len(gateway.requests) != 1 || gateway.requests[0].OrderReq == nil {
				t.Fatalf("perp hedge did not submit exactly one request: %+v", gateway.requests)
			}
			decision := decisions[len(decisions)-1]
			request := gateway.requests[0].OrderReq
			if decision.ActionOrDeferReason != "SUBMIT_IOC" || decision.TargetPerpPosition != -tc.physical ||
				decision.PolicyVersion != perpExposureHedgerPolicyVersion ||
				decision.Side != tc.wantSide.String() || decision.LimitPrice != tc.wantPrice || decision.RequestedQty != 10 ||
				decision.DecisionFrontierLinkID != 9 || decision.DecisionFrontierOrdinal != 4 ||
				len(decision.BookFingerprint) != 32 || decision.BookFingerprint == "00000000000000000000000000000000" ||
				request.RequestID != decision.RequestID || request.Symbol != "ABC-PERP" || request.Side != tc.wantSide ||
				request.Price != tc.wantPrice || request.Qty != 10 || request.TimeInForce != exchange.IOC {
				t.Fatalf("physical target/local order mismatch: decision=%+v request=%+v", decision, request)
			}

			h.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{RequestID: request.RequestID, OrderID: 77}})
			h.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderFilled, Data: actor.OrderFillEvent{
				OrderID: 77, Symbol: "ABC-PERP", Side: tc.wantSide, Qty: 10, Price: tc.wantPrice, TradeID: 88, IsFull: true, Timestamp: now.Add(2 * time.Second).UnixNano(),
			}})
			if h.PerpPosition() != tc.wantPosition || len(fills) != 1 || fills[0].PrePosition != 0 || fills[0].PostPosition != tc.wantPosition {
				t.Fatalf("actor did not attest the filled perp transition: position=%d fills=%+v", h.PerpPosition(), fills)
			}
		})
	}
}

func TestPerpExposureHedgerDisabledUpdatesStateButCannotSubmit(t *testing.T) {
	gateway := newFundingCarryStubGateway()
	var decisions []PerpExposureHedgerDecision
	cfg := perpExposureTestConfig()
	cfg.Enabled = false
	cfg.DecisionObserver = func(decision PerpExposureHedgerDecision) { decisions = append(decisions, decision) }
	h := NewPerpExposureHedger(1, gateway, cfg)
	now := time.Unix(10, 0)
	h.subscribed = true
	observePerpExposureBook(h, gateway, now, 100, 101)

	h.onTick(now)
	decision := decisions[len(decisions)-1]
	if decision.ActionOrDeferReason != "POLICY_DISABLED" || decision.PhysicalStep == 0 || decision.PhysicalAfter == 0 ||
		decision.TargetPerpPosition != -decision.PhysicalAfter || len(gateway.requests) != 0 {
		t.Fatalf("disabled actor did not retain auditable state-only behavior: decision=%+v requests=%+v", decision, gateway.requests)
	}
}

func TestPerpExposureHedgerKeepsPresentZeroTouchDistinctFromAbsence(t *testing.T) {
	gateway := newFundingCarryStubGateway()
	var decisions []PerpExposureHedgerDecision
	cfg := perpExposureTestConfig()
	cfg.DecisionObserver = func(decision PerpExposureHedgerDecision) { decisions = append(decisions, decision) }
	h := NewPerpExposureHedger(1, gateway, cfg)
	now := time.Unix(10, 0)
	h.subscribed, h.lastUpdate, h.physicalExposure = true, now.UnixNano(), 10
	observePerpExposureBook(h, gateway, now, 0, 1)

	h.onTick(now.Add(time.Second))
	decision := decisions[len(decisions)-1]
	if decision.ActionOrDeferReason != "PERP_PRICE_OUTSIDE_DOMAIN" || !decision.HasBid || decision.BidPrice != 0 || len(gateway.requests) != 0 {
		t.Fatalf("present zero bid became absent or tradable: decision=%+v requests=%+v", decision, gateway.requests)
	}
}

func TestPerpExposureHedgerConfigRejectsAmbiguousOrUnsafeContract(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*PerpExposureHedgerConfig)
	}{
		{"wrong symbol", func(c *PerpExposureHedgerConfig) { c.Symbol = "ABC/USD" }},
		{"nonmultiple exposure clock", func(c *PerpExposureHedgerConfig) { c.ExposureInterval = 1500 * time.Millisecond }},
		{"zero prefunding", func(c *PerpExposureHedgerConfig) { c.InitialMargin = 0 }},
		{"zero price grid", func(c *PerpExposureHedgerConfig) { c.TickSize = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := perpExposureTestConfig()
			tc.edit(&cfg)
			if err := cfg.validate(); err == nil {
				t.Fatal("invalid P2 policy accepted")
			}
		})
	}
}

func TestPerpExposureHedgerFixedLiabilityEntersOnceAndThenHolds(t *testing.T) {
	gateway := newFundingCarryStubGateway()
	var decisions []PerpExposureHedgerDecision
	cfg := perpExposureTestConfig()
	cfg.ExposureMode = fixedLiabilityExposureMode
	cfg.InitialPhysicalExposure = -20
	cfg.DecisionObserver = func(decision PerpExposureHedgerDecision) { decisions = append(decisions, decision) }
	h := NewPerpExposureHedger(1, gateway, cfg)
	if h.PhysicalExposure() != -20 {
		t.Fatalf("fixed liability was not installed: %d", h.PhysicalExposure())
	}
	now := time.Unix(10, 0)
	h.onTick(now)
	if len(decisions) != 1 || decisions[0].PolicyVersion != fixedLiabilityHedgerPolicyVersion || decisions[0].ExposureMode != fixedLiabilityExposureMode || decisions[0].PhysicalBefore != -20 || decisions[0].PhysicalAfter != -20 || decisions[0].ActionOrDeferReason != "NOT_SUBSCRIBED" {
		t.Fatalf("fixed initial decision is not auditable: %+v", decisions)
	}
	observePerpExposureBook(h, gateway, now, 100, 101)
	h.onTick(now.Add(time.Second))
	if len(gateway.requests) < 2 || len(decisions) != 2 {
		t.Fatalf("fixed liability did not submit its entry IOC: requests=%+v decisions=%+v", gateway.requests, decisions)
	}
	decision := decisions[1]
	request := gateway.requests[len(gateway.requests)-1].OrderReq
	if decision.ActionOrDeferReason != "SUBMIT_IOC" || decision.Side != exchange.Buy.String() || decision.RequestedQty != cfg.MaxRequestQty || decision.TargetPerpPosition != 20 || decision.PolicyVersion != fixedLiabilityHedgerPolicyVersion {
		t.Fatalf("fixed entry decision mismatch: %+v", decision)
	}
	h.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{RequestID: request.RequestID, OrderID: 77}})
	h.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderFilled, Data: actor.OrderFillEvent{OrderID: 77, Symbol: cfg.Symbol, Side: exchange.Buy, Qty: cfg.MaxRequestQty, Price: 101, TradeID: 88, IsFull: true, Timestamp: now.Add(2 * time.Second).UnixNano()}})
	if h.PerpPosition() != cfg.MaxRequestQty || h.entryComplete {
		t.Fatalf("partial fixed entry unexpectedly became complete: position=%d complete=%t", h.PerpPosition(), h.entryComplete)
	}
	// A second IOC completes the fixed target; only then is the policy held.
	h.onTick(now.Add(3 * time.Second))
	if len(decisions) != 3 || decisions[2].ActionOrDeferReason != "SUBMIT_IOC" || len(gateway.requests) != 3 {
		t.Fatalf("fixed liability did not continue its bounded entry: decision=%+v requests=%+v", decisions[2], gateway.requests)
	}
	request = gateway.requests[len(gateway.requests)-1].OrderReq
	h.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{RequestID: request.RequestID, OrderID: 78}})
	h.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderFilled, Data: actor.OrderFillEvent{OrderID: 78, Symbol: cfg.Symbol, Side: exchange.Buy, Qty: cfg.MaxRequestQty, Price: 101, TradeID: 89, IsFull: true, Timestamp: now.Add(4 * time.Second).UnixNano()}})
	if h.PerpPosition() != 2*cfg.MaxRequestQty || !h.entryComplete {
		t.Fatalf("fixed entry did not become complete: position=%d complete=%t", h.PerpPosition(), h.entryComplete)
	}
	h.onTick(now.Add(5 * time.Second))
	if len(decisions) != 4 || decisions[3].ActionOrDeferReason != "FIXED_LIABILITY_HELD" || decisions[3].RequestID != 0 || len(gateway.requests) != 3 {
		t.Fatalf("fixed liability reopened after entry: decision=%+v requests=%+v", decisions[3], gateway.requests)
	}
}

func TestPerpExposureHedgerFixedLiabilityConfigRequiresBoundedNonzeroExposure(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*PerpExposureHedgerConfig)
	}{
		{"zero physical exposure", func(c *PerpExposureHedgerConfig) {
			c.ExposureMode, c.InitialPhysicalExposure = fixedLiabilityExposureMode, 0
		}},
		{"exposure above bound", func(c *PerpExposureHedgerConfig) {
			c.ExposureMode, c.InitialPhysicalExposure = fixedLiabilityExposureMode, c.MaxAbsExposure+1
		}},
		{"unknown mode", func(c *PerpExposureHedgerConfig) { c.ExposureMode = "random_shock" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := perpExposureTestConfig()
			tc.edit(&cfg)
			if err := cfg.validate(); err == nil {
				t.Fatal("invalid fixed-liability policy accepted")
			}
		})
	}
}

func TestPerpExposureHedgerConfigRequiresAuditedDelayedFeed(t *testing.T) {
	policy := perpExposureTestConfig()
	base := Config{LogDir: t.TempDir(), LogMode: "full", PerpExposureHedger: &policy}
	if _, err := NewSim(time.Second, base); err == nil {
		t.Fatal("P2 accepted without an explicit participant-local feed")
	}
	base.LatencyProfiles = map[string]LatencyProfile{"perp_exposure_hedger": {Model: "constant", Delay: time.Millisecond}}
	sim, err := NewSim(time.Second, base)
	if err != nil {
		t.Fatalf("P2 recorder-neutral path rejected documented local feed: %v", err)
	}
	sim.Close()
	base.RecordPerpExposureHedgerDecisions = true
	if _, err := NewSim(time.Second, base); err == nil {
		t.Fatal("instrumented P2 accepted without a strict participant-role roster")
	}
	base.StrictPopulationAccounting = true
	if _, err := NewSim(time.Second, base); err == nil {
		t.Fatal("instrumented P2 accepted without independently recorded local feed receipts")
	}
	base.RecordMarketDataReceipts = true
	base.MarketDataReceiptRoles = []string{"perp_exposure_hedger"}
	sim, err = NewSim(time.Second, base)
	if err != nil {
		t.Fatalf("P2 rejected documented receipt path: %v", err)
	}
	defer sim.Close()
	for _, venue := range sim.Venues {
		if len(venue.PerpExposureHedgers) != 1 {
			t.Fatalf("venue %s perp exposure actors = %d, want 1", venue.ID, len(venue.PerpExposureHedgers))
		}
	}
}

func TestPerpExposureHedgerEvidenceHasIndependentReplay(t *testing.T) {
	dir := perpExposureEvidenceRun(t)
	run, err := analysis.Open(dir)
	if err != nil {
		t.Fatalf("open evidence: %v", err)
	}
	audit, err := run.MeasurePerpExposureHedger()
	if err != nil {
		t.Fatalf("independent replay: %v", err)
	}
	if !audit.Valid || audit.Decisions == 0 || audit.StateUpdates == 0 || audit.Submitted == 0 {
		t.Fatalf("invalid P2 independent replay: %+v", audit)
	}
}

func TestPerpExposureHedgerAuditCatchesDecisionMutations(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any) bool
		caught func(*analysis.PerpExposureHedgerAudit) bool
	}{
		{
			name: "reversed target sign",
			mutate: func(payload map[string]any) bool {
				if payload["action_or_defer_reason"] != "SUBMIT_IOC" {
					return false
				}
				payload["target_perp_position"] = float64(0)
				return true
			},
			caught: func(audit *analysis.PerpExposureHedgerAudit) bool { return audit.DecisionMismatches > 0 },
		},
		{
			name: "future cached book",
			mutate: func(payload map[string]any) bool {
				if payload["action_or_defer_reason"] != "SUBMIT_IOC" {
					return false
				}
				payload["book_published_at"] = payload["decision_time"].(float64) + 1
				return true
			},
			caught: func(audit *analysis.PerpExposureHedgerAudit) bool {
				return audit.DecisionMismatches+audit.MissingReceipts > 0
			},
		},
		{
			name: "off-touch over-cap IOC",
			mutate: func(payload map[string]any) bool {
				if payload["action_or_defer_reason"] != "SUBMIT_IOC" {
					return false
				}
				payload["limit_price"] = payload["limit_price"].(float64) + 1_000_000
				payload["requested_qty"] = payload["requested_qty"].(float64) + 1
				return true
			},
			caught: func(audit *analysis.PerpExposureHedgerAudit) bool {
				return audit.DecisionMismatches+audit.GatewayMismatches+audit.OutcomeMismatches > 0
			},
		},
		{
			name: "forged cached book body identity",
			mutate: func(payload map[string]any) bool {
				if payload["action_or_defer_reason"] != "SUBMIT_IOC" {
					return false
				}
				payload["book_fingerprint"] = "00000000000000000000000000000000"
				return true
			},
			caught: func(audit *analysis.PerpExposureHedgerAudit) bool {
				return audit.ReceiptMismatches > 0
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := perpExposureEvidenceRun(t)
			if !mutateFirstP2Decision(t, dir, tc.mutate) {
				t.Fatal("fixture had no eligible P2 submission")
			}
			run, err := analysis.Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			audit, err := run.MeasurePerpExposureHedger()
			if err != nil {
				t.Fatal(err)
			}
			if audit.Valid || !tc.caught(audit) {
				t.Fatalf("mutation survived: %+v", audit)
			}
		})
	}
}

func TestPerpExposureHedgerAuditCatchesDroppedFillAttestation(t *testing.T) {
	dir := perpExposureEvidenceRun(t)
	run, err := analysis.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := run.MeasurePerpExposureHedger()
	if err != nil || !baseline.Valid || baseline.Fills == 0 {
		t.Fatalf("P2 fill fixture was not exercised: audit=%+v err=%v", baseline, err)
	}
	if !dropFirstP2EvidenceEvent(t, dir, "perp_exposure_hedger_fill") {
		t.Fatal("fixture emitted no P2 fill attestation")
	}
	mutated, err := analysis.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	audit, err := mutated.MeasurePerpExposureHedger()
	if err != nil {
		t.Fatal(err)
	}
	if audit.Valid || audit.MissingFillEvidence == 0 || audit.FillEvidenceMismatches == 0 {
		t.Fatalf("dropped P2 fill attestation survived: %+v", audit)
	}
}

func perpExposureEvidenceRun(t *testing.T) string {
	t.Helper()
	policy := perpExposureTestConfig()
	policy.DecisionInterval = 2 * time.Second
	policy.ExposureInterval = 10 * time.Second
	policy.ExposureStepQty = 10_000_000
	policy.MaxAbsExposure = 100_000_000
	policy.MaxRequestQty = 10_000_000
	policy.TickSize = 1_000_000
	policy.InitialQuoteBalance = 200_000_000 * mvQuotePrecision
	policy.InitialMargin = 100_000_000 * mvQuotePrecision
	dir := t.TempDir()
	cfg := Config{
		LogDir: dir, LogMode: "full", Seed: 101, TakerFeeBps: 5,
		StrictPopulationAccounting:        true,
		RecordMarketDataReceipts:          true,
		MarketDataReceiptRoles:            []string{"perp_exposure_hedger"},
		RecordPerpExposureHedgerDecisions: true,
		LatencyProfiles:                   map[string]LatencyProfile{"perp_exposure_hedger": {Model: "constant", Delay: 40 * time.Millisecond}},
		PerpExposureHedger:                &policy,
	}
	sim, err := NewSim(16*time.Second, cfg)
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	if err := sim.Run(context.Background()); err != nil {
		sim.Close()
		t.Fatalf("Run: %v", err)
	}
	report, err := json.Marshal(struct {
		InitialAccounts  []ParticipantAccountSnapshot `json:"initial_accounts"`
		TerminalAccounts []ParticipantAccountSnapshot `json:"terminal_accounts"`
		VenueLedgers     []VenueLedger                `json:"venue_ledgers"`
	}{sim.InitialAccounts, sim.TerminalAccounts, sim.CaptureVenueLedgers()})
	if err != nil {
		sim.Close()
		t.Fatalf("marshal report: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.LogDir, "greeks.json"), report, 0o644); err != nil {
		sim.Close()
		t.Fatalf("write report: %v", err)
	}
	sim.Close()
	if count := countRawEvent(t, cfg.LogDir, "perp_exposure_hedger_decision"); count == 0 {
		t.Fatal("P2 recorder emitted no decisions")
	}
	return dir
}

// mutateFirstP2Decision changes exactly one persisted decision after a valid
// world completed. It intentionally does not repair any evidence digest: the
// independent replay must reject the semantic contradiction itself.
func mutateFirstP2Decision(t *testing.T, dir string, mutate func(map[string]any) bool) bool {
	t.Helper()
	changed := false
	err := filepath.WalkDir(filepath.Join(dir, "venues"), func(path string, entry os.DirEntry, err error) error {
		if err != nil || changed || entry.IsDir() || filepath.Ext(path) != ".jsonl" {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
		for index, line := range lines {
			var envelope map[string]any
			if json.Unmarshal([]byte(line), &envelope) != nil || envelope["event"] != "perp_exposure_hedger_decision" {
				continue
			}
			data, ok := envelope["data"].(map[string]any)
			if !ok {
				continue
			}
			payload, ok := data["payload"].(map[string]any)
			if !ok || !mutate(payload) {
				continue
			}
			encoded, err := json.Marshal(envelope)
			if err != nil {
				return err
			}
			lines[index], changed = string(encoded), true
			break
		}
		if !changed {
			return nil
		}
		return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	})
	if err != nil {
		t.Fatalf("mutate P2 decision: %v", err)
	}
	return changed
}

func dropFirstP2EvidenceEvent(t *testing.T, dir, eventName string) bool {
	t.Helper()
	dropped := false
	err := filepath.WalkDir(filepath.Join(dir, "venues"), func(path string, entry os.DirEntry, err error) error {
		if err != nil || dropped || entry.IsDir() || filepath.Ext(path) != ".jsonl" {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
		for index, line := range lines {
			var envelope struct {
				Event string `json:"event"`
			}
			if json.Unmarshal([]byte(line), &envelope) != nil || envelope.Event != eventName {
				continue
			}
			lines = append(lines[:index], lines[index+1:]...)
			dropped = true
			break
		}
		if !dropped {
			return nil
		}
		return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	})
	if err != nil {
		t.Fatalf("drop P2 evidence: %v", err)
	}
	return dropped
}
