package multivenue

import (
	"context"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	etypes "exchange_sim/types"
)

func elasticSupplierSnapshot(symbol string, timestamp, bid, ask int64) *actor.Event {
	return &actor.Event{
		Type: actor.EventBookSnapshot,
		Data: actor.BookSnapshotEvent{
			Symbol:    symbol,
			Timestamp: timestamp,
			Snapshot: &etypes.BookSnapshot{
				Bids: []etypes.PriceLevel{{Price: bid, VisibleQty: 100}},
				Asks: []etypes.PriceLevel{{Price: ask, VisibleQty: 100}},
			},
		},
	}
}

func TestElasticLiquiditySupplierQuotesOneInventorySensitiveSide(t *testing.T) {
	gw := newMetaGateway()
	var decisions []ElasticLiquiditySupplierDecision
	supplier := NewElasticLiquiditySupplier(1, gw, ElasticLiquiditySupplierConfig{
		Role: "cdf_elastic_supplier_1", ClientID: 7, Symbol: "CDF/USD",
		Interval: time.Second, MaxObservationAge: time.Minute,
		ReferencePrice: 3_000, ReferenceHalfLife: time.Hour,
		BaseHolding: 0, ElasticityPerPercent: 10, MaxPosition: 100, MaxQuoteQty: 25,
		DecisionObserver: func(decision ElasticLiquiditySupplierDecision) { decisions = append(decisions, decision) },
	})
	now := int64(time.Second)
	supplier.onTick(time.Unix(0, now))
	supplier.HandleEvent(context.Background(), elasticSupplierSnapshot("CDF/USD", now, 2_699, 2_701))
	supplier.onTick(time.Unix(0, 2*now))
	orders := gw.orders()
	if len(orders) != 1 {
		t.Fatalf("orders = %d, want one one-sided quote", len(orders))
	}
	order := orders[0]
	if order.Side != exchange.Buy || order.Price != 2_699 || order.Qty != 25 || !order.PostOnly || order.TimeInForce != exchange.GTC {
		t.Fatalf("quote = %+v, want passive buy at local bid with bounded quantity", order)
	}
	if len(decisions) == 0 || decisions[len(decisions)-1].Action != "submit" {
		t.Fatalf("decision evidence = %+v, want submit", decisions)
	}
	if decisions[len(decisions)-1].Side != "BUY" || decisions[len(decisions)-1].QuoteQty != 25 {
		t.Fatalf("decision = %+v, want inventory-sensitive buy", decisions[len(decisions)-1])
	}
}

func TestElasticLiquiditySupplierReducesQuoteAfterInventoryFill(t *testing.T) {
	gw := newMetaGateway()
	var decisions []ElasticLiquiditySupplierDecision
	supplier := NewElasticLiquiditySupplier(1, gw, ElasticLiquiditySupplierConfig{
		Role: "cdf_elastic_supplier_1", ClientID: 7, Symbol: "CDF/USD",
		Interval: time.Second, MaxObservationAge: time.Minute,
		ReferencePrice: 3_000, ReferenceHalfLife: time.Hour,
		BaseHolding: 0, ElasticityPerPercent: 10, MaxPosition: 100, MaxQuoteQty: 100,
		DecisionObserver: func(decision ElasticLiquiditySupplierDecision) { decisions = append(decisions, decision) },
	})
	ctx := context.Background()
	supplier.onTick(time.Unix(0, int64(time.Second)))
	supplier.HandleEvent(ctx, elasticSupplierSnapshot("CDF/USD", int64(time.Second), 2_699, 2_701))
	supplier.onTick(time.Unix(0, int64(2*time.Second)))
	request := gw.orders()[0].RequestID
	supplier.HandleEvent(ctx, &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{OrderID: 41, RequestID: request}})
	supplier.HandleEvent(ctx, &actor.Event{Type: actor.EventOrderFilled, Data: actor.OrderFillEvent{
		OrderID: 41, Symbol: "CDF/USD", Side: exchange.Buy, Qty: 25, Price: 2_699, IsFull: true,
	}})
	supplier.HandleEvent(ctx, elasticSupplierSnapshot("CDF/USD", int64(3*time.Second), 2_699, 2_701))
	supplier.onTick(time.Unix(0, int64(4*time.Second)))
	orders := gw.orders()
	if len(orders) != 2 || orders[1].Qty >= orders[0].Qty {
		t.Fatalf("quotes after fill = %+v, want reduced replacement quantity", orders)
	}
	if supplier.Position() != 25 {
		t.Fatalf("position = %d, want 25", supplier.Position())
	}
	if len(decisions) < 2 || decisions[len(decisions)-1].TargetPosition <= decisions[len(decisions)-1].Position {
		t.Fatalf("post-fill decision = %+v, want remaining inventory gap", decisions[len(decisions)-1])
	}
}

func TestElasticLiquiditySupplierWithdrawsOnUnavailableLocalSide(t *testing.T) {
	gw := newMetaGateway()
	supplier := NewElasticLiquiditySupplier(1, gw, ElasticLiquiditySupplierConfig{
		Role: "cdf_elastic_supplier_1", ClientID: 7, Symbol: "CDF/USD",
		Interval: time.Second, MaxObservationAge: time.Minute,
		ReferencePrice: 3_000, ReferenceHalfLife: time.Hour,
		BaseHolding: 0, ElasticityPerPercent: 10, MaxPosition: 100, MaxQuoteQty: 25,
	})
	ctx := context.Background()
	supplier.onTick(time.Unix(0, int64(time.Second)))
	supplier.HandleEvent(ctx, elasticSupplierSnapshot("CDF/USD", int64(time.Second), 2_699, 2_701))
	supplier.onTick(time.Unix(0, int64(2*time.Second)))
	request := gw.orders()[0].RequestID
	supplier.HandleEvent(ctx, &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{OrderID: 41, RequestID: request}})
	supplier.HandleEvent(ctx, &actor.Event{Type: actor.EventBookSnapshot, Data: actor.BookSnapshotEvent{
		Symbol: "CDF/USD", Timestamp: int64(3 * time.Second), Snapshot: &etypes.BookSnapshot{
			Bids: []etypes.PriceLevel{{Price: 2_699, VisibleQty: 100}},
		},
	}})
	supplier.onTick(time.Unix(0, int64(4*time.Second)))
	if len(gw.requests) != 3 || gw.requests[2].Type != etypes.ReqCancelOrder || gw.requests[2].CancelReq.OrderID != 41 {
		t.Fatalf("requests after one-sided snapshot = %+v, want withdrawal only", gw.requests)
	}
}

func testElasticLiquiditySupplierSpec() ElasticLiquiditySupplierSpec {
	return ElasticLiquiditySupplierSpec{
		Role: "cdf_elastic_supplier_1", Symbol: "CDF/USD",
		BasePrecision:      mvBasePrecision,
		InitialBaseBalance: 1_000 * mvBasePrecision, InitialQuoteBalance: 500_000_000 * mvQuotePrecision,
		Interval: time.Second, MaxObservationAge: time.Minute,
		ReferencePrice: mvCDFBootstrap, ReferenceHalfLife: 4 * time.Hour,
		BaseHolding: 0, ElasticityPerPercent: 15 * mvBasePrecision,
		MaxPosition: 1_000 * mvBasePrecision, MaxQuoteQty: mvBasePrecision / 2,
	}
}

func TestElasticLiquiditySupplierRosterRequiresDelayedLocalLink(t *testing.T) {
	cfg := Config{
		LogDir: t.TempDir(), CrossAssetSpotGraph: true,
		ElasticLiquiditySuppliers: []ElasticLiquiditySupplierSpec{testElasticLiquiditySupplierSpec()},
	}
	if err := cfg.normalize(); err == nil {
		t.Fatal("roster without an explicit delayed local link was accepted")
	}
}

func TestElasticLiquiditySupplierRosterWiresSeparatelyFromHistoricalSuppliers(t *testing.T) {
	cfg := Config{
		LogDir: t.TempDir(), CrossAssetSpotGraph: true,
		ElasticSupplierCount:      8,
		ElasticLiquiditySuppliers: []ElasticLiquiditySupplierSpec{testElasticLiquiditySupplierSpec()},
		LatencyProfiles: map[string]LatencyProfile{
			"cdf_elastic_supplier": {Model: "constant", Delay: time.Millisecond},
		},
	}
	sim, err := NewSim(time.Second, cfg)
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	defer sim.Close()
	for _, venue := range sim.Venues {
		if len(venue.Suppliers) != 8 || len(venue.ElasticLiquiditySuppliers) != 1 {
			t.Fatalf("supplier rosters = (%d historical, %d successor), want (8, 1)", len(venue.Suppliers), len(venue.ElasticLiquiditySuppliers))
		}
	}
}
