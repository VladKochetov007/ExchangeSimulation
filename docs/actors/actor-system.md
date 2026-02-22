# Actor System

Event-driven actor framework for implementing trading strategies.

## Actor Interface

```go
type Actor interface {
    OnEvent(event *Event)
    Start(ctx context.Context) error
    Stop() error
    ID() uint64
    Gateway() *exchange.ClientGateway
}
```

## BaseActor

Embedding BaseActor provides core functionality:

```go
type BaseActor struct {
    id             uint64
    gateway        *exchange.ClientGateway
    eventCh        chan *Event  // Buffer: 1000
    stopCh         chan struct{}
    running        atomic.Bool
    activeOrders   map[uint64]*OrderInfo
    requestToOrder map[uint64]uint64
    tickerFactory  TickerFactory
}
```

### Lifecycle

```go
actor := NewMyActor(id, gateway, config)
actor.SetTickerFactory(tickerFactory)  // Optional: for simulation
actor.Start(ctx)                        // Start event loop
// ... trading happens ...
actor.Stop()                            // Graceful shutdown
```

## Event Loop

```go
func (a *BaseActor) run(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        case <-a.stopCh:
            return
        case resp := <-a.gateway.ResponseCh:
            a.handleResponse(resp)
        case md := <-a.gateway.MarketData:
            a.handleMarketData(md)
        case event := <-a.eventCh:
            a.OnEvent(event)  // Subclass handles
        }
    }
}
```

**Three event sources:**
1. **ResponseCh**: Order responses (accepted, rejected, filled)
2. **MarketData**: Book snapshots, trades, funding updates
3. **EventCh**: Custom events (timers, strategy signals)

## Event Types

### Order Events

```go
const (
    EventOrderAccepted      // Order placed successfully
    EventOrderRejected      // Order rejected (insufficient balance, etc.)
    EventOrderPartialFill   // Order partially filled
    EventOrderFilled        // Order completely filled
    EventOrderCancelled     // Order cancelled
    EventOrderCancelRejected  // Cancel request failed
)
```

### Market Data Events

```go
const (
    EventTrade          // Trade occurred
    EventBookDelta      // Order book changed
    EventBookSnapshot   // Full book state
    EventFundingUpdate  // Funding rate updated
    EventOpenInterest   // Open interest changed
)
```

## Submitting Orders

### Simple Limit Order

```go
err := actor.SubmitOrder(&OrderRequest{
    Symbol:      "BTC-PERP",
    Side:        exchange.Buy,
    Price:       50000 * exchange.USD_PRECISION,
    Qty:         1 * exchange.BTC_PRECISION,
    TimeInForce: exchange.GTC,
})
```

### Market Order

```go
err := actor.SubmitOrder(&OrderRequest{
    Symbol:      "BTC-PERP",
    Side:        exchange.Sell,
    Type:        exchange.Market,
    Qty:         1 * exchange.BTC_PRECISION,
    TimeInForce: exchange.IOC,
})
```

### Iceberg Order

```go
err := actor.SubmitOrderFull(&OrderRequest{
    Symbol:      "BTC-PERP",
    Side:        exchange.Buy,
    Price:       50000 * exchange.USD_PRECISION,
    Qty:         10 * exchange.BTC_PRECISION,     // Total
    IcebergQty:  1 * exchange.BTC_PRECISION,      // Visible
    Visibility:  exchange.Iceberg,
    TimeInForce: exchange.GTC,
})
```

## Handling Events

```go
func (a *MyActor) OnEvent(event *Event) {
    switch event.Type {
    case EventOrderAccepted:
        e := event.Data.(*OrderAcceptedEvent)
        a.handleOrderAccepted(e.OrderID, e.RequestID)

    case EventOrderFilled:
        e := event.Data.(*OrderFillEvent)
        a.handleFill(e)

    case EventTrade:
        e := event.Data.(*TradeEvent)
        a.handleTrade(e.Trade)

    case EventBookSnapshot:
        e := event.Data.(*BookSnapshotEvent)
        a.handleBookSnapshot(e)
    }
}
```

## Order Tracking

BaseActor tracks active orders automatically:

```go
type OrderInfo struct {
    OrderID   uint64
    Symbol    string
    Side      Side
    Price     int64
    Qty       int64
    FilledQty int64
}
```

**Access:**
```go
orderInfo := actor.activeOrders[orderID]
if orderInfo != nil {
    remaining := orderInfo.Qty - orderInfo.FilledQty
}
```

## Position Management (OMS)

### NettingOMS

Single net position per instrument:

```go
oms := actor.NewNettingOMS()

oms.OnFill("BTC-PERP", OrderFillEvent{
    Side:  Buy,
    Qty:   10 * BTC_PRECISION,
    Price: 50000 * USD_PRECISION,
}, BTC_PRECISION)

pos := oms.GetNetPosition("BTC-PERP")
// pos = 10 BTC (long)
```

**Averaging:**
```go
oms.OnFill("BTC-PERP", fillEvent1)  // Buy 10 @ 50,000
oms.OnFill("BTC-PERP", fillEvent2)  // Buy 5 @ 51,000

pos := oms.GetPosition("BTC-PERP")
// Entry price: weighted average
```

### HedgingOMS

Multiple positions per side:

```go
oms := actor.NewHedgingOMS()

oms.OnFill("BTC-PERP", fillEvent1)  // Buy 10 @ 50,000
oms.OnFill("BTC-PERP", fillEvent2)  // Buy 5 @ 51,000

positions := oms.GetPositions("BTC-PERP")
// Two separate long positions with distinct cost basis
```

## Example Actor: Simple Market Maker

```go
type SimpleMMConfig struct {
    Symbol     string
    SpreadBps  int64
    QuoteSize  int64
}

type SimpleMarketMaker struct {
    *actor.BaseActor
    config      SimpleMMConfig
    instrument  exchange.Instrument
    activeBid   uint64
    activAsk    uint64
    lastMid     int64
}

func (mm *SimpleMarketMaker) Start(ctx context.Context) error {
    mm.BaseActor.Start(ctx)

    ticker := mm.TickerFactory().NewTicker(1 * time.Second)
    go mm.requoteLoop(ctx, ticker)

    return nil
}

func (mm *SimpleMarketMaker) requoteLoop(ctx context.Context, ticker Ticker) {
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C():
            mm.requote()
        }
    }
}

func (mm *SimpleMarketMaker) requote() {
    mid := mm.getMidPrice()  // From book snapshot or last trade

    spreadTicks := (mid * mm.config.SpreadBps) / 10000
    bidPrice := mid - spreadTicks/2
    askPrice := mid + spreadTicks/2

    if mm.activeBid != 0 {
        mm.CancelOrder(mm.activeBid)
    }
    if mm.activeAsk != 0 {
        mm.CancelOrder(mm.activeAsk)
    }

    mm.SubmitOrder(&OrderRequest{
        Symbol: mm.config.Symbol,
        Side:   exchange.Buy,
        Price:  bidPrice,
        Qty:    mm.config.QuoteSize,
    })

    mm.SubmitOrder(&OrderRequest{
        Symbol: mm.config.Symbol,
        Side:   exchange.Sell,
        Price:  askPrice,
        Qty:    mm.config.QuoteSize,
    })
}

func (mm *SimpleMarketMaker) OnEvent(event *Event) {
    switch event.Type {
    case EventOrderAccepted:
        e := event.Data.(*OrderAcceptedEvent)
        // Track order ID

    case EventOrderFilled:
        e := event.Data.(*OrderFillEvent)
        // Update inventory, requote if needed

    case EventBookSnapshot:
        e := event.Data.(*BookSnapshotEvent)
        mm.lastMid = mm.calculateMid(e)
    }
}
```

## Ticker Abstraction

### Real-Time Ticker

```go
ticker := mm.TickerFactory().NewTicker(1 * time.Second)
defer ticker.Stop()

for {
    select {
    case <-ticker.C():
        mm.doWork()
    }
}
```

**Real-time:** Fires every 1 second of wall-clock time.

### Simulated Ticker

```go
// Same code, but TickerFactory = SimTickerFactory
ticker := mm.TickerFactory().NewTicker(1 * time.Second)

// Fires every 1 simulated second
// Advances when clock.Advance() called
```

**Simulation:** Fires based on simulated time, not wall-clock.

## Market Data Subscription

```go
err := actor.Subscribe("BTC-PERP", MDSnapshot | MDTrade | MDFunding)
```

**Message types:**
- `MDSnapshot`: Full order book snapshots
- `MDDelta`: Incremental book updates
- `MDTrade`: Trade notifications
- `MDFunding`: Funding rate updates
- `MDOpenInterest`: Open interest changes

## Example Actor: Momentum Trader

```go
type MomentumTrader struct {
    *actor.BaseActor
    symbol      string
    instrument  exchange.Instrument
    recentTrades []Trade
    position     int64
}

func (mt *MomentumTrader) OnEvent(event *Event) {
    switch event.Type {
    case EventTrade:
        e := event.Data.(*TradeEvent)
        mt.recentTrades = append(mt.recentTrades, e.Trade)

        if len(mt.recentTrades) > 20 {
            mt.recentTrades = mt.recentTrades[1:]
        }

        mt.checkMomentum()

    case EventOrderFilled:
        e := event.Data.(*OrderFillEvent)
        if e.Side == exchange.Buy {
            mt.position += e.Qty
        } else {
            mt.position -= e.Qty
        }
    }
}

func (mt *MomentumTrader) checkMomentum() {
    if len(mt.recentTrades) < 20 {
        return
    }

    buyVolume := int64(0)
    sellVolume := int64(0)

    for _, trade := range mt.recentTrades {
        if trade.Side == exchange.Buy {
            buyVolume += trade.Qty
        } else {
            sellVolume += trade.Qty
        }
    }

    if buyVolume > sellVolume*2 && mt.position <= 0 {
        // Strong buy momentum, go long
        mt.SubmitOrder(&OrderRequest{
            Symbol: mt.symbol,
            Side:   exchange.Buy,
            Type:   exchange.Market,
            Qty:    1 * exchange.BTC_PRECISION,
        })
    } else if sellVolume > buyVolume*2 && mt.position >= 0 {
        // Strong sell momentum, go short
        mt.SubmitOrder(&OrderRequest{
            Symbol: mt.symbol,
            Side:   exchange.Sell,
            Type:   exchange.Market,
            Qty:    1 * exchange.BTC_PRECISION,
        })
    }
}
```

## Thread Safety

**BaseActor handles:**
- Event serialization (one event at a time)
- Channel synchronization
- Atomic running flag

**Actor responsibilities:**
- Don't spawn goroutines that access mutable state
- Use channels for communication
- If multi-goroutine needed, use proper synchronization

**Safe:**
```go
func (a *MyActor) OnEvent(event *Event) {
    a.position += delta  // Safe: single-threaded event loop
}
```

**Unsafe:**
```go
func (a *MyActor) OnEvent(event *Event) {
    go func() {
        a.position += delta  // RACE: concurrent access
    }()
}
```

## Configuration Pattern

```go
type MyActorConfig struct {
    Symbol      string
    Parameter1  int64
    Parameter2  time.Duration
}

func NewMyActor(id uint64, gateway *exchange.ClientGateway, cfg MyActorConfig) *MyActor {
    return &MyActor{
        BaseActor: actor.NewBaseActor(id, gateway),
        config:    cfg,
    }
}
```

**Immutable config:** Pass config struct at construction, don't mutate after Start().

## Order Rejections

### Rejection Reasons

```go
const (
    RejectNone                  // No rejection
    RejectInsufficientBalance   // Not enough balance
    RejectInsufficientMargin    // Not enough margin (perp)
    RejectInvalidPrice          // Price doesn't meet tick size
    RejectInvalidQty            // Qty below minimum or invalid
    RejectMinSize               // Below minimum order size
    RejectMaxSize               // Above maximum order size
    RejectSelfTrade             // Would match own order
    RejectMarketClosed          // Market not accepting orders
    RejectInstrumentNotFound    // Symbol doesn't exist
    RejectDuplicateOrderID      // Order ID already exists
    RejectPostOnly              // Would take liquidity (post-only order)
)
```

### Handling Rejections

```go
func (a *MyActor) OnEvent(event *Event) {
    switch event.Type {
    case EventOrderRejected:
        e := event.Data.(*OrderRejectedEvent)
        a.handleRejection(e)
    }
}

func (a *MyActor) handleRejection(e *OrderRejectedEvent) {
    switch e.Reason {
    case exchange.RejectInsufficientBalance:
        // Wait for balance or reduce order size
        a.retryWithSmallerSize(e.RequestID)

    case exchange.RejectInsufficientMargin:
        // Close some positions or add margin
        a.reducePositions()

    case exchange.RejectInvalidPrice:
        // Round to valid tick
        a.retryWithValidPrice(e.RequestID)

    case exchange.RejectInvalidQty:
        // Adjust to minimum size
        a.retryWithValidQty(e.RequestID)

    case exchange.RejectSelfTrade:
        // Cancel opposite side order first
        a.cancelConflictingOrders()

    case exchange.RejectMarketClosed:
        // Queue for market open
        a.queueForReopening(e.RequestID)

    default:
        // Log unexpected rejection
        log.Printf("Unexpected rejection: %v", e.Reason)
    }
}
```

### Retry Patterns

**Exponential backoff:**
```go
type RetryActor struct {
    *BaseActor
    retryQueue  []RetryItem
    backoffBase time.Duration
}

type RetryItem struct {
    Request    *OrderRequest
    Attempts   int
    NextRetry  int64  // Unix nano
}

func (a *RetryActor) OnEvent(event *Event) {
    switch event.Type {
    case EventOrderRejected:
        e := event.Data.(*OrderRejectedEvent)

        if a.shouldRetry(e.Reason) {
            a.enqueueRetry(e.RequestID)
        }
    }
}

func (a *RetryActor) shouldRetry(reason exchange.RejectReason) bool {
    switch reason {
    case exchange.RejectInsufficientBalance,
         exchange.RejectInsufficientMargin,
         exchange.RejectMarketClosed:
        return true
    default:
        return false
    }
}

func (a *RetryActor) enqueueRetry(requestID uint64) {
    req := a.pendingRequests[requestID]

    item := RetryItem{
        Request:  req,
        Attempts: 1,
        NextRetry: time.Now().UnixNano() + int64(a.backoffBase),
    }

    a.retryQueue = append(a.retryQueue, item)
}

func (a *RetryActor) processRetries() {
    now := time.Now().UnixNano()

    for i := 0; i < len(a.retryQueue); {
        item := a.retryQueue[i]

        if now >= item.NextRetry {
            if item.Attempts < 5 {  // Max 5 attempts
                a.SubmitOrder(item.Request)
                item.Attempts++
                item.NextRetry = now + int64(a.backoffBase) * (1 << item.Attempts)
                a.retryQueue[i] = item
                i++
            } else {
                // Give up after 5 attempts
                a.retryQueue = append(a.retryQueue[:i], a.retryQueue[i+1:]...)
            }
        } else {
            i++
        }
    }
}
```

## Custom Events

### Creating Custom Events

Extend the event system for custom actor logic:

```go
const (
    // Start custom events above standard events
    EventCustomBase = 1000

    EventCustomSignal        = EventCustomBase + 1
    EventCustomTimeout       = EventCustomBase + 2
    EventCustomIndicator     = EventCustomBase + 3
)

type CustomSignalEvent struct {
    Signal     string
    Strength   float64
    Timestamp  int64
}

type CustomTimeoutEvent struct {
    TimerID    string
    Payload    interface{}
}

type CustomIndicatorEvent struct {
    Name      string
    Value     float64
    Threshold float64
}
```

### Emitting Custom Events

```go
type SignalGeneratorActor struct {
    *BaseActor
    indicators map[string]float64
}

func (a *SignalGeneratorActor) calculateIndicator() {
    // Calculate some indicator
    macd := a.calculateMACD()

    // Emit custom event to self
    a.EventChannel() <- &Event{
        Type: EventCustomIndicator,
        Data: &CustomIndicatorEvent{
            Name:      "MACD",
            Value:     macd,
            Threshold: 0.5,
        },
    }
}

func (a *SignalGeneratorActor) OnEvent(event *Event) {
    switch event.Type {
    case EventCustomIndicator:
        e := event.Data.(*CustomIndicatorEvent)
        a.handleIndicator(e)

    case EventTrade:
        // Update indicators on new trade
        a.calculateIndicator()
    }
}
```

### Custom Exchange Events

Create custom exchange that emits domain-specific events:

```go
type CustomExchange struct {
    *exchange.Exchange
    eventBus chan CustomExchangeEvent
}

type CustomExchangeEvent struct {
    Type      string
    Symbol    string
    Data      interface{}
    Timestamp int64
}

// Emit custom event when circuit breaker triggers
func (ex *CustomExchange) CheckCircuitBreaker(symbol string, price int64) {
    prevPrice := ex.lastPrices[symbol]
    change := abs(price - prevPrice) * 10000 / prevPrice

    if change > ex.circuitBreakerThreshold {
        // Halt trading
        ex.haltTrading(symbol)

        // Emit custom event
        ex.eventBus <- CustomExchangeEvent{
            Type:   "circuit_breaker",
            Symbol: symbol,
            Data: map[string]interface{}{
                "old_price": prevPrice,
                "new_price": price,
                "change_bps": change,
            },
            Timestamp: ex.Clock.NowUnixNano(),
        }
    }
}
```

### Actor Receiving Custom Exchange Events

```go
type CircuitBreakerAwareActor struct {
    *BaseActor
    customExchange *CustomExchange
}

func (a *CircuitBreakerAwareActor) Start(ctx context.Context) error {
    a.BaseActor.Start(ctx)

    // Subscribe to custom exchange events
    go a.listenToCustomEvents(ctx)

    return nil
}

func (a *CircuitBreakerAwareActor) listenToCustomEvents(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        case event := <-a.customExchange.eventBus:
            a.handleCustomExchangeEvent(event)
        }
    }
}

func (a *CircuitBreakerAwareActor) handleCustomExchangeEvent(event CustomExchangeEvent) {
    switch event.Type {
    case "circuit_breaker":
        // Cancel all orders on this symbol
        data := event.Data.(map[string]interface{})
        symbol := event.Symbol

        a.cancelAllOrders(symbol)
        a.logCircuitBreaker(symbol, data)

    case "market_reopening":
        // Resume trading
        a.resumeTrading(event.Symbol)
    }
}
```

## Custom Matching Engine Events

Matching engine can emit custom events:

```go
type AuctionMatchingEngine struct {
    auctionEndTime int64
    collectedOrders []*Order
}

func (m *AuctionMatchingEngine) Match(
    bidBook, askBook *Book,
    order *Order,
) *MatchResult {
    if m.Clock.NowUnixNano() < m.auctionEndTime {
        // Collect orders during auction
        m.collectedOrders = append(m.collectedOrders, order)

        // Emit custom event
        return &MatchResult{
            CustomEvent: &CustomMatchEvent{
                Type: "auction_order_collected",
                Data: map[string]interface{}{
                    "orders_collected": len(m.collectedOrders),
                    "time_remaining": m.auctionEndTime - m.Clock.NowUnixNano(),
                },
            },
        }
    }

    // Auction ended, execute all orders
    results := m.executeAuction()

    return &MatchResult{
        Executions: results,
        CustomEvent: &CustomMatchEvent{
            Type: "auction_executed",
            Data: map[string]interface{}{
                "orders_matched": len(results),
                "clearing_price": m.calculateClearingPrice(),
            },
        },
    }
}
```

## Advanced Actor Patterns

### Multi-Strategy Actor

Combines multiple strategies with custom event routing:

```go
type MultiStrategyActor struct {
    *BaseActor
    strategies map[string]Strategy
    router     EventRouter
}

type Strategy interface {
    OnEvent(event *Event)
    ShouldHandle(event *Event) bool
}

type EventRouter struct {
    rules map[EventType][]string  // Event type -> strategy names
}

func (a *MultiStrategyActor) OnEvent(event *Event) {
    // Route to appropriate strategies
    strategyNames := a.router.rules[event.Type]

    for _, name := range strategyNames {
        strategy := a.strategies[name]
        if strategy.ShouldHandle(event) {
            strategy.OnEvent(event)
        }
    }
}

func (a *MultiStrategyActor) emitInternalEvent(eventType EventType, data interface{}) {
    a.EventChannel() <- &Event{
        Type: eventType,
        Data: data,
    }
}
```

### Event Aggregator

Collects events over time window:

```go
type EventAggregator struct {
    *BaseActor
    window     time.Duration
    events     []Event
    lastFlush  int64
}

func (a *EventAggregator) OnEvent(event *Event) {
    // Collect certain events
    if a.shouldAggregate(event) {
        a.events = append(a.events, *event)
    }

    // Flush if window elapsed
    now := time.Now().UnixNano()
    if now - a.lastFlush >= int64(a.window) {
        a.flushEvents()
        a.lastFlush = now
    }
}

func (a *EventAggregator) flushEvents() {
    if len(a.events) == 0 {
        return
    }

    // Emit aggregated event
    a.EventChannel() <- &Event{
        Type: EventCustomSignal,
        Data: &AggregatedSignal{
            EventCount: len(a.events),
            TimeWindow: a.window,
            Summary:    a.summarize(a.events),
        },
    }

    a.events = nil
}
```

### State Machine Actor

```go
type StateMachineActor struct {
    *BaseActor
    state        string
    transitions  map[string]map[EventType]string
    stateHandlers map[string]func(*Event)
}

func (a *StateMachineActor) OnEvent(event *Event) {
    // Check for state transition
    if nextState, ok := a.transitions[a.state][event.Type]; ok {
        a.transitionTo(nextState)
    }

    // Handle event in current state
    if handler, ok := a.stateHandlers[a.state]; ok {
        handler(event)
    }
}

func (a *StateMachineActor) transitionTo(newState string) {
    oldState := a.state
    a.state = newState

    // Emit state transition event
    a.EventChannel() <- &Event{
        Type: EventCustomSignal,
        Data: &StateTransitionEvent{
            From: oldState,
            To:   newState,
        },
    }
}
```

## Error Handling

```go
err := actor.SubmitOrder(req)
if err != nil {
    // Channel full or gateway closed
    log.Printf("Failed to submit order: %v", err)
    return
}

// Later, in OnEvent:
case EventOrderRejected:
    e := event.Data.(*OrderRejectedEvent)
    switch e.Reason {
    case exchange.RejectInsufficientBalance:
        // Handle insufficient balance
    case exchange.RejectInvalidPrice:
        // Handle invalid price
    }
```

## CompositeActor and SubActor

For simulations with multiple coordinated strategies sharing one exchange connection and
one pool of capital, use `CompositeActor` + `SubActor` instead of standalone actors.

### Why

A standalone actor owns its own gateway. If you run a spot MM and a funding arb actor
separately, they each hold independent USD balances and don't see each other's positions.
`CompositeActor` wraps N sub-actors behind a single gateway with a shared `SharedContext`.

### SubActor interface

```go
type SubActor interface {
    OnEvent(event *Event, ctx *SharedContext, submit OrderSubmitter)
    GetSymbols() []string
    GetID() uint64
}

type OrderSubmitter func(symbol string, side Side, orderType OrderType, price, qty int64) uint64
```

Events are routed by symbol: each sub-actor only receives events for symbols it returns
from `GetSymbols()`. Order-lifecycle events (accepted, filled, rejected, cancelled) are
broadcast to all sub-actors.

### SharedContext

Holds the group's shared balance state. Sub-actors call it to check/update balances
and track positions.

```go
ctx.GetQuoteBalance()               // total USD
ctx.GetAvailableQuote()             // USD minus reserved
ctx.CanReserveQuote(amount)         // pre-flight check (does NOT deduct)
ctx.GetBaseBalance("BTC")           // spot base balance

ctx.CanSubmitOrder(id, sym, side, qty, maxInv)  // guard before quoting
ctx.OnFill(id, sym, fill, prec, base)           // update position + balances
ctx.UpdateBalances(base, baseDelta, quoteDelta) // direct balance adjustment
```

### Wiring

```go
mm := NewPureMMSubActor(1, "BTC/USD", PureMMSubActorConfig{...})
taker := NewRandomTakerSubActor(2, "BTC/USD", RandomTakerSubActorConfig{...}, seed)
arb := NewInternalFundingArb(InternalFundingArbConfig{...})

composite := NewCompositeActor(id, gateway, []SubActor{mm, taker, arb})
composite.InitializeBalances(map[string]int64{"BTC": 100 * BTC_PRECISION}, 10_000 * USD_PRECISION)

// inject cancel capability into sub-actors that need it
mm.SetCancelFn(composite.CancelOrder)

composite.Start(ctx)
```

The composite subscribes to all unique symbols from all sub-actors automatically.

## Best Practices

**Event handling:**
- Keep OnEvent fast (no blocking operations)
- Use custom events for complex internal logic
- Aggregate events when appropriate

**Rejection handling:**
- Always handle rejections explicitly
- Use retry logic for transient failures
- Log unexpected rejections

**Custom events:**
- Use constants for event types (avoid magic numbers)
- Document custom event data structures
- Keep event data serializable

**State management:**
- Update state only in OnEvent
- Use state machines for complex workflows
- Emit events for state transitions

## Next Steps

- [Market Makers](market-makers.md) - MM strategies in detail
- [Takers](takers.md) - Taker strategies
- [Simulated Time](../simulation/simulated-time.md) - Ticker factories and simulation
- [Custom Models](../advanced/custom-models.md) - Create custom exchanges and matching engines
