package multivenue

import "testing"

// H-029. RT-020 established that the mark is anchored to the index and clamped
// around it, which makes the index the load-bearing number. This measures the
// index's own robustness: whether a participant can move it, and whether it
// forgets.
//
// The campaign builds spotIndexProvider in consensus mode over
// venueMids[symbol][venueID], populated from each venue's automation tick and
// guarded by a two-sided-mid check. Two properties follow from the data
// structure rather than from any policy, and this test states both as numbers.
func TestAuditIndexConsensusRobustness(t *testing.T) {
	const symbol = "ABC/USD"

	t.Run("one venue is the index", func(t *testing.T) {
		p := newSpotIndexProvider("consensus", symbol)
		p.observeVenueMid(symbol, "north", 100)
		price, err := p.Price(symbol)
		if err != nil {
			t.Fatalf("price: %v", err)
		}
		if price != 100 {
			t.Errorf("index = %d, want the sole observation 100", price)
		}
	})

	t.Run("two venues resolve to the upper observation, not the average", func(t *testing.T) {
		p := newSpotIndexProvider("consensus", symbol)
		p.observeVenueMid(symbol, "north", 100)
		p.observeVenueMid(symbol, "south", 200)
		price, err := p.Price(symbol)
		if err != nil {
			t.Fatalf("price: %v", err)
		}
		t.Logf("two venues quoting 100 and 200 give an index of %d", price)
		if price != 200 {
			t.Errorf("index = %d, want the upper observation 200: mids[len/2] selects it", price)
		}
	})

	t.Run("three venues resist one", func(t *testing.T) {
		p := newSpotIndexProvider("consensus", symbol)
		p.observeVenueMid(symbol, "north", 100)
		p.observeVenueMid(symbol, "central", 101)
		p.observeVenueMid(symbol, "south", 100_000)
		price, err := p.Price(symbol)
		if err != nil {
			t.Fatalf("price: %v", err)
		}
		if price != 101 {
			t.Errorf("index = %d, want 101: one venue quoting 100 000 must not carry the median", price)
		}
	})

	// The property with no policy behind it: a venue that stops publishing does
	// not lose its vote, because the map entry is overwritten rather than aged.
	t.Run("a venue that stops quoting keeps voting", func(t *testing.T) {
		p := newSpotIndexProvider("consensus", symbol)
		p.observeVenueMid(symbol, "north", 100)
		p.observeVenueMid(symbol, "central", 100)
		p.observeVenueMid(symbol, "south", 100)

		// South's book empties. Its automation tick no longer calls
		// observeVenueMid at all — the campaign's call site is guarded by a
		// two-sided-mid check — so its last observation stays.
		// The other two move away.
		p.observeVenueMid(symbol, "north", 400)
		p.observeVenueMid(symbol, "central", 400)

		price, err := p.Price(symbol)
		if err != nil {
			t.Fatalf("price: %v", err)
		}
		t.Logf("two live venues at 400, one silent venue last seen at 100: index %d", price)
		if price != 400 {
			t.Errorf("index = %d: the silent venue's stale 100 is still deciding the median", price)
		}

		// And with the majority silent instead, the stale votes win outright.
		q := newSpotIndexProvider("consensus", symbol)
		q.observeVenueMid(symbol, "north", 100)
		q.observeVenueMid(symbol, "central", 100)
		q.observeVenueMid(symbol, "south", 100)
		q.observeVenueMid(symbol, "south", 400) // only the live venue updates
		stale, err := q.Price(symbol)
		if err != nil {
			t.Fatalf("price: %v", err)
		}
		t.Logf("one live venue at 400, two silent venues last seen at 100: index %d", stale)
		if stale != 100 {
			t.Errorf("index = %d, want 100: the two stale observations should outvote the only live one", stale)
		}
	})
}
