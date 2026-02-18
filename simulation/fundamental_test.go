package simulation

import (
	"math"
	"testing"
)

const testPrec = int64(100_000) // USD_PRECISION stand-in

func TestGBM_InitialPrice(t *testing.T) {
	initial := int64(50_000 * testPrec)
	g := NewGBMProcess(initial, testPrec, 0, 0, 42)
	if got := g.Price(); got != initial {
		t.Errorf("initial Price(): want %d, got %d", initial, got)
	}
}

func TestGBM_UnregisteredSymbolReturnsZero(t *testing.T) {
	g := NewGBMProcess(int64(100*testPrec), testPrec, 0, 0.5, 42)
	if price := g.GetIndexPrice("UNKNOWN", 0); price != 0 {
		t.Errorf("unregistered symbol: want 0, got %d", price)
	}
}

func TestGBM_RegisteredSymbolReturnsPrice(t *testing.T) {
	initial := int64(100 * testPrec)
	g := NewGBMProcess(initial, testPrec, 0, 0, 42)
	g.Register("BTC-USD")
	if got := g.GetIndexPrice("BTC-USD", 0); got != initial {
		t.Errorf("registered symbol before Advance: want %d, got %d", initial, got)
	}
}

func TestGBM_GetSignalMatchesGetIndexPrice(t *testing.T) {
	g := NewGBMProcess(int64(100*testPrec), testPrec, 0, 0.5, 42)
	g.Register("X")
	g.Advance(1.0 / 252)
	if a, b := g.GetIndexPrice("X", 0), g.GetSignal("X", 0); a != b {
		t.Errorf("GetSignal %d != GetIndexPrice %d", b, a)
	}
}

func TestGBM_ZeroVolNoChange(t *testing.T) {
	// S(t+dt) = S(t)*exp((mu-σ²/2)*dt + σ*sqrt(dt)*Z). With σ=0 and mu=0: exp(0)=1 always.
	initial := int64(50_000 * testPrec)
	g := NewGBMProcess(initial, testPrec, 0, 0, 42)
	for i := 0; i < 100; i++ {
		g.Advance(1.0 / 252)
	}
	if got := g.Price(); got != initial {
		t.Errorf("zero-vol zero-drift: price changed from %d to %d", initial, got)
	}
}

func TestGBM_PositiveDriftPriceGrows(t *testing.T) {
	initial := int64(100 * testPrec)
	g := NewGBMProcess(initial, testPrec, 0.10, 0, 42) // 10% annual drift, σ=0
	g.Advance(1.0)                                      // advance 1 year; exp(0.10) ≈ 1.105
	if got := g.Price(); got <= initial {
		t.Errorf("positive drift: price should have grown from %d, got %d", initial, got)
	}
}

func TestGBM_PriceAlwaysPositive(t *testing.T) {
	// Moderate params over ~4 years; log-normal S can never reach zero in float64.
	g := NewGBMProcess(int64(100*testPrec), testPrec, 0, 0.3, 7)
	for i := 0; i < 1000; i++ {
		g.Advance(1.0 / 252)
		if g.Price() <= 0 {
			t.Fatalf("price went non-positive after %d steps: %d", i+1, g.Price())
		}
	}
}

func TestGBM_MultipleRegistrations(t *testing.T) {
	g := NewGBMProcess(int64(100*testPrec), testPrec, 0, 0, 42)
	g.Register("BTC-USD")
	g.Register("BTC-PERP")

	if g.GetIndexPrice("BTC-USD", 0) == 0 {
		t.Error("BTC-USD should return price")
	}
	if g.GetIndexPrice("BTC-PERP", 0) == 0 {
		t.Error("BTC-PERP should return price")
	}
	if g.GetIndexPrice("ETH-USD", 0) != 0 {
		t.Error("ETH-USD not registered, should return 0")
	}
}

func TestGBM_DriftTermExact(t *testing.T) {
	// With σ=0 the path is deterministic: S(T) = S(0)*exp(mu*T).
	// Catches errors in the drift exponent (missing 0.5*σ² term, wrong sign, etc.).
	const (
		mu      = 0.10
		initial = 100_000
		steps   = 252
		dt      = 1.0 / 252
	)
	g := NewGBMProcess(initial*testPrec, testPrec, mu, 0, 42)
	for i := 0; i < steps; i++ {
		g.Advance(dt)
	}
	expected := float64(initial*testPrec) * math.Exp(mu*1.0)
	relErr := math.Abs(float64(g.Price())-expected) / expected
	// Allow 0.1% for integer rounding accumulated over 252 steps.
	if relErr > 0.001 {
		t.Errorf("drift mismatch: want ~%.0f, got %d (rel err %.4f)", expected, g.Price(), relErr)
	}
}
