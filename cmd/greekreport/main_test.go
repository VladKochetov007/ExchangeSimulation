package main

import (
	"math"
	"testing"

	"exchange_sim/simulations/derivsim"
)

func TestSummarizeGreekReportsSeparatesRollingGenerations(t *testing.T) {
	report := derivsim.GreekReport{PositionProfiles: []derivsim.GreekPosition{
		{Timestamp: 10, ListedNano: 1, ExpiryNano: 101, TimeToExpiryNano: 91, Symbol: "C1", Delta: 1, Gamma: 2, Vega: 3},
		{Timestamp: 10, ListedNano: 1, ExpiryNano: 101, TimeToExpiryNano: 91, Symbol: "P1", Delta: -0.5, Gamma: -1, Vega: -4},
		{Timestamp: 20, ListedNano: 1, ExpiryNano: 101, TimeToExpiryNano: 81, Symbol: "C1", Delta: 2, Gamma: 4, Vega: 6},
		{Timestamp: 20, ListedNano: 11, ExpiryNano: 111, TimeToExpiryNano: 91, Symbol: "C2", Delta: -3, Gamma: 5, Vega: -7},
	}}
	got, err := SummarizeGreekReports(map[string]derivsim.GreekReport{"north": report})
	if err != nil {
		t.Fatalf("SummarizeGreekReports: %v", err)
	}
	if len(got.Tenors) != 2 {
		t.Fatalf("tenors = %d, want 2", len(got.Tenors))
	}
	first := got.Tenors[0]
	if first.ListingTenorNano != 100 || first.Samples != 2 || first.Symbols != 2 {
		t.Fatalf("first generation metadata = %+v", first)
	}
	if first.LastPreExpiry.Timestamp != 20 || first.LastPreExpiry.OptionDelta != 2 || first.LastPreExpiry.Gamma != 4 || first.LastPreExpiry.Vega != 6 {
		t.Fatalf("last pre-expiry aggregate = %+v", first.LastPreExpiry)
	}
	if math.Abs(first.MeanAbsDelta-1.25) > 1e-12 || first.MaxAbsVega != 6 {
		t.Fatalf("unexpected moments: %+v", first)
	}
	if got.Tenors[1].ListedNano != 11 || got.Tenors[1].LastPreExpiry.OptionDelta != -3 {
		t.Fatalf("rolling generation merged incorrectly: %+v", got.Tenors[1])
	}
}

func TestSummarizeGreekReportsRejectsUnknownListingGeneration(t *testing.T) {
	_, err := SummarizeGreekReports(map[string]derivsim.GreekReport{"north": {PositionProfiles: []derivsim.GreekPosition{{Symbol: "C", ExpiryNano: 5}}}})
	if err == nil {
		t.Fatal("expected invalid listing generation rejection")
	}
}

func TestSummarizeRemainingMaturitiesUsesLiveTimeToExpiry(t *testing.T) {
	reports := map[string]derivsim.GreekReport{"north": {PositionProfiles: []derivsim.GreekPosition{
		{Timestamp: 10, Symbol: "aged-long", TimeToExpiryNano: 5, Delta: 1, Gamma: 2, Vega: 3},
		{Timestamp: 10, Symbol: "fresh-long", TimeToExpiryNano: 30, Delta: 4, Gamma: 5, Vega: 6},
		{Timestamp: 20, Symbol: "aged-long", TimeToExpiryNano: 4, Delta: -2, Gamma: -3, Vega: -4},
	}}}
	got, err := SummarizeRemainingMaturities(reports, []MaturityBand{{Name: "short", MinNanos: 1, MaxNanos: 6}, {Name: "long", MinNanos: 24}})
	if err != nil {
		t.Fatalf("SummarizeRemainingMaturities: %v", err)
	}
	if len(got) != 2 || got[0].Band.Name != "short" || got[0].Samples != 2 || got[0].Last.OptionDelta != -2 || got[0].Last.MinTimeToExpiryNano != 4 {
		t.Fatalf("short live-maturity bucket = %+v", got)
	}
	if got[1].Band.Name != "long" || got[1].Samples != 1 || got[1].First.OptionDelta != 4 {
		t.Fatalf("long live-maturity bucket = %+v", got)
	}
}
