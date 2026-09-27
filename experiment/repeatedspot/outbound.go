package repeatedspot

import (
	"encoding/json"
	"errors"
	"fmt"

	"exchange_sim/exchange"
	worldspot "exchange_sim/simulations/repeatedspot"
)

type OutboundSummary struct {
	PlaceSent       int64 `json:"place_sent"`
	CancelSent      int64 `json:"cancel_sent"`
	PlaceAccepted   int64 `json:"place_accepted"`
	PlaceRejected   int64 `json:"place_rejected"`
	CancelAccepted  int64 `json:"cancel_accepted"`
	CancelRejected  int64 `json:"cancel_rejected"`
	UnresolvedAtEnd int64 `json:"unresolved_at_end"`
}

type requestKey struct {
	clientID  uint64
	requestID uint64
}

type sentOrder struct {
	request          sentRequest
	at               int64
	resolved         bool
	outcome          string
	outcomeAt        int64
	outcomeOrderID   uint64
	outcomeRemaining int64
	outcomeReason    string
}

type sentRequest struct {
	Type      string            `json:"Type"`
	OrderReq  *sentPlacement    `json:"OrderReq"`
	CancelReq *sentCancellation `json:"CancelReq"`
	QueryReq  *json.RawMessage  `json:"QueryReq"`
}

type sentPlacement struct {
	RequestID    uint64 `json:"request_id"`
	Side         string `json:"side"`
	Type         string `json:"type"`
	Price        int64  `json:"price"`
	Qty          int64  `json:"qty"`
	Symbol       string `json:"symbol"`
	TimeInForce  string `json:"time_in_force"`
	Visibility   string `json:"visibility"`
	IcebergQty   int64  `json:"iceberg_qty"`
	PositionSide uint8  `json:"position_side"`
	PostOnly     bool   `json:"post_only"`
	ReduceOnly   bool   `json:"reduce_only"`
}

type sentCancellation struct {
	RequestID uint64 `json:"request_id"`
	OrderID   uint64 `json:"order_id"`
}

func (state *replayState) orderSend(event Event) error {
	account := state.accounts[event.ClientID]
	if account == nil || event.Route != state.contract.Instrument.Symbol {
		return errors.New("repeated spot: outbound request has unknown actor or route")
	}
	var request sentRequest
	if err := decodePayload(event.Payload, &request); err != nil {
		return err
	}
	var requestID uint64
	switch request.Type {
	case exchange.ReqPlaceOrder:
		order := request.OrderReq
		if order == nil || request.CancelReq != nil || request.QueryReq != nil ||
			order.RequestID == 0 || order.Symbol != state.contract.Instrument.Symbol || order.Qty <= 0 ||
			order.Price < 0 || !validSide(order.Side) ||
			(order.Type != "LIMIT" && order.Type != "MARKET") ||
			(order.TimeInForce != "GTC" && order.TimeInForce != "IOC" && order.TimeInForce != "FOK") ||
			order.Visibility != "NORMAL" || order.PositionSide != uint8(exchange.PositionBoth) ||
			order.IcebergQty != 0 || order.ReduceOnly {
			return errors.New("repeated spot: malformed one-book outbound placement")
		}
		requestID = order.RequestID
		account.outbound.PlaceSent++
	case exchange.ReqCancelOrder:
		cancel := request.CancelReq
		if cancel == nil || request.OrderReq != nil || request.QueryReq != nil ||
			cancel.RequestID == 0 || cancel.OrderID == 0 {
			return errors.New("repeated spot: malformed outbound cancellation")
		}
		requestID = cancel.RequestID
		account.outbound.CancelSent++
	default:
		return errors.New("repeated spot: unsupported outbound request type")
	}
	key := requestKey{event.ClientID, requestID}
	if _, duplicate := state.sentRequests[key]; duplicate {
		return errors.New("repeated spot: duplicate actor request identity")
	}
	sent := &sentOrder{request: request, at: event.Timestamp}
	state.sentRequests[key] = sent
	if account.maker != nil {
		account.pendingSends = append(account.pendingSends, sent)
	}
	return nil
}

func (state *replayState) checkOrderOutcome(event Event) error {
	var requestID uint64
	var expectedType string
	switch event.Name {
	case "OrderAccepted":
		var accepted acceptedOrder
		if err := json.Unmarshal(event.Payload, &accepted); err != nil {
			return err
		}
		requestID, expectedType = accepted.RequestID, exchange.ReqPlaceOrder
		sent, err := state.resolveRequest(event.ClientID, requestID, expectedType, event.Timestamp)
		if err != nil {
			return err
		}
		order := sent.request.OrderReq
		if order.Symbol != state.contract.Instrument.Symbol || accepted.ClientID != event.ClientID ||
			accepted.Side != order.Side || accepted.Price != order.Price || accepted.Qty != order.Qty ||
			accepted.Type != order.Type || accepted.TimeInForce != order.TimeInForce ||
			accepted.PostOnly != order.PostOnly || accepted.Visibility != order.Visibility ||
			accepted.IcebergQty != order.IcebergQty || accepted.PositionSide != order.PositionSide ||
			accepted.FilledQty != 0 || accepted.Status != uint8(exchange.Open) ||
			accepted.Timestamp != event.Timestamp {
			return errors.New("repeated spot: accepted order differs from actor submission")
		}
		state.accounts[event.ClientID].outbound.PlaceAccepted++
		sent.outcome, sent.outcomeAt, sent.outcomeOrderID = event.Name, event.Timestamp, accepted.OrderID
	case "OrderRejected":
		var rejected struct {
			RequestID   uint64 `json:"request_id"`
			Error       string `json:"error"`
			Symbol      string `json:"symbol"`
			Qty         int64  `json:"qty"`
			Side        string `json:"side"`
			Type        string `json:"type"`
			TimeInForce string `json:"time_in_force"`
			PostOnly    bool   `json:"post_only"`
			Price       int64  `json:"price"`
		}
		if err := json.Unmarshal(event.Payload, &rejected); err != nil {
			return err
		}
		requestID, expectedType = rejected.RequestID, exchange.ReqPlaceOrder
		sent, err := state.resolveRequest(event.ClientID, requestID, expectedType, event.Timestamp)
		if err != nil {
			return err
		}
		order := sent.request.OrderReq
		if rejected.Symbol != order.Symbol || rejected.Qty != order.Qty ||
			rejected.Side != order.Side || rejected.Type != order.Type ||
			rejected.TimeInForce != order.TimeInForce || rejected.PostOnly != order.PostOnly ||
			rejected.Price != order.Price {
			return errors.New("repeated spot: rejected order differs from actor submission")
		}
		state.accounts[event.ClientID].outbound.PlaceRejected++
		sent.outcome, sent.outcomeAt, sent.outcomeReason = event.Name, event.Timestamp, rejected.Error
	case "OrderCancelled":
		var cancelled struct {
			RequestID    *uint64 `json:"request_id"`
			OrderID      uint64  `json:"order_id"`
			RemainingQty int64   `json:"remaining_qty"`
			Reason       string  `json:"reason"`
		}
		if err := json.Unmarshal(event.Payload, &cancelled); err != nil {
			return err
		}
		if cancelled.Reason != "" {
			switch cancelled.Reason {
			case "IOC_EXPIRED", "NO_LIQUIDITY":
				accepted := state.trades.orders[cancelled.OrderID]
				if cancelled.RequestID == nil || accepted.OrderID == 0 ||
					*cancelled.RequestID != accepted.RequestID {
					return errors.New("repeated spot: expiry cancellation has wrong placement request")
				}
			case "EXCHANGE_FORCED_FEE_RESERVATION", "EXCHANGE_FORCED_LIFECYCLE",
				"EXCHANGE_FORCED_SELF_TRADE_PREVENTION", "EXCHANGE_FORCED_BOOK_ADMISSION":
				if cancelled.RequestID != nil {
					return errors.New("repeated spot: exchange-forced cancellation carries an actor request")
				}
			default:
				return errors.New("repeated spot: unsupported forced cancellation reason")
			}
			return nil
		}
		if cancelled.RequestID == nil {
			return errors.New("repeated spot: actor cancellation lacks request identity")
		}
		sent, err := state.resolveRequest(event.ClientID, *cancelled.RequestID, exchange.ReqCancelOrder, event.Timestamp)
		if err != nil {
			return err
		}
		if sent.request.CancelReq.OrderID != cancelled.OrderID {
			return errors.New("repeated spot: cancellation targets a different order")
		}
		state.accounts[event.ClientID].outbound.CancelAccepted++
		sent.outcome, sent.outcomeAt, sent.outcomeOrderID, sent.outcomeRemaining = event.Name, event.Timestamp, cancelled.OrderID, cancelled.RemainingQty
	case "OrderCancelRejected":
		var rejected struct {
			RequestID uint64 `json:"request_id"`
			OrderID   uint64 `json:"order_id"`
			Error     string `json:"error"`
		}
		if err := json.Unmarshal(event.Payload, &rejected); err != nil {
			return err
		}
		sent, err := state.resolveRequest(event.ClientID, rejected.RequestID, exchange.ReqCancelOrder, event.Timestamp)
		if err != nil {
			return err
		}
		if sent.request.CancelReq.OrderID != rejected.OrderID {
			return errors.New("repeated spot: cancel rejection targets a different order")
		}
		state.accounts[event.ClientID].outbound.CancelRejected++
		sent.outcome, sent.outcomeAt, sent.outcomeOrderID, sent.outcomeReason = event.Name, event.Timestamp, rejected.OrderID, rejected.Error
	}
	return nil
}

func (state *replayState) resolveRequest(clientID, requestID uint64, kind string, at int64) (*sentOrder, error) {
	sent := state.sentRequests[requestKey{clientID, requestID}]
	if sent == nil || sent.resolved || sent.request.Type != kind || at < sent.at {
		return nil, fmt.Errorf("repeated spot: exchange outcome lacks a prior unresolved %s request", kind)
	}
	sent.resolved = true
	return sent, nil
}

func (account *accountState) verifyMakerSends(decision worldspot.MakerDecision, at int64) error {
	placedSide := make(map[string]bool, 2)
	for _, sent := range account.pendingSends {
		if sent.at != at {
			return errors.New("repeated spot: maker order send is not attached to its decision time")
		}
		switch decision.Action {
		case "evaluate_placements":
			order := sent.request.OrderReq
			if order == nil || !order.PostOnly || order.Type != "LIMIT" ||
				order.TimeInForce != "GTC" || order.Qty > account.maker.quoteQty ||
				placedSide[order.Side] ||
				order.Side == "BUY" && order.Price != decision.TargetBid ||
				order.Side == "SELL" && order.Price != decision.TargetAsk {
				return errors.New("repeated spot: maker placement differs from its computed target")
			}
			placedSide[order.Side] = true
		case "cancel_quotes":
			if sent.request.CancelReq == nil {
				return errors.New("repeated spot: maker cancellation decision submitted a placement")
			}
		default:
			return errors.New("repeated spot: maker submitted an order during a no-send decision")
		}
	}
	account.pendingSends = nil
	return nil
}
