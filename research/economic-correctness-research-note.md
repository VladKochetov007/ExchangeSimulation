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
