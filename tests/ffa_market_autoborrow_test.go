package exchange_test

import (
	"testing"

	. "exchange_sim/exchange"
)

// Regression: with AutoBorrowSpot enabled, a spot MARKET order must be able to
// draw on margin exactly like the economically identical LIMIT order.
//
// checkMarketOrderFunds compared free balance directly and never reached
// tryReserveOrBorrow, so a margin-funded market sell was rejected with
// INSUFFICIENT_BALANCE while the same limit sell was accepted. In the
// multi-venue ecology this silently disabled every option-dealer spot hedge
// (the dealer holds no base inventory), turning a "hedged" treatment arm into
// an unhedged one without any error surfacing.
func TestRegressionSpotMarketOrderUsesAutoBorrow(t *testing.T) {
	newVenue := func(t *testing.T) *DefaultExchange {
		t.Helper()
		ex := NewExchange(2, &RealClock{})
		ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
		// Maker holds inventory and posts a bid the seller can hit.
		ex.ConnectNewClient(1, map[string]int64{"USD": USDAmount(1_000_000), "BTC": BTCAmount(10)}, &FixedFee{})
		// Seller holds only cash: any sale must be financed by borrowing BTC.
		ex.ConnectNewClient(2, map[string]int64{"USD": USDAmount(1_000_000)}, &FixedFee{})
		if err := ex.EnableBorrowing(BorrowingConfig{
			Enabled:           true,
			AutoBorrowSpot:    true,
			DefaultMarginMode: CrossMargin,
			CollateralFactors: map[string]float64{"USD": 1, "BTC": 1},
			MaxBorrowPerAsset: map[string]int64{"USD": USDAmount(1_000_000), "BTC": BTCAmount(100)},
			AssetPrecisions:   map[string]int64{"USD": USD_PRECISION, "BTC": BTC_PRECISION},
			PriceSource:       NewStaticPriceOracle(map[string]int64{"USD": USD_PRECISION, "BTC": USDAmount(100)}),
		}); err != nil {
			t.Fatal(err)
		}
		bid := ex.PlaceOrder(1, &OrderRequest{
			RequestID: 1, Symbol: "BTC/USD", Side: Buy, Type: LimitOrder,
			Price: USDAmount(100), Qty: BTCAmount(1), TimeInForce: GTC, Visibility: Normal,
		})
		if !bid.Success {
			t.Fatalf("maker bid rejected: %v", bid.Error)
		}
		return ex
	}

	// Control: the limit path already finances the sale through auto-borrow.
	limitVenue := newVenue(t)
	limitResp := limitVenue.PlaceOrder(2, &OrderRequest{
		RequestID: 2, Symbol: "BTC/USD", Side: Sell, Type: LimitOrder,
		Price: USDAmount(100), Qty: BTCAmount(1), TimeInForce: GTC, Visibility: Normal,
	})
	if !limitResp.Success {
		t.Fatalf("control limit sell rejected: %v", limitResp.Error)
	}

	// Treatment: the market path must behave the same way.
	marketVenue := newVenue(t)
	marketResp := marketVenue.PlaceOrder(2, &OrderRequest{
		RequestID: 2, Symbol: "BTC/USD", Side: Sell, Type: Market,
		Qty: BTCAmount(1), TimeInForce: IOC, Visibility: Normal,
	})
	if !marketResp.Success {
		t.Fatalf("margin-funded spot market sell rejected: %v", marketResp.Error)
	}

	seller := marketVenue.Clients[2]
	if got := seller.Borrowed["BTC"]; got != BTCAmount(1) {
		t.Fatalf("expected 1 BTC borrowed to finance the sale, got %d", got)
	}
	if got := seller.Balances["BTC"]; got != 0 {
		t.Fatalf("borrowed base must be fully sold, leftover=%d", got)
	}
	if got := seller.Balances["USD"]; got != USDAmount(1_000_100) {
		t.Fatalf("sale proceeds not credited: USD=%d", got)
	}
}

// A market order that no amount of permitted borrowing can fund must still be
// rejected: auto-borrow relaxes funding, it does not remove the limit.
func TestRegressionSpotMarketOrderRejectsBeyondBorrowLimit(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
	ex.ConnectNewClient(1, map[string]int64{"USD": USDAmount(1_000_000), "BTC": BTCAmount(10)}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{"USD": USDAmount(1_000_000)}, &FixedFee{})
	if err := ex.EnableBorrowing(BorrowingConfig{
		Enabled:           true,
		AutoBorrowSpot:    true,
		DefaultMarginMode: CrossMargin,
		CollateralFactors: map[string]float64{"USD": 1, "BTC": 1},
		// Borrowing BTC is capped below the amount this sale would need.
		MaxBorrowPerAsset: map[string]int64{"USD": USDAmount(1_000_000), "BTC": BTCAmount(1) / 2},
		AssetPrecisions:   map[string]int64{"USD": USD_PRECISION, "BTC": BTC_PRECISION},
		PriceSource:       NewStaticPriceOracle(map[string]int64{"USD": USD_PRECISION, "BTC": USDAmount(100)}),
	}); err != nil {
		t.Fatal(err)
	}
	if bid := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 1, Symbol: "BTC/USD", Side: Buy, Type: LimitOrder,
		Price: USDAmount(100), Qty: BTCAmount(1), TimeInForce: GTC, Visibility: Normal,
	}); !bid.Success {
		t.Fatalf("maker bid rejected: %v", bid.Error)
	}

	resp := ex.PlaceOrder(2, &OrderRequest{
		RequestID: 2, Symbol: "BTC/USD", Side: Sell, Type: Market,
		Qty: BTCAmount(1), TimeInForce: IOC, Visibility: Normal,
	})
	if resp.Success {
		t.Fatal("unfundable market sell must be rejected")
	}
	seller := ex.Clients[2]
	if seller.Borrowed["BTC"] != 0 {
		t.Fatalf("rejected order must not leave debt: %d", seller.Borrowed["BTC"])
	}
	if seller.Balances["BTC"] != 0 {
		t.Fatalf("rejected order must not credit base: %d", seller.Balances["BTC"])
	}
}
