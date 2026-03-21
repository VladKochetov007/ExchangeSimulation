# User Guide

Practical guide for building simulations with the exchange simulation library. For architecture overview, see [README.md](../README.md).

---

## Table of Contents

1. [Quick Start](#1-quick-start)
2. [Building a Strategy (Actor)](#2-building-a-strategy-actor)
3. [Exchange Configuration](#3-exchange-configuration)
4. [Matching Engines](#4-matching-engines)
5. [Fee Models](#5-fee-models)
6. [Simulation and Clock](#6-simulation-and-clock)
7. [Latency Modeling](#7-latency-modeling)
8. [Multi-Venue Simulations](#8-multi-venue-simulations)
9. [Price Sources and Funding](#9-price-sources-and-funding)
10. [Logging and Event Format (JSONL)](#10-logging-and-event-format-jsonl)
11. [Market Data Subscriptions](#11-market-data-subscriptions)
12. [Full Working Example](#12-full-working-example)
13. [Common Pitfalls](#13-common-pitfalls)

---

## 1. Quick Start

Every simulation follows a five-step pattern: **Clock -> Exchange -> Instruments -> Clients -> Actors**.

```go
package main

import (
    "context"
    "time"

    "exchange_sim/actor"
    "exchange_sim/exchange"
    "exchange_sim/simulation"
)

func main() {
    // 1. Clock
    simClock := simulation.NewSimulatedClock(
        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano(),
    )
    scheduler := simulation.NewEventScheduler(simClock)
    simClock.SetScheduler(scheduler)
    timerFact := simulation.NewSimTimerFactory(scheduler)

    // 2. Exchange
    ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
        EstimatedClients: 10,
        Clock:            simClock,
        TickerFactory:    timerFact,
        SnapshotInterval: time.Second,
    })

    // 3. Instruments
    spot := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD",
        exchange.BTC_PRECISION,  // 100_000_000 (satoshis per BTC)
        exchange.USD_PRECISION,  // 100_000     (cents × 1000)
        exchange.DOLLAR_TICK,    // 100_000     ($1 minimum price increment)
        exchange.BTC_PRECISION/100, // minimum order size: 0.01 BTC
    )
    ex.AddInstrument(spot)

    // 4. Clients
    balances := map[string]int64{
        "BTC": 10 * exchange.BTC_PRECISION,
        "USD": 100_000 * exchange.USD_PRECISION,
    }
    fee := &exchange.PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true}
    mount := simulation.NewMount(ex, simulation.LatencyConfig{})
    gw := mount.ConnectNewClient(1, balances, fee)

    // 5. Actor
    myActor := NewMyStrategy(1, gw)
    myActor.SetTickerFactory(timerFact)

    // Run
    runner := simulation.NewRunner(simClock, simulation.RunnerConfig{
        Iterations: 600_000,        // 600k steps
        Step:       time.Millisecond, // 1ms per step = 10 minutes sim time
    })
    runner.AddMount(mount)
    runner.AddActor(myActor)

    ctx := context.Background()
    runner.Run(ctx)
}
```

---

## 2. Building a Strategy (Actor)

### Minimal Actor

Embed `actor.BaseActor`, implement `actor.EventHandler`, register a ticker for periodic logic.

```go
package mystrategy

import (
    "context"
    "time"

    "exchange_sim/actor"
    "exchange_sim/exchange"
)

type SimpleMMConfig struct {
    Symbol    string
    TickSize  int64
    LevelSize int64
}

type SimpleMM struct {
    *actor.BaseActor
    cfg SimpleMMConfig
    mid int64
}

func NewSimpleMM(id uint64, gw actor.Gateway, cfg SimpleMMConfig) *SimpleMM {
    mm := &SimpleMM{
        BaseActor: actor.NewBaseActor(id, gw),
        cfg:       cfg,
        mid:       50_000 * exchange.USD_PRECISION, // bootstrap mid
    }
    mm.SetHandler(mm) // inline event handling (no extra goroutine)
    mm.AddTicker(100*time.Millisecond, mm.onTick)
    return mm
}

func (mm *SimpleMM) HandleEvent(_ context.Context, evt *actor.Event) {
    switch evt.Type {
    case actor.EventOrderAccepted:
        e := evt.Data.(actor.OrderAcceptedEvent)
        _ = e.OrderID // track if needed

    case actor.EventOrderPartialFill, actor.EventOrderFilled:
        e := evt.Data.(actor.OrderFillEvent)
        mm.mid = e.Price // update mid from last fill

    case actor.EventOrderCancelled:
        // clean up tracking state

    case actor.EventOrderRejected:
        e := evt.Data.(actor.OrderRejectedEvent)
        _ = e.Reason // e.g. "INSUFFICIENT_BALANCE", "INVALID_PRICE"
    }
}

func (mm *SimpleMM) onTick(_ time.Time) {
    bidPrice := mm.mid - mm.cfg.TickSize
    askPrice := mm.mid + mm.cfg.TickSize

    mm.SubmitOrder(mm.cfg.Symbol, exchange.Buy, exchange.LimitOrder, bidPrice, mm.cfg.LevelSize)
    mm.SubmitOrder(mm.cfg.Symbol, exchange.Sell, exchange.LimitOrder, askPrice, mm.cfg.LevelSize)
}
```

### BaseActor Methods

| Method | Description |
|--------|-------------|
| `SubmitOrder(symbol, side, orderType, price, qty) uint64` | Submit order, returns request ID |
| `SubmitOrderFull(symbol, side, orderType, price, qty, visibility, icebergQty) uint64` | Full control (iceberg/hidden) |
| `CancelOrder(orderID uint64)` | Cancel a resting order |
| `Subscribe(symbol string, types ...MDType)` | Subscribe to market data |
| `Unsubscribe(symbol string)` | Unsubscribe |
| `QueryBalance()` | Request balance snapshot |
| `QueryAccount()` | Request full account state (includes positions) |
| `SetHandler(h EventHandler)` | Set inline event handler (call before Start) |
| `AddTicker(d time.Duration, fn func(time.Time))` | Register periodic callback (call before Start) |
| `SetTickerFactory(factory TickerFactory)` | Inject sim-aware ticker factory (call before Start) |

### Event Types

| Event | Data Type | When |
|-------|-----------|------|
| `EventOrderAccepted` | `OrderAcceptedEvent` | Limit order resting in book |
| `EventOrderRejected` | `OrderRejectedEvent` | Order refused (balance, price, qty) |
| `EventOrderPartialFill` | `OrderFillEvent` | Partial execution (`IsFull = false`) |
| `EventOrderFilled` | `OrderFillEvent` | Full execution (`IsFull = true`) |
| `EventOrderCancelled` | `OrderCancelledEvent` | Cancel confirmed (includes forced liquidation cancels) |
| `EventOrderCancelRejected` | `OrderCancelRejectedEvent` | Cancel refused |
| `EventTrade` | `TradeEvent` | Market data: trade on subscribed symbol |
| `EventBookSnapshot` | `BookSnapshotEvent` | Market data: full book state |
| `EventBookDelta` | `BookDeltaEvent` | Market data: single level change |
| `EventFundingUpdate` | `FundingUpdateEvent` | Market data: funding rate update |
| `EventOpenInterest` | `OpenInterestEvent` | Market data: OI change |
| `EventBalanceUpdate` | `BalanceUpdateEvent` | Response to `QueryBalance()` |
| `EventAccountUpdate` | `AccountUpdateEvent` | Response to `QueryAccount()` |

### Concurrency Model

All events, fills, and ticker callbacks run inside a single goroutine (`BaseActor.run`). No locking is needed inside `HandleEvent` or ticker functions — state mutations are naturally serialized.

```
select on:
  ctx.Done()           → exit
  stopCh               → exit
  gateway.Responses()  → decode → HandleEvent
  gateway.MarketDataCh → decode → HandleEvent
  tickCh               → fire ticker callback
```

### Order Types and Enums

```go
// Sides
exchange.Buy
exchange.Sell

// Order types
exchange.Market      // immediate execution, no resting
exchange.LimitOrder  // rests in book at specified price

// Time in force (via SubmitOrderFull)
exchange.GTC  // good till cancel (default)
exchange.IOC  // immediate or cancel
exchange.FOK  // fill or kill

// Visibility (via SubmitOrderFull)
exchange.Normal
exchange.Iceberg  // partial display qty
exchange.Hidden   // dark order
```

---

## 3. Exchange Configuration

### ExchangeConfig

```go
ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
    ID:                      "binance-sim",        // exchange identifier (default: "exchange")
    EstimatedClients:        20,                    // pre-allocate maps
    Clock:                   simClock,              // Clock interface (default: RealClock)
    TickerFactory:           timerFact,             // TickerFactory (default: RealTickerFactory)
    SnapshotInterval:        time.Second,           // book snapshot publish interval
    BalanceSnapshotInterval: 10 * time.Second,      // balance log interval (0 = disabled)
})
```

### Instruments

**Spot:**

```go
inst := exchange.NewSpotInstrument(
    "ETH/USD",                // symbol
    "ETH",                    // base asset
    "USD",                    // quote asset
    1_000_000_000,            // base precision (1 ETH = 1e9 units)
    100_000,                  // quote precision (1 USD = 1e5 units)
    10_000,                   // tick size ($0.10 in quote precision)
    1_000_000,                // min order size (0.001 ETH)
)
```

**Perpetual futures:**

```go
perp := exchange.NewPerpFutures(
    "ETH-PERP", "ETH", "USD",
    1_000_000_000, 100_000,
    10_000,        // tick size
    1_000_000,     // min order size
)
// Optional: configure funding interval (seconds between settlements)
perp.GetFundingRate().Interval = 120  // 2-minute funding
```

All prices are integers in quote precision units. A price of `50_000 * exchange.USD_PRECISION` represents $50,000.00. Prices must be multiples of tick size — the exchange rejects orders with `RejectInvalidPrice` otherwise.

### Connecting Clients

```go
// Spot balances
gw := mount.ConnectNewClient(clientID, map[string]int64{
    "BTC": 10 * exchange.BTC_PRECISION,
    "USD": 100_000 * exchange.USD_PRECISION,
}, feePlan)

// For perpetual trading, add perp wallet balance separately
ex.AddPerpBalance(clientID, "USD", 1_000_000 * exchange.USD_PRECISION)
```

Each client gets independent balances. The fee plan is per-client — different clients can have different fee tiers.

---

## 4. Matching Engines

### Interface

```go
// matching/matching.go
type MatchingEngine interface {
    Match(bidBook, askBook *book.Book, incomingOrder *types.Order) *MatchResult
}

type MatchResult struct {
    Executions  []*types.Execution
    FullyFilled bool
}
```

### Built-in Engines

**Price-Time (FIFO)** — default. Orders at the same price are filled in arrival order.

```go
import "exchange_sim/matching"

matcher := matching.NewPriceTimeMatcher(clock) // nil = RealClock
```

**Pro-Rata** — orders at the same price are filled proportionally to their size (CME Globex style).

```go
matcher := matching.NewProRataMatcher(clock)
```

### Custom Matching Engine

The `Matcher` field on `DefaultExchange` is public. Replace it after construction:

```go
ex := exchange.NewExchange(10, simClock)
ex.Matcher = &MyCustomMatcher{clock: simClock}
```

Implement the interface:

```go
type MyCustomMatcher struct {
    clock types.Clock
}

func (m *MyCustomMatcher) Match(bidBook, askBook *book.Book, incoming *types.Order) *matching.MatchResult {
    // Your matching logic here.
    // Walk the opposing book, create Execution structs for each fill.
    // Return MatchResult with all executions and whether the incoming order was fully filled.
    //
    // Important: update order.FilledQty for each execution.
    // Important: remove fully filled orders from the book (book.UnlinkOrder + delete from book.Orders).
    executions := make([]*types.Execution, 0)
    // ... matching logic ...
    return &matching.MatchResult{
        Executions:  executions,
        FullyFilled: incoming.FilledQty >= incoming.Qty,
    }
}
```

---

## 5. Fee Models

### Interface

```go
// types/interfaces.go
type FeeModel interface {
    CalculateFee(ctx FillContext) Fee
}

type FillContext struct {
    Exec       *Execution
    IsMaker    bool
    BaseAsset  string
    QuoteAsset string
    Precision  int64
}

type Fee struct {
    Asset  string
    Amount int64
}
```

### Built-in Fee Models

**Percentage fee** (most common):

```go
fee := &exchange.PercentageFee{
    MakerBps: 2,     // 0.02% maker fee
    TakerBps: 5,     // 0.05% taker fee
    InQuote:  true,   // fee charged in quote asset (USD)
}
```

When `InQuote = false`, fee is charged in base asset (e.g., BTC).

**Fixed fee:**

```go
fee := &exchange.FixedFee{
    MakerFee: exchange.Fee{Asset: "USD", Amount: 100_000},  // $1.00 per fill
    TakerFee: exchange.Fee{Asset: "USD", Amount: 500_000},  // $5.00 per fill
}
```

### Custom Fee Model

```go
type VIPTieredFee struct {
    volumeThreshold int64
    lowBps          int64
    highBps         int64
}

func (f *VIPTieredFee) CalculateFee(ctx exchange.FillContext) exchange.Fee {
    bps := f.highBps
    if ctx.IsMaker {
        bps = 0 // maker rebate = zero fee
    }
    tradeValue := (ctx.Exec.Price * ctx.Exec.Qty) / ctx.Precision
    amount := (tradeValue * bps) / 10000
    return exchange.Fee{Asset: ctx.QuoteAsset, Amount: amount}
}
```

---

## 6. Simulation and Clock

### Clock Wiring

Three components work together for deterministic simulation time:

```go
// 1. SimulatedClock — holds current time, advances on command
simClock := simulation.NewSimulatedClock(startTimeNanos)

// 2. EventScheduler — priority queue of time-based callbacks
scheduler := simulation.NewEventScheduler(simClock)
simClock.SetScheduler(scheduler)

// 3. SimTimerFactory — creates tickers backed by the scheduler
timerFact := simulation.NewSimTimerFactory(scheduler)
```

Pass `simClock` to the exchange and `timerFact` to both the exchange and every actor:

```go
ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
    Clock:         simClock,
    TickerFactory: timerFact,
})

myActor.SetTickerFactory(timerFact) // BEFORE calling Start()
```

**Forgetting `SetTickerFactory` on an actor means its tickers run on wall-clock time.** In a simulation that completes in 2 seconds, a 100ms real-time ticker fires ~20 times instead of the expected thousands.

### Runner

The `Runner` starts all actors, advances the clock, and shuts everything down on completion.

```go
runner := simulation.NewRunner(simClock, simulation.RunnerConfig{
    Iterations: 600_000,          // number of clock steps
    Step:       time.Millisecond, // each step advances clock by 1ms
    // Duration: 30 * time.Second, // optional wall-clock timeout
})

runner.AddMount(mount)
runner.AddActor(actor1)
runner.AddActor(actor2)

// Optional progress reporting
runner.SetProgressCallback(100_000, func(done, total int) {
    fmt.Printf("Progress: %d/%d\n", done, total)
})

runner.Run(ctx) // blocks until iterations complete or ctx cancelled
```

The runner calls `clock.Advance(step)` for each iteration. Each advance fires all scheduled events up to the new time (tickers, funding settlements, snapshot publishers).

### Mount (Venue)

`Mount` wraps an exchange with optional latency. Use it even without latency — it's the standard way to connect clients to venues in simulation mode.

```go
mount := simulation.NewMount(ex, simulation.LatencyConfig{}) // no latency
gw := mount.ConnectNewClient(clientID, balances, feePlan)
```

---

## 7. Latency Modeling

### LatencyProvider Interface

```go
type LatencyProvider interface {
    Delay() time.Duration
}
```

### Per-Channel Configuration

Each communication channel (request, response, market data) can have independent latency:

```go
mount := simulation.NewMount(ex, simulation.LatencyConfig{
    Request:    simulation.NewConstantLatency(1 * time.Millisecond),
    Response:   simulation.NewLogNormalLatency(500*time.Microsecond, 2*time.Millisecond, 0.5, 42),
    MarketData: simulation.NewConstantLatency(500 * time.Microsecond),
})
```

`nil` on any field means zero delay for that channel.

### Built-in Latency Providers

**Constant** — fixed delay, useful for baseline testing:

```go
simulation.NewConstantLatency(1 * time.Millisecond)
```

**Uniform Random** — delay sampled uniformly from `[min, max]`:

```go
simulation.NewUniformRandomLatency(
    500 * time.Microsecond, // min
    5 * time.Millisecond,   // max
    42,                     // seed
)
```

**Normal** — Gaussian distribution (clamped to >= 0):

```go
simulation.NewNormalLatency(
    2 * time.Millisecond,   // mean
    500 * time.Microsecond, // stddev
    42,                     // seed
)
```

**Log-Normal** — heavy-tailed, models retransmit spikes and GC pauses:

```go
simulation.NewLogNormalLatency(
    500 * time.Microsecond, // min (hard floor)
    2 * time.Millisecond,   // median above min
    0.5,                    // logSigma (tail heaviness)
    42,                     // seed
)
```

Calibrating `logSigma`:

| logSigma | p99 / median | Profile |
|----------|-------------|---------|
| 0.3 | ~2x | Tight, stable LAN |
| 0.5 | ~3x | Typical co-location |
| 1.0 | ~10x | WAN / congested path |

**Load-Scaled** — latency grows linearly with in-flight request count:

```go
ls := simulation.NewLoadScaledLatency(
    1 * time.Millisecond,     // base latency
    100 * time.Microsecond,   // additional per in-flight request
)
// Call ls.Inc() on order submit, ls.Dec() on acknowledgement
```

**Hawkes (Self-Exciting)** — models exchange queue congestion. Each order submission injects a latency spike that decays exponentially:

```go
simulation.NewHawkesLatency(
    500 * time.Microsecond,  // min latency
    10 * time.Microsecond,   // jump per event (alpha)
    10.0,                    // decay per second (beta)
)
// Call h.RecordEvent() on each order submission
```

Calibrating `beta` (decay rate):

| beta | Half-life | Profile |
|------|-----------|---------|
| 1 | 693ms | Slow drain, persistent congestion |
| 10 | 69ms | Moderate, burst clears in ~150ms |
| 100 | 7ms | Fast, typical co-located exchange |

Steady-state added latency: `jump * orderRate / beta`. Example: 10us jump, 1000 orders/s, beta=10 -> +1ms above minimum.

---

## 8. Multi-Venue Simulations

### Setup

Create separate exchanges, wrap each in a Mount, connect the same actor to multiple venues:

```go
// Two exchanges with different characteristics
fastEx := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
    ID: "fast-venue", Clock: simClock, TickerFactory: timerFact,
})
slowEx := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
    ID: "slow-venue", Clock: simClock, TickerFactory: timerFact,
})
// ... add instruments to both ...

fastMount := simulation.NewMount(fastEx, simulation.LatencyConfig{
    Request:  simulation.NewConstantLatency(1 * time.Millisecond),
    Response: simulation.NewConstantLatency(1 * time.Millisecond),
})
slowMount := simulation.NewMount(slowEx, simulation.LatencyConfig{
    Request:  simulation.NewLogNormalLatency(5*time.Millisecond, 10*time.Millisecond, 0.5, 42),
    Response: simulation.NewLogNormalLatency(5*time.Millisecond, 10*time.Millisecond, 0.5, 43),
})

// Same client ID, different venues → two gateways
fastGW := fastMount.ConnectNewClient(1, balances, fee)
slowGW := slowMount.ConnectNewClient(1, balances, fee)

// Actor owns both gateways
arb := NewCrossVenueArb(1, fastGW, slowGW)
arb.SetTickerFactory(timerFact)

runner := simulation.NewRunner(simClock, simulation.RunnerConfig{...})
runner.AddMount(fastMount)
runner.AddMount(slowMount)
runner.AddActor(arb)
runner.Run(ctx)
```

### Gateway = Venue Identity

Which channel delivers a message tells you which exchange it came from. No tagging needed:

```go
func (a *CrossVenueArb) run(ctx context.Context) {
    for {
        select {
        case resp := <-a.fastGW.Responses():
            a.onFastResponse(resp)
        case resp := <-a.slowGW.Responses():
            a.onSlowResponse(resp)
        case md := <-a.fastGW.MarketDataCh():
            a.onFastMarketData(md)
        case md := <-a.slowGW.MarketDataCh():
            a.onSlowMarketData(md)
        case <-ctx.Done():
            return
        }
    }
}
```

Multi-venue actors cannot use `BaseActor` directly (it manages a single gateway). Implement the `actor.Actor` interface and manage the `select` loop yourself.

---

## 9. Price Sources and Funding

### Mark Price Calculators

Mark price feeds funding rate calculation, margin checks, and liquidation triggers. Seven built-in calculators:

| Calculator | Formula | Use case |
|-----------|---------|----------|
| `NewLastPriceCalculator()` | Last trade price | Simple, manipulable |
| `NewMidPriceCalculator()` | `(bestBid + bestAsk) / 2` | Fair with symmetric liquidity |
| `NewWeightedMidPriceCalculator()` | Qty-weighted mid | Pulls toward thicker side |
| `NewMedianMarkPrice(indexProvider)` | `median(index, bestBid, bestAsk)` | Resists spoofing |
| `NewEMAMarkPrice(indexProvider, windowSamples)` | `index + EMA(perpMid - index)` | Smooths basis noise |
| `NewClampedEMAMarkPrice(index, window, bandBps)` | EMA clamped to index +/- band | Liquidation protection |
| `NewTWAPMarkPrice(index, window, bandBps)` | TWAP clamped to index +/- band | Rolling window average |

### Index Price Sources

**Static** — fixed prices, useful for testing:

```go
oracle := exchange.NewStaticPriceOracle(map[string]int64{
    "BTC-PERP": 50_000 * exchange.USD_PRECISION,
    "ETH-PERP":  3_000 * exchange.USD_PRECISION,
})
```

**Mid-Price from Order Book** — derives index from spot book mid:

```go
oracle := exchange.NewMidPriceOracle(ex)
oracle.MapSymbol("BTC-PERP", "BTC/USD")  // use BTC/USD spot mid as BTC-PERP index
oracle.MapSymbol("ETH-PERP", "ETH/USD")
```

### Automation Wiring

```go
ex.ConfigureAutomation(exchange.AutomationConfig{
    MarkPriceCalc:       exchange.NewWeightedMidPriceCalculator(),
    IndexProvider:       oracle,
    PriceUpdateInterval: 30 * time.Second,   // mark/funding update cadence
    CollateralRate:      500,                 // 5% annual on borrowed margin
    LiquidationHandler:  myRiskManager,       // optional callbacks
})

ex.StartAutomation(ctx)
defer ex.StopAutomation()
```

Automation spawns three goroutines:
1. **Price update** — recalculates mark price, updates funding rate, publishes `MDFunding`
2. **Funding settlement** — settles funding payments between longs and shorts
3. **Collateral charge** — accrues interest on borrowed margin

### Custom Funding Calculator

```go
type MyFundingCalc struct{}

func (f *MyFundingCalc) Calculate(indexPrice, markPrice int64) int64 {
    // Return funding rate in bps
    premium := ((markPrice - indexPrice) * 10000) / indexPrice
    return clamp(premium, -75, 75)
}

perp := exchange.NewPerpFutures(...)
perp.SetFundingCalculator(&MyFundingCalc{})
```

### Liquidation Handler

```go
type MyRiskManager struct{}

func (r *MyRiskManager) OnMarginCall(event *exchange.MarginCallEvent) {
    // Warning: margin ratio approaching maintenance level
}

func (r *MyRiskManager) OnLiquidation(event *exchange.LiquidationEvent) {
    // Position force-closed
}

func (r *MyRiskManager) OnInsuranceFund(event *exchange.InsuranceFundEvent) {
    // Shortfall covered by insurance fund
}
```

---

## 10. Logging and Event Format (JSONL)

### Overview

The exchange logs events as **NDJSON** (newline-delimited JSON) — one JSON object per line. Each line contains a base set of fields plus event-specific fields merged at the top level.

### Logger Interface

```go
type Logger interface {
    LogEvent(simTime int64, clientID uint64, eventName string, event any)
}
```

The built-in logger (`logger.New(writer)`) writes to any `io.Writer`:

```go
import "exchange_sim/logger"

f, _ := os.Create("trades.jsonl")
log := logger.New(f)
ex.SetLogger("BTC/USD", log)
```

### Logger Keys

```go
ex.SetLogger("_global", globalLog)   // exchange-wide events
ex.SetLogger("BTC/USD", spotLog)     // per-symbol spot events
ex.SetLogger("BTC-PERP", perpLog)    // per-symbol perp events
```

### Base Fields (Every Line)

```json
{
    "sim_time": 1735689600000000000,
    "server_time": 1711036800123456789,
    "event": "Trade",
    "client_id": 1
}
```

### Event Examples

**Trade** (per-symbol logger):

```json
{
    "sim_time": 1735689601000000000,
    "server_time": 1711036800200000000,
    "event": "Trade",
    "client_id": 0,
    "trade_id": 42,
    "price": 5000000000000,
    "qty": 100000000,
    "side": "BUY",
    "taker_order_id": 15,
    "maker_order_id": 8
}
```

**OrderFill** (per-symbol, logged once per participant):

```json
{
    "sim_time": 1735689601000000000,
    "event": "OrderFill",
    "client_id": 1,
    "order_id": 15,
    "symbol": "BTC/USD",
    "qty": 100000000,
    "price": 5000000000000,
    "side": "BUY",
    "filled_qty": 100000000,
    "remaining_qty": 0,
    "is_full": true,
    "trade_id": 42,
    "role": "TAKER",
    "fee_amount": 250000,
    "fee_asset": "USD"
}
```

**balance_change** (per-symbol or global):

```json
{
    "sim_time": 1735689601000000000,
    "event": "balance_change",
    "client_id": 1,
    "reason": "trade_settlement",
    "changes": [
        {"asset": "BTC", "wallet": "spot", "old_balance": 1000000000, "new_balance": 1100000000, "delta": 100000000},
        {"asset": "USD", "wallet": "spot", "old_balance": 100000000000, "new_balance": 95000000000, "delta": -5000000000}
    ]
}
```

**balance_snapshot** (global, periodic):

```json
{
    "sim_time": 1735689610000000000,
    "event": "balance_snapshot",
    "client_id": 1,
    "spot_balances": [
        {"asset": "BTC", "free": 900000000, "locked": 100000000},
        {"asset": "USD", "free": 95000000000, "locked": 5000000000}
    ],
    "perp_balances": [
        {"asset": "USD", "free": 9500000000, "locked": 500000000}
    ]
}
```

**position_update** (global):

```json
{
    "event": "position_update",
    "client_id": 5,
    "symbol": "BTC-PERP",
    "old_size": 100000000,
    "old_entry_price": 5000000000000,
    "new_size": 200000000,
    "new_entry_price": 5005000000000,
    "trade_qty": 100000000,
    "trade_price": 5010000000000,
    "trade_side": "BUY",
    "reason": "trade"
}
```

**realized_pnl** (global, non-zero only):

```json
{
    "event": "realized_pnl",
    "client_id": 5,
    "symbol": "BTC-PERP",
    "trade_id": 100,
    "closed_qty": 50000000,
    "entry_price": 5000000000000,
    "exit_price": 5050000000000,
    "pnl": 25000000,
    "side": "SELL"
}
```

**BookSnapshot** (per-symbol, periodic):

```json
{
    "event": "BookSnapshot",
    "bids": [
        {"price": 4999900000000, "qty": 100000000},
        {"price": 4999800000000, "qty": 200000000}
    ],
    "asks": [
        {"price": 5000100000000, "qty": 100000000},
        {"price": 5000200000000, "qty": 150000000}
    ]
}
```

**Futures-only events** (per-symbol):

```json
{"event": "mark_price_update", "symbol": "BTC-PERP", "mark_price": 5000050000000, "index_price": 5000000000000}
{"event": "funding_rate_update", "symbol": "BTC-PERP", "rate": 10, "next_funding": 1735689720}
```

### Custom Logger

Implement the `Logger` interface for custom output (database, metrics, etc.):

```go
type JSONLinesLogger struct {
    mu sync.Mutex
    w  *bufio.Writer
    f  *os.File
}

func (l *JSONLinesLogger) LogEvent(simTime int64, clientID uint64, eventName string, event any) {
    b, _ := json.Marshal(map[string]any{
        "sim_ts":    simTime,
        "client_id": clientID,
        "event":     eventName,
        "data":      event,
    })
    l.mu.Lock()
    l.w.Write(b)
    l.w.WriteByte('\n')
    l.mu.Unlock()
}
```

### Wallet Names in balance_change

| Wallet | Meaning |
|--------|---------|
| `spot` | Spot balance |
| `perp` | Perp margin balance |
| `reserved_spot` | Spot locked in open orders |
| `reserved_perp` | Perp locked as order margin |
| `borrowed` | Margin loan outstanding |

---

## 11. Market Data Subscriptions

### MDType Filter

Actors subscribe to specific data types per symbol:

```go
// Subscribe to trades and book snapshots
mm.Subscribe("BTC/USD", exchange.MDSnapshot, exchange.MDTrade)

// Subscribe to funding rate updates (perp only)
arb.Subscribe("BTC-PERP", exchange.MDFunding)

// Subscribe to everything
mm.Subscribe("BTC/USD", exchange.MDSnapshot, exchange.MDDelta, exchange.MDTrade)
```

| MDType | Event in Handler | Data |
|--------|-----------------|------|
| `MDSnapshot` | `EventBookSnapshot` | Full book (all price levels, bid/ask) |
| `MDDelta` | `EventBookDelta` | Single level change (side, price, qty) |
| `MDTrade` | `EventTrade` | Executed trade (price, qty, taker side) |
| `MDFunding` | `EventFundingUpdate` | Funding rate, next settlement time |
| `MDOpenInterest` | `EventOpenInterest` | Total open interest |

### Handling Market Data in Actor

```go
func (a *MyActor) HandleEvent(_ context.Context, evt *actor.Event) {
    switch evt.Type {
    case actor.EventBookSnapshot:
        snap := evt.Data.(actor.BookSnapshotEvent)
        // snap.Symbol, snap.Snapshot.Bids, snap.Snapshot.Asks

    case actor.EventTrade:
        trade := evt.Data.(actor.TradeEvent)
        // trade.Symbol, trade.Trade.Price, trade.Trade.Qty

    case actor.EventFundingUpdate:
        funding := evt.Data.(actor.FundingUpdateEvent)
        // funding.FundingRate.Rate (bps), funding.FundingRate.NextFunding (unix seconds)
    }
}
```

---

## 12. Full Working Example

See `cmd/randomwalk/main.go` and `simulations/randomwalk/sim.go` for a complete 13-client simulation with:

- 3 market makers (one per asset, quoting spot + perp)
- 1 random taker (across all 6 symbols)
- 3 basis arbitrage actors (one per spot/perp pair)
- 3 funding arbitrage actors (carry trade around settlement)
- 1 cross-pair market maker (derived fair value from USD pairs)
- 2 triangular arbitrage actors (3-leg state machines)

The wiring pattern in `sim.go`:

```go
func NewSim(simTime time.Duration) (*Sim, error) {
    // 1. Clock stack
    simClock := simulation.NewSimulatedClock(...)
    scheduler := simulation.NewEventScheduler(simClock)
    simClock.SetScheduler(scheduler)
    timerFact := simulation.NewSimTimerFactory(scheduler)

    // 2. Exchange with sim clock
    ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
        Clock: simClock, TickerFactory: timerFact, ...
    })

    // 3. Register instruments + loggers
    for _, asset := range assets {
        ex.AddInstrument(exchange.NewSpotInstrument(...))
        ex.AddInstrument(exchange.NewPerpFutures(...))
        ex.SetLogger(spotSymbol, spotLogger)
        ex.SetLogger(perpSymbol, perpLogger)
    }

    // 4. Configure automation (mark price, index, funding)
    ex.ConfigureAutomation(exchange.AutomationConfig{...})

    // 5. Mount (venue wrapper)
    mount := simulation.NewMount(ex, simulation.LatencyConfig{})

    // 6. Connect clients + create actors
    for each actor {
        gw := mount.ConnectNewClient(clientID, balances, fee)
        ex.AddPerpBalance(clientID, "USD", perpBalance)
        actor := NewSomeActor(clientID, gw, config)
        actor.SetTickerFactory(timerFact)  // CRITICAL for sim time
    }

    // 7. Runner
    runner := simulation.NewRunner(simClock, simulation.RunnerConfig{
        Iterations: int(simTime / step),
        Step:       step,
    })
    runner.AddMount(mount)
    for _, a := range allActors { runner.AddActor(a) }

    return &Sim{Runner: runner, ...}, nil
}
```

Run: `go run cmd/randomwalk/main.go`

Logs written to `logs/randomwalk/` with one JSONL file per symbol + a global file.

---

## 13. Common Pitfalls

### Handle BOTH Partial and Full Fills

The actor framework emits `EventOrderPartialFill` when `IsFull = false` and `EventOrderFilled` when `IsFull = true`. If your actor only handles `EventOrderFilled`, partially filled orders will never be cleaned up from your tracking state.

```go
// CORRECT
case actor.EventOrderPartialFill, actor.EventOrderFilled:
    e := evt.Data.(actor.OrderFillEvent)
    // handle fill...
```

### Use Signed Position Counters, Not Booleans

```go
// WRONG — actor freezes after first trade
var inPosition bool

// CORRECT — supports incremental position building and unwinding
var position int64
```

### Always Call SetTickerFactory Before Start

```go
myActor := NewMyActor(id, gw, config)
myActor.SetTickerFactory(timerFact) // BEFORE Start()
// runner.AddActor(myActor) — Start() is called by Runner.Run()
```

### Align Prices to Tick Size

The exchange rejects orders where `price % tickSize != 0`. Always align:

```go
alignedPrice := (rawPrice / tickSize) * tickSize
```

### Handle Late Accepts (Ghost Orders)

When you cancel all orders for a symbol, some accept responses may still be in flight. The accepted orders are live in the book but untracked by your actor:

```go
func (mm *MyMM) onAccepted(e actor.OrderAcceptedEvent) {
    sym, ok := mm.reqToSym[e.RequestID]
    if !ok {
        // Late accept — request was already cleared by cancel-all.
        // Cancel this orphan immediately.
        mm.CancelOrder(e.OrderID)
        return
    }
    // ... normal tracking ...
}

func (mm *MyMM) cancelAllForSym(sym string) {
    for orderID := range mm.pending[sym] {
        mm.CancelOrder(orderID)
        delete(mm.pending[sym], orderID)
    }
    // Also clear in-flight request IDs to mark future accepts as orphans
    for reqID, s := range mm.reqToSym {
        if s == sym {
            delete(mm.reqToSym, reqID)
        }
    }
}
```

### Relative vs Absolute Parameters

When simulating multiple assets at different price levels, parameters expressed in absolute terms (tick size, lot size, thresholds) behave inconsistently. A $1 tick is 0.002% of $50,000 but 0.67% of $150. Use per-instrument configuration or express thresholds in basis points.
