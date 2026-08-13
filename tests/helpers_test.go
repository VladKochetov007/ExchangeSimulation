package exchange_test

import (
	"sync/atomic"
	"time"

	"exchange_sim/exchange"
)

// nextTestRequestID hands out unique request IDs. Responses are delivered
// asynchronously through the gateway outbox, so a helper that matched on
// anything less specific than its own request ID could return on a previous
// order's leftover message and read exchange state before its own order was
// processed.
var testRequestSeq atomic.Uint64

func nextTestRequestID() uint64 { return 1_000_000 + testRequestSeq.Add(1) }

// InjectLimitOrder injects a limit order directly into the exchange for testing.
// Returns the OrderID and RejectReason ("" if successful).
func InjectLimitOrder(ex *exchange.Exchange, clientID uint64, symbol string, side exchange.Side, price, qty int64) (uint64, exchange.RejectReason) {
	gateway := ex.Gateways[clientID]
	if gateway == nil {
		return 0, exchange.RejectUnknownClient
	}

	reqID := nextTestRequestID()

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
				return orderID, ""
			}
		case <-timeout:
			return 0, exchange.RejectUnknownInstrument
		}
	}
}

// InjectMarketOrder injects a market order directly into the exchange for testing.
// Returns the OrderID and RejectReason ("" if successful).
func InjectMarketOrder(ex *exchange.Exchange, clientID uint64, symbol string, side exchange.Side, qty int64) (uint64, exchange.RejectReason) {
	gateway := ex.Gateways[clientID]
	if gateway == nil {
		return 0, exchange.RejectUnknownClient
	}

	reqID := nextTestRequestID()
	req := exchange.Request{
		Type: exchange.ReqPlaceOrder,
		OrderReq: &exchange.OrderRequest{
			RequestID:   reqID,
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

	// A market order's fills are delivered BEFORE its accept response, so
	// match strictly on the request ID: anything else returns while the
	// order is still being processed.
	timeout := time.After(2 * time.Second)
	for {
		select {
		case resp := <-gateway.ResponseCh:
			if resp.RequestID != reqID {
				continue
			}
			if !resp.Success {
				return 0, resp.Error
			}
			if orderID, ok := resp.Data.(uint64); ok {
				return orderID, ""
			}
			return 0, exchange.RejectUnknownInstrument
		case <-timeout:
			panic("test timeout: InjectMarketOrder did not receive response - this indicates a real bug")
		}
	}
}
