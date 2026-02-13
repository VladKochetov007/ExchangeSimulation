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
	EMADecay         float64       // EMA decay factor for trade price (0.0-1.0, higher = faster adaptation)
}

// SlowMarketMakerActor implements a market maker that requotes on timer only (no instant refills).
type SlowMarketMakerActor struct {
	*actor.BaseActor
	config SlowMarketMakerConfig

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

	requoteTicker *time.Ticker
	stopCh        chan struct{}
}

// NewSlowMarketMaker creates a new slow market maker actor.
func NewSlowMarketMaker(id uint64, gateway *exchange.ClientGateway, config SlowMarketMakerConfig) *SlowMarketMakerActor {
	if config.RequoteInterval == 0 {
		config.RequoteInterval = 1 * time.Second
	}

	mm := &SlowMarketMakerActor{
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
func (smm *SlowMarketMakerActor) Start(ctx context.Context) error {
	smm.requoteTicker = time.NewTicker(smm.config.RequoteInterval)

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
	if accepted.RequestID == smm.lastBidReqID {
		smm.activeBidID = accepted.OrderID
	} else if accepted.RequestID == smm.lastAskReqID {
		smm.activeAskID = accepted.OrderID
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
		if fill.OrderID == smm.activeBidID {
			smm.activeBidID = 0
		} else if fill.OrderID == smm.activeAskID {
			smm.activeAskID = 0
		}
	}

	// CRITICAL: Do NOT requote immediately after fills!
	// Let the price drift until next timer tick
}

func (smm *SlowMarketMakerActor) onOrderCancelled(cancelled actor.OrderCancelledEvent) {
	if cancelled.OrderID == smm.activeBidID {
		smm.activeBidID = 0
	} else if cancelled.OrderID == smm.activeAskID {
		smm.activeAskID = 0
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
		case <-smm.requoteTicker.C:
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

	// SIMPLE: Just quote around the provided mid-price with our spread
	// No drift calculations, no complexity - let market dynamics create drift
	spreadHalf := (midPrice * smm.config.SpreadBps) / (2 * 10000)
	targetBid := midPrice - spreadHalf
	targetAsk := midPrice + spreadHalf

	// Align to tick size
	targetBid = (targetBid / tickSize) * tickSize
	targetAsk = (targetAsk / tickSize) * tickSize

	// ALWAYS cancel ALL existing orders (even if partially filled)
	// This ensures we adapt to current mid-price and don't leave stale orders
	if smm.activeBidID != 0 {
		smm.CancelOrder(smm.activeBidID)
		smm.activeBidID = 0
	}
	if smm.activeAskID != 0 {
		smm.CancelOrder(smm.activeAskID)
		smm.activeAskID = 0
	}

	// Update tracked prices
	smm.currentBid = targetBid
	smm.currentAsk = targetAsk

	if canBuy && smm.activeBidID == 0 {
		smm.lastBidReqID = smm.BaseActor.SubmitOrder(
			smm.config.Symbol,
			exchange.Buy,
			exchange.LimitOrder,
			targetBid,
			smm.config.QuoteSize,
		)
	}

	if canSell && smm.activeAskID == 0 {
		smm.lastAskReqID = smm.BaseActor.SubmitOrder(
			smm.config.Symbol,
			exchange.Sell,
			exchange.LimitOrder,
			targetAsk,
			smm.config.QuoteSize,
		)
	}
}

func (smm *SlowMarketMakerActor) cancelOrders() {
	if smm.activeBidID != 0 {
		smm.CancelOrder(smm.activeBidID)
		smm.activeBidID = 0
	}
	if smm.activeAskID != 0 {
		smm.CancelOrder(smm.activeAskID)
		smm.activeAskID = 0
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
