package exchange

import "testing"

// H-006, second blind spot. Companion to
// tests/economic_audit_detector_sensitivity_test.go, which holds the cases that
// do not need the unexported recorder.
//
// Value destroyed and then faithfully recorded moves the held total and the
// recorded total together, so the gap never changes. The tracker detects
// unrecorded mutations, not unbalanced ones: nothing in it requires a debit to
// have a matching credit. Catching this is the job of the identity check in
// research/accounting-audit.md, not of this tracker.
func TestConservationTrackerIsBlindToRecordedDestruction(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", 1_000_000)

	gapOf := func() int64 {
		for _, v := range ex.VerifyConservation() {
			if v.Asset == "USD" {
				return v.Gap
			}
		}
		return 0
	}
	if gapOf() != 0 {
		t.Fatalf("fixture not clean: gap %d", gapOf())
	}

	const burn = int64(400_000)
	old := ex.Clients[1].PerpBalances["USD"]
	ex.Clients[1].PerpBalances["USD"] = old - burn
	logBalanceChange(ex, 0, 1, "", "audit_destroy", []BalanceDelta{
		{Asset: "USD", Wallet: "perp", OldBalance: old, NewBalance: old - burn, Delta: -burn},
	})

	if got := gapOf(); got != 0 {
		t.Fatalf("expected the tracker to be blind to recorded destruction, got gap %d", got)
	}
	if got := ex.Clients[1].PerpBalances["USD"]; got != old-burn {
		t.Fatalf("fixture did not actually destroy the cash: %d", got)
	}
}
