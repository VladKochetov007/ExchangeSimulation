# Industrial Simulation Plan

## Balance Model — Current State vs Required

### How it works NOW

Exchange (`exchange/client.go`):
- `Client.Balances map[string]int64` — total balance per asset (shared across ALL markets)
- `Client.Reserved map[string]int64` — locked amount per asset for pending limit orders
- `Available = Balances[asset] - Reserved[asset]`
- Spot buy: locks quote asset. Spot sell: locks base asset.
- Perp buy/sell: locks **nothing** (no margin requirement currently)
- `Position.Margin int64` field exists but is unused placeholder

Actors maintain **local copies** that diverge from exchange truth:
- `FirstLiquidityProvidingActor.BaseBalance / QuoteBalance` — updated manually on fills (can drift)
- `MultiSymbolLP.baseBalances / quoteBalance / reservedQuoteBalance` — manual accounting layer on top of exchange
- `PureMarketMakerActor` — tracks only `inventory int64`, no balance at all; relies on exchange rejecting orders

**Critical problem:** Spot and perp share the same `Client.Balances` pool. No market separation. No realistic margin for perp.

### How it should work (real exchange model)

**Exchange/OMS boundary — critical distinction:**

The exchange stores only:
1. **Wallet balances** — actual collateral, changes only on realized events (fills, funding, withdrawals)
2. **Open positions** — raw records: `{symbol, size, entry_price}`. NOT PnL.
3. **Reserved margin** — collateral locked for open perp positions

The exchange does NOT store unrealized PnL. It computes `(mark_price - entry_price) × size` on-demand only for margin checks and liquidation triggers. PnL attribution, strategy tracking, netting — all OMS concerns, not exchange.

Netting (one position per symbol) is one OMS model. The exchange just records position entries — actors/OMS decide how to interpret them.

```
Client on Exchange
├── Spot Wallet
│   ├── BTC:  total=52.0, reserved=2.0 (locked in open sell orders)
│   └── USD:  total=100000.0, reserved=5000.0 (locked in open buy orders)
└── Perp Wallet
    ├── USD:  total=20000.0, reserved=3000.0 (posted as margin)
    └── Positions (raw records, no PnL stored):
        └── {symbol:"BTC-PERP", size:1, entry_price:98000}
```

Unrealized PnL = `(current_mark - entry) × size` — computed by actor/OMS when needed.
Realized PnL on close → transferred to perp wallet balance.
Funding payment → direct debit/credit to perp wallet balance.

### Required Changes to Exchange Layer

**Phase 0 (before industrial sim):** Market-separated wallets + margin system

#### Client struct
```go
type Client struct {
    ID           uint64
    SpotBalances map[string]int64 // total spot per asset
    SpotReserved map[string]int64 // locked in open spot orders
    PerpBalances map[string]int64 // perp collateral per asset
    PerpReserved map[string]int64 // locked as initial margin in open perp orders
    Borrowed     map[string]int64 // outstanding margin loan per asset (cross-margin)
    // Positions tracked separately in PositionManager
}
```

All maps initialized (never nil); all mutations behind exchange mutex — thread-safe.

#### Wallet API (thread-safe methods)
```go
func (c *Client) SpotAvailable(asset string) int64  // SpotBalances - SpotReserved
func (c *Client) PerpAvailable(asset string) int64  // PerpBalances - PerpReserved
func (c *Client) Transfer(from, to walletType, asset string, amount int64) error
```

#### Transfer between wallets
- Actor calls `Transfer("spot", "perp", "USD", amount)` to post margin
- Actor calls `Transfer("perp", "spot", "USD", amount)` to withdraw idle margin
- Logged as `TransferLogEntry` to `transfers.log`
- Validation: cannot transfer more than `free` balance in source wallet
- Thread-safe: exchange holds mutex during transfer

#### Margin / Borrowing
- Actor can borrow against collateral in spot wallet (like margin trading)
- `Client.Borrowed[asset]` tracks outstanding loan
- `CollateralRate` configured per exchange (e.g. 5% APR on borrowed USD)
- `ExchangeAutomation` charges interest periodically (daily or per-hour sim time):
  ```go
  interest := (borrowed * collateralRate * dt) / (365 * 24 * 3600 * precision)
  client.SpotBalances[asset] -= interest
  ```
- Actor can repay: `Repay(asset, amount)` — deducts from spot wallet, reduces `Borrowed`
- Logged as a `MarginInterestEvent` in `transfers.log`

#### Perp order flow (updated)
- Open perp: reserve `initial_margin = qty * price * margin_rate` from `PerpBalances`
- Close perp: `realized_pnl = (exit - entry) * size` → adjust `PerpBalances`; release `PerpReserved`
- Funding: directly debit/credit `PerpBalances[quote]` (no reservation)

#### Exchange Balance & Insurance Fund
The exchange itself tracks accumulated revenue and a safety fund:
```go
type ExchangeBalance struct {
    FeeRevenue    map[string]int64 `json:"fee_revenue"`    // fees collected per asset
    InsuranceFund map[string]int64 `json:"insurance_fund"` // covers liquidation shortfalls
}
```
- All trade fees credited to `FeeRevenue`
- Liquidation surplus (client leaves money on table) → `InsuranceFund`
- Liquidation deficit (client goes negative equity) → `InsuranceFund` absorbs loss
- Exchange balance is also periodic snapshot in `balances.log` as `actor_id: 0` (exchange account)

#### Margin Calls and Liquidations
Every mark price update, automation checks all open perp positions:

```
margin_ratio = (PerpAvailable(client) + unrealized_pnl) / initial_margin_posted
```

- `margin_ratio < warning_threshold` (e.g. 150% of maintenance) → emit `MarginCallEvent`
- `margin_ratio < maintenance_margin_rate` (e.g. 50%) → trigger liquidation

Liquidation process (all atomic, exchange mutex held):
1. Cancel all open orders for client on that symbol
2. Place forced market order opposite to position
3. If fill covers debt → return residual to client `PerpBalances`; surplus → `InsuranceFund`
4. If fill cannot cover → `InsuranceFund` absorbs deficit; client balance zeroed for that market
5. Emit `LiquidationEvent`; log to `transfers.log`

New event structs (all with JSON struct tags):
```go
type MarginCallEvent struct {
    Timestamp        int64  `json:"timestamp"`
    ClientID         uint64 `json:"client_id"`
    Symbol           string `json:"symbol"`
    MarginRatio      int64  `json:"margin_ratio_bps"`
    LiquidationPrice int64  `json:"liquidation_price"`
}

type LiquidationEvent struct {
    Timestamp     int64  `json:"timestamp"`
    ClientID      uint64 `json:"client_id"`
    Symbol        string `json:"symbol"`
    PositionSize  int64  `json:"position_size"`
    FillPrice     int64  `json:"fill_price"`
    RemainingDebt int64  `json:"remaining_debt"` // covered by insurance if > 0
}

type InsuranceFundEvent struct {
    Timestamp int64  `json:"timestamp"`
    Symbol    string `json:"symbol"`
    Delta     int64  `json:"delta"`   // positive = surplus in, negative = deficit covered
    Balance   int64  `json:"balance"` // fund balance after event
}

type MarginInterestEvent struct {
    Timestamp int64  `json:"timestamp"`
    ClientID  uint64 `json:"client_id"`
    Asset     string `json:"asset"`
    Amount    int64  `json:"amount"` // interest charged
}
```

All logged to `transfers.log` with an `"event"` discriminator field.

#### Exchange Automation additions (exchange/automation.go)
New loops alongside existing price update + funding settlement:
```go
// collateralChargeLoop charges margin interest on borrowed amounts (hourly sim time)
func (a *ExchangeAutomation) collateralChargeLoop() { ... }

// liquidationCheckLoop checks all positions after every mark price update
func (a *ExchangeAutomation) liquidationCheckLoop() { ... }
```
Both plugged into `Start()`/`Stop()` lifecycle. All state mutations protected by exchange mutex.

#### Tests required
- Spot limit buy: quote locked in `SpotReserved`, released on cancel or fill
- Spot limit sell: base locked in `SpotReserved`
- Perp open: initial margin locked in `PerpReserved` from perp wallet
- Perp close: PnL settled to `PerpBalances`; `PerpReserved` released
- Perp funding: debits/credits `PerpBalances`, no reservation
- Cross-market isolation: spot order has zero effect on perp wallet
- Transfer spot→perp: spot decreases, perp increases; logged to `transfers.log`
- Transfer perp→spot: perp decreases, spot increases; logged
- Borrow: `Borrowed` increases, `SpotBalances` increases
- Collateral charge: `SpotBalances` decreases by interest after elapsed sim time
- Margin call: position approaches liquidation → `MarginCallEvent` emitted
- Liquidation with surplus: client balance zeroed + surplus → insurance fund; `LiquidationEvent` logged
- Liquidation with deficit: insurance fund absorbs shortfall; `InsuranceFundEvent` logged
- Exchange balance: fee revenue accumulates; snapshot in `balances.log` as `actor_id=0`
- Concurrent transfers/orders/liquidations: no data races (`go test -race`)

**Note:** This is a prerequisite for the industrial simulation to be realistic.

---

## Implementation Principle

**Before implementing anything, verify it does not already exist or already work correctly.**
Read the relevant source files first. If logic is already in place and correct, skip it.
Only implement what is genuinely new or provably broken.

---

## Goal

Build `realistic_sim/cmd/industrial/` — an industrial-grade exchange simulation that:
- Runs for months of simulation time (configurable speedup, e.g. 1000x)
- Graceful Ctrl+C shutdown with log flushing
- Full L3 logging (snapshots + deltas) per symbol for Python reconstruction
- `balances.log` with multi-exchange actor balances as JSON
- `go run ./realistic_sim/cmd/industrial` to start

## Why Not Fix cmd/multisim

Root bugs in current simulation:
1. `BaseActor.handleResponse()` drops `*BalanceSnapshot` — actors never get initial balances → empty books
2. `MultiSymbolLP` divides quote across 17+ symbols — insufficient capital per symbol
3. Balance split among all actors including takers (~69 actors for 50 BTC = 0.72 BTC each)
4. Takers send market orders into empty book → failures, no recovery

## Architecture

Use `realistic_sim/actors/` pattern: single-symbol, inventory-based, no balance queries needed.
Actors track state from fills/cancels internally. Initial balance allocated per actor at start.

## Bootstrap Lifecycle

```
T=0      FirstLP (1 per symbol, wide spread ~150bps) — establishes initial price
T+5s     PureMM (3-4 per symbol, 5/10/20/30 bps) — waits for bid+ask present
T+10s    AvellanedaStoikov (2 per symbol) — waits for 20+ trades (vol buffer)
T+20s    FundingArb (1 per spot/perp pair), Momentum, CrossSectional
T+30s    RandomTakers (2-3 per symbol, 50% market / 50% limit)
```

Monitor every 10s sim time:
- If book empty → restart FirstLP
- If one side empty → MMs widen spread (emergency mode)

## Log Structure

```
logs/
  {exchange}/
    spot/{symbol}.log     # BookSnapshot, BookDelta, Trade (JSON lines)
    perp/{symbol}.log     # BookSnapshot, BookDelta, Trade, Funding (JSON lines)
  balances.log            # Periodic wallet snapshots (JSON lines)
  transfers.log           # Spot↔Perp transfers and margin events (JSON lines)
```

Balance log structs (all fields use JSON struct tags):
```go
type WalletSnapshot struct {
    Free     map[string]int64 `json:"free"`     // available per asset
    Reserved map[string]int64 `json:"reserved"` // locked in orders / margin
    Borrowed map[string]int64 `json:"borrowed"` // outstanding margin loan per asset
}

type ExchangeWallets struct {
    Spot WalletSnapshot `json:"spot"`
    Perp WalletSnapshot `json:"perp"`
}

type BalanceLogEntry struct {
    Timestamp int64                      `json:"timestamp"`
    ActorID   uint64                     `json:"actor_id"`
    Exchanges map[string]ExchangeWallets `json:"exchanges"`
}
```

Example `balances.log` line:
```json
{"timestamp":1000000000,"actor_id":1,"exchanges":{"binance":{"spot":{"free":{"BTC":5000000000,"USD":95000000000},"reserved":{"USD":5000000000},"borrowed":{}},"perp":{"free":{"USD":17000000000},"reserved":{"USD":3000000000},"borrowed":{"USD":2000000000}}}}}
```

Transfer log struct:
```go
type TransferLogEntry struct {
    Timestamp  int64  `json:"timestamp"`
    ActorID    uint64 `json:"actor_id"`
    Exchange   string `json:"exchange"`
    FromWallet string `json:"from_wallet"` // "spot" or "perp"
    ToWallet   string `json:"to_wallet"`
    Asset      string `json:"asset"`
    Amount     int64  `json:"amount"`
}
```

Collect balances every 10s sim time via `exchange.GetClients()`.

## Files to Create

1. `realistic_sim/cmd/industrial/main.go` — runner + signal handling
2. `realistic_sim/cmd/industrial/config.go` — config struct + YAML loading
3. `realistic_sim/cmd/industrial/config.yaml` — defaults
4. `realistic_sim/cmd/industrial/lifecycle_setup.go` — bootstrap + monitor
5. `realistic_sim/cmd/industrial/actor_factory.go` — create actors with configs
6. `realistic_sim/cmd/industrial/logger.go` — L3 + balance logging
7. `realistic_sim/cmd/industrial/funding_updater.go` — funding rate updates + 8h settlement
8. `realistic_sim/cmd/industrial/main_test.go` — integration tests

## Files to Modify

1. `realistic_sim/actors/enhanced_random.go` — book-aware: don't trade into empty book
2. `actor/actor.go` — extract repeated gateway-send guard (6 copies) into `send()` helper
3. `Makefile` — add `run-industrial` and `run-industrial-fast` targets

## Key Configs

```yaml
simulation:
  duration: 720h       # 30 days sim time
  speedup: 1000.0      # ~41 min real time
  snapshot_interval: 1s
  log_dir: logs/industrial

lifecycle:
  monitor_interval: 10s
  restart_cooldown: 30s
```

## Success Criteria

- `go run ./realistic_sim/cmd/industrial` works
- Books have continuous liquidity throughout run
- L3 reconstruction shows non-zero depth at all times
- Funding rates update every 8h sim time
- One-sided quoting when inventory limit reached
- FirstLP restarts if book goes empty
- Ctrl+C flushes logs and exits 0
- Python can parse all log files
