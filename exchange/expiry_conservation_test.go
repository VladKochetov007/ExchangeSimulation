package exchange

import (
	"testing"
	"time"
)

func TestExpirySettlementIsRecordedForConservation(t *testing.T) {
	clock := &RealClock{}
	exchange := NewExchange(2, clock)
	future := NewExpiringFutures(
		"ABC-FUT-CONS", "ABC", "USD", valuationBasePrecision, valuationQuotePrecision,
		valuationQuotePrecision, valuationBasePrecision/100,
		time.Now().Add(-time.Second).UnixNano(),
	)
	future.ObserveSettlement(120*valuationQuotePrecision, clock.NowUnixNano())
	exchange.AddInstrument(future)

	for _, clientID := range []uint64{1, 2} {
		exchange.ConnectNewClient(clientID, nil, &FixedFee{})
		exchange.AddPerpBalance(clientID, "USD", 1000*valuationQuotePrecision)
	}
	exchange.Positions.UpdatePosition(1, future.Symbol(), valuationBasePrecision, 100*valuationQuotePrecision, Buy, PositionBoth)
	exchange.Positions.UpdatePosition(2, future.Symbol(), valuationBasePrecision, 140*valuationQuotePrecision, Sell, PositionBoth)

	if violations := exchange.VerifyConservation(); len(violations) != 0 {
		t.Fatalf("conservation already violated before expiry: %+v", violations)
	}

	exchange.CheckExpiries()

	if exchange.Instruments[future.Symbol()] != nil {
		t.Fatal("expiry did not settle the future")
	}
	if violations := exchange.VerifyConservation(); len(violations) != 0 {
		t.Fatalf("expiry settlement was not recorded for conservation: %+v", violations)
	}
}
