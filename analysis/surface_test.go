package analysis

import "testing"

func TestOptionSurfaceRetainsPresentPricesOutsideBlack76Domain(t *testing.T) {
	const (
		optionSymbol = "ABC-3600-100-C"
		tradeTS      = int64(1e9)
	)
	tests := []struct {
		name              string
		index             int64
		premium           int64
		wantDomain        int
		wantNonInvertible int
	}{
		{
			name:       "signed index is present but outside Black-76 domain",
			index:      0,
			premium:    5,
			wantDomain: 1,
		},
		{
			name:              "zero premium is present but has no invertible time value",
			index:             100,
			premium:           0,
			wantNonInvertible: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := writeRun(t, Report{}, map[string][]string{
				"north/derivatives.jsonl": {
					`{"sim_ts":1000000000,"client_id":0,"event":"mark_price_update","data":{"venue_id":"north","symbol":"ABC-PERP","payload":{"timestamp":1000000000,"symbol":"ABC-PERP","index_price":` + itoa(test.index) + `}}}`,
					`{"sim_ts":1000000000,"client_id":1,"event":"Trade","data":{"venue_id":"north","symbol":"` + optionSymbol + `","payload":{"price":` + itoa(test.premium) + `,"qty":1}}}`,
				},
			})
			run, err := Open(dir)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			result, err := run.MeasureOptionSurface(SurfaceOptions{QuotePrecision: 1})
			if err != nil {
				t.Fatalf("measure: %v", err)
			}
			if result.Trades != 1 || result.Priced != 0 || result.Skipped != 1 {
				t.Fatalf("trade accounting = %+v", result)
			}
			if result.SkippedBlack76Domain != test.wantDomain || result.SkippedNonInvertiblePremium != test.wantNonInvertible {
				t.Fatalf("domain accounting = %+v", result)
			}
			if result.SkippedUnavailableIndex != 0 || result.SkippedInversionBounds != 0 {
				t.Fatalf("present inputs must not become unavailable/bounds skips: %+v", result)
			}
		})
	}
}
