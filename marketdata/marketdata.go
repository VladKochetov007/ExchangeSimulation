package marketdata

import (
	"sync"

	etypes "exchange_sim/types"
)

// MDMsg pool — only used within this package.
var mdMsgPool = sync.Pool{
	New: func() any { return &etypes.MarketDataMsg{} },
}

// GetMDMsg retrieves a MarketDataMsg from the pool.
func GetMDMsg() *etypes.MarketDataMsg {
	return mdMsgPool.Get().(*etypes.MarketDataMsg)
}

// PutMDMsg returns a MarketDataMsg to the pool.
func PutMDMsg(m *etypes.MarketDataMsg) {
	m.Type = etypes.MDSnapshot
	m.Symbol = ""
	m.SeqNum = 0
	m.Timestamp = 0
	m.Data = nil
	mdMsgPool.Put(m)
}

// Subscriber is the interface MDPublisher uses to deliver market data.
// *exchange.ClientGateway satisfies this interface.
type Subscriber interface {
	MarketDataChan() chan *etypes.MarketDataMsg
	IsRunning() bool
}

// MDPublisher fans out market data to subscribed gateways.
type MDPublisher struct {
	Subscriptions map[string]map[uint64]*etypes.Subscription
	gateways      map[uint64]Subscriber
	mu            sync.Mutex
	seqNum        uint64
}

func NewMDPublisher() *MDPublisher {
	return &MDPublisher{
		Subscriptions: make(map[string]map[uint64]*etypes.Subscription),
		gateways:      make(map[uint64]Subscriber),
	}
}

func (p *MDPublisher) Subscribe(clientID uint64, symbol string, types []etypes.MDType, gateway Subscriber) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.Subscriptions[symbol] == nil {
		p.Subscriptions[symbol] = make(map[uint64]*etypes.Subscription)
	}
	p.Subscriptions[symbol][clientID] = &etypes.Subscription{
		ClientID: clientID,
		Symbol:   symbol,
		Types:    types,
	}
	p.gateways[clientID] = gateway
}

func (p *MDPublisher) Unsubscribe(clientID uint64, symbol string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.Subscriptions[symbol] != nil {
		delete(p.Subscriptions[symbol], clientID)
		if len(p.Subscriptions[symbol]) == 0 {
			delete(p.Subscriptions, symbol)
		}
	}
}

func (p *MDPublisher) Publish(symbol string, mdType etypes.MDType, data any, timestamp int64) {
	p.mu.Lock()
	subs := p.Subscriptions[symbol]
	if len(subs) == 0 {
		p.mu.Unlock()
		return
	}

	p.seqNum++
	seqNum := p.seqNum

	for clientID := range subs {
		gateway := p.gateways[clientID]
		if gateway != nil {
			if !gateway.IsRunning() {
				continue
			}
			msgCopy := &etypes.MarketDataMsg{
				Type:      mdType,
				Symbol:    symbol,
				SeqNum:    seqNum,
				Timestamp: timestamp,
				Data:      data,
			}
			select {
			case gateway.MarketDataChan() <- msgCopy:
			default:
				// Gateway closed, silently drop
			}
		}
	}
	p.mu.Unlock()
}

func (p *MDPublisher) PublishDelta(symbol string, side etypes.Side, price, visible, hidden int64, timestamp int64) {
	delta := &etypes.BookDelta{
		Side:       side,
		Price:      price,
		VisibleQty: visible,
		HiddenQty:  hidden,
	}
	p.Publish(symbol, etypes.MDDelta, delta, timestamp)
}

func (p *MDPublisher) PublishTrade(symbol string, trade *etypes.Trade, timestamp int64) {
	p.Publish(symbol, etypes.MDTrade, trade, timestamp)
}

func (p *MDPublisher) PublishFunding(symbol string, funding *etypes.FundingRate, timestamp int64) {
	p.Publish(symbol, etypes.MDFunding, funding, timestamp)
}

func (p *MDPublisher) PublishOpenInterest(symbol string, oi *etypes.OpenInterest, timestamp int64) {
	p.Publish(symbol, etypes.MDOpenInterest, oi, timestamp)
}
