package exchange

import (
	"testing"
	"time"
)

func TestExpirySettlementIsRecordedForConservation(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(2, clock)
	future := NewExpiringFutures(
		"ABC-FUT-CONS", "ABC", "USD", valuationBasePrecision, valuationQuotePrecision,
		valuationQuotePrecision, valuationBasePrecision/100,
		time.Now().Add(-time.Second).UnixNano(),
	)
	future.ObserveSettlement(120*valuationQuotePrecision, clock.NowUnixNano())
	ex.AddInstrument(future)

	for _, clientID := range []uint64{1, 2} {
		ex.ConnectNewClient(clientID, nil, &FixedFee{})
		ex.AddPerpBalance(clientID, "USD", 1000*valuationQuotePrecision)
	}
	ex.Positions.UpdatePosition(1, future.Symbol(), valuationBasePrecision, 100*valuationQuotePrecision, Buy, PositionBoth)
	ex.Positions.UpdatePosition(2, future.Symbol(), valuationBasePrecision, 140*valuationQuotePrecision, Sell, PositionBoth)

	if violations := ex.VerifyConservation(); len(violations) != 0 {
		t.Fatalf("conservation already violated before expiry: %+v", violations)
	}

	ex.CheckExpiries()

	if ex.Instruments[future.Symbol()] != nil {
		t.Fatal("expiry did not settle the future")
	}
	if violations := ex.VerifyConservation(); len(violations) != 0 {
		t.Fatalf("expiry settlement was not recorded for conservation: %+v", violations)
	}
}
