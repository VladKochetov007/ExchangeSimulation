# Market Makers

## SlowMarketMaker (standalone)

Timer-based market maker. Requotes on a fixed interval, **not** on fills or snapshots.
This is the correct architecture for random-walk price discovery — see
[microstructure-patterns.md](microstructure-patterns.md) for why.

```go
mm := actors.NewSlowMarketMaker(id, gateway, actors.SlowMarketMakerConfig{
    Symbol:          "BTC-PERP",
    Instrument:      perpInst,
    SpreadBps:       10,
    QuoteSize:       1 * exchange.BTC_PRECISION,
    MaxInventory:    100 * exchange.BTC_PRECISION,
    RequoteInterval: 1000 * time.Millisecond, // timer only; fills do NOT trigger requote
    BootstrapPrice:  exchange.PriceUSD(50000, exchange.CENT_TICK),
    EMADecay:        0.2,  // weight on each new trade price; higher = faster adaptation
    Levels:          4,    // levels per side
    LevelSpacingBps: 5,    // spacing between levels in bps
})
mm.SetTickerFactory(tickerFactory)
mm.Start(ctx)
```

**Price discovery:** EMA updated on `EventTrade` (market-wide signal). Book snapshots are
only used to seed `lastMidPrice` before the first trade.

**Inventory skew:** When `|inventory| > 50% MaxInventory`, the number of active levels
scales down linearly toward 1. Inventory is never skewed into the mid price — the MM
simply quotes fewer levels.

---

## PureMMSubActor (SubActor)

Snapshot-driven market maker designed for use inside a `CompositeActor`. Reprices when
EMA mid moves past `RequoteThreshold` from the last quoted price, and on every fill.

```go
mm := actors.NewPureMMSubActor(id, "BTC/USD", actors.PureMMSubActorConfig{
    SpreadBps:        20,
    QuoteSize:        5 * exchange.BTC_PRECISION,
    MaxInventory:     50 * exchange.BTC_PRECISION,
    RequoteThreshold: 10, // price units; triggers requote when EMA moves this far
    Precision:        exchange.BTC_PRECISION,
    BootstrapPrice:   bootstrapPrice,
    EMAAlpha:         20, // per-fill EMA weight: 20% toward new fill price
})

composite := actor.NewCompositeActor(id, gateway, []actor.SubActor{mm})
mm.SetCancelFn(composite.CancelOrder)
```

**Inventory skew:** `skew = (position/MaxInventory) * halfSpread`, capped at ±halfSpread.
Both bid and ask shift together: high inventory → lower prices to attract sells.

---

## AvellanedaStoikov (standalone / SubActor)

Full Avellaneda-Stoikov stochastic control model. Computes reservation price and
optimal spread analytically.

```go
// Reservation price: r = mid - position * gamma * sigma^2 * (T - t)
// Optimal spread:    delta = gamma * sigma^2 * (T - t) + (2/gamma) * ln(1 + gamma/k)
```

Parameters: `Gamma` (risk aversion), `Sigma` (volatility), `K` (order arrival rate),
`T` (time horizon in seconds).

---

## SpreadModel interface

Inject a `SpreadModel` into any actor that accepts one to decouple spread logic from
quoting logic.

```go
type SpreadModel interface {
    HalfSpread(instrument exchange.Instrument, inventory int64) int64
}
```

### FixedHalfSpread

Returns `Bps` regardless of conditions.

```go
&actors.FixedHalfSpread{Bps: 10}
```

### OFISpreadModel

Widens spread when order flow imbalance is high (buy pressure → widen ask; sell
pressure → widen bid side). Calibrated via a rolling signed-volume EMA.

```go
model := &actors.OFISpreadModel{
    BaseBps:      10,    // minimum half-spread
    MaxExtraBps:  30,    // added at maximum imbalance
    WindowVolume: 100 * exchange.BTC_PRECISION, // normalisation denominator
    DecayFactor:  900,   // 900 = retain 90% per trade (half-life ≈ 6.6 trades)
}

// Call from actor's trade handler:
model.OnTrade(fill.Side, fill.Qty)
```

`DecayFactor` calibration for desired half-life H (trades):
`DecayFactor = round(1000 * exp(-ln2/H))`:
- H=3 → 794 (fast)
- H=7 → 906
- H=20 → 966 (slow)
