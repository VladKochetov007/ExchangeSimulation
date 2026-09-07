# Red-team findings — economic / correctness audit

Append-only. Each finding records its own base revision; later findings may be
measured against a later one.

**Scientific base revision for RT-001 … RT-003:**
`a666d02faede3d40f046b11e60eb672c59386a94`, head of
`feature/r2-cdf-survival-successor`.

**Method.** Invariants are tested against a real run, not a unit fixture,
because reachability is part of the claim. Primary probe: dev-607 / seed 607 /
**7 simulated hours**, chosen because a 20-minute run reaches no funding
interval, no expiry, no settlement and no exercise — its only balance-change
reasons are `trade_settlement`, `initial_deposit`, `borrow` and
`interest_charge`. Seven hours crosses three futures expiries and three option
expiries per venue. No holdout was read or run. No economics were changed.

---

## RT-001 — Expiry settlement bypasses the conservation tracker

**Severity:** high (evidence integrity / detection capability). Not a loss of
funds.

**Reachability:** ordinary cells. Fires in every venue of a standard dev-607
run, from the first futures expiry onward, permanently.

**Invariant.** Every balance mutation is recorded in the conservation tracker
before the logger is consulted. This is stated on `logBalanceChange`
(`exchange/helpers.go:22`): *"Recorded before the log is consulted: a movement
that happens while no logger is attached is still a movement, and leaving it out
of the running total would make the verification depend on the logging
configuration."* `VerifyConservation` then compares recorded movements against
held balances, so that *"a balance changed without a logged movement leaves the
recorded total behind the held total."*

**Observed violation.** `settleExpiredInstrument` (`exchange/expiry.go:640`)
mutates `client.PerpBalances[quote]` and then emits the `balance_change` event
by calling `log.LogEvent` **directly**, constructing the event by hand rather
than calling `logBalanceChange`. The movement therefore reaches the log file but
never reaches `conservation.record`. This is the only such bypass in production
code; every other mutation site uses the helper.

**Reproduction.** dev-607 / seed 607 / 7h. `conservation_violation` events:

| venue | events | first at | gap after 1st / 2nd / 3rd futures expiry |
| --- | ---: | --- | ---: |
| north | 17,999 | t=+7202 s | -2,129,658,925 / -5,078,053,185 / -10,736,084,864 |
| central | 17,999 | t=+7202 s | -5,962,824,577 / -10,600,880,874 / -25,287,737,787 |
| south | 17,999 | t=+7202 s | -4,396,311,208 / -9,820,755,564 / -23,948,562,684 |

The gap is exactly zero before the first settlement, and each step appears one
second after an `instrument_settled` event. The step equals the settled cash
exactly: north's first futures settlement has Σ cashflow `-2,129,658,925` and
north's first gap is `-2,129,658,925`. Same for central and south.

**Economic interpretation: no funds are lost.** This was checked before
proposing a fix, because recording a movement that was genuinely destroying
money would have silenced a true positive. The project's own auditor closes the
accounting identity on the same run:

```
identity USD  external 16072200000000000  internal -345452610503
              exchange 357011627991  open -11559017469  residual 19 (1.18e-15)
expiry: 9 instants, largest net -14686856908 (not required to be zero)
```

Futures expiry cash is not required to net to zero — surviving positions carry
unpaired bases because partial closes already realised their PnL in cash — and
the residual is 19 units in 1.6e16. The defect is that the tracker is not told
about a legitimate, logged movement.

**Consequence.** Worse than a spurious report. The tracker exists so that an
unrecorded mutation is impossible to hide; once the baseline is wrong from the
first expiry, a later genuinely unrecorded mutation is indistinguishable from
the gap settlement already opened. The detector is disabled for the rest of the
run, and it emits 53,997 false violations per 7-hour run.

**Fix.** Record the movement unconditionally, immediately before the logger is
consulted, keeping the logged bytes unchanged. The site cannot simply call
`logBalanceChange`: the settlement record carries `PositionSide`, which the
helper's event does not, and the logged bytes are evidence.

**Semantic impact: none.** Re-running the same cell patched, all 12 non-`general`
log files are byte-identical, and each `general.jsonl` is identical once the
`conservation_violation` lines are removed. `conservation_violation` count goes
17,999 → **0** in every venue. The execution hash changes only because those
spurious events leave the stream.

**Regression test.** `exchange/expiry_conservation_test.go`,
`TestExpirySettlementIsRecordedForConservation`. Fails on `a666d02` with
`Gap: 4000000`; passes patched. The two sides are given different entry prices
deliberately: the tracker compares per-asset totals, so a settlement whose cash
nets to zero across the book moves neither total and hides the omission
completely. That is also why the bug is invisible in a balanced unit fixture and
only surfaced on a real run.

**Recommendation: adopt.** The intended contract is unambiguous and stated in
the code; the fix restores a detector rather than changing economics.

---

## RT-002 — Option expiry net cash is not exactly zero

**Severity:** low. Documented, not fixed.

**Classification:** EDGE CASE / rounding.

**Invariant.** The project's own auditor states it: *"option expiry … must be
zero: payoff does not depend on entry price."* An option's expiry cash is
`MulDiv(size, intrinsicValue(settlementPrice), basePrecision)`, which has no
entry-price term, so the long and short legs of a netted book must cancel.

**Observed violation.** On the 7h run the auditor reports *"option expiry: 9
instants, worst net 6"*. Per-contract sums are in the range ±3 units of 1e-8
USD. Positions net to zero on every contract (`sum(size) = 0` on all 99
settlements audited).

**Mechanism.** The payoff is truncated per position, not per book.
`MulDiv(3, v, p)` is not `3 × MulDiv(1, v, p)`, so one long facing three unit
shorts leaves a residual of a few units. Equal-and-opposite pairs cancel exactly;
unequal aggregations do not.

**Economic interpretation.** A few units of 1e-8 USD per expiry instant. Real,
bounded, and of the same nature as the rounding carry the futures path already
drains deliberately through `CommitPositionAccountingCarry`.

**Fix: none proposed.** Changing it changes payouts, and the choice between
per-position truncation and a book-level carry is a modelling decision. The
futures path resolves the same problem by making the venue the residual
counterparty; whether options should do likewise is the scientific owner's call.

**Recommendation:** owner decides. Flagged because the auditor's own text says
this quantity must be zero and it is not.

---

## RT-003 — Invariants tested and found intact

Recorded so the audit's negative space is explicit.

- **Position netting.** For every derivative contract on every venue, the signed
  positions of all clients sum to exactly zero — 9 perp/futures contracts and 90
  option contracts, 0 with a non-zero net. No phantom counterparty.
- **System accounting identity.** Closes to 19 units in 1.6e16 on USD, and
  exactly on ABC and CDF.
- **Venue take reconstruction.** Fee revenue reconstructs exactly from its
  movement stream for all three assets.
- **Borrow/repay symmetry.** `borrow` nets exactly zero on both ABC and CDF.
- **Spot trade settlement.** Nets exactly zero on ABC.
- **Funding.** Residual ≤ 4 units per instant, with the remainder explicitly
  routed to `funding_remainder` on the venue ledger.

---

## Unaudited

Named so the freeze knows what this report does not cover.

- Liquidation paths: no liquidation occurred in the probe run, so debt
  disappearance, collateral duplication and residual-value routing are untested
  here.
- Cross-venue transfer and latency composition.
- Margin call and insurance-fund draw sequencing.
- Option exercise/assignment against a live underlying position (delta hedge
  accounting).
- Relisting after settlement under the same symbol.
- Negative and zero price domains at settlement.
- The holdout cells, by instruction.

---

## RT-004 — Conservation tracker detects unrecorded mutations only

**Severity:** medium (audit coverage, not an economic defect).
**Classification:** CORRECT BUT SURPRISING. Layer: evidence / detector.
**Base revision:** `a666d02`.

**What was tested.** Seven controlled faults injected into a clean fixture, with
the outcome predicted before running. See E-009 in the research note.

| injected fault | detected |
| --- | --- |
| valid control | no report (correct) |
| unrecorded credit (+7) | yes |
| unrecorded debit (−3) | yes |
| debt silently cancelled | yes |
| one smallest currency unit | yes |
| value paid to the wrong participant | **no** |
| value destroyed but faithfully recorded | **no** |

**Interpretation.** `VerifyConservation` compares per-asset totals of recorded
movements against per-asset totals of holdings. It therefore detects unrecorded
mutations and only those. Nothing in it requires a debit to have a matching
credit, and nothing in it identifies a recipient — so a payment to the wrong
customer preserves every total it checks, and destruction that is faithfully
logged moves both totals together.

**Why this matters for reading other results.** RT-001 was invisible in a
balanced two-party fixture for exactly this reason and only surfaced on a run
where settlement cash did not net to zero. A green tracker is not evidence that
payments reached the right parties.

**Disposition: no code change.** The two blind spots are covered by the identity
check in `research/accounting-audit.md`
(`InternalNet + ExchangeTake + OpenLinearValue = 0`, via
`mvanalyze -metric conservation`). The checks are complementary and neither
subsumes the other. The gap is recorded rather than closed, and the surviving
faults are asserted as surviving in the tests so that a future change to
sensitivity shows up as a failure.

**Regression tests.** `tests/economic_audit_detector_sensitivity_test.go`,
`exchange/economic_audit_recorded_destruction_test.go`.

---

## RT-005 — A bankrupt account's spot wallet is not seized

**Severity:** open pending owner decision. **Classification: NOT ENOUGH
EVIDENCE** to call it either a defect or an intended assumption.
Layer: specification. **Base revision:** `a666d02`.

**Observation.** `liquidate` (`exchange/exchange.go:2242`) resolves a bankrupt
account by zeroing negative *perp* cash and debiting `VenueInsuranceFund` the
same amount. The repay path above it touches only `PerpBalances` and `Borrowed`.
`client.Balances` — the spot wallet — is never consulted. In E-008 the
defaulter keeps 500 USD of spot cash while the fund absorbs the full 100 USD
deficit.

**Economic consequence if unintended.** The insurance fund, and ultimately the
venue, bears a loss that an aggregate-solvent account could have covered. That
is a loss forced onto a party that should not bear it.

**Evidence it may be intended.** `Client.BorrowedSpot` is documented as
splitting a liability by wallet precisely so that "perp equity, liquidation
estimates, and snapshots must not charge a spot-credited loan to the perp
wallet." That reads as deliberate wallet segregation.

**Owner decision required.** If wallets are segregated by design this is an
INTENDED MODEL ASSUMPTION and should be stated as one. If the model claims
cross-margin netting across wallets, this is a real defect. The audit does not
resolve it and has not changed it.

**Reachability.** The transition is reachable in a fixture. It was **not**
exercised in the 7h integration run — no liquidation occurred there at all — so
production reachability on dev cells is unestablished.

---

## RT-006 — Latency is delivered as configured; no unearned speed advantage

**Severity:** none — bounded no-violation result, recorded because actor
fairness is a load-bearing assumption of every relative-performance conclusion
the campaign draws. **Base revision:** `a666d02`. Evidence: E-011.

**Invariant.** A participant class may only receive the information and
execution speed its configuration grants it. If a class were faster than
configured, its measured performance would reflect the harness rather than its
strategy.

**Result.** dev-607 / seed 607 / 20m, 225 link x channel rows across 27
participant classes and 3 remote maker feeds: every row's delivered latency
matches the model its config declares, including the spiky mixture
(0.99·1ms + 0.01·50ms = 1.49ms), two lognormal means, a normal, an explicit
`market_data_scale: 2` on two classes, the 1s `cross_venue_base_latency`, and
three remote feeds at exactly 10/20/30 ms. No link has a zero-latency channel.
Undelivered at shutdown is 0.056% of scheduled messages, spread across 140 of
225 rows — a drain boundary, not one actor being starved.

Statically, no actor holds an exchange, book or position reference; the only
direct handle is `Venue.Exchange`, which is the venue. The courier boundary
itself was already covered by
`simulation/information_boundary_test.go` and `simulation/delayed_gateway_test.go`,
which this audit did not duplicate.

**Scope.** One config, one seed, 20 simulated minutes, transport only. It does
**not** establish that an actor's decision logic consults only what its inbox
already held — see H-011, which is open and testable from evidence the runs
already emit.

**Method note.** Two earlier passes of this reconciliation reported 15 and then
9 mismatches; all were errors in the expectation model, not the system. For a
stochastic latency profile the configured `delay` is not the expected delivered
mean, and `market_data_scale`, `cross_venue_base_latency` and the remote-feed
table each have their own rule. Reporting either pass would have produced a
false fairness finding.

## RT-007 — The participant-information audit's review oracle accepted evidence the production auditor rejects

**Classification.** REAL BUG. Layer = evidence/detector, not economics.
Severity medium. **Not reachable in campaign output**: `AuditMarketDataReceipts`
calls only the streaming path, so no reported audit result is affected. The
exposure is to review, which is where a detector's own correctness is decided.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`.

**What the two functions are.** `analysis/receipts.go` holds two full
implementations of the V2 participant-information audit.
`auditMarketDataReceiptsStreaming` is production. `auditMarketDataReceiptsBuffered`
is retained, in its own words, as "a review oracle while the production path
moves to bounded streaming ... making semantic comparison against the prior
implementation straightforward in tests and review."

**What was wrong.** They did not agree on what valid evidence is. The buffered
path collected schedules, receipts and decisions into one slice and sorted it by
event ordinal:

```go
sort.Slice(events, func(i, j int) bool { return events[i].ordinal < events[j].ordinal })
```

Sorting repairs a file whose records are stored out of event order before any
check can observe it. The streaming path merges the three files as stored, so it
sees the disorder and raises `bad_global_event_order`. `BadEventOrder` feeds
`Valid`.

**Minimal reproduction.** Swap the two schedule *records* wholesale in the
fixture. Nothing else changes: the multiset of event ordinals is identical, and
every record stays internally consistent — only the storage order moves.

| | `Valid` | `BadEventOrder` |
|---|---|---|
| streaming (production) | `false` | 2 |
| buffered (oracle), base | **`true`** | 0 |
| buffered (oracle), fixed | `false` | 2 |

**Why it matters.** The oracle is strictly more permissive about record
ordering than the code it is meant to check. An engineer comparing a change to
the streaming path against this oracle would read a correct streaming rejection
as a streaming regression, and would have no way to see that the oracle had
silently repaired the input. A checker whose subject is order must not sort its
input.

**Fix (audit branch).** Replace the sort with the same three-way merge the
streaming auditor performs: at each step the stream whose head carries the
strictly smallest event ordinal wins, ties resolved in schedule, receipt,
decision order. The oracle then traverses exactly the order production does.

**Regression.** `analysis/economic_audit_receipt_oracle_test.go`,
`TestAuditMarketDataEvidenceOracleAgreesUnderFaults`: fifteen fault injections
driven through both implementations, each rewriting every file digest so a
checksum cannot stand in for a semantic catch. It asserts that the two never
disagree about `Valid`, that every other counter matches exactly, and that each
fault is caught by both — a fault neither notices is recorded as a coverage gap
rather than as agreement. Discriminating: with the fix reverted it fails on two
faults; with it applied all fifteen pass and the pre-existing `analysis` suite
is unchanged.

**One divergence remains, deliberately.** Reordering two schedules' per-link
ordinals is rejected by both but classified differently — the streaming spill is
bounded and retains a schedule only while its ordinal is in sequence, so a
receipt whose schedule was dropped reads as `receipt_without_schedule`, while
the oracle's full map reads it as `schedule_receipt_mismatch`. That is what
bounded memory costs, not a defect, and erasing it would turn the oracle into a
copy. It is pinned in the test with its exact counters so that a change turning
it into a disagreement about validity fails loudly.

**Prior art not duplicated.** The project already checks decision causality, and
more strongly than this audit assumed: each decision cites a frontier whose
16-byte digest the auditor recomputes as a hash chain over the receipts
delivered on that link, so an actor cannot cite an observation the receipt
stream does not contain or one ahead of its own decision. See the H-011 entry in
`research/economic-correctness-research-note.md` for the three limits that
remain.

## RT-008 — The taker fee depends on how the counterparty's liquidity was sliced

**Classification.** EDGE CASE by magnitude at campaign prices; the *mechanism*
is real, directional, and invisible to every existing check. Disposition open:
fee semantics are the owner's to decide, and aggregating a fee across a match
would be a change to scientific economics.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`.

**Mechanism.** `PercentageFee{InQuote: true}` charges
`trunc(trunc(qty*price/basePrecision) * bps / 10000)` **per execution**. Integer
truncation is subadditive, so for any partition of a quantity into fills,
`sum_i fee(q_i) <= fee(sum_i q_i)`. The same exposure taken as N executions can
therefore only ever cost less than the same exposure taken as one.

**Why it is a fairness question, not a rounding nit.** The taker does not choose
the partition — the resting side does. Two takers submitting the identical order
at the identical price pay different fees depending on whether the book in front
of them was one large order or many small clips.

**Why no detector sees it.** The fee that is not charged is never moved. The
conservation tracker compares recorded movements against holdings, and both are
consistent; the identity `InternalNet + ExchangeTake + OpenLinearValue = 0` also
balances. The shortfall exists only against a counterfactual, which is precisely
the class RT-004 established the tracker is blind to.

**Measured magnitude** (`tests/economic_audit_fee_partition_test.go`):

| arm | one execution | ten executions | shortfall |
|---|---:|---:|---:|
| 100 USD, clip 150 001 | 75 | 70 | 5 (6.67%) |
| 50 000 USD (ABC bootstrap), minimum clip | 25 000 | 25 000 | 0 |
| 50 000 USD, clip chosen to maximise truncation | 25 009 | 25 000 | 9 (0.036%) |

**The 6.67% row is a small-fixture artefact and must not be quoted as a campaign
number.** At the campaign's price level the charge reduces to `qty/40` quote
units, so a ten-way slice can lose at most `trunc(39/4) = 9` units — derived
first, then confirmed exactly. At a round minimum clip nothing is lost.

**Latent boundary.** A fill pays nothing when its trade value falls below
`10000/bps` quote units. At 5 bps and the ABC venue minimum of 0.001 base, that
is every price up to **19.00 USD**. ABC bootstraps at 50 000 and CDF at 3 000,
so the free-fill regime is **not reachable in the campaign**. It is recorded
because it follows from the fee model and the minimum order size together, and a
future low-priced instrument would cross it with no warning.

**Regression.** The test does not demand a policy. It pins the shape: that
slicing can never make the taker pay *more* (a reversal would mean the fee had
become superadditive, which no rounding rule produces by accident), and that the
shortfall stays within the derived per-execution bound.

## RT-009 — A hidden order keeps full time priority over displayed size

**Classification.** INTENDED MODEL ASSUMPTION, and **NOT EXERCISED**. No
campaign result depends on it.

**Mechanism.** `matching.makerAvailable` throttles an iceberg to its display
tranche and returns the full remainder for everything else, hidden orders
included. Both matchers use it. So at one price a hidden order carries exactly
the time priority a displayed order of the same size carries, while contributing
nothing to the public snapshot.

**Measured** (`tests/economic_audit_hidden_priority_test.go`): a hidden clip
resting first took the entire incoming fill (1 000 000 base units); the
displayed clip resting behind it sold nothing; the public ask level showed only
the displayed clip throughout.

**Why it is recorded.** Real venues subordinate hidden size to displayed size at
the same price precisely because otherwise displaying is irrational — a
participant who shows size gives information away and receives nothing for it.
Under this model `Hidden` weakly dominates `Normal`: identical fills, less
information leaked. Any actor using `Normal` would be handicapped with no
compensating benefit.

**Reachability.** Nothing in the tree constructs a non-Normal order.
`BaseActor.SubmitOrderFull`, the only route from an actor to a visibility other
than `Normal`, has no callers anywhere, tests included. Icebergs are modelled
correctly by contrast: `refreshIcebergTranche` unlinks and re-links the order, so
a refreshed tranche goes to the back of its price level and loses time priority.

**What this finding is for.** Not a request to change the matcher. It makes
enabling hidden orders a deliberate act with a known consequence instead of a
silent one, and the test fails loudly if the rule changes in either direction.

## RT-010 — Bounded no-violation results on the execution path

Recorded so the negative results are preserved and are not re-derived under
different wording. Base `a666d02faede3d40f046b11e60eb672c59386a94`.

**Reservation lifecycle** (`tests/economic_audit_reservation_lifecycle_test.go`).
`Available = Balances - Reserved`, so an earmark that outlives its order removes
buying power an actor is entitled to, and one released too eagerly grants buying
power its capital does not support. Both are silent: `ReleasePerp` clamps at
zero, and RT-004 established the conservation tracker cannot see either.

Every exit tested restores the earmark exactly: fill-or-kill that cannot fill
completely, post-only that would cross, an order larger than its balance, a
resting order half filled (the earmark is exactly half of what the whole order
held), and cancelling that half-filled remainder (back to the idle baseline —
not less, which would strand, and not more, which would free collateral the fill
had already converted into an asset). `GetAvailable` never exceeds the balance.

The reason is visible in the code: `releaseReserved` "releases what was locked,
not a recomputed approximation." The audit's prediction that the partial paths
would be the weak ones was wrong, and is recorded as wrong.

**Self-trade prevention** (`tests/economic_audit_self_cross_test.go`).
`RejectSelfTrade` is declared in the reject vocabulary but no code path produces
it, which raised the question of whether one participant could leave the public
book crossed against itself — best bid, best ask, mid and spread being the
inputs every other participant quotes against.

It cannot. `cancelOwnCrossingQuotes` (`exchange/order_handling.go:1718`)
implements cancel-maker: once the matcher has consumed every crossable order
from other clients, anything still crossing belongs to the incoming client and
is withdrawn. Four properties are now pinned —

1. the book is never left crossed and no wash trade prints;
2. the withdrawn quote's collateral is released, not stranded;
3. the owner receives a `ForcedCancelNotification` for each withdrawal — this is
   the failure mode the project has already been bitten by, where an order
   removed without telling its owner leaves the actor believing it still rests;
4. the notifications arrive in **placement** order, not price order and not map
   order. The implementation collects targets from a map and sorts by order ID
   for exactly this reason; without the sort, map iteration would reach the
   evidence stream and the execution hash would stop being reproducible.

**Method note.** The first run of property 3 reported zero cancellations
delivered, which would have been a false finding of the silent-forced-cancel bug
class in code that delivers correctly. `enqueueResponse` appends to an outbox
drained by a separate goroutine, so a non-blocking read races delivery instead of
observing it. When the observable is produced asynchronously, an instrument that
samples once measures the scheduler.

## RT-010 (continued) — Termination the order did not initiate

`tests/economic_audit_lifecycle_termination_test.go`. An immediate-or-cancel
remainder killed by the venue, and a resting order on a dated future that
reaches expiry — with a settlement price and without one, so the contract enters
settlement-pending instead of settling.

Expiry is the sharp case: the instrument disappears, so an earmark left behind
has nothing to point at and no later cancel can reach it. All three release the
earmark to the idle baseline, stop tracking the order, keep available at or
below balance, and deliver a `ForcedCancelNotification` for the exact order ID.
Three further `CheckExpiries` passes on the pending contract change nothing —
`cancelClientOrdersOnBook` looks each order up in the live book and
`client.RemoveOrder` has already removed it, so the retry finds nothing to
release. The audit predicted the retry path would be the weak link. It was not.

**Method note.** The first attempt backdated the expiry and placed the order
afterwards, so every placement was refused with `INSTRUMENT_EXPIRED` and the
test would have "passed" without ever exercising its premise. The fixture now
advances a controllable clock so the order is admitted while the contract is
live.

**Plateau.** Three consecutive falsifications on the execution path. Every
invariant tested to date has been single-account, single-instrument,
single-book, and that frame is now exhausted. The next work is cross-book value
flow through a shared account (H-019), where a per-book invariant can hold
everywhere and the system still leak.

## RT-011 — Margin is aggregated across books; liquidation is not

**Classification.** CORRECT BUT SURPRISING / specification question. Severity
medium. Reachable by any account holding positions in two books. **Owner
decision** — partial-close ordering and cross-book seizure rules are scientific
economics. No value is created and conservation is intact.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`.

**The asymmetry.** `buildAccountMarginProfile` (`exchange/exchange.go:1848`)
walks every book in sorted symbol order, adds each position's unrealized PnL to
equity, and fails the whole profile closed if any sibling exposure is
settlement-pending. Margin is genuinely cross-book. `CheckLiquidations`
(`exchange/exchange.go:2060`) is entered per symbol from that symbol's mark
update and, when the account breaches, closes only the positions **in that
symbol**.

**Minimal reproduction.** 900 USD of perp cash, long 1 `ABC-PERP` at 100, long
10 `ABC-FUT` at 100. The sibling's mark collapses to 5; the perp does not move.

    sibling uPnL = 10 * (5 - 100)   = -950 USD
    perp    uPnL =  1 * (100 - 100) =    0 USD
    equity       = 900 - 950 + 0    =  -50 USD

| step | liquidations | `ABC-PERP` | `ABC-FUT` | perp cash | insurance fund |
|---|---:|---:|---:|---:|---:|
| mark update on the healthy book | 1 | **0** | 10 | 900 | 0 |
| second check on the same book | 1 | 0 | 10 | 900 | 0 |
| mark update on the losing book | 2 | 0 | **0** | 0 | **-50** |

The perp position, sitting exactly at its entry price with no loss at all, is
the one confiscated — to answer a deficit caused entirely by the future. The
future is untouched. A second check on the perp finds nothing, because
`CheckLiquidations` returns early at `len(positions) == 0`; between the two
ticks the account carries 10 units of unmargined exposure and is invisible
through the door it was found by.

**Severity bound, established by attacking the result rather than reporting it.**
The hypothesis stopped at "the account remains below maintenance", which would
have implied a permanent hole. It is not permanent: a mark update on the losing
book reaches the exposure, closes it, and the fund absorbs exactly the
hand-derived 50 USD. Reporting the first half alone would have overstated the
finding.

**What remains.** Which of an actor's positions is confiscated depends on which
book happened to tick first, not on which exposure caused the loss. Two actors
with identical portfolios and identical losses can lose different positions
depending on the arrival order of marks on instruments neither controls, and an
actor hedging across two books risks having the hedge taken while the loss stays
open. That is an unearned difference in outcome between participants, produced
by the venue rather than by their strategies.

**The test does not prescribe a policy.** It pins the measured behaviour in both
directions: that the trigger symbol's position is closed and the sibling's is
not, and that the sibling is reached on its own next tick with the fund
absorbing exactly the derived deficit. Any change to either becomes visible.

## RT-012 — A settlement-pending sibling suspends liquidation of the whole account

**Classification.** Specification question with a measured cost. **Owner
decision** — the two readings below are both defensible and choosing between
them is scientific economics. Severity depends on reachability, which this
audit does **not** establish.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`.

**Mechanism.** `buildAccountMarginProfile` fails the whole profile closed when
any of the account's positions sits on a settlement-pending contract, for a
stated and sound reason: "Retained pending exposure is not an economic zero. No
valid mark exists, so fail the whole account profile closed instead of allowing
active sibling risk to ignore it." `CheckLiquidations` receives that error,
reports it through `reportPriceUnavailable`, and `continue`s to the next client.
**Failing closed on the measurement fails open on the action.**

**Minimal reproduction.** 100 USD of perp cash, long 10 `ABC-PERP` at 100, plus
one unit of an `ABC-FUT` that expires with no settlement price. The perp mark
halves to 50, putting equity at `100 + 10*(50-100) = -400 USD`. Result:
`liquidations=0`, the position stands at its full size, cash untouched. Once the
future receives a price and settles, the same call liquidates normally.

**The bound, which changes the finding.** The account is **frozen, not
privileged**. `order_handling.go:557` refuses every order from a client with
settlement-pending exposure — including one that would *reduce* the position —
with `ACCOUNT_SETTLEMENT_PENDING`. The preregistration called this "immune to
liquidation on every other book"; the accurate word is suspended, and the
correction is recorded rather than quietly dropped.

**What the suspension costs, isolated in a second fixture.** The position rides
the market while nobody can close it, so the deficit is set by the price
available when the freeze lifts, not by the price at the breach:

| position closes at | fund absorbs | hand-derived |
|---|---:|---|
| 50, the breach price | -400 USD | `10*(50-100) = -500` against 100 cash |
| 25, after the market moved | -650 USD | `10*(25-100) = -750` against 100 cash |

The 250 USD is what the delay transfers from the defaulter to the insurance
fund. Capping exactly that growth is what a liquidation is for.

**Why it remains a fairness question.** The defaulter's downside is capped at
zero cash by the bankruptcy write-down, so everything beyond that is the fund's.
An actor frozen through a falling market keeps the recovery and not the tail,
while an actor without a pending contract is closed out at the breach. Two
identical losing positions, two different outcomes, and the difference is
whether one of them happened to hold an expired contract awaiting a price.

**Competing readings.** (a) The caller is wrong: the profile's own comment says
fail closed, and skipping the client fails open on the action; an unmeasurable
account should be escalated rather than passed over. (b) The caller is right:
a liquidation whose total exposure cannot be valued cannot be sized, so
declining to act is conservative and the admission freeze is the mitigation.
Not decided here.

**Reachability.** Requires a dated contract reaching expiry with no settlement
price. `expiryUnavailableRetryForever` shows the condition is anticipated and
unbounded in duration. Whether the campaign's configurations produce it is not
tested and must not be assumed from this experiment.

## RT-013 — Funding is not invariant under account partition

**Classification.** EDGE CASE by magnitude. Recorded because it is the **second**
instance of the RT-008 mechanism family, which makes it a pattern rather than a
one-off. No value is lost: the residual is booked to exchange revenue.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`.

**Mechanism.** `settleFunding` charges each position
`TryMulDiv(positionValue, rate, 10000)` **per position**. Truncation is
subadditive, so the same exposure spread over more accounts is charged less. The
code already accounts for the two sides not netting — `netExchangeFlow` is
accumulated explicitly, validated, and routed to exchange revenue with the
comment that on a real venue this is the insurance fund's residual.

**Measured** (`tests/economic_audit_funding_partition_test.go`), with the other
side held in a single account in both arms so only the partition differs:

| split side | one account | eight accounts | exchange residual |
|---|---:|---:|---:|
| payer (long) | pays 8801 | **8800** | 0 → **-1** |
| receiver (short) | receives 8801 | **8800** | 0 → **+1** |

**Fragmentation is not an unconditional advantage.** It moves the rounding away
from the fragmented side's cash flow in both directions: it reduces the
magnitude of whatever that side pays *or* receives. It helps a payer and hurts a
receiver, and the choice reverses when the funding rate does. The hypothesis
predicted the payer half; the receiver half is measured, not assumed.

**Magnitude, stated before the structural claim.** One quote unit in 8801, or
0.011%, bounded by about one unit per extra account per settlement.
Economically negligible.

**Why it is recorded anyway.** RT-008 found the same shape in fees. Two
independent instances make the generalisation worth writing down: **every
per-item integer charge in this system is partition-dependent**, because each
truncates per item and truncation is subadditive. Margin and settlement use the
same `MulDiv` idiom and have not been checked from this angle.

**Two invalid fixtures preceded the valid one, and either would have closed the
hypothesis as falsified.** The first used a mark of 100 USD, where
`AbsMulDiv(size, mark, precision)` divides the size by ten and absorbs the leg
differences before the bps step sees them — it reported exact equality. The
second scanned leg sizes in steps of one, which the same division also swallows:
zero differences across forty offsets. Setting the mark equal to
`BTC_PRECISION`, so position value *is* the raw size and the bps step is the only
truncation, gives thirty-one differing partitions in the same scan. **A null
result from an instrument that cannot resolve the effect is not a null result** —
the third such near-miss in this audit, after RT-006's latency expectations and
RT-010's asynchronous outbox.

## RT-014 — The cost of leverage depends on how much is borrowed

**Classification.** REAL BUG, layer = economics, severity **high in kind**:
reachable by every actor with no special access, recurring every simulated
minute, and it changes relative performance directly because a carry or basis
strategy's economics are set by its funding cost. Campaign-scale magnitude is
**not yet measured** and must not be assumed in either direction.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`.

**Mechanism.** `chargeCollateralInterestLocked` computes, once per simulated
minute per client per asset,

```go
interest, ok := etypes.TryMulDiv(borrowed, e.CollateralRate, collateralInterestDenominator)
...
if interest <= 0 { continue }
```

with `collateralInterestDenominator = 365*24*3600*10000/60 = 5_256_000_000`. At
the default 500 bps that is `borrowed / 10_512_000`. A debt below **10_512_000
quote units — 105.12 USD** rounds to zero and is skipped, and nothing is carried
forward, so the exemption repeats every minute forever. Bisection confirms the
boundary exactly: the largest interest-free debt is 10_511_999 and the first
charged debt is 10_512_000.

**Delivered rate against configured rate**, one simulated day of charges:

| borrowed (USD) | delivered, of the configured 500 bps |
|---:|---:|
| 50 | 0.0 |
| 100 | 0.0 |
| 105 | 0.0 |
| 200 | 262.8 |
| 1 000 | 473.0 |
| 10 000 | 499.3 |
| 100 000 | 499.8 |
| 1 000 000 | 500.0 |

A 200 USD borrower pays 2.6%; a 1 000 000 USD borrower pays 5.0%. The
configured rate is not the delivered rate, and the gap is a function of the
principal.

**Under account partition**, the same 1000 USD of debt over one simulated day:

| held as | interest collected | annualised |
|---|---:|---:|
| one account of 1000 USD | 12 960 | 4.730% |
| ten accounts of 100 USD | **0** | **0.000%** |

**Why this is worse in kind than RT-008 and RT-013.** Those move at most one
quote unit per item — a rounding transfer, negligible in magnitude. This forgives
the *entire* charge below a threshold and delivers a materially wrong rate across
two decades of principal above it, with no residual accumulated.

**Reachability.** Borrowing is enabled in the campaign
(`ex.EnableBorrowing`, `simulations/multivenue/sim.go:2878`, limits of
20 000 000 USD and 20 000 ABC) and `ChargeCollateralInterest` runs as a
deterministic phase job (`exchange/exchange.go:1347`). The mechanism is live.
What is **not** established is the distribution of debt sizes actors actually
carry; at large debts the delivered rate is within 0.2 bps of configured, so the
practical impact could be small. Measuring that distribution from an existing run
is the next step and is cheap.

**Disposition.** Not fixed here. The rate's rounding rule is scientific
economics — accumulating the sub-unit remainder, charging per second instead of
per minute, or scaling the denominator are all defensible and they are the
owner's choice. The tests pin the current behaviour and the exact threshold so
any change is visible.

**Fixture note.** The first two runs reported zero interest for *every*
principal, which read as the threshold swallowing everything. It was the
fixture: the 500 bps default is applied inside `ConfigureAutomation`, not in the
constructor, so an exchange built with `NewExchange` alone carries
`CollateralRate == 0` and charges nothing. The campaign reaches the default the
same way the corrected fixture does. Worth knowing in its own right: any code
that builds an exchange without configuring automation charges no interest at
all.

## RT-014 (continued) — Measured at campaign scale

`research/tools/interestscan/main.go`, run against
`research/configs/clock-control-5h-101.json`, seed 607, 30 simulated minutes,
`-log-mode full`. Development configuration; no holdout used.

The charge is `floor(borrowed·rate/denominator)` per minute, so an observed
amount `A` bounds the debt that produced it and bounds the delivered rate from
below by `A/(A+1)`.

| asset | charged per minute | occurrences | implied debt (raw units) | delivered ≥ |
|---|---:|---:|---|---:|
| ABC | 1 | 76 | [10 512 000, 21 024 000) | **50.0%** |
| ABC | 2 | 18 | [21 024 000, 31 536 000) | **66.7%** |
| ABC | 3 | 16 | [31 536 000, 42 048 000) | **75.0%** |

76 borrow events, 110 charges, 160 quote units collected. **Every debt in the
run sits in the three lowest buckets, so the delivered rate is roughly 250–430
bps against a configured 500.** This is not a threshold curiosity at campaign
scale — it is the operating regime, and borrowers are under-charged by 15–50%
of their interest every minute of the run.

**The threshold is denominated in raw asset units.** All borrowing here is in
ABC, not USD, and the same constant `10 512 000` applies to both. At
`BTC_PRECISION` that is 0.105 ABC, worth about **5 256 USD** at the 50 000
bootstrap — fifty times the 105.12 USD ceiling the same constant imposes on a
USD loan. Two actors with identical dollar leverage pay materially different
rates depending on which asset they borrowed. A single shared constant produces
a per-asset inequity, and it was invisible from the USD-only fixture.

**Scope.** One config, one seed, 30 simulated minutes. Debts may grow over a
five-hour run and move accounts into buckets where the delivered rate approaches
the configured one. Not measured; not assumed either way.

**Instrument note — the fourth in this audit, and the most dangerous.** The
first scan reported 110 charges totalling **zero** quote units, which reads as
"the campaign pays no interest at all". False: the venue logger wraps every
payload one level deeper than the emitting struct suggests
(`data.payload.amount`, not `data.amount`), so the parser read zero for every
record. One look at a raw line settled it. With RT-006's expectation model,
RT-010's asynchronous outbox and RT-013's price conversion, that is four
instruments returning confident wrong answers, three of them nulls. **Read one
raw record before trusting any aggregate computed over it.**

## RT-015 — The borrow gate values cash only

**Classification.** REAL BUG in the sense of an inconsistency between two
valuations of the same account, with the permissive one gating leverage.
Severity medium-high: reachable with no special access, exercised in the
campaign. **Owner decision** — how much leverage an account may take is
scientific economics.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`.

**Mechanism.** `validateCrossMarginCollateral` builds `totalAssetValue` from
`client.PerpBalances` and `client.Balances` at oracle prices, subtracts
`client.Borrowed`, and limits the new borrow against that net equity. It never
looks at positions, and a reservation is an earmark *inside* the balances it
sums. Meanwhile `buildAccountMarginProfile` — the risk engine — does add every
position's unrealized PnL to equity. The two disagree, and the borrow gate is
the more generous.

The gate's own comment shows the author reasoning carefully about the equity
base: "Limit against NET equity (assets minus debt): borrowed-in cash sits in the
balances, so limiting against gross assets would let each borrow enlarge the base
for the next one." The omission of positions and earmarks sits alongside that
care, which is why it reads as an oversight rather than a decision — but the
disposition is still the owner's.

**Measured** (`tests/economic_audit_borrow_valuation_test.go`), collateral factor
0.5, 1000 USD of cash, limit found by bisecting the gate:

| account | economic equity | admitted borrow |
|---|---:|---:|
| no position | 1000 USD | 500.00 |
| long 5 at 100, mark 60 (unrealized −200) | 800 USD | **500.00** |

An engine counting the loss would admit 400. The account borrows 25% more than
its equity supports, against a loss the risk engine already recognises.

**The second arm is the sharper one** — the same capital counted twice rather
than a stale valuation:

| account | cash | reserved | available | admitted borrow |
|---|---:|---:|---:|---:|
| resting bid, 90 ABC at 100 | 1000 | 900 | **100** | **500.00** |

An account with 100 USD actually available borrows 500. The same capital backs
the resting order and the loan simultaneously.

**Competing reading, and why it fails.** One could argue positions are margined
separately, so the cash is genuinely unencumbered. The second arm refutes it:
`PerpReserved` *is* the order and position margin, it sits inside the balance the
gate sums, and the gate therefore counts that margin as collateral for a new
loan.

**Reachability.** Borrowing is enabled in the campaign with auto-borrow on both
wallets, and the 30-minute run in E-022 produced 76 borrow events.

**The tests pin, they do not prescribe.** They fail if the gate starts counting
either the unrealized loss or the earmark, so the disagreement between the two
valuations cannot change silently in either direction.

## RT-014 (correction) — the 30-minute figure was a warm-up artefact

The same config and seed run for its designed 5 simulated hours instead of 30
minutes, scanned with the same tool.

| | 30 min | 5 h |
|---|---:|---:|
| borrow events | 76 | 978 |
| interest charges | 110 | 2 455 |
| collected | 160 | 26 805 |
| charge buckets observed | 1–3 | 1–51 |
| aggregate delivered, lower bound | — | **91.6% = 458 bps of 500** |

**The earlier claim was too strong.** "The delivered rate is roughly 250–430 bps
… at campaign scale this is the operating regime" over-states it: the 30-minute
window sampled the warm-up, when debts are small and sit in the lowest buckets.
Over the full run the aggregate under-collection is at most 8.4%.

**What survives is the distributional claim, which was the finding.** 513 of
2 455 charges — 20.9% — fall in buckets 1–3, where the delivered rate is bounded
below by 50.0%, 66.7% and 75.0%. The cost of leverage still depends on the size
of the debt, and no configuration states it. The exact threshold from the unit
tests is unchanged.

Only three clients borrow in this run (12, 13, 14), so the distortion is
concentrated rather than population-wide here.

**Method note.** A run length chosen for convenience is not a sample of the
regime the campaign reports on. The measurement was right; the scope sentence
attached to it was not.

## RT-016 — Borrow collateral is priced by a static oracle

**Classification.** Specification question with a bounded, measured
consequence. **Owner decision.** Latent in the configuration measured; the
condition under which it becomes live is stated below so it can be checked
rather than assumed.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`.

**Mechanism.** The risk engine values derivative exposure through `riskMark`,
which tracks each instrument's stored funding mark. The borrow gate never
consults `riskMark`: it reads `BorrowingConfig.PriceSource`, and the campaign
supplies `exchange.NewStaticPriceOracle` with ABC pinned to the 50 000 bootstrap
(`simulations/multivenue/sim.go:2862`). ABC is absent from `CollateralFactors`
so it takes the 0.75 default — a 25% haircut, covering a 25% adverse move and no
more.

**Measured** (`tests/economic_audit_collateral_oracle_test.go`), 10 ABC of
collateral:

| market price of ABC | oracle price | admitted borrow |
|---:|---:|---:|
| 50 000 | 50 000 | 500 000 USD |
| **25 000** | 50 000 | **500 000 USD** |
| 25 000 | 25 000 | 250 000 USD |

A 50% fall leaves borrowing power unchanged at twice what the collateral is then
worth.

**How far the market actually moves** (`research/tools/pricerange`), two reads of
one 5-hour control run in progress:

| ABC-USD, trades scanned | low | high | widest |
|---:|---:|---:|---:|
| 144 101 (partial) | −0.24% | +0.04% | 0.24% |
| 257 411 (partial) | −0.48% | +0.04% | 0.48% |
| **544 835 (complete 5 h)** | **−1.06%** | **+0.04%** | **1.06%** |

The band is **not stationary and not symmetric**: ABC's high never leaves +0.04%
while its low walks to −1.06%, so the excursion grows in one direction with run
length, roughly doubling as the sample doubles. CDF-USD against its own 3 000
bootstrap behaves differently — inside **0.17%** over 431 985 trades and moving
both ways — so the drift is a property of ABC here, not of the venue.

At 1.06% against a 25% haircut the oracle is accurate by a factor of about
twenty-four, so the mechanism is latent in this configuration. A claim that it
stays latent in a less anchored configuration is **not** supported and is not
made.

**The condition under which it bites**: any configuration where ABC's excursion
from its bootstrap approaches the haircut. The anchored control does not. A
stress configuration (`research/configs/v005-stress-perp.json` and siblings) is
where this should be re-measured before such a run is treated as economically
faithful. Not measured here.

**Competing reading, kept alive.** A static collateral oracle deliberately
breaks the circularity of valuing collateral with the very market the borrower is
moving, and avoids a liquidation-spiral artefact. Under that reading this is a
documentation gap plus a bounded fairness consequence rather than a defect.
Nothing in the code states the choice.

**Instrument note, the fifth.** The price tool first searched for an event named
`"trade"`. The evidence writes `"Trade"`, and the payload carries **no symbol** —
the book a trade belongs to is the file it is written in. Either mistake yields a
confident empty result. Caught by reading one raw line, which is the standing
rule from RT-014.

## RT-017 — Valuation discipline is inversely ordered to authority

**Classification.** Structural observation over RT-015 and RT-016, not a new
instance. **Owner decision.** No new mechanism; what is new is the ordering.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`.

Three parts of this system value the same account, and they do not use the same
price:

| valuation | decision it feeds | price | staleness discipline | provenance |
|---|---|---|---|---|
| `MarkedAccount` via `populationValuationSpec` | scoring; moves no money | live two-sided ABC/USD mid | bounded window; fails closed on a non-positive mark | records `markSource` |
| `buildAccountMarginProfile` | liquidation | stored funding mark via `riskMark` | fails closed on a settlement-pending sibling | none |
| `validateCrossMarginCollateral` | how much leverage an actor may take | static oracle pinned to the bootstrap constant | **none; the concept is absent on this path** | none |

**Care decreases as authority increases.** The path that moves no money records
where its price came from and refuses to report on a stale one. The path that
decides leverage reads a constant fixed before the simulation began.

**Measured** (`tests/economic_audit_valuation_triad_test.go`), one account, one
instant, 10 ABC held, market at 25 000 against a 50 000 bootstrap:

- scoring equity at the live mark: **250 000 USD**
- scoring equity at the bootstrap mark: 500 000 USD
- borrow admitted against the same 10 ABC: **500 000 USD — 2× live equity**

**The 2× is a demonstration, not a campaign claim.** It uses a 50% price move to
make the mechanism visible. The measured ABC excursion in the control
configuration is 1.06% (RT-016), so the campaign-scale gap is about one percent.
The test labels the extreme as such.

**Why record the ordering separately.** A reviewer looking for where valuation
discipline is weakest should look where the consequences are largest. In this
system that is exactly where it is absent, and neither RT-015 nor RT-016 says so
on its own.

## RT-018 — Borrowed spot exposure is governed by nothing

**Classification.** REAL BUG in the sense of an exposure class with no
enforcement path at all. Severity depends on whether real runs reach negative
equity this way, which is **not measured**. **Owner decision** — whether spot
debt should be liquidatable is scientific economics.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`.

**Mechanism, and it is structural rather than a missing check.** The exchange has
exactly two liquidation entry points — `CheckLiquidations` for perps and dated
futures, `CheckPositionMarginerLiquidations` for options. Both walk **positions**.
Spot debt is not a position: it lives in `Client.Borrowed` and
`Client.BorrowedSpot`. Neither entry point can see it. The borrow gate refuses
*new* credit once equity is gone, but refusing new credit is not unwinding old
exposure.

`AutoBorrowSpot: true` — which the campaign sets — borrows an asset for a
participant short of it at settlement. That is a venue-financed short spot
position.

**Measured** (`tests/economic_audit_spot_debt_test.go`). 100 000 USD account
borrows 1 ABC at 50 000 and sells it; ABC quadruples to 200 000:

| | equity |
|---|---:|
| at entry | +100 000 USD |
| after the move | **−50 000 USD** |

Both liquidation entry points invoked:

| | before | after |
|---|---:|---:|
| ABC debt | 100 000 000 | 100 000 000 |
| USD cash | 15 000 000 000 | 15 000 000 000 |
| insurance fund | 0 | 0 |

Nothing moves, and no event records that the venue is carrying 50 000 USD.

**The asymmetry is the finding.** An actor whose derivative goes bad is closed
out and its deficit charged to the insurance fund — bounded, logged,
attributable (E-018, E-019). An actor whose borrowed spot goes bad is closed out
by nothing and the shortfall is recorded nowhere. Same economic short, different
rules, decided by which instrument expressed it.

**What is not established.** The mechanism is live — E-022 observed real
`auto_spot` ABC borrows — but whether any account in a real run reaches negative
equity this way is not measured, and this finding must not be read as saying it
does. `RepayMargin` exists and ordinary trading retires these debts; what is
absent is the forced unwind.

**Fixture note.** The sale of the borrowed ABC is injected by writing balances
directly rather than crossing a book — an unrecorded mutation of the kind that
invalidated E-007. It cannot affect these assertions, which are the invariance of
debt, cash and fund across the liquidation calls. It would matter if the claim
were about conservation; it is not.

## RT-019 — Expiring contracts share one settlement observation

**Classification.** Bounded no-violation result, pinned. Recorded because
nothing else asserts it and its regression would be visible only to hedgers.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`.

`UpdateDerivativeMarks` resolves **one** underlying observation per tick through
`derivativeUnderlyingPrice` and hands that same number to every expirable via
`ObserveSettlement`. Measured with an option and a dated future on the same
underlying expiring together: both settle at 10 000 000, the spot mid, rather
than at either derivative's own book. A calendar hedge between them therefore
nets exactly.

**Why pin it.** If the shared observation were ever replaced by per-instrument
sampling, calendar hedges would quietly stop netting. No directional participant
would notice and no existing test would fail — only hedgers would pay, which is
the hardest kind of unfairness to detect from aggregate metrics.

**What this does not cover.** Hedging an expiring option with the **perp** leaves
genuine basis risk: the option settles to the spot reference while the perp stays
open at its own mark (`exchange.go:1704`). That is real economics the hedger
owns, not a venue artefact. The original hypothesis assumed otherwise and was
wrong about the economics, not about the code.

**Instrument note, the sixth — and a new shape.** The first run reported that the
future got no settlement observation while the option, from the same call, got
one. That looked like a genuine asymmetry between expirable types. It was the
fixture: `NewExpiringFutures` takes no underlying argument, so a bare
construction leaves `Underlying` empty and the resolution falls through to the
configured index, which publishes only the four spot symbols. The campaign never
builds one that way — `instrument/listing.go:97` sets it, and the listing
scheduler is the only path that lists dated futures.

The five earlier instrument errors were about reading the system wrong. This one
is about building it wrong. **Prefer the construction path production uses; a
bare constructor can leave a field the whole lifecycle depends on.**

## RT-020 — The mark defence bounds manipulation without pricing it

**Classification.** Property of the rules with a measured bound. **Not an
observed exploit** — nothing in the campaign's actor population does this, and
this audit has not searched run evidence for it. **Owner decision.**

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`.

**The defence is better than the hypothesis assumed, and that half is a negative
result worth keeping.** `ensureAnchoredMarkCalcs` installs a
`ClampedEMAMarkPrice` on **every** margined book whose instrument has an
underlying or for which an index provider exists — with the campaign's provider
present, that is all of them, dated futures included. The calculator fails
**closed**: when the index is unavailable it returns an error rather than falling
back to the manipulable mid. The predicted coverage gap does not exist.
`NewExchangeWithConfig`'s own comment names the attack it is defending against:
"a margined book marked at its own mid lets liquidations trade into the very
price that triggers them (self-feeding cascade)."

**What the measurement found** (`tests/economic_audit_mark_manipulation_test.go`):

| step | value |
|---|---|
| index | 50 000 |
| honest two-sided market | 49 000 / 51 000 |
| one minimum-size bid inside the spread, never trading | mid → **+1.90%** |
| mark after 200 passes | **+1.90% of index** |
| clamp | ±3.00% of index |
| maintenance margin rate | 500 bps |

One unit of base quantity — `1`, not one lot — resting inside the spread and
never trading moves the mark 1.90%. The clamp is never reached, so the mid sets
this number, not the band.

**Why the number matters.** Maintenance margin is 500 bps, so 1.90% is **38% of
the entire maintenance buffer** and the clamp permits up to 3.00%, or **60%** of
it. An account near maintenance can be pushed materially toward or away from
liquidation by a participant risking one unit. Funding is charged on position
value at the same mark, so the same quote also changes what every other holder
pays that interval.

**The gap is that the bound is not priced.** The anchored calculator reads
`book.GetMidPrice()` — an unweighted mid — so the displacement is independent of
the quoting actor's size. A quantity-weighted calculator exists in the codebase
(`WeightedMidPriceCalculator`) and is not what the anchor uses. Weighting the
mid, narrowing the band, or requiring a minimum resting quantity to influence the
mark are all defensible responses and all change scientific economics.

**Instrument note, the seventh.** The first version posted the manipulating bid
*through* the ask — index + 20 000 against an ask at index + 10. That is a
marketable order, not a resting quote, and it left the book crossed, so
`GetMidPrice` failed, the calculator returned the bare index, and the test
reported a mark move of exactly 0.0000% — reading as a complete defence. **A
manipulation fixture must post a quote the venue would actually leave resting; a
crossed book measures the error path, not the mechanism.**

## RT-021 — The index has no staleness bound

**Classification.** Structural property with a measured mechanism. Frequency in
real runs **not measured**. **Owner decision.**

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`.

RT-020 showed the mark is anchored to the index and clamped ±3% around it, which
makes the index — not the mark — the load-bearing number. This is the index's own
robustness.

`spotIndexProvider` in consensus mode takes a median over
`venueMids[symbol][venueID]`. That map is **overwritten, never aged**, and the
campaign's call site writes only when `TwoSidedMidPrice(symbol)` succeeds. A
venue that goes one-sided stops updating without losing its vote.

**Measured** (`simulations/multivenue/economic_audit_index_consensus_test.go`):

| observations | index |
|---|---|
| one venue at 100 | 100 |
| two venues at 100 and 200 | **200** — the upper, not the average |
| three at 100, 101, 100 000 | 101 — one venue cannot carry it |
| two live at 400, one silent last seen at 100 | 400 |
| **one live at 400, two silent last seen at 100** | **100** |

The last row is the finding: with the campaign's three venues, two silent ones
outvote the only live market, and **the index publishes a price no venue is
currently showing**. "Silent" is a low bar — `TwoSidedMidPrice` needs both sides,
so an active market quoting only bids already qualifies.

**Median-of-three is a real defence** and the third row shows it working against
a venue quoting 100 000. The weakness is not the median; it is that membership is
permanent.

**Where this sits.** The scoring path, which moves no money, takes an explicit
`maxStaleness` and records whether the mark it used was fresh or `recent_`. The
index that drives every mark takes none. That is RT-017's inverse ordering again,
one level deeper: **the anchor with the most authority has the least memory
discipline.**

**Not established.** Whether any venue's `ABC/USD` book goes one-sided during a
run, and for how long, was not measured. The mechanism is structural; its
frequency is an empirical question this experiment did not ask.

## RT-021 (quantified) — the stale-vote configuration is absent where it would matter most

`research/tools/booksidedness/main.go` against `clock-control-5h-101.json`,
seed 607, full 5 simulated hours. A venue is "silent" for the index when its book
is one-sided, because `TwoSidedMidPrice` then fails and its entry is not updated.
18 000 snapshot instants:

| symbol | north | central | south | ≥2 silent | all silent | **exactly 2 silent** |
|---|---:|---:|---:|---:|---:|---:|
| `ABC/USD` | 0.40% | 0.40% | 0.40% | 3 (0.02%) | 3 (0.02%) | **0** |
| `CDF/USD` | 0.64% | 4.76% | 3.83% | 71 (0.39%) | 4 (0.02%) | **67 (0.37%)** |
| `ABC/CDF` | 7.35% | 8.67% | 8.36% | 456 (2.53%) | 16 (0.09%) | **440 (2.44%)** |

**The distinction that decides severity.** The harm needs *exactly* two silent
venues and one live one, so two stale observations outvote a live market. When
**all** venues are silent nobody updates and the median is entirely stale, but no
live market is contradicted — a weaker condition. The last column separates them.

**On `ABC/USD` the harmful configuration occurred zero times in 18 000
instants.** That is the book anchoring the perp mark, and therefore margin,
liquidation and funding. Every multi-venue silence there was total silence.
RT-021's consequence for the margin system is, in this configuration, absent
rather than merely rare.

**On `ABC/CDF` it is common** — 2.44%, about 440 instants, roughly one per 41
seconds of simulated time — and `CDF/USD` sits between at 0.37%. The mechanism is
live on the books whose index feeds cross-asset pricing, not on the one that
governs margin.

**A second observation the table forced.** Venue silence is not symmetric: on
`CDF/USD`, central is one-sided 4.76% of the time against north's 0.64%. The
venues contribute unequally to the consensus that prices everyone — an
actor-fairness input in its own right, and not something this experiment set out
to measure.

**Scope.** Snapshots are periodic, so this is a *sampled* silence rate; silence
that begins and ends between two snapshots is invisible. Per-venue snapshot
counts slightly exceed the distinct instant count, so a few instants carry more
than one snapshot per venue; the "exactly 2 silent" column is derived as `≥2`
minus `all` rather than counted directly.

## RT-022 — Venue placement carries an environment term in the campaign's comparisons

**Classification.** Methodological, **not an economics defect**. The venues are
genuinely different environments and a venue-local strategy's result should
depend on its venue. What follows is how the campaign's numbers must be read.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`.
Run: `clock-control-5h-101.json`, seed 607, 5 simulated hours.

The campaign places the same participant counts on all three venues, so every
role class exists three times over — a within-class design that holds strategy
fixed and varies only the environment.

| class | n/venue | between-venue | within-venue | ratio | **% of result** |
|---|---:|---:|---:|---:|---:|
| `triangle_arb` | 2 | 1 119 237 | 3 819 | 293× | **8.23%** |
| `elastic_supplier` | 8 | 41 586 | 191 | 218× | 0.39% |
| `dated_carry_arb` | 2 | 10 818 | 90 | 120× | 0.41% |
| `latent_liquidity` | 6 | 98 086 | 7 407 | 13× | — |
| `fixed_distance_maker` | 8 | 46 486 | 1 860 149 | 0.02× | — |
| `noise_flow` | 6 | 1 221 324 | 32 347 493 | 0.04× | — |

`triangle_arb` returns **+14 258 332** on north, +13 393 986 on south,
+13 139 095 on central.

**Read the last column, not the ratio.** The ratio is inflated: same-class actors
on the same venue are near-clones of deterministic strategies, so the
within-venue spread is tiny by construction and dividing by it produces large
numbers whether or not the venue effect matters economically.
`elastic_supplier`'s 218× is a venue spread of 0.39% of its own result. Only
`triangle_arb`, at 8.23%, is large enough to change a conclusion.

**The finding is methodological.** Any statement of the form "class X
outperformed class Y" averages over three environments that E-031 already showed
are not equivalent (`ABC/CDF` one-sidedness: 7.35% north, 8.67% central). For
classes at the bottom of the table this is irrelevant. For `triangle_arb` a
comparison that does not control for venue carries an environment term worth
8.23% of the result.

**Instrument note, the eighth.** The first version grouped by the numbered role
(`elastic_supplier_3`) rather than the class, so every group held one participant
per venue, the within-venue spread was zero by construction, and every ratio
printed `inf`. The tool now strips the index, and a class that genuinely has one
participant per venue is reported as "no yardstick" rather than given a ratio
against zero.

## RT-023 — The venue design confounds matching rule with funding interval

**Classification.** Methodological. Makes RT-022's environment term real but
**unattributable**. Single configuration, single seed.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`.

| venue | matching rule | funding interval |
|---|---|---|
| north | `price_time` | 8 h |
| central | `pro_rata` | 1 h |
| south | `pro_rata` | 2 h |

North is the only price-time venue **and** the only long-funding venue. Any
north-versus-others contrast therefore mixes the two, and the campaign's three
venues do not contain the cell that would break it — price-time with a short
funding interval.

**The one clean contrast shows the confound is not negligible.** `central` versus
`south` holds the rule fixed and varies only funding, 1 h against 2 h:

| class | rule axis (contaminated) | funding axis (clean) | mean |
|---|---:|---:|---:|
| `triangle_arb` | +991 791 | −254 891 | 13 597 138 |
| `abc_cdf_spot_maker` | +415 725 | **−3 279 751** | −2 100 770 |
| `noise_flow` | −584 037 | **+1 221 324** | −9 649 395 |
| `imbalance_maker` | +122 266 | −124 211 | −483 679 |
| `fixed_distance_maker` | −33 264 | −26 444 | −558 857 |

For `abc_cdf_spot_maker` and `noise_flow` the funding term alone is several
times the quantity being attributed to the matching rule. Attributing a venue
effect to either factor is unsupported.

**A prediction that failed, recorded as such.** H-032 predicted the rule axis
would dominate and that its sign would be consistent across liquidity-taking
classes. Neither holds: the funding axis is larger for several classes, and the
rule axis's sign splits across classes with similar execution needs —
`abc_cdf_spot_maker` gains where `fixed_distance_maker` loses.

**Process error recorded with the finding.** RT-022 framed the venues as
"genuinely different environments" as though discovering it. The configuration
states the heterogeneity explicitly and I read it only after reporting. **Read
the configuration before characterising what a measurement means.**

**Scope.** One configuration, one seed. Whether the per-class signs are stable
across seeds is not measured; with a single run they could as easily be sampling
noise as structure.

## RT-022 / RT-023 (replicated) — the venue term is structure; the funding axis is the attributable one

Seeds **607, 608, 609, 610**, `clock-control-5h-101.json`, 5 simulated hours
each. Development seeds; no scientific holdout touched.

**RT-022 upgraded.** `triangle_arb`'s rule-axis term is **+991 791, +800 529,
+653 236, +792 955** — positive in all four seeds, same order of magnitude,
never near zero. The venue term is structure, not sampling noise.

**The control worked.** The prediction required small-magnitude classes to flip,
or the sign test would be meaningless. They do: `option_flow` (−++−),
`latent_liquidity` (+++−), `dated_carry_arb` (+++−), `parity_arb` (+++−),
`elastic_supplier` (+++−).

Rule-axis sign stable in all four: `triangle_arb`, `imbalance_maker`,
`spot_maker`, `option_dealer`, `metaorder_trader`, `perp_maker` (all +),
`fixed_distance_maker`, `vanna_volga_desk` (both −).

**RT-023's example was wrong; its conclusion is not.** RT-023 cited
`abc_cdf_spot_maker` as the clearest case of funding swamping rule. That class's
*rule* axis is the least stable quantity in the table — +415 725, −1 921 747,
−2 172 357, −365 331 — so it was the wrong witness. The confound is structural,
so the conclusion stands; the example is replaced.

**What replication newly identifies.** `central` versus `south` holds the rule
fixed and varies only funding (1 h vs 2 h), so it is the one clean contrast, and
for several classes it is stable across all four seeds:

| class | 607 | 608 | 609 | 610 | mean |
|---|---:|---:|---:|---:|---:|
| `noise_flow` | +1 221 324 | +323 478 | +2 139 099 | +2 816 095 | **+1 624 999** |
| `abc_cdf_spot_maker` | −3 279 751 | −186 314 | −6 065 150 | −8 189 817 | **−4 430 258** |
| `imbalance_maker` | −124 211 | −249 168 | −66 494 | −211 055 | −162 732 |
| `perp_maker` | +11 432 | +74 820 | +157 292 | +39 387 | +70 733 |

**Shortening the funding interval from 2 h to 1 h is worth +1.6 M to
`noise_flow` and −4.4 M to `abc_cdf_spot_maker` on average** — identified,
replicated, and larger than the unattributable rule term for both.

**Scope.** Four seeds, one configuration, 5 simulated hours each. Sign stability
at n=4 is weak on its own; it carries here because the small-magnitude classes
visibly fail it.
