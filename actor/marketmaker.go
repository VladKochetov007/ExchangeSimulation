package actor

import (
	"context"

	"exchange_sim/exchange"
)

type MarketMakerConfig struct {
	Symbol        string
	SpreadBps     int64
	QuoteQty      int64
	RefreshOnFill bool
}

type MarketMakerActor struct {
	*BaseActor
	config       MarketMakerConfig
	midPrice     int64
	bidOrderID   uint64
	askOrderID   uint64
	hasSnapshot  bool
}

func NewMarketMaker(id uint64, gateway *exchange.ClientGateway, config MarketMakerConfig) *MarketMakerActor {
	return &MarketMakerActor{
		BaseActor: NewBaseActor(id, gateway),
		config:    config,
	}
}

func (m *MarketMakerActor) Start(ctx context.Context) error {
	m.Subscribe(m.config.Symbol)
	go m.eventLoop(ctx)
	return m.BaseActor.Start(ctx)
}

func (m *MarketMakerActor) eventLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-m.EventChannel():
			m.OnEvent(event)
		}
	}
}

func (m *MarketMakerActor) OnEvent(event *Event) {
	switch event.Type {
	case EventBookSnapshot:
		m.onBookSnapshot(event.Data.(BookSnapshotEvent))
	case EventTrade:
		m.onTrade(event.Data.(TradeEvent))
	case EventOrderFilled:
		if m.config.RefreshOnFill {
			m.refreshQuotes()
		}
	}
}

func (m *MarketMakerActor) onBookSnapshot(snapshot BookSnapshotEvent) {
	if len(snapshot.Snapshot.Bids) == 0 || len(snapshot.Snapshot.Asks) == 0 {
		return
	}
	bestBid := snapshot.Snapshot.Bids[0].Price
	bestAsk := snapshot.Snapshot.Asks[0].Price
	m.midPrice = (bestBid + bestAsk) / 2
	m.hasSnapshot = true
	m.placeQuotes()
}

func (m *MarketMakerActor) onTrade(trade TradeEvent) {
	m.midPrice = trade.Trade.Price
	if m.hasSnapshot && m.config.RefreshOnFill {
		m.refreshQuotes()
	}
}

func (m *MarketMakerActor) placeQuotes() {
	if m.midPrice == 0 {
		return
	}
	halfSpread := (m.midPrice * m.config.SpreadBps) / (2 * 10000)
	bidPrice := m.midPrice - halfSpread
	askPrice := m.midPrice + halfSpread

	m.SubmitOrder(m.config.Symbol, exchange.Buy, exchange.LimitOrder, bidPrice, m.config.QuoteQty)
	m.SubmitOrder(m.config.Symbol, exchange.Sell, exchange.LimitOrder, askPrice, m.config.QuoteQty)
}

func (m *MarketMakerActor) refreshQuotes() {
	if m.bidOrderID != 0 {
		m.CancelOrder(m.bidOrderID)
	}
	if m.askOrderID != 0 {
		m.CancelOrder(m.askOrderID)
	}
	m.placeQuotes()
}
