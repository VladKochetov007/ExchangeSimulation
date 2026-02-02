# Architecture Diagram

## System Overview

```mermaid
graph TB
    subgraph "Simulation Layer"
        Runner[Simulation Runner]
        Clock[Clock Interface]
        RealClock[RealClock]
        SimClock[SimulatedClock]
        Latency[Latency Providers]
    end

    subgraph "Actor Layer"
        Actor1[Market Maker 1]
        Actor2[Market Maker 2]
        ActorN[Market Maker N]
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
    participant MM as Market Maker Actor
    participant BA as BaseActor
    participant GW as Client Gateway
    participant EX as Exchange
    participant BK as Order Book
    participant MT as Matcher
    participant MD as MD Publisher

    MM->>BA: SubmitOrder(symbol, side, price, qty)
    BA->>BA: Increment requestSeq
    BA->>GW: RequestCh send OrderRequest

    GW->>EX: handleClientRequests()
    EX->>EX: Validate client, balance, price/qty
    EX->>EX: Reserve collateral
    EX->>BK: Check crossing

    alt Order Crosses
        BK->>MT: Match(order)
        MT->>EX: Executions
        EX->>EX: Process settlements
        EX->>MD: Publish trade and deltas
    else Order Rests
        BK->>BK: Add to limit
        EX->>MD: Publish delta
    end

    EX->>GW: ResponseCh send Response(orderID)
    GW->>BA: Response received
    BA->>BA: handleResponse()
    BA->>BA: eventCh send OrderAccepted
    BA->>MM: EventChannel()
    MM->>MM: OnEvent(OrderAccepted)
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

    EventType -->|OrderAccepted| TrackOrder[Track Order ID]
    EventType -->|OrderFilled| UpdateState[Update Position]
    EventType -->|Trade| OnTrade[on_trade_tick]
    EventType -->|BookSnapshot| OnSnapshot[on_orderbook_snapshot]
    EventType -->|BookDelta| OnDelta[on_orderbook_delta]

    TrackOrder --> EventLoop
    UpdateState --> EventLoop
    OnTrade --> EventLoop
    OnSnapshot --> EventLoop
    OnDelta --> EventLoop

    Shutdown --> End[Exit]
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
