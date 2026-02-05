package exchange

import (
	"testing"
	"time"
)

func TestBookGetSnapshot(t *testing.T) {
	book := newBook(Buy)

	order1 := getOrder()
	order1.ID = 1
	order1.ClientID = 100
	order1.Price = 50000
	order1.Qty = 100
	order1.Visibility = Normal
	book.addOrder(order1)

	order2 := getOrder()
	order2.ID = 2
	order2.ClientID = 101
	order2.Price = 49000
	order2.Qty = 200
	order2.Visibility = Normal
	book.addOrder(order2)

	order3 := getOrder()
	order3.ID = 3
	order3.ClientID = 102
	order3.Price = 48000
	order3.Qty = 150
	order3.Visibility = Normal
	book.addOrder(order3)

	snapshot := book.getSnapshot()

	if len(snapshot) != 3 {
		t.Fatalf("Expected 3 levels, got %d", len(snapshot))
	}

	if snapshot[0].Price != 50000 {
		t.Errorf("First level should be 50000, got %d", snapshot[0].Price)
	}
	if snapshot[0].VisibleQty != 100 {
		t.Errorf("First level qty should be 100, got %d", snapshot[0].VisibleQty)
	}

	if snapshot[1].Price != 49000 {
		t.Errorf("Second level should be 49000, got %d", snapshot[1].Price)
	}
}

func TestVisibleQtyWithIceberg(t *testing.T) {
	limit := &Limit{Price: 50000}

	order1 := &Order{
		ID:         1,
		Qty:        1000,
		FilledQty:  0,
		Visibility: Iceberg,
		IcebergQty: 100,
	}
	linkOrder(limit, order1)

	order2 := &Order{
		ID:         2,
		Qty:        500,
		FilledQty:  0,
		Visibility: Normal,
	}
	linkOrder(limit, order2)

	visible := visibleQty(limit)

	expected := int64(100 + 500)
	if visible != expected {
		t.Errorf("Expected visible qty %d, got %d", expected, visible)
	}
}

func TestVisibleQtyWithHidden(t *testing.T) {
	limit := &Limit{Price: 50000}

	order1 := &Order{
		ID:         1,
		Qty:        1000,
		FilledQty:  0,
		Visibility: Hidden,
	}
	linkOrder(limit, order1)

	order2 := &Order{
		ID:         2,
		Qty:        500,
		FilledQty:  100,
		Visibility: Normal,
	}
	linkOrder(limit, order2)

	visible := visibleQty(limit)

	expected := int64(400)
	if visible != expected {
		t.Errorf("Expected visible qty %d (hidden order not counted), got %d", expected, visible)
	}
}

func TestMDPublisherSubscribeUnsubscribe(t *testing.T) {
	mdp := NewMDPublisher()
	gateway := &ClientGateway{
		ClientID:   1,
		MarketData: make(chan *MarketDataMsg, 100),
	}

	types := []MDType{MDSnapshot, MDDelta, MDTrade}
	mdp.Subscribe(1, "BTC/USD", types, gateway)

	if len(mdp.subscriptions["BTC/USD"]) != 1 {
		t.Errorf("Expected 1 subscription, got %d", len(mdp.subscriptions["BTC/USD"]))
	}

	sub := mdp.subscriptions["BTC/USD"][1]
	if sub == nil {
		t.Fatalf("Subscription not found")
	}
	if sub.ClientID != 1 {
		t.Errorf("Expected client ID 1, got %d", sub.ClientID)
	}
	if sub.Symbol != "BTC/USD" {
		t.Errorf("Expected symbol BTC/USD, got %s", sub.Symbol)
	}

	mdp.Unsubscribe(1, "BTC/USD")

	if len(mdp.subscriptions["BTC/USD"]) != 0 {
		t.Errorf("Expected 0 subscriptions after unsubscribe, got %d", len(mdp.subscriptions["BTC/USD"]))
	}
}

func TestMDPublisherPublishDelta(t *testing.T) {
	mdp := NewMDPublisher()
	gateway := &ClientGateway{
		ClientID:   1,
		MarketData: make(chan *MarketDataMsg, 100),
	}

	types := []MDType{MDDelta}
	mdp.Subscribe(1, "BTC/USD", types, gateway)

	mdp.PublishDelta("BTC/USD", Buy, 50000, 100, 0, time.Now().UnixNano())

	select {
	case msg := <-gateway.MarketData:
		if msg.Type != MDDelta {
			t.Errorf("Expected MDDelta, got %v", msg.Type)
		}
		receivedDelta := msg.Data.(*BookDelta)
		if receivedDelta.Price != 50000 {
			t.Errorf("Expected price 50000, got %d", receivedDelta.Price)
		}
		if receivedDelta.VisibleQty != 100 {
			t.Errorf("Expected visible qty 100, got %d", receivedDelta.VisibleQty)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("Timeout waiting for delta message")
	}
}

func TestMDPublisherPublishFunding(t *testing.T) {
	mdp := NewMDPublisher()
	gateway := &ClientGateway{
		ClientID:   1,
		MarketData: make(chan *MarketDataMsg, 100),
	}

	types := []MDType{MDFunding}
	mdp.Subscribe(1, "BTC-PERP", types, gateway)

	funding := &FundingRate{
		Symbol:      "BTC-PERP",
		Rate:        25,
		NextFunding: time.Now().Unix() + 28800,
		Interval:    28800,
		MarkPrice:   50100 * SATOSHI,
		IndexPrice:  PriceUSD(50000, DOLLAR_TICK),
	}

	mdp.PublishFunding("BTC-PERP", funding, time.Now().UnixNano())

	select {
	case msg := <-gateway.MarketData:
		if msg.Type != MDFunding {
			t.Errorf("Expected MDFunding, got %v", msg.Type)
		}
		receivedFunding := msg.Data.(*FundingRate)
		if receivedFunding.Rate != 25 {
			t.Errorf("Expected rate 25, got %d", receivedFunding.Rate)
		}
		if receivedFunding.Symbol != "BTC-PERP" {
			t.Errorf("Expected symbol BTC-PERP, got %s", receivedFunding.Symbol)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("Timeout waiting for funding message")
	}
}

func TestMDPublisherMultipleSubscribers(t *testing.T) {
	mdp := NewMDPublisher()

	gateway1 := &ClientGateway{
		ClientID:   1,
		MarketData: make(chan *MarketDataMsg, 100),
	}
	gateway2 := &ClientGateway{
		ClientID:   2,
		MarketData: make(chan *MarketDataMsg, 100),
	}

	types := []MDType{MDTrade}
	mdp.Subscribe(1, "BTC/USD", types, gateway1)
	mdp.Subscribe(2, "BTC/USD", types, gateway2)

	trade := &Trade{
		TradeID:      1,
		Price:        50000,
		Qty:          100,
		Side:         Buy,
		TakerOrderID: 1,
		MakerOrderID: 2,
	}

	mdp.PublishTrade("BTC/USD", trade, time.Now().UnixNano())

	received1 := false
	received2 := false

	select {
	case msg := <-gateway1.MarketData:
		if msg.Type == MDTrade {
			received1 = true
		}
	case <-time.After(100 * time.Millisecond):
	}

	select {
	case msg := <-gateway2.MarketData:
		if msg.Type == MDTrade {
			received2 = true
		}
	case <-time.After(100 * time.Millisecond):
	}

	if !received1 {
		t.Errorf("Gateway 1 should receive trade")
	}
	if !received2 {
		t.Errorf("Gateway 2 should receive trade")
	}
}

func TestPoolCleanup(t *testing.T) {
	exec := getExecution()
	exec.TakerOrderID = 123
	exec.MakerOrderID = 456
	exec.TakerClientID = 789
	exec.MakerClientID = 999
	exec.Price = 50000
	exec.Qty = 100
	exec.Timestamp = time.Now().UnixNano()

	putExecution(exec)

	exec2 := getExecution()
	if exec2.TakerOrderID != 0 {
		t.Errorf("TakerOrderID should be reset, got %d", exec2.TakerOrderID)
	}
	if exec2.TakerClientID != 0 {
		t.Errorf("TakerClientID should be reset, got %d", exec2.TakerClientID)
	}
	if exec2.MakerClientID != 0 {
		t.Errorf("MakerClientID should be reset, got %d", exec2.MakerClientID)
	}
}

func TestMDMsgPoolCleanup(t *testing.T) {
	msg := getMDMsg()
	msg.Type = MDTrade
	msg.Symbol = "BTC/USD"
	msg.SeqNum = 123
	msg.Timestamp = time.Now().UnixNano()
	msg.Data = &Trade{}

	putMDMsg(msg)

	msg2 := getMDMsg()
	if msg2.Symbol != "" {
		t.Errorf("Symbol should be reset, got %s", msg2.Symbol)
	}
	if msg2.SeqNum != 0 {
		t.Errorf("SeqNum should be reset, got %d", msg2.SeqNum)
	}
	if msg2.Data != nil {
		t.Errorf("Data should be reset to nil")
	}
}
