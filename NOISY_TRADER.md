# NoisyTrader Actor

## Overview

The `NoisyTrader` actor simulates a retail trader who places random limit orders around the mid-price. This adds realistic market noise and liquidity provision at various price levels.

## Behavior

The actor:
1. **Subscribes** to market data to track best bid/ask and compute mid-price
2. **Places random orders** at configurable intervals
3. **Varies price** within +/- configurable basis points from mid-price
4. **Randomizes quantity** between min and max limits
5. **Cancels stale orders** after a configurable lifetime
6. **Limits active orders** to prevent overwhelming the book

## Configuration

```go
type NoisyTraderConfig struct {
    Symbol          string        // Trading symbol
    Interval        time.Duration // How often to place new orders
    PriceRangeBps   int64         // +/- range from mid (100 = 1%)
    MinQty          int64         // Minimum order quantity
    MaxQty          int64         // Maximum order quantity
    MaxActiveOrders int           // Maximum concurrent orders
    OrderLifetime   time.Duration // Auto-cancel after this time
}
```

## Example Usage

```go
// Create noisy traders with 1% price range around mid
for i := uint64(400); i <= 403; i++ {
    gateway := ex.ConnectClient(i, initialBalances, feePlan)
    noisy := actor.NewNoisyTrader(i, gateway, actor.NoisyTraderConfig{
        Symbol:          "BTCUSD",
        Interval:        1500 * time.Millisecond, // Place order every 1.5s
        PriceRangeBps:   100,                     // +/- 1% from mid
        MinQty:          exchange.BTCAmount(0.1), // 0.1 BTC minimum
        MaxQty:          exchange.BTCAmount(1.0), // 1.0 BTC maximum
        MaxActiveOrders: 3,                       // Max 3 orders at once
        OrderLifetime:   5 * time.Second,         // Cancel after 5s
    })
    runner.AddActor(noisy)
}
```

## Algorithm

### Order Placement

For each order:
1. Random side selection (50% buy, 50% sell)
2. Random price offset: `price = mid + (mid * randomBps / 10000)`
   - Where `randomBps` ∈ [-PriceRangeBps, +PriceRangeBps]
3. Random quantity: `qty = MinQty + random(MaxQty - MinQty)`

### Mid-Price Tracking

The actor maintains:
- `bestBid`: Highest bid price from snapshots/deltas
- `bestAsk`: Lowest ask price from snapshots/deltas
- `midPrice = (bestBid + bestAsk) / 2`

### Order Lifecycle

- **Initial delay**: Random delay up to `Interval` to stagger actors
- **Placement**: New order placed every `Interval` if `activeOrders < MaxActiveOrders`
- **Tracking**: Orders tracked in `activeOrders` map with placement timestamp
- **Cleanup**: Every `OrderLifetime / 2`, stale orders are cancelled

## Impact on Simulation

Adding noisy traders:
- ✅ Increases orderbook depth around mid-price
- ✅ Creates price discovery noise
- ✅ Generates more realistic spread dynamics
- ✅ Provides liquidity at multiple price levels
- ✅ Simulates retail order flow

## Tuning Parameters

### Conservative (tight spread, small size)
```go
PriceRangeBps:   50,  // +/- 0.5%
MinQty:          0.05 BTC
MaxQty:          0.2 BTC
MaxActiveOrders: 2
OrderLifetime:   3 * time.Second
```

### Aggressive (wide spread, large size)
```go
PriceRangeBps:   300,  // +/- 3%
MinQty:          0.5 BTC
MaxQty:          5.0 BTC
MaxActiveOrders: 5
OrderLifetime:   10 * time.Second
```

### High Frequency
```go
Interval:        500 * time.Millisecond
PriceRangeBps:   20,   // +/- 0.2%
MaxActiveOrders: 10
OrderLifetime:   2 * time.Second
```

## Differences from Other Actors

| Actor | Purpose | Order Placement |
|-------|---------|----------------|
| **FirstLP** | Bootstrap liquidity | Fixed spread from reference price |
| **DelayedMaker** | Competitive makers | Fixed ladder around base price |
| **RandomizedTaker** | Market orders | Executes against book |
| **NoisyTrader** | Retail flow | Random around mid-price |

## Visualization

The noisy traders' orders appear as:
- Green dots (buys) and red dots (sells) when executed
- Spread around the mid-price in the orderbook depth
- More dynamic best bid/ask movement

To visualize impact:
```bash
./sim
python3 visualize_orderbook.py \
    --trades output/trades.csv \
    --orderbook output/book_observed.csv \
    --symbol BTCUSD \
    --bin-width 0.5 \
    --output output/orderbook_with_noise.png
```

## Testing

The actor can be tested with:
```bash
go test ./actor -v -run TestNoisyTrader
```

Key test cases:
- Order placement within price range
- Quantity within min/max bounds
- Active order limit enforcement
- Stale order cancellation
- Mid-price tracking accuracy
