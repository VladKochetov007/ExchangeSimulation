# ME-003 — Execution-instruction contrast

## Question and scope

How does one order instruction change completion and cost for a fixed finite objective?

Strongest intended claim: **CAUSAL** (proposed, not established).
One contrast at a time; identical economic objective; allocation and routing held fixed.

Authorization and current stage: [authoritative registry](../../registry.json).
The owner's bounded continuation permits readiness work after the reviewed
ME-002 result. The [prospective protocol](protocol.md) is a DRAFT; this card
does not authorize a world. The older two-seed sketch below is historical
and is superseded by the draft's three paired development seeds where they
conflict.

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

The first contrast should be a fixed-price **limit IOC versus limit FOK** on
the same immediate finite buy mandate, not a four-way policy tournament.
The venue implements both TIFs and tests FOK preflight/partial IOC, but the
[executionlab parent](../../../../simulations/executionlab/execution.go)
currently submits a market child with default GTC. The generic
[actor gateway](../../../../actor/actor.go) can send TIF-specific orders;
the parent adapter, provenance contract and independent analyzer do not yet
establish this order-instruction comparison. Passive GTC/deadline execution
is a distinct later study; the term-carry passive exit is not a substitute.

Primary question: when the delayed delivered ask book makes partial
execution feasible, how does an all-or-none instruction change completion,
unfilled obligation and all-in target cost relative to IOC? Falsifier for an
economic benefit: no practically useful improvement in the registered
completion/cost mandate over the tested opportunity set; identical outcomes
without any partial-depth opportunity are an identification limit, not a
policy victory. Competing explanations include limit-price protection,
queue evolution during transport, FOK admission rejection, spread/fees and
terminal residual marking. Price cap, target, decision time, actor resources,
venue allocation and background ecology must be identical across arms.

Minimal readiness fixtures: partial displayed depth, full displayed depth,
cancel/reject race, fee asset and reservation release, fill-before-response,
terminal unfilled quantity and explicit no-opportunity case. The independent
join must recover `request_id`, TIF, acceptance/rejection, fill quantity,
cancel remainder, charged fees and terminal mark from canonical evidence.
Primary endpoint is the all-assigned *joint* completion/target-shortfall
response; never rank a cheap rejected FOK order as successful because its
unfilled target was merely marked. ME-001's C−/5-ABC result demonstrates that
specific estimator pitfall, not an IOC/FOK causal effect.

Smallest **proposed**, unregistered screen after fixtures: IOC/FOK × 2 and
5 ABC × two new development seeds in C0 = at most eight economic worlds,
plus two controls. The fixed price cap and 4-second-or-longer drain horizon
must be prospectively selected from the venue's tick/latency contract, not
from observed favorable outcomes. Future ceiling: 15 minutes, 4 GiB peak
RSS, 1 GiB evidence, subject to actual preflight. No seeds or prices are
assigned here; implementation and execution authorization remain **false**.
