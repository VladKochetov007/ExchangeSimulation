package analysis

import (
	"fmt"
	"testing"
)

func lifecycleLine(ts int64, venue, event, symbol, kind string) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":0,"event":%q,"data":{"venue_id":%q,"payload":{"symbol":%q,"instrument_type":%q}}}`,
		ts, event, venue, symbol, kind)
}

func fundingLine(ts int64, venue string, clientID uint64, instant int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"balance_change","data":{"venue_id":%q,"payload":{"symbol":"ABC-PERP","payload":{"reason":"funding_settlement","timestamp":%d,"symbol":"ABC-PERP"}}}}`,
		ts, clientID, venue, instant)
}

// A viability corridor cannot tell whether the market was asked to survive
// anything: a population that never lists, never expires and never charges
// funding holds every book alive trivially. The census is what makes a
// lifecycle claim checkable.
func TestLifecycleCountsRoundsNotEvents(t *testing.T) {
	const hour = int64(3_600_000_000_000)
	lines := []string{
		lifecycleLine(0, "north", "instrument_listed", "ABC-FUT-1", "FUTURE"),
		lifecycleLine(0, "north", "instrument_listed", "ABC-1-C", "OPTION"),
		lifecycleLine(0, "north", "instrument_listed", "ABC-1-P", "OPTION"),
		lifecycleLine(2*hour, "north", "instrument_settled", "ABC-FUT-1", "FUTURE"),
		lifecycleLine(2*hour, "north", "instrument_settled", "ABC-1-C", "OPTION"),
		lifecycleLine(2*hour, "north", "instrument_settled", "ABC-1-P", "OPTION"),
		lifecycleLine(2*hour, "north", "instrument_listed", "ABC-FUT-2", "FUTURE"),
		lifecycleLine(4*hour, "north", "instrument_settled", "ABC-FUT-2", "FUTURE"),
	}
	dir := writeRun(t, Report{}, map[string][]string{"north/general.jsonl": lines})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureLifecycle(LifecycleOptions{})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if result.Listings["OPTION"] != 2 || result.Listings["FUTURE"] != 2 {
		t.Errorf("listings = %v, want two of each", result.Listings)
	}
	// Six settlements happened at two instants: two cycles, not six.
	if result.SettlementRounds != 2 {
		t.Errorf("settlement rounds = %d, want 2", result.SettlementRounds)
	}
	if result.ListingRounds != 2 {
		t.Errorf("listing rounds = %d, want 2", result.ListingRounds)
	}
	if len(result.Contracts) != 4 {
		t.Fatalf("contracts = %d, want 4", len(result.Contracts))
	}
	for _, contract := range result.Contracts {
		if contract.Symbol == "ABC-FUT-1" && contract.SettledNano != 2*hour {
			t.Errorf("%s settled at %d, want %d", contract.Symbol, contract.SettledNano, 2*hour)
		}
	}
}

// Venues settling on different periods is the point: a schedule shared by
// every venue leaves a desk holding the same exposure at two of them with
// nothing to trade between the two payments.
func TestLifecycleReportsEachVenuesFundingPeriodAndTheirIntersections(t *testing.T) {
	const hour = int64(3_600_000_000_000)
	north := []string{}
	central := []string{}
	south := []string{}
	for i := int64(1); i <= 8; i++ {
		central = append(central, fundingLine(i*hour, "central", 1, i*hour))
	}
	for i := int64(1); i <= 4; i++ {
		south = append(south, fundingLine(2*i*hour, "south", 2, 2*i*hour))
	}
	north = append(north, fundingLine(8*hour, "north", 3, 8*hour))
	dir := writeRun(t, Report{}, map[string][]string{
		"north/derivatives.jsonl":   north,
		"central/derivatives.jsonl": central,
		"south/derivatives.jsonl":   south,
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureLifecycle(LifecycleOptions{})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	periods := map[string]float64{}
	counts := map[string]int{}
	for _, schedule := range result.Funding {
		periods[schedule.VenueID] = schedule.PeriodSeconds
		counts[schedule.VenueID] = schedule.Settlements
	}
	if periods["central"] != 3600 || periods["south"] != 7200 {
		t.Errorf("periods = %v, want central hourly and south two-hourly", periods)
	}
	if counts["north"] != 1 || counts["central"] != 8 || counts["south"] != 4 {
		t.Errorf("settlement counts = %v", counts)
	}
	// Four instants had two venues settling together and one had all three.
	if result.FundingIntersections[3] != 1 {
		t.Errorf("instants with all three venues = %d, want 1: %v", result.FundingIntersections[3], result.FundingIntersections)
	}
	if result.FundingIntersections[2] != 3 {
		t.Errorf("instants with two venues = %d, want 3: %v", result.FundingIntersections[2], result.FundingIntersections)
	}
}

// A claim about how many cycles a market survived is per venue. Two venues
// expiring on offset schedules produce more distinct instants between them
// than either one lived through, so the pooled count overstates both.
func TestLifecycleCountsRoundsPerVenue(t *testing.T) {
	const hour = int64(3_600_000_000_000)
	north := []string{
		lifecycleLine(0, "north", "instrument_listed", "N-1", "FUTURE"),
		lifecycleLine(2*hour, "north", "instrument_settled", "N-1", "FUTURE"),
	}
	south := []string{
		lifecycleLine(hour, "south", "instrument_listed", "S-1", "FUTURE"),
		lifecycleLine(3*hour, "south", "instrument_settled", "S-1", "FUTURE"),
	}
	dir := writeRun(t, Report{}, map[string][]string{
		"north/general.jsonl": north,
		"south/general.jsonl": south,
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureLifecycle(LifecycleOptions{})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if result.SettlementRounds != 2 {
		t.Errorf("pooled settlement rounds = %d, want 2", result.SettlementRounds)
	}
	if result.SettlementRoundsByVenue["north"] != 1 || result.SettlementRoundsByVenue["south"] != 1 {
		t.Errorf("per-venue settlement rounds = %v, want one each", result.SettlementRoundsByVenue)
	}
}
