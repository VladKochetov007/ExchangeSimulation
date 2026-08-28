package exchange

// ForcedCancelNotification is sent to a client's gateway when the exchange
// cancels an order on its behalf (e.g. during liquidation) without a client
// cancel request. Actors decode this via decodeResponse to clean up state.
type ForcedCancelNotification struct {
	OrderID      uint64
	RemainingQty int64
}

// forceClose cancels all open orders for clientID on the given book, then executes
// a market order to close qty on the given side. Its final boolean says
// whether any fill occurred; a numeric zero may be the valid last fill price
// of a signed contract. A thin book can absorb only part of the position, and
// downstream accounting (clearance fee) must bill what executed, not what was
// attempted.
// Caller must hold e.mu.Lock().
func (e *DefaultExchange) forceClose(clientID uint64, client *Client, book *OrderBook, instrument Instrument, side Side, posSide PositionSide, qty, timestamp int64) (fillPrice, filledQty int64, filled bool) {
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

	// Quote price-dependent fees against precisely the book that remains after
	// the liquidated client's resting orders are removed, but do so before
	// removing them. A missing configured fee reference must defer the close
	// without mutating the book or client reservations.
	excluded := make(map[uint64]struct{}, len(client.OrderIDs))
	for _, orderID := range client.OrderIDs {
		if book.FindOrder(orderID) != nil {
			excluded[orderID] = struct{}{}
		}
	}
	plan, failure := e.prepareFeeExecutionPlan(book, order, excluded)
	if failure != nil {
		if failure.err != nil {
			e.reportPriceUnavailable(timestamp, book.Symbol, "liquidation_fee_preflight", failure.err)
			putOrder(order)
			return 0, 0, false
		}
		panic("matching engine could not produce liquidation fee preflight")
	}
	e.cancelClientOrdersOnBook(client, book, instrument)
	result := e.Matcher.Match(book.Bids, book.Asks, order)
	if !plan.matches(result.Executions) {
		panic("matching engine violated liquidation fee preflight")
	}
	if len(result.Executions) > 0 {
		fillPrice = result.Executions[len(result.Executions)-1].Price
	}
	filledQty = order.FilledQty
	filled = filledQty > 0
	levels := collectAffectedLevels(book, result.Executions)
	e.processExecutions(book, result.Executions, order, plan)
	e.removeMakerOrders(book, result.Executions)
	e.publishLevels(book, levels)
	putOrder(order)
	return fillPrice, filledQty, filled
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
		e.logExchangeForcedCancellation(book, order, remainingQty, exchangeForcedLifecycleReason)
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
		// The order left the live book due to an exchange-side lifecycle action
		// (liquidation or pending expiry), not a fill. Retaining Open here would
		// let an actor or audit treat a permanently halted order as executable.
		order.Status = Cancelled
		// At-least-once, same as fills: a dropped forced cancel leaves the
		// actor with a ghost pending order that blocks its quoting loop
		// forever (randomwalk postmortem bug 3).
		gw.enqueueResponse(Response{Success: true, Data: &ForcedCancelNotification{OrderID: orderID, RemainingQty: remainingQty}})
	}
}
