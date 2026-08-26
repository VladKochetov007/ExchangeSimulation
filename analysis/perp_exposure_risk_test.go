package analysis

import (
	"fmt"
	"testing"
)

func p7dRiskPositionLine(ts int64, client uint64, venue string, oldSize, oldEntry, newSize, newEntry int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"position_update","data":{"venue_id":%q,"payload":{"symbol":"ABC-PERP","payload":{"timestamp":%d,"client_id":%d,"symbol":"ABC-PERP","old_size":%d,"old_entry_price":%d,"new_size":%d,"new_entry_price":%d}}}}`, ts, client, venue, ts, client, oldSize, oldEntry, newSize, newEntry)
}

func p7dRiskMarkLine(ts int64, venue string, mark int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":0,"event":"mark_price_update","data":{"venue_id":%q,"payload":{"symbol":"ABC-PERP","payload":{"timestamp":%d,"symbol":"ABC-PERP","mark_price":%d,"index_price":%d}}}}`, ts, venue, ts, mark, mark)
}

func p7dRiskBalanceLine(ts int64, client uint64, venue string, reason string, changes string) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"balance_change","data":{"venue_id":%q,"payload":{"timestamp":%d,"client_id":%d,"symbol":"ABC-PERP","reason":%q,"changes":[%s]}}}`, ts, client, venue, ts, client, reason, changes)
}

func p7dRiskBorrowLine(ts int64, client uint64, venue string, amount int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"borrow","data":{"venue_id":%q,"payload":{"timestamp":%d,"client_id":%d,"asset":"USD","amount":%d,"reason":"auto_perp"}}}`, ts, client, venue, ts, client, amount)
}

func p7dRiskCheckLine(ts int64, client uint64, venue string, mark, balance, contribution, equity, notional, maintenance int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"liquidation_check","data":{"venue_id":%q,"payload":{"timestamp":%d,"client_id":%d,"symbol":"ABC-PERP","mark_price":%d,"balance":%d,"derivative_equity_contribution":%d,"equity":%d,"notional":%d,"maintenance_margin":%d}}}`, ts, client, venue, ts, client, mark, balance, contribution, equity, notional, maintenance)
}

func p7dRiskLiquidationLine(ts int64, client uint64, venue string, position, fill, debt int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"liquidation","data":{"venue_id":%q,"payload":{"symbol":"ABC-PERP","payload":{"timestamp":%d,"symbol":"ABC-PERP","position_size":%d,"fill_price":%d,"remaining_debt":%d}}}}`, ts, client, venue, ts, position, fill, debt)
}

func p7dRiskRun(t *testing.T, derivative, general []string, initial, terminal Account) *Run {
	t.Helper()
	report := Report{
		InitialAccounts:  []AccountRow{{VenueID: "north", ClientID: 7, Role: "perp_exposure_hedger_1", Account: initial}},
		TerminalAccounts: []AccountRow{{VenueID: "north", ClientID: 7, Role: "perp_exposure_hedger_1", Account: terminal}},
	}
	run, err := Open(writeRun(t, report, map[string][]string{
		"north/derivatives.jsonl": derivative,
		"north/general.jsonl":     general,
	}))
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func p7dRiskOptions() PerpExposureRiskOptions {
	return PerpExposureRiskOptions{Role: "perp_exposure_hedger", Symbol: "ABC-PERP", QuoteAsset: "USD", BasePrecision: 100, MaintenanceMarginBps: 500}
}

func p7dRiskAccount(net, debt int64, size, entry int64) Account {
	account := Account{Timestamp: 1, PerpBalances: []Balance{{Asset: "USD", NetAsset: net, Borrowed: debt}}}
	if size != 0 {
		account.Positions = []Position{{Symbol: "ABC-PERP", Size: size, EntryPrice: entry}}
	}
	return account
}

func TestPerpExposureRiskReplaysBorrowedBreachAndLiquidation(t *testing.T) {
	derivative := []string{
		p7dRiskPositionLine(2, 7, "north", 0, 0, -100, 100),
		p7dRiskMarkLine(3, "north", 200),
		p7dRiskPositionLine(3, 7, "north", -100, 100, -40, 100),
		p7dRiskLiquidationLine(3, 7, "north", -100, 50, 0),
	}
	general := []string{
		p7dRiskBalanceLine(2, 7, "north", "borrow", `{"asset":"USD","wallet":"perp","old_balance":100,"new_balance":200,"delta":100},{"asset":"USD","wallet":"borrowed","old_balance":0,"new_balance":100,"delta":100}`),
		p7dRiskBorrowLine(2, 7, "north", 100),
		p7dRiskCheckLine(3, 7, "north", 200, 200, -100, 0, 200, 10),
	}
	initial := p7dRiskAccount(100, 0, 0, 0)
	terminal := p7dRiskAccount(100, 100, -40, 100)
	result, err := p7dRiskRun(t, derivative, general, initial, terminal).MeasurePerpExposureRisk(p7dRiskOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.ExpectedBreaches != 1 || result.ObservedChecks != 1 || result.ParticipantLiquidations != 1 || result.PositionPathFailures != 0 || result.BorrowAmountMismatches != 0 || result.TerminalStateMismatches != 0 {
		t.Fatalf("borrowed breach replay = %+v", result)
	}
}

func TestPerpExposureRiskCatchesMissingOrWrongChecks(t *testing.T) {
	derivative := []string{
		p7dRiskPositionLine(2, 7, "north", 0, 0, -100, 100),
		p7dRiskMarkLine(3, "north", 200),
	}
	initial := p7dRiskAccount(100, 0, 0, 0)
	terminal := p7dRiskAccount(100, 100, -100, 100)
	baseGeneral := []string{
		p7dRiskBalanceLine(2, 7, "north", "borrow", `{"asset":"USD","wallet":"perp","old_balance":100,"new_balance":200,"delta":100},{"asset":"USD","wallet":"borrowed","old_balance":0,"new_balance":100,"delta":100}`),
		p7dRiskBorrowLine(2, 7, "north", 100),
	}
	for _, test := range []struct {
		name                        string
		check                       string
		missing, unexpected, fields int
	}{
		{name: "missing", missing: 1},
		{name: "wrong equity", check: p7dRiskCheckLine(3, 7, "north", 200, 200, -100, 1, 200, 10), fields: 1},
		{name: "wrong mark", check: p7dRiskCheckLine(3, 7, "north", 199, 200, -100, 0, 200, 10), fields: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			general := append([]string{}, baseGeneral...)
			if test.check != "" {
				general = append(general, test.check)
			}
			result, err := p7dRiskRun(t, derivative, general, initial, terminal).MeasurePerpExposureRisk(p7dRiskOptions())
			if err != nil {
				t.Fatal(err)
			}
			if result.MissingChecks != test.missing || result.UnexpectedChecks != test.unexpected || result.FieldMismatches != test.fields || result.Valid {
				t.Fatalf("risk mutation survived: %+v", result)
			}
		})
	}
}

func TestPerpExposureRiskControlWithoutPositionIsValid(t *testing.T) {
	derivative := []string{p7dRiskMarkLine(2, "north", 200)}
	account := p7dRiskAccount(100, 0, 0, 0)
	result, err := p7dRiskRun(t, derivative, nil, account, account).MeasurePerpExposureRisk(p7dRiskOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.ExpectedBreaches != 0 || result.ObservedChecks != 0 || result.ParticipantLiquidations != 0 {
		t.Fatalf("control replay = %+v", result)
	}
}

func TestPerpExposureRiskCatchesBorrowOnlyWireMutation(t *testing.T) {
	derivative := []string{p7dRiskPositionLine(2, 7, "north", 0, 0, -100, 100)}
	initial := p7dRiskAccount(100, 0, 0, 0)
	terminal := p7dRiskAccount(100, 100, -100, 100)
	general := []string{
		p7dRiskBalanceLine(2, 7, "north", "borrow", `{"asset":"USD","wallet":"perp","old_balance":100,"new_balance":200,"delta":100},{"asset":"USD","wallet":"borrowed","old_balance":0,"new_balance":100,"delta":100}`),
		p7dRiskBorrowLine(2, 7, "north", 99),
	}
	result, err := p7dRiskRun(t, derivative, general, initial, terminal).MeasurePerpExposureRisk(p7dRiskOptions())
	if err != nil {
		t.Fatal(err)
	}
	if result.BorrowAmountMismatches == 0 || result.Valid {
		t.Fatalf("borrow-only mutation survived: %+v", result)
	}
}
