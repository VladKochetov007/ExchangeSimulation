package actor

import (
	"context"
	"time"

	"exchange_sim/exchange"
)

type ExitStrategyFunc func(exposure, bestBid, bestAsk, bidLiquidity, askLiquidity, liquidityMultiple int64) bool

type FirstLPConfig struct {
	Symbol            string              // Trading pair
	SpreadBps         int64               // Spread around mid in basis points (e.g., 10 = 0.1%)
	LiquidityMultiple int64               // Exit when market has this multiple of our exposure (default: 10)
	MonitorInterval   time.Duration       // How often to check exit conditions (default: 100ms)
	MinExitSize       int64               // Don't exit if exposure below this (default: 0)
	ExitStrategy      ExitStrategyFunc    // Custom exit logic (optional, uses default if nil)
	BootstrapPrice    int64               // Initial mid price if book is empty (0 = wait for market)
}

type FirstLiquidityProvidingActor struct {
	*BaseActor
	Config FirstLPConfig

	Symbol        string
	Precision     int64
	BaseAsset     string
	QuoteAsset    string
	BaseBalance   int64
	QuoteBalance  int64
	NetPosition   int64
	AvgEntryPrice int64
	ActiveBidID   uint64
	ActiveAskID   uint64
	BidSize       int64
	AskSize       int64
	lastBidReqID  uint64
	lastAskReqID  uint64
	LastMidPrice  int64
	BestBid       int64
	BestAsk       int64
	BidLiquidity  int64
	AskLiquidity  int64
	monitorTicker *time.Ticker
}

func DefaultExitStrategy(exposure, bestBid, bestAsk, bidLiq, askLiq, multiple int64) bool {
	if exposure == 0 {
		return false
	}

	absExposure := exposure
	if absExposure < 0 {
		absExposure = -absExposure
	}

	threshold := absExposure * multiple

	if exposure > 0 {
		// Long position - need to sell - check bid liquidity
		return bidLiq >= threshold
	}
	// Short position - need to buy - check ask liquidity
	return askLiq >= threshold
}

func NewFirstLP(id uint64, gateway *exchange.ClientGateway, config FirstLPConfig) *FirstLiquidityProvidingActor {
	// Set defaults
	if config.LiquidityMultiple == 0 {
		config.LiquidityMultiple = 10
	}
	if config.MonitorInterval == 0 {
		config.MonitorInterval = 100 * time.Millisecond
	}
	if config.ExitStrategy == nil {
		config.ExitStrategy = DefaultExitStrategy
	}

	return &FirstLiquidityProvidingActor{
		BaseActor: NewBaseActor(id, gateway),
		Config:    config,
		Symbol:    config.Symbol,
	}
}

func (f *FirstLiquidityProvidingActor) Start(ctx context.Context) error {
	f.Subscribe(f.Symbol)
	f.QueryBalance()

	f.monitorTicker = time.NewTicker(f.Config.MonitorInterval)

	go f.eventLoop(ctx)
	go f.monitorLoop(ctx)

	return f.BaseActor.Start(ctx)
}

func (f *FirstLiquidityProvidingActor) Stop() error {
	if f.monitorTicker != nil {
		f.monitorTicker.Stop()
	}
	return f.BaseActor.Stop()
}

func (f *FirstLiquidityProvidingActor) eventLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-f.EventChannel():
			f.OnEvent(event)
		}
	}
}

func (f *FirstLiquidityProvidingActor) monitorLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-f.monitorTicker.C:
			f.CheckExitConditions()
		}
	}
}

func (f *FirstLiquidityProvidingActor) OnEvent(event *Event) {
	switch event.Type {
	case EventBookSnapshot:
		f.onBookSnapshot(event.Data.(BookSnapshotEvent))
	case EventBookDelta:
		f.onBookDelta(event.Data.(BookDeltaEvent))
	case EventOrderAccepted:
		f.onOrderAccepted(event.Data.(OrderAcceptedEvent))
	case EventOrderFilled, EventOrderPartialFill:
		f.onOrderFilled(event.Data.(OrderFillEvent))
	case EventOrderCancelled:
		f.onOrderCancelled(event.Data.(OrderCancelledEvent))
	}
}

func (f *FirstLiquidityProvidingActor) SetInitialState(precision int64, baseAsset, quoteAsset string) {
	f.Precision = precision
	f.BaseAsset = baseAsset
	f.QuoteAsset = quoteAsset
}

func (f *FirstLiquidityProvidingActor) UpdateBalances(baseBalance, quoteBalance int64) {
	f.BaseBalance = baseBalance
	f.QuoteBalance = quoteBalance
}

func (f *FirstLiquidityProvidingActor) onBookSnapshot(snap BookSnapshotEvent) {
	if snap.Symbol != f.Symbol {
		return
	}

	// Update market state
	if len(snap.Snapshot.Bids) > 0 {
		f.BestBid = snap.Snapshot.Bids[0].Price
		f.BidLiquidity = snap.Snapshot.Bids[0].VisibleQty
	}
	if len(snap.Snapshot.Asks) > 0 {
		f.BestAsk = snap.Snapshot.Asks[0].Price
		f.AskLiquidity = snap.Snapshot.Asks[0].VisibleQty
	}

	if f.BestBid > 0 && f.BestAsk > 0 {
		f.LastMidPrice = (f.BestBid + f.BestAsk) / 2
	} else if f.LastMidPrice == 0 && f.Config.BootstrapPrice > 0 {
		// Bootstrap: use configured price when book is empty
		f.LastMidPrice = f.Config.BootstrapPrice
	}

	// If we have balances and no active orders, place quotes
	if f.LastMidPrice > 0 && f.BaseBalance > 0 && f.QuoteBalance > 0 &&
		f.ActiveBidID == 0 && f.ActiveAskID == 0 {
		f.PlaceQuotes()
	}
}

func (f *FirstLiquidityProvidingActor) onBookDelta(delta BookDeltaEvent) {
	if delta.Delta.Side == exchange.Buy {
		if delta.Delta.Price >= f.BestBid {
			f.BestBid = delta.Delta.Price
			if delta.Delta.VisibleQty > 0 {
				f.BidLiquidity = delta.Delta.VisibleQty
			} else {
				f.BidLiquidity = 0
			}
		}
	} else {
		if f.BestAsk == 0 || delta.Delta.Price <= f.BestAsk {
			f.BestAsk = delta.Delta.Price
			if delta.Delta.VisibleQty > 0 {
				f.AskLiquidity = delta.Delta.VisibleQty
			} else {
				f.AskLiquidity = 0
			}
		}
	}
}

func (f *FirstLiquidityProvidingActor) onOrderFilled(fill OrderFillEvent) {
	// Update position and balances
	if fill.Side == exchange.Buy {
		f.UpdatePosition(fill.Qty, fill.Price)
		f.BaseBalance += fill.Qty
		notional := (fill.Qty * fill.Price) / f.Precision
		f.QuoteBalance -= (notional + fill.FeeAmount)
	} else {
		f.UpdatePosition(-fill.Qty, fill.Price)
		f.BaseBalance -= fill.Qty
		notional := (fill.Qty * fill.Price) / f.Precision
		f.QuoteBalance += (notional - fill.FeeAmount)
	}

	// Cancel unfilled orders
	f.CancelActiveOrders()

	// Requote with updated balances
	// Clear active order IDs immediately to allow requoting
	f.ActiveBidID = 0
	f.ActiveAskID = 0
	f.PlaceQuotes()

	// Exit check happens in monitor loop
}

func (f *FirstLiquidityProvidingActor) onOrderCancelled(cancelled OrderCancelledEvent) {
	// Clear tracking
	if cancelled.OrderID == f.ActiveBidID {
		f.ActiveBidID = 0
	}
	if cancelled.OrderID == f.ActiveAskID {
		f.ActiveAskID = 0
	}
}

func (f *FirstLiquidityProvidingActor) PlaceQuotes() {
	if f.LastMidPrice == 0 || f.Precision == 0 {
		return
	}

	halfSpread := (f.LastMidPrice * f.Config.SpreadBps) / (2 * 10000)
	bidPrice := f.LastMidPrice - halfSpread
	askPrice := f.LastMidPrice + halfSpread

	// Spot market: use base for sell, quote for buy
	if f.BaseBalance > 0 {
		// Track that we're placing an ask order
		// Note: In real implementation, would need to track the actual RequestID
		f.SubmitOrder(f.Symbol, exchange.Sell, exchange.LimitOrder, askPrice, f.BaseBalance)
		f.AskSize = f.BaseBalance
	}
	if f.QuoteBalance > 0 {
		bidQty := (f.QuoteBalance * f.Precision) / bidPrice
		if bidQty > 0 {
			// Track that we're placing a bid order
			f.SubmitOrder(f.Symbol, exchange.Buy, exchange.LimitOrder, bidPrice, bidQty)
			f.BidSize = bidQty
		}
	}
}

func (f *FirstLiquidityProvidingActor) onOrderAccepted(accepted OrderAcceptedEvent) {
	// Simple heuristic: if we have a bid size pending and no bid ID, this is the bid
	if f.BidSize > 0 && f.ActiveBidID == 0 {
		f.ActiveBidID = accepted.OrderID
	} else if f.AskSize > 0 && f.ActiveAskID == 0 {
		f.ActiveAskID = accepted.OrderID
	}
}

func (f *FirstLiquidityProvidingActor) CancelActiveOrders() {
	if f.ActiveBidID != 0 {
		f.CancelOrder(f.ActiveBidID)
	}
	if f.ActiveAskID != 0 {
		f.CancelOrder(f.ActiveAskID)
	}
}

func (f *FirstLiquidityProvidingActor) CheckExitConditions() {
	absExposure := f.NetPosition
	if absExposure < 0 {
		absExposure = -absExposure
	}

	if absExposure < f.Config.MinExitSize {
		return
	}

	shouldExit := f.Config.ExitStrategy(
		f.NetPosition,
		f.BestBid,
		f.BestAsk,
		f.BidLiquidity,
		f.AskLiquidity,
		f.Config.LiquidityMultiple,
	)

	if shouldExit {
		f.ExecuteExit()
	}
}

func (f *FirstLiquidityProvidingActor) ExecuteExit() {
	if f.NetPosition == 0 {
		return
	}

	f.CancelActiveOrders()

	exitSize := f.NetPosition
	if exitSize < 0 {
		exitSize = -exitSize
	}

	if f.NetPosition > 0 {
		f.SubmitOrder(f.Symbol, exchange.Sell, exchange.Market, 0, exitSize)
	} else {
		f.SubmitOrder(f.Symbol, exchange.Buy, exchange.Market, 0, exitSize)
	}
}

func (f *FirstLiquidityProvidingActor) UpdatePosition(deltaQty int64, price int64) {
	if f.NetPosition == 0 {
		// New position
		f.NetPosition = deltaQty
		f.AvgEntryPrice = price
		return
	}

	if (f.NetPosition > 0 && deltaQty > 0) || (f.NetPosition < 0 && deltaQty < 0) {
		// Increasing position - weighted average
		totalNotional := (f.NetPosition * f.AvgEntryPrice) + (deltaQty * price)
		f.NetPosition += deltaQty
		if f.NetPosition != 0 {
			f.AvgEntryPrice = totalNotional / f.NetPosition
		}
	} else {
		// Reducing or flipping position
		f.NetPosition += deltaQty
		if f.NetPosition == 0 {
			f.AvgEntryPrice = 0
		} else if (f.NetPosition > 0 && deltaQty < 0) || (f.NetPosition < 0 && deltaQty > 0) {
			// Position flipped
			f.AvgEntryPrice = price
		}
	}
}

func (f *FirstLiquidityProvidingActor) GetPosition() (netPosition, avgEntryPrice int64) {
	return f.NetPosition, f.AvgEntryPrice
}

// TEST ONLY - Expose balances for verification
func (f *FirstLiquidityProvidingActor) GetBalances() (base, quote int64) {
	return f.BaseBalance, f.QuoteBalance
}

// TEST ONLY - Set market state for testing
func (f *FirstLiquidityProvidingActor) SetMarketState(bestBid, bidLiq, bestAsk, askLiq int64) {
	f.BestBid = bestBid
	f.BidLiquidity = bidLiq
	f.BestAsk = bestAsk
	f.AskLiquidity = askLiq
}
