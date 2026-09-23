# ME-007 — Three-asset executable conversion

## Question and scope

Can both triangular routes be reconstructed with legal quantities, real fee assets and residual costs?

Strongest intended claim: **MECHANICAL** (proposed, not established).
Three assets, one venue first; finite balances; actual sequential legs.

Authorization and current stage: [authoritative registry](../../registry.json).
No study execution or readiness implementation is authorized by this card.

## Entities and readiness

Policies: [S17](../../policy-catalogue.md#s17).
Actor resources, objective, financing and deployment must be specified independently
at protocol lock; the catalogue describes existing scope and missing capabilities.
Instrument/venue and information constraints follow the scope above; fields not
specified here are open design work, never permission to invent effective defaults.

Source/test starting points:

- [simulations/feesim/triarb.go](../../../../simulations/feesim/triarb.go)
- [simulations/multivenue/naive.go](../../../../simulations/multivenue/naive.go)

Required measurement/opportunity contract: Asset-specific cashflows, fee rounding, partial legs and residual inventory.

Dependencies: [ME-000](../ME-000/idea.md).
These are readiness dependencies, not authorization to run ancestors.

## Historical evidence and limits

- HISTORY: [research/no-arbitrage-audit.md](../../../../research/no-arbitrage-audit.md). Use its own run/config/analyzer identity and verdict; do not relabel as current-main evidence.

Start with mechanics. A later endogenous opportunity hypothesis needs its own reviewed protocol.

## Tentative design and next action

Design directional/unit/rounding fixtures and identify missing full-depth accounting.
Numeric choices remain prospective unless the authoritative draft below specifies
them. Budget for this authoring task is zero market worlds. Future work needs its
own protocol, modules/N/A reasons, fixtures, formula domains and owner budget.
