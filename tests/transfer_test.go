package exchange_test

import (
	. "exchange_sim/exchange"
	"testing"
)

func setupTransferExchange() *Exchange {
	ex := NewExchange(10, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
	ex.ConnectClient(1, map[string]int64{"USD": USDAmount(10_000), "BTC": BTCAmount(1)}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(5_000))
	return ex
}

func TestTransfer_SpotToPerp(t *testing.T) {
	ex := setupTransferExchange()
	spotBefore := ex.Clients[1].Balances["USD"]
	perpBefore := ex.Clients[1].PerpBalances["USD"]
	amount := USDAmount(1_000)

	if err := ex.Transfer(1, "spot", "perp", "USD", amount); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ex.Clients[1].Balances["USD"] != spotBefore-amount {
		t.Errorf("spot balance: expected %d, got %d", spotBefore-amount, ex.Clients[1].Balances["USD"])
	}
	if ex.Clients[1].PerpBalances["USD"] != perpBefore+amount {
		t.Errorf("perp balance: expected %d, got %d", perpBefore+amount, ex.Clients[1].PerpBalances["USD"])
	}
}

func TestTransfer_PerpToSpot(t *testing.T) {
	ex := setupTransferExchange()
	spotBefore := ex.Clients[1].Balances["USD"]
	perpBefore := ex.Clients[1].PerpBalances["USD"]
	amount := USDAmount(1_000)

	if err := ex.Transfer(1, "perp", "spot", "USD", amount); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ex.Clients[1].PerpBalances["USD"] != perpBefore-amount {
		t.Errorf("perp balance: expected %d, got %d", perpBefore-amount, ex.Clients[1].PerpBalances["USD"])
	}
	if ex.Clients[1].Balances["USD"] != spotBefore+amount {
		t.Errorf("spot balance: expected %d, got %d", spotBefore+amount, ex.Clients[1].Balances["USD"])
	}
}

func TestTransfer_UnknownClientReturnsError(t *testing.T) {
	ex := setupTransferExchange()
	err := ex.Transfer(999, "spot", "perp", "USD", USDAmount(100))
	if err == nil {
		t.Fatal("expected error for unknown client")
	}
	// TransferError.Error() must return the message (covers the Error() method)
	if err.Error() == "" {
		t.Error("expected non-empty error message")
	}
}

func TestTransfer_InsufficientSpotBalance(t *testing.T) {
	ex := setupTransferExchange()
	err := ex.Transfer(1, "spot", "perp", "USD", USDAmount(999_999))
	if err == nil {
		t.Error("expected error for insufficient spot balance")
	}
}

func TestTransfer_InsufficientPerpBalance(t *testing.T) {
	ex := setupTransferExchange()
	err := ex.Transfer(1, "perp", "spot", "USD", USDAmount(999_999))
	if err == nil {
		t.Error("expected error for insufficient perp balance")
	}
}

func TestTransfer_InvalidWalletType(t *testing.T) {
	ex := setupTransferExchange()
	err := ex.Transfer(1, "unknown", "perp", "USD", USDAmount(100))
	if err == nil {
		t.Error("expected error for invalid wallet type")
	}
}

func TestTransfer_Conservation(t *testing.T) {
	ex := setupTransferExchange()
	totalBefore := ex.Clients[1].Balances["USD"] + ex.Clients[1].PerpBalances["USD"]
	_ = ex.Transfer(1, "spot", "perp", "USD", USDAmount(2_000))
	totalAfter := ex.Clients[1].Balances["USD"] + ex.Clients[1].PerpBalances["USD"]
	if totalBefore != totalAfter {
		t.Errorf("transfer should conserve total: before=%d, after=%d", totalBefore, totalAfter)
	}
}
