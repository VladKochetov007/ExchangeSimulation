# ME-000 — Immediate-execution pilot readiness

## Question and scope

Can the existing pilot plan and delivered-opportunity-to-outcome path be independently reconstructed?

Strongest intended claim: **MECHANICAL** (proposed, not established).
Implementation/evidence tasks only; no change to economics or study matrix.

Authorization and current stage: [authoritative registry](../../registry.json).
The separately authorized F1/F2 readiness work is complete at `b25223a`;
see the [scoped report](report.md) and [machine result](result.json).
This card's proposal text below is retained as the original task definition.

## Entities and readiness

Policies: [S01](../../policy-catalogue.md#s01), [S02](../../policy-catalogue.md#s02), [S06](../../policy-catalogue.md#s06).
Actor resources, objective, financing and deployment must be specified independently
at protocol lock; the catalogue describes existing scope and missing capabilities.
Instrument/venue and information constraints follow the scope above; fields not
specified here are open design work, never permission to invent effective defaults.

Source/test starting points:

- [simulations/executionlab/sim.go](../../../../simulations/executionlab/sim.go)
- [simulations/executionlab/execution.go](../../../../simulations/executionlab/execution.go)
- [simulations/executionlab/sim_test.go](../../../../simulations/executionlab/sim_test.go)
- [cmd/executionlab/main.go](../../../../cmd/executionlab/main.go)

Required measurement/opportunity contract: Exact effective configuration and independent request/order/fill/fee/cancel/mark joins.

Dependencies: none; this is an entry readiness question.
These are readiness dependencies, not authorization to run ancestors.

## Historical evidence and limits

- HISTORY: [research/NEXT-STUDY-READINESS-PACKET.md](../../../../research/NEXT-STUDY-READINESS-PACKET.md). Use its own run/config/analyzer identity and verdict; do not relabel as current-main evidence.

Fixture mismatch, unsupported config value, evidence mutation or behavior change is a readiness failure.

## Tentative design and next action

Complete F1 and F2 only after separate owner implementation authorization.
Numeric choices remain prospective unless the authoritative draft below specifies
them. Budget for this authoring task is zero market worlds. Future work needs its
own protocol, modules/N/A reasons, fixtures, formula domains and owner budget.

## F1 — canonical configuration/provenance adapter (historical proposal)

- Trace `SimConfig` normalization in executionlab/sim.go and CLI wiring in
  cmd/executionlab/main.go. The current CLI does not expose the full composition matrix.
- Implement a reusable validation/plan function with a thin external adapter.
  Bind effective maker/noise counts, target, policy, duration, latency, clock,
  fee/endowment constants, realized indexed maker cadences, source and toolchain.
- Reject unsupported effective fees/endowments; do not duplicate unchecked
  constants or silently substitute defaults. If a configuration seam is needed,
  propose it as default-preserving and independently review it.
- Reject malformed/unregistered fields before startup and refuse output reuse.
- Acceptance fixtures: all 27 proposed plans have distinct canonical identities;
  output path alone does not alter the typed plan digest; economic fields do.
  Distinguish raw-file digest from canonical typed-plan digest.
- Candidate, binary, config and plan must agree through existing provenance paths.
  No production runner or 29-world batch is authorized by these fixtures.

## F2 — independent opportunity/execution reconstruction (historical proposal)

- Inspect executionlab/execution.go (actor report), executionlab/sim.go (terminal
  mark), actor/events.go and exchange evidence adapters.
- Instrument only the necessary delivered decision state, observation age,
  request/order/trade identities, venue timestamps and actor receipt times.
  Preserve latest delivered state versus policy-retained state explicitly.
- Reconstruct requested, admitted, rejected, filled and cancelled quantities,
  per-fill notional and charged-asset fees, terminal mark provenance and
  both shortfall measures without calling the actor report implementation.
- Accepted quantity equals fills plus cancelled residual at completion;
  a rejected request has no fabricated order/cancellation.
- Fixtures: complete, partial, full rejection, zero displayed facing depth,
  changing book during latency, one-sided/missing mark, fee asset/rounding,
  late delivered earlier fill, mismatched/unanchored/overrun fills.
- Mutation acceptance: dropped/duplicate/misordered/mismatched or incomplete
  packets fail closed. Fresh-process determinism and evidence on/off tests
  preserve the economic digest. Reuse the existing binary evidence contract.
- Inspect tests before execution; only expressly authorized synthetic fixtures
  belong to PREPARE. The producer's last two-sided cache must be represented
  truthfully; do not change economic decisions under an evidence-only task.

The readiness packet is authoritative. Neither fix may change fees, roster,
capital, matching, policy or the 10 bp mandate to make tests or future worlds pass.
