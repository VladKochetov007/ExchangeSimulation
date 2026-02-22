# Borrowing and Margin Lending

`BorrowingManager` provides cross-margin lending for both spot and perpetual accounts. It is optional — disabled by default and enabled via `BorrowingConfig`.

## Configuration

```go
bm := exchange.NewBorrowingManager(ex, exchange.BorrowingConfig{
    Enabled:        true,
    AutoBorrowSpot: true,   // borrow automatically when spot order needs more balance
    AutoBorrowPerp: true,   // borrow automatically when perp margin is short

    DefaultMarginMode: exchange.CrossMargin,

    BorrowRates: map[string]int64{
        "USD": 500,  // 5% APR in bps
        "BTC": 200,
    },
    CollateralFactors: map[string]float64{
        "USD": 1.0,
        "BTC": 0.9,   // 90% of BTC value counted as collateral
        "default": 0.75,
    },
    MaxBorrowPerAsset: map[string]int64{
        "USD": 1_000_000 * exchange.USD_PRECISION,
    },
    PriceOracle: myOracle, // implements CollateralPriceOracle
})
ex.BorrowingMgr = bm
```

The `default` key in `BorrowRates` and `CollateralFactors` is the fallback for any asset not explicitly listed. If no default is set, the default borrow rate is 500 bps (5%) and default collateral factor is 0.75.

## CollateralPriceOracle

Required for cross-margin collateral validation. Implement the one-method interface:

```go
type CollateralPriceOracle interface {
    GetPrice(asset string) int64  // returns price in quote precision
}
```

Example using a fixed oracle for testing:

```go
type FixedOracle struct{ prices map[string]int64 }
func (o *FixedOracle) GetPrice(asset string) int64 { return o.prices[asset] }

oracle := &FixedOracle{prices: map[string]int64{
    "BTC": 100_000 * exchange.USD_PRECISION,
    "USD": 1 * exchange.USD_PRECISION,
}}
```

## Manual Borrow and Repay

```go
// Borrow 10,000 USD into the client's perp balance
err := ex.BorrowingMgr.BorrowMargin(clientID, "USD", 10_000*exchange.USD_PRECISION, "manual")
if err != nil {
    // "borrowing disabled", "insufficient collateral", "exceeds max borrow limit"
}

// Repay up to the full borrowed amount (auto-caps to outstanding debt)
err = ex.BorrowingMgr.RepayMargin(clientID, "USD", 10_000*exchange.USD_PRECISION)
```

`BorrowMargin` validates collateral before crediting: the total value of the client's perp balances multiplied by their collateral factors must cover all existing plus new debt. It then adds the borrowed amount to both `client.PerpBalances[asset]` and `client.Borrowed[asset]`.

`RepayMargin` checks that `client.PerpAvailable(asset) >= amount` (the client must have available balance to repay from), then subtracts from both maps.

## Auto-Borrow

When `AutoBorrowSpot` or `AutoBorrowPerp` is true, the exchange calls borrow automatically when it detects insufficient balance at order placement:

```go
// Called internally by the exchange before processing a spot buy order
borrowed, err := bm.AutoBorrowForSpotTrade(clientID, "USD", requiredAmount)

// Called before processing a perp margin order
borrowed, err := bm.AutoBorrowForPerpTrade(clientID, "USD", requiredMargin)
```

Both functions are no-ops if the client already has enough available balance. If they borrow, they return `(true, nil)`.

## Collateral Validation (cross-margin only)

The formula applied in `BorrowMargin`:

```
maxBorrowValue = Σ(perpBalance[asset] × collateralFactor[asset]) × price[asset]
existingDebt   = Σ(borrowed[asset] × price[asset])
newDebtValue   = borrowAmount × price[borrowAsset]

allowed: existingDebt + newDebtValue ≤ maxBorrowValue
```

Isolated margin borrow is not supported via `BorrowMargin` directly — it requires a position context from `MarginModeManager`.

## Events

Both operations emit events to the `_global` logger:

**BorrowEvent:**
```json
{
  "event": "borrow",
  "timestamp": 1708000000000000000,
  "client_id": 2,
  "asset": "USD",
  "amount": 10000000000,
  "reason": "auto_perp",
  "margin_mode": "cross",
  "interest_rate_bps": 500,
  "collateral_used": 13333333333
}
```

**RepayEvent:**
```json
{
  "event": "repay",
  "timestamp": 1708000000000000001,
  "client_id": 2,
  "asset": "USD",
  "principal": 10000000000,
  "interest": 0,
  "remaining_debt": 0
}
```

`interest` is currently always 0 — interest accrual is not implemented; the borrow rate field in `BorrowEvent` records the configured rate for offline calculation.

## Balance Snapshots

Borrowed amounts appear in `BalanceSnapshot.Borrowed`:

```go
snapshot := client.GetBalanceSnapshot(ex.Clock.NowUnixNano())
for asset, amount := range snapshot.Borrowed {
    fmt.Printf("Outstanding debt %s: %d\n", asset, amount)
}
```

Assets with zero outstanding debt are omitted from the map.

## Related

- [Balance Snapshots](balance-snapshots.md)
- [Positions and Margin](positions-and-margin.md)
