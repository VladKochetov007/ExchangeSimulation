package exchange

import (
	"fmt"
	"math/big"

	"exchange_sim/instrument"
)

// ScaledFundingAccrual carries sub-quote-unit cash between scheduled payments.
// A caller owns one state per venue, client and perpetual symbol, including
// while flat and across signed-position reversals. Hedge-mode legs must be netted before
// Preview. A later cash posting can discharge an older fractional claim: term
// economics must attribute posted cash plus the change in remainder/denominator,
// not assign the full later cash posting to the current term.
// Preview is pure so a settlement batch can preflight every wallet and venue
// residual before committing any balance or accumulator mutation.
type ScaledFundingAccrual struct {
	basePrecision int64
	unitsPerBp    int64
	denominator   big.Int
	remainder     big.Int
}

type ScaledFundingAccrualSnapshot struct {
	BasePrecision      int64  `json:"base_precision"`
	RateUnitsPerBp     int64  `json:"rate_units_per_bp"`
	Denominator        string `json:"denominator"`
	RemainderNumerator string `json:"remainder_numerator"`
}

func NewScaledFundingAccrual(basePrecision, unitsPerBp int64) (ScaledFundingAccrual, error) {
	if basePrecision <= 0 || unitsPerBp <= 0 {
		return ScaledFundingAccrual{}, fmt.Errorf("scaled funding: base precision and units per bp must be positive")
	}
	state := ScaledFundingAccrual{basePrecision: basePrecision, unitsPerBp: unitsPerBp}
	state.denominator.Mul(big.NewInt(basePrecision), big.NewInt(10_000))
	state.denominator.Mul(&state.denominator, big.NewInt(unitsPerBp))
	return state, nil
}

func RestoreScaledFundingAccrual(basePrecision, unitsPerBp int64, snapshot ScaledFundingAccrualSnapshot) (ScaledFundingAccrual, error) {
	state, err := NewScaledFundingAccrual(basePrecision, unitsPerBp)
	if err != nil {
		return ScaledFundingAccrual{}, err
	}
	if snapshot.BasePrecision != basePrecision || snapshot.RateUnitsPerBp != unitsPerBp ||
		snapshot.Denominator != state.denominator.String() {
		return ScaledFundingAccrual{}, fmt.Errorf("scaled funding: persisted precision, scale or denominator changed")
	}
	remainder, ok := new(big.Int).SetString(snapshot.RemainderNumerator, 10)
	if !ok || remainder.String() != snapshot.RemainderNumerator || new(big.Int).Abs(remainder).Cmp(&state.denominator) >= 0 {
		return ScaledFundingAccrual{}, fmt.Errorf("scaled funding: invalid or noncanonical remainder numerator")
	}
	state.remainder.Set(remainder)
	return state, nil
}

func (state ScaledFundingAccrual) BasePrecision() int64       { return state.basePrecision }
func (state ScaledFundingAccrual) UnitsPerBp() int64          { return state.unitsPerBp }
func (state ScaledFundingAccrual) Denominator() string        { return state.denominator.String() }
func (state ScaledFundingAccrual) RemainderNumerator() string { return state.remainder.String() }

func (state ScaledFundingAccrual) Snapshot() ScaledFundingAccrualSnapshot {
	return ScaledFundingAccrualSnapshot{BasePrecision: state.basePrecision, RateUnitsPerBp: state.unitsPerBp,
		Denominator: state.denominator.String(), RemainderNumerator: state.remainder.String()}
}

// Preview returns the signed quote-asset-atom change to the client. Positive
// funding makes a long pay and a short receive. The exact numerator is kept
// intact until division by basePrecision*10000*unitsPerBp; truncation toward
// zero leaves a signed fractional remainder for the next event.
func (state ScaledFundingAccrual) Preview(positionSize, markPrice int64, rate instrument.QuantizedFundingRate) (int64, ScaledFundingAccrual, error) {
	if state.basePrecision <= 0 || state.unitsPerBp <= 0 || state.denominator.Sign() <= 0 ||
		markPrice <= 0 || rate.UnitsPerBp() != state.unitsPerBp {
		return 0, ScaledFundingAccrual{}, fmt.Errorf("scaled funding: invalid state, positive mark or rate unit scale")
	}
	numerator := new(big.Int).Mul(big.NewInt(positionSize), big.NewInt(markPrice))
	numerator.Mul(numerator, big.NewInt(rate.SignedUnits()))
	numerator.Neg(numerator)
	numerator.Add(numerator, &state.remainder)
	quoteDelta, remainder := new(big.Int).QuoRem(numerator, &state.denominator, new(big.Int))
	if !quoteDelta.IsInt64() {
		return 0, ScaledFundingAccrual{}, fmt.Errorf("scaled funding: client cash delta overflows int64")
	}
	next := ScaledFundingAccrual{basePrecision: state.basePrecision, unitsPerBp: state.unitsPerBp}
	next.denominator.Set(&state.denominator)
	next.remainder.Set(remainder)
	return quoteDelta.Int64(), next, nil
}
