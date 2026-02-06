package actors

import (
	"context"
	"sync/atomic"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/realistic_sim/position"
	"exchange_sim/realistic_sim/signals"
)

type CrossSectionalMRConfig struct {
	Symbols            []string
	LookbackPeriod     time.Duration
	AllocatedCapital   int64
	RebalanceInterval  time.Duration
	MaxPositionSize    int64
	MinSignalThreshold int64
}

type CrossSectionalMRActor struct {
	*actor.BaseActor
	config          CrossSectionalMRConfig
	instruments     map[string]exchange.Instrument
	signals         *signals.CrossSectionalSignals
	positionMgr     *position.PositionManager
	riskFilter      *position.CompositeFilter
	lastMidPrices   map[string]int64
	clock           exchange.Clock
	rebalanceTicker *time.Ticker
	requestSeq      uint64
}

func NewCrossSectionalMR(
	id uint64,
	gateway *exchange.ClientGateway,
	config CrossSectionalMRConfig,
	clock exchange.Clock,
	oms *actor.NettingOMS,
	policy position.SizingPolicy,
	filters ...position.RiskFilter,
) *CrossSectionalMRActor {
	if config.RebalanceInterval == 0 {
		config.RebalanceInterval = 10 * time.Second
	}
	if config.LookbackPeriod == 0 {
		config.LookbackPeriod = 30 * time.Second
	}

	csSignals := signals.NewCrossSectionalSignals(config.LookbackPeriod, 10000)
	for _, symbol := range config.Symbols {
		csSignals.AddSymbol(symbol, config.LookbackPeriod, 10000)
	}

	return &CrossSectionalMRActor{
		BaseActor:     actor.NewBaseActor(id, gateway),
		config:        config,
		instruments:   make(map[string]exchange.Instrument),
		signals:       csSignals,
		positionMgr:   position.NewPositionManager(oms, policy, config.AllocatedCapital),
		riskFilter:    position.NewCompositeFilter(filters...),
		lastMidPrices: make(map[string]int64),
		clock:         clock,
	}
}

func (csmr *CrossSectionalMRActor) Start(ctx context.Context) error {
	csmr.rebalanceTicker = time.NewTicker(csmr.config.RebalanceInterval)

	go csmr.eventLoop(ctx)
	go csmr.rebalanceLoop(ctx)

	if err := csmr.BaseActor.Start(ctx); err != nil {
		return err
	}

	for _, symbol := range csmr.config.Symbols {
		csmr.Subscribe(symbol)
	}

	return nil
}

func (csmr *CrossSectionalMRActor) Stop() error {
	if csmr.rebalanceTicker != nil {
		csmr.rebalanceTicker.Stop()
	}
	return csmr.BaseActor.Stop()
}

func (csmr *CrossSectionalMRActor) eventLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-csmr.EventChannel():
			csmr.OnEvent(event)
		}
	}
}

func (csmr *CrossSectionalMRActor) rebalanceLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-csmr.rebalanceTicker.C:
			csmr.rebalance()
		}
	}
}

func (csmr *CrossSectionalMRActor) OnEvent(event *actor.Event) {
	switch event.Type {
	case actor.EventBookSnapshot:
		csmr.onBookSnapshot(event.Data.(actor.BookSnapshotEvent))
	case actor.EventTrade:
		csmr.onTrade(event.Data.(actor.TradeEvent))
	}
}

func (csmr *CrossSectionalMRActor) onBookSnapshot(snap actor.BookSnapshotEvent) {
	if len(snap.Snapshot.Bids) > 0 && len(snap.Snapshot.Asks) > 0 {
		bestBid := snap.Snapshot.Bids[0].Price
		bestAsk := snap.Snapshot.Asks[0].Price
		csmr.lastMidPrices[snap.Symbol] = (bestBid + bestAsk) / 2
	}
}

func (csmr *CrossSectionalMRActor) onTrade(trade actor.TradeEvent) {
	csmr.lastMidPrices[trade.Symbol] = trade.Trade.Price
	timestamp := csmr.clock.NowUnixNano()
	csmr.signals.AddPrice(trade.Symbol, trade.Trade.Price, timestamp)
}

func (csmr *CrossSectionalMRActor) rebalance() {
	currentTime := csmr.clock.NowUnixNano()
	signalMap := csmr.signals.Calculate(csmr.config.Symbols, currentTime)
	if len(signalMap) == 0 {
		return
	}

	totalSignal := int64(0)
	for _, signal := range signalMap {
		absSignal := signal
		if absSignal < 0 {
			absSignal = -absSignal
		}
		totalSignal += absSignal
	}

	if totalSignal == 0 {
		return
	}

	for symbol, signal := range signalMap {
		absSignal := signal
		if absSignal < 0 {
			absSignal = -absSignal
		}
		if absSignal < csmr.config.MinSignalThreshold {
			continue
		}

		midPrice := csmr.lastMidPrices[symbol]
		if midPrice == 0 {
			continue
		}

		currentPosition := csmr.positionMgr.GetPosition(symbol)
		targetPosition := csmr.positionMgr.TargetPosition(signal, totalSignal, midPrice)

		if targetPosition > csmr.config.MaxPositionSize {
			targetPosition = csmr.config.MaxPositionSize
		} else if targetPosition < -csmr.config.MaxPositionSize {
			targetPosition = -csmr.config.MaxPositionSize
		}

		orderQty := targetPosition - currentPosition
		if orderQty == 0 {
			continue
		}

		csmr.submitRebalanceOrder(symbol, orderQty)
	}
}

func (csmr *CrossSectionalMRActor) submitRebalanceOrder(symbol string, orderQty int64) {
	var side exchange.Side
	var qty int64

	if orderQty > 0 {
		side = exchange.Buy
		qty = orderQty
	} else {
		side = exchange.Sell
		qty = -orderQty
	}

	atomic.AddUint64(&csmr.requestSeq, 1)
	csmr.SubmitOrder(symbol, side, exchange.Market, 0, qty)
}

func (csmr *CrossSectionalMRActor) SetInstrument(symbol string, instrument exchange.Instrument) {
	csmr.instruments[symbol] = instrument
}
