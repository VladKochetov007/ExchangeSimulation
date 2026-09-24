# ME-003 — IOC versus FOK execution-instruction development response map

Status: **REVIEWED DEVELOPMENT RESPONSE MAP**. All 12 registered economic
cells and two fresh-process controls completed. The bounded mechanics/evidence
and causal/statistical post-result reviews accepted their respective scopes.
This is not confirmation, an empirical-exchange claim, a trading-profit study,
or a universal IOC/FOK ranking.

## Question and bounded result

For the same immediate finite ABC/USD buy mandate, fixed price cap,
counterparties, resources and delayed deployment, how does limit IOC versus
limit FOK change execution completion? The registered all-assigned filled
fraction differed in **one of six matched seed/target pairs**. At 5 ABC,
seed 14001, IOC bought 4.80395196 ABC and cancelled 0.19604804 ABC, whereas
FOK rejected with `FOK_NOT_FILLED` and bought none. The other five pairs
fully filled in both arms. The three-pair median IOC-minus-FOK contrast is
zero at each target. This is a real observed instruction-path difference in
one development world, not evidence that IOC is generally preferable.

## Registered mechanism and fixed scope

The [locked protocol](protocol.md) fixes C0: one ABC/USD price-time venue,
four finite makers, eight random takers and one immediate BUY parent (client
13) with finite endowments. The parent receives local market data and sends
one child at or after 1 simulated second. Symmetric feed/request/response
latency and poll interval are 1 ms; the terminal horizon is four simulated
seconds. Taker fee is 5 bps in quote units and maker fee is zero. The child
is a limit BUY capped at 50,100 USD/ABC, encoded as `5_010_000_000` quote
precision units. The only treatment within a pair is IOC versus FOK;
targets are 0.5 and 5 ABC, each at seeds 14001, 14011 and 14017. No
passive GTC, post-only, market-order or adaptive policy was bundled into
this contrast.

IOC may fill accessible quantity up to the cap and cancel its remainder.
FOK preflights all-or-none execution and can reject before any fill. A
rejection can be due to matcher preview or fee/spot-plan pruning, so
`FOK_NOT_FILLED` alone does not reveal a unique cause. The fixed cap protects
against an above-cap fill; it does not guarantee that a sampled local ask
will remain executable at venue arrival.

## Identity, review and retained evidence

Execution/protocol candidate C is
`ae0fc333f51b2975827116d2c72db00b4f282ad9`, tree
`f15f2c750786792b0083fbe0bfe3b5a0cd3ee32d`, Go 1.27.0,
`instruction-pilot-opaque-v1` evidence schema. The protocol blob at C is
`a5ff309de9c9b674796273d6b7f5a447d5208b78`; the current report is a
later status artifact, not a change to C. Clean simulator SHA-256:
`b36ba97df08540fc2d65a10e121acff026bb784c9e22d9c07816d787e3682bf0`;
pinned analyzer SHA-256:
`1f24aa42c56eface199a4acd73845d05aa2a9af31f724760d5ecb62d190601a5`.
Both binaries report C and `vcs.modified=false`. The all-assigned Go
aggregator is `12f4d398749443de19806799711ffad5b9a3b14b`, binary
SHA-256 `f16623bead6681c9c842988bc8f5b8976316c13cfad7da0fa74e95783ab8d6a7`.
The separate Go diagnostic is `5b015b80271945ca3e9358f4cff70f529657d75f`,
binary SHA-256 `e2246829c2e64a65b6aeb3842fd1a23de53c41616b1f94d64919fdef37a2b615`.
The repo-local research skill at C has blob
`94099bf128823086c93f056ba4a559e15ab32bf1`, tree
`ee529dd73d061b43717e57d8b7fa832543f0e5a2`.

External retained namespace:
`/home/vlad/ExchangeSimulation-me003-development-ae0fc33`. It contains
the pinned binaries, 12 per-cell plans, 12 immutable run directories,
12 individual analyzer results, per-run `/usr/bin/time -v` records, two
technical controls, and the all-assigned
`instruction-surface.json` (SHA-256
`f3465897ad8f338eb3b9c9427d856687e409f04445f51b3d430e1ade0778172b`).
The bound `instruction-diagnostics.json` has SHA-256
`d6137e6c4c1ca87bb9df131eaa1a45f853432d948b7d2c92073d44715db1aa02`.
External location is not a claim of remote backup. The
[technical-control release](control-gate-20260924.md),
[prospective review ledger](../../reviews/me003-prospective-20260924.md),
and [post-result reviews](../../reviews/me003-result-20260924.md) preserve
the rejected earlier candidate and exact acceptance scopes.

## Measurement and reconstruction

Primary outcome: `Y = exchange-filled ABC base units / assigned ABC target
base units`. One ABC equals 100,000,000 base units. A valid rejection,
accepted-unfilled order or no-send would score zero; invalid or incomplete
evidence would be `UNASSESSABLE`, never a zero or an omitted survivor.
The registered paired contrast is `Y_IOC − Y_FOK` at each seed and target.
Three matched seed pairs, not event rows or fills, are the uncertainty units.
The three-pair median and observed range are descriptive; no p-value,
equivalence margin or practical-benefit threshold was registered.

Each pinned plan binds the effective world, source/tree, both binary hashes,
toolchain, instruction and cap. The analyzer joins actor-local publication,
delivery, selection, send, venue arrival, admission, exchange-time trade/fill,
delayed receipt, cancellation/rejection, charged quote fees, balance deltas
and terminal balances/mark. The all-assigned Go aggregator rehashed every
plan, manifest, evidence and actor file, re-walked every canonical stream,
independently reconstructed the 12 outcomes and compared them to the stored
actor and analyzer results. It failed closed on incomplete or contradictory
cells. The reviewer checked this path and retained hashes; neither reviewer
ran another raw replay. All 12 assigned cells passed.

The opportunity chain is published public book → delivered local quote →
selected two-sided snapshot → sampled displayed ask quantity at/below cap →
send → venue admission/execution → delayed response → terminal obligation.
The selected cap-bounded ask quantity is a **sampled actor-observed proxy**,
not continuously observed executable depth at later arrival. The diagnostic
compared the recorded pre-arrival fields, including snapshot sequence/time,
decision midpoint/time, sampled ask, send/arrival times and initial balances;
they align within all six IOC/FOK pairs. That does not prove equality of
every unrecorded background event merely because seed labels match.

## All assigned outcomes

Values are filled fractions, not percentages. Seed order is
`14001 / 14011 / 14017`.

| Target | IOC by seed | FOK by seed | Full completion by instruction |
|---:|---|---|---|
| 0.5 ABC | 1 / 1 / 1 | 1 / 1 / 1 | IOC 3/3, FOK 3/3 |
| 5 ABC | 0.960790392 / 1 / 1 | 0 / 1 / 1 | IOC 2/3, FOK 2/3 |

| Target | Seed | IOC − FOK | Recorded path |
|---:|---:|---:|---|
| 0.5 ABC | 14001 / 14011 / 14017 | 0 / 0 / 0 | Both fully filled |
| 5 ABC | 14001 | +0.960790392 | IOC partial/cancel; FOK rejected |
| 5 ABC | 14011 / 14017 | 0 / 0 | Both fully filled |

The three-seed median contrast is 0 at both targets. Observed range is
0–0 for 0.5 ABC and 0–0.960790392 for 5 ABC. No invalid, missing, no-send,
or accepted-unfilled economic cell was hidden from the denominator. All
12 focal decisions occurred at 1 simulated second and all 12 terminal
two-sided marks were available. Quote fees and balance changes matched
execution evidence in every cell.

For 5 ABC/seed 14001, both arms selected snapshot 726, sent request 2 at
1.000 s and arrived at 1.001 s. IOC recorded 20 fills totaling 4.80395196
ABC, 12,022,281 quote units of taker fees, and an `IOC_EXPIRED` residual
of 0.19604804 ABC. FOK recorded zero fills/fees, unchanged ABC/USD balances,
no leaked reservation and `FOK_NOT_FILLED`. This is activation of a distinct
instruction response, not identification of the exact FOK preflight branch.

The selected snapshot showed 5 ABC within the cap in seed 14001, yet IOC
filled less and FOK rejected. In seed 14011 it showed only 4.90497559 ABC
within the cap, yet both arms filled 5 ABC. This two-way discrepancy is a
concrete reason not to relabel selected depth as the at-arrival opportunity
denominator. It may reflect book evolution or preflight constraints; the
current evidence does not apportion those explanations exactly. The other
five zero-contrast pairs are observed nulls for this particular instruction
comparison, not equivalence or proof that no relevant opportunity arose.

Filled and target shortfall are secondary. All 12 have defined decision and
terminal marks, but FOK seed-14001 reports zero target shortfall because the
entire unmet 5-ABC residual is marked at an unchanged terminal midpoint.
That zero is **not** execution, low-cost success or profit. The IOC partial
cell's target shortfall is 14.7304928 bp; its filled-only shortfall is
15.331640410492366 bp. Missing terminal marks in a future world would make
target shortfall unavailable without invalidating a valid filled fraction.
No markout or causal price-impact claim is made.

## Resources, controls, reproduction and review limits

The two fresh C controls used IOC/0.5 ABC/seed 14001 at `GOMAXPROCS=1` and
`7`. Their canonical evidence, actor reports, manifests and reconstructed
results were byte-identical; evidence SHA-256 is
`bb84efd3043c3f1b402b33bcbbe29096991260cb3250646dcf6a8d8c494ee849`.
The measured controls took 0.40/0.33 s with peak RSS 41,344/33,024 KiB.
The 12 development cells were launched sequentially in protocol order with
`GOMAXPROCS=7` and a 30-s per-world timeout; every simulator and analyzer
exited zero. Economic evidence totaled 61,134,641 bytes; measured world wall
time totaled 3.92 s, maximum 0.34 s, and peak RSS 33,280 KiB. The namespace
occupied about 86 MiB including binaries at closeout inspection, below the
1-GiB retained-evidence cap. The content-bound plan/run manifest binds content and
source identities but does not attest shell process settings, execution order,
or absence of earlier attempts; those settings are a session-level account
corroborated by retained resource files, not a cryptographic claim.

To reproduce the *offline* all-assigned reconstruction, in a clean checkout
of aggregation commit `12f4d39` build `./cmd/meinstructionsurface` with Go
1.27.0 and compare its binary hash above. Run only against the retained
namespace, writing to a new output path:

```bash
meinstructionsurface -root /home/vlad/ExchangeSimulation-me003-development-ae0fc33 \
  -repo <clean-12f4d39-checkout> \
  -execution-source ae0fc333f51b2975827116d2c72db00b4f282ad9 \
  -out <new-surface-path>
```

Then build `./cmd/meinstructiondiagnostics` at `5b015b8` and run
`meinstructiondiagnostics -surface <verified-surface-path> -out
<new-diagnostics-path>`. Neither command launches a market world. The
simulator trajectories stay pinned to C; later analysis/report commits are
different identities. Original ME-001, ME-002 and historical R2/SV1D runs
retain their own populations, analyzers and verdicts.

The two bounded [post-result reviews](../../reviews/me003-result-20260924.md)
accepted retained mechanical evidence and a scoped causal/statistical
interpretation. They did not independently replay every raw stream or certify
empirical behavior. The initial prospective reviewer REJECT of `f407505`
was resolved in a separately committed successor and remains visible. A
third model-self-identification attempt was not counted as a qualified
Sol-6 review.

## Interpretation, limitations and next question

The 0.5-ABC endpoint hit an all-full ceiling; it cannot reveal differences
below completion. At 5 ABC, one seed exercised the IOC/FOK lifecycle trade-off
while two did not produce a filled-fraction difference. That is a bounded
simulation-internal causal response map under fixed one-shot policy, cap,
ecology, logical latency and four-second horizon. It gives no reliable
frequency of FOK rejection, tail risk, strategy utility, trading PnL,
capital capacity, passive-order ranking, long-run stationarity, or real-market
effect. No confirmation seeds, empirical corridor or reserved holdouts were
used. The 12-cell development budget is exhausted; no seed or cap was retuned.

Potential follow-ups are **PROPOSED ONLY**; none is authorized by this result:

| Motivation and conditional hypothesis | Alternative explanation | Smallest future test / dependency / compute estimate |
|---|---|---|
| Seed-14001 FOK cause is ambiguous: explicit venue preflight evidence may distinguish reachable-depth failure from fee/spot-plan pruning. | The observed rejection may remain indistinguishable without changing evidence semantics. | First specify and adversarially test one non-actor-visible preflight event; offline fixture plus a separately registered ≤4-world development check if later authorized. |
| The one-shot immediate policy omits passive queue exposure: a deadline policy may trade fill probability against adverse selection. | Any effect may be a changed objective/decision rule rather than TIF alone. | Separate ME-003 successor protocol after actor/evidence readiness; ≤12 development worlds plus controls as a proposal, not a matrix extension here. |
| ME-005: delayed two-venue executable edges may lead to non-atomic leg risk rather than riskless convergence. | Apparent edge may be midpoint/fee/transfer artifact or never actor-observed. | Begin source/evidence readiness only, then at most four prospectively locked OFF/ON development worlds if feasible and separately reviewed. |

Execution semantics, passive deadline, capacity, substitution, latency
geography, games, capital flows and empirical validation are separate
modules. The next authorized task, if any, must be chosen prospectively.
