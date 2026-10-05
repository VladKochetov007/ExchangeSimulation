# ME-014 runner integration boundary — source diagnostic, 2026-10-05

## Review execution and verdict

Execution: **COMPLETED**. Verdict: **NOT_ISSUED**. This was a bounded, read-only source diagnostic to locate the production integration boundary; it was not a candidate-promotion review and did not issue a substantive ACCEPT or REJECT.

Independent reviewer: Sol-6 medium, fresh read-only context `01a10d8f-65ba-77c1-aa64-f775542f1634` (Archimedes). The reviewer did not edit source, run tests or simulations, or inspect holdouts.

Reviewed candidate: branch `feature/e2-atomic-payment-20260927`, commit `04953a4f4eee59aa0868c75d433ed62ee90d15a7`, tree as checked on 2026-10-05. The reviewed source includes the previously scoped joint `exchange.SettleFundingEpoch` mechanics checkpoint. This diagnostic did not extend that checkpoint's acceptance.

## Question and source scope

Can the selected ME-014 funding contract be connected to the current production runner, exchange automation, actor feed, risk boundary and output lifecycle without changing historical/default funding semantics?

The inspection covered `simulation/runner.go`, `exchange/exchange.go`, `exchange/funding.go`, `exchange/funding_settlement.go`, `simulations/multivenue/sim.go`, `simulations/multivenue/term_carry.go`, the prospective [settlement contract](../ideas/ME-014/settlement-contract-20260927.md), and the existing joint-coordinator review. No economic world was run.

## Findings

### 1. The E2 pre-instant hook does not own or replace legacy settlement

**Directly established from source:** In deterministic mode the runner invokes `BeforeTimestampPhase` before draining the same-timestamp venue automation, ingress, egress and actor phases (`simulation/runner.go`, `runDeterministicPhases` and `drainDeterministicPhases`). `DefaultExchange.StartAutomation` independently registers `CheckAndSettleFunding` as a periodic automation job (`exchange/exchange.go`). The E2 coordinator itself does not advance or disable the legacy `FundingRate.NextFunding` schedule.

Consequently, inserting `SettleFundingEpoch` in the existing callback alone does not install an exclusive E2 settlement path. The legacy sweep remains live after that callback. Its first schedule anchor and its `FundingRate` calendar are separate from the selected absolute N/S E2 calendars.

**Not established by execution:** No assembled E2 world was run, so this report does not claim an observed duplicate cash posting. The source proves coexistence of two active paths if the callback is wired naively; an extra or out-of-phase legacy transfer is the resulting integration risk. Exact boundary behavior must be tested in a production-path fixture.

**Required boundary:** Add an opt-in E2 funding mode that owns the complete registered set of E2 venue/perpetual calendars and suppresses legacy settlement only for those covered instruments. Preserve the existing legacy path and defaults for all non-E2 configurations. Test exactly-once posting across coincident, non-coincident, first-tick and ordinary legacy boundaries.

### 2. The existing actor funding view is not the E2 rate contract

**Directly established from source:** The existing funding event/API exposes `FundingRate.Rate` as the legacy integer-basis-point quantity together with its legacy next time/interval. `TermCarryAllocator` copies that value into `FundingRateBps` and schedules its interpretation from `NextFunding` (`simulations/multivenue/term_carry.go`). The selected E2 calculation instead returns a scale-carrying `QuantizedFundingRate` in micro-basis-point units and uses absolute, venue-specific phase calendars plus a prior-window rate.

Converting the E2 integer units into the legacy field without a distinct type and contract would reinterpret a scaled value as whole basis points; publishing the legacy simple current rate would instead give the actor a different economic signal from the settlement rate. The existing event is therefore not an adequate representation of the E2 rate/calendar view.

**Required boundary:** Define an opt-in, versioned E2 actor-visible funding message containing the rate units/scale, venue and instrument identity, publication and effective times, next scheduled instant, source-window identity, and unavailable status. Deliver it through the configured delayed information path. Prove actors cannot observe the settlement rate before the registered publication boundary. Keep legacy feeds unchanged outside E2.

### 3. The economic early-stop contract lacks a runner/output completion path

The prospective settlement contract already specifies `E2_EARLY_STOP`, a sorted set of terminal reasons, and the distinction between a valid economic/valuation stop and `INVALID_CONTRACT_OR_EVIDENCE`. It also requires valid coincident venue payments to be committed before stopping, and labels later outcomes censored. This is not an unresolved economic choice.

**Directly established from source:** `SettleFundingEpoch` returns a result containing terminal reasons separately from its error. The current `BeforeTimestampPhase` callback returns only `error`; `Sim.Run` treats runner errors as failed execution and the normal multivenue output path finalizes its terminal snapshots/reports after successful completion. The ordinary `captureVenueRisk` observation is not the all-exposed-account, post-payment risk preflight required by the contract.

**Required boundary:** Carry a typed terminal outcome through the runner and `Sim.Run`, perform the declared all-exposed-account post-payment risk preflight before same-time ingress, and seal a valid censored economic result with `E2_EARLY_STOP` plus its complete reason set. Structural/evidence errors must remain invalid execution and must not be represented as valid economic stops. Test that already committed payment evidence survives an economic stop and that no same-time actor or liquidation phase runs afterward.

This is missing wiring for an already selected contract, not permission to weaken strict valuation or add a backstop.

### 4. The existing public automation configuration has no funding replacement hook

**Directly established from source:** `exchange.AutomationConfig` supports mark/index, price-update, collateral, liquidation and lifecycle settings, but it exposes no funding-settlement policy or opt-out. `DefaultExchange.StartAutomation` unconditionally registers `CheckAndSettleFunding` in deterministic mode (and starts its legacy settlement loop otherwise). The ordinary `multivenue.Sim.Run` always calls `StartAutomation` for every venue.

Therefore the normal `NewSim`/`Sim.Run` composition cannot currently give E2 exclusive funding ownership using only supported configuration. A very long legacy interval is not an acceptable substitute: it changes a published schedule, leaves the legacy actor view semantically misleading, and does not establish a complete-roster ownership invariant. Reimplementing the exchange's other automation outside `StartAutomation` would duplicate framework behavior and is not a bounded adapter.

**Authority boundary:** The repository instruction for this worktree prohibits adding source/types/logic inside the library and requires extension through existing public composition points. No suitable public funding replacement point exists. I therefore did not edit `exchange/` or `simulation/` to add one. The smallest reasonable future library API, if separately authorized, is an optional injected funding-settlement handler (defaulting exactly to the current legacy handler) with an explicit complete-contract ownership boundary; it must not silently change the default or let an E2 handler coexist with legacy settlement on the same perpetual. That API should be reviewed as a general extension point before the E2 simulation adapter is implemented.

## Promotion implication

The existing coordinator acceptance remains scoped to source-level joint settlement mechanics. It does not accept E2 runner integration. ME-014 remains `PREPARE`, `run=false`.

If a supported library extension point is added/authorized, the next bounded engineering gate is an opt-in production composition that, as one tested boundary:

1. freezes and validates the complete deployment roster and calendar set;
2. gives those E2 contracts exclusive ownership over their funding settlements while leaving legacy mode unchanged;
3. captures/settles under the runner-owned pre-instant ordering;
4. publishes the scale-aware E2 view only through delayed actor feeds;
5. performs the contract's post-payment risk preflight and typed early-stop handling;
6. seals and independently reconstructs complete, unavailable, shortfall, structural-failure and censored outcomes.

Before implementing this composition, pin its public API and status/output transition tests. Then seek a bounded independent review of the exact integrated candidate. No economic protocol or run becomes authorized merely by passing these mechanical gates.

Until then, E2 production integration is blocked by a specific missing public extension point plus the repository's no-library-edit constraint—not by review-service availability or an unresolved funding convention.

## Not reviewed or concluded

- No end-to-end E2 runner build, appender, risk preflight, or actor publication was accepted here.
- No tests were run by the reviewer; prior test results remain attributed to their exact recorded commits.
- No funding activation, carry profitability, payment frequency, market survival, or causal effect was measured.
- No historical trajectory, prior review verdict, protocol, or economic result was changed.
- No seed, economic world, confirmation case, or holdout was run or consumed.
