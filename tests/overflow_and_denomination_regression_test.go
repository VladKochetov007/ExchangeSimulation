package exchange_test

import (
	"math"
	"testing"
	"time"

	. "exchange_sim/exchange"
)

func TestRegressionOverflowingDerivativeMarginCannotCreateCollateral(t *testing.T) {
	tests := []struct {
		name string
		inst Instrument
	}{
		{
			name: "perpetual",
			inst: NewPerpFutures("ABC-PERP", "ABC", "USD", BTC_PRECISION, USD_PRECISION, 1, 1),
		},
		{
			name: "option",
			inst: NewEuropeanOption("ABC-C", "ABC", "USD", "ABC/USD", BTC_PRECISION, USD_PRECISION, 1, 1, 1, time.Now().Add(time.Hour).UnixNano(), true),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ex := NewExchange(1, &RealClock{})
			defer ex.Shutdown()
			ex.AddInstrument(tc.inst)
			ex.ConnectNewClient(1, nil, &FixedFee{})

			resp := ex.PlaceOrder(1, &OrderRequest{
				RequestID: 1, Symbol: tc.inst.Symbol(), Side: Sell, Type: LimitOrder,
				Price: math.MaxInt64 - 1, Qty: BTC_PRECISION, TimeInForce: GTC, Visibility: Normal,
			})
			if resp.Success {
				t.Fatalf("overflowing margin order was accepted: %#v", resp)
			}
			if got := ex.Clients[1].PerpReserved["USD"]; got < 0 {
				t.Fatalf("negative perp reservation %d created collateral", got)
			}
		})
	}
}

func TestRegressionRejectLocalDerivativeWithMismatchedSettlementDenomination(t *testing.T) {
	ex := NewExchange(1, &RealClock{})
	defer ex.Shutdown()
	ex.AddInstrument(NewSpotInstrument("ABC/USD", "ABC", "USD", BTC_PRECISION, USD_PRECISION, 1, 1))

	wrongCurrency := NewEuropeanOption("ABC-EUR-C", "ABC", "EUR", "ABC/USD", BTC_PRECISION, USD_PRECISION, 1, 1, 90*USD_PRECISION, time.Now().Add(time.Hour).UnixNano(), true)
	ex.AddInstrument(wrongCurrency)
	if ex.Books[wrongCurrency.Symbol()] != nil {
		t.Fatal("accepted EUR option against USD underlying without an FX conversion")
	}

	wrongPrecision := NewEuropeanOption("ABC-USD-C", "ABC", "USD", "ABC/USD", BTC_PRECISION, 100, 1, 1, 90*100, time.Now().Add(time.Hour).UnixNano(), true)
	ex.AddInstrument(wrongPrecision)
	if ex.Books[wrongPrecision.Symbol()] != nil {
		t.Fatal("accepted derivative with a mismatched quote precision")
	}
}

func TestRegressionPositionExposureOverflowRejectsBeforeMatching(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	defer ex.Shutdown()
	perp := NewPerpFutures("ABC-PERP", "ABC", "USD", 1, 1, 1, 1)
	perp.MarginRate = 0
	ex.AddInstrument(perp)
	ex.ConnectNewClient(1, nil, &FixedFee{})
	ex.ConnectNewClient(2, nil, &FixedFee{})
	ex.Positions.(*PositionManager).InjectPosition(1, "ABC-PERP", &Position{
		ClientID: 1, Symbol: "ABC-PERP", Size: math.MaxInt64, EntryPrice: 1,
	})

	resp := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 1, Symbol: "ABC-PERP", Side: Buy, Type: LimitOrder,
		Price: 1, Qty: 1, TimeInForce: GTC, Visibility: Normal,
	})
	if resp.Success || resp.Error != RejectExceedsPosition {
		t.Fatalf("overflowing opening order = %#v, want RejectExceedsPosition", resp)
	}
	if pos := ex.Positions.GetPosition(1, "ABC-PERP"); pos == nil || pos.Size != math.MaxInt64 {
		t.Fatalf("position mutated before matching: %#v", pos)
	}
	if len(ex.Books["ABC-PERP"].Bids.Orders) != 0 {
		t.Fatal("rejected overflow order reached the book")
	}
}
