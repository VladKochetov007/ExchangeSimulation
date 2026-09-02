package multivenue

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/analysis"
	"exchange_sim/exchange"
	"exchange_sim/simulation"
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

type oneShotMarketSeller struct {
	*actor.BaseActor
	triggerAt int64
	quantity  int64
	submitted bool
}

func newOneShotMarketSeller(id uint64, gateway actor.Gateway, triggerAt, quantity int64) *oneShotMarketSeller {
	seller := &oneShotMarketSeller{BaseActor: actor.NewBaseActor(id, gateway), triggerAt: triggerAt, quantity: quantity}
	seller.SetHandler(seller)
	seller.AddTicker(time.Second, seller.onTick)
	return seller
}

func (s *oneShotMarketSeller) HandleEvent(context.Context, *actor.Event) {}

func (s *oneShotMarketSeller) onTick(now time.Time) {
	if !s.submitted && now.UnixNano() >= s.triggerAt {
		s.SubmitOrderWithTimeInForce("CDF/USD", exchange.Sell, exchange.Market, 0, s.quantity, exchange.IOC)
		s.submitted = true
	}
}

func TestElasticLiquiditySupplierQuotesOneInventorySensitiveSide(t *testing.T) {
	gw := newMetaGateway()
	var decisions []ElasticLiquiditySupplierDecision
	fingerprint := [16]byte{1, 2, 3}
	supplier := NewElasticLiquiditySupplier(1, gw, ElasticLiquiditySupplierConfig{
		Role: "cdf_elastic_supplier_1", ClientID: 7, Symbol: "CDF/USD",
		Interval: time.Second, MaxObservationAge: time.Minute,
		ReferencePrice: 3_000, ReferenceHalfLife: time.Hour,
		BaseHolding: 0, ElasticityPerPercent: 10, MaxPosition: 100, MaxQuoteQty: 25,
		ObservationFrontier: func() simulation.MarketDataFrontier {
			return simulation.MarketDataFrontier{LinkID: 4, Ordinal: 8, DeliveredAt: int64(time.Second), Fingerprint: fingerprint}
		},
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
	if decisions[len(decisions)-1].BestBidQty != 100 || decisions[len(decisions)-1].BestAskQty != 100 {
		t.Fatalf("decision touch depth = (%d, %d), want (100, 100)", decisions[len(decisions)-1].BestBidQty, decisions[len(decisions)-1].BestAskQty)
	}
	decision := decisions[len(decisions)-1]
	if decision.ObservationLinkID != 4 || decision.ObservationOrdinal != 8 || decision.ObservationDeliveredAt != int64(time.Second) || decision.ObservationFingerprint != "01020300000000000000000000000000" {
		t.Fatalf("decision observation frontier = %+v, want exact delayed-message identity", decision)
	}
}

func TestElasticLiquiditySupplierWithdrawsAfterMarkedLossBudget(t *testing.T) {
	gateway := newMetaGateway()
	var decisions []ElasticLiquiditySupplierDecision
	supplier := NewElasticLiquiditySupplier(1, gateway, ElasticLiquiditySupplierConfig{
		Role: "cdf_elastic_supplier_1", ClientID: 7, Symbol: "CDF/USD",
		BaseAsset: "CDF", QuoteAsset: "USD", BasePrecision: 1, QuotePrecision: 1,
		InitialBaseBalance: 100, InitialQuoteBalance: 1_000,
		Interval: time.Second, MaxObservationAge: time.Minute,
		ReferencePrice: 100, ReferenceHalfLife: time.Hour,
		BaseHolding: 0, ElasticityPerPercent: 10, MaxPosition: 100,
		MaxInventory: 200, MaxQuoteQty: 25, MaxLossQuote: 500,
		DecisionObserver: func(decision ElasticLiquiditySupplierDecision) { decisions = append(decisions, decision) },
	})
	if !supplier.equityInitialized || supplier.initialEquityQuote != 11_000 {
		t.Fatalf("initial marked equity = (%t, %d), want initialized at 11000", supplier.equityInitialized, supplier.initialEquityQuote)
	}
	supplier.onTick(time.Unix(0, int64(time.Second)))
	supplier.HandleEvent(context.Background(), elasticSupplierSnapshot("CDF/USD", int64(time.Second), 89, 91))
	// Model the supplier's filled base inventory and spent quote cash before
	// presenting an adverse delayed local mark.
	supplier.position = 10
	supplier.quoteCashAvailable = 0
	supplier.onTick(time.Unix(0, int64(2*time.Second)))

	if len(decisions) != 2 {
		t.Fatalf("decisions = %d, want subscription and risk response", len(decisions))
	}
	decision := decisions[len(decisions)-1]
	if decision.Action != "wait" || decision.Reason != "loss_limit" {
		t.Fatalf("risk decision = %+v, want fail-closed loss-limit wait", decision)
	}
	if !decision.EquityAvailable || !decision.RiskLimitTriggered || decision.MarkPrice != 90 || decision.RiskMarkPrice != 90 {
		t.Fatalf("risk state = %+v, want available triggered state at midpoint 90", decision)
	}
	if decision.EquityQuote != 9_900 || decision.LossFromInitialQuote != 1_100 || decision.DrawdownQuote != 1_100 {
		t.Fatalf("marked loss = %+v, want equity=9900 loss/drawdown=1100", decision)
	}
	encoded, err := json.Marshal(decision)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"risk_mark_price", "quote_cash_reserved", "initial_equity_quote", "equity_quote", "peak_equity_quote", "loss_from_initial_quote", "drawdown_quote", "max_loss_quote", "equity_available", "risk_limit_triggered"} {
		if _, ok := fields[field]; !ok {
			t.Fatalf("risk decision omitted required field %q: %s", field, encoded)
		}
	}
}

func TestPositiveDifferenceFailsClosedOnOverflow(t *testing.T) {
	if difference, ok := positiveDifference(math.MaxInt64, math.MinInt64); ok || difference != 0 {
		t.Fatalf("positive difference overflow = (%d, %t), want (0, false)", difference, ok)
	}
}

func TestElasticLiquiditySupplierRejectsNegativeLossBudget(t *testing.T) {
	spec := ElasticLiquiditySupplierSpec{
		Role: "cdf_elastic_supplier_1", Symbol: "CDF/USD", BaseAsset: "CDF", QuoteAsset: "USD",
		BasePrecision: 1, QuotePrecision: 1, InitialBaseBalance: 100, InitialQuoteBalance: 10_000,
		Interval: time.Second, MaxObservationAge: time.Minute, ReferencePrice: 100,
		ReferenceHalfLife: time.Hour, ElasticityPerPercent: 10, MaxPosition: 50,
		MaxInventory: 150, MaxQuoteQty: 10, MaxLossQuote: -1,
	}
	if err := spec.validate(); err == nil || !strings.Contains(err.Error(), "maximum loss budget") {
		t.Fatalf("negative loss budget validation error = %v", err)
	}
}

func TestElasticLiquiditySupplierReducesQuoteAfterInventoryFill(t *testing.T) {
	gw := newMetaGateway()
	var decisions []ElasticLiquiditySupplierDecision
	var fills []ElasticLiquiditySupplierFill
	supplier := NewElasticLiquiditySupplier(1, gw, ElasticLiquiditySupplierConfig{
		Role: "cdf_elastic_supplier_1", ClientID: 7, Symbol: "CDF/USD",
		Interval: time.Second, MaxObservationAge: time.Minute,
		ReferencePrice: 3_000, ReferenceHalfLife: time.Hour,
		BaseHolding: 0, ElasticityPerPercent: 10, MaxPosition: 100, MaxQuoteQty: 100,
		DecisionObserver: func(decision ElasticLiquiditySupplierDecision) { decisions = append(decisions, decision) },
		FillObserver:     func(fill ElasticLiquiditySupplierFill) { fills = append(fills, fill) },
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
	if len(fills) != 1 || !fills[0].IsFull {
		t.Fatalf("fill evidence = %+v, want one full fill", fills)
	}
	if len(decisions) < 2 || decisions[len(decisions)-1].TargetPosition <= decisions[len(decisions)-1].Position {
		t.Fatalf("post-fill decision = %+v, want remaining inventory gap", decisions[len(decisions)-1])
	}
}

func TestElasticLiquiditySupplierTracksPartialFillRemainingQuantity(t *testing.T) {
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
	order := gw.orders()[0]
	supplier.HandleEvent(ctx, &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{OrderID: 41, RequestID: order.RequestID}})
	supplier.HandleEvent(ctx, &actor.Event{Type: actor.EventOrderPartialFill, Data: actor.OrderFillEvent{
		OrderID: 41, Symbol: "CDF/USD", Side: exchange.Buy, Qty: 4, Price: 2_699, IsFull: false,
	}})
	if supplier.quote.qty != order.Qty-4 {
		t.Fatalf("remaining quote quantity = %d, want %d", supplier.quote.qty, order.Qty-4)
	}
	supplier.HandleEvent(ctx, elasticSupplierSnapshot("CDF/USD", int64(3*time.Second), 2_699, 2_701))
	supplier.onTick(time.Unix(0, int64(4*time.Second)))
	if len(gw.orders()) != 1 {
		t.Fatalf("orders after partial fill = %+v, want no replacement while the original order remains live", gw.orders())
	}
	if len(decisions) < 2 || decisions[len(decisions)-1].Action != "rest" || decisions[len(decisions)-1].QuoteOrderID != 41 || decisions[len(decisions)-1].QuoteQty != order.Qty-4 {
		t.Fatalf("post-partial decision = %+v, want rest at exchange remaining quantity", decisions[len(decisions)-1])
	}
}

func TestElasticLiquiditySupplierAdmissionHeadroomBoundsBothSides(t *testing.T) {
	supplier := NewElasticLiquiditySupplier(1, newMetaGateway(), ElasticLiquiditySupplierConfig{
		InitialBaseBalance: 500, MaxPosition: 500, MaxInventory: 1_000,
		ReferencePrice: 3_000, ElasticityPerPercent: 100,
	})
	if got := supplier.TargetPosition(1); got != 500 {
		t.Fatalf("low-price target = %d, want gross inventory cap target 500", got)
	}
	if got := supplier.TargetPosition(10_000); got != -500 {
		t.Fatalf("high-price target = %d, want sell-side displacement bound -500", got)
	}
	supplier.position = 500
	if got := supplier.availableBuyInventory(); got != 0 {
		t.Fatalf("buy headroom at gross cap = %d, want zero", got)
	}
	if got := supplier.availableSellInventory(); got != 1_000 {
		t.Fatalf("sell headroom at gross cap = %d, want 1000", got)
	}
	supplier.position = -500
	if got := supplier.availableBuyInventory(); got != 1_000 {
		t.Fatalf("buy headroom after full sale = %d, want 1000", got)
	}
	if got := supplier.availableSellInventory(); got != 0 {
		t.Fatalf("sell headroom after full sale = %d, want zero", got)
	}
}

func TestElasticLiquiditySupplierBoundsBuyQuoteByFiniteCash(t *testing.T) {
	gw := newMetaGateway()
	var decisions []ElasticLiquiditySupplierDecision
	supplier := NewElasticLiquiditySupplier(1, gw, ElasticLiquiditySupplierConfig{
		Role: "cdf_elastic_supplier_1", ClientID: 7, Symbol: "CDF/USD",
		BasePrecision: 1, QuotePrecision: 1, InitialQuoteBalance: 21,
		Interval: time.Second, MaxObservationAge: time.Minute,
		ReferencePrice: 100, ReferenceHalfLife: time.Hour,
		BaseHolding: 0, ElasticityPerPercent: 10, MaxPosition: 100, MaxQuoteQty: 100,
		MakerFeeBps:      1_000,
		DecisionObserver: func(decision ElasticLiquiditySupplierDecision) { decisions = append(decisions, decision) },
	})
	supplier.onTick(time.Unix(0, int64(time.Second)))
	supplier.HandleEvent(context.Background(), elasticSupplierSnapshot("CDF/USD", int64(time.Second), 10, 12))
	supplier.onTick(time.Unix(0, int64(2*time.Second)))
	orders := gw.orders()
	if len(orders) != 1 || orders[0].Qty != 1 {
		t.Fatalf("cash-bounded quote = %+v, want one unit because 2 units require 22 quote cash", orders)
	}
	if len(decisions) == 0 || decisions[len(decisions)-1].QuoteCashAvailable != 21 || decisions[len(decisions)-1].QuoteCashRequired != 11 {
		t.Fatalf("cash admission evidence = %+v, want available 21 and required 11", decisions)
	}
}

func TestElasticLiquiditySupplierDoesNotSubmitBelowMinimumExecutableQty(t *testing.T) {
	gateway := newMetaGateway()
	var decisions []ElasticLiquiditySupplierDecision
	supplier := NewElasticLiquiditySupplier(1, gateway, ElasticLiquiditySupplierConfig{
		Role: "cdf_elastic_supplier_1", ClientID: 7, Symbol: "CDF/USD",
		Interval: time.Second, MaxObservationAge: time.Minute,
		ReferencePrice: 100, ReferenceHalfLife: time.Hour,
		BaseHolding: 0, ElasticityPerPercent: 5, MaxPosition: 100, MaxQuoteQty: 100,
		MinimumExecutableQty: 10,
		DecisionObserver:     func(decision ElasticLiquiditySupplierDecision) { decisions = append(decisions, decision) },
	})
	supplier.onTick(time.Unix(0, int64(time.Second)))
	supplier.HandleEvent(context.Background(), elasticSupplierSnapshot("CDF/USD", int64(time.Second), 98, 100))
	supplier.onTick(time.Unix(0, int64(2*time.Second)))
	if len(gateway.orders()) != 0 {
		t.Fatalf("sub-minimum orders = %+v, want no submission", gateway.orders())
	}
	if len(decisions) == 0 || decisions[len(decisions)-1].Action != "wait" || decisions[len(decisions)-1].Reason != "below_minimum_executable_qty" {
		t.Fatalf("sub-minimum decision = %+v, want explicit executable floor", decisions)
	}

	supplier.cfg.ElasticityPerPercent = 10
	supplier.onTick(time.Unix(0, int64(2*time.Second)))
	orders := gateway.orders()
	if len(orders) != 1 || orders[0].Qty < supplier.cfg.MinimumExecutableQty {
		t.Fatalf("minimum-sized order = %+v, want one executable quote", orders)
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

func TestElasticLiquiditySupplierWithdrawsStaleQuoteAndWaitsForCancel(t *testing.T) {
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
	orders := gw.orders()
	if len(orders) != 1 {
		t.Fatalf("orders before stale withdrawal = %d, want one", len(orders))
	}
	supplier.HandleEvent(ctx, &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{OrderID: 41, RequestID: orders[0].RequestID}})
	supplier.onTick(time.Unix(0, int64(62*time.Second)))
	if len(gw.requests) != 3 || gw.requests[2].Type != etypes.ReqCancelOrder || gw.requests[2].CancelReq.OrderID != 41 || gw.requests[2].CancelReq.RequestID == 0 {
		t.Fatalf("requests after stale withdrawal = %+v, want one cancellation", gw.requests)
	}
	supplier.onTick(time.Unix(0, int64(63*time.Second)))
	if len(gw.requests) != 3 {
		t.Fatalf("requests while cancellation is delayed = %+v, want no replacement", gw.requests)
	}
}

func TestElasticLiquiditySupplierRecoversFromFillCancelRace(t *testing.T) {
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
	orders := gw.orders()
	if len(orders) != 1 {
		t.Fatalf("initial orders = %d, want one", len(orders))
	}
	order := orders[0]
	supplier.HandleEvent(ctx, &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{OrderID: 41, RequestID: order.RequestID}})
	supplier.HandleEvent(ctx, &actor.Event{Type: actor.EventBookSnapshot, Data: actor.BookSnapshotEvent{
		Symbol: "CDF/USD", Timestamp: int64(3 * time.Second), Snapshot: &etypes.BookSnapshot{
			Bids: []etypes.PriceLevel{{Price: 2_699, VisibleQty: 100}},
		},
	}})
	supplier.onTick(time.Unix(0, int64(4*time.Second)))
	if len(gw.requests) != 3 || gw.requests[2].Type != etypes.ReqCancelOrder {
		t.Fatalf("requests before concurrent fill = %+v, want cancellation", gw.requests)
	}
	supplier.HandleEvent(ctx, &actor.Event{Type: actor.EventOrderFilled, Data: actor.OrderFillEvent{
		OrderID: 41, Symbol: "CDF/USD", Side: exchange.Buy, Qty: 25, Price: 2_699, IsFull: true,
	}})
	supplier.HandleEvent(ctx, &actor.Event{Type: actor.EventOrderCancelRejected, Data: actor.OrderCancelRejectedEvent{OrderID: 41}})
	supplier.HandleEvent(ctx, elasticSupplierSnapshot("CDF/USD", int64(5*time.Second), 2_699, 2_701))
	supplier.onTick(time.Unix(0, int64(6*time.Second)))
	if len(gw.orders()) != 2 {
		t.Fatalf("orders after concurrent fill = %+v, want replacement quote", gw.orders())
	}
	if supplier.cancelPending || supplier.cancelRequestID != 0 {
		t.Fatalf("supplier remained cancel-pending after full fill: pending=%t request=%d", supplier.cancelPending, supplier.cancelRequestID)
	}
}

func testElasticLiquiditySupplierSpec() ElasticLiquiditySupplierSpec {
	return ElasticLiquiditySupplierSpec{
		Role: "cdf_elastic_supplier_1", Symbol: "CDF/USD", BaseAsset: "CDF", QuoteAsset: "USD",
		BasePrecision: mvBasePrecision, QuotePrecision: mvQuotePrecision,
		InitialBaseBalance: 500 * mvBasePrecision, InitialQuoteBalance: 500_000_000 * mvQuotePrecision,
		Interval: time.Second, MaxObservationAge: time.Minute,
		ReferencePrice: mvCDFBootstrap, ReferenceHalfLife: 4 * time.Hour,
		BaseHolding: 0, ElasticityPerPercent: 15 * mvBasePrecision,
		MaxPosition: 500 * mvBasePrecision, MaxInventory: 1_000 * mvBasePrecision, MaxQuoteQty: mvBasePrecision / 2, MakerFeeBps: 5,
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

func TestElasticLiquiditySupplierSpecAcceptsConfiguredAssetPair(t *testing.T) {
	spec := testElasticLiquiditySupplierSpec()
	spec.Role = "alt_liquidity_provider_1"
	spec.Symbol = "ALT/EUR"
	spec.BaseAsset = "ALT"
	spec.QuoteAsset = "EUR"
	if err := spec.validate(); err != nil {
		t.Fatalf("generic configured asset pair rejected: %v", err)
	}
}

func TestElasticLiquiditySupplierSpecRejectsInvalidMakerFee(t *testing.T) {
	spec := testElasticLiquiditySupplierSpec()
	for _, fee := range []int64{-1, 10_001} {
		spec.MakerFeeBps = fee
		if err := spec.validate(); err == nil {
			t.Fatalf("maker fee %d was accepted", fee)
		}
	}
}

func TestElasticLiquiditySupplierSpecRejectsInvalidDecisionPhase(t *testing.T) {
	spec := testElasticLiquiditySupplierSpec()
	for _, phase := range []time.Duration{-time.Nanosecond, time.Second} {
		spec.DecisionPhaseOffset = phase
		if err := spec.validate(); err == nil {
			t.Fatalf("decision phase %s was accepted", phase)
		}
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

func TestElasticLiquiditySupplierRealGatewayStaleWithdrawalIntegration(t *testing.T) {
	spec := testElasticLiquiditySupplierSpec()
	spec.MaxObservationAge = 1500 * time.Millisecond
	spec.MaxQuoteQty = 2 * mvBasePrecision
	spec.BaseHolding = 100 * mvBasePrecision
	dir := t.TempDir()
	cfg := Config{
		LogDir: dir, LogMode: "full", Seed: 101, StrictPopulationAccounting: true, CrossAssetSpotGraph: true,
		SnapshotInterval:         2 * time.Second,
		RecordMarketDataReceipts: true, MarketDataReceiptRoles: []string{"cdf_elastic_supplier"},
		RecordElasticLiquiditySupplierDecisions: true,
		ElasticLiquiditySuppliers:               []ElasticLiquiditySupplierSpec{spec},
		LatencyProfiles: map[string]LatencyProfile{
			"cdf_elastic_supplier": {Model: "constant", Delay: 500 * time.Millisecond},
		},
	}
	sim, err := NewSim(70*time.Second, cfg)
	if err != nil {
		t.Fatal(err)
	}
	venue := sim.Venues[0]
	timerFactory := simulation.NewSimTimerFactory(venue.scheduler)
	sim.Runner.AddIdler(timerFactory)
	clientID, gateway := venue.connectParticipant(venue.Mount, "stale_probe_taker_1", map[string]int64{
		"CDF": 10 * mvBasePrecision, "USD": 1_000 * mvQuotePrecision,
	}, 0, &exchange.PercentageFee{TakerBps: 0, InQuote: true})
	seller := newOneShotMarketSeller(100_000, gateway, venue.clock.NowUnixNano()+11*int64(time.Second), 2*mvBasePrecision)
	seller.SetTickerFactory(timerFactory)
	sim.Runner.AddActor(seller)
	if sim.InitialAccounts, err = sim.capturePopulationAccounts("initial"); err != nil {
		t.Fatal(err)
	}
	if err := sim.Run(context.Background()); err != nil {
		_ = sim.Close()
		t.Fatal(err)
	}
	if err := sim.Close(); err != nil {
		t.Fatal(err)
	}
	general, err := os.ReadFile(filepath.Join(dir, "venues", "north", "general.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	generalText := string(general)
	book, err := os.ReadFile(filepath.Join(dir, "venues", "north", "spot", "CDF-USD.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	bookText := string(book)
	accepted := strings.Count(bookText, `"event":"OrderAccepted"`)
	cancelled := strings.Count(bookText, `"event":"OrderCancelled"`)
	withdrawn := strings.Count(generalText, `"action":"withdraw"`)
	if accepted == 0 || cancelled == 0 || withdrawn == 0 {
		preview := generalText
		if len(preview) > 4000 {
			preview = preview[:4000]
		}
		matched := make([]string, 0, 12)
		for _, line := range strings.Split(generalText, "\n") {
			if strings.Contains(line, "elastic_liquidity_supplier") || strings.Contains(line, "order_accepted") || strings.Contains(line, "OrderAccepted") {
				matched = append(matched, line)
				if len(matched) == 12 {
					break
				}
			}
		}
		t.Fatalf("accepted=%d cancelled=%d withdrawn=%d cdf_matches=%d general_prefix=%s matches=%s", accepted, cancelled, withdrawn, strings.Count(generalText, "elastic_liquidity_supplier"), preview, strings.Join(matched, "\n"))
	}
	t.Logf("stale integration seller client=%d submitted=%t accepted=%d cancelled=%d withdrawn=%d", clientID, seller.submitted, accepted, cancelled, withdrawn)
	greeks, err := json.Marshal(struct {
		InitialAccounts  []ParticipantAccountSnapshot `json:"initial_accounts"`
		TerminalAccounts []ParticipantAccountSnapshot `json:"terminal_accounts"`
	}{InitialAccounts: sim.InitialAccounts, TerminalAccounts: sim.TerminalAccounts})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "greeks.json"), greeks, 0644); err != nil {
		t.Fatal(err)
	}
	auditRun, err := analysis.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	audit, err := auditRun.MeasureCDFLiquidity()
	if err != nil {
		t.Fatal(err)
	}
	receiptAudit, err := analysis.AuditMarketDataReceipts(dir)
	if err != nil || !receiptAudit.Valid || receiptAudit.Decisions == 0 {
		t.Fatalf("real delayed-gateway receipt audit = %+v, err=%v", receiptAudit, err)
	}
	if audit.SupplierCount != 3 || audit.AcceptedQuoteCount == 0 || audit.CompletedQuoteCount == 0 || audit.WithdrawCount == 0 {
		t.Fatalf("real delayed-gateway stale lifecycle = %+v", audit)
	}
	for _, check := range audit.Checks {
		if strings.HasPrefix(check.Failure, "stale withdrawal") {
			t.Fatalf("real gateway stale lifecycle was not reconstructed: %+v", audit.Checks)
		}
		for _, invalidObservationFailure := range []string{
			"invalid decision bounds or observation time",
			"supplier decision has an incomplete observation frontier",
			"decision observation age exceeds registered delayed-data bound",
		} {
			if strings.HasPrefix(check.Failure, invalidObservationFailure) {
				t.Fatalf("valid fail-closed wait was rejected by the observation contract: %+v", check)
			}
		}
	}
	mutatedBook, removed := removeOneStaleCancellation(general, book)
	if !removed {
		t.Fatal("real gateway fixture has no cancellation matching a stale withdrawal")
	}
	if err := os.WriteFile(filepath.Join(dir, "venues", "north", "spot", "CDF-USD.jsonl"), mutatedBook, 0644); err != nil {
		t.Fatal(err)
	}
	mutatedAudit, err := auditRun.MeasureCDFLiquidity()
	if err != nil {
		t.Fatal(err)
	}
	if !hasCDFFailurePrefix(mutatedAudit.Checks, "stale withdrawal has no later matching exchange cancellation outcome") {
		t.Fatalf("missing exchange cancellation was not rejected: %+v", mutatedAudit.Checks)
	}
	if err := os.WriteFile(filepath.Join(dir, "venues", "north", "spot", "CDF-USD.jsonl"), book, 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("real gateway stale lifecycle: suppliers=%d accepted=%d completed=%d withdrawals=%d fills=%d receipt_decisions=%d mutation_rejected=true", audit.SupplierCount, audit.AcceptedQuoteCount, audit.CompletedQuoteCount, audit.WithdrawCount, audit.FillCount, receiptAudit.Decisions)
}

type cdfLogEnvelope struct {
	ClientID uint64 `json:"client_id"`
	Event    string `json:"event"`
	Data     struct {
		Payload json.RawMessage `json:"payload"`
	} `json:"data"`
}

type staleCancellationReference struct {
	ClientID        uint64
	OrderID         uint64
	CancelRequestID uint64
}

func removeOneStaleCancellation(general, book []byte) ([]byte, bool) {
	staleOrders := make(map[staleCancellationReference]struct{})
	for _, line := range strings.Split(string(general), "\n") {
		var envelope cdfLogEnvelope
		if err := json.Unmarshal([]byte(line), &envelope); err != nil || envelope.Event != "elastic_liquidity_supplier_decision" {
			continue
		}
		var decision struct {
			Action          string `json:"action"`
			Reason          string `json:"reason"`
			QuoteOrderID    uint64 `json:"quote_order_id"`
			CancelRequestID uint64 `json:"cancel_request_id"`
		}
		if err := json.Unmarshal(envelope.Data.Payload, &decision); err != nil || decision.Action != "withdraw" || decision.Reason != "stale_or_missing_observation" || decision.QuoteOrderID == 0 || decision.CancelRequestID == 0 {
			continue
		}
		staleOrders[staleCancellationReference{ClientID: envelope.ClientID, OrderID: decision.QuoteOrderID, CancelRequestID: decision.CancelRequestID}] = struct{}{}
	}
	lines := strings.SplitAfter(string(book), "\n")
	for index, line := range lines {
		var envelope cdfLogEnvelope
		if err := json.Unmarshal([]byte(line), &envelope); err != nil || envelope.Event != "OrderCancelled" {
			continue
		}
		var cancellation struct {
			OrderID   uint64 `json:"order_id"`
			RequestID uint64 `json:"request_id"`
		}
		if err := json.Unmarshal(envelope.Data.Payload, &cancellation); err != nil {
			continue
		}
		reference := staleCancellationReference{ClientID: envelope.ClientID, OrderID: cancellation.OrderID, CancelRequestID: cancellation.RequestID}
		if _, exists := staleOrders[reference]; !exists {
			continue
		}
		return []byte(strings.Join(append(lines[:index], lines[index+1:]...), "")), true
	}
	return book, false
}

func hasCDFFailurePrefix(checks []analysis.CDFLiquidityCheck, prefix string) bool {
	for _, check := range checks {
		if strings.HasPrefix(check.Failure, prefix) {
			return true
		}
	}
	return false
}
