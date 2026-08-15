package price

import (
	"math"
	"testing"
)

func TestBlack76GammaMatchesDeltaFiniteDifference(t *testing.T) {
	forward, strike := int64(10_000), int64(10_000)
	vol, expiry := 0.6, 0.25
	step := int64(5)
	got := Black76Gamma(forward, strike, vol, expiry)
	want := (Black76Delta(forward+step, strike, vol, expiry, true) -
		Black76Delta(forward-step, strike, vol, expiry, true)) / float64(2*step)
	if math.Abs(got-want) > 1e-7 {
		t.Fatalf("gamma = %.12f, finite-difference delta slope = %.12f", got, want)
	}
	if got <= 0 {
		t.Fatalf("ATM gamma = %.12f, want positive", got)
	}
}

func TestBlack76VegaMatchesPremiumFiniteDifference(t *testing.T) {
	forward, strike := int64(100_000), int64(100_000)
	vol, expiry, step := 0.8, 0.5, 0.01
	got := Black76Vega(forward, strike, vol, expiry)
	want := float64(Black76Premium(forward, strike, vol+step, expiry, true)-
		Black76Premium(forward, strike, vol-step, expiry, true)) / (2 * step)
	// Premium is returned in integer quote units, so the finite difference has
	// at most one unit of rounding error on each side.
	if math.Abs(got-want) > 110 {
		t.Fatalf("vega = %.6f, finite-difference premium slope = %.6f", got, want)
	}
	if got <= 0 {
		t.Fatalf("ATM vega = %.6f, want positive", got)
	}
}

func TestBlack76SensitivitiesRespectCallPutParity(t *testing.T) {
	call, ok := Black76Sensitivities(100_000, 95_000, 0.7, 0.25, true)
	if !ok {
		t.Fatal("call sensitivities unexpectedly invalid")
	}
	put, ok := Black76Sensitivities(100_000, 95_000, 0.7, 0.25, false)
	if !ok {
		t.Fatal("put sensitivities unexpectedly invalid")
	}
	if math.Abs((call.Delta-put.Delta)-1) > 1e-12 {
		t.Fatalf("delta call-put = %.15f, want 1", call.Delta-put.Delta)
	}
	if math.Abs(call.Gamma-put.Gamma) > 1e-15 || math.Abs(call.Vega-put.Vega) > 1e-12 {
		t.Fatalf("call/put curvature differs: call=%+v put=%+v", call, put)
	}
	if call.Theta >= 0 || put.Theta >= 0 {
		t.Fatalf("zero-rate Black-76 theta must be negative: call=%f put=%f", call.Theta, put.Theta)
	}
}

func TestBlack76GreeksAreZeroForDegenerateContracts(t *testing.T) {
	for _, tc := range []struct {
		forward, strike int64
		vol, expiry     float64
	}{
		{0, 100, 0.5, 1},
		{100, 0, 0.5, 1},
		{100, 100, 0, 1},
		{100, 100, 0.5, 0},
		{100, 100, math.NaN(), 1},
		{100, 100, math.Inf(1), 1},
	} {
		if got := Black76Gamma(tc.forward, tc.strike, tc.vol, tc.expiry); got != 0 {
			t.Fatalf("gamma(%+v) = %v, want 0", tc, got)
		}
		if got := Black76Vega(tc.forward, tc.strike, tc.vol, tc.expiry); got != 0 {
			t.Fatalf("vega(%+v) = %v, want 0", tc, got)
		}
		if _, ok := Black76Sensitivities(tc.forward, tc.strike, tc.vol, tc.expiry, true); ok {
			t.Fatalf("sensitivities(%+v) unexpectedly valid", tc)
		}
	}
}
