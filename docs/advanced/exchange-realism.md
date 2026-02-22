# Exchange Simulation — Realism Review

**Date:** 2026-02-22
**Scope:** Full review of docs, exchange core, actor system, and simulation layer against real exchange behavior.

---

## Executive Summary

The exchange core is **financially correct**. Matching, margin, funding settlement, liquidation, and money conservation all implement production exchange mechanics accurately and are covered by conservation tests.

The actor/simulation layer is **mostly production-grade** with 17 implemented strategies, but has six documented gaps that matter for specific use cases.

Multi-exchange trading is **fully supported** via `MultiVenueGateway` — actors can trade across venues simultaneously. This is not documented anywhere.

The docs have three missing files (linked but absent) and one visible arithmetic scratchpad in `funding-rates.md` that needs cleaning.

---

## 1. Exchange Core

### 1.1 Order Matching

**Implementation:** `exchange/matching.go`, `exchange/book.go`

Priority order: price → visibility (normal > iceberg > hidden) → time (FIFO). Implemented via doubly-linked list per price level for O(1) FIFO access and O(1) best-bid/ask.

Self-trade prevention: orders from the same `clientID` never match — correct per production exchange behavior.

Both `DefaultMatcher` (price-time-visibility) and `ProRataMatcher` (CME-style proportional fill at best level) are implemented. All match behavior is injectable via the `MatchingEngine` interface.

**Verdict:** Correct. Matches production limit order book behavior.

**Doc issue:** `MICROSTRUCTURE_MISSING.md` lists "no pro-rata matching" as a gap but `ProRataMatcher` already exists in code. That gap entry should be removed.

---

### 1.2 Funding Rates (Perpetuals)

**Implementation:** `exchange/funding.go`

Formula:
```
premium    = (markPrice - indexPrice) × 10000 / indexPrice   [bps]
fundingRate = baseRate + (premium × damping / 100)
fundingRate ∈ [-maxRate, maxRate]
payment    = absSize × entryPrice / basePrecision × rate / 10000
```

Settlement in `PositionManager.SettleFunding()`: longs pay when rate > 0, shorts receive. Zero-sum within each settlement; net imbalance goes to exchange fee revenue.

Pluggable: `FundingCalculator` interface, `SimpleFundingCalc`, `ZeroFundingCalc`. Mark price pluggable via `MarkPriceCalculator` (mid, last, weighted-mid, Binance median-of-three).

Typical production parameters (Binance/Bybit style):
- 8-hour intervals, ±0.75% cap
- Damping 50–100%
- Arbitrageurs enforce convergence

**Verdict:** Correct. Formula and settlement match production perpetual futures mechanics.

**Doc issue:** `docs/core-concepts/funding-rates.md` contains a visible arithmetic scratchpad mid-document ("let me recalculate" paragraphs with contradictory intermediate results). The final answer is correct but the derivation looks broken. Clean this.

---

### 1.3 Positions, Margin, PnL

**Implementation:** `exchange/types.go`, `exchange/funding.go` (PositionManager)

Position tracking: weighted average entry price on extends, realized PnL on reductions and flips. Correct.

Margin modes:
- **Cross:** all positions share the account's full balance. `AvailableMargin = Balance + ΣUnrealizedPnL - ΣUsedMargin`
- **Isolated:** per-position dedicated collateral via `MarginModeManager.AllocateCollateralToPosition()`

Liquidation: triggered when `(balance + unrealizedPnL) < maintenanceMargin`. Executes at market, collects penalty to insurance fund, handles auto-deleveraging.

Overflow-safe: all PnL calculations divide by `BasePrecision` before multiplying (avoids int64 overflow on large positions).

**Verdict:** Correct. Matches Binance/Bybit perpetual margin mechanics.

---

### 1.4 Borrows / Repays

**Implementation:** `exchange/borrowing.go`

`BorrowingManager` supports:
- Manual: `BorrowMargin(clientID, asset, amount, reason)`, `RepayMargin(clientID, asset, amount)`
- Auto-triggered at order placement: `AutoBorrowForSpotTrade()`, `AutoBorrowForPerpTrade()`
- Collateral validation via `CollateralPriceOracle` interface
- Per-asset rates (`BorrowRates map[string]int64`) and limits (`MaxBorrowPerAsset`)
- Full event logging: `BorrowEvent`, `RepayEvent` (with interestRate, collateralUsed, remainingDebt)

**Verdict:** Implemented and functional. Matches spot margin lending mechanics on centralized exchanges.

**Doc gap:** `borrowing.md` and `borrowing-and-leverage.md` are linked from `balance-snapshots.md` and `positions-and-margin.md` but **do not exist**. The borrowing system has zero user-facing documentation.

---

### 1.5 Snapshotting

**Implementation:** `exchange/exchange.go`, `exchange/balance_logger.go`

Two types:
1. **Market data snapshots** (`EnablePeriodicSnapshots`): top-20 L2 book at fixed intervals. Uses `TickerFactory` — simulation-time aware. Correct.
2. **Balance snapshots** (`EnableBalanceSnapshots`): spot/perp/borrowed per client. Uses `time.NewTicker` — **wall-clock only**.

Balance snapshot contents: `SpotBalances`, `PerpBalances`, `Borrowed` (map), all in precision units. Zero balances omitted. Point-in-time consistent.

**Bug:** Balance snapshots are not simulation-time aware. In accelerated simulation (e.g., 1 hour of trading in 10 seconds wall time), balance snapshots will fire once while the simulation runs through thousands of funding intervals and thousands of trades. The snapshots will not capture any intermediate state. This should use `TickerFactory` like market data snapshots do.

**Doc coverage:** `core-concepts/balance-snapshots.md` — accurate and production-quality. Does not mention the sim-time desync issue.

---

### 1.6 Money Conservation

`conservation_test.go` verifies that all operations are zero-sum: no money created or destroyed across trading, funding settlement, fees, and liquidations. This is production-grade correctness.

---

## 2. Actor System

### 2.1 Single-Exchange Actors

`BaseActor` wraps one `ClientGateway`. Three-channel event loop: responses, market data, custom events (timers, signals). Single-threaded per actor — no race conditions if `OnEvent` doesn't spawn goroutines.

`CompositeActor` + `SubActor`: N strategies share one gateway/balance pool. Symbol routing: each sub-actor only sees events for its declared symbols. `SharedContext` tracks shared balance state across sub-actors.

This correctly models a firm running multiple strategies from one account: the strategies see each other's capital but not each other's order flow.

17 actor implementations in `realistic_sim/actors/`: `PureMarketMaker`, `AvellanedaStoikov`, `InformedTrader`, `FundingArbitrage`, `TriangleArbitrage`, `MomentumTrader`, `NoisyTrader`, `RandomizedTaker`, `CrossSectionalMR`, and others.

---

### 2.2 Multi-Exchange Trading

**Answer: Yes, actors can trade multiple exchanges simultaneously.**

`MultiVenueGateway` (`simulation/venue.go`):

```go
type MultiVenueGateway struct {
    clientID     uint64
    gateways     map[VenueID]*exchange.ClientGateway
    responseCh   chan VenueResponse     // tagged by VenueID
    marketDataCh chan VenueMarketData   // tagged by VenueID
}
```

One `ClientGateway` per venue. All responses and market data tagged with `VenueID`. Methods: `SubmitOrder(venue, req)`, `Subscribe(venue, symbol)`, `GetGateway(venue)`.

`LatencyArbitrageActor` (`simulation/latency_arbitrage.go`) demonstrates live cross-venue trading: monitors the same symbol on a fast and slow venue, exploits price discrepancies.

`MultiExchangeRunner` (`simulation/multi_runner.go`) orchestrates multiple venues under a shared simulated clock, with per-venue actor/balance configuration.

**Limitation:** The `Actor` interface defines `Gateway() *exchange.ClientGateway`. Actors using `MultiVenueGateway` cannot implement this interface method meaningfully — they'd have to return nil or pick one venue arbitrarily. Multi-venue actors must be written outside the standard `Actor` interface.

**Doc gap:** `MultiVenueGateway`, `VenueRegistry`, `MultiExchangeRunner`, and the latency arbitrage example are completely absent from the documentation. This is the largest documentation omission in the project.

---

## 3. Realistic Simulation Readiness

### What works for realistic simulation

| Capability | Implementation |
|------------|---------------|
| Informed vs noise trader split | `InformedTrader` (Kyle/Glosten-Milgrom model) + `NoisyTrader` |
| Price discovery | `GBMProcess` as fundamental value, `InformedTrader` trades toward it |
| Funding arbitrage convergence | `InternalFundingArb`, `FundingArbitrage` — delta-neutral cash-and-carry |
| Market making with inventory skew | `PureMarketMaker`, `AvellanedaStoikov` |
| Latency modeling | `DelayedGateway` with `ConstantLatency`, `UniformRandomLatency`, `NormalLatency`, `LoadScaledLatency` |
| Cross-venue arbitrage | `LatencyArbitrageActor`, `TriangleArbitrage` |
| Multi-venue market structure | `MultiVenueGateway`, `MultiExchangeRunner`, shared simulated clock |
| Observable output | NDJSON event log: trades, fills, funding, OI, balance changes, snapshots |

### Known gaps (actor/simulation layer only)

All of the following can be addressed by implementing new types that satisfy existing interfaces — no changes to exchange core needed.

**Gap 1 — MM spread does not react to order flow toxicity**

Market makers widen spreads when they detect informed flow (via OFI = order flow imbalance, or VPIN). Current `AvellanedaStoikov` and `PureMarketMaker` adjust only for inventory skew. Under heavy informed flow, spreads are unrealistically tight and MM P&L is unrealistically negative.

Fix: `ToxicityAdjustedSpread` implementing a `SpreadModel` interface, using rolling OFI over a configurable window.

**Gap 2 — No permanent vs temporary price impact separation**

Large orders consume book depth (correct), but after execution there is no model for transient mean-reversion (bid-ask bounce) vs permanent information-driven drift. The index price is static unless the user wires up a `GBMProcess` or similar.

Fix: `FundamentalValueProcess` as index, market makers quote against fundamental; temporary impact dissipates as MMs refill.

**Gap 3 — Latency distributions are i.i.d. with wrong shapes**

`UniformLatency` has no tail. `NormalLatency` is symmetric and can produce negative values (clamped to zero, creating a spike). Real network RTT is log-normal: right-skewed, strictly positive.

Additionally, all latency models are i.i.d. — each call independent. Real latency has persistent congestion states (Markov regime: normal → congested → spike). This matters for HFT race simulations.

Fix: `LogNormalLatency`, `MarkovLatencyModel` with per-regime `LatencyProvider`.

**Gap 4 — Mark price not EMA or median-of-three**

`SimpleFundingCalc` computes rate from instantaneous mark price (book mid or last trade). Real exchanges use EMA of recent trades or `median(bid, ask, lastTrade)` to resist manipulation.

Fix: `EMAMarkPriceCalculator`, `MedianMarkPriceCalculator` — both satisfy the existing `MarkPriceCalculator` interface.

---

## 4. Issue Tracker

| # | Severity | File | Issue |
|---|----------|------|-------|
| 1 | **Medium** | `docs/core-concepts/funding-rates.md` | Visible arithmetic scratchpad mid-doc ("let me recalculate" with contradictory intermediate steps). Final answer correct, derivation looks broken. |
| 2 | **Medium** | `docs/` | `borrowing.md` and `advanced/borrowing-and-leverage.md` linked but do not exist. Borrowing system fully implemented but zero user docs. |
| 3 | **Medium** | `docs/` | `MultiVenueGateway`, `VenueRegistry`, `MultiExchangeRunner`, multi-exchange runner pattern completely absent from docs. Largest single doc omission. |
| 4 | **Low** | `docs/architecture/MICROSTRUCTURE_MISSING.md` | Gap 5 ("no pro-rata matching") and Gap 6 ("no circuit breakers") both already implemented. Entries removed. |
| 5 | **Low** | `exchange/exchange.go` | `EnableBalanceSnapshots` uses `time.NewTicker` (wall-clock), not `TickerFactory`. Balance snapshots desynced from simulation time in accelerated runs. |
| 6 | **Low** | `docs/actors/actor-system.md` | `Actor` interface `Gateway() *exchange.ClientGateway` makes multi-venue actors second-class — they can't implement the interface without returning a misleading value. |

---

## 5. Recommendations

**Immediate (docs only):**
1. Clean `funding-rates.md` — remove the visible scratchpad, leave only the clean final calculation.
2. Write `docs/core-concepts/borrowing.md` and `docs/advanced/borrowing-and-leverage.md` to cover `BorrowingManager` API.
3. Write `docs/simulation/multi-venue.md` covering `VenueRegistry`, `MultiVenueGateway`, `MultiExchangeRunner`, and the latency arbitrage pattern.
4. Update `MICROSTRUCTURE_MISSING.md`: remove pro-rata gap (implemented), add balance snapshot sim-time desync.

**Short-term (simulation quality):**
5. Fix `EnableBalanceSnapshots` to accept a `TickerFactory` parameter, consistent with market data snapshots.
6. Implement `EMAMarkPriceCalculator` and `MedianMarkPriceCalculator` — small effort, directly improves funding rate realism.
7. Implement `LogNormalLatency` — log-normal is the empirically correct RTT distribution.

**Medium-term (microstructure completeness):**
8. `ToxicityAdjustedSpread` with OFI signal — required for realistic MM P&L decomposition.
9. `MarkovLatencyModel` — required for realistic HFT race and co-location advantage modeling.
10. `CircuitBreaker` interface + `PercentBandCircuitBreaker` — required for flash crash scenarios.

**Interface design:**
11. Consider replacing `Actor.Gateway() *exchange.ClientGateway` with a broader abstraction (e.g., `Gateway() OrderRouter` where both `ClientGateway` and `MultiVenueGateway` implement `OrderRouter`). This would allow multi-venue actors to participate in the same framework as single-venue actors.
