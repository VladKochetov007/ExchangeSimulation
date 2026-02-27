# Exchange Realism Cleanup Plan

## Feature Goal

Eliminate five categories of design debt that currently make the library incorrect,
misleading, or harder to extend:

1. Redundant `IsPerp()`/`InstrumentType()` binary dispatch hard-codes two instrument types
   into the interface.
2. `ValidateQty` ignores `MinOrderSize`, so the field is stored but never enforced.
3. Eight "test-accessible wrapper" methods expose internal mechanics as fake public API.
4. Isolated margin mode stores collateral but never uses it in accounting.
5. `PositionSnapshot` always reports `CrossMargin` and leaves three fields empty.

Problems 2 and 3 are pure cleanup — zero behavior change, low risk. Problem 1 is an
interface change but confined to two files. Problems 4 and 5 are the feature work:
making isolated margin actually function correctly.

---

## Problem 1 — `IsPerp()` and `InstrumentType()` Redundancy

### Root Cause

`IsPerp() bool` is a boolean type-tag on the `Instrument` interface. The library
dispatches `processPerpExecution` vs `settleSpotExecution` based on it. This is the
Go equivalent of an `instanceof` chain. It means:

- The library can only ever have two instrument types without changing the interface
- Users cannot add `"OPTION"` or `"FUTURES"` without patching the library
- `InstrumentType() string` carries the same information redundantly

`IsPerp()` appears at 12 call sites inside the library. `InstrumentType()` appears
only in tests and logging. Neither belongs on the `Instrument` interface as primary
dispatch.

### Recommended Solution

**Remove `IsPerp()` from the `Instrument` interface. Retain `InstrumentType() string`
with a string constant convention. Replace boolean dispatch with a type assertion
guard pattern.**

The dispatch pattern becomes:

```go
// settlement.go
perp, isPerp := instrument.(*PerpFutures)
if isPerp {
    processPerpExecution(perp, ...)
} else {
    settleSpotExecution(...)
}
```

This is idiomatic Go. The exchange already does `perp := instrument.(*PerpFutures)`
in several places anyway — this just moves the nil-guard up one level. A user who
creates their own `OptionsInstrument` will write their own handler without touching
library code; the library simply does not handle it (falls through to spot settlement).

**Why not a `PerpBehavior` interface?**

A `type PerpBehavior interface { MarginRate() int64; FundingRate() *FundingRate }`
approach would be cleaner for extensions, but it requires every perp dispatch site
to check two types. Given this library is early-stage and `PerpFutures` is the only
non-spot type, the type-assert approach is simpler and honest. If a third type is
ever needed, the plan is to introduce a `DerivativeBehavior` interface at that point.

**What happens to `InstrumentType()`?**

It stays as a convenience method on both `SpotInstrument` and `PerpFutures`. It is
NOT on the `Instrument` interface. Users can still call it via their own concrete type.
Tests that check `inst.InstrumentType() != "SPOT"` must change to
`if _, ok := inst.(*SpotInstrument); !ok`.

**String constants** for the type strings live in `types/enums.go`:

```go
const (
    InstrumentTypeSpot = "SPOT"
    InstrumentTypePerp = "PERP"
)
```

These are informational only — no library dispatch reads them.

### Call-Site Transformation

Every call to `instrument.IsPerp()` must be replaced. There are 12 sites:

| File | Current | After |
|---|---|---|
| `exchange/settlement.go:handleExecution` | `isPerp := instrument.IsPerp()` | `perp, isPerp := instrument.(*PerpFutures)` |
| `exchange/settlement.go:processPerpExecution` | `perp := book.Instrument.(*PerpFutures)` | already has assertion; no IsPerp check needed here |
| `exchange/order_handling.go:checkMarketOrderFunds` | `if instrument.IsPerp() { perp := instrument.(*PerpFutures)` | `if perp, ok := instrument.(*PerpFutures); ok {` |
| `exchange/order_handling.go:reserveLimitOrderFunds` | `if instrument.IsPerp() { perp := instrument.(*PerpFutures)` | `if perp, ok := instrument.(*PerpFutures); ok {` |
| `exchange/order_handling.go:releaseOrderFunds` | `if instrument.IsPerp() { perp := instrument.(*PerpFutures)` | `if perp, ok := instrument.(*PerpFutures); ok {` |
| `exchange/automation.go:updateAllPerpPrices` | `if !book.Instrument.IsPerp() { continue }` then `book.Instrument.(*PerpFutures)` | `perp, ok := book.Instrument.(*PerpFutures); if !ok { continue }` |
| `exchange/automation.go:CheckAndSettleFunding` | `if inst.IsPerp() { perps = append(perps, inst.(*PerpFutures))` | `if perp, ok := inst.(*PerpFutures); ok { perps = append(perps, perp)` |
| `exchange/exchange.go:CancelAllClientOrders` | `releaseOrderFunds(client, book.Instrument, ...)` — calls through to `releaseOrderFunds` which uses `IsPerp()` | fixed via `releaseOrderFunds` change above |
| `tests/` | `inst.InstrumentType() != "SPOT"` / `!= "PERP"` | `_, ok := inst.(*SpotInstrument)` / `(*PerpFutures)` |

Net interface delta: remove `IsPerp() bool` from `types/interfaces.go`. Remove from
`instrument/spot.go` and `instrument/perp.go`. Remove the `type alias` in `exchange/types.go`
if any. Add constants to `types/enums.go`.

### Files to Modify

- `/home/vlad/development/exchange_simulation/types/interfaces.go` — remove `IsPerp() bool`
- `/home/vlad/development/exchange_simulation/types/enums.go` — add `InstrumentTypeSpot`, `InstrumentTypePerp`
- `/home/vlad/development/exchange_simulation/instrument/spot.go` — remove `IsPerp()` method
- `/home/vlad/development/exchange_simulation/instrument/perp.go` — remove `IsPerp()` method
- `/home/vlad/development/exchange_simulation/exchange/settlement.go` — replace dispatch (2 sites)
- `/home/vlad/development/exchange_simulation/exchange/order_handling.go` — replace dispatch (3 sites)
- `/home/vlad/development/exchange_simulation/exchange/automation.go` — replace dispatch (2 sites)
- `/home/vlad/development/exchange_simulation/tests/exchange_instrument_test.go` — update assertions

### Risk

Medium. Twelve call sites. The compiler will catch every missed replacement because
`IsPerp()` no longer exists on the interface. If the code compiles, dispatch is correct.
Perp-specific tests will fail immediately if a site is wrong.

---

## Problem 2 — `ValidateQty` Does Not Enforce `MinOrderSize`

### Root Cause

```go
// instrument/spot.go
func (i *SpotInstrument) ValidateQty(qty int64) bool {
    return qty > 0  // MinOrderSize field is stored but never read here
}
```

`MinOrderSize` is set by the constructor, exposed via `MinOrderSize() int64`, but the
validation method ignores it. Any quantity > 0 is accepted, making the minimum order
constraint purely cosmetic.

### Fix

```go
func (i *SpotInstrument) ValidateQty(qty int64) bool {
    return qty >= i.minOrderSize
}
```

This is a one-line change. `PerpFutures` embeds `SpotInstrument`, so it inherits
the fix automatically.

### Edge Cases

**Zero minOrderSize:** If `minOrderSize == 0` (which `NewSpotInstrument` allows since
it takes the value as a parameter), then `qty >= 0` is always true for any realistic
qty. This is a configuration responsibility — callers that pass 0 explicitly disable
the minimum. This is correct behavior.

**Market orders:** `ValidateQty` is called for all order types including Market.
A market order for 1 satoshi below `minOrderSize` will now be rejected. This is
correct — real exchanges enforce minimum size regardless of order type.

**Tests that pass qty = 1 as a trivial value:** Search reveals `USD_PRECISION/1000`
is used as minOrderSize in many test helpers (e.g. `NewSpotInstrument(..., USD_PRECISION/1000)`).
All `InjectLimitOrder` and `InjectMarketOrder` calls use `BTCAmount(n)` which
is always `>= BTC_PRECISION` (1 full coin). No existing tests use quantities below
`minOrderSize`. The change is safe.

**One test to add:** A test that places an order with qty below minOrderSize and
expects `RejectInvalidQty`. This confirms the fix works.

### Files to Modify

- `/home/vlad/development/exchange_simulation/instrument/spot.go` — one-line fix
- `/home/vlad/development/exchange_simulation/tests/` — add `TestValidateQty_BelowMinOrderSize`

---

## Problem 3 — "Test-Accessible Wrapper" Anti-Pattern

### Root Cause

Eight methods in `exchange/exchange.go` exist solely to re-expose unexported functions
as public methods "for tests":

```
PlaceOrder        → placeOrder         // legitimate API
CancelOrder       → cancelOrder        // legitimate API
QueryBalance      → queryBalance       // legitimate API
QueryAccount      → queryAccount       // legitimate API
Subscribe         → subscribe          // legitimate API
Unsubscribe       → unsubscribe        // legitimate API
HandleClientRequests → handleClientRequests  // borderline
PublishSnapshot   → publishSnapshot    // test-only internal
LogAllBalances    → logAllBalances     // test-only internal
SetRunning        → sets e.running     // test-only internal
```

The comment pattern "// PlaceOrder is the public test-accessible wrapper for placeOrder"
reveals the confusion: functions that ARE the public API are being hidden behind
unexported implementations for no reason.

### Decision by Method

**PlaceOrder, CancelOrder, QueryBalance, QueryAccount, Subscribe, Unsubscribe:**
These are legitimate public library API. Actors, strategies, and client code call
them directly (in addition to going through the request channel). The correct fix is
to make the implementations directly public — rename `placeOrder` to `PlaceOrder`,
eliminate the wrapper layer entirely. The private copies disappear.

```go
// Before: two functions
func (e *Exchange) PlaceOrder(clientID uint64, req *OrderRequest) Response {
    return e.placeOrder(clientID, req)  // wrapper
}
func (e *Exchange) placeOrder(clientID uint64, req *OrderRequest) Response { ... } // impl

// After: one function
func (e *Exchange) PlaceOrder(clientID uint64, req *OrderRequest) Response { ... } // impl
```

The `handleClientRequests` loop which currently calls `e.placeOrder` will call
`e.PlaceOrder` directly.

**HandleClientRequests:**
Currently called as `go ex.HandleClientRequests(gateway)` in multiple tests that
bypass `ConnectClient`. These tests are testing the request dispatch loop in isolation.
This is valid testing but the right home is a white-box test in `exchange/` package
(not `tests/`), or it should simply be public because it IS useful for tests that
want to control the goroutine lifecycle.

Decision: **keep it public**. The method is genuinely useful for tests that set up
custom gateways without going through `ConnectClient`. Rename the internal function
so there is no duplicate — the public one IS the implementation.

**PublishSnapshot:**
Called only in `tests/coverage_test.go` to directly exercise the snapshot path.
This is a coverage hack. The real fix is either:
- Move the test to `package exchange` (white-box test), or
- Accept it as a public utility (snapshot triggering from outside is a valid use case
  for simulation tools).

Decision: **make it legitimately public**. Remove the "test-accessible wrapper"
comment. A simulation can legitimately trigger book snapshots on demand. The method
is already correct — just remove the apology comment.

**LogAllBalances:**
Called in two test functions to exercise logger branches. This is another coverage
hack. However, forcing a balance log from outside is a plausible simulation tool
feature (e.g., "dump balances at simulation end").

Decision: **make it legitimately public, rename to `LogBalances()`** to be more
descriptive. The "All" is implicit from context.

**SetRunning:**
Used in exactly one test: `TestShutdownStopsExchange` in `tests/edge_cases_test.go`.
The test calls `ex.SetRunning(true)` to put the exchange in running state, then
calls `ex.Shutdown()` and checks that `IsRunning()` returns false.

The problem: `SetRunning(true)` without actually starting the goroutines creates
an inconsistent state — the exchange thinks it is running but has no snapshot loop
running and `shutdownCh` is unclosed. The test only works because `Shutdown` closes
channels unconditionally.

This is testing internal state mutation, not observable behavior. The test should
instead use `ConnectClient` to start the exchange properly, then call `Shutdown`.

Decision: **delete `SetRunning`**. Rewrite `TestShutdownStopsExchange` to use
`ConnectClient` to start the exchange, then `Shutdown`. The test already has
`TestShutdownStopsExchange` — the `SetRunning` call is artificial.

### Summary of Changes

| Method | Action |
|---|---|
| `PlaceOrder` + `placeOrder` | Merge: make `placeOrder` → `PlaceOrder` |
| `CancelOrder` + `cancelOrder` | Merge: make `cancelOrder` → `CancelOrder` |
| `QueryBalance` + `queryBalance` | Merge: make `queryBalance` → `QueryBalance` |
| `QueryAccount` + `queryAccount` | Merge: make `queryAccount` → `QueryAccount` |
| `Subscribe` + `subscribe` | Merge: make `subscribe` → `Subscribe` |
| `Unsubscribe` + `unsubscribe` | Merge: make `unsubscribe` → `Unsubscribe` |
| `HandleClientRequests` + `handleClientRequests` | Merge: make `handleClientRequests` → `HandleClientRequests` |
| `PublishSnapshot` + `publishSnapshot` | Merge + remove apology comment |
| `LogAllBalances` + `logAllBalances` | Merge + rename to `LogBalances` |
| `SetRunning` | Delete. Rewrite test. |

**Impact on `handleClientRequests` loop:** The loop body currently calls
`e.placeOrder`, `e.cancelOrder`, etc. After renaming, it calls `e.PlaceOrder`,
`e.CancelOrder`, etc. No behavior change.

### Files to Modify

- `/home/vlad/development/exchange_simulation/exchange/exchange.go` — merge 9 method pairs, delete SetRunning
- `/home/vlad/development/exchange_simulation/exchange/order_handling.go` — rename the implementations
- `/home/vlad/development/exchange_simulation/tests/edge_cases_test.go` — rewrite `TestShutdownStopsExchange`

---

## Problem 4 — Isolated Margin Accounting (Centerpiece)

### Root Cause

The current `IsolatedPosition` stores collateral but:

1. `reserveLimitOrderFunds` always draws from `client.PerpBalances` (global)
2. `settlePerpSide` always credits/debits `client.PerpBalances` (global)
3. `CheckLiquidations` uses `client.PerpAvailable(quoteAsset)` (global)
4. `SetMarginMode` is global per-client, not per-symbol
5. There is no per-symbol margin mode concept at all

### What "Isolated Margin" Actually Means

For symbol `BTC-PERP` in isolated mode:

1. **Funding source**: Before placing an order, trader allocates a fixed amount from
   `PerpBalances` into `IsolatedPositions["BTC-PERP"].Collateral`. This IS already
   implemented (the `AllocateCollateralToPosition` method works correctly).

2. **Order reservation**: Margin for a new order is reserved from the isolated pool,
   not the global pool. If the isolated pool has insufficient margin, the order is
   rejected — even if the global pool has plenty.

3. **Settlement credit/debit**: PnL from fills flows into/out of the isolated pool's
   collateral.

4. **Liquidation scope**: Only the isolated pool is at risk. If the position is
   liquidated, only `IsolatedPositions["BTC-PERP"].Collateral` is consumed. Other
   positions (cross or isolated on other symbols) are unaffected.

5. **Fee accounting**: Fees come out of the isolated pool's collateral.

### Per-Symbol Margin Mode

Replace the global `client.MarginMode` with per-symbol tracking:

```go
// exchange/client.go — Client struct
type Client struct {
    ...
    SymbolMarginModes  map[string]MarginMode  // per-symbol; default = CrossMargin
    IsolatedPositions  map[string]*IsolatedPosition
    // MarginMode field REMOVED
}
```

```go
// New method on Client
func (c *Client) MarginModeFor(symbol string) MarginMode {
    if mode, ok := c.SymbolMarginModes[symbol]; ok {
        return mode
    }
    return CrossMargin
}
```

```go
// exchange/margin.go
func (e *Exchange) SetMarginMode(clientID uint64, symbol string, mode MarginMode) error {
    e.mu.Lock()
    defer e.mu.Unlock()
    client := e.Clients[clientID]
    if client == nil {
        return errors.New("unknown client")
    }
    pos := e.Positions.GetPosition(clientID, symbol)
    if pos != nil && pos.Size != 0 {
        return errors.New("cannot change margin mode with open position on symbol")
    }
    if client.SymbolMarginModes == nil {
        client.SymbolMarginModes = make(map[string]MarginMode)
    }
    client.SymbolMarginModes[symbol] = mode
    return nil
}
```

The old single-argument `SetMarginMode(clientID, mode)` is **removed**. The new
signature adds `symbol string`. This is a breaking change to the public API — but
the old API was semantically wrong (global margin mode is not how any real exchange
works). The existing `margin_mode_test.go` tests must be updated.

### `IsolatedPosition` — Add Reserved Tracking

The isolated pool needs its own reserved amount so margin reservation works
consistently:

```go
// types/account.go
type IsolatedPosition struct {
    Symbol     string
    Collateral map[string]int64  // total deposited
    Reserved   map[string]int64  // reserved for open orders
    Borrowed   map[string]int64
}
```

Helper methods on `IsolatedPosition` mirror those on `Client`:

```go
func (ip *IsolatedPosition) Available(asset string) int64 {
    return ip.Collateral[asset] - ip.Reserved[asset]
}

func (ip *IsolatedPosition) Reserve(asset string, amount int64) bool {
    if ip.Available(asset) < amount {
        return false
    }
    ip.Reserved[asset] += amount
    return true
}

func (ip *IsolatedPosition) Release(asset string, amount int64) {
    ip.Reserved[asset] = max(0, ip.Reserved[asset]-amount)
}
```

### Order Placement in Isolated Mode

`reserveLimitOrderFunds` must check the margin mode and draw from the correct pool:

```go
// exchange/order_handling.go
func (e *Exchange) reserveLimitOrderFunds(client *Client, instrument Instrument, order *Order, precision int64) bool {
    perp, isPerp := instrument.(*PerpFutures)
    if !isPerp {
        // spot logic unchanged
        if order.Side == Buy {
            amount := (order.Qty * order.Price) / precision
            return e.tryReserveOrBorrow(order.ClientID, instrument.QuoteAsset(), amount, client.Reserve, false)
        }
        return e.tryReserveOrBorrow(order.ClientID, instrument.BaseAsset(), order.Qty, client.Reserve, false)
    }

    margin := calcMargin(order.Qty, order.Price, perp.MarginRate, precision)
    quote := instrument.QuoteAsset()
    symbol := instrument.Symbol()

    if client.MarginModeFor(symbol) == IsolatedMargin {
        isolated := client.IsolatedPositions[symbol]
        if isolated == nil || !isolated.Reserve(quote, margin) {
            return false  // insufficient isolated collateral
        }
        return true
    }
    // Cross margin: use global perp pool
    return e.tryReserveOrBorrow(order.ClientID, quote, margin, client.ReservePerp, true)
}
```

The same logic applies to `releaseOrderFunds`:

```go
func releaseOrderFunds(client *Client, instrument Instrument, side Side, qty, price int64) {
    if qty <= 0 {
        return
    }
    precision := instrument.BasePrecision()
    perp, isPerp := instrument.(*PerpFutures)
    if !isPerp {
        if side == Buy {
            client.Release(instrument.QuoteAsset(), (qty*price)/precision)
        } else {
            client.Release(instrument.BaseAsset(), qty)
        }
        return
    }
    margin := calcMargin(qty, price, perp.MarginRate, precision)
    quote := instrument.QuoteAsset()
    if client.MarginModeFor(instrument.Symbol()) == IsolatedMargin {
        if isolated := client.IsolatedPositions[instrument.Symbol()]; isolated != nil {
            isolated.Release(quote, margin)
        }
        return
    }
    client.ReleasePerp(quote, margin)
}
```

And `checkMarketOrderFunds`:

```go
func checkMarketOrderFunds(client *Client, book *OrderBook, order *Order, precision int64) bool {
    perp, isPerp := book.Instrument.(*PerpFutures)
    if !isPerp {
        // spot unchanged
        ...
    }
    refPrice := marketRefPrice(book)
    if refPrice == 0 {
        return true
    }
    margin := calcMargin(order.Qty, refPrice, perp.MarginRate, precision)
    quote := book.Instrument.QuoteAsset()
    if client.MarginModeFor(book.Instrument.Symbol()) == IsolatedMargin {
        isolated := client.IsolatedPositions[book.Instrument.Symbol()]
        return isolated != nil && isolated.Available(quote) >= margin
    }
    return client.PerpAvailable(quote) >= margin
}
```

### Settlement in Isolated Mode

`settlePerpSide` must credit/debit the isolated collateral pool instead of global:

```go
// exchange/settlement.go
func (e *Exchange) settlePerpSide(ctx perpSideCtx, book *OrderBook, exec *Execution, quote string, basePrecision, timestamp int64) int64 {
    pnl := realizedPerpPnL(ctx.delta.OldSize, ctx.delta.OldEntryPrice, exec.Qty, exec.Price, ctx.side, basePrecision)
    // ... log PnL event unchanged ...

    symbol := book.Symbol
    if ctx.client.MarginModeFor(symbol) == IsolatedMargin {
        isolated := ctx.client.IsolatedPositions[symbol]
        if isolated != nil {
            old := isolated.Collateral[quote]
            isolated.Collateral[quote] += pnl
            isolated.Collateral[ctx.fee.Asset] -= ctx.fee.Amount
            logBalanceChange(e, timestamp, ctx.clientID, symbol, "trade_settlement", []BalanceDelta{
                isolatedDelta(symbol, quote, old, isolated.Collateral[quote]),
            })
        }
        return pnl
    }
    // Cross margin: existing global path unchanged
    old := ctx.client.PerpBalances[quote]
    ctx.client.PerpBalances[quote] += pnl
    ctx.client.PerpBalances[ctx.fee.Asset] -= ctx.fee.Amount
    logBalanceChange(e, timestamp, ctx.clientID, symbol, "trade_settlement", []BalanceDelta{
        perpDelta(quote, old, ctx.client.PerpBalances[quote]),
    })
    return pnl
}
```

Add `isolatedDelta` to `helpers.go`:

```go
func isolatedDelta(symbol, asset string, old, new int64) BalanceDelta {
    return BalanceDelta{Asset: asset, Wallet: "isolated_" + symbol, OldBalance: old, NewBalance: new, Delta: new - old}
}
```

### Margin Adjustment in Isolated Mode

`adjustPerpMargin` calls `client.ReservePerp` and `client.ReleasePerp`. It must
also check margin mode. The `perpSideCtx` struct needs the instrument symbol to look
up the isolated pool:

```go
// Add Symbol to perpSideCtx
type perpSideCtx struct {
    client     *Client
    clientID   uint64
    side       Side
    delta      PositionDelta
    closedQty  int64
    fee        Fee
    isMarket   bool
    orderPrice int64
    symbol     string  // NEW
    isIsolated bool    // NEW — computed once at construction time
}
```

`adjustPerpMargin` then dispatches on `ctx.isIsolated`:

```go
func adjustPerpMargin(ctx perpSideCtx, execPrice, execQty int64, perp *PerpFutures, quote string, basePrecision int64) {
    margin := func(qty, price int64) int64 { return calcMargin(qty, price, perp.MarginRate, basePrecision) }

    reserveFn := ctx.client.ReservePerp
    releaseFn := ctx.client.ReleasePerp

    if ctx.isIsolated {
        if isolated := ctx.client.IsolatedPositions[ctx.symbol]; isolated != nil {
            reserveFn = isolated.Reserve
            releaseFn = isolated.Release
        }
    }

    if ctx.isMarket {
        if openedQty := execQty - ctx.closedQty; openedQty > 0 {
            reserveFn(quote, margin(openedQty, execPrice))
        }
    } else if ctx.closedQty > 0 {
        releaseFn(quote, margin(ctx.closedQty, ctx.orderPrice))
    }
    if ctx.closedQty > 0 && ctx.delta.OldSize != 0 {
        releaseFn(quote, margin(ctx.closedQty, ctx.delta.OldEntryPrice))
    }
}
```

### Liquidation in Isolated Mode

`CheckLiquidations` must distinguish pools:

```go
func (a *ExchangeAutomation) CheckLiquidations(symbol string, perp *PerpFutures, markPrice int64) {
    // ... setup unchanged ...
    for clientID, client := range a.exchange.Clients {
        pos := a.exchange.Positions.GetPosition(clientID, symbol)
        if pos == nil || pos.Size == 0 {
            continue
        }

        // ... unrealizedPnL calculation unchanged ...

        isIsolated := client.MarginModeFor(symbol) == IsolatedMargin
        var equity int64
        if isIsolated {
            isolated := client.IsolatedPositions[symbol]
            if isolated == nil {
                continue
            }
            equity = isolated.Available(perp.QuoteAsset()) + unrealizedPnL
        } else {
            equity = client.PerpAvailable(perp.QuoteAsset()) + unrealizedPnL
        }

        // ... margin ratio check, liquidation trigger unchanged ...
    }
}
```

The `liquidate` function already scoped to a single symbol, so isolated liquidation
is naturally correct: `forceClose` closes the position, `handlePostLiquidation`
debits the isolated pool, and the global pool is untouched.

`EstimateLiquidationPrice` also needs the mode:

```go
func (a *ExchangeAutomation) EstimateLiquidationPrice(pos *Position, client *Client, perp *PerpFutures, precision int64) int64 {
    var available int64
    if client.MarginModeFor(pos.Symbol) == IsolatedMargin {
        if isolated := client.IsolatedPositions[pos.Symbol]; isolated != nil {
            available = isolated.Available(perp.QuoteAsset())
        }
    } else {
        available = client.PerpAvailable(perp.QuoteAsset())
    }
    if pos.Size == 0 {
        return 0
    }
    if pos.Size > 0 {
        return pos.EntryPrice - available*precision/pos.Size
    }
    return pos.EntryPrice + available*precision/(-pos.Size)
}
```

### `AllocateCollateralToPosition` / `ReleaseCollateralFromPosition` — Update Check

These currently check `client.MarginMode != IsolatedMargin`. They must now check
`client.MarginModeFor(symbol)`:

```go
func (e *Exchange) AllocateCollateralToPosition(clientID uint64, symbol, asset string, amount int64) error {
    // ...
    if client.MarginModeFor(symbol) != IsolatedMargin {
        return errors.New("symbol is not in isolated margin mode")
    }
    // rest unchanged
}
```

### Funding Settlement in Isolated Mode

`settleFunding` calls `client.PerpBalances[quote]` directly. Funding payments must
route to the isolated pool if the position is isolated. The `settleFunding` free
function currently receives a callback `PositionsForFunding` which does not carry
margin mode context.

The simplest fix: pass clients map and check `client.MarginModeFor(symbol)` inside
the settlement function. The funding callback already passes `clientID`, and `clients`
map is already passed in:

```go
// exchange/funding.go — inside settleFunding loop
store.PositionsForFunding(perp.Symbol(), func(clientID uint64, pos Position) {
    client := clients[clientID]
    if client == nil {
        return
    }
    positionValue := abs(pos.Size) * pos.EntryPrice / precision
    funding := positionValue * fundingRate.Rate / 10000

    if client.MarginModeFor(pos.Symbol) == IsolatedMargin {
        isolated := client.IsolatedPositions[pos.Symbol]
        if isolated == nil {
            return
        }
        oldCollateral := isolated.Collateral[quote]
        if pos.Size > 0 {
            isolated.Collateral[quote] -= funding
            netExchangeFlow += funding
        } else {
            isolated.Collateral[quote] += funding
            netExchangeFlow -= funding
        }
        if sink.logBalance != nil {
            sink.logBalance(timestamp, clientID, pos.Symbol, "funding_settlement", []BalanceDelta{
                isolatedDelta(pos.Symbol, quote, oldCollateral, isolated.Collateral[quote]),
            })
        }
        return
    }
    // Cross margin path: unchanged
    oldBalance := client.PerpBalances[quote]
    ...
})
```

### Borrowing in Isolated Mode

`tryReserveOrBorrow` currently auto-borrows from perp balance for under-margined
positions. In isolated mode this should borrow from the isolated pool, not the global
pool. The `BorrowContext` already has the client; we need to add the symbol and
margin mode signal:

For now: **auto-borrow is disabled for isolated positions**. If the isolated pool
lacks margin, the order is rejected. Users can manually add collateral via
`AllocateCollateralToPosition`. This matches the behavior of most real exchanges.

The guard in `tryReserveOrBorrow`:

```go
// Do not auto-borrow into an isolated position
if isPerp {
    // Determine symbol for this borrow attempt — pass it in or skip auto-borrow
    // For simplicity in Phase 3, skip auto-borrow when client is in isolated mode
    // for any perp symbol. TODO: refine per-symbol in future.
}
```

Since `tryReserveOrBorrow` does not currently have the symbol, the simplest
implementation is to pass the symbol alongside the `isPerp` flag and let the function
skip auto-borrow for isolated-mode symbols.

### Scope Boundary

This plan does **not** implement:
- Isolated mode borrowing (manual allocation is sufficient for the simulation)
- Position-side hedge mode in isolated context (PositionBoth only for now)
- Isolated cross-contamination prevention (a client with same symbol in cross mode
  cannot have it simultaneously in isolated — guard this in `SetMarginMode`)

### Files to Modify (Problem 4)

- `/home/vlad/development/exchange_simulation/types/account.go` — add `Reserved` to `IsolatedPosition`, helper methods
- `/home/vlad/development/exchange_simulation/exchange/client.go` — replace `MarginMode` with `SymbolMarginModes`, add `MarginModeFor()`
- `/home/vlad/development/exchange_simulation/exchange/margin.go` — update `SetMarginMode` signature, `AllocateCollateralToPosition` guard
- `/home/vlad/development/exchange_simulation/exchange/order_handling.go` — update all three fund reservation/release functions
- `/home/vlad/development/exchange_simulation/exchange/settlement.go` — `settlePerpSide`, `perpSideCtx`, `adjustPerpMargin`, `processPerpExecution`
- `/home/vlad/development/exchange_simulation/exchange/automation.go` — `CheckLiquidations`, `EstimateLiquidationPrice`
- `/home/vlad/development/exchange_simulation/exchange/funding.go` — `settleFunding` funding payment routing
- `/home/vlad/development/exchange_simulation/exchange/helpers.go` — add `isolatedDelta`
- `/home/vlad/development/exchange_simulation/exchange/types.go` — `IsolatedMargin` const alias already present, keep
- `/home/vlad/development/exchange_simulation/tests/margin_mode_test.go` — update `SetMarginMode` calls to new signature

---

## Problem 5 — `PositionSnapshot` Fields That Always Lie

### Root Cause

`buildPositionSnapshots` in `exchange/exchange.go`:

```go
snapshots = append(snapshots, PositionSnapshot{
    ...
    MarginType: CrossMargin,  // always hardcoded, never checks actual mode
})
```

`IsolatedMargin`, `Leverage`, and `LiquidationPrice` are never populated.

### Fix

After Problem 4 is implemented, `buildPositionSnapshots` has access to
`client.MarginModeFor(symbol)` and the isolated pool. The full correct snapshot:

```go
func (e *Exchange) buildPositionSnapshots(clientID uint64) []PositionSnapshot {
    client := e.Clients[clientID]
    positions := e.Positions.GetAllPositions(clientID)
    if len(positions) == 0 {
        return nil
    }
    snapshots := make([]PositionSnapshot, 0, len(positions))
    for _, pos := range positions {
        instrument := e.Instruments[pos.Symbol]
        if instrument == nil {
            continue
        }
        perp, isPerp := instrument.(*PerpFutures)
        if !isPerp {
            continue  // only perp positions tracked in PositionSnapshot
        }

        book := e.Books[pos.Symbol]
        var markPrice int64
        if book != nil {
            markPrice = marketRefPrice(book)
        }

        precision := instrument.BasePrecision()
        var unrealizedPnL int64
        if markPrice > 0 && pos.EntryPrice > 0 {
            sign := int64(1)
            if pos.Size < 0 {
                sign = -1
            }
            unrealizedPnL = abs(pos.Size) * sign * (markPrice - pos.EntryPrice) / precision
        }

        marginMode := client.MarginModeFor(pos.Symbol)

        var isolatedMarginAmt int64
        var liqPrice int64
        if marginMode == IsolatedMargin {
            if isolated := client.IsolatedPositions[pos.Symbol]; isolated != nil {
                isolatedMarginAmt = isolated.Available(instrument.QuoteAsset())
            }
            automation := e.automation // see note below
            if automation != nil {
                liqPrice = automation.EstimateLiquidationPrice(&pos, client, perp, precision)
            }
        } else {
            // Cross margin: liq price uses global available
            if e.automation != nil {
                liqPrice = e.automation.EstimateLiquidationPrice(&pos, client, perp, precision)
            }
        }

        // Leverage = 1 / marginRate expressed as integer
        // e.g. marginRate 1000 bps = 10% = 10x leverage
        leverage := int64(10000) / perp.MarginRate

        snapshots = append(snapshots, PositionSnapshot{
            Symbol:           pos.Symbol,
            PositionSide:     pos.PositionSide,
            Size:             pos.Size,
            EntryPrice:       pos.EntryPrice,
            MarkPrice:        markPrice,
            UnrealizedPnL:    unrealizedPnL,
            MarginType:       marginMode,
            IsolatedMargin:   isolatedMarginAmt,
            Leverage:         leverage,
            LiquidationPrice: liqPrice,
        })
    }
    return snapshots
}
```

**Note on `e.automation`:** `Exchange` does not currently hold a reference to
`ExchangeAutomation`. `EstimateLiquidationPrice` is a method on `ExchangeAutomation`.
Two options:
1. Move `EstimateLiquidationPrice` to a free function that takes `*Position`,
   `*Client`, `*PerpFutures`, `int64` — no automation dependency.
2. Store `*ExchangeAutomation` on `Exchange`.

Option 1 is cleaner. Extract:

```go
// exchange/automation.go or exchange/margin.go
func estimateLiquidationPrice(pos *Position, available int64, precision int64) int64 {
    if pos.Size == 0 {
        return 0
    }
    if pos.Size > 0 {
        return pos.EntryPrice - available*precision/pos.Size
    }
    return pos.EntryPrice + available*precision/(-pos.Size)
}
```

`ExchangeAutomation.EstimateLiquidationPrice` becomes a thin wrapper. `buildPositionSnapshots`
calls `estimateLiquidationPrice` directly with the appropriate `available` amount.

### Files to Modify (Problem 5)

- `/home/vlad/development/exchange_simulation/exchange/exchange.go` — rewrite `buildPositionSnapshots`
- `/home/vlad/development/exchange_simulation/exchange/automation.go` — extract `estimateLiquidationPrice` as free function
- `/home/vlad/development/exchange_simulation/types/account.go` — no change needed, fields already exist

---

## Architecture Overview

After all phases complete, the key data flow for an isolated perp order is:

```
PlaceOrder(client=1, BTC-PERP, Buy, qty=0.1, price=$50k)
  |
  +-- validatePlaceOrder: instrument.ValidateQty(qty) >= minOrderSize  [Problem 2 fix]
  |
  +-- reserveLimitOrderFunds:
  |     client.MarginModeFor("BTC-PERP") == IsolatedMargin?
  |       YES: IsolatedPositions["BTC-PERP"].Reserve("USD", margin)
  |       NO:  client.ReservePerp("USD", margin)
  |
  +-- Match → Execution
  |
  +-- processPerpExecution:
  |     perp, ok := instrument.(*PerpFutures)  [Problem 1 fix: no IsPerp()]
  |     adjustPerpMargin: routes to isolated.Release / isolated.Reserve
  |     settlePerpSide:   routes to isolated.Collateral += pnl
  |
  +-- buildPositionSnapshots (queryAccount):
        MarginType = client.MarginModeFor(symbol)  [Problem 5 fix]
        IsolatedMargin = isolated.Available(...)
        Leverage = 10000 / perp.MarginRate
        LiquidationPrice = estimateLiquidationPrice(pos, available, precision)
```

---

## Implementation Phases

### Phase 1 — Quick Wins (Problems 2 and 3): ~2 hours, zero behavior change

**Phase 1a: MinOrderSize enforcement**

1. Edit `instrument/spot.go`: change `ValidateQty` to `return qty >= i.minOrderSize`
2. Add test `TestValidateQty_BelowMinOrderSize` in `tests/order_test.go` or new file
3. Run `make test` — all tests must pass

**Phase 1b: Test wrapper cleanup**

1. In `exchange/exchange.go`: rename `placeOrder` → `PlaceOrder` (in `order_handling.go`), etc.
   Update `handleClientRequests` loop to call the public names.
2. Delete `SetRunning`.
3. Rewrite `TestShutdownStopsExchange` in `tests/edge_cases_test.go` to use `ConnectClient` + `Shutdown`.
4. Remove all "// X is the public test-accessible wrapper for x" comments.
5. Run `make test`

### Phase 2 — Interface Cleanup (Problem 1): ~3 hours, compiler-verified

1. Remove `IsPerp() bool` from `types/interfaces.go`
2. Remove `IsPerp()` from `instrument/spot.go` and `instrument/perp.go`
3. Add `InstrumentTypeSpot`, `InstrumentTypePerp` constants to `types/enums.go`
4. Fix all 12 call sites in `exchange/` (compiler will enumerate them for you)
5. Update tests in `tests/exchange_instrument_test.go`
6. Run `make test`

**Critical path gate:** Phase 2 must land before Phase 3 because Phase 3 changes
`reserveLimitOrderFunds` and `releaseOrderFunds`, which are also modified in Phase 2.
Doing them separately reduces diff size and review complexity.

### Phase 3 — Isolated Margin Accounting (Problems 4 and 5): ~1-2 days

The phases within Phase 3 are ordered by dependency:

**Phase 3a: Per-symbol margin mode infrastructure**
1. Add `Reserved` map to `IsolatedPosition` in `types/account.go`
2. Add `Available()`, `Reserve()`, `Release()` methods to `IsolatedPosition`
3. Replace `client.MarginMode` with `client.SymbolMarginModes` and `MarginModeFor()` in `client.go`
4. Update `NewClient` initializer
5. Update `margin.go:SetMarginMode` to new per-symbol signature
6. Update `AllocateCollateralToPosition` and `ReleaseCollateralFromPosition` guards
7. Update `margin_mode_test.go` to new API
8. Run `make test`

**Phase 3b: Order reservation and release**
1. Update `reserveLimitOrderFunds` — isolated pool branch
2. Update `releaseOrderFunds` — isolated pool branch
3. Update `checkMarketOrderFunds` — isolated pool branch
4. Add `isolatedDelta` helper to `helpers.go`
5. Run `make test`

**Phase 3c: Settlement routing**
1. Add `symbol string` and `isIsolated bool` to `perpSideCtx`
2. Populate `isIsolated` in `processPerpExecution` based on `client.MarginModeFor(symbol)`
3. Update `adjustPerpMargin` to route reserve/release calls
4. Update `settlePerpSide` to credit/debit isolated pool
5. Run `make test`

**Phase 3d: Liquidation routing**
1. Update `CheckLiquidations` equity calculation
2. Update `EstimateLiquidationPrice` to accept available as parameter (or extract free function)
3. Run `make test`

**Phase 3e: Funding routing**
1. Update `settleFunding` to route funding payments to isolated pool when applicable
2. Run `make test`

**Phase 3f: PositionSnapshot population (Problem 5)**
1. Extract `estimateLiquidationPrice` as free function
2. Rewrite `buildPositionSnapshots` to populate all fields
3. Run `make test`

---

## Files to Create / Modify

| File | Change Type | Phase |
|---|---|---|
| `/home/vlad/development/exchange_simulation/instrument/spot.go` | Modify: `ValidateQty` fix, remove `IsPerp()` | 1a, 2 |
| `/home/vlad/development/exchange_simulation/instrument/perp.go` | Modify: remove `IsPerp()` | 2 |
| `/home/vlad/development/exchange_simulation/types/interfaces.go` | Modify: remove `IsPerp() bool` | 2 |
| `/home/vlad/development/exchange_simulation/types/enums.go` | Modify: add `InstrumentType*` constants | 2 |
| `/home/vlad/development/exchange_simulation/types/account.go` | Modify: `IsolatedPosition.Reserved`, helper methods | 3a |
| `/home/vlad/development/exchange_simulation/exchange/client.go` | Modify: `SymbolMarginModes`, `MarginModeFor()` | 3a |
| `/home/vlad/development/exchange_simulation/exchange/margin.go` | Modify: `SetMarginMode`, `AllocateCollateralToPosition` | 3a |
| `/home/vlad/development/exchange_simulation/exchange/helpers.go` | Modify: add `isolatedDelta` | 3b |
| `/home/vlad/development/exchange_simulation/exchange/order_handling.go` | Modify: reserve/release/check market funds; rename private→public | 1b, 3b |
| `/home/vlad/development/exchange_simulation/exchange/settlement.go` | Modify: `perpSideCtx`, `adjustPerpMargin`, `settlePerpSide` | 3c |
| `/home/vlad/development/exchange_simulation/exchange/automation.go` | Modify: liquidation equity, liq price, `IsPerp` removal | 2, 3d |
| `/home/vlad/development/exchange_simulation/exchange/funding.go` | Modify: `settleFunding` routing | 3e |
| `/home/vlad/development/exchange_simulation/exchange/exchange.go` | Modify: wrapper cleanup, `buildPositionSnapshots` | 1b, 3f |
| `/home/vlad/development/exchange_simulation/tests/edge_cases_test.go` | Modify: `TestShutdownStopsExchange` rewrite | 1b |
| `/home/vlad/development/exchange_simulation/tests/exchange_instrument_test.go` | Modify: InstrumentType assertions | 2 |
| `/home/vlad/development/exchange_simulation/tests/margin_mode_test.go` | Modify: new `SetMarginMode` signature | 3a |
| `/home/vlad/development/exchange_simulation/tests/order_test.go` | Add: `TestValidateQty_BelowMinOrderSize` | 1a |

No new files need to be created. All changes are additive within existing files.

---

## Success Criteria

**Phase 1:**
- `make test` passes
- `instrument.ValidateQty(minOrderSize - 1)` returns `false`
- No method in `exchange/exchange.go` has the comment "test-accessible wrapper"
- `SetRunning` does not exist

**Phase 2:**
- `make test` passes
- `Instrument` interface has no `IsPerp()` method
- `grep -r "IsPerp()" exchange/` returns zero results
- `grep -r "IsPerp()" instrument/` returns zero results

**Phase 3:**
- `make test` passes with all existing tests
- A new integration test `TestIsolatedMargin_OrderReservationUsesIsolatedPool` passes
- A new integration test `TestIsolatedMargin_LiquidationDoesNotAffectOtherSymbols` passes
- A new integration test `TestIsolatedMargin_SettlementCreditsIsolatedPool` passes
- `buildPositionSnapshots` returns correct `MarginType`, `IsolatedMargin`, `Leverage`,
  and a non-zero `LiquidationPrice` for positions with open interest

---

## Risks and Mitigations

| Risk | Likelihood | Mitigation |
|---|---|---|
| Phase 2 misses a call site and compile fails | Very Low | Compiler enumerates all missing implementations |
| Isolated fund routing breaks cross-margin conservation tests | Medium | Run `make test` after each Phase 3 sub-step; conservation tests will catch leaks immediately |
| `adjustPerpMargin` function signature change is tricky | Medium | The `isIsolated` bool on `perpSideCtx` avoids passing the isolated pool pointer directly; the function looks it up from `ctx.client.IsolatedPositions[ctx.symbol]` |
| Funding settlement routing to isolated pool breaks zero-sum conservation | Medium | `netExchangeFlow` accounting does not change — only where the client-side balance is stored changes |
| Per-symbol `SetMarginMode` breaks existing tests that pass a global mode | High (certain) | `margin_mode_test.go` must be updated in Phase 3a before continuing |
| `buildPositionSnapshots` skips spot positions (correct) but tests may expect them | Low | Currently `buildPositionSnapshots` returns all positions from `PositionStore`; the rewrite adds a `perp, isPerp := ...` guard. Check `tests/` for `queryAccount` assertions on spot positions |

---

## Rollback Plan

Each phase is an independent commit. If Phase 3c (settlement routing) introduces
accounting bugs visible in conservation tests, revert to the Phase 3b commit.
Phases 1 and 2 are independent of Phase 3 and never need to be rolled back to fix
Phase 3 issues.

The highest-risk single change is Phase 3a (replacing `client.MarginMode` with
`SymbolMarginModes`). This touches every callsite that reads margin mode. Do it in
one commit that includes the tests, so a revert is clean.

---

## What This Plan Does NOT Do

- Does not implement isolated-mode auto-borrowing (manual allocation is sufficient)
- Does not implement hedge-mode isolated margin (PositionBoth only)
- Does not implement margin mode switching while position is open (already blocked)
- Does not match every edge case of Binance/OKX production behavior
- Does not add a cross-isolated pool transfer function (out of scope)
- Does not address the `btcPrecision` TODO in `helpers.go` (separate ticket)
