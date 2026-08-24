# V2-5 P4 — funding/carry causal identification protocol

Status: **evidence and decision protocol preregistered after P3e lifecycle
closure and before any P4 implementation, immutable cell config, preflight, or
outcome run.** This document does not authorize a simulation. A separate
pre-run addendum must freeze the funding intervention, paired seeds, horizon,
capital, clocks, and immutable configs before implementation or outcome
inspection.

Parent result: P3e lifecycle is **SUPPORTED (screening)** at `e1ae6f3`, only
for the finite-term execution contract. P0 remains **SUPPORTED (screening)**
only for activation. Signed-price remains complete at merge `320262e` and
hardening `5afdd45`. None of those results is reopened here.

## Scope and question

The next question is narrower than “does funding anchor price?”:

> Does a changed, delayed public funding observation alter independently
> recomputed expected funding, exact fully costed net carry, the participant's
> target inventory, actual spot/perpetual submissions and fills, and only then
> measured basis dynamics in the registered direction?

The six links are ordered. Later evidence cannot substitute for an earlier
missing link. A basis difference without the complete participant chain is
`NOT IDENTIFIED`, not support. An inventory/order difference without a basis
response is an activated mechanism with a falsified or mixed market endpoint,
not a funding anchor.

P3e may be reused only as the frozen finite-term exit mechanism. The P4 design
may not tune its passive slice, deadline semantics, retry rules, demand,
displayed depth, spread, clocks, venue minimum, or counterparty flow to obtain
the desired basis outcome. Any justified change to those inputs is a different
experiment with a new preregistration.

## Forbidden shortcut

Funding is an observed participant input and ledger transfer. It must never:

- write a spot, perpetual, index, mark, quote, fair-value, or target market
  price;
- move a book without an ordinary admitted order and canonical fill;
- read an exchange's future or undelivered funding state;
- force the second leg, hide partial/orphan inventory, or fabricate closure;
- be inferred from aggregate PnL or a basis path after the fact; or
- share an intervention with demand, liquidity, spread, latency, clock, or
  execution-policy changes.

A code path with any such shortcut is `INVALID`, even if basis narrows.

## Required observable chain

### 1. Delivered premium and funding observation

Each qualifying decision must join to delayed local receipt evidence for the
spot book, perpetual book, and `MDFunding` publication. The record must retain
source venue, symbol, publication time, sequence, scheduled delivery, actual
delivery, link ordinal/digest, decision frontier, next funding time, and age.
The premium must be independently recomputed from delivered executable or
registered reference prices with explicit presence and signed-price domain
checks.

A missing, stale, future, duplicated, reordered, or identity-mismatched source
fails this link. Numeric zero is never an unavailable observation.

### 2. Independently recomputed expected funding

The analyzer must recompute signed expected funding from the delivered rate,
candidate direction, declared notional, exact interval count/horizon, next
settlement timing, and rational scaling. The participant's stored expected
funding field is only a comparison target. Sign-mirror fixtures must prove that
positive-premium/positive-funding and negative-premium/negative-funding cases
produce the correct receipt/payment directions.

No stale-rate extrapolation or whole-bps rounding may create or erase an edge.
Intermediate numerators, denominators, rounding mode, and any overflow/domain
failure must be retained.

### 3. Exact net carry after every declared cost

The independent calculation must retain expected funding and each cost
separately:

- entry and expected exit fees for both legs;
- spot borrow or lending over the actual declared horizon;
- balance-sheet capital charge;
- initial/maintenance margin and liquidation-risk charge;
- latency and non-atomic leg-risk charge; and
- any separately declared minimum required return.

Net carry is expected funding plus any registered premium realization less all
named costs, using exact rational or fixed-point arithmetic tied to observed
notional and time. Omitted costs, hidden free borrowing, double-counted premium,
forged signs, unregistered rounding, or a field that cannot be recomputed make
the link fail.

This screen does not claim realized profitability. Such a claim would also
need all entry/exit fills, fees, borrow, funding ledger transfers, orphan risk,
marking, and terminal inventory reconstructed through real closure.

### 4. Changed target inventory

The treatment must change the participant's target spot/perpetual inventory in
the direction implied by the independently positive net carry. The evidence
must show the prior target, new target, current independently reconstructed
positions, target gap, capital/margin constraint, and named action or defer
reason.

A changed expected-funding number with an unchanged target is a missing link.
A target changed by a direct price rule, hidden global state, or a simultaneous
policy/capital change is `INVALID`.

### 5. Actual submitted and filled spot/perpetual orders

Target intent must join through local receipt frontier, gateway request,
canonical admission, ordinary order lifecycle, canonical fills, actor receipt,
fees, and independently replayed spot/perpetual positions. Submitted, accepted,
partially filled, fully filled, rejected, cancelled, and absent legs remain
separate endpoints. The analyzer must preserve leg ordering, latency,
non-atomic exposure, and any orphan inventory.

An unsubmitted target, rejected child, or zero observed fill does not satisfy
the order-flow link. It remains reportable activation/execution evidence.

### 6. Resulting basis dynamics

Only after links 1–5 pass may the screen score basis. The basis tape must be
reconstructed from canonical spot and perpetual market evidence, never from an
actor target or stored “fair value.” Qualifying order/fill times must anchor a
preregistered signed event study and paired aggregate endpoint. For positive
premium and a long-spot/short-perpetual carry target, the registered direction
is a reduction in signed premium; the sign mirror applies for negative premium.

The numeric horizons, censoring rule, overlapping-event rule, weighting,
liquidity/presence gate, paired seed set, and primary aggregate must be fixed
in the immutable-cell addendum. Looking across horizons and selecting the best
one is prohibited.

## Independent analyzer and adversarial requirements

The P4 analyzer must replay raw persisted evidence and emit every link as a
separate per-decision/per-term endpoint plus cell and paired aggregates. It
must not reduce the chain to one boolean. It must distinguish
`not_applicable`, `not_exercised`, `not_observed`, observed numeric zero, and
observed nonzero values.

Before any market run, adversarial fixtures must reject at least:

- future, stale, duplicate, reordered, or wrong-identity funding receipts;
- a premium recomputed from a future or absent book;
- reversed funding sign or candidate direction;
- omitted, duplicated, rounded-away, overflowed, or forged cost components;
- a stored net-carry value inconsistent with exact recomputation;
- a target mutation without changed net carry, or changed net carry without a
  target mutation;
- forged gateway requests, canonical admissions, fills, fees, or leg identity;
- a submitted-but-unfilled order counted as order-flow completion;
- partial/orphan inventory counted as a matched carry term;
- same-timestamp or cross-file scan order used as global causality;
- basis computed from the participant's target price or actor state; and
- a basis move without a complete links 1–5 chain.

Runtime/offline exact artifact digests, receipt frontiers, canonical order
chains, position replay, funding direction/duplication, derivatives, and
conservation must all pass. Raw evidence remains retained and unpruned.

## Classification

The later immutable-cell addendum may narrow but not weaken these rules:

- **INVALID**: any evidence, information-frontier, arithmetic, canonical-chain,
  position, accounting, conservation, digest, or forbidden-shortcut failure.
- **NOT EXERCISED**: the registered funding intervention is delivered but no
  paired decision reaches the preregistered premium/net-carry eligibility
  condition.
- **NOT IDENTIFIED**: any one of the six ordered links is missing in either
  required paired seed, including a target without filled orders or basis
  movement without the participant chain.
- **FALSIFIED AT ACTIVATION**: links 1–3 pass, but the participant does not
  change target inventory in the registered direction.
- **FALSIFIED AT EXECUTION**: the target changes, but the registered ordinary
  spot/perpetual order-and-fill intervention does not occur.
- **SUPPORTED (screening)**: every link passes in every required paired seed,
  treatment produces the registered target and filled-order direction, and all
  paired primary basis effects have the preregistered convergence sign.
- **MIXED**: the complete chain activates but paired primary basis directions
  disagree or only a subset of required pairs has a measurable basis endpoint.
- **FALSIFIED**: the complete inventory/order intervention occurs in all
  required pairs but no paired primary basis effect has the registered
  convergence sign.

Secondary exposure, funding, fill, residual, and execution-timing endpoints
remain reportable under their own names and cannot upgrade the primary verdict.

## Required immutable-cell addendum before work continues

Before implementation or a P4 preflight, commit a separate addendum and configs
that fix:

1. the sole funding intervention and its exact control/treatment serialization;
2. paired development seeds and untouched holdout policy;
3. horizon, term count, funding intervals, analysis cutoff, and censoring;
4. participant capital, exact cost parameters, target rule, and position caps;
5. the already-passed P3e exit contract without economic retuning;
6. all market population, demand, liquidity, spread, latency, and clock inputs;
7. the primary event-study horizon and paired basis statistic;
8. activation, execution, and minimum measurable-basis conditions; and
9. source revision, binary hashing, completion sentinels, extraction order,
   raw-retention policy, and machine-readable verdict schema.

Until that addendum exists, P4 has no authorized cells and no funding-anchor
claim can be scored.

## Dated carry boundary

Dated futures are out of scope. They require a separate preregistration with
observable time to expiry, settlement terms and price, delivery/settlement
risk, expiry-specific financing, and convergence endpoints. A fixed quote
clock or direct time-to-expiry target-price rule cannot stand in for that
protocol.
