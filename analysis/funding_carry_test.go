package analysis

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"exchange_sim/exchange"
	"exchange_sim/simulation"
)

func TestFundingCarryAuditReplaysLocalFundingDecision(t *testing.T) {
	run := fundingCarryTestRun(t, nil)
	result, err := run.MeasureFundingCarry()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.Decisions != 1 || result.Submitted != 1 || result.Accepted != 1 || result.FundingObservationMatches != 1 || result.BookObservationMatches != 2 || result.MissingGatewayDecisions != 0 || len(result.Checks) != 0 {
		t.Fatalf("valid funding-carry audit = %+v", result)
	}
}

func TestFundingCarryAuditCatchesEvidenceAndEconomicMutations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string, *fundingCarryDecision)
		check  func(*FundingCarryAudit) bool
	}{
		{
			name: "frontier digest substitution",
			mutate: func(_ *testing.T, _ string, decision *fundingCarryDecision) {
				decision.DecisionFrontierDigest = "00000000000000000000000000000000"
			},
			check: func(result *FundingCarryAudit) bool {
				return result.ReceiptMismatches > 0
			},
		},
		{
			name: "future decision frontier",
			mutate: func(_ *testing.T, _ string, decision *fundingCarryDecision) {
				decision.DecisionFrontierDeliveredAt = decision.DecisionTime + 1
			},
			check: func(result *FundingCarryAudit) bool {
				return result.ReceiptMismatches > 0
			},
		},
		{
			name: "missing cached source identity",
			mutate: func(_ *testing.T, _ string, decision *fundingCarryDecision) {
				decision.SpotSequence = 4
			},
			check: func(result *FundingCarryAudit) bool {
				return result.MissingBookObservation > 0
			},
		},
		{
			name: "cached funding appears after decision frontier",
			mutate: func(t *testing.T, dir string, decision *fundingCarryDecision) {
				decision.DecisionFrontierOrdinal = 2
				decision.DecisionFrontierDeliveredAt = 40
				decision.DecisionFrontierDigest = fundingCarryFixtureFrontierDigest(t, dir, 2)
			},
			check: func(result *FundingCarryAudit) bool {
				return result.ReceiptMismatches > 0
			},
		},
		{
			name: "funding arithmetic substitution",
			mutate: func(_ *testing.T, _ string, decision *fundingCarryDecision) {
				decision.FundingIncomeBps--
			},
			check: func(result *FundingCarryAudit) bool {
				return result.FundingArithmeticMismatches > 0
			},
		},
		{
			name: "gateway side substitution",
			mutate: func(_ *testing.T, _ string, decision *fundingCarryDecision) {
				decision.Side = exchange.Sell.String()
			},
			check: func(result *FundingCarryAudit) bool {
				return result.GatewayDecisionMismatches > 0
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			run := fundingCarryTestRun(t, tc.mutate)
			result, err := run.MeasureFundingCarry()
			if err != nil {
				t.Fatal(err)
			}
			if result.Valid || !tc.check(result) {
				t.Fatalf("mutation survived: %+v", result)
			}
		})
	}
}

// fundingCarryTestRun builds a minimal retained-evidence world. The actor is
// intentionally absent: the analyzer must replay evidence, not call the
// policy that originally produced it. The scalar decision uses BUY (the zero
// enum member), exercising the evidence wire contract at this boundary.
func fundingCarryTestRun(t *testing.T, mutate func(*testing.T, string, *fundingCarryDecision)) *Run {
	t.Helper()
	dir := t.TempDir()
	policy := fundingCarryPolicyConfig{
		Enabled: true, SpotSymbol: "ABC/USD", PerpSymbol: "ABC-PERP",
		DecisionPeriod: 1, FundingHorizon: 1, MaxFundingAge: 1_000,
		TakerFeeBps: 1, MinNetCarryBps: 1, MaxPosition: 100, LotQty: 100,
		MinOrderSize: 1, SpotTick: 1, PerpTick: 1,
	}
	writeFundingCarryManifest(t, dir, policy)
	writeFundingCarryReceipts(t, dir)

	decision := fundingCarryDecision{
		VenueID: "north", Desk: "funding_carry_arb_1", ClientID: 7,
		PolicyVersion: "v2_5_p0_funding_carry_v2", DecisionTime: 60,
		Enabled: true, Subscribed: true, Action: "SUBMIT_SPOT_TARGET_IOC",
		SpotSymbol: "ABC/USD", PerpSymbol: "ABC-PERP",
		DesiredSpotPosition: 100, DesiredPerpPosition: -100,
		HasSpotBook: true, SpotPublishedAt: 10, SpotSequence: 1,
		HasSpotBid: true, SpotBid: 1_000, SpotBidQty: 100,
		HasSpotAsk: true, SpotAsk: 1_002, SpotAskQty: 100,
		HasPerpBook: true, PerpPublishedAt: 11, PerpSequence: 2,
		HasPerpBid: true, PerpBid: 1_010, PerpBidQty: 100,
		HasPerpAsk: true, PerpAsk: 1_012, PerpAskQty: 100,
		HasFunding: true, FundingRateBps: 10, FundingPublishedAt: 12,
		FundingSequence: 3, FundingNextAt: 1_000,
		FundingIntervalSeconds: 100, FundingMarkAvailable: true,
		FundingMarkPrice: 1_011, FundingIndexAvailable: true,
		FundingIndexPrice: 1_001, FundingAgeNanos: 48, FundingHorizon: 1,
		HoldingNanos: 940, SpotMid: 1_001, PerpMid: 1_011, PremiumBps: 99,
		FundingIncomeBps: 10, TakerFeeCostBps: 4, NetCarryBps: 6,
		MinNetCarryBps: 1, Leg: "SPOT_TARGET_ADJUSTMENT", Side: "BUY",
		LimitPrice: 1_002, RequestedQty: 100, RequestID: 42,
	}
	decision.DecisionFrontierLinkID = 1
	decision.DecisionFrontierOrdinal = 3
	decision.DecisionFrontierDeliveredAt = 50
	decision.DecisionFrontierDigest = fundingCarryFixtureFrontierDigest(t, dir, 3)
	if mutate != nil {
		mutate(t, dir, &decision)
	}
	accepted := fundingCarryVenueOrder{
		RequestID: decision.RequestID, OrderID: 100, Symbol: "ABC/USD",
		Side: exchange.Buy.String(), Type: exchange.LimitOrder.String(),
		TimeInForce: exchange.IOC.String(), Price: 1_002, Qty: 100,
	}
	acceptance := fundingCarryOutcome{
		VenueID: "north", Desk: "funding_carry_arb_1", ClientID: 7,
		DecisionTime: 60, Event: "ORDER_ACCEPTED", Leg: "SPOT_TARGET_ADJUSTMENT",
		RequestID: 42, OrderID: 100, Symbol: "ABC/USD",
	}
	lines := []string{
		logLine(60, 7, "funding_carry_decision", mustFundingCarryMap(t, decision)),
		logLine(61, 7, "OrderAccepted", mustFundingCarryMap(t, accepted)),
		logLine(60, 7, "funding_carry_leg_outcome", mustFundingCarryMap(t, acceptance)),
	}
	writeFundingCarryLog(t, dir, lines)
	run, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func writeFundingCarryManifest(t *testing.T, dir string, policy fundingCarryPolicyConfig) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"config": map[string]any{"funding_carry_arbitrageur": policy},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	report := Report{TerminalAccounts: []AccountRow{{VenueID: "north", ClientID: 7, Role: "funding_carry_arb_1"}}}
	raw, err = json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "greeks.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeFundingCarryReceipts(t *testing.T, dir string) {
	t.Helper()
	recorder, err := simulation.NewMarketDataReceiptRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	link := "north/funding_carry_arb/client/7"
	recorder.RegisterLink("north", link, "funding_carry_arb")
	schedules := []simulation.MarketDataSchedule{
		{ClientID: 7, SourceVenue: "north", Link: link, Symbol: "ABC/USD", Type: exchange.MDSnapshot, Sequence: 1, Fingerprint: [16]byte{1}, PublishedAt: 10, ScheduledAt: 20, LinkOrdinal: 1},
		{ClientID: 7, SourceVenue: "north", Link: link, Symbol: "ABC-PERP", Type: exchange.MDSnapshot, Sequence: 2, Fingerprint: [16]byte{2}, PublishedAt: 11, ScheduledAt: 21, LinkOrdinal: 2},
		{ClientID: 7, SourceVenue: "north", Link: link, Symbol: "ABC-PERP", Type: exchange.MDFunding, Sequence: 3, Fingerprint: [16]byte{3}, PublishedAt: 12, ScheduledAt: 22, LinkOrdinal: 3},
	}
	delivered := []int64{30, 40, 50}
	var frontier simulation.MarketDataFrontier
	for index, schedule := range schedules {
		recorder.RecordSchedule(schedule)
		frontier = recorder.RecordReceipt(simulation.MarketDataReceipt{MarketDataSchedule: schedule, DeliveredAt: delivered[index]})
	}
	// Buy is encoded as zero in the fixed-width decision record. Its presence
	// must remain distinct from an absent side.
	recorder.RecordDecision(simulation.MarketDataDecision{
		ClientID: 7, SourceVenue: "north", Link: link, Symbol: "ABC/USD", RequestID: 42,
		Side: exchange.Buy, OrderType: exchange.LimitOrder, TimeInForce: exchange.IOC,
		Price: 1_002, Qty: 100, DecisionAt: 60, Frontier: frontier,
	})
	if err := recorder.Finalize(100); err != nil {
		t.Fatal(err)
	}
}

func fundingCarryFixtureFrontierDigest(t *testing.T, dir string, ordinal uint64) string {
	t.Helper()
	_, raw, err := fundingCarryFixtureReceiptRaw(dir)
	if err != nil {
		t.Fatal(err)
	}
	frontiers := reconstructReceiptHistory(raw)
	frontier, found := frontiers[vectorFrontierKey{clientID: 7, linkID: 1, ordinal: ordinal}]
	if !found {
		t.Fatal("funding receipt frontier missing")
	}
	return fmt.Sprintf("%x", frontier.digest)
}

func fundingCarryFixtureReceiptRaw(dir string) (marketDataEvidenceManifest, []byte, error) {
	manifestRaw, err := os.ReadFile(filepath.Join(dir, "market-data-evidence-v2.json"))
	if err != nil {
		return marketDataEvidenceManifest{}, nil, err
	}
	var manifest marketDataEvidenceManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return marketDataEvidenceManifest{}, nil, err
	}
	raw, _, err := readEvidenceFile(dir, manifest.Receipts.File, marketDataReceiptRecordBytes, manifest.Receipts.Records, manifest.Receipts.Digest)
	return manifest, raw, err
}

func writeFundingCarryLog(t *testing.T, dir string, lines []string) {
	t.Helper()
	path := filepath.Join(dir, "venues", "north", "spot", "ABC-USD.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := ""
	for _, line := range lines {
		content += line + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustFundingCarryMap(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
