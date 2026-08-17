package exchange_test

import (
	"testing"

	. "exchange_sim/exchange"
)

func newIOCExchange() *Exchange {
	ex := NewExchange(10, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/1000))
	return ex
}

// An IOC buy priced at or above the resting best ask must trade. A metaorder
// desk submitted 899 such children over 900 simulated seconds and filled none,
// while the trade log showed a print at or below its own limit price in every
// one of those seconds. Second-resolution timestamps cannot prove the liquidity
// was there at the matching instant, so this pins the engine's behaviour
// directly.
func TestIOCBuyCrossesRestingAsk(t *testing.T) {
	restingPrice := PriceUSD(50000, DOLLAR_TICK)
	for _, tc := range []struct {
		name      string
		takerBump int64
	}{
		{"exactly at the ask", 0},
		{"one tick through the ask", DOLLAR_TICK},
		{"far through the ask", 10 * DOLLAR_TICK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ex := newIOCExchange()
			balances := map[string]int64{"BTC": BTCAmount(10), "USD": USDAmount(1_000_000)}
			ex.ConnectNewClient(1, balances, &FixedFee{})
			ex.ConnectNewClient(2, balances, &FixedFee{})

			maker := &OrderRequest{
				RequestID: 1, Symbol: "BTC/USD", Side: Sell, Type: LimitOrder,
				Price: restingPrice, Qty: BTCAmount(2), TimeInForce: GTC,
			}
			makerResp := ex.PlaceOrder(1, maker)
			if !makerResp.Success {
				t.Fatalf("resting sell rejected: %v", makerResp.Error)
			}
			makerID := makerResp.Data.(uint64)

			taker := &OrderRequest{
				RequestID: 2, Symbol: "BTC/USD", Side: Buy, Type: LimitOrder,
				Price: restingPrice + tc.takerBump, Qty: BTCAmount(1), TimeInForce: IOC,
			}
			resp := ex.PlaceOrder(2, taker)
			if !resp.Success {
				t.Fatalf("IOC buy rejected: %v", resp.Error)
			}

			restingOrder := ex.Books["BTC/USD"].Asks.Orders[makerID]
			if restingOrder == nil {
				return // fully consumed, which is a fill
			}
			if restingOrder.FilledQty != BTCAmount(1) {
				t.Fatalf("IOC buy at %d against ask %d filled %d, want %d",
					taker.Price, restingPrice, restingOrder.FilledQty, BTCAmount(1))
			}
		})
	}
}

// The mirror case, since every stall observed in an unthrottled run was a buy
// and the sell path was never exercised by one.
func TestIOCSellCrossesRestingBid(t *testing.T) {
	ex := newIOCExchange()
	balances := map[string]int64{"BTC": BTCAmount(10), "USD": USDAmount(1_000_000)}
	ex.ConnectNewClient(1, balances, &FixedFee{})
	ex.ConnectNewClient(2, balances, &FixedFee{})

	restingPrice := PriceUSD(50000, DOLLAR_TICK)
	maker := &OrderRequest{
		RequestID: 1, Symbol: "BTC/USD", Side: Buy, Type: LimitOrder,
		Price: restingPrice, Qty: BTCAmount(2), TimeInForce: GTC,
	}
	makerResp := ex.PlaceOrder(1, maker)
	if !makerResp.Success {
		t.Fatalf("resting buy rejected: %v", makerResp.Error)
	}
	makerID := makerResp.Data.(uint64)
	taker := &OrderRequest{
		RequestID: 2, Symbol: "BTC/USD", Side: Sell, Type: LimitOrder,
		Price: restingPrice, Qty: BTCAmount(1), TimeInForce: IOC,
	}
	if resp := ex.PlaceOrder(2, taker); !resp.Success {
		t.Fatalf("IOC sell rejected: %v", resp.Error)
	}
	restingOrder := ex.Books["BTC/USD"].Bids.Orders[makerID]
	if restingOrder == nil {
		return
	}
	if restingOrder.FilledQty != BTCAmount(1) {
		t.Fatalf("IOC sell at %d against bid %d filled %d, want %d",
			taker.Price, restingPrice, restingOrder.FilledQty, BTCAmount(1))
	}
}
