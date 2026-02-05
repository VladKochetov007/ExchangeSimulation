# Precision Model Improvement Proposal

## Current State Analysis

### How It Works Now

**Instrument Definition:**
```go
instrument := NewSpotInstrument(
    "BTC/USD",
    "BTC",              // base asset name
    "USD",              // quote asset name
    SATOSHI,            // tickSize (price alignment, NOT base precision!)
    SATOSHI/1000,       // minOrderSize (minimum order qty)
)
```

**Client Balances:**
```go
type Client struct {
    Balances map[string]int64  // Just int64, NO precision metadata!
    Reserved map[string]int64
}
```

**Key Finding:**
- ❌ Instruments do NOT store or expose base/quote precisions
- ❌ Balances have NO precision metadata attached
- ❌ Precision is entirely implicit (user must know the convention)

### Current Problems

1. **Confusing notation:**
   ```go
   "USD": 100000 * (exchange.SATOSHI / 1000)  // Why mention Bitcoin for USD?
   ```

2. **No single source of truth:**
   - Precision is NOT stored in the instrument
   - Precision is NOT stored with the balance
   - User must remember the convention

3. **Coupling to Bitcoin:**
   - `SATOSHI` constant used as base for all precisions
   - Confusing when dealing with ETH, USD, or other assets

## Proposed Solutions

### Option 1: Asset-Specific Precision Constants (Simplest)

Add clear constants in `exchange/test_helpers.go`:

```go
const (
    // Base asset precisions
    BTC_PRECISION = 100_000_000  // 1 BTC = 100,000,000 units (satoshis)
    ETH_PRECISION = 1_000_000    // 1 ETH = 1,000,000 units (micro-ETH)

    // Quote asset precisions
    USD_PRECISION = 100_000      // 1 USD = 100,000 units (0.001 USD minimum)
    USDT_PRECISION = 100_000     // Same as USD

    // Legacy alias (for backward compatibility)
    SATOSHI = BTC_PRECISION

    // Price tick sizes (for price alignment)
    CENT_TICK    = BTC_PRECISION / 100    // 0.01 USD precision
    DOLLAR_TICK  = BTC_PRECISION           // 1 USD precision
    HUNDRED_TICK = 100 * BTC_PRECISION     // 100 USD precision
)
```

**Usage:**
```go
balances := map[string]int64{
    "BTC": 10 * BTC_PRECISION,      // Clear: 10 BTC
    "USD": 100000 * USD_PRECISION,  // Clear: 100,000 USD
    "ETH": 50 * ETH_PRECISION,      // Clear: 50 ETH
}
```

**Pros:**
- ✅ Simple to implement (just add constants)
- ✅ Very clear and readable
- ✅ No coupling between assets
- ✅ Works with current code (no breaking changes)

**Cons:**
- ❌ Still requires user to know which constant to use
- ❌ Doesn't enforce correctness at compile time
- ❌ Need to add constant for each new asset

### Option 2: Precision Registry (Medium Complexity)

Create a global registry mapping asset names to precisions:

```go
package exchange

type AssetInfo struct {
    Precision int64
    Name      string
    Symbol    string
}

var AssetRegistry = map[string]AssetInfo{
    "BTC":  {Precision: 100_000_000, Name: "Bitcoin", Symbol: "₿"},
    "ETH":  {Precision: 1_000_000, Name: "Ethereum", Symbol: "Ξ"},
    "USD":  {Precision: 100_000, Name: "US Dollar", Symbol: "$"},
    "USDT": {Precision: 100_000, Name: "Tether", Symbol: "$"},
}

// Helper function
func AssetAmount(asset string, amount float64) int64 {
    info, ok := AssetRegistry[asset]
    if !ok {
        panic(fmt.Sprintf("unknown asset: %s", asset))
    }
    return int64(amount * float64(info.Precision))
}

// Or for integer amounts
func AssetBalance(asset string, wholeUnits int64) int64 {
    info, ok := AssetRegistry[asset]
    if !ok {
        panic(fmt.Sprintf("unknown asset: %s", asset))
    }
    return wholeUnits * info.Precision
}
```

**Usage:**
```go
balances := map[string]int64{
    "BTC": AssetBalance("BTC", 10),          // 10 BTC
    "USD": AssetBalance("USD", 100000),      // 100,000 USD
    "ETH": AssetAmount("ETH", 50.5),         // 50.5 ETH (float)
}
```

**Pros:**
- ✅ Single source of truth for asset precisions
- ✅ Easy to add new assets (just update registry)
- ✅ Can include metadata (name, symbol, etc.)
- ✅ Runtime validation (panics on unknown asset)

**Cons:**
- ❌ Requires maintaining a registry
- ❌ Runtime errors instead of compile-time
- ❌ Global mutable state (could be an issue)

### Option 3: Instrument-Based Precisions (Most Correct)

Add precision methods to instruments:

```go
type Instrument interface {
    Symbol() string
    BaseAsset() string
    QuoteAsset() string
    BasePrecision() int64      // NEW
    QuotePrecision() int64     // NEW
    TickSize() int64
    MinOrderSize() int64
    ValidatePrice(price int64) bool
    ValidateQty(qty int64) bool
    IsPerp() bool
    InstrumentType() string
}

type SpotInstrument struct {
    symbol         string
    base           string
    quote          string
    basePrecision  int64    // NEW
    quotePrecision int64    // NEW
    tickSize       int64
    minOrderSize   int64
}

func NewSpotInstrument(
    symbol, base, quote string,
    basePrecision, quotePrecision int64,   // NEW parameters
    tickSize, minOrderSize int64,
) *SpotInstrument {
    return &SpotInstrument{
        symbol:         symbol,
        base:           base,
        quote:          quote,
        basePrecision:  basePrecision,
        quotePrecision: quotePrecision,
        tickSize:       tickSize,
        minOrderSize:   minOrderSize,
    }
}

func (i *SpotInstrument) BasePrecision() int64 {
    return i.basePrecision
}

func (i *SpotInstrument) QuotePrecision() int64 {
    return i.quotePrecision
}

// Helper method
func (i *SpotInstrument) InitialBalances(baseAmount, quoteAmount float64) map[string]int64 {
    return map[string]int64{
        i.base:  int64(baseAmount * float64(i.basePrecision)),
        i.quote: int64(quoteAmount * float64(i.quotePrecision)),
    }
}
```

**Usage:**
```go
// Create instrument
instrument := NewSpotInstrument(
    "BTC/USD",
    "BTC", "USD",
    BTC_PRECISION, USD_PRECISION,    // Explicit precisions
    DOLLAR_TICK, SATOSHI/1000,
)

// Get balances from instrument
balances := instrument.InitialBalances(10.0, 100000.0)
// Returns: {"BTC": 1000000000, "USD": 10000000000}

// Or manually with precision methods
balances := map[string]int64{
    instrument.BaseAsset():  10 * instrument.BasePrecision(),
    instrument.QuoteAsset(): 100000 * instrument.QuotePrecision(),
}
```

**Pros:**
- ✅ Precision is part of the instrument specification
- ✅ Single source of truth (the instrument knows its precisions)
- ✅ Type-safe (linked to specific instrument)
- ✅ Helper methods make balance creation easy

**Cons:**
- ❌ Breaking change (need to update all NewSpotInstrument calls)
- ❌ More parameters to constructor
- ❌ Doesn't help with assets not in an instrument

### Option 4: Hybrid Approach (Recommended)

Combine Option 1 + Option 3:

**Step 1:** Add clear constants (no breaking changes)
```go
const (
    BTC_PRECISION  = 100_000_000
    ETH_PRECISION  = 1_000_000
    USD_PRECISION  = 100_000
    USDT_PRECISION = 100_000

    SATOSHI = BTC_PRECISION  // Backward compatibility
)
```

**Step 2:** Optionally add precisions to instruments (future enhancement)
```go
// Can be added later without breaking existing code
type InstrumentV2 interface {
    Instrument
    BasePrecision() int64
    QuotePrecision() int64
}
```

**Step 3:** Add convenience helpers
```go
// For creating balances
func BalanceMap(pairs ...struct{ asset string; amount, precision int64 }) map[string]int64 {
    m := make(map[string]int64)
    for _, p := range pairs {
        m[p.asset] = p.amount * p.precision
    }
    return m
}

// Usage:
balances := BalanceMap(
    {"BTC", 10, BTC_PRECISION},
    {"USD", 100000, USD_PRECISION},
)
```

## Recommendation

**Start with Option 1** (Asset-Specific Constants):
1. Immediate improvement in readability
2. No breaking changes
3. Simple to implement
4. Can evolve to Option 3 later if needed

**Then consider Option 3** (Instrument-Based) for v2:
- Add as optional interface extension
- Gradually migrate existing code
- Provides stronger type safety

## Implementation Plan

### Phase 1: Add Clear Constants
```go
// exchange/test_helpers.go

const (
    // Asset precisions
    BTC_PRECISION  = 100_000_000  // 1 BTC = 100M units
    ETH_PRECISION  = 1_000_000    // 1 ETH = 1M units (micro-ETH)
    USD_PRECISION  = 100_000      // 1 USD = 100K units (0.001 USD tick)
    USDT_PRECISION = 100_000      // Same as USD

    // Legacy (backward compatibility)
    SATOSHI = BTC_PRECISION

    // Price ticks (for alignment)
    CENT_TICK    = BTC_PRECISION / 100
    DOLLAR_TICK  = BTC_PRECISION
    HUNDRED_TICK = 100 * BTC_PRECISION
)
```

### Phase 2: Update Helper Functions
```go
// Fix the buggy USDAmount function
func USDAmount(usd float64) int64 {
    return int64(usd * float64(USD_PRECISION))  // Use correct precision!
}

func ETHAmount(eth float64) int64 {
    return int64(eth * float64(ETH_PRECISION))
}

// Generic version
func AssetAmount(amount float64, precision int64) int64 {
    return int64(amount * float64(precision))
}
```

### Phase 3: Update Documentation
- Update PRECISION_GUIDE.md with new constants
- Add examples using clear notation
- Mark SATOSHI usage for USD as deprecated pattern

### Phase 4: Migrate Existing Code (Gradual)
```go
// Before
"USD": 100000 * (exchange.SATOSHI / 1000)

// After
"USD": 100000 * exchange.USD_PRECISION
```

## Examples of Improved Readability

### Before (Confusing)
```go
balances := map[string]int64{
    "BTC": 10 * exchange.SATOSHI,
    "USD": 100000 * (exchange.SATOSHI / 1000),  // Why SATOSHI for USD?
    "ETH": 50 * (exchange.SATOSHI / 100),       // Even worse!
}
```

### After (Clear)
```go
balances := map[string]int64{
    "BTC": 10 * exchange.BTC_PRECISION,
    "USD": 100000 * exchange.USD_PRECISION,
    "ETH": 50 * exchange.ETH_PRECISION,
}
```

### With Type Safety (Future)
```go
instrument := NewSpotInstrument(...)
balances := instrument.CreateBalances(
    baseAmount: 10.0,      // 10 BTC
    quoteAmount: 100000.0, // 100,000 USD
)
```
