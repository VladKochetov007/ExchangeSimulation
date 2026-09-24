package crossvenue

import (
	"testing"

	"exchange_sim/analysis"
)

func TestME005ConservationRejectsBrokenLedgerAndResiduals(t *testing.T) {
	valid := func() *analysis.Conservation {
		return &analysis.Conservation{
			Deltas:          analysis.DeltaConsistency{Checked: 2, ChainChecked: 2},
			Identities:      []analysis.ConservationIdentity{{Asset: "USD"}},
			VenueIdentities: []analysis.VenueConservationIdentity{{VenueID: "north", ConservationIdentity: analysis.ConservationIdentity{Asset: "USD"}}},
		}
	}
	if err := validateConservation(valid()); err != nil {
		t.Fatalf("valid ledger rejected: %v", err)
	}
	cases := []struct {
		name   string
		mutate func(*analysis.Conservation)
	}{
		{"missing-checks", func(result *analysis.Conservation) { result.Deltas.Checked = 0 }},
		{"broken-chain", func(result *analysis.Conservation) { result.Deltas.ChainBroken = 1 }},
		{"missing-fee", func(result *analysis.Conservation) { result.Deltas.TradingFeeMismatches = 1 }},
		{"bad-venue-sequence", func(result *analysis.Conservation) { result.Deltas.VenueSequenceMismatches = 1 }},
		{"asset-residual", func(result *analysis.Conservation) { result.Identities[0].Residual = 1 }},
		{"venue-residual", func(result *analysis.Conservation) { result.VenueIdentities[0].Residual = -1 }},
		{"funding-residual", func(result *analysis.Conservation) {
			result.FundingInstants = []analysis.InstantResidual{{VenueID: "north", Net: 1}}
		}},
		{"invalid-rounding", func(result *analysis.Conservation) { result.PositionRounding.Events = 1 }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result := valid()
			test.mutate(result)
			if err := validateConservation(result); err == nil {
				t.Fatal("broken conservation accepted")
			}
		})
	}
}
