package exchange_test

import (
	"testing"

	. "exchange_sim/exchange"
)

// H-016. Every exit from an order's reservation lifecycle must leave the
// earmark equal to what the surviving exposure requires.
//
// Available = Balances - Reserved. An earmark that outlives its order removes
// buying power the actor is entitled to; one released too eagerly grants buying
// power its capital does not support. Both are silent: Client.ReleasePerp
// clamps at max(0, reserved-amount) so an over-release leaves no trace, and
// RT-004 established that the conservation tracker cannot see either, because a
// reservation is an earmark inside the balance and no total moves.
//
// H-005 covered the cancel paths. These are the admission-refusal and
// partial-fill paths.
func newReservationFixture(t *testing.T) *DefaultExchange {
	t.Helper()
	ex := NewExchange(4, &RealClock{})
	ex.AddInstrument(NewSpotInstrument(
		"ABC/USD", "ABC", "USD", BTC_PRECISION, USD_PRECISION, USD_PRECISION, BTC_PRECISION/1_000))
	for _, id := range []uint64{1, 2} {
		ex.ConnectNewClient(id, map[string]int64{
			"USD": 1_000_000 * USD_PRECISION,
			"ABC": 1_000 * BTC_PRECISION,
		}, &FixedFee{})
	}
	return ex
}

func reservations(ex *DefaultExchange, clientID uint64) (quote, base int64) {
	return ex.Clients[clientID].Reserved["USD"], ex.Clients[clientID].Reserved["ABC"]
}

func TestAuditRefusedOrderStrandsNoCollateral(t *testing.T) {
	price := int64(100 * USD_PRECISION)
	clip := int64(BTC_PRECISION / 100)

	cases := []struct {
		name  string
		setup func(t *testing.T, ex *DefaultExchange)
		order OrderRequest
		// want is the reject the exchange is expected to produce. A case that
		// stops rejecting is a change in admission semantics, not a pass.
		//
		// Self-trade is deliberately absent from this table. RejectSelfTrade is
		// declared but never produced: the matchers prevent self-execution by
		// skipping the resting order, so the incoming order is accepted and
		// rests. That path leaks no collateral, but it does leave the book
		// crossed against itself, which is H-017 and is measured separately in
		// economic_audit_self_cross_test.go.
		want RejectReason
	}{
		{
			name: "fill-or-kill that cannot be filled completely",
			setup: func(t *testing.T, ex *DefaultExchange) {
				// Only half the quantity is available to take.
				if _, reject := InjectLimitOrder(ex, 2, "ABC/USD", Sell, price, clip/2); reject != "" {
					t.Fatalf("liquidity rejected: %s", reject)
				}
			},
			order: OrderRequest{
				RequestID: 10, Symbol: "ABC/USD", Side: Buy, Type: LimitOrder,
				Price: price, Qty: clip, TimeInForce: FOK,
			},
			want: RejectFOKNotFilled,
		},
		{
			name: "post-only that would cross",
			setup: func(t *testing.T, ex *DefaultExchange) {
				if _, reject := InjectLimitOrder(ex, 2, "ABC/USD", Sell, price, clip); reject != "" {
					t.Fatalf("liquidity rejected: %s", reject)
				}
			},
			order: OrderRequest{
				RequestID: 11, Symbol: "ABC/USD", Side: Buy, Type: LimitOrder,
				Price: price, Qty: clip, TimeInForce: GTC, PostOnly: true,
			},
			want: RejectPostOnlyWouldTake,
		},
		{
			name: "order larger than the balance behind it",
			order: OrderRequest{
				RequestID: 13, Symbol: "ABC/USD", Side: Buy, Type: LimitOrder,
				Price: price, Qty: 1_000_000 * BTC_PRECISION, TimeInForce: GTC,
			},
			want: RejectInsufficientBalance,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			ex := newReservationFixture(t)
			if test.setup != nil {
				test.setup(t, ex)
			}
			// Baseline after setup: the self-trade case legitimately holds an
			// earmark for the actor's own resting order.
			quoteBefore, baseBefore := reservations(ex, 1)

			resp := ex.PlaceOrder(1, &test.order)
			if resp.Success {
				t.Fatalf("expected the order to be refused with %s, it was accepted: %#v", test.want, resp)
			}
			if resp.Error != test.want {
				t.Fatalf("refused with %q, want %q: the admission semantics changed and this case no longer tests what it names",
					resp.Error, test.want)
			}

			quoteAfter, baseAfter := reservations(ex, 1)
			if quoteAfter != quoteBefore {
				t.Errorf("a refused order moved the quote earmark from %d to %d: %d units of buying power %s",
					quoteBefore, quoteAfter, quoteAfter-quoteBefore, strandedOrFreed(quoteAfter-quoteBefore))
			}
			if baseAfter != baseBefore {
				t.Errorf("a refused order moved the base earmark from %d to %d: %d units %s",
					baseBefore, baseAfter, baseAfter-baseBefore, strandedOrFreed(baseAfter-baseBefore))
			}
		})
	}
}

func strandedOrFreed(delta int64) string {
	if delta > 0 {
		return "stranded"
	}
	return "freed without a matching release"
}

// A resting order filled in part must keep an earmark for exactly its
// remainder, and cancelling it afterwards must free exactly that remainder and
// nothing more.
func TestAuditPartiallyFilledOrderKeepsTheRightEarmark(t *testing.T) {
	price := int64(100 * USD_PRECISION)
	clip := int64(BTC_PRECISION / 100)

	ex := newReservationFixture(t)
	idleQuote, idleBase := reservations(ex, 1)

	resp := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 1, Symbol: "ABC/USD", Side: Buy, Type: LimitOrder,
		Price: price, Qty: clip, TimeInForce: GTC,
	})
	if !resp.Success {
		t.Fatalf("resting order rejected: %#v", resp)
	}
	restingQuote, _ := reservations(ex, 1)
	if restingQuote <= idleQuote {
		t.Fatalf("a resting buy reserved nothing: %d", restingQuote)
	}
	orderID := lastOrderID(t, ex, 1)

	// Half of it trades.
	if _, reject := InjectLimitOrder(ex, 2, "ABC/USD", Sell, price, clip/2); reject != "" {
		t.Fatalf("crossing order rejected: %s", reject)
	}
	halfQuote, _ := reservations(ex, 1)

	// The earmark for a half-filled order must be half of what the whole order
	// held, measured against the idle baseline rather than assumed to be zero.
	wantHalf := idleQuote + (restingQuote-idleQuote)/2
	if halfQuote != wantHalf {
		t.Errorf("after a half fill the quote earmark is %d, want %d (%d reserved for a remainder worth half the order)",
			halfQuote, wantHalf, halfQuote-idleQuote)
	}

	// Cancelling the remainder must return the earmark to the idle baseline
	// exactly: not less, which would strand collateral, and not more, which
	// would free collateral the fill had already converted into an asset.
	if r := ex.CancelOrder(1, &CancelRequest{RequestID: 2, OrderID: orderID}); !r.Success {
		t.Fatalf("cancel of the remainder failed: %#v", r)
	}
	finalQuote, finalBase := reservations(ex, 1)
	if finalQuote != idleQuote {
		t.Errorf("after cancelling a half-filled order the quote earmark is %d, want the idle %d: %d units %s",
			finalQuote, idleQuote, finalQuote-idleQuote, strandedOrFreed(finalQuote-idleQuote))
	}
	if finalBase != idleBase {
		t.Errorf("base earmark drifted from %d to %d", idleBase, finalBase)
	}

	// The economic consequence, stated independently of the earmark: available
	// balance must never exceed the balance itself.
	if avail, bal := ex.Clients[1].GetAvailable("USD"), ex.Clients[1].Balances["USD"]; avail > bal {
		t.Errorf("available %d exceeds balance %d: the account holds free buying power", avail, bal)
	}
}
