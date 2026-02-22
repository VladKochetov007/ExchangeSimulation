# Takers

## RandomizedTaker (standalone)

Fires a random market order (buy or sell) on a fixed interval. Used in `randomwalk_v2`.

```go
taker := actors.NewRandomizedTaker(id, gateway, actors.RandomizedTakerConfig{
    Symbol:         "BTC-PERP",
    Interval:       500 * time.Millisecond,
    MinQty:         50 * exchange.BTC_PRECISION / 100,  // 0.5 BTC
    MaxQty:         200 * exchange.BTC_PRECISION / 100, // 2.0 BTC
    BasePrecision:  exchange.BTC_PRECISION,
    QuotePrecision: exchange.USD_PRECISION,
})
taker.SetTickerFactory(tickerFactory)
taker.Start(ctx)
```

Direction is uniformly random. For balanced flow use multiple takers with different
seeds; by law of large numbers, direction approaches 50/50.

---

## RandomTakerSubActor (SubActor)

Snapshot-driven taker for use inside a `CompositeActor`. Fires when the simulated
interval has elapsed since the last trade (measured in snapshot timestamps).

```go
taker := actors.NewRandomTakerSubActor(id, "BTC/USD", actors.RandomTakerSubActorConfig{
    Interval:    600 * time.Millisecond,
    MinQty:      20_000_000,   // 0.2 BTC (in BTC_PRECISION units)
    MaxQty:      200_000_000,  // 2.0 BTC
    Precision:   exchange.BTC_PRECISION,
    Instrument:  spotInst,     // optional; enables perp-vs-spot balance check
    TakerFeeBps: 10,           // fee buffer added to notional pre-check
}, seed)
```

**Balance guards (automatic):**
- Buy: checks `notional * (1 + fee) ≤ ctx.GetAvailableQuote()`
- Spot sell: checks `qty ≤ ctx.GetBaseBalance(baseAsset)`
- Perp short: no base balance check (exchange handles margin)

---

## InformedTrader (standalone)

Trades directionally toward a private fundamental value signal. Models Kyle (1985) /
Glosten-Milgrom (1985) informed order flow: drives price discovery and creates adverse
selection costs for market makers.

```go
// GBMProcess implements PrivateSignalOracle
gbm := simulation.NewGBMProcess(
    exchange.PriceUSD(50000, exchange.CENT_TICK),
    exchange.USD_PRECISION,
    0.0,   // zero drift
    0.5,   // 50% annual volatility
    42,    // seed
)
gbm.Register("BTC-PERP")

trader := actors.NewInformedTrader(id, gateway, actors.InformedTraderConfig{
    Symbol:       "BTC-PERP",
    Oracle:       gbm,
    ThresholdBps: 20,                       // minimum signal deviation to trade
    OrderQty:     1 * exchange.BTC_PRECISION,
    PollInterval: 500 * time.Millisecond,
})
trader.SetTickerFactory(tickerFactory)
trader.Start(ctx)

// Advance GBM in your sim loop:
gbm.Advance(dtSeconds)
```

When `signal > mid + threshold`: submit buy market order.
When `signal < mid - threshold`: submit sell market order.
While an order is in flight, no new orders are submitted.
