package actors

import (
	"context"
	"sync/atomic"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// PureMarketMakerConfig configures a pure market making strategy with fixed spreads.
type PureMarketMakerConfig struct {
	Symbol           string              // Trading symbol
	Instrument       exchange.Instrument // Trading instrument (for tick size, precision)
	SpreadBps        int64               // Fixed spread in basis points
	QuoteSize        int64               // Order size per side
	MaxInventory     int64               // Maximum absolute position limit
	RequoteThreshold int64               // Minimum mid-price change to trigger requote (bps)
	MonitorInterval  time.Duration       // How often to check for requote conditions
}

// PureMarketMakerActor implements a simple market making strategy with fixed spreads.
type PureMarketMakerActor struct {
	*actor.BaseActor
	config PureMarketMakerConfig

	instrument exchange.Instrument
	baseAsset  string
	quoteAsset string

	lastMidPrice int64
	currentBid   int64
	currentAsk   int64
	activeBidID  uint64
	activeAskID  uint64
	lastBidReqID uint64
	lastAskReqID uint64

	inventory int64

	requestSeq    uint64
	monitorTicker *time.Ticker
	stopCh        chan struct{}
}

// NewPureMarketMaker creates a new pure market maker actor.
func NewPureMarketMaker(id uint64, gateway *exchange.ClientGateway, config PureMarketMakerConfig) *PureMarketMakerActor {
	if config.MonitorInterval == 0 {
		config.MonitorInterval = 100 * time.Millisecond
	}
	if config.RequoteThreshold == 0 {
		config.RequoteThreshold = 5 // 5 bps default
	}

	mm := &PureMarketMakerActor{
		BaseActor: actor.NewBaseActor(id, gateway),
		config:    config,
		stopCh:    make(chan struct{}),
	}

	if config.Instrument != nil {
		mm.instrument = config.Instrument
		mm.baseAsset = config.Instrument.BaseAsset()
		mm.quoteAsset = config.Instrument.QuoteAsset()
	}

	return mm
}

// Start starts the actor.
func (pmm *PureMarketMakerActor) Start(ctx context.Context) error {
	pmm.monitorTicker = time.NewTicker(pmm.config.MonitorInterval)

	go pmm.eventLoop(ctx)
	go pmm.monitorLoop(ctx)

	if err := pmm.BaseActor.Start(ctx); err != nil {
		return err
	}

	pmm.Subscribe(pmm.config.Symbol)
	pmm.QueryBalance()

	return nil
}

// Stop stops the actor.
func (pmm *PureMarketMakerActor) Stop() error {
	if pmm.monitorTicker != nil {
		pmm.monitorTicker.Stop()
	}
	close(pmm.stopCh)
	return pmm.BaseActor.Stop()
}

// OnEvent handles incoming events.
func (pmm *PureMarketMakerActor) OnEvent(event *actor.Event) {
	switch event.Type {
	case actor.EventBookSnapshot:
		pmm.onBookSnapshot(event.Data.(actor.BookSnapshotEvent))
	case actor.EventOrderAccepted:
		pmm.onOrderAccepted(event.Data.(actor.OrderAcceptedEvent))
	case actor.EventOrderFilled:
		pmm.onOrderFilled(event.Data.(actor.OrderFillEvent))
	case actor.EventOrderPartialFill:
		pmm.onOrderFilled(event.Data.(actor.OrderFillEvent))
	case actor.EventOrderCancelled:
		pmm.onOrderCancelled(event.Data.(actor.OrderCancelledEvent))
	}
}

func (pmm *PureMarketMakerActor) onBookSnapshot(snap actor.BookSnapshotEvent) {
	if snap.Symbol != pmm.config.Symbol {
		return
	}

	if pmm.instrument == nil {
		// Instrument should be set in config, but handle gracefully
		return
	}

	if len(snap.Snapshot.Bids) == 0 || len(snap.Snapshot.Asks) == 0 {
		return
	}

	bestBid := snap.Snapshot.Bids[0].Price
	bestAsk := snap.Snapshot.Asks[0].Price
	midPrice := (bestBid + bestAsk) / 2

	if pmm.lastMidPrice == 0 {
		pmm.lastMidPrice = midPrice
		pmm.requote(midPrice)
		return
	}

	// Check if mid-price changed significantly
	bpsChange := BPSChange(pmm.lastMidPrice, midPrice)
	if bpsChange < 0 {
		bpsChange = -bpsChange
	}

	if bpsChange >= pmm.config.RequoteThreshold {
		pmm.lastMidPrice = midPrice
		pmm.requote(midPrice)
	}
}

func (pmm *PureMarketMakerActor) onOrderAccepted(accepted actor.OrderAcceptedEvent) {
	if accepted.RequestID == pmm.lastBidReqID {
		pmm.activeBidID = accepted.OrderID
	} else if accepted.RequestID == pmm.lastAskReqID {
		pmm.activeAskID = accepted.OrderID
	}
}

func (pmm *PureMarketMakerActor) onOrderFilled(fill actor.OrderFillEvent) {
	// Update inventory
	if fill.Side == exchange.Buy {
		pmm.inventory += int64(fill.Qty)
	} else {
		pmm.inventory -= int64(fill.Qty)
	}

	// Clear active order IDs on full fill
	if fill.IsFull {
		if fill.OrderID == pmm.activeBidID {
			pmm.activeBidID = 0
		} else if fill.OrderID == pmm.activeAskID {
			pmm.activeAskID = 0
		}
	}

	// Replenish liquidity after fill using stored mid-price
	if pmm.lastMidPrice > 0 {
		pmm.requote(pmm.lastMidPrice)
	}
}

func (pmm *PureMarketMakerActor) onOrderCancelled(cancelled actor.OrderCancelledEvent) {
	if cancelled.OrderID == pmm.activeBidID {
		pmm.activeBidID = 0
	} else if cancelled.OrderID == pmm.activeAskID {
		pmm.activeAskID = 0
	}
}

func (pmm *PureMarketMakerActor) requote(midPrice int64) {
	if pmm.instrument == nil {
		return
	}

	// Check inventory limits
	canBuy := pmm.inventory < pmm.config.MaxInventory
	canSell := pmm.inventory > -pmm.config.MaxInventory

	if !canBuy && !canSell {
		// Stop quoting if inventory limit reached in both directions
		pmm.cancelOrders()
		return
	}

	tickSize := pmm.instrument.TickSize()

	// Calculate bid/ask prices around mid
	spreadHalf := (midPrice * pmm.config.SpreadBps) / (2 * 10000)
	bidPrice := midPrice - spreadHalf
	askPrice := midPrice + spreadHalf

	// Align to tick size
	bidPrice = (bidPrice / tickSize) * tickSize
	askPrice = (askPrice / tickSize) * tickSize

	// Check if we need to requote
	bidFine := !canBuy || (canBuy && bidPrice == pmm.currentBid && pmm.activeBidID != 0)
	askFine := !canSell || (canSell && askPrice == pmm.currentAsk && pmm.activeAskID != 0)

	if bidFine && askFine {
		return
	}

	// Cancel existing orders
	pmm.cancelOrders()

	// Place new orders
	pmm.currentBid = bidPrice
	pmm.currentAsk = askPrice

	if canBuy {
		pmm.lastBidReqID = atomic.AddUint64(&pmm.requestSeq, 1)
		pmm.BaseActor.SubmitOrder(
			pmm.config.Symbol,
			exchange.Buy,
			exchange.LimitOrder,
			bidPrice,
			pmm.config.QuoteSize,
		)
	}

	if canSell {
		pmm.lastAskReqID = atomic.AddUint64(&pmm.requestSeq, 1)
		pmm.BaseActor.SubmitOrder(
			pmm.config.Symbol,
			exchange.Sell,
			exchange.LimitOrder,
			askPrice,
			pmm.config.QuoteSize,
		)
	}
}

func (pmm *PureMarketMakerActor) cancelOrders() {
	if pmm.activeBidID != 0 {
		pmm.CancelOrder(pmm.activeBidID)
		pmm.activeBidID = 0
	}
	if pmm.activeAskID != 0 {
		pmm.CancelOrder(pmm.activeAskID)
		pmm.activeAskID = 0
	}
}

func (pmm *PureMarketMakerActor) eventLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-pmm.stopCh:
			return
		case event := <-pmm.BaseActor.EventChannel():
			pmm.OnEvent(event)
		}
	}
}

func (pmm *PureMarketMakerActor) monitorLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-pmm.stopCh:
			return
		case <-pmm.monitorTicker.C:
			// Periodic health check - could add logic here
		}
	}
}
