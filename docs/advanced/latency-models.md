# Latency Models

**Date**: 2026-02-18
**Package**: `simulation`
**Interface**: `LatencyProvider`

---

## Why Latency Matters for Market Microstructure

Latency shapes who wins and loses in a simulated market:

- **Co-location**: fast actors see fills before slow actors react; the race is real only if latency has temporal clustering
- **Adverse selection**: informed traders can get in before stale quotes are cancelled only if latency has realistic tail behaviour
- **Queue position**: FIFO matching's value depends on consistent low-latency order of arrival; pro-rata's value does not

If latency is i.i.d. and symmetric (Normal), none of these dynamics emerge correctly.

---

## Problems with the Existing Models

| Model | Distribution problem | Correlation problem |
|-------|---------------------|---------------------|
| `ConstantLatency` | No randomness | No randomness |
| `UniformLatency` | Flat, no tail, no mode | i.i.d. |
| `NormalLatency` | Symmetric; clamped negatives spike at 0 | i.i.d. |
| `LoadScaledLatency` | Deterministic given load | i.i.d. |

Two independent deficiencies:

1. **Wrong marginal distribution.** Real network RTT is right-skewed with a heavy tail (packet retransmits, NIC interrupt coalescing, kernel scheduling jitter, GC stop-the-world). A Normal with the same mean and stddev drastically underestimates extreme quantiles. Log-normal is the empirically observed distribution for RTT.

2. **No temporal autocorrelation (no Markov property).** Every `Delay()` call is statistically independent of the previous one. Real congestion is persistent: once a burst hits, the next several packets are also elevated because the same cause is still active. Models that are i.i.d. cannot reproduce latency clustering or flash-congestion events.

---

## New Models

### `LogNormalLatency`

```
log(L − min) ~ N(logMu, logSigma²)
L = min + exp(N(logMu, logSigma²))
```

Strictly positive above the floor. Correct heavy right tail.

**Parameters** (constructor):

| Parameter | Type | Meaning |
|-----------|------|---------|
| `min` | `time.Duration` | Hard floor: irreducible physical propagation |
| `medianAboveMin` | `time.Duration` | Typical excess delay; exp(logMu) = medianAboveMin |
| `logSigma` | `float64` | Spread of log-delay; controls tail heaviness |
| `seed` | `int64` | RNG seed |

**Derived quantities** (for reference when choosing parameters):

```
mean   = min + exp(logMu + logSigma²/2)
median = min + medianAboveMin
p95    = min + exp(logMu + 1.645 · logSigma)
p99    = min + exp(logMu + 2.326 · logSigma)
```

**Example** (co-located actor, 50µs floor, 200µs typical excess, σ=0.5):
```
median ≈ 250µs,  mean ≈ 277µs,  p99 ≈ 690µs
```

**Usage**:
```go
lat := simulation.NewLogNormalLatency(
    50*time.Microsecond,  // min: co-location floor
    200*time.Microsecond, // median above floor
    0.5,                  // logSigma: moderate tail
    42,
)
```

---

### `HawkesLatency` — Self-Exciting Model

Models congestion clustering: each order submitted excites the system, raising latency for subsequent orders. The excitation decays exponentially.

**Kernel (exponential Hawkes)**:

```
R(t)    = R(t_last) · exp(−β · (t − t_last))     between events
R(t_n+) = R(t_n−)  + α                            at each event
L(t)    = minLatency + R(t)
```

The exponential kernel has a key property: the O(1) recursive update
```
R_n = R_{n-1} · exp(−β · Δt) + α
```
is mathematically exact — no approximation, no history retained. This is the defining advantage of the exponential kernel over polynomial or power-law kernels.

**Parameters**:

| Parameter | Type | Meaning |
|-----------|------|---------|
| `minLatency` | `time.Duration` | Floor: irreducible minimum (speed-of-light + processing floor) |
| `jumpPerEvent` | `time.Duration` | α: how much each order submission raises excitation |
| `decayPerSec` | `float64` | β: exponential decay rate; half-life = ln(2)/β ≈ 0.693/β seconds |

**Steady-state excitation** (exogenous arrivals at mean rate ρ):

```
E[R∞] ≈ α · ρ / β        (approximation, accurate to O(β/ρ))

Exact (Poisson arrivals): E[R∞] = α · (ρ + β) / β
```

The process is **always stable** regardless of α and β because events are exogenous — orders arrive from external actors, they do not trigger the Hawkes process to generate its own events. There is no branching ratio constraint (that constraint applies to the classical self-exciting Hawkes where the process generates its own offspring events).

**Parameter design guide**:

Pick β from the congestion half-life, α from desired load sensitivity:

| β | Half-life | Congestion character |
|---|-----------|---------------------|
| 1/s | 693ms | Slow, persistent congestion |
| 10/s | 69ms | Moderate burst decay |
| 100/s | 7ms | Fast recovery, short spikes |

At steady-state rate ρ=1000 orders/s, β=10/s, α=10µs: E[R∞] ≈ 1000µs extra latency under load.

**Usage**:
```go
lat := simulation.NewHawkesLatency(
    50*time.Microsecond,  // minLatency: physical floor
    10*time.Microsecond,  // jumpPerEvent: α
    10.0,                 // decayPerSec: β = 10/s, half-life ≈ 69ms
)

// On every order submission:
lat.RecordEvent()

// To get current delay:
d := lat.Delay()
```

`RecordEvent()` and `Delay()` are goroutine-safe.

---

## Composition

The `LatencyProvider` interface (`Delay() time.Duration`) composes with `LoadScaledLatency`. For maximum realism, stack Hawkes excitation on top of a log-normal noise floor:

```go
// Not a built-in; wire manually in the actor:
//
//   base := simulation.NewLogNormalLatency(min, median, sigma, seed)
//   hawkes := simulation.NewHawkesLatency(min, jump, decay)
//
//   // In actor submit path:
//   hawkes.RecordEvent()
//   effectiveDelay := base.Delay() + hawkes.Delay() - min  // avoid double-counting floor
```

For most simulations, `HawkesLatency` alone is sufficient: it provides the Markov property (temporal clustering) which is the dominant missing feature.

---

## Model Selection

| Scenario | Recommended model |
|----------|------------------|
| Baseline, no realism needed | `ConstantLatency` |
| Simple randomness, no clustering | `LogNormalLatency` |
| Realistic congestion clustering | `HawkesLatency` |
| Queue-depth realism (known load) | `LoadScaledLatency` |
| Full model: clustering + queue pressure | `HawkesLatency` (minLatency = physical floor) |

---

## Mathematical Reference

### Why log-normal for RTT

Network RTT is the sum of multiple independent multiplicative factors (propagation, queueing, retransmit probability). By the law of large numbers applied to products, the log of the sum converges to normal. This is the same argument as for asset returns: `log(L) ~ N` when L is a product of independent factors each near 1.

### Exponential kernel recursive formula derivation

Given events at times t₁ < t₂ < … < tₙ and kernel φ(u) = α·exp(−βu):

```
R(tₙ) = Σᵢ₌₁ⁿ α · exp(−β · (tₙ − tᵢ))
       = α · exp(−β · (tₙ − tₙ₋₁)) · Σᵢ₌₁ⁿ⁻¹ exp(−β · (tₙ₋₁ − tᵢ)) + α
       = exp(−β · Δtₙ) · R(tₙ₋₁) + α
```

No approximation. The exponential kernel is the unique kernel for which this O(1) update is exact.

### Steady-state derivation (Poisson arrivals)

Let T ~ Exp(ρ) be the inter-arrival time. At steady state:

```
E[R] = E[R · exp(−β·T)] + α
     = E[R] · E[exp(−β·T)] + α        (R independent of next T at stationarity)
     = E[R] · ρ/(ρ+β) + α             (Laplace transform of Exp(ρ))

⟹  E[R] · (1 − ρ/(ρ+β)) = α
⟹  E[R] = α · (ρ+β) / β
         ≈ α · ρ / β   for ρ ≫ β
```
