package exchange_test

import (
	"testing"
	"time"

	. "exchange_sim/exchange"
)

// H-018. An order can be terminated by something the order did not initiate:
// the venue killing an immediate-or-cancel remainder, or the instrument itself
// reaching expiry. Expiry is the sharper case, because the instrument
// disappears: an earmark left behind has nothing to point at and no later
// cancel can reach it, so the buying power is gone permanently with no event to
// explain it.
func TestAuditImmediateOrCancelReleasesTheKilledRemainder(t *testing.T) {
	ex := newReservationFixture(t)
	price := int64(100 * USD_PRECISION)
	clip := int64(BTC_PRECISION / 100)

	// Only half the quantity is available to take.
	if _, reject := InjectLimitOrder(ex, 2, "ABC/USD", Sell, price, clip/2); reject != "" {
		t.Fatalf("liquidity rejected: %s", reject)
	}
	idleQuote, idleBase := reservations(ex, 1)

	resp := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 1, Symbol: "ABC/USD", Side: Buy, Type: LimitOrder,
		Price: price, Qty: clip, TimeInForce: IOC,
	})
	if !resp.Success {
		t.Fatalf("IOC rejected: %#v", resp)
	}

	// Half filled, half killed. Nothing rests, so nothing may stay earmarked.
	quote, base := reservations(ex, 1)
	if quote != idleQuote {
		t.Errorf("the killed IOC remainder left %d quote units earmarked (idle %d): %s",
			quote, idleQuote, strandedOrFreed(quote-idleQuote))
	}
	if base != idleBase {
		t.Errorf("base earmark drifted from %d to %d", idleBase, base)
	}
	if ids := ex.Clients[1].OrderIDs; len(ids) != 0 {
		t.Errorf("the client still tracks %v after an IOC that cannot rest", ids)
	}
	if avail, bal := ex.Clients[1].GetAvailable("USD"), ex.Clients[1].Balances["USD"]; avail > bal {
		t.Errorf("available %d exceeds balance %d", avail, bal)
	}
}

// Expiry with and without a settlement price. The second is the case the
// implementation has to get right twice: it must cancel resting orders on the
// first pass and must not release their collateral again on the retries, and
// ReleasePerp clamps at zero so a double release would leave no trace.
func TestAuditExpiryDoesNotStrandCollateralOfRestingOrders(t *testing.T) {
	const (
		settlementAvailable = true
		settlementMissing   = false
	)
	newFuture := func(t *testing.T, withSettlement bool) (*DefaultExchange, Gateway, string, *testClock) {
		t.Helper()
		// The order has to be admitted while the contract is still live, so the
		// clock has to move rather than the expiry being backdated.
		clock := &testClock{now: time.Now().UnixNano()}
		expiry := clock.now + int64(time.Minute)
		ex := NewExchange(4, clock)
		symbol := "ABC-FUT"
		future := NewExpiringFutures(
			symbol, "ABC", "USD", BTC_PRECISION, USD_PRECISION, USD_PRECISION,
			BTC_PRECISION/1_000, expiry)
		if withSettlement {
			future.ObserveSettlement(100*USD_PRECISION, clock.NowUnixNano())
		}
		ex.AddInstrument(future)
		gw := ex.ConnectNewClient(1, nil, &FixedFee{})
		ex.AddPerpBalance(1, "USD", 10_000*USD_PRECISION)
		return ex, gw, symbol, clock
	}

	for _, test := range []struct {
		name           string
		withSettlement bool
	}{
		{name: "settlement price available", withSettlement: settlementAvailable},
		{name: "settlement price unavailable, contract stays pending", withSettlement: settlementMissing},
	} {
		t.Run(test.name, func(t *testing.T) {
			ex, gw, symbol, clock := newFuture(t, test.withSettlement)
			idle := ex.Clients[1].PerpReserved["USD"]

			// A resting bid well below any plausible settlement, so it cannot
			// fill and can only be terminated by the lifecycle.
			resp := ex.PlaceOrder(1, &OrderRequest{
				RequestID: 1, Symbol: symbol, Side: Buy, Type: LimitOrder,
				Price: 50 * USD_PRECISION, Qty: BTC_PRECISION / 100, TimeInForce: GTC,
			})
			if !resp.Success {
				t.Fatalf("resting order on the future rejected: %#v", resp)
			}
			resting := ex.Clients[1].PerpReserved["USD"]
			if resting <= idle {
				t.Fatalf("the resting order reserved nothing: %d", resting)
			}
			orderID := lastOrderID(t, ex, 1)

			// The contract reaches expiry with the order still resting on it.
			clock.now += int64(2 * time.Minute)
			ex.CheckExpiries()

			if got := ex.Clients[1].PerpReserved["USD"]; got != idle {
				t.Errorf("expiry left %d quote units earmarked for an order on a contract that no longer trades (idle %d): %s",
					got, idle, strandedOrFreed(got-idle))
			}
			if ids := ex.Clients[1].OrderIDs; len(ids) != 0 {
				t.Errorf("the client still tracks %v after its instrument expired", ids)
			}
			if avail, bal := ex.Clients[1].PerpAvailable("USD"), ex.Clients[1].PerpBalances["USD"]; avail > bal {
				t.Errorf("perp available %d exceeds balance %d: collateral was over-released", avail, bal)
			}

			// The owner must learn its order is gone; a dropped forced cancel
			// leaves the actor with a ghost pending order forever.
			var notified bool
			deadline := time.After(2 * time.Second)
		await:
			for !notified {
				select {
				case response := <-gw.Responses():
					if notice, ok := response.Data.(*ForcedCancelNotification); ok && notice.OrderID == orderID {
						notified = true
					}
				case <-deadline:
					break await
				}
			}
			if !notified {
				t.Errorf("no forced cancellation was delivered for order %d", orderID)
			}

			// Repeated passes must be idempotent. For the pending contract this
			// is the real test: CheckExpiries runs again on every tick, and a
			// second release would be absorbed silently by the clamp in
			// ReleasePerp.
			for pass := 0; pass < 3; pass++ {
				ex.CheckExpiries()
			}
			if got := ex.Clients[1].PerpReserved["USD"]; got != idle {
				t.Errorf("repeated expiry passes moved the earmark to %d, want the idle %d", got, idle)
			}
			if got := ex.Clients[1].PerpBalances["USD"]; got < 0 {
				t.Errorf("repeated expiry passes drove the perp balance negative: %d", got)
			}
		})
	}
}
