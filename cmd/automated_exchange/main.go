package main

import (
	"context"
	"fmt"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// SimpleMarketMaker places orders around mid price
type SimpleMarketMaker struct {
	*actor.BaseActor
	symbol string
	spread int64
	size   int64
}

func NewSimpleMarketMaker(id uint64, gateway *exchange.ClientGateway, symbol string, spread, size int64) *SimpleMarketMaker {
	return &SimpleMarketMaker{
		BaseActor: actor.NewBaseActor(id, gateway),
		symbol:    symbol,
		spread:    spread,
		size:      size,
	}
}

func (mm *SimpleMarketMaker) OnEvent(event *actor.Event) {
	switch event.Type {
	case actor.EventBookSnapshot:
		snap := event.Data.(actor.BookSnapshotEvent)
		if snap.Symbol != mm.symbol {
			return
		}

		if len(snap.Snapshot.Bids) > 0 && len(snap.Snapshot.Asks) > 0 {
			bestBid := snap.Snapshot.Bids[0].Price
			bestAsk := snap.Snapshot.Asks[0].Price
			mid := (bestBid + bestAsk) / 2

			// Place orders around mid
			mm.SubmitOrder(mm.symbol, exchange.Buy, exchange.LimitOrder, mid-mm.spread, mm.size)
			mm.SubmitOrder(mm.symbol, exchange.Sell, exchange.LimitOrder, mid+mm.spread, mm.size)
		}

	case actor.EventFundingUpdate:
		funding := event.Data.(actor.FundingUpdateEvent)
		fmt.Printf("[MM] Funding update for %s: Rate=%d bps, Mark=%d, Index=%d\n",
			funding.Symbol,
			funding.FundingRate.Rate,
			funding.FundingRate.MarkPrice,
			funding.FundingRate.IndexPrice)
	}
}

func main() {
	fmt.Println("=== Industry-Standard Automated Exchange Demo ===")

	// Create exchange
	ex := exchange.NewExchange(10, &exchange.RealClock{})

	// Add spot instrument (for index price)
	spotInst := exchange.NewSpotInstrument(
		"BTC/USD",
		"BTC",
		"USD",
		exchange.SATOSHI,      // basePrecision
		exchange.SATOSHI/1000, // quotePrecision
		exchange.DOLLAR_TICK,  // tickSize
		exchange.SATOSHI/100,  // minOrderSize
	)
	ex.AddInstrument(spotInst)

	// Add perpetual futures
	perpInst := exchange.NewPerpFutures(
		"BTC-PERP",
		"BTC",
		"USD",
		exchange.SATOSHI,      // basePrecision
		exchange.SATOSHI/1000, // quotePrecision
		exchange.DOLLAR_TICK,  // tickSize
		exchange.SATOSHI/100,  // minOrderSize
	)
	ex.AddInstrument(perpInst)

	// Create index provider (spot price as index)
	indexProvider := exchange.NewSpotIndexProvider(ex)
	indexProvider.MapPerpToSpot("BTC-PERP", "BTC/USD")

	// Create automation with industry-standard config
	automation := exchange.NewExchangeAutomation(ex, exchange.AutomationConfig{
		MarkPriceCalc:       exchange.NewMidPriceCalculator(),
		IndexProvider:       indexProvider,
		PriceUpdateInterval: 3 * time.Second,
	})

	// Start automatic operations
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	automation.Start(ctx)
	fmt.Println("✓ Automated exchange started")
	fmt.Println("  - Mark prices auto-calculated from order book")
	fmt.Println("  - Index prices auto-calculated from spot")
	fmt.Println("  - Funding rates auto-updated every 3 seconds")
	fmt.Println("  - Funding settlement auto-scheduled every 8 hours")

	// Create client balances
	balances := map[string]int64{
		"BTC": 10 * exchange.SATOSHI,
		"USD": 1000000 * (exchange.SATOSHI / 1000),
	}

	// Connect market makers for spot (creates liquidity for index)
	spotMM1 := ex.ConnectClient(1, balances, &exchange.FixedFee{})
	spotActor1 := NewSimpleMarketMaker(1, spotMM1, "BTC/USD", exchange.DOLLAR_TICK*10, exchange.SATOSHI/10)
	spotActor1.Start(ctx)

	// Subscribe to market data
	ex.MDPublisher.Subscribe(1, "BTC/USD", []exchange.MDType{
		exchange.MDSnapshot,
		exchange.MDTrade,
	}, spotMM1)

	// Connect market makers for perp
	perpMM1 := ex.ConnectClient(2, balances, &exchange.FixedFee{})
	perpActor1 := NewSimpleMarketMaker(2, perpMM1, "BTC-PERP", exchange.DOLLAR_TICK*10, exchange.SATOSHI/10)
	perpActor1.Start(ctx)

	// Subscribe to funding updates
	ex.MDPublisher.Subscribe(2, "BTC-PERP", []exchange.MDType{
		exchange.MDSnapshot,
		exchange.MDTrade,
		exchange.MDFunding,
	}, perpMM1)

	// Place initial orders to establish market
	fmt.Println("Placing initial orders to establish market...")

	// Spot market around $50,000
	spotMM1.RequestCh <- exchange.Request{
		Type: exchange.ReqPlaceOrder,
		OrderReq: &exchange.OrderRequest{
			RequestID:   1,
			Symbol:      "BTC/USD",
			Side:        exchange.Buy,
			Type:        exchange.LimitOrder,
			Price:       exchange.PriceUSD(49950, exchange.DOLLAR_TICK),
			Qty:         exchange.SATOSHI,
			TimeInForce: exchange.GTC,
			Visibility:  exchange.Normal,
		},
	}

	spotMM1.RequestCh <- exchange.Request{
		Type: exchange.ReqPlaceOrder,
		OrderReq: &exchange.OrderRequest{
			RequestID:   2,
			Symbol:      "BTC/USD",
			Side:        exchange.Sell,
			Type:        exchange.LimitOrder,
			Price:       exchange.PriceUSD(50050, exchange.DOLLAR_TICK),
			Qty:         exchange.SATOSHI,
			TimeInForce: exchange.GTC,
			Visibility:  exchange.Normal,
		},
	}

	// Perp market with premium (bullish)
	perpMM1.RequestCh <- exchange.Request{
		Type: exchange.ReqPlaceOrder,
		OrderReq: &exchange.OrderRequest{
			RequestID:   3,
			Symbol:      "BTC-PERP",
			Side:        exchange.Buy,
			Type:        exchange.LimitOrder,
			Price:       exchange.PriceUSD(50050, exchange.DOLLAR_TICK), // Premium to spot
			Qty:         exchange.SATOSHI,
			TimeInForce: exchange.GTC,
			Visibility:  exchange.Normal,
		},
	}

	perpMM1.RequestCh <- exchange.Request{
		Type: exchange.ReqPlaceOrder,
		OrderReq: &exchange.OrderRequest{
			RequestID:   4,
			Symbol:      "BTC-PERP",
			Side:        exchange.Sell,
			Type:        exchange.LimitOrder,
			Price:       exchange.PriceUSD(50150, exchange.DOLLAR_TICK), // Premium to spot
			Qty:         exchange.SATOSHI,
			TimeInForce: exchange.GTC,
			Visibility:  exchange.Normal,
		},
	}

	// Let the automation run
	fmt.Println("Running automated exchange for 15 seconds...")
	fmt.Println("Watch for automatic funding rate updates every 3 seconds!")

	time.Sleep(15 * time.Second)

	// Check final funding rate
	finalRate := perpInst.GetFundingRate()
	fmt.Printf("\n=== Final State ===\n")
	fmt.Printf("Perpetual: %s\n", perpInst.Symbol())
	fmt.Printf("  Mark Price:  %d (%.2f USD)\n", finalRate.MarkPrice, float64(finalRate.MarkPrice)/float64(exchange.SATOSHI))
	fmt.Printf("  Index Price: %d (%.2f USD)\n", finalRate.IndexPrice, float64(finalRate.IndexPrice)/float64(exchange.SATOSHI))
	fmt.Printf("  Funding Rate: %d bps (%.4f%%)\n", finalRate.Rate, float64(finalRate.Rate)/100.0)
	fmt.Printf("  Next Settlement: %d\n", finalRate.NextFunding)
	fmt.Printf("  Interval: %d seconds\n", finalRate.Interval)

	premium := float64(finalRate.MarkPrice-finalRate.IndexPrice) / float64(finalRate.IndexPrice) * 100
	fmt.Printf("  Premium: %.4f%%\n", premium)

	// Cleanup
	fmt.Println("\n✓ Shutting down automated exchange...")
	cancel()
	automation.Stop()
	fmt.Println("✓ Done!")
}
