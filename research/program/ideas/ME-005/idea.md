# ME-005 — Prefunded two-venue executable arbitrage

## Question and scope

When can finite non-atomic routing capture an executable dislocation and change its persistence?

Strongest intended claim: **CAUSAL** (proposed, not established).
Two venues proposed; current integrated harness uses three, so isolate wiring readiness explicitly.

Authorization and current stage: [authoritative registry](../../registry.json).
No study execution or readiness implementation is authorized by this card.

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

Specify static positive/negative edge and one-leg failure fixtures before any endogenous protocol.
Numeric choices remain prospective unless the authoritative draft below specifies
them. Budget for this authoring task is zero market worlds. Future work needs its
own protocol, modules/N/A reasons, fixtures, formula domains and owner budget.
