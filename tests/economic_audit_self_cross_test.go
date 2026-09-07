package exchange_test

import (
	"testing"
	"time"

	. "exchange_sim/exchange"
)

// H-017. Can one participant leave the public book crossed against itself?
//
// RejectSelfTrade is declared in the reject vocabulary but no code path
// produces it, and the matchers prevent self-execution by skipping a resting
// order whose ClientID equals the incoming order's. Skipping alone would leave
// a participant's own bid above its own ask: no third party put it there, none
// can be filled to remove it, and best bid, best ask, mid and spread are the
// inputs every other participant quotes and hedges against.
//
// FALSIFIED. The exchange does not skip and rest; it cancels. After the matcher
// has consumed every crossable order belonging to other clients, any price still
// crossing the remainder must belong to the incoming client, and
// cancelOwnCrossingQuotes withdraws those resting quotes — the venue
// "cancel maker" self-trade-prevention mode.
//
// This test therefore records the policy that is in force and the four
// properties that make it safe, so that a change to any of them is visible.
func TestAuditSelfTradePreventionCancelsRatherThanCrossing(t *testing.T) {
	const clip = BTC_PRECISION / 100

	newFixture := func(t *testing.T) (*DefaultExchange, Gateway) {
		t.Helper()
		ex := NewExchange(4, &RealClock{})
		ex.AddInstrument(NewSpotInstrument(
			"ABC/USD", "ABC", "USD", BTC_PRECISION, USD_PRECISION, USD_PRECISION, BTC_PRECISION/1_000))
		gw := ex.ConnectNewClient(1, map[string]int64{
			"USD": 1_000_000 * USD_PRECISION,
			"ABC": 1_000 * BTC_PRECISION,
		}, &FixedFee{})
		return ex, gw
	}

	// Responses are delivered from an outbox by a separate goroutine, so a
	// non-blocking drain would race the delivery rather than observe it.
	// Collect until the expected count arrives or the deadline passes; a
	// shortfall is then a real shortfall, not a scheduling artefact.
	collectForcedCancels := func(gw Gateway, want int) []uint64 {
		ids := make([]uint64, 0, want)
		deadline := time.After(2 * time.Second)
		for len(ids) < want {
			select {
			case resp := <-gw.Responses():
				if notice, ok := resp.Data.(*ForcedCancelNotification); ok {
					ids = append(ids, notice.OrderID)
				}
			case <-deadline:
				return ids
			}
		}
		return ids
	}
	drainPending := func(gw Gateway) {
		for {
			select {
			case <-gw.Responses():
			case <-time.After(50 * time.Millisecond):
				return
			}
		}
	}

	t.Run("the book is never left crossed, and no wash trade prints", func(t *testing.T) {
		ex, _ := newFixture(t)
		ask := int64(90 * USD_PRECISION)
		bid := int64(110 * USD_PRECISION)

		if _, reject := InjectLimitOrder(ex, 1, "ABC/USD", Sell, ask, clip); reject != "" {
			t.Fatalf("own ask rejected: %s", reject)
		}
		usdBefore, abcBefore := ex.Clients[1].Balances["USD"], ex.Clients[1].Balances["ABC"]

		resp := ex.PlaceOrder(1, &OrderRequest{
			RequestID: 1, Symbol: "ABC/USD", Side: Buy, Type: LimitOrder,
			Price: bid, Qty: clip, TimeInForce: GTC,
		})
		if !resp.Success {
			t.Fatalf("the self-crossing buy was refused with %q", resp.Error)
		}

		book := ex.GetBook("ABC/USD")
		bestBid, bidErr := book.GetBestBid()
		if bidErr != nil || bestBid != bid {
			t.Fatalf("the incoming order did not rest: bid %d, err %v", bestBid, bidErr)
		}
		if _, askErr := book.GetBestAsk(); askErr == nil {
			t.Errorf("the participant's own ask survived alongside its higher bid: the book is crossed")
		}
		// No self-execution: nothing traded, so nothing was washed.
		if _, err := book.GetLastPrice(); err == nil {
			t.Errorf("a trade printed against the participant's own order")
		}
		if ex.Clients[1].Balances["USD"] != usdBefore || ex.Clients[1].Balances["ABC"] != abcBefore {
			t.Errorf("balances moved without a trade: USD %d -> %d, ABC %d -> %d",
				usdBefore, ex.Clients[1].Balances["USD"], abcBefore, ex.Clients[1].Balances["ABC"])
		}
	})

	t.Run("the cancelled quote's collateral is released, not stranded", func(t *testing.T) {
		ex, _ := newFixture(t)
		if _, reject := InjectLimitOrder(ex, 1, "ABC/USD", Sell, 90*USD_PRECISION, clip); reject != "" {
			t.Fatalf("own ask rejected: %s", reject)
		}
		if got := ex.Clients[1].Reserved["ABC"]; got != clip {
			t.Fatalf("the resting sell reserved %d base units, want %d", got, clip)
		}

		if resp := ex.PlaceOrder(1, &OrderRequest{
			RequestID: 1, Symbol: "ABC/USD", Side: Buy, Type: LimitOrder,
			Price: 110 * USD_PRECISION, Qty: clip, TimeInForce: GTC,
		}); !resp.Success {
			t.Fatalf("self-crossing buy refused: %q", resp.Error)
		}

		if got := ex.Clients[1].Reserved["ABC"]; got != 0 {
			t.Errorf("the cancelled sell left %d base units earmarked: buying power stranded", got)
		}
		if avail, bal := ex.Clients[1].GetAvailable("ABC"), ex.Clients[1].Balances["ABC"]; avail > bal {
			t.Errorf("available %d exceeds balance %d: collateral was over-released", avail, bal)
		}
		if ids := ex.Clients[1].OrderIDs; len(ids) != 1 {
			t.Errorf("the client still tracks %v: a cancelled order was not removed from its book", ids)
		}
	})

	// The pattern this project has been bitten by before: an order removed
	// without telling its owner leaves the actor believing it still rests, and
	// its own bookkeeping blocked indefinitely.
	t.Run("the owner is told, in a deterministic order", func(t *testing.T) {
		ex, gw := newFixture(t)
		// Three own asks, deliberately placed from the highest price down so
		// that price order and placement order disagree. Cancellation must
		// follow placement order, which is what the implementation sorts by.
		var placed []uint64
		for index, price := range []int64{95, 92, 90} {
			if _, reject := InjectLimitOrder(ex, 1, "ABC/USD", Sell, price*USD_PRECISION, clip); reject != "" {
				t.Fatalf("own ask %d rejected: %s", index, reject)
			}
			placed = append(placed, lastOrderID(t, ex, 1))
		}
		drainPending(gw)

		if resp := ex.PlaceOrder(1, &OrderRequest{
			RequestID: 1, Symbol: "ABC/USD", Side: Buy, Type: LimitOrder,
			Price: 110 * USD_PRECISION, Qty: clip, TimeInForce: GTC,
		}); !resp.Success {
			t.Fatalf("self-crossing buy refused: %q", resp.Error)
		}

		cancelled := collectForcedCancels(gw, len(placed))
		if len(cancelled) != len(placed) {
			t.Fatalf("%d quotes were withdrawn but %d cancellations were delivered: %v",
				len(placed), len(cancelled), cancelled)
		}
		for index := range placed {
			if cancelled[index] != placed[index] {
				t.Fatalf("cancellations arrived as %v, want placement order %v: map iteration is reaching the evidence stream",
					cancelled, placed)
			}
		}
	})
}
