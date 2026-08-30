package multivenue

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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

func TestR2CalendarListingsObserveTheFirstAutomationPoll(t *testing.T) {
	dir := t.TempDir()
	calendar := CompressedR2ExpiryCalendar()
	sim, err := NewSim(2*time.Second, Config{
		LogDir:           dir,
		LogMode:          "full",
		Seed:             607,
		R2ExpiryCalendar: &calendar,
	})
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	if err := sim.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := sim.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	const calendarEpochNano = int64(1735689600000000000)
	const firstPollOffsetNano = int64(time.Second)
	const firstShortExpiryNano = int64(2 * time.Hour)
	var futureListings int
	var firstListingAt int64
	var firstExpiry int64
	err = filepath.WalkDir(filepath.Join(dir, "venues"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		file, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			var record struct {
				SimTS int64           `json:"sim_ts"`
				Event string          `json:"event"`
				Data  json.RawMessage `json:"data"`
			}
			if decodeErr := json.Unmarshal(scanner.Bytes(), &record); decodeErr != nil || record.Event != "instrument_listed" {
				continue
			}
			var data struct {
				Payload struct {
					InstrumentType string `json:"instrument_type"`
					ExpiryNano     int64  `json:"expiry_nano"`
				} `json:"payload"`
			}
			if decodeErr := json.Unmarshal(record.Data, &data); decodeErr != nil || data.Payload.InstrumentType != "FUTURE" {
				continue
			}
			futureListings++
			if firstListingAt == 0 || record.SimTS < firstListingAt {
				firstListingAt = record.SimTS
				firstExpiry = data.Payload.ExpiryNano
			}
		}
		return scanner.Err()
	})
	if err != nil {
		t.Fatalf("scan lifecycle evidence: %v", err)
	}
	if futureListings == 0 {
		t.Fatal("no future listing evidence was persisted")
	}
	if want := calendarEpochNano + firstPollOffsetNano; firstListingAt != want {
		t.Fatalf("first future listing timestamp = %d, want first automation poll %d", firstListingAt, want)
	}
	if want := calendarEpochNano + firstShortExpiryNano; firstExpiry != want {
		t.Fatalf("first future expiry = %d, want calendar expiry %d", firstExpiry, want)
	}
}
