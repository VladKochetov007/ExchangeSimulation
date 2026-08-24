package analysis

import (
	"fmt"
	"sort"
)

// PerpSignalOptions declares the observed public perpetual signal whose
// availability and variation are audited. A numeric zero remains a present
// value: missing values are represented only by a nil decoded field.
type PerpSignalOptions struct {
	Symbol         string
	RequiredVenues []string
}

// PerpSignalVenue is one venue's persisted public mark/funding evidence.
// Pointer-valued first/last observations distinguish a present zero from no
// observed value on the JSON artifact wire.
type PerpSignalVenue struct {
	VenueID                 string `json:"venue_id"`
	MarkUpdates             int    `json:"mark_updates"`
	FundingRateUpdates      int    `json:"funding_rate_updates"`
	FundingSettlementEvents int    `json:"funding_settlement_events"`
	MarkAvailable           bool   `json:"mark_available"`
	FundingRateAvailable    bool   `json:"funding_rate_available"`
	FirstMark               *int64 `json:"first_mark"`
	FirstIndex              *int64 `json:"first_index"`
	LastMark                *int64 `json:"last_mark"`
	LastIndex               *int64 `json:"last_index"`
	FirstFundingRate        *int64 `json:"first_funding_rate"`
	LastFundingRate         *int64 `json:"last_funding_rate"`
	DistinctMarkIndexPairs  int    `json:"distinct_mark_index_pairs"`
	DistinctFundingRates    int    `json:"distinct_funding_rates"`
}

// PerpSignalAudit describes raw public mark/funding observations without
// inferring a causal effect or treating a numeric price/rate as availability.
type PerpSignalAudit struct {
	Symbol                     string            `json:"symbol"`
	Venues                     []PerpSignalVenue `json:"venues"`
	PooledDistinctMarkPairs    int               `json:"pooled_distinct_mark_index_pairs"`
	PooledDistinctFundingRates int               `json:"pooled_distinct_funding_rates"`
	InvalidMarkRecords         int               `json:"invalid_mark_records"`
	InvalidFundingRecords      int               `json:"invalid_funding_records"`
	MissingRequiredVenues      []string          `json:"missing_required_venues"`
	Ready                      bool              `json:"ready"`
	Valid                      bool              `json:"valid"`
}

type perpSignalMarkPayload struct {
	MarkPrice  *int64 `json:"mark_price"`
	IndexPrice *int64 `json:"index_price"`
}

type perpSignalFundingPayload struct {
	Rate *int64 `json:"rate"`
}

type perpSignalAccumulator struct {
	venue PerpSignalVenue
	pairs map[[2]int64]struct{}
	rates map[int64]struct{}
}

func (a *perpSignalAccumulator) addMark(mark, index int64) {
	if !a.venue.MarkAvailable {
		a.venue.MarkAvailable = true
		a.venue.FirstMark = int64Pointer(mark)
		a.venue.FirstIndex = int64Pointer(index)
	}
	a.venue.MarkUpdates++
	a.venue.LastMark = int64Pointer(mark)
	a.venue.LastIndex = int64Pointer(index)
	a.pairs[[2]int64{mark, index}] = struct{}{}
}

func (a *perpSignalAccumulator) addFunding(rate int64) {
	if !a.venue.FundingRateAvailable {
		a.venue.FundingRateAvailable = true
		a.venue.FirstFundingRate = int64Pointer(rate)
	}
	a.venue.FundingRateUpdates++
	a.venue.LastFundingRate = int64Pointer(rate)
	a.rates[rate] = struct{}{}
}

func int64Pointer(value int64) *int64 { return &value }

// MeasurePerpSignals independently counts persisted public mark and funding
// observations. It is intentionally not a basis or funding-semantic metric:
// a short run may legally publish signal updates yet have no funding
// settlement. Required JSON scalar fields are pointers so a present zero is
// not confused with a missing field.
func (r *Run) MeasurePerpSignals(opts PerpSignalOptions) (*PerpSignalAudit, error) {
	if opts.Symbol == "" {
		return nil, fmt.Errorf("perp signal audit: symbol is required")
	}
	byVenue := make(map[string]*perpSignalAccumulator)
	accumulator := func(venue string) *perpSignalAccumulator {
		item := byVenue[venue]
		if item == nil {
			item = &perpSignalAccumulator{
				venue: PerpSignalVenue{VenueID: venue},
				pairs: make(map[[2]int64]struct{}),
				rates: make(map[int64]struct{}),
			}
			byVenue[venue] = item
		}
		return item
	}

	result := &PerpSignalAudit{Symbol: opts.Symbol, MissingRequiredVenues: make([]string, 0)}
	err := r.Scan(ScanOptions{Events: []string{"mark_price_update", "funding_rate_update", "balance_change"}, Workers: 1}, func(event Event) {
		switch event.Name {
		case "mark_price_update":
			if event.Symbol != opts.Symbol {
				return
			}
			var payload perpSignalMarkPayload
			if err := event.Decode(&payload); err != nil || payload.MarkPrice == nil || payload.IndexPrice == nil {
				result.InvalidMarkRecords++
				return
			}
			accumulator(event.VenueID).addMark(*payload.MarkPrice, *payload.IndexPrice)
		case "funding_rate_update":
			if event.Symbol != opts.Symbol {
				return
			}
			var payload perpSignalFundingPayload
			if err := event.Decode(&payload); err != nil || payload.Rate == nil {
				result.InvalidFundingRecords++
				return
			}
			accumulator(event.VenueID).addFunding(*payload.Rate)
		case "balance_change":
			var payload balanceChangeRecord
			if event.Decode(&payload) != nil || payload.Symbol != opts.Symbol || payload.Reason != "funding_settlement" {
				return
			}
			accumulator(event.VenueID).venue.FundingSettlementEvents++
		}
	})
	if err != nil {
		return nil, fmt.Errorf("perp signal audit: scan: %w", err)
	}

	pooledPairs := make(map[[2]int64]struct{})
	pooledRates := make(map[int64]struct{})
	for _, item := range byVenue {
		item.venue.DistinctMarkIndexPairs = len(item.pairs)
		item.venue.DistinctFundingRates = len(item.rates)
		result.Venues = append(result.Venues, item.venue)
		for pair := range item.pairs {
			pooledPairs[pair] = struct{}{}
		}
		for rate := range item.rates {
			pooledRates[rate] = struct{}{}
		}
	}
	sort.Slice(result.Venues, func(i, j int) bool { return result.Venues[i].VenueID < result.Venues[j].VenueID })
	result.PooledDistinctMarkPairs = len(pooledPairs)
	result.PooledDistinctFundingRates = len(pooledRates)
	for _, venue := range opts.RequiredVenues {
		item := byVenue[venue]
		if item == nil || !item.venue.MarkAvailable || !item.venue.FundingRateAvailable {
			result.MissingRequiredVenues = append(result.MissingRequiredVenues, venue)
		}
	}
	sort.Strings(result.MissingRequiredVenues)
	result.Valid = result.InvalidMarkRecords == 0 && result.InvalidFundingRecords == 0
	result.Ready = result.Valid && len(result.MissingRequiredVenues) == 0 &&
		result.PooledDistinctMarkPairs >= 2 && result.PooledDistinctFundingRates >= 2
	return result, nil
}
