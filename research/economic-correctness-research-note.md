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
Status: **OPEN — registered, not yet tested.**
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
book-level carry, as futures already do).

Next, in priority order: H-004 (liquidation / debt extinguishment), then H-005
(double collateral release), then H-008 (RT-002 residual characterisation), then
H-006 (detector sensitivity to a mis-directed transfer, via controlled
mutations).

Reproduction: worktree `redteam/economic-audit` on `a666d02`; build
`cmd/multivenue` and `cmd/mvanalyze`; run dev-607 seed 607 for 7h to reach
expiry; `mvanalyze -metric conservation` for the identity.
