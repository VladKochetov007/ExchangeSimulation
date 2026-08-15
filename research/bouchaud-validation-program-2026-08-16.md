# Bouchaud validation program

A simulator that reproduces known empirical regularities is a simulator whose
mechanics can be trusted for questions nobody has measured. This file records
which regularities we are trying to reproduce, why each is a test of a specific
mechanism rather than of a number, and what has been run so far.

Selection principle from the literature survey: prefer the *conditional*
predictions. A simulator will produce an impact exponent near 0.5 for several
trivial reasons, mechanical book depletion being the most common. What
separates reproducing the physics from reproducing the number is whether the
conditional structure survives — independence from participation rate, the
propagator identity, the relation between order-flow memory and metaorder size
distribution.

## Papers and their status

| # | Paper | arXiv | Tests | Needs | Status |
| --- | --- | --- | --- | --- | --- |
| 3 | Wyart, Bouchaud, Kockelkoren, Potters, Vettorazzo — spread, impact and volatility | [physics/0603084](https://arxiv.org/abs/physics/0603084) | whether spread is set by adverse selection | nothing new | **run: falsified, see below** |
| 2 | Bouchaud, Gefen, Potters, Wyart — fluctuations and response | [cond-mat/0307332](https://arxiv.org/abs/cond-mat/0307332) | transient impact reconciling persistent flow with diffusive prices | persistent order flow | next |
| 5 | Tóth, Palit, Lillo, Farmer — why is order flow so persistent | [1108.1632](https://arxiv.org/abs/1108.1632) | splitting versus herding as the source of flow memory | execution agent | blocked on execution agent |
| 1 | Tóth et al — anomalous price impact, critical liquidity | [1105.1694](https://arxiv.org/abs/1105.1694) | square-root impact law and its independence from participation rate | execution agent | blocked on execution agent |
| 7 | Bucci, Benzaquen, Lillo, Bouchaud — linear to square-root crossover | [1811.05230](https://arxiv.org/abs/1811.05230) | two liquidity timescales | execution agent + heterogeneous maker requote rates | blocked |
| 4 | Eisler, Bouchaud, Kockelkoren — impact of order book events | [0904.0900](https://arxiv.org/abs/0904.0900) | event-typed propagators, large-tick vs small-tick switch | nothing new | queued |
| 6 | Donier, Bonart, Mastromatteo, Bouchaud — minimal model for non-linear impact | [1412.0141](https://arxiv.org/abs/1412.0141) | latent liquidity, V-shaped book, prefactor scaling | latent liquidity agent | later |
| 8 | Benzaquen, Mastromatteo, Eisler, Bouchaud — cross-impact | [1609.02395](https://arxiv.org/abs/1609.02395) | cross-asset impact versus common news | correlated fundamentals | later |

The single highest-value build is an **execution agent** that splits a parent
order over a horizon at a controlled participation rate: it unlocks #1, #5 and
#7, and generates persistent order flow endogenously for #2.

## Test 3, spread versus volatility per trade: falsified

**Prediction.** From a marginal-profitability condition on market making, the
spread should be proportional to volatility measured per trade, with a constant
of order unity and a tight fit across a scatter of markets.

**Method.** Four controls swept separately, each over four values, two seeds,
three venues, 30 simulated minutes per cell: noise-taker count, maker risk
aversion, fundamental volatility, informed-trader count. Per run we measure the
time-weighted relative spread, the standard deviation of midpoint log returns
per sample, and the trade count, then form volatility per trade as
`sigma_per_sample / sqrt(trades per sample)`.

Sweeping several controls is the whole point: if the relation is real, every
control must trace the same line.

**Result, small-tick regime.**

| control | c through origin | slope | R² |
| --- | ---: | ---: | ---: |
| noise intensity | 29.52 | −0.38 | 0.052 |
| maker risk aversion | 13.17 | 10.87 | 0.858 |
| fundamental volatility | 29.52 | −0.12 | 0.008 |
| informed count | 31.07 | 0.39 | 0.028 |
| pooled | 14.13 | 11.03 | 0.877 |

The controls trace different lines, the constant is 13–31 rather than order
unity, and three of four sweeps show no relation at all. The pooled R² of 0.877
is an artifact of the risk-aversion arm dominating the range.

**Why.** The spread does not respond to market conditions at all:

| control | value range | spread | volatility per trade |
| --- | --- | --- | --- |
| noise takers | 1 → 8 | 0.01497% → 0.01509% | unchanged |
| fundamental vol | 5e-5 → 4e-4 | 0.01496% → 0.01511% | 3e-6 → 6e-6 |
| informed traders | 0 → 4 | 0.01500% → 0.01581% | 1e-6 → 5e-6 |
| maker risk aversion | 2e-4 → 2e-3 | 0.01082% → 0.08488% | 2e-6 → 6e-5 |

An eightfold change in fundamental volatility doubles volatility per trade and
moves the spread by 1%. Only the maker's own configured risk aversion moves it.

The mechanism is visible in the control law. The Avellaneda-Stoikov spread is
`gamma*sigma^2*tau + (2/gamma)*log1p(gamma/kappa)`. At the calibrated risk
aversion the first term — the only volatility-dependent one — is negligible
against the second, which is a constant. Our makers therefore quote a fixed
fraction of price and do not price adverse selection at all.

This is consistent with the direct evidence from the balance ledger, which no
empiricist has: makers earn a small systematic profit when no fast informed
participant is present (+239 to +348 USD) and lose heavily when one appears
(−52,969 USD). They are not marginally profitable in either case, so the
premise the derivation rests on does not hold here.

**Confirmed on the way: the large-tick degeneracy.** The paper's own caveat is
that the relation degenerates when the spread is pinned at one tick. At the
original $10 tick the spread sat at 2.96–3.00 ticks in every cell regardless of
control. Making the tick configurable and reducing it to $0.10 moved the spread
off the floor (0.0150% relative, unchanged at $0.01), which is the small-tick
regime the test requires. Both regimes behaved exactly as the paper predicts.

**What this means for the simulator.** Spread here is a configured parameter,
not an equilibrium outcome. Any experiment whose conclusion depends on spreads
responding to volatility or to informed-flow intensity is currently invalid.
Making the makers marginally profitable — so that the spread is *derived* from
the adverse selection they suffer — is now the highest-value realism change,
and this test is the acceptance criterion for it.
