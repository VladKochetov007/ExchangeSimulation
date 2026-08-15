package feesim

import (
	"context"
	"testing"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

func TestBasisArbExecutableEdgeRejectsMidpointOnlySignal(t *testing.T) {
	arb := &FeeAwareBasisArb{cfg: BasisArbConfig{
		LotSize: 1, BasePrecision: 1, SpotFeeBps: 100, PerpFeeBps: 100,
	}}
	// Midpoints suggest the perp is $2 rich: 102.5 - 100.5. A rich-side
	// capture must actually sell perp at 102 and buy spot at 101, then pay a
	// $1 fee on each leg, so its realized one-lot cashflow is negative.
	arb.spotBid, arb.spotAsk = 100, 101
	arb.perpBid, arb.perpAsk = 102, 103

	edge, ok := arb.executableEdge(exchange.Sell)
	if !ok || edge >= 0 {
		t.Fatalf("midpoint-only rich signal edge = %d, ok=%v; want negative executable edge", edge, ok)
	}
	edge, ok = arb.executableEdge(exchange.Buy)
	if !ok || edge >= 0 {
		t.Fatalf("reverse edge = %d, ok=%v; want negative executable edge", edge, ok)
	}
}

func TestBasisArbReconstructsDisplayedTouchFromSnapshotAndDeltas(t *testing.T) {
	arb := &FeeAwareBasisArb{cfg: BasisArbConfig{SpotSymbol: "ABC/USD", PerpSymbol: "ABC-PERP"}}
	arb.onSnapshot(actor.BookSnapshotEvent{
		Symbol: "ABC/USD",
		Snapshot: &exchange.BookSnapshot{
			Bids: []exchange.PriceLevel{{Price: 100, VisibleQty: 2}, {Price: 99, VisibleQty: 3}},
			Asks: []exchange.PriceLevel{{Price: 102, VisibleQty: 2}, {Price: 103, VisibleQty: 3}},
		},
	})
	if arb.spotBid != 100 || arb.spotAsk != 102 {
		t.Fatalf("snapshot touch = %d/%d, want 100/102", arb.spotBid, arb.spotAsk)
	}

	arb.onDelta(actor.BookDeltaEvent{Symbol: "ABC/USD", Delta: &exchange.BookDelta{
		Side: exchange.Buy, Price: 101, VisibleQty: 1,
	}})
	arb.onDelta(actor.BookDeltaEvent{Symbol: "ABC/USD", Delta: &exchange.BookDelta{
		Side: exchange.Sell, Price: 102, VisibleQty: 0,
	}})
	if arb.spotBid != 101 || arb.spotAsk != 103 {
		t.Fatalf("delta touch = %d/%d, want 101/103", arb.spotBid, arb.spotAsk)
	}
}

func TestBasisArbDoesNotInventQuotesFromTradePrints(t *testing.T) {
	arb := &FeeAwareBasisArb{cfg: BasisArbConfig{SpotSymbol: "ABC/USD", PerpSymbol: "ABC-PERP", Reactive: true}}
	arb.onSnapshot(actor.BookSnapshotEvent{
		Symbol: "ABC/USD",
		Snapshot: &exchange.BookSnapshot{
			Bids: []exchange.PriceLevel{{Price: 100, VisibleQty: 1}},
			Asks: []exchange.PriceLevel{{Price: 102, VisibleQty: 1}},
		},
	})
	arb.HandleEvent(context.Background(), &actor.Event{
		Type: actor.EventTrade,
		Data: actor.TradeEvent{Symbol: "ABC/USD", Trade: &exchange.Trade{Price: 200}},
	})
	if arb.spotBid != 100 || arb.spotAsk != 102 {
		t.Fatalf("trade print altered executable touch to %d/%d", arb.spotBid, arb.spotAsk)
	}
}

func TestBasisArbReportCountsObservedFillsAndFees(t *testing.T) {
	arb := &FeeAwareBasisArb{cfg: BasisArbConfig{
		SpotSymbol: "ABC/USD", PerpSymbol: "ABC-PERP",
		LotSize: 1, BasePrecision: 1, MaxPosition: 10,
	}}
	arb.onFill(actor.OrderFillEvent{
		Symbol: "ABC-PERP", Qty: 2, Price: 100, Side: exchange.Sell,
		FeeAsset: "USD", FeeAmount: 3,
	})
	arb.onFill(actor.OrderFillEvent{
		Symbol: "ABC/USD", Qty: 2, Price: 99, Side: exchange.Buy,
		FeeAsset: "USD", FeeAmount: 2,
	})
	arb.onFill(actor.OrderFillEvent{
		Symbol: "ABC/USD", Qty: 1, Price: 99, Side: exchange.Sell,
		FeeAsset: "ABC", FeeAmount: 1,
	})

	report := arb.Report()
	if report.PerpSoldQty != 2 || report.SpotBoughtQty != 2 || report.SpotSoldQty != 1 {
		t.Fatalf("fill quantities = %#v", report)
	}
	if report.PerpNotional != 200 || report.SpotNotional != 297 || report.QuoteFees != 5 || report.UnpricedFeeCount != 1 {
		t.Fatalf("fill ledger = %#v", report)
	}
	if report.ResidualBaseQty != -1 || report.OpenPerpPositionLots != 2 {
		t.Fatalf("exposure report = %#v", report)
	}
}
