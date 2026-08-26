# V2-7 P7d development result

Status: **SUPPORTED (screening)** for the registered finite-capital
directional activation and participant-specific maintenance-risk replay.  This
result does not claim liquidation realism for the ecology, funding or basis
anchoring, profitability, or any stylized fact.  The reserved holdout policy is
not represented in this document and no holdout world has been consumed.

## Protocol and provenance

The immutable protocol is
[`v2-7-p7d-directional-distress-causal-preregistration.md`](v2-7-p7d-directional-distress-causal-preregistration.md).
The development cells are C (disabled control), L (+2,000,000,000 raw ABC)
and S (-2,000,000,000 raw ABC), with seeds 431 and 433.  The target is
reached only through ordinary admitted IOC orders; the directional desk has
finite own perp collateral and may use only the declared, capped `auto_perp`
quote borrow.  No synthetic close or balance reset is permitted.

All six cells completed the registered four-hour, full-evidence horizon.  The
simulator source revision recorded by the run metadata is
`8b1d013a33129beeb61a578b6f59aefa59cce0c2`; the simulator binary SHA-256 is
`0bcc40ef78f87f08301555bf203366780569c19ed42e84c024e39de34d2ebece` and the
analyzer binary SHA-256 is
`934993a87a817cf3fcb52eb29e8b1392d0b81e2149b52a4f5706026d57e763ad`.
Every cell has successful run/extraction status, the final `greeks.json` and
`latency.json` completion sentinels, all sixteen P7d metric artifacts, and
complete analysis metadata.  Runtime and offline persisted-evidence artifact
digests agree exactly in every cell.  Raw evidence remains retained; this
protocol has no prune authority.

The six immutable configuration hashes are:

| cell | seed | config SHA-256 |
|---|---:|---|
| C | 431 | `5bdf0bb6c12353afe3ca0ad263e03318e7b4af89b65210d9d39ee89e0e0a8fe4` |
| C | 433 | `a76c031440643acad94544a181e1367f3ac0513dc5c5a158938c380ce80497f8` |
| L | 431 | `de58790c8afdd20da25d5b9cea467389998cde753aea475874a69eae1dd24e66` |
| L | 433 | `f74dea1ede816890a7fa57dcae4ed7e2dd2eaf7bd7304f64a0aa2937eaf32108` |
| S | 431 | `3cf7b81ab90d6b8cbd5ca46e5cf2f88c0283f944f8d4b41a0af5311458a00ec9` |
| S | 433 | `be933e99848605e6f99dabe5593c3be84f921f86a0249fc047d4c5a946f8db17` |

The analyzer-only source revision differs for C-431 (`abc42a1`) and the other
five cells (`dae67ae`), but the analyzer bytes and SHA are identical.  The
`abc42a1` revision narrowed borrow replay to the declared
`perp_exposure_hedger` role; the same role-scoping logic is therefore present
in the analyzer binary for every scored cell.  The simulator binary was built
before these analyzer-only commits; its Go build stamp reports an older
revision and a dirty working-tree flag, while `git diff` over simulator source
paths between that stamp and `4b2856e` is empty.  This is recorded provenance,
not a claim that the stamp should be rewritten after the fact.

The first C-431 extraction attempts failed closed and are retained.  A later
sequential retry passed after the role-scoped borrow analyzer was installed;
the retry did not rerun or change the simulated world.

## Evidence and activation

All cells passed receipt/frontier replay (`future_decision_use=0`,
`bad_decision_frontier=0`, `receipt_errors=0`), order/fill/position links,
conservation, lifecycle, settlement, and the P7d configuration/borrow gates.
Raw event counts and evidence digests are:

| cell | seed | decisions | enabled | admitted | fills | filled qty (raw ABC) | evidence events | evidence digest |
|---|---:|---:|---:|---:|---:|---:|---:|---|
| C | 431 | 21,600 | 0 | 0 | 0 | 0 | 14,326,272 | `d7fb4765a3ae5f56f1b763120662f6375da4bfe844ff872cc71a17a1ccda2f68` |
| C | 433 | 21,600 | 0 | 0 | 0 | 0 | 14,417,901 | `8f385ae653c8f62f95e37b44f6ed43dee62028ddfb4c419beafdb34d9b0ad325` |
| L | 431 | 21,600 | 21,600 | 306 | 40 | 6,000,000,000 | 14,294,967 | `9a52d217ff01e4bd87a16127c710e3798c570466b37983ffdaee886712f9b03e` |
| L | 433 | 21,600 | 21,600 | 266 | 40 | 6,000,000,000 | 14,237,964 | `f32d059fcfbc0ed2a3bc7c71e3cf95d3ec463ba02b675de21e6e194507ef3918` |
| S | 431 | 21,600 | 21,600 | 46 | 47 | 6,000,000,000 | 14,303,031 | `9cc9a4f9d32ea0580d62626efd484995801e8f0c29f43c21b5fd778dd70c9f62` |
| S | 433 | 21,600 | 21,600 | 22 | 51 | 6,000,000,000 | 14,356,776 | `ef80f19a15bd4c9e49bd51da9b9b13556bc9fe338b05c3602754ae8ee6bc2584` |

Controls remained disabled (the three `NOT_SUBSCRIBED` records are the
expected roster observations; all remaining decisions are `POLICY_DISABLED`).
Each active orientation submitted ordinary local-touch IOC orders and reached
the declared 6,000,000,000 raw-ABC target, with zero terminal target gap at
each venue.  This supports activation/execution at screening level; it says
nothing about whether the market would supply the same liquidity out of
sample.

## Participant-specific risk replay

The independent `perpexposurerisk` replay used contemporaneous mark updates,
gross wallet, declared borrow debt, position and balance continuity, and the
registered 5% maintenance rule.  Risk checks were evaluated before same-time
state mutations.  All arithmetic, mark-domain, balance, contribution,
equity, notional, maintenance, cross-file, malformed-record, path and
terminal-state counters were zero in every cell.

| cell | seed | candidates | mark updates | expected breaches | observed checks | participant liquidations | deficits | insurance deficit |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| C | 431 | 3 | 43,186 | 0 | 0 | 0 | 0 | 0 |
| C | 433 | 3 | 43,189 | 0 | 0 | 0 | 0 | 0 |
| L | 431 | 3 | 43,188 | 16 | 16 | 10 | 3 | 3,970,335,945 |
| L | 433 | 3 | 43,189 | 14 | 14 | 12 | 1 | 823,797,845 |
| S | 431 | 3 | 43,185 | 1 | 1 | 1 | 0 | 0 |
| S | 433 | 3 | 43,189 | 1 | 1 | 1 | 0 | 0 |

The long and short orientations both exercise a participant-specific
maintenance breach with an independently linked risk event, so the registered
participant-risk predicate is **SUPPORTED (screening)**.  The controls have no
participant risk path by design.  Generic exchange liquidation counts (603,
1,455, 1,228, 293, 616 and 369 respectively) are for the wider population;
they are not substituted for the participant replay.

The long cells also show non-zero deficit/insurance movements (three and one
events).  This endpoint is recorded as **OBSERVED; separate accounting review
required**, not folded into the participant-risk verdict.  No P7d cell provides
a registered participant bankruptcy claim; ordinary generic liquidation
activity is not evidence of bankruptcy for this desk.  Residual positions
after partial forced closes remain visible in the terminal participant state;
no synthetic reset or reopening was used.

## Registered score and interpretation

The fail-closed score is
[`p7d-development-score.json`](artifacts/v2-7-p7d/p7d-development-score.json).
Its classification is `SUPPORTED (screening)` with all control, activation,
evidence-integrity and participant-replay predicates true.  The two signs are
both exercised, so this is not the `MIXED` orientation case.  The development
seeds are only two paired screening seeds; no robustness claim is licensed.

The reserved holdout seeds are 439, 443 and 449 and remain unconsumed.  Under
the preregistration, valid activation in both development seeds and clean
participant risk events authorize the holdout policy.  Holdout execution is a
new gate and must retain the same immutable protocol, evidence contract and
role-scoped analyzer.

This result does not establish funding/carry effects, basis convergence,
profitability, market stability, liquidation realism for other participants,
or any stylized fact.  Those questions remain separate V2 programs.
