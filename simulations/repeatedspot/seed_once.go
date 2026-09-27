package repeatedspot

import (
	"context"
	"fmt"
	"sync/atomic"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// SeedOnceConfig specifies finite opening liquidity. It does not include a
// reference-price feed or a replenishment rule.
type SeedOnceConfig struct {
	Symbol   string `json:"symbol"`
	BidPrice int64  `json:"bid_price"`
	AskPrice int64  `json:"ask_price"`
	BidQty   int64  `json:"bid_qty"`
	AskQty   int64  `json:"ask_qty"`
}

// SeedOnce sends one post-only order per configured side at actor start.
// Fills, rejections and cancellations never trigger another submission.
type SeedOnce struct {
	*actor.BaseActor
	cfg     SeedOnceConfig
	started atomic.Bool
}

func NewSeedOnce(id uint64, gateway actor.Gateway, cfg SeedOnceConfig) (*SeedOnce, error) {
	if gateway == nil || cfg.Symbol == "" || cfg.BidPrice <= 0 || cfg.AskPrice <= cfg.BidPrice || cfg.BidQty <= 0 || cfg.AskQty <= 0 {
		return nil, fmt.Errorf("repeatedspot: invalid seed-once orders")
	}
	seed := &SeedOnce{BaseActor: actor.NewBaseActor(id, gateway), cfg: cfg}
	seed.SetHandler(seed)
	return seed, nil
}

func (seed *SeedOnce) Start(ctx context.Context) error {
	if !seed.started.CompareAndSwap(false, true) {
		return fmt.Errorf("repeatedspot: seed-once actor already started")
	}
	if err := seed.BaseActor.Start(ctx); err != nil {
		return err
	}
	seed.SubmitPostOnlyOrder(seed.cfg.Symbol, exchange.Buy, seed.cfg.BidPrice, seed.cfg.BidQty)
	seed.SubmitPostOnlyOrder(seed.cfg.Symbol, exchange.Sell, seed.cfg.AskPrice, seed.cfg.AskQty)
	return nil
}

func (seed *SeedOnce) HandleEvent(context.Context, *actor.Event) {}
