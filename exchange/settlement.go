package exchange

func (e *DefaultExchange) processExecutions(book *OrderBook, executions []*Execution, takerOrder *Order) {
	instrument := book.Instrument
	timestamp := e.Clock.NowUnixNano()
	basePrecision := instrument.BasePrecision()
	log := e.getLogger(book.Symbol)
	positionChanged := false
	for _, exec := range executions {
		if e.handleExecution(book, exec, takerOrder, instrument, basePrecision, timestamp, log) {
			positionChanged = true
		}
	}
	if positionChanged {
		e.publishOpenInterest(book, timestamp)
	}
}

// handleExecution processes one matched pair: settle, update volumes, record trade, notify.
// Returns true if a position changed (for OI tracking).
func (e *DefaultExchange) handleExecution(
	book *OrderBook, exec *Execution, takerOrder *Order,
	instrument Instrument, basePrecision, timestamp int64, log Logger,
) bool {
	taker := e.Clients[exec.TakerClientID]
	maker := e.Clients[exec.MakerClientID]
	baseAsset, quoteAsset := instrument.BaseAsset(), instrument.QuoteAsset()
	takerFee := taker.FeePlan.CalculateFee(FillContext{Exec: exec, IsMaker: false, BaseAsset: baseAsset, QuoteAsset: quoteAsset, Precision: basePrecision})
	makerFee := maker.FeePlan.CalculateFee(FillContext{Exec: exec, IsMaker: true, BaseAsset: baseAsset, QuoteAsset: quoteAsset, Precision: basePrecision})

	// Fully filled makers stay in book.Orders until removeMakerOrders, so this
	// lookup normally succeeds; exec carries the fallbacks for custom matchers.
	makerOrder := book.FindOrder(exec.MakerOrderID)
	makerPosSide := exec.MakerPosSide
	if makerOrder != nil {
		makerPosSide = makerOrder.PositionSide
	}

	var result SettlementResult
	var positionChanged bool
	if s, ok := instrument.(Settleable); ok {
		result = s.Settle(e.buildSettlementContext(book, exec, takerOrder, makerOrder, makerPosSide, takerFee, makerFee, basePrecision, timestamp, log))
		positionChanged = true
	} else {
		notional := MulDiv(exec.Qty, exec.Price, basePrecision)
		e.settleSpotExecution(book, exec, takerOrder, makerOrder, taker, maker, takerFee, makerFee, notional, timestamp)
	}
	tradeID := e.createTrade(book, exec, takerOrder, timestamp, log)
	e.notifyFill(exec, takerOrder, makerPosSide, takerFee, makerFee, tradeID, book, log, timestamp, result.TakerDelta, result.MakerDelta, result.TakerPnL, result.MakerPnL)
	return positionChanged
}

func (e *DefaultExchange) buildSettlementContext(
	book *OrderBook, exec *Execution, takerOrder, makerOrder *Order,
	makerPosSide PositionSide, takerFee, makerFee Fee,
	basePrecision, timestamp int64, log Logger,
) SettlementContext {
	clients := e.Clients
	return SettlementContext{
		Exec:              exec,
		TakerOrder:        takerOrder,
		MakerOrder:        makerOrder,
		MakerPosSide:      makerPosSide,
		TakerFee:          takerFee,
		MakerFee:          makerFee,
		Positions:         e.Positions,
		PerpBalance:       func(clientID uint64, asset string) int64 { return clients[clientID].PerpBalance(asset) },
		MutatePerpBalance: func(clientID uint64, asset string, delta int64) { clients[clientID].MutatePerpBalance(asset, delta) },
		// Post-trade margin is owed once the fill happened: force-reserve even
		// past available so the shortfall is visible to the liquidation sweep.
		ReservePerp: func(clientID uint64, asset string, amount int64) bool {
			clients[clientID].ForceReservePerp(asset, amount)
			return true
		},
		ReleasePerp: func(clientID uint64, asset string, amount int64) { clients[clientID].ReleasePerp(asset, amount) },
		RecordFeeRevenue: func(asset string, takerAmt, makerAmt int64) {
			e.recordFeeRevenue(asset, Fee{Amount: takerAmt}, Fee{Amount: makerAmt}, book, timestamp)
		},
		LogBalanceChange: func(clientID uint64, symbol, reason string, deltas []BalanceDelta) {
			logBalanceChange(e, timestamp, clientID, symbol, reason, deltas)
		},
		Log:           log,
		GlobalLog:     e.getLogger("_global"),
		BasePrecision: basePrecision,
		Timestamp:     timestamp,
		BookSymbol:    book.Symbol,
		BookSeqNum:    book.SeqNum,
	}
}

// createTrade records the trade, increments SeqNum, publishes to MD.
func (e *DefaultExchange) createTrade(book *OrderBook, exec *Execution, takerOrder *Order, timestamp int64, log Logger) uint64 {
	tradeID := book.SeqNum
	book.SeqNum++
	trade := &Trade{
		TradeID:      tradeID,
		Price:        exec.Price,
		Qty:          exec.Qty,
		Side:         takerOrder.Side,
		TakerOrderID: exec.TakerOrderID,
		MakerOrderID: exec.MakerOrderID,
	}
	book.LastTrade = trade
	if log != nil {
		log.LogEvent(timestamp, 0, "Trade", trade)
	}
	e.MDPublisher.PublishTrade(book.Symbol, trade, timestamp)
	return tradeID
}

// notifyFill sends gateway and log fill events to both taker and maker.
// Per-execution state comes from exec (TakerFilledQty/MakerFilledQty captured
// at match time), not from the order's final post-match state.
func (e *DefaultExchange) notifyFill(
	exec *Execution, takerOrder *Order, makerPosSide PositionSide, takerFee, makerFee Fee,
	tradeID uint64, book *OrderBook, log Logger, timestamp int64,
	takerDelta, makerDelta PositionDelta, takerPnL, makerPnL int64,
) {
	sendFillNotification(e.Gateways[exec.TakerClientID], exec.TakerOrderID, exec.TakerClientID,
		tradeID, exec, takerOrder.Side, takerOrder.PositionSide, takerFee,
		exec.TakerFilledQty >= takerOrder.Qty, book.Symbol, takerDelta, takerPnL)
	logFill(log, timestamp, exec.TakerClientID, exec.TakerOrderID, exec,
		takerOrder.Side, takerOrder.PositionSide, exec.TakerFilledQty, takerOrder.Qty,
		tradeID, takerFee, takerDelta, takerPnL, book.Symbol, "taker")

	sendFillNotification(e.Gateways[exec.MakerClientID], exec.MakerOrderID, exec.MakerClientID,
		tradeID, exec, exec.MakerSide, makerPosSide, makerFee,
		exec.MakerFilledQty >= exec.MakerTotalQty, book.Symbol, makerDelta, makerPnL)
	logFill(log, timestamp, exec.MakerClientID, exec.MakerOrderID, exec,
		exec.MakerSide, makerPosSide, exec.MakerFilledQty, exec.MakerTotalQty,
		tradeID, makerFee, makerDelta, makerPnL, book.Symbol, "maker")
}

func (e *DefaultExchange) publishOpenInterest(book *OrderBook, timestamp int64) {
	e.MDPublisher.PublishOpenInterest(book.Symbol, &OpenInterest{
		Symbol:         book.Symbol,
		TotalContracts: e.Positions.CalculateOpenInterest(book.Symbol),
		Timestamp:      timestamp,
	}, timestamp)
}

// settleSpotExecution settles balances for both taker and maker in a spot trade.
// Caller must hold e.mu.Lock().
func (e *DefaultExchange) settleSpotExecution(
	book *OrderBook, exec *Execution, takerOrder, makerOrder *Order,
	taker, maker *Client, takerFee, makerFee Fee,
	notional, timestamp int64,
) {
	base, quote := book.Instrument.BaseAsset(), book.Instrument.QuoteAsset()
	if takerOrder.Side == Buy {
		e.settleSpotBuyer(taker, exec.TakerClientID, book, takerOrder, exec, base, quote, exec.Qty, notional, takerFee, timestamp)
		e.settleSpotSeller(maker, exec.MakerClientID, book, makerOrder, exec, base, quote, exec.Qty, notional, makerFee, timestamp)
	} else {
		e.settleSpotSeller(taker, exec.TakerClientID, book, takerOrder, exec, base, quote, exec.Qty, notional, takerFee, timestamp)
		e.settleSpotBuyer(maker, exec.MakerClientID, book, makerOrder, exec, base, quote, exec.Qty, notional, makerFee, timestamp)
	}
	e.recordFeeRevenue(quote, takerFee, makerFee, book, timestamp)
}

// spotFillRelease computes the reservation to unlock for this fill as a delta
// against the order's ledger: previous reservation minus what the unfilled
// remainder still requires at the order's limit price. This releases
// price-improvement and fee headroom exactly instead of recomputing from the
// execution price. Market orders reserve nothing and release nothing — the
// old unconditional release silently consumed reservations backing the
// client's OTHER resting orders (the classic "out of money" corruption).
func spotFillRelease(client *Client, book *OrderBook, order *Order, exec *Execution, side Side, precision int64) int64 {
	if order == nil {
		// Custom matcher removed the order pre-settlement; release at the
		// execution price (legacy behavior, may leak improvement delta).
		if side == Buy {
			return MulDiv(exec.Qty, exec.Price, precision)
		}
		return exec.Qty
	}
	if order.Type == Market {
		return 0
	}
	instrument := book.Instrument
	stillNeeded := spotOrderReservation(client.FeePlan, instrument, side, order.Qty-order.FilledQty, order.Price, precision)
	release := order.Reserved - stillNeeded
	if release <= 0 {
		return 0
	}
	order.Reserved -= release
	return release
}

// settleSpotBuyer releases the buyer's quote reservation and settles balances.
// Caller must hold e.mu.Lock().
func (e *DefaultExchange) settleSpotBuyer(client *Client, clientID uint64, book *OrderBook, order *Order, exec *Execution, base, quote string, qty, notional int64, fee Fee, timestamp int64) {
	oldBase, oldQuote := client.Balances[base], client.Balances[quote]
	oldFeeAsset := client.Balances[fee.Asset]
	client.Release(quote, spotFillRelease(client, book, order, exec, Buy, book.Instrument.BasePrecision()))
	client.Balances[quote] -= notional
	client.Balances[fee.Asset] -= fee.Amount
	client.Balances[base] += qty
	deltas := []BalanceDelta{
		spotDelta(base, oldBase, client.Balances[base]),
		spotDelta(quote, oldQuote, client.Balances[quote]),
	}
	if fee.Asset != quote && fee.Asset != base {
		deltas = append(deltas, spotDelta(fee.Asset, oldFeeAsset, client.Balances[fee.Asset]))
	}
	logBalanceChange(e, timestamp, clientID, book.Symbol, "trade_settlement", deltas)
}

// settleSpotSeller releases the seller's base reservation and settles balances.
// Caller must hold e.mu.Lock().
func (e *DefaultExchange) settleSpotSeller(client *Client, clientID uint64, book *OrderBook, order *Order, exec *Execution, base, quote string, qty, notional int64, fee Fee, timestamp int64) {
	oldBase, oldQuote := client.Balances[base], client.Balances[quote]
	oldFeeAsset := client.Balances[fee.Asset]
	client.Release(base, spotFillRelease(client, book, order, exec, Sell, book.Instrument.BasePrecision()))
	client.Balances[base] -= qty
	client.Balances[quote] += notional
	client.Balances[fee.Asset] -= fee.Amount
	deltas := []BalanceDelta{
		spotDelta(base, oldBase, client.Balances[base]),
		spotDelta(quote, oldQuote, client.Balances[quote]),
	}
	if fee.Asset != quote && fee.Asset != base {
		deltas = append(deltas, spotDelta(fee.Asset, oldFeeAsset, client.Balances[fee.Asset]))
	}
	logBalanceChange(e, timestamp, clientID, book.Symbol, "trade_settlement", deltas)
}

// recordFeeRevenue updates exchange fee balances and logs the revenue event.
// Each fee is booked into ITS OWN asset — clients are debited in Fee.Asset, so
// crediting revenue under any other asset would destroy one asset and mint
// another. defaultAsset covers fees with an unset asset (zero-fee probes).
// Caller must hold e.mu.Lock().
func (e *DefaultExchange) recordFeeRevenue(defaultAsset string, takerFee, makerFee Fee, book *OrderBook, timestamp int64) {
	takerAsset, makerAsset := takerFee.Asset, makerFee.Asset
	if takerAsset == "" {
		takerAsset = defaultAsset
	}
	if makerAsset == "" {
		makerAsset = defaultAsset
	}
	e.ExchangeBalance.FeeRevenue[takerAsset] += takerFee.Amount
	e.ExchangeBalance.FeeRevenue[makerAsset] += makerFee.Amount

	log := e.getLogger(book.Symbol)
	if log == nil {
		return
	}
	logRevenue := func(asset string, takerAmt, makerAmt int64) {
		log.LogEvent(timestamp, 0, "fee_revenue", FeeRevenueEvent{
			Timestamp: timestamp,
			Symbol:    book.Symbol,
			TradeID:   book.SeqNum,
			TakerFee:  takerAmt,
			MakerFee:  makerAmt,
			Asset:     asset,
		})
	}
	if takerAsset == makerAsset {
		logRevenue(takerAsset, takerFee.Amount, makerFee.Amount)
		return
	}
	logRevenue(takerAsset, takerFee.Amount, 0)
	logRevenue(makerAsset, 0, makerFee.Amount)
}

func logFill(log Logger, timestamp int64, clientID, orderID uint64, exec *Execution,
	side Side, posSide PositionSide, filledQty, totalQty int64,
	tradeID uint64, fee Fee, delta PositionDelta, realizedPnL int64,
	symbol, role string,
) {
	if log == nil {
		return
	}
	log.LogEvent(timestamp, clientID, "OrderFill", map[string]any{
		"order_id":        orderID,
		"symbol":          symbol,
		"qty":             exec.Qty,
		"price":           exec.Price,
		"side":            side.String(),
		"position_side":   posSide.String(),
		"filled_qty":      filledQty,
		"remaining_qty":   totalQty - filledQty,
		"is_full":         filledQty >= totalQty,
		"trade_id":        tradeID,
		"role":            role,
		"fee_amount":      fee.Amount,
		"fee_asset":       fee.Asset,
		"realized_pnl":    realizedPnL,
		"new_size":        delta.NewSize,
		"new_entry_price": delta.NewEntryPrice,
	})
}

func sendFillNotification(
	gw *ClientGateway, orderID, clientID, tradeID uint64,
	exec *Execution, side Side, posSide PositionSide, fee Fee, isFull bool,
	symbol string, delta PositionDelta, realizedPnL int64,
) {
	if gw == nil {
		return
	}
	gw.ResponseCh <- Response{
		Success: true,
		Data: &FillNotification{
			OrderID:       orderID,
			ClientID:      clientID,
			TradeID:       tradeID,
			Symbol:        symbol,
			Qty:           exec.Qty,
			Price:         exec.Price,
			Side:          side,
			PositionSide:  posSide,
			IsFull:        isFull,
			FeeAmount:     fee.Amount,
			FeeAsset:      fee.Asset,
			RealizedPnL:   realizedPnL,
			NewSize:       delta.NewSize,
			NewEntryPrice: delta.NewEntryPrice,
		},
	}
}
