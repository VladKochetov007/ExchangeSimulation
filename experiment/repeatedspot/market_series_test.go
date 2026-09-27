package repeatedspot

import (
	"testing"
	"time"

	"exchange_sim/types"
)

func TestPublicBookSeriesUsesElapsedTimeAndOrderedDeltas(t *testing.T) {
	series := newPublicBookSeries(0, 1)
	if err := series.delta(int64(time.Second), "BUY", 99, 2, 0, 2); err != nil {
		t.Fatal(err)
	}
	if err := series.delta(3*int64(time.Second), "SELL", 101, 3, 0, 3); err != nil {
		t.Fatal(err)
	}
	bids := []types.PriceLevel{{Price: 99, VisibleQty: 2}}
	asks := []types.PriceLevel{{Price: 101, VisibleQty: 3}}
	if err := series.verifySnapshot(3*int64(time.Second), bids, asks); err != nil {
		t.Fatal(err)
	}
	if err := series.verifySnapshot(4*int64(time.Second), bids, asks); err != nil {
		t.Fatal(err)
	}
	if err := series.verifyTerminal(5*int64(time.Second), bids, asks); err != nil {
		t.Fatal(err)
	}
	summary := series.summary()
	if summary.HorizonNanos != 5*int64(time.Second) ||
		summary.EmptyNanos != int64(time.Second) ||
		summary.BidOnlyNanos != 2*int64(time.Second) ||
		summary.TwoSidedNanos != 2*int64(time.Second) ||
		summary.AskOnlyNanos != 0 || summary.PublicSnapshotMessages != 2 ||
		summary.SpreadPriceUnitNanos != "4000000000" ||
		summary.BidDepthBaseUnitNanos != "8000000000" ||
		summary.AskDepthBaseUnitNanos != "6000000000" {
		t.Fatalf("wrong time-weighted book summary: %+v", summary)
	}
	for name, malformed := range map[string]func() error{
		"forged depth": func() error {
			return series.verifySnapshot(5*int64(time.Second), []types.PriceLevel{{Price: 99, VisibleQty: 3}}, asks)
		},
		"regressed time": func() error { return series.accrue(4 * int64(time.Second)) },
		"invalid side":   func() error { return series.delta(5*int64(time.Second), "UNKNOWN", 99, 2, 0, 2) },
	} {
		if err := malformed(); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
}
