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

H-004, H-005, H-006, H-010, H-011 and H-012 are now closed within their tested
scope; E-008, E-009, E-010, E-011 and E-012 hold the evidence.

Next, in priority order:

1. **H-013 — decision-record coverage is a property of the run configuration,
   not of the code.** `-record-market-data-receipts` requires an explicit
   `-market-data-receipt-roles` list, so an un-audited participant class emits
   no decision record and can neither pass nor fail the causality check. The
   fairness claim in RT-006 and the causality guarantee in H-011 therefore cover
   whatever fraction of the 27 participant classes that list names. Measuring
   that fraction is cheap — read the roles flag used by the campaign's own run
   scripts and compare it against the class table — and it converts "decisions
   are causal" into "decisions are causal for N of 27 classes", which is the
   honest form of the claim.
2. **H-008** — RT-002 residual: sign, bound, accumulation, extraction.
3. The execution path from admission to fill to clearing, which no experiment
   here has touched.

Reproduction: worktree `redteam/economic-audit` on `a666d02`; build
`cmd/multivenue` and `cmd/mvanalyze`; run dev-607 seed 607 for 7h to reach
expiry; `mvanalyze -metric conservation` for the identity. The evidence-audit
differential needs no run: `go test ./analysis/ -run
TestAuditMarketDataEvidenceOracleAgreesUnderFaults -v`.

Base check performed this session: `feature/r2-cdf-survival-successor` is still
at `a666d02` (2026-09-03) and is the most recent scientific line;
`autoresearch/ffa-ecology-gen0` remains at `230e78f` (2026-08-31). The audit is
not reported against a stale base.
