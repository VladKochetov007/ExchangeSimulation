# First Simulation

Build and run your first exchange simulation in 5 minutes.

## Prerequisites

```bash
go version  # 1.21 or higher
make --version
python3 --version  # For analysis
```

## Clone and Build

```bash
git clone <repository-url> exchange_simulation
cd exchange_simulation
make build
```

**Output:**
```
Building all binaries...
✓ bin/randomwalk_v2
✓ bin/multisim
✓ All binaries built successfully
```

## Run Simulation

```bash
./bin/randomwalk_v2
```

**Output (live):**
```
[30s] Mid: $50012.34 | Bid: $50010.00 | Ask: $50014.67 | Spread: $4.67
[1m0s] Mid: $49987.21 | Bid: $49985.50 | Ask: $49988.92 | Spread: $3.42
[1m30s] Mid: $50025.78 | Bid: $50023.10 | Ask: $50028.45 | Spread: $5.35
...
```

**Completion (~50 minutes wall-clock):**
```
=== Simulation Complete ===
Wall-clock time: 50m12s
Simulated time: 83h20m
Actual speedup: 99.87x
Log directory: logs/randomwalk_v2
```

## Understand What Happened

### Actors

**3 Market Makers:**
- MM1: Tight spread (5 bps), fast requoting (800ms)
- MM2: Medium spread (10 bps), medium requoting (1s)
- MM3: Wide spread (20 bps), slow requoting (1.2s)

Each quotes 4 levels deep, 1 BTC per level.

**10 Takers:**
- Random buy/sell every 300-700ms
- Qty: 0.5-2.2 BTC per trade
- Total: ~20-30 trades/second

### Market Dynamics

**Price discovery:**
- Starts at \$50,000 (bootstrap price)
- Random walk from balanced buy/sell flow
- Makers adjust quotes based on inventory and trade prices

**No funding:**
- Funding rate = 0 (disabled)
- Pure price discovery without arbitrage anchor

**Time compression:**
- 100x speedup
- 83 simulated hours in 50 real minutes

## Examine Logs

### Log Structure

```
logs/randomwalk_v2/
├── general.log          # Balance changes, transfers, system events
└── perp/
    └── BTC-PERP.log     # Orders, fills, trades, positions
```

### View Recent Events

```bash
# Last 10 events
tail -10 logs/randomwalk_v2/perp/BTC-PERP.log

# Count event types
jq -r '.event' logs/randomwalk_v2/perp/BTC-PERP.log | sort | uniq -c

# Filter to fills only
jq -c 'select(.event == "OrderFill")' logs/randomwalk_v2/perp/BTC-PERP.log | head -5
```

### Sample Events

**OrderAccepted:**
```json
{
  "sim_time": 500000000,
  "event": "OrderAccepted",
  "client_id": 2,
  "symbol": "BTC-PERP",
  "order_id": 1,
  "side": "Buy",
  "price": 5000000000000,
  "qty": 100000000
}
```

**OrderFill:**
```json
{
  "sim_time": 600000000,
  "event": "OrderFill",
  "client_id": 10,
  "symbol": "BTC-PERP",
  "order_id": 42,
  "side": "Buy",
  "price": 5000000000000,
  "qty": 75000000,
  "trade_id": 1
}
```

## Analyze Results

### Basic Statistics

```bash
python3 scripts/analyze_simple.py logs/randomwalk_v2/perp/BTC-PERP.log
```

**Output:**
```
=== Event Statistics ===
OrderAccepted: 12,543
OrderFill: 8,321
OrderCancelled: 3,105
Trade: 4,160

=== Trade Statistics ===
Total trades: 4,160
Total volume: 8,456.32 BTC
Average trade size: 2.03 BTC
Price range: $48,234.56 - $51,987.23
```

### Plot Price Evolution

```bash
python3 scripts/plot_price.py logs/randomwalk_v2/perp/BTC-PERP.log
```

Generates `price_chart.png` showing:
- Trade prices over time
- Price trend
- Volatility

### Reconstruct Order Book

```bash
python3 scripts/plot_book.py logs/randomwalk_v2/perp/BTC-PERP.log
```

Shows order book depth at different timestamps.

## Modify the Simulation

### Change Market Maker Spreads

Edit `cmd/randomwalk_v2/main.go`:

```go
spreads := []int64{2, 5, 10}  // Tighter spreads (was 5, 10, 20)
```

Rebuild and run:
```bash
make build
./bin/randomwalk_v2
```

**Effect:** Tighter spreads → less profit for makers → more aggressive inventory management.

### Add More Takers

```go
// Duplicate taker configs
takerConfigs := []struct {
    interval      time.Duration
    minQty, maxQty int64
}{
    {300 * time.Millisecond, 50, 150},
    {300 * time.Millisecond, 50, 150},  // Added
    {400 * time.Millisecond, 60, 180},
    {400 * time.Millisecond, 60, 180},  // Added
    // ... existing configs
}
```

**Effect:** More flow → faster price discovery → potentially more volatility.

### Shorter Simulation

```go
const simDuration = 30000 * time.Second  // 8.3 hours (was 83 hours)
```

**Effect:** Runs in ~5 minutes instead of 50.

### Enable Funding

```go
perpInst.SetFundingCalculator(&exchange.SimpleFundingCalc{
    BaseRate: 10,    // 0.1% base
    Damping:  100,   // Full premium
    MaxRate:  750,   // ±7.5% cap
})

automation := exchange.NewExchangeAutomation(ex, exchange.AutomationConfig{
    CollateralRate: 500,  // Enable funding (was 0)
    // ... other config
})
```

**Effect:** Funding anchors perp price to index, reduces drift.

## Run Tests

```bash
make test
```

**Output:**
```
Running tests...
ok      exchange_sim/exchange      2.431s
ok      exchange_sim/actor         0.823s
ok      exchange_sim/simulation    1.234s
```

### View Coverage

```bash
make coverage-html
```

Opens browser showing test coverage by file.

## Clean Up

```bash
# Remove binaries
make clean

# Remove logs
rm -rf logs/

# Remove both
make clean && rm -rf logs/
```

## Next Steps

- [Random Walk Example](02-randomwalk-example.md) - Detailed walkthrough
- [Creating Actors](03-creating-actors.md) - Build custom strategies
- [Exchange Architecture](../core-concepts/exchange-architecture.md) - Understand the engine
- [Simulated Time](../simulation/simulated-time.md) - How time compression works

## Troubleshooting

### Build Fails

```bash
# Update Go
go version  # Check >= 1.21

# Clean and rebuild
make clean
go mod tidy
make build
```

### Simulation Runs Slowly

**Possible causes:**
- High CPU usage from other processes
- Insufficient RAM
- Debug logging enabled

**Solutions:**
```go
// Reduce log verbosity
ex.SetLogger("_global", nil)  // Disable global logger

// Increase time step
const simTimeStep = 100 * time.Millisecond  // Was 10ms
```

### Logs Too Large

```bash
# Compress completed logs
gzip logs/randomwalk_v2/perp/BTC-PERP.log

# Or reduce simulation duration
const simDuration = 30000 * time.Second  // Shorter run
```

### Python Analysis Fails

```bash
# Install dependencies
pip3 install pandas matplotlib

# Or use conda
conda install pandas matplotlib
```
