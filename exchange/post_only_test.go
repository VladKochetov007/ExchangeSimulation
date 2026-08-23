package exchange

import "testing"

func newPostOnlyTestExchange(t *testing.T) *DefaultExchange {
	t.Helper()
	ex := NewExchange(2, &RealClock{})
	t.Cleanup(ex.Shutdown)
	ex.AddInstrument(NewSpotInstrument("ABC/USD", "ABC", "USD", 1, 1, 1, 1))
	ex.ConnectNewClient(1, map[string]int64{"ABC": 10, "USD": 1_000}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{"ABC": 10, "USD": 1_000}, &FixedFee{})
	return ex
}

func TestPostOnlyCrossingOrderRejectsBeforeAdmissionMutation(t *testing.T) {
	tests := []struct {
		name     string
		seedSide Side
		postSide Side
	}{
		{name: "buy hits ask", seedSide: Sell, postSide: Buy},
		{name: "sell hits bid", seedSide: Buy, postSide: Sell},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ex := newPostOnlyTestExchange(t)
			if response := ex.PlaceOrder(2, &OrderRequest{
				RequestID: 1, Symbol: "ABC/USD", Side: tc.seedSide, Type: LimitOrder,
				Price: 100, Qty: 3, TimeInForce: GTC, Visibility: Normal,
			}); !response.Success {
				t.Fatalf("seed liquidity rejected: %+v", response)
			}
			client := ex.Clients[1]
			beforeID := ex.NextOrderID
			beforeBalances := map[string]int64{"ABC": client.Balances["ABC"], "USD": client.Balances["USD"]}
			beforeReserved := map[string]int64{"ABC": client.Reserved["ABC"], "USD": client.Reserved["USD"]}

			response := ex.PlaceOrder(1, &OrderRequest{
				RequestID: 2, Symbol: "ABC/USD", Side: tc.postSide, Type: LimitOrder,
				Price: 100, Qty: 2, TimeInForce: GTC, Visibility: Normal, PostOnly: true,
			})
			if response.Success || response.Error != RejectPostOnlyWouldTake {
				t.Fatalf("post-only crossing response = %+v, want POST_ONLY_WOULD_TAKE", response)
			}
			if ex.NextOrderID != beforeID {
				t.Fatalf("post-only rejection allocated order ID: got %d want %d", ex.NextOrderID, beforeID)
			}
			if client.Balances["ABC"] != beforeBalances["ABC"] || client.Balances["USD"] != beforeBalances["USD"] ||
				client.Reserved["ABC"] != beforeReserved["ABC"] || client.Reserved["USD"] != beforeReserved["USD"] || len(client.OrderIDs) != 0 {
				t.Fatalf("post-only rejection mutated client: balances=%v reserved=%v orders=%v", client.Balances, client.Reserved, client.OrderIDs)
			}
			book := ex.Books["ABC/USD"]
			if book.LastTrade != nil {
				t.Fatalf("post-only rejection matched: %+v", book.LastTrade)
			}
			if tc.postSide == Buy {
				if book.Asks.Best == nil || book.Asks.Best.Price != 100 || book.Asks.Best.TotalQty != 3 || book.Bids.Best != nil {
					t.Fatalf("post-only buy mutated book: bids=%#v asks=%#v", book.Bids.Best, book.Asks.Best)
				}
			} else {
				if book.Bids.Best == nil || book.Bids.Best.Price != 100 || book.Bids.Best.TotalQty != 3 || book.Asks.Best != nil {
					t.Fatalf("post-only sell mutated book: bids=%#v asks=%#v", book.Bids.Best, book.Asks.Best)
				}
			}
		})
	}
}

func TestPostOnlyNonCrossingOrderRestsAndRetainsContract(t *testing.T) {
	ex := newPostOnlyTestExchange(t)
	if response := ex.PlaceOrder(2, &OrderRequest{
		RequestID: 1, Symbol: "ABC/USD", Side: Sell, Type: LimitOrder,
		Price: 100, Qty: 3, TimeInForce: GTC, Visibility: Normal,
	}); !response.Success {
		t.Fatalf("seed ask rejected: %+v", response)
	}
	response := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 2, Symbol: "ABC/USD", Side: Buy, Type: LimitOrder,
		Price: 99, Qty: 2, TimeInForce: GTC, Visibility: Normal, PostOnly: true,
	})
	if !response.Success {
		t.Fatalf("non-crossing post-only rejected: %+v", response)
	}
	orderID, ok := response.Data.(uint64)
	if !ok {
		t.Fatalf("accepted order ID = %#v", response.Data)
	}
	order := ex.Books["ABC/USD"].FindOrder(orderID)
	if order == nil || !order.PostOnly || order.Price != 99 || order.FilledQty != 0 {
		t.Fatalf("resting order lost post-only contract: %#v", order)
	}
}

// This is the P0 adversarial mutation: stripping the post-only bit from the
// same crossing request must change the outcome to a real taker fill. If this
// test ever stops distinguishing the paths, the post-only detector is weak.
func TestPostOnlyMutationStrippingBitPermitsCrossingFill(t *testing.T) {
	ex := newPostOnlyTestExchange(t)
	if response := ex.PlaceOrder(2, &OrderRequest{
		RequestID: 1, Symbol: "ABC/USD", Side: Sell, Type: LimitOrder,
		Price: 100, Qty: 3, TimeInForce: GTC, Visibility: Normal,
	}); !response.Success {
		t.Fatalf("seed ask rejected: %+v", response)
	}
	response := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 2, Symbol: "ABC/USD", Side: Buy, Type: LimitOrder,
		Price: 100, Qty: 2, TimeInForce: GTC, Visibility: Normal,
		// Mutation: PostOnly omitted/false.
	})
	if !response.Success {
		t.Fatalf("stripped-bit request rejected: %+v", response)
	}
	book := ex.Books["ABC/USD"]
	if book.LastTrade == nil || book.LastTrade.Price != 100 || book.Asks.Best == nil || book.Asks.Best.TotalQty != 1 {
		t.Fatalf("stripped-bit request did not produce distinct taker outcome: last=%#v ask=%#v", book.LastTrade, book.Asks.Best)
	}
}

func TestPostOnlyRequiresRestingLimitOrder(t *testing.T) {
	tests := []OrderRequest{
		{RequestID: 1, Symbol: "ABC/USD", Side: Buy, Type: Market, Qty: 1, TimeInForce: GTC, Visibility: Normal, PostOnly: true},
		{RequestID: 2, Symbol: "ABC/USD", Side: Buy, Type: LimitOrder, Price: 99, Qty: 1, TimeInForce: IOC, Visibility: Normal, PostOnly: true},
		{RequestID: 3, Symbol: "ABC/USD", Side: Buy, Type: LimitOrder, Price: 99, Qty: 1, TimeInForce: FOK, Visibility: Normal, PostOnly: true},
	}
	for _, request := range tests {
		t.Run(request.Type.String()+"_"+request.TimeInForce.String(), func(t *testing.T) {
			ex := newPostOnlyTestExchange(t)
			beforeID := ex.NextOrderID
			response := ex.PlaceOrder(1, &request)
			if response.Success || response.Error != RejectPostOnlyInvalid {
				t.Fatalf("invalid post-only response = %+v", response)
			}
			if ex.NextOrderID != beforeID || len(ex.Clients[1].OrderIDs) != 0 {
				t.Fatalf("invalid post-only request mutated admission state: next=%d want=%d orders=%v", ex.NextOrderID, beforeID, ex.Clients[1].OrderIDs)
			}
		})
	}
}
