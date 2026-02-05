# Precision Library Design Fix

## Problem Statement

Current implementation **VIOLATES library design principles**:

❌ Hardcoded precision constants in library code
❌ Users must modify `exchange/test_helpers.go` to add new assets
❌ No way to use custom precisions without editing library
❌ Precisions are not part of the instrument specification

From CLAUDE.md:
> A design is **invalid** if:
> - extending behavior requires editing library files
> - functionality is centralized in "registry" files that must be modified

## Current Bad Design

```go
// exchange/test_helpers.go - LIBRARY FILE
const (
    BTC_PRECISION = 100_000_000  // User can't change this!
    USD_PRECISION = 100_000      // User can't add custom assets!
)
```

**Problem:** User wants to simulate DOGE/JPY with custom precisions - they'd have to edit library code!

## Correct Solution: Make Instrument Store Precisions

### Step 1: Extend Instrument Interface

```go
// exchange/instrument.go
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
```

### Step 2: Update SpotInstrument

```go
// exchange/instrument.go
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
    basePrecision, quotePrecision int64,   // NEW
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

// Convenience method
func (i *SpotInstrument) MakeBalance(baseAmount, quoteAmount int64) map[string]int64 {
    return map[string]int64{
        i.base:  baseAmount * i.basePrecision,
        i.quote: quoteAmount * i.quotePrecision,
    }
}
```

### Step 3: Update PerpFutures

```go
func NewPerpFutures(
    symbol, base, quote string,
    basePrecision, quotePrecision int64,   // NEW
    tickSize, minOrderSize int64,
) *PerpFutures {
    return &PerpFutures{
        SpotInstrument: SpotInstrument{
            symbol:         symbol,
            base:           base,
            quote:          quote,
            basePrecision:  basePrecision,
            quotePrecision: quotePrecision,
            tickSize:       tickSize,
            minOrderSize:   minOrderSize,
        },
        // ... funding rate initialization
    }
}
```

### Step 4: Keep Test Helpers as DEFAULTS (not requirements)

```go
// exchange/test_helpers.go - DEFAULT VALUES for common cases
const (
    // Common asset precisions (use these or define your own!)
    BTC_PRECISION  = 100_000_000
    ETH_PRECISION  = 1_000_000
    USD_PRECISION  = 100_000
    USDT_PRECISION = 100_000

    // Legacy
    SATOSHI = BTC_PRECISION

    // Common tick sizes
    CENT_TICK    = BTC_PRECISION / 100
    DOLLAR_TICK  = BTC_PRECISION
    HUNDRED_TICK = 100 * BTC_PRECISION
)

// Helper for common case (NOT required!)
func BTCAmount(btc float64) int64 {
    return int64(btc * float64(BTC_PRECISION))
}

// Generic helper - works with ANY precision
func Amount(value float64, precision int64) int64 {
    return int64(value * float64(precision))
}
```

## User Code Examples

### Example 1: Standard BTC/USD (using defaults)

```go
import "exchange_sim/exchange"

// User can use provided defaults if they want
instrument := exchange.NewSpotInstrument(
    "BTC/USD",
    "BTC", "USD",
    exchange.BTC_PRECISION,  // Use default
    exchange.USD_PRECISION,  // Use default
    exchange.DOLLAR_TICK,
    exchange.SATOSHI/1000,
)

balances := instrument.MakeBalance(10, 100000)
// Returns: {"BTC": 1000000000, "USD": 10000000000}
```

### Example 2: Custom Asset (user defines precision)

```go
// User defines their own precisions - NO library modification needed!
const (
    DOGE_PRECISION = 100_000_000_000  // 1 DOGE = 100B units
    JPY_PRECISION  = 100              // 1 JPY = 100 units (0.01 yen)
)

instrument := exchange.NewSpotInstrument(
    "DOGE/JPY",
    "DOGE", "JPY",
    DOGE_PRECISION,    // Custom precision
    JPY_PRECISION,     // Custom precision
    JPY_PRECISION,     // Tick size
    DOGE_PRECISION/100, // Min order
)

balances := instrument.MakeBalance(1000, 50000)
// Returns: {"DOGE": 100000000000000, "JPY": 5000000}
```

### Example 3: Generic Helper

```go
// User code - no dependency on library constants
myBalances := map[string]int64{
    "BTC":  exchange.Amount(5.5, instrument.BasePrecision()),
    "USD":  exchange.Amount(100000.0, instrument.QuotePrecision()),
    "DOGE": exchange.Amount(1000.0, DOGE_PRECISION),
}
```

## Migration Path

### Phase 1: Add precision fields (BACKWARD COMPATIBLE)

Add optional precision parameters with defaults:

```go
func NewSpotInstrument(
    symbol, base, quote string,
    tickSize, minOrderSize int64,
    opts ...InstrumentOption,
) *SpotInstrument {
    inst := &SpotInstrument{
        symbol:         symbol,
        base:           base,
        quote:          quote,
        basePrecision:  inferPrecision(base),  // Default
        quotePrecision: inferPrecision(quote), // Default
        tickSize:       tickSize,
        minOrderSize:   minOrderSize,
    }

    for _, opt := range opts {
        opt(inst)
    }

    return inst
}

type InstrumentOption func(*SpotInstrument)

func WithPrecisions(base, quote int64) InstrumentOption {
    return func(i *SpotInstrument) {
        i.basePrecision = base
        i.quotePrecision = quote
    }
}

func inferPrecision(asset string) int64 {
    // Sensible defaults (can be overridden)
    switch asset {
    case "BTC": return BTC_PRECISION
    case "ETH": return ETH_PRECISION
    case "USD", "USDT": return USD_PRECISION
    default: return 1_000_000 // Generic default
    }
}
```

**Old code still works:**
```go
instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", tickSize, minSize)
// Uses inferred precisions
```

**New code can override:**
```go
instrument := exchange.NewSpotInstrument(
    "BTC/USD", "BTC", "USD",
    tickSize, minSize,
    exchange.WithPrecisions(BTC_PRECISION, USD_PRECISION),
)
```

### Phase 2: Update Existing Code

Gradually update tests and examples to use new explicit API.

### Phase 3: Deprecate Inference (Optional)

After migration, make precisions required parameters.

## Benefits

✅ **Library principle**: User can use ANY precision without editing library
✅ **Single source of truth**: Instrument knows its precisions
✅ **Type safe**: Balances are linked to instrument specification
✅ **Backward compatible**: Can add via options pattern first
✅ **Extensible**: User can add custom assets in their own code
✅ **No global state**: Each instrument has its own configuration

## Implementation Checklist

- [ ] Add BasePrecision() and QuotePrecision() to Instrument interface
- [ ] Add precision fields to SpotInstrument and PerpFutures
- [ ] Update NewSpotInstrument and NewPerpFutures signatures
- [ ] Add MakeBalance() convenience method to instruments
- [ ] Update all existing NewSpotInstrument calls in tests
- [ ] Add generic Amount() helper that takes precision parameter
- [ ] Update documentation to show custom precision usage
- [ ] Mark test_helpers.go constants as "DEFAULTS" not "REQUIREMENTS"
