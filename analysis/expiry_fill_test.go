package analysis

import (
	"fmt"
	"testing"
)

func expiryInstrumentLine(event string, timestamp, expiry int64, venue, symbol, kind string) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":0,"event":%q,"data":{"venue_id":%q,"payload":{"symbol":%q,"instrument_type":%q,"expiry_nano":%d}}}`,
		timestamp, event, venue, symbol, kind, expiry)
}

func expirySettledLine(settled, expiry int64, venue, symbol, kind string) string {
	return expiryInstrumentLine("instrument_settled", settled, expiry, venue, symbol, kind)
}

func expiryFillLine(timestamp int64, venue, symbol string) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":7,"event":"OrderFill","data":{"venue_id":%q,"symbol":%q,"payload":{"symbol":%q,"qty":1}}}`,
		timestamp, venue, symbol, symbol)
}

func expirySnapshotLine(timestamp int64, venue, symbol string, bid, ask int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":0,"event":"BookSnapshot","data":{"venue_id":%q,"payload":{"symbol":%q,"payload":{"bids":[{"visible_qty":%d}],"asks":[{"visible_qty":%d}]}}}}`,
		timestamp, venue, symbol, bid, ask)
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
	if result.Contracts != 2 || result.Futures != 1 || result.Options != 1 || result.SettledContracts != 2 || result.FillRecords != 3 || result.FillsAfterExpiry != 2 || result.MissingExpiryMetadata != 0 {
		t.Fatalf("expiry fill audit = %+v", result)
	}
}

func TestExpiryFillAuditCatchesExpiredContractWithoutSettlement(t *testing.T) {
	lines := []string{
		expiryInstrumentLine("instrument_listed", 1, 100, "north", "ABC-FUT-1", "FUTURE"),
		expiryFillLine(101, "north", "ABC-FUT-1"),
		expirySnapshotLine(101, "north", "ABC-FUT-1", 1, 1),
	}
	report := Report{TerminalAccounts: []AccountRow{{Account: Account{Timestamp: 101}}}}
	run, err := Open(writeRun(t, report, map[string][]string{"north/general.jsonl": lines}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureExpiryFills()
	if err != nil {
		t.Fatal(err)
	}
	if result.ExpiredContracts != 1 || result.ExpiredUnsettledContracts != 1 ||
		result.FillsAfterExpiry != 1 || result.SnapshotRecordsAfterExpiry != 1 ||
		result.NonEmptySnapshotsAfterExpiry != 1 || len(result.Checks) != 1 ||
		result.Checks[0].Settled {
		t.Fatalf("expired unsettled contract survived: %+v", result)
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
