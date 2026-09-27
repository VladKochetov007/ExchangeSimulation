package repeatedspot

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"exchange_sim/exchange"
	worldspot "exchange_sim/simulations/repeatedspot"
)

type MakerResponseSummary struct {
	Received             int64 `json:"received"`
	Processed            int64 `json:"processed"`
	FillReceived         int64 `json:"fill_received"`
	FillProcessed        int64 `json:"fill_processed"`
	BufferedAtEnd        int64 `json:"buffered_at_end"`
	NetProcessedFillBase int64 `json:"net_processed_fill_base_units"`
	SubscriptionAcks     int64 `json:"subscription_acks"`
}

type makerReceiptKey struct {
	clientID  uint64
	kind      string
	requestID uint64
	orderID   uint64
	tradeID   uint64
}

type makerReceipt struct {
	response    worldspot.MakerProcessedResponse
	deliveredAt int64
	sequence    uint64
	consumed    bool
}

type makerWireResponse struct {
	RequestID uint64          `json:"request_id"`
	Success   bool            `json:"success"`
	Data      json.RawMessage `json:"data"`
	Error     string          `json:"error"`
}

type makerWireFill struct {
	OrderID       uint64 `json:"order_id"`
	ClientID      uint64 `json:"client_id"`
	TradeID       uint64 `json:"trade_id"`
	Symbol        string `json:"symbol"`
	Qty           int64  `json:"qty"`
	Price         int64  `json:"price"`
	Side          string `json:"side"`
	PositionSide  uint8  `json:"position_side"`
	IsFull        bool   `json:"is_full"`
	FeeAmount     int64  `json:"fee_amount"`
	FeeAsset      string `json:"fee_asset"`
	RealizedPnL   int64  `json:"realized_pnl"`
	NewSize       int64  `json:"new_size"`
	NewEntryPrice int64  `json:"new_entry_price"`
	Timestamp     int64  `json:"timestamp"`
}

func requireMakerFields(raw json.RawMessage, fields ...string) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return errors.New("repeated spot: maker response is not an object")
	}
	for _, field := range fields {
		if _, exists := object[field]; !exists {
			return fmt.Errorf("repeated spot: maker response omitted %s", field)
		}
	}
	return nil
}

func makerKey(clientID uint64, response worldspot.MakerProcessedResponse) makerReceiptKey {
	return makerReceiptKey{clientID: clientID, kind: response.Kind, requestID: response.RequestID,
		orderID: response.OrderID, tradeID: response.TradeID}
}

func (state *replayState) makerResponseReceipt(event Event) error {
	account := state.accounts[event.ClientID]
	if account == nil || account.maker == nil || event.Route != state.contract.Instrument.Symbol {
		return errors.New("repeated spot: response receipt has unknown maker or route")
	}
	if err := requireMakerFields(event.Payload, "request_id", "success", "error"); err != nil {
		return err
	}
	var wire makerWireResponse
	if err := decodePayload(event.Payload, &wire); err != nil {
		return err
	}
	if wire.Success && wire.Error != "" || !wire.Success && wire.Error == "" {
		return errors.New("repeated spot: contradictory maker response status")
	}
	response := worldspot.MakerProcessedResponse{ActorID: account.actorID}
	anchorAt := int64(0)
	dataPresent := len(wire.Data) != 0 && !bytes.Equal(wire.Data, []byte("null"))
	request := state.sentRequests[requestKey{event.ClientID, wire.RequestID}]
	switch {
	case wire.Success && !dataPresent:
		if wire.RequestID == 0 || request != nil {
			return errors.New("repeated spot: order response missing required data")
		}
		account.localResponses.SubscriptionAcks++
		return nil
	case !wire.Success:
		if wire.RequestID == 0 || request == nil || !request.resolved {
			return errors.New("repeated spot: unanchored maker rejection receipt")
		}
		response.RequestID, response.Reason = wire.RequestID, wire.Error
		if request.request.Type == exchange.ReqPlaceOrder && request.outcome == "OrderRejected" {
			response.Kind = "order_rejected"
		} else if request.request.Type == exchange.ReqCancelOrder && request.outcome == "OrderCancelRejected" {
			response.Kind, response.OrderID = "cancel_rejected", request.outcomeOrderID
		} else {
			return errors.New("repeated spot: maker rejection differs from exchange outcome")
		}
		if response.Reason != request.outcomeReason || request.outcomeAt > event.Timestamp {
			return errors.New("repeated spot: maker rejection reason or time differs from exchange")
		}
		anchorAt = request.outcomeAt
	case wire.Success && wire.RequestID != 0 && request != nil:
		response.RequestID = wire.RequestID
		if !request.resolved || request.outcomeAt > event.Timestamp {
			return errors.New("repeated spot: maker success precedes exchange outcome")
		}
		switch request.request.Type {
		case exchange.ReqPlaceOrder:
			response.Kind = "accepted"
			if err := json.Unmarshal(wire.Data, &response.OrderID); err != nil ||
				response.OrderID == 0 || request.outcome != "OrderAccepted" ||
				response.OrderID != request.outcomeOrderID {
				return errors.New("repeated spot: maker acceptance differs from exchange admission")
			}
		case exchange.ReqCancelOrder:
			response.Kind, response.OrderID = "cancelled", request.outcomeOrderID
			if err := json.Unmarshal(wire.Data, &response.RemainingQty); err != nil ||
				response.RemainingQty < 0 || request.outcome != "OrderCancelled" ||
				response.RemainingQty != request.outcomeRemaining {
				return errors.New("repeated spot: maker cancellation differs from exchange outcome")
			}
		default:
			return errors.New("repeated spot: unsupported successful maker request")
		}
		anchorAt = request.outcomeAt
	case wire.Success && dataPresent:
		if wire.RequestID != 0 {
			return errors.New("repeated spot: unanchored unsolicited response request identity")
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(wire.Data, &fields); err != nil || fields == nil {
			return errors.New("repeated spot: unsupported maker response data")
		}
		if _, fill := fields["trade_id"]; fill {
			if err := state.makerFillReceipt(event, wire.Data, &response); err != nil {
				return err
			}
			anchorAt = response.ExchangeAt
		} else {
			if err := state.makerForcedCancelReceipt(event, wire.Data, &response); err != nil {
				return err
			}
			anchorAt = state.trades.cancelAt[response.OrderID]
		}
	default:
		return errors.New("repeated spot: unsupported maker response")
	}
	if anchorAt > event.Timestamp || account.responseDelay > event.Timestamp-anchorAt {
		return errors.New("repeated spot: maker response arrived before its declared transport delay")
	}
	key := makerKey(event.ClientID, response)
	if _, duplicate := state.makerReceipts[key]; duplicate {
		return errors.New("repeated spot: duplicate maker response receipt")
	}
	state.makerReceipts[key] = &makerReceipt{response: response, deliveredAt: event.Timestamp, sequence: event.Sequence}
	account.localResponses.Received++
	if response.Kind == "fill" {
		account.localResponses.FillReceived++
	}
	return nil
}

func (state *replayState) makerFillReceipt(event Event, raw json.RawMessage, response *worldspot.MakerProcessedResponse) error {
	if err := requireMakerFields(raw, "order_id", "client_id", "trade_id", "symbol", "qty", "price", "side", "position_side", "is_full", "fee_amount", "fee_asset", "timestamp"); err != nil {
		return err
	}
	var fill makerWireFill
	if err := decodePayload(raw, &fill); err != nil {
		return err
	}
	record := state.trades.trades[fill.TradeID]
	if record == nil || fill.OrderID == 0 || fill.ClientID != event.ClientID ||
		fill.Symbol != state.contract.Instrument.Symbol || fill.Qty <= 0 || fill.Price <= 0 ||
		!validSide(fill.Side) || fill.PositionSide != uint8(exchange.PositionBoth) ||
		fill.Timestamp != record.timestamp || fill.Timestamp > event.Timestamp ||
		fill.RealizedPnL != 0 || fill.NewSize != 0 || fill.NewEntryPrice != 0 {
		return errors.New("repeated spot: invalid or unanchored maker fill receipt")
	}
	order := state.trades.orders[fill.OrderID]
	matched := false
	for _, leg := range record.fills {
		if leg.clientID != event.ClientID || leg.fill.OrderID != fill.OrderID {
			continue
		}
		matched = leg.fill.Qty == fill.Qty && leg.fill.Price == fill.Price &&
			leg.fill.Side == fill.Side && leg.fill.FeeAmount == fill.FeeAmount &&
			leg.fill.FeeAsset == fill.FeeAsset && fill.IsFull == (leg.fill.FilledQty == order.Qty)
	}
	if !matched {
		return errors.New("repeated spot: maker fill receipt differs from canonical exchange fill")
	}
	*response = worldspot.MakerProcessedResponse{ActorID: state.accounts[event.ClientID].actorID, Kind: "fill",
		OrderID: fill.OrderID, TradeID: fill.TradeID, Symbol: fill.Symbol, Qty: fill.Qty,
		Price: fill.Price, Side: fill.Side, IsFull: fill.IsFull, FeeAmount: fill.FeeAmount,
		FeeAsset: fill.FeeAsset, ExchangeAt: fill.Timestamp}
	return nil
}

func (state *replayState) makerForcedCancelReceipt(event Event, raw json.RawMessage, response *worldspot.MakerProcessedResponse) error {
	if err := requireMakerFields(raw, "OrderID", "RemainingQty"); err != nil {
		return err
	}
	var cancelled exchange.ForcedCancelNotification
	if err := decodePayload(raw, &cancelled); err != nil {
		return err
	}
	order := state.trades.orders[cancelled.OrderID]
	if cancelled.OrderID == 0 || order.ClientID != event.ClientID ||
		!state.trades.forcedCancels[cancelled.OrderID] ||
		cancelled.RemainingQty != state.trades.cancelRemainders[cancelled.OrderID] {
		return errors.New("repeated spot: forced cancellation receipt lacks matching venue transition")
	}
	response.Kind, response.OrderID, response.RemainingQty = "forced_cancel", cancelled.OrderID, cancelled.RemainingQty
	return nil
}

func (state *replayState) makerProcessedResponse(event Event) error {
	if err := requireMakerFields(event.Payload, "actor_id", "kind", "processed_at", "request_id",
		"order_id", "trade_id", "symbol", "qty", "price", "side", "is_full",
		"fee_amount", "fee_asset", "exchange_at", "remaining_qty", "reason", "filled_inventory"); err != nil {
		return err
	}
	var processed worldspot.MakerProcessedResponse
	if err := decodePayload(event.Payload, &processed); err != nil {
		return err
	}
	account := state.accounts[event.ClientID]
	if account == nil || account.maker == nil || event.Route != state.contract.Instrument.Symbol ||
		processed.ActorID != account.actorID || processed.ProcessedAt != event.Timestamp {
		return errors.New("repeated spot: processed maker response has wrong actor or time")
	}
	receipt := state.makerReceipts[makerKey(event.ClientID, processed)]
	if receipt == nil || receipt.consumed || receipt.deliveredAt > event.Timestamp || receipt.sequence >= event.Sequence {
		return errors.New("repeated spot: maker processed response lacks an earlier delivered receipt")
	}
	expected := receipt.response
	expected.ProcessedAt = processed.ProcessedAt
	expected.FilledInventory = processed.FilledInventory
	if expected != processed {
		return errors.New("repeated spot: processed maker response differs from delivered exchange response")
	}
	if processed.Kind == "accepted" {
		account.acceptedLocally[processed.OrderID] = true
	}
	if processed.Kind == "fill" {
		if !account.acceptedLocally[processed.OrderID] {
			return errors.New("repeated spot: maker processed a fill before its order acceptance")
		}
		quantity := processed.Qty
		if processed.Side == "SELL" {
			quantity = -quantity
		}
		updated, ok := checkedAdd(account.localResponses.NetProcessedFillBase, quantity)
		if !ok || updated < -account.workingLimit || updated > account.workingLimit {
			return errors.New("repeated spot: processed maker inventory exceeds finite working limit")
		}
		account.localResponses.NetProcessedFillBase = updated
		account.localResponses.FillProcessed++
		if err := account.localRisk.add(event.Timestamp, quantity); err != nil {
			return err
		}
	}
	if processed.FilledInventory != account.localResponses.NetProcessedFillBase {
		return errors.New("repeated spot: maker-local inventory disagrees with processed fill sequence")
	}
	receipt.consumed = true
	account.localResponses.Processed++
	return nil
}

func (state *replayState) finishMakerResponses() error {
	for key, receipt := range state.makerReceipts {
		if receipt.consumed {
			continue
		}
		account := state.accounts[key.clientID]
		if key.kind != "fill" || account.acceptedLocally[key.orderID] {
			return fmt.Errorf("repeated spot: maker %d has an unprocessed delivered %s response", key.clientID, key.kind)
		}
		order := state.trades.orders[key.orderID]
		acceptance := makerReceiptKey{clientID: key.clientID, kind: "accepted",
			requestID: order.RequestID, orderID: key.orderID}
		if state.makerReceipts[acceptance] != nil {
			return fmt.Errorf("repeated spot: maker %d buffered fill despite delivered acceptance", key.clientID)
		}
		account.localResponses.BufferedAtEnd++
	}
	terminalAt := state.contract.StartUnixNano + int64(state.contract.Iterations)*state.contract.Step
	for key, sent := range state.sentRequests {
		account := state.accounts[key.clientID]
		if account.maker == nil || !sent.resolved || !responseDueByEnd(sent.outcomeAt, account.responseDelay, terminalAt) {
			continue
		}
		kind := map[string]string{"OrderAccepted": "accepted", "OrderRejected": "order_rejected",
			"OrderCancelled": "cancelled", "OrderCancelRejected": "cancel_rejected"}[sent.outcome]
		if kind == "" || state.makerReceipts[makerReceiptKey{clientID: key.clientID, kind: kind,
			requestID: key.requestID, orderID: sent.outcomeOrderID}] == nil {
			return fmt.Errorf("repeated spot: maker %d missing due response to request %d", key.clientID, key.requestID)
		}
	}
	for tradeID, record := range state.trades.trades {
		for _, leg := range record.fills {
			account := state.accounts[leg.clientID]
			if account.maker != nil && responseDueByEnd(record.timestamp, account.responseDelay, terminalAt) &&
				state.makerReceipts[makerReceiptKey{clientID: leg.clientID, kind: "fill",
					orderID: leg.fill.OrderID, tradeID: tradeID}] == nil {
				return fmt.Errorf("repeated spot: maker %d missing due fill response for trade %d", leg.clientID, tradeID)
			}
		}
	}
	for orderID, forced := range state.trades.forcedCancels {
		if !forced {
			continue
		}
		order := state.trades.orders[orderID]
		account := state.accounts[order.ClientID]
		if account.maker != nil && responseDueByEnd(state.trades.cancelAt[orderID], account.responseDelay, terminalAt) &&
			state.makerReceipts[makerReceiptKey{clientID: order.ClientID, kind: "forced_cancel", orderID: orderID}] == nil {
			return fmt.Errorf("repeated spot: maker %d missing due forced-cancel response for order %d", order.ClientID, orderID)
		}
	}
	return nil
}

func responseDueByEnd(exchangeAt, delay, terminalAt int64) bool {
	return exchangeAt <= terminalAt && delay <= terminalAt-exchangeAt
}
