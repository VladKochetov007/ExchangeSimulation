# Market Data Recording System

## Overview

The recorder now implements an exchange-grade market data recording system that matches real cryptocurrency exchange feeds (Bybit, Binance, etc.).

## Key Changes

### 1. Per-Instrument File Separation

Files are now separated by instrument and data type:

```
output/
├── BTCUSD_PERP_orderbook.csv
├── BTCUSD_PERP_trades.csv
├── BTCUSD_PERP_openinterest.csv
├── BTCUSD_PERP_funding.csv
├── ETHUSD_SPOT_orderbook.csv
└── ETHUSD_SPOT_trades.csv
```

**Naming convention**: `{SYMBOL}_{TYPE}_{DATATYPE}.csv`
- `TYPE`: SPOT, PERP, FUTURES
- `DATATYPE`: orderbook, trades, openinterest, funding

### 2. Exchange-Style Snapshot/Delta Format

**Orderbook CSV Format:**
```csv
timestamp,seq,type,side,price,visible_qty,hidden_qty
1770098127337417440,15,snapshot,ask,100100000000,10000000000,0
1770098127337505680,17,delta,bid,99900000000,77740536,0
```

- **snapshot**: Full book state at a point in time (all levels)
- **delta**: Incremental update (single price level change)
- **qty=0**: Means delete this price level

### 3. Trades Format

```csv
timestamp,seq,trade_id,side,price,qty
1770098129337275942,0,0,Buy,100100000000,18848536
```

- `side`: Taker side (Buy/Sell)
- Includes trade ID for deduplication

### 4. Open Interest Format (Perpetuals/Futures Only)

```csv
timestamp,seq,total_contracts
1770098129337275942,0,37697072
```

- Tracks total outstanding contracts
- Updated on every trade

### 5. Funding Rate Format (Perpetuals Only)

```csv
timestamp,seq,rate,mark_price,index_price,next_funding_time,interval_seconds
1760325052630,0,-500,100000000000000,100050000000000,1760356800000,28800
```

- `rate`: Funding rate in basis points
- `interval_seconds`: Funding period (typically 28800 = 8 hours)

## Configuration

```go
type RecorderConfig struct {
    // File organization
    OutputDir            string            // Base directory (default: "output")
    Symbols              []string          // Symbols to record
    RotationStrategy     RotationStrategy  // None, Daily, Hourly

    // Performance
    FlushInterval        time.Duration     // Buffer flush frequency (default: 1s)

    // Snapshot control
    SnapshotInterval     time.Duration     // Time-based snapshots (default: 30s)
    SnapshotDeltaCount   uint64           // Delta-based snapshots (default: 100)

    // Feature flags
    RecordTrades         bool              // Record trades (default: true)
    RecordOrderbook      bool              // Record orderbook (default: true)
    RecordOpenInterest   bool              // Record OI (default: true)
    RecordFunding        bool              // Record funding (default: true)
    SeparateHiddenFiles  bool              // Split visible/hidden (default: false)
}
```

## Example Usage

```go
// Create instruments
btcusd := exchange.NewPerpFutures("BTCUSD", "BTC", "USD", 100000000, 1000000)
ethusd := exchange.NewSpotInstrument("ETHUSD", "ETH", "USD", 10000000, 10000000)
ex.AddInstrument(btcusd)
ex.AddInstrument(ethusd)

// Create recorder
recorderGateway := ex.ConnectClient(999, initialBalances, feePlan)
recorder, err := actor.NewRecorder(999, recorderGateway, actor.RecorderConfig{
    OutputDir:           "output",
    Symbols:             []string{"BTCUSD", "ETHUSD"},
    FlushInterval:       time.Second,
    SnapshotInterval:    5 * time.Second,
    SnapshotDeltaCount:  50,
    RotationStrategy:    actor.RotationNone,
    RecordTrades:        true,
    RecordOrderbook:     true,
    RecordOpenInterest:  true,
    RecordFunding:       true,
    SeparateHiddenFiles: false,
}, ex.Instruments)

runner.AddActor(recorder)
```

## Snapshot Strategy

The recorder uses a hybrid snapshot strategy:

1. **Initial snapshot**: Sent immediately on subscription
2. **Periodic snapshots**: Based on two triggers (whichever comes first):
   - **Time-based**: Every `SnapshotInterval` seconds
   - **Delta-based**: Every `SnapshotDeltaCount` deltas
3. **Recovery**: Snapshots allow easy replay and validation

## File Rotation

Optional time-based file rotation:

```go
RotationStrategy: actor.RotationDaily
// → BTCUSD_PERP_orderbook_20260203.csv

RotationStrategy: actor.RotationHourly
// → BTCUSD_PERP_orderbook_2026020315.csv
```

Useful for:
- Large datasets
- Daily archiving
- Time-series analysis

## Hidden Liquidity

Two modes for recording hidden/iceberg orders:

### Combined (default)
```csv
timestamp,seq,type,side,price,visible_qty,hidden_qty
1770098127337417440,15,snapshot,ask,100100000000,10000000000,500000000
```

### Separate Files
```
BTCUSD_PERP_orderbook.csv          # Visible orders
BTCUSD_PERP_orderbook_hidden.csv   # Hidden orders
```

Enable with `SeparateHiddenFiles: true`

## Visualization

Updated visualization script for new format:

```bash
python3 visualize_orderbook.py \
    --trades output/BTCUSD_PERP_trades.csv \
    --orderbook output/BTCUSD_PERP_orderbook.csv \
    --symbol BTCUSD \
    --bin-width 0.5 \
    --output output/btcusd_perp_viz.png
```

Note: Symbol parameter is now optional since files are pre-filtered.

## Data Replay

To replay recorded data:

1. **Read snapshot**: Build initial book state from all snapshot rows at same timestamp
2. **Apply deltas**: Process delta rows in sequence number order
3. **Handle qty=0**: Remove price level from book
4. **Verify integrity**: Check sequence numbers for gaps

Example Python code:

```python
import polars as pl

# Load orderbook
ob = pl.read_csv("BTCUSD_PERP_orderbook.csv")

# Get initial snapshot
snapshot = ob.filter(
    (pl.col("type") == "snapshot") &
    (pl.col("timestamp") == ob["timestamp"].min())
)

# Build book from snapshot
bids = snapshot.filter(pl.col("side") == "bid").to_dict()
asks = snapshot.filter(pl.col("side") == "ask").to_dict()

# Apply deltas in order
deltas = ob.filter(pl.col("type") == "delta").sort("seq")
for delta in deltas.iter_rows(named=True):
    price = delta["price"]
    side = delta["side"]
    qty = delta["visible_qty"]

    if qty == 0:
        # Delete level
        del book[side][price]
    else:
        # Update level
        book[side][price] = qty
```

## Performance

- **Buffered writes**: 10000-event buffer with periodic flushing
- **Non-blocking**: Recorder never blocks exchange operations
- **Minimal overhead**: Single writer goroutine per recorder
- **Efficient format**: CSV is human-readable and efficiently parsed

## Testing

All recorder functionality is tested:

```bash
go test ./actor -v -run TestRecorder
```

Tests cover:
- File creation and naming
- Snapshot/delta recording
- Trade recording
- Separate hidden files
- Multi-instrument recording

## Differences from Previous Version

| Aspect | Old | New |
|--------|-----|-----|
| **File organization** | Mixed (trades.csv, book_observed.csv) | Per-instrument (BTCUSD_PERP_trades.csv) |
| **Snapshot format** | Individual rows | Type column (snapshot/delta) |
| **Delta semantics** | Cumulative qty | Absolute qty (0=delete) |
| **Sequence numbers** | Hardcoded to 0 | Real sequence numbers |
| **Open interest** | Not recorded | Recorded for perps/futures |
| **Funding** | Not recorded | Recorded for perps |
| **Hidden liquidity** | Separate file only | Combined or separate |
| **Symbol field** | In CSV | In filename |

## Backward Compatibility

**Breaking changes**:
- New constructor signature: `NewRecorder(..., instruments map[string]exchange.Instrument)`
- New config structure (removed old field names)
- New CSV format (incompatible with old readers)

Migration guide:
1. Update recorder constructor calls to pass instruments map
2. Update config to use new field names
3. Update CSV readers to handle new format
4. Use per-instrument file paths instead of combined files

## Future Enhancements

Potential improvements:
- [ ] JSON output format option
- [ ] Compression (gzip) support
- [ ] AWS S3 / cloud storage integration
- [ ] Real-time streaming via websocket
- [ ] Parquet format for analytics
- [ ] Depth aggregation (e.g., 0.01% price buckets)
