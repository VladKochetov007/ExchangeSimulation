package instrument

import (
	"fmt"
	"math/big"
)

// FundingCalendar identifies scheduled instants independently of successful
// settlement. Cycle one is one full interval plus the phase after the epoch.
type FundingCalendar struct {
	EpochNano       int64
	IntervalSeconds int64
	PhaseSeconds    int64
}

func (calendar FundingCalendar) ScheduledAt(cycle int64) (int64, error) {
	if calendar.EpochNano < 0 || calendar.IntervalSeconds <= 0 || calendar.PhaseSeconds < 0 ||
		calendar.PhaseSeconds >= calendar.IntervalSeconds || cycle < 1 {
		return 0, fmt.Errorf("funding calendar: invalid epoch, interval, phase or cycle")
	}
	seconds := new(big.Int).Mul(big.NewInt(cycle), big.NewInt(calendar.IntervalSeconds))
	seconds.Add(seconds, big.NewInt(calendar.PhaseSeconds))
	nanoseconds := seconds.Mul(seconds, big.NewInt(1_000_000_000))
	nanoseconds.Add(nanoseconds, big.NewInt(calendar.EpochNano))
	if !nanoseconds.IsInt64() {
		return 0, fmt.Errorf("funding calendar: scheduled instant overflows int64")
	}
	return nanoseconds.Int64(), nil
}

func (calendar FundingCalendar) NextAtOrAfter(atNano int64) (int64, error) {
	first, err := calendar.ScheduledAt(1)
	if err != nil {
		return 0, err
	}
	if atNano <= first {
		return first, nil
	}
	intervalNano := new(big.Int).Mul(big.NewInt(calendar.IntervalSeconds), big.NewInt(1_000_000_000))
	delta := new(big.Int).Sub(big.NewInt(atNano), big.NewInt(first))
	cyclesAfterFirst, remainder := new(big.Int).QuoRem(delta, intervalNano, new(big.Int))
	if remainder.Sign() != 0 {
		cyclesAfterFirst.Add(cyclesAfterFirst, big.NewInt(1))
	}
	cyclesAfterFirst.Add(cyclesAfterFirst, big.NewInt(1))
	if !cyclesAfterFirst.IsInt64() {
		return 0, fmt.Errorf("funding calendar: cycle overflows int64")
	}
	return calendar.ScheduledAt(cyclesAfterFirst.Int64())
}

type FundingWindowSample struct {
	TimestampNano int64
	IndexPrice    int64
	MarkPrice     int64
}

// FundingSettlementTerms bind the rate and notional mark calculated from one
// immutable input window. The caller must attest the window's venue-local book
// source before using these terms to post cash.
type FundingSettlementTerms struct {
	Rate              QuantizedFundingRate
	NotionalMarkPrice int64
}

// QuantizedFundingRate keeps its unit scale attached to its signed value.
// The legacy FundingRate.Rate field means whole basis points; callers must
// not copy SignedUnits there without an explicit, exact conversion.
type QuantizedFundingRate struct {
	signedUnits int64
	unitsPerBp  int64
}

func NewQuantizedFundingRate(signedUnits, unitsPerBp int64) (QuantizedFundingRate, error) {
	if unitsPerBp <= 0 {
		return QuantizedFundingRate{}, fmt.Errorf("quantized funding rate: units per basis point must be positive")
	}
	return QuantizedFundingRate{signedUnits: signedUnits, unitsPerBp: unitsPerBp}, nil
}

func (rate QuantizedFundingRate) SignedUnits() int64 { return rate.signedUnits }
func (rate QuantizedFundingRate) UnitsPerBp() int64  { return rate.unitsPerBp }

func (rate QuantizedFundingRate) ExactIntegerBps() (int64, error) {
	if rate.unitsPerBp <= 0 || rate.signedUnits%rate.unitsPerBp != 0 {
		return 0, fmt.Errorf("quantized funding rate: not exactly representable in whole basis points")
	}
	return rate.signedUnits / rate.unitsPerBp, nil
}

// WindowedFundingRateContract computes a per-settlement rate in configured
// integer units per basis point. A caller must independently attest that each
// sample came from the intended venue's public book at its stated time.
type WindowedFundingRateContract struct {
	SampleCount              int
	SampleSpacingNano        int64
	BaseRateBps              int64
	PremiumWeightNumerator   int64
	PremiumWeightDenominator int64
	MaxAbsRateBps            int64
	NormalizationSeconds     int64
	RateUnitsPerBp           int64
}

func (contract WindowedFundingRateContract) Validate(intervalSeconds int64) error {
	if contract.SampleCount <= 0 || contract.SampleSpacingNano <= 0 ||
		contract.PremiumWeightDenominator <= 0 || contract.MaxAbsRateBps < 0 ||
		contract.NormalizationSeconds <= 0 || contract.RateUnitsPerBp <= 0 ||
		intervalSeconds <= 0 {
		return fmt.Errorf("windowed funding: invalid contract or interval")
	}
	return nil
}

func (contract WindowedFundingRateContract) RateAt(settlementNano, intervalSeconds int64, samples []FundingWindowSample) (QuantizedFundingRate, error) {
	if err := contract.Validate(intervalSeconds); err != nil || settlementNano < 0 || len(samples) != contract.SampleCount {
		return QuantizedFundingRate{}, fmt.Errorf("windowed funding: invalid contract, settlement time or sample count")
	}
	premiumSum := new(big.Rat)
	for index, sample := range samples {
		lookback := new(big.Int).Mul(big.NewInt(int64(contract.SampleCount-index)), big.NewInt(contract.SampleSpacingNano))
		expected := new(big.Int).Sub(big.NewInt(settlementNano), lookback)
		if !expected.IsInt64() || expected.Sign() < 0 || sample.TimestampNano != expected.Int64() ||
			sample.IndexPrice <= 0 || sample.MarkPrice <= 0 {
			return QuantizedFundingRate{}, fmt.Errorf("windowed funding: missing, mistimed or non-positive sample %d", index)
		}
		difference := new(big.Int).Sub(big.NewInt(sample.MarkPrice), big.NewInt(sample.IndexPrice))
		premium := new(big.Rat).SetFrac(difference.Mul(difference, big.NewInt(10_000)), big.NewInt(sample.IndexPrice))
		premiumSum.Add(premiumSum, premium)
	}
	meanPremium := premiumSum.Quo(premiumSum, new(big.Rat).SetInt64(int64(contract.SampleCount)))
	weight := new(big.Rat).SetFrac(big.NewInt(contract.PremiumWeightNumerator), big.NewInt(contract.PremiumWeightDenominator))
	perNormalization := new(big.Rat).Add(new(big.Rat).SetInt64(contract.BaseRateBps), meanPremium.Mul(meanPremium, weight))
	timeScale := new(big.Rat).SetFrac(big.NewInt(intervalSeconds), big.NewInt(contract.NormalizationSeconds))
	perSettlement := perNormalization.Mul(perNormalization, timeScale)
	cap := new(big.Rat).Mul(new(big.Rat).SetInt64(contract.MaxAbsRateBps), timeScale)
	negativeCap := new(big.Rat).Neg(cap)
	if perSettlement.Cmp(cap) > 0 {
		perSettlement = cap
	} else if perSettlement.Cmp(negativeCap) < 0 {
		perSettlement = negativeCap
	}
	quantized, err := roundRationalHalfEven(perSettlement, contract.RateUnitsPerBp)
	if err != nil {
		return QuantizedFundingRate{}, err
	}
	return NewQuantizedFundingRate(quantized, contract.RateUnitsPerBp)
}

// SettlementTermsAt derives the scheduled rate and the half-even-quantized
// arithmetic mean of the same complete past perp-mark window. It cannot attest
// book provenance or current risk-mark availability on its own.
func (contract WindowedFundingRateContract) SettlementTermsAt(settlementNano, intervalSeconds int64, samples []FundingWindowSample) (FundingSettlementTerms, error) {
	window := append([]FundingWindowSample(nil), samples...)
	rate, err := contract.RateAt(settlementNano, intervalSeconds, window)
	if err != nil {
		return FundingSettlementTerms{}, err
	}
	markSum := new(big.Int)
	for _, sample := range window {
		markSum.Add(markSum, big.NewInt(sample.MarkPrice))
	}
	markMean := new(big.Rat).SetFrac(markSum, big.NewInt(int64(len(window))))
	markPrice, err := roundRationalHalfEven(markMean, 1)
	if err != nil || markPrice <= 0 {
		return FundingSettlementTerms{}, fmt.Errorf("windowed funding: invalid settlement notional mark")
	}
	return FundingSettlementTerms{Rate: rate, NotionalMarkPrice: markPrice}, nil
}

func roundRationalHalfEven(value *big.Rat, unitsPerWhole int64) (int64, error) {
	scaled := new(big.Rat).Mul(value, new(big.Rat).SetInt64(unitsPerWhole))
	numerator := new(big.Int).Abs(scaled.Num())
	quotient, remainder := new(big.Int).QuoRem(numerator, scaled.Denom(), new(big.Int))
	twiceRemainder := new(big.Int).Mul(remainder, big.NewInt(2))
	comparison := twiceRemainder.Cmp(scaled.Denom())
	if comparison > 0 || comparison == 0 && quotient.Bit(0) == 1 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if scaled.Sign() < 0 {
		quotient.Neg(quotient)
	}
	if !quotient.IsInt64() {
		return 0, fmt.Errorf("windowed funding: quantized rate overflows int64")
	}
	return quotient.Int64(), nil
}
