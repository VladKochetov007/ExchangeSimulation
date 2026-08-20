package derivsim

import (
	"testing"
	"time"

	eprice "exchange_sim/price"
)

// A dealer must price from its injected model, and fall back to the configured
// level when the model declines rather than leaving the book empty.
func TestOptionMakerPricesFromItsVolatilityModel(t *testing.T) {
	mm := &OptionMarketMaker{cfg: OptionMMConfig{IV: 0.8, VolModel: eprice.FlatVolatility(0.3)}}
	if got := mm.volatility(45_000, 0.25, true); got != 0.3 {
		t.Errorf("volatility = %v, want the model's 0.3", got)
	}
	mm.cfg.VolModel = eprice.FlatVolatility(0)
	if got := mm.volatility(45_000, 0.25, true); got != 0.8 {
		t.Errorf("volatility = %v, want the configured fallback 0.8", got)
	}
	mm.cfg.VolModel = nil
	if got := mm.volatility(45_000, 0.25, true); got != 0.8 {
		t.Errorf("volatility = %v, want the configured 0.8", got)
	}
}

// The estimator only learns if the dealer feeds it, and it must be fed the
// underlying's own path rather than an option premium.
func TestOptionMakerFeedsItsEstimatorTheUnderlying(t *testing.T) {
	estimator := eprice.NewRealizedVolatility(0, 600, 1, 0, 0)
	mm := &OptionMarketMaker{cfg: OptionMMConfig{IV: 0.8, VolModel: estimator}}
	price := int64(50_000 * 100_000_000)
	for i := 0; i < 50; i++ {
		move := 1.001
		if i%2 == 1 {
			move = 0.999
		}
		price = int64(float64(price) * move)
		mm.spotMid = price
		mm.observeUnderlying(int64(i+1) * int64(time.Second))
	}
	if estimator.Samples() < 40 {
		t.Fatalf("estimator saw %d samples, want the dealer's whole path", estimator.Samples())
	}
	if vol := mm.volatility(45_000, 0.25, true); vol <= 0 || vol == 0.8 {
		t.Errorf("dealer priced at %v, which is not its own estimate", vol)
	}
}
