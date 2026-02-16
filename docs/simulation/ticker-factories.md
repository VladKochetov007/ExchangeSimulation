# Ticker Factories

Abstraction for periodic timers supporting real-time and simulated execution.

## Ticker Interface

```go
type Ticker interface {
    C() <-chan time.Time
    Stop()
}
```

**Standard Go ticker API:**
- `C()` returns channel receiving tick timestamps
- `Stop()` cancels recurring ticks

## TickerFactory Interface

```go
type TickerFactory interface {
    NewTicker(d time.Duration) Ticker
}
```

**Dependency injection:**
- Actors receive factory at construction
- Same actor code works with real or simulated time
- Factory determines ticker behavior

## RealTickerFactory

```go
type RealTickerFactory struct{}

func (f *RealTickerFactory) NewTicker(d time.Duration) Ticker {
    return &realTicker{
        ticker: time.NewTicker(d),
    }
}

type realTicker struct {
    ticker *time.Ticker
}

func (t *realTicker) C() <-chan time.Time {
    return t.ticker.C
}

func (t *realTicker) Stop() {
    t.ticker.Stop()
}
```

**Behavior:**
- Wraps `time.Ticker`
- Wall-clock time
- OS scheduler determines timing
- Non-deterministic

**Use case:** Production, live trading.

## SimTickerFactory

```go
type SimTickerFactory struct {
    scheduler *EventScheduler
}

func NewSimTickerFactory(scheduler *EventScheduler) *SimTickerFactory {
    return &SimTickerFactory{scheduler: scheduler}
}
```

### SimTicker Implementation

```go
type simTicker struct {
    scheduler *EventScheduler
    interval  int64
    ch        chan time.Time  // Buffered: 10
    eventID   uint64
    stopped   bool
    mu        sync.Mutex
}

func (f *SimTickerFactory) NewTicker(d time.Duration) Ticker {
    t := &simTicker{
        scheduler: f.scheduler,
        interval:  int64(d),
        ch:        make(chan time.Time, 10),
    }
    t.start()
    return t
}

func (t *simTicker) start() {
    t.eventID = t.scheduler.ScheduleRepeating(
        t.interval,
        func() {
            t.mu.Lock()
            stopped := t.stopped
            t.mu.Unlock()

            if !stopped {
                select {
                case t.ch <- time.Unix(0, t.scheduler.clock.NowUnixNano()):
                default:
                    // Non-blocking: skip tick if channel full
                }
            }
        },
    )
}

func (t *simTicker) Stop() {
    t.mu.Lock()
    t.stopped = true
    t.mu.Unlock()

    t.scheduler.Cancel(t.eventID)
}

func (t *simTicker) C() <-chan time.Time {
    return t.ch
}
```

**Behavior:**
- Registered with event scheduler
- Fires when `simClock.Advance()` reaches tick time
- Non-blocking send (skips if channel full)
- Deterministic timing

**Use case:** Backtesting, simulation.

## Comparison

| Feature | RealTicker | SimTicker |
|---------|------------|-----------|
| Timing source | OS scheduler | Event scheduler |
| Precision | ~1ms typical | Exact (nanosecond) |
| Deterministic | No | Yes |
| Blocking | No (buffered) | No (buffered) |
| Speed | 1x wall-clock | Arbitrary |
| Dependencies | None | Requires scheduler |

## Usage in Actors

### Construction

```go
type MyActor struct {
    *BaseActor
    tickerFactory TickerFactory
    config        MyConfig
}

func NewMyActor(id uint64, gateway *ClientGateway, cfg MyConfig) *MyActor {
    return &MyActor{
        BaseActor: NewBaseActor(id, gateway),
        config:    cfg,
    }
}

func (a *MyActor) SetTickerFactory(factory TickerFactory) {
    a.tickerFactory = factory
}
```

### Start Method

```go
func (a *MyActor) Start(ctx context.Context) error {
    a.BaseActor.Start(ctx)

    ticker := a.tickerFactory.NewTicker(a.config.Interval)
    go a.tradingLoop(ctx, ticker)

    return nil
}
```

### Trading Loop

```go
func (a *MyActor) tradingLoop(ctx context.Context, ticker Ticker) {
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C():
            a.doWork()  // Executes every interval (real or sim)
        }
    }
}
```

**Key:** Same loop works with both ticker types.

## Non-Blocking Sends

```go
select {
case t.ch <- timestamp:
    // Sent successfully
default:
    // Channel full, skip this tick
}
```

**Why skip instead of block:**
- Prevents deadlock if consumer slow
- Allows simulation to progress
- Lost ticks acceptable for most strategies (just delayed action)

**Alternatives:**
- Block (risks deadlock)
- Panic (too harsh)
- Log and skip (diagnostic overhead)

## Buffered Channel

```go
ch: make(chan time.Time, 10)
```

**Buffer size = 10:**
- Allows bursts of 10 ticks
- Typical: actor consumes 1 tick per iteration
- Overflow only if actor stalls for 10+ intervals

**Tradeoffs:**
- Larger buffer: More tolerance for slowness
- Smaller buffer: Faster detection of problems
- Zero buffer: Immediate backpressure

## Example: Market Maker Requoting

```go
type MarketMaker struct {
    *BaseActor
    tickerFactory TickerFactory
    requoteInterval time.Duration
}

func (mm *MarketMaker) Start(ctx context.Context) error {
    mm.BaseActor.Start(ctx)

    ticker := mm.tickerFactory.NewTicker(mm.requoteInterval)
    go mm.requoteLoop(ctx, ticker)

    return nil
}

func (mm *MarketMaker) requoteLoop(ctx context.Context, ticker Ticker) {
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C():
            mm.cancelOldQuotes()
            mm.placeNewQuotes()
        }
    }
}
```

**Real-time:**
- `requoteInterval = 1 * time.Second`
- Requotes every 1 wall-clock second

**Simulated:**
- `requoteInterval = 1 * time.Second`
- Requotes every 1 simulated second
- May be 0.01 wall-clock seconds with 100x speedup

## Example: Randomized Taker

```go
type RandomizedTaker struct {
    *BaseActor
    tickerFactory TickerFactory
    tradeInterval time.Duration
    rng           *rand.Rand
}

func (rt *RandomizedTaker) Start(ctx context.Context) error {
    rt.BaseActor.Start(ctx)

    ticker := rt.tickerFactory.NewTicker(rt.tradeInterval)
    go rt.tradeLoop(ctx, ticker)

    return nil
}

func (rt *RandomizedTaker) tradeLoop(ctx context.Context, ticker Ticker) {
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C():
            side := rt.randomSide()
            qty := rt.randomQty()

            rt.SubmitOrder(&OrderRequest{
                Symbol: rt.config.Symbol,
                Side:   side,
                Type:   Market,
                Qty:    qty,
            })
        }
    }
}
```

**Deterministic randomness:**
```go
rng: rand.New(rand.NewSource(seed))  // Seeded RNG
```

Same seed → same trades → reproducible results.

## Multiple Tickers

```go
func (a *MyActor) Start(ctx context.Context) error {
    a.BaseActor.Start(ctx)

    fastTicker := a.tickerFactory.NewTicker(100 * time.Millisecond)
    slowTicker := a.tickerFactory.NewTicker(1 * time.Second)

    go a.fastLoop(ctx, fastTicker)
    go a.slowLoop(ctx, slowTicker)

    return nil
}

func (a *MyActor) fastLoop(ctx context.Context, ticker Ticker) {
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C():
            a.checkMarket()  // Fast check
        }
    }
}

func (a *MyActor) slowLoop(ctx context.Context, ticker Ticker) {
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C():
            a.requote()  // Slow requote
        }
    }
}
```

**Use case:** Different task frequencies (monitoring vs trading).

## Testing

```go
func TestActorWithSimTicker(t *testing.T) {
    simClock := simulation.NewSimulatedClock(0)
    scheduler := simulation.NewEventScheduler(simClock)
    simClock.SetScheduler(scheduler)

    factory := simulation.NewSimTickerFactory(scheduler)

    actor := NewMyActor(1, gateway, config)
    actor.SetTickerFactory(factory)
    actor.Start(context.Background())

    // Advance time, check behavior
    simClock.Advance(1 * time.Second)
    // Actor's ticker fires once

    simClock.Advance(5 * time.Second)
    // Actor's ticker fires 5 more times

    // Total: 6 ticks (deterministic)
}
```

**Benefits:**
- Fast tests (no actual waiting)
- Deterministic
- Precise control

## Performance

### Real Ticker

**Overhead:**
- OS timer: ~1µs per tick
- Channel send: ~100ns
- Total: Negligible for most strategies

**Limitations:**
- Minimum precision: ~1ms (OS dependent)
- Jitter: ±1-10ms typical

### Sim Ticker

**Overhead:**
- Heap insertion: O(log n) per schedule
- Callback execution: Variable
- Total: Dominated by callback work

**Scalability:**
- 1,000 tickers: ~100µs per advance
- 10,000 tickers: ~1ms per advance
- Heap operations dominate

## Best Practices

**Inject factory:**
```go
actor.SetTickerFactory(factory)  // Before Start()
```

**Stop tickers:**
```go
defer ticker.Stop()  // Always cleanup
```

**Non-blocking receives:**
```go
for {
    select {
    case <-ctx.Done():
        return
    case <-ticker.C():
        doWork()
    }
}
```

**Deterministic RNG:**
```go
rng := rand.New(rand.NewSource(seed))  // Per-actor RNG
```

## Next Steps

- [Simulated Time](simulated-time.md) - Event scheduler details
- [Actor System](../actors/actor-system.md) - Actor framework
- [Random Walk Example](../quickstart/02-randomwalk-example.md) - Complete example
