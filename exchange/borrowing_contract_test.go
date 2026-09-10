package exchange

import (
	"strings"
	"testing"
)

func TestForbidBorrowingCannotBeReplacedOrBypassedByOrderAdmission(t *testing.T) {
	ex := NewExchangeWithConfig(ExchangeConfig{
		ID:              "strict-test",
		Clock:           &RealClock{},
		ForbidBorrowing: true,
	})
	defer ex.Shutdown()
	ex.AddInstrument(NewSpotInstrument("ABC/USD", "ABC", "USD", 1, 1, 1, 1))
	ex.ConnectNewClient(1, map[string]int64{"ABC": 1}, &FixedFee{})

	if err := ex.EnableBorrowing(BorrowingConfig{
		Enabled:        true,
		AutoBorrowSpot: true,
		PriceSource:    NewStaticPriceOracle(map[string]int64{"USD": 1}),
	}); err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("enabled borrowing under immutable no-debt boundary: %v", err)
	}
	if err := ex.EnableBorrowing(BorrowingConfig{Enabled: false}); err != nil {
		t.Fatalf("installing disabled compatibility manager: %v", err)
	}
	if err := ex.BorrowMargin(1, "USD", 1, "strict-test"); err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("direct borrow under immutable no-debt boundary: %v", err)
	}

	response := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 1, Symbol: "ABC/USD", Side: Buy, Type: LimitOrder,
		Price: 10, Qty: 1, TimeInForce: GTC, Visibility: Normal,
	})
	if response.Success {
		t.Fatal("order admission created a borrowing-backed quote under no-debt contract")
	}
	client := ex.Clients[1]
	if client.Borrowed["USD"] != 0 || client.Balances["USD"] != 0 || client.Reserved["USD"] != 0 {
		t.Fatalf("failed order changed debt or quote wallet: borrowed=%v balances=%v reserved=%v", client.Borrowed, client.Balances, client.Reserved)
	}
	if err := ex.ValidateNoBorrowingDebt(); err != nil {
		t.Fatalf("clean account failed no-debt validation: %v", err)
	}
	client.Borrowed["USD"] = 1
	if err := ex.ValidateNoBorrowingDebt(); err == nil {
		t.Fatal("nonzero borrowing debt passed no-debt validation")
	}
	client.Borrowed["USD"] = 0
	client.BorrowedSpot["USD"] = 1
	if err := ex.ValidateNoBorrowingDebt(); err == nil {
		t.Fatal("nonzero spot debt attribution passed no-debt validation")
	}
}
