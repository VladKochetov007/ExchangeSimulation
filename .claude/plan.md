# Exchange Simulation - Core Architecture Plan

## Design Philosophy

Following the HFT orderbook research and Go performance guidelines from CLAUDE.md:

1. **Zero allocation hot paths** - Use object pools, preallocated slices
2. **Pointer surgery over search** - O(1) operations via direct pointer manipulation
3. **Separation of concerns** - Orders, price levels, book sides strictly separated
4. **Cache locality** - Contiguous memory layouts, avoid pointer chasing
5. **Minimal code** - No defensive bloat, no unnecessary abstraction layers

## Module Structure

```
exchange_sim/
├── exchange/
│   ├── types.go              # Core types: Order, Limit, Side enums
│   ├── order.go              # Order lifecycle and linking
│   ├── limit.go              # Price level (FIFO queue) operations
│   ├── book.go               # Orderbook structure and L3 operations
│   ├── matching.go           # Matching engine interface + default impl
│   ├── client.go             # Client accounts, balances, reserves
│   ├── fee.go                # Fee model interface + implementations
│   ├── instrument.go         # Instrument interface (spot/perp futures)
│   ├── exchange.go           # Top-level exchange orchestration
│   ├── gateway.go            # Client request/response gateway
│   ├── marketdata.go         # Market data publisher (snapshots/deltas/trades)
│   ├── funding.go            # Funding rate calculation for perp futures
│   └── pools.go              # sync.Pool for Order/Limit reuse
├── cmd/
│   └── main.go               # Simulation entry point
└── go.mod
```

## Core Data Structures

### Order (Doubly-Linked List Node)

```go
type Order struct {
    ID          uint64
    ClientID    uint64
    Side        Side           // Buy/Sell
    Type        OrderType      // Market/Limit
    TimeInForce TimeInForce    // GTC/IOC/FOK
    Price       int64          // Price in ticks
    Qty         int64          // Quantity in base asset units
    FilledQty   int64
    Visibility  Visibility     // Normal/Iceberg/Hidden
    IcebergQty  int64          // Display qty for iceberg
    Status      OrderStatus    // Open/PartialFill/Filled/Cancelled/Rejected
    Timestamp   int64          // Nanoseconds

    // Linked list pointers
    Prev   *Order
    Next   *Order
    Parent *Limit  // O(1) access to price level
}
```

**Key decisions:**
- `int64` for price/qty avoids float imprecision
- Embedded list pointers for O(1) cancel
- Parent pointer for O(1) limit access
- No comments explaining field names (anti-slop)

### Limit (Price Level - FIFO Queue)

```go
type Limit struct {
    Price     int64
    TotalQty  int64
    OrderCnt  int32

    Head *Order
    Tail *Order

    // Price ladder links (for sparse array approach)
    Prev *Limit
    Next *Limit
}
```

**Key decisions:**
- Cached aggregates (TotalQty, OrderCnt) for O(1) queries
- Doubly-linked for O(1) limit removal
- No tree structure - using hybrid ladder approach

### Book (One Side)

```go
type Book struct {
    Side      Side
    Ladder    []*Limit        // Sparse array indexed by tick
    Best      *Limit          // Cached best price
    ActiveHead *Limit         // Linked list of active prices
    ActiveTail *Limit

    Orders    map[uint64]*Order  // Order ID -> Order
    Limits    map[int64]*Limit   // Price -> Limit
}
```

**Key decisions:**
- Hybrid structure: sparse array + active price linked list
- Best pointer cached for O(1) access
- Separate maps for O(1) lookups
- Buy and sell are separate instances

### Client Account

```go
type Client struct {
    ID           uint64
    Balances     map[string]int64    // Asset -> quantity
    Reserved     map[string]int64    // Asset -> reserved qty
    OrderIDs     []uint64            // Active order IDs
    FeePlan      FeePlan
    VIPLevel     int
    MakerVolume  int64
    TakerVolume  int64
}
```

**Key decisions:**
- String keys for assets (BTC, USD, USDT, etc.)
- Reserved tracking for limit order collateral
- Volume tracking for VIP tier calculation
- Slice not map for OrderIDs (most clients have few orders)

### Client Gateway (Communication Layer)

```go
type ClientGateway struct {
    ClientID   uint64
    RequestCh  chan Request
    ResponseCh chan Response
    MarketData chan MarketDataMsg
    Running    bool
}

type Request struct {
    Type      RequestType
    OrderReq  *OrderRequest
    CancelReq *CancelRequest
    QueryReq  *QueryRequest
}

type RequestType uint8
const (
    ReqPlaceOrder RequestType = iota
    ReqCancelOrder
    ReqQueryBalance
    ReqQueryOrders
    ReqSubscribe
    ReqUnsubscribe
)

type Response struct {
    RequestID uint64
    Success   bool
    Data      interface{}  // OrderAck, BalanceSnapshot, etc.
    Error     RejectReason
}

type OrderRequest struct {
    RequestID   uint64
    Side        Side
    Type        OrderType
    Price       int64
    Qty         int64
    Symbol      string
    TimeInForce TimeInForce
    Visibility  Visibility
    IcebergQty  int64
}

type CancelRequest struct {
    RequestID uint64
    OrderID   uint64
}

type QueryRequest struct {
    RequestID uint64
    QueryType QueryType
    Symbol    string
}

type QueryType uint8
const (
    QueryBalance QueryType = iota
    QueryOrders
    QueryOrder
)
```

**Key decisions:**
- Separate channels for requests, responses, market data
- Buffered channels (size configurable, default 1000)
- Request/Response pattern for sync operations
- Pub/Sub pattern for market data
- Single gateway per client (one goroutine reads requests)

### Balance Query Response

```go
type BalanceSnapshot struct {
    Timestamp int64
    Balances  []AssetBalance
}

type AssetBalance struct {
    Asset     string
    Total     int64
    Available int64  // Total - Reserved
    Reserved  int64
}
```

**Key decisions:**
- Slice not map (typically < 10 assets per client)
- Explicit Available calculation
- Timestamp for client reconciliation

### Market Data Subscriptions

```go
type MarketDataMsg struct {
    Type      MDType
    Symbol    string
    SeqNum    uint64
    Timestamp int64
    Data      interface{}  // Snapshot, Delta, or Trade
}

type MDType uint8
const (
    MDSnapshot MDType = iota
    MDDelta
    MDTrade
    MDFunding
)

type BookSnapshot struct {
    Bids []PriceLevel
    Asks []PriceLevel
}

type BookDelta struct {
    Side   Side
    Price  int64
    Qty    int64  // 0 means level removed
}

type Trade struct {
    TradeID      uint64
    Price        int64
    Qty          int64
    Side         Side  // Aggressor side
    TakerOrderID uint64
    MakerOrderID uint64
}

type PriceLevel struct {
    Price int64
    Qty   int64
}

type Subscription struct {
    ClientID uint64
    Symbol   string
    Types    []MDType
}
```

**Key decisions:**
- Sequence numbers for gap detection
- Delta updates for efficiency (only changes)
- Snapshot on subscribe + periodic snapshots
- Trade includes aggressor side for market direction
- Multiple subscription types per client/symbol

### Market Data Publisher

```go
type MDPublisher struct {
    subscriptions map[string]map[uint64]*Subscription  // Symbol -> ClientID -> Sub
    mu            sync.RWMutex
    seqNum        uint64
    snapshotPool  sync.Pool
    deltaPool     sync.Pool
}
```

**Key decisions:**
- Nested map for fast lookup by symbol
- RWMutex (many readers during matching, few writers on sub/unsub)
- Atomic seqNum increment
- Pools for snapshot/delta structs

### Funding Rate (Perp Futures)

```go
type FundingRate struct {
    Symbol       string
    Rate         int64  // Basis points (1/10000)
    NextFunding  int64  // Unix timestamp
    Interval     int64  // Seconds (typically 28800 = 8 hours)
    MarkPrice    int64
    IndexPrice   int64
}

type FundingCalculator interface {
    Calculate(book *OrderBook, indexPrice int64) int64
}

type SimpleFundingCalc struct {
    BaseRate     int64
    Damping      int64
}
```

**Funding settlement flow:**
```
1. Every 8 hours, calculate funding rate:
   rate = clamp((markPrice - indexPrice) / indexPrice, -maxRate, +maxRate)
2. For each client with open perp position:
   if long: pay funding = position_value * rate
   if short: receive funding = position_value * rate
3. Update client balances (no actual trade, just P&L adjustment)
```

**Key decisions:**
- Basis points for precision
- Mark price = mid of orderbook (or last trade)
- Index price provided externally (or from oracle)
- Simple damping formula (can be replaced with TWAP/EMA)
- Interface for custom funding calculations

## Interface Definitions

### Matching Engine

```go
type MatchingEngine interface {
    Match(book *OrderBook, incomingOrder *Order) []*Execution
    Priority() Priority
}

type Priority struct {
    Primary   PriorityType  // Price/ProRata
    Secondary PriorityType  // Time/Size
    Tertiary  PriorityType  // Visibility
}

type Execution struct {
    TakerOrderID uint64
    MakerOrderID uint64
    Price        int64
    Qty          int64
    Timestamp    int64
}
```

**Key decisions:**
- Interface allows pluggable matching algorithms
- Default: price-time-visibility
- Execution is simple struct, not method with side effects
- Return slice of executions for batch processing

### Fee Model

```go
type FeeModel interface {
    CalculateFee(exec *Execution, side Side, isMaker bool) Fee
}

type Fee struct {
    Asset  string
    Amount int64
}

type PercentageFee struct {
    MakerBps int64
    TakerBps int64
    InQuote  bool  // true = quote asset, false = base asset
}

type FixedFee struct {
    MakerFee Fee
    TakerFee Fee
}
```

**Key decisions:**
- Interface for extensibility
- Two concrete implementations cover most cases
- Basis points for precision
- Explicit asset specification

### Instrument

```go
type Instrument interface {
    Symbol() string
    BaseAsset() string
    QuoteAsset() string
    TickSize() int64
    MinOrderSize() int64  // In quote asset
    ValidatePrice(price int64) bool
    ValidateQty(qty int64) bool
    IsPerp() bool
}

type SpotInstrument struct {
    symbol       string
    base         string
    quote        string
    tickSize     int64
    minOrderSize int64
}

type PerpFutures struct {
    SpotInstrument
    fundingRate     *FundingRate
    fundingCalc     FundingCalculator
}

type Position struct {
    ClientID   uint64
    Symbol     string
    Size       int64  // Positive = long, negative = short
    EntryPrice int64  // Average entry price
    Margin     int64
}
```

**Key decisions:**
- Spot is base implementation
- Perp futures extends spot with funding
- Validation methods for exchange rules
- Tick-based pricing (int64)
- Position tracking for perp futures
- Signed Size field for long/short

## Exchange Orchestration

```go
type Exchange struct {
    Clients       map[uint64]*Client
    Gateways      map[uint64]*ClientGateway
    Books         map[string]*OrderBook
    Instruments   map[string]Instrument
    Positions     map[uint64]map[string]*Position  // ClientID -> Symbol -> Position
    NextOrderID   uint64
    Matcher       MatchingEngine
    MDPublisher   *MDPublisher

    orderPool     sync.Pool
    limitPool     sync.Pool
    executionPool sync.Pool
    mdMsgPool     sync.Pool

    running       bool
    shutdownCh    chan struct{}
}

type OrderBook struct {
    Symbol     string
    Instrument Instrument
    Bids       *Book
    Asks       *Book
    LastTrade  *Trade
    SeqNum     uint64
}
```

**Key decisions:**
- Single Exchange struct owns all state
- Object pools for hot path allocations
- Atomic NextOrderID increment
- One OrderBook per instrument, splits bid/ask
- Gateways map for client communication
- MDPublisher for market data distribution
- Positions map for perp futures tracking
- Graceful shutdown via channel

## Operation Flows

### Add Limit Order

```
1. Validate: client exists, sufficient balance, valid price/qty
2. Reserve collateral (buy: qty*price in quote, sell: qty in base)
3. Check crossing: if crosses, route to matching engine
4. If not filled, add to book:
   a. Get or create Limit at price
   b. Append Order to Limit tail
   c. Update aggregates (TotalQty, OrderCnt)
   d. Update best price if needed
   e. Add to Client.OrderIDs
5. Return order ID or rejection reason
```

**Complexity:** O(1) amortized (only slow if new price level created)

### Cancel Order

```
1. Lookup Order by ID in book.Orders
2. Unlink from Limit (prev.Next = next, next.Prev = prev)
3. Update Limit aggregates
4. If Limit.OrderCnt == 0, remove Limit from ladder and active list
5. Release reserved collateral
6. Remove from Client.OrderIDs
7. Return Order to pool
```

**Complexity:** O(1) strict

### Execute (Matching)

```
1. Pop Head from best price level
2. Create Execution record
3. Update Order.FilledQty
4. If Order fully filled, remove (same as cancel flow)
5. If Limit empty after removal, advance Best pointer
6. Calculate fees for maker and taker
7. Update client balances (debit/credit with fees)
8. Update volume stats for VIP tiers
```

**Complexity:** O(1) per order matched

### Market Order

```
1. Validate client, balance
2. Loop: while qty > 0 and opposite.Best exists:
   a. Execute against Best.Head
   b. Decrement qty
3. If qty > 0 at end, partial fill (or reject if FOK)
4. No book insertion
```

**Complexity:** O(N) where N = number of maker orders hit

### Client Request Handling (Gateway)

```
Exchange runs one goroutine per client gateway:

1. Select on RequestCh:
   - ReqPlaceOrder: validate -> execute -> send Response
   - ReqCancelOrder: cancel -> send Response
   - ReqQueryBalance: build BalanceSnapshot -> send Response
   - ReqQueryOrders: collect client orders -> send Response
   - ReqSubscribe: MDPublisher.Subscribe()
   - ReqUnsubscribe: MDPublisher.Unsubscribe()
2. On shutdown signal: close channels, exit goroutine
```

**Balance query flow:**
```
1. Lock client (read-only)
2. For each asset in client.Balances:
   available = balances[asset] - reserved[asset]
3. Build BalanceSnapshot slice
4. Send via ResponseCh
```

**Complexity:** O(A) where A = number of assets (typically < 10)

### Market Data Publishing

**On order add:**
```
1. After order added to book, check if Limit is new
2. If new or qty changed: create BookDelta
3. MDPublisher.Publish(symbol, delta)
```

**On order cancel:**
```
1. After order removed, check if Limit removed
2. If qty changed: create BookDelta (qty=0 if removed)
3. MDPublisher.Publish(symbol, delta)
```

**On execution:**
```
1. After execution, create Trade message
2. Create BookDelta for qty reduction
3. MDPublisher.Publish(symbol, trade)
4. MDPublisher.Publish(symbol, delta)
```

**On subscribe:**
```
1. Add subscription to MDPublisher.subscriptions[symbol][clientID]
2. Build BookSnapshot from current book state
3. Send snapshot via client.MarketData channel
4. Future updates sent as deltas
```

**Periodic snapshots:**
```
Every N seconds (e.g., 5):
1. For each symbol with subscriptions:
   a. Build BookSnapshot
   b. Send to all subscribed clients
2. Prevents delta accumulation issues
```

**Publishing implementation:**
```go
func (p *MDPublisher) Publish(symbol string, msg *MarketDataMsg) {
    p.mu.RLock()
    defer p.mu.RUnlock()

    subs := p.subscriptions[symbol]
    for clientID, sub := range subs {
        select {
        case sub.Gateway.MarketData <- msg:
        default:
            // Channel full, client too slow - skip
        }
    }
}
```

**Key decisions:**
- Non-blocking sends (default case)
- Slow clients don't block exchange
- Periodic snapshots for recovery
- RLock during publish (no sub changes)

**Complexity:** O(S) where S = subscribers for symbol (typically < 100)

### Funding Rate Calculation (Perp Futures)

**Periodic calculation (every 8 hours):**
```
1. For each perp instrument:
   a. Calculate mark price = (bestBid + bestAsk) / 2
   b. Get index price (from oracle or external feed)
   c. premium = (markPrice - indexPrice) / indexPrice
   d. rate = clamp(premium * damping, -maxRate, +maxRate)
   e. Update instrument.FundingRate
   f. Publish funding rate via market data
```

**Settlement (at funding time):**
```
1. For each client with open perp position:
   a. funding = position.Size * position.EntryPrice * rate / 10000
   b. If long (size > 0): client.Balances[quote] -= funding
   c. If short (size < 0): client.Balances[quote] += funding
2. Emit funding settlement events
```

**Position updates (on trade):**
```
For perp instruments only:
1. Update client position size: position.Size += qty * (buy ? 1 : -1)
2. Update average entry price:
   newSize = oldSize + tradedQty
   newEntry = (oldSize * oldEntry + tradedQty * tradePrice) / newSize
3. Check margin requirements
```

**Key decisions:**
- Funding paid/received in quote asset
- Simple clamp formula (can extend to TWAP)
- Position tracking separate from balances
- Margin checks on position updates

**Complexity:** O(C) where C = clients with open positions

## Memory Management Strategy

### Object Pools

```go
var orderPool = sync.Pool{
    New: func() interface{} {
        return &Order{}
    },
}

var limitPool = sync.Pool{
    New: func() interface{} {
        return &Limit{}
    },
}
```

**Usage:**
- `orderPool.Get()` on order creation
- `orderPool.Put()` on cancel/fill
- Same for limits
- Reduces GC pressure by 70-90% in similar systems

### Preallocated Slices

```go
func NewExchange(estimatedClients int) *Exchange {
    return &Exchange{
        Clients:     make(map[uint64]*Client, estimatedClients),
        Books:       make(map[string]*OrderBook, 16),
        Instruments: make(map[string]Instrument, 16),
    }
}
```

### Zero-Copy Patterns

- Pass `*Order` and `*Limit` everywhere (never copy)
- Use `strings.Builder` for symbol concatenation
- Avoid `[]byte` ↔ `string` conversions

## Rejection Reasons

```go
type RejectReason uint8

const (
    RejectInsufficientBalance RejectReason = iota
    RejectInvalidPrice
    RejectInvalidQty
    RejectUnknownClient
    RejectUnknownInstrument
    RejectSelfTrade
    RejectDuplicateOrderID
)
```

**Key decisions:**
- Enum not string (zero allocation)
- Exhaustive list
- No error wrapping in hot path

## Order Types and Time-in-Force

```go
type OrderType uint8
const (
    Market OrderType = iota
    Limit
)

type TimeInForce uint8
const (
    GTC TimeInForce = iota  // Good-til-cancelled
    IOC                     // Immediate-or-cancel
    FOK                     // Fill-or-kill
)

type Visibility uint8
const (
    Normal Visibility = iota
    Iceberg
    Hidden
)
```

**Execution Priority (Default Matching Engine):**
1. Price (best first)
2. Visibility (Normal > Iceberg > Hidden)
3. Time (earlier first)

**Iceberg:** Only `IcebergQty` visible in book aggregates, rest hidden until previous qty filled

**Hidden:** Not counted in book aggregates, only matchable by incoming orders

## Client Usage Flow Example

```go
// Simulation setup
exchange := NewExchange(1000)
exchange.AddInstrument(NewPerpFutures("BTC/USD", 1, 10))

// Connect client
clientID := uint64(1)
gateway := exchange.ConnectClient(clientID, initialBalances)

// Start client goroutine
go func() {
    for {
        select {
        case resp := <-gateway.ResponseCh:
            // Handle order acks, balance responses, etc.
            processResponse(resp)
        case md := <-gateway.MarketData:
            // Handle market data updates
            processMarketData(md)
        }
    }
}()

// Subscribe to BTC/USD market data
gateway.RequestCh <- Request{
    Type: ReqSubscribe,
    QueryReq: &QueryRequest{
        Symbol: "BTC/USD",
    },
}

// Query balance
gateway.RequestCh <- Request{
    Type: ReqQueryBalance,
    QueryReq: &QueryRequest{
        RequestID: 1,
    },
}

// Place limit order
gateway.RequestCh <- Request{
    Type: ReqPlaceOrder,
    OrderReq: &OrderRequest{
        RequestID: 2,
        Symbol: "BTC/USD",
        Side: Buy,
        Type: Limit,
        Price: 50000 * 100,  // $50,000 (tick size = $1)
        Qty: 1 * 100000000,   // 1 BTC (in satoshis)
        TimeInForce: GTC,
        Visibility: Normal,
    },
}

// Wait for response
resp := <-gateway.ResponseCh
if resp.Success {
    orderID := resp.Data.(uint64)
    // Order placed successfully
}

// Later: cancel order
gateway.RequestCh <- Request{
    Type: ReqCancelOrder,
    CancelReq: &CancelRequest{
        RequestID: 3,
        OrderID: orderID,
    },
}

// Market data arrives asynchronously
md := <-gateway.MarketData
switch md.Type {
case MDSnapshot:
    snapshot := md.Data.(*BookSnapshot)
    // Full orderbook snapshot
case MDDelta:
    delta := md.Data.(*BookDelta)
    // Update local orderbook copy
case MDTrade:
    trade := md.Data.(*Trade)
    // New trade executed
case MDFunding:
    funding := md.Data.(*FundingRate)
    // Funding rate updated
}
```

**Key flow:**
1. Exchange creates gateways for clients
2. Clients send requests via RequestCh
3. Exchange processes requests, sends responses via ResponseCh
4. Market data flows continuously via MarketData channel
5. Clients maintain local orderbook copy using snapshots + deltas

## Testing Strategy

### Unit Tests
- Order linking/unlinking
- Limit FIFO correctness
- Book operations (add/cancel/execute)
- Fee calculations
- Balance reserve/release

### Property Tests
- Orderbook invariants (Best is always best, aggregates match reality)
- No negative balances
- No double-spend

### Benchmark Targets
- Add order: < 100ns
- Cancel order: < 50ns
- Execute order: < 150ns
- 100k orders/sec sustained on single core

## Implementation Order

1. **Phase 1: Core structures**
   - types.go (enums, basic structs, request/response types)
   - order.go (linking operations)
   - limit.go (FIFO queue ops)
   - pools.go (sync.Pool setup for all pooled types)

2. **Phase 2: Book logic**
   - book.go (add/cancel/query)
   - matching.go (default price-time-visibility matcher)

3. **Phase 3: Business logic**
   - fee.go (interfaces + implementations)
   - instrument.go (spot + perp with positions)
   - client.go (accounts, reserves)

4. **Phase 4: Communication layer**
   - gateway.go (client request/response handling)
   - marketdata.go (publisher, subscriptions, snapshots/deltas)
   - funding.go (funding rate calculation and settlement)

5. **Phase 5: Orchestration**
   - exchange.go (top-level API, gateway management, main event loop)
   - Integration tests (full flow: connect -> subscribe -> trade -> query)

6. **Phase 6: Optimization**
   - Benchmarking (with realistic client load)
   - Profile-guided optimization (PGO)
   - Memory layout tuning
   - Channel buffer size tuning

## Anti-Slop Principles Applied

- **No defensive checks in trusted paths** (e.g., no nil checks after map lookups we know succeed)
- **No log-and-rethrow** (errors propagate via return values)
- **No single-use variables** (inline unless reused or improves clarity)
- **No narrative comments** (code documents itself)
- **No unused parameters** (matching engine gets exactly what it needs)
- **No premature abstractions** (two implementations justify interface, not one)
- **No TODO comments without tickets**

## Open Questions for User

None at this stage - design is deterministic from requirements.

## Gateway and Market Data Performance

### Channel Buffer Sizing

```go
const (
    RequestChSize   = 1000    // 1000 pending requests per client
    ResponseChSize  = 1000    // 1000 pending responses
    MarketDataChSize = 10000  // 10k market data messages (bursts during volatile periods)
)
```

**Trade-offs:**
- Larger buffers: higher memory, more buffering capacity
- Smaller buffers: lower latency visibility (backpressure propagates faster)
- Market data needs largest buffer (1 execution can generate N updates)

### Slow Client Handling

```go
select {
case client.MarketData <- msg:
default:
    // Drop message or disconnect slow client
    client.Stats.Dropped++
}
```

**Key decisions:**
- Non-blocking sends to market data channels
- Slow clients don't block exchange core
- Client responsible for handling backpressure
- Alternative: disconnect clients with full channels

### Goroutine Model

**Per client:**
- 1 goroutine reading RequestCh

**Exchange core:**
- 1 goroutine for main event loop (order processing)
- 1 goroutine per orderbook for periodic snapshots (optional)
- 1 goroutine for funding rate calculation (perp only)

**Total:** O(C) goroutines where C = connected clients

**Memory:** ~2-8 KB per goroutine stack (Go 1.25)

### Market Data Efficiency

**Problem:** 100 clients subscribed to BTC/USD, 1 trade executed
- Naive: 100 allocations, 100 channel sends
- Optimized: 1 allocation (pooled), 100 channel sends (unavoidable)

**Solution:**
```go
msg := p.mdMsgPool.Get().(*MarketDataMsg)
msg.Type = MDTrade
msg.Data = trade
// Reuse same msg pointer for all sends
for _, sub := range subs {
    sub.Gateway.MarketData <- msg
}
// Don't return to pool - clients will read it
```

**Issue:** Clients all reference same msg - race condition!

**Correct solution:**
```go
// Option 1: Copy per client (simple, allocates)
for _, sub := range subs {
    msgCopy := *msg  // Stack copy
    sub.Gateway.MarketData <- &msgCopy
}

// Option 2: Reference counting (complex, zero-alloc)
// Not worth it for simulation
```

**Decision for simulation:** Accept allocations on market data publish (not hot path for matching)

## Performance Validation

Post-implementation, we'll verify:
- GC pause < 1ms during 100k orders/sec
- Memory stable (no leaks from pool misuse)
- CPU cache hit rate > 95% (perf stat)
- Zero allocations in cancel path
- Market data lag < 100µs for subscribed clients
- Gateway request latency < 10µs (request to response)
- GOGC tuning if needed
- Channel buffer overflow rate < 0.01%

---

**Plan Size:** ~550 LOC across 12 files
**Estimated Complexity:** Medium-High (orderbook core is proven, communication layer adds concurrency complexity)
**Risk:** Medium
- Low risk: orderbook/matching (proven patterns)
- Medium risk: gateway concurrency, market data fan-out, channel sizing
