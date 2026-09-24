# ME-005 — Prefunded two-venue executable arbitrage

## Question and scope

When can finite non-atomic routing capture an executable dislocation and change its persistence?

Strongest intended claim: **CAUSAL** (proposed, not established).
Two venues proposed; current integrated harness uses three, so isolate wiring readiness explicitly.

Authorization and current stage: [authoritative registry](../../registry.json).
No study execution or readiness implementation is authorized by this card.
The source-grounded [2026-09-24 readiness assessment](readiness-20260924.md)
ends at **READY AFTER FOUR SPECIFIC FIXES**; it is not a protocol or result.
The [prospective preparation contract](prepare-contract-20260924.md) selects
the local closeout convention and scopes those four fixes before implementation.
The [implementation checkpoint](readiness-implementation-20260924.md) records
the current partial acceptance work and its independent design limitation;
it is not a locked protocol or an economic result.

## Entities and readiness

Policies: [S14](../../policy-catalogue.md#s14), [S15](../../policy-catalogue.md#s15).
Actor resources, objective, financing and deployment must be specified independently
at protocol lock; the catalogue describes existing scope and missing capabilities.
Instrument/venue and information constraints follow the scope above; fields not
specified here are open design work, never permission to invent effective defaults.

Source/test starting points:

- [simulations/multivenue/router.go](../../../../simulations/multivenue/router.go)
- [simulations/multivenue/router_test.go](../../../../simulations/multivenue/router_test.go)

Required measurement/opportunity contract: Depth/fees/admission/balances, completed group cashflow, segregated inventory and costed closeout.

Dependencies: [ME-000](../ME-000/idea.md).
These are readiness dependencies, not authorization to run ancestors.

## Historical evidence and limits

- HISTORY: [research/v2-2b-price-discovery-smoke-results.md](../../../../research/v2-2b-price-discovery-smoke-results.md). Use its own run/config/analyzer identity and verdict; do not relabel as current-main evidence.
- HISTORY: [research/multivenue-crossvenue-latency-race-2026-08-15.md](../../../../research/multivenue-crossvenue-latency-race-2026-08-15.md). Use its own run/config/analyzer identity and verdict; do not relabel as current-main evidence.

Static edge success is MECHANICAL only; historical completed routes did not prove reduced sampled edge.

## Tentative design and next action

The [current router](../../../../simulations/multivenue/router.go) is a
three-endpoint positive-spot FOK router with independently funded venue legs.
It checks one-lot displayed depth and fee-adjusted touch edge and records
both legs and residual base, but requires **exactly three** venue endpoints;
its completed quote cashflow is not net profit after local-inventory unwind
or transfer. A two-venue study therefore needs explicit harness/router
readiness, not a renamed three-venue historical result.

Primary question: when two delayed local feeds show an *executable* positive
edge, how often can a finite prefunded non-atomic router complete both legs,
and does enabling it reduce the duration of that edge in the tested ecology?
Falsifier for the proposed market effect: eligible episodes and submitted
routes occur, yet matched router-on/off worlds show no prospectively
meaningful edge-duration reduction. No eligible episodes mean an
identification limit; positive cashflow in a static fixture is mechanical,
not emergent alpha. Competing explanations include a shared price anchor,
background order flow, feed delay, leg race, rejected FOK, and venue-local
inventory left after two fills.

Before any endogenous protocol, require static positive/zero/negative
all-in-edge fixtures with quantity/fee rounding, stale delivered-feed
frontiers, one-leg rejection, partial/mismatched evidence, venue-specific
cash/asset reconciliation and a costed *optional* unwind/transfer policy.
The decision denominator is executable delayed-feed opportunities; report
actual request/admission/fill, residual venue inventories and incomplete
groups separately. No omniscient same-time price scan may be described as
the actor's opportunity. If the two-venue adapter cannot preserve exact
frontier and account segregation, label this study NOT READY.

Smallest **proposed**, unregistered endogenous screen after those gates:
router off/on × two new development seeds = four worlds at one fixed lot,
fee, latency and ecology, plus two determinism controls. A horizon must cover
multiple opportunity lifetimes and the costed closeout window; do not copy
ME-001's four seconds. Future upper budget: 15 minutes, 4 GiB peak RSS,
1 GiB evidence **subject to measured preflight**, with stop on invalid
evidence. No seeds, lot or horizon are locked and zero worlds or source
changes are authorized by this readiness card.
