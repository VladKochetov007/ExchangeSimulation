# Mark Price Models

**Date**: 2026-02-18
**Package**: `exchange`
**Interface**: `MarkPriceCalculator`

---

## Why Mark Price Matters

Mark price drives two critical exchange mechanics:

1. **Unrealised PnL** — determines whether a position is profitable before closing
2. **Liquidation trigger** — when mark price crosses the liquidation threshold, the position is force-closed

An exchange that uses its own order book's last trade price or mid price as mark is **trivially manipulable**: a wash trader can move the mark price to trigger competitor liquidations with near-zero cost. All production perpetual futures exchanges anchor the mark price to an **external index** that cannot be moved by trading the perp book alone.

---

## Architecture

```
MarkPriceCalculator interface
    Calculate(book *OrderBook) int64

IndexPriceProvider interface
    GetIndexPrice(symbol string, timestamp int64) int64
```

All manipulation-resistant models take an `IndexPriceProvider`. In production this is a weighted median of spot prices from multiple external venues (Binance Spot, Coinbase, Kraken, etc.). In simulation it can be wired to a `GBMProcess`, which implements `IndexPriceProvider`.

An attacker moving the perp book cannot move the index. Manipulation requires controlling two independent price sources simultaneously.

---

## Models

### Non-Anchored (Book-Only)

These do not require an index. Use only for spot markets or where manipulation resistance is not required.

#### `LastPriceCalculator`
```
mark = last_trade_price
```
Simplest possible. Trivially manipulated by wash trading a single lot at any price.

#### `MidPriceCalculator`
```
mark = (best_bid + best_ask) / 2
```
Better than last trade. Still fully manipulable by placing a single spoofed order on one side.

#### `WeightedMidPriceCalculator`
```
mark = (bid_price × ask_qty + ask_price × bid_qty) / (bid_qty + ask_qty)
```
Quantity-weighted mid. The thicker side pulls mark toward it. Reduces impact of thin spoofed orders but still manipulable with sufficient size.

---

### Index-Anchored (Manipulation-Resistant)

All models below require an external `IndexPriceProvider`. They differ in how they smooth and bound the premium (perp_mid − index).

#### `BinanceMarkPrice` — Binance style
```
mark = median(index, best_bid, best_ask)
```
Three-input median. To move the mark, an attacker must control **two** of three inputs simultaneously. Moving the perp book requires both sides, which costs real capital on both sides of the spread.

**Construction**:
```go
NewBinanceMarkPrice(symbol string, index IndexPriceProvider) *BinanceMarkPrice
```

**Manipulation cost**: High. Requires controlling both perp bid and ask or the external index.

---

#### `BitMEXMarkPrice` — BitMEX style
```
basis_t = perp_mid - index
EMA_basis_t = α × basis_t + (1 − α) × EMA_basis_{t−1}
mark = index + EMA_basis
```
The EMA is applied to the **basis** (premium), not the absolute price. An attacker shifting the perp mid only nudges the EMA by `α` per sample; the basis cannot be moved abruptly. For a window of N samples, `α = 2/(N+1)`.

**Construction**:
```go
NewBitMEXMarkPrice(symbol string, index IndexPriceProvider, windowSamples int) *BitMEXMarkPrice
```

**Manipulation cost**: Proportional to `1/windowSamples`. Larger window → smaller per-sample nudge → higher attack cost.

**Note**: The EMA has no hard bound. A sustained attack over many samples can still drift the mark arbitrarily far from index.

---

#### `BybitMarkPrice` — Bybit style
```
basis_t = perp_mid - index
EMA_basis_t = α × basis_t + (1 − α) × EMA_basis_{t−1}
EMA_basis_t = clamp(EMA_basis_t, −halfBand, +halfBand)
mark = index + clamp(EMA_basis, −halfBand, +halfBand)
where halfBand = index × bandBps / 2 / 10000
```
Same as BitMEX plus a **hard band**: the mark can never deviate more than `bandBps/2` from the index regardless of how many samples the attacker sustains. `bandBps` is typically the initial margin rate in bps (e.g. 1000 for 10x leverage), so the mark is bounded within the maintenance margin.

**Construction**:
```go
NewBybitMarkPrice(symbol string, index IndexPriceProvider, windowSamples int, bandBps int64) *BybitMarkPrice
```

**Manipulation cost**: Bounded. Attack cannot push mark beyond `index ± index×bandBps/20000`, making systematic liquidation attacks financially impossible if the band is set at the margin rate.

---

#### `DydxMarkPrice` — dYdX style
```
basis_t = perp_mid - index
TWAP_basis = mean(basis_{t}, basis_{t−1}, …, basis_{t−window+1})
mark = index + clamp(TWAP_basis, −halfBand, +halfBand)
```
Uses a **TWAP** (time-weighted average, implemented as a circular buffer) rather than an EMA. The TWAP is more robust to outlier spikes than EMA: a single extreme sample contributes exactly `1/windowSamples` to the average regardless of magnitude, whereas EMA assigns exponentially more weight to recent samples.

In production dYdX uses Chainlink/Pyth oracle prices as index. In simulation, wire a `GBMProcess`.

**Construction**:
```go
NewDydxMarkPrice(symbol string, index IndexPriceProvider, windowSamples int, bandBps int64) *DydxMarkPrice
```

**Manipulation cost**: Same hard bound as Bybit. A spike attack is absorbed uniformly over the window rather than decaying exponentially.

---

## Comparison

| Model | Index required | Smoothing | Hard band | Attack surface |
|-------|---------------|-----------|-----------|----------------|
| `LastPriceCalculator` | No | None | No | Single wash trade |
| `MidPriceCalculator` | No | None | No | Single spoofed order |
| `WeightedMidPriceCalculator` | No | None | No | Size-scaled spoofed order |
| `BinanceMarkPrice` | Yes | None | No (median) | Both perp book sides simultaneously |
| `BitMEXMarkPrice` | Yes | EMA of basis | No | Sustained multi-sample attack |
| `BybitMarkPrice` | Yes | EMA of basis | Yes | Bounded; cost = `bandBps × index / 2` |
| `DydxMarkPrice` | Yes | TWAP of basis | Yes | Bounded; uniform sample weight |

---

## Wiring Index to GBM in Simulation

```go
// Shared fundamental process
gbm := simulation.NewGBMProcess(
    50_000 * USD_PRECISION, // $50,000 initial price
    USD_PRECISION,
    0.0,  // zero drift
    0.50, // 50% annual vol
    42,
)
gbm.Register("BTC-PERP")

// Advance the GBM from a simulation tick loop:
// gbm.Advance(dtSeconds)

// Wire as mark price calculator (BitMEX style, 10-minute window at 1s ticks)
ex.Books["BTC-PERP"].MarkCalc = exchange.NewBitMEXMarkPrice("BTC-PERP", gbm, 600)

// Wire same process as informed trader signal
informedTrader := actors.NewInformedTrader(1, gateway, actors.InformedTraderConfig{
    Symbol:       "BTC-PERP",
    Oracle:       gbm,  // GBMProcess implements PrivateSignalOracle
    ThresholdBps: 10,
    OrderQty:     BTC_PRECISION / 10,
})
```

---

## What Was Wrong Before

The previous `EMAMarkPriceCalculator` applied EMA to the perp book's **own last trade price**:
```
mark = EMA(last_trade_price)
```
This is **as manipulable as `LastPriceCalculator`**: a wash trader who controls the last trade price also controls the EMA, just with a lag equal to the window. The EMA provides no protection because both the attack input and the mark output live in the same book.

Manipulation resistance requires an **external reference** that the attacker cannot move by trading the perp book. That is the index price.

The previous `MedianMarkPriceCalculator` computed `median(bid, ask, last_trade)` — all three from the same book. Same problem. The median of three correlated book variables offers no protection.

Both were replaced with the four index-anchored models above.
