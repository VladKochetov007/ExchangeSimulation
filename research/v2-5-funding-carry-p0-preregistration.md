# V2-5 P0 — funding-incentive integrity preregistration

Status: **preregistered before implementation and before a V2-5 outcome run.**

Parent: `2f711fc` on `autoresearch/ffa-ecology-gen0`.  This is a narrow
mechanism/evidence gate, not a basis-convergence or realism experiment.  It
does not alter V2-3/V2-4 population, phase, spread, latency, price-discovery,
or signed-price policies.

## Why this gate is necessary

The current `multivenue.CarryArbitrageur` subscribes only to spot and perpetual
book snapshots.  `targetCarry` applies a fixed midpoint-basis threshold, and
no funding rate, next-funding timestamp, borrow rate, fee estimate,
balance-sheet charge, margin cost, or leg-risk term enters its target.  Its
activity report consequently cannot establish the required causal chain:

```text
premium -> observed expected funding -> net carry estimate
        -> desired inventory -> actual non-atomic leg orders
```

The existing dated `derivsim.CashCarryArb` is deliberately excluded from P0.
It uses a fixed edge, optionally scaled by `sqrt(time_to_expiry / tenor)`.
That is a direct time-dependent admission rule, not evidence that settlement
economics produces convergence.  Dated carry receives a separate V2-5 slice
after the perpetual mechanism passes this integrity gate.

## Local hypothesis

A carry participant that receives a delayed public `MDFunding` snapshot should
change *its own target carry* only when the directionally signed expected
funding revenue over a declared holding horizon exceeds the declared costs of
executing and carrying the two legs.  It must not read an exchange's live
funding object or use funding to set either book price.

For a positive perpetual premium with positive funding, the candidate position
is long spot / short perp.  The short perp's expected funding receipt is
positive.  For a negative premium with negative funding, the mirror position
is short spot / long perp.  The sign convention must be proven with direct
fixtures, not inferred from aggregate PnL.

## P0 implementation contract

The new policy is opt-in and is not a modification of the legacy threshold
desk.  Its config must explicitly declare:

| Input | Required policy meaning |
| --- | --- |
| `FundingHorizon` | number of *next delivered funding intervals* priced into the decision; P0 uses one and does not extrapolate a stale rate indefinitely |
| `MaxFundingAge` | maximum age of the public funding snapshot at decision time |
| `TakerFeeBps` | two-leg entry plus eventual two-leg exit estimate, applied as a non-negative execution cost |
| `BorrowAnnualBps` | declared spot financing estimate prorated over the horizon; this is an estimate, not a hidden free loan |
| `BalanceSheetBps`, `MarginRiskBps`, `LegRiskBps` | separately named non-negative costs; they are participant economics, never a price correction |
| `MinNetCarryBps` | minimum net expected carry after every declared component |

The first implementation may use a simple expected-cost model, but it must
retain every component separately in evidence.  It may not set a fair price,
modify a mark, alter funding, force the opposite leg, or replace a failed leg.
Actual spot and perp legs remain independent IOC actions.  Partial fills and
orphan inventory remain possible and must be reported.

## Evidence contract

P0 adds compact append-only decision records for the funding-aware policy only.
Each record must include:

- actor and venue identity, decision time, policy version, and action/defer
  reason;
- exact local funding snapshot identity, rate, publication time, delivery time,
  next-funding time, and age;
- local executable bid/ask and a reference only when both required sides were
  delivered;
- signed premium, funding-income estimate, each named cost estimate, net carry,
  desired target, current spot/perp position, and requested leg;
- request IDs and later fill/reject/cancel linkage, without treating intent as
  a completed carry position.

The record must join to V2-0 receipts/frontier evidence.  An independent
auditor must be able to verify that the funding observation was delivered no
later than the decision and that a missing/stale/non-positive-domain reference
is reported as a named defer, never as a zero rate or price.

## Cheap falsification fixtures

These fixtures are required before a market smoke:

1. **Positive premium / positive delivered funding:** under sufficient
   net-carry edge, desired position is long spot / short perp and exactly one
   first-leg request is emitted.
2. **Identical books / zero funding:** a policy whose declared costs make net
   carry non-positive submits no entry.  The decision is an observable
   `NET_CARRY_BELOW_MINIMUM` defer, not an absent price.
3. **Sign mirror:** negative premium / negative funding selects short spot /
   long perp only if its signed funding income exceeds the same cost model.
4. **Delayed, dropped, duplicated, and reordered funding snapshots:** no
   decision may use an undelivered/future snapshot; a stale/missing snapshot
   defers with its reason; duplicate/reordered snapshots cannot advance the
   local funding frontier.
5. **Funding-sign mutation:** reverse the expected receipt sign in the
   independent auditor.  It must reject the positive/negative mirror fixtures.
6. **Telemetry-neutrality:** evidence OFF/ON and fresh `GOMAXPROCS=1/4` runs
   retain the same ordered execution hash.  The recorder schedules no event,
   consumes no RNG, and exposes no actor-visible state.

## Activation and decision rules

P0 passes only if all of the following are true:

1. a delayed public `MDFunding` event reaches the policy and is independently
   receipt-audited;
2. every submitted funding-aware entry has a fresh delivered funding record;
3. the signed funding-income and net-carry fields recompute from the recorded
   inputs and have the declared direction;
4. changing only the delivered rate across the sign/zero fixtures changes the
   policy action in the preregistered direction;
5. rejected/partial first legs preserve explicit unmatched inventory rather
   than being silently marked as a complete carry.

**Kill criterion:** if a changed funding rate does not change the recorded
net-carry calculation, desired inventory, or submitted order in the controlled
fixture, funding is **NOT IDENTIFIED**.  Do not run a basis-on/off market
campaign or infer an anchoring verdict.  If a rate is injected through hidden
global state or prices are changed to produce the decision, the implementation
fails the information/economic contract.

## Explicit non-claims

Passing P0 does not show that funding anchors the perpetual, that the policy is
profitable, that a basis mean-reverts, that liquidity is realistic, that a
dated future converges, or that a market-level relationship is emergent.  The
next gate, only after P0 passes, is a preregistered P1 control/treatment market
smoke with public funding enabled/disabled while the funding-aware policy and
independent-demand population remain fixed.
