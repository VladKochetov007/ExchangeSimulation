# V2-1d — heterogeneous informed-maker roster preregistration

## Question

Can a small maker roster retain genuinely distinct participant-local public
information sets without recovering the ae13f9a shared consensus index by
configuration accident?

## Local mechanism

Each declared target is one ABC/USD maker, identified by venue and its
one-based maker ordinal. It keeps its delayed local cache plus one feed-only
remote source. The feed has an independent deterministic latency profile,
source weight/confidence, and maximum usable publication age. The composite is
withheld when either required cache is unavailable or too stale. The other
ABC/USD maker at each venue remains local-only.

`weight * confidence` is the remote component's effective mixture weight;
the complement remains the local component. These are policy inputs, not a
global reference or an outcome target.

## Fixed smoke roster

| target maker | remote source | weight | confidence | maximum source age | feed delay |
|---|---|---:|---:|---:|---:|
| `north`, 1 | `south` | 0.50 | 0.80 | 2 s | 10 ms |
| `central`, 1 | `north` | 0.35 | 0.90 | 4 s | 20 ms |
| `south`, 1 | `central` | 0.45 | 0.60 | 6 s | 30 ms |

All primary `spot_maker` feeds use a constant 10 ms delay. Maker anchor is
explicitly `own_mid`; shared index anchoring is prohibited. This roster is a
construction test and uses a 20-second smoke with seed 101, followed by
fresh-process two-minute evidence controls. It is not a calibration choice or
a price-discovery experiment.

## Predictions and kill criteria

| check | prediction | failure interpretation |
|---|---|---|
| configuration | exactly three distinct target makers and feed sessions; no duplicate target | NOT IDENTIFIED; roster mapping ambiguous |
| activation | each target cache has its declared source and nonzero updates | NOT IDENTIFIED; intended information path inactive |
| staleness | a deliberately expired remote observation suppresses that maker's composite | cache horizon is decorative; reject mechanism |
| evidence | every target scalar order has exactly one V3 vector with local + declared remote receipt prefixes | evidence gap; stop before economics |
| determinism | exact execution hash OFF/ON and across GOMAXPROCS 1/4 | scheduler/instrumentation defect |
| feed isolation | remote session accepts no order request | invalid information boundary |

## Explicit non-claims

This test does not predict lower dispersion, improved realism, or any effect
on a stylized fact. Those require the later informed-maker/explicit-arbitrage
2x2 with fixed population, fees, and latency distributions.

## Result

All preregistered construction checks passed in the 20-second seed-101 smoke.
There are exactly three feed-only sessions and exactly three activated remote
caches, with declared source identity and nonzero updates. The local-only
makers have no remote cache. Target makers emit quotes, scalar V2-0 evidence is
valid, and every target gateway decision has one two-component V3 vector.

Fresh two-minute processes at GOMAXPROCS 1 and 4 have one execution hash with
evidence OFF and ON; evidence-on scalar and vector counts/digests match across
the two processor settings. This promotes the roster's provenance contract,
not any market outcome.
