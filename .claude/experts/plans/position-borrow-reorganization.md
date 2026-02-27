# Position and Borrow Reorganization Plan

## Feature Goal

Break the circular / inverted dependency between `PositionManager`, `BorrowingManager`,
and `Exchange`. The current design has both managers holding a `*Exchange` pointer and
acquiring `exchange.mu` directly, making them untestable in isolation, and creating
subtle lock-ordering hazards. The goal is to invert those dependencies so:

1. `PositionManager` becomes an injectable interface — users can supply custom position
   tracking (e.g. backed by a real database) without touching library code.
2. `BorrowingManager` never touches `exchange.mu`; it receives already-validated client
   state via narrowly-scoped interfaces.
3. Lock ownership is unambiguous: `exchange.mu` is **the** exchange-wide lock; neither
   manager acquires it directly.
4. `Exchange.ForceClose` encapsulates liquidation order execution so `ExchangeAutomation`
   does not reach into exchange internals.

All existing behaviour is preserved. No new concepts are introduced beyond what already
exists implicitly.

---

## Exact Problems Found in the Code

### Problem 1 — Double-lock: PositionManager.mu vs exchange.mu

`settlement.go:processPerpExecution` calls
`e.Positions.UpdatePositionWithDelta(…, e, "trade")` while already holding
`exchange.mu.Lock()`. Inside `UpdatePositionWithDelta`, `pm.mu.Lock()` is acquired.
That is fine as written (Go allows acquiring a second distinct lock), but:

- `margin.go:hasOpenPositions` acquires `exchange.mu.Lock()` then
  `e.Positions.mu.RLock()` — lock order A→B.
- `automation.go:CheckLiquidations` acquires `exchange.mu.Lock()` then calls
  `e.Positions.GetPosition` which acquires `pm.mu.RLock()` — same A→B order.
- The **standalone** public API `GetPosition` / `CalculateOpenInterest` acquires
  only `pm.mu` (no exchange lock) — needed for external callers.

Currently there is no B→A inversion (no code acquires pm.mu first, then exchange.mu),
so there is no **actual deadlock today**. However `UpdatePositionWithDelta` receives
`*Exchange` and calls `exchange.getLogger()` — meaning it reads `e.Loggers` map while
`pm.mu` is held, after `exchange.mu` is already held. If any future code path acquires
`pm.mu` first, then `exchange.mu`, it will deadlock. The design invites this mistake.

The fix: remove `*Exchange` from `UpdatePositionWithDelta`. Pass in a `Logger` directly.
The `pm.mu` then protects only position state, never Logger state.

### Problem 2 — PositionManager owns logging it should not own

`UpdatePositionWithDelta` and `SettleFunding` both call `exchange.getLogger()` internally.
A position store should be a dumb ledger. Logging is the caller's responsibility.

The fix: return richer results from `UpdatePositionWithDelta`; the caller
(`settlement.go`) already has the logger and logs the events there. For `SettleFunding`,
accept a `FundingEventSink` interface instead of `*Exchange`.

### Problem 3 — BorrowingManager holds *Exchange, acquires exchange.mu directly

`BorrowMargin` and `RepayMargin` call `bm.exchange.mu.Lock()` directly.
`AutoBorrowForSpotTrade` calls `bm.exchange.mu.RLock()` then calls `BorrowMargin` which
tries to acquire `mu.Lock()` — this is a lock **upgrade** inside the same goroutine.
It happens to work because `tryReserveOrBorrow` explicitly unlocks `e.mu` before calling
`AutoBorrowForSpotTrade`, which then acquires `mu.RLock()`, then releases it, then calls
`BorrowMargin` which acquires `mu.Lock()`. So the sequence is:

```
placeOrder: mu.Lock()
tryReserveOrBorrow: mu.Unlock()
AutoBorrowForSpotTrade: mu.RLock() ... mu.RUnlock()
BorrowMargin: mu.Lock() ... mu.Unlock()
tryReserveOrBorrow: mu.Lock()   ← re-acquire after borrow
placeOrder: mu.Unlock() (deferred)
```

This is deliberately fragile. If anyone touches this ordering, or adds a code path that
skips the `mu.Unlock()` in `tryReserveOrBorrow`, the lock is acquired twice.

The fix: `BorrowingManager` receives a `BorrowContext` (containing already-resolved
client state) from the caller. The exchange locks once, reads client state, passes it in,
and `BorrowingManager` mutates client fields it receives — no lock acquisition inside
the manager.

### Problem 4 — ExchangeAutomation.liquidate reaches into exchange internals

`liquidate()` directly:
- Cancels orders by iterating `book.Bids.Orders` / `book.Asks.Orders`
- Calls `a.exchange.Matcher.Match(...)`
- Calls `a.exchange.processExecutions(...)`
- Mutates `a.exchange.ExchangeBalance.InsuranceFund`

This bypasses all the exchange's own validation and accounting guards. A public
`Exchange.ForceClose(clientID, symbol, side, qty)` method would perform the same
operations but through the proper code path, owned by the exchange.

### Problem 5 — PositionManager is a concrete type, not injectable

`Exchange.Positions` is `*PositionManager` — a concrete struct. There is no way for a
library user to substitute custom position tracking. The entire premise of
library-first design requires an interface here.

---

## Chosen Approach — Surgical Interface Extraction + Dependency Inversion

We do **not** move PositionManager to a separate package. The `types/` package already
has `Position`, `PositionDelta`, `PositionSide` etc. We add a `PositionStore` interface
to `types/interfaces.go`. The existing `PositionManager` struct stays in `exchange/`
and implements that interface. The `Exchange` struct field changes from
`*PositionManager` to `PositionStore`.

We do **not** restructure `BorrowingManager` into a separate package. Instead we remove
`*Exchange` from it and pass a `BorrowContext` struct from the exchange-level call sites.

We do **not** move logging outside the settlement path — the settlement functions
already have the logger; they just need to stop delegating that work to PositionManager.

### What does NOT change
- `PositionManager` internal algorithm (netting, hedge mode, VWAP entry)
- `SettleFunding` funding math
- All 38 integration tests (test API does not change)
- `Exchange.EnableBorrowing` signature
- `BorrowingConfig` struct
- Any public exchange method signatures (except `UpdatePositionWithDelta` which is
  internal to `exchange/`)

---

## Interface Definitions

### Addition to `types/interfaces.go`

```go
// PositionStore is the minimal interface for position tracking.
// Implement this to substitute custom position persistence.
type PositionStore interface {
    // UpdatePosition applies a trade delta and returns old/new state.
    // Logging is the caller's responsibility — the store only mutates state.
    UpdatePosition(clientID uint64, symbol string, qty, price int64,
        tradeSide Side, posSide PositionSide) PositionDelta

    // GetPosition returns a copy of the current position. Returns nil if none.
    GetPosition(clientID uint64, symbol string) *Position

    // GetPositionBySide returns a copy for a specific hedge side.
    GetPositionBySide(clientID uint64, symbol string, posSide PositionSide) *Position

    // CalculateOpenInterest returns the sum of absolute position sizes for symbol.
    CalculateOpenInterest(symbol string) int64

    // PositionsForFunding iterates over all non-zero positions for a symbol,
    // calling fn(clientID, position) for each one.
    // Used exclusively by SettleFunding; avoids exposing the raw map.
    PositionsForFunding(symbol string, fn func(clientID uint64, pos *Position))
}
```

Note: `UpdatePositionWithDelta` is renamed to `UpdatePosition` in the interface.
The `*Exchange` parameter is gone. The old `UpdatePosition` (the simpler version
without delta return) is merged in — the interface returns `PositionDelta` always.
The caller ignores it when they do not need it.

`PositionsForFunding` is a narrow iterator that exposes exactly what `SettleFunding`
needs without exposing the full internal map. This keeps the interface minimal and
avoids forcing implementors to provide a `GetAll()` that returns maps.

### Replacement `FundingEventSink` interface in `exchange/` (not types/)

This is exchange-internal. `SettleFunding` needs to log balance changes and record
exchange fee revenue. Rather than accepting `*Exchange`, it accepts two callbacks via
a small struct:

```go
// fundingEventSink captures the two side-effects SettleFunding needs from
// the exchange. Defined in exchange/ because it references exchange-internal
// types (Client, PerpFutures).
type fundingEventSink struct {
    logBalanceFn   func(timestamp, clientID int64, symbol, reason string, changes []BalanceDelta)
    recordRevenueFn func(asset string, amount int64)
}
```

This struct is unexported. `SettleFunding` moves to `exchange/settlement.go` as a
free function (or stays on `PositionManager` but takes `fundingEventSink` instead of
`*Exchange`). See Phase 2 for the implementation decision.

### BorrowContext — replaces *Exchange in BorrowingManager

```go
// BorrowContext is passed by the exchange into BorrowingManager per borrow/repay call.
// It carries already-resolved, mutable client state. The exchange holds the lock;
// BorrowingManager must not acquire any lock.
type BorrowContext struct {
    Client    *Client
    ClientID  uint64
    Timestamp int64
    // logBalanceFn routes to exchange.logBalanceChange under the held lock.
    LogBalanceFn   func(reason string, changes []BalanceDelta)
    // logEventFn routes to exchange.getLogger("_global").LogEvent.
    LogEventFn     func(clientID uint64, event string, data any)
    // recordRevenueFn routes to exchange.ExchangeBalance.FeeRevenue mutation.
    RecordRevenueFn func(asset string, amount int64)
}
```

`BorrowContext` is defined in `exchange/borrowing.go` (unexported helper fields,
exported struct only if needed for testing). The BorrowingManager methods become:

```go
func (bm *BorrowingManager) BorrowMargin(ctx BorrowContext, asset string, amount int64, reason string) error
func (bm *BorrowingManager) RepayMargin(ctx BorrowContext, asset string, amount int64) error
// AutoBorrow variants become private helpers called by the exchange
func (bm *BorrowingManager) autoBorrowIfNeeded(ctx BorrowContext, asset string, required int64, available int64) (bool, error)
```

The `AutoBorrowForSpotTrade` / `AutoBorrowForPerpTrade` public methods are removed.
Their callers (`tryReserveOrBorrow`) move the availability check inline.

---

## Dependency Injection Changes

### PositionManager

**Before:**
```go
// Exchange field
Positions *PositionManager

// settlement.go call site
takerDelta = e.Positions.UpdatePositionWithDelta(
    exec.TakerClientID, book.Symbol, exec.Qty, exec.Price,
    takerOrder.Side, takerOrder.PositionSide, e, "trade")

// funding.go method signature
func (pm *PositionManager) UpdatePositionWithDelta(
    clientID uint64, symbol string, qty, price int64,
    tradeSide Side, posSide PositionSide,
    exchange *Exchange, reason string) PositionDelta
```

**After:**
```go
// Exchange field
Positions PositionStore  // interface from types/

// settlement.go call site — logger already in scope
takerDelta = e.Positions.UpdatePosition(
    exec.TakerClientID, book.Symbol, exec.Qty, exec.Price,
    takerOrder.Side, takerOrder.PositionSide)
// caller logs position_update and open_interest events here using existing `log` var

// funding.go method signature
func (pm *PositionManager) UpdatePosition(
    clientID uint64, symbol string, qty, price int64,
    tradeSide Side, posSide PositionSide) PositionDelta
```

The `UpdatePositionWithDelta` name disappears. The existing `UpdatePosition` (which
returned nothing) is replaced by the new one (which returns `PositionDelta`).
The lock `pm.mu` is unchanged — it protects only the `positions` map.

### Logging after UpdatePosition

The `position_update` and `open_interest` log events currently fire inside
`UpdatePositionWithDelta`. After the change, `processPerpExecution` already holds the
logger (`log` variable). We add two logging calls there, using the returned `PositionDelta`:

```go
// settlement.go — processPerpExecution
takerDelta = e.Positions.UpdatePosition(...)
makerDelta = e.Positions.UpdatePosition(...)
logPositionUpdate(log, timestamp, exec.TakerClientID, book.Symbol, exec.Qty, exec.Price, takerOrder.Side, takerDelta)
logPositionUpdate(log, timestamp, exec.MakerClientID, book.Symbol, exec.Qty, exec.Price, exec.MakerSide, makerDelta)
logOpenInterest(log, timestamp, book.Symbol, e.Positions.CalculateOpenInterest(book.Symbol))
```

A new `logPositionUpdate` helper follows the single-helper-per-event pattern from the
coding guidelines.

### SettleFunding

**Before:**
```go
func (pm *PositionManager) SettleFunding(clients map[uint64]*Client, perp *PerpFutures, exchange *Exchange)
```

`SettleFunding` iterates `pm.positions` directly — it is on `PositionManager` because
it needs internal access to the positions map. That is fine but it reaches into `*Exchange`
for logging.

**After:**
```go
func (pm *PositionManager) SettleFunding(clients map[uint64]*Client, perp *PerpFutures, sink fundingEventSink)
```

`sink` is built by the caller (`automation.go:CheckAndSettleFunding`) just before
the call, using captured exchange state:

```go
// automation.go
sink := fundingEventSink{
    logBalanceFn: func(ts, cid int64, sym, reason string, changes []BalanceDelta) {
        logBalanceChange(a.exchange, ts, uint64(cid), sym, reason, changes)
    },
    recordRevenueFn: func(asset string, amount int64) {
        a.exchange.ExchangeBalance.FeeRevenue[asset] += amount
    },
}
a.exchange.Positions.SettleFunding(a.exchange.Clients, perp, sink)
```

Because `PositionStore` is an interface, `SettleFunding` cannot be on the interface
(it exposes internal map iteration). It stays as a concrete method on `*PositionManager`
OR moves to a free function in `exchange/funding.go` that accepts `PositionStore`.

Decision: keep it as a concrete method on `*PositionManager`. The `PositionsForFunding`
interface method is used by any alternative implementation. `SettleFunding` as a
free function that calls `store.PositionsForFunding(...)` is the extensible path but
adds complexity we do not need yet. For now: `SettleFunding` stays on
`*PositionManager`, the exchange holds a `PositionStore` interface for the
`UpdatePosition` / `GetPosition` calls, and `SettleFunding` is called via a type
assertion only inside `automation.go` IF the concrete type implements it. That is
ugly. Better: move `SettleFunding` out of `PositionManager` entirely:

**Final decision:**

```go
// exchange/funding.go — free function, no longer on PositionManager
func settleFunding(store PositionStore, clients map[uint64]*Client, perp *PerpFutures,
    clock Clock, sink fundingEventSink)
```

`settleFunding` calls `store.PositionsForFunding(perp.Symbol(), fn)` to iterate.
This way the `PositionStore` interface is self-contained and `SettleFunding` works
with any implementation. The `PositionManager.SettleFunding` method is removed;
`automation.go` calls `settleFunding(e.Positions, …)`.

### BorrowingManager

**Before:**
```go
func NewBorrowingManager(exchange *Exchange, config BorrowingConfig) *BorrowingManager
// methods acquire bm.exchange.mu directly
```

**After:**
```go
func NewBorrowingManager(config BorrowingConfig) *BorrowingManager
// methods accept BorrowContext instead of self-locking
```

`EnableBorrowing` in `exchange.go` changes to:
```go
e.BorrowingMgr = NewBorrowingManager(config)
```

`tryReserveOrBorrow` in `order_handling.go` changes to build a `BorrowContext` inline
and call the new API. The unlock/relock dance **disappears** because `BorrowingManager`
no longer acquires any lock:

```go
// order_handling.go — NEW tryReserveOrBorrow
func (e *Exchange) tryReserveOrBorrow(
    clientID uint64, asset string, amount int64,
    reserveFn func(string, int64) bool,
    isPerp bool,
) bool {
    if reserveFn(asset, amount) {
        return true
    }
    if e.BorrowingMgr == nil {
        return false
    }
    client := e.Clients[clientID]
    var available int64
    if isPerp {
        available = client.PerpAvailable(asset)
    } else {
        available = client.GetAvailable(asset)
    }
    if available >= amount {
        return false  // shouldn't happen since reserveFn just failed, but be safe
    }
    shortfall := amount - available
    ctx := buildBorrowContext(e, client, clientID)
    if err := e.BorrowingMgr.BorrowMargin(ctx, asset, shortfall, borrowReason(isPerp)); err != nil {
        return false
    }
    return reserveFn(asset, amount)
}
// e.mu is held throughout — no unlock/relock needed
```

`BorrowMargin` and `RepayMargin` mutate `ctx.Client` directly (since the caller holds
the lock and passed in a pointer). They call `ctx.LogBalanceFn` and `ctx.LogEventFn`
instead of reaching into exchange.

---

## Lock Ownership Clarification

After the changes, the invariant is simple:

| Resource | Protected by |
|---|---|
| `exchange.Clients`, `exchange.Books`, `exchange.Instruments`, `exchange.ExchangeBalance` | `exchange.mu` |
| `PositionManager.positions` map | `pm.mu` |
| Neither manager acquires the other's lock | enforced by removing cross-references |

**Lock ordering when both are needed:**

Always acquire `exchange.mu` first, then `pm.mu` (if needed at all). Currently the
only code paths that hold both are:
- `settlement.go:processPerpExecution` — holds `exchange.mu`, calls `pm.UpdatePosition`
  which acquires `pm.mu` internally.
- `order_handling.go:buildPositionSnapshots` — acquires `exchange.mu.RLock()`, then
  `pm.mu.RLock()`.

These both follow A (exchange.mu) → B (pm.mu) order. After the change, no code holds
B first, then A. The invariant is documented in a comment on `Exchange`.

**pm.mu necessity after change:**

`pm.mu` is still needed because `GetPosition` / `CalculateOpenInterest` /
`PositionsForFunding` can be called from external goroutines (e.g. actor strategies)
without holding `exchange.mu`. The lock is correct and minimal. Do NOT remove it.

---

## Exchange.ForceClose Signature

```go
// ForceClose immediately closes clientID's position in symbol on the given side.
// qty must be the full position size (obtained from GetPosition before calling).
// Called by ExchangeAutomation.liquidate; may also be used by custom liquidators.
// Caller must hold e.mu.Lock().
func (e *Exchange) forceClose(clientID uint64, symbol string, side Side, qty int64, timestamp int64)
```

Initially unexported (`forceClose`) because the only caller is `liquidate()` which
already holds `exchange.mu`. If users need to trigger external liquidations, expose as
`ForceClose` with its own lock acquisition.

`forceClose` implementation extracts the order-cancellation and market-order-execution
logic currently embedded in `ExchangeAutomation.liquidate`:

```go
func (e *Exchange) forceClose(clientID uint64, symbol string, side Side, qty, timestamp int64) []*Execution {
    book := e.Books[symbol]
    if book == nil {
        return nil
    }
    e.cancelClientOrdersOnBook(clientID, book) // extracted helper
    orderID := e.NextOrderID
    e.NextOrderID++
    order := buildLiquidationOrder(clientID, orderID, side, qty, timestamp)
    result := e.Matcher.Match(book.Bids, book.Asks, order)
    e.processExecutions(book, result.Executions, order)
    putOrder(order)
    return result.Executions
}
```

`ExchangeAutomation.liquidate` reduces to:

```go
func (a *ExchangeAutomation) liquidate(clientID uint64, client *Client, symbol string, pos *Position, perp *PerpFutures, timestamp int64) {
    closeSide := Sell
    if pos.Size < 0 {
        closeSide = Buy
    }
    executions := a.exchange.forceClose(clientID, symbol, closeSide, abs(pos.Size), timestamp)
    fillPrice := lastFillPrice(executions)
    a.handlePostLiquidation(clientID, client, symbol, pos, perp, fillPrice, timestamp)
}
```

`handlePostLiquidation` handles borrow repayment, insurance fund, and event emission.

---

## File-by-File Change Summary

### `types/interfaces.go`
- Add `PositionStore` interface (7 methods as defined above)

### `exchange/funding.go`
- Remove `UpdatePositionWithDelta` method — replaced by `UpdatePosition` (renamed +
  simplified)
- Rename existing `UpdatePosition` (no-return) to `UpdatePosition` with `PositionDelta`
  return — merge the two methods
- Remove `*Exchange` parameter from `UpdatePositionWithDelta` entirely
- Remove `SettleFunding` method from `PositionManager`
- Add `PositionsForFunding` method on `PositionManager` to satisfy `PositionStore`
  interface
- Add free function `settleFunding(store PositionStore, clients map[uint64]*Client,
  perp *PerpFutures, clock Clock, sink fundingEventSink)` using `PositionsForFunding`
- Add `fundingEventSink` struct (unexported)
- Keep `realizedPerpPnL` as-is (free function, correct place)
- Keep `abs` as-is

### `exchange/settlement.go`
- In `processPerpExecution`: replace both `UpdatePositionWithDelta(…, e, "trade")`
  calls with `UpdatePosition(…)` (no exchange param)
- After the two `UpdatePosition` calls, add `logPositionUpdate` calls using the
  returned deltas and the already-in-scope `log Logger`
- Add `logPositionUpdate(log Logger, …)` helper (unexported, follows coding guidelines)
- Add `logOpenInterest(log Logger, …)` helper (DRY)

### `exchange/borrowing.go`
- Remove `exchange *Exchange` field from `BorrowingManager`
- Change `NewBorrowingManager` to not take `*Exchange`
- Add `BorrowContext` struct (exported for testability)
- Rewrite `BorrowMargin(ctx BorrowContext, …)` — mutates `ctx.Client` directly, calls
  `ctx.LogBalanceFn` / `ctx.LogEventFn`
- Rewrite `RepayMargin(ctx BorrowContext, …)` — same pattern
- Remove `AutoBorrowForSpotTrade` and `AutoBorrowForPerpTrade` public methods
- Add unexported `buildBorrowContext(e *Exchange, client *Client, clientID uint64) BorrowContext`
  helper in `exchange/helpers.go`

### `exchange/order_handling.go`
- Rewrite `tryReserveOrBorrow` — remove the `mu.Unlock()` / `mu.Lock()` pair;
  build `BorrowContext` inline, call new `BorrowMargin` API
- Add `borrowReason(isPerp bool) string` helper

### `exchange/exchange.go`
- Change field: `Positions *PositionManager` → `Positions PositionStore`
- `EnableBorrowing`: change `NewBorrowingManager(e, config)` → `NewBorrowingManager(config)`
- `NewExchangeWithConfig`: `Positions: NewPositionManager(config.Clock)` stays as-is
  (the concrete type still implements the interface)

### `exchange/margin.go`
- `hasOpenPositions`: change `e.Positions.mu.RLock()` / `e.Positions.positions[…]`
  to use `GetPositionBySide` or add `HasOpenPositions(clientID uint64) bool` to
  `PositionStore` interface (preferred — the current code accesses pm internals through
  Exchange which breaks the interface abstraction)

  Add `HasOpenPositions(clientID uint64) bool` to `PositionStore` and implement on
  `*PositionManager`.

### `exchange/order_handling.go` (`buildPositionSnapshots`)
- `buildPositionSnapshots` currently accesses `e.Positions.mu.RLock()` and
  `e.Positions.positions[clientID]` directly — breaks the interface.
- Add `GetAllPositions(clientID uint64) []*Position` to `PositionStore` interface
  (returns a snapshot slice, no lock needed by caller), implement on `*PositionManager`.
- Rewrite `buildPositionSnapshots` to call `e.Positions.GetAllPositions(clientID)`.

### `exchange/automation.go`
- `CheckAndSettleFunding`: replace `e.Positions.SettleFunding(clients, perp, e)` with
  `settleFunding(e.Positions, clients, perp, e.Clock, buildFundingSink(e))`
- Add `buildFundingSink(e *Exchange) fundingEventSink` helper (in `exchange/helpers.go`)
- `liquidate`: refactor to call `e.forceClose(…)` then `handlePostLiquidation(…)`
- `CheckLiquidations`: replace `a.exchange.Positions.GetPosition(…)` — this already
  goes through the interface method, no change needed

### `exchange/helpers.go`
- Add `buildBorrowContext(e *Exchange, client *Client, clientID uint64) BorrowContext`
- Add `buildFundingSink(e *Exchange) fundingEventSink`
- Add `borrowReason(isPerp bool) string`

---

## Revised PositionStore Interface (complete, after incorporating margin.go and order_handling.go findings)

```go
// types/interfaces.go

type PositionStore interface {
    UpdatePosition(clientID uint64, symbol string, qty, price int64,
        tradeSide Side, posSide PositionSide) PositionDelta

    GetPosition(clientID uint64, symbol string) *Position
    GetPositionBySide(clientID uint64, symbol string, posSide PositionSide) *Position

    HasOpenPositions(clientID uint64) bool

    CalculateOpenInterest(symbol string) int64

    // PositionsForFunding calls fn for every non-zero position for symbol.
    // fn must not modify the position; it receives a read-only copy.
    PositionsForFunding(symbol string, fn func(clientID uint64, pos Position))

    // GetAllPositions returns a snapshot of all non-zero positions for clientID.
    GetAllPositions(clientID uint64) []Position
}
```

Note: `PositionsForFunding` and `GetAllPositions` pass `Position` by value (not pointer)
to prevent callers from accidentally mutating internal state.

---

## Implementation Phases

### Phase 1 — PositionStore interface + PositionManager conforms to it (no behaviour change)

1. Add `PositionStore` to `types/interfaces.go`
2. Add the alias `type PositionStore = etypes.PositionStore` to `exchange/types.go`
3. Add `HasOpenPositions`, `PositionsForFunding`, `GetAllPositions` to `PositionManager`
4. Rename `UpdatePositionWithDelta` → `UpdatePosition` (with delta return), remove old
   `UpdatePosition` (no return)
5. Remove `*Exchange` parameter from the method
6. Update `settlement.go` call sites: add inline log calls after each `UpdatePosition`
7. Update `margin.go:hasOpenPositions` to use `HasOpenPositions`
8. Update `order_handling.go:buildPositionSnapshots` to use `GetAllPositions`
9. Change `Exchange.Positions` field type to `PositionStore`
10. Run `make test` — all 38 tests must pass

**Validation:** `make test` green. No change in log output (logging moves to caller,
same events emitted). `exchange.Positions` can be replaced with a mock in new tests.

### Phase 2 — SettleFunding moved to free function

1. Add `fundingEventSink` struct to `exchange/funding.go`
2. Add free function `settleFunding(store PositionStore, …, sink fundingEventSink)`
3. Remove `SettleFunding` from `PositionManager`
4. Update `automation.go:CheckAndSettleFunding` to call `settleFunding(...)`
5. Add `buildFundingSink` helper to `exchange/helpers.go`
6. Run `make test`

**Validation:** `make test` green. Funding settlement events unchanged.

### Phase 3 — BorrowingManager dependency inversion

1. Add `BorrowContext` struct to `exchange/borrowing.go`
2. Rewrite `BorrowMargin` and `RepayMargin` to use `BorrowContext`
3. Remove `AutoBorrowForSpotTrade` / `AutoBorrowForPerpTrade`
4. Remove `exchange *Exchange` from `BorrowingManager`
5. Change `NewBorrowingManager` signature
6. Update `exchange.go:EnableBorrowing`
7. Rewrite `tryReserveOrBorrow` in `order_handling.go` — remove unlock/relock
8. Add `buildBorrowContext` and `borrowReason` helpers
9. Run `make test`

**Validation:** `make test` green. Lock acquisition path simplified (no unlock/relock).

### Phase 4 — ForceClose extraction

1. Add `forceClose` method on `Exchange` in `exchange/exchange.go` or new
   `exchange/liquidation.go`
2. Add `cancelClientOrdersOnBook` helper (extracted from `liquidate`)
3. Add `handlePostLiquidation` helper (extracted from `liquidate`)
4. Refactor `automation.go:liquidate` to use `forceClose`
5. Run `make test`

**Validation:** `make test` green. Liquidation events and insurance fund mutations
unchanged.

---

## Migration: Tests that Need to Change

After examining `tests/`, the only tests that access `exchange.Positions` directly are
those that call:
- `e.Positions.InjectPosition(…)` — used in `tests/perp_margin_test.go` and similar

`InjectPosition` is a testing helper on `*PositionManager`. Since `Exchange.Positions`
becomes a `PositionStore` interface, tests cannot call `InjectPosition` without a type
assertion. Two options:

**Option A (preferred):** Add `InjectPosition` to the interface as an optional test
method. Rejected — pollutes the production interface.

**Option B:** Tests do a type assertion:
```go
pm := ex.Positions.(*exchange.PositionManager)
pm.InjectPosition(...)
```

**Option C:** Add a separate `TestPositionStore` interface in `tests/` that embeds
`PositionStore` and adds `InjectPosition`. Tests check if the store satisfies it.

**Decision: Option B.** The type assertion is explicit and honest — only tests that
know they are using the default implementation call it. No production interface
pollution.

The concrete type `*PositionManager` remains exported so the type assertion works.
`InjectPosition`, `GetPositions`, `Lock`, `Unlock`, `Abs` remain on the concrete type.

Tests that directly access `e.Positions.mu` (none found — `margin.go` accesses it but
is not a test file) or `e.Positions.positions` (same): these are already fixed in
Phase 1 step 6-8 above.

Grep confirms: no test file accesses `e.Positions.mu` or `e.Positions.positions`
directly. The only test-facing API is `InjectPosition`, `GetPositions`, and the
public read methods.

---

## Risks and Mitigations

| Risk | Likelihood | Mitigation |
|---|---|---|
| Type assertion `*PositionManager` fails if user swaps the store | Low (tests run against default) | Document in InjectPosition comment |
| `PositionsForFunding` callback exposes position copies — performance cost | Low (funding settles infrequently, client count is small) | Copy is deliberate; hot path is trading, not funding |
| Removing unlock/relock in `tryReserveOrBorrow` changes timing for auto-borrow | Medium | Existing borrowing tests (`tests/borrowing_test.go`) cover this path; run with `-race` |
| `fundingEventSink` closures capture `*Exchange` — back to square one | Medium | Closures are defined in `automation.go` which already holds `*Exchange`; they just call already-correct exchange methods. No new coupling introduced. |
| `GetAllPositions` allocates a slice on every `queryAccount` | Low | Already allocates today (`buildPositionSnapshots` builds a slice); no regression |

---

## Rollback Plan

Each phase is independently mergeable and the tests pass after each. If Phase 3
(BorrowingManager) introduces regressions, revert to Phase 2 state. The earlier
phases (interface extraction) are additive and non-breaking and need not be rolled back.

Phase 4 (ForceClose) is the highest-risk refactor. If it breaks liquidation behaviour,
revert and keep the existing `liquidate` implementation. The earlier three phases are
fully independent of Phase 4.

---

## Critical Path (what blocks what)

```
Phase 1 (PositionStore interface)
    |
    +-- Phase 2 (SettleFunding free fn) -- can be done in parallel with Phase 3
    |
    +-- Phase 3 (BorrowingManager inversion)
            |
            +-- Phase 4 (ForceClose) -- depends on Phase 3 only for clean exchange state
```

Phase 1 must land first. Phases 2 and 3 are independent of each other.
Phase 4 depends on nothing in Phase 2 or 3 architecturally, but practically it is
easiest to do last since Phase 3 clarifies lock ownership.

---

## Success Criteria

- `make test` passes (all 38 integration tests) after each phase
- `make test-race` passes (no data races) after Phase 3
- `exchange.Positions` field is typed as `PositionStore` (interface)
- `BorrowingManager` has no `*Exchange` field
- `tryReserveOrBorrow` contains no `e.mu.Unlock()` / `e.mu.Lock()` pair
- `ExchangeAutomation.liquidate` contains no direct `book.Bids.Orders` iteration
- A user can substitute a custom `PositionStore` without modifying any library file
