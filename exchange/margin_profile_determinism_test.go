package exchange

import (
	"strings"
	"testing"
)

// Cross-margin profile construction can report the first unmarked book.  The
// error is part of the ordered execution evidence, so the choice must not be
// made by Go's randomized map iteration order.
func TestBuildAccountMarginProfileUsesCanonicalBookOrderForUnavailableMarks(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	defer ex.Shutdown()

	for _, symbol := range []string{"Z-PERP", "A-PERP"} {
		perp := NewPerpFutures(symbol, "ABC", "USD", 1, 1, 1, 1)
		ex.AddInstrument(perp)
		perp.GetFundingRate().MarkAvailable = false
	}

	for i := 0; i < 100; i++ {
		_, err := ex.buildAccountMarginProfile(1, "USD", "", 0)
		if err == nil || !strings.Contains(err.Error(), "A-PERP") {
			t.Fatalf("profile attempt %d error = %v, want canonical first book A-PERP", i, err)
		}
	}
}
