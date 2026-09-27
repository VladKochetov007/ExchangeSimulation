package repeatedspot

import (
	"encoding/json"
	"testing"
	"time"

	worldspot "exchange_sim/simulations/repeatedspot"
	"exchange_sim/types"
)

func TestLocalReferenceParametersAndDeliveredSnapshotReplay(t *testing.T) {
	config := worldspot.LocalReferenceFixedMakerConfig{
		BoundedFixedMakerConfig: worldspot.BoundedFixedMakerConfig{
			Maker: worldspot.RecurringMakerConfig{Symbol: "ABC/USD", QuoteQty: 6, MinQuoteQty: 1,
				WorkingLimit: 10, TickSize: 1, QuoteInterval: time.Second}, SpreadBps: 2,
		}, MaxAge: 15 * time.Second,
	}
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	participant := replayParticipant{}
	participant.Policy.Name = "bounded_fixed_maker_v3"
	participant.Policy.Parameters = raw
	parameters, err := parseMakerParameters(participant)
	if err != nil || parameters == nil || !parameters.trackReference ||
		parameters.localReferenceAge != int64(15*time.Second) {
		t.Fatalf("versioned reference parameters did not replay: %+v, %v", parameters, err)
	}
	var withoutLifetime map[string]json.RawMessage
	if err := json.Unmarshal(raw, &withoutLifetime); err != nil {
		t.Fatal(err)
	}
	delete(withoutLifetime, "local_reference_max_age_ns")
	participant.Policy.Parameters, err = json.Marshal(withoutLifetime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseMakerParameters(participant); err == nil {
		t.Fatal("versioned maker without an explicit reference lifetime was accepted")
	}

	account := &accountState{actorID: 7, clientID: 7, workingLimit: 10, maker: parameters}
	state := &replayState{
		contract: replayContract{Instrument: worldspot.InstrumentConfig{Symbol: "ABC/USD"}},
		accounts: map[uint64]*accountState{7: account},
		publicSnapshots: map[uint64]publicSnapshot{
			11: {timestamp: int64(time.Second), bids: []types.PriceLevel{{Price: 99}}, asks: []types.PriceLevel{{Price: 101}}},
			12: {timestamp: int64(2 * time.Second), bids: []types.PriceLevel{{Price: 99}}},
		},
		latestMakerSnapshot: make(map[uint64]worldspot.MakerObservation),
		lastTwoSidedMaker:   make(map[uint64]worldspot.MakerObservation),
	}
	first := worldspot.MakerObservation{ActorID: 7, Kind: "snapshot", ProcessedAt: int64(2 * time.Second),
		SourceAt: int64(time.Second), SourceSequence: 11, Symbol: "ABC/USD", BestBid: 99, BestAsk: 101}
	second := worldspot.MakerObservation{ActorID: 7, Kind: "snapshot", ProcessedAt: int64(3 * time.Second),
		SourceAt: int64(2 * time.Second), SourceSequence: 12, Symbol: "ABC/USD", BestBid: 99}
	observe := func(observation worldspot.MakerObservation) error {
		payload, err := json.Marshal(observation)
		if err != nil {
			return err
		}
		return state.makerObservation(Event{Timestamp: observation.ProcessedAt, ClientID: 7, Payload: payload})
	}
	if err := observe(first); err != nil {
		t.Fatal(err)
	}
	if err := observe(second); err != nil {
		t.Fatal(err)
	}
	decision := worldspot.MakerDecision{DecisionAt: int64(5 * time.Second), BestBid: 99,
		ReferenceMode: "cached_two_sided", ReferenceMid: 100,
		ReferenceSourceAt: int64(time.Second), ReferenceSequence: 11}
	if mid, err := state.verifiedMakerReference(account, decision); err != nil || mid != 100 {
		t.Fatalf("delivered local cache failed reconstruction: %d, %v", mid, err)
	}
	for _, mutate := range []func(*worldspot.MakerDecision){
		func(value *worldspot.MakerDecision) { value.ReferenceMid++ },
		func(value *worldspot.MakerDecision) { value.ReferenceSourceAt++ },
		func(value *worldspot.MakerDecision) { value.ReferenceSequence++ },
		func(value *worldspot.MakerDecision) { value.ReferenceMode = "live_two_sided" },
		func(value *worldspot.MakerDecision) { value.DecisionAt = int64(16 * time.Second) },
		func(value *worldspot.MakerDecision) { value.DecisionAt = 0 },
	} {
		forged := decision
		mutate(&forged)
		if _, err := state.verifiedMakerReference(account, forged); err == nil {
			t.Fatalf("forged or expired reference accepted: %+v", forged)
		}
	}

	bad := second
	bad.SourceSequence = 999
	if err := observe(bad); err == nil {
		t.Fatal("unpublished source sequence accepted")
	}
	old := first
	old.ProcessedAt = int64(4 * time.Second)
	if err := observe(old); err == nil {
		t.Fatal("source-time regression accepted")
	}
}
