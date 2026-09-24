package analysis

import (
	"fmt"
	"slices"

	etypes "exchange_sim/types"
)

type CrossVenueEdgeWorldInput struct {
	Seed               int64
	Arm                string
	BackgroundIdentity string
	Venues             [2]string
	Transitions        []CrossVenuePublicTransition
	HorizonNano        int64
	LotQty             int64
	BasePrecision      int64
	TakerFeeBps        int64
}

type CrossVenueEdgeWorldSummary struct {
	Seed                    int64
	Arm                     string
	BackgroundIdentity      string
	Venues                  [2]string
	HorizonNano             int64
	LotQty                  int64
	BasePrecision           int64
	TakerFeeBps             int64
	EpisodeCount            int
	CensoredEpisodes        int
	PositiveNanos           int64
	NoOpportunity           bool
	BroaderEpisodeCount     int
	BroaderCensoredEpisodes int
	BroaderPositiveNanos    int64
}

type CrossVenuePairedEdgeContrast struct {
	Seed               int64
	Off                CrossVenueEdgeWorldSummary
	On                 CrossVenueEdgeWorldSummary
	OnMinusOffNanos    int64
	OnMinusOffEpisodes int
	TradeAttribution   string
}

// SummarizeCrossVenueEdgeWorld counts event-time executable-edge episodes.
// A same-timestamp episode is counted but contributes zero duration.
func SummarizeCrossVenueEdgeWorld(input CrossVenueEdgeWorldInput) (CrossVenueEdgeWorldSummary, error) {
	if input.Arm != "OFF" && input.Arm != "ON" || input.BackgroundIdentity == "" {
		return CrossVenueEdgeWorldSummary{}, fmt.Errorf("cross-venue edge world: missing arm or background identity")
	}
	episodes, err := ReconstructCrossVenueEdgeEpisodes(input.Venues, input.Transitions, input.HorizonNano, input.LotQty, input.BasePrecision, input.TakerFeeBps)
	if err != nil {
		return CrossVenueEdgeWorldSummary{}, err
	}
	broaderEpisodes, err := ReconstructCrossVenueLegSideEdgeEpisodes(input.Venues, input.Transitions, input.HorizonNano, input.LotQty, input.BasePrecision, input.TakerFeeBps)
	if err != nil {
		return CrossVenueEdgeWorldSummary{}, err
	}
	result := CrossVenueEdgeWorldSummary{
		Seed: input.Seed, Arm: input.Arm, BackgroundIdentity: input.BackgroundIdentity,
		Venues: input.Venues, HorizonNano: input.HorizonNano, LotQty: input.LotQty,
		BasePrecision: input.BasePrecision, TakerFeeBps: input.TakerFeeBps,
		EpisodeCount: len(episodes), NoOpportunity: len(episodes) == 0,
		BroaderEpisodeCount: len(broaderEpisodes),
	}
	result.PositiveNanos, result.CensoredEpisodes, err = summarizeCrossVenueEpisodeDurations(episodes, input.HorizonNano)
	if err != nil {
		return CrossVenueEdgeWorldSummary{}, err
	}
	result.BroaderPositiveNanos, result.BroaderCensoredEpisodes, err = summarizeCrossVenueEpisodeDurations(broaderEpisodes, input.HorizonNano)
	if err != nil {
		return CrossVenueEdgeWorldSummary{}, err
	}
	if result.BroaderPositiveNanos < result.PositiveNanos {
		return CrossVenueEdgeWorldSummary{}, fmt.Errorf("cross-venue edge world: broader duration excludes policy opportunity")
	}
	return result, nil
}

func summarizeCrossVenueEpisodeDurations(episodes []CrossVenueEdgeEpisode, horizonNano int64) (int64, int, error) {
	var positiveNanos int64
	var censoredEpisodes int
	for _, episode := range episodes {
		if episode.EndTS < episode.StartTS || episode.EndTS > horizonNano {
			return 0, 0, fmt.Errorf("cross-venue edge world: episode outside horizon")
		}
		duration := episode.EndTS - episode.StartTS
		next, ok := etypes.TryAdd(positiveNanos, duration)
		if !ok || next > horizonNano {
			return 0, 0, fmt.Errorf("cross-venue edge world: episode durations exceed horizon")
		}
		positiveNanos = next
		if episode.Censored {
			censoredEpisodes++
		}
	}
	return positiveNanos, censoredEpisodes, nil
}

// CompareCrossVenuePairedEdgeWorlds requires every declared seed to have one
// valid OFF and one valid ON world. BackgroundIdentity is a caller-supplied
// design binding, not proof that stochastic streams remained coupled.
func CompareCrossVenuePairedEdgeWorlds(worlds []CrossVenueEdgeWorldSummary) ([]CrossVenuePairedEdgeContrast, error) {
	if len(worlds) == 0 || len(worlds)%2 != 0 {
		return nil, fmt.Errorf("cross-venue paired worlds: incomplete cell inventory")
	}
	bySeed := make(map[int64]map[string]CrossVenueEdgeWorldSummary, len(worlds)/2)
	for _, world := range worlds {
		if world.Arm != "OFF" && world.Arm != "ON" || world.BackgroundIdentity == "" || world.HorizonNano <= 0 ||
			world.LotQty <= 0 || world.BasePrecision <= 0 || world.TakerFeeBps < 0 ||
			world.Venues[0] == "" || world.Venues[1] == "" || world.Venues[0] == world.Venues[1] ||
			world.PositiveNanos < 0 || world.PositiveNanos > world.HorizonNano || world.EpisodeCount < 0 ||
			world.CensoredEpisodes < 0 || world.CensoredEpisodes > world.EpisodeCount || world.NoOpportunity != (world.EpisodeCount == 0) ||
			world.BroaderEpisodeCount < 0 || world.BroaderCensoredEpisodes < 0 || world.BroaderCensoredEpisodes > world.BroaderEpisodeCount ||
			world.BroaderPositiveNanos < world.PositiveNanos || world.BroaderPositiveNanos > world.HorizonNano ||
			world.BroaderEpisodeCount == 0 && (world.EpisodeCount != 0 || world.BroaderPositiveNanos != 0) {
			return nil, fmt.Errorf("cross-venue paired worlds: invalid world summary")
		}
		arms := bySeed[world.Seed]
		if arms == nil {
			arms = make(map[string]CrossVenueEdgeWorldSummary, 2)
			bySeed[world.Seed] = arms
		}
		if _, duplicate := arms[world.Arm]; duplicate {
			return nil, fmt.Errorf("cross-venue paired worlds: duplicate seed/arm")
		}
		arms[world.Arm] = world
	}
	seeds := make([]int64, 0, len(bySeed))
	for seed := range bySeed {
		seeds = append(seeds, seed)
	}
	slices.Sort(seeds)
	contrasts := make([]CrossVenuePairedEdgeContrast, 0, len(seeds))
	for _, seed := range seeds {
		arms := bySeed[seed]
		off, offExists := arms["OFF"]
		on, onExists := arms["ON"]
		if !offExists || !onExists || off.BackgroundIdentity != on.BackgroundIdentity || off.HorizonNano != on.HorizonNano ||
			off.LotQty != on.LotQty || off.BasePrecision != on.BasePrecision || off.TakerFeeBps != on.TakerFeeBps ||
			!sameCrossVenuePair(off.Venues, on.Venues) {
			return nil, fmt.Errorf("cross-venue paired worlds: seed %d has missing or incompatible arms", seed)
		}
		difference, ok := etypes.TrySub(on.PositiveNanos, off.PositiveNanos)
		if !ok {
			return nil, fmt.Errorf("cross-venue paired worlds: duration contrast overflows")
		}
		contrasts = append(contrasts, CrossVenuePairedEdgeContrast{
			Seed: seed, Off: off, On: on, OnMinusOffNanos: difference,
			OnMinusOffEpisodes: on.EpisodeCount - off.EpisodeCount,
			TradeAttribution:   "NOT_IDENTIFIED",
		})
	}
	return contrasts, nil
}

func sameCrossVenuePair(left, right [2]string) bool {
	return left == right || left[0] == right[1] && left[1] == right[0]
}
