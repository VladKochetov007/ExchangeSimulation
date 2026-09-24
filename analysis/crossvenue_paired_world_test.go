package analysis

import "testing"

func crossVenuePairedWorldFixture(arm string, southBidAtEnd int64) CrossVenueEdgeWorldInput {
	return CrossVenueEdgeWorldInput{
		Seed: 607, Arm: arm, BackgroundIdentity: "same-finite-two-venue-ecology",
		Venues: [2]string{"north", "south"}, HorizonNano: 10,
		LotQty: 1, BasePrecision: 1, TakerFeeBps: 0,
		Transitions: []CrossVenuePublicTransition{
			{SimTS: 1, GlobalSequence: 1, VenueID: "north", Touch: CrossVenueTouch{HasBid: true, Bid: 90, BidQty: 1, HasAsk: true, Ask: 100, AskQty: 1}},
			{SimTS: 2, GlobalSequence: 2, VenueID: "south", Touch: CrossVenueTouch{HasBid: true, Bid: 110, BidQty: 1, HasAsk: true, Ask: 120, AskQty: 1}},
			{SimTS: 5, GlobalSequence: 3, VenueID: "south", Touch: CrossVenueTouch{HasBid: true, Bid: southBidAtEnd, BidQty: 1, HasAsk: true, Ask: 120, AskQty: 1}},
		},
	}
}

func TestCrossVenuePairedWorldsRetainEventDurationAndUnidentifiedAttribution(t *testing.T) {
	off, err := SummarizeCrossVenueEdgeWorld(crossVenuePairedWorldFixture("OFF", 110))
	if err != nil {
		t.Fatal(err)
	}
	on, err := SummarizeCrossVenueEdgeWorld(crossVenuePairedWorldFixture("ON", 95))
	if err != nil {
		t.Fatal(err)
	}
	if off.PositiveNanos != 8 || off.CensoredEpisodes != 1 || on.PositiveNanos != 3 || on.CensoredEpisodes != 0 {
		t.Fatalf("world episodes: off=%#v on=%#v", off, on)
	}
	contrasts, err := CompareCrossVenuePairedEdgeWorlds([]CrossVenueEdgeWorldSummary{on, off})
	if err != nil || len(contrasts) != 1 || contrasts[0].OnMinusOffNanos != -5 || contrasts[0].TradeAttribution != "NOT_IDENTIFIED" {
		t.Fatalf("paired world contrast = %#v, %v", contrasts, err)
	}
	on.Venues = [2]string{"south", "north"}
	if _, err := CompareCrossVenuePairedEdgeWorlds([]CrossVenueEdgeWorldSummary{off, on}); err != nil {
		t.Fatalf("venue-list permutation changed comparison: %v", err)
	}
	noOp := off
	noOp.Arm = "ON"
	contrasts, err = CompareCrossVenuePairedEdgeWorlds([]CrossVenueEdgeWorldSummary{off, noOp})
	if err != nil || contrasts[0].OnMinusOffNanos != 0 || contrasts[0].OnMinusOffEpisodes != 0 {
		t.Fatalf("no-op router world contrast = %#v, %v", contrasts, err)
	}
}

func TestCrossVenuePairedWorldsCountZeroDurationAndNoOpportunity(t *testing.T) {
	input := crossVenuePairedWorldFixture("OFF", 95)
	input.Transitions[2].SimTS = 2
	world, err := SummarizeCrossVenueEdgeWorld(input)
	if err != nil || world.EpisodeCount != 1 || world.PositiveNanos != 0 || world.NoOpportunity {
		t.Fatalf("same-time episode = %#v, %v", world, err)
	}
	input.Transitions[1].Touch.Bid = 95
	world, err = SummarizeCrossVenueEdgeWorld(input)
	if err != nil || world.EpisodeCount != 0 || !world.NoOpportunity {
		t.Fatalf("valid zero-opportunity world = %#v, %v", world, err)
	}
}

func TestCrossVenueWorldReportsBroaderLegSideDiagnosticSeparately(t *testing.T) {
	input := crossVenuePairedWorldFixture("OFF", 95)
	input.Transitions[0].Touch.HasBid = false
	input.Transitions[1].Touch.HasAsk = false
	input.Transitions[2].Touch.HasAsk = false
	world, err := SummarizeCrossVenueEdgeWorld(input)
	if err != nil || world.EpisodeCount != 0 || !world.NoOpportunity || world.PositiveNanos != 0 ||
		world.BroaderEpisodeCount != 1 || world.BroaderPositiveNanos != 3 || world.BroaderCensoredEpisodes != 0 {
		t.Fatalf("leg-side diagnostic replaced or disappeared from policy denominator: %#v, %v", world, err)
	}
	on := world
	on.Arm = "ON"
	on.BroaderPositiveNanos = -1
	if _, err := CompareCrossVenuePairedEdgeWorlds([]CrossVenueEdgeWorldSummary{world, on}); err == nil {
		t.Fatal("invalid broader duration accepted by paired comparison")
	}
}

func TestCrossVenuePairedWorldsRejectMissingOrConfoundedCells(t *testing.T) {
	off, err := SummarizeCrossVenueEdgeWorld(crossVenuePairedWorldFixture("OFF", 110))
	if err != nil {
		t.Fatal(err)
	}
	on, err := SummarizeCrossVenueEdgeWorld(crossVenuePairedWorldFixture("ON", 95))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		worlds []CrossVenueEdgeWorldSummary
	}{
		{"missing-arm", []CrossVenueEdgeWorldSummary{off}},
		{"duplicate-arm", []CrossVenueEdgeWorldSummary{off, off}},
		{"different-background", []CrossVenueEdgeWorldSummary{off, func() CrossVenueEdgeWorldSummary {
			row := on
			row.BackgroundIdentity = "changed-background"
			return row
		}()}},
		{"different-horizon", []CrossVenueEdgeWorldSummary{off, func() CrossVenueEdgeWorldSummary { row := on; row.HorizonNano++; return row }()}},
		{"different-quantity", []CrossVenueEdgeWorldSummary{off, func() CrossVenueEdgeWorldSummary { row := on; row.LotQty++; return row }()}},
		{"missing-episode-censor", []CrossVenueEdgeWorldSummary{off, func() CrossVenueEdgeWorldSummary { row := on; row.CensoredEpisodes = 2; return row }()}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := CompareCrossVenuePairedEdgeWorlds(test.worlds); err == nil {
				t.Fatal("incomplete or confounded paired worlds accepted")
			}
		})
	}
}
