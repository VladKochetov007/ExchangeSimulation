package instrument

import (
	"testing"

	etypes "exchange_sim/types"
)

// strictExactStoreProbe intentionally does not implement
// PositionAccountingPolicy. The settlement context must still prevent a
// strict exchange from falling back to the legacy position update.
type strictExactStoreProbe struct {
	etypes.PositionStore
}

func (strictExactStoreProbe) CanUpdatePositionWithAccounting(uint64, string, int64, int64, etypes.Side, etypes.PositionSide) bool {
	return true
}

func (strictExactStoreProbe) UpdatePositionWithAccounting(uint64, string, int64, int64, etypes.Side, etypes.PositionSide) (etypes.PositionDelta, etypes.PositionAccountingDelta) {
	return etypes.PositionDelta{}, etypes.PositionAccountingDelta{}
}

func (strictExactStoreProbe) PositionUnrealizedPnL(etypes.Position, int64, int64) (int64, bool) {
	return 0, false
}

func (strictExactStoreProbe) CanSettlePositionAtPrice(etypes.Position, int64, int64) bool {
	return false
}

func (strictExactStoreProbe) SettlePositionAtPrice(etypes.Position, int64, int64) (int64, bool) {
	return 0, false
}

func (strictExactStoreProbe) PreviewPositionAccountingTerminalization(string, int64, int64) ([]etypes.PositionAccountingRounding, bool) {
	return nil, true
}

func (strictExactStoreProbe) CommitPositionAccountingCarry(string, int64, []etypes.PositionAccountingRounding) ([]etypes.PositionAccountingRounding, bool) {
	return nil, true
}

func (strictExactStoreProbe) PositionLiquidationPrice(etypes.Position, int64, int64) (int64, bool) {
	return 0, false
}

func (strictExactStoreProbe) DrainPositionAccountingCarry(string, int64) ([]etypes.PositionAccountingRounding, bool) {
	return nil, true
}

func TestStrictSettlementContextRejectsExactStoreFallback(t *testing.T) {
	store := strictExactStoreProbe{}
	panicked := false
	func() {
		defer func() { panicked = recover() != nil }()
		updatePositionWithAccounting(store, true, 1, "ABC-PERP", 1, 100, etypes.Buy, etypes.PositionBoth)
	}()
	if !panicked {
		t.Fatal("strict settlement accepted an exact store that returned invalid accounting")
	}
}
