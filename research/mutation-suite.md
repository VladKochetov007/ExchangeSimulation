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
| Negate Black-76 delta in live option-dealer hedge decisions, ecology run | 903 exchange-owned risk snapshots / all three hedge policies | actual underlying hedge offsets independently marked option delta | mean \|net delta\| 0.0170; max 0.1650; drift 0.00038/h | **mean \|net delta\| 1.9264; max 10.8592; drift 1.1844/h** | yes, through exchange-owned `-metric exposure` |
| Settle first ABC-PERP match twice but emit one fill, ecology run | 1 match / 2 participant position paths | each logged linear fill has exactly one matching position transition | 248,898 linear fills and updates; 0 extras | **248,898 fills, 248,900 updates; 2 extras** | yes, through `-metric fillpositions` |
| Omit settlement side effects for first ABC-PERP match but emit its fills, ecology run | 1 match / 2 participant position paths | each logged linear fill has exactly one matching position transition | 248,898 linear fills and updates; 0 missing | **248,898 fills, 248,896 updates; 2 missing** | yes, through `-metric fillpositions` |
| Inject negative-latency market data through deterministic courier | 1 actor-bound message | source timestamp plus configured delay is the earliest actor-inbox delivery | no pre-due delivery; delivered at 1.010 s | **delivered at 1.000 s** | yes, through direct courier-boundary test |
| Delay contractual expiry/delisting five minutes, ecology run | 66 expired contracts (6 futures, 60 options) | no persisted fill after listed contract ExpiryNano | 206,360 expired-contract fill records; 0 late | **212,584 records; 7,326 late across all 66 contracts** | yes, through `-metric expiryfills` |
| Use previous stored perpetual mark for liquidation sweep, ecology run | 39 observed ABC-PERP checks / 35 forced closes | reported liquidation trigger uses the contemporaneous mark and its derived PnL, equity, notional, and maintenance | all 39 fields exact | **14 stale-mark field mismatches** (mark, PnL, equity, notional, maintenance) | yes, through independent `-metric marginchecks` |
| Drop every persisted ABC-PERP fill after real settlement, ecology run | 111,398 suppressed participant fill records | every persisted linear position transition has an observed economic fill; immediate orders retain a terminal evidence record | 248,898 fill/position paths; no lifecycle errors | **111,398 unmatched position paths; 47,268 missing immediate terminals; 28,309 quantity mismatches** | yes, through `-metric fillpositions` and `-metric orderlifecycle` |
| Make north spot-maker courier links instantaneous while the manifest remains nonzero, ecology run | 1,040,345 delivered north spot-maker messages | persisted courier telemetry reports the actual nonzero delay promised by the manifest | 0.566 ms mean drawn delay on all three channels | **0 ns drawn and delivered delay on all three channels** | yes, through persisted `latency.json`; price-edge proxy rejected |

### Delta-sign ecology mutation

The delta-sign mutation is intentionally stronger than a unit test of the
analytic formula. It negates `Black76Delta` only where the option dealer
chooses its live hedge; the exchange's periodic risk snapshot instead obtains
the dealer's filled option positions and actual ABC balance, then marks those
positions with `Black76Sensitivities`. Thus the outcome is not a comparison of
one actor cache against itself. Over the five-hour seed-101 control and mutant
worlds, all three configured policies (`banded`, `static`, and `timed`) were
active and 903 exchange-owned snapshots were retained.

The correct control had pooled mean absolute net delta 0.0170, maximum 0.1650,
and drift 0.000378 contracts/hour. The sign mutant had 1.9264, 10.8592, and
1.1844 respectively: 113.1x, 65.8x, and more than three orders of magnitude
larger. The detector therefore catches the intended error. In contrast, the
absolute hedge ratio remained about one in both worlds (1.0011 control versus
1.0007 mutant). A check that only counted hedge activity, volume, or an
absolute hedge ratio would have falsely passed exactly the sign failure it was
supposed to detect. Full provenance and evidence digest are in
`research/artifacts/mutations/delta-sign.json`; raw logs remain at
`logs/mut_delta_sign`.

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

## Remaining NOT TESTED / evidence-limited classes

| mutation | invariant that must fail | detector |
|---|---|---|
| Fail to cancel expired resting orders | the same, plus the book still quoting a delisted contract | `-metric settlements`; needs a delisting check that does not exist yet |
| Drop a GTC cancellation request or state transition | resting order can remain executable after a requested cancel; request evidence is not persisted | none yet |
| Use stale collateral for liquidation beyond one-perpetual/no-debt scope | full cross-margin trigger must use contemporaneous collateral and all risk marks | `marginchecks` covers only ABC-PERP, USD cash, and no-debt accounts; options, FX collateral, isolated margin, and borrowing remain untestable from retained evidence |

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

The scheduler-backed delivery path now rejects an explicit negative-latency
mutation: one message published at 1.000 s with 10 ms latency is absent until
1.010 s, while the scratch source mutation is caught at publication. This is
not a per-observation trace of the historical logs, and it does not cover
IB-1's direct dealer-inventory read. The immediate-order cancellation *record*
is now covered, but an unlogged GTC cancellation request or state transition
remains untestable from the retained evidence. Two more classes are blocked
behind mechanisms that never execute. So the audit as it stands covers
money and lifecycle thoroughly, covers derivative semantics well now that the
funding direction check has been rebuilt and the exercise fixtures exist, and
covers matching, order handling and information flow barely.

Two early mutation runs were initially **missed** -- the unrecorded venue
movement and the reversed funding sign -- and in both cases the fix was to
strengthen the detector rather than to accept the pass. Those misses remain
evidence about the audit, not historical footnotes to erase.

That is a statement about the audit rather than about the simulator, and it
belongs in the same document as the passes: an invariant suite is only as
strong as the mutations it can catch, and this one has not yet been shown to
catch the ones that matter most to a matching engine.

### Future-information courier mutation

`future_information_delivery` mutates the deterministic courier's scheduled
arrival from `source + delay` to `source - delay`. The direct test injects one
market-data message into the real gateway and asserts that the actor inbox is
empty before the due timestamp. The control passes, while the mutant fails at
the publication instant. This is a CAUGHT fixture-level semantic mutation,
not a claim that the historic raw event logs independently recorded every
participant observation. Full compact record:
`research/artifacts/mutations/future-information-delivery.json`.

### Contractual-expiry mutation

`delay_expiry_settlement` holds each expired book open for five simulated
minutes while leaving its published `ExpiryNano` unchanged. The control has 99
listed contracts, 66 contractually expired, and zero late fill records. The
mutant has 7,326 after-expiry `OrderFill` records across all 66 exercised
contracts: six dated futures and sixty options. The former settlement audit
only reports the 1,918 dated-future fill records, which exposed its missing
option scope. The new listing-anchored `expiryfills` audit reports the entire
failure and also unit-tests the complementary no-settlement case as
`expired_unsettled`. Order lifecycle and conservation remain clean, so they
cannot substitute for a contractual-lifetime audit. Full artifact:
`research/artifacts/mutations/delay-expiry-settlement.json`.

The same non-circular audit was replayed over all three preserved 24-hour
controls. Each has 396 listed contracts, 363 expired-and-settled contracts,
and zero late fills, expired-unsettled contracts, or listing/settlement
metadata defects. This is control evidence for the contractual-boundary claim,
not merely a passing fixture.

### Linear fill-to-position mutation

The `double_perp_settlement_once` mutant applies the settlement side effects
of exactly one north `ABC-PERP` match twice, but deliberately creates only the
original trade and `OrderFill` records. The normal five-hour control has
248,898 linear (perpetual/dated-future) fills and exactly 248,898 matching
trade position updates. The mutant has the same 248,898 fills but 248,900
updates, so the new `-metric fillpositions` catches two unmatched updates.

This is a necessary detector rather than a redundant presentation of an old
one: the mutant's balance-delta chains, closed-system conservation, terminal
linear contract net size, terminal report comparison, and order lifecycle all
remain clean. The duplicate economic settlement was visible only in the
one-to-one fill/position relation. Full artifact:
`research/artifacts/mutations/double-perp-settlement-once.json`.

The audit is deliberately limited to linear instruments. The frozen logger
does not persist an option `position_update` for each option fill, so option
fill-to-position equality is **NOT TESTED**, not a pass. This is now recorded
as V-023 in `validation-audit.md`; it constrains the claims the ae13f9a
autopsy may make about option-path evidence.

The complementary `drop_perp_settlement_once` mutant preserves a single
reported north `ABC-PERP` fill pair while suppressing both settlement paths.
It produces two missing, rather than two extra, position updates. Conservation,
terminal positions and order lifecycle again remain clean; `fillpositions`
catches the two missing transitions. The detector therefore rejects both a
duplicated and an omitted economic settlement. Full artifact:
`research/artifacts/mutations/drop-perp-settlement-once.json`.

### Accidental zero-latency mutation

`zero_north_spot_maker_latency` leaves the configuration manifest unchanged
but replaces the north `spot_maker` per-client request, response, and
market-data providers with zero-delay providers. It deliberately retains the
scheduled delayed gateway rather than connecting it directly, so the compact
delivery sidecar has to expose the zero instead of merely losing a row.

The paired five-hour seed-101 control reports 0.566 ms mean drawn delay on
each north spot-maker channel. The mutant reports exactly zero drawn and
delivered nanoseconds for all 1,040,345 scheduled/delivered north messages,
while south's corresponding channels remain about 0.567 ms. The unit test
`TestScheduledZeroLatencyProducesZeroTelemetry` independently verifies the
same courier/sidecar contract. This is therefore **CAUGHT**.

The originally suggested cross-venue-arbitrage proxy did not behave in its
assumed direction: the control has a 6.20% profitable north-to-central share
(maximum 1.53 bps), while the mutant has only a 0.25% central-to-north share
(maximum 0.12 bps). That price result is retained as diagnostic evidence, not
used as a latency detector. Full compact provenance is
`research/artifacts/mutations/zero-north-spot-maker-latency.json`; raw logs
remain retained.
