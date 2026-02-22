# Price Processes

## GBMProcess

Geometric Brownian Motion for a latent fundamental asset price. Implements both
`exchange.IndexPriceProvider` (for mark price models) and `PrivateSignalOracle`
(for `InformedTrader`).

```go
gbm := simulation.NewGBMProcess(
    initialPrice int64,   // in exchange precision units
    prec int64,           // e.g. exchange.USD_PRECISION
    annualDrift float64,  // μ, e.g. 0.0
    annualVol float64,    // σ, e.g. 0.5 = 50% annual
    seed int64,
)
gbm.Register("BTC-PERP") // associate with symbol(s)
```

**Advance in sim loop:**

```go
// dtSeconds = simStep / time.Second
gbm.Advance(dtSeconds)
```

Call `Advance` every simulation tick before `simClock.Advance`. The process uses
`S(t+dt) = S(t) * exp((μ - σ²/2)dt + σ√dt·Z)` where Z ~ N(0,1).

**Wire as index price provider:**

```go
automation := exchange.NewExchangeAutomation(ex, exchange.AutomationConfig{
    MarkPriceCalc: exchange.NewBinanceMarkPrice("BTC-PERP", gbm),
    IndexProvider: gbm,
    ...
})
```

**Wire as informed-trader oracle:**

```go
trader := actors.NewInformedTrader(id, gw, actors.InformedTraderConfig{
    Oracle: gbm,
    ...
})
```

---

## Latency Models

All models implement `LatencyProvider`:

```go
type LatencyProvider interface {
    Delay() time.Duration
}
```

| Type | Constructor | Notes |
|------|-------------|-------|
| `ConstantLatency` | `NewConstantLatency(d)` | Fixed delay |
| `UniformRandomLatency` | `NewUniformRandomLatency(min, max, seed)` | Uniform in [min,max] |
| `NormalLatency` | `NewNormalLatency(mean, stddev, seed)` | Gaussian, clamped ≥ 0 |
| `LogNormalLatency` | `NewLogNormalLatency(min, medianAboveMin, logSigma, seed)` | Heavy-tailed; common for real networks |
| `HawkesLatency` | `NewHawkesLatency(minLatency, jumpPerEvent, decayPerSec)` | Self-exciting: bursts under load |
| `LoadScaledLatency` | `NewLoadScaledLatency(base, perRequest)` | `base + n*perRequest` where n=concurrent requests |

`HawkesLatency.Trigger()` must be called on each event to accumulate intensity.
