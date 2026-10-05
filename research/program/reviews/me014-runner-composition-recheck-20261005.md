# ME-014 runner composition recheck — 2026-10-05

## Status and scope

This is a bounded, read-only source synthesis, not a candidate-promotion review.
Review execution: **COMPLETED**. Substantive verdict: **NOT_ISSUED**. The
independent reviewer was a fresh Sol-6 medium context, Kepler
(`01a10e0c-4ac1-71e2-969e-360321acfdbb`), which did not edit files or run tests
or simulations. I independently checked the relevant public paths after its
report. No economic world, seed, or holdout was run or inspected.

Pinned source: branch `feature/e2-atomic-payment-20260927`, commit
`1fa38f8b391bb1b1e5c9cb56c065c35c289ef502`, tree
`4e4208a4c0f00a5cf6dede7a61cb8c28bc2cc6d6`.

This note refines, but does not replace, the earlier
[runner integration-boundary diagnostic](me014-runner-integration-boundary-20261005.md).
That record remains intact as the historical result of its source review.

## Findings

### Legacy funding calendar: bounded non-payment is possible, ownership is not

`multivenue.Config.normalize` defaults `FundingIntervalSeconds` to 28,800 s.
The E2 draft's observation horizon is 140 minutes (8,400 s). In source,
`StartAutomation` runs the legacy funding check on one-second ticks, and the
first tick anchors the legacy due time at that tick plus the configured
interval. Therefore, **assuming the configured legacy interval remains greater
than the fixed world horizon**, no legacy cash funding payment should become
due in that particular world. For the current proposal this means keeping the
legacy interval at 28,800 s for both venues and representing the 300/480-s E2
calendars only in the separate E2 calendar. If either per-venue legacy override
is set to 300 or 480 s, the old settlement path becomes due repeatedly during
the world. This is a source-derived timing argument, not an assembled-run
observation.

That does not make the 8-hour calendar a funding-replacement interface. The
legacy price-update path continues publishing `FundingRate` values and its
relative `NextFunding` field. Existing `FundingCarryArbitrageur` and
`TermCarryAllocator` inputs consume that legacy representation. E2's absolute
N/S calendars and micro-bp rates must not be represented by those fields, and
an E2 policy must not make decisions from them. A bounded adapter could leave
the legacy default dormant over the finite horizon while publishing E2 through
a distinct actor feed, but it must prove all of the following:

- every configured legacy interval is strictly longer than the fixed world
  horizon, and the E2 300/480-s calendars are not copied into legacy interval
  fields;
- no E2 policy consumes the legacy rate/calendar;
- no legacy settlement cash event occurs;
- the manifest and actor-visible evidence label the legacy rate as
  non-operative for E2;
- extending the horizon past that boundary fails validation rather than
  silently enabling a second funding regime.

The earlier diagnostic's categorical rejection of an interval workaround was
too broad if read to say that an 8-hour configured interval necessarily creates
a second payment within the 140-minute world. The corrected conclusion is
narrower: the interval can establish absence of legacy **payments within a
pinned finite horizon**, but cannot establish exclusive E2 ownership or safe
actor information semantics by itself. No interval or economic parameter is
changed by this note.

### Typed delayed publication: composable in principle

`BaseActor.AddMarketDataFeed` delivers `MarketDataMsg` values to a feed-specific
callback. The callback need not use the default `MDFunding` decoder, which
expects `*FundingRate`; `MarketDataMsg.Data` can carry a distinct scale-aware E2
value. `simulation.Mount` supports delayed gateways and deterministic phase
delivery. This makes an isolated E2 feed **feasible in principle** without a
new market-data enum or modification to the actor package.

It is not yet integrated or validated. The publisher must own immutable payload
values because the generic publisher does not clone arbitrary `any` values.
Tests must bind source time, publication time, delayed scheduled delivery,
receipt time, payload identity and dropped/reordered delivery; demonstrate no
actor can use the payment at settlement time; and show ordinary participants
cannot decode or consume the E2 payload on their normal gateway. This finding
does not establish that the current `NewSim` builder exposes all needed wiring.

### Typed early stop: control-flow composition is feasible in principle

The deterministic runner propagates a `BeforeTimestampPhase` error before it
drains same-time venue phases. A private typed sentinel could therefore carry
an already-committed economic stop out of the runner while preserving the
no-same-time-ingress boundary. The runner also calls its shutdown hook on this
path.

The current `Sim.Run` treats every runner error as failed execution, and the
normal command path does not emit the contract's typed `E2_EARLY_STOP` result.
Checkpoint finalization is currently bound to the planned horizon; a censored
stop must instead seal required evidence at the actual stop time. This is not
solved by catching an arbitrary error and returning success. The wrapper must
distinguish a registered economic sentinel from appender, source, close,
receipt-finalization and other structural errors; retain payments already
committed at that epoch; write the full sorted terminal-reason set; and mark
later outcomes censored. No such path has been tested.

### Read-only risk preflight: current public calls do not satisfy the contract

`MarkedAccount` is strict about valuation but calls `riskMark`; for perps,
`riskMark` accepts the stored `FundingRate.MarkPrice` whenever
`MarkAvailable=true`. `FundingRate` has no mark timestamp, so the caller cannot
establish from that value alone how recent its source state is. `CommitMarkEpoch`
binds available stored marks into a coherent snapshot but stamps the snapshot
with the current time; it does not refresh or attest the source age.

The obvious refresh, `UpdatePerpPrices`, updates marks and executes the exchange
liquidation path as part of that pass. It cannot be used as a harmless
read-only preflight before deciding whether *any* exposed account is
unpriceable. `CheckLiquidations` is also a mutating sweep and reports
unpriceable account conditions through evidence rather than returning a
whole-roster preflight result. Thus the existing public calls do not prove the
selected contract's sequence:

1. identify all accounts with relevant open exposure;
2. validate a fresh, coherent post-payment risk mark for each, without mutation;
3. if any are unpriceable, stop before any same-time liquidation or ingress;
4. otherwise liquidate using exactly the marks just preflighted.

Changing this to accept the last stored mark under a new age rule would be a
prospective risk-semantic amendment, not a measurement-only workaround. It is
not made here. The exact `t⁻` admissibility rule remains unresolved by the
current public API.

## Integration boundary and minimum acceptance tests

The public `RunnerConfig.BeforeTimestampPhase` callback is fixed when the
runner is constructed. `multivenue.NewSim` currently owns that callback for
source capture; `Sim.Run` starts exchange automation internally. The exchange
does expose the `FundingSettlementEpochAppender` interface, but the current
multivenue canonical writer has no settlement-epoch adapter exposed to an
external consumer that returns the writer's actual event identities. A mount
decorator could intercept a venue pump in principle, but source capture alone
does not give it the private recorder state or a supported way to append all
payment receipts to the same canonical global stream. That possibility has not
been implemented or proven equivalent.

The smallest bounded integration acceptance suite, before any economic world,
is:

1. finite-horizon legacy guard: zero legacy settlement records inside 140
   minutes with the 28,800-s legacy interval; a 300/480-s legacy override is
   rejected; extending past the 28,800-s due boundary is rejected; E2 actors
   ignore the legacy `FundingRate` view;
2. delayed E2 view: exact typed payload and scale survive publication/delivery,
   and actor receipt cannot precede the configured delay;
3. joint epoch: coincident N/S payments are appended atomically with actual
   canonical global sequence receipts; source, payment, balance, reserve and
   outcome mutations fail closed;
4. post-payment risk: mixed priceable/unpriceable exposed accounts cause zero
   same-time liquidation/ingress and a valid censored stop; when all are
   priceable, the ordinary sweep uses the identical coherent mark set;
5. stop lifecycle: payments already committed at the stop survive; exact stop
   time, complete reason set, stream termination/hash and receipt finalization
   are recorded; structural failures never become valid economic stops;
6. OFF/ON determinism and unchanged legacy-mode controls, including a fresh
   process replay.

The current repository constraint forbids modifying library source or
directories. These acceptance conditions cannot be waived to fit that
constraint. The next engineering boundary is to identify a supported,
out-of-library composition point that can own the joint callback, appender,
risk preflight and stop lifecycle together. If none exists, the owner or
maintainer must provide/authorize an appropriate extension point before
implementation. No E2 economic run is authorized by this finding.

## Decision

ME-014 remains **PREPARE; `run=false`**. The settlement arithmetic and
prospective economic contract remain unchanged. A finite 8-hour legacy
interval is conditionally compatible with this one 140-minute horizon, but it
does not close the integration gate. The typed feed and stop wrapper are
promising composition seams, not accepted implementations. The blocking
requirement is a fresh, coherent, all-exposed-account risk preflight and
same-mark liquidation boundary that the current API does not expose, plus a
supported way to bind E2 epoch appends into the canonical stream. No
independent substantive ACCEPT/REJECT was issued for this composition.
