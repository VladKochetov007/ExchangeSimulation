# Market-ecology paper notebook — development evidence only

This notebook is a claim/evidence index, not a results paper. It separates
mechanical validity, simulation-internal development effects, later
confirmation, and empirical comparison. No current study supports a general
market-realism or future trading-profit claim.

## ME-001 — composition × execution quantity

- Question: how does finite-resource maker/random-taker class replacement
  alter delivered ask depth, completion, and all-in target shortfall for one
  immediate ABC/USD buyer?
- Mechanism: changed background order provision/flow changes the locally
  delivered ask curve and then the buyer's attainable fill and cost. This is
  a bundled class replacement, including indexed clocks, fee roles and IDs,
  not isolated maker count.
- Exact development result: 27/27 economic cells and two controls were valid.
  Both replacement directions separated five-level delivered ask depth as
  registered. Fifteen cells passed the joint full-completion/≤10-bp mandate;
  shortfall effects changed sign or flattened by target size. At 5 ABC no
  composition was 3/3 admissible. See the reviewed
  [report](../program/ideas/ME-001/report.md) and
  [machine result](../program/ideas/ME-001/result.json), not this summary,
  for all assigned values and identities.
- Evidence class: independently reconstructed, reviewed, three-seed
  simulation-internal **development** response map.
- Candidate figure/table: target size × composition response surface with
  all individual seed points and completion/shortfall shown separately.
  Any figure must be generated from the versioned machine surface, not
  hand-copied table points.
- Limitations: no capital-capacity, profitability, real-data or general
  equilibrium conclusion. Shortfall marks the unfilled obligation; it does
  not execute it. New independent conditions and empirical comparison would
  be needed for stronger claims.

## ME-002 — synthetic network × actor-processing delay

- Question: holding the immediate policy and C0 ecology fixed, how do
  directed transport and actor-side processing separately change filled
  fraction at 0.5 and 5 ABC?
- Mechanism: feed/processing changes which snapshot is eligible; outbound
  transport changes order arrival. The fixed 1-second earliest-decision
  gate may absorb nominal processing delay before an order can be sent.
- Exact reviewed development result: 24/24 economic
  cells and two corrected-candidate controls completed with valid evidence.
  All 0.5-ABC targets fully filled. At 5 ABC the three paired network effects
  on filled fraction were −0.013709140, −0.037816292 and +0.034719890;
  processing effects and interactions were zero in all matched quartets.
  Processing changed selected snapshots in 12/12 matched pairs but order
  arrival in 0/12. Network changed arrival in 12/12 pairs and filled quantity
  in 6/12. See the [report](../program/ideas/ME-002/report.md), locked
  [protocol](../program/ideas/ME-002/protocol.md), and
  [bounded reviews](../program/reviews/me002-result-20260923.md).
- Evidence class: independently reconstructed three-seed simulation-internal
  development screen; bounded mechanics/evidence and causal/statistical
  post-result reviews accepted. Neither review certifies empirical realism.
- Candidate figure/table: per-seed 2×2 filled-fraction points alongside
  selected-message publication-to-arrival and decision-to-arrival timing.
  A plot should show individual worlds, the 0.5-ABC ceiling and censored
  opportunity episodes; no smooth fitted curve or p-value.
- Limitations: processing order-arrival channel did not activate under the
  one-shot 1-second gate; zero fill effect is not computation irrelevance.
  Network signs are mixed. Selected displayed-depth episodes are snapshot
  proxies and usually censored. No geography, city connection, confirmation,
  empirical benchmark, long-run risk, PnL or capital-capacity conclusion.

## ME-003 — limit IOC versus limit FOK

- Question: at one venue and a fixed immediate finite buy mandate, cap,
  counterparties and deployment, how does the child time-in-force alter
  completion and non-fill risk?
- Mechanism: IOC may take reachable capped depth and cancel the residual;
  FOK rejects unless its preflight can complete the whole quantity.
- Exact reviewed development result: 12/12 assigned worlds and two controls
  had valid evidence. At 0.5 ABC all IOC/FOK seed pairs fully filled. At
  5 ABC, seed 14001 IOC bought 4.80395196 ABC and cancelled 0.19604804,
  whereas FOK bought none and rejected; both instructions filled 5 ABC in
  the other two seeds. The three-pair median filled-fraction difference is
  zero at both targets. See the [report](../program/ideas/ME-003/report.md),
  [machine result](../program/ideas/ME-003/result.json) and
  [scoped reviews](../program/reviews/me003-result-20260924.md).
- Evidence class: pinned binary/canonical evidence, independent Go replay
  and two bounded Sol-6 medium post-result reviews. This is a
  simulation-internal three-seed **development** response map, not a
  confirmation or empirical claim.
- Candidate figure/table: paired seed points for IOC and FOK filled fraction
  at 0.5/5 ABC, with the partial/rejected lifecycle annotated and sampled
  selected ask quantity shown as a separate proxy. If plotted, derive every
  point from the versioned `instruction-surface.json` using a versioned
  `.venv` matplotlib script; do not smooth a six-pair surface or make fills
  independent uncertainty samples.
- Limitations: `FOK_NOT_FILLED` does not reveal the exact preflight branch;
  delayed selected depth is not at-arrival executable depth. FOK's zero
  marked shortfall in the rejected seed is an entirely unmet mandate, not
  good execution. No universal TIF preference, PnL, long-run capacity,
  real-data comparison or holdout result follows.

## ME-005 — two-venue first-attempt opportunity boundary

- Question: did a finite prefunded router encounter a positive fee/depth-adjusted
  one-lot two-venue ABC/USD edge in its local information, attempt a route and
  produce a reconstructible economic closeout?
- Exact reviewed development result: four valid five-minute economic cells
  and one technical duplicate. Both seeds had zero registered public
  fee-positive opportunities in OFF and ON; the ON router evaluated 783 and
  790 local updates but submitted no group. Profit per attempt and convergence
  were not identified. See the [ME-005 report](../program/ideas/ME-005/report.md),
  [machine result](../program/ideas/ME-005/result.json) and
  [bounded post-result review](../reviews/me005-post-result-review-20260924.md).
- A separately versioned [offline public-stage diagnostic](../program/ideas/ME-005/exploratory-public-stages-20260926.md)
  found zero *gross* cross-venue bid/ask crossings conditional on the
  router's two-sided public-book requirement, before the one-lot depth and
  fee filters. It is exploratory, not a replacement registered result.
- Candidate table: the nested public-stage funnel, explicitly separating
  directional quote-state duration from economic opportunity count; no
  per-attempt profit point or causal convergence plot exists.
- Limitations: two seeds/five minutes, no qualifying edge, no route, no
  identifiable conditional funding/delivery/profit effect, no empirical
  comparison. Do not interpret zero opportunities as strategy ineffectiveness
  or a general impossibility result.

## ME-002-B — preregistered first-action cadence question

- The [prospective protocol](../program/ideas/ME-002-B/protocol.md) is a
  separate 12-world 1/80-ms policy poll × 1/90-ms directed network screen
  for one 5-ABC child, with zero added processing. The earlier plan and
  source/evidence candidate are reviewed; **no result belongs in this
  notebook until the registered worlds and independent reconstruction finish**.
- The fixed 1-s gate makes the 80-ms first eligible tick 1.040 s; this is a
  phase-dependent first-action question, not recurring high-frequency trading.

## Draft article architecture, not yet results text

1. Why conditional market ecology requires explicit policy, actor,
   deployment and venue assumptions.
2. Reproducible execution/evidence mechanics and the opportunity-to-action
   denominator.
3. ME-001 composition × quantity response (development only).
4. ME-002 deployment × quantity response (reviewed development only).
5. ME-003 IOC/FOK conditional completion and non-fill response (reviewed
   development only).
6. ME-005's valid no-opportunity boundary and ME-002-B's fixed-gate question,
   with any later ME-002-B result added only after completed review.
7. Contradictory and null outcomes, fixed-clock limits and what remains
   unconfirmed.
8. A future separately authorized confirmation/empirical-comparison design.
