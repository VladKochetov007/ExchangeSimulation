package exchange

import (
	"testing"

	etypes "exchange_sim/types"
)

// unsidedStore wraps the manager without the optional extension, so the
// per-side fallback path can be exercised.
type unsidedStore struct{ inner *PositionManager }

func (u unsidedStore) UpdatePosition(clientID uint64, symbol string, qty, price int64, tradeSide Side, posSide PositionSide) PositionDelta {
	return u.inner.UpdatePosition(clientID, symbol, qty, price, tradeSide, posSide)
}
func (u unsidedStore) GetPosition(clientID uint64, symbol string) *Position {
	return u.inner.GetPosition(clientID, symbol)
}
func (u unsidedStore) GetPositionBySide(clientID uint64, symbol string, posSide PositionSide) *Position {
	return u.inner.GetPositionBySide(clientID, symbol, posSide)
}
func (u unsidedStore) HasOpenPositions(clientID uint64) bool {
	return u.inner.HasOpenPositions(clientID)
}
func (u unsidedStore) CalculateOpenInterest(symbol string) int64 {
	return u.inner.CalculateOpenInterest(symbol)
}
func (u unsidedStore) PositionsForFunding(symbol string, fn func(clientID uint64, pos Position)) {
	u.inner.PositionsForFunding(symbol, fn)
}
func (u unsidedStore) GetAllPositions(clientID uint64) []Position {
	return u.inner.GetAllPositions(clientID)
}

// requireSameSides holds the batched read to three separate per-side reads,
// which is the equivalence the risk paths depend on.
func requireSameSides(t *testing.T, manager *PositionManager, clientID uint64, symbol string) {
	t.Helper()
	batched := positionsAcrossSides(manager, clientID, symbol)
	fallback := positionsAcrossSides(unsidedStore{inner: manager}, clientID, symbol)
	for i, side := range positionSideOrder {
		single := manager.GetPositionBySide(clientID, symbol, side)
		for name, got := range map[string]*Position{"batched": batched[i], "fallback": fallback[i]} {
			switch {
			case single == nil && got != nil:
				t.Fatalf("%s side %v: got %+v, per-side read found none", name, side, *got)
			case single != nil && got == nil:
				t.Fatalf("%s side %v: got none, per-side read found %+v", name, side, *single)
			case single != nil && *got != *single:
				t.Fatalf("%s side %v: got %+v, per-side read got %+v", name, side, *got, *single)
			}
		}
	}
}

// TestPositionsAcrossSidesMatchesPerSideReads covers the states the risk probe
// meets: a client with nothing, a client with positions in another symbol, and
// a client holding one, two or all three sides.
func TestPositionsAcrossSidesMatchesPerSideReads(t *testing.T) {
	manager := NewPositionManager(&RealClock{})

	// Nothing at all.
	requireSameSides(t, manager, 1, "ABC-PERP")

	// Positions in a different symbol only.
	manager.UpdatePosition(1, "CDF-PERP", 5, 100, Buy, PositionBoth)
	requireSameSides(t, manager, 1, "ABC-PERP")
	requireSameSides(t, manager, 1, "CDF-PERP")

	// One side.
	manager.UpdatePosition(2, "ABC-PERP", 7, 200, Buy, PositionLong)
	requireSameSides(t, manager, 2, "ABC-PERP")

	// Two sides.
	manager.UpdatePosition(2, "ABC-PERP", 3, 210, Sell, PositionShort)
	requireSameSides(t, manager, 2, "ABC-PERP")

	// All three.
	manager.UpdatePosition(2, "ABC-PERP", 2, 205, Buy, PositionBoth)
	requireSameSides(t, manager, 2, "ABC-PERP")

	// A client that never traded.
	requireSameSides(t, manager, 99, "ABC-PERP")
}

// TestPositionsAcrossSidesReturnsIndependentCopies pins the property that lets
// callers hold the result: mutating the store afterwards must not change what
// was already returned, exactly as GetPositionBySide guarantees.
func TestPositionsAcrossSidesReturnsIndependentCopies(t *testing.T) {
	manager := NewPositionManager(&RealClock{})
	manager.UpdatePosition(1, "ABC-PERP", 10, 100, Buy, PositionBoth)

	before := manager.PositionsAcrossSides(1, "ABC-PERP")
	if before[0] == nil {
		t.Fatal("expected a PositionBoth entry")
	}
	sizeBefore := before[0].Size

	manager.UpdatePosition(1, "ABC-PERP", 5, 110, Buy, PositionBoth)
	if before[0].Size != sizeBefore {
		t.Fatalf("returned position tracked a later update: %d became %d", sizeBefore, before[0].Size)
	}
	after := manager.PositionsAcrossSides(1, "ABC-PERP")
	if after[0].Size == sizeBefore {
		t.Fatal("a later read did not observe the update")
	}
}

// TestPositionsAcrossSidesReportsTheDocumentedOrder pins the side order the
// interface promises, since callers index the result positionally.
func TestPositionsAcrossSidesReportsTheDocumentedOrder(t *testing.T) {
	manager := NewPositionManager(&RealClock{})
	manager.UpdatePosition(1, "ABC-PERP", 4, 100, Buy, PositionLong)

	got := manager.PositionsAcrossSides(1, "ABC-PERP")
	if got[0] != nil {
		t.Fatalf("PositionBoth slot holds %+v, want none", *got[0])
	}
	if got[1] == nil || got[1].PositionSide != PositionLong {
		t.Fatalf("PositionLong slot = %+v", got[1])
	}
	if got[2] != nil {
		t.Fatalf("PositionShort slot holds %+v, want none", *got[2])
	}
	if positionSideOrder != [3]PositionSide{PositionBoth, PositionLong, PositionShort} {
		t.Fatalf("side order = %v", positionSideOrder)
	}
}

// TestPositionManagerImplementsSidedPositionStore keeps the optional extension
// wired: without it every probe silently falls back to three reads.
func TestPositionManagerImplementsSidedPositionStore(t *testing.T) {
	var store etypes.PositionStore = NewPositionManager(&RealClock{})
	if _, ok := store.(SidedPositionStore); !ok {
		t.Fatal("PositionManager no longer implements SidedPositionStore")
	}
	if _, ok := etypes.PositionStore(unsidedStore{inner: NewPositionManager(&RealClock{})}).(SidedPositionStore); ok {
		t.Fatal("a store without the extension reported as implementing it")
	}
}
