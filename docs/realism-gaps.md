# Realism Gaps: Simulation vs Real Venues

Staff-quant audit (July 2026) of behaviors that differ from production crypto
derivatives venues (Binance, Deribit, CME conventions). Each entry states the
gap, the real-venue behavior, the impact on simulation results, and how it is
handled: **FIXED**, **CONFIG** (user-selectable), **DOCUMENTED** (accepted
simplification), or **TODO** (worth building when an experiment needs it).

The hunt combined a manual staff-level audit with an adversarial logic-hunter
agent pass; findings were cross-validated before fixing.

Fixed in this pass (see `tests/bughunt_test.go` for regressions):

- Liquidation market orders reused the most recently placed order's ID
  (`forceClose` post-increment vs `PlaceOrder` pre-increment). **FIXED**
- Hedge-mode reduce overshoot margined phantom quantity; overshooting
  reduce orders are now rejected at placement (`RejectExceedsPosition`),
  matching venue reduce-only semantics. **FIXED**
- Per-symbol liquidation checks counted the same account cash once per
  symbol; equity and maintenance now aggregate across every margined book
  in the quote asset. **FIXED**
- `ForcedCancelNotification` could be silently dropped on a full channel,
  desynchronizing actors from the book. Now blocking, like fills. **FIXED**
- Derivsim market makers never cancelled stale quotes (order IDs were never
  captured from accepts), stacking zombie liquidity every requote. **FIXED**
- Fee revenue was booked under the quote asset regardless of `Fee.Asset`,
  breaking per-asset conservation for base-denominated fees. **FIXED**
- Market sells ignored base-denominated fee headroom in the funds check.
  **FIXED**
- `MulDiv` overflowed its partial product for user precision configurations
  (quotePrecision ≥ 1e8 at realistic prices); now 128-bit exact. **FIXED**
- Contracts whose underlying never printed settled at price 0 (longs debited
  full entry notional); they now close flat with a logged warning. **FIXED**
- Borrow limits compounded: borrowed cash counted as fresh collateral, and
  negative balances were skipped — effective leverage factor/(1−factor)
  instead of factor. Now limited against net equity. **FIXED**
- `FundingRate` was mutated outside the exchange lock and the live pointer
  was published to actor goroutines; now mutated under lock, snapshots
  published. **FIXED**
- `CancelAllClientOrders` cancelled silently; it now sends
  `ForcedCancelNotification` like liquidation cancels. **FIXED**

## Matching and order book

### Self-trade handling
The matcher skips the incoming client's own resting orders and continues to
deeper levels — no self-fill, and a taker can still trade *through* their own
better-priced quote (paying a wider effective spread). The crossed-book
artifact is gone: a remainder whose price still crosses the client's own
opposite quote now cancels that stale quote before resting (**cancel-maker
STP**, one of the standard venue modes), with a `ForcedCancelNotification` to
the gateway. The book can no longer display bid ≥ ask. **FIXED** (August-2026
execution hunt; regression `TestSelfCrossingLimitDoesNotCrossBook`). Other STP
modes (expire-taker, cancel-both) remain unimplemented — configuration hook if
an experiment needs them.

### Iceberg orders
Icebergs now carry venue refresh semantics: only the displayed tranche
(`DisplayRemaining`) holds time priority; a taker consuming the display fills
the next resting order first, and the exhausted iceberg re-queues at the back
of its level with a fresh tranche (both matchers; aggressive orders rescan the
level so a lone iceberg still fills to completion). **FIXED** (regression
`TestIcebergReserveDoesNotJumpQueue`). Remaining gap: FOK's `canFillFully`
probe still counts hidden depth, so dark liquidity is knowable to FOK
submitters; hidden (fully dark) orders keep full time priority rather than
deranking behind displayed volume. **DOCUMENTED**

### Hidden order market data
Placement and cancellation of `Hidden` orders no longer emit public book
deltas (previously a delta with `VisibleQty: 0` broadcast the exact price
where dark liquidity arrived). Trades against hidden liquidity still print,
as on real venues. **FIXED** (regression `TestHiddenOrderEmitsNoPublicDelta`).

### Market order protection
Market orders walk the entire opposing book with no protection points, price
bands, or max-slippage caps (CME protected market orders; crypto venues
convert to limit at ±N%). Liquidation closes inherit this: a thin book lets a
single liquidation print arbitrarily deep. Real venues band the order and ADL
the remainder. Impact: fire-sale cascades in the sim are *sharper* than
reality; that can be a feature for stress experiments but must be labeled.
**DOCUMENTED**

### Feed integrity
Book deltas and trades carry no per-book sequence numbers for gap detection
and no recovery channel. Requests and non-fill responses are dropped silently
when a gateway channel (10k buffer) is full; fills and forced cancels block.
Actors must treat the request channel as lossy. **DOCUMENTED** — sized so that
drops do not occur at simulation volumes; assert on reject/timeout in actors.

## Margin and liquidation

### Option maintenance is single-quote cross-margin only
Short options post Deribit-style initial margin at fill time and are included
in the mark-refresh maintenance sweep through `OptionMarginParams.MMBps`.
The sweep can liquidate an under-margined option position before expiry.
It remains limited to the contract quote wallet: spot, other quote assets, and
portfolio offsets are not collateral. **DOCUMENTED** — use this for isolated
quote-wallet solvency experiments, not portfolio-margin realism.

### No ADL or socialized loss
Liquidations charge a configured clearance fee into the insurance fund, but
there is no ADL or socialized loss when the fund is exhausted. Impact:
system-wide solvency remains incomplete even though liquidation no longer has
zero fund inflow. **DOCUMENTED**.

### Reduce-only orders still post initial margin
A hedge-mode closing order reserves full order margin even though it only
reduces exposure; venues exempt reduce-only orders from IM. Impact: actors
need spare margin budget to exit positions — deleveraging spirals are slightly
*more* likely than reality. **DOCUMENTED**

### Cross-margin scope
Equity for liquidation = perp-wallet quote balance + aggregated uPnL of
margined books in that quote asset. Spot balances, other quote assets, and
unrealized option premium are not collateral (no multi-asset collateral
haircuts). Additionally, borrowing collateral valuation counts *gross*
balances including amounts already reserved for orders — the same funds can
back an order and a loan simultaneously. **DOCUMENTED**

### Funding mechanics
The funding rate applies to the instantaneous mark at the settlement tick, not
a TWAP of the premium index over the interval (Binance uses time-weighted
premium averaging precisely so the settlement instant cannot be gamed. An
actor could manipulate the mark just before settlement here). Zero-sum is
preserved: per-client truncation dust routes to exchange revenue. Missed
intervals settle once, not prorated. **DOCUMENTED** — use
`ClampedEMAMarkPrice`/`TWAPMarkPrice` per symbol to blunt manipulation.

### Mark price default
`ConfigureAutomation` defaults to `MidPriceCalculator` — the venue's own book
mid — for liquidation triggers. Every real venue anchors marks to an external
index specifically to prevent self-referential liquidation cascades. The
index-anchored calculators exist (`MedianMarkPrice`, `ClampedEMAMarkPrice`,
`TWAPMarkPrice`); the default is only acceptable for single-venue experiments
where the book *is* the market. **CONFIG**

## Settlement

### Settlement TWAP
Dated futures and options settle on an equal-weight mean of 1s-cadence
observations over the window (default 60s; Deribit uses 30min). With no
samples the settlement falls back to the *last price ever observed*, which can
be arbitrarily stale on an illiquid underlying — positions then settle at a
price the market left long ago. No outlier trimming. **DOCUMENTED** — size the
window to the experiment; ensure the underlying trades.

### Expiry bookkeeping
Expiry cancels resting orders with exact ledger releases and notifies
gateways, but market-data subscriptions to the delisted symbol are never
removed and subscribers get no terminal snapshot; actors must GC on the
`settled` announcement (derivsim's `contractSet` does). **DOCUMENTED**

### Isolated margin is half-built
`AllocateCollateralToPosition` moves funds out of `PerpBalances` into
`IsolatedPositions`, but no solvency check reads isolated collateral and there
is no per-position isolated liquidation — allocating collateral makes the
client look *poorer* to the cross-margin sweep and can trigger a false
liquidation. Do not use isolated mode in experiments until it is wired end to
end. **TODO**

## Fees and rounding

All fee and PnL integer math truncates toward zero. Real venues round fees
*up* in their own favor; here the exchange under-collects up to one quote unit
per fill. Conservation holds (dust is simply never charged), but fee-revenue
totals are a lower bound. `MulDiv` is now 128-bit exact for any precision
configuration. **DOCUMENTED**

### Per-execution fee fragmentation
`FixedFee` charges per execution. Spot matching now clones the exact incoming
match, validates all fee cash flows and residual reservations before mutating
the live book, and virtually removes unfundable resting makers while planning.
It commits those forced cancellations only after the incoming order has a
solvent final plan, so a rejected order cannot change unrelated liquidity.
This preserves spot balances for partial, pro-rata, and iceberg executions,
but it deliberately rejects an otherwise solvent prefix when the complete
batch would leave its maker or residual order underfunded.

Margin instruments still reserve only one maker/taker fee headroom for a
resting order, while their settlement/margin lifecycle needs a separate
ledger-aware execution plan. Do not use `FixedFee` on perp/dated/option
experiments until that path is hardened. **TODO**

### Interest accrual floor
Collateral interest truncates to zero each per-minute charge for small debts
(below ≈ $105 at the default 500 bps annual rate with `USD_PRECISION = 1e5`)
and the fractional remainder is dropped rather than accrued — small debts pay
no interest at all. Real venues accumulate fractional accruals. **DOCUMENTED**

## Ecology / experiment design

- Two colluding clients can paint the tape freely (self-trade prevention is
  per-client only); volume and last-price metrics are trust-free. Use
  mark-price calculators robust to it.
- `CashCarryArb` updates its position counter at submit, not at fill, and its
  legs are independent market orders (leg risk, no atomicity). With generous
  funding and zero rejects this holds, but the counter silently drifts if any
  leg is ever rejected. Gen-9 basis experiments should move to passive limit
  legs per the EXPERIMENTS.md diagnosis.
- Futures fair value carries no interest rate (zero-rate Black-76, carry-free
  quoting): basis levels are flow artifacts by construction and must not be
  compared to real term structure.
