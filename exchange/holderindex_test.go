package exchange

import (
	"slices"
	"testing"

	etypes "exchange_sim/types"
)

// The holder index lets the risk sweep visit only the clients that can do work
// in a symbol. It is safe only while it never omits a client the dense scan
// would have reached and acted on, so these tests hold it to a full scan.

// holdersByScan is the answer the dense sweep computed implicitly: every client
// with a position entry in the symbol. The sweep walks sortedClientIDs, so the
// scan is taken in ascending order to match the order the index must produce.
func holdersByScan(manager *PositionManager, symbol string, clients []uint64) []uint64 {
	ordered := append([]uint64(nil), clients...)
	slices.Sort(ordered)
	var found []uint64
	for _, clientID := range ordered {
		for _, side := range positionSideOrder {
			if manager.GetPositionBySide(clientID, symbol, side) != nil {
				found = append(found, clientID)
				break
			}
		}
	}
	return found
}

func TestHolderIndexMatchesAFullScan(t *testing.T) {
	manager := NewPositionManager(&RealClock{})
	clients := []uint64{5, 1, 9, 3, 7}

	// Nothing yet.
	if got := manager.HoldersOfSymbol("ABC-PERP"); len(got) != 0 {
		t.Fatalf("empty manager reported holders %v", got)
	}

	manager.UpdatePosition(5, "ABC-PERP", 3, 100, Buy, PositionBoth)
	manager.UpdatePosition(1, "ABC-PERP", 2, 100, Buy, PositionLong)
	manager.UpdatePosition(9, "CDF-PERP", 4, 100, Buy, PositionBoth)
	manager.UpdatePosition(3, "ABC-PERP", 1, 100, Sell, PositionShort)

	for _, symbol := range []string{"ABC-PERP", "CDF-PERP", "NONE-PERP"} {
		want := holdersByScan(manager, symbol, clients)
		got := manager.HoldersOfSymbol(symbol)
		if !slices.Equal(got, want) {
			t.Fatalf("%s: index %v, full scan %v", symbol, got, want)
		}
		if !slices.IsSorted(got) {
			t.Fatalf("%s: index is not ascending: %v", symbol, got)
		}
	}
}

// TestHolderIndexKeepsZeroSizedPositions is the property that makes the index
// safe to substitute for a full scan. A closed position leaves a zero-size entry
// behind, and the sweep applies its own size check; an index that dropped those
// entries would change which accounts the sweep considers.
func TestHolderIndexKeepsZeroSizedPositions(t *testing.T) {
	manager := NewPositionManager(&RealClock{})
	manager.UpdatePosition(4, "ABC-PERP", 5, 100, Buy, PositionBoth)
	manager.UpdatePosition(4, "ABC-PERP", 5, 100, Sell, PositionBoth) // close it

	if pos := manager.GetPositionBySide(4, "ABC-PERP", PositionBoth); pos == nil {
		t.Fatal("closing the position removed its entry; this test needs revisiting")
	} else if pos.Size != 0 {
		t.Fatalf("position size after closing = %d, want 0", pos.Size)
	}
	if got := manager.HoldersOfSymbol("ABC-PERP"); !slices.Equal(got, []uint64{4}) {
		t.Fatalf("index dropped a client whose position closed to zero: %v", got)
	}
}

// TestHolderIndexRecordsEachClientOnce guards the in-place insert: repeated
// trades must not duplicate a client, which would make the sweep visit it twice
// and double-count its exposure.
func TestHolderIndexRecordsEachClientOnce(t *testing.T) {
	manager := NewPositionManager(&RealClock{})
	for i := 0; i < 20; i++ {
		manager.UpdatePosition(7, "ABC-PERP", 1, 100, Buy, PositionBoth)
		manager.UpdatePosition(2, "ABC-PERP", 1, 100, Buy, PositionLong)
	}
	got := manager.HoldersOfSymbol("ABC-PERP")
	if !slices.Equal(got, []uint64{2, 7}) {
		t.Fatalf("index = %v, want [2 7] with no duplicates", got)
	}
}

// TestHolderIndexStaysSortedUnderArbitraryInsertOrder exercises the in-place
// insert across an order that would break a naive append.
func TestHolderIndexStaysSortedUnderArbitraryInsertOrder(t *testing.T) {
	manager := NewPositionManager(&RealClock{})
	for _, clientID := range []uint64{50, 10, 90, 30, 1, 70, 20} {
		manager.UpdatePosition(clientID, "ABC-PERP", 1, 100, Buy, PositionBoth)
	}
	got := manager.HoldersOfSymbol("ABC-PERP")
	want := []uint64{1, 10, 20, 30, 50, 70, 90}
	if !slices.Equal(got, want) {
		t.Fatalf("index = %v, want %v", got, want)
	}
}

// TestInjectedPositionsAreIndexed covers the third entry-creation site, which a
// test fixture uses; missing it would hide injected positions from the sweep.
func TestInjectedPositionsAreIndexed(t *testing.T) {
	manager := NewPositionManager(&RealClock{})
	manager.InjectPosition(11, "ABC-PERP", &Position{
		ClientID: 11, Symbol: "ABC-PERP", PositionSide: PositionBoth, Size: 5, EntryPrice: 100,
	})
	if got := manager.HoldersOfSymbol("ABC-PERP"); !slices.Equal(got, []uint64{11}) {
		t.Fatalf("injected position not indexed: %v", got)
	}
}

// TestPositionManagerImplementsSymbolHolderIndex keeps the extension wired:
// without it the sweep silently falls back to scanning every client, which is
// correct but forfeits the optimization with no signal.
func TestPositionManagerImplementsSymbolHolderIndex(t *testing.T) {
	var store etypes.PositionStore = NewPositionManager(&RealClock{})
	if _, ok := store.(SymbolHolderIndex); !ok {
		t.Fatal("PositionManager no longer implements SymbolHolderIndex")
	}
}
