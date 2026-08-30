package instrument

import (
	"cmp"
	"fmt"
	"slices"

	etypes "exchange_sim/types"
)

// ExpirySchedule describes one listing family on an expiry calendar. The
// family name is descriptive metadata; it is never part of an instrument's
// economic identity.
type ExpirySchedule struct {
	Name                string `json:"name"`
	ListingIntervalNano int64  `json:"listing_interval_nano"`
	TimeToExpiryNano    int64  `json:"time_to_expiry_nano"`
	PhaseOffsetNano     int64  `json:"phase_offset_nano,omitempty"`
}

// ExpiryCalendar is immutable configuration shared by futures and option
// listers. The listers own their cursors because each policy may observe the
// same calendar at a different point in an exchange lifecycle.
type ExpiryCalendar struct {
	Schedules []ExpirySchedule `json:"schedules"`
}

func (c ExpiryCalendar) Validate() error {
	if len(c.Schedules) == 0 {
		return fmt.Errorf("expiry calendar requires at least one schedule")
	}
	seenNames := make(map[string]struct{}, len(c.Schedules))
	for _, schedule := range c.Schedules {
		if schedule.Name == "" {
			return fmt.Errorf("expiry calendar schedule name is required")
		}
		if _, exists := seenNames[schedule.Name]; exists {
			return fmt.Errorf("expiry calendar schedule %q is duplicated", schedule.Name)
		}
		seenNames[schedule.Name] = struct{}{}
		if schedule.ListingIntervalNano <= 0 || schedule.TimeToExpiryNano <= 0 {
			return fmt.Errorf("expiry calendar schedule %q requires positive interval and time to expiry", schedule.Name)
		}
		if schedule.PhaseOffsetNano < 0 || schedule.PhaseOffsetNano >= schedule.ListingIntervalNano {
			return fmt.Errorf("expiry calendar schedule %q phase is outside its listing interval", schedule.Name)
		}
	}
	return nil
}

type calendarListingRequest struct {
	scheduleName string
	index        uint64
	listingNano  int64
	expiryNano   int64
}

// calendarCursor turns schedule configuration into due requests. It returns
// a copy of the next-index map so callers can commit cursor movement only
// after any required price-dependent construction succeeds.
type calendarCursor struct {
	nextIndex map[string]uint64
}

func (c *calendarCursor) due(calendar *ExpiryCalendar, epochNano, nowNano int64) ([]calendarListingRequest, map[string]uint64, error) {
	if calendar == nil {
		return nil, nil, fmt.Errorf("expiry calendar is nil")
	}
	if err := calendar.Validate(); err != nil {
		return nil, nil, err
	}
	if c.nextIndex == nil {
		c.nextIndex = make(map[string]uint64, len(calendar.Schedules))
	}
	nextIndex := make(map[string]uint64, len(c.nextIndex))
	for name, index := range c.nextIndex {
		nextIndex[name] = index
	}
	schedules := slices.Clone(calendar.Schedules)
	slices.SortFunc(schedules, func(left, right ExpirySchedule) int {
		return cmp.Compare(left.Name, right.Name)
	})

	var requests []calendarListingRequest
	for _, schedule := range schedules {
		index := nextIndex[schedule.Name]
		for {
			listingNano, ok := scheduledNano(epochNano, schedule, index)
			if !ok {
				return nil, nil, fmt.Errorf("expiry calendar schedule %q index %d is unrepresentable", schedule.Name, index)
			}
			if listingNano > nowNano {
				break
			}
			expiryNano, ok := etypes.TryAdd(listingNano, schedule.TimeToExpiryNano)
			if !ok {
				return nil, nil, fmt.Errorf("expiry calendar schedule %q index %d expiry is unrepresentable", schedule.Name, index)
			}
			if index == ^uint64(0) {
				return nil, nil, fmt.Errorf("expiry calendar schedule %q cursor overflow", schedule.Name)
			}
			nextIndex[schedule.Name] = index + 1
			if expiryNano > nowNano {
				requests = append(requests, calendarListingRequest{
					scheduleName: schedule.Name,
					index:        index,
					listingNano:  listingNano,
					expiryNano:   expiryNano,
				})
			}
			index++
		}
	}
	slices.SortFunc(requests, func(left, right calendarListingRequest) int {
		if byListing := cmp.Compare(left.listingNano, right.listingNano); byListing != 0 {
			return byListing
		}
		if byExpiry := cmp.Compare(left.expiryNano, right.expiryNano); byExpiry != 0 {
			return byExpiry
		}
		if bySchedule := cmp.Compare(left.scheduleName, right.scheduleName); bySchedule != 0 {
			return bySchedule
		}
		return cmp.Compare(left.index, right.index)
	})
	return requests, nextIndex, nil
}

func scheduledNano(epochNano int64, schedule ExpirySchedule, index uint64) (int64, bool) {
	maxInt64 := int64(^uint64(0) >> 1)
	if index > uint64(maxInt64/schedule.ListingIntervalNano) {
		return 0, false
	}
	offset := int64(index) * schedule.ListingIntervalNano
	var ok bool
	if offset, ok = etypes.TryAdd(offset, schedule.PhaseOffsetNano); !ok {
		return 0, false
	}
	return etypes.TryAdd(epochNano, offset)
}
