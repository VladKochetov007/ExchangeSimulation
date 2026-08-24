# V2-4 L1-P2 — liability/noise relative-phase result

The preregistered 30-minute full-evidence 2×2 screen is **SUPPORTED
(screening)** for one narrow claim: in this retained V2-4 L1 population, the
relative phase of the declared CDF/USD delivery-liability hedger and the broad
`noise_flow_*` population changes the registered local-gap endpoint.

This is not an economic tuning result, price-stability claim, or an
ecology-wide timing-robustness claim.

## Provenance and evidence gate

The scored cells are A/B/C/D × seeds 101/103, each at 30 minutes with full
raw evidence. They ran from `a6daf93d5db8046d7ef63a37c215afb8fb4bf96c`,
using multivenue SHA-256
`c4bdcc1a283b3d9ed82ee41c2775720f3795ff22f6b1af9d6e5ca4b87238dee2`
and `GOMAXPROCS=3`. Completion was determined only from final `greeks.json`
and `latency.json`; raw logs remain under
`research/artifacts/v2-4-l1p2/{A,B,C,D}/seed-{101,103}` and are not prunable.

An initial background-launch attempt died at the tool-shell boundary before
either completion sidecar existed. It is retained as `NON_EVIDENCE` at
`research/artifacts/historical/v2-4-l1p2-attempt0-shell-terminated` and is
not pooled with the scored cells.

All eight scored cells have:

- valid V2 receipt evidence and persisted-evidence artifact hashes;
- valid independent liability policy/state/fill replays;
- valid independent noise timing replays;
- 540 liability state updates, zero nonreducing liability fills, and a passing
  CDF/USD post-warmup non-collapse floor; and
- exactly 18 `noise_flow_*` actors. Zero noise phase gives 900 ticks per actor
  (16,200 rows); one-second noise phase gives 899 (16,182 rows), as required
  by the fixed 30-minute horizon.

The compact cell-level provenance, raw integer aggregates, and digests are in
`research/artifacts/v2-4-l1p2/l1p2-summary.json`.

## Registered endpoint

`M = absolute_gap_sum / gap_samples` at the liability decision times. Values
below preserve the exact numerator and denominator; decimals are descriptive.

| seed | A: L0/N0 | B: L1/N0 | C: L0/N1 | D: L1/N1 |
| --- | ---: | ---: | ---: | ---: |
| 101 | 898,726,083,300 / 2,700 = 332,861,512.333 | 248,170,300,184 / 2,697 = 92,017,167.291 | 239,882,748,070 / 2,700 = 88,845,462.248 | 367,839,888,814 / 2,697 = 136,388,538.678 |
| 103 | 1,310,482,630,795 / 2,700 = 485,363,937.331 | 235,658,169,214 / 2,697 = 87,377,889.957 | 260,038,590,689 / 2,700 = 96,310,589.144 | 675,072,737,350 / 2,697 = 250,305,056.489 |

Here `L0/L1` are the liability phase (0/1 second) and `N0/N1` are the broad
noise-flow phase (0/1 second). A and D are aligned; B and C are de-aligned.

| seed | aligned mean | de-aligned mean | aligned − de-aligned | interaction `(D−C)−(B−A)` |
| --- | ---: | ---: | ---: |
| 101 | 234,625,025.506 | 90,431,314.770 | +144,193,710.736 | +288,387,421.472 |
| 103 | 367,834,496.910 | 91,844,239.551 | +275,990,257.360 | +551,980,514.719 |

The exact common-denominator numerators are retained in the machine artifact.
Both seeds satisfy the preregistered direction and positive-interaction test.
Therefore the registered verdict is **SUPPORTED (screening)**.

## Interpretation and limits

The result identifies the broad `noise_flow_*` two-second decision cadence as
a causal counterpart for the L1-P phase contrast in this ecology. It does not
identify why the interaction is large, nor reduce the effect to a single LCM
story: the two aligned cells differ materially from each other, so absolute
phase and other fixed clocks may still matter.

It does not license price/spread/volume/wealth conclusions, a claim that this
is realistic demand, demotion of the legacy noise population, or economic
parameter tuning. Two seeds remain screening evidence only. The next gate is
a fresh 2×2 replication on untouched holdout seeds, retaining the exact
population and evidence contract before any roster-replacement decision.
