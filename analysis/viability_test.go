package analysis

import (
	"fmt"
	"testing"
)

func fillLine(ts int64, clientID uint64, venue, symbol, role string, qty int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"OrderFill","data":{"venue_id":%q,"payload":{"symbol":%q,"role":%q,"qty":%d}}}`,
		ts, clientID, venue, symbol, role, qty)
}

func viabilitySnapshotLine(ts int64, venue, symbol string, bid, ask, depth int64) string {
	bids, asks := "[]", "[]"
	if bid > 0 {
		bids = fmt.Sprintf(`[{"price":%d,"visible_qty":%d,"hidden_qty":0}]`, bid, depth)
	}
	if ask > 0 {
		asks = fmt.Sprintf(`[{"price":%d,"visible_qty":%d,"hidden_qty":0}]`, ask, depth)
	}
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":0,"event":"BookSnapshot","data":{"venue_id":%q,"payload":{"symbol":%q,"bids":%s,"asks":%s}}}`,
		ts, venue, symbol, bids, asks)
}

func TestBestWithDepthRetainsSignedZeroLevels(t *testing.T) {
	levels := []bookLevel{
		{Price: -10, VisibleQty: 2},
		{Price: 0, VisibleQty: 3},
		{Price: 5, VisibleQty: 0},
	}
	if best, depth, ok := bestWithDepth(levels, true); !ok || best != 0 || depth != 3 {
		t.Fatalf("signed bid best = (%d, %d, %t), want (0, 3, true)", best, depth, ok)
	}
	if best, depth, ok := bestWithDepth(levels, false); !ok || best != -10 || depth != 2 {
		t.Fatalf("signed ask best = (%d, %d, %t), want (-10, 2, true)", best, depth, ok)
	}
	if best, depth, ok := bestWithDepth([]bookLevel{{Price: 0, VisibleQty: 0}}, true); ok || best != 0 || depth != 0 {
		t.Fatalf("empty side = (%d, %d, %t), want (0, 0, false)", best, depth, ok)
	}
}

func viabilityRun(t *testing.T) *Run {
	t.Helper()
	const second = int64(1_000_000_000)
	report := Report{TerminalAccounts: []AccountRow{
		{VenueID: "north", ClientID: 1, Role: "spot_maker_1"},
		{VenueID: "north", ClientID: 2, Role: "noise_flow_1"},
		{VenueID: "north", ClientID: 3, Role: "round_trip_1"},
	}}
	lines := []string{
		// First window: two taker classes against one maker, two-sided book.
		fillLine(0, 2, "north", "ABC/USD", "taker", 100),
		fillLine(0, 1, "north", "ABC/USD", "maker", 100),
		fillLine(1*second, 3, "north", "ABC/USD", "taker", 100),
		fillLine(1*second, 1, "north", "ABC/USD", "maker", 100),
		viabilitySnapshotLine(0, "north", "ABC/USD", 9_900, 10_100, 50),
		// Second window: one taker class only, and the ask side has gone.
		fillLine(10*second, 2, "north", "ABC/USD", "taker", 100),
		fillLine(10*second, 1, "north", "ABC/USD", "maker", 100),
		viabilitySnapshotLine(10*second, "north", "ABC/USD", 9_900, 0, 50),
	}
	dir := writeRun(t, report, map[string][]string{"north/spot/ABC-USD.jsonl": lines})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return run
}

// The corridor has to separate windows, not average them: a market that dies
// halfway through a run has healthy totals and a dead second half.
func TestViabilitySeparatesWindowsAndCountsClasses(t *testing.T) {
	run := viabilityRun(t)
	result, err := run.MeasureViability(ViabilityOptions{WindowNanos: 5_000_000_000, TickSize: 100})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if len(result.Windows) != 2 {
		t.Fatalf("windows = %d, want 2: %+v", len(result.Windows), result.Windows)
	}
	first, second := result.Windows[0], result.Windows[1]
	if first.Trades != 2 || first.Volume != 200 {
		t.Errorf("first window trades/volume = %d/%d, want 2/200", first.Trades, first.Volume)
	}
	if first.TakerRoles != 2 || first.MakerRoles != 1 {
		t.Errorf("first window roles = %d takers %d makers, want 2 and 1", first.TakerRoles, first.MakerRoles)
	}
	if first.TopRoleVolumeShare != 0.5 {
		t.Errorf("first window concentration = %.3f, want 0.5", first.TopRoleVolumeShare)
	}
	if first.SpreadTicks.Median != 2 {
		t.Errorf("first window spread = %v ticks, want 2", first.SpreadTicks.Median)
	}
	if second.TakerRoles != 1 {
		t.Errorf("second window taker classes = %d, want 1", second.TakerRoles)
	}
	if second.EmptySideSnapshots != 1 {
		t.Errorf("second window one-sided snapshots = %d, want 1", second.EmptySideSnapshots)
	}
	if second.TopRoleVolumeShare != 1 {
		t.Errorf("second window concentration = %.3f, want 1", second.TopRoleVolumeShare)
	}
}

// Volume above zero is the test the corridor exists to replace: the window
// where the book went one-sided and a single class was left trading passes it.
func TestViabilityRulesRejectAMarketThatStillTrades(t *testing.T) {
	run := viabilityRun(t)
	result, err := run.MeasureViability(ViabilityOptions{
		WindowNanos: 5_000_000_000,
		TickSize:    100,
		Rules: []ViabilityRule{
			{Name: "no_volume", Breached: func(w MarketWindow) bool { return w.Volume == 0 }},
			{Name: "one_sided", Breached: func(w MarketWindow) bool { return w.EmptySideSnapshots > 0 }},
			{Name: "single_taker_class", Breached: func(w MarketWindow) bool { return w.TakerRoles < 2 }},
		},
	})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if result.ViableWindows != 1 || result.BreachedWindows != 1 {
		t.Fatalf("viable/breached = %d/%d, want 1/1", result.ViableWindows, result.BreachedWindows)
	}
	if result.BreachesByRule["no_volume"] != 0 {
		t.Error("a window with volume was rejected for having none")
	}
	if result.BreachesByRule["one_sided"] != 1 || result.BreachesByRule["single_taker_class"] != 1 {
		t.Errorf("breaches = %v, want one each for one_sided and single_taker_class", result.BreachesByRule)
	}
	if len(result.DeadBooks) != 0 {
		t.Errorf("a book viable in one window was reported dead: %v", result.DeadBooks)
	}
}

// A book that breaches in every window it appears in is dead, and saying so is
// the difference between a run that degraded and a run with one bad window.
func TestViabilityNamesBooksThatBreachedEverywhere(t *testing.T) {
	run := viabilityRun(t)
	result, err := run.MeasureViability(ViabilityOptions{
		WindowNanos: 5_000_000_000,
		Rules:       []ViabilityRule{{Name: "always", Breached: func(MarketWindow) bool { return true }}},
	})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if len(result.DeadBooks) != 1 || result.DeadBooks[0] != "north ABC/USD" {
		t.Errorf("dead books = %v, want [north ABC/USD]", result.DeadBooks)
	}
	if result.Books != 1 {
		t.Errorf("books = %d, want 1", result.Books)
	}
}

// A spot book publishes without naming itself at either level, so the only
// record of which book it is comes from the file. Pooling those into one
// nameless bucket leaves every spot book with no measured spread at all.
func TestViabilityRecoversTheSpotBookFromItsFile(t *testing.T) {
	report := Report{TerminalAccounts: []AccountRow{{VenueID: "north", ClientID: 1, Role: "spot_maker_1"}}}
	unnamed := `{"sim_ts":0,"client_id":0,"event":"BookSnapshot","data":{"venue_id":"north","payload":{"bids":[{"price":9900,"visible_qty":5,"hidden_qty":0}],"asks":[{"price":10100,"visible_qty":5,"hidden_qty":0}]}}}`
	dir := writeRun(t, report, map[string][]string{"north/spot/ABC-USD.jsonl": {unnamed}})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureViability(ViabilityOptions{WindowNanos: 1_000_000_000, TickSize: 100})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if len(result.Windows) != 1 {
		t.Fatalf("windows = %d, want 1", len(result.Windows))
	}
	if got := result.Windows[0].Symbol; got != "ABC/USD" {
		t.Errorf("symbol = %q, want ABC/USD recovered from the file name", got)
	}
	if result.Windows[0].SpreadTicks.N != 1 {
		t.Error("the recovered book has no measured spread")
	}
}

// A ten-cycle run is read one row per book, not sixteen hundred windows. A
// market that died in a late cycle and one that was never alive are different
// failures and the rollup has to tell them apart.
func TestViabilityRollsUpEachBookAndDatesItsFirstBreach(t *testing.T) {
	run := viabilityRun(t)
	result, err := run.MeasureViability(ViabilityOptions{
		WindowNanos: 5_000_000_000,
		Rules:       []ViabilityRule{{Name: "single_taker_class", Breached: func(w MarketWindow) bool { return w.TakerRoles < 2 }}},
	})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if len(result.BookSummaries) != 1 {
		t.Fatalf("summaries = %d, want 1", len(result.BookSummaries))
	}
	summary := result.BookSummaries[0]
	if summary.Windows != 2 || summary.Viable != 1 {
		t.Errorf("summary = %d windows %d viable, want 2 and 1", summary.Windows, summary.Viable)
	}
	if summary.TakerRoles != 2 || summary.MakerRoles != 1 || summary.Snapshots != 2 || summary.EmptySideSnapshots != 1 {
		t.Errorf("whole-book role/snapshot totals = %+v, want 2 takers, 1 maker, 2 snapshots, 1 empty", summary)
	}
	// The second window's events land at ten seconds, which is window two of a
	// five-second grid counted from the run's first window.
	if summary.FirstBreachWindow != 2 {
		t.Errorf("first breach = window %d, want 2", summary.FirstBreachWindow)
	}
	if summary.LastViableWindow != 0 {
		t.Errorf("last viable = window %d, want 0", summary.LastViableWindow)
	}
	if summary.Trades != 3 {
		t.Errorf("trades = %d, want 3", summary.Trades)
	}
	if summary.Breaches["single_taker_class"] != 1 {
		t.Errorf("breaches = %v, want one single_taker_class", summary.Breaches)
	}
}

// A book that never passed must be distinguishable from one that passed and
// then stopped: its first breach is its first window and it was never viable.
func TestViabilityMarksABookThatWasNeverAlive(t *testing.T) {
	run := viabilityRun(t)
	result, err := run.MeasureViability(ViabilityOptions{
		WindowNanos: 5_000_000_000,
		Rules:       []ViabilityRule{{Name: "always", Breached: func(MarketWindow) bool { return true }}},
	})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	summary := result.BookSummaries[0]
	if summary.Viable != 0 || summary.FirstBreachWindow != 0 || summary.LastViableWindow != -1 {
		t.Errorf("summary = %+v, want never viable with a first breach at window 0", summary)
	}
}

// A contract that listed part way through a window, or settled part way
// through one, cannot show a window's worth of volume. Those windows have to
// be identifiable, or every expiry reads as a market that died.
func TestViabilityMarksTheWindowsAtEachEndOfABooksLife(t *testing.T) {
	run := viabilityRun(t)
	result, err := run.MeasureViability(ViabilityOptions{WindowNanos: 5_000_000_000})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if len(result.Windows) != 2 {
		t.Fatalf("windows = %d, want 2", len(result.Windows))
	}
	first, last := result.Windows[0], result.Windows[1]
	if !first.FirstForBook || first.LastForBook {
		t.Errorf("first window marked %+v, want first only", first)
	}
	if last.FirstForBook || !last.LastForBook {
		t.Errorf("last window marked %+v, want last only", last)
	}
}
