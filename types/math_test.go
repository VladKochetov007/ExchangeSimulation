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

func TestTryAbsMulDivUsesMagnitudeWithoutMinIntOverflow(t *testing.T) {
	tests := []struct {
		name string
		a    int64
		b    int64
		c    int64
		want int64
		ok   bool
	}{
		{name: "negative price", a: 3, b: -20, c: 1, want: 60, ok: true},
		{name: "zero price", a: 3, b: 0, c: 1, want: 0, ok: true},
		{name: "min price divided", a: 1, b: math.MinInt64, c: 2, want: 1 << 62, ok: true},
		{name: "unrepresentable min magnitude", a: 1, b: math.MinInt64, c: 1, ok: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := TryAbsMulDiv(tc.a, tc.b, tc.c)
			if ok != tc.ok || ok && got != tc.want {
				t.Fatalf("TryAbsMulDiv(%d, %d, %d) = (%d, %t), want (%d, %t)", tc.a, tc.b, tc.c, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestTryPriceChangeMulDivMatchesBigIntAcrossSignedPriceChanges(t *testing.T) {
	tests := []struct {
		qty, to, from, precision int64
	}{
		{qty: 1, from: 20, to: -20, precision: 1},
		{qty: -1, from: 20, to: -20, precision: 1},
		{qty: 1, from: -20, to: 20, precision: 1},
		{qty: -1, from: -20, to: 20, precision: 1},
		{qty: 1, from: -20, to: -40, precision: 1},
		{qty: -1, from: -40, to: -20, precision: 1},
		{qty: 1, from: math.MinInt64, to: math.MaxInt64, precision: math.MaxInt64},
	}
	for _, tc := range tests {
		t.Run("signed price change", func(t *testing.T) {
			want := new(big.Int).Mul(big.NewInt(tc.qty), new(big.Int).Sub(big.NewInt(tc.to), big.NewInt(tc.from)))
			want.Quo(want, big.NewInt(tc.precision))
			if !want.IsInt64() {
				t.Fatal("test reference unexpectedly exceeds int64")
			}
			got, ok := TryPriceChangeMulDiv(tc.qty, tc.to, tc.from, tc.precision)
			if !ok || got != want.Int64() {
				t.Fatalf("TryPriceChangeMulDiv(%d, %d, %d, %d) = (%d, %t), want (%d, true)", tc.qty, tc.to, tc.from, tc.precision, got, ok, want.Int64())
			}
		})
	}
}
