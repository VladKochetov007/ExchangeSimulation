# Order Matching

Price-time-visibility priority matching engine with FIFO execution within price levels.

## Matching Priority

Orders match in this priority:

1. **Price**: Best price wins
2. **Visibility**: Normal > Iceberg > Hidden (at same price)
3. **Time**: First-in-first-out (FIFO) within same price and visibility

## Order Book Structure

### Limit Price Level

```go
type Limit struct {
    Price     int64
    Side      Side
    Head      *Order  // First order (oldest)
    Tail      *Order  // Last order (newest)
    Prev      *Limit  // Next better price
    Next      *Limit  // Next worse price
    TotalQty  int64   // Sum of remaining qty
    OrderCnt  int     // Count of orders
}
```

**Linked list of orders:**
```
Limit @ $50,000
  Head -> Order(ID=1, Qty=10) -> Order(ID=2, Qty=5) -> Order(ID=3, Qty=20) -> Tail
  TotalQty = 35
  OrderCnt = 3
```

### Book Side

```go
type Book struct {
    Side       Side
    Best       *Limit         // Best bid or ask
    ActiveHead *Limit         // Linked list of active prices
    ActiveTail *Limit
    Orders     map[uint64]*Order  // Fast lookup by order ID
    Limits     map[int64]*Limit   // Fast lookup by price
}
```

**Bid book structure:**
```
Best = $50,010 (highest bid)
  ↓
$50,010 -> 3 orders, 25 BTC total
  ↓
$50,005 -> 2 orders, 15 BTC total
  ↓
$50,000 -> 5 orders, 40 BTC total
```

## Matching Algorithm

```go
func (m *DefaultMatcher) Match(
    bidBook, askBook *Book,
    order *Order,
) *MatchResult {
    var executions []*Execution

    book := askBook  // Match buy order against ask book
    if order.Side == Sell {
        book = bidBook  // Match sell order against bid book
    }

    for order.FilledQty < order.Qty && book.Best != nil {
        if !priceOverlaps(order, book.Best) {
            break
        }

        for lvl := book.Best; lvl != nil; lvl = lvl.Next {
            if !priceOverlaps(order, lvl) {
                break
            }

            for resting := lvl.Head; resting != nil; {
                if resting.ClientID == order.ClientID {
                    resting = resting.Next
                    continue  // Self-trade prevention
                }

                fillQty := min(order.Qty - order.FilledQty,
                              resting.Qty - resting.FilledQty)

                exec := &Execution{
                    TakerOrderID: order.ID,
                    MakerOrderID: resting.ID,
                    Price:        resting.Price,  // Maker price
                    Qty:          fillQty,
                    Timestamp:    timestamp,
                }
                executions = append(executions, exec)

                order.FilledQty += fillQty
                resting.FilledQty += fillQty

                next := resting.Next
                if resting.FilledQty >= resting.Qty {
                    book.removeOrder(resting)  // Fully filled
                }
                resting = next

                if order.FilledQty >= order.Qty {
                    break  // Taker fully filled
                }
            }
        }
    }

    updateOrderStatus(order)
    return &MatchResult{Executions: executions}
}
```

## Price Overlap Logic

```go
func priceOverlaps(order *Order, limit *Limit) bool {
    if order.Type == Market {
        return true  // Market orders match any price
    }

    if order.Side == Buy {
        return order.Price >= limit.Price  // Buy at or above ask
    }
    return order.Price <= limit.Price  // Sell at or below bid
}
```

**Example:**
- Buy limit @ \$50,100
- Ask levels: \$50,050, \$50,100, \$50,150
- Overlaps: Yes (order price >= ask price)
- Matches at: \$50,050 first, then \$50,100

## Execution Price

**Critical:** Executions occur at the **maker price**, not taker price.

**Example:**
- Resting ask @ \$50,000
- Incoming buy limit @ \$51,000 (aggressive)
- **Execution price: \$50,000** (maker gets price improvement)

This is standard in limit order books (price-time priority).

## Order Types

### Market Order

```go
Order{
    Type: Market,
    Qty:  100 * BTC_PRECISION / 100,  // 1 BTC
}
```

- Matches immediately at best available prices
- No price limit
- May experience slippage across multiple levels
- Rejects if insufficient liquidity (FOK) or partially fills (IOC)

### Limit Order

```go
Order{
    Type:  Limit,
    Price: 50000 * USD_PRECISION,  // $50,000
    Qty:   100 * BTC_PRECISION / 100,
    TimeInForce: GTC,  // Good Till Cancel
}
```

- Matches at price or better
- Unfilled quantity rests in book
- Can be cancelled

## Time In Force

### GTC (Good Till Cancel)

```go
TimeInForce: GTC
```

- Remains in book until filled or cancelled
- Most common for market makers
- Default if not specified

### IOC (Immediate Or Cancel)

```go
TimeInForce: IOC
```

- Fills immediately or cancels unfilled portion
- Never rests in book
- Common for takers who want immediate execution

**Example:**
- IOC buy 10 BTC
- Only 6 BTC available at acceptable price
- Fills 6 BTC, cancels remaining 4 BTC

### FOK (Fill Or Kill)

```go
TimeInForce: FOK
```

- Fills completely or rejects entirely
- All-or-nothing execution
- Used when partial fills unacceptable

**Example:**
- FOK buy 10 BTC
- Only 6 BTC available
- Order rejected completely

## Visibility

### Normal

```go
Visibility: Normal
```

Full quantity visible in order book.

### Iceberg

```go
Visibility: Iceberg
IcebergQty: 10 * BTC_PRECISION / 100,  // 1 BTC visible
Qty:        100 * BTC_PRECISION / 100,  // 10 BTC total
```

- Only `IcebergQty` visible in book depth
- Full quantity matchable
- Replenishes visible qty as filled
- Prevents signaling large orders

**Example:**
- Iceberg: 10 BTC total, 1 BTC visible
- Market taker buys 1 BTC → fills visible portion
- Order refreshes: still 1 BTC visible (9 BTC hidden remaining)

### Hidden

```go
Visibility: Hidden
```

- Zero quantity visible in book
- Still matchable at price level
- Prevents front-running
- Still has time priority at price level

## Self-Trade Prevention

```go
if resting.ClientID == order.ClientID {
    continue  // Skip to next order
}
```

Orders from same client never match each other. Prevents:
- Wash trading (artificial volume)
- Accidental self-hedging
- Fee gaming

**Example:**
- Market maker (Client 123): bid @ \$50,000
- Same market maker (Client 123): ask @ \$50,010
- Taker buy crosses spread → matches ask only
- Maker's own bid skipped

## Matching Examples

### Example 1: Simple Fill

**Book state:**
```
Asks:
  $50,010: 5 BTC
  $50,020: 10 BTC

Bids:
  $49,990: 8 BTC
  $49,980: 12 BTC
```

**Incoming:** Market buy 7 BTC

**Result:**
- Match 5 BTC @ \$50,010 (exhaust first ask)
- Match 2 BTC @ \$50,020 (partial fill second ask)
- Order filled: 7 BTC
- Executions: 2
- Average fill price: \$50,012.86

### Example 2: Limit Order Resting

**Book state:**
```
Asks:
  $50,010: 5 BTC

Bids:
  $49,990: 8 BTC
```

**Incoming:** Limit buy 10 BTC @ \$50,000

**Result:**
- No match (buy price \$50,000 < ask price \$50,010)
- Order rests in bid book:
```
Bids:
  $50,000: 10 BTC [NEW]
  $49,990: 8 BTC
```

### Example 3: Price Improvement

**Book state:**
```
Asks:
  $50,000: 3 BTC (Order A)
  $50,010: 5 BTC (Order B)
```

**Incoming:** Limit buy 10 BTC @ \$51,000

**Result:**
- Match 3 BTC @ \$50,000 (maker price, not \$51,000)
- Match 5 BTC @ \$50,010
- Remaining 2 BTC rests @ \$51,000
- Taker saved: $(51,000 - 50,003.75) × 8 = $7,980

### Example 4: FIFO Within Level

**Book state:**
```
Asks @ $50,000:
  Order 1: 10 BTC (timestamp: 100ms)
  Order 2: 15 BTC (timestamp: 150ms)
  Order 3: 20 BTC (timestamp: 200ms)
```

**Incoming:** Market buy 25 BTC

**Result:**
- Fill Order 1: 10 BTC (oldest)
- Fill Order 2: 15 BTC (next oldest)
- Order 3: Still 20 BTC unfilled (not reached)

### Example 5: Iceberg Replenishment

**Book state:**
```
Bids @ $50,000:
  Order 1: 100 BTC total, 10 BTC visible (iceberg)
```

**Incoming:** Market sell 12 BTC

**Result:**
- Match 10 BTC @ \$50,000 (visible portion)
- Iceberg refreshes: 10 BTC visible again (90 BTC hidden)
- Continue matching 2 BTC from refreshed visible
- Final: 88 BTC hidden, 8 BTC visible

**After:**
```
Bids @ $50,000:
  Order 1: 88 BTC total, 8 BTC visible
```

## Performance Characteristics

### Time Complexity

| Operation | Complexity |
|-----------|-----------|
| Get best bid/ask | O(1) |
| Place limit order | O(1) |
| Cancel order | O(1) |
| Match market order | O(n) executions |
| Match limit order | O(n) executions |

### Space Complexity

- Orders: O(n) where n = active orders
- Limits: O(m) where m = distinct price levels
- Indexes: 2 × O(n) for order ID and price lookups

### Object Pooling

```go
var orderPool = sync.Pool{
    New: func() interface{} { return &Order{} },
}

func getOrder() *Order {
    return orderPool.Get().(*Order)
}

func putOrder(o *Order) {
    o.reset()
    orderPool.Put(o)
}
```

Reduces GC pressure in high-frequency matching (1M+ orders/second).

## Matching Engine Variants

### This Implementation

- Price-time-visibility priority
- Linked lists for FIFO
- Object pooling
- Self-trade prevention
- Iceberg order support

### Alternative Matching Models

**Pro-Rata Allocation:**
- Used by CME Globex, Euronext for liquid futures contracts
- Distributes fills proportionally by resting order size at each level
- Rewards larger quotes; queue position (time priority) used only as tiebreaker

```go
ex.Matcher = exchange.NewProRataMatcher()
```

Set before any orders are placed. Fills are distributed proportionally across all
resting orders at the best price; remainder after integer division is assigned in
arrival order (FIFO tiebreaker) until exhausted.

**Time-Weighted:**
- Orders get priority based on time at level
- Longer wait = higher priority
- Discourages order modification

**Size-Priority:**
- Larger orders match first
- Used in some dark pools
- Encourages liquidity provision

### Extensibility

**Adding custom matching:**

```go
type CustomMatcher struct {
    priority PriorityFunc
}

type PriorityFunc func(a, b *Order) bool  // true if a has priority

func (m *CustomMatcher) Match(...) *MatchResult {
    // Sort resting orders by custom priority
    // Match in priority order
}
```

### Circuit Breakers

Two injection points wrap the matching engine:

- **`CircuitBreaker`** (pre-trade): called before each match; can `CBReject` or `CBHalt` a symbol
- **`HaltEvaluator`** (post-match): called after match with actual executions; halts on fill price

Wire by replacing `ex.Matcher` with a `CircuitBreakerMatcher`:

```go
// Symmetric ±5% band around last traded price
cb := &exchange.PercentBandCircuitBreaker{BandBps: 500}
ex.Matcher = exchange.NewCircuitBreakerMatcher(
    exchange.NewDefaultMatcher(),
    cb,
    ex.Books,
    ex.Clock,
)

// Tiered: ±5% → reject; ±10% → 15-minute halt
tiered := exchange.NewTieredCircuitBreaker([]exchange.BreakerTier{
    {ThresholdBps: 500,  Action: exchange.CBReject},
    {ThresholdBps: 1000, Action: exchange.CBHalt, HaltDuration: 15 * time.Minute},
})
tiered.SetRefPrice("BTCUSD", 100_000*exchange.USD_PRECISION)
ex.Matcher = exchange.NewCircuitBreakerMatcher(exchange.NewDefaultMatcher(), tiered, ex.Books, ex.Clock)
```

Composite breakers chain multiple checks:
```go
&exchange.CompositeCircuitBreaker{Breakers: []exchange.CircuitBreaker{cb1, cb2}}
```

**Available implementations:** `PercentBandCircuitBreaker` (symmetric ±N%), `AsymmetricBandCircuitBreaker` (independent up/down bands), `TieredCircuitBreaker` (escalating actions), `CompositeCircuitBreaker` (combine multiple).

**Supported order types (extensible):**

This implementation supports:
- Market and limit orders
- Time-in-force: GTC, IOC, FOK
- Visibility: Normal, iceberg, hidden

Can be extended with:
- Stop orders (stop-loss, stop-limit)
- Trailing stops
- Post-only orders (reject if crosses spread)
- Reduce-only orders (position management)
- Auction matching (opening/closing)
- Conditional orders (OCO, etc.)

Extensions made via MatchingEngine interface or custom Order validation.

## Validation

Orders validated before matching:

```go
func validateOrder(order *Order, inst Instrument) RejectReason {
    if order.Qty < inst.MinOrderSize() {
        return RejectMinSize
    }
    if order.Type == Limit && order.Price % inst.TickSize() != 0 {
        return RejectInvalidPrice
    }
    if order.Qty % inst.BasePrecision() != 0 {
        return RejectInvalidQty
    }
    return RejectNone
}
```

## Next Steps

- [Exchange Architecture](exchange-architecture.md) - How matching integrates with exchange
- [Instruments](instruments.md) - Precision and validation rules
- [Positions and Margin](positions-and-margin.md) - Post-execution settlement
