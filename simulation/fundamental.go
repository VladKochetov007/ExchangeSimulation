package simulation

import (
	"math"
	"math/rand"
	"sync"
)

// FundamentalValueProcess models the latent "true price" of an asset via
// geometric Brownian motion. It implements exchange.IndexPriceProvider so it
// can be wired directly into any of the index-anchored mark price calculators,
// and exchange.PrivateSignalOracle so informed traders can access the signal.
//
// Units: price is stored in the same integer precision as the exchange
// (e.g. USD_PRECISION = 100_000 units per dollar). dt is in seconds.
//
// GBM: S(t+dt) = S(t) * exp((mu - 0.5*sigma^2)*dt + sigma*sqrt(dt)*Z)
// where Z ~ N(0,1) drawn per Advance call.
type GBMProcess struct {
	mu    float64 // annualised drift (e.g. 0.0 for zero-drift)
	sigma float64 // annualised volatility (e.g. 0.5 for 50%)
	price float64 // current value in float (converted from/to int precision)
	prec  int64   // integer precision (e.g. USD_PRECISION = 100_000)
	rng   *rand.Rand
	mu_   sync.Mutex

	// perpToSymbol maps perp symbol -> this process handles it.
	// A single GBM can serve multiple perp symbols (e.g. for testing).
	symbols map[string]struct{}
}

func NewGBMProcess(initialPrice int64, prec int64, annualDrift, annualVol float64, seed int64) *GBMProcess {
	return &GBMProcess{
		mu:      annualDrift,
		sigma:   annualVol,
		price:   float64(initialPrice) / float64(prec),
		prec:    prec,
		rng:     rand.New(rand.NewSource(seed)),
		symbols: make(map[string]struct{}),
	}
}

// Register associates a symbol with this process so GetIndexPrice returns it.
func (g *GBMProcess) Register(symbol string) {
	g.mu_.Lock()
	g.symbols[symbol] = struct{}{}
	g.mu_.Unlock()
}

// Advance steps the GBM forward by dt seconds. Call this from a simulation
// tick loop to evolve the fundamental value over time.
func (g *GBMProcess) Advance(dtSeconds float64) {
	g.mu_.Lock()
	defer g.mu_.Unlock()
	z := g.rng.NormFloat64()
	g.price *= math.Exp((g.mu-0.5*g.sigma*g.sigma)*dtSeconds +
		g.sigma*math.Sqrt(dtSeconds)*z)
}

// Price returns the current fundamental value as an integer in exchange units.
func (g *GBMProcess) Price() int64 {
	g.mu_.Lock()
	defer g.mu_.Unlock()
	return int64(g.price * float64(g.prec))
}

// GetIndexPrice implements exchange.IndexPriceProvider.
// timestamp is ignored; callers advance time via Advance().
func (g *GBMProcess) GetIndexPrice(symbol string, _ int64) int64 {
	g.mu_.Lock()
	_, ok := g.symbols[symbol]
	g.mu_.Unlock()
	if !ok {
		return 0
	}
	return g.Price()
}

// GetSignal implements PrivateSignalOracle.
// Returns the current fundamental value regardless of symbol (single-asset process).
func (g *GBMProcess) GetSignal(symbol string, _ int64) int64 {
	return g.GetIndexPrice(symbol, 0)
}
