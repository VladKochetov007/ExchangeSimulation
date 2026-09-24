package multivenue

import "exchange_sim/exchange"

// CrossVenueArbResponseReceipt records actor-inbox arrival. A FILL's
// ExchangeAt remains the earlier match time; ReceivedAt is the courier time.
type CrossVenueArbResponseReceipt struct {
	RouterID     uint64                `json:"router_id"`
	ActorID      uint64                `json:"actor_id"`
	ClientID     uint64                `json:"client_id"`
	VenueID      string                `json:"venue_id"`
	ReceivedAt   int64                 `json:"received_at"`
	Kind         string                `json:"kind"`
	RequestID    uint64                `json:"request_id"`
	Success      bool                  `json:"success"`
	Error        exchange.RejectReason `json:"error"`
	OrderID      uint64                `json:"order_id"`
	TradeID      uint64                `json:"trade_id"`
	Symbol       string                `json:"symbol"`
	Side         string                `json:"side"`
	Qty          int64                 `json:"qty"`
	Price        int64                 `json:"price"`
	FeeAmount    int64                 `json:"fee_amount"`
	FeeAsset     string                `json:"fee_asset"`
	ExchangeAt   int64                 `json:"exchange_at"`
	IsFull       bool                  `json:"is_full"`
	RemainingQty int64                 `json:"remaining_qty"`
}

func newCrossVenueArbResponseReceipt(routerID, actorID, clientID uint64, venueID string, response exchange.Response, receivedAt int64) CrossVenueArbResponseReceipt {
	receipt := CrossVenueArbResponseReceipt{
		RouterID: routerID, ActorID: actorID, ClientID: clientID, VenueID: venueID,
		ReceivedAt: receivedAt, RequestID: response.RequestID, Success: response.Success, Error: response.Error,
		Kind: "OTHER",
	}
	if !response.Success {
		receipt.Kind = "REJECTED"
		return receipt
	}
	switch data := response.Data.(type) {
	case uint64:
		receipt.Kind, receipt.OrderID = "ORDER_ACCEPTED", data
	case int64:
		receipt.Kind, receipt.RemainingQty = "ORDER_CANCELLED", data
	case *exchange.FillNotification:
		if data != nil {
			receipt.Kind = "FILL"
			receipt.OrderID, receipt.TradeID = data.OrderID, data.TradeID
			receipt.Symbol, receipt.Side = data.Symbol, data.Side.String()
			receipt.Qty, receipt.Price = data.Qty, data.Price
			receipt.FeeAmount, receipt.FeeAsset = data.FeeAmount, data.FeeAsset
			receipt.ExchangeAt, receipt.IsFull = data.Timestamp, data.IsFull
		}
	case *exchange.ForcedCancelNotification:
		if data != nil {
			receipt.Kind, receipt.OrderID, receipt.RemainingQty = "FORCED_CANCELLED", data.OrderID, data.RemainingQty
		}
	}
	return receipt
}
