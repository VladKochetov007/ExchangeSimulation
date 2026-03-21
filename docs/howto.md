# Randomwalk Simulation Debugging Postmortem

## Summary

The `randomwalk` simulation started with two observable failures:

1. **Book death**: the ABC-PERP order book went silent at ~450 simulation-seconds; taker
   market orders were accepted but returned zero fills thereafter.
2. **Price divergence**: after fixing the book death, spot (ABC-USD) and perp (ABC-PERP)
   prices diverged by up to $120 and never converged, even though a basis-arbitrage actor
   was present.

Four distinct bugs were found and fixed. Three are exchange/actor infrastructure bugs
(bugs 1–3); one is an economic design flaw in the arbitrage strategy (bug 4).

---

## Bug 1 — `EventOrderPartialFill` not handled in MarketMaker

### File
`simulations/randomwalk/mm.go`

### Symptom
Perp trading stopped at t ≈ 300 s on the first run with the rebuilt binary.

### Root Cause
`HandleEvent` only matched `actor.EventOrderFilled`:

```go
// BEFORE
case actor.EventOrderFilled:
    mm.onFilled(evt.Data.(actor.OrderFillEvent))
```

The taker places 0.4 BTC market orders; the MM rests 1 BTC per level. Every fill is a
**partial** fill from the MM's perspective (`IsFull = false`). The actor framework emits
`EventOrderPartialFill` for those events, not `EventOrderFilled`. Because the MM never
handled `EventOrderPartialFill`:

- The order was never removed from `mm.pending[sym]`.
- `mm.pending[sym]` was always non-empty.
- `onTick` checks `len(mm.pending[sym]) == 0` before re-quoting — it was always false.
- The book starved: existing orders were gradually consumed by the taker but no new ones
  were placed.

### Fix
```go
// AFTER
case actor.EventOrderPartialFill, actor.EventOrderFilled:
    mm.onFilled(evt.Data.(actor.OrderFillEvent))
```

Also added an explicit cancel for the partially-filled order in `onFilled` so the
remaining quantity is withdrawn and the book is immediately re-quoted with fresh levels:

```go
if !e.IsFull {
    mm.CancelOrder(e.OrderID)
}
mm.cancelAllForSym(sym)
```

### Effect
Perp trading extended from ~300 s to ~450 s. A second, deeper bug was still preventing
the full 900 s run.

---

## Bug 2 — Ghost entries in `pending[sym]` after `cancelAllForSym`

### File
`simulations/randomwalk/mm.go`

### Symptom
Perp book went silent at t ≈ 450 s. BookDelta events confirmed new orders were placed at
t = 450.001–450.008 s, but the book snapshot at t = 451 s was empty, with no removal
events in between.

### Root Cause
`quote()` submits N order requests and records all N request IDs in `reqToSym`:

```go
bidReqID := mm.SubmitOrder(...)
mm.reqToSym[bidReqID] = sym
```

The actor goroutine and the exchange runner are concurrent. A race is possible:

1. `quote()` submits requests r₁…r₁₀, all go into `reqToSym`.
2. Accepts for r₁…r₇ arrive first → added to `pending[sym]` and `orderToSym`.
3. A taker fill arrives and triggers `onFilled` → `cancelAllForSym`:
   - Cancels o₁…o₇ (those already in `pending[sym]`).
   - Clears `pending[sym]`.
   - **Does NOT clear `reqToSym`** — r₈, r₉, r₁₀ still map to `sym`.
4. The exchange finishes processing r₈, r₉, r₁₀ and sends their accepts.
5. `onAccepted` fires for each: `reqToSym[rₙ]` is found → orders added back into
   `pending[sym]` as **ghost entries**.
6. Timer sees `len(pending[sym]) > 0` → skips `quote()` forever.

The ghost orders from step 5 are real, live orders in the book — but the MM never
cancels them and never re-quotes. Over many fill cycles, ghosts accumulate. Each ghost
has margin reserved in `PerpReserved`, slowly draining `PerpAvailable`.

### Fix

**`cancelAllForSym`** — also sweep `reqToSym` for the symbol:

```go
func (mm *MarketMaker) cancelAllForSym(sym string) {
    for orderID := range mm.pending[sym] {
        mm.CancelOrder(orderID)
        delete(mm.orderToSym, orderID)
        delete(mm.pending[sym], orderID)
    }
    // Clear in-flight requests so late accepts become orphans.
    for reqID, s := range mm.reqToSym {
        if s == sym {
            delete(mm.reqToSym, reqID)
        }
    }
}
```

**`onAccepted`** — handle orphaned accepts (their reqID was cleared):

```go
func (mm *MarketMaker) onAccepted(e actor.OrderAcceptedEvent) {
    sym, ok := mm.reqToSym[e.RequestID]
    if !ok {
        // Request was cleared by cancelAllForSym; cancel the live order immediately.
        mm.CancelOrder(e.OrderID)
        return
    }
    delete(mm.reqToSym, e.RequestID)
    mm.pending[sym][e.OrderID] = true
    mm.orderToSym[e.OrderID] = sym
}
```

The orphan path sends a cancel to the exchange, properly removing the order from the
book with full `publishBookUpdate` coverage and margin release.

---

## Bug 3 — Silent order removal in `cancelClientOrdersOnBook` (liquidation)

### Files
`exchange/liquidation.go`, `actor/actor.go`

### Symptom
Corroborated bug 2's effect: even after fixing ghost entries, the book could still go
silently empty. Log analysis showed:

- No `BookDelta` removal events when orders disappeared.
- No `OrderCancelled` events delivered to the MM actor.
- The disappearance occurred at approximately 30-second intervals (the mark price update
  cadence), confirming the liquidation path.

### Root Cause
When `CheckLiquidations` fires every 30 s and finds a client whose equity / initial
margin is below the maintenance threshold, it calls `liquidate()` → `forceClose()` →
`cancelClientOrdersOnBook()`. That function:

```go
// BEFORE (excerpt)
if order.Side == Buy {
    book.Bids.CancelOrder(orderID)
} else {
    book.Asks.CancelOrder(orderID)
}
client.RemoveOrder(orderID)
// ← no publishBookUpdate()
// ← no Response sent to actor gateway
```

Two omissions:

1. **No `publishBookUpdate()`** — the book data structure was mutated but no `BookDelta`
   event was emitted. Market data subscribers (and log consumers) saw no change; the
   snapshot then returned an empty book, which looked like the orders had never existed.

2. **No cancel notification to the actor** — the MM's `pending[sym]` retained the order
   IDs forever. Since the gateway never received a cancel response, `onCancelled` was
   never called, and `pending[sym]` stayed non-empty, blocking re-quoting indefinitely.

The ghost entries from bug 2 were the trigger: accumulated ghost orders inflated
`PerpReserved`, causing `PerpAvailable = PerpBalance − PerpReserved` to go sufficiently
negative that the liquidation check triggered even with a 10 M USD balance.

### Fix

**`exchange/liquidation.go`** — add a new exported notification type, call
`publishBookUpdate`, and push notifications to the gateway:

```go
// New type in liquidation.go
type ForcedCancelNotification struct {
    OrderID      uint64
    RemainingQty int64
}

func (e *DefaultExchange) cancelClientOrdersOnBook(...) {
    gw := e.Gateways[client.ID]
    for _, orderID := range append([]uint64{}, client.OrderIDs...) {
        // ... find order, release margin ...
        if order.Side == Buy {
            book.Bids.CancelOrder(orderID)
            e.publishBookUpdate(book, Buy, order.Price)   // ← added
        } else {
            book.Asks.CancelOrder(orderID)
            e.publishBookUpdate(book, Sell, order.Price)  // ← added
        }
        client.RemoveOrder(orderID)
        if gw != nil && gw.IsRunning() {
            select {
            case gw.ResponseCh <- Response{
                Success: true,
                Data: &ForcedCancelNotification{
                    OrderID: orderID, RemainingQty: remainingQty,
                },
            }:
            default:
            }
        }
    }
}
```

**`actor/actor.go`** — decode `ForcedCancelNotification` in `decodeResponse`:

```go
case *exchange.ForcedCancelNotification:
    if val, ok := a.activeOrders.LoadAndDelete(data.OrderID); ok {
        info := val.(*OrderInfo)
        a.requestToOrder.Delete(info.RequestID)
    }
    return &Event{
        Type: EventOrderCancelled,
        Data: OrderCancelledEvent{
            OrderID:      data.OrderID,
            RemainingQty: data.RemainingQty,
        },
    }
```

The MM's existing `onCancelled` handler then cleans up `pending[sym]` and `orderToSym`
correctly, allowing the timer to re-quote on the next tick.

### Why `ForcedCancelNotification` Is a Separate Type

The existing cancel response protocol routes by `RequestID`: the exchange stores
`requestID → orderID` in the actor's `requestToOrder` sync.Map. Forced cancels have no
client-originated `RequestID`, so the existing int64 path would produce
`EventOrderCancelled{OrderID: 0}`, which the MM's handler ignores (early return). A new
struct type carries the `OrderID` directly and is decoded via a dedicated `case` branch,
requiring no changes to the existing request/response contract.

---

## Bug 4 — BasisArbActor: funding-rate carry trader vs price-convergence trader

### File
`simulations/randomwalk/arb.go`

### Symptom
After bugs 1–3 were fixed, the simulation ran the full 900 s, but the price chart showed
a persistent and growing divergence between ABC-PERP and ABC-USD — up to $120 spread
(~24 bps) — despite the arbitrageur being present.

### Root Cause: Category Error in Strategy Design

The original `BasisArbActor` was designed as a **funding-rate carry trader**:

```go
// BEFORE
func (a *BasisArbActor) Start(ctx context.Context) error {
    a.Subscribe(a.cfg.PerpSymbol, exchange.MDFunding)  // ← 30s event
    return a.BaseActor.Start(ctx)
}

func (a *BasisArbActor) onFundingUpdate(e actor.FundingUpdateEvent) {
    rate := e.FundingRate.Rate
    switch {
    case !a.inPosition && rate > a.cfg.OpenThresholdBps:
        // place two market orders
        a.inPosition = true   // ← permanently blocks further action
    case a.inPosition && abs(rate) < a.cfg.CloseThresholdBps:
        // unwind (never reached)
    }
}
```

Five compounding problems:

| # | Problem | Consequence |
|---|---------|-------------|
| 1 | Triggered by `MDFunding` events fired every **30 s** | Taker fires every 100 ms → arb is 300× slower than noise source |
| 2 | `inPosition = true` after first trade | Actor permanently frozen; no further trades for the remaining 870 s |
| 3 | `OpenThresholdBps = 5`; `SimpleFundingCalc.BaseRate = 10` | Rate = 10 bps even at zero price spread → arb triggers immediately at startup, not when basis is actually wide |
| 4 | One 1-BTC trade moves price by $1 per leg ($2 basis reduction); observable basis was $100+ | Single arb trade closes 2% of the spread; 50 sequential trades would be needed |
| 5 | Close condition requires `abs(rate) < 1 bps`; rate stays elevated because basis never closes | `inPosition = false` is never triggered; actor is locked in position for the entire simulation |

The strategy modelled the **economics** of funding correctly (long/short carry based on
periodic settlement) but applied the wrong **mechanism**: a one-shot position entry
rather than continuous price convergence.

### Fix: Continuous Price-Driven Arbitrage

The rewritten actor:

1. **Subscribes to `MDTrade`** on both symbols — updates last-known price on every fill,
   giving ~100 ms price resolution (matching taker cadence).
2. **Checks basis on a 100 ms timer** — decouples price observation from trading
   decision, prevents cascade self-triggering from own fills.
3. **Builds position incrementally** — one lot (1 BTC) per tick, up to `MaxPosition`
   lots, as long as `|basis| > threshold`.
4. **Reduces position incrementally** — one lot per tick when `|basis| < threshold / 2`.
5. **No boolean lock** — position is a signed integer counter; the actor can always add
   or reduce regardless of history.

```go
// AFTER
func (a *BasisArbActor) checkBasis() {
    basis := a.perpPrice - a.spotPrice
    threshold := a.cfg.ThresholdBps * a.spotPrice / 10000

    switch {
    case basis > threshold && a.position < a.cfg.MaxPosition:
        a.SubmitOrder(a.cfg.PerpSymbol, exchange.Sell, exchange.Market, 0, a.cfg.LotSize)
        a.SubmitOrder(a.cfg.SpotSymbol, exchange.Buy,  exchange.Market, 0, a.cfg.LotSize)
        a.position++

    case basis < -threshold && a.position > -a.cfg.MaxPosition:
        a.SubmitOrder(a.cfg.PerpSymbol, exchange.Buy,  exchange.Market, 0, a.cfg.LotSize)
        a.SubmitOrder(a.cfg.SpotSymbol, exchange.Sell, exchange.Market, 0, a.cfg.LotSize)
        a.position--

    case a.position > 0 && basis < threshold/2:
        a.SubmitOrder(a.cfg.PerpSymbol, exchange.Buy,  exchange.Market, 0, a.cfg.LotSize)
        a.SubmitOrder(a.cfg.SpotSymbol, exchange.Sell, exchange.Market, 0, a.cfg.LotSize)
        a.position--

    case a.position < 0 && basis > -threshold/2:
        a.SubmitOrder(a.cfg.PerpSymbol, exchange.Sell, exchange.Market, 0, a.cfg.LotSize)
        a.SubmitOrder(a.cfg.SpotSymbol, exchange.Buy,  exchange.Market, 0, a.cfg.LotSize)
        a.position++
    }
}
```

Config parameters updated in `sim.go`:

```go
// BEFORE
BasisArbConfig{OpenThresholdBps: 5, CloseThresholdBps: 1, PositionSize: btcPrecision}

// AFTER
BasisArbConfig{ThresholdBps: 1, LotSize: btcPrecision, MaxPosition: 30}
```

`ThresholdBps = 1` (≈ $5 at $50k) means the arb reacts to any spread larger than the
MM's natural ±$1 quote offset. `MaxPosition = 30` caps the net exposure at 30 BTC (~$150k
notional at 10% margin), well within the 10 M USD perp balance.

### Effect
- Basis bounded to ±4 bps (was ±24 bps).
- Funding rate stabilised at 7–13 bps average (was climbing monotonically to 30 bps).
- Spot and perp prices visually track each other throughout the full 900 s run.
- Trade count increased (~8,500 perp trades vs ~3,300 before) — the arb now contributes
  meaningful liquidity in both books.

---

## Causal Chain Summary

```
Bug 1 (partial fill unhandled)
  → MM pending[sym] never cleared on partial fills
  → book starved at ~300 s

Bug 2 (reqToSym not cleared in cancelAllForSym)
  → late accepts after cancel create ghost pending entries
  → ghost orders accumulate in book with reserved margin
  → PerpReserved grows → PerpAvailable shrinks
  → after ~450 s: CheckLiquidations triggers

Bug 3 (cancelClientOrdersOnBook silent)
  → liquidation removes orders without BookDelta or cancel notification
  → MM pending never cleaned up
  → book permanently empty, no re-quoting possible

Bug 4 (funding-rate one-shot arb vs price-convergence)
  → independent of bugs 1–3
  → arb fires once at t ≈ 0, locks inPosition = true
  → provides zero corrective force for remaining 870 s
  → spot and perp drift apart driven by independent random taker orders
```

## Files Changed

| File | Change |
|------|--------|
| `simulations/randomwalk/mm.go` | Handle `EventOrderPartialFill`; fix `cancelAllForSym` to clear `reqToSym`; orphan-cancel in `onAccepted` |
| `simulations/randomwalk/arb.go` | Complete rewrite: MDTrade subscription, timer-driven, incremental position |
| `simulations/randomwalk/sim.go` | Updated `BasisArbConfig` fields to match new arb design |
| `exchange/liquidation.go` | Added `ForcedCancelNotification` type; added `publishBookUpdate` and gateway notification to `cancelClientOrdersOnBook` |
| `actor/actor.go` | Handle `*exchange.ForcedCancelNotification` in `decodeResponse` |

---

## Bug 5 — BasisArbActor: MaxPosition cap creates permanent freeze state

### File
`simulations/randomwalk/sim.go`

### Symptom
After bugs 1–4 were fixed, spot and perp prices still diverged for ~2000 seconds at a
time without converging, even though the arb actor was running and had no balance issues
and zero order rejections.

### Investigation
Log analysis confirmed:
- No order rejections anywhere.
- Arb client position never hit `MaxPosition=30` by raw fill count.
- Max observed basis was −18 bps; threshold was 1 bps — arb should have been firing
  continuously.

The apparent contradiction resolved once the four `switch` cases in `checkBasis()` were
read together as a state machine:

| State | Condition | Action |
|-------|-----------|--------|
| `position < MaxPosition` AND `basis > threshold` | open (sell perp, buy spot) | ✓ |
| `position > −MaxPosition` AND `basis < −threshold` | open (buy perp, sell spot) | ✓ |
| `position > 0` AND `basis < threshold/2` | close long | ✓ |
| `position < 0` AND `basis > −threshold/2` | close short | ✓ |
| `position == MaxPosition` AND `basis > threshold` | **none** | ✗ frozen |

The last row is the dead state. Once position reaches 30 lots the arb cannot open
(capped) and cannot close (basis still wide, not below `threshold/2`). It is a spectator
until the random walk organically narrows the spread below half-threshold. With a taker
firing 0.4 BTC every 100 ms across 6 symbols, directional pressure can sustain the
divergence for hundreds or thousands of seconds before reverting.

### Fix
Raised `MaxPosition` from 30 to 500 in `sim.go`.

```go
// BEFORE
MaxPosition: 30

// AFTER
MaxPosition: 500
```

Arb accounts hold 1 000 BTC spot + $100 M spot USD + $10 M perp USD. 500 BTC at $50k =
$25 M notional, well within available capital.

### How Good
- Directly addresses the symptom: arb now has 16× more capacity before hitting the cap.
- At 0.4 BTC taker per tick × 100 ms, it would take ~125 seconds of fully one-directional
  taker flow to saturate 500 lots — a highly unlikely sustained condition.
- Zero code-logic changes; pure configuration. Easy to reason about.

### How Bad
- It is a **band-aid**, not a structural fix. A sufficiently long directional random walk
  will still hit 500 and reproduce the freeze. The correct solution is either:
  1. **Dynamic position limit** computed from available margin at runtime, or
  2. **No hard cap** — let the arb trade until it is genuinely rejected by the exchange
     for insufficient balance, and handle `EventOrderRejected` to pause.
- Raising the cap also increases the arb's P&L variance: a 500-lot position at $50k/BTC
  carries $25 M delta exposure during the hold period. In a real system this would require
  explicit risk approval.
- The freeze-state logic (open condition and close condition use different thresholds:
  `threshold` vs `threshold/2`) remains. This hysteresis band was intentional to prevent
  oscillation, but it means the arb will always hold a residual position even after the
  basis narrows to exactly threshold, silently accumulating unrealised P&L.

---

## Bug 6 — FundingArbActor missing `SetTickerFactory`

### File
`simulations/randomwalk/sim.go`

### Symptom
Latent — no observable failure at the time of discovery. `FundingArbActor` was wired
into the simulation without `SetTickerFactory(timerFact)` being called on it, unlike
every other actor.

### Root Cause
`actor.NewBaseActor` initialises `tickerFactory` to `&exchange.RealTickerFactory{}` (the
real wall-clock). `SetTickerFactory` must be called explicitly before `Start()` to
override it with the simulated factory. The three basis arb actors (clients 5–7) had the
call; the three funding arb actors (clients 8–10) did not.

`FundingArbActor` has no tickers today — it is purely event-driven via `MDFunding`
subscriptions — so the missing call had no effect. However, any future addition of a
`AddTicker` call inside `NewFundingArbActor` would silently use the real clock. In a
simulation that completes in <2 seconds of wall time, a 100 ms real-time ticker would
fire 0–19 times instead of 9 000 times, producing behaviour indistinguishable from the
actor being completely inactive.

### Fix
```go
// AFTER (sim.go, funding arb construction loop)
arb.SetTickerFactory(timerFact)
fundingArbs = append(fundingArbs, arb)
```

### How Good
- Eliminates a class of silent failure. Any ticker added to `FundingArbActor` in the
  future will automatically use simulated time without requiring the author to remember
  to update `sim.go`.
- One line; zero risk.

### How Bad
- The root cause is architectural: `NewBaseActor` defaults to the real clock, and callers
  must remember to call `SetTickerFactory`. This is an invisible contract — nothing in
  the type system or compiler enforces it. The correct fix is to require the ticker
  factory as a constructor parameter of `BaseActor` (or at minimum `NewFundingArbActor`),
  so the sim clock is injected at construction time and omission is a compile error.
- Until that refactor happens, every new actor added to `NewSim` is one forgotten line
  away from silently running on the real clock.

---

## Finding 7 — Absolute tick size creates systematic scale asymmetry across assets

### Observation
After fixing bugs 1–6, the price chart still showed ABC evolving much more slowly and
flatly than DEF and GHI. Trade counts remained ~4× lower for ABC (~250) vs DEF/GHI
(~1 000). Investigation confirmed the simulation mechanics (timers, arb logic, taker
distribution) were all correct. The root cause is a scale mismatch baked into the asset
configuration.

### Root Cause: All Assets Share One Absolute Tick Size

All three instruments are configured with `tickSize = exchange.DOLLAR_TICK` ($1 per
tick), but their prices differ by 333×:

| Asset | Price | 1 tick | Relative move per tick |
|-------|-------|--------|------------------------|
| ABC | $50 000 | $1 | **0.002 %** |
| DEF | $3 000 | $1 | 0.033 % |
| GHI | $150 | $1 | **0.67 %** |

Because every simulation parameter that should be *relative* is expressed in absolute
dollar terms, GHI moves 333× more per tick than ABC in percentage terms. This single
fact propagates into every subsystem:

**BasisArb threshold (`arb.go`)**
```go
threshold := a.cfg.ThresholdBps * a.spotPrice / 10000  // 1 bps × price
```
The threshold is 1 bps of price, but $1 tick is 0.002 bps of ABC and 67 bps of GHI.
So the arb fires after almost every GHI taker trade and almost never for ABC.

**FundingCalc integer truncation (`instrument/funding.go`)**
```go
premium := ((markPrice - indexPrice) * 10000) / indexPrice
```
A 1-tick ($1) mark/index divergence on ABC:
`(100 000 × 10 000) / 5 000 000 000 = 0` (integer division truncates to zero).
The same divergence on GHI:
`(100 000 × 10 000) / 15 000 000 = 66` → rate hits `MaxRate = 75` cap.
ABC funding is permanently flat at `BaseRate = 10 bps`; GHI is permanently capped.

**Visual appearance**
On an absolute price chart, ABC's ±$5 random walk looks static next to GHI's ±$10
swings, even though both represent the same number of ticks. The chart misleadingly
suggests ABC simulation time is "slower."

**Taker volume asymmetry**
The original taker used a fixed base quantity (0.4 BTC per order), meaning the same
physical book impact but wildly different notional values ($20 000 for ABC, $60 for GHI).
Addressed by switching to a fixed quote notional in `taker.go`, but the downstream arb
and funding issues persist until tick sizes are also normalised.

### The Correct Fix (not yet applied)

Make tick sizes proportional to price so that one tick represents the same relative move
across all assets:

```go
var assets = []assetSpec{
    {name: "ABC", price: 50_000 * exchange.DOLLAR_TICK, tickSize: 50 * exchange.DOLLAR_TICK}, // ~0.10 %
    {name: "DEF", price:  3_000 * exchange.DOLLAR_TICK, tickSize:  3 * exchange.DOLLAR_TICK}, // ~0.10 %
    {name: "GHI", price:    150 * exchange.DOLLAR_TICK, tickSize:      exchange.CENT_TICK  }, // ~0.07 %
}
```

With proportional tick sizes, BasisArb threshold, FundingCalc premium, and chart
volatility all behave consistently across instruments.

### Key Principle

Any simulation parameter expressed in absolute price units (tick size, lot size, arb
threshold, funding damping) will behave inconsistently across instruments whose prices
differ significantly. Parameters that encode economic significance must be relative
(bps, % of price) or must be set per-instrument to achieve the intended relative
magnitude.
