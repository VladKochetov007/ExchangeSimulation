package analysis

import (
	"fmt"
	"testing"
)

func settledLine(ts int64, venue, symbol string, price, expiry int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":0,"event":"instrument_settled","data":{"venue_id":%q,"payload":{"action":"settled","symbol":%q,"instrument_type":"FUTURE","expiry_nano":%d,"settlement_price":%d,"timestamp":%d}}}`,
		ts, venue, symbol, expiry, price, ts)
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
