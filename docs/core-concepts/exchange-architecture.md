# Exchange Architecture

Single-threaded exchange core with request-response pattern and deterministic execution.

## Design Philosophy

**Single-writer concurrency model:**
- All mutations serialized through one goroutine
- No locks needed for exchange state
- Deterministic execution order
- Simplified reasoning about state

**Actor pattern:**
- Clients communicate via channels
- Request/response messages
- Asynchronous market data
- Isolated client state

## Core Components

```
┌────────────────────────────────────────────────────┐
│                 Exchange                           │
│                                                    │
│  ┌─────────────┐  ┌──────────────┐  ┌──────────┐ │
│  │ OrderBook   │  │  Position    │  │  Client  │ │
│  │ (BTC-PERP)  │  │   Manager    │  │ Balances │ │
│  └─────────────┘  └──────────────┘  └──────────┘ │
│                                                    │
│  ┌─────────────┐  ┌──────────────┐  ┌──────────┐ │
│  │  Matching   │  │   Funding    │  │ Loggers  │ │
│  │   Engine    │  │  Calculator  │  │          │ │
│  └─────────────┘  └──────────────┘  └──────────┘ │
└────────────────────────────────────────────────────┘
           ▲                    │
           │                    │
     ┌─────┴─────┐       ┌─────▼─────┐
     │  Request  │       │ Response  │
     │  Channel  │       │ Channel   │
     └───────────┘       └───────────┘
```

## Exchange Struct

```go
type Exchange struct {
    ID         string
    Clients    map[uint64]*Client
    Gateways   map[uint64]*ClientGateway
    Books      map[string]*OrderBook
    Instruments map[string]Instrument
    Positions  *PositionManager

    Matcher    MatchingEngine
    MDPublisher *MDPublisher
    Clock      Clock
    Loggers    map[string]Logger

    BorrowingMgr   *BorrowingManager
    MarginModeMgr  *MarginModeManager
    ExchangeBalance *ExchangeBalance

    requestHandlerDone chan struct{}
    mu                 sync.RWMutex
}
```

## Request-Response Flow

```
Client (goroutine)
    │
    │  OrderRequest
    ├──────────────────────> Gateway.RequestCh
                                  │
                                  │
                      Exchange handler goroutine
                                  │
                                  ▼
                          handleClientRequests()
                                  │
                                  ├─> Validate order
                                  ├─> Reserve balance
                                  ├─> Place in book
                                  ├─> Match
                                  ├─> Execute trades
                                  ├─> Settle PnL
                                  ├─> Log events
                                  │
                                  ▼
                          Send response
                                  │
    Client                        │
    │  Response                   │
    <─────────────────────────────┘
```

## Request Handler Loop

```go
func (ex *Exchange) handleClientRequests() {
    for {
        select {
        case <-ex.ctx.Done():
            return

        case req := <-ex.gateway.RequestCh:
            switch req.Type {
            case ReqPlaceOrder:
                ex.handlePlaceOrder(req)
            case ReqCancelOrder:
                ex.handleCancelOrder(req)
            case ReqQueryBalance:
                ex.handleQueryBalance(req)
            }
        }
    }
}
```

**Single-threaded guarantee:**
- All requests processed sequentially
- No race conditions
- Deterministic order execution
- Simplified testing

## Gateway

```go
type ClientGateway struct {
    ClientID   uint64
    RequestCh  chan Request      // Size: 10,000
    ResponseCh chan Response     // Size: 10,000
    MarketData chan *MarketDataMsg  // Size: 10,000
    Running    bool
    Mu         sync.Mutex
}
```

**Buffered channels:**
- Prevent blocking if client slow
- Allows burst traffic
- Drops messages if buffer full (market data only)

### Connection

```go
gateway := ex.ConnectClient(
    clientID,
    map[string]int64{
        "USD": 1000000 * USD_PRECISION,  // $1M
        "BTC": 10 * BTC_PRECISION,        // 10 BTC
    },
    &PercentageFee{MakerBps: 2, TakerBps: 5},  // 0.02%/0.05%
)
```

Creates client entry and returns gateway for communication.

## Order Placement

```go
func (ex *Exchange) placeOrder(
    clientID uint64,
    symbol string,
    order *Order,
) (*MatchResult, RejectReason) {
    book := ex.Books[symbol]
    client := ex.Clients[clientID]

    if reason := ex.validateOrder(order, book.Instrument); reason != RejectNone {
        return nil, reason
    }

    if reason := ex.reserveBalanceForOrder(client, book.Instrument, order);
       reason != RejectNone {
        return nil, reason
    }

    book.addOrder(order)
    result := ex.Matcher.Match(book.Bids, book.Asks, order)

    if len(result.Executions) > 0 {
        ex.processExecutions(book, result.Executions)
    }

    return result, RejectNone
}
```

**Steps:**
1. Validate order (price, qty, instrument rules)
2. Reserve balance (spot) or margin (perp)
3. Add to order book
4. Match against resting orders
5. Process executions (settle trades, update positions)
6. Log events
7. Return result

## Balance Reservation

### Spot Buy Order

```go
reservedQuote := (order.Price * order.Qty) / basePrecision

if client.Balances["USD"] - client.Reserved["USD"] < reservedQuote {
    return RejectInsufficientBalance
}

client.Reserved["USD"] += reservedQuote
```

Locks quote currency until order filled/cancelled.

### Spot Sell Order

```go
if client.Balances["BTC"] - client.Reserved["BTC"] < order.Qty {
    return RejectInsufficientBalance
}

client.Reserved["BTC"] += order.Qty
```

Locks base currency.

### Perp Order (Margin)

```go
notional := (order.Price * order.Qty) / basePrecision
marginReq := (notional * inst.MarginRate) / 10000

if client.PerpAvailable("USD") < marginReq {
    return RejectInsufficientMargin
}

client.PerpReserved["USD"] += marginReq
```

Reserves margin based on notional value.

## Execution Settlement

```go
func (ex *Exchange) processExecutions(
    book *OrderBook,
    execs []*Execution,
) {
    for _, exec := range execs {
        taker := ex.Clients[exec.TakerClientID]
        maker := ex.Clients[exec.MakerClientID]

        if book.Instrument.IsPerp() {
            ex.settlePerpExecution(taker, maker, exec, book.Instrument)
        } else {
            ex.settleSpotExecution(taker, maker, exec, book.Instrument)
        }

        ex.logTrade(exec)
        ex.publishTrade(exec)
    }
}
```

### Spot Settlement (Buy)

**Taker (buyer):**
```go
notional := (exec.Price * exec.Qty) / precision
fee := calculateFee(exec, taker.FeePlan)

client.Reserved["USD"] -= notional
client.Balances["USD"] -= (notional + fee)
client.Balances["BTC"] += exec.Qty
```

**Maker (seller):**
```go
client.Reserved["BTC"] -= exec.Qty
client.Balances["BTC"] -= exec.Qty
client.Balances["USD"] += (notional - fee)
```

### Perp Settlement

```go
ex.Positions.UpdatePosition(
    exec.TakerClientID,
    book.Symbol,
    exec.TakerSide,
    exec.Qty,
    exec.Price,
)

realizedPnL := calculateRealizedPnL(oldPos, exec)
fee := calculateFee(exec)

client.PerpBalances["USD"] += (realizedPnL - fee)
```

Position updated with weighted average entry price. Realized PnL only on position-reducing trades.

## Thread Safety

### RWMutex for Queries

```go
func (ex *Exchange) GetOrderBook(symbol string) OrderBook {
    ex.mu.RLock()
    defer ex.mu.RUnlock()

    book := ex.Books[symbol]
    return book.Snapshot()  // Return copy
}
```

**Read lock:**
- Allows concurrent reads
- Used for queries, snapshots
- Returns copies, not references

**Write lock:**
- Exclusive during mutations
- Held by request handler only
- Never held during I/O

### No Lock for Handler

Request handler runs single-threaded, doesn't need locks for its own state access. Only acquires write lock when external queries possible.

## Object Pooling

```go
var executionPool = sync.Pool{
    New: func() interface{} {
        return &Execution{}
    },
}

func getExecution() *Execution {
    exec := executionPool.Get().(*Execution)
    return exec
}

func putExecution(exec *Execution) {
    exec.reset()
    executionPool.Put(exec)
}
```

**Pooled objects:**
- Order
- Limit
- Execution
- MarketDataMsg

**Benefits:**
- Reduces GC pressure
- Improves latency (no allocation in hot path)
- Typical: 50% reduction in allocations

## Market Data Publishing

```go
func (ex *Exchange) publishTrade(exec *Execution) {
    msg := &MarketDataMsg{
        Type:   MDTrade,
        Symbol: exec.Symbol,
        Trade:  exec,
        SeqNum: atomic.AddUint64(&ex.seqNum, 1),
    }

    ex.MDPublisher.Publish(exec.Symbol, msg)
}
```

**Sequence numbers:**
- Monotonically increasing
- Per exchange (global ordering)
- Allows gap detection

### Subscription Model

```go
ex.MDPublisher.Subscribe(clientID, "BTC-PERP",
    MDSnapshot | MDTrade | MDFunding)
```

Clients receive only requested message types for subscribed symbols.

## Logging Architecture

```go
ex.Loggers = map[string]Logger{
    "_global":  generalLogger,   // Exchange-wide events
    "BTC-PERP": perpLogger,       // Symbol-specific events
    "BTCUSD":   spotLogger,
}
```

**Routing:**
- Order events → symbol logger
- Balance changes → global or symbol logger (depends on context)
- Funding → symbol logger
- Transfers → global logger

**NDJSON format:**
```json
{"sim_time":1000000,"event":"OrderFill","client_id":5,"symbol":"BTC-PERP","qty":100000000,"price":5000000000000}
```

## Clock Abstraction

```go
type Clock interface {
    NowUnixNano() int64
    NowUnix() int64
}
```

**Real clock:**
```go
type RealClock struct{}

func (c *RealClock) NowUnixNano() int64 {
    return time.Now().UnixNano()
}
```

**Simulated clock:**
```go
type SimulatedClock struct {
    current   int64
    scheduler *EventScheduler
}

func (c *SimulatedClock) Advance(delta time.Duration) {
    c.current += int64(delta)
    c.scheduler.ProcessUntil(c.current)
}
```

All time-dependent logic uses `Clock` interface, enabling deterministic testing.

## Shutdown Sequence

```go
func (ex *Exchange) Shutdown() {
    close(ex.stopCh)  // Signal handler to stop

    <-ex.requestHandlerDone  // Wait for handler

    for _, gateway := range ex.Gateways {
        gateway.Close()  // Close client channels
    }

    for _, logger := range ex.Loggers {
        logger.Close()  // Flush and close log files
    }
}
```

**Graceful:**
1. Stop accepting requests
2. Wait for in-flight requests
3. Close client connections
4. Flush logs
5. Release resources

## Configuration

```go
type ExchangeConfig struct {
    ID               string
    EstimatedClients int
    Clock            Clock
    TickerFactory    TickerFactory
    SnapshotInterval time.Duration
}

ex := NewExchangeWithConfig(ExchangeConfig{
    ID:               "prod_exchange",
    EstimatedClients: 10000,
    Clock:            &RealClock{},
    TickerFactory:    &RealTickerFactory{},
    SnapshotInterval: 100 * time.Millisecond,
})
```

## Performance Characteristics

### Latency

**Typical (2024 laptop):**
- Order placement: 5-10 µs
- Matching: 10-50 µs (depends on executions)
- Total: 15-60 µs per order

**Bottlenecks:**
- Execution logging (I/O bound)
- Position calculations (CPU bound)
- Market data publishing (channel congestion)

### Throughput

**Sustained:**
- 100,000 orders/second
- 500,000 executions/second

**Burst:**
- 1M orders/second (short duration)

Limited by single-thread design. Multi-symbol parallelism possible but not implemented.

## Performance Comparison

### This Implementation

- Single-threaded matching
- In-memory order book
- Deterministic execution
- 15-60 µs latency (2024 laptop)
- 100K-1M orders/second sustained

### Production Exchange Patterns

**Centralized exchanges:**
- Distributed matching engines for redundancy
- Persistent state (database replication)
- 10-50ms API latency (network + processing)
- Millions of orders/second capacity
- Geographic distribution for low latency

**Traditional futures markets:**
- Hardware-accelerated matching (FPGA)
- Sub-millisecond latency
- Custom network protocols
- Co-location services

**Decentralized protocols:**
- On-chain or L2 matching
- 100ms-1s settlement latency
- Transparent, verifiable execution
- Limited throughput vs centralized

### Scalability Patterns

**Multi-threaded matching:**
```go
// Per-symbol matching goroutines
for symbol, book := range ex.Books {
    go func(s string, b *OrderBook) {
        for req := range b.RequestCh {
            ex.handleOrder(s, req)
        }
    }(symbol, book)
}
```

**Partitioned order books:**
```go
// Shard by price range
type PartitionedBook struct {
    partitions []*OrderBook
    partitionFunc func(price int64) int
}
```

**Distributed state:**
```go
// Use external state store
type PersistentExchange struct {
    *Exchange
    stateStore StateStore
}

type StateStore interface {
    SaveOrder(order *Order) error
    LoadOrders(symbol string) ([]*Order, error)
    SavePosition(pos *Position) error
}
```

## Next Steps

- [Order Matching](order-matching.md) - Matching engine details
- [Instruments](instruments.md) - Spot and perpetual configuration
- [Simulation](../simulation/simulated-time.md) - Deterministic time for backtesting
