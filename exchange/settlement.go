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

	var result SettlementResult
	var positionChanged bool
	if s, ok := instrument.(Settleable); ok {
		makerOrder := book.FindOrder(exec.MakerOrderID)
		makerPosSide := PositionBoth
		if makerOrder != nil {
			makerPosSide = makerOrder.PositionSide
		}
		result = s.Settle(e.buildSettlementContext(book, exec, takerOrder, makerPosSide, takerFee, makerFee, basePrecision, timestamp, log))
		positionChanged = true
	} else {
		notional := (exec.Price * exec.Qty) / basePrecision
		e.settleSpotExecution(book, exec, takerOrder, taker, maker, takerFee, makerFee, notional, timestamp)
	}
	tradeID := e.createTrade(book, exec, takerOrder, timestamp, log)
	e.notifyFill(exec, takerOrder, takerFee, makerFee, tradeID, book, log, timestamp, result.TakerDelta, result.MakerDelta, result.TakerPnL, result.MakerPnL)
	return positionChanged
}

func (e *DefaultExchange) buildSettlementContext(
	book *OrderBook, exec *Execution, takerOrder *Order,
	makerPosSide PositionSide, takerFee, makerFee Fee,
	basePrecision, timestamp int64, log Logger,
) SettlementContext {
	clients := e.Clients
	return SettlementContext{
		Exec:         exec,
		TakerOrder:   takerOrder,
		MakerPosSide: makerPosSide,
		TakerFee:     takerFee,
		MakerFee:     makerFee,
		Positions:    e.Positions,
		PerpBalance:       func(clientID uint64, asset string) int64        { return clients[clientID].PerpBalance(asset) },
		MutatePerpBalance: func(clientID uint64, asset string, delta int64) { clients[clientID].MutatePerpBalance(asset, delta) },
		ReservePerp:       func(clientID uint64, asset string, amount int64) bool { return clients[clientID].ReservePerp(asset, amount) },
		ReleasePerp:       func(clientID uint64, asset string, amount int64) { clients[clientID].ReleasePerp(asset, amount) },
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
func (e *DefaultExchange) notifyFill(
	exec *Execution, takerOrder *Order, takerFee, makerFee Fee,
	tradeID uint64, book *OrderBook, log Logger, timestamp int64,
	takerDelta, makerDelta PositionDelta, takerPnL, makerPnL int64,
) {
	sendFillNotification(e.Gateways[exec.TakerClientID], exec.TakerOrderID, exec.TakerClientID,
		tradeID, exec, takerOrder.Side, takerOrder.PositionSide, takerFee,
		takerOrder.FilledQty >= takerOrder.Qty, book.Symbol, takerDelta, takerPnL)
	logFill(log, timestamp, exec.TakerClientID, exec.TakerOrderID, exec,
		takerOrder.Side, takerOrder.PositionSide, takerOrder.FilledQty, takerOrder.Qty,
		tradeID, takerFee, takerDelta, takerPnL, book.Symbol, "taker")

	makerOrder := book.FindOrder(exec.MakerOrderID)
	makerPosSide := PositionBoth
	if makerOrder != nil {
		makerPosSide = makerOrder.PositionSide
	}
	sendFillNotification(e.Gateways[exec.MakerClientID], exec.MakerOrderID, exec.MakerClientID,
		tradeID, exec, exec.MakerSide, makerPosSide, makerFee,
		makerOrder != nil && makerOrder.FilledQty >= makerOrder.Qty, book.Symbol, makerDelta, makerPnL)
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
	book *OrderBook, exec *Execution, takerOrder *Order,
	taker, maker *Client, takerFee, makerFee Fee,
	notional, timestamp int64,
) {
	base, quote := book.Instrument.BaseAsset(), book.Instrument.QuoteAsset()
	if takerOrder.Side == Buy {
		e.settleSpotBuyer(taker, exec.TakerClientID, book, base, quote, exec.Qty, notional, takerFee, timestamp)
		e.settleSpotSeller(maker, exec.MakerClientID, book, base, quote, exec.Qty, notional, makerFee, timestamp)
	} else {
		e.settleSpotSeller(taker, exec.TakerClientID, book, base, quote, exec.Qty, notional, takerFee, timestamp)
		e.settleSpotBuyer(maker, exec.MakerClientID, book, base, quote, exec.Qty, notional, makerFee, timestamp)
	}
	e.recordFeeRevenue(quote, takerFee, makerFee, book, timestamp)
}

// settleSpotBuyer releases the buyer's quote reservation and settles balances.
// Caller must hold e.mu.Lock().
func (e *DefaultExchange) settleSpotBuyer(client *Client, clientID uint64, book *OrderBook, base, quote string, qty, notional int64, fee Fee, timestamp int64) {
	oldBase, oldQuote := client.Balances[base], client.Balances[quote]
	oldFeeAsset := client.Balances[fee.Asset]
	client.Release(quote, notional)
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
func (e *DefaultExchange) settleSpotSeller(client *Client, clientID uint64, book *OrderBook, base, quote string, qty, notional int64, fee Fee, timestamp int64) {
	oldBase, oldQuote := client.Balances[base], client.Balances[quote]
	oldFeeAsset := client.Balances[fee.Asset]
	client.Release(base, qty)
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

func calcMargin(qty, price, rate, precision int64) int64 {
	return (qty * price / precision) * rate / 10000
}

type perpSideCtx struct {
	client     *Client
	clientID   uint64
	side       Side
	delta      PositionDelta
	closedQty  int64
	fee        Fee
	isMarket   bool
	orderPrice int64
}

// adjustPerpMargin adjusts margin reserves for one perp trade participant.
// Market takers reserve margin for the opened portion; limit orders release
// pre-reserved order margin on the closing portion. Both release position
// margin for any closed portion using the old entry price.
func adjustPerpMargin(ctx perpSideCtx, execPrice, execQty int64, perp *PerpFutures, quote string, basePrecision int64) {
	margin := func(qty, price int64) int64 { return calcMargin(qty, price, perp.MarginRate, basePrecision) }
	if ctx.isMarket {
		if openedQty := execQty - ctx.closedQty; openedQty > 0 {
			ctx.client.ReservePerp(quote, margin(openedQty, execPrice))
		}
	} else if ctx.closedQty > 0 {
		ctx.client.ReleasePerp(quote, margin(ctx.closedQty, ctx.orderPrice))
	}
	if ctx.closedQty > 0 && ctx.delta.OldSize != 0 {
		ctx.client.ReleasePerp(quote, margin(ctx.closedQty, ctx.delta.OldEntryPrice))
	}
}

// settlePerpSide realizes PnL, logs it, and settles the perp balance for one participant.
// Returns the realized PnL amount.
// Caller must hold e.mu.Lock().
func (e *DefaultExchange) settlePerpSide(ctx perpSideCtx, book *OrderBook, exec *Execution, quote string, basePrecision, timestamp int64) int64 {
	pnl := realizedPerpPnL(ctx.delta.OldSize, ctx.delta.OldEntryPrice, exec.Qty, exec.Price, ctx.side, basePrecision)
	if pnl != 0 {
		if globalLog := e.getLogger("_global"); globalLog != nil {
			globalLog.LogEvent(timestamp, ctx.clientID, "realized_pnl", RealizedPnLEvent{
				Timestamp:  timestamp,
				ClientID:   ctx.clientID,
				Symbol:     book.Symbol,
				TradeID:    book.SeqNum,
				ClosedQty:  ctx.closedQty,
				EntryPrice: ctx.delta.OldEntryPrice,
				ExitPrice:  exec.Price,
				PnL:        pnl,
				Side:       ctx.side.String(),
			})
		}
	}
	old := ctx.client.PerpBalances[quote]
	ctx.client.PerpBalances[quote] += pnl
	ctx.client.PerpBalances[ctx.fee.Asset] -= ctx.fee.Amount
	logBalanceChange(e, timestamp, ctx.clientID, book.Symbol, "trade_settlement", []BalanceDelta{
		perpDelta(quote, old, ctx.client.PerpBalances[quote]),
	})
	return pnl
}

// processPerpExecution settles a single perp execution for both taker and maker.
// Returns taker/maker position deltas and realized PnL for each.
// Caller must hold e.mu.Lock().
func (e *DefaultExchange) processPerpExecution(
	book *OrderBook, exec *Execution, takerOrder *Order,
	taker, maker *Client, takerFee, makerFee Fee,
	basePrecision, timestamp int64,
) (takerDelta, makerDelta PositionDelta, takerPnL, makerPnL int64) {
	perp := book.Instrument.(*PerpFutures)
	quote := book.Instrument.QuoteAsset()

	makerOrder := book.FindOrder(exec.MakerOrderID)
	makerPosSide := PositionBoth
	if makerOrder != nil {
		makerPosSide = makerOrder.PositionSide
	}

	takerDelta = e.Positions.UpdatePosition(exec.TakerClientID, book.Symbol, exec.Qty, exec.Price, takerOrder.Side, takerOrder.PositionSide)
	makerDelta = e.Positions.UpdatePosition(exec.MakerClientID, book.Symbol, exec.Qty, exec.Price, exec.MakerSide, makerPosSide)
	if globalLog := e.getLogger("_global"); globalLog != nil {
		logPositionUpdate(globalLog, timestamp, exec.TakerClientID, book.Symbol, exec.Qty, exec.Price, takerOrder.Side, takerDelta)
		logPositionUpdate(globalLog, timestamp, exec.MakerClientID, book.Symbol, exec.Qty, exec.Price, exec.MakerSide, makerDelta)
		logOpenInterest(globalLog, timestamp, book.Symbol, e.Positions.CalculateOpenInterest(book.Symbol))
	}

	takerCtx := perpSideCtx{
		client:     taker,
		clientID:   exec.TakerClientID,
		side:       takerOrder.Side,
		delta:      takerDelta,
		closedQty:  calculateClosedQty(takerDelta.OldSize, exec.Qty, takerOrder.Side),
		fee:        takerFee,
		isMarket:   takerOrder.Type == Market,
		orderPrice: takerOrder.Price,
	}
	makerCtx := perpSideCtx{
		client:    maker,
		clientID:  exec.MakerClientID,
		side:      exec.MakerSide,
		delta:     makerDelta,
		closedQty: calculateClosedQty(makerDelta.OldSize, exec.Qty, exec.MakerSide),
		fee:       makerFee,
		// Use exec.Price since maker order may be removed from book after full fill
		orderPrice: exec.Price,
	}

	adjustPerpMargin(takerCtx, exec.Price, exec.Qty, perp, quote, basePrecision)
	adjustPerpMargin(makerCtx, exec.Price, exec.Qty, perp, quote, basePrecision)
	takerPnL = e.settlePerpSide(takerCtx, book, exec, quote, basePrecision, timestamp)
	makerPnL = e.settlePerpSide(makerCtx, book, exec, quote, basePrecision, timestamp)
	e.recordFeeRevenue(quote, takerFee, makerFee, book, timestamp)
	return
}

func calculateClosedQty(oldSize, tradeQty int64, side Side) int64 {
	if oldSize == 0 {
		return 0
	}
	deltaSize := tradeQty
	if side == Sell {
		deltaSize = -tradeQty
	}
	if (oldSize > 0 && deltaSize >= 0) || (oldSize < 0 && deltaSize <= 0) {
		return 0
	}
	closedQty := min(abs(deltaSize), abs(oldSize))
	return closedQty
}

// recordFeeRevenue updates exchange fee balance and logs the revenue event.
// Caller must hold e.mu.Lock().
func (e *DefaultExchange) recordFeeRevenue(asset string, takerFee, makerFee Fee, book *OrderBook, timestamp int64) {
	e.ExchangeBalance.FeeRevenue[asset] += takerFee.Amount + makerFee.Amount
	if globalLog := e.getLogger("_global"); globalLog != nil {
		globalLog.LogEvent(timestamp, 0, "fee_revenue", FeeRevenueEvent{
			Timestamp: timestamp,
			Symbol:    book.Symbol,
			TradeID:   book.SeqNum,
			TakerFee:  takerFee.Amount,
			MakerFee:  makerFee.Amount,
			Asset:     asset,
		})
	}
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

func logPositionUpdate(log Logger, timestamp int64, clientID uint64, symbol string, qty, price int64, side Side, delta PositionDelta) {
	log.LogEvent(timestamp, clientID, "position_update", PositionUpdateEvent{
		Timestamp:     timestamp,
		ClientID:      clientID,
		Symbol:        symbol,
		OldSize:       delta.OldSize,
		OldEntryPrice: delta.OldEntryPrice,
		NewSize:       delta.NewSize,
		NewEntryPrice: delta.NewEntryPrice,
		TradeQty:      qty,
		TradePrice:    price,
		TradeSide:     side.String(),
		Reason:        "trade",
	})
}

func logOpenInterest(log Logger, timestamp int64, symbol string, openInterest int64) {
	log.LogEvent(timestamp, 0, "open_interest", OpenInterestEvent{
		Timestamp:    timestamp,
		Symbol:       symbol,
		OpenInterest: openInterest,
	})
}
