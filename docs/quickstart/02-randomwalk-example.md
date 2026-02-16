# Random Walk Example Walkthrough

Complete walkthrough of `cmd/randomwalk_v2/main.go` - a perpetual futures simulation with market makers and random takers.

## Overview

This simulation demonstrates:
- Simulated time with 100x speedup
- 3 market makers with different strategies
- 10 randomized takers providing flow
- Zero funding rates (pure price discovery)
- NDJSON logging for analysis

Run time: ~83 simulated hours compressed to ~50 minutes wall-clock.

## Configuration Constants

```go
const (
    simDuration    = 300000 * time.Second  // 83 hours simulated
    speedup        = 100.0                  // 100x time compression
    simTimeStep    = 10 * time.Millisecond  // Clock advances every 10ms sim-time
    bootstrapPrice = 50000                  // BTC starting at $50,000
)
```

**Time compression math:**
- Simulation advances 10ms every `10ms / 100 = 0.1ms` wall-clock
- 1 simulated second = 10ms wall-clock
- 1 simulated hour = 36 seconds wall-clock

## Exchange Setup

### Simulated Clock

```go
startTime := time.Now().UnixNano()
simClock := simulation.NewSimulatedClock(startTime)
scheduler := simulation.NewEventScheduler(simClock)
simClock.SetScheduler(scheduler)
tickerFactory := simulation.NewSimTickerFactory(scheduler)
```

**Why simulated time:**
- Deterministic execution
- Reproducible results
- Faster than real-time testing
- Event ordering guaranteed

### Exchange Creation

```go
ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
    ID:               "randomwalk_v2",
    EstimatedClients: 100,
    Clock:            simClock,
    TickerFactory:    tickerFactory,
    SnapshotInterval: 100 * time.Millisecond,
})
```

**Config details:**
- `ID`: Used in log file organization
- `EstimatedClients`: Size hint for internal maps
- `Clock`: Simulated time provider
- `TickerFactory`: Creates sim-time tickers for periodic tasks
- `SnapshotInterval`: Order book snapshot frequency

## Instrument: BTC-PERP

```go
perpInst := exchange.NewPerpFutures(
    "BTC-PERP",
    "BTC", "USD",
    exchange.BTC_PRECISION,    // 100,000,000 (satoshis)
    exchange.USD_PRECISION,    // 100,000,000 (cents)
    exchange.CENT_TICK,        // 1 cent minimum price increment
    exchange.BTC_PRECISION/10000,  // 0.0001 BTC minimum order size
)
perpInst.SetFundingCalculator(&ZeroFundingCalc{})
ex.AddInstrument(perpInst)
```

### Precision Math

BTC precision = 100,000,000 represents 1 BTC in integer units.

**Example:** 1.5 BTC order
```
qty = 1.5 × 100,000,000 = 150,000,000
```

**Example:** Price of $50,123.45
```
price = 50123.45 × 100,000,000 = 5,012,345,000,000
```

### Zero Funding

```go
type ZeroFundingCalc struct{}

func (c *ZeroFundingCalc) Calculate(indexPrice, markPrice int64) int64 {
    return 0
}
```

Funding disabled to isolate price discovery from arbitrage anchoring.

## Logging Setup

```go
logDir := "logs/randomwalk_v2"
perpDir := filepath.Join(logDir, "perp")
os.MkdirAll(perpDir, 0755)
```

### General Logger

```go
generalLogFile, _ := os.Create(filepath.Join(logDir, "general.log"))
generalLogger := logger.New(generalLogFile)
ex.SetLogger("_global", generalLogger)
```

Logs exchange-wide events:
- Balance changes
- Transfers
- Margin operations
- System events

### Symbol Logger

```go
perpLogFile, _ := os.Create(filepath.Join(perpDir, "BTC-PERP.log"))
perpLogger := logger.New(perpLogFile)
ex.SetLogger("BTC-PERP", perpLogger)
```

Logs BTC-PERP specific events:
- Order acceptance/rejection
- Fills and partial fills
- Order book snapshots
- Trades
- Position updates

## Exchange Automation

```go
indexProvider := exchange.NewFixedIndexProvider()
indexProvider.SetPrice("BTC-PERP",
    exchange.PriceUSD(bootstrapPrice, exchange.CENT_TICK))

automation := exchange.NewExchangeAutomation(ex, exchange.AutomationConfig{
    MarkPriceCalc:       exchange.NewMidPriceCalculator(),
    IndexProvider:       indexProvider,
    PriceUpdateInterval: 3 * time.Second,
    CollateralRate:      0,  // Zero funding
    TickerFactory:       tickerFactory,
})
```

**Automation tasks:**
- Updates mark price every 3 seconds using mid-price
- Calculates funding rates (but rate is zero)
- Performs liquidation checks
- Charges collateral interest

## Market Makers

Three makers with different characteristics:

```go
spreads := []int64{5, 10, 20}  // bps
requoteIntervals := []time.Duration{
    800 * time.Millisecond,   // MM1: fast, tight spread
    1000 * time.Millisecond,  // MM2: medium
    1200 * time.Millisecond,  // MM3: slow, wide spread
}
emaDecays := []float64{
    0.3,  // MM1: fast adaptation
    0.2,  // MM2: medium adaptation
    0.1,  // MM3: slow adaptation
}
```

### SlowMarketMaker Configuration

```go
for i, spreadBps := range spreads {
    mmID := uint64(2 + i)
    mmGateway := ex.ConnectClient(mmID, map[string]int64{}, &exchange.FixedFee{})
    ex.AddPerpBalance(mmID, "USD", 100_000_000*exchange.USD_PRECISION)

    mm := actors.NewSlowMarketMaker(mmID, mmGateway, actors.SlowMarketMakerConfig{
        Symbol:          "BTC-PERP",
        Instrument:      perpInst,
        SpreadBps:       spreadBps,
        QuoteSize:       100 * exchange.BTC_PRECISION / 100,  // 1.0 BTC
        MaxInventory:    1000 * exchange.BTC_PRECISION,       // 1,000 BTC
        RequoteInterval: requoteIntervals[i],
        BootstrapPrice:  exchange.PriceUSD(bootstrapPrice, exchange.CENT_TICK),
        EMADecay:        emaDecays[i],
        Levels:          4,
        LevelSpacingBps: spreadBps / 2,
    })
    marketMakers = append(marketMakers, mm)
}
```

**Per maker:**
- $100M USD capital
- 1 BTC per level
- 4 levels deep (e.g., for 5bps: bids at -5, -7.5, -10, -12.5 bps)
- Max inventory: 1,000 BTC
- EMA tracks recent trade prices for fair value

**Why different parameters:**
- Staggered requote times prevent synchronization
- Different EMA decays create diverse views of fair value
- Varying spreads provide depth at multiple price points
- Independent price discovery without coordination

## Takers

Ten randomized takers with varying activity:

```go
takerConfigs := []struct {
    interval      time.Duration
    minQty, maxQty int64
}{
    {300 * time.Millisecond, 50, 150},   // Very fast, medium size
    {400 * time.Millisecond, 60, 180},   // Fast, large
    {500 * time.Millisecond, 70, 200},   // Medium, large
    {600 * time.Millisecond, 80, 220},   // Medium, very large
    {700 * time.Millisecond, 60, 180},   // Slow, large
    {400 * time.Millisecond, 55, 170},   // Fast, medium
    {500 * time.Millisecond, 65, 190},   // Medium, large
    {600 * time.Millisecond, 75, 210},   // Medium, very large
    {350 * time.Millisecond, 50, 150},   // Very fast, medium
    {550 * time.Millisecond, 70, 200},   // Medium, large
}
```

### RandomizedTaker Setup

```go
for i, config := range takerConfigs {
    takerID := uint64(10 + i)
    takerGateway := ex.ConnectClient(takerID, map[string]int64{}, &exchange.FixedFee{})
    ex.AddPerpBalance(takerID, "USD", 10_000_000*exchange.USD_PRECISION)

    taker := actors.NewRandomizedTaker(takerID, takerGateway,
        actors.RandomizedTakerConfig{
            Symbol:         "BTC-PERP",
            Interval:       config.interval,
            MinQty:         config.minQty * exchange.BTC_PRECISION / 100,
            MaxQty:         config.maxQty * exchange.BTC_PRECISION / 100,
            BasePrecision:  exchange.BTC_PRECISION,
            QuotePrecision: exchange.USD_PRECISION,
        })
    takers = append(takers, taker)
}
```

**Per taker:**
- $10M USD capital
- Trades every 300-700ms
- Random qty: 0.5-2.2 BTC
- Random side: 50/50 buy/sell

**Aggregate flow:**
- ~20-30 trades per second
- Balanced buy/sell over time
- Continuous liquidity consumption

## Actor Lifecycle

### Ticker Factory Injection

```go
for _, a := range allActors {
    switch act := a.(type) {
    case *actors.SlowMarketMakerActor:
        act.SetTickerFactory(tickerFactory)
    case *actors.RandomizedTakerActor:
        act.SetTickerFactory(tickerFactory)
    }
}
```

**Why:** Actors use tickers for periodic tasks (requoting, trading). Simulation mode requires sim-time tickers.

### Starting Actors

```go
automation.Start(ctx)

for _, actor := range allActors {
    actor.Start(ctx)
}
```

**Start order:**
1. Automation (mark price updates, liquidation checks)
2. Market makers (start quoting)
3. Takers (start trading)

## Clock Advancement Loop

```go
tickInterval := time.Duration(float64(simTimeStep) / speedup)
ticker := time.NewTicker(tickInterval)  // Real-time ticker

wallStart := time.Now()
lastLogTime := startTime

for {
    select {
    case <-ctx.Done():
        goto shutdown
    case <-ticker.C:
        simClock.Advance(simTimeStep)

        if simClock.NowUnixNano()-lastLogTime >= 30*int64(time.Second) {
            lastLogTime = simClock.NowUnixNano()
            elapsed := time.Duration(simClock.NowUnixNano() - startTime)

            book := ex.Books["BTC-PERP"]
            if book != nil && book.Bids.Best != nil && book.Asks.Best != nil {
                bestBid := book.Bids.Best.Price
                bestAsk := book.Asks.Best.Price
                midPrice := bestBid + (bestAsk-bestBid)/2

                fmt.Printf("[%v] Mid: $%.2f | Bid: $%.2f | Ask: $%.2f\n",
                    elapsed.Round(time.Second),
                    float64(midPrice)/float64(exchange.USD_PRECISION),
                    float64(bestBid)/float64(exchange.USD_PRECISION),
                    float64(bestAsk)/float64(exchange.USD_PRECISION))
            }
        }
    }
}
```

### How Clock Advancement Works

```
Wall-clock ticker fires every 0.1ms
  ↓
simClock.Advance(10ms)
  ↓
scheduler.ProcessUntil(newTime)
  ↓
Fires all events scheduled ≤ newTime:
  - Market maker requote timers
  - Taker trade timers
  - Automation tasks
  ↓
Actors receive ticker events, submit orders
  ↓
Exchange processes requests, executes matches
  ↓
Logs written to files
```

### Progress Logging

Every 30 simulated seconds, print:
- Elapsed sim-time
- Current mid-price
- Best bid/ask
- Spread

Example output:
```
[30s] Mid: $50012.34 | Bid: $50010.00 | Ask: $50014.67
[1m0s] Mid: $49987.21 | Bid: $49985.50 | Ask: $49988.92
[1m30s] Mid: $50025.78 | Bid: $50023.10 | Ask: $50028.45
```

## Graceful Shutdown

```go
shutdown:
for _, actor := range allActors {
    actor.Stop()
}
ex.Shutdown()

wallElapsed := time.Since(wallStart)
simElapsed := time.Duration(simClock.NowUnixNano() - startTime)
actualSpeedup := float64(simElapsed) / float64(wallElapsed)

fmt.Printf("\n=== Simulation Complete ===\n")
fmt.Printf("Wall-clock time: %v\n", wallElapsed)
fmt.Printf("Simulated time: %v\n", simElapsed.Round(time.Second))
fmt.Printf("Actual speedup: %.2fx\n", actualSpeedup)
```

**Shutdown sequence:**
1. Stop all actors (cease order submission)
2. Exchange shutdown (flush logs, close books)
3. Print performance stats

Example output:
```
=== Simulation Complete ===
Wall-clock time: 50m12s
Simulated time: 83h20m
Actual speedup: 99.87x
Log directory: logs/randomwalk_v2
```

## Log Output Structure

```
logs/randomwalk_v2/
├── general.log          # Balance changes, transfers, system events
└── perp/
    └── BTC-PERP.log     # Orders, fills, trades, positions
```

### Sample Log Lines

**general.log:**
```json
{"sim_time":1000000000,"server_time":1707925123456789000,"event":"balance_change","client_id":2,"reason":"trade_settlement","changes":[{"asset":"USD","wallet":"perp","old_balance":10000000000000000,"new_balance":10000005000000000,"delta":5000000000}]}
```

**BTC-PERP.log:**
```json
{"sim_time":500000000,"server_time":1707925123456234000,"event":"OrderAccepted","client_id":2,"order_id":1,"symbol":"BTC-PERP","side":"Buy","price":5000000000000,"qty":100000000}
{"sim_time":600000000,"server_time":1707925123456345000,"event":"OrderFill","client_id":2,"order_id":1,"symbol":"BTC-PERP","side":"Buy","price":5000000000000,"qty":50000000,"trade_id":1}
```

## Analysis

Use Python scripts to analyze results:

```bash
# Basic statistics
python scripts/analyze_simple.py logs/randomwalk_v2/perp/BTC-PERP.log

# Plot price evolution
python scripts/plot_price.py logs/randomwalk_v2/perp/BTC-PERP.log

# Reconstruct order book
python scripts/plot_book.py logs/randomwalk_v2/perp/BTC-PERP.log
```

See [Analysis Tools](../observability/analysis-tools.md) for details.

## Key Takeaways

**Market Maker Behavior:**
- Makers update quotes based on inventory and recent trade prices
- EMA provides dynamic fair value estimate
- Multiple levels provide depth

**Price Discovery:**
- Random taker flow creates natural price walk
- Makers adjust around realized trades
- No external anchor (funding disabled)

**Simulation Benefits:**
- 100x speedup: 83 hours in 50 minutes
- Deterministic: same seed = same results
- Observable: complete audit trail in logs
- Testable: validate strategy logic

**Next Steps:**
- Modify maker spreads and observe impact
- Add momentum takers to create trends
- Enable funding and observe arbitrage
- Implement custom actor strategies
