# ME-002 — Identical-policy deployment assignment

## Question and scope

Does swapping fixed directed latency assignments change outcomes for otherwise identical policies/resources?

Strongest intended claim: **CAUSAL** (proposed, not established).
Start L0/L1 only; same policy/capital, separately assigned deployment; geography deferred.
ME-001's 1 ms focal transport and 1 ms decision polling were held fixed, so
its size-dependent response map is not evidence for or against latency effects.

Authorization and current stage: [authoritative registry](../../registry.json).
The bounded development screen is complete and independently reviewed; see
the [report](report.md), [machine result](result.json) and
[development lock](development-lock-2026-09-23.md). Its 24-cell economic budget
is exhausted. This card itself authorizes no additional world. The older
prospective design text below remains historical planning context where it
differs from the locked protocol.

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

Use the existing [immediate ABC/USD policy](../ME-001/report.md) as the first
focal policy, not S14/S16: it has the strongest current execution/evidence
contract. Hold the C0 4-maker/8-taker ecology, target, fees, capital, IDs,
matching and shocks fixed. Compare the *same actor* under five proposed
deployment assignments: equal-delay reference; fast network/fast computation;
fast network/slow computation; slow network/fast computation; slow network/slow
computation. A separate two-identical-actor location-swap fixture must show
that actor ID and scheduler tie order are not being mistaken for deployment.
The exact milliseconds are **not locked**; choose them prospectively after
checking the 1 ms runner resolution and measured opportunity/quote lifetimes.

Primary question: under the declared one-venue execution mandate, does a
network-versus-computation delay change *all-assigned completion and target
shortfall* at fixed policy and resources? The proposed local mechanism predicts
more missed short-lived opportunities when realized action delay crosses their
lifetime; its finite-grid falsifier is a reversed miss/completion response in
eligible matched worlds after the delay crossing is verified. Two seeds cannot
establish equivalence or general latency irrelevance. If
nominal profiles yield identical realized delivery, this is a clock-resolution
or wiring limitation, not evidence that latency is economically irrelevant.
Competing explanations: changed clock phase, endogenous book divergence,
policy-ID tie order and a fixed transport component that dominates the
nominal difference. Report realized inbound receipt, decision wait, outbound
arrival, match and response receipt, plus action-latency/opportunity-lifetime
and computation-delay/decision-period ratios.

Readiness gap: [executionlab.NewSim](../../../../simulations/executionlab/sim.go)
currently applies one `ExecutionLatency` to request, response and public-feed
paths, with no independently scheduled computation delay. The generic
[LatencyConfig](../../../../simulation/latency.go) has separate logical channels,
but this experiment's adapter does not expose them. `ParentCount>1` also
alternates trade side, so it cannot serve as an unchanged identical-policy
swap without an explicit fixture. Needed before protocol lock: configurable
deployment adapter outside the policy, exact logical delay and phase fixtures,
stable actor-specific RNG namespaces, and fail-closed realized-time joins.
Do not add wall-clock sleeps or city labels.

The owner subsequently requested a 2×2 network-versus-processing design,
holding decision cadence fixed, with both 0.5- and 5-ABC targets. The newer
[protocol](protocol.md) supersedes the earlier five-profile/one-size
proposal above where they conflict. The four factorial arms, three
development seeds and finite 24-world budget were subsequently locked,
executed and reported under their exact candidate; this old planning
paragraph is not a current launch instruction.
