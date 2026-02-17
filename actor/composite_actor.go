package actor

import (
	"context"
	"sync/atomic"

	"exchange_sim/exchange"
)

type OrderSubmitter func(symbol string, side exchange.Side, orderType exchange.OrderType, price, qty int64) uint64

type SubActor interface {
	OnEvent(event *Event, ctx *SharedContext, submit OrderSubmitter)
	GetSymbols() []string
	GetID() uint64
}

type CompositeActor struct {
	*BaseActor
	subActors     []SubActor
	context       *SharedContext
	symbolRouting map[string][]SubActor
	stopCh        chan struct{}
}

func NewCompositeActor(id uint64, gateway *exchange.ClientGateway, subActors []SubActor) *CompositeActor {
	ca := &CompositeActor{
		BaseActor:     NewBaseActor(id, gateway),
		subActors:     subActors,
		context:       NewSharedContext(),
		symbolRouting: make(map[string][]SubActor),
		stopCh:        make(chan struct{}),
	}

	for _, sub := range subActors {
		for _, symbol := range sub.GetSymbols() {
			ca.symbolRouting[symbol] = append(ca.symbolRouting[symbol], sub)
		}
	}

	return ca
}

func (ca *CompositeActor) GetSharedContext() *SharedContext {
	return ca.context
}

func (ca *CompositeActor) InitializeBalances(baseBalances map[string]int64, quoteBalance int64) {
	ca.context.InitializeBalances(baseBalances, quoteBalance)
}

func (ca *CompositeActor) Start(ctx context.Context) error {
	go ca.eventLoop(ctx)

	if err := ca.BaseActor.Start(ctx); err != nil {
		return err
	}

	allSymbols := make(map[string]bool)
	for _, sub := range ca.subActors {
		for _, symbol := range sub.GetSymbols() {
			allSymbols[symbol] = true
		}
	}

	for symbol := range allSymbols {
		ca.Subscribe(symbol)
	}
	ca.QueryBalance()

	return nil
}

func (ca *CompositeActor) Stop() error {
	close(ca.stopCh)
	return ca.BaseActor.Stop()
}

func (ca *CompositeActor) eventLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ca.stopCh:
			return
		case event := <-ca.EventChannel():
			ca.OnEvent(event)
		}
	}
}

func (ca *CompositeActor) SubmitOrder(symbol string, side exchange.Side, orderType exchange.OrderType, price, qty int64) uint64 {
	reqID := atomic.AddUint64(&ca.BaseActor.requestSeq, 1)

	req := exchange.Request{
		Type: exchange.ReqPlaceOrder,
		OrderReq: &exchange.OrderRequest{
			RequestID: reqID,
			Symbol:    symbol,
			Side:      side,
			Type:      orderType,
			Price:     price,
			Qty:       qty,
		},
	}

	ca.Gateway().RequestCh <- req
	return reqID
}

func (ca *CompositeActor) OnEvent(event *Event) {
	submit := ca.SubmitOrder
	symbol := ca.extractSymbol(event)

	if symbol == "" {
		for _, sub := range ca.subActors {
			sub.OnEvent(event, ca.context, submit)
		}
		return
	}

	if subs, ok := ca.symbolRouting[symbol]; ok {
		for _, sub := range subs {
			sub.OnEvent(event, ca.context, submit)
		}
	}
}

func (ca *CompositeActor) extractSymbol(event *Event) string {
	switch event.Type {
	case EventBookSnapshot:
		return event.Data.(BookSnapshotEvent).Symbol
	case EventBookDelta:
		return event.Data.(BookDeltaEvent).Symbol
	case EventFundingUpdate:
		return event.Data.(FundingUpdateEvent).Symbol
	case EventTrade:
		return event.Data.(TradeEvent).Symbol
	case EventOpenInterest:
		return event.Data.(OpenInterestEvent).Symbol
	}
	return ""
}
