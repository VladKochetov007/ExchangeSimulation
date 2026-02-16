# Creating Custom Actors

Build a custom trading actor from scratch.

## Actor Template

```go
package main

import (
    "context"
    "time"

    "exchange_sim/actor"
    "exchange_sim/exchange"
    "exchange_sim/simulation"
)

type SimpleTrader struct {
    *actor.BaseActor
    config SimpleTraderConfig

    instrument    exchange.Instrument
    tickerFactory simulation.TickerFactory

    position      int64
    activeOrderID uint64
}

type SimpleTraderConfig struct {
    Symbol      string
    Instrument  exchange.Instrument
    Interval    time.Duration
    TradeSize   int64
}

func NewSimpleTrader(
    id uint64,
    gateway *exchange.ClientGateway,
    cfg SimpleTraderConfig,
) *SimpleTrader {
    return &SimpleTrader{
        BaseActor:  actor.NewBaseActor(id, gateway),
        config:     cfg,
        instrument: cfg.Instrument,
    }
}

func (st *SimpleTrader) SetTickerFactory(factory simulation.TickerFactory) {
    st.tickerFactory = factory
}

func (st *SimpleTrader) Start(ctx context.Context) error {
    st.BaseActor.Start(ctx)

    ticker := st.tickerFactory.NewTicker(st.config.Interval)
    go st.tradingLoop(ctx, ticker)

    return nil
}

func (st *SimpleTrader) tradingLoop(ctx context.Context, ticker simulation.Ticker) {
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C():
            st.trade()
        }
    }
}

func (st *SimpleTrader) trade() {
    // Trading logic here
}

func (st *SimpleTrader) OnEvent(event *actor.Event) {
    switch event.Type {
    case actor.EventOrderAccepted:
        st.handleOrderAccepted(event.Data.(*actor.OrderAcceptedEvent))
    case actor.EventOrderFilled:
        st.handleOrderFilled(event.Data.(*actor.OrderFillEvent))
    }
}

func (st *SimpleTrader) handleOrderAccepted(e *actor.OrderAcceptedEvent) {
    st.activeOrderID = e.OrderID
}

func (st *SimpleTrader) handleOrderFilled(e *actor.OrderFillEvent) {
    if e.Side == exchange.Buy {
        st.position += e.Qty
    } else {
        st.position -= e.Qty
    }
    st.activeOrderID = 0
}
```

## Example 1: Mean Reversion Trader

Buys when price drops, sells when price rises.

```go
type MeanReversionConfig struct {
    Symbol         string
    Instrument     exchange.Instrument
    CheckInterval  time.Duration
    TargetPrice    int64
    Threshold      int64  // Price deviation to trigger trade
    TradeSize      int64
    MaxPosition    int64
}

type MeanReversionTrader struct {
    *actor.BaseActor
    config        MeanReversionConfig
    tickerFactory simulation.TickerFactory

    lastPrice     int64
    position      int64
}

func (mrt *MeanReversionTrader) tradingLoop(ctx context.Context, ticker simulation.Ticker) {
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C():
            mrt.checkMeanReversion()
        }
    }
}

func (mrt *MeanReversionTrader) checkMeanReversion() {
    if mrt.lastPrice == 0 {
        return  // No price data yet
    }

    deviation := mrt.lastPrice - mrt.config.TargetPrice

    if deviation > mrt.config.Threshold && mrt.position > -mrt.config.MaxPosition {
        // Price too high, sell
        mrt.SubmitOrder(&actor.OrderRequest{
            Symbol: mrt.config.Symbol,
            Side:   exchange.Sell,
            Type:   exchange.Market,
            Qty:    mrt.config.TradeSize,
        })
    } else if deviation < -mrt.config.Threshold && mrt.position < mrt.config.MaxPosition {
        // Price too low, buy
        mrt.SubmitOrder(&actor.OrderRequest{
            Symbol: mrt.config.Symbol,
            Side:   exchange.Buy,
            Type:   exchange.Market,
            Qty:    mrt.config.TradeSize,
        })
    }
}

func (mrt *MeanReversionTrader) OnEvent(event *actor.Event) {
    switch event.Type {
    case actor.EventTrade:
        e := event.Data.(*actor.TradeEvent)
        mrt.lastPrice = e.Trade.Price

    case actor.EventOrderFilled:
        e := event.Data.(*actor.OrderFillEvent)
        if e.Side == exchange.Buy {
            mrt.position += e.Qty
        } else {
            mrt.position -= e.Qty
        }
    }
}
```

### Usage

```go
func main() {
    ex := setupExchange()

    gateway := ex.ConnectClient(100, map[string]int64{
        "USD": 1000000 * exchange.USD_PRECISION,
    }, &exchange.FixedFee{})

    trader := NewMeanReversionTrader(100, gateway, MeanReversionConfig{
        Symbol:        "BTC-PERP",
        Instrument:    perpInst,
        CheckInterval: 5 * time.Second,
        TargetPrice:   exchange.PriceUSD(50000, exchange.CENT_TICK),
        Threshold:     exchange.PriceUSD(500, exchange.CENT_TICK),  // $500
        TradeSize:     1 * exchange.BTC_PRECISION,  // 1 BTC
        MaxPosition:   10 * exchange.BTC_PRECISION,  // ±10 BTC
    })

    trader.SetTickerFactory(tickerFactory)
    trader.Start(ctx)

    // Subscribe to trades for price updates
    trader.Subscribe("BTC-PERP", actor.MDTrade)
}
```

## Example 2: Spread Trader

Trades the spread between two instruments.

```go
type SpreadTraderConfig struct {
    Symbol1      string
    Symbol2      string
    Instrument1  exchange.Instrument
    Instrument2  exchange.Instrument
    CheckInterval time.Duration
    SpreadThreshold int64  // In bps
    TradeSize    int64
}

type SpreadTrader struct {
    *actor.BaseActor
    config        SpreadTraderConfig
    tickerFactory simulation.TickerFactory

    price1        int64
    price2        int64
    position1     int64
    position2     int64
}

func (st *SpreadTrader) checkSpread() {
    if st.price1 == 0 || st.price2 == 0 {
        return
    }

    // Calculate spread in bps
    spread := ((st.price1 - st.price2) * 10000) / st.price2

    if spread > st.config.SpreadThreshold {
        // Spread too wide: sell instrument1, buy instrument2
        st.SubmitOrder(&actor.OrderRequest{
            Symbol: st.config.Symbol1,
            Side:   exchange.Sell,
            Type:   exchange.Market,
            Qty:    st.config.TradeSize,
        })
        st.SubmitOrder(&actor.OrderRequest{
            Symbol: st.config.Symbol2,
            Side:   exchange.Buy,
            Type:   exchange.Market,
            Qty:    st.config.TradeSize,
        })
    } else if spread < -st.config.SpreadThreshold {
        // Spread too narrow: buy instrument1, sell instrument2
        st.SubmitOrder(&actor.OrderRequest{
            Symbol: st.config.Symbol1,
            Side:   exchange.Buy,
            Type:   exchange.Market,
            Qty:    st.config.TradeSize,
        })
        st.SubmitOrder(&actor.OrderRequest{
            Symbol: st.config.Symbol2,
            Side:   exchange.Sell,
            Type:   exchange.Market,
            Qty:    st.config.TradeSize,
        })
    }
}

func (st *SpreadTrader) OnEvent(event *actor.Event) {
    switch event.Type {
    case actor.EventTrade:
        e := event.Data.(*actor.TradeEvent)
        if e.Symbol == st.config.Symbol1 {
            st.price1 = e.Trade.Price
        } else if e.Symbol == st.config.Symbol2 {
            st.price2 = e.Trade.Price
        }

    case actor.EventOrderFilled:
        e := event.Data.(*actor.OrderFillEvent)
        delta := e.Qty
        if e.Side == exchange.Sell {
            delta = -delta
        }

        if e.Symbol == st.config.Symbol1 {
            st.position1 += delta
        } else if e.Symbol == st.config.Symbol2 {
            st.position2 += delta
        }
    }
}
```

## Example 3: TWAP Executor

Time-Weighted Average Price execution.

```go
type TWAPConfig struct {
    Symbol       string
    Instrument   exchange.Instrument
    TotalQty     int64
    Duration     time.Duration
    SliceCount   int
}

type TWAPExecutor struct {
    *actor.BaseActor
    config        TWAPConfig
    tickerFactory simulation.TickerFactory

    sliceQty      int64
    sliceInterval time.Duration
    executed      int64
    slicesLeft    int
}

func NewTWAPExecutor(id uint64, gateway *exchange.ClientGateway, cfg TWAPConfig) *TWAPExecutor {
    sliceQty := cfg.TotalQty / int64(cfg.SliceCount)
    sliceInterval := cfg.Duration / time.Duration(cfg.SliceCount)

    return &TWAPExecutor{
        BaseActor:     actor.NewBaseActor(id, gateway),
        config:        cfg,
        sliceQty:      sliceQty,
        sliceInterval: sliceInterval,
        slicesLeft:    cfg.SliceCount,
    }
}

func (te *TWAPExecutor) tradingLoop(ctx context.Context, ticker simulation.Ticker) {
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C():
            if te.slicesLeft > 0 {
                te.executeSlice()
            }
        }
    }
}

func (te *TWAPExecutor) executeSlice() {
    qty := te.sliceQty
    if te.slicesLeft == 1 {
        // Last slice: execute remaining
        qty = te.config.TotalQty - te.executed
    }

    te.SubmitOrder(&actor.OrderRequest{
        Symbol: te.config.Symbol,
        Side:   exchange.Buy,
        Type:   exchange.Market,
        Qty:    qty,
    })
}

func (te *TWAPExecutor) OnEvent(event *actor.Event) {
    switch event.Type {
    case actor.EventOrderFilled:
        e := event.Data.(*actor.OrderFillEvent)
        te.executed += e.Qty

        if e.FilledQty >= e.Qty {
            // Slice fully filled
            te.slicesLeft--
        }
    }
}
```

### Usage

```go
// Execute 100 BTC over 1 hour in 60 slices (1 per minute)
twap := NewTWAPExecutor(200, gateway, TWAPConfig{
    Symbol:     "BTC-PERP",
    Instrument: perpInst,
    TotalQty:   100 * exchange.BTC_PRECISION,
    Duration:   1 * time.Hour,
    SliceCount: 60,
})

twap.SetTickerFactory(tickerFactory)
twap.Start(ctx)
```

## Using OMS for Position Tracking

```go
type MyTrader struct {
    *actor.BaseActor
    oms actor.OMS
}

func NewMyTrader(id uint64, gateway *exchange.ClientGateway) *MyTrader {
    return &MyTrader{
        BaseActor: actor.NewBaseActor(id, gateway),
        oms:       actor.NewNettingOMS(),
    }
}

func (mt *MyTrader) OnEvent(event *actor.Event) {
    switch event.Type {
    case actor.EventOrderFilled:
        e := event.Data.(*actor.OrderFillEvent)

        // Update OMS
        mt.oms.OnFill(e.Symbol, *e, mt.config.Instrument.BasePrecision())

        // Get net position
        pos := mt.oms.GetNetPosition(e.Symbol)

        // Get position details
        position := mt.oms.GetPosition(e.Symbol)
        fmt.Printf("Position: %d @ $%.2f\n",
            position.Size,
            float64(position.EntryPrice)/float64(exchange.USD_PRECISION))
    }
}
```

## Market Data Subscriptions

```go
func (mt *MyTrader) Start(ctx context.Context) error {
    mt.BaseActor.Start(ctx)

    // Subscribe to market data
    mt.Subscribe(mt.config.Symbol,
        actor.MDTrade | actor.MDSnapshot | actor.MDFunding)

    ticker := mt.tickerFactory.NewTicker(mt.config.Interval)
    go mt.tradingLoop(ctx, ticker)

    return nil
}

func (mt *MyTrader) OnEvent(event *actor.Event) {
    switch event.Type {
    case actor.EventTrade:
        e := event.Data.(*actor.TradeEvent)
        mt.handleTrade(e.Trade)

    case actor.EventBookSnapshot:
        e := event.Data.(*actor.BookSnapshotEvent)
        mt.handleBookSnapshot(e)

    case actor.EventFundingUpdate:
        e := event.Data.(*actor.FundingUpdateEvent)
        mt.handleFunding(e.FundingRate)
    }
}
```

## Error Handling

```go
func (mt *MyTrader) trade() {
    err := mt.SubmitOrder(&actor.OrderRequest{
        Symbol: mt.config.Symbol,
        Side:   exchange.Buy,
        Qty:    mt.config.TradeSize,
        Type:   exchange.Market,
    })

    if err != nil {
        // Channel full or gateway closed
        fmt.Printf("Failed to submit order: %v\n", err)
        return
    }
}

func (mt *MyTrader) OnEvent(event *actor.Event) {
    switch event.Type {
    case actor.EventOrderRejected:
        e := event.Data.(*actor.OrderRejectedEvent)

        switch e.Reason {
        case exchange.RejectInsufficientBalance:
            fmt.Println("Insufficient balance")
        case exchange.RejectInvalidPrice:
            fmt.Println("Invalid price")
        case exchange.RejectInsufficientMargin:
            fmt.Println("Insufficient margin")
        }
    }
}
```

## Testing Your Actor

```go
func TestMyTrader(t *testing.T) {
    simClock := simulation.NewSimulatedClock(0)
    scheduler := simulation.NewEventScheduler(simClock)
    simClock.SetScheduler(scheduler)

    ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
        Clock: simClock,
        TickerFactory: simulation.NewSimTickerFactory(scheduler),
    })

    inst := exchange.NewPerpFutures("BTC-PERP", "BTC", "USD",
        exchange.BTC_PRECISION, exchange.USD_PRECISION,
        exchange.CENT_TICK, exchange.BTC_PRECISION/10000)
    ex.AddInstrument(inst)

    gateway := ex.ConnectClient(1, map[string]int64{
        "USD": 1000000 * exchange.USD_PRECISION,
    }, &exchange.FixedFee{})
    ex.AddPerpBalance(1, "USD", 1000000 * exchange.USD_PRECISION)

    trader := NewMyTrader(1, gateway, MyConfig{
        Symbol:   "BTC-PERP",
        Interval: 1 * time.Second,
    })
    trader.SetTickerFactory(simulation.NewSimTickerFactory(scheduler))
    trader.Start(context.Background())

    // Advance time and check behavior
    simClock.Advance(1 * time.Second)
    // Trader should have traded once

    trader.Stop()
}
```

## Best Practices

**Configuration:**
- Immutable config struct
- Validate config in constructor
- Use dependency injection

**State Management:**
- Keep state simple
- Update in OnEvent (single-threaded)
- Avoid goroutines accessing mutable state

**Order Management:**
- Track active orders
- Handle partial fills
- Cancel stale orders

**Position Tracking:**
- Use OMS for complex strategies
- Track position changes in OnEvent
- Validate position limits

**Testing:**
- Use simulated clock for determinism
- Test with various market conditions
- Verify error handling

## Next Steps

- [Actor System](../actors/actor-system.md) - Actor framework details
- [Market Makers](../actors/market-makers.md) - Advanced MM strategies
- [Simulated Time](../simulation/simulated-time.md) - Time and event scheduling
