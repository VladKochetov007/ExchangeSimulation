package exchange

import (
	"math"
	"math/big"
	"testing"

	"exchange_sim/instrument"
)

func TestScaledFundingAccrualCarriesSubUnitCashAcrossPayments(t *testing.T) {
	state, err := NewScaledFundingAccrual(100_000_000, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	rate := scaledFundingTestRate(t, 10_417, 1_000_000)
	firstDelta, first, err := state.Preview(10_000, 5_000_000_000, rate)
	if err != nil || firstDelta != 0 || first.RemainderNumerator() != "-520850000000000000" {
		t.Fatalf("first subunit payment = %d, %s, %v", firstDelta, first.RemainderNumerator(), err)
	}
	if state.RemainderNumerator() != "0" {
		t.Fatalf("preview mutated input remainder: %s", state.RemainderNumerator())
	}
	secondDelta, second, err := first.Preview(10_000, 5_000_000_000, rate)
	if err != nil || secondDelta != -1 || second.RemainderNumerator() != "-41700000000000000" {
		t.Fatalf("second subunit payment = %d, %s, %v", secondDelta, second.RemainderNumerator(), err)
	}
	restored, err := RestoreScaledFundingAccrual(100_000_000, 1_000_000, first.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	replayedDelta, replayed, err := restored.Preview(10_000, 5_000_000_000, rate)
	if err != nil || replayedDelta != secondDelta || replayed.RemainderNumerator() != second.RemainderNumerator() {
		t.Fatalf("restored replay = %d, %s, %v", replayedDelta, replayed.RemainderNumerator(), err)
	}
}

func TestScaledFundingAccrualPreservesSignsAndVenueResidual(t *testing.T) {
	longState, err := NewScaledFundingAccrual(100_000_000, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	shortState, err := NewScaledFundingAccrual(100_000_000, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	rate := scaledFundingTestRate(t, 10_417, 1_000_000)
	totalLongCashDelta := int64(0)
	for payment := 0; payment < 96; payment++ {
		longDelta, nextLong, err := longState.Preview(10_000_000, 5_000_000_000, rate)
		if err != nil {
			t.Fatal(err)
		}
		shortDelta, nextShort, err := shortState.Preview(-10_000_000, 5_000_000_000, rate)
		if err != nil {
			t.Fatal(err)
		}
		if longDelta+shortDelta != 0 {
			t.Fatalf("payment %d long/short cash transfer not zero-sum: %d, %d", payment, longDelta, shortDelta)
		}
		totalLongCashDelta += longDelta
		longState, shortState = nextLong, nextShort
	}
	if totalLongCashDelta != -50_001 {
		t.Fatalf("96 exact rounded cash postings = %d, want -50001 quote atoms", totalLongCashDelta)
	}
	if longState.RemainderNumerator() != "-600000000000000000" ||
		shortState.RemainderNumerator() != "600000000000000000" {
		t.Fatalf("opposite remainders = %s, %s", longState.RemainderNumerator(), shortState.RemainderNumerator())
	}
	hedgeAccount, err := NewScaledFundingAccrual(100_000_000, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	hedgeNetPosition := int64(10_000_000 - 6_000_000)
	hedgeDelta, hedgeNext, err := hedgeAccount.Preview(hedgeNetPosition, 5_000_000_000, rate)
	if err != nil || hedgeDelta != -208 || hedgeNext.RemainderNumerator() != "-340000000000000000" {
		t.Fatalf("same-account hedge net transfer = %d, %s, %v", hedgeDelta, hedgeNext.RemainderNumerator(), err)
	}
	longOnly, err := NewScaledFundingAccrual(100_000_000, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	clientDelta, _, err := longOnly.Preview(10_000_000, 5_000_000_000, rate)
	if err != nil || clientDelta != -520 || -clientDelta != 520 {
		t.Fatalf("unmatched long client/venue flow = %d/%d, %v", clientDelta, -clientDelta, err)
	}
}

func TestScaledFundingAccrualZeroPositionAndRateReversal(t *testing.T) {
	state, err := NewScaledFundingAccrual(100_000_000, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	positive := scaledFundingTestRate(t, 10_417, 1_000_000)
	negative := scaledFundingTestRate(t, -10_417, 1_000_000)
	firstDelta, held, err := state.Preview(10_000, 5_000_000_000, positive)
	if err != nil || firstDelta != 0 {
		t.Fatalf("initial small accrual = %d, %v", firstDelta, err)
	}
	zeroDelta, unchanged, err := held.Preview(0, 5_000_000_000, positive)
	if err != nil || zeroDelta != 0 || unchanged.RemainderNumerator() != held.RemainderNumerator() {
		t.Fatalf("closed position changed accrual = %d, %s, %v", zeroDelta, unchanged.RemainderNumerator(), err)
	}
	secondDelta, afterReopen, err := unchanged.Preview(10_000, 5_000_000_000, positive)
	if err != nil || secondDelta != -1 {
		t.Fatalf("reopened small position = %d, %v; want -1", secondDelta, err)
	}
	denominator, ok := new(big.Int).SetString(held.Denominator(), 10)
	if !ok {
		t.Fatal("invalid test denominator")
	}
	beforeRemainder, ok := new(big.Int).SetString(held.RemainderNumerator(), 10)
	if !ok {
		t.Fatal("invalid test opening remainder")
	}
	afterRemainder, ok := new(big.Int).SetString(afterReopen.RemainderNumerator(), 10)
	if !ok {
		t.Fatal("invalid test closing remainder")
	}
	termAccrualNumerator := new(big.Int).Mul(big.NewInt(secondDelta), denominator)
	termAccrualNumerator.Add(termAccrualNumerator, new(big.Int).Sub(afterRemainder, beforeRemainder))
	if termAccrualNumerator.String() != "-520850000000000000" {
		t.Fatalf("reopened term accrual numerator = %s; want only its own -0.52085 atom", termAccrualNumerator)
	}
	shortDelta, reversedExposure, err := held.Preview(-10_000, 5_000_000_000, positive)
	if err != nil || shortDelta != 0 || reversedExposure.RemainderNumerator() != "0" {
		t.Fatalf("same-account signed reversal did not net fractional funding = %d, %s, %v",
			shortDelta, reversedExposure.RemainderNumerator(), err)
	}
	longDelta, longAccrual, err := state.Preview(10_000_000, 5_000_000_000, positive)
	if err != nil || longDelta != -520 {
		t.Fatalf("positive rate long payment = %d, %v", longDelta, err)
	}
	reverseDelta, reversed, err := longAccrual.Preview(10_000_000, 5_000_000_000, negative)
	if err != nil || reverseDelta != 520 || reversed.RemainderNumerator() != "0" {
		t.Fatalf("reverse rate payment = %d, %s, %v", reverseDelta, reversed.RemainderNumerator(), err)
	}
}

func TestScaledFundingAccrualRejectsMalformedStateAndPreviewsAtomically(t *testing.T) {
	for _, inputs := range [][2]int64{{0, 1}, {1, 0}, {-1, 1}, {1, -1}} {
		if _, err := NewScaledFundingAccrual(inputs[0], inputs[1]); err == nil {
			t.Fatalf("accepted invalid precision/scale %v", inputs)
		}
	}
	state, err := NewScaledFundingAccrual(100_000_000, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	for _, remainder := range []string{"", "-0", "+1", "01", "1e3", "1000000000000000000", "-1000000000000000000"} {
		snapshot := state.Snapshot()
		snapshot.RemainderNumerator = remainder
		if _, err := RestoreScaledFundingAccrual(100_000_000, 1_000_000, snapshot); err == nil {
			t.Fatalf("accepted malformed remainder %q", remainder)
		}
	}
	for _, mutate := range []func(*ScaledFundingAccrualSnapshot){
		func(snapshot *ScaledFundingAccrualSnapshot) { snapshot.BasePrecision++ },
		func(snapshot *ScaledFundingAccrualSnapshot) { snapshot.RateUnitsPerBp++ },
		func(snapshot *ScaledFundingAccrualSnapshot) { snapshot.Denominator = "1000000000000000001" },
	} {
		snapshot := state.Snapshot()
		mutate(&snapshot)
		if _, err := RestoreScaledFundingAccrual(100_000_000, 1_000_000, snapshot); err == nil {
			t.Fatalf("accepted changed persisted scale %#v", snapshot)
		}
	}
	validRate := scaledFundingTestRate(t, 10_417, 1_000_000)
	mismatchedScale := scaledFundingTestRate(t, 1, 1)
	for _, input := range []struct {
		mark int64
		rate instrument.QuantizedFundingRate
	}{
		{0, validRate}, {-1, validRate}, {5_000_000_000, mismatchedScale},
	} {
		if _, _, err := state.Preview(10_000, input.mark, input.rate); err == nil {
			t.Fatalf("accepted invalid mark/rate input %+v", input)
		}
	}
	hugeRate := scaledFundingTestRate(t, math.MaxInt64, 1_000_000)
	if _, _, err := state.Preview(math.MaxInt64, math.MaxInt64, hugeRate); err == nil {
		t.Fatal("accepted overflowing cash payment")
	}
	if state.RemainderNumerator() != "0" || state.Denominator() != "1000000000000000000" {
		t.Fatalf("failed preview mutated state: remainder=%s denominator=%s", state.RemainderNumerator(), state.Denominator())
	}
}

func scaledFundingTestRate(t *testing.T, signedUnits, unitsPerBp int64) instrument.QuantizedFundingRate {
	t.Helper()
	rate, err := instrument.NewQuantizedFundingRate(signedUnits, unitsPerBp)
	if err != nil {
		t.Fatal(err)
	}
	return rate
}
