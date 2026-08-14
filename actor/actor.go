package actor

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"exchange_sim/exchange"
	"exchange_sim/types"
)

type Gateway = types.Gateway

// EventHandler receives decoded events inline from the actor's run loop.
// Implement this and call SetHandler before Start to avoid a second goroutine.
// When a handler is set, EventChannel is not written.
type EventHandler interface {
	HandleEvent(ctx context.Context, event *Event)
}

// Actor is the interface for any trading participant.
type Actor interface {
	Start(ctx context.Context) error
	Stop() error
	ID() uint64
	Gateway() Gateway
}

type tickEntry struct {
	interval time.Duration
	fn       func(time.Time)
}

type tickCall struct {
	ticker exchange.Ticker
	fn     func(time.Time)
	t      time.Time
}

type phaseTicker struct {
	ticker exchange.Ticker
	fn     func(time.Time)
}

type tickAcknowledger interface {
	Acknowledge()
}

func acknowledgeTick(ticker exchange.Ticker) {
	if acknowledger, ok := ticker.(tickAcknowledger); ok {
		acknowledger.Acknowledge()
	}
}

type BaseActor struct {
	id            uint64
	gateway       Gateway
	eventCh       chan *Event
	stopCh        chan struct{}
	stopOnce      sync.Once
	running       atomic.Bool
	requestSeq    uint64
	tickerFactory exchange.TickerFactory

	handler EventHandler
	tickers []tickEntry

	// phaseMode is an opt-in single-threaded execution mode used by the
	// simulation runner. It keeps the ordinary asynchronous actor path intact,
	// but replaces its select/fan-in goroutines with an explicit inbox pump.
	// It is configured before Start and never changed while running.
	phaseMode    bool
	phaseTickers []phaseTicker

	activeOrders   sync.Map // orderID -> *OrderInfo
	requestToOrder sync.Map // requestID -> orderID
	cancelRequests sync.Map // cancel requestID -> orderID
	// earlyOrderEvents buffers fills/cancels delivered before the placement
	// response. The exchange may match an incoming order before acknowledging
	// it, but strategies need Accepted before they can associate the order ID.
	earlyOrderEvents sync.Map // orderID -> []*Event

	// processing is non-zero while the run loop is inside a handler. A
	// deterministic runner needs to know the difference between "no work
	// queued" and "work queued but not started", and a queue-length check
	// alone cannot see the message already dequeued and being handled.
	processing atomic.Int64
	// pendingTicks counts ticks received by the fan-in goroutines but not yet
	// executed by the run loop. A tick sitting in that hand-off is real
	// pending work that no channel length reveals.
	pendingTicks atomic.Int64
}

// Idle reports whether the actor has nothing queued and nothing in flight.
// Used by the runner's quiescence barrier to decide that simulated time may
// advance without cutting a reaction short.
func (a *BaseActor) Idle() bool {
	idle := a.processing.Load() == 0 &&
		a.pendingTicks.Load() == 0 &&
		len(a.eventCh) == 0 &&
		len(a.gateway.Responses()) == 0 &&
		len(a.gateway.MarketDataCh()) == 0
	if !idle {
		return false
	}
	for _, ticker := range a.phaseTickers {
		if len(ticker.ticker.C()) != 0 {
			return false
		}
	}
	return true
}

type OrderInfo struct {
	OrderID   uint64
	RequestID uint64
	FilledQty int64
	TotalQty  int64
}

func NewBaseActor(id uint64, gateway Gateway) *BaseActor {
	return &BaseActor{
		id:            id,
		gateway:       gateway,
		eventCh:       make(chan *Event, 1000),
		stopCh:        make(chan struct{}),
		tickerFactory: &exchange.RealTickerFactory{},
	}
}

func (a *BaseActor) ID() uint64       { return a.id }
func (a *BaseActor) Gateway() Gateway { return a.gateway }

// SetHandler registers an EventHandler called inline from the run loop.
// Must be called before Start. Mutually exclusive with EventChannel — when
// a handler is set the eventCh is not written.
func (a *BaseActor) SetHandler(h EventHandler) { a.handler = h }

// EnableDeterministicPhases switches this actor to the simulation runner's
// explicit phase pump. It must be called before Start.
func (a *BaseActor) EnableDeterministicPhases() { a.phaseMode = true }

// SupportsDeterministicPhases reports whether the actor can be driven without
// a goroutine. Pull-based EventChannel consumers are intentionally rejected:
// the runner cannot observe or order work performed by an external consumer.
func (a *BaseActor) SupportsDeterministicPhases() bool { return a.handler != nil }

// AddTicker registers a periodic callback driven by the actor's TickerFactory.
// Must be called before Start. The callback fires inside the run goroutine, so
// it shares the same concurrency domain as HandleEvent — no extra locking needed.
// With a SimulatedClock TickerFactory the callback advances with simulation time.
func (a *BaseActor) AddTicker(d time.Duration, fn func(time.Time)) {
	a.tickers = append(a.tickers, tickEntry{d, fn})
}

func (a *BaseActor) Start(ctx context.Context) error {
	if !a.running.CompareAndSwap(false, true) {
		return nil
	}
	if a.phaseMode {
		a.startPhaseTickers()
		return nil
	}
	// Register simulation timers before returning. Runner starts actors in a
	// deterministic order; deferring registration to the run goroutine makes
	// equal-time scheduler sequence IDs depend on Go scheduling.
	tickCh := a.startTickers(ctx)
	go a.run(ctx, tickCh)
	return nil
}

func (a *BaseActor) Stop() error {
	if !a.running.Load() {
		return nil
	}
	a.stopOnce.Do(func() { close(a.stopCh) })
	for _, ticker := range a.phaseTickers {
		ticker.ticker.Stop()
	}
	if a.phaseMode {
		a.running.Store(false)
	}
	return nil
}

func (a *BaseActor) run(ctx context.Context, tickCh <-chan tickCall) {
	defer a.running.Store(false)

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		case resp := <-a.gateway.Responses():
			a.processResponse(ctx, resp)
		case md := <-a.gateway.MarketDataCh():
			a.processMarketData(ctx, md)
		case tc := <-tickCh:
			a.processTick(tc.fn, tc.t)
			a.pendingTicks.Add(-1)
			acknowledgeTick(tc.ticker)
		}
	}
}

func (a *BaseActor) processResponse(ctx context.Context, resp exchange.Response) {
	a.processing.Add(1)
	for _, evt := range a.decodeResponse(resp) {
		a.dispatch(ctx, evt)
	}
	a.processing.Add(-1)
}

func (a *BaseActor) processMarketData(ctx context.Context, md *exchange.MarketDataMsg) {
	a.processing.Add(1)
	if evt := a.decodeMarketData(md); evt != nil {
		a.dispatch(ctx, evt)
	}
	a.processing.Add(-1)
}

func (a *BaseActor) processTick(fn func(time.Time), t time.Time) {
	a.processing.Add(1)
	fn(t)
	a.processing.Add(-1)
}

// PumpDeterministicPhase drains this actor's inbox under a fixed priority:
// responses, market data, then tickers in registration order. Every callback
// runs on the runner goroutine. The policy is deliberately explicit because
// an ordinary select would let the Go scheduler choose economic causality.
func (a *BaseActor) PumpDeterministicPhase(ctx context.Context) bool {
	if !a.phaseMode || !a.running.Load() {
		return false
	}

	processed := false
	for {
		select {
		case resp := <-a.gateway.Responses():
			a.processResponse(ctx, resp)
			processed = true
			continue
		default:
		}

		select {
		case md := <-a.gateway.MarketDataCh():
			a.processMarketData(ctx, md)
			processed = true
			continue
		default:
		}

		for _, ticker := range a.phaseTickers {
			select {
			case t := <-ticker.ticker.C():
				a.processTick(ticker.fn, t)
				acknowledgeTick(ticker.ticker)
				processed = true
				goto nextMessage
			default:
			}
		}
		return processed

	nextMessage:
	}
}

func (a *BaseActor) dispatch(ctx context.Context, evt *Event) {
	if a.handler != nil {
		a.handler.HandleEvent(ctx, evt)
		return
	}
	select {
	case a.eventCh <- evt:
	default:
	}
}

// startTickers creates one fan-in goroutine per registered ticker. Returns a
// nil channel (never fires in select) when no tickers are registered.
// Each goroutine exits on ctx cancellation or stopCh close, whichever comes first.
func (a *BaseActor) startTickers(ctx context.Context) <-chan tickCall {
	if len(a.tickers) == 0 {
		return nil
	}
	ch := make(chan tickCall, len(a.tickers))
	for _, entry := range a.tickers {
		ticker := a.tickerFactory.NewTicker(entry.interval)
		fn := entry.fn
		stopCh := a.stopCh
		go func() {
			defer ticker.Stop()
			for {
				select {
				case t := <-ticker.C():
					a.pendingTicks.Add(1)
					select {
					case ch <- tickCall{ticker: ticker, fn: fn, t: t}:
					case <-ctx.Done():
						a.pendingTicks.Add(-1)
						acknowledgeTick(ticker)
						return
					case <-stopCh:
						a.pendingTicks.Add(-1)
						acknowledgeTick(ticker)
						return
					}
				case <-ctx.Done():
					return
				case <-stopCh:
					return
				}
			}
		}()
	}
	return ch
}

func (a *BaseActor) startPhaseTickers() {
	if len(a.tickers) == 0 {
		return
	}
	a.phaseTickers = make([]phaseTicker, 0, len(a.tickers))
	for _, entry := range a.tickers {
		a.phaseTickers = append(a.phaseTickers, phaseTicker{
			ticker: a.tickerFactory.NewTicker(entry.interval),
			fn:     entry.fn,
		})
	}
}

func (a *BaseActor) decodeResponse(resp exchange.Response) []*Event {
	if !resp.Success {
		if orderID, ok := a.cancelRequests.LoadAndDelete(resp.RequestID); ok {
			return []*Event{{
				Type: EventOrderCancelRejected,
				Data: OrderCancelRejectedEvent{
					OrderID:   orderID.(uint64),
					RequestID: resp.RequestID,
					Reason:    resp.Error,
				},
			}}
		}
		return []*Event{{
			Type: EventOrderRejected,
			Data: OrderRejectedEvent{
				RequestID: resp.RequestID,
				Reason:    resp.Error,
			},
		}}
	}

	switch data := resp.Data.(type) {
	case uint64:
		a.requestToOrder.Store(resp.RequestID, data)
		a.activeOrders.Store(data, &OrderInfo{
			OrderID:   data,
			RequestID: resp.RequestID,
		})
		events := []*Event{{
			Type: EventOrderAccepted,
			Data: OrderAcceptedEvent{
				OrderID:   data,
				RequestID: resp.RequestID,
			},
		}}
		if buffered, ok := a.earlyOrderEvents.LoadAndDelete(data); ok {
			for _, event := range buffered.([]*Event) {
				a.applyOrderEvent(event)
				events = append(events, event)
			}
		}
		return events

	case int64:
		orderID := uint64(0)
		if val, ok := a.cancelRequests.LoadAndDelete(resp.RequestID); ok {
			orderID = val.(uint64)
			a.applyOrderEvent(&Event{Type: EventOrderCancelled, Data: OrderCancelledEvent{OrderID: orderID}})
		}
		return []*Event{{
			Type: EventOrderCancelled,
			Data: OrderCancelledEvent{
				OrderID:      orderID,
				RequestID:    resp.RequestID,
				RemainingQty: data,
			},
		}}

	case *exchange.FillNotification:
		eventType := EventOrderPartialFill
		if data.IsFull {
			eventType = EventOrderFilled
		}
		event := &Event{
			Type: eventType,
			Data: OrderFillEvent{
				OrderID:   data.OrderID,
				Symbol:    data.Symbol,
				Qty:       data.Qty,
				Price:     data.Price,
				Side:      data.Side,
				IsFull:    data.IsFull,
				TradeID:   data.TradeID,
				FeeAmount: data.FeeAmount,
				FeeAsset:  data.FeeAsset,
			},
		}
		if _, ok := a.activeOrders.Load(data.OrderID); !ok {
			a.bufferEarlyOrderEvent(data.OrderID, event)
			return nil
		}
		a.applyOrderEvent(event)
		return []*Event{event}

	case *exchange.ForcedCancelNotification:
		event := &Event{
			Type: EventOrderCancelled,
			Data: OrderCancelledEvent{
				OrderID:      data.OrderID,
				RemainingQty: data.RemainingQty,
			},
		}
		if _, ok := a.activeOrders.Load(data.OrderID); !ok {
			a.bufferEarlyOrderEvent(data.OrderID, event)
			return nil
		}
		a.applyOrderEvent(event)
		return []*Event{event}

	case *exchange.BalanceSnapshot:
		return []*Event{{
			Type: EventBalanceUpdate,
			Data: BalanceUpdateEvent{Snapshot: data},
		}}

	case *exchange.AccountSnapshot:
		return []*Event{{
			Type: EventAccountUpdate,
			Data: AccountUpdateEvent{Snapshot: data},
		}}
	}
	return nil
}

func (a *BaseActor) bufferEarlyOrderEvent(orderID uint64, event *Event) {
	var events []*Event
	if existing, ok := a.earlyOrderEvents.Load(orderID); ok {
		events = existing.([]*Event)
	}
	a.earlyOrderEvents.Store(orderID, append(events, event))
}

func (a *BaseActor) applyOrderEvent(event *Event) {
	switch data := event.Data.(type) {
	case OrderFillEvent:
		value, ok := a.activeOrders.Load(data.OrderID)
		if !ok {
			return
		}
		info := value.(*OrderInfo)
		info.FilledQty += data.Qty
		if data.IsFull {
			a.activeOrders.Delete(data.OrderID)
			a.requestToOrder.Delete(info.RequestID)
		}
	case OrderCancelledEvent:
		if value, ok := a.activeOrders.LoadAndDelete(data.OrderID); ok {
			a.requestToOrder.Delete(value.(*OrderInfo).RequestID)
		}
	}
}

func (a *BaseActor) decodeMarketData(md *exchange.MarketDataMsg) *Event {
	if md == nil {
		return nil
	}
	switch md.Type {
	case exchange.MDTrade:
		return &Event{
			Type: EventTrade,
			Data: TradeEvent{
				Symbol:    md.Symbol,
				Trade:     md.Data.(*exchange.Trade),
				Timestamp: md.Timestamp,
			},
		}
	case exchange.MDDelta:
		return &Event{
			Type: EventBookDelta,
			Data: BookDeltaEvent{
				Symbol:    md.Symbol,
				Delta:     md.Data.(*exchange.BookDelta),
				Timestamp: md.Timestamp,
				SeqNum:    md.SeqNum,
			},
		}
	case exchange.MDSnapshot:
		return &Event{
			Type: EventBookSnapshot,
			Data: BookSnapshotEvent{
				Symbol:    md.Symbol,
				Snapshot:  md.Data.(*exchange.BookSnapshot),
				Timestamp: md.Timestamp,
				SeqNum:    md.SeqNum,
			},
		}
	case exchange.MDFunding:
		return &Event{
			Type: EventFundingUpdate,
			Data: FundingUpdateEvent{
				Symbol:      md.Symbol,
				FundingRate: md.Data.(*exchange.FundingRate),
				Timestamp:   md.Timestamp,
			},
		}
	case exchange.MDOpenInterest:
		return &Event{
			Type: EventOpenInterest,
			Data: OpenInterestEvent{
				Symbol:       md.Symbol,
				OpenInterest: md.Data.(*exchange.OpenInterest),
				Timestamp:    md.Timestamp,
			},
		}
	case exchange.MDInstrument:
		return &Event{
			Type: EventInstrument,
			Data: InstrumentEvent{
				Announcement: md.Data.(*exchange.InstrumentAnnouncement),
				Timestamp:    md.Timestamp,
			},
		}
	}
	return nil
}

func (a *BaseActor) SubmitOrder(symbol string, side exchange.Side, orderType exchange.OrderType, price, qty int64) uint64 {
	reqID := atomic.AddUint64(&a.requestSeq, 1)
	a.gateway.Send(exchange.Request{
		Type: exchange.ReqPlaceOrder,
		OrderReq: &exchange.OrderRequest{
			RequestID:   reqID,
			Side:        side,
			Type:        orderType,
			Price:       price,
			Qty:         qty,
			Symbol:      symbol,
			TimeInForce: exchange.GTC,
			Visibility:  exchange.Normal,
		},
	})
	return reqID
}

func (a *BaseActor) SubmitOrderFull(symbol string, side exchange.Side, orderType exchange.OrderType, price, qty int64, visibility exchange.Visibility, icebergQty int64) {
	reqID := atomic.AddUint64(&a.requestSeq, 1)
	a.gateway.Send(exchange.Request{
		Type: exchange.ReqPlaceOrder,
		OrderReq: &exchange.OrderRequest{
			RequestID:   reqID,
			Side:        side,
			Type:        orderType,
			Price:       price,
			Qty:         qty,
			Symbol:      symbol,
			TimeInForce: exchange.GTC,
			Visibility:  visibility,
			IcebergQty:  icebergQty,
		},
	})
}

func (a *BaseActor) CancelOrder(orderID uint64) {
	reqID := atomic.AddUint64(&a.requestSeq, 1)
	a.cancelRequests.Store(reqID, orderID)
	a.gateway.Send(exchange.Request{
		Type: exchange.ReqCancelOrder,
		CancelReq: &exchange.CancelRequest{
			RequestID: reqID,
			OrderID:   orderID,
		},
	})
}

func (a *BaseActor) QueryBalance() {
	reqID := atomic.AddUint64(&a.requestSeq, 1)
	a.gateway.Send(exchange.Request{
		Type: exchange.ReqQueryBalance,
		QueryReq: &exchange.QueryRequest{
			RequestID: reqID,
			QueryType: exchange.QueryBalance,
		},
	})
}

func (a *BaseActor) Subscribe(symbol string, types ...exchange.MDType) {
	reqID := atomic.AddUint64(&a.requestSeq, 1)
	a.gateway.Send(exchange.Request{
		Type: exchange.ReqSubscribe,
		QueryReq: &exchange.QueryRequest{
			RequestID: reqID,
			Symbol:    symbol,
			Types:     types,
		},
	})
}

func (a *BaseActor) QueryAccount() {
	reqID := atomic.AddUint64(&a.requestSeq, 1)
	a.gateway.Send(exchange.Request{
		Type:     exchange.ReqQueryAccount,
		QueryReq: &exchange.QueryRequest{RequestID: reqID},
	})
}

func (a *BaseActor) Unsubscribe(symbol string) {
	reqID := atomic.AddUint64(&a.requestSeq, 1)
	a.gateway.Send(exchange.Request{
		Type: exchange.ReqUnsubscribe,
		QueryReq: &exchange.QueryRequest{
			RequestID: reqID,
			Symbol:    symbol,
		},
	})
}

// EventChannel returns the event channel for pull-based consumers.
// Do not use together with SetHandler — when a handler is set, this channel
// is not written.
func (a *BaseActor) EventChannel() <-chan *Event {
	return a.eventCh
}

func (a *BaseActor) PeekNextRequestID() uint64 {
	return atomic.LoadUint64(&a.requestSeq) + 1
}

func (a *BaseActor) SetTickerFactory(factory exchange.TickerFactory) {
	a.tickerFactory = factory
}

func (a *BaseActor) GetTickerFactory() exchange.TickerFactory {
	return a.tickerFactory
}
