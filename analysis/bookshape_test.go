package analysis

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"
)

// snapshotLine renders one BookSnapshot record in the envelope the venues write.
func snapshotLine(ts int64, bids, asks [][3]int64) string {
	level := func(entries [][3]int64) []map[string]int64 {
		out := make([]map[string]int64, 0, len(entries))
		for _, entry := range entries {
			out = append(out, map[string]int64{"price": entry[0], "visible_qty": entry[1], "hidden_qty": entry[2]})
		}
		return out
	}
	payload := map[string]any{
		"venue_id": "north",
		"payload":  map[string]any{"bids": level(bids), "asks": level(asks)},
	}
	raw, err := json.Marshal(map[string]any{
		"client_id": 0, "event": "BookSnapshot", "sim_ts": ts, "data": payload,
	})
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func openShapeRun(t *testing.T, lines []string) *Run {
	t.Helper()
	dir := writeRun(t, Report{}, map[string][]string{"north/spot/ABC-USD.jsonl": lines})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open run: %v", err)
	}
	return run
}

func shapeFiles(t *testing.T, run *Run) []string {
	t.Helper()
	files := run.BookFiles("north", "ABC-USD")
	if len(files) == 0 {
		t.Fatal("no book files selected")
	}
	return files
}

// A book whose depth all rests at one price is the degenerate case the metric
// exists to detect: nothing can be consumed beyond the touch.
func TestBookShapeDetectsASingleLevelBook(t *testing.T) {
	run := openShapeRun(t, []string{
		snapshotLine(1, [][3]int64{{100, 50, 0}}, [][3]int64{{102, 50, 0}}),
		snapshotLine(2, [][3]int64{{100, 70, 0}}, [][3]int64{{102, 30, 0}}),
	})
	shape, err := run.MeasureBookShape(BookShapeOptions{Files: shapeFiles(t, run), TickSize: 1})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if shape.Snapshots != 2 {
		t.Errorf("snapshots = %d, want 2", shape.Snapshots)
	}
	if shape.TouchShare.Median != 1 || shape.TouchShare.Mean != 1 {
		t.Errorf("single-level book touch share = %+v, want 1", shape.TouchShare)
	}
	if shape.BeyondTouchDepth.Max != 0 {
		t.Errorf("single-level book reports %v depth beyond the touch", shape.BeyondTouchDepth.Max)
	}
	if shape.BidLevels.Median != 1 || shape.AskLevels.Median != 1 {
		t.Errorf("levels = %v/%v, want 1/1", shape.BidLevels.Median, shape.AskLevels.Median)
	}
	if shape.SpreadTicks.Median != 2 {
		t.Errorf("spread = %v ticks, want 2", shape.SpreadTicks.Median)
	}
}

// A laddered book must report a touch share strictly below one, and the share
// must equal the constructed proportion rather than merely being less than one.
func TestBookShapeMeasuresALadder(t *testing.T) {
	run := openShapeRun(t, []string{
		snapshotLine(1,
			[][3]int64{{100, 25, 0}, {99, 50, 0}, {98, 25, 0}},
			[][3]int64{{102, 25, 0}, {103, 50, 0}, {104, 25, 0}}),
	})
	shape, err := run.MeasureBookShape(BookShapeOptions{Files: shapeFiles(t, run), TickSize: 1})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if got := shape.TouchShare.Median; math.Abs(got-0.25) > 1e-9 {
		t.Errorf("touch share = %v, want 0.25", got)
	}
	if shape.BeyondTouchDepth.Median != 75 {
		t.Errorf("beyond-touch depth = %v, want 75", shape.BeyondTouchDepth.Median)
	}
	if shape.BidLevels.Median != 3 || shape.AskLevels.Median != 3 {
		t.Errorf("levels = %v/%v, want 3/3", shape.BidLevels.Median, shape.AskLevels.Median)
	}
}

// Two makers quoting the same price are one level of liquidity, not two. A
// taker crossing it pays one price, so counting resting orders would overstate
// how laddered the book is.
func TestBookShapeMergesOrdersSharingAPrice(t *testing.T) {
	run := openShapeRun(t, []string{
		snapshotLine(1,
			[][3]int64{{100, 30, 0}, {100, 20, 0}},
			[][3]int64{{102, 50, 0}}),
	})
	shape, err := run.MeasureBookShape(BookShapeOptions{Files: shapeFiles(t, run), TickSize: 1})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if shape.BidLevels.Median != 1 {
		t.Errorf("two orders at one price counted as %v levels", shape.BidLevels.Median)
	}
	if shape.TouchShare.Median != 1 {
		t.Errorf("merged level touch share = %v, want 1", shape.TouchShare.Median)
	}
	if shape.TouchDepth.Median != 50 {
		t.Errorf("merged touch depth = %v, want 50", shape.TouchDepth.Median)
	}
}

// The best price is the highest bid and the lowest ask. A metric that trusted
// the array order would read the touch off whichever level happened to be
// first, so the levels are given out of order deliberately.
func TestBookShapeFindsTheBestPriceRegardlessOfOrder(t *testing.T) {
	run := openShapeRun(t, []string{
		snapshotLine(1,
			[][3]int64{{98, 10, 0}, {100, 40, 0}, {99, 10, 0}},
			[][3]int64{{104, 10, 0}, {102, 40, 0}, {103, 10, 0}}),
	})
	shape, err := run.MeasureBookShape(BookShapeOptions{Files: shapeFiles(t, run), TickSize: 1})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if shape.SpreadTicks.Median != 2 {
		t.Errorf("spread = %v, want 2 (best bid 100, best ask 102)", shape.SpreadTicks.Median)
	}
}

// An empty side is not a shallow book, it is no book. Counting it as depth
// zero would drag the touch-share distribution toward a false conclusion.
func TestBookShapeSeparatesEmptySidesFromShallowOnes(t *testing.T) {
	run := openShapeRun(t, []string{
		snapshotLine(1, [][3]int64{{100, 50, 0}}, nil),
		snapshotLine(2, nil, nil),
		snapshotLine(3, [][3]int64{{100, 50, 0}}, [][3]int64{{102, 50, 0}}),
	})
	shape, err := run.MeasureBookShape(BookShapeOptions{Files: shapeFiles(t, run), TickSize: 1})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if shape.Snapshots != 3 {
		t.Errorf("snapshots = %d, want 3", shape.Snapshots)
	}
	if shape.OneSideEmpty != 1 {
		t.Errorf("one-side-empty = %d, want 1", shape.OneSideEmpty)
	}
	if shape.BothSidesEmpty != 1 {
		t.Errorf("both-sides-empty = %d, want 1", shape.BothSidesEmpty)
	}
	// Three populated sides across the two snapshots that had any liquidity.
	if shape.TouchShare.N != 3 {
		t.Errorf("touch share sampled %d sides, want 3", shape.TouchShare.N)
	}
	if shape.SpreadTicks.N != 1 {
		t.Errorf("spread sampled %d snapshots, want 1 two-sided", shape.SpreadTicks.N)
	}
}

// Hidden quantity is depth a taker can consume but cannot see. It belongs in
// its own share rather than in the visible totals.
func TestBookShapeKeepsHiddenDepthOutOfTheVisibleTotals(t *testing.T) {
	run := openShapeRun(t, []string{
		snapshotLine(1, [][3]int64{{100, 25, 75}}, [][3]int64{{102, 100, 0}}),
	})
	shape, err := run.MeasureBookShape(BookShapeOptions{Files: shapeFiles(t, run), TickSize: 1})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if shape.TouchDepth.Median != 100 {
		t.Errorf("touch depth median = %v; hidden quantity leaked into visible", shape.TouchDepth.Median)
	}
	if shape.HiddenShare.Max != 0.75 {
		t.Errorf("hidden share max = %v, want 0.75", shape.HiddenShare.Max)
	}
}

// The walkable fraction is the decisive quantity: it says what proportion of
// the time an order of a given size would have to reach past the best price.
func TestWalkableFractionBracketsTheTouch(t *testing.T) {
	run := openShapeRun(t, []string{
		// Every side shows 40 at the touch and 60 beyond it.
		snapshotLine(1, [][3]int64{{100, 40, 0}, {99, 60, 0}}, [][3]int64{{102, 40, 0}, {103, 60, 0}}),
		snapshotLine(2, [][3]int64{{100, 40, 0}, {99, 60, 0}}, [][3]int64{{102, 40, 0}, {103, 60, 0}}),
	})
	fractions, err := run.MeasureWalkable(BookShapeOptions{Files: shapeFiles(t, run)}, []int64{150, 10, 50})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if len(fractions) != 3 {
		t.Fatalf("got %d results, want 3", len(fractions))
	}
	// Results are returned in ascending size regardless of the caller's order.
	for i := 1; i < len(fractions); i++ {
		if fractions[i].SizeBase <= fractions[i-1].SizeBase {
			t.Fatalf("sizes not ascending: %+v", fractions)
		}
	}
	want := []struct{ exceedsTouch, exceedsBook float64 }{
		{0, 0}, // 10 fits inside the touch
		{1, 0}, // 50 walks past the touch but fits in the book
		{1, 1}, // 150 exhausts the whole visible side
	}
	for i, expected := range want {
		if fractions[i].ExceedsTouch != expected.exceedsTouch || fractions[i].ExceedsBook != expected.exceedsBook {
			t.Errorf("size %d: %+v, want touch=%v book=%v",
				fractions[i].SizeBase, fractions[i], expected.exceedsTouch, expected.exceedsBook)
		}
		if fractions[i].Sides != 4 {
			t.Errorf("size %d sampled %d sides, want 4", fractions[i].SizeBase, fractions[i].Sides)
		}
	}
}

// A size exactly equal to the resting depth fills at that price without
// reaching the next one, so it must not be counted as walking the book.
func TestWalkableFractionExcludesAnExactFill(t *testing.T) {
	run := openShapeRun(t, []string{
		snapshotLine(1, [][3]int64{{100, 40, 0}, {99, 60, 0}}, [][3]int64{{102, 40, 0}, {103, 60, 0}}),
	})
	fractions, err := run.MeasureWalkable(BookShapeOptions{Files: shapeFiles(t, run)}, []int64{40, 41, 100, 101})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	for _, testCase := range []struct {
		size                      int64
		exceedsTouch, exceedsBook float64
	}{
		{40, 0, 0}, {41, 1, 0}, {100, 1, 0}, {101, 1, 1},
	} {
		var found *WalkableFraction
		for i := range fractions {
			if fractions[i].SizeBase == testCase.size {
				found = &fractions[i]
			}
		}
		if found == nil {
			t.Fatalf("size %d missing from %+v", testCase.size, fractions)
		}
		if found.ExceedsTouch != testCase.exceedsTouch || found.ExceedsBook != testCase.exceedsBook {
			t.Errorf("size %d: touch=%v book=%v, want %v/%v",
				testCase.size, found.ExceedsTouch, found.ExceedsBook, testCase.exceedsTouch, testCase.exceedsBook)
		}
	}
}

// A run with no snapshots must report nothing rather than a plausible-looking
// zero, since the two are indistinguishable downstream otherwise.
func TestBookShapeOnAnEmptyRunReportsNoSample(t *testing.T) {
	run := openShapeRun(t, []string{
		fmt.Sprintf(`{"client_id":0,"event":"Trade","sim_ts":1,"data":{"payload":{"price":100,"qty":1,"side":"BUY"}}}`),
	})
	shape, err := run.MeasureBookShape(BookShapeOptions{Files: shapeFiles(t, run), TickSize: 1})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if shape.Snapshots != 0 || shape.TouchShare.N != 0 {
		t.Errorf("empty run reported %+v", shape)
	}
}
