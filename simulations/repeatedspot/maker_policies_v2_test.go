package repeatedspot

import (
	"context"
	"math"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

func e0RecurringMakerConfig() RecurringMakerConfig {
	return RecurringMakerConfig{
		Symbol: e0Symbol, QuoteQty: e0BasePrecision / 10,
		MinQuoteQty: e0BasePrecision / 1_000, WorkingLimit: 10 * e0BasePrecision,
		TickSize: e0TickSize, QuoteInterval: 5 * time.Second, RequoteBps: 1,
		InitialLogVariancePerSecond: 1e-8, VolatilityHalfLife: 120 * time.Second,
		VolatilitySampleInterval: 30 * time.Second, MaxLogVarianceMultiple: 4,
	}
}

func e0StoikovPolicyConfig() BoundedStoikovMakerConfig {
	return BoundedStoikovMakerConfig{
		Maker: e0RecurringMakerConfig(), QuotePrecision: e0QuotePrecision,
		RelativeRiskAversion: 50, RelativeFillDecay: 20_000,
		InventoryHorizon: 600 * time.Second, MinHalfSpreadTicks: 1,
	}
}

func TestBoundedMakerQuoteRulesAtDeclaredPrior(t *testing.T) {
	input := MakerQuoteInput{MidPrice: 50_000 * e0QuotePrecision,
		WorkingLimit: 10 * e0BasePrecision, LogVariancePerSecond: 1e-8}
	fixed, ok := fixedMakerQuoteRule(2, e0TickSize)(input)
	if !ok || fixed.BidPrice != 49_990*e0QuotePrecision || fixed.AskPrice != 50_010*e0QuotePrecision {
		t.Fatalf("fixed maker quote = %+v, %t", fixed, ok)
	}
	stoikovRule := stoikovMakerQuoteRule(e0StoikovPolicyConfig())
	flat, ok := stoikovRule(input)
	if !ok || flat.BidPrice != 49_990*e0QuotePrecision || flat.AskPrice != 50_010*e0QuotePrecision {
		t.Fatalf("flat AS-style quote = %+v, %t", flat, ok)
	}
	input.FilledInventory = 10 * e0BasePrecision
	long, ok := stoikovRule(input)
	if !ok || long.BidPrice != 49_975*e0QuotePrecision || long.AskPrice != 49_995*e0QuotePrecision {
		t.Fatalf("long-inventory AS-style quote = %+v, %t", long, ok)
	}
	input.FilledInventory = -10 * e0BasePrecision
	short, ok := stoikovRule(input)
	if !ok || short.BidPrice != 50_005*e0QuotePrecision || short.AskPrice != 50_025*e0QuotePrecision {
		t.Fatalf("short-inventory AS-style quote = %+v, %t", short, ok)
	}
}

func TestRecurringMakerVarianceUsesOnlyDeliveredPastTrades(t *testing.T) {
	gateway := &makerRecordingGateway{response: make(chan exchange.Response), data: make(chan *exchange.MarketDataMsg)}
	var seen []MakerQuoteInput
	maker, err := NewRecurringMaker(7, gateway, RecurringMakerConfig{
		Symbol: "ABC/USD", QuoteQty: 1, MinQuoteQty: 1, WorkingLimit: 10,
		TickSize: 1, QuoteInterval: time.Second,
		InitialLogVariancePerSecond: 1e-8, VolatilityHalfLife: 120 * time.Second,
		VolatilitySampleInterval: 30 * time.Second, MaxLogVarianceMultiple: 4,
	}, func(input MakerQuoteInput) (MakerQuote, bool) {
		seen = append(seen, input)
		return MakerQuote{BidPrice: input.MidPrice - 2, AskPrice: input.MidPrice + 2}, true
	})
	if err != nil {
		t.Fatal(err)
	}
	maker.onTick(time.Time{})
	observeMakerBook(maker, 99, 101)
	maker.onTick(time.Time{})
	if len(seen) != 1 || seen[0].LogVariancePerSecond != 1e-8 || seen[0].DeliveredTradeSamples != 0 {
		t.Fatalf("decision used non-delivered volatility: %+v", seen)
	}
	for _, sample := range []struct {
		price int64
		at    time.Duration
	}{
		{100, 0}, {200, 10 * time.Second}, {110, 30 * time.Second},
	} {
		maker.HandleEvent(context.Background(), &actor.Event{Type: actor.EventTrade,
			Data: actor.TradeEvent{Symbol: "ABC/USD", Timestamp: int64(sample.at), Trade: &exchange.Trade{Price: sample.price}}})
	}
	if maker.Fault() != nil {
		t.Fatal(maker.Fault())
	}
	if maker.tradeSamples != 2 || math.Abs(maker.logVariancePerSecond-4e-8) > 1e-18 {
		t.Fatalf("delivered samples=%d variance=%g; want 2 and capped 4e-8", maker.tradeSamples, maker.logVariancePerSecond)
	}
	if seen[0].LogVariancePerSecond != 1e-8 {
		t.Fatal("later trade mutated the earlier decision input")
	}
	// Event-order regression is invalid evidence, not a quiet estimator reset.
	maker.HandleEvent(context.Background(), &actor.Event{Type: actor.EventTrade,
		Data: actor.TradeEvent{Symbol: "ABC/USD", Timestamp: int64(29 * time.Second), Trade: &exchange.Trade{Price: 111}}})
	if maker.Fault() == nil {
		t.Fatal("regressed delivered trade source time accepted")
	}
}

func TestBoundedMakerPoliciesComposeInSyntheticE0Fixture(t *testing.T) {
	pure, err := NewBoundedFixedMakerPolicy(BoundedFixedMakerConfig{Maker: e0RecurringMakerConfig(), SpreadBps: 2})
	if err != nil {
		t.Fatal(err)
	}
	stoikov, err := NewBoundedStoikovMakerPolicy(e0StoikovPolicyConfig())
	if err != nil {
		t.Fatal(err)
	}
	config, err := DraftE0Config([4]PolicyDefinition{pure, pure, stoikov, stoikov},
		[4]int64{1, 2, 3, 4}, [2]int64{5, 6})
	if err != nil {
		t.Fatal(err)
	}
	config.Iterations = 40 // deterministic fixture; not the 55-minute economic horizon
	world, err := Build(config)
	if err != nil {
		t.Fatal(err)
	}
	var makerPlacementRequests int
	for _, participant := range world.actors[1:5] {
		maker, ok := participant.(*RecurringMaker)
		if !ok {
			t.Fatalf("E0 v2 maker does not use shared lifecycle: %T", participant)
		}
		maker.SetOrderDecisionObserver(func(request exchange.Request) {
			if request.Type == exchange.ReqPlaceOrder {
				makerPlacementRequests++
			}
		})
	}
	if err := world.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	var makerFillIdentities int
	for _, participant := range world.actors[1:5] {
		maker := participant.(*RecurringMaker)
		makerFillIdentities += len(maker.inventory.tradeIDs)
		if lower, upper, err := maker.inventory.envelope(); err != nil || lower < -maker.config.WorkingLimit || upper > maker.config.WorkingLimit {
			t.Fatalf("maker %d breached risk envelope: (%d,%d,%v)", maker.ID(), lower, upper, err)
		}
	}
	if makerFillIdentities == 0 {
		bidDepth, askDepth := world.exchange.GetBestLiquidity(e0Symbol)
		t.Fatalf("synthetic 40-second roster never exercised a v2 maker fill; maker placements=%d bid depth=%d ask depth=%d",
			makerPlacementRequests, bidDepth, askDepth)
	}
}
