# Microstructure: What Is Missing

**Date**: 2026-02-17
**Scope**: Exchange core is correct. These gaps are on the actor/simulation layer.

---

## Exchange Core — Status

The exchange engine is financially correct:

| Mechanic | Status |
|----------|--------|
| Funding settlement (8-hour, zero-sum) | ✅ Correct |
| Spot margin interest (APR × dt / seconds_year) | ✅ Correct |
| Liquidation (initMargin overflow-safe, insurance fund) | ✅ Correct |
| Precision (BasePrecision for PnL, TickSize only for price validation) | ✅ Correct |
| Order book matching (price → time → iceberg visibility priority) | ✅ Correct |
| Market data (trades, L2 deltas, snapshots, funding, OI) | ✅ Correct |
| Money conservation (all operations zero-sum, covered by conservation_test.go) | ✅ Correct |

---

## Gap 1 — No Informed Trader Model (Critical)

**What's missing**: All actors are uninformed. No actor has a private signal that other actors don't have.

Real microstructure has two trader types:
- **Informed**: trade on private information; their order flow predicts future price moves
- **Noise / liquidity**: trade for exogenous reasons; their order flow carries no predictive signal

Without this split, the simulation cannot produce:
- Adverse selection costs for market makers
- Price discovery via order flow
- Bid-ask spread as compensation for adverse selection (Glosten-Milgrom, Kyle)
- Realistic trade-sign autocorrelation

**What to build**:
- `InformedTrader` actor: receives a latent `TruePriceProvider` signal, trades toward it
- `PrivateSignalOracle` interface: returns a signed quantity pressure to a subset of actors
- Signal decay model: price impact dissipates over time toward fundamental

---

## Gap 2 — Market Makers Don't React to Order Flow Toxicity

**What's missing**: `AvellanedaStoikov` adjusts quotes for inventory skew, but not for adverse selection. The spread is fixed or inventory-driven only.

Real MMs widen spread when order flow is toxic:
- High order flow imbalance → widen
- Large trade arriving into thin book → widen
- Detect informed flow via trade-sign autocorrelation or VPIN

**What to build**:
- `ToxicitySignal`: rolling order flow imbalance tracker (OFI = bid volume − ask volume over window)
- `SpreadModel` interface: replaces hardcoded spread in AS and PureMMConfig
- Two implementations: `InventorySpread` (current), `ToxicityAdjustedSpread` (OFI-weighted)

---

## Gap 3 — No Permanent vs Temporary Price Impact

**What's missing**: Orders execute at book prices. Large orders move through price levels by consuming liquidity, but after execution the book simply has a hole — no recovery model, no transient impact.

Real markets have:
- **Temporary impact**: price overshoots on large order, then mean-reverts (bid-ask bounce)
- **Permanent impact**: informed orders shift the fair value permanently
- **Market depth replenishment**: MMs refill levels after being hit

The exchange book is technically correct (orders do consume depth across levels). What is missing is:
- Any `IndexPriceProvider` that tracks true fundamental value separately from mark price
- `SimpleFundingCalc` premium-based rate anchors to index, but index is static unless wired up by the user

**What to build**:
- `FundamentalValueProcess`: GBM or mean-reverting process that evolves independently
- `InformedIndexProvider`: returns the fundamental + noise as the index price
- MMs quoting against the fundamental recover faster than against the mark

---

## Gap 4 — Latency Models: Wrong Distributions and No Temporal Correlation

**What exists**: `ConstantLatency`, `UniformRandomLatency`, `NormalLatency`, `LoadScaledLatency` — all i.i.d., each `Delay()` call independent of the previous.

**Two distinct problems:**

### 4a — Wrong Distributions

| Model | Problem |
|-------|---------|
| `UniformLatency` | Flat between [min, max]. No tail, no mode. Unrealistic. |
| `NormalLatency` | Gaussian is symmetric; real RTT is right-skewed. Negative values clamped to 0 creates impossible spike at zero. |
| `LoadScaledLatency` | Deterministic given fixed load. No randomness — not a distribution. |

Real network RTT is empirically **log-normal**: `log(L) ~ N(μ, σ²)`. Strictly positive support. Heavy right tail captures retransmit spikes, GC pauses, NIC interrupt coalescing. What's missing: `LogNormalLatency`.

### 4b — No Markov Property (No Temporal Autocorrelation)

Every `Delay()` call is i.i.d. — statistically independent of the previous value. Real latency has **persistent congestion states**: if a packet takes 50ms extra due to a GC pause or kernel scheduling stall, the next several packets are also elevated because the same cause is still active.

Real latency has regime-switching behavior:
- **NORMAL**: low stable latency, ~95% of time
- **CONGESTED**: moderately elevated, triggered by burst traffic
- **SPIKE**: extreme outlier (retransmit, GC stop-the-world), rare but impactful

Each regime is persistent (Markov chain), not memoryless. This matters for:
- Latency arbitrage realism — HFT strategies that depend on winning races
- HFT vs retail asymmetry — co-location advantage is regime-correlated, not just mean-shifted

**What to build**:
- `LogNormalLatency`: `L = exp(rng.NormFloat64() × logSigma + logMu)`, no floor needed
- `MarkovLatencyModel`: state machine with `transitionMatrix [nStates][nStates]float64`; each state has its own `LatencyProvider` (e.g. `LogNormal` for each state with different params); `Delay()` advances the Markov chain then draws from the current state's distribution
- `LoadScaledLatency` is correct as a deterministic component — combine with `LogNormalLatency` as the noise term rather than using it standalone

---

## Gap 5 — No Pro-Rata Matching Option

**What exists**: Strict price-time (FIFO) matching only.

**What's missing**: Many futures exchanges (CME, Euronext) use pro-rata at the best price — all resting orders at the best level share fills proportionally to their size, not by arrival time.

This changes MM strategy fundamentally:
- FIFO: value is in being first in queue (speed matters)
- Pro-rata: value is in posting large size (queue position irrelevant)

**What to build**:
- `MatchingAlgorithm` interface injected into `Matcher`
- `ProRataMatcher`: distribute fill proportionally at each price level
- `FIFOMatcher` (current behavior): existing `Match()` implementation

This is a library-level change but is clean: `NewExchange` already has a `Matcher` field — the interface just needs to be wider.

---

## Gap 6 — No Circuit Breakers / Reference Price Bands

**What's missing**: A flash crash in the simulation immediately liquidates every position in sequence. No halt, no reference price, no cooling-off.

Real exchanges implement:
- Price band: reject orders more than X% from last traded price
- Trading halt: pause matching when price moves more than Y% in Z seconds
- Limit-up/limit-down: daily price limits (futures)

**What to build**:
- `CircuitBreaker` interface: `ShouldHalt(symbol string, lastPrice, newPrice int64) bool`
- Plugged into the matching loop before executing each fill
- `PercentBandCircuitBreaker`: simplest implementation

---

## Gap 7 — Funding Rate Uses Static Mark/Index Gap, No Volume-Weighted Mark Price

**What exists**: `SimpleFundingCalc` computes rate from `(markPrice - indexPrice) / indexPrice`. Mark price comes from `MarkPriceCalculator` (currently mid or weighted mid of book).

**What's missing**:
- Real mark price is the EMA of recent trades, not instantaneous book mid
- Manipulation resistance: real exchanges use median-of-three or time-weighted average
- Index price is a basket of spot prices from multiple external venues — here it requires a user-supplied `IndexPriceProvider`

**What to build**:
- `EMAMarkPriceCalculator`: EMA of trades over a configurable window
- `MedianMarkPriceCalculator`: median of three: bid, ask, and last trade
- These implement the existing `MarkPriceCalculator` interface — no library changes needed

---

## Summary Table

| Gap | Affects | Effort |
|-----|---------|--------|
| No informed traders | Price discovery, adverse selection, spread decomposition | Medium |
| MM toxicity-blind | Spread realism, MM P&L realism | Small |
| No fundamental value process | Impact permanence, index anchoring | Small |
| Latency not load-dependent | HFT latency race realism | Small |
| No pro-rata matching | CME/futures venue realism | Medium |
| No circuit breakers | Flash crash behavior | Small |
| Mark price not EMA/median | Funding rate manipulation resistance | Small |

All gaps are **actor/simulation layer only**. The exchange core (matching, margin, funding, liquidation) is correct and does not need to change. All can be addressed by implementing new actors or injecting new interface implementations without modifying library code.
