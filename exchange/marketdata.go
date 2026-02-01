package exchange

import (
	"sync"
	"sync/atomic"
)

type MDPublisher struct {
	subscriptions map[string]map[uint64]*Subscription
	gateways      map[uint64]*ClientGateway
	mu            sync.RWMutex
	seqNum        uint64
}

func NewMDPublisher() *MDPublisher {
	return &MDPublisher{
		subscriptions: make(map[string]map[uint64]*Subscription),
		gateways:      make(map[uint64]*ClientGateway),
		seqNum:        0,
	}
}

func (p *MDPublisher) Subscribe(clientID uint64, symbol string, types []MDType, gateway *ClientGateway) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.subscriptions[symbol] == nil {
		p.subscriptions[symbol] = make(map[uint64]*Subscription)
	}
	p.subscriptions[symbol][clientID] = &Subscription{
		ClientID: clientID,
		Symbol:   symbol,
		Types:    types,
	}
	p.gateways[clientID] = gateway
}

func (p *MDPublisher) Unsubscribe(clientID uint64, symbol string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.subscriptions[symbol] != nil {
		delete(p.subscriptions[symbol], clientID)
		if len(p.subscriptions[symbol]) == 0 {
			delete(p.subscriptions, symbol)
		}
	}
}

func (p *MDPublisher) Publish(symbol string, mdType MDType, data interface{}, timestamp int64) {
	p.mu.RLock()
	subs := p.subscriptions[symbol]
	if len(subs) == 0 {
		p.mu.RUnlock()
		return
	}

	seqNum := atomic.AddUint64(&p.seqNum, 1)

	for clientID := range subs {
		gateway := p.gateways[clientID]
		if gateway != nil {
			msgCopy := &MarketDataMsg{
				Type:      mdType,
				Symbol:    symbol,
				SeqNum:    seqNum,
				Timestamp: timestamp,
				Data:      data,
			}
			select {
			case gateway.MarketData <- msgCopy:
			default:
			}
		}
	}
	p.mu.RUnlock()
}

func (p *MDPublisher) PublishDelta(symbol string, side Side, price, qty int64, timestamp int64) {
	delta := &BookDelta{
		Side:  side,
		Price: price,
		Qty:   qty,
	}
	p.Publish(symbol, MDDelta, delta, timestamp)
}

func (p *MDPublisher) PublishTrade(symbol string, trade *Trade, timestamp int64) {
	p.Publish(symbol, MDTrade, trade, timestamp)
}

func (p *MDPublisher) PublishFunding(symbol string, funding *FundingRate, timestamp int64) {
	p.Publish(symbol, MDFunding, funding, timestamp)
}
