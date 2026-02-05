# Precision and Balance Guide

## Core Concept

All amounts in this simulation use **integer arithmetic** to avoid floating-point precision issues. Each asset has its own **precision multiplier**.

## Precision Constants

```go
const (
    // Asset precisions (units per whole unit)
    BTC_PRECISION  = 100_000_000 // 1 BTC = 100,000,000 satoshis
    ETH_PRECISION  = 1_000_000   // 1 ETH = 1,000,000 micro-ETH
    USD_PRECISION  = 100_000     // 1 USD = 100,000 units (0.001 USD minimum)
    USDT_PRECISION = 100_000     // Same as USD

    // Legacy constant (backward compatibility - do NOT use for non-BTC assets!)
    SATOSHI = BTC_PRECISION

    // Price tick sizes (for price alignment in BTC/USD pairs)
    CENT_TICK    = BTC_PRECISION / 100  // 0.01 USD tick
    DOLLAR_TICK  = BTC_PRECISION        // 1 USD tick
    HUNDRED_TICK = 100 * BTC_PRECISION  // 100 USD tick
)
```

## Asset Precision Rules

### BTC (Base Currency)
- **Precision**: `SATOSHI` = 100,000,000
- **1 BTC** = 100,000,000 units
- **0.01 BTC** = 1,000,000 units
- **1 satoshi** = 1 unit (smallest amount)

### USD (Quote Currency)
- **Precision**: Depends on instrument configuration
- **Common**: SATOSHI/1000 = 100,000 (0.001 USD = 1 milli-dollar)
- **1 USD** = 100,000 units
- **0.001 USD** = 1 unit (smallest amount)

⚠️ **CRITICAL**: USD does NOT use SATOSHI precision!

## Instrument Configuration

When creating instruments, specify precisions for both base and quote:

```go
instrument := exchange.NewSpotInstrument(
    "BTC/USD",           // symbol
    "BTC",               // base asset
    "USD",               // quote asset
    exchange.SATOSHI,    // basePrecision (BTC uses 100,000,000)
    exchange.SATOSHI/1000, // quotePrecision (USD uses 100,000)
)
```

### Perp Futures Example
```go
perp := exchange.NewPerpFutures(
    "BTCUSD",
    "BTC",
    "USD",
    exchange.SATOSHI,      // base precision
    exchange.SATOSHI/1000, // quote precision
)
```

## Balance Rules

### ❌ WRONG - Using SATOSHI for USD
```go
balances := map[string]int64{
    "BTC": 10 * exchange.SATOSHI,     // Acceptable (SATOSHI = BTC_PRECISION)
    "USD": 100000 * exchange.SATOSHI, // ✗ WRONG! USD ≠ BTC precision
}
```

### ✅ CORRECT - Using Asset-Specific Precisions
```go
balances := map[string]int64{
    "BTC": 10 * exchange.BTC_PRECISION,      // 10 BTC (clear and explicit)
    "USD": 100000 * exchange.USD_PRECISION,  // 100,000 USD (clear and explicit)
    "ETH": 50 * exchange.ETH_PRECISION,      // 50 ETH (if needed)
}
```

Or using helper functions (for float amounts):
```go
balances := map[string]int64{
    "BTC":  exchange.BTCAmount(10.0),      // 10 BTC
    "USD":  exchange.USDAmount(100000.0),  // 100,000 USD
    "ETH":  exchange.ETHAmount(50.5),      // 50.5 ETH
    "USDT": exchange.USDTAmount(1000.0),   // 1,000 USDT
}
```

✅ **Helper functions now use correct precisions!**

## Helper Functions (TEST ONLY)

### BTCAmount - ✅ Correct
```go
func BTCAmount(btc float64) int64 {
    return int64(btc * float64(SATOSHI))
}
```
Usage: `BTCAmount(1.5)` = 150,000,000 (1.5 BTC)

### USDAmount - ⚠️ BUGGY (uses wrong precision)
```go
func USDAmount(usd float64) int64 {
    return int64(usd * float64(SATOSHI))  // WRONG! Should use USD precision
}
```
**Should be:**
```go
func USDAmount(usd float64) int64 {
    return int64(usd * float64(SATOSHI/1000))  // Correct USD precision
}
```

### PriceUSD - ✅ Correct
```go
func PriceUSD(price float64, tickSize int64) int64 {
    raw := int64(price * float64(SATOSHI))
    return (raw / tickSize) * tickSize
}
```
Usage: `PriceUSD(50000.0, DOLLAR_TICK)` = 50000 * SATOSHI

## Common Mistakes

### Mistake 1: USD Balance Using Bitcoin Precision
```go
❌ "USD": 1000000 * SATOSHI           // This is 1 trillion USD! (wrong precision)
❌ "USD": 1000000 * (SATOSHI/1000)   // Correct value but confusing notation
✅ "USD": 1000000 * USD_PRECISION    // Clear: 1 million USD
```

### Mistake 2: Profit Calculation
```go
// Price is in SATOSHI per BTC
// Qty is in satoshis
priceDiff := fastBid - slowAsk  // in SATOSHI (per BTC)
qty := SATOSHI / 10              // 0.1 BTC

❌ profit := priceDiff * qty / SATOSHI  // Wrong! Result is in BTC terms

✅ profitUSD := (priceDiff * qty) / (SATOSHI * SATOSHI/1000)  // Correct USD amount
```

### Mistake 3: Mixing Precisions
```go
// When instrument has quotePrecision = SATOSHI/1000
instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", SATOSHI, SATOSHI/1000)

❌ balance["USD"] = 50000 * SATOSHI      // Wrong precision
✅ balance["USD"] = 50000 * (SATOSHI/1000)  // Match instrument's quotePrecision
```

## Quick Reference Table

| Asset | Precision | 1 Unit | Example Balance | Integer Value |
|-------|-----------|--------|-----------------|---------------|
| BTC   | SATOSHI (10^8) | 1 satoshi | 10 BTC | 1,000,000,000 |
| USD   | SATOSHI/1000 (10^5) | 0.001 USD | 100,000 USD | 10,000,000,000 |
| ETH   | 10^6 | 1 micro-ETH | 100 ETH | 100,000,000 |

## Conversion Examples

### BTC to Satoshis
```go
1 BTC = SATOSHI = 100,000,000
0.1 BTC = SATOSHI / 10 = 10,000,000
0.00000001 BTC = 1 satoshi
```

### USD (with SATOSHI/1000 precision)
```go
1 USD = SATOSHI / 1000 = 100,000
1000 USD = 1000 * (SATOSHI/1000) = 100,000,000
0.001 USD = 1 unit (milli-dollar)
```

### Price (BTC/USD)
```go
// Price of $50,000 per BTC
price := 50000 * SATOSHI = 5,000,000,000,000

// With DOLLAR_TICK alignment
price := PriceUSD(50000, DOLLAR_TICK) = 5,000,000,000,000
```

## Best Practices

1. **Always use instrument's precision**: Get `BasePrecision()` and `QuotePrecision()` from instrument
2. **Never hardcode SATOSHI for non-BTC assets**: Each asset has its own precision
3. **Use helper functions sparingly**: They're test-only and have limited precision
4. **Document precision in comments**: Make it clear what each value represents
5. **Test with real numbers**: Verify calculations work with actual amounts

## Code Template

```go
// Define instrument with explicit precisions
instrument := exchange.NewSpotInstrument(
    "BTC/USD",
    "BTC",
    "USD",
    exchange.BTC_PRECISION,   // Tick size (price alignment)
    exchange.SATOSHI/1000,    // Min order size
)

// Setup balances using clear, asset-specific constants
balances := map[string]int64{
    "BTC": 10 * exchange.BTC_PRECISION,      // 10 BTC
    "USD": 100000 * exchange.USD_PRECISION,  // 100,000 USD
}

// Calculate profit in quote currency (USD)
priceDiff := sellPrice - buyPrice  // in base currency precision
qtyTraded := exchange.SATOSHI / 10 // 0.1 BTC

// Profit calculation (convert to USD)
profitInBasePrecision := priceDiff * qtyTraded / exchange.SATOSHI
profitInUSD := profitInBasePrecision  // Already in USD terms from price difference
profitInUSDUnits := profitInUSD / (exchange.SATOSHI/1000)  // Convert to USD precision units
```

## Summary

- **BTC**: Use `BTC_PRECISION` (100,000,000) - clear and explicit
- **USD**: Use `USD_PRECISION` (100,000) - no Bitcoin terminology!
- **ETH**: Use `ETH_PRECISION` (1,000,000) - asset-specific constant
- **Other assets**: Add new `*_PRECISION` constants as needed
- **Legacy code**: `SATOSHI` still works for BTC (alias for `BTC_PRECISION`)
- **Prices**: Always in quote currency precision, aligned to tick size
- **Never mix precisions**: Each asset has its own precision constant
