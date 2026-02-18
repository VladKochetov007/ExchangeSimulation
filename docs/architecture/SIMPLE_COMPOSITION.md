# Simple Actor Composition (The Right Way)

**Date**: 2026-02-16
**Problem**: Gateway wrappers and complex forwarding made things too complicated
**Solution**: Keep it simple - actors make decisions using shared state

## Core Principle: Actor-First

**Actors make their own decisions.** They just need access to shared information.

## What You Actually Need

### 1. Shared Balance (Already Works)

```go
type SharedContext struct {
    baseBalances map[string]int64
    quoteBalance int64
    // ... rest
}
```

Sub-actors can query: "How much balance does the group have?"

### 2. Two-Level OMS (Already Works)

```go
// Individual: What's MY position?
myOMS := ctx.GetActorOMS(myID, symbol)
myPosition := myOMS.GetNetPosition(symbol)

// Composite: What's OUR total position?
compositeOMS := ctx.GetCompositeOMS(symbol)
totalPosition := compositeOMS.GetNetPosition(symbol)
```

Sub-actors can ask: "What's the group's total exposure?"

### 3. Smart Decision Making (This is the key!)

**Funding arb example**:

```go
func (fa *FundingArb) shouldEnter(ctx *SharedContext) bool {
    // Check composite position
    compositeOMS := ctx.GetCompositeOMS(fa.spotSymbol)
    totalSpotPos := compositeOMS.GetNetPosition(fa.spotSymbol)

    // Smart decision:
    // - If composite already long spot → don't need to buy more
    // - If composite flat or short → need to establish position

    needToEstablish := totalSpotPos < fa.targetPosition
    return fa.fundingRate > fa.threshold && needToEstablish
}
```

**This is actor-first**: The funding arb actor makes its own decision, it just looks at shared state.

## Single Exchange: Use CompositeActor (Current Design)

```go
composite := actor.NewCompositeActor(id, gateway, []actor.SubActor{
    fundingArb,
    triangleArb,
    marketMaker,
})
```

**How it works**:
1. All sub-actors share ONE gateway (same client ID)
2. All sub-actors share balance via `SharedContext`
3. Sub-actors see both individual and composite OMS
4. Sub-actors make smart decisions based on shared state

**No gateway wrapping. No complex forwarding. Simple.**

## Multi-Exchange: Use Multiple Actors (Not Composition!)

**Wrong approach** (what I built):
- Complex `ActorGroup` with gateway wrappers
- Forwarding logic for all message types
- Hard to understand and maintain

**Right approach**:
```go
// Binance client
binanceGateway := binanceEx.ConnectClient(clientID1, balances, fees)
binanceActor := NewArbitrageActor(1, binanceGateway, ...)

// FTX client
ftxGateway := ftxEx.ConnectClient(clientID2, balances, fees)
ftxActor := NewArbitrageActor(2, ftxGateway, ...)

// Share state between them if needed
sharedState := &CrossExchangeState{
    binancePosition: 0,
    ftxPosition: 0,
}

binanceActor.SetSharedState(sharedState)
ftxActor.SetSharedState(sharedState)
```

**Key insight**: Multi-exchange is just **multiple client connections**, not special composition.

## Why This Is Simpler

### Old (Complex) Way
```
Actor → WrappedGateway → Intercept → Validate → Forward → Gateway → Exchange
                ↓
         Track symbols
                ↓
         Update balances
```

### New (Simple) Way
```
Actor → Check SharedContext → Make Decision → Submit Order → Gateway → Exchange
                                                                ↓
                                                        Update SharedContext on fill
```

## Implementation: What to Keep, What to Delete

### ✅ Keep

**`actor/shared_context.go`** - Core shared state:
```go
type SharedContext struct {
    baseBalances map[string]int64
    quoteBalance int64
    compositeOMS map[string]*NettingOMS
    actorOMS     map[uint64]map[string]*NettingOMS
}
```

**`actor/composite_actor.go`** - Simple composition:
```go
type CompositeActor struct {
    subActors []SubActor
    context   *SharedContext
    symbolRouting map[string][]SubActor
}
```

### ❌ Delete (Overengineered)

- `actor/gateway_wrapper.go` - Too complex, not needed
- `actor/general_composite.go` - Over-abstraction

### 🔧 Simplify

**`SubActor` interface** - Just needs events and submit function:
```go
type SubActor interface {
    OnEvent(event *Event, ctx *SharedContext, submit OrderSubmitter)
    GetSymbols() []string
    GetID() uint64
}
```

No gateway wrapping. No message forwarding. **Actor receives events and makes decisions.**

## Example: Smart Funding Arbitrage

```go
type InternalFundingArb struct {
    id         uint64
    spotSymbol string
    perpSymbol string
    spotOMS    *NettingOMS  // My individual tracking
    perpOMS    *NettingOMS
}

func (ifa *InternalFundingArb) OnEvent(event *Event, ctx *SharedContext, submit OrderSubmitter) {
    switch event.Type {
    case EventFundingUpdate:
        ifa.evaluateStrategy(ctx, submit)
    case EventOrderFilled:
        ifa.onFill(event, ctx)
    }
}

func (ifa *InternalFundingArb) evaluateStrategy(ctx *SharedContext, submit OrderSubmitter) {
    // 1. Check composite position
    compositeSpotOMS := ctx.GetCompositeOMS(ifa.spotSymbol)
    compositePerpOMS := ctx.GetCompositeOMS(ifa.perpSymbol)

    totalSpotPos := compositeSpotOMS.GetNetPosition(ifa.spotSymbol)
    totalPerpPos := compositePerpOMS.GetNetPosition(ifa.perpSymbol)

    // 2. Smart decision: What do I need to do?
    targetSpotPos := 100_000_000  // 1.0 BTC long spot
    targetPerpPos := -100_000_000 // 1.0 BTC short perp

    needSpot := targetSpotPos - totalSpotPos
    needPerp := targetPerpPos - totalPerpPos

    // 3. Only submit what's needed
    if ifa.fundingRate > ifa.threshold {
        if needSpot > 0 {
            submit(ifa.spotSymbol, exchange.Buy, exchange.Market, 0, needSpot)
        }
        if needPerp < 0 {
            submit(ifa.perpSymbol, exchange.Sell, exchange.Market, 0, -needPerp)
        }
    }
}

func (ifa *InternalFundingArb) onFill(event *Event, ctx *SharedContext) {
    fill := event.Data.(OrderFillEvent)

    // Update my individual OMS
    if fill.Side == exchange.Buy {
        ifa.spotOMS.OnFill(ifa.spotSymbol, fill, 100_000_000)
    } else {
        ifa.perpOMS.OnFill(ifa.perpSymbol, fill, 100_000_000)
    }

    // SharedContext automatically updates composite OMS
    // (already done in CompositeActor.OnEvent)
}
```

**This is simple**: Actor looks at total position, calculates what it needs, submits orders.

## Multi-Venue: Even Simpler

**You don't need composition for multi-venue.** Use `MultiVenueGateway` (already exists):

```go
// MultiVenueGateway already in codebase
venues := simulation.NewVenueRegistry()
venues.RegisterVenue("binance", binanceEx)
venues.RegisterVenue("ftx", ftxEx)

multiGateway := simulation.NewMultiVenueGateway(clientID, venues)

// Actor just uses it
actor := NewCrossExchangeArb(id, multiGateway, ...)
```

The actor routes orders: `submit("binance:BTC/USD", ...)` vs `submit("ftx:BTC/USD", ...)`

**No special composition needed.**

## Summary: What Changed

### Before (Complicated)
- Gateway wrappers intercept all messages
- Complex forwarding logic
- Hard to understand

### After (Simple)
- Actors query `SharedContext` for group state
- Actors make smart decisions based on total exposure
- Submit orders directly through shared submit function
- `CompositeActor` just routes events and updates shared state

### For Multi-Exchange
- **Not composition** - just multiple client connections
- Use `MultiVenueGateway` if routing within one actor
- Use separate actors if truly independent strategies

## Action Items

1. **Keep** `SharedContext` - it's good
2. **Keep** `CompositeActor` - it's simple
3. **Delete** `gateway_wrapper.go` - overengineered
4. **Delete** `general_composite.go` - unnecessary
5. **Simplify** sub-actors to just query shared state and make decisions

**Back to actor-first. Back to simple.**
