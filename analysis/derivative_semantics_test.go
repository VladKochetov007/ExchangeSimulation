package analysis

import (
	"fmt"
	"testing"
)

func fundingRateLine(ts int64, venue string, rate int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":0,"event":"funding_rate_update","data":{"venue_id":%q,"symbol":"ABC-PERP","payload":{"symbol":"ABC-PERP","payload":{"timestamp":%d,"symbol":"ABC-PERP","rate":%d}}}}`,
		ts, venue, ts, rate)
}

func fundingPayLine(ts int64, venue string, clientID uint64, delta int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"balance_change","data":{"venue_id":%q,"symbol":"ABC-PERP","payload":{"symbol":"ABC-PERP","payload":{"timestamp":%d,"client_id":%d,"symbol":"ABC-PERP","reason":"funding_settlement","changes":[{"asset":"USD","wallet":"perp","old_balance":0,"new_balance":%d,"delta":%d}]}}}}`,
		ts, clientID, venue, ts, clientID, delta, delta)
}

func fundingRateStrictLine(ts, nextFunding, interval int64, venue string, rate int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":0,"event":"funding_rate_update","data":{"venue_id":%q,"symbol":"ABC-PERP","payload":{"symbol":"ABC-PERP","payload":{"timestamp":%d,"symbol":"ABC-PERP","rate":%d,"next_funding":%d,"interval":%d}}}}`,
		ts, venue, ts, rate, nextFunding, interval)
}

func fundingSettlementLine(ts, nextFunding, interval int64, venue string, rate int64) string {
	return fundingSettlementLineWithTerms(ts, nextFunding, interval, venue, rate, auditPrecision, auditPrecision)
}

func fundingSettlementLineWithTerms(ts, nextFunding, interval int64, venue string, rate, markPrice, basePrecision int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":0,"event":"funding_settlement","data":{"venue_id":%q,"symbol":"ABC-PERP","payload":{"symbol":"ABC-PERP","payload":{"timestamp":%d,"symbol":"ABC-PERP","rate":%d,"next_funding":%d,"interval":%d,"mark_price":%d,"base_precision":%d}}}}`,
		ts, venue, ts, rate, nextFunding, interval, markPrice, basePrecision)
}

func optionSettledLine(ts int64, venue, symbol string, strike, settle int64, isCall bool) string {
	return optionSettledAtLine(ts, ts, venue, symbol, strike, settle, isCall)
}

func optionSettledAtLine(eventTS, expiry int64, venue, symbol string, strike, settle int64, isCall bool) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":0,"event":"instrument_settled","data":{"venue_id":%q,"payload":{"action":"settled","symbol":%q,"instrument_type":"OPTION","strike":%d,"is_call":%t,"settlement_price":%d,"expiry_nano":%d,"timestamp":%d}}}`,
		eventTS, venue, symbol, strike, isCall, settle, expiry, eventTS)
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

func TestFundingAuditAcceptsOneAggregateHedgeModePosting(t *testing.T) {
	const instant = int64(2_000_000_000)
	lines := []string{
		positionSideLine(instant-10, "north", 1, "ABC-PERP", 10, 0, "LONG"),
		positionSideLine(instant-10, "north", 1, "ABC-PERP", -4, 0, "SHORT"),
		positionSideLine(instant-10, "north", 2, "ABC-PERP", -6, 0, "SHORT"),
		fundingRateStrictLine(instant-1_000_000_000, instant, 1, "north", 5),
		// The source aggregates the two hedge legs into one client-level
		// settlement: the net position is long six contracts and pays 60. The
		// other account receives that same amount, closing the contract-level
		// transfer identity.
		fundingPayLine(instant, "north", 1, -60),
		fundingPayLine(instant, "north", 2, 60),
	}
	run, err := Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureDerivativeSemantics(DerivativeAuditOptions{
		BasePrecision: auditPrecision, RequireExactReplay: true, ExpectedFundingIntervalSeconds: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.FundingBroken != 0 || result.FundingSignWrong != 0 || result.FundingMisdirected != 0 ||
		result.FundingDuplicatePayments != 0 || result.FundingEvidenceFailures != 0 {
		t.Fatalf("aggregate hedge-mode funding was rejected: %+v", result)
	}
}

func TestFundingAuditDoesNotRequirePostingForNetZeroHedge(t *testing.T) {
	const instant = int64(2_500_000_000)
	lines := []string{
		positionSideLine(instant-10, "north", 1, "ABC-PERP", 10, 0, "LONG"),
		positionSideLine(instant-10, "north", 1, "ABC-PERP", -10, 0, "SHORT"),
		fundingRateStrictLine(instant-1_000_000_000, instant, 1, "north", 5),
		fundingSettlementLine(instant, instant+1_000_000_000, 1, "north", 5),
	}
	run, err := Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureDerivativeSemantics(DerivativeAuditOptions{
		BasePrecision: auditPrecision, RequireExactReplay: true, ExpectedFundingIntervalSeconds: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.FundingMissingSettlements != 0 || result.FundingTimingFailures != 0 {
		t.Fatalf("net-zero hedge was incorrectly required to post funding: %+v", result)
	}
}

func TestFundingAuditDoesNotCountPostRateSameTimestampPositionAsMissingSettlement(t *testing.T) {
	const instant = int64(3_500_000_000)
	postRate := []string{
		fundingRateStrictLine(instant, instant, 1, "north", 5),
		// This position is published after the deadline's rate observation and
		// cannot have been present for the settlement it names.
		positionLine(instant, "north", 1, "ABC-PERP", auditPrecision, 0),
	}
	run, err := Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": postRate}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureDerivativeSemantics(DerivativeAuditOptions{
		BasePrecision: auditPrecision, RequireExactReplay: true, ExpectedFundingIntervalSeconds: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.FundingMissingSettlements != 0 || result.FundingTimingFailures != 0 {
		t.Fatalf("post-rate position was treated as pre-settlement exposure: %+v", result)
	}

	preRate := []string{
		positionLine(instant, "north", 1, "ABC-PERP", auditPrecision, 0),
		fundingRateStrictLine(instant, instant, 1, "north", 5),
	}
	run, err = Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": preRate}))
	if err != nil {
		t.Fatal(err)
	}
	result, err = run.MeasureDerivativeSemantics(DerivativeAuditOptions{
		BasePrecision: auditPrecision, RequireExactReplay: true, ExpectedFundingIntervalSeconds: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.FundingMissingSettlements != 1 {
		t.Fatalf("pre-rate position did not require settlement evidence: %+v", result)
	}
}

func TestFundingAuditAcceptsZeroCashSettlementMarker(t *testing.T) {
	const instant = int64(4_000_000_000)
	lines := []string{
		positionLine(instant-10, "north", 1, "ABC-PERP", 1, 0),
		fundingRateStrictLine(instant-1_000_000_000, instant, 1, "north", 5),
		// The per-account fixed-point amount rounded to zero, so no balance
		// change exists. The operation marker is the required evidence that the
		// schedule nevertheless advanced.
		fundingSettlementLine(instant, instant+1_000_000_000, 1, "north", 5),
	}
	run, err := Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureDerivativeSemantics(DerivativeAuditOptions{
		BasePrecision: auditPrecision, RequireExactReplay: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Funding) != 1 || result.FundingMissingSettlements != 0 || result.FundingTimingFailures != 0 ||
		result.FundingBroken != 0 || result.FundingSignWrong != 0 || result.FundingEvidenceFailures != 0 {
		t.Fatalf("zero-cash settlement marker was not accepted: %+v", result)
	}
}

func TestFundingAuditRejectsMarkerThatOmitsNonzeroPayments(t *testing.T) {
	const instant = int64(5_000_000_000)
	lines := []string{
		positionLine(instant-10, "north", 1, "ABC-PERP", auditPrecision, 0),
		positionLine(instant-10, "north", 2, "ABC-PERP", -auditPrecision, 0),
		fundingRateStrictLine(instant-1_000_000_000, instant, 1, "north", 5000),
		// The marker proves the operation ran, but both account postings are
		// intentionally absent. A structural marker-only check must not pass.
		fundingSettlementLineWithTerms(instant, instant+1_000_000_000, 1, "north", 5000, auditPrecision, auditPrecision),
	}
	run, err := Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureDerivativeSemantics(DerivativeAuditOptions{
		BasePrecision: auditPrecision, RequireExactReplay: true, ExpectedFundingIntervalSeconds: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.FundingPaymentMismatches != 2 || result.FundingBroken != 1 || result.FundingSignWrong != 1 {
		t.Fatalf("marker-only missing payments were accepted: %+v", result)
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

func TestExerciseAuditRequiresDeclaredExpiryTiming(t *testing.T) {
	const expiry = int64(1_000_000_000)
	strike := 50_000 * auditPrecision
	settle := 55_000 * auditPrecision

	lateAnnouncement := []string{
		optionSettledAtLine(expiry+1, expiry, "north", "ABC-LATE-C", strike, settle, true),
		expiryPayLine(expiry, "north", 1, "ABC-LATE-C", settle-strike),
	}
	run, err := Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lateAnnouncement}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureDerivativeSemantics(DerivativeAuditOptions{BasePrecision: auditPrecision})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExerciseTimingFailures != 1 {
		// The payout is at the declared expiry; only the terminal announcement
		// is late, so this is one lifecycle failure, not two.
		t.Fatalf("late option announcement was not counted exactly once: %+v", result)
	}

	earlyPayout := []string{
		optionSettledLine(expiry, "north", "ABC-EARLY-C", strike, settle, true),
		expiryPayLine(expiry-1, "north", 1, "ABC-EARLY-C", settle-strike),
	}
	run, err = Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": earlyPayout}))
	if err != nil {
		t.Fatal(err)
	}
	result, err = run.MeasureDerivativeSemantics(DerivativeAuditOptions{BasePrecision: auditPrecision})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExerciseTimingFailures != 1 {
		t.Fatalf("early option payout was not counted exactly once: %+v", result)
	}
}

func TestExerciseAuditJoinsLifecycleAndOptionBookFilesByDefault(t *testing.T) {
	const expiry = int64(1_000_000_000)
	strike := int64(50_000) * auditPrecision
	settle := int64(55_000) * auditPrecision
	intrinsic := settle - strike
	run, err := Open(writeRun(t, Report{}, map[string][]string{
		"north/general.jsonl": {
			optionSettledLine(expiry, "north", "ABC-CROSS-FILE-C", strike, settle, true),
		},
		"north/spot/ABC-USD.jsonl": {
			optionFillForAudit(expiry-1, 1, "ABC-CROSS-FILE-C", "BUY", auditPrecision),
			optionFillForAudit(expiry-1, 2, "ABC-CROSS-FILE-C", "SELL", auditPrecision),
		},
		"north/derivatives.jsonl": {
			expiryPayLine(expiry, "north", 1, "ABC-CROSS-FILE-C", intrinsic),
			expiryPayLine(expiry, "north", 2, "ABC-CROSS-FILE-C", -intrinsic),
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureDerivativeSemantics(DerivativeAuditOptions{BasePrecision: auditPrecision})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Exercises) != 1 || result.ExerciseTimingFailures != 0 || result.ExerciseEvidenceFailures != 0 {
		t.Fatalf("cross-file option lifecycle was not joined: %+v", result)
	}
	check := result.Exercises[0]
	if check.Holders != 2 || check.PaidOut != 0 || check.Residual != 0 || check.HoldersMispaid != 0 || result.ExerciseBroken != 0 {
		t.Fatalf("cross-file option exercise = %+v", check)
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

// Funding and a fill can share a simulator timestamp at an expiry boundary.
// The funded side is the side before the funding record, not the side after a
// later fill in the same persisted derivative log.
func TestFundingAuditUsesPreFundingSameTimestampPosition(t *testing.T) {
	const instant = int64(1_000_000_000)
	lines := []string{
		// Negative funding charges the short and credits the long.
		positionLine(instant-1, "north", 1, "ABC-PERP", -auditPrecision, 0),
		positionLine(instant-1, "north", 2, "ABC-PERP", auditPrecision, 0),
		fundingRateLine(instant-1, "north", -5),
		fundingPayLine(instant, "north", 1, -400),
		fundingPayLine(instant, "north", 2, 400),
		// This trade is later in the same file and flips client 1 long. It must
		// not retroactively reverse the direction of the preceding funding.
		positionLine(instant, "north", 1, "ABC-PERP", auditPrecision, 0),
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
	if result.FundingMisdirected != 0 || result.FundingSignWrong != 0 {
		t.Fatalf("post-funding same-timestamp fill rewrote settlement side: %+v", result.Funding)
	}
}

// A second identical settlement keeps the funding pool balanced and directed,
// so a net-zero/sign-only check accepts it. The audit must also require one
// nonzero posting per funded account at each contract/instant.
func TestFundingAuditCatchesDuplicateAccountPayment(t *testing.T) {
	const instant = int64(1_000_000_000)
	lines := []string{
		positionLine(instant-10, "north", 1, "ABC-PERP", auditPrecision, 0),
		positionLine(instant-10, "north", 2, "ABC-PERP", -auditPrecision, 0),
		fundingRateLine(instant-1, "north", 5),
		fundingPayLine(instant, "north", 1, -400),
		fundingPayLine(instant, "north", 2, 400),
		fundingPayLine(instant, "north", 1, -400),
		fundingPayLine(instant, "north", 2, 400),
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
	if result.FundingBroken != 1 || result.FundingDuplicatePayments != 2 || result.Funding[0].DuplicatePayments != 2 {
		t.Fatalf("duplicate funding accepted: %+v", result)
	}
}

func TestStrictFundingAuditBindsRateScheduleAndPhysicalOrder(t *testing.T) {
	const instant = int64(3_000_000_000)
	const interval = int64(1)
	rateTimestamp := instant - interval*1_000_000_000
	good := []string{
		positionLine(instant-10, "north", 1, "ABC-PERP", auditPrecision, 0),
		positionLine(instant-10, "north", 2, "ABC-PERP", -auditPrecision, 0),
		fundingRateStrictLine(rateTimestamp, instant, interval, "north", 5),
		fundingPayLine(instant, "north", 1, -400),
		fundingPayLine(instant, "north", 2, 400),
	}
	run, err := Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": good}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureDerivativeSemantics(DerivativeAuditOptions{
		BasePrecision: auditPrecision, RequireExactReplay: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.FundingEvidenceFailures != 0 || result.FundingMissingRates != 0 || result.FundingTimingFailures != 0 ||
		result.FundingSignWrong != 0 || !result.Funding[0].RateAvailable || result.Funding[0].NextFunding != instant {
		t.Fatalf("valid scheduled funding was rejected: %+v", result)
	}

	backdated := []string{
		positionLine(instant-10, "north", 1, "ABC-PERP", auditPrecision, 0),
		positionLine(instant-10, "north", 2, "ABC-PERP", -auditPrecision, 0),
		fundingPayLine(instant, "north", 1, -400),
		fundingPayLine(instant, "north", 2, 400),
		// Timestamp alone would accept this announcement; its persisted order is
		// after the settlement it claims to explain.
		fundingRateStrictLine(rateTimestamp, instant, interval, "north", 5),
	}
	run, err = Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": backdated}))
	if err != nil {
		t.Fatal(err)
	}
	result, err = run.MeasureDerivativeSemantics(DerivativeAuditOptions{
		BasePrecision: auditPrecision, RequireExactReplay: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.FundingMissingRates != 1 || result.FundingTimingFailures != 1 || result.FundingSignWrong != 1 {
		t.Fatalf("backdated funding rate was accepted: %+v", result)
	}

	shifted := []string{
		positionLine(instant-10, "north", 1, "ABC-PERP", auditPrecision, 0),
		positionLine(instant-10, "north", 2, "ABC-PERP", -auditPrecision, 0),
		fundingRateStrictLine(rateTimestamp, instant+interval*1_000_000_000, interval, "north", 5),
		fundingPayLine(instant, "north", 1, -400),
		fundingPayLine(instant, "north", 2, 400),
	}
	run, err = Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": shifted}))
	if err != nil {
		t.Fatal(err)
	}
	result, err = run.MeasureDerivativeSemantics(DerivativeAuditOptions{
		BasePrecision: auditPrecision, RequireExactReplay: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.FundingMissingRates != 1 || result.FundingTimingFailures != 1 {
		t.Fatalf("shifted funding schedule was accepted: %+v", result)
	}
}

func TestStrictFundingAuditUsesLatestRateBeforeBoundarySettlement(t *testing.T) {
	const instant = int64(3_000_000_000)
	lines := []string{
		positionLine(instant-10, "north", 1, "ABC-PERP", auditPrecision, 0),
		positionLine(instant-10, "north", 2, "ABC-PERP", -auditPrecision, 0),
		// The exchange refreshes the rate at the settlement timestamp before
		// the settlement operation advances NextFunding. The current deadline is
		// therefore valid when the refresh physically precedes the payments.
		fundingRateStrictLine(instant, instant, 1, "north", 5),
		fundingPayLine(instant, "north", 1, -400),
		fundingPayLine(instant, "north", 2, 400),
	}
	run, err := Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureDerivativeSemantics(DerivativeAuditOptions{BasePrecision: auditPrecision, RequireExactReplay: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.FundingMissingRates != 0 || result.FundingTimingFailures != 0 || result.FundingSignWrong != 0 || !result.Funding[0].RateAvailable {
		t.Fatalf("boundary rate refresh was rejected: %+v", result)
	}
}

func TestStrictFundingAuditRejectsStaleDeadlineAtSettlement(t *testing.T) {
	const instant = int64(3_000_000_000)
	lines := []string{
		positionLine(instant-10, "north", 1, "ABC-PERP", auditPrecision, 0),
		positionLine(instant-10, "north", 2, "ABC-PERP", -auditPrecision, 0),
		// This publication is validly formed but names the prior deadline. It
		// must not be used to cover a settlement for the current deadline.
		fundingRateStrictLine(instant-2, instant-1, 1, "north", 5),
		fundingPayLine(instant, "north", 1, -400),
		fundingPayLine(instant, "north", 2, 400),
	}
	run, err := Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureDerivativeSemantics(DerivativeAuditOptions{BasePrecision: auditPrecision, RequireExactReplay: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.FundingMissingRates != 1 || result.FundingTimingFailures != 1 || result.FundingSignWrong != 1 {
		t.Fatalf("stale funding deadline was accepted: %+v", result)
	}
}

func TestStrictFundingAuditRejectsWrongAndIncompleteCadence(t *testing.T) {
	const intervalSeconds = int64(1)
	const firstDeadline = int64(3_000_000_000)
	base := []string{
		positionLine(firstDeadline-10, "north", 1, "ABC-PERP", auditPrecision, 0),
		positionLine(firstDeadline-10, "north", 2, "ABC-PERP", -auditPrecision, 0),
		fundingRateStrictLine(firstDeadline-intervalSeconds*1_000_000_000, firstDeadline, intervalSeconds, "north", 5),
		fundingPayLine(firstDeadline, "north", 1, -400),
		fundingPayLine(firstDeadline, "north", 2, 400),
	}
	run, err := Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": base}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureDerivativeSemantics(DerivativeAuditOptions{
		BasePrecision: auditPrecision, RequireExactReplay: true, ExpectedFundingIntervalSeconds: intervalSeconds,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.FundingTimingFailures != 0 {
		t.Fatalf("valid registered cadence was rejected: %+v", result)
	}

	wrongInterval := append([]string{}, base...)
	wrongInterval[2] = fundingRateStrictLine(firstDeadline-intervalSeconds*1_000_000_000, firstDeadline, 2, "north", 5)
	run, err = Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": wrongInterval}))
	if err != nil {
		t.Fatal(err)
	}
	result, err = run.MeasureDerivativeSemantics(DerivativeAuditOptions{
		BasePrecision: auditPrecision, RequireExactReplay: true, ExpectedFundingIntervalSeconds: intervalSeconds,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.FundingTimingFailures == 0 {
		t.Fatalf("arbitrary funding interval was accepted: %+v", result)
	}

	incomplete := append([]string{}, base[:2]...)
	incomplete = append(incomplete,
		fundingRateStrictLine(firstDeadline-intervalSeconds*1_000_000_000, firstDeadline, intervalSeconds, "north", 5),
		fundingRateStrictLine(firstDeadline, firstDeadline+3*1_000_000_000, intervalSeconds, "north", 5),
		base[3], base[4])
	run, err = Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": incomplete}))
	if err != nil {
		t.Fatal(err)
	}
	result, err = run.MeasureDerivativeSemantics(DerivativeAuditOptions{
		BasePrecision: auditPrecision, RequireExactReplay: true, ExpectedFundingIntervalSeconds: intervalSeconds,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.FundingTimingFailures == 0 {
		t.Fatalf("skipped funding deadline was accepted: %+v", result)
	}

	duplicateDeadline := append([]string{}, base[:2]...)
	duplicateDeadline = append(duplicateDeadline,
		fundingRateStrictLine(firstDeadline-intervalSeconds*1_000_000_000, firstDeadline, intervalSeconds, "north", 5),
		fundingRateStrictLine(firstDeadline-intervalSeconds*1_000_000_000/2, firstDeadline, intervalSeconds, "north", 6),
		base[3], base[4])
	run, err = Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": duplicateDeadline}))
	if err != nil {
		t.Fatal(err)
	}
	result, err = run.MeasureDerivativeSemantics(DerivativeAuditOptions{
		BasePrecision: auditPrecision, RequireExactReplay: true, ExpectedFundingIntervalSeconds: intervalSeconds,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.FundingTimingFailures != 0 || result.FundingMissingSettlements != 0 {
		t.Fatalf("repeated observation of one funding deadline was treated as a second settlement: %+v", result)
	}

	missingSettlement := append([]string{}, base[:2]...)
	missingSettlement = append(missingSettlement,
		fundingRateStrictLine(firstDeadline-intervalSeconds*1_000_000_000, firstDeadline, intervalSeconds, "north", 5),
		fundingRateStrictLine(firstDeadline, firstDeadline+intervalSeconds*1_000_000_000, intervalSeconds, "north", 5),
		base[3], base[4])
	run, err = Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": missingSettlement}))
	if err != nil {
		t.Fatal(err)
	}
	result, err = run.MeasureDerivativeSemantics(DerivativeAuditOptions{
		BasePrecision: auditPrecision, RequireExactReplay: true, ExpectedFundingIntervalSeconds: intervalSeconds,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.FundingMissingSettlements != 1 || result.FundingTimingFailures == 0 {
		t.Fatalf("published deadline without settlement was accepted: %+v", result)
	}
}

func TestStrictFundingAuditUsesPerVenueCadence(t *testing.T) {
	const northDeadline = int64(3_000_000_000)
	const southDeadline = int64(6_000_000_000)
	north := []string{
		positionLine(northDeadline-10, "north", 1, "ABC-PERP", auditPrecision, 0),
		positionLine(northDeadline-10, "north", 2, "ABC-PERP", -auditPrecision, 0),
		fundingRateStrictLine(2_000_000_000, northDeadline, 1, "north", 5),
		fundingPayLine(northDeadline, "north", 1, -400),
		fundingPayLine(northDeadline, "north", 2, 400),
	}
	south := []string{
		positionLine(southDeadline-10, "south", 3, "ABC-PERP", auditPrecision, 0),
		positionLine(southDeadline-10, "south", 4, "ABC-PERP", -auditPrecision, 0),
		fundingRateStrictLine(4_000_000_000, southDeadline, 2, "south", 5),
		fundingPayLine(southDeadline, "south", 3, -400),
		fundingPayLine(southDeadline, "south", 4, 400),
	}
	run, err := Open(writeRun(t, Report{}, map[string][]string{
		"north/derivatives.jsonl": north,
		"south/derivatives.jsonl": south,
	}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureDerivativeSemantics(DerivativeAuditOptions{
		BasePrecision: auditPrecision, RequireExactReplay: true,
		ExpectedFundingIntervals: map[string]int64{"north": 1, "south": 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.FundingTimingFailures != 0 || result.FundingMissingSettlements != 0 || result.FundingSignWrong != 0 || len(result.Funding) != 2 {
		t.Fatalf("heterogeneous venue cadence was not audited independently: %+v", result)
	}
}

func optionFillForAudit(ts int64, clientID uint64, symbol, side string, quantity int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"OrderFill","data":{"venue_id":"north","symbol":%q,"payload":{"symbol":%q,"order_id":%d,"qty":%d,"side":%q}}}`,
		ts, clientID, symbol, symbol, clientID+100, quantity, side)
}

func TestExerciseAuditRequiresExactHolderPaymentCardinality(t *testing.T) {
	const expiry = int64(1_000_000_000)
	strike := int64(50_000) * auditPrecision
	settle := int64(55_000) * auditPrecision
	intrinsic := settle - strike
	lines := []string{
		optionSettledLine(expiry, "north", "ABC-CARDINAL-C", strike, settle, true),
		optionFillForAudit(expiry-1, 1, "ABC-CARDINAL-C", "BUY", auditPrecision),
		optionFillForAudit(expiry-1, 2, "ABC-CARDINAL-C", "SELL", auditPrecision),
		expiryPayLine(expiry, "north", 1, "ABC-CARDINAL-C", intrinsic),
		expiryPayLine(expiry, "north", 1, "ABC-CARDINAL-C", intrinsic),
		expiryPayLine(expiry, "north", 2, "ABC-CARDINAL-C", -intrinsic),
		expiryPayLine(expiry, "north", 3, "ABC-CARDINAL-C", 1),
	}
	run, err := Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureDerivativeSemantics(DerivativeAuditOptions{BasePrecision: auditPrecision})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Exercises) != 1 {
		t.Fatalf("exercises = %d, want one", len(result.Exercises))
	}
	check := result.Exercises[0]
	if check.DuplicatePayouts != 1 || check.UnknownPayoutHolders != 1 || result.ExerciseBroken != 1 {
		t.Fatalf("duplicate/unknown option payments were accepted: %+v", result)
	}
}

func TestFillPositionAuditExcludesOptionProducerSchema(t *testing.T) {
	lines := []string{
		derivativePositionLine(10, 1, "north", "ABC-PERP", "BUY", 5, 0, 5),
		derivativeFillLine(10, 1, "north", "ABC-PERP", "BUY", 5, 5),
		// This is the option producer schema: it has no linear-only new_size or
		// price fields and therefore must not become a malformed linear fill.
		optionFillForAudit(10, 2, "ABC-OPT-C", "BUY", 5),
	}
	run, err := Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureFillPositions()
	if err != nil {
		t.Fatal(err)
	}
	if result.LinearFills != 1 || result.TradePositionUpdates != 1 || result.Matched != 1 || result.MalformedFillRecords != 0 || result.MalformedPositionUpdates != 0 {
		t.Fatalf("option producer schema contaminated linear audit: %+v", result)
	}
}

func TestStrictDerivativeAuditCountsMalformedEvidence(t *testing.T) {
	const expiry = int64(1_000_000_000)
	lines := []string{
		optionSettledLine(expiry, "north", "ABC-1-C", 50, 55, true),
		`{"sim_ts":1,"client_id":7,"event":"OrderFill","data":{"venue_id":"north","symbol":"ABC-1-C","payload":"broken"}}`,
		fundingRateLine(expiry-1, "north", 5),
		fundingPayLine(expiry, "north", 1, -400),
	}
	run, err := Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureDerivativeSemantics(DerivativeAuditOptions{
		BasePrecision: auditPrecision, RequireExactReplay: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExerciseEvidenceFailures == 0 || result.FundingEvidenceFailures == 0 {
		t.Fatalf("malformed derivative records disappeared: %+v", result)
	}
}

func TestStrictDerivativeAuditRejectsMissingRequiredFieldsAndOverflow(t *testing.T) {
	const expiry = int64(1_000_000_000)
	lines := []string{
		`{"sim_ts":1000000000,"client_id":0,"event":"instrument_settled","data":{"venue_id":"north","payload":{"action":"settled","symbol":"ABC-MISSING-C","instrument_type":"OPTION","strike":50,"settlement_price":55,"expiry_nano":1000000000,"timestamp":1000000000}}}`,
		`{"sim_ts":1,"client_id":1,"event":"OrderFill","data":{"venue_id":"north","symbol":"ABC-MISSING-C","payload":{"symbol":"ABC-MISSING-C","qty":1}}}`,
		`{"sim_ts":2,"client_id":1,"event":"balance_change","data":{"venue_id":"north","symbol":"ABC-MISSING-C","payload":{"timestamp":2,"client_id":1,"symbol":"ABC-MISSING-C","reason":"expiry_settlement","changes":[{"asset":"USD","wallet":"perp","old_balance":0,"new_balance":1}]}}}`,
	}
	run, err := Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureDerivativeSemantics(DerivativeAuditOptions{
		BasePrecision: auditPrecision, RequireExactReplay: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExerciseEvidenceFailures < 3 {
		t.Fatalf("missing strict fields disappeared: %+v", result)
	}

	overflow := []string{
		optionSettledLine(expiry, "north", "ABC-OVERFLOW-C", -9223372036854775807, 9223372036854775807, true),
	}
	run, err = Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": overflow}))
	if err != nil {
		t.Fatal(err)
	}
	result, err = run.MeasureDerivativeSemantics(DerivativeAuditOptions{BasePrecision: auditPrecision})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExerciseArithmeticFailures != 1 || len(result.Exercises) != 1 || !result.Exercises[0].Unrepresentable {
		t.Fatalf("intrinsic overflow was not fail-closed: %+v", result)
	}
}
