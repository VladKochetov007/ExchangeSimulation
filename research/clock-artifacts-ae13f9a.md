# ae13f9a clock-artifact screen

## Question and design

This is a timing-sensitivity screen on the frozen `ae13f9a` simulator, not an
economic redesign or a new freeze. Each world ran for five simulated hours with
full raw logging, paired seeds 101 and 103, and a retained
`evidenceartifacthash.json` over persisted JSON records.

The frozen control is byte-identical to the baseline configuration after
removing experiment metadata, seed, and horizon. The timing treatments are:

| condition | delta from frozen control | purpose |
|---|---|---|
| `control` | none | paired five-hour reference |
| `step_100ms` | runner step 1s -> 100ms only | distinguish time resolution from cadence changes |
| `destagger` | 100ms step plus de-aligned publication, quote, flow, hedge, and strategy clocks | screen the synchronized cadence lattice |

All economic fields, assets, participant counts, fees, risk limits, and latency
distributions are unchanged. `step` is nevertheless a real model parameter: it
changes when scheduled deliveries can be observed. Neither treatment is an
equivalent implementation of the baseline.

## Result

The frozen ecological conclusions are **clock-sensitive**.

| seed | condition | pooled abs perp basis (bps) | half-life (s) | central triangular mean edge (bps) | 1s return ACF(1) | 1s abs-return ACF(1) |
|---|---:|---:|---:|---:|---:|---:|
| 101 | control | 62.53 | 254.10 | 8,869.72 | -0.566 | 0.336 |
| 101 | step_100ms | 201.01 | 1,121.84 | 19,770.94 | -0.546 | 0.270 |
| 101 | destagger | 2.80 | 2.27 | 22,926.03 | -0.529 | 0.127 |
| 103 | control | 93.31 | 517.74 | 8,686.53 | -0.551 | 0.305 |
| 103 | step_100ms | 188.78 | 283.19 | 20,843.65 | -0.540 | 0.265 |
| 103 | destagger | 2.37 | 5.87 | 26,880.57 | -0.523 | 0.122 |

For both seeds, `step_100ms` widened and made the perp basis more persistent,
whereas the de-staggered package compressed it by 95.5% and 97.5% relative to
the paired controls. The de-staggered result is therefore not a simple
numerical-resolution correction: it is an interaction between the finer
delivery lattice and one or more de-aligned cadences.

The screen strengthens, rather than repairs, the triangular-arbitrage failure:
the central triangular mean edge rises from about 8.7–8.9k bps in the control
to 19.8–20.8k with the finer step and 22.9–26.9k in the de-staggered worlds.
The negative 1s raw-return ACF remains large under every condition, so the
timing package does not restore the target near-zero raw-return ACF. The
north-to-central executable cross-venue share falls to zero in both 100ms-step
conditions, a separate resolution sensitivity.

## Verdict and limits

**CLOCK-SENSITIVE (screening).** A claim that the frozen ecology has a stable
perp-basis mechanism, a particular cross-venue opportunity rate, or a timing
independent volatility-clustering estimate is unsupported. This does not
identify an individual clock or establish an LCM mechanism. There are only two
seeds and five-hour horizons. A later factorial clock experiment must isolate
the implicated cadence classes rather than treating this whole package as one
mechanism.

All six worlds have zero order-lifecycle evidence-contract failures: no missing
terminal immediate order, post-terminal fill, quantity mismatch, or unknown
lifecycle record. This is separate from financial conservation and does not
prove that an unlogged GTC cancellation request was honored.

## Provenance

Raw logs remain under `logs/clock_*`; no clock evidence has been pruned.
Per-run metric bundles, manifests, latency sidecars, lifecycle audits, and
unordered persisted-evidence digests are under
`research/artifacts/clock-ae13f9a/`. The machine-readable index is
[`clock-ae13f9a.json`](artifacts/clock-ae13f9a.json). Simulator freeze:
`ae13f9aa6e5fd23539637a8c4a3d2d4f4c3ad107`; simulator binary SHA-256:
`d6565a49d99514a2faa6f92e1d07a949abc1c2ca1099d226ea518bb01e1212c9`;
analysis binary SHA-256:
`4d538cdd02714b9b53323c068e2c96d1610983490b5ef086be3b9d51373da307`.

## Factor follow-up — timing sensitivity is confirmed; individual clock cause is unresolved

The preregistered follow-up in
[`clock-factorial-plan-ae13f9a.md`](clock-factorial-plan-ae13f9a.md) holds the
100ms runner step fixed and compares every arm with the retained 100ms-step
pair. Its original screen midpoint rule is `(step_100ms + destagger) / 2`:
101.91 bps for seed 101 and 95.58 bps for seed 103. Every world has full raw
evidence, an offline persisted-record digest equal to its runtime attestation,
and zero order-lifecycle failures.

| arm | seed 101 abs basis bps / half-life s | seed 103 abs basis bps / half-life s | midpoint rule | reading |
|---|---:|---:|---|---|
| 100ms step reference | 201.01 / 1121.84 | 188.78 / 283.19 | — | reference |
| all-clock destagger | 2.80 / 2.27 | 2.37 / 5.87 | — | package endpoint |
| publication (`snapshot` only) | 71.33 / 1112.83 | 4.11 / 3.72 | pass / pass | stage-one package implicated |
| maker-flow package | 2.36 / 3.44 | 2.48 / 3.55 | pass / pass | stage-one package implicated |
| risk/options/carry package | 34.14 / 223.61 | 21.09 / 57.04 | pass / pass | stage-one package implicated |
| quote only | 90.99 / 495.70 | 185.00 / 629.30 | pass / fail | **unresolved** |
| dated-carry only | 116.27 / 330.12 | 76.28 / 249.71 | fail / pass | **unresolved** |

The first-stage packages all crossed their declared two-seed primary threshold,
but the two narrowest follow-ups disagree by seed. Therefore neither quote
refresh nor dated-carry scan cadence is supported as a standalone cause. The
defensible conclusion is **cadence-lattice sensitivity with an unresolved
interaction**, not three independent economic anchors and not a demonstrated
LCM mechanism.

This qualification matters: changing an interval alters both its phase
relationship and how often an actor executes. The frozen configuration has no
initial-phase/offset control, so it cannot isolate an LCM explanation without a
new simulator timing feature and hence a new freeze. In particular,
`DatedCarryCheckInterval=0` defaults to `QuoteInterval`; the code explicitly
warns that this phase-lock can make the desk scan while makers have cancelled
and not yet replaced. The dated-carry-only result shows that this observation
is not sufficient by itself across the paired seeds.

Secondary outcomes remain adverse: all ten worlds retain a strongly negative
1s raw-return ACF (−0.4475 to −0.5508), while triangular mean edge stays in
the thousands of bps. No cadence variant repairs either frozen stylized-fact
failure. The compact machine-readable table is
[`clock-factor-ae13f9a.json`](artifacts/clock-factor-ae13f9a.json); raw logs
and detailed metrics remain retained under `logs/clock_factor_*` and
`research/artifacts/clock-factor-ae13f9a/`.
