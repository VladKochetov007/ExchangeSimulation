# Logging System

NDJSON event logging with per-symbol and global loggers for complete audit trails.

## Logger Interface

```go
type Logger interface {
    LogEvent(simTime int64, clientID uint64, eventName string, event any)
}
```

**Simple contract:**
- `simTime`: Simulation timestamp (nanoseconds)
- `clientID`: Actor/client identifier
- `eventName`: Event type string
- `event`: Event data struct (JSON serializable)

## NDJSON Format

**Newline-Delimited JSON:**
- One JSON object per line
- No array wrapper
- Streamable
- Easy to parse incrementally
- Standard for log processing

**Example:**
```json
{"sim_time":1000000000,"server_time":1707925123456789000,"event":"OrderAccepted","client_id":2,"order_id":1}
{"sim_time":1001000000,"server_time":1707925123457890000,"event":"OrderFill","client_id":2,"order_id":1,"qty":100000000}
```

## Implementation

```go
type Logger struct {
    writer io.Writer
    mu     sync.Mutex
}

func New(w io.Writer) *Logger {
    return &Logger{writer: w}
}

func (l *Logger) LogEvent(simTime int64, clientID uint64, eventName string, event any) {
    l.mu.Lock()
    defer l.mu.Unlock()

    entry := map[string]any{
        "sim_time":    simTime,
        "server_time": time.Now().UnixNano(),
        "event":       eventName,
        "client_id":   clientID,
    }

    if event != nil {
        eventBytes, _ := json.Marshal(event)
        var eventMap map[string]any
        json.Unmarshal(eventBytes, &eventMap)

        for k, v := range eventMap {
            entry[k] = v
        }
    }

    json.NewEncoder(l.writer).Encode(entry)
}
```

**Key features:**
- Merges event struct fields into entry (flat structure)
- Thread-safe with mutex
- Buffered writes via io.Writer
- Automatic flushing per event

## Log Organization

### Two-Level Structure

```
logs/simulation_name/
├── general.log           # Global logger (_global key)
└── perp/
    └── BTC-PERP.log      # Symbol logger (BTC-PERP key)
```

**Global logger:** Exchange-wide events
- Balance changes (transfers, margin operations)
- System events
- Cross-symbol operations

**Symbol loggers:** Per-instrument events
- Order acceptance/rejection
- Fills and executions
- Trades
- Position updates
- Funding events

## Setup

```go
logDir := "logs/my_simulation"
os.MkdirAll(logDir, 0755)

// Global logger
generalFile, _ := os.Create(filepath.Join(logDir, "general.log"))
generalLogger := logger.New(generalFile)
ex.SetLogger("_global", generalLogger)

// Symbol logger
perpDir := filepath.Join(logDir, "perp")
os.MkdirAll(perpDir, 0755)

perpFile, _ := os.Create(filepath.Join(perpDir, "BTC-PERP.log"))
perpLogger := logger.New(perpFile)
ex.SetLogger("BTC-PERP", perpLogger)
```

## Event Routing

```go
func (ex *Exchange) logOrderAccepted(symbol string, clientID uint64, orderID uint64) {
    logger := ex.Loggers[symbol]  // Symbol-specific logger
    if logger == nil {
        logger = ex.Loggers["_global"]  // Fallback
    }

    logger.LogEvent(
        ex.Clock.NowUnixNano(),
        clientID,
        "OrderAccepted",
        map[string]any{
            "symbol":   symbol,
            "order_id": orderID,
        },
    )
}
```

**Routing rules:**
1. Symbol-specific events → symbol logger
2. No symbol or global events → global logger
3. Missing symbol logger → fallback to global

## Event Types Logged

### Order Lifecycle

**OrderAccepted:**
```json
{
  "sim_time": 1000000000,
  "server_time": 1707925123456789000,
  "event": "OrderAccepted",
  "client_id": 2,
  "symbol": "BTC-PERP",
  "order_id": 1,
  "side": "Buy",
  "price": 5000000000000,
  "qty": 100000000
}
```

**OrderRejected:**
```json
{
  "event": "OrderRejected",
  "client_id": 2,
  "symbol": "BTC-PERP",
  "reason": "InsufficientBalance"
}
```

**OrderFill:**
```json
{
  "event": "OrderFill",
  "client_id": 2,
  "symbol": "BTC-PERP",
  "order_id": 1,
  "side": "Buy",
  "price": 5000000000000,
  "qty": 50000000,
  "trade_id": 1,
  "fee_asset": "USD",
  "fee_amount": 125000000
}
```

**OrderCancelled:**
```json
{
  "event": "OrderCancelled",
  "client_id": 2,
  "order_id": 1,
  "remaining_qty": 50000000
}
```

### Balance Changes

**BalanceChange:**
```json
{
  "event": "balance_change",
  "client_id": 2,
  "reason": "trade_settlement",
  "changes": [
    {
      "asset": "USD",
      "wallet": "perp",
      "old_balance": 10000000000000000,
      "new_balance": 10000005000000000,
      "delta": 5000000000
    }
  ]
}
```

**Reasons:**
- `trade_settlement`: Trade execution
- `funding_settlement`: Funding payment
- `transfer`: Spot ↔ Perp transfer
- `borrow`: Margin borrow
- `repay`: Margin repayment
- `allocate_collateral`: Isolated margin allocation
- `release_collateral`: Isolated margin release

### Position Events

**PositionUpdate:**
```json
{
  "event": "position_update",
  "client_id": 2,
  "symbol": "BTC-PERP",
  "old_size": 0,
  "new_size": 1000000000,
  "old_entry_price": 0,
  "new_entry_price": 5000000000000,
  "realized_pnl": 0
}
```

**RealizedPnL:**
```json
{
  "event": "realized_pnl",
  "client_id": 2,
  "symbol": "BTC-PERP",
  "pnl": 2000000000000,
  "closed_qty": 500000000
}
```

### Funding

**FundingRateUpdate:**
```json
{
  "event": "funding_rate_update",
  "symbol": "BTC-PERP",
  "rate": 10,
  "index_price": 5000000000000,
  "mark_price": 5000100000000,
  "next_funding": 1707925200000000000
}
```

**FundingSettlement:**
```json
{
  "event": "funding_settlement",
  "client_id": 2,
  "symbol": "BTC-PERP",
  "payment": 125000000000,
  "position_size": 1000000000,
  "funding_rate": 10
}
```

### Market Data

**Trade:**
```json
{
  "event": "Trade",
  "symbol": "BTC-PERP",
  "price": 5000000000000,
  "qty": 100000000,
  "side": "Buy",
  "timestamp": 1707925123456789000,
  "trade_id": 1
}
```

**BookSnapshot:**
```json
{
  "event": "BookSnapshot",
  "symbol": "BTC-PERP",
  "seq_num": 42,
  "bids": [
    {"price": 5000000000000, "qty": 500000000},
    {"price": 4999900000000, "qty": 300000000}
  ],
  "asks": [
    {"price": 5000100000000, "qty": 400000000},
    {"price": 5000200000000, "qty": 600000000}
  ]
}
```

## Balance Snapshots

Periodic complete balance state:

```go
ex.EnableBalanceSnapshots(10 * time.Second)
```

**BalanceSnapshot event:**
```json
{
  "event": "balance_snapshot",
  "timestamp": 1707925123456789000,
  "client_id": 2,
  "spot_balances": [
    {
      "asset": "BTC",
      "total": 1000000000,
      "available": 800000000,
      "reserved": 200000000
    },
    {
      "asset": "USD",
      "total": 100000000000000,
      "available": 95000000000000,
      "reserved": 5000000000000
    }
  ],
  "perp_balances": [
    {
      "asset": "USD",
      "total": 50000000000000,
      "available": 40000000000000,
      "reserved": 10000000000000
    }
  ],
  "borrowed": {
    "USD": 10000000000000
  }
}
```

**Use cases:**
- Verify balance correctness
- Reconstruct state from logs
- Detect balance discrepancies

## Buffered Writing

```go
logFile, _ := os.Create("trade.log")
buffered := bufio.NewWriter(logFile)
logger := logger.New(buffered)

// ... logging happens ...

buffered.Flush()  // Ensure all written
logFile.Close()
```

**Benefits:**
- Reduced syscalls
- Better throughput
- Lower latency (non-blocking)

**Tradeoff:**
- Unflushed data lost on crash
- Use `bufio.Writer` for performance
- Call `Flush()` periodically or at shutdown

## Parsing Logs

### Python

```python
import json

with open('logs/simulation/perp/BTC-PERP.log') as f:
    for line in f:
        event = json.loads(line)
        if event['event'] == 'OrderFill':
            print(f"Fill: {event['qty']} @ {event['price']}")
```

### jq (command-line)

```bash
# Filter to fills only
jq -c 'select(.event == "OrderFill")' BTC-PERP.log

# Extract prices
jq -r '.price' BTC-PERP.log | head -10

# Count events by type
jq -r '.event' BTC-PERP.log | sort | uniq -c
```

### Go

```go
file, _ := os.Open("BTC-PERP.log")
scanner := bufio.NewScanner(file)

for scanner.Scan() {
    var event map[string]interface{}
    json.Unmarshal(scanner.Bytes(), &event)

    if event["event"] == "OrderFill" {
        fmt.Printf("Fill: %v\n", event)
    }
}
```

## Log Rotation

For long-running simulations:

```go
type RotatingLogger struct {
    baseDir    string
    maxLines   int
    currentLog *Logger
    lineCount  int
    fileIndex  int
}

func (rl *RotatingLogger) LogEvent(...) {
    if rl.lineCount >= rl.maxLines {
        rl.rotate()
    }

    rl.currentLog.LogEvent(...)
    rl.lineCount++
}

func (rl *RotatingLogger) rotate() {
    rl.currentLog.Close()
    rl.fileIndex++
    filename := fmt.Sprintf("%s/log.%04d.ndjson", rl.baseDir, rl.fileIndex)
    rl.currentLog = logger.New(createFile(filename))
    rl.lineCount = 0
}
```

## Performance

**Typical throughput:**
- 100,000 events/second (unbuffered)
- 500,000+ events/second (buffered)
- Bottleneck: I/O, not JSON encoding

**Optimization:**
- Use buffered writers
- Write to SSD (not HDD)
- Batch flushes
- Compress completed logs (gzip)

## Real-World Comparison

### This Implementation

- NDJSON format
- Per-symbol + global files
- Synchronous writes (with buffering)
- File-based storage

### Production Exchanges

**Binance:**
- Real-time WebSocket streams
- Historical via REST API
- Kafka for internal logging
- S3 for archival

**CME:**
- FIX protocol for live
- Market Data Replay (MDR) for historical
- Proprietary binary format

**dYdX:**
- On-chain event logs (Ethereum/StarkEx)
- Indexer for query API
- Immutable, cryptographically verified

## Next Steps

- [Balance Tracking](balance-tracking.md) - Balance snapshot mechanics
- [Analysis Tools](analysis-tools.md) - Python scripts for log analysis
- [Random Walk Example](../quickstart/02-randomwalk-example.md) - Complete logging example
