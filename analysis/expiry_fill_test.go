package analysis

import (
	"fmt"
	"testing"
)

func expirySettledLine(settled, expiry int64, venue, symbol, kind string) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":0,"event":"instrument_settled","data":{"venue_id":%q,"payload":{"symbol":%q,"instrument_type":%q,"expiry_nano":%d}}}`,
		settled, venue, symbol, kind, expiry)
}

func expiryFillLine(timestamp int64, venue, symbol string) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":7,"event":"OrderFill","data":{"venue_id":%q,"symbol":%q,"payload":{"symbol":%q,"qty":1}}}`,
		timestamp, venue, symbol, symbol)
}

func TestExpiryFillAuditUsesContractualExpiryForFuturesAndOptions(t *testing.T) {
	lines := []string{
		expirySettledLine(100, 100, "north", "ABC-FUT-1", "FUTURE"),
		expirySettledLine(100, 100, "north", "ABC-1-C", "OPTION"),
		expiryFillLine(100, "north", "ABC-FUT-1"),
		expiryFillLine(101, "north", "ABC-FUT-1"),
		expiryFillLine(102, "north", "ABC-1-C"),
		expiryFillLine(999, "north", "ABC-PERP"),
	}
	run, err := Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureExpiryFills()
	if err != nil {
		t.Fatal(err)
	}
	if result.Contracts != 2 || result.Futures != 1 || result.Options != 1 || result.FillRecords != 3 || result.FillsAfterExpiry != 2 || result.MissingExpiryMetadata != 0 {
		t.Fatalf("expiry fill audit = %+v", result)
	}
}

func TestExpiryFillAuditReportsMissingContractExpiry(t *testing.T) {
	lines := []string{expirySettledLine(100, 0, "north", "ABC-FUT-1", "FUTURE")}
	run, err := Open(writeRun(t, Report{}, map[string][]string{"north/general.jsonl": lines}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureExpiryFills()
	if err != nil {
		t.Fatal(err)
	}
	if result.Contracts != 0 || result.MissingExpiryMetadata != 1 {
		t.Fatalf("missing expiry metadata = %+v", result)
	}
}
