package feesim

import (
	"context"
	"testing"
	"time"

	"exchange_sim/exchange"
)

// systemTotal is every unit of an asset the system holds: client wallets,
// minus outstanding debt (a borrow creates balance against a matching
// liability), plus the exchange's own buckets.
func systemTotal(ex *exchange.Exchange, asset string) int64 {
	ex.RLock()
	defer ex.RUnlock()
	var total int64
	for _, client := range ex.Clients {
		total += client.Balances[asset]
		total += client.PerpBalances[asset]
		total -= client.Borrowed[asset]
	}
	total += ex.ExchangeBalance.FeeRevenue[asset]
	total += ex.ExchangeBalance.InsuranceFund[asset]
	return total
}

// openPositionCostBasis is Σ size×entry/precision over open perp positions:
// the quote value still carried inside positions rather than in cash.
func openPositionCostBasis(ex *exchange.Exchange) int64 {
	ex.RLock()
	defer ex.RUnlock()
	pm := ex.Positions.(*exchange.PositionManager)
	var basis int64
	for id := range ex.Clients {
		for _, pos := range pm.GetPositions(id) {
			// Each instrument carries its own base precision — this ecology
			// mixes 1e8 and 1e6 assets, so a shared constant would mis-scale
			// every position on the smaller-precision book.
			inst := ex.Instruments[pos.Symbol]
			if inst == nil {
				continue
			}
			basis += exchange.MulDiv(pos.Size, pos.EntryPrice, inst.BasePrecision())
		}
	}
	return basis
}

// The whole ecology — MMs, Hawkes taker, basis/funding/triangle arbs, value
// traders, reactive race entrants — must conserve every asset.
//
// Cash alone is NOT the invariant while positions are open: a trader closing
// against an averaged entry realizes cash immediately while the offsetting
// loss sits unrealized in the counterparties' open positions. Total wealth at
// any mark M is Σcash + Σ size×(M−entry)/prec, and the mark term vanishes
// because positions are zero-sum, so the conserved quantity is
// Σcash − Σ size×entry/prec. Only entry-price rounding may leak, and only by
// dust.
func TestFeesimEcologyConservesEveryAsset(t *testing.T) {
	for _, reactive := range []bool{false, true} {
		name := "polling"
		if reactive {
			name = "reactive"
		}
		t.Run(name, func(t *testing.T) {
			cfg := DefaultSimConfig()
			cfg.LogDir = t.TempDir()
			cfg.RaceArbTiers = []float64{1.0, 0.2}
			cfg.RaceArbReactive = reactive

			sim, err := NewSim(2*time.Minute, cfg)
			if err != nil {
				t.Fatalf("NewSim: %v", err)
			}
			defer sim.Close()

			ex := sim.Exchange()
			assets := []string{"USD", "ABC", "Q"}
			before := make(map[string]int64, len(assets))
			for _, a := range assets {
				before[a] = systemTotal(ex, a)
			}

			ctx, cancel := context.WithCancel(context.Background())
			ex.StartAutomation(ctx)
			sim.Runner.SetShutdownHook(func() {
				cancel()
				ex.StopAutomation()
			})
			if err := sim.Runner.Run(ctx); err != nil {
				t.Fatalf("Run: %v", err)
			}

			// Entry prices are stored as truncated integers, so each
			// position-reducing fill can leak up to (position size /
			// precision) units of quote. That scales with inventory, not with
			// a fixed number, so the bound is relative to the money in the
			// system: a systematic leak grows without limit and blows through
			// this, while rounding stays orders of magnitude below it. Only
			// the perp quote asset is affected — the spot legs conserve
			// exactly, which is the evidence that rounding is the only source.
			for _, a := range assets {
				dustTolerance := before[a] / 100_000_000
				if dustTolerance < 1_000_000 {
					dustTolerance = 1_000_000
				}
				got := systemTotal(ex, a)
				if a == "USD" {
					got -= openPositionCostBasis(ex)
				}
				delta := got - before[a]
				if delta < 0 {
					delta = -delta
				}
				if delta > dustTolerance {
					t.Errorf("%s not conserved: %d -> %d (delta %d, tolerance %d)",
						a, before[a], got, got-before[a], dustTolerance)
				} else {
					t.Logf("%s conserved within dust: delta %d", a, got-before[a])
				}
			}
		})
	}
}
