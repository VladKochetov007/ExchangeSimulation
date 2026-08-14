package exchange

import (
	"testing"
	"time"
)

// reportsPartialAfterMatching simulates a third-party matcher whose declared
// FOK result disagrees with the liquidity it has already consumed. The
// exchange must reject this during detached preflight, before it can affect
// the live book or either account's reservation ledger.
type reportsPartialAfterMatching struct{ MatchingEngine }

func (m reportsPartialAfterMatching) Match(bids, asks *Book, order *Order) *MatchResult {
	result := m.MatchingEngine.Match(bids, asks, order)
	if result != nil {
		result.FullyFilled = false
	}
	return result
}

func TestFOKMatcherMismatchRejectsBeforeLiveMutation(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	defer ex.Shutdown()
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
	ex.ConnectNewClient(1, map[string]int64{"USD": USDAmount(100_000)}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{"BTC": BTCAmount(1)}, &FixedFee{})

	price := PriceUSD(50_000, DOLLAR_TICK)
	if response := ex.PlaceOrder(2, &OrderRequest{
		RequestID: 1, Symbol: "BTC/USD", Side: Sell, Type: LimitOrder,
		Price: price, Qty: BTC_PRECISION, TimeInForce: GTC, Visibility: Normal,
	}); !response.Success {
		t.Fatalf("seed ask rejected: %s", response.Error)
	}
	ex.Matcher = reportsPartialAfterMatching{MatchingEngine: ex.Matcher}

	response := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 2, Symbol: "BTC/USD", Side: Buy, Type: LimitOrder,
		Price: price, Qty: BTC_PRECISION, TimeInForce: FOK, Visibility: Normal,
	})
	if response.Success || response.Error != RejectFOKNotFilled {
		t.Fatalf("FOK response = %+v, want preflight rejection", response)
	}

	ask := ex.Books["BTC/USD"].Asks.Best
	if ask == nil || ask.TotalQty != BTC_PRECISION {
		t.Fatalf("FOK preflight mutated live ask: %#v", ask)
	}
	if got := ex.Clients[1].Balances["USD"]; got != USDAmount(100_000) {
		t.Fatalf("FOK preflight changed buyer USD: got %d", got)
	}
	if got := ex.Clients[1].Reserved["USD"]; got != 0 {
		t.Fatalf("FOK preflight leaked buyer reservation: got %d", got)
	}
}

func TestDiscardedImmediateOrderRemainderIsCancelledBeforeAcceptance(t *testing.T) {
	tests := []struct {
		name        string
		orderType   OrderType
		timeInForce TimeInForce
		seedAskQty  int64
		wantFillQty int64
		wantRemain  int64
	}{
		{
			name:        "partial market order",
			orderType:   Market,
			timeInForce: GTC,
			seedAskQty:  BTC_PRECISION / 4,
			wantFillQty: BTC_PRECISION / 4,
			wantRemain:  BTC_PRECISION - BTC_PRECISION/4,
		},
		{
			name:        "zero fill market order",
			orderType:   Market,
			timeInForce: GTC,
			wantRemain:  BTC_PRECISION,
		},
		{
			name:        "partial IOC limit order",
			orderType:   LimitOrder,
			timeInForce: IOC,
			seedAskQty:  BTC_PRECISION / 4,
			wantFillQty: BTC_PRECISION / 4,
			wantRemain:  BTC_PRECISION - BTC_PRECISION/4,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ex := NewExchange(2, &RealClock{})
			defer ex.Shutdown()
			ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
			ex.ConnectNewClient(1, map[string]int64{"USD": USDAmount(100_000)}, &FixedFee{})
			ex.ConnectNewClient(2, map[string]int64{"BTC": BTCAmount(1)}, &FixedFee{})

			if tc.seedAskQty > 0 {
				resp := ex.PlaceOrder(2, &OrderRequest{
					RequestID:   1,
					Symbol:      "BTC/USD",
					Side:        Sell,
					Type:        LimitOrder,
					Price:       PriceUSD(50_000, DOLLAR_TICK),
					Qty:         tc.seedAskQty,
					TimeInForce: GTC,
					Visibility:  Normal,
				})
				if !resp.Success {
					t.Fatalf("seed ask rejected: %s", resp.Error)
				}
			}

			const requestID = uint64(17)
			gateway := ex.Gateways[1]
			gateway.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: &OrderRequest{
				RequestID:   requestID,
				Symbol:      "BTC/USD",
				Side:        Buy,
				Type:        tc.orderType,
				Price:       PriceUSD(50_000, DOLLAR_TICK),
				Qty:         BTC_PRECISION,
				TimeInForce: tc.timeInForce,
				Visibility:  Normal,
			}}

			expectedResponses := 2
			if tc.wantFillQty > 0 {
				expectedResponses++
			}
			var responses []Response
			deadline := time.After(time.Second)
			for len(responses) < expectedResponses {
				select {
				case response := <-gateway.ResponseCh:
					responses = append(responses, response)
				case <-deadline:
					t.Fatalf("received %d responses, want %d: %#v", len(responses), expectedResponses, responses)
				}
			}

			index := 0
			if tc.wantFillQty > 0 {
				fill, ok := responses[index].Data.(*FillNotification)
				if !ok {
					t.Fatalf("response %d = %T, want fill notification", index, responses[index].Data)
				}
				if fill.Qty != tc.wantFillQty || fill.IsFull {
					t.Fatalf("fill = %#v, want partial qty %d", fill, tc.wantFillQty)
				}
				index++
			}

			cancel, ok := responses[index].Data.(*ForcedCancelNotification)
			if !ok {
				t.Fatalf("response %d = %T, want forced cancellation", index, responses[index].Data)
			}
			if cancel.RemainingQty != tc.wantRemain {
				t.Fatalf("cancel remainder = %d, want %d", cancel.RemainingQty, tc.wantRemain)
			}
			index++

			accept := responses[index]
			if accept.RequestID != requestID || !accept.Success {
				t.Fatalf("acceptance = %#v, want successful response for request %d", accept, requestID)
			}
			orderID, ok := accept.Data.(uint64)
			if !ok || cancel.OrderID != orderID {
				t.Fatalf("cancel order %d, acceptance %#v", cancel.OrderID, accept.Data)
			}
		})
	}
}

func TestFullyFilledImmediateOrderDoesNotEmitForcedCancellation(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	defer ex.Shutdown()
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
	ex.ConnectNewClient(1, map[string]int64{"USD": USDAmount(100_000)}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{"BTC": BTCAmount(1)}, &FixedFee{})

	if resp := ex.PlaceOrder(2, &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Sell,
		Type:        LimitOrder,
		Price:       PriceUSD(50_000, DOLLAR_TICK),
		Qty:         BTC_PRECISION,
		TimeInForce: GTC,
		Visibility:  Normal,
	}); !resp.Success {
		t.Fatalf("seed ask rejected: %s", resp.Error)
	}

	const requestID = uint64(19)
	gateway := ex.Gateways[1]
	gateway.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: &OrderRequest{
		RequestID:   requestID,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        Market,
		Qty:         BTC_PRECISION,
		TimeInForce: GTC,
		Visibility:  Normal,
	}}

	var responses [2]Response
	for index := range responses {
		select {
		case responses[index] = <-gateway.ResponseCh:
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for response %d", index)
		}
	}
	if fill, ok := responses[0].Data.(*FillNotification); !ok || !fill.IsFull {
		t.Fatalf("response 0 = %#v, want full fill", responses[0])
	}
	if responses[1].RequestID != requestID || !responses[1].Success {
		t.Fatalf("response 1 = %#v, want successful acceptance", responses[1])
	}
	select {
	case response := <-gateway.ResponseCh:
		t.Fatalf("unexpected response after full fill: %#v", response)
	case <-time.After(20 * time.Millisecond):
	}
}
