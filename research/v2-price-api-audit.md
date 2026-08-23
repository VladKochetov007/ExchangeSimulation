# V2 price API audit

Status: **completed V2 migration, pending the V2 freeze.** This work is not
part of frozen `ae13f9a`. It changes V2 error, lifecycle, and price-reference
semantics deliberately; no scheduler, RNG, matching priority, actor-visible
feed, or concurrency mechanism was changed solely for this migration.

## Invariant

No usable market/reference price is represented as numeric `0`.

Every positive-price API now returns `(int64, error)` or `(int64, available)`.
The numeric member of a failed result may be Go's conventional zero, but it is
not usable without a nil error/true availability bit. APIs whose output can
legitimately equal zero (option premium, rounded fee, PnL, a computed
liquidation estimate) carry a distinct availability/error channel.

## Completed repository-wide call graph

| API | Consumers | Required price contract | Pre-V2 absence | Completed V2 behavior |
| --- | --- | --- | --- | --- |
| `OrderBook.GetMidPrice` | `DefaultExchange.MidPrice`, `MidPriceOracle`, generic/anchored mark calculators | true two-sided midpoint | last trade or `0` | requires positive, uncrossed bid and ask; returns `ErrNoPrice`; uses `bid + (ask-bid)/2` |
| `OrderBook.GetBestBid` / `GetBestAsk` | executable-price and named one-sided-reference paths | executable side | `0` | `(price,error)`; no side is unavailable |
| `OrderBook.GetLastPrice` / `LastPriceCalculator` | only explicitly last-trade-based marks | last trade | `0` | `(price,error)`; no trade is unavailable |
| `DefaultExchange.MidPrice` | `MidPriceOracle`, venue/index reference sources | true two-sided midpoint | `0` | wraps `ErrNoBookPrice`; never substitutes last trade |
| `TwoSidedMidPrice` | V2 index observer, valuation-mark cache, research metrics | true midpoint | `(0,false)` | retained as an explicitly named in-process availability tuple; no caller passes its zero to an order/valuation API |
| `liveBookReferencePrice` (replaces `marketRefPrice`) | display marks, initial perp risk/cross-margin fallback | named one-sided book reference | mid-or-last then one-sided then `0` | true midpoint, then sole ask, then sole bid; no last trade; otherwise wrapped error |
| `PriceSource.Price` | configured index, borrowing, option fee, basket, simulation index/degraded index | named external/index source | `0` | `(price,error)` everywhere; unknown symbol/source/non-positive result wraps `ErrNoPrice` |
| `MidPriceOracle` / `StaticPriceOracle` / `BasketIndex` | mark calculators and configured indices | strict book/static/weighted source | `0`, source dropping | explicit source/quorum error; baskets retain only valid contributors and fail below quorum |
| mark calculators | perp/future mark, margin, liquidation, funding | calculator-specific positive mark | mixed `0`/implicit last trade | `(mark,error)`; last-trade protection remains only in named `BinanceMedianMarkPrice` policy |
| `FundingRate` mark/index fields | funding, cross-margin, risk mark, MD subscribers | current shared mark/index | `0` treated as unavailable or stale state retained | `MarkAvailable` and `IndexAvailable` accompany fields; failed mark/index clears availability and funding defers |
| `SettlementPrice` / settlement observer | expiry, exercise, delivery fee | declared underlying observations | zero or cached option mark fallback | `(price,error)`; window mean or explicitly named last *declared-reference observation* only; no last trade, cached option mark, or zero fallback |
| `EstimateLiquidationPrice` | margin-call handler/tests | computed risk estimate, not a market mark | invalid input returned `0` | `(price,error)`; a computed zero remains valid; absent input is an error |
| account/display snapshots | gateway/client response, research telemetry | display mark/estimate | absent mark appeared as `0` | `PositionSnapshot.MarkPrice` and `LiquidationPrice` are pointers; mark reason is recorded |
| local actor/cache midpoints | Stoikov, fixed-distance/imbalance makers, supplier, metaorder, carry, microstructure | local two-sided reference | manual `(bid+ask)/2`, zero guards | common overflow-safe `twoSidedMidpoint` returns `(price,available)`; actors defer on false |

## Boundary decisions and evidence

| Boundary | No-price outcome | Evidence/response |
| --- | --- | --- |
| Client order with configured fee source unavailable | reject before order ID, reserve, borrow, book, or match mutation | `RejectPriceUnavailable` plus `price_unavailable: order_fee_admission` |
| Client order with non-zero collateral/debt lacking a configured price | reject before borrowing/reservation/matching | wrapped `ErrNoBookPrice`, `RejectPriceUnavailable` |
| Market order without executable depth | reject; executable bid/ask is required | normal insufficient-liquidity/funds path, never a midpoint substitute |
| Option listing | defer until strict underlying midpoint exists | `price_unavailable: listing` |
| Derivative mark/index calculation | defer all mark/funding/margin/liquidation work for that symbol | `price_unavailable: derivative_mark`, `perp_index`, or `perp_mark` |
| Scheduled/manual funding lacking a current shared mark | preserve due timestamp, do not debit/credit/advance schedule | error to manual caller; `price_unavailable: funding_settlement` for periodic path |
| Cross-margin/risk/strict marked account | fail closed; no entry-price/zero substitute | wrapped `ErrNoBookPrice` to caller; automation records operation-specific diagnostic |
| Ordinary account display | return state but omit unavailable mark | nil `MarkPrice` and `MarkUnavailableReason` |
| Actor/arbitrage local decision | defer according to the actor's declared policy | no order request is generated from an unavailable local mid/touch |

## Explicit expiry lifecycle

The default terminal-unavailable policy is `RETRY_FOREVER`; it is intentionally
not an automatic price fallback.

```text
ACTIVE
  -> EXPIRY_REACHED (new orders permanently rejected)
  -> SETTLEMENT_PENDING (resting orders cancelled; positions/collateral held)
  -> SETTLED (one declared-reference settlement; positions close; delist)
```

`SETTLEMENT_PENDING` retries only the declared reference at ordinary expiry
checks. It records `expiry_settlement_pending` with attempt count, policy, and
reason, plus `price_unavailable: expiry_settlement`. While pending, derivative
marks, liquidation, and funding work are skipped. There is no reopening, no
post-expiry fill, no zero/last-trade settlement, and no double release. If the
source never becomes available the contract remains halted and pending; a
future terminal fallback must be added as a separately named instrument policy.

The only ordinary observer fallback is explicitly defined: when a rolling
settlement window is empty, it may use the last observation previously received
through `ObserveSettlement` from that same declared reference. It is neither a
book last trade nor an option mark cache. Options no longer settle from a
cached `SetMarks` value without an observed settlement reference.

## Behavior intentionally changed in V2

1. Empty/one-sided generic midpoint calls no longer get a last trade.
2. Automatic option listing requires a true two-sided underlying midpoint.
3. An unavailable current external/index/mark source invalidates current
   funding-mark availability rather than retaining a stale usable mark.
4. Funding no longer falls back to each position's entry price. It uses one
   explicit shared mark or defers.
5. Expired contracts with no declared settlement observation halt pending
   rather than being forced flat at zero/cached mark. This changes lifecycle
   behavior and is covered by deterministic recovery/retry tests.
6. A configured underlying-notional option fee source never silently changes
   to the premium schedule on source failure or rounded zero; premium pricing
   is used only when that schedule was selected by configuration.
7. Price-dependent client admission now fails before any state mutation.

## Regression coverage

- Table-driven strict midpoint tests cover missing, empty, one-sided, crossed,
  equal, odd-spread, ordinary, and near-`MaxInt64` prices.
- `TestExpirySettlementPendingRetriesThenSettlesExactlyOnce` covers multiple
  unavailable retries, halted trading/resting-order cancellation, later source
  recovery, exactly one settlement, closed positions, and conservation.
- `TestFundingMissingMarkDefersWithoutEntryPriceFallback` proves both manual
  error propagation and periodic observable deferral without schedule advance.
- `TestFeePriceUnavailableRejectsBeforeMutation` and
  `TestAutoBorrowPriceUnavailableRejectsBeforeMutation` prove client preflight
  fails before IDs, balances, reserves, loans, or books change.
- `TestOptionFeeUsesOnlyItsDeclaredPriceSchedule` distinguishes a legitimate
  rounded zero fee from an unavailable external reference.
- `TestTwoSidedMidpointIsOverflowSafeAndExplicit` covers participant-side
  cache arithmetic; fresh-process V2 information-boundary determinism tests
  continue to guard instrumentation neutrality.
- `TestIndexPriceLockedUsesExplicitLockHeldOraclePath` guards the exchange
  lock-held index lookup; the focused fee-simulation conservation case now
  completes instead of deadlocking behind a queued writer.

## Legitimate remaining zero-valued fields

These are quantities, not availability sentinels:

- balances, debt, PnL, fees, funding rate, quantities, open interest, and
  delivery/exercise cash flow;
- option `MarkPremium` / `PositionMark`: zero is a valid OTM rounded premium;
  `UnderlyingMark > 0` identifies an initialized option mark pair;
- `FundingRate.Rate == 0`: valid zero funding. `MarkPrice` and `IndexPrice`
  may physically be zero only with their corresponding `Available == false`;
  no consumer may infer availability from those numeric fields;
- a computed liquidation estimate may equal zero; invalid inputs now return an
  error and account snapshots use a nil pointer when no estimate exists;
- an order request for a `Market` order legitimately has protocol
  `Price == 0`; order type distinguishes it from an unavailable market or
  reference price, and limit requests remain strictly positive;
- a metaorder report has `VWAP == nil` when no child filled; a positive pointer
  is an exact weighted execution price, so an unavailable VWAP is never
  serialized as numeric zero;
- `SettlementPrice`/`PriceSource`/book helpers may return numeric zero only
  together with a non-nil error, and `(0,false)` cache helpers only as explicit
  availability tuples.

## Audit note

While making `UpdateDerivativeMarks` emit unavailable-price diagnostics, the
audit found its expirable-instrument map iteration would make the diagnostics
nondeterministic. The sweep is now symbol-sorted. Fresh-process execution-hash
and V2 evidence-neutrality tests pass after that correction. This was an
instrumentation ordering defect, not an economic mechanism change.

The migration also exposed a latent lock-order defect: the lock-safe
`MidPriceOracle` could re-enter `DefaultExchange.mu` from an already locked
index calculation and deadlock when a writer queued. The explicit
`PriceWithProviderLockHeld` / `MidPriceLocked` contract removes that re-entry;
the public oracle path remains lock-safe.
