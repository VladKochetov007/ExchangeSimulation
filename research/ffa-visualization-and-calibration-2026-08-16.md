# Visualization audit and market calibration, 2026-08-16

This note records problems found by *looking at* the simulated market rather
than by reading its code, what each turned out to be, and how it was resolved.
Three of the five were defects in the measurement, not in the simulator, which
is the reason to keep plotting alongside testing.

## 1. Sweep run against a stale binary (method error)

**Symptom.** The first informed-capacity sweep reported that
`value_trader_count` 0 and 2 gave bit-identical statistics, as did 4 and 8.

**Cause.** `Config.ValueTraderCount` had been changed from `int` to `*int` so
that an explicit `0` could mean "no informed participants" rather than being
replaced by the default. The sweep was then run against a binary built before
that change, so the zero arm silently ran the default of 2.

**Resolution.** Rebuild inside the sweep path and verify the arm before
interpreting it: the check is that the reported participant roster actually
contains the requested number of value traders. After rebuilding, the zero arm
separates cleanly (mean absolute log deviation 0.0514 against 0.0243 with one
informed participant).

**Rule adopted.** An experiment that varies a configuration field must first
assert that field is observable in the run's own output. A sweep whose arms
produce identical numbers is a bug report, not a null result.

## 2. Options plotted against the wrong forward

**Symptom.** Implied volatility inverted from traded option prices reached the
500% inversion ceiling across all strikes.

**Cause.** The renderer used the perpetual mark as the forward. The dealer
prices its options from the spot midpoint, and the perpetual basis was swinging
several hundred basis points, so the moneyness axis mixed in a basis the option
prices never contained.

**Resolution.** Prefer the spot midpoint, matching the pricing convention, and
fall back to the perpetual mark only when spot is unavailable. Values pinned at
the inversion ceiling are now reported as "not explicable by Black-76 at any
plausible volatility", which is a finding rather than a number to plot.

## 3. Dated-futures term structure was flat by construction

**Symptom.** Every dated-future basis point sat exactly on zero.

**Cause.** The engine marks a dated future at the underlying index. Plotting
marks therefore plots the index against itself.

**Resolution.** The panel uses traded futures prices. It currently reports "no
dated-futures trades in window", which is the real state of that board: it has
makers and no takers (FFA-09).

## 4. Funding "cumulative" summed the wrong thing

**Symptom.** Cumulative funding reached 1,400bps in ten simulated minutes.

**Cause.** The funding rate is charged once per period, but the log records it
on every automation tick. Summing samples counts each period thousands of times.

**Resolution.** Plot the rate only, alongside the traded basis that drives it,
and label the funding period taken from `next_funding`.

## 5. The market itself was mis-calibrated

**Symptom.** Once plotted, the perpetual basis oscillated between roughly
+650bps and -300bps, and the spot spread averaged 3.5%.

**Cause.** Making the Avellaneda-Stoikov control scale invariant changed the
units of its risk parameters, and the configured values were never
recalibrated. The spread reduced to the variance cap: with a relative risk
aversion of 250 and a 600 second horizon, the risk term is 3.75%, which is what
the book showed. Realized volatility measured about 57x the fundamental
volatility, so the quoted market was dominated by the makers' own inventory
dynamics rather than by information.

A related bias was measured and fixed on the way: consecutive trade prints
alternate between bid and ask, so per-print sampling measured the maker's own
spread. Over 90 minutes, one-second sampling gave 1.57e-2 against 4.15e-3 for
the midpoint, and the two agreed by 30-second sampling. Makers now sample trade
prices no more often than `stoikov_volatility_sample_interval`, default 30s.

**Resolution.** A calibration sweep over risk aversion and inventory horizon
showed the control depends only on their product:

| risk aversion | horizon | mean abs log deviation | max | fraction beyond 1% |
| --- | --- | --- | --- | --- |
| 0.005 | 600s | 0.0071 | 0.0235 | 0.305 |
| 0.005 | 60s | 0.0034 | 0.0084 | 0.000 |
| 0.0005 | 600s | 0.0034 | 0.0084 | 0.000 |
| 0.0005 | 60s | 0.0047 | 0.0105 | 0.006 |
| 0.00005 | 600s | 0.0047 | 0.0105 | 0.006 |
| 0.00005 | 60s | 0.0048 | 0.0106 | 0.007 |

The two cells with product 0.3 agree to four decimals, which is the scaling law
rather than a coincidence. The default risk aversion is now 0.0005 at the
existing 600s horizon. Mean spot spread falls from 3.318% to 0.174%.

## What remains

At 90 minutes the calibrated market still drifts to a 24% deviation from
fundamental value before returning. With spreads no longer dominating, that
residual is informed capacity (FFA-10): value traders are bounded by inventory,
and the sweep shows marginal participants beyond the fourth never trade at all
because deterministic client-ID priority lets the first movers consume the whole
displayed opportunity each tick. Heterogeneous reaction times are the natural
next treatment, and are also what the latency questions require.
