# ME-001 — three-seed development screen (review pending)

Status: **PROVISIONAL DEVELOPMENT RESULT**. The registered 29 executions
(two technical controls and 27 economic worlds) completed, but independent
result reviews are not yet dispositioned. This is not a frozen, confirmed,
empirical or holdout finding.

## 1. Question and bounded conclusion

How does replacing finite-resource background makers with delayed random
takers change one existing immediate ABC/USD buy policy's delivered depth,
execution completion and all-in target shortfall as its target grows?
Within this registered four-second, one-venue development grid, composition
separated delivered ask depth and changed the execution-scale admissibility
map. The simple directional shortfall prediction was **mixed across sizes**;
the preregistration calls this a response map, not binary support or
falsification. The strongest possible causal object is the *bundled* class
replacement under this simulator, not maker count, real-market liquidity,
profitability, or capital capacity.

## 2. Exact scope and mechanism

The [preregistered protocol](protocol.md), independently accepted before
world execution at `13c02d533892e20bfa3153ed789b8a51ef893ddb`, fixes
one ABC/USD price-time venue, one immediate buy parent, twelve background
accounts, common initial endowment per actor, 5 bp focal/noise taker fee in
USD, zero maker fee, 1 ms focal and 2 ms noise transport, a 1-second decision
wait and 4-second horizon. The intervention replaces two maker instances by
two delayed random takers (C−), or the reverse (C+), around C0=4/8. Actor
count and nominal aggregate initial endowment are fixed; actor IDs, policy,
fee role and indexed refresh clocks change as part of the bundle. The focal
policy's target sizes are 0.5, 2 and 5 ABC. It is an execution-cost mandate,
not a strategy that seeks trading PnL.

The predicted channel is changed delivered ask depth -> changed request
fill/cost. Rival explanations include the class-specific clocks, fees,
client-ID/queue ordering, and treatment-dependent terminal residual. The
same numeric seed does not guarantee identical background shock paths after
class replacement.

## 3. Historical context and separate identities

This is a new one-venue development line, not an R2/SV1D rescue. R2 remains
non-viable at its registered 24-hour survival gate; SV1D remains a valid
non-activation boundary. Historical executionlab policy comparisons retain
their own source and populations, not this candidate's identity.

Executable/protocol source: `13c02d533892e20bfa3153ed789b8a51ef893ddb`
(tree `92083079e1b3ec1a5e7e72277ff1781b788401a1`). Per-world simulator
SHA-256: `b0549005e74c953a4121914de6c878b6b611a8471f57e6214e4f6816952debb2`;
analyzer SHA-256: `d58fd8243e8ef31165a89ef8afb32894146403fade97fa9e231ca3766e5948fb`.
Go toolchain: 1.27.0; evidence schema: `execution-pilot-opaque-v3`.
The later Go-only all-assigned aggregator is
`16d5a505efa02adeaf99660057c014e0ede2fba2`, not the execution source;
its binary SHA-256 is `ba2b2e4f059e07b2c53bbd2279e9911402f3d5fa1d8cfd3473c9c78445a30199`.
Skill commit/tree: `f2cca325fa552766e23f83c3204bfa8f83f9e241` /
`d1ff061a7d98b9e003d7ac4166d1f7434f54eb17`.

## 4. Formula, units and denominator

Base precision is 100,000,000 units per ABC and quote precision is 100,000
units per USD. The buy target-shortfall convention is:

```text
C = sum checked fixed-point(q_k * p_k / B)
U = target - filled
S_target = C + U * terminal_mid / B - target * decision_mid / B + quote_fees
IS_bps = 10,000 * S_target / (target * decision_mid / B)
```

`U * terminal_mid / B` marks the *unfilled obligation*; it is not executed
cash cost. A positive two-sided terminal mark is required for this statistic.
The independent analyzer uses venue fills, fees and ledger changes, not the
focal actor's report, then compares its reconstructed result to that report.
All 27 current worlds had a defined terminal mark. Delivered five-level ask
quantity at the focal decision is the opportunity denominator. The 10 bp
mandate requires **both** 100% completion and `IS_bps ≤ 10`.

## 5. Reproduction and retained evidence

The immutable development namespace is
`/home/vlad/ExchangeSimulation-me001-development-13c02d5` on this host.
It contains 27 distinct locked plans, 29 run directories (including controls),
29 individual analyzer results, process resource logs, pinned binaries and
`analysis/response-surface.json`. The surface SHA-256 is
`63f5ea420c3882fb475b4ca9be63998761045e77ad7cdf61215b552288f5156a`.
It contains every assigned cell, exact plan/evidence IDs, independently
replayed outcomes, nine admissibility groups and six registered contrasts.
Its 27 economic evidence streams total 137,922,632 bytes; the complete
namespace occupied about 153 MiB at first closeout check. These external
files are retained in place, not copied over historical evidence. Their
metadata/contents are verified by the per-world manifest and by the later
aggregator's fresh raw replay. The external path is a location, not a claim
of remote backup.

To reproduce one cell from a clean checkout of the execution commit, build
`./cmd/mepilot` and `./cmd/mepilotanalyze` with Go 1.27.0 and compare the
binary hashes above; use `mepilot run` only in a *new* namespace with that
cell's locked plan. Do not overwrite retained raw evidence. To reproduce the
aggregate without any new world, build `./cmd/mepilotsurface` from the
aggregation commit, then run:

```bash
mepilotsurface -root /home/vlad/ExchangeSimulation-me001-development-13c02d5 \
  -repo /home/vlad/ExchangeSimulation-market-ecology-me001-20260923 \
  -execution-source 13c02d533892e20bfa3153ed789b8a51ef893ddb \
  -out <new-output-path>
```

The aggregator requires a clean analysis checkout, verifies all 27 plans,
manifests, evidence/actor hashes and stored results, and replays each raw
stream before calculating groups/contrasts. The current checkout must be at
the aggregation revision for that command; a later report-only HEAD requires
a clean checkout of `16d5a50` or a separately versioned reproducibility run.

## 6. Execution inventory and resources

Both fresh-process C0/S2/1009 controls completed. `GOMAXPROCS=1` and `7`
produced byte-identical evidence and reconstructed result files, including
canonical execution hash
`dd16fb27597981c7df9fbf728e60a71836c955c6ee5d2979c2c3b2fa2aceae5d`.
The first control used 0.40 seconds wall time and 41,344 KiB peak RSS;
the second used 0.33 seconds and 32,640 KiB. Their evidence trees were about
4.9 MiB each. The first measured extrapolation was below the registered
15-minute, 4-GiB and 1-GiB ceilings, so all 27 cells ran sequentially with
`GOMAXPROCS=7` and a 30-second per-world timeout. Every process exited 0,
produced a terminal manifest, and passed independent analysis. No cell was
selectively rerun or retuned. No holdout or ME-002+ world ran.
The control process settings are recorded in the invocation record and output
labels, **not inside the signed per-world manifests**; byte equality and
hashes verify the outputs, while those settings have weaker attestation.

## 7. All-assigned values

Seed order in each triple is `1009 / 1013 / 1019`. `IS` is target shortfall
bp; `fill` is executed ABC of the target. Full-precision values and hashes
remain in the machine surface. All cells have valid evidence, a local focal
decision opportunity, an active request and a defined terminal valuation.

| Arm | Target ABC | Filled ABC by seed | IS bp by seed | Mandate pass |
|---|---:|---|---|---:|
| C0 | 0.5 | 0.5 / 0.5 / 0.5 | 7.001 / 7.001 / 7.000992 | 3/3 |
| C+ | 0.5 | 0.5 / 0.5 / 0.5 | 7.001 / 7.001 / 7.000992 | 3/3 |
| C− | 0.5 | 0.5 / 0.5 / 0.5 | 7.001 / 8.098892 / 7.553604 | 3/3 |
| C0 | 2 | 2 / 2 / 2 | 9.002 / 9.002 / 9.533997 | 3/3 |
| C+ | 2 | 2 / 2 / 2 | 8.0015 / 8.0015 / 8.36122 | 3/3 |
| C− | 2 | 2 / 2 / 2 | 13.004 / 14.101896 / 14.307593 | 0/3 |
| C0 | 5 | 5 / 5 / 4.86706661 | 15.005 / 15.005 / 14.8188664 | 0/3 |
| C+ | 5 | 5 / 5 / 5 | 11.8034 / 11.8034 / 12.2350656 | 0/3 |
| C− | 5 | 2.5 / 2.36283133 / 2.30585648 | 7.5025 / 7.310436 / 7.130528 | 0/3 |

The complete tested 3/3-admissible region is C0={0.5,2}, C+={0.5,2},
C−={0.5} ABC. No point at 5 ABC passes 3/3, so this grid did not observe
an admissible upper point. This is not a monotone extrapolated frontier or
a sustainable capital-capacity estimate. At S3, four worlds partially filled:
one C0 and all three C−; every such world remains in the denominator.

## 8. Registered contrasts and uncertainty

Depth differences are delivered five-level ask quantity in ABC, treatment
minus C0. The same pre-decision book is used across target sizes within a
composition/seed, so depth contrasts repeat across S1–S3 by design.
Shortfall differences are bp, treatment minus C0. The table gives the
three-seed median and the observed paired range, not a p-value.

| Arm vs C0 | Target ABC | Median depth Δ ABC | Median IS Δ bp | Paired IS range bp |
|---|---:|---:|---:|---:|
| C+ | 0.5 | +2.60411060 | 0 | 0 to 0 |
| C− | 0.5 | −2.41880459 | +0.552612 | 0 to +1.097892 |
| C+ | 2 | +2.60411060 | −1.0005 | −1.172777 to −1.0005 |
| C− | 2 | −2.41880459 | +4.773596 | +4.001999 to +5.099896 |
| C+ | 5 | +2.60411060 | −3.2016 | −3.2016 to −2.583801 |
| C− | 5 | −2.41880459 | −7.6883384 | −7.694564 to −7.5025 |

Both opportunity-separation signs satisfy the registered median-depth rule.
The shortfall prediction is not uniform: C+ is flat at S1 but lower at S2/S3;
C− is higher at S1/S2 but lower at S3. Under protocol §1's predeclared
decision rule, these are **mixed-size response maps**, not a forced
`SUPPORTED` or `FALSIFIED` verdict for a universal directional effect.
Three independent development seeds are the uncertainty units; order/event
rows and multiple fills are not independent replications.

## 9. Interpretation, alternative explanations and limits

The C− S3 target shortfall appears low despite poor completion because the
unfilled 2.5–2.69 ABC is marked at terminal midpoint rather than bought
through the spread or charged a taker fee. This is the preregistered
mark-to-complete convention, not a zero-cost executable fill. All three
remain economic failures under the conjunction of completion and ≤10 bp.
The C+ S1 zero shortfall contrast despite much deeper asks is consistent
with small orders paying the same touch and fee; additional depth need not
improve a small market order's price. Neither explanation isolates maker
count from the bundled policy/fee/clock replacement.

The four-second compressed horizon does not establish long-run stationarity.
This is a development/calibration screen, not confirmation on unseen seeds,
not an empirical real-data comparison, and not evidence of a causal price
impact law. No liability hedge, financing, derivatives, cross-venue
arbitrage, adaptive capital or restricted equilibrium was studied; those
modules are N/A rather than silently scored. Matching, fee and clock rules
are fixed within this comparison. No geographical latency claim is made.

## 10. Review and next boundary

ME-000 mechanical readiness was independently accepted at `b25223a`.
The prospective protocol was separately accepted by a fresh read-only
Sol-6 medium reviewer at exact `13c02d5` before any economic world.
Post-result Reviewer A (independent read-only Sol-6 medium) **ACCEPTED** the
execution/accounting/evidence integrity of the 27 development cells and the
`16d5a50` aggregation at its stated scope. It independently checked the
matrix, per-cell artifact coverage, sampled binary hashes, all 27 plan,
evidence-file and actor-file hashes, the replay implementation, and the four
partial-fill classifications. It did not execute a new raw replay. It noted
the control-environment attestation limit above and that the group summary
does not list every possible class separately, although each actual cell
does. Reviewer B's causal/statistical interpretation is pending. Until that
substantive finding is dispositioned, this report is provisional. No review
approval is inferred from service execution or from green tests.

The only next authorized step here is to finish those reviews, publish a
scope-limited report and separately propose ME-002/003/005/007 readiness
plans. No holdout, confirmation set, or ME-002+ world is authorized by this
development result.
