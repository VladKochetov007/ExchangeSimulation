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
