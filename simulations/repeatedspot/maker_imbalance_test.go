package repeatedspot

import (
	"context"
	"math"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

func e1ImbalanceConfig() ImbalanceStoikovMakerConfig {
	return ImbalanceStoikovMakerConfig{
		LocalReferenceStoikovMakerConfig: LocalReferenceStoikovMakerConfig{
			BoundedStoikovMakerConfig: e0StoikovPolicyConfig(), MaxAge: 15 * time.Second,
		},
		SignalGainBps: 20, MaxSignalAge: 10 * time.Second,
	}
}

func TestImbalanceStoikovQuoteNestsAndShifts(t *testing.T) {
	config := e1ImbalanceConfig()
	input := MakerQuoteInput{MidPrice: 50_000 * e0QuotePrecision,
		WorkingLimit: 10 * e0BasePrecision, LogVariancePerSecond: 1e-8,
		TopBidVisibleQty: 3 * e0BasePrecision, TopAskVisibleQty: e0BasePrecision,
		BookSourceAge: 5 * time.Second}
	base, ok := stoikovMakerQuoteRule(config.BoundedStoikovMakerConfig)(input)
	if !ok {
		t.Fatal("AS base quote unavailable")
	}
	rule := imbalanceStoikovQuoteRule(config)
	positive, ok := rule(input)
	if !ok || positive.BidPrice-base.BidPrice != 50*e0QuotePrecision ||
		positive.AskPrice-base.AskPrice != 50*e0QuotePrecision {
		t.Fatalf("positive imbalance quote=%+v base=%+v, valid=%t", positive, base, ok)
	}
	input.TopBidVisibleQty, input.TopAskVisibleQty = input.TopAskVisibleQty, input.TopBidVisibleQty
	negative, ok := rule(input)
	if !ok || negative.BidPrice-base.BidPrice != -50*e0QuotePrecision ||
		negative.AskPrice-base.AskPrice != -50*e0QuotePrecision {
		t.Fatalf("negative imbalance quote=%+v base=%+v, valid=%t", negative, base, ok)
	}
	input.TopBidVisibleQty = input.TopAskVisibleQty
	if equal, ok := rule(input); !ok || equal != base {
		t.Fatalf("equal displayed size failed exact nesting: %+v, %t", equal, ok)
	}
	input.TopBidVisibleQty = 3 * e0BasePrecision
	input.BookSourceAge = config.MaxSignalAge
	if stale, ok := rule(input); !ok || stale != base {
		t.Fatalf("exact signal expiry failed nesting: %+v, %t", stale, ok)
	}
	input.BookSourceAge = 5 * time.Second
	input.TopAskVisibleQty = 0
	if oneSided, ok := rule(input); !ok || oneSided != base {
		t.Fatalf("one-sided signal did not abstain from directional shift: %+v, %t", oneSided, ok)
	}
	config.SignalGainBps = 0
	if zeroGain, ok := imbalanceStoikovQuoteRule(config)(input); !ok || zeroGain != base {
		t.Fatalf("zero-gain signal failed exact AS nesting: %+v, %t", zeroGain, ok)
	}
}

func TestImbalanceStoikovFailsClosedOnInvalidDepthAndPrice(t *testing.T) {
	config := e1ImbalanceConfig()
	rule := imbalanceStoikovQuoteRule(config)
	input := MakerQuoteInput{MidPrice: 50_000 * e0QuotePrecision,
		WorkingLimit: 10 * e0BasePrecision, LogVariancePerSecond: 1e-8,
		TopBidVisibleQty: 1, TopAskVisibleQty: 1}
	for name, mutate := range map[string]func(*MakerQuoteInput){
		"negative displayed depth": func(input *MakerQuoteInput) { input.TopBidVisibleQty = -1 },
		"future snapshot age":      func(input *MakerQuoteInput) { input.BookSourceAge = -time.Second },
		"size-sum overflow": func(input *MakerQuoteInput) {
			input.TopBidVisibleQty, input.TopAskVisibleQty = math.MaxInt64, math.MaxInt64
		},
		"invalid price": func(input *MakerQuoteInput) { input.MidPrice = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			caseInput := input
			mutate(&caseInput)
			if _, ok := rule(caseInput); ok {
				t.Fatal("invalid signal or price produced a quote")
			}
		})
	}
	config.SignalGainBps = math.MaxInt64
	input.TopBidVisibleQty, input.TopAskVisibleQty = 3, 1
	if _, ok := imbalanceStoikovQuoteRule(config)(input); ok {
		t.Fatal("unrepresentable signal gain produced a quote")
	}
	config = e1ImbalanceConfig()
	config.SignalGainBps = 20_000
	input.TopBidVisibleQty, input.TopAskVisibleQty = 1, 1_000_000
	if _, ok := imbalanceStoikovQuoteRule(config)(input); ok {
		t.Fatal("negative-price directional shift produced a quote")
	}
	config.MaxSignalAge = 0
	if _, err := NewImbalanceStoikovMakerPolicy(config); err == nil {
		t.Fatal("zero signal lifetime accepted")
	}
	config = e1ImbalanceConfig()
	config.SignalGainBps = -1
	if _, err := NewImbalanceStoikovMakerPolicy(config); err == nil {
		t.Fatal("negative signal gain accepted")
	}
}

func TestImbalanceStoikovTruncatesSubtickSignalTowardZero(t *testing.T) {
	config := e1ImbalanceConfig()
	config.Maker.TickSize = 10 * e0QuotePrecision
	config.SignalGainBps = 1
	input := MakerQuoteInput{MidPrice: 50_000 * e0QuotePrecision,
		WorkingLimit: 10 * e0BasePrecision, LogVariancePerSecond: 1e-8,
		TopBidVisibleQty: 3 * e0BasePrecision, TopAskVisibleQty: e0BasePrecision}
	base, ok := stoikovMakerQuoteRule(config.BoundedStoikovMakerConfig)(input)
	if !ok {
		t.Fatal("AS base quote unavailable")
	}
	for _, sizes := range [][2]int64{{3 * e0BasePrecision, e0BasePrecision},
		{e0BasePrecision, 3 * e0BasePrecision}} {
		input.TopBidVisibleQty, input.TopAskVisibleQty = sizes[0], sizes[1]
		quote, ok := imbalanceStoikovQuoteRule(config)(input)
		if !ok || quote != base {
			t.Fatalf("subtick signed shift did not truncate to exact base quote: %+v %+v", quote, base)
		}
	}
}

func TestImbalanceMakerUsesDeliveredSnapshotAndSharedLifecycle(t *testing.T) {
	config := e1ImbalanceConfig()
	config.SignalGainBps = 1
	gateway := &makerRecordingGateway{response: make(chan exchange.Response), data: make(chan *exchange.MarketDataMsg)}
	maker, err := NewRecurringMakerWithInformation(7, gateway, config.Maker,
		imbalanceStoikovQuoteRule(config), MakerInformationOptions{
			LocalReferenceMaxAge: config.MaxAge, EmitReferenceEvidence: true, EmitTopDepthEvidence: true,
		})
	if err != nil {
		t.Fatal(err)
	}
	var observations []MakerObservation
	var decisions []MakerDecision
	maker.SetObservationObserver(func(observation MakerObservation) { observations = append(observations, observation) })
	maker.SetDecisionObserver(func(decision MakerDecision) { decisions = append(decisions, decision) })
	maker.onTick(time.Unix(5, 0))
	snapshot := &exchange.BookSnapshot{
		Bids: []exchange.PriceLevel{{Price: 49_999 * e0QuotePrecision, VisibleQty: 3 * e0BasePrecision}},
		Asks: []exchange.PriceLevel{{Price: 50_001 * e0QuotePrecision, VisibleQty: e0BasePrecision}},
	}
	maker.HandleEvent(context.Background(), &actor.Event{Type: actor.EventBookSnapshot,
		Data: actor.BookSnapshotEvent{Symbol: e0Symbol, Snapshot: snapshot, Timestamp: int64(5 * time.Second), SeqNum: 7}})
	snapshot.Bids[0].VisibleQty = e0BasePrecision
	maker.onTick(time.Unix(10, 0))
	if err := maker.Fault(); err != nil {
		t.Fatal(err)
	}
	if len(observations) != 1 || observations[0].TopBidVisibleQty == nil ||
		*observations[0].TopBidVisibleQty != 3*e0BasePrecision ||
		observations[0].TopAskVisibleQty == nil || *observations[0].TopAskVisibleQty != e0BasePrecision {
		t.Fatalf("local delivered depth changed after source snapshot mutation: %+v", observations)
	}
	if len(decisions) != 2 || decisions[1].TopBidVisibleQty == nil ||
		*decisions[1].TopBidVisibleQty != 3*e0BasePrecision ||
		decisions[1].LatestBookSourceAt != int64(5*time.Second) ||
		decisions[1].DecisionAt != int64(10*time.Second) || decisions[1].Action != "evaluate_placements" {
		t.Fatalf("signal decision did not use delivered state: %+v", decisions)
	}
	if len(gateway.requests) != 3 {
		t.Fatalf("expected subscription and one post-only quote pair, got %+v", gateway.requests)
	}
	if futureQuote, reference, usable := maker.targetQuote(int64(4 * time.Second)); usable || reference.mode != "unavailable" || futureQuote != (MakerQuote{}) {
		t.Fatalf("future-dated local book produced quote: %+v %+v", futureQuote, reference)
	}
	for _, request := range gateway.requests[1:] {
		if request.OrderReq == nil || !request.OrderReq.PostOnly || request.OrderReq.Qty != config.Maker.QuoteQty {
			t.Fatalf("new signal policy bypassed shared maker lifecycle: %+v", request)
		}
	}
	if lower, upper, err := maker.inventory.envelope(); err != nil ||
		lower < -config.Maker.WorkingLimit || upper > config.Maker.WorkingLimit {
		t.Fatalf("signal maker breached pending-inclusive cap: (%d,%d,%v)", lower, upper, err)
	}
	priorQuote, priorReference, usable := maker.targetQuote(int64(15 * time.Second))
	baseQuote, baseUsable := stoikovMakerQuoteRule(config.BoundedStoikovMakerConfig)(MakerQuoteInput{
		MidPrice: 50_000 * e0QuotePrecision, WorkingLimit: config.Maker.WorkingLimit,
		LogVariancePerSecond: config.Maker.InitialLogVariancePerSecond,
	})
	if !usable || !baseUsable || priorReference.mode != "live_two_sided" || priorQuote != baseQuote {
		t.Fatalf("signal did not expire at exact source-age boundary: quote=%+v ref=%+v", priorQuote, priorReference)
	}
	maker.HandleEvent(context.Background(), &actor.Event{Type: actor.EventBookSnapshot,
		Data: actor.BookSnapshotEvent{Symbol: e0Symbol, Snapshot: &exchange.BookSnapshot{
			Bids: []exchange.PriceLevel{{Price: 49_999 * e0QuotePrecision, VisibleQty: e0BasePrecision}},
		}, Timestamp: int64(11 * time.Second), SeqNum: 8}})
	noSignalQuote, reference, usable := maker.targetQuote(int64(12 * time.Second))
	if !usable || reference.mode != "cached_two_sided" || noSignalQuote != baseQuote ||
		*observations[0].TopBidVisibleQty != 3*e0BasePrecision {
		t.Fatalf("one-sided local state used directional signal or mutated prior evidence: %+v %+v", noSignalQuote, reference)
	}
}

func TestLegacyMakerEmitsNoTopDepthEvidence(t *testing.T) {
	maker, _ := makerFixture(t)
	var observations []MakerObservation
	var decisions []MakerDecision
	maker.SetObservationObserver(func(observation MakerObservation) { observations = append(observations, observation) })
	maker.SetDecisionObserver(func(decision MakerDecision) { decisions = append(decisions, decision) })
	maker.onTick(time.Time{})
	observeMakerBook(maker, 99, 101)
	maker.onTick(time.Time{})
	if len(observations) != 1 || observations[0].TopBidVisibleQty != nil ||
		observations[0].TopAskVisibleQty != nil || len(decisions) != 2 ||
		decisions[1].TopBidVisibleQty != nil || decisions[1].TopAskVisibleQty != nil {
		t.Fatalf("legacy maker emitted changed evidence payload: obs=%+v decisions=%+v", observations, decisions)
	}
}

func TestImbalanceMakerReceivesOnlyDelayedGatewayDepth(t *testing.T) {
	signal, err := NewImbalanceStoikovMakerPolicy(e1ImbalanceConfig())
	if err != nil {
		t.Fatal(err)
	}
	pure, err := NewBoundedFixedMakerPolicy(BoundedFixedMakerConfig{Maker: e0RecurringMakerConfig(), SpreadBps: 2})
	if err != nil {
		t.Fatal(err)
	}
	config, err := DraftE0Config([4]PolicyDefinition{signal, pure, pure, pure},
		[4]int64{1, 2, 3, 4}, [2]int64{5, 6})
	if err != nil {
		t.Fatal(err)
	}
	config.Iterations = 40
	world, err := Build(config)
	if err != nil {
		t.Fatal(err)
	}
	maker, ok := world.actors[1].(*RecurringMaker)
	if !ok {
		t.Fatalf("signal policy did not compose through the production world: %T", world.actors[1])
	}
	var observations []MakerObservation
	var decisions []MakerDecision
	maker.SetObservationObserver(func(observation MakerObservation) { observations = append(observations, observation) })
	maker.SetDecisionObserver(func(decision MakerDecision) { decisions = append(decisions, decision) })
	if err := world.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(observations) == 0 || len(decisions) == 0 {
		t.Fatal("production gateway fixture produced no signal-maker observations or decisions")
	}
	var deliveredTwoSided, decisionsUsingDeliveredBook int
	for _, observation := range observations {
		if observation.Kind != "snapshot" {
			continue
		}
		if observation.TopBidVisibleQty == nil || observation.TopAskVisibleQty == nil ||
			observation.ProcessedAt-observation.SourceAt < int64(config.Participants[1].Latency.MarketDataLatency) {
			t.Fatalf("signal-maker snapshot bypassed delayed gateway or omitted depth: %+v", observation)
		}
		if observation.BestBid > 0 && observation.BestAsk > observation.BestBid &&
			*observation.TopBidVisibleQty > 0 && *observation.TopAskVisibleQty > 0 {
			deliveredTwoSided++
		}
	}
	for _, decision := range decisions {
		if !decision.BookSeen {
			continue
		}
		if decision.TopBidVisibleQty == nil || decision.TopAskVisibleQty == nil ||
			decision.LatestBookProcessedAt > decision.DecisionAt ||
			decision.LatestBookSourceAt > decision.LatestBookProcessedAt ||
			decision.LatestBookProcessedAt-decision.LatestBookSourceAt < int64(config.Participants[1].Latency.MarketDataLatency) {
			t.Fatalf("signal-maker decision used future or undelivered depth: %+v", decision)
		}
		decisionsUsingDeliveredBook++
	}
	if deliveredTwoSided == 0 || decisionsUsingDeliveredBook == 0 {
		t.Fatalf("fixture did not exercise delayed book signal: snapshots=%d decisions=%d", deliveredTwoSided, decisionsUsingDeliveredBook)
	}
}
