package exchange

import (
	"errors"
	"testing"

	efee "exchange_sim/fee"
)

// TestFeePriceUnavailableRejectsBeforeMutation protects the client-facing
// admission boundary: an explicitly configured external price for fees is a
// precondition, not a reason to borrow, reserve, consume an order ID, or let a
// matcher touch the book and then compensate afterwards.
func TestFeePriceUnavailableRejectsBeforeMutation(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	defer ex.Shutdown()
	ex.AddInstrument(NewSpotInstrument("ABC/USD", "ABC", "USD", 1, 1, 1, 1))
	log := &recordingLogger{}
	ex.SetLogger("ABC/USD", log)

	feePlan := &efee.OptionFee{
		TakerUnderlyingBps: 10,
		MakerUnderlyingBps: 10,
		Source: priceSourceFunc(func(symbol string) (int64, error) {
			return 0, errors.Join(ErrNoBookPrice, errors.New("index feed disconnected for "+symbol))
		}),
		SymbolMap: func(_, _ string) string { return "ABC/USD" },
	}
	ex.ConnectNewClient(1, map[string]int64{"ABC": 10}, feePlan)
	client := ex.Clients[1]
	initialBalances := map[string]int64{"ABC": client.Balances["ABC"]}
	initialID := ex.NextOrderID

	response := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 7, Symbol: "ABC/USD", Side: Sell, Type: LimitOrder,
		Price: 10, Qty: 1, TimeInForce: GTC, Visibility: Normal,
	})
	if response.Success || response.Error != RejectPriceUnavailable {
		t.Fatalf("unpriced option-fee admission = %#v, want PRICE_UNAVAILABLE", response)
	}
	if ex.NextOrderID != initialID {
		t.Fatalf("unavailable fee consumed order ID: got %d want %d", ex.NextOrderID, initialID)
	}
	if client.Balances["ABC"] != initialBalances["ABC"] || client.Reserved["ABC"] != 0 || len(client.OrderIDs) != 0 {
		t.Fatalf("unavailable fee mutated client admission state: balances=%v reserved=%v orders=%v", client.Balances, client.Reserved, client.OrderIDs)
	}
	if ex.Books["ABC/USD"].FindOrder(initialID+1) != nil {
		t.Fatal("unavailable fee left an order in the book")
	}
	foundUnavailable := false
	for _, record := range log.records {
		if record.event != "price_unavailable" {
			continue
		}
		event, ok := record.data.(PriceUnavailableEvent)
		if ok && event.Operation == "order_fee_admission" && event.Reason != "" {
			foundUnavailable = true
		}
	}
	if !foundUnavailable {
		t.Fatalf("missing observable fee-price deferral: %#v", log.records)
	}
}

// TestAutoBorrowPriceUnavailableRejectsBeforeMutation covers the other
// client-facing price boundary. Collateral valuation must fail before a loan,
// reservation, order-ID allocation, or book mutation; treating an unpriced
// non-zero asset as absent would otherwise manufacture borrowing capacity.
func TestAutoBorrowPriceUnavailableRejectsBeforeMutation(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	defer ex.Shutdown()
	ex.AddInstrument(NewSpotInstrument("ABC/USD", "ABC", "USD", 1, 1, 1, 1))
	log := &recordingLogger{}
	ex.SetLogger("ABC/USD", log)
	if err := ex.EnableBorrowing(BorrowingConfig{
		Enabled: true, AutoBorrowSpot: true, DefaultMarginMode: CrossMargin,
		PriceSource: priceSourceFunc(func(asset string) (int64, error) {
			if asset == "USD" {
				return 1, nil
			}
			return 0, errors.Join(ErrNoBookPrice, errors.New("missing collateral price for "+asset))
		}),
		AssetPrecisions: map[string]int64{"ABC": 1, "USD": 1},
	}); err != nil {
		t.Fatalf("enable borrowing: %v", err)
	}
	ex.ConnectNewClient(1, map[string]int64{"ABC": 10}, &FixedFee{})
	client := ex.Clients[1]
	initialID := ex.NextOrderID

	response := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 8, Symbol: "ABC/USD", Side: Buy, Type: LimitOrder,
		Price: 10, Qty: 1, TimeInForce: GTC, Visibility: Normal,
	})
	if response.Success || response.Error != RejectPriceUnavailable {
		t.Fatalf("unpriced collateral admission = %#v, want PRICE_UNAVAILABLE", response)
	}
	if ex.NextOrderID != initialID || client.Borrowed["USD"] != 0 || client.Balances["USD"] != 0 || client.Reserved["USD"] != 0 || len(client.OrderIDs) != 0 {
		t.Fatalf("unpriced collateral mutated admission state: id=%d borrowed=%v balances=%v reserved=%v orders=%v", ex.NextOrderID, client.Borrowed, client.Balances, client.Reserved, client.OrderIDs)
	}
	foundUnavailable := false
	for _, record := range log.records {
		if event, ok := record.data.(PriceUnavailableEvent); record.event == "price_unavailable" && ok && event.Operation == "order_admission" && event.Reason != "" {
			foundUnavailable = true
		}
	}
	if !foundUnavailable {
		t.Fatalf("missing observable collateral-price rejection: %#v", log.records)
	}
}
