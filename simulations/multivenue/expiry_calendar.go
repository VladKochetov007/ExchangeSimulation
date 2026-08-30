package multivenue

import (
	"time"

	"exchange_sim/instrument"
)

// CompressedR2ExpiryCalendar is the registered calendar for the R2 successor
// population. Values are listing cadence / time-to-expiry, not a rolling tenor
// ladder. Callers opt in by assigning the returned value to Config's
// R2ExpiryCalendar; historical configurations remain on their legacy policy.
func CompressedR2ExpiryCalendar() instrument.ExpiryCalendar {
	return instrument.ExpiryCalendar{Schedules: []instrument.ExpirySchedule{
		{Name: "short", ListingIntervalNano: int64(time.Hour), TimeToExpiryNano: int64(2 * time.Hour)},
		{Name: "medium", ListingIntervalNano: int64(3 * time.Hour), TimeToExpiryNano: int64(6 * time.Hour)},
		{Name: "long", ListingIntervalNano: int64(6 * time.Hour), TimeToExpiryNano: int64(12 * time.Hour)},
	}}
}
