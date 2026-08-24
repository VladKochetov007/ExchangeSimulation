package analysis

import (
	"math"
	"testing"
)

func TestMulDivMakesUnrepresentableResultsExplicit(t *testing.T) {
	if got, ok := mulDiv(math.MaxInt64, 1, 1); !ok || got != math.MaxInt64 {
		t.Fatalf("exact product = (%d, %t), want (%d, true)", got, ok, int64(math.MaxInt64))
	}
	if got, ok := mulDiv(math.MaxInt64, 2, 1); ok || got != 0 {
		t.Fatalf("overflow product = (%d, %t), want unavailable", got, ok)
	}
	if got, ok := mulDiv(-5, 3, 2); !ok || got != -7 {
		t.Fatalf("signed truncation = (%d, %t), want (-7, true)", got, ok)
	}
	if got, ok := mulDiv(1, 1, 0); ok || got != 0 {
		t.Fatalf("zero denominator = (%d, %t), want unavailable", got, ok)
	}
}
