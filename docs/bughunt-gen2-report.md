# Bug Hunt Report — Generation 2 (July 2026)

Branch `autoresearch/bughunt-gen2`. This document records (1) what the last five
commits on the branch already fixed, (2) the fourteen uncommitted gen-2 fixes in
the working tree, (3) the regression coverage written for them, and (4) the
findings of the independent post-fix hunt (three parallel review/hunt agents)
run before merging to `main`.

---

## Part 1 — What was: the five commits under review

### `1da1b07` — docs: development report for July 17–20 work

`docs/changelog-2026-07-17-20.md` (381 lines): full narrative of the ecology
buildout, derivatives stack, and gen-1 bug-hunt arcs. No code.

### `6eb0487` — test: randomized invariant fuzzer

Two fuzz harnesses (699 lines): `tests/invariant_fuzz_test.go` drives random
order/cancel/deposit/withdraw sequences against the exchange core and checks
conservation invariants (reserved ≥ 0, balance conservation, no negative free
balances) after every step; `tests/invariant_fuzz_deriv_test.go` runs the
derivatives lifecycle (listing → trading → expiry) under the same randomized
regime.

### `3498917` — fix: iceberg display priority, hidden-order MD leak, cancel-maker STP

Three exchange-logic fixes with tests (423 lines across book/, matching/,
exchange/, types/):

- **Iceberg display priority**: refilled display quantity re-queues at the back
  of the price level instead of inheriting the original time priority.
- **Hidden-order MD leak**: hidden orders no longer appear in published depth.
- **Cancel-maker STP**: self-trade prevention gained the cancel-maker policy
  branch in both matchers.

### `26025c2` — fix: nil IndexProvider crash in price loop

Comma-ok guard in the mark-price loop for exchanges constructed without an
IndexProvider, plus `tests/concurrent_fuzz_test.go` (196 lines) hammering the
gateway from concurrent goroutines under `-race`.

### `2e3a230` — fix: reconnect account preservation; iceberg display validation

Reconnecting a client ID no longer resets its balances/positions to zero;
iceberg orders with display ≥ total or display ≤ 0 are rejected at validation;
one more invariant (I6) added to the fuzzer.

---

## Part 2 — What is changed: the fourteen gen-2 fixes (working tree)

### Borrowing (`exchange/borrowing.go`)

1. **`BorrowMargin` rejects `amount <= 0`.** A negative borrow created negative
   debt (the exchange owing the client) and silently drained the wallet past
   every downstream limit check.
2. **`RepayMargin` nil-client guard.** Unknown client previously panicked on
   map access.
3. **`RepayMargin` rejects `amount <= 0`.** A negative repay inflated both the
   wallet and the debt — free credit paired with more liability; zero was a
   silent no-op success.
4. **Repay wallet selection.** A borrow credits either the perp wallet (margin
   borrow) or the spot wallet (auto-borrow for a spot order), but the debt
   ledger does not record which. Repay now draws from whichever wallet holds
   the funds — perp first, preserving the historical path — so a spot-credited
   loan is no longer permanently unrepayable. Failure when neither wallet alone
   covers the amount is atomic.

### Client snapshot (`exchange/client.go`)

5. **Perp snapshot reports `Borrowed` and nets it from `NetAsset`.** Borrowed
   quote is a liability, not owned equity; the perp entries now match the spot
   snapshot's treatment.

### Exchange core (`exchange/exchange.go`)

6. **`AddInstrument` is a no-op on an already-listed symbol.** Re-listing
   swapped in a fresh empty book, stranding every resting order — and its
   reservation — in the old book where no cancel or match could reach it.
7. **`CheckLiquidations` nets borrowed quote debt out of equity.** A loan could
   otherwise mask an undercollateralized account and dodge liquidation.
8. **`EstimateLiquidationPrice` nets borrowed debt out of collateral.** The
   reported liquidation price was too optimistic for leveraged accounts.
9. **`CheckAndSettleFunding` uses comma-ok assertion.** A custom Instrument
   reporting `IsPerp()` via an embedded `*PerpFutures` but with a different
   concrete type panicked the whole funding sweep.

### Order handling (`exchange/order_handling.go`)

10. **Market-order funds check includes worst-case quote fee** (both the
    `OrderMarginer` and `Margined` branches, via new `quoteFeeHeadroom`).
11. **Limit-order reservation includes worst-case quote fee**
    (`order.Reserved = margin + fee`). An account funded to the last cent of
    margin is now rejected up front instead of going insolvent on the fill fee.
12. **Foreign-fee affordability check** (new `checkForeignFeeFunds`, called from
    `reserveOrderFunds`). A fee denominated in a third asset (neither base nor
    quote — e.g. a BNB fee on BTC/USD) has nothing backing it in the
    reservation; it is now checked at placement against the perp wallet for
    margined/order-margined instruments and the spot wallet otherwise.
    Documented limitation: this is an affordability check, not a lock — several
    resting orders can over-commit the same foreign balance.

### Settlement (`exchange/settlement.go`)

13. **Nil `FeePlan` treated as zero-fee at settlement** (new `calcClientFee`,
    both taker and maker call sites). Previously panicked on the first fill of
    a client constructed without a fee plan, diverging from the reservation
    path which already treated nil as zero-fee.

### Simulation clock (`simulation/clock.go`, `simulation/scheduler.go`)

14. **`Advance` walks event-by-event.** `ProcessUntil` now `SetTime`s the clock
    to each due event's timestamp (forward-only guard: past-due events fire at
    the current time, never rewinding) before firing, so a callback observes
    its own scheduled instant — and anything it schedules relative to "now"
    chains correctly — rather than the end of the whole jump. Afterwards the
    clock rests at the requested target. Nil-clock guard added in
    `ProcessUntil`.

---

## Part 3 — Regression coverage

Four test files cover the fourteen groups; every fault-detecting test was
verified to fail on pre-fix code (fixes stashed, tests run, stash restored):

- `tests/bughunt_regression_test.go` + `tests/bughunt_regression_borrow_test.go`
  — written alongside the fixes (11 tests).
- `tests/bughunt_regression_gen2_gaps_test.go` — 13 tests / 22 cases closing
  the audit gaps: `EstimateLiquidationPrice` debt netting, both market-order
  fee-headroom branches, the option (OrderMarginer) limit branch, the
  perp-wallet and market arms of the foreign-fee check plus positive
  "funded client still trades, fee debited exactly" guards, zero-amount
  borrow/repay boundaries, repay wallet preference and atomicity, the snapshot
  `Borrowed` field, book identity on re-listing, and the maker-side nil-FeePlan
  call site.
- `tests/bughunt_regression_clock_test.go` — 5 tests: rest-at-target with
  beyond-target deferral, repeating-tick instants, past-due no-rewind, the
  nil-clock guard, and a 4-producer concurrent Schedule-vs-Advance race test.

One documented non-coverage: `calcClientFee`'s nil-*client* arm is unreachable
through the public API (a fill implies both clients connected); defensive only.

`make test` and `make test-race`: all packages pass, zero data races; the
concurrency test survived 25 consecutive `-race` runs.

---

## Part 4 — Independent post-fix hunt (gen 3)

Three parallel agents ran before merge: a commit-range reviewer over
`HEAD~5..HEAD` plus the working tree, a logic hunter over the exchange core,
and a logic hunter over simulation/book/matching/marketdata. The reviewer
returned MERGE-SAFE with zero findings; the two hunters returned fifteen,
three of them runtime-proven with failing tests. Every finding was verified
at source before fixing. Thirteen were fixed; two deferred with rationale.

### Fixed (with regression tests)

| # | Finding | Fix |
|---|---------|-----|
| 1 | **Fee-headroom leak on partial fill** (HIGH). Instrument-level `releaseOrderMargin` computes margin-only `stillNeeded`, while placement reserved margin + worst-case fee: the first fill released the ENTIRE fee headroom, leaving the resting remainder's future fee unbacked — precisely the insolvency the gen-2 fix targets. | Exchange-side `restoreFeeHeadroom` after `Settle` re-reserves the remainder's fee and tops `order.Reserved` back up (`exchange/settlement.go`). Instrument release stays margin-only — the fee concept never leaks into the instrument layer. |
| 2 | **Snapshot double-nets the liability** (HIGH, reporting). Spot row (pre-existing) and the gen-2 perp row both subtracted the same account-level `Borrowed`. | Debt attribution: new `Client.BorrowedSpot` records the spot-credited share; each wallet row nets only its own portion. |
| 3 | **Spot-credited loan liquidates a solvent account** (MED-HIGH). Perp equity netted total `Borrowed`, but auto-borrow-spot credits the SPOT wallet — the cash never entered the perp wallet, so netting it there understates equity → premature `forceClose`. | `CheckLiquidations` and `EstimateLiquidationPrice` net `BorrowedPerpPortion` only. |
| 4 | **`Transfer` accepts negative amounts** (HIGH). Direction reverses while the availability check guards the declared source — reserved perp margin siphoned to spot unchecked. | `amount <= 0` rejected. |
| 5 | **Base-asset fee on margined instruments unbacked** (MED). `checkForeignFeeFunds` exempted base fees, valid only for spot (which nets against the received base leg); a margined fill exchanges no base leg, so `PercentageFee{InQuote:false}` drove perp base balance negative, invisible to quote-only equity. | Margined/order-margined instruments now pre-check base-denominated fees against the perp wallet like any third asset. |
| 6 | **Repay stuck across wallets** (MED). All-or-nothing per wallet: perp 60 + spot 60 could not repay 100 though the account held 120. | Repay splits the debit — perp first, spot remainder — atomically against combined available. |
| 7 | **Interest always debits perp wallet** (MED). A spot-only borrower's empty perp balance went negative every sweep. | Interest split by attribution: each wallet billed its `Borrowed*Portion` share. |
| 8 | **Zombie repeating ticker** (HIGH, runtime-proven). Cancel of an in-flight repeating event found nothing in the heap (event popped, mid-fire) and the re-push was unconditional — the ticker fired forever after `Stop()`. | Scheduler keeps a `cancelled` set; the re-push honors it. |
| 9 | **EMA mark re-seeds from raw basis** (HIGH, runtime-proven). `emaBasis == 0` doubled as the uninitialized sentinel, but integer decay legitimately reaches 0 — the next sample discarded all smoothing and one print teleported the mark (→ false liquidations). Both `EMAMarkPrice` and `ClampedEMAMarkPrice`. | Explicit `seeded` flag. |
| 10 | **Zero-interval repeating event hangs the sim** (MED, runtime-proven). `Time += 0` never passes `untilTime`; `ProcessUntil` loops forever. | Interval clamped to ≥ 1ns in `ScheduleRepeating`; `SimTimerFactory.NewTicker` panics on non-positive duration, mirroring `time.NewTicker`. |
| 11 | **`simTimer.Stop` race** (MED-HIGH). Unsynchronized `stopped`/`eventID`, and `close(t.ch)` racing a mid-fire callback's send → send-on-closed-channel panic. | Mutex on timer state; the channel is never closed (matching `time.Ticker.Stop` semantics). |
| 12 | **Frozen EMA for large windows** (HIGH arithmetic / MED impact). `alpha = 20000/(N+1)` floors to 0 for N ≥ 19999 (24h at 3s sampling = 28800), freezing the mark at its seed forever. | `emaAlpha` floors the coefficient at 1. |
| 13 | **Concurrent `Advance` loses time** (MED, latent regression). Both callers computed targets from the same base after the gen-2 rework dropped the `current += delta` atomicity. | `goal` accumulator: each Advance claims a disjoint window; concurrent calls compose additively. |

Also fixed while there: `MDPublisher.Unsubscribe` leaked the gateway
reference forever once a client dropped its last subscription.

### Deferred, with rationale

- **Fee headroom priced at `order.Price`, worst-case taker** (LOW): a crossing
  sell filling at a better maker price owes a slightly larger quote fee than
  reserved, and a `FeeModel` with maker > taker under-reserves resting orders.
  Magnitude is a fraction of one fee tick on price-improved fills only; the
  settlement force-debit and liquidation sweep absorb it. Revisit if a fee
  model with maker > taker ever ships.
- **MD delta drop on full gateway buffer**: the seqNum is burned, so a lagging
  subscriber sees a gap with no re-snapshot path. Real feeds solve this with
  gap-detection + snapshot re-request; that is a protocol feature, not a
  one-line fix. The drop site now documents the contract (consumers must treat
  a seq gap as a resync signal). Tracked as a realism-roadmap item.
- **Multi-order foreign-fee overcommit** (pre-existing, documented in gen 2):
  the placement-time affordability check is not a lock; several resting orders
  can commit the same foreign balance. Left to settlement/liquidation
  invariants, as before.

### Gen-3 regression coverage

- `tests/bughunt_regression_gen3_test.go` — 6 tests: negative/zero transfer,
  partial-fill fee headroom (exact `Reserved = margin(rem)+fee(rem)` check),
  spot-loan vs perp liquidation estimate, per-wallet snapshot netting,
  interest wallet attribution, base-fee-on-perp pre-check.
- `simulation/scheduler_regression_test.go` — 5 tests: cancel-while-firing,
  zero-interval clamp, concurrent-Advance additivity, Stop-vs-Advance race
  (`-race`), NewTicker panic contract.
- `price/calculators_regression_test.go` — 2 tests: no re-seed after decay to
  zero, alpha floor keeps large windows moving.
- Split-repay semantics: `TestRegressionRepayInsufficientInBothWalletsIsAtomic`
  updated (combined-available shortfall stays atomic-fail) +
  `TestRegressionRepaySplitsAcrossWallets` added.

---

## Status

All suites pass (`go test ./...`), race detector clean across every package
(`go test -race ./...`). Thirteen of fifteen hunt findings fixed with
regression tests; two deferred with written rationale. Merged to `main`.
