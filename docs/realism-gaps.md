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
deeper levels. Consequences: (a) no self-fill — correct; (b) a taker can trade
*through* their own better-priced order, executing at worse prices while their
own quote stays live — no real venue does this; production STP modes are
cancel-taker, cancel-maker, or cancel-both (Binance STP, CME self-match
prevention). Impact: actors quoting both sides pay a slightly wider effective
spread; volume is not inflated. A sharper artifact: the skipped remainder can
REST crossed with the client's own opposite quote, so the public book shows
bid ≥ ask until a third party trades through it — this distorts every
mid-based mark calculator while it lasts. **DOCUMENTED** — an STP-mode config
on the matcher is the natural extension point if an experiment studies MM
internalization.

### Iceberg orders
Hidden quantity keeps full price-time priority and never "refreshes": real
venues re-enter the displayed clip at the back of the queue on each refill
(CME, Nasdaq) and often derank hidden volume behind displayed volume at the
same price. Here an iceberg is simply an order whose display quantity is
cosmetic for market data — the full hidden size fills in a single execution.
FOK's `canFillFully` probe also counts hidden depth, so dark liquidity is
fully knowable to FOK submitters. Impact: iceberg users get unrealistically
good queue position; fill-probability studies involving icebergs are
optimistic. **DOCUMENTED**

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

### Options have no maintenance sweep
Short options post Deribit-style initial margin at fill time
(`max(IMBase − OTM%, IMFloor) × S + mark`), but there is no mark-to-market
re-margining and no option liquidation: `OptionMarginParams.MMBps` is reserved
and unused. A short call whose underlying doubles stays margined at the stale
IM until expiry; the deficit lands on the insurance fund at settlement.
Deribit runs a continuous PM/MM sweep and liquidates option books. Impact: sim
option sellers can run economically bankrupt positions to expiry — fine for
flow experiments, wrong for solvency experiments. **TODO** (MM sweep +
liquidation using the existing `Expirable` mark updates).

### No liquidation penalty / insurance-fund inflow
Liquidations charge no fee; the insurance fund only ever pays (absorbing
bankruptcy deficits) and can go unboundedly negative — there is no ADL and no
socialized loss. Real venues charge the liquidated account a penalty (roughly
the maintenance margin) into the fund and trigger ADL when the fund is
exhausted. Impact: system-wide solvency metrics are one-sided; the fund
balance is a loss counter, not a buffer model. **DOCUMENTED** (fund inflow is
a small change if needed; ADL is a project).

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

### FixedFee on multi-fill orders
`FixedFee` charges its amount per EXECUTION, but spot reservations add fee
headroom for one execution per order: an order filling in N partial executions
pays (N−1) extra fees beyond what the placement check reserved and can drive
the balance negative. The reservation cannot know N in advance without
absurd worst-case over-locking (N ≤ qty). Use `PercentageFee` for economic
sims; treat `FixedFee` as a test fixture or keep amounts negligible.
**DOCUMENTED** (same applies to `marketBuyCost`, which adds headroom once per
price level while fees are charged per maker execution).

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
