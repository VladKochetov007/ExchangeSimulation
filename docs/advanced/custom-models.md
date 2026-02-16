# Custom Models and Markets

Build custom instruments, matching engines, fee models, and price oracles.

## Custom Instruments

### Basic Custom Instrument

```go
type MyCustomInstrument struct {
    symbol         string
    baseAsset      string
    quoteAsset     string
    basePrecision  int64
    quotePrecision int64
    tickSize       int64
    minOrderSize   int64

    // Custom fields
    customParam1   int64
    customParam2   string
}

// Implement Instrument interface
func (i *MyCustomInstrument) Symbol() string { return i.symbol }
func (i *MyCustomInstrument) BaseAsset() string { return i.baseAsset }
func (i *MyCustomInstrument) QuoteAsset() string { return i.quoteAsset }
func (i *MyCustomInstrument) BasePrecision() int64 { return i.basePrecision }
func (i *MyCustomInstrument) QuotePrecision() int64 { return i.quotePrecision }
func (i *MyCustomInstrument) TickSize() int64 { return i.tickSize }
func (i *MyCustomInstrument) MinOrderSize() int64 { return i.minOrderSize }
func (i *MyCustomInstrument) IsPerp() bool { return false }
func (i *MyCustomInstrument) InstrumentType() string { return "custom" }

func (i *MyCustomInstrument) ValidatePrice(price int64) bool {
    return price > 0 && price%i.tickSize == 0
}

func (i *MyCustomInstrument) ValidateQty(qty int64) bool {
    return qty >= i.minOrderSize
}
```

### Custom Precision Instrument

Higher precision for specific use cases:

```go
const (
    NANO_PRECISION = 1_000_000_000_000_000_000  // 1e18 (like Ethereum Wei)
)

type NanoPrecisionInstrument struct {
    symbol         string
    baseAsset      string
    quoteAsset     string
    basePrecision  int64  // 1e18
    quotePrecision int64  // 1e18
    tickSize       int64
    minOrderSize   int64
}

func NewNanoInstrument(symbol, base, quote string) *NanoPrecisionInstrument {
    return &NanoPrecisionInstrument{
        symbol:         symbol,
        baseAsset:      base,
        quoteAsset:     quote,
        basePrecision:  NANO_PRECISION,
        quotePrecision: NANO_PRECISION,
        tickSize:       NANO_PRECISION / 1000000,  // 1e-12 precision
        minOrderSize:   NANO_PRECISION / 1000,     // 0.001 unit min
    }
}

// IMPORTANT: Notional calculation must avoid overflow
func (i *NanoPrecisionInstrument) CalculateNotional(price, qty int64) int64 {
    // Divide one operand first to prevent overflow
    return (price / 1e9) * (qty / 1e9)  // Result in standard precision
}
```

**Type safety:**
```go
// NEVER mix precision types
var nanoPrice int64 = 1000 * NANO_PRECISION      // 1e21
var standardQty int64 = 10 * BTC_PRECISION       // 1e9

// WRONG: Direct multiply overflows
// notional := nanoPrice * standardQty  // OVERFLOW

// CORRECT: Scale down first
notional := (nanoPrice / 1e9) * standardQty  // Safe
```

### Futures with Expiry

```go
type ExpiringFutures struct {
    *exchange.SpotInstrument
    expiryTime      int64  // Unix nano
    settlementPrice int64
    settled         bool
}

func (f *ExpiringFutures) IsExpired(clock exchange.Clock) bool {
    return clock.NowUnixNano() >= f.expiryTime
}

func (f *ExpiringFutures) Settle(finalPrice int64) {
    f.settlementPrice = finalPrice
    f.settled = true
}

func (f *ExpiringFutures) ValidateOrder(order *Order, clock Clock) error {
    if f.IsExpired(clock) {
        return errors.New("instrument expired")
    }
    return nil
}
```

### Options Instrument

```go
type OptionInstrument struct {
    symbol         string
    underlying     string
    strikePrice    int64
    expiryTime     int64
    optionType     string  // "call" or "put"
    basePrecision  int64
    quotePrecision int64

    // Greeks calculation
    volatility     float64
    riskFreeRate   float64
}

func (o *OptionInstrument) Symbol() string { return o.symbol }
func (o *OptionInstrument) IsPerp() bool { return false }
func (o *OptionInstrument) InstrumentType() string { return "option" }

func (o *OptionInstrument) CalculateIntrinsicValue(spotPrice int64) int64 {
    if o.optionType == "call" {
        if spotPrice > o.strikePrice {
            return spotPrice - o.strikePrice
        }
    } else {  // put
        if o.strikePrice > spotPrice {
            return o.strikePrice - spotPrice
        }
    }
    return 0
}

func (o *OptionInstrument) BlackScholesPrice(
    spotPrice int64,
    timeToExpiry float64,
) float64 {
    // Implement Black-Scholes formula
    // Convert int64 prices to float64, calculate, convert back
    S := float64(spotPrice) / float64(o.quotePrecision)
    K := float64(o.strikePrice) / float64(o.quotePrecision)

    // Black-Scholes calculation...
    // Return in precision units
}
```

## Custom Matching Engines

### Priority Queue Matcher

```go
type PriorityMatcher struct {
    getPriority func(order *Order) int64
}

func (m *PriorityMatcher) Match(
    bidBook, askBook *exchange.Book,
    order *exchange.Order,
) *exchange.MatchResult {
    book := askBook
    if order.Side == exchange.Sell {
        book = bidBook
    }

    var executions []*exchange.Execution

    // Get all orders at best price
    bestOrders := m.getOrdersAtPrice(book, book.Best.Price)

    // Sort by priority
    sort.Slice(bestOrders, func(i, j int) bool {
        return m.getPriority(bestOrders[i]) > m.getPriority(bestOrders[j])
    })

    // Match in priority order
    for _, resting := range bestOrders {
        if order.FilledQty >= order.Qty {
            break
        }

        fillQty := min(order.Qty-order.FilledQty, resting.Qty-resting.FilledQty)

        exec := &exchange.Execution{
            TakerOrderID: order.ID,
            MakerOrderID: resting.ID,
            Price:        resting.Price,
            Qty:          fillQty,
        }
        executions = append(executions, exec)

        order.FilledQty += fillQty
        resting.FilledQty += fillQty
    }

    return &exchange.MatchResult{Executions: executions}
}
```

### Pro-Rata Matcher

```go
type ProRataMatcher struct {
    minAllocation int64  // Minimum fill size
}

func (m *ProRataMatcher) Match(
    bidBook, askBook *exchange.Book,
    order *exchange.Order,
) *exchange.MatchResult {
    book := askBook
    if order.Side == exchange.Sell {
        book = bidBook
    }

    // Get all orders at best price
    restingOrders := m.getOrdersAtPrice(book, book.Best.Price)

    // Calculate total size at level
    totalSize := int64(0)
    for _, o := range restingOrders {
        totalSize += (o.Qty - o.FilledQty)
    }

    var executions []*exchange.Execution
    remaining := order.Qty - order.FilledQty

    // Distribute proportionally
    for _, resting := range restingOrders {
        restingRemaining := resting.Qty - resting.FilledQty

        // Calculate pro-rata share
        share := (restingRemaining * remaining) / totalSize

        if share < m.minAllocation {
            continue  // Skip small allocations
        }

        fillQty := min(share, restingRemaining, remaining)

        exec := &exchange.Execution{
            TakerOrderID: order.ID,
            MakerOrderID: resting.ID,
            Price:        resting.Price,
            Qty:          fillQty,
        }
        executions = append(executions, exec)

        order.FilledQty += fillQty
        resting.FilledQty += fillQty
        remaining -= fillQty
    }

    return &exchange.MatchResult{Executions: executions}
}
```

## Custom Fee Models

### Tiered Fee Model

```go
type TieredFeeModel struct {
    tiers []FeeTier
}

type FeeTier struct {
    MinVolume  int64  // 30-day volume threshold
    MakerBps   int64
    TakerBps   int64
}

func (f *TieredFeeModel) CalculateFee(
    exec *exchange.Execution,
    side exchange.Side,
    isMaker bool,
    baseAsset, quoteAsset string,
    precision int64,
) exchange.Fee {
    // Get client's 30-day volume
    volume := f.getClientVolume(exec.TakerClientID)

    // Find applicable tier
    var tier FeeTier
    for i := len(f.tiers) - 1; i >= 0; i-- {
        if volume >= f.tiers[i].MinVolume {
            tier = f.tiers[i]
            break
        }
    }

    bps := tier.TakerBps
    if isMaker {
        bps = tier.MakerBps
    }

    notional := (exec.Price * exec.Qty) / precision
    feeAmount := (notional * bps) / 10000

    return exchange.Fee{
        Asset:  quoteAsset,
        Amount: feeAmount,
    }
}
```

### Maker Rebate Model

```go
type RebateFeeModel struct {
    TakerBps int64  // Positive (charge)
    MakerBps int64  // Negative (rebate)
}

func (f *RebateFeeModel) CalculateFee(...) exchange.Fee {
    bps := f.TakerBps
    if isMaker {
        bps = f.MakerBps  // Negative value
    }

    notional := (exec.Price * exec.Qty) / precision
    feeAmount := (notional * bps) / 10000  // Negative = rebate

    return exchange.Fee{
        Asset:  quoteAsset,
        Amount: feeAmount,  // Can be negative
    }
}
```

## Custom Price Oracles

### Weighted Average Oracle

```go
type WeightedAverageOracle struct {
    sources map[string]PriceSource
    weights map[string]int64  // Weights sum to 10000 (100%)
}

type PriceSource interface {
    GetPrice(symbol string) int64
}

func (o *WeightedAverageOracle) GetPrice(asset string) int64 {
    weightedSum := int64(0)
    totalWeight := int64(0)

    for sourceName, source := range o.sources {
        price := source.GetPrice(asset)
        if price == 0 {
            continue  // Skip unavailable sources
        }

        weight := o.weights[sourceName]
        weightedSum += price * weight
        totalWeight += weight
    }

    if totalWeight == 0 {
        return 0  // No sources available
    }

    return weightedSum / totalWeight
}
```

### TWAP Oracle

```go
type TWAPOracle struct {
    prices    []PricePoint
    window    time.Duration
    clock     exchange.Clock
}

type PricePoint struct {
    Price     int64
    Timestamp int64
}

func (o *TWAPOracle) AddPrice(price int64) {
    o.prices = append(o.prices, PricePoint{
        Price:     price,
        Timestamp: o.clock.NowUnixNano(),
    })

    // Remove old prices outside window
    cutoff := o.clock.NowUnixNano() - int64(o.window)
    for len(o.prices) > 0 && o.prices[0].Timestamp < cutoff {
        o.prices = o.prices[1:]
    }
}

func (o *TWAPOracle) GetPrice(asset string) int64 {
    if len(o.prices) == 0 {
        return 0
    }

    sum := int64(0)
    for _, p := range o.prices {
        sum += p.Price
    }

    return sum / int64(len(o.prices))
}
```

## Clock Customization

### Stepped Clock

Advances in fixed increments:

```go
type SteppedClock struct {
    current int64
    step    time.Duration
    mu      sync.RWMutex
}

func (c *SteppedClock) NowUnixNano() int64 {
    c.mu.RLock()
    defer c.mu.RUnlock()
    return c.current
}

func (c *SteppedClock) Tick() {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.current += int64(c.step)
}
```

### Historical Replay Clock

```go
type HistoricalClock struct {
    events    []TimestampedEvent
    index     int
    mu        sync.RWMutex
}

type TimestampedEvent struct {
    Timestamp int64
    Data      interface{}
}

func (c *HistoricalClock) NowUnixNano() int64 {
    c.mu.RLock()
    defer c.mu.RUnlock()

    if c.index >= len(c.events) {
        return c.events[len(c.events)-1].Timestamp
    }
    return c.events[c.index].Timestamp
}

func (c *HistoricalClock) NextEvent() interface{} {
    c.mu.Lock()
    defer c.mu.Unlock()

    if c.index >= len(c.events) {
        return nil
    }

    event := c.events[c.index]
    c.index++
    return event.Data
}
```

## Complete Custom Market Example

```go
type CustomMarket struct {
    ex            *exchange.Exchange
    instrument    *MyCustomInstrument
    matcher       exchange.MatchingEngine
    feeModel      exchange.FeeModel
    priceOracle   PriceOracle
    clock         exchange.Clock
}

func NewCustomMarket() *CustomMarket {
    // Custom clock
    clock := simulation.NewSimulatedClock(time.Now().UnixNano())

    // Custom instrument with high precision
    inst := &NanoPrecisionInstrument{
        symbol:         "CUSTOM/USD",
        basePrecision:  NANO_PRECISION,
        quotePrecision: NANO_PRECISION,
        tickSize:       NANO_PRECISION / 1000000,
    }

    // Custom matcher (pro-rata)
    matcher := &ProRataMatcher{
        minAllocation: NANO_PRECISION / 1000,
    }

    // Custom fees (tiered with rebates)
    feeModel := &TieredFeeModel{
        tiers: []FeeTier{
            {MinVolume: 0, MakerBps: 5, TakerBps: 10},
            {MinVolume: 1000000 * USD_PRECISION, MakerBps: -2, TakerBps: 8},
        },
    }

    // Custom oracle (weighted average)
    oracle := &WeightedAverageOracle{
        sources: map[string]PriceSource{
            "source1": &ExternalAPI{},
            "source2": &OnChainOracle{},
        },
        weights: map[string]int64{
            "source1": 7000,  // 70%
            "source2": 3000,  // 30%
        },
    }

    // Create exchange with custom components
    ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
        ID:    "custom_market",
        Clock: clock,
    })

    // Set custom matcher (if exchange supports it)
    // ex.SetMatcher(matcher)

    ex.AddInstrument(inst)

    return &CustomMarket{
        ex:          ex,
        instrument:  inst,
        matcher:     matcher,
        feeModel:    feeModel,
        priceOracle: oracle,
        clock:       clock,
    }
}
```

## Best Practices

**Precision handling:**
- Always use int64 for prices/quantities
- Document precision in type/struct comments
- Validate overflow in critical paths
- Use consistent precision within instrument

**Type conversions:**
```go
// GOOD: Explicit, safe conversions
price := int64(math.Round(floatPrice * float64(precision)))

// BAD: Implicit truncation
price := int64(floatPrice * float64(precision))  // Truncates
```

**Custom validation:**
```go
func (i *MyInstrument) ValidateOrder(order *Order) error {
    // Price validation
    if err := i.ValidatePrice(order.Price); err != nil {
        return err
    }

    // Quantity validation
    if err := i.ValidateQty(order.Qty); err != nil {
        return err
    }

    // Custom business logic
    if order.Qty > i.maxSingleOrder {
        return errors.New("order exceeds maximum size")
    }

    return nil
}
```

## Next Steps

- [Instruments](../core-concepts/instruments.md) - Base instrument types
- [Funding Rates](../core-concepts/funding-rates.md) - Custom funding models
- [Price Oracles](price-oracles.md) - Oracle implementations
