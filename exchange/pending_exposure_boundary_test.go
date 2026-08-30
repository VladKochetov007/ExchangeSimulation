package exchange

import (
	"errors"
	"testing"

	etypes "exchange_sim/types"
)

func TestSettlementPendingExposureStopsSiblingOrdersAndBorrow(t *testing.T) {
	clock := &expiryManualClock{now: 100}
	ex := NewExchange(4, clock)
	defer ex.Shutdown()

	active := NewPerpFutures("ACTIVE-PERP", "ABC", "USD", 1, 1, 1, 1)
	pending := NewExpiringFutures("PENDING-FUT", "ABC", "USD", 1, 1, 1, 1, 1)
	ex.AddInstrument(active)
	ex.AddInstrument(pending)
	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", 1_000)
	ex.Positions.UpdatePosition(1, pending.Symbol(), 1, 100, Buy, PositionBoth)
	if err := ex.EnableBorrowing(BorrowingConfig{
		Enabled: true, PriceSource: NewStaticPriceOracle(map[string]int64{"USD": 1}), AutoBorrowPerp: true,
	}); err != nil {
		t.Fatalf("EnableBorrowing: %v", err)
	}

	// No underlying observation exists, so the actual lifecycle transition is
	// settlement-pending rather than a synthetic direct map injection.
	ex.CheckExpiries()
	if _, ok := ex.settlementPending[pending.Symbol()]; !ok {
		t.Fatal("unavailable expiry did not enter settlement-pending state")
	}

	response := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 1, Symbol: active.Symbol(), Side: Buy, Type: LimitOrder,
		Price: 100, Qty: 1, TimeInForce: GTC, Visibility: Normal,
	})
	if response.Success || response.Error != etypes.RejectSettlementPendingExposure {
		t.Fatalf("sibling order after pending transition = %#v, want account-settlement-pending rejection", response)
	}
	if ex.NextOrderID != 1 || len(ex.Books[active.Symbol()].Bids.Orders) != 0 {
		t.Fatal("pending-exposure order rejection mutated the active book")
	}

	oldBalance := ex.Clients[1].PerpBalances["USD"]
	oldDebt := ex.Clients[1].Borrowed["USD"]
	err := ex.BorrowMargin(1, "USD", 1, "pending-exposure-test")
	if !errors.Is(err, ErrSettlementPendingExposure) {
		t.Fatalf("manual borrow after pending transition = %v, want ErrSettlementPendingExposure", err)
	}
	if ex.Clients[1].PerpBalances["USD"] != oldBalance || ex.Clients[1].Borrowed["USD"] != oldDebt {
		t.Fatal("pending-exposure borrow rejection mutated account state")
	}
}
