package analysis

import "testing"

func TestMeasureCalendarDeduplicatesIdentitiesAndMeasuresCycles(t *testing.T) {
	dir := writeRun(t, Report{}, map[string][]string{
		"north/lifecycle.jsonl": {
			`{"sim_ts":1,"client_id":0,"event":"instrument_listed","data":{"venue_id":"north","payload":{"symbol":"ABC-FUT-3-U4142432f555344","instrument_type":"FUTURE","expiry_nano":3}}}`,
			`{"sim_ts":1,"client_id":0,"event":"instrument_listed","data":{"venue_id":"north","payload":{"symbol":"ABC-OPT-U4142432f555344-3-K100-C","instrument_type":"OPTION","expiry_nano":3}}}`,
			`{"sim_ts":1,"client_id":0,"event":"instrument_listed","data":{"venue_id":"north","payload":{"symbol":"ABC-OPT-U4142432f555344-3-K100-P","instrument_type":"OPTION","expiry_nano":3}}}`,
			`{"sim_ts":1,"client_id":0,"event":"instrument_listed","data":{"venue_id":"north","payload":{"symbol":"ABC-FUT-3-U4142432f555344","instrument_type":"FUTURE","expiry_nano":3}}}`,
			`{"sim_ts":2,"client_id":0,"event":"instrument_listed","data":{"venue_id":"north","payload":{"symbol":"ABC-FUT-4-U4142432f555344","instrument_type":"FUTURE","expiry_nano":4}}}`,
			`{"sim_ts":3,"client_id":0,"event":"instrument_settled","data":{"venue_id":"north","payload":{"symbol":"ABC-FUT-3-U4142432f555344","instrument_type":"FUTURE","expiry_nano":3}}}`,
			`{"sim_ts":3,"client_id":0,"event":"instrument_settled","data":{"venue_id":"north","payload":{"symbol":"ABC-OPT-U4142432f555344-3-K100-C","instrument_type":"OPTION","expiry_nano":3}}}`,
			`{"sim_ts":3,"client_id":0,"event":"instrument_settled","data":{"venue_id":"north","payload":{"symbol":"ABC-OPT-U4142432f555344-3-K100-P","instrument_type":"OPTION","expiry_nano":3}}}`,
		},
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureCalendar(CalendarOptions{})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if result.SchemaVersion != 1 || result.Contract != "calendar-audit-v1" || len(result.Venues) != 1 {
		t.Fatalf("calendar envelope = %+v", result)
	}
	venue := result.Venues[0]
	if result.ListingEvents != 5 || result.SettlementEvents != 3 || result.DuplicateListings != 1 {
		t.Fatalf("event counts = %+v", result)
	}
	if venue.FuturesListed != 2 || venue.OptionsListed != 2 || venue.FuturesSettled != 1 || venue.OptionsSettled != 2 {
		t.Fatalf("venue lifecycle counts = %+v", venue)
	}
	if venue.DuplicateFutureListings != 1 || venue.FutureExpiryCycles != 1 || venue.OptionExpiryCycles != 1 {
		t.Fatalf("venue identity/cycle counts = %+v", venue)
	}
	if venue.MaxSimultaneousFutureExpiries != 2 || venue.MaxSimultaneousOptionExpiries != 1 {
		t.Fatalf("simultaneous expiry counts = %+v", venue)
	}
	if len(result.SharedExpiryNanos) != 1 || result.SharedExpiryNanos[0] != 3 {
		t.Fatalf("shared expiry set = %v", result.SharedExpiryNanos)
	}
}

func TestMeasureCalendarPreservesSameFileLifecycleOrder(t *testing.T) {
	dir := writeRun(t, Report{}, map[string][]string{
		"north/lifecycle.jsonl": {
			`{"sim_ts":1,"client_id":0,"event":"instrument_listed","data":{"venue_id":"north","payload":{"symbol":"ABC-FUT-1-U4142432f555344","instrument_type":"FUTURE","expiry_nano":1}}}`,
			`{"sim_ts":1,"client_id":0,"event":"instrument_listed","data":{"venue_id":"north","payload":{"symbol":"ABC-FUT-2-U4142432f555344","instrument_type":"FUTURE","expiry_nano":2}}}`,
			`{"sim_ts":3,"client_id":0,"event":"instrument_settled","data":{"venue_id":"north","payload":{"symbol":"ABC-FUT-1-U4142432f555344","instrument_type":"FUTURE","expiry_nano":1}}}`,
			`{"sim_ts":3,"client_id":0,"event":"instrument_settled","data":{"venue_id":"north","payload":{"symbol":"ABC-FUT-2-U4142432f555344","instrument_type":"FUTURE","expiry_nano":2}}}`,
			`{"sim_ts":3,"client_id":0,"event":"instrument_listed","data":{"venue_id":"north","payload":{"symbol":"ABC-FUT-3-U4142432f555344","instrument_type":"FUTURE","expiry_nano":3}}}`,
		},
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureCalendar(CalendarOptions{})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if len(result.Venues) != 1 {
		t.Fatalf("venues = %d, want one", len(result.Venues))
	}
	if got := result.Venues[0].MaxSimultaneousFutureExpiries; got != 2 {
		t.Fatalf("peak future coexistence = %d, want 2", got)
	}
}

func TestMeasureCalendarRejectsSameTimestampSettlementBeforeListing(t *testing.T) {
	dir := writeRun(t, Report{}, map[string][]string{
		"north/lifecycle.jsonl": {
			`{"sim_ts":3,"client_id":0,"event":"instrument_settled","data":{"venue_id":"north","payload":{"symbol":"ABC-FUT-3-U4142432f555344","instrument_type":"FUTURE","expiry_nano":3}}}`,
			`{"sim_ts":3,"client_id":0,"event":"instrument_listed","data":{"venue_id":"north","payload":{"symbol":"ABC-FUT-3-U4142432f555344","instrument_type":"FUTURE","expiry_nano":3}}}`,
		},
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureCalendar(CalendarOptions{})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	venue := result.Venues[0]
	if venue.SettlementBeforeListing != 1 {
		t.Fatalf("settlements before listing = %d, want one", venue.SettlementBeforeListing)
	}
	if venue.FuturesListed != 1 || venue.FuturesSettled != 0 {
		t.Fatalf("same-timestamp reversed lifecycle counts = %+v", venue)
	}
}

func TestMeasureCalendarRejectsCrossFileSameTimestampLifecycleOrder(t *testing.T) {
	dir := writeRun(t, Report{}, map[string][]string{
		"north/a.jsonl": {
			`{"sim_ts":3,"client_id":0,"event":"instrument_listed","data":{"venue_id":"north","payload":{"symbol":"ABC-FUT-3-U4142432f555344","instrument_type":"FUTURE","expiry_nano":3}}}`,
		},
		"north/z.jsonl": {
			`{"sim_ts":3,"client_id":0,"event":"instrument_settled","data":{"venue_id":"north","payload":{"symbol":"ABC-FUT-3-U4142432f555344","instrument_type":"FUTURE","expiry_nano":3}}}`,
		},
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := run.MeasureCalendar(CalendarOptions{}); err == nil {
		t.Fatal("same-timestamp cross-file lifecycle order was not rejected")
	}
}
