package exchange

const (
	SATOSHI = 100_000_000

	CENT_TICK    = SATOSHI / 100
	DOLLAR_TICK  = SATOSHI
	HUNDRED_TICK = 100 * SATOSHI
)

// TEST ONLY - Limited precision (~15 decimal digits), not for production.
func BTCAmount(btc float64) int64 {
	return int64(btc * float64(SATOSHI))
}

// TEST ONLY - Limited precision (~15 decimal digits), not for production.
func USDAmount(usd float64) int64 {
	return int64(usd * float64(SATOSHI))
}

// TEST ONLY - Rounds price DOWN to nearest tickSize.
func PriceUSD(price float64, tickSize int64) int64 {
	raw := int64(price * float64(SATOSHI))
	return (raw / tickSize) * tickSize
}

// TEST ONLY - Injects a limit order directly into the exchange for testing.
// Returns the OrderID and RejectReason (0 if successful).
func InjectLimitOrder(ex *Exchange, clientID uint64, symbol string, side Side, price, qty int64) (uint64, RejectReason) {
	gateway := ex.Gateways[clientID]
	if gateway == nil {
		return 0, RejectUnknownClient
	}

	req := Request{
		Type: ReqPlaceOrder,
		OrderReq: &OrderRequest{
			RequestID:   1000000, // Use high request ID for test orders
			Side:        side,
			Type:        LimitOrder,
			Price:       price,
			Qty:         qty,
			Symbol:      symbol,
			TimeInForce: GTC,
			Visibility:  Normal,
		},
	}

	// Send request
	gateway.RequestCh <- req

	// Wait for response
	resp := <-gateway.ResponseCh
	if !resp.Success {
		return 0, resp.Error
	}

	// Response data should be the OrderID
	orderID, ok := resp.Data.(uint64)
	if !ok {
		return 0, RejectUnknownInstrument // Generic error
	}

	return orderID, 0
}

// TEST ONLY - Injects a market order directly into the exchange for testing.
// Returns the OrderID and RejectReason (0 if successful).
func InjectMarketOrder(ex *Exchange, clientID uint64, symbol string, side Side, qty int64) (uint64, RejectReason) {
	gateway := ex.Gateways[clientID]
	if gateway == nil {
		return 0, RejectUnknownClient
	}

	req := Request{
		Type: ReqPlaceOrder,
		OrderReq: &OrderRequest{
			RequestID:   1000000, // Use high request ID for test orders
			Side:        side,
			Type:        Market,
			Price:       0,
			Qty:         qty,
			Symbol:      symbol,
			TimeInForce: GTC,
			Visibility:  Normal,
		},
	}

	// Send request
	gateway.RequestCh <- req

	// Wait for response (might be accept or fill notification)
	resp := <-gateway.ResponseCh
	if !resp.Success {
		return 0, resp.Error
	}

	// For market orders, we might get OrderID or FillNotification first
	switch data := resp.Data.(type) {
	case uint64:
		return data, 0
	case *FillNotification:
		return data.OrderID, 0
	default:
		return 0, RejectUnknownInstrument // Generic error
	}
}
