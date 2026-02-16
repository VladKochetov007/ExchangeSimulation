# Balance Snapshots

**Status**: Production-ready
**Last Updated**: 2026-02-16

## Overview

`BalanceSnapshot` provides a complete view of a client's balance state across all wallet types at a specific point in time. This is essential for balance verification, audit trails, and state reconstruction.

## Type Definition

```go
// exchange/types.go
type BalanceSnapshot struct {
    Timestamp    int64            `json:"timestamp"`     // Unix nanosecond timestamp
    ClientID     uint64           `json:"client_id"`     // Client identifier
    SpotBalances []AssetBalance   `json:"spot_balances"` // Spot wallet balances
    PerpBalances []AssetBalance   `json:"perp_balances"` // Perpetual wallet balances
    Borrowed     map[string]int64 `json:"borrowed"`      // Borrowed amounts per asset
}

type AssetBalance struct {
    Asset     string `json:"asset"`     // Asset symbol (e.g., "BTC", "USD")
    Total     int64  `json:"total"`     // Total balance (in asset precision units)
    Available int64  `json:"available"` // Available = Total - Reserved
    Reserved  int64  `json:"reserved"`  // Reserved for active orders
}
```

## Key Concepts

### Wallet Types

1. **Spot Wallet** (`SpotBalances`)
   - Used for spot trading
   - Buy orders reserve quote asset
   - Sell orders reserve base asset
   - Balances persist until withdrawn

2. **Perpetual Wallet** (`PerpBalances`)
   - Used for perpetual futures trading
   - Margin reserved based on position and orders
   - Funding settlements affect perpetual balances
   - Separate from spot wallet

3. **Borrowed** (`Borrowed`)
   - Tracks borrowed amounts per asset
   - Used when leverage/margin borrowing enabled
   - Must be repaid with interest
   - Map structure (asset → amount)

### Balance Equation

For each asset in each wallet:

```
Available = Total - Reserved
```

Where:
- **Total**: Complete balance including reserved amounts
- **Reserved**: Amount locked for active orders
- **Available**: Amount available for new orders or withdrawals

## API Usage

### Getting a Snapshot

```go
// From client object
client := exchange.Clients[clientID]
snapshot := client.GetBalanceSnapshot(exchange.Clock.NowUnixNano())

// From query balance request
req := &QueryRequest{RequestID: 1}
resp := exchange.queryBalance(clientID, req)
snapshot := resp.Data.(*BalanceSnapshot)
```

### Reading Snapshot Data

```go
// Check spot balances
for _, bal := range snapshot.SpotBalances {
    fmt.Printf("Spot %s: Total=%d, Available=%d, Reserved=%d\n",
        bal.Asset, bal.Total, bal.Available, bal.Reserved)
}

// Check perp balances
for _, bal := range snapshot.PerpBalances {
    fmt.Printf("Perp %s: Total=%d, Available=%d, Reserved=%d\n",
        bal.Asset, bal.Total, bal.Available, bal.Reserved)
}

// Check borrowed amounts
for asset, amount := range snapshot.Borrowed {
    if amount > 0 {
        fmt.Printf("Borrowed %s: %d\n", asset, amount)
    }
}
```

## Periodic Snapshots

Enable automatic periodic snapshots for all clients:

```go
// Snapshot every 10 seconds
exchange.EnableBalanceSnapshots(10 * time.Second)
```

**Logged as NDJSON:**
```json
{
  "event": "balance_snapshot",
  "timestamp": 1707925123456789000,
  "client_id": 2,
  "spot_balances": [
    {
      "asset": "BTC",
      "total": 1000000000,
      "available": 800000000,
      "reserved": 200000000
    }
  ],
  "perp_balances": [
    {
      "asset": "USD",
      "total": 50000000000000,
      "available": 40000000000000,
      "reserved": 10000000000000
    }
  ],
  "borrowed": {
    "USD": 10000000000000
  }
}
```

## Use Cases

### 1. Balance Verification

```go
snapshot := client.GetBalanceSnapshot(timestamp)

// Verify available calculation
for _, bal := range snapshot.SpotBalances {
    expected := bal.Total - bal.Reserved
    if bal.Available != expected {
        log.Fatalf("Balance inconsistency: %s available=%d, expected=%d",
            bal.Asset, bal.Available, expected)
    }
}
```

### 2. State Reconstruction

Reconstruct balance state from event logs:

```go
// 1. Start with initial snapshot
snapshot := client.GetBalanceSnapshot(t0)

// 2. Apply balance change events
for _, event := range balanceChangeEvents {
    applyBalanceChange(snapshot, event)
}

// 3. Verify against final snapshot
finalSnapshot := client.GetBalanceSnapshot(t1)
verify(snapshot, finalSnapshot)
```

### 3. Audit Trail

```go
// Generate balance report
func GenerateBalanceReport(clientID uint64, timestamp int64) {
    snapshot := client.GetBalanceSnapshot(timestamp)

    totalValue := int64(0)
    for _, bal := range snapshot.SpotBalances {
        price := oracle.GetPrice(bal.Asset)
        totalValue += bal.Total * price
    }

    for _, bal := range snapshot.PerpBalances {
        price := oracle.GetPrice(bal.Asset)
        totalValue += bal.Total * price
    }

    for asset, amount := range snapshot.Borrowed {
        price := oracle.GetPrice(asset)
        totalValue -= amount * price
    }

    fmt.Printf("Total Portfolio Value: %d\n", totalValue)
}
```

### 4. Risk Management

```go
// Check if client can place order
func CanPlaceOrder(client *Client, quoteAsset string, requiredMargin int64) bool {
    snapshot := client.GetBalanceSnapshot(time.Now().UnixNano())

    // Find perp balance for quote asset
    for _, bal := range snapshot.PerpBalances {
        if bal.Asset == quoteAsset {
            return bal.Available >= requiredMargin
        }
    }

    return false
}
```

## Important Notes

### Precision Units

All balance amounts are in precision units, not decimals:

```go
// BTC with 8 decimal places (satoshi precision)
BTC_PRECISION = 100_000_000

// Example: 1.5 BTC
balanceInSatoshis := int64(1.5 * float64(BTC_PRECISION))  // 150000000

// USD with 6 decimal places (micro-dollar precision)
USD_PRECISION = 1_000_000

// Example: $50,000.00
balanceInMicroUSD := int64(50000 * USD_PRECISION)  // 50000000000
```

### Reserved Balances

Reserved amounts are **included** in total but **excluded** from available:

```go
// After placing buy order
spotBalance := AssetBalance{
    Asset:     "USD",
    Total:     100_000_000_000,  // $100,000.00
    Reserved:   50_000_000_000,  // $50,000.00 locked for order
    Available:  50_000_000_000,  // $50,000.00 available
}

// Available for new orders: $50,000.00
// NOT $100,000.00
```

### Snapshot Consistency

Snapshots are **point-in-time consistent**:

```go
// CORRECT: Single snapshot at specific time
snapshot := client.GetBalanceSnapshot(timestamp)

// WRONG: Multiple snapshots may be inconsistent
spotSnapshot := client.GetBalanceSnapshot(timestamp1)
perpSnapshot := client.GetBalanceSnapshot(timestamp2)  // Different time!
```

### Zero Balances

Assets with zero balance are **not included** in the snapshot:

```go
// Client has only USD
snapshot.SpotBalances = []AssetBalance{
    {Asset: "USD", Total: 100000000000, ...},
}

// BTC balance is zero, so NOT in array
// Don't assume all assets are present!
```

### Borrowed Filtering

Borrowed map only includes amounts > 0:

```go
// Client borrowed USD, repaid BTC
snapshot.Borrowed = map[string]int64{
    "USD": 10000000000,  // Active loan
    // "BTC" not present (was repaid)
}
```

## Testing

See `exchange/balance_snapshot_test.go` for comprehensive test examples:

- Empty balances (new client)
- Spot only
- Perp only
- Mixed spot + perp + borrowed
- Reserved balance calculations
- Borrowed filtering (> 0)

## Related

- [Balance Change Logging](../observability/logging-system.md#balance-changes)
- [Balance Replication Plan](../architecture/BALANCE_REPLICATION_PLAN.md)
- [Borrowing System](./borrowing.md)
- [Margin System](./margin.md)
