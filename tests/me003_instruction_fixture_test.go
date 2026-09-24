package exchange_test

import (
	"testing"

	. "exchange_sim/exchange"
)

func TestME003StaleLocalAskCannotForceExecutionAboveLimit(t *testing.T) {
	for _, timeInForce := range []TimeInForce{IOC, FOK} {
		t.Run(timeInForce.String(), func(t *testing.T) {
			ex := newIOCExchange()
			defer ex.Shutdown()
			balances := map[string]int64{"BTC": BTCAmount(10), "USD": USDAmount(1_000_000)}
			ex.ConnectNewClient(1, balances, &FixedFee{})
			ex.ConnectNewClient(2, balances, &FixedFee{})
			localObservedAsk := PriceUSD(50_000, DOLLAR_TICK)
			initial := ex.PlaceOrder(1, &OrderRequest{RequestID: 1, Symbol: "BTC/USD",
				Side: Sell, Type: LimitOrder, Price: localObservedAsk,
				Qty: BTCAmount(1), TimeInForce: GTC})
			if !initial.Success {
				t.Fatalf("seed ask rejected: %s", initial.Error)
			}
			if cancelled := ex.CancelOrder(1, &CancelRequest{RequestID: 2,
				OrderID: initial.Data.(uint64)}); !cancelled.Success {
				t.Fatalf("old ask could not be removed: %s", cancelled.Error)
			}
			arrivalAsk := localObservedAsk + DOLLAR_TICK
			replacement := ex.PlaceOrder(1, &OrderRequest{RequestID: 3, Symbol: "BTC/USD",
				Side: Sell, Type: LimitOrder, Price: arrivalAsk,
				Qty: BTCAmount(1), TimeInForce: GTC})
			if !replacement.Success {
				t.Fatalf("replacement ask rejected: %s", replacement.Error)
			}
			beforeBTC := ex.Clients[2].Balances["BTC"]
			beforeUSD := ex.Clients[2].Balances["USD"]
			response := ex.PlaceOrder(2, &OrderRequest{RequestID: 4, Symbol: "BTC/USD",
				Side: Buy, Type: LimitOrder, Price: localObservedAsk,
				Qty: BTCAmount(1), TimeInForce: timeInForce})
			if timeInForce == IOC && !response.Success {
				t.Fatalf("unfilled IOC should be admitted: %s", response.Error)
			}
			if timeInForce == FOK && (response.Success || response.Error != RejectFOKNotFilled) {
				t.Fatalf("FOK should reject after venue ask reprices: %+v", response)
			}
			if ex.Clients[2].Balances["BTC"] != beforeBTC || ex.Clients[2].Balances["USD"] != beforeUSD ||
				ex.Clients[2].Reserved["USD"] != 0 {
				t.Fatal("stale local quote produced above-cap fill or leaked reservation")
			}
			ask := ex.Books["BTC/USD"].Asks.Best
			if ask == nil || ask.Price != arrivalAsk || ask.TotalQty != BTCAmount(1) {
				t.Fatalf("arrival book was unexpectedly consumed: %+v", ask)
			}
		})
	}
}
