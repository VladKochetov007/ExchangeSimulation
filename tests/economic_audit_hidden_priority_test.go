package exchange_test

import (
	"testing"

	. "exchange_sim/exchange"
)

// H-014. A hidden order pays nothing for being hidden.
//
// matching.makerAvailable throttles an iceberg to its display tranche and
// returns the full remainder for everything else, hidden orders included, and
// both matchers use it. So at one price a hidden order carries exactly the time
// priority a displayed order of the same size would carry, while contributing
// nothing to the public snapshot.
//
// Real venues subordinate hidden size to displayed size at the same price
// precisely because otherwise displaying is irrational: a participant who shows
// size gives information away and gets nothing for it. Under this model Hidden
// weakly dominates Normal — identical fills, less information leaked — so any
// actor using Normal is handicapped with no compensating benefit.
//
// This test does not argue for display priority. It records which rule is in
// force, so the choice is visible and a change to it is deliberate. The
// campaign is unaffected either way: no simulation constructs a non-Normal
// order, and BaseActor.SubmitOrderFull, the only route from an actor to a
// visibility other than Normal, has no callers anywhere in the tree.
func TestAuditHiddenOrderKeepsFullTimePriorityOverDisplayedSize(t *testing.T) {
	ex := NewExchange(4, &RealClock{})
	ex.AddInstrument(NewSpotInstrument(
		"ABC/USD", "ABC", "USD", BTC_PRECISION, USD_PRECISION, USD_PRECISION, BTC_PRECISION/1_000))
	for _, id := range []uint64{1, 2, 3} {
		ex.ConnectNewClient(id, map[string]int64{
			"USD": 1_000_000 * USD_PRECISION,
			"ABC": 1_000 * BTC_PRECISION,
		}, &FixedFee{})
	}

	price := int64(100 * USD_PRECISION)
	clip := int64(BTC_PRECISION / 100)

	// Client 2 rests hidden size first; client 3 rests displayed size behind it.
	hidden := ex.PlaceOrder(2, &OrderRequest{
		RequestID: 1, Symbol: "ABC/USD", Side: Sell, Type: LimitOrder,
		Price: price, Qty: clip, TimeInForce: GTC, Visibility: Hidden,
	})
	if !hidden.Success {
		t.Fatalf("hidden order rejected: %#v", hidden)
	}
	displayed := ex.PlaceOrder(3, &OrderRequest{
		RequestID: 1, Symbol: "ABC/USD", Side: Sell, Type: LimitOrder,
		Price: price, Qty: clip, TimeInForce: GTC, Visibility: Normal,
	})
	if !displayed.Success {
		t.Fatalf("displayed order rejected: %#v", displayed)
	}

	// While both rest, the public book shows only the displayed clip. This is
	// the benefit side of the trade-off; the priority result below is the cost
	// side, and there is none.
	publicQty := int64(0)
	for _, level := range ex.GetBook("ABC/USD").Asks.GetPublicSnapshot() {
		if level.Price == price {
			publicQty = level.VisibleQty
		}
	}
	if publicQty != clip {
		t.Errorf("public ask at %d shows %d, want only the displayed clip %d", price, publicQty, clip)
	}

	hiddenBefore := ex.Clients[2].Balances["ABC"]
	displayedBefore := ex.Clients[3].Balances["ABC"]

	// A taker for exactly one clip. Whoever is served is the one with priority.
	if _, reject := InjectLimitOrder(ex, 1, "ABC/USD", Buy, price, clip); reject != "" {
		t.Fatalf("taker rejected: %s", reject)
	}

	hiddenSold := hiddenBefore - ex.Clients[2].Balances["ABC"]
	displayedSold := displayedBefore - ex.Clients[3].Balances["ABC"]
	t.Logf("hidden resting first sold %d, displayed resting second sold %d", hiddenSold, displayedSold)

	if hiddenSold != clip || displayedSold != 0 {
		t.Errorf("the matcher no longer treats hidden size as fully time-prioritised: hidden sold %d, displayed sold %d",
			hiddenSold, displayedSold)
	}

}
