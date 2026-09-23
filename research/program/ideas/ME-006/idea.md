# ME-006 — Informed maker by explicit arbitrage 2x2

## Question and scope

How do information-mediated quoting and executed arbitrage interact in venue synchronization?

Strongest intended claim: **CAUSAL** (proposed, not established).
Both mechanisms off, maker information only, router only, both; finite resources normalized.

Authorization and current stage: [authoritative registry](../../registry.json).
No study execution or readiness implementation is authorized by this card.

## Entities and readiness

Policies: [S14](../../policy-catalogue.md#s14), [S16](../../policy-catalogue.md#s16).
Actor resources, objective, financing and deployment must be specified independently
at protocol lock; the catalogue describes existing scope and missing capabilities.
Instrument/venue and information constraints follow the scope above; fields not
specified here are open design work, never permission to invent effective defaults.

Source/test starting points:

- [simulations/multivenue/stoikov.go](../../../../simulations/multivenue/stoikov.go)
- [simulations/multivenue/router.go](../../../../simulations/multivenue/router.go)
- [simulation/feed_gateway.go](../../../../simulation/feed_gateway.go)

Required measurement/opportunity contract: Fresh local/remote receipt frontier, quote changes, fills, dislocation and activity denominators.

Dependencies: [ME-005](../ME-005/idea.md).
These are readiness dependencies, not authorization to run ancestors.

## Historical evidence and limits

- HISTORY: [research/v2-2b-price-discovery-smoke-results.md](../../../../research/v2-2b-price-discovery-smoke-results.md). Use its own run/config/analyzer identity and verdict; do not relabel as current-main evidence.

Information may erase router opportunities; a missing trade-channel estimand must remain missing.

## Tentative design and next action

Draft a new, resource-normalized 2x2 only after route/opportunity readiness.
Numeric choices remain prospective unless the authoritative draft below specifies
them. Budget for this authoring task is zero market worlds. Future work needs its
own protocol, modules/N/A reasons, fixtures, formula domains and owner budget.
