# Instruments

Trading instruments with precision mathematics and validation rules.

## Instrument Interface

```go
type Instrument interface {
    Symbol() string
    BaseAsset() string
    QuoteAsset() string
    BasePrecision() int64
    QuotePrecision() int64
    TickSize() int64
    MinOrderSize() int64
    ValidatePrice(price int64) bool
    ValidateQty(qty int64) bool
    IsPerp() bool
    InstrumentType() string
}
```

## Precision Mathematics

All prices and quantities use **integer math** to avoid floating-point errors.

### Why Integer Math

**Problem with floats:**
```go
// WRONG: Floating point errors
price := 50123.45
qty := 1.23456789
notional := price * qty  // 61,902.4567... (precision loss)
```

**Solution: Fixed-point integers:**
```go
// CORRECT: Exact integer math
price := 50123.45 * USD_PRECISION    // 5,012,345,000,000
qty := 1.23456789 * BTC_PRECISION    // 123,456,789
notional := (price * qty) / BTC_PRECISION  // Exact
```

### Standard Precisions

```go
const (
    BTC_PRECISION = 100_000_000  // 1e8 (satoshis)
    ETH_PRECISION = 1_000_000_000_000_000_000  // 1e18 (wei)
    USD_PRECISION = 100_000_000  // 1e8 (cents with 6 decimals)

    CENT_TICK = 1_000_000  // $0.01 tick size in USD_PRECISION
)
```

**BTC_PRECISION = 1e8:**
- Matches Bitcoin's satoshi unit
- 1 BTC = 100,000,000 satoshis
- Maximum ~21M BTC fits in int64
- Standard across crypto exchanges

**USD_PRECISION = 1e8:**
- Supports prices like $50,123.456789
- Compatible with BTC precision (same exponent)
- Notional = (price × qty) / precision (no overflow for reasonable values)

### Precision Examples

**Example 1: BTC Quantity**
```
User input: 1.5 BTC
Internal:   1.5 × 100,000,000 = 150,000,000
Display:    150,000,000 / 100,000,000 = 1.5 BTC
```

**Example 2: USD Price**
```
User input: $50,123.45
Internal:   50123.45 × 100,000,000 = 5,012,345,000,000
Display:    5,012,345,000,000 / 100,000,000 = $50,123.45
```

**Example 3: Notional Calculation**
```
Price:    $50,000 → 5,000,000,000,000
Qty:      1.5 BTC → 150,000,000
Notional: (5,000,000,000,000 × 150,000,000) / 100,000,000
        = 750,000,000,000,000 / 100,000,000
        = 7,500,000,000,000 (in USD precision)
        = $75,000
```

## SpotInstrument

```go
type SpotInstrument struct {
    symbol         string
    baseAsset      string
    quoteAsset     string
    basePrecision  int64
    quotePrecision int64
    tickSize       int64
    minOrderSize   int64
}
```

### Creation

```go
inst := exchange.NewSpotInstrument(
    "BTCUSD",       // Symbol
    "BTC",          // Base asset
    "USD",          // Quote asset
    exchange.BTC_PRECISION,   // 1e8
    exchange.USD_PRECISION,   // 1e8
    exchange.CENT_TICK,       // $0.01 tick
    exchange.BTC_PRECISION/10000,  // 0.0001 BTC min
)
```

### Validation

**Price validation:**
```go
func (i *SpotInstrument) ValidatePrice(price int64) bool {
    return price > 0 && price % i.tickSize == 0
}
```

Valid prices: multiples of tick size.

**Examples:**
- Tick = $0.01 (1,000,000 in precision)
- Valid: $50,000.00, $50,000.01, $50,000.99
- Invalid: $50,000.001 (sub-tick)

**Quantity validation:**
```go
func (i *SpotInstrument) ValidateQty(qty int64) bool {
    return qty >= i.minOrderSize && qty % (i.basePrecision/100000000) == 0
}
```

## PerpFutures

Extends SpotInstrument with perpetual-specific features.

```go
type PerpFutures struct {
    SpotInstrument

    fundingRate            *FundingRate
    fundingCalc            FundingCalculator
    MarginRate             int64  // bps
    MaintenanceMarginRate  int64  // bps
    WarningMarginRate      int64  // bps
}
```

### Creation

```go
perp := exchange.NewPerpFutures(
    "BTC-PERP",
    "BTC", "USD",
    exchange.BTC_PRECISION,
    exchange.USD_PRECISION,
    exchange.CENT_TICK,
    exchange.BTC_PRECISION/10000,  // 0.0001 BTC min
)
```

**Default margin rates:**
```go
MarginRate:            1000,  // 10% (10x leverage)
MaintenanceMarginRate: 500,   // 5% (liquidation threshold)
WarningMarginRate:     750,   // 7.5% (warning level)
```

### Margin Requirements

**Initial margin (order placement):**
```go
notional := (price × qty) / precision
initialMargin := (notional × MarginRate) / 10000
```

**Example:** 10 BTC @ \$50,000, MarginRate = 1000 bps
```
Notional: $500,000
Initial margin: $500,000 × 0.10 = $50,000
Leverage: 10x
```

**Maintenance margin (liquidation):**
```go
maintenanceMargin := (notional × MaintenanceMarginRate) / 10000
```

**Example:** Same position, MaintenanceMarginRate = 500 bps
```
Maintenance margin: $500,000 × 0.05 = $25,000
Liquidation if balance < $25,000
```

### Funding Configuration

```go
fundingCalc := &exchange.SimpleFundingCalc{
    BaseRate: 10,    // 0.1% base
    Damping:  100,   // Full premium
    MaxRate:  750,   // ±7.5% cap
}
perp.SetFundingCalculator(fundingCalc)
```

## Price Conversion Helpers

```go
// Convert user-friendly price to internal representation
func PriceUSD(dollars float64, tickSize int64) int64 {
    return int64(dollars * float64(USD_PRECISION))
}

// Convert internal price to user-friendly
func PriceToFloat(price int64, precision int64) float64 {
    return float64(price) / float64(precision)
}

// Convert quantity
func QtyBTC(btc float64) int64 {
    return int64(btc * float64(BTC_PRECISION))
}
```

**Usage:**
```go
price := exchange.PriceUSD(50000.00, exchange.CENT_TICK)
// price = 5,000,000,000,000

qty := exchange.QtyBTC(1.5)
// qty = 150,000,000
```

## Type Conversions and Precision Safety

### Integer vs Float Operations

**Type rules:**
```go
// SAFE: int64 operations
var price int64 = 50000 * USD_PRECISION      // int64
var qty int64 = 10 * BTC_PRECISION           // int64
notional := (price * qty) / BTC_PRECISION    // All int64, exact

// UNSAFE: Mixing types
priceFloat := 50000.123                      // float64
qtyInt := int64(priceFloat * float64(USD_PRECISION))  // WRONG: precision loss
```

**Conversion guidelines:**
1. **Never** convert float → int in hot path (trading logic)
2. Float → int **only** at boundaries (user input, display)
3. All internal math: int64 only
4. Configuration: accept float, convert once at startup

**Safe conversion:**
```go
// User input: 50000.123 USD
func ParsePrice(userInput float64, precision int64) int64 {
    return int64(math.Round(userInput * float64(precision)))
}

price := ParsePrice(50000.123, USD_PRECISION)  // 5,000,012,300,000
```

**Display conversion:**
```go
func FormatPrice(price int64, precision int64) float64 {
    return float64(price) / float64(precision)
}

display := FormatPrice(5000012300000, USD_PRECISION)  // 50000.123
```

### Overflow Protection

**Maximum safe values (int64):**
```
Max int64:         9,223,372,036,854,775,807  (~9.2e18)
Max BTC (1e8):     92,233,720,368 BTC
Max USD (1e8):     $92,233,720,368
```

**Overflow risk analysis:**
```go
// For BTC/USD with standard precision:
price := 1_000_000 * USD_PRECISION  // $1M/BTC (hypothetical)
qty := 10_000 * BTC_PRECISION       // 10,000 BTC

// Direct multiplication:
product := price * qty  // 1e14 × 1e12 = 1e26 (OVERFLOW!)
```

**Solution 1: Divide first**
```go
notional := (price / BTC_PRECISION) * qty  // Safe: 1e6 × 1e12 = 1e18
```

**Solution 2: Validate limits**
```go
const MaxOrderSize = 1000 * BTC_PRECISION  // 1,000 BTC max
const MaxPrice = 1_000_000 * USD_PRECISION // $1M max

func ValidateOrder(price, qty int64) error {
    if qty > MaxOrderSize {
        return errors.New("order too large")
    }
    if price > MaxPrice {
        return errors.New("price too high")
    }

    // Now safe: max product = 1e6 × 1e11 × 1e8 = 1e25 / 1e8 = 1e17
    notional := (price * qty) / BTC_PRECISION
    return nil
}
```

**Solution 3: Check before multiply**
```go
func SafeMultiply(a, b, divisor int64) (int64, error) {
    if a > math.MaxInt64/b {
        return 0, errors.New("overflow")
    }
    return (a * b) / divisor, nil
}

notional, err := SafeMultiply(price, qty, BTC_PRECISION)
```

### Precision Overflow Protection

**When multiplying large precisions:**
```go
// WRONG: Overflow risk
const CUSTOM_PRECISION = 1e18  // Like Ethereum Wei
price := 50000 * CUSTOM_PRECISION
qty := 100 * CUSTOM_PRECISION
notional := (price * qty) / CUSTOM_PRECISION  // Overflow!

// RIGHT: Scale down intermediate values
price := 50000 * 1e12  // Use smaller precision for price
qty := 100 * 1e18      // Full precision for qty
notional := (price / 1e6) * (qty / 1e12)  // Result in dollars
```

**General rule:**
- If `precision1 × precision2 > 1e18`, use intermediate scaling
- Total precision in calculation ≤ 1e16 for safety margin

## Tick Size Examples

**BTC/USD:**
```go
TickSize: 1 cent = 1,000,000 (in USD_PRECISION)
```
Prices: \$50,000.00, \$50,000.01, \$50,000.02, ...

**ETH/USD (smaller asset):**
```go
TickSize: 0.01 cent = 10,000 (in USD_PRECISION)
```
Prices: \$2,500.00, \$2,500.001, \$2,500.002, ...

**Low-price asset:**
```go
TickSize: 0.0001 cent = 1,000 (in USD_PRECISION)
```
Prices: \$0.0001, \$0.0002, \$0.0003, ...

## Min Order Size

**BTC/USD:**
```go
MinOrderSize: 0.0001 BTC = 10,000 (in BTC_PRECISION)
```

**Why minimum?**
- Prevents spam (too many tiny orders)
- Ensures economical fee payment
- Reduces order book bloat

**Typical values:**
- BTC: 0.0001-0.001 BTC
- ETH: 0.001-0.01 ETH
- Small cap: 1-100 units

## Creating Custom Instruments

### Custom Spot Instrument

```go
type CustomSpotInstrument struct {
    exchange.SpotInstrument
    customField string
}

func NewCustomSpot(symbol, base, quote string, customData string) *CustomSpotInstrument {
    return &CustomSpotInstrument{
        SpotInstrument: *exchange.NewSpotInstrument(
            symbol, base, quote,
            100_000_000,  // Base precision
            100_000_000,  // Quote precision
            10_000,       // Tick size
            1_000_000,    // Min order size
        ),
        customField: customData,
    }
}
```

### High-Precision Instrument

For assets requiring more than 8 decimals (e.g., micro-cap tokens):

```go
const MICRO_PRECISION = 1_000_000_000_000  // 1e12 (12 decimals)

microInst := exchange.NewSpotInstrument(
    "SHIB/USD",
    "SHIB", "USD",
    MICRO_PRECISION,        // 12 decimals for tiny token
    100_000_000,            // Standard USD precision
    1,                      // 1e-12 USD tick (very small)
    1_000_000_000,          // 1000 tokens minimum
)
```

**Overflow check:**
```go
// With 1e12 precision:
maxPrice := math.MaxInt64 / MICRO_PRECISION / 1000  // Leave margin
// maxPrice ≈ 9e6 per token (safe for micro-caps)
```

### Custom Perpetual with Complex Margin

```go
type TieredMarginPerp struct {
    *exchange.PerpFutures
    marginTiers []MarginTier
}

type MarginTier struct {
    MaxNotional  int64
    MarginRate   int64  // bps
    MaintRate    int64  // bps
}

func (t *TieredMarginPerp) GetMarginRequirement(notional int64) (initial, maint int64) {
    for _, tier := range t.marginTiers {
        if notional <= tier.MaxNotional {
            initial = (notional * tier.MarginRate) / 10000
            maint = (notional * tier.MaintRate) / 10000
            return
        }
    }
    // Default to last tier
    lastTier := t.marginTiers[len(t.marginTiers)-1]
    initial = (notional * lastTier.MarginRate) / 10000
    maint = (notional * lastTier.MaintRate) / 10000
    return
}
```

**Usage:**
```go
tieredPerp := &TieredMarginPerp{
    PerpFutures: exchange.NewPerpFutures(...),
    marginTiers: []MarginTier{
        {MaxNotional: 50_000 * USD_PRECISION, MarginRate: 1000, MaintRate: 500},   // ≤$50k: 10x
        {MaxNotional: 250_000 * USD_PRECISION, MarginRate: 2000, MaintRate: 1000}, // ≤$250k: 5x
        {MaxNotional: math.MaxInt64, MarginRate: 3333, MaintRate: 1666},           // >$250k: 3x
    },
}
```

### Options Instrument (Custom)

```go
type OptionInstrument struct {
    symbol         string
    underlying     string
    strikePrice    int64
    expiry         int64  // Unix timestamp
    optionType     string // "call" or "put"
    basePrecision  int64
    quotePrecision int64
}

func (o *OptionInstrument) Symbol() string { return o.symbol }
func (o *OptionInstrument) IsPerp() bool { return false }
func (o *OptionInstrument) InstrumentType() string { return "option" }

// Implement remaining Instrument interface methods...

func (o *OptionInstrument) ValidateOrder(order *Order, clock Clock) error {
    if clock.NowUnix() > o.expiry {
        return errors.New("option expired")
    }
    // Additional validation...
    return nil
}
```

## Example: Production-Style Configuration

### Centralized Exchange Style

High leverage, tight spreads:

```go
btcPerp := exchange.NewPerpFutures(
    "BTCUSD",
    "BTC", "USD",
    100_000_000,
    100_000_000,
    10_000,        // $0.10 tick
    1_000_000,     // 0.01 BTC min
)
btcPerp.MarginRate = 100              // 100x max leverage
btcPerp.MaintenanceMarginRate = 40    // 0.4% maintenance
btcPerp.WarningMarginRate = 60        // 0.6% warning

fundingCalc := &SimpleFundingCalc{
    BaseRate: 0,
    Damping:  100,
    MaxRate:  200,  // ±2% cap
}
btcPerp.SetFundingCalculator(fundingCalc)
```

### Decentralized Exchange Style

Conservative leverage, wider caps:

```go
btcPerp := exchange.NewPerpFutures(
    "BTC-USD",
    "BTC", "USD",
    100_000_000,
    100_000_000,
    100_000,       // $1 tick (wider)
    100_000,       // 0.001 BTC min
)
btcPerp.MarginRate = 500              // 20x max leverage
btcPerp.MaintenanceMarginRate = 300   // 3% maintenance
btcPerp.WarningMarginRate = 400       // 4% warning

fundingCalc := &SimpleFundingCalc{
    BaseRate: 0,
    Damping:  100,
    MaxRate:  75,  // ±0.75% cap (tighter)
}
btcPerp.SetFundingCalculator(fundingCalc)

## Next Steps

- [Positions and Margin](positions-and-margin.md) - Position tracking with margin
- [Funding Rates](funding-rates.md) - Funding mechanism and formulas
- [Order Matching](order-matching.md) - How orders execute
