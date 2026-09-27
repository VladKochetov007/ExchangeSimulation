package repeatedspot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	worldspot "exchange_sim/simulations/repeatedspot"
)

func signalMakerFixture(t *testing.T) *worldspot.World {
	return signalMakerFixtureWith(t, 3, 1, 5*time.Second, false)
}

func signalMakerFixtureWith(t *testing.T, seedBidQty, seedAskQty int64, signalAge time.Duration, consumeAsks bool) *worldspot.World {
	t.Helper()
	seed, err := worldspot.NewSeedOncePolicy(worldspot.SeedOnceConfig{
		Symbol: "ABC/USD", BidPrice: 99, AskPrice: 101, BidQty: seedBidQty, AskQty: seedAskQty,
	})
	if err != nil {
		t.Fatal(err)
	}
	config := worldspot.ImbalanceStoikovMakerConfig{
		LocalReferenceStoikovMakerConfig: worldspot.LocalReferenceStoikovMakerConfig{
			BoundedStoikovMakerConfig: worldspot.BoundedStoikovMakerConfig{
				Maker: worldspot.RecurringMakerConfig{Symbol: "ABC/USD", QuoteQty: 1,
					MinQuoteQty: 1, WorkingLimit: 5, TickSize: 1, QuoteInterval: 2 * time.Second,
					InitialLogVariancePerSecond: 1e-8, VolatilityHalfLife: 4 * time.Second,
					VolatilitySampleInterval: time.Second, MaxLogVarianceMultiple: 4},
				QuotePrecision: 1, RelativeRiskAversion: 50, RelativeFillDecay: 20_000,
				InventoryHorizon: 10 * time.Second, MinHalfSpreadTicks: 1,
			}, MaxAge: 8 * time.Second,
		}, SignalGainBps: 300, MaxSignalAge: signalAge,
	}
	maker, err := worldspot.NewImbalanceStoikovMakerPolicy(config)
	if err != nil {
		t.Fatal(err)
	}
	fees, err := worldspot.NewPercentageFee(worldspot.PercentageFeeConfig{TakerBps: 100, InQuote: true})
	if err != nil {
		t.Fatal(err)
	}
	deployment := worldspot.Deployment{RequestLatency: time.Nanosecond,
		ResponseLatency: time.Nanosecond, MarketDataLatency: time.Nanosecond}
	participants := []worldspot.Participant{
		{ActorID: 1, ClientID: 1, Role: "seed", Policy: seed, Fees: fees, Latency: deployment,
			Balances: map[string]int64{"ABC": 10, "USD": 1000}},
		{ActorID: 2, ClientID: 2, Role: "signal_maker", Policy: maker, Fees: fees, Latency: deployment,
			Balances: map[string]int64{"ABC": 10, "USD": 1000}},
	}
	if consumeAsks {
		aggressor, err := worldspot.DefinePolicy("fixture_ask_consumer", struct{}{},
			func(id uint64, gateway actor.Gateway, _ exchange.TickerFactory, _ struct{}) (actor.Actor, error) {
				result := &testAggressor{BaseActor: actor.NewBaseActor(id, gateway), enabled: true, market: true, qty: 2}
				result.SetHandler(result)
				result.AddTicker(3*time.Second, result.onTick)
				return result, nil
			})
		if err != nil {
			t.Fatal(err)
		}
		participants = append(participants, worldspot.Participant{
			ActorID: 3, ClientID: 3, Role: "ask_consumer", Policy: aggressor, Fees: fees, Latency: deployment,
			Balances: map[string]int64{"ABC": 0, "USD": 1000},
		})
	}
	world, err := worldspot.Build(worldspot.Config{
		VenueID: "signal-fixture",
		Instrument: worldspot.InstrumentConfig{Symbol: "ABC/USD", BaseAsset: "ABC", QuoteAsset: "USD",
			BasePrecision: 1, QuotePrecision: 1, TickSize: 1, MinOrderSize: 1},
		Step: time.Second, Iterations: 12, SnapshotInterval: time.Second,
		ForbidBorrowing: true, RecordSnapshotProjectionEvidence: true,
		Participants: participants,
	})
	if err != nil {
		t.Fatal(err)
	}
	return world
}

func replayMutatedSignal(t *testing.T, contract []byte, events []Event, directory string) (*EconomicReplay, error) {
	t.Helper()
	var raw bytes.Buffer
	recorder, err := NewRecorderForSchema(&raw, SignalEvidenceSchemaID)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		recorder.Record(event.Timestamp, event.ClientID, event.Source, event.Name, event.Route, event.Payload)
	}
	identity, err := recorder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	return Replay(contract, bytes.NewReader(raw.Bytes()), identity, directory)
}

func TestSignalEvidenceHasOwnSchemaAndIndependentReplay(t *testing.T) {
	world := signalMakerFixture(t)
	contract, events, directory := capturedFixtureWorld(t, world)
	if _, err := replayMutatedSignal(t, contract, events, directory); err != nil {
		t.Fatalf("valid signal-maker production fixture failed independent replay: %v", err)
	}
	var seenDepth, seenDecision bool
	for _, event := range events {
		if event.ClientID != 2 {
			continue
		}
		if event.Name == "maker_observation" {
			var observation worldspot.MakerObservation
			if err := json.Unmarshal(event.Payload, &observation); err != nil {
				t.Fatal(err)
			}
			seenDepth = seenDepth || observation.Kind == "snapshot" && observation.TopBidVisibleQty != nil &&
				observation.TopAskVisibleQty != nil && *observation.TopBidVisibleQty > *observation.TopAskVisibleQty
		}
		if event.Name == "maker_decision" {
			var decision worldspot.MakerDecision
			if err := json.Unmarshal(event.Payload, &decision); err != nil {
				t.Fatal(err)
			}
			seenDecision = seenDecision || decision.TopBidVisibleQty != nil && decision.TopAskVisibleQty != nil &&
				*decision.TopBidVisibleQty > *decision.TopAskVisibleQty && decision.Action == "evaluate_placements"
		}
	}
	if !seenDepth || !seenDecision {
		t.Fatal("production fixture did not exercise unequal delivered depth and signal-maker placement")
	}
	if _, err := replayMutated(t, contract, events, directory); err == nil {
		t.Fatal("v4 evidence schema was accepted for v5 signal policy")
	}
}

func TestSignalReplayRejectsRehashedDepthAndQuoteMutations(t *testing.T) {
	contract, original, directory := capturedFixtureWorld(t, signalMakerFixture(t))
	for name, mutate := range map[string]func(*Event){
		"forged public-linked depth": func(event *Event) { replacePayloadField(t, event, "top_bid_visible_qty", 999) },
		"missing depth":              func(event *Event) { replacePayloadField(t, event, "top_bid_visible_qty", nil) },
		"forged decision depth":      func(event *Event) { replacePayloadField(t, event, "top_ask_visible_qty", 999) },
		"missing decision depth":     func(event *Event) { replacePayloadField(t, event, "top_ask_visible_qty", nil) },
		"future signal source":       func(event *Event) { replacePayloadField(t, event, "latest_book_source_at", int64(50*time.Second)) },
		"forged quote":               func(event *Event) { replacePayloadField(t, event, "target_bid", int64(99)) },
	} {
		t.Run(name, func(t *testing.T) {
			changed := append([]Event(nil), original...)
			selected := -1
			for index, event := range changed {
				if event.ClientID != 2 {
					continue
				}
				if name == "forged public-linked depth" || name == "missing depth" {
					if event.Name == "maker_observation" {
						var observation worldspot.MakerObservation
						if err := json.Unmarshal(event.Payload, &observation); err != nil {
							t.Fatal(err)
						}
						if observation.Kind == "snapshot" {
							selected = index
							break
						}
					}
				} else if event.Name == "maker_decision" {
					var decision worldspot.MakerDecision
					if err := json.Unmarshal(event.Payload, &decision); err != nil {
						t.Fatal(err)
					}
					if decision.Action == "evaluate_placements" {
						selected = index
						break
					}
				}
			}
			if selected < 0 {
				t.Fatal("fixture lacks required signal event")
			}
			changed[selected].Payload = bytes.Clone(changed[selected].Payload)
			mutate(&changed[selected])
			if _, err := replayMutatedSignal(t, contract, changed, directory); err == nil {
				t.Fatal("rehashed signal evidence corruption passed replay")
			}
		})
	}
}

func TestSignalReplayRejectsMalformedPolicyParameters(t *testing.T) {
	contract, _, _ := capturedFixtureWorld(t, signalMakerFixture(t))
	var decoded replayContract
	if err := json.Unmarshal(contract, &decoded); err != nil {
		t.Fatal(err)
	}
	participant := decoded.Participants[1]
	parameters, err := parseMakerParameters(participant)
	if err != nil || parameters == nil || !parameters.trackTopDepth ||
		parameters.signalGainBps != 300 || parameters.maxSignalAge != int64(5*time.Second) {
		t.Fatalf("signal contract was not independently parsed: %+v %v", parameters, err)
	}
	for name, mutate := range map[string]func(map[string]json.RawMessage){
		"missing gain":     func(fields map[string]json.RawMessage) { delete(fields, "signal_gain_bps") },
		"missing lifetime": func(fields map[string]json.RawMessage) { delete(fields, "max_signal_age_ns") },
		"zero lifetime":    func(fields map[string]json.RawMessage) { fields["max_signal_age_ns"] = json.RawMessage("0") },
		"negative gain":    func(fields map[string]json.RawMessage) { fields["signal_gain_bps"] = json.RawMessage("-1") },
	} {
		t.Run(name, func(t *testing.T) {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(participant.Policy.Parameters, &fields); err != nil {
				t.Fatal(err)
			}
			mutate(fields)
			changed, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			participant.Policy.Parameters = changed
			if _, err := parseMakerParameters(participant); err == nil {
				t.Fatal("malformed signal policy parameters passed replay parser")
			}
		})
	}
}

func TestIndependentSignalEstimatorHasSignedTruncationAndOverflow(t *testing.T) {
	parameters := &makerParameters{kind: "bounded_imbalance_stoikov_maker_v1", trackTopDepth: true,
		tickSize: 10, signalGainBps: 20, maxSignalAge: int64(10 * time.Second)}
	bidQty, askQty := int64(3), int64(1)
	decision := worldspot.MakerDecision{DecisionAt: int64(5 * time.Second), TopBidVisibleQty: &bidQty,
		TopAskVisibleQty: &askQty}
	bid, ask, ok := expectedImbalanceQuote(parameters, decision, 50_000, 49_900, 50_100)
	if !ok || bid != 49_950 || ask != 50_150 {
		t.Fatalf("independent positive signal accounting: %d/%d %t", bid, ask, ok)
	}
	bidQty, askQty = 1, 3
	bid, ask, ok = expectedImbalanceQuote(parameters, decision, 50_000, 49_900, 50_100)
	if !ok || bid != 49_850 || ask != 50_050 {
		t.Fatalf("independent negative signal accounting: %d/%d %t", bid, ask, ok)
	}
	parameters.signalGainBps = math.MaxInt64
	if _, _, ok := expectedImbalanceQuote(parameters, decision, 50_000, 49_900, 50_100); ok {
		t.Fatal("unrepresentable replay signal was accepted")
	}
}

func TestSignalProductionReplayHandlesNegativeExpiredAndCachedInputs(t *testing.T) {
	for _, scenario := range []struct {
		name           string
		bidQty, askQty int64
		signalAge      time.Duration
		consumeAsks    bool
		accept         func(worldspot.MakerDecision) bool
	}{
		{name: "negative imbalance", bidQty: 1, askQty: 3, signalAge: 5 * time.Second,
			accept: func(decision worldspot.MakerDecision) bool {
				return decision.Action == "evaluate_placements" && decision.TopBidVisibleQty != nil &&
					decision.TopAskVisibleQty != nil && *decision.TopBidVisibleQty < *decision.TopAskVisibleQty
			}},
		{name: "expired signal", bidQty: 3, askQty: 1, signalAge: time.Nanosecond,
			accept: func(decision worldspot.MakerDecision) bool {
				return decision.Action == "evaluate_placements" && decision.TopBidVisibleQty != nil &&
					decision.TopAskVisibleQty != nil && *decision.TopBidVisibleQty > *decision.TopAskVisibleQty &&
					decision.DecisionAt-decision.LatestBookSourceAt >= int64(time.Nanosecond)
			}},
		{name: "cached reference without directional signal", bidQty: 3, askQty: 1,
			signalAge: 5 * time.Second, consumeAsks: true,
			accept: func(decision worldspot.MakerDecision) bool {
				return decision.ReferenceMode == "cached_two_sided" && decision.TopBidVisibleQty != nil &&
					decision.TopAskVisibleQty != nil &&
					(*decision.TopBidVisibleQty == 0 || *decision.TopAskVisibleQty == 0)
			}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			contract, events, directory := capturedFixtureWorld(t,
				signalMakerFixtureWith(t, scenario.bidQty, scenario.askQty, scenario.signalAge, scenario.consumeAsks))
			if _, err := replayMutatedSignal(t, contract, events, directory); err != nil {
				t.Fatalf("valid signal variant failed independent replay: %v", err)
			}
			for _, event := range events {
				if event.ClientID != 2 || event.Name != "maker_decision" {
					continue
				}
				var decision worldspot.MakerDecision
				if err := json.Unmarshal(event.Payload, &decision); err != nil {
					t.Fatal(err)
				}
				if scenario.accept(decision) {
					return
				}
			}
			t.Fatal("fixture never exercised its declared signed, expired or cached signal condition")
		})
	}
}

func TestSignalReplayRejectsValidButChangedGainAfterRebindingContract(t *testing.T) {
	contract, original, directory := capturedFixtureWorld(t, signalMakerFixture(t))
	var document map[string]json.RawMessage
	if err := json.Unmarshal(contract, &document); err != nil {
		t.Fatal(err)
	}
	var participants []json.RawMessage
	if err := json.Unmarshal(document["participants"], &participants); err != nil {
		t.Fatal(err)
	}
	var participant, policy, parameters map[string]json.RawMessage
	if err := json.Unmarshal(participants[1], &participant); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(participant["policy"], &policy); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(policy["parameters"], &parameters); err != nil {
		t.Fatal(err)
	}
	parameters["signal_gain_bps"] = json.RawMessage("500")
	var err error
	policy["parameters"], err = json.Marshal(parameters)
	if err != nil {
		t.Fatal(err)
	}
	participant["policy"], err = json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	participants[1], err = json.Marshal(participant)
	if err != nil {
		t.Fatal(err)
	}
	document["participants"], err = json.Marshal(participants)
	if err != nil {
		t.Fatal(err)
	}
	amendedContract, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	changed := append([]Event(nil), original...)
	begin := firstEvent(t, changed, "world_begin")
	changed[begin].Payload = bytes.Clone(changed[begin].Payload)
	hash := sha256.Sum256(amendedContract)
	replacePayloadField(t, &changed[begin], "contract_sha256", hex.EncodeToString(hash[:]))
	if _, err := replayMutatedSignal(t, amendedContract, changed, directory); err == nil || !strings.Contains(err.Error(), "target quote differs") {
		t.Fatalf("valid changed gain evaded independent policy reconstruction or failed for the wrong reason: %v", err)
	}
}
