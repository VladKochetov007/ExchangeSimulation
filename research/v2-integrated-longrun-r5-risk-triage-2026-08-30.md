# r5 risk-semantics triage before dev-607

Date: 2026-08-30
Scientific branch: `autoresearch/ffa-ecology-gen0`
Base HEAD at triage start: `887899f`
Status: **dev-607 blocked under the previous freeze-candidate semantics; semantic
fix applied; mechanical gate rerun required before a successor candidate.**

## 1. Scope and provenance

An independent market-logic audit produced on the isolated performance branch
`autoresearch/v2-performance-research` raised five candidate defects in the
risk engine (F1, F2, F3, F6, F8). This document is the required triage against
the exact current scientific HEAD.

What was read, and how:

* `git fetch github autoresearch/v2-performance-research` (read-only).
* `git show FETCH_HEAD:research/performance/v2-risk-semantics-audit.md`,
  `…/v2-preview-semantics-audit.md`, `…/v2-simulator-performance.md`.
* The active scientific worktree was never switched to that branch, and
  **nothing was merged or cherry-picked from it**. The performance commits
  `af8535d`, `57077e9`, `2989934` remain unintegrated.

`git merge-base HEAD FETCH_HEAD` resolves to `887899f`, i.e. **the performance
branch was cut from exactly the current scientific HEAD**, so the audit's base
and this triage's base are the same tree and no code drift had to be accounted
for. That was verified rather than assumed. Every finding was nevertheless
re-derived from HEAD source and re-reproduced by tests written here.

No holdout was read, listed, parsed, or run: neither
`research/artifacts/v2-7-p7d/holdout` nor the configs `holdout-619.json`,
`holdout-631.json`, `holdout-641.json`.

## 2. Method

Reproductions live in `tests/r5_risk_triage_test.go` and are driven entirely
through the **exported** API — `CheckLiquidations`, `UpdatePerpPrices`,
`ChargeCollateralInterest`, `ConfigureAutomation` — so each asserts an
economic outcome (was this account liquidated?) rather than an internal call
sequence. Each defect test is paired with a control that shows the same
economics without the trigger.

Reachability was decided empirically, not by argument. A **triage probe** ran
the registered `dev-607` config, seed 607, `log_mode=full`, GOMAXPROCS=4, for
**6h30m of simulated time** — long enough to cross the first two short dated-
future expiries (t=7201 s, t=14401 s) and the long expiry (t=21601 s), which
the performance branch's 5- and 15-minute measurement windows could not reach.
6.5 sim-hours completed in 6m13s wall. The probe wrote to
`/home/vlad/v2-triage-probe-20260830`, outside the r5 evidence root; it is a
triage artifact, not a registered cell.

`dev-613` and `dev-617` differ from `dev-607` only in `experiment_id` and
`seed` (613 / 617). Instrument naming, tenors, listing schedule and population
are identical, so every reachability conclusion below transfers to all three
registered cells unchanged.

## 3. Findings and classification

| # | Finding | Class | r5 activation (measured) |
|---|---|---|---|
| **F1** | `buildAccountMarginProfile` resolves a mark for every same-quote margined book **before** establishing position relevance, so a book the account holds nothing in fails the whole profile | **REAL BUG AND REACHABLE IN r5** | **917 suppressed option-liquidation decisions** in 6h30m, at 2 ticks |
| **F2** | `CheckLiquidations` uses `return`, not `continue`, on profile failure, skipping every higher-ID account | **REAL BUG, REACHABLE IN PRINCIPLE; not observed firing** | 0 occurrences; perp-path immunity rests on a naming accident (§4.2) |
| **F3** | Mark application and the risk sweep are interleaved per symbol, so cross-margined siblings are valued at the previous tick's mark; outcome depends on lexicographic symbol order | **REAL BUG, EXPOSURE SURFACE POPULATED** | 0 wrong outcomes; **11 of 28** perp-core holders cross-margined, **max 5 simultaneous legs** |
| **F6** | Positions in a `settlementPending` contract contribute zero equity and zero maintenance | **REAL BUG BUT UNREACHABLE IN REGISTERED r5** | **0** occurrences in 66 settlements; mechanism excluded in §4.4 |
| **F8** | Borrow interest truncates to zero below 10,512,000 units with no remainder carry | **INTENDED AND DOCUMENTED SEMANTICS** (`docs/realism-gaps.md` § Interest accrual floor) | small-debt regime **absent** after warm-up; residual ≤ 0.475 USD per 24 h |

## 4. Per-finding detail

### 4.1 F1 — confirmed, and active in the registered configuration

**Mechanism on HEAD.** In `buildAccountMarginProfile`
(`exchange/exchange.go:1801`) the perp branch resolved `mark` — via
`GetFundingRate().MarkPrice`, else `liveBookReferencePrice(book)` — before the
position loop began. `liveBookReferencePrice`
(`exchange/order_handling.go:768`) returns `ErrNoBookPrice` on an empty book,
so an unmarked, untraded, same-quote margined book failed the whole profile.

The option branch is **not** affected: `addPositionMarginerExposure`
(`exchange/exchange.go:1948`) checks `pos == nil || pos.Size == 0` first and
only then calls `riskMark`. The two branches disagreed about whether an
instrument the account does not hold is an input to its solvency.

**Reproduction.** `TestTriageF1UnmarkedZeroExposureBookSuppressesLiquidation`.
Client 1 is long 10 base at entry 100 with 100 quote of collateral, marked at
94: equity 40 against a maintenance requirement of 47, with a covering bid
resting. The control liquidates. Adding an empty, never-marked `ZZZ-PERP` in
the same quote — in which client 1 holds nothing — suppressed the liquidation
entirely.

**Activation predicate.** *A margined (perp-core) book exists in quote Q with
no available mark and no book reference, while some account holds a position in
another book of quote Q, and a risk sweep runs before that book's next mark
tick.*

**Measured in r5.** The probe recorded **917** `price_unavailable` records with
`operation: option_liquidation`, every one carrying the reason
`cross-margin mark for ABC-FUT-…: no usable price`. They fall on exactly **two
simulated seconds**: t=7201 s (608 records) and t=21601 s (309 records) —
precisely the instants when a dated future is listed:

```
t=0       ABC-PERP, ABC/CDF, ABC/USD, CDF/USD
t=1       ABC-FUT-1735696801, ABC-FUT-1735711201
t=4       ABC-1735696804-*  (option chain)
t=7201    ABC-FUT-1735704001            <- 608 suppressed decisions
t=7204    ABC-1735704004-*  (option chain)   <- none
t=21601   ABC-FUT-1735718401, ABC-FUT-1735732801  <- 309 suppressed decisions
t=21604   ABC-1735718404-*  (option chain)   <- none
```

The listing scheduler runs inside the **expiry** automation job, and
`CheckPositionMarginerLiquidations` runs in that same job immediately after it.
The price job — the only thing that assigns a mark — already ran earlier in the
tick. So a newly listed future is unmarked for exactly one tick, and every
option-liquidation decision at that venue is discarded for that tick. The
account provably has zero exposure to the failing book: it was listed that
instant, so no position in it can exist.

The empirical asymmetry confirms the mechanism independently: **future**
listings suppress, **option** listings (t=7204, t=21604) do not, exactly as the
per-account-correct option branch predicts.

At a 7200 s short tenor a 24-hour cell has roughly **12 such listing
instants per venue**, so the registered cell discards on the order of
**3,000–5,000 liquidation-eligibility evaluations**.

**No observed changed outcome.** The probe contains **zero** `liquidation`,
`liquidation_check` and `margin_call` events of any kind, so no account came
near maintenance in 6h30m and every suppressed decision would have returned
"no breach". The defect removed the evaluation, not a known liquidation.

That distinction is why this is a block and not a retraction: the evidence
cannot say what the suppressed decisions would have been, because the
counterfactual requires re-evaluating equity against a mark the run never
stored. It is not rescorable from artifacts.

### 4.2 F2 — confirmed; dev-607's immunity is a naming accident

**Mechanism on HEAD.** `CheckLiquidations` (`exchange/exchange.go:1985`)
reported `price_unavailable` and then `return`ed on profile failure, while its
sibling `CheckPositionMarginerLiquidations` (`:1869`) uses `continue` for the
identical condition. Clients are visited in ascending ID order, so a
low-numbered account's unpriceable exposure cancelled the liquidation decision
for every higher-numbered account at that mark.

**Reproduction.** `TestTriageF2ProfileFailureAbortsSweepForOtherAccounts`.
Client 1 is solvent on the trigger perp but also holds a short in a never-marked
option — a genuinely per-account unpriceable exposure, so failing closed **for
client 1** is defensible. Client 3 is underwater on the perp with no option
exposure and was never evaluated. The control, identical but with nobody holding
the option, liquidates client 3.

**Why it did not fire in r5, and why that is not reassuring.** The probe has
zero `operation: liquidation` records. The perp sweep escapes F1's window
because it runs in the price job, and at the next tick the newly listed future
is marked *before* `ABC-PERP` is swept — because `ABC-FUT-…` sorts before
`ABC-PERP`. Had the tenor naming produced a symbol sorting *after* the
perpetual, `ABC-PERP`'s sweep would have hit an unmarked sibling, F1 would have
failed the profile, and F2 would have aborted the entire perp liquidation sweep
for all higher client IDs. **dev-607's protection here is lexicographic luck,
not an invariant** — the same fragility class as F3.

### 4.3 F3 — confirmed; exposure surface populated more heavily than reported

**Mechanism on HEAD.** `updateAllPerpPrices` (`exchange/exchange.go:1512`)
applied a symbol's mark and then immediately swept it, per symbol, while
`buildAccountMarginProfile` prices non-trigger symbols from their last **stored**
mark. The sweep triggered by the first symbol in sort order therefore valued
every cross-margined sibling at the previous tick's mark. The code comment at
`:1610` acknowledges this and resolves it by sorting — which makes the choice
reproducible but leaves it economically arbitrary. **A determinism comment is
not a specification that the economics are intended**, and no document in
`docs/` states that a cross-margin leg should be valued at a stale price when a
fresh one was computed in the same pass.

**Reproduction.** `TestTriageF3CrossMarginOutcomeDependsOnSymbolOrder`. One
account, long 10 of each of two same-quote perps at entry 100 with 101 of
collateral. Both marks are warmed to 100 before any position exists, so the
stale mark under test is a genuinely stored, available one. One tick later the
legs move to 140 and 70. Against the fully refreshed set the account is
comfortably solvent: equity 101 + 400 − 300 = 201 against a maintenance
requirement of 105. Yet:

* riser sorts first → account survives;
* faller sorts first → the faller's sweep values the riser at its stale 100,
  giving equity 101 + 0 − 300 = −199 against maintenance 85 → **the solvent
  account is liquidated, and both legs are closed**.

Renaming the legs — economically meaningless — flips the outcome.

An earlier draft of this test was wrong in an instructive way and is recorded
here so it is not repeated: the lone covering bid became the sibling's risk
mark through the one-sided `liveBookReferencePrice` fallback, so the fixture
demonstrated a *different* mechanism with the same symptom. Warming the marks
first isolates the stale-stored-mark path.

**Measured exposure in r5.** On the `north` venue alone, across 85,204
perp-core `position_update` records: 28 accounts ever hold a perp-core USD
book, and **11 of them hold two or more simultaneously**. The maximum is **5
concurrent dated futures of different tenors** (client 10 at t=21606 s, holding
`ABC-FUT-1735696801`, `…704001`, `…711201`, `…718401`, `…732801`). Different
tenors carry genuinely different basis and can move in opposite directions on a
single tick, which is exactly F3's precondition. The performance branch reported
33 of 79 accounts holding two books; the shape of the surface is confirmed and
the leg count is higher than reported.

No wrong outcome occurred, for the same reason as F1: no account approached
maintenance.

### 4.4 F6 — real, but excluded in the registered world

Every risk path skips a `settlementPending` symbol
(`exchange/exchange.go:1549`, `:1816`, `:1878`, `:1990`, `:2269`), while
`exchange/expiry.go:414` deliberately **retains** positions in it. A retained
position therefore contributes zero equity and zero maintenance, so a short has
its liability erased and a long its asset erased. The halt is correct; valuing a
retained position at zero is not.

It is unreachable in r5, and the reason is mechanical rather than statistical.
`settlementPending` is entered only when `Expirable.SettlementPrice()` fails at
expiry (`exchange/expiry.go:408`). For dated futures that resolves through
`settlementObserver.settlementPrice()`
(`instrument/settlement_obs.go:47`), whose fallback is
`lastDeclaredReference` — a field that is set on the **first** observation and
never cleared. `UpdateDerivativeMarks` (`exchange/expiry.go:309`) feeds an
observation every second of the contract's life. A contract on a 7200 s or
21600 s tenor therefore needs to receive *no observation at all across its
entire life* to go pending, which requires the underlying reference to be
unavailable for the whole tenor.

Confirmed empirically: **66 `instrument_settled` events (22 per venue) and zero
`expiry_settlement_pending` records** in the probe. Both of the audit's stated
rescore triggers were therefore checked and are absent.

The option path adds a domain check (`instrument/option.go:161` rejects a
non-positive underlying without freezing), which is a narrower window still and
also did not fire.

F6 needs no fix before r5. It should be fixed before any configuration that can
withhold a settlement source, and it is recorded as an open library defect.

### 4.5 F8 — already documented, and the audit's headline number does not survive a longer run

**This is written down.** `docs/realism-gaps.md` § "Interest accrual floor"
already records the behaviour *and* the threshold: "Collateral interest
truncates to zero each per-minute charge for small debts (below ≈ $105 at the
default 500 bps annual rate with `USD_PRECISION = 1e5`) and the fractional
remainder is dropped rather than accrued — small debts pay no interest at all."
F8 is therefore INTENDED AND DOCUMENTED SEMANTICS, not an ambiguity, and the
performance branch's AMBIGUOUS classification should be corrected: the audit
did not find this entry.

The mechanism is confirmed: `ChargeCollateralInterest`
(`exchange/exchange.go:1713`) computes
`borrowed × rate × 60 / (31 536 000 × 10 000)` in integer arithmetic, gated by
`if interest > 0`, with no remainder carry. At the default 500 bps — dev-607
sets no `CollateralRate`, so `ConfigureAutomation` applies the 500 default —
one charged unit per minute requires a debt of at least **10,512,000 units, or
105.12 USD** at `USD_PRECISION = 100000`.

**The performance branch's conclusion was a warm-up artifact.** It reported
"15/15 sampled debts below threshold, 0 interest charged, 100 % activation"
from a 15-minute window. Measured over 6h30m of the same config and seed:

| quantity | measured |
|---|---:|
| `borrow` events | 18,147 |
| `margin_interest` charges | 10,471 |
| interest charged | 1,196,203,822 units = **11,962 USD** |
| borrow principal, p50 / max | 676,396,650 / 17,799,145,124 |
| outstanding debt per (venue, client, asset) — smallest | 42,712,358 units = **427 USD** |
| debts below the 10,512,000 threshold | **0 of 33** |

Borrow interest is charged normally at r5 scale. What remains is sub-unit
rounding, bounded by strictly less than one unit per (client, asset, minute):
at most 33 × 1440 = 47,520 units ≈ **0.475 USD** across a 24-hour run, against
roughly 68,000 USD actually charged — a relative error near 7 × 10⁻⁶.

This is pinned rather than repaired, in
`TestTriageF8BorrowInterestTruncationIsBounded`, which asserts the threshold
from both sides, the small-debt zero regime, and the sub-unit bound. F8 does
not block r5.

## 5. The specific questions asked

**F3 — can any r5 participant hold cross-margin exposure to ≥2 relevant books
sharing collateral/quote currency?** Yes. 11 of 28 perp-core holders on `north`
do so simultaneously; the maximum is 5 concurrent dated futures of different
tenors, all USD-quoted and all marked in the same interleaved sweep.

**F6 — can r5 produce `expiry_settlement_pending`, and can it persist across a
risk evaluation?** No, on the evidence and on the mechanism.
`lastDeclaredReference` is set on a contract's first observation and never
cleared, so a 2 h or 6 h tenor cannot reach expiry unobserved. 66 settlements
produced 0 pending records.

**F8 — does r5 create borrow principals in the zero-interest range, for how
long, at what magnitude?** Not after warm-up. All 33 outstanding debts are at
least 427 USD, four times the 105.12 USD threshold, and 11,962 USD of interest
was charged in 6h30m. The zero-interest regime is confined to the first
minutes of a run.

**F1/F2 — can an unavailable mark occur while another account in the same risk
domain should still receive a liquidation decision?** Yes, and it does, 917
times in 6h30m. Every dated-future listing instant leaves a margined book
unmarked for one tick, and the option-liquidation sweep in that same automation
job fails for every account in the quote asset. F2's compounding abort did not
fire only because the perp sweep runs earlier in the tick and `ABC-FUT-…` sorts
before `ABC-PERP`.

## 6. Decision

F1 removes liquidation-eligibility evaluations that the registered r5
configuration demonstrably reaches. That is inside the stop rule, so:

* **the previous freeze candidate is blocked**; dev-607 was not run under those
  semantics;
* F1, F2 and F3 are fixed in a **separate scientific commit** — no semantic
  change is carried inside a performance commit, and no performance code was
  integrated;
* F6 and F8 are recorded and left as they stand, with the narrow adjudication
  above;
* the mechanical gate is rerun and a successor candidate created.

### The fix

`exchange/exchange.go`, three changes, all in the risk path:

1. **F1** — `buildAccountMarginProfile` establishes position relevance before
   resolving a mark. A book the account holds nothing in is skipped, so its
   price is no longer an input to that account's solvency. Fail-closed is
   retained for the case it exists for: exposure the account *does* hold whose
   mark is genuinely unavailable.
2. **F2** — `CheckLiquidations` `continue`s instead of `return`ing on profile
   failure, matching the sibling sweep. One account's unpriceable exposure no
   longer cancels every other account's decision.
3. **F3** — `updateAllPerpPrices` commits every mark this tick produced, then
   runs the sweep once against the complete refreshed set. Mark and funding
   publication order is untouched, so only the risk decision moves.

Regression coverage: `tests/r5_risk_triage_test.go` (five tests, each with a
control) and `exchange/margin_profile_determinism_test.go`.

The pre-existing test
`TestBuildAccountMarginProfileUsesCanonicalBookOrderForUnavailableMarks` was
the **only** test in the repository that asserted F1's behaviour: it built a
profile for a client holding no positions in either book and required an error,
pinning the determinism of the error while taking the error itself as the
oracle. Its determinism intent is preserved — the account now holds both
unmarked books, so the canonical-first book must still be the one reported —
and a companion test pins the corrected semantics.

## 7. Provenance consequence

The fix changes the evidence stream: the 917 `option_liquidation`
`price_unavailable` records disappear, and any liquidation decision they
suppressed now happens. Existing dev-607 evidence produced under the previous
semantics cannot be carried forward for the risk-path claims and is superseded
rather than rescored — the counterfactual is not recoverable from the
artifacts. F1's evidence delta is measured in §8.

## 8. Post-fix verification

The probe was rerun on the fixed tree, same config, same seed, same duration,
same GOMAXPROCS. The delta is exact and accounts for itself completely.

| quantity | before fix | after fix | delta |
|---|---:|---:|---:|
| ordered execution events | 28,021,129 | 28,020,212 | **−917** |
| `price_unavailable`, all operations | 968 | 51 | **−917** |
| `price_unavailable`, `operation: option_liquidation` | 917 | **0** | −917 |
| `price_unavailable`, `operation: liquidation` | 0 | 0 | 0 |
| `liquidation` / `liquidation_check` events | 0 | 0 | 0 |
| `expiry_settlement_pending` | 0 | 0 | 0 |

**Every removed event is one of the 917 suppressed decisions, and nothing else
in the stream moved.** The 51 records that remain are precisely the warm-up set
the audit documented — `perp_index` 24, `derivative_mark` 18, `listing` 9 — all
inside simulated seconds 0 to 3, before the `ABC/USD` spot book is two-sided.
No risk-path `price_unavailable` record survives.

Three consequences follow, and they are worth stating separately:

* **F1's repair signature is exact**: −917 records, and no liquidation appears
  in their place. The decisions now execute and all return "no breach", which
  is what a run with no account near maintenance should produce.
* **F2's and F3's repairs are evidence-neutral for dev-607.** F2 changes nothing
  because the perp sweep never failed a profile in this configuration, and F3
  changes nothing because reordering mark commitment relative to the sweep can
  only matter when a liquidation actually fires, and none does. That is a
  useful property: two of the three changes cannot have perturbed anything
  measured here, so the whole −917 delta is attributable to F1 alone.
* The ordered execution stream hash changes, as it must
  (`35b1edfc568a4754…` → `5ac3b47c18f2671c…`), so binary and evidence identity
  are superseded rather than carried forward.

Mechanical gate on the fixed tree: `make test` green across all 22 packages
with zero failures, including `simulations/multivenue` (185 s) and
`exchange_sim/tests`, plus `scripts/test-v2-integrated-longrun-contract.sh` and
the integrated long-run archive tests.

The direct acceptance test for a successor dev-607 cell is therefore simple:
**zero `price_unavailable` records with `operation` of `liquidation` or
`option_liquidation`.**

## 9. Preview audit

The matching-preview audit on the performance branch reports 0 divergences
across roughly 80,000 differential scenarios over FIFO and pro-rata matching,
with a tallied list of the states actually reached. It is recorded here as
supporting evidence that `previewMatchExcluding` agrees with committed
matching. **Its performance implementation is not integrated**, and the active
r5 gate is not a moment to adopt it.

## 10. What this triage could not establish

* What the 917 suppressed decisions would have been. No account came near
  maintenance in 6h30m, so the likely answer is "no breach" in every case, but
  the run does not contain the refreshed marks the counterfactual needs.
* Whether F3 has ever changed a published result. It needs a liquidation on a
  cross-margined account; no run examined here produced a liquidation of any
  kind.
* Anything about the holdouts. None was read or run.
* Whether a 24-hour run reaches maintenance anywhere. The probe covers 6h30m of
  24h and contains zero breaches; a longer run may differ, which is one more
  reason the fix precedes the cell rather than following it.
