# Economic correctness research note

Canonical entry point for the economic / execution correctness audit.
`research/red-team-findings.md` is the findings index; this note is the record of
hypotheses, experiments, corrections and coverage. Every substantive claim there
has an entry here.

Cross-references:
- `research/accounting-audit.md` — pre-existing statement of the accounting
  identities. Reused, not restated. Its definitions are authoritative for this
  audit unless a correction below says otherwise.
- `research/red-team-findings.md` — finding index, IDs `RT-nnn`.

ID convention: hypotheses `H-nnn`, experiments `E-nnn`, findings `RT-nnn`
(preserved from the existing report). IDs are never reused for a different claim.

---

## A. Scope and provenance

| item | value |
| --- | --- |
| scientific branch | `feature/r2-cdf-survival-successor` |
| pinned revision | `a666d02faede3d40f046b11e60eb672c59386a94` |
| audit branch | `redteam/economic-audit`, based on that revision |
| toolchain | `go1.26.7-X:nodwarf5 linux/amd64`, `GOAMD64=v1` (default) |
| configs used | `research/configs/v2-integrated-longrun/dev-607.json` only |
| holdouts | 619 / 631 / 641 — **never read, never run** |
| last checkpoint | CP-2, this note created; RT-001 reconciled against pinned rev |

The scientific head was re-pinned from the repository at the start of this
session rather than taken from the previous transcript. It is unchanged since
RT-001 was written.

Assumptions: development configs only; no live venues, funds or credentials;
economics are not modified; scientific metric definitions are not decided here.

---

## B. Economic model and invariant map

Entities inside the boundary: participants (cash `Balances`, perp cash
`PerpBalances`, earmarks `Reserved` / `PerpReserved`, debt `Borrowed` split by
`BorrowedSpot`), the venue (`ExchangeBalance.FeeRevenue`, `InsuranceFund`),
positions held in the `PositionManager`, and instruments with lifecycle state.

Authoritative identity, from `research/accounting-audit.md`:

    InternalNet + ExchangeTake + OpenLinearValue = 0

with external deposits and borrowing outside the zero-sum, entering once.
`OpenLinearValue` is independently defined there as unrealised PnL on positions
nobody has closed — cash not yet paid. This audit treats that as a definition
with an owner and an extinguishment rule (it goes to zero at settlement), which
is the bar the instructions set for a reconciliation term.

Invariants derived and used so far:

| ID | invariant | scope |
| --- | --- | --- |
| INV-1 | every balance mutation is recorded in the conservation tracker before the logger is consulted | all mutation sites |
| INV-2 | signed positions per contract sum to zero | every derivative contract |
| INV-3 | option expiry cash nets to zero (payoff has no entry-price term) | option settlement |
| INV-4 | futures expiry cash is **not** required to net to zero — surviving bases are unpaired because partial closes already realised PnL | futures settlement |
| INV-5 | venue take reconstructs exactly from its own movement stream | fee revenue |
| INV-6 | borrow and repay net to zero per asset | credit |
| INV-7 | debt does not disappear with an account; a write-off has a named creditor loss or funding source | liquidation / default |
| INV-8 | collateral released once per obligation; no release without a matching reservation | margin lifecycle |

INV-4 is recorded explicitly because imposing the naive local invariant instead
would have produced a false finding — see G-1.

---

## C. Audit coverage

| mechanism | transition | invariant | status | evidence |
| --- | --- | --- | --- | --- |
| expiry settlement (futures) | open → settled | INV-1, INV-4 | **defect found, fixed** | RT-001, E-001..E-004 |
| expiry settlement (options) | open → settled | INV-3 | **defect open** | RT-002, E-003 |
| position netting | any | INV-2 | no violation, bounded | E-002 |
| system identity | whole run | §B identity | no violation, residual 19/1.6e16 | E-003 |
| venue take | fee accrual | INV-5 | no violation | E-003 |
| borrow / repay | credit → repay | INV-6 | net zero on ABC, CDF | E-003 |
| liquidation / default | margin call → liquidation → shortfall | INV-7 | fixture: no violation. **NOT EXERCISED in integration** | E-008 |
| detector sensitivity | injected faults | INV-1 | boundary established, 2 gaps | E-009 |
| collateral release | reserve → cancel / fill | INV-8 | no violation, 2 races | E-010 |
| transfers in flight | debit → transit → credit | — | not tested | — |
| option exercise vs live hedge | exercise → assignment | — | not tested | — |
| relisting under a reused symbol | settled → listed | — | not tested | — |
| information causality (transport) | publish → deliver → decide | causality | no violation, 225 rows | E-011 |
| execution path (admission → fill → clearing) | — | — | not tested | — |

Coverage classes: E-002/E-003 are *reachable integration* evidence from one
config and one seed. Everything marked "not tested" has no fixture coverage
either.

---

## D. Hypothesis register

**H-001 — expiry settlement is not recorded for conservation.**
Status: **SUPPORTED WITHIN TESTED SCOPE**, fixed. → RT-001.
Origin: code inspection of the single `balance_change` emission that bypasses
`logBalanceChange`, then confirmed on a real run.

**H-002 — option expiry cash does not net to zero.**
Status: **SUPPORTED WITHIN TESTED SCOPE**, unfixed by choice. → RT-002.
Disposition is an owner decision, not a code change.

**H-003 — derivative positions can fail to net (phantom counterparty).**
Status: **FALSIFIED WITHIN TESTED SCOPE.** 99 contracts, all net exactly zero.
Scope: one config, one seed, 7 simulated hours.

**H-004 — debt can disappear at liquidation, or residual value can reach the
wrong party.** Status: **FALSIFIED WITHIN TESTED SCOPE** (E-008, on `a666d02`).
Scope: one forced-bankruptcy fixture, cross-margin default mode, no clearance
fee, single instrument. Not a statement about production reachability, nor
about multi-instrument or cascade cases.
Origin: RT-003 records liquidation as unexercised; it is the largest unaudited
surface and the classic location for an extinguished obligation without a payer.
Claim: there exists a reachable sequence (borrow → short → adverse move →
liquidation with collateral shortfall) after which either total debt falls
without a creditor loss, or the insurance fund absorbs an amount that does not
equal the shortfall, or collateral is released twice.
Invariant: INV-7, INV-8.
Why it matters: an extinguished liability is unsupported wealth creation, and a
mis-signed insurance draw silently subsidises a defaulter.
Candidate paths: `exchange/liquidation.go`, `exchange/borrowing.go`,
`exchange/margin.go`, `ExchangeBalance.InsuranceFund`.
Competing explanations: liquidation may be unreachable in dev configs; the
shortfall path may be guarded by admission checks.
Discriminating test: a development-only fixture that forces a shortfall, with an
independently computed expected debt and insurance draw.
Known limitation: a fixture proves the transition, not its production
reachability; reachability must be argued separately.

**H-005 — collateral can be released more than once, or released without a
matching reservation.** Status: **FALSIFIED WITHIN TESTED SCOPE** (E-010, on
`a666d02`). Scope: double cancel, and cancel after a full fill, on one perp with
one account. Not tested: partial fills, amend/replace, cross-margin with several
instruments sharing the earmark, or a cancel racing a fill under concurrency.
Origin: `ReleasePerp` clamps at zero (`max(0, PerpReserved-amount)`), which
silently absorbs an over-release instead of failing. That is a detector-shaped
weakness: a double release leaves no trace.
Invariant: INV-8.

**H-006 — the conservation tracker cannot see a mis-directed transfer.**
Status: **SUPPORTED WITHIN TESTED SCOPE** (E-009). Confirmed by controlled
mutation, together with a second blind spot found while testing it. Origin: `VerifyConservation` compares per-asset totals, so a
payment to the wrong participant preserves every total it checks. Demonstrated
incidentally by E-004: a balanced two-party settlement hid an omitted recording
entirely. This is an audit-coverage hypothesis about the detector, not the
economy.

**H-007 — repeated settlement or exercise of the same contract double-pays.**
Status: **OPEN.** Origin: `settleExpiredInstrument` deletes the book and
instrument at the end; a retry path (`settlementPending`) exists for the
price-unavailable case. Whether a second call can pay twice is untested.

---

## E. Experiment journal (append-only)

**E-001 — RETROSPECTIVE. Static map of balance-mutation sites.**
Source: previous session, reconstructed from the repository, revision `a666d02`.
Method: `grep` for `"balance_change"` emissions and `conservation.record` calls.
Result: exactly one production emission bypasses `logBalanceChange` —
`exchange/expiry.go:640`. All other sites use the helper; every other match is
in the analysis layer (readers).
Establishes: the bypass is unique and bounded. Does not establish reachability.

**E-002 — RETROSPECTIVE. Position netting on a real run.**
Command: `multivenue -config dev-607.json -duration 7h -seed 607`, completion
verified by `done: sim=7h0m0s wall=6m28s`.
Result: 9 perp/futures contracts and 90 option contracts, signed net exactly 0
on all 99. H-003 falsified within scope.

**E-003 — RETROSPECTIVE. Project auditor on the same run.**
Command: `mvanalyze -metric conservation <run>`.
Key numbers: `identity USD external 16072200000000000 internal -345452610503
exchange 357011627991 open -11559017469 residual 19 (1.18e-15)`;
`expiry: largest net -14686856908 (not required to be zero)`;
`option expiry: worst net 6 (must be zero)`.
Establishes: no value destruction; INV-4 confirmed as the auditor's own rule;
INV-3 violated by ≤6 units → RT-002.

**E-004 — RETROSPECTIVE. First regression test was non-discriminating.**
A two-party settlement with equal and opposite bases passed on the *unpatched*
base, because the tracker compares per-asset totals and the settlement cash
netted to zero. **INVALID as evidence for H-001.** Superseded by E-005.

**E-005 — RETROSPECTIVE, then re-run this session. Discriminating regression
test.** `exchange/expiry_conservation_test.go`, bases 100 vs 140 so settlement
cash does not net.
This session, on pinned `a666d02` (fresh worktree, `go test ./exchange/ -run
TestExpirySettlementIsRecordedForConservation`): **FAIL**, `Gap: 4000000`.
On `redteam/economic-audit` `20a5626`: **PASS**.
Establishes: RT-001 reproduces on the pinned revision and the fix addresses it.

**E-006 — RETROSPECTIVE. Fix impact on a full run.**
7h run patched vs unpatched: `conservation_violation` 17,999 → 0 per venue
(53,997 → 0 total); all 12 non-`general` log files byte-identical; each
`general.jsonl` identical once violation lines are removed.
Establishes: semantic impact none beyond removing the spurious events.

**E-007 — H-004 liquidation fixture, first attempt. INVALID.**
Command: `go test ./tests/ -run TestAuditLiquidationDeficitHasAPayerAndDebtSurvives`
on `redteam/economic-audit`. Result: four economic assertions passed; the
conservation assertion failed with `Gap: 46000000`.
**The gap was the test's own setup, not the product.** The fixture assigned
`Balances["USD"] = 500` and `Borrowed["USD"] = 40` by direct field write, which
are themselves unrecorded mutations: 500 − 40 = 460, and the reported gap is
exactly 46,000,000 at this precision. An invalid experiment cannot support or
falsify anything; superseded by E-008.

**E-008 — H-004 liquidation fixture, corrected.**
Change: assert on the *change* in the conservation gap across the liquidation
rather than its absolute value, so the setup artifact is held constant and the
transition is what is measured.
Run on the pinned scientific revision `a666d02` in a clean worktree (not on the
audit branch, so the RT-001 patch cannot be credited for the result):
**PASS**. Independently hand-derived expectations, all met:
realized on close = 10 × (80 − 100) = −200 USD; perp cash 100 − 200 = −100 →
deficit 100; insurance fund = −100 exactly; bankrupt perp cash → 0; borrowed
principal unchanged at 40; spot balance unchanged at 500; the liquidation moves
the conservation gap by zero.
Establishes: within this scope the deficit has a named payer, the loan is not
extinguished, and the write-down records both legs. Does not establish anything
about cascades, multi-instrument cross-margin, or production reachability —
liquidation did not occur at all in the 7h integration run (E-002).

**E-009 — detector sensitivity, controlled mutations.**
Files: `tests/economic_audit_detector_sensitivity_test.go` and
`exchange/economic_audit_recorded_destruction_test.go`. All results as
predicted before running:

| injected fault | detected by `VerifyConservation`? |
| --- | --- |
| valid control, no fault | no report (correct) |
| unrecorded credit (+7) | **yes**, gap +7 |
| unrecorded debit (−3) | **yes**, gap −3 |
| debt silently cancelled (50) | **yes**, gap +50 |
| one smallest currency unit | **yes**, gap 1 |
| value paid to the wrong participant | **no — survives** |
| value destroyed but faithfully recorded | **no — survives** |

Establishes the detector's boundary empirically: it detects *unrecorded*
mutations, and only those. Nothing in it requires a debit to have a matching
credit, and nothing in it identifies a recipient. Both surviving faults are
audit-coverage gaps for this detector and are covered instead by the identity
check in `research/accounting-audit.md`. The two checks are complementary;
neither subsumes the other. Recorded rather than weakened, per the audit rules.

**E-010 — H-005 collateral release, cancel/fill races.**
File: `tests/economic_audit_collateral_test.go`. Run on the pinned base
`a666d02` as well as the audit branch: **PASS** on both.
Cases: (a) cancelling the same resting order twice frees the earmark once and
the second cancel moves nothing; the earmark returns exactly to its pre-order
value. (b) Cancelling an order that has already filled completely does not
release the position's margin, and `PerpAvailable` never exceeds
`PerpBalances`.
Why this framing: `PerpAvailable = PerpBalances - PerpReserved`, so an
under-counted earmark is buying power the account's capital does not support —
free leverage relative to other participants, not merely a bookkeeping slip.
`ReleasePerp` clamps at `max(0, reserved-amount)`, so an over-release would be
absorbed silently, and RT-004 established the conservation tracker cannot see it
either, because the earmark lives inside `PerpBalances` and no total moves.
Establishes: neither of the two obvious double-release paths fires. Does not
establish: behaviour under partial fills, amend/replace, multi-instrument
cross-margin, or a genuine concurrent cancel/fill race.

**H-010 — an actor can act on information it should not have (fairness).**
Status: **FALSIFIED WITHIN TESTED SCOPE** for the two forms tested (E-011).
Static: no actor holds an exchange reference. Configured-vs-delivered: all 225
link x channel rows reconcile. Not tested: whether an actor's *decision*
timestamp ever precedes the delivery timestamp of the data it used — the
per-decision causality check is still open, and is now H-011.
Origin: the operator's framing that the simulation must be a fair battle of
actors. Instruction 6 names the specific risk: a participant observing a fill
before the modelled receipt, reading a future price, or reaching global state
directly rather than through its gateway.
Claim: some actor path reads exchange or venue state that its own message flow
would not have delivered yet, giving it an advantage no other actor could obtain.
Invariant: information causality — an actor's decision at time t may depend only
on messages delivered to it by t.
Candidate paths: actor implementations under `simulations/multivenue/`, any
direct reference from an actor to `*DefaultExchange`, `MDPublisher`, or a
position/book structure rather than to its gateway.
Discriminating test: static — enumerate actor→exchange references that bypass
the gateway; then dynamic — check that no decision timestamp precedes the
delivery timestamp of the information it used.
Why it matters: a latency or information advantage that is not part of the
modelled economics invalidates every relative-performance conclusion drawn from
these runs, which is precisely what the campaign measures.

**E-011 — H-010, information causality and latency fairness.**

*Stage 1, static.* No file under `simulations/multivenue/` holds a
`*DefaultExchange`, an `OrderBook` or a `PositionManager`. Actors reach the
venue only through `actor.Gateway` behind a `simulation.Mount`; the only direct
exchange handle is `Venue.Exchange`, which is the venue itself. So an actor
cannot read venue state directly, and the courier boundary is the only path.

*Prior art, not duplicated.* `simulation/information_boundary_test.go` already
asserts that market data cannot arrive before publication plus latency and that
receipts attest inbox arrival, and `simulation/delayed_gateway_test.go` covers
request, response and market-data latency separately. The audit did not rewrite
these.

*Stage 2, configured versus delivered.* dev-607 / seed 607 / 20m, `latency.json`
(`domain: courier_delivery`), 225 link x channel rows over 27 participant
classes and 3 remote maker feeds. Every row's drawn latency matches the model
its config declares:

| class | model | expected mean | delivered |
| --- | --- | ---: | ---: |
| `fixed_distance_maker` | spiky, p=0.01, spike 50 ms | 0.99·1e6 + 0.01·50e6 = 1,490,000 | 1,387k–1,530k across venues/channels |
| `future_flow` | lognormal σ=0.8 | 15e6·e^0.32 = 20,656,500 | 19.6e6–20.9e6 |
| `noise_flow` | lognormal σ=1.0, cap 500 ms | 20e6·e^0.5 = 32,974,000 | 32.8e6–33.4e6 |
| `imbalance_maker` | normal, sd 0.5 ms | 2,000,000 | 1,996,842–2,004,565 |
| `liability_hedger`, `option_liability_user` | constant + `market_data_scale: 2` | md 40e6, req/resp 20e6 | exactly 40e6 / 20e6 |
| `cross_venue_arb` | `cross_venue_base_latency` | 1,000,000,000 | exactly 1e9 |
| remote maker feeds ×3 | constant | 10e6, 20e6, 30e6 | exactly 10e6, 20e6, 30e6 |

No link has a zero-latency channel. Undelivered at shutdown is 1,486 of
2,637,617 scheduled messages (0.056%), spread over 140 rows, worst single row
132 of 232,074 — a shutdown-boundary drain, not a systematic starvation of one
actor.

Establishes: within this config and seed, no participant class receives a speed
advantage the configuration did not grant it, and the two direction-asymmetric
classes are asymmetric *by explicit configuration*.

*Correction, recorded because it nearly became a finding.* The first two passes
of this reconciliation reported 15 and then 9 "mismatches". Every one was my own
expectation model, not the system: pass 1 compared the configured `delay`
parameter against a delivered *mean* for stochastic models and did not know
about `market_data_scale` or `cross_venue_base_latency`; pass 2 still gave the
spiky mixture zero tolerance and mapped the remote-feed link names in the wrong
direction. Reporting either pass would have produced a false finding about the
fairness of the actor population. The lesson generalises: for a stochastic
latency model the configured parameter is not the expected delivered mean.

**H-011 — an actor's decision uses data it had not yet received.**
Status: **OPEN.** E-011 establishes that the *transport* honours its
configuration; it does not establish that an actor's decision logic consults
only what its inbox already held. A decision that read a shared structure
directly, or that ran before draining its inbox, would satisfy every check in
E-011 and still be unfair.
Discriminating test: the runs already emit market-data receipts
(`market-data-receipts-v2.bin`) and maker decision records with timestamps; join
decision events to the receipt of the observation they cite and assert the
receipt precedes the decision. Cheap, and uses evidence that already exists.

**H-011 update — PREREGISTERED 2026-09-07, before E-012 was designed or run.**

Reading `analysis/receipts.go` before writing anything showed that the project
has already built the check H-011 asked for, and built it stronger than this
note assumed. `MarketDataReceiptAudit` carries `FutureDecisionUse` and
`BadDecisionFrontier`, and both feed `Valid`. The mechanism is not a timestamp
comparison. Each decision record cites a *frontier*: an ordinal, a delivery
timestamp, and a 16-byte digest. The auditor recomputes that digest itself, as a
hash chain over every receipt delivered on that `(client, link)` in receipt
order, and rejects the decision unless the cited triple equals the chain the
receipt stream independently produces. An actor therefore cannot cite an
observation the receipt file does not contain, cannot cite one further ahead
than the receipts that precede its decision in the global event order, and
cannot cite the right ordinal with the wrong content.

Writing a competing causality checker would have duplicated this. It is recorded
as prior art, not re-derived. What is worth stating is the boundary of what it
proves, because that boundary is what remains open:

1. It constrains what a decision *cites*, not what the deciding code *read*. An
   actor that consulted a shared structure directly and then honestly reported
   its inbox frontier would pass every check. The record layer cannot see this;
   only the static argument in E-011 stage 1 can, and that argument is about
   reachability, not about every call site.
2. It covers only instrumented decisions. `-record-market-data-receipts`
   requires an explicit `-market-data-receipt-roles` list, so an un-audited role
   emits no decision record and contributes nothing to check or to violation.
   Coverage is therefore a property of the run configuration, not of the code.
3. `FutureDecisionUse` is only evaluated when `frontierOrdinal > 0`. A decision
   taken before any observation arrived is checked by the frontier-equality test
   alone.

Status of H-011: **SUPPORTED WITHIN TESTED SCOPE, by the project's own
instrument rather than by this audit** — with limits 1-3 as the residual. Limit
2 is measurable and is deferred to a later experiment.

**H-012 — the participant-information audit has two independent
implementations, and they are known to agree only on clean evidence.**
Origin: `AuditMarketDataReceipts` delegates to `auditMarketDataReceiptsStreaming`;
`auditMarketDataReceiptsBuffered` is retained and documented as "a review oracle
while the production path moves to bounded streaming".
`TestAuditMarketDataEvidenceStreamingMatchesBufferedOracle` compares them on one
valid fixture. `TestAuditMarketDataEvidenceCatchesAdversarialMutations` runs six
faults, but through the streaming path only. So the oracle is exercised where
the two paths are least likely to differ, and not exercised where they are most
likely to differ.

Why divergence is plausible rather than pedantic: the two are not a refactor of
each other. `DuplicateSource` is a map lookup in the buffered path and an
external merge sort over spilled runs in the streaming path. `MissingDueReceipt`
is a map iteration over retained schedules in one and a disk-backed scan in the
other. Different algorithms answering the same question is exactly where a
detector loses sensitivity silently.

Why it matters beyond tidiness: `Valid` is a gate on evidence the campaign
relies on. If the two disagree under fault, then whether a fault is caught
depends on which path ran, and the buffered function's standing as a review
oracle is not established.

Prediction, recorded before running: I expect agreement on the majority of
faults, with the divergence risk concentrated in `DuplicateSource` and
`MissingDueReceipt`. A run in which every fault produces byte-identical audit
structs falsifies H-012 within the tested fault set and *strengthens* the
oracle's standing. A single divergence is a finding.
Falsifier: all faults produce `reflect.DeepEqual` audit results.

**E-012 — H-012, the two participant-information audit implementations under
fault.** Preregistered above, before the test was written.
Artifact: `analysis/economic_audit_receipt_oracle_test.go`.
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Reproduce: `go test ./analysis/ -run TestAuditMarketDataEvidenceOracleAgreesUnderFaults -v`.

Method: fifteen fault injections into the V2 evidence fixture, each rewriting
every file digest so the auditor must catch broken semantics rather than a
checksum, driven through `auditMarketDataReceiptsStreaming` (the production
path) and `auditMarketDataReceiptsBuffered` (the retained review oracle), with
the full audit structs compared. A clean control is included, and every fault is
required to be detected by *both* — a fault neither notices is an audit-coverage
gap, not an agreement result, and the test says so.

Result: **H-012 SUPPORTED.** Two divergences, in different classes.

*Divergence 1, a disagreement about validity.* Swapping the two schedule
*records* wholesale changes only the order the file stores them in: the multiset
of event ordinals is untouched and every record stays internally consistent. On
the base revision the streaming auditor reported `Valid=false`
(`BadEventOrder=2`) and the buffered oracle reported `Valid=true`, with every
counter zero. The mechanism: the buffered path collected all three record kinds
into one slice and `sort.Slice`d it by event ordinal, which *repairs* a file
stored out of order before any check sees it; the streaming path merges the
three files as stored and therefore observes the disorder. An oracle that
accepts evidence the production path rejects cannot serve as a review oracle,
which is the role its own comment assigns it. Recorded as RT-007 and fixed on
the audit branch by replacing the sort with the same three-way merge the
streaming auditor performs — smallest head wins, ties in schedule, receipt,
decision order. Discrimination check: with the fix reverted the new test fails on
this fault and on "global event order is permuted"; with it applied all fifteen
pass, and the pre-existing `analysis` suite still passes.

*Divergence 2, a disagreement about classification only.* Reordering the two
schedules' per-link ordinals is rejected by both, but the streaming path calls
it `receipt_without_schedule` twice while the oracle calls it
`receipt_without_schedule` once and `schedule_receipt_mismatch` once. The
mechanism is deliberate: the streaming spill is bounded, so it appends a
schedule only while its per-link ordinal is in sequence, and a receipt whose
schedule was dropped has no schedule to compare against; the oracle keeps every
schedule in a map, so the schedule is present but wrong. Not fixed — this is
what bounded memory costs, and erasing it would make the oracle a copy rather
than an independent view. It is *pinned* in the test with its exact counters, so
a change that turns it into a disagreement about validity fails loudly, and
every other counter is still compared exactly.

*Prediction accuracy, recorded because it was wrong.* The preregistration named
`DuplicateSource` and `MissingDueReceipt` as the likely divergence sites, on the
reasoning that those two use genuinely different algorithms (external merge sort
versus map; disk scan versus map iteration). Both agreed on every fault. The
real divergences were in traversal order and in schedule retention — neither
predicted. The hypothesis was supported and the mechanism reasoning was not: I
looked for divergence where the *data structures* differ, and it was where the
*control flow* differs. Sorting versus merging is not an implementation detail
of a checker whose subject is order.

Scope: fifteen faults on a two-record fixture, one link topology. It does not
establish equivalence in general; it establishes that the specific permissive
gap is closed and that the remaining difference is bounded and named.

**H-013 — decision-record coverage is a property of the run configuration.**
Status: **FALSIFIED WITHIN TESTED SCOPE as a vacuous-pass risk; the residual is
documentation, not a defect.** The concern was that a role list which names
nothing real would produce zero decision records and a vacuously `Valid` audit.
The project already forecloses this, in three places:
`NormalizeConfig` rejects a receipt role that is not an unnumbered role class,
that is duplicated, or that has no explicit nonzero delayed link
(`simulations/multivenue/sim.go:1249`); and `validateMarketDataReceiptCoverage`
rejects a run where an audited role has no participants or feed sessions at all,
or where the number of instrumented links does not equal the number of
participants — its comment states the reason exactly: "Evidence that silently
covers only a subset of a role is worse than no claim of an information boundary
at all."
What remains true and is not a bug: roles *absent* from the list emit no
decision records, so the causality guarantee is scoped to the listed classes.
Every role class that has decision instrumentation also has its own test that
enables receipts for it alone (`simulations/multivenue/*_test.go`), so the
per-class check is exercised; what no single run establishes is all classes at
once. Recorded as scope, not as a finding.

**H-014 — a hidden order pays nothing for being hidden.**
Origin: `matching.makerAvailable` returns the full remainder for `Normal` and
`Hidden` alike, and only icebergs are throttled to their display tranche. Both
matchers use it. So at one price a hidden order has exactly the time priority a
displayed order of the same size would have, while contributing nothing to the
public snapshot.
Why it is a fairness question: real venues subordinate hidden size to displayed
size at the same price precisely because otherwise displaying is irrational. If
hiding costs nothing, `Hidden` weakly dominates `Normal` — same fills, less
information leaked — and any actor using `Normal` is handicapped for no
compensating benefit.
Predicted observable: a hidden order resting ahead of a displayed order at the
same price takes the whole fill.
Falsifier: displayed size is served first at equal price.

**H-015 — the fee charged on one economic exposure depends on how the
counterparty sliced it.**
Representation change: stop looking at a fill and look at the *partition* of a
quantity into fills. Fee is computed per execution from an integer
multiply-divide, and integer division truncates. Truncation is subadditive:
`sum_i trunc(f(q_i)) <= trunc(f(sum_i q_i))`, with the gap growing as the
partition gets finer. So the same exposure, taken as N small fills instead of
one, should cost *less* in fees, and the shortfall lands on the exchange's fee
revenue.
Why this is an actor-fairness question and not a rounding nit: a participant's
cost would then depend on a decision the *counterparty* made — how finely to
slice — and an actor that deliberately slices into minimum-size clips pays
systematically less than one that trades the same quantity at once. At a fine
enough partition each slice's fee truncates to zero and the trade becomes free.
That is a free-money mechanism reachable with no special privileges, only order
sizing.
Predicted observable, recorded before measuring: total fee across N equal
partial fills is **less than or equal to** the fee on one aggregate fill, with
equality only when no truncation occurs, and the deficit appears in
`ExchangeBalance.FeeRevenue`, not in a counterparty's balance.
Cheapest discriminating test: fill the same total quantity against the same
resting price twice — once as one execution, once as N — and compare the fee
totals and the exchange's fee revenue. Deterministic, no simulation run needed.
Falsifier: the two totals are equal for every partition, which would mean the
fee is computed on an aggregate or that the rounding is compensated.
Mechanism family: quantization asymmetry.

**E-013 — H-015, fee dependence on the partition of a fill.** Preregistered
above. Artifact: `tests/economic_audit_fee_partition_test.go`.
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Reproduce: `go test ./tests/ -run 'TestAuditTakerFee|TestAuditFeeVanishes' -v`.

Method: the same total quantity is taken against the same resting price twice,
once as one execution and once as ten, and the exchange's collected fee revenue
is compared. Deterministic; no simulation run.

Result: **H-015 SUPPORTED, direction as predicted, magnitude much smaller than
the first fixture suggested.**

| arm | one execution | ten executions | shortfall |
|---|---:|---:|---:|
| 100 USD, clip 150 001 | 75 | 70 | 5 (6.67%) |
| 50 000 USD, minimum clip | 25 000 | 25 000 | 0 |
| 50 000 USD, worst-case clip | 25 009 | 25 000 | 9 (0.036%) |

*The first row is not a campaign number and must not be quoted as one.* At the
campaign's own price level the charge reduces to `qty/40` quote units, so the
loss for a ten-way slice is `trunc((qty mod 40)/4) <= 9` units — derived first,
then confirmed exactly. At a round minimum clip nothing is lost at all.

What is nonetheless real, and is why this is recorded rather than dropped:
the effect is **one-directional**. Truncation is subadditive, so slicing can only
ever reduce the fee, never raise it. The shortfall is not paid by a counterparty
— it never reaches `ExchangeBalance.FeeRevenue` — so **no conservation check can
see it**: the money that is not charged is simply not moved. This is the same
structural blind spot RT-004 established for the conservation tracker, reached
from a different direction.

The fairness content: a taker does not choose the partition, the resting side
does. Two takers submitting identical orders at identical prices pay different
fees depending on whether the book in front of them was one large order or many
small ones.

*The latent boundary.* A fill pays no fee at all when its trade value is below
`10000/bps` quote units. At 5 bps and the ABC venue minimum of 0.001, that is
every price up to **19.00 USD** — measured, not derived by hand. ABC bootstraps
at 50 000 and CDF at 3 000, so **the free-fill regime is not reachable in the
campaign**. It is recorded because it is a property of the fee model and the
minimum order size together, and a future low-priced instrument would cross it
silently.

**E-014 — H-014, hidden orders and queue priority.**
Artifact: `tests/economic_audit_hidden_priority_test.go`.
Reproduce: `go test ./tests/ -run TestAuditHiddenOrder -v`.

Result: **H-014 SUPPORTED.** A hidden clip resting first took the entire
incoming fill (1 000 000 base units) while the displayed clip resting behind it
sold nothing, and the public ask level showed only the displayed clip
throughout. Hiding costs no queue position.

**Reachability: NOT EXERCISED.** No simulation constructs a non-Normal order,
and `BaseActor.SubmitOrderFull` — the only route from an actor to a visibility
other than `Normal` — has no callers anywhere in the tree, tests included. The
asymmetry is latent, so no campaign result depends on it. Recorded as RT-009 so
that enabling hidden orders is a deliberate act with a known consequence rather
than a silent one.

**H-016 — an order that is refused, or filled in part, leaves collateral
stranded or frees too much.**
Representation change: stop treating an order as an event and treat it as a
*reservation lifecycle* — reserve on admission, convert on fill, release on
termination — and ask whether every path through that lifecycle returns the
earmark to exactly what the surviving exposure requires. H-005 audited the
cancel paths. The admission-refusal and partial-fill paths are untested.

Why it is an actor-fairness question: `Available = Balances - Reserved`. An
earmark that outlives its order removes buying power an actor is entitled to;
one that is released too eagerly grants buying power its capital does not
support. Both are silent — `ReleasePerp` clamps at zero, so an over-release
leaves no trace, and RT-004 established that the conservation tracker cannot see
either, because a reservation is an earmark inside the balance and no total
moves.

Sub-cases, each a distinct exit from the lifecycle:
- (a) a fill-or-kill order that cannot be filled completely;
- (b) a post-only order that would cross;
- (c) an order refused for self-trade;
- (d) a resting order filled in part, then cancelled;
- (e) an immediate-or-cancel order filled in part, remainder killed;
- (f) an order refused for insufficient balance, which must not have consumed
  anything on its way to the refusal.

Predicted observable, recorded before running: the refusal paths (a, b, c, f)
return the earmark to its pre-order value exactly, because a refusal is the
easiest case to get right and the code rejects before or immediately after
reserving. The partial paths (d, e) are where a discrepancy is most likely,
because the release has to be computed from a remainder rather than from the
original order.
Falsifier for the hypothesis as a whole: every path restores the earmark to
exactly what the surviving exposure requires.
Mechanism family: state-machine leak.

**E-015 — H-016, reservation lifecycle on refusal and partial fill.**
Preregistered above. Artifact:
`tests/economic_audit_reservation_lifecycle_test.go`.
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Reproduce: `go test ./tests/ -run 'TestAuditRefusedOrder|TestAuditPartiallyFilled' -v`.

Result: **H-016 FALSIFIED WITHIN TESTED SCOPE.** Every exit returns the earmark
to exactly what the surviving exposure requires.

- fill-or-kill that cannot be filled completely (`FOK_NOT_FILLED`): earmark
  restored exactly.
- post-only that would cross (`POST_ONLY_WOULD_TAKE`): restored exactly.
- order larger than the balance behind it (`INSUFFICIENT_BALANCE`): restored
  exactly.
- resting order half filled: the earmark is exactly half of what the whole
  order held, measured against the idle baseline rather than assumed to be
  zero.
- cancelling that half-filled remainder: returns to the idle baseline exactly —
  not less, which would strand collateral, and not more, which would free
  collateral the fill had already converted into an asset.
- `GetAvailable` never exceeds the balance at any point.

*A prediction that was wrong, recorded as such.* The preregistration expected
the partial-fill paths to be the likely failure sites, "because the release has
to be computed from a remainder rather than from the original order". They are
in fact exact by construction: `releaseReserved` releases what was locked rather
than recomputing an approximation, and says so.

*A case that could not be run as written.* The self-trade arm was removed from
the table: `RejectSelfTrade` is declared in the reject vocabulary but **no code
path produces it**, so the order is accepted rather than refused. That is not a
defect — it is a different self-trade-prevention policy — and it became H-017.

**E-016 — H-017, self-trade prevention and the crossed book.**
Artifact: `tests/economic_audit_self_cross_test.go`.
Reproduce: `go test ./tests/ -run TestAuditSelfTradePrevention -v`.

Result: **H-017 FALSIFIED.** The exchange does not skip and rest. After the
matcher has consumed every crossable order belonging to other clients, any price
still crossing the remainder must belong to the incoming client, and
`cancelOwnCrossingQuotes` (`exchange/order_handling.go:1718`) withdraws those
resting quotes — the "cancel maker" self-trade-prevention mode. Its comment
states the reason this audit had hypothesised as a risk: "Resting it as-is would
display a crossed/locked book."

Four properties measured, each now pinned:

1. **The book is never left crossed and no wash trade prints.** A participant
   resting a sell at 90 and then buying at 110 ends with a bid at 110 and no
   ask; no trade prints and no balance moves.
2. **The cancelled quote's collateral is released, not stranded.** The base
   earmark returns to zero, available never exceeds balance, and the client
   stops tracking the withdrawn order.
3. **The owner is told.** A `ForcedCancelNotification` is delivered for each
   withdrawn quote. This is the failure mode this project has already been
   bitten by — an order removed without telling its owner leaves the actor
   believing it still rests, and its bookkeeping blocked indefinitely.
4. **In a deterministic order.** Three own asks placed from the highest price
   down, so price order and placement order disagree, are cancelled in
   *placement* order. The implementation collects targets by iterating a map and
   then sorts by order ID precisely for this reason; without the sort, map
   iteration would reach the evidence stream and the execution hash would stop
   being reproducible.

*Method note, recorded because it nearly became a false finding of exactly the
bug class above.* The first run of property 3 reported "3 quotes were withdrawn
but 0 cancellations were delivered". That was my harness: `enqueueResponse`
appends to an outbox that a separate goroutine drains, so a non-blocking read
races the delivery rather than observing it. Reporting it would have claimed a
silent-forced-cancel bug in code that delivers correctly. The test now collects
until the expected count arrives or a deadline passes, so a shortfall is a real
shortfall. The general lesson matches RT-006's: when the observable is produced
asynchronously, an instrument that samples once measures the scheduler.

**H-018 — an order outlives the instrument it rests on, or an unfilled
remainder outlives its order.**
The remaining exits from the reservation lifecycle after E-015 and E-016, chosen
because each one terminates an order through a path the *order itself* did not
initiate:
- (a) immediate-or-cancel filled in part, remainder killed by the venue;
- (b) a resting order on a dated instrument that reaches expiry with a
  settlement price available;
- (c) the same, but with **no** settlement price available, so the contract
  enters the settlement-pending state instead of settling.

Why (b) and (c) matter more than (a): an expiry is the one termination where the
*instrument* disappears. If a book is dropped while orders rest on it, the
earmark has nothing left to point at and no later cancel can reach it —
buying power removed permanently, with no event to explain it. Case (c) is
sharper still, because the contract does not settle: the code must cancel
resting orders on the first pass and must not cancel them again on the retries,
and `ReleasePerp` clamps at zero, so a double release would leave no trace.

Predicted observable, recorded before running: (a) is exact, for the same
reason E-015 found the partial paths exact. For (b) and (c) I expect the orders
to be cancelled and the earmark released — the settlement-pending branch visibly
sorts client IDs before cancelling, which is the signature of someone who has
already thought about this path — but I expect the **retry** case to be the
weakest link, because `CheckExpiries` is called repeatedly and only the first
pass is guarded.
Falsifier: every path releases the earmark exactly once and leaves it at the
idle baseline.
Mechanism family: state-machine leak, lifecycle boundary.

**E-017 — H-018, termination the order did not initiate.**
Preregistered above. Artifact:
`tests/economic_audit_lifecycle_termination_test.go`.
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Reproduce: `go test ./tests/ -run 'TestAuditImmediateOrCancel|TestAuditExpiryDoesNotStrand' -v`.

Result: **H-018 FALSIFIED WITHIN TESTED SCOPE**, including the retry case the
preregistration singled out as the likely weak link.

- **immediate-or-cancel, half filled and half killed**: the earmark returns to
  the idle baseline, the client tracks no orders, available never exceeds
  balance.
- **dated future reaching expiry with a settlement price**, order still resting:
  cancelled, earmark released to the idle baseline, client stops tracking it,
  perp available never exceeds perp balance, and a `ForcedCancelNotification`
  for that exact order ID is delivered.
- **the same with no settlement price**, so the contract enters
  settlement-pending rather than settling: identical outcome, and three further
  `CheckExpiries` passes leave the earmark unchanged and the perp balance
  non-negative.

The prediction that the retry path would leak was wrong, and the reason is
visible: `cancelClientOrdersOnBook` looks each order up in the live book and
skips it when it is no longer there, and `client.RemoveOrder` has already
removed it, so a second pass finds nothing to release. The pending branch also
sorts client IDs before cancelling, so the cancellation order does not depend on
map iteration.

*Method note.* The first attempt backdated the expiry and placed the order
afterwards; every placement was refused with `INSTRUMENT_EXPIRED`, which would
have "passed" a test that never exercised its own premise. The fixture now moves
a controllable clock forward instead, so the order is admitted while the
contract is live and the expiry happens under it. An expired-instrument refusal
is itself correct behaviour — it is simply not what H-018 asks about.

**Plateau, and the reframe it forces.** E-015, E-016 and E-017 are three
consecutive falsifications on the same surface: every exit from an order's life
returns collateral exactly, notifies the owner, and orders its evidence
deterministically. That is a plateau in the skill's sense — repeated valid runs
finding nothing — and the response is a representation change rather than more
cases in the same frame.

Every invariant tested so far has been **single-account, single-instrument,
single-book**. The untested surface is the coupling *between* books through a
shared account: cross-margin between `ABC-PERP` and a dated `ABC-FUT`, an option
exercised against a live hedge, collateral held in CDF while trading `ABC/CDF`,
a mark on one instrument feeding a liquidation on another. Those are exactly the
places where every per-book invariant can hold and the system still leaks,
because no single book owns the identity that would catch it. Registered as
H-019 and taken next.

**H-019 — the exposure that causes a deficit is not the exposure that gets
closed.**
Representation change: stop asking "is this position correctly margined" and ask
"when an account fails, which book pays?" Margin is aggregated across every book
the account touches — `buildAccountMarginProfile` walks all books in sorted
symbol order, adds each position's unrealized PnL to equity, and fails the whole
profile closed if any sibling exposure is settlement-pending. Liquidation is not
aggregated the same way: `CheckLiquidations(symbol, ...)` is entered per symbol
from that symbol's mark update, and when the account breaches, it liquidates only
the positions **in that symbol**.

Mechanism: an account long a small `ABC-PERP` and long a large `ABC-FUT` whose
mark collapses is under maintenance because of the future. If a mark update
arrives on the perp, the perp position — the healthy one — is closed, and the
future, which caused the deficit, is untouched. Worse, the account is now
invisible through that door: the next `CheckLiquidations("ABC-PERP", ...)`
returns early at `len(positions) == 0`, so the breach can only be found again
through a mark update on the future itself.

Why this is an actor-fairness question: which of an actor's positions is
confiscated depends on which book happened to receive a mark update, not on
which exposure caused the loss. Two actors with identical portfolios and
identical losses lose different positions depending on the tick order of
instruments they do not control. And an account left holding the loss-maker with
no margin is a deficit the insurance fund has not absorbed, because bankruptcy is
only detected inside `liquidate`.

Predicted observable, recorded before running: the perp position is closed, the
future position survives at full size, and the account remains below maintenance
afterwards.
Falsifier: liquidation reaches the sibling exposure, or the account is above
maintenance once the trigger symbol's positions are closed.
Mechanism family: cross-book coupling, aggregation asymmetry.

**E-018 — H-019, which book pays when an account fails.**
Preregistered above. Artifact:
`tests/economic_audit_cross_book_liquidation_test.go`.
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Reproduce: `go test ./tests/ -run TestAuditCrossBookLiquidation -v`.

Fixture, hand-derived rather than read back from the margin engine: client 1
holds 900 USD of perp cash, is long 1 `ABC-PERP` at 100 and long 10 `ABC-FUT` at
100. The sibling's mark collapses to 5; the perp does not move.

    sibling uPnL = 10 * (5 - 100)   = -950 USD
    perp    uPnL =  1 * (100 - 100) =    0 USD
    equity       = 900 - 950 + 0    =  -50 USD

Result: **H-019 SUPPORTED as predicted, and then bounded by its own follow-up.**

| step | liquidations | `ABC-PERP` | `ABC-FUT` | perp cash | fund |
|---|---:|---:|---:|---:|---:|
| mark update on the healthy book | 1 | **0** | 10 | 900 | 0 |
| second check on the same book | 1 | 0 | 10 | 900 | 0 |
| mark update on the losing book | 2 | 0 | **0** | 0 | **-50** |

The perp position — sitting exactly at its entry price, carrying no loss at all —
is the one confiscated, to answer a deficit caused entirely by the future. The
future is untouched. A second check on the perp then finds nothing, because
`CheckLiquidations` returns early at `len(positions) == 0`, so between the two
ticks the account carries 10 units of unmargined exposure and is invisible
through the door it was found by.

**The severity bound, established by attacking my own result.** The
preregistration stopped at "the account remains below maintenance afterwards",
which would have implied a permanent hole. It is not permanent: a mark update on
the losing book does reach the exposure, closes it, and the insurance fund
absorbs exactly the hand-derived 50 USD. So the defect is not lost solvency —
it is *which* position is taken, and a window of unmargined exposure between
ticks. Reporting the first half alone would have overstated it.

**What remains, and why it is a fairness question.** Margin is aggregated across
every book (`buildAccountMarginProfile` walks all books in sorted symbol order
and sums each position's unrealized PnL into equity); liquidation is not
(`CheckLiquidations` closes only the trigger symbol's positions). Because of
that asymmetry, which of an actor's positions is confiscated depends on which
book happened to tick first, not on which exposure caused the loss. Two actors
with identical portfolios and identical losses can lose different positions
depending on the arrival order of marks on instruments neither controls. An
actor that hedges across two books is exposed to having the hedge taken and the
loss left open.

Recorded as RT-011. Classification **CORRECT BUT SURPRISING / specification
question**, not a bug to fix here: partial-close ordering and cross-book seizure
are scientific economics and the owner's to decide. Conservation is intact and
no value is created — the fund absorbed the shortfall exactly.

**H-020 — a position on a settlement-pending contract makes the whole account
unliquidatable.**
Invert the lens used for RT-011. There the question was which book pays; here it
is whether any book can be made to pay at all.

Mechanism: `buildAccountMarginProfile` fails the entire profile closed when any
sibling exposure sits on a settlement-pending contract — "Retained pending
exposure is not an economic zero. No valid mark exists, so fail the whole account
profile closed instead of allowing active sibling risk to ignore it." That
reasoning is sound for *measuring* risk. But the caller in `CheckLiquidations`
treats the error as a diagnostic, not as a breach: it calls
`reportPriceUnavailable` and `continue`s to the next client. Failing closed on
the measurement therefore fails *open* on the action.

A dated contract enters that state whenever it reaches expiry with no settlement
price, and the retry policy is `expiryUnavailableRetryForever`, so it can stay
there indefinitely while its positions are retained.

Consequence if true: an account holding any position on such a contract cannot be
liquidated on any symbol, however far underwater its other positions are, until
the pending contract settles. Two actors with identical losing positions get
different treatment — one is closed out, the other is not — and the difference is
whether they happen to hold an expired contract awaiting a price. That is an
unearned advantage granted by the venue, and it is the strongest form of the
unfairness this campaign is looking for, because it converts a data-availability
gap into an economic privilege.

Predicted observable, recorded before running: with a deeply underwater
`ABC-PERP` position and any position on a settlement-pending `ABC-FUT`,
`CheckLiquidations("ABC-PERP", ...)` performs no liquidation; once the future
receives a settlement price and settles, the same call liquidates.
Falsifier: the account is liquidated while the sibling is pending, or the profile
error is escalated rather than skipped.
Mechanism family: cross-book coupling, fail-open on the action.

**E-019 — H-020, a settlement-pending sibling and the whole account.**
Preregistered above. Artifact:
`tests/economic_audit_pending_immunity_test.go`.
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Reproduce: `go test ./tests/ -run 'TestAuditPendingSibling|TestAuditSuspendedLiquidation' -v`.

Fixture: 100 USD of perp cash, long 10 `ABC-PERP` at 100, plus one unit of an
`ABC-FUT` that reaches expiry with no settlement price and so enters
settlement-pending. The perp mark halves to 50, which alone is
`100 + 10*(50-100) = -400 USD` of equity.

Result: **H-020 SUPPORTED, then substantially bounded by the follow-up.**

While the sibling is pending: `liquidations=0`, the perp position stands at its
full 10 units, and cash is untouched. `buildAccountMarginProfile` fails closed
because no valid mark exists for the pending contract; `CheckLiquidations`
treats that error as a diagnostic, calls `reportPriceUnavailable`, and continues
to the next client. Failing closed on the *measurement* fails open on the
*action*.

**The bound, and it changes the finding materially.** The account is not
privileged, it is **frozen**: `order_handling.go:557` refuses every order from a
client with settlement-pending exposure, and the test confirms the refusal
carries `ACCOUNT_SETTLEMENT_PENDING`. The actor cannot add risk — and cannot
shed it either, since a closing order is refused on the same grounds. Once the
contract settles the account is liquidated normally and the fund absorbs the
deficit. My preregistration called this "immune to liquidation on every other
book", which overstates it; the accurate word is suspended.

**What the suspension costs, isolated in a second fixture.** The position rides
the market while nobody can close it, so the deficit is set by the price
available when the freeze lifts rather than by the price at the breach:

| position closes at | fund absorbs | hand-derived |
|---|---:|---|
| 50, the breach price | -400 USD | `10*(50-100) = -500` against 100 cash |
| 25, after the market moved | -650 USD | `10*(25-100) = -750` against 100 cash |

The 250 USD difference is what the delay transfers from the defaulter to the
insurance fund. A liquidation exists precisely to cap that growth.

**Why it is still a fairness question.** The defaulter's downside is capped at
zero cash by the bankruptcy write-down, so the tail beyond that is the fund's.
An actor frozen through a falling market therefore holds the recovery and not
the tail, while an actor without a pending contract is closed out at the breach.
Two identical losing positions, two different outcomes, and the difference is
whether one of them happened to hold an expired contract awaiting a price.

**Competing readings, both legitimate.** (a) The caller is wrong: the comment on
the profile says fail closed, and skipping the client fails open on the action;
an unmeasurable account should be escalated, not passed over. (b) The caller is
right: you cannot size a liquidation whose total exposure you cannot value, so
declining to act and freezing the account is the conservative choice, and the
freeze is the mitigation. This audit does not decide between them — that is
scientific economics. Recorded as RT-012 with the measured cost attached so the
decision can be made on numbers.

**Reachability not established.** The state requires a dated contract reaching
expiry with no settlement price, and `expiryUnavailableRetryForever` shows the
condition is anticipated and unbounded in duration. Whether the campaign's own
configurations ever produce it is *not* tested here and should not be assumed
from this experiment.

**H-021 — funding cost depends on how many accounts a position is spread over.**
New mechanism family for this campaign: a *recurring* transfer between actors,
rather than a one-off event at a lifecycle boundary. Funding is the purest
actor-versus-actor flow in the model — longs pay shorts every interval — so if
it is not partition-invariant, the unfairness compounds every interval instead
of happening once.

Mechanism: `settleFunding` computes each position's payment as
`TryMulDiv(positionValue, rate, 10000)` **per position**, then aggregates a
client's hedge legs. Integer truncation is subadditive, so the same total
exposure split across N accounts pays `sum_i trunc(v_i·r/1e4)`, which is at most
`trunc((sum_i v_i)·r/1e4)` and generally less. The code already recognises that
the two sides need not net: `netExchangeFlow` is accumulated explicitly,
validated, and routed to exchange revenue with the comment that on real
exchanges this is the insurance fund's residual. So the residual is *accounted*.
The question this asks is whether it is *neutral*.

Why it is a fairness question and not bookkeeping: if splitting reduces what a
payer pays, an actor able to open several accounts pays less funding for the
same exposure than one who cannot, every interval, forever. The counterparty
does not gain it — the exchange residual absorbs the difference — so it is not a
transfer between traders but a discount on an obligation, available only to
whoever fragments.

Predicted observable, recorded before running: with a positive funding rate,
total funding paid by N split longs is **less than or equal to** that paid by
one long of the same size, the deficit appears in the exchange residual, and it
is bounded by roughly one quote unit per extra account. Symmetrically, a split
*receiver* should receive less, which would make fragmentation good for payers
and bad for receivers.
Falsifier: the two arms pay exactly the same, which would mean funding is
computed on an aggregate or the rounding is compensated.
Mechanism family: quantization asymmetry, recurring transfer.

**E-020 — H-021, funding under account partition.**
Preregistered above. Artifact:
`tests/economic_audit_funding_partition_test.go`.
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Reproduce: `go test ./tests/ -run TestAuditFundingIsNotInvariant -v`.

Result: **H-021 SUPPORTED in direction, and its "unconditional advantage"
framing FALSIFIED by the symmetric arm.**

| split side | one account | eight accounts | exchange residual |
|---|---:|---:|---:|
| payer (long) | pays 8801 | pays **8800** | 0 → **-1** |
| receiver (short) | receives 8801 | receives **8800** | 0 → **+1** |

Fragmentation moves the rounding away from the fragmented side's cash flow in
*both* directions. It reduces the magnitude of whatever that side pays or
receives, so it helps a payer and hurts a receiver. The preregistration
predicted the payer half and guessed the receiver half; both are now measured
rather than assumed. It is therefore not a free-money mechanism: an actor
choosing to fragment must know which side of the funding flow it is on, and the
choice reverses when the rate does.

The residual is booked, not lost — `netExchangeFlow` is accumulated explicitly,
validated, and routed to exchange revenue — so this is a transfer, not a leak.
What is new is that the residual's *sign* is set by which side is more
fragmented, which makes it a population property rather than a wash.

*Two invalid fixtures before the valid one, recorded because either would have
produced a confident "falsified".* The first used a mark of 100 USD, where
`AbsMulDiv(size, mark, precision)` divides the size by ten and absorbs the leg
differences before the bps step ever sees them; it reported equality and would
have closed H-021 as falsified. The second scanned leg sizes in steps of one,
which the same division also swallows: 0 differences in 40 offsets. Setting the
mark equal to `BTC_PRECISION`, so position value *is* the raw size and the bps
step is the only truncation, gives 31 differing partitions in the same 40-offset
scan. A null result from an instrument that cannot resolve the effect is not a
null result.

**Magnitude, stated before any structural claim.** One quote unit in 8801, or
0.011%, bounded by about one unit per extra account per settlement. Economically
negligible. It is recorded for a different reason: it is the same mechanism
family as RT-008 in fees, and two independent instances make the generalisation
worth stating — **every per-item integer charge in this system is
partition-dependent**, because every one of them truncates per item and
truncation is subadditive. Fees and funding are the two found so far; margin and
settlement use the same `MulDiv` idiom and have not been checked from this
angle.

**H-022 — the partition-dependence pattern, swept.**
RT-008 (fees) and RT-013 (funding) are the same mechanism twice. The
generalisation to test: *every per-item integer charge in this system is
partition-dependent, because each truncates per item and truncation is
subadditive.* The instrument is the search unit here, not another single case —
a sweep over the charge sites, reporting for each whether it is
partition-invariant and what the deviation is bounded by.

Charge sites found by grepping the `MulDiv`/`MulBps` idiom: `PercentageFee`
(RT-008), `settleFunding` (RT-013), maintenance and warning margin in
`buildAccountMarginProfile`, option premium and settlement in
`instrument/option.go`, and **collateral interest** in
`exchange/collateral_interest.go`.

Collateral interest is singled out before measuring, because reading it suggests
a different *kind* of failure from the previous two. It computes
`interest = TryMulDiv(borrowed, CollateralRate, collateralInterestDenominator)`
per client per asset, with
`collateralInterestDenominator = 365*24*3600*10000/60 = 5_256_000_000`, and then

    if interest <= 0 { continue }

At the default 500 bps that makes `interest = borrowed / 10_512_000`, so a debt
below 10_512_000 quote units — **105.12 USD at `USD_PRECISION`** — rounds to zero
and is skipped. The charge runs once per simulated minute, so the exemption does
not accumulate into a later payment: it isforgiven, every minute, forever.

Why this would be categorically worse than RT-008 and RT-013: those move at most
one quote unit per item, a rounding transfer. This one would forgive the *entire*
charge below a threshold, so a borrower who splits one loan across enough
accounts pays **nothing at all** rather than slightly less. That is a free-money
mechanism at full economic scale rather than a sub-unit artefact.

Predicted observable, recorded before running: one account borrowing an amount
above the threshold accrues interest per minute; the same total split into
sub-threshold pieces accrues exactly zero. Effect size to be measured as an
annualised percentage of principal, not as raw units, so it can be compared
against the 5% the rate is supposed to charge.
Falsifier: the split arm accrues the same interest as the whole arm, or the
sub-threshold case accrues a residual that is carried forward.
Mechanism family: quantization asymmetry, threshold exemption.

**E-021 — H-022, the partition sweep, and what it found in collateral interest.**
Preregistered above. Artifact:
`tests/economic_audit_interest_threshold_test.go`.
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Reproduce: `go test ./tests/ -run 'TestAuditCollateralInterest|TestAuditDeliveredInterestRate' -v`.

Result: **H-022 SUPPORTED, and collateral interest is a categorically worse
instance than the two that motivated the sweep.**

`chargeCollateralInterestLocked` computes
`interest = TryMulDiv(borrowed, CollateralRate, 5_256_000_000)` once per
simulated minute per client per asset, then `if interest <= 0 { continue }`. At
the default 500 bps that is `borrowed / 10_512_000`, so a debt below
**10_512_000 quote units = 105.12 USD** is charged nothing, and the shortfall is
not carried forward. The bisection confirms the boundary exactly: the largest
interest-free debt is 10_511_999 units and the first charged debt is 10_512_000.

Partition arm, one simulated day (1440 charges) on 1000 USD of debt:

| held as | interest collected | annualised |
|---|---:|---:|
| one account of 1000 USD | 12 960 | **4.730%** |
| ten accounts of 100 USD | **0** | **0.000%** |

**The generalisation that does not need an actor to open several accounts.** The
delivered rate is a function of the size of the debt:

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

A 200 USD borrower pays 2.6% where a 1 000 000 USD borrower pays 5.0%. Every
actor faces this, with no special access: **the cost of leverage depends on how
much is borrowed, in a way the configuration does not state.** That is directly
a relative-performance distortion between actors, because a carry or basis
strategy's economics are set by its funding cost.

Why this is worse in kind than RT-008 and RT-013. Those move at most one quote
unit per item — a rounding transfer. This forgives the *whole* charge below a
threshold and delivers a materially wrong rate for two decades of principal
above it, every minute, without accumulating a residual.

**Reachability.** Borrowing is enabled in the campaign
(`ex.EnableBorrowing` at `simulations/multivenue/sim.go:2878`, with limits of
20 000 000 USD and 20 000 ABC), and `ChargeCollateralInterest` runs as a
deterministic phase job (`exchange/exchange.go:1347`), so the mechanism is live.
The *magnitude* in campaign runs depends on the debt sizes actors actually
carry, which this experiment has **not** measured; at large debts the delivered
rate is within 0.2 bps of configured. This must not be reported as either
material or immaterial for the campaign without that measurement.

*Fixture note.* The first two runs reported zero interest for every principal,
including 1000 USD, which read as "the threshold swallows everything". It was
the fixture: the 500 bps default is applied inside `ConfigureAutomation`, not in
the constructor, so an exchange built with `NewExchange` alone carries
`CollateralRate == 0` and charges nothing at all. The campaign reaches the
default the same way the corrected fixture now does. Worth recording in its own
right as a trap for any code that builds an exchange without configuring
automation.

**E-022 — RT-014 at campaign scale: the measurement the finding was missing.**
Artifact: `research/tools/interestscan/main.go` (Go, per the project's rule that
data processing stays out of Python).
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Run: `research/configs/clock-control-5h-101.json`, seed 607, 30 simulated
minutes, `-log-mode full`. Development configuration; no holdout was used.
Reproduce: build `cmd/multivenue`, run as above, then
`go run research/tools/interestscan/main.go -dir <logdir>`.

The charge is `floor(borrowed·rate/denominator)` per minute, so an observed
amount `A` bounds the debt that produced it and therefore bounds the delivered
rate from below by `A/(A+1)`. That is enough to decide severity without
reconstructing every account's debt path.

Result:

| asset | charged per minute | occurrences | implied debt (raw units) | delivered ≥ |
|---|---:|---:|---|---:|
| ABC | 1 | 76 | [10 512 000, 21 024 000) | **50.0%** |
| ABC | 2 | 18 | [21 024 000, 31 536 000) | **66.7%** |
| ABC | 3 | 16 | [31 536 000, 42 048 000) | **75.0%** |

76 borrow events, 110 interest charges, 160 quote units collected in total.
Every debt in the run sits in the three lowest charge buckets, so **the
delivered rate is roughly 250–430 bps against a configured 500**. Borrowers are
systematically under-charged by 15–50% of their interest, every minute, for the
whole run. RT-014 is therefore not a threshold curiosity at campaign scale; it
is the operating regime.

**The part the USD framing missed.** All borrowing in this run is in **ABC**,
not USD, and the threshold is denominated in *raw asset units*: 10 512 000 units
regardless of what a unit is worth. At `BTC_PRECISION` that is 0.105 ABC, which
at the 50 000 bootstrap is about **5 256 USD** of interest-free debt — fifty
times the 105.12 USD ceiling that the same constant imposes on a USD loan. Two
actors with the same dollar leverage pay materially different rates depending on
which asset they borrowed. That is a per-asset inequity produced by a single
shared constant, and it was invisible from the USD-only fixture in E-021.

**Scope.** One config, one seed, 30 simulated minutes. Debts may grow over a
five-hour run and push accounts into higher buckets where the delivered rate
approaches the configured one; that is not measured here and must not be assumed
in either direction.

*Fourth instrument near-miss, and the most dangerous so far.* The first scan
reported **110 charges totalling zero quote units**, which reads as "the campaign
pays no interest at all" — a dramatic finding, and false. The venue logger wraps
every event payload one level deeper than the emitting struct suggests
(`data.payload.amount`, not `data.amount`), so the parser silently read zero for
every record. The raw line settled it in one look. Recording it because the
failure mode is now a pattern in this audit: RT-006's expectation model,
RT-010's asynchronous outbox, RT-013's price conversion, and now a schema
mismatch — four instruments that returned a confident wrong answer, three of
them nulls. **Read one raw record before trusting any aggregate over it.**

**H-023 — the borrow limit is computed on cash the account has already lost or
already committed.**
`validateCrossMarginCollateral` builds `totalAssetValue` by summing
`client.PerpBalances` and `client.Balances` at oracle prices, subtracts
`client.Borrowed`, and limits the new borrow against that net equity. The
reasoning it states is careful — negative balances subtract, and the limit is
against net equity precisely so each borrow cannot enlarge the base for the
next.

What the sum does not contain is anything about **positions**. An account long a
perp whose mark has fallen carries an unrealized loss that has not touched its
cash balance, so the collateral valuation still sees the full pre-loss cash. The
same is true of `Reserved`: an earmark sits inside `Balances`, so capital already
committed to resting orders is counted as free collateral.

Why this is an actor-fairness question rather than a modelling choice: the
liquidation engine *does* value positions — `buildAccountMarginProfile` adds
every position's unrealized PnL to equity. Two parts of the same system
therefore disagree about what the account is worth, and the disagreement points
one way: the borrow gate is the more generous of the two. An actor that is
already losing can lever up on equity the risk engine knows it no longer has,
and the counterparties to that new exposure are the ones carrying it.

Predicted observable, recorded before running: an account with cash `C` and an
unrealized loss `L` (with `L` small enough that it is not yet liquidatable) can
borrow up to `factor·C` rather than `factor·(C−L)`. A borrow that should be
refused once the loss is counted will succeed.
Falsifier: the borrow is refused, or the permitted amount shrinks by the
unrealized loss.
Mechanism family: cross-book coupling, valuation disagreement.

**E-023 — H-023, what the borrow gate can see.**
Preregistered above. Artifact:
`tests/economic_audit_borrow_valuation_test.go`.
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Reproduce: `go test ./tests/ -run TestAuditBorrowLimitIgnores -v`.

Result: **H-023 SUPPORTED on both counts.** Collateral factor 0.5, 1000 USD of
cash, limit found by bisecting the gate itself.

| account | economic equity | admitted borrow |
|---|---:|---:|
| no position | 1000 USD | 500.00 USD |
| long 5 at 100, mark 60 (unrealized -200) | 800 USD | **500.00 USD** |

An engine that counted the loss would admit 400. The account borrows 25% more
than its equity supports, and the risk engine already knows the equity is gone:
`buildAccountMarginProfile` adds every position's unrealized PnL. Two parts of
the same system disagree about what the account is worth, and the borrow gate is
the more generous of the two.

The second arm is sharper, because it is the same capital counted twice rather
than a stale valuation. A reservation is an earmark *inside* `PerpBalances`, and
the gate sums `PerpBalances`:

| account | cash | reserved | available | admitted borrow |
|---|---:|---:|---:|---:|
| resting bid for 90 ABC at 100 | 1000 | 900 | **100** | **500.00 USD** |

An account with 100 USD actually available borrows 500. The same capital backs
the resting order and the loan at once — collateral reuse, and it needs no
special access.

**Reachability.** Borrowing is enabled in the campaign with auto-borrow on both
wallets, and the 30-minute run in E-022 produced 76 borrow events, so the gate is
live and exercised.

**Disposition: owner decision, not fixed.** How much leverage an account may take
is scientific economics. What the tests do is pin the behaviour in both
directions — they now fail if the gate starts counting either the loss or the
earmark — so the disagreement between the two valuations cannot change silently.

**Competing reading, recorded.** Positions are margined separately, so one could
argue the cash is genuinely unencumbered and the position's own margin is the
control. That argument does not survive the second arm: `PerpReserved` *is* the
position and order margin, it sits inside the balance the gate sums, and the gate
therefore counts the margin as collateral for a new loan.

**E-024 — RT-014 over the full five hours, and a correction to E-022.**
Same config and seed as E-022 (`clock-control-5h-101.json`, seed 607), run for
its designed 5 simulated hours instead of 30 minutes. 6.6 GB of logs, scanned
with the same `research/tools/interestscan`.

| | E-022 (30 min) | E-024 (5 h) |
|---|---:|---:|
| borrow events | 76 | 978 |
| interest charges | 110 | 2 455 |
| collected | 160 | 26 805 |
| charge buckets observed | 1–3 | **1–51** |
| aggregate delivered, lower bound | — | **91.6% (458 bps of 500)** |

**Correction, stated plainly.** E-022 concluded that "the delivered rate is
roughly 250–430 bps against a configured 500" and that "at campaign scale this
is the operating regime". That over-states it. The 30-minute window sampled the
warm-up, when debts are small and therefore sit in the lowest charge buckets.
Over the full run the aggregate delivered rate is bounded below by
`collected/(collected + charges) = 26 805/29 260 = 91.6%`, so **≥ 458 bps of the
configured 500** — an aggregate under-collection of at most 8.4%, not 15–50%.

**What survives, and it is the part that was the finding.** The delivered rate
still depends on the size of the debt, and the dependence is visible in the
distribution rather than in the aggregate: **513 of 2 455 charges (20.9%) fall in
buckets 1–3**, where the delivered rate is bounded below by only 50.0%, 66.7%
and 75.0%. Small debts pay materially less than large ones for the same nominal
rate, every minute, and no configuration states this. The threshold result of
E-021 is unchanged and exact.

**Population note.** Only three clients borrow at all in this run (12, 13 and
14, taking 10 711, 8 365 and 7 729 quote units of the 26 805). The distortion is
therefore concentrated rather than population-wide in this configuration, which
also means a per-actor performance comparison involving those three is where it
would show up.

**Method note.** The correction is the direct consequence of extrapolating a
30-minute window to a 5-hour claim. The measurement was right; the scope
sentence attached to it was not. A run length chosen for convenience is not a
sample of the regime the campaign actually reports on.

**H-024 — the borrow gate prices collateral at a constant, forever.**
RT-015 established that the borrow gate and the risk engine disagree about
*what* to value. This asks whether they also disagree about *at what price*.

`buildAccountMarginProfile` and `MarkedAccount` both value derivative exposure
through `riskMark`, which reads each instrument's stored funding mark and falls
back to a live book reference only before the first mark update. The borrow gate
does not use `riskMark` at all: it reads `bm.Config.PriceSource`, and the
campaign supplies `exchange.NewStaticPriceOracle(collateralPrices)` with
`"ABC": mvBootstrapPrice` — the 50 000 bootstrap, fixed at construction
(`simulations/multivenue/sim.go:2862`).

If that oracle never updates, then borrowing power derived from ABC collateral
is pinned to the price ABC had before the simulation began. In a falling market
an actor keeps full borrowing power against collateral that has lost value; in a
rising one it is denied borrowing power it has earned. Either way leverage is
decoupled from the market the same actor is trading, and the decoupling grows
with the price excursion.

Note the second-order term: `CollateralFactors` names only `"USD": 1`, so ABC
falls through to the 0.75 default. A haircut of 25% covers a 25% adverse move
and no more.

Predicted observable, recorded before running: with an account holding ABC
collateral, halving the live market price of ABC leaves the admitted borrow
unchanged. Then, from a real run, the excursion of the live ABC mid away from
50 000 bounds how large the mispricing actually gets.
Falsifier: the admitted borrow tracks the live price.
Competing reading to keep alive: a static collateral oracle deliberately breaks
the circularity of valuing collateral with the same market the borrower is
moving, which is a defensible choice for a research venue. If so the finding is
a documentation gap plus a fairness consequence, not a defect.
Mechanism family: valuation disagreement, stale reference.

**E-025 — H-024, the collateral oracle against the market.**
Preregistered above. Artifacts:
`tests/economic_audit_collateral_oracle_test.go` and
`research/tools/pricerange/main.go`.
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Reproduce: `go test ./tests/ -run TestAuditBorrowCollateralIsPriced -v`, and
`go run research/tools/pricerange/main.go -dir <logdir> -symbol ABC-USD`.

**Mechanism: SUPPORTED, exactly and decisively.** 10 ABC of collateral, USD
factor 1:

| market price of ABC | oracle price | admitted borrow |
|---:|---:|---:|
| 50 000 | 50 000 | 500 000 USD |
| **25 000** | 50 000 | **500 000 USD** |
| 25 000 | 25 000 | 250 000 USD |

A 50% fall in ABC leaves borrowing power completely unchanged, at twice what the
collateral is then worth. The gate never consults `riskMark`; it reads
`BorrowingConfig.PriceSource`, and the campaign supplies
`exchange.NewStaticPriceOracle` with ABC pinned to the 50 000 bootstrap
(`simulations/multivenue/sim.go:2862`). ABC is absent from `CollateralFactors`,
so it falls through to the 0.75 default — a 25% haircut, which covers a 25%
adverse move and no more.

**Magnitude: small in this configuration, and drifting — which is why the
partial numbers are reported as partial.** Two reads of the same run in
progress:

| ABC-USD, trades scanned | low vs oracle | high vs oracle | widest |
|---:|---:|---:|---:|
| 144 101 (partial) | −0.24% | +0.04% | 0.24% |
| 257 411 (partial) | −0.48% | +0.04% | 0.48% |
| **544 835 (complete 5 h)** | **−1.06%** | **+0.04%** | **1.06%** |

The band is not stationary and not symmetric: across the whole run ABC's high
never leaves +0.04% while its low walks to −1.06%, so the excursion grows with
run length in one direction rather than oscillating around the reference. It
roughly doubled with each doubling of the sample.

The second collateral asset behaves differently. CDF-USD, against its own 3 000
bootstrap, stays inside **0.17%** over 431 985 trades and moves *both* ways
(−0.10% / +0.17%). So the drift is a property of ABC in this configuration, not
of the venue.

At 1.06% against a 25% haircut the static oracle is accurate by a factor of
about twenty-four, so **the mechanism is a latent hazard in this configuration,
not a live mispricing.** A claim that it stays latent in a less anchored
configuration is not supported by this measurement and is not made.

**The condition under which it would bite, stated so it can be checked rather
than assumed:** any configuration in which ABC's excursion from its bootstrap
approaches the 25% haircut. The anchored control config does not; a stress
configuration (`research/configs/v005-stress-perp.json` and its siblings) is
where this should be re-measured before any run of that family is treated as
economically faithful. This audit has not measured those.

**Competing reading, kept alive.** A static collateral oracle deliberately
breaks the circularity of valuing collateral with the very market the borrower
is moving, which is defensible for a research venue and avoids a
liquidation-spiral artefact. Under that reading the finding is a documentation
gap plus a bounded fairness consequence, not a defect. Nothing in the code
states the choice, which is why it is recorded.

*Fifth instrument error, caught before it produced a number.* The price tool
first searched for an event named `"trade"`. The evidence writes `"Trade"`,
capitalised, and the `Trade` payload carries **no symbol** — the book a trade
belongs to is the file it is written in. Both mistakes would have yielded a
confident "no trades found" or a silently empty band. Checking one raw line
before trusting the aggregate caught it, which is now the standing rule from
E-022.

**H-025 — the three valuations of an account rank in care opposite to their
authority.**
RT-015 and RT-016 each found the borrow gate disagreeing with the risk engine.
This closes the frame by adding the third valuation and asking a structural
question rather than another instance question: *does the system's care about
pricing an account increase with the consequence of the decision it feeds?*

The three:

1. **`MarkedAccount`** (`exchange/valuation.go`) — telemetry and scoring. Its own
   comment says it is "intended for research/risk telemetry rather than order
   admission" and that "the caller supplies every conversion into one reporting
   asset because the venue does not invent an FX graph".
2. **`buildAccountMarginProfile`** — liquidation. Values positions through
   `riskMark`, which reads each instrument's stored funding mark and fails closed
   when no valid mark exists.
3. **`validateCrossMarginCollateral`** — leverage. Reads
   `BorrowingConfig.PriceSource`, which the campaign fills with a static oracle
   pinned to the bootstrap price.

Predicted ordering, recorded before checking the campaign's caller: care
*decreases* as authority increases. The scoring path, which moves no money,
should be the most careful; the borrow gate, which decides how much leverage an
actor may take, the least.

Discriminating observations: (a) what mark the campaign supplies to
`MarkedAccount`, and whether it carries provenance and a staleness bound; (b)
whether the same account, at the same instant, is valued differently by
`MarkedAccount` and by the borrow gate, and by how much.
Falsifier: the scoring path is as loose as the borrow gate, or the borrow gate
carries an equivalent staleness discipline.
Mechanism family: valuation disagreement, structural.

**E-026 — H-025, the three valuations side by side.**
Preregistered above. Artifact:
`tests/economic_audit_valuation_triad_test.go`.
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Reproduce: `go test ./tests/ -run TestAuditTheSameAccountIsValued -v`.

Result: **H-025 SUPPORTED, including the predicted ordering.** The prediction was
recorded before reading `populationValuationSpec`, which turned out to be the
most disciplined of the three.

| valuation | decision it feeds | price it uses | staleness discipline | provenance |
|---|---|---|---|---|
| `MarkedAccount` via `populationValuationSpec` | scoring, moves no money | live two-sided ABC/USD mid | bounded window, fails closed on a non-positive mark | records `markSource`, distinguishing `two_sided_ABC_USD_mid` from `recent_…` and from `bootstrap_manifest` |
| `buildAccountMarginProfile` | liquidation | instrument's stored funding mark via `riskMark`, live-book fallback only before the first update | fails closed on a settlement-pending sibling | none recorded |
| `validateCrossMarginCollateral` | how much leverage an actor may take | static oracle pinned to the bootstrap constant | **none — the concept does not exist on this path** | none |

**Care decreases as authority increases.** The path that moves no money records
where its price came from and refuses to report on a stale one. The path that
decides how much leverage an actor may take reads a constant fixed before the
simulation started.

Measured on one account at one instant, 10 ABC held, market at 25 000 against a
50 000 bootstrap:

- scoring equity at the live mark: **250 000 USD**
- scoring equity at the bootstrap mark: 500 000 USD
- borrow admitted against the same 10 ABC: **500 000 USD — 2× the account's live
  equity**

**Magnitude discipline, applied prospectively.** That 2× uses a 50% price move to
make the mechanism visible. The measured ABC excursion in the control
configuration is **1.06%** (E-025), so the campaign-scale gap between the
scoring valuation and the borrowing valuation is about one percent, not a
factor of two. The extreme is a demonstration, not a claim about the campaign,
and it is labelled as such in the test.

**What is new here beyond RT-015 and RT-016.** Those were two instances of one
subsystem disagreeing with another. This is the structural statement: the
disagreement is not random, it is ordered, and it is ordered the wrong way
round. A reviewer looking for where valuation discipline is weakest should look
where the consequences are largest, and in this system that is exactly where the
discipline is absent. Recorded as RT-017.

**H-026 — borrowed spot exposure is never forcibly unwound.**
The frame that produced RT-015 to RT-017 asked which valuation governs. This
asks the prior question: *which exposures are governed at all?*

The exchange has exactly two liquidation entry points:
`CheckLiquidations` (perp and dated futures) and
`CheckPositionMarginerLiquidations` (options). Both walk *positions*. Spot debt
lives in `Client.Borrowed` / `Client.BorrowedSpot` and is not a position, so
neither entry point can see it. The borrow gate refuses to extend *new* credit
once equity is gone, but refusing new credit is not unwinding old exposure.

Mechanism: with `AutoBorrowSpot: true` — which the campaign sets — a participant
short of an asset at settlement has it borrowed for them. That is a short spot
position financed by the venue. If the borrowed asset then appreciates, the debt
grows against a fixed pile of sale proceeds, and there is no code path that
closes it.

Why it is an actor-fairness question: an actor holding a losing *derivative* is
closed out and its deficit is charged to the insurance fund, a visible and
bounded event. An actor holding a losing *borrowed-spot* position is closed out
by nothing. Two actors with the same economic short therefore face different
rules depending on which instrument expressed it, and the second one's downside
is carried by the venue with no event marking it.

Predicted observable, recorded before running: after a large adverse move, the
account's marked equity is negative, and invoking both liquidation entry points
leaves the debt, the balances and the insurance fund unchanged.
Falsifier: some path closes the position, charges the fund, or margin-calls the
account.
Mechanism family: ungoverned exposure.

**E-027 — H-026, what governs borrowed spot exposure.**
Preregistered above. Artifact: `tests/economic_audit_spot_debt_test.go`.
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Reproduce: `go test ./tests/ -run TestAuditBorrowedSpotExposure -v`.

Result: **H-026 SUPPORTED. Nothing governs it.**

An account with 100 000 USD borrows 1 ABC at 50 000 and sells it — a
venue-financed short, which is what `AutoBorrowSpot` produces when a participant
is short of an asset at settlement. ABC then quadruples to 200 000.

| | equity |
|---|---:|
| at entry, ABC at 50 000 | +100 000 USD |
| after ABC reaches 200 000 | **−50 000 USD** |

Both liquidation entry points are then invoked — `CheckLiquidations` for perps
and dated futures, `CheckPositionMarginerLiquidations` for options:

| | before | after |
|---|---:|---:|
| ABC debt | 100 000 000 | 100 000 000 |
| USD cash | 15 000 000 000 | 15 000 000 000 |
| insurance fund | 0 | 0 |

Nothing moves. The account is 50 000 USD underwater, no path closes it, and no
event records that the venue is carrying the loss.

**The asymmetry, which is the finding.** An actor whose *derivative* goes bad is
closed out and its deficit is charged to the insurance fund — a bounded, logged,
attributable event, as E-018 and E-019 measured. An actor whose *borrowed spot*
goes bad is closed out by nothing and the shortfall is recorded nowhere. Two
actors holding the same economic short face different rules according to which
instrument expressed it, and the venue silently carries the second.

The reason is structural rather than a missing check: both liquidation entry
points walk **positions**, and spot debt is not a position — it lives in
`Client.Borrowed` and `Client.BorrowedSpot`. The borrow gate does refuse *new*
credit once equity is gone, but refusing new credit is not unwinding old
exposure.

**Scope and what is not established.** `AutoBorrowSpot: true` is set by the
campaign and E-022 observed real `auto_spot` ABC borrows, so the mechanism is
live. Whether any account in a real run actually reaches negative equity this
way is **not measured here**, and the finding must not be read as saying it does.
Voluntary `RepayMargin` exists and ordinary trading retires these debts; what is
absent is the forced unwind when equity goes negative.

*Fixture note.* The sale of the borrowed ABC is injected by writing the balances
directly rather than crossing a book. That is an unrecorded mutation of the kind
E-007 invalidated an earlier experiment for — but the assertions here are the
invariance of debt, cash and fund across the liquidation calls, none of which the
injection can affect. It would matter if the claim were about conservation; it is
not.

**H-027 — a perfectly hedged option position does not end flat at expiry.**
The last named surface from the original unaudited list, and the one that tests
the venue's treatment of *hedgers* specifically rather than of directional
actors.

Mechanism to test: expiring contracts and perps do not get their price from the
same place. `refreshDerivativeMarks` resolves **one** underlying observation per
tick through `derivativeUnderlyingPrice`, which reads
`bookReferencePrice(underlyingSymbol)` — the spot book — and hands that same
number to every expirable via `ObserveSettlement`. So an option and a dated
future on the same underlying settle against an identical price, which is
correct and deliberate. The perp is marked separately, through
`UpdateFundingRate(index, mark)` fed by the simulation.

If the perp's mark is anything other than that same spot reference, then an
actor who is short an option and delta-hedged in the perp has the two legs
valued against two different prices at the instant of expiry, and books a
phantom profit or loss equal to the basis — despite having taken no net
exposure. The venue would then be creating and destroying value specifically for
participants who hedge, which is the opposite of what a hedge is for.

Predicted observable, recorded before reading where the simulation sources the
perp mark: an exactly delta-hedged account's total equity changes across the
expiry boundary by the basis between the spot reference and the perp mark, and
by zero when the two coincide.
Falsifier: the hedged account's equity is unchanged across expiry for any basis,
which would mean both legs are settled against one price.
Mechanism family: valuation disagreement, applied to a hedge rather than to an
account.

**E-028 — H-027, hedged option positions at expiry.**
Preregistered above. Artifact:
`tests/economic_audit_shared_settlement_test.go`.
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Reproduce: `go test ./tests/ -run TestAuditExpiringContractsShareOne -v`.

Result: **H-027 FALSIFIED, and the reasoning was corrected twice before any
number was produced.**

*First correction, from reading rather than running.* The hypothesis assumed that
an option hedged in the perp books a phantom PnL at expiry because the two legs
are priced differently. They are — the option settles to the spot reference while
the perp is marked at its own book (`exchange.go:1704`, mark = the perp's own
book, index = the configured spot index). But that is **genuine basis risk that
the hedger holds**, not a venue artefact: the perp position is not settled, it
stays open at its own mark, and an actor who hedges an option with a perp really
does own the basis. Not a defect, and the hypothesis as written was wrong about
the economics rather than about the code.

*What is actually load-bearing.* `UpdateDerivativeMarks` resolves **one**
underlying observation per tick through `derivativeUnderlyingPrice` and hands
that same number to every expirable via `ObserveSettlement`. So a calendar hedge
— an option and a dated future on the same underlying, expiring together —
settles both legs against an identical price and nets exactly. Measured: one
observation, future settles at 10 000 000, option at 10 000 000, both equal to
the spot mid rather than to either derivative's own book.

Nothing else asserts this. If the shared observation were replaced by
per-instrument sampling, calendar hedges would quietly stop netting and **only
hedgers would pay for it** — the failure would be invisible to any directional
participant and to every existing test. Pinned as RT-019.

*Second correction, and a new shape of instrument error.* The first run reported
that the future got no settlement observation while the option, from the same
call, got one. That looked like a real asymmetry between the two expirable
types. It was the fixture: `NewExpiringFutures` takes no underlying argument, so
a bare construction leaves `Underlying` empty and
`derivativeUnderlyingPrice` falls through to the configured index, which does not
publish per-contract symbols. The campaign never constructs one that way — the
listing scheduler sets it (`instrument/listing.go:97`) and is the only path that
lists dated futures.

The five earlier instrument errors were all about *reading* the system wrong.
This one is about *building* it wrong: constructing a domain object directly
instead of through the factory the system actually uses, and then measuring
behaviour the system never exhibits. **Prefer the construction path production
uses; a bare constructor can leave a field the whole lifecycle depends on.**

**H-028 — one actor can move the number that governs every other actor.**
New lens, and the one the campaign's framing most directly asks for: every
hypothesis so far has asked whether the venue's *accounting* is fair. This asks
whether the venue's *inputs* can be moved by a participant. Funding is charged on
position value at the mark, and liquidation triggers on equity measured at the
mark. If the mark is computed from a book any actor can post into, then one
actor's order changes what every other actor pays and when they are closed out.

The venue is not naive about this. `NewExchangeWithConfig` sets
`autoAnchorMarks` whenever the caller supplies no mark calculator — which the
campaign does not — and its comment states the exact attack: "a margined book
marked at its own mid lets liquidations trade into the very price that triggers
them (self-feeding cascade)." There is also an EMA (`MarkPriceEMAWindow`,
default 10) and a band (`MarkPriceBandBps`, default 600, so ±6%).

So the question is not whether a defence exists but what it **covers**. The
comment says books with no resolvable index "keep the mid", and the campaign's
index provider publishes exactly four symbols — `ABC/USD`, `ABC-PERP`,
`CDF/USD`, `ABC/CDF` (`sim.go:2439`). Dated futures and options are listed
dynamically by the listing scheduler and are not among them. Dated futures are
margined, so their mark drives margin and liquidation for anyone holding them.

Predicted observable, recorded before measuring: `ABC-PERP` is auto-anchored and
its mark does not follow a lone order posted into its book; a dated future is
not anchored, and a single minimum-size order at an improved price moves its
mark, and therefore moves the margin of every account holding that contract.
Falsifier: dated futures are anchored too, or the EMA and band absorb a
one-order move entirely.
Mechanism family: manipulable input, coverage gap in an existing defence.

**E-029 — H-028, how far one actor can walk the mark.**
Preregistered above. Artifact:
`tests/economic_audit_mark_manipulation_test.go`.
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Reproduce: `go test ./tests/ -run TestAuditHowFarOneActorCanWalk -v`.

**The predicted coverage gap was falsified; the defence is better than the
hypothesis assumed.** `ensureAnchoredMarkCalcs` installs a
`ClampedEMAMarkPrice` on **every** margined book whose instrument has an
underlying *or* for which an index provider exists — which, with the campaign's
provider present, is all of them, dated futures included. And the calculator
fails **closed**: when the index itself is unavailable it returns an error rather
than falling back to the manipulable mid. There is no unanchored margined book.

**What the measurement then found is a bound, not an immunity.**

| step | value |
|---|---|
| index | 50 000 |
| honest two-sided market | 49 000 / 51 000 |
| one minimum-size bid posted inside the spread, never trading | book mid → **+1.90%** |
| mark after 200 passes | **+1.90% of index** |
| clamp | ±3.00% of index (600 bps band, halved) |
| maintenance margin rate | 500 bps |

One unit of base quantity — `1`, not one lot — resting inside the spread and
never trading moves the mark by 1.90%. The clamp is never reached, so it is the
mid itself, not the band, that sets this number; the EMA converges to the full
basis in far fewer than 200 passes.

**Why the number matters.** Maintenance margin is 500 bps. A 1.90% mark move is
**38% of the entire maintenance buffer**, and the clamp permits up to 3.00%,
which is **60% of it**. An account sitting near maintenance can therefore be
pushed materially closer to liquidation — or away from it — by a participant
risking one unit of size. Funding is charged on position value at the same mark,
so the same quote also changes what every other holder pays that interval.

**What this is and is not.** It is a property of the rules, not an observed
exploit: nothing in the campaign's actor population does this, and this audit
has not looked for it in run evidence. The mid is a plain mid — `GetMidPrice`,
which the anchored calculator calls directly — so the displacement is
independent of the quoting actor's size. A quantity-weighted mid exists in the
codebase (`WeightedMidPriceCalculator`) and is not what the anchor uses.

Recorded as RT-020. Disposition is the owner's: weighting the mid by size,
narrowing the band, or requiring a minimum resting quantity to influence the mark
are all defensible and all change scientific economics.

*Seventh instrument error.* The first version posted the manipulating bid
*through* the ask, at index + 20 000 against an ask at index + 10. That is not a
resting manipulation, it is a marketable order, and it left the book crossed so
`GetMidPrice` failed and the calculator returned the bare index — reporting a
mark move of exactly 0.0000% and reading as a complete defence. The fixture now
quotes inside the spread. **A manipulation fixture must post a quote the venue
would actually leave resting; a crossed book measures the error path, not the
mechanism.**

**H-029 — the anchor the mark trusts has no staleness bound and degenerates on
thin participation.**
RT-020 established that the mark is anchored to the index and clamped around it.
That makes the index the load-bearing number, so the same lens now points one
level down: *can a participant move the anchor, and does the anchor forget?*

The campaign's index is `spotIndexProvider` in `consensus` mode: a median over
`venueMids[symbol][venueID]`, populated from each venue's automation tick with
`observeVenueMid(symbol, venue.ID, mid)` guarded by
`if mid, ok := venue.Exchange.TwoSidedMidPrice(symbol); ok`. Two properties
follow from the data structure rather than from any policy:

1. **The map has no expiry.** A venue that stops producing a two-sided mid stops
   *updating* its entry; it does not lose its vote. Its last observation keeps
   voting in the median indefinitely. Contrast the scoring path, which takes an
   explicit `maxStaleness` and distinguishes `two_sided_..._mid` from
   `recent_..._mid` in the evidence — the same asymmetry RT-017 recorded, now
   one level deeper.
2. **A median is only robust while the sample is odd and plural.** With three
   venues an attacker must move two. With two, `mids[len/2]` selects the
   **upper** of the pair, so a single venue quoting up carries the index. With
   one, the median is that venue.

Predicted observable, recorded before running: with two contributing venues the
index equals the higher mid, not their average; with three it resists one venue
entirely; and a venue whose book goes empty keeps its last mid in the consensus
for as long as the run continues.
Falsifier: entries expire, or the two-venue case takes a low/average rather than
the upper observation.
Mechanism family: manipulable input, one level below the mark.

**E-030 — H-029, the anchor's own robustness.**
Preregistered above. Artifact:
`simulations/multivenue/economic_audit_index_consensus_test.go`.
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Reproduce: `go test ./simulations/multivenue/ -run TestAuditIndexConsensusRobustness -v`.

Result: **H-029 SUPPORTED on both parts.**

| observations | index |
|---|---|
| one venue at 100 | 100 |
| two venues at 100 and 200 | **200** — the upper, not the average |
| three venues at 100, 101, 100 000 | 101 — one venue cannot carry it |
| two live at 400, one silent last seen at 100 | 400 |
| **one live at 400, two silent last seen at 100** | **100** |

The last row is the finding. `venueMids[symbol][venueID]` is overwritten, never
aged, and the campaign's call site only writes when
`venue.Exchange.TwoSidedMidPrice(symbol)` succeeds. A venue that goes one-sided
therefore stops updating without losing its vote, and with three venues two
silent ones outvote the only live market. **The index can publish a price no
venue is currently showing.**

`TwoSidedMidPrice` needs both sides, so "silent" here does not mean a dead
venue — an active market quoting only bids already qualifies.

**Where this sits.** RT-020 showed the mark is anchored to the index and clamped
±3% around it, which makes the index the load-bearing number rather than the
mark. One level down, that number has no staleness bound at all. The scoring
path, which moves no money, takes an explicit `maxStaleness` and records in its
evidence whether the mark it used was fresh or `recent_`. **This is RT-017's
inverse ordering again, one level deeper: the anchor with the most authority has
the least memory discipline.**

Median-of-three is a real defence and the third row shows it working. The
weakness is not the median, it is that membership is permanent.

**Reachability not established.** Whether any venue's `ABC/USD` book actually
goes one-sided during a campaign run, and for how long, is **not measured here**.
The mechanism is structural; its frequency is an empirical question this
experiment did not ask. Recorded as RT-021.

**H-030 — the index's stale-vote weakness is not merely structural.**
RT-021 established the mechanism — `venueMids` is overwritten, never aged, and a
venue that cannot produce a two-sided mid stops updating without losing its vote
— but explicitly did not measure how often a venue is in that state. Without
that number the finding cannot be ranked against the others, and the discipline
this audit has already had to apply twice (E-022's over-stated 30-minute
extrapolation, RT-016's bounded excursion) says the number comes before the
severity claim.

The observable: each venue publishes periodic `BookSnapshot` events per symbol
carrying `bids` and `asks`. A snapshot with either side empty is a moment when
`TwoSidedMidPrice` would fail and that venue would contribute nothing new to the
consensus while its previous observation kept voting.

Predicted observable, recorded before measuring: one-sidedness is rare on
`ABC/USD` at the venue with the maker population and more common on the thinner
books, and the fraction of *simultaneous* two-venue silence — the case where the
live market is outvoted — is far smaller than any single venue's silence rate.
Falsifier: every venue is two-sided in essentially every snapshot, which would
make RT-021 structural only and reduce its severity to a latent note.
Scope limit stated in advance: snapshots are periodic, so this measures a
*sampled* silence rate and not the instantaneous one, and it cannot see silence
that begins and ends between two snapshots.

**E-031 — H-030, how often the stale-vote configuration actually occurs.**
Preregistered above. Artifact: `research/tools/booksidedness/main.go`.
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Run: `clock-control-5h-101.json`, seed 607, full 5 simulated hours, `-log-mode full`.
Reproduce: `go run research/tools/booksidedness/main.go -dir <logdir> -symbol ABC-USD`.

A venue is "silent" for the index when its book is one-sided, because
`TwoSidedMidPrice` then fails and the campaign's call site does not update that
venue's entry. Over 18 000 snapshot instants:

| symbol | north | central | south | ≥2 silent at once | all silent | **exactly 2 silent (the harmful case)** |
|---|---:|---:|---:|---:|---:|---:|
| `ABC/USD` | 0.40% | 0.40% | 0.40% | 3 (0.02%) | 3 (0.02%) | **0** |
| `CDF/USD` | 0.64% | 4.76% | 3.83% | 71 (0.39%) | 4 (0.02%) | **67 (0.37%)** |
| `ABC/CDF` | 7.35% | 8.67% | 8.36% | 456 (2.53%) | 16 (0.09%) | **440 (2.44%)** |

**The distinction that decides the severity.** RT-021's harm needs *exactly* two
silent venues and one live one: then two stale observations outvote a live
market. When **all** venues are silent nobody updates, the median is entirely
stale, but no live market is being contradicted — that is a different and much
weaker condition. The last column is the difference between the two.

**Result, and it downgrades RT-021 where it matters most.** On `ABC/USD` — the
book that anchors the perp mark and therefore margin, liquidation and funding —
the harmful configuration occurred **zero times in 18 000 instants**. Every
multi-venue silence there was total silence. RT-021's consequence for the margin
system is, in this configuration, **not merely rare but absent**.

On the thin cross book `ABC/CDF` it is common: **2.44%, about 440 instants**, or
roughly one per 41 seconds of simulated time. `CDF/USD` sits between at 0.37%.
So the mechanism is live, but on the books whose index feeds cross-asset pricing
rather than on the one that governs margin.

**A second thing the table shows.** Venue silence is not symmetric: on `ABC/CDF`
central is one-sided 8.67% of the time against north's 7.35%, and on `CDF/USD`
central is 4.76% against north's 0.64%. The venues contribute unequally to the
consensus that prices everyone, which is an actor-fairness input in its own right
and was not something this experiment set out to measure.

**Scope, stated in advance and unchanged.** Snapshots are periodic, so this is a
*sampled* silence rate; silence beginning and ending between two snapshots is
invisible. The per-venue snapshot counts (18 018–18 076) slightly exceed the
18 000 distinct instants, so a few instants carry more than one snapshot per
venue; the effect on the percentages is below their reported precision, and the
"exactly 2 silent" column is derived as `≥2 silent` minus `all silent` rather
than counted directly.

RT-021 is updated with these numbers rather than left as a structural note.

**H-031 — which venue an actor is placed on changes its outcome.**
E-031 surfaced this without looking for it: venue silence is not symmetric. On
`CDF/USD` central's book is one-sided 4.76% of the time against north's 0.64%,
and on `ABC/CDF` the three venues run 7.35%, 8.36% and 8.67%. The venues are not
equivalent environments.

Why that is the campaign's problem rather than a curiosity: the configuration
places the same participant *counts* on all three venues, so every role class
exists three times over. Any statement of the form "strategy X outperformed
strategy Y" is therefore an average over three environments — and if those
environments differ materially, part of the measured difference between two
classes is the venue they happened to be quoted into, not the strategy.

Representation change: stop comparing *classes* and compare *one class against
itself* across venues. That holds strategy fixed and varies only the environment,
which is the clean design the campaign's own layout already provides for free.

The evidence exists: `ParticipantAccountSnapshot` records every participant's
marked account with its role, its venue and the marks it was valued at, at
named lifecycle phases, and the run writes `terminal-outcome.json`.

Predicted observable, recorded before measuring: within at least one role class,
terminal marked equity differs across venues by materially more than it differs
between participants of the same class on the same venue. The within-venue spread
is the natural yardstick — if between-venue differences sit inside it, venue
assignment is not a confounder and H-031 is falsified.
Falsifier: for every class, the between-venue spread is comparable to or smaller
than the within-venue spread.
Mechanism family: environment confound, actor-fairness at the population level.

**E-032 — H-031, does venue placement change an actor's outcome?**
Preregistered above. Artifact: `research/tools/venueeffect/main.go`.
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Run: `clock-control-5h-101.json`, seed 607, 5 simulated hours, `-log-mode none`
(the population artifact is written regardless of log mode).
Reproduce: `go run research/tools/venueeffect/main.go -file <logdir>/greeks.json -detail`.

Design: the campaign places the same participant counts on all three venues, so
every role class exists three times over. Hold the strategy fixed, vary only the
environment, and compare the between-venue spread of class means against the
within-venue spread of individuals.

**Result: H-031 SUPPORTED, and the metric needed correcting on the way.**

| class | n/venue | between-venue spread | within-venue spread | ratio | **spread as % of result** |
|---|---:|---:|---:|---:|---:|
| `triangle_arb` | 2 | 1 119 237 | 3 819 | 293× | **8.23%** |
| `elastic_supplier` | 8 | 41 586 | 191 | 218× | 0.39% |
| `dated_carry_arb` | 2 | 10 818 | 90 | 120× | 0.41% |
| `parity_arb` | 2 | 10 529 | 146 | 72× | — |
| `option_value_taker` | 4 | 16 033 | 454 | 35× | — |
| `latent_liquidity` | 6 | 98 086 | 7 407 | 13× | — |
| `fixed_distance_maker` | 8 | 46 486 | 1 860 149 | **0.02×** | — |
| `noise_flow` | 6 | 1 221 324 | 32 347 493 | **0.04×** | — |

Per-venue means for the largest effect: `triangle_arb` returns **+14 258 332**
on north, +13 393 986 on south and +13 139 095 on central.

**The metric correction, which changes how the table should be read.** The ratio
column is inflated, and not by a small amount. Same-class actors on the same
venue are near-clones — these are deterministic strategies with near-identical
parameters — so the within-venue spread is tiny by construction, and dividing by
it produces large numbers regardless of whether the venue effect is
economically meaningful. `elastic_supplier`'s 218× is a venue spread of 0.39% of
its own result. **The load-bearing column is the last one**, and by that measure
only `triangle_arb`, at 8.23%, shows a venue effect of a size that could change
a conclusion.

**Competing reading, and it is strong.** A venue effect is *expected* for
venue-local strategies. A triangular arbitrageur trades three books on its own
venue; its result should depend on that venue's book quality. This is not
evidence of a defect in the venue's economics — the venues really are different
environments, which E-031 already showed from the other direction (`ABC/CDF`
one-sidedness of 7.35% on north against 8.67% on central).

**What it is evidence of is methodological, and that is the finding.** Any
comparison of the form "class X outperformed class Y" is an average over three
environments that are not equivalent. For classes near the bottom of the table
that does not matter — `fixed_distance_maker`'s venue term is 2% of its
individual variation. For `triangle_arb` the venue term is 8.23% of the result
itself, so a comparison involving it that does not control for venue is carrying
an environment term of that size. Recorded as RT-022.

*Eighth instrument error, caught before reporting.* The first version grouped by
the numbered role (`elastic_supplier_3`) rather than the role class, so every
group held exactly one participant per venue, the within-venue spread was zero by
construction, and every ratio printed as `inf`. The tool now strips the index,
and classes that genuinely have one participant per venue — `perp_maker` — are
reported as "no yardstick" instead of being given a ratio against zero.

**H-032 — RT-022's venue effect is a matching-rule effect, not a venue
mystery.** Self-correction, raised by attacking E-032 rather than extending it.

E-032 reported that a class's result varies across venues and framed the venues
as "genuinely different environments". Checking the configuration *after*
reporting — which is the wrong order and is recorded as such — shows the
heterogeneity is **deliberate and explicit**:

| venue | matching rule | funding interval |
|---|---|---|
| north | `price_time` | 28 800 s (8 h) |
| central | `pro_rata` | 3 600 s (1 h) |
| south | `pro_rata` | 7 200 s (2 h) |

So a venue effect is not a discovery; the campaign built three different venues
on purpose, and a class-level number aggregated across them is *designed* to mix
three matching rules and three funding intervals. E-032's magnitude measurement
stands; its framing overstated the novelty and is corrected below.

The configuration also hands over a decomposition E-032 did not use. North is
the only price-time venue; central and south are both pro-rata and differ only in
funding interval. Therefore:

- `central` vs `south` isolates the **funding interval** (1 h vs 2 h),
- `north` vs the mean of the two pro-rata venues isolates the **matching rule**,
  confounded only by north's longer funding interval.

Predicted observable, recorded before recomputing: for `triangle_arb` the
matching-rule axis is several times larger than the funding-interval axis, and
the sign is consistent across the liquidity-taking classes — price-time
advantages an actor that wants a determinate queue position, pro-rata dilutes it.
Falsifier: the two axes are comparable in size, or the sign of the matching-rule
axis differs across classes with similar execution needs, either of which would
mean the split explains nothing and the venue term is genuinely unattributed.

**E-033 — H-032, decomposing the venue effect, and a correction to RT-022.**
Preregistered above. Artifact: `research/tools/venueeffect/main.go`.
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Run: `clock-control-5h-101.json`, seed 607, 5 simulated hours.

**First, the correction to E-032, and the process error behind it.** E-032
framed the venues as "genuinely different environments" as though that were a
discovery. The configuration says so explicitly, and I read it only *after*
reporting:

| venue | matching rule | funding interval |
|---|---|---|
| north | `price_time` | 8 h |
| central | `pro_rata` | 1 h |
| south | `pro_rata` | 2 h |

The heterogeneity is deliberate. E-032's magnitudes stand; its framing implied an
unnoticed confound where the campaign had made a design choice. **Read the
configuration before characterising what a measurement means, not after.**

**Second, the decomposition — which fails, and the failure is the finding.**
North is the only price-time venue and central/south are both pro-rata, so
`central − south` isolates the funding interval while `north −
mean(pro-rata)` was intended to isolate the matching rule.

| class | rule axis | funding axis | mean | rule as % |
|---|---:|---:|---:|---:|
| `triangle_arb` | +991 791 | −254 891 | 13 597 138 | +7.29% |
| `imbalance_maker` | +122 266 | −124 211 | −483 679 | +25.28% |
| `option_dealer` | +12 854 | +1 206 | 61 391 | +20.94% |
| `abc_cdf_spot_maker` | +415 725 | **−3 279 751** | −2 100 770 | +19.79% |
| `noise_flow` | −584 037 | **+1 221 324** | −9 649 395 | −6.05% |
| `vanna_volga_desk` | −10 479 | −693 | −34 451 | −30.42% |
| `fixed_distance_maker` | −33 264 | −26 444 | −558 857 | −5.95% |

**H-032 MIXED, and both halves of the prediction fail.** The prediction was that
the rule axis would dominate and that its sign would be consistent across
liquidity-taking classes. Neither holds: for `abc_cdf_spot_maker` and
`noise_flow` the *funding* axis is several times the rule axis, and the rule
axis's sign splits across classes that are not obviously different in execution
needs — `abc_cdf_spot_maker` gains on price-time while `fixed_distance_maker`
loses.

**What the numbers actually establish.** North differs from the pro-rata venues
in **both** rule and funding interval, so the "rule axis" is contaminated by an
8 h-versus-1.5 h funding difference. The one clean contrast — `central` versus
`south`, identical rules, 1 h versus 2 h funding — shows a funding term that is
frequently *larger* than the quantity being attributed to the rule. The
decomposition therefore cannot separate the two effects, and neither can any
single run of this configuration.

**The finding is that the venue design confounds matching rule with funding
interval.** Attributing a measured venue effect to either is unsupported, and the
campaign's three venues do not contain the cell — price-time with a short funding
interval — that would break the confound. Recorded as RT-023, and RT-022 is
amended to say that its environment term is real but **unattributable**, not that
it is a matching-rule effect.

**Scope.** One configuration, one seed. Whether the per-class signs are stable
across seeds is not measured, and with a single run they could as easily be
sampling noise as structure. That check is the natural next experiment and this
one does not substitute for it.

**H-033 — the venue effect is structure, not sampling noise.**
The discriminating test E-033 said it owed. RT-022 reports an 8.23% venue spread
for `triangle_arb` and RT-023 reports per-class rule- and funding-axis terms,
both from **one seed**. With a single run those numbers cannot be told apart from
sampling variation, and this audit has already had to downgrade two findings for
exactly that kind of over-reach.

Design: rerun the identical configuration at three further seeds and compare the
per-class axis terms. Seeds 608, 609 and 610 — development seeds; the scientific
holdouts are not touched.

The discriminating quantity is **sign stability**. A venue term that is real
structure should keep its sign across seeds for a given class; one that is
sampling noise should flip. Magnitude agreement is a weaker and less reliable
signal at n=4, so the sign is the test.

Predicted observable, recorded before running: `triangle_arb`'s rule-axis term
stays positive in all four seeds, because a 7.29% effect against a 0.03%
within-venue spread is far outside plausible noise for a deterministic strategy.
The small-magnitude classes — `option_flow` at −0.00%, `round_trip` at 0.32% —
flip freely, which would confirm the sign test is measuring something rather than
returning "stable" for everything.
Falsifier: `triangle_arb`'s rule axis changes sign across seeds, which would mean
RT-022's headline number is noise and must be withdrawn rather than merely
qualified.
Mechanism family: replication.

**E-034 — H-033, replication across four seeds.**
Preregistered above. Artifact: `research/tools/venueeffect/main.go`.
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Runs: `clock-control-5h-101.json`, seeds **607, 608, 609, 610**, 5 simulated
hours each, `-log-mode none`. Development seeds; no scientific holdout touched.

**Result: H-033 SUPPORTED for the headline class, and the built-in control
worked.**

`triangle_arb`'s rule-axis term, seed by seed: **+991 791, +800 529, +653 236,
+792 955**. Positive in all four, same order of magnitude, never close to zero.
RT-022's venue term is **structure, not sampling noise**, and can be stated as a
replicated result rather than a single observation.

The prediction also said the small-magnitude classes should flip freely,
otherwise the sign test would be returning "stable" for everything. They do —
`option_flow` (−++−), `latent_liquidity` (+++−), `dated_carry_arb` (+++−),
`parity_arb` (+++−), `elastic_supplier` (+++−) all change sign. The test
discriminates.

Rule-axis sign stable across all four seeds: `triangle_arb` (++++),
`imbalance_maker` (++++), `spot_maker` (++++), `option_dealer` (++++),
`metaorder_trader` (++++), `perp_maker` (++++), `fixed_distance_maker` (−−−−),
`vanna_volga_desk` (−−−−).

**A result that corrects RT-023's evidence.** RT-023 cited
`abc_cdf_spot_maker` as the clearest case of the funding term swamping the rule
term. Its *rule* axis turns out to be the least stable quantity in the table —
+415 725, then −1 921 747, −2 172 357, −365 331 — so that class was the wrong
witness. RT-023's conclusion is unchanged, because the confound is structural
rather than empirical, but the example is replaced below.

**What replication newly identifies.** `central` versus `south` holds the
matching rule fixed and varies only the funding interval, so it is the one
*clean* contrast — and for several classes its sign is stable across all four
seeds:

| class | 607 | 608 | 609 | 610 | mean |
|---|---:|---:|---:|---:|---:|
| `noise_flow` | +1 221 324 | +323 478 | +2 139 099 | +2 816 095 | **+1 624 999** |
| `abc_cdf_spot_maker` | −3 279 751 | −186 314 | −6 065 150 | −8 189 817 | **−4 430 258** |
| `imbalance_maker` | −124 211 | −249 168 | −66 494 | −211 055 | −162 732 |
| `perp_maker` | +11 432 | +74 820 | +157 292 | +39 387 | +70 733 |
| `round_trip` | −42 | −221 | −107 | −38 | −102 |

So the **funding-interval effect is both identified and replicated**: shortening
the interval from 2 h to 1 h is worth on average +1.6 M to `noise_flow` and
−4.4 M to `abc_cdf_spot_maker`. That is the one venue factor this configuration
can attribute, and it is larger than the unattributable rule term for both
classes.

**Standing corrections.** RT-022 is upgraded from "single seed, could be noise"
to replicated across four seeds. RT-023 is unchanged: replication does not break
a confound, and north remains the only venue that is both price-time and
long-funding. What is new is that the *other* axis is clean, stable and larger
than the confounded one for the classes where it matters.

**Scope.** Four seeds, one configuration, 5 simulated hours each. Sign stability
at n=4 is a weak test individually; it is meaningful here because the
small-magnitude classes visibly fail it.

**H-034 — the population's gains and losses do not close against the venue's
take.**
New lens after the venue thread completed: every invariant this audit has
checked is per-account or per-book. This one is **population-level**. E-032's
table showed `triangle_arb` ending +13.6 M while `elastic_supplier` ends
−10.6 M and `noise_flow` −9.6 M. Someone is paying for that, and whether the
totals close is a question no per-account check can answer.

The identity, per venue and in the report asset:

    Σ_participants (terminal equity − initial equity)
      + venue fee revenue + venue insurance fund
      = revaluation of the population's net inventory

The left side is what the accounts and the venue ledger say. The right side is
the only legitimate source of a non-zero total: participants are net long the
base asset, so a price change lifts or drops everyone's marked equity at once
without anyone trading. `ParticipantAccountSnapshot` records the `marks` each row
was valued at precisely so this term can be separated — its comment says so.

`StrictPopulationAccounting` is already on, but it only requires that every
participant *has* an initial and terminal marked account. It is a completeness
guarantee, not a sum check, and no closure check exists in `analysis/`.

Predicted observable, recorded before running: the residual is small relative to
gross flows and has the sign of the mark drift, since RT-016 measured ABC moving
about 1% over five hours. A residual that is large relative to gross flows, or
whose sign contradicts the mark drift, is unaccounted value at the population
level.
Falsifier: residual within a fraction of a percent of gross participant flow, and
consistent with the measured mark change.
Scope stated in advance: the reported residual mixes true revaluation with any
accounting gap, and this experiment separates them only by magnitude and sign,
not exactly. It is a screen, not a proof.
Mechanism family: population-level closure.

**E-035 — H-034, does the population's ledger close?**
Preregistered above. Artifact: `research/tools/populationclosure/main.go`.
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Run: `clock-control-5h-101.json`, seed 607, 5 simulated hours, `-log-mode none`.
Reproduce: `go run research/tools/populationclosure/main.go -file <logdir>/greeks.json`.

Result: **H-034 FALSIFIED. The population ledger closes.**

| venue | participant net | venue take | residual | implied ABC | per head |
|---|---:|---:|---:|---:|---:|
| central | −373 509 674 | 1 115 991 | −372 393 683 | 708 579 | **8 239** |
| north | −371 756 050 | 1 136 102 | −370 619 948 | 705 204 | **8 200** |
| south | −371 024 227 | 1 101 923 | −369 922 304 | 703 877 | **8 185** |

Mark drift over the run: ABC **−1.051%**, CDF +0.083%, USD flat.

Dividing each venue's residual by the mark drift gives the base-asset inventory
that residual implies: **8 185 to 8 239 ABC per participant**, against a
configured maker endowment of `10 000 * mvBasePrecision`
(`simulations/multivenue/sim.go`, `mmBalances`). Not every one of the 86
participants per venue is endowed at that level — takers and noise traders hold
less or none — so an implied average slightly below 10 000 is exactly what a
closed ledger should produce. The residual is revaluation of a large net-long
inventory, not unaccounted value.

**The instrument error, and it is the ninth.** The first version of this tool
reported the residual as a percentage of gross participant flow and printed
**−82%**, which reads as catastrophic. Gross participant flow is itself dominated
by the same revaluation, so normalising by it makes any revaluation look total.
The correct normaliser is the mark drift, because dividing by it yields a
quantity — implied inventory — that can be checked against a configured number.
**Do not normalise by a quantity that contains the effect being measured.**

**What the experiment leaves open, as preregistered.** The screen separates
revaluation from an accounting gap only by magnitude and consistency, not
exactly. A gap smaller than the ~2% difference between the implied 8 200 and the
endowed 10 000 would be invisible to it. Closing that would need per-account
inventory in the artifact, which `MarkedAccountSnapshot` does not carry — it
reports `SpotEquity`, `PerpCashEquity`, `DerivativeUnrealized` and
`OptionMarketValue`, all already valued at marks.

Recorded as RT-024: a bounded no-violation result plus a reusable screen.

**H-035 — the population ledger closes *exactly*, not merely to within the
revaluation estimate.**
RT-024 left a stated hole: its screen separates revaluation from an accounting
gap only by magnitude, so a gap smaller than the ~2% between the implied 8 200
ABC per head and the endowed 10 000 is invisible to it. That is a wide door, and
the class of defect it hides — value created or destroyed at the population level
— is exactly the class no per-account invariant can see.

Representation change: **hold the marks fixed instead of estimating their
effect.** `MarkedAccount(clientID, spec)` is exported and the caller supplies the
spec, so the terminal population can be valued a second time at the *initial*
marks. Revaluation is then identically zero by construction and the residual is
whatever the accounting actually leaves over:

    residual_fixed = Σ (terminal equity at INITIAL marks − initial equity)
                     + fee revenue + insurance fund

This needs a harness rather than an artifact, because the second valuation has to
happen while the terminal state is still live. `Sim`, `NewSim`, `Run` and
`MarkedAccount` are all exported, so the harness is a reader of the same public
surface the campaign uses, not a fork of it.

Predicted observable, recorded before running: `residual_fixed` is a small
multiple of the smallest quote unit per venue — not zero, because fees and
settlement truncate per item (RT-008, RT-013, RT-014 all leave sub-unit residue)
— and orders of magnitude below the −370 M revaluation term the screen was
measuring. Anything larger is unaccounted value and the size of it is the finding.
Falsifier: `residual_fixed` is comparable to the revaluation term, which would
mean the fixed-mark valuation has not actually removed it and the harness is
wrong rather than the ledger.
Mechanism family: population-level closure, exact.

**E-036 — H-035, exact population closure. METHOD INVALID.**
Preregistered above. Artifact: `research/tools/fixedmarkclosure/main.go`.
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Run: `clock-control-5h-101.json`, seed 607, 5 simulated hours.

The plan was to remove revaluation by construction: run the simulation, then
value the terminal population a second time at the **initial** marks, so the
residual would be pure accounting. Measured:

| venue | change at initial marks | venue take | residual |
|---|---:|---:|---:|
| central | −238 730 230 755 | 111 599 100 195 | −127 131 130 560 |
| north | −242 546 516 137 | 113 610 202 087 | −128 936 314 050 |
| south | −239 410 592 406 | 110 192 269 985 | −129 218 322 421 |

That is −1.27 M USD per venue against a take of 1.12 M, which read as a large
unaccounted gap. **It is not a finding; the method does not do what it claims.**

`MarkedAccount` passes the supplied `spec` only to `valueWallet` and
`valueIsolated` — the spot and perp *balances*. Derivative exposure is valued
through `riskMark(book.Instrument, book)`, which reads the instrument's own
stored marks and **ignores the spec entirely**
(`exchange/valuation.go`). Fixing the spec's asset marks therefore removes
wallet revaluation and leaves derivative revaluation untouched, so the residual
is the derivative revaluation the method was supposed to eliminate. The harness
did work — it removed 99.4% of the change RT-024 measured, −373.5 M down to
−2.39 M — but the part it could not remove is exactly the part that matters.

**What would be needed.** A valuation entry point that accepts derivative marks
as well as asset marks, so a caller can revalue a whole account at a fixed point
in price space. That does not exist and building one is a change to a
scientific-branch surface, which this audit does not make.

**A collision risk, checked and cleared.** Client IDs are allocated by a
*per-venue* counter (`v.nextClient++`), so all three venues use 1..86 and every
tool in this audit that keys an initial value by client ID alone — `venueeffect`,
`populationclosure`, this harness — collides across venues. Measured directly:
all 86 shared IDs carry the **same role and the same initial equity** on all
three venues, so the collision returns the correct value in every case and
RT-022, RT-023, RT-024 and E-034 are unaffected. Recorded because the check was
not obvious and the next tool to key by client ID needs to know.

**Standing result unchanged.** RT-024's screen remains the population-level check,
with its stated limit: it separates revaluation from an accounting gap by
magnitude and consistency, not exactly. H-035 does not close that gap and the
note does not claim it does.

**H-036 — the population's accounting gap is zero, identified by regression
rather than by valuation surgery.**
E-036 failed because it tried to remove revaluation by changing how accounts are
*valued*, and `AccountValuationSpec` cannot reach the derivative term. The
simpler route does not touch valuation at all.

Representation change: treat the residual as a **linear function of the mark
drift** across seeds, and identify the two terms by their different behaviour
rather than by isolating one of them.

    residual(seed) = inventory × drift(seed) + gap

Each seed produces its own ABC drift and its own residual, so a fit over several
seeds separates them: the **slope** is the population's net long inventory and
the **intercept is the accounting gap** — the quantity RT-024 could only bound by
magnitude and E-036 could not reach at all. This needs no new API and no change
to any scientific surface; it needs only runs at different seeds, which cost
minutes each.

Predicted observable, recorded before running: the fit is close to linear
because the population's inventory barely changes across seeds — the endowments
are identical and only trading moves them — and the intercept is small relative
to the residuals it is extracted from. "Small" is defined in advance as **under
one percent of the mean absolute residual**; anything larger is an accounting gap
worth naming.
Falsifier: a large intercept, or a fit poor enough that the intercept is not
identified — if the drifts across seeds cluster too tightly, the regression has
no leverage and the experiment answers nothing. That second outcome is a real
possibility and will be reported as such rather than dressed up.
Mechanism family: population-level closure, identified by variation.

**E-037 — H-036, identifying the accounting gap by regression. NOT IDENTIFIED.**
Preregistered above. Artifact: `research/tools/closureregression/main.go`.
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Runs: `clock-control-5h-101.json`, seeds **607–614**, 5 simulated hours each.

The model was `residual = inventory × drift + gap`, fitted across seeds so the
intercept would be the accounting gap RT-024 could only bound and E-036 could not
reach.

| seed | ABC drift | residual (quote units) |
|---|---:|---:|
| 611 | −1.1102% | −1 127 510 973 |
| 609 | −1.0912% | −1 211 711 695 |
| 607 | −1.0511% | −1 112 935 935 |
| 610 | −1.0419% | −1 103 291 125 |
| 612 | −1.0324% | −842 954 514 |
| 614 | −1.0200% | −591 526 032 |
| 608 | −0.9591% | −1 013 042 345 |
| 613 | −0.9575% | −1 180 091 996 |

Fit: slope 75 819 252 488, **intercept −239 727 063 ± 1 572 059 399**,
**R² = 0.0398**.

**The intercept is not identified, and the preregistration named this outcome in
advance** — "if the drifts across seeds cluster too tightly, the regression has
no leverage and the experiment answers nothing. That second outcome is a real
possibility and will be reported as such rather than dressed up." The standard
error is **6.5× the estimate**. The intercept is −23% of the mean absolute
residual, and that number means nothing.

**Two independent reasons it failed, and the second is the more interesting.**

1. *No leverage.* Eight seeds produced drifts spanning only **0.1527%**, from
   −1.1102% to −0.9575%. A regressor that barely varies cannot identify an
   intercept.
2. *Misspecification.* R² of 0.04 says the residual is not tracking drift at all.
   The model treated inventory as a constant nuisance across seeds; it is not.
   Participants trade, so the population's net long position at the terminal
   instant is a **per-seed variable**, and `inventory × drift` is not a
   fixed-slope relation. More seeds would not fix this — the model is wrong, not
   merely underpowered.

**Consolidated position on RT-024's hole.** Three routes have now been tried and
all three fail for distinct, documented reasons:

| route | why it fails |
|---|---|
| magnitude screen (RT-024) | cannot separate revaluation from a gap below ~2% of the endowment |
| fixed-mark revaluation (E-036) | `AccountValuationSpec` reaches wallet balances only; derivative exposure is valued from the instruments' own marks |
| regression on drift (E-037) | no drift leverage across seeds, and inventory is a per-seed variable so the model is misspecified |

**The gap is not closable from the current artifact surface**, and the missing
piece is the same one E-036 identified: the artifacts record equity but not the
**inventory** behind it. `MarkedAccountSnapshot` reports `SpotEquity`,
`PerpCashEquity`, `DerivativeUnrealized` and `OptionMarketValue`, all already
valued at marks. One additional field — the population's net base-asset position
at each capture — would make the gap computable directly, with no regression and
no valuation surgery. That is a concrete, small instrumentation request and it is
the useful output of these three experiments.

**H-037 / E-038 — the live conservation check is silenced by the logging
configuration.**
Found by reframing after three failed routes to the population gap: instead of
building a new check, ask what the *existing* one does. `verifyConservation` runs
on every automation tick in the venue's `PostDerivativeMarkHook`.

    func (v *Venue) verifyConservation(now int64) {
        violations := v.Exchange.VerifyConservation()
        if len(violations) == 0 { return }
        log := v.makerStateLog
        if log.sink == nil && log.inner == nil { return }   // <-- here
        for _, violation := range violations {
            log.LogEvent(now, 0, "conservation_violation", violation)
        }
    }

`makerStateLog` is assigned only when `LogMode == "full"` or a checkpoint sink
exists (`sim.go:2718`). Otherwise it is the zero `venueLogger` and the guard
returns — **the violations are computed and discarded**.

Measured: a run with `-log-mode none` writes `greeks.json`,
`terminal-outcome.json`, `latency.json` and `manifest.json` and **no `venues/`
directory at all**. `terminal-outcome.json` carries `status`,
`terminal_population_captured`, `terminal_risk_captured` and
`strict_population_accounting` — and **no conservation field**. `LogEvent` at
`sim.go:4117` is the *only* emission path in the tree. So under logs-off there is
no counter, no error, no flag: a violation leaves no trace anywhere.

**Why this is the same defect as RT-001, one layer up.** The exchange goes to
explicit trouble to keep *recording* independent of logging, and says why on
`logBalanceChange`: "a movement that happens while no logger is attached is still
a movement, and leaving it out of the running total would make the verification
depend on the logging configuration." The recording is independent. The
**reporting of the resulting violation is not.**

**Reachability and scope.** The campaign's own configuration sets
`"log_mode": "full"`, so its headline runs do report violations. The gap is real
for every logs-off run — which includes performance work, ablations run without
logs, and **every run this audit performed in its last eight experiments**. Those
runs were, unknowingly, unverified on this axis. That is worth stating plainly
about my own evidence rather than only about someone else's.

Recorded as RT-025. Disposition is the owner's: surfacing the count in
`terminal-outcome.json`, or failing the run, are both changes to what a run
reports and neither is mine to make.

**H-038 — RT-001 and RT-025 are two instances of a class: checks whose only
output is a log line.**
Change the search unit. RT-001 found a movement recorded only when a logger was
attached; RT-025 found a violation reported only when a logger was attached. Two
instances of one shape, found a campaign apart and by unrelated routes, is reason
to look for the shape itself rather than wait for a third to surface.

The shape: a check computes a negative result — a violation, an unavailable
price, a failed invariant — and the **only** thing it does with that result is
emit a log event. Whether the finding survives then depends on the logging
configuration, which is a deployment choice rather than a property of the run.
Such a check is not a check; it is telemetry that happens to be shaped like one.

The sweep: every call site that emits an event whose name marks a failure —
`*_violation`, `*_failed`, `*_unavailable`, `*_error`, `reject*` — and then ask
of each whether anything else observes the same condition: a returned error, a
counter, a field in `terminal-outcome.json`, a test.

Predicted observable, recorded before sweeping: a handful of sites, most of them
legitimate — a rejected order is *returned to the caller* as well as logged, so
the caller observes it regardless of logging. The interesting residue is the
sites where the log really is the only consumer, and I expect one or two beyond
the two already known.
Falsifier: every failure-shaped event has a second observer, which would make
RT-001 and RT-025 a coincidence of two rather than a class.
Mechanism family: detector reachability, systematic sweep.

**E-039 — H-038, sweeping for checks whose only output is a log line.**
Preregistered above. Method: enumerate every `LogEvent` whose event name marks a
failure (`*_violation`, `*_failed`, `*_unavailable`, `*_error`), then ask of each
whether any second observer sees the same condition — a returned error, a
counter, a state flag, a field in `terminal-outcome.json`.

Result: **H-038 SUPPORTED. The class has four members and every one of them has
the identical shape.**

| site | event | second observer? |
|---|---|---|
| `simulations/multivenue/sim.go:4117` | `conservation_violation` | none — RT-025 |
| `exchange/collateral_interest.go:138` | `margin_interest_failed` | **none** |
| `exchange/exchange.go:2426` | `funding_settlement_failed` | **none** |
| `exchange/expiry.go:78` | `price_unavailable` | partial |

All four are literally `if log != nil { log.LogEvent(...) }` and nothing else:
no counter, no error returned to a caller who acts on it, no field in any
artifact. `ChargeCollateralInterest` swallows its error and calls the reporter;
`CheckAndSettleFunding` reports and continues to the next contract.

**The economically material one is the interest failure.** RT-014 established
that collateral interest is a live, material charge — borrowing is enabled and
the delivered rate reaches ~458 bps of a configured 500. If
`chargeCollateralInterestLocked` returns an error, the sweep is abandoned, *no
interest is charged that minute*, and under a logs-off run there is no trace at
all. Free leverage for as long as the condition persists, with nothing to say it
happened.

`price_unavailable` is the partial case and the distinction matters. On the
expiry path the condition also sets `settlementPending`, which is durable state
that gates admission (RT-012) — so it has a second observer. On the
**liquidation** path it does not: `buildAccountMarginProfile` fails,
`CheckLiquidations` calls the reporter and `continue`s, and the account is simply
not risk-assessed. RT-012 measured that consequence; this adds that even the
diagnostic disappears when logs are off.

**The contrast case that shows this is not universal.** Order rejections use
`rejectWithLog`, which logs *and returns the rejection to the caller*. The caller
observes it regardless of logging configuration. That is what a check with a
second observer looks like, and it is why the four above stand out rather than
being the house style.

**Prediction accuracy.** I predicted "one or two beyond the two already known";
there are three. Close, and recorded as approximately right rather than rounded
to correct.

**What this changes.** RT-001 and RT-025 were two findings a campaign apart,
reached by unrelated routes. They are one defect class with four known members,
all silenced together by a single deployment flag. Recorded as RT-026; the
remedy — a counter, an error, or a field in `terminal-outcome.json` — is one
decision covering all four rather than four separate fixes.

**H-039 — the sibling class: errors on value-moving paths that are not reported
at all.**
RT-026 enumerated checks whose result reaches only a log. The sibling shape is a
condition that reaches **nothing**: a returned error discarded with `_ =`, or a
loop that `continue`s past a failure without recording it. Where RT-026's members
are silenced by a deployment flag, these are silent by construction.

The sweep is over `exchange/` only, because that is where value moves, and over
two patterns: a discarded error return on a call that mutates balances,
positions or the venue ledger; and a `continue`/early `return` on an error inside
a loop that settles, charges or liquidates.

Predicted observable, recorded before sweeping: most hits are legitimate — `_ =`
on a `hash.Write`, which cannot fail, or on a deferred `Close` whose failure is
already handled elsewhere. The interesting residue is a discarded error on a path
that has already moved value or is about to. I expect **zero to two** genuine
ones, because the code has repeatedly shown careful error handling on the paths
this audit has read closely — `settleFunding` threads an `arithmeticError`
through its whole callback, the expiry preflight panics rather than proceeding on
an unrepresentable settlement, and `TryAdd`/`TryMulDiv` are used in preference to
bare arithmetic throughout.
Falsifier: no discarded error on any value-moving path, which would close the
class at one member (RT-026) rather than two.
Mechanism family: detector reachability, systematic sweep, second pass.

**E-040 — H-039, the sibling class does not exist.**
Preregistered above. Method: sweep `exchange/` for errors that reach nothing —
discarded returns, and error blocks that neither report nor propagate.

Result: **H-039 FALSIFIED. There is no second member.**

- **Discarded error returns in `exchange/`: zero.** Not "few" — the package
  contains no `_ =` assignment at all.
- **Error blocks that neither report nor propagate: zero**, after classifying the
  six candidates a structural scan produced. Five propagate by a route the scan's
  regex could not see: `settleFunding` carries a captured `arithmeticError` out
  of its callback (`funding.go:741`, `:783`); `exchange.go:1127` is a comma-ok
  accessor; and `exchange.go:1613`/`:1623` are a *deliberate* deferral that
  appends the failed symbol and its error to a `deferred` slice, so the condition
  survives as state.
- The sixth, `mustMarshalJSON` returning `[]byte("\"\"")` on error
  (`evstream_schema.go:521`), has exactly one call site and marshals a **string**
  symbol. `json.Marshal` cannot fail on a string, so the branch is unreachable
  and the fallback is a defensive default rather than a swallow.

**Prediction accuracy.** The preregistration expected "zero to two genuine ones,
because the code has repeatedly shown careful error handling on the paths this
audit has read closely". The answer is zero — the low end of a range that was
stated in advance and for a stated reason.

**Why the negative sharpens RT-026 rather than merely padding the record.** The
two sweeps together give a precise statement of the defect:

> The exchange propagates or reports **every** error it encounters. What it does
> not do, for four specific conditions, is give that report a **second
> observer**.

So RT-026 is not an error-handling discipline problem, and a remedy framed as
"handle errors properly" would find nothing to fix. It is a **reporting-channel**
problem: four conditions have exactly one channel, and that channel is switched
off by a deployment flag. The fix is to add a channel — a counter, a returned
error, a field in `terminal-outcome.json` — not to change how errors are
handled.

That distinction is the useful output, and it is also the honest context for
where this audit's findings sit: the arithmetic and error-handling layers have
held up under every sweep, and the findings have accumulated at the
specification, reporting and valuation layers instead.

**H-040 — some configured actor classes are effectively inert.**
E-040 established that the arithmetic and error-handling layers hold, and that
the findings cluster at specification, reporting and valuation. This turns the
lens onto the part of the specification no experiment here has touched: **the
actors themselves.**

Every finding so far concerns the venue. But "a fair battle of actors" also
requires that each configured actor *fights*. An actor whose quoting loop stalls
— the ghost-order failure this project has already been bitten by, where a late
accept after a cancel leaves a pending entry that blocks the timer forever —
still appears in the population, still holds its endowment, and still contributes
a row to every class-level average. It would read as "this strategy performs
near zero" when the truth is "this strategy stopped playing".

The observable already exists: every venue log carries `OrderAccepted` per
client, and the population artifact carries role and equity. Counting orders per
role class over a full run, and comparing the first and second halves of the run,
distinguishes an actor that is quiet by design from one that stopped.

Two candidates are already visible in E-032's table without having been looked
for: `round_trip` shows a between-venue spread of **45** and a within-venue
spread of **159** where comparable classes move in the millions, and
`metaorder_trader` shows 1 672 against 35 823. Either is consistent with a class
that barely trades — or with one that trades in tiny size by design.

Predicted observable, recorded before measuring: every class places orders in
both halves of the run, and the small-magnitude classes are small **by design**
(small clip sizes, long holding periods) rather than stalled. I expect H-040
**falsified** — this project's postmortem shows the ghost-order class of bug was
found and fixed, and the audit has repeatedly found the actor-facing paths
hardened.
Falsifier for my own prediction: a class with orders in the first half and none
in the second, which is a stall rather than a design.
Mechanism family: population validity, actor liveness.

**E-041 — H-040, does every configured actor act?**
Preregistered above. Artifact: `research/tools/actorliveness/main.go`.
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Run: `clock-control-5h-101.json`, seed 607, 5 simulated hours, `-log-mode full`.
Reproduce: `go run research/tools/actorliveness/main.go -dir <logdir>`.

Result: **H-040 FALSIFIED as predicted — no class is inert and none stops
entirely.** All 21 role classes place orders in both halves of the run.

**But the prediction was right for the wrong reason, and the instrument was
wrong.** The first version of the tool tested only for zero, and printed
"active" for every class including this one:

| class | first half | second half | ratio |
|---|---:|---:|---:|
| `dated_carry_arb` | 2 374 | **6** | **0.003** |
| `cdf_spot_maker` | 18 430 | 3 646 | 0.198 |
| `fixed_distance_maker` | 60 438 | 36 126 | 0.598 |
| `option_dealer` | 533 065 | 369 843 | 0.694 |

**A zero-test is not a liveness test.** A class that falls by 99.7% is not
"active" in any sense a reader would accept, and calling it that would have
turned a real signal into a clean bill of health. The tool now reports the ratio
and names a collapse. Eleventh instrument correction.

**What the collapse is, and what is not established.** Three dated futures are
listed in the run, at expiries 2 h, 4 h and 6 h after the start; the 5-hour run
ends before the third expires, and exactly one relisting occurs — the 4 h
contract is listed when the 2 h one expires, and nothing is listed after that. So
the dated board thins from three contracts to one over the run, and
`dated_carry_arb` trades a *relationship between* contracts.

That makes a decline expected. It does **not** establish that a decline to
**0.003** is expected, and this experiment cannot tell design from stall without
reading the actor's own logic. Recorded as an open question rather than as a
finding in either direction.

**Why it matters for the campaign's own numbers.** `dated_carry_arb` still
contributes a full row to every class-level average and to E-032's venue-effect
table, where it showed one of the higher between-venue ratios (120×). If the
class spends 60% of the run effectively out of the market, its score measures a
shorter and different period than its peers' — which is an unearned difference
between actors of exactly the kind this campaign is meant to detect, and it comes
from the instrument board rather than from strategy.

**Next, and cheap**: read `dated_carry_arb`'s trigger condition and check whether
it requires a contract pair that ceases to exist, or whether it should still be
quoting the single remaining contract.

**E-042 — RT-027 resolved: the collapse is design, not a stall.**
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Source read: `simulations/derivsim/carryarb.go`, `simulations/multivenue/sim.go:3439`.

RT-027 left open whether `dated_carry_arb`'s fall to 0.003 of its first-half
order rate was a stalled quoting loop — the ghost-order pathology this project
has been bitten by — or the instrument board thinning. Reading the actor settles
it.

**Two gates bound the desk's activity, both deliberate.**

1. **A net position cap per symbol.** `MaxPosPerSym = 5 × mvBasePrecision` with
   `LotQty = mvBasePrecision / 10`, so **50 lots per contract**, and each arm of
   the switch is guarded by `st.position > -MaxPosPerSym` / `< MaxPosPerSym`.
   Once the desk is at its cap on a contract *in the direction the basis
   favours*, that contract stops trading.
2. **Edge scaled by time to expiry**: `edge = EdgeBps × sqrt(timeToExpiry /
   TenorNano)`. This **lowers** the bar as expiry approaches, so it cannot
   produce the collapse — it would produce the opposite. Ruling it out is what
   makes the first gate the explanation rather than a guess.

**So the collapse is the cap binding against a persistently one-signed basis,
combined with the board thinning from three contracts to one** (E-041: expiries
at 2 h, 4 h and 6 h against a 5-hour run, with exactly one relisting). No ghost
order, no blocked timer, no stalled loop.

*Arithmetic, stated as consistent-with rather than as proof.* Saturation predicts
2 desks × 3 contracts × 50 lots × 2 legs × 3 venues ≈ **1 800** orders; the
first half shows **2 374**. The 32% excess is churn — the cap is on *net*
position, so a basis that changes sign lets the desk trade back and re-accumulate.
The figures agree in magnitude and mechanism, not exactly, and the difference is
explained rather than ignored.

**What survives from RT-027, now with a mechanism.** The desk earns its entire
result in the first part of the run and then sits at its cap. Its score is
therefore **front-loaded and bounded by configuration**, while an uncapped class
keeps compounding for the full five hours. Comparing the two on a per-run basis
compares different things — not because either is broken, but because
`MaxPosPerSym` is a limit on one and not on the other. That is a real caveat for
any class-level ranking and it is now attributable to a named parameter rather
than to a suspicion.

RT-027 is closed: **design**. The caveat about class-level comparison stands and
is recorded against the parameter that causes it.

**H-041 — the classes are not all playing the same game, because only some of
them have a ceiling.**
E-042 traced `dated_carry_arb`'s collapse to a named parameter: a 50-lot net
position cap per contract. That is not a defect, but it has a consequence the
campaign's class-level numbers do not carry — a capped class stops compounding
once it is full, while an uncapped one keeps going for the whole run.

Generalise it. If some classes are bounded by a configured ceiling and others
are not, then a per-run comparison between them is not a comparison of
strategies; it is partly a comparison of **how long each was allowed to keep
playing**. That is the same shape as RT-022's environment term, arriving through
the actor configuration rather than the venue.

The sweep is static and cheap: enumerate every actor construction in
`simulations/multivenue/sim.go` and record which carry a position, trade-count or
notional ceiling and which do not.

Predicted observable, recorded before sweeping: a **mixed** population — the
derivative desks carry explicit caps because unbounded derivative exposure would
be reckless, while the flow and maker classes are bounded by capital or by their
own inventory logic rather than by a hard limit. If that is what the sweep finds,
the finding is not that anything is wrong but that **class-level rankings need
the ceiling column beside them**, and that column does not currently exist.
Falsifier: every class has an equivalent ceiling, which would make the playing
time uniform and RT-027's caveat specific to one class rather than structural.
Mechanism family: population validity, comparability of scores.

**E-043 — H-041, the classes are not given the same board.**
Preregistered above. Static sweep of every actor construction in
`simulations/multivenue/sim.go`.
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.

Result: **H-041 SUPPORTED, and the spread is larger than the prediction
allowed for.** Every ceiling below is expressed in the same unit —
`mvBasePrecision`, one ABC — so they are directly comparable:

| class | ceiling | in ABC | source |
|---|---|---:|---|
| `option_value_taker` | `OptionValueTakerMaxPosition` default | **1** | `sim.go:1216` |
| `dated_carry_arb` | `MaxPosPerSym` | **5** | `sim.go:3441` |
| `fixed_distance_maker` | `MaxInventory` | **200** | `sim.go:3361` |
| `imbalance_maker` | `MaxInventory` | **200** | `sim.go:3376` |
| `carry_arb` | `CarryMaxPosition` default | **500** | `sim.go:1171` |
| `elastic_supplier` | `MaxPosition` | **10 000** | `sim.go:3587` |
| `parity_arb` | `MaxTrades: 100 000` | — | count, not position |

**Four orders of magnitude separate the tightest position ceiling from the
loosest** — 1 ABC for the option value taker against 10 000 for the elastic
supplier. `dated_carry_arb`, whose collapse started this thread, sits at 5: forty
times tighter than a maker and two thousand times tighter than a supplier.

**What this does to a class-level number.** A per-run result is the product of
edge per unit and units allowed. Two classes with identical skill and identical
opportunity will post results differing by their ceiling ratio, and the ceilings
here differ by up to 2 000×. E-032's magnitudes line up with exactly that
reading: `elastic_supplier` at −10.6 M and `triangle_arb` at +13.6 M against
`dated_carry_arb` at −2.6 M is as much a statement about allowances as about
strategies.

**This is not a defect.** Unbounded derivative exposure would be reckless, and a
tight cap on an option value taker is prudent risk design. The prediction
anticipated a mixed population and that is what the sweep found. What it did not
anticipate is the **magnitude** of the spread, and that is the part worth
recording.

**The finding is that the ceiling is not reported beside the score.** Nothing in
`greeks.json`, `terminal-outcome.json` or the population artifact carries the
constraint each class was operating under, so a reader comparing class rows has
no way to normalise for it. RT-022 established an environment term coming from
the venue; this is the same shape arriving through the actor configuration, and
unlike the venue term it is a **known constant**, available at construction, that
could simply be emitted.

Recorded as RT-028. The remedy is small and is the owner's: carry each class's
binding ceiling into the population artifact so that a per-run result can be read
per unit of allowance as well as in absolute terms.

**H-042 — ranking the classes by return on capital reorders them.**
RT-028 showed the classes are bounded at different scales. Capital is the
sibling dimension, and the more consequential one: a ceiling limits how much a
class can hold, but capital is what a *rate of return* divides by. The
construction sweep already showed endowments differing by design — makers get
10 000 ABC and 500 M USD, noise traders 10 M, dealers 150 M — so absolute PnL
across classes is a comparison of differently-sized books.

Representation change: stop reading the result as an amount and read it as a
**rate**. The population artifact already carries each participant's initial
marked equity in USD, so return on capital is computable from evidence that
exists, for every class, with no new instrumentation.

This is the normalisation nothing in the campaign computes. If it merely
rescales the table, it is a footnote. If it **reorders** it, then absolute
class-level PnL is ranking capital allocation rather than skill, and every
comparison drawn from it inherits that.

Predicted observable, recorded before measuring: the ordering changes
materially — at least one class in the top three by absolute PnL leaves it, and
at least one small-capital class enters. The reason to expect it is arithmetic
rather than suspicion: E-032's absolute results span roughly ±13 M while the
endowments span at least an order of magnitude, so the two orderings cannot both
be dominated by the same term.
Falsifier: the two rankings agree up to noise, which would mean capital is
roughly uniform across classes and RT-028's ceiling spread is the only
comparability problem.
Mechanism family: comparability of scores, normalisation.

**E-044 — H-042, ranking the classes by return on capital.**
Preregistered above. Artifact: `research/tools/returnoncapital/main.go`.
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Run: `clock-control-5h-101.json`, seed 607, 5 simulated hours.
Reproduce: `go run research/tools/returnoncapital/main.go -file <logdir>/greeks.json`.

Result: **H-042 PARTIALLY SUPPORTED, and the prediction was right about the
wrong part of the table.**

**Capital spans 510×** across classes — `round_trip` at 265 M against
`latent_liquidity` at 135 054 M — which is a wider spread than RT-028's
ceilings produce in practice.

**The top is stable.** By absolute change: `triangle_arb`, `option_dealer`,
`round_trip`. By return: `triangle_arb`, `option_dealer`, `vanna_volga_desk`.
The first two hold their places on both measures, so `triangle_arb`'s dominance
is **not** a capital artifact — it earns +3.675% on 2 220 M, the best rate in the
population as well as the largest amount. That is worth stating positively: the
campaign's headline result survives the normalisation.

**The middle is scrambled.** Total rank displacement is **62 places over 21
classes**, averaging three places each, with two large moves in opposite
directions:

| class | rank by absolute | rank by return | capital | return |
|---|---:|---:|---:|---:|
| `latent_liquidity` | 21 | **11** | 135 054 M | −0.351% |
| `metaorder_trader` | 6 | **18** | 1 854 M | −0.518% |

`latent_liquidity` looks like the population's worst performer at −474 M and is
mid-table once its book size is accounted for; `metaorder_trader` looks
sixth-best at −9.6 M and is fourth-from-last per unit of capital. Both move on
capital alone.

**Where the prediction was wrong.** It said "at least one class in the top three
leaves it, and at least one small-capital class enters", which happened —
`round_trip` out, `vanna_volga_desk` in — but as a single-place swap, not the
material reordering the wording implied. The material reordering is in the middle
of the table, which the prediction did not anticipate at all. Recorded as
partially right rather than rounded up.

**What it means for reading the campaign's numbers.** An absolute class-level PnL
is a fair summary at the extremes and misleading in the middle. Two specific
conclusions that do **not** survive normalisation: that `latent_liquidity`
performs worst, and that `metaorder_trader` performs well. Neither is a defect in
the simulation — the classes are endowed differently by design — but neither
ordering can be read off the artifact as it stands, because the artifact reports
the numerator and not the denominator in any derived form.

Recorded as RT-029, together with RT-028: the ceiling and the capital are both
known constants, both absent from the reported result, and both change how a
class-level table should be read.

**H-043 — the triangular arbitrageur harvests a pricing artifact, not a market.**
E-044 produced a number worth interrogating rather than filing: `triangle_arb`
earns **+81.6 M on 2 220 M in five hours — +3.675%** — while every other class
except the option dealer loses money and the population as a whole is down.

Triangular arbitrage is riskless by construction: it profits from an
inconsistency between `ABC/USD`, `CDF/USD` and the cross `ABC/CDF`. Someone must
therefore be quoting inconsistently, persistently, for five hours. Two readings:

- **Market.** The cross book is quoted independently, the inconsistency appears
  and is competed away, and the residual oscillates around zero. The arb is doing
  its designed job of disciplining the cross, and its profit is the ecology
  working.
- **Artifact.** The cross maker anchors to something other than the implied rate
  `ABC/USD ÷ CDF/USD`, so the residual is persistently one-signed and the arb is
  harvesting a modelling choice rather than outcompeting anyone. Its entire
  result would then be a property of the configuration.

Discriminating observable: the sign history of the triangular residual
`implied − actual` over the run. **Oscillation around zero is a market; a
persistent sign is an artifact.** Magnitude alone cannot separate them, which is
why the test is on sign persistence rather than on size.

Predicted observable, recorded before measuring: persistently one-signed. The
reason is the return itself — 3.675% riskless in five hours is not what a
contested inconsistency pays, and `abc_cdf_spot_maker` is simultaneously among
the larger absolute losers at −12.6 M, which is the shape of a counterparty being
picked off on one side rather than trading around a fair value.
Falsifier: the residual changes sign repeatedly and spends comparable time on
each side, which would make the profit a competed market outcome and this
hypothesis wrong.
Mechanism family: designed-ecology validity, artifact versus market.

**E-045 — H-043, the triangular residual. SUPPORTED on sign, UNRESOLVED on
cause.**
Preregistered above. Artifact: `research/tools/triangleresidual/main.go`.
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Run: `clock-control-5h-101.json`, seed 607, 5 simulated hours, full logs.
Reproduce: `go run research/tools/triangleresidual/main.go -dir <logdir> -venue north`.

**The sign test is decisive.** Across ~16 000 instants per venue where all three
books carry a two-sided mid:

| venue | positive | negative | sign flips | dominant sign holds |
|---|---:|---:|---:|---:|
| north | 16 438 | 159 | **6** | 99.0% |
| central | 15 607 | 54 | **36** | 99.7% |
| south | 15 704 | 169 | **14** | 98.9% |

Six sign changes in five hours on north. This is **not** a contested
inconsistency being competed away; the residual holds one sign for essentially
the whole run on all three venues. H-043's discriminating prediction — persistent
sign, not oscillation — is supported.

**The magnitude is where I stop and flag my own instrument.** Mean |residual| is
**38%**, max 68.8%. Terminal book state on north:

| | value | per unit |
|---|---:|---|
| `ABC/USD` mid | 4 947 620 000 | 49 476.20 USD per ABC |
| `CDF/USD` mid | 300 150 000 | 3 001.50 USD per CDF |
| implied cross | — | **16.4838 CDF per ABC** |
| `ABC/CDF` mid | 514 150 000 | **5.1415 CDF per ABC** |

A ratio of **3.206**. And the configuration's own intended bootstrap for the
cross is `MulDiv(mvBootstrapPrice, mvBasePrecision, mvCDFBootstrap)` =
1 666 666 667, i.e. **16.6667 CDF per ABC** — which matches the implied rate and
not the observed book.

**Two readings, and I am not choosing between them on this evidence.**

1. **Real dislocation.** The cross book has drifted 3.2× from the rate its own
   bootstrap sets, `triangle_arb` has been harvesting it all run (+81.6 M,
   +3.675%), and `abc_cdf_spot_maker` has been paying for it (−12.6 M). The
   persistent sign and the matching PnL signs both fit.
2. **My units.** A 3.2× standing dislocation with an arbitrageur present is not
   what a working ecology looks like, and this campaign has now caught twelve
   instrument errors — three of which were confident readings of a quantity whose
   scale I had wrong.

The configured bootstrap matching the implied rate is evidence for reading 1, since
it makes 16.67 the intended cross rate rather than a number I constructed. But it
is not conclusive: the bootstrap sets an initial price, and nothing forces the
book to stay there.

**Not recorded as a finding.** The sign result stands on its own and is recorded
as such. The magnitude claim is held back pending the decisive check, which is
cheap and named: read `abc_cdf_spot_maker`'s anchor to see whether it quotes
around the implied cross rate or around its own book's mid. If the latter, the
book can drift arbitrarily far from the implied rate and reading 1 is confirmed
with a mechanism; if the former, my units are wrong and the residual is an
artifact of my tool.

Recorded as RT-030, status **open**, with the sign evidence attached and the
magnitude explicitly withheld.

**E-046 — RT-030 resolved: the cross book is self-referential and drifts 69%
from its own bootstrap.**
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Run: `clock-control-5h-101.json`, seed 607, 5 simulated hours, full logs.
Reproduce: `go run research/tools/pricerange/main.go -dir <logdir> -symbol ABC-CDF -reference 1666666667`.

**The unit question is settled by comparing the book to itself.** Tracing
`ABC/CDF` against its own configured bootstrap
(`MulDiv(mvBootstrapPrice, mvBasePrecision, mvCDFBootstrap)` = 1 666 666 667):

| | price | vs bootstrap |
|---|---:|---:|
| high | 1 669 200 000 | **+0.15%** |
| last | 514 200 000 | **−69.15%** |

Both figures are the same book in the same units, so **no unit error of mine can
produce this**. The book starts at its bootstrap — the +0.15% high — and ends
3.24× below it. Over the same run `ABC/USD` moved −1.05% and `CDF/USD` +0.08%.
The dislocation is real. Reading 1 of RT-030 is confirmed and the withheld
magnitude is now supported.

**The mechanism, and it is one line.**

    crossConfig := stoikovConfig("ABC/CDF", "ABC/CDF", crossBootstrap, mvBasePrecision, crossTick)

`stoikovConfig(symbol, reference, ...)` sets `ReferenceSymbol: reference`, and
for the cross book **both arguments are `"ABC/CDF"`**. The maker quotes around
its own book. Nothing ties the cross to the implied rate `ABC/USD ÷ CDF/USD`; the
bootstrap seeds it at the right value and it free-runs from there.

Worse, the reference is self-referential at the *population* level too. `ABC/CDF`
is published by `spotIndexProvider`, whose consensus for a symbol is the **median
of the three venues' own mids of that symbol**. So the cross market's reference is
a consensus of itself, and all three venues can drift together — which is exactly
what E-045 measured, with mean |residual| of 37.8-38.2% on all three.

The code names this failure mode itself, in `anchor.go`: *"a market with no
reference of its own falls back to its book midpoint and becomes
self-referential."* The cross book is that case, and the index publishing its
symbol does not rescue it, because the index for that symbol is built from the
same books.

**What it means for the campaign's numbers.** Triangular consistency is enforced
by nobody except `triangle_arb`, which is capped like every other class
(RT-028). E-044's headline — `triangle_arb` at **+3.675%**, best in the
population on both absolute and return measures, while `abc_cdf_spot_maker` loses
12.6 M as the counterparty — is now explained: it is harvesting a 69% standing
dislocation, not outcompeting anyone. **Any conclusion about cross-asset or
triangular dynamics drawn from this configuration is measuring a self-referential
book drifting, not a market.**

Recorded as RT-031. This is the strongest economic finding since RT-014, and
unlike RT-014 it is not a rounding boundary: it changes what the cross-asset
population's results mean.

**H-044 (PREREGISTERED) — the ABC/USD price level is a configured peg, not a
market outcome.**

Inspection, base `a666d02faede3d40f046b11e60eb672c59386a94`:

1. `clock-control-5h-101.json` omits `elastic_supplier_symbols`, so
   `makerSymbol(nil, i)` returns `"ABC/USD"` for **all 8** suppliers
   (`sim.go:1616`). `CDF/USD`, `ABC/CDF` and `ABC-PERP` have **no price-elastic
   demand at all** — which is the missing ingredient RT-031 needed.
2. The config sets `"elastic_supplier_reference_half_life": 0`, and no default
   overrides it. `supplier.go` documents exactly what that means: *"A fixed
   reference is an exogenous fundamental... a participant trading against
   deviations from it is a peg rather than a demand curve: measured over six
   runs the terminal price was minus excess supply over aggregate elasticity to
   three significant figures, which is that actor's configuration read back out
   rather than a market outcome."*
3. The construction site (`sim.go:3193`) carries the opposite comment —
   *"Seeded at the opening price and revised toward what it observes, so the
   participant holds a private belief rather than a standing instruction"* —
   which describes a configuration this campaign does not use.

**Prediction.** With reference fixed at `mvBootstrapPrice` = 50 000 USD and
`ElasticityPerPercent` = 15 000 000 000 base units (150 ABC per percent per
supplier, 1 200 ABC per percent aggregate), the suppliers' terminal aggregate
ABC position should equal

    aggregate_position ≈ −1200 ABC × (terminal ABC/USD percent deviation from 50 000)

E-046 measured that deviation as −1.05%, predicting **+1 260 ABC net long**.

**Falsifiers.** (a) aggregate terminal position differs from the prediction by
more than 25%; (b) any supplier sits at `MaxPosition` (10 000 ABC), which would
make the level a cap rather than a curve; (c) suppliers appear on books other
than ABC/USD.

**Amendment, written before any result was read.** Reading
`supplier.go:86-101` and the construction loop more carefully changes two things:

- The 8 suppliers are built **per venue**, so there are 24 in total and the
  prediction applies **per venue** against that venue's own ABC/USD terminal
  price. That gives three independent replications inside one run, which is a
  stronger test than the pooled version above.
- `TargetPosition` clips at ±`MaxPosition` and the participant closes its gap at
  `RebalanceLot` = 0.5 ABC per tick. Reaching +1 260 ABC needs ~2 520 fills per
  venue. **Convergence lag is therefore a named alternative explanation for a
  miss**: an aggregate position short of the prediction *in the direction of
  zero* is evidence of rate-limiting, not evidence against the peg. Only an
  overshoot, a sign error, or a cap hit falsifies H-044.

**Discriminating experiment E-047**, preregistered here before the run: re-run
`clock-control-5h-101.json` seed 607 with full logs, sum signed ABC fills per
`elastic_supplier_*` role, and compare with the prediction above.
Status: **SUPPORTED WITHIN TESTED SCOPE** by E-047 — ratio 1.000 on all three
venues, no falsifier fired.


**E-047 — H-044 SUPPORTED WITHIN TESTED SCOPE, in its strongest form. The
ABC/USD terminal price is the elastic suppliers' configuration read back out.**
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Run: `clock-control-5h-101.json`, seed 607, 8 simulated hours (see correction
below), `-log-mode full`.
Reproduce: `go run research/tools/elasticpeg/main.go -file <logdir>/greeks.json`.

Aggregate terminal position of the 8 `elastic_supplier_*` participants per
venue, taken from **wallet balances** rather than the actor's own counter, so
the number does not depend on the actor being correct:

| venue | terminal mark | vs 50 000 | aggregate position | predicted by price | ratio |
|---|---:|---:|---:|---:|---:|
| central | 4 929 505 000 | −1.4099% | +1 692.00 ABC | +1 691.88 ABC | **1.0001** |
| north | 4 929 380 000 | −1.4124% | +1 695.00 ABC | +1 694.88 ABC | **1.0001** |
| south | 4 929 375 000 | −1.4125% | +1 695.36 ABC | +1 695.00 ABC | **1.0002** |

**Every preregistered falsifier fails to fire.** No participant is at
`MaxPosition` (0 of 8 on every venue), so this is a curve and not a cap. There
is no overshoot and no sign disagreement. The ratio is not merely within the
25% band — it is **1.000 to four significant figures on three independent
venues**, and it is not even below one, so the named convergence-lag confound is
excluded too: the population is fully converged onto its supply curve.

This is `supplier.go`'s own documented failure mode, reproduced exactly: *"the
terminal price was minus excess supply over aggregate elasticity to three
significant figures, which is that actor's configuration read back out rather
than a market outcome."* The comment says three significant figures. The
measurement gives four.

**A second consequence, not preregistered and therefore POST-HOC.** The three
venues' terminal marks agree to within **0.0026%** despite deliberately
heterogeneous rules (RT-022: price-time vs pro-rata, 1h/2h/8h funding). They are
not three environments that happen to converge; they are three books pinned to
the same configured reference. Cross-venue price-level dispersion in this
campaign is not a market outcome either.

**Why RT-031 and this finding are the same mechanism seen twice.** The peg
reaches exactly one book. `elastic_supplier_symbols` is `null` in the effective
config, so `makerSymbol(nil, i)` puts **all 8 suppliers on ABC/USD**
(`sim.go:1616`). `CDF/USD`, `ABC/CDF` and `ABC-PERP` get none. So ABC/USD is
pinned to four significant figures while the cross book, with no elastic demand
and a maker that references itself, drifts 69%. The campaign has one anchored
book and the rest float.

Recorded as RT-032.

**CORRECTION (metadata, affects many prior records).** The simulated horizon is
a **CLI flag**, `-duration`, defaulting to **8h** in `cmd/multivenue/main.go:234`.
It is not in the config at all. The config's filename says `5h`, and E-022,
E-033, E-044, E-045 and E-046 all report "5 simulated hours" — I was reading the
**filename** rather than the run. This run logs `sim=8h0m0s`. No measured value
changes, because the runs were what they were; the stated horizon on those
records is wrong and should read 8h unless a record explicitly passed
`-duration`. Same class of process error as E-033: **read what the run did, not
what its name says.**


**H-045 (LABELLED POST-HOC — see the process failure below) — the population's
profit is sourced from a configured donor.**

From [[RT-032]]: the elastic suppliers are forced buyers as the price falls, and
bought +1 695 ABC per venue on a −1.41% move because their reference never moves.
A participant that must buy into every decline is a donor by construction.

**Instrument, defined before the measurement.** Raw terminal-minus-initial equity
cannot answer this — every participant is net long ABC, so one mark move hits all
of them and a class endowed with more inventory shows a larger "result" without
trading. RT-024 records that trap. Carry-adjusted PnL removes it exactly:

    carry_adjusted_pnl = Δequity − Σ_asset initial_balance_asset × Δmark_asset

Everyone starts flat in derivatives, so initial wallet balances carry the whole
revaluation term.

**Self-test that gates the result.** Summed over all participants the
decomposition must close against what the venues took:
`Σ carry_adjusted_pnl + exchange_take ≈ 0`. A residual above **1% of gross flow**
makes the ranking inadmissible; `classpnl` then prints no ranking and exits
non-zero, so a failed decomposition cannot be quoted.

**Prediction:** `elastic_supplier` is the largest net donor class.
**Falsifiers:** (a) closure fails → INCONCLUSIVE; (b) `elastic_supplier` is not
the largest donor → falsified; (c) its result is positive → falsified in the
opposite direction.

**PROCESS FAILURE, recorded rather than repaired.** The above was committed in
the session transcript before the run, but the edit that was supposed to write it
into this note **silently did nothing**: the script used `'\\n'` where it needed
`'\n'`, so `str.replace` matched no anchor, changed no bytes, and **printed a
success message it had not earned**. The experiment therefore ran while I
believed it was preregistered here and it was not. I am not back-dating it. The
record stands as POST-HOC in the artifact, which is the weaker and correct label,
and the numbers below are unaffected because the prediction and falsifiers were
fixed before the data existed — just not durably.

*Lesson, of the same family as the twelve instrument errors already logged: a
script that reports success must verify the success, not assume it. Every note
edit from here asserts that the anchor matched and that the file changed.* A
claim that exists only in chat is not a completed research deliverable — that
rule applied to a preregistration this time, not a finding.

**E-048 — H-045 FALSIFIED WITHIN TESTED SCOPE. The suppliers are not the donor.
The battle is decided by one class of six taking three quarters of all gains.**
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Run: `clock-control-5h-101.json`, seed 607, 8h (the `-duration` default),
`-log-mode none`.
Reproduce: `go run research/tools/classpnl/main.go -file <logdir>/greeks.json`.

**Closure self-test, run before the ranking was read:**

    Σ carry-adjusted pnl   −9 890 673 USD
    exchange take          +4 944 035 USD
    residual               −4 946 638 USD  (0.8866% of gross)

Inside the 1% tolerance, so the ranking is admissible — **but only just**, and
the resolution limit that implies is stated below rather than buried.

| class | n | carry-adjusted | raw Δequity | per head |
|---|---:|---:|---:|---:|
| noise_flow | 18 | **−220 204 919** | −254 954 519 | −12 233 607 |
| spot_maker | 12 | −6 817 441 | −64 733 441 | −568 120 |
| future_flow | 9 | −5 993 965 | −23 368 765 | −665 996 |
| carry_arb | 6 | −2 423 266 | −9 553 866 | −403 878 |
| elastic_supplier | 24 | **−2 026 313** | **−335 454 313** | −84 430 |
| latent_liquidity | 18 | −1 029 326 | **−632 232 326** | −57 185 |
| fixed_distance_maker | 24 | +11 483 857 | +84 725 457 | +478 494 |
| imbalance_maker | 24 | +27 556 949 | +100 798 549 | +1 148 206 |
| triangle_arb | 6 | **+174 217 981** | +192 528 381 | **+29 036 330** |

**H-045 is falsified, and cleanly.** `elastic_supplier` is the **fifth** donor,
not the first, at −2.0 M against `noise_flow`'s −220 M — two orders of magnitude,
far outside any resolution concern. The peg costs the suppliers **revaluation,
not trading**: raw Δequity −335 M, carry-adjusted −2.0 M, so **99.4% of their
apparent loss is the mark move on an endowment they were given** rather than the
price of being a forced buyer. Falsifier (c) also fails: they are slightly
negative, not positive.

**The instrument earned its keep on the exact trap it was built for.** Ranked on
raw Δequity this population's great losers are `latent_liquidity` (−632 M) and
`elastic_supplier` (−335 M). Carry-adjusted they are 22nd and 18th. **Both
"findings" would have been endowment size wearing a result's clothes** — RT-024's
error caught before publication instead of after.

**What the run actually shows: the battle is not close.** Total positive
carry-adjusted result across all classes is ≈+231 M. `triangle_arb` alone takes
**+174.2 M, 75.4% of every gain in the population, with 6 participants of 252**.
Its per-head result is **25× the best market-making class** (`imbalance_maker`)
and 61× `fixed_distance_maker`. The funder is `noise_flow` at −220 M, −12.2 M
per head.

**Read with [[RT-031]] this is not a competitive outcome.** The cross book is
self-referential and ends −69% from its bootstrap; `triangle_arb` is the only
class trading the triangle; and `noise_flow` is configured onto `CDF/USD` and
`ABC/CDF` by `noise_target_qty_by_symbol`. The campaign's dominant result is one
small class harvesting a standing modelling dislocation from uninformed flow
routed into it. E-044 ranked `triangle_arb` first on a coarser instrument; that
is reproduced here and now explained rather than celebrated.

**Resolution limit, stated because the closure passed narrowly.** The residual is
−4.95 M, so **no class whose carry-adjusted result is smaller than about ±5 M is
distinguishable from it**. That covers `carry_arb`, `elastic_supplier`,
`latent_liquidity`, `option_flow`, `metaorder_trader`, `round_trip`,
`dated_carry_arb`, `parity_arb`, `option_value_taker`, `cdf_spot_maker`,
`option_dealer` and `vanna_volga_desk` — twelve of twenty classes, whose ordering
among themselves is **NOT EXERCISED** here. Only `noise_flow`, `triangle_arb`,
`imbalance_maker`, `fixed_distance_maker`, `spot_maker`, `future_flow`,
`futures_maker` and `abc_cdf_spot_maker` clear it. Both the falsification and the
concentration result rest on classes far outside the band.

Recorded as RT-033.

**Next experiment, highest information gain, not yet run.** Attribute
`triangle_arb`'s +174.2 M to books by summing its signed fills per symbol from a
full-log run. Majority on `ABC/CDF` ⇒ the concentration and RT-031 are one
mechanism and the campaign has a single dominant artifact. Spread across
`ABC/USD` and `CDF/USD` ⇒ two separate problems.


**H-046 (PREREGISTERED) — `triangle_arb`'s +174.2 M is earned on the
self-referential cross book, making [[RT-031]] and [[RT-033]] one artifact rather
than two problems.**

E-048 left this as the highest-information-gain next step. RT-031 showed
`ABC/CDF` is self-referential and ends −69% from its bootstrap; RT-033 showed
`triangle_arb` takes 75.4% of every gain in the population. Whether those are the
same fact is not yet measured.

**Representation change.** E-048 measured value per *class*. This measures value
per *(class, book)* pair and per *counterparty*, which the account snapshots
cannot express. `OrderFill` evidence (`exchange/settlement.go:526`) carries
`symbol`, `qty`, `price`, `side`, `trade_id` and the acting `client_id`, and two
fill rows share a `trade_id`, so the trade graph can be reconstructed: who traded
what with whom, on which book.

**Per-book contribution.** For a book `BASE/QUOTE`, a participant's cash flow
accrues in QUOTE and its inventory in BASE:

    contribution_book = Σ(signed quote cash flow) + (net base inventory) × terminal mark

both converted to USD at terminal marks.

**Self-test that gates the result, independent of E-048's.** Summed over books,
a class's contribution must reproduce its carry-adjusted PnL from `classpnl`:

    Σ_books contribution ≈ carry_adjusted_pnl

These are computed from **different sources** — fill-by-fill evidence versus
account snapshots — so agreement is a real cross-check rather than a restatement.
A mismatch above 5% of the class result means the fill stream does not account
for the class's result, and the per-book split is then reported as INCONCLUSIVE
rather than explained away. Funding, borrow interest and liquidation transfers
are the known terms the fill stream omits, so a mismatch is *expected* for
derivative classes and is itself informative about which classes earn outside the
order book.

**Prediction.** For `triangle_arb`, the `ABC/CDF` book contributes the majority
of +174.2 M, and its dominant counterparty classes there are `noise_flow` and
`abc_cdf_spot_maker`.

**Falsifiers.** (a) the self-test fails for `triangle_arb` → INCONCLUSIVE;
(b) `ABC/CDF` contributes less than half → H-046 falsified and the concentration
is a separate problem from RT-031; (c) the dominant counterparty is a class other
than the two named → the routing story in RT-033 is wrong even if the book is
right.

**Discriminating experiment E-049**, preregistered before the run: re-run
`clock-control-5h-101.json` seed 607 with `-log-mode full`, reconstruct the trade
graph from `OrderFill` evidence, run the self-test, then read the split.
Status: **SUPPORTED WITHIN TESTED SCOPE** by E-049 — 98.1% of the result is
`ABC/CDF` and 92.3% of that is a single counterparty; the `noise_flow` half of
the counterparty prediction was wrong.


**E-049 — H-046 SUPPORTED WITHIN TESTED SCOPE, and it exposes an error in
RT-033's mechanism sentence. 98.1% of `triangle_arb`'s result is one book, and
92.3% of that is one counterparty.**
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Run: `clock-control-5h-101.json`, seed 607, 8h, `-log-mode full`.
Reproduce: `go run research/tools/flowattrib/main.go -dir <logdir>`.
5 130 138 `OrderFill` records across 15 files, **zero unpaired executions**.

**Self-test first, and it passes on the class that matters.** Fill-stream
contribution against account-snapshot carry-adjusted PnL — two entirely
different sources:

| class | from fills | from snapshots | gap |
|---|---:|---:|---:|
| triangle_arb | 174 203 545 | 174 217 981 | **−0.0%** |
| noise_flow | −219 422 999 | −220 204 919 | 0.4% |
| abc_cdf_spot_maker | 6 858 652 | 6 879 438 | −0.3% |
| elastic_supplier | −2 028 598 | −2 026 313 | −0.1% |
| latent_liquidity | −1 030 003 | −1 029 326 | −0.1% |
| futures_maker | 0 | 7 056 275 | −100.0% |
| perp_maker | 0 | 2 484 659 | −100.0% |
| option_dealer | 5 645 | 795 504 | −99.3% |

The spot-traded classes reconcile to a fraction of a percent from two independent
sources. The 100% gaps are the preregistered expectation working as designed:
those classes trade only derivative books, which this tool deliberately does not
fold in, so **the gap column reads off which classes earn nothing in a spot order
book**. `futures_maker`'s +7.06 M and `perp_maker`'s +2.48 M are entirely
derivative-side.

**Where `triangle_arb`'s +174.2 M comes from:**

| book | contribution |
|---|---:|
| **ABC/CDF** | **170 904 176 (98.1%)** |
| ABC/USD | 2 083 684 |
| CDF/USD | 1 215 685 |

**Who pays it, on ABC/CDF:**

| counterparty | value to `triangle_arb` | base traded |
|---|---:|---:|
| **abc_cdf_spot_maker** | **157 661 074 (92.3%)** | 6 547.6 ABC |
| imbalance_maker | 6 790 225 | 257.8 ABC |
| fixed_distance_maker | 6 452 877 | 230.7 ABC |

H-046 is supported: the cross book is where essentially all of it is earned, so
[[RT-031]] and [[RT-033]] are **one artifact, not two problems**.

**The rate is the tell.** 157.7 M USD across 6 547.6 ABC is **24 080 USD of
profit per ABC traded**, against an ABC worth ≈49 294 USD — **48.8% of notional
captured per unit**. No spread, no latency edge and no inventory skill produces
half of notional. It is the signature of picking off a counterparty quoting
around a mid that RT-031 showed had drifted −69% from fair.

**CORRECTION to RT-033, published because it was wrong in a way that matters.**
RT-033 stated the mechanism as `triangle_arb` "harvesting a self-referential
book's 69% dislocation **from uninformed flow configured into it**". The
counterparty table falsifies the second half: **`noise_flow` does not appear as a
direct counterparty of `triangle_arb` on any book.** The direct donor is
`abc_cdf_spot_maker`, the self-anchored maker itself. Falsifier (c) does not
fire, because the dominant counterparty is one of the two classes named — but the
half of the prediction naming `noise_flow` was wrong and the causal sentence in
RT-033 must be corrected rather than quietly re-read.

The corrected chain is a **two-step transfer, and the middle link is not a
loser**: `abc_cdf_spot_maker` pays 157.7 M to `triangle_arb` and still finishes
at **+6.88 M overall**, so it recovers ≈164 M from the rest of its flow. Which
class supplies that is **NOT EXERCISED** by this experiment — the tool
reconstructs counterparties for the focus class only. `noise_flow`'s −219 M is
still the population's funding, but the path from it to `triangle_arb` is
inferred, not measured.

**Next experiment, highest information gain.** Re-run `flowattrib` with
`-class abc_cdf_spot_maker` to close the chain: if its ≈164 M of recovery comes
from `noise_flow` on `ABC/CDF`, the whole campaign result is one mechanism end to
end. That is a single command against logs of the same run.


**E-050 — the chain closes exactly. The campaign's entire competitive result is
one mechanism, end to end.**
Same run and logs as E-049; the experiment E-049 named, run as specified.
Reproduce: `go run research/tools/flowattrib/main.go -dir <logdir> -class abc_cdf_spot_maker`.

`abc_cdf_spot_maker` on `ABC/CDF`, its only book:

| counterparty | value to the maker | base traded |
|---|---:|---:|
| **noise_flow** | **+188 620 463** | 354 838.7 ABC |
| abc_cdf_spot_maker | 0 | 17 200.7 ABC |
| imbalance_maker | −9 419 245 | 715.2 ABC |
| fixed_distance_maker | −14 598 989 | 1 148.3 ABC |
| **triangle_arb** | **−157 743 578** | 6 547.6 ABC |
| **net** | **+6 858 652** | |

The five rows sum to the class's total to the unit. The ≈164 M of recovery
E-049 could only infer is now measured: **188.6 M of it comes from `noise_flow`
on `ABC/CDF`**, exactly as the preregistered interpretation required. The
population's headline result is a single two-step transfer:

    noise_flow  ──+188.6M──▶  abc_cdf_spot_maker  ──−157.7M──▶  triangle_arb

**The two rates are the fairness verdict, and they are not comparable
quantities.**

| leg | notional | value | rate |
|---|---:|---:|---:|
| noise_flow → maker | 354 838.7 ABC | 188.6 M | **531 USD/ABC ≈ 1.08% of notional** |
| maker → triangle_arb | 6 547.6 ABC | 157.7 M | **24 092 USD/ABC ≈ 48.9% of notional** |

The maker collects an ordinary ~1% spread from uninformed flow — a market-making
result — and hands **83.6% of that gross away on 1.72% of its traded volume** to
one class, at forty-five times the per-unit rate it charges. A market maker does
not lose half of notional to adverse selection; it loses a spread. **This is not
adverse selection, it is a maker quoting around a mid that is −69% from fair
(RT-031) while a single class lifts the wrong side of it.**

**Incidental observation, POST-HOC.** `abc_cdf_spot_maker` trades **17 200.7 ABC
against itself** — the two cross makers on a venue crossing each other — at
exactly zero net value to the class. That is 2.6× `triangle_arb`'s entire volume
on the book, redistributed between same-class participants. It nets to zero at
class level so it does not affect any result above, and it is **NOT EXERCISED**
whether it distorts per-participant rankings inside the class or the book's
reported volume statistics. Flagged, not claimed.

Recorded as RT-035.


**H-047 (PREREGISTERED) — the scheduler grants permanent construction-order
privilege, so identical actors do not face a fair race.**

New mechanism family: **scheduler tie-breaking**, unrelated to every finding so
far (RT-031→RT-035 are all one cross-book artifact).

**Mechanism, from inspection.** `simulation/scheduler.go:163`:

    // Less orders by time, then by schedule sequence so equal-timestamp events
    // fire in FIFO order — heap sift order is not deterministic across runs.
    func (h eventHeap) Less(i, j int) bool {
        if h[i].Time != h[j].Time { return h[i].Time < h[j].Time }
        return h[i].id < h[j].id
    }

`id` is `es.nextID++` at **registration**. A repeating event is re-pushed with
its **Time advanced but its id unchanged** (`scheduler.go:150`). So among actors
sharing a tick interval, the firing order is fixed at construction and **never
rotates for the entire run**. Participant 1 of a class acts before participant 2
at every single shared tick, for eight simulated hours.

The tie-break itself is correct and deliberate — it buys determinism, which the
comment states. The question is whether it also buys a systematic edge.

**The natural experiment already exists in the campaign.** [[RT-032]] established
that all 8 `elastic_supplier` participants per venue are placed on **the same
book** with **identical configuration** — same reference, same elasticity, same
lot, same interval. They differ in exactly one respect: registration order. Any
systematic performance difference among them is ordering privilege and nothing
else.

**Prediction.** Per-participant carry-adjusted PnL within
`elastic_supplier` on a venue is **monotone decreasing in participant index**,
consistently signed across all three venues.

**Falsifiers.** (a) no consistent sign of the index/PnL relationship across the
three venues → H-047 falsified, the tie-break is deterministic but not
advantageous; (b) the spread across the eight participants is smaller than the
per-participant closure noise → INCONCLUSIVE at this instrument's resolution,
and a larger or repeated-seed design is required.

**Named confound, controlled by design.** For classes spread over several books
by `makerSymbol` round-robin, participant index also selects the *book*, so index
would correlate with result for a reason that has nothing to do with priority.
`elastic_supplier` is immune because `elastic_supplier_symbols` is null and all
eight sit on `ABC/USD` — which is why it is the chosen test group rather than the
larger maker classes.

**Discriminating experiment E-051**, preregistered before the run: re-run
`clock-control-5h-101.json` seed 607, extend `classpnl` with a per-participant
mode, and read carry-adjusted PnL by participant index within
`elastic_supplier` on each venue.
Status: **MIXED** by E-051 — the ordering effect is real and certain (24/24
monotone, p ≈ 1e−14) but its predicted *direction* is falsified (acting first is
worse) and its magnitude is 0.28%, below this instrument's resolution.


**E-051 — H-047's mechanism SUPPORTED, its predicted direction FALSIFIED.
Construction order determines outcome with certainty, and acting first is
*worse*.**
Base: `a666d02faede3d40f046b11e60eb672c59386a94`.
Run: `clock-control-5h-101.json`, seed 607, 8h, `-log-mode none`.
Reproduce: `go run research/tools/classpnl/main.go -file <logdir>/greeks.json -by-participant elastic_supplier`.

Carry-adjusted PnL of the eight identically-configured suppliers, in
registration order, on each venue:

| index | central | north | south |
|---:|---:|---:|---:|
| 1 | −84 444 | −84 777 | −84 464 |
| 2 | −84 393 | −84 733 | −84 432 |
| 3 | −84 346 | −84 686 | −84 400 |
| 4 | −84 316 | −84 649 | −84 358 |
| 5 | −84 282 | −84 608 | −84 339 |
| 6 | −84 262 | −84 576 | −84 295 |
| 7 | −84 237 | −84 522 | −84 261 |
| 8 | −84 204 | −84 509 | −84 220 |
| spread | 240 | 268 | 244 |

**Strictly monotone in registration order on all three venues — 24 of 24 rows,
no exception.** A random permutation of eight is sorted with probability
1/8! = 1/40 320; three independent venues agreeing gives ≈**1.5 × 10⁻¹⁴**. These
participants differ in *nothing* but the order they were constructed in: same
book, same reference, same elasticity, same lot, same interval, same endowment
([[RT-032]]). The ordering is not noise.

**The predicted direction is wrong.** I predicted PnL decreasing in index —
acting first is better. It is the reverse: **participant 1 is the worst on every
venue and participant 8 the best.** Acting first at a shared tick is a
*disadvantage* here. The suppliers are buying into a decline, and the first to
act each tick commits at the pre-trade mid while later participants recompute
their target against a mid their predecessors have already moved. I did not
predict this and am not going to dress it up as though I had.

**Magnitude: negligible for this class.** The spread is 240–268 USD on a
−84 400 USD result, **0.28%**. Construction order decides the ranking with
certainty and decides almost nothing about the money — for *these* actors.

**A mis-specified falsifier, recorded rather than quietly dropped.**
Preregistered falsifier (b) said the result is INCONCLUSIVE if the spread across
the eight is smaller than the per-participant closure noise. It is: the closure
residual is −4.95 M over 252 participants, ≈19 600 USD each, eighty times the
spread. **By the letter of my own falsifier this reads INCONCLUSIVE.** But (b)
was the wrong statistic for the claim: the closure residual is a systematic
unmodelled-transfer term, and an additive bias — per population or per venue —
**cannot manufacture a strict monotone ordering within a venue**. The ordering
test is scale-free and survives any bias that hits the eight equally. So: the
*ordering* claim is supported at p ≈ 1e−14; the *magnitude* claim is below this
instrument's resolution and is reported as a bound, not a value. The lesson is
that I wrote a level-uncertainty falsifier for an ordering hypothesis.

**Scope limit.** This experiment measures the *effect* of construction order.
The scheduler tie-break at `scheduler.go:163` is the mechanism identified by
inspection, and repeating events keep their id across firings
(`scheduler.go:148-150`), so the order is permanent. But this run does not
exclude some *other* construction-order-dependent iteration producing the same
signature. The two are not separated here.

**Why this matters more than 240 USD.** The privilege is permanent and applies to
**every class sharing a tick interval**. For a price-elastic buyer it is worth
0.28%. For a market maker under **price-time** matching — the `north` venue's
rule (RT-022) — acting first at a shared quote interval is **queue position at
the touch**, which is exactly what price-time priority pays for. That is where
this should be worth real money, and it is not measured here.

Recorded as RT-036.

**Next experiment, highest information gain.** Same per-participant measure on a
maker class, restricted to one book and the `north` (price-time) venue, comparing
against `central`/`south` (pro-rata, where queue position is worth nothing by
construction). If the index effect is large on price-time and absent on pro-rata,
construction order is buying queue priority and the makers' ranking is partly an
artifact of build order. The named confound applies — `makerSymbol` round-robin
makes index select the book — so the comparison must be within a single symbol.


**E-052 — the named follow-up, run immediately. Construction order buys queue
priority under price-time matching and **exactly nothing** under pro-rata. The
pro-rata venues are a built-in null control and they return zero.**
Same run and snapshot as E-051.
Reproduce: `go run research/tools/classpnl/main.go -file <logdir>/greeks.json -by-participant imbalance_maker`
(and `fixed_distance_maker`), then pair participants sharing a symbol —
`makerSymbol` round-robin over 4 books means index *i* and *i+4* sit on the same
book, which controls the confound named in H-047.

Each row is the same class, same book, same venue: two participants differing
only in construction order.

**`north` — price-time matching:**

| class | book | early | late | early edge |
|---|---|---:|---:|---:|
| fixed_distance_maker | ABC-PERP | 904 553 | 811 491 | **+11.5%** |
| fixed_distance_maker | ABC/CDF | 1 149 522 | 1 051 230 | **+9.4%** |
| imbalance_maker | ABC/CDF | 4 929 070 | 4 578 736 | **+7.6%** |
| imbalance_maker | ABC-PERP | 347 114 | 330 136 | **+5.1%** |
| fixed_distance_maker | ABC/USD | 521 | 372 | +40.1% |
| fixed_distance_maker | CDF/USD | −44 115 | −44 120 | + |
| imbalance_maker | ABC/USD | −1 331 | −1 749 | + |
| imbalance_maker | CDF/USD | −8 669 | −6 683 | − |

**Every pair differs, and the earlier-registered participant wins 7 of 8.**

**`central` and `south` — pro-rata matching:**

| class | book | early | late | difference |
|---|---|---:|---:|---:|
| fixed_distance_maker | ABC-PERP (central) | 723 941 | 723 941 | **exactly 0** |
| fixed_distance_maker | ABC/USD (central) | 741 | 741 | **exactly 0** |
| fixed_distance_maker | CDF/USD (central) | −43 383 | −43 383 | **exactly 0** |
| imbalance_maker | ABC-PERP (central) | 274 847 | 274 847 | **exactly 0** |
| imbalance_maker | ABC/USD (central) | −891 | −891 | **exactly 0** |
| imbalance_maker | CDF/USD (central) | −6 768 | −6 768 | **exactly 0** |
| fixed_distance_maker | ABC-PERP (south) | 658 336 | 658 336 | **exactly 0** |
| fixed_distance_maker | ABC/CDF (south) | 1 233 631 | 1 234 875 | 0.10% |
| imbalance_maker | ABC/CDF (central) | 4 076 369 | 4 076 368 | 0.00002% |

Across the sixteen pro-rata pairs the typical difference is **exactly zero to the
unit**, and the largest is **0.10%**. Across the eight price-time pairs the
effect is **5.1% to 11.5%** on the four economically material books. That is a
50–100× separation with the mechanism's exact fingerprint: **pro-rata allocates
by size, so acting first buys nothing; price-time allocates by arrival, so acting
first is queue position at the touch.**

This is the strongest form of the result because the null control was not
constructed for the occasion — the venue heterogeneity is the campaign's own
(RT-022), and it happens to switch the mechanism off.

**Conclusion, and it is a fairness defect rather than an accounting one.** On the
price-time venue, a maker's result is **5–11% determined by the order it was
constructed in** — a property of a `for` loop in `sim.go`, not of its strategy.
Two participants with byte-identical configuration do not face a fair race, and
the advantage never rotates because a repeating event keeps its scheduler id for
the entire run (`scheduler.go:148-150`). Any comparison of maker strategies on a
price-time venue in this simulator carries this bias.

**Scope.** Eight price-time pairs and sixteen pro-rata pairs from one seed. The
sign is consistent (7/8) and the venue contrast is unambiguous, but the effect
size per book rests on single pairs. Multiple seeds would tighten the magnitude;
they are not needed for the qualitative claim, which the exact-zero control
carries.

**This does not affect [[RT-031]]–[[RT-035]].** Those results are cross-venue
aggregates dominated by `ABC/CDF`, where `triangle_arb` takes 92.3% of its value
from a maker whose quoting is mispriced by 69% — an effect three orders of
magnitude larger than a queue-position edge.

Recorded as RT-037.


**H-048 (PREREGISTERED) — the 157.7 M transfer is driven by the −69% mispricing,
not by the cross maker's slow link. An ablation that could overturn
[[RT-035]].**

**Structural fact, from the effective config.** 13 roles have an explicit
`latency_profiles` entry; **15 inherit `default_latency_profile`, constant 5 ms**:
`abc_cdf_spot_maker`, `cdf_spot_maker`, `elastic_supplier`, `latent_liquidity`,
`metaorder_trader`, `round_trip`, `futures_maker`, `perp_exposure_hedger`,
`liability_hedger`, `bootstrap_depth`, `funding_carry_arb`,
`dated_execution_mandate`, `dated_term_carry_allocator`,
`term_carry_allocator`, `remote_maker_feed`.

Of the campaign's spot makers, **only `spot_maker` (ABC/USD) was given a fast
link** — lognormal 500 µs. `abc_cdf_spot_maker` and `cdf_spot_maker` are absent
from the table and silently take 5 ms. Meanwhile `triangle_arb` has the
**fastest deterministic link in the population: constant 800 µs, zero jitter**.

So the counterparty that pays `triangle_arb` 157.7 M ([[RT-034]]) is **6.25×
slower than it**, purely because a role name is missing from a config table.

**The suggestive pattern, POST-HOC.** Extraction from `abc_cdf_spot_maker` on
`ABC/CDF` orders exactly with link speed:

| taker | link | extracted |
|---|---|---:|
| triangle_arb | 800 µs constant | 157.7 M |
| fixed_distance_maker | 1 ms spiky | 14.6 M |
| imbalance_maker | 2 ms normal | 9.4 M |

Monotone. But these are different strategies with different sizes and roles, so
the ordering is **confounded and proves nothing on its own**. That is why an
ablation is needed rather than another correlation.

**Ablation.** Give `abc_cdf_spot_maker` the same link as `triangle_arb` —
`{"model":"constant","delay":800000}` — and change nothing else. This is a
config-only change in the audit worktree; no engine or economics code is touched.

**Prediction.** The transfer survives. A 4.2 ms staleness edge is worth basis
points in a book this slow; it cannot be worth **48.9% of notional** ([[RT-035]]).
So the mispricing is the channel and I predict **more than 70% of the 157.7 M
transfer remains**.

**Falsifiers.** (a) the transfer falls below **50%** of baseline → latency was
the dominant channel and RT-035's mechanism sentence needs revision, which would
be the second correction to that finding; (b) `triangle_arb`'s total collapses
while `ABC/CDF` stays −69% from bootstrap → the dislocation persists but its
harvesting is a latency race, a materially different story.

**Scope limit, stated up front.** Changing a latency profile changes the realized
event ordering, so this is **not a marginal ablation on a fixed path** — the
price trajectory will differ. The comparison is between regimes, not between two
versions of the same history. A large change in the *dislocation itself* would
therefore be a confound, so the cross book's terminal deviation from bootstrap is
reported alongside the transfer.

**Discriminating experiment E-053**, preregistered before the run: run the
ablation config at seed 607, 8h, full logs; report `triangle_arb`'s `ABC/CDF`
contribution, its extraction from `abc_cdf_spot_maker`, and the book's terminal
deviation from bootstrap, against the E-049/E-050 baseline of 170.9 M / 157.7 M /
−69.15%. Status: **INVALID** — E-053 shows latency has no effect at this step
size, so the ablation could not test what it was designed to test. Its question
is nonetheless answered: a latency race is impossible here.

**Also recorded, a bounded negative result.** `flowSeed`
(`sim.go:4003`) derives per-actor RNG streams by XOR-mixing venue, participant
and flow class. XOR mixing is not obviously injective, so a collision would make
two actors share a random stream. Brute-forced over venue 0–2, participant 0–39,
flow class 0–23 at seed 607: **2 880 tuples, 2 880 distinct seeds, 0 collisions.**
NOT EXERCISED beyond that box, and the nine flow classes actually used
(1, 2, 5, 9, 11, 13, 14, 15, 16) are pairwise distinct at every call site.


**E-053 — H-048's ablation is INVALID, and the reason is the finding: the entire
latency model is inert. Four latency configurations spanning 1 µs to 500 ms
produce byte-identical runs.**
Base: `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8h.

The preregistered ablation gave `abc_cdf_spot_maker` an 800 µs link. Result:
`triangle_arb` 170 904 176 on `ABC/CDF`, 157 661 074 from the cross maker,
6 547.6 base — **every digit identical to the baseline**. That is not "the
transfer survived"; a 6.25× link change cannot leave a run bit-identical. So I
stopped and tested the instrument instead of reporting the result.

**Escalation, each a full run, compared by md5 of `greeks.json`:**

| change | magnitude | greeks.json |
|---|---|---|
| baseline | — | `ac3a46fd…` |
| `abc_cdf_spot_maker` → 800 µs constant | 6.25× faster | `ac3a46fd…` **identical** |
| `abc_cdf_spot_maker` → 500 ms constant | 625× slower | **identical** |
| `noise_flow` → 1 µs constant | 20 000× faster | `ac3a46fd…` **identical** |
| `default_latency_profile` → 500 ms | 100× slower, hits 15 roles | `ac3a46fd…` **identical** |
| **seed 607 → 608** (positive control) | — | **all classes move**: `noise_flow` −220.2 M → −265.6 M, `triangle_arb` +174.2 M → +184.7 M |
| `default_latency_profile` → **3 s** | 3× the `step` | `b627d78f…` **changes** |

The positive control rules out a broken pipeline: the measurement detects change
when change exists. **Every configured latency is invisible; a delay of 3 s is
not.**

**Mechanism (inferred, not proven).** The config sets `"step": 1000000000` — a
**one-second** simulation step. Every configured delay is far below it: the
slowest is `noise_flow` at lognormal 20 ms capped at 500 ms, and the whole
per-role table spans 500 µs to 20 ms. The measured boundary lies in
**(500 ms, 3 s]**, consistent with delays being quantised away by the 1 s step.
I did not bisect the boundary further, so "the step is the cause" is the
consistent explanation rather than a demonstrated one.

**What is void.** The campaign's per-role latency heterogeneity has **no effect
on any outcome**: `triangle_arb`'s 800 µs "fastest link in the population",
`noise_flow`'s 20 ms lognormal tail, `fixed_distance_maker`'s 1% chance of a
50 ms spike, `spot_maker`'s 500 µs advantage over the two cross makers that
inherit 5 ms — all decorative. Any claim in this campaign about latency
arbitrage, link-based information asymmetry, or fast-versus-slow participants is
**NOT EXERCISED** at this step size. The validation at `sim.go:709`/`:821` that
refuses to run without "an explicit nonzero delayed link" enforces a field that
changes nothing, which is worse than no check: it reads as assurance.

**H-048 status: the experiment is INVALID as designed** — a latency ablation
cannot be run in a configuration where latency does nothing. Its preregistered
prediction and falsifiers are moot and are not scored.

**But the null result settles H-048's question by a stronger route.** I wanted to
know whether the 157.7 M transfer is a mispricing effect or a latency race. If
latency has *no effect at all*, a latency race is not merely unlikely — it is
**impossible** in this configuration. So [[RT-035]]'s mechanism stands, and the
monotone "extraction orders with link speed" pattern recorded in H-048 is
**refuted as causal**: `triangle_arb` 157.7 M, `fixed_distance_maker` 14.6 M,
`imbalance_maker` 9.4 M order with configured link speed **coincidentally**,
since those links are inert. That pattern must not be cited. I recorded it as
confounded when I wrote it; it is now known to be spurious.

**Owner decision.** Either the `step` must drop far below the modelled latencies
for the link model to bite, or the latency configuration should be recognised as
inactive at this resolution. Which one is the owner's call; the current state —
an elaborate, validated, per-role latency table that provably changes nothing —
is the one option that misleads.

Recorded as RT-038.

**Instrument note.** The 3 s run's closure residual is **2.2582% of gross**, and
`classpnl` **refused to print a ranking** (exit 3), exactly as its self-test was
built to do. No numbers from that run are quoted beyond the fact that it differs.


**H-049 (PREREGISTERED) — [[RT-038]]'s inertness is caused by the step size, not
by broken latency plumbing.**

E-053 established that latency changes do nothing and *inferred* the 1 s `step`
as the cause without proving it. The two candidate diagnoses have completely
different fixes, so the distinction is worth one cheap experiment:

- **Resolution artifact** — delays below `step` are quantised away. Fix: reduce
  `step`. The latency model is correct and merely out-resolved.
- **Broken plumbing** — latency never reaches delivery at any resolution. Fix: a
  code defect in the mount/gateway path. Materially worse, and it would mean the
  link model has never worked in any configuration.

**Design: a 2×2, comparing *within* each step regime so the step change is never
itself the comparison.**

| cell | step | `default_latency_profile` |
|---|---|---|
| A | 1 s | 5 ms (as configured) |
| B | 1 s | 500 ms |
| C | 1 ms | 5 ms |
| D | 1 ms | 500 ms |

Duration 2 m for every cell, seed 607, compared by md5 of `greeks.json`.
Comparing A↔B and C↔D keeps `step` fixed inside each contrast, so a difference
is attributable to latency alone.

**Prediction.** **A = B** (reproducing E-053 at short duration) and **C ≠ D**
(latency bites once the step is finer than the delay). That is the resolution
artifact.

**Falsifiers.** (a) **C = D** → the step is *not* the cause and the latency path
is inert at any resolution — a code defect, and RT-038's diagnosis must be
rewritten from "out-resolved" to "broken"; (b) **A ≠ B** → E-053's byte-identical
result does not reproduce at 2 m, meaning the effect is duration-dependent and
the whole RT-038 measurement needs re-examination.

**Discriminating experiment E-054**, preregistered before any cell is run.
Status: **SUPPORTED WITHIN TESTED SCOPE** — A = B byte for byte, C ≠ D; neither
falsifier fired.


**E-054 — H-049 SUPPORTED WITHIN TESTED SCOPE. [[RT-038]]'s inertness is a step
resolution artifact, not broken plumbing. The latency model works; the 1 s step
out-resolves it.**
Base: `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 2 m simulated, four
runs.
Reproduce: run the four cells below and compare `md5sum <logdir>/greeks.json`.

| cell | step | default latency | wall | `greeks.json` |
|---|---|---|---:|---|
| A | 1 s | 5 ms | 1 s | `fd2541239f4a` |
| B | 1 s | **500 ms** | 1 s | `fd2541239f4a` |
| C | 1 ms | 5 ms | 1 m 59 s | `075d1737b1b6` |
| D | 1 ms | **500 ms** | 2 m 5 s | `945defb36c8d` |

**A = B, byte for byte.** A hundredfold latency change at the campaign's 1 s step
does nothing — reproducing E-053 at a 240× shorter horizon, so the effect is not
duration-dependent.

**C ≠ D.** The *same* hundredfold latency change at a 1 ms step changes the run.

Because `step` is held fixed inside each contrast, the difference is attributable
to latency alone. **Neither falsifier fired**: C ≠ D rules out broken plumbing,
and A = B rules out a duration artifact.

**RT-038's diagnosis is upgraded from inferred to demonstrated.** The latency
machinery — mounts, per-role profiles, per-client sample paths — is **correct and
functioning**. It is simply invisible at a step 200× coarser than the slowest
configured delay. The fix is a resolution choice, not a code repair.

**And the resolution choice has a price, now measured.** A 1 ms step costs
**~120× more wall time** for the same simulated span: 2 m of sim took 1 s at the
1 s step and 1 m 59 s at 1 ms. Extrapolating, the campaign's 8 h run would go
from ≈4 minutes to **≈8 hours of wall time**. That is the actual trade the owner
faces, and it explains why the coarse step was chosen. It does not change the
conclusion that every latency-based claim is currently unexercised — it only
prices the remedy.

**The owner's options, stated neutrally.** (1) Keep the 1 s step and treat the
per-role latency table as inactive, removing the validation at `sim.go:709`/
`:821` that currently reads as assurance. (2) Drop the step to resolve the
modelled delays and accept ≈120× compute. (3) Keep the coarse step for
population-scale questions and run a separate fine-step configuration for the
latency questions specifically. Choosing among these is not mine to do.


**H-050 (PREREGISTERED) — the closure residual is not noise: it *is* the exchange
take, counted once where it should be counted twice (or charged twice where it is
recorded once).**

**Mined from a residual I had been dismissing.** Every `classpnl` run this session
printed the same three numbers at seed 607:

    Σ carry-adjusted pnl   −9 890 673 USD
    exchange take          +4 944 035 USD
    residual               −4 946 638 USD   (0.8866% of gross)

I reported that residual as a magnitude — "0.89% of gross, admissible but
narrow" — and moved on. Its **structure** is far more informative:

    |residual| / take = 4 946 638 / 4 944 035 = 1.00053

The residual is the take, to five parts in ten thousand. Equivalently
**Σ carry_adjusted ≈ −2 × take**.

**Why that is a defect and not an identity.** Fees move value from participants to
the venue. Participants should lose exactly what the venue gains, so
`Σ Δequity = −take` and, with revaluation removed,
`Σ carry_adjusted + take = 0`. Observed instead is
`Σ carry_adjusted + take = −take`. Participants are down **twice** what the
ledger says the venue collected. One of these is true:

1. participants are charged twice per fill and the ledger records once;
2. the ledger records one side of a two-sided charge (but `MakerBps` is 0, so
   there should be only a taker side);
3. a second sink exists that is not `FeeRevenue + InsuranceFund` — funding,
   borrow interest, or a liquidation transfer — and coincidentally equals the
   take;
4. my `classpnl` take extraction reads only part of the ledger.

**(4) is mine and must be excluded first**, so the experiment reads the ledger
fields directly rather than trusting the tool's aggregate.

**Prediction.** Across independent seeds, `|residual| / take` stays ≈ 1 with a
tight spread. A structural miscount reproduces; a coincidence does not.

**Falsifiers.** (a) the ratio varies materially across seeds (outside ±10%) →
seed 607 was a coincidence, H-050 falsified, and the residual returns to being an
unexplained magnitude; (b) the ratio is ≈1 but a non-fee sink of the same size is
identified → explanation (3), which is a different finding and not a
double-count.

**Consequence if supported.** Every closure self-test in this session
([[RT-033]], [[RT-034]]) passed at 0.89% *because the tolerance was 1%*. If the
residual is a structural double-count rather than noise, the tolerance was
absorbing a real defect, and the "admissible but narrow" caveat I attached to
RT-033 was the right instinct for the wrong reason.

**Discriminating experiment E-055**, preregistered before the runs: repeat at
seeds 607, 608, 609 at the 8 h horizon; print `Σ carry-adjusted`, the raw ledger
`fee_revenue` and `insurance_fund` per venue and asset, and the ratio.
Status: **FALSIFIED WITHIN TESTED SCOPE** — the ratio does not reproduce across
seeds, and the cause is explanation (4), my own tool discarding non-USD fee
revenue.


**E-055 — H-050 FALSIFIED. The residual was my own tool discarding non-USD fee
revenue. Corrected, the population accounting closes to 0.0001%, and two earlier
scope limits I published were far too conservative.**
Base: `a666d02faede3d40f046b11e60eb672c59386a94`, 8 h, seeds 607/608/609.
Reproduce: `go run research/tools/classpnl/main.go -file <logdir>/greeks.json`.

**Falsifier (a) fires on the seed sweep.** `|residual| / take` was 1.00053 at
seed 607, 1.00450 at 608, but **0.81955 at 609** — an 18% spread, outside the
preregistered ±10% band. The "residual is exactly the take" pattern does not
reproduce, so the double-charge reading was wrong.

**Falsifier (4) — the explanation I flagged as mine and required to be excluded
first — is the actual cause.** Dumping the raw ledger per asset:

| seed | `fee_revenue/USD` | `fee_revenue/CDF` | `fee_revenue/ABC` |
|---|---:|---:|---:|
| 607 | 494 403 495 651 | **153 469 520 553** | 89 805 |
| 608 | 463 937 957 961 | **155 245 978 261** | 73 893 |
| 609 | 502 372 867 994 | **154 672 166 782** | 93 574 |

**The venue takes fees in whichever asset the book quotes.** `ABC/CDF` fees
accrue in **CDF** — visible all along in the fill records as
`"fee_asset": "CDF"`. `classpnl` summed `FeeRevenue["USD"]` only, discarding
≈1 534 CDF ≈ 4.6 M USD per run: almost exactly the size of the USD take, which is
what manufactured the near-perfect 1.0 ratio at two of three seeds.

**Corrected closure, same runs, same carry-adjusted numbers, take now valued
across all assets at terminal marks:**

| seed | Σ carry-adjusted | take | residual | of gross |
|---|---:|---:|---:|---:|
| 607 | −9 890 673 | +9 891 169 | **+496** | **0.0001%** |
| 608 | −9 299 639 | +9 299 900 | **+261** | **0.0000%** |
| 609 | −9 140 911 | +9 144 241 | **+3 330** | **0.0005%** |

**This is a strong positive result about the simulator that I had been
reporting as a weakness.** The population's entire trading loss equals the venue's
entire take to **496 USD out of ~558 M of gross flow**. Conservation across
participants, venues and three assets holds essentially exactly. My published
figure of "0.8866% of gross, admissible but narrow" understated the accounting's
accuracy by nearly four orders of magnitude.

**CORRECTION 1 — [[RT-033]]'s resolution limit was wrong.** I wrote that no class
below ≈±5 M was distinguishable from the residual, and listed **twelve of twenty
classes** as NOT EXERCISED on that basis. With the residual at ~500 USD, **every
class is resolved**, including `elastic_supplier` (−2 026 313) and
`latent_liquidity` (−1 029 326). The ranking itself is unchanged — carry-adjusted
PnL never depended on the take — but the uncertainty I attached to it was
inflated by my own defect. The twelve classes are reinstated as measured.

**CORRECTION 2 — [[RT-036]]'s mis-specified falsifier is resolved, in its
favour.** E-051 recorded that preregistered falsifier (b) compared the suppliers'
240 USD spread against "per-participant closure noise" of ≈19 600 USD, so by its
letter the result read INCONCLUSIVE, and I argued the ordering claim survived
because monotonicity is scale-free. With the corrected residual the
per-participant noise is ≈**2 USD**, not 19 600. The 240 USD spread is two orders
of magnitude above it. **Falsifier (b) does not fire at all**, and RT-036's
*magnitude* claim — construction order is worth 0.28% to a price-elastic taker —
is supported rather than bounded. My scale-free defence was correct but was not
needed.

**Instrument defect, and its blast radius.** `research/tools/populationclosure`
carries the same single-asset take (`l.FeeRevenue[*asset]`). Its published
outputs are affected the same way and any residual it reported for a cross-asset
run is overstated by the discarded CDF fees. Not re-run here; flagged.
`flowattrib`'s cross-check never used the take, which is why it independently
reconciled to −0.0% and gave no hint of a problem — a useful reminder that the
tool which agreed with reality was the one that did not depend on the broken
term.

**Why this was missed for four checkpoints.** The self-test was designed to
refuse a bad decomposition and it never fired, because the defect sat *inside*
the reference quantity the test compares against, and it happened to leave the
residual just under the 1% tolerance. A gate calibrated in a single asset cannot
detect a multi-asset omission. The lesson is not "tighten the tolerance" — at 1%
the run passed legitimately — but **that a conservation check must enumerate
every asset the system can move value in**, not the one the report is
denominated in.

Recorded as RT-039.


**H-051 (PREREGISTERED) — the fill-vs-snapshot gaps for spot-only classes are my
tools collapsing per-venue marks into one global map, not a simulator accounting
term.**

**Why the gap should be exactly zero.** For a participant holding no derivative
position, equity is `Σ_asset net_asset × mark`. Terminal net asset is the initial
balance plus the fill-derived delta, so

    carry_adjusted = Δequity − Σ net_init × Δmark
                   = Σ (net_final − net_init) × mark_final
                   = Σ fill_delta_asset × mark_final

and `flowattrib`'s contribution is, by construction,
`Σ_books (base_delta × mark_base + quote_delta × mark_quote)` = the same sum.
**They are algebraically identical.** For a spot-only class the gap must be 0.

**Observed instead** (E-049, unchanged by RT-039 since neither quantity uses the
take):

| class | from fills | from snapshots | gap | per head |
|---|---:|---:|---:|---:|
| abc_cdf_spot_maker | 6 858 652 | 6 879 438 | **−20 786** | −3 464 |
| cdf_spot_maker | 444 956 | 441 276 | **+3 680** | +613 |
| elastic_supplier | −2 028 598 | −2 026 313 | **−2 285** | −95 |
| latent_liquidity | −1 030 003 | −1 029 326 | −677 | −38 |

Non-zero where the algebra says zero.

**The suspected cause is in my own tooling, again.** Both `classpnl` and
`flowattrib` build `endMarks` by iterating terminal rows and writing
`endMarks[asset] = mark` — **last write wins**. Marks are **per venue**, and
E-047 measured them differing: ABC terminal mark is 4 929 505 000 on `central`,
4 929 380 000 on `north`, 4 929 375 000 on `south`. So one venue's marks are
applied to all three venues' participants. A 0.0026% mark spread on a few
thousand units of inventory produces a discrepancy of exactly this order:
20 786 USD at ~1.28 USD per ABC of mark error implies ≈16 200 ABC of inventory,
which is the right scale for six cross makers.

`classpnl`'s per-class revaluation is **not** affected — it uses `row.Marks`, the
participant's own row. The contaminated paths are `flowattrib`'s contribution and
carry-adjusted recomputation, and the multi-asset take conversion added in
RT-039.

**Prediction.** With marks keyed by venue, spot-only classes reconcile to
**|gap| < 0.01%** (currently 0.1%–0.8%), and the aggregate closure stays at its
RT-039 level or improves.

**Falsifiers.** (a) gaps persist above 0.05% after the fix → a genuine
non-fill accounting term exists for spot-only participants and this becomes a
finding about the simulator rather than about my tools; (b) the fix moves any
**published class ranking** materially → RT-033/RT-034's numbers were affected
and must be reissued, not merely annotated.

**Discriminating experiment E-056**, preregistered before the change: key marks
by `(venue, asset)` in both tools, re-run against the same run, and compare the
gap table and the closure.
Status: **SUPPORTED WITHIN TESTED SCOPE** — closure exactly 0, spot-only classes
within 1 USD, neither falsifier fired.


**E-056 — H-051 SUPPORTED. Per-venue marks drive the closure to exactly zero and
reconcile every spot-only class to within 1 USD across 5.13 M fills.**
Base: `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, `-log-mode full`.
Reproduce: `go run research/tools/classpnl/main.go -file <logdir>/greeks.json`
and `go run research/tools/flowattrib/main.go -dir <logdir>`.

**Closure, after keying the take's mark conversion by venue:**

    Σ carry-adjusted pnl   −9 890 673 USD
    exchange take          +9 890 673 USD
    residual                        −0 USD   (0.0000% of gross)

**Exactly zero.** RT-039 had brought this to +496 USD; the remaining 496 was the
same class of defect one level down — one venue's marks pricing another venue's
fee revenue.

**Spot-only reconciliation, fill stream against account snapshots — two
independent computations over the same run:**

| class | before | after | 5.13 M fills |
|---|---:|---:|---|
| abc_cdf_spot_maker | −20 786 | **+1** | 6 879 439 vs 6 879 438 |
| cdf_spot_maker | +3 680 | **+1** | 441 277 vs 441 276 |
| elastic_supplier | −2 285 | **+1** | −2 026 314 vs −2 026 313 |
| latent_liquidity | −677 | **0** | −1 029 326 vs −1 029 326 |
| metaorder_trader | ~0 | **0** | −263 779 vs −263 779 |
| round_trip | ~0 | **0** | −94 128 vs −94 128 |
| triangle_arb | −14 436 | **−3** | 174 217 978 vs 174 217 981 |

Every spot-only class lands within **3 USD**, against the preregistered threshold
of 0.01%. The algebra in H-051 said these must be identical; they now are, so the
gaps were my tools and there is **no unexplained non-fill accounting term** for
spot participants. Falsifier (a) does not fire.

`noise_flow` remains at 0.3% and the derivative classes at ±100%, both expected
and unchanged in meaning: those participants trade `ABC-PERP` and the option and
futures books, which this tool deliberately does not fold in.

**Falsifier (b) does not fire either — no published ranking moves materially.**
`classpnl`'s per-class carry-adjusted figures are **bit-identical** to those in
RT-033, because that path always used `row.Marks`, the participant's own row. The
fill-attribution numbers move by ~0.01%: `triangle_arb`'s `ABC/CDF` contribution
170 904 176 → 170 923 896, and its extraction from `abc_cdf_spot_maker`
157 661 074 → 157 679 289. **RT-034's headline percentages are unchanged**: the
cross book is 98.1% of the class result and the cross maker 92.3% of that. The
findings are annotated, not reissued.

**The instrument is now materially stronger than when RT-033 was written.** Two
computations built from different sources — a 5.13 M-record fill stream and 252
account snapshots — agree to 1 USD, and the population's trading loss equals the
venue's take to the unit. That is a genuine conservation result for the
simulator, and it is the third time in two checkpoints that a discrepancy I was
prepared to attribute to the system turned out to be my own measurement.

**The recurring defect has one shape.** RT-039 was "a value can live in an asset
the tool does not enumerate". E-056 is "a value can live in a venue the tool does
not enumerate". Both are the same mistake — **collapsing a dimension the system
actually varies over** — and both hid inside a quantity a self-test compared
against, which is why neither gate fired. The general rule for the remaining
work: **before trusting a reconciliation, list every dimension the system can
price things along, and confirm the instrument keys on all of them.** Asset and
venue are now covered; time is not, and a mark is a point-in-time quantity.

Recorded as RT-040.


**H-052 (PREREGISTERED) — [[RT-034]]/[[RT-035]]'s concentration result is
structural, not a property of seed 607.**

**Why this is overdue.** Every headline in this session — the 98.1% book share,
the 92.3% counterparty share, the 48.9%-of-notional extraction rate — comes from
**one seed**. The protocol requires a fresh run before a result is promoted to
supported, and I have not done it for the campaign's largest claim. Until now the
claim is properly `INCONCLUSIVE ACROSS SEEDS`, whatever its internal evidence.

**There is already a reason to doubt naive transfer.** The seed sweep in E-055
showed `abc_cdf_spot_maker` at **+6 879 438 (seed 607)** but **+36 922 938 (seed
608)** — a 5.4× swing in the very counterparty the mechanism runs through, and
`noise_flow` at −220.2 M vs −265.6 M. The *levels* clearly move a lot between
seeds. The question is whether the *structure* does.

**Claims under test, stated as thresholds before the runs:**

1. `ABC/CDF` is **≥90%** of `triangle_arb`'s total contribution (607: 98.1%).
2. `abc_cdf_spot_maker` is **≥80%** of `triangle_arb`'s `ABC/CDF` contribution
   (607: 92.3%).
3. `triangle_arb`'s extraction rate from the cross maker is **≥20% of notional**
   (607: 48.9%) — the claim that this cannot be spread or adverse selection.
4. The cross book's terminal price is **≥30% below** its bootstrap (607: −69.15%),
   i.e. [[RT-031]]'s dislocation is not seed-specific.

**Falsifiers.** Any of the four thresholds missed on **either** seed 608 or 609
falsifies H-052 for that claim, and the corresponding finding is downgraded from
a general statement about the configuration to a statement about seed 607.
Partial failure is reported per claim rather than averaged into a verdict.

**What would count as the interesting outcome.** If levels swing 5× while the
four structural ratios hold, that is stronger evidence for the mechanism than the
original single-seed measurement: it would show the dislocation and its
harvesting are properties of the configuration, with the seed setting only the
scale.

**Discriminating experiment E-057**, preregistered before either run: seeds 608
and 609, 8 h, `-log-mode full`, `flowattrib` for the shares and `pricerange` for
the cross book's deviation from bootstrap.
Status: **SUPPORTED WITHIN TESTED SCOPE** — all four thresholds pass on both
reproduction seeds.


**E-057 — H-052 SUPPORTED on all four claims and all three seeds. The levels
swing 5×; the structure holds to under one percentage point.**
Base: `a666d02faede3d40f046b11e60eb672c59386a94`, 8 h, `-log-mode full`, seeds
607/608/609. Reproduce: `flowattrib -dir <logdir>` and
`pricerange -dir <logdir> -symbol ABC-CDF -reference 1666666667`.

| seed | `ABC/CDF` share | maker share | USD per ABC | % of notional | base traded | cross book last |
|---|---:|---:|---:|---:|---:|---:|
| 607 | 98.11% | 92.25% | 24 082 | 48.85% | 6 547.6 | −69.15% |
| 608 | 99.14% | 92.48% | 25 929 | 52.49% | 6 531.0 | −82.86% |
| 609 | 99.63% | 92.96% | 28 104 | 57.12% | 6 766.4 | −83.15% |
| **threshold** | **≥90%** | **≥80%** | — | **≥20%** | — | **≤−30%** |

**All four preregistered thresholds pass on every seed. No falsifier fires.**

**The reproduction is stronger than the original measurement, for the reason
preregistered as "the interesting outcome".** The *levels* move a great deal
between seeds — `triangle_arb`'s total is 174.2 M / 184.7 M / 205.3 M, and
`abc_cdf_spot_maker`'s **net** result swings 5.4× (+6.88 M at 607 to +36.92 M at
608, E-055). Against that, the structural ratios are nearly constant:

- book share **98.11 → 99.63%**, a 1.5 pp spread;
- counterparty share **92.25 → 92.96%**, a **0.71 pp spread**;
- base traded against the maker **6 547.6 / 6 531.0 / 6 766.4 ABC**, a 3.6%
  spread.

A quantity that stable across seeds whose levels swing 5× is not a coincidence of
one run. The near-constant traded quantity is the signature of the position caps
([[RT-028]]): the arbitrageur trades **about the same amount every time** and the
seed sets only what each unit is worth.

**A relationship I did not preregister, so POST-HOC.** Extraction rate rises with
the depth of the dislocation across the three seeds — −69.15% / 48.85%,
−82.86% / 52.49%, −83.15% / 57.12%. The ordering is monotone and the direction is
what the mechanism predicts, but three points with two nearly tied on the
independent variable is not evidence of a quantitative law, and I am not claiming
one. It is consistent with, not confirmation of, [[RT-035]]'s causal story.

**Status changes.** [[RT-031]], [[RT-034]] and [[RT-035]] move from single-seed
to **reproduced across three independent seeds**. Their headline percentages
should be quoted as ranges — book share 98–99.6%, counterparty share 92.3–93.0%,
extraction 49–57% of notional, cross-book terminal deviation −69% to −83% — not
as the seed-607 point values I published.

**What this does not establish.** Three seeds of one configuration. Nothing here
speaks to other configurations, other horizons, or the campaign's other
scenarios, and the dislocation's *depth* is clearly seed-sensitive (−69% to
−83%). The claim promoted is "structural within this configuration", not
"universal".

**Method note.** This should have been run before RT-034 was written, not four
checkpoints later. The protocol requires a fresh run before promoting a result,
and I published three findings' worth of percentages from a single seed first.
The reproduction happened to support them; that is luck, not process.


**H-053 (PREREGISTERED) — the funding rate is not scaled by the settlement
interval, so identical perp positions bear ~8× the funding drag on the 1 h venue
as on the 8 h venue.**

**New mechanism family: the derivative side.** `flowattrib`'s gap column showed
`futures_maker` (+7.06 M), `perp_maker` (+2.48 M), `option_dealer`, `option_flow`,
`future_flow`, `option_value_taker` and `vanna_volga_desk` earning **nothing** in
a spot order book — their entire result comes from books my instruments have
never touched. This is the first hypothesis in that family.

**Mechanism, from inspection.** `SimpleFundingCalc.Calculate(indexPrice,
markPrice)` (`instrument/funding.go:20`) takes **no interval argument**: the rate
is `BaseRate + Damping × premium`, clamped to `±MaxRate`, where the premium is
purely `(mark − index)/index`. The settlement then applies it directly
(`exchange/funding.go:753`):

    funding, ok := etypes.TryMulDiv(positionValue, fundingRate.Rate, 10000)

**No interval term anywhere.** `fundingRate.Interval` appears only in
`nextFundingTimestamp`, which schedules the *next* settlement — it never scales
the *amount*.

**Consequence.** RT-022's deliberate venue heterogeneity sets funding intervals of
1 h (`central`), 2 h (`south`) and 8 h (`north`). Over an 8 h run the same
position is charged the same per-settlement rate **8, 4 and 1 times**
respectively. That is a funding cost that differs by **8× purely from a
scheduling parameter**, not from any market condition. Real venues quote funding
per interval and prorate; this model charges a full interval's rate however often
it is asked.

**Prediction.** Per venue, the **mean absolute rate per settlement** is
comparable (same formula, similar books), while the **cumulative Σ|rate| over
the run** differs in approximately the ratio of settlement counts, ≈**8 : 4 : 1**
for central : south : north.

**Falsifiers.** (a) cumulative Σ|rate| is comparable across venues (all within
2× of each other) → some scaling exists that inspection missed, H-053 falsified;
(b) settlement counts are not 8/4/1 → the interval is not doing what the config
says and the finding is about scheduling instead; (c) mean per-settlement rate
differs by ≈8× in the opposite direction → the premium itself is
interval-sensitive and the effect cancels, which would be a different and much
subtler mechanism.

**What this is not.** Whether funding *should* be time-scaled is a modelling
choice and the owner's to make — `BaseRate` and `MaxRate` are configurable per
venue and could in principle be set to compensate. The finding is that the
current configuration does **not** compensate, so cross-venue perp results are
not comparable.

**Discriminating experiment E-058**, preregistered before the run: 8 h,
`-log-mode full`, seed 607; parse `funding_settlement` events per venue and
report settlement count, mean |rate|, and cumulative Σ|rate|.
Status: **SUPPORTED WITHIN TESTED SCOPE** on the primary claim; auxiliary
falsifier (b) fired on a mis-derived count prediction that was never the claim.


**E-058 — H-053 SUPPORTED, and more extreme than predicted: funding is not
interval-scaled, and the 8 h venue settles funding **zero times** in an 8 h run.**
Base: `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, `-log-mode full`.
Reproduce: count `funding_settlement` events per venue in the log tree.

| venue | configured interval | settlements | mean \|rate\| | cumulative \|rate\| | ABC-PERP fills |
|---|---:|---:|---:|---:|---:|
| central | 3 600 s | 7 | 38.57 bps | **270 bps** | 61 694 |
| south | 7 200 s | 3 | 28.00 bps | **84 bps** | 60 210 |
| north | 28 800 s | **0** | — | **0 bps** | 59 572 |

The `interval` field in the settlement events reads 3600 and 7200, exactly as
`venue_rules` configures, so the scheduler is doing what it is told.

**The primary claim is supported.** Mean per-settlement rate is comparable across
venues (38.6 vs 28.0 bps, within 1.4×), while cumulative funding differs by
**270 : 84 : 0**. Falsifier (a) — cumulative rates comparable within 2× — does not
fire. An identical perp position bears a **2.7% funding drag on `central` and
exactly none on `north`**, decided entirely by a scheduling parameter.

**The larger result was not predicted: `north`'s perpetual has no funding at
all.** Its first settlement would fall at t = 8 h, the horizon itself, and does
not fire. Yet the book trades **59 572 fills** — comparable to the other two
venues. So for the whole campaign, `north` runs a "perpetual" with **no funding
mechanism whatsoever**: the one economic device that tethers a perp's mark to its
index is absent, leaving an instrument with no convergence force. This is the
same shape as [[RT-031]] — an instrument with nothing anchoring it — reached by a
completely different route.

**Falsifier (b) fired as written, and I am recording that rather than reading
past it.** I predicted settlement counts of 8 / 4 / 1; the observed counts are
**7 / 3 / 0**, because settlements land at strict multiples of the interval
inside the horizon — 1 h…7 h on `central`, 2 h/4 h/6 h on `south`, and never on
`north`. My falsifier said such a mismatch would mean "the interval is not doing
what the config says and the finding is about scheduling instead". That
inference is **wrong on the evidence**: the logged `interval` values match the
config exactly, so the scheduler is correct and my arithmetic about boundary
conventions was what failed. The falsifier fired on a prediction I had no need to
make — settlement *counts* were never the claim, cumulative *rate* was — and it
does not touch the mechanism. Recorded as a badly-chosen auxiliary prediction,
the third preregistration-design error in this campaign after E-051's
level-uncertainty falsifier and E-053's untestable ablation.

**What is the owner's decision.** Whether funding should be time-scaled is a
modelling choice, and `BaseRate`/`MaxRate` are per-venue configurable, so a
compensating configuration is possible. What is not a choice: the current
configuration does **not** compensate, so **cross-venue perp results are not
comparable**, and any conclusion drawn from comparing perp behaviour on `north`
against the other two venues is comparing a funded instrument with an unfunded
one.

**Scope.** One seed, one configuration, one horizon. The `north` zero-settlement
result is a horizon boundary effect and would change with a longer run — at 16 h
it would settle once. That does not soften the finding for **this** campaign,
whose every published run is 8 h.

Recorded as RT-042.


**H-054 (PREREGISTERED) — the funding controller is saturated: the mark/index
premium routinely exceeds the ±75 bps cap, so funding supplies no proportional
restoring force. Plus a direct test of [[RT-042]]'s boundary explanation.**

**Parameters, from `sim.go:2757`:**

    &instrument.SimpleFundingCalc{BaseRate: 1, Damping: 100, MaxRate: s.Config.FundingMaxRateBps}

with `funding_max_rate_bps: 75`. `Damping: 100` enters the formula as
`premium × Damping / 100`, i.e. a multiplier of **1.0 — no damping at all**. The
rate is therefore the raw mark-to-index premium in bps, plus 1 bp, hard-clamped
at ±75 bps.

**Why saturation matters.** A funding rate pinned at its cap is a bang-bang
signal: once the premium exceeds 0.75%, further divergence produces **no
additional restoring force**, and the rate stops carrying information about the
premium's size. Every basis and carry strategy in the population —
`carry_arb`, `funding_carry_arb`, `dated_carry_arb` — trades on that signal.
E-058 measured mean |rate| of **38.57 bps on `central` and 28.00 on `south`
against a cap of 75**, which is high enough to suspect the cap binds often.

**Second claim, testing my own explanation.** RT-042 attributed `north`'s zero
settlements to a horizon boundary — its first settlement falls at t = 8 h, the
end of the run — rather than to a broken scheduler. That explanation makes a
sharp prediction at a **12 h** horizon: `north` should settle **exactly once**,
at t = 8 h.

**Predictions.**
1. **≥25%** of settlements sit exactly at the ±75 bps cap.
2. At 12 h: `north` settles **exactly 1** time, `south` **5**, `central` **11**
   (strict interval multiples inside the horizon).

**Falsifiers.**
(a) **<5%** of settlements at the cap → not saturated, claim 1 falsified and the
funding signal is proportional after all;
(b) `north` ≠ 1 at 12 h → RT-042's boundary explanation is **wrong**, the
scheduler is doing something else, and that finding must be reopened;
(c) `central` ≠ 11 or `south` ≠ 5 → the strict-multiple rule I inferred in E-058
is wrong, and the count arithmetic that already tripped falsifier (b) there is
still not right.

**Cost note.** A 12 h full-log run is ≈18 GB against 32 GB of tmpfs. Chosen over
24 h for that reason; it still discriminates every prediction above.

**Discriminating experiment E-059**, preregistered before the run: seed 607,
`-duration 12h`, `-log-mode full`; report per-venue settlement counts and the
full distribution of settled rates including the fraction at ±`MaxRate`.
Status: **SUPPORTED WITHIN TESTED SCOPE** on both claims; no falsifier fired.


**E-059 — H-054 SUPPORTED on both claims. The funding controller saturates and
stays saturated, and [[RT-042]]'s boundary explanation is confirmed exactly.**
Base: `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, **12 h**,
`-log-mode full`, 18 GB.
Reproduce: parse `funding_settlement` events per venue from the log tree.

| venue | settlements | at ±75 cap | mean \|rate\| | settled rates (bps, in order) |
|---|---:|---:|---:|---|
| central | 11 (h=1…11) | **7 (64%)** | 51.8 | −14, −1, −75, −9, −21, **−75, −75, −75, −75, −75, −75** |
| south | 5 (h=2,4,6,8,10) | **3 (60%)** | 46.8 | 1, −8, **−75, −75, −75** |
| north | 1 (h=8) | **1 (100%)** | 75.0 | −75 |

**Claim 1 supported, far beyond the predicted threshold.** I predicted ≥25% of
settlements at the cap; the measurement is **64% / 60% / 100%**. Falsifier (a)
(<5%) does not fire.

**The time series is worse than the fraction.** On `central` **the last six
consecutive settlements are all pinned at −75**, and on `south` the last three.
From hour 6 onward the funding rate is a constant, and the controller has stopped
responding to the premium entirely. This is not occasional clipping; it is a
mechanism that **latches at its limit and never returns**.

**How far past the limit.** At hour 11 `central`'s perp mark is **4 769 999 250**
against an ABC/USD mark of **4 911 390 000** — a perp basis of **−2.88%**, which
is **3.8× the ±0.75% cap**. Funding has less than a third of the authority it
would need to close the gap it is supposed to close. *(Caveat: the perp mark is
read at h=11 and the spot mark at the h=12 terminal snapshot; spot drifts on the
order of 0.2%/h, so roughly 2.7% of the gap survives the timing mismatch.)*

**Claim 2 supported: RT-042's boundary explanation is confirmed.** `north`
settles **exactly once, at h = 8.0**, precisely as predicted from the strict
interval-multiple rule. `central` settles 11 times at h=1…11 and `south` 5 times
at h=2,4,6,8,10 — every count and every timestamp as predicted. Falsifiers (b)
and (c) do not fire, and the count arithmetic that failed in E-058 is now correct
and verified against timestamps rather than inferred.

**What this means economically.** `Damping: 100` is a multiplier of 1.0 — no
damping — so the rate is the raw premium, and the only shaping is a hard clamp.
The result is a perpetual whose tether **detaches** once the basis exceeds 0.75%
and never reattaches. Every basis and carry strategy in the population
(`carry_arb`, `funding_carry_arb`, `dated_carry_arb`) trades a signal that is a
constant for half the run.

**This is the third instrument in this campaign with no working anchor**, each
found by a different route: the cross book quotes around itself ([[RT-031]]),
`north`'s perp never funds at all ([[RT-042]]), and now every venue's perp
detaches from its index once the basis passes 0.75% ([[RT-043]]). Meanwhile
`ABC/USD` is held rigid by a configured peg ([[RT-032]]). The campaign's price
system is one pegged book and a set of instruments floating away from it.

**Scope.** One seed, one configuration, 12 h. The saturation fraction and the
basis magnitude are single-seed; the *direction* (persistent discount, latched
cap) held across all three venues within this run, which are not independent
samples but do share no order flow.

Recorded as RT-043.


**H-055 (PREREGISTERED) — the perp mark is pinned at its clamp boundary, so the
reported basis is the band rather than the market, and positions are marked at a
price the book has left.**

**Mechanism, from inspection.** `exchange.go:1527` auto-installs
`NewClampedEMAMarkPrice(symbol, index, window, band)` for every margin instrument
that has an index, and `exchange.go:1514` defaults `band = 600` bps. The config
overrides nothing, so the perp mark is

    mark = index + clamp(EMA(perp_mid − index), ±index × 600/2/10000)

a **hard ±3% clamp** around index. `price/calculators.go:227` also clamps
`c.emaBasis` **in place**, so the clamp is applied to the filter's *state*, not
only to its output.

**Why this is the surprise in [[RT-043]].** I measured the perp basis at
**−2.88%**, which is **96% of the 3% half-band**. I reported it as a market
outcome. If the clamp is binding, it is not a market outcome at all — it is the
band, and the number I published describes a configuration constant.

**Two nested saturations.** The mark clamps at 3% from index; funding then clamps
at 0.75% of that already-clamped premium ([[RT-043]]). The outer limiter hides
how far the book has actually gone, and the inner one removes what little signal
survives.

**The risk consequence, which matters more than the economics.** Margin and
liquidation consume the mark. If the book's true mid is well beyond ±3% of index,
positions are marked at a price **better than the book** — a long is carried at a
value it could not realise, and liquidation does not fire when it should. That is
a solvency question, not a pricing preference.

**Claims.**
1. At late timestamps the clamp **binds**: `|EMA(mid − index)| / index` reaches
   the 3% half-band rather than sitting strictly inside it.
2. The perp book's **raw mid** diverges further from index than the clamp allows,
   i.e. `|mid − index| / index > 3%` while the clamp is binding.

**Falsifiers.**
(a) `|mid − index| / index` never reaches 3% → the clamp never binds, claim 1
falsified, and RT-043's −2.88% is a market outcome after all — which would be the
better news;
(b) raw mid stays inside the band while the mark sits at its edge → the mark is
*stale* (EMA lag) rather than clamped, a different mechanism and a different
finding;
(c) the divergence is transient (binding for <10% of samples) → the clamp is
doing its job as an outlier guard and no claim is made.

**Instrument.** New tool `research/tools/perpbasis`: from `BookSnapshot`
evidence, compute per venue and per timestamp the `ABC-PERP` mid, the index as
the **median of the three venues' `ABC/USD` mids** (matching
`spotIndexProvider`'s consensus mode), and the ratio. Reports the distribution of
`(mid − index)/index` and the fraction of samples beyond the ±3% band. This is
independent of the funding evidence used in RT-043.

**Discriminating experiment E-060**, preregistered before the run: seed 607, 8 h,
`-log-mode full`. Status: **SUPPORTED WITHIN TESTED SCOPE** on both claims; no
falsifier fired, magnitude ~6x predicted.


**E-060 — H-055 SUPPORTED on both claims, at roughly six times the predicted
magnitude. The perp mark sits exactly on its clamp while the book is 17–29%
away, so positions are marked far above what they could realise.**
Base: `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, `-log-mode full`.
Reproduce: `go run research/tools/perpbasis/main.go -dir <logdir>`.

`(perp mid − consensus index) / index`, from `BookSnapshot` evidence — an
**independent source** from the funding evidence [[RT-043]] used:

| venue | samples | mean | worst | beyond ±3% clamp | final-quarter mean |
|---|---:|---:|---:|---:|---:|
| central | 28 539 | −4.902% | **−28.850%** | 9 264 (**32.5%**) | **−17.193%** |
| north | 28 483 | −4.825% | −28.756% | 9 154 (32.1%) | −17.049% |
| south | 28 549 | −4.921% | −28.898% | 9 280 (32.5%) | −17.230% |

All three venues agree to a tenth of a percent, so this is systematic rather than
one venue's accident.

**Claim 1 — the clamp binds, and exactly.** 32.5% of samples lie beyond the band,
against falsifier (c)'s "transient, <10%". Better than a fraction, the mark lands
**on the boundary to the unit**: at h=8 the settled mark is **4 781 619 850**, and
the spot mid at that instant is **4 929 505 000**, whose 97% is
**4 781 619 850** — identical. The mark is not near the clamp, it *is* the clamp.

**Claim 2 — the book is far outside it.** The terminal snapshot has the perp at
bid **3 507 080 000** / ask **3 507 380 000** against a spot mid of
**4 929 505 000**: a basis of **−28.85%**, while the mark reports −3%.

**The consequence is solvency, not pricing.** Margin and liquidation consume the
mark. At the terminal state a long is marked at **4 781 619 850** while the best
bid is **3 507 080 000** — the position is valued **26.7% above** what it could
realise. For the final quarter of the run the gap averages ~14 points. **A
liquidation engine reading this mark does not fire when it should**, and every
margin figure in the campaign's perp accounting is optimistic by that amount.

**This corrects [[RT-043]]'s magnitude.** I reported the perp basis as −2.88% and
called it 3.8× the funding cap. That was the **clamped mark's** basis — a
configuration constant, exactly as suspected. The **book's** basis reaches
−28.85%, which is **38× the 0.75% funding cap**, not 3.8×. RT-043's mechanism is
unchanged and its saturation finding stands; its number described the limiter
rather than the market, which is precisely the error H-055 was written to catch.

**Two nested limiters, now both measured.** The mark clamp holds the reported
price 3% from index while the book goes to 29%; the funding cap then acts on that
already-clamped premium and latches at 0.75%. Neither limiter reports that it is
saturated. The only way to see it is to read the raw book, which is what this
tool does.

**Instrument note.** The first version of `perpbasis` returned "no timestamps with
both books two-sided" for every venue. The cause was a schema assumption:
per-book spot files write the levels at the top of the payload and identify the
book **by file path**, while the shared `derivatives.jsonl` nests them under a
symbol. A reader that assumes one shape silently finds nothing and reports a
clean empty result. This is the same trap recorded earlier for `Trade` payloads;
the tool now handles both shapes explicitly.

**Scope.** One seed, one configuration, 8 h. Three venues agree, but they share
the same index and the same population, so they are not independent replicates.

Recorded as RT-044.


**H-056 (PREREGISTERED) — [[RT-044]]'s asserted consequence, tested rather than
asserted: accounts are solvent at the clamped mark and insolvent at the book.**

RT-044 measured the perp mark pinned at −3% while the book sits 17–29% lower, and
**asserted** that "a liquidation engine reading this mark does not fire when it
should". That is an inference, not a measurement, and it is the kind of claim
this campaign has repeatedly caught itself publishing unverified. This tests it.

**Method.** For every terminal account, recompute the perp position's unrealised
PnL at the **terminal book midpoint** instead of the mark:

    equity_at_book = equity − unrealized_at_mark + (book_mid − entry) × size / precision

then compare the sign of `equity` against `equity_at_book`. An account that is
non-negative at the mark and negative at the book is **hidden insolvency**: the
risk engine believes it is solvent, and the order book says it cannot be closed
out at that value.

**Claims.**
1. At least one account is non-negative at the mark and **negative** at the book.
2. Liquidations attributable to `ABC-PERP` are few relative to a 29% adverse
   move — the engine does not react to a dislocation of that size.

**Falsifiers, and claim 1 is genuinely at risk.**
(a) **zero accounts flip sign** → no hidden insolvency, H-056 falsified, and
RT-044's consequence must be **retracted as overstated**. This is a live
possibility: perp makers are endowed with 100 M USD of perp collateral, so
positions may be small enough relative to margin that a 29% gap threatens no
one. If so the finding is that the mark gap is real but economically inert here,
which is worth publishing as a limit on RT-044.
(b) many `ABC-PERP` liquidations already fired → some path uses the book price
rather than the mark, contradicting RT-044's mechanism.
(c) accounts hold no material perp position at all → the test is **NOT
EXERCISED** and neither claim is scored.

**Instrument.** New Go tool `research/tools/marksolvency` (the repo's rule is that
data processing lives in Go). Inputs: `greeks.json` and a book midpoint per
venue; output: per-account equity at mark vs at book, the count that flip sign,
and the aggregate shortfall.

**Discriminating experiment E-061**, preregistered before the run: seed 607, 8 h,
`-log-mode full`; take the terminal `ABC-PERP` midpoint per venue from
`BookSnapshot` evidence, recompute, and report.
Status: **FALSIFIED WITHIN TESTED SCOPE** — zero accounts flip sign; RT-044's
liquidation consequence is retracted.


**E-061 — H-056 FALSIFIED. No account is hidden-insolvent, so [[RT-044]]'s
liquidation claim is RETRACTED. What replaces it is larger: the clamp moves
±50 M of reported value between classes and inverts two of [[RT-033]]'s
rankings.**
Base: `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, `-log-mode full`.
Reproduce: `go run research/tools/marksolvency/main.go -file <logdir>/greeks.json -book-mids central=3507230000,north=3511885000,south=3504895000`.

**Falsifier (a) fires. Claim 1 is false.**

    solvent at mark, insolvent at book: 0 accounts

and the run logs **0 liquidation events** in total. `ABC-PERP` terminal exposure
is 51 accounts, 4 363.03 long against 4 363.03 short.

**RT-044's asserted consequence is retracted.** I wrote that "a liquidation engine
reading this mark does not fire when it should" and that "every margin figure is
optimistic". The first half is **not supported**: nobody is close enough to
insolvency for the mark gap to matter, because the perp participants are heavily
over-collateralised — `carry_arb` carries 100 M USD of perp collateral against
3 000 contracts whose book loss is ~38 M. The engine is not failing to fire; **there
is nothing to fire on.** The measured facts of RT-044 (mark pinned exactly on the
clamp, book 17–29% away) are unaffected; the inference I hung on them was wrong,
and this is the fourth time in this campaign that an asserted consequence has
failed its own test.

**What the test found instead — the mark clamp redistributes reported value.**
Revaluing every perp position at the book midpoint:

| class | net contracts | reported above book |
|---|---:|---:|
| carry_arb | +3 000.00 | **+38 206 022** |
| fixed_distance_maker | +591.47 | +7 532 466 |
| imbalance_maker | +208.62 | +2 657 346 |
| perp_maker | +124.67 | +1 602 173 |
| noise_flow | −249.12 | −3 179 918 |
| spot_maker | −3 675.64 | **−46 818 089** |

The column sums to zero to the unit — it is a pure transfer, which is the
internal check that the revaluation is arithmetically sound.

**This inverts two of RT-033's published rankings.** Those figures were computed
at marks. At book midpoints:

| class | RT-033 (at mark) | at book | rank change |
|---|---:|---:|---|
| spot_maker | −6 817 441 | **+40 000 648** | 2nd-largest donor → **2nd-largest winner** |
| carry_arb | −2 423 266 | **−40 629 288** | 4th donor → **2nd-largest donor** |

A 16× change for `carry_arb` and a sign flip for `spot_maker`. The concentration
result is untouched — `triangle_arb` holds no perp position and its +174 M is
unchanged — but **any statement about the relative performance of
perp-holding classes depends on which valuation is used**, and the two answers
disagree by tens of millions.

**This is an owner decision and I am not making it.** The mark is the exchange's
official valuation and the one margin actually consumes; the book midpoint is
closer to what could be realised, though liquidating 3 675 contracts would itself
move a book that thin. Neither is unambiguously correct. What is not ambiguous:
the two diverge by ~50 M **because the clamp is binding**, and a ranking that
does not say which basis it used is under-specified.

**Falsifier (c) does not apply** — 51 accounts hold material positions, so the
test was genuinely exercised rather than vacuous.

**Scope.** One seed, one configuration, terminal snapshot only. The revaluation
uses a single terminal midpoint per venue and takes no account of the depth that
would be consumed closing these positions, so it is an upper bound on realisable
value, not an estimate of it.

Recorded as RT-045.


**H-057 (PREREGISTERED) — the perp subsystem is saturated at every layer: mark at
its clamp, funding at its cap, and the carry arbitrageur at its position limit.**

**Arithmetic that motivates it.** `carry_max_position: 50000000000` is **500.00
contracts**, and `carry_arbitrageur_count: 2` on three venues gives six
participants. [[RT-045]] measured the class net at **exactly +3 000.00**, which is
6 × 500.00. Every carry arbitrageur appears to be **pinned at its maximum long**.

**Why that completes a pattern rather than adding an isolated fact.** Three
limiters govern this subsystem and all three appear to be against their stops:

| layer | limit | state |
|---|---|---|
| mark | ±300 bps clamp | binding exactly ([[RT-044]]) |
| funding | ±75 bps cap | latched, last 6 settlements ([[RT-043]]) |
| carry arb | ±500 contracts | apparently at maximum |

`carry_entry_bps: 2` against a measured basis of **−2 885 bps** means the entry
condition is satisfied by a factor of ~1 440 and can never reverse, so the
strategy is a step function that fired once. **Nothing in the perp subsystem is
responding to anything.**

**Claim 1 is a real check, not a formality.** RT-045 reported the class *sum*. A
sum of +3 000.00 is also consistent with, say, +600/+400 splits. This tests each
participant individually.

**Claims.**
1. **Every** `carry_arb` participant's terminal position is exactly
   **+500.00** contracts, not merely the sum.
2. They reach the cap early and stay: **≥80%** of the run is spent at the cap.

**Falsifiers.**
(a) any participant is materially below the cap → claim 1 falsified, and the sum
was hiding dispersion;
(b) time at cap **<50%** → the arbitrageurs are actively trading around the
limit rather than latched, and the "saturated at every layer" framing is wrong;
(c) positions oscillate across zero → the strategy is working as designed and the
terminal snapshot was unrepresentative, which would make RT-045's exposure table
a poor summary.

**Instrument.** New Go tool `research/tools/positionpath`: reconstructs each
participant's signed position over time from `OrderFill` evidence, reporting
time-to-cap, fraction of the run at the cap, and the terminal position per
participant. Independent of the account snapshots.

**Discriminating experiment E-062**, preregistered before the run: seed 607, 8 h,
`-log-mode full`, symbol `ABC-PERP`, cap 500.00.
Status: **MIXED** — claim 1 supported exactly, claim 2 falsified (31-44% at cap,
not the predicted >=80%).


**E-062 — H-057 MIXED. Claim 1 supported exactly; claim 2 FALSIFIED, and with it
my "saturated at every layer" framing.**
Base: `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, `-log-mode full`.
Reproduce: `go run research/tools/positionpath/main.go -dir <logdir> -symbol ABC-PERP -role-prefix carry_arb -limit 500`.

| venue | role | fills | terminal | first at limit | time at limit |
|---|---|---:|---:|---:|---:|
| central | carry_arb_1 | 1 529 | **500.00** | 4.27 h | 40.8% |
| north | carry_arb_1 | 1 804 | **500.00** | 5.38 h | 32.7% |
| south | carry_arb_1 | 1 399 | **500.00** | 3.82 h | 43.6% |
| central | carry_arb_2 | 1 421 | **500.00** | 4.95 h | 38.1% |
| north | carry_arb_2 | 1 673 | **500.00** | 5.52 h | 31.0% |
| south | carry_arb_2 | 1 311 | **500.00** | 4.61 h | 42.4% |

**Claim 1 SUPPORTED, per participant.** All six end at **exactly +500.00**, the
configured `carry_max_position`. RT-045's class sum of +3 000.00 was not hiding
dispersion, and this confirms it from the **fill stream** — an independent source
from the account snapshots RT-045 used.

**Claim 2 FALSIFIED. Falsifier (b) fires on all six.** I predicted ≥80% of the run
at the cap; the measurement is **31.0%–43.6%**, every participant below the 50%
threshold. They first reach the limit only at **3.82–5.52 h** into an 8 h run, and
each trades 1 300–1 800 times.

**So the unifying story I proposed is wrong, and the correction is more
interesting than the claim.** I framed the perp subsystem as "saturated at every
layer: mark at its clamp, funding at its cap, arb at its limit". The first two are
measured and stand ([[RT-043]], [[RT-044]]). The third does not: **the carry
arbitrageur is not latched — it trades actively through the first half of the run
and pins only in the second**, as the basis blows out past anything it can absorb.
The saturation is **progressive, not initial**, and the arb is the layer that
keeps responding longest.

**That also qualifies [[RT-045]].** Its exposure table is a terminal snapshot
showing every carry arb at its cap, which reads as a permanent state. The
population spent only about a third of the run there. The snapshot is accurate and
unrepresentative at the same time. *(Falsifier (c) asked specifically about
oscillation across zero; this experiment measures time-at-limit and first-hit, not
sign changes, so whether the paths cross zero is **NOT EXERCISED**.)*

**POST-HOC, not preregistered.** Time-at-limit orders consistently by venue —
south earliest and longest (3.82/4.61 h, 43.6%/42.4%), north latest and shortest
(5.38/5.52 h, 32.7%/31.0%), central between — with both participants agreeing
within each venue. Six points across three venues is not enough to claim a venue
effect, and it is confounded with the funding-interval heterogeneity of
[[RT-042]]. Noted, not claimed.

**Instrument note — the third occurrence of one trap, and the first that a
cross-check caught.** The first version of `positionpath` reported **terminal
0.00 for all six participants while counting 1 529 fills each**. Derivative
`OrderFill` records nest the fill fields under `payload.payload` and keep only the
symbol at the outer level, so the tool matched the symbol, counted the record, and
read `qty` as zero. The output was internally consistent and entirely plausible —
"the carry arbs end flat" is a perfectly reasonable finding — and **the only
reason it was caught is that RT-045 had already measured +3 000.00 from a
different source and the two disagreed.** The same nesting trap has now appeared
in `Trade` payloads, `BookSnapshot` payloads and `OrderFill` payloads. `flowattrib`
is unaffected: it reads the outer symbol, finds no `/`, and skips derivatives,
which is why its spot figures never depended on the nested fields.

The lesson is not "handle the schema" — it is that a tool returning a plausible
wrong answer is invisible without an independent measurement to contradict it.
The two-source discipline that RT-040 established is what made this catchable.

Recorded as RT-046.


**H-058 (PREREGISTERED) — the spot makers' hedge stopped hedging: it is written
against an instrument that decoupled from the thing it protects, turning a hedge
into the population's largest directional bet.**

**Configuration.** `maker_hedge_symbol: "ABC-PERP"`, band 1.00 contract, 60 s
interval. The ABC/USD makers offset spot inventory by trading the perpetual
(`stoikov.go:1502`). That is why [[RT-045]] found `spot_maker` holding the
population's largest perp short, **−3 675.64 contracts** — a fact I flagged and
did not explain.

**Why the hedge should fail here, mechanically.** A hedge works only if the two
legs move together. This campaign has established that they do not:

- `ABC/USD` is held rigid by a configured peg — terminal drift **−1.4%**
  ([[RT-032]]);
- `ABC-PERP` has no working tether — mark pinned on its clamp, book **−17% to
  −29%** from index ([[RT-043]], [[RT-044]]).

So the maker is long a book that barely moves and short a book that collapsed.
The short does not offset a loss; it **manufactures a gain** far larger than the
exposure it was meant to neutralise. That is the mechanism behind RT-045's
finding that `spot_maker` is reported **−46.8 M below** what the book would pay —
the largest single revaluation in the population.

**Claims.**
1. The perp position **tracks spot inventory in sign and rough magnitude** — it
   is a hedge by construction, not an independent position.
2. The hedge relationship **breaks**: the perp leg's gain exceeds the spot leg's
   loss by a factor of **>5**, because the two prices decoupled.

**Falsifiers.**
(a) perp position is uncorrelated with, or the same sign as, spot inventory →
it is not a hedge and the mechanism is something else entirely;
(b) perp gain and spot loss are comparable (ratio **<2**) → the hedge is doing
its job, there is no decoupling story, and RT-045's revaluation has another
cause;
(c) `spot_maker` spot inventory is near zero → there was nothing to hedge and the
perp position is discretionary, which is a different finding.

**Instrument change with a built-in cross-check.** `positionpath` currently reads
`new_size`, the exchange's post-fill position, which spot fills do not carry (they
log `new_size: 0`). It will accumulate signed quantity instead, **and compare
against `new_size` wherever that field is non-zero**, reporting any disagreement.
That converts the accumulate-versus-authoritative question into a self-test rather
than an assumption — the accumulation path is exactly what produced the plausible
wrong answer in [[RT-046]].

**Discriminating experiment E-063**, preregistered before the run: seed 607, 8 h,
`-log-mode full`; per `spot_maker` participant report `ABC/USD` inventory and
`ABC-PERP` position, and decompose the class result into spot and perp legs.
Status: **SUPPORTED WITHIN TESTED SCOPE** on both claims; no falsifier fired.


**E-063 — H-058 SUPPORTED on both claims. The hedge is executed almost perfectly
and is economically catastrophic, because the hedge instrument decoupled from the
asset it protects.**
Base: `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, `-log-mode full`.
Reproduce: `go run research/tools/positionpath/main.go -dir <logdir> -symbol ABC-PERP -role-prefix spot_maker`
and the same with `-symbol ABC/USD`.

**Claim 1 SUPPORTED, and far more tightly than "rough magnitude".** Per
participant, spot inventory against perp position:

| participant | `ABC/USD` inventory | `ABC-PERP` position | residual |
|---|---:|---:|---:|
| central_2 | +501.71 | −498.81 | +2.90 |
| south_2 | +636.62 | −635.93 | +0.69 |
| north_2 | +476.61 | −476.87 | −0.26 |
| central_4 | +186.17 | −188.17 | −2.00 |
| … (12 in total) | | | |
| **class** | **+3 673.77** | **−3 675.65** | **−1.88** |

Every participant is delta-flat to within **2.90 contracts** on positions of
130–640, and the class nets to **−1.88 contracts out of 3 674** — inside the
configured 1.00-contract hedge band per participant. This is a hedge working
exactly as designed, and it independently reproduces [[RT-045]]'s −3 675.64 from
the fill stream rather than the account snapshots.

**Instrument self-test.** On the perp leg, **7 803 fills carried an
exchange-reported post-fill position and 0 disagreed with accumulation**. On the
spot leg no fill carries one, and the tool says so — "accumulation is unchecked"
— rather than implying a verification it did not perform.

**Claim 2 SUPPORTED. The hedge neutralises quantity and not value.**

| leg | value |
|---|---:|
| inventory exposure being hedged (3 673.77 ABC × −704.95 USD) | **−2.59 M** |
| spot leg result from fills | −0.88 M |
| perp leg at mark | −5.94 M |
| **perp leg at book** | **+40.88 M** |

The perp leg at book is **15.8×** the inventory exposure it was written to
neutralise, and **46×** the spot leg's own result. Predicted >5; falsifier (b)
required <2.

**The mechanism is the collision of two findings.** A hedge is only a hedge if
the legs move together. This campaign has measured that they do not: `ABC/USD` is
held rigid by a configured peg at **−1.4%** ([[RT-032]]) while `ABC-PERP`'s book
falls to **−29%** ([[RT-044]]). The maker is long a book that cannot move and
short a book with nothing holding it. Delta-neutral in contracts, wildly
directional in value.

**What this does to the fairness reading.** [[RT-045]] showed `spot_maker` moving
from the second-largest donor at marks (−6.8 M) to the second-largest winner at
book (+40.0 M). E-063 identifies the whole of that swing as **one configured
hedge into a dislocated instrument**. It is not market-making skill, and it is not
a strategy outcompeting anyone — the same shape as `triangle_arb`'s +174 M
([[RT-035]]), reached through a different mechanism. **Two of the population's
three largest results are now traced to instruments that lost their anchors.**

**Scope.** One seed, one configuration. The perp-leg-at-book figure is assembled
from three tools measuring the same run — `flowattrib` for the spot leg,
`classpnl` for the class total, `marksolvency` for the mark-to-book gap — so it
inherits each of their scope limits, and the book valuation ignores the depth
closing 3 675 contracts would consume.

Recorded as RT-047.


**H-059 (PREREGISTERED) — `noise_flow`'s −220 M is not a spread payment either.
Its loss rate is concentrated on the dislocated book.**

**The claim under test is my own.** After [[RT-047]] I wrote that `noise_flow`'s
−220 M is "uninformed flow paying spread — the only one of the three largest
results that looks like a market outcome". That sentence is an assertion, and
this campaign has now caught four of those failing their own tests. It gets one.

**Why it is suspect.** [[RT-035]] measured `noise_flow` paying
`abc_cdf_spot_maker` **188.6 M across 354 838.7 ABC, i.e. 531 USD/ABC or 1.08% of
notional**, and I read 1.08% as an ordinary market-making spread. But
`noise_target_qty_by_symbol` routes this flow onto `ABC/CDF`, the book that
[[RT-031]] showed running **69% from its bootstrap** and [[RT-041]] reproduced at
−69% to −83% across seeds. A participant transacting on a book that far from fair
is not paying a spread; it is trading at a wrong price. The two are
distinguishable by where the loss falls.

**Discriminator.** Decompose `noise_flow`'s result **per book**, and normalise by
the notional it traded on each. Spread payment is a property of the venue's
quoting and should cost a similar fraction of notional everywhere. Dislocation is
a property of one book.

**Prediction.** Loss per unit of notional on `ABC/CDF` is **≥3×** the rate on
`ABC/USD`.

**Falsifiers.**
(a) the two rates are within **2×** → the loss is roughly uniform spread payment,
H-059 falsified, and my "market outcome" reading stands as written;
(b) `ABC/USD` shows the higher rate → the dislocation story is backwards and the
loss is driven by the pegged book instead, which would be a different and more
surprising finding;
(c) `noise_flow` trades essentially one book → the comparison is **NOT
EXERCISED** and no rate claim can be made.

**Note on what this cannot settle.** Even a large `ABC/CDF` rate does not by
itself prove causation from the dislocation — the cross book also has a different
tick, different depth and a different maker. The result bounds how much of the
−220 M can be read as ordinary spread; it does not attribute the remainder to any
single cause.

**Discriminating experiment E-064**, preregistered before the run: seed 607, 8 h,
`-log-mode full`; `flowattrib -class noise_flow` for per-book contribution and
per-counterparty base traded, then rate per book.
Status: **SUPPORTED WITHIN TESTED SCOPE** at 21.4x, seven times the predicted
threshold; no falsifier fired.


**E-064 — H-059 SUPPORTED at 21×, seven times the predicted threshold. My "market
outcome" reading of `noise_flow` was wrong, and the cross book now accounts for
both ends of the leaderboard.**
Base: `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, `-log-mode full`.
Reproduce: `go run research/tools/flowattrib/main.go -dir <logdir> -class noise_flow`.

Loss normalised by **fair-value notional** (base traded × the asset's terminal
USD mark), the same basis on every book:

| book | loss | base traded | notional | rate | share of loss |
|---|---:|---:|---:|---:|---:|
| ABC/USD | −947 040 | 35 464.9 | 1 748 244 019 | **5.4 bps** | 0.4% |
| CDF/USD | −2 571 725 | 794 346.1 | 2 558 985 961 | **10.0 bps** | 1.2% |
| **ABC/CDF** | **−215 945 892** | 377 269.2 | 18 597 504 077 | **116.1 bps** | **98.4%** |
| total | −219 464 657 | | | | |

**Ratio 21.4× against `ABC/USD` and 11.6× against `CDF/USD`**, versus a
preregistered threshold of 3× and a falsifier band of 2×. Falsifier (a) does not
fire; (b) does not fire, the pegged book is the *cheapest* to trade; (c) does not
fire, all three books are traded.

**The claim I was testing was my own, and it is false.** I wrote that
`noise_flow`'s −220 M is "uninformed flow paying spread — the only one of the
three largest results that looks like a market outcome". **98.4% of that loss
occurs on one book**, at a rate **21× what the same participants pay on the
pegged book**. Uninformed flow on `ABC/USD` pays 5.4 bps, which *is* an ordinary
spread. The same actors on `ABC/CDF` pay 116 bps. Whatever that is, it is not the
venue's quoting cost.

**RT-035's 1.08% is re-read.** That figure was the rate `noise_flow` pays
`abc_cdf_spot_maker` computed on the cross book's own transaction prices, and I
called it "an ordinary market-making result". On a fair-value basis it is 116 bps
against 5.4 bps for the identical strategy one book over. The number was right;
calling it ordinary required a comparison I had not made.

**The leaderboard now has one cause.** All three of the population's largest
results sit on the cross book or on an instrument that lost its anchor:

| result | magnitude | attribution |
|---|---:|---|
| `triangle_arb` | +174 M | **98.1%** on `ABC/CDF` ([[RT-034]]) |
| `noise_flow` | −220 M | **98.4%** on `ABC/CDF` (here) |
| `spot_maker` | +40 M at book | hedge into the unanchored perp ([[RT-047]]) |

**The winner and the loser of this campaign are the same book.** `ABC/CDF`
produces the population's largest gain and its largest loss, and the middle link
between them — `abc_cdf_spot_maker` — nets +6.9 M while passing 193 M from one to
the other.

**What this does not establish**, as preregistered: a high rate on `ABC/CDF` does
not by itself prove the dislocation causes it. That book also has a different
tick, different depth and a different maker. The measurement bounds how much of
the −220 M can be read as ordinary spread — **at the `ABC/USD` rate of 5.4 bps,
the cross-book flow would have cost 10.0 M rather than 215.9 M** — and leaves the
remaining 205.9 M attributed to something specific to that book, without
apportioning it among the candidates.

**Scope.** One seed, one configuration. The notional basis uses terminal marks
for the whole run, so the rates are averages against an end-of-run valuation, not
trade-time rates.

Recorded as RT-048.


**H-060 (PREREGISTERED) — the noise traders' 116 bps on `ABC/CDF` is not a wide
spread; it is a directional loss from accumulating inventory in a collapsing
book.**

[[RT-048]] deliberately left 205.9 M unapportioned among "tick, depth, maker and
dislocation". This decomposes it into the only two components a taker's result
can have.

**Decomposition.** A taker's loss is
`spread paid` + `inventory revaluation on what it accumulates`. The first scales
with the number and size of trades and the quoted half-spread; the second scales
with the **net position it ends up holding** times how far the price moved. They
are separately measurable and they predict opposite things about the book's
quoting.

**Why the spread reading is doubtful.** 116 bps of half-spread on a book quoted
by a Stoikov maker with `maker_min_half_spread_ticks: 1` and a cross tick of
`mvBasePrecision/1000` — 0.006% of a 1.67e9 price — would be three orders of
magnitude above the tick floor. Possible, but it would make the cross maker's
quoting, not the dislocation, the story.

**Claims.**
1. `ABC/CDF`'s median quoted half-spread is **< 30 bps** — too small to account
   for a 116 bps loss rate.
2. `noise_flow` ends **net long** `ABC/CDF`, and the directional component
   (terminal position × price move over the run) explains **≥50%** of the
   215.9 M.

**Falsifiers.**
(a) `ABC/CDF` half-spread **≥100 bps** → the loss *is* spread, the book is simply
quoted very wide, and no dislocation-specific explanation is needed. This would
relocate the finding to the maker's quoting policy;
(b) `noise_flow`'s terminal `ABC/CDF` position ≈ 0 → there is no directional
component and the loss is transaction-cost-like after all;
(c) the directional component is **<25%** of the loss → neither component
dominates, the decomposition does not explain the number, and the result is
reported **INCONCLUSIVE** rather than split by assumption.

**Note on what "directional loss" would mean.** If claim 2 holds, the noise
traders lose because they are long an asset that fell — which is an ordinary
market outcome *given the price path*. It would place the anomaly entirely in why
the price fell 69% ([[RT-031]]: the book quotes around itself), and **not** in the
transaction mechanics. That is a materially different conclusion from RT-048's
framing and would narrow rather than widen the defect.

**Instrument.** New Go tool `research/tools/bookspread`: median and mean relative
half-spread per book from `BookSnapshot` evidence, handling both the per-book and
nested payload shapes that have now caught three tools.

**Discriminating experiment E-065**, preregistered before the run: seed 607, 8 h,
`-log-mode full`; `bookspread` for quoted spreads, `positionpath -role-prefix
noise_flow` per book for terminal inventory.
Status: **MIXED / INCONCLUSIVE** — claim 1 supported (spread is 1.80 bp against a
116.1 bps loss), claim 2 falsified with a sign reversal; the decomposition is
reported inconclusive as falsifier (c) specified.


**E-065 — H-060 MIXED, and the decomposition is reported INCONCLUSIVE as
preregistered. Claim 1 supported decisively; claim 2 falsified with a sign
reversal that rules the inventory story out affirmatively.**
Base: `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, `-log-mode full`.
Reproduce: `go run research/tools/bookspread/main.go -dir <logdir>` and
`go run research/tools/positionpath/main.go -dir <logdir> -symbol ABC/CDF -role-prefix noise_flow`.

**Claim 1 SUPPORTED. Quoted spread cannot explain the loss.**

| book | samples | median half-spread | mean | p90 | max | `noise_flow` loss rate |
|---|---:|---:|---:|---:|---:|---:|
| ABC/USD | 86 410 | **0.35 bp** | 0.34 | 0.39 | 2.56 | 5.4 bps |
| ABC-PERP | 85 595 | 0.41 bp | 0.41 | 0.42 | 3.30 | — |
| **ABC/CDF** | 77 814 | **1.80 bp** | 2.10 | 3.88 | 8.78 | **116.1 bps** |
| CDF/USD | 83 573 | **4.99 bp** | 4.48 | 5.00 | 6.67 | 10.0 bps |

The cross book's loss rate is **64× its own quoted half-spread**, and the quoted
spread accounts for **at most 1.8%** of it. Predicted <30 bps; measured 1.80 bp.
Falsifier (a), which required ≥100 bps, misses by nearly two orders of magnitude.

**Spread and loss rate are not even ordered together.** `CDF/USD` has the
**widest** spread in the population (4.99 bp) and the **second-lowest** loss rate
(10.0 bps); `ABC/CDF` has a spread under half that and a loss rate 11.6× higher.
Whatever drives the cross book's cost, it is not what the book charges to cross
it.

**Claim 2 FALSIFIED, and the sign is the informative part.** I predicted
`noise_flow` ends **net long** `ABC/CDF` so that a falling book would explain the
loss as inventory revaluation. Measured: **net −13 224.08 contracts, short**, with
**14 of 18 participants short**. A short position in a book that fell 69%
([[RT-031]]) **gains**. The inventory-revaluation story does not merely fail to
explain the loss — it points the wrong way, so it is ruled out rather than left
open.

**Status: INCONCLUSIVE, exactly as falsifier (c) specified.** Both candidate
components fail: the quoted spread bounds at ~2% of the loss rate, and the
directional component has the opposite sign. I am not splitting the remainder by
assumption. What the experiment did achieve is **elimination**: the two mechanisms
a taker's result can ordinarily be decomposed into are both excluded, which
narrows the search rather than answering it.

**What remains, and the next experiment that discriminates it.** With spread and
inventory drift both eliminated, the residual candidate is that the loss is
**realised at the moment of trade** — the noise traders exchanging ABC and CDF at
a rate far from the two assets' USD values, on a book [[RT-041]] measured at −69%
to −83% from its implied rate. The decisive measurement is a **volume-weighted
execution price**: total CDF received per ABC sold on `ABC/CDF`, converted at the
CDF mark, against ABC's own USD mark. If noise traders systematically sell ABC for
materially less USD-equivalent than ABC is worth, the loss is per-transaction
dislocation. **I am not claiming that yet** — it is the sixth mechanism sentence
this campaign would have published unmeasured, and the previous five were wrong.

**Scope.** One seed, one configuration. Half-spreads are top-of-book only and
ignore depth, so a taker consuming multiple levels pays more than the quoted
figure; that widens the spread component but not by the factor of 64 required.
Positions are accumulated from fills with no exchange-reported cross-check
available on spot, which the tool states.

Recorded as RT-049.


**H-061 (PREREGISTERED) — the cross book's cost is realised at execution: the
noise traders exchange ABC and CDF at rates far from the two assets' own USD
values, and the per-fill dislocation reproduces the −215.9 M.**

[[RT-049]] eliminated spread (1.80 bp against a 116.1 bps loss) and inventory
drift (net **short** 13 224 contracts in a falling book, wrong sign). It named
this residual and deliberately refused to claim it. This measures it.

**The arithmetic that makes it testable rather than a story.** At terminal marks
ABC is 49 295.05 USD and CDF is 3 221.50 USD, so the **fair cross rate is
15.302 CDF per ABC**. The cross book's terminal price is ≈5.14 CDF per ABC — 66%
below fair. A participant that is net short 13 224 ABC, having sold at rates
averaging some distance below fair, loses `net_flow × dislocation × CDF_USD`. At a
plausible average dislocation this lands in the right order of magnitude, and the
point of the experiment is that it either reproduces the measured loss or it does
not.

**Method — per fill, not per average.** For every `noise_flow` fill on `ABC/CDF`,
compare the execution rate against the fair cross rate **at that timestamp**,
built from the same `spotIndexProvider` consensus rule: the median across venues
of the `ABC/USD` mid divided by the median of the `CDF/USD` mid. Using terminal
marks for the whole run would confound the dislocation with the drift, which is
the trap [[RT-040]] recorded.

**Prediction.** Summed over fills, the implied loss
`Σ qty × (fair_rate − exec_rate) × CDF_USD`, signed by direction, reproduces the
measured **−215 945 892** within **±25%**.

**Falsifiers.**
(a) implied loss **<50%** of measured → execution dislocation does not explain the
loss and the search moves elsewhere; the residual would then have no candidate
left among the three ordinary ones;
(b) implied loss **>200%** of measured → the instrument over-explains, indicating
double counting or a sign error, and nothing is claimed until it reconciles;
(c) volume-weighted execution rate ≈ fair rate → there is no execution
dislocation and the mechanism is dead outright.

**Self-test built in.** The implied loss is computed from fills and fair rates;
the measured loss came from `flowattrib`'s independent contribution calculation.
Agreement between them is a genuine cross-check, not a restatement — the two share
no arithmetic beyond reading the same fill records.

**Instrument.** New Go tool `research/tools/crossexec`: two passes over the
evidence — one to build the per-timestamp consensus fair rate from `ABC/USD` and
`CDF/USD` snapshots, one to walk `ABC/CDF` fills for a role class and accumulate
signed dislocation. Handles both payload shapes.

**Discriminating experiment E-066**, preregistered before the run: seed 607, 8 h,
`-log-mode full`. Status: **SUPPORTED WITHIN TESTED SCOPE** — implied loss is
114.0% of measured, inside the ±25% band; no falsifier fired.


**E-066 — H-061 SUPPORTED. The loss is realised at execution, and the causal
chain from one config line to the population's largest loss is now closed
quantitatively.**
Base: `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, `-log-mode full`.
Reproduce: `go run research/tools/crossexec/main.go -dir <logdir> -class noise_flow`.

216 529 fills matched, **0 skipped** for a missing fair rate.

| | volume | VWAP (CDF/ABC) | fair at execution | gap |
|---|---:|---:|---:|---:|
| bought | 182 022.51 | 7.9059 | 16.2996 | **−51.50%** |
| sold | 195 246.58 | 8.0708 | 16.3087 | **−50.51%** |
| net flow | **−13 224.07** | | | |

**The noise traders transact ABC at roughly half its fair value in CDF, in both
directions, across 216 529 fills.** They buy cheap and sell cheap, and because
they sell 13 224 more than they buy, the net is a large loss.

**The preregistered cross-check passes.**

    implied loss from execution away from fair   −246 188 281 USD
    measured loss (flowattrib contribution)      −215 945 892 USD
    ratio                                              114.0%

Inside the ±25% band. Falsifier (a) needed <50%, (b) needed >200%, (c) needed
VWAP ≈ fair — none fire. The two figures come from different arithmetic on the
same evidence: one accumulates per-fill deviation from a contemporaneous
consensus rate, the other sums cash flows and values terminal inventory at
terminal marks. **The residual 14% is expected and is not slack** — it is the
taker fee (5 bps), the snapshot-to-fill timing gap, and precisely that difference
in how terminal inventory is valued.

**A second, unplanned cross-check landed exactly.** `crossexec` reports net base
flow **−13 224.07** from fill-by-fill accumulation; [[RT-049]]'s `positionpath`
independently reported **−13 224.08**. Two tools, separately written, agree to
0.01 contracts.

**The chain is now closed end to end**, each link measured rather than inferred:

| link | finding | evidence |
|---|---|---|
| the cross maker quotes around its own mid | [[RT-031]] | `ReferenceSymbol` = its own symbol |
| so the book leaves fair value | [[RT-041]] | −69% to −83%, three seeds |
| uninformed flow is routed onto it | config | `noise_target_qty_by_symbol` |
| and loses at 116 bps, 21× the pegged book | [[RT-048]] | 98.4% of a −219.5 M loss |
| not from spread — 1.80 bp | [[RT-049]] | loss is 64× the half-spread |
| not from inventory — net **short** in a falling book | [[RT-049]] | wrong sign |
| but at execution, ~51% below fair | **E-066** | 216 529 fills, 114% of measured |

**One line of configuration — a maker whose reference is its own book — produces
a −246 M transfer through 216 529 trades.** That is the campaign's largest loss,
and it is fully attributable.

**Scope.** One seed, one configuration. The fair rate is the venue's own
consensus rule applied to snapshot mids at or before each fill, so it inherits the
1 s snapshot cadence; a fill can be compared against a rate up to one second
stale. Depth is ignored — VWAP is computed from executed prices, which already
include whatever depth was consumed.

Recorded as RT-050.


**H-062 (PREREGISTERED) — the option books are one-sided for most of the run, so
option positions cannot be exited and the option "market" is not one.**

**New lineage.** The `ABC/CDF` chain is closed end to end ([[RT-050]]). The
option and dated-futures books have never been audited: [[RT-045]] showed
`option_dealer`, `option_flow`, `option_value_taker` and `vanna_volga_desk`
earning **nothing** in any spot book, so their entire results come from surfaces
no instrument in this campaign has examined.

**The observation that motivates it**, already sitting in E-065's output and not
chased at the time:

| book | samples | median half-spread | one-sided snapshots |
|---|---:|---:|---:|
| ABC-…-48000-C | 21 606 | 7.1% | 12 |
| ABC-…-48000-P | 9 076 | **75.4%** | **12 542** |
| ABC-…-49000-P | 7 134 | **97.4%** | **14 484** |
| ABC-…-51000-C | 10 239 | **96.8%** | **11 379** |
| ABC-…-52000-P | 21 606 | 6.5% | 12 |

**The obvious innocent explanation must be tested, not assumed.** A deep
out-of-the-money option is nearly worthless, so a one-tick absolute spread is a
huge *relative* spread and a missing bid is unremarkable. That is falsifier (c)
and it is a live possibility for most of these books.

**But one row resists it.** `49000-P` is one-sided in **14 484** snapshots while
terminal ABC is **49 295** — a strike within 0.6% of spot, i.e. **at the money**,
not deep OTM. An at-the-money option with no market two-thirds of the time is not
explained by worthlessness.

**Claims.**
1. A majority of option books are one-sided for **>50%** of their snapshots.
2. The missing side is predominantly the **bid** — nobody bids, so a holder
   cannot exit.
3. The one-sidedness is **not** confined to deep OTM strikes: at least one book
   within 2% of spot is one-sided for >50% of its snapshots.

**Falsifiers.**
(a) typical one-sidedness **<20%** → the books are fine and E-065's counts were
dominated by a few dead strikes;
(b) the missing side is predominantly the **ask** → the story is that options
cannot be bought rather than sold, a different and less serious finding;
(c) every book with >50% one-sidedness is **more than 5% out of the money** →
this is ordinary deep-OTM behaviour, no defect, and claim 3 fails. **This is the
falsifier I expect to be closest.**

**Instrument.** Extend `bookspread` to report, per book, the fraction of
snapshots that are two-sided, bid-only and ask-only, so the missing side is
identified rather than inferred from a count.

**Discriminating experiment E-067**, preregistered before the run: seed 607, 8 h,
`-log-mode full`; report per option book the two-sided fraction, which side is
missing, and the strike's distance from terminal spot.
Status: **MIXED** — claim 1 falsified (24%, not a majority), claims 2 and 3
supported, falsifier (c) does not fire.


**E-067 — H-062 MIXED. Claim 1 falsified as written; claims 2 and 3 supported,
and the surface splits perfectly on moneyness: every in-the-money option has a
two-sided market, every out-of-the-money option loses its bid.**
Base: `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, `-log-mode full`.
Reproduce: `go run research/tools/bookspread/main.go -dir <logdir>`.

50 option books, terminal spot **49 295.05**:

| group | n | two-sided (min) | two-sided (median) | half-spread (median) |
|---|---:|---:|---:|---:|
| **in the money** | 25 | **99.9%** | 99.9% | 10.0% |
| **out of the money** | 25 | **21.4%** | **58.3%** | **76.8%** |

**The separation is total: the worst ITM book (99.9% two-sided) is better than the
best OTM book (98.8%).** Not a tendency — a partition.

**Claim 1 FALSIFIED as written.** I predicted "a majority of option books are
one-sided for >50% of their snapshots". Only **12 of 50 (24%)** are. The
prediction was wrong; the structure it was groping at is sharper than the
threshold I chose.

**Claim 2 SUPPORTED, overwhelmingly.** The missing side is the **bid**, in **12 of
12** broken books. Across the whole surface, ask-only (no bid) reaches **16 962**
snapshots while bid-only (no ask) never exceeds **18**. **Holders of
out-of-the-money options cannot sell them** for 21%–79% of the run.

**Claim 3 SUPPORTED, and falsifier (c) — the one I expected to be closest — does
not fire.** Every one of the 12 worst books is within **5.5% of spot**; none is
deep out of the money. The most broken near-ATM case is `49000-P` at **−0.6% from
spot**, two-sided only **22.1%** of the time. The "deep OTM options are worthless,
so a missing bid is unremarkable" explanation is excluded by measurement rather
than by argument.

**The mechanism is NOT determined, and I checked before guessing.** My first
explanation was that OTM bids round down below one tick and are therefore not
placed. **That is wrong.** Sampling `49000-P` directly: bid **1 600 000**, ask
**31 400 000** — in USD, a **16 USD bid against a 314 USD ask** on the same
option, a bid at 5% of the ask rather than an absent one. The healthy `49000-C`
quotes 585 against 879. So OTM quotes are extremely skewed toward the ask, and
the bid vanishes only some of the time. **Why** is unmeasured, and after five
mechanism sentences in this campaign that turned out wrong, it stays unclaimed.

**Why it matters for the fairness question.** [[RT-045]] showed `option_dealer`,
`option_flow`, `option_value_taker` and `vanna_volga_desk` earning their entire
results in books no instrument here had examined. Half that surface has **no bid
at all** for much of the run, and where a bid exists the half-spread is 76.8%. Any
result attributed to option strategy skill is a result obtained in a market where
one side of half the instruments is frequently absent.

**Next experiment, highest information gain.** Identify who supplies each side of
an option book — from maker/taker roles on fills per book — and whether the
dealer's quoting is skewed by construction or the bid is being withdrawn by a
risk limit. That distinguishes a quoting-policy artifact from an inventory
constraint, and it is one pass over evidence already collected.

**Scope.** One seed, one configuration. Moneyness is computed against the
**terminal** spot; a book classified OTM at the end may have been ITM earlier, so
the partition is cleanest for strikes far from 49 295 and softest for `49000`.
That caveat cannot explain the result, since `49000-C` (ITM by 0.6%) is 99.9%
two-sided while `49000-P` (OTM by 0.6%) is 22.1%.

Recorded as RT-051.


**H-063 (PREREGISTERED) — the OTM options have no bid because nobody places one,
by quoting policy rather than by risk-limit withdrawal.**

[[RT-051]] measured the partition — every ITM book 99.9% two-sided, every OTM book
losing its bid — and explicitly refused to name a mechanism after the obvious
guess (tick rounding) was checked and refuted. This tests the two remaining
candidates against each other.

**Why fills cannot answer it and placements can.** Attributing quote provision
from `OrderFill` is circular: a book with no bid has no bid-side fills by
construction, so the absence would be its own evidence. `OrderAccepted`
(33 892 events in a 20-minute probe) records **placements** — client, symbol,
side, price — independent of whether anything traded.

**The two candidates make opposite time predictions.**

| candidate | signature |
|---|---|
| **quoting policy** — the dealer's model never bids for these contracts | bid placements ≈0 on OTM books **from the first minutes**, flat over the run |
| **risk-limit withdrawal** — the dealer bids until inventory or margin stops it | bid placements start healthy and **decline** as the run proceeds |

**Claims.**
1. On OTM books, bid placements are a small fraction of ask placements —
   **below 20%** of the ask count.
2. That ratio is **already low in the first quarter** of the run and does not
   fall by more than half between the first and last quarter, i.e. it is a policy,
   not a withdrawal.
3. `option_dealer` is the dominant ask provider on those books, so the asymmetry
   is attributable to the dealer rather than to incidental flow.

**Falsifiers.**
(a) the bid/ask placement ratio **declines by more than half** first quarter to
last → risk-limit withdrawal, claim 2 falsified and the finding changes character;
(b) bid placements are **comparable to ask placements** (ratio >50%) on OTM books
→ the missing bid is not a placement phenomenon at all; bids are being placed and
then cancelled or consumed, and a third mechanism is required;
(c) a class other than `option_dealer` dominates ask provision → the dealer is
not the quoting party and claim 3 fails.

**Instrument.** New Go tool `research/tools/quoteside`: tallies `OrderAccepted`
by book, role class and side, split into run quarters, with books grouped
in-the-money versus out-of-the-money against a supplied spot.

**Note on provenance.** The 20-minute run used to discover the event schema is
**not** the measurement; E-068 runs the full 8 h at seed 607 so the quarter-split
is meaningful.

**Discriminating experiment E-068**, preregistered before the measurement run:
seed 607, 8 h, `-log-mode full`.
Status: **MIXED** — claim 1 falsified at 22.5% against a <20% prediction,
claim 3 supported, risk-limit withdrawal excluded, but a symmetric third quarter
leaves the mechanism open.


**E-068 — H-063 MIXED. The dealer quotes in-the-money books perfectly
symmetrically and out-of-the-money books 4.45:1 to the ask. Risk-limit withdrawal
is excluded, but a third-quarter reversal neither candidate predicts leaves the
mechanism open.**
Base: `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, `-log-mode full`.
Reproduce: `go run research/tools/quoteside/main.go -dir <logdir>`.

`option_dealer` placements, from `OrderAccepted` — what was **placed**, not what
traded, so the measurement is not circular:

| group | bids | asks | bid/ask | by quarter (bids/asks) |
|---|---:|---:|---:|---|
| **in the money** | 336 292 | 336 292 | **100.0%** | 100% (97 767/97 767), 100% (103 384/103 384), 100% (51 189/51 189), 100% (83 952/83 952) |
| **out of the money** | 122 127 | 543 513 | **22.5%** | 21%, 23%, **100%**, 14% |

**On in-the-money books the dealer places exactly one bid for every ask — in
every quarter, to the unit.** 97 767/97 767, 103 384/103 384, 51 189/51 189,
83 952/83 952. It is a strictly two-sided quoting loop. On out-of-the-money books
the same dealer places **4.45 asks for every bid**.

**Claim 1 falsified, narrowly and it counts.** I predicted a bid/ask ratio below
20%; the measurement is **22.5%**. The direction is right and the threshold was
wrong, and a threshold missed by 2.5 points is still missed.

**Claim 3 supported.** `option_dealer` places **90.8%** of all OTM asks, so the
asymmetry belongs to the dealer, not to incidental flow.

**Falsifier (a) does not fire, so risk-limit withdrawal is excluded.** The ratio
runs 21% → 14% from first quarter to last, a factor of **0.66**, where the
falsifier required a fall below 0.5. The dealer does not start healthy and get
squeezed out; it is already at 21% in the first quarter.

**But the third quarter breaks both candidates, and it is not noise.** In Q3 the
dealer placed **25 092 bids against 25 093 asks — 100%** — on the very books it
otherwise skews 4:1. That is a 25 000-placement sample, not a small-denominator
artifact, and it sits between 23% and 14%. Within Q3, falsifier (b) would fire on
its own terms. **Neither a static quoting policy nor a monotone withdrawal
predicts a symmetric quarter in the middle of an asymmetric run.**

**A visible confound I will not resolve by assumption.** Total placements also
collapse in Q3 — OTM 50 185 against 216 535 in Q1, ITM 102 378 against 195 534 —
and moneyness here is classified against **terminal** spot, a caveat already
recorded in [[RT-051]]. Options expire and relist through the run (tenors 2 h and
6 h, five listing timestamps), so a book counted OTM at the end may have been at
or in the money during Q3. **That is a candidate explanation and it is untested.**

**Status: the mechanism remains open**, now with one candidate eliminated and a
specific new anomaly to explain. This is the seventh mechanism question in this
campaign and the second time in two checkpoints that the honest answer is "not
yet" — which is the point of measuring before writing.

**Next experiment.** Recompute moneyness **per listing epoch against
contemporaneous spot** rather than terminal spot, and re-split the quarters. If
Q3's symmetry disappears once books are classified by what they were at the time,
the anomaly was a classification artifact and the policy reading stands. If it
survives, the dealer changes behaviour mid-run and neither candidate is right.

**Scope.** One seed, one configuration. Placements count orders accepted, not
resting depth or time-weighted presence, so a class that places many short-lived
orders outweighs one that rests a single quote.

Recorded as RT-052.


**H-064 (PREREGISTERED) — the dealer's bid depends on moneyness *at the moment it
quotes*, and [[RT-052]]'s symmetric third quarter is a classification artifact of
using terminal spot.**

**Representation change.** RT-051 and RT-052 both bucketed *books* by their
moneyness at the **end** of the run. Options expire and relist across five listing
timestamps with 2 h and 6 h tenors, so a contract labelled out-of-the-money at the
end may have been in the money while it was being quoted. The fix is to stop
classifying books and start classifying **placements**: for every
`OrderAccepted`, compute the contract's moneyness against the **contemporaneous**
`ABC/USD` consensus mid, using the same median-across-venues rule the venue's own
index uses.

Define signed moneyness `m = (S − K)/S` for calls and `(K − S)/S` for puts, so
**positive is in the money** for both.

**Claims.**
1. The dealer's bid/ask placement ratio is a **monotone function of quote-time
   moneyness** — near 100% for in-the-money buckets, low for out-of-the-money
   ones, with the transition at the money.
2. [[RT-052]]'s Q3 anomaly **disappears**: within each moneyness bucket the ratio
   varies by less than **2×** across the four quarters.

**Falsifiers.**
(a) the Q3 excursion **persists within buckets** → the dealer genuinely changes
behaviour mid-run, the classification artifact explanation is wrong, and RT-052's
open question stays open with one more candidate eliminated;
(b) **no monotone relation** with quote-time moneyness → moneyness is not the
driver at all, and the framing of both RT-051 and RT-052 needs rebuilding rather
than refining;
(c) the ratio is low **even for deep in-the-money** placements → the asymmetry is
not about moneyness but about the dealer's inventory or direction, a different
mechanism.

**What makes this decisive rather than another correlation.** If claim 1 holds and
claim 2 holds, the whole picture reduces to one rule — *the dealer does not bid
for options it prices near zero* — and RT-052's anomaly was my own measurement
choice, not the simulator's behaviour. If claim 2 fails, the anomaly is real and
belongs to the dealer.

**Instrument.** Extend `quoteside` to bucket each placement by quote-time
moneyness, reusing the per-timestamp consensus machinery already written for
`crossexec`, and report bid/ask ratio per bucket per quarter with the underlying
counts.

**Discriminating experiment E-069**, preregistered before the run: seed 607, 8 h,
`-log-mode full`.
Status: **MIXED** — claim 1 falsified as stated but supported across the 1.31 M
placements that carry the volume; claim 2 falsified and falsifier (a) fires.


**E-069 — H-064 MIXED, and my proposed explanation is REFUTED. Falsifier (a)
fires: the third-quarter symmetry survives reclassification, so it is the
dealer's behaviour and not my measurement choice.**
Base: `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, `-log-mode full`.
Reproduce: `go run research/tools/quoteside/main.go -dir <logdir>`.

`option_dealer` placements bucketed by moneyness **at the instant of the quote**,
against the contemporaneous median `ABC/USD` mid:

| bucket | bids | asks | bid/ask | by quarter |
|---|---:|---:|---:|---|
| deep ITM (>+5%) | 7 778 | 7 778 | 100.0% | – – **100%** 100% |
| ITM (+1…+5%) | 262 427 | 262 427 | **100.0%** | 100% 100% 100% 100% |
| at the money (±1%) | 107 281 | 143 299 | **74.9%** | 78% 79% **100%** 59% |
| OTM (−1…−5%) | 77 049 | 462 417 | **16.7%** | 14% 17% **100%** 12% |
| deep OTM (<−5%) | 3 884 | 3 884 | 100.0% | – – **100%** – |

**Claim 2 FALSIFIED and falsifier (a) fires.** I predicted the Q3 excursion would
disappear once contracts were classified by what they were when quoted. It does
not: within the OTM bucket the ratio runs **14%, 17%, 100%, 12%** — a spread of
**8.3×**, where the falsifier required under 2×. **In Q3 every bucket is exactly
100%.** The symmetric quarter is real dealer behaviour. My explanation was wrong,
and [[RT-052]]'s open question stays open with the classification-artifact
candidate now eliminated.

**Claim 1 FALSIFIED as stated, supported where the volume is.** The relation is
not monotone across all five buckets: the two tails read 100%. But those tails
hold **23 324 placements, 1.7% of the total**, and appear only in the quarters
where Q3 dominates. Across the three buckets carrying **1 314 900 placements** the
relation is clean and monotone — **100.0% → 74.9% → 16.7%** as contracts move from
in the money through at the money to out of it. The threshold-shaped claim failed;
the shape it was reaching for is there.

**The in-the-money exactness is the sharpest single number.** 262 427 bids against
262 427 asks — **equal to the unit** across 8 simulated hours and every quarter.
The dealer runs a strictly paired two-sided loop for in-the-money contracts and
abandons it for out-of-the-money ones, and in Q3 abandons the abandonment.

**What Q3 now looks like, stated as an observation rather than a mechanism.**
Every bucket at exactly 100% is not a gradual shift — it is the signature of a
**different quoting path**, one that emits strictly paired quotes. The deep-ITM
and deep-OTM buckets exist almost only in Q3, and [[RT-052]] measured total
placements collapsing there (OTM 50 185 against 216 535 in Q1). Something changes
regime for a quarter. **I am not naming it.** Two candidates have now been
eliminated by measurement — risk-limit withdrawal (RT-052) and classification
artifact (here) — and the count of wrong mechanism sentences in this campaign is
the reason the third is not being guessed.

**Next experiment, cheap and discriminating.** Tally Q3 placements by **listing
epoch** — the timestamp embedded in each option symbol. Options relist across five
epochs with 2 h and 6 h tenors. If Q3's paired quoting is confined to
freshly-listed contracts, the regime is a listing-bootstrap path; if it is spread
across all live expiries, it is a change in the dealer itself. One pass over
evidence already collected.

**Scope.** One seed, one configuration. Placements count accepted orders, not
resting depth or time-weighted presence. The quote-time spot is the consensus mid
at or before each placement, inheriting the 1 s snapshot cadence.

Recorded as RT-053.


**H-065 (PREREGISTERED) — [[RT-053]]'s third-quarter symmetry is a
**time-to-expiry** effect, not a calendar one.**

**The clue that reframes it.** The timestamp embedded in an option symbol is its
**expiry**, not its listing. Decoded against the run start, the five expiries fall
at exactly **+2 h, +4 h, +6 h, +8 h and +12 h**. The anomalous third quarter spans
**+4 h to +6 h** — it is bounded by two expiries. A "quarter" is a calendar
artifact of my own bucketing; the dealer has no notion of it. Time to expiry is a
quantity the dealer actually sees.

**Representation change.** Stop bucketing by run quarter and bucket by **hours
remaining to expiry at the moment of the quote**, cross-cut with the quote-time
moneyness already established in RT-053. If the anomaly is really about tenor,
the calendar pattern should dissolve into a tenor pattern.

**Claims.**
1. The bid/ask placement ratio rises toward **~100% at short time to expiry**,
   across moneyness buckets — i.e. the dealer quotes both sides for contracts near
   expiry regardless of whether they are in the money.
2. Controlling for tenor, RT-053's moneyness rule **persists**: within a
   comparable tenor band, in-the-money placements still show a far higher bid/ask
   ratio than out-of-the-money ones.
3. Q3's placements are concentrated at **short tenor**, which is what made the
   calendar quarter look anomalous.

**Falsifiers.**
(a) the ratio is **flat across tenor** (varies less than 2× between the shortest
and longest bands) → tenor is not the driver, a **third** candidate is eliminated,
and RT-053's anomaly stays open with no remaining hypothesis;
(b) Q3 placements are **not** concentrated at short tenor → claim 3 fails and the
calendar coincidence is not the explanation;
(c) the moneyness rule **disappears** once tenor is controlled → RT-053's headline
was a tenor effect in disguise and must be reissued, which would be the more
serious outcome.

**Why this is worth one more experiment rather than moving on.** Two candidates
have already been eliminated by measurement (risk-limit withdrawal, classification
artifact). If tenor also fails, the honest position is that the dealer's quoting
has a regime change nobody has explained, and that is a finding in itself — a
bounded negative result rather than an open loop.

**Instrument.** Extend `quoteside` with a tenor axis: hours to expiry at quote
time, cross-cut with quote-time moneyness, reporting counts alongside every ratio.

**Discriminating experiment E-070**, preregistered before the run: seed 607, 8 h,
`-log-mode full`.
Status: **MIXED** — claim 1 falsified, claims 2 and 3 supported; tenor is
eliminated as the anomaly's explanation, the third candidate to fall.


**E-070 — H-065 MIXED. Tenor is eliminated as the explanation for
[[RT-053]]'s anomaly — the third candidate to fall — but the anomaly is now
described precisely: in Q3 the dealer quotes **only** short-dated contracts, and
quotes every one of them in strictly paired form.**
Base: `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, `-log-mode full`.
Reproduce: `go run research/tools/quoteside/main.go -dir <logdir>` and the same
with `-quarter 3`.

**Whole run**, bid/ask by quote-time moneyness × hours to expiry:

| | <0.5h | 0.5–1h | 1–2h | 2–4h | ≥4h |
|---|---|---|---|---|---|
| ITM (+1…+5%) | **100%** | **100%** | **100%** | **100%** | **100%** |
| at the money | 58% | 74% | 65% | 94% | 88% |
| OTM (−1…−5%) | 34% | 15% | **9%** | 47% | 17% |

**Claim 2 SUPPORTED — the moneyness rule is robust to tenor.** In-the-money
placements are **exactly paired in every tenor band**; out-of-the-money ones never
exceed 47%. RT-053's headline is not a tenor effect in disguise, so falsifier (c)
does not fire.

**Claim 1 FALSIFIED.** I predicted the ratio would rise toward 100% at short
tenor. For OTM it runs **34%, 15%, 9%, 47%, 17%** — non-monotone, and the shortest
band is not the highest. Tenor does matter (a 5.2× spread, so falsifier (a)'s
"flat" does not fire either) but not in the direction predicted.

**Claim 3 SUPPORTED: Q3 is entirely short-dated.** Its placements are 21.6%
under 0.5 h, 23.7% at 0.5–1 h, 54.6% at 1–2 h — **99.9% under two hours** — and
**zero** at ≥4 h, against **170 439** such placements in the run as a whole.

**But tenor does not explain the anomaly, and the Q3-only cut proves it.**
Restricting to the third quarter, **every cell is exactly 100%**:

| Q3 only | <0.5h | 0.5–1h | 1–2h |
|---|---|---|---|
| ITM | 100% (6 383/6 383) | 100% (7 218/7 218) | 100% (19 698/19 698) |
| at the money | 100% (3 088/3 088) | 100% (3 459/3 459) | 100% (8 973/8 973) |
| **OTM** | **100% (3 475/3 476)** | **100% (3 542/3 542)** | **100% (8 750/8 750)** |

Removing Q3 from the whole-run figures, the OTM band reads **27.7%, 10.4%, 3.9%,
47.3%, 17.4%** — so outside Q3 short tenor does *not* produce paired quoting, and
inside Q3 every tenor does. **Tenor and the anomaly are orthogonal.** Third
candidate eliminated.

**What Q3 actually is, stated as description rather than mechanism.** Two things
change together: the dealer **stops quoting every contract with more than about
two hours to expiry**, and it quotes those that remain in **strictly paired**
form. Not a shift in degree — 3 542/3 542, 8 750/8 750, 19 698/19 698 — with a
single unpaired ask in one cell (3 475/3 476) as the only blemish in 60 000
placements.

**Status: a bounded negative result, which is a finding rather than an open
loop.** Three candidate explanations have now been eliminated by measurement —
risk-limit withdrawal ([[RT-052]]), classification artifact ([[RT-053]]), and
tenor (here). The dealer has a regime in which it quotes a restricted, short-dated
subset of contracts symmetrically. **I do not know why, and after seven mechanism
questions in this campaign that is the honest place to stop this lineage** rather
than to keep generating candidates.

**What an owner can do with this without further audit work.** The measurement is
precise enough to act on: `option_dealer`'s quoting has two distinct modes, the
transition happens around the +4 h expiry boundary, and in one mode it abandons
all contracts beyond ~2 h tenor. Whether that is intended is a question about the
dealer's design that its author can answer far more cheaply than I can measure it.

**Scope.** One seed, one configuration. Placements count accepted orders, not
resting depth. Quarters are calendar bucketing of the observer's choosing; the
tenor axis is the dealer's own.

Recorded as RT-054.


**H-066 (PREREGISTERED) — the option dealer's missing bid is a spot-proportional
half-spread, and [[RT-054]]'s unexplained regime is inventory skew.**

**Reopening a lineage I closed, and why that is legitimate.** RT-054 stopped after
three candidate mechanisms were eliminated *by measurement*. This does not add a
fourth guess — it consults the **specification**, which I had never read for this
actor. RT-031 was found the same way. Reading the source is evidence of a
different kind, not speculation.

**The mechanism, from `simulations/derivsim/optionmm.go:351`:**

    theo := eprice.Black76Premium(...)
    half := mm.spotMid * mm.cfg.SpreadBps / 10000
    skew := mm.spotMid * mm.cfg.SkewPerLotBps / 10000 * q.inventory / mm.cfg.LotQty
    bid := alignDown(theo-half-skew, tick)
    ask := alignUp(theo+half-skew, tick)
    ...
    if bid > 0 {
        mm.SubmitOrder(sym, exchange.Buy, ...)   // bid: conditional
    }
    mm.SubmitOrder(sym, exchange.Sell, ...)      // ask: unconditional

**The ask is placed unconditionally; the bid only when it prices above zero.**
And `half` is **30 bps of the underlying spot** (`SpreadBps: 30`, hardcoded at
`sim.go:3195`) — *not* a fraction of the option's own premium. At a spot of
49 295 USD that is a fixed **≈147.9 USD** half-width applied to every contract,
whether it is worth 20 USD or 2 000.

**So any option whose theoretical premium is below ≈148 USD is quoted with a
negative bid and therefore has no bid at all** — which is exactly the partition
[[RT-051]] measured, the 4.45:1 ask skew [[RT-052]] attributed to the dealer, and
the monotone moneyness rule [[RT-053]] found.

**And skew explains RT-054's regime.** `SkewPerLotBps: 5` with
`LotQty = mvBasePrecision/20` makes each lot of inventory shift both quotes by
`spot × 5/10000 / 20` ≈ **24.6 USD**. Skew enters with a **minus** sign, so a
**short** dealer inventory raises the bid. Roughly **6 lots short (0.3 contracts)**
lifts a zero-premium contract's bid above zero — after which *every* contract is
quoted two-sided, at every moneyness and every tenor. That is precisely the Q3
signature.

**Claims.**
1. **Arithmetic**: predicted half-width `spot × 0.003` reproduces the sampled
   quote (bid 16 / ask 314, half-width 149) within **5%**.
2. **Threshold**: OTM contracts with a premium below ≈148 USD are the ones
   without a bid; the ask-only fraction tracks the fraction of contracts priced
   under that level.
3. **Regime**: the dealer's **net option inventory is short during Q3** and not
   short in the quarters where the ask skew appears.

**Falsifiers.**
(a) predicted half-width misses the sampled quote by >5% → the formula is not what
drives the observed quotes;
(b) dealer inventory is **not** short in Q3, or is short in Q1/Q2/Q4 too → skew
does not explain the regime and RT-054's anomaly returns to unexplained, with a
fourth candidate eliminated;
(c) ITM contracts also lose their bid at some point → the `bid > 0` gate is not
the operative condition.

**Discriminating experiment E-071**, preregistered before the run: seed 607, 8 h,
`-log-mode full`; measure `option_dealer` net option inventory per quarter across
all contracts.
Status: **MIXED** — claim 1 supported at 0.7% error and the mechanism found;
claim 3 falsified and mis-specified (aggregate measured, per-contract required).


**E-071 — H-066 MIXED. The mechanism for the missing bid is FOUND and verified to
0.7%, closing [[RT-051]]–[[RT-053]]. Claim 3 is falsified *and* was
mis-specified; [[RT-054]]'s Q3 regime stays open.**
Base: `a666d02faede3d40f046b11e60eb672c59386a94`, seed 607, 8 h, `-log-mode full`.

**Claim 1 SUPPORTED — the arithmetic reproduces the observed quotes.** Predicted
half-width `spot × 30 bps` = **147.9 USD** against the sampled quote's observed
**149** (bid 16 / ask 314): **0.7% error**, against a 5% falsifier.

**The mechanism, and it is a single conditional** (`optionmm.go:351`):

    half := mm.spotMid * mm.cfg.SpreadBps / 10000   // 30 bps of SPOT
    bid  := alignDown(theo-half-skew, tick)
    ask  := alignUp(theo+half-skew, tick)
    if bid > 0 {                                     // bid: conditional
        mm.SubmitOrder(sym, exchange.Buy, ...)
    }
    mm.SubmitOrder(sym, exchange.Sell, ...)          // ask: unconditional

**The half-spread is 30 bps of the underlying, not of the option's premium.** At
a 49 295 USD spot that is a flat ≈148 USD applied to every contract, whether it is
worth 20 USD or 2 000. The ask is always placed; the bid only when it prices
above zero. So **every option worth less than ≈148 USD is quoted ask-only**.

**This closes three prior findings quantitatively.**

| option premium | half-spread as % of premium |
|---:|---:|
| 100 USD | 148% |
| **148 USD** | **100%** (bid hits zero) |
| **193 USD** | **76.7%** |
| 600 USD | 24.7% |
| 1 200 USD | 12.3% |

[[RT-051]] measured a **76.8% median OTM half-spread** — implying a typical OTM
premium of **≈193 USD**, exactly on this curve — against **10.0% for ITM**, which
implies ≈1 480 USD. The partition, the 4.45:1 ask skew of [[RT-052]] and the
monotone moneyness rule of [[RT-053]] are all one line of code.

**Claim 3 FALSIFIED, and the test was mis-specified — both are recorded.**
Measured `option_dealer` net option inventory by quarter, all three dealers on all
three venues:

| quarter | inventory (contracts) |
|---|---|
| Q1 | −6.3 to −8.5 |
| Q2 | −14.1 to −17.8 |
| Q3 | −39.3 to −44.2 |
| Q4 | **−46.7 to −51.6** |

The dealer is **short in every quarter**, monotonically, so falsifier (b) fires:
skew does not distinguish Q3. Fourth candidate eliminated.

**But the measurement was of the wrong quantity, which I state rather than
bury.** `skew` uses `q.inventory` — `q` is the **per-contract** quote state, so
the skew that matters is the dealer's position *in that contract*, not its
aggregate book. I measured the aggregate. So the skew hypothesis is **not
properly tested**: it is falsified only in the aggregate form I wrote, and the
per-contract form remains untested. My preregistered arithmetic ("6 lots lifts a
zero bid") was also built on the aggregate reading and does not survive it — an
aggregate of −8 contracts would imply skew large enough to bid for everything in
Q1, which the 14% Q1 ratio contradicts. **That contradiction is itself evidence
the skew term is per-contract, as the source says.**

**Net position of the lineage.** The headline question — why out-of-the-money
options have no bid — is **answered and verified**. RT-054's separate question —
why Q3 quotes everything in pairs — remains open, with four candidates now
eliminated and one (per-contract skew) identified as untested because I measured
the wrong aggregate.

**Owner-facing statement.** A half-spread proportional to the **underlying**
rather than to the option premium makes cheap options untradeable on one side by
construction. `SpreadBps: 30` is hardcoded at `sim.go:3195`, not configurable.
Whether that is intended is a design question; that it produces a 148 USD
half-spread on a 165 USD option is arithmetic.

**Scope.** One seed, one configuration. The 0.7% arithmetic check is against a
single sampled quote; the premium-to-spread curve is derived, not fitted to the
distribution.

Recorded as RT-055.


---

## F. Findings

See `research/red-team-findings.md` for the full records.

- **RT-001** — expiry settlement bypasses the conservation tracker.
  REAL BUG, layer = evidence/detector, severity high, reachable in ordinary
  cells. Fixed on the audit branch. No funds lost.
- **RT-002** — option expiry net cash ≠ 0 by ≤6 units. EDGE CASE by magnitude;
  **disposition open** — sign, bound, accumulation and extraction potential are
  not yet established, so "edge case" is a classification, not a safety claim.
  See H-008 below for the work that would close it.
- **RT-003** — bounded no-violation results (INV-2, INV-5, INV-6, identity).
- **RT-006** — latency is delivered as configured across 225 link x channel
  rows; no unearned speed advantage. Transport only.
- **RT-055** — **the missing option bid is one conditional.** `optionmm.go:351`
  places the ask unconditionally and the bid only `if bid > 0`, where
  `half = spotMid × 30bps` is **30 bps of the underlying, not of the option
  premium** — a flat ≈148 USD on every contract. Any option worth less than that
  is quoted **ask-only**. Verified: predicted half-width 147.9 against 149
  observed, **0.7% error**. This closes [[RT-051]] (76.8% median OTM half-spread ⇒
  ≈193 USD premium, on the curve), [[RT-052]] and [[RT-053]] as one line of code.
  Claim 3 falsified — dealer inventory is short in **all four** quarters — and the
  test was **mis-specified**, since `skew` is per-contract and I measured the
  aggregate. [[RT-054]]'s Q3 regime stays open.
- **RT-054** — **tenor eliminated** as the explanation for [[RT-053]]'s anomaly,
  the third candidate to fall. The moneyness rule is robust to tenor (ITM exactly
  paired in **every** band; OTM never above 47%), but the anomaly is orthogonal:
  outside Q3 short tenor does not produce paired quoting (OTM 27.7%/10.4%/3.9%),
  and inside Q3 **every** cell is exactly 100%. Q3 is now precisely described —
  the dealer **stops quoting all contracts beyond ~2 h tenor** (zero placements at
  ≥4 h, against 170 439 in the run) and quotes the rest in strictly paired form,
  one unpaired ask in 60 000 placements. **Why is unknown, and this lineage stops
  here** as a bounded negative result rather than a fourth guess.
- **RT-053** — bucketing the dealer's quotes by **moneyness at quote time** gives a
  clean monotone rule over the 1.31 M placements that carry the volume —
  **100.0% in the money, 74.9% at the money, 16.7% out of it** — with the ITM
  books quoted **262 427 bids against 262 427 asks, equal to the unit**. But it
  **refutes my explanation of [[RT-052]]'s Q3**: the excursion survives
  reclassification (OTM quarters 14%, 17%, **100%**, 12%, an 8.3x spread), so the
  symmetric quarter is real dealer behaviour, not a terminal-spot artifact. Every
  bucket reads exactly 100% in Q3 — the signature of a different quoting path.
  Two candidate mechanisms now eliminated; the third is not being guessed.
- **RT-052** — the option dealer quotes **in-the-money books with exactly one bid
  per ask, every quarter, to the unit** (97 767/97 767, 103 384/103 384,
  51 189/51 189, 83 952/83 952) and out-of-the-money books at **4.45 asks per
  bid**. It places **90.8%** of all OTM asks, so [[RT-051]]'s missing bid is the
  dealer's own quoting. **Risk-limit withdrawal is excluded** — the ratio falls
  only 21%→14% across the run, against a falsifier needing a halving. But **Q3 is
  symmetric (25 092/25 093) on those same books**, which neither a static policy
  nor a withdrawal predicts, so the mechanism stays open. Confound named and
  untested: moneyness is classified against terminal spot while options relist
  through the run.
- **RT-051** — the option surface **splits perfectly on moneyness**: all 25
  in-the-money books are **99.9%** two-sided, all 25 out-of-the-money books drop
  to a median of **58.3%** and a minimum of **21.4%**, with median half-spreads of
  10.0% against **76.8%**. The worst ITM book is better than the best OTM book —
  a partition, not a tendency. The missing side is always the **bid** (no-bid
  reaches 16 962 snapshots, no-ask never exceeds 18), so **OTM option holders
  cannot sell**. Not a deep-OTM artifact: all 12 worst books are within 5.5% of
  spot, the worst being `49000-P` at **−0.6%**. Mechanism **not determined** — the
  rounding explanation was checked and is false (bid 16 USD against a 314 USD
  ask, not absent).
- **RT-050** — the cross book's cost is **realised at execution**. Across
  **216 529 fills** `noise_flow` buys ABC at **−51.50%** and sells at **−50.51%**
  against the venue's own contemporaneous consensus rate; the implied loss of
  **−246.2 M is 114.0% of the −215.9 M measured independently**, inside the
  preregistered ±25% band, with the residual accounted for by fees, snapshot
  timing and terminal-inventory valuation. Net flow −13 224.07 matches
  `positionpath`'s −13 224.08 from separately written code. **The chain from
  [[RT-031]]'s one-line self-reference to the population's largest loss is now
  closed with every link measured.**
- **RT-049** — the cross book's 116.1 bps loss rate is **not the spread and not
  inventory drift**. Its median quoted half-spread is **1.80 bp — the loss is 64x
  it**, and spread bounds at ~2% of the rate; `CDF/USD` has the population's
  *widest* spread (4.99 bp) with the second-*lowest* loss rate, so the two are not
  even ordered together. And `noise_flow` ends **net short 13 224 contracts** (14
  of 18 participants short), so a falling book would have *gained* it money —
  the inventory story is ruled out by sign, not merely unsupported. Both ordinary
  components of a taker's result are eliminated; the decomposition is reported
  **INCONCLUSIVE** rather than split by assumption.
- **RT-048** — **`noise_flow`'s loss is not a spread payment either.** On a
  fair-value notional basis it pays **5.4 bps on `ABC/USD`, 10.0 bps on
  `CDF/USD`, and 116.1 bps on `ABC/CDF`** — **21.4x** the pegged book — with
  **98.4% of the entire −219.5 M falling on the cross book**. Retracts my reading
  that this was "the only one of the three largest results that looks like a
  market outcome". At the `ABC/USD` rate the cross-book flow would have cost
  **10.0 M instead of 215.9 M**. **All three of the population's largest results
  now sit on `ABC/CDF` or on an instrument that lost its anchor** — the campaign's
  biggest winner and biggest loser are the same book.
- **RT-047** — the spot makers' configured hedge into `ABC-PERP` is executed
  **almost perfectly in quantity** (class net −1.88 contracts out of 3 674, every
  participant flat to within 2.90) and is **economically enormous**, because the
  legs decoupled: `ABC/USD` is pegged at −1.4% ([[RT-032]]) while the perp book
  falls to −29% ([[RT-044]]). The perp leg at book is **+40.88 M — 15.8x the
  inventory exposure it was written to neutralise and 46x the spot leg's own
  result**. This is the entire source of [[RT-045]]'s swing from second-largest
  donor to second-largest winner. **Two of the population's three largest results
  now trace to instruments that lost their anchors.**
- **RT-046** — all six `carry_arb` participants end at **exactly +500.00**, the
  configured cap, confirming [[RT-045]]'s class sum from the fill stream rather
  than the snapshots. But they spend only **31-44%** of the run there and first
  reach it at **3.8-5.5 h**, which **falsifies my "saturated at every layer"
  framing**: the mark clamp and funding cap latch, the arbitrageur does not. The
  saturation is **progressive, not initial**. Third appearance of the nested
  derivative-payload trap, and the first caught only because an independent
  measurement disagreed with a plausible wrong answer.
- **RT-045** — **retracts [[RT-044]]'s liquidation claim**: zero accounts are
  solvent at the mark and insolvent at the book, and the run logs **0
  liquidations**, because perp participants are heavily over-collateralised. The
  measured mark gap stands; the inference did not. What replaces it is larger:
  revaluing perp positions at the book moves **±50 M of reported value between
  classes** (`carry_arb` +38.2 M above book, `spot_maker` −46.8 M below), which
  **inverts two of [[RT-033]]'s rankings** — `spot_maker` −6.8 M → **+40.0 M**,
  `carry_arb` −2.4 M → **−40.6 M**. Mark-versus-book valuation is an owner
  decision; a ranking that does not state its basis is under-specified.
- **RT-044** — the perp **mark is pinned exactly on its ±3% clamp while the book
  is 17-29% away**. At h=8 the mark is 4 781 619 850 and the spot mid
  4 929 505 000, whose 97% is 4 781 619 850 to the unit; **32.5% of samples lie
  beyond the band** and the final-quarter mean basis is **−17.2%**, worst
  **−28.85%**, on all three venues alike. Margin and liquidation consume that
  mark, so a long is valued **26.7% above its best bid** at the terminal state and
  **liquidation cannot fire when it should**. Corrects [[RT-043]]: the −2.88% I
  published was the clamped mark's basis, a configuration constant; the book's is
  −28.85%, **38x** the funding cap rather than 3.8x.
- **RT-043** — the funding controller **saturates and latches**. With
  `Damping: 100` (a multiplier of 1.0, i.e. none) the rate is the raw premium
  hard-clamped at ±75 bps; **64% / 60% / 100%** of settlements sit at the cap, and
  on `central` **the last six consecutive settlements are all −75**. The perp
  basis reaches **−2.88%, 3.8x the cap**, so funding has under a third of the
  authority needed to close it. Every carry and basis strategy trades a signal
  that is constant for half the run. Also **confirms [[RT-042]]'s boundary
  explanation exactly**: at 12 h `north` settles once at h=8, `central` 11 times,
  `south` 5 times — every count and timestamp as predicted.
- **RT-042** — funding is **not scaled by the settlement interval**
  (`SimpleFundingCalc.Calculate` takes no interval; `funding.go:753` applies the
  rate directly), so cumulative funding over 8 h is **270 : 84 : 0 bps** on
  central : south : north for comparable per-settlement rates. **`north`'s
  perpetual settles funding zero times while trading 59 572 fills** — its first
  settlement falls at the horizon — so the campaign runs a perp with **no
  convergence mechanism at all** on one venue. Same shape as [[RT-031]], reached
  by another route. Cross-venue perp results are not comparable.
- **RT-041** — [[RT-031]]/[[RT-034]]/[[RT-035]] **reproduce on three independent
  seeds**. Levels swing 5.4x (the cross maker's net is +6.88 M at seed 607 and
  +36.92 M at 608) while the structure barely moves: book share **98.1-99.6%**,
  counterparty share **92.3-93.0%** (0.71 pp spread), extraction **49-57% of
  notional**, cross-book terminal deviation **-69% to -83%**. Base traded against
  the maker is 6 531-6 766 ABC, a 3.6% spread - the signature of the position
  caps ([[RT-028]]) fixing the quantity while the seed sets only its value.
  Headline figures should now be quoted as these ranges, not seed-607 points.
- **RT-040** — keying marks by **venue** as well as asset drives the population
  closure to **exactly zero** and reconciles every spot-only class between the
  fill stream and the account snapshots to **within 1 USD across 5.13 M fills**.
  The venues publish different marks ([[RT-032]]); both my tools collapsed them.
  Same shape as [[RT-039]]: **collapsing a dimension the system actually varies
  over**, hidden inside the quantity a self-test compares against. No published
  ranking moves — `classpnl` used per-row marks throughout — and RT-034's 98.1% /
  92.3% are unchanged.
- **RT-039** — the closure residual I reported at 0.89% for four checkpoints was
  **my own tool discarding non-USD fee revenue**: the venue takes `ABC/CDF` fees
  in CDF, and `classpnl` summed only `FeeRevenue["USD"]`. Corrected, the
  population's trading loss equals the venue take to **496 USD on ~558 M of gross
  flow (0.0001%)** across three seeds — conservation holds essentially exactly.
  Retracts [[RT-033]]'s ±5 M resolution limit (twelve classes reinstated as
  measured) and resolves [[RT-036]]'s mis-specified falsifier in its favour
  (per-participant noise ≈2 USD, not 19 600). **A conservation check must
  enumerate every asset the system moves value in, not the one the report is
  denominated in.** `populationclosure` has the same defect, flagged not fixed.
- **RT-038** — **the entire latency model is inert.** Four configurations
  spanning 1 µs to 500 ms — including a 625x change to the cross maker and a
  20 000x change to `noise_flow` — produce **byte-identical runs** (same
  `greeks.json` md5), while a seed change moves every class and a 3 s delay does
  change the run. Every configured delay is below the **1 s `step`**. The
  campaign's per-role latency heterogeneity, and the validation that refuses to
  run without "an explicit nonzero delayed link", change nothing. **E-054 proves
  the cause is the step, not broken plumbing**: at a 1 ms step the same latency
  change does alter the run, at ~120x the wall time. Kills the
  latency reading of [[RT-035]] — a latency race is *impossible* here, so the
  mispricing mechanism stands.
- **RT-037** — construction order **buys queue priority under price-time matching
  and exactly nothing under pro-rata**. Same class, same book, same venue, two
  participants differing only in build order: on `north` (price-time) every pair
  differs and the earlier one wins 7 of 8, by **5.1%–11.5%** on the material
  books; across the sixteen `central`/`south` (pro-rata) pairs the difference is
  **exactly zero to the unit**, largest 0.10%. The campaign's own venue
  heterogeneity supplies the null control. **On a price-time venue a maker's
  result is 5–11% decided by a `for` loop in `sim.go`**, and the advantage never
  rotates. See [[RT-036]].
- **RT-036** — the scheduler grants **permanent construction-order privilege**:
  `eventHeap.Less` breaks equal-timestamp ties by registration id, and repeating
  events keep that id forever, so same-interval actors fire in build order for
  the whole run. Measured on eight identically-configured suppliers per venue,
  carry-adjusted PnL is **strictly monotone in registration order on all three
  venues, 24/24 rows, p ≈ 1e−14** — but acting first is *worse*, not better, and
  the spread is only 0.28%. Negligible for a price-elastic buyer; **unmeasured,
  and expected to matter, for makers under price-time matching, where acting
  first is queue position at the touch**.
- **RT-035** — the chain closes: **noise_flow −188.6 M → abc_cdf_spot_maker
  −157.7 M → triangle_arb**, five counterparty rows summing to the maker's total
  to the unit. The maker charges uninformed flow **1.08% of notional** and pays
  `triangle_arb` **48.9% of notional** on 1.72% of its volume — 45x the rate,
  83.6% of its gross. Not adverse selection: a maker quoting around a mid that is
  −69% from fair. The campaign's entire competitive result is one mechanism.
- **RT-034** — `triangle_arb`'s dominance is **one book and one counterparty**:
  98.1% of its +174.2 M is earned on `ABC/CDF`, and 92.3% of that comes from
  `abc_cdf_spot_maker` — the self-anchored maker of [[RT-031]] — at **24 080 USD
  per ABC traded, 48.8% of notional**. [[RT-031]] and [[RT-033]] are one artifact.
  Corrects RT-033: `noise_flow` is **not** a direct counterparty of
  `triangle_arb` on any book; the transfer is two-step through the maker.
- **RT-033** — the campaign's competitive outcome is **not close and not
  competitive**. With the shared revaluation tide removed, `triangle_arb` takes
  **+174.2 M, 75.4% of every gain in the population, with 6 participants of
  252** — 25x the best market-making class per head — funded by `noise_flow` at
  −220 M. With [[RT-031]] the mechanism is one class harvesting a
  self-referential book's 69% dislocation from uninformed flow configured into
  it. Separately, ranking this population on raw Δequity would have named
  `latent_liquidity` and `elastic_supplier` its great losers; carry-adjusted they
  are 22nd and 18th, because **99.4% of the suppliers' apparent loss is
  revaluation of a given endowment**, not trading.
- **RT-032** — the campaign's ABC/USD price level is a **configured peg**, not a
  market outcome. All 8 elastic suppliers per venue run with
  `elastic_supplier_reference_half_life: 0`, which `supplier.go` documents as an
  exogenous anchor. Their aggregate terminal position equals the position their
  venue's terminal price predicts with **ratio 1.0001 / 1.0001 / 1.0002**, no
  participant at its cap. The three heterogeneous venues agree on the level to
  0.0026%. Because `elastic_supplier_symbols` is null, the peg reaches **only
  ABC/USD**: CDF/USD, ABC/CDF and ABC-PERP have no price-elastic demand, which is
  the missing ingredient behind [[RT-031]]'s 69% drift. **One anchored book, the
  rest floating.**
- **RT-031** — the `ABC/CDF` cross book is **self-referential**: its maker's
  `ReferenceSymbol` is the book itself, and the index that publishes the symbol
  is a median of the same three books. It starts at its bootstrap (+0.15% high)
  and ends **−69.15%** below it while `ABC/USD` moves −1.05%. Nothing enforces
  triangular consistency except a capped arbitrageur. `triangle_arb`'s +3.675%
  is harvesting a standing dislocation, not outcompeting anyone. **Any
  cross-asset conclusion from this configuration is measuring a drifting
  self-referential book.**
- **RT-030** — **RESOLVED by E-046 (see RT-031).** Was open. The triangular residual holds one sign for essentially
  the whole run (6 to 36 sign flips in five hours; dominant sign 98.9-99.7% of
  instants), so `triangle_arb`'s +3.675% is not a competed market outcome. The
  measured magnitude — a 3.2x gap between the observed cross mid and both the
  implied rate and the configuration's own bootstrap — is **withheld pending a
  units check**, because a standing 3.2x dislocation with an arbitrageur present
  is not what a working ecology looks like. Next: read `abc_cdf_spot_maker`'s
  anchor.
- **RT-029** — capital spans **510x** across classes, and normalising by it moves
  the table **62 rank-places over 21 classes**: `latent_liquidity` goes 21st to
  11th, `metaorder_trader` 6th to 18th. The **top two are stable**, so
  `triangle_arb`'s dominance is not a capital artifact. Absolute class PnL is a
  fair summary at the extremes and misleading in the middle. **Owner decision.**
- **RT-028** — the actor classes are bounded at **wildly different scales** —
  1 ABC for `option_value_taker`, 5 for `dated_carry_arb`, 200 for the makers,
  500 for `carry_arb`, 10 000 for `elastic_supplier` — a spread of four orders of
  magnitude, and **no artifact reports the ceiling beside the score**. Not a
  defect; prudent risk design. But a class-level comparison cannot be normalised
  for it from the evidence a run emits. **Owner decision**, and the remedy is to
  emit a constant already known at construction.
- **RT-027** — no actor class is inert, but `dated_carry_arb` collapses to
  **0.003** of its first-half order rate while the dated board thins from three
  contracts to one. **Resolved in E-042: design, not a stall** — a 50-lot net
  position cap per contract binds against a one-signed basis, and the edge
  scaling by time-to-expiry *lowers* the bar near expiry so it cannot be the
  cause. The caveat stands with a named cause: the class's score is front-loaded
  and bounded by `MaxPosPerSym` while uncapped classes compound for the full run,
  so class-level rankings compare different things.
- **RT-026** — RT-001 and RT-025 are members of a **class**: four checks whose
  only output is a log line, silenced together by one deployment flag —
  `conservation_violation`, `margin_interest_failed`,
  `funding_settlement_failed`, and `price_unavailable` on the liquidation path.
  The interest one is economically material per RT-014. One remedy covers all
  four. **E-040 sharpened this**: `exchange/` has **zero** discarded errors and
  zero blocks that neither report nor propagate, so RT-026 is a
  **reporting-channel** defect, not an error-handling one. **Owner decision.**
- **RT-025** — the live conservation check is **silenced by the logging
  configuration**: `verifyConservation` computes violations every tick and
  discards them when no venue logger exists, which is the case for every
  `log_mode: none` run. No counter, no error, no field in
  `terminal-outcome.json`. Same defect shape as RT-001, one layer up. The
  campaign runs `log_mode: full` so its headline runs are covered; **this
  audit's own last eight runs were not**. **Owner decision.**
- **RT-024** — the population ledger **closes**. Participant net change plus the
  venue take leaves a residual whose implied base inventory is 8 185-8 239 ABC
  per head against a 10 000 endowment — revaluation, not unaccounted value.
  Bounded no-violation result; the screen is reusable. **E-036 attempted an
  exact version and the method was invalid**: `AccountValuationSpec` reaches only
  wallet balances, while derivative exposure is valued from the instruments'
  own marks, so fixing the spec cannot remove derivative revaluation. **E-037
  tried a third route, regression across eight seeds, and the intercept was not
  identified** — 0.15% drift spread and R²=0.04. The gap is **not closable from
  the current artifact surface**; one extra field (net base-asset position at
  each capture) would make it computable directly.
- **RT-023** — the venue design **confounds matching rule with funding
  interval**: north is the only price-time venue and also the only 8-hour funding
  venue, so no venue effect can be attributed to either. The one clean contrast
  (central vs south, same rule, 1 h vs 2 h) often shows a larger term than the
  one being attributed to the rule. **Replicated across seeds 607-610 (E-034)**,
  which also identifies the funding axis as the one attributable venue factor:
  shortening 2 h to 1 h is worth +1.6 M to `noise_flow` and -4.4 M to
  `abc_cdf_spot_maker` on average.
- **RT-022** (amended by E-033) — venue placement carries an environment term:
  `triangle_arb` differs by **8.23%** across venues against 0.03% between
  same-venue clones. **Replicated across four seeds (E-034)**: the term is
  positive in all of 607-610 at 653 k-992 k, so it is structure rather than
  sampling noise — but it remains **unattributable**, since the venues differ
  deliberately in both rule and funding.
  **Methodological, not an economics defect.**
- **RT-021** — the index that anchors every mark has no staleness bound: a venue
  that goes one-sided stops updating without losing its vote, so two silent
  venues outvote the only live one and the index publishes a price no venue is
  showing. Median-of-three works; permanent membership is the weakness.
  **Quantified in E-031**: the harmful configuration — exactly two silent venues
  outvoting one live one — occurred **zero times in 18 000 instants on
  `ABC/USD`**, the book that anchors the perp mark, and **2.44% of the time on
  the thin `ABC/CDF`**. Live on the cross books, absent on the margin-governing
  one. **Owner decision.**
- **RT-020** — one minimum-size resting quote that never trades moves the perp
  mark 1.90%, and the clamp permits 3.00%, against a 500 bps maintenance margin
  — 38% and 60% of the buffer respectively. The anchoring defence is complete in
  coverage and fails closed, but bounds manipulation without pricing it: the mid
  it reads is unweighted, so displacement is independent of the manipulator's
  size. Property of the rules, **not an observed exploit**. **Owner decision.**
- **RT-019** — every expiring contract settles against one shared underlying
  observation, so a calendar hedge nets exactly. Bounded no-violation result,
  pinned because nothing else asserts it and its regression would be visible
  only to hedgers.
- **RT-018** — borrowed spot exposure is governed by nothing. Both liquidation
  entry points walk positions; spot debt is not a position. An account 50 000 USD
  underwater on a venue-financed short is untouched by either, and no event
  records that the venue carries the loss — while the same economic short
  expressed as a derivative is closed out and charged to the insurance fund.
  Mechanism live (`AutoBorrowSpot: true`); whether a real run reaches negative
  equity this way is **not measured**. **Owner decision.**
- **RT-017** — the system holds three valuations of an account, and their
  pricing discipline is **inversely ordered to their authority**: the scoring
  path records provenance and bounds staleness, the risk engine uses a live mark
  and fails closed, and the borrow gate — which sets leverage — reads a constant
  fixed before the run. Structural observation over RT-015 and RT-016 rather
  than a new instance. **Owner decision.**
- **RT-016** — the borrow gate prices collateral from a static oracle pinned to
  the bootstrap price, so a 50% fall in ABC leaves borrowing power unchanged at
  twice the collateral's worth. **Latent, not live**: measured excursion in the
  anchored control config is 1.06% over a full 5-hour run against a 25% haircut,
  one-directional and still growing with run length; CDF stays inside 0.17%.
  Re-measure before trusting a stress configuration. **Owner decision.**
- **RT-015** — the borrow gate values cash only. It ignores unrealized losses
  (an account 200 USD under water still borrows the full 500) and counts
  reserved margin as free collateral (100 USD available, 500 USD borrowed), so
  the same capital backs a resting order and a loan at once. The risk engine
  values positions; the borrow gate does not, and it is the more generous of the
  two. Reachable and exercised. **Owner decision.**
- **RT-014** — collateral interest is forgiven entirely below 105.12 USD of
  debt, and the delivered rate rises with principal from 0 bps to the configured
  500. The cost of leverage depends on how much is borrowed. Reachable by every
  actor; borrowing is enabled in the campaign and the charge runs as a phase
  job. Measured over the full 5-hour run in E-024: aggregate delivered rate is
  **at least 458 bps of the configured 500**, with 20.9% of charges in the three
  lowest buckets where it is 50-75%. E-022's 30-minute figure of 250-430 bps was
  a warm-up artefact and is **corrected**. The threshold is denominated in raw
  asset units, so it is worth about 5 256 USD on ABC against 105.12 USD on
  USD.
- **RT-013** — funding is not invariant under account partition: the more
  fragmented side gets the rounding, so splitting helps a payer and hurts a
  receiver, and the exchange residual absorbs the difference. EDGE CASE by
  magnitude (1 unit in 8801, bounded by one unit per extra account per
  settlement); recorded because it is the second instance of the RT-008
  mechanism family and establishes it as a pattern.
- **RT-012** — a position on a settlement-pending contract suspends liquidation
  of the entire account: the margin profile fails closed and the caller skips
  the client. The account is frozen, not privileged (new orders are refused),
  but the position rides the market uncapped and the fund absorbs the delay —
  measured at 400 USD if closed at the breach price versus 650 USD after the
  market moved. **Owner decision**; reachability in campaign configs not
  established.
- **RT-011** — margin is aggregated across books, liquidation is not, so the
  position an actor loses depends on which book ticked rather than on which
  exposure caused the deficit. CORRECT BUT SURPRISING / specification question,
  severity medium, reachable by any account with positions in two books.
  Conservation intact; the deficit is transient, closed on the losing book's
  own next tick. **Owner decision.**
- **RT-010** — bounded no-violation results: every exit from the reservation
  lifecycle (FOK kill, post-only refusal, insufficient balance, partial fill,
  cancel of a partially filled remainder) restores the earmark exactly; and
  self-trade prevention is cancel-maker, leaving the book uncrossed, the
  collateral released, the owner notified, and the notifications ordered by
  placement rather than by map iteration.
- **RT-008** — the taker fee depends on how the counterparty's liquidity was
  sliced, always in the taker's favour, and no conservation check can see the
  shortfall. EDGE CASE by magnitude at campaign prices (<=9 quote units in
  25 009, 0.036%); the free-fill boundary below 19.00 USD is **not reachable**
  at ABC 50 000 / CDF 3 000. Disposition open — fee semantics are the owner's.
- **RT-009** — a hidden order keeps full time priority over displayed size at
  the same price, so `Hidden` weakly dominates `Normal`. INTENDED MODEL
  ASSUMPTION, and **NOT EXERCISED**: nothing in the tree constructs a non-Normal
  order.
- **RT-007** — the participant-information audit's retained review oracle
  accepted evidence the production auditor rejects, because it sorted the event
  stream instead of merging it. REAL BUG, layer = evidence/detector, severity
  medium, **not reachable in campaign output** (only the streaming path is
  called). Fixed on the audit branch; regression is a fifteen-fault differential
  between the two implementations.

**H-009 — a bankrupt account's spot wallet is not seized, so the insurance fund
absorbs a deficit an aggregate-solvent account could have covered.**
Status: **OPEN — specification question, not a bug to fix here.**
Origin: reading `liquidate` at `exchange/exchange.go:2242`; confirmed by E-008,
where the defaulter's 500 USD spot balance is untouched while the fund absorbs
the whole 100 USD deficit.
Evidence it is deliberate: `Client.BorrowedSpot` is documented as splitting a
liability by wallet precisely so that "perp equity, liquidation estimates, and
snapshots must not charge a spot-credited loan to the perp wallet."
Competing readings: (a) wallets are deliberately segregated, so this is
INTENDED MODEL ASSUMPTION; (b) the model claims cross-margin netting across
wallets, in which case the venue eats a loss a solvent account should bear.
Owner decision required. The audit does not resolve it and does not change it.

**H-008 — RT-002 residual is directional and extractable.** Status: **OPEN.**
The instructions correctly reject "small relative to capital" as a disposition.
Required: sign of the residual, theoretical per-settlement bound, whether it
accumulates, who receives it, and whether splitting a position across accounts
increases extraction. Not yet run.

---

## G. Corrections and invalidated evidence

**G-1 — the naive settlement invariant was wrong.**
Earlier reasoning treated "Σ signed positions = 0 at one settlement price" as
implying "Σ settlement cashflow = 0", and the observed −59,972,385,325 as
possible value destruction. That is incorrect: surviving positions carry
unpaired entry prices because partial closes already realised PnL in cash. The
project auditor states the same rule ("not required to be zero"). Had the fix
been applied under the wrong interpretation it would have *silenced a true
positive*; it was checked first. INV-4 records the correct rule.

**G-2 — E-004 was not a valid test.** A passing test on the unpatched base is
not evidence of anything; it was replaced, not reinterpreted.

**G-3 — a 20-minute horizon is economically empty for this config.** Its only
balance-change reasons are `trade_settlement`, `initial_deposit`, `borrow`,
`interest_charge`. No funding, expiry, settlement, exercise or liquidation.
Any earlier "no violation" claim measured at 20 minutes covers none of those
mechanisms and must not be read as covering them.

---

## H. Residual risk and continuation

Not exercised anywhere in this audit: liquidation and default, collateral
release accounting, transfers in flight, option exercise against a live hedge,
relisting under a reused symbol, the execution path from admission to clearing,
negative and zero price domains at settlement.

Owner decisions outstanding: RT-002 disposition (per-position truncation vs a
book-level carry, as futures already do); H-009 (whether a bankrupt account's
spot wallet should be seized before the insurance fund absorbs the deficit).

H-004, H-005, H-006, H-010 through H-018 are now closed within their tested
scope; E-008 through E-017 hold the evidence. Three of them (H-013, H-016,
H-017) were closed by falsification — the risk they named was already
foreclosed — and those are recorded in as much detail as the confirmed ones.

Next, in priority order:

1. **H-020 — the rest of the cross-book surface.** E-018 opened it and found
   RT-011 on the first probe, which argues for staying in this frame. Still
   untested: an option exercised against a live hedge; collateral held in CDF
   while trading `ABC/CDF`, where `buildAccountMarginProfile` skips any
   instrument whose quote asset differs (`perp.QuoteAsset() != quote` →
   `continue`), so exposure in a second quote asset is invisible to the first
   one's risk check; and an option exercised against a live hedge. The
   settlement-pending question is closed as RT-012.
2. **H-022 — the partition-dependence pattern.** RT-008 (fees) and RT-013
   (funding) are the same mechanism seen twice. Margin and settlement use the
   same per-item `MulDiv` idiom and have not been checked from this angle; a
   sweep of every per-item integer charge would either close the pattern or find
   the instance where the magnitude matters.
2. **RETIRED — H-013 — decision-record coverage is a property of the run
   configuration, not of the code.** `-record-market-data-receipts` requires an explicit
   `-market-data-receipt-roles` list, so an un-audited participant class emits
   no decision record and can neither pass nor fail the causality check. The
   fairness claim in RT-006 and the causality guarantee in H-011 therefore cover
   whatever fraction of the 27 participant classes that list names. Measuring
   that fraction is cheap — read the roles flag used by the campaign's own run
   scripts and compare it against the class table — and it converts "decisions
   are causal" into "decisions are causal for N of 27 classes", which is the
   honest form of the claim.
3. **H-008** — RT-002 residual: sign, bound, accumulation, extraction.

Reproduction: worktree `redteam/economic-audit` on `a666d02`; build
`cmd/multivenue` and `cmd/mvanalyze`; run dev-607 seed 607 for 7h to reach
expiry; `mvanalyze -metric conservation` for the identity. The evidence-audit
differential needs no run: `go test ./analysis/ -run
TestAuditMarketDataEvidenceOracleAgreesUnderFaults -v`.

Base check performed this session: `feature/r2-cdf-survival-successor` is still
at `a666d02` (2026-09-03) and is the most recent scientific line;
`autoresearch/ffa-ecology-gen0` remains at `230e78f` (2026-08-31). The audit is
not reported against a stale base.
