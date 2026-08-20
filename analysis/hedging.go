package analysis

import (
	"sort"
	"sync"
)

// HedgingOptions selects the book a hedge is executed in.
type HedgingOptions struct {
	// Symbol is the underlying the dealers hedge in.
	Symbol string
	Files  []string
	// FilesSelected marks that the caller performed a selection.
	FilesSelected bool
	// Roles, when non-empty, keeps only these participant classes.
	Roles []string
}

// HedgeProfile is one participant's hedging behaviour in the underlying.
type HedgeProfile struct {
	VenueID  string `json:"venue_id"`
	ClientID uint64 `json:"client_id"`
	Role     string `json:"role"`
	// Trades and Qty are the hedges that reached the book.
	Trades int   `json:"trades"`
	Qty    int64 `json:"qty"`
	// MedianGapSeconds is the median spacing between hedges, which separates a
	// desk rebalancing on a schedule from one rebalancing on a band: the first
	// has a spacing, the second has whatever the market gave it.
	MedianGapSeconds float64 `json:"median_gap_seconds"`
	// GapSpreadSeconds is the interquartile spread of that spacing. A timed
	// policy is near zero here however often it trades; a banded one is not.
	GapSpreadSeconds float64 `json:"gap_spread_seconds"`
	// BuyShare is the fraction of hedges that bought. A desk covering a
	// one-sided option book sits far from a half.
	BuyShare float64 `json:"buy_share"`
}

// Hedging compares how participants hedge in one book.
//
// It exists because a population can be configured with three hedging policies
// and run as though it had one: the configuration says what a dealer was told
// to do and this says what it did.
type Hedging struct {
	Profiles []HedgeProfile `json:"profiles"`
}

// MeasureHedging reads hedging trades in one book, by participant.
func (r *Run) MeasureHedging(opts HedgingOptions) (*Hedging, error) {
	type fillPayload struct {
		Symbol string `json:"symbol"`
		Qty    int64  `json:"qty"`
		Side   string `json:"side"`
		Role   string `json:"role"`
	}
	keep := make(map[string]struct{}, len(opts.Roles))
	for _, role := range opts.Roles {
		keep[role] = struct{}{}
	}

	var mu sync.Mutex
	type accumulator struct {
		role   string
		trades int
		qty    int64
		buys   int
		stamps []int64
	}
	perClient := make(map[Participant]*accumulator)

	scan := ScanOptions{Events: []string{"OrderFill"}, Files: opts.Files, FilesSelected: opts.FilesSelected}
	if err := r.Scan(scan, func(event Event) {
		var fill fillPayload
		if event.Decode(&fill) != nil || fill.Qty <= 0 {
			return
		}
		// A hedge is a taker fill: the desk crosses to get flat rather than
		// waiting, which is what makes hedging expensive.
		if fill.Role != "taker" {
			return
		}
		symbol := event.Symbol
		if symbol == "" {
			symbol = fill.Symbol
		}
		if symbol == "" {
			symbol = symbolFromSpotFile(event.File)
		}
		if opts.Symbol != "" && symbol != opts.Symbol {
			return
		}
		role := r.Role(event.VenueID, event.ClientID)
		if len(keep) > 0 {
			if _, wanted := keep[role]; !wanted {
				return
			}
		}
		key := Participant{event.VenueID, event.ClientID}
		mu.Lock()
		state := perClient[key]
		if state == nil {
			state = &accumulator{role: role}
			perClient[key] = state
		}
		state.trades++
		state.qty += fill.Qty
		if fill.Side == "BUY" {
			state.buys++
		}
		state.stamps = append(state.stamps, event.SimTS)
		mu.Unlock()
	}); err != nil {
		return nil, err
	}

	result := &Hedging{}
	for participant, state := range perClient {
		sort.Slice(state.stamps, func(i, j int) bool { return state.stamps[i] < state.stamps[j] })
		median, spread := gapStatistics(state.stamps)
		profile := HedgeProfile{
			VenueID: participant.VenueID, ClientID: participant.ClientID, Role: state.role,
			Trades: state.trades, Qty: state.qty,
			MedianGapSeconds: median, GapSpreadSeconds: spread,
		}
		if state.trades > 0 {
			profile.BuyShare = float64(state.buys) / float64(state.trades)
		}
		result.Profiles = append(result.Profiles, profile)
	}
	sort.Slice(result.Profiles, func(i, j int) bool {
		if result.Profiles[i].VenueID != result.Profiles[j].VenueID {
			return result.Profiles[i].VenueID < result.Profiles[j].VenueID
		}
		return result.Profiles[i].ClientID < result.Profiles[j].ClientID
	})
	return result, nil
}

// gapStatistics returns the median spacing between stamps and its
// interquartile spread, both in seconds.
func gapStatistics(stamps []int64) (float64, float64) {
	if len(stamps) < 3 {
		return 0, 0
	}
	gaps := make([]float64, 0, len(stamps)-1)
	for i := 1; i < len(stamps); i++ {
		gaps = append(gaps, float64(stamps[i]-stamps[i-1])/1e9)
	}
	sort.Float64s(gaps)
	return gaps[len(gaps)/2], gaps[len(gaps)*3/4] - gaps[len(gaps)/4]
}
