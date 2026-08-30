package main

import (
	"reflect"
	"testing"
)

// sampleMetricSettings uses a distinct value for every field, so a constructor
// that reads the wrong one produces a visibly wrong option rather than a
// coincidentally equal default.
func sampleMetricSettings() metricSettings {
	return metricSettings{
		basePrecision:           11,
		quotePrecision:          22,
		requireExactReplay:      true,
		deliveryFeePolicy:       "zero",
		fundingIntervalSeconds:  28800,
		arbFeeBps:               3.5,
		arbStaleness:            2.5,
		base:                    "BASE-SYM",
		quote:                   "QUOTE-SYM",
		cross:                   "CROSS-SYM",
		crossPrecision:          33,
		crossVenueSymbol:        "XV-SYM",
		crossVenueMin:           4,
		crossVenuePositiveTimes: true,
		perpSignalSymbol:        "PERP-SYM",
		perpSignalVenues:        "north,central",
		postOnlyRoles:           "maker,taker",
		postOnlySymbols:         "A,B",
		hedgeSymbol:             "HEDGE-SYM",
		fundingIntervals:        map[string]int64{"ABC-PERP": 28800},
	}
}

// TestSharedOptionsReadTheirSettings pins each constructor to the settings it is
// supposed to consume. These option sets are the configuration contract between
// the single-metric path and the fused driver: both call these functions, so a
// constructor reading the wrong field would change what a metric measures in
// both paths at once, consistently and therefore invisibly.
func TestSharedOptionsReadTheirSettings(t *testing.T) {
	s := sampleMetricSettings()

	postOnly := postOnlyOptions(s)
	if !reflect.DeepEqual(postOnly.Roles, []string{"maker", "taker"}) {
		t.Errorf("post-only roles = %v", postOnly.Roles)
	}
	if !reflect.DeepEqual(postOnly.Symbols, []string{"A", "B"}) {
		t.Errorf("post-only symbols = %v", postOnly.Symbols)
	}

	if got := hedgingOptions(s); got.Symbol != "HEDGE-SYM" ||
		!reflect.DeepEqual(got.Roles, []string{"option_dealer", "vanna_volga_desk"}) {
		t.Errorf("hedging options = %+v", got)
	}

	if got := perpSignalOptions(s); got.Symbol != "PERP-SYM" ||
		!reflect.DeepEqual(got.RequiredVenues, []string{"north", "central"}) {
		t.Errorf("perp signal options = %+v", got)
	}

	if got := exposureOptions(s); !reflect.DeepEqual(got.Roles, []string{"option_dealer"}) {
		t.Errorf("exposure options = %+v", got)
	}

	if got := optionSurfaceOptions(s); got.QuotePrecision != 22 {
		t.Errorf("option surface quote precision = %d, want the quote precision 22", got.QuotePrecision)
	}

	if got := positionOptions(s); got.BasePrecision != 11 || !got.RequireExactReplay {
		t.Errorf("position options = %+v", got)
	}

	if got := settlementOptions(s); got.BasePrecision != 11 || !got.RequireExactReplay ||
		got.DeliveryFeePolicy != "zero" {
		t.Errorf("settlement options = %+v", got)
	}

	intervals := map[string]int64{"ABC-PERP": 28800}
	got := derivativeOptions(s, intervals)
	if got.BasePrecision != 11 || !got.RequireExactReplay ||
		got.ExpectedFundingIntervalSeconds != 28800 ||
		!reflect.DeepEqual(got.ExpectedFundingIntervals, intervals) {
		t.Errorf("derivative options = %+v", got)
	}

	arb := arbitrageOptions(s)
	if arb.TakerFeeBps != 3.5 || arb.StalenessNanos != int64(2.5*1e9) ||
		arb.BaseSymbol != "BASE-SYM" || arb.QuoteSymbol != "QUOTE-SYM" ||
		arb.CrossSymbol != "CROSS-SYM" || arb.CrossPrecision != 33 ||
		arb.CrossVenueSymbol != "BASE-SYM" || arb.PerpSymbol != "ABC-PERP" ||
		arb.SpotSymbol != "BASE-SYM" || arb.ParityUnderlying != "BASE-SYM" {
		t.Errorf("arbitrage options = %+v", arb)
	}

	xv := crossVenueOptions(s)
	if xv.Symbol != "XV-SYM" || xv.StalenessNanos != int64(2.5*1e9) ||
		xv.MinVenues != 4 || !xv.CapturePositiveObservationTimes {
		t.Errorf("cross-venue options = %+v", xv)
	}
}

// TestFusedDriverCoversTheRegisteredMetricSet requires the fused driver to know
// every metric the registered extraction computes through it. A metric added to
// the extraction script but not here would fall back to a separate process
// silently, which is a performance regression rather than a wrong answer — but
// one nobody would notice.
func TestFusedDriverCoversTheRegisteredMetricSet(t *testing.T) {
	registered := []string{
		"conservation", "positions", "fillpositions", "orderlifecycle", "lifecycle",
		"settlements", "expiryfills", "streamhash", "arbitrage", "crossvenue",
		"roleaudit", "derivatives", "liquidations", "marginchecks", "optionsurface",
		"exposure", "hedging", "postonly", "perpsignals",
	}
	available := fusedMetrics()
	for _, name := range registered {
		if _, ok := available[name]; !ok {
			t.Errorf("fused driver does not implement registered metric %q", name)
		}
	}
	for name := range available {
		found := false
		for _, want := range registered {
			if want == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("fused driver implements %q, which this test does not list as registered; "+
				"add it to the list or remove it from the driver", name)
		}
	}
}
