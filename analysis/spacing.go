package analysis

import "math"

// LevelSpacing describes the geometry of the book a taker actually faces:
// how many prices are stacked on a side, and how far apart they sit.
//
// It separates two things that a level count alone confuses. A book can gain
// impact per sweep either by acquiring more levels to walk or by spreading the
// levels it already has further apart. The first is a change in the book's
// shape; the second only rescales its price axis, and only the second follows
// from makers shifting their reservation prices.
type LevelSpacing struct {
	Observations int `json:"observations"`

	// FirstGapTicks is the distance from the best price to the next one, which
	// is what an order clearing exactly the touch must cross.
	FirstGapTicks Distribution `json:"first_gap_ticks"`
	// AllGapsTicks pools every adjacent pair on a side.
	AllGapsTicks Distribution `json:"all_gaps_ticks"`
	// LevelsPerSide counts prices resting on one side.
	LevelsPerSide Distribution `json:"levels_per_side"`
	// SpreadTicks is the distance between the two best prices.
	SpreadTicks Distribution `json:"spread_ticks"`

	// SingleTickGapShare is the fraction of adjacent pairs exactly one tick
	// apart. A high value means the true spacing is below the tick and the
	// grid is what is being measured, not the makers' dispersion.
	SingleTickGapShare float64 `json:"single_tick_gap_share"`

	Drift ReplayDrift `json:"drift"`
}

// SpacingOptions configures the measurement.
type SpacingOptions struct {
	TickSize int64
	// SampleEvery keeps one observation in this many trades, since consecutive
	// trades see almost the same book and pooling them all overweights busy
	// instants. Zero keeps every trade.
	SampleEvery int
}

// MeasureLevelSpacing replays a book and summarises its geometry as seen by
// each aggressive order.
func MeasureLevelSpacing(path string, opts SpacingOptions) (*LevelSpacing, error) {
	if opts.TickSize <= 0 {
		opts.TickSize = 1
	}
	sampleEvery := opts.SampleEvery
	if sampleEvery < 1 {
		sampleEvery = 1
	}

	var firstGaps, allGaps, levels, spreads []float64
	singleTick, totalGaps := 0, 0
	seen := 0

	drift, err := ReplayFile(path, func(_ int64, _ tradePayload, book *ReplayedBook) {
		seen++
		if seen%sampleEvery != 0 {
			return
		}
		bid, bidOK := book.BestBid()
		ask, askOK := book.BestAsk()
		if bidOK && askOK && bid <= ask {
			spreads = append(spreads, (float64(ask)-float64(bid))/float64(opts.TickSize))
		}
		for _, side := range []bool{true, false} {
			prices := book.sortedLevels(side)
			if len(prices) == 0 {
				continue
			}
			levels = append(levels, float64(len(prices)))
			if len(prices) < 2 {
				continue
			}
			// sortedLevels returns prices in consumption order, so successive
			// entries are adjacent levels moving away from the touch.
			first := math.Abs(float64(prices[1]) - float64(prices[0]))
			firstGaps = append(firstGaps, first/float64(opts.TickSize))
			for i := 1; i < len(prices); i++ {
				gap := math.Abs(float64(prices[i]) - float64(prices[i-1]))
				allGaps = append(allGaps, gap/float64(opts.TickSize))
				totalGaps++
				if gap == float64(opts.TickSize) {
					singleTick++
				}
			}
		}
	})
	if err != nil {
		return nil, err
	}

	result := &LevelSpacing{Observations: len(levels), Drift: *drift}
	result.FirstGapTicks = Describe(firstGaps)
	result.AllGapsTicks = Describe(allGaps)
	result.LevelsPerSide = Describe(levels)
	result.SpreadTicks = Describe(spreads)
	if totalGaps > 0 {
		result.SingleTickGapShare = float64(singleTick) / float64(totalGaps)
	}
	return result, nil
}
