# Microstructure V1 Simulation - Implementation Plan

**Date**: 2026-02-16
**Status**: Approved - Ready for Implementation
**Estimated Time**: 8.5 hours

## Executive Summary

Create a production-grade, comprehensive microstructure simulation with:
- **15 Markets**: 10 spot (5 USD-quoted + 4 ABC-quoted) + 5 perpetual futures
- **186 Actors**: Market makers, arbitrageurs, takers across all markets
- **OMS Integration**: Position tracking to prevent rejected orders
- **Fee-Aware**: All strategies profitable after commissions
- **Composite Actors**: General pattern for multi-symbol/multi-strategy actors

## Key Innovation: General Composite Actor Pattern

**Current State** (`actor/composite.go`):
- `MultiSymbolLP` - FirstLP for multiple symbols
- `MultiSymbolMM` - Pure MM for multiple symbols

**Proposed Enhancement**:
Create **general composition framework** that works with ANY actor type:

```go
// Generic composite actor
type CompositeActor struct {
    *actor.BaseActor
    subActors    []actor.Actor           // Any actor type
    symbolRouting map[string]actor.Actor  // Route events by symbol
    sharedState  interface{}             // Optional shared state
}
```

**Benefits**:
1. **Mix actor types**: AS market maker + funding arb + triangle arb in one composite
2. **Shared resources**: One gateway, shared balances, unified OMS
3. **Coordinated strategies**: Sub-actors can communicate via shared state
4. **Simpler configuration**: One actor ID instead of many

**Examples**:
- Composite with 3 PureMM (different symbols) + 1 FundingArb
- Composite with 5 AvellanedaStoikov (different parameters) for same symbol
- Composite with TriangleArb + 2 SpotMM (provides own liquidity for arb)

## Asset and Market Structure

### Assets and Prices
```go
assets := []string{"ABC", "BCD", "CDE", "DEF", "EFG"}

bootstrapPrices := map[string]int64{
    "ABC": 50_000,  // $50,000
    "BCD": 25_000,  // $25,000
    "CDE": 10_000,  // $10,000
    "DEF": 5_000,   // $5,000
    "EFG": 1_000,   // $1,000
}

const (
    ASSET_PRECISION = 100_000_000  // 8 decimals (satoshi-like)
    USD_PRECISION   = 100_000      // cents
    CENT_TICK       = 1            // $0.01 tick
)
```

### Market Structure (15 total)

**Spot Markets (10)**:
1. USD-quoted (5): ABC/USD, BCD/USD, CDE/USD, DEF/USD, EFG/USD
2. ABC-quoted (4): BCD/ABC, CDE/ABC, DEF/ABC, EFG/ABC

**Perpetual Futures (5)**:
- ABC-PERP, BCD-PERP, CDE-PERP, DEF-PERP, EFG-PERP (all vs USD)

**Cross-rate calculation**:
- BCD/ABC price = BCD/USD / ABC/USD
- Enables triangle arbitrage opportunities

## Actor Configuration (186 total)

### 1. Avellaneda-Stoikov Market Makers (75 actors)

**5 configs per instrument × 15 instruments = 75 actors**

```go
asConfigs := []AvellanedaStoikovConfig{
    {Gamma: 100, RequoteInterval: 500*ms, VolatilityWindow: 20},
    {Gamma: 200, RequoteInterval: 750*ms, VolatilityWindow: 15},
    {Gamma: 300, RequoteInterval: 1000*ms, VolatilityWindow: 10},
    {Gamma: 400, RequoteInterval: 1250*ms, VolatilityWindow: 25},
    {Gamma: 500, RequoteInterval: 1500*ms, VolatilityWindow: 30},
}
```

**Per-instrument settings**:
- K = 10000 (spread control)
- T = 3600 (1 hour horizon)
- QuoteQty = 0.1 × asset precision
- MaxInventory = varies by asset (5-10× precision)

### 2. Pure Market Makers (75 actors)

**5 configs per instrument × 15 instruments = 75 actors**

```go
pureMMConfigs := []PureMarketMakerConfig{
    {SpreadBps: 5,  MonitorInterval: 50*ms},   // Tight, fast
    {SpreadBps: 10, MonitorInterval: 100*ms},  // Medium
    {SpreadBps: 20, MonitorInterval: 150*ms},  // Wide
    {SpreadBps: 35, MonitorInterval: 175*ms},  // Very wide
    {SpreadBps: 50, MonitorInterval: 200*ms},  // Ultra wide
}
```

**Features**:
- Fixed spreads (no volatility adaptation)
- Requote on timer OR when mid-price moves > threshold
- Simple inventory management

### 3. Composite Multi-Symbol Market Makers (10 actors)

**Using existing `MultiSymbolMM` from `actor/composite.go`**:

```go
// 2 actors: All USD-spot pairs (10 bps)
NewMultiSymbolMM(id, gateway, MultiSymbolMMConfig{
    Symbols:     []string{"ABC/USD", "BCD/USD", "CDE/USD", "DEF/USD", "EFG/USD"},
    SpreadBps:   10,
    QuoteSize:   0.1 * ASSET_PRECISION,
    MaxInventory: 10 * ASSET_PRECISION,
})

// 2 actors: All perp pairs (15 bps)
NewMultiSymbolMM(id, gateway, MultiSymbolMMConfig{
    Symbols:     []string{"ABC-PERP", "BCD-PERP", "CDE-PERP", "DEF-PERP", "EFG-PERP"},
    SpreadBps:   15,
    QuoteSize:   0.1 * ASSET_PRECISION,
    MaxInventory: 5 * ASSET_PRECISION,
})

// 2 actors: BCD/ABC, CDE/ABC (20 bps)
// 2 actors: DEF/ABC, EFG/ABC (25 bps)
// 2 actors: Mixed ABC spot+perp
```

**Advantages**:
- One gateway per composite (not per symbol)
- Shared balance across symbols
- Coordinated risk management
- Simpler logging (one actor ID)

### 4. Funding Arbitrage Actors (5 actors)

**One per asset, improved with OMS integration**:

```go
type InternalFundingArbConfig struct {
    SpotSymbol      string  // "ABC/USD"
    PerpSymbol      string  // "ABC-PERP"
    MinFundingRate  int64   // Enter when rate > 20 bps
    ExitFundingRate int64   // Exit when rate < 5 bps
    MaxPositionSize int64

    // OMS for position tracking
    SpotOMS *actor.NettingOMS
    PerpOMS *actor.NettingOMS
}
```

**Strategy**:
1. Monitor perp funding rate
2. When rate > threshold + fees: Buy spot + short perp
3. Collect funding payments (shorts receive payment when funding > 0)
4. When rate < exit: Close both positions
5. Use OMS to validate positions before entry

**Fee awareness**:
- Only enter when: `fundingRate > (2 × takerFee) + profitMargin`
- Example: Enter at 15 bps (10 bps fees + 5 bps profit)

### 5. Triangle Arbitrage Actor (1 actor)

**Detect arbitrage in ABC-quoted triangles**:

```go
type TriangleArbConfig struct {
    USDBalance   int64
    BaseAssets   []string  // ["BCD", "CDE", "DEF", "EFG"]
    ThresholdBps int64     // Min profit after fees (e.g., 20 bps)
    MaxTradeSize int64
}
```

**Example triangle** (USD → ABC → BCD → USD):
```
Path: Buy ABC/USD, Buy BCD/ABC, Sell BCD/USD
Profit = (Price_ABC/USD × Price_BCD/ABC × Price_USD/BCD) - 1

Execute if: profit > (3 × takerFee) + threshold
Example: profit > 20 bps (15 bps fees + 5 bps profit)
```

**Triangles to monitor**:
- USD → ABC → BCD → USD
- USD → ABC → CDE → USD
- USD → ABC → DEF → USD
- USD → ABC → EFG → USD

### 6. Randomized Takers (20 actors)

**2 per asset × 5 assets = 10 spot takers**:
```go
RandomizedTakerConfig{
    Symbol:    "ABC/USD",  // or BCD/USD, etc.
    Interval:  random(500ms, 2000ms),
    MinQty:    0.01 * ASSET_PRECISION,
    MaxQty:    0.5 * ASSET_PRECISION,
}
```

**2 per asset × 5 assets = 10 perp takers**:
- Same config but for perp symbols

**Total Takers**: 20

## OMS Integration Strategy

### Why OMS is Critical

**Problem**: Without OMS, actors can:
- Submit orders that exceed position limits (rejected)
- Not know current position (can't rebalance)
- Over-trade and accumulate unwanted inventory

**Solution**: Every actor integrates `actor.NettingOMS`

### OMS Integration Pattern

```go
type EnhancedActor struct {
    *actor.BaseActor
    oms    map[string]*actor.NettingOMS  // One per symbol
    config ActorConfig
}

// On every fill, update OMS
func (a *EnhancedActor) onOrderFilled(fill actor.OrderFillEvent) {
    a.oms[symbol].OnFill(symbol, fill, ASSET_PRECISION)

    // Check new position
    pos := a.oms[symbol].GetNetPosition(symbol)
    if abs(pos) >= a.config.MaxInventory {
        // Stop quoting on this side
    }
}

// Before submitting order, check position
func (a *EnhancedActor) canSubmitOrder(symbol string, side exchange.Side, qty int64) bool {
    currentPos := a.oms[symbol].GetNetPosition(symbol)

    if side == exchange.Buy {
        return currentPos + qty <= a.config.MaxInventory
    } else {
        return currentPos - qty >= -a.config.MaxInventory
    }
}
```

### Actors Requiring OMS

- ✅ Pure Market Makers (inventory limits)
- ✅ Avellaneda-Stoikov (inventory skew formula needs position)
- ✅ Funding Arb (must validate hedge before entry)
- ✅ Multi-Symbol MMs (track per-symbol positions)
- ❌ Randomized Takers (don't care about position)
- ❌ Triangle Arb (executes and exits immediately)

## Fee Configuration

### Realistic Exchange Fees

```go
standardFees := &exchange.PercentageFee{
    MakerBps: 2,   // 0.02% for liquidity providers
    TakerBps: 5,   // 0.05% for liquidity takers
    InQuote:  true,
}
```

### Profitability Thresholds

**Market Makers**:
- Must maintain: `spread > 2 × takerFee` (to cover adverse selection)
- Minimum viable: 5 bps spread
- Our configs: 5-50 bps ✅ All profitable

**Funding Arbitrage**:
- Entry cost: 2 × takerFee (buy spot + short perp)
- Must earn: `fundingRate > 10 bps + profitMargin`
- Our threshold: 20 bps ✅ Profitable

**Triangle Arbitrage**:
- Execution cost: 3 × takerFee (three legs)
- Must earn: `arbitrageProfit > 15 bps + profitMargin`
- Our threshold: 20 bps ✅ Profitable

## File Structure

```
cmd/microstructure_v1/
  ├── main.go              # Main simulation (~800 lines)
  ├── actor_factory.go     # Actor creation (~600 lines)
  ├── market_config.go     # Instruments & prices (~300 lines)
  └── logging_setup.go     # Log directories (~200 lines)

realistic_sim/actors/
  ├── composite.go         # EXISTS: MultiSymbolMM, MultiSymbolLP
  ├── internal_funding_arb.go  # NEW: OMS-integrated funding arb (~350 lines)
  └── triangle_arbitrage.go    # NEW: Triangle arb actor (~400 lines)

logs/microstructure_v1/
  ├── general.log
  ├── spot/
  │   ├── ABC-USD.log
  │   ├── BCD-USD.log
  │   ├── ...
  │   ├── BCD-ABC.log
  │   └── ...
  └── perp/
      ├── ABC-PERP.log
      └── ...
```

## Implementation Checklist

### Phase 1: New Actors (~3 hours)

- [ ] `realistic_sim/actors/internal_funding_arb.go`
  - [ ] Improve existing FundingArbActor with OMS
  - [ ] Separate OMS for spot and perp
  - [ ] Position validation before entries
  - [ ] Fee-aware thresholds
  - [ ] Test with unit tests

- [ ] `realistic_sim/actors/triangle_arbitrage.go`
  - [ ] Subscribe to relevant spot markets
  - [ ] Calculate triangle opportunities
  - [ ] Execute 3-leg trades
  - [ ] Track profitability vs fees
  - [ ] Test with unit tests

### Phase 2: Configuration & Setup (~2 hours)

- [ ] `cmd/microstructure_v1/market_config.go`
  - [ ] `CreateInstruments()` - all 15 markets
  - [ ] `GetBootstrapPrices()` - initial prices
  - [ ] `GetAssetPrecision()` - precision helpers

- [ ] `cmd/microstructure_v1/logging_setup.go`
  - [ ] `SetupLogDirectories()` - create dirs
  - [ ] `CreateLoggers()` - per-symbol loggers

- [ ] `cmd/microstructure_v1/actor_factory.go`
  - [ ] `CreateAvellanedaStoikovMakers()` - 75 AS actors
  - [ ] `CreatePureMarketMakers()` - 75 Pure MM actors
  - [ ] `CreateMultiSymbolMakers()` - 10 composite actors
  - [ ] `CreateFundingArbActors()` - 5 funding arbs
  - [ ] `CreateTriangleArbActor()` - 1 triangle arb
  - [ ] `CreateRandomizedTakers()` - 20 takers

### Phase 3: Main Simulation (~2 hours)

- [ ] `cmd/microstructure_v1/main.go`
  - [ ] Initialize simulated clock
  - [ ] Create exchange with config
  - [ ] Add all 15 instruments
  - [ ] Configure funding rates (different intervals)
  - [ ] Setup logging (16 files total)
  - [ ] Create automation (mark price, funding)
  - [ ] Create all 186 actors
  - [ ] Start all actors
  - [ ] Simulation loop with monitoring
  - [ ] Shutdown and summary

### Phase 4: Testing (~1 hour)

- [ ] Unit tests for new actors
  - [ ] `internal_funding_arb_test.go`
  - [ ] `triangle_arbitrage_test.go`

- [ ] Integration test
  - [ ] Run 1-minute simulation
  - [ ] Verify all actors start
  - [ ] Check log files created
  - [ ] Ensure no panics

### Phase 5: Documentation (~30 minutes)

- [ ] `cmd/microstructure_v1/README.md`
  - [ ] Overview of simulation
  - [ ] How to run
  - [ ] Actor configurations
  - [ ] Expected output
  - [ ] How to analyze logs

## Expected Output

### Console Output (Sample)

```
=== Microstructure Simulation Started ===
Total Actors: 186
Total Markets: 15 (10 spot + 5 perp)
Simulation Duration: 24 hours

[5m] Market Snapshot:
      ABC/USD: Mid=$50000.45  Spread=$  2.50  BidQty=  1000  AskQty=  1200
    ABC-PERP: Mid=$50001.20  Spread=$  3.00  BidQty=   800  AskQty=   900
     BCD/ABC: Mid=$    0.50  Spread=$  0.01  BidQty= 50000  AskQty= 52000

[10m] Funding Arb [Actor 160]: Entered (rate=25 bps, threshold=20 bps)
[12m] Triangle Arb [Actor 166]: USD→ABC→BCD→USD profit=22 bps

[24h] Market Snapshot:
      ABC/USD: Mid=$50123.10  Spread=$  2.55  BidQty=  1100  AskQty=  1150
    ABC-PERP: Mid=$50125.80  Spread=$  3.10  BidQty=   850  AskQty=   920

=== Simulation Complete ===
Wall-clock time: 14m23s
Simulated time: 24h0m0s
Actual speedup: 100.12x
Log directory: logs/microstructure_v1
```

### Success Criteria

1. ✅ Compiles without errors
2. ✅ All 186 actors start successfully
3. ✅ All 15 markets have active liquidity
4. ✅ Funding arbitrage activates when rates exceed threshold
5. ✅ Triangle arbitrage executes when opportunities exist
6. ✅ No order rejections (OMS prevents them)
7. ✅ Market makers maintain positive P&L
8. ✅ 100x+ speedup maintained
9. ✅ All log files created with valid NDJSON
10. ✅ No negative reserved balances

## References

### Existing Code to Leverage

- `actor/composite.go` - MultiSymbolMM and MultiSymbolLP patterns
- `actor/oms.go` - NettingOMS and HedgingOMS
- `realistic_sim/actors/funding_arbitrage.go` - Base funding arb
- `realistic_sim/actors/avellaneda_stoikov.go` - AS implementation
- `realistic_sim/actors/pure_market_maker.go` - Pure MM implementation
- `cmd/randomwalk_v2/main.go` - Simulation structure pattern

### Key Patterns Discovered

**From exploration**:
1. **Clock setup**: SimulatedClock → EventScheduler → SimTickerFactory
2. **Actor initialization**: Create → SetTickerFactory → Start
3. **Logging**: Global logger + per-symbol loggers
4. **Automation**: Starts before actors, handles mark price & funding
5. **Simulation loop**: Wall-clock ticker drives sim clock advancement

### Questions for Clarification

1. **Composite actors**: Should we enhance `actor/composite.go` with a general `CompositeActor` that can wrap ANY actor type, not just MMs?

2. **Funding intervals**: Different per perp (ABC=8h, BCD=4h, CDE=1h, DEF=30m, EFG=15m)?

3. **Bootstrap liquidity**: Should we use FirstLP actors to bootstrap empty books, or assume MMs provide initial liquidity?

4. **Simulation duration**: 24 simulated hours sufficient, or run longer?

5. **Actor IDs**: Any specific ID allocation scheme (e.g., 1-75 AS, 76-150 PureMM, etc.)?

## Estimated Timeline

- **Phase 1**: 3 hours (new actors + tests)
- **Phase 2**: 2 hours (config & helpers)
- **Phase 3**: 2 hours (main simulation)
- **Phase 4**: 1 hour (integration testing)
- **Phase 5**: 30 minutes (documentation)

**Total**: ~8.5 hours

## Next Steps

1. Clarify questions about composite actors and configuration
2. Begin Phase 1: Implement new actors
3. Create comprehensive tests
4. Build main simulation
5. Run and validate

---

**Plan Status**: Ready for implementation pending clarification on composite actor architecture
