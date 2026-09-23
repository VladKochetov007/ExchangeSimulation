# ME-004 — Venue allocation at fixed policy

## Question and scope

How does FIFO versus the implemented pro-rata variant change allocation under fixed policies?

Strongest intended claim: **CAUSAL** (proposed, not established).
Freeze policy before venue rule; adaptive response is a separate study.

Authorization and current stage: [authoritative registry](../../registry.json).
No study execution or readiness implementation is authorized by this card.

## Entities and readiness

Policies: [S01](../../policy-catalogue.md#s01), [S06](../../policy-catalogue.md#s06).
Actor resources, objective, financing and deployment must be specified independently
at protocol lock; the catalogue describes existing scope and missing capabilities.
Instrument/venue and information constraints follow the scope above; fields not
specified here are open design work, never permission to invent effective defaults.

Source/test starting points:
- [matching/prorata.go](../../../../matching/prorata.go)
- [matching/prorata_test.go](../../../../matching/prorata_test.go)
- [types/order.go](../../../../types/order.go)
- [tests/order_test.go](../../../../tests/order_test.go)

Required measurement/opportunity contract: Allocation, queue/request order, fees, partial fills and same-total-resource splitting.

Dependencies: [ME-000](../ME-000/idea.md), [ME-003](../ME-003/idea.md).
These are readiness dependencies, not authorization to run ancestors.

## Historical evidence and limits

- HISTORY: [research/validation-audit.md](../../../../research/validation-audit.md). Use its own run/config/analyzer identity and verdict; do not relabel as current-main evidence.

Account fragmentation and quote splitting are not new capital; do not generalize one pro-rata variant to all venues.

## Tentative design and next action

Locate exact allocation implementation and minimal rounding/tie fixtures, then propose one contrast.
Numeric choices remain prospective unless the authoritative draft below specifies
them. Budget for this authoring task is zero market worlds. Future work needs its
own protocol, modules/N/A reasons, fixtures, formula domains and owner budget.
