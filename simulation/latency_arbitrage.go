package simulation

import (
	"context"
	"sync/atomic"

	"exchange_sim/exchange"
)

type BookState struct {
	bestBid    int64
	bestAsk    int64
	bestBidQty int64
	bestAskQty int64
}

func NewBookState() *BookState {
	return &BookState{}
}

func (b *BookState) Update(md *exchange.MarketDataMsg) {
	switch md.Type {
	case exchange.MDSnapshot:
		snap := md.Data.(*exchange.BookSnapshot)
		if len(snap.Bids) > 0 {
			b.bestBid = snap.Bids[0].Price
			b.bestBidQty = snap.Bids[0].VisibleQty
		}
		if len(snap.Asks) > 0 {
			b.bestAsk = snap.Asks[0].Price
			b.bestAskQty = snap.Asks[0].VisibleQty
		}

	case exchange.MDDelta:
		delta := md.Data.(*exchange.BookDelta)
		if delta.Side == exchange.Buy {
			if delta.VisibleQty > 0 {
				b.bestBid = delta.Price
				b.bestBidQty = delta.VisibleQty
			}
		} else {
			if delta.VisibleQty > 0 {
				b.bestAsk = delta.Price
				b.bestAskQty = delta.VisibleQty
			}
		}
	}
}

func (b *BookState) BestBid() int64 {
	return b.bestBid
}

func (b *BookState) BestAsk() int64 {
	return b.bestAsk
}

func (b *BookState) BestBidQty() int64 {
	return b.bestBidQty
}

func (b *BookState) BestAskQty() int64 {
	return b.bestAskQty
}

type LatencyArbitrageConfig struct {
	FastVenue    VenueID
	SlowVenue    VenueID
	Symbol       string
	MinProfitBps int64 // Minimum profit in basis points
	MaxQty       int64 // Maximum position size per trade
}

type LatencyArbitrageActor struct {
	id       uint64
	mgw      *MultiVenueGateway
	config   LatencyArbitrageConfig
	fastBook *BookState
	slowBook *BookState
	stopCh   chan struct{}
	running  atomic.Bool

	requestSeq uint64

	// Statistics
	totalArbitrages atomic.Int64
	totalProfit     atomic.Int64
}

func NewLatencyArbitrageActor(
	id uint64,
	mgw *MultiVenueGateway,
	config LatencyArbitrageConfig,
) *LatencyArbitrageActor {
	return &LatencyArbitrageActor{
		id:       id,
		mgw:      mgw,
		config:   config,
		fastBook: NewBookState(),
		slowBook: NewBookState(),
		stopCh:   make(chan struct{}),
	}
}

func (a *LatencyArbitrageActor) ID() uint64 {
	return a.id
}

func (a *LatencyArbitrageActor) Start(ctx context.Context) error {
	if !a.running.CompareAndSwap(false, true) {
		return nil
	}

	// Subscribe to both venues
	a.mgw.Subscribe(a.config.FastVenue, a.config.Symbol)
	a.mgw.Subscribe(a.config.SlowVenue, a.config.Symbol)

	go a.run(ctx)
	return nil
}

func (a *LatencyArbitrageActor) Stop() error {
	if !a.running.Load() {
		return nil
	}
	close(a.stopCh)
	return nil
}

func (a *LatencyArbitrageActor) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			a.running.Store(false)
			return
		case <-a.stopCh:
			a.running.Store(false)
			return

		case vResp := <-a.mgw.ResponseCh():
			// Handle order confirmations
			_ = vResp

		case vData := <-a.mgw.MarketDataCh():
			// Update book state for the venue
			switch vData.Venue {
			case a.config.FastVenue:
				a.fastBook.Update(vData.Data)
			case a.config.SlowVenue:
				a.slowBook.Update(vData.Data)
			}

			// Check for arbitrage opportunities
			a.detectAndExecute()
		}
	}
}

func (a *LatencyArbitrageActor) detectAndExecute() {
	fastBid := a.fastBook.BestBid()
	slowAsk := a.slowBook.BestAsk()

	if fastBid == 0 || slowAsk == 0 {
		return // Books not initialized
	}

	// Check if we can buy on slow venue and sell on fast venue
	if fastBid > slowAsk {
		profitBps := ((fastBid - slowAsk) * 10000) / slowAsk

		if profitBps >= a.config.MinProfitBps {
			// Calculate quantity (use minimum of configured max and available liquidity)
			qty := min(a.config.MaxQty, a.slowBook.BestAskQty(), a.fastBook.BestBidQty())

			if qty > 0 {
				a.executeArbitrage(slowAsk, fastBid, qty)
			}
		}
	}

	// Check reverse: sell on fast, buy on slow
	fastAsk := a.fastBook.BestAsk()
	slowBid := a.slowBook.BestBid()

	if fastAsk == 0 || slowBid == 0 {
		return
	}

	if slowBid > fastAsk {
		profitBps := ((slowBid - fastAsk) * 10000) / fastAsk

		if profitBps >= a.config.MinProfitBps {
			qty := min(a.config.MaxQty, a.fastBook.BestAskQty(), a.slowBook.BestBidQty())

			if qty > 0 {
				a.executeReverseArbitrage(fastAsk, slowBid, qty)
			}
		}
	}
}

func (a *LatencyArbitrageActor) executeArbitrage(slowAsk, fastBid, qty int64) {
	reqID := atomic.AddUint64(&a.requestSeq, 1)

	// Buy on slow venue (stale low price)
	a.mgw.SubmitOrder(a.config.SlowVenue, &exchange.OrderRequest{
		RequestID:   reqID,
		Side:        exchange.Buy,
		Type:        exchange.LimitOrder,
		Price:       slowAsk,
		Qty:         qty,
		Symbol:      a.config.Symbol,
		TimeInForce: exchange.IOC, // Immediate or cancel
		Visibility:  exchange.Normal,
	})

	// Sell on fast venue (current high price)
	reqID = atomic.AddUint64(&a.requestSeq, 1)
	a.mgw.SubmitOrder(a.config.FastVenue, &exchange.OrderRequest{
		RequestID:   reqID,
		Side:        exchange.Sell,
		Type:        exchange.LimitOrder,
		Price:       fastBid,
		Qty:         qty,
		Symbol:      a.config.Symbol,
		TimeInForce: exchange.IOC,
		Visibility:  exchange.Normal,
	})

	// Track statistics
	a.totalArbitrages.Add(1)
	// Profit calculation: price diff (in exchange.BTC_PRECISION per BTC) * qty (in satoshis) / (exchange.BTC_PRECISION * 1000)
	// This gives profit in USD precision units (exchange.BTC_PRECISION/1000)
	// For BTC/USD with quotePrecision = exchange.BTC_PRECISION/1000:
	// priceDiff is in "USD scaled by exchange.BTC_PRECISION", qty is in satoshis
	// Result: profit in USD (scaled by exchange.BTC_PRECISION/1000)
	profit := (fastBid - slowAsk) * qty / (exchange.BTC_PRECISION * 1000)
	a.totalProfit.Add(profit)
}

func (a *LatencyArbitrageActor) executeReverseArbitrage(fastAsk, slowBid, qty int64) {
	reqID := atomic.AddUint64(&a.requestSeq, 1)

	// Buy on fast venue
	a.mgw.SubmitOrder(a.config.FastVenue, &exchange.OrderRequest{
		RequestID:   reqID,
		Side:        exchange.Buy,
		Type:        exchange.LimitOrder,
		Price:       fastAsk,
		Qty:         qty,
		Symbol:      a.config.Symbol,
		TimeInForce: exchange.IOC,
		Visibility:  exchange.Normal,
	})

	// Sell on slow venue
	reqID = atomic.AddUint64(&a.requestSeq, 1)
	a.mgw.SubmitOrder(a.config.SlowVenue, &exchange.OrderRequest{
		RequestID:   reqID,
		Side:        exchange.Sell,
		Type:        exchange.LimitOrder,
		Price:       slowBid,
		Qty:         qty,
		Symbol:      a.config.Symbol,
		TimeInForce: exchange.IOC,
		Visibility:  exchange.Normal,
	})

	// Track statistics
	a.totalArbitrages.Add(1)
	// Profit in USD precision units (exchange.BTC_PRECISION/1000) - see executeArbitrage for explanation
	profit := (slowBid - fastAsk) * qty / (exchange.BTC_PRECISION * 1000)
	a.totalProfit.Add(profit)
}

func (a *LatencyArbitrageActor) Stats() (arbitrages int64, profit int64) {
	return a.totalArbitrages.Load(), a.totalProfit.Load()
}
