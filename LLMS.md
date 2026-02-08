# Exchange Simulation - Complete Reference for LLMs

**Version**: 1.0
**Last Updated**: 2025
**Purpose**: Comprehensive single-file reference for AI assistants working with this codebase

## Table of Contents

1. [Project Overview](#project-overview)
2. [Architecture & Design](#architecture--design)
3. [Quick Start](#quick-start)
4. [Core Concepts](#core-concepts)
5. [Precision & Arithmetic](#precision--arithmetic)
6. [Actor Framework](#actor-framework)
7. [Perpetual Futures](#perpetual-futures)
8. [Automated Exchange](#automated-exchange)
9. [Order Rejection Handling](#order-rejection-handling)
10. [Logging & Data Recording](#logging--data-recording)
11. [Building & Testing](#building--testing)
12. [Package Structure](#package-structure)

---

## Project Overview

### What This Is

A **high-performance, event-driven cryptocurrency exchange simulation** in Go, designed for:
- Algorithmic trading strategy development and testing
- Market microstructure research
- Multi-venue latency arbitrage simulation
- Order book dynamics analysis
- Perpetual futures with funding mechanisms

### Design Philosophy

**Library-First Architecture**:
- Core exchange provides primitives
- Users extend via composition and dependency injection
- No modification of library code required for extensions
- Configuration over hard-coding
- Scripts/commands are thin adapters over library functions

**Key Principles**:
- Integer arithmetic (no floating-point)
- Event-driven actor model
- Clock abstraction (real-time and simulated)
- Zero external dependencies for core exchange
- Flat package structure for simplicity

### Goals

✅ **Production-Grade Simulation**: Industry-standard matching engine, funding, position tracking
✅ **Research Platform**: Full L3 orderbook reconstruction from logs
✅ **Educational**: Clear, documented, testable code
✅ **Extensible**: Add instruments, strategies, venues without modifying library
✅ **Fast**: Efficient matching, pooling, minimal allocations

---

## Architecture & Design

### System Components

```
┌─────────────────────────────────────────────────────────┐
│                   SIMULATION LAYER                      │
│  - SimulatedClock / RealClock (time abstraction)       │
│  - Simulation Runner (lifecycle management)             │
│  - Latency Providers (network delay simulation)         │
└─────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────┐
│                     ACTOR LAYER                         │
│  - BaseActor (framework for strategies)                 │
│  - Trading Actors (FirstLP, MarketMaker, Takers, etc.)  │
│  - Event Loop (OrderAccepted, Filled, BookSnapshot)     │
└─────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────┐
│                   EXCHANGE CORE                         │
│  ┌───────────────────────────────────────────────────┐  │
│  │  Gateway Layer: ClientGateway (per-client comms)  │  │
│  └───────────────────────────────────────────────────┘  │
│  - Exchange (main coordinator)                          │
│  - Order Books (limit tree, matching engine)            │
│  - Position Manager (perp futures positions)            │
│  - Market Data Publisher (events to subscribers)        │
│  - Instruments (Spot, PerpFutures)                      │
│  - Fee Models (Fixed, Percentage)                       │
└─────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────┐
│                     STORAGE                             │
│  - JSONL Logs (struct tags → JSON marshalling)         │
│  - Python Analysis (Polars for visualization)           │
└─────────────────────────────────────────────────────────┘
```

### Data Flow: Order Submission

```
Actor → BaseActor.SubmitOrder()
  → ClientGateway.RequestCh (channel)
    → Exchange.handleClientRequests()
      → Validate (balance, price, qty, instrument)
      → OrderBook.Match()
        → IF crosses: Execute, settle, publish trade
        → IF rests: Add to limit, publish delta
      → ClientGateway.ResponseCh (channel)
        → BaseActor.handleResponse()
          → IF success: EventOrderAccepted
          → IF failure: EventOrderRejected
            → Actor.OnEvent() → handle rejection, retry
```

### Data Flow: Market Data

```
OrderBook.Match() → Trade execution
  → MarketDataPublisher.PublishTrade()
    → ClientGateway.MarketData (channel, broadcast)
      → Actor.handleMarketData()
        → Actor.OnEvent(EventTrade)
          → Actor strategy logic
```

### Event Types (10 Total)

**Order Lifecycle (6)**:
1. `EventOrderAccepted` - Order placed successfully
2. `EventOrderRejected` - Order rejected (balance, price, qty, etc.)
3. `EventOrderPartialFill` - Order partially filled
4. `EventOrderFilled` - Order fully filled
5. `EventOrderCancelled` - Order cancelled successfully
6. `EventOrderCancelRejected` - Cancel failed (already filled, not found, etc.)

**Market Data (3)**:
7. `EventTrade` - Trade executed
8. `EventBookDelta` - Order book level changed
9. `EventBookSnapshot` - Full order book snapshot

**Perpetual Futures (1)**:
10. `EventFundingUpdate` - Funding rate updated

### Clock Abstraction

**Why**: Allows deterministic, fast-forward simulation.

```go
type Clock interface {
    NowUnixNano() int64
}

// RealClock - Production, real wall-clock time
type RealClock struct{}
func (c *RealClock) NowUnixNano() int64 { return time.Now().UnixNano() }

// SimulatedClock - Testing, controllable time
type SimulatedClock struct {
    time int64
}
func (c *SimulatedClock) Advance(d time.Duration) { c.time += int64(d) }
```

**Usage**: Exchange, Matcher, PositionManager all use `Clock` interface, never `time.Now()` directly.

---

## Quick Start

### Build & Run

```bash
# Build all binaries
make build

# Run main simulation (10 seconds, multiple exchanges)
./bin/multisim

# Run industrial simulation (months of sim time)
./bin/industrial

# Check logs
ls logs/

# Run tests
make test

# View coverage
make coverage-html
```

### Minimal Example

```go
package main

import (
    "context"
    "time"
    "exchange_sim/exchange"
    "exchange_sim/realistic_sim/actors"
)

func main() {
    // Create exchange with real-time clock
    ex := exchange.NewExchange(100, &exchange.RealClock{})

    // Add spot instrument
    inst := exchange.NewSpotInstrument(
        "BTC/USD", "BTC", "USD",
        exchange.BTC_PRECISION,       // Base: 100,000,000 (8 decimals)
        exchange.USD_PRECISION,       // Quote: 100,000 (5 decimals)
        exchange.DOLLAR_TICK,         // Price tick: $1
        exchange.BTC_PRECISION/1000,  // Min order: 0.00001 BTC
    )
    ex.AddInstrument(inst)

    // Connect client with balances
    balances := map[string]int64{
        "BTC": 10 * exchange.BTC_PRECISION,     // 10 BTC
        "USD": 100000 * exchange.USD_PRECISION, // $100,000
    }
    gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

    // Create first liquidity provider
    lp := actors.NewFirstLP(1, gateway, actors.FirstLPConfig{
        Symbol:          "BTC/USD",
        HalfSpreadBps:   50,  // 0.5% half-spread = 1% total spread
        BootstrapPrice:  exchange.PriceUSD(50000, exchange.DOLLAR_TICK),
    })
    lp.SetInitialState(inst)
    lp.UpdateBalances(balances["BTC"], balances["USD"])

    // Start actor
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    lp.Start(ctx)
    defer lp.Stop()

    // Simulation runs...
    <-ctx.Done()
    ex.Shutdown()
}
```

---

## Core Concepts

### Instruments

**Spot Instrument**:
- Direct ownership of assets
- Buy 1 BTC → +1 BTC balance, -$50k USD balance
- No positions tracked, only balances
- No funding payments

**Perpetual Futures**:
- Contract representing exposure to asset
- Position tracking (long/short/flat)
- Funding payments every 8 hours
- Never expires
- Leverage possible (not enforced by default)

```go
// Spot
spot := exchange.NewSpotInstrument(
    "BTC/USD", "BTC", "USD",
    basePrecision, quotePrecision, tickSize, minOrderSize,
)

// Perpetual
perp := exchange.NewPerpFutures(
    "BTC-PERP", "BTC", "USD",
    basePrecision, quotePrecision, tickSize, minOrderSize,
)
```

### Order Types

**Limit Order**:
- Specify price and quantity
- Rests on book if doesn't cross (maker)
- Crosses immediately if matches (taker)

**Market Order**:
- No price specified (use 0)
- Always crosses immediately (taker)
- Rejects if no liquidity available

```go
// Limit order - may be maker or taker
actor.SubmitOrder(symbol, exchange.Buy, exchange.LimitOrder, price, qty)

// Market order - always taker
actor.SubmitOrder(symbol, exchange.Buy, exchange.Market, 0, qty)
```

### Maker vs Taker

**Determined by order behavior, NOT actor type**:
- **Maker**: Order rests on book → provides liquidity → pays maker fee (lower/rebate)
- **Taker**: Order crosses spread → removes liquidity → pays taker fee (higher)

Example:
```go
// Best bid: $50,000, Best ask: $50,100

// MAKER: Limit bid at $49,900 (doesn't cross, rests on book)
actor.SubmitOrder(symbol, Buy, LimitOrder, 49900*DOLLAR_TICK, qty)

// TAKER: Limit bid at $50,100 (crosses ask immediately)
actor.SubmitOrder(symbol, Buy, LimitOrder, 50100*DOLLAR_TICK, qty)

// TAKER: Market buy (always crosses)
actor.SubmitOrder(symbol, Buy, Market, 0, qty)
```

### Fee Models

```go
// Fixed fee (e.g., $0.01 per trade)
&exchange.FixedFee{} // Currently zero, but extensible

// Percentage fee (basis points)
&exchange.PercentageFee{
    MakerBps: 2,   // 0.02% for maker orders
    TakerBps: 5,   // 0.05% for taker orders
    InQuote:  true, // Fee deducted in quote asset (USD)
}
```

---

## Precision & Arithmetic

### 🚨 CRITICAL: Never Use SATOSHI for Non-BTC Assets

**SATOSHI is a deprecated legacy constant.** It equals BTC_PRECISION but implies Bitcoin-specific usage.

**The Problem**:
```go
// ❌ DISASTER - wrong precision multipliers
balances := map[string]int64{
    "USD": 100000 * SATOSHI,  // Uses 100,000,000 instead of 100,000
    "ETH": 50 * SATOSHI,       // Uses 100,000,000 instead of 1,000,000
}
// Result: USD balance is 1000x too large, ETH balance is 100x too large!
```

**Always use asset-specific precisions**:
- `BTC_PRECISION` = 100,000,000 (8 decimals)
- `USD_PRECISION` = 100,000 (5 decimals)
- `ETH_PRECISION` = 1,000,000 (6 decimals)

**Rule**: If you type `SATOSHI` anywhere except for backward-compatible constants, you're doing it wrong.

---

### Why Integer Arithmetic?

Floating-point has precision issues:
```python
0.1 + 0.2 == 0.3  # False!
```

Integer arithmetic is exact:
```go
10 + 20 == 30  // Always true
```

### Asset Precisions

All amounts are integers with asset-specific multipliers:

```go
const (
    // Asset precisions - ALWAYS use these
    BTC_PRECISION  = 100_000_000  // 1 BTC = 100M satoshis
    ETH_PRECISION  = 1_000_000    // 1 ETH = 1M micro-ETH
    USD_PRECISION  = 100_000      // 1 USD = 100K units (0.001 USD minimum)

    // ⚠️ DEPRECATED: SATOSHI is a legacy alias - DO NOT USE
    // Use BTC_PRECISION for BTC amounts
    // Use asset-specific precisions (ETH_PRECISION, USD_PRECISION, etc.)
    SATOSHI = BTC_PRECISION  // DEPRECATED - kept only for backward compatibility

    // Price tick sizes (for BTC/USD) - use BTC_PRECISION for new code
    CENT_TICK    = BTC_PRECISION / 100   // $0.01 tick
    DOLLAR_TICK  = BTC_PRECISION         // $1.00 tick
    HUNDRED_TICK = 100 * BTC_PRECISION   // $100 tick
)
```

**❌ NEVER use SATOSHI** - it's BTC-specific and creates confusion:
- ❌ `100000 * SATOSHI` for USD amounts - **WRONG!**
- ❌ `50 * SATOSHI` for ETH amounts - **WRONG!**
- ❌ Mixing SATOSHI with non-BTC assets - **WRONG!**

**✅ ALWAYS use asset-specific precisions**:
- ✅ `10 * BTC_PRECISION` for BTC
- ✅ `100000 * USD_PRECISION` for USD
- ✅ `50 * ETH_PRECISION` for ETH

### Precision Rules

| Asset | Precision | 1 Unit | Example | Integer Value |
|-------|-----------|--------|---------|---------------|
| BTC | 100,000,000 | 1 satoshi | 10 BTC | 1,000,000,000 |
| USD | 100,000 | 0.001 USD | $100,000 | 10,000,000,000 |
| ETH | 1,000,000 | 1 micro-ETH | 100 ETH | 100,000,000 |

### Helper Functions

```go
// Convert float to integer (TEST ONLY - not for production)
func BTCAmount(btc float64) int64 {
    return int64(btc * float64(BTC_PRECISION))
}
// BTCAmount(1.5) = 150,000,000

func USDAmount(usd float64) int64 {
    return int64(usd * float64(USD_PRECISION))
}
// USDAmount(50000.0) = 5,000,000,000

func PriceUSD(price float64, tickSize int64) int64 {
    raw := int64(price * float64(BTC_PRECISION))
    return (raw / tickSize) * tickSize
}
// PriceUSD(50000.0, DOLLAR_TICK) = 5,000,000,000,000
```

### Correct Balance Initialization

❌ **WRONG - Never use SATOSHI**:
```go
balances := map[string]int64{
    "BTC": 10 * exchange.SATOSHI,      // WRONG! SATOSHI is deprecated
    "USD": 100000 * exchange.SATOSHI,  // WRONG! USD ≠ BTC precision
    "ETH": 50 * exchange.SATOSHI,      // WRONG! ETH ≠ BTC precision
}
```

✅ **CORRECT - Use asset-specific precisions**:
```go
balances := map[string]int64{
    "BTC": 10 * exchange.BTC_PRECISION,      // 10 BTC
    "USD": 100000 * exchange.USD_PRECISION,  // $100,000
    "ETH": 50 * exchange.ETH_PRECISION,      // 50 ETH
}
```

**Why this matters**:
- BTC_PRECISION = 100,000,000 (8 decimals)
- USD_PRECISION = 100,000 (5 decimals, $0.00001 minimum)
- ETH_PRECISION = 1,000,000 (6 decimals)

Using SATOSHI for non-BTC assets causes **severe calculation errors**.

### Instrument Configuration

```go
// Spot BTC/USD
instrument := exchange.NewSpotInstrument(
    "BTC/USD",
    "BTC",
    "USD",
    exchange.BTC_PRECISION,     // Base precision (BTC) - 100,000,000
    exchange.USD_PRECISION,     // Quote precision (USD) - 100,000
    exchange.DOLLAR_TICK,       // Tick size ($1) = BTC_PRECISION
    exchange.BTC_PRECISION/1000, // Min order size (0.00001 BTC)
)

// Spot ETH/USD
instrument := exchange.NewSpotInstrument(
    "ETH/USD",
    "ETH",
    "USD",
    exchange.ETH_PRECISION,     // Base precision (ETH) - 1,000,000
    exchange.USD_PRECISION,     // Quote precision (USD) - 100,000
    exchange.DOLLAR_TICK,       // Tick size ($1)
    exchange.ETH_PRECISION/100,  // Min order size (0.01 ETH)
)
```

**🚨 CRITICAL**:
- Base and quote precisions are **DIFFERENT for each asset**
- **NEVER use SATOSHI** - it's BTC-specific (100,000,000)
- USD_PRECISION (100,000) ≠ BTC_PRECISION (100,000,000) ≠ ETH_PRECISION (1,000,000)
- Using wrong precision causes **catastrophic calculation errors**

**Pattern to follow**:
```go
// ✅ CORRECT pattern for any asset
{ASSET}_PRECISION  // Always use the asset-specific constant
```

### Precision Cheat Sheet

| Asset | Constant | Value | Decimals | 1 Unit |
|-------|----------|-------|----------|--------|
| BTC | `BTC_PRECISION` | 100,000,000 | 8 | 1 satoshi |
| ETH | `ETH_PRECISION` | 1,000,000 | 6 | 1 micro-ETH |
| USD | `USD_PRECISION` | 100,000 | 5 | $0.00001 |

**Memory Aid**:
```go
// ✅ DO THIS
amount := quantity * BTC_PRECISION  // For BTC
amount := quantity * ETH_PRECISION  // For ETH
amount := quantity * USD_PRECISION  // For USD

// ❌ NEVER DO THIS
amount := quantity * SATOSHI  // WRONG - only for BTC, deprecated anyway
```

**Common Mistakes to Avoid**:
1. ❌ Using SATOSHI for any calculations
2. ❌ Mixing precisions (e.g., USD amount with BTC_PRECISION)
3. ❌ Hardcoding precision values instead of using constants
4. ❌ Assuming all assets have 8 decimals like Bitcoin

**When in doubt**: Match the precision constant to the asset name. `BTC` → `BTC_PRECISION`, `USD` → `USD_PRECISION`, `ETH` → `ETH_PRECISION`.

---

## Actor Framework

### Actor Model

All trading strategies extend `BaseActor`:

```go
type MyActor struct {
    *actor.BaseActor
    // Strategy-specific state
    symbol string
    position int64
    activeOrders map[uint64]bool
}

func (a *MyActor) Start(ctx context.Context) error {
    a.Subscribe(a.symbol)
    go a.eventLoop(ctx)
    return a.BaseActor.Start(ctx)
}

func (a *MyActor) eventLoop(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        case event := <-a.EventChannel():
            a.OnEvent(event)
        }
    }
}

func (a *MyActor) OnEvent(event *actor.Event) {
    switch event.Type {
    case actor.EventOrderAccepted:
        // Track order
    case actor.EventOrderFilled:
        // Update position
    case actor.EventBookSnapshot:
        // Analyze market, place orders
    }
}
```

### BaseActor Provides

- **Order submission**: `SubmitOrder()`, `CancelOrder()`
- **Balance queries**: `QueryBalance()`
- **Market data**: `Subscribe(symbol)`, event routing
- **Request tracking**: Automatic request ID generation
- **Event conversion**: Converts responses to events automatically

### Actor Strategies

**First Liquidity Provider** (`realistic_sim/actors/first_lp.go`):
- Bootstraps empty markets
- Places initial bid/ask with wide spread
- Exits position when sufficient competing liquidity

**Pure Market Maker** (`realistic_sim/actors/pure_market_maker.go`):
- Two-sided quoting around mid-price
- Inventory management (skews quotes when imbalanced)
- One-sided quoting at inventory limits

**Avellaneda-Stoikov Market Maker** (`realistic_sim/actors/avellaneda_stoikov.go`):
- Academic market-making model
- Dynamic spread based on volatility and inventory
- Risk-averse quoting

**Funding Arbitrage** (`realistic_sim/actors/funding_arb.go`):
- Long spot + short perp (or vice versa) to capture funding
- Enters when funding rate exceeds threshold
- Exits when funding normalizes

**Randomized Taker** (`realistic_sim/actors/randomized_taker.go`):
- Random market/limit orders
- Creates realistic order flow

**Noisy Trader** (`actor/noisy_trader.go`):
- Places limit orders around mid-price
- Manages max active orders
- Cancels stale orders

### Example: Simple Taker

```go
type SimpleTaker struct {
    *actor.BaseActor
    symbol      string
    targetPrice int64
}

func (a *SimpleTaker) OnEvent(event *actor.Event) {
    switch event.Type {
    case actor.EventBookSnapshot:
        snap := event.Data.(actor.BookSnapshotEvent)
        if len(snap.Snapshot.Asks) > 0 {
            bestAsk := snap.Snapshot.Asks[0].Price
            if bestAsk <= a.targetPrice {
                // Buy with market order (taker)
                a.SubmitOrder(a.symbol, exchange.Buy, exchange.Market, 0, qty)
            }
        }

    case actor.EventOrderFilled:
        fill := event.Data.(actor.OrderFillEvent)
        // React to fill

    case actor.EventOrderRejected:
        rejection := event.Data.(actor.OrderRejectedEvent)
        // Handle rejection (CRITICAL - see next section)
    }
}
```

---

## Order Rejection Handling

### Critical Concept

**Rejections are NORMAL, not errors**. Actors MUST handle them.

### All Rejection Reasons (10 Total)

**Order Submission (7)**:
- `RejectInsufficientBalance` (0) - Not enough balance
- `RejectInvalidPrice` (1) - Price not multiple of tick size
- `RejectInvalidQty` (2) - Quantity below minimum
- `RejectUnknownClient` (3) - Client not connected
- `RejectUnknownInstrument` (4) - Symbol doesn't exist
- `RejectSelfTrade` (5) - Would match own order
- `RejectDuplicateOrderID` (6) - OrderID collision

**Order Cancellation (3)**:
- `RejectOrderNotFound` (7) - Order doesn't exist
- `RejectOrderAlreadyFilled` (8) - Order completed before cancel
- `RejectOrderAlreadyCancelled` (9) - Order already cancelled

### Handling Strategy

```go
func (a *MyActor) OnEvent(event *actor.Event) {
    switch event.Type {
    case actor.EventOrderRejected:
        rejection := event.Data.(actor.OrderRejectedEvent)

        switch rejection.Reason {
        case exchange.RejectInsufficientBalance:
            // Wait for fills to free up capital
            a.pauseTrading = true

        case exchange.RejectInvalidPrice:
            // Recalculate with valid tick size
            validPrice := (rejection.Price / a.tickSize) * a.tickSize
            a.SubmitOrder(a.symbol, rejection.Side, exchange.LimitOrder, validPrice, rejection.Qty)

        case exchange.RejectSelfTrade:
            // Cancel conflicting order or skip
            a.CancelOrder(a.symbol, a.conflictingOrderID)

        default:
            // Log and skip
        }
    }
}
```

**Anti-patterns**:
- ❌ Ignoring rejections silently
- ❌ Retrying infinitely without fixing root cause
- ❌ Treating rejections as fatal errors

**Best practices**:
- ✅ Handle each rejection reason explicitly
- ✅ Fix root cause before retry
- ✅ Use exponential backoff for retries
- ✅ Log rejections for debugging

---

## Logging & Data Recording

### Current System: JSONL with Struct Tags

**All logging uses the `logger` package** which serializes events to JSONL (JSON Lines) format using struct tags.

#### Logger Interface

```go
type Logger struct {
    w  io.Writer
    mu sync.Mutex
}

// LogEvent serializes any struct with JSON tags
func (l *Logger) LogEvent(simTime int64, clientID uint64, eventName string, event any)
```

#### How It Works

1. Define events with JSON tags:
```go
type TradeEvent struct {
    Symbol    string `json:"symbol"`
    Price     int64  `json:"price"`
    Quantity  int64  `json:"quantity"`
    Side      Side   `json:"side"`
    IsMaker   bool   `json:"is_maker"`
}
```

2. Log events:
```go
logger.LogEvent(simTime, clientID, "trade", TradeEvent{
    Symbol:   "BTC/USD",
    Price:    50000 * exchange.DOLLAR_TICK,
    Quantity: exchange.BTC_PRECISION,
    Side:     exchange.Buy,
    IsMaker:  false,
})
```

3. Output (JSONL):
```jsonl
{"sim_time":1000000000,"server_time":1738953600000000000,"event":"trade","client_id":1,"symbol":"BTC/USD","price":5000000000000,"quantity":100000000,"side":"Buy","is_maker":false}
```

#### Key Features

- **Struct tags drive serialization**: Add `json:"field_name"` tags to control output
- **Type-safe**: Compile-time checking of event structures
- **Efficient**: Direct JSON marshalling, no intermediate formats
- **Extensible**: Add fields by updating struct definitions
- **Thread-safe**: Mutex-protected writes

#### Analysis with Python/Polars

```python
import polars as pl

# Read JSONL logs
df = pl.read_ndjson("simulation.log")

# Filter trades
trades = df.filter(pl.col("event") == "trade")

# Analyze
trades.group_by("symbol").agg([
    pl.col("quantity").sum().alias("total_volume"),
    pl.col("price").mean().alias("avg_price"),
])
```

### Deprecated: CSV Recording

**⚠️ CSV logging is deprecated and should not be used.**

Historical references to CSV in documentation are outdated:
- Old `Recorder` actor (removed)
- CSV file formats in MARKET_DATA_RECORDING.md (deprecated)
- ARCHITECTURE.md diagrams showing CSV (outdated)

**Migration path**:
- Use `logger.Logger` with struct tags for all new logging
- Existing CSV analysis scripts should read JSONL instead
- Polars handles JSONL efficiently with `read_ndjson()`

**Why JSONL over CSV**:
- **Schema evolution**: Add fields without breaking readers
- **Type safety**: Preserve types (no string conversion)
- **Nested data**: Support complex structures
- **Standard tooling**: Polars, jq, etc. handle JSONL natively
- **No escaping issues**: JSON handles special characters correctly

### Example: Logging Custom Events

```go
// Define event structure
type PositionUpdateEvent struct {
    Symbol       string `json:"symbol"`
    PositionQty  int64  `json:"position_qty"`
    AvgEntryPrice int64 `json:"avg_entry_price"`
    UnrealizedPnL int64 `json:"unrealized_pnl"`
}

// Log from actor
func (a *Actor) logPosition() {
    a.Logger.LogEvent(
        a.Clock.NowUnixNano(),
        a.ClientID,
        "position_update",
        PositionUpdateEvent{
            Symbol:        a.symbol,
            PositionQty:   a.position,
            AvgEntryPrice: a.avgEntry,
            UnrealizedPnL: a.calculatePnL(),
        },
    )
}
```

**Best practices**:
- Keep event structs focused (single responsibility)
- Use consistent naming across events
- Include timestamp context (sim_time, server_time)
- Add client_id for multi-actor simulations
- Document non-obvious field semantics in comments

---

## Building & Testing

### Build System

See `BUILD.md` for complete documentation.

```bash
make build          # Build all binaries
make test           # Run tests
make coverage-html  # View coverage
make clean          # Clean artifacts
```

### Running Simulations

```bash
# Multi-exchange simulation (10 seconds)
./bin/multisim

# Industrial-scale simulation (months of sim time)
./bin/industrial

# Check generated logs
ls logs/
```

### Testing Strategy

- **Unit tests**: Core exchange logic (matching, positions, fees)
- **Integration tests**: Actor interactions, event flows
- **Property tests**: Invariants (balance conservation, position consistency)

Always run `make test` before committing.

---

## Package Structure

```
exchange_simulation/
├── exchange/           # Core exchange engine
│   ├── exchange.go     # Main coordinator
│   ├── orderbook.go    # Limit order book
│   ├── position.go     # Position manager (perps)
│   ├── instrument.go   # Spot/Perp definitions
│   └── fee.go          # Fee models
│
├── actor/              # Actor framework
│   ├── base_actor.go   # Base actor implementation
│   ├── event.go        # Event types
│   └── strategies/     # Example strategies
│
├── realistic_sim/      # Simulation components
│   ├── actors/         # Trading actor implementations
│   │   ├── first_lp.go
│   │   ├── pure_market_maker.go
│   │   ├── avellaneda_stoikov.go
│   │   ├── funding_arb.go
│   │   └── randomized_taker.go
│   ├── simulation.go   # Simulation runner
│   └── clock.go        # Time abstraction
│
├── logger/             # Logging system
│   └── logger.go       # JSONL event logger
│
├── cmd/                # Binary entry points
│   ├── multisim/       # Multi-exchange demo
│   └── industrial/     # Large-scale simulation
│
└── logs/               # Output directory (gitignored)
```

**Design principles**:
- `exchange/` has no dependencies (pure library)
- `actor/` depends only on `exchange/`
- `realistic_sim/` composes actors and exchange
- `cmd/` are thin adapters over library functions

---