package instrument

import (
	"math"
	"math/big"
	"testing"
	"time"
)

func TestFundingCalendarKeepsVenuePhaseAndAdvancesAfterMiss(t *testing.T) {
	for _, test := range []struct {
		name       string
		calendar   FundingCalendar
		first      time.Duration
		windowHits int
	}{
		{"N", FundingCalendar{IntervalSeconds: 5 * 60}, 5 * time.Minute, 24},
		{"S", FundingCalendar{IntervalSeconds: 8 * 60, PhaseSeconds: 60}, 9 * time.Minute, 15},
	} {
		t.Run(test.name, func(t *testing.T) {
			first, err := test.calendar.NextAtOrAfter(0)
			if err != nil || first != int64(test.first) {
				t.Fatalf("first = %d, %v; want %d", first, err, test.first)
			}
			if got, err := test.calendar.NextAtOrAfter(first); err != nil || got != first {
				t.Fatalf("inclusive boundary = %d, %v; want %d", got, err, first)
			}
			next, err := test.calendar.NextAtOrAfter(first + 1)
			if err != nil || next != first+test.calendar.IntervalSeconds*int64(time.Second) {
				t.Fatalf("post-miss boundary = %d, %v", next, err)
			}
			count := 0
			for cycle := int64(1); ; cycle++ {
				at, err := test.calendar.ScheduledAt(cycle)
				if err != nil {
					t.Fatal(err)
				}
				if at > int64(140*time.Minute) {
					break
				}
				if at > int64(20*time.Minute) {
					count++
				}
			}
			if count != test.windowHits {
				t.Fatalf("scheduled observation-window instants = %d, want %d", count, test.windowHits)
			}
		})
	}
}

func TestFundingCalendarRejectsInvalidAndOverflowingSchedule(t *testing.T) {
	for _, calendar := range []FundingCalendar{
		{IntervalSeconds: 0},
		{IntervalSeconds: 300, PhaseSeconds: -1},
		{IntervalSeconds: 300, PhaseSeconds: 300},
		{EpochNano: -1, IntervalSeconds: 300},
		{EpochNano: math.MaxInt64, IntervalSeconds: 300},
	} {
		if _, err := calendar.ScheduledAt(1); err == nil {
			t.Fatalf("accepted invalid calendar %#v", calendar)
		}
	}
	if _, err := (FundingCalendar{IntervalSeconds: 300}).ScheduledAt(0); err == nil {
		t.Fatal("accepted zero cycle")
	}
	secondCalendar := FundingCalendar{IntervalSeconds: 1}
	if last, err := secondCalendar.ScheduledAt(9_223_372_036); err != nil || last != 9_223_372_036_000_000_000 {
		t.Fatalf("last representable second = %d, %v", last, err)
	}
	if _, err := secondCalendar.ScheduledAt(9_223_372_037); err == nil {
		t.Fatal("accepted overflowing next cycle")
	}
	if _, err := secondCalendar.NextAtOrAfter(math.MaxInt64); err == nil {
		t.Fatal("accepted request after last representable instant")
	}
}

func TestWindowedFundingRateUsesCompletePastWindowAndScaledUnits(t *testing.T) {
	contract := WindowedFundingRateContract{
		SampleCount: 60, SampleSpacingNano: int64(time.Second),
		BaseRateBps: 1, PremiumWeightNumerator: 1, PremiumWeightDenominator: 1,
		MaxAbsRateBps: 75, NormalizationSeconds: 8 * 60 * 60, RateUnitsPerBp: 1_000_000,
	}
	settlement := int64(5 * time.Minute)
	samples := fundingTestSamples(settlement, 60, 50_000, 50_000)
	if rate, err := contract.RateAt(settlement, 300, samples); err != nil || rate.SignedUnits() != 10_417 || rate.UnitsPerBp() != 1_000_000 {
		t.Fatalf("N zero-premium rate = %+v, %v; want 10417 micro-bp", rate, err)
	}
	if rate, err := contract.RateAt(settlement, 480, samples); err != nil || rate.SignedUnits() != 16_667 {
		t.Fatalf("S zero-premium rate = %+v, %v; want 16667 micro-bp", rate, err)
	}
	for index := range samples {
		samples[index].MarkPrice = 50_001
	}
	if rate, err := contract.RateAt(settlement, 300, samples); err != nil || rate.SignedUnits() != 12_500 {
		t.Fatalf("uncapped premium rate = %+v, %v; want 12500 micro-bp", rate, err)
	}
	for index := range samples {
		samples[index].MarkPrice = 50_500
	}
	if rate, err := contract.RateAt(settlement, 300, samples); err != nil || rate.SignedUnits() != 781_250 {
		t.Fatalf("positive cap = %+v, %v; want 781250 micro-bp", rate, err)
	}
	for index := range samples {
		samples[index].MarkPrice = 49_500
	}
	if rate, err := contract.RateAt(settlement, 300, samples); err != nil || rate.SignedUnits() != -781_250 {
		t.Fatalf("negative cap = %+v, %v; want -781250 micro-bp", rate, err)
	}
}

func TestWindowedFundingRateFailsClosedOnAbsentOrFutureSamples(t *testing.T) {
	contract := WindowedFundingRateContract{SampleCount: 3, SampleSpacingNano: int64(time.Second),
		PremiumWeightNumerator: 1, PremiumWeightDenominator: 1, MaxAbsRateBps: 75,
		NormalizationSeconds: 28_800, RateUnitsPerBp: 1_000_000}
	settlement := int64(5 * time.Minute)
	valid := fundingTestSamples(settlement, 3, 50_000, 50_001)
	if _, err := contract.RateAt(settlement, 300, valid); err != nil {
		t.Fatal(err)
	}
	missing := append([]FundingWindowSample(nil), valid[:2]...)
	if _, err := contract.RateAt(settlement, 300, missing); err == nil {
		t.Fatal("accepted incomplete sample window")
	}
	for _, mutate := range []func([]FundingWindowSample){
		func(samples []FundingWindowSample) { samples[0].TimestampNano++ },
		func(samples []FundingWindowSample) { samples[2].TimestampNano = settlement },
		func(samples []FundingWindowSample) { samples[1].IndexPrice = 0 },
		func(samples []FundingWindowSample) { samples[1].MarkPrice = -1 },
	} {
		samples := append([]FundingWindowSample(nil), valid...)
		mutate(samples)
		if _, err := contract.RateAt(settlement, 300, samples); err == nil {
			t.Fatalf("accepted malformed window %#v", samples)
		}
	}
}

func TestWindowedFundingRateNegativeAndMixedPremium(t *testing.T) {
	contract := WindowedFundingRateContract{SampleCount: 2, SampleSpacingNano: int64(time.Second),
		BaseRateBps: 1, PremiumWeightNumerator: 1, PremiumWeightDenominator: 1,
		MaxAbsRateBps: 75, NormalizationSeconds: 28_800, RateUnitsPerBp: 1_000_000}
	settlement := int64(5 * time.Minute)
	samples := fundingTestSamples(settlement, 2, 50_000, 49_994)
	if rate, err := contract.RateAt(settlement, 300, samples); err != nil || rate.SignedUnits() != -2_083 {
		t.Fatalf("uncapped negative rate = %+v, %v; want -2083 micro-bp", rate, err)
	}
	samples[0].MarkPrice = 50_002
	samples[1].MarkPrice = 49_998
	if rate, err := contract.RateAt(settlement, 300, samples); err != nil || rate.SignedUnits() != 10_417 {
		t.Fatalf("mixed-sign premium mean = %+v, %v; want 10417 micro-bp", rate, err)
	}
}

func TestWindowedFundingRateTiesAndOverflowAtPublicBoundary(t *testing.T) {
	contract := WindowedFundingRateContract{SampleCount: 1, SampleSpacingNano: int64(time.Second),
		PremiumWeightDenominator: 1, MaxAbsRateBps: 100,
		NormalizationSeconds: 2, RateUnitsPerBp: 1}
	settlement := int64(5 * time.Minute)
	samples := fundingTestSamples(settlement, 1, 50_000, 50_000)
	for _, test := range []struct{ base, want int64 }{{1, 0}, {3, 2}, {-1, 0}, {-3, -2}} {
		contract.BaseRateBps = test.base
		rate, err := contract.RateAt(settlement, 1, samples)
		if err != nil || rate.SignedUnits() != test.want {
			t.Fatalf("base %d half-even result = %+v, %v; want %d", test.base, rate, err, test.want)
		}
	}
	contract.BaseRateBps = math.MaxInt64
	contract.MaxAbsRateBps = math.MaxInt64
	contract.NormalizationSeconds = 1
	contract.RateUnitsPerBp = 2
	if _, err := contract.RateAt(settlement, 2, samples); err == nil {
		t.Fatal("accepted quantized-rate overflow")
	}
}

func TestQuantizedFundingRateRequiresExactLegacyConversion(t *testing.T) {
	rate, err := NewQuantizedFundingRate(10_417, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rate.ExactIntegerBps(); err == nil {
		t.Fatal("fractional basis point silently converted to legacy integer rate")
	}
	exact, err := NewQuantizedFundingRate(-2_000_000, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if bps, err := exact.ExactIntegerBps(); err != nil || bps != -2 {
		t.Fatalf("exact legacy conversion = %d, %v; want -2", bps, err)
	}
	if _, err := NewQuantizedFundingRate(1, 0); err == nil {
		t.Fatal("accepted zero rate scale")
	}
	if _, err := (QuantizedFundingRate{}).ExactIntegerBps(); err == nil {
		t.Fatal("accepted uninitialized rate scale")
	}
}

func TestFundingRateQuantizationUsesSignedHalfEven(t *testing.T) {
	for _, test := range []struct {
		numerator, denominator, want int64
	}{
		{1, 2, 0}, {3, 2, 2}, {5, 2, 2}, {7, 2, 4},
		{-1, 2, 0}, {-3, 2, -2}, {-5, 2, -2}, {-7, 2, -4},
	} {
		rate := new(big.Rat).SetFrac64(test.numerator, test.denominator)
		got, err := roundRationalHalfEven(rate, 1)
		if err != nil || got != test.want {
			t.Fatalf("%d/%d -> %d, %v; want %d", test.numerator, test.denominator, got, err, test.want)
		}
	}
}

func TestWindowedFundingSettlementTermsUseSameCompleteWindow(t *testing.T) {
	contract := WindowedFundingRateContract{SampleCount: 2, SampleSpacingNano: int64(time.Second),
		BaseRateBps: 1, PremiumWeightNumerator: 1, PremiumWeightDenominator: 1,
		MaxAbsRateBps: 75, NormalizationSeconds: 28_800, RateUnitsPerBp: 1_000_000}
	settlement := int64(5 * time.Minute)
	samples := fundingTestSamples(settlement, 2, 50_000, 50_000)
	samples[1].MarkPrice = 50_001
	terms, err := contract.SettlementTermsAt(settlement, 300, samples)
	if err != nil || terms.NotionalMarkPrice != 50_000 || terms.Rate.SignedUnits() != 11_458 {
		t.Fatalf("even half-atom tie terms = %+v, %v", terms, err)
	}
	samples[0].MarkPrice = 50_001
	samples[1].MarkPrice = 50_002
	terms, err = contract.SettlementTermsAt(settlement, 300, samples)
	if err != nil || terms.NotionalMarkPrice != 50_002 || terms.Rate.SignedUnits() != 13_542 {
		t.Fatalf("odd half-atom tie terms = %+v, %v", terms, err)
	}
	samples[1].TimestampNano = settlement
	if _, err := contract.SettlementTermsAt(settlement, 300, samples); err == nil {
		t.Fatal("accepted present-time sample for settlement mark")
	}
}

func TestWindowedFundingSettlementTermsRejectIncompleteAndOverflowingRates(t *testing.T) {
	contract := WindowedFundingRateContract{SampleCount: 2, SampleSpacingNano: int64(time.Second),
		PremiumWeightDenominator: 1, MaxAbsRateBps: math.MaxInt64,
		NormalizationSeconds: 1, RateUnitsPerBp: 2}
	settlement := int64(5 * time.Minute)
	samples := fundingTestSamples(settlement, 2, math.MaxInt64, math.MaxInt64)
	if terms, err := contract.SettlementTermsAt(settlement, 1, samples); err != nil || terms.NotionalMarkPrice != math.MaxInt64 || terms.Rate.SignedUnits() != 0 {
		t.Fatalf("large exact mean = %+v, %v", terms, err)
	}
	if _, err := contract.SettlementTermsAt(settlement, 1, samples[:1]); err == nil {
		t.Fatal("accepted missing sample for settlement mark")
	}
	contract.BaseRateBps = math.MaxInt64
	if _, err := contract.SettlementTermsAt(settlement, 2, samples); err == nil {
		t.Fatal("accepted overflowing rate with otherwise valid mark")
	}
}

func fundingTestSamples(settlement int64, count int, indexPrice, markPrice int64) []FundingWindowSample {
	samples := make([]FundingWindowSample, count)
	for sample := range samples {
		samples[sample] = FundingWindowSample{TimestampNano: settlement - int64(count-sample)*int64(time.Second),
			IndexPrice: indexPrice, MarkPrice: markPrice}
	}
	return samples
}
