# ME-002 — Identical-policy deployment assignment

## Question and scope

Does swapping fixed directed latency assignments change outcomes for otherwise identical policies/resources?

Strongest intended claim: **CAUSAL** (proposed, not established).
Start L0/L1 only; same policy/capital, separately assigned deployment; geography deferred.

Authorization and current stage: [authoritative registry](../../registry.json).
No study execution or readiness implementation is authorized by this card.

## Entities and readiness

Policies: [S01](../../policy-catalogue.md#s01), [S14](../../policy-catalogue.md#s14), [S16](../../policy-catalogue.md#s16).
Actor resources, objective, financing and deployment must be specified independently
at protocol lock; the catalogue describes existing scope and missing capabilities.
Instrument/venue and information constraints follow the scope above; fields not
specified here are open design work, never permission to invent effective defaults.

Source/test starting points:
- [simulation/latency.go](../../../../simulation/latency.go)
- [simulation/gateway.go](../../../../simulation/gateway.go)
- [simulation/scheduler.go](../../../../simulation/scheduler.go)
- [simulation/observation_receipts.go](../../../../simulation/observation_receipts.go)

Required measurement/opportunity contract: Realized receipt/decision/order/match times, clock phase and opportunity lifetime.

Dependencies: [ME-000](../ME-000/idea.md).
These are readiness dependencies, not authorization to run ancestors.

## Historical evidence and limits

- HISTORY: [research/multivenue-crossvenue-latency-race-2026-08-15.md](../../../../research/multivenue-crossvenue-latency-race-2026-08-15.md). Use its own run/config/analyzer identity and verdict; do not relabel as current-main evidence.
- HISTORY: [research/v2-4-l1p2-noise-phase-results.md](../../../../research/v2-4-l1p2-noise-phase-results.md). Use its own run/config/analyzer identity and verdict; do not relabel as current-main evidence.

Nominal changes with identical realized delivery identify a resolution issue, not latency irrelevance.

## Tentative design and next action

Design a fixed-profile swap and realized-time evidence fixture; do not add it to ME-001.
Numeric choices remain prospective unless the authoritative draft below specifies
them. Budget for this authoring task is zero market worlds. Future work needs its
own protocol, modules/N/A reasons, fixtures, formula domains and owner budget.
