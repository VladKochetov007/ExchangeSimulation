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
