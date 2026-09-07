package exchange_test

import (
	"testing"

	. "exchange_sim/exchange"
)

// H-006. Auditing the auditor.
//
// VerifyConservation is the exchange's guard against a balance that changes
// without a recorded movement. This test injects controlled faults and asserts
// which ones it actually catches, so the detector's sensitivity boundary is
// written down rather than assumed. A fault that survives is an audit-coverage
// gap, recorded as such in the research note — not a reason to weaken the fault.
//
// The boundary this establishes: the tracker compares per-asset TOTALS of
// recorded movements against per-asset TOTALS of holdings. It therefore detects
// unrecorded mutations, and only those. It is blind by construction to
//
//   - value moved to the wrong participant, because totals are preserved, and
//   - value destroyed in a way that is faithfully recorded, because the
//     recorded total falls with the held total.
//
// Both blind spots are covered by a different, stronger check: the identity
// InternalNet + ExchangeTake + OpenLinearValue = 0 in research/accounting-audit.md,
// evaluated by `mvanalyze -metric conservation`. Neither check subsumes the
// other, which is why the audit uses both.
func newDetectorFixture(t *testing.T) *DefaultExchange {
	t.Helper()
	ex := NewExchange(3, &RealClock{})
	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(1_000))
	ex.AddPerpBalance(2, "USD", USDAmount(1_000))
	if gap := conservationGap(ex, "USD"); gap != 0 {
		t.Fatalf("fixture is not clean: gap %d", gap)
	}
	return ex
}

func TestAuditConservationDetectorSensitivity(t *testing.T) {
	t.Run("control: an untouched fixture reports nothing", func(t *testing.T) {
		ex := newDetectorFixture(t)
		if gap := conservationGap(ex, "USD"); gap != 0 {
			t.Fatalf("valid control failed: gap %d", gap)
		}
	})

	t.Run("caught: unrecorded credit", func(t *testing.T) {
		ex := newDetectorFixture(t)
		ex.Clients[1].PerpBalances["USD"] += USDAmount(7)
		if gap := conservationGap(ex, "USD"); gap != USDAmount(7) {
			t.Fatalf("unrecorded credit of 7 gave gap %d, want %d", gap, USDAmount(7))
		}
	})

	t.Run("caught: unrecorded debit", func(t *testing.T) {
		ex := newDetectorFixture(t)
		ex.Clients[1].PerpBalances["USD"] -= USDAmount(3)
		if gap := conservationGap(ex, "USD"); gap != -USDAmount(3) {
			t.Fatalf("unrecorded debit of 3 gave gap %d, want %d", gap, -USDAmount(3))
		}
	})

	t.Run("caught: silently cancelled debt", func(t *testing.T) {
		ex := newDetectorFixture(t)
		ex.Clients[1].Borrowed["USD"] = USDAmount(50)
		afterBorrow := conservationGap(ex, "USD")
		// Extinguishing the liability without a recorded repayment raises the
		// account's net holdings, which the tracker sees.
		ex.Clients[1].Borrowed["USD"] = 0
		if got := conservationGap(ex, "USD"); got != afterBorrow+USDAmount(50) {
			t.Fatalf("cancelled debt gave gap %d, want %d", got, afterBorrow+USDAmount(50))
		}
	})

	t.Run("caught: one smallest unit", func(t *testing.T) {
		ex := newDetectorFixture(t)
		ex.Clients[1].PerpBalances["USD"]++
		if gap := conservationGap(ex, "USD"); gap != 1 {
			t.Fatalf("a one-unit credit gave gap %d, want 1", gap)
		}
	})

	// The two below are the documented blind spots. They are asserted as
	// SURVIVING so that the boundary is explicit and a future change that
	// closes them is visible as a test failure rather than going unnoticed.
	t.Run("SURVIVES: value paid to the wrong participant", func(t *testing.T) {
		ex := newDetectorFixture(t)
		amount := USDAmount(250)
		ex.Clients[1].PerpBalances["USD"] -= amount
		ex.Clients[2].PerpBalances["USD"] += amount // recipient 2 was owed nothing
		if gap := conservationGap(ex, "USD"); gap != 0 {
			t.Fatalf("expected the totals-based tracker to be blind here, got gap %d", gap)
		}
	})

	// The "value destroyed but faithfully recorded" case needs the unexported
	// recording helper and lives in exchange/economic_audit_recorded_destruction_test.go.
}
