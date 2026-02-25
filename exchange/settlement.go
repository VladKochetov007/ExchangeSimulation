package exchange

func (e *Exchange) processExecutions(book *OrderBook, executions []*Execution, takerOrder *Order) {
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
// Returns true if a perp position changed (for OI tracking).
func (e *Exchange) handleExecution(
	book *OrderBook, exec *Execution, takerOrder *Order,
	instrument Instrument, basePrecision, timestamp int64, log Logger,
) bool {
	taker := e.Clients[exec.TakerClientID]
	maker := e.Clients[exec.MakerClientID]
	takerFee := taker.FeePlan.CalculateFee(exec, takerOrder.Side, false, instrument.BaseAsset(), instrument.QuoteAsset(), basePrecision)
	makerFee := maker.FeePlan.CalculateFee(exec, exec.MakerSide, true, instrument.BaseAsset(), instrument.QuoteAsset(), basePrecision)
	notional := (exec.Price * exec.Qty) / basePrecision
	isPerp := instrument.IsPerp()
	if isPerp {
		e.processPerpExecution(book, exec, takerOrder, taker, maker, takerFee, makerFee, basePrecision, timestamp)
	} else {
		e.settleSpotExecution(book, exec, takerOrder, taker, maker, takerFee, makerFee, notional, timestamp)
	}
	taker.TakerVolume += notional
	maker.MakerVolume += notional
	tradeID := e.createTrade(book, exec, takerOrder, timestamp, log)
	e.notifyFill(exec, takerOrder, takerFee, makerFee, tradeID, book, log, timestamp)
	return isPerp
}

// createTrade records the trade, increments SeqNum, publishes to MD.
func (e *Exchange) createTrade(book *OrderBook, exec *Execution, takerOrder *Order, timestamp int64, log Logger) uint64 {
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
func (e *Exchange) notifyFill(
	exec *Execution, takerOrder *Order, takerFee, makerFee Fee,
	tradeID uint64, book *OrderBook, log Logger, timestamp int64,
) {
	sendFillNotification(e.Gateways[exec.TakerClientID], exec.TakerOrderID, exec.TakerClientID,
		tradeID, exec, takerOrder.Side, takerFee, takerOrder.FilledQty >= takerOrder.Qty)
	logFill(log, timestamp, exec.TakerClientID, exec.TakerOrderID, exec,
		takerOrder.Side, takerOrder.FilledQty, takerOrder.Qty, tradeID, takerFee, "taker")

	makerOrder := book.findOrder(exec.MakerOrderID)
	sendFillNotification(e.Gateways[exec.MakerClientID], exec.MakerOrderID, exec.MakerClientID,
		tradeID, exec, exec.MakerSide, makerFee, makerOrder != nil && makerOrder.FilledQty >= makerOrder.Qty)
	logFill(log, timestamp, exec.MakerClientID, exec.MakerOrderID, exec,
		exec.MakerSide, exec.MakerFilledQty, exec.MakerTotalQty, tradeID, makerFee, "maker")
}

func (e *Exchange) publishOpenInterest(book *OrderBook, timestamp int64) {
	e.MDPublisher.PublishOpenInterest(book.Symbol, &OpenInterest{
		Symbol:         book.Symbol,
		TotalContracts: e.Positions.CalculateOpenInterest(book.Symbol),
		Timestamp:      timestamp,
	}, timestamp)
}

// settleSpotExecution settles balances for both taker and maker in a spot trade.
// Caller must hold e.mu.Lock().
func (e *Exchange) settleSpotExecution(
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
func (e *Exchange) settleSpotBuyer(client *Client, clientID uint64, book *OrderBook, base, quote string, qty, notional int64, fee Fee, timestamp int64) {
	oldBase, oldQuote := client.Balances[base], client.Balances[quote]
	client.Release(quote, notional)
	client.Balances[quote] -= notional + fee.Amount
	client.Balances[base] += qty
	e.balanceTracker.LogBalanceChange(timestamp, clientID, book.Symbol, "trade_settlement", []BalanceDelta{
		spotDelta(base, oldBase, client.Balances[base]),
		spotDelta(quote, oldQuote, client.Balances[quote]),
	})
}

// settleSpotSeller releases the seller's base reservation and settles balances.
// Caller must hold e.mu.Lock().
func (e *Exchange) settleSpotSeller(client *Client, clientID uint64, book *OrderBook, base, quote string, qty, notional int64, fee Fee, timestamp int64) {
	oldBase, oldQuote := client.Balances[base], client.Balances[quote]
	client.Release(base, qty)
	client.Balances[base] -= qty
	client.Balances[quote] += notional - fee.Amount
	e.balanceTracker.LogBalanceChange(timestamp, clientID, book.Symbol, "trade_settlement", []BalanceDelta{
		spotDelta(base, oldBase, client.Balances[base]),
		spotDelta(quote, oldQuote, client.Balances[quote]),
	})
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
// Caller must hold e.mu.Lock().
func (e *Exchange) settlePerpSide(ctx perpSideCtx, book *OrderBook, exec *Execution, quote string, basePrecision, timestamp int64) {
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
	ctx.client.PerpBalances[quote] += pnl - ctx.fee.Amount
	e.balanceTracker.LogBalanceChange(timestamp, ctx.clientID, book.Symbol, "trade_settlement", []BalanceDelta{
		perpDelta(quote, old, ctx.client.PerpBalances[quote]),
	})
}

// processPerpExecution settles a single perp execution for both taker and maker.
// Caller must hold e.mu.Lock().
func (e *Exchange) processPerpExecution(
	book *OrderBook, exec *Execution, takerOrder *Order,
	taker, maker *Client, takerFee, makerFee Fee,
	basePrecision, timestamp int64,
) {
	perp := book.Instrument.(*PerpFutures)
	quote := book.Instrument.QuoteAsset()

	takerDelta := e.Positions.UpdatePositionWithDelta(exec.TakerClientID, book.Symbol, exec.Qty, exec.Price, takerOrder.Side, e, "trade")
	makerDelta := e.Positions.UpdatePositionWithDelta(exec.MakerClientID, book.Symbol, exec.Qty, exec.Price, exec.MakerSide, e, "trade")

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
	e.settlePerpSide(takerCtx, book, exec, quote, basePrecision, timestamp)
	e.settlePerpSide(makerCtx, book, exec, quote, basePrecision, timestamp)
	e.recordFeeRevenue(quote, takerFee, makerFee, book, timestamp)
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
func (e *Exchange) recordFeeRevenue(asset string, takerFee, makerFee Fee, book *OrderBook, timestamp int64) {
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

func logFill(log Logger, timestamp int64, clientID, orderID uint64, exec *Execution, side Side, filledQty, totalQty int64, tradeID uint64, fee Fee, role string) {
	if log == nil {
		return
	}
	log.LogEvent(timestamp, clientID, "OrderFill", map[string]any{
		"order_id":      orderID,
		"qty":           exec.Qty,
		"price":         exec.Price,
		"side":          side.String(),
		"filled_qty":    filledQty,
		"remaining_qty": totalQty - filledQty,
		"is_full":       filledQty >= totalQty,
		"trade_id":      tradeID,
		"role":          role,
		"fee_amount":    fee.Amount,
		"fee_asset":     fee.Asset,
	})
}

func sendFillNotification(gw *ClientGateway, orderID, clientID, tradeID uint64, exec *Execution, side Side, fee Fee, isFull bool) {
	if gw == nil {
		return
	}
	gw.ResponseCh <- Response{
		Success: true,
		Data: &FillNotification{
			OrderID:   orderID,
			ClientID:  clientID,
			TradeID:   tradeID,
			Qty:       exec.Qty,
			Price:     exec.Price,
			Side:      side,
			IsFull:    isFull,
			FeeAmount: fee.Amount,
			FeeAsset:  fee.Asset,
		},
	}
}
