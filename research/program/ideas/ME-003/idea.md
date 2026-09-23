# ME-003 — Execution-instruction contrast

## Question and scope

How does one order instruction change completion and cost for a fixed finite objective?

Strongest intended claim: **CAUSAL** (proposed, not established).
One contrast at a time; identical economic objective; allocation and routing held fixed.

Authorization and current stage: [authoritative registry](../../registry.json).
No study execution or readiness implementation is authorized by this card.

## Entities and readiness

Policies: [S01](../../policy-catalogue.md#s01), [S04](../../policy-catalogue.md#s04), [S05](../../policy-catalogue.md#s05).
Actor resources, objective, financing and deployment must be specified independently
at protocol lock; the catalogue describes existing scope and missing capabilities.
Instrument/venue and information constraints follow the scope above; fields not
specified here are open design work, never permission to invent effective defaults.

Source/test starting points:

- [types/order.go](../../../../types/order.go)
- [tests/ioc_crossing_test.go](../../../../tests/ioc_crossing_test.go)
- [simulations/executionlab/execution.go](../../../../simulations/executionlab/execution.go)

Required measurement/opportunity contract: Rejected/unadmitted, filled, cancelled and resting quantities; deadline/exit cost.

Dependencies: [ME-000](../ME-000/idea.md), [ME-001](../ME-001/idea.md).
These are readiness dependencies, not authorization to run ancestors.

## Historical evidence and limits

- HISTORY: [research/executionlab-2026-08-15.md](../../../../research/executionlab-2026-08-15.md). Use its own run/config/analyzer identity and verdict; do not relabel as current-main evidence.

Passive noncompletion and adverse selection can dominate fee savings; generic deadline executor is not yet verified.

## Tentative design and next action

Choose one supported IOC/FOK/GTC/passive contrast after execution evidence readiness.
Numeric choices remain prospective unless the authoritative draft below specifies
them. Budget for this authoring task is zero market worlds. Future work needs its
own protocol, modules/N/A reasons, fixtures, formula domains and owner budget.
