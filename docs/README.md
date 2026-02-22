# Exchange Simulation Documentation

High-performance exchange simulator for spot and perpetual futures markets. Built in Go with deterministic simulation support for backtesting trading strategies.

## Prerequisites

- Go 1.21+
- Python 3.8+ (for analysis tools)
- Make

## Quick Navigation

### Getting Started
- [First Simulation](quickstart/01-first-simulation.md) - Build and run your first simulation
- [Random Walk Example](quickstart/02-randomwalk-example.md) - Walkthrough of the complete randomwalk_v2 example
- [Creating Actors](quickstart/03-creating-actors.md) - Build custom trading actors

### Core Concepts
- [Exchange Architecture](core-concepts/exchange-architecture.md) - Exchange design, threading model, object pooling
- [Order Matching](core-concepts/order-matching.md) - Price-time-visibility priority, FIFO matching, execution generation
- [Instruments](core-concepts/instruments.md) - Spot vs perpetual futures, precision math
- [Positions and Margin](core-concepts/positions-and-margin.md) - Position tracking, PnL calculation, leverage
- [Funding Rates](core-concepts/funding-rates.md) - Perpetual futures funding mechanism with formulas
- [Balance Snapshots](core-concepts/balance-snapshots.md) - Complete balance state across all wallet types

### Actor System
- [Actor System](actors/actor-system.md) - BaseActor, CompositeActor, SubActor, SharedContext
- [Market Makers](actors/market-makers.md) - SlowMarketMaker, PureMMSubActor, AvellanedaStoikov, SpreadModel
- [Takers](actors/takers.md) - RandomizedTaker, RandomTakerSubActor, InformedTrader
- [Arbitrage](actors/arbitrage.md) - InternalFundingArb, TriangleArbitrage
- [Microstructure Patterns](actors/microstructure-patterns.md) - SimTicker constraints, timer-based MMs, EMA on trades, anti-patterns

### Simulation Infrastructure
- [Simulated Time](simulation/simulated-time.md) - Clock abstraction, event scheduling, time compression
- [Ticker Factories](simulation/ticker-factories.md) - Real vs simulated time tickers
- [Price Processes](simulation/price-processes.md) - GBMProcess, latency models

### Observability
- [Logging System](observability/logging-system.md) - NDJSON event logging, per-symbol loggers
- [Balance Tracking](observability/balance-tracking.md) - Balance deltas and snapshots
- [Analysis Tools](observability/analysis-tools.md) - Python scripts for log analysis

## Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│                    Exchange Core                         │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │ Order Books  │  │  Positions   │  │  Balances    │  │
│  │  (per sym)   │  │   Manager    │  │   Manager    │  │
│  └──────────────┘  └──────────────┘  └──────────────┘  │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │   Matching   │  │   Funding    │  │   Logging    │  │
│  │    Engine    │  │  Calculator  │  │   System     │  │
│  └──────────────┘  └──────────────┘  └──────────────┘  │
└─────────────────────────────────────────────────────────┘
           ▲                    ▲                    ▲
           │                    │                    │
    ┌──────┴──────┐      ┌──────┴──────┐      ┌──────┴──────┐
    │   Gateway   │      │   Gateway   │      │   Gateway   │
    │  (Client 1) │      │  (Client 2) │      │  (Client N) │
    └──────┬──────┘      └──────┬──────┘      └──────┬──────┘
           │                    │                    │
    ┌──────▼──────┐      ┌──────▼──────┐      ┌──────▼──────┐
    │ Market Maker│      │    Taker    │      │ Arbitrageur │
    │    Actor    │      │    Actor    │      │    Actor    │
    └─────────────┘      └─────────────┘      └─────────────┘
```

## Key Features

**Matching Engine**
- Price-time-visibility priority
- FIFO ordering within price levels
- Self-trade prevention
- Iceberg and hidden order support
- O(1) best bid/ask access

**Perpetual Futures**
- Mark price calculation (last, mid, weighted-mid)
- Index price providers (spot-based, fixed, custom)
- Funding rate mechanism with configurable formula
- Cross-margin and isolated-margin modes
- Automatic liquidation

**Simulation**
- Deterministic simulated time
- Event-driven scheduling
- 100x+ time compression
- Same code for backtest and production
- Ticker abstraction (real vs simulated)

**Observability**
- NDJSON event logs
- Per-symbol and global loggers
- Balance snapshots and deltas
- Complete audit trail
- Python analysis tools

## Common Use Cases

**Strategy Backtesting**
```bash
# Run historical simulation with custom actors
go run cmd/my_strategy/main.go
```

**Market Microstructure Research**
```bash
# Run random walk simulation and analyze order flow
make build
./bin/randomwalk_v2
python scripts/analyze_detailed.py logs/randomwalk_v2
```

**Trading Algorithm Development**
- Implement custom Actor interface
- Test against simulated market makers
- Validate PnL and position tracking
- Measure strategy performance

**Exchange Mechanics Testing**
- Funding rate behavior
- Liquidation cascades
- Fee impact on profitability
- Margin utilization patterns

## Project Structure

```
exchange_simulation/
├── actor/              # Actor interface and base implementation
├── exchange/           # Exchange core (matching, positions, funding)
├── simulation/         # Clock, scheduler, ticker factories
├── realistic_sim/      # Concrete actor implementations
│   └── actors/         # Market makers, takers, arbitrageurs
├── logger/             # Event logging
├── cmd/                # Executable simulations
│   ├── randomwalk_v2/  # Random walk example
│   └── ...
├── scripts/            # Python analysis tools
└── docs/               # This documentation
```

## Building

```bash
make build          # Build all binaries
make test           # Run tests
make coverage-html  # View coverage report
```

See [BUILD.md](../BUILD.md) for detailed build instructions.
