package analysis

import (
	"fmt"
	"testing"
)

func hedgeFill(ts int64, clientID uint64, venue, symbol, role, side string, qty int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"OrderFill","data":{"venue_id":%q,"payload":{"symbol":%q,"role":%q,"side":%q,"qty":%d}}}`,
		ts, clientID, venue, symbol, role, side, qty)
}

// A population can be configured with three hedging policies and run as though
// it had one. The configuration says what a dealer was told to do; this says
// what it did, which is the only thing a reader can check.
func TestHedgingSeparatesATimedDeskFromABandedOne(t *testing.T) {
	const second = int64(1_000_000_000)
	report := Report{TerminalAccounts: []AccountRow{
		{VenueID: "north", ClientID: 1, Role: "option_dealer_1"},
		{VenueID: "north", ClientID: 2, Role: "option_dealer_2"},
		{VenueID: "north", ClientID: 3, Role: "noise_flow_1"},
	}}
	var lines []string
	// A timed desk hedges every sixty seconds, whatever the market did.
	for i := int64(1); i <= 12; i++ {
		lines = append(lines, hedgeFill(i*60*second, 1, "north", "ABC/USD", "taker", "BUY", 100))
	}
	// A banded desk hedges when the band breaks, so its spacing varies.
	for i, gap := range []int64{5, 90, 7, 240, 11, 33, 400, 15, 60, 8, 700, 21} {
		_ = i
		lines = append(lines, hedgeFill(gap*second*int64(len(lines)+1), 2, "north", "ABC/USD", "taker", "SELL", 100))
	}
	// Uninformed flow is not hedging and must be excluded by role.
	lines = append(lines, hedgeFill(10*second, 3, "north", "ABC/USD", "taker", "BUY", 100))
	dir := writeRun(t, report, map[string][]string{"north/spot/ABC-USD.jsonl": lines})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureHedging(HedgingOptions{Symbol: "ABC/USD", Roles: []string{"option_dealer"}})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if len(result.Profiles) != 2 {
		t.Fatalf("profiles = %d, want the two dealers only: %+v", len(result.Profiles), result.Profiles)
	}
	timed, banded := result.Profiles[0], result.Profiles[1]
	if timed.Trades != 12 || timed.MedianGapSeconds != 60 {
		t.Errorf("timed desk = %d trades at %v seconds, want 12 at 60", timed.Trades, timed.MedianGapSeconds)
	}
	if timed.GapSpreadSeconds != 0 {
		t.Errorf("a timed desk had a spacing spread of %v, want none", timed.GapSpreadSeconds)
	}
	if banded.GapSpreadSeconds <= timed.GapSpreadSeconds {
		t.Errorf("the banded desk's spacing is no more variable than the timed one's: %v against %v",
			banded.GapSpreadSeconds, timed.GapSpreadSeconds)
	}
	if timed.BuyShare != 1 || banded.BuyShare != 0 {
		t.Errorf("buy shares = %v and %v, want one-sided books in opposite directions", timed.BuyShare, banded.BuyShare)
	}
}

// A maker fill is not a hedge: the desk that waited for someone to trade with
// it did not pay to get flat.
func TestHedgingCountsOnlyTakerFills(t *testing.T) {
	report := Report{TerminalAccounts: []AccountRow{{VenueID: "north", ClientID: 1, Role: "option_dealer_1"}}}
	lines := []string{
		hedgeFill(1_000_000_000, 1, "north", "ABC/USD", "maker", "BUY", 100),
		hedgeFill(2_000_000_000, 1, "north", "ABC/USD", "taker", "BUY", 50),
	}
	dir := writeRun(t, report, map[string][]string{"north/spot/ABC-USD.jsonl": lines})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureHedging(HedgingOptions{Symbol: "ABC/USD"})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if len(result.Profiles) != 1 || result.Profiles[0].Trades != 1 || result.Profiles[0].Qty != 50 {
		t.Errorf("profiles = %+v, want one taker fill of 50", result.Profiles)
	}
}
