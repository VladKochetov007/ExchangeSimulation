# Actor Strategy Examples

Actors are not limited to market making. Any trading strategy can be implemented by extending `BaseActor`.

## Market Maker (Provided)

**File:** `marketmaker.go`

**Strategy:** Provide liquidity on both sides of the book
- Places limit orders (becomes maker)
- Profits from bid-ask spread
- Continuously quotes both bid and ask

```go
mm := actor.NewMarketMaker(id, gateway, actor.MarketMakerConfig{
    Symbol:        "BTCUSD",
    SpreadBps:     20,
    QuoteQty:      100000000,
    RefreshOnFill: false,
})
```

## Aggressive Taker Example

**Strategy:** Take liquidity when price is favorable
- Monitors orderbook snapshots
- Submits market orders (becomes taker)
- Reacts to price movements

```go
type AggressiveTaker struct {
    *actor.BaseActor
    symbol      string
    targetPrice int64
    orderSize   int64
}

func NewAggressiveTaker(id uint64, gateway *exchange.ClientGateway,
                        symbol string, targetPrice, orderSize int64) *AggressiveTaker {
    return &AggressiveTaker{
        BaseActor:   actor.NewBaseActor(id, gateway),
        symbol:      symbol,
        targetPrice: targetPrice,
        orderSize:   orderSize,
    }
}

func (a *AggressiveTaker) Start(ctx context.Context) error {
    a.Subscribe(a.symbol)
    go a.eventLoop(ctx)
    return a.BaseActor.Start(ctx)
}

func (a *AggressiveTaker) eventLoop(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        case event := <-a.EventChannel():
            a.OnEvent(event)
        }
    }
}

func (a *AggressiveTaker) OnEvent(event *actor.Event) {
    switch event.Type {
    case actor.EventBookSnapshot:
        snap := event.Data.(actor.BookSnapshotEvent)
        if snap.Symbol == a.symbol && len(snap.Snapshot.Asks) > 0 {
            bestAsk := snap.Snapshot.Asks[0].Price
            if bestAsk <= a.targetPrice {
                // Favorable price - take liquidity
                a.SubmitOrder(a.symbol, exchange.Buy, exchange.Market, 0, a.orderSize)
            }
        }
    case actor.EventOrderFilled:
        fill := event.Data.(actor.OrderFillEvent)
        // React to fill - adjust position, update target, etc.
    }
}
```

## Momentum Trader Example

**Strategy:** Trade based on recent price movements
- Tracks trade flow
- Takes position when momentum detected
- Mixed maker/taker based on conditions

```go
type MomentumTrader struct {
    *actor.BaseActor
    symbol       string
    recentTrades []int64
    position     int64
    maxPosition  int64
}

func (a *MomentumTrader) OnEvent(event *actor.Event) {
    switch event.Type {
    case actor.EventTrade:
        trade := event.Data.(actor.TradeEvent)
        a.recentTrades = append(a.recentTrades, trade.Trade.Price)
        if len(a.recentTrades) > 10 {
            a.recentTrades = a.recentTrades[1:]
        }

        momentum := a.calculateMomentum()
        if momentum > 0 && a.position < a.maxPosition {
            // Upward momentum - go long (could be maker or taker)
            a.SubmitOrder(a.symbol, exchange.Buy, exchange.LimitOrder,
                         trade.Trade.Price, 10000000)
        } else if momentum < 0 && a.position > -a.maxPosition {
            // Downward momentum - go short
            a.SubmitOrder(a.symbol, exchange.Sell, exchange.LimitOrder,
                         trade.Trade.Price, 10000000)
        }

    case actor.EventOrderFilled:
        fill := event.Data.(actor.OrderFillEvent)
        if fill.Side == exchange.Buy {
            a.position += fill.Qty
        } else {
            a.position -= fill.Qty
        }
    }
}

func (a *MomentumTrader) calculateMomentum() int64 {
    if len(a.recentTrades) < 2 {
        return 0
    }
    return a.recentTrades[len(a.recentTrades)-1] - a.recentTrades[0]
}
```

## Arbitrage Actor Example

**Strategy:** Exploit price differences between instruments
- Monitors multiple orderbooks
- Takes simultaneously on both sides
- Pure taker strategy

```go
type ArbitrageActor struct {
    *actor.BaseActor
    symbol1 string
    symbol2 string
    book1   map[string]*actor.BookSnapshotEvent
    book2   map[string]*actor.BookSnapshotEvent
    minSpread int64
}

func (a *ArbitrageActor) OnEvent(event *actor.Event) {
    switch event.Type {
    case actor.EventBookSnapshot:
        snap := event.Data.(actor.BookSnapshotEvent)
        if snap.Symbol == a.symbol1 {
            a.book1[snap.Symbol] = &snap
        } else if snap.Symbol == a.symbol2 {
            a.book2[snap.Symbol] = &snap
        }

        a.checkArbitrage()

    case actor.EventOrderFilled:
        // Track fills to ensure both legs complete
    }
}

func (a *ArbitrageActor) checkArbitrage() {
    if a.book1[a.symbol1] == nil || a.book2[a.symbol2] == nil {
        return
    }

    book1Snap := a.book1[a.symbol1].Snapshot
    book2Snap := a.book2[a.symbol2].Snapshot

    if len(book1Snap.Asks) > 0 && len(book2Snap.Bids) > 0 {
        spread := book2Snap.Bids[0].Price - book1Snap.Asks[0].Price
        if spread > a.minSpread {
            // Profitable arbitrage: buy symbol1, sell symbol2
            qty := min(book1Snap.Asks[0].Qty, book2Snap.Bids[0].Qty)
            a.SubmitOrder(a.symbol1, exchange.Buy, exchange.Market, 0, qty)
            a.SubmitOrder(a.symbol2, exchange.Sell, exchange.Market, 0, qty)
        }
    }
}
```

## TWAP Execution Example

**Strategy:** Time-Weighted Average Price execution
- Splits large order into smaller chunks
- Places orders at intervals
- Minimizes market impact (mostly taker)

```go
type TWAPExecutor struct {
    *actor.BaseActor
    symbol       string
    totalQty     int64
    remainingQty int64
    interval     time.Duration
    chunkSize    int64
    side         exchange.Side
}

func (a *TWAPExecutor) Start(ctx context.Context) error {
    a.Subscribe(a.symbol)
    go a.executionLoop(ctx)
    return a.BaseActor.Start(ctx)
}

func (a *TWAPExecutor) executionLoop(ctx context.Context) {
    ticker := time.NewTicker(a.interval)
    defer ticker.Stop()

    for a.remainingQty > 0 {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            qty := min(a.chunkSize, a.remainingQty)
            // Use market order for immediate execution (taker)
            a.SubmitOrder(a.symbol, a.side, exchange.Market, 0, qty)
            a.remainingQty -= qty
        }
    }
}

func (a *TWAPExecutor) OnEvent(event *actor.Event) {
    switch event.Type {
    case actor.EventOrderFilled:
        // Track execution, log slippage
    }
}
```

## Mixed Strategy Example

**Strategy:** Adaptive - switch between making and taking
- Provides liquidity when spread is wide (maker)
- Takes liquidity when opportunity arises (taker)
- Adapts to market conditions

```go
type AdaptiveTrader struct {
    *actor.BaseActor
    symbol          string
    minSpreadBps    int64
    currentMidPrice int64
}

func (a *AdaptiveTrader) OnEvent(event *actor.Event) {
    switch event.Type {
    case actor.EventBookSnapshot:
        snap := event.Data.(actor.BookSnapshotEvent)
        if len(snap.Snapshot.Bids) == 0 || len(snap.Snapshot.Asks) == 0 {
            return
        }

        bestBid := snap.Snapshot.Bids[0].Price
        bestAsk := snap.Snapshot.Asks[0].Price
        a.currentMidPrice = (bestBid + bestAsk) / 2

        spreadBps := ((bestAsk - bestBid) * 10000) / a.currentMidPrice

        if spreadBps > a.minSpreadBps {
            // Wide spread - provide liquidity (maker)
            bidPrice := a.currentMidPrice - (a.currentMidPrice * 5 / 10000)
            askPrice := a.currentMidPrice + (a.currentMidPrice * 5 / 10000)
            a.SubmitOrder(a.symbol, exchange.Buy, exchange.LimitOrder, bidPrice, 10000000)
            a.SubmitOrder(a.symbol, exchange.Sell, exchange.LimitOrder, askPrice, 10000000)
        } else {
            // Tight spread - wait for better opportunity
        }

    case actor.EventTrade:
        trade := event.Data.(actor.TradeEvent)
        deviation := abs(trade.Trade.Price - a.currentMidPrice)
        if deviation > a.currentMidPrice/100 {
            // Price moved significantly - take liquidity (taker)
            if trade.Trade.Price < a.currentMidPrice {
                // Price dropped - buy
                a.SubmitOrder(a.symbol, exchange.Buy, exchange.Market, 0, 10000000)
            } else {
                // Price increased - sell
                a.SubmitOrder(a.symbol, exchange.Sell, exchange.Market, 0, 10000000)
            }
        }
    }
}

func abs(x int64) int64 {
    if x < 0 {
        return -x
    }
    return x
}

func min(a, b int64) int64 {
    if a < b {
        return a
    }
    return b
}
```

## Key Points

1. **All actors extend BaseActor** - Inherits event handling, order submission, market data
2. **Makers vs Takers** - Determined by order behavior, not actor type
3. **Event-driven** - React to OrderAccepted, OrderFilled, Trade, BookSnapshot, etc.
4. **State management** - Track positions, pending orders, market conditions
5. **Extensible** - Add any strategy by implementing `OnEvent`

## Using Custom Actors

```go
// In cmd/sim/main.go or your simulation
runner := simulation.NewRunner(config)
ex := runner.Exchange()

// Add instruments
btcusd := exchange.NewSpotInstrument("BTCUSD", "BTC", "USD", 100000000, 1000000)
ex.AddInstrument(btcusd)

// Connect clients and create actors
balances := map[string]int64{"BTC": 1000000000, "USD": 1000000000000}
feePlan := &exchange.PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true}

// Market maker (provides liquidity)
gateway1 := ex.ConnectClient(1, balances, feePlan)
mm := actor.NewMarketMaker(1, gateway1, actor.MarketMakerConfig{
    Symbol: "BTCUSD", SpreadBps: 20, QuoteQty: 100000000,
})
runner.AddActor(mm)

// Aggressive taker (takes liquidity)
gateway2 := ex.ConnectClient(2, balances, feePlan)
taker := NewAggressiveTaker(2, gateway2, "BTCUSD", 5000000000000, 50000000)
runner.AddActor(taker)

// Mixed strategy
gateway3 := ex.ConnectClient(3, balances, feePlan)
adaptive := NewAdaptiveTrader(3, gateway3, "BTCUSD", 10)
runner.AddActor(adaptive)

runner.Run(context.Background())
```

## Handling Rejections (Critical!)

**All actors must handle rejection events** - they're a normal part of trading:

```go
type RobustActor struct {
    *actor.BaseActor
    symbol          string
    pendingOrders   map[uint64]*OrderInfo  // RequestID -> OrderInfo
    retryQueue      []*RetryOrder
}

type OrderInfo struct {
    requestID uint64
    symbol    string
    side      exchange.Side
    price     int64
    qty       int64
    retries   int
}

func (a *RobustActor) OnEvent(event *actor.Event) {
    switch event.Type {
    case actor.EventOrderAccepted:
        accepted := event.Data.(actor.OrderAcceptedEvent)
        // Success - track the order
        if info, ok := a.pendingOrders[accepted.RequestID]; ok {
            info.orderID = accepted.OrderID
            a.activeOrders[accepted.OrderID] = info
            delete(a.pendingOrders, accepted.RequestID)
        }

    case actor.EventOrderRejected:
        rejection := event.Data.(actor.OrderRejectedEvent)

        switch rejection.Reason {
        case exchange.RejectInsufficientBalance:
            // Reduce position size and retry
            if info, ok := a.pendingOrders[rejection.RequestID]; ok && info.retries < 3 {
                newQty := info.qty / 2  // Halve the size
                if newQty >= minQty {
                    a.SubmitOrder(info.symbol, info.side, exchange.LimitOrder, info.price, newQty)
                    info.qty = newQty
                    info.retries++
                } else {
                    // Too small, give up
                    delete(a.pendingOrders, rejection.RequestID)
                }
            }

        case exchange.RejectInvalidPrice:
            // Price not aligned to tick size - round and retry
            if info, ok := a.pendingOrders[rejection.RequestID]; ok {
                roundedPrice := (info.price / tickSize) * tickSize
                a.SubmitOrder(info.symbol, info.side, exchange.LimitOrder, roundedPrice, info.qty)
                info.price = roundedPrice
            }

        case exchange.RejectSelfTrade:
            // Would cross our own order - wait and retry with slight price adjustment
            if info, ok := a.pendingOrders[rejection.RequestID]; ok && info.retries < 3 {
                time.Sleep(100 * time.Millisecond)
                adjustedPrice := info.price
                if info.side == exchange.Buy {
                    adjustedPrice -= tickSize  // Lower bid
                } else {
                    adjustedPrice += tickSize  // Raise ask
                }
                a.SubmitOrder(info.symbol, info.side, exchange.LimitOrder, adjustedPrice, info.qty)
                info.price = adjustedPrice
                info.retries++
            }

        case exchange.RejectUnknownInstrument:
            // Symbol doesn't exist - fatal error
            delete(a.pendingOrders, rejection.RequestID)

        default:
            // Unknown rejection - log and remove
            delete(a.pendingOrders, rejection.RequestID)
        }

    case actor.EventOrderCancelRejected:
        cancelRejection := event.Data.(actor.OrderCancelRejectedEvent)

        switch cancelRejection.Reason {
        case exchange.RejectOrderNotFound:
            // Order already filled or cancelled - update tracking
            delete(a.activeOrders, cancelRejection.OrderID)

        case exchange.RejectOrderAlreadyFilled:
            // Order filled before cancel arrived
            // We should receive EventOrderFilled shortly
            // Do nothing - wait for fill event

        case exchange.RejectOrderNotOwned:
            // Logic error - should never happen
            // This indicates a bug in our order tracking
        }
    }
}
```

### Retry Strategy Pattern

```go
type RetryStrategy struct {
    maxRetries    int
    backoffMs     int
    reduceOnRetry bool
}

func (s *RetryStrategy) ShouldRetry(info *OrderInfo, reason exchange.RejectReason) bool {
    if info.retries >= s.maxRetries {
        return false
    }

    switch reason {
    case exchange.RejectInsufficientBalance,
         exchange.RejectInvalidPrice,
         exchange.RejectSelfTrade:
        return true  // Retryable

    case exchange.RejectUnknownClient,
         exchange.RejectUnknownInstrument:
        return false  // Fatal, don't retry

    default:
        return info.retries < 1  // Retry once for unknown errors
    }
}

func (s *RetryStrategy) AdjustOrder(info *OrderInfo, reason exchange.RejectReason) {
    time.Sleep(time.Duration(s.backoffMs * (info.retries + 1)) * time.Millisecond)

    switch reason {
    case exchange.RejectInsufficientBalance:
        if s.reduceOnRetry {
            info.qty = info.qty * 3 / 4  // Reduce by 25%
        }

    case exchange.RejectInvalidPrice:
        info.price = (info.price / tickSize) * tickSize

    case exchange.RejectSelfTrade:
        if info.side == exchange.Buy {
            info.price -= tickSize
        } else {
            info.price += tickSize
        }
    }

    info.retries++
}
```

## Key Takeaways

1. **Rejections are normal** - Not errors, just validation feedback
2. **Always handle EventOrderRejected** - Ignore at your own risk
3. **Cancel rejections happen** - Order might fill before cancel arrives
4. **Retry with adjustment** - Don't blindly retry same order
5. **Track request IDs** - Match rejections to original orders
6. **Have retry limits** - Prevent infinite loops
7. **Some rejections are fatal** - Unknown client/instrument means stop

The actor framework is **strategy-agnostic** - implement any trading logic you need!
