# Account Design Fixes Plan

## Feature Goal

Fix three structural defects in the `Client` and `Exchange` account model:

1. `MakerVolume` / `TakerVolume` — single `int64` fields accumulate cross-asset notionals,
   producing semantically meaningless numbers when a client trades on multiple markets.

2. `MarginMode` — a global per-client field, but real exchanges support per-symbol margin mode.
   The current design blocks position-mode changes whenever ANY position is open, which is
   unrealistic. The isolated mode path also never connects to order placement — placing a perp
   order in "isolated" mode today silently behaves as cross.

3. `map[string]Instrument` key semantics — symbol strings are the map key. The current API
   exposes the raw map, requiring callers to know the exact key format. `ListInstruments`
   exists but is underused.

---

## Codebase State as Read (February 2026)

The previous refactoring plan (`position-borrow-reorganization.md`) has already been
implemented. Confirmed state:

- `exchange.Positions` is typed as `PositionStore` (interface). Done.
- `BorrowingManager` has no `*Exchange` field; uses `BorrowContext`. Done.
- `tryReserveOrBorrow` contains no unlock/relock. Done.
- `forceClose` is a method on `*Exchange` in `exchange/liquidation.go`. Done.
- `settleFunding` is a free function in `exchange/funding.go`. Done.

The three problems in this plan are genuinely independent of the previous work and were
explicitly deferred from it.

---

## Problem 1 — `MakerVolume` / `TakerVolume`

### Root Cause

`settlement.go:handleExecution` computes:

```go
notional := (exec.Price * exec.Qty) / basePrecision
taker.TakerVolume += notional
maker.MakerVolume += notional
```

`notional` is in the quote asset of the instrument. For BTC/USD, that is USD. For ETH/BTC,
that is BTC. A client trading both markets accumulates USD-notional and BTC-notional in the
same `int64`, which has no interpretable unit.

The only consumer in the test suite is:

```go
// gateway_integration_test.go
if client1.MakerVolume == 0 { ... }  // non-zero check only
if client2.TakerVolume == 0 { ... }  // non-zero check only
```

The fields are not used anywhere in production logic. They exist only to expose a hook for
fee-tier progression — but that hook is never actually consumed.

### Real Exchange Behavior

Binance, OKX, and Bybit track volume per quote asset (or in a normalized USD equivalent).
Fee tier calculations use 30-day rolling USD volume, not raw notional in mixed currencies.

The library cannot compute "USD-equivalent volume" without a price oracle and a normalization
currency — both of which are user-supplied concepts. Embedding this in the library would be
overreach.

### Alternatives Considered

**Option A: `map[string]int64` keyed by quote asset**

```go
MakerVolumeByAsset map[string]int64  // "USD" -> 5_000_000_000
TakerVolumeByAsset map[string]int64
```

Pro: truthful about what the number means. Con: still accumulates in the library without the
user being able to influence normalization. The library now manages an extra map per client
per trade. Marginally better but still wrong by design.

**Option B: `map[string]int64` keyed by symbol**

```go
MakerVolumeBySymbol map[string]int64
TakerVolumeBySymbol map[string]int64
```

Same as A but at finer granularity. Higher memory cost. Same design problem.

**Option C: Remove fields, let users observe volume via Logger (recommended)**

The `Logger` interface already receives `OrderFill` events with `qty` and `price` in them.
Users who want volume statistics subscribe to fill events and compute whatever aggregation
they need (rolling windows, USD normalization, per-symbol breakdown). This is already
possible and requires zero changes to the library.

This is the correct answer under library-first design: the library emits events; consumers
decide what to track.

**Option D: Track at OrderBook level**

Add `TotalVolume`, `TakerVolume` to `OrderBook`. These are per-symbol and per-asset (quote).
Useful for market statistics (24h volume tickers). Does not solve the per-client problem.
Can be added later as a separate orthogonal feature.

### Recommended Solution: Option C — Remove

Delete `MakerVolume` and `TakerVolume` from `Client`. Delete the two increment lines in
`handleExecution`. Update the one test that checks `MakerVolume != 0` to verify the fill
event or post-trade balance instead.

This is a breaking API change, but the fields carry no semantic meaning today, so nothing
real breaks. The test must be updated.

---

## Problem 2 — `MarginMode` Global Per Client

### Root Cause

```go
// client.go
MarginMode MarginMode  // one value for all symbols

// margin.go
func (e *Exchange) SetMarginMode(clientID uint64, mode MarginMode) error {
    if e.hasOpenPositions(client) {
        return errors.New("cannot change margin mode with open positions")
    }
    client.MarginMode = mode
}

// margin.go
func (e *Exchange) AllocateCollateralToPosition(...) error {
    if client.MarginMode != IsolatedMargin {
        return errors.New("client not in isolated margin mode")
    }
    ...
}

// borrowing.go
if ctx.Client.MarginMode == CrossMargin { /* cross validation */ }
```

Three problems compound:

1. A client with BTC-PERP (cross) and ETH-PERP (isolated) is impossible today.
2. You cannot switch BTC-PERP to isolated while holding an ETH-PERP position.
3. Order placement ignores `MarginMode` entirely for isolated mode. `tryReserveOrBorrow` and
   `reserveLimitOrderFunds` draw from the global `PerpBalances` whether the mode is cross or
   isolated. The `IsolatedPositions` map is populated by `AllocateCollateralToPosition` but
   is never consulted during order placement or settlement. Isolated mode is stub
   infrastructure, not a working feature.

### Real Exchange Behavior (Binance/OKX)

1. Each symbol has an independent margin mode setting per client.
2. Switching is blocked only if that specific symbol has open positions or open orders.
3. When a symbol is in isolated mode, orders for that symbol must be funded from the
   isolated collateral pool, not the cross pool.
4. Liquidation for an isolated position only liquidates that position, not others.

### Alternatives Considered

**Option A: Move `MarginMode` to per-symbol config on `Client` as `map[string]MarginMode`**

```go
// client.go
SymbolMarginModes map[string]MarginMode  // "BTC-PERP" -> IsolatedMargin

// margin.go
func (e *Exchange) SetMarginMode(clientID uint64, symbol string, mode MarginMode) error {
    // block only if symbol has open position or open orders
}
```

This is the real-exchange model. It requires:
- Updating `SetMarginMode` to accept a `symbol` parameter (breaking API change)
- Updating `AllocateCollateralToPosition` validation to use symbol-specific mode
- Updating borrowing validation in `BorrowMargin` to use symbol-specific mode
- Updating `buildPositionSnapshots` to populate `MarginType` correctly

**Option B: Keep global mode but add per-symbol override map**

A hybrid: `MarginMode` stays as the default, with an optional `SymbolMarginModes` map that
overrides per-symbol. This preserves backward compatibility but creates a confusing dual-path
logic. Rejected.

**Option C: Keep global mode, fix `hasOpenPositions` guard to be per-symbol**

Partial improvement. Still prevents multi-symbol isolated portfolios. Rejected.

**Option D: Fully implement isolated mode mechanics**

The real gap is not just the mode tracking — it is that isolated mode is non-functional (the
collateral pool is never consulted during order placement). A full implementation requires:
- Per-symbol mode tracking (Option A)
- `reserveLimitOrderFunds` to check `IsolatedPositions` collateral when symbol is isolated
- `settlePerpSide` to debit from isolated collateral, not global `PerpBalances`
- `buildPositionSnapshots` to populate `MarginType` and `IsolatedMargin` correctly
- `CheckLiquidations` to use isolated collateral for margin ratio of isolated positions

This is a large feature, not a bug fix.

### Recommended Solution: Two-Phase Approach

**Phase 2a — Fix the API shape (breaking but correct)**

Change `SetMarginMode` to accept `symbol string`:

```go
func (e *Exchange) SetMarginMode(clientID uint64, symbol string, mode MarginMode) error
```

Change `Client.MarginMode` to `SymbolMarginModes map[string]MarginMode`. Add a helper:

```go
func (c *Client) MarginModeFor(symbol string) MarginMode {
    if mode, ok := c.SymbolMarginModes[symbol]; ok {
        return mode
    }
    return CrossMargin  // default
}
```

Update `SetMarginMode` guard to block only if that symbol has open positions
(call `e.Positions.GetPosition(clientID, symbol)` — if non-nil and size != 0, reject).

Update `AllocateCollateralToPosition` validation:
```go
if client.MarginModeFor(symbol) != IsolatedMargin { return error }
```

Update `BorrowMargin` in `borrowing.go`:
```go
if ctx.Client.MarginModeFor(symbol) == CrossMargin { /* cross validation */ }
```

Wait — `BorrowContext` currently has no `symbol` field. Add it:
```go
type BorrowContext struct {
    Client    *Client
    ClientID  uint64
    Timestamp int64
    Symbol    string  // instrument symbol for per-symbol mode lookup
    LogBalance func(...)
    LogEvent   func(...)
}
```

Update `buildBorrowContext` to accept and pass the symbol.

Update `buildPositionSnapshots` to read `client.MarginModeFor(pos.Symbol)` instead of
hardcoding `CrossMargin`.

Update `NewClient` to initialize `SymbolMarginModes`:
```go
SymbolMarginModes: make(map[string]MarginMode),
```

**Phase 2b — Wire isolated collateral to order placement (larger feature, deferred)**

The isolated collateral pool mechanics (fund orders from `IsolatedPositions.Collateral`,
settle PnL against isolated pool, liquidate per-position) are a full feature. They should
be a separate plan. Phase 2a makes the API correct without implementing the mechanics.

The `IsolatedPositions` map stays. It is populated by `AllocateCollateralToPosition` but
is not yet consulted during settlement. Document this limitation clearly in the field comment.

### Migration Impact for Phase 2a

Files to change:
- `exchange/client.go` — replace `MarginMode MarginMode` with `SymbolMarginModes map[string]MarginMode`; add `MarginModeFor`
- `exchange/margin.go` — `SetMarginMode` new signature; guard uses symbol-specific position; `AllocateCollateralToPosition` uses `MarginModeFor`
- `exchange/borrowing.go` — `BorrowMargin` uses `ctx.Client.MarginModeFor(ctx.Symbol)`
- `exchange/helpers.go` — `buildBorrowContext` gains `symbol` parameter
- `exchange/order_handling.go` — `tryReserveOrBorrow` passes symbol to `buildBorrowContext`
- `exchange/order_handling.go:buildPositionSnapshots` — uses `MarginModeFor(pos.Symbol)`
- `types/account.go` — no change (IsolatedPosition stays)
- `tests/margin_mode_test.go` — all tests must pass a symbol to `SetMarginMode`

The old `SetMarginMode(clientID, mode)` signature is a breaking change. The test file
already exercises the current API; all tests there will need the new symbol parameter.

---

## Problem 3 — `map[string]Instrument` Key Semantics

### Root Cause Assessment

After reading the full codebase, this is a documentation and discoverability concern, not a
functional defect. The symbol string (e.g. `"BTC/USD"`, `"BTC-PERP"`) is a natural primary
key for instruments. The map operations are correct.

The actual concerns worth addressing:

**Concern 3a: Raw map is publicly accessible**

```go
type Exchange struct {
    Instruments map[string]Instrument  // exported: users can write to this
    Books       map[string]*OrderBook  // same
}
```

Users can bypass `AddInstrument` and insert directly into the map. `AddInstrument` sets up
the `OrderBook` and acquires the lock — direct insertion would bypass both.

**Concern 3b: `ListInstruments` is the safe API, but the map is more prominent**

`ListInstruments(baseFilter, quoteFilter string)` exists and is the correct read API. But
`exchange.Instruments["BTC/USD"]` is easier to type in tests and usage. This is a library
design smell — the map is implementation detail that leaked public.

**Concern 3c: No `GetInstrument(symbol string)` accessor**

Callers either use the raw map or scan `ListInstruments`. A `GetInstrument(symbol string) (Instrument, bool)` method is the missing safe read path.

**Concern 3d: Multi-leg instruments**

Considered but not a current concern. The exchange is single-symbol-per-book. Calendar
spreads would require a new instrument type with a composite symbol (e.g. `"BTC-PERP-240329"`).
The symbol-as-key design already handles this — no design change needed.

### Real Exchange Behavior

Exchanges expose an instrument lookup API (`/api/v3/exchangeInfo` on Binance). Raw data maps
are not exposed. The key is the symbol string.

### Recommended Solution

Three additive changes, no breaking changes:

**3a: Add `GetInstrument(symbol string) (Instrument, bool)`**

```go
// exchange/exchange.go
func (e *Exchange) GetInstrument(symbol string) (Instrument, bool) {
    e.mu.RLock()
    defer e.mu.RUnlock()
    inst, ok := e.Instruments[symbol]
    return inst, ok
}
```

**3b: Add `GetBook(symbol string)` — already exists**

Confirmed: `GetBook(symbol string) *OrderBook` already exists (line 526, exchange.go).
No action needed.

**3c: Keep `Instruments` and `Books` public**

The library is simulation-oriented: tests and actors need direct access for performance
and for test injection. Making these private would require adding numerous query methods
that don't add value in a simulation context. Leave them public with a comment: "Read via
GetInstrument or ListInstruments in production code; direct map access is provided for
test and simulation performance."

This is pragmatic, not a compromise of principle — the user cannot modify what is already
there, only read it. `AddInstrument` is the only sanctioned write path and it is already
the only method that creates books.

---

## Specific File Changes

### Problem 1 — Remove `MakerVolume` / `TakerVolume`

**`exchange/client.go`**
```go
// REMOVE:
MakerVolume int64
TakerVolume int64
```

**`exchange/settlement.go:handleExecution`**
```go
// REMOVE these two lines:
taker.TakerVolume += notional
maker.MakerVolume += notional
```

**`tests/gateway_integration_test.go:TestProcessExecutionsTakerSell`**

Replace the volume checks with a balance or fill verification. The test purpose is to
confirm that a matched trade settles correctly. Use post-trade balance delta:

```go
// REPLACE:
if client1.MakerVolume == 0 { t.Errorf("Client 1 should have maker volume recorded") }
if client2.TakerVolume == 0 { t.Errorf("Client 2 should have taker volume recorded") }

// WITH (example — verify BTC was transferred):
if client1.Balances["BTC"] <= BTCAmount(10) {
    t.Errorf("Client 1 (maker/buyer) should have received BTC")
}
if client2.Balances["BTC"] >= BTCAmount(10) {
    t.Errorf("Client 2 (taker/seller) should have sent BTC")
}
```

Check the test setup: client1 placed a buy limit (maker), client2 placed a sell limit
(taker). After fill, client1 should have more BTC, client2 should have more USD.

---

### Problem 2 — Per-Symbol Margin Mode

**`exchange/client.go`**
```go
type Client struct {
    ID               uint64
    Balances         map[string]int64
    Reserved         map[string]int64
    PerpBalances     map[string]int64
    PerpReserved     map[string]int64
    Borrowed         map[string]int64
    OrderIDs         []uint64
    FeePlan          FeeModel
    SymbolMarginModes map[string]MarginMode   // REPLACES: MarginMode MarginMode
    IsolatedPositions map[string]*IsolatedPosition
    // IsolatedPositions: collateral allocated to isolated symbols. Populated by
    // AllocateCollateralToPosition. NOT yet consulted during order placement —
    // isolated mode collateral pooling is deferred to a future implementation.
}

func (c *Client) MarginModeFor(symbol string) MarginMode {
    if mode, ok := c.SymbolMarginModes[symbol]; ok {
        return mode
    }
    return CrossMargin
}
```

**`NewClient`**
```go
func NewClient(id uint64, feePlan FeeModel) *Client {
    return &Client{
        ...
        SymbolMarginModes: make(map[string]MarginMode),
        IsolatedPositions: make(map[string]*IsolatedPosition),
    }
}
```

**`exchange/margin.go`**
```go
// SetMarginMode sets the margin mode for a specific symbol for clientID.
// Blocked if the client has an open position on that symbol.
func (e *Exchange) SetMarginMode(clientID uint64, symbol string, mode MarginMode) error {
    e.mu.Lock()
    defer e.mu.Unlock()

    client := e.Clients[clientID]
    if client == nil {
        return errors.New("unknown client")
    }

    if symbol == "" {
        return errors.New("symbol required")
    }

    // Block if the specific symbol has an open position
    if pos := e.Positions.GetPosition(clientID, symbol); pos != nil && pos.Size != 0 {
        return errors.New("cannot change margin mode with open positions on symbol")
    }

    client.SymbolMarginModes[symbol] = mode
    return nil
}

func (e *Exchange) AllocateCollateralToPosition(
    clientID uint64, symbol string, asset string, amount int64,
) error {
    e.mu.Lock()
    defer e.mu.Unlock()

    client := e.Clients[clientID]
    if client == nil {
        return errors.New("unknown client")
    }

    if client.MarginModeFor(symbol) != IsolatedMargin {  // CHANGED
        return errors.New("symbol not in isolated margin mode")
    }
    ...
}
```

Note: `hasOpenPositions(client *Client) bool` in the current code calls
`e.Positions.HasOpenPositions(client.ID)`. After this change, `SetMarginMode` no longer
uses `hasOpenPositions` — it calls `e.Positions.GetPosition(clientID, symbol)` directly.
`hasOpenPositions` may still be useful elsewhere; keep it but it is no longer called from
`SetMarginMode`.

**`exchange/borrowing.go`**
```go
type BorrowContext struct {
    Client    *Client
    ClientID  uint64
    Timestamp int64
    Symbol    string  // ADD: instrument symbol for per-symbol mode lookup
    LogBalance func(reason string, changes []BalanceDelta)
    LogEvent   func(event string, data any)
}

func (bm *BorrowingManager) BorrowMargin(ctx BorrowContext, asset string, amount int64, reason string) error {
    ...
    if ctx.Client.MarginModeFor(ctx.Symbol) == CrossMargin {  // CHANGED
        if err := bm.validateCrossMarginCollateral(ctx.Client, asset, amount); err != nil {
            return err
        }
    } else {
        return errors.New("isolated margin borrow requires position context")
    }
    ...
    if ctx.LogEvent != nil {
        ctx.LogEvent("borrow", BorrowEvent{
            ...
            MarginMode: ctx.Client.MarginModeFor(ctx.Symbol).String(),  // CHANGED
            ...
        })
    }
    ...
}
```

**`exchange/helpers.go`**
```go
// buildBorrowContext gains symbol parameter
func buildBorrowContext(e *Exchange, client *Client, clientID uint64, symbol string) BorrowContext {
    timestamp := e.Clock.NowUnixNano()
    return BorrowContext{
        Client:    client,
        ClientID:  clientID,
        Timestamp: timestamp,
        Symbol:    symbol,  // ADD
        LogBalance: func(reason string, changes []BalanceDelta) {
            logBalanceChange(e, timestamp, clientID, "", reason, changes)
        },
        LogEvent: func(event string, data any) {
            if log := e.getLogger("_global"); log != nil {
                log.LogEvent(timestamp, clientID, event, data)
            }
        },
    }
}
```

**`exchange/order_handling.go`**
```go
// tryReserveOrBorrow: pass book.Instrument.Symbol() to buildBorrowContext
func (e *Exchange) reserveLimitOrderFunds(client *Client, instrument Instrument, order *Order, precision int64) bool {
    if instrument.IsPerp() {
        ...
        return e.tryReserveOrBorrow(order.ClientID, instrument.QuoteAsset(), margin, client.ReservePerp, true, instrument.Symbol())
    }
    if order.Side == Buy {
        amount := (order.Qty * order.Price) / precision
        return e.tryReserveOrBorrow(order.ClientID, instrument.QuoteAsset(), amount, client.Reserve, false, instrument.Symbol())
    }
    return e.tryReserveOrBorrow(order.ClientID, instrument.BaseAsset(), order.Qty, client.Reserve, false, instrument.Symbol())
}

func (e *Exchange) tryReserveOrBorrow(
    clientID uint64, asset string, amount int64,
    reserveFn func(string, int64) bool,
    isPerp bool,
    symbol string,  // ADD
) bool {
    ...
    ctx := buildBorrowContext(e, client, clientID, symbol)  // CHANGED
    ...
}

// buildPositionSnapshots: use MarginModeFor
func (e *Exchange) buildPositionSnapshots(clientID uint64) []PositionSnapshot {
    ...
    client := e.Clients[clientID]
    for _, pos := range positions {
        ...
        var marginMode MarginMode = CrossMargin  // default
        if client != nil {
            marginMode = client.MarginModeFor(pos.Symbol)
        }
        snapshots = append(snapshots, PositionSnapshot{
            ...
            MarginType: marginMode,  // CHANGED from hardcoded CrossMargin
        })
    }
    ...
}
```

**`exchange/exchange.go:BorrowMargin` and `RepayMargin` public methods**

These are the public entry points that call `buildBorrowContext`. They need a symbol:

```go
func (e *Exchange) BorrowMargin(clientID uint64, symbol string, asset string, amount int64, reason string) error {
    ...
    ctx := buildBorrowContext(e, client, clientID, symbol)
    ...
}

func (e *Exchange) RepayMargin(clientID uint64, symbol string, asset string, amount int64) error {
    ...
    ctx := buildBorrowContext(e, client, clientID, symbol)
    ...
}
```

Note: `BorrowMargin` and `RepayMargin` public signatures gain a `symbol` parameter. This is
a breaking API change for direct callers. The borrowing tests in `tests/borrowing_test.go`
and `tests/balance_borrowing_test.go` will need a symbol argument.

For the collateral interest charging in `automation.go:ChargeCollateralInterest`, the
symbol is not needed (it iterates assets, not symbols). That path does not call
`BorrowMargin`/`RepayMargin` and does not go through `BorrowContext`. No change required.

**`tests/margin_mode_test.go`**

Update `TestSetMarginMode_*` tests to pass a symbol:
```go
ex.SetMarginMode(1, "BTC-PERP", IsolatedMargin)
```

Update `TestSetMarginMode_FailsWithOpenPositions` — it currently tests global position
blocking. After the change, it should test symbol-specific blocking. The test still passes
with the same setup because it uses `"BTC-PERP"` — the client has a BTC-PERP position, so
`SetMarginMode(1, "BTC-PERP", ...)` is blocked.

However, the test verifying that SetMarginMode fails while the client holds a position
needs to call it on the SAME symbol as the open position:
```go
err := ex.SetMarginMode(1, "BTC-PERP", IsolatedMargin)
if err == nil {
    t.Error("expected error when changing margin mode with open position on this symbol")
}
```

Confirm: if client 1 has a BTC-PERP position, `SetMarginMode(1, "ETH-PERP", Isolated)`
should now SUCCEED (no BTC-PERP position blocking ETH-PERP). This is correct behavior.
Update the test to also verify this permissive case.

---

### Problem 3 — `GetInstrument` Accessor

**`exchange/exchange.go`**
```go
// GetInstrument returns the Instrument for symbol, or (nil, false) if not found.
// Thread-safe. Prefer this over direct map access in production code.
func (e *Exchange) GetInstrument(symbol string) (Instrument, bool) {
    e.mu.RLock()
    defer e.mu.RUnlock()
    inst, ok := e.Instruments[symbol]
    return inst, ok
}
```

No other changes. No tests break. No migration needed.

---

## Implementation Phases

### Phase 1 — Remove `MakerVolume` / `TakerVolume` (safest, least risk)

**Scope**: 3 files.

1. `exchange/client.go`: delete two fields.
2. `exchange/settlement.go`: delete two increment lines in `handleExecution`.
3. `tests/gateway_integration_test.go:TestProcessExecutionsTakerSell`: replace volume
   checks with balance checks.

**Validation**: `make test` passes. `make test-race` passes.
**Reversibility**: Trivial rollback if needed.
**Risk**: Near zero. The only users of these fields are the two test assertions being replaced.

### Phase 2 — Per-Symbol Margin Mode

**Scope**: 7 files. Two external API breaks (`SetMarginMode` signature, `BorrowMargin`
and `RepayMargin` signatures).

Do Phase 2 in a sub-phase order:

**Phase 2a**: Client struct change + MarginModeFor helper + NewClient init.
- `exchange/client.go`
- Run `make test` — will fail on margin_mode tests (expected, they still use old API).

**Phase 2b**: Update `SetMarginMode` + `AllocateCollateralToPosition`.
- `exchange/margin.go`
- `tests/margin_mode_test.go`
- Run `make test` — margin_mode tests should now pass.

**Phase 2c**: Update borrowing path — `BorrowContext.Symbol` + `buildBorrowContext` + `tryReserveOrBorrow` + public `BorrowMargin`/`RepayMargin`.
- `exchange/borrowing.go`
- `exchange/helpers.go`
- `exchange/order_handling.go`
- `exchange/exchange.go`
- `tests/borrowing_test.go`, `tests/balance_borrowing_test.go`
- Run `make test` — all borrowing tests must pass.

**Phase 2d**: `buildPositionSnapshots` — populate `MarginType` correctly.
- `exchange/order_handling.go` (small change)
- `tests/perp_margin_test.go` or snapshot tests — confirm MarginType in AccountSnapshot
  reflects what was set.
- Run `make test`.

**Validation after all of Phase 2**: `make test` green, `make test-race` green.

### Phase 3 — `GetInstrument` Accessor

**Scope**: 1 file, 8 lines, additive only.

1. `exchange/exchange.go`: add `GetInstrument`.
2. Run `make test` — all tests must still pass (no behavior change).

**This phase can be done in any order, including first.**

---

## Migration: What Breaks, What Doesn't

| Change | Breaking? | Who is affected |
|---|---|---|
| Remove `Client.MakerVolume`, `Client.TakerVolume` | Yes — compile break | Any code reading these fields |
| `SetMarginMode(clientID, symbol, mode)` new sig | Yes — compile break | All callers of `SetMarginMode` |
| `BorrowMargin(clientID, symbol, asset, ...)` new sig | Yes — compile break | All callers of public `BorrowMargin`/`RepayMargin` |
| `buildBorrowContext` gains `symbol` param | No — unexported | Internal to `exchange/` |
| `tryReserveOrBorrow` gains `symbol` param | No — unexported | Internal to `exchange/` |
| `GetInstrument` added | No — additive | No existing code affected |
| `SymbolMarginModes` replaces `MarginMode` | Yes — compile break (indirect via `client.MarginMode` access) | Tests directly accessing `client.MarginMode` |

Search for direct accesses of `client.MarginMode`:

```
tests/margin_mode_test.go line 24: if ex.Clients[1].MarginMode != IsolatedMargin
```

This is the only direct field read outside `exchange/`. It needs updating to:
```go
if ex.Clients[1].MarginModeFor("BTC-PERP") != IsolatedMargin
```

---

## Priority Ordering

**1st: Phase 1 (Remove volumes)**

Zero risk. Zero behavioral impact. Cleans up misleading API.

**2nd: Phase 3 (GetInstrument)**

Additive. Takes 10 minutes. Zero risk.

**3rd: Phase 2 (Per-symbol margin mode)**

This is the meaningful change. Do after understanding any existing dependencies on
`MarginMode` field access. The sub-phase ordering (2a → 2b → 2c → 2d) ensures tests are
updated alongside each code change, not all at the end.

**Do NOT implement isolated mode settlement mechanics (Phase 2b deferred feature) in this
plan.** That is a separate feature requiring new position management logic, collateral
reservation per-symbol, and updated liquidation handling. It would double the scope and
introduce correctness risks in settlement accounting.

---

## Files to Create / Modify

| File | Action | Reason |
|---|---|---|
| `exchange/client.go` | Modify | Remove volumes; replace `MarginMode` with `SymbolMarginModes`; add `MarginModeFor` |
| `exchange/settlement.go` | Modify | Remove two volume increment lines |
| `exchange/margin.go` | Modify | `SetMarginMode` new sig; `AllocateCollateralToPosition` uses `MarginModeFor` |
| `exchange/borrowing.go` | Modify | `BorrowContext` gains `Symbol`; `BorrowMargin` uses `MarginModeFor` |
| `exchange/helpers.go` | Modify | `buildBorrowContext` gains `symbol` param |
| `exchange/order_handling.go` | Modify | `tryReserveOrBorrow` and `reserveLimitOrderFunds` pass symbol; `buildPositionSnapshots` uses `MarginModeFor` |
| `exchange/exchange.go` | Modify | `BorrowMargin`/`RepayMargin` gain `symbol`; add `GetInstrument` |
| `tests/gateway_integration_test.go` | Modify | Replace volume assertions with balance assertions |
| `tests/margin_mode_test.go` | Modify | Pass symbol to `SetMarginMode`; update direct `MarginMode` field access |
| `tests/borrowing_test.go` | Modify | Pass symbol to `BorrowMargin`/`RepayMargin` |
| `tests/balance_borrowing_test.go` | Modify | Same |

---

## Risks and Mitigations

| Risk | Likelihood | Mitigation |
|---|---|---|
| Tests access `client.MarginMode` in unexpected places | Low | `grep -r "\.MarginMode"` before starting; only one external accessor found |
| Borrowing tests call `BorrowMargin` without symbol — some use empty symbol | Medium | Audit all callers; pass `""` as symbol is defensible (falls through to cross margin default) |
| `hasOpenPositions` no longer used after margin.go change | Low | Keep it — used in any future calls; Go compiler will warn if truly unused |
| `IsolatedPositions` data becomes inconsistent with new mode system | Low | No behavior change; `AllocateCollateralToPosition` guard is updated; allocation still works |
| `automation.go:ChargeCollateralInterest` bypasses `BorrowContext` entirely | No risk | It directly mutates balances without going through BorrowMargin — no symbol needed there |

---

## Rollback Plan

- Phase 1 (volumes): trivially reversible. Add back the two fields and two increment lines.
- Phase 2a-2d: each sub-phase is committed separately. If Phase 2c (borrowing path) breaks
  tests, revert 2c and keep 2a/2b. The sub-phases are designed so each one compiles and the
  tests point to exactly what broke.
- Phase 3: additive, no rollback needed.

---

## Success Criteria

- `make test` passes after each phase.
- `make test-race` passes after Phase 2c (borrowing path changes, most race-sensitive).
- `client.MakerVolume` and `client.TakerVolume` no longer exist.
- `client.MarginMode` no longer exists; `client.MarginModeFor(symbol)` returns correct mode.
- `ex.SetMarginMode(clientID, "BTC-PERP", IsolatedMargin)` succeeds while client holds an
  ETH-PERP position (different symbol).
- `ex.SetMarginMode(clientID, "BTC-PERP", IsolatedMargin)` fails while client holds a
  BTC-PERP position.
- `PositionSnapshot.MarginType` reflects the actual per-symbol margin mode, not `CrossMargin`.
- `ex.GetInstrument("BTC/USD")` returns the instrument without acquiring a write lock.
- A user can implement per-symbol isolated margin mode in their own code by calling
  `SetMarginMode(clientID, symbol, IsolatedMargin)` and `AllocateCollateralToPosition`
  without modifying library files.
