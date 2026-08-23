package analysis

import (
	"sort"
	"sync"
)

// CrossVenueDispersionOptions selects same-asset venue quotes for an offline,
// staleness-bounded midpoint comparison. This observer is omniscient and is
// never a participant information set or a router decision rule.
type CrossVenueDispersionOptions struct {
	Files          []string
	FilesSelected  bool
	Symbol         string
	StalenessNanos int64
	MinVenues      int
}

// CrossVenueDispersion describes the contemporaneous range of fresh,
// two-sided venue midpoints. Bps is the relative max-minus-min midpoint range
// using the minimum midpoint as denominator.
type CrossVenueDispersion struct {
	Symbol                   string       `json:"symbol"`
	StalenessNanos           int64        `json:"staleness_nanos"`
	MinVenues                int          `json:"min_venues"`
	QuoteUpdates             int          `json:"quote_updates"`
	Evaluated                int          `json:"evaluated"`
	SkippedInsufficientFresh int          `json:"skipped_insufficient_fresh"`
	MidpointRangeBps         Distribution `json:"midpoint_range_bps"`
	LongestPositiveRunNanos  int64        `json:"longest_positive_run_nanos"`
}

type dispersionQuote struct {
	venue string
	quote venueMidQuote
}

type venueMidQuote struct {
	mid     int64
	at      int64
	ordinal int64
}

// MeasureCrossVenueDispersion independently reconstructs venue midpoint
// dispersion from persisted periodic snapshots. It rejects empty, one-sided,
// crossed, and stale quotes rather than manufacturing a midpoint or treating
// an unavailable price as zero.
func (r *Run) MeasureCrossVenueDispersion(opts CrossVenueDispersionOptions) (*CrossVenueDispersion, error) {
	if opts.StalenessNanos <= 0 {
		opts.StalenessNanos = int64(2e9)
	}
	if opts.MinVenues <= 0 {
		opts.MinVenues = 3
	}
	result := &CrossVenueDispersion{
		Symbol: opts.Symbol, StalenessNanos: opts.StalenessNanos, MinVenues: opts.MinVenues,
	}
	series := make(map[string][]venueMidQuote)
	var mu sync.Mutex
	err := r.Scan(ScanOptions{Events: []string{"BookSnapshot"}, Files: opts.Files, FilesSelected: opts.FilesSelected}, func(event Event) {
		if !isPeriodicSnapshot(event) {
			return
		}
		var envelope bookSnapshotEnvelope
		if event.Decode(&envelope) != nil {
			return
		}
		symbol := event.Symbol
		if symbol == "" {
			symbol = symbolFromPath(event.File)
		}
		if opts.Symbol != "" && symbol != opts.Symbol {
			return
		}
		bids, asks := envelope.levels()
		if len(bids) == 0 || len(asks) == 0 {
			return
		}
		bid, ask := bestBid(bids), bestAsk(asks)
		if bid <= 0 || ask <= 0 || bid > ask {
			return
		}
		// Positive uncrossed prices imply ask-bid is nonnegative and cannot
		// overflow int64. This is the same strict midpoint contract as V2.
		quote := venueMidQuote{mid: bid + (ask-bid)/2, at: event.SimTS, ordinal: event.Ordinal}
		mu.Lock()
		series[event.VenueID] = append(series[event.VenueID], quote)
		result.QuoteUpdates++
		mu.Unlock()
	})
	if err != nil {
		return nil, err
	}

	instants := make(map[int64]struct{})
	venues := make([]string, 0, len(series))
	for venue, quotes := range series {
		// All periodic snapshots for a (venue, symbol) are in one file. Its
		// physical ordinal therefore gives the persisted tie-break when two
		// publications share a simulated timestamp. Sorting by time alone would
		// make the final same-time quote an implementation accident of sort.
		sort.Slice(quotes, func(i, j int) bool {
			if quotes[i].at != quotes[j].at {
				return quotes[i].at < quotes[j].at
			}
			return quotes[i].ordinal < quotes[j].ordinal
		})
		series[venue] = quotes
		venues = append(venues, venue)
		for _, quote := range quotes {
			instants[quote.at] = struct{}{}
		}
	}
	sort.Strings(venues)
	ordered := make([]int64, 0, len(instants))
	for at := range instants {
		ordered = append(ordered, at)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })

	cursors := make(map[string]int, len(venues))
	live := make(map[string]venueMidQuote, len(venues))
	ranges := make([]float64, 0, len(ordered))
	positiveRunStart, positiveRunLast := int64(0), int64(0)
	inPositiveRun := false
	for _, at := range ordered {
		fresh := make([]dispersionQuote, 0, len(venues))
		for _, venue := range venues {
			quotes := series[venue]
			for cursors[venue] < len(quotes) && quotes[cursors[venue]].at <= at {
				live[venue] = quotes[cursors[venue]]
				cursors[venue]++
			}
			quote, ok := live[venue]
			if !ok || at-quote.at > opts.StalenessNanos {
				continue
			}
			fresh = append(fresh, dispersionQuote{venue: venue, quote: quote})
		}
		if len(fresh) < opts.MinVenues {
			result.SkippedInsufficientFresh++
			inPositiveRun = false
			continue
		}
		low, high := fresh[0].quote.mid, fresh[0].quote.mid
		for _, item := range fresh[1:] {
			if item.quote.mid < low {
				low = item.quote.mid
			}
			if item.quote.mid > high {
				high = item.quote.mid
			}
		}
		bps := 1e4 * float64(high-low) / float64(low)
		ranges = append(ranges, bps)
		result.Evaluated++
		if bps <= 0 {
			inPositiveRun = false
			continue
		}
		if !inPositiveRun {
			positiveRunStart, inPositiveRun = at, true
		}
		positiveRunLast = at
		if span := positiveRunLast - positiveRunStart; span > result.LongestPositiveRunNanos {
			result.LongestPositiveRunNanos = span
		}
	}
	result.MidpointRangeBps = Describe(ranges)
	return result, nil
}
