# ME-001 — Immediate-execution quantity/composition screen

## Question and scope

How does fixed-resource participant replacement change admissible immediate execution quantity and shortfall?

Strongest intended claim: **CAUSAL** (proposed, not established).
One ABC/USD venue; immediate buy; 2/10, 4/8, 6/6 maker/taker compositions and 0.5/2/5 ABC targets. Three proposed seeds, 27 economic worlds plus two controls. No seeds reserved.

Authorization and current stage: [authoritative registry](../../registry.json).
ME-000 readiness is closed. The owner's overnight goal conditionally permits
development execution only under the [prospective lock](protocol.md) after
its independent review; this card alone is never a run authorization.

## Entities and readiness

Policies: [S01](../../policy-catalogue.md#s01), [S06](../../policy-catalogue.md#s06).
Actor resources, objective, financing and deployment must be specified independently
at protocol lock; the catalogue describes existing scope and missing capabilities.
Instrument/venue and information constraints follow the scope above; fields not
specified here are open design work, never permission to invent effective defaults.

Source/test starting points:

- [simulations/executionlab/sim.go](../../../../simulations/executionlab/sim.go)
- [simulations/executionlab/execution.go](../../../../simulations/executionlab/execution.go)
- [simulations/feesim/mm.go](../../../../simulations/feesim/mm.go)
- [simulations/feesim/taker.go](../../../../simulations/feesim/taker.go)

Required measurement/opportunity contract: All-assigned availability/admissibility and fully paired target shortfall; absent opportunities can leave contrasts undefined.

Dependencies: [ME-000](../ME-000/idea.md).
These are readiness dependencies, not authorization to run ancestors.

## Historical evidence and limits

- HISTORY: [research/executionlab-2026-08-15.md](../../../../research/executionlab-2026-08-15.md). Use its own run/config/analyzer identity and verdict; do not relabel as current-main evidence.
- HISTORY: [research/NEXT-STUDY-READINESS-PACKET.md](../../../../research/NEXT-STUDY-READINESS-PACKET.md). Use its own run/config/analyzer identity and verdict; do not relabel as current-main evidence.

No profitability, capital-capacity, equilibrium or empirical-realism inference. Preserve the merged draft's falsifier and 10 bp all-in mandate.

## Tentative design and next action

Review the committed protocol lock, which inherits the merged draft's
economic matrix and records only explicit prospective amendments.
Numeric choices remain prospective unless the authoritative draft below specifies
them. Budget for this authoring task is zero market worlds. Future work needs its
own protocol, modules/N/A reasons, fixtures, formula domains and owner budget.

Authoritative protocol: [ME-001 lock](protocol.md), incorporating the
[merged historical draft](../../../../research/market-ecology-capacity-pilot-protocol-DRAFT.md).
