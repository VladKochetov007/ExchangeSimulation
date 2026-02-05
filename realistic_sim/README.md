# Realistic Multi-Exchange Simulation

A comprehensive simulation environment for testing multiple trading strategies across multiple exchanges with realistic latency models, funding rates, and market dynamics.

## Overview

This package implements a **library-first** approach - it's a USER of the exchange simulation library, demonstrating how to build complex simulations without modifying core library code.

## Architecture

```
realistic_sim/
├── actors/           # Trading strategy implementations
│   ├── indicators.go              # Math utilities (SMA, StdDev, ZScore)
│   ├── pure_market_maker.go       # Fixed spread market making
│   ├── enhanced_random.go         # Hybrid random trader (market + limit)
│   ├── funding_arbitrage.go       # Funding rate arbitrage (spot + perp)
│   ├── momentum_trader.go         # Trend following (SMA crossover)
│   └── *_test.go                  # Comprehensive test coverage
├── tracking/         # Balance/fee/revenue tracking (TODO)
├── config/          # Configuration helpers (TODO)
└── cmd/            # Runnable simulation (TODO)
```

## Implemented Actors

### 1. Pure Market Maker (`pure_market_maker.go`)

**Strategy**: Quote fixed spreads around mid-price with no alpha signal.

**Key Features**:
- Fixed spread in basis points
- Inventory limits to prevent runaway positions
- Requotes when mid-price changes > threshold
- Tick-aligned prices (PRECISION_GUIDE.md compliant)

**Usage**:
```go
inst := exchange.NewSpotInstrument(
    "BTC/USD", "BTC", "USD",
    exchange.BTC_PRECISION,
    exchange.USD_PRECISION,
    exchange.DOLLAR_TICK,
    exchange.SATOSHI/100,
)

config := PureMarketMakerConfig{
    Symbol:           "BTC/USD",
    Instrument:       inst,
    SpreadBps:        20,              // 0.20% spread
    QuoteSize:        exchange.SATOSHI, // 1 BTC per side
    MaxInventory:     10 * exchange.SATOSHI,
    RequoteThreshold: 5,               // Requote on 5 bps move
}

mm := NewPureMarketMaker(clientID, gateway, config)
mm.Start(ctx)
```

**Precision Notes**:
- All prices aligned to `Instrument.TickSize()` before submission
- Spread calculated as: `spreadHalf = (midPrice * SpreadBps) / (2 * 10000)`
- Ensures exchange acceptance (no `RejectInvalidPrice`)

### 2. Enhanced Random Trader (`enhanced_random.go`)

**Strategy**: Mix of random market orders and random limit orders.

**Key Features**:
- Configurable split between market/limit orders (default 50/50)
- Random side (buy/sell) and random quantity
- Limit orders priced within BPS range of mid
- All prices tick-aligned

**Usage**:
```go
config := EnhancedRandomConfig{
    Symbol:             "BTC/USD",
    Instrument:         inst,
    MinQty:             exchange.SATOSHI / 10,  // 0.1 BTC
    MaxQty:             exchange.SATOSHI,       // 1.0 BTC
    TradeInterval:      2 * time.Second,
    LimitOrderPct:      50,                     // 50% limit orders
    LimitPriceRangeBps: 100,                    // ±1% from mid
}

er := NewEnhancedRandom(clientID, gateway, config, seed)
er.Start(ctx)
```

**Precision Notes**:
- Random offset calculated: `maxOffset = (midPrice * LimitPriceRangeBps) / 10000`
- Price aligned: `price = (price / tickSize) * tickSize`
- Ensures valid limit order prices

### 3. Funding Arbitrage Actor (`funding_arbitrage.go`)

**Strategy**: Long spot + short perpetual hedge to collect positive funding payments.

**Key Features**:
- Enters when funding rate > MinFundingRate
- Exits when funding rate < ExitFundingRate
- Maintains configurable hedge ratio (default 1:1)
- Rebalances when ratio drifts > threshold

**Usage**:
```go
spotInst := exchange.NewSpotInstrument(
    "BTC/USD", "BTC", "USD",
    exchange.BTC_PRECISION,
    exchange.USD_PRECISION,
    exchange.DOLLAR_TICK,
    exchange.SATOSHI/100,
)

perpInst := exchange.NewPerpFutures(
    "BTC-PERP", "BTC", "USD",
    exchange.BTC_PRECISION,
    exchange.USD_PRECISION,
    exchange.DOLLAR_TICK,
    exchange.SATOSHI/100,
)

config := FundingArbConfig{
    SpotSymbol:         "BTC/USD",
    PerpSymbol:         "BTC-PERP",
    SpotInstrument:     spotInst,
    PerpInstrument:     perpInst,
    MinFundingRate:     10,    // Enter when rate > 0.10%
    ExitFundingRate:    -10,   // Exit when rate < -0.10%
    HedgeRatio:         10000, // 1:1 hedge
    MaxPositionSize:    exchange.BTC_PRECISION,
    RebalanceThreshold: 100,   // Rebalance when drift > 1%
}

fa := NewFundingArbitrage(clientID, gateway, config)
fa.Start(ctx)
```

**Precision Notes**:
- Funding rate in basis points (10 = 0.10%)
- Position sizes in base asset precision (BTC_PRECISION = 100M satoshis)
- Rebalance threshold in basis points

### 4. Momentum Trader (`momentum_trader.go`)

**Strategy**: Trend following using fast/slow SMA crossover.

**Key Features**:
- Fast SMA crosses above slow SMA → Long signal
- Fast SMA crosses below slow SMA → Short signal
- Reverse crossover → Exit position
- Uses CircularBuffer for efficient SMA calculation

**Usage**:
```go
config := MomentumTraderConfig{
    Symbol:       "BTC/USD",
    Instrument:   inst,
    FastWindow:   10,  // 10-period fast SMA
    SlowWindow:   50,  // 50-period slow SMA
    PositionSize: exchange.BTC_PRECISION,
}

mt := NewMomentumTrader(clientID, gateway, config)
mt.Start(ctx)
```

**Precision Notes**:
- Tracks trade prices in CircularBuffer (int64)
- SMA calculated as integer average: `sum / count`
- Position sizes in base asset precision

## Shared Utilities

### Indicators (`indicators.go`)

Mathematical utilities for strategy implementation:

**CircularBuffer**:
- Fixed-size rolling window for efficient calculations
- O(1) add operation
- O(n) SMA and StdDev calculations

**Functions**:
- `SMA()` - Simple Moving Average
- `StdDev(mean)` - Standard Deviation
- `ZScore(value, mean, stdDev)` - Z-score scaled by 10000
- `CalculateRatio(price1, price2)` - Price ratio scaled by 10000
- `BPSChange(oldPrice, newPrice)` - Basis points change

**Example**:
```go
buf := NewCircularBuffer(50)
for _, price := range prices {
    buf.Add(price)
}
sma := buf.SMA()
stdDev := buf.StdDev(sma)
zscore := ZScore(currentPrice, sma, stdDev)
```

## Testing

All actors have comprehensive test coverage (43 tests total):

```bash
# Run all tests
go test -v ./realistic_sim/actors

# Run specific actor tests
go test -v ./realistic_sim/actors -run "PureMarket"
go test -v ./realistic_sim/actors -run "EnhancedRandom"
go test -v ./realistic_sim/actors -run "FundingArb"
go test -v ./realistic_sim/actors -run "MomentumTrader"
go test -v ./realistic_sim/actors -run "Indicator"
```

## Design Principles

### Library-First Architecture

✅ **Core library remains untouched**:
- No modifications to `exchange/`, `actor/`, or `simulation/` packages
- All new code in `realistic_sim/` package
- Demonstrates extensibility without library changes

✅ **Dependency Injection**:
- Instruments passed in config (not hardcoded)
- Precision values from instrument interfaces
- Fee structures configurable per venue

✅ **Configuration over Hardcoding**:
- All strategy parameters in `*Config` structs
- Sensible defaults in constructors
- Users can override any parameter

### Precision System Compliance

All actors follow **PRECISION_GUIDE.md**:

✅ **Tick Size Alignment**:
```go
tickSize := instrument.TickSize()
price = (price / tickSize) * tickSize
```

✅ **Precision Constants**:
- `BTC_PRECISION = 100_000_000` (100M satoshis per BTC)
- `USD_PRECISION = 100_000` (0.00001 USD minimum)
- `DOLLAR_TICK = 100_000_000` ($1.00 minimum tick)

✅ **Integer Arithmetic**:
- All prices, quantities, and calculations use int64
- Basis points scaled by 10000 (100 bps = 1%)
- No floating-point in core logic

## Status

### Completed ✅
- ✅ Indicators utility (CircularBuffer, SMA, StdDev, ZScore)
- ✅ Pure Market Maker
- ✅ Enhanced Random Trader
- ✅ Funding Arbitrage
- ✅ Momentum Trader
- ✅ Comprehensive test coverage (43 tests, all passing)

### Remaining (Per Plan)
- ⏳ Cross-Sectional Mean Reversion (pairs trading)
- ⏳ Avellaneda-Stoikov Market Maker (optimal MM)
- ⏳ Balance Recorder (track all client balances)
- ⏳ Fee Recorder (track all fee payments)
- ⏳ Revenue Calculator (exchange revenue)
- ⏳ Wealth Tracker (system wealth conservation)
- ⏳ Simulation Runner (multi-exchange orchestration)
- ⏳ Configuration helpers

## Next Steps

According to the plan in `/home/vlad/.claude/plans/warm-wishing-wombat.md`:

1. **Phase 3: Advanced Strategies**
   - Implement Cross-Sectional Mean Reversion
   - Implement Avellaneda-Stoikov Market Maker

2. **Phase 4: Tracking Infrastructure**
   - Balance Recorder
   - Fee Recorder
   - Revenue Calculator
   - Wealth Tracker

3. **Phase 5: Simulation Runner**
   - Multi-exchange setup (Coinbase, Binance, Bybit)
   - Actor distribution (30+ actors)
   - Real-time statistics display
   - CSV output for analysis

## References

- **Main Plan**: `/home/vlad/.claude/plans/warm-wishing-wombat.md`
- **Precision Guide**: `/home/vlad/development/exchange_simulation/PRECISION_GUIDE.md`
- **Architecture**: `/home/vlad/development/exchange_simulation/ARCHITECTURE.md`
- **Core Library**: `/home/vlad/development/exchange_simulation/actor/`
