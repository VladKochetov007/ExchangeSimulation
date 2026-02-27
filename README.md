# Exchange Simulation

A Go library for cryptocurrency exchange simulation. Designed for market microstructure research, strategy backtesting, and multi-actor algorithm development.

This is a **library**, not an application. You write actors and wire them to exchanges. The library provides the exchange mechanics, matching engine, position tracking, funding settlement, and simulation infrastructure.

---

## Architecture

```mermaid
flowchart LR
    subgraph AL["Actors"]
        A1["Actor A"]
        A2["Actor B"]
    end

    subgraph GL["Gateway Layer"]
        DGW["DelayedGateway</br>(optional latency)"]
        CGW["ClientGateway"]
    end

    subgraph EX["Exchange"]
        OB["Order Book</br>(bids / asks)"]
        ME["Matching Engine</br>(PriceTime · ProRata)"]
        ST["Settlement</br>(PnL · fees · margin)"]
        PM["Position Manager</br>(one-way · hedge</br>cross · isolated)"]
        FM["Funding"]
        BM["Borrowing"]
        MDP["Market Data Publisher"]
        AUT["Automation</br>(prices · funding</br>liquidation)"]
        subgraph PS["Price Sources"]
            MP["Mark Price</br>(mid · last · EMA · TWAP)"]
            IP["Index Price</br>(book · static · dynamic)"]
        end
    end

    subgraph SIM["Simulation"]
        VENUE["Venue"]
        RUNNER["Runner"]
        CLK["Clock</br>(Real · Simulated)"]
    end

    A1 -- Request --> DGW
    A2 -- Request --> DGW
    DGW --> CGW
    CGW --> EX
    EX -- "Response / Fill" --> CGW
    CGW --> DGW
    DGW -- "Response / Fill" --> A1
    DGW -- "Response / Fill" --> A2

    OB --> ME
    ME --> ST
    ST --> PM
    ST --> BM
    MP -. mark .-> ST
    MP -. mark .-> AUT
    IP -. index .-> AUT
    AUT --> FM
    FM --> ST
    AUT --> MDP
    AUT -. "LiquidationHandler</br>callback" .-> A1
    AUT -. "LiquidationHandler</br>callback" .-> A2
    AUT --> ME

    MDP -- "book · trades</br>funding · OI" --> CGW

    CLK -.-> EX
    RUNNER --> VENUE
    VENUE --> DGW
```

**Request flow**: Actor sends `Request` → `Gateway` (optionally delayed) → `Exchange.HandleClientRequests` → order matching → `Response` + `FillNotification` back through the same gateway. Market data (book snapshots/deltas, trades, funding rate updates, open interest) flows one-way from `MDPublisher` to all subscribed gateways, filtered by the `MDType` set in the subscription.

**Automation**: `ExchangeAutomation` runs in the background — recalculates mark and index prices, updates funding rates, publishes them via `MDPublisher`, settles periodic funding payments through `Settlement`, and checks margin ratios after every price update. Liquidation force-closes positions through the `MatchingEngine`. Margin call warnings and liquidation events are delivered via the `LiquidationHandler` interface — a direct callback, not routed through the gateway.

**Latency modeling**: `DelayedGateway` wraps any `ClientGateway` and introduces per-channel (request / response / market data) delay drawn from a pluggable `LatencyProvider`. Five implementations ship out of the box: constant, uniform random, normal, log-normal (heavy tail), and Hawkes (self-exciting, models exchange queue congestion). Multiple actors on the same exchange can have independent latency profiles.

**Multi-venue**: `Venue` pairs an `Exchange` with a `LatencyConfig`. `Runner` orchestrates any number of venues and actors, advancing a shared `SimulatedClock` or running in wall-clock time.

---

## Packages

| Package | Contents |
|---------|----------|
| `exchange/` | `Exchange`, `Client`, `ClientGateway`, `PositionManager`, settlement, funding, borrowing, order routing |
| `types/` | Value types: `Order`, `Side`, `FillNotification`, `AssetBalance`, `PositionSnapshot`, `AccountSnapshot`, `FeeModel` interface, enums |
| `book/` | `OrderBook` — price-time ordered bid/ask levels |
| `matching/` | `PriceTimeMatcher` (FIFO), `ProRataMatcher` |
| `price/` | Mark price calculators (last, mid, weighted-mid, Binance, BitMEX, Bybit) and index price providers (spot-derived, GBM, fixed) |
| `instrument/` | `SpotInstrument`, `PerpFutures`, `FundingCalculator` |
| `fee/` | `FixedFee`, `PercentageFee` (maker/taker bps, fee in any asset) |
| `clock/` | `RealClock`, `RealTickerFactory` |
| `marketdata/` | `MDPublisher` — fan-out with per-subscription `MDType` filter |
| `logger/` | NDJSON event logger |
| `simulation/` | `Venue`, `Runner`, `DelayedGateway`, `SimulatedClock`, latency providers, `Scheduler` |
| `actor/` | `Actor` interface, `BaseActor` (order tracking, event dispatch) |

---

## What the library provides

**Exchange core** — price-time FIFO and pro-rata matching engines; order book with iceberg and hidden order support; position manager with cross and isolated margin, one-way and hedge mode, PnL tracking and liquidation; perpetual funding settlement; margin borrowing and interest; NDJSON event logging.

**Account model** — Binance-style balances: `Free / Locked / Borrowed / Interest / NetAsset` for spot and perp wallets. `ReqQueryBalance` returns `BalanceSnapshot`; `ReqQueryAccount` returns the full `AccountSnapshot` including open positions with mark price, unrealized PnL, leverage, and liquidation price.

**Fee model** — `FeeModel.CalculateFee(FillContext)` receives the full execution context and returns `Fee{Amount, Asset}`. Fee asset is arbitrary — BNB-style fee-in-any-asset works out of the box.

**Market data** — `MDPublisher` delivers `MDSnapshot`, `MDDelta`, `MDTrade`, `MDFunding`, `MDOpenInterest` to subscribed gateways. Each subscription carries a type filter; actors receive only the streams they asked for.

**Simulation layer** — `SimulatedClock` with deterministic advancement; `Runner` manages venues and actors with wall-clock duration or iteration count limits; five `LatencyProvider` implementations for network/queue modeling.

**Actor framework** — `BaseActor` handles channel routing, order tracking, and event dispatch. Embed it to build any strategy.

---

## Logging

The exchange uses NDJSON event logging. Each log line is a JSON object with `sim_time`, `server_time`, `event`, `client_id`, plus event-specific fields. Loggers are assigned per key via `exchange.SetLogger(key, logger)`.

### Logger keys

There are two categories of logger: a single `_global` logger for exchange-wide events, and one logger per instrument symbol (`"BTC/USD"`, `"BTC-PERP"`, etc.) for trade and book events.

```
exchange.SetLogger("_global", logger.New(globalFile))   // exchange-wide
exchange.SetLogger("BTC/USD", logger.New(spotFile))     // spot instrument
exchange.SetLogger("BTC-PERP", logger.New(perpFile))    // futures instrument
```

### `_global` events

| Event | Description | Key fields |
|-------|-------------|------------|
| `balance_snapshot` | Periodic snapshot of all client balances (spot + perp) | `client_id`, `spot_balances[]`, `perp_balances[]`, `borrowed{}` |
| `balance_change` | Any wallet mutation with no instrument context (funding settlement, transfers) | `client_id`, `reason`, `changes[]{asset, wallet, old_balance, new_balance, delta}` |
| `fee_revenue` | Exchange fee collected per trade | `symbol`, `trade_id`, `taker_fee`, `maker_fee`, `asset` |
| `realized_pnl` | Perp position close PnL (non-zero only) | `client_id`, `symbol`, `trade_id`, `closed_qty`, `entry_price`, `exit_price`, `pnl`, `side` |
| `position_update` | Every perp position state change | `client_id`, `symbol`, `old_size`, `old_entry_price`, `new_size`, `new_entry_price`, `trade_qty`, `trade_price`, `trade_side`, `reason` |
| `margin_interest` | Periodic interest charged on borrowed amounts | `client_id`, `asset`, `amount` |
| `borrow` | Margin loan taken | `client_id`, `asset`, `amount`, `reason`, `margin_mode`, `interest_rate_bps`, `collateral_used` |
| `repay` | Margin loan repaid | `client_id`, `asset`, `principal`, `interest`, `remaining_debt` |
| `liquidation_check` | Debug: margin ratios when maintenance margin breached | `client_id`, `symbol`, `position_size`, `mark_price`, `equity`, `margin_ratio`, `threshold` |

### Per-symbol events (spot: `"BTC/USD"`, futures: `"BTC-PERP"`)

| Event | Description | Key fields |
|-------|-------------|------------|
| `Trade` | Every matched execution | `trade_id`, `price`, `qty`, `side`, `taker_order_id`, `maker_order_id` |
| `OrderFill` | Per-participant fill record (logged twice: taker and maker) | `order_id`, `symbol`, `qty`, `price`, `side`, `position_side`, `filled_qty`, `remaining_qty`, `is_full`, `trade_id`, `role`, `fee_amount`, `fee_asset`, `realized_pnl`, `new_size`, `new_entry_price` |
| `balance_change` | Spot/perp wallet mutation tied to this trade | `client_id`, `reason: "trade_settlement"`, `changes[]{asset, wallet, old_balance, new_balance, delta}` |
| `BookSnapshot` | Periodic full book state (all price levels) | `bids[]`, `asks[]` |

### Futures-only per-symbol events

| Event | Description | Key fields |
|-------|-------------|------------|
| `mark_price_update` | Recalculated mark and index price | `symbol`, `mark_price`, `index_price` |
| `funding_rate_update` | Updated funding rate | `symbol`, `rate`, `next_funding` |

### Wallet names in `balance_change`

| Wallet | Meaning |
|--------|---------|
| `spot` | Spot balance (`client.Balances`) |
| `perp` | Perp margin balance (`client.PerpBalances`) |
| `reserved_spot` | Spot locked in open orders (`client.Reserved`) |
| `reserved_perp` | Perp locked as order margin (`client.PerpReserved`) |
| `borrowed` | Margin loan outstanding (`client.Borrowed`) |

### Setup example

```go
globalLog := logger.New(globalWriter)
spotLog   := logger.New(spotWriter)
perpLog   := logger.New(perpWriter)

ex.SetLogger("_global", globalLog)
ex.SetLogger("BTC/USD",  spotLog)
ex.SetLogger("BTC-PERP", perpLog)

ex.EnableBalanceSnapshots(5 * time.Second) // periodic balance_snapshot to _global
```

---

## Extension points

Every non-trivial behavior is injectable:

| Interface | Implementations |
|-----------|-----------------|
| `MatchingEngine` | `PriceTimeMatcher` (FIFO), `ProRataMatcher`, custom |
| `FeeModel` | `FixedFee`, `PercentageFee`, custom (any fee asset) |
| `FundingCalculator` | Custom funding formula |
| `MarkPriceCalculator` | Last, mid, weighted-mid, Binance, BitMEX, Bybit |
| `IndexPriceProvider` | Spot-derived, GBM process, fixed, custom |
| `Clock` / `TickerFactory` | `RealClock`, `SimulatedClock`, historical replay |
| `LatencyProvider` | Constant, uniform, normal, log-normal, Hawkes, load-scaled |
| `Actor` | Any trading strategy — embed `BaseActor` |
| `Instrument` | `SpotInstrument`, `PerpFutures`, custom |

---

## Minimal example

```go
ex := exchange.NewExchange(100, &clock.RealClock{})

ex.AddInstrument(exchange.NewSpotInstrument(
    "BTC/USD", "BTC", "USD",
    exchange.BTC_PRECISION, exchange.USD_PRECISION,
    exchange.DOLLAR_TICK, exchange.USD_PRECISION/1000,
))

gw := ex.ConnectClient(1, map[string]int64{
    "BTC": 10 * exchange.BTC_PRECISION,
    "USD": 100_000 * exchange.USD_PRECISION,
}, &fee.PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true})

go ex.HandleClientRequests(gw)

// Implement Actor interface — OnEvent, Start, Stop, ID, Gateway
myActor := mypackage.NewMyActor(1, gw)
myActor.Start(context.Background())
```

## Perpetual futures example

```go
ex := exchange.NewExchange(100, &clock.RealClock{})

perp := exchange.NewPerpFutures(
    "BTC-PERP", "BTC", "USD",
    exchange.BTC_PRECISION, exchange.USD_PRECISION,
    exchange.DOLLAR_TICK, exchange.SATOSHI/100,
)
ex.AddInstrument(perp)

gw := ex.ConnectClient(1, map[string]int64{
    "USD": 100_000 * exchange.USD_PRECISION,
}, &fee.PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true})

// Hedge mode: independent long and short positions
req := &exchange.OrderRequest{
    Symbol:       "BTC-PERP",
    Side:         exchange.Buy,
    Type:         exchange.LimitOrder,
    Price:        exchange.PriceUSD(50000, exchange.DOLLAR_TICK),
    Qty:          exchange.BTC_PRECISION,
    TimeInForce:  exchange.GTC,
    PositionSide: exchange.PositionLong, // hedge mode
}
```

## Multi-venue with latency

```go
fastEx := exchange.NewExchange(100, &clock.RealClock{})
slowEx := exchange.NewExchange(100, &clock.RealClock{})
// ... add instruments to each ...

fastVenue := &simulation.Venue{
    Exchange: fastEx,
    Latency: simulation.LatencyConfig{
        Request:  simulation.NewConstantLatency(1 * time.Millisecond),
        Response: simulation.NewConstantLatency(1 * time.Millisecond),
    },
}
slowVenue := &simulation.Venue{
    Exchange: slowEx,
    Latency: simulation.LatencyConfig{
        Request:  simulation.NewLogNormalLatency(5*time.Millisecond, 10*time.Millisecond, 0.5, 42),
        Response: simulation.NewLogNormalLatency(5*time.Millisecond, 10*time.Millisecond, 0.5, 43),
    },
}

runner := simulation.NewRunner(&clock.RealClock{}, simulation.RunnerConfig{
    Duration: 30 * time.Second,
})
runner.AddVenue(fastVenue)
runner.AddVenue(slowVenue)

fastGW := fastVenue.ConnectClient(1, balances, feePlan)
slowGW := slowVenue.ConnectClient(1, balances, feePlan)

runner.AddActor(mypackage.NewArbitrageActor(1, fastGW, slowGW))
runner.Run(context.Background())
```

**Gateway = venue identity.** An actor that trades on N venues owns N gateways and multiplexes them in a single `select` loop. Which channel delivers the message tells you which exchange it came from — no explicit tagging needed. This works identically for fills, balance queries, market data, and cancel confirmations:

```go
func (a *ArbitrageActor) run(ctx context.Context) {
    for {
        select {
        case resp := <-a.fastGW.Responses():
            a.onFastResponse(resp) // fills, balance snapshots, cancel confirms
        case resp := <-a.slowGW.Responses():
            a.onSlowResponse(resp)
        case md := <-a.fastGW.MarketDataCh():
            a.onFastMarketData(md) // book deltas, trades, funding, OI
        case md := <-a.slowGW.MarketDataCh():
            a.onSlowMarketData(md)
        case <-ctx.Done():
            return
        }
    }
}
```

To query balances on a specific venue: `fastGW.Send(exchange.Request{Type: exchange.ReqQueryBalance, ...})` — the response arrives on `fastGW.Responses()`, so venue identity is preserved automatically.

---

## Build

```bash
make build          # Build all binaries to bin/
make test           # Run all tests
make test-race      # Run with race detector
make coverage-html  # View coverage in browser
make all            # Format, vet, test, build
```
