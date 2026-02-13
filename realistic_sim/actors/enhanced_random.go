package actors

import (
	"context"
	"math/rand"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// EnhancedRandomConfig configures a random trader that places both market and limit orders.
type EnhancedRandomConfig struct {
	Symbol             string               // Trading symbol
	Instrument         exchange.Instrument  // Trading instrument
	MinQty             int64                // Minimum order quantity
	MaxQty             int64                // Maximum order quantity
	TradeInterval      time.Duration        // Time between trades
	LimitOrderPct      int64                // 0-100, percentage of orders as limit
	LimitPriceRangeBps int64                // Max distance from mid for limit orders (bps)
}

// EnhancedRandomActor implements a random trading strategy with mixed order types.
type EnhancedRandomActor struct {
	*actor.BaseActor
	config EnhancedRandomConfig

	instrument exchange.Instrument
	baseAsset  string
	quoteAsset string

	lastMidPrice int64
	rng          *rand.Rand

	tradeTicker exchange.Ticker
	stopCh      chan struct{}
}

// NewEnhancedRandom creates a new enhanced random trader actor.
func NewEnhancedRandom(id uint64, gateway *exchange.ClientGateway, config EnhancedRandomConfig, seed int64) *EnhancedRandomActor {
	if config.TradeInterval == 0 {
		config.TradeInterval = 2 * time.Second
	}
	if config.LimitOrderPct == 0 {
		config.LimitOrderPct = 50 // 50% limit orders by default
	}
	if config.LimitPriceRangeBps == 0 {
		config.LimitPriceRangeBps = 50 // 0.5% range
	}

	er := &EnhancedRandomActor{
		BaseActor: actor.NewBaseActor(id, gateway),
		config:    config,
		rng:       rand.New(rand.NewSource(seed)),
		stopCh:    make(chan struct{}),
	}

	if config.Instrument != nil {
		er.instrument = config.Instrument
		er.baseAsset = config.Instrument.BaseAsset()
		er.quoteAsset = config.Instrument.QuoteAsset()
	}

	return er
}

// Start starts the actor.
func (er *EnhancedRandomActor) Start(ctx context.Context) error {
	er.tradeTicker = er.GetTickerFactory().NewTicker(er.config.TradeInterval)

	go er.eventLoop(ctx)
	go er.tradingLoop(ctx)

	if err := er.BaseActor.Start(ctx); err != nil {
		return err
	}

	er.Subscribe(er.config.Symbol)
	er.QueryBalance()

	return nil
}

// Stop stops the actor.
func (er *EnhancedRandomActor) Stop() error {
	if er.tradeTicker != nil {
		er.tradeTicker.Stop()
	}
	close(er.stopCh)
	return er.BaseActor.Stop()
}

// OnEvent handles incoming events.
func (er *EnhancedRandomActor) OnEvent(event *actor.Event) {
	switch event.Type {
	case actor.EventBookSnapshot:
		er.onBookSnapshot(event.Data.(actor.BookSnapshotEvent))
	}
}

func (er *EnhancedRandomActor) onBookSnapshot(snap actor.BookSnapshotEvent) {
	if snap.Symbol != er.config.Symbol {
		return
	}

	if er.instrument == nil {
		return
	}

	if len(snap.Snapshot.Bids) == 0 || len(snap.Snapshot.Asks) == 0 {
		return
	}

	bestBid := snap.Snapshot.Bids[0].Price
	bestAsk := snap.Snapshot.Asks[0].Price
	er.lastMidPrice = bestBid + (bestAsk-bestBid)/2
}

func (er *EnhancedRandomActor) tradingLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-er.stopCh:
			return
		case <-er.tradeTicker.C():
			er.executeTrade()
		}
	}
}

func (er *EnhancedRandomActor) executeTrade() {
	if er.instrument == nil || er.lastMidPrice == 0 {
		return
	}

	// Random side
	side := exchange.Buy
	if er.rng.Intn(2) == 1 {
		side = exchange.Sell
	}

	// Random quantity between MinQty and MaxQty
	qtyRange := er.config.MaxQty - er.config.MinQty
	qty := er.config.MinQty
	if qtyRange > 0 {
		qty = er.config.MinQty + er.rng.Int63n(qtyRange)
	}

	// Decide order type: limit or market
	isLimit := er.rng.Intn(100) < int(er.config.LimitOrderPct)

	if isLimit {
		er.placeLimitOrder(side, qty)
	} else {
		er.placeMarketOrder(side, qty)
	}
}

func (er *EnhancedRandomActor) placeMarketOrder(side exchange.Side, qty int64) {
	er.BaseActor.SubmitOrder(
		er.config.Symbol,
		side,
		exchange.Market,
		0, // Market orders don't need price
		qty,
	)
}

func (er *EnhancedRandomActor) placeLimitOrder(side exchange.Side, qty int64) {
	tickSize := er.instrument.TickSize()

	// Random price within BPS range of mid
	maxOffset := (er.lastMidPrice * er.config.LimitPriceRangeBps) / 10000
	if maxOffset == 0 {
		maxOffset = tickSize
	}

	// Random offset within range
	offset := int64(0)
	if maxOffset > tickSize {
		offset = er.rng.Int63n(maxOffset/tickSize) * tickSize
	}

	var price int64
	if side == exchange.Buy {
		// Buy limit: below mid
		price = er.lastMidPrice - offset
	} else {
		// Sell limit: above mid
		price = er.lastMidPrice + offset
	}

	// Align to tick size
	price = (price / tickSize) * tickSize

	// Ensure positive price
	if price <= 0 {
		price = tickSize
	}

	er.BaseActor.SubmitOrder(
		er.config.Symbol,
		side,
		exchange.LimitOrder,
		price,
		qty,
	)
}

func (er *EnhancedRandomActor) eventLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-er.stopCh:
			return
		case event := <-er.BaseActor.EventChannel():
			er.OnEvent(event)
		}
	}
}
