package multivenue

import (
	"testing"
	"time"
)

func TestCompressedR2ExpiryCalendarIsExplicitAndOrderedByFamily(t *testing.T) {
	calendar := CompressedR2ExpiryCalendar()
	if err := calendar.Validate(); err != nil {
		t.Fatal(err)
	}
	want := []struct {
		name           string
		interval, lead time.Duration
	}{
		{name: "short", interval: time.Hour, lead: 2 * time.Hour},
		{name: "medium", interval: 3 * time.Hour, lead: 6 * time.Hour},
		{name: "long", interval: 6 * time.Hour, lead: 12 * time.Hour},
	}
	if len(calendar.Schedules) != len(want) {
		t.Fatalf("schedule count = %d, want %d", len(calendar.Schedules), len(want))
	}
	for index, schedule := range calendar.Schedules {
		if schedule.Name != want[index].name || time.Duration(schedule.ListingIntervalNano) != want[index].interval || time.Duration(schedule.TimeToExpiryNano) != want[index].lead {
			t.Fatalf("schedule %d = %+v, want %s %s/%s", index, schedule, want[index].name, want[index].interval, want[index].lead)
		}
	}
}
