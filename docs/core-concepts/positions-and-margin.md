# Positions and Margin

Position tracking, PnL calculation, margin requirements, and liquidation for perpetual futures.

## Position Structure

```go
type Position struct {
    ClientID   uint64
    Symbol     string
    Size       int64   // Positive = long, negative = short, 0 = flat
    EntryPrice int64   // Weighted average entry price
    Margin     int64   // Allocated margin
    Instrument Instrument
}
```

## Position Lifecycle

### Opening a Position

**Initial trade:** Buy 10 BTC @ \$50,000

```
Before:
  Size = 0
  EntryPrice = 0

After:
  Size = 10 × 1e8
  EntryPrice = 50,000 × 1e8
```

### Increasing a Position

**Weighted average entry price:**

$$P_{new} = \frac{(Size_{old} \times P_{old}) + (Qty_{new} \times P_{new})}{Size_{old} + Qty_{new}}$$

**Example:** Long 10 BTC @ \$50,000, buy 5 more @ \$51,000

```
Old position value: 10 × 50,000 = 500,000
New trade value:    5 × 51,000 = 255,000
Total value:        755,000
New size:           15 BTC
New entry price:    755,000 / 15 = $50,333.33
```

**Code:**
```go
totalNotional := oldSize*oldPrice + qty*price
newSize := oldSize + qty
newEntryPrice := totalNotional / newSize
```

### Reducing a Position

**Realized PnL on reduction:**

$$PnL_{realized} = ClosedQty \times sign(OldSize) \times \frac{(ExitPrice - EntryPrice)}{Precision}$$

**Example:** Long 10 BTC @ \$50,000, sell 6 BTC @ \$52,000

```
Closed qty: 6 BTC
Entry price: $50,000
Exit price: $52,000
PnL = 6 × 1 × (52,000 - 50,000) = $12,000 profit

Remaining position:
  Size: 4 BTC (still long)
  EntryPrice: $50,000 (unchanged)
```

**Code:**
```go
closedQty := min(abs(oldSize), qty)
sign := 1 if oldSize > 0 else -1
realizedPnL := closedQty × sign × (exitPrice - entryPrice) / precision
```

### Closing a Position

**Example:** Long 10 BTC @ \$50,000, sell 10 BTC @ \$53,000

```
PnL = 10 × (53,000 - 50,000) = $30,000 profit

After:
  Size = 0
  EntryPrice = 0 (reset)
```

### Flipping a Position

**Example:** Long 10 BTC @ \$50,000, sell 15 BTC @ \$52,000

```
Close existing: 10 BTC
  PnL = 10 × (52,000 - 50,000) = $20,000 profit

Open new short: 5 BTC
  Size = -5 × 1e8
  EntryPrice = 52,000 × 1e8
```

**Code:**
```go
if oldSize > 0 && qty > oldSize {
    // Close long, open short
    realizedPnL := oldSize × (exitPrice - entryPrice) / precision
    newSize := -(qty - oldSize)
    newEntryPrice := exitPrice
}
```

## Unrealized PnL

**Formula:**

$$PnL_{unrealized} = Size \times \frac{(MarkPrice - EntryPrice)}{Precision}$$

**Example:** Long 10 BTC @ \$50,000, mark price = \$52,000

```
PnL = 10 × (52,000 - 50,000) = $20,000 unrealized profit
```

**For shorts:**

$$PnL_{unrealized} = Size \times \frac{(EntryPrice - MarkPrice)}{Precision}$$

(Size is negative, so formula flips automatically)

**Example:** Short 10 BTC @ \$50,000, mark price = \$48,000

```
Size = -10 BTC
PnL = -10 × (48,000 - 50,000) = -10 × (-2,000) = $20,000 profit
```

## Margin Requirements

### Initial Margin

**Required to open position:**

$$Margin_{initial} = \frac{Notional \times MarginRate}{10000}$$

$$Notional = \frac{Price \times Qty}{Precision}$$

**Example:** Open 10 BTC @ \$50,000, MarginRate = 1000 bps (10%, 10x leverage)

```
Notional = 10 × 50,000 = $500,000
Initial margin = 500,000 × 0.10 = $50,000
Leverage = 500,000 / 50,000 = 10x
```

### Maintenance Margin

**Minimum to avoid liquidation:**

$$Margin_{maintenance} = \frac{Notional \times MaintenanceMarginRate}{10000}$$

**Example:** 10 BTC @ \$50,000, MaintenanceMarginRate = 500 bps (5%)

```
Maintenance margin = 500,000 × 0.05 = $25,000
```

**Liquidation trigger:**
```
If (Balance + UnrealizedPnL) < MaintenanceMargin:
    Liquidate position
```

### Warning Margin

**Issue margin call:**

$$Margin_{warning} = \frac{Notional \times WarningMarginRate}{10000}$$

**Example:** WarningMarginRate = 750 bps (7.5%)

```
Warning margin = 500,000 × 0.075 = $37,500
```

Warns user they're approaching liquidation.

## Margin Modes

### Cross Margin

**Shared collateral pool:**
- All positions share account balance
- Unrealized PnL affects all positions
- One losing position can't liquidate if others profitable

**Available margin:**
```
AvailableMargin = Balance + ΣUnrealizedPnL - ΣUsedMargin
```

**Example:**
```
Balance: $100,000
Position 1: Long 5 BTC @ $50,000, unrealized PnL = +$5,000
Position 2: Short 10 BTC @ $48,000, unrealized PnL = -$3,000
Position 3: Long 3 BTC @ $51,000, unrealized PnL = +$1,000

Available = 100,000 + 5,000 - 3,000 + 1,000 - (used margins)
          = $103,000 - (used margins)
```

### Isolated Margin

**Per-position collateral:**
- Each position has dedicated margin
- Liquidation risk isolated
- Must manually allocate margin per position

**Allocation:**
```go
ex.MarginModeMgr.AllocateCollateralToPosition(
    clientID,
    "BTC-PERP",
    "USD",
    50_000 * USD_PRECISION,  // Allocate $50k to this position
)
```

**Liquidation:**
- Only affects single position
- Remaining account balance untouched
- Useful for hedging strategies

## Margin Calculation in Code

```go
func (pm *PositionManager) UpdatePosition(
    clientID uint64,
    symbol string,
    side Side,
    qty int64,
    price int64,
) (*Position, *PositionDelta) {
    pos := pm.GetPosition(clientID, symbol)

    oldSize := pos.Size
    oldEntryPrice := pos.EntryPrice

    if oldSize == 0 {
        // Open new position
        pos.Size = qty if side == Buy else -qty
        pos.EntryPrice = price
    } else if (oldSize > 0 && side == Buy) || (oldSize < 0 && side == Sell) {
        // Increase position
        totalNotional := (oldSize*oldEntryPrice + qty*price)
        newSize := oldSize + (qty if side == Buy else -qty)
        pos.Size = newSize
        pos.EntryPrice = totalNotional / newSize
    } else {
        // Reduce or flip
        closedQty := min(abs(oldSize), qty)

        if abs(oldSize) >= qty {
            // Partial close
            pos.Size = oldSize + (-qty if oldSize > 0 else qty)
            // Entry price unchanged
        } else {
            // Full close + flip
            pos.Size = -(qty - abs(oldSize)) if oldSize > 0 else (qty - abs(oldSize))
            pos.EntryPrice = price
        }
    }

    return pos, &PositionDelta{
        OldSize: oldSize,
        OldEntryPrice: oldEntryPrice,
        NewSize: pos.Size,
        NewEntryPrice: pos.EntryPrice,
    }
}
```

## Realized PnL Calculation

```go
func realizedPerpPnL(
    oldSize, oldPrice int64,
    fillQty, fillPrice int64,
    precision int64,
) int64 {
    if oldSize == 0 {
        return 0  // Opening position, no realized PnL
    }

    if (oldSize > 0 && fillQty < 0) || (oldSize < 0 && fillQty > 0) {
        // Position-reducing trade
        closedQty := min(abs(oldSize), abs(fillQty))

        if oldSize > 0 {
            // Closing long
            return closedQty * (fillPrice - oldPrice) / precision
        } else {
            // Closing short
            return closedQty * (oldPrice - fillPrice) / precision
        }
    }

    return 0  // Position-increasing trade
}
```

## Liquidation Process

### Liquidation Check

```go
func (ex *Exchange) checkLiquidation(clientID uint64) bool {
    client := ex.Clients[clientID]
    balance := client.PerpBalances["USD"]

    for symbol, pos := range ex.Positions.GetPositions(clientID) {
        if pos.Size == 0 {
            continue
        }

        inst := ex.Instruments[symbol].(*PerpFutures)
        markPrice := ex.getMarkPrice(symbol)

        unrealizedPnL := (pos.Size * (markPrice - pos.EntryPrice)) / inst.BasePrecision()
        notional := (abs(pos.Size) * markPrice) / inst.BasePrecision()
        maintenanceMargin := (notional * inst.MaintenanceMarginRate) / 10000

        if (balance + unrealizedPnL) < maintenanceMargin {
            return true  // Liquidate
        }
    }

    return false
}
```

### Liquidation Execution

**Steps:**
1. Calculate liquidation price
2. Close position at market
3. Deduct losses from balance
4. Collect penalty to insurance fund
5. Log liquidation event

**Liquidation price:**

$$P_{liquidation} = EntryPrice \pm \frac{(Balance - MaintenanceMargin) \times Precision}{Size}$$

**Example:** Long 10 BTC @ \$50,000, balance = \$50,000, maintenance = \$25,000

```
Liquidation price = 50,000 - ((50,000 - 25,000) / 10)
                  = 50,000 - 2,500
                  = $47,500
```

If mark price drops to \$47,500, position liquidated.

## Insurance Fund

**Penalty on liquidation:**
```go
penalty := (notional × PenaltyRate) / 10000  // Typical: 50 bps
ex.ExchangeBalance.InsuranceFund += penalty
```

**Insurance fund covers:**
- Bankruptcies (losses beyond account balance)
- Auto-deleveraging events
- System failures

**Production patterns:**
- Successful venues: insurance funds grow over time from liquidation penalties
- Crisis scenarios: insurance fund depletion triggers auto-deleveraging (profitable positions reduced)
- Typical growth: 0.1-0.5% of open interest per year in stable markets

## Position Examples

### Example 1: Long Profit

```
Initial: Balance = $100,000

1. Open long 10 BTC @ $50,000
   - Entry price: $50,000
   - Initial margin: $50,000 (10x leverage)
   - Remaining balance: $50,000

2. Mark price → $55,000
   - Unrealized PnL: 10 × (55,000 - 50,000) = $50,000
   - Total equity: $100,000 + $50,000 = $150,000

3. Close 10 BTC @ $55,000
   - Realized PnL: $50,000
   - Final balance: $150,000
```

### Example 2: Short Loss

```
Initial: Balance = $100,000

1. Open short 10 BTC @ $50,000
   - Entry price: $50,000
   - Initial margin: $50,000
   - Remaining: $50,000

2. Mark price → $52,000
   - Unrealized PnL: -10 × (52,000 - 50,000) = -$20,000
   - Total equity: $100,000 - $20,000 = $80,000

3. Close 10 BTC @ $52,000
   - Realized PnL: -$20,000
   - Final balance: $80,000
```

### Example 3: Partial Close

```
Initial: Balance = $100,000

1. Open long 10 BTC @ $50,000
   - Entry price: $50,000
   - Margin: $50,000

2. Price → $55,000, close 6 BTC
   - Realized PnL: 6 × (55,000 - 50,000) = $30,000
   - Balance: $130,000
   - Remaining position: 4 BTC @ $50,000 (entry unchanged)

3. Price → $48,000, close 4 BTC
   - Realized PnL: 4 × (48,000 - 50,000) = -$8,000
   - Final balance: $122,000
   - Net PnL: +$30,000 - $8,000 = +$22,000
```

### Example 4: Liquidation

```
Initial: Balance = $50,000

1. Open long 10 BTC @ $50,000 (10x leverage)
   - Notional: $500,000
   - Initial margin: $50,000
   - Maintenance margin: $25,000 (5%)
   - Liquidation price: ~$47,500

2. Price drops to $48,000
   - Unrealized PnL: 10 × (48,000 - 50,000) = -$20,000
   - Equity: $50,000 - $20,000 = $30,000
   - Still above maintenance ($25,000), no liquidation

3. Price drops to $47,400
   - Unrealized PnL: 10 × (47,400 - 50,000) = -$26,000
   - Equity: $50,000 - $26,000 = $24,000
   - Below maintenance ($25,000) → LIQUIDATE

4. Liquidation execution
   - Close 10 BTC at market (~$47,400)
   - Loss: $26,000
   - Penalty: ~$250 to insurance fund
   - Final balance: ~$23,750
```

## Next Steps

- [Funding Rates](funding-rates.md) - Funding payments on positions
- [Liquidation](../advanced/liquidation.md) - Detailed liquidation mechanics
- [Borrowing and Leverage](../advanced/borrowing-and-leverage.md) - Margin borrowing
