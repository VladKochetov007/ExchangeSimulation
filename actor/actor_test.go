package actor

import (
	"context"
	"testing"
	"time"

	"exchange_sim/exchange"
)

func TestBaseActorID(t *testing.T) {
	gateway := exchange.NewClientGateway(123)
	actor := NewBaseActor(123, gateway)

	if actor.ID() != 123 {
		t.Errorf("Expected ID 123, got %d", actor.ID())
	}
}

func TestBaseActorGateway(t *testing.T) {
	gateway := exchange.NewClientGateway(456)
	actor := NewBaseActor(456, gateway)

	if actor.Gateway() != gateway {
		t.Error("Gateway() returned wrong gateway")
	}
}

func TestBaseActorSubmitOrder(t *testing.T) {
	gateway := exchange.NewClientGateway(1)
	actor := NewBaseActor(1, gateway)

	done := make(chan bool)
	go func() {
		actor.SubmitOrder("BTCUSD", exchange.Buy, exchange.LimitOrder, 5000000000000, 100000000)
		done <- true
	}()

	select {
	case req := <-gateway.RequestCh:
		if req.Type != exchange.ReqPlaceOrder {
			t.Errorf("Expected ReqPlaceOrder, got %v", req.Type)
		}
		if req.OrderReq.Symbol != "BTCUSD" {
			t.Errorf("Expected BTCUSD, got %s", req.OrderReq.Symbol)
		}
		if req.OrderReq.Side != exchange.Buy {
			t.Errorf("Expected Buy, got %v", req.OrderReq.Side)
		}
		if req.OrderReq.Price != 5000000000000 {
			t.Errorf("Expected price 5000000000000, got %d", req.OrderReq.Price)
		}
		if req.OrderReq.Qty != 100000000 {
			t.Errorf("Expected qty 100000000, got %d", req.OrderReq.Qty)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for order request")
	}

	<-done
}

func TestBaseActorCancelOrder(t *testing.T) {
	gateway := exchange.NewClientGateway(1)
	actor := NewBaseActor(1, gateway)

	done := make(chan bool)
	go func() {
		actor.CancelOrder(999)
		done <- true
	}()

	select {
	case req := <-gateway.RequestCh:
		if req.Type != exchange.ReqCancelOrder {
			t.Errorf("Expected ReqCancelOrder, got %v", req.Type)
		}
		if req.CancelReq.OrderID != 999 {
			t.Errorf("Expected OrderID 999, got %d", req.CancelReq.OrderID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for cancel request")
	}

	<-done
}

func TestBaseActorQueryBalance(t *testing.T) {
	gateway := exchange.NewClientGateway(1)
	actor := NewBaseActor(1, gateway)

	done := make(chan bool)
	go func() {
		actor.QueryBalance()
		done <- true
	}()

	select {
	case req := <-gateway.RequestCh:
		if req.Type != exchange.ReqQueryBalance {
			t.Errorf("Expected ReqQueryBalance, got %v", req.Type)
		}
		if req.QueryReq.QueryType != exchange.QueryBalance {
			t.Errorf("Expected QueryBalance, got %v", req.QueryReq.QueryType)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for balance query")
	}

	<-done
}

func TestBaseActorSubscribe(t *testing.T) {
	gateway := exchange.NewClientGateway(1)
	actor := NewBaseActor(1, gateway)

	done := make(chan bool)
	go func() {
		actor.Subscribe("ETHUSD")
		done <- true
	}()

	select {
	case req := <-gateway.RequestCh:
		if req.Type != exchange.ReqSubscribe {
			t.Errorf("Expected ReqSubscribe, got %v", req.Type)
		}
		if req.QueryReq.Symbol != "ETHUSD" {
			t.Errorf("Expected ETHUSD, got %s", req.QueryReq.Symbol)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for subscribe request")
	}

	<-done
}

func TestBaseActorUnsubscribe(t *testing.T) {
	gateway := exchange.NewClientGateway(1)
	actor := NewBaseActor(1, gateway)

	done := make(chan bool)
	go func() {
		actor.Unsubscribe("BTCUSD")
		done <- true
	}()

	select {
	case req := <-gateway.RequestCh:
		if req.Type != exchange.ReqUnsubscribe {
			t.Errorf("Expected ReqUnsubscribe, got %v", req.Type)
		}
		if req.QueryReq.Symbol != "BTCUSD" {
			t.Errorf("Expected BTCUSD, got %s", req.QueryReq.Symbol)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for unsubscribe request")
	}

	<-done
}

func TestBaseActorHandleResponseSuccess(t *testing.T) {
	gateway := exchange.NewClientGateway(1)
	actor := NewBaseActor(1, gateway)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	actor.Start(ctx)

	resp := exchange.Response{
		RequestID: 1,
		Success:   true,
		Data:      uint64(123),
	}

	gateway.ResponseCh <- resp

	select {
	case event := <-actor.EventChannel():
		if event.Type != EventOrderAccepted {
			t.Errorf("Expected EventOrderAccepted, got %v", event.Type)
		}
		acceptedEvent := event.Data.(OrderAcceptedEvent)
		if acceptedEvent.OrderID != 123 {
			t.Errorf("Expected OrderID 123, got %d", acceptedEvent.OrderID)
		}
		if acceptedEvent.RequestID != 1 {
			t.Errorf("Expected RequestID 1, got %d", acceptedEvent.RequestID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for OrderAccepted event")
	}

	actor.Stop()
}

func TestBaseActorHandleResponseRejection(t *testing.T) {
	gateway := exchange.NewClientGateway(1)
	actor := NewBaseActor(1, gateway)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	actor.Start(ctx)

	resp := exchange.Response{
		RequestID: 2,
		Success:   false,
		Error:     exchange.RejectInsufficientBalance,
	}

	gateway.ResponseCh <- resp

	select {
	case event := <-actor.EventChannel():
		if event.Type != EventOrderRejected {
			t.Errorf("Expected EventOrderRejected, got %v", event.Type)
		}
		rejectedEvent := event.Data.(OrderRejectedEvent)
		if rejectedEvent.RequestID != 2 {
			t.Errorf("Expected RequestID 2, got %d", rejectedEvent.RequestID)
		}
		if rejectedEvent.Reason != exchange.RejectInsufficientBalance {
			t.Errorf("Expected RejectInsufficientBalance, got %v", rejectedEvent.Reason)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for OrderRejected event")
	}

	actor.Stop()
}

func TestBaseActorHandleResponseCancelled(t *testing.T) {
	gateway := exchange.NewClientGateway(1)
	actor := NewBaseActor(1, gateway)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	actor.Start(ctx)

	resp := exchange.Response{
		RequestID: 3,
		Success:   true,
		Data:      int64(50000000),
	}

	gateway.ResponseCh <- resp

	select {
	case event := <-actor.EventChannel():
		if event.Type != EventOrderCancelled {
			t.Errorf("Expected EventOrderCancelled, got %v", event.Type)
		}
		cancelledEvent := event.Data.(OrderCancelledEvent)
		if cancelledEvent.RequestID != 3 {
			t.Errorf("Expected RequestID 3, got %d", cancelledEvent.RequestID)
		}
		if cancelledEvent.RemainingQty != 50000000 {
			t.Errorf("Expected RemainingQty 50000000, got %d", cancelledEvent.RemainingQty)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for OrderCancelled event")
	}

	actor.Stop()
}

func TestBaseActorHandleMarketDataTrade(t *testing.T) {
	gateway := exchange.NewClientGateway(1)
	actor := NewBaseActor(1, gateway)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	actor.Start(ctx)

	trade := &exchange.Trade{
		TradeID: 456,
		Price:   5000000000000,
		Qty:     100000000,
		Side:    exchange.Buy,
	}

	md := &exchange.MarketDataMsg{
		Type:      exchange.MDTrade,
		Symbol:    "BTCUSD",
		Timestamp: 1234567890000000000,
		Data:      trade,
	}

	gateway.MarketData <- md

	select {
	case event := <-actor.EventChannel():
		if event.Type != EventTrade {
			t.Errorf("Expected EventTrade, got %v", event.Type)
		}
		tradeEvent := event.Data.(TradeEvent)
		if tradeEvent.Symbol != "BTCUSD" {
			t.Errorf("Expected BTCUSD, got %s", tradeEvent.Symbol)
		}
		if tradeEvent.Trade.TradeID != 456 {
			t.Errorf("Expected TradeID 456, got %d", tradeEvent.Trade.TradeID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for Trade event")
	}

	actor.Stop()
}

func TestBaseActorHandleMarketDataSnapshot(t *testing.T) {
	gateway := exchange.NewClientGateway(1)
	actor := NewBaseActor(1, gateway)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	actor.Start(ctx)

	snapshot := &exchange.BookSnapshot{
		Bids: []exchange.PriceLevel{{Price: 5000000000000, Qty: 100000000}},
		Asks: []exchange.PriceLevel{{Price: 5001000000000, Qty: 100000000}},
	}

	md := &exchange.MarketDataMsg{
		Type:      exchange.MDSnapshot,
		Symbol:    "ETHUSD",
		Timestamp: 1234567890000000000,
		Data:      snapshot,
	}

	gateway.MarketData <- md

	select {
	case event := <-actor.EventChannel():
		if event.Type != EventBookSnapshot {
			t.Errorf("Expected EventBookSnapshot, got %v", event.Type)
		}
		snapshotEvent := event.Data.(BookSnapshotEvent)
		if snapshotEvent.Symbol != "ETHUSD" {
			t.Errorf("Expected ETHUSD, got %s", snapshotEvent.Symbol)
		}
		if len(snapshotEvent.Snapshot.Bids) != 1 {
			t.Errorf("Expected 1 bid level, got %d", len(snapshotEvent.Snapshot.Bids))
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for BookSnapshot event")
	}

	actor.Stop()
}

func TestBaseActorHandleMarketDataDelta(t *testing.T) {
	gateway := exchange.NewClientGateway(1)
	actor := NewBaseActor(1, gateway)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	actor.Start(ctx)

	delta := &exchange.BookDelta{
		Side:  exchange.Buy,
		Price: 5000000000000,
		Qty:   100000000,
	}

	md := &exchange.MarketDataMsg{
		Type:      exchange.MDDelta,
		Symbol:    "BTCUSD",
		Timestamp: 1234567890000000000,
		Data:      delta,
	}

	gateway.MarketData <- md

	select {
	case event := <-actor.EventChannel():
		if event.Type != EventBookDelta {
			t.Errorf("Expected EventBookDelta, got %v", event.Type)
		}
		deltaEvent := event.Data.(BookDeltaEvent)
		if deltaEvent.Symbol != "BTCUSD" {
			t.Errorf("Expected BTCUSD, got %s", deltaEvent.Symbol)
		}
		if deltaEvent.Delta.Side != exchange.Buy {
			t.Errorf("Expected Buy side, got %v", deltaEvent.Delta.Side)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for BookDelta event")
	}

	actor.Stop()
}

func TestBaseActorHandleMarketDataFunding(t *testing.T) {
	gateway := exchange.NewClientGateway(1)
	actor := NewBaseActor(1, gateway)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	actor.Start(ctx)

	fundingRate := &exchange.FundingRate{
		Symbol:      "BTCUSD",
		Rate:        10,
		NextFunding: 1234567890,
		Interval:    28800,
	}

	md := &exchange.MarketDataMsg{
		Type:      exchange.MDFunding,
		Symbol:    "BTCUSD",
		Timestamp: 1234567890000000000,
		Data:      fundingRate,
	}

	gateway.MarketData <- md

	select {
	case event := <-actor.EventChannel():
		if event.Type != EventFundingUpdate {
			t.Errorf("Expected EventFundingUpdate, got %v", event.Type)
		}
		fundingEvent := event.Data.(FundingUpdateEvent)
		if fundingEvent.Symbol != "BTCUSD" {
			t.Errorf("Expected BTCUSD, got %s", fundingEvent.Symbol)
		}
		if fundingEvent.FundingRate.Rate != 10 {
			t.Errorf("Expected rate 10, got %d", fundingEvent.FundingRate.Rate)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for FundingUpdate event")
	}

	actor.Stop()
}

func TestBaseActorEventChannel(t *testing.T) {
	gateway := exchange.NewClientGateway(1)
	actor := NewBaseActor(1, gateway)

	ch := actor.EventChannel()
	if ch == nil {
		t.Error("EventChannel() returned nil")
	}
	if cap(ch) != 1000 {
		t.Errorf("Expected channel capacity 1000, got %d", cap(ch))
	}
}
