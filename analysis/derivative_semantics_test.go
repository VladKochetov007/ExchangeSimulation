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

// The direction of a funding transfer cannot be read off the sign of the
// total: reversing which side pays leaves the total unchanged. It can only be
// checked against each account's own side, so this is the test that a
// sign-reversed mechanism fails.
func TestFundingAuditCatchesAReversedSignAgainstReconstructedSides(t *testing.T) {
	const instant = int64(1_000_000_000)
	// Client 1 is long, client 2 is short, and the rate is positive, so the
	// long must be debited and the short credited.
	correct := []string{
		positionLine(instant-10, "north", 1, "ABC-PERP", auditPrecision, 0),
		positionLine(instant-10, "north", 2, "ABC-PERP", -auditPrecision, 0),
		fundingRateLine(instant-1, "north", 5),
		fundingPayLine(instant, "north", 1, -400),
		fundingPayLine(instant, "north", 2, 400),
	}
	dir := writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": correct})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureDerivativeSemantics(DerivativeAuditOptions{BasePrecision: auditPrecision})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if result.FundingSignWrong != 0 || result.FundingMisdirected != 0 {
		t.Fatalf("a correctly directed settlement was rejected: %+v", result.Funding)
	}
	if result.FundingUndirected != 0 {
		t.Fatalf("both sides were published, so neither is undirected: %+v", result.Funding)
	}
	if !result.Funding[0].LongsPaid {
		t.Errorf("the long was debited, so LongsPaid must be true: %+v", result.Funding)
	}

	// Same totals, same accounts, sides reversed: the ledger nets to zero and
	// the structural check still passes, which is exactly why it is not
	// enough.
	reversed := []string{
		positionLine(instant-10, "north", 1, "ABC-PERP", auditPrecision, 0),
		positionLine(instant-10, "north", 2, "ABC-PERP", -auditPrecision, 0),
		fundingRateLine(instant-1, "north", 5),
		fundingPayLine(instant, "north", 1, 400),
		fundingPayLine(instant, "north", 2, -400),
	}
	dir = writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": reversed})
	run, err = Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err = run.MeasureDerivativeSemantics(DerivativeAuditOptions{BasePrecision: auditPrecision})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if result.FundingMisdirected != 2 {
		t.Errorf("both accounts were charged against the rate: %+v", result.Funding)
	}
	if result.FundingSignWrong != 1 {
		t.Errorf("the instant must be reported as misdirected: %+v", result.Funding)
	}
	if result.FundingBroken != 0 {
		t.Errorf("the reversed settlement still nets to zero, so it is not unbalanced: %+v", result.Funding)
	}

	// An account whose position was never published is not evidence either
	// way, and must be counted as undirected rather than silently passing.
	unknown := []string{
		fundingRateLine(instant-1, "north", 5),
		fundingPayLine(instant, "north", 3, -400),
		fundingPayLine(instant, "north", 4, 400),
	}
	dir = writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": unknown})
	run, err = Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err = run.MeasureDerivativeSemantics(DerivativeAuditOptions{BasePrecision: auditPrecision})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if result.FundingUndirected != 2 || result.FundingMisdirected != 0 {
		t.Errorf("unpublished sides must be undirected, not passes: %+v", result.Funding)
	}
}
