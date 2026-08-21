package analysis

import (
	"fmt"
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
