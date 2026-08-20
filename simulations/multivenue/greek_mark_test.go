package multivenue

import (
	"context"
	"testing"
	"time"
)

// The exchange marks an option with the instrument's own volatility. Once
// dealers hold their own models the two views diverge, and a report about the
// dealer's book labelled with the venue's number invites reading a dealer's
// opinion off something it had no part in.
func TestVenueRiskMarksTheDealerBookWithTheDealersVolatility(t *testing.T) {
	cfg := Config{
		LogDir: t.TempDir(), LogMode: "none", Seed: 101,
		OptionIV:          0.8,
		OptionDealerCount: 2,
		OptionDealerVol: OptionDealerVolConfig{
			Model: "realized", HalfLifeSeconds: []float64{300}, Premiums: []float64{1}, Floor: 0.05, Ceiling: 3,
		},
	}
	sim, err := NewSim(10*time.Minute, cfg)
	if err != nil {
		t.Fatalf("new sim: %v", err)
	}
	defer sim.Close()
	if err := sim.Run(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}
	venue := sim.Venues[0]
	dealerVol := venue.OptionDealer.PricingVolatility(0, 0, true)
	if dealerVol <= 0 || dealerVol == cfg.OptionIV {
		t.Fatalf("the dealer's estimate never moved off the configured level: %v", dealerVol)
	}
	risk, err := captureVenueRisk(venue, "probe")
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if risk.GreekProfile.Contracts == 0 {
		t.Skip("the dealer held no options at the end of this run")
	}
	if risk.GreekProfile.ImpliedVolatility == cfg.OptionIV {
		t.Errorf("the dealer's book was marked at the venue's %v rather than its own %v",
			cfg.OptionIV, dealerVol)
	}
}
