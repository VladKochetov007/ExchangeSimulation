package actor

import (
	"testing"

	"exchange_sim/exchange"
)

type mockSubActor struct {
	id            uint64
	symbols       []string
	receivedEvents []*Event
	submittedOrders []orderSubmission
	ctx           *SharedContext
	submit        OrderSubmitter
}

type orderSubmission struct {
	symbol    string
	side      exchange.Side
	orderType exchange.OrderType
	price     int64
	qty       int64
}

func newMockSubActor(id uint64, symbols []string) *mockSubActor {
	return &mockSubActor{
		id:      id,
		symbols: symbols,
	}
}

func (m *mockSubActor) GetID() uint64 {
	return m.id
}

func (m *mockSubActor) GetSymbols() []string {
	return m.symbols
}

func (m *mockSubActor) OnEvent(event *Event, ctx *SharedContext, submit OrderSubmitter) {
	m.receivedEvents = append(m.receivedEvents, event)
	m.ctx = ctx
	m.submit = submit
}

func (m *mockSubActor) submitOrder(symbol string, side exchange.Side, orderType exchange.OrderType, price, qty int64) {
	m.submittedOrders = append(m.submittedOrders, orderSubmission{
		symbol:    symbol,
		side:      side,
		orderType: orderType,
		price:     price,
		qty:       qty,
	})
	if m.submit != nil {
		m.submit(symbol, side, orderType, price, qty)
	}
}

func TestCompositeActorEventRouting(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	inst := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", 100_000_000, 100_000, 1, 1000)
	ex.AddInstrument(inst)

	gateway := ex.ConnectClient(1, map[string]int64{"BTC": 10 * 100_000_000}, &exchange.FixedFee{})

	sub1 := newMockSubActor(1, []string{"BTC/USD"})
	sub2 := newMockSubActor(2, []string{"ETH/USD"})
	sub3 := newMockSubActor(3, []string{"BTC/USD"})

	composite := NewCompositeActor(100, gateway, []SubActor{sub1, sub2, sub3})

	bookEvent := &Event{
		Type: EventBookSnapshot,
		Data: BookSnapshotEvent{
			Symbol: "BTC/USD",
			Snapshot: &exchange.BookSnapshot{
				Bids: []exchange.PriceLevel{{Price: 50000, VisibleQty: 100}},
				Asks: []exchange.PriceLevel{{Price: 50100, VisibleQty: 100}},
			},
		},
	}

	composite.OnEvent(bookEvent)

	if len(sub1.receivedEvents) != 1 {
		t.Errorf("sub1 should receive BTC/USD event, got %d events", len(sub1.receivedEvents))
	}
	if len(sub2.receivedEvents) != 0 {
		t.Errorf("sub2 should NOT receive BTC/USD event, got %d events", len(sub2.receivedEvents))
	}
	if len(sub3.receivedEvents) != 1 {
		t.Errorf("sub3 should receive BTC/USD event, got %d events", len(sub3.receivedEvents))
	}
}

func TestCompositeActorBroadcastEvents(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	inst := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", 100_000_000, 100_000, 1, 1000)
	ex.AddInstrument(inst)

	gateway := ex.ConnectClient(1, map[string]int64{"BTC": 10 * 100_000_000}, &exchange.FixedFee{})

	sub1 := newMockSubActor(1, []string{"BTC/USD"})
	sub2 := newMockSubActor(2, []string{"ETH/USD"})

	composite := NewCompositeActor(100, gateway, []SubActor{sub1, sub2})

	orderAccepted := &Event{
		Type: EventOrderAccepted,
		Data: OrderAcceptedEvent{
			OrderID:   1,
			RequestID: 100,
		},
	}

	composite.OnEvent(orderAccepted)

	if len(sub1.receivedEvents) != 1 {
		t.Errorf("sub1 should receive broadcast event, got %d events", len(sub1.receivedEvents))
	}
	if len(sub2.receivedEvents) != 1 {
		t.Errorf("sub2 should receive broadcast event, got %d events", len(sub2.receivedEvents))
	}
}

func TestCompositeActorOrderRejection(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	gateway := ex.ConnectClient(1, map[string]int64{}, &exchange.FixedFee{})

	sub1 := newMockSubActor(1, []string{"BTC/USD"})
	composite := NewCompositeActor(100, gateway, []SubActor{sub1})

	rejectionEvent := &Event{
		Type: EventOrderRejected,
		Data: OrderRejectedEvent{
			RequestID: 100,
			Reason:    exchange.RejectInsufficientBalance,
		},
	}

	composite.OnEvent(rejectionEvent)

	if len(sub1.receivedEvents) != 1 {
		t.Fatalf("Sub-actor should receive rejection event, got %d events", len(sub1.receivedEvents))
	}

	if sub1.receivedEvents[0].Type != EventOrderRejected {
		t.Error("Event should be EventOrderRejected")
	}

	rejData := sub1.receivedEvents[0].Data.(OrderRejectedEvent)
	if rejData.Reason != exchange.RejectInsufficientBalance {
		t.Errorf("Expected RejectInsufficientBalance, got %v", rejData.Reason)
	}
	if rejData.RequestID != 100 {
		t.Errorf("Expected RequestID 100, got %d", rejData.RequestID)
	}
}

func TestSharedContextBalanceTracking(t *testing.T) {
	ctx := NewSharedContext()

	baseBalances := map[string]int64{
		"BTC": 10 * 100_000_000,
		"ETH": 20 * 100_000_000,
	}
	ctx.InitializeBalances(baseBalances, 100_000 * 100_000)

	if ctx.GetBaseBalance("BTC") != 10*100_000_000 {
		t.Errorf("Expected BTC balance 10, got %d", ctx.GetBaseBalance("BTC")/100_000_000)
	}

	if ctx.GetQuoteBalance() != 100_000*100_000 {
		t.Errorf("Expected quote balance 100000, got %d", ctx.GetQuoteBalance()/100_000)
	}

	fill := OrderFillEvent{
		OrderID:   1,
		Side:      exchange.Buy,
		Price:     50000 * 100_000,
		Qty:       1 * 100_000_000,
		FeeAmount: 100_000,
		IsFull:    true,
	}

	ctx.OnFill(1, "BTC/USD", fill, 100_000_000, "BTC")

	expectedBTC := int64(11 * 100_000_000)
	if ctx.GetBaseBalance("BTC") != expectedBTC {
		t.Errorf("After buy, expected BTC balance %d, got %d", expectedBTC, ctx.GetBaseBalance("BTC"))
	}

	expectedQuote := int64(100_000*100_000 - (50000*100_000 + 100_000))
	if ctx.GetQuoteBalance() != expectedQuote {
		t.Errorf("After buy, expected quote balance %d, got %d", expectedQuote, ctx.GetQuoteBalance())
	}
}

func TestSharedContextOMSIntegration(t *testing.T) {
	ctx := NewSharedContext()

	actorID := uint64(1)
	symbol := "BTC/USD"

	actorOMS := ctx.GetActorOMS(actorID, symbol)
	compositeOMS := ctx.GetCompositeOMS(symbol)

	if actorOMS == nil {
		t.Fatal("Actor OMS should not be nil")
	}
	if compositeOMS == nil {
		t.Fatal("Composite OMS should not be nil")
	}

	fill := OrderFillEvent{
		OrderID: 1,
		Side:    exchange.Buy,
		Price:   50000 * 100_000,
		Qty:     1 * 100_000_000,
		IsFull:  true,
	}

	ctx.OnFill(actorID, symbol, fill, 100_000_000, "BTC")

	actorPos := actorOMS.GetNetPosition(symbol)
	compositePos := compositeOMS.GetNetPosition(symbol)

	if actorPos != 1*100_000_000 {
		t.Errorf("Actor position should be 1 BTC, got %d", actorPos)
	}
	if compositePos != 1*100_000_000 {
		t.Errorf("Composite position should be 1 BTC, got %d", compositePos)
	}
}

func TestSharedContextMultipleActors(t *testing.T) {
	ctx := NewSharedContext()
	ctx.InitializeBalances(map[string]int64{"BTC": 0}, 1000_000*100_000)

	symbol := "BTC/USD"

	fill1 := OrderFillEvent{
		OrderID: 1,
		Side:    exchange.Buy,
		Price:   50000 * 100_000,
		Qty:     1 * 100_000_000,
		IsFull:  true,
	}
	ctx.OnFill(1, symbol, fill1, 100_000_000, "BTC")

	fill2 := OrderFillEvent{
		OrderID: 2,
		Side:    exchange.Buy,
		Price:   50000 * 100_000,
		Qty:     2 * 100_000_000,
		IsFull:  true,
	}
	ctx.OnFill(2, symbol, fill2, 100_000_000, "BTC")

	actor1Pos := ctx.GetActorOMS(1, symbol).GetNetPosition(symbol)
	actor2Pos := ctx.GetActorOMS(2, symbol).GetNetPosition(symbol)
	compositePos := ctx.GetCompositeOMS(symbol).GetNetPosition(symbol)

	if actor1Pos != 1*100_000_000 {
		t.Errorf("Actor 1 position should be 1 BTC, got %d", actor1Pos/100_000_000)
	}
	if actor2Pos != 2*100_000_000 {
		t.Errorf("Actor 2 position should be 2 BTC, got %d", actor2Pos/100_000_000)
	}
	if compositePos != 3*100_000_000 {
		t.Errorf("Composite position should be 3 BTC, got %d", compositePos/100_000_000)
	}

	if ctx.GetBaseBalance("BTC") != 3*100_000_000 {
		t.Errorf("Total BTC balance should be 3, got %d", ctx.GetBaseBalance("BTC")/100_000_000)
	}
}

func TestSharedContextCanSubmitOrder(t *testing.T) {
	ctx := NewSharedContext()

	actorID := uint64(1)
	symbol := "BTC/USD"
	maxInventory := int64(5 * 100_000_000)

	if !ctx.CanSubmitOrder(actorID, symbol, exchange.Buy, 2*100_000_000, maxInventory) {
		t.Error("Should be able to submit buy order when position is 0")
	}

	fill := OrderFillEvent{
		OrderID: 1,
		Side:    exchange.Buy,
		Price:   50000 * 100_000,
		Qty:     4 * 100_000_000,
		IsFull:  true,
	}
	ctx.OnFill(actorID, symbol, fill, 100_000_000, "BTC")

	if ctx.CanSubmitOrder(actorID, symbol, exchange.Buy, 2*100_000_000, maxInventory) {
		t.Error("Should NOT be able to buy 2 more when position is 4 and max is 5")
	}

	if !ctx.CanSubmitOrder(actorID, symbol, exchange.Buy, 1*100_000_000, maxInventory) {
		t.Error("Should be able to buy 1 more when position is 4 and max is 5")
	}

	if !ctx.CanSubmitOrder(actorID, symbol, exchange.Sell, 4*100_000_000, maxInventory) {
		t.Error("Should be able to sell 4 to flatten when position is 4")
	}

	if ctx.CanSubmitOrder(actorID, symbol, exchange.Sell, 10*100_000_000, maxInventory) {
		t.Error("Should NOT be able to sell 10 when position is 4 and max is 5 (would go to -6)")
	}
}

func TestCompositeActorMultipleSymbols(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})

	btcInst := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", 100_000_000, 100_000, 1, 1000)
	ethInst := exchange.NewSpotInstrument("ETH/USD", "ETH", "USD", 100_000_000, 100_000, 1, 1000)
	ex.AddInstrument(btcInst)
	ex.AddInstrument(ethInst)

	gateway := ex.ConnectClient(1, map[string]int64{
		"BTC": 10 * 100_000_000,
		"ETH": 100 * 100_000_000,
	}, &exchange.FixedFee{})

	sub1 := newMockSubActor(1, []string{"BTC/USD", "ETH/USD"})
	sub2 := newMockSubActor(2, []string{"BTC/USD"})

	composite := NewCompositeActor(100, gateway, []SubActor{sub1, sub2})

	btcEvent := &Event{
		Type: EventBookSnapshot,
		Data: BookSnapshotEvent{Symbol: "BTC/USD"},
	}
	ethEvent := &Event{
		Type: EventBookSnapshot,
		Data: BookSnapshotEvent{Symbol: "ETH/USD"},
	}

	composite.OnEvent(btcEvent)
	composite.OnEvent(ethEvent)

	if len(sub1.receivedEvents) != 2 {
		t.Errorf("sub1 subscribed to both, should get 2 events, got %d", len(sub1.receivedEvents))
	}
	if len(sub2.receivedEvents) != 1 {
		t.Errorf("sub2 subscribed to BTC only, should get 1 event, got %d", len(sub2.receivedEvents))
	}
}

func TestCompositeActorTradeEventRouting(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	inst := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", 100_000_000, 100_000, 1, 1000)
	ex.AddInstrument(inst)

	gateway := ex.ConnectClient(1, map[string]int64{}, &exchange.FixedFee{})

	sub1 := newMockSubActor(1, []string{"BTC/USD"})
	sub2 := newMockSubActor(2, []string{"ETH/USD"})

	composite := NewCompositeActor(100, gateway, []SubActor{sub1, sub2})

	tradeEvent := &Event{
		Type: EventTrade,
		Data: TradeEvent{
			Symbol: "BTC/USD",
			Trade: &exchange.Trade{
				TradeID: 1,
				Price:   50000,
				Qty:     100,
			},
		},
	}

	composite.OnEvent(tradeEvent)

	if len(sub1.receivedEvents) != 1 {
		t.Errorf("sub1 should receive BTC/USD trade, got %d events", len(sub1.receivedEvents))
	}
	if len(sub2.receivedEvents) != 0 {
		t.Errorf("sub2 should NOT receive BTC/USD trade, got %d events", len(sub2.receivedEvents))
	}

	if sub1.receivedEvents[0].Type != EventTrade {
		t.Error("Event should be EventTrade")
	}
}

func TestSharedContextQuoteReservation(t *testing.T) {
	ctx := NewSharedContext()
	ctx.InitializeBalances(map[string]int64{}, 1000*100_000)

	if !ctx.CanReserveQuote(500 * 100_000) {
		t.Error("Should be able to reserve 500 from 1000")
	}

	if !ctx.ReserveQuote(500 * 100_000) {
		t.Error("Reserve should succeed")
	}

	if ctx.GetAvailableQuote() != 500*100_000 {
		t.Errorf("Available quote should be 500, got %d", ctx.GetAvailableQuote()/100_000)
	}

	if ctx.CanReserveQuote(600 * 100_000) {
		t.Error("Should NOT be able to reserve 600 when only 500 available")
	}

	ctx.ReleaseQuote(200 * 100_000)

	if ctx.GetAvailableQuote() != 700*100_000 {
		t.Errorf("After releasing 200, available should be 700, got %d", ctx.GetAvailableQuote()/100_000)
	}
}
