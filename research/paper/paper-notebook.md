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

## Draft article architecture, not yet results text

1. Why conditional market ecology requires explicit policy, actor,
   deployment and venue assumptions.
2. Reproducible execution/evidence mechanics and the opportunity-to-action
   denominator.
3. ME-001 composition × quantity response (development only).
4. ME-002 deployment × quantity response (reviewed development only).
5. Contradictory and null outcomes, fixed-clock limits, and what remains
   unconfirmed.
6. A future separately authorized confirmation/empirical-comparison design.

No ME-003 or ME-005 result exists at this notebook revision. Their proposed
figures and economic claims remain placeholders until registered evidence is
available; none should be drawn from fixtures alone.
