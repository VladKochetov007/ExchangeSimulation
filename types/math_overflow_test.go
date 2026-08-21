package types

import (
	"math"
	"math/bits"
	"math/rand"
	"testing"
)

// Go wraps silently on int64 overflow, so a balance one unit past the ceiling
// becomes a large negative number that reads as a debt. The checked add exists
// to make that impossible to miss.
func TestTryAddDetectsWrapInBothDirections(t *testing.T) {
	cases := []struct {
		a, b int64
		ok   bool
		want int64
	}{
		{math.MaxInt64, 1, false, 0},
		{math.MaxInt64 - 1, 1, true, math.MaxInt64},
		{math.MinInt64, -1, false, 0},
		{math.MinInt64 + 1, -1, true, math.MinInt64},
		{math.MaxInt64, math.MinInt64, true, -1},
		{0, 0, true, 0},
		{-5, 5, true, 0},
	}
	for _, c := range cases {
		got, ok := TryAdd(c.a, c.b)
		if ok != c.ok {
			t.Errorf("TryAdd(%d, %d) ok = %v, want %v", c.a, c.b, ok, c.ok)
			continue
		}
		if ok && got != c.want {
			t.Errorf("TryAdd(%d, %d) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// The property that matters: the checked add accepts exactly when the wrapped
// sum is the true sum, and refuses exactly when the wrap changed the sign.
func TestTryAddAgreesWithWrapDetection(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 200_000; i++ {
		a, b := rng.Int63()-rng.Int63(), rng.Int63()-rng.Int63()
		if i%7 == 0 {
			a = math.MaxInt64 - rng.Int63n(1024)
		}
		if i%11 == 0 {
			b = math.MinInt64 + rng.Int63n(1024)
		}
		got, ok := TryAdd(a, b)
		wrapped := a + b
		sameSign := (a >= 0) == (b >= 0)
		overflowed := sameSign && ((a >= 0) != (wrapped >= 0))
		if ok == overflowed {
			t.Fatalf("TryAdd(%d, %d) ok = %v but overflow = %v", a, b, ok, overflowed)
		}
		if ok && got != wrapped {
			t.Fatalf("TryAdd(%d, %d) = %d, want %d", a, b, got, wrapped)
		}
	}
}

// AddAmount is for invariants rather than inputs: it must stop rather than
// wrap, because an exchange that cannot represent its own revenue has to fail
// loudly.
func TestAddAmountPanicsRatherThanWrapping(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("AddAmount wrapped instead of panicking")
		}
	}()
	AddAmount(math.MaxInt64, 1)
}

// MulDiv is the only multiplication on the money paths, so its refusal
// boundary is the arithmetic boundary of the whole system.
func TestMulDivMatchesAnIndependent128BitComputation(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	checked := 0
	for i := 0; i < 200_000; i++ {
		// Sized like the money paths themselves: a quantity in base units and
		// a price in quote units against a precision divisor. Drawing all
		// three uniformly from the whole range makes almost every case
		// unrepresentable and tests nothing.
		a := rng.Int63n(1<<40) - 1<<39
		b := rng.Int63n(1<<40) - 1<<39
		c := rng.Int63n(1<<30) + 1
		got, ok := TryMulDiv(a, b, c)
		if !ok {
			continue
		}
		checked++
		if want := referenceMulDiv(a, b, c); got != want {
			t.Fatalf("TryMulDiv(%d, %d, %d) = %d, want %d", a, b, c, got, want)
		}
	}
	if checked < 1000 {
		t.Fatalf("only %d cases were representable; the test proved almost nothing", checked)
	}
}

// A product that does not fit must be refused rather than truncated, since a
// truncated notional is a silently wrong trade.
func TestMulDivRefusesUnrepresentableQuotients(t *testing.T) {
	if _, ok := TryMulDiv(math.MaxInt64, math.MaxInt64, 1); ok {
		t.Error("an unrepresentable quotient was accepted")
	}
	if _, ok := TryMulDiv(math.MaxInt64, 2, 1); ok {
		t.Error("an unrepresentable quotient was accepted")
	}
	if _, ok := TryMulDiv(1, 1, 0); ok {
		t.Error("division by zero was accepted")
	}
}

// referenceMulDiv recomputes the quotient through the 128-bit primitives
// directly, which is an independent path from the production implementation's
// sign and bound handling.
func referenceMulDiv(a, b, c int64) int64 {
	negative := (a < 0) != (b < 0)
	hi, lo := bits.Mul64(magnitude(a), magnitude(b))
	quotient, _ := bits.Div64(hi, lo, uint64(c))
	if negative {
		return -int64(quotient)
	}
	return int64(quotient)
}

func magnitude(v int64) uint64 {
	if v < 0 {
		return uint64(-v)
	}
	return uint64(v)
}
