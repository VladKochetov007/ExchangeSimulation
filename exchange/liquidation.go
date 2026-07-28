package exchange

// ForcedCancelNotification is sent to a client's gateway when the exchange
// cancels an order on its behalf (e.g. during liquidation) without a client
// cancel request. Actors decode this via decodeResponse to clean up state.
type ForcedCancelNotification struct {
	OrderID      uint64
	RemainingQty int64
}

// forceClose cancels all open orders for clientID on the given book, then executes
// a market order to close qty on the given side. Returns the last fill price
// (0 if no fill) and the quantity actually closed — a thin book can absorb
// only part of the position, and downstream accounting (clearance fee) must
// bill what executed, not what was attempted.
// Caller must hold e.mu.Lock().
func (e *DefaultExchange) forceClose(clientID uint64, client *Client, book *OrderBook, instrument Instrument, side Side, posSide PositionSide, qty, timestamp int64) (fillPrice, filledQty int64) {
	e.cancelClientOrdersOnBook(client, book, instrument)

	// Same allocation pattern as PlaceOrder (increment, then use): taking the
	// value first would reuse the most recently placed order's ID.
	e.NextOrderID++
	orderID := e.NextOrderID
	order := getOrder()
	order.ID = orderID
	order.ClientID = clientID
	order.Side = side
	order.PositionSide = posSide
	order.Type = Market
	order.Qty = qty
	order.Status = Open
	order.Timestamp = timestamp

	result := e.Matcher.Match(book.Bids, book.Asks, order)
	if len(result.Executions) > 0 {
		fillPrice = result.Executions[len(result.Executions)-1].Price
	}
	filledQty = order.FilledQty
	levels := collectAffectedLevels(book, result.Executions)
	e.processExecutions(book, result.Executions, order)
	e.removeMakerOrders(book, result.Executions)
	e.publishLevels(book, levels)
	putOrder(order)
	return fillPrice, filledQty
}

// cancelClientOrdersOnBook cancels all open orders for client on the given book,
// releasing reserved perp margin, publishing book deltas, and notifying the client
// gateway so actors can clean up their local state.
// Caller must hold e.mu.Lock().
func (e *DefaultExchange) cancelClientOrdersOnBook(client *Client, book *OrderBook, instrument Instrument) {
	gw := e.Gateways[client.ID]
	for _, orderID := range append([]uint64{}, client.OrderIDs...) {
		var order *Order
		if o := book.Bids.Orders[orderID]; o != nil {
			order = o
		} else if o := book.Asks.Orders[orderID]; o != nil {
			order = o
		}
		if order == nil || order.ClientID != client.ID {
			continue
		}
		remainingQty := order.Qty - order.FilledQty
		releaseReserved(client, instrument, order)
		if order.Side == Buy {
			book.Bids.CancelOrder(orderID)
		} else {
			book.Asks.CancelOrder(orderID)
		}
		if order.Visibility != Hidden {
			e.publishBookUpdate(book, order.Side, order.Price)
		}
		client.RemoveOrder(orderID)
		// At-least-once, same as fills: a dropped forced cancel leaves the
		// actor with a ghost pending order that blocks its quoting loop
		// forever (randomwalk postmortem bug 3).
		gw.enqueueResponse(Response{Success: true, Data: &ForcedCancelNotification{OrderID: orderID, RemainingQty: remainingQty}})
	}
}
