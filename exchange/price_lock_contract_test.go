package exchange

import "testing"

// TestIndexPriceLockedUsesExplicitLockHeldOraclePath guards the lock-order
// boundary that the full fee-simulation integration test exercises: an index
// oracle mapped back to this exchange must not acquire e.mu a second time
// while indexPriceLocked already holds it.
func TestIndexPriceLockedUsesExplicitLockHeldOraclePath(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	defer ex.Shutdown()
	ex.AddInstrument(NewSpotInstrument("ABC/USD", "ABC", "USD", 1, 1, 1, 1))
	ex.AddInstrument(NewPerpFutures("ABC-PERP", "ABC", "USD", 1, 1, 1, 1))
	addBookPriceQuote(t, ex, Buy, 100)
	addBookPriceQuote(t, ex, Sell, 102)

	oracle := NewMidPriceOracle(ex)
	oracle.MapSymbol("ABC-PERP", "ABC/USD")
	ex.ConfigureAutomation(AutomationConfig{IndexProvider: oracle})

	ex.mu.Lock()
	price, err := ex.indexPriceLocked("ABC-PERP")
	ex.mu.Unlock()
	if err != nil || price != 101 {
		t.Fatalf("locked index price = (%d, %v), want (101, nil)", price, err)
	}
}
