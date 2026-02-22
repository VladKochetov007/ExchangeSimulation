# Exchange Simulation

A Go library for building cryptocurrency exchange simulations. Designed for market microstructure research, strategy backtesting, and algorithm development.

This is a **library**, not an application. You write actors and wire them to exchanges. The library provides the exchange mechanics, matching engine, position tracking, funding, and simulation infrastructure.

## What the library provides

**Exchange core** — matching engine (price-time-visibility FIFO, pro-rata), order book, position manager (cross/isolated margin, liquidation, PnL), funding settlement, borrowing/margin lending, circuit breakers, NDJSON event logging.

**Simulation layer** — simulated clock, ticker factory abstraction (same actor code works in real-time and simulation), `DelayedGateway` for network latency modeling, multi-venue support via `VenueRegistry` and `MultiVenueGateway`, `MultiExchangeRunner` for full multi-exchange setups.

**Actor framework** — `BaseActor`, `CompositeActor`/`SubActor` for shared-balance strategy groups, `SharedContext` for inter-strategy coordination.

**Reference actors** — 17 concrete implementations in `realistic_sim/actors/`: market makers (Avellaneda-Stoikov, pure MM, multi-symbol LP), takers (random, informed, noisy), arbitrageurs (funding arb, triangle, latency arb), directional (momentum, cross-sectional mean reversion).

## Minimal example

```go
clock := &exchange.RealClock{}
ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{Clock: clock})

ex.AddInstrument(exchange.NewPerpFutures(
    "BTC-PERP", "BTC", "USD",
    exchange.BTC_PRECISION, exchange.USD_PRECISION,
    exchange.DOLLAR_TICK, exchange.SATOSHI/100,
))

gw := ex.ConnectClient(1, map[string]int64{
    "USD": 100_000 * exchange.USD_PRECISION,
}, &exchange.PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true})

// implement the Actor interface — OnEvent, Start, Stop, ID, Gateway
actor := mypackage.NewMyActor(1, gw, mypackage.Config{...})
actor.Start(context.Background())
```

## Multi-venue example

```go
registry := simulation.NewVenueRegistry()
registry.Register("fast", fastExchange)
registry.Register("slow", slowExchange)

mgw := simulation.NewMultiVenueGateway(clientID, registry,
    map[simulation.VenueID]map[string]int64{
        "fast": {"USD": 50_000 * exchange.USD_PRECISION},
        "slow": {"USD": 50_000 * exchange.USD_PRECISION},
    },
    nil, // fee plans, nil = zero fees
)
mgw.Start()

// Your actor reads from mgw.ResponseCh() and mgw.MarketDataCh(),
// both tagged with VenueID. Submits via mgw.SubmitOrder(venue, req).
actor := mypackage.NewCrossVenueActor(clientID, mgw, mypackage.Config{...})
actor.Start(context.Background())
```

## Extension points

Every non-trivial behavior is injectable:

| Interface | Purpose |
|-----------|---------|
| `MatchingEngine` | FIFO, pro-rata, or custom priority |
| `CircuitBreaker` / `HaltEvaluator` | Price band halts, tiered breakers |
| `FundingCalculator` | Custom funding formula |
| `MarkPriceCalculator` | Last, mid, weighted-mid, Binance, BitMEX, Bybit |
| `IndexPriceProvider` | Spot-derived, GBM process, fixed, custom |
| `FeeModel` | Fixed, percentage, tiered, rebate |
| `Clock` / `TickerFactory` | Real-time, simulated, historical replay |
| `LatencyProvider` | Constant, uniform, normal, log-normal |
| `CollateralPriceOracle` | Collateral valuation for margin borrow |
| `Actor` / `SubActor` | Trading strategies |
| `Instrument` | Spot, perp, or custom instrument types |

## Documentation

Full documentation is in [`docs/`](docs/README.md).

## Build

```bash
make build          # Build all binaries to bin/
make test           # Run all tests
make test-race      # Run with race detector
make coverage-html  # View coverage in browser
```
