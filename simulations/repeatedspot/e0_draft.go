package repeatedspot

import (
	"fmt"
	"time"
)

const (
	e0BasePrecision  = int64(100_000_000)
	e0QuotePrecision = int64(100_000)
	e0TickSize       = e0QuotePrecision
	e0Symbol         = "ABC/USD"
)

// DraftE0Config composes the provisional equal-resource E0 roster. The caller
// chooses all four maker policies and all six random streams. This helper is
// not a locked protocol and does not authorize an economic world.
func DraftE0Config(makers [4]PolicyDefinition, takerSeeds [4]int64, roundTripSeeds [2]int64) (Config, error) {
	fee, err := NewPercentageFee(PercentageFeeConfig{TakerBps: 1, InQuote: true})
	if err != nil {
		return Config{}, err
	}
	seed, err := NewSeedOncePolicy(SeedOnceConfig{
		Symbol: e0Symbol, BidPrice: 49_950 * e0QuotePrecision, AskPrice: 50_050 * e0QuotePrecision,
		BidQty: e0BasePrecision / 10, AskQty: e0BasePrecision / 10,
	})
	if err != nil {
		return Config{}, err
	}
	deployment := Deployment{RequestLatency: time.Second, ResponseLatency: time.Second, MarketDataLatency: time.Second}
	participants := []Participant{{
		ActorID: 1, ClientID: 1, Role: "seed_once", Policy: seed, Fees: fee, Latency: deployment,
		Balances: map[string]int64{"ABC": e0BasePrecision, "USD": 10_000 * e0QuotePrecision},
	}}
	for slot, policy := range makers {
		if policy.build == nil {
			return Config{}, fmt.Errorf("repeatedspot: maker slot %d has no policy", slot+1)
		}
		id := uint64(slot + 2)
		participants = append(participants, Participant{
			ActorID: id, ClientID: id, Role: fmt.Sprintf("maker_slot_%d", slot+1),
			Policy: policy, Fees: fee, Latency: deployment,
			Balances: map[string]int64{"ABC": 50 * e0BasePrecision, "USD": 2_500_000 * e0QuotePrecision},
		})
	}
	for slot, randomSeed := range takerSeeds {
		policy, err := NewRandomTakerPolicy(RandomTakerConfig{
			Symbol: e0Symbol, TargetQty: e0BasePrecision / 10, TakeInterval: 2 * time.Second, Seed: randomSeed,
		})
		if err != nil {
			return Config{}, err
		}
		id := uint64(slot + 6)
		participants = append(participants, Participant{
			ActorID: id, ClientID: id, Role: fmt.Sprintf("random_taker_%d", slot+1),
			Policy: policy, Fees: fee, Latency: deployment,
			Balances: map[string]int64{"ABC": 20 * e0BasePrecision, "USD": 1_000_000 * e0QuotePrecision},
		})
	}
	for slot, randomSeed := range roundTripSeeds {
		policy, err := NewRoundTripPolicy(RoundTripConfig{
			Symbol: e0Symbol, BasePrecision: e0BasePrecision, LotQty: e0BasePrecision / 10,
			MinOrderSize: e0BasePrecision / 1_000, Interval: 2 * time.Second,
			HoldDuration: 5 * time.Minute, OpenProbability: 0.05, Seed: randomSeed,
		})
		if err != nil {
			return Config{}, err
		}
		id := uint64(slot + 10)
		participants = append(participants, Participant{
			ActorID: id, ClientID: id, Role: fmt.Sprintf("round_trip_%d", slot+1),
			Policy: policy, Fees: fee, Latency: deployment,
			Balances: map[string]int64{"ABC": 5 * e0BasePrecision, "USD": 250_000 * e0QuotePrecision},
		})
	}
	return Config{
		VenueID: "E0-ABC-SPOT", Instrument: InstrumentConfig{Symbol: e0Symbol, BaseAsset: "ABC", QuoteAsset: "USD",
			BasePrecision: e0BasePrecision, QuotePrecision: e0QuotePrecision,
			TickSize: e0TickSize, MinOrderSize: e0BasePrecision / 1_000},
		Step: time.Second, Iterations: 55 * 60, SnapshotInterval: time.Second,
		ForbidBorrowing: true, RecordSnapshotProjectionEvidence: true, Participants: participants,
	}, nil
}
