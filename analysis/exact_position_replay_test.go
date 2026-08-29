package analysis

import "testing"

func TestExactPositionReplayUsesAggregateBasisForMarkedCash(t *testing.T) {
	var replay exactPositionReplay
	trades := []exactPositionTrade{
		{OldSize: 0, OldEntryPrice: 0, NewSize: 3, NewEntryPrice: 100, TradeQty: 3, TradePrice: 100, TradeSide: "BUY"},
		{OldSize: 3, OldEntryPrice: 100, NewSize: 5, NewEntryPrice: 100, TradeQty: 2, TradePrice: 101, TradeSide: "BUY"},
		{OldSize: 5, OldEntryPrice: 100, NewSize: 3, NewEntryPrice: 100, TradeQty: 2, TradePrice: 102, TradeSide: "SELL"},
	}
	for _, trade := range trades {
		if _, err := replay.apply(trade, 1); err != nil {
			t.Fatalf("apply %#v: %v", trade, err)
		}
	}
	if got, ok := replay.unrealizedPnL(103, 1); !ok || got != 8 {
		t.Fatalf("exact marked cash = (%d, %t), want (8, true)", got, ok)
	}
}

func TestExactPositionReplayCarriesFlatLifecycleRemainder(t *testing.T) {
	var replay exactPositionReplay
	trades := []exactPositionTrade{
		{OldSize: 0, OldEntryPrice: 0, NewSize: 2, NewEntryPrice: 0, TradeQty: 2, TradePrice: 0, TradeSide: "BUY"},
		{OldSize: 2, OldEntryPrice: 0, NewSize: 1, NewEntryPrice: 0, TradeQty: 1, TradePrice: 6, TradeSide: "SELL"},
		{OldSize: 1, OldEntryPrice: 0, NewSize: 0, NewEntryPrice: 0, TradeQty: 1, TradePrice: 6, TradeSide: "SELL"},
		{OldSize: 0, OldEntryPrice: 0, NewSize: 1, NewEntryPrice: 0, TradeQty: 1, TradePrice: 0, TradeSide: "BUY"},
	}
	for _, trade := range trades {
		if _, err := replay.apply(trade, 10); err != nil {
			t.Fatalf("apply %#v: %v", trade, err)
		}
	}
	if replay.carryNumerator != 2 {
		t.Fatalf("carry numerator = %d, want 2", replay.carryNumerator)
	}
	if got, ok := replay.unrealizedPnL(8, 10); !ok || got != 1 {
		t.Fatalf("reopened exact marked cash = (%d, %t), want (1, true)", got, ok)
	}
}

func TestExactPositionReplayRejectsPersistedChainMismatch(t *testing.T) {
	var replay exactPositionReplay
	_, err := replay.apply(exactPositionTrade{
		OldSize: 9, OldEntryPrice: 100, NewSize: 10, NewEntryPrice: 100,
		TradeQty: 1, TradePrice: 100, TradeSide: "BUY",
	}, 1)
	if err == nil {
		t.Fatal("replay accepted an old-size chain mismatch")
	}
}

func TestExactPositionReplayRejectsMislabeledHedgeSide(t *testing.T) {
	var replay exactPositionReplay
	_, err := replay.apply(exactPositionTrade{
		OldSize: 0, OldEntryPrice: 0, NewSize: -1, NewEntryPrice: 100,
		TradeQty: 1, TradePrice: 100, TradeSide: "SELL", PositionSide: "LONG",
	}, 1)
	if err == nil {
		t.Fatal("replay accepted a negative LONG position")
	}
}
