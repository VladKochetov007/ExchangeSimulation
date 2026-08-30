package multivenue

import (
	"context"
	"encoding/json"
	"reflect"
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

func TestConfigRoundTripsR2ExpiryCalendar(t *testing.T) {
	calendar := CompressedR2ExpiryCalendar()
	original := Config{R2ExpiryCalendar: &calendar}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.R2ExpiryCalendar == nil || !reflect.DeepEqual(*decoded.R2ExpiryCalendar, calendar) {
		t.Fatalf("decoded calendar = %#v, want %#v", decoded.R2ExpiryCalendar, calendar)
	}
}

func TestNewSimActivatesR2CalendarAndSharesDerivativeExpiries(t *testing.T) {
	calendar := CompressedR2ExpiryCalendar()
	sim, err := NewSim(10*time.Second, Config{
		LogDir:           t.TempDir(),
		Seed:             607,
		LogMode:          "none",
		R2ExpiryCalendar: &calendar,
	})
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	defer sim.Close()
	if err := sim.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(sim.Venues) == 0 {
		t.Fatal("NewSim created no venues")
	}
	for _, venue := range sim.Venues {
		futureExpiries := make(map[int64]struct{})
		optionExpiries := make(map[int64]struct{})
		for _, listed := range venue.Exchange.ListInstruments("", "") {
			switch instrument := listed.(type) {
			case interface{ ExpiryNano() int64 }:
				switch listed.InstrumentType() {
				case "FUTURE":
					futureExpiries[instrument.ExpiryNano()] = struct{}{}
				case "OPTION":
					optionExpiries[instrument.ExpiryNano()] = struct{}{}
				}
			}
		}
		if len(futureExpiries) < 3 || len(optionExpiries) < 3 {
			t.Fatalf("venue %s did not activate calendar population: futures=%v options=%v", venue.ID, futureExpiries, optionExpiries)
		}
		if !reflect.DeepEqual(futureExpiries, optionExpiries) {
			t.Fatalf("venue %s futures/options expiry sets differ: futures=%v options=%v", venue.ID, futureExpiries, optionExpiries)
		}
	}
}
