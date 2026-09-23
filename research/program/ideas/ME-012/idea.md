# ME-012 — Explicit long-run capital dynamics

## Question and scope

Under an explicit allocation/entry/exit mechanism, does the ecology coexist, stabilize, cycle or go extinct?

Strongest intended claim: **GAME-THEORETIC** (proposed, not established).
A separately authorized long-horizon protocol with clock-artifact checks.

Authorization and current stage: [authoritative registry](../../registry.json).
No study execution or readiness implementation is authorized by this card.

## Entities and readiness

Policies: [S07](../../policy-catalogue.md#s07), [S10](../../policy-catalogue.md#s10), [S12](../../policy-catalogue.md#s12), [S14](../../policy-catalogue.md#s14).
Actor resources, objective, financing and deployment must be specified independently
at protocol lock; the catalogue describes existing scope and missing capabilities.
Instrument/venue and information constraints follow the scope above; fields not
specified here are open design work, never permission to invent effective defaults.

Source/test starting points:
- [simulation/clock.go](../../../../simulation/clock.go)
- [simulations/multivenue/sim.go](../../../../simulations/multivenue/sim.go)

Required measurement/opportunity contract: Capital shares, perturbation recovery, flow accounting, regime durations and extinction.

Dependencies: [ME-011](../ME-011/idea.md).
These are readiness dependencies, not authorization to run ancestors.

## Historical evidence and limits

- HISTORY: [research/v2-design.md](../../../../research/v2-design.md). Use its own run/config/analyzer identity and verdict; do not relabel as current-main evidence.
- HISTORY: [research/v2-r2-sv1d-iteration-closeout.md](../../../../research/v2-r2-sv1d-iteration-closeout.md). Use its own run/config/analyzer identity and verdict; do not relabel as current-main evidence.

Stationary prices, strategic equilibrium and forced calendar periodicity are distinct; learning/reinvestment is new economics.

## Tentative design and next action

Propose adaptation and source/sink rules only after shorter payoff studies; no implementation now.
Numeric choices remain prospective unless the authoritative draft below specifies
them. Budget for this authoring task is zero market worlds. Future work needs its
own protocol, modules/N/A reasons, fixtures, formula domains and owner budget.
