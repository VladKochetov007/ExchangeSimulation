package analysis

import (
	"fmt"
	"strings"
	"testing"
)

func liquidationLine(ts int64, venue string, clientID uint64, debt int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"liquidation","data":{"venue_id":%q,"payload":{"symbol":"ABC-PERP","payload":{"symbol":"ABC-PERP","position_size":-100,"fill_price":5000,"remaining_debt":%d}}}}`, ts, clientID, venue, debt)
}

func liquidationSummaryLine(ts int64, venue string, clientID, liquidationID uint64, symbol, side string, positionSize, attemptedQty, filledQty, remainingQty, filledNotional, vwapPrice, fillPrice, debt int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"liquidation","data":{"venue_id":%q,"payload":{"symbol":%q,"payload":{"symbol":%q,"position_side":%q,"liquidation_id":%d,"position_size":%d,"attempted_qty":%d,"filled_qty":%d,"remaining_qty":%d,"filled_notional":%d,"vwap_price":%d,"fill_price":%d,"base_precision":1,"remaining_debt":%d}}}}`,
		ts, clientID, venue, symbol, symbol, side, liquidationID, positionSize, attemptedQty, filledQty, remainingQty, filledNotional, vwapPrice, fillPrice, debt)
}

func insuranceLine(ts int64, venue string, debt int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":0,"event":"insurance_fund","data":{"venue_id":%q,"payload":{"symbol":"ABC-PERP","payload":{"timestamp":%d,"symbol":"ABC-PERP","delta":-%d,"reason":"liquidation_deficit"}}}}`, ts, venue, ts, debt)
}

func liquidationPositionLine(ts int64, venue string, clientID uint64, oldSize, newSize int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"position_update","data":{"venue_id":%q,"payload":{"symbol":"ABC-PERP","payload":{"timestamp":%d,"client_id":%d,"symbol":"ABC-PERP","old_size":%d,"new_size":%d}}}}`,
		ts, clientID, venue, ts, clientID, oldSize, newSize)
}

func liquidationPositionSideLine(ts int64, venue string, clientID uint64, symbol, side string, oldSize, newSize int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"position_update","data":{"venue_id":%q,"payload":{"symbol":%q,"payload":{"timestamp":%d,"client_id":%d,"symbol":%q,"position_side":%q,"old_size":%d,"new_size":%d}}}}`,
		ts, clientID, venue, symbol, ts, clientID, symbol, side, oldSize, newSize)
}

func liquidationCheckLine(ts int64, venue string, clientID uint64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"liquidation_check","data":{"venue_id":%q,"payload":{"timestamp":%d,"client_id":%d,"symbol":"ABC-PERP"}}}`,
		ts, clientID, venue, ts, clientID)
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
		liquidationPositionLine(instant, "north", 8, 40, -20),
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
	if result.PositionPathRecords != 1 || result.PositionPathMissing != 0 || result.PositionPathFailures != 0 || result.PositionConservationRecords != 1 || result.PositionConservationMissing != 0 || result.PositionConservationFailures != 0 || result.PositionConservationResidual != 0 {
		t.Fatalf("valid forced-close path = %+v", result)
	}

	lines[1] = liquidationPositionLine(instant, "north", 8, 40, 20)
	dir = writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines})
	run, _ = Open(dir)
	result, err = run.MeasureLiquidations()
	if err != nil {
		t.Fatalf("measure invalid path: %v", err)
	}
	if result.PositionPathRecords != 1 || result.PositionConservationFailures != 1 || result.PositionConservationResidual != 40 {
		t.Fatalf("position conservation failure accepted: %+v", result)
	}

	lines[0] = liquidationPositionLine(instant, "north", 7, -100, -120)
	lines[1] = liquidationPositionLine(instant, "north", 8, 40, 60)
	dir = writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines})
	run, _ = Open(dir)
	result, err = run.MeasureLiquidations()
	if err != nil {
		t.Fatalf("measure nonreducing path: %v", err)
	}
	if result.PositionPathFailures != 1 || result.PositionConservationFailures != 0 {
		t.Fatalf("nonreducing close accepted: %+v", result)
	}
}

func TestLiquidationAuditRetainsSignedFillPriceAsPresent(t *testing.T) {
	line := liquidationLine(int64(1e9), "north", 7, 0)
	line = strings.Replace(line, `"fill_price":5000`, `"fill_price":0`, 1)
	dir := writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": {line}})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureLiquidations()
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if result.Liquidations != 1 || result.InvalidLiquidations != 0 || result.SignedOrZeroFillPrices != 1 {
		t.Fatalf("zero fill price must remain present evidence: %+v", result)
	}
}

func TestLiquidationAuditValidatesAccountScopedExecutionSummary(t *testing.T) {
	const instant = int64(1_000_000_000)
	lines := []string{
		liquidationSummaryLine(instant, "north", 7, 11, "A-PERP", "LONG", -10, 10, 4, 6, 200, 50, 50, 40),
		liquidationSummaryLine(instant, "north", 7, 11, "B-PERP", "LONG", 10, 10, 10, 0, 1500, 150, 150, 0),
		changeLine(instant, "north", 7, "A-PERP", "liquidation_deficit", [][3]any{{"USD", int64(-40), int64(40)}}),
		insuranceLine(instant, "north", 40),
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
	if result.ExecutionSummaryRecords != 2 || result.ExecutionSummaryFailures != 0 || result.DuplicateDeficitRecords != 0 {
		t.Fatalf("valid execution summary = %+v", result)
	}

	lines[1] = liquidationSummaryLine(instant, "north", 7, 11, "B-PERP", "LONG", 10, 10, 10, 0, 1500, 150, 150, 40)
	dir = writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines})
	run, _ = Open(dir)
	result, err = run.MeasureLiquidations()
	if err != nil {
		t.Fatalf("measure duplicate debt: %v", err)
	}
	if result.ExecutionSummaryFailures != 0 || result.DuplicateDeficitRecords != 1 || result.DeficitMismatchInstants == 0 {
		t.Fatalf("duplicate deficit attribution was accepted: %+v", result)
	}

	lines[1] = liquidationSummaryLine(instant, "north", 7, 11, "B-PERP", "LONG", 10, 10, 12, -2, 1500, 150, 150, 0)
	dir = writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines})
	run, _ = Open(dir)
	result, err = run.MeasureLiquidations()
	if err != nil {
		t.Fatalf("measure invalid quantities: %v", err)
	}
	if result.ExecutionSummaryFailures != 1 {
		t.Fatalf("invalid execution summary was accepted: %+v", result)
	}
}

func TestLiquidationAuditRejectsOrphanedReducingPositionBatch(t *testing.T) {
	const instant = int64(1_000_000_000)
	lines := []string{
		liquidationCheckLine(instant, "north", 7),
		liquidationPositionSideLine(instant, "north", 7, "ABC-PERP", "LONG", -100, -40),
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
	if result.PositionPathOrphaned != 1 {
		t.Fatalf("omitted liquidation receipt was not rejected: %+v", result)
	}
}

func TestLiquidationAuditBindsDeficitCreditToTheLiquidatedAccount(t *testing.T) {
	const instant = int64(1_000_000_000)
	lines := []string{
		liquidationLine(instant, "north", 7, 40),
		changeLine(instant, "north", 8, "ABC-PERP", "liquidation_deficit", [][3]any{{"USD", int64(-40), int64(40)}}),
		insuranceLine(instant, "north", 40),
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
	if result.DeficitMismatchInstants != 2 || result.DeficitBalanceMismatchAccounts != 2 {
		t.Fatalf("wrong-account deficit credit was accepted: %+v", result)
	}
}

func TestLiquidationAuditRejectsDuplicateExecutionReceiptAndVWAPMismatch(t *testing.T) {
	const instant = int64(1_000_000_000)
	valid := liquidationSummaryLine(instant, "north", 7, 11, "A-PERP", "LONG", -10, 10, 4, 6, 200, 50, 50, 0)
	duplicate := []string{valid, valid}
	dir := writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": duplicate})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureLiquidations()
	if err != nil {
		t.Fatalf("measure duplicate: %v", err)
	}
	if result.DuplicateExecutionSummaryRecords != 1 || result.ExecutionSummaryFailures != 1 {
		t.Fatalf("duplicate execution receipt was accepted: %+v", result)
	}

	mismatched := liquidationSummaryLine(instant, "north", 7, 12, "A-PERP", "LONG", -10, 10, 4, 6, 201, 50, 50, 0)
	dir = writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": {mismatched}})
	run, err = Open(dir)
	if err != nil {
		t.Fatalf("open mismatch: %v", err)
	}
	result, err = run.MeasureLiquidations()
	if err != nil {
		t.Fatalf("measure mismatch: %v", err)
	}
	if result.ExecutionSummaryFailures != 1 {
		t.Fatalf("inconsistent notional/VWAP summary was accepted: %+v", result)
	}
}
