package analysis

import (
	"fmt"
	"math"
	"testing"
)

func settledLine(ts int64, venue, symbol string, price, expiry int64) string {
	return settledLineWithPrecision(ts, venue, symbol, price, expiry, auditPrecision)
}

func settledLineWithPrecision(ts int64, venue, symbol string, price, expiry, precision int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":0,"event":"instrument_settled","data":{"venue_id":%q,"payload":{"action":"settled","symbol":%q,"instrument_type":"FUTURE","quote_asset":"USD","base_precision":%d,"expiry_nano":%d,"settlement_price":%d,"timestamp":%d}}}`,
		ts, venue, symbol, precision, expiry, price, ts)
}

func TestSettlementAuditDoesNotConvertOverflowToZeroPayout(t *testing.T) {
	const expiry = int64(1_000_000_000)
	lines := []string{
		positionLine(expiry-1, "north", 1, "OIL-FUT-1", 2, math.MinInt64),
		settledLine(expiry, "north", "OIL-FUT-1", math.MaxInt64, expiry),
	}
	dir := writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureSettlements(SettlementAuditOptions{BasePrecision: 1})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if result.ArithmeticFailures != 1 || !result.Checks[0].Unrepresentable {
		t.Fatalf("overflow status = %+v, want explicit unrepresentable check", result)
	}
	if result.Mismatched != 0 {
		t.Fatalf("unrepresentable contract became mismatch=%d instead of explicit unresolved status", result.Mismatched)
	}
}

func TestStrictSettlementAuditIgnoresNonFutureFills(t *testing.T) {
	const expiry = int64(1_000_000_000)
	lines := []string{
		fmt.Sprintf(`{"sim_ts":1,"client_id":0,"event":"instrument_listed","data":{"venue_id":"north","payload":{"action":"listed","symbol":"ABC-FUT-1","instrument_type":"FUTURE","quote_asset":"USD","base_precision":%d,"expiry_nano":%d,"timestamp":1}}}`, auditPrecision, expiry),
		settledLine(expiry, "north", "ABC-FUT-1", 105*auditPrecision, expiry),
		expiryFillLine(expiry, "north", "ABC-PERP"),
	}
	run, err := Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureSettlements(SettlementAuditOptions{BasePrecision: auditPrecision, RequireExactReplay: true, DeliveryFeePolicy: "zero"})
	if err != nil {
		t.Fatal(err)
	}
	if result.SettlementEventMismatches != 0 || result.SettlementTimingFailures != 0 {
		t.Fatalf("non-future fill contaminated settlement audit: %+v", result)
	}
}

func TestSettlementAuditRejectsExplicitlyUnavailablePrice(t *testing.T) {
	line := `{"sim_ts":1000000000,"client_id":0,"event":"instrument_settled","data":{"venue_id":"north","payload":{"action":"settled","symbol":"OIL-FUT-1","instrument_type":"FUTURE","expiry_nano":1000000000,"settlement_price":0,"settlement_price_available":false,"timestamp":1000000000}}}`
	dir := writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": {line}})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureSettlements(SettlementAuditOptions{BasePrecision: 1})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if result.ExplicitUnavailableAnnouncements != 1 || len(result.Checks) != 0 {
		t.Fatalf("unavailable terminal settlement was reconstructed: %+v", result)
	}
}

func positionLine(ts int64, venue string, clientID uint64, symbol string, size, entry int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"position_update","data":{"venue_id":%q,"payload":{"symbol":%q,"payload":{"timestamp":%d,"client_id":%d,"symbol":%q,"new_size":%d,"new_entry_price":%d}}}}`,
		ts, clientID, venue, symbol, ts, clientID, symbol, size, entry)
}

func expiryPayLine(ts int64, venue string, clientID uint64, symbol string, amount int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"balance_change","data":{"venue_id":%q,"payload":{"symbol":%q,"payload":{"timestamp":%d,"client_id":%d,"symbol":%q,"reason":"expiry_settlement","changes":[{"asset":"USD","wallet":"perp","old_balance":0,"new_balance":%d,"delta":%d}]}}}}`,
		ts, clientID, venue, symbol, ts, clientID, symbol, amount, amount)
}

const auditPrecision = int64(100_000_000)

// A settlement is right only if the money matches the positions and the
// published price. The audit recomputes it from three independent streams, so
// it has to catch a venue that pays the wrong amount.
func TestSettlementAuditRecomputesThePayout(t *testing.T) {
	const expiry = int64(1_000_000_000)
	// One long entered at 100 and one short at 110, one contract each,
	// settling at 105. The payout is the price difference times the size,
	// scaled out of base precision.
	const long, short = auditPrecision, -auditPrecision
	longPay := (105*auditPrecision - 100*auditPrecision) * long / auditPrecision
	shortPay := (105*auditPrecision - 110*auditPrecision) * short / auditPrecision
	lines := []string{
		positionLine(expiry-1, "north", 1, "ABC-FUT-1", long, 100*auditPrecision),
		positionLine(expiry-1, "north", 2, "ABC-FUT-1", short, 110*auditPrecision),
		settledLine(expiry, "north", "ABC-FUT-1", 105*auditPrecision, expiry),
		expiryPayLine(expiry, "north", 1, "ABC-FUT-1", longPay),
		expiryPayLine(expiry, "north", 2, "ABC-FUT-1", shortPay),
	}
	dir := writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureSettlements(SettlementAuditOptions{BasePrecision: auditPrecision})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if len(result.Checks) != 1 {
		t.Fatalf("checks = %d, want 1", len(result.Checks))
	}
	check := result.Checks[0]
	if check.Holders != 2 || check.PaidAccounts != 2 {
		t.Errorf("holders %d paid %d, want two of each", check.Holders, check.PaidAccounts)
	}
	if check.NetSize != 0 {
		t.Errorf("net size at expiry = %d, want zero", check.NetSize)
	}
	if check.Residual != 0 {
		t.Errorf("residual = %d, want the payout to match the positions", check.Residual)
	}
}

func TestSettlementAuditUsesExactReplayCashAndVerifiesExpiryEvidence(t *testing.T) {
	const expiry = int64(10)
	lines := []string{
		`{"sim_ts":1,"client_id":0,"event":"instrument_listed","data":{"venue_id":"north","payload":{"action":"listed","symbol":"ABC-PERP","instrument_type":"PERP","quote_asset":"USD","base_precision":1,"timestamp":1}}}`,
		`{"sim_ts":1,"client_id":9,"event":"position_update","data":{"venue_id":"north","symbol":"ABC-PERP","payload":{"timestamp":1,"client_id":9,"symbol":"ABC-PERP","position_side":"BOTH","base_precision":1,"old_size":0,"old_entry_price":0,"new_size":1,"new_entry_price":100,"trade_qty":1,"trade_price":100,"trade_side":"BUY","reason":"trade"}}}`,
		`{"sim_ts":1,"client_id":0,"event":"instrument_listed","data":{"venue_id":"north","payload":{"action":"listed","symbol":"ABC-FUT-1","instrument_type":"FUTURE","quote_asset":"USD","base_precision":1,"expiry_nano":10,"timestamp":1}}}`,
		`{"sim_ts":1,"client_id":1,"event":"position_update","data":{"venue_id":"north","symbol":"ABC-FUT-1","payload":{"timestamp":1,"client_id":1,"symbol":"ABC-FUT-1","position_side":"BOTH","base_precision":1,"old_size":0,"old_entry_price":0,"new_size":3,"new_entry_price":100,"trade_qty":3,"trade_price":100,"trade_side":"BUY","reason":"trade"}}}`,
		`{"sim_ts":2,"client_id":1,"event":"position_update","data":{"venue_id":"north","symbol":"ABC-FUT-1","payload":{"timestamp":2,"client_id":1,"symbol":"ABC-FUT-1","position_side":"BOTH","base_precision":1,"old_size":3,"old_entry_price":100,"new_size":5,"new_entry_price":100,"trade_qty":2,"trade_price":101,"trade_side":"BUY","reason":"trade"}}}`,
		`{"sim_ts":3,"client_id":1,"event":"position_update","data":{"venue_id":"north","symbol":"ABC-FUT-1","payload":{"timestamp":3,"client_id":1,"symbol":"ABC-FUT-1","position_side":"BOTH","base_precision":1,"old_size":5,"old_entry_price":100,"new_size":3,"new_entry_price":100,"trade_qty":2,"trade_price":102,"trade_side":"SELL","reason":"trade"}}}`,
		`{"sim_ts":1,"client_id":2,"event":"position_update","data":{"venue_id":"north","symbol":"ABC-FUT-1","payload":{"timestamp":1,"client_id":2,"symbol":"ABC-FUT-1","position_side":"BOTH","base_precision":1,"old_size":0,"old_entry_price":0,"new_size":-3,"new_entry_price":110,"trade_qty":3,"trade_price":110,"trade_side":"SELL","reason":"trade"}}}`,
		settledLineWithPrecision(expiry, "north", "ABC-FUT-1", 103, expiry, 1),
		`{"sim_ts":10,"client_id":1,"event":"expiry_settlement","data":{"venue_id":"north","symbol":"ABC-FUT-1","payload":{"timestamp":10,"client_id":1,"symbol":"ABC-FUT-1","position_side":"BOTH","base_precision":1,"size":3,"entry_price":100,"settlement_price":103,"cash_flow":8,"delivery_fee":0}}}`,
		`{"sim_ts":10,"client_id":1,"event":"balance_change","data":{"venue_id":"north","symbol":"ABC-FUT-1","payload":{"timestamp":10,"client_id":1,"symbol":"ABC-FUT-1","position_side":"BOTH","reason":"expiry_settlement","changes":[{"asset":"USD","wallet":"perp","old_balance":0,"new_balance":8,"delta":8}]}}}`,
		`{"sim_ts":10,"client_id":2,"event":"expiry_settlement","data":{"venue_id":"north","symbol":"ABC-FUT-1","payload":{"timestamp":10,"client_id":2,"symbol":"ABC-FUT-1","position_side":"BOTH","base_precision":1,"size":-3,"entry_price":110,"settlement_price":103,"cash_flow":21,"delivery_fee":0}}}`,
		`{"sim_ts":10,"client_id":2,"event":"balance_change","data":{"venue_id":"north","symbol":"ABC-FUT-1","payload":{"timestamp":10,"client_id":2,"symbol":"ABC-FUT-1","position_side":"BOTH","reason":"expiry_settlement","changes":[{"asset":"USD","wallet":"perp","old_balance":0,"new_balance":21,"delta":21}]}}}`,
	}
	dir := writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureSettlements(SettlementAuditOptions{BasePrecision: 1, RequireExactReplay: true, DeliveryFeePolicy: "zero"})
	if err != nil {
		t.Fatalf("measure settlements: %v", err)
	}
	if result.ExactReplayChecks != 4 || result.ExactReplayFailures != 0 {
		t.Fatalf("exact replay status = (%d, %d), want (4, 0)", result.ExactReplayChecks, result.ExactReplayFailures)
	}
	if result.Mismatched != 0 || result.SettlementEventMismatches != 0 || result.Checks[0].Residual != 0 {
		t.Fatalf("exact settlement audit = %+v, want no mismatch", result)
	}
	if result.Checks[0].ExpectedPayout != 29 || result.Checks[0].ExpectedNetPayout != 29 {
		t.Fatalf("exact expected payout = %+v, want 29 net", result.Checks[0])
	}
}

func TestStrictSettlementAuditRejectsNonZeroNetFutureSupply(t *testing.T) {
	const expiry = int64(10)
	lines := []string{
		`{"sim_ts":1,"client_id":0,"event":"instrument_listed","data":{"venue_id":"north","payload":{"action":"listed","symbol":"ABC-FUT-1","instrument_type":"FUTURE","quote_asset":"USD","base_precision":1,"expiry_nano":10,"timestamp":1}}}`,
		`{"sim_ts":1,"client_id":1,"event":"position_update","data":{"venue_id":"north","symbol":"ABC-FUT-1","payload":{"timestamp":1,"client_id":1,"symbol":"ABC-FUT-1","position_side":"BOTH","base_precision":1,"old_size":0,"old_entry_price":0,"new_size":1,"new_entry_price":100,"trade_qty":1,"trade_price":100,"trade_side":"BUY","reason":"trade"}}}`,
		settledLineWithPrecision(expiry, "north", "ABC-FUT-1", 103, expiry, 1),
		`{"sim_ts":10,"client_id":1,"event":"expiry_settlement","data":{"venue_id":"north","symbol":"ABC-FUT-1","payload":{"timestamp":10,"client_id":1,"symbol":"ABC-FUT-1","position_side":"BOTH","base_precision":1,"size":1,"entry_price":100,"settlement_price":103,"cash_flow":3,"delivery_fee":0}}}`,
		expiryPayLine(expiry, "north", 1, "ABC-FUT-1", 3),
	}
	run, err := Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureSettlements(SettlementAuditOptions{BasePrecision: 1, RequireExactReplay: true, DeliveryFeePolicy: "zero"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Mismatched != 1 || result.Checks[0].NetSize != 1 || result.Checks[0].EventMismatches == 0 {
		t.Fatalf("non-zero net supply was accepted: %+v", result)
	}
}

func TestStrictSettlementAuditRequiresPinnedDeliveryFeePolicy(t *testing.T) {
	dir := writeRun(t, Report{}, map[string][]string{})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := run.MeasureSettlements(SettlementAuditOptions{BasePrecision: 1, RequireExactReplay: true}); err == nil {
		t.Fatal("strict settlement audit accepted an unpinned delivery fee policy")
	}
}

func TestStrictSettlementAuditBindsPaymentAfterSettlementInTheSameFile(t *testing.T) {
	const expiry = int64(10)
	lines := []string{
		`{"sim_ts":1,"client_id":0,"event":"instrument_listed","data":{"venue_id":"north","payload":{"action":"listed","symbol":"ABC-FUT-1","instrument_type":"FUTURE","quote_asset":"USD","base_precision":1,"expiry_nano":10,"timestamp":1}}}`,
		`{"sim_ts":1,"client_id":1,"event":"position_update","data":{"venue_id":"north","symbol":"ABC-FUT-1","payload":{"timestamp":1,"client_id":1,"symbol":"ABC-FUT-1","position_side":"BOTH","base_precision":1,"old_size":0,"old_entry_price":0,"new_size":3,"new_entry_price":100,"trade_qty":3,"trade_price":100,"trade_side":"BUY","reason":"trade"}}}`,
		settledLineWithPrecision(expiry, "north", "ABC-FUT-1", 103, expiry, 1),
		`{"sim_ts":10,"client_id":1,"event":"balance_change","data":{"venue_id":"north","symbol":"ABC-FUT-1","payload":{"timestamp":10,"client_id":1,"symbol":"ABC-FUT-1","position_side":"BOTH","reason":"expiry_settlement","changes":[{"asset":"USD","wallet":"perp","old_balance":0,"new_balance":8,"delta":8}]}}}`,
		`{"sim_ts":10,"client_id":1,"event":"expiry_settlement","data":{"venue_id":"north","symbol":"ABC-FUT-1","payload":{"timestamp":10,"client_id":1,"symbol":"ABC-FUT-1","position_side":"BOTH","base_precision":1,"size":3,"entry_price":100,"settlement_price":103,"cash_flow":8,"delivery_fee":0}}}`,
	}
	dir := writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureSettlements(SettlementAuditOptions{BasePrecision: 1, RequireExactReplay: true, DeliveryFeePolicy: "zero"})
	if err != nil {
		t.Fatalf("measure settlements: %v", err)
	}
	if result.SettlementTimingFailures == 0 || result.Mismatched == 0 {
		t.Fatalf("causal timing violation was accepted: %+v", result)
	}
}

func TestStrictSettlementAuditUsesIndependentDeliveryFeeResolver(t *testing.T) {
	const expiry = int64(10)
	lines := []string{
		`{"sim_ts":1,"client_id":0,"event":"instrument_listed","data":{"venue_id":"north","payload":{"action":"listed","symbol":"ABC-FUT-1","instrument_type":"FUTURE","quote_asset":"USD","base_precision":1,"expiry_nano":10,"timestamp":1}}}`,
		`{"sim_ts":1,"client_id":1,"event":"position_update","data":{"venue_id":"north","symbol":"ABC-FUT-1","payload":{"timestamp":1,"client_id":1,"symbol":"ABC-FUT-1","position_side":"BOTH","base_precision":1,"old_size":0,"old_entry_price":0,"new_size":3,"new_entry_price":100,"trade_qty":3,"trade_price":100,"trade_side":"BUY","reason":"trade"}}}`,
		`{"sim_ts":1,"client_id":2,"event":"position_update","data":{"venue_id":"north","symbol":"ABC-FUT-1","payload":{"timestamp":1,"client_id":2,"symbol":"ABC-FUT-1","position_side":"BOTH","base_precision":1,"old_size":0,"old_entry_price":0,"new_size":-3,"new_entry_price":110,"trade_qty":3,"trade_price":110,"trade_side":"SELL","reason":"trade"}}}`,
		settledLineWithPrecision(expiry, "north", "ABC-FUT-1", 103, expiry, 1),
		`{"sim_ts":10,"client_id":1,"event":"expiry_settlement","data":{"venue_id":"north","symbol":"ABC-FUT-1","payload":{"timestamp":10,"client_id":1,"symbol":"ABC-FUT-1","position_side":"BOTH","base_precision":1,"size":3,"entry_price":100,"settlement_price":103,"cash_flow":9,"delivery_fee":1}}}`,
		`{"sim_ts":10,"client_id":1,"event":"balance_change","data":{"venue_id":"north","symbol":"ABC-FUT-1","payload":{"timestamp":10,"client_id":1,"symbol":"ABC-FUT-1","position_side":"BOTH","reason":"expiry_settlement","changes":[{"asset":"USD","wallet":"perp","old_balance":0,"new_balance":8,"delta":8}]}}}`,
		`{"sim_ts":10,"client_id":2,"event":"expiry_settlement","data":{"venue_id":"north","symbol":"ABC-FUT-1","payload":{"timestamp":10,"client_id":2,"symbol":"ABC-FUT-1","position_side":"BOTH","base_precision":1,"size":-3,"entry_price":110,"settlement_price":103,"cash_flow":21,"delivery_fee":1}}}`,
		`{"sim_ts":10,"client_id":2,"event":"balance_change","data":{"venue_id":"north","symbol":"ABC-FUT-1","payload":{"timestamp":10,"client_id":2,"symbol":"ABC-FUT-1","position_side":"BOTH","reason":"expiry_settlement","changes":[{"asset":"USD","wallet":"perp","old_balance":0,"new_balance":20,"delta":20}]}}}`,
	}
	dir := writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureSettlements(SettlementAuditOptions{
		BasePrecision: 1, RequireExactReplay: true,
		DeliveryFeeResolver: func(context DeliveryFeeContext) (int64, error) {
			if context.Symbol != "ABC-FUT-1" || (context.Size != 3 && context.Size != -3) || context.SettlementPrice != 103 || context.BasePrecision != 1 {
				t.Fatalf("unexpected fee context: %+v", context)
			}
			return 1, nil
		},
	})
	if err != nil {
		t.Fatalf("measure settlements: %v", err)
	}
	check := result.Checks[0]
	if result.Mismatched != 0 || result.DeliveryFeeMismatches != 0 || check.DeliveryFee != 2 || check.LoggedDeliveryFee != 2 || check.ExpectedNetPayout != 28 {
		t.Fatalf("resolver settlement audit = %+v, want two independent fees and no mismatch", result)
	}
}

func TestStrictSettlementAuditRejectsMissingZeroSettlementFields(t *testing.T) {
	const expiry = int64(10)
	lines := []string{
		`{"sim_ts":1,"client_id":0,"event":"instrument_listed","data":{"venue_id":"north","payload":{"action":"listed","symbol":"ABC-FUT-1","instrument_type":"FUTURE","quote_asset":"USD","base_precision":1,"expiry_nano":10,"timestamp":1}}}`,
		`{"sim_ts":1,"client_id":1,"event":"position_update","data":{"venue_id":"north","symbol":"ABC-FUT-1","payload":{"timestamp":1,"client_id":1,"symbol":"ABC-FUT-1","position_side":"BOTH","base_precision":1,"old_size":0,"old_entry_price":0,"new_size":1,"new_entry_price":100,"trade_qty":1,"trade_price":100,"trade_side":"BUY","reason":"trade"}}}`,
		settledLineWithPrecision(expiry, "north", "ABC-FUT-1", 100, expiry, 1),
		`{"sim_ts":10,"client_id":1,"event":"expiry_settlement","data":{"venue_id":"north","symbol":"ABC-FUT-1","payload":{"timestamp":10,"client_id":1,"symbol":"ABC-FUT-1","position_side":"BOTH","base_precision":1,"size":1,"entry_price":100}}}`,
		`{"sim_ts":10,"client_id":1,"event":"balance_change","data":{"venue_id":"north","symbol":"ABC-FUT-1","payload":{"timestamp":10,"client_id":1,"symbol":"ABC-FUT-1","position_side":"BOTH","reason":"expiry_settlement","changes":[{"asset":"USD","wallet":"perp","old_balance":0,"new_balance":0,"delta":0}]}}}`,
	}
	run, err := Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureSettlements(SettlementAuditOptions{BasePrecision: 1, RequireExactReplay: true, DeliveryFeePolicy: "zero"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Mismatched != 1 || result.SettlementEventMismatches == 0 {
		t.Fatalf("missing zero settlement fields = %+v, want fail-closed mismatch", result)
	}
}

func TestSettlementAuditRejectsUnpinnedPrecision(t *testing.T) {
	dir := writeRun(t, Report{}, map[string][]string{})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := run.MeasureSettlements(SettlementAuditOptions{}); err == nil {
		t.Fatal("settlement audit accepted nonpositive precision")
	}
}

func TestSettlementBalanceDeltaRequiresOneConsistentPerpetualWalletChange(t *testing.T) {
	valid := balanceChangeRecord{Changes: []balanceDeltaRecord{{
		Asset: "USD", Wallet: "perp", OldBalance: 10, NewBalance: 17, Delta: 7,
	}}}
	if amount, ok := settlementBalanceDelta(valid); !ok || amount != 7 {
		t.Fatalf("valid settlement balance change = (%d, %t), want (7, true)", amount, ok)
	}
	for name, record := range map[string]balanceChangeRecord{
		"multiple changes": {Changes: []balanceDeltaRecord{
			{Asset: "USD", Wallet: "perp", OldBalance: 0, NewBalance: 7, Delta: 7},
			{Asset: "USD", Wallet: "spot", OldBalance: 0, NewBalance: -7, Delta: -7},
		}},
		"wrong wallet": {Changes: []balanceDeltaRecord{{
			Asset: "USD", Wallet: "spot", OldBalance: 0, NewBalance: 7, Delta: 7,
		}}},
		"inconsistent delta": {Changes: []balanceDeltaRecord{{
			Asset: "USD", Wallet: "perp", OldBalance: 0, NewBalance: 7, Delta: 6,
		}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, ok := settlementBalanceDelta(record); ok {
				t.Fatal("malformed settlement balance change was accepted")
			}
		})
	}
}

// The audit must fail when a holder is not paid, which is the failure a total
// that happens to net out would hide.
func TestSettlementAuditCatchesAnUnpaidHolder(t *testing.T) {
	const expiry = int64(1_000_000_000)
	lines := []string{
		positionLine(expiry-1, "north", 1, "ABC-FUT-1", auditPrecision, 100*auditPrecision),
		positionLine(expiry-1, "north", 2, "ABC-FUT-1", -auditPrecision, 100*auditPrecision),
		settledLine(expiry, "north", "ABC-FUT-1", 105*auditPrecision, expiry),
		expiryPayLine(expiry, "north", 1, "ABC-FUT-1", 5*auditPrecision),
	}
	dir := writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureSettlements(SettlementAuditOptions{BasePrecision: auditPrecision})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if result.Unpaid != 1 {
		t.Errorf("unpaid contracts = %d, want 1: %+v", result.Unpaid, result.Checks)
	}
	if result.Mismatched != 1 {
		t.Errorf("payout mismatches = %d, want 1: the short was never charged", result.Mismatched)
	}
}

// Trading a contract after it has expired is a lifecycle failure, and the
// audit is the thing that would notice.
func TestSettlementAuditCatchesAFillAfterExpiry(t *testing.T) {
	const expiry = int64(1_000_000_000)
	lines := []string{
		positionLine(expiry-1, "north", 1, "ABC-FUT-1", auditPrecision, 100*auditPrecision),
		settledLine(expiry, "north", "ABC-FUT-1", 100*auditPrecision, expiry),
		expiryPayLine(expiry, "north", 1, "ABC-FUT-1", 0),
		fmt.Sprintf(`{"sim_ts":%d,"client_id":3,"event":"OrderFill","data":{"venue_id":"north","payload":{"symbol":"ABC-FUT-1","qty":100,"role":"taker","side":"BUY"}}}`, expiry+1),
	}
	dir := writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureSettlements(SettlementAuditOptions{BasePrecision: auditPrecision})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if result.TotalTradesAfterExpiry != 1 {
		t.Errorf("fills after expiry = %d, want 1", result.TotalTradesAfterExpiry)
	}
}
