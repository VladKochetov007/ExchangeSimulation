package exchange

import (
	"slices"
	"testing"
)

// The cached iteration orders are correctness-critical: the risk sweep
// dereferences e.Books[symbol] for every symbol the cache reports, so a stale
// entry is a nil dereference rather than a wrong number. These tests pin the
// cache to the maps it summarizes across insertion, deletion and reuse.
func TestSortedBookSymbolsTracksTheBookSet(t *testing.T) {
	e := &DefaultExchange{Books: map[string]*OrderBook{}, Clients: map[uint64]*Client{}}

	if got := e.sortedBookSymbols(); len(got) != 0 {
		t.Fatalf("empty exchange reported symbols %v", got)
	}

	e.Books["ZETA"] = &OrderBook{Symbol: "ZETA"}
	e.Books["ALPHA"] = &OrderBook{Symbol: "ALPHA"}
	e.Books["MID"] = &OrderBook{Symbol: "MID"}
	e.bookGeneration++
	want := []string{"ALPHA", "MID", "ZETA"}
	if got := e.sortedBookSymbols(); !slices.Equal(got, want) {
		t.Fatalf("symbols = %v, want %v", got, want)
	}
	// A second call must return the same order from the cache.
	if got := e.sortedBookSymbols(); !slices.Equal(got, want) {
		t.Fatalf("cached symbols = %v, want %v", got, want)
	}

	delete(e.Books, "MID")
	e.bookGeneration++
	want = []string{"ALPHA", "ZETA"}
	if got := e.sortedBookSymbols(); !slices.Equal(got, want) {
		t.Fatalf("after delete symbols = %v, want %v", got, want)
	}

	// Every reported symbol must still resolve to a book. This is the exact
	// failure a missing generation bump caused: the sweep dereferenced a book
	// that expiry had already removed.
	for _, symbol := range e.sortedBookSymbols() {
		if e.Books[symbol] == nil {
			t.Fatalf("cache reported symbol %q with no book", symbol)
		}
	}
}

// TestSortedCachesSurviveAMissedGenerationBump covers the guard: a mutation
// that forgets to bump the counter must not produce a stale entry.
func TestSortedCachesSurviveAMissedGenerationBump(t *testing.T) {
	e := &DefaultExchange{Books: map[string]*OrderBook{}, Clients: map[uint64]*Client{}}
	e.Books["A"] = &OrderBook{Symbol: "A"}
	e.Books["B"] = &OrderBook{Symbol: "B"}
	e.bookGeneration++
	_ = e.sortedBookSymbols()

	delete(e.Books, "B") // deliberately no generation bump
	for _, symbol := range e.sortedBookSymbols() {
		if e.Books[symbol] == nil {
			t.Fatalf("stale symbol %q survived a missed generation bump", symbol)
		}
	}

	e.Clients[7] = &Client{}
	e.clientGeneration++
	_ = e.sortedClientIDs()
	e.Clients[3] = &Client{} // deliberately no generation bump
	if got := e.sortedClientIDs(); !slices.Equal(got, []uint64{3, 7}) {
		t.Fatalf("client IDs = %v, want [3 7] after a missed generation bump", got)
	}
}

func TestSortedClientIDsAreAscending(t *testing.T) {
	e := &DefaultExchange{Books: map[string]*OrderBook{}, Clients: map[uint64]*Client{}}
	for _, id := range []uint64{9, 1, 40, 3, 18446744073709551615, 0} {
		e.Clients[id] = &Client{}
	}
	e.clientGeneration++
	want := []uint64{0, 1, 3, 9, 40, 18446744073709551615}
	if got := e.sortedClientIDs(); !slices.Equal(got, want) {
		t.Fatalf("client IDs = %v, want %v", got, want)
	}
}
