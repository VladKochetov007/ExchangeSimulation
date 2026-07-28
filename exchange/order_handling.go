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
	} else {
		book.Asks.CancelOrder(req.OrderID)
	}
	// Hidden orders emit no public deltas: their placement was dark, so their
	// cancellation must be too.
	if order.Visibility != Hidden {
		e.publishBookUpdate(book, order.Side, order.Price)
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

	// The reference-data feed has no book: subscribe directly.
	if req.Symbol == InstrumentFeedSymbol {
		types := req.Types
		if len(types) == 0 {
			types = []MDType{MDInstrument}
		}
		e.MDPublisher.Subscribe(clientID, req.Symbol, types, gateway)
		return Response{RequestID: req.RequestID, Success: true}
	}

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
	// Venues enforce a positive display size on icebergs; IcebergQty ≤ 0
	// would silently degrade the order to fully-hidden semantics.
	if req.Visibility == Iceberg && req.IcebergQty <= 0 {
		return reject(RejectInvalidQty)
	}
	if reason := e.hedgeReduceViolation(clientID, book, req); reason != "" {
		return reject(reason)
	}
	return nil
}

// hedgeReduceViolation rejects hedge-mode reducing orders when the new
// quantity plus the client's already-resting reduce quantity would exceed the
// position (venue reduce-only semantics). Counting resting reduces preserves
// the invariant "resting reduce qty ≤ position size" through every fill, so
// a reduce can never overshoot at execution time and vanish quantity while
// the counterparty's fill stands.
func (e *DefaultExchange) hedgeReduceViolation(clientID uint64, book *OrderBook, req *OrderRequest) RejectReason {
	if req.PositionSide != PositionLong && req.PositionSide != PositionShort {
		return ""
	}
	reducing := (req.PositionSide == PositionLong && req.Side == Sell) ||
		(req.PositionSide == PositionShort && req.Side == Buy)
	if !reducing {
		return ""
	}
	var size int64
	if pos := e.Positions.GetPositionBySide(clientID, req.Symbol, req.PositionSide); pos != nil {
		size = abs(pos.Size)
	}
	resting := int64(0)
	side := book.Asks
	if req.Side == Buy {
		side = book.Bids
	}
	for _, order := range side.Orders {
		if order.ClientID == clientID && order.PositionSide == req.PositionSide {
			resting += order.Qty - order.FilledQty
		}
	}
	if req.Qty+resting > size {
		return RejectExceedsPosition
	}
	return ""
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
	if req.Visibility == Iceberg {
		order.DisplayRemaining = min(req.IcebergQty, req.Qty)
	}
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
	base, quote := instrument.BaseAsset(), instrument.QuoteAsset()
	if om, ok := instrument.(OrderMarginer); ok {
		refPrice := marketRefPrice(book)
		required := om.MarginForMarketOrder(order.Side, order.Qty, refPrice, precision)
		required += quoteFeeHeadroom(client.FeePlan, base, quote, order.Qty, refPrice, precision)
		return required == 0 || client.PerpAvailable(quote) >= required
	}
	if m, ok := instrument.(Margined); ok {
		refPrice := marketRefPrice(book)
		required := m.MarginForMarket(order.Qty, refPrice, precision)
		required += quoteFeeHeadroom(client.FeePlan, base, quote, order.Qty, refPrice, precision)
		return required == 0 || client.PerpAvailable(quote) >= required
	}
	if order.Side == Buy {
		cost := marketBuyCost(client.FeePlan, book, order, precision)
		return client.GetAvailable(instrument.QuoteAsset()) >= cost
	}
	// Sell must cover base-denominated fee headroom too, or the fee debit
	// drives the base balance negative.
	needed := spotOrderReservation(client.FeePlan, instrument, Sell, order.Qty, marketRefPrice(book), precision)
	return client.GetAvailable(instrument.BaseAsset()) >= needed
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
	base, quote := instrument.BaseAsset(), instrument.QuoteAsset()
	if om, ok := instrument.(OrderMarginer); ok {
		// Reserve margin AND the worst-case fee: the fee is debited from the same
		// quote wallet at settlement, so an order funded to the last cent of
		// margin would go insolvent the instant it fills. Reserving both here
		// lets the exchange reject the order up front instead.
		margin := om.MarginForOrder(order.Side, order.Qty, order.Price, precision)
		total := margin + quoteFeeHeadroom(client.FeePlan, base, quote, order.Qty, order.Price, precision)
		if !e.tryReserveOrBorrow(order.ClientID, quote, total, client.ReservePerp, true) {
			return false
		}
		order.Reserved = total
		return true
	}
	if m, ok := instrument.(Margined); ok {
		margin := m.MarginRequired(order.Qty, order.Price, precision)
		total := margin + quoteFeeHeadroom(client.FeePlan, base, quote, order.Qty, order.Price, precision)
		if !e.tryReserveOrBorrow(order.ClientID, quote, total, client.ReservePerp, true) {
			return false
		}
		order.Reserved = total
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

// checkForeignFeeFunds reports whether the client can cover the worst-case
// (taker) fee when it is denominated in an asset the reservation does not
// back. Quote fees are covered by the reservation's fee headroom; spot base
// fees net against the base leg the client receives. Everything else is
// unbacked — a third asset (a BNB fee on BTC/USD), or a base fee on a margined
// instrument, whose fill exchanges no base leg at all — and must be
// pre-checked here.
//
// This is a placement-time affordability check, not a lock: a client resting
// several orders could in principle over-commit the same foreign balance. That
// narrow case is left to the liquidation/settlement invariants; the common
// single-order path (and any taker) is fully covered.
func checkForeignFeeFunds(client *Client, book *OrderBook, order *Order, precision int64) bool {
	if client.FeePlan == nil || order.Qty <= 0 {
		return true
	}
	instrument := book.Instrument
	base, quote := instrument.BaseAsset(), instrument.QuoteAsset()
	price := order.Price
	if order.Type == Market {
		price = marketRefPrice(book)
	}
	probe := Execution{Price: price, Qty: order.Qty}
	fee := client.FeePlan.CalculateFee(FillContext{
		Exec:       &probe,
		IsMaker:    false,
		BaseAsset:  base,
		QuoteAsset: quote,
		Precision:  precision,
	})
	if fee.Amount <= 0 || fee.Asset == "" || fee.Asset == quote {
		return true
	}
	_, isMargined := instrument.(Margined)
	_, isOrderMargined := instrument.(OrderMarginer)
	if isMargined || isOrderMargined {
		// A margined fill exchanges no base leg: a base-denominated fee is just
		// as unbacked as any third asset, so it gets the same pre-check.
		return client.PerpAvailable(fee.Asset) >= fee.Amount
	}
	if fee.Asset == base {
		return true
	}
	return client.GetAvailable(fee.Asset) >= fee.Amount
}

// quoteFeeHeadroom returns the worst-case (taker) fee, in the quote asset, that
// a margined or premium order of qty at price would owe at settlement. Perp and
// option fees are debited from the quote (perp) wallet, so reserving this
// alongside margin is what lets the exchange reject an order the account cannot
// fully afford instead of debiting the fee past a zero balance after the fill.
// A fee in any other asset is caught separately by checkForeignFeeFunds.
func quoteFeeHeadroom(feePlan FeeModel, base, quote string, qty, price, precision int64) int64 {
	if feePlan == nil || qty <= 0 || price <= 0 {
		return 0
	}
	probe := Execution{Price: price, Qty: qty}
	fee := feePlan.CalculateFee(FillContext{
		Exec:       &probe,
		IsMaker:    false,
		BaseAsset:  base,
		QuoteAsset: quote,
		Precision:  precision,
	})
	if fee.Asset != quote || fee.Amount <= 0 {
		return 0
	}
	return fee.Amount
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
	// A fee charged in a foreign asset (neither base nor quote) has nothing
	// backing it: the reservation covers only the trade legs, and settlement
	// would drive the foreign balance negative. Reject up front, before locking
	// any funds, when the client cannot cover the worst-case fee.
	if !checkForeignFeeFunds(client, book, order, precision) {
		resp := e.rejectOrder(order, requestID, order.ClientID, RejectInsufficientBalance, log)
		return &resp
	}
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
		e.cancelOwnCrossingQuotes(client, book, order)
		if order.Side == Buy {
			book.Bids.AddOrder(order)
			if order.Visibility != Hidden {
				e.publishBookUpdate(book, Buy, order.Price)
			}
		} else {
			book.Asks.AddOrder(order)
			if order.Visibility != Hidden {
				e.publishBookUpdate(book, Sell, order.Price)
			}
		}
		client.AddOrder(order.ID)
	} else {
		releaseReserved(client, book.Instrument, order)
		putOrder(order)
	}
}

// cancelOwnCrossingQuotes implements self-trade prevention with cancel-maker
// semantics: the matcher consumed every crossable order from OTHER clients,
// so any price still crossing the remainder belongs to this client. Resting
// it as-is would display a crossed/locked book; cancelling the stale opposite
// quote (the venue "cancel maker" STP mode) keeps the book valid while
// letting the client flip direction through their own level.
// Caller must hold e.mu.Lock().
func (e *DefaultExchange) cancelOwnCrossingQuotes(client *Client, book *OrderBook, order *Order) {
	opposite := book.Asks
	if order.Side == Sell {
		opposite = book.Bids
	}
	var targets []*Order
	for _, o := range opposite.Orders {
		// Parent == nil means fully filled and awaiting settlement cleanup.
		if o.ClientID != client.ID || o.Parent == nil {
			continue
		}
		if order.Side == Buy && o.Price > order.Price {
			continue
		}
		if order.Side == Sell && o.Price < order.Price {
			continue
		}
		targets = append(targets, o)
	}
	gw := e.Gateways[client.ID]
	for _, o := range targets {
		remainingQty := o.Qty - o.FilledQty
		orderID := o.ID
		visibility := o.Visibility
		side, price := o.Side, o.Price
		releaseReserved(client, book.Instrument, o)
		opposite.CancelOrder(orderID)
		if visibility != Hidden {
			e.publishBookUpdate(book, side, price)
		}
		client.RemoveOrder(orderID)
		putOrder(o)
		if gw != nil && gw.IsRunning() {
			sendResponse(gw, Response{Success: true, Data: &ForcedCancelNotification{OrderID: orderID, RemainingQty: remainingQty}})
		}
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
	_, margined := instrument.(Margined)
	_, orderMargined := instrument.(OrderMarginer)
	if margined || orderMargined {
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
