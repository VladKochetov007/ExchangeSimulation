package multivenue

import (
	"testing"
	"time"
)

func TestVenueSizedQtyRejectsBelowMinimum(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		intended, available, minimum int64
		wantQty                      int64
		wantOK                       bool
	}{
		{"clamped below minimum is refused", 1000, 40, 100, 0, false},
		{"clamped above minimum passes", 1000, 400, 100, 400, true},
		{"exactly the minimum passes", 100, 0, 100, 100, true},
		{"one below the minimum is refused", 99, 0, 100, 0, false},
		{"no minimum configured allows any clamp", 1000, 1, 0, 1, true},
		{"no visible size leaves the intent alone", 1000, 0, 100, 1000, true},
		{"zero intent is refused", 0, 500, 100, 0, false},
		{"negative intent is refused", -5, 500, 100, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			qty, ok := venueSizedQty(tc.intended, tc.available, tc.minimum)
			if qty != tc.wantQty || ok != tc.wantOK {
				t.Fatalf("venueSizedQty(%d,%d,%d) = (%d,%v), want (%d,%v)",
					tc.intended, tc.available, tc.minimum, qty, ok, tc.wantQty, tc.wantOK)
			}
		})
	}
}

// The maker's minimum half-spread was hardcoded to one tick. It is the quantity
// the inventory skew competes against — when skew separates two makers'
// reservations by more than they quote, their quotes cross — so it has to be
// configurable to be calibrated at all.
func TestMakerMinHalfSpreadTicksIsConfigurable(t *testing.T) {
	cfg := Config{LogDir: t.TempDir()}
	if err := cfg.normalize(); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if cfg.MakerMinHalfSpreadTicks != 1 {
		t.Fatalf("default half-spread floor = %d, want 1", cfg.MakerMinHalfSpreadTicks)
	}
	cfg.MakerMinHalfSpreadTicks = 12
	if err := cfg.normalize(); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if cfg.MakerMinHalfSpreadTicks != 12 {
		t.Fatalf("configured half-spread floor overwritten: got %d, want 12", cfg.MakerMinHalfSpreadTicks)
	}
}

// A triangular loop crosses three books, so its round trip costs three taker
// fees. A desk that fires on a configured edge without adding that cost trades
// the same way at any fee: measured over eight hours it earned 3527 USD at 2
// bps, lost 59 at 5 and lost 6522 at 10, while the cross-rate deviation stayed
// at a 17.5 bps maximum in every case.
func TestTriangleRequiredEdgeIncludesEveryLegsFee(t *testing.T) {
	for _, tc := range []struct {
		name string
		edge int64
		fee  int64
		legs int64
		want float64
	}{
		{"no fee leaves the configured edge", 5, 0, 0, 5},
		{"three legs by default", 5, 10, 0, 35},
		{"explicit leg count", 5, 10, 2, 25},
		{"fee alone", 0, 4, 3, 12},
	} {
		t.Run(tc.name, func(t *testing.T) {
			desk := &TriangleArbTaker{cfg: TriangleArbConfig{EdgeBps: tc.edge, TakerFeeBps: tc.fee, Legs: tc.legs}}
			if got := desk.requiredEdgeBps(); got != tc.want {
				t.Fatalf("requiredEdgeBps() = %v, want %v", got, tc.want)
			}
		})
	}
}

// Uninformed flow arriving on a fixed clock with an independent side produces a
// market with none of the regularities a traded market shows. The scenario must
// pass the excitation and herding settings through, or the mechanism that
// creates them cannot be reached from a configuration at all.
func TestNoiseFlowExcitationReachesTheTaker(t *testing.T) {
	cfg := Config{LogDir: t.TempDir()}
	if err := cfg.normalize(); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if cfg.NoiseExciteAlpha != 0 || cfg.NoiseImbalanceCoupling != 0 || cfg.NoiseExciteBetaPerSec != 0 {
		t.Fatalf("defaults must leave flow unexcited: %+v", cfg)
	}
	cfg.NoiseExciteAlpha = 0.4
	cfg.NoiseExciteBetaPerSec = 0.2
	cfg.NoiseImbalanceCoupling = 0.5
	if err := cfg.normalize(); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if cfg.NoiseExciteAlpha != 0.4 || cfg.NoiseExciteBetaPerSec != 0.2 || cfg.NoiseImbalanceCoupling != 0.5 {
		t.Fatalf("configured excitation overwritten: %+v", cfg)
	}
}

// Quote size and the inventory limit are risk quantities, so they have to be
// denominated in value rather than in base units. Passed through raw they gave
// each book the same number of units of base assets whose prices differ by a
// factor of sixteen: measured median depth was 848,935 on ABC/USD against
// 29,840 on CDF/USD, and the thin book peaked 763% above its opening price
// while ABC/USD fell 18%.
func TestMakerSizeIsDenominatedInValueAcrossBooks(t *testing.T) {
	cfg := Config{LogDir: t.TempDir(), CrossAssetSpotGraph: true}
	if err := cfg.normalize(); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	sim, err := NewSim(time.Minute, cfg)
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	byBook := map[string]int64{}
	for _, venue := range sim.Venues {
		for _, maker := range venue.SpotMakers {
			byBook[maker.cfg.Symbol] = maker.cfg.QuoteQty
		}
	}
	abc, okABC := byBook["ABC/USD"]
	cdf, okCDF := byBook["CDF/USD"]
	if !okABC || !okCDF {
		t.Fatalf("expected both books to have makers, got %v", byBook)
	}
	// CDF trades near 3000 USD and ABC near 50000, so an equal-value quote is
	// about sixteen times more CDF units.
	ratio := float64(cdf) / float64(abc)
	if ratio < 12 || ratio > 22 {
		t.Fatalf("CDF quote is %d against ABC %d, a ratio of %.1f; want about 16.7 for equal value", cdf, abc, ratio)
	}
	if cross, ok := byBook["ABC/CDF"]; ok && cross != abc {
		t.Errorf("ABC/CDF quote is %d against ABC/USD %d; its base is also ABC so it should match", cross, abc)
	}
}
