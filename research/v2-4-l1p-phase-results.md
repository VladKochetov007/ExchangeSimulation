# V2-4 L1-P results — CDF/USD liability-hedger phase screen

Status: **SUPPORTED (screening), narrow phase-robust local-motive contract.**
The registered criterion is satisfied at the tested half-period offset: in all
four full-evidence cells every exercised delivery-liability fill reduced its
own independently reconstructed absolute delivery gap, every evidence contract
passed, and every CDF/USD venue cleared the preregistered non-collapse floor.

This is not a claim that the whole ecology is phase-insensitive. The
preregistered descriptive diagnostics move substantially and consistently when
only this phase changes. That is an unresolved clock-interaction discovery
candidate, not a post-hoc directional score or justification for retuning.

## Contract and provenance

[`v2-4-l1p-phase-preregistration.md`](v2-4-l1p-phase-preregistration.md)
fixed the parent, policies, populations, prices, fees, latency, intervals, and
all clocks before this implementation or campaign. P0 and P1 both inherit
L1-B's `delivery_liability` policy. The sole behavioural config delta is:

| arm | decision phase | first liability decision |
| --- | ---: | ---: |
| P0 | 0 ns | simulation start + 2 s |
| P1 | 1,000,000,000 ns | simulation start + 3 s |

The immutable P0/P1 config comparison is empty after removing declared
provenance and `cdf_liability_hedger.decision_phase_offset`.

Implementation revision `af7a284` adds the deterministic phase capability:
zero delegates through the legacy ticker path; a nonzero offset changes only
the first deterministic periodic event and consumes no RNG. `160508a` adds a
fresh-process equivalence test proving explicit zero phase and the otherwise
identical legacy L1 delivery configuration have the same execution hash at
GOMAXPROCS 1 and 4. The nonzero-phase fresh-process evidence-on/off matrix also
has one execution hash across GOMAXPROCS 1 and 4; its independent replay
validates every declared phase timestamp. The focused phase race test passed
in 88.527 s.

The four 30-minute full-evidence cells ran from source revision
`160508a73f08d17679fe9d9e72713d7197fa0dc5`, with `GOMAXPROCS=3` and
`multivenue` SHA-256
`cfc920ce12c1aa9790bf43424418fdcb921a5dbbebcd2f22c4109550523515e1`.
The final analysis revision is the same source revision and `mvanalyze`
SHA-256 is `09a66168f74a91c48655ea4680542e552aecde6950ca7414de10dfc0a518b93f`.
Each cell became complete only when its final `greeks.json` and `latency.json`
sidecars existed; raw evidence remains retained under
`research/artifacts/v2-4-l1p/{P0,P1}/seed-{101,103}`. No L1-P evidence is
prunable.

## Evidence and activation gate

Every cell has a valid V2 observation receipt/frontier audit, an independently
recomputed persisted-evidence artifact digest, a valid policy/state/fill/phase
replay with zero checks, all three slots at 180 state updates, at least one
accepted request per slot, and a passing post-warmup CDF/USD non-collapse
floor.

| arm / seed | evidence events / digest prefix | receipt schedule / receipt / decision | phase evidence | policy decisions / updates | accepted / fills | floor |
| --- | --- | ---: | --- | ---: | ---: | --- |
| P0 / 101 | 2,398,538 / `2a059d0f` | 484,185 / 483,954 / 54,435 | 0 ns, valid | 2,700 / 540 | 1,982 / 1,944 | pass |
| P0 / 103 | 2,396,423 / `8b5a7f1f` | 485,679 / 485,540 / 59,138 | 0 ns, valid | 2,700 / 540 | 2,014 / 2,068 | pass |
| P1 / 101 | 2,356,591 / `d8e5d886` | 474,512 / 474,293 / 52,008 | 1 s, valid | 2,697 / 540 | 1,797 / 3,995 | pass |
| P1 / 103 | 2,310,735 / `da058fbf` | 466,090 / 465,967 / 52,514 | 1 s, valid | 2,697 / 540 | 1,690 / 3,802 | pass |

The replay found zero future decision uses, missing due receipts,
receipt-without-schedule rows, decision-without-link rows, bad decision
frontiers, phase-field mismatches, off-phase decision records, or nonreducing
delivery fills. P1 has 2,697 rather than 2,700 decisions because the declared
one-second phase moves one initial decision per venue beyond the fixed
30-minute horizon; its obligation updates remain 180 per venue.

Every CDF/USD venue passes at least 150 trades, two taker roles, one maker
role, and 95% two-sided post-warmup snapshots. The lowest observed two-sided
share is 96.78% (P0/101 central).

## Registered local-motive score

The exact sums below use integer raw units and are retained with their sample
counts. Displayed means and paired deltas are descriptive decimal renderings;
no floating state enters the replay.

| seed | P0 exact mean absolute gap | P1 exact mean absolute gap | P1 − P0 | P0 fills reducing / nonreducing | P1 fills reducing / nonreducing |
| --- | ---: | ---: | ---: | ---: | ---: |
| 101 | 898,726,083,300 / 2,700 = 332,861,512.333 | 248,170,300,184 / 2,697 = 92,017,167.291 | −240,844,345.042 | 1,944 / 0 | 3,995 / 0 |
| 103 | 1,310,482,630,795 / 2,700 = 485,363,937.331 | 235,658,169,214 / 2,697 = 87,377,889.957 | −397,986,047.374 | 2,068 / 0 | 3,802 / 0 |

The registered criterion therefore holds: the local delivery policy remains
independently correct and the non-collapse floor survives the tested phase.
This supports the bounded statement that the L1 local motive is not an
artifact of **only** the original all-zero liability phase.

## Clock finding and limits

P1's lower mean gap and larger fill count are a sizeable, same-direction
descriptive change in both paired seeds. The screen had no preregistered
directional threshold for gap, fills, price, spread, volume, wealth, or
stability, so these values cannot be promoted to a causal market claim here.
They do, however, falsify the casual assumption that this one half-period
phase has negligible downstream effects.

Consequently this result does **not** license:

- an ecology-wide phase-robustness claim;
- price-stability, demand-elasticity, or `noise_flow`-replacement inference;
- coefficient, spread, frequency, latency, population, or inventory tuning;
- L2 roster demotion based on apparent P1 improvement.

The next high-information gate is a separately preregistered L1-P2
phase-decomposition screen. It must retain the L1 policy and population,
replicate both phases in additional holdout seeds, then vary only one declared
periodic relationship at a time to identify whether the descriptive effect is
caused by a liability decision/maker refresh alignment, an obligation-clock
alignment, a latency delivery boundary, or another periodic channel. Until
then the only supported conclusion is the narrow local-motive result above.
