package exchange_test

import (
	"testing"

	. "exchange_sim/exchange"
)

// H-005. Collateral released more than once, or released without a matching
// reservation.
//
// Why this is an actor-fairness question and not only an accounting one:
// PerpAvailable is PerpBalances minus PerpReserved. Under-counting the
// reservation overstates available margin, so an account gets buying power its
// capital does not support — free leverage that other participants are not
// given. Client.ReleasePerp clamps at max(0, reserved-amount), so an
// over-release is absorbed silently and leaves no trace, and RT-004 established
// that the conservation tracker cannot see it either: reservations are an
// earmark inside PerpBalances, not a separate pot, so no total moves.
//
// The invariant (INV-8): after every order is gone, the earmark must be back to
// what open positions genuinely require, and never below it.
func TestAuditCollateralReleaseIsNotDoubleCounted(t *testing.T) {
	newFixture := func(t *testing.T) (*DefaultExchange, *PerpFutures) {
		t.Helper()
		ex := NewExchange(3, &RealClock{})
		perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
		ex.AddInstrument(perp)
		ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
		ex.ConnectNewClient(2, map[string]int64{}, &FixedFee{})
		ex.AddPerpBalance(1, "USD", USDAmount(10_000))
		ex.AddPerpBalance(2, "USD", USDAmount(10_000))
		return ex, perp
	}

	t.Run("cancelling twice does not release the earmark twice", func(t *testing.T) {
		ex, _ := newFixture(t)
		before := ex.Clients[1].PerpReserved["USD"]

		resp := ex.PlaceOrder(1, &OrderRequest{
			RequestID: 1, Symbol: "BTC-PERP", Side: Buy, Type: LimitOrder,
			Price: USDAmount(100), Qty: BTCAmount(1), TimeInForce: GTC, Visibility: Normal,
		})
		if !resp.Success {
			t.Fatalf("resting order rejected: %#v", resp)
		}
		reserved := ex.Clients[1].PerpReserved["USD"]
		if reserved <= before {
			t.Fatalf("order reserved nothing: %d", reserved)
		}
		orderID := lastOrderID(t, ex, 1)

		if r := ex.CancelOrder(1, &CancelRequest{RequestID: 2, OrderID: orderID}); !r.Success {
			t.Fatalf("first cancel failed: %#v", r)
		}
		afterFirst := ex.Clients[1].PerpReserved["USD"]

		// The second cancel must not free anything: the order is already gone.
		ex.CancelOrder(1, &CancelRequest{RequestID: 3, OrderID: orderID})
		if got := ex.Clients[1].PerpReserved["USD"]; got != afterFirst {
			t.Errorf("second cancel moved the earmark from %d to %d: collateral released twice",
				afterFirst, got)
		}
		if afterFirst != before {
			t.Errorf("earmark after cancel = %d, want %d restored", afterFirst, before)
		}
	})

	t.Run("cancelling a filled order does not release its earmark again", func(t *testing.T) {
		ex, _ := newFixture(t)

		resp := ex.PlaceOrder(1, &OrderRequest{
			RequestID: 1, Symbol: "BTC-PERP", Side: Buy, Type: LimitOrder,
			Price: USDAmount(100), Qty: BTCAmount(1), TimeInForce: GTC, Visibility: Normal,
		})
		if !resp.Success {
			t.Fatalf("resting order rejected: %#v", resp)
		}
		orderID := lastOrderID(t, ex, 1)

		// Counterparty crosses and fills it completely.
		if _, reject := InjectLimitOrder(ex, 2, "BTC-PERP", Sell, USDAmount(100), BTCAmount(1)); reject != "" {
			t.Fatalf("crossing order rejected: %s", reject)
		}
		if pos := ex.Positions.GetPosition(1, "BTC-PERP"); pos == nil || pos.Size != BTCAmount(1) {
			t.Fatalf("fill did not open the position: %#v", pos)
		}
		afterFill := ex.Clients[1].PerpReserved["USD"]

		// Cancelling a fully filled order must be a no-op for collateral: the
		// order's reservation became the position's margin at the fill.
		ex.CancelOrder(1, &CancelRequest{RequestID: 2, OrderID: orderID})
		if got := ex.Clients[1].PerpReserved["USD"]; got != afterFill {
			t.Errorf("cancel after fill moved the earmark from %d to %d: the position's margin was released",
				afterFill, got)
		}

		// The economically meaningful consequence: available margin must not
		// exceed what an unencumbered account would have.
		if avail, bal := ex.Clients[1].PerpAvailable("USD"), ex.Clients[1].PerpBalances["USD"]; avail > bal {
			t.Errorf("available %d exceeds balance %d: the account holds free leverage", avail, bal)
		}
	})
}

// lastOrderID returns the most recent order the client still tracks.
func lastOrderID(t *testing.T, ex *DefaultExchange, clientID uint64) uint64 {
	t.Helper()
	ids := ex.Clients[clientID].OrderIDs
	if len(ids) == 0 {
		t.Fatalf("client %d tracks no orders", clientID)
	}
	return ids[len(ids)-1]
}
