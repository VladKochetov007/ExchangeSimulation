package exchange

import (
	"testing"
	"time"
)

// Expiry settlement moves cash. The conservation tracker exists so that a
// balance which changes without a recorded movement is impossible to hide, and
// its contract is stated on logBalanceChange: the movement is recorded before
// the logger is consulted, so verification never depends on the logging
// configuration.
//
// The settlement path builds its own balance_change event rather than calling
// that helper, because the settlement record carries PositionSide and the
// helper's does not. Building the event by hand also skipped the recording, so
// every expiry left the tracker behind by the settled cash and VerifyConservation
// reported a violation from the first expiry onward — permanently, and for a
// movement that had actually been logged.
//
// The consequence is worse than a spurious report. Once the baseline is wrong
// the tracker can no longer detect the thing it exists for: a later genuinely
// unrecorded mutation is indistinguishable from the gap the settlement already
// opened.
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

	// The two sides carry different bases. That matters for what this test can
	// see: the tracker compares per-asset totals, so a settlement whose cash
	// nets to zero across the book moves neither total and hides an omitted
	// recording entirely. Opposite sizes at different entry prices make the
	// settlement cash sum non-zero, which is what makes the missing record
	// observable at all. It is a device for observability, not a claim that
	// matched trading would produce this basis.
	for _, id := range []uint64{1, 2} {
		ex.ConnectNewClient(id, nil, &FixedFee{})
		ex.AddPerpBalance(id, "USD", 1000*valuationQuotePrecision)
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
