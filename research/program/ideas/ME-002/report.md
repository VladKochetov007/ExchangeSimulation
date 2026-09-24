# ME-002 — network × actor-processing development response map

Status: **REVIEWED DEVELOPMENT RESPONSE MAP**. All 24 registered economic
cells and two fresh-process controls completed. Both bounded post-result
reviews accepted their respective scopes after reporting corrections. This is a
simulation-internal development screen, not confirmation, empirical validation,
or a claim that latency generally helps or hurts execution.

## 1. Question and bounded conclusion

For one fixed immediate ABC/USD buy policy in the C0=4-maker/8-random-taker
ecology, how do synthetic directed network delay and actor-side processing
delay change filled fraction at two target quantities? The registered primary
endpoint is filled ABC divided by assigned target ABC, for every valid assigned
world. Network delay moved the order's actual venue-arrival timestamp in all
12 matched pairs, but its 5-ABC fill effect changed sign across the three
seeds. Processing changed which snapshot was selected in all 12 matched pairs,
yet the 1-second earliest-decision gate held order arrival and fill quantity
unchanged. At 0.5 ABC, all worlds fully filled, a ceiling for this endpoint.
These observations do **not** establish general network or computation-latency
irrelevance, equivalence, geography effects, or profitability.

## 2. Scope, mechanism and competing explanations

The [prospective protocol](protocol.md) and [operational lock](development-lock-2026-09-23.md)
fix one ABC/USD price-time venue, one immediate BUY parent, 12 background
actors, identical focal policy, capital, client ID 13, fees, target and noise
settings within each matched quartet. The focal policy polls each 1 ms, but
cannot decide before 1 simulated second; each world lasts four simulated
seconds. The 2×2 deployment factors are fast/slow network (1/90 ms on each
feed, request and response leg) and fast/slow actor processing (0/120 ms).
Targets are 0.5 and 5 ABC; seeds are 12001, 12011 and 12017. Delays are
synthetic logical-time assignments, not measured geographic links or host CPU
times. Processing is a fixed pipeline wait, not a serial computation queue.

The prediction was that longer realized action delay could miss short-lived
displayed ask depth, especially at 5 ABC. Relevant alternatives are the fixed
decision gate, snapshot sampling, phase, a changed information selection but
unchanged request time, endogenous book response, and a fill ceiling at the
small target. Same seed is a paired design label; no claim is made that all
background random draws are globally coupled after intervention.

## 3. Distinct provenance and historical context

Execution/protocol candidate: `e042e685dd82ee06d8f2d49c52e8c219a7f58091`,
tree `6c2d5f6214393d65090d7bfcc4e8663bcf8b28a7`, Go 1.27.0,
`latency-pilot-opaque-v1` evidence schema. Clean simulator SHA-256:
`4cf2ace97d9ad0dc6346d0138695adb082be130cb42100a59f7d92015c5b786c`;
analyzer SHA-256:
`b6f35a4fbfc08a8d64d904229af122f7fa13b0baf9bcc4d60ac68fd4778a003d`.
The locked protocol content is the blob at that candidate, not this later
report revision. The all-assigned Go aggregator is `19e5835105fc1c4f3ec30577fe3cbc7219ec8c6c`
(binary SHA-256 `4a60cb49dc98e608b0a057423452d24aca9c8f731862385c81184f9c68f84a01`);
the separate Go timing diagnostic is `ec6045277a53a266d1b443f1d6cf017769df1009`
(binary SHA-256 `7161d64573d9cff7fe7b6cdcd5e383a373df6f3bfa15f745a62b598be1524403`).
Skill commit/tree at lock: `a6afaa905b8c2258c175189bd39106d246ebfdb9` /
`ee529dd73d061b43717e57d8b7fa832543f0e5a2`.

ME-001's 27-world composition screen is a distinct development population.
Historical timing and R2/SV1D results retain their own source, analyzer and
verdicts; no historical result is promoted by this deployment experiment.
At `7c7b7e6`, two raw technical controls were byte-identical but an analyzer
wire-decoding defect prevented promotion. Their raw artifacts and failed
attempt remain at their original namespace. The corrected `e042e68` candidate
passed focused and full tests, vet and targeted race checks; a fresh bounded
Sol-6 medium review accepted that technical correction and the prospective
execution gate. It did not pre-accept the outcome interpretation here.

## 4. Formula, units, opportunity denominator and censoring

The primary response is `Y = reconstructed filled base units / assigned target
base units`; one ABC is 100,000,000 base units. A valid no-send, rejected or
accepted-unfilled world would have `Y=0`; invalid evidence would be
`UNASSESSABLE`, never silently omitted. For each paired seed quartet and target, the registered
network effect is `((S/F + S/S) - (F/F + F/S))/2`, processing effect is
`((F/S + S/S) - (F/F + S/F))/2`, and interaction is
`S/S - S/F - F/S + F/F`. Effects below are fraction units; multiply by 100
for percentage points. The three paired seed quartets, not fills or snapshot
rows, are the uncertainty units; their observed range is not a population
bound. No p-value or equivalence margin was registered.

The evidence joins exchange publication to focal receipt, processing,
selection, decision, request send, venue arrival/admission/fill, delayed
response, ledger and terminal mark. The selected-message
publication-to-arrival interval includes inbound delay, processing,
processing-to-decision wait **including** the 1-second gate, and outbound
delay. It is not pure CPU or pure poll wait. Qualifying displayed depth is a
sampled public-snapshot proxy, not continuous executable depth at later
arrival. An episode without both observed start and end is censored; its
duration ratio is undefined. A nonqualifying *selected* snapshot does not
prove there was no later trading opportunity or execution.

## 5. Retained evidence and offline reproduction

External namespace: `/home/vlad/ExchangeSimulation-me002-development-e042e68`.
It retains locked plans, 26 run directories, individual analyzer results,
process resource stamps, pinned binaries, and the verified all-assigned
`analysis/latency-surface.json` (SHA-256
`a4d0effb4d8a7a0123221c7971f45a281cff72a56d8f9346df5d29df8afe3b6f`).
The Go aggregator verified all 24 plans/manifests/file hashes, independently
replayed raw evidence, and compared reconstructed and stored actor/outcome
records. The derived `analysis/latency-diagnostics.json` is SHA-256
`c08fc6fec3e9e05cc22ad2e1d4c1eb8cf530b955ced9bd15fd0a492090561d1a`
and binds the surface hash. External location does not imply remote backup.

To reproduce the *offline* aggregate in a clean checkout of `19e5835`, build
`./cmd/melatencysurface` using Go 1.27.0, compare its binary SHA above, and
write only to a new output path:

```bash
melatencysurface -root /home/vlad/ExchangeSimulation-me002-development-e042e68 \
  -repo <clean-19e5835-checkout> \
  -execution-source e042e685dd82ee06d8f2d49c52e8c219a7f58091 \
  -out <new-surface-path>
```

For the derived timing table, build `./cmd/melatencydiagnostics` at `ec60452`
and run it against that verified surface with `-surface <surface-path> -out
<new-diagnostics-path>`. This is retained-evidence analysis, not a new market
world. Do not overwrite these artifacts or rerun a valid unfavorable seed.

## 6. Execution inventory and resources

Fresh C=`e042e68` F/F target-0.5 seed-12001 controls, invoked in the research
session as `GOMAXPROCS=1` and `7`, each exited zero, reconstructed
`FULLY_FILLED`, and produced byte-identical
evidence SHA-256
`6493d699b97de2ef887bbce6372fcca2b570d933d8996b0a7e3b8a1a0be2c5e1`,
canonical execution hash
`3361f1267a94aed9aaa166534395ec5319625aabf280d5197327ac88e4ddb40c`
(17,858 frames), and result SHA-256
`a27699b86e055ff55e8ba9fc84ac7cd2ca02bb5baa23f73d8044ebb5c16ba88b`.
Control wall times were 0.40/0.34 s and peak RSS 39,800/33,024 KiB. The
retained run manifests bind files and identities but are **not cryptographically
signed** and do not contain process settings. No separate invocation record was
found in the retained namespace; the `GOMAXPROCS` settings are a session-level
account, not independently attested by those artifacts.

The recorded procedure submitted the 24 economic cells sequentially with
`GOMAXPROCS=7`, 30-s per-world timeout and 15-minute total cap; the retained
manifests establish 24 unique assigned outputs but do not independently prove
that process setting, execution order, or absence of any earlier attempts.
Simulator and pinned analyzer exited zero for every retained cell; no invalid
or incomplete retained cell was excluded. Economic evidence totals 122,855,172 bytes, measured world wall
time totals 7.93 s, maximum world wall time is 0.34 s and maximum process RSS
is 34,304 KiB. The namespace occupied about 150 MiB at closeout check. All
24 terminal marks were defined. No confirmation or holdout world ran.

## 7. All-assigned outcomes and registered contrasts

Seed order is `12001 / 12011 / 12017`. Entries are filled fractions, not
percentages. `F/S` means fast network, slow processing.

| Target ABC | F/F | F/S | S/F | S/S | Full completion by arm |
|---:|---|---|---|---|---|
| 0.5 | 1 / 1 / 1 | 1 / 1 / 1 | 1 / 1 / 1 | 1 / 1 / 1 | 3/3, 3/3, 3/3, 3/3 |
| 5 | 1 / 1 / 0.950849246 | 1 / 1 / 0.950849246 | 0.986290860 / 0.962183708 / 0.985569136 | 0.986290860 / 0.962183708 / 0.985569136 | 2/3, 2/3, 0/3, 0/3 |

| Target ABC | Seed | Network effect | Processing effect | Interaction |
|---:|---:|---:|---:|---:|
| 0.5 | 12001 / 12011 / 12017 | 0 / 0 / 0 | 0 / 0 / 0 | 0 / 0 / 0 |
| 5 | 12001 | −0.013709140 | 0 | 0 |
| 5 | 12011 | −0.037816292 | 0 | 0 |
| 5 | 12017 | +0.034719890 | 0 | 0 |

At 5 ABC, the network-effect three-seed median is −0.013709140, observed
range −0.037816292 to +0.034719890 (−3.782 to +3.472 percentage points).
The mixed sign does not support a uniform slower-network-worsens-fill claim.
The 0.5-ABC all-full ceiling cannot identify small effects below completion.
These are development-world contrasts, not out-of-sample uncertainty bounds.

| Target ABC | Effect | Three-seed median | Observed paired range |
|---:|---|---:|---:|
| 0.5 | Network | 0 | 0 to 0 |
| 0.5 | Processing | 0 | 0 to 0 |
| 0.5 | Interaction | 0 | 0 to 0 |
| 5 | Network | −0.013709140 | −0.037816292 to +0.034719890 |
| 5 | Processing | 0 | 0 to 0 |
| 5 | Interaction | 0 | 0 to 0 |

All 24 requests were admitted and filled at least partly; none was rejected
or no-send. At 0.5 ABC every residual is zero and each first/last venue fill
is at 1.001 s in fast-network arms or 1.090 s in slow-network arms. At 5 ABC,
all first/last venue fills have those same arm-specific timestamps and each
cell has 20 venue fill records. The following secondary outcomes retain all
partial cells; seed order is again `12001 / 12011 / 12017`. Quote fees use
the locked quote-asset fixed-point units, not ABC.

| Network arm (both processing levels) | Unfilled ABC by seed | Target shortfall bp by seed | Quote fees by seed |
|---|---|---|---|
| Fast | 0 / 0 / 0.24575377 | 15.005 / 15.005 / 14.6608952 | 12,512,500 / 12,512,500 / 11,897,992 |
| Slow | 0.06854570 / 0.18908146 / 0.07215432 | 14.909022 / 14.7402476 / 14.9039692 | 12,341,101 / 12,039,701 / 12,332,078 |

For example, fast-network seed 12017 leaves 0.24575377 ABC unfilled while
its marked target shortfall is 14.6608952 bp, below the 15.005 bp of fully
filled fast-network seeds. The cheaper mark is **not** completed execution:
the residual is valued at terminal midpoint and receives no hypothetical
taker fee. Every terminal mark is defined, so no secondary shortfall cell is
missing; none of these shortfall values is a profitability measure.

## 8. Timing and opportunity diagnosis

All 24 focal decisions occurred at exactly 1 simulated second. Among 12
processing-only matched pairs, the selected snapshot changed 12/12, but order
arrival changed 0/12 and filled quantity changed 0/12. Among 12 network-only
matched pairs, order arrival changed 12/12 and filled quantity changed 6/12.
Median selected-publication-to-arrival intervals by arm are F/F 101 ms, F/S
201 ms, S/F 190 ms and S/S 390 ms; median decision-to-arrival is 1 ms in
fast-network arms and 90 ms in slow-network arms. These summaries hold at both
targets. The 120-ms processing wait changes the selected observation but is
absorbed before the fixed 1-second decision gate. Its zero filled-fraction
contrast is therefore **non-activation of the order-arrival timing channel in
this one-shot policy/horizon**, not proof that processing delay is harmless.

For 0.5 ABC, the selected public snapshot qualifies under the registered
displayed-depth proxy in 3/3 seeds per arm, but all selected episodes lack a
complete sampled right boundary; their duration ratio is undefined. For 5 ABC,
selected-qualifying counts are F/F=1/3, F/S=0/3, S/F=1/3, S/S=0/3; complete
sampled selected episodes are 1/3, 0/3, 1/3, 0/3 respectively. No ratio or
continuous quote-lifetime inference is made from the censored episodes. These
counts are selected-snapshot diagnostics, not the denominator for the
all-assigned primary endpoint and not a claim that the other worlds lacked
tradable depth.

## 9. Interpretation, invalid alternatives and nonapplicable modules

The network manipulation produced an actual venue-arrival contrast and mixed
fill outcomes at 5 ABC. The processing manipulation changed information age
without changing action time; with a fixed one-shot decision this design does
not isolate a general effect of computation delay on execution. At 0.5 ABC,
completion saturation hides possible cost/depth differences. Partial 5-ABC
fills are retained, not treated as failed evidence or silently excluded.
Shortfall is secondary and cannot turn an unfilled residual into an executable
purchase. A later fill report after cancellation must be judged by exchange
execution time and response delivery separately, not file traversal.

The experiment does not measure capital capacity, PnL, a city-to-city network,
adaptive routing, equilibrium, derivatives or empirical latency; these modules
are N/A. Four seconds and three development seeds do not establish long-run
stability or tail probabilities. The prospective 1-second gate is an explicit
behavioral assumption, not an endogenous market consequence.

## 10. Review, claims and next boundary

Prospective design/evidence review and corrected technical preflight were
accepted at their stated scopes. The [post-result review ledger](../../reviews/me002-result-20260923.md)
records Review A's initial required reporting corrections and bounded ACCEPT
follow-up, Review B's unavailable first attempt, and the fresh Sol-6 medium
Review B ACCEPT. A reviewed development result is not a platform realism
certificate. No reviewer independently reran a world; Review A checked raw
identities and source joins but did not repeat the full replay; Review B
checked statistical interpretation, not mechanics.

The ME-002 economic budget is exhausted. No confirmation seed, ME-002B study,
other ME-* world, frozen partition or historical holdout follows from this
report. The smallest next question, conditional on the reviews, is whether an
instruction contrast or later decision-phase design can separate opportunity
from the fixed 1-second gate under a separately prospective protocol.
