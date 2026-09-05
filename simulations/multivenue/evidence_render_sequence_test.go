package multivenue

import (
	"strings"
	"testing"
)

// The renderer's per-venue sequence contract is what makes the merge exact, and
// nothing tested it. addRenderRecord used to re-check it with a linear scan of
// every record already appended, which made the render quadratic; the scan was
// removed because validateRenderRecords already enforces the same contract with
// a set, across the whole venue rather than one route. These tests hold that
// remaining check to the stronger claim.
func routesWith(records ...renderRecord) map[renderRouteKey][]renderRecord {
	routes := map[renderRouteKey][]renderRecord{}
	key := renderRouteKey{venue: "north", route: "general.jsonl"}
	for _, record := range records {
		routes[key] = append(routes[key], record)
	}
	return routes
}

func TestRenderRejectsDuplicateSequenceWithinARoute(t *testing.T) {
	routes := routesWith(
		renderRecord{sequence: 1, raw: []byte("{}")},
		renderRecord{sequence: 2, raw: []byte("{}")},
		renderRecord{sequence: 2, raw: []byte("{}")},
	)
	err := validateRenderRecords(routes)
	if err == nil {
		t.Fatal("a repeated sequence was accepted")
	}
	if !strings.Contains(err.Error(), "duplicate venue sequence") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenderRejectsDuplicateSequenceAcrossRoutes(t *testing.T) {
	routes := map[renderRouteKey][]renderRecord{
		{venue: "north", route: "general.jsonl"}:     {{sequence: 1, raw: []byte("{}")}},
		{venue: "north", route: "derivatives.jsonl"}: {{sequence: 1, raw: []byte("{}")}},
	}
	err := validateRenderRecords(routes)
	if err == nil {
		t.Fatal("one sequence claimed by two routes of the same venue was accepted")
	}
	if !strings.Contains(err.Error(), "duplicate venue sequence") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenderRejectsAGapInTheVenueSequence(t *testing.T) {
	routes := routesWith(
		renderRecord{sequence: 1, raw: []byte("{}")},
		renderRecord{sequence: 3, raw: []byte("{}")},
	)
	err := validateRenderRecords(routes)
	if err == nil {
		t.Fatal("a missing sequence was accepted")
	}
	if !strings.Contains(err.Error(), "missing venue sequence") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenderAcceptsAContiguousVenueSequence(t *testing.T) {
	routes := map[renderRouteKey][]renderRecord{
		{venue: "north", route: "general.jsonl"}:     {{sequence: 1}, {sequence: 3}},
		{venue: "north", route: "derivatives.jsonl"}: {{sequence: 2}},
	}
	if err := validateRenderRecords(routes); err != nil {
		t.Fatalf("a contiguous sequence split across routes was rejected: %v", err)
	}
}
