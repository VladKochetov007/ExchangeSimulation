# Simulated Time

Deterministic time simulation for backtesting with event scheduling and time compression.

## Clock Abstraction

```go
type Clock interface {
    NowUnixNano() int64
    NowUnix() int64
}
```

**Why abstraction:**
- Same code for production (real-time) and backtesting (simulated)
- Deterministic execution in tests
- Time compression (100x+ speedup)
- Reproducible results

## RealClock

```go
type RealClock struct{}

func (c *RealClock) NowUnixNano() int64 {
    return time.Now().UnixNano()
}
```

**Use case:** Production trading, live market connection.

**Characteristics:**
- Wall-clock time
- Non-deterministic
- Real-time progression
- No control over speed

## SimulatedClock

```go
type SimulatedClock struct {
    current   int64         // Current sim time (nanoseconds)
    mu        sync.RWMutex
    scheduler *EventScheduler
}

func NewSimulatedClock(startTime int64) *SimulatedClock {
    return &SimulatedClock{
        current: startTime,
    }
}
```

**Manual time progression:**
```go
simClock.Advance(10 * time.Millisecond)  // Jump forward 10ms
```

**Characteristics:**
- Controlled time
- Deterministic
- Arbitrary speed (1x, 100x, 1000x)
- Event-driven

## Time Advancement

```go
func (c *SimulatedClock) Advance(delta time.Duration) {
    c.mu.Lock()
    c.current += int64(delta)
    newTime := c.current
    c.mu.Unlock()

    if c.scheduler != nil {
        c.scheduler.ProcessUntil(newTime)
    }
}
```

**Flow:**
1. Increment current time
2. Process all scheduled events ≤ new time
3. Events fire callbacks (actor timers, automation tasks)
4. Callbacks may submit orders, change state
5. Return control to caller

## Event Scheduler

Min-heap priority queue for time-based callbacks.

```go
type EventScheduler struct {
    clock     *SimulatedClock
    events    eventHeap  // Min-heap ordered by fireTime
    nextID    uint64
    mu        sync.Mutex
}
```

### Scheduling Events

**One-time event:**
```go
id := scheduler.Schedule(
    time.Now().UnixNano() + int64(5*time.Second),
    func() {
        fmt.Println("5 seconds elapsed (sim-time)")
    },
)
```

**Repeating event:**
```go
id := scheduler.ScheduleRepeating(
    int64(1 * time.Second),  // Interval
    func() {
        actor.requote()  // Called every 1 sim-second
    },
)
```

**Cancellation:**
```go
scheduler.Cancel(id)
```

### Event Processing

```go
func (s *EventScheduler) ProcessUntil(untilTime int64) {
    s.mu.Lock()
    defer s.mu.Unlock()

    for s.events.Len() > 0 {
        ev := s.events.Peek()
        if ev.fireTime > untilTime {
            break  // No more events due
        }

        s.events.Pop()

        if ev.repeating {
            // Reschedule for next interval
            ev.fireTime += ev.interval
            s.events.Push(ev)
        }

        s.mu.Unlock()
        ev.callback()  // Execute outside lock
        s.mu.Lock()
    }
}
```

**Key properties:**
- Events fire in time order
- Callbacks execute serially (no concurrency)
- Repeating events reschedule automatically
- Unlock during callback prevents deadlock

## Event Heap

```go
type eventHeap []*scheduledEvent

type scheduledEvent struct {
    id        uint64
    fireTime  int64
    interval  int64
    callback  func()
    repeating bool
}

func (h eventHeap) Less(i, j int) bool {
    return h[i].fireTime < h[j].fireTime  // Min-heap by time
}
```

**Complexity:**
- Insert: O(log n)
- Pop min: O(log n)
- Peek min: O(1)

## Simulation Loop

```go
simClock := simulation.NewSimulatedClock(time.Now().UnixNano())
scheduler := simulation.NewEventScheduler(simClock)
simClock.SetScheduler(scheduler)

ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
    Clock: simClock,
    TickerFactory: simulation.NewSimTickerFactory(scheduler),
})

// Start actors
for _, actor := range actors {
    actor.SetTickerFactory(simulation.NewSimTickerFactory(scheduler))
    actor.Start(ctx)
}

// Time compression: 100x speedup
simTimeStep := 10 * time.Millisecond
realTimeStep := simTimeStep / 100
ticker := time.NewTicker(realTimeStep)  // Real-time ticker

for {
    select {
    case <-ctx.Done():
        return
    case <-ticker.C:
        simClock.Advance(simTimeStep)  // Advance sim-time
        // All scheduled events fire here
    }
}
```

**Time compression math:**
```
Sim time step: 10ms
Real time step: 10ms / 100 = 0.1ms
Speedup: 10ms / 0.1ms = 100x

1 simulated second = 10ms wall-clock
1 simulated hour = 36 seconds wall-clock
1 simulated day = 14.4 minutes wall-clock
```

## Ticker Factories

### TickerFactory Interface

```go
type TickerFactory interface {
    NewTicker(d time.Duration) Ticker
}

type Ticker interface {
    C() <-chan time.Time
    Stop()
}
```

### RealTickerFactory

```go
type RealTickerFactory struct{}

func (f *RealTickerFactory) NewTicker(d time.Duration) Ticker {
    return &realTicker{
        ticker: time.NewTicker(d),
    }
}
```

**Behavior:** Standard Go ticker, wall-clock time.

### SimTickerFactory

```go
type SimTickerFactory struct {
    scheduler *EventScheduler
}

type simTicker struct {
    scheduler *EventScheduler
    interval  int64
    ch        chan time.Time  // Buffered: 10
    eventID   uint64
    stopped   bool
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
            if !t.stopped {
                select {
                case t.ch <- time.Unix(0, t.scheduler.clock.NowUnixNano()):
                default:
                    // Skip if channel full (non-blocking)
                }
            }
        },
    )
}
```

**Behavior:**
- Registered with event scheduler
- Fires on simulated time progression
- Non-blocking send (skip tick if slow consumer)
- Deterministic timing

## Actor Integration

```go
type MyActor struct {
    *BaseActor
    tickerFactory TickerFactory
}

func (a *MyActor) Start(ctx context.Context) error {
    a.BaseActor.Start(ctx)

    ticker := a.tickerFactory.NewTicker(1 * time.Second)
    go a.tradingLoop(ctx, ticker)

    return nil
}

func (a *MyActor) tradingLoop(ctx context.Context, ticker Ticker) {
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C():
            a.trade()  // Fires every 1 second (real or sim)
        }
    }
}
```

**Key:** Same code works with both ticker types.

## Determinism

**Sources of non-determinism in trading:**
1. Wall-clock time (time.Now())
2. Random number generators (without seed)
3. Map iteration order
4. Goroutine scheduling
5. External I/O timing

**Solutions:**
1. Use Clock interface
2. Seed RNG explicitly: `rand.New(rand.NewSource(seed))`
3. Sort map keys before iteration
4. Single-threaded event processing
5. Simulate I/O with deterministic delays

**Example: Deterministic RNG**
```go
type MyActor struct {
    rng *rand.Rand
}

func NewMyActor(id uint64, gateway *exchange.ClientGateway, seed int64) *MyActor {
    return &MyActor{
        rng: rand.New(rand.NewSource(seed)),  // Deterministic
    }
}

func (a *MyActor) randomQty() int64 {
    return a.rng.Int63n(100)  // Same sequence every run
}
```

## Example: Backtesting a Day

```go
func backtestDay() {
    startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano()
    simClock := simulation.NewSimulatedClock(startTime)
    scheduler := simulation.NewEventScheduler(simClock)
    simClock.SetScheduler(scheduler)

    ex := setupExchange(simClock, scheduler)
    actors := setupActors(ex, scheduler)

    // Simulate 24 hours in steps of 10ms
    simTimeStep := 10 * time.Millisecond
    totalSteps := int(24 * time.Hour / simTimeStep)  // 8,640,000 steps

    for i := 0; i < totalSteps; i++ {
        simClock.Advance(simTimeStep)

        if i % 100000 == 0 {
            // Progress: ~every 1000 sim-seconds
            elapsed := time.Duration(simClock.NowUnixNano() - startTime)
            fmt.Printf("Progress: %v\n", elapsed)
        }
    }

    // 24 hours simulated
    // Time taken: depends on CPU, typically 1-5 minutes
}
```

## Comparison: Real vs Simulated

| Feature | RealClock + RealTicker | SimClock + SimTicker |
|---------|------------------------|----------------------|
| Time source | `time.Now()` | Manual `Advance()` |
| Speed | 1x (wall-clock) | Arbitrary (100x+) |
| Deterministic | No | Yes |
| Reproducible | No | Yes (with seeded RNG) |
| Use case | Production | Backtesting, testing |
| Event ordering | Non-deterministic | Deterministic |
| Waiting | Real time delays | Instant (virtual) |

## Performance

**Simulated time advantages:**
- No actual waiting (sleep)
- CPU-bound, not I/O-bound
- Parallelizable across simulations
- Faster than real-time

**Example benchmarks (2024 laptop):**
- 1 simulated day: ~2-5 minutes wall-clock
- 1 simulated week: ~15-30 minutes
- 100x speedup typical
- 1000x+ possible with sparse events

## Testing with Simulated Time

```go
func TestMarketMaker(t *testing.T) {
    simClock := simulation.NewSimulatedClock(0)
    scheduler := simulation.NewEventScheduler(simClock)
    simClock.SetScheduler(scheduler)

    ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
        Clock: simClock,
    })

    mm := NewMarketMaker(1, gateway, config)
    mm.SetTickerFactory(simulation.NewSimTickerFactory(scheduler))
    mm.Start(context.Background())

    // Advance time, check state
    simClock.Advance(1 * time.Second)
    assert.Equal(t, 2, len(mm.activeOrders))  // Bid + ask

    simClock.Advance(10 * time.Second)
    // MM should have requoted
}
```

**Benefits:**
- Fast tests (no actual waiting)
- Deterministic (no flakiness)
- Precise timing control
- Parallel test execution

## Custom Clock Implementations

### Replay Clock

Replays historical timestamps from a dataset:

```go
type ReplayClock struct {
    timestamps []int64
    index      int
    mu         sync.RWMutex
}

func NewReplayClock(timestamps []int64) *ReplayClock {
    return &ReplayClock{
        timestamps: timestamps,
        index:      0,
    }
}

func (c *ReplayClock) NowUnixNano() int64 {
    c.mu.RLock()
    defer c.mu.RUnlock()

    if c.index >= len(c.timestamps) {
        return c.timestamps[len(c.timestamps)-1]
    }
    return c.timestamps[c.index]
}

func (c *ReplayClock) Advance() {
    c.mu.Lock()
    defer c.mu.Unlock()

    if c.index < len(c.timestamps)-1 {
        c.index++
    }
}
```

**Use case:** Replay historical market data with exact original timestamps.

### Controllable Real Clock

Real-time clock with pause/resume:

```go
type ControllableClock struct {
    startReal  time.Time
    startSim   int64
    offset     int64
    paused     bool
    pausedAt   int64
    mu         sync.RWMutex
}

func NewControllableClock() *ControllableClock {
    now := time.Now()
    return &ControllableClock{
        startReal: now,
        startSim:  now.UnixNano(),
    }
}

func (c *ControllableClock) NowUnixNano() int64 {
    c.mu.RLock()
    defer c.mu.RUnlock()

    if c.paused {
        return c.pausedAt
    }

    elapsed := time.Since(c.startReal).Nanoseconds()
    return c.startSim + elapsed + c.offset
}

func (c *ControllableClock) Pause() {
    c.mu.Lock()
    defer c.mu.Unlock()

    if !c.paused {
        c.pausedAt = c.startSim + time.Since(c.startReal).Nanoseconds() + c.offset
        c.paused = true
    }
}

func (c *ControllableClock) Resume() {
    c.mu.Lock()
    defer c.mu.Unlock()

    if c.paused {
        pauseDuration := time.Since(c.startReal).Nanoseconds() -
                        (c.pausedAt - c.startSim - c.offset)
        c.offset -= pauseDuration
        c.paused = false
    }
}

func (c *ControllableClock) SetSpeed(multiplier float64) {
    // Adjust offset to change effective speed
    c.mu.Lock()
    defer c.mu.Unlock()
    // Implementation depends on use case
}
```

**Use case:** Interactive simulations with pause/resume, time dilation.

### Hybrid Clock

Uses real time but can skip ahead:

```go
type HybridClock struct {
    realClock *RealClock
    offset    int64
    mu        sync.RWMutex
}

func (c *HybridClock) NowUnixNano() int64 {
    c.mu.RLock()
    defer c.mu.RUnlock()

    return time.Now().UnixNano() + c.offset
}

func (c *HybridClock) SkipAhead(duration time.Duration) {
    c.mu.Lock()
    defer c.mu.Unlock()

    c.offset += int64(duration)
}
```

**Use case:** Testing time-sensitive code (expirations, timeouts) without full simulation.

## Clock Selection Guide

| Clock Type | Deterministic | Speed | Use Case |
|------------|--------------|-------|----------|
| RealClock | No | 1x | Production, live trading |
| SimulatedClock | Yes | Any | Backtesting, unit tests |
| ReplayClock | Yes | Variable | Historical replay |
| ControllableClock | No | Variable | Interactive simulation |
| HybridClock | No | 1x + jumps | Testing timeouts/expiry |

## Custom Exchange with Clock

```go
type CustomExchange struct {
    *exchange.Exchange
    customClock Clock
}

func (ex *CustomExchange) Now() time.Time {
    return time.Unix(0, ex.customClock.NowUnixNano())
}

// Use custom clock for all time-dependent operations
func (ex *CustomExchange) CheckExpiry(order *Order) bool {
    if order.ExpireTime == 0 {
        return false
    }
    return ex.customClock.NowUnixNano() > order.ExpireTime
}
```

## Next Steps

- [Ticker Factories](ticker-factories.md) - Ticker implementation details
- [Actor System](../actors/actor-system.md) - How actors use tickers
- [Random Walk Example](../quickstart/02-randomwalk-example.md) - Full simulation example
