# The exogenous fundamental: what it is, what it still touches, and whether to delete it

Written for a decision. No code changed.

## What it is

`FundamentalValue` (`simulations/multivenue/fundamental.go`) is a seeded random
walk in log price. It starts at the constant `mvBootstrapPrice` of 50.00, takes
one step per `automation_interval`, and each step is drawn with standard
deviation `fundamental_log_vol_per_step`. The path is generated once and never
redrawn, so it is deterministic and independent of which participant asks first.

It is a number series with a clock. Nothing more.

## What it touches today, after the no-oracle gate

Under a configuration that `NewSim` accepts without `debug_oracle_mode`:

| consumer | file | effect on the market |
|---|---|---|
| `SpotIndex.observeFundamental` | sim.go:1094 | stores the value; `Price()` only returns it when the anchor mode is `fundamental`, which is now refused. Under `consensus` and `own_mid` it returns venue midpoints. **Stored and never read.** |
| `Mispricing.observe(mark, fundamental)` | sim.go:1100 | computes `log(mark / fundamental)` as a reported statistic. **Measurement only.** |
| `logFundamental` | sim.go:1101 | writes a log line. **Measurement only.** |
| `ValueTrader` | sim.go:1360, 1467 | reads it and trades against it. **Refused outside debug mode.** |

So in a legal run the fundamental influences no price, no order, no fill, no
reject, no margin call and no liquidation. Prices come from actors' orders.
Venue marks come from two-sided midpoints of real books. Margin and liquidation
use those venue marks.

It also does **not** set the opening price. That is the constant
`mvBootstrapPrice`, the same number every run.

## The consequence nobody has stated yet

Because the fundamental drives nothing in a legal run,
`fundamental_log_vol_per_step` **does nothing in a legal run**. It is a
configuration knob that silently has no effect, which is exactly the failure mode
strict config decoding was added to prevent.

That kills the carrying-capacity experiment as designed. "Raise fundamental
volatility twenty times and watch makers become insolvent" is not a legal
experiment any more, because there is no exogenous volatility lever. Volatility
now has to be produced by the population itself, which is harder and is the point.

## Arguments for keeping it

1. **Module validation.** A known price path is genuinely useful for checking
   that a quoting engine tracks its reference, that a liquidation fires at the
   price it should, or that a hedge closes the exposure it claims to. These are
   engineering checks, not research claims.
2. **It is currently inert and gated.** Two validation errors and a regression
   test stand between it and any scientific run.
3. **Deterministic scenario shaping for tests.** A fixed, reproducible path makes
   some regression tests simpler to write than an emergent one.

## Arguments against keeping it

1. **It encodes the idea the research rejects.** A variable named "fundamental
   value" in the scenario builder asserts that a correct price exists. There is
   no correct price for a currency or a stock. Anyone reading this code learns
   the wrong model, and the wrong model is what produced the contaminated
   campaign in the first place.
2. **It has already contaminated everything once.** 28 of 30 archived campaign
   configs used it as an oracle. A gate is a lock on a door that should not be
   in the wall.
3. **The mispricing statistic is not a valid output under the law.** "Deviation
   from fundamental" presupposes a fundamental. It is still computed and still
   written into `greeks.json`, where it invites exactly the reasoning we have
   banned.
4. **It leaves a dead knob in the config.** `fundamental_log_vol_per_step`
   silently does nothing, which is the precise class of silent misconfiguration
   we just hardened against.
5. **An exogenous volatility lever is a crutch.** With it available there is a
   standing temptation to manufacture a regime instead of producing one. Without
   it, a volatility regime has to be an emergent property of the population, and
   that is the actual research question.

## Recommendation

Delete it from the scenario builder. If a known-price scaffold is wanted for
engineering checks, build it in the test package where it cannot reach a
research run: a test can construct a price path and drive a quoting engine
directly without the scenario owning a "true value".

Concretely that means removing `FundamentalValue` and `fundamental_log_vol_per_step`
from `Config`, removing `ValueTrader` from the library or moving it to tests,
removing `observeFundamental` and the `fundamental` anchor mode from
`spotIndexProvider`, and either deleting `Mispricing` or redefining it against an
endogenous reference such as the cross-venue consensus midpoint, which measures
venue disagreement rather than deviation from truth.

The cost is honest and worth stating: the volatility-regime experiments cannot be
re-run in their current form, and several regression tests that lean on a known
path will need rewriting against emergent prices.
