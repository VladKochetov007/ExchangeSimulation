# V2-5 carry-horizon lifecycle audit

Status: **design constraint found before a new carry experiment.** This is a
static call-path audit of the current `FundingCarryArbitrageur`, not a new
market result.

## Finding — C-001

`FundingHorizon` is currently an estimate-only input. In
`fundingCarryComputeFinancials`, it multiplies expected funding income and
extends the priced borrowing interval. The actor itself stores neither entry
time nor an `exit_at` time. After the two legs reach their desired offset it
emits `AT_TARGET`; a later `ZERO_PREMIUM`, unavailable, or stale-funding
decision defers without an explicit close. `TerminalNano` only censors a new
entry that cannot complete two request periods before the simulation end; it
is not an ownership-maturity or unwind mechanism.

Thus the existing policy can price four entry/eventual-exit fees without
having a lifecycle contract that actually executes that eventual exit. It also
cannot distinguish a planned term carry holding from an indefinitely retained
offsetting inventory after the original funding observation becomes obsolete.

## Provenance and scope

The audited call path is the current source through `8c29024`:

```text
onTick → decision → fundingCarryComputeFinancials(FundingHorizon)
       → submitSpotAdjustment / orphanRepair → AT_TARGET or defer
```

There is no state or branch representing `entry_at`, `holding_until`,
`unwind_pending`, or a scheduled close. The independent `fundingcarry`
analyzer correctly replays the *declared current policy*, but it also cannot
validate a term lifecycle the actor never records.

P0's single five-minute activation result remains valid for its stated claim:
local source use, signed calculation, and non-atomic entry/orphan evidence.
P1a remains `NOT EXERCISED`, with zero submitted legs. Neither result claimed
that the active policy realizes a term carry return, so no scored funding or
basis conclusion is invalidated.

## Consequence

Do not make P1 active merely by increasing `FundingHorizon` in its existing
configuration. That would couple a longer expected-income number to an actor
without a matching ownership/unwind contract. P2 signal readiness likewise
does not cure this deficiency.

The next permissible V2-5 implementation is a separately named, lifecycle-
bearing carry participant. Before a market-backed activation cell it must
persist and independently replay: entry source/time, declared term, target
positions, non-atomic entry and unwind legs, every actual funding settlement,
early-risk/terminal policy, and exact close condition. It must have a named
economic motive for its term and retain fees, borrow, balance-sheet use,
margin/liquidation risk, and orphan inventory as explicit costs/states.
