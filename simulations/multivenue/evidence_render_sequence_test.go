package multivenue

import (
	"strings"
	"testing"
)

// The renderer's per-venue sequence contract is what makes the merge exact, and
// nothing tested it. It used to be enforced twice: once by a linear scan of
// every record already appended to a route, which made rendering quadratic, and
// once by a set covering the whole venue. The scan was removed because the
// second check is strictly stronger, and the set became a bitmap when rendering
// stopped holding the run in memory. These tests hold whatever enforces it to
// the same claims.
func sequencesOf(t *testing.T, route string, sequences ...uint64) *venueSequences {
	t.Helper()
	seen := newVenueSequences()
	key := renderRouteKey{venue: "north", route: route}
	for _, sequence := range sequences {
		if err := seen.observe(key, sequence); err != nil {
			t.Fatalf("observe %d: %v", sequence, err)
		}
	}
	return seen
}

func TestRenderRejectsDuplicateSequenceWithinARoute(t *testing.T) {
	seen := sequencesOf(t, "general.jsonl", 1, 2)
	err := seen.observe(renderRouteKey{venue: "north", route: "general.jsonl"}, 2)
	if err == nil {
		t.Fatal("a repeated sequence was accepted")
	}
	if !strings.Contains(err.Error(), "duplicate venue sequence") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenderRejectsDuplicateSequenceAcrossRoutes(t *testing.T) {
	seen := sequencesOf(t, "general.jsonl", 1)
	err := seen.observe(renderRouteKey{venue: "north", route: "derivatives.jsonl"}, 1)
	if err == nil {
		t.Fatal("one sequence claimed by two routes of the same venue was accepted")
	}
	if !strings.Contains(err.Error(), "duplicate venue sequence") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenderRejectsAGapInTheVenueSequence(t *testing.T) {
	seen := sequencesOf(t, "general.jsonl", 1, 3)
	err := seen.validate()
	if err == nil {
		t.Fatal("a missing sequence was accepted")
	}
	if !strings.Contains(err.Error(), "missing venue sequence north#2") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenderAcceptsAContiguousVenueSequenceSplitAcrossRoutes(t *testing.T) {
	seen := sequencesOf(t, "general.jsonl", 1, 3)
	if err := seen.observe(renderRouteKey{venue: "north", route: "derivatives.jsonl"}, 2); err != nil {
		t.Fatalf("observe 2: %v", err)
	}
	if err := seen.validate(); err != nil {
		t.Fatalf("a contiguous sequence split across routes was rejected: %v", err)
	}
}

// The bitmap grows as sequences arrive, so a run whose first record is far from
// one must still be tracked exactly.
func TestRenderTracksSparseAndDistantSequences(t *testing.T) {
	seen := sequencesOf(t, "general.jsonl", 5000, 1)
	if err := seen.observe(renderRouteKey{venue: "north", route: "general.jsonl"}, 5000); err == nil {
		t.Fatal("a repeated distant sequence was accepted")
	}
	if err := seen.validate(); err == nil || !strings.Contains(err.Error(), "missing venue sequence north#2") {
		t.Fatalf("expected the first gap to be reported, got %v", err)
	}
}

func TestRenderRejectsAZeroSequence(t *testing.T) {
	seen := newVenueSequences()
	err := seen.observe(renderRouteKey{venue: "north", route: "general.jsonl"}, 0)
	if err == nil || !strings.Contains(err.Error(), "incomplete rendered evidence record") {
		t.Fatalf("expected a zero sequence to be refused, got %v", err)
	}
}
