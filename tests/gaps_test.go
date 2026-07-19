package exchange_test

import (
	. "exchange_sim/exchange"
	"testing"
)

func newGapsExchange() *Exchange {
	ex := NewExchange(10, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/1000))
	return ex
}

// A client whose new limit order crosses only its own resting order must not
// leave the book internally crossed. Every real venue policy (reject, expire
// taker, expire maker, or allow the self-trade) ends with best bid < best ask;
// resting the crossing remainder is not a valid outcome under any of them.
func TestSelfCrossingLimitDoesNotCrossBook(t *testing.T) {
	ex := newGapsExchange()
	ex.ConnectNewClient(1, map[string]int64{"BTC": BTCAmount(10), "USD": USDAmount(1_000_000)}, &FixedFee{})

	sell := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Sell,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, DOLLAR_TICK),
		Qty:         BTCAmount(0.5),
		TimeInForce: GTC,
	}
	if resp := ex.PlaceOrder(1, sell); !resp.Success {
		t.Fatalf("resting sell rejected: %v", resp.Error)
	}

	buy := &OrderRequest{
		RequestID:   2,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        LimitOrder,
		Price:       PriceUSD(51000, DOLLAR_TICK),
		Qty:         BTCAmount(0.5),
		TimeInForce: GTC,
	}
	ex.PlaceOrder(1, buy)

	book := ex.Books["BTC/USD"]
	if book.Bids.Best != nil && book.Asks.Best != nil && book.Bids.Best.Price >= book.Asks.Best.Price {
		t.Fatalf("book internally crossed after self-cross: best bid %d >= best ask %d",
			book.Bids.Best.Price, book.Asks.Best.Price)
	}
}

// Only the displayed tranche of an iceberg holds time priority. When a taker
// consumes the display, the next resting order at the level fills before the
// iceberg's hidden reserve refills (behind the queue).
func TestIcebergReserveDoesNotJumpQueue(t *testing.T) {
	ex := newGapsExchange()
	balances := map[string]int64{"BTC": BTCAmount(10), "USD": USDAmount(200_000)}
	ex.ConnectNewClient(1, balances, &FixedFee{})
	ex.ConnectNewClient(2, balances, &FixedFee{})
	ex.ConnectNewClient(3, balances, &FixedFee{})

	price := PriceUSD(50000, DOLLAR_TICK)
	iceberg := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Sell,
		Type:        LimitOrder,
		Price:       price,
		Qty:         BTCAmount(2),
		TimeInForce: GTC,
		Visibility:  Iceberg,
		IcebergQty:  BTCAmount(0.5),
	}
	respA := ex.PlaceOrder(1, iceberg)
	if !respA.Success {
		t.Fatalf("iceberg sell rejected: %v", respA.Error)
	}

	normal := &OrderRequest{
		RequestID:   2,
		Symbol:      "BTC/USD",
		Side:        Sell,
		Type:        LimitOrder,
		Price:       price,
		Qty:         BTCAmount(1),
		TimeInForce: GTC,
	}
	respB := ex.PlaceOrder(2, normal)
	if !respB.Success {
		t.Fatalf("normal sell rejected: %v", respB.Error)
	}

	taker := &OrderRequest{
		RequestID:   3,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        Market,
		Qty:         BTCAmount(1),
		TimeInForce: IOC,
	}
	if resp := ex.PlaceOrder(3, taker); !resp.Success {
		t.Fatalf("market buy rejected: %v", resp.Error)
	}

	asks := ex.Books["BTC/USD"].Asks
	icebergOrder := asks.Orders[respA.Data.(uint64)]
	normalOrder := asks.Orders[respB.Data.(uint64)]
	if icebergOrder == nil || normalOrder == nil {
		t.Fatalf("both makers should still rest: iceberg=%v normal=%v", icebergOrder, normalOrder)
	}
	if icebergOrder.FilledQty != BTCAmount(0.5) || normalOrder.FilledQty != BTCAmount(0.5) {
		t.Fatalf("iceberg reserve jumped the queue: iceberg filled %d (want %d), normal filled %d (want %d)",
			icebergOrder.FilledQty, BTCAmount(0.5), normalOrder.FilledQty, BTCAmount(0.5))
	}
}

// Hidden orders must produce no public market data. Broadcasting a delta on
// placement — even with VisibleQty 0 — reveals the exact price where dark
// liquidity arrived.
func TestHiddenOrderEmitsNoPublicDelta(t *testing.T) {
	ex := newGapsExchange()
	balances := map[string]int64{"BTC": BTCAmount(10), "USD": USDAmount(200_000)}
	ex.ConnectNewClient(1, balances, &FixedFee{})
	ex.ConnectNewClient(2, balances, &FixedFee{})

	gw := ex.Gateways[1]
	sub := ex.Subscribe(1, &QueryRequest{RequestID: 1, Symbol: "BTC/USD", Types: []MDType{MDDelta}}, gw)
	if !sub.Success {
		t.Fatalf("subscribe failed: %v", sub.Error)
	}

	hiddenPrice := PriceUSD(49000, DOLLAR_TICK)
	hidden := &OrderRequest{
		RequestID:   2,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        LimitOrder,
		Price:       hiddenPrice,
		Qty:         BTCAmount(1),
		TimeInForce: GTC,
		Visibility:  Hidden,
	}
	if resp := ex.PlaceOrder(2, hidden); !resp.Success {
		t.Fatalf("hidden buy rejected: %v", resp.Error)
	}

	// Publish is synchronous inside PlaceOrder, so a non-blocking drain sees
	// everything that was broadcast.
	for {
		select {
		case msg := <-gw.MarketData:
			if msg.Type != MDDelta {
				continue
			}
			delta := msg.Data.(*BookDelta)
			if delta.Price == hiddenPrice {
				t.Fatalf("hidden order placement leaked a public delta at its price: %+v", delta)
			}
		default:
			return
		}
	}
}
