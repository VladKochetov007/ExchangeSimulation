# Exchange Simulation

A high-performance, feature-complete cryptocurrency exchange simulation written in Go. Designed for strategy backtesting, market microstructure research, and algorithmic trading development.

## Features

✅ **Industry-Standard Exchange Operations**
- Automatic mark/index price calculation
- Automatic funding rate updates (every 3 seconds)
- Automatic funding settlement (every 8 hours)
- Actors just trade - everything else is automatic!

✅ **Order Types & Matching**
- Limit orders (maker/taker)
- Market orders
- Iceberg orders (hidden liquidity)
- Price-time priority matching

✅ **Instruments**
- Spot trading
- Perpetual futures with funding
- Position netting (one position per instrument)
- Configurable precision per asset

✅ **Actor System**
- Event-driven architecture
- 10 event types (orders, fills, rejections, funding, market data)
- BaseActor framework for strategy development
- Market makers, takers, and mixed strategies

✅ **Multi-Venue Support**
- Latency arbitrage across venues
- Configurable latency models (constant, uniform, normal)
- Cross-venue trading strategies

✅ **Data Recording**
- CSV output (trades, orderbook snapshots, funding)
- Non-blocking writes with buffered channels
- Graceful shutdown with buffer draining

✅ **Simulation Infrastructure**
- Clock abstraction (RealClock, SimulatedClock)
- Deterministic testing with simulated time
- Latency simulation

## Quick Start

### Basic Setup (Manual Mode)

```go
package main

import (
    "exchange_sim/exchange"
    "exchange_sim/actor"
)

func main() {
    // Create exchange
    ex := exchange.NewExchange(100, &exchange.RealClock{})

    // Add instrument
    inst := exchange.NewSpotInstrument(
        "BTC/USD", "BTC", "USD",
        exchange.SATOSHI,      // basePrecision
        exchange.SATOSHI/1000, // quotePrecision
        exchange.DOLLAR_TICK,  // tickSize
        exchange.SATOSHI/100,  // minOrderSize
    )
    ex.AddInstrument(inst)

    // Connect client
    balances := map[string]int64{
        "BTC": 10 * exchange.SATOSHI,
        "USD": 100000 * (exchange.SATOSHI / 1000),
    }
    gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

    // Create actor
    mm := actor.NewMarketMaker(1, gateway, actor.MarketMakerConfig{
        Symbol:    "BTC/USD",
        SpreadBps: 20,
        QuoteQty:  exchange.SATOSHI,
    })

    // Start trading
    ctx := context.Background()
    mm.Start(ctx)
}
```

### Industry-Standard Automated Mode

```go
// Add spot (for index price)
ex.AddInstrument(exchange.NewSpotInstrument("BTC/USD", ...))

// Add perpetual
ex.AddInstrument(exchange.NewPerpFutures("BTC-PERP", ...))

// Setup automation
indexProvider := exchange.NewSpotIndexProvider(ex)
indexProvider.MapPerpToSpot("BTC-PERP", "BTC/USD")

automation := exchange.NewExchangeAutomation(ex, exchange.AutomationConfig{
    MarkPriceCalc:       exchange.NewMidPriceCalculator(),
    IndexProvider:       indexProvider,
    PriceUpdateInterval: 3 * time.Second,
})

// Start and forget!
automation.Start(ctx)

// Now actors just trade - everything automatic! 🎉
```

See [`cmd/automated_exchange/main.go`](cmd/automated_exchange/main.go) for a complete example.

## Documentation

### Core Guides

| Document | Description |
|----------|-------------|
| **[ARCHITECTURE.md](ARCHITECTURE.md)** | System architecture, data flow, component diagrams |
| **[AUTOMATED_EXCHANGE.md](AUTOMATED_EXCHANGE.md)** | Industry-standard automatic exchange mode |
| **[POSITION_NETTING_AND_FUNDING.md](POSITION_NETTING_AND_FUNDING.md)** | Position netting, funding mechanism, mark/index prices |
| **[PRECISION_GUIDE.md](PRECISION_GUIDE.md)** | Integer arithmetic, precision constants, balance calculations |
| **[REJECTION_HANDLING.md](REJECTION_HANDLING.md)** | Order/cancel rejection handling, retry strategies |

### Advanced Topics

| Document | Description |
|----------|-------------|
| **[MULTI_VENUE_LATENCY_ARBITRAGE.md](MULTI_VENUE_LATENCY_ARBITRAGE.md)** | Cross-venue arbitrage, latency simulation |
| **[MARKET_DATA_RECORDING.md](MARKET_DATA_RECORDING.md)** | CSV recording, data collection patterns |
| **[actor/EXAMPLES.md](actor/EXAMPLES.md)** | Actor strategy examples (makers, takers, mixed) |

### Project Setup

| File | Description |
|------|-------------|
| **[CLAUDE.md](CLAUDE.md)** | Development guidelines and project philosophy |

## Architecture Overview

```
exchange_simulation/
├── exchange/              # Core exchange (flat structure)
│   ├── types.go          # Enums, structs
│   ├── order.go          # Order linking
│   ├── book.go           # Order book
│   ├── matching.go       # Matching engine
│   ├── exchange.go       # Main exchange
│   ├── gateway.go        # Communication
│   ├── funding.go        # Funding rates & position tracking
│   ├── instrument.go     # Instruments & price calculators
│   ├── automation.go     # Automated exchange operations
│   ├── price_calculator.go  # Mark price calculators
│   └── index_provider.go    # Index price providers
│
├── actor/                # Actor framework
│   ├── events.go         # Event types (10 total)
│   ├── actor.go          # BaseActor
│   ├── marketmaker.go    # Market maker strategy
│   └── recorder.go       # Data recorder
│
├── simulation/           # Simulation infrastructure
│   ├── clock.go          # Clock abstraction
│   ├── latency.go        # Latency simulation
│   ├── latency_arbitrage.go  # Multi-venue arbitrage
│   └── runner.go         # Simulation runner
│
└── cmd/                  # Examples
    ├── automated_exchange/  # Industry-standard automated example
    └── latency_arb/         # Latency arbitrage example
```

## Key Concepts

### Position Netting

The exchange implements **intra-instrument position netting** (industry standard):
- One position per client per symbol
- Long (positive) or Short (negative) or Flat (zero)
- Cannot be simultaneously long and short same instrument
- Matches BitMEX, Binance, Deribit behavior

### Funding Mechanism

Perpetual futures use funding payments to anchor to spot:
- **Mark Price** - Calculated from order book (mid-price, last trade, etc.)
- **Index Price** - Reference price (from spot or external)
- **Premium** = (Mark - Index) / Index
- **Funding Rate** = BaseRate + (Premium × Damping)
- **Settlement** - Every 8 hours, longs pay/receive funding

In automated mode, all of this happens automatically!

### Precision System

All amounts use **integer arithmetic** for precision:
- BTC: `SATOSHI` = 100,000,000 (8 decimals)
- USD: `SATOSHI/1000` = 100,000 (3 decimals)
- Each asset has its own precision constant
- Instruments store their precisions

### Event System

Actors receive 10 event types:
1. `EventOrderAccepted` - Order placed successfully
2. `EventOrderRejected` - Order rejected (10 reject reasons)
3. `EventOrderPartialFill` - Partial fill
4. `EventOrderFilled` - Full fill
5. `EventOrderCancelled` - Cancel successful
6. `EventOrderCancelRejected` - Cancel rejected
7. `EventTrade` - Trade executed
8. `EventBookDelta` - Orderbook update
9. `EventBookSnapshot` - Full orderbook
10. `EventFundingUpdate` - Funding rate update

## Testing

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package
go test -v ./exchange

# Run with race detector
go test -race ./...
```

## Examples

### Running the Automated Exchange Demo

```bash
cd cmd/automated_exchange
go run main.go
```

Output:
```
=== Industry-Standard Automated Exchange Demo ===

✓ Automated exchange started
  - Mark prices auto-calculated from order book
  - Index prices auto-calculated from spot
  - Funding rates auto-updated every 3 seconds
  - Funding settlement auto-scheduled every 8 hours

Running automated exchange for 15 seconds...
Watch for automatic funding rate updates every 3 seconds!

=== Final State ===
Perpetual: BTC-PERP
  Mark Price:  50.00 USD
  Index Price: 49.50 USD
  Funding Rate: 75 bps (0.7500%)
  Premium: 1.0101%
```

### Latency Arbitrage Example

```bash
cd cmd/latency_arb
go run main.go
```

## Performance

- **Matching Engine**: ~100k orders/sec (single-threaded)
- **Memory**: Efficient object pooling for orders and executions
- **Concurrency**: Lock-free where possible, fine-grained locking
- **Simulation**: Handles 1M+ events in backtests

## Design Philosophy

From [CLAUDE.md](CLAUDE.md):

### Library-First Approach
- Core exchange is a **library**, not an application
- Users extend without modifying library code
- Dependency injection, interfaces, configuration
- Open for extension, closed for modification

### Key Principles
- Everything configurable (no hard-coded decisions)
- Users own their strategies
- Testable and deterministic
- Production-quality code

## Contributing

This is a simulation library designed for research and strategy development. Follow the guidelines in [CLAUDE.md](CLAUDE.md) for contributions.

## License

[Add your license here]

## Acknowledgments

Built with industry-standard exchange mechanics matching Binance Futures, BitMEX, and Deribit behavior.
