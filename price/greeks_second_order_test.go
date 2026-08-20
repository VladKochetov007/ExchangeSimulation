package price

import (
	"math"
	"testing"
)

const (
	greekForward = int64(50_000 * 100_000_000)
	greekYears   = 0.25
	greekVol     = 0.6
)

// Vanna and volga have to agree with the derivatives they claim to be, or a
// desk hedging on them is hedging a number that describes nothing.
func TestVannaMatchesTheDerivativeOfVegaInTheForward(t *testing.T) {
	strike := int64(55_000 * 100_000_000)
	const bump = int64(100_000_000)
	up := Black76Vega(greekForward+bump, strike, greekVol, greekYears)
	down := Black76Vega(greekForward-bump, strike, greekVol, greekYears)
	numeric := (up - down) / (2 * float64(bump))
	analytic := Black76Vanna(greekForward, strike, greekVol, greekYears)
	if math.Abs(numeric-analytic) > math.Abs(numeric)*0.01+1e-12 {
		t.Errorf("vanna = %v, numeric derivative of vega = %v", analytic, numeric)
	}
}

func TestVolgaMatchesTheDerivativeOfVegaInVolatility(t *testing.T) {
	strike := int64(60_000 * 100_000_000)
	const bump = 0.001
	up := Black76Vega(greekForward, strike, greekVol+bump, greekYears)
	down := Black76Vega(greekForward, strike, greekVol-bump, greekYears)
	numeric := (up - down) / (2 * bump)
	analytic := Black76Volga(greekForward, strike, greekVol, greekYears)
	if math.Abs(numeric-analytic) > math.Abs(numeric)*0.01+1e-6 {
		t.Errorf("volga = %v, numeric derivative of vega = %v", analytic, numeric)
	}
}

// Volga is near zero at the money and large in the wings. A desk reading it
// the other way round would hedge the wrong contracts.
func TestVolgaIsSmallAtTheMoneyAndLargeInTheWings(t *testing.T) {
	atm := math.Abs(Black76Volga(greekForward, greekForward, greekVol, greekYears))
	wing := math.Abs(Black76Volga(greekForward, greekForward*2, greekVol, greekYears))
	if atm >= wing {
		t.Errorf("at-the-money volga %v is not below the wing's %v", atm, wing)
	}
}

// Degenerate inputs must return zero rather than a number a desk would trade on.
func TestSecondOrderGreeksRefuseDegenerateInputs(t *testing.T) {
	cases := [][4]float64{
		{0, 1, 0.5, 0.25},
		{1, 0, 0.5, 0.25},
		{1, 1, 0, 0.25},
		{1, 1, 0.5, 0},
	}
	for _, c := range cases {
		if got := Black76Vanna(int64(c[0]), int64(c[1]), c[2], c[3]); got != 0 {
			t.Errorf("vanna%v = %v, want 0", c, got)
		}
		if got := Black76Volga(int64(c[0]), int64(c[1]), c[2], c[3]); got != 0 {
			t.Errorf("volga%v = %v, want 0", c, got)
		}
	}
}
