package analysis

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	etypes "exchange_sim/types"
)

func crossVenueReplayEvent(t *testing.T, name, venue string, sequence uint64, timestamp int64, payload any) Event {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return Event{Name: name, VenueID: venue, Symbol: "ABC/USD", GlobalSequence: sequence, SimTS: timestamp, payload: raw}
}

func TestCollectCrossVenuePublicEventsUsesFrameOrderAcrossFiles(t *testing.T) {
	root := t.TempDir()
	makeLog := func(venue string, sequence uint64) string {
		t.Helper()
		path := filepath.Join(root, "venues", venue, "spot", "ABC-USD.jsonl")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		line, err := json.Marshal(map[string]any{
			"sim_ts": 10, "client_id": 0, "event": "BookSnapshot",
			"data": map[string]any{"venue_id": venue, "global_sequence": sequence,
				"payload": crossVenueSnapshotEvidence{SourceSequence: sequence}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(line, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	south := makeLog("south", 1)
	north := makeLog("north", 2)
	run := &Run{files: []string{north, south}}
	events, err := run.CollectCrossVenuePublicEvents([2]string{"north", "south"}, "ABC/USD")
	if err != nil || len(events) != 2 || events[0].VenueID != "south" || events[1].VenueID != "north" {
		t.Fatalf("ordered events = %#v, %v", events, err)
	}
	if _, err := ReplayCrossVenuePublicEvents(events, CrossVenuePublicReplayOptions{Venues: [2]string{"north", "south"}, Symbol: "ABC/USD"}); err != nil {
		t.Fatalf("rendered spot log selection/replay: %v", err)
	}
}

func TestCrossVenuePublicReplayUsesGlobalOrderAndVisibleProjection(t *testing.T) {
	northBids := []etypes.PriceLevel{{Price: 99, VisibleQty: 1}, {Price: 90, HiddenQty: 10}}
	northAsks := []etypes.PriceLevel{{Price: 100, VisibleQty: 1}}
	southBids := []etypes.PriceLevel{{Price: 105, VisibleQty: 1}}
	southAsks := []etypes.PriceLevel{{Price: 106, VisibleQty: 1}}
	events := []Event{
		crossVenueReplayEvent(t, "BookSnapshot", "north", 1, 10, crossVenueSnapshotEvidence{Bids: northBids, Asks: northAsks, SourceSequence: 1, PublicBids: northBids[:1], PublicAsks: northAsks}),
		crossVenueReplayEvent(t, "BookSnapshot", "south", 2, 10, crossVenueSnapshotEvidence{Bids: southBids, Asks: southAsks, SourceSequence: 1, PublicBids: southBids, PublicAsks: southAsks}),
		crossVenueReplayEvent(t, "BookDelta", "south", 3, 10, crossVenueDeltaEvidence{Side: "BUY", Price: 105, VisibleQty: 0, HiddenQty: 2, TotalQty: 2}),
	}
	replay, err := ReplayCrossVenuePublicEvents(events, CrossVenuePublicReplayOptions{Venues: [2]string{"north", "south"}, Symbol: "ABC/USD"})
	if err != nil {
		t.Fatal(err)
	}
	if len(replay.Transitions) != 3 || replay.Transitions[1].Touch.Bid != 105 || replay.Transitions[2].Touch.HasBid || len(replay.Terminal["north"].Bids) != 1 {
		t.Fatalf("public projection/updates = %#v", replay)
	}
	if replay.Transitions[2].GlobalSequence != 3 || replay.Transitions[2].SimTS != 10 {
		t.Fatalf("same-time event identity lost: %#v", replay.Transitions[2])
	}
}

func TestCrossVenuePublicReplayRejectsMalformedOrUnanchoredEvidence(t *testing.T) {
	valid := crossVenueReplayEvent(t, "BookSnapshot", "north", 1, 1, crossVenueSnapshotEvidence{
		Bids: []etypes.PriceLevel{{Price: 99, VisibleQty: 1}}, PublicBids: []etypes.PriceLevel{{Price: 99, VisibleQty: 1}}, SourceSequence: 1,
	})
	options := CrossVenuePublicReplayOptions{Venues: [2]string{"north", "south"}, Symbol: "ABC/USD", InitiallyEmpty: true}
	tests := []struct {
		name   string
		events []Event
		want   string
	}{
		{"missing-projection", []Event{crossVenueReplayEvent(t, "BookSnapshot", "north", 1, 1, map[string]any{"bids": []any{}, "asks": []any{}, "source_sequence": 1, "public_bids": []any{}})}, "missing required field"},
		{"false-projection", []Event{crossVenueReplayEvent(t, "BookSnapshot", "north", 1, 1, crossVenueSnapshotEvidence{Bids: []etypes.PriceLevel{{Price: 99, VisibleQty: 1}}, SourceSequence: 1})}, "disagrees"},
		{"negative-depth", []Event{crossVenueReplayEvent(t, "BookDelta", "north", 1, 1, crossVenueDeltaEvidence{Side: "BUY", Price: 99, VisibleQty: -1, TotalQty: -1})}, "invalid side or quantity"},
		{"duplicate-sequence", []Event{valid, crossVenueReplayEvent(t, "BookDelta", "north", 1, 1, crossVenueDeltaEvidence{Side: "BUY", Price: 99, VisibleQty: 0})}, "out of global order"},
		{"unknown-venue", []Event{crossVenueReplayEvent(t, "BookSnapshot", "east", 1, 1, crossVenueSnapshotEvidence{})}, "outside registered"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ReplayCrossVenuePublicEvents(test.events, options)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
	_, err := ReplayCrossVenuePublicEvents([]Event{crossVenueReplayEvent(t, "BookDelta", "north", 1, 1, crossVenueDeltaEvidence{Side: "BUY", Price: 99, VisibleQty: 1, TotalQty: 1})}, CrossVenuePublicReplayOptions{Venues: options.Venues, Symbol: options.Symbol})
	if err == nil || !strings.Contains(err.Error(), "precedes known") {
		t.Fatalf("unanchored delta error = %v", err)
	}
	if _, err := ReplayCrossVenuePublicEvents(nil, options); err == nil || !strings.Contains(err.Error(), "no observed") {
		t.Fatalf("missing venue stream error = %v", err)
	}
}
