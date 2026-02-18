# CompositeActor Test Coverage

## Test Summary

All 10 tests pass ✓

### Event Routing Tests

**TestCompositeActorEventRouting** - Symbol-based routing
- Events with symbols (BookSnapshot) are routed only to sub-actors subscribed to that symbol
- Sub-actors not subscribed don't receive events
- Multiple sub-actors can subscribe to same symbol

**TestCompositeActorBroadcastEvents** - Broadcast to all actors
- Order-related events (OrderAccepted) are broadcast to ALL sub-actors
- All actors receive event regardless of their symbol subscriptions

**TestCompositeActorOrderRejection** - Rejection handling
- OrderRejected events are broadcast to all sub-actors
- Rejection reasons (RejectInsufficientBalance) are preserved
- RequestID is correctly forwarded

**TestCompositeActorTradeEventRouting** - Trade event routing
- Trade events are routed by symbol (NOT broadcast)
- Only interested actors receive trades

**TestCompositeActorMultipleSymbols** - Multi-symbol actors
- Sub-actors can subscribe to multiple symbols
- Each receives only events for their subscribed symbols

### SharedContext Tests

**TestSharedContextBalanceTracking** - Balance updates
- Initial balances set correctly
- Fills update both base and quote balances
- Buy: increases base, decreases quote (by notional + fee)
- Sell: decreases base, increases quote (by notional - fee)

**TestSharedContextOMSIntegration** - Two-level OMS
- Actor-level OMS tracks individual positions
- Composite-level OMS tracks aggregate positions
- Both are updated on fills

**TestSharedContextMultipleActors** - Multiple actors sharing context
- Multiple actors can use same SharedContext
- Each actor's OMS tracks their position separately
- Composite OMS shows sum of all actor positions
- Total balance reflects all actors' trades

**TestSharedContextCanSubmitOrder** - Position limits
- Validates orders against max inventory limits
- Allows orders that stay within limits
- Rejects orders that would exceed limits
- Correctly handles both buy and sell sides

**TestSharedContextQuoteReservation** - Quote balance reservation
- Can reserve quote balance for pending orders
- Available balance decreases when reserved
- Cannot over-reserve
- Releasing reservation increases available balance

## What Gets Routed vs Broadcast

### Routed by Symbol (to interested actors only)
```
✓ EventBookSnapshot
✓ EventBookDelta
✓ EventFundingUpdate
✓ EventTrade
✓ EventOpenInterest
```

### Broadcast to ALL Sub-Actors
```
✓ EventOrderAccepted
✓ EventOrderRejected (with RejectReason)
✓ EventOrderFilled
✓ EventOrderPartialFill
✓ EventOrderCancelled
✓ EventOrderCancelRejected
```

## Coverage Verified

- ✅ Event routing works correctly
- ✅ Order rejections reach sub-actors with reasons
- ✅ Balance tracking accurate for buy and sell
- ✅ OMS integration at both levels works
- ✅ Multiple actors can share context
- ✅ Position limits enforced correctly
- ✅ Quote reservation system works
- ✅ Trade and OpenInterest events routed by symbol

## Run Tests

```bash
# All composition tests
go test -v ./actor -run "TestComposite|TestShared"

# Specific test
go test -v ./actor -run TestCompositeActorEventRouting

# Full suite
make test
```
