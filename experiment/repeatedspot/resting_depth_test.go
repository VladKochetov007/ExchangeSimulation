package repeatedspot

import (
	"testing"

	"exchange_sim/types"
)

func TestRestingDepthExactWindowAndOwnTwoSidedTime(t *testing.T) {
	series := newRestingDepthSeries(0, MeasurementWindow{StartAt: 5, EndAt: 15})
	for _, change := range []struct {
		at    int64
		side  string
		delta int64
	}{{2, "BUY", 3}, {7, "SELL", 2}, {10, "BUY", -1}, {13, "SELL", -2}} {
		if err := series.change(change.at, change.side, change.delta); err != nil {
			t.Fatal(err)
		}
	}
	summary, err := series.finish(20)
	if err != nil {
		t.Fatal(err)
	}
	if summary.WindowNanos != 10 || summary.BidPresentNanos != 10 ||
		summary.AskPresentNanos != 6 || summary.TwoSidedNanos != 6 ||
		summary.BidDepthBaseUnitNanos != "25" || summary.AskDepthBaseUnitNanos != "12" ||
		summary.TerminalBidBaseUnits != 2 || summary.TerminalAskBaseUnits != 0 {
		t.Fatalf("wrong own-resting-depth exposure: %+v", summary)
	}
}

func TestRestingOrdersIndependentlyReconstructPublicLevels(t *testing.T) {
	book := &restingBook{orders: map[uint64]*restingOrder{
		1: {clientID: 1, price: 99, side: "BUY", qty: 1},
		2: {clientID: 2, price: 99, side: "BUY", qty: 2},
		3: {clientID: 3, price: 101, side: "SELL", qty: 3},
	}}
	bids := []types.PriceLevel{{Price: 99, VisibleQty: 3}}
	asks := []types.PriceLevel{{Price: 101, VisibleQty: 3}}
	if err := book.verifySnapshot(bids, asks); err != nil {
		t.Fatal(err)
	}
	if err := book.verifySnapshot([]types.PriceLevel{{Price: 99, VisibleQty: 2}}, asks); err == nil {
		t.Fatal("published depth concealed one resting client's quantity")
	}
}
