package analysis

import (
	"fmt"
	"testing"
)

func fundingRateLine(ts int64, venue string, rate int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":0,"event":"funding_rate_update","data":{"venue_id":%q,"payload":{"symbol":"ABC-PERP","payload":{"timestamp":%d,"symbol":"ABC-PERP","rate":%d}}}}`,
		ts, venue, ts, rate)
}

func fundingPayLine(ts int64, venue string, clientID uint64, delta int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"balance_change","data":{"venue_id":%q,"payload":{"symbol":"ABC-PERP","payload":{"timestamp":%d,"client_id":%d,"symbol":"ABC-PERP","reason":"funding_settlement","changes":[{"asset":"USD","wallet":"perp","old_balance":0,"new_balance":%d,"delta":%d}]}}}}`,
		ts, clientID, venue, ts, clientID, delta, delta)
}

func optionSettledLine(ts int64, venue, symbol string, strike, settle int64, isCall bool) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":0,"event":"instrument_settled","data":{"venue_id":%q,"payload":{"action":"settled","symbol":%q,"instrument_type":"OPTION","strike":%d,"is_call":%t,"settlement_price":%d,"expiry_nano":%d,"timestamp":%d}}}`,
		ts, venue, symbol, strike, isCall, settle, ts, ts)
}

// Funding is a transfer between the two sides of one contract. An instant that
// does not net to zero is the venue paying, and a direction that contradicts
// the published rate is a sign error in the mechanism.
func TestFundingAuditCatchesUnbalancedAndMisdirectedSettlements(t *testing.T) {
	const instant = int64(1_000_000_000)
	good := []string{
		fundingRateLine(instant-1, "north", 5),
		fundingPayLine(instant, "north", 1, -400),
		fundingPayLine(instant, "north", 2, 400),
	}
	dir := writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": good})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureDerivativeSemantics(DerivativeAuditOptions{BasePrecision: auditPrecision})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if result.FundingBroken != 0 || result.FundingSignWrong != 0 {
		t.Fatalf("a balanced settlement was rejected: %+v", result.Funding)
	}

	bad := []string{
		fundingRateLine(instant-1, "north", 5),
		fundingPayLine(instant, "north", 1, -400),
		fundingPayLine(instant, "north", 2, 500),
	}
	dir = writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": bad})
	run, _ = Open(dir)
	result, err = run.MeasureDerivativeSemantics(DerivativeAuditOptions{BasePrecision: auditPrecision})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if result.FundingBroken != 1 {
		t.Errorf("an unbalanced settlement was accepted: %+v", result.Funding)
	}

	// A positive rate with nobody charged is the direction being wrong.
	misdirected := []string{
		fundingRateLine(instant-1, "north", 5),
		fundingPayLine(instant, "north", 1, 400),
		fundingPayLine(instant, "north", 2, -400),
	}
	misdirected[1] = fundingPayLine(instant, "north", 1, 0)
	misdirected[2] = fundingPayLine(instant, "north", 2, 0)
	dir = writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": misdirected})
	run, _ = Open(dir)
	result, _ = run.MeasureDerivativeSemantics(DerivativeAuditOptions{BasePrecision: auditPrecision})
	if result.FundingSignWrong != 1 {
		t.Errorf("a positive rate that charged nobody was accepted: %+v", result.Funding)
	}
}

// An option pays its intrinsic value, one holder's gain is another's loss, and
// a worthless option pays nothing at all.
func TestExerciseAuditRecomputesIntrinsicValueAndCatchesWorthlessPayouts(t *testing.T) {
	const expiry = int64(1_000_000_000)
	strike := 50_000 * auditPrecision
	settle := 55_000 * auditPrecision
	// A long of one contract and a short of one, call, five thousand in the
	// money: the long receives five thousand and the short pays it.
	intrinsic := settle - strike
	lines := []string{
		positionLine(expiry-1, "north", 1, "ABC-1-C", auditPrecision, 0),
		positionLine(expiry-1, "north", 2, "ABC-1-C", -auditPrecision, 0),
		optionSettledLine(expiry, "north", "ABC-1-C", strike, settle, true),
		expiryPayLine(expiry, "north", 1, "ABC-1-C", intrinsic),
		expiryPayLine(expiry, "north", 2, "ABC-1-C", -intrinsic),
	}
	dir := writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureDerivativeSemantics(DerivativeAuditOptions{BasePrecision: auditPrecision})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if len(result.Exercises) != 1 {
		t.Fatalf("exercises = %d, want 1", len(result.Exercises))
	}
	check := result.Exercises[0]
	if check.Intrinsic != intrinsic {
		t.Errorf("intrinsic = %d, want %d", check.Intrinsic, intrinsic)
	}
	if check.Residual != 0 {
		t.Errorf("residual = %d, want the payout to match the terms", check.Residual)
	}

	// The same option finishing out of the money must pay nothing.
	worthless := []string{
		positionLine(expiry-1, "north", 1, "ABC-2-C", auditPrecision, 0),
		optionSettledLine(expiry, "north", "ABC-2-C", strike, strike-auditPrecision, true),
		expiryPayLine(expiry, "north", 1, "ABC-2-C", 12345),
	}
	dir = writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": worthless})
	run, _ = Open(dir)
	result, _ = run.MeasureDerivativeSemantics(DerivativeAuditOptions{BasePrecision: auditPrecision})
	if result.WorthlessPaid != 1 {
		t.Errorf("an out-of-the-money option paid out and was accepted: %+v", result.Exercises)
	}
}
