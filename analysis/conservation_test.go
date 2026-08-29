package analysis

import (
	"fmt"
	"math"
	"testing"
)

func changeLine(ts int64, venue string, clientID uint64, symbol, reason string, deltas [][3]any) string {
	changes := ""
	for i, d := range deltas {
		if i > 0 {
			changes += ","
		}
		asset := d[0].(string)
		old := d[1].(int64)
		delta := d[2].(int64)
		changes += fmt.Sprintf(`{"asset":%q,"wallet":"perp","old_balance":%d,"new_balance":%d,"delta":%d}`,
			asset, old, old+delta, delta)
	}
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"balance_change","data":{"venue_id":%q,"payload":{"symbol":%q,"payload":{"timestamp":%d,"client_id":%d,"symbol":%q,"reason":%q,"changes":[%s]}}}}`,
		ts, clientID, venue, symbol, ts, clientID, symbol, reason, changes)
}

// Funding is a transfer between longs and shorts, so an instant that does not
// net to zero is value appearing from nowhere. The audit exists to catch that
// and must catch it in the direction it actually happens.
func TestConservationFindsAFundingInstantThatDoesNotNet(t *testing.T) {
	const instant = int64(1_000_000_000)
	balanced := []string{
		changeLine(instant, "north", 1, "ABC-PERP", "funding_settlement", [][3]any{{"USD", int64(1000), int64(-40)}}),
		changeLine(instant, "north", 2, "ABC-PERP", "funding_settlement", [][3]any{{"USD", int64(1000), int64(40)}}),
	}
	dir := writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": balanced})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureConservation(ConservationOptions{})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if len(result.FundingInstants) != 1 || result.FundingInstants[0].Net != 0 {
		t.Fatalf("a balanced funding instant reported %+v", result.FundingInstants)
	}

	broken := append([]string{}, balanced...)
	broken[1] = changeLine(instant, "north", 2, "ABC-PERP", "funding_settlement", [][3]any{{"USD", int64(1000), int64(41)}})
	dir = writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": broken})
	run, err = Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err = run.MeasureConservation(ConservationOptions{})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	worst, ok := WorstResidual(result.FundingInstants)
	if !ok || worst.Net != 1 {
		t.Errorf("worst funding residual = %+v, want a net of 1", worst)
	}
}

// A balance change that reports moving one amount while its own before and
// after differ by another is a lie the whole audit rests on, so it is checked
// separately from any conservation identity.
func TestConservationCatchesASelfInconsistentDelta(t *testing.T) {
	line := `{"sim_ts":1,"client_id":1,"event":"balance_change","data":{"venue_id":"north","payload":{"symbol":"ABC/USD","payload":{"timestamp":1,"client_id":1,"symbol":"ABC/USD","reason":"trade_settlement","changes":[{"asset":"USD","wallet":"spot","old_balance":1000,"new_balance":900,"delta":-50}]}}}}`
	dir := writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": {line}})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureConservation(ConservationOptions{})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if result.Deltas.Mismatched != 1 || result.Deltas.WorstGap != 50 {
		t.Errorf("delta consistency = %+v, want one mismatch of 50", result.Deltas)
	}
}

func TestConservationReconcilesVenueStreamsAndChecksVenueArithmetic(t *testing.T) {
	report := Report{VenueLedgers: []VenueLedger{{
		VenueID:       "north",
		FeeRevenue:    map[string]int64{"USD": 10},
		InsuranceFund: map[string]int64{"USD": -3},
	}}}
	valid := []string{
		logLine(1, 0, "venue_balance_change", map[string]any{
			"timestamp": 1, "bucket": "fee_revenue", "asset": "USD", "reason": "taker_fee",
			"old_balance": 0, "new_balance": 7, "delta": 7,
		}),
		logLine(1, 0, "fee_revenue", map[string]any{
			"asset": "USD", "taker_fee": 10, "maker_fee": 0,
		}),
	}
	run, err := Open(writeRun(t, report, map[string][]string{"north/derivatives.jsonl": valid}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureConservation(ConservationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deltas.VenueBalanceMismatches != 0 || result.Deltas.FeeRevenueMismatches != 0 || result.Deltas.MalformedVenueRecords != 0 {
		t.Fatalf("consistent venue streams were rejected: %+v", result.Deltas)
	}

	invalid := append([]string{}, valid...)
	invalid[0] = logLine(1, 0, "venue_balance_change", map[string]any{
		"timestamp": 1, "bucket": "fee_revenue", "asset": "USD", "reason": "taker_fee",
		"old_balance": 0, "new_balance": 8, "delta": 7,
	})
	run, err = Open(writeRun(t, report, map[string][]string{"north/derivatives.jsonl": invalid}))
	if err != nil {
		t.Fatal(err)
	}
	result, err = run.MeasureConservation(ConservationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deltas.MalformedVenueRecords != 1 || result.Deltas.VenueBalanceMismatches != 1 {
		t.Fatalf("venue old/new/delta inconsistency was accepted: %+v", result.Deltas)
	}
}

func TestConservationChecksMovementOnlyParticipants(t *testing.T) {
	run, err := Open(writeRun(t, Report{}, map[string][]string{
		"north/derivatives.jsonl": {changeLine(1, "north", 7, "ABC-PERP", "trade_settlement", [][3]any{{"USD", int64(0), int64(100)}})},
	}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureConservation(ConservationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deltas.ChainChecked != 1 || result.Deltas.ChainBroken != 1 {
		t.Fatalf("participant missing from terminal report was accepted: %+v", result.Deltas)
	}
}

func TestConservationPropagatesIdentityArithmeticFailures(t *testing.T) {
	const maximum = int64(math.MaxInt64)
	report := Report{
		TerminalAccounts: []AccountRow{{
			VenueID: "north", ClientID: 1,
			Account: Account{SpotBalances: []Balance{{Asset: "USD", NetAsset: maximum}}},
		}},
		VenueLedgers: []VenueLedger{{VenueID: "north", FeeRevenue: map[string]int64{"USD": 1}}},
	}
	run, err := Open(writeRun(t, report, map[string][]string{
		"north/derivatives.jsonl": {changeLine(1, "north", 1, "ABC-PERP", "trade_settlement", [][3]any{{"USD", int64(0), maximum}})},
	}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureConservation(ConservationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deltas.ArithmeticFailures == 0 {
		t.Fatalf("identity overflow disappeared from returned deltas: %+v", result.Deltas)
	}
}

// Reasons must be separated: a deposit creates value legitimately and a trade
// must not, so pooling them would hide exactly the error being hunted.
func TestConservationSeparatesExternalInflowFromTransfers(t *testing.T) {
	lines := []string{
		changeLine(1, "north", 1, "", "initial_deposit", [][3]any{{"USD", int64(0), int64(1_000_000)}}),
		changeLine(2, "north", 1, "ABC/USD", "trade_settlement", [][3]any{{"USD", int64(1_000_000), int64(-500)}}),
		changeLine(2, "north", 2, "ABC/USD", "trade_settlement", [][3]any{{"USD", int64(0), int64(500)}}),
	}
	dir := writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureConservation(ConservationOptions{})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	byReason := map[string]AssetFlow{}
	for _, flow := range result.Flows {
		byReason[flow.Reason] = flow
	}
	if byReason["trade_settlement"].Net != 0 {
		t.Errorf("trades netted %d, want 0", byReason["trade_settlement"].Net)
	}
	if byReason["initial_deposit"].Net != 1_000_000 {
		t.Errorf("deposit netted %d, want the whole deposit", byReason["initial_deposit"].Net)
	}
	if result.PerVenueNet["north"]["USD"] != 1_000_000 {
		t.Errorf("venue net = %d, want the deposit alone", result.PerVenueNet["north"]["USD"])
	}
}

func TestConservationAuditsPositionRoundingLinksAndRemainder(t *testing.T) {
	dir := writeRun(t, Report{}, map[string][]string{
		"north/derivatives.jsonl": {
			`{"sim_ts":1,"client_id":1,"event":"position_rounding","data":{"venue_id":"north","payload":{"timestamp":1,"client_id":1,"symbol":"ABC-FUT","asset":"USD","cash_adjustment":3,"remainder_numerator":-2,"precision":10}}}`,
			`{"sim_ts":1,"client_id":1,"event":"balance_change","data":{"venue_id":"north","payload":{"timestamp":1,"client_id":1,"symbol":"ABC-FUT","reason":"position_rounding","changes":[{"asset":"USD","wallet":"perp","old_balance":0,"new_balance":3,"delta":3}]}}}`,
			`{"sim_ts":1,"client_id":0,"event":"venue_balance_change","data":{"venue_id":"north","payload":{"timestamp":1,"bucket":"fee_revenue","asset":"USD","symbol":"ABC-FUT","reason":"position_rounding","old_balance":0,"new_balance":-3,"delta":-3}}}`,
		},
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	conservation, err := run.MeasureConservation(ConservationOptions{})
	if err != nil {
		t.Fatalf("MeasureConservation: %v", err)
	}
	audit := conservation.PositionRounding
	if !audit.Valid || audit.Events != 1 || audit.UniqueTerminalKeys != 1 || audit.RemainderOutOfRange != 0 || audit.BalanceLinkFailures != 0 || audit.VenueLinkFailures != 0 {
		t.Fatalf("rounding audit = %+v, want one valid linked event", audit)
	}
}

func TestConservationRejectsUnboundedPositionRoundingRemainder(t *testing.T) {
	dir := writeRun(t, Report{}, map[string][]string{
		"north/derivatives.jsonl": {
			`{"sim_ts":1,"client_id":1,"event":"position_rounding","data":{"venue_id":"north","payload":{"timestamp":1,"client_id":1,"symbol":"ABC-FUT","asset":"USD","cash_adjustment":0,"remainder_numerator":10,"precision":10}}}`,
		},
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	conservation, err := run.MeasureConservation(ConservationOptions{})
	if err != nil {
		t.Fatalf("MeasureConservation: %v", err)
	}
	if conservation.PositionRounding.Valid || conservation.PositionRounding.Events != 1 || conservation.PositionRounding.RemainderOutOfRange != 1 {
		t.Fatalf("unbounded remainder audit = %+v, want invalid", conservation.PositionRounding)
	}
}

func TestConservationRejectsPositionRoundingLinkViolations(t *testing.T) {
	baseEvent := `{"sim_ts":1,"client_id":1,"event":"position_rounding","data":{"venue_id":"north","payload":{"timestamp":1,"client_id":1,"symbol":"ABC-FUT","asset":"USD","cash_adjustment":3,"remainder_numerator":-2,"precision":10}}}`
	measure := func(t *testing.T, lines []string) PositionRoundingAudit {
		t.Helper()
		run, err := Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines}))
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		conservation, err := run.MeasureConservation(ConservationOptions{})
		if err != nil {
			t.Fatalf("MeasureConservation: %v", err)
		}
		return conservation.PositionRounding
	}

	missing := measure(t, []string{baseEvent})
	if missing.Valid || missing.BalanceLinkFailures == 0 || missing.VenueLinkFailures == 0 {
		t.Fatalf("missing links audit = %+v, want both link failures", missing)
	}

	wrongWallet := measure(t, []string{
		baseEvent,
		`{"sim_ts":1,"client_id":1,"event":"balance_change","data":{"venue_id":"north","payload":{"timestamp":1,"client_id":1,"symbol":"ABC-FUT","reason":"position_rounding","changes":[{"asset":"USD","wallet":"spot","old_balance":0,"new_balance":3,"delta":3}]}}}`,
		`{"sim_ts":1,"client_id":0,"event":"venue_balance_change","data":{"venue_id":"north","payload":{"timestamp":1,"bucket":"fee_revenue","asset":"USD","symbol":"ABC-FUT","reason":"position_rounding","old_balance":0,"new_balance":-3,"delta":-3}}}`,
	})
	if wrongWallet.Valid || wrongWallet.AssetWalletFailures == 0 {
		t.Fatalf("wrong wallet audit = %+v, want invalid asset/wallet link", wrongWallet)
	}

	wrongAsset := measure(t, []string{
		baseEvent,
		`{"sim_ts":1,"client_id":1,"event":"balance_change","data":{"venue_id":"north","payload":{"timestamp":1,"client_id":1,"symbol":"ABC-FUT","reason":"position_rounding","changes":[{"asset":"ABC","wallet":"perp","old_balance":0,"new_balance":3,"delta":3}]}}}`,
		`{"sim_ts":1,"client_id":0,"event":"venue_balance_change","data":{"venue_id":"north","payload":{"timestamp":1,"bucket":"fee_revenue","asset":"USD","symbol":"ABC-FUT","reason":"position_rounding","old_balance":0,"new_balance":-3,"delta":-3}}}`,
	})
	if wrongAsset.Valid || wrongAsset.BalanceLinkFailures == 0 {
		t.Fatalf("wrong asset audit = %+v, want invalid denomination link", wrongAsset)
	}

	wrongBucket := measure(t, []string{
		baseEvent,
		`{"sim_ts":1,"client_id":1,"event":"balance_change","data":{"venue_id":"north","payload":{"timestamp":1,"client_id":1,"symbol":"ABC-FUT","reason":"position_rounding","changes":[{"asset":"USD","wallet":"perp","old_balance":0,"new_balance":3,"delta":3}]}}}`,
		`{"sim_ts":1,"client_id":0,"event":"venue_balance_change","data":{"venue_id":"north","payload":{"timestamp":1,"bucket":"insurance_fund","asset":"USD","symbol":"ABC-FUT","reason":"position_rounding","old_balance":0,"new_balance":-3,"delta":-3}}}`,
	})
	if wrongBucket.Valid || wrongBucket.VenueBucketFailures == 0 {
		t.Fatalf("wrong bucket audit = %+v, want invalid venue bucket link", wrongBucket)
	}
}

func TestConservationDistinguishesRoundingRelistingsAndOverflow(t *testing.T) {
	relisting := func(timestamp int64) string {
		return `{"sim_ts":` + itoa(timestamp) + `,"client_id":1,"event":"position_rounding","data":{"venue_id":"north","payload":{"timestamp":` + itoa(timestamp) + `,"client_id":1,"symbol":"ABC-FUT","asset":"USD","cash_adjustment":0,"remainder_numerator":1,"precision":10}}}`
	}
	run, err := Open(writeRun(t, Report{}, map[string][]string{
		"north/derivatives.jsonl": {relisting(1), relisting(2)},
	}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	conservation, err := run.MeasureConservation(ConservationOptions{})
	if err != nil {
		t.Fatalf("MeasureConservation: %v", err)
	}
	if !conservation.PositionRounding.Valid || conservation.PositionRounding.UniqueTerminalKeys != 2 || conservation.PositionRounding.DuplicateTerminalKeys != 0 {
		t.Fatalf("relisting audit = %+v, want two distinct terminal events", conservation.PositionRounding)
	}
	run, err = Open(writeRun(t, Report{}, map[string][]string{
		"north/derivatives.jsonl": {relisting(1), relisting(1)},
	}))
	if err != nil {
		t.Fatalf("Open duplicate: %v", err)
	}
	conservation, err = run.MeasureConservation(ConservationOptions{})
	if err != nil {
		t.Fatalf("MeasureConservation duplicate: %v", err)
	}
	if conservation.PositionRounding.Valid || conservation.PositionRounding.DuplicateTerminalKeys != 1 {
		t.Fatalf("duplicate audit = %+v, want invalid duplicate", conservation.PositionRounding)
	}

	overflowEvent := func(clientID int64) string {
		return `{"sim_ts":1,"client_id":` + itoa(clientID) + `,"event":"position_rounding","data":{"venue_id":"north","payload":{"timestamp":1,"client_id":` + itoa(clientID) + `,"symbol":"ABC-FUT","asset":"USD","cash_adjustment":0,"remainder_numerator":9223372036854775806,"precision":9223372036854775807}}}`
	}
	run, err = Open(writeRun(t, Report{}, map[string][]string{
		"north/derivatives.jsonl": {overflowEvent(1), overflowEvent(2)},
	}))
	if err != nil {
		t.Fatalf("Open overflow: %v", err)
	}
	conservation, err = run.MeasureConservation(ConservationOptions{})
	if err != nil {
		t.Fatalf("MeasureConservation overflow: %v", err)
	}
	if conservation.PositionRounding.Valid || !conservation.PositionRounding.AggregateRemainderOverflow {
		t.Fatalf("overflow audit = %+v, want invalid aggregate overflow", conservation.PositionRounding)
	}
}

func TestConservationCountsMalformedVenueAndFeeRecords(t *testing.T) {
	dir := writeRun(t, Report{}, map[string][]string{
		"north/derivatives.jsonl": {
			`{"sim_ts":1,"client_id":0,"event":"venue_balance_change","data":{"venue_id":"north","payload":{"asset":"USD","delta":1}}}`,
			`{"sim_ts":2,"client_id":0,"event":"fee_revenue","data":{"venue_id":"north","payload":{"asset":"USD","taker_fee":1}}}`,
		},
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureConservation(ConservationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deltas.MalformedVenueRecords != 1 || result.Deltas.MalformedFeeRecords != 1 {
		t.Fatalf("malformed venue/fee evidence disappeared: %+v", result.Deltas)
	}
}

func TestConservationRejectsAggregateIntegerOverflow(t *testing.T) {
	dir := writeRun(t, Report{}, map[string][]string{
		"north/spot/ABC-USD.jsonl": {
			`{"sim_ts":1,"client_id":1,"event":"balance_change","data":{"venue_id":"north","payload":{"timestamp":1,"client_id":1,"reason":"trade_settlement","changes":[{"asset":"USD","wallet":"spot","old_balance":0,"new_balance":9223372036854775807,"delta":9223372036854775807}]}}}`,
			`{"sim_ts":2,"client_id":1,"event":"balance_change","data":{"venue_id":"north","payload":{"timestamp":2,"client_id":1,"reason":"trade_settlement","changes":[{"asset":"USD","wallet":"spot","old_balance":0,"new_balance":1,"delta":1}]}}}`,
		},
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureConservation(ConservationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deltas.ArithmeticFailures == 0 {
		t.Fatalf("aggregate overflow was silently wrapped: %+v", result.Deltas)
	}
}
