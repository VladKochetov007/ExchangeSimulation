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
