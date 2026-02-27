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
	fn func(time.Time)
	t  time.Time
}

type BaseActor struct {
	id            uint64
	gateway       Gateway
	eventCh       chan *Event
	stopCh        chan struct{}
	running       atomic.Bool
	requestSeq    uint64
	tickerFactory exchange.TickerFactory

	handler EventHandler
	tickers []tickEntry

	activeOrders   sync.Map // orderID -> *OrderInfo
	requestToOrder sync.Map // requestID -> orderID
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
	go a.run(ctx)
	return nil
}

func (a *BaseActor) Stop() error {
	if !a.running.Load() {
		return nil
	}
	close(a.stopCh)
	return nil
}

func (a *BaseActor) run(ctx context.Context) {
	defer a.running.Store(false)

	tickCh := a.startTickers(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		case resp := <-a.gateway.Responses():
			if evt := a.decodeResponse(resp); evt != nil {
				a.dispatch(ctx, evt)
			}
		case md := <-a.gateway.MarketDataCh():
			if evt := a.decodeMarketData(md); evt != nil {
				a.dispatch(ctx, evt)
			}
		case tc := <-tickCh:
			tc.fn(tc.t)
		}
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
					select {
					case ch <- tickCall{fn, t}:
					case <-ctx.Done():
						return
					case <-stopCh:
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

func (a *BaseActor) decodeResponse(resp exchange.Response) *Event {
	if !resp.Success {
		return &Event{
			Type: EventOrderRejected,
			Data: OrderRejectedEvent{
				RequestID: resp.RequestID,
				Reason:    resp.Error,
			},
		}
	}

	switch data := resp.Data.(type) {
	case uint64:
		a.requestToOrder.Store(resp.RequestID, data)
		a.activeOrders.Store(data, &OrderInfo{
			OrderID:   data,
			RequestID: resp.RequestID,
		})
		return &Event{
			Type: EventOrderAccepted,
			Data: OrderAcceptedEvent{
				OrderID:   data,
				RequestID: resp.RequestID,
			},
		}

	case int64:
		orderID := uint64(0)
		if val, ok := a.requestToOrder.Load(resp.RequestID); ok {
			orderID = val.(uint64)
			a.activeOrders.Delete(orderID)
			a.requestToOrder.Delete(resp.RequestID)
		}
		return &Event{
			Type: EventOrderCancelled,
			Data: OrderCancelledEvent{
				OrderID:      orderID,
				RequestID:    resp.RequestID,
				RemainingQty: data,
			},
		}

	case *exchange.FillNotification:
		isFull := data.IsFull
		if val, ok := a.activeOrders.Load(data.OrderID); ok {
			info := val.(*OrderInfo)
			info.FilledQty += data.Qty
			if isFull {
				a.activeOrders.Delete(data.OrderID)
				a.requestToOrder.Delete(info.RequestID)
			}
		}
		eventType := EventOrderPartialFill
		if isFull {
			eventType = EventOrderFilled
		}
		return &Event{
			Type: eventType,
			Data: OrderFillEvent{
				OrderID:   data.OrderID,
				Qty:       data.Qty,
				Price:     data.Price,
				Side:      data.Side,
				IsFull:    isFull,
				TradeID:   data.TradeID,
				FeeAmount: data.FeeAmount,
				FeeAsset:  data.FeeAsset,
			},
		}

	case *exchange.BalanceSnapshot:
		return &Event{
			Type: EventBalanceUpdate,
			Data: BalanceUpdateEvent{Snapshot: data},
		}

	case *exchange.AccountSnapshot:
		return &Event{
			Type: EventAccountUpdate,
			Data: AccountUpdateEvent{Snapshot: data},
		}
	}
	return nil
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
