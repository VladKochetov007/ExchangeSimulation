package analysis

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"exchange_sim/exchange"
	"exchange_sim/simulation"
)

func TestMeasureCDFLiquidityReconstructsBoundedSupplier(t *testing.T) {
	run := writeCDFLiquidityFixture(t, true, false)
	audit, err := run.MeasureCDFLiquidity()
	if err != nil {
		t.Fatalf("MeasureCDFLiquidity: %v", err)
	}
	if !audit.Valid {
		t.Fatalf("audit invalid: %+v", audit.Checks)
	}
	if audit.SupplierCount != 1 || audit.DecisionCount != 3 || audit.FillCount != 1 || audit.AcceptedQuoteCount != 2 || audit.CompletedQuoteCount != 2 {
		t.Fatalf("audit counts = %+v", audit)
	}
	if audit.SupplierVolumeQty != 5 || audit.TotalTradeVolumeQty != 25 || audit.SupplierVolumeShare != .2 {
		t.Fatalf("audit volume = %+v", audit)
	}
	if audit.Suppliers[0].PnL != 5 || audit.Suppliers[0].TerminalPosition != 5 || audit.Suppliers[0].MaxObservedTouchShare != .5 || audit.Suppliers[0].SupplierVolumeShare != .2 || audit.Suppliers[0].TimeWeightedRestingDepthShare != .2 {
		t.Fatalf("supplier diagnostics = %+v", audit.Suppliers[0])
	}
	if audit.BalanceSnapshotCount != 2 || audit.BalanceReconciliationResidual != 0 || audit.PnLReconciliationResidual != 0 || audit.TradingPnL != 5 || audit.TradingPnLReconciliationResidual != 0 {
		t.Fatalf("aggregate conservation diagnostics = %+v", audit)
	}
	if audit.ExpectedHistoricalCount != 1 || audit.Venues[0].ExpectedHistoricalCount != 1 {
		t.Fatalf("historical counts = aggregate %d, venue %d", audit.ExpectedHistoricalCount, audit.Venues[0].ExpectedHistoricalCount)
	}
}

func TestCDFDepthShareUsesNonEmptyIntervalsAndIncludesTerminalInterval(t *testing.T) {
	state := &CDFLiquiditySupplierAudit{}
	run := &CDFLiquidityRunAudit{
		lastDepthSnapshotAt:       map[string]int64{"north": 9},
		lastDepthTotal:            map[string]int64{"north": 20},
		lastSupplierDepthByClient: map[string]map[uint64]int64{"north": {7: 10}},
		terminalAt:                12,
	}
	states := map[cdfParticipantKey]*CDFLiquiditySupplierAudit{{VenueID: "north", ClientID: 7}: state}
	run.lastDepthTotal["north"] = 20
	run.lastSupplierDepthByClient["north"] = map[uint64]int64{7: 5}
	run.accumulateDepthInterval("north", 1, 3, states)
	run.lastDepthTotal["north"] = 0
	run.lastSupplierDepthByClient["north"] = map[uint64]int64{}
	run.accumulateDepthInterval("north", 3, 7, states)
	run.lastDepthTotal["north"] = 20
	run.lastSupplierDepthByClient["north"] = map[uint64]int64{7: 10}
	run.accumulateDepthInterval("north", 7, 9, states)
	run.accumulateTerminalDepth(states)
	if state.restingDepthWeightedNumerator != 60 || state.restingDepthWeightedDenominator != 140 {
		t.Fatalf("depth integrals = (%v, %v), want (60, 140) with empty interval excluded", state.restingDepthWeightedNumerator, state.restingDepthWeightedDenominator)
	}
	if got := state.restingDepthWeightedNumerator / state.restingDepthWeightedDenominator; got != 3.0/7.0 {
		t.Fatalf("depth share = %v, want %v", got, 3.0/7.0)
	}
}

func TestMeasureCDFLiquiditySeparatesCancelPendingFromLiveQuotes(t *testing.T) {
	run := writeCDFLiquidityFixture(t, true, false)
	bookPath := filepath.Join(run.Dir, "venues", "north", "spot", "CDF-USD.jsonl")
	raw, err := os.ReadFile(bookPath)
	if err != nil {
		t.Fatal(err)
	}
	withoutCancellation := strings.Replace(string(raw), cdfFixtureLine(6, 2, "OrderCancelled", `{"order_id":8,"request_id":2,"remaining_qty":4}`)+"\n", "", 1)
	if withoutCancellation == string(raw) {
		t.Fatal("fixture cancellation was not found")
	}
	if err := os.WriteFile(bookPath, []byte(withoutCancellation), 0644); err != nil {
		t.Fatal(err)
	}
	audit, err := run.MeasureCDFLiquidity()
	if err != nil {
		t.Fatal(err)
	}
	if !audit.Valid || audit.CensoredQuoteCount != 1 || audit.CancelPendingQuoteCount != 1 || audit.LiveAcceptedQuoteCount != 0 || audit.PendingSubmissionCount != 0 {
		t.Fatalf("terminal quote categories = %+v, want one cancel-pending accepted quote", audit)
	}
	if audit.Suppliers[0].CancelPendingQuoteCount != 1 || audit.Suppliers[0].LiveAcceptedQuoteCount != 0 {
		t.Fatalf("supplier terminal quote categories = %+v", audit.Suppliers[0])
	}
}

func TestMeasureCDFLiquidityFailsClosedOnMissingFillField(t *testing.T) {
	run := writeCDFLiquidityFixture(t, true, true)
	audit, err := run.MeasureCDFLiquidity()
	if err != nil {
		t.Fatalf("MeasureCDFLiquidity: %v", err)
	}
	if audit.Valid || !hasCDFCheck(audit.Checks, "malformed supplier fill") {
		t.Fatalf("audit = %+v, want malformed fill rejection", audit)
	}
}

func TestCompareCDFLiquidityRunsRequiresSeparateRoster(t *testing.T) {
	treatment := writeCDFLiquidityFixture(t, true, false)
	control := writeCDFLiquidityFixture(t, false, false)
	comparison, err := CompareCDFLiquidityRuns(treatment, control)
	if err != nil {
		t.Fatalf("CompareCDFLiquidityRuns: %v", err)
	}
	if !comparison.Valid || comparison.Control.SupplierCount != 0 || comparison.Treatment.SupplierCount != 1 {
		t.Fatalf("comparison = %+v", comparison)
	}
}

func TestCompareCDFLiquidityRunsRejectsInactiveTreatment(t *testing.T) {
	treatment := writeCDFLiquidityFixture(t, true, false)
	generalPath := filepath.Join(treatment.Dir, "venues", "north", "general.jsonl")
	if err := os.WriteFile(generalPath, []byte(cdfFixtureLine(1, 2, "elastic_liquidity_supplier_decision", `{"role":"cdf_elastic_supplier_1","client_id":2,"symbol":"CDF/USD","decision_time":1,"observation_time":1,"observation_age":0,"observation_sequence":1,"best_bid":99,"best_bid_qty":10,"best_ask":101,"best_ask_qty":10,"mark_price":100,"reference_price":100,"position":0,"target_position":5,"inventory_limit":10,"action":"submit","reason":"inventory_target_gap","side":"BUY","quote_price":99,"quote_qty":5,"quote_request_id":1,"quote_submitted_at":1}`)+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	bookPath := filepath.Join(treatment.Dir, "venues", "north", "spot", "CDF-USD.jsonl")
	book := cdfFixtureLine(1, 0, "BookSnapshot", `{"bids":[{"price":99,"visible_qty":10,"hidden_qty":0}],"asks":[{"price":101,"visible_qty":10,"hidden_qty":0}]}`) + "\n"
	book += cdfFixtureLine(2, 0, "Trade", `{"trade_id":1,"price":99,"qty":5,"side":"SELL","taker_order_id":9}`) + "\n"
	if err := os.WriteFile(bookPath, []byte(book), 0644); err != nil {
		t.Fatal(err)
	}
	audit, err := treatment.MeasureCDFLiquidity()
	if err != nil {
		t.Fatal(err)
	}
	if audit.Valid || !hasCDFCheck(audit.Checks, "supplier activation contract is incomplete") {
		t.Fatalf("inactive treatment audit = %+v, want fail-closed activation rejection", audit)
	}
}

func TestMeasureCDFLiquidityRejectsConfiguredQuoteLimitMutation(t *testing.T) {
	run := writeCDFLiquidityFixture(t, true, false)
	manifestPath := filepath.Join(run.Dir, "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	raw = []byte(strings.Replace(string(raw), `"max_quote_qty":5`, `"max_quote_qty":4`, 1))
	if err := os.WriteFile(manifestPath, raw, 0644); err != nil {
		t.Fatal(err)
	}
	audit, err := run.MeasureCDFLiquidity()
	if err != nil {
		t.Fatal(err)
	}
	if audit.Valid || !hasCDFCheck(audit.Checks, "submitted quote exceeds registered maximum quantity") {
		t.Fatalf("quote-limit mutation audit = %+v, want fail-closed limit rejection", audit)
	}
}

func TestMeasureCDFLiquidityRejectsObservationFingerprintMutation(t *testing.T) {
	run := writeCDFLiquidityReceiptFixture(t)
	generalPath := filepath.Join(run.Dir, "venues", "north", "general.jsonl")
	raw, err := os.ReadFile(generalPath)
	if err != nil {
		t.Fatal(err)
	}
	mutated := strings.Replace(string(raw), `"observation_fingerprint":"01000000000000000000000000000000"`, `"observation_fingerprint":"02000000000000000000000000000000"`, 1)
	if mutated == string(raw) {
		t.Fatal("fixture fingerprint was not found")
	}
	if err := os.WriteFile(generalPath, []byte(mutated), 0644); err != nil {
		t.Fatal(err)
	}
	audit, err := run.MeasureCDFLiquidity()
	if err != nil {
		t.Fatal(err)
	}
	if audit.Valid || !hasCDFCheck(audit.Checks, "supplier decision frontier does not match its delayed local observation") {
		t.Fatalf("fingerprint mutation audit = %+v, want fail-closed frontier rejection", audit)
	}
}

func TestMeasureCDFLiquidityRejectsObservationOrdinalMutation(t *testing.T) {
	run := writeCDFLiquidityReceiptFixture(t)
	generalPath := filepath.Join(run.Dir, "venues", "north", "general.jsonl")
	raw, err := os.ReadFile(generalPath)
	if err != nil {
		t.Fatal(err)
	}
	mutated := strings.Replace(string(raw), `"observation_ordinal":1`, `"observation_ordinal":2`, 1)
	if mutated == string(raw) {
		t.Fatal("fixture observation ordinal was not found")
	}
	if err := os.WriteFile(generalPath, []byte(mutated), 0644); err != nil {
		t.Fatal(err)
	}
	audit, err := run.MeasureCDFLiquidity()
	if err != nil {
		t.Fatal(err)
	}
	if audit.Valid || !hasCDFCheck(audit.Checks, "supplier decision frontier does not match its delayed local observation") {
		t.Fatalf("ordinal mutation audit = %+v, want fail-closed frontier rejection", audit)
	}
}

func TestMeasureCDFLiquidityRejectsObservationDigestMutation(t *testing.T) {
	run := writeCDFLiquidityReceiptFixture(t)
	generalPath := filepath.Join(run.Dir, "venues", "north", "general.jsonl")
	raw, err := os.ReadFile(generalPath)
	if err != nil {
		t.Fatal(err)
	}
	rawString := string(raw)
	digestPrefix := `"observation_digest":"`
	digestStart := strings.Index(rawString, digestPrefix)
	if digestStart < 0 {
		t.Fatal("fixture observation digest was not found")
	}
	digestStart += len(digestPrefix)
	digestEnd := strings.IndexByte(rawString[digestStart:], '"')
	if digestEnd < 0 {
		t.Fatal("fixture observation digest is unterminated")
	}
	digestEnd += digestStart
	mutated := rawString[:digestStart] + strings.Repeat("02", 16) + rawString[digestEnd:]
	if err := os.WriteFile(generalPath, []byte(mutated), 0644); err != nil {
		t.Fatal(err)
	}
	audit, err := run.MeasureCDFLiquidity()
	if err != nil {
		t.Fatal(err)
	}
	if audit.Valid || !hasCDFCheck(audit.Checks, "supplier decision frontier does not match its delayed local observation") {
		t.Fatalf("digest mutation audit = %+v, want fail-closed frontier rejection", audit)
	}
}

func TestCDFReceiptEvidenceRejectsOlderValidFrontier(t *testing.T) {
	dir := writeCDFReceiptContractFixture(t, true)
	if _, err := readCDFMarketDataEvidence(dir); err == nil || !strings.Contains(err.Error(), "decision frontier") {
		t.Fatalf("older valid frontier was accepted: %v", err)
	}
}

func TestCDFReceiptEvidenceRejectsManifestEnvelopeMutation(t *testing.T) {
	dir := writeCDFReceiptContractFixture(t, false)
	manifestPath := filepath.Join(dir, "market-data-evidence-v2.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	mutated := strings.Replace(string(raw), `"schema_version": 2`, `"schema_version": 3`, 1)
	if mutated == string(raw) {
		t.Fatal("manifest schema version was not found")
	}
	if err := os.WriteFile(manifestPath, []byte(mutated), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readCDFMarketDataEvidence(dir); err == nil || !strings.Contains(err.Error(), "manifest violates schema contract") {
		t.Fatalf("manifest envelope mutation was accepted: %v", err)
	}
}

func TestCDFReceiptEvidenceRequiresReceiptForDueSchedule(t *testing.T) {
	dir := t.TempDir()
	recorder, err := simulation.NewMarketDataReceiptRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	link := "north/cdf_elastic_supplier"
	if recorder.RegisterLink("north", link, "cdf_elastic_supplier") == 0 {
		t.Fatal("receipt fixture link registration failed")
	}
	schedule := simulation.MarketDataSchedule{
		ClientID: 2, SourceVenue: "north", Link: link, Symbol: "CDF/USD", Type: exchange.MDSnapshot,
		Sequence: 1, Fingerprint: [16]byte{1}, PublishedAt: 1, ScheduledAt: 1, LinkOrdinal: 1,
	}
	if recorder.RecordSchedule(schedule) == 0 {
		t.Fatal("receipt fixture schedule registration failed")
	}
	if err := recorder.Finalize(1); err != nil {
		t.Fatal(err)
	}
	if _, err := readCDFMarketDataEvidence(dir); err == nil || !strings.Contains(err.Error(), "due by terminal time without a receipt") {
		t.Fatalf("due schedule without receipt was accepted: %v", err)
	}
}

func TestCDFReceiptEvidenceAllowsPostTerminalSchedule(t *testing.T) {
	dir := t.TempDir()
	recorder, err := simulation.NewMarketDataReceiptRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	link := "north/cdf_elastic_supplier"
	if recorder.RegisterLink("north", link, "cdf_elastic_supplier") == 0 {
		t.Fatal("receipt fixture link registration failed")
	}
	schedule := simulation.MarketDataSchedule{
		ClientID: 2, SourceVenue: "north", Link: link, Symbol: "CDF/USD", Type: exchange.MDSnapshot,
		Sequence: 1, Fingerprint: [16]byte{1}, PublishedAt: 2, ScheduledAt: 2, LinkOrdinal: 1,
	}
	if recorder.RecordSchedule(schedule) == 0 {
		t.Fatal("receipt fixture schedule registration failed")
	}
	if err := recorder.Finalize(1); err != nil {
		t.Fatal(err)
	}
	if _, err := readCDFMarketDataEvidence(dir); err != nil {
		t.Fatalf("post-terminal schedule was rejected: %v", err)
	}
}

func TestCDFReceiptEvidenceRejectsGlobalEventOrdinalGap(t *testing.T) {
	run := writeCDFLiquidityReceiptFixture(t)
	decisionsPath := filepath.Join(run.Dir, "market-data-decisions-v2.bin")
	decisions, err := os.ReadFile(decisionsPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) < simulation.MarketDataDecisionRecordBytes {
		t.Fatal("decision fixture is empty")
	}
	// The shared schedule/receipt/decision ordinal is deliberately distinct
	// from the action ledger ordinal. Moving one decision past the stream end
	// leaves valid record bytes and a valid file digest but creates a global gap.
	const eventOrdinalOffset = simulation.MarketDataDecisionRecordBytes + 88
	decisions[eventOrdinalOffset+7] = 5
	if err := os.WriteFile(decisionsPath, decisions, 0644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(run.Dir, "market-data-evidence-v2.json")
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatal(err)
	}
	decisionArtifact := manifest["decisions"].(map[string]any)
	digest := sha256.Sum256(decisions)
	decisionArtifact["digest"] = hex.EncodeToString(digest[:])
	updatedManifest, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, append(updatedManifest, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readCDFMarketDataEvidence(run.Dir); err == nil || !strings.Contains(err.Error(), "global event ordinal stream has a gap") {
		t.Fatalf("global event ordinal gap was accepted: %v", err)
	}
}

func TestMeasureCDFLiquidityRejectsSyntheticStaleWithdrawalWithoutLiveOrder(t *testing.T) {
	run := writeCDFLiquidityFixture(t, true, false)
	generalPath := filepath.Join(run.Dir, "venues", "north", "general.jsonl")
	raw, err := os.ReadFile(generalPath)
	if err != nil {
		t.Fatal(err)
	}
	staleWithdrawal := cdfFixtureLine(12, 2, "elastic_liquidity_supplier_decision", `{"role":"cdf_elastic_supplier_1","client_id":2,"symbol":"CDF/USD","decision_time":12,"observation_time":1,"observation_age":11,"observation_sequence":1,"best_bid":99,"best_bid_qty":10,"best_ask":101,"best_ask_qty":10,"mark_price":100,"reference_price":100,"position":5,"target_position":5,"inventory_limit":10,"action":"withdraw","reason":"stale_or_missing_observation","quote_order_id":8,"cancel_request_id":4}`) + "\n"
	if err := os.WriteFile(generalPath, append(raw, []byte(staleWithdrawal)...), 0644); err != nil {
		t.Fatal(err)
	}
	audit, err := run.MeasureCDFLiquidity()
	if err != nil {
		t.Fatal(err)
	}
	if audit.Valid || !hasCDFCheck(audit.Checks, "stale withdrawal references a supplier order already closed before the decision") || !hasCDFCheck(audit.Checks, "stale withdrawal has no later matching exchange cancellation outcome") {
		t.Fatalf("synthetic stale withdrawal audit = %+v, want live-order rejection", audit)
	}
}

func TestMeasureCDFLiquidityAcceptsStaleWithdrawalFillCancelRace(t *testing.T) {
	run := writeCDFLiquidityFullFillCancelRaceFixture(t)
	audit, err := run.MeasureCDFLiquidity()
	if err != nil {
		t.Fatalf("MeasureCDFLiquidity: %v", err)
	}
	if !audit.Valid || audit.FillCount != 2 || audit.CompletedQuoteCount != 2 {
		t.Fatalf("fill-wins-cancel-race audit = %+v", audit)
	}
	if hasCDFCheck(audit.Checks, "stale withdrawal has no later matching exchange cancellation outcome") {
		t.Fatalf("legitimate fill-wins-cancel-race was rejected: %+v", audit.Checks)
	}
}

func TestMeasureCDFLiquidityRejectsUnattestedWaitState(t *testing.T) {
	run := writeCDFLiquidityFixture(t, true, false)
	generalPath := filepath.Join(run.Dir, "venues", "north", "general.jsonl")
	raw, err := os.ReadFile(generalPath)
	if err != nil {
		t.Fatal(err)
	}
	unattestedPending := cdfFixtureLine(7, 2, "elastic_liquidity_supplier_decision", `{"role":"cdf_elastic_supplier_1","client_id":2,"symbol":"CDF/USD","decision_time":7,"observation_time":1,"observation_age":6,"observation_sequence":1,"best_bid":99,"best_bid_qty":10,"best_ask":101,"best_ask_qty":10,"mark_price":100,"reference_price":100,"position":5,"target_position":9,"inventory_limit":10,"action":"wait","reason":"order_pending","quote_request_id":99}`) + "\n"
	acceptedBeforePending := cdfFixtureLine(4, 2, "elastic_liquidity_supplier_decision", `{"role":"cdf_elastic_supplier_1","client_id":2,"symbol":"CDF/USD","decision_time":4,"observation_time":1,"observation_age":3,"observation_sequence":1,"best_bid":99,"best_bid_qty":10,"best_ask":101,"best_ask_qty":10,"mark_price":100,"reference_price":100,"position":5,"target_position":9,"inventory_limit":10,"action":"wait","reason":"order_pending","quote_request_id":1}`) + "\n"
	unattestedCancel := cdfFixtureLine(8, 2, "elastic_liquidity_supplier_decision", `{"role":"cdf_elastic_supplier_1","client_id":2,"symbol":"CDF/USD","decision_time":8,"observation_time":1,"observation_age":7,"observation_sequence":1,"best_bid":99,"best_bid_qty":10,"best_ask":101,"best_ask_qty":10,"mark_price":100,"reference_price":100,"position":5,"target_position":9,"inventory_limit":10,"action":"wait","reason":"cancel_pending","quote_order_id":8,"cancel_request_id":99}`) + "\n"
	if err := os.WriteFile(generalPath, append(raw, []byte(unattestedPending+acceptedBeforePending+unattestedCancel)...), 0644); err != nil {
		t.Fatal(err)
	}
	audit, err := run.MeasureCDFLiquidity()
	if err != nil {
		t.Fatalf("MeasureCDFLiquidity: %v", err)
	}
	if audit.Valid || !hasCDFCheck(audit.Checks, "order-pending wait has no outstanding submission") || !hasCDFCheck(audit.Checks, "order-pending wait follows an already accepted order") || !hasCDFCheck(audit.Checks, "cancel-pending wait has no matching live cancellation") {
		t.Fatalf("unattested wait audit = %+v", audit)
	}
}

func TestMeasureCDFLiquidityRejectsReorderedFillCancelRace(t *testing.T) {
	run := writeCDFLiquidityFullFillCancelRaceFixture(t)
	bookPath := filepath.Join(run.Dir, "venues", "north", "spot", "CDF-USD.jsonl")
	raw, err := os.ReadFile(bookPath)
	if err != nil {
		t.Fatal(err)
	}
	fill := cdfFixtureLine(13, 2, "OrderFill", `{"order_id":8,"trade_id":2,"side":"BUY","price":99,"qty":4,"filled_qty":4,"remaining_qty":0,"is_full":true}`) + "\n"
	rejection := cdfFixtureLine(14, 2, "OrderCancelRejected", `{"request_id":4,"success":false,"error":"ORDER_ALREADY_FILLED"}`) + "\n"
	reordered := cdfFixtureLine(13, 2, "OrderCancelRejected", `{"request_id":4,"success":false,"error":"ORDER_ALREADY_FILLED"}`) + "\n" + fill
	if !strings.Contains(string(raw), fill+rejection) {
		t.Fatal("fill/cancel-rejection sequence was not found")
	}
	if err := os.WriteFile(bookPath, []byte(strings.Replace(string(raw), fill+rejection, reordered, 1)), 0644); err != nil {
		t.Fatal(err)
	}
	audit, err := run.MeasureCDFLiquidity()
	if err != nil {
		t.Fatalf("MeasureCDFLiquidity: %v", err)
	}
	if audit.Valid || !hasCDFCheck(audit.Checks, "stale withdrawal has no later matching exchange cancellation outcome") {
		t.Fatalf("reordered fill/cancel race audit = %+v", audit)
	}
}

func TestUpdateSupplierPnLHandlesPartialClosureReversalAndFees(t *testing.T) {
	state := &CDFLiquiditySupplierAudit{configuredBasePrecision: 1, configuredQuoteAsset: "USD"}
	fills := []cdfFillEvidence{
		{Side: "BUY", Price: 100, Qty: 2, PositionBefore: 0, PositionAfter: 2},
		{Side: "BUY", Price: 101, Qty: 1, PositionBefore: 2, PositionAfter: 3},
		{Side: "SELL", Price: 102, Qty: 2, FeeAmount: 3, FeeAsset: "USD", PositionBefore: 3, PositionAfter: 1},
		{Side: "SELL", Price: 90, Qty: 3, FeeAmount: 2, FeeAsset: "USD", PositionBefore: 1, PositionAfter: -2},
		{Side: "BUY", Price: 80, Qty: 1, PositionBefore: -2, PositionAfter: -1},
		{Side: "BUY", Price: 80, Qty: 1, PositionBefore: -1, PositionAfter: 0},
	}
	for index, fill := range fills {
		if err := updateSupplierPnL(state, fill); err != nil {
			t.Fatalf("fill %d: %v", index, err)
		}
	}
	if state.realizedPnL != 9 || state.entryPrice != 0 {
		t.Fatalf("fixed-point PnL = %d, entry price = %d; want 9 and 0", state.realizedPnL, state.entryPrice)
	}
}

func TestMeasureCDFLiquidityRejectsActionIdentityMutation(t *testing.T) {
	run := writeCDFLiquidityReceiptFixture(t)
	actionsPath := filepath.Join(run.Dir, "market-data-actions-v2.bin")
	actions, err := os.ReadFile(actionsPath)
	if err != nil {
		t.Fatal(err)
	}
	const cancelActionOffset = 2 * simulation.MarketDataActionRecordBytes
	if len(actions) < cancelActionOffset+44 {
		t.Fatalf("action fixture has %d bytes, want cancellation record", len(actions))
	}
	actions[cancelActionOffset+43] ^= 1
	if err := os.WriteFile(actionsPath, actions, 0644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(run.Dir, "market-data-evidence-v2.json")
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatal(err)
	}
	actionsArtifact, ok := manifest["actions"].(map[string]any)
	if !ok {
		t.Fatal("action artifact missing from manifest")
	}
	digest := sha256.Sum256(actions)
	actionsArtifact["digest"] = hex.EncodeToString(digest[:])
	updatedManifest, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, append(updatedManifest, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	audit, err := run.MeasureCDFLiquidity()
	if err != nil {
		t.Fatal(err)
	}
	if audit.Valid || !hasCDFCheck(audit.Checks, "supplier action is not reconciled to a market-data gateway boundary") {
		t.Fatalf("action identity mutation audit = %+v, want fail-closed gateway mismatch", audit)
	}
}

func writeCDFReceiptContractFixture(t *testing.T, olderFrontier bool) string {
	t.Helper()
	dir := t.TempDir()
	recorder, err := simulation.NewMarketDataReceiptRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	link := "north/cdf_elastic_supplier"
	linkID := recorder.RegisterLink("north", link, "cdf_elastic_supplier")
	if linkID == 0 {
		t.Fatal("receipt contract fixture link registration failed")
	}
	var firstFrontier simulation.MarketDataFrontier
	for ordinal := uint64(1); ordinal <= 2; ordinal++ {
		schedule := simulation.MarketDataSchedule{
			ClientID: 2, SourceVenue: "north", Link: link, Symbol: "CDF/USD", Type: exchange.MDSnapshot,
			Sequence: ordinal, Fingerprint: [16]byte{byte(ordinal)}, PublishedAt: int64(ordinal), ScheduledAt: int64(ordinal), LinkOrdinal: ordinal,
		}
		if recorder.RecordSchedule(schedule) == 0 {
			t.Fatal("receipt contract fixture schedule registration failed")
		}
		frontier := recorder.RecordReceipt(simulation.MarketDataReceipt{MarketDataSchedule: schedule, DeliveredAt: int64(ordinal)})
		if ordinal == 1 {
			firstFrontier = frontier
		}
		if ordinal == 2 && olderFrontier {
			recorder.RecordDecision(simulation.MarketDataDecision{
				ClientID: 2, SourceVenue: "north", Link: link, Symbol: "CDF/USD", RequestID: 31,
				Side: exchange.Buy, OrderType: exchange.LimitOrder, TimeInForce: exchange.GTC, Price: 99, Qty: 3, DecisionAt: 3,
				Frontier: firstFrontier,
			})
		}
	}
	if err := recorder.Finalize(3); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestMeasureCDFLiquidityRejectsGrossInventoryLimitMutation(t *testing.T) {
	run := writeCDFLiquidityFixture(t, true, false)
	manifestPath := filepath.Join(run.Dir, "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	mutated := strings.Replace(string(raw), `"max_inventory":110`, `"max_inventory":104`, 1)
	if mutated == string(raw) {
		t.Fatal("fixture gross inventory limit was not found")
	}
	if err := os.WriteFile(manifestPath, []byte(mutated), 0644); err != nil {
		t.Fatal(err)
	}
	audit, err := run.MeasureCDFLiquidity()
	if err != nil {
		t.Fatal(err)
	}
	if audit.Valid || !hasCDFCheck(audit.Checks, "decision gross inventory contract disagrees with registered configuration") {
		t.Fatalf("gross inventory mutation audit = %+v, want fail-closed cap rejection", audit)
	}
}

func TestMeasureCDFLiquidityConservesNonzeroQuoteFee(t *testing.T) {
	run := writeCDFLiquidityFixture(t, true, false)
	generalPath := filepath.Join(run.Dir, "venues", "north", "general.jsonl")
	raw, err := os.ReadFile(generalPath)
	if err != nil {
		t.Fatal(err)
	}
	mutated := strings.Replace(string(raw), `"fee_amount":0`, `"fee_amount":1`, 1)
	mutated = strings.Replace(mutated, `"fee_asset":""`, `"fee_asset":"USD"`, 1)
	mutated = strings.Replace(mutated, `"quote_cash_required":495`, `"quote_cash_required":496`, 1)
	mutated = strings.Replace(mutated, `"quote_cash_available":505`, `"quote_cash_available":504`, 1)
	mutated = strings.Replace(mutated, `"net_asset":505`, `"net_asset":504`, 1)
	mutated = strings.Replace(mutated, `"free":505`, `"free":504`, 1)
	if mutated == string(raw) {
		t.Fatal("fixture fee or terminal balance was not found")
	}
	if err := os.WriteFile(generalPath, []byte(mutated), 0644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(run.Dir, "manifest.json")
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifestMutated := strings.Replace(string(manifestRaw), `"max_observation_age":10`, `"max_observation_age":10,"maker_fee_bps":25`, 1)
	if manifestMutated == string(manifestRaw) {
		t.Fatal("fixture maker-fee configuration was not found")
	}
	if err := os.WriteFile(manifestPath, []byte(manifestMutated), 0644); err != nil {
		t.Fatal(err)
	}
	greeksPath := filepath.Join(run.Dir, "greeks.json")
	greeks, err := os.ReadFile(greeksPath)
	if err != nil {
		t.Fatal(err)
	}
	greeksMutated := strings.Replace(string(greeks), `"equity":11005`, `"equity":11004`, 1)
	greeksMutated = strings.Replace(greeksMutated, `"net_asset":505`, `"net_asset":504`, 1)
	if greeksMutated == string(greeks) {
		t.Fatal("fixture terminal account was not found")
	}
	if err := os.WriteFile(greeksPath, []byte(greeksMutated), 0644); err != nil {
		t.Fatal(err)
	}
	run, err = Open(run.Dir)
	if err != nil {
		t.Fatal(err)
	}
	audit, err := run.MeasureCDFLiquidity()
	if err != nil {
		t.Fatal(err)
	}
	if !audit.Valid || audit.TradingPnL != 4 || audit.RealizedPnL != -1 || audit.UnrealizedPnL != 5 || audit.TradingPnLReconciliationResidual != 0 {
		t.Fatalf("nonzero fee conservation audit = %+v, want fee-aware decomposition", audit)
	}
}

func TestMeasureCDFLiquidityRejectsMalformedBalanceSnapshot(t *testing.T) {
	run := writeCDFLiquidityFixture(t, true, false)
	generalPath := filepath.Join(run.Dir, "venues", "north", "general.jsonl")
	raw, err := os.ReadFile(generalPath)
	if err != nil {
		t.Fatal(err)
	}
	mutated := strings.Replace(string(raw), `"net_asset":100`, `"net_asset":99`, 1)
	if mutated == string(raw) {
		t.Fatal("fixture balance was not found")
	}
	if err := os.WriteFile(generalPath, []byte(mutated), 0644); err != nil {
		t.Fatal(err)
	}
	audit, err := run.MeasureCDFLiquidity()
	if err != nil {
		t.Fatal(err)
	}
	if audit.Valid || !hasCDFCheck(audit.Checks, "supplier spot balance snapshot arithmetic is invalid") {
		t.Fatalf("balance mutation audit = %+v, want fail-closed arithmetic rejection", audit)
	}
}

func TestMeasureCDFLiquidityRejectsMissingBalanceSnapshots(t *testing.T) {
	run := writeCDFLiquidityFixture(t, true, false)
	generalPath := filepath.Join(run.Dir, "venues", "north", "general.jsonl")
	raw, err := os.ReadFile(generalPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	retained := lines[:0]
	for _, line := range lines {
		if strings.Contains(line, `"event":"balance_snapshot"`) {
			continue
		}
		retained = append(retained, line)
	}
	if err := os.WriteFile(generalPath, []byte(strings.Join(retained, "\n")), 0644); err != nil {
		t.Fatal(err)
	}
	audit, err := run.MeasureCDFLiquidity()
	if err != nil {
		t.Fatal(err)
	}
	if audit.Valid || !hasCDFCheck(audit.Checks, "supplier has no balance snapshot evidence") {
		t.Fatalf("missing balance snapshot audit = %+v, want fail-closed completeness rejection", audit)
	}
}

func TestMeasureCDFLiquidityCountsTerminalPendingSubmission(t *testing.T) {
	run := writeCDFLiquidityFixture(t, true, false)
	generalPath := filepath.Join(run.Dir, "venues", "north", "general.jsonl")
	raw, err := os.ReadFile(generalPath)
	if err != nil {
		t.Fatal(err)
	}
	pending := cdfFixtureLine(7, 2, "elastic_liquidity_supplier_decision", `{"role":"cdf_elastic_supplier_1","client_id":2,"symbol":"CDF/USD","decision_time":7,"observation_time":1,"observation_age":6,"observation_sequence":1,"best_bid":99,"best_bid_qty":10,"best_ask":101,"best_ask_qty":10,"mark_price":100,"reference_price":100,"position":5,"target_position":9,"inventory_limit":10,"action":"submit","reason":"inventory_target_gap","side":"BUY","quote_price":99,"quote_qty":3,"quote_request_id":3,"quote_submitted_at":7,"quote_cash_available":505,"quote_cash_required":297}`) + "\n"
	if err := os.WriteFile(generalPath, append(raw, []byte(pending)...), 0644); err != nil {
		t.Fatal(err)
	}
	audit, err := run.MeasureCDFLiquidity()
	if err != nil {
		t.Fatal(err)
	}
	if !audit.Valid || audit.CensoredQuoteCount != 1 || audit.Suppliers[0].CensoredQuoteCount != 1 {
		t.Fatalf("pending submission audit = %+v, want one terminal censored quote", audit)
	}
}

func TestMeasureCDFLiquidityClosesRejectedSubmission(t *testing.T) {
	run := writeCDFLiquidityFixture(t, true, false)
	generalPath := filepath.Join(run.Dir, "venues", "north", "general.jsonl")
	raw, err := os.ReadFile(generalPath)
	if err != nil {
		t.Fatal(err)
	}
	submission := cdfFixtureLine(7, 2, "elastic_liquidity_supplier_decision", `{"role":"cdf_elastic_supplier_1","client_id":2,"symbol":"CDF/USD","decision_time":7,"observation_time":1,"observation_age":6,"observation_sequence":1,"best_bid":99,"best_bid_qty":10,"best_ask":101,"best_ask_qty":10,"mark_price":100,"reference_price":100,"position":5,"target_position":9,"inventory_limit":10,"action":"submit","reason":"inventory_target_gap","side":"BUY","quote_price":99,"quote_qty":3,"quote_request_id":3,"quote_submitted_at":7,"quote_cash_available":505,"quote_cash_required":297}`) + "\n"
	if err := os.WriteFile(generalPath, append(raw, []byte(submission)...), 0644); err != nil {
		t.Fatal(err)
	}
	bookPath := filepath.Join(run.Dir, "venues", "north", "spot", "CDF-USD.jsonl")
	book, err := os.ReadFile(bookPath)
	if err != nil {
		t.Fatal(err)
	}
	rejection := cdfFixtureLine(8, 2, "OrderRejected", `{"request_id":3,"success":false,"error":"INVALID_PRICE","symbol":"CDF/USD","side":"BUY","type":"LIMIT","time_in_force":"GTC","post_only":true,"price":99,"qty":3}`) + "\n"
	if err := os.WriteFile(bookPath, append(book, []byte(rejection)...), 0644); err != nil {
		t.Fatal(err)
	}
	audit, err := run.MeasureCDFLiquidity()
	if err != nil {
		t.Fatal(err)
	}
	if !audit.Valid || audit.CensoredQuoteCount != 0 || audit.Suppliers[0].CensoredQuoteCount != 0 {
		t.Fatalf("rejected submission audit = %+v, want no terminal censoring", audit)
	}
}

func writeCDFLiquidityReceiptFixture(t *testing.T) *Run {
	t.Helper()
	run := writeCDFLiquidityFixture(t, true, false)
	generalPath := filepath.Join(run.Dir, "venues", "north", "general.jsonl")
	raw, err := os.ReadFile(generalPath)
	if err != nil {
		t.Fatal(err)
	}
	frontierFields := `,"observation_link_id":1,"observation_ordinal":1,"observation_delivered_at":1,"observation_fingerprint":"01000000000000000000000000000000"`
	updated := strings.ReplaceAll(string(raw), `"observation_sequence":1`, `"observation_sequence":1`+frontierFields)
	if err := os.WriteFile(generalPath, []byte(updated), 0644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(run.Dir, "manifest.json")
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest := strings.Replace(string(manifestRaw), `"record_market_data_receipts":false`, `"record_market_data_receipts":true,"market_data_receipt_roles":["cdf_elastic_supplier"]`, 1)
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	recorder, err := simulation.NewMarketDataReceiptRecorder(run.Dir)
	if err != nil {
		t.Fatal(err)
	}
	link := "north/cdf_elastic_supplier"
	if recorder.RegisterLink("north", link, "cdf_elastic_supplier") == 0 {
		t.Fatal("receipt fixture link registration failed")
	}
	schedule := simulation.MarketDataSchedule{
		ClientID: 2, SourceVenue: "north", Link: link, Symbol: "CDF/USD", Type: exchange.MDSnapshot,
		Sequence: 1, Fingerprint: [16]byte{1}, PublishedAt: 1, ScheduledAt: 1, LinkOrdinal: 1,
	}
	if recorder.RecordSchedule(schedule) == 0 {
		t.Fatal("receipt fixture schedule registration failed")
	}
	frontier := recorder.RecordReceipt(simulation.MarketDataReceipt{MarketDataSchedule: schedule, DeliveredAt: 1})
	updatedRaw, err := os.ReadFile(generalPath)
	if err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", frontier.Digest)
	updatedWithDigest := strings.ReplaceAll(string(updatedRaw), `"observation_digest":""`, `"observation_digest":"`+digest+`"`)
	if err := os.WriteFile(generalPath, []byte(updatedWithDigest), 0644); err != nil {
		t.Fatal(err)
	}
	for _, decision := range []struct {
		requestID uint64
		price     int64
		qty       int64
		at        int64
	}{{1, 99, 5, 1}, {2, 99, 4, 4}} {
		recorder.RecordAction(simulation.MarketDataAction{
			ClientID: 2, SourceVenue: "north", Link: link, Symbol: "CDF/USD", RequestType: exchange.ReqPlaceOrder,
			RequestID: decision.requestID, Side: exchange.Buy, OrderType: exchange.LimitOrder, TimeInForce: exchange.GTC,
			Price: decision.price, Qty: decision.qty, DecisionAt: decision.at, Frontier: frontier,
		})
		recorder.RecordDecision(simulation.MarketDataDecision{
			ClientID: 2, SourceVenue: "north", Link: link, Symbol: "CDF/USD", RequestID: decision.requestID,
			Side: exchange.Buy, OrderType: exchange.LimitOrder, TimeInForce: exchange.GTC,
			Price: decision.price, Qty: decision.qty, DecisionAt: decision.at, Frontier: frontier,
		})
	}
	recorder.RecordAction(simulation.MarketDataAction{
		ClientID: 2, SourceVenue: "north", Link: link, RequestType: exchange.ReqCancelOrder,
		RequestID: 3, OrderID: 8, DecisionAt: 6, Frontier: frontier,
	})
	if err := recorder.Finalize(6); err != nil {
		t.Fatal(err)
	}
	return run
}

func writeCDFLiquidityFullFillCancelRaceFixture(t *testing.T) *Run {
	t.Helper()
	run := writeCDFLiquidityFixture(t, true, false)
	generalPath := filepath.Join(run.Dir, "venues", "north", "general.jsonl")
	general, err := os.ReadFile(generalPath)
	if err != nil {
		t.Fatal(err)
	}
	oldCancelDecision := cdfFixtureLine(6, 2, "elastic_liquidity_supplier_decision", `{"role":"cdf_elastic_supplier_1","client_id":2,"symbol":"CDF/USD","decision_time":6,"observation_time":1,"observation_age":5,"observation_sequence":1,"best_bid":99,"best_bid_qty":10,"best_ask":101,"best_ask_qty":10,"mark_price":100,"reference_price":100,"position":5,"target_position":9,"inventory_limit":10,"action":"cancel","reason":"reprice_for_inventory_or_touch","quote_order_id":8,"cancel_request_id":3}`) + "\n"
	staleWithdrawal := cdfFixtureLine(12, 2, "elastic_liquidity_supplier_decision", `{"role":"cdf_elastic_supplier_1","client_id":2,"symbol":"CDF/USD","decision_time":12,"observation_time":1,"observation_age":11,"observation_sequence":1,"best_bid":99,"best_bid_qty":10,"best_ask":101,"best_ask_qty":10,"mark_price":100,"reference_price":100,"position":5,"target_position":5,"inventory_limit":10,"action":"withdraw","reason":"stale_or_missing_observation","quote_order_id":8,"cancel_request_id":4}`) + "\n"
	if !strings.Contains(string(general), oldCancelDecision) {
		t.Fatal("fixture cancel decision was not found")
	}
	general = []byte(strings.Replace(string(general), oldCancelDecision, staleWithdrawal, 1))
	fillFields := `{"role":"cdf_elastic_supplier_1","client_id":2,"symbol":"CDF/USD","order_id":8,"trade_id":2,"timestamp":13,"side":"BUY","price":99,"qty":4,"fee_amount":0,"fee_asset":"","is_full":true,"position_before":5,"position_after":9}`
	general = append(general, []byte(cdfFixtureLine(13, 2, "elastic_liquidity_supplier_fill", fillFields)+"\n")...)
	if err := os.WriteFile(generalPath, general, 0644); err != nil {
		t.Fatal(err)
	}

	greeksPath := filepath.Join(run.Dir, "greeks.json")
	greeks, err := os.ReadFile(greeksPath)
	if err != nil {
		t.Fatal(err)
	}
	oldTerminal := `{"venue_id":"north","client_id":2,"role":"cdf_elastic_supplier_1","marks":{"CDF":100,"USD":1},"account":{"equity":11005,"spot_balances":[{"asset":"CDF","net_asset":105,"borrowed":0},{"asset":"USD","net_asset":505,"borrowed":0}]}}`
	newTerminal := `{"venue_id":"north","client_id":2,"role":"cdf_elastic_supplier_1","marks":{"CDF":100,"USD":1},"account":{"equity":11009,"spot_balances":[{"asset":"CDF","net_asset":109,"borrowed":0},{"asset":"USD","net_asset":109,"borrowed":0}]}}`
	if !strings.Contains(string(greeks), oldTerminal) {
		t.Fatal("fixture terminal account was not found")
	}
	if err := os.WriteFile(greeksPath, []byte(strings.Replace(string(greeks), oldTerminal, newTerminal, 1)), 0644); err != nil {
		t.Fatal(err)
	}

	bookPath := filepath.Join(run.Dir, "venues", "north", "spot", "CDF-USD.jsonl")
	book, err := os.ReadFile(bookPath)
	if err != nil {
		t.Fatal(err)
	}
	oldCancellation := cdfFixtureLine(6, 2, "OrderCancelled", `{"order_id":8,"request_id":2,"remaining_qty":4}`) + "\n"
	if !strings.Contains(string(book), oldCancellation) {
		t.Fatal("fixture cancellation was not found")
	}
	book = []byte(strings.Replace(string(book), oldCancellation, "", 1))
	book = append(book, []byte(cdfFixtureLine(13, 2, "OrderFill", `{"order_id":8,"trade_id":2,"side":"BUY","price":99,"qty":4,"filled_qty":4,"remaining_qty":0,"is_full":true}`)+"\n")...)
	book = append(book, []byte(cdfFixtureLine(14, 2, "OrderCancelRejected", `{"request_id":4,"success":false,"error":"ORDER_ALREADY_FILLED"}`)+"\n")...)
	if err := os.WriteFile(bookPath, book, 0644); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(run.Dir)
	if err != nil {
		t.Fatal(err)
	}
	return reopened
}

func writeCDFLiquidityFixture(t *testing.T, supplier, malformedFill bool) *Run {
	t.Helper()
	dir := t.TempDir()
	initialAccounts := `[{"venue_id":"north","client_id":1,"role":"elastic_supplier_1","account":{"equity":10}}]`
	terminalAccounts := `[{"venue_id":"north","client_id":1,"role":"elastic_supplier_1","account":{"equity":10}}]`
	general := ""
	if supplier {
		initialAccounts = `[{"venue_id":"north","client_id":1,"role":"elastic_supplier_1","account":{"equity":10}},{"venue_id":"north","client_id":2,"role":"cdf_elastic_supplier_1","marks":{"CDF":100,"USD":1},"account":{"equity":11000,"spot_balances":[{"asset":"CDF","net_asset":100,"borrowed":0},{"asset":"USD","net_asset":1000,"borrowed":0}]}}]`
		terminalAccounts = `[{"venue_id":"north","client_id":1,"role":"elastic_supplier_1","account":{"equity":10}},{"venue_id":"north","client_id":2,"role":"cdf_elastic_supplier_1","marks":{"CDF":100,"USD":1},"account":{"equity":11005,"spot_balances":[{"asset":"CDF","net_asset":105,"borrowed":0},{"asset":"USD","net_asset":505,"borrowed":0}]}}]`
		general = cdfFixtureLine(1, 2, "elastic_liquidity_supplier_decision", `{"role":"cdf_elastic_supplier_1","client_id":2,"symbol":"CDF/USD","decision_time":1,"observation_time":1,"observation_age":0,"observation_sequence":1,"best_bid":99,"best_bid_qty":10,"best_ask":101,"best_ask_qty":10,"mark_price":100,"reference_price":100,"position":0,"target_position":5,"inventory_limit":10,"action":"submit","reason":"inventory_target_gap","side":"BUY","quote_price":99,"quote_qty":5,"quote_request_id":1,"quote_submitted_at":1,"quote_cash_available":1000,"quote_cash_required":495}`) + "\n"
		fillFields := `"role":"cdf_elastic_supplier_1","client_id":2,"symbol":"CDF/USD","order_id":7,"trade_id":1,"timestamp":3,"side":"BUY","price":99,"qty":5,"fee_amount":0,"fee_asset":"","is_full":true,"position_before":0,"position_after":5`
		if malformedFill {
			fillFields = strings.Replace(fillFields, `,"is_full":true`, "", 1)
		}
		general += cdfFixtureLine(3, 2, "elastic_liquidity_supplier_fill", "{"+fillFields+"}") + "\n"
		general += cdfFixtureLine(2, 2, "balance_snapshot", `{"timestamp":2,"client_id":2,"spot_balances":[{"asset":"CDF","free":100,"locked":0,"borrowed":0,"interest":0,"net_asset":100},{"asset":"USD","free":1000,"locked":0,"borrowed":0,"interest":0,"net_asset":1000}],"perp_balances":[],"borrowed":{}}`) + "\n"
		general += cdfFixtureLine(6, 2, "balance_snapshot", `{"timestamp":6,"client_id":2,"spot_balances":[{"asset":"CDF","free":105,"locked":0,"borrowed":0,"interest":0,"net_asset":105},{"asset":"USD","free":505,"locked":0,"borrowed":0,"interest":0,"net_asset":505}],"perp_balances":[],"borrowed":{}}`) + "\n"
		general += cdfFixtureLine(4, 2, "elastic_liquidity_supplier_decision", `{"role":"cdf_elastic_supplier_1","client_id":2,"symbol":"CDF/USD","decision_time":4,"observation_time":1,"observation_age":3,"observation_sequence":1,"best_bid":99,"best_bid_qty":10,"best_ask":101,"best_ask_qty":10,"mark_price":100,"reference_price":100,"position":5,"target_position":9,"inventory_limit":10,"action":"submit","reason":"inventory_target_gap","side":"BUY","quote_price":99,"quote_qty":4,"quote_request_id":2,"quote_submitted_at":4,"quote_cash_available":505,"quote_cash_required":396}`) + "\n"
		general += cdfFixtureLine(6, 2, "elastic_liquidity_supplier_decision", `{"role":"cdf_elastic_supplier_1","client_id":2,"symbol":"CDF/USD","decision_time":6,"observation_time":1,"observation_age":5,"observation_sequence":1,"best_bid":99,"best_bid_qty":10,"best_ask":101,"best_ask_qty":10,"mark_price":100,"reference_price":100,"position":5,"target_position":9,"inventory_limit":10,"action":"cancel","reason":"reprice_for_inventory_or_touch","quote_order_id":8,"cancel_request_id":3}`) + "\n"
	}
	greeks := fmt.Sprintf(`{"initial_accounts":%s,"terminal_accounts":%s}`, initialAccounts, terminalAccounts)
	if err := os.WriteFile(filepath.Join(dir, "greeks.json"), []byte(greeks), 0644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"config":{"elastic_supplier_count":1,"record_market_data_receipts":false,"elastic_liquidity_suppliers":[]}}`
	if supplier {
		manifest = `{"config":{"elastic_supplier_count":1,"record_market_data_receipts":false,"elastic_liquidity_suppliers":[{"role":"cdf_elastic_supplier_1","symbol":"CDF/USD","base_asset":"CDF","quote_asset":"USD","base_precision":1,"quote_precision":1,"initial_base_balance":100,"initial_quote_balance":1000,"max_position":10,"max_inventory":110,"max_quote_qty":5,"max_observation_age":10}]}}`
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	venueDir := filepath.Join(dir, "venues", "north", "spot")
	if err := os.MkdirAll(venueDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "venues", "north", "general.jsonl"), []byte(general), 0644); err != nil {
		t.Fatal(err)
	}
	book := cdfFixtureLine(1, 0, "BookSnapshot", `{"bids":[{"price":99,"visible_qty":10,"hidden_qty":0}],"asks":[{"price":101,"visible_qty":10,"hidden_qty":0}]}`) + "\n"
	book += cdfFixtureLine(2, 2, "OrderAccepted", `{"order_id":7,"client_id":2,"request_id":1,"side":"BUY","type":"LIMIT","time_in_force":"GTC","post_only":true,"price":99,"qty":5}`) + "\n"
	book += cdfFixtureLine(2, 0, "BookSnapshot", `{"bids":[{"price":99,"visible_qty":10,"hidden_qty":0}],"asks":[{"price":101,"visible_qty":10,"hidden_qty":0}]}`) + "\n"
	book += cdfFixtureLine(3, 0, "Trade", `{"trade_id":1,"price":99,"qty":5,"side":"SELL","taker_order_id":9}`) + "\n"
	book += cdfFixtureLine(3, 2, "OrderFill", `{"order_id":7,"trade_id":1,"side":"BUY","price":99,"qty":5,"filled_qty":5,"remaining_qty":0,"is_full":true}`) + "\n"
	book += cdfFixtureLine(4, 0, "Trade", `{"trade_id":2,"price":99,"qty":20,"side":"SELL","taker_order_id":10}`) + "\n"
	book += cdfFixtureLine(5, 2, "OrderAccepted", `{"order_id":8,"client_id":2,"request_id":2,"side":"BUY","type":"LIMIT","time_in_force":"GTC","post_only":true,"price":99,"qty":4}`) + "\n"
	book += cdfFixtureLine(6, 2, "OrderCancelled", `{"order_id":8,"request_id":2,"remaining_qty":4}`) + "\n"
	if err := os.WriteFile(filepath.Join(venueDir, "CDF-USD.jsonl"), []byte(book), 0644); err != nil {
		t.Fatal(err)
	}
	run, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func cdfFixtureLine(sequence uint64, clientID uint64, event, payload string) string {
	if event == "elastic_liquidity_supplier_decision" {
		position := int64(0)
		if strings.Contains(payload, `"position":5`) {
			position = 5
		}
		payload = strings.TrimSuffix(payload, "}") + fmt.Sprintf(`,"observation_digest":"","initial_base_balance":100,"gross_inventory":%d,"gross_inventory_limit":110}`, 100+position)
	}
	return fmt.Sprintf(`{"client_id":%d,"data":{"venue_id":"north","sequence":%d,"payload":%s},"event":"%s","sim_ts":%d}`, clientID, sequence, payload, event, sequence)
}

func hasCDFCheck(checks []CDFLiquidityCheck, prefix string) bool {
	for _, check := range checks {
		if strings.HasPrefix(check.Failure, prefix) {
			return true
		}
	}
	return false
}
