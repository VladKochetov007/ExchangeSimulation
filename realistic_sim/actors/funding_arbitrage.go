package actors

import (
	"context"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// FundingArbConfig configures a funding arbitrage strategy (long spot + short perp).
type FundingArbConfig struct {
	SpotSymbol       string               // Spot symbol (e.g., "BTC/USD")
	PerpSymbol       string               // Perp symbol (e.g., "BTC-PERP")
	SpotInstrument   exchange.Instrument  // Spot instrument
	PerpInstrument   exchange.Instrument  // Perp instrument
	MinFundingRate   int64                // Enter when funding rate > this (bps)
	ExitFundingRate  int64                // Exit when funding rate < this (bps)
	HedgeRatio       int64                // Target hedge ratio (10000 = 1:1)
	MaxPositionSize  int64                // Maximum position per side (in base asset precision)
	MonitorInterval  time.Duration        // How often to check conditions
	RebalanceThreshold int64              // Rebalance when ratio drifts > this (bps)
}

// FundingArbActor implements a funding arbitrage strategy.
// Strategy: Long spot + short perp to collect positive funding payments.
type FundingArbActor struct {
	*actor.BaseActor
	config FundingArbConfig

	spotInstrument exchange.Instrument
	perpInstrument exchange.Instrument
	baseAsset      string
	quoteAsset     string

	// Positions tracked from fills
	spotPosition int64 // Long position in spot
	perpPosition int64 // Short position in perp (will be negative)

	// Market state
	spotMid       int64
	perpMid       int64
	fundingRate   int64 // Current funding rate in bps
	nextFunding   int64 // Timestamp of next funding settlement

	// Strategy state
	isActive bool // Whether we have an active hedge position

	monitorTicker *time.Ticker
	stopCh        chan struct{}
}

// NewFundingArbitrage creates a new funding arbitrage actor.
func NewFundingArbitrage(id uint64, gateway *exchange.ClientGateway, config FundingArbConfig) *FundingArbActor {
	if config.MonitorInterval == 0 {
		config.MonitorInterval = 5 * time.Second
	}
	if config.HedgeRatio == 0 {
		config.HedgeRatio = 10000 // 1:1 default
	}
	if config.RebalanceThreshold == 0 {
		config.RebalanceThreshold = 100 // 1% drift
	}

	fa := &FundingArbActor{
		BaseActor: actor.NewBaseActor(id, gateway),
		config:    config,
		stopCh:    make(chan struct{}),
	}

	if config.SpotInstrument != nil {
		fa.spotInstrument = config.SpotInstrument
		fa.baseAsset = config.SpotInstrument.BaseAsset()
		fa.quoteAsset = config.SpotInstrument.QuoteAsset()
	}
	if config.PerpInstrument != nil {
		fa.perpInstrument = config.PerpInstrument
	}

	return fa
}

// Start starts the actor.
func (fa *FundingArbActor) Start(ctx context.Context) error {
	fa.monitorTicker = time.NewTicker(fa.config.MonitorInterval)

	go fa.eventLoop(ctx)
	go fa.monitorLoop(ctx)

	if err := fa.BaseActor.Start(ctx); err != nil {
		return err
	}

	// Subscribe to both spot and perp markets
	fa.Subscribe(fa.config.SpotSymbol)
	fa.Subscribe(fa.config.PerpSymbol)
	fa.QueryBalance()

	return nil
}

// Stop stops the actor.
func (fa *FundingArbActor) Stop() error {
	if fa.monitorTicker != nil {
		fa.monitorTicker.Stop()
	}
	close(fa.stopCh)
	return fa.BaseActor.Stop()
}

// OnEvent handles incoming events.
func (fa *FundingArbActor) OnEvent(event *actor.Event) {
	switch event.Type {
	case actor.EventBookSnapshot:
		fa.onBookSnapshot(event.Data.(actor.BookSnapshotEvent))
	case actor.EventOrderFilled:
		fa.onOrderFilled(event.Data.(actor.OrderFillEvent))
	case actor.EventOrderPartialFill:
		fa.onOrderFilled(event.Data.(actor.OrderFillEvent))
	case actor.EventFundingUpdate:
		fa.onFundingUpdate(event.Data.(actor.FundingUpdateEvent))
	}
}

func (fa *FundingArbActor) onBookSnapshot(snap actor.BookSnapshotEvent) {
	if fa.spotInstrument == nil || fa.perpInstrument == nil {
		return
	}

	if len(snap.Snapshot.Bids) == 0 || len(snap.Snapshot.Asks) == 0 {
		return
	}

	bestBid := snap.Snapshot.Bids[0].Price
	bestAsk := snap.Snapshot.Asks[0].Price
	mid := (bestBid + bestAsk) / 2

	if snap.Symbol == fa.config.SpotSymbol {
		fa.spotMid = mid
	} else if snap.Symbol == fa.config.PerpSymbol {
		fa.perpMid = mid
	}
}

func (fa *FundingArbActor) onOrderFilled(fill actor.OrderFillEvent) {
	// Update positions based on fills
	// We need to track which symbol the fill came from, but OrderFillEvent doesn't include symbol
	// For simplicity, we'll track positions via separate logic
	// In a real implementation, we'd use OMS or track order IDs -> symbols

	// This is a simplified version - in production, use NettingOMS
	// For now, we'll update positions in the monitoring loop based on strategy state
}

func (fa *FundingArbActor) onFundingUpdate(update actor.FundingUpdateEvent) {
	if update.Symbol != fa.config.PerpSymbol {
		return
	}

	fa.fundingRate = update.FundingRate.Rate
	fa.nextFunding = update.FundingRate.NextFunding
}

func (fa *FundingArbActor) monitorLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-fa.stopCh:
			return
		case <-fa.monitorTicker.C:
			fa.evaluateStrategy()
		}
	}
}

func (fa *FundingArbActor) evaluateStrategy() {
	if fa.spotInstrument == nil || fa.perpInstrument == nil {
		return
	}
	if fa.spotMid == 0 || fa.perpMid == 0 {
		return
	}

	// Check if we should enter a position
	if !fa.isActive {
		if fa.fundingRate >= fa.config.MinFundingRate {
			fa.enterPosition()
		}
	} else {
		// Check if we should exit
		if fa.fundingRate < fa.config.ExitFundingRate {
			fa.exitPosition()
		} else {
			// Check if we need to rebalance
			fa.checkRebalance()
		}
	}
}

func (fa *FundingArbActor) enterPosition() {
	// Enter funding arbitrage position:
	// - Buy spot (long)
	// - Sell perp (short)

	positionSize := fa.config.MaxPositionSize
	if positionSize == 0 {
		return
	}

	// Buy spot at market
	fa.BaseActor.SubmitOrder(
		fa.config.SpotSymbol,
		exchange.Buy,
		exchange.Market,
		0, // Market order
		positionSize,
	)

	// Sell perp at market
	fa.BaseActor.SubmitOrder(
		fa.config.PerpSymbol,
		exchange.Sell,
		exchange.Market,
		0, // Market order
		positionSize,
	)

	fa.isActive = true
	fa.spotPosition = positionSize
	fa.perpPosition = -positionSize // Short position
}

func (fa *FundingArbActor) exitPosition() {
	// Exit funding arbitrage position:
	// - Sell spot
	// - Buy perp to close short

	if fa.spotPosition > 0 {
		// Sell spot at market
		fa.BaseActor.SubmitOrder(
			fa.config.SpotSymbol,
			exchange.Sell,
			exchange.Market,
			0,
			fa.spotPosition,
		)
	}

	if fa.perpPosition < 0 {
		// Buy perp to close short
		fa.BaseActor.SubmitOrder(
			fa.config.PerpSymbol,
			exchange.Buy,
			exchange.Market,
			0,
			-fa.perpPosition, // Convert negative to positive quantity
		)
	}

	fa.isActive = false
	fa.spotPosition = 0
	fa.perpPosition = 0
}

func (fa *FundingArbActor) checkRebalance() {
	// Check if hedge ratio has drifted
	if fa.spotPosition == 0 && fa.perpPosition == 0 {
		return
	}

	// Calculate current ratio
	// target ratio = config.HedgeRatio (e.g., 10000 for 1:1)
	// current ratio = (spotPosition * 10000) / abs(perpPosition)

	absPerp := fa.perpPosition
	if absPerp < 0 {
		absPerp = -absPerp
	}

	if absPerp == 0 {
		// Need to rebalance - perp position is zero but spot isn't
		fa.exitPosition()
		return
	}

	currentRatio := (fa.spotPosition * 10000) / absPerp
	ratioDiff := currentRatio - fa.config.HedgeRatio
	if ratioDiff < 0 {
		ratioDiff = -ratioDiff
	}

	// If ratio drifted more than threshold, rebalance
	if ratioDiff > fa.config.RebalanceThreshold {
		// For simplicity, exit and re-enter
		// In production, calculate exact rebalance quantities
		fa.exitPosition()
		time.Sleep(100 * time.Millisecond) // Brief delay
		fa.enterPosition()
	}
}

func (fa *FundingArbActor) eventLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-fa.stopCh:
			return
		case event := <-fa.BaseActor.EventChannel():
			fa.OnEvent(event)
		}
	}
}
