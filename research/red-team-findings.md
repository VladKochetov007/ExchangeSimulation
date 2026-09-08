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

## RT-024 — The population ledger closes

**Classification.** Bounded no-violation result, plus a reusable screen. No
population-level closure check existed before this; `StrictPopulationAccounting`
requires only that every participant *has* an initial and terminal marked
account, which is completeness, not a sum.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`.
`clock-control-5h-101.json`, seed 607, 5 simulated hours.

    residual = Σ (terminal equity − initial equity) + fee revenue + insurance fund

| venue | participant net | venue take | residual | implied ABC | per head |
|---|---:|---:|---:|---:|---:|
| central | −373 509 674 | 1 115 991 | −372 393 683 | 708 579 | **8 239** |
| north | −371 756 050 | 1 136 102 | −370 619 948 | 705 204 | **8 200** |
| south | −371 024 227 | 1 101 923 | −369 922 304 | 703 877 | **8 185** |

ABC drifted **−1.051%** over the run. Dividing each residual by that drift gives
the inventory it implies: **8 185–8 239 ABC per participant**, against a
configured maker endowment of 10 000. With 86 participants per venue of whom
only the makers are endowed at that level, an implied average slightly under
10 000 is what a closed ledger should produce. The residual is revaluation of a
net-long population, not unaccounted value.

**Instrument note, the ninth, and the most misleading yet.** The first version
reported the residual as a percentage of gross participant flow and printed
**−82%**, which reads as catastrophic. Gross flow is itself dominated by the same
revaluation, so normalising by it makes any revaluation look total. The mark
drift is the correct normaliser because dividing by it yields implied inventory,
a quantity checkable against a configured number. **Do not normalise by a
quantity that contains the effect being measured.**

**Limit, as preregistered.** The screen separates revaluation from an accounting
gap by magnitude and consistency, not exactly. A gap smaller than the ~2%
difference between the implied 8 200 and the endowed 10 000 would be invisible.
Closing that needs per-account inventory, which `MarkedAccountSnapshot` does not
carry — every field it reports is already valued at marks.

## RT-024 (addendum) — why the exact version cannot be built from the current surface

An attempt to replace RT-024's magnitude screen with an exact check failed, and
the reason is worth recording because it bounds what any future population check
can do.

The plan: run the simulation, then value the terminal population a second time at
the **initial** marks, so revaluation is zero by construction and the residual is
pure accounting. Measured residual: **−1.27 M USD per venue** against a take of
1.12 M — which reads as a large unaccounted gap.

**It is not one.** `MarkedAccount` passes the supplied `AccountValuationSpec`
only to `valueWallet` and `valueIsolated`, the spot and perp *balances*.
Derivative exposure is valued through `riskMark(book.Instrument, book)`, which
reads the instrument's stored marks and **ignores the spec**. Fixing the spec
removes wallet revaluation only; the residual is the derivative revaluation the
method was meant to eliminate. The harness did remove 99.4% of the change RT-024
measured — −373.5 M down to −2.39 M — but the remainder is precisely the term
that mattered.

**What would be needed**: a valuation entry point accepting derivative marks as
well as asset marks, so an account can be revalued at a fixed point in price
space. It does not exist, and adding one is a change to a scientific-branch
surface this audit does not make.

**A collision risk, checked and cleared.** Client IDs come from a *per-venue*
counter, so all three venues use 1..86, and every tool here that keys an initial
value by client ID alone collides across venues. Measured: all 86 shared IDs
carry the same role and the same initial equity on all three venues, so the
lookup returns the correct value and RT-022, RT-023, RT-024 and E-034 are
unaffected. Recorded because the next tool to key by client ID needs to know.

## RT-024 (consolidated) — the population gap is not closable from the current artifacts

Three routes tried, three distinct documented failures:

| route | why it fails |
|---|---|
| magnitude screen (RT-024) | cannot separate revaluation from a gap below ~2% of the endowment |
| fixed-mark revaluation (E-036) | `AccountValuationSpec` reaches wallet balances only; derivative exposure is valued from the instruments' own marks and ignores the spec |
| regression on drift (E-037) | no drift leverage across seeds, and inventory is a per-seed variable so the model is misspecified |

**E-037 in detail.** Fitting `residual = inventory × drift + gap` across seeds
607–614 gives intercept **−239 727 063 ± 1 572 059 399** with **R² = 0.0398** —
a standard error 6.5× the estimate. The preregistration named this outcome in
advance and it is reported rather than dressed up.

Two independent causes. The drifts span only **0.1527%** (−1.1102% to −0.9575%),
so the regressor barely varies. And R² of 0.04 says the residual is not tracking
drift at all: the model treats inventory as a constant across seeds, but
participants trade, so the terminal net long position is a **per-seed variable**.
More seeds would not help — the model is wrong, not underpowered.

**The concrete request this produces.** The artifacts record equity but not the
inventory behind it; every field of `MarkedAccountSnapshot` is already valued at
marks. **One additional field — the population's net base-asset position at each
capture — makes the gap computable directly**, with no regression and no
valuation surgery. That is the useful output of these three experiments, and it
is small enough to be worth doing.

## RT-025 — The live conservation check is silenced by the logging configuration

**Classification.** REAL BUG, layer = evidence/detector. Same shape as RT-001,
one layer up. **Owner decision** on the remedy.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`.

`verifyConservation` runs on every automation tick in the venue's
`PostDerivativeMarkHook`:

```go
violations := v.Exchange.VerifyConservation()
if len(violations) == 0 { return }
log := v.makerStateLog
if log.sink == nil && log.inner == nil { return }   // violations discarded
for _, violation := range violations {
    log.LogEvent(now, 0, "conservation_violation", violation)
}
```

`makerStateLog` is assigned only when `LogMode == "full"` or a checkpoint sink
exists (`sim.go:2718`). Otherwise it is the zero `venueLogger`, the guard
returns, and **the violations are computed and thrown away**.

**Measured.** A `-log-mode none` run writes `greeks.json`,
`terminal-outcome.json`, `latency.json`, `manifest.json` — and **no `venues/`
directory**. `terminal-outcome.json` has no conservation field. `sim.go:4117` is
the only emission path in the tree. Under logs off, a violation leaves **no trace
anywhere**: no counter, no error, no flag.

**Why it is the same defect as RT-001.** The exchange takes explicit care to keep
recording independent of logging, and says why on `logBalanceChange`: "a movement
that happens while no logger is attached is still a movement, and leaving it out
of the running total would make the verification depend on the logging
configuration." Recording is independent. **Reporting the violation is not.**

**Scope, including about this audit's own evidence.** The campaign configures
`"log_mode": "full"`, so its headline runs do report violations. Every logs-off
run does not — performance work, ablations, and **the last eight experiments in
this audit**, which were unknowingly unverified on this axis. Stated plainly
because it applies to my own runs as much as anyone's.

**Remedy is the owner's.** Surfacing a violation count in
`terminal-outcome.json`, or failing the run outright, both change what a run
reports.

## RT-026 — Four checks whose only output is a log line

**Classification.** Defect class, generalising RT-001 and RT-025. **Owner
decision**, and one decision covers all four members.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`.

Method: enumerate every `LogEvent` whose name marks a failure, then ask whether
any second observer sees the same condition — a returned error, a counter, a
state flag, an artifact field.

| site | event | second observer? |
|---|---|---|
| `simulations/multivenue/sim.go:4117` | `conservation_violation` | none (RT-025) |
| `exchange/collateral_interest.go:138` | `margin_interest_failed` | **none** |
| `exchange/exchange.go:2426` | `funding_settlement_failed` | **none** |
| `exchange/expiry.go:78` | `price_unavailable` | partial |

Every one is `if log != nil { log.LogEvent(...) }` and nothing else.
`ChargeCollateralInterest` swallows its error and calls the reporter;
`CheckAndSettleFunding` reports and continues to the next contract.

**The material one is the interest failure.** RT-014 established collateral
interest as a live charge — borrowing is enabled, delivered rate ≈458 bps of a
configured 500. If `chargeCollateralInterestLocked` errors, the sweep is
abandoned, no interest is charged that minute, and under logs-off there is no
trace at all: free leverage for as long as the condition lasts, with nothing to
say it happened.

**`price_unavailable` is the partial case, and the split matters.** On the expiry
path the condition also sets `settlementPending`, durable state that gates
admission (RT-012) — a real second observer. On the **liquidation** path it does
not: the margin profile fails, `CheckLiquidations` reports and `continue`s, the
account goes un-assessed. RT-012 measured that consequence; this adds that the
diagnostic also disappears when logs are off.

**The contrast case.** Order rejections use `rejectWithLog`, which logs **and
returns the rejection to the caller**. The caller observes it whatever the
logging configuration. That is what a check with a second observer looks like,
and it is why these four stand out rather than being the house style.

**Prediction accuracy.** The sweep was preregistered expecting "one or two beyond
the two already known". There are three — approximately right, recorded as such.

## RT-026 (sharpened) — a reporting-channel defect, not an error-handling one

A second sweep looked for the sibling class: errors on value-moving paths that
reach **nothing** — discarded returns, or blocks that neither report nor
propagate. Scope `exchange/`, where value moves.

**There is no second member.**

- Discarded error returns in `exchange/`: **zero**. The package contains no `_ =`
  assignment at all.
- Blocks that neither report nor propagate: **zero**, after classifying six
  structural candidates. Five propagate by routes a regex cannot see —
  `settleFunding` carries a captured `arithmeticError` out of its callback,
  `exchange.go:1127` is a comma-ok accessor, and `exchange.go:1613`/`:1623` are a
  deliberate deferral that appends the failed symbol and its error to a
  `deferred` slice so the condition survives as state.
- The sixth, `mustMarshalJSON` returning `""` on error, has one call site and
  marshals a **string**. `json.Marshal` cannot fail on a string; the branch is
  unreachable.

**The two sweeps together state the defect precisely:**

> The exchange propagates or reports **every** error it encounters. What it does
> not do, for four specific conditions, is give that report a **second
> observer**.

A remedy framed as "handle errors properly" would find nothing to fix. The fix is
to **add a channel** — a counter, a returned error, a field in
`terminal-outcome.json` — not to change error handling.

This is also the honest context for where this audit's findings sit: the
arithmetic and error-handling layers have held under every sweep, and the
findings have accumulated at the specification, reporting and valuation layers.

## RT-027 — Every actor acts, but one class effectively leaves the market

**Classification.** Open question, not a finding in either direction. The
liveness result is a bounded negative; the collapse it exposed is unresolved.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`.
`clock-control-5h-101.json`, seed 607, 5 simulated hours, full logs.

All 21 role classes place orders in both halves of the run: **no class is inert
and none stops entirely.** That was the predicted outcome and it holds.

**The instrument was wrong, and it nearly hid the interesting row.** The first
version tested only for zero and printed "active" for everything, including:

| class | first half | second half | ratio |
|---|---:|---:|---:|
| `dated_carry_arb` | 2 374 | **6** | **0.003** |
| `cdf_spot_maker` | 18 430 | 3 646 | 0.198 |
| `fixed_distance_maker` | 60 438 | 36 126 | 0.598 |

**A zero-test is not a liveness test.** A class down 99.7% is not "active" in any
sense a reader would accept. The tool now reports the ratio and names a collapse.

**What is and is not established.** Three dated futures list in the run, expiring
at 2 h, 4 h and 6 h; the 5-hour run ends before the third expires, and exactly
one relisting happens — the 4 h contract when the 2 h one expires, then nothing.
The dated board thins from three contracts to one, and `dated_carry_arb` trades a
relationship *between* contracts, so some decline is expected. That a decline to
**0.003** is expected is **not** established, and this experiment cannot separate
design from stall without reading the actor's trigger condition.

**Why it matters for the campaign's numbers.** The class still contributes a full
row to every class-level average and to E-032's venue table, where it showed one
of the higher between-venue ratios. If it spends most of the run out of the
market, its score measures a shorter and different period than its peers' — an
unearned difference between actors arising from the instrument board rather than
from strategy.

**Next and cheap**: read `dated_carry_arb`'s trigger to see whether it requires a
contract pair that ceases to exist, or should still be quoting the one that
remains.

## RT-027 (resolved) — the collapse is design, and the caveat has a named cause

Reading `simulations/derivsim/carryarb.go` settles the open question.

**Two gates bound the desk, both deliberate.**

1. **Net position cap per symbol.** `MaxPosPerSym = 5 × mvBasePrecision`,
   `LotQty = mvBasePrecision / 10` — **50 lots per contract** — and each arm of
   the trading switch is guarded by `st.position > -MaxPosPerSym` /
   `< MaxPosPerSym`. At the cap in the direction the basis favours, that contract
   stops trading.
2. **Edge scaled by time to expiry**, `edge = EdgeBps × sqrt(timeToExpiry /
   TenorNano)`. This *lowers* the bar as expiry approaches, so it cannot cause a
   collapse — it would cause the opposite. Ruling it out is what makes the cap the
   explanation rather than a guess.

**Verdict: design.** The cap binds against a persistently one-signed basis while
the board thins from three contracts to one (expiries at 2 h, 4 h, 6 h against a
5-hour run, one relisting). No ghost order, no blocked timer, no stalled loop.

*Arithmetic, consistent-with rather than proof.* Saturation predicts
2 desks × 3 contracts × 50 lots × 2 legs × 3 venues ≈ 1 800 orders; the first half
shows 2 374. The 32% excess is churn — the cap is on *net* position, so a basis
that changes sign lets the desk unwind and re-accumulate. Magnitudes and mechanism
agree; the difference is explained, not ignored.

**The caveat survives with a cause.** The desk earns its result early and then
sits at its cap, so its score is front-loaded and bounded by `MaxPosPerSym` while
an uncapped class compounds for the full run. A class-level ranking across the two
compares different things — not because either is broken, but because the
parameter binds one and not the other. That is now attributable to a named
configuration value rather than to a suspicion.

## RT-028 — The classes are not given the same board, and the board is not reported

**Classification.** Not a defect — prudent risk design. The finding is that the
constraint is **invisible in the evidence**, so class-level results cannot be
normalised for it. **Owner decision**; the remedy is to emit a constant already
known at construction.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`.

Every ceiling below is in the same unit, `mvBasePrecision` = one ABC:

| class | ceiling | in ABC | source |
|---|---|---:|---|
| `option_value_taker` | `OptionValueTakerMaxPosition` default | **1** | `sim.go:1216` |
| `dated_carry_arb` | `MaxPosPerSym` | **5** | `sim.go:3441` |
| `fixed_distance_maker` | `MaxInventory` | **200** | `sim.go:3361` |
| `imbalance_maker` | `MaxInventory` | **200** | `sim.go:3376` |
| `carry_arb` | `CarryMaxPosition` default | **500** | `sim.go:1171` |
| `elastic_supplier` | `MaxPosition` | **10 000** | `sim.go:3587` |
| `parity_arb` | `MaxTrades: 100 000` | — | a trade count, not a position |

**Four orders of magnitude** separate the tightest from the loosest.
`dated_carry_arb`, whose collapse opened this thread (RT-027), sits at 5 — forty
times tighter than a maker, two thousand times tighter than a supplier.

**Consequence for a class-level number.** A per-run result is edge per unit times
units allowed. Two classes with identical skill and identical opportunity post
results differing by their ceiling ratio. E-032's magnitudes read consistently
with that: `elastic_supplier` −10.6 M and `triangle_arb` +13.6 M against
`dated_carry_arb` −2.6 M is as much a statement about allowances as about
strategies.

**Why it is worth recording despite not being a defect.** Nothing in
`greeks.json`, `terminal-outcome.json` or the population artifact carries the
constraint a class operated under. RT-022 found an environment term arriving
through the venue; this is the same shape arriving through the actor
configuration — except that unlike the venue term it is a **known constant** at
construction time and could simply be emitted beside the score.

## RT-029 — Normalising by capital reorders the middle of the table

**Classification.** Not a defect — classes are endowed differently by design.
The finding is that the denominator is absent from the reported result, so two
specific conclusions drawn from absolute PnL do not survive normalisation.
**Owner decision.**

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`.
`clock-control-5h-101.json`, seed 607, 5 simulated hours.

**Capital spans 510×**: `round_trip` at 265 M against `latent_liquidity` at
135 054 M — wider than RT-028's ceiling spread produces in practice.

**The top is stable.** Absolute: `triangle_arb`, `option_dealer`, `round_trip`.
By return: `triangle_arb`, `option_dealer`, `vanna_volga_desk`. The first two
hold, so **`triangle_arb`'s dominance is not a capital artifact** — +3.675% on
2 220 M is the best rate as well as the largest amount. The campaign's headline
result survives normalisation.

**The middle is scrambled** — 62 rank-places of displacement over 21 classes:

| class | rank by absolute | rank by return | capital | return |
|---|---:|---:|---:|---:|
| `latent_liquidity` | 21 | **11** | 135 054 M | −0.351% |
| `metaorder_trader` | 6 | **18** | 1 854 M | −0.518% |

`latent_liquidity` looks like the worst performer at −474 M and is mid-table per
unit of capital. `metaorder_trader` looks sixth-best at −9.6 M and is
fourth-from-last. Both move on capital alone.

**Two conclusions that do not survive**: that `latent_liquidity` performs worst,
and that `metaorder_trader` performs well.

**Prediction accuracy.** The preregistration expected the *top three* to reorder
materially. A single-place swap happened there; the material reordering is in the
middle, which the prediction did not anticipate. Recorded as partially right.

**With RT-028**: the ceiling and the capital are both known constants, both
absent from the reported result, and both change how a class-level table should
be read.

## RT-030 — The triangular residual never changes sign (magnitude withheld)

**Status: OPEN.** The sign result is recorded; the magnitude claim is withheld
pending one named check.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`.
`clock-control-5h-101.json`, seed 607, 5 simulated hours, full logs.

E-044 showed `triangle_arb` earning **+81.6 M on 2 220 M (+3.675%)** in five
hours while almost every other class lost money. Triangular arbitrage is riskless
by construction, so something must be quoting inconsistently for five hours.

**The sign test, across ~16 000 instants per venue where all three books are
two-sided:**

| venue | positive | negative | sign flips | dominant sign |
|---|---:|---:|---:|---:|
| north | 16 438 | 159 | **6** | 99.0% |
| central | 15 607 | 54 | **36** | 99.7% |
| south | 15 704 | 169 | **14** | 98.9% |

Six sign changes in five hours. **This is not an inconsistency being competed
away** — the residual holds one sign for essentially the entire run on every
venue. The arbitrageur's profit is not a contested market outcome.

**The magnitude, and why it is withheld.** Mean |residual| 38%, max 68.8%.
Terminal north state: `ABC/USD` 49 476.20 USD/ABC, `CDF/USD` 3 001.50 USD/CDF,
implied cross **16.4838** CDF/ABC, observed `ABC/CDF` **5.1415** CDF/ABC — a
ratio of **3.206**. The configuration's own bootstrap,
`MulDiv(mvBootstrapPrice, mvBasePrecision, mvCDFBootstrap)` = 1 666 666 667 =
**16.6667** CDF/ABC, matches the implied rate and not the observed book.

Two readings: a real 3.2× dislocation that `triangle_arb` harvests and
`abc_cdf_spot_maker` pays for (−12.6 M, sign fits), or an error in my unit
scaling. A standing 3.2× dislocation with an arbitrageur present is not what a
working ecology looks like, and this audit has caught twelve instrument errors,
three of them confident readings at the wrong scale. The bootstrap matching the
implied rate favours the first reading but does not settle it, since a bootstrap
sets an initial price and does not pin the book.

**The decisive check, cheap and named**: read `abc_cdf_spot_maker`'s anchor. If it
quotes around its own book's mid, the book can drift arbitrarily from the implied
rate and the dislocation is real with a mechanism. If it quotes around the implied
cross rate, my units are wrong and the residual is my tool's artifact.

## RT-031 — The cross book is self-referential and drifts 69% from its bootstrap

**Classification.** REAL, and it changes what the cross-asset population's
results mean. Resolves RT-030's withheld magnitude.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`.
`clock-control-5h-101.json`, seed 607, 5 simulated hours.

**The unit question is settled by comparing the book to itself.** `ABC/CDF`
against its own configured bootstrap
(`MulDiv(mvBootstrapPrice, mvBasePrecision, mvCDFBootstrap)` = 1 666 666 667):

| | price | vs bootstrap |
|---|---:|---:|
| high | 1 669 200 000 | **+0.15%** |
| last | 514 200 000 | **−69.15%** |

Same book, same units, so no unit error can produce this. It starts at its
bootstrap and ends 3.24× below. `ABC/USD` moved −1.05% and `CDF/USD` +0.08% over
the same run.

**The mechanism is one line:**

```go
crossConfig := stoikovConfig("ABC/CDF", "ABC/CDF", crossBootstrap, mvBasePrecision, crossTick)
```

`stoikovConfig(symbol, reference, …)` sets `ReferenceSymbol: reference`, and both
arguments are `"ABC/CDF"`. **The maker quotes around its own book.** Nothing ties
the cross to the implied rate `ABC/USD ÷ CDF/USD`.

**And it is self-referential at the population level too.** `ABC/CDF` is
published by `spotIndexProvider`, whose consensus is the **median of the three
venues' own mids of that symbol** — a consensus of itself. All three venues drift
together, which is what E-045 measured: mean |residual| 37.8–38.2% on all three,
with 6–36 sign flips in five hours.

The code names this failure mode in `anchor.go`: *"a market with no reference of
its own falls back to its book midpoint and becomes self-referential."* The cross
book is that case, and the index publishing its symbol does not rescue it,
because the index is built from the same books.

**Consequence.** Triangular consistency is enforced by nobody except
`triangle_arb`, itself capped like every class (RT-028). E-044's headline —
`triangle_arb` best in the population on both absolute and return measures at
+3.675%, `abc_cdf_spot_maker` losing 12.6 M as counterparty — is explained: it
harvests a 69% standing dislocation rather than outcompeting anyone. **Any
cross-asset or triangular conclusion drawn from this configuration measures a
self-referential book drifting, not a market.**

## RT-032 — The ABC/USD price level is a configured peg, not a market outcome

**Classification.** REAL, and it is the widest-reaching finding in this audit: it
governs what every price-level result in the campaign means.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`.
`clock-control-5h-101.json`, seed 607, 8 simulated hours, `-log-mode full`.

**Mechanism.** `ElasticSupplier` targets
`position = −(percent above reference) × ElasticityPerPercent`. With
`ReferenceHalfLife` at zero the reference never moves off its seed, so the
participants are a peg at `mvBootstrapPrice` = 50 000 USD rather than a demand
curve. The effective config confirms `"elastic_supplier_reference_half_life": 0`
survives normalization; no default overrides it.

`supplier.go` documents this exact outcome: *"A fixed reference is an exogenous
fundamental... measured over six runs the terminal price was minus excess supply
over aggregate elasticity to three significant figures, which is that actor's
configuration read back out rather than a market outcome."*

**Measured** (`research/tools/elasticpeg`, positions taken from wallet balances,
not the actor's own counter):

| venue | terminal mark | vs 50 000 | aggregate position | predicted by price | ratio |
|---|---:|---:|---:|---:|---:|
| central | 4 929 505 000 | −1.4099% | +1 692.00 ABC | +1 691.88 ABC | **1.0001** |
| north | 4 929 380 000 | −1.4124% | +1 695.00 ABC | +1 694.88 ABC | **1.0001** |
| south | 4 929 375 000 | −1.4125% | +1 695.36 ABC | +1 695.00 ABC | **1.0002** |

Three independent venues, ratio 1.000 to four significant figures, **0 of 8
participants at `MaxPosition` on every venue**. Not a cap, no overshoot, and not
rate-limited — the population is fully converged onto its supply curve. The
code's comment claims three significant figures; the measurement gives four.

**Second consequence.** The three venues' terminal marks agree to within
**0.0026%** despite deliberately heterogeneous matching rules and funding
intervals (RT-022). Cross-venue price-level dispersion here is not a market
outcome; all three books are pinned to the same configured reference.

**Third consequence, and the link to RT-031.** `elastic_supplier_symbols` is
`null`, so `makerSymbol(nil, i)` (`sim.go:1616`) places **all 8 suppliers on
ABC/USD**. `CDF/USD`, `ABC/CDF` and `ABC-PERP` receive none. That is the missing
ingredient behind RT-031: ABC/USD is pinned to four significant figures while
ABC/CDF, with no elastic demand and a maker referencing itself, drifts −69%. The
campaign has **one anchored book and the rest floating**.

**Owner decision, not mine to make.** Whether the campaign wants a peg (a known
correct price) or a demand curve (a belief revised toward what trades) is a
modelling choice. The code offers both and defaults to neither: the behaviour is
selected by a config field left at zero. What is not a choice is that
price-level conclusions drawn from the current configuration are reporting
`ElasticSupplierUnitsPerPercent` and `mvBootstrapPrice`, not participant
competition.

**Instrument note.** The comment at the construction site (`sim.go:3193`) says
the participant is *"seeded at the opening price and revised toward what it
observes, so the participant holds a private belief rather than a standing
instruction about the correct level."* With this config's half-life of zero, it
is not revised. The comment describes a configuration the campaign does not run.

## RT-033 — The competitive outcome is one class of six taking 75% of all gains

**Classification.** REAL. It is the answer to "is this a fair battle of actors",
and the answer is no.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`.
`clock-control-5h-101.json`, seed 607, 8h, `-log-mode none`.

**Instrument.** Raw terminal-minus-initial equity cannot rank this population:
every participant is net long ABC, so one mark move hits all of them at once and
the biggest endowment shows the biggest "result". `research/tools/classpnl`
removes the shared tide exactly —
`carry_adjusted_pnl = Δequity − Σ_asset initial_balance × Δmark` — and refuses to
print a ranking unless `Σ carry_adjusted_pnl + exchange_take` closes to within 1%
of gross flow. Measured closure: **0.8866%**, admissible but narrow.

**Result.**

| class | n | carry-adjusted | raw Δequity | per head |
|---|---:|---:|---:|---:|
| noise_flow | 18 | **−220 204 919** | −254 954 519 | −12 233 607 |
| spot_maker | 12 | −6 817 441 | −64 733 441 | −568 120 |
| future_flow | 9 | −5 993 965 | −23 368 765 | −665 996 |
| elastic_supplier | 24 | −2 026 313 | **−335 454 313** | −84 430 |
| latent_liquidity | 18 | −1 029 326 | **−632 232 326** | −57 185 |
| fixed_distance_maker | 24 | +11 483 857 | +84 725 457 | +478 494 |
| imbalance_maker | 24 | +27 556 949 | +100 798 549 | +1 148 206 |
| triangle_arb | 6 | **+174 217 981** | +192 528 381 | **+29 036 330** |

Total positive carry-adjusted result across all twenty classes is ≈+231 M.
**`triangle_arb` takes +174.2 M of it — 75.4% — with 6 participants out of 252**,
at 25× the per-head result of the best market-making class. The funder is
`noise_flow` at −220 M.

**Mechanism, joint with RT-031.** The cross book is self-referential and ends
−69% from its bootstrap. `triangle_arb` is the only class that trades the
triangle. `noise_target_qty_by_symbol` routes uninformed flow onto `CDF/USD` and
`ABC/CDF`. So the dominant competitive result in the campaign is one small class
harvesting a standing modelling dislocation from flow configured into it — not a
strategy outperforming a counterparty.

**Second result: two apparent findings that the instrument destroyed.** Ranked on
raw Δequity, this population's great losers are `latent_liquidity` (−632 M) and
`elastic_supplier` (−335 M). Carry-adjusted they are 22nd and 18th of the flow.
**99.4% of the suppliers' apparent loss is revaluation of an endowment they were
given**, not the cost of being RT-032's forced buyer. Anyone ranking these actors
on equity change is ranking endowments. RT-024 recorded this trap; this is the
first time it has been measured away rather than only warned about.

**Falsified en route.** H-045 predicted `elastic_supplier` would be the largest
donor, on the reasoning that RT-032's peg makes it a forced buyer. It is fifth,
by two orders of magnitude. Being a configured peg turns out to cost revaluation,
not trading result.

**Scope limit.** The closure residual is −4.95 M, so classes with |result| below
about ±5 M are not distinguishable from it: `carry_arb`, `elastic_supplier`,
`latent_liquidity`, `option_flow`, `metaorder_trader`, `round_trip`,
`dated_carry_arb`, `parity_arb`, `option_value_taker`, `cdf_spot_maker`,
`option_dealer`, `vanna_volga_desk`. Their ordering among themselves is not
exercised. The concentration result and the falsification both rest on classes
far outside that band.

**Instrument note.** The preregistration for H-045 was committed in the session
before the run but the edit writing it into the research note silently changed
nothing — the script used `'\\n'` where it needed `'\n'`, matched no anchor, and
reported success anyway. The hypothesis is therefore labelled POST-HOC in the
artifact rather than back-dated. Note edits now assert that the anchor matched
and that the file grew.

## RT-034/RT-035 — The campaign's competitive result is one mechanism, end to end

**Classification.** REAL. Resolves how RT-031 and RT-033 relate, and corrects
RT-033's mechanism sentence.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`.
`clock-control-5h-101.json`, seed 607, 8h, `-log-mode full`.
`research/tools/flowattrib` over 5 130 138 `OrderFill` records, 15 files, **zero
unpaired executions**.

**Instrument and its self-test.** Two `OrderFill` rows share a `trade_id`
(`exchange/settlement.go:526`), so the trade graph is recoverable: who traded
what, with whom, on which book. Contribution per book is
`Σ(signed quote cash flow, net of quote fees) + (net base inventory) × terminal mark`.
Before reporting any split the tool reconciles each class against a completely
different source — the account snapshots' carry-adjusted PnL of RT-033:

| class | from fills | from snapshots | gap |
|---|---:|---:|---:|
| triangle_arb | 174 203 545 | 174 217 981 | **−0.0%** |
| noise_flow | −219 422 999 | −220 204 919 | 0.4% |
| abc_cdf_spot_maker | 6 858 652 | 6 879 438 | −0.3% |
| futures_maker | 0 | 7 056 275 | −100.0% |
| perp_maker | 0 | 2 484 659 | −100.0% |

Spot-traded classes agree to a fraction of a percent from independent sources.
The 100% gaps are the tool working as designed: it deliberately folds in no
derivative book, so **the gap column reads off which classes earn nothing in a
spot order book** — `futures_maker` and `perp_maker` are entirely derivative-side.

**RT-034 — one book, one counterparty.** `triangle_arb`'s +174.2 M splits
`ABC/CDF` **170 904 176 (98.1%)**, `ABC/USD` 2 083 684, `CDF/USD` 1 215 685. On
`ABC/CDF` its counterparties are `abc_cdf_spot_maker` **157 661 074 (92.3%)**,
`imbalance_maker` 6 790 225, `fixed_distance_maker` 6 452 877. So RT-031 and
RT-033 are **one artifact, not two problems**.

**RT-035 — the chain closes.** `abc_cdf_spot_maker` on its only book:
`noise_flow` **+188 620 463** (354 838.7 ABC), itself 0 (17 200.7 ABC),
`imbalance_maker` −9 419 245, `fixed_distance_maker` −14 598 989, `triangle_arb`
**−157 743 578** (6 547.6 ABC), net **+6 858 652** — summing to the class total to
the unit.

    noise_flow  ──+188.6M──▶  abc_cdf_spot_maker  ──−157.7M──▶  triangle_arb

**The two rates are the verdict.** The maker charges uninformed flow
**531 USD/ABC, 1.08% of notional** — an ordinary market-making result — and pays
`triangle_arb` **24 092 USD/ABC, 48.9% of notional**, on **1.72% of its traded
volume**, which is **83.6% of its gross**. Forty-five times the rate. A market
maker loses a spread to adverse selection, not half of notional. This is a maker
quoting around a mid that RT-031 measured at −69% from fair while one class lifts
the wrong side of it.

**Correction to RT-033, published because it was wrong in a way that matters.**
RT-033 stated `triangle_arb` was "harvesting a self-referential book's 69%
dislocation **from uninformed flow configured into it**". `noise_flow` is **not a
direct counterparty of `triangle_arb` on any book**. The direct donor is the
self-anchored maker. The transfer is two-step, and the middle link is not a loser
— it finishes +6.88 M.

**Owner decision.** Nothing here is an exchange-engine defect: the accounting
reconciles from two independent sources to a fraction of a percent. What it shows
is that the population's competitive ranking is produced by a modelling choice —
a cross maker with `ReferenceSymbol` equal to its own symbol and no price-elastic
demand on its book — and not by strategy quality. Whether to give `ABC/CDF` an
implied-rate anchor or elastic demand is the owner's call; drawing
strategy-performance conclusions from the current configuration is not.

**Incidental, flagged not claimed.** `abc_cdf_spot_maker` trades 17 200.7 ABC
**against itself** — same-class makers on a venue crossing each other — at exactly
zero net class value, 2.6× `triangle_arb`'s entire volume on the book. It nets to
zero at class level so no result above is affected; whether it distorts
per-participant rankings inside the class or the book's volume statistics is not
exercised.

## RT-036/RT-037 — The scheduler grants permanent construction-order privilege

**Classification.** REAL, and a fairness defect rather than an accounting one. A
new mechanism family, unrelated to the RT-031→RT-035 cross-book artifact.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`.
`clock-control-5h-101.json`, seed 607, 8h.

**Mechanism, from inspection.** `simulation/scheduler.go:163` breaks
equal-timestamp ties by scheduler id, and the id is `es.nextID++` assigned at
**registration**. A repeating event is re-pushed with its time advanced and its
**id unchanged** (`scheduler.go:148-150`):

```go
} else if event.Repeating {
    event.Time += event.Interval
    heap.Push(&es.events, event)
}
```

So among actors sharing a tick interval, firing order is fixed at construction
and **never rotates for the whole run**. The tie-break is deliberate and correct
for determinism — the comment says so. The question is whether it also confers an
edge.

**RT-036 — the ordering effect is certain; for a taker it is worth 0.28%.**
`elastic_supplier` is the ideal test group: RT-032 established that all eight per
venue sit on one book with byte-identical configuration and endowment, differing
only in build order. Carry-adjusted PnL is **strictly monotone in registration
order on all three venues, 24 of 24 rows** (central −84 444 → −84 204, north
−84 777 → −84 509, south −84 464 → −84 220). A random permutation of eight sorts
with probability 1/8!; three venues agreeing is ≈**1.5 × 10⁻¹⁴**.

The **predicted direction was wrong**: acting first is *worse*, not better. These
are takers buying into a decline, and the first to act each tick commits at the
pre-trade mid while later participants recompute their target against a mid their
predecessors already moved.

**RT-037 — for a maker under price-time it is worth 5–11%, and under pro-rata
exactly nothing.** Pairing participants that share a book (`makerSymbol`
round-robin puts index *i* and *i+4* on the same book, controlling the confound):

| venue rule | pairs | result |
|---|---|---|
| `north`, price-time | 8 | **every pair differs; earlier wins 7 of 8**; +11.5%, +9.4%, +7.6%, +5.1% on the four material books |
| `central`/`south`, pro-rata | 16 | **typically zero to the unit**; largest 0.10% |

A 50–100× separation with the mechanism's exact fingerprint: pro-rata allocates
by size so arrival order buys nothing; price-time allocates by arrival so acting
first is queue position at the touch. **The null control was not built for the
occasion** — it is the campaign's own venue heterogeneity (RT-022) switching the
mechanism off.

**Consequence.** On a price-time venue a maker's result is **5–11% determined by
the order a `for` loop in `sim.go` constructed it**, not by its strategy. Two
byte-identical participants do not face a fair race, and the advantage never
rotates. Any comparison of maker strategies on a price-time venue in this
simulator carries this bias.

**Scope.** Eight price-time and sixteen pro-rata pairs, one seed. Sign is
consistent and the venue contrast is unambiguous; per-book effect sizes rest on
single pairs and would need multiple seeds to tighten. The qualitative claim is
carried by the exact-zero control.

**Does not affect RT-031→RT-035.** Those are aggregates dominated by `ABC/CDF`,
where the mispricing is 69% — orders of magnitude above a queue-position edge.

**Instrument note.** Preregistered falsifier (b) for H-047 said the result is
inconclusive if the per-participant spread is below the closure noise. It is —
19 600 USD per participant against a 240 USD spread — so by the letter of my own
falsifier RT-036 reads INCONCLUSIVE. That falsifier was the wrong statistic: the
closure residual is a systematic unmodelled-transfer term, and an additive bias
hitting all eight equally **cannot manufacture a strict monotone ordering**. The
ordering claim is scale-free and stands; the magnitude claim is reported as a
bound, not a value. Recorded because writing a level-uncertainty falsifier for an
ordering hypothesis is a design error worth not repeating.

## RT-038 — The entire latency model is inert at this step size

**Classification.** REAL, and it voids a whole class of the campaign's claims.
Discovered by an ablation that failed to ablate.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8h.

**How it surfaced.** A preregistered ablation gave `abc_cdf_spot_maker` the same
800 µs link as `triangle_arb` to test whether RT-035's 157.7 M transfer was a
latency race. The result was **bit-identical to baseline** — 170 904 176 on
`ABC/CDF`, 157 661 074 from the cross maker, 6 547.6 base. A 6.25× link change
cannot leave a run bit-identical, so the instrument was tested instead of the
result being reported.

**Measured**, each row a full run compared by md5 of `greeks.json`:

| change | magnitude | outcome |
|---|---|---|
| `abc_cdf_spot_maker` → 800 µs | 6.25× faster | **identical** (`ac3a46fd…`) |
| `abc_cdf_spot_maker` → 500 ms | 625× slower | **identical** |
| `noise_flow` → 1 µs | 20 000× faster | **identical** |
| `default_latency_profile` → 500 ms | hits 15 roles | **identical** |
| seed 607 → 608 (**positive control**) | — | every class moves |
| `default_latency_profile` → 3 s | 3× the `step` | **changes** (`b627d78f…`) |

The positive control rules out a broken measurement pipeline. Every configured
latency is invisible; a 3 s delay is not.

**Mechanism (inferred, not proven).** `"step": 1000000000` — a one-second
simulation step. The entire per-role table spans 500 µs to 20 ms, the slowest
capped at 500 ms. The measured boundary is in **(500 ms, 3 s]**, consistent with
sub-step delays being quantised away. The boundary was not bisected further, so
the step is the consistent explanation rather than a demonstrated one.

**What this voids.** Per-role latency heterogeneity has no effect on any outcome:
`triangle_arb`'s 800 µs "fastest link in the population", `noise_flow`'s 20 ms
lognormal tail, `fixed_distance_maker`'s 1% chance of a 50 ms spike, and
`spot_maker`'s 500 µs link versus the 5 ms default that `abc_cdf_spot_maker` and
`cdf_spot_maker` silently inherit. Any claim about latency arbitrage, link-based
information asymmetry, or fast-versus-slow participants is **not exercised** at
this step size. The validation at `sim.go:709` and `:821` that refuses to run
without "an explicit nonzero delayed link" enforces a field that changes nothing
— worse than no check, because it reads as assurance.

**Effect on earlier findings — it strengthens them.** RT-035's mechanism was that
`triangle_arb` extracts 48.9% of notional because the cross maker quotes around a
mid 69% from fair. The alternative was a latency race. If latency has no effect
at all, a latency race is **impossible** here, so the mispricing mechanism stands
on a stronger footing than the ablation would have given it.

**A pattern now known to be spurious.** Extraction from the cross maker orders
monotonically with configured link speed — `triangle_arb` (800 µs) 157.7 M,
`fixed_distance_maker` (1 ms) 14.6 M, `imbalance_maker` (2 ms) 9.4 M. It was
recorded as confounded when observed; it is now known to be **coincidence**,
since those links are inert. It must not be cited as evidence of a speed
advantage.

**Owner decision.** Either `step` drops far below the modelled latencies so the
link model bites, or the latency configuration is recognised as inactive at this
resolution. The current state — an elaborate, validated, per-role latency table
that provably changes nothing — is the one option that misleads.

**Instrument note.** The 3 s run's closure residual is 2.2582% of gross and
`classpnl` refused to print a ranking (exit 3), as its self-test is built to do.
Nothing is quoted from that run beyond the fact that it differs.

**UPDATE (E-054) — diagnosis upgraded from inferred to demonstrated.** A 2x2 was
run holding `step` fixed inside each contrast, so a difference is attributable to
latency alone:

| cell | step | default latency | wall | `greeks.json` |
|---|---|---|---:|---|
| A | 1 s | 5 ms | 1 s | `fd2541239f4a` |
| B | 1 s | **500 ms** | 1 s | `fd2541239f4a` |
| C | 1 ms | 5 ms | 1 m 59 s | `075d1737b1b6` |
| D | 1 ms | **500 ms** | 2 m 5 s | `945defb36c8d` |

**A = B byte for byte** — a hundredfold latency change at the campaign's step
does nothing, reproducing this finding at a 240x shorter horizon, so it is not
duration-dependent. **C != D** — the same change at a 1 ms step does alter the
run. Neither preregistered falsifier fired.

So the latency machinery — mounts, per-role profiles, per-client sample paths —
is **correct and functioning**, and merely invisible at a step 200x coarser than
the slowest configured delay. **The fix is a resolution choice, not a code
repair.**

The choice has a measured price: a 1 ms step costs **~120x more wall time** for
the same simulated span, which would take the campaign's 8 h run from ~4 minutes
to **~8 hours**. That explains why the coarse step was chosen; it does not change
the fact that every latency-based claim is currently unexercised.

**Owner options, stated neutrally.** (1) Keep the 1 s step and treat the per-role
table as inactive, removing the validation that currently reads as assurance.
(2) Drop the step to resolve the modelled delays and accept ~120x compute.
(3) Keep the coarse step for population-scale questions and run a separate
fine-step configuration for latency questions. Not my choice to make.

## RT-039 — The closure residual was my own tool, and the real accounting is exact

**Classification.** INSTRUMENT DEFECT in this audit's tooling, not in the
simulator. Corrects two scope limits published in RT-033 and RT-036, both in the
simulator's favour.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`, 8 h, seeds 607/608/609.

**How it surfaced.** Every `classpnl` run this session printed a residual of
−4 946 638 USD against a take of +4 944 035 USD — a ratio of 1.00053. I had been
reporting the magnitude ("0.89% of gross, admissible but narrow") and ignoring
the structure. A residual that equals the take suggests participants are charged
twice what the ledger records, which would be a serious defect, so it was
preregistered as H-050 with my own tool listed as candidate explanation (4) and
required to be excluded first.

**Result.** The seed sweep falsified the double-charge reading — the ratio is
1.00053, 1.00450, **0.81955**, an 18% spread outside the ±10% band — and the raw
per-asset ledger identified the cause:

| seed | `fee_revenue/USD` | `fee_revenue/CDF` | `fee_revenue/ABC` |
|---|---:|---:|---:|
| 607 | 494 403 495 651 | **153 469 520 553** | 89 805 |
| 608 | 463 937 957 961 | **155 245 978 261** | 73 893 |
| 609 | 502 372 867 994 | **154 672 166 782** | 93 574 |

**The venue takes fees in whichever asset the book quotes.** `ABC/CDF` fees accrue
in CDF — visible in every fill record as `"fee_asset": "CDF"`. `classpnl` summed
`FeeRevenue["USD"]` only, discarding ~1 534 CDF ~ 4.6 M USD per run, coincidentally
close to the USD take, which manufactured the near-perfect ratio at two seeds.

**Corrected closure** (same runs, same carry-adjusted numbers; take now valued
across all assets at terminal marks):

| seed | Σ carry-adjusted | take | residual | of gross |
|---|---:|---:|---:|---:|
| 607 | −9 890 673 | +9 891 169 | **+496** | **0.0001%** |
| 608 | −9 299 639 | +9 299 900 | **+261** | **0.0000%** |
| 609 | −9 140 911 | +9 144 241 | **+3 330** | **0.0005%** |

The population's entire trading loss equals the venue's entire take to **496 USD
out of ~558 M of gross flow**, across three assets and three venues. This is a
strong conservation result that I had been reporting as a weakness.

**Correction 1 — RT-033's resolution limit is retracted.** RT-033 stated that no
class below ~±5 M was distinguishable from the residual and listed **twelve of
twenty classes** as not exercised. With the residual at ~500 USD every class is
resolved, `elastic_supplier` and `latent_liquidity` included. The ranking is
unchanged, because carry-adjusted PnL never used the take; only the uncertainty
attached to it was inflated, by my own defect.

**Correction 2 — RT-036's mis-specified falsifier resolves in its favour.** That
falsifier compared a 240 USD spread against per-participant closure noise of
~19 600 USD. Corrected, that noise is ~**2 USD**, so the spread is two orders of
magnitude above it. The falsifier does not fire, and RT-036's magnitude claim is
supported rather than merely bounded.

**Blast radius.** `research/tools/populationclosure` has the same single-asset
take (`l.FeeRevenue[*asset]`); any residual it reported for a cross-asset run is
overstated by the discarded CDF fees. Flagged, not re-run. `flowattrib` never
used the take, which is why its independent cross-check reconciled to −0.0% and
gave no warning — the tool that agreed with reality was the one that did not
depend on the broken term.

**Why it survived four checkpoints.** The self-test was built to refuse a bad
decomposition and never fired, because the defect lived *inside* the reference
quantity the test compares against and left the residual just under the 1%
tolerance. A gate calibrated in one asset cannot detect a multi-asset omission.
The lesson is not to tighten the tolerance — at 1% the runs passed legitimately —
but that **a conservation check must enumerate every asset the system can move
value in, not the one the report is denominated in.**

## RT-040 — Marks are per venue, and collapsing them hid the last of the residual

**Classification.** INSTRUMENT DEFECT in this audit's tooling. Same shape as
RT-039, one level down. Strengthens the simulator's conservation result; changes
no published ranking.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, full logs,
5 130 138 `OrderFill` records.

**Why the gap had to be zero.** For a participant holding no derivative position,
equity is `Σ_asset net_asset × mark`, and terminal net asset is the initial
balance plus the fill-derived delta. So carry-adjusted PnL reduces to
`Σ fill_delta_asset × mark_final`, which is exactly what `flowattrib` computes
from the fill stream. For a spot-only class the two must be identical. They were
not: `abc_cdf_spot_maker` −20 786, `cdf_spot_maker` +3 680, `elastic_supplier`
−2 285.

**Cause.** Both tools built `endMarks[asset] = mark` while iterating terminal
rows — **last write wins**. Marks are published **per venue**, and the venues do
not agree: RT-032 measured the terminal ABC mark at 4 929 505 000 on `central`,
4 929 380 000 on `north`, 4 929 375 000 on `south`. One venue's prices were being
applied to another venue's inventory.

**After keying marks by `(venue, asset)`:**

    Σ carry-adjusted pnl   −9 890 673 USD
    exchange take          +9 890 673 USD
    residual                        −0 USD   (0.0000% of gross)

| class | gap before | gap after |
|---|---:|---:|
| abc_cdf_spot_maker | −20 786 | **+1** |
| cdf_spot_maker | +3 680 | **+1** |
| elastic_supplier | −2 285 | **+1** |
| latent_liquidity | −677 | **0** |
| metaorder_trader | ~0 | **0** |
| round_trip | ~0 | **0** |
| triangle_arb | −14 436 | **−3** |

Every spot-only class within **3 USD**. There is **no unexplained non-fill
accounting term** for spot participants. `noise_flow` (0.3%) and the derivative
classes (±100%) are unchanged in meaning — they trade books this tool does not
fold in.

**No published ranking moves.** `classpnl`'s per-class carry-adjusted figures are
bit-identical to RT-033's, because that path always used `row.Marks`, the
participant's own row. Fill-attribution figures move ~0.01% —
`triangle_arb`'s `ABC/CDF` contribution 170 904 176 → 170 923 896, its extraction
from the cross maker 157 661 074 → 157 679 289 — leaving **RT-034's 98.1% and
92.3% unchanged**. Findings annotated, not reissued.

**Net effect: a real conservation result for the simulator.** Two computations
from different sources — a 5.13 M-record fill stream and 252 account snapshots —
agree to 1 USD, and the population's trading loss equals the venue's take to the
unit.

**The recurring shape, stated so it stops recurring.** RT-039 was "a value can
live in an **asset** the tool does not enumerate". This is "a value can live in a
**venue** the tool does not enumerate". Both are the same error — collapsing a
dimension the system actually varies over — and both hid inside a quantity that a
self-test compares against, which is why no gate fired. Rule for the remaining
work: **before trusting a reconciliation, enumerate every dimension the system
prices along and confirm the instrument keys on all of them.** Asset and venue
are now covered. Time is not, and a mark is a point-in-time quantity.

## RT-041 — The concentration result reproduces on three independent seeds

**Classification.** REPRODUCTION. Promotes RT-031, RT-034 and RT-035 from
single-seed to reproduced, and replaces their point figures with ranges.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`, 8 h, `-log-mode full`,
seeds 607/608/609.

**Thresholds were preregistered before either reproduction run**, so this is a
test rather than a description.

| seed | `ABC/CDF` share | maker share | USD per ABC | % of notional | base traded | cross book last |
|---|---:|---:|---:|---:|---:|---:|
| 607 | 98.11% | 92.25% | 24 082 | 48.85% | 6 547.6 | −69.15% |
| 608 | 99.14% | 92.48% | 25 929 | 52.49% | 6 531.0 | −82.86% |
| 609 | 99.63% | 92.96% | 28 104 | 57.12% | 6 766.4 | −83.15% |
| **threshold** | **>=90%** | **>=80%** | — | **>=20%** | — | **<=-30%** |

All four pass on every seed; no falsifier fires.

**The reproduction is stronger evidence than the original measurement.** The
levels move a lot between seeds — `triangle_arb`'s total is 174.2 M / 184.7 M /
205.3 M, and `abc_cdf_spot_maker`'s **net** result swings **5.4x**, from +6.88 M
at seed 607 to +36.92 M at 608. Against that, the structure barely moves: book
share spans 1.5 pp, counterparty share **0.71 pp**, and the base traded against
the maker spans **3.6%** (6 531–6 766 ABC).

That near-constant traded quantity is the signature of the position caps
(RT-028): the arbitrageur trades about the same amount in every run, and the seed
sets only what each unit is worth. A structure this stable across seeds whose
levels swing five-fold is not an artifact of one run.

**Quote as ranges from now on**: book share 98–99.6%, counterparty share
92.3–93.0%, extraction 49–57% of notional, cross-book terminal deviation −69% to
−83%.

**POST-HOC, not preregistered and not claimed.** The extraction rate rises with
the depth of the dislocation across the three seeds (−69.15%/48.85%,
−82.86%/52.49%, −83.15%/57.12%). The ordering is monotone and in the direction
the mechanism predicts, but three points with two nearly tied on the independent
variable is not a quantitative law. Consistent with RT-035's causal story, not
confirmation of it.

**Scope.** Three seeds of one configuration. Nothing here speaks to other
configurations, horizons, or scenarios, and the dislocation's depth is plainly
seed-sensitive. The promotion is to "structural within this configuration", not
"universal".

**Method note against myself.** This belonged before RT-034 was written, not four
checkpoints after. The protocol requires a fresh run before promoting a result,
and three findings' worth of percentages were published from a single seed first.
The reproduction happened to support them, which is luck rather than process.

## RT-042 — Funding is not interval-scaled, and one venue's perp never funds

**Classification.** REAL. First finding on the derivative side, which this
audit's instruments had not touched.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, full logs.

**Mechanism.** `SimpleFundingCalc.Calculate(indexPrice, markPrice)`
(`instrument/funding.go:20`) takes **no interval argument** — the rate is
`BaseRate + Damping x premium`, clamped at `±MaxRate`, from the mark/index
premium alone. Settlement applies it directly (`exchange/funding.go:753`):

```go
funding, ok := etypes.TryMulDiv(positionValue, fundingRate.Rate, 10000)
```

`fundingRate.Interval` appears only in `nextFundingTimestamp`, which schedules the
next settlement. It never scales the amount, so a full interval's rate is charged
however often it is asked.

**Measured.**

| venue | interval | settlements | mean \|rate\| | cumulative \|rate\| | ABC-PERP fills |
|---|---:|---:|---:|---:|---:|
| central | 3 600 s | 7 | 38.57 bps | **270 bps** | 61 694 |
| south | 7 200 s | 3 | 28.00 bps | **84 bps** | 60 210 |
| north | 28 800 s | **0** | — | **0 bps** | 59 572 |

Logged `interval` values are 3600 and 7200, matching `venue_rules` exactly, so
the scheduler is correct. Mean per-settlement rates are comparable (within 1.4x)
while cumulative funding differs 270 : 84 : 0. **An identical perp position bears
a 2.7% funding drag on `central` and none at all on `north`**, decided by a
scheduling parameter rather than a market condition.

**The larger, unpredicted result: `north` has no funding mechanism.** Its first
settlement would land at t = 8 h, the horizon itself, and never fires — while the
book trades 59 572 fills, comparable to the other venues. The campaign therefore
runs a "perpetual" on one venue with **no device tethering its mark to its
index**. That is the same shape as RT-031's self-referential cross book, reached
by a completely different route.

**Consequence.** Cross-venue perp comparisons are invalid in this campaign: they
compare a funded instrument against an unfunded one. Whether funding *should* be
time-scaled is the owner's modelling choice, and `BaseRate`/`MaxRate` are
per-venue configurable so a compensating configuration exists. The current one
does not compensate.

**Scope.** One seed, one configuration, one horizon. `north`'s zero-settlement
result is a horizon boundary effect and would change at a longer run — at 16 h it
settles once. That does not soften it for this campaign, whose every published
run is 8 h.

**Instrument note against myself.** The preregistration predicted settlement
counts of 8 / 4 / 1; observed is 7 / 3 / 0, because settlements land at strict
interval multiples inside the horizon. Falsifier (b) fired on that mismatch and
said it would mean "the interval is not doing what the config says". That
inference is wrong on the evidence — the logged intervals match the config — so
what failed was my boundary arithmetic in an auxiliary prediction that was never
the claim. Cumulative rate was the claim and it is supported. This is the third
preregistration-design error in the campaign, after a level-uncertainty falsifier
written for an ordering hypothesis (RT-036) and an ablation that could not
ablate (RT-038).

## RT-043 — The funding controller saturates and latches at its cap

**Classification.** REAL. Second finding on the derivative side. Also confirms
RT-042's boundary explanation by direct test.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, **12 h**, full
logs.

**Mechanism.** `sim.go:2757` builds the calculator as
`&instrument.SimpleFundingCalc{BaseRate: 1, Damping: 100, MaxRate: 75}`.
`Damping` enters as `premium x Damping / 100`, so **100 is a multiplier of 1.0 —
no damping at all**. The rate is the raw mark-to-index premium in bps plus 1 bp,
shaped only by a hard clamp at ±75.

**Measured.**

| venue | settlements | at ±75 cap | mean \|rate\| | settled rates (bps, in order) |
|---|---:|---:|---:|---|
| central | 11 (h=1…11) | **7 (64%)** | 51.8 | −14, −1, −75, −9, −21, **−75, −75, −75, −75, −75, −75** |
| south | 5 (h=2,4,6,8,10) | **3 (60%)** | 46.8 | 1, −8, **−75, −75, −75** |
| north | 1 (h=8) | **1 (100%)** | 75.0 | −75 |

Against a preregistered threshold of 25% at the cap, the measurement is 64%, 60%
and 100%.

**The sequence matters more than the fraction.** On `central` the **last six
consecutive settlements are all pinned at −75**, on `south` the last three. From
hour 6 the funding rate is a constant and the controller has stopped responding
to the premium. It does not clip occasionally; it **latches at its limit and
never returns**.

**How far past the limit.** At h=11 `central`'s perp mark is 4 769 999 250 against
an ABC/USD mark of 4 911 390 000 — a basis of **−2.88%, or 3.8x the ±0.75% cap**.
Funding has under a third of the authority needed to close the gap it exists to
close. (Caveat: perp mark read at h=11, spot mark at the h=12 snapshot; spot
drifts ~0.2%/h, so ~2.7% survives the timing mismatch.)

**RT-042's boundary explanation is confirmed by direct test.** At a 12 h horizon
`north` settles **exactly once, at h=8.0**, `central` 11 times at h=1…11, and
`south` 5 times at h=2,4,6,8,10 — every count and every timestamp as predicted
from the strict interval-multiple rule. The count arithmetic that failed in
RT-042's preregistration is now correct and verified against logged timestamps
rather than inferred.

**Economic consequence.** The perpetual's tether detaches once the basis exceeds
0.75% and never reattaches. `carry_arb`, `funding_carry_arb` and `dated_carry_arb`
all trade a funding signal that is a constant for half the run.

**Third anchorless instrument, third route.** The cross book quotes around itself
(RT-031); `north`'s perp never funds (RT-042); every venue's perp detaches past
0.75% (this). Meanwhile ABC/USD is held rigid by a configured peg (RT-032). The
campaign's price system is one pegged book with a set of instruments floating
away from it.

**Scope.** One seed, one configuration, 12 h. Saturation fraction and basis
magnitude are single-seed. The direction — persistent discount, latched cap —
held on all three venues in this run; those share no order flow but are not
independent samples.

**Owner decision.** `funding_max_rate_bps` and `Damping` are configurable. Whether
a 75 bps cap with no damping is the intended model is not mine to decide; that it
does not restrain a −2.88% basis is measurement.

## RT-044 — The perp mark is pinned on its clamp while the book is 29% away

**Classification.** REAL, and the most serious finding of the derivative
sequence: it is a solvency question, not a pricing preference. Corrects RT-043's
magnitude.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, full logs.

**Mechanism.** `exchange.go:1527` auto-installs `ClampedEMAMarkPrice` for every
margin instrument with an index, and `exchange.go:1514` defaults the band to 600
bps, which the config does not override. So

    mark = index + clamp(EMA(perp_mid - index), +/- index * 3%)

**Measured** (`research/tools/perpbasis`, from `BookSnapshot` evidence — an
independent source from the funding evidence RT-043 used):

| venue | samples | mean | worst | beyond ±3% clamp | final-quarter mean |
|---|---:|---:|---:|---:|---:|
| central | 28 539 | −4.902% | **−28.850%** | 9 264 (**32.5%**) | **−17.193%** |
| north | 28 483 | −4.825% | −28.756% | 9 154 (32.1%) | −17.049% |
| south | 28 549 | −4.921% | −28.898% | 9 280 (32.5%) | −17.230% |

Three venues agree to a tenth of a percent: systematic, not one venue's accident.

**The clamp binds exactly, not approximately.** At h=8 the settled mark is
**4 781 619 850**; the spot mid at that instant is **4 929 505 000**, and 97% of
that is **4 781 619 850** — identical to the unit. The mark is the clamp.

**The book is far outside it.** Terminal snapshot: perp bid **3 507 080 000**,
ask **3 507 380 000**, against a spot mid of **4 929 505 000** — a basis of
**−28.85%** while the mark reports −3%.

**Consequence.** Margin and liquidation consume the mark. At the terminal state a
long is marked at 4 781 619 850 while the best bid is 3 507 080 000: the position
is valued **26.7% above what it could realise**, and across the final quarter the
gap averages about 14 points. **A liquidation engine reading this mark does not
fire when it should**, and every margin figure in the campaign's perp accounting
is optimistic by that amount.

**Corrects RT-043.** That finding reported the perp basis as −2.88% and called it
3.8x the funding cap. That was the **clamped mark's** basis — a configuration
constant. The book's basis reaches −28.85%, **38x** the 0.75% cap. RT-043's
saturation mechanism is unchanged and stands; its number described the limiter
rather than the market.

**Two nested limiters, neither of which reports saturation.** The mark clamp holds
the reported price 3% from index while the book travels to 29%; the funding cap
then acts on that already-clamped premium and latches at 0.75%. The only way to
see either is to read the raw book.

**Instrument note.** The first version of `perpbasis` reported "no timestamps with
both books two-sided" for all three venues — a clean, confident, empty result.
The cause was a schema assumption: per-book spot files write levels at the top of
the payload and name the book **by file path**, while the shared
`derivatives.jsonl` nests them under a symbol. Same trap recorded earlier for
`Trade` payloads. The tool now handles both shapes explicitly.

**Scope.** One seed, one configuration, 8 h. The three venues share an index and a
population, so they are not independent replicates.

## RT-045 — Retracts RT-044's liquidation claim; the clamp redistributes instead

**Classification.** RETRACTION plus a larger replacement finding. The measured
facts of RT-044 stand; the consequence I inferred from them does not.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, full logs.

**The retraction.** RT-044 asserted that "a liquidation engine reading this mark
does not fire when it should" and that "every margin figure is optimistic". Tested
directly by revaluing every terminal perp position at the book midpoint:

    solvent at mark, insolvent at book: 0 accounts

and the run logs **0 liquidation events** in total, against 51 accounts holding
`ABC-PERP` and 4 363.03 contracts long versus 4 363.03 short. **No account is
hidden-insolvent.** The perp participants are heavily over-collateralised —
`carry_arb` holds 100 M USD of perp collateral against 3 000 contracts whose book
loss is about 38 M. The engine is not failing to fire; there is nothing to fire
on. RT-044's measurements (mark pinned exactly on the clamp, book 17–29% away)
are unaffected.

**What the test found instead.** Revaluing perp positions at the book midpoint
moves reported value between classes:

| class | net contracts | reported above book |
|---|---:|---:|
| carry_arb | +3 000.00 | **+38 206 022** |
| fixed_distance_maker | +591.47 | +7 532 466 |
| imbalance_maker | +208.62 | +2 657 346 |
| perp_maker | +124.67 | +1 602 173 |
| noise_flow | −249.12 | −3 179 918 |
| spot_maker | −3 675.64 | **−46 818 089** |

The column sums to zero to the unit — a pure transfer, which is the internal
check that the arithmetic is sound.

**This inverts two of RT-033's published rankings**, which were computed at marks:

| class | at mark | at book | rank change |
|---|---:|---:|---|
| spot_maker | −6 817 441 | **+40 000 648** | 2nd-largest donor → 2nd-largest winner |
| carry_arb | −2 423 266 | **−40 629 288** | 4th donor → 2nd-largest donor |

A sign flip and a 16x change. The concentration result is untouched:
`triangle_arb` holds no perp position, so its +174 M stands either way.

**Owner decision, not mine.** The mark is the exchange's official valuation and
the one margin consumes; the book midpoint is closer to realisable value, though
closing 3 675 contracts would move a book that thin. Neither is unambiguously
correct. What is not ambiguous is that they diverge by ~50 M **because the clamp
is binding**, and that a performance ranking which does not state its valuation
basis is under-specified.

**Scope.** One seed, terminal snapshot only. The revaluation uses one midpoint per
venue and ignores the depth that closing these positions would consume, so it is
an upper bound on realisable value rather than an estimate of it.

**Process note.** This is the fourth asserted consequence in this campaign to fail
its own test, after the latency-ordering pattern (RT-038), the funding count
prediction (RT-042) and the level-uncertainty falsifier (RT-036). The pattern is
consistent: measurements survive, and the sentences I write *around* them are
where the errors are. Consequences now get tested before they are published, not
after.

## RT-046 — The carry arbitrageurs cap out, but only in the second half

**Classification.** MIXED. Confirms one measurement exactly and falsifies the
unifying framing I proposed around it.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, full logs.

**Measured** (`research/tools/positionpath`, reconstructed from fill evidence):

| venue | role | fills | terminal | first at limit | time at limit |
|---|---|---:|---:|---:|---:|
| central | carry_arb_1 | 1 529 | **500.00** | 4.27 h | 40.8% |
| north | carry_arb_1 | 1 804 | **500.00** | 5.38 h | 32.7% |
| south | carry_arb_1 | 1 399 | **500.00** | 3.82 h | 43.6% |
| central | carry_arb_2 | 1 421 | **500.00** | 4.95 h | 38.1% |
| north | carry_arb_2 | 1 673 | **500.00** | 5.52 h | 31.0% |
| south | carry_arb_2 | 1 311 | **500.00** | 4.61 h | 42.4% |

**Confirmed.** Every participant ends at exactly +500.00, the configured
`carry_max_position`. RT-045's class sum of +3 000.00 was not hiding dispersion,
and this reconstructs it from the fill stream rather than the account snapshots.

**Falsified.** I predicted ≥80% of the run at the cap. Measured 31.0%–43.6%, every
participant under the 50% falsifier threshold, first reaching the limit only at
3.82–5.52 h into an 8 h run while trading 1 300–1 800 times each.

**The framing this kills.** I proposed that the perp subsystem is "saturated at
every layer: mark at its clamp, funding at its cap, arb at its limit". The first
two are measured and stand (RT-043, RT-044). The third does not. **The carry
arbitrageur trades actively through the first half and pins only in the second**,
as the basis blows out past what it can absorb. Saturation here is **progressive,
not initial**, and the arbitrageur is the layer that keeps responding longest.

**This also qualifies RT-045.** Its exposure table is a terminal snapshot showing
every carry arb at its cap, which reads as a standing state; the population spent
about a third of the run there. Accurate and unrepresentative at once.

**POST-HOC, noted not claimed.** Time-at-limit orders by venue — south earliest
and longest, north latest and shortest, central between, with both participants
agreeing inside each venue. Six points over three venues cannot support a venue
effect, and it is confounded with RT-042's funding-interval heterogeneity.

**Instrument note: the third occurrence of one trap, and the first caught by a
cross-check.** The first version of this tool reported **terminal 0.00 for all six
while counting 1 529 fills each**. Derivative `OrderFill` records nest the fill
fields under `payload.payload`, keeping only the symbol at the outer level, so the
tool matched the symbol, counted the record, and read `qty` as zero. The result
was internally consistent and entirely plausible — "the carry arbs end flat" is a
reasonable finding — and the **only** reason it was caught is that RT-045 had
already measured +3 000.00 from a different source. The same nesting trap has now
appeared in `Trade`, `BookSnapshot` and `OrderFill` payloads. `flowattrib` is
unaffected: it reads the outer symbol, finds no `/`, and skips derivatives.

The lesson is not "handle the schema". It is that **a tool returning a plausible
wrong answer is invisible without an independent measurement to contradict it**,
which is what the two-source discipline from RT-040 bought.

## RT-047 — The spot makers' hedge stopped hedging

**Classification.** REAL. Explains the largest unexplained structural fact in the
population and traces a second of the three biggest results to a lost anchor.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, full logs.

**Configuration.** `maker_hedge_symbol: "ABC-PERP"`, band 1.00 contract, 60 s
interval: the ABC/USD makers offset spot inventory in the perpetual
(`stoikov.go:1502`).

**The hedge is executed almost perfectly.** Per participant, spot inventory
against perp position (12 participants, class totals shown):

| | `ABC/USD` inventory | `ABC-PERP` position | residual |
|---|---:|---:|---:|
| worst participant | | | **2.90 contracts** |
| **class** | **+3 673.77** | **−3 675.65** | **−1.88** |

Every participant is delta-flat to within 2.90 contracts on positions of 130–640,
and the class nets to −1.88 out of 3 674 — inside the configured 1.00-contract
band. It also reproduces RT-045's −3 675.64 from the fill stream rather than the
account snapshots.

**Self-test.** On the perp leg, 7 803 fills carried an exchange-reported post-fill
position and **0 disagreed** with accumulation. Spot fills carry none, and the
tool reports "accumulation is unchecked" rather than implying a verification it
did not perform.

**The hedge neutralises quantity and not value.**

| leg | value |
|---|---:|
| inventory exposure being hedged (3 673.77 ABC x −704.95 USD) | **−2.59 M** |
| spot leg result from fills | −0.88 M |
| perp leg at mark | −5.94 M |
| **perp leg at book** | **+40.88 M** |

The perp leg at book is **15.8x** the exposure the hedge was written to
neutralise and **46x** the spot leg's own result.

**Mechanism: two anchoring failures colliding.** A hedge works only if the legs
move together. `ABC/USD` is held rigid by a configured peg at −1.4% (RT-032);
`ABC-PERP`'s book falls to −29% (RT-044). The maker is long a book that cannot
move and short a book with nothing holding it. Delta-neutral in contracts, wildly
directional in value.

**Consequence for the fairness reading.** RT-045 showed `spot_maker` moving from
second-largest donor at marks (−6.8 M) to second-largest winner at book (+40.0 M).
This identifies the whole of that swing as one configured hedge into a dislocated
instrument — not market-making skill and not a strategy outcompeting anyone. It is
the same shape as `triangle_arb`'s +174 M (RT-035) by a different route.
**Two of the population's three largest results now trace to instruments that lost
their anchors.**

**Scope.** One seed, one configuration. The perp-leg-at-book figure is assembled
from three tools measuring the same run — `flowattrib` for the spot leg,
`classpnl` for the class total, `marksolvency` for the mark-to-book gap — so it
inherits each of their limits, and the book valuation ignores the depth that
closing 3 675 contracts would consume.

## RT-048 — The noise traders' loss is not a spread payment either

**Classification.** REAL, and it retracts a reading I published one checkpoint
ago. Completes the attribution of the population's three largest results.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, full logs.

**Measured**, loss normalised by fair-value notional (base traded x the asset's
terminal USD mark), the same basis on every book:

| book | loss | base traded | notional | rate | share of loss |
|---|---:|---:|---:|---:|---:|
| ABC/USD | −947 040 | 35 464.9 | 1 748 244 019 | **5.4 bps** | 0.4% |
| CDF/USD | −2 571 725 | 794 346.1 | 2 558 985 961 | **10.0 bps** | 1.2% |
| **ABC/CDF** | **−215 945 892** | 377 269.2 | 18 597 504 077 | **116.1 bps** | **98.4%** |
| total | −219 464 657 | | | | |

**21.4x** the pegged book's rate and **11.6x** the CDF book's, against a
preregistered threshold of 3x. No falsifier fired.

**The retraction.** After RT-047 I wrote that `noise_flow`'s −220 M is
"uninformed flow paying spread — the only one of the three largest results that
looks like a market outcome". **98.4% of the loss falls on one book**, at 21x
what the same participants pay on `ABC/USD`. Uninformed flow on the pegged book
pays 5.4 bps, which is an ordinary spread. The same actors on the cross book pay
116 bps. That is not the venue's quoting cost.

**RT-035's 1.08% is re-read, not corrected.** That figure was computed on the
cross book's own transaction prices and I called it "an ordinary market-making
result". On a fair-value basis it is 116 bps against 5.4 bps for the identical
strategy one book over. The number stands; calling it ordinary required a
comparison I had not made.

**The leaderboard has one cause.**

| result | magnitude | attribution |
|---|---:|---|
| `triangle_arb` | +174 M | 98.1% on `ABC/CDF` (RT-034) |
| `noise_flow` | −220 M | 98.4% on `ABC/CDF` (this) |
| `spot_maker` | +40 M at book | hedge into the unanchored perp (RT-047) |

The campaign's largest gain and largest loss are **the same book**, with
`abc_cdf_spot_maker` netting +6.9 M while passing 193 M between them.

**What this does not establish.** A high rate on `ABC/CDF` does not by itself
prove the dislocation causes it; that book also has a different tick, depth and
maker. The measurement bounds how much of the −220 M reads as ordinary spread —
**at the `ABC/USD` rate the cross-book flow would have cost 10.0 M rather than
215.9 M** — and leaves 205.9 M attributed to something specific to that book
without apportioning it among the candidates.

**Scope.** One seed, one configuration. Notional uses terminal marks for the whole
run, so these are averages against an end-of-run valuation rather than trade-time
rates.

## RT-049 — The cross book's cost is neither its spread nor inventory drift

**Classification.** ELIMINATION. Both ordinary components of a taker's result are
excluded; the residual is named but explicitly not claimed.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, full logs.

**Quoted half-spreads, top of book** (`research/tools/bookspread`):

| book | samples | median | mean | p90 | max | `noise_flow` loss rate |
|---|---:|---:|---:|---:|---:|---:|
| ABC/USD | 86 410 | **0.35 bp** | 0.34 | 0.39 | 2.56 | 5.4 bps |
| ABC-PERP | 85 595 | 0.41 bp | 0.41 | 0.42 | 3.30 | — |
| **ABC/CDF** | 77 814 | **1.80 bp** | 2.10 | 3.88 | 8.78 | **116.1 bps** |
| CDF/USD | 83 573 | **4.99 bp** | 4.48 | 5.00 | 6.67 | 10.0 bps |

The cross book's loss rate is **64x its own quoted half-spread**, which bounds the
spread component at about **1.8%** of it. The preregistered falsifier required a
half-spread of 100 bps or more to explain the loss; the measurement is 1.80 bp.

**Spread and cost are not even ordered together.** `CDF/USD` carries the widest
spread in the population and the second-lowest loss rate; `ABC/CDF` has under half
that spread and 11.6x the loss rate. Whatever the cross book costs its takers, it
is not what the book charges to cross it.

**The inventory story is ruled out by sign.** The prediction was that
`noise_flow` ends net long, so that a collapsing book would explain the loss as
revaluation. Measured: **net −13 224.08 contracts, with 14 of 18 participants
short**. A short in a book that fell 69% *gains*. The mechanism does not merely
fail to explain the loss, it points the other way.

**Status: inconclusive by design.** Falsifier (c) specified that if neither
component dominates, the decomposition is reported inconclusive rather than split
by assumption. Both components are eliminated, which narrows the search without
answering it.

**What remains, and how to settle it.** With spread and inventory drift excluded,
the residual candidate is that the loss is realised **at the moment of trade** —
ABC and CDF exchanged at a rate far from the two assets' USD values, on a book
measured at −69% to −83% from its implied rate. The decisive measurement is a
volume-weighted execution price: CDF received per ABC sold, converted at the CDF
mark, against ABC's own USD mark. **That is not claimed here.** It would be the
sixth mechanism sentence this campaign published without measuring it, and the
previous five were wrong.

**Scope.** One seed, one configuration. Half-spreads are top-of-book and ignore
depth, so a taker sweeping levels pays more than quoted — that widens the spread
component but nowhere near the factor of 64 required. Spot positions are
accumulated from fills with no exchange-reported cross-check available, which the
tool reports.

## RT-050 — The cross book's cost is realised at execution, and the chain closes

**Classification.** REAL, and it completes the campaign's central causal chain.
Every link is now measured rather than inferred.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, full logs.
216 529 fills matched, 0 skipped for a missing fair rate.

**Measured** (`research/tools/crossexec`; fair rate rebuilt as the venue's own
consensus, median `ABC/USD` mid over median `CDF/USD` mid, at or before each
fill):

| | volume | VWAP (CDF/ABC) | fair at execution | gap |
|---|---:|---:|---:|---:|
| bought | 182 022.51 | 7.9059 | 16.2996 | **−51.50%** |
| sold | 195 246.58 | 8.0708 | 16.3087 | **−50.51%** |
| net flow | **−13 224.07** | | | |

The noise traders transact ABC at roughly **half its fair value in CDF, in both
directions**, over 216 529 fills. They buy cheap and sell cheap; because they sell
13 224 more than they buy, the net is a large loss.

**Preregistered cross-check passes.**

    implied loss from execution away from fair   −246 188 281 USD
    measured loss (independent contribution calc) −215 945 892 USD
    ratio                                              114.0%

Inside the ±25% band; falsifiers needed <50%, >200%, or VWAP ~ fair, and none
fire. The two numbers come from different arithmetic on the same evidence — one
accumulates per-fill deviation from a contemporaneous rate, the other sums cash
flows and values terminal inventory at terminal marks. The residual 14% is
accounted for by the 5 bp taker fee, the snapshot-to-fill timing gap, and exactly
that difference in terminal valuation.

**An unplanned second cross-check landed exactly.** `crossexec` computes net base
flow −13 224.07 from fills; `positionpath`, written separately, reported
−13 224.08. Agreement to 0.01 contracts.

**The chain, end to end, every link measured:**

| link | finding |
|---|---|
| the cross maker quotes around its own mid | RT-031 (`ReferenceSymbol` = its own symbol) |
| so the book leaves fair value | RT-041 (−69% to −83%, three seeds) |
| uninformed flow is routed onto it | config `noise_target_qty_by_symbol` |
| and loses 116 bps, 21x the pegged book | RT-048 (98.4% of −219.5 M) |
| not from spread — 1.80 bp | RT-049 (loss is 64x the half-spread) |
| not from inventory — net short in a falling book | RT-049 (wrong sign) |
| but at execution, ~51% below fair | **RT-050** (114% of measured) |

**One line of configuration — a maker whose reference is its own book — produces a
−246 M transfer across 216 529 trades.** It is the campaign's largest loss and it
is now fully attributable.

**Scope.** One seed, one configuration. The fair rate inherits the 1 s snapshot
cadence, so a fill may be compared against a rate up to a second stale. Depth is
not modelled separately; VWAP uses executed prices, which already reflect whatever
depth was consumed.

## RT-051 — Every out-of-the-money option loses its bid; every in-the-money one does not

**Classification.** REAL, and the first finding on the option surface, which no
instrument in this audit had examined. Mechanism deliberately not claimed.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, full logs.
50 option books, terminal spot 49 295.05.

**Measured** (`research/tools/bookspread`):

| group | n | two-sided (min) | two-sided (median) | half-spread (median) |
|---|---:|---:|---:|---:|
| **in the money** | 25 | **99.9%** | 99.9% | 10.0% |
| **out of the money** | 25 | **21.4%** | **58.3%** | **76.8%** |

**The separation is total.** The worst ITM book is 99.9% two-sided; the best OTM
book is 98.8%. There is no overlap — a partition, not a tendency.

**The missing side is always the bid.** In all 12 books that are two-sided less
than half the time, no-bid dominates. Across the surface, ask-only snapshots (no
bid) reach **16 962** while bid-only (no ask) never exceeds **18**. **A holder of
an out-of-the-money option cannot sell it** for 21% to 79% of the run.

**Not a deep-OTM artifact.** All 12 worst books lie within **5.5% of spot**. The
most extreme near-the-money case is `49000-P`, **0.6% from spot**, two-sided only
**22.1%** of the time. The natural innocent explanation — worthless options
attract no bid — was the preregistered falsifier most likely to fire, and it does
not.

**Mechanism not determined, and the first guess was checked and refuted.** The
obvious explanation is that OTM bids round below one tick and are never placed.
**False.** Sampling `49000-P`: bid **1 600 000**, ask **31 400 000** — a **16 USD
bid against a 314 USD ask** on the same contract, a bid at 5% of the ask rather
than an absent one. The healthy `49000-C` quotes 585 against 879. OTM quotes are
extremely skewed toward the ask and the bid vanishes intermittently; why is
unmeasured and stays unclaimed.

**Consequence for the fairness question.** RT-045 established that
`option_dealer`, `option_flow`, `option_value_taker` and `vanna_volga_desk` earn
their entire results in these books. Half the surface has no bid for much of the
run, and where a bid exists the median half-spread is 76.8%. Any result attributed
to option strategy skill was obtained in a market where one side of half the
instruments is frequently absent.

**Next experiment.** Identify who supplies each side of an option book, from
maker/taker roles on fills, and whether the dealer's quoting is skewed by
construction or its bid is withdrawn by a risk limit. That separates a
quoting-policy artifact from an inventory constraint, in one pass over evidence
already collected.

**Scope.** One seed, one configuration. Moneyness uses terminal spot, so a book
classified OTM may have been ITM earlier; that softens the partition at the 49 000
strike and cannot explain it, since `49000-C` (ITM by 0.6%) is 99.9% two-sided
while `49000-P` (OTM by 0.6%) is 22.1%.

## RT-052 — The dealer quotes ITM books symmetrically and OTM books 4.45:1 to the ask

**Classification.** REAL for the asymmetry; the *mechanism* remains open, with one
candidate eliminated and a new anomaly identified.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, full logs.

**Method.** Attribution from `OrderAccepted` — what was **placed** — not from
fills. Attributing quote provision from fills is circular: a book with no bid has
no bid-side fills by construction.

| group | bids | asks | bid/ask | by quarter |
|---|---:|---:|---:|---|
| **in the money** | 336 292 | 336 292 | **100.0%** | 100%, 100%, 100%, 100% |
| **out of the money** | 122 127 | 543 513 | **22.5%** | 21%, 23%, **100%**, 14% |

**On in-the-money books the dealer places exactly one bid per ask, in every
quarter, to the unit**: 97 767/97 767, 103 384/103 384, 51 189/51 189,
83 952/83 952. On out-of-the-money books the same dealer places **4.45 asks per
bid**. It supplies **90.8%** of all OTM asks, so RT-051's missing bid is the
dealer's own quoting rather than incidental flow.

**Risk-limit withdrawal is excluded.** The registered discriminator was the time
profile: a dealer squeezed out by inventory or margin starts healthy and declines.
The ratio runs 21% to 14% first quarter to last — a factor of 0.66, where the
falsifier required below 0.5 — and is already at 21% in the first quarter.

**The registered claim was still missed.** I predicted a ratio below 20%; it is
22.5%. Direction right, threshold wrong, and a threshold missed is missed.

**Q3 breaks both candidates and is not noise.** In the third quarter the dealer
placed **25 092 bids against 25 093 asks — 100%** — on the same books it otherwise
skews 4:1. That is a 25 000-placement sample sitting between 23% and 14%. Neither
a static quoting policy nor a monotone withdrawal predicts a symmetric quarter in
the middle of an asymmetric run.

**Named confound, untested.** Total placements also collapse in Q3 (OTM 50 185
against 216 535 in Q1). Moneyness is classified against **terminal** spot, and
options expire and relist through the run across five listing timestamps, so a
book counted OTM at the end may have been at or in the money during Q3. That is a
candidate explanation and it has not been tested.

**Next experiment.** Recompute moneyness per listing epoch against contemporaneous
spot and re-split the quarters. If Q3's symmetry disappears, it was a
classification artifact and the quoting-policy reading stands; if it survives, the
dealer changes behaviour mid-run and neither candidate is correct.

**Scope.** One seed, one configuration. Placements count orders accepted, not
resting depth or time-weighted presence, so a class placing many short-lived
orders outweighs one resting a single quote.

## RT-053 — Quote-time moneyness gives a clean rule, and refutes my Q3 explanation

**Classification.** REAL for the moneyness rule; **REFUTATION** of my own
explanation for RT-052's anomaly, which is now confirmed as genuine dealer
behaviour.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, full logs.

**Method change.** RT-051 and RT-052 bucketed *books* by moneyness at the **end**
of the run, while options relist across five epochs with 2 h and 6 h tenors. This
buckets each **placement** by the contract's moneyness at the instant it was
quoted, against the contemporaneous median `ABC/USD` mid.

| bucket | bids | asks | bid/ask | by quarter |
|---|---:|---:|---:|---|
| deep ITM (>+5%) | 7 778 | 7 778 | 100.0% | – – **100%** 100% |
| ITM (+1…+5%) | 262 427 | 262 427 | **100.0%** | 100% 100% 100% 100% |
| at the money (±1%) | 107 281 | 143 299 | **74.9%** | 78% 79% **100%** 59% |
| OTM (−1…−5%) | 77 049 | 462 417 | **16.7%** | 14% 17% **100%** 12% |
| deep OTM (<−5%) | 3 884 | 3 884 | 100.0% | – – **100%** – |

**The rule, where the volume is.** Across the three buckets carrying **1 314 900
placements**, the bid/ask ratio falls monotonically as contracts move out of the
money: **100.0% → 74.9% → 16.7%**. In-the-money quoting is exact — **262 427 bids
against 262 427 asks**, equal to the unit across every quarter. The dealer runs a
strictly paired two-sided loop in the money and abandons it out of the money.

**The registered monotonicity claim still failed.** The two tail buckets read
100%, so the relation is not monotone across all five. Those tails hold 23 324
placements, **1.7% of the total**, and exist almost only in the anomalous quarter.
The threshold-shaped claim was wrong; the shape it reached for is present.

**My explanation of RT-052's Q3 is refuted.** I predicted the excursion was a
classification artifact that would vanish under quote-time moneyness. It does not:
the OTM bucket runs **14%, 17%, 100%, 12%**, a spread of **8.3x** against a
falsifier requiring under 2x. **In Q3 every bucket is exactly 100%.** The
symmetric quarter is the dealer's behaviour, not my measurement choice.

**Observation, not mechanism.** Every bucket at exactly 100% is not a gradual
shift but the signature of a **different quoting path** emitting strictly paired
quotes. RT-052 also measured total placements collapsing in that quarter. Two
candidate mechanisms have now been eliminated by measurement — risk-limit
withdrawal and classification artifact — and the third is deliberately not being
guessed.

**Next experiment.** Tally Q3 placements by **listing epoch**, the timestamp
embedded in each option symbol. If the paired quoting is confined to
freshly-listed contracts, the regime is a listing-bootstrap path; if it is spread
across all live expiries, the dealer itself changes. One pass over evidence
already collected.

**Scope.** One seed, one configuration. Placements count accepted orders, not
resting depth or time-weighted presence. Quote-time spot is the consensus mid at
or before each placement, inheriting the 1 s snapshot cadence.

## RT-054 — Tenor eliminated; the dealer's second regime described precisely

**Classification.** BOUNDED NEGATIVE RESULT. Third candidate mechanism eliminated
by measurement; the anomaly is now described sharply enough to act on without
being explained.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, full logs.

**Whole run**, bid/ask by quote-time moneyness x hours to expiry:

| | <0.5h | 0.5–1h | 1–2h | 2–4h | ≥4h |
|---|---|---|---|---|---|
| ITM (+1…+5%) | **100%** | **100%** | **100%** | **100%** | **100%** |
| at the money | 58% | 74% | 65% | 94% | 88% |
| OTM (−1…−5%) | 34% | 15% | **9%** | 47% | 17% |

**The moneyness rule is robust to tenor.** In-the-money placements are exactly
paired in *every* tenor band; out-of-the-money ones never exceed 47%. RT-053's
headline is not a tenor effect in disguise.

**The registered tenor prediction failed.** I expected the ratio to rise toward
100% at short tenor. For OTM it runs 34, 15, 9, 47, 17 per cent — non-monotone,
and the shortest band is not the highest. Tenor matters (a 5.2x spread) but not in
the predicted direction.

**Q3 is entirely short-dated**: 21.6% under 0.5 h, 23.7% at 0.5–1 h, 54.6% at
1–2 h — **99.9% under two hours** — and **zero** placements at ≥4 h, against
**170 439** in the run as a whole.

**But tenor does not explain the anomaly.** Restricting to Q3, every cell is
exactly 100%: OTM reads 3 475/3 476, 3 542/3 542, 8 750/8 750. Removing Q3 from
the whole-run figures, the OTM band reads 27.7%, 10.4%, 3.9%, 47.3%, 17.4%.
**Outside Q3 short tenor does not produce paired quoting; inside Q3 every tenor
does.** The two are orthogonal.

**What Q3 is, as description rather than mechanism.** Two things change together:
the dealer **stops quoting every contract beyond about two hours to expiry**, and
quotes those that remain in **strictly paired** form — 3 542/3 542, 8 750/8 750,
19 698/19 698, with a single unpaired ask in roughly 60 000 placements.

**Three candidates eliminated by measurement**: risk-limit withdrawal (RT-052),
classification artifact (RT-053), tenor (this). **I do not know why the regime
changes**, and after seven mechanism questions in this campaign that is where this
lineage stops rather than producing a fourth guess.

**Actionable without further audit work.** `option_dealer` has two distinct
quoting modes; the transition sits near the +4 h expiry boundary; in one mode it
abandons all contracts beyond ~2 h tenor. Whether that is intended is a question
about the dealer's design its author can answer far more cheaply than this can be
measured.

**Scope.** One seed, one configuration. Placements count accepted orders, not
resting depth. Quarters are the observer's calendar bucketing; the tenor axis is
the dealer's own.

## RT-055 — The missing option bid is one conditional and a spot-proportional spread

**Classification.** REAL, and it closes RT-051, RT-052 and RT-053 with a verified
mechanism. RT-054's separate anomaly stays open.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, full logs.

**Mechanism** (`simulations/derivsim/optionmm.go:351`):

```go
half := mm.spotMid * mm.cfg.SpreadBps / 10000   // 30 bps of SPOT
bid  := alignDown(theo-half-skew, tick)
ask  := alignUp(theo+half-skew, tick)
if bid > 0 {                                     // bid: conditional
    mm.SubmitOrder(sym, exchange.Buy, ...)
}
mm.SubmitOrder(sym, exchange.Sell, ...)          // ask: unconditional
```

**The half-spread is 30 bps of the underlying, not of the option's premium.**
`SpreadBps: 30` is hardcoded at `sim.go:3195`. At a 49 295 USD spot that is a flat
**≈148 USD** applied to every contract, whether it is worth 20 USD or 2 000. The
ask is always placed; the bid only when it prices above zero. **Every option worth
less than ≈148 USD is quoted ask-only.**

**Verified.** Predicted half-width `spot x 0.003` = **147.9 USD** against the
sampled quote's **149** (bid 16 / ask 314): **0.7% error**.

**It closes three findings quantitatively.**

| option premium | half-spread as % of premium |
|---:|---:|
| 100 USD | 148% |
| **148 USD** | **100%** (bid hits zero) |
| **193 USD** | **76.7%** |
| 1 200 USD | 12.3% |

RT-051 measured a **76.8% median OTM half-spread**, implying a typical OTM premium
of ≈193 USD — on this curve — against 10.0% for ITM, implying ≈1 480 USD. The
ITM/OTM partition, RT-052's 4.45:1 ask skew and RT-053's monotone moneyness rule
are all consequences of this one line.

**The skew hypothesis is falsified as tested and mis-specified as written.**
Measured dealer net option inventory by quarter (three dealers, three venues):
Q1 −6.3 to −8.5, Q2 −14.1 to −17.8, Q3 −39.3 to −44.2, Q4 −46.7 to −51.6
contracts. **Short in every quarter, monotonically**, so it does not distinguish
Q3 — fourth candidate eliminated.

But `skew` uses `q.inventory`, and `q` is the **per-contract** quote state, so the
relevant position is the dealer's holding *in that contract*, not its aggregate
book. **I measured the aggregate.** The skew explanation is therefore falsified
only in the aggregate form and remains untested in the per-contract form. The
preregistered arithmetic ("6 lots lifts a zero bid") was built on the same
aggregate reading; an aggregate of −8 contracts would imply bids for everything in
Q1, which the measured 14% Q1 ratio contradicts — and that contradiction is itself
evidence the term is per-contract, as the source says.

**Owner-facing.** A half-spread proportional to the underlying rather than to the
option premium makes cheap options untradeable on one side by construction, and
`SpreadBps: 30` is hardcoded rather than configured. Whether that is intended is a
design question; that it puts a 148 USD half-spread on a 165 USD option is
arithmetic.

**Scope.** One seed, one configuration. The 0.7% check is against a single sampled
quote; the premium-to-spread curve is derived from the formula, not fitted.

## RT-056 — Per-contract inventory is the bid gate; the option lineage closes

**Classification.** REAL, and it closes RT-054's open anomaly. Two of three
registered claims falsified on a confound of my own design; the mechanism stands.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, full logs.

**Measured** (`research/tools/dealerskew`; inventory is the exchange's own
post-fill position in that contract, so it needs no accumulation cross-check):

| inventory (lots) | bids | asks | bid/ask | share of Q3 |
|---|---:|---:|---:|---:|
| **≤ −20** | 42 653 | 42 653 | **100.0%** | **52.4%** |
| −20…−6 | 295 117 | 342 684 | 86.1% | 32.3% |
| −6…−2 | 70 522 | 397 107 | **17.8%** | **0.0%** |
| −2…0 | 23 669 | 61 261 | 38.6% | 0.0% |
| 0…+2 | 7 741 | 15 134 | 51.1% | 0.0% |
| > +2 | 18 717 | 20 966 | 89.3% | 15.3% |

**The formula's one unconfoundable prediction is confirmed to the unit.** Skew is
**24.65 USD per lot** against a **147.9 USD** half-spread, so **6.0 lots short
exactly cancels it**, and beyond that `bid = theo + (skew − half) > 0` **whatever
the contract is worth**. At ≤ −20 lots the margin is +345 USD, so a bid must
always be placed — measured **42 653 bids against 42 653 asks, exactly 100%**.

**RT-054's anomaly is explained.** Q3's placements are **84.7% beyond the 6-lot
short threshold** (52.4% at ≤ −20, 32.3% at −20…−6) and **0.0%** in the two middle
buckets. In that quarter the dealer's per-contract shorts are large enough that
skew lifts every bid above zero, which is why every moneyness and tenor cell read
100%. It fits RT-054's own observation that Q3 quotes a restricted short-dated
set: the same aggregate short spread over fewer contracts gives larger
per-contract positions.

**Two claims falsified, on a confound I built in.** The relation is U-shaped, not
monotone — 100%, 86%, 17.8%, 38.6%, 51.1%, 89.3% — and long inventory shows a
*high* ratio where the formula predicts a low one. The cause is that this table
**does not control for moneyness**: in-the-money contracts carry premiums far
above 148 USD and are quoted two-sided at any inventory, inflating whichever
bucket holds them. I conflated two variables I had already shown to matter
separately. The threshold claim also misses on its own terms — short beyond 6 lots
gives 87.7% against 90% predicted.

**No falsifier fires**: the ratio is not flat (5.6x range), Q3 does sit at high
short inventory, and high short inventory does not show a low ratio.

**The lineage closes.** RT-055 explained the missing bid; this explains the one
behaviour it left open. RT-051 through RT-056 reduce the entire option surface to
two lines of `optionmm.go` and their interaction with per-contract inventory.

**If reopened**, the cheap next step is to cross inventory with quote-time
moneyness in one table, separating the variables this experiment conflated and
turning the U-shape into the two monotone slices it almost certainly is.

**Scope.** One seed, one configuration. Placements count accepted orders, not
resting depth.

## RT-057 — The liquidation subsystem is never exercised

**Classification.** COVERAGE GAP, not a defect. An unexercised path is untested,
not broken — and this one is untested by a factor of 335.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`, 8 h, seeds 607/608/609.

**Measured** (`research/tools/marginheadroom`; headroom is equity over
maintenance margin, so liquidation is due at 1.0x):

| seed | accounts | zero maintenance | **min headroom** | median | closest account |
|---|---:|---:|---:|---:|---|
| 607 | 258 | 140 | **335.6x** | 16 651x | `carry_arb_1` |
| 608 | 258 | 138 | **335.0x** | 16 089x | `carry_arb_2` |
| 609 | 258 | 140 | **335.7x** | 16 993x | `carry_arb_1` |

The nearest approach to liquidation anywhere in the population is **335x the
threshold**, against a registered prediction of 10x and a falsifier at 2x. **138
to 140 of 258 accounts (54%) carry zero maintenance margin** — they hold no
position the risk engine can act on.

**The stability is the interesting part.** The minimum is 335.6, 335.0, 335.7
across three independent seeds — a **0.2% spread** on runs whose PnL levels swing
five-fold (RT-041). The closest account is always a **carry arbitrageur**, and
RT-046 showed every carry arbitrageur pinned at its configured 500-contract cap.
The cap fixes the position, which fixes the maintenance margin, which fixes the
headroom. **The population's closest approach to insolvency is a configuration
constant.**

**Corroborated independently.** The **insurance fund is empty in all three seeds**
— no venue, no asset, no entry. Never drawn on because never needed.

**What this campaign therefore does not test**: the liquidation trigger,
liquidation ordering across a book, partial versus full unwind, the insurance
fund, bankruptcy accounting, and any auto-deleveraging path. RT-045 recorded zero
liquidation events; this shows it was not a near miss.

**The honest framing.** Nothing here says the liquidation engine is wrong. It says
every conclusion this campaign supports about fairness under stress comes from a
population that never experienced any, and that the risk parameters — absent from
every config key, so engine defaults nobody chose for this scenario — have never
had to hold.

**Correction to a published denominator.** RT-033 and RT-041 describe
`triangle_arb` as "6 participants out of 252". The population is **258**; 252 was
the total excluding `triangle_arb` itself. The 75.4% share-of-gains figure is a
ratio of values, not headcounts, and is unaffected.

**Scope.** Three seeds, one configuration, terminal snapshots. A transient mid-run
approach would not appear, though a 335x terminal margin makes one implausible; a
time-resolved check is the obvious extension.

## RT-058 — The dated futures never converge, and that is the design's own test

**Classification.** REAL. A deliberately-built experiment with a decisive negative
result. The exposure it creates is latent rather than realized.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, full logs.

**Measured** (`research/tools/futbasis`), basis against the median-of-venues
`ABC/USD` by hours to expiry:

| band | samples | median (signed) | mean \|basis\| | max \|basis\| |
|---|---:|---:|---:|---:|
| **<0.5 h to expiry** | 21 597 | **+27.380%** | **43.079%** | **99.506%** |
| 0.5–1 h | 21 600 | +19.919% | 35.145% | 89.365% |
| 1–2 h | 43 025 | +9.827% | 22.579% | 79.281% |
| 2–4 h | 21 600 | +43.834% | 43.457% | 59.071% |
| ≥4 h | 43 028 | +13.698% | 13.399% | 30.998% |

The registered falsifier required a median under 2% in the final half hour; the
measurement is **27.4% signed, 43.1% mean absolute**. The basis never approaches
zero in any band, and across the three bands nearest expiry it **rises** — 9.8%,
19.9%, 27.4%.

**Verified on raw prices.** At the expiry instant of `ABC-FUT-1735711201`: futures
bid **9 814 120 000** / ask **9 820 060 000** against a spot mid of
**4 929 505 000** — **1.99x its underlying** as it settles. Same base and quote
precisions as spot with no multiplier (`instrument/listing.go:95`), so this is a
price level, not a unit artifact.

**It is deliberate, and it is the design's own question that fails.**
`futmm.go:21`, with `futures_maker_self_anchored: true` in the config:

```go
// SelfAnchored quotes each future around its own last trade (bootstrapped
// at spot on listing) instead of pegging to the spot mid. This lets the
// futures price wander on its own flow, so any basis convergence must
// come from arbitrage rather than from the quoting rule.
```

The quoting tether was removed on purpose, to test whether arbitrage enforces
convergence. **The answer is no**: the basis grows to twice the underlying. Not an
undocumented defect — a designed experiment with a clear negative result.

**Settlement reads the underlying, so the exposure is real.**
`exchange/expiry.go:326` feeds the settlement observer `underlyingPrice`, and the
observer's contract states the price arrives "by the contract's declared
underlying-reference path; it is never a trade, book-mid, or numeric-zero
fallback". A position carried into expiry settles roughly **50% away from where
the book last traded it**.

**But it is latent, and that was measured rather than assumed.** Realized PnL at
the expiry instants totals **51 USD across 6 events over all five contracts**.
Almost nobody carries a futures position into settlement, so the mispricing
transfers nothing today. It is an exposure the population happens not to take.

**Third instrument, third failed tether.** RT-031: the cross book self-references
by accident. RT-043/RT-044: the perpetual's tether saturates. RT-058: the future
is self-anchored on purpose and arbitrage fails to converge it. Only `ABC/USD`
holds, and RT-032 showed that is a configured peg. **Every instrument in this
campaign is either pinned by configuration or has no working anchor at all.**

**Scope.** One seed, one configuration. Basis uses top-of-book mids against a
consensus spot at or before each snapshot. At-expiry realized PnL counts events
within 2 s of the expiry timestamp; a settlement booked outside that window would
be missed, though the near-zero total across five contracts makes a large missed
transfer unlikely.

**QUALIFIED by RT-059.** The futures book carries **1 205 contracts of volume
against 205 926 on `ABC/USD` — 0.59%**. The basis fails to converge partly because
**the market barely exists**, not only because arbitrage was tried and failed.
"Arbitrage does not converge it" overstates what a book this thin can test.

## RT-059 — The convergence force is too small, but not in the way I predicted

**Classification.** MIXED. Capacity explains two of three unanchored instruments;
the third needs the position cap instead. Also qualifies RT-058.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, full logs.

| book | total volume | maker share | designated arb | % of total | **% of taker side** |
|---|---:|---:|---|---:|---:|
| ABC/CDF | 789 548 | 48.2% | `triangle_arb` | 0.9% | **1.7%** |
| CDF/USD | 1 602 899 | 49.6% | `triangle_arb` | 0.4% | **0.9%** |
| ABC-FUT | 1 205 | 50.0% | `dated_carry_arb` | 2.6% | **5.3%** |
| **ABC-PERP** | 116 722 | 47.9% | `carry_arb` | 12.6% | **24.3%** |

One maker is ~50% of every book, which is mechanical when volume is counted per
fill side. The taker-side column is the meaningful one.

**The registered claim is falsified.** I predicted every designated arbitrageur
under 5% of volume; `carry_arb` is 12.6% of total and **24.3% of taker flow** on
the perpetual — missed by a factor of 2.5. The falsifier at 20% of *total* does
not fire, but the threshold I wrote does.

**The split is the finding.** On `ABC/CDF` the arbitrageur is 1.7% of taker flow
against a book 69% from fair (RT-031); on the futures 5.3% against a basis of 27%
to 99% (RT-058). There, capacity is a sufficient explanation. On the perpetual it
is **24.3%** — substantial — and RT-044 still measured the book **29% from
index**. **Capacity does not explain the perpetual.**

**What does is already in the record.** RT-046 measured every `carry_arb` pinned
at exactly 500 contracts, so the class holds at most **3 000** while trading
**14 744** — a **4.9x turnover**. **The arbitrageur has flow but no balance
sheet.** Moving a price requires holding the other side of an imbalance, not
churning through it. My arithmetic framing was right in substance and wrong in
variable: the binding constraint is the position cap, and volume share is what I
chose to measure.

**Unplanned second result.** The dated futures book carries **1 205 contracts
against 205 926 on `ABC/USD` — 0.59%**, and 0.15% of `ABC/CDF`. The futures market
barely exists, which qualifies RT-058: its basis fails to converge partly because
almost nobody is there, a thinner claim than "arbitrage was tested and failed".
RT-058 has been annotated accordingly.

**Net.** Across three unanchored instruments the convergence force is 1.7%, 5.3%
and 24.3% of taker flow, capped at positions far below the imbalances it faces,
and where its flow is substantial the cap still stops it holding a position that
would matter. **The campaign's convergence questions were posed to participants
configured too small to answer them.**

**Scope.** One seed, one configuration. Volume is per fill side. "Designated
arbitrageur" is my reading of which class is meant to converge each book; the
config does not state it.

## RT-060 — The cap binds exactly when the dislocation is material

**Classification.** VERIFICATION of RT-059's asserted mechanism, and the
reconciliation of two earlier measurements that looked inconsistent.

**Base.** `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, full logs.

**Measured** (`research/tools/arbresponse`), `carry_arb` aggregate position
against `ABC-PERP` |basis|, class cap 3 000 contracts:

| basis band | samples | mean basis | mean position | max position | **% of cap** |
|---|---:|---:|---:|---:|---:|
| 0–5% | 19 983 | 0.37% | 1 209.6 | 3 000.0 | 40.3% |
| **5–10%** | 2 583 | 7.17% | **3 000.0** | 3 000.0 | **100.0%** |
| **10–20%** | 3 802 | 14.46% | **3 000.0** | 3 000.0 | **100.0%** |
| **20–30%** | 2 395 | 24.88% | **3 000.0** | 3 000.0 | **100.0%** |

**Whenever the basis exceeds 5% the position is at exactly the cap** — mean and
maximum both 3 000.0 — in every one of 8 780 samples. The registered claim asked
for 80% of cap beyond a 20% basis; the measurement is 100%. No falsifier fires.

**It reconciles RT-046 with RT-059.** RT-046 measured these participants at their
cap only **31–44% of the time**, which I had flagged as evidence *against* a
binding constraint. The basis is under 5% for **69.5% of the run**, and during
those stretches the arbitrageur legitimately sits at 40% of its limit because
there is little to arbitrage. It is pinned for **30.5%** of the run — exactly the
fraction during which the dislocation is material. The two measurements were never
in conflict; a time-average had been compared against a conditional one.

**RT-059's sentence is verified, not retracted.** "Flow but no balance sheet" now
has a measurement: at full extension the class holds **3 000 contracts against a
book carrying 116 722 contracts of volume — 2.6%** — and the basis stays at
**−29%** while it is pinned there. **Even at maximum permitted capacity the
convergence force cannot close the gap**, which is stronger than the volume-share
argument because it is conditional on the arbitrageur doing everything its
configuration allows.

**Calibration note.** Six asserted mechanisms in this campaign failed their own
tests and each was logged. This is the first load-bearing one in that sequence to
survive. The note belongs in both directions: testing assertions is not a ritual
that always finds them wrong, and the one that held now carries the campaign's
central conclusion about capacity.

**For the owner.** The perpetual's convergence force is configured at 3 000
contracts across six participants, reaches that limit whenever the basis exceeds
5%, and holds it while the basis runs to 29%. **`carry_max_position` is the single
parameter that would make the convergence question answerable**; at its current
value the answer is bounded by configuration rather than by market behaviour.

**Scope.** One seed, one configuration. Position is the exchange's post-fill
position per contract summed across the class; basis is the top-of-book perp mid
against the consensus index at matching timestamps.
