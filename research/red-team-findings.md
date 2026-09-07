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
