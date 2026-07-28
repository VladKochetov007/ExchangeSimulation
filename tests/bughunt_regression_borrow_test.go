package exchange_test

import (
	"testing"

	. "exchange_sim/exchange"
)

// Regression: an auto-borrow for a spot order credits the SPOT wallet and
// records the debt in the single Borrowed ledger. RepayMargin used to debit the
// PERP wallet unconditionally, so the spot-credited loan was permanently
// unrepayable (its cash stranded in spot). Repay must draw from the wallet that
// actually holds the funds.
func TestRegressionSpotCreditedBorrowIsRepayable(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
	ex.ConnectNewClient(1, map[string]int64{"USD": USDAmount(50)}, &FixedFee{})
	if err := ex.EnableBorrowing(BorrowingConfig{
		Enabled:           true,
		AutoBorrowSpot:    true,
		CollateralFactors: map[string]float64{"USD": 1, "BTC": 1},
		AssetPrecisions:   map[string]int64{"USD": USD_PRECISION, "BTC": BTC_PRECISION},
		PriceSource:       NewStaticPriceOracle(map[string]int64{"USD": USD_PRECISION, "BTC": USDAmount(1)}),
	}); err != nil {
		t.Fatal(err)
	}

	// Buy 1 BTC @ 100 USD needs 100 USD but the client holds 50 → auto-borrow 50
	// into the SPOT wallet.
	resp := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 1, Symbol: "BTC/USD", Side: Buy, Type: LimitOrder,
		Price: USDAmount(100), Qty: BTCAmount(1), TimeInForce: GTC, Visibility: Normal,
	})
	if !resp.Success {
		t.Fatalf("auto-borrow buy rejected: %v", resp.Error)
	}
	client := ex.Clients[1]

	// Cancel to free the reserved spot USD so repayment has funds available.
	orderID := client.OrderIDs[0]
	if c := ex.CancelOrder(1, &CancelRequest{RequestID: 2, OrderID: orderID}); !c.Success {
		t.Fatalf("cancel rejected: %v", c.Error)
	}

	debt := client.Borrowed["USD"]
	if debt == 0 {
		t.Fatal("expected an outstanding spot-credited debt to repay")
	}
	if err := ex.RepayMargin(1, "USD", debt); err != nil {
		t.Fatalf("spot-credited debt is unrepayable: %v (spot USD=%d perp USD=%d borrowed=%d)",
			err, client.Balances["USD"], client.PerpBalances["USD"], client.Borrowed["USD"])
	}
	if client.Borrowed["USD"] != 0 {
		t.Fatalf("debt not cleared: borrowed=%d", client.Borrowed["USD"])
	}
	if client.Balances["USD"] < 0 || client.PerpBalances["USD"] < 0 {
		t.Fatalf("repay drove a wallet negative: spot USD=%d perp USD=%d",
			client.Balances["USD"], client.PerpBalances["USD"])
	}
}
