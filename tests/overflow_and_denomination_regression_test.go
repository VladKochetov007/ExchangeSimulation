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
