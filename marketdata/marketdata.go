package marketdata

import (
	"reflect"
	"slices"
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
	// gateways is keyed the same way as Subscriptions. A client can have
	// different sessions subscribed to different symbols; a client-wide
	// gateway mapping would redirect every existing symbol to the last
	// subscriber gateway.
	gateways map[string]map[uint64]Subscriber
	mu       sync.Mutex
	seqNum   uint64
}

func NewMDPublisher() *MDPublisher {
	return &MDPublisher{
		Subscriptions: make(map[string]map[uint64]*etypes.Subscription),
		gateways:      make(map[string]map[uint64]Subscriber),
	}
}

func (p *MDPublisher) Subscribe(clientID uint64, symbol string, types []etypes.MDType, gateway Subscriber) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.Subscriptions[symbol] == nil {
		p.Subscriptions[symbol] = make(map[uint64]*etypes.Subscription)
		p.gateways[symbol] = make(map[uint64]Subscriber)
	}
	// Subscribing adds feeds rather than replacing them. Unsubscribe removes a
	// whole symbol, not one type, so a replacing Subscribe gave no way to hold
	// two feeds on one symbol: a strategy that wanted snapshots and trades
	// silently kept only whichever it asked for last.
	existing := p.Subscriptions[symbol][clientID]
	merged := make([]etypes.MDType, 0, len(types)+2)
	if existing != nil && sameSubscriber(p.gateways[symbol][clientID], gateway) {
		merged = append(merged, existing.Types...)
	}
	for _, mdType := range types {
		if !slices.Contains(merged, mdType) {
			merged = append(merged, mdType)
		}
	}
	p.Subscriptions[symbol][clientID] = &etypes.Subscription{
		ClientID: clientID,
		Symbol:   symbol,
		Types:    merged,
	}
	p.gateways[symbol][clientID] = gateway
}

// Unsubscribe removes a subscription. When gateway is provided, it must be
// the session that created the subscription; this prevents a delayed request
// from a replaced gateway from removing a reconnect's subscription.
//
// The optional form preserves the direct administrative API, which has no
// session identity to validate.
func (p *MDPublisher) Unsubscribe(clientID uint64, symbol string, gateway ...Subscriber) {
	p.mu.Lock()
	defer p.mu.Unlock()

	subs := p.Subscriptions[symbol]
	if subs == nil {
		return
	}
	if len(gateway) > 0 && !sameSubscriber(p.gateways[symbol][clientID], gateway[0]) {
		return
	}

	delete(subs, clientID)
	delete(p.gateways[symbol], clientID)
	if len(subs) == 0 {
		delete(p.Subscriptions, symbol)
		delete(p.gateways, symbol)
	}
}

// UnsubscribeClient removes every session subscription for a client. Exchange
// reconnect and disconnect paths use this before replacing or discarding the
// gateway, so a new connection never inherits symbols from an old one.
func (p *MDPublisher) UnsubscribeClient(clientID uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for symbol, subs := range p.Subscriptions {
		delete(subs, clientID)
		delete(p.gateways[symbol], clientID)
		if len(subs) == 0 {
			delete(p.Subscriptions, symbol)
			delete(p.gateways, symbol)
		}
	}
}

func sameSubscriber(left, right Subscriber) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	leftValue := reflect.ValueOf(left)
	rightValue := reflect.ValueOf(right)
	if leftValue.Type() != rightValue.Type() || !leftValue.Type().Comparable() {
		return false
	}
	return leftValue.Interface() == rightValue.Interface()
}

func containsMDType(types []etypes.MDType, target etypes.MDType) bool {
	for _, t := range types {
		if t == target {
			return true
		}
	}
	return false
}

func (p *MDPublisher) Publish(symbol string, mdType etypes.MDType, data any, timestamp int64) uint64 {
	p.mu.Lock()
	subs := p.Subscriptions[symbol]
	if len(subs) == 0 {
		p.mu.Unlock()
		return 0
	}

	p.seqNum++
	seqNum := p.seqNum

	// Fan out in client-ID order. Ranging the map delivers in a different
	// order every run, and the subscriber handed the message first is the one
	// that gets to react first — which randomizes precisely the thing a
	// latency experiment is trying to measure. Ordering does hand a fixed
	// advantage to low client IDs when two subscribers are otherwise
	// identical, but that only decides exact ties: subscribers with modelled
	// latency are delivered through the scheduler at their own arrival times,
	// so publish order does not move them.
	clientIDs := make([]uint64, 0, len(subs))
	for clientID := range subs {
		clientIDs = append(clientIDs, clientID)
	}
	slices.Sort(clientIDs)

	for _, clientID := range clientIDs {
		sub := subs[clientID]
		if !containsMDType(sub.Types, mdType) {
			continue
		}
		gateway := p.gateways[symbol][clientID]
		if gateway != nil {
			if !gateway.IsRunning() {
				continue
			}
			msgCopy := &etypes.MarketDataMsg{
				Type:      mdType,
				Symbol:    symbol,
				SeqNum:    seqNum,
				Timestamp: timestamp,
				Data:      cloneMarketDataData(data),
			}
			select {
			case gateway.MarketDataChan() <- msgCopy:
			default:
				// Buffer full (or gateway closed): the message is dropped but its
				// seqNum was already consumed, so a lagging subscriber sees a gap.
				// Known limitation — consumers should treat a seq gap as a signal
				// to re-request a snapshot.
			}
		}
	}
	p.mu.Unlock()
	return seqNum
}

// cloneMarketDataData gives each subscriber independent ownership of mutable
// payloads. In particular, snapshots contain mutable slices and trade payloads
// must not alias OrderBook.LastTrade.
func cloneMarketDataData(data any) any {
	switch value := data.(type) {
	case *etypes.BookSnapshot:
		if value == nil {
			return (*etypes.BookSnapshot)(nil)
		}
		return &etypes.BookSnapshot{
			Bids: slices.Clone(value.Bids),
			Asks: slices.Clone(value.Asks),
		}
	case etypes.BookSnapshot:
		return etypes.BookSnapshot{
			Bids: slices.Clone(value.Bids),
			Asks: slices.Clone(value.Asks),
		}
	case *etypes.BookDelta:
		return clonePointer(value)
	case *etypes.Trade:
		return clonePointer(value)
	case *etypes.FundingRate:
		return clonePointer(value)
	case *etypes.OpenInterest:
		return clonePointer(value)
	case *etypes.InstrumentAnnouncement:
		return clonePointer(value)
	default:
		return data
	}
}

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
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
