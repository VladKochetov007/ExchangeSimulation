package lifecycle

import (
	"context"
	"exchange_sim/actor"
	"exchange_sim/exchange"
	"sync"
	"time"
)

type MarketMonitor struct {
	exchange        *exchange.Exchange
	symbol          string
	restartActors   []actor.Actor
	lastLiquidity   int64
	checkInterval   time.Duration
	restartCooldown time.Duration
	lastRestart     time.Time
	consecutiveZero int
	mu              sync.Mutex
}

func NewMarketMonitor(ex *exchange.Exchange, symbol string, checkInterval time.Duration) *MarketMonitor {
	return &MarketMonitor{
		exchange:        ex,
		symbol:          symbol,
		restartActors:   make([]actor.Actor, 0),
		checkInterval:   checkInterval,
		restartCooldown: 10 * time.Second,
	}
}

func (mm *MarketMonitor) AddRestartActor(a actor.Actor) {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	mm.restartActors = append(mm.restartActors, a)
}

func (mm *MarketMonitor) Monitor(ctx context.Context) {
	ticker := time.NewTicker(mm.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mm.checkAndRestart(ctx)
		}
	}
}

func (mm *MarketMonitor) checkAndRestart(ctx context.Context) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	book := mm.exchange.Books[mm.symbol]
	if book == nil {
		return
	}

	totalLiquidity := int64(0)
	if book.Bids.Best != nil {
		totalLiquidity += book.Bids.Best.TotalQty
	}
	if book.Asks.Best != nil {
		totalLiquidity += book.Asks.Best.TotalQty
	}

	if totalLiquidity == 0 {
		mm.consecutiveZero++
	} else {
		mm.consecutiveZero = 0
	}

	if mm.consecutiveZero >= 3 && mm.lastLiquidity > 0 {
		if time.Since(mm.lastRestart) >= mm.restartCooldown {
			for _, a := range mm.restartActors {
				a.Stop()
				time.Sleep(100 * time.Millisecond)
				a.Start(ctx)
			}
			mm.lastRestart = time.Now()
		}
	}

	mm.lastLiquidity = totalLiquidity
}
