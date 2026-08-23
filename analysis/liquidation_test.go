package analysis

import (
	"fmt"
	"testing"
)

func liquidationLine(ts int64, venue string, clientID uint64, debt int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"liquidation","data":{"venue_id":%q,"payload":{"symbol":"ABC-PERP","payload":{"symbol":"ABC-PERP","position_size":-100,"fill_price":5000,"remaining_debt":%d}}}}`, ts, clientID, venue, debt)
}

func insuranceLine(ts int64, venue string, debt int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":0,"event":"insurance_fund","data":{"venue_id":%q,"payload":{"symbol":"ABC-PERP","payload":{"timestamp":%d,"symbol":"ABC-PERP","delta":-%d,"reason":"liquidation_deficit"}}}}`, ts, venue, ts, debt)
}

func liquidationPositionLine(ts int64, venue string, clientID uint64, oldSize, newSize int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"position_update","data":{"venue_id":%q,"payload":{"symbol":"ABC-PERP","payload":{"timestamp":%d,"client_id":%d,"symbol":"ABC-PERP","old_size":%d,"new_size":%d}}}}`,
		ts, clientID, venue, ts, clientID, oldSize, newSize)
}

func TestLiquidationAuditReconcilesDeficitThreeWays(t *testing.T) {
	const instant = int64(1_000_000_000)
	good := []string{
		liquidationLine(instant, "north", 7, 40),
		changeLine(instant, "north", 7, "ABC-PERP", "liquidation_deficit", [][3]any{{"USD", int64(-40), int64(40)}}),
		insuranceLine(instant, "north", 40),
	}
	dir := writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": good})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureLiquidations()
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if result.Liquidations != 1 || result.AffectedAccounts != 1 || result.TotalDeficit != 40 || result.DeficitMismatchInstants != 0 || result.DeficitInsuranceResidual != 0 || result.DeficitBalanceResidual != 0 {
		t.Fatalf("good liquidation reconciliation = %+v", result)
	}

	bad := append([]string{}, good...)
	bad[2] = insuranceLine(instant, "north", 39)
	dir = writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": bad})
	run, _ = Open(dir)
	result, err = run.MeasureLiquidations()
	if err != nil {
		t.Fatalf("measure bad: %v", err)
	}
	if result.DeficitMismatchInstants != 1 || result.DeficitInsuranceResidual != 1 {
		t.Fatalf("mismatched insurance was accepted: %+v", result)
	}
}

func TestLiquidationAuditReconstructsForcedClosePositionPath(t *testing.T) {
	const instant = int64(1_000_000_000)
	lines := []string{
		liquidationPositionLine(instant, "north", 7, -100, -40),
		liquidationLine(instant, "north", 7, 0),
	}
	dir := writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureLiquidations()
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if result.PositionPathRecords != 1 || result.PositionPathMissing != 0 || result.PositionPathFailures != 0 {
		t.Fatalf("valid forced-close path = %+v", result)
	}

	lines[0] = liquidationPositionLine(instant, "north", 7, -100, -120)
	dir = writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines})
	run, _ = Open(dir)
	result, err = run.MeasureLiquidations()
	if err != nil {
		t.Fatalf("measure invalid path: %v", err)
	}
	if result.PositionPathRecords != 1 || result.PositionPathFailures != 1 {
		t.Fatalf("position increase accepted: %+v", result)
	}
}
