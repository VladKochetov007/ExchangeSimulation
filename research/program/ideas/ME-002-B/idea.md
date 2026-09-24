# ME-002-B — first-decision cadence × network delay (PLAN ONLY)

Status: **PROPOSED, NOT REGISTERED FOR EXECUTION**. This is the optional
ME-002B decision-cadence plan requested after the completed
[ME-002 development screen](../ME-002/report.md). It is a separate idea,
not an amendment, confirmation or rescue of ME-002. No source change,
seed reservation, economic world or holdout is authorized by this card.

## One question and intended claim

For the *same* finite immediate ABC/USD buyer, resources, C0 counterparties
and venue rules, does the policy's poll interval alter the **first eligible
order's** timing and fill, or modify the effect of a fixed directed network
delay? This small screen cannot locate a threshold where polling dominates
transport; that would need separately chosen cadence levels and conditions.
The strongest possible future claim is a conditional simulation-internal
**CAUSAL development contrast for first-action cadence**. It would not
establish the economics of a repeatedly adaptive or high-frequency strategy.
The existing immediate policy sends exactly one child; its `PollInterval`
controls when that first decision can occur, not a sequence of revisions.

ME-002's 24 valid worlds found that 120 ms actor processing changed selected
information but not order arrival under a one-second earliest-decision gate,
while the 1 versus 90 ms network assignment changed arrival and produced
mixed-sign 5-ABC filled-fraction effects. Those are the motivation, not
ME-002B outcomes. The retained [pre-outcome timing basis](../ME-002/timing-baseline.md)
found a 1 ms runner/policy tick, 100 ms default periodic snapshots, and
sampled 5-ABC depth episodes of roughly 73–200 ms in ME-001 C0 evidence.
These are **snapshot-observed proxies**, not continuous executable lifetimes.

## Mechanism, alternative explanations and entities

- **Policy:** existing [`Immediate`](../../../../simulations/executionlab/execution.go)
  finite-target buy, with one child, fixed target and objective; its only
  proposed treatment parameter here is `PollInterval`. An event-driven
  variant is not established by this source and is not silently added.
- **Actor:** one fixed-endowment parent account with the same target,
  liability/exit rule, fees and client identity within matched worlds.
- **Deployment:** fixed delayed public-feed/request/response paths and zero
  added processing delay within a cadence pair; network delay is the other
  explicit factor. Synthetic milliseconds are logical time, not geography.
- **Venue:** one ABC/USD price-time book with unchanged background population,
  matching, fee schedule and market-data publication clock.

The predicted local channel is:

`public quote publication -> delivered snapshot -> next eligible policy tick
-> request send -> venue arrival -> fill or non-fill`.

If a coarser poll increases the *gate-adjusted* wait for the first action, it
may move venue arrival across a relevant quote/depth state and change the
network assignment's fill effect. This is a *hypothesis*, not a monotonic law.
At the one-second gate the 80 ms tick adds 40 ms versus the 1 ms tick; the
1-to-90 ms request-leg change adds 89 ms, and feed plus request can add up
to 178 ms. The proposed grid therefore cannot demonstrate that polling
dominates transport even if an interaction is observed.
Alternative explanations are the one-second decision gate, poll/gate phase,
100 ms snapshot cadence, changed selected quote, endogenous book response,
and a filled-fraction ceiling. A nominal poll contrast with the same actual
decision/arrival timestamp is non-activation, not latency irrelevance.

## Tentative finite design, not a locked protocol

A minimal future screen could hold the C0 4-maker/8-taker ecology, 5-ABC
target, actor resources and processing delay fixed, and cross:

| Factor | Candidate low | Candidate high | Source-grounded reason |
|---|---:|---:|---|
| Directed feed/request/response delay | 1 ms | 90 ms | Existing ME-002 synthetic comparison; retain its explicit channel decomposition. |
| Policy poll interval | 1 ms | 80 ms | 1 ms runner resolution versus sub-100-ms sampled depth episodes; 80 ms is below the 100-ms periodic snapshot clock but large enough to expose a gate-phase wait. |

At most three **new, prospectively selected development seeds** would give
at most 12 economic worlds, plus two fresh-process technical controls. No
seed labels are reserved here. Four simulated seconds is only a candidate
horizon: a future lock must verify the last decision, order, fill and delayed
response drain at both poll levels. A 0.5-ABC ceiling arm is not included
by default; ME-002's all-full 0.5-ABC result makes it weak for this primary
endpoint. No extra geography, instruction, actor-count, capital or maker
information factor belongs in this first matrix.

The candidate 80 ms ticker does **not** divide the one-second gate: its next
tick is 1.04 s, whereas the 1 ms ticker can act at 1.00 s. That phase is an
explicit part of this candidate's treatment, not proof of an average cadence
effect. Before lock, use same-time and neighboring-phase fixtures and choose
either a stated fixed-phase estimand or a *separately budgeted* phase design.
Do not append a favorable phase after seeing economic outcomes. Locating a
general threshold would require additional prospectively chosen cadence
levels and conditions, not interpolation from this 2×2 screen.

Potential primary endpoint: all-assigned filled ABC / fixed target ABC,
including valid no-send/rejected/partial worlds. The proposed paired
network, cadence and interaction contrasts use each seed's four worlds;
seed/world, not tick/fill rows, is the uncertainty unit. Secondary timing
quantities should include publication-to-receipt, receipt-to-eligible-tick,
tick-to-send, send-to-arrival, response delivery and sampled
opportunity-lifetime ratio where both boundaries are observed. Target
shortfall is secondary only with valid decision/terminal marks; an unfilled
residual marked cheaply is not execution. Three seeds cannot prove a
population threshold or equivalence.

Decompose the raw receipt-to-decision interval before interpreting cadence.
For a valid first decision, reconstruct `t_usable`, the first time the actor's
cache held the positive two-sided quote needed by its policy; then
`t_eligible = max(t_usable, DecisionAfter)` and
`gate_adjusted_poll_wait = decision_at - t_eligible`. Report the separate
`max(0, DecisionAfter - t_usable)` gate wait and the selected-quote age.
If the actor never has a usable quote or never decides, mark the relevant
interval censored/undefined rather than zero. This decomposition is a
proposed estimator requiring independent source/evidence fixtures, not a
number already reconstructed by ME-002.

## Readiness and smallest next test

The producer has a configurable `PollInterval` and emits `decision_tick`
evidence, but the current [`executionpilot` replay](../../../../experiment/executionpilot/reconstruct.go)
hard-codes a 1 ms poll, one-second gate and expected tick count for ME-002.
No existing ME-002 analyzer result therefore certifies a coarse-poll world.
Before any protocol lock, parameterize a separately versioned replay contract
and test exact tick count/phase, selected quote identity, missing/duplicate
ticks, processing-before-decision order, no-action ticks, delayed response
drain and ledger/fee arithmetic. A minimal **fixture**, not an economic
world, should show the 1.00 versus 1.04 s first decision under otherwise
identical delivered quotes, then a nearby gate phase where that difference
changes. The same fixture must fail closed on omitted ticks. Verify fresh-
process determinism and evidence neutrality after a pinned implementation.

If the intended question is instead **repeated strategy frequency**, use a
separately proposed repeated-decision policy and evidence contract—e.g. a
maker's explicit quote clock with fixed inventory objective. Do not present
this one-child immediate policy as that experiment. The historical
[`FixedDistanceMaker`](../../../../simulations/multivenue/naive.go) has a
configurable quote interval, but its inventory/quote/competition channels
would make it a new population and economic question, not a direct ME-002
replication.

Smallest next authorized action: decide whether the narrow first-action
question merits a separate protocol and readiness implementation. If so,
lock exact effective configuration, new seed labels, horizon, finite resource
budget, primary contrast, opportunity proxy, cancellation/terminal rules and
independent review before any world. Estimated future compute: no more than
12 development worlds plus two controls, provisionally under 15 minutes,
4 GiB peak RSS and 1 GiB evidence **only after measured preflight**. A valid
null, absent opportunity or non-activated timing contrast remains reportable.
