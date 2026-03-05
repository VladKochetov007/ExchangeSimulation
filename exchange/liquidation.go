package exchange

// ForcedCancelNotification is sent to a client's gateway when the exchange
// cancels an order on its behalf (e.g. during liquidation) without a client
// cancel request. Actors decode this via decodeResponse to clean up state.
type ForcedCancelNotification struct {
	OrderID      uint64
	RemainingQty int64
}

// forceClose cancels all open orders for clientID on the given book, then executes
// a market order to close qty on the given side. Returns the fill price (0 if no fill).
// Caller must hold e.mu.Lock().
func (e *DefaultExchange) forceClose(clientID uint64, client *Client, book *OrderBook, instrument Instrument, side Side, qty, timestamp int64) (fillPrice int64) {
	e.cancelClientOrdersOnBook(client, book, instrument)

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
// releasing reserved perp margin, publishing book deltas, and notifying the client
// gateway so actors can clean up their local state.
// Caller must hold e.mu.Lock().
func (e *DefaultExchange) cancelClientOrdersOnBook(client *Client, book *OrderBook, instrument Instrument) {
	m, isMargined := instrument.(Margined)
	precision := instrument.BasePrecision()
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
		if isMargined {
			client.ReleasePerp(instrument.QuoteAsset(), m.MarginOnCancel(remainingQty, order.Price, precision))
		}
		if order.Side == Buy {
			book.Bids.CancelOrder(orderID)
			e.publishBookUpdate(book, Buy, order.Price)
		} else {
			book.Asks.CancelOrder(orderID)
			e.publishBookUpdate(book, Sell, order.Price)
		}
		client.RemoveOrder(orderID)
		if gw != nil && gw.IsRunning() {
			select {
			case gw.ResponseCh <- Response{Success: true, Data: &ForcedCancelNotification{OrderID: orderID, RemainingQty: remainingQty}}:
			default:
			}
		}
	}
}
