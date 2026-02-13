package actors

import (
	"context"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// SlowMarketMakerConfig configures a market maker that does NOT immediately requote after fills.
// This allows price to drift between requote intervals, enabling random walk behavior.
type SlowMarketMakerConfig struct {
	Symbol           string
	Instrument       exchange.Instrument
	SpreadBps        int64
	QuoteSize        int64
	MaxInventory     int64
	RequoteInterval  time.Duration // Only requotes on this timer, NOT after fills
	BootstrapPrice   int64
	EMADecay         float64 // EMA decay factor for trade price (0.0-1.0, higher = faster adaptation)
	Levels           int     // Number of price levels to quote (default: 1)
	LevelSpacingBps  int64   // Spacing between levels in bps (default: SpreadBps)
}

// SlowMarketMakerActor implements a market maker that requotes on timer only (no instant refills).
type SlowMarketMakerActor struct {
	*actor.BaseActor
	config SlowMarketMakerConfig

	instrument exchange.Instrument
	baseAsset  string
	quoteAsset string

	lastMidPrice int64

	// Multi-level order tracking
	activeBidIDs  []uint64 // Order IDs for each bid level
	activeAskIDs  []uint64 // Order IDs for each ask level
	lastBidReqIDs []uint64 // Request IDs for bid orders
	lastAskReqIDs []uint64 // Request IDs for ask orders

	inventory int64

	requoteTicker exchange.Ticker
	stopCh        chan struct{}
}

// NewSlowMarketMaker creates a new slow market maker actor.
func NewSlowMarketMaker(id uint64, gateway *exchange.ClientGateway, config SlowMarketMakerConfig) *SlowMarketMakerActor {
	if config.RequoteInterval == 0 {
		config.RequoteInterval = 1 * time.Second
	}
	if config.Levels <= 0 {
		config.Levels = 1 // Default: single level
	}
	if config.LevelSpacingBps <= 0 {
		config.LevelSpacingBps = config.SpreadBps // Default: same as spread
	}

	mm := &SlowMarketMakerActor{
		BaseActor:     actor.NewBaseActor(id, gateway),
		config:        config,
		stopCh:        make(chan struct{}),
		activeBidIDs:  make([]uint64, config.Levels),
		activeAskIDs:  make([]uint64, config.Levels),
		lastBidReqIDs: make([]uint64, config.Levels),
		lastAskReqIDs: make([]uint64, config.Levels),
	}

	if config.Instrument != nil {
		mm.instrument = config.Instrument
		mm.baseAsset = config.Instrument.BaseAsset()
		mm.quoteAsset = config.Instrument.QuoteAsset()
	}

	return mm
}

// Start starts the actor.
func (smm *SlowMarketMakerActor) Start(ctx context.Context) error {
	smm.requoteTicker = smm.GetTickerFactory().NewTicker(smm.config.RequoteInterval)

	go smm.eventLoop(ctx)
	go smm.requoteLoop(ctx)

	if err := smm.BaseActor.Start(ctx); err != nil {
		return err
	}

	smm.Subscribe(smm.config.Symbol)
	smm.QueryBalance()

	return nil
}

// Stop stops the actor.
func (smm *SlowMarketMakerActor) Stop() error {
	if smm.requoteTicker != nil {
		smm.requoteTicker.Stop()
	}
	close(smm.stopCh)
	return smm.BaseActor.Stop()
}

// OnEvent handles incoming events.
func (smm *SlowMarketMakerActor) OnEvent(event *actor.Event) {
	switch event.Type {
	case actor.EventBookSnapshot:
		smm.onBookSnapshot(event.Data.(actor.BookSnapshotEvent))
	case actor.EventBookDelta:
		smm.onBookDelta(event.Data.(actor.BookDeltaEvent))
	case actor.EventTrade:
		smm.onTrade(event.Data.(actor.TradeEvent))
	case actor.EventOrderAccepted:
		smm.onOrderAccepted(event.Data.(actor.OrderAcceptedEvent))
	case actor.EventOrderFilled:
		smm.onOrderFilled(event.Data.(actor.OrderFillEvent))
	case actor.EventOrderCancelled:
		smm.onOrderCancelled(event.Data.(actor.OrderCancelledEvent))
	}
}

func (smm *SlowMarketMakerActor) onTrade(trade actor.TradeEvent) {
	if trade.Symbol != smm.config.Symbol {
		return
	}

	// Use EMA of trade prices instead of raw last trade
	// This gives each maker a different view of fair value based on their decay rate
	// Prevents all makers from converging to identical prices (which creates oscillation)
	tradePrice := float64(trade.Trade.Price)

	if smm.lastMidPrice == 0 {
		// Bootstrap with first trade
		smm.lastMidPrice = trade.Trade.Price
	} else {
		// EMA update: ema_new = alpha * trade + (1-alpha) * ema_old
		alpha := smm.config.EMADecay
		if alpha <= 0 || alpha > 1 {
			alpha = 0.1 // Default 10% decay if not configured
		}
		currentEMA := float64(smm.lastMidPrice)
		newEMA := alpha*tradePrice + (1-alpha)*currentEMA
		smm.lastMidPrice = int64(newEMA)
	}
}

func (smm *SlowMarketMakerActor) onBookDelta(delta actor.BookDeltaEvent) {
	// Periodic snapshots now stream automatically every 100ms simulation time
	// No need to manually request snapshots - they arrive via the stream
}

func (smm *SlowMarketMakerActor) onBookSnapshot(snap actor.BookSnapshotEvent) {
	if snap.Symbol != smm.config.Symbol {
		return
	}

	if smm.instrument == nil {
		return
	}

	// FALLBACK: Only use BBO for initial bootstrapping (before first trade)
	// After trades start, onTrade() handles price discovery
	// This prevents circular dependency where makers quote around their own quotes
	if smm.lastMidPrice == 0 {
		var midPrice int64
		if len(snap.Snapshot.Bids) > 0 && len(snap.Snapshot.Asks) > 0 {
			bestBid := snap.Snapshot.Bids[0].Price
			bestAsk := snap.Snapshot.Asks[0].Price
			midPrice = (bestBid + bestAsk) / 2
		} else if smm.config.BootstrapPrice > 0 {
			midPrice = smm.config.BootstrapPrice
		} else {
			return
		}

		smm.lastMidPrice = midPrice
	}
}

func (smm *SlowMarketMakerActor) onOrderAccepted(accepted actor.OrderAcceptedEvent) {
	// Match request ID to the appropriate level
	for i := range smm.lastBidReqIDs {
		if accepted.RequestID == smm.lastBidReqIDs[i] {
			smm.activeBidIDs[i] = accepted.OrderID
			return
		}
	}
	for i := range smm.lastAskReqIDs {
		if accepted.RequestID == smm.lastAskReqIDs[i] {
			smm.activeAskIDs[i] = accepted.OrderID
			return
		}
	}
}

func (smm *SlowMarketMakerActor) onOrderFilled(fill actor.OrderFillEvent) {
	// Update inventory
	if fill.Side == exchange.Buy {
		smm.inventory += int64(fill.Qty)
	} else {
		smm.inventory -= int64(fill.Qty)
	}

	// Clear active order IDs on full fill
	if fill.IsFull {
		for i := range smm.activeBidIDs {
			if fill.OrderID == smm.activeBidIDs[i] {
				smm.activeBidIDs[i] = 0
				return
			}
		}
		for i := range smm.activeAskIDs {
			if fill.OrderID == smm.activeAskIDs[i] {
				smm.activeAskIDs[i] = 0
				return
			}
		}
	}

	// CRITICAL: Do NOT requote immediately after fills!
	// Let the price drift until next timer tick
}

func (smm *SlowMarketMakerActor) onOrderCancelled(cancelled actor.OrderCancelledEvent) {
	for i := range smm.activeBidIDs {
		if cancelled.OrderID == smm.activeBidIDs[i] {
			smm.activeBidIDs[i] = 0
			return
		}
	}
	for i := range smm.activeAskIDs {
		if cancelled.OrderID == smm.activeAskIDs[i] {
			smm.activeAskIDs[i] = 0
			return
		}
	}
}

func (smm *SlowMarketMakerActor) requoteLoop(ctx context.Context) {
	// Place initial quotes
	time.Sleep(100 * time.Millisecond) // Wait for book snapshot
	if smm.lastMidPrice == 0 {
		smm.lastMidPrice = smm.config.BootstrapPrice
	}
	smm.requote(smm.lastMidPrice)

	for {
		select {
		case <-ctx.Done():
			return
		case <-smm.stopCh:
			return
		case <-smm.requoteTicker.C():
			// ALWAYS requote on timer (not just when orders missing)
			// This allows adaptation even with partial fills
			if smm.lastMidPrice > 0 {
				smm.requote(smm.lastMidPrice)
			}
		}
	}
}

func (smm *SlowMarketMakerActor) requote(midPrice int64) {
	if smm.instrument == nil {
		return
	}

	// Check inventory limits
	canBuy := smm.inventory < smm.config.MaxInventory
	canSell := smm.inventory > -smm.config.MaxInventory

	if !canBuy && !canSell {
		smm.cancelOrders()
		return
	}

	tickSize := smm.instrument.TickSize()

	// ALWAYS cancel ALL existing orders at all levels
	smm.cancelOrders()

	// Calculate base spread
	spreadHalf := (midPrice * smm.config.SpreadBps) / (2 * 10000)
	levelSpacing := (midPrice * smm.config.LevelSpacingBps) / 10000

	// Place orders at each level
	for level := 0; level < smm.config.Levels; level++ {
		// Calculate price for this level (farther from mid with each level)
		levelOffset := int64(level) * levelSpacing
		targetBid := midPrice - spreadHalf - levelOffset
		targetAsk := midPrice + spreadHalf + levelOffset

		// Align to tick size
		targetBid = (targetBid / tickSize) * tickSize
		targetAsk = (targetAsk / tickSize) * tickSize

		// Place bid at this level
		if canBuy {
			smm.lastBidReqIDs[level] = smm.BaseActor.SubmitOrder(
				smm.config.Symbol,
				exchange.Buy,
				exchange.LimitOrder,
				targetBid,
				smm.config.QuoteSize,
			)
		}

		// Place ask at this level
		if canSell {
			smm.lastAskReqIDs[level] = smm.BaseActor.SubmitOrder(
				smm.config.Symbol,
				exchange.Sell,
				exchange.LimitOrder,
				targetAsk,
				smm.config.QuoteSize,
			)
		}
	}
}

func (smm *SlowMarketMakerActor) cancelOrders() {
	// Cancel all bid levels
	for i := range smm.activeBidIDs {
		if smm.activeBidIDs[i] != 0 {
			smm.CancelOrder(smm.activeBidIDs[i])
			smm.activeBidIDs[i] = 0
		}
	}
	// Cancel all ask levels
	for i := range smm.activeAskIDs {
		if smm.activeAskIDs[i] != 0 {
			smm.CancelOrder(smm.activeAskIDs[i])
			smm.activeAskIDs[i] = 0
		}
	}
}

func (smm *SlowMarketMakerActor) eventLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-smm.stopCh:
			return
		case event := <-smm.BaseActor.EventChannel():
			smm.OnEvent(event)
		}
	}
}
