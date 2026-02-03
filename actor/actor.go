package actor

import (
	"context"
	"sync"
	"sync/atomic"

	"exchange_sim/exchange"
)

type Actor interface {
	OnEvent(event *Event)
	Start(ctx context.Context) error
	Stop() error
	ID() uint64
	Gateway() *exchange.ClientGateway
}

type BaseActor struct {
	id         uint64
	gateway    *exchange.ClientGateway
	eventCh    chan *Event
	stopCh     chan struct{}
	running    atomic.Bool
	requestSeq uint64

	// Order tracking for fill notifications
	activeOrders  sync.Map // orderID -> *OrderInfo
	requestToOrder sync.Map // requestID -> orderID
}

type OrderInfo struct {
	OrderID   uint64
	RequestID uint64
	FilledQty int64
	TotalQty  int64
}

func NewBaseActor(id uint64, gateway *exchange.ClientGateway) *BaseActor {
	return &BaseActor{
		id:      id,
		gateway: gateway,
		eventCh: make(chan *Event, 1000),
		stopCh:  make(chan struct{}),
	}
}

func (a *BaseActor) ID() uint64 {
	return a.id
}

func (a *BaseActor) Gateway() *exchange.ClientGateway {
	return a.gateway
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
	for {
		select {
		case <-ctx.Done():
			a.running.Store(false)
			return
		case <-a.stopCh:
			a.running.Store(false)
			return
		case resp := <-a.gateway.ResponseCh:
			a.handleResponse(resp)
		case md := <-a.gateway.MarketData:
			a.handleMarketData(md)
		}
	}
}

func (a *BaseActor) handleResponse(resp exchange.Response) {
	if !resp.Success {
		a.eventCh <- &Event{
			Type: EventOrderRejected,
			Data: OrderRejectedEvent{
				RequestID: resp.RequestID,
				Reason:    resp.Error,
			},
		}
		return
	}

	switch data := resp.Data.(type) {
	case uint64:
		// Order accepted
		a.eventCh <- &Event{
			Type: EventOrderAccepted,
			Data: OrderAcceptedEvent{
				OrderID:   data,
				RequestID: resp.RequestID,
			},
		}
		// Track the order
		a.requestToOrder.Store(resp.RequestID, data)
		a.activeOrders.Store(data, &OrderInfo{
			OrderID:   data,
			RequestID: resp.RequestID,
			FilledQty: 0,
			TotalQty:  0, // We don't know TotalQty here, would need to track from request
		})

	case int64:
		// Order cancelled
		orderID := uint64(0)
		if val, ok := a.requestToOrder.Load(resp.RequestID); ok {
			orderID = val.(uint64)
			a.activeOrders.Delete(orderID)
			a.requestToOrder.Delete(resp.RequestID)
		}
		a.eventCh <- &Event{
			Type: EventOrderCancelled,
			Data: OrderCancelledEvent{
				OrderID:      orderID,
				RequestID:    resp.RequestID,
				RemainingQty: data,
			},
		}

	case *exchange.FillNotification:
		// Fill notification
		isFull := data.IsFull

		// Update order tracking
		if val, ok := a.activeOrders.Load(data.OrderID); ok {
			info := val.(*OrderInfo)
			info.FilledQty += data.Qty
			if isFull {
				a.activeOrders.Delete(data.OrderID)
				a.requestToOrder.Delete(info.RequestID)
			}
		}

		// Generate fill event
		eventType := EventOrderPartialFill
		if isFull {
			eventType = EventOrderFilled
		}

		a.eventCh <- &Event{
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
	}
}

func (a *BaseActor) handleMarketData(md *exchange.MarketDataMsg) {
	switch md.Type {
	case exchange.MDTrade:
		trade := md.Data.(*exchange.Trade)
		a.eventCh <- &Event{
			Type: EventTrade,
			Data: TradeEvent{
				Symbol:    md.Symbol,
				Trade:     trade,
				Timestamp: md.Timestamp,
			},
		}
	case exchange.MDDelta:
		delta := md.Data.(*exchange.BookDelta)
		a.eventCh <- &Event{
			Type: EventBookDelta,
			Data: BookDeltaEvent{
				Symbol:    md.Symbol,
				Delta:     delta,
				Timestamp: md.Timestamp,
			},
		}
	case exchange.MDSnapshot:
		snapshot := md.Data.(*exchange.BookSnapshot)
		a.eventCh <- &Event{
			Type: EventBookSnapshot,
			Data: BookSnapshotEvent{
				Symbol:    md.Symbol,
				Snapshot:  snapshot,
				Timestamp: md.Timestamp,
			},
		}
	case exchange.MDFunding:
		fundingRate := md.Data.(*exchange.FundingRate)
		a.eventCh <- &Event{
			Type: EventFundingUpdate,
			Data: FundingUpdateEvent{
				Symbol:      md.Symbol,
				FundingRate: fundingRate,
				Timestamp:   md.Timestamp,
			},
		}
	case exchange.MDOpenInterest:
		oi := md.Data.(*exchange.OpenInterest)
		a.eventCh <- &Event{
			Type: EventOpenInterest,
			Data: OpenInterestEvent{
				Symbol:       md.Symbol,
				OpenInterest: oi,
				Timestamp:    md.Timestamp,
			},
		}
	}
}

func (a *BaseActor) SubmitOrder(symbol string, side exchange.Side, orderType exchange.OrderType, price, qty int64) {
	reqID := atomic.AddUint64(&a.requestSeq, 1)
	req := exchange.Request{
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
	}
	a.gateway.RequestCh <- req
}

func (a *BaseActor) CancelOrder(orderID uint64) {
	reqID := atomic.AddUint64(&a.requestSeq, 1)
	req := exchange.Request{
		Type: exchange.ReqCancelOrder,
		CancelReq: &exchange.CancelRequest{
			RequestID: reqID,
			OrderID:   orderID,
		},
	}
	a.gateway.RequestCh <- req
}

func (a *BaseActor) QueryBalance() {
	reqID := atomic.AddUint64(&a.requestSeq, 1)
	req := exchange.Request{
		Type: exchange.ReqQueryBalance,
		QueryReq: &exchange.QueryRequest{
			RequestID: reqID,
			QueryType: exchange.QueryBalance,
		},
	}
	a.gateway.RequestCh <- req
}

func (a *BaseActor) Subscribe(symbol string) {
	reqID := atomic.AddUint64(&a.requestSeq, 1)
	req := exchange.Request{
		Type: exchange.ReqSubscribe,
		QueryReq: &exchange.QueryRequest{
			RequestID: reqID,
			Symbol:    symbol,
		},
	}
	a.gateway.RequestCh <- req
}

func (a *BaseActor) Unsubscribe(symbol string) {
	reqID := atomic.AddUint64(&a.requestSeq, 1)
	req := exchange.Request{
		Type: exchange.ReqUnsubscribe,
		QueryReq: &exchange.QueryRequest{
			RequestID: reqID,
			Symbol:    symbol,
		},
	}
	a.gateway.RequestCh <- req
}

func (a *BaseActor) EventChannel() <-chan *Event {
	return a.eventCh
}
