package exchange

func (e *DefaultExchange) PlaceOrder(clientID uint64, req *OrderRequest) Response {
	e.mu.Lock()
	defer e.mu.Unlock()

	if reject := e.validatePlaceOrder(clientID, req); reject != nil {
		return *reject
	}

	book, client, log := e.Books[req.Symbol], e.Clients[clientID], e.getLogger(req.Symbol)

	e.NextOrderID++
	order := newOrderFromRequest(clientID, e.NextOrderID, req, e.Clock.NowUnixNano())

	// FOK must be checked BEFORE matching: Match mutates the book, and
	// abandoning its executions would strip maker quantity without settlement.
	if req.TimeInForce == FOK && !canFillFully(book, order) {
		return e.rejectOrder(order, req.RequestID, clientID, RejectFOKNotFilled, log)
	}

	if reject := e.reserveOrderFunds(client, book, order, req.RequestID, log); reject != nil {
		return *reject
	}

	if log != nil {
		log.LogEvent(e.Clock.NowUnixNano(), clientID, "OrderAccepted", order)
	}

	result := e.Matcher.Match(book.Bids, book.Asks, order)
	if req.TimeInForce == FOK && !result.FullyFilled {
		// Safety net for custom matchers whose fill rules diverge from the
		// canFillFully probe. Executions already applied cannot be rolled back.
		releaseReserved(client, book.Instrument, order)
		return e.rejectOrder(order, req.RequestID, clientID, RejectFOKNotFilled, log)
	}

	levels := collectAffectedLevels(book, result.Executions)
	e.processExecutions(book, result.Executions, order)
	e.removeMakerOrders(book, result.Executions)
	e.publishLevels(book, levels)
	e.restOrReleaseOrder(client, book, order, req)

	return Response{RequestID: req.RequestID, Success: true, Data: e.NextOrderID}
}

func (e *DefaultExchange) CancelOrder(clientID uint64, req *CancelRequest) Response {
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

	remainingQty := order.Qty - order.FilledQty
	releaseReserved(client, book.Instrument, order)
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

func (e *DefaultExchange) QueryAccount(clientID uint64, req *QueryRequest) Response {
	e.mu.RLock()
	defer e.mu.RUnlock()

	client := e.Clients[clientID]
	if client == nil {
		return Response{RequestID: req.RequestID, Success: false, Error: RejectUnknownClient}
	}

	timestamp := e.Clock.NowUnixNano()
	snap := client.GetBalanceSnapshot(timestamp)
	positions := e.buildPositionSnapshots(clientID)
	return Response{RequestID: req.RequestID, Success: true, Data: &AccountSnapshot{BalanceSnapshot: *snap, Positions: positions}}
}

func (e *DefaultExchange) buildPositionSnapshots(clientID uint64) []PositionSnapshot {
	positions := e.Positions.GetAllPositions(clientID)
	if len(positions) == 0 {
		return nil
	}
	snapshots := make([]PositionSnapshot, 0, len(positions))
	for _, pos := range positions {
		book := e.Books[pos.Symbol]
		var markPrice int64
		if book != nil {
			markPrice = marketRefPrice(book)
		}
		var unrealizedPnL int64
		if markPrice > 0 && pos.EntryPrice > 0 {
			if instrument := e.Instruments[pos.Symbol]; instrument != nil {
				unrealizedPnL = positionUPnL(&pos, markPrice, instrument.BasePrecision())
			}
		}
		snapshots = append(snapshots, PositionSnapshot{
			Symbol:        pos.Symbol,
			PositionSide:  pos.PositionSide,
			Size:          pos.Size,
			EntryPrice:    pos.EntryPrice,
			MarkPrice:     markPrice,
			UnrealizedPnL: unrealizedPnL,
			MarginType:    CrossMargin,
		})
	}
	return snapshots
}

func (e *DefaultExchange) QueryBalance(clientID uint64, req *QueryRequest) Response {
	e.mu.RLock()
	defer e.mu.RUnlock()

	client := e.Clients[clientID]
	if client == nil {
		return Response{RequestID: req.RequestID, Success: false, Error: RejectUnknownClient}
	}

	snapshot := client.GetBalanceSnapshot(e.Clock.NowUnixNano())
	return Response{RequestID: req.RequestID, Success: true, Data: snapshot}
}

func (e *DefaultExchange) Subscribe(clientID uint64, req *QueryRequest, gateway *ClientGateway) Response {
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

	e.MDPublisher.Publish(req.Symbol, MDSnapshot, &BookSnapshot{
		Bids: book.Bids.GetPublicSnapshot(),
		Asks: book.Asks.GetPublicSnapshot(),
	}, e.Clock.NowUnixNano())

	if log := e.Loggers[req.Symbol]; log != nil {
		log.LogEvent(e.Clock.NowUnixNano(), clientID, "BookSnapshot", map[string]any{
			"bids": book.Bids.GetSnapshot(),
			"asks": book.Asks.GetSnapshot(),
		})
	}

	return Response{RequestID: req.RequestID, Success: true}
}

func (e *DefaultExchange) Unsubscribe(clientID uint64, req *QueryRequest) Response {
	e.MDPublisher.Unsubscribe(clientID, req.Symbol)
	return Response{RequestID: req.RequestID, Success: true}
}

// publishBookUpdate publishes a delta update for a specific price level.
// Caller must hold e.mu lock.
func (e *DefaultExchange) publishBookUpdate(book *OrderBook, side Side, price int64) {
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

	// Public deltas carry displayed quantity only; hidden depth stays dark.
	delta := &BookDelta{
		Side:       side,
		Price:      price,
		VisibleQty: visible,
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
func (e *DefaultExchange) validatePlaceOrder(clientID uint64, req *OrderRequest) *Response {
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

// checkMarketOrderFunds verifies the client can afford the worst-case cost of
// a market order by walking the opposing book (best-price × full qty would
// understate the spend on a deep walk and allow overdrafts).
func checkMarketOrderFunds(client *Client, book *OrderBook, order *Order, precision int64) bool {
	instrument := book.Instrument
	if m, ok := instrument.(Margined); ok {
		refPrice := marketRefPrice(book)
		required := m.MarginForMarket(order.Qty, refPrice, precision)
		return required == 0 || client.PerpAvailable(instrument.QuoteAsset()) >= required
	}
	if order.Side == Buy {
		cost := marketBuyCost(client.FeePlan, book, order, precision)
		return client.GetAvailable(instrument.QuoteAsset()) >= cost
	}
	return client.GetAvailable(instrument.BaseAsset()) >= order.Qty
}

// marketBuyCost walks the ask book and sums the quote spend (plus quote fees)
// for filling order.Qty, skipping the client's own resting orders.
func marketBuyCost(feePlan FeeModel, book *OrderBook, order *Order, precision int64) int64 {
	instrument := book.Instrument
	remaining := order.Qty
	cost := int64(0)
	for limit := book.Asks.Best; limit != nil && remaining > 0; limit = limit.Next {
		levelQty := int64(0)
		for o := limit.Head; o != nil && levelQty < remaining; o = o.Next {
			if o.ClientID == order.ClientID {
				continue
			}
			levelQty += o.Qty - o.FilledQty
		}
		fillQty := min(levelQty, remaining)
		if fillQty <= 0 {
			continue
		}
		cost += spotOrderReservation(feePlan, instrument, Buy, fillQty, limit.Price, precision)
		remaining -= fillQty
	}
	return cost
}

func (e *DefaultExchange) reserveLimitOrderFunds(client *Client, instrument Instrument, order *Order, precision int64) bool {
	if m, ok := instrument.(Margined); ok {
		margin := m.MarginRequired(order.Qty, order.Price, precision)
		if !e.tryReserveOrBorrow(order.ClientID, instrument.QuoteAsset(), margin, client.ReservePerp, true) {
			return false
		}
		order.Reserved = margin
		return true
	}
	asset := reserveAsset(instrument, order.Side)
	amount := spotOrderReservation(client.FeePlan, instrument, order.Side, order.Qty, order.Price, precision)
	if !e.tryReserveOrBorrow(order.ClientID, asset, amount, client.Reserve, false) {
		return false
	}
	order.Reserved = amount
	return true
}

func reserveAsset(instrument Instrument, side Side) string {
	if side == Buy {
		return instrument.QuoteAsset()
	}
	return instrument.BaseAsset()
}

// spotOrderReservation returns the reserve-asset amount to lock for qty at
// price: the notional (buy) or base qty (sell), plus worst-case taker fee
// headroom when the fee is charged in the reserve asset. Without headroom,
// fees debit past the reservation and drive balances negative.
func spotOrderReservation(feePlan FeeModel, instrument Instrument, side Side, qty, price, precision int64) int64 {
	var amount int64
	if side == Buy {
		amount = MulDiv(qty, price, precision)
	} else {
		amount = qty
	}
	if qty <= 0 || feePlan == nil {
		return max(amount, 0)
	}
	probe := Execution{Price: price, Qty: qty}
	fee := feePlan.CalculateFee(FillContext{
		Exec:       &probe,
		IsMaker:    false,
		BaseAsset:  instrument.BaseAsset(),
		QuoteAsset: instrument.QuoteAsset(),
		Precision:  precision,
	})
	if fee.Asset == reserveAsset(instrument, side) {
		amount += fee.Amount
	}
	return amount
}

// canFillFully reports whether the opposing book holds enough crossable
// quantity from other clients to fill the order completely.
func canFillFully(book *OrderBook, order *Order) bool {
	side := book.Asks
	if order.Side == Sell {
		side = book.Bids
	}
	remaining := order.Qty
	for limit := side.Best; limit != nil && remaining > 0; limit = limit.Next {
		if order.Type == LimitOrder {
			if order.Side == Buy && limit.Price > order.Price {
				break
			}
			if order.Side == Sell && limit.Price < order.Price {
				break
			}
		}
		for o := limit.Head; o != nil && remaining > 0; o = o.Next {
			if o.ClientID == order.ClientID {
				continue
			}
			remaining -= o.Qty - o.FilledQty
		}
	}
	return remaining <= 0
}

// reserveOrderFunds checks or reserves funds depending on order type.
// Returns a rejection Response if funds are insufficient, nil otherwise.
// Caller must hold e.mu.Lock().
func (e *DefaultExchange) reserveOrderFunds(client *Client, book *OrderBook, order *Order, requestID uint64, log Logger) *Response {
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
		} else {
			// Fully filled order was removed from book.Orders by the matcher;
			// exec carries the price and side directly.
			levels[exec.Price] = exec.MakerSide
		}
	}
	return levels
}

// removeMakerOrders removes fully filled maker orders from the book index.
// The matcher unlinked them from their price level but left them in
// book.Orders so settlement could read the reservation ledger.
// Caller must hold e.mu.Lock().
func (e *DefaultExchange) removeMakerOrders(book *OrderBook, executions []*Execution) {
	for _, exec := range executions {
		if exec.MakerFilledQty < exec.MakerTotalQty {
			continue
		}
		side := book.Asks
		if exec.MakerSide == Buy {
			side = book.Bids
		}
		if makerOrder := side.RemoveFilledOrder(exec.MakerOrderID); makerOrder != nil {
			putOrder(makerOrder)
		}
		e.Clients[exec.MakerClientID].RemoveOrder(exec.MakerOrderID)
	}
}

func (e *DefaultExchange) publishLevels(book *OrderBook, levels map[int64]Side) {
	for price, side := range levels {
		e.publishBookUpdate(book, side, price)
	}
}

// restOrReleaseOrder either rests the order as a GTC limit in the book or releases its funds.
// Caller must hold e.mu.Lock().
func (e *DefaultExchange) restOrReleaseOrder(client *Client, book *OrderBook, order *Order, req *OrderRequest) {
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
		releaseReserved(client, book.Instrument, order)
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
func (e *DefaultExchange) rejectOrder(order *Order, requestID uint64, clientID uint64, reason RejectReason, log Logger) Response {
	putOrder(order)
	resp := Response{RequestID: requestID, Success: false, Error: reason}
	if log != nil {
		log.LogEvent(e.Clock.NowUnixNano(), clientID, "OrderRejected", resp)
	}
	return resp
}

// releaseReserved returns an order's remaining reserved funds to the client
// and zeroes the ledger. Exact by construction: it releases what was locked,
// not a recomputed approximation.
func releaseReserved(client *Client, instrument Instrument, order *Order) {
	if order.Reserved <= 0 {
		return
	}
	if _, ok := instrument.(Margined); ok {
		client.ReleasePerp(instrument.QuoteAsset(), order.Reserved)
	} else {
		client.Release(reserveAsset(instrument, order.Side), order.Reserved)
	}
	order.Reserved = 0
}

// tryReserveOrBorrow attempts reserveFn; on failure, if BorrowingMgr is configured
// it borrows the shortfall inline and retries. Caller must hold e.mu.Lock().
// No unlock/relock needed — BorrowingManager no longer acquires the exchange lock.
func (e *DefaultExchange) tryReserveOrBorrow(
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
	cfg := e.BorrowingMgr.Config
	if isPerp && !cfg.AutoBorrowPerp {
		return false
	}
	if !isPerp && !cfg.AutoBorrowSpot {
		return false
	}
	client := e.Clients[clientID]
	if client == nil {
		return false
	}
	var available int64
	if isPerp {
		available = client.PerpAvailable(asset)
	} else {
		available = client.GetAvailable(asset)
	}
	if available >= amount {
		return false
	}
	reason := "auto_spot"
	if isPerp {
		reason = "auto_perp"
	}
	ctx := buildBorrowContext(e, client, clientID)
	ctx.CreditSpot = !isPerp
	if err := e.BorrowingMgr.BorrowMargin(ctx, asset, amount-available, reason); err != nil {
		return false
	}
	return reserveFn(asset, amount)
}
