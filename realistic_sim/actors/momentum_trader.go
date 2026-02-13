package actors

import (
	"context"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// MomentumTraderConfig configures a momentum trading strategy using SMA crossovers.
type MomentumTraderConfig struct {
	Symbol          string               // Trading symbol
	Instrument      exchange.Instrument  // Trading instrument
	FastWindow      int                  // Fast SMA window size
	SlowWindow      int                  // Slow SMA window size
	PositionSize    int64                // Trade size (in base asset precision)
	MonitorInterval time.Duration        // How often to check for signals
}

// MomentumTraderActor implements a trend following strategy using SMA crossovers.
// - Long signal: Fast SMA crosses above Slow SMA
// - Short signal: Fast SMA crosses below Slow SMA
// - Exit: Reverse crossover
type MomentumTraderActor struct {
	*actor.BaseActor
	config MomentumTraderConfig

	instrument exchange.Instrument
	baseAsset  string
	quoteAsset string

	// Price tracking
	fastBuffer *CircularBuffer // Fast SMA
	slowBuffer *CircularBuffer // Slow SMA

	// Signal tracking
	lastSignal int64 // +1 = long, -1 = short, 0 = flat
	position   int64 // Current position

	// State
	lastMidPrice int64

	monitorTicker exchange.Ticker
	stopCh        chan struct{}
}

// NewMomentumTrader creates a new momentum trading actor.
func NewMomentumTrader(id uint64, gateway *exchange.ClientGateway, config MomentumTraderConfig) *MomentumTraderActor {
	if config.MonitorInterval == 0 {
		config.MonitorInterval = 1 * time.Second
	}
	if config.FastWindow == 0 {
		config.FastWindow = 10
	}
	if config.SlowWindow == 0 {
		config.SlowWindow = 50
	}

	mt := &MomentumTraderActor{
		BaseActor:  actor.NewBaseActor(id, gateway),
		config:     config,
		fastBuffer: NewCircularBuffer(config.FastWindow),
		slowBuffer: NewCircularBuffer(config.SlowWindow),
		stopCh:     make(chan struct{}),
	}

	if config.Instrument != nil {
		mt.instrument = config.Instrument
		mt.baseAsset = config.Instrument.BaseAsset()
		mt.quoteAsset = config.Instrument.QuoteAsset()
	}

	return mt
}

// Start starts the actor.
func (mt *MomentumTraderActor) Start(ctx context.Context) error {
	mt.monitorTicker = mt.GetTickerFactory().NewTicker(mt.config.MonitorInterval)

	go mt.eventLoop(ctx)
	go mt.monitorLoop(ctx)

	if err := mt.BaseActor.Start(ctx); err != nil {
		return err
	}

	mt.Subscribe(mt.config.Symbol)
	mt.QueryBalance()

	return nil
}

// Stop stops the actor.
func (mt *MomentumTraderActor) Stop() error {
	if mt.monitorTicker != nil {
		mt.monitorTicker.Stop()
	}
	close(mt.stopCh)
	return mt.BaseActor.Stop()
}

// OnEvent handles incoming events.
func (mt *MomentumTraderActor) OnEvent(event *actor.Event) {
	switch event.Type {
	case actor.EventTrade:
		mt.onTrade(event.Data.(actor.TradeEvent))
	case actor.EventOrderFilled:
		mt.onOrderFilled(event.Data.(actor.OrderFillEvent))
	case actor.EventOrderPartialFill:
		mt.onOrderFilled(event.Data.(actor.OrderFillEvent))
	case actor.EventBookSnapshot:
		mt.onBookSnapshot(event.Data.(actor.BookSnapshotEvent))
	}
}

func (mt *MomentumTraderActor) onTrade(trade actor.TradeEvent) {
	if trade.Symbol != mt.config.Symbol {
		return
	}

	if trade.Trade == nil {
		return
	}

	// Add trade price to both buffers
	mt.fastBuffer.Add(trade.Trade.Price)
	mt.slowBuffer.Add(trade.Trade.Price)
}

func (mt *MomentumTraderActor) onBookSnapshot(snap actor.BookSnapshotEvent) {
	if snap.Symbol != mt.config.Symbol {
		return
	}

	if mt.instrument == nil {
		return
	}

	if len(snap.Snapshot.Bids) == 0 || len(snap.Snapshot.Asks) == 0 {
		return
	}

	bestBid := snap.Snapshot.Bids[0].Price
	bestAsk := snap.Snapshot.Asks[0].Price
	mt.lastMidPrice = bestBid + (bestAsk-bestBid)/2
}

func (mt *MomentumTraderActor) onOrderFilled(fill actor.OrderFillEvent) {
	// Update position tracking
	if fill.Side == exchange.Buy {
		mt.position += fill.Qty
	} else {
		mt.position -= fill.Qty
	}
}

func (mt *MomentumTraderActor) monitorLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-mt.stopCh:
			return
		case <-mt.monitorTicker.C():
			mt.checkSignals()
		}
	}
}

func (mt *MomentumTraderActor) checkSignals() {
	// Need both buffers to be full before generating signals
	if !mt.fastBuffer.IsFull() || !mt.slowBuffer.IsFull() {
		return
	}

	if mt.instrument == nil {
		return
	}

	// Calculate SMAs
	fastSMA := mt.fastBuffer.SMA()
	slowSMA := mt.slowBuffer.SMA()

	if fastSMA == 0 || slowSMA == 0 {
		return
	}

	// Determine current signal
	var newSignal int64
	if fastSMA > slowSMA {
		newSignal = 1 // Long signal
	} else if fastSMA < slowSMA {
		newSignal = -1 // Short signal
	} else {
		newSignal = 0 // Neutral
	}

	// Check for crossover
	if newSignal != mt.lastSignal {
		mt.handleSignalChange(newSignal)
		mt.lastSignal = newSignal
	}
}

func (mt *MomentumTraderActor) handleSignalChange(newSignal int64) {
	// Close existing position if we have one
	if mt.position != 0 {
		mt.closePosition()
	}

	// Open new position based on signal
	if newSignal == 1 {
		// Long signal - buy
		mt.openLongPosition()
	} else if newSignal == -1 {
		// Short signal - sell
		mt.openShortPosition()
	}
	// If newSignal == 0, stay flat (already closed position above)
}

func (mt *MomentumTraderActor) openLongPosition() {
	if mt.config.PositionSize == 0 {
		return
	}

	mt.BaseActor.SubmitOrder(
		mt.config.Symbol,
		exchange.Buy,
		exchange.Market,
		0,
		mt.config.PositionSize,
	)
}

func (mt *MomentumTraderActor) openShortPosition() {
	if mt.config.PositionSize == 0 {
		return
	}

	mt.BaseActor.SubmitOrder(
		mt.config.Symbol,
		exchange.Sell,
		exchange.Market,
		0,
		mt.config.PositionSize,
	)
}

func (mt *MomentumTraderActor) closePosition() {
	if mt.position == 0 {
		return
	}

	var side exchange.Side
	var qty int64

	if mt.position > 0 {
		// Long position - sell to close
		side = exchange.Sell
		qty = mt.position
	} else {
		// Short position - buy to close
		side = exchange.Buy
		qty = -mt.position
	}

	mt.BaseActor.SubmitOrder(
		mt.config.Symbol,
		side,
		exchange.Market,
		0,
		qty,
	)
}

func (mt *MomentumTraderActor) eventLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-mt.stopCh:
			return
		case event := <-mt.BaseActor.EventChannel():
			mt.OnEvent(event)
		}
	}
}
