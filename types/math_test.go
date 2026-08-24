package types

import (
	"math"
	"math/big"
	"testing"
)

func refMulDiv(a, b, c int64) int64 {
	r := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	r.Quo(r, big.NewInt(c)) // Quo truncates toward zero, matching int64 division
	return r.Int64()
}

func TestMulDivMatchesBigIntReference(t *testing.T) {
	cases := []struct{ a, b, c int64 }{
		{0, 5, 3},
		{7, -1, 10},
		{17, -3, 10},
		{-17, 3, 10},
		{-17, -3, 10},
		{15, -1, 10},
		{1, 1, 1},
		{100_000_000, 5_000_000_000, 100_000_000},
		// The old (a/c)*b + (a%c)*b/c form overflowed here:
		// (qty % 1e8) × price with quotePrecision=1e8 scale prices.
		{150_000_000, 5_000_000_000_000, 100_000_000},
		{99_999_999, 9_000_000_000_000, 100_000_000},
		{-99_999_999, 9_000_000_000_000, 100_000_000},
		{99_999_999, -9_000_000_000_000, 100_000_000},
		{math.MaxInt64, 1, 1},
		{math.MinInt64, 1, 1},
		{math.MinInt64, 1, 2},
		{math.MaxInt64, 2, 4},
	}
	for _, tc := range cases {
		want := refMulDiv(tc.a, tc.b, tc.c)
		if got := MulDiv(tc.a, tc.b, tc.c); got != want {
			t.Errorf("MulDiv(%d, %d, %d) = %d, want %d", tc.a, tc.b, tc.c, got, want)
		}
	}
}

func TestMulDivOverflowPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic when quotient exceeds int64")
		}
	}()
	MulDiv(math.MaxInt64, math.MaxInt64, 1)
}

func TestMulDivNonPositiveDivisorPanics(t *testing.T) {
	for _, c := range []int64{0, -1, -100} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("expected panic for divisor %d", c)
				}
			}()
			MulDiv(10, 10, c)
		}()
	}
}

func TestTryMulBps(t *testing.T) {
	if got, ok := TryMulBps(math.MaxInt64, 10_000); !ok || got != math.MaxInt64 {
		t.Fatalf("max exact bps = (%d, %t), want (%d, true)", got, ok, int64(math.MaxInt64))
	}
	if _, ok := TryMulBps(math.MaxInt64, 10_001); ok {
		t.Fatal("overflowing bps calculation reported success")
	}
	if got, ok := TryMulBps(10_000, -25); !ok || got != -25 {
		t.Fatalf("rebate bps = (%d, %t), want (-25, true)", got, ok)
	}
}

func TestMidpointMatchesBigIntAcrossSignedDomain(t *testing.T) {
	cases := []struct {
		name string
		a    int64
		b    int64
	}{
		{name: "min/min", a: math.MinInt64, b: math.MinInt64},
		{name: "max/max", a: math.MaxInt64, b: math.MaxInt64},
		{name: "min/max", a: math.MinInt64, b: math.MaxInt64},
		{name: "negative/negative odd", a: -5, b: -4},
		{name: "negative/zero odd", a: -5, b: 0},
		{name: "negative/positive odd", a: -5, b: 4},
		{name: "zero/positive odd", a: 0, b: 5},
		{name: "ordinary positive odd", a: 100, b: 101},
		{name: "equal endpoints", a: -99, b: -99},
		{name: "wide signed interval", a: math.MinInt64 + 1, b: math.MaxInt64},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := new(big.Int).Add(big.NewInt(tc.a), big.NewInt(tc.b))
			want.Quo(want, big.NewInt(2))
			if got := Midpoint(tc.a, tc.b); got != want.Int64() {
				t.Fatalf("Midpoint(%d, %d) = %d, want %d", tc.a, tc.b, got, want.Int64())
			}
		})
	}
}

func TestMidpointExhaustiveSmallSignedRange(t *testing.T) {
	for a := int64(-256); a <= 256; a++ {
		for b := int64(-256); b <= 256; b++ {
			want := (a + b) / 2
			if got := Midpoint(a, b); got != want {
				t.Fatalf("Midpoint(%d, %d) = %d, want %d", a, b, got, want)
			}
		}
	}
}
