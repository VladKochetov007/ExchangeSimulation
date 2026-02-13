# Multi-Venue Logging Structure

## Overview

The exchange simulation now uses a hierarchical, venue-aware logging structure that supports both single-exchange and multi-exchange simulations. This is the **default logging model** for all simulations.

## Directory Structure

```
logs/
├── <exchange_id>/
│   ├── general.log              # Exchange-wide events
│   ├── spot/
│   │   ├── BTCUSD.log          # Spot market symbol events
│   │   └── ETHUSD.log
│   └── perp/
│       ├── BTC-PERP.log        # Perpetual futures symbol events
│       └── ETH-PERP.log
```

### Example: Multi-Venue Simulation
```
logs/
├── binance/
│   ├── general.log
│   ├── spot/
│   │   └── BTCUSD.log
│   └── perp/
│       └── BTC-PERP.log
└── coinbase/
    ├── general.log
    ├── spot/
    │   └── BTCUSD.log
    └── perp/
        └── BTC-PERP.log
```

## Event Routing

### general.log (Exchange-wide events)
Logs to `_global` logger, stored in `<exchange>/general.log`:
- **Balance snapshots** (`balance_snapshot`) - periodic snapshots of all client balances
- **Balance changes** without symbol (`balance_change`):
  - Borrow operations
  - Repay operations
  - Initial deposits
  - Transfers
  - General balance adjustments

### Symbol Logs (Market-specific events)
Logs to symbol logger, stored in `<exchange>/<spot|perp>/<symbol>.log`:
- **Balance changes** with symbol (`balance_change`):
  - Funding settlements (perp only)
  - Trade settlements
  - Liquidation events
- **Funding events** (perp only):
  - Mark price updates (`mark_price_update`)
  - Funding rate updates (`funding_rate_update`)
- **Trading events**:
  - Order acceptance/rejection
  - Order fills
  - Order cancellations
  - Trades
  - Book snapshots and deltas

## Implementation Changes

### 1. Exchange ID
Every exchange now has an `ID` field:
```go
type Exchange struct {
    ID string  // Identifies the exchange (e.g., "binance", "randomwalk_v2")
    // ... other fields
}

type ExchangeConfig struct {
    ID string  // Default: "exchange"
    // ... other fields
}
```

### 2. Event Routing Logic
Balance changes now route based on symbol:
```go
// balance_logger.go
func (t *BalanceChangeTracker) LogBalanceChange(..., symbol string, ...) {
    logKey := "_global"
    if symbol != "" {
        logKey = symbol  // Route to symbol logger
    }
    log := t.exchange.getLogger(logKey)
    // ... log event
}
```

Funding events route to symbol logger:
```go
// automation.go
if log := a.exchange.getLogger(u.symbol); log != nil {  // Changed from "_global"
    log.LogEvent(timestamp, 0, "mark_price_update", ...)
    log.LogEvent(timestamp, 0, "funding_rate_update", ...)
}
```

### 3. Multi-Venue Runner
`MultiExchangeRunner` creates proper directory structure:
```go
// Create general.log for each exchange
generalLogPath := filepath.Join(exchangeDir, "general.log")
generalLogFile, err := os.Create(generalLogPath)
generalLogger := logger.New(generalLogFile)
ex.SetLogger("_global", generalLogger)

// Create symbol-specific logs in spot/ or perp/ subdirectories
logPath := filepath.Join(logDir, symbol+".log")
ex.SetLogger(symbol, symbolLogger)
```

## Multi-Venue Actor Balance Queries

### Querying Balances
Multi-venue actors (like `LatencyArbitrageActor`) can:

1. **Query specific venue:**
   ```go
   mgw.QueryBalance(venue, &exchange.QueryRequest{...})
   ```

2. **Read balance snapshots:**
   - Each `general.log` contains periodic `balance_snapshot` events
   - Search by `client_id` across all venues

3. **Aggregate balances:**
   ```bash
   # Find all balances for client_id=1
   grep '"client_id":1' logs/*/general.log | grep balance_snapshot
   ```

### Example: Balance Recovery
To recover complete state for a multi-venue actor:
```bash
# Get balances from all venues for client_id=42
for venue in logs/*/general.log; do
    jq -r 'select(.client_id == 42 and .event == "balance_snapshot")' "$venue" | tail -1
done
```

Result shows balances on each exchange independently:
```json
// logs/binance/general.log
{"client_id": 42, "event": "balance_snapshot", "spot_balances": [...]}

// logs/coinbase/general.log
{"client_id": 42, "event": "balance_snapshot", "spot_balances": [...]}
```

## Benefits

### 1. Venue Isolation
- No venue ID in JSON (kept clean)
- Directory structure provides venue context
- Each exchange logs independently (no contention)

### 2. Easy Balance Reconstruction
- Multi-venue actor balance = sum across `logs/*/general.log` for that `client_id`
- All data available, well-structured
- Simple to recover state after simulation

### 3. Market-Type Separation
- Spot events in `spot/` subdirectory
- Perpetual futures events in `perp/` subdirectory
- Easy to analyze specific market types

### 4. Event Locality
- Symbol-specific events (trades, funding) in symbol log
- Exchange-wide events (borrows, snapshots) in general log
- Reduces log file size per symbol

## Backwards Compatibility

### Tests
All tests work with the new structure. Tests that log to `_global` continue to work as before. Tests that need symbol-specific events (like funding rates) now also set a symbol logger:
```go
ex.SetLogger("_global", logger)
ex.SetLogger("BTC-PERP", logger)  // For funding/mark price events
```

### Single-Exchange Simulations
Continue to work with default ID `"exchange"`:
```go
ex := exchange.NewExchange(100, clock)  // ID defaults to "exchange"
```

Produces:
```
logs/
└── exchange/
    ├── general.log
    └── perp/
        └── BTC-PERP.log
```

## Migration Guide

### For Existing Simulations

1. **No changes needed** if using `NewExchange()`
   - Default ID is `"exchange"`
   - Logs go to `logs/exchange/` directory

2. **For named exchanges**, use `NewExchangeWithConfig`:
   ```go
   ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
       ID:    "binance",
       Clock: clock,
   })
   ```

3. **Set up logging**:
   ```go
   // Create general.log
   generalLogger := logger.New(generalLogFile)
   ex.SetLogger("_global", generalLogger)

   // Create symbol logs
   symbolLogger := logger.New(symbolLogFile)
   ex.SetLogger("BTCUSD", symbolLogger)
   ```

### For Multi-Venue Simulations

Use `MultiExchangeRunner` which handles everything automatically:
```go
config := simulation.MultiSimConfig{
    LogDir: "logs",
    Exchanges: []simulation.ExchangeConfig{
        {Name: "binance", ...},
        {Name: "coinbase", ...},
    },
}
runner, err := simulation.NewMultiExchangeRunner(config)
```

## File Formats

All logs use JSONL (JSON Lines) format - one JSON object per line:
```json
{"sim_time":1234567890,"server_time":1707832800,"event":"balance_change","client_id":1,...}
{"sim_time":1234567891,"server_time":1707832801,"event":"trade","symbol":"BTCUSD",...}
```

This allows easy processing with `jq`, `grep`, and other tools.
