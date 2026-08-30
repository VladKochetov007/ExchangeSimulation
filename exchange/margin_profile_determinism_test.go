package exchange

import (
	"strings"
	"testing"
)

// A cross-margin profile only needs a mark for books in which the account has
// nonzero exposure. An unrelated unmarked listing must not suppress a
// liquidation on a fully priceable position.
func TestBuildAccountMarginProfileIgnoresUnmarkedZeroExposureBooks(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	defer ex.Shutdown()

	for _, symbol := range []string{"Z-PERP", "A-PERP"} {
		perp := NewPerpFutures(symbol, "ABC", "USD", 1, 1, 1, 1)
		ex.AddInstrument(perp)
		perp.GetFundingRate().MarkAvailable = false
	}

	if _, err := ex.buildAccountMarginProfile(1, "USD", "", 0); err != nil {
		t.Fatalf("unmarked zero-exposure books made profile unavailable: %v", err)
	}

	// Once the account actually holds the canonical first book, its missing
	// mark remains an explicit profile failure rather than being hidden.
	ex.Positions.UpdatePosition(1, "A-PERP", 1, 100, Buy, PositionBoth)
	_, err := ex.buildAccountMarginProfile(1, "USD", "", 0)
	if err == nil || !strings.Contains(err.Error(), "A-PERP") {
		t.Fatalf("exposed unmarked book error = %v, want A-PERP", err)
	}
}

func TestBuildAccountMarginProfileFailsClosedForPendingExposure(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	defer ex.Shutdown()

	active := NewPerpFutures("ACTIVE-PERP", "ABC", "USD", 1, 1, 1, 1)
	pending := NewExpiringFutures("PENDING-FUT", "ABC", "USD", 1, 1, 1, 1, 1)
	ex.AddInstrument(active)
	ex.AddInstrument(pending)
	ex.settlementPending[pending.Symbol()] = expirySettlementPending{
		State: expiryStateSettlementPending, Policy: expiryUnavailableRetryForever,
	}
	ex.Positions.UpdatePosition(1, active.Symbol(), 1, 100, Buy, PositionBoth)
	ex.Positions.UpdatePosition(1, pending.Symbol(), 1, 100, Buy, PositionBoth)

	_, err := ex.buildAccountMarginProfile(1, "USD", active.Symbol(), 100)
	if err == nil || !strings.Contains(err.Error(), "PENDING-FUT") {
		t.Fatalf("pending exposure was treated as zero: %v", err)
	}
}
