package simulation

import (
	"exchange_sim/exchange"
	"testing"
	"time"
)

func TestVenueRegistry(t *testing.T) {
	registry := NewVenueRegistry()

	ex1 := exchange.NewExchange(10, &RealClock{})
	ex2 := exchange.NewExchange(10, &RealClock{})

	registry.Register("binance", ex1)
	registry.Register("coinbase", ex2)

	if registry.Get("binance") != ex1 {
		t.Fatal("Registry should return correct exchange for binance")
	}
	if registry.Get("coinbase") != ex2 {
		t.Fatal("Registry should return correct exchange for coinbase")
	}
	if registry.Get("unknown") != nil {
		t.Fatal("Registry should return nil for unknown venue")
	}

	venues := registry.ListVenues()
	if len(venues) != 2 {
		t.Fatalf("Expected 2 venues, got %d", len(venues))
	}
}

func TestMultiVenueGatewayCreation(t *testing.T) {
	registry := NewVenueRegistry()

	ex1 := exchange.NewExchange(10, &RealClock{})
	instrument1 := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", 100000000, 1000000)
	ex1.AddInstrument(instrument1)

	ex2 := exchange.NewExchange(10, &RealClock{})
	instrument2 := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", 100000000, 1000000)
	ex2.AddInstrument(instrument2)

	registry.Register("binance", ex1)
	registry.Register("coinbase", ex2)

	initialBalances := map[VenueID]map[string]int64{
		"binance":  {"BTC": 1000000000, "USD": 100000000000000},
		"coinbase": {"BTC": 2000000000, "USD": 200000000000000},
	}

	feePlans := map[VenueID]exchange.FeeModel{
		"binance":  &exchange.FixedFee{},
		"coinbase": &exchange.FixedFee{},
	}

	mgw := NewMultiVenueGateway(1, registry, initialBalances, feePlans)
	mgw.Start()
	defer mgw.Stop()

	if mgw.GetGateway("binance") == nil {
		t.Fatal("Should have gateway for binance")
	}
	if mgw.GetGateway("coinbase") == nil {
		t.Fatal("Should have gateway for coinbase")
	}
	if mgw.GetGateway("unknown") != nil {
		t.Fatal("Should not have gateway for unknown venue")
	}
}

func TestMultiVenueGatewayOrderRouting(t *testing.T) {
	registry := NewVenueRegistry()

	ex1 := exchange.NewExchange(10, &RealClock{})
	instrument1 := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", 100000000, 1000000)
	ex1.AddInstrument(instrument1)

	ex2 := exchange.NewExchange(10, &RealClock{})
	instrument2 := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", 100000000, 1000000)
	ex2.AddInstrument(instrument2)

	registry.Register("binance", ex1)
	registry.Register("coinbase", ex2)

	initialBalances := map[VenueID]map[string]int64{
		"binance":  {"BTC": 1000000000, "USD": 100000000000000},
		"coinbase": {"BTC": 2000000000, "USD": 200000000000000},
	}

	feePlans := map[VenueID]exchange.FeeModel{
		"binance":  &exchange.FixedFee{},
		"coinbase": &exchange.FixedFee{},
	}

	mgw := NewMultiVenueGateway(1, registry, initialBalances, feePlans)
	mgw.Start()
	defer mgw.Stop()

	mgw.SubmitOrder("binance", &exchange.OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        exchange.Buy,
		Type:        exchange.LimitOrder,
		Price:       5000000000000,
		Qty:         100000000,
		TimeInForce: exchange.GTC,
	})

	select {
	case vResp := <-mgw.ResponseCh():
		if vResp.Venue != "binance" {
			t.Fatalf("Expected response from binance, got %s", vResp.Venue)
		}
		if !vResp.Response.Success {
			t.Fatalf("Order should succeed, got error %v", vResp.Response.Error)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for response from binance")
	}

	mgw.SubmitOrder("coinbase", &exchange.OrderRequest{
		RequestID:   2,
		Symbol:      "BTC/USD",
		Side:        exchange.Sell,
		Type:        exchange.LimitOrder,
		Price:       5100000000000,
		Qty:         100000000,
		TimeInForce: exchange.GTC,
	})

	select {
	case vResp := <-mgw.ResponseCh():
		if vResp.Venue != "coinbase" {
			t.Fatalf("Expected response from coinbase, got %s", vResp.Venue)
		}
		if !vResp.Response.Success {
			t.Fatalf("Order should succeed, got error %v", vResp.Response.Error)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for response from coinbase")
	}
}

func TestMultiVenueGatewayMarketData(t *testing.T) {
	registry := NewVenueRegistry()

	ex1 := exchange.NewExchange(10, &RealClock{})
	instrument1 := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", 100000000, 1000000)
	ex1.AddInstrument(instrument1)

	ex2 := exchange.NewExchange(10, &RealClock{})
	instrument2 := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", 100000000, 1000000)
	ex2.AddInstrument(instrument2)

	registry.Register("binance", ex1)
	registry.Register("coinbase", ex2)

	initialBalances := map[VenueID]map[string]int64{
		"binance":  {"BTC": 1000000000, "USD": 100000000000000},
		"coinbase": {"BTC": 2000000000, "USD": 200000000000000},
	}

	feePlans := map[VenueID]exchange.FeeModel{
		"binance":  &exchange.FixedFee{},
		"coinbase": &exchange.FixedFee{},
	}

	mgw := NewMultiVenueGateway(1, registry, initialBalances, feePlans)
	mgw.Start()
	defer mgw.Stop()

	mgw.Subscribe("binance", "BTC/USD")

	select {
	case vResp := <-mgw.ResponseCh():
		if !vResp.Response.Success {
			t.Fatalf("Subscribe should succeed, got error %v", vResp.Response.Error)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for subscribe response")
	}

	mgw.SubmitOrder("binance", &exchange.OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        exchange.Sell,
		Type:        exchange.LimitOrder,
		Price:       5000000000000,
		Qty:         100000000,
		TimeInForce: exchange.GTC,
	})

	<-mgw.ResponseCh()

	receivedBinanceData := false
	timeout := time.After(1 * time.Second)
	for !receivedBinanceData {
		select {
		case vmd := <-mgw.MarketDataCh():
			if vmd.Venue == "binance" {
				receivedBinanceData = true
			}
		case <-timeout:
			t.Fatal("Timeout waiting for market data from binance")
		}
	}
}

func TestMultiVenueGatewayArbitrage(t *testing.T) {
	registry := NewVenueRegistry()

	ex1 := exchange.NewExchange(10, &RealClock{})
	instrument1 := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", 100000000, 1000000)
	ex1.AddInstrument(instrument1)

	ex2 := exchange.NewExchange(10, &RealClock{})
	instrument2 := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", 100000000, 1000000)
	ex2.AddInstrument(instrument2)

	registry.Register("binance", ex1)
	registry.Register("coinbase", ex2)

	initialBalances := map[VenueID]map[string]int64{
		"binance":  {"BTC": 1000000000, "USD": 100000000000000},
		"coinbase": {"BTC": 1000000000, "USD": 100000000000000},
	}

	feePlans := map[VenueID]exchange.FeeModel{
		"binance":  &exchange.FixedFee{},
		"coinbase": &exchange.FixedFee{},
	}

	mgw := NewMultiVenueGateway(1, registry, initialBalances, feePlans)
	mgw.Start()
	defer mgw.Stop()

	mgw.SubmitOrder("binance", &exchange.OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        exchange.Buy,
		Type:        exchange.LimitOrder,
		Price:       5000000000000,
		Qty:         100000000,
		TimeInForce: exchange.GTC,
	})

	binanceResp := <-mgw.ResponseCh()
	if binanceResp.Venue != "binance" {
		t.Fatalf("Expected response from binance, got %s", binanceResp.Venue)
	}
	if !binanceResp.Response.Success {
		t.Fatalf("Binance order should succeed, got error %v", binanceResp.Response.Error)
	}

	mgw.SubmitOrder("coinbase", &exchange.OrderRequest{
		RequestID:   2,
		Symbol:      "BTC/USD",
		Side:        exchange.Sell,
		Type:        exchange.LimitOrder,
		Price:       5100000000000,
		Qty:         100000000,
		TimeInForce: exchange.GTC,
	})

	coinbaseResp := <-mgw.ResponseCh()
	if coinbaseResp.Venue != "coinbase" {
		t.Fatalf("Expected response from coinbase, got %s", coinbaseResp.Venue)
	}
	if !coinbaseResp.Response.Success {
		t.Fatalf("Coinbase order should succeed, got error %v", coinbaseResp.Response.Error)
	}
}
