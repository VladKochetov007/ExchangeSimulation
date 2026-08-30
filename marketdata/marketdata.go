package marketdata

import (
	"exchange_sim/census"
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
	// orderedSubscribers caches each symbol's subscriber list in client-ID
	// order. Publish fans out in that order on every message, and rebuilding
	// and re-sorting it per publish was 250ms of Publish's 570ms on a
	// 15-minute integrated run, over a subscription set that changes only when
	// a client subscribes or disconnects. subscriptionGeneration is bumped by
	// every mutation of Subscriptions or gateways under mu, which invalidates
	// the whole cache.
	orderedSubscribers     map[string][]uint64
	orderedSubscribersGen  map[string]uint64
	subscriptionGeneration uint64
	mu                     sync.Mutex
	seqNum                 uint64
}

func NewMDPublisher() *MDPublisher {
	return &MDPublisher{
		Subscriptions:         make(map[string]map[uint64]*etypes.Subscription),
		gateways:              make(map[string]map[uint64]Subscriber),
		orderedSubscribers:    make(map[string][]uint64),
		orderedSubscribersGen: make(map[string]uint64),
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
	p.subscriptionGeneration++
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
	p.subscriptionGeneration++
	if len(subs) == 0 {
		delete(p.Subscriptions, symbol)
		delete(p.gateways, symbol)
		delete(p.orderedSubscribers, symbol)
		delete(p.orderedSubscribersGen, symbol)
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
		p.subscriptionGeneration++
		if len(subs) == 0 {
			delete(p.Subscriptions, symbol)
			delete(p.gateways, symbol)
			delete(p.orderedSubscribers, symbol)
			delete(p.orderedSubscribersGen, symbol)
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

// subscribersInClientOrder returns a symbol's subscribers in ascending
// client-ID order. Callers must hold p.mu. The returned slice is owned by the
// publisher: read it, do not retain or mutate it.
//
// The length comparison is a guard, not an optimization: a future mutation site
// that forgets to bump subscriptionGeneration would otherwise hand Publish a
// client that no longer subscribes. It is O(1) and turns that mistake into a
// rebuild.
func (p *MDPublisher) subscribersInClientOrder(symbol string, subs map[uint64]*etypes.Subscription) []uint64 {
	if cached, ok := p.orderedSubscribers[symbol]; ok &&
		p.orderedSubscribersGen[symbol] == p.subscriptionGeneration &&
		len(cached) == len(subs) {
		return cached
	}
	clientIDs := make([]uint64, 0, len(subs))
	for clientID := range subs {
		clientIDs = append(clientIDs, clientID)
	}
	slices.Sort(clientIDs)
	p.orderedSubscribers[symbol] = clientIDs
	p.orderedSubscribersGen[symbol] = p.subscriptionGeneration
	return clientIDs
}

// censusPublish counts fan-outs that reach nobody. The payload handed to
// Publish is built by the caller before the call, so a publish with no
// interested subscriber has already paid for its own argument.
var censusPublish = census.Register("marketdata.Publish",
	"no subscriber received it: either none subscribed to the symbol or none wanted this MDType")

// Per-type sites: the aggregate says half the fan-outs reach nobody, but the
// fix belongs at whichever call site builds the most expensive payload for
// them, so the breakdown is what makes the number actionable.
var censusPublishByType = map[etypes.MDType]*census.Site{
	etypes.MDSnapshot:   census.Register("marketdata.Publish[Snapshot]", "reached no subscriber"),
	etypes.MDDelta:      census.Register("marketdata.Publish[Delta]", "reached no subscriber"),
	etypes.MDTrade:      census.Register("marketdata.Publish[Trade]", "reached no subscriber"),
	etypes.MDFunding:    census.Register("marketdata.Publish[Funding]", "reached no subscriber"),
	etypes.MDInstrument: census.Register("marketdata.Publish[Instrument]", "reached no subscriber"),
}

func (p *MDPublisher) Publish(symbol string, mdType etypes.MDType, data any, timestamp int64) {
	delivered := 0
	if census.Enabled {
		defer func() {
			censusPublish.Call(delivered == 0)
			if site := censusPublishByType[mdType]; site != nil {
				site.Call(delivered == 0)
			}
		}()
	}
	p.mu.Lock()
	subs := p.Subscriptions[symbol]
	if len(subs) == 0 {
		p.mu.Unlock()
		return
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
	clientIDs := p.subscribersInClientOrder(symbol, subs)
	symbolGateways := p.gateways[symbol]

	for _, clientID := range clientIDs {
		sub := subs[clientID]
		if !containsMDType(sub.Types, mdType) {
			continue
		}
		delivered++
		gateway := symbolGateways[clientID]
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

// censusDelta counts BookDelta values allocated for a fan-out that reaches
// nobody. Deltas fire on every book mutation, so this is the highest-frequency
// allocation in the publisher.
var censusDelta = census.Register("marketdata.PublishDelta.alloc",
	"BookDelta allocated and then dropped because the symbol has no delta subscriber")

func (p *MDPublisher) PublishDelta(symbol string, side etypes.Side, price, visible, hidden int64, timestamp int64) {
	if census.Enabled {
		p.mu.Lock()
		wanted := false
		for _, sub := range p.Subscriptions[symbol] {
			if containsMDType(sub.Types, etypes.MDDelta) {
				wanted = true
				break
			}
		}
		p.mu.Unlock()
		censusDelta.Call(!wanted)
	}
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
