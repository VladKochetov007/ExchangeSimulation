package exchange

import (
	"math"
	"slices"
	"testing"
)

func TestPositionAccountingUsesExactLotsAndLifecycleComplement(t *testing.T) {
	manager := NewPositionManager(&RealClock{})
	manager.SetPositionPrecision("ABC-PERP", 1)

	manager.UpdatePositionWithAccounting(1, "ABC-PERP", 3, 100, Buy, PositionBoth)
	manager.UpdatePositionWithAccounting(1, "ABC-PERP", 2, 101, Buy, PositionBoth)
	closing, accounting := manager.UpdatePositionWithAccounting(1, "ABC-PERP", 2, 102, Sell, PositionBoth)
	if !accounting.Valid || accounting.RealizedPnL != 3 {
		t.Fatalf("partial close accounting = %#v, want valid realized 3; delta=%#v", accounting, closing)
	}

	position := manager.GetPosition(1, "ABC-PERP")
	if position == nil || position.Size != 3 || position.EntryPrice != 100 {
		t.Fatalf("public position = %#v, want size 3 and weighted display price 100", position)
	}
	marked, ok := manager.PositionUnrealizedPnL(*position, 103, 1)
	if !ok || marked != 8 {
		t.Fatalf("marked remaining value = (%d, %t), want (8, true)", marked, ok)
	}
}

func TestPositionAccountingCarriesSubunitCashAcrossPartialCloses(t *testing.T) {
	manager := NewPositionManager(&RealClock{})
	manager.SetPositionPrecision("ABC-PERP", 10)

	manager.UpdatePositionWithAccounting(1, "ABC-PERP", 2, 0, Buy, PositionBoth)
	_, firstAccounting := manager.UpdatePositionWithAccounting(1, "ABC-PERP", 1, 6, Sell, PositionBoth)
	_, secondAccounting := manager.UpdatePositionWithAccounting(1, "ABC-PERP", 1, 6, Sell, PositionBoth)
	if firstAccounting.RealizedPnL != 0 || secondAccounting.RealizedPnL != 1 {
		t.Fatalf("carried realized cash = (%d, %d), want (0, 1)", firstAccounting.RealizedPnL, secondAccounting.RealizedPnL)
	}

	manager.UpdatePositionWithAccounting(1, "ABC-PERP", 1, 0, Buy, PositionBoth)
	_, reopenedAccounting := manager.UpdatePositionWithAccounting(1, "ABC-PERP", 1, 8, Sell, PositionBoth)
	if reopenedAccounting.RealizedPnL != 1 {
		t.Fatalf("reopen did not retain lifecycle remainder: realized=%d, want 1", reopenedAccounting.RealizedPnL)
	}
}

func TestPositionAccountingHandlesShortsAndFlips(t *testing.T) {
	manager := NewPositionManager(&RealClock{})
	manager.SetPositionPrecision("ABC-PERP", 1)

	manager.UpdatePositionWithAccounting(1, "ABC-PERP", 2, 110, Sell, PositionBoth)
	flip, accounting := manager.UpdatePositionWithAccounting(1, "ABC-PERP", 3, 100, Buy, PositionBoth)
	if !accounting.Valid || accounting.RealizedPnL != 20 {
		t.Fatalf("short flip accounting = %#v, want valid realized 20; delta=%#v", accounting, flip)
	}
	position := manager.GetPosition(1, "ABC-PERP")
	if position == nil || position.Size != 1 || position.EntryPrice != 100 {
		t.Fatalf("flipped position = %#v, want long one at 100", position)
	}
	marked, ok := manager.PositionSettlementCashFlow(*position, 90, 1)
	if !ok || marked != -10 {
		t.Fatalf("flipped position settlement value = (%d, %t), want (-10, true)", marked, ok)
	}
}

func TestPositionAccountingLiquidationUsesAggregateBasis(t *testing.T) {
	manager := NewPositionManager(&RealClock{})
	manager.SetPositionPrecision("ABC-PERP", 1)
	manager.UpdatePositionWithAccounting(1, "ABC-PERP", 3, 100, Buy, PositionBoth)
	manager.UpdatePositionWithAccounting(1, "ABC-PERP", 2, 101, Buy, PositionBoth)
	position := manager.GetPosition(1, "ABC-PERP")
	liquidationPrice, ok := manager.PositionLiquidationPrice(*position, 50, 1)
	if !ok || liquidationPrice != 90 {
		t.Fatalf("exact liquidation price = (%d, %t), want (90, true)", liquidationPrice, ok)
	}
}

func TestPositionAccountingFallsBackForInjectedPositionWithoutHistory(t *testing.T) {
	manager := NewPositionManager(&RealClock{})
	manager.SetPositionPrecision("ABC-PERP", 1)
	manager.Lock()
	manager.InjectPosition(1, "ABC-PERP", &Position{ClientID: 1, Symbol: "ABC-PERP", PositionSide: PositionBoth, Size: 2, EntryPrice: 100})
	manager.Unlock()

	delta, accounting := manager.UpdatePositionWithAccounting(1, "ABC-PERP", 1, 110, Sell, PositionBoth)
	if accounting.Valid {
		t.Fatal("injected position unexpectedly claimed exact accounting")
	}
	if got := realizedPerpPnL(delta.OldSize, delta.OldEntryPrice, 1, 110, Sell, 1); got != 10 {
		t.Fatalf("legacy injected PnL = %d, want 10", got)
	}
}

func TestStrictPositionAccountingRejectsHedgeOvershootBeforeMutation(t *testing.T) {
	manager := NewPositionManager(&RealClock{})
	manager.SetPositionPrecision("ABC-PERP", 1)
	manager.SetRequireExactLinearPositionAccounting(true)
	manager.UpdatePositionWithAccounting(1, "ABC-PERP", 2, 100, Buy, PositionLong)

	panicked := false
	func() {
		defer func() { panicked = recover() != nil }()
		manager.UpdatePositionWithAccounting(1, "ABC-PERP", 3, 90, Sell, PositionLong)
	}()
	if !panicked {
		t.Fatal("strict accounting accepted a hedge overshoot")
	}
	position := manager.GetPositionBySide(1, "ABC-PERP", PositionLong)
	if position == nil || position.Size != 2 {
		t.Fatalf("hedge position mutated after rejected overshoot: %#v", position)
	}
}

func TestStrictPositionAccountingRejectsDirectionInvalidHedgeTransitions(t *testing.T) {
	manager := NewPositionManager(&RealClock{})
	manager.SetPositionPrecision("ABC-PERP", 1)
	manager.SetRequireExactLinearPositionAccounting(true)
	if manager.CanUpdatePositionWithAccounting(1, "ABC-PERP", 1, 100, Sell, PositionLong) {
		t.Fatal("empty long hedge accepted a sell transition")
	}
	if manager.CanUpdatePositionWithAccounting(1, "ABC-PERP", 1, 100, Buy, PositionShort) {
		t.Fatal("empty short hedge accepted a buy transition")
	}
}

func TestPositionAccountingExpiryTransitionIsAtomic(t *testing.T) {
	manager := NewPositionManager(&RealClock{})
	manager.SetPositionPrecision("ABC-FUT-1", 1)
	manager.UpdatePositionWithAccounting(1, "ABC-FUT-1", 2, 100, Buy, PositionBoth)
	position := manager.GetPosition(1, "ABC-FUT-1")
	cash, ok := manager.SettlePositionAtPrice(*position, 110, 1)
	if !ok || cash != 20 {
		t.Fatalf("expiry transition = (%d, %t), want (20, true)", cash, ok)
	}
	if remaining := manager.GetPosition(1, "ABC-FUT-1"); remaining != nil && remaining.Size != 0 {
		t.Fatalf("position remained open after atomic expiry: %#v", remaining)
	}
	if _, ok := manager.SettlePositionAtPrice(*position, 110, 1); ok {
		t.Fatal("second expiry transition unexpectedly succeeded")
	}
}

func TestPositionAccountingAllowsIndependentSymbolsAndHedgeSides(t *testing.T) {
	manager := NewPositionManager(&RealClock{})
	manager.SetPositionPrecision("ABC-PERP", 1)
	manager.SetPositionPrecision("XYZ-PERP", 1)

	if !manager.CanUpdatePositionWithAccounting(7, "XYZ-PERP", 1, 100, Buy, PositionBoth) {
		t.Fatal("new symbol was rejected because another accounting key existed")
	}
	manager.UpdatePositionWithAccounting(7, "ABC-PERP", 1, 100, Buy, PositionBoth)
	manager.UpdatePositionWithAccounting(7, "XYZ-PERP", 1, 100, Buy, PositionBoth)
	manager.UpdatePositionWithAccounting(7, "ABC-PERP", 2, 101, Buy, PositionLong)
	if !manager.CanUpdatePositionWithAccounting(7, "ABC-PERP", 2, 99, Sell, PositionShort) {
		t.Fatal("second hedge side was rejected by unrelated accounting state")
	}
	manager.UpdatePositionWithAccounting(7, "ABC-PERP", 2, 99, Sell, PositionShort)
	if long := manager.GetPositionBySide(7, "ABC-PERP", PositionLong); long == nil || long.Size != 2 {
		t.Fatalf("long hedge side = %#v, want size 2", long)
	}
	if short := manager.GetPositionBySide(7, "ABC-PERP", PositionShort); short == nil || short.Size != -2 {
		t.Fatalf("short hedge side = %#v, want size -2", short)
	}
}

func TestPositionAccountingTerminalizationPreviewMatchesDrain(t *testing.T) {
	manager := NewPositionManager(&RealClock{})
	manager.SetPositionPrecision("ABC-FUT", 10)
	manager.UpdatePositionWithAccounting(1, "ABC-FUT", 2, 0, Buy, PositionBoth)
	manager.UpdatePositionWithAccounting(1, "ABC-FUT", 1, 6, Sell, PositionBoth)
	manager.UpdatePositionWithAccounting(1, "ABC-FUT", 1, 6, Sell, PositionBoth)

	want := []PositionAccountingRounding{{ClientID: 1, Amount: 0, RemainderNumerator: 2}}
	preview, ok := manager.PreviewPositionAccountingTerminalization("ABC-FUT", 100, 10)
	if !ok || !slices.Equal(preview, want) {
		t.Fatalf("terminalization preview = %#v, %t; want %#v, true", preview, ok, want)
	}
	drained, ok := manager.DrainPositionAccountingCarry("ABC-FUT", 10)
	if !ok || !slices.Equal(drained, want) {
		t.Fatalf("drained rounding = %#v, %t; want %#v, true", drained, ok, want)
	}
	if drainedAgain, ok := manager.DrainPositionAccountingCarry("ABC-FUT", 10); !ok || len(drainedAgain) != 0 {
		t.Fatalf("carry survived drain: %#v, %t", drainedAgain, ok)
	}
}

func TestPositionAccountingCommitCarryComparesBeforeClearing(t *testing.T) {
	manager := NewPositionManager(&RealClock{})
	manager.SetPositionPrecision("ABC-FUT", 10)
	manager.UpdatePositionWithAccounting(1, "ABC-FUT", 1, 0, Buy, PositionBoth)
	manager.UpdatePositionWithAccounting(1, "ABC-FUT", 1, 6, Sell, PositionBoth)
	want := []PositionAccountingRounding{{ClientID: 1, Amount: 0, RemainderNumerator: 6}}
	wrong := []PositionAccountingRounding{{ClientID: 1, Amount: 1, RemainderNumerator: -4}}
	if committed, ok := manager.CommitPositionAccountingCarry("ABC-FUT", 10, wrong); ok || committed != nil {
		t.Fatalf("mismatched carry commit succeeded: %#v, %t", committed, ok)
	}
	if preview, ok := manager.PreviewPositionAccountingTerminalization("ABC-FUT", 100, 10); !ok || !slices.Equal(preview, want) {
		t.Fatalf("mismatch cleared carry: %#v, %t", preview, ok)
	}
	preview, ok := manager.PreviewPositionAccountingTerminalization("ABC-FUT", 100, 10)
	if !ok || !slices.Equal(preview, want) {
		t.Fatalf("carry preview = %#v, %t; want %#v, true", preview, ok, want)
	}
	committed, ok := manager.CommitPositionAccountingCarry("ABC-FUT", 10, want)
	if !ok || !slices.Equal(committed, want) {
		t.Fatalf("matching carry commit = %#v, %t; want %#v, true", committed, ok, want)
	}
	if committed, ok := manager.CommitPositionAccountingCarry("ABC-FUT", 10, want); ok || committed != nil {
		t.Fatalf("repeated carry commit succeeded: %#v, %t", committed, ok)
	}
}

func TestPositionAccountingFailedDrainDoesNotMutateCarry(t *testing.T) {
	manager := NewPositionManager(&RealClock{})
	manager.SetPositionPrecision("ABC-FUT", 10)
	manager.UpdatePositionWithAccounting(1, "ABC-FUT", 1, 0, Buy, PositionBoth)
	manager.UpdatePositionWithAccounting(1, "ABC-FUT", 1, 6, Sell, PositionBoth)
	manager.UpdatePositionWithAccounting(1, "ABC-FUT", 1, 0, Buy, PositionBoth)

	if drained, ok := manager.DrainPositionAccountingCarry("ABC-FUT", 10); ok || drained != nil {
		t.Fatalf("drain unexpectedly succeeded with an open position: %#v, %t", drained, ok)
	}
	if position := manager.GetPosition(1, "ABC-FUT"); position == nil || position.Size != 1 {
		t.Fatalf("failed drain changed open position: %#v", position)
	}
	manager.UpdatePositionWithAccounting(1, "ABC-FUT", 1, 0, Sell, PositionBoth)
	drained, ok := manager.DrainPositionAccountingCarry("ABC-FUT", 10)
	if !ok || len(drained) != 1 || drained[0].RemainderNumerator != 6 {
		t.Fatalf("carry was lost after failed drain: %#v, %t", drained, ok)
	}
}

func TestPositionAccountingLiquidationIncludesCarriedRemainder(t *testing.T) {
	manager := NewPositionManager(&RealClock{})
	manager.SetPositionPrecision("ABC-PERP", 10)
	manager.UpdatePositionWithAccounting(1, "ABC-PERP", 1, 0, Buy, PositionBoth)
	manager.UpdatePositionWithAccounting(1, "ABC-PERP", 1, 6, Sell, PositionBoth)
	manager.UpdatePositionWithAccounting(1, "ABC-PERP", 1, 0, Buy, PositionBoth)
	position := manager.GetPosition(1, "ABC-PERP")
	liquidationPrice, ok := manager.PositionLiquidationPrice(*position, 0, 10)
	if !ok || liquidationPrice != 3 {
		t.Fatalf("liquidation price with carry = (%d, %t), want (3, true)", liquidationPrice, ok)
	}
}

func TestStrictExactTransitionRejectsMinIntWithoutMutation(t *testing.T) {
	manager := NewPositionManager(&RealClock{})
	manager.SetPositionPrecision("ABC-PERP", 1)
	manager.SetRequireExactLinearPositionAccounting(true)
	if manager.CanUpdatePositionWithAccounting(1, "ABC-PERP", math.MinInt64, 1, Buy, PositionBoth) {
		t.Fatal("MinInt64 quantity passed exact preflight")
	}
	panicked := false
	func() {
		defer func() { panicked = recover() != nil }()
		manager.UpdatePositionWithAccounting(1, "ABC-PERP", math.MinInt64, 1, Buy, PositionBoth)
	}()
	if !panicked || manager.GetPosition(1, "ABC-PERP") != nil {
		t.Fatalf("MinInt64 transition mutated state: panicked=%t position=%#v", panicked, manager.GetPosition(1, "ABC-PERP"))
	}
}

func TestPositionAccountingDoesNotRegisterOptionsAsLinear(t *testing.T) {
	exchange := NewExchangeWithConfig(ExchangeConfig{
		Clock:                                &RealClock{},
		RequireExactLinearPositionAccounting: true,
	})
	option := NewEuropeanOption("ABC-C-100", "ABC", "USD", "ABC/USD", 1, 1, 1, 1, 100, 1_000_000, true)
	exchange.AddInstrument(option)
	manager := exchange.Positions.(*PositionManager)
	manager.mu.RLock()
	_, registered := manager.precisions[option.Symbol()]
	manager.mu.RUnlock()
	if registered {
		t.Fatal("option received linear exact-accounting precision")
	}
}

func TestPositionAccountingLiquidationUsesDiscreteTowardZeroBoundary(t *testing.T) {
	manager := NewPositionManager(&RealClock{})
	manager.SetPositionPrecision("ABC-PERP", 1)
	manager.UpdatePositionWithAccounting(1, "ABC-PERP", 1, 3, Buy, PositionBoth)
	manager.UpdatePositionWithAccounting(1, "ABC-PERP", 2, 4, Buy, PositionBoth)
	long := manager.GetPosition(1, "ABC-PERP")
	longPrice, ok := manager.PositionLiquidationPrice(*long, 12, 1)
	if !ok || longPrice != -1 {
		t.Fatalf("long discrete liquidation price = (%d, %t), want (-1, true)", longPrice, ok)
	}

	manager.UpdatePositionWithAccounting(2, "ABC-PERP", 3, 11, Sell, PositionBoth)
	short := manager.GetPosition(2, "ABC-PERP")
	shortPrice, ok := manager.PositionLiquidationPrice(*short, 1, 1)
	if !ok || shortPrice != 12 {
		t.Fatalf("short discrete liquidation price = (%d, %t), want (12, true)", shortPrice, ok)
	}
}
