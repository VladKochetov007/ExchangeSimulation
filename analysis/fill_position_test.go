package analysis

import (
	"fmt"
	"testing"
)

func derivativeFillLine(ts int64, client uint64, venue, symbol, side string, qty, newSize int64, prices ...int64) string {
	price := int64(100)
	if len(prices) > 0 {
		price = prices[0]
	}
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"OrderFill","data":{"venue_id":%q,"payload":{"symbol":%q,"qty":%d,"side":%q,"new_size":%d,"price":%d}}}`,
		ts, client, venue, symbol, qty, side, newSize, price)
}

func derivativePositionLine(ts int64, client uint64, venue, symbol, side string, qty, oldSize, newSize int64, prices ...int64) string {
	price := int64(100)
	if len(prices) > 0 {
		price = prices[0]
	}
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"position_update","data":{"venue_id":%q,"payload":{"symbol":%q,"payload":{"symbol":%q,"old_size":%d,"new_size":%d,"trade_qty":%d,"trade_price":%d,"trade_side":%q,"reason":"trade"}}}}`,
		ts, client, venue, symbol, symbol, oldSize, newSize, qty, price, side)
}

func TestFillPositionAuditPairsDerivativeOutcomes(t *testing.T) {
	lines := []string{
		derivativePositionLine(10, 1, "north", "ABC-PERP", "BUY", 5, 0, 5),
		derivativePositionLine(10, 2, "north", "ABC-PERP", "SELL", 5, 0, -5),
		derivativeFillLine(10, 1, "north", "ABC-PERP", "BUY", 5, 5),
		derivativeFillLine(10, 2, "north", "ABC-PERP", "SELL", 5, -5),
	}
	run, err := Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureFillPositions()
	if err != nil {
		t.Fatal(err)
	}
	if result.LinearFills != 2 || result.TradePositionUpdates != 2 || result.Matched != 2 || result.MissingPositionUpdate != 0 || result.UnexpectedPositionUpdate != 0 || result.PositionChainFailures != 0 {
		t.Fatalf("valid fill-position pairing = %+v", result)
	}
}

func TestFillPositionAuditCatchesExtraOrMissingSettlement(t *testing.T) {
	base := []string{
		derivativePositionLine(10, 1, "north", "ABC-PERP", "BUY", 5, 0, 5),
		derivativeFillLine(10, 1, "north", "ABC-PERP", "BUY", 5, 5),
	}
	for _, test := range []struct {
		name                   string
		lines                  []string
		wantMissing, wantExtra int
	}{
		{"duplicate settlement", append(base, derivativePositionLine(10, 1, "north", "ABC-PERP", "BUY", 5, 5, 10)), 0, 1},
		{"dropped settlement", base[1:], 1, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			run, err := Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": test.lines}))
			if err != nil {
				t.Fatal(err)
			}
			result, err := run.MeasureFillPositions()
			if err != nil {
				t.Fatal(err)
			}
			if result.MissingPositionUpdate != test.wantMissing || result.UnexpectedPositionUpdate != test.wantExtra {
				t.Fatalf("fill-position mismatch = %+v", result)
			}
		})
	}
}

func TestFillPositionAuditChecksPhysicalPositionChain(t *testing.T) {
	lines := []string{
		derivativePositionLine(10, 1, "north", "ABC-PERP", "BUY", 5, 0, 5),
		derivativeFillLine(10, 1, "north", "ABC-PERP", "BUY", 5, 5),
		derivativePositionLine(20, 1, "north", "ABC-PERP", "BUY", 5, 99, 10),
		derivativeFillLine(20, 1, "north", "ABC-PERP", "BUY", 5, 10),
	}
	run, err := Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureFillPositions()
	if err != nil {
		t.Fatal(err)
	}
	if result.PositionChainChecks != 1 || result.PositionChainFailures != 1 {
		t.Fatalf("position-chain mutation survived: %+v", result)
	}
}

func TestFillPositionAuditBindsExecutionPriceToPositionUpdate(t *testing.T) {
	lines := []string{
		derivativePositionLine(10, 1, "north", "ABC-PERP", "BUY", 5, 0, 5, 100),
		derivativeFillLine(10, 1, "north", "ABC-PERP", "BUY", 5, 5, 101),
	}
	run, err := Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureFillPositions()
	if err != nil {
		t.Fatal(err)
	}
	if result.PriceMismatches != 1 || result.Matched != 0 || result.MissingPositionUpdate != 1 || result.UnexpectedPositionUpdate != 1 {
		t.Fatalf("execution/position price mutation was not rejected: %+v", result)
	}
}
