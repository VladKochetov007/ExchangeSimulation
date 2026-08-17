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
