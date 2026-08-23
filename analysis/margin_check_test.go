package analysis

import (
	"fmt"
	"testing"
)

func marginPositionLine(ts int64, venue string, client uint64, oldSize, newSize, entry int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"position_update","data":{"venue_id":%q,"payload":{"symbol":"ABC-PERP","payload":{"symbol":"ABC-PERP","old_size":%d,"new_size":%d,"new_entry_price":%d}}}}`,
		ts, client, venue, oldSize, newSize, entry)
}

func marginMarkLine(ts int64, venue string, mark int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":0,"event":"mark_price_update","data":{"venue_id":%q,"payload":{"symbol":"ABC-PERP","payload":{"symbol":"ABC-PERP","mark_price":%d}}}}`, ts, venue, mark)
}

func marginCheckLine(ts int64, venue string, client uint64, mark, balance, contribution, equity, notional, maintenance int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"liquidation_check","data":{"venue_id":%q,"payload":{"symbol":"ABC-PERP","mark_price":%d,"balance":%d,"derivative_equity_contribution":%d,"equity":%d,"notional":%d,"maintenance_margin":%d}}}`, ts, client, venue, mark, balance, contribution, equity, notional, maintenance)
}

func marginTestRun(t *testing.T, derivative, general []string) *Run {
	t.Helper()
	report := Report{InitialAccounts: []AccountRow{{
		VenueID: "north", ClientID: 7, Role: "noise_flow_1",
		Account: Account{Timestamp: 1, PerpBalances: []Balance{{Asset: "USD", NetAsset: 100}}},
	}}}
	run, err := Open(writeRun(t, report, map[string][]string{
		"north/derivatives.jsonl": derivative,
		"north/general.jsonl":     general,
	}))
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func marginTestOptions() MarginCheckOptions {
	return MarginCheckOptions{Role: "noise_flow", Symbol: "ABC-PERP", QuoteAsset: "USD", BasePrecision: 100, MaintenanceMarginBps: 500}
}

func TestMarginCheckAuditReplaysObservedBreach(t *testing.T) {
	// Short one base unit from 100 to 200: -100 contribution; balance 100
	// leaves zero equity against ten units of maintenance.
	run := marginTestRun(t,
		[]string{marginPositionLine(2, "north", 7, 0, -100, 100), marginMarkLine(3, "north", 200)},
		[]string{marginCheckLine(3, "north", 7, 200, 100, -100, 0, 200, 10)},
	)
	result, err := run.MeasureMarginChecks(marginTestOptions())
	if err != nil {
		t.Fatal(err)
	}
	if result.EligibleCandidates != 1 || result.ActiveMarkChecks != 1 || result.ExpectedBreaches != 1 || result.ObservedChecks != 1 || result.MissingChecks != 0 || result.UnexpectedChecks != 0 || result.FieldChecks != 1 || result.FieldMismatches != 0 {
		t.Fatalf("margin replay = %+v", result)
	}
}

func TestMarginCheckAuditCatchesMissingAndIncorrectObservedCheck(t *testing.T) {
	derivative := []string{marginPositionLine(2, "north", 7, 0, -100, 100), marginMarkLine(3, "north", 200)}
	for _, test := range []struct {
		name                                string
		general                             []string
		wantMissing, wantBad, wantDuplicate int
	}{
		{"missing", nil, 1, 0, 0},
		{"wrong equity", []string{marginCheckLine(3, "north", 7, 200, 100, -100, 1, 200, 10)}, 0, 1, 0},
		{"stale mark", []string{marginCheckLine(3, "north", 7, 199, 100, -100, 0, 200, 10)}, 0, 1, 0},
		{"duplicate", []string{
			marginCheckLine(3, "north", 7, 200, 100, -100, 0, 200, 10),
			marginCheckLine(3, "north", 7, 200, 100, -100, 0, 200, 10),
		}, 0, 0, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := marginTestRun(t, derivative, test.general).MeasureMarginChecks(marginTestOptions())
			if err != nil {
				t.Fatal(err)
			}
			if result.MissingChecks != test.wantMissing || result.FieldMismatches != test.wantBad || result.DuplicateChecks != test.wantDuplicate {
				t.Fatalf("mutation survived: %+v", result)
			}
			if test.name == "stale mark" && result.MarkMismatches != 1 {
				t.Fatalf("stale mark was not localized: %+v", result)
			}
		})
	}
}

func TestMarginCheckAuditExcludesCrossInstrumentAccount(t *testing.T) {
	derivative := []string{
		marginPositionLine(2, "north", 7, 0, -100, 100),
		derivativeFillLine(2, 7, "north", "ABC-OPTION", "BUY", 1, 1),
		marginMarkLine(3, "north", 200),
	}
	result, err := marginTestRun(t, derivative, nil).MeasureMarginChecks(marginTestOptions())
	if err != nil {
		t.Fatal(err)
	}
	if result.EligibleCandidates != 0 || result.ExcludedCandidates != 1 || result.ExpectedBreaches != 0 || len(result.Exclusions) != 1 || result.Exclusions[0].Reasons[0] != "other_instrument_fill" {
		t.Fatalf("cross-instrument candidate was guessed about: %+v", result)
	}
}

func TestMarginCheckAuditExcludesAmbiguousSameTimestampMark(t *testing.T) {
	derivative := []string{
		marginPositionLine(2, "north", 7, 0, -100, 100),
		marginMarkLine(3, "north", 200),
		marginMarkLine(3, "north", 201),
	}
	general := []string{marginCheckLine(3, "north", 7, 200, 100, -100, 0, 200, 10)}
	result, err := marginTestRun(t, derivative, general).MeasureMarginChecks(marginTestOptions())
	if err != nil {
		t.Fatal(err)
	}
	if result.AmbiguousMarkTimestampCollisions != 1 || result.EligibleCandidates != 0 || result.ExcludedCandidates != 1 || result.ExpectedBreaches != 0 {
		t.Fatalf("same-timestamp marks were guessed about: %+v", result)
	}
	if len(result.Exclusions) != 1 || result.Exclusions[0].Reasons[0] != "ambiguous_same_timestamp_mark" {
		t.Fatalf("ambiguous mark exclusion = %+v", result.Exclusions)
	}
}

func TestIndependentMarginCheckUsesExactIntermediateArithmetic(t *testing.T) {
	// These are an observed V-005 check's pre-liquidation inputs. The raw
	// quantity-times-price products exceed int64, so a float or int64-only
	// analyzer would silently change the breach boundary.
	check, ok := independentMarginCheck(1532150943205, -74209105910, 2956839955, 4795020400, DefaultMarginCheckOptions())
	if !ok {
		t.Fatal("margin arithmetic unexpectedly unrepresentable")
	}
	if check.contribution != -1364097273246 || check.equity != 168053669959 || check.notional != 3558341767042 || check.maintenance != 177917088352 {
		t.Fatalf("overflow-safe margin replay = %+v", check)
	}
}
