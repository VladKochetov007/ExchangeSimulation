package multivenue

import (
	"testing"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

func localCacheSnapshot(symbol string, sequence uint64, timestamp, bid, ask int64) actor.BookSnapshotEvent {
	return actor.BookSnapshotEvent{
		Symbol: symbol, SeqNum: sequence, Timestamp: timestamp,
		Snapshot: &exchange.BookSnapshot{
			Bids: []exchange.PriceLevel{{Price: bid, VisibleQty: 10}},
			Asks: []exchange.PriceLevel{{Price: ask, VisibleQty: 20}},
		},
	}
}

func TestLocalBookCacheOnlyAdvancesFromNewDeliveredSnapshot(t *testing.T) {
	cache := NewLocalBookCache("north", "ABC/USD")
	if cache.ObserveSnapshot(localCacheSnapshot("CDF/USD", 1, 10, 90, 110)) {
		t.Fatal("cache accepted a different symbol")
	}
	if !cache.ObserveSnapshot(localCacheSnapshot("ABC/USD", 9, 100, 99, 101)) {
		t.Fatal("cache rejected its first two-sided observation")
	}
	if got, ok := cache.Mid(); !ok || got != 100 {
		t.Fatalf("cache mid = %d, %t; want 100, true", got, ok)
	}
	if cache.ObserveSnapshot(localCacheSnapshot("ABC/USD", 8, 101, 199, 201)) {
		t.Fatal("cache accepted a stale sequence")
	}
	if got, ok := cache.Mid(); !ok || got != 100 {
		t.Fatalf("stale source rewrote cache mid = %d, %t", got, ok)
	}
	if !cache.ObserveSnapshot(localCacheSnapshot("ABC/USD", 10, 102, 109, 111)) {
		t.Fatal("cache rejected newer source observation")
	}
	view, ok := cache.View()
	if !ok || view.SourceVenue != "north" || view.Sequence != 10 || view.PublishedAt != 102 || view.Updates != 2 {
		t.Fatalf("cache view = %+v, %t", view, ok)
	}
	if cache.RejectedStale() != 1 {
		t.Fatalf("rejected stale = %d, want 1", cache.RejectedStale())
	}
}

func TestLocalBookCacheRejectsInvalidTop(t *testing.T) {
	cache := NewLocalBookCache("north", "ABC/USD")
	for _, snapshot := range []actor.BookSnapshotEvent{
		{Symbol: "ABC/USD", SeqNum: 1, Timestamp: 1},
		{Symbol: "ABC/USD", SeqNum: 1, Timestamp: 1, Snapshot: &exchange.BookSnapshot{
			Bids: []exchange.PriceLevel{{Price: 100, VisibleQty: 1}},
			Asks: []exchange.PriceLevel{{Price: 100, VisibleQty: 1}},
		}},
		{Symbol: "ABC/USD", SeqNum: 1, Timestamp: 1, Snapshot: &exchange.BookSnapshot{
			Bids: []exchange.PriceLevel{{Price: 99, VisibleQty: 0}},
			Asks: []exchange.PriceLevel{{Price: 101, VisibleQty: 1}},
		}},
	} {
		if cache.ObserveSnapshot(snapshot) {
			t.Fatalf("invalid snapshot admitted: %+v", snapshot)
		}
	}
	if _, ok := cache.View(); ok {
		t.Fatal("invalid source created usable cache state")
	}
}
