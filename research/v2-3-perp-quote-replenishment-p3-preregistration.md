# V2-3 P3 — confirmed perpetual passive-quote replenishment

Status: **preregistered before implementation, configuration rendering, or
simulation.** This immutable first screen implements the mechanism stated in
[`v2-3-perp-quote-replenishment-p3-design.md`](v2-3-perp-quote-replenishment-p3-design.md).

## Question

Can an `ABC-PERP` Stoikov maker, using only its own confirmed order lifecycle,
identify a materially depleted passive quote and issue the existing ordinary
refresh at the next quote tick? This screen does not ask whether the result
stabilizes price, repairs P3c, improves funding/carry, or produces realistic
market depth.

## Fixed mechanism and arms

Each seeded world uses the same short, full-evidence parent configuration and
the existing parent maker cadence/order policy. The only economic delta is:

| arm | `perp_maker_replenish_below_bps` |
| --- | ---: |
| A | 0 |
| B | 5,000 |

The implementation must reject values outside `[0, 10,000]`. Arm B's strict
policy is `known_resting * 10,000 < target * 5,000`; equality does not refresh.
The actor makes no new timer, request type, price, size target, information
feed, fee, or matching exception. It executes its pre-existing pair cancel /
submit sequence only on its normal quote tick.

The rendered differ permits only seed, labels/provenance, raw-evidence / V2
receipt recorder settings, and the one declared field. It rejects changes to
quote interval/phase, price or size policy, all inventory policy, market-data
or request latency, router, carry allocator, funding, population, fees, book
construction, post-only, or ordering fields.

## Evidence contract

Both arms record `perp_quote_replenishment_decision`; only B can mark
`refresh_due=true` for a below-threshold confirmed residual. Independent
analysis uses venue `OrderAccepted`, `OrderRejected`, `OrderFill`, and
`OrderCancelled` records joined by venue/client/request/order identity. It
derives residuals with arbitrary-precision arithmetic and rejects a missing,
duplicated, reordered, wrong-side, or unjoined lifecycle relation. It may not
read actor state or substitute `maker_state` targets for resting quantity.

The decision record is evidence-only and must not change execution hash.
Final-horizon decisions are explicitly censored; nonterminal missing venue
outcomes are failures. The complete cell requires the final `greeks.json` and
`latency.json` sentinels, exact persisted-evidence artifact hash, V2 receipt /
frontier validation, replenishment artifact, and ordinary lifecycle/viability
extraction before any pruning.

## Primary score and activation gate

For each seed, score in this fixed order:

1. evidence parses, all decision/order chains join, and the replay reports no
   structural or threshold mismatch;
2. B has at least one confirmed partial own perpetual quote fill whose residual
   becomes strictly below half target while price/target otherwise remain
   unchanged at the next decision; B refreshes exactly then;
3. A has zero policy-triggered replenishment actions; and
4. ordinary maker/book viability remains observable: nonzero quote-decision
   count, accepted or explicit rejected quote activity, nonzero perp snapshot
   count, nonzero two-sided snapshot share, and nonzero trade volume.

If item 2 has zero qualifying partial fills, the seed is **NOT EXERCISED**.
If it occurs but B does not refresh, B is **FALSIFIED**. A policy-triggered
control refresh, a new timer, a side/threshold mismatch, a request without the
declared ordinary pathway, fabricated resting state, or evidence failure makes
the cell **INVALID**, not a favorable market result. Do not alter 5,000 bps,
the population, timing, size, or prices after the screen.

Secondary descriptive values are policy-trigger count; residual fraction at
trigger; request/accept/reject/cancel/fill counts; refresh latency in quote
ticks; target and displayed touch depth; quote presence; two-sided share;
trade volume; and terminal maker inventory. They do not rescue primary
activation and are not price-stability targets.

## Required mutations and limits

The implementation/analyzer gate requires deterministic fixtures and
mutations for a suppressed residual decrement, swapped bid/ask decrement,
non-strict `<=` boundary, duplicate fill, dropped cancellation, refresh when
above threshold, and a fake refresh when policy is disabled. Each must be
caught by independent replay. Recorder on/off and fresh-process process-count
checks must retain the execution hash.

An activated screen establishes only an auditable local passive-replenishment
policy. A future lifecycle experiment must be newly preregistered and cannot
reuse this screen to claim finite carry completion, funding identification, or
economic realism.
