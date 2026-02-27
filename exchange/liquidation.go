package exchange

// forceClose cancels all open orders for clientID on the given book, then executes
// a market order to close qty on the given side. Returns the fill price (0 if no fill).
// Caller must hold e.mu.Lock().
func (e *Exchange) forceClose(clientID uint64, client *Client, book *OrderBook, perp *PerpFutures, side Side, qty, timestamp int64) (fillPrice int64) {
	e.cancelClientOrdersOnBook(client, book, perp)

	orderID := e.NextOrderID
	e.NextOrderID++
	order := getOrder()
	order.ID = orderID
	order.ClientID = clientID
	order.Side = side
	order.Type = Market
	order.Qty = qty
	order.Status = Open
	order.Timestamp = timestamp

	result := e.Matcher.Match(book.Bids, book.Asks, order)
	if len(result.Executions) > 0 {
		fillPrice = result.Executions[len(result.Executions)-1].Price
	}
	e.processExecutions(book, result.Executions, order)
	putOrder(order)
	return fillPrice
}

// cancelClientOrdersOnBook cancels all open orders for client on the given book,
// releasing reserved perp margin for each cancelled order.
// Caller must hold e.mu.Lock().
func (e *Exchange) cancelClientOrdersOnBook(client *Client, book *OrderBook, perp *PerpFutures) {
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
		remainingMargin := (remainingQty * order.Price / perp.BasePrecision()) * perp.MarginRate / 10000
		client.ReleasePerp(perp.QuoteAsset(), remainingMargin)
		if order.Side == Buy {
			book.Bids.CancelOrder(orderID)
		} else {
			book.Asks.CancelOrder(orderID)
		}
		client.RemoveOrder(orderID)
	}
}
