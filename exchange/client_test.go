package exchange

import "testing"

func TestNewClient(t *testing.T) {
	fee := &PercentageFee{MakerBps: 5, TakerBps: 10, InQuote: true}
	client := NewClient(1, fee)

	if client.ID != 1 {
		t.Error("Client ID should be 1")
	}
	if len(client.Balances) != 0 {
		t.Error("Balances should be empty")
	}
}

func TestAddBalance(t *testing.T) {
	fee := &PercentageFee{MakerBps: 5, TakerBps: 10, InQuote: true}
	client := NewClient(1, fee)

	client.AddBalance("USD", 10000)
	if client.GetBalance("USD") != 10000 {
		t.Errorf("Balance should be 10000, got %d", client.GetBalance("USD"))
	}
}

func TestReserveBalance(t *testing.T) {
	fee := &PercentageFee{MakerBps: 5, TakerBps: 10, InQuote: true}
	client := NewClient(1, fee)
	client.AddBalance("USD", 10000)

	if !client.Reserve("USD", 5000) {
		t.Error("Reserve should succeed")
	}
	if client.GetAvailable("USD") != 5000 {
		t.Errorf("Available should be 5000, got %d", client.GetAvailable("USD"))
	}
	if client.GetReserved("USD") != 5000 {
		t.Errorf("Reserved should be 5000, got %d", client.GetReserved("USD"))
	}
}

func TestReserveInsufficientBalance(t *testing.T) {
	fee := &PercentageFee{MakerBps: 5, TakerBps: 10, InQuote: true}
	client := NewClient(1, fee)
	client.AddBalance("USD", 10000)

	if client.Reserve("USD", 15000) {
		t.Error("Reserve should fail with insufficient balance")
	}
}

func TestReleaseReserve(t *testing.T) {
	fee := &PercentageFee{MakerBps: 5, TakerBps: 10, InQuote: true}
	client := NewClient(1, fee)
	client.AddBalance("USD", 10000)
	client.Reserve("USD", 5000)

	client.Release("USD", 5000)
	if client.GetAvailable("USD") != 10000 {
		t.Errorf("Available should be 10000 after release, got %d", client.GetAvailable("USD"))
	}
	if client.GetReserved("USD") != 0 {
		t.Errorf("Reserved should be 0 after release, got %d", client.GetReserved("USD"))
	}
}

func TestSubBalance(t *testing.T) {
	fee := &PercentageFee{MakerBps: 5, TakerBps: 10, InQuote: true}
	client := NewClient(1, fee)
	client.AddBalance("USD", 10000)

	if !client.SubBalance("USD", 3000) {
		t.Error("SubBalance should succeed")
	}
	if client.GetBalance("USD") != 7000 {
		t.Errorf("Balance should be 7000, got %d", client.GetBalance("USD"))
	}
}

func TestSubBalanceInsufficientFunds(t *testing.T) {
	fee := &PercentageFee{MakerBps: 5, TakerBps: 10, InQuote: true}
	client := NewClient(1, fee)
	client.AddBalance("USD", 10000)

	if client.SubBalance("USD", 15000) {
		t.Error("SubBalance should fail with insufficient funds")
	}
}
