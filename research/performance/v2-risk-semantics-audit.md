# V2 risk-engine semantics audit

Adversarial correctness audit of the liquidation / margin path. Worktree
`exsim-auditR`, branch `perf/auditR`, Go 1.26.7. **No timing claims are made
anywhere in this document**; every measurement below is a count of events or a
comparison of evidence digests.

Measurement config: `research/configs/v2-integrated-longrun/dev-607.json`,
seed 900101, `-log-mode full`, 5 and 15 simulated minutes, `GOMAXPROCS=1`.
Three venues (`north`, `central`, `south`), 1 s automation interval.

Reproductions live in `tests/risk_semantics_audit_test.go` and
`tests/risk_ordering_metamorphic_test.go`. The tests for **open** findings
fail by design and are skipped unless `AUDIT_FINDINGS=1` is set, so an open
finding does not turn `make test` red:

```
AUDIT_FINDINGS=1 go test ./tests/ -run TestAudit -count=1
```

---

## 1. Risk dependency graph

What can change an account's equity, margin requirement, maintenance
requirement, liquidation eligibility, or collateral value — and what the code
actually recomputes when it happens.

There are exactly **two** entry points into risk evaluation in the whole
codebase (`grep CheckLiquidations|CheckPositionMarginerLiquidations`):

* `exchange/exchange.go:1800` — `CheckLiquidations(symbol, perp, mark)`, called
  once per margined symbol from the price tick, immediately after that
  symbol's mark is applied.
* `exchange/exchange.go:1438` / `exchange/expiry.go:234` —
  `CheckPositionMarginerLiquidations()`, called from the 1 s expiry tick after
  `UpdateDerivativeMarks`, sweeping only accounts that hold option positions.

Nothing else re-evaluates risk. In particular no fill, cash flow, or wallet
movement does.

| # | State mutation | Site | Risk quantity it moves | Recomputed? |
|---|---|---|---|---|
| 1 | Perp / dated-future mark | `updateAllPerpPrices` `exchange.go:1625` | equity (uPnL), notional, maintenance, warning | **Yes** — `CheckLiquidations` for *that symbol only*, `:1800` |
| 2 | Index price | `indexPriceLocked` `exchange.go:1563` | mark, funding rate | Via (1) |
| 3 | Mark becomes *unavailable* (`ClearMarkReferences`, `:1714`) | deferral path | account becomes unpriceable | No check of its own; instead poisons *other* symbols' checks — **F1** |
| 4 | Option premium mark + underlying mark | `UpdateDerivativeMarks` `expiry.go:280` | option equity contribution, maintenance | **Yes** — `CheckPositionMarginerLiquidations`, same 1 s tick |
| 5 | Option IV / Greeks | `Black76Premium` → `opt.SetMarks` | via (4) | Via (4) |
| 6 | Funding **rate** | `UpdateFundingRate` | only future cash flow | n/a |
| 7 | Funding **settlement** (cash debit/credit) | `settleFunding` `funding.go:730` | perp balance → equity | **No** |
| 8 | Borrow principal | `BorrowingManager.BorrowMargin` | perp balance, `BorrowedPerpPortion` | Admission check only (`validateCrossMarginCollateral`); no liquidation re-eval |
| 9 | Borrow interest | `ChargeCollateralInterest` `exchange.go:1826` | perp **and spot** balance → equity | **No** |
| 10 | Position size / entry (fill) | `settleExecution` → `UpdatePosition` | size, entry, position margin, realized PnL | **No** |
| 11 | Realized PnL on reduce | `realizedPerpPnL` → balance | equity | **No** |
| 12 | Trading fees | `recordFeeRevenue` / settlement | balance | **No** |
| 13 | Spot leg settlement | `settleSpotExecution` | spot `Balances` only | n/a — spot is not collateral (documented) |
| 14 | Wallet transfer spot↔perp | `Transfer` `exchange.go:839` | perp balance → equity | **No**; admission uses `balance − reserved` and ignores unrealized loss — **F4** |
| 15 | Collateral valuation / FX | static `CollateralPrices` in `BorrowingConfig` | borrow admission only | Not part of liquidation equity (documented) |
| 16 | Instrument expiry, settlement price unavailable | `settlementPending`, `expiry.go:398` | positions **retained**, symbol skipped at `exchange.go:1924`, `:2091`, `:2363` | Positions become invisible to risk — **F6** |
| 17 | Expiry settlement (cash + margin release) | `settleExpiredInstrument` | balance, positions removed, margin released | **Partially** — `CheckPositionMarginerLiquidations` runs next in the same tick, but sweeps only accounts holding *option* positions; a perp-only account waits for the next price tick |
| 18 | Liquidation itself | `liquidate` `exchange.go:2202` | balance, position, reserved, insurance fund | Inside `CheckLiquidations` the profile is rebuilt per client, so later clients see it. Inside `CheckPositionMarginerLiquidations` the cached profile is invalidated for the liquidated `(client, quote)` only |
| 19 | Order placement / cancel | `order_handling.go` | `PerpReserved`, not equity | Reservation checks only |

Cadence consequence: rows 7, 9, 10, 11, 12, 14 and 17 all move equity with no
re-evaluation, so an account can sit below maintenance for up to one price
interval (1 s in dev-607). Evaluating risk on a mark cadence is what real
venues do, so this is **INTENDED MARKET SEMANTICS** — with the caveat in F6
below, where the wait is unbounded rather than one tick.

Equity, for the record, is
`PerpBalance(quote) − BorrowedPerpPortion(quote) + Σ uPnL over margined books in that quote`
(`exchange.go:2122`). Spot balances and other assets are not collateral. That
is already recorded in `docs/realism-gaps.md` § "Cross-margin scope" and is
**INTENDED MARKET SEMANTICS**, even though `validateCrossMarginCollateral`
(`borrowing.go:171`) values *all* assets in *both* wallets when admitting a
loan. The two modules use different collateral definitions on purpose; the
inconsistency is documented, not new.

---

## 2. Adjudication: the unmarked-book abort

### 2.1 What the code does

In `buildAccountMarginProfile` (`exchange.go:1914`) the perp branch resolves a
mark for **every** perp book in the quote currency before it looks at whether
the client holds anything in that symbol (`exchange.go:1942`–`1954`; the
position loop only starts at `:1956`). On failure the whole profile fails, and
`CheckLiquidations` reports `price_unavailable` and **`return`s** —
`exchange.go:2113`.

### 2.2 Verdict — this is not the fail-closed conservatism it looks like

Fail-closed is the right instinct in exactly one case: the account has
exposure to a book whose mark cannot be resolved, so its equity is genuinely
unknown and liquidating on a guess would be worse than waiting. That case is
correct and should stay.

The implemented behaviour is broader in two independent ways, and neither is
defensible as conservatism:

**F1 — a book the account has no position in can suppress its liquidation.**
An account whose own risk is fully priceable is not liquidated because some
*other* instrument in the same quote currency has no mark. A real venue's risk
engine values the portfolio it actually holds; an unrelated instrument's price
feed is not an input to that account's solvency. "Do not liquidate on
incomplete information" does not apply, because the information about *this*
account is complete. The zero-exposure book contributes 0 to equity, 0 to
notional and 0 to maintenance whatever its mark is — the failure is caused by
work whose result is discarded.

Minimal reproduction: `TestAuditUnmarkedZeroExposureBookSuppressesLiquidation`.
Client 1 is long 10 BTC-PERP at entry 100, balance 100 USD, mark 94 → equity
40 vs maintenance 47, and there is a covering bid. With only BTC-PERP listed
the account is liquidated (`TestAuditControlUnderwaterAccountIsLiquidated`
passes). Adding an empty, never-marked `ZZZ-PERP` in the same quote — in which
client 1 holds nothing — suppresses the liquidation entirely.

**F2 — one account's unpriceable exposure aborts the sweep for every other
account.** `CheckLiquidations` uses `return`, not `continue`. Because clients
are visited in ascending ID order, a low-numbered client holding an unpriceable
instrument silently cancels the liquidation check for every higher-numbered
client at that mark — including accounts with no exposure to the unpriceable
instrument at all. The sibling sweep,
`CheckPositionMarginerLiquidations` (`exchange.go:2029`), uses `continue` for
the identical condition, so the two paths disagree about what a profile failure
means.

Minimal reproduction: `TestAuditProfileFailureAbortsSweepForOtherAccounts`.
Client 1 holds a short in a never-marked option (a genuinely per-account
unpriceable exposure — `addPositionMarginerExposure` only reaches `riskMark`
for symbols the client actually holds); client 3 is underwater on BTC-PERP
with no option exposure and is never evaluated.

Note that the existing test
`exchange/margin_profile_determinism_test.go:11` asserts F1's behaviour as
correct: it builds a profile for client 1, who holds **no positions in either
book**, and requires an error. It pins the determinism of the error (which is
worth keeping) but takes the error itself as the oracle.

### 2.3 Measured frequency: zero

`price_unavailable` records across the whole evidence tree, both durations:

| operation | 5 min | 15 min |
|---|---|---|
| `perp_index` | 24 | 24 |
| `derivative_mark` | 18 | 18 |
| `listing` | 9 | 9 |
| **`liquidation`** | **0** | **0** |
| **`option_liquidation`** | **0** | **0** |
| `liquidation_price_estimate` | 0 | 0 |

All 51 records land in the first 4 simulated seconds, before the `ABC/USD`
spot book is two-sided, and the counts are identical at 5 and 15 minutes —
i.e. purely warm-up. There are also **zero** `liquidation` and zero
`liquidation_check` events in either run: in dev-607 no account ever reaches
its warning tier, let alone maintenance, so the liquidation path is never
exercised at all.

The near-miss is real, though. The deferral path at `exchange.go:1714` calls
`ClearMarkReferences` on a perp whose index is unavailable and then, later in
the same tick, `CheckLiquidations` walks that now-unmarked book while building
every other account's profile (`liveBookReferencePrice` on a freshly listed
future's empty book returns `ErrNoBookPrice`). The only reason it did not fire
is that the three affected windows are all before anyone holds a position.
Any config where a dated future or perp loses its index *after* trading starts
reaches F1 immediately.

### 2.4 The fix is evidence-preserving

I applied the candidate fix — resolve the mark lazily, only for books in which
the client actually holds a position, and change the `return` at
`exchange.go:2113` to `continue` — rebuilt, and reran dev-607/900101/5 min.
`evidence-artifact-hash.json` is **byte-identical** to the unpatched run. The
only test in the repository that fails under the fix is
`TestBuildAccountMarginProfileUsesCanonicalBookOrderForUnavailableMarks`,
which asserts the behaviour F1 describes for a client with no positions; all
of `./exchange/` and `./tests/` otherwise passes. The fix has been reverted in
this worktree — this audit ships findings and tests, not behaviour changes.

---

## 3. Same-timestamp ordering

### 3.1 Where the ordering is specified

Same-timestamp order is specified and deterministic at three levels:

* `Runner.drainDeterministicPhases` (`simulation/runner.go:289`) — venue jobs,
  then venue ingress, then egress, then actors in `AddActor` order, repeated to
  a fixed point.
* Automation jobs run in registration order (`exchange.go:1431`–`1439`): price
  → funding → collateral → expiry.
* Within `updateAllPerpPrices`, books are visited in lexicographic symbol order
  (`exchange.go:1728`).

Consequences that follow from this and are **INTENDED MARKET SEMANTICS**:
a trade at T never affects the mark at T (marks precede ingress, so a mark
reflects the book as of before T's trades); a position opened at T pays no
funding at T; an order arriving at an expiry timestamp is rejected because
settlement and delisting already ran; a liquidation's forced close consumes
book liquidity before any same-timestamp new order.

### 3.2 F3 — cross-margin liquidation depends on lexicographic symbol order

`updateAllPerpPrices` interleaves mark application and liquidation *per
symbol*: apply symbol A's mark, sweep A, apply symbol B's mark, sweep B
(`exchange.go:1755`–`1800`). `buildAccountMarginProfile` prices non-trigger
symbols from their last **stored** mark. So the sweep triggered by the first
symbol in sort order values every cross-margined sibling at the **previous
tick's** mark, while the sweep triggered by the last symbol sees a fully
refreshed set. The code comment at `exchange.go:1723` states this explicitly
and resolves it by sorting — which makes the choice reproducible but leaves it
economically arbitrary, because instrument names carry no economic content.

Metamorphic test:
`TestAuditSameTickMarkOrderingChangesLiquidationOutcome`. Two perps in the
same quote, one account long 10 of each at entry 100, balance 101. Both marks
start at 100 (equity 101 vs maintenance 100 — solvent). One tick later the
riser marks at 140 and the faller at 70. Against the **fully refreshed** mark
set the account is comfortably solvent: equity 101 + 400 − 300 = 201 against a
maintenance requirement of 105.

* Riser sorts first (`AAA-PERP` rises, `BBB-PERP` falls): riser's sweep sees
  equity 501 vs 120 — no breach; faller's sweep sees the full refreshed set,
  201 vs 105 — no breach. **The account survives.**
* Faller sorts first (`AAA-PERP` falls, `BBB-PERP` rises): the faller's sweep
  values the riser at its stale 100, giving equity 101 + 0 − 300 = −199 against
  maintenance 85. **The solvent account is liquidated.**

Renaming the instruments — economically meaningless — flips the outcome. This
is a genuine ordering dependence between two events the model does not order,
and unlike the others in §3.1 it is not a defensible venue convention: no
venue evaluates one leg of a cross-margined portfolio against a stale price for
the other leg when both prices were computed in the same pass. The correct
sequence is the one the code already computes and then discards: build the
complete candidate mark set (`candidates`, `exchange.go:1728`), commit all
marks, then run the risk sweep once over accounts.

**This is populated in the real config.** In dev-607, 33 of 79 clients hold
positions in two USD-quoted margined books at the same time
(`ABC-FUT-1735696801` and `ABC-FUT-1735711201` — different tenors, so genuinely
different basis and capable of moving in opposite directions on one tick).
The exposure surface exists; it produced no wrong outcome in the 5 and 15
minute runs only because nothing came near maintenance.

### 3.3 F6 — positions in a settlement-pending contract are invisible to risk

When an instrument reaches expiry with no declared settlement price
(`expiry.go:398`), resting orders are cancelled once and **positions are
deliberately retained** until a settlement source appears. But every risk path
skips a `settlementPending` symbol outright: `buildAccountMarginProfile`
(`exchange.go:1924`), `CheckLiquidations` (`:2091`), the mark loop (`:1662`),
`CheckAndSettleFunding` (`:2363`), and the symbol list in
`CheckPositionMarginerLiquidations` (`:1984`).

The position therefore still exists but contributes **zero** to equity and
**zero** to maintenance. An account short that contract has its liability
erased from the risk calculation — equity is overstated and it can escape a
liquidation it deserves. An account long it has its asset erased — equity is
understated and it can be liquidated on a symbol it is solvent on. The halt is
correct; excluding a retained position from valuation is not. The right
treatment is to keep valuing the position at the last declared mark (or, if
that too is gone, to refuse the profile the way F1's *legitimate* case does),
not to price it at zero.

Not reproduced by a test: constructing it requires driving a contract past
expiry with its settlement source withheld, which is more machinery than the
finding needs. Measured frequency in dev-607: **zero** — no
`expiry_settlement_pending` records, because the first dated-future expiry is
about two hours into simulated time and the runs are 5 and 15 minutes. A grep
for `expiry_settlement_pending` across the archived artifacts in this worktree
also returned nothing, but those are mostly summary JSON rather than full
evidence streams, so that is weak evidence.

### 3.4 F4 — wallet withdrawal ignores unrealized loss

`Transfer` (`exchange.go:839`) admits a perp→spot movement on
`PerpAvailable = PerpBalances − PerpReserved` and runs no risk check. Position
margin is reserved at **entry** price (`ForceReservePerp`,
`settlement.go:273`), so the reservation does not shrink as the position loses
money. An account with an open loss can therefore withdraw cash it has
already lost, becoming liquidatable the instant the transfer lands, and nothing
re-evaluates it until the next price tick. A venue limits a cross-margin
withdrawal by `equity − initial margin`, which subtracts unrealized loss.

Zero measured impact: no simulation actor calls `Transfer` (grep over
`simulation/`, `simulations/`, `actor/`, `cmd/` finds no call sites) and the
15-minute run contains zero `transfer` events. This is a latent library
defect.

### 3.5 F7 — a breach closes every leg without re-testing solvency

`CheckLiquidations` collects `positions` before the breach test and then closes
all of them (`exchange.go:2144`), as does
`CheckPositionMarginerLiquidations` (`:2039`). Neither re-tests whether the
account is back above maintenance after the first close. In hedge mode
(`PositionLong` + `PositionShort` on one symbol) that means both legs are
force-closed on a single breach. Real venues stop liquidating once the account
clears maintenance.

Zero measured impact: dev-607 runs entirely in netting mode — all 41,394
`position_update` records carry `"position_side":"BOTH"` — so
`positionsAcrossSides` returns a single position and the loop closes one leg.

### 3.6 F8 — borrow interest truncates to zero and is never charged

`ChargeCollateralInterest` computes, per minute,
`interest = borrowed × rate × 60 / (31 536 000 × 10 000)` (`exchange.go:1854`)
in integer arithmetic with **no remainder carry**. At the configured 500 bps a
debt must be at least 10,512,000 units for even one unit of interest to be
charged in a minute; below that the charge truncates to zero every minute,
forever, rather than accruing.

Measured in the 15-minute run: 15 `borrow` events, 0 `repay` events, and
**0** `margin_interest` / `interest_charge` events. Aggregate outstanding debt
per (venue, client, asset) ranges from 28,970 to 3,557,722 units — every one of
them 3× to 360× below the threshold. Borrow interest in dev-607 is identically
zero for the whole run despite a declared 500 bps rate.

The absolute amount forgone is negligible at these debt sizes (~0.34 units per
minute on the largest debt), so this does not move any economic result here.
What it does introduce is a step nonlinearity: borrowing is free below a
threshold and priced above it. Any experiment that reasons about borrow cost
should either carry the sub-unit remainder or accrue from a timestamp delta.

---

## 4. Classification

| # | Finding | Class | Measured frequency (dev-607 / 900101 / ≤15 min) |
|---|---|---|---|
| **F1** | Unmarked book with **zero account exposure** fails the margin profile and suppresses liquidation (`exchange.go:1942`) | **SCIENTIFIC-BLOCKING MARKET-LOGIC BUG** | **0** occurrences |
| **F2** | One account's unpriceable exposure `return`s out of the sweep, skipping every higher-ID account (`exchange.go:2113`) | **SCIENTIFIC-BLOCKING MARKET-LOGIC BUG** | **0** occurrences |
| **F3** | Cross-margin liquidation decided against a sibling's stale mark; outcome depends on lexicographic symbol order; liquidates a solvent account (`exchange.go:1755`–`1800`) | **SCIENTIFIC-BLOCKING MARKET-LOGIC BUG** | **0** wrong outcomes; exposure surface populated (33/79 clients cross-margined in USD) |
| **F6** | Positions in a settlement-pending contract contribute 0 equity and 0 maintenance (`exchange.go:1924`) | **SCIENTIFIC-BLOCKING MARKET-LOGIC BUG** | **0** occurrences (no expiry reached in ≤15 min) |
| **F4** | `Transfer` admits withdrawal on `balance − reserved`, ignoring unrealized loss; no risk re-check (`exchange.go:839`) | **AMBIGUOUS SEMANTICS** | **0** — no caller in the simulation |
| **F7** | A single breach closes every leg without re-testing solvency (`exchange.go:2144`) | **AMBIGUOUS SEMANTICS** | **0** — netting mode only |
| **F8** | Borrow interest truncates to zero below ~10.5 M units; no remainder carry (`exchange.go:1854`) | **AMBIGUOUS SEMANTICS** | **100 %** — 15/15 debts below threshold, 0 interest charged |
| — | Risk evaluated only on the mark cadence; fills, fees, funding, interest and settlement cash do not re-trigger it | **INTENDED MARKET SEMANTICS** | bounded by one 1 s price tick (except F6, which is unbounded) |
| — | Equity is quote-wallet only; spot and other assets are not collateral, while borrow admission values all assets | **INTENDED MARKET SEMANTICS** | documented in `docs/realism-gaps.md` § Cross-margin scope |
| — | Marks precede ingress; trade at T does not move the mark at T; funding snapshot precedes same-T fills; expiry precedes same-T orders; liquidation precedes same-T new orders | **INTENDED MARKET SEMANTICS** | specified in `simulation/runner.go:289` and `exchange.go:1431` |
| — | Mark resolution for zero-exposure books (the work F1's failure comes from) | **PERFORMANCE ONLY** (secondary; the correctness fix removes it as a side effect) | 93.5 % of pairs per the sweep census |

### Does existing evidence need rescoring or rerunning?

**No rerun is required for anything measured here, and existing dev-607
evidence can be left as it stands.** The reason is blunt: dev-607 at 5 and 15
minutes produces **zero liquidations, zero margin calls, and zero
`liquidation`-path `price_unavailable` records**. F1, F2, F3 and F6 are all
defects in decisions that were never made. The candidate fix for F1 and F2
reproduces the evidence digest byte-for-byte, which is direct confirmation.

Two conditions would change that answer, and both are cheap to check against
any long-run artifact before trusting it:

1. **Any `liquidation` event on an account holding positions in two or more
   margined books of the same quote asset.** That is F3's trigger. Such a run
   cannot be rescored from its artifacts — the counterfactual requires
   re-evaluating equity against the full refreshed mark set, which the evidence
   does not contain — so it would need a rerun after the fix.
2. **Any `expiry_settlement_pending` record.** That is F6's trigger, and it is
   plausible in multi-hour runs, which do cross the dated-future expiries.
   Positions held in the pending contract were valued at zero for as long as the
   pending state lasted.

A run with a `price_unavailable` record whose `operation` is `liquidation` or
`option_liquidation` is F1/F2 firing directly and should be treated the same
way.

I could not check the holdout artifacts (`research/artifacts/v2-7-p7d/holdout`
is out of scope for this audit), so nothing here speaks to them.

## 5. What I could not establish

* Whether F3 has ever changed a published result. It requires a liquidation on
  a cross-margined account, which no run I was permitted to execute or read
  produced. The two checks in §4 settle it for any given artifact.
* Whether F6 has fired in the campaign's multi-hour runs. My runs are far too
  short to reach a dated-future expiry, and the archived artifacts in this
  worktree are summaries rather than full evidence streams.
* The intended venue model for a retained position in a settlement-pending
  contract. `docs/realism-gaps.md` documents the halt but not the valuation,
  so F6's *correct* behaviour is my judgement (value at last mark), not a
  restatement of a written spec.
* Whether the `PERFORMANCE ONLY` census figures quoted in the task brief
  (900 sweeps, 1,514,700 pairs, 93.5 % empty) reproduce, since I did not
  instrument for them; they are consistent with 3 venues × 300 ticks × 3
  margined symbols, which the evidence does confirm.
