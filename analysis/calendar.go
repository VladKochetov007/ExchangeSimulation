package analysis

import (
	"fmt"
	"sort"
	"sync"
)

// CalendarOptions selects the lifecycle evidence used by the calendar census.
type CalendarOptions struct {
	Files         []string
	FilesSelected bool
}

// CalendarVenueAudit is the realized derivative calendar for one venue.
// Expiries are sets rather than family-labelled rows: overlapping listing
// policies describe one economic contract and must not be counted twice.
type CalendarVenueAudit struct {
	VenueID string `json:"venue_id"`

	FuturesExpiryNanos []int64           `json:"futures_expiry_nanos"`
	OptionExpiryNanos  []int64           `json:"option_expiry_nanos"`
	SharedExpiryNanos  []int64           `json:"shared_expiry_nanos"`
	ListingTimeline    []CalendarListing `json:"listing_timeline"`

	FuturesListed  int `json:"futures_listed"`
	OptionsListed  int `json:"options_listed"`
	FuturesSettled int `json:"futures_settled"`
	OptionsSettled int `json:"options_settled"`

	FutureExpiryCycles int `json:"future_expiry_cycles"`
	OptionExpiryCycles int `json:"option_expiry_cycles"`

	DuplicateFutureListings    int `json:"duplicate_future_listings"`
	DuplicateOptionListings    int `json:"duplicate_option_listings"`
	DuplicateFutureSettlements int `json:"duplicate_future_settlements"`
	DuplicateOptionSettlements int `json:"duplicate_option_settlements"`
	SettlementWithoutListing   int `json:"settlement_without_listing"`
	SettlementBeforeListing    int `json:"settlement_before_listing"`
	MalformedDerivativeEvents  int `json:"malformed_derivative_events"`

	MaxSimultaneousFutureExpiries int `json:"max_simultaneous_future_expiries"`
	MaxSimultaneousOptionExpiries int `json:"max_simultaneous_option_expiries"`
}

// CalendarListing records the realized first-listing instant and cardinality
// for one economic expiry. Schedule-family identity is deliberately absent:
// overlapping families must converge on one future and one option chain.
type CalendarListing struct {
	ExpiryNano              int64 `json:"expiry_nano"`
	FutureFirstListedAtNano int64 `json:"future_first_listed_at_nano"`
	OptionFirstListedAtNano int64 `json:"option_first_listed_at_nano"`
	FutureContractCount     int   `json:"future_contract_count"`
	OptionContractCount     int   `json:"option_contract_count"`
}

// CalendarAudit is the deterministic, analyzer-owned attestation input for
// the R2 calendar gate. It records realized sets and lifecycle defects without
// inferring a schedule family from a symbol.
type CalendarAudit struct {
	SchemaVersion int    `json:"schema_version"`
	Contract      string `json:"contract"`

	Venues             []CalendarVenueAudit `json:"venues"`
	FuturesExpiryNanos []int64              `json:"futures_expiry_nanos"`
	OptionExpiryNanos  []int64              `json:"option_expiry_nanos"`
	SharedExpiryNanos  []int64              `json:"shared_expiry_nanos"`

	ListingEvents             int `json:"listing_events"`
	SettlementEvents          int `json:"settlement_events"`
	DuplicateListings         int `json:"duplicate_listings"`
	DuplicateSettlements      int `json:"duplicate_settlements"`
	MalformedDerivativeEvents int `json:"malformed_derivative_events"`
}

type calendarLifecycleEvent struct {
	venueID string
	symbol  string
	kind    string
	expiry  int64
	at      int64
	listed  bool
	file    string
	ordinal int64
}

type calendarEventPosition struct {
	at      int64
	file    string
	ordinal int64
}

type calendarInstrumentKey struct {
	venue  string
	kind   string
	symbol string
}

type calendarListingTimeline struct {
	listing    CalendarListing
	futureSeen bool
	optionSeen bool
}

// MeasureCalendar reconstructs derivative listing, coexistence and
// settlement from exchange-owned lifecycle announcements. It is intentionally
// independent of symbol grammar except for the announcement's typed expiry.
func (r *Run) MeasureCalendar(opts CalendarOptions) (*CalendarAudit, error) {
	var mu sync.Mutex
	events := make([]calendarLifecycleEvent, 0)
	scan := ScanOptions{
		Events:        []string{"instrument_listed", "instrument_settled"},
		Files:         opts.Files,
		FilesSelected: opts.FilesSelected,
	}
	if err := r.Scan(scan, func(event Event) {
		var payload struct {
			Symbol         string `json:"symbol"`
			InstrumentType string `json:"instrument_type"`
			ExpiryNano     int64  `json:"expiry_nano"`
		}
		if event.Decode(&payload) != nil {
			return
		}
		if payload.InstrumentType != "FUTURE" && payload.InstrumentType != "OPTION" {
			return
		}
		mu.Lock()
		events = append(events, calendarLifecycleEvent{
			venueID: event.VenueID, symbol: payload.Symbol, kind: payload.InstrumentType,
			expiry: payload.ExpiryNano, at: event.SimTS,
			listed: event.Name == "instrument_listed", file: event.File, ordinal: event.Ordinal,
		})
		mu.Unlock()
	}); err != nil {
		return nil, err
	}
	filesAtTimestamp := make(map[string]string)
	for _, event := range events {
		key := fmt.Sprintf("%s\x00%d", event.venueID, event.at)
		if previousFile, exists := filesAtTimestamp[key]; exists && previousFile != event.file {
			return nil, fmt.Errorf("calendar: ambiguous same-timestamp lifecycle records span %q and %q", previousFile, event.file)
		}
		filesAtTimestamp[key] = event.file
	}
	sort.Slice(events, func(i, j int) bool {
		left, right := events[i], events[j]
		if left.at != right.at {
			return left.at < right.at
		}
		if left.venueID != right.venueID {
			return left.venueID < right.venueID
		}
		if left.file != right.file {
			return left.file < right.file
		}
		return left.ordinal < right.ordinal
	})

	type venueState struct {
		audit           CalendarVenueAudit
		listed          map[calendarInstrumentKey]calendarEventPosition
		settled         map[calendarInstrumentKey]bool
		active          map[string]map[int64]int
		settledExpiries map[string]map[int64]bool
		listingTimeline map[int64]calendarListingTimeline
	}
	states := make(map[string]*venueState)
	stateFor := func(venueID string) *venueState {
		state := states[venueID]
		if state != nil {
			return state
		}
		state = &venueState{
			audit:           CalendarVenueAudit{VenueID: venueID},
			listed:          make(map[calendarInstrumentKey]calendarEventPosition),
			settled:         make(map[calendarInstrumentKey]bool),
			active:          map[string]map[int64]int{"FUTURE": {}, "OPTION": {}},
			settledExpiries: map[string]map[int64]bool{"FUTURE": {}, "OPTION": {}},
			listingTimeline: make(map[int64]calendarListingTimeline),
		}
		states[venueID] = state
		return state
	}

	globalFutures := make(map[int64]struct{})
	globalOptions := make(map[int64]struct{})
	firstListings := make(map[calendarInstrumentKey]calendarEventPosition)
	for _, event := range events {
		if event.symbol == "" || event.expiry <= 0 || !event.listed {
			continue
		}
		key := calendarInstrumentKey{venue: event.venueID, kind: event.kind, symbol: event.symbol}
		if _, exists := firstListings[key]; !exists {
			firstListings[key] = calendarEventPosition{at: event.at, file: event.file, ordinal: event.ordinal}
		}
	}
	for _, event := range events {
		state := stateFor(event.venueID)
		if event.symbol == "" || event.expiry <= 0 {
			state.audit.MalformedDerivativeEvents++
			continue
		}
		key := calendarInstrumentKey{venue: event.venueID, kind: event.kind, symbol: event.symbol}
		if event.listed {
			if _, exists := state.listed[key]; exists {
				if event.kind == "FUTURE" {
					state.audit.DuplicateFutureListings++
				} else {
					state.audit.DuplicateOptionListings++
				}
				continue
			}
			state.listed[key] = calendarEventPosition{at: event.at, file: event.file, ordinal: event.ordinal}
			state.active[event.kind][event.expiry]++
			timeline := state.listingTimeline[event.expiry]
			timeline.listing.ExpiryNano = event.expiry
			if event.kind == "FUTURE" {
				timeline.listing.FutureContractCount++
				if !timeline.futureSeen {
					timeline.listing.FutureFirstListedAtNano = event.at
					timeline.futureSeen = true
				}
			} else {
				timeline.listing.OptionContractCount++
				if !timeline.optionSeen {
					timeline.listing.OptionFirstListedAtNano = event.at
					timeline.optionSeen = true
				}
			}
			state.listingTimeline[event.expiry] = timeline
			if event.kind == "FUTURE" {
				state.audit.FuturesListed++
				globalFutures[event.expiry] = struct{}{}
			} else {
				state.audit.OptionsListed++
				globalOptions[event.expiry] = struct{}{}
			}
			continue
		}

		listedAt, listed := firstListings[key]
		if !listed {
			state.audit.SettlementWithoutListing++
			continue
		}
		if calendarPositionPrecedes(calendarEventPosition{at: event.at, file: event.file, ordinal: event.ordinal}, listedAt) {
			state.audit.SettlementBeforeListing++
			continue
		}
		if state.settled[key] {
			if event.kind == "FUTURE" {
				state.audit.DuplicateFutureSettlements++
			} else {
				state.audit.DuplicateOptionSettlements++
			}
			continue
		}
		state.settled[key] = true
		state.active[event.kind][event.expiry]--
		state.settledExpiries[event.kind][event.expiry] = true
		if event.kind == "FUTURE" {
			state.audit.FuturesSettled++
		} else {
			state.audit.OptionsSettled++
		}
	}

	// Re-scan the ordered lifecycle after the identity pass to measure peak
	// maturity coexistence using complete event ordering at each timestamp.
	for venueID, state := range states {
		active := map[string]map[int64]int{"FUTURE": {}, "OPTION": {}}
		seenListed := make(map[calendarInstrumentKey]struct{})
		seenSettled := make(map[calendarInstrumentKey]struct{})
		for _, event := range events {
			if event.venueID != venueID || event.symbol == "" || event.expiry <= 0 {
				continue
			}
			key := calendarInstrumentKey{venue: event.venueID, kind: event.kind, symbol: event.symbol}
			if event.listed {
				if _, seen := seenListed[key]; seen {
					continue
				}
				seenListed[key] = struct{}{}
				active[event.kind][event.expiry]++
			} else if _, seen := seenSettled[key]; !seen {
				seenSettled[key] = struct{}{}
				if _, listed := state.listed[key]; !listed {
					continue
				}
				active[event.kind][event.expiry]--
			}
			futureCount, optionCount := 0, 0
			for _, count := range active["FUTURE"] {
				if count > 0 {
					futureCount++
				}
			}
			for _, count := range active["OPTION"] {
				if count > 0 {
					optionCount++
				}
			}
			if futureCount > state.audit.MaxSimultaneousFutureExpiries {
				state.audit.MaxSimultaneousFutureExpiries = futureCount
			}
			if optionCount > state.audit.MaxSimultaneousOptionExpiries {
				state.audit.MaxSimultaneousOptionExpiries = optionCount
			}
		}
	}

	venueIDs := make([]string, 0, len(states))
	for venueID := range states {
		venueIDs = append(venueIDs, venueID)
	}
	sort.Strings(venueIDs)
	result := &CalendarAudit{SchemaVersion: 2, Contract: "calendar-audit-v2"}
	for _, venueID := range venueIDs {
		state := states[venueID]
		state.audit.FuturesExpiryNanos = expirySetFromEvents(events, venueID, "FUTURE", true)
		state.audit.OptionExpiryNanos = expirySetFromEvents(events, venueID, "OPTION", true)
		state.audit.SharedExpiryNanos = intersectExpiries(state.audit.FuturesExpiryNanos, state.audit.OptionExpiryNanos)
		timelineExpiries := make([]int64, 0, len(state.listingTimeline))
		for expiry := range state.listingTimeline {
			timelineExpiries = append(timelineExpiries, expiry)
		}
		sort.Slice(timelineExpiries, func(i, j int) bool { return timelineExpiries[i] < timelineExpiries[j] })
		state.audit.ListingTimeline = make([]CalendarListing, 0, len(timelineExpiries))
		for _, expiry := range timelineExpiries {
			state.audit.ListingTimeline = append(state.audit.ListingTimeline, state.listingTimeline[expiry].listing)
		}
		state.audit.FutureExpiryCycles = len(state.settledExpiries["FUTURE"])
		state.audit.OptionExpiryCycles = len(state.settledExpiries["OPTION"])
		result.Venues = append(result.Venues, state.audit)
	}
	// The counters above intentionally separate terminal lifecycle events from
	// listings; per-venue duplicate and malformed counts are folded into the
	// top-level audit for a single fail-closed predicate.
	for _, venue := range result.Venues {
		result.ListingEvents += venue.FuturesListed + venue.OptionsListed + venue.DuplicateFutureListings + venue.DuplicateOptionListings
		result.SettlementEvents += venue.FuturesSettled + venue.OptionsSettled + venue.DuplicateFutureSettlements + venue.DuplicateOptionSettlements
		result.DuplicateListings += venue.DuplicateFutureListings + venue.DuplicateOptionListings
		result.DuplicateSettlements += venue.DuplicateFutureSettlements + venue.DuplicateOptionSettlements
		result.MalformedDerivativeEvents += venue.MalformedDerivativeEvents
	}
	result.FuturesExpiryNanos = sortedExpirySet(globalFutures)
	result.OptionExpiryNanos = sortedExpirySet(globalOptions)
	result.SharedExpiryNanos = intersectExpiries(result.FuturesExpiryNanos, result.OptionExpiryNanos)
	return result, nil
}

func calendarPositionPrecedes(left, right calendarEventPosition) bool {
	if left.at != right.at {
		return left.at < right.at
	}
	if left.file != right.file {
		return false
	}
	return left.ordinal < right.ordinal
}

func expirySetFromEvents(events []calendarLifecycleEvent, venueID, kind string, listedOnly bool) []int64 {
	set := make(map[int64]struct{})
	for _, event := range events {
		if event.venueID == venueID && event.kind == kind && event.expiry > 0 && (!listedOnly || event.listed) {
			set[event.expiry] = struct{}{}
		}
	}
	return sortedExpirySet(set)
}

func sortedExpirySet(set map[int64]struct{}) []int64 {
	values := make([]int64, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	return values
}

func intersectExpiries(left, right []int64) []int64 {
	set := make(map[int64]struct{}, len(right))
	for _, expiry := range right {
		set[expiry] = struct{}{}
	}
	shared := make(map[int64]struct{})
	for _, expiry := range left {
		if _, exists := set[expiry]; exists {
			shared[expiry] = struct{}{}
		}
	}
	return sortedExpirySet(shared)
}
