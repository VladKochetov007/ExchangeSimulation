# DRAFT protocol — composition-conditioned immediate-execution capacity

Status: **NOT AUTHORIZED TO IMPLEMENT OR RUN**  
Protocol state: decision-ready draft, contingent on two bounded fixes  
Source baseline inspected: `a878dca984911ab379fa5d619efc274ae43736a6`  
Companion readiness packet: [`NEXT-STUDY-READINESS-PACKET.md`](NEXT-STUDY-READINESS-PACKET.md)

This document proposes exact economic cells but does not register them. The
owner must separately authorize implementation of the two evidence/adapter
prerequisites and then preregister execution. Candidate seeds below are
proposals, not reservations and not assertions that their outcomes are unseen.

## 1. Primary question

> In the one-venue ABC/USD execution laboratory, how does replacing adaptive
> makers with delayed random takers—while preserving background account count
> and nominal aggregate initial endowment—change the execution-scale
> admissibility and all-in target implementation shortfall of a fixed immediate
> buy policy?

### Prediction

The maker-poor/flow-rich population will expose less delivered ask depth and
will have lower completion and/or higher target shortfall at the larger target
sizes. The maker-rich population will show the opposite ordering.

### Falsifier

If valid evidence establishes the predicted delivered-depth ordering but the
paired outcome response maps do not change in the predicted direction at any
tested scale, the proposed mechanism is not supported. An absent depth
difference is an identification failure, not a mechanism falsification.

## 2. Scope and fixed assumptions

- one ABC/USD spot book and one continuous price-time matching venue;
- existing `executionlab.Immediate` focal policy, one buy parent per world;
- deterministic ingress/phases and 1 ms simulation step;
- focal decision at 1 simulated second, 1 ms poll interval;
- focal request/response/market-data latency fixed at 1 ms;
- background random-taker latency fixed at 2 ms;
- maker direct mount, as in the existing laboratory;
- five maker levels, 0.25 ABC each, ten-dollar tick spacing, weighted-mid
  control, 10–30 ms deterministic refresh schedule;
- random taker decision interval 25 ms, 0.05 ABC target-size parameter,
  independent actor streams, and visible-depth bounding;
- focal and noise taker fees 5 bp in USD; maker fee zero;
- common initial balance per participant: 100,000 ABC and USD 100,000,000;
- no margin, financing, derivatives, multiple venues, hidden liquidity,
  learning, capital evolution, or R2 calendar;
- one 4-second simulated horizon: 1 second warm-up and 3 seconds after the
  immediate parent decision.

The horizon contains roughly 33–100 refresh opportunities per maker level and
40 random-taker decision ticks before focal entry. It is adequate for this
single-decision mechanism probe. It is not evidence of stationarity or
long-horizon survival.

## 3. Focal strategy and scale intervention

The focal strategy submits exactly one market child of `TargetQty` after a
delivered two-sided snapshot becomes available at or after the decision time.
No other policy field changes across cells.

Proposed target quantities:

| scale | target | executable rationale |
|---|---:|---|
| S1 | 0.5 ABC (`50,000,000` base units) | equals nominal one-level ask depth with two makers; below it with four/six |
| S2 | 2 ABC (`200,000,000`) | walks multiple nominal levels in every composition but remains below total initial depth |
| S3 | 5 ABC (`500,000,000`) | equals four-maker nominal five-level depth, exceeds two-maker depth, remains below six-maker depth |

This is execution/flow scale. Focal cash is fixed and nonbinding; it is not a
capital intervention. Increasing unused cash would not alter the order and
cannot establish capacity.

## 4. Composition intervention

| composition | adaptive makers | delayed random takers | background accounts | nominal background ABC | nominal background USD |
|---|---:|---:|---:|---:|---:|
| C0 baseline | 4 | 8 | 12 | 1,200,000 ABC | USD 1,200,000,000 |
| C+ maker-rich | 6 | 6 | 12 | 1,200,000 ABC | USD 1,200,000,000 |
| C− flow-rich | 2 | 10 | 12 | 1,200,000 ABC | USD 1,200,000,000 |

This is replacement, not removal or additive capital. Total account count and
nominal endowment are fixed. The participant class changes as a bundle:
objective, action policy, latency, and fee schedule change together. The pilot
therefore identifies a population-composition contrast, not the isolated
coefficient of maker count, latency, or fees.

## 5. Candidate development matrix

Candidate development seeds: `1009`, `1013`, `1019`.

These values are not reserved. Before preregistration, a provenance-only check
must establish that no result for the exact proposed protocol has been
inspected. If any candidate is ineligible, replacement must be selected by a
predeclared deterministic rule without running candidate worlds.

The economic matrix is:

```text
3 compositions × 3 target sizes × 3 development seeds = 27 worlds
```

Only the immediate policy runs. The pilot does not re-run the historical
immediate-versus-TWAP comparison.

Two technical controls are proposed after the adapter/evidence fixes:

1. duplicate C0/S2/1009 in fresh processes under `GOMAXPROCS=1` and `14`;
2. require byte-identical canonical plan, economic execution digest, and
   reconstructed report while allowing only declared runtime metadata to
   differ.

Total proposed executions: 29. Technical duplicates do not add independent
economic observations.

## 6. Opportunity and eligibility contract

A world is eligible for economic interpretation only if all stages reconstruct:

1. the focal actor received a positive two-sided ABC/USD snapshot;
2. delivered price levels and quantities at the decision are retained;
3. target quantity and balances made the action feasible under declared venue
   rules;
4. exactly one focal request was sent with the registered side/type/quantity;
5. admission or rejection is linked by request and order identity;
6. every exchange fill links to the order with exact quantity, price, fee,
   side, trade ID, and match timestamp;
7. fill plus cancelled residual equals requested quantity;
8. actor-local child totals equal independent exchange reconstruction;
9. terminal mark is a later positive two-sided book midpoint and is labelled
   as a mark, not a fill;
10. canonical evidence is complete, terminated, hashed, and provenance-bound.

The opportunity denominator is the delivered ask curve at decision time. The
minimum diagnostics are ask quantity at the touch, cumulative ask quantity
through five levels, executable quantity at each proposed target, and expected
mechanical sweep notional before the focal request. These are descriptive
states observed by the focal actor; they are not hidden exchange access.

## 7. Primary and supporting outcomes

### Primary outcome

All-in target implementation shortfall in basis points, reconstructed from
exchange fills, quote fees, decision midpoint, and the separately labelled
terminal mark for any unfilled residual.

### Admissibility map

A seed/scale/composition world is admissible when:

- evidence and provenance are valid;
- no unpriced fee, overfill, schedule mismatch, or quantity gap exists;
- completion is exactly 100%; and
- target implementation shortfall is at most 10 bp.

The 10 bp budget is the configured 5 bp taker fee plus a prospective 5 bp
price/opportunity-cost budget. Report the full continuous outcomes even when
the threshold fails. For each composition, report every tested scale as 0/3,
1/3, 2/3, or 3/3 admissible. The headline screening region contains only 3/3
points; it may be non-monotone.

Passing S3 means the upper boundary was not observed. Failing all points means
no admissible scale in this grid, not universally zero capacity.

### Supporting outcomes

- filled and unfilled quantity; completion ratio;
- filled-only and target shortfall, in quote units and bp;
- executed notional, quote fees, and utilized quote capital;
- delivered touch and five-level ask depth;
- pre-request mechanical sweep estimate versus realized execution;
- first/last fill latency, rejection count, and terminal cancellation;
- spread and midpoint at decision and terminal time;
- background trade count/quantity by class, if independently reconstructible.

No trading PnL, annualized return, wealth ranking, market-impact law, or
equilibrium statistic is a pilot endpoint.

## 8. Exact arithmetic contract

For buy side, base precision `B`, target `Q`, fills `(q_k,p_k)`, filled `Q_f`,
decision midpoint `M_0`, terminal midpoint `M_T`, and quote fees `F`:

```text
C        = Σ floor-or-exact-fixed-point(q_k × p_k / B)
U        = Q - Q_f
S_fill   = C - Q_f × M_0 / B + F
C_target = C + U × M_T / B
S_target = C_target - Q × M_0 / B + F
IS_bps   = 10,000 × S_target / (Q × M_0 / B)
```

The reconstructor must match the engine's checked fixed-point operations and
rounding exactly; prose algebra does not override implementation units. A
sell-side fixture is required even though the pilot action is buy-only.

`M_T` values a residual obligation. It does not pretend that residual was
executed, and `UnfilledQty` must always accompany target shortfall.

## 9. Contrasts and uncertainty

For each seed and scale, compute:

```text
Δ+ = outcome(C+) - outcome(C0)
Δ− = outcome(C−) - outcome(C0)
```

Primary contrasts are `Δ+` and `Δ−` for target shortfall bp at S1/S2/S3: six
predeclared contrasts. Report all three paired seed differences, their median,
and range. Do not treat event rows or parent fills as independent observations.
Do not report asymptotic p-values from three worlds.

Secondary mechanism checks compare delivered ask depth and completion in the
same blocks. A composition claim requires both valid treatment realization and
valid focal outcome. If roster counts differ as intended but delivered depth
does not, report `OPPORTUNITY_NOT_SEPARATED`.

Common world seeds preserve per-index noise streams for overlapping actors,
but composition changes actor IDs, queue priority, and the number of streams.
Those differences are part of class replacement. “Same seed” is not described
as identical shocks after the intervention.

## 10. Cell classification

Each world receives exactly one top-level classification:

| Class | Rule | Interpretation |
|---|---|---|
| `VALID_ACTIVE` | eligible request and outcome reconstruct | include in response map |
| `VALID_INACTIVE` | focal had a delivered two-sided opportunity but policy made no request | valid policy non-activation; unexpected for Immediate and must be explained |
| `NO_OPPORTUNITY` | no eligible delivered two-sided state before horizon | identification limitation |
| `ECONOMIC_FAILURE` | valid active world violates completion/cost mandate | retain as capacity evidence |
| `INVALID_EVIDENCE` | missing/malformed/unreconciled packet | no economic interpretation; localize defect |
| `FAILED_PROCESS` | process/timeout/resource failure | operational failure, not market collapse |

Missing valuation, illiquidity, rejection, and incomplete execution remain in
the table. No survivor-only estimator is permitted.

## 11. Evidence and implementation prerequisites

Execution remains blocked on both items below:

1. **Canonical adapter/provenance:** an external experiment adapter must expose
   every registered field, validate fixed account/endowment totals, hash the
   typed plan, bind source/toolchain/config, and refuse stale output reuse.
2. **Independent evidence reconstruction:** a separate analyzer must rebuild
   the delivered opportunity, request, fills, fees, cancellation, terminal
   mark, and outcomes from canonical exchange/receipt evidence; corruption
   tests must fail closed and evidence must be behavior-neutral.

No policy, matching, fee, latency, endowment, or actor economic logic should be
changed to satisfy these prerequisites.

## 12. Finite resource budget

The economic matrix contains 27 worlds × 4,000 one-millisecond runner steps,
or 108,000 world-steps, plus two duplicates. Historical runtime metadata is not
provenance-bound, so this is an engineering estimate, not a measurement.

Proposed hard budget after implementation and authorization:

- at most 29 total executions;
- at most four concurrent processes;
- at most 15 minutes wall time for the complete screening batch;
- at most 4 GiB aggregate resident memory;
- at most 1 GiB retained canonical evidence and reports;
- stop before execution if a one-world measured preflight extrapolates above
  any bound; do not lower evidence requirements to pass.

No registered long-run, capacity campaign, R2/SV1D cell, or holdout is part of
this budget.

## 13. Promotion, null, and stop rules

- An invalid world stops its matched block; repair requires new provenance.
- A valid null is retained and does not license target-size, composition,
  horizon, or threshold tuning.
- A valid economic failure is a result and remains in the admissibility map.
- `NO_OPPORTUNITY` does not support or refute the mechanism.
- If all S1 cells fail mechanically, stop: the laboratory is unsuitable for
  the question.
- If the composition arms do not separate delivered depth, stop before causal
  interpretation; propose a new design separately.
- If C− systematically loses a two-sided decision state, report market
  availability failure rather than silently conditioning on surviving books.

Three development seeds support screening only. If the pilot supports
continuation, freeze source/config/analyzer and prospectively specify a finite
confirmation set of new seeds and at least one unseen composition or size.
Confirmation must not be selected after viewing candidate outcomes. It tests
replication in the model, not empirical realism.

## 14. Empirical boundary

The pilot is simulation-internal. A later empirical comparison may use
message/book data such as LOBSTER to compare spread, depth, replenishment, and
hypothetical immediate sweep cost in compatible units. Public order-book data
does not reveal the maker/noise objective labels used here, so it cannot
validate the composition coefficient. No empirical corridor is registered in
this draft.

## 15. Authorization boundary

Owner decisions still required:

1. authorize implementation and review of the two bounded prerequisites;
2. accept or amend the proposed 10 bp mandate and exact 3×3 matrix;
3. approve a deterministic untouched-status rule for the candidate seeds;
4. after both prerequisite gates pass, preregister and authorize development
   execution separately.

Until then:

```text
PROTOCOL: DRAFT
IMPLEMENTATION: NOT AUTHORIZED
PILOT EXECUTION: NOT AUTHORIZED
SEEDS: PROPOSED, NOT RESERVED
HOLDOUTS: NONE SELECTED OR CONSUMED
R2/SV1D: CLOSED AND UNAFFECTED
```
