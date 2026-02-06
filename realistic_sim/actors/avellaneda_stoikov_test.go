package actors

import (
	"context"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

func TestAvellanedaStoikovReservationPrice(t *testing.T) {
	ex := exchange.NewExchange(100, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION, 1, exchange.BTC_PRECISION/100)
	ex.AddInstrument(instrument)

	balances := map[string]int64{
		"BTC": 100 * exchange.BTC_PRECISION,
		"USD": 1000000 * exchange.USD_PRECISION,
	}
	feePlan := &exchange.PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true}
	gateway := ex.ConnectClient(1, balances, feePlan)

	config := AvellanedaStoikovConfig{
		Symbol:           "BTC/USD",
		Gamma:            100000,
		K:                10000,
		T:                3600,
		QuoteQty:         exchange.BTC_PRECISION / 10,
		MaxInventory:     10 * exchange.BTC_PRECISION,
		VolatilityWindow: 20,
		RequoteInterval:  100 * time.Millisecond,
	}

	asActor := NewAvellanedaStoikov(1, gateway, config)
	asActor.SetInstrument(instrument)
	asActor.lastMid = 50000 * exchange.USD_PRECISION
	asActor.startTime = time.Now()

	basePrice := int64(50000 * exchange.USD_PRECISION)
	for i := 0; i < 21; i++ {
		price := basePrice
		if i%3 == 0 {
			price += 2000 * exchange.USD_PRECISION
		} else if i%3 == 1 {
			price -= 2000 * exchange.USD_PRECISION
		}
		asActor.volatility.AddPrice(price)
	}

	t.Logf("Volatility size: %d, sigma: %d", asActor.volatility.Size(), asActor.volatility.Volatility())

	asActor.inventory = 0
	bid0, ask0 := asActor.calculateQuotes()
	if bid0 == 0 || ask0 == 0 {
		t.Fatalf("calculateQuotes returned zero: bid0=%d, ask0=%d", bid0, ask0)
	}
	mid0 := (bid0 + ask0) / 2
	t.Logf("inventory=0: bid=%d, ask=%d, mid=%d", bid0, ask0, mid0)

	asActor.inventory = 5 * exchange.BTC_PRECISION
	bidLong, askLong := asActor.calculateQuotes()
	midLong := (bidLong + askLong) / 2
	t.Logf("inventory=5 BTC: bid=%d, ask=%d, mid=%d", bidLong, askLong, midLong)

	asActor.inventory = -5 * exchange.BTC_PRECISION
	bidShort, askShort := asActor.calculateQuotes()
	midShort := (bidShort + askShort) / 2
	t.Logf("inventory=-5 BTC: bid=%d, ask=%d, mid=%d", bidShort, askShort, midShort)

	t.Logf("mid0=$%.8f, midLong=$%.8f, midShort=$%.8f",
		float64(mid0)/float64(exchange.USD_PRECISION),
		float64(midLong)/float64(exchange.USD_PRECISION),
		float64(midShort)/float64(exchange.USD_PRECISION))

	if midLong >= mid0 {
		t.Errorf("Long inventory should lower reservation price: mid0=%d, midLong=%d", mid0, midLong)
	}

	if midShort <= mid0 {
		t.Errorf("Short inventory should raise reservation price: mid0=%d, midShort=%d", mid0, midShort)
	}
}

func TestAvellanedaStoikovSpreadWidensWithVolatility(t *testing.T) {
	ex := exchange.NewExchange(100, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION, exchange.DOLLAR_TICK, exchange.BTC_PRECISION/100)
	ex.AddInstrument(instrument)

	balances := map[string]int64{
		"BTC": 100 * exchange.BTC_PRECISION,
		"USD": 1000000 * exchange.USD_PRECISION,
	}
	feePlan := &exchange.PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true}
	gateway := ex.ConnectClient(1, balances, feePlan)

	config := AvellanedaStoikovConfig{
		Symbol:           "BTC/USD",
		Gamma:            1000,
		K:                10000,
		T:                3600,
		QuoteQty:         exchange.BTC_PRECISION / 10,
		MaxInventory:     10 * exchange.BTC_PRECISION,
		VolatilityWindow: 20,
		RequoteInterval:  100 * time.Millisecond,
	}

	asActor := NewAvellanedaStoikov(1, gateway, config)
	asActor.SetInstrument(instrument)
	asActor.lastMid = 50000 * exchange.USD_PRECISION

	for i := 0; i < 20; i++ {
		asActor.volatility.AddPrice(50000 * exchange.USD_PRECISION)
	}

	bid1, ask1 := asActor.calculateQuotes()
	spread1 := ask1 - bid1

	for i := 0; i < 20; i++ {
		price := int64(50000 * exchange.USD_PRECISION)
		if i%2 == 0 {
			price += 500 * exchange.USD_PRECISION
		}
		asActor.volatility.AddPrice(price)
	}

	bid2, ask2 := asActor.calculateQuotes()
	spread2 := ask2 - bid2

	if spread2 <= spread1 {
		t.Errorf("Higher volatility should widen spread: spread1=%d, spread2=%d", spread1, spread2)
	}
}

func TestAvellanedaStoikovTickAlignment(t *testing.T) {
	ex := exchange.NewExchange(100, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION, exchange.DOLLAR_TICK, exchange.BTC_PRECISION/100)
	ex.AddInstrument(instrument)

	balances := map[string]int64{
		"BTC": 100 * exchange.BTC_PRECISION,
		"USD": 1000000 * exchange.USD_PRECISION,
	}
	feePlan := &exchange.PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true}
	gateway := ex.ConnectClient(1, balances, feePlan)

	config := AvellanedaStoikovConfig{
		Symbol:           "BTC/USD",
		Gamma:            1000,
		K:                10000,
		T:                3600,
		QuoteQty:         exchange.BTC_PRECISION / 10,
		MaxInventory:     10 * exchange.BTC_PRECISION,
		VolatilityWindow: 20,
		RequoteInterval:  100 * time.Millisecond,
	}

	asActor := NewAvellanedaStoikov(1, gateway, config)
	asActor.SetInstrument(instrument)
	asActor.lastMid = 50000 * exchange.USD_PRECISION

	for i := 0; i < 20; i++ {
		asActor.volatility.AddPrice(50000 * exchange.USD_PRECISION)
	}

	bid, ask := asActor.calculateQuotes()

	tickSize := instrument.TickSize()

	if bid%tickSize != 0 {
		t.Errorf("Bid price not aligned to tick size: bid=%d, tickSize=%d, remainder=%d", bid, tickSize, bid%tickSize)
	}

	if ask%tickSize != 0 {
		t.Errorf("Ask price not aligned to tick size: ask=%d, tickSize=%d, remainder=%d", ask, tickSize, ask%tickSize)
	}
}

func TestAvellanedaStoikovInventoryLimits(t *testing.T) {
	ex := exchange.NewExchange(100, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION, exchange.DOLLAR_TICK, exchange.BTC_PRECISION/100)
	ex.AddInstrument(instrument)

	balances := map[string]int64{
		"BTC": 100 * exchange.BTC_PRECISION,
		"USD": 1000000 * exchange.USD_PRECISION,
	}
	feePlan := &exchange.PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true}
	gateway := ex.ConnectClient(1, balances, feePlan)

	config := AvellanedaStoikovConfig{
		Symbol:           "BTC/USD",
		Gamma:            1000,
		K:                10000,
		T:                3600,
		QuoteQty:         exchange.BTC_PRECISION / 10,
		MaxInventory:     5 * exchange.BTC_PRECISION,
		VolatilityWindow: 20,
		RequoteInterval:  100 * time.Millisecond,
	}

	asActor := NewAvellanedaStoikov(1, gateway, config)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := asActor.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start actor: %v", err)
	}
	defer asActor.Stop()

	asActor.Subscribe("BTC/USD")

	asActor.SetInstrument(instrument)
	asActor.lastMid = 50000 * exchange.USD_PRECISION

	for i := 0; i < 20; i++ {
		asActor.volatility.AddPrice(50000 * exchange.USD_PRECISION)
	}

	asActor.inventory = 6 * exchange.BTC_PRECISION
	asActor.placeQuotes()

	time.Sleep(50 * time.Millisecond)

	if asActor.activeBidID != 0 {
		t.Error("Should not place bid when at max long inventory")
	}

	asActor.inventory = -6 * exchange.BTC_PRECISION
	asActor.placeQuotes()

	time.Sleep(50 * time.Millisecond)

	if asActor.activeAskID != 0 {
		t.Error("Should not place ask when at max short inventory")
	}
}

func TestAvellanedaStoikovIntegration(t *testing.T) {
	ex := exchange.NewExchange(100, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION, exchange.DOLLAR_TICK, exchange.BTC_PRECISION/100)
	ex.AddInstrument(instrument)

	mmBalances := map[string]int64{
		"BTC": 50 * exchange.BTC_PRECISION,
		"USD": 5000000 * exchange.USD_PRECISION,
	}
	takerBalances := map[string]int64{
		"BTC": 50 * exchange.BTC_PRECISION,
		"USD": 5000000 * exchange.USD_PRECISION,
	}
	feePlan := &exchange.PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true}
	mmGateway := ex.ConnectClient(1, mmBalances, feePlan)
	takerGateway := ex.ConnectClient(2, takerBalances, feePlan)

	config := AvellanedaStoikovConfig{
		Symbol:           "BTC/USD",
		Gamma:            1000,
		K:                10000,
		T:                3600,
		QuoteQty:         exchange.BTC_PRECISION / 10,
		MaxInventory:     5 * exchange.BTC_PRECISION,
		VolatilityWindow: 20,
		RequoteInterval:  50 * time.Millisecond,
	}

	asActor := NewAvellanedaStoikov(1, mmGateway, config)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := asActor.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start AS actor: %v", err)
	}
	defer asActor.Stop()

	asActor.SetInstrument(instrument)
	asActor.lastMid = 50000 * exchange.USD_PRECISION

	for i := 0; i < 20; i++ {
		asActor.volatility.AddPrice(50000 * exchange.USD_PRECISION)
	}

	time.Sleep(100 * time.Millisecond)

	for i := 0; i < 50; i++ {
		takerReq := exchange.Request{
			Type: exchange.ReqPlaceOrder,
			OrderReq: &exchange.OrderRequest{
				Side:        exchange.Buy,
				Type:        exchange.Market,
				Qty:         exchange.BTC_PRECISION / 20,
				Symbol:      "BTC/USD",
				TimeInForce: exchange.GTC,
				Visibility:  exchange.Normal,
			},
		}
		takerGateway.RequestCh <- takerReq

		time.Sleep(20 * time.Millisecond)

		takerReq.OrderReq.Side = exchange.Sell
		takerGateway.RequestCh <- takerReq

		time.Sleep(20 * time.Millisecond)
	}

	<-ctx.Done()

	inventory := asActor.GetInventory()
	absInventory := inventory
	if absInventory < 0 {
		absInventory = -absInventory
	}

	if absInventory > config.MaxInventory {
		t.Errorf("Inventory exceeded max: inventory=%d, max=%d", absInventory, config.MaxInventory)
	}
}

func TestAvellanedaStoikovOrderRejection(t *testing.T) {
	ex := exchange.NewExchange(100, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION, exchange.DOLLAR_TICK, exchange.BTC_PRECISION/100)
	ex.AddInstrument(instrument)

	balances := map[string]int64{
		"BTC": 0, // No BTC balance - orders will be rejected
		"USD": 100 * exchange.USD_PRECISION,
	}
	feePlan := &exchange.PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true}
	gateway := ex.ConnectClient(1, balances, feePlan)

	config := AvellanedaStoikovConfig{
		Symbol:           "BTC/USD",
		Gamma:            1000,
		K:                10000,
		T:                3600,
		QuoteQty:         exchange.BTC_PRECISION / 10,
		MaxInventory:     5 * exchange.BTC_PRECISION,
		VolatilityWindow: 20,
		RequoteInterval:  100 * time.Millisecond,
	}

	asActor := NewAvellanedaStoikov(1, gateway, config)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := asActor.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start actor: %v", err)
	}
	defer asActor.Stop()

	asActor.SetInstrument(instrument)
	asActor.lastMid = 50000 * exchange.USD_PRECISION

	for i := 0; i < 20; i++ {
		asActor.volatility.AddPrice(50000 * exchange.USD_PRECISION)
	}

	asActor.placeQuotes()

	time.Sleep(100 * time.Millisecond)

	if asActor.lastBidReqID != 0 {
		t.Errorf("lastBidReqID should be cleared after rejection, got %d", asActor.lastBidReqID)
	}

	if asActor.lastAskReqID != 0 {
		t.Errorf("lastAskReqID should be cleared after rejection, got %d", asActor.lastAskReqID)
	}

	if asActor.activeBidID != 0 {
		t.Error("activeBidID should remain 0 after rejection")
	}

	if asActor.activeAskID != 0 {
		t.Error("activeAskID should remain 0 after rejection")
	}
}

func TestAvellanedaStoikovCancelRejection(t *testing.T) {
	ex := exchange.NewExchange(100, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION, exchange.DOLLAR_TICK, exchange.BTC_PRECISION/100)
	ex.AddInstrument(instrument)

	balances := map[string]int64{
		"BTC": 100 * exchange.BTC_PRECISION,
		"USD": 1000000 * exchange.USD_PRECISION,
	}
	feePlan := &exchange.PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true}
	gateway := ex.ConnectClient(1, balances, feePlan)

	config := AvellanedaStoikovConfig{
		Symbol:           "BTC/USD",
		Gamma:            1000,
		K:                10000,
		T:                3600,
		QuoteQty:         exchange.BTC_PRECISION / 10,
		MaxInventory:     5 * exchange.BTC_PRECISION,
		VolatilityWindow: 20,
		RequoteInterval:  10 * time.Second,
	}

	asActor := NewAvellanedaStoikov(1, gateway, config)

	asActor.activeBidID = 123
	asActor.activeAskID = 456

	asActor.onOrderCancelRejected(actor.OrderCancelRejectedEvent{
		OrderID: 123,
		Reason:  exchange.RejectOrderNotFound,
	})

	if asActor.activeBidID != 0 {
		t.Error("activeBidID should be cleared after cancel rejection")
	}

	if asActor.activeAskID != 456 {
		t.Error("activeAskID should not be affected by bid cancel rejection")
	}
}

func TestAvellanedaStoikovRequestIDMatching(t *testing.T) {
	ex := exchange.NewExchange(100, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION, exchange.DOLLAR_TICK, exchange.BTC_PRECISION/100)
	ex.AddInstrument(instrument)

	balances := map[string]int64{
		"BTC": 100 * exchange.BTC_PRECISION,
		"USD": 1000000 * exchange.USD_PRECISION,
	}
	feePlan := &exchange.PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true}
	gateway := ex.ConnectClient(1, balances, feePlan)

	config := AvellanedaStoikovConfig{
		Symbol:           "BTC/USD",
		Gamma:            1000,
		K:                10000,
		T:                3600,
		QuoteQty:         exchange.BTC_PRECISION / 10,
		MaxInventory:     5 * exchange.BTC_PRECISION,
		VolatilityWindow: 20,
		RequoteInterval:  10 * time.Second,
	}

	asActor := NewAvellanedaStoikov(1, gateway, config)

	asActor.lastBidReqID = 101
	asActor.lastAskReqID = 102

	asActor.onOrderAccepted(actor.OrderAcceptedEvent{
		OrderID:   201,
		RequestID: 101,
	})

	if asActor.activeBidID != 201 {
		t.Errorf("Expected activeBidID=201, got %d", asActor.activeBidID)
	}

	if asActor.lastBidReqID != 0 {
		t.Errorf("lastBidReqID should be cleared after acceptance, got %d", asActor.lastBidReqID)
	}

	asActor.onOrderAccepted(actor.OrderAcceptedEvent{
		OrderID:   202,
		RequestID: 102,
	})

	if asActor.activeAskID != 202 {
		t.Errorf("Expected activeAskID=202, got %d", asActor.activeAskID)
	}

	if asActor.lastAskReqID != 0 {
		t.Errorf("lastAskReqID should be cleared after acceptance, got %d", asActor.lastAskReqID)
	}

	asActor.lastBidReqID = 103
	asActor.onOrderAccepted(actor.OrderAcceptedEvent{
		OrderID:   203,
		RequestID: 999, // Wrong ID
	})

	if asActor.activeBidID != 201 {
		t.Error("activeBidID should not change with wrong request ID")
	}

	if asActor.lastBidReqID != 103 {
		t.Error("lastBidReqID should not be cleared with wrong request ID")
	}
}
