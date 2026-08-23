# Mutation suite

Section 15 asks for deliberately broken variants of the simulator, each paired
with the invariant that must fail. A mutation that passes means the audit is
too weak and has to be strengthened — that is the point of the exercise, and it
is why each row below names a specific detector rather than "the audit".

Two mutations have been run. The rest are specified here with their expected
detector so that executing them is mechanical.

## Method

Each mutation is a one-line edit to the engine, built to a scratch binary,
never committed:

    bash scratch/mutate.sh <name> <file> scratch/muts/<name>.py <duration>

which copies the file aside, applies the edit, builds a mutant binary into
`scratch/`, **reverts the source before running anything**, and then runs the
mutant for the stated horizon into `logs/mut_<name>`. The duration is an
argument rather than a constant because each mutation has to declare the
shortest horizon that reaches the code it breaks: a two-hour run never settles
an option, so an exercise mutation measured over one is NOT TESTED and not a
pass.

For semantics rather than integration, the detector is a fixture rather than a
run:

    bash scratch/mutate_test.sh <name> <file> scratch/muts/<name>.py <TestSelector>

Revert immediately after building. A mutant binary must never be produced from
a tree that stays mutated, and no mutation may be committed.

## Coverage ledger

A mutation that no test ever executed is **NOT TESTED**, not a pass. The
trigger count below is the number of times the mutated code actually ran under
its detector: fixture subtests for a semantic detector, settlement instants for
an ecology run.

| mutation | trigger count | intended invariant | control | mutant | caught for the intended reason |
|---|---|---|---|---|---|
| Credit 1000 extra units on ~0.1% of spot settlements | whole run | closed-system identity, per asset | residuals exactly 0 | ABC 41,726,000; CDF 24,702,000; USD 31,624,727 | yes |
| Move venue revenue without recording it | whole run | venue take reconstructs from its own movement stream | take reconstructs | 562,254 ABC and 17,232,038 USD unaccounted | yes, after the ledger fix |
| Reverse the funding sign | 17 account-instants | each account charged the way the published rate says | 0 of 17 misdirected | **17 of 17 misdirected**, `sign_consistent=false` | yes -- and only after the detector was rebuilt, see below |
| Swap the call and put payoff | 9 fixtures / 18 holders | payout equals intrinsic value at the settlement price | all exact | 8 assertions fail: ITM call pays 0, OTM call pays 500,000,000 | yes |
| Settle against a strike 1% away | 9 fixtures / 18 holders | payout is computed from the contract's own strike | all exact | 8 assertions fail: ITM call pays 450,000,000 for 500,000,000; ATM put pays 55,000,000 while worthless | yes |
| Ignore the contract multiplier | 9 fixtures / 18 holders | payout scales by size divided by the multiplier | all exact | 7 assertions fail, payouts out by 10^8 | yes |
| Serve each price level from the tail (LIFO) | 5 priority cases | at one price the earlier arrival fills first | expected sequence | 3 of 5 fail, and the price-only case still passes | yes |
| Skip the best price level | 5 priority cases | the best price is taken first, and never through the taker limit | expected sequence | 3 of 5 fail, and the time-only case still passes | yes |
| Swap the call and put payoff, **ecology run** | 60 settlements over 5h | per-holder payout against the contract terms | 0 mispaid | **766 holders mispaid** | yes |
| Settle against a strike 1% away, **ecology run** | 60 settlements over 5h | the same | 0 mispaid | **386 holders mispaid** | yes |
| Settle funding twice, **ecology run** | 5 funding instants / 85 duplicate account postings | one funding posting per funded account, contract, and instant | 0 duplicates | **85 duplicates; 5 broken instants** | yes, after V-022 rebuilt the detector |
| Drop immediate-order cancellation **records**, ecology run | 169,935 immediate cancellation records | every accepted non-resting order has a persisted fill-or-cancel terminal record | 0 missing terminals | **169,935 missing terminals** | yes, through `-metric orderlifecycle` |
| Credit spot fee revenue twice, ecology run | 6,048,990 balance deltas | closed-system identity includes every venue fee credit | bounded truncation residual only | **CDF 126,548,770,107; USD 235,583,218,129 residual** | yes, through `-metric conservation` |

### Horizon, and why 5h rather than 2h

The two exercise mutations were first run for two hours and settled nothing at
all, because the short option tenor is exactly two hours. Re-run for five they
settle sixty contracts each, and the per-holder check finds them. The mutation
harness takes the horizon as an argument for this reason: each mutation has to
declare the shortest run that reaches the code it breaks, and a mutation
measured over a horizon that never executes it is NOT TESTED.

Note what stays clean in both ecology runs: `exercise_broken` is 0. That is the
summed residual per contract, and both mutations are symmetric between the long
and the short, so the sum is right while every individual payout is wrong. The
same result the fixtures gave, reproduced end to end.

Artifacts for these runs are kept in `research/artifacts/mutations/` with
checksums; the raw event logs, some 27GB of them, were deleted once the
detector outputs were extracted and recorded.

### What the funding mutation exposed about the audit

The first run of the reversed-sign mutant **passed**. The direction check
asserted only that a non-zero rate moved money between at least one payer and
one receiver -- true under any sign convention -- and reported `longs_paid` as
`paid < 0`, which is true whenever anybody paid at all. The check has been
rebuilt to reconstruct each account's perpetual side from the position stream
and compare each account's own movement against the published rate, counting
accounts whose side was never published as undirected rather than as passes.
The mutant now fails it 17 out of 17.

### What the exercise mutations exposed about conservation

All three exercise mutations **still conserve cash**: every one of them is
symmetric between the long and the short, so the settlement nets to zero and
the closed-system identity is satisfied while the payouts are wrong. A
conservation audit alone cannot detect a mispriced settlement. Only the
per-holder comparison against the contract's own terms can, which is why the
fixtures check each holder and each writer individually rather than the sum.

### Why the exercise detectors are fixtures rather than runs

A two-hour ecology run at the frozen configuration settles **zero** options --
the short tenor is exactly two hours -- so the exercise audit over it reported
`exercises=0` and `exercise_broken=0` for the control and for the mutant
alike. That is NOT TESTED. The fixtures force the settlement path with known
positions, a pinned settlement price and a closed set of holders, so every
branch of the payoff is definitely executed. The longer ecology runs are still
worth doing, but they test lifecycle wiring, not payoff semantics.

The fixture matrix covers: ITM call, OTM call, ITM put, OTM put, both ATM
boundaries, a non-unit multiplier with fractional and multi-contract holdings,
a strike far from the settlement price, and five holders of asymmetric size on
both sides. Each is checked for the holder's own payout, the writer's own
payout, oddness of the payoff in position size, zero payout when worthless,
conservation across the closed set, and the absence of any position, order or
listing after expiry.

## Specified, not yet run

| mutation | invariant that must fail | detector |
|---|---|---|
| Duplicate one fill | movements reconstruct the reported holdings; contract net size stays zero | `-metric conservation` chain check; `-metric positions` |
| Delete one fill | the same two | as above |
| Omit one settlement | payout residual per contract; holders paid against holders present | `-metric settlements` |
| Wrong sign on the Black-76 delta | dealer net delta grows without bound instead of being hedged back | `-metric hedging`, buy share and net delta drift |
| Execute an order after expiry | no fill may be recorded after the expiry instant | `-metric settlements`, `TradesAfterExpiry` |
| Fail to cancel expired resting orders | the same, plus the book still quoting a delisted contract | `-metric settlements`; needs a delisting check that does not exist yet |
| Drop a GTC cancellation request or state transition | resting order can remain executable after a requested cancel; request evidence is not persisted | none yet |
| Give one venue zero latency accidentally | cross-venue edge appears where none did | `-metric arbitrage`, cross-venue cycle |
| Use stale collateral for liquidation | cannot be tested: liquidation never fires (V-005) | blocked on the stress arm |
| Inject future information into one actor | no detector exists; the delivery path makes look-ahead structurally impossible but nothing instruments it (see `research/information-boundary-audit.md`) | none yet |

## What the table already says about the audit

Matching priority now has a detector, stated as an observable fill sequence
rather than an accounting identity -- no accounting invariant is violated by
filling the wrong resting order, since the money still moves and still
balances. It discriminates: a LIFO queue fails the time cases and passes the
price-only one, and skipping the best level fails the price cases and passes
the time-only one. It is a matcher-level detector, though; a run-level
queue-order check over the logged book deltas still does not exist, so a
priority defect introduced in the wiring around the matcher rather than in the
matcher would still go unseen.

Injected look-ahead still has **no detector at all**. The immediate-order
cancellation *record* is now covered, but an unlogged GTC cancellation request
or state transition remains untestable from the retained evidence. Two more
classes are blocked behind mechanisms that never execute. So the audit as it
stands covers
money and lifecycle thoroughly, covers derivative semantics well now that the
funding direction check has been rebuilt and the exercise fixtures exist, and
covers matching, order handling and information flow barely.

Two of the eight executed mutations were initially **missed** -- the unrecorded
venue movement and the reversed funding sign -- and in both cases the fix was
to strengthen the detector rather than to accept the pass. That ratio is the
most useful number in this file: a quarter of the mutations run so far found a
hole in the audit rather than confirming it.

That is a statement about the audit rather than about the simulator, and it
belongs in the same document as the passes: an invariant suite is only as
strong as the mutations it can catch, and this one has not yet been shown to
catch the ones that matter most to a matching engine.
