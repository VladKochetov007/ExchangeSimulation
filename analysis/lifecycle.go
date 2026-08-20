package analysis

import (
	"sort"
	"sync"
)

// LifecycleOptions selects what a lifecycle census counts.
type LifecycleOptions struct {
	Files         []string
	FilesSelected bool
}

// InstrumentLifecycle is one contract's listing and settlement.
type InstrumentLifecycle struct {
	VenueID     string `json:"venue_id"`
	Symbol      string `json:"symbol"`
	ListedNano  int64  `json:"listed_nano"`
	SettledNano int64  `json:"settled_nano"`
}

// FundingSchedule is one venue's realised funding settlement times.
type FundingSchedule struct {
	VenueID string `json:"venue_id"`
	// Settlements is how many distinct instants funding was charged at.
	Settlements int `json:"settlements"`
	// PeriodSeconds is the median gap between them, which is the period the
	// venue actually settled on rather than the one it was configured with.
	PeriodSeconds float64 `json:"period_seconds"`
	Instants      []int64 `json:"instants"`
}

// Lifecycle is the census of what happened to contracts and funding over a run.
//
// It answers the question a viability corridor cannot: whether the market was
// asked to survive anything. A population that never lists, never expires and
// never charges funding can hold every book alive trivially.
type Lifecycle struct {
	Listings    map[string]int `json:"listings"`
	Settlements map[string]int `json:"settlements"`
	// SettlementRounds is how many distinct instants contracts settled at,
	// which is the number of completed lifecycle cycles a run contains.
	SettlementRounds int `json:"settlement_rounds"`
	// ListingRounds is the same for listings.
	ListingRounds int `json:"listing_rounds"`
	// SettlementRoundsByVenue is the count each venue saw. A claim about how
	// many cycles a market survived is per venue: two venues expiring on
	// offset schedules produce more distinct instants between them than either
	// one lived through.
	SettlementRoundsByVenue map[string]int `json:"settlement_rounds_by_venue"`
	ListingRoundsByVenue    map[string]int `json:"listing_rounds_by_venue"`

	Funding []FundingSchedule `json:"funding"`
	// FundingIntersections counts the instants at which each number of venues
	// settled funding together, keyed by that number. A population whose venues
	// all settle on one period has every instant under the highest key, and
	// nothing to arbitrage between schedules.
	FundingIntersections map[int]int `json:"funding_intersections"`

	Contracts []InstrumentLifecycle `json:"contracts,omitempty"`
}

// MeasureLifecycle counts listings, settlements and funding charges.
func (r *Run) MeasureLifecycle(opts LifecycleOptions) (*Lifecycle, error) {
	type instrumentPayload struct {
		Symbol         string `json:"symbol"`
		InstrumentType string `json:"instrument_type"`
	}
	type balancePayload struct {
		Reason    string `json:"reason"`
		Timestamp int64  `json:"timestamp"`
		Symbol    string `json:"symbol"`
	}

	var mu sync.Mutex
	result := &Lifecycle{
		Listings:             make(map[string]int),
		Settlements:          make(map[string]int),
		FundingIntersections: make(map[int]int),
	}
	listedAt := make(map[string]int64)
	settledAt := make(map[string]int64)
	listingInstants := make(map[int64]struct{})
	settlementInstants := make(map[int64]struct{})
	listingByVenue := make(map[string]map[int64]struct{})
	settlementByVenue := make(map[string]map[int64]struct{})
	fundingInstants := make(map[string]map[int64]struct{})

	scan := ScanOptions{
		Events:        []string{"instrument_listed", "instrument_settled", "balance_change"},
		Files:         opts.Files,
		FilesSelected: opts.FilesSelected,
	}
	if err := r.Scan(scan, func(event Event) {
		switch event.Name {
		case "instrument_listed", "instrument_settled":
			var payload instrumentPayload
			if event.Decode(&payload) != nil || payload.Symbol == "" {
				return
			}
			kind := payload.InstrumentType
			if kind == "" {
				kind = "UNKNOWN"
			}
			key := event.VenueID + " " + payload.Symbol
			mu.Lock()
			if event.Name == "instrument_listed" {
				result.Listings[kind]++
				listingInstants[event.SimTS] = struct{}{}
				if listingByVenue[event.VenueID] == nil {
					listingByVenue[event.VenueID] = make(map[int64]struct{})
				}
				listingByVenue[event.VenueID][event.SimTS] = struct{}{}
				if _, seen := listedAt[key]; !seen {
					listedAt[key] = event.SimTS
				}
			} else {
				result.Settlements[kind]++
				settlementInstants[event.SimTS] = struct{}{}
				if settlementByVenue[event.VenueID] == nil {
					settlementByVenue[event.VenueID] = make(map[int64]struct{})
				}
				settlementByVenue[event.VenueID][event.SimTS] = struct{}{}
				settledAt[key] = event.SimTS
			}
			mu.Unlock()
		case "balance_change":
			var payload balancePayload
			if event.Decode(&payload) != nil || payload.Reason != "funding_settlement" {
				return
			}
			instant := payload.Timestamp
			if instant == 0 {
				instant = event.SimTS
			}
			mu.Lock()
			if fundingInstants[event.VenueID] == nil {
				fundingInstants[event.VenueID] = make(map[int64]struct{})
			}
			fundingInstants[event.VenueID][instant] = struct{}{}
			mu.Unlock()
		}
	}); err != nil {
		return nil, err
	}

	result.ListingRounds = len(listingInstants)
	result.SettlementRounds = len(settlementInstants)
	result.ListingRoundsByVenue = make(map[string]int, len(listingByVenue))
	for venue, instants := range listingByVenue {
		result.ListingRoundsByVenue[venue] = len(instants)
	}
	result.SettlementRoundsByVenue = make(map[string]int, len(settlementByVenue))
	for venue, instants := range settlementByVenue {
		result.SettlementRoundsByVenue[venue] = len(instants)
	}
	for key, listed := range listedAt {
		venue, symbol := splitBookKey(key)
		result.Contracts = append(result.Contracts, InstrumentLifecycle{
			VenueID: venue, Symbol: symbol, ListedNano: listed, SettledNano: settledAt[key],
		})
	}
	sort.Slice(result.Contracts, func(i, j int) bool {
		if result.Contracts[i].VenueID != result.Contracts[j].VenueID {
			return result.Contracts[i].VenueID < result.Contracts[j].VenueID
		}
		return result.Contracts[i].Symbol < result.Contracts[j].Symbol
	})

	venues := make([]string, 0, len(fundingInstants))
	for venue := range fundingInstants {
		venues = append(venues, venue)
	}
	sort.Strings(venues)
	perInstant := make(map[int64]int)
	for _, venue := range venues {
		instants := make([]int64, 0, len(fundingInstants[venue]))
		for instant := range fundingInstants[venue] {
			instants = append(instants, instant)
			perInstant[instant]++
		}
		sort.Slice(instants, func(i, j int) bool { return instants[i] < instants[j] })
		result.Funding = append(result.Funding, FundingSchedule{
			VenueID: venue, Settlements: len(instants),
			PeriodSeconds: medianGapSeconds(instants), Instants: instants,
		})
	}
	for _, venuesTogether := range perInstant {
		result.FundingIntersections[venuesTogether]++
	}
	return result, nil
}

// medianGapSeconds is the median spacing between consecutive instants, which
// reports the period a venue settled on rather than the one it was told to.
func medianGapSeconds(instants []int64) float64 {
	if len(instants) < 2 {
		return 0
	}
	gaps := make([]float64, 0, len(instants)-1)
	for i := 1; i < len(instants); i++ {
		gaps = append(gaps, float64(instants[i]-instants[i-1])/1e9)
	}
	sort.Float64s(gaps)
	return gaps[len(gaps)/2]
}

// splitBookKey undoes the venue-and-symbol join used for map keys.
func splitBookKey(key string) (string, string) {
	for i := 0; i < len(key); i++ {
		if key[i] == ' ' {
			return key[:i], key[i+1:]
		}
	}
	return key, ""
}
