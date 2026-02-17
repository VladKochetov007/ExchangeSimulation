package exchange

import (
	"context"
	"sync"
	"time"
)

// ExchangeAutomation provides industry-standard automatic exchange operations
// - Automatic mark/index price calculation
// - Automatic funding rate updates
// - Automatic funding settlement on schedule
// - Liquidation checks after price updates
// - Collateral interest charging on borrowed amounts
type ExchangeAutomation struct {
	exchange            *Exchange
	markPriceCalc       MarkPriceCalculator
	indexProvider       IndexPriceProvider
	priceUpdateInterval time.Duration
	collateralRate      int64 // annual rate in bps (e.g. 500 = 5%)
	liquidationHandler  LiquidationHandler
	tickerFactory       TickerFactory

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
}

// LiquidationHandler is called when a liquidation event occurs.
type LiquidationHandler interface {
	OnMarginCall(event *MarginCallEvent)
	OnLiquidation(event *LiquidationEvent)
	OnInsuranceFund(event *InsuranceFundEvent)
}

// AutomationConfig configures the automatic exchange operations
type AutomationConfig struct {
	// MarkPriceCalc calculates mark price from order book (required)
	MarkPriceCalc MarkPriceCalculator

	// IndexProvider provides index prices for perpetuals (required)
	IndexProvider IndexPriceProvider

	// PriceUpdateInterval is how often to update funding rates (default: 3s)
	PriceUpdateInterval time.Duration

	// CollateralRate is annual interest rate on borrowed amounts in bps (default: 500 = 5%)
	CollateralRate int64

	// LiquidationHandler receives liquidation events (optional)
	LiquidationHandler LiquidationHandler

	// TickerFactory creates tickers for periodic operations (default: RealTickerFactory)
	TickerFactory TickerFactory
}

// NewExchangeAutomation creates a new automation manager
func NewExchangeAutomation(exchange *Exchange, config AutomationConfig) *ExchangeAutomation {
	if config.MarkPriceCalc == nil {
		config.MarkPriceCalc = NewMidPriceCalculator()
	}
	if config.PriceUpdateInterval == 0 {
		config.PriceUpdateInterval = 3 * time.Second
	}
	if config.CollateralRate == 0 {
		config.CollateralRate = 500 // 5% APR default
	}
	if config.TickerFactory == nil {
		config.TickerFactory = &RealTickerFactory{}
	}

	return &ExchangeAutomation{
		exchange:            exchange,
		markPriceCalc:       config.MarkPriceCalc,
		indexProvider:       config.IndexProvider,
		priceUpdateInterval: config.PriceUpdateInterval,
		collateralRate:      config.CollateralRate,
		liquidationHandler:  config.LiquidationHandler,
		tickerFactory:       config.TickerFactory,
	}
}

// Start begins automatic operations
// Runs until context is cancelled or Stop() is called
func (a *ExchangeAutomation) Start(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.ctx != nil {
		return // Already running
	}

	a.ctx, a.cancel = context.WithCancel(ctx)

	a.wg.Add(1)
	go a.priceUpdateLoop()

	a.wg.Add(1)
	go a.fundingSettlementLoop()

	a.wg.Add(1)
	go a.collateralChargeLoop()
}

// Stop stops all automatic operations and waits for completion
func (a *ExchangeAutomation) Stop() {
	a.mu.Lock()
	if a.cancel != nil {
		a.cancel()
	}
	a.mu.Unlock()

	a.wg.Wait()

	a.mu.Lock()
	a.ctx = nil
	a.cancel = nil
	a.mu.Unlock()
}

// priceUpdateLoop continuously updates funding rates for all perpetuals
func (a *ExchangeAutomation) priceUpdateLoop() {
	defer a.wg.Done()

	ticker := a.tickerFactory.NewTicker(a.priceUpdateInterval)
	defer ticker.Stop()

	// Immediate first update
	a.updateAllPerpPrices()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C():
			a.updateAllPerpPrices()
		}
	}
}

// fundingSettlementLoop continuously checks and settles funding for all perpetuals
func (a *ExchangeAutomation) fundingSettlementLoop() {
	defer a.wg.Done()

	// Check every second for funding settlements
	ticker := a.tickerFactory.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C():
			a.checkAndSettleFunding()
		}
	}
}

// updateAllPerpPrices updates funding rates for all perpetual instruments
func (a *ExchangeAutomation) updateAllPerpPrices() {
	timestamp := a.exchange.Clock.NowUnixNano()

	a.exchange.mu.RLock()
	type perpUpdate struct {
		symbol     string
		perp       *PerpFutures
		markPrice  int64
		indexPrice int64
	}
	updates := make([]perpUpdate, 0, len(a.exchange.Books))
	for _, book := range a.exchange.Books {
		if !book.Instrument.IsPerp() {
			continue
		}
		markPrice := a.markPriceCalc.Calculate(book)
		if markPrice == 0 {
			continue
		}
		indexPrice := a.indexProvider.GetIndexPrice(book.Symbol, timestamp)
		if indexPrice == 0 {
			continue
		}
		updates = append(updates, perpUpdate{
			symbol:     book.Symbol,
			perp:       book.Instrument.(*PerpFutures),
			markPrice:  markPrice,
			indexPrice: indexPrice,
		})
	}
	a.exchange.mu.RUnlock()

	for _, u := range updates {
		u.perp.UpdateFundingRate(u.indexPrice, u.markPrice)
		a.exchange.MDPublisher.PublishFunding(u.symbol, u.perp.GetFundingRate(), timestamp)

		if log := a.exchange.getLogger(u.symbol); log != nil {
			log.LogEvent(timestamp, 0, "mark_price_update", MarkPriceUpdateEvent{
				Timestamp:  timestamp,
				Symbol:     u.symbol,
				MarkPrice:  u.markPrice,
				IndexPrice: u.indexPrice,
			})

			// Log funding rate update (real exchanges log this with mark prices)
			fundingRate := u.perp.GetFundingRate()
			log.LogEvent(timestamp, 0, "funding_rate_update", FundingRateUpdateEvent{
				Timestamp:   timestamp,
				Symbol:      u.symbol,
				Rate:        fundingRate.Rate,
				NextFunding: fundingRate.NextFunding,
			})
		}

		a.checkLiquidations(u.symbol, u.perp, u.markPrice)
	}
}

// collateralChargeLoop charges interest on borrowed amounts periodically (every hour of sim time).
func (a *ExchangeAutomation) collateralChargeLoop() {
	defer a.wg.Done()

	ticker := a.tickerFactory.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C():
			a.chargeCollateralInterest()
		}
	}
}

func (a *ExchangeAutomation) chargeCollateralInterest() {
	a.exchange.mu.Lock()
	defer a.exchange.mu.Unlock()

	const dtSeconds = 60
	const secondsPerYear = 365 * 24 * 3600
	timestamp := a.exchange.Clock.NowUnixNano()

	for _, client := range a.exchange.Clients {
		for asset, borrowed := range client.Borrowed {
			if borrowed <= 0 {
				continue
			}
			interest := borrowed * a.collateralRate * dtSeconds / (int64(secondsPerYear) * 10000)
			if interest > 0 {
				oldBalance := client.PerpBalances[asset]
				client.PerpBalances[asset] -= interest
				a.exchange.ExchangeBalance.FeeRevenue[asset] += interest

				a.exchange.balanceTracker.LogBalanceChange(timestamp, client.ID, "", "interest_charge", []BalanceDelta{
					perpDelta(asset, oldBalance, client.PerpBalances[asset]),
				})

				if log := a.exchange.getLogger("_global"); log != nil {
					log.LogEvent(timestamp, client.ID, "margin_interest", MarginInterestEvent{
						Timestamp: timestamp,
						ClientID:  client.ID,
						Asset:     asset,
						Amount:    interest,
					})
				}
			}
		}
	}
}

// checkLiquidations evaluates all positions for a symbol after a mark price update.
func (a *ExchangeAutomation) checkLiquidations(symbol string, perp *PerpFutures, markPrice int64) {
	if markPrice == 0 {
		return
	}
	precision := perp.BasePrecision() // Fixed: use BasePrecision not TickSize

	a.exchange.mu.Lock()
	defer a.exchange.mu.Unlock()

	for clientID, client := range a.exchange.Clients {
		pos := a.exchange.Positions.GetPosition(clientID, symbol)
		if pos == nil || pos.Size == 0 {
			continue
		}

		// Unrealized PnL
		sign := int64(1)
		if pos.Size < 0 {
			sign = -1
		}
		unrealizedPnL := abs(pos.Size) * sign * (markPrice - pos.EntryPrice) / precision

		// Initial margin posted = abs(size) * entryPrice * marginRate / (precision * 10000)
		initMargin := abs(pos.Size) * pos.EntryPrice * perp.MarginRate / (precision * 10000)
		if initMargin == 0 {
			continue
		}

		equity := client.PerpAvailable(perp.QuoteAsset()) + unrealizedPnL
		marginRatio := equity * 10000 / initMargin // in bps

		timestamp := a.exchange.Clock.NowUnixNano()

		// Debug logging before liquidation
		if marginRatio < perp.MaintenanceMarginRate {
			if log := a.exchange.getLogger("_global"); log != nil {
				log.LogEvent(timestamp, clientID, "liquidation_check", map[string]interface{}{
					"timestamp":       timestamp,
					"client_id":       clientID,
					"symbol":          symbol,
					"position_size":   pos.Size,
					"entry_price":     pos.EntryPrice,
					"mark_price":      markPrice,
					"balance":         client.PerpBalances[perp.QuoteAsset()],
					"reserved":        client.PerpReserved[perp.QuoteAsset()],
					"available":       client.PerpAvailable(perp.QuoteAsset()),
					"unrealized_pnl":  unrealizedPnL,
					"equity":          equity,
					"init_margin":     initMargin,
					"margin_ratio":    marginRatio,
					"threshold":       perp.MaintenanceMarginRate,
				})
			}
			a.liquidate(clientID, client, symbol, pos, perp, markPrice, timestamp)
		} else if marginRatio < perp.WarningMarginRate && a.liquidationHandler != nil {
			liqPrice := a.estimateLiquidationPrice(pos, client, perp, precision)
			a.liquidationHandler.OnMarginCall(&MarginCallEvent{
				Timestamp:        timestamp,
				ClientID:         clientID,
				Symbol:           symbol,
				MarginRatioBps:   marginRatio,
				LiquidationPrice: liqPrice,
			})
		}
	}
}

// estimateLiquidationPrice returns the price at which the position would be liquidated.
func (a *ExchangeAutomation) estimateLiquidationPrice(pos *Position, client *Client, perp *PerpFutures, precision int64) int64 {
	// margin runs out when equity = 0
	// equity = available + size * (liqPrice - entry) / precision = 0
	// liqPrice = entry - available * precision / size  (for long)
	// liqPrice = entry + available * precision / size  (for short)
	available := client.PerpAvailable(perp.QuoteAsset())
	if pos.Size == 0 {
		return 0
	}
	if pos.Size > 0 {
		return pos.EntryPrice - available*precision/pos.Size
	}
	return pos.EntryPrice + available*precision/(-pos.Size)
}

// liquidate forcibly closes a position via market order when maintenance margin is breached.
// Caller must hold exchange.mu.
func (a *ExchangeAutomation) liquidate(clientID uint64, client *Client, symbol string, pos *Position, perp *PerpFutures, markPrice, timestamp int64) {
	book := a.exchange.Books[symbol]
	if book == nil {
		return
	}

	// Force close: opposite side market order
	closeSide := Sell
	if pos.Size < 0 {
		closeSide = Buy
	}
	closeQty := abs(pos.Size)

	// Cancel all existing orders for this client on this symbol first
	for _, orderID := range append([]uint64{}, client.OrderIDs...) {
		var order *Order
		if o := book.Bids.Orders[orderID]; o != nil {
			order = o
		} else if o := book.Asks.Orders[orderID]; o != nil {
			order = o
		}
		if order == nil || order.ClientID != clientID {
			continue
		}
		remainingQty := order.Qty - order.FilledQty
		remainingMargin := (remainingQty * order.Price / perp.BasePrecision()) * perp.MarginRate / 10000
		client.ReleasePerp(perp.QuoteAsset(), remainingMargin)
		if order.Side == Buy {
			book.Bids.cancelOrder(orderID)
		} else {
			book.Asks.cancelOrder(orderID)
		}
		client.RemoveOrder(orderID)
	}

	// Place forced market order (execute against best available)
	orderID := a.exchange.NextOrderID
	a.exchange.NextOrderID++
	order := getOrder()
	order.ID = orderID
	order.ClientID = clientID
	order.Side = closeSide
	order.Type = Market
	order.Qty = closeQty
	order.Status = Open
	order.Timestamp = timestamp

	result := a.exchange.Matcher.Match(book.Bids, book.Asks, order)

	fillPrice := int64(0)
	if len(result.Executions) > 0 {
		fillPrice = result.Executions[len(result.Executions)-1].Price
	}

	// processExecutions settles both sides: margins, PnL, positions, fees.
	a.exchange.processExecutions(book, result.Executions, order)

	// Repay borrowed amounts from liquidation proceeds
	if a.exchange.BorrowingMgr != nil {
		borrowed := client.Borrowed[perp.QuoteAsset()]
		if borrowed > 0 {
			availableForRepay := client.PerpAvailable(perp.QuoteAsset())
			if availableForRepay > 0 {
				repayAmount := borrowed
				if repayAmount > availableForRepay {
					repayAmount = availableForRepay
				}

				oldBorrowed := client.Borrowed[perp.QuoteAsset()]
				oldPerp := client.PerpBalances[perp.QuoteAsset()]
				client.Borrowed[perp.QuoteAsset()] -= repayAmount
				client.PerpBalances[perp.QuoteAsset()] -= repayAmount

				a.exchange.balanceTracker.LogBalanceChange(timestamp, clientID, symbol, "liquidation_repay", []BalanceDelta{
					perpDelta(perp.QuoteAsset(), oldPerp, client.PerpBalances[perp.QuoteAsset()]),
					borrowedDelta(perp.QuoteAsset(), oldBorrowed, client.Borrowed[perp.QuoteAsset()]),
				})

				if log := a.exchange.getLogger("_global"); log != nil {
					log.LogEvent(timestamp, clientID, "repay", RepayEvent{
						Timestamp:     timestamp,
						ClientID:      clientID,
						Asset:         perp.QuoteAsset(),
						Principal:     repayAmount,
						Interest:      0,
						RemainingDebt: client.Borrowed[perp.QuoteAsset()],
					})
				}
			}
		}
	}

	// Check for deficit/surplus
	remainingEquity := client.PerpAvailable(perp.QuoteAsset())
	debt := int64(0)
	if remainingEquity < 0 {
		// Deficit: insurance fund covers the loss
		debt = -remainingEquity
		oldBalance := client.PerpBalances[perp.QuoteAsset()]
		oldReserved := client.PerpReserved[perp.QuoteAsset()]
		client.PerpBalances[perp.QuoteAsset()] = 0
		client.PerpReserved[perp.QuoteAsset()] = 0
		a.exchange.ExchangeBalance.InsuranceFund[perp.QuoteAsset()] -= debt

		a.exchange.balanceTracker.LogBalanceChange(timestamp, clientID, symbol, "liquidation_deficit", []BalanceDelta{
			perpDelta(perp.QuoteAsset(), oldBalance, 0),
			reservedPerpDelta(perp.QuoteAsset(), oldReserved, 0),
		})
	} else if remainingEquity > 0 {
		// Surplus: return to client by releasing reserved margin
		// The balance stays with the client, we just release the reserved portion
		oldReserved := client.PerpReserved[perp.QuoteAsset()]
		client.PerpReserved[perp.QuoteAsset()] = 0

		a.exchange.balanceTracker.LogBalanceChange(timestamp, clientID, symbol, "liquidation_surplus", []BalanceDelta{
			reservedPerpDelta(perp.QuoteAsset(), oldReserved, 0),
		})
	}

	putOrder(order)

	if a.liquidationHandler != nil {
		a.liquidationHandler.OnLiquidation(&LiquidationEvent{
			Timestamp:     timestamp,
			ClientID:      clientID,
			Symbol:        symbol,
			PositionSize:  pos.Size,
			FillPrice:     fillPrice,
			RemainingDebt: debt,
		})
		if debt > 0 || remainingEquity > 0 {
			a.liquidationHandler.OnInsuranceFund(&InsuranceFundEvent{
				Timestamp: timestamp,
				Symbol:    symbol,
				Delta:     remainingEquity - debt,
				Balance:   a.exchange.ExchangeBalance.InsuranceFund[perp.QuoteAsset()],
			})
		}
	}
}

// checkAndSettleFunding checks if any perpetuals need funding settlement
func (a *ExchangeAutomation) checkAndSettleFunding() {
	a.exchange.mu.RLock()
	perps := make([]*PerpFutures, 0, len(a.exchange.Instruments))
	for _, inst := range a.exchange.Instruments {
		if inst.IsPerp() {
			perps = append(perps, inst.(*PerpFutures))
		}
	}
	clients := a.exchange.Clients
	a.exchange.mu.RUnlock()

	now := a.exchange.Clock.NowUnixNano()

	for _, perp := range perps {
		fundingRate := perp.GetFundingRate()

		// Check if it's time for settlement
		if now >= fundingRate.NextFunding {
			a.exchange.mu.Lock()
			a.exchange.Positions.SettleFunding(clients, perp, a.exchange)
			a.exchange.mu.Unlock()

			a.exchange.MDPublisher.PublishFunding(perp.Symbol(), perp.GetFundingRate(), now)
		}
	}
}
