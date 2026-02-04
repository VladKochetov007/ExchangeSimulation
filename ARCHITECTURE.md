# Architecture Diagram

## System Overview

```mermaid
graph LR
    subgraph "Simulation Layer"
        Runner[Simulation Runner]
        Clock[Clock Interface]
        RealClock[RealClock]
        SimClock[SimulatedClock]
        Latency[Latency Providers]
    end

    subgraph "Actor Layer"
        Actor1[Trading Actor 1]
        Actor2[Trading Actor 2]
        ActorN[Trading Actor N]
        Recorder[Recorder Actor]
        BaseActor[BaseActor Framework]
    end

    subgraph "Exchange Core"
        Exchange[Exchange]
        Books[Order Books]
        Matcher[Matching Engine]
        MDPub[Market Data Publisher]
        PosMan[Position Manager]

        subgraph "Gateway Layer"
            GW1[Client Gateway 1]
            GW2[Client Gateway 2]
            GWN[Client Gateway N]
        end
    end

    subgraph "Storage"
        TradesCSV[trades.csv]
        SnapshotsCSV[snapshots.csv]
    end

    Runner -->|manages| Exchange
    Runner -->|manages| Actor1
    Runner -->|manages| Actor2
    Runner -->|manages| ActorN
    Runner -->|manages| Recorder
    Runner -->|injects| Clock

    Clock -.->|implements| RealClock
    Clock -.->|implements| SimClock

    Actor1 -->|uses| BaseActor
    Actor2 -->|uses| BaseActor
    ActorN -->|uses| BaseActor
    Recorder -->|uses| BaseActor

    BaseActor -->|connects via| GW1
    BaseActor -->|connects via| GW2
    Actor1 -.->|sends requests| GW1
    Actor2 -.->|sends requests| GW2
    Recorder -.->|subscribes| GWN

    GW1 -->|processes| Exchange
    GW2 -->|processes| Exchange
    GWN -->|processes| Exchange

    Exchange -->|uses| Clock
    Exchange -->|owns| Books
    Exchange -->|uses| Matcher
    Exchange -->|uses| MDPub
    Exchange -->|uses| PosMan

    Matcher -->|uses| Clock
    PosMan -->|uses| Clock

    MDPub -->|publishes to| GW1
    MDPub -->|publishes to| GW2
    MDPub -->|publishes to| GWN

    Books -->|executions| MDPub

    Recorder -->|writes| TradesCSV
    Recorder -->|writes| SnapshotsCSV

    Latency -.->|can wrap| GW1
    Latency -.->|can wrap| GW2
```

## Data Flow: Order Submission

```mermaid
sequenceDiagram
    participant TA as Trading Actor
    participant BA as BaseActor
    participant GW as Client Gateway
    participant EX as Exchange
    participant BK as Order Book
    participant MT as Matcher
    participant MD as MD Publisher

    TA->>BA: SubmitOrder(symbol, side, price, qty)
    BA->>BA: Increment requestSeq
    BA->>GW: RequestCh send OrderRequest

    GW->>EX: handleClientRequests()
    EX->>EX: Validate client, balance, price/qty
    EX->>EX: Reserve collateral
    EX->>BK: Check crossing

    alt Order Crosses (Taker)
        BK->>MT: Match(order)
        MT->>EX: Executions
        EX->>EX: Process settlements
        EX->>MD: Publish trade and deltas
    else Order Rests (Maker)
        BK->>BK: Add to limit
        EX->>MD: Publish delta
    end

    EX->>GW: ResponseCh send Response(orderID)
    GW->>BA: Response received
    BA->>BA: handleResponse()

    alt Success
        BA->>BA: eventCh send OrderAccepted
        BA->>TA: EventChannel()
        TA->>TA: OnEvent(OrderAccepted)
    else Rejection
        BA->>BA: eventCh send OrderRejected
        BA->>TA: EventChannel()
        TA->>TA: OnEvent(OrderRejected)
        Note right of TA: Actor handles rejection<br/>Could retry, log, adjust strategy
    end
```

## Data Flow: Market Data

```mermaid
sequenceDiagram
    participant BK as Order Book
    participant MD as MD Publisher
    participant G1 as Gateway 1
    participant G2 as Gateway 2
    participant AC as Actor
    participant RC as Recorder

    BK->>MD: Trade executed
    MD->>MD: Create MarketDataMsg

    par Broadcast to all subscribers
        MD->>G1: MarketData send TradeMsg
        MD->>G2: MarketData send TradeMsg
    end

    G1->>AC: Channel read
    AC->>AC: handleMarketData()
    AC->>AC: OnEvent(TradeEvent)

    G2->>RC: Channel read
    RC->>RC: handleMarketData()
    RC->>RC: writeCh send tradeRecord
    RC->>RC: writeLoop()
    RC->>RC: Write to CSV
```

## Clock Abstraction

```mermaid
graph LR
    subgraph "Clock Interface"
        Clock[Clock Interface]
    end

    subgraph "Implementations"
        RealClock[RealClock<br/>time.Now]
        SimClock[SimulatedClock<br/>controlled time]
    end

    subgraph "Consumers"
        Exchange[Exchange]
        Matcher[Matcher]
        PosMan[Position Manager]
    end

    Clock -.->|implements| RealClock
    Clock -.->|implements| SimClock

    Exchange -->|uses| Clock
    Matcher -->|uses| Clock
    PosMan -->|uses| Clock
```

## Actor Event Loop

```mermaid
graph TD
    Start[Actor.Start] --> EventLoop[Event Loop]

    EventLoop --> Select{Select on channels}

    Select -->|ctx.Done| Shutdown[Shutdown]
    Select -->|stopCh| Shutdown
    Select -->|EventChannel| HandleEvent[OnEvent]

    HandleEvent --> EventType{Event Type?}

    EventType -->|OrderAccepted| TrackOrder[Track Order ID<br/>Maker or Taker]
    EventType -->|OrderRejected| HandleReject[Handle Rejection<br/>Log reason, retry]
    EventType -->|OrderPartialFill| UpdatePartial[Update Position<br/>Track remaining]
    EventType -->|OrderFilled| UpdateFilled[Update Position<br/>Fully filled]
    EventType -->|OrderCancelled| RemoveOrder[Remove from tracking<br/>Release collateral]
    EventType -->|OrderCancelRejected| HandleCancelFail[Handle Cancel Fail<br/>Order still active]
    EventType -->|Trade| OnTrade[on_trade_tick<br/>React to Market]
    EventType -->|BookSnapshot| OnSnapshot[on_orderbook_snapshot<br/>Analyze Liquidity]
    EventType -->|BookDelta| OnDelta[on_orderbook_delta<br/>Update Local Book]
    EventType -->|FundingUpdate| OnFunding[Handle Funding Rate<br/>Perp futures only]

    TrackOrder --> EventLoop
    HandleReject --> EventLoop
    UpdatePartial --> EventLoop
    UpdateFilled --> EventLoop
    RemoveOrder --> EventLoop
    HandleCancelFail --> EventLoop
    OnTrade --> EventLoop
    OnSnapshot --> EventLoop
    OnDelta --> EventLoop
    OnFunding --> EventLoop

    Shutdown --> End[Exit]
```

### All Event Types (10 total)

**Order Lifecycle Events:**
1. `EventOrderAccepted` - Order placed successfully (returns OrderID)
2. `EventOrderRejected` - Order rejected (includes RejectReason)
3. `EventOrderPartialFill` - Order partially filled
4. `EventOrderFilled` - Order fully filled
5. `EventOrderCancelled` - Order cancelled successfully (includes remaining qty)
6. `EventOrderCancelRejected` - Cancel failed (includes RejectReason)

**Market Data Events:**
7. `EventTrade` - Trade executed on exchange
8. `EventBookDelta` - Orderbook level changed
9. `EventBookSnapshot` - Full orderbook snapshot

**Perp Futures Event:**
10. `EventFundingUpdate` - Funding rate updated

## Rejection Handling

### Order Rejection Reasons

When an order is rejected, actors receive `EventOrderRejected` with one of these reasons:

| Reject Reason | Code | Description | Actor Response |
|---------------|------|-------------|----------------|
| `RejectInsufficientBalance` | 0 | Not enough balance for order | Query balance, adjust size |
| `RejectInvalidPrice` | 1 | Price not multiple of tick size | Round to tick size, resubmit |
| `RejectInvalidQty` | 2 | Quantity below minimum | Increase qty or skip |
| `RejectUnknownClient` | 3 | Client ID not connected | Fatal error, reconnect |
| `RejectUnknownInstrument` | 4 | Symbol doesn't exist | Check ListInstruments() |
| `RejectSelfTrade` | 5 | Would match own order | Wait, adjust price |
| `RejectDuplicateOrderID` | 6 | OrderID collision (rare) | Retry |
| `RejectOrderNotFound` | 7 | Cancel failed - order doesn't exist | Already filled/cancelled |
| `RejectOrderNotOwned` | 8 | Cancel failed - not your order | Logic error |
| `RejectOrderAlreadyFilled` | 9 | Cancel failed - already filled | Order completed |

### Cancel Rejection Reasons

When a cancel request is rejected, actors receive `EventOrderCancelRejected`:

```go
type OrderCancelRejectedEvent struct {
    OrderID   uint64
    RequestID uint64
    Reason    exchange.RejectReason  // RejectOrderNotFound, RejectOrderNotOwned, RejectOrderAlreadyFilled
}
```

**Common scenarios:**
- `RejectOrderNotFound` - Order already filled or cancelled
- `RejectOrderNotOwned` - Logic error (trying to cancel someone else's order)
- `RejectOrderAlreadyFilled` - Order executed between cancel request and processing

### Example: Handling Rejections

```go
func (a *MyActor) OnEvent(event *Event) {
    switch event.Type {
    case EventOrderRejected:
        rejection := event.Data.(OrderRejectedEvent)

        switch rejection.Reason {
        case exchange.RejectInsufficientBalance:
            // Query balance, adjust position sizing
            a.QueryBalance()

        case exchange.RejectInvalidPrice:
            // Price not aligned to tick size
            // Recalculate and resubmit with proper rounding

        case exchange.RejectUnknownInstrument:
            // Symbol doesn't exist
            // Query available instruments
            instruments := a.exchange.ListInstruments("", "USD")

        case exchange.RejectSelfTrade:
            // Would cross our own order
            // Wait a bit, market will change

        default:
            // Log and move on
        }

    case EventOrderCancelRejected:
        rejection := event.Data.(OrderCancelRejectedEvent)

        switch rejection.Reason {
        case exchange.RejectOrderNotFound:
            // Order already gone (filled or cancelled)
            // Remove from local tracking

        case exchange.RejectOrderAlreadyFilled:
            // Order filled before cancel arrived
            // Update position, order completed successfully

        case exchange.RejectOrderNotOwned:
            // Logic error - trying to cancel wrong order
            // This should never happen, indicates bug
        }
    }
}
```

### Rejection Flow Diagram

```mermaid
sequenceDiagram
    participant TA as Trading Actor
    participant BA as BaseActor
    participant EX as Exchange

    TA->>BA: SubmitOrder(BTCUSD, Buy, 100 BTC)
    BA->>EX: OrderRequest

    EX->>EX: Validate balance
    Note over EX: Insufficient balance!

    EX->>BA: Response(Success=false, Error=RejectInsufficientBalance)
    BA->>BA: handleResponse()
    BA->>BA: Create OrderRejectedEvent
    BA->>TA: EventChannel()
    TA->>TA: OnEvent(OrderRejected)

    Note over TA: Actor handles rejection
    TA->>BA: QueryBalance()
    BA->>EX: QueryRequest
    EX->>BA: Response(BalanceSnapshot)

    Note over TA: Adjust order size
    TA->>BA: SubmitOrder(BTCUSD, Buy, 1 BTC)
    BA->>EX: OrderRequest
    EX->>BA: Response(Success=true, OrderID=123)
    BA->>TA: OnEvent(OrderAccepted)
```

## Recorder Actor Data Flow

```mermaid
graph LR
    subgraph "Input"
        Trade[Trade Events]
        Snapshot[Snapshot Events]
    end

    subgraph "Recorder Actor"
        EventLoop[Event Loop]
        WriteCh[Write Channel<br/>buffer: 10k]
        WriteLoop[Write Loop]
        Buf1[Trades Buffer]
        Buf2[Snapshots Buffer]
    end

    subgraph "Output"
        CSV1[trades.csv]
        CSV2[snapshots.csv]
    end

    Trade --> EventLoop
    Snapshot --> EventLoop

    EventLoop -->|non-blocking| WriteCh
    WriteCh --> WriteLoop

    WriteLoop --> Buf1
    WriteLoop --> Buf2

    Buf1 -->|periodic flush| CSV1
    Buf2 -->|periodic flush| CSV2

    WriteLoop -.->|every 1s| Flush[Flush to disk]
```

## Latency Simulation

```mermaid
graph TD
    subgraph "Latency Providers"
        Constant[ConstantLatency<br/>fixed delay]
        Uniform[UniformRandomLatency<br/>random in range]
        Normal[NormalLatency<br/>Gaussian distribution]
    end

    subgraph "DelayedGateway (future)"
        DGW[DelayedGateway]
        Latency[LatencyProvider]
    end

    Actor[Actor] -->|SendRequest| DGW
    DGW -->|Delay| Latency
    Latency -.->|implements| Constant
    Latency -.->|implements| Uniform
    Latency -.->|implements| Normal
    DGW -->|sleep| RealGW[Real Gateway]
```

## Trading Actor Strategies

Actors are not limited to market making - they can implement any trading strategy:

### Actor Types

1. **Market Maker** (`actor/marketmaker.go`)
   - Provides liquidity on both sides
   - Places limit orders (maker)
   - Profits from bid-ask spread
   - Example: 2-sided quoting with configurable spread

2. **Taker Strategies** (extensible)
   - Consumes liquidity with market orders
   - Aggressive limit orders that cross the spread
   - Examples: momentum traders, arbitrageurs
   - React to market data events

3. **Mixed Strategies** (extensible)
   - Combine making and taking
   - Example: Place maker orders, occasionally take when opportunity arises
   - Adaptive strategies based on market conditions
   - Switch between making and taking based on signals

4. **Passive Actors**
   - Recorder actor (data collection)
   - Monitor-only actors
   - Risk management actors

### Strategy Implementation

All actors inherit from `BaseActor` and implement their strategy in `OnEvent`:

```go
type MyTakerActor struct {
    *BaseActor
    targetPrice int64
}

func (a *MyTakerActor) OnEvent(event *Event) {
    switch event.Type {
    case EventBookSnapshot:
        snap := event.Data.(BookSnapshotEvent)
        if len(snap.Snapshot.Asks) > 0 {
            bestAsk := snap.Snapshot.Asks[0].Price
            if bestAsk <= a.targetPrice {
                // Aggressive taker: use market order
                a.SubmitOrder(snap.Symbol, exchange.Buy, exchange.Market, 0, qty)
            }
        }
    case EventTrade:
        // React to market trades
        // Adjust strategy based on market direction
    }
}
```

### Maker vs Taker

- **Maker**: Order rests on book, provides liquidity, gets maker fee (lower/rebate)
- **Taker**: Order crosses spread immediately, removes liquidity, pays taker fee (higher)
- **Mixed**: Strategy determines when to make vs take based on signals

The exchange doesn't distinguish actor types - all actors can submit any order type and the matching engine determines if they're maker or taker based on order behavior.

## Package Structure

```
exchange_simulation/
├── exchange/              # Core exchange (flat structure)
│   ├── types.go          # Enums, structs
│   ├── order.go          # Order linking
│   ├── book.go           # Order book
│   ├── matching.go       # Matching engine
│   ├── client.go         # Client accounts
│   ├── exchange.go       # Main exchange
│   ├── gateway.go        # Communication
│   ├── marketdata.go     # Market data pub
│   ├── funding.go        # Funding rates
│   ├── fee.go            # Fee models
│   ├── instrument.go     # Instruments
│   └── pools.go          # Object pools
│
├── actor/                # Actor framework
│   ├── events.go         # Event types
│   ├── actor.go          # BaseActor
│   ├── marketmaker.go    # Market maker
│   └── recorder.go       # Data recorder
│
├── simulation/           # Simulation infrastructure
│   ├── clock.go          # Clock abstraction
│   ├── latency.go        # Latency simulation
│   └── runner.go         # Simulation runner
│
└── cmd/sim/             # Entry point
    └── main.go
```

## Why Exchange Package Stayed Flat

Per the implementation plan:
> **Module Restructuring**: Current: 28 files flat in `exchange/`. Proposed: Keep flat for now, add new top-level directories.
>
> **Rationale**: Moving exchange files would break imports in 50+ tests. Not worth the disruption. New modules are cleanly separated.

Benefits of flat structure:
- ✅ Simple imports: `import "exchange_sim/exchange"`
- ✅ All tests work without changes
- ✅ No circular dependency issues
- ✅ Fast compilation (Go compiler optimizes flat packages)
- ✅ Easy navigation (all files in one place)

Alternative (if needed):
```
exchange/
├── core/        # Order, Book, Limit
├── matching/    # Matching engine
├── client/      # Client, Gateway
├── market/      # Instruments, Funding
└── pubsub/      # Market data
```

But this adds complexity without clear benefit for a simulation.
