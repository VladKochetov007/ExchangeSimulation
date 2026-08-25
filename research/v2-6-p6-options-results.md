# V2-6 P6 staged-options development result

Status: **development screen incomplete; no holdouts authorized**.

This file records the preregistered P6 development cells without changing the
preregistration or selecting a convenient subset of outcomes.  The immutable
stage contract is in `v2-6-p6-options-causal-preregistration.md` and the
numeric choices are in `v2-6-p6-options-numeric-addendum.md`.

## Provenance

The seven completed cells were run with the full evidence contract and
extracted by `scripts/extract-v2-6-p6-cell.sh` at analysis revision
`cf13deef4ecd45c6c5cc3827e2365f80b97a8581`.  The analyzer binary SHA-256 was
`5e18884fcbcc4a6be13f3447d8260855acec6cdf7d93276db29e40832b531281`.
`scripts/check-v2-6-p6-configs.sh` verified every cell's immutable config
hash and the declared stage-only delta before the worlds were launched.

Each `OK` cell has:

* final `greeks.json` and `latency.json` completion sentinels;
* all 18 registered metric artifacts and complete `analysis-metadata.json`;
* receipt/frontier, conservation, position/fill, lifecycle, settlement and
  expiry checks passing;
* exact equality of run-time and offline persisted-evidence event count and
  digest.

The runtime evidence digest is the persisted unordered-multiset digest.  It is
not an execution-domain hash and is reported separately by the evidence
contract.

## Cell ledger

| cell | status | evidence events | persisted-evidence digest | config SHA-256 |
|---|---|---:|---|---|
| O0-211 | complete / valid | 31,241,189 | `31c6c0044d4020b3c9ea78d9db1627e0fb034b92aa59f4453cb9d6bff32b85a5` | `f8421827eb23314a9988678443f9da2da3dcb6dabf2c98121f562804389559b8` |
| O0-213 | complete / valid | 30,641,101 | `2a3e31d0ab9ac997be6b3c851f4673e01fde2545fa6e444c5bca2db4b50021d9` | `d55dc184495dbadea930be0f269f5a5ca931fa0660e450e193422fb543294ccb` |
| O1-211 | complete / valid | 34,820,083 | `c48b97b9efe801b78fae9df2ff4e080206a762ff2a8012b9cf855e181ff67837` | `7e519ee2a55010d5189bc4234d3e6385e3bcb1398a9dd02ff6950a3fbab79106` |
| O1-213 | complete / valid | 34,952,873 | `7bc320dc99cbbd2e18d1c6905d2df04189673b124a98751af83267bb0841a58b` | `5036d62929e5fd76bf920ee371826d73c2506c8a65c2329c181947637becad80` |
| O2-211 | complete / valid | 34,749,895 | `6dcbf17604a670a66355aafaef85ed4c9c3954a3123a24c28df8bb7142900c1c` | `d859f1c9c6ab319175ae3ee008285f97becd2e77e08f203cc863e947860b1b7d` |
| O2-213 | complete / valid | 32,693,986 | `d120315aa78126f873bd1e4a3856c0ba577361a6294cf3495dd79ddca3e40d74` | `ee1e9ef750392ff0328200e9c9b75940e5dc8df2798fd57fd05bedd2722f0f24` |
| O3-213 | complete / valid | 33,783,047 | `d1d1db482ad57ba11e72676459072e2337dd9e5ebe93aaeb6741454ec3deae64d` | `0082c72729e5fd76bf920ee371826d73c2506c8a65c2329c181947637becad80` |

The following launch attempts are retained under
`research/artifacts/historical/v2-6-p6-incomplete/` and are **not evidence**:

| cell | terminal result |
|---|---|
| O3-211 | strict dealer risk capture failed at central because no usable two-sided CDF/USD mark remained |
| O4-211 | strict dealer risk capture failed at north because no usable two-sided CDF/USD mark remained |
| O4-213 | strict dealer risk capture failed at north because no usable two-sided CDF/USD mark remained |

The failures are not analyzer or logging failures.  The retained CDF/USD
snapshots show the affected venue with a non-empty bid and an empty ask near
termination (O3-211 central at `1735718389000000000`; O4-211 north at
`1735718397000000000`; O4-213 north at `1735718387000000000`).  The event
stream also records explicit `price_unavailable` collateral/order-admission
events.  The current multivenue contract intentionally rejects valuation when
the bounded two-sided mark is unavailable; no implicit zero, last-trade or
cross-venue fallback was introduced.  This is a population-viability
limitation under the inherited cross-asset mark contract.

## Registered activation scores

Scores below are activation/integrity scores only.  They are not realism,
profitability, or market-causality claims.

### O0 — simple flat reference

**SUPPORTED (screening) for activation/integrity in both development seeds.**
Each seed has 150 market-price option points, 73,192/73,322 independently
priced observations, 15 fitted expiries and 160,268/161,692 canonical
option-dealer maker fills.  The five short-option generations per venue are
listed, settled and reconstructed with zero post-expiry fills.  A canonical
maker fill is evidence that a dealer quote was admitted and met by an
opposing order; it is the operational two-sided-quote evidence available in
the registered artifacts.

### O1 — heterogeneous flat-vol dealers

**SUPPORTED (screening) for activation/integrity in both development seeds.**
The three-dealer roster is active (245,566 and 247,272 canonical dealer maker
fills), and the option surface remains independently priced with 150 points,
15 fitted expiries and 154,036/155,695 priced observations.  The descriptive
surface values are retained below; no post-outcome threshold was applied.

### O2 — liability demand plus dealer delta hedge

**SUPPORTED (screening) for participant activation and evidence integrity in
both development seeds.**  The liability user produced 17,280 local decisions
per seed, 59/64 admitted orders, and 43/44 canonical fill records, reaching
the registered 300,000,000 raw-unit target in every venue (100,000,000 per
venue).  All decision/fill joins passed with zero future-observation,
missing-outcome or mismatch errors.  Each seed has non-zero exchange-owned
dealer option inventory and non-zero hedge-tagged underlying flow:

| seed | pooled hedge ratio | pooled mean abs net delta | pooled max abs net delta | pooled option/underlying volume correlation | hedge-tagged underlying volume |
|---:|---:|---:|---:|---:|---:|
| 211 | 1.0009521815 | 0.0075035026 | 0.1184106443 | 0.0182867934 | 38,345,478,321 |
| 213 | 1.0000930601 | 0.0070023414 | 0.1099478552 | 0.0146108637 | 39,317,238,853 |

The hedge activity gate is therefore exercised.  The preregistration says
that a directional transmission score must have a *registered sign*, but it
does not state that sign or a threshold.  Consequently the directional
component is **NOT IDENTIFIED** rather than retroactively being declared
positive merely because both observed correlations are positive.  The raw
correlations and all supporting exposure fields remain available for a future
properly preregistered test.

### O3 — minority SABR-view value taker

**NOT EXERCISED as a paired development stage.**  O3-213 independently shows
activation: three value-taker participants, 10,956 decisions/admitted orders,
16,731 canonical option fills, and active O2 liability/hedge activity.  The
paired O3-211 world did not complete because strict central CDF/USD mark
capture failed.  Therefore no O3−O2 causal or holdout claim is licensed.

### O4 — Vanna–Volga transfer

**NOT EXERCISED.**  Neither registered seed completed the evidence contract;
both stopped at strict CDF/USD mark capture (north).  No VV activation or
surface attribution may be scored.

## Descriptive surface contrasts (not causal claims)

These are the preregistered raw surface summaries from independently inferred
market prices.  O3 values are shown only for the one valid seed and O4 has no
valid cell.

| cell | ATM IV | slope | curvature | cross-strike dispersion | trades | priced |
|---|---:|---:|---:|---:|---:|---:|
| O0-211 | 2.498944 | 0.555312 | 924.320899 | 0.325938 | 160,268 | 73,192 |
| O0-213 | 2.511146 | 1.016757 | 920.381661 | 0.346261 | 161,692 | 73,322 |
| O1-211 | 1.978976 | 1.029850 | 1,233.047522 | 0.424521 | 245,566 | 154,036 |
| O1-213 | 1.973338 | 0.906718 | 1,274.554393 | 0.439467 | 247,272 | 155,695 |
| O2-211 | 1.978648 | 1.479763 | 1,231.842311 | 0.424587 | 251,628 | 156,390 |
| O2-213 | 1.972958 | 0.906808 | 1,264.582901 | 0.437695 | 243,031 | 154,045 |
| O3-213 | 1.768740 | 1.571462 | 1,360.986809 | 0.465731 | 258,912 | 178,312 |

For reference, the paired raw differences for the valid stages are:

| seed | contrast | ATM IV | slope | curvature | dispersion |
|---:|---|---:|---:|---:|---:|
| 211 | O1−O0 | -0.519968 | +0.474539 | +308.726622 | +0.098584 |
| 213 | O1−O0 | -0.537808 | -0.110039 | +354.172731 | +0.093206 |
| 211 | O2−O1 | -0.000328 | +0.449913 | -1.205210 | +0.000066 |
| 213 | O2−O1 | -0.000380 | +0.000090 | -9.971492 | -0.001772 |
| 213 | O3−O2 | -0.204218 | +0.664654 | +96.403908 | +0.028036 |

The O3 contrast includes an explicit SABR prior and is therefore inherited
structure, not an emergent-smile claim.  The O1/O2 contrasts are descriptive
only because the preregistered protocol did not fix effect-size corridors.

## Gate decision and next action

The evidence and activation gates are valid for O0/O1/O2, and the one valid
O3 cell is retained as a partial activation observation.  The development
stage screen as a whole is incomplete because O3 is not paired and O4 has no
completed seed.  Untouched holdout seeds 223/227/229 are therefore **not
authorized**.  No funding, liquidity, population, clock, or mark parameters
were changed to rescue the failed cells.  The next research decision must
address the inherited cross-asset mark viability as a separate, explicitly
preregistered mechanism question; it must not silently convert these incomplete
worlds into P6 evidence.
