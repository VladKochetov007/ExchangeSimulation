package actor

import (
	"context"
	"testing"
	"time"

	"exchange_sim/exchange"
)

func TestMarketMakerCreation(t *testing.T) {
	gateway := exchange.NewClientGateway(1)
	config := MarketMakerConfig{
		Symbol:        "BTCUSD",
		SpreadBps:     20,
		QuoteQty:      100000000,
		RefreshOnFill: false,
	}

	mm := NewMarketMaker(1, gateway, config)
	if mm == nil {
		t.Fatal("NewMarketMaker returned nil")
	}
	if mm.config.Symbol != "BTCUSD" {
		t.Errorf("Expected symbol BTCUSD, got %s", mm.config.Symbol)
	}
	if mm.config.SpreadBps != 20 {
		t.Errorf("Expected spread 20, got %d", mm.config.SpreadBps)
	}
}

func TestMarketMakerStart(t *testing.T) {
	gateway := exchange.NewClientGateway(1)
	config := MarketMakerConfig{
		Symbol:        "BTCUSD",
		SpreadBps:     20,
		QuoteQty:      100000000,
		RefreshOnFill: false,
	}

	mm := NewMarketMaker(1, gateway, config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := mm.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	select {
	case req := <-gateway.RequestCh:
		if req.Type != exchange.ReqSubscribe {
			t.Errorf("Expected ReqSubscribe, got %v", req.Type)
		}
		if req.QueryReq.Symbol != "BTCUSD" {
			t.Errorf("Expected BTCUSD, got %s", req.QueryReq.Symbol)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for subscribe request")
	}

	mm.Stop()
}

func TestMarketMakerOnBookSnapshot(t *testing.T) {
	gateway := exchange.NewClientGateway(1)
	config := MarketMakerConfig{
		Symbol:        "BTCUSD",
		SpreadBps:     20,
		QuoteQty:      100000000,
		RefreshOnFill: false,
	}

	mm := NewMarketMaker(1, gateway, config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mm.Start(ctx)

	<-gateway.RequestCh

	snapshot := &exchange.BookSnapshot{
		Bids: []exchange.PriceLevel{{Price: 5000000000000, Qty: 100000000}},
		Asks: []exchange.PriceLevel{{Price: 5010000000000, Qty: 100000000}},
	}

	event := &Event{
		Type: EventBookSnapshot,
		Data: BookSnapshotEvent{
			Symbol:    "BTCUSD",
			Snapshot:  snapshot,
			Timestamp: 1234567890000000000,
		},
	}

	mm.eventCh <- event

	ordersPlaced := 0
	timeout := time.After(200 * time.Millisecond)

	for ordersPlaced < 2 {
		select {
		case req := <-gateway.RequestCh:
			if req.Type == exchange.ReqPlaceOrder {
				ordersPlaced++
				orderReq := req.OrderReq
				if orderReq.Symbol != "BTCUSD" {
					t.Errorf("Expected BTCUSD, got %s", orderReq.Symbol)
				}
				if orderReq.Qty != 100000000 {
					t.Errorf("Expected qty 100000000, got %d", orderReq.Qty)
				}
			}
		case <-timeout:
			t.Fatalf("Timeout waiting for orders, got %d/2", ordersPlaced)
		}
	}

	mm.Stop()
}

func TestMarketMakerOnTrade(t *testing.T) {
	gateway := exchange.NewClientGateway(1)
	config := MarketMakerConfig{
		Symbol:        "BTCUSD",
		SpreadBps:     20,
		QuoteQty:      100000000,
		RefreshOnFill: true,
	}

	mm := NewMarketMaker(1, gateway, config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mm.Start(ctx)

	<-gateway.RequestCh

	snapshot := &exchange.BookSnapshot{
		Bids: []exchange.PriceLevel{{Price: 5000000000000, Qty: 100000000}},
		Asks: []exchange.PriceLevel{{Price: 5010000000000, Qty: 100000000}},
	}

	snapshotEvent := &Event{
		Type: EventBookSnapshot,
		Data: BookSnapshotEvent{
			Symbol:    "BTCUSD",
			Snapshot:  snapshot,
			Timestamp: 1234567890000000000,
		},
	}

	mm.eventCh <- snapshotEvent

	<-gateway.RequestCh
	<-gateway.RequestCh

	trade := &exchange.Trade{
		TradeID: 123,
		Price:   5005000000000,
		Qty:     50000000,
		Side:    exchange.Buy,
	}

	tradeEvent := &Event{
		Type: EventTrade,
		Data: TradeEvent{
			Symbol:    "BTCUSD",
			Trade:     trade,
			Timestamp: 1234567890000000000,
		},
	}

	mm.eventCh <- tradeEvent

	time.Sleep(50 * time.Millisecond)

	if mm.midPrice != 5005000000000 {
		t.Errorf("Expected midPrice 5005000000000, got %d", mm.midPrice)
	}

	mm.Stop()
}

func TestMarketMakerSpreadCalculation(t *testing.T) {
	gateway := exchange.NewClientGateway(1)
	config := MarketMakerConfig{
		Symbol:        "BTCUSD",
		SpreadBps:     100,
		QuoteQty:      100000000,
		RefreshOnFill: false,
	}

	mm := NewMarketMaker(1, gateway, config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mm.Start(ctx)

	<-gateway.RequestCh

	snapshot := &exchange.BookSnapshot{
		Bids: []exchange.PriceLevel{{Price: 10000000000000, Qty: 100000000}},
		Asks: []exchange.PriceLevel{{Price: 10000000000000, Qty: 100000000}},
	}

	event := &Event{
		Type: EventBookSnapshot,
		Data: BookSnapshotEvent{
			Symbol:    "BTCUSD",
			Snapshot:  snapshot,
			Timestamp: 1234567890000000000,
		},
	}

	mm.eventCh <- event

	ordersReceived := 0
	var bidPrice, askPrice int64

	timeout := time.After(200 * time.Millisecond)

	for ordersReceived < 2 {
		select {
		case req := <-gateway.RequestCh:
			if req.Type == exchange.ReqPlaceOrder {
				ordersReceived++
				if req.OrderReq.Side == exchange.Buy {
					bidPrice = req.OrderReq.Price
				} else {
					askPrice = req.OrderReq.Price
				}
			}
		case <-timeout:
			t.Fatalf("Timeout waiting for orders")
		}
	}

	midPrice := int64(10000000000000)
	expectedHalfSpread := (midPrice * 100) / (2 * 10000)

	if bidPrice != midPrice-expectedHalfSpread {
		t.Errorf("Expected bid price %d, got %d", midPrice-expectedHalfSpread, bidPrice)
	}
	if askPrice != midPrice+expectedHalfSpread {
		t.Errorf("Expected ask price %d, got %d", midPrice+expectedHalfSpread, askPrice)
	}

	mm.Stop()
}

func TestMarketMakerEmptyBookSnapshot(t *testing.T) {
	gateway := exchange.NewClientGateway(1)
	config := MarketMakerConfig{
		Symbol:        "BTCUSD",
		SpreadBps:     20,
		QuoteQty:      100000000,
		RefreshOnFill: false,
	}

	mm := NewMarketMaker(1, gateway, config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mm.Start(ctx)

	<-gateway.RequestCh

	emptySnapshot := &exchange.BookSnapshot{
		Bids: []exchange.PriceLevel{},
		Asks: []exchange.PriceLevel{},
	}

	event := &Event{
		Type: EventBookSnapshot,
		Data: BookSnapshotEvent{
			Symbol:    "BTCUSD",
			Snapshot:  emptySnapshot,
			Timestamp: 1234567890000000000,
		},
	}

	mm.eventCh <- event

	select {
	case <-gateway.RequestCh:
		t.Error("Should not place orders on empty book snapshot")
	case <-time.After(100 * time.Millisecond):
	}

	if mm.hasSnapshot {
		t.Error("hasSnapshot should be false for empty book")
	}

	mm.Stop()
}

func TestMarketMakerRefreshOnFill(t *testing.T) {
	gateway := exchange.NewClientGateway(1)
	config := MarketMakerConfig{
		Symbol:        "BTCUSD",
		SpreadBps:     20,
		QuoteQty:      100000000,
		RefreshOnFill: true,
	}

	mm := NewMarketMaker(1, gateway, config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mm.Start(ctx)

	<-gateway.RequestCh

	snapshot := &exchange.BookSnapshot{
		Bids: []exchange.PriceLevel{{Price: 5000000000000, Qty: 100000000}},
		Asks: []exchange.PriceLevel{{Price: 5010000000000, Qty: 100000000}},
	}

	snapshotEvent := &Event{
		Type: EventBookSnapshot,
		Data: BookSnapshotEvent{
			Symbol:    "BTCUSD",
			Snapshot:  snapshot,
			Timestamp: 1234567890000000000,
		},
	}

	mm.eventCh <- snapshotEvent

	<-gateway.RequestCh
	<-gateway.RequestCh

	mm.bidOrderID = 100
	mm.askOrderID = 200

	fillEvent := &Event{
		Type: EventOrderFilled,
		Data: OrderFillEvent{
			OrderID: 100,
			Qty:     50000000,
			Price:   5000000000000,
			Side:    exchange.Buy,
			IsFull:  true,
		},
	}

	mm.eventCh <- fillEvent

	cancelCount := 0
	placeCount := 0
	timeout := time.After(200 * time.Millisecond)

loop:
	for {
		select {
		case req := <-gateway.RequestCh:
			if req.Type == exchange.ReqCancelOrder {
				cancelCount++
			} else if req.Type == exchange.ReqPlaceOrder {
				placeCount++
				if placeCount >= 2 {
					break loop
				}
			}
		case <-timeout:
			break loop
		}
	}

	if cancelCount < 1 {
		t.Errorf("Expected at least 1 cancel request, got %d", cancelCount)
	}
	if placeCount < 2 {
		t.Errorf("Expected 2 place requests, got %d", placeCount)
	}

	mm.Stop()
}

func TestMarketMakerNoRefreshOnFill(t *testing.T) {
	gateway := exchange.NewClientGateway(1)
	config := MarketMakerConfig{
		Symbol:        "BTCUSD",
		SpreadBps:     20,
		QuoteQty:      100000000,
		RefreshOnFill: false,
	}

	mm := NewMarketMaker(1, gateway, config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mm.Start(ctx)

	<-gateway.RequestCh

	snapshot := &exchange.BookSnapshot{
		Bids: []exchange.PriceLevel{{Price: 5000000000000, Qty: 100000000}},
		Asks: []exchange.PriceLevel{{Price: 5010000000000, Qty: 100000000}},
	}

	snapshotEvent := &Event{
		Type: EventBookSnapshot,
		Data: BookSnapshotEvent{
			Symbol:    "BTCUSD",
			Snapshot:  snapshot,
			Timestamp: 1234567890000000000,
		},
	}

	mm.eventCh <- snapshotEvent

	<-gateway.RequestCh
	<-gateway.RequestCh

	fillEvent := &Event{
		Type: EventOrderFilled,
		Data: OrderFillEvent{
			OrderID: 100,
			Qty:     50000000,
			Price:   5000000000000,
			Side:    exchange.Buy,
			IsFull:  true,
		},
	}

	mm.eventCh <- fillEvent

	select {
	case <-gateway.RequestCh:
		t.Error("Should not refresh quotes when RefreshOnFill is false")
	case <-time.After(100 * time.Millisecond):
	}

	mm.Stop()
}
