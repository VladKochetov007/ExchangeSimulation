# Implementation Plan: Multi-Exchange Simulation Bug Fixes

## Executive Summary

Fix three critical bugs in the multi-exchange simulation that cause incorrect balance distribution, missing perp/spot validation logic, and premature LP exits.

**Total Actors**: 27 (18 LPs/MMs + 9 Takers across 3 exchanges)
- 3 exchanges × (2 LPs + 4 MMs) = 18 LP/MM actors
- 9 symbol occurrences × 1 Taker = 9 Taker actors

**Target Balance**: 1M USD equivalent total for ALL actors (currently 63×-68× inflated)

---

## Bug 1: CRITICAL - Balance Multiplication (63× Inflation)

### Problem
Each of 27 actors receives FULL `InitialBalances` instead of their divided share:
- Config: 100M USD per actor
- Current total: 27 × 100M = 2.7B USD (should be 1M USD total)
- Inflation factor: 2,700×

### Root Cause
Lines 144, 183, 224 in `simulation/multi_runner.go`:
```go
gateway := ex.ConnectClient(actorID, r.config.InitialBalances, feePlan)
```

This passes the FULL balance map to every actor instead of a divided share.

### Solution Strategy

**Option A: Divide at Connection Time (Recommended)**
Pre-calculate divided balances once, pass to each `ConnectClient()` call.

**Option B: Divide in Config**
Modify `InitialBalances` in config to already contain per-actor amounts.

**Recommendation**: Option A - More transparent, keeps config as "total pool" semantics.

### Implementation Steps

1. **Calculate total actors before creating them**
   - Count: `numExchanges × (LPsPerSymbol + MMsPerSymbol) + totalSymbols × TakersPerSymbol`
   - Location: After line 42 in `multi_runner.go` (after exchanges created, before actors)

2. **Create balance division helper function**
```go
// In multi_runner.go, before createActorsForExchange
func divideBalances(total map[string]int64, numActors int) map[string]int64 {
    divided := make(map[string]int64)
    for asset, amount := range total {
        divided[asset] = amount / int64(numActors)
    }
    return divided
}
```

3. **Replace all ConnectClient calls** (lines 144, 183, 224)
```go
// OLD:
gateway := ex.ConnectClient(actorID, r.config.InitialBalances, feePlan)

// NEW:
gateway := ex.ConnectClient(actorID, dividedBalances, feePlan)
```

4. **Adjust LP balance initialization** (lines 149-155)
Currently `SetBalances()` uses full config amounts - update to use divided amounts.

### Validation
- Query total exchange balance across all clients = 1M USD equivalent
- Each actor should have ~37K USD (1M ÷ 27)
- Base assets divided proportionally (BTC: 100 ÷ 27 ≈ 3.7 BTC per actor)

---

## Bug 2: HIGH - No Spot/Perp Distinction in Validation

### Problem
Sell order validation (lines 253-286 in `exchange/exchange.go`) requires base asset balance for BOTH spot and perp instruments:
```go
// Line 254-261: Market sell validation
} else {
    if client.GetAvailable(instrument.BaseAsset()) < req.Qty {
        // REJECT
    }
}

// Line 276-284: Limit sell validation
} else {
    asset = instrument.BaseAsset()
    if !client.Reserve(asset, req.Qty) {
        // REJECT
    }
}
```

**Spot**: MUST have base asset (can't sell BTC without owning BTC)
**Perp**: Can short without base asset (uses margin, tracked in PositionManager)

### Solution Strategy

Add `instrument.IsPerp()` branching to sell order validation logic.

### Implementation Steps

1. **Market Sell Orders (line 253-262)**
```go
} else { // Sell side
    // Spot requires base asset, perp does not (can short)
    if !instrument.IsPerp() {
        if client.GetAvailable(instrument.BaseAsset()) < req.Qty {
            putOrder(order)
            resp := Response{RequestID: req.RequestID, Success: false, Error: RejectInsufficientBalance}
            if log != nil {
                log.LogEvent(e.Clock.NowUnixNano(), clientID, "OrderRejected", resp)
            }
            return resp
        }
    }
    // Perp: no balance check needed for sell (margin system handles it)
}
```

2. **Limit Sell Orders (line 275-285)**
```go
} else { // Sell side
    // Spot requires base asset reservation, perp does not
    if !instrument.IsPerp() {
        asset = instrument.BaseAsset()
        if !client.Reserve(asset, req.Qty) {
            putOrder(order)
            resp := Response{RequestID: req.RequestID, Success: false, Error: RejectInsufficientBalance}
            if log != nil {
                log.LogEvent(e.Clock.NowUnixNano(), clientID, "OrderRejected", resp)
            }
            return resp
        }
    }
    // Perp: no reservation needed (position tracked separately)
}
```

3. **Order Cancellation (line 420-423)**
Verify release logic only applies to spot:
```go
} else { // Sell side
    if !instrument.IsPerp() {
        client.Release(instrument.BaseAsset(), remainingQty)
    }
    book.Asks.cancelOrder(req.OrderID)
    e.publishBookUpdate(book, Sell, order.Price)
}
```

4. **Partial Fill Release (line 357-359)**
```go
} else { // Sell side
    if !instrument.IsPerp() {
        client.Release(instrument.BaseAsset(), order.Qty-order.FilledQty)
    }
}
```

### Validation
- Perp sell order with 0 base balance: ACCEPTED
- Spot sell order with 0 base balance: REJECTED (InsufficientBalance)
- Position tracking works correctly for perps (check PositionManager)

---

## Bug 3: HIGH - FirstLP MinExitSize=0 Causes Premature Exit

### Problem
`MultiSymbolLP` created without `MinExitSize` (line 136 in `multi_runner.go`):
```go
lpConfig := actor.MultiSymbolLPConfig{
    Symbols:           symbols,
    Instruments:       instruments,
    SpreadBps:         r.config.LPSpreadBps,
    BootstrapPrices:   bootstrapPrices,
    LiquidityMultiple: 10,
    // MinExitSize: MISSING - defaults to 0
}
```

From `composite.go` line 416-418:
```go
if absExposure < m.config.MinExitSize {
    return // Skip exit check
}
```

**With MinExitSize=0**: Every fill triggers exit evaluation
**Impact**: LPs exit after minimal exposure, destroying liquidity

### Solution Strategy

Set `MinExitSize` to meaningful threshold based on typical order size.

### Calculation
- User request: "taking volume around the size of first level of the book"
- Typical LP quotes: ~0.5-1 BTC (50M-100M satoshis)
- Recommended: `MinExitSize = 50 * exchange.SATOSHI` (50M satoshis = 0.5 BTC)

### Implementation Steps

1. **Add MinExitSize to config** (line 136 in `multi_runner.go`)
```go
lpConfig := actor.MultiSymbolLPConfig{
    Symbols:           symbols,
    Instruments:       instruments,
    SpreadBps:         r.config.LPSpreadBps,
    BootstrapPrices:   bootstrapPrices,
    LiquidityMultiple: 10,
    MinExitSize:       50 * exchange.SATOSHI, // 0.5 BTC minimum
}
```

2. **Alternative: Make it configurable** (Optional Enhancement)
Add to `MultiSimConfig` in `simulation/config.go`:
```go
type MultiSimConfig struct {
    // ... existing fields ...
    LPMinExitSize int64 // Minimum position size before LP considers exiting
}
```

Then use: `MinExitSize: r.config.LPMinExitSize`

### Validation
- LPs should maintain positions through small fills
- Exit only when position ≥ 0.5 BTC AND counter-liquidity ≥ 10× exposure
- Log exit events to verify threshold works

---

## Implementation Order

1. **Bug 1 First** (Balance Division)
   - Foundation for correct testing
   - Prevents cascading balance issues
   - Easier to validate other bugs with correct balances

2. **Bug 2 Second** (Spot/Perp Validation)
   - Now actors have correct balances to test with
   - Can verify perp shorting works
   - Can verify spot requires base assets

3. **Bug 3 Last** (MinExitSize)
   - Needs running simulation to observe exit behavior
   - Benefits from correct balances and perp logic

---

## Testing Strategy

### Unit Tests
1. Balance division math (1M ÷ 27 = ~37K per actor)
2. Perp sell without base asset (should ACCEPT)
3. Spot sell without base asset (should REJECT)
4. MinExitSize threshold (< 0.5 BTC = no exit)

### Integration Tests
1. Run 10-second simulation
2. Query all actor balances (total should equal initial - fees)
3. Verify perp shorts execute successfully
4. Verify LP positions grow beyond MinExitSize before exiting

### Validation Queries
```go
// After simulation:
totalUSD := 0
for _, client := range exchange.Clients {
    totalUSD += client.Balances["USD"]
}
// Should be ≈ 1M - trading fees
```

---

## Edge Cases to Consider

### Bug 1 Edge Cases
- **Rounding errors**: 1M ÷ 27 = 37,037.037... USD
  - Solution: Integer division, accept small remainder
- **Unequal actor counts**: If config changes actor ratios
  - Solution: Calculate dynamically, don't hardcode 27

### Bug 2 Edge Cases
- **FOK orders on perp**: Should still validate quantity
- **Position limits**: Perp shorts may need max position checks (future feature)
- **Margin requirements**: Currently no margin model (positions unlimited)

### Bug 3 Edge Cases
- **Multiple symbols**: MinExitSize is per-symbol position
- **Cross-position netting**: Currently each symbol tracked independently
- **Zero MinExitSize**: Valid for aggressive exit strategies

---

## Rollback Strategy

If bugs introduce new issues:

### Bug 1 Rollback
```go
// Revert to full balances (temporary)
gateway := ex.ConnectClient(actorID, r.config.InitialBalances, feePlan)
```

### Bug 2 Rollback
```go
// Remove IsPerp() checks, revert to original validation
if client.GetAvailable(instrument.BaseAsset()) < req.Qty {
    // reject
}
```

### Bug 3 Rollback
```go
// Set MinExitSize to 0 (original behavior)
MinExitSize: 0,
```

---

## Performance Impact

### Bug 1 (Balance Division)
- **Impact**: None (one-time calculation)
- **Memory**: Negligible (1 extra map allocation)

### Bug 2 (Spot/Perp Branching)
- **Impact**: Minimal (one boolean check per order)
- **Hot path**: Yes, but trivial overhead

### Bug 3 (MinExitSize)
- **Impact**: Reduces exit evaluations (performance IMPROVEMENT)
- **Frequency**: Exit check runs every 100ms per LP

---

## Success Criteria

1. **Bug 1 Fixed**
   - Total system balance ≤ 1.1M USD (allowing fees)
   - Each actor has 20K-50K USD (depending on PnL)

2. **Bug 2 Fixed**
   - Perp actors can short without base asset
   - Spot actors cannot short without base asset
   - No false rejections on perp orders

3. **Bug 3 Fixed**
   - LPs maintain positions through small fills
   - LPs only exit when position ≥ MinExitSize
   - Order book maintains depth during simulation

---

## Critical Files for Implementation

- `simulation/multi_runner.go` - Balance division (Bug 1), MinExitSize config (Bug 3)
- `exchange/exchange.go` - Spot/perp validation branching (Bug 2)
- `simulation/config.go` - Optional: Add LPMinExitSize config field (Bug 3)
- `actor/composite.go` - Reference for MinExitSize usage pattern (Bug 3)
- `exchange/test_helpers.go` - Constants for SATOSHI precision (Bug 3)
