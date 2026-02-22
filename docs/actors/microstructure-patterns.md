# Microstructure Simulation Patterns

Lessons learned from comparing a broken simulation (`microstructure_v1`) against a working
one (`randomwalk_v2`). The patterns here apply to any sim that runs faster than real time.

---

## 1. The SimTicker Constraint (Critical)

`SimTicker` uses a **capacity-1 buffered channel** with a **non-blocking send**:

```go
// simulation/ticker.go
ch: make(chan time.Time, 1)

select {
case t.ch <- time.Unix(0, ...):
default: // tick silently dropped
}
```

When you call `simClock.Advance(D)`, the scheduler fires **every event** due within D.
If a ticker with interval T fires `D/T` times per advance, only the **first tick** reaches
the actor. All remaining ticks are silently dropped.

### Rule

```
simStep  ≤  min(all_ticker_intervals)
```

At most one tick fires per ticker per Advance call → zero drops.

### Examples

| simStep | SnapshotInterval | Ticks fired | Ticks delivered | Drop rate |
|---------|-----------------|-------------|-----------------|-----------|
| 60 s    | 500 ms          | 120         | 1               | 99.2%     |
| 5 s     | 500 ms          | 10          | 1               | 90%       |
| **500 ms** | **500 ms**   | **1**       | **1**           | **0%** ✓  |
| 10 ms   | 100 ms          | 0 or 1      | 0 or 1          | **0%** ✓  |

`randomwalk_v2` uses `simTimeStep=10ms` and `SnapshotInterval=100ms` → no drops.
`microstructure_v1` (fixed) uses `simStep=500ms` and `SnapshotInterval=500ms` → no drops.

---

## 2. Market Maker Architecture: Timer vs Snapshot

### Correct: Timer-Based Requoting (SlowMarketMaker)

The MM owns an independent requote ticker. Repricing is **decoupled from book snapshots**.

```
Wall clock →  [every 10ms simClock.Advance(10ms)]
                   │
Snapshot ticker ─→ onBookSnapshot() → update lastMidPrice ONLY if uninitialized
Trade events    ─→ onTrade()         → update EMA mid
Requote ticker  ─→ requoteLoop()     → cancel + replace all levels
Fill events     ─→ onOrderFilled()   → update inventory ONLY (no requote!)
```

Key property: **requote rate = 1/RequoteInterval**, independent of snapshot rate or fill rate.

### Anti-pattern: Snapshot-Driven Requoting

If requoting is triggered by `EventBookSnapshot`, the requote rate is tied to how many
snapshots actually arrive. With tick drops, the MM may never reprice.

Additionally, if the MM's own quotes dominate the book, its mid-price calculation reads its
own prices → **circular dependency** that locks the book at the bootstrap price forever.

### Anti-pattern: Requoting on Every Fill

Immediately repricing after each fill causes the MM to chase its own trades:

```
Taker hits bid → fill → requote lower → taker hits new bid → fill → requote lower ...
```

The price collapses monotonically instead of performing a random walk. Fills should update
inventory and EMA but must **not** trigger an immediate requote.

---

## 3. Price Discovery: EMA on Trades, Not Fills

### Correct: Update EMA on `EventTrade`

```go
func (smm *SlowMarketMakerActor) onTrade(trade actor.TradeEvent) {
    alpha := smm.config.EMADecay   // e.g. 0.3 for fast, 0.1 for slow
    newEMA := alpha*tradePrice + (1-alpha)*currentEMA
    smm.lastMidPrice = int64(newEMA)
}
```

`EventTrade` is broadcast to **all subscribers**, including every MM. Each MM applies its
own EMA decay, giving each an independent view of fair value. With different decays
(0.3 / 0.2 / 0.1) the MMs never converge to identical prices simultaneously, preventing
synchronized oscillations.

### Anti-pattern: EMA on Own Fills Only

If the MM only updates its price reference when *it* gets filled, makers with large quote
sizes relative to taker size will rarely see fills → price stays anchored at bootstrap.

For the EMA to work as price discovery, it needs the **market's trade stream**, not just
own fills.

---

## 4. Quote Size vs Taker Size

For `fill.IsFull` to fire, a taker's order must be **at least as large** as one MM level.

| Scenario | Result |
|----------|--------|
| QuoteSize=5 BTC, MaxTakerQty=2 BTC | Partial fills only — `IsFull` never fires |
| QuoteSize=1 BTC, MaxTakerQty=0.5-2.2 BTC | Both partial and full fills occur |

If `IsFull` never fires, any code path that triggers on full fills (repricing, inventory reset)
never executes.

**Rule:** `MaxTakerQty ≥ QuoteSize` (or handle partial fills explicitly).

---

## 5. Staggered Requote Intervals

Running multiple MMs with identical requote intervals causes synchronized repricing:

```
T=0.0s: MM1, MM2, MM3 all cancel and requote simultaneously
T=1.0s: MM1, MM2, MM3 all cancel and requote simultaneously
```

This creates a periodic "book vacuum" every second where no liquidity exists between the
cancel and the new quotes landing. Takers hitting during this window see an empty book.

**Fix:** Stagger intervals so only one MM reprices at a time:

```go
requoteIntervals := []time.Duration{
    800 * time.Millisecond,  // MM1
    1000 * time.Millisecond, // MM2
    1200 * time.Millisecond, // MM3
}
```

Combined with different EMA decays, this produces genuinely independent price views.

---

## 6. Exit Condition Logic for Arb Actors

### Correct: Mirror entry logic with OR

An arb entered on **either** condition A or condition B should exit when **either** reverts:

```go
// Entry: A OR B triggers entry
if condA || condB {
    enter()
}

// Exit: A OR B reverting triggers exit
if !condA || !condB {
    exit()
}
```

### Anti-pattern: AND exit condition

```go
// Wrong: requires BOTH conditions to revert before exiting
if !condA && !condB {
    exit()
}
```

With this bug, a position entered on condition A alone stays open forever if condition B
never reverses. Common in funding arb: entered on basis alone, exits only when both basis
AND funding rate revert.

---

## 7. Reference Implementation: randomwalk_v2

`cmd/randomwalk_v2/main.go` demonstrates all correct patterns:

```
simTimeStep    = 10 ms          → simStep < SnapshotInterval (no tick drops)
SnapshotInterval = 100 ms       → 1 snapshot per 10 advances
RequoteIntervals = 800/1000/1200 ms  → staggered, no synchronization
EMADecays      = 0.3 / 0.2 / 0.1   → independent price views per MM
QuoteSize      = 1.0 BTC        → takers (0.5-2.2 BTC) can fully fill
EMA updated on EventTrade        → market-wide signal, not own fills
No requote on fills              → price drifts to next timer tick
4 levels per side               → takers hit stale quotes mid-level for free price movement
```

Result: 365 trades/10s, 20 bps spread, genuine random-walk price process.

---

## 8. Checklist for New Simulations

Before running a simulation, verify:

- [ ] `simStep ≤ min(all ticker intervals, SnapshotInterval)`
- [ ] MMs use timer-based requoting, not snapshot-triggered
- [ ] EMA/mid updated on `EventTrade`, not `EventOrderFilled`
- [ ] No immediate requote inside `onOrderFilled`
- [ ] `MaxTakerQty ≥ MM QuoteSize` (or partial-fill handled)
- [ ] MM requote intervals are staggered across instances
- [ ] Different EMA decays per MM instance
- [ ] Arb exit conditions use OR, not AND
- [ ] All actors checked for `fill.OrderID == 0` guard before routing fills
