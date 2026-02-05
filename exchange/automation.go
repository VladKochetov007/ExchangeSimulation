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
type ExchangeAutomation struct {
	exchange            *Exchange
	markPriceCalc       MarkPriceCalculator
	indexProvider       IndexPriceProvider
	priceUpdateInterval time.Duration

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
}

// AutomationConfig configures the automatic exchange operations
type AutomationConfig struct {
	// MarkPriceCalc calculates mark price from order book (required)
	MarkPriceCalc MarkPriceCalculator

	// IndexProvider provides index prices for perpetuals (required)
	IndexProvider IndexPriceProvider

	// PriceUpdateInterval is how often to update funding rates (default: 3s)
	PriceUpdateInterval time.Duration
}

// NewExchangeAutomation creates a new automation manager
func NewExchangeAutomation(exchange *Exchange, config AutomationConfig) *ExchangeAutomation {
	if config.MarkPriceCalc == nil {
		config.MarkPriceCalc = NewMidPriceCalculator()
	}

	if config.PriceUpdateInterval == 0 {
		config.PriceUpdateInterval = 3 * time.Second
	}

	return &ExchangeAutomation{
		exchange:            exchange,
		markPriceCalc:       config.MarkPriceCalc,
		indexProvider:       config.IndexProvider,
		priceUpdateInterval: config.PriceUpdateInterval,
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

	// Start price update loop
	a.wg.Add(1)
	go a.priceUpdateLoop()

	// Start funding settlement loop
	a.wg.Add(1)
	go a.fundingSettlementLoop()
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

	ticker := time.NewTicker(a.priceUpdateInterval)
	defer ticker.Stop()

	// Immediate first update
	a.updateAllPerpPrices()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.updateAllPerpPrices()
		}
	}
}

// fundingSettlementLoop continuously checks and settles funding for all perpetuals
func (a *ExchangeAutomation) fundingSettlementLoop() {
	defer a.wg.Done()

	// Check every second for funding settlements
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.checkAndSettleFunding()
		}
	}
}

// updateAllPerpPrices updates funding rates for all perpetual instruments
func (a *ExchangeAutomation) updateAllPerpPrices() {
	a.exchange.mu.RLock()
	books := make([]*OrderBook, 0, len(a.exchange.Books))
	for _, book := range a.exchange.Books {
		if book.Instrument.IsPerp() {
			books = append(books, book)
		}
	}
	a.exchange.mu.RUnlock()

	timestamp := a.exchange.Clock.NowUnixNano()

	for _, book := range books {
		perp := book.Instrument.(*PerpFutures)

		// Calculate mark price from order book
		markPrice := a.markPriceCalc.Calculate(book)
		if markPrice == 0 {
			continue // No valid mark price yet
		}

		// Get index price from provider
		indexPrice := a.indexProvider.GetIndexPrice(book.Symbol, timestamp)
		if indexPrice == 0 {
			continue // No valid index price yet
		}

		// Update funding rate
		perp.UpdateFundingRate(indexPrice, markPrice)

		// Publish funding update event
		a.exchange.MDPublisher.PublishFunding(book.Symbol, perp.GetFundingRate(), timestamp)
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
			// Settle funding
			a.exchange.Positions.SettleFunding(clients, perp)

			// Publish funding event (NextFunding updated by SettleFunding)
			a.exchange.MDPublisher.PublishFunding(perp.Symbol(), perp.GetFundingRate(), now)
		}
	}
}
