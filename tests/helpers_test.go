package exchange_test

import (
	"time"

	"exchange_sim/exchange"
)

// InjectLimitOrder injects a limit order directly into the exchange for testing.
// Returns the OrderID and RejectReason (0 if successful).
func InjectLimitOrder(ex *exchange.Exchange, clientID uint64, symbol string, side exchange.Side, price, qty int64) (uint64, exchange.RejectReason) {
	gateway := ex.Gateways[clientID]
	if gateway == nil {
		return 0, exchange.RejectUnknownClient
	}

	reqID := uint64(1000000 + ex.Clock.NowUnixNano()%1000000)

	req := exchange.Request{
		Type: exchange.ReqPlaceOrder,
		OrderReq: &exchange.OrderRequest{
			RequestID:   reqID,
			Side:        side,
			Type:        exchange.LimitOrder,
			Price:       price,
			Qty:         qty,
			Symbol:      symbol,
			TimeInForce: exchange.GTC,
			Visibility:  exchange.Normal,
		},
	}

	gateway.RequestCh <- req

	timeout := time.After(2 * time.Second)
	for {
		select {
		case resp := <-gateway.ResponseCh:
			if resp.RequestID == reqID {
				if !resp.Success {
					return 0, resp.Error
				}
				orderID, ok := resp.Data.(uint64)
				if !ok {
					return 0, exchange.RejectUnknownInstrument
				}
				return orderID, 0
			}
		case <-timeout:
			return 0, exchange.RejectUnknownInstrument
		}
	}
}

// InjectMarketOrder injects a market order directly into the exchange for testing.
// Returns the OrderID and RejectReason (0 if successful).
func InjectMarketOrder(ex *exchange.Exchange, clientID uint64, symbol string, side exchange.Side, qty int64) (uint64, exchange.RejectReason) {
	gateway := ex.Gateways[clientID]
	if gateway == nil {
		return 0, exchange.RejectUnknownClient
	}

	req := exchange.Request{
		Type: exchange.ReqPlaceOrder,
		OrderReq: &exchange.OrderRequest{
			RequestID:   1000000,
			Side:        side,
			Type:        exchange.Market,
			Price:       0,
			Qty:         qty,
			Symbol:      symbol,
			TimeInForce: exchange.GTC,
			Visibility:  exchange.Normal,
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
			case *exchange.FillNotification:
				return data.OrderID, 0
			default:
				continue
			}
		case <-timeout:
			panic("test timeout: InjectMarketOrder did not receive response - this indicates a real bug")
		}
	}
}
