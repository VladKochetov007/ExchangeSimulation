package exchange

func (e *Exchange) placeOrder(clientID uint64, req *OrderRequest) Response {
	e.mu.Lock()
	defer e.mu.Unlock()

	if reject := e.validatePlaceOrder(clientID, req); reject != nil {
		return *reject
	}

	book, client, log := e.Books[req.Symbol], e.Clients[clientID], e.getLogger(req.Symbol)

	e.NextOrderID++
	order := newOrderFromRequest(clientID, e.NextOrderID, req, e.Clock.NowUnixNano())
	if reject := e.reserveOrderFunds(client, book, order, req.RequestID, log); reject != nil {
		return *reject
	}

	if log != nil {
		log.LogEvent(e.Clock.NowUnixNano(), clientID, "OrderAccepted", order)
	}

	result := e.Matcher.Match(book.Bids, book.Asks, order)
	if req.TimeInForce == FOK && !result.FullyFilled {
		if req.Type == LimitOrder {
			releaseOrderFunds(client, book.Instrument, req.Side, req.Qty, req.Price)
		}
		return e.rejectOrder(order, req.RequestID, clientID, RejectFOKNotFilled, log)
	}

	levels := collectAffectedLevels(book, result.Executions)
	e.processExecutions(book, result.Executions, order)
	e.removeMakerOrders(book, result.Executions)
	e.publishLevels(book, levels)
	e.restOrReleaseOrder(client, book, order, req)

	return Response{RequestID: req.RequestID, Success: true, Data: e.NextOrderID}
}

func (e *Exchange) cancelOrder(clientID uint64, req *CancelRequest) Response {
	e.mu.Lock()
	defer e.mu.Unlock()

	client := e.Clients[clientID]
	if client == nil {
		return Response{RequestID: req.RequestID, Success: false, Error: RejectUnknownClient}
	}

	var order *Order
	var book *OrderBook
	for _, b := range e.Books {
		if o := b.FindOrder(req.OrderID); o != nil {
			order = o
			book = b
			break
		}
	}

	if order == nil {
		return Response{RequestID: req.RequestID, Success: false, Error: RejectOrderNotFound}
	}

	log := e.getLogger(book.Symbol)

	if order.ClientID != clientID {
		resp := Response{RequestID: req.RequestID, Success: false, Error: RejectOrderNotOwned}
		if log != nil {
			log.LogEvent(e.Clock.NowUnixNano(), clientID, "OrderCancelRejected", resp)
		}
		return resp
	}
	if order.Status == Filled {
		resp := Response{RequestID: req.RequestID, Success: false, Error: RejectOrderAlreadyFilled}
		if log != nil {
			log.LogEvent(e.Clock.NowUnixNano(), clientID, "OrderCancelRejected", resp)
		}
		return resp
	}

	instrument := book.Instrument
	remainingQty := order.Qty - order.FilledQty
	releaseOrderFunds(client, instrument, order.Side, remainingQty, order.Price)
	if order.Side == Buy {
		book.Bids.CancelOrder(req.OrderID)
		e.publishBookUpdate(book, Buy, order.Price)
	} else {
		book.Asks.CancelOrder(req.OrderID)
		e.publishBookUpdate(book, Sell, order.Price)
	}

	client.RemoveOrder(req.OrderID)
	order.Status = Cancelled
	putOrder(order)

	if log != nil {
		cancelEvent := map[string]any{
			"order_id":      req.OrderID,
			"request_id":    req.RequestID,
			"remaining_qty": remainingQty,
		}
		log.LogEvent(e.Clock.NowUnixNano(), clientID, "OrderCancelled", cancelEvent)
	}

	return Response{RequestID: req.RequestID, Success: true, Data: remainingQty}
}

func (e *Exchange) queryAccount(clientID uint64, req *QueryRequest) Response {
	e.mu.RLock()
	defer e.mu.RUnlock()

	client := e.Clients[clientID]
	if client == nil {
		return Response{RequestID: req.RequestID, Success: false, Error: RejectUnknownClient}
	}

	timestamp := e.Clock.NowUnixNano()
	snap := client.GetBalanceSnapshot(timestamp)
	positions := e.buildPositionSnapshots(clientID, timestamp)
	return Response{RequestID: req.RequestID, Success: true, Data: &AccountSnapshot{BalanceSnapshot: *snap, Positions: positions}}
}

func (e *Exchange) buildPositionSnapshots(clientID uint64, timestamp int64) []PositionSnapshot {
	e.Positions.mu.RLock()
	defer e.Positions.mu.RUnlock()

	clientPositions := e.Positions.positions[clientID]
	if len(clientPositions) == 0 {
		return nil
	}

	snapshots := make([]PositionSnapshot, 0, len(clientPositions))
	for key, pos := range clientPositions {
		if pos.Size == 0 {
			continue
		}
		book := e.Books[key.Symbol]
		var markPrice int64
		if book != nil {
			markPrice = marketRefPrice(book)
		}
		var unrealizedPnL int64
		if markPrice > 0 && pos.EntryPrice > 0 {
			instrument := e.Instruments[key.Symbol]
			if instrument != nil {
				precision := instrument.BasePrecision()
				sign := int64(1)
				if pos.Size < 0 {
					sign = -1
				}
				unrealizedPnL = abs(pos.Size) * sign * (markPrice - pos.EntryPrice) / precision
			}
		}
		snapshots = append(snapshots, PositionSnapshot{
			Symbol:        key.Symbol,
			PositionSide:  key.Side,
			Size:          pos.Size,
			EntryPrice:    pos.EntryPrice,
			MarkPrice:     markPrice,
			UnrealizedPnL: unrealizedPnL,
			MarginType:    CrossMargin,
		})
	}
	return snapshots
}

func (e *Exchange) queryBalance(clientID uint64, req *QueryRequest) Response {
	e.mu.RLock()
	defer e.mu.RUnlock()

	client := e.Clients[clientID]
	if client == nil {
		return Response{RequestID: req.RequestID, Success: false, Error: RejectUnknownClient}
	}

	snapshot := client.GetBalanceSnapshot(e.Clock.NowUnixNano())
	return Response{RequestID: req.RequestID, Success: true, Data: snapshot}
}

func (e *Exchange) subscribe(clientID uint64, req *QueryRequest, gateway *ClientGateway) Response {
	e.mu.RLock()
	defer e.mu.RUnlock()

	book := e.Books[req.Symbol]
	if book == nil {
		return Response{RequestID: req.RequestID, Success: false, Error: RejectUnknownInstrument}
	}

	types := req.Types
	if len(types) == 0 {
		types = []MDType{MDSnapshot, MDDelta, MDTrade}
	}
	e.MDPublisher.Subscribe(clientID, req.Symbol, types, gateway)

	snapshot := &BookSnapshot{
		Bids: book.Bids.GetSnapshot(),
		Asks: book.Asks.GetSnapshot(),
	}
	e.MDPublisher.Publish(req.Symbol, MDSnapshot, snapshot, e.Clock.NowUnixNano())

	if log := e.Loggers[req.Symbol]; log != nil {
		snapshotLog := map[string]any{
			"bids": snapshot.Bids,
			"asks": snapshot.Asks,
		}
		log.LogEvent(e.Clock.NowUnixNano(), clientID, "BookSnapshot", snapshotLog)
	}

	return Response{RequestID: req.RequestID, Success: true}
}

func (e *Exchange) unsubscribe(clientID uint64, req *QueryRequest) Response {
	e.MDPublisher.Unsubscribe(clientID, req.Symbol)
	return Response{RequestID: req.RequestID, Success: true}
}

// publishBookUpdate publishes a delta update for a specific price level.
// Caller must hold e.mu lock.
func (e *Exchange) publishBookUpdate(book *OrderBook, side Side, price int64) {
	var limit *Limit
	if side == Buy {
		limit = book.Bids.Limits[price]
	} else {
		limit = book.Asks.Limits[price]
	}

	var totalQty, visible, hidden int64
	if limit != nil {
		totalQty = limit.TotalQty
		visible = visibleQty(limit)
		hidden = totalQty - visible
	}

	delta := &BookDelta{
		Side:       side,
		Price:      price,
		VisibleQty: visible,
		HiddenQty:  hidden,
	}
	e.MDPublisher.Publish(book.Symbol, MDDelta, delta, e.Clock.NowUnixNano())

	if log := e.Loggers[book.Symbol]; log != nil {
		deltaLog := map[string]any{
			"side":        side.String(),
			"price":       price,
			"visible_qty": visible,
			"hidden_qty":  hidden,
			"total_qty":   totalQty,
		}
		log.LogEvent(e.Clock.NowUnixNano(), 0, "BookDelta", deltaLog)
	}
}

// validatePlaceOrder runs early guards for gateway, client, instrument, and price/qty.
// Caller must hold e.mu.Lock().
func (e *Exchange) validatePlaceOrder(clientID uint64, req *OrderRequest) *Response {
	// Close the TOCTOU window: gateway IsRunning is checked here under e.mu to
	// prevent races with CancelAllClientOrders during shutdown.
	if gw := e.Gateways[clientID]; gw != nil && !gw.IsRunning() {
		resp := Response{RequestID: req.RequestID, Success: false}
		return &resp
	}
	log := e.getLogger(req.Symbol)
	reject := func(reason RejectReason) *Response {
		resp := rejectWithLog(req.RequestID, clientID, reason, log, e.Clock)
		return &resp
	}
	if e.Clients[clientID] == nil {
		return reject(RejectUnknownClient)
	}
	book := e.Books[req.Symbol]
	if book == nil {
		return reject(RejectUnknownInstrument)
	}
	if req.Type == LimitOrder && !book.Instrument.ValidatePrice(req.Price) {
		return reject(RejectInvalidPrice)
	}
	if !book.Instrument.ValidateQty(req.Qty) {
		return reject(RejectInvalidQty)
	}
	return nil
}

func newOrderFromRequest(clientID, orderID uint64, req *OrderRequest, timestamp int64) *Order {
	order := getOrder()
	order.ID = orderID
	order.ClientID = clientID
	order.Side = req.Side
	order.PositionSide = req.PositionSide
	order.Type = req.Type
	order.TimeInForce = req.TimeInForce
	order.Price = req.Price
	order.Qty = req.Qty
	order.Visibility = req.Visibility
	order.IcebergQty = req.IcebergQty
	order.Status = Open
	order.Timestamp = timestamp
	return order
}

// marketRefPrice returns the best available reference price for margin estimation,
// falling back from mid to one-sided best bid/ask.
func marketRefPrice(book *OrderBook) int64 {
	if mid := book.GetMidPrice(); mid > 0 {
		return mid
	}
	if book.Asks.Best != nil {
		return book.Asks.Best.Price
	}
	if book.Bids.Best != nil {
		return book.Bids.Best.Price
	}
	return 0
}

func checkMarketOrderFunds(client *Client, book *OrderBook, order *Order, precision int64) bool {
	instrument := book.Instrument
	if instrument.IsPerp() {
		perp := instrument.(*PerpFutures)
		refPrice := marketRefPrice(book)
		return refPrice == 0 || client.PerpAvailable(instrument.QuoteAsset()) >= calcMargin(order.Qty, refPrice, perp.MarginRate, precision)
	}
	if order.Side == Buy {
		if book.Asks.Best == nil {
			return true
		}
		return client.GetAvailable(instrument.QuoteAsset()) >= (order.Qty*book.Asks.Best.Price)/precision
	}
	return client.GetAvailable(instrument.BaseAsset()) >= order.Qty
}

func (e *Exchange) reserveLimitOrderFunds(client *Client, instrument Instrument, order *Order, precision int64) bool {
	if instrument.IsPerp() {
		perp := instrument.(*PerpFutures)
		margin := calcMargin(order.Qty, order.Price, perp.MarginRate, precision)
		return e.tryReserveOrBorrow(order.ClientID, instrument.QuoteAsset(), margin, client.ReservePerp, true)
	}
	if order.Side == Buy {
		amount := (order.Qty * order.Price) / precision
		return e.tryReserveOrBorrow(order.ClientID, instrument.QuoteAsset(), amount, client.Reserve, false)
	}
	return e.tryReserveOrBorrow(order.ClientID, instrument.BaseAsset(), order.Qty, client.Reserve, false)
}

// reserveOrderFunds checks or reserves funds depending on order type.
// Returns a rejection Response if funds are insufficient, nil otherwise.
// Caller must hold e.mu.Lock().
func (e *Exchange) reserveOrderFunds(client *Client, book *OrderBook, order *Order, requestID uint64, log Logger) *Response {
	precision := book.Instrument.BasePrecision()
	var ok bool
	switch order.Type {
	case Market:
		ok = checkMarketOrderFunds(client, book, order, precision)
	case LimitOrder:
		ok = e.reserveLimitOrderFunds(client, book.Instrument, order, precision)
	default:
		return nil
	}
	if !ok {
		resp := e.rejectOrder(order, requestID, order.ClientID, RejectInsufficientBalance, log)
		return &resp
	}
	return nil
}

func collectAffectedLevels(book *OrderBook, executions []*Execution) map[int64]Side {
	levels := make(map[int64]Side, len(executions))
	for _, exec := range executions {
		if makerOrder := book.FindOrder(exec.MakerOrderID); makerOrder != nil {
			levels[makerOrder.Price] = makerOrder.Side
		}
	}
	return levels
}

// removeMakerOrders removes fully filled maker orders from the book.
// Caller must hold e.mu.Lock().
func (e *Exchange) removeMakerOrders(book *OrderBook, executions []*Execution) {
	for _, exec := range executions {
		makerOrder := book.FindOrder(exec.MakerOrderID)
		if makerOrder == nil || makerOrder.Status != Filled {
			continue
		}
		if makerOrder.Side == Buy {
			book.Bids.CancelOrder(exec.MakerOrderID)
		} else {
			book.Asks.CancelOrder(exec.MakerOrderID)
		}
		e.Clients[exec.MakerClientID].RemoveOrder(exec.MakerOrderID)
		putOrder(makerOrder)
	}
}

func (e *Exchange) publishLevels(book *OrderBook, levels map[int64]Side) {
	for price, side := range levels {
		e.publishBookUpdate(book, side, price)
	}
}

// restOrReleaseOrder either rests the order as a GTC limit in the book or releases its funds.
// Caller must hold e.mu.Lock().
func (e *Exchange) restOrReleaseOrder(client *Client, book *OrderBook, order *Order, req *OrderRequest) {
	if order.Status != Filled && req.Type == LimitOrder && req.TimeInForce == GTC {
		if order.Side == Buy {
			book.Bids.AddOrder(order)
			e.publishBookUpdate(book, Buy, order.Price)
		} else {
			book.Asks.AddOrder(order)
			e.publishBookUpdate(book, Sell, order.Price)
		}
		client.AddOrder(order.ID)
	} else {
		releaseOrderFunds(client, book.Instrument, order.Side, order.Qty-order.FilledQty, order.Price)
		putOrder(order)
	}
}

// rejectWithLog builds a failed Response and optionally logs it.
func rejectWithLog(requestID uint64, clientID uint64, reason RejectReason, log Logger, clock Clock) Response {
	resp := Response{RequestID: requestID, Success: false, Error: reason}
	if log != nil {
		log.LogEvent(clock.NowUnixNano(), clientID, "OrderRejected", resp)
	}
	return resp
}

// rejectOrder recycles the order and returns a logged rejection Response.
// Caller must hold e.mu.Lock().
func (e *Exchange) rejectOrder(order *Order, requestID uint64, clientID uint64, reason RejectReason, log Logger) Response {
	putOrder(order)
	resp := Response{RequestID: requestID, Success: false, Error: reason}
	if log != nil {
		log.LogEvent(e.Clock.NowUnixNano(), clientID, "OrderRejected", resp)
	}
	return resp
}

// releaseOrderFunds releases the reserved balance for an order (or partial qty).
func releaseOrderFunds(client *Client, instrument Instrument, side Side, qty, price int64) {
	if qty <= 0 {
		return
	}
	precision := instrument.BasePrecision()
	if instrument.IsPerp() {
		perp := instrument.(*PerpFutures)
		margin := calcMargin(qty, price, perp.MarginRate, precision)
		client.ReleasePerp(instrument.QuoteAsset(), margin)
	} else if side == Buy {
		client.Release(instrument.QuoteAsset(), (qty*price)/precision)
	} else {
		client.Release(instrument.BaseAsset(), qty)
	}
}

// tryReserveOrBorrow attempts reserveFn; on failure, if BorrowingMgr is configured it
// temporarily releases e.mu, auto-borrows, reacquires, then retries the reservation.
// Caller must hold e.mu.Lock().
func (e *Exchange) tryReserveOrBorrow(
	clientID uint64, asset string, amount int64,
	reserveFn func(string, int64) bool,
	isPerp bool,
) bool {
	if reserveFn(asset, amount) {
		return true
	}
	if e.BorrowingMgr == nil {
		return false
	}
	e.mu.Unlock()
	var borrowed bool
	if isPerp {
		borrowed, _ = e.BorrowingMgr.AutoBorrowForPerpTrade(clientID, asset, amount)
	} else {
		borrowed, _ = e.BorrowingMgr.AutoBorrowForSpotTrade(clientID, asset, amount)
	}
	e.mu.Lock()
	return borrowed && reserveFn(asset, amount)
}
