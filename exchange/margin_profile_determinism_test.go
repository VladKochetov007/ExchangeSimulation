package exchange

import (
	"strings"
	"testing"
)

// marginProfileUnmarkedBooks lists two USD perps in reverse canonical order,
// neither of them marked and both with empty books, so nothing can price them.
func marginProfileUnmarkedBooks(t *testing.T, ex *DefaultExchange) {
	t.Helper()
	for _, symbol := range []string{"Z-PERP", "A-PERP"} {
		perp := NewPerpFutures(symbol, "ABC", "USD", 1, 1, 1, 1)
		ex.AddInstrument(perp)
		perp.GetFundingRate().MarkAvailable = false
	}
}

// Cross-margin profile construction can report the first unmarked book the
// account is actually exposed to.  The error is part of the ordered execution
// evidence, so the choice must not be made by Go's randomized map iteration
// order.
func TestBuildAccountMarginProfileUsesCanonicalBookOrderForUnavailableMarks(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	defer ex.Shutdown()
	marginProfileUnmarkedBooks(t, ex)

	// The account must hold both books for either to be a candidate: a mark is
	// resolved only where the position exists.
	pm := ex.Positions.(*PositionManager)
	pm.Lock()
	for _, symbol := range []string{"Z-PERP", "A-PERP"} {
		pm.InjectPosition(1, symbol, &Position{
			ClientID: 1, Symbol: symbol, PositionSide: PositionBoth, Size: 5, EntryPrice: 100,
		})
	}
	pm.Unlock()

	for i := 0; i < 100; i++ {
		_, err := ex.buildAccountMarginProfile(1, "USD", "", 0)
		if err == nil || !strings.Contains(err.Error(), "A-PERP") {
			t.Fatalf("profile attempt %d error = %v, want canonical first book A-PERP", i, err)
		}
	}
}

// A book the account holds nothing in contributes zero equity, zero notional
// and zero maintenance whatever its mark is, so its price is not an input to
// this account's solvency and must not fail the profile.  Resolving the mark
// before establishing position relevance made a newly listed contract —
// unmarked for the single tick before its first mark — fail the profile of
// every account in the quote asset and suppress their liquidation decisions.
// In the registered dev-607 configuration that fired at every dated-future
// listing instant.
func TestBuildAccountMarginProfileIgnoresUnmarkedBooksTheAccountDoesNotHold(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	defer ex.Shutdown()
	marginProfileUnmarkedBooks(t, ex)

	// Client 1 holds nothing anywhere.
	profile, err := ex.buildAccountMarginProfile(1, "USD", "", 0)
	if err != nil {
		t.Fatalf("profile for an account with no exposure = %v, want no error", err)
	}
	if profile != (accountMarginProfile{}) {
		t.Fatalf("account with no exposure contributed %+v, want a zero profile", profile)
	}

	// Client 1 holds one of the two books.  The held book is still unpriceable,
	// so fail-closed must still apply — and must name that book, not the other.
	pm := ex.Positions.(*PositionManager)
	pm.Lock()
	pm.InjectPosition(1, "Z-PERP", &Position{
		ClientID: 1, Symbol: "Z-PERP", PositionSide: PositionBoth, Size: 5, EntryPrice: 100,
	})
	pm.Unlock()

	_, err = ex.buildAccountMarginProfile(1, "USD", "", 0)
	if err == nil {
		t.Fatal("profile succeeded while the account's own book was unpriceable")
	}
	if !strings.Contains(err.Error(), "Z-PERP") {
		t.Fatalf("profile error = %v, want the held book Z-PERP", err)
	}
	if strings.Contains(err.Error(), "A-PERP") {
		t.Fatalf("profile error = %v, want no mention of the unheld book A-PERP", err)
	}
}
