package multivenue

import (
	"context"
	"encoding/hex"
	"math"
	"strings"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/simulation"
	etypes "exchange_sim/types"
)

func cdfSupplierBookEvent(symbol string, timestamp, sequence, bid, ask, bidQty, askQty int64) *actor.Event {
	snapshot := &etypes.BookSnapshot{}
	if bid != 0 {
		snapshot.Bids = []etypes.PriceLevel{{Price: bid, VisibleQty: bidQty}}
	}
	if ask != 0 {
		snapshot.Asks = []etypes.PriceLevel{{Price: ask, VisibleQty: askQty}}
	}
	return &actor.Event{
		Type: actor.EventBookSnapshot,
		Data: actor.BookSnapshotEvent{
			Symbol: symbol, Timestamp: timestamp, SeqNum: uint64(sequence), Snapshot: snapshot,
		},
	}
}

func cdfSupplierUnitConfig() ElasticLiquiditySupplierConfig {
	return ElasticLiquiditySupplierConfig{
		Role:                           "cdf_elastic_supplier_1",
		ClientID:                       7,
		Symbol:                         "CDF/USD",
		BaseAsset:                      "CDF",
		QuoteAsset:                     "USD",
		BasePrecision:                  1,
		InitialBaseBalance:             100,
		QuotePrecision:                 1,
		InitialQuoteBalance:            100_000,
		Interval:                       time.Second,
		MaxObservationAge:              time.Minute,
		ReferencePrice:                 1_000,
		ReferenceHalfLife:              time.Hour,
		BaseHolding:                    0,
		ElasticityPerPercent:           10,
		MaxPosition:                    100,
		MaxInventory:                   200,
		MaxQuoteQty:                    50,
		MinimumExecutableQty:           1,
		MinimumQualifyingQty:           10,
		RegisteredMinimumExecutableQty: 1,
		TickSize:                       100,
		QuoteOnOneSidedLocalBook:       true,
		MaxLossQuote:                   25_000,
		MakerFeeBps:                    100,
	}
}

func TestCDFSupplierDisabledOneSidedBookWaits(t *testing.T) {
	gateway := newMetaGateway()
	cfg := cdfSupplierUnitConfig()
	cfg.QuoteOnOneSidedLocalBook = false
	supplier := NewElasticLiquiditySupplier(1, gateway, cfg)
	supplier.onTick(time.Unix(0, int64(time.Second)))
	supplier.HandleEvent(context.Background(), cdfSupplierBookEvent(cfg.Symbol, int64(time.Second), 11, 1_200, 0, 100, 0))
	supplier.onTick(time.Unix(0, int64(2*time.Second)))
	if got := len(gateway.orders()); got != 0 {
		t.Fatalf("mode-off one-sided orders = %d, want zero", got)
	}
}

func TestCDFSupplierDecisionCarriesDeliveredObservationFingerprint(t *testing.T) {
	gateway := newMetaGateway()
	fingerprint := [16]byte{1, 2, 3, 4}
	cfg := cdfSupplierUnitConfig()
	cfg.ObservationFrontier = func() simulation.MarketDataFrontier {
		return simulation.MarketDataFrontier{LinkID: 3, Ordinal: 7, DeliveredAt: 1_000, Fingerprint: fingerprint}
	}
	supplier := NewElasticLiquiditySupplier(1, gateway, cfg)
	decision := supplier.baseDecision(2_000)
	if decision.ObservationFingerprint != hex.EncodeToString(fingerprint[:]) {
		t.Fatalf("decision fingerprint = %q, want the delivered message identity", decision.ObservationFingerprint)
	}
}

func TestCDFSupplierMissingSnapshotClearsStaleLocalState(t *testing.T) {
	gateway := newMetaGateway()
	cfg := cdfSupplierUnitConfig()
	supplier := NewElasticLiquiditySupplier(1, gateway, cfg)
	supplier.HandleEvent(context.Background(), cdfSupplierBookEvent(cfg.Symbol, int64(time.Second), 11, 900, 1_100, 100, 100))
	if supplier.bestBid != 900 || supplier.bestAsk != 1_100 || supplier.observationSequence != 11 {
		t.Fatalf("initial local state = bid %d ask %d sequence %d", supplier.bestBid, supplier.bestAsk, supplier.observationSequence)
	}
	supplier.HandleEvent(context.Background(), &actor.Event{
		Type: actor.EventBookSnapshot,
		Data: actor.BookSnapshotEvent{Symbol: cfg.Symbol, Timestamp: int64(2 * time.Second), SeqNum: 12},
	})
	if supplier.bestBid != 0 || supplier.bestBidQty != 0 || supplier.bestAsk != 0 || supplier.bestAskQty != 0 ||
		supplier.riskMarkPrice != 0 || supplier.observationTime != int64(2*time.Second) || supplier.observationSequence != 12 {
		t.Fatalf("missing snapshot retained local state: bid=%d/%d ask=%d/%d mark=%d time=%d sequence=%d",
			supplier.bestBid, supplier.bestBidQty, supplier.bestAsk, supplier.bestAskQty,
			supplier.riskMarkPrice, supplier.observationTime, supplier.observationSequence)
	}
	if observation := supplier.observeLocalBook(); observation.ok {
		t.Fatalf("missing snapshot produced usable local book: %+v", observation)
	}
}

func TestCDFSupplierValidSnapshotPreservesRiskMarkUntilRiskUpdate(t *testing.T) {
	gateway := newMetaGateway()
	cfg := cdfSupplierUnitConfig()
	decisions := make([]ElasticLiquiditySupplierDecision, 0, 1)
	cfg.DecisionObserver = func(decision ElasticLiquiditySupplierDecision) {
		decisions = append(decisions, decision)
	}
	supplier := NewElasticLiquiditySupplier(1, gateway, cfg)
	supplier.riskMarkPrice = 777
	supplier.pendingRequestID = 42
	supplier.subscribed = true
	supplier.HandleEvent(context.Background(), cdfSupplierBookEvent(cfg.Symbol, int64(time.Second), 11, 900, 1_100, 100, 100))
	supplier.onTick(time.Unix(0, int64(2*time.Second)))
	if supplier.riskMarkPrice != 777 {
		t.Fatalf("valid snapshot cleared risk mark before risk update: got %d, want 777", supplier.riskMarkPrice)
	}
	if len(decisions) != 1 || decisions[0].Action != "wait" || decisions[0].Reason != "order_pending" || decisions[0].RiskMarkPrice != 777 {
		t.Fatalf("valid snapshot early-return decision = %+v, want pending with risk mark 777", decisions)
	}
}

func TestCDFSupplierMissingSnapshotWithdrawsAndLaterValidSnapshotCanRecover(t *testing.T) {
	gateway := newMetaGateway()
	cfg := cdfSupplierUnitConfig()
	supplier := NewElasticLiquiditySupplier(1, gateway, cfg)
	supplier.onTick(time.Unix(0, int64(time.Second)))
	supplier.HandleEvent(context.Background(), cdfSupplierBookEvent(cfg.Symbol, int64(time.Second), 11, 1_200, 1_300, 100, 100))
	supplier.onTick(time.Unix(0, int64(2*time.Second)))
	orders := gateway.orders()
	if len(orders) != 1 {
		t.Fatalf("initial orders = %+v, want one", orders)
	}
	supplier.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{OrderID: 42, RequestID: orders[0].RequestID}})
	supplier.HandleEvent(context.Background(), &actor.Event{
		Type: actor.EventBookSnapshot,
		Data: actor.BookSnapshotEvent{Symbol: cfg.Symbol, Timestamp: int64(3 * time.Second), SeqNum: 12},
	})
	supplier.onTick(time.Unix(0, int64(4*time.Second)))
	if len(gateway.requests) != 3 || gateway.requests[2].Type != etypes.ReqCancelOrder || gateway.requests[2].CancelReq.OrderID != 42 {
		t.Fatalf("missing-snapshot requests = %+v, want one withdrawal cancel", gateway.requests)
	}
	supplier.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderCancelled, Data: actor.OrderCancelledEvent{OrderID: 42, RequestID: gateway.requests[2].CancelReq.RequestID}})
	supplier.HandleEvent(context.Background(), cdfSupplierBookEvent(cfg.Symbol, int64(5*time.Second), 13, 1_000, 1_100, 100, 100))
	supplier.onTick(time.Unix(0, int64(6*time.Second)))
	if len(gateway.orders()) != 2 {
		t.Fatalf("orders after valid recovery = %+v, want a new quote after withdrawal", gateway.orders())
	}
}

func TestCDFSupplierQuotesMissingAskOnRegisteredGrid(t *testing.T) {
	gateway := newMetaGateway()
	decisions := make([]ElasticLiquiditySupplierDecision, 0)
	cfg := cdfSupplierUnitConfig()
	cfg.DecisionObserver = func(decision ElasticLiquiditySupplierDecision) {
		decisions = append(decisions, decision)
	}
	supplier := NewElasticLiquiditySupplier(1, gateway, cfg)
	supplier.onTick(time.Unix(0, int64(time.Second)))
	supplier.HandleEvent(context.Background(), cdfSupplierBookEvent(cfg.Symbol, int64(time.Second), 11, 1_200, 0, 100, 0))
	supplier.onTick(time.Unix(0, int64(2*time.Second)))
	orders := gateway.orders()
	if len(orders) != 1 {
		t.Fatalf("orders = %d, want one missing-ask quote", len(orders))
	}
	order := orders[0]
	if order.Side != exchange.Sell || order.Price != 1_300 || order.Qty != 50 || !order.PostOnly || order.TimeInForce != exchange.GTC {
		t.Fatalf("missing-ask order = %+v, want bounded sell of 50 at 1300", order)
	}
	decision := decisions[len(decisions)-1]
	if decision.LocalBookMode != "one_sided" || decision.QuotePriceSource != "one_sided_missing_side_blended" ||
		decision.RiskMarkSource != "one_sided_bid" || decision.RiskMarkPrice != 1_200 || decision.ObservationSequence != 11 {
		t.Fatalf("missing-ask provenance = %+v", decision)
	}
}

func TestCDFSupplierQuotesMissingBidOnlyWithNoGrossInventory(t *testing.T) {
	gateway := newMetaGateway()
	decisions := make([]ElasticLiquiditySupplierDecision, 0)
	cfg := cdfSupplierUnitConfig()
	cfg.DecisionObserver = func(decision ElasticLiquiditySupplierDecision) {
		decisions = append(decisions, decision)
	}
	supplier := NewElasticLiquiditySupplier(1, gateway, cfg)
	supplier.position = -cfg.InitialBaseBalance
	// Selling the finite initial inventory would have produced this cash state.
	supplier.quoteCashAvailable = cfg.InitialQuoteBalance + cfg.ReferencePrice*cfg.InitialBaseBalance
	supplier.onTick(time.Unix(0, int64(time.Second)))
	supplier.HandleEvent(context.Background(), cdfSupplierBookEvent(cfg.Symbol, int64(time.Second), 12, 0, 800, 0, 100))
	supplier.onTick(time.Unix(0, int64(2*time.Second)))
	orders := gateway.orders()
	if len(orders) != 1 {
		t.Fatalf("orders = %d, want one missing-bid quote; decisions = %+v", len(orders), decisions)
	}
	if orders[0].Side != exchange.Buy || orders[0].Price != 700 || orders[0].Qty != 50 {
		t.Fatalf("missing-bid order = %+v, want bounded buy of 50 at 700", orders[0])
	}
	decision := decisions[len(decisions)-1]
	if decision.RiskMarkSource != "one_sided_ask_zero_inventory" || !decision.EquityAvailable {
		t.Fatalf("missing-bid risk provenance = %+v", decision)
	}
}

func TestCDFSupplierAskOnlyWithGrossInventoryFailsClosed(t *testing.T) {
	gateway := newMetaGateway()
	decisions := make([]ElasticLiquiditySupplierDecision, 0)
	cfg := cdfSupplierUnitConfig()
	cfg.DecisionObserver = func(decision ElasticLiquiditySupplierDecision) {
		decisions = append(decisions, decision)
	}
	supplier := NewElasticLiquiditySupplier(1, gateway, cfg)
	supplier.position = 10
	supplier.onTick(time.Unix(0, int64(time.Second)))
	supplier.HandleEvent(context.Background(), cdfSupplierBookEvent(cfg.Symbol, int64(time.Second), 13, 0, 800, 0, 100))
	supplier.onTick(time.Unix(0, int64(2*time.Second)))
	if got := len(gateway.orders()); got != 0 {
		t.Fatalf("ask-only gross-inventory orders = %d, want zero", got)
	}
	decision := decisions[len(decisions)-1]
	if decision.Action != "wait" || decision.Reason != "equity_unavailable" || decision.EquityAvailable || decision.RiskMarkPrice != 0 {
		t.Fatalf("ask-only fail-closed decision = %+v", decision)
	}
}

func TestCDFSupplierFiniteQuoteCashBoundsBuyAndReconcilesFill(t *testing.T) {
	gateway := newMetaGateway()
	fills := make([]ElasticLiquiditySupplierFill, 0)
	cfg := cdfSupplierUnitConfig()
	cfg.InitialQuoteBalance = 1_000
	cfg.FillObserver = func(fill ElasticLiquiditySupplierFill) { fills = append(fills, fill) }
	supplier := NewElasticLiquiditySupplier(1, gateway, cfg)
	supplier.onTick(time.Unix(0, int64(time.Second)))
	// The midpoint is 800, so the inventory target is positive and the supplier
	// must buy. One unit costs 808 including the registered 1% maker fee; two do
	// not fit in the finite quote wallet.
	supplier.HandleEvent(context.Background(), cdfSupplierBookEvent(cfg.Symbol, int64(time.Second), 14, 800, 900, 100, 100))
	supplier.onTick(time.Unix(0, int64(2*time.Second)))
	orders := gateway.orders()
	if len(orders) != 1 || orders[0].Side != exchange.Buy || orders[0].Qty != 1 {
		t.Fatalf("finite-cash orders = %+v, want one one-unit buy", orders)
	}
	requestID := orders[0].RequestID
	supplier.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{OrderID: 41, RequestID: requestID}})
	supplier.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderFilled, Data: actor.OrderFillEvent{
		OrderID: 41, Symbol: cfg.Symbol, Side: exchange.Buy, Qty: 1, Price: 800,
		FeeAmount: 8, FeeAsset: cfg.QuoteAsset, IsFull: true, TradeID: 71,
	}})
	if supplier.Position() != 1 || supplier.quoteCashAvailable != 192 || supplier.quoteCashReserved != 0 {
		t.Fatalf("post-fill state = position %d, cash %d, reserved %d; want (1,192,0)", supplier.Position(), supplier.quoteCashAvailable, supplier.quoteCashReserved)
	}
	if len(fills) != 1 || fills[0].PositionBefore != 0 || fills[0].PositionAfter != 1 || !fills[0].IsFull {
		t.Fatalf("fill evidence = %+v, want one reconciled full fill", fills)
	}
}

func TestCDFSupplierLossBudgetWithdraws(t *testing.T) {
	gateway := newMetaGateway()
	decisions := make([]ElasticLiquiditySupplierDecision, 0)
	cfg := cdfSupplierUnitConfig()
	cfg.MaxLossQuote = 500
	cfg.DecisionObserver = func(decision ElasticLiquiditySupplierDecision) {
		decisions = append(decisions, decision)
	}
	supplier := NewElasticLiquiditySupplier(1, gateway, cfg)
	supplier.position = 10
	supplier.quoteCashAvailable = 0
	supplier.onTick(time.Unix(0, int64(time.Second)))
	supplier.HandleEvent(context.Background(), cdfSupplierBookEvent(cfg.Symbol, int64(time.Second), 15, 89, 91, 100, 100))
	supplier.onTick(time.Unix(0, int64(2*time.Second)))
	decision := decisions[len(decisions)-1]
	if decision.Action != "wait" || decision.Reason != "loss_limit" || !decision.RiskLimitTriggered {
		t.Fatalf("loss-limit decision = %+v", decision)
	}
	if decision.RiskMarkPrice != 90 || decision.EquityQuote != 9_900 || decision.LossFromInitialQuote != 190_100 {
		t.Fatalf("loss accounting = %+v, want mark 90/equity 9900/loss 190100", decision)
	}
}

func TestCDFSupplierWithdrawsAndDoesNotReplenishAfterEmptyBook(t *testing.T) {
	gateway := newMetaGateway()
	cfg := cdfSupplierUnitConfig()
	supplier := NewElasticLiquiditySupplier(1, gateway, cfg)
	supplier.onTick(time.Unix(0, int64(time.Second)))
	supplier.HandleEvent(context.Background(), cdfSupplierBookEvent(cfg.Symbol, int64(time.Second), 16, 1_200, 1_300, 100, 100))
	supplier.onTick(time.Unix(0, int64(2*time.Second)))
	orders := gateway.orders()
	if len(orders) != 1 {
		t.Fatalf("initial orders = %+v, want one", orders)
	}
	supplier.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{OrderID: 42, RequestID: orders[0].RequestID}})
	supplier.HandleEvent(context.Background(), cdfSupplierBookEvent(cfg.Symbol, int64(3*time.Second), 17, 0, 0, 0, 0))
	supplier.onTick(time.Unix(0, int64(4*time.Second)))
	if len(gateway.requests) != 3 || gateway.requests[2].Type != etypes.ReqCancelOrder || gateway.requests[2].CancelReq.OrderID != 42 {
		t.Fatalf("empty-book requests = %+v, want one withdrawal cancel", gateway.requests)
	}
	supplier.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderCancelled, Data: actor.OrderCancelledEvent{OrderID: 42, RequestID: gateway.requests[2].CancelReq.RequestID}})
	before := len(gateway.orders())
	supplier.onTick(time.Unix(0, int64(5*time.Second)))
	if len(gateway.orders()) != before {
		t.Fatalf("orders after withdrawal = %+v, want no forced replenishment", gateway.orders())
	}
}

func TestCDFSupplierIntegerPriceHelpersFailClosed(t *testing.T) {
	if price, ok := floorToPositiveTick(math.MaxInt64, 2); !ok || price != math.MaxInt64-1 {
		t.Fatalf("floor rounding = (%d,%t), want (%d,true)", price, ok, math.MaxInt64-1)
	}
	if price, ok := ceilToPositiveTick(1_001, 100); !ok || price != 1_100 {
		t.Fatalf("ceil rounding = (%d,%t), want (1100,true)", price, ok)
	}
	if price, ok := ceilToPositiveTick(math.MaxInt64, 2); ok || price != 0 {
		t.Fatalf("ceil overflow = (%d,%t), want (0,false)", price, ok)
	}
	if amount, ok := exactQuoteAmount(math.MaxInt64, 1); ok || amount != 0 {
		t.Fatalf("quote overflow = (%d,%t), want (0,false)", amount, ok)
	}
}

func TestCDFSupplierSpecRejectsNonCDFRoleAndInvalidOneSidedContract(t *testing.T) {
	spec := ElasticLiquiditySupplierSpec{
		Role: "other_supplier_1", Symbol: "CDF/USD", BaseAsset: "CDF", QuoteAsset: "USD",
		BasePrecision: 1, QuotePrecision: 1, InitialBaseBalance: 100, InitialQuoteBalance: 100_000,
		Interval: time.Second, MaxObservationAge: time.Minute, ReferencePrice: 1_000,
		ReferenceHalfLife: time.Hour, ElasticityPerPercent: 10, MaxPosition: 100,
		MaxInventory: 200, MaxQuoteQty: 50,
	}
	if err := spec.validate(); err == nil {
		t.Fatal("non-CDF supplier role was accepted")
	}
	spec.Role = "cdf_elastic_supplier_1"
	spec.QuoteOnOneSidedLocalBook = true
	if err := spec.validate(); err == nil {
		t.Fatal("one-sided supplier without risk/tick contract was accepted")
	}
}

func TestCDFSupplierRosterIsOptInAndSeparateFromHistoricalSuppliers(t *testing.T) {
	defaultSim, err := NewSim(time.Second, Config{LogDir: t.TempDir(), LogMode: "none", ElasticSupplierCount: 8})
	if err != nil {
		t.Fatalf("default NewSim: %v", err)
	}
	for _, venue := range defaultSim.Venues {
		if len(venue.Suppliers) != 8 || len(venue.ElasticLiquiditySuppliers) != 0 {
			defaultSim.Close()
			t.Fatalf("default rosters = (%d historical,%d successor), want (8,0)", len(venue.Suppliers), len(venue.ElasticLiquiditySuppliers))
		}
	}
	defaultSim.Close()

	spec := ElasticLiquiditySupplierSpec{
		Role: "cdf_elastic_supplier_1", Symbol: "CDF/USD", BaseAsset: "CDF", QuoteAsset: "USD",
		BasePrecision: mvBasePrecision, QuotePrecision: mvQuotePrecision,
		InitialBaseBalance: 4_000_000_000, InitialQuoteBalance: 18_000_000_000,
		Interval: 2 * time.Second, DecisionPhaseOffset: time.Second / 2, MaxObservationAge: time.Minute,
		ReferencePrice: mvCDFBootstrap, ReferenceHalfLife: 3 * time.Hour,
		ElasticityPerPercent: 12_000_000_000, MaxPosition: 4_000_000_000, MaxInventory: 8_000_000_000,
		MaxQuoteQty: 40_000_000, MinimumExecutableQty: 100_000, MinimumQualifyingQty: 1_000_000,
		RegisteredMinimumExecutableQty: mvBasePrecision / 1_000, TickSize: mvQuotePrecision,
		QuoteOnOneSidedLocalBook: true, MaxLossQuote: 3_000_000_000, MakerFeeBps: 5,
	}
	successor, err := NewSim(time.Second, Config{
		LogDir: t.TempDir(), LogMode: "none", EvidenceFormat: binaryRepresentation, EvidenceContractVersion: 2,
		RecordElasticLiquiditySupplierDecisions: true, RecordMarketDataReceipts: true,
		MarketDataReceiptRoles: []string{"cdf_elastic_supplier"},
		CrossAssetSpotGraph:    true, ElasticSupplierCount: 8,
		StrictPopulationAccounting: true, StrictRiskContract: true, AutoBorrowSpot: boolPointer(false),
		ElasticLiquiditySuppliers: []ElasticLiquiditySupplierSpec{spec},
		LatencyProfiles: map[string]LatencyProfile{
			"cdf_elastic_supplier": {Model: "constant", Delay: time.Millisecond},
		},
	})
	if err != nil {
		t.Fatalf("successor NewSim: %v", err)
	}
	defer successor.Close()
	for _, venue := range successor.Venues {
		if len(venue.Suppliers) != 8 || len(venue.ElasticLiquiditySuppliers) != 1 {
			t.Fatalf("successor rosters = (%d historical,%d successor), want (8,1)", len(venue.Suppliers), len(venue.ElasticLiquiditySuppliers))
		}
	}
}

func TestCDFSupplierRosterRequiresStrictNoDebtContract(t *testing.T) {
	spec := ElasticLiquiditySupplierSpec{
		Role: "cdf_elastic_supplier_1", Symbol: "CDF/USD", BaseAsset: "CDF", QuoteAsset: "USD",
		BasePrecision: mvBasePrecision, QuotePrecision: mvQuotePrecision,
		InitialBaseBalance: 4_000_000_000, InitialQuoteBalance: 18_000_000_000,
		Interval: 2 * time.Second, MaxObservationAge: time.Minute,
		ReferencePrice: mvCDFBootstrap, ReferenceHalfLife: 3 * time.Hour,
		ElasticityPerPercent: 12_000_000_000, MaxPosition: 4_000_000_000, MaxInventory: 8_000_000_000,
		MaxQuoteQty: 40_000_000, MinimumExecutableQty: 100_000, MinimumQualifyingQty: 1_000_000,
		RegisteredMinimumExecutableQty: mvBasePrecision / 1_000, TickSize: mvQuotePrecision,
		QuoteOnOneSidedLocalBook: true, MaxLossQuote: 3_000_000_000,
	}
	_, err := NewSim(time.Second, Config{
		LogDir: t.TempDir(), LogMode: "none", CrossAssetSpotGraph: true,
		ElasticLiquiditySuppliers: []ElasticLiquiditySupplierSpec{spec},
		LatencyProfiles: map[string]LatencyProfile{
			"cdf_elastic_supplier": {Model: "constant", Delay: time.Millisecond},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "strict risk contract") {
		t.Fatalf("CDF roster without strict no-debt contract: %v", err)
	}
}

func boolPointer(value bool) *bool { return &value }
