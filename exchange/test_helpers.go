package exchange

import "time"

const (
	// Asset precisions (units per whole unit)
	BTC_PRECISION  = 100_000_000 // 1 BTC = 100,000,000 satoshis
	ETH_PRECISION  = 1_000_000   // 1 ETH = 1,000,000 micro-ETH (gwei would be 1B)
	USD_PRECISION  = 100_000     // 1 USD = 100,000 units (0.001 USD minimum)
	USDT_PRECISION = 100_000     // Same as USD

	// Legacy constant (backward compatibility - do NOT use for non-BTC assets!)
	SATOSHI = BTC_PRECISION

	// Price tick sizes (for price alignment in BTC/USD pairs)
	CENT_TICK    = USD_PRECISION / 100 // 0.01 USD tick = 1,000 units
	DOLLAR_TICK  = USD_PRECISION       // 1 USD tick
	HUNDRED_TICK = 100 * USD_PRECISION // 100 USD tick
)

// TEST ONLY - Limited precision (~15 decimal digits), not for production.
func BTCAmount(btc float64) int64 {
	return int64(btc * float64(BTC_PRECISION))
}

// TEST ONLY - Limited precision (~15 decimal digits), not for production.
func ETHAmount(eth float64) int64 {
	return int64(eth * float64(ETH_PRECISION))
}

// TEST ONLY - Limited precision (~15 decimal digits), not for production.
func USDAmount(usd float64) int64 {
	return int64(usd * float64(USD_PRECISION))
}

// TEST ONLY - Limited precision (~15 decimal digits), not for production.
func USDTAmount(usdt float64) int64 {
	return int64(usdt * float64(USDT_PRECISION))
}

// TEST ONLY - Rounds price DOWN to nearest tickSize.
// Price is in USD per BTC, returns value in USD_PRECISION units.
func PriceUSD(price float64, tickSize int64) int64 {
	raw := int64(price * float64(USD_PRECISION))
	return (raw / tickSize) * tickSize
}

// TEST ONLY - Injects a limit order directly into the exchange for testing.
// Returns the OrderID and RejectReason (0 if successful).
func InjectLimitOrder(ex *Exchange, clientID uint64, symbol string, side Side, price, qty int64) (uint64, RejectReason) {
	gateway := ex.Gateways[clientID]
	if gateway == nil {
		return 0, RejectUnknownClient
	}

	// Generate unique request ID using timestamp
	reqID := uint64(1000000 + ex.Clock.NowUnixNano()%1000000)

	req := Request{
		Type: ReqPlaceOrder,
		OrderReq: &OrderRequest{
			RequestID:   reqID,
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

	// Wait for matching response (with timeout)
	timeout := time.After(2 * time.Second)
	for {
		select {
		case resp := <-gateway.ResponseCh:
			if resp.RequestID == reqID {
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
			// Wrong response, keep reading
		case <-timeout:
			return 0, RejectUnknownInstrument // Timeout treated as error
		}
	}
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

	gateway.RequestCh <- req

	timeout := time.After(200 * time.Millisecond)
	for {
		select {
		case resp := <-gateway.ResponseCh:
			if !resp.Success {
				return 0, resp.Error
			}
			switch data := resp.Data.(type) {
			case uint64:
				return data, 0
			case *FillNotification:
				return data.OrderID, 0
			default:
				continue
			}
		case <-timeout:
			panic("test timeout: InjectMarketOrder did not receive response - this indicates a real bug")
		}
	}
}
