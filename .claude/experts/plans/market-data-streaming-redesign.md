# Market Data Streaming System Redesign

**Date**: 2026-02-13
**Status**: Design Review
**Complexity**: Medium (refactoring existing system, not new feature)

---

## Problem Statement

### Current Broken Architecture

**One-shot snapshot problem:**
```go
// exchange.go:812-840
func (e *Exchange) subscribe(clientID uint64, req *QueryRequest, gateway *ClientGateway) Response {
    // ...
    e.MDPublisher.Subscribe(clientID, req.Symbol, types, gateway)

    snapshot := &BookSnapshot{...}
    e.MDPublisher.Publish(req.Symbol, MDSnapshot, snapshot, timestamp)  // ONLY SENT ONCE

    return Response{RequestID: req.RequestID, Success: true}
}
```

**Actors hack around this:**
```go
// slow_market_maker.go:137-143
func (smm *SlowMarketMakerActor) onBookDelta(delta actor.BookDeltaEvent) {
    if delta.Delta.VisibleQty == 0 {
        // HACK: Re-subscribe to get fresh snapshot
        smm.Subscribe(smm.config.Symbol)  // ← Abusing subscription API
    }
}
```

**Periodic snapshots exist but only log to files:**
```go
// exchange.go:162-183
func (e *Exchange) logSnapshots() {
    // Writes to logger files, NOT to market data streams
    log.LogEvent(timestamp, 0, "BookSnapshot", snapshotLog)  // clientID = 0 (no subscriber)
}
```

### Why This Matters

**Real trading systems need:**
1. **Initial synchronization**: Get current book state when joining market
2. **Continuous delta updates**: Stream every book change (already working)
3. **Periodic resync**: Full snapshots to prevent delta accumulation drift
4. **On-demand refresh**: Request snapshot when needed (e.g., after network glitch)

**Current system fails at #3 and #4**, forcing actors to abuse Subscribe() for refreshes.

---

## Interrogation: What Do We Actually Need?

### Use Cases

**UC1: Market Maker startup**
- Subscribe once → get snapshot + continuous deltas
- Requote based on book changes
- **Need**: Initial snapshot only

**UC2: Slow Market Maker (current problem)**
- Monitor BBO changes
- When best level consumed → need fresh BBO
- **Need**: On-demand snapshot refresh (NOT re-subscription)

**UC3: Statistical arbitrage**
- Track book depth over time
- Prevent delta drift accumulation
- **Need**: Periodic automatic snapshots (e.g., every 5 seconds)

**UC4: Multi-venue latency arbitrage**
- Sync orderbooks across venues
- Detect stale data
- **Need**: Sequence number checking + periodic snapshots

### Scale Constraints

- **Simulation scale**: 10-100 actors, 1-10 instruments
- **Snapshot cost**: O(book depth) = ~100 price levels = negligible
- **Network**: In-memory channels (no real network cost)
- **Trade-off**: Simplicity over micro-optimization

### Failure Modes

| Scenario | Current Behavior | Desired Behavior |
|----------|------------------|------------------|
| Actor misses delta | No recovery | Can request snapshot |
| Best level consumed | Re-subscribe hack | Request snapshot or get periodic |
| Long-running sim | Delta drift possible | Periodic snapshots prevent drift |
| Subscribe during active market | Snapshot + flood of deltas | Same (working) |

---

## Explore Alternatives

### Option A: Periodic Auto-Send (WebSocket-style)

**Design:**
```go
type Subscription struct {
    ClientID         uint64
    Symbol           string
    Types            []MDType
    SnapshotInterval time.Duration  // 0 = no periodic, >0 = auto-send every N
}

// Subscribe with periodic snapshots
actor.Subscribe(symbol, PeriodicSnapshots(5*time.Second))

// Under the hood: MDPublisher goroutine per symbol
func (p *MDPublisher) runSnapshotTicker(symbol string, interval time.Duration) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    for range ticker.C {
        p.mu.RLock()
        subs := p.subscriptions[symbol]
        p.mu.RUnlock()

        for clientID, sub := range subs {
            if sub.WantsPeriodic {
                p.sendSnapshot(symbol, clientID)
            }
        }
    }
}
```

**Pros:**
- Fully automatic, no actor code changes needed
- Prevents delta drift without actor intervention
- Mirrors real WebSocket APIs (Binance, Kraken)

**Cons:**
- Goroutine per symbol (10 goroutines for 10 symbols = trivial but not free)
- All subscribers get same interval (inflexible)
- Can't disable periodic snapshots per subscriber
- Wasted bandwidth if actor doesn't need periodic

**Complexity**: Medium
**Reversibility**: Easy (flag to disable)
**Team fit**: Straightforward goroutine + ticker pattern

---

### Option B: On-Demand Request (FIX-style)

**Design:**
```go
type RequestType uint8
const (
    ReqPlaceOrder RequestType = iota
    ReqCancelOrder
    ReqQueryBalance
    ReqSubscribe
    ReqUnsubscribe
    ReqRequestSnapshot  // ← NEW
)

// Actor requests snapshot when needed
actor.RequestSnapshot(symbol)

// Exchange handler
func (e *Exchange) requestSnapshot(clientID uint64, req *QueryRequest) Response {
    book := e.Books[req.Symbol]
    snapshot := &BookSnapshot{
        Bids: book.Bids.getSnapshot(),
        Asks: book.Asks.getSnapshot(),
    }
    e.MDPublisher.Publish(req.Symbol, MDSnapshot, snapshot, timestamp)
    return Response{RequestID: req.RequestID, Success: true}
}
```

**Pros:**
- Zero overhead when not needed
- Actor decides when to refresh (full control)
- No background goroutines
- Minimal code changes (add one request type)

**Cons:**
- Actors must explicitly request (more actor code)
- Doesn't solve periodic resync automatically
- Actors can abuse (spam snapshot requests)

**Complexity**: Low
**Reversibility**: Easy (just don't call it)
**Team fit**: Actors already use request/response pattern

---

### Option C: Hybrid (Best of Both)

**Design:**
```go
type Subscription struct {
    ClientID         uint64
    Symbol           string
    Types            []MDType
    SnapshotMode     SnapshotMode  // OnSubscribe, Periodic, OnDemand
    SnapshotInterval time.Duration // Only used if Periodic
}

type SnapshotMode uint8
const (
    SnapshotOnSubscribe SnapshotMode = iota  // Default: one snapshot on subscribe
    SnapshotPeriodic                          // Auto-send every interval
    SnapshotOnDemand                          // Only via RequestSnapshot()
)

// Flexible API
actor.Subscribe(symbol, SnapshotOnSubscribe)            // Current behavior
actor.Subscribe(symbol, SnapshotPeriodic(5*time.Second)) // Auto-send
actor.RequestSnapshot(symbol)                            // Manual refresh
```

**Pros:**
- Supports all use cases
- Backward compatible (default = OnSubscribe)
- Zero overhead for simple actors
- Periodic mode for sophisticated actors
- On-demand for edge cases

**Cons:**
- More API surface area
- Three code paths to maintain
- Risk of over-engineering

**Complexity**: Medium-High
**Reversibility**: Hard (once actors rely on modes, removing them breaks code)
**Team fit**: Requires clear documentation of when to use which mode

---

## Trade-off Matrix

| Criterion | Option A: Periodic Auto | Option B: On-Demand | Option C: Hybrid |
|-----------|-------------------------|---------------------|------------------|
| **Simplicity** | ★★★☆☆ (goroutines) | ★★★★★ (just add request) | ★★☆☆☆ (3 modes) |
| **Zero overhead** | ★☆☆☆☆ (always ticking) | ★★★★★ (only on call) | ★★★★☆ (per-actor choice) |
| **Prevents drift** | ★★★★★ (automatic) | ★★☆☆☆ (actor must remember) | ★★★★★ (if actor chooses) |
| **Flexibility** | ★★☆☆☆ (one interval) | ★★★★☆ (actor decides when) | ★★★★★ (all options) |
| **Backward compat** | ★★★☆☆ (need opt-in flag) | ★★★★★ (no breaking change) | ★★★★★ (default = current) |
| **Code complexity** | ★★★☆☆ (1 goroutine/symbol) | ★★★★★ (10 lines) | ★★☆☆☆ (3 code paths) |
| **Matches real exchanges** | ★★★★☆ (Binance-style) | ★★★★☆ (FIX-style) | ★★★★★ (all styles) |

---

## Recommendation: Option B + Lightweight Periodic (Option B.5)

**Why:**

1. **YAGNI principle**: Most actors don't need periodic snapshots
2. **Simplicity wins**: On-demand is 10 lines of code, Option C is 100+
3. **Actor control**: Library-first philosophy = actors decide, not library
4. **Easy to add periodic later**: If we find actors all implement periodic logic, we can promote it to library

**But add simple periodic support:**
- Allow actors to create their own ticker goroutine
- Provide helper: `actor.PeriodicSnapshot(symbol, interval)` (optional utility)
- Helper calls `RequestSnapshot()` on timer

**This gives:**
- ✅ Zero overhead for simple actors
- ✅ On-demand refresh (solves slow_market_maker hack)
- ✅ Optional periodic (for actors that want it)
- ✅ Library stays simple (core = on-demand, periodic = actor util)
- ✅ Easy to test (no hidden goroutines in library)

---

## Chosen Approach: On-Demand Snapshots with Actor-Level Periodic Helpers

### Architecture

**Core Library (exchange package):**
```
MDPublisher
├── Subscribe(clientID, symbol, types, gateway)
│   └── Sends ONE snapshot on subscribe (current behavior)
│
├── Publish(symbol, mdType, data, timestamp)
│   └── Broadcasts to all subscribers (current behavior)
│
└── RequestSnapshot(clientID, symbol, gateway) [NEW]
    └── Sends snapshot to ONE client on-demand
```

**Actor Framework (actor package):**
```
BaseActor
├── Subscribe(symbol)                       [existing]
├── RequestSnapshot(symbol)                 [NEW]
└── StartPeriodicSnapshots(symbol, interval) [NEW - optional helper]
    └── Goroutine that calls RequestSnapshot() on timer
```

**Data Flow:**
```
Actor startup:
  → Subscribe(symbol)
    → MDPublisher.Subscribe()
      → Send initial snapshot
      → Start streaming deltas/trades

Actor needs refresh:
  → RequestSnapshot(symbol)
    → Exchange.requestSnapshot()
      → MDPublisher.Publish(MDSnapshot, ...)
        → Actor receives snapshot

Actor wants periodic (optional):
  → StartPeriodicSnapshots(symbol, 5*time.Second)
    → Internal ticker goroutine
      → Calls RequestSnapshot() every 5s
```

### Components & Interfaces

**1. Exchange Request Handler (exchange/exchange.go)**
```go
// Add to RequestType enum in types.go
const (
    ReqPlaceOrder RequestType = iota
    ReqCancelOrder
    ReqQueryBalance
    ReqSubscribe
    ReqUnsubscribe
    ReqRequestSnapshot  // NEW
)

// Add handler in exchange.go
func (e *Exchange) requestSnapshot(clientID uint64, req *QueryRequest) Response {
    e.mu.RLock()
    defer e.mu.RUnlock()

    book := e.Books[req.Symbol]
    if book == nil {
        return Response{RequestID: req.RequestID, Success: false, Error: RejectUnknownInstrument}
    }

    // Check if client is subscribed (prevent spam)
    if !e.MDPublisher.IsSubscribed(clientID, req.Symbol) {
        return Response{RequestID: req.RequestID, Success: false, Error: RejectNotSubscribed}
    }

    snapshot := &BookSnapshot{
        Bids: book.Bids.getSnapshot(),
        Asks: book.Asks.getSnapshot(),
    }

    e.MDPublisher.Publish(req.Symbol, MDSnapshot, snapshot, e.Clock.NowUnixNano())

    return Response{RequestID: req.RequestID, Success: true}
}

// Add to handleClientRequests() switch
case ReqRequestSnapshot:
    resp = e.requestSnapshot(gateway.ClientID, req.QueryReq)
```

**2. MDPublisher Helper (exchange/marketdata.go)**
```go
// Add subscription check
func (p *MDPublisher) IsSubscribed(clientID uint64, symbol string) bool {
    p.mu.RLock()
    defer p.mu.RUnlock()

    if p.subscriptions[symbol] == nil {
        return false
    }
    _, exists := p.subscriptions[symbol][clientID]
    return exists
}
```

**3. BaseActor Request Method (actor/actor.go)**
```go
func (a *BaseActor) RequestSnapshot(symbol string) uint64 {
    reqID := atomic.AddUint64(&a.requestSeq, 1)
    req := exchange.Request{
        Type: exchange.ReqRequestSnapshot,
        QueryReq: &exchange.QueryRequest{
            RequestID: reqID,
            Symbol:    symbol,
        },
    }

    if a.gateway == nil {
        return reqID
    }

    a.gateway.Mu.Lock()
    if !a.gateway.Running {
        a.gateway.Mu.Unlock()
        return reqID
    }
    a.gateway.Mu.Unlock()

    select {
    case a.gateway.RequestCh <- req:
    default:
        // Gateway closed, silently drop
    }

    return reqID
}
```

**4. Optional Periodic Helper (actor/periodic_snapshot.go - NEW FILE)**
```go
package actor

import (
    "context"
    "time"
)

// PeriodicSnapshotConfig configures periodic snapshot requests
type PeriodicSnapshotConfig struct {
    Symbol   string
    Interval time.Duration
}

// StartPeriodicSnapshots starts a goroutine that requests snapshots on a timer.
// Returns a stop function to cancel the periodic requests.
//
// Usage:
//   stop := actor.StartPeriodicSnapshots(ctx, PeriodicSnapshotConfig{
//       Symbol:   "BTC/USD",
//       Interval: 5 * time.Second,
//   })
//   defer stop()
func (a *BaseActor) StartPeriodicSnapshots(ctx context.Context, cfg PeriodicSnapshotConfig) func() {
    stopCh := make(chan struct{})

    go func() {
        ticker := time.NewTicker(cfg.Interval)
        defer ticker.Stop()

        for {
            select {
            case <-ctx.Done():
                return
            case <-stopCh:
                return
            case <-ticker.C:
                a.RequestSnapshot(cfg.Symbol)
            }
        }
    }()

    return func() {
        close(stopCh)
    }
}
```

### Integration Points

**Modified files:**
- `exchange/types.go`: Add `ReqRequestSnapshot` to RequestType enum, `RejectNotSubscribed` to RejectReason
- `exchange/exchange.go`: Add `requestSnapshot()` handler, add case in `handleClientRequests()`
- `exchange/marketdata.go`: Add `IsSubscribed()` helper
- `actor/actor.go`: Add `RequestSnapshot()` method
- `actor/periodic_snapshot.go`: NEW - optional periodic snapshot helper

**New files:**
- `actor/periodic_snapshot.go`: Periodic snapshot helper
- `actor/periodic_snapshot_test.go`: Unit tests for periodic helper

---

## Implementation Phases

### Phase 1: Core On-Demand Snapshot (Critical Path)

**Goal**: Fix the re-subscription hack in slow_market_maker

**Tasks:**
1. Add `ReqRequestSnapshot` to `RequestType` enum
2. Add `RejectNotSubscribed` to `RejectReason` enum
3. Implement `Exchange.requestSnapshot()`
4. Add case to `handleClientRequests()` switch
5. Implement `MDPublisher.IsSubscribed()`
6. Implement `BaseActor.RequestSnapshot()`

**Validation:**
- Unit test: Request snapshot without subscription → RejectNotSubscribed
- Unit test: Request snapshot with subscription → receive snapshot
- Integration test: Subscribe → request snapshot → verify data matches current book
- Fix slow_market_maker.go to use `RequestSnapshot()` instead of re-subscribing

**Success Criteria:**
- `slow_market_maker.go` line 142 changes from `smm.Subscribe(...)` to `smm.RequestSnapshot(...)`
- All existing tests pass
- No re-subscription hacks remain in codebase

---

### Phase 2: Periodic Helper (Optional Enhancement)

**Goal**: Provide opt-in periodic snapshot utility

**Tasks:**
1. Create `actor/periodic_snapshot.go`
2. Implement `BaseActor.StartPeriodicSnapshots()`
3. Write unit tests for periodic helper
4. Document usage in LLMS.md

**Validation:**
- Unit test: Start periodic → verify requests sent at interval
- Unit test: Stop periodic → verify requests cease
- Unit test: Context cancel → verify goroutine exits

**Success Criteria:**
- Actor can call `StartPeriodicSnapshots()` and receive snapshots every N seconds
- Stop function cleanly terminates goroutine
- No goroutine leaks

---

### Phase 3: Migration & Cleanup (Validation)

**Goal**: Verify no actors abuse Subscribe for refresh

**Tasks:**
1. Grep codebase for Subscribe patterns
2. Audit all actors for re-subscription hacks
3. Update documentation (LLMS.md) with snapshot request patterns
4. Add logging/metrics for snapshot requests (optional)

**Validation:**
- `Grep` for `Subscribe` calls in event handlers → should find none
- All actors use `RequestSnapshot()` or `StartPeriodicSnapshots()`
- Documentation updated with examples

**Success Criteria:**
- No Subscribe calls in actor event loops (except startup)
- LLMS.md documents snapshot request patterns
- All tests pass

---

## Success Metrics

**Functional:**
- ✅ Actors can request snapshots on-demand
- ✅ No re-subscription hacks in codebase
- ✅ Periodic snapshots available as opt-in helper
- ✅ All existing tests pass

**Non-functional:**
- ✅ Zero performance overhead for actors not using periodic snapshots
- ✅ No goroutine leaks
- ✅ Clean separation: core library (on-demand) vs actor utils (periodic)

**API Quality:**
- ✅ `RequestSnapshot(symbol)` mirrors `Subscribe(symbol)` simplicity
- ✅ Periodic helper returns stop function (idiomatic cleanup)
- ✅ Error handling: RejectNotSubscribed prevents spam

---

## Risks & Mitigations

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| **Actors spam snapshot requests** | Performance degradation | Medium | Check subscription before sending, add rate limiting if needed |
| **Goroutine leaks in periodic helper** | Memory leak | Low | Context cancellation + explicit stop function |
| **Backward incompatible change** | Breaking actors | Low | On-demand is additive, no existing APIs changed |
| **Over-engineering periodic helper** | Complexity creep | Medium | Keep periodic as separate optional file, not core |
| **Actors still re-subscribe** | Hack not fixed | Low | Grep codebase, update all actors in Phase 3 |

**Mitigation for spam:**
```go
// Optional rate limiting (add if needed)
type Subscription struct {
    ClientID          uint64
    Symbol            string
    Types             []MDType
    LastSnapshotReq   int64  // Unix nano timestamp
    SnapshotReqLimit  int64  // Min time between requests (e.g., 100ms)
}

func (e *Exchange) requestSnapshot(...) Response {
    // Check rate limit
    sub := e.MDPublisher.subscriptions[req.Symbol][clientID]
    now := e.Clock.NowUnixNano()
    if now - sub.LastSnapshotReq < sub.SnapshotReqLimit {
        return Response{..., Error: RejectRateLimitExceeded}
    }
    sub.LastSnapshotReq = now
    // ... rest of handler
}
```

---

## Rollback Plan

**If on-demand snapshots break something:**

1. **Immediate**: Comment out `case ReqRequestSnapshot:` in `handleClientRequests()` → requests silently ignored
2. **Actors fail gracefully**: `RequestSnapshot()` already has no error handling (fire-and-forget), actors won't crash
3. **Revert slow_market_maker**: Change `RequestSnapshot()` back to `Subscribe()` hack
4. **Full rollback**: `git revert` commits from Phase 1

**If periodic helper leaks goroutines:**

1. **Immediate**: Stop using `StartPeriodicSnapshots()` in actors
2. **Actors implement own ticker**: Simple `time.Ticker` in actor event loop
3. **Delete helper**: Remove `actor/periodic_snapshot.go` if fundamentally broken

**Testing before merge:**
- Run simulation for 1 hour simulated time
- Monitor goroutine count: `pprof` goroutine profile before/after
- Memory profile: No leaks in periodic snapshot goroutines

---

## Questions for Review

1. **Is on-demand + optional periodic the right split?**
   - Alternative: Always periodic (Option A)
   - Trade-off: Simplicity vs automatic drift prevention

2. **Should RequestSnapshot require subscription?**
   - Current design: Yes (prevents spam from non-subscribers)
   - Alternative: Allow anyone to request (more flexible, more abusable)

3. **Rate limiting for snapshot requests?**
   - Current design: No rate limiting (trust actors)
   - Alternative: Min 100ms between requests per client

4. **Should periodic helper live in actor package or separate?**
   - Current design: `actor/periodic_snapshot.go` (actor utility)
   - Alternative: `actor/helpers/` subpackage (if we add more helpers)

5. **Backward compatibility: Keep EnablePeriodicSnapshots()?**
   - Current design: Keep for logging snapshots to files
   - Alternative: Remove and replace with actor-level periodic

---

## Related Documentation

- **LLMS.md**: Update "Data Flow: Market Data" section with RequestSnapshot pattern
- **CLAUDE.md**: Confirm design follows library-first philosophy (actors extend, library provides primitives)
- **BUILD.md**: No changes (no new external dependencies)

---

## Approval Checklist

- [ ] Design reviewed by team
- [ ] Trade-offs understood (on-demand vs periodic auto-send)
- [ ] Migration plan for slow_market_maker approved
- [ ] Success criteria agreed upon
- [ ] Rollback plan reviewed

**Next Step**: User selects approach, then hand off to `go-dev` for implementation.
