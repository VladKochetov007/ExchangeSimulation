package analysis

import (
	"math"
	"testing"
)

// A book whose levels sit a fixed distance apart must report exactly that
// spacing, and the same book scaled in price must report double it while its
// level count and touch share are unchanged. That is the whole distinction the
// metric exists to draw: a stretched price axis against a changed shape.
func TestLevelSpacingSeparatesStretchFromShape(t *testing.T) {
	build := func(gap int64) string {
		lines := []string{}
		// Five levels a side, evenly spaced, symmetric about 100000.
		for i := int64(0); i < 5; i++ {
			lines = append(lines,
				deltaLine(1, "BUY", 100_000-gap*(i+1), 100),
				deltaLine(1, "SELL", 100_000+gap*(i+1), 100))
		}
		// Trades so the visitor fires; they must not change the book.
		for i := int64(0); i < 40; i++ {
			lines = append(lines, tradeEventLine(2+i, uint64(i+1), "BUY", 100_000+gap, 1))
		}
		return writeLog(t, lines)
	}

	narrow, err := MeasureLevelSpacing(build(10), SpacingOptions{TickSize: 10})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	wide, err := MeasureLevelSpacing(build(20), SpacingOptions{TickSize: 10})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}

	if narrow.LevelsPerSide.Median != 5 || wide.LevelsPerSide.Median != 5 {
		t.Errorf("level counts changed under a pure stretch: %v against %v",
			narrow.LevelsPerSide.Median, wide.LevelsPerSide.Median)
	}
	if narrow.FirstGapTicks.Median != 1 {
		t.Errorf("first gap = %v ticks, want 1", narrow.FirstGapTicks.Median)
	}
	if wide.FirstGapTicks.Median != 2 {
		t.Errorf("first gap = %v ticks, want 2", wide.FirstGapTicks.Median)
	}
	if wide.AllGapsTicks.Median != 2*narrow.AllGapsTicks.Median {
		t.Errorf("doubling the spacing gave %v against %v ticks",
			wide.AllGapsTicks.Median, narrow.AllGapsTicks.Median)
	}
	// The narrow book sits exactly on the tick grid, so every gap is one tick
	// and the quantisation warning must fire.
	if narrow.SingleTickGapShare != 1 {
		t.Errorf("single-tick share = %v on a book spaced one tick apart", narrow.SingleTickGapShare)
	}
	if wide.SingleTickGapShare != 0 {
		t.Errorf("single-tick share = %v on a book spaced two ticks apart", wide.SingleTickGapShare)
	}
}

// Gaps are measured between adjacent levels moving away from the touch, on
// both sides, and the two sides order oppositely. Measuring the bid side
// outward-in would report the distance to the far end of the book.
func TestLevelSpacingMeasuresBothSidesOutward(t *testing.T) {
	lines := []string{
		// Bids 3 ticks apart, asks 7 ticks apart, so the pooled distribution
		// must contain both and the sides must not be confused.
		deltaLine(1, "BUY", 100_000, 100),
		deltaLine(1, "BUY", 99_970, 100),
		deltaLine(1, "SELL", 100_100, 100),
		deltaLine(1, "SELL", 100_170, 100),
	}
	for i := int64(0); i < 40; i++ {
		lines = append(lines, tradeEventLine(2+i, uint64(i+1), "BUY", 100_100, 1))
	}
	result, err := MeasureLevelSpacing(writeLog(t, lines), SpacingOptions{TickSize: 10})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if result.AllGapsTicks.Median != 3 && result.AllGapsTicks.Median != 7 {
		t.Errorf("pooled gaps median = %v, want one of the two constructed spacings",
			result.AllGapsTicks.Median)
	}
	if result.AllGapsTicks.Max != 7 {
		t.Errorf("largest gap = %v ticks, want 7", result.AllGapsTicks.Max)
	}
	if got := result.SpreadTicks.Median; got != 10 {
		t.Errorf("spread = %v ticks, want 10", got)
	}
}

// Sampling must thin the observations without changing what they say.
func TestLevelSpacingSamplingPreservesTheDistribution(t *testing.T) {
	lines := []string{
		deltaLine(1, "BUY", 100_000, 1_000_000),
		deltaLine(1, "BUY", 99_950, 1_000_000),
		deltaLine(1, "SELL", 100_100, 1_000_000),
		deltaLine(1, "SELL", 100_150, 1_000_000),
	}
	for i := int64(0); i < 400; i++ {
		lines = append(lines, tradeEventLine(2+i, uint64(i+1), "BUY", 100_100, 1))
	}
	path := writeLog(t, lines)
	every, err := MeasureLevelSpacing(path, SpacingOptions{TickSize: 10})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	tenth, err := MeasureLevelSpacing(path, SpacingOptions{TickSize: 10, SampleEvery: 10})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if tenth.Observations*10 > every.Observations+20 || tenth.Observations == 0 {
		t.Errorf("sampling every tenth trade kept %d of %d observations",
			tenth.Observations, every.Observations)
	}
	if math.Abs(tenth.AllGapsTicks.Median-every.AllGapsTicks.Median) > 1e-9 {
		t.Errorf("sampling changed the median gap: %v against %v",
			tenth.AllGapsTicks.Median, every.AllGapsTicks.Median)
	}
}

// Distance is measured away from the midpoint on the order's own side, so a
// bid below the mid and an ask above it by the same amount read the same. A
// signed convention that did not flip for bids would report them as negative
// and they would be discarded as marketable.
func TestRestingPlacementMeasuresBothSidesAwayFromMid(t *testing.T) {
	lines := []string{
		deltaLine(1, "BUY", 100_000, 100),
		deltaLine(1, "SELL", 100_200, 100),
		// Mid is 100100. A bid 10 ticks below and an ask 10 ticks above.
		acceptLine(2, 1, "BUY", 100_000, 50),
		acceptLine(2, 2, "SELL", 100_200, 50),
	}
	result, err := MeasureRestingPlacement(writeLog(t, lines), RestingOptions{
		TickSize: 10,
		Role:     func(clientID uint64) string { return "maker" },
	})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	stats := result.ByRole["maker"]
	if stats == nil || stats.Orders != 2 {
		t.Fatalf("both sides not attributed: %+v", result)
	}
	if stats.DistanceTicks.Median != 10 || stats.DistanceTicks.Max != 10 {
		t.Errorf("distances = %+v, want both at 10 ticks", stats.DistanceTicks)
	}
}

// An order priced through the mid is taking liquidity, not resting it, and
// counting it as depth at a negative distance would corrupt the distribution.
func TestRestingPlacementExcludesMarketableOrders(t *testing.T) {
	lines := []string{
		deltaLine(1, "BUY", 100_000, 100),
		deltaLine(1, "SELL", 100_200, 100),
		acceptLine(2, 1, "BUY", 100_500, 50), // through the ask
		acceptLine(2, 2, "BUY", 100_000, 50), // resting
	}
	result, err := MeasureRestingPlacement(writeLog(t, lines), RestingOptions{
		TickSize: 10,
		Role:     func(clientID uint64) string { return "maker" },
	})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if result.Marketable != 1 {
		t.Errorf("marketable orders = %d, want 1", result.Marketable)
	}
	if stats := result.ByRole["maker"]; stats == nil || stats.Orders != 1 {
		t.Errorf("resting orders = %+v, want 1", stats)
	}
}

// The classes furthest from the touch are the ones that set the book's width,
// so they must be listed first.
func TestRestingPlacementRanksByDistance(t *testing.T) {
	lines := []string{
		deltaLine(1, "BUY", 100_000, 100),
		deltaLine(1, "SELL", 100_200, 100),
		acceptLine(2, 1, "BUY", 100_090, 50), // 1 tick out
		acceptLine(2, 2, "BUY", 99_100, 50),  // 100 ticks out
	}
	result, err := MeasureRestingPlacement(writeLog(t, lines), RestingOptions{
		TickSize: 10,
		Role: func(clientID uint64) string {
			if clientID == 1 {
				return "near"
			}
			return "far"
		},
	})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	ranked := result.RolesByDistance()
	if len(ranked) != 2 || ranked[0] != "far" {
		t.Errorf("ranking = %v, want the distant class first", ranked)
	}
}
