package actors

import (
	"context"
	"math/rand"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

type RandomizedTakerConfig struct {
	Symbol         string
	Interval       time.Duration
	MinQty         int64
	MaxQty         int64
	BasePrecision  int64
	QuotePrecision int64
}

type RandomizedTakerActor struct {
	*actor.BaseActor
	Config RandomizedTakerConfig
	ticker exchange.Ticker
	rng    *rand.Rand
	side   exchange.Side
}

func NewRandomizedTaker(id uint64, gateway *exchange.ClientGateway, config RandomizedTakerConfig) *RandomizedTakerActor {
	if config.Interval == 0 {
		config.Interval = 2 * time.Second
	}
	if config.MinQty == 0 {
		config.MinQty = exchange.BTCAmount(0.1)
	}
	if config.MaxQty == 0 {
		config.MaxQty = exchange.BTCAmount(1.0)
	}
	if config.BasePrecision == 0 {
		config.BasePrecision = exchange.SATOSHI
	}
	if config.QuotePrecision == 0 {
		config.QuotePrecision = exchange.SATOSHI / 1000
	}

	// Use better seed to avoid correlation between actors
	// Multiply ID by large prime to ensure seed diversity
	seed := time.Now().UnixNano() + (int64(id) * 104729)
	rng := rand.New(rand.NewSource(seed))

	// Randomize initial side (don't hardcode to Buy!)
	var initialSide exchange.Side
	if rng.Intn(2) == 0 {
		initialSide = exchange.Buy
	} else {
		initialSide = exchange.Sell
	}

	return &RandomizedTakerActor{
		BaseActor: actor.NewBaseActor(id, gateway),
		Config:    config,
		rng:       rng,
		side:      initialSide,
	}
}

func (s *RandomizedTakerActor) Start(ctx context.Context) error {
	s.Subscribe(s.Config.Symbol)
	s.ticker = s.GetTickerFactory().NewTicker(s.Config.Interval)

	go s.eventLoop(ctx)
	go s.tradingLoop(ctx)

	return s.BaseActor.Start(ctx)
}

func (s *RandomizedTakerActor) Stop() error {
	if s.ticker != nil {
		s.ticker.Stop()
	}
	return s.BaseActor.Stop()
}

func (s *RandomizedTakerActor) eventLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-s.EventChannel():
			s.OnEvent(event)
		}
	}
}

func (s *RandomizedTakerActor) tradingLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.ticker.C():
			s.executeRandomTrade()
			s.side = s.randomSide()
		}
	}
}

func (s *RandomizedTakerActor) OnEvent(event *actor.Event) {
	// RandomizedTaker doesn't need to react to events
}

func (s *RandomizedTakerActor) executeRandomTrade() {
	qtyRange := s.Config.MaxQty - s.Config.MinQty
	if qtyRange <= 0 {
		qtyRange = s.Config.MinQty
	}

	qty := s.Config.MinQty
	if qtyRange > 0 {
		qty += s.rng.Int63n(qtyRange)
	}

	s.SubmitOrder(s.Config.Symbol, s.side, exchange.Market, 0, qty)
}

func (s *RandomizedTakerActor) randomSide() exchange.Side {
	if s.rng.Intn(2) == 0 {
		return exchange.Buy
	}
	return exchange.Sell
}
