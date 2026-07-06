# Feesim Simulation Debugging Postmortem

## Summary

The `feesim` simulation had two observable failures:

1. **Price stuck at bootstrap**: prices oscillated in a 4-tick range instead of showing
   taker-driven random walk. 73% of trades at the same price.
2. **Spot-perp divergence**: Q/USD reached $3,992 while Q-PERP fell to $2,158 — a 61%
   gap — despite an active basis arbitrage actor.

Both failures stem from **implicit coupling between actors and the MM's price discovery
mechanism** (`mm.mid = fillPrice`). Every fill from any participant becomes the MM's next
quoting reference. This coupling, combined with latency artifacts and arb feedback loops,
created behaviors that were the opposite of their intended effect.

---

## Bug 1 — Latency desynchronizes MM state from exchange book

### Symptom
Prices oscillated in a 4-tick range around the bootstrap price. Expected: smooth trending
random walk driven by taker flow.

### Root Cause
With a single mount sharing 1-3ms latency across all actors (including MMs):

1. MM timer fires → `cancelAll()` clears `pending` map **instantly**
2. Cancel requests take 1-3ms to reach the exchange
3. MM immediately calls `quote()` → new orders submitted (also 1-3ms to arrive)
4. **For 1-3ms, old and new orders coexist on the book**
5. Taker fills a stale order at the old mid price
6. MM receives fill: `mm.mid = stalePrice` → reverts any random walk progress

The MM's in-memory state (`pending` map) and the exchange's actual state (order book)
are desynchronized during the latency window. The MM thinks old orders are cancelled;
the exchange still has them resting.

### Fix
**Dual mount architecture**: MMs connect via a zero-latency mount (co-located), takers and
arbs connect via a latency-modeled mount. Both mounts connect to the same exchange.

```go
mmMount := simulation.NewMount(ex, simulation.LatencyConfig{})       // zero latency
actorMount := simulation.NewMount(ex, actorLatency)                  // 1-3ms latency
```

With zero latency, the MM's `cancelAll()` takes effect immediately on the exchange book.
No stale order overlap. `pending` map stays synchronized with the book.

### Guideline
**MMs must use zero-latency mounts.** Real institutional MMs are co-located with
sub-microsecond latency. Simulating them with retail latency creates state
desynchronization artifacts that prevent realistic price dynamics. Use separate mounts
with different latency profiles to model the co-location advantage.

---

## Bug 2 — Arb feedback loop dominates price discovery

### Symptom
Spot and perp prices diverged massively instead of staying coupled.

### Root Cause
The basis arb used market orders on a 100ms timer. Each market order fills at the MM's best
price, which sets `mm.mid = fillPrice` (exactly 1 tick of movement, regardless of fill
quantity). At 100ms intervals over 600 seconds, the arb could fire up to 6,000 times.

The arb's fills became the **dominant** price signal for the MM — not the taker's random walk.
Per-client fill analysis proved it:

```
Q/USD net buys by client:
  client  2: -1193  (MM, counterparty to all)
  client  6:   +81  (taker, nearly balanced — expected sqrt(N) drift)
  client  8:  +962  (basis arb — 12x the taker's directional impact)
```

The arb's corrective trades accumulated into directional pressure: +962 buys pushed Q/USD UP,
while -944 sells pushed Q-PERP DOWN. The arb created the divergence it was trying to correct.

### Fix attempts that failed

| Attempt | Rationale | Result |
|---------|-----------|--------|
| MaxPosition 500 → 100,000 | Give arb more room to correct | More room for feedback loop |
| Remove close logic | Stop enter-close cycles from canceling out | Position accumulates unboundedly in one direction |
| Increase lot size | Bigger corrections per trade | Irrelevant — `mm.mid = fillPrice` moves 1 tick regardless of quantity |
| MaxPosition = 10 | Hard-cap directional impact | Arb saturates quickly, can't correct sustained basis |
| MaxPosition = 50 | Moderate cap | Seed-dependent — fixed one pair, broke the other |

### Fix that worked
**Slow arb check interval from 100ms to 1 second**, combined with MaxPosition=100.

- At 1s intervals, the arb fires at most 600 times over 10 minutes
- Between arb interventions, the taker makes ~10 trades per symbol, establishing the
  random walk as the dominant price signal
- MaxPosition=100 provides ample correction capacity without the arb dominating
- The taker drives the walk; the arb keeps markets coupled

### Guideline
**Arb check frequency must be slower than the rate at which the taker establishes new price
levels.** If the arb fires faster than the taker walks, the arb's fills dominate the MM's
mid and create a feedback loop. Rule of thumb: arb interval should be 5-10x the taker interval.

---

## Design Guidelines for Simulation Actors

### 1. `mm.mid = fillPrice` is implicit coupling

Every fill from any participant — taker, arb, funding arb, triangle arb — sets the MM's
next quoting midpoint. This means **any actor that takes liquidity from the MM controls the
MM's price**. Design arb actors with this in mind.

- Lot size does not control price impact. A 0.001 BTC fill and a 1 BTC fill both move
  `mid` by exactly 1 tick.
- The only controls are: **how often** the arb fires and **whether it fires at all**
  (threshold logic).

### 2. Arb frequency vs taker frequency

The taker is the intended source of randomness (price drift). The arb is a corrective force.
For the taker to dominate:

```
arb_trades_per_second  <<  taker_trades_per_symbol_per_second
```

With 5 symbols and a 100ms taker interval, there are ~2 taker trades per symbol per second.
An arb checking every 100ms (10/s) will dominate. At 1s intervals (1/s), the arb is
subordinate to the taker.

### 3. Position limits interact with frequency

| MaxPosition | Check interval | Max trades/10min | Behavior |
|-------------|---------------|------------------|----------|
| 10 | 100ms | 6,000 | Saturates in 1s, stops correcting |
| 100,000 | 100ms | 6,000 | Feedback loop, arb drives divergence |
| 100 | 1s | 600 | Sweet spot: enough capacity, no feedback |

The sweet spot requires both: enough capacity (MaxPosition) AND slow enough frequency
(CheckInterval) to let the taker drive price discovery between arb interventions.

### 4. Per-client fill analysis is the definitive diagnostic

When prices behave unexpectedly, tally net buys per client ID from the trade log. This
immediately reveals which actor is driving the market. In our case, it showed the arb
(net +962 buys) was 12x more directional than the taker (net +81).

### 5. Zero-latency mount for MMs

Always use a zero-latency mount for market makers. Simulated latency on MMs creates
stale order overlap that prevents realistic random walk behavior. Reserve latency modeling
for participants that genuinely experience it (takers, arbs, retail flow).
