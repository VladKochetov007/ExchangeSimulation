package price

import (
	"math"
	"testing"
)

const sabrForward = int64(50_000 * 100_000_000)

func flatSABR() SABRVolatility   { return SABRVolatility{Alpha: 0.4, Beta: 1, Rho: 0, Nu: 0} }
func smiledSABR() SABRVolatility { return SABRVolatility{Alpha: 0.4, Beta: 1, Rho: -0.3, Nu: 0.6} }

// With no volatility of volatility the model has no smile, and at beta one its
// level is alpha. Anything else means the expansion is wrong at its own
// degenerate point.
func TestSABRWithoutVolOfVolIsFlatAtItsAlpha(t *testing.T) {
	model := flatSABR()
	atm := model.Volatility(sabrForward, sabrForward, 0.25, true)
	if math.Abs(atm-0.4) > 1e-6 {
		t.Errorf("at-the-money volatility = %v, want alpha 0.4", atm)
	}
	for _, strike := range []int64{sabrForward / 2, sabrForward * 3 / 2, sabrForward * 2} {
		wing := model.Volatility(sabrForward, strike, 0.25, true)
		if math.Abs(wing-atm) > 1e-3 {
			t.Errorf("strike %d priced at %v against %v at the money, which is a smile the model does not have", strike, wing, atm)
		}
	}
}

// Volatility of volatility makes the wings dearer than the money, which is the
// curvature the model exists to express.
func TestSABRVolOfVolLiftsTheWings(t *testing.T) {
	model := smiledSABR()
	atm := model.Volatility(sabrForward, sabrForward, 0.25, true)
	low := model.Volatility(sabrForward, sabrForward/2, 0.25, true)
	high := model.Volatility(sabrForward, sabrForward*2, 0.25, true)
	if low <= atm || high <= atm {
		t.Errorf("wings %v and %v are not above the money at %v", low, high, atm)
	}
}

// A negative correlation tilts the smile toward low strikes, which is the
// asymmetry that distinguishes it from a symmetric parabola.
func TestSABRCorrelationTiltsTheSmile(t *testing.T) {
	model := smiledSABR()
	low := model.Volatility(sabrForward, sabrForward*3/4, 0.25, true)
	high := model.Volatility(sabrForward, sabrForward*5/4, 0.25, true)
	if low <= high {
		t.Errorf("a negative correlation did not favour low strikes: %v against %v", low, high)
	}
	model.Rho = 0.3
	low, high = model.Volatility(sabrForward, sabrForward*3/4, 0.25, true), model.Volatility(sabrForward, sabrForward*5/4, 0.25, true)
	if high <= low {
		t.Errorf("a positive correlation did not favour high strikes: %v against %v", high, low)
	}
}

// The at-the-money branch is a limit taken to avoid a zero-over-zero, so it has
// to agree with the general branch just beside it.
func TestSABRIsContinuousAtTheMoney(t *testing.T) {
	model := smiledSABR()
	atm := model.Volatility(sabrForward, sabrForward, 0.25, true)
	beside := model.Volatility(sabrForward, sabrForward+sabrForward/1_000_000, 0.25, true)
	if math.Abs(atm-beside) > 1e-4 {
		t.Errorf("the at-the-money limit %v disagrees with its neighbour %v", atm, beside)
	}
}

// A model that cannot price must decline rather than return a number a dealer
// would quote.
func TestSABRRefusesImpossibleParameters(t *testing.T) {
	for _, model := range []SABRVolatility{
		{Alpha: 0, Beta: 1, Rho: 0, Nu: 0.5},
		{Alpha: 0.4, Beta: 1, Rho: -1, Nu: 0.5},
		{Alpha: 0.4, Beta: 1, Rho: 1, Nu: 0.5},
		{Alpha: 0.4, Beta: 2, Rho: 0, Nu: 0.5},
		{Alpha: 0.4, Beta: 1, Rho: 0, Nu: -1},
	} {
		if got := model.Volatility(sabrForward, sabrForward, 0.25, true); got != 0 {
			t.Errorf("%+v priced at %v, want a refusal", model, got)
		}
	}
	model := smiledSABR()
	if got := model.Volatility(0, sabrForward, 0.25, true); got != 0 {
		t.Errorf("a zero forward priced at %v", got)
	}
	if got := model.Volatility(sabrForward, sabrForward, 0, true); got != 0 {
		t.Errorf("an expired contract priced at %v", got)
	}
}
