package multivenue

import "testing"

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
