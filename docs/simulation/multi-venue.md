# Multi-Venue Simulation

The simulation package provides first-class support for running actors across multiple independent exchanges simultaneously. The key types are `VenueRegistry`, `MultiVenueGateway`, and `MultiExchangeRunner`.

## VenueRegistry

A registry that holds named exchanges. Build one, register your exchanges, then pass it to any actor that needs cross-venue access.

```go
registry := simulation.NewVenueRegistry()
registry.Register("binance", binanceEx)
registry.Register("okx", okxEx)

// Look up later
ex := registry.Get("binance")
venues := registry.ListVenues()  // []VenueID{"binance", "okx"}
```

## MultiVenueGateway

Connects a single client to all venues in a registry. Under the hood it calls `ex.ConnectClient()` for each venue, then fans in all responses and market data onto unified channels tagged with `VenueID`.

```go
mgw := simulation.NewMultiVenueGateway(
    clientID,
    registry,
    map[simulation.VenueID]map[string]int64{
        "binance": {"USD": 100_000 * exchange.USD_PRECISION, "BTC": 1 * exchange.BTC_PRECISION},
        "okx":     {"USD": 50_000 * exchange.USD_PRECISION},
    },
    map[simulation.VenueID]exchange.FeeModel{
        "binance": &exchange.PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true},
        "okx":     &exchange.PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true},
    },
)
mgw.Start()
defer mgw.Stop()
```

**Sending orders:**
```go
mgw.SubmitOrder("binance", &exchange.OrderRequest{
    Symbol:      "BTCUSD",
    Side:        exchange.Buy,
    Type:        exchange.LimitOrder,
    Price:       100_000 * exchange.USD_PRECISION,
    Qty:         1 * exchange.BTC_PRECISION,
    TimeInForce: exchange.GTC,
})

mgw.CancelOrder("okx", &exchange.CancelRequest{OrderID: orderID})
mgw.QueryBalance("binance", &exchange.QueryRequest{RequestID: 1})
```

**Subscribing to market data:**
```go
mgw.Subscribe("binance", "BTCUSD")
mgw.Subscribe("okx", "BTCUSD")
```

**Receiving events (in your actor's run loop):**
```go
select {
case vResp := <-mgw.ResponseCh():
    // vResp.Venue identifies which exchange responded
    // vResp.Response is the exchange.Response
    switch r := vResp.Response.(type) {
    case *exchange.OrderAccepted:
        fmt.Printf("Order accepted on %s: %d\n", vResp.Venue, r.OrderID)
    case *exchange.Fill:
        fmt.Printf("Fill on %s: %d @ %d\n", vResp.Venue, r.Qty, r.Price)
    }

case vData := <-mgw.MarketDataCh():
    // vData.Venue identifies the exchange
    // vData.Data is *exchange.MarketDataMsg
    switch vData.Data.Type {
    case exchange.MDSnapshot:
        snap := vData.Data.Data.(*exchange.BookSnapshot)
        _ = snap
    }
}
```

**Direct gateway access:**
```go
// Get the raw ClientGateway for a specific venue if needed
gw := mgw.GetGateway("binance")
```

### Channel capacity

`responseCh` buffers 100 messages; `marketDataCh` buffers 1000. Messages dropped silently when full — keep your event loop fast.

## LatencyArbitrageActor

The reference multi-venue actor. Monitors the same symbol on a fast and slow venue; when the fast venue's best bid exceeds the slow venue's best ask by at least `MinProfitBps`, it simultaneously buys on the slow venue and sells on the fast venue.

```go
mgw := simulation.NewMultiVenueGateway(arbID, registry, balances, fees)
mgw.Start()

arb := simulation.NewLatencyArbitrageActor(arbID, mgw, simulation.LatencyArbitrageConfig{
    FastVenue:    "binance",
    SlowVenue:    "okx",
    Symbol:       "BTCUSD",
    MinProfitBps: 5,                      // at least 0.05% spread
    MaxQty:       exchange.BTCAmount(1),  // max 1 BTC per leg
})
arb.Start(ctx)

// Later:
arbitrages, profit := arb.Stats()
```

The actor uses IOC orders on both legs so a partial fill on one side doesn't leave a resting position. It does not hedge inventory across fills — it relies on legs filling simultaneously at the observed prices. Use this as a template, not production code.

## MultiExchangeRunner

High-level runner for full multi-venue simulations. Takes a `MultiSimConfig` and constructs exchanges, instruments, loggers, and actors automatically.

```go
config := simulation.DefaultMultiSimConfig()
// Defaults: binance (1ms), okx (5ms), bybit (3ms)
// 5 assets with 60% overlap, 50% spot / 50% perp
// 2 LPs + 3 MMs + 1 taker per symbol
// 5 seconds wall-clock duration (10x speedup)

// Override what you need:
config.Duration = 60 * time.Second
config.LogDir = "./logs"
config.SimSpeedup = 50.0
config.InitialBalances["BTC"] = 10 * exchange.BTC_PRECISION

runner, err := simulation.NewMultiExchangeRunner(config)
if err != nil {
    log.Fatal(err)
}
defer runner.Close()

ctx := context.Background()
if err := runner.Run(ctx); err != nil {
    log.Fatal(err)
}
```

### MultiSimConfig fields

| Field | Type | Description |
|-------|------|-------------|
| `Exchanges` | `[]ExchangeConfig` | Per-venue name and latency |
| `GlobalAssets` | `[]string` | Base assets: BTC, ETH, SOL, … |
| `QuoteAsset` | `string` | Quote currency (typically "USD") |
| `OverlapRatio` | `float64` | 0–1, fraction of assets listed on all venues |
| `SpotToFuturesRatio` | `float64` | 0.5 = half spot, half perp per venue |
| `LPsPerSymbol` | `int` | Multi-symbol LP actors per venue |
| `MMsPerSymbol` | `int` | Market-maker actors per venue |
| `TakersPerSymbol` | `int` | Random-taker actors per symbol |
| `LPSpreadBps` | `int64` | LP quoted spread |
| `TakerInterval` | `time.Duration` | Taker order cadence |
| `InitialBalances` | `map[string]int64` | Total capital divided among all actors |
| `Duration` | `time.Duration` | Simulation wall-clock duration |
| `LogDir` | `string` | Root directory for NDJSON logs |
| `SnapshotInterval` | `time.Duration` | Market data snapshot interval (0 = disabled) |
| `SimSpeedup` | `float64` | Speeds up actor intervals; does not advance simulated clock |

### Symbol overlap

`GenerateSymbolsWithOverlap` distributes assets so the first `OverlapRatio` fraction are listed on all venues; the rest are split round-robin. With `GlobalAssets = [BTC, ETH, SOL, XRP, DOGE]` and `OverlapRatio = 0.6`, BTC and ETH appear on every venue while SOL, XRP, DOGE are split across venues.

### Log layout

```
logs/
  binance/
    general.log          # balance snapshots, borrow, repay events
    spot/
      BTCUSD.log
      ETHUSD.log
    perp/
      SOLUSD.log
  okx/
    general.log
    spot/
      BTCUSD.log
    perp/
      ETHUSD.log
```

Each file is NDJSON. See [Logging System](../observability/logging-system.md) for event schemas.

## Shared clock

All exchanges created by `MultiExchangeRunner` share the same `RealClock`. For deterministic simulation with accelerated time, replace the clock and pass a `SimTickerFactory`:

```go
clock := simulation.NewSimulatedClock(time.Now())
tf := simulation.NewSimTickerFactory(clock)

for _, cfg := range config.Exchanges {
    ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
        ID:            cfg.Name,
        Clock:         clock,
        TickerFactory: tf,
    })
    // ...
}
```

See [Simulated Time](simulated-time.md) for the full pattern.

## Related

- [Simulated Time](simulated-time.md) — clock and ticker factories
- [Latency Models](../advanced/latency-models.md) — DelayedGateway and LatencyProvider
- [Actor System](../actors/actor-system.md) — BaseActor and CompositeActor
