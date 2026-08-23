package exchange

import "fmt"

// Settling one execution is an orchestration: fees are quoted, balances and
// positions move, a trade is recorded, and both sides are told what happened.
// Those are four different concerns and this file keeps them apart —
// executionContext carries the identity of the matched pair, settleExecution
// moves the money, and settlementOutcome is the record everything downstream is
// built from. handleExecution only sequences them.

// calcClientFee computes a client's fee for a fill, treating a nil client or nil
// FeePlan as the zero-fee plan (matching the reservation path).
func calcClientFee(client *Client, ctx FillContext) (Fee, error) {
	if client == nil || client.FeePlan == nil {
		return Fee{}, nil
	}
	return client.FeePlan.CalculateFee(ctx)
}

// normalizedExecutionFee makes an omitted fee asset mean the instrument quote
// asset, matching fee-revenue accounting. A zero fee must not create a phantom
// empty-string ledger asset merely because a zero-fee plan returned Fee{}.
func normalizedExecutionFee(fee Fee, quoteAsset string) Fee {
	if fee.Amount == 0 {
		return Fee{}
	}
	if fee.Asset == "" {
		fee.Asset = quoteAsset
	}
	return fee
}

// executionContext is one matched pair and everything settling it needs.
//
// It exists so the settlement, trade-recording and reporting steps take one
// argument rather than a dozen positional ones, which is what let an earlier
// version pass thirteen values to a single notification call.
//
// The caller must hold e.mu.Lock() for the whole lifetime of a context: it
// holds live pointers into the book and the client ledger.
type executionContext struct {
	book       *OrderBook
	exec       *Execution
	instrument Instrument

	takerOrder *Order
	makerOrder *Order
	taker      *Client
	maker      *Client
	// makerPosSide comes from the resting order when it is still in the book,
	// and from the execution otherwise, which is what a custom matcher leaves.
	makerPosSide PositionSide

	takerFee Fee
	makerFee Fee

	baseAsset     string
	quoteAsset    string
	basePrecision int64
	timestamp     int64
	log           Logger
}

// requireParties asserts the invariant every settlement path relies on: both
// sides of a match are registered clients.
//
// Clients are never removed from the registry — only gateways are, on
// disconnect — and an order cannot reach the book without one, so a missing
// party means the book outlived its owner. That is corruption, and settling
// half a trade against it would silently mint or destroy assets. Failing here,
// where both parties are resolved once, beats a nil dereference inside an
// instrument's Settle closure with no indication of which side was missing.
func (c executionContext) requireParties() {
	if c.taker == nil {
		panic(fmt.Sprintf("settlement: taker %d of order %d on %s is not a registered client",
			c.exec.TakerClientID, c.exec.TakerOrderID, c.book.Symbol))
	}
	if c.maker == nil {
		panic(fmt.Sprintf("settlement: maker %d of order %d on %s is not a registered client",
			c.exec.MakerClientID, c.exec.MakerOrderID, c.book.Symbol))
	}
}

// notional is the quote-currency value of the execution.
func (c executionContext) notional() int64 {
	return MulDiv(c.exec.Qty, c.exec.Price, c.basePrecision)
}

// newExecutionContext resolves the parties and attaches the preflighted fees
// for one match.
//
// A planned spot match has already quoted the exact per-execution fees on a
// detached book. Re-asking a stateful fee model here would charge a different
// schedule from the one that passed admission, so settlement never consults a
// fee model after Match. Every live matching path must therefore supply its
// corresponding detached execution plan.
func (e *DefaultExchange) newExecutionContext(
	book *OrderBook, exec *Execution, takerOrder *Order,
	instrument Instrument, basePrecision, timestamp int64, log Logger,
	planned *plannedSpotExecution,
) executionContext {
	baseAsset, quoteAsset := instrument.BaseAsset(), instrument.QuoteAsset()
	// Fully filled makers stay in book.Orders until removeMakerOrders, so this
	// lookup normally succeeds; exec carries the fallbacks for custom matchers.
	makerOrder := book.FindOrder(exec.MakerOrderID)
	makerPosSide := exec.MakerPosSide
	if makerOrder != nil {
		makerPosSide = makerOrder.PositionSide
	}

	ctx := executionContext{
		book:          book,
		exec:          exec,
		instrument:    instrument,
		takerOrder:    takerOrder,
		makerOrder:    makerOrder,
		taker:         e.Clients[exec.TakerClientID],
		maker:         e.Clients[exec.MakerClientID],
		makerPosSide:  makerPosSide,
		baseAsset:     baseAsset,
		quoteAsset:    quoteAsset,
		basePrecision: basePrecision,
		timestamp:     timestamp,
		log:           log,
	}
	ctx.requireParties()
	if planned == nil || !planned.fingerprint.matches(exec) {
		panic("settlement received execution without matching fee preflight")
	}
	ctx.takerFee, ctx.makerFee = planned.takerFee, planned.makerFee
	ctx.takerFee = normalizedExecutionFee(ctx.takerFee, quoteAsset)
	ctx.makerFee = normalizedExecutionFee(ctx.makerFee, quoteAsset)
	return ctx
}

// fillSide is one party's view of a settled execution, in the form both the log
// and the gateway notification need. Building it once means the two reports
// cannot disagree about what happened.
type fillSide struct {
	clientID    uint64
	orderID     uint64
	side        Side
	posSide     PositionSide
	fee         Fee
	filledQty   int64
	totalQty    int64
	delta       PositionDelta
	realizedPnL int64
	role        string
}

func (f fillSide) isFull() bool { return f.filledQty >= f.totalQty }

// settlementOutcome is what settling an execution produced. Everything reported
// downstream is derived from it rather than recomputed.
type settlementOutcome struct {
	taker fillSide
	maker fillSide
	// positionChanged is true when the instrument keeps positions, which is what
	// makes open interest worth republishing.
	positionChanged bool
	tradeID         uint64
}

func (e *DefaultExchange) processExecutions(book *OrderBook, executions []*Execution, takerOrder *Order, plan *spotExecutionPlan) {
	instrument := book.Instrument
	timestamp := e.Clock.NowUnixNano()
	basePrecision := instrument.BasePrecision()
	log := e.getLogger(book.Symbol)
	positionChanged := false
	for i, exec := range executions {
		var planned *plannedSpotExecution
		if plan != nil {
			if i >= len(plan.fills) || !plan.fills[i].fingerprint.matches(exec) {
				panic("spot execution plan diverged during settlement")
			}
			planned = &plan.fills[i]
		}
		if e.handleExecution(book, exec, takerOrder, instrument, basePrecision, timestamp, log, planned) {
			positionChanged = true
		}
	}
	if positionChanged {
		e.publishOpenInterest(book, timestamp)
	}
}

// handleExecution sequences the settlement of one matched pair and returns
// whether a position changed, which is what open-interest tracking needs.
//
// Caller must hold e.mu.Lock().
func (e *DefaultExchange) handleExecution(
	book *OrderBook, exec *Execution, takerOrder *Order,
	instrument Instrument, basePrecision, timestamp int64, log Logger, planned *plannedSpotExecution,
) bool {
	ctx := e.newExecutionContext(book, exec, takerOrder, instrument, basePrecision, timestamp, log, planned)
	outcome := e.settleExecution(ctx)
	outcome.tradeID = e.createTrade(ctx)
	e.reportFill(ctx, outcome)
	return outcome.positionChanged
}

// settleExecution moves money and positions for one execution and returns what
// each side ended up with. It does not record or report anything.
func (e *DefaultExchange) settleExecution(ctx executionContext) settlementOutcome {
	var result SettlementResult
	positionChanged := false
	if settleable, ok := ctx.instrument.(Settleable); ok {
		result = settleable.Settle(e.buildSettlementContext(ctx))
		positionChanged = true
		// The instrument's margin-only release just freed the entire fee
		// headroom, including the share backing the unfilled remainder.
		e.restoreFeeHeadroom(ctx.book, ctx.takerOrder, ctx.basePrecision)
		e.restoreFeeHeadroom(ctx.book, ctx.makerOrder, ctx.basePrecision)
	} else {
		e.settleSpotExecution(ctx)
	}
	e.restoreForeignFeeReservation(ctx.book, ctx.takerOrder, ctx.basePrecision)
	e.restoreForeignFeeReservation(ctx.book, ctx.makerOrder, ctx.basePrecision)

	// Per-execution state comes from exec (TakerFilledQty/MakerFilledQty
	// captured at match time), not from the order's final post-match state.
	return settlementOutcome{
		taker: fillSide{
			clientID: ctx.exec.TakerClientID, orderID: ctx.exec.TakerOrderID,
			side: ctx.takerOrder.Side, posSide: ctx.takerOrder.PositionSide, fee: ctx.takerFee,
			filledQty: ctx.exec.TakerFilledQty, totalQty: ctx.takerOrder.Qty,
			delta: result.TakerDelta, realizedPnL: result.TakerPnL, role: "taker",
		},
		maker: fillSide{
			clientID: ctx.exec.MakerClientID, orderID: ctx.exec.MakerOrderID,
			side: ctx.exec.MakerSide, posSide: ctx.makerPosSide, fee: ctx.makerFee,
			filledQty: ctx.exec.MakerFilledQty, totalQty: ctx.exec.MakerTotalQty,
			delta: result.MakerDelta, realizedPnL: result.MakerPnL, role: "maker",
		},
		positionChanged: positionChanged,
	}
}

// restoreFeeHeadroom re-reserves the worst-case quote fee for the unfilled
// remainder of a margined limit order after a fill's margin release.
func (e *DefaultExchange) restoreFeeHeadroom(book *OrderBook, order *Order, precision int64) {
	if order == nil || order.Type == Market {
		return
	}
	remaining := order.Qty - order.FilledQty
	if remaining <= 0 {
		return
	}
	instrument := book.Instrument
	_, isOrderMargined := instrument.(OrderMarginer)
	_, isMargined := instrument.(Margined)
	if !isOrderMargined && !isMargined {
		return
	}
	client := e.Clients[order.ClientID]
	if client == nil {
		return
	}
	headroom, err := quoteFeeHeadroom(client.FeePlan, instrument.BaseAsset(), instrument.QuoteAsset(), remaining, order.Price, precision)
	if err != nil {
		e.reportPriceUnavailable(e.Clock.NowUnixNano(), book.Symbol, "fee_headroom_refresh", err)
		e.cancelUnfundedFeeRemainder(book, client, order)
		return
	}
	if headroom <= 0 {
		return
	}
	// Force-reserve: the fill's release just freed at least this much, so the
	// cash is present; going through the checked path could spuriously fail
	// when other orders hold the rest of the wallet.
	client.ForceReservePerp(instrument.QuoteAsset(), headroom)
	order.Reserved += headroom
}

func (e *DefaultExchange) buildSettlementContext(ctx executionContext) SettlementContext {
	clients := e.Clients
	book, timestamp := ctx.book, ctx.timestamp
	return SettlementContext{
		Exec:              ctx.exec,
		TakerOrder:        ctx.takerOrder,
		MakerOrder:        ctx.makerOrder,
		MakerPosSide:      ctx.makerPosSide,
		TakerFee:          ctx.takerFee,
		MakerFee:          ctx.makerFee,
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
		Log:           ctx.log,
		GlobalLog:     e.getLogger("_global"),
		BasePrecision: ctx.basePrecision,
		Timestamp:     timestamp,
		BookSymbol:    book.Symbol,
		BookSeqNum:    book.SeqNum,
	}
}

// spotLeg is one party to a spot settlement. Buyer and seller differ only in
// which asset they give up, so they share one settlement path: keeping two
// near-identical ones is how a fix reaches one side and not the other.
type spotLeg struct {
	client   *Client
	clientID uint64
	order    *Order
	side     Side
	fee      Fee
}

// oppositeSide is the other side of a match.
//
// The settling maker's side is derived from the taker's rather than read from
// the execution, which is what the two-branch version it replaced did. The
// execution's own MakerSide is still what gets reported, so a custom matcher
// that disagrees with itself keeps whatever behaviour it had.
func oppositeSide(side Side) Side {
	if side == Buy {
		return Sell
	}
	return Buy
}

// settleSpotExecution settles balances for both parties to a spot trade.
// Caller must hold e.mu.Lock().
func (e *DefaultExchange) settleSpotExecution(ctx executionContext) {
	takerLeg := spotLeg{client: ctx.taker, clientID: ctx.exec.TakerClientID, order: ctx.takerOrder, side: ctx.takerOrder.Side, fee: ctx.takerFee}
	makerLeg := spotLeg{client: ctx.maker, clientID: ctx.exec.MakerClientID, order: ctx.makerOrder, side: oppositeSide(ctx.takerOrder.Side), fee: ctx.makerFee}
	e.settleSpotLeg(ctx, takerLeg)
	e.settleSpotLeg(ctx, makerLeg)
	e.recordFeeRevenue(ctx.quoteAsset, ctx.takerFee, ctx.makerFee, ctx.book, ctx.timestamp)
}

// settleSpotLeg releases one party's reservation and moves its balances.
//
// A buyer gives up quote and receives base; a seller does the reverse. The
// reservation released is always in the asset given up.
//
// Caller must hold e.mu.Lock().
func (e *DefaultExchange) settleSpotLeg(ctx executionContext, leg spotLeg) {
	notional := ctx.notional()
	give, receive := ctx.quoteAsset, ctx.baseAsset
	giveAmount, receiveAmount := notional, ctx.exec.Qty
	if leg.side == Sell {
		give, receive = ctx.baseAsset, ctx.quoteAsset
		giveAmount, receiveAmount = ctx.exec.Qty, notional
	}

	client := leg.client
	oldGive, oldReceive := client.Balances[give], client.Balances[receive]
	oldFeeAsset := int64(0)
	if leg.fee.Amount != 0 {
		oldFeeAsset = client.Balances[leg.fee.Asset]
	}

	client.Release(give, e.spotFillRelease(client, ctx.book, leg.order, ctx.exec, leg.side, ctx.basePrecision))
	client.Balances[give] -= giveAmount
	client.Balances[receive] += receiveAmount
	if leg.fee.Amount != 0 {
		client.Balances[leg.fee.Asset] -= leg.fee.Amount
	}

	deltas := []BalanceDelta{
		spotDelta(ctx.baseAsset, oldBalanceOf(ctx.baseAsset, give, receive, oldGive, oldReceive), client.Balances[ctx.baseAsset]),
		spotDelta(ctx.quoteAsset, oldBalanceOf(ctx.quoteAsset, give, receive, oldGive, oldReceive), client.Balances[ctx.quoteAsset]),
	}
	// A fee paid in a third asset is its own ledger line; one paid in base or
	// quote is already inside the deltas above.
	if leg.fee.Amount != 0 && leg.fee.Asset != ctx.quoteAsset && leg.fee.Asset != ctx.baseAsset {
		deltas = append(deltas, spotDelta(leg.fee.Asset, oldFeeAsset, client.Balances[leg.fee.Asset]))
	}
	logBalanceChange(e, ctx.timestamp, leg.clientID, ctx.book.Symbol, "trade_settlement", deltas)
}

// oldBalanceOf picks which captured balance belongs to an asset, so the deltas
// are reported base-first and quote-second whichever side the leg is.
func oldBalanceOf(asset, give, receive string, oldGive, oldReceive int64) int64 {
	if asset == give {
		return oldGive
	}
	if asset == receive {
		return oldReceive
	}
	return 0
}

// spotFillRelease computes the reservation to unlock for this fill as a delta
// against the order's ledger: previous reservation minus what the unfilled
// remainder still requires at the order's limit price. This releases
// price-improvement and fee headroom exactly instead of recomputing from the
// execution price. Market orders reserve nothing and release nothing — the
// old unconditional release silently consumed reservations backing the
// client's OTHER resting orders (the classic "out of money" corruption).
func (e *DefaultExchange) spotFillRelease(client *Client, book *OrderBook, order *Order, exec *Execution, side Side, precision int64) int64 {
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
	stillNeeded, ok, err := spotOrderReservation(client.FeePlan, instrument, side, order.Qty-order.FilledQty, order.Price, precision)
	if err != nil {
		e.reportPriceUnavailable(e.Clock.NowUnixNano(), book.Symbol, "spot_fee_release", err)
		e.cancelUnfundedFeeRemainder(book, client, order)
		return 0
	}
	if !ok {
		// The original admission had already accepted a representable larger
		// reservation. Retaining it is safer than releasing into an overflowed
		// remainder calculation; the order will be released on cancellation.
		return 0
	}
	release := order.Reserved - stillNeeded
	if release <= 0 {
		return 0
	}
	order.Reserved -= release
	return release
}

// createTrade records the trade, increments SeqNum, publishes to MD.
func (e *DefaultExchange) createTrade(ctx executionContext) uint64 {
	book := ctx.book
	tradeID := book.SeqNum
	book.SeqNum++
	trade := &Trade{
		TradeID:      tradeID,
		Price:        ctx.exec.Price,
		Qty:          ctx.exec.Qty,
		Side:         ctx.takerOrder.Side,
		TakerOrderID: ctx.exec.TakerOrderID,
		MakerOrderID: ctx.exec.MakerOrderID,
	}
	book.LastTrade = trade
	if ctx.log != nil {
		ctx.log.LogEvent(ctx.timestamp, 0, "Trade", trade)
	}
	e.MDPublisher.PublishTrade(book.Symbol, trade, ctx.timestamp)
	return tradeID
}

// reportFill tells both parties what happened, through the gateway and the log.
func (e *DefaultExchange) reportFill(ctx executionContext, outcome settlementOutcome) {
	for _, side := range []fillSide{outcome.taker, outcome.maker} {
		sendFillNotification(e.Gateways[side.clientID], ctx, outcome.tradeID, side)
		logFill(ctx, outcome.tradeID, side)
	}
}

func (e *DefaultExchange) publishOpenInterest(book *OrderBook, timestamp int64) {
	e.MDPublisher.PublishOpenInterest(book.Symbol, &OpenInterest{
		Symbol:         book.Symbol,
		TotalContracts: e.Positions.CalculateOpenInterest(book.Symbol),
		Timestamp:      timestamp,
	}, timestamp)
}

// recordFeeRevenue updates exchange fee balances and logs the revenue event.
// Each fee is booked into ITS OWN asset — clients are debited in Fee.Asset, so
// crediting revenue under any other asset would destroy one asset and mint
// another. defaultAsset covers fees with an unset asset (zero-fee probes).
// Caller must hold e.mu.Lock().
func (e *DefaultExchange) recordFeeRevenue(defaultAsset string, takerFee, makerFee Fee, book *OrderBook, timestamp int64) {
	if takerFee.Amount == 0 && makerFee.Amount == 0 {
		return
	}
	takerAsset, makerAsset := takerFee.Asset, makerFee.Asset
	if takerAsset == "" {
		takerAsset = defaultAsset
	}
	if makerAsset == "" {
		makerAsset = defaultAsset
	}
	e.moveVenueBalance(VenueFeeRevenue, takerAsset, takerFee.Amount, timestamp, book.Symbol, "taker_fee")
	e.moveVenueBalance(VenueFeeRevenue, makerAsset, makerFee.Amount, timestamp, book.Symbol, "maker_fee")

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

func logFill(ctx executionContext, tradeID uint64, side fillSide) {
	if ctx.log == nil {
		return
	}
	ctx.log.LogEvent(ctx.timestamp, side.clientID, "OrderFill", map[string]any{
		"order_id":        side.orderID,
		"symbol":          ctx.book.Symbol,
		"qty":             ctx.exec.Qty,
		"price":           ctx.exec.Price,
		"side":            side.side.String(),
		"position_side":   side.posSide.String(),
		"filled_qty":      side.filledQty,
		"remaining_qty":   side.totalQty - side.filledQty,
		"is_full":         side.isFull(),
		"trade_id":        tradeID,
		"role":            side.role,
		"fee_amount":      side.fee.Amount,
		"fee_asset":       side.fee.Asset,
		"realized_pnl":    side.realizedPnL,
		"new_size":        side.delta.NewSize,
		"new_entry_price": side.delta.NewEntryPrice,
	})
}

func sendFillNotification(gw *ClientGateway, ctx executionContext, tradeID uint64, side fillSide) {
	// Enqueued, not sent: fills are generated while the exchange lock is held,
	// and a blocking send there stalls the whole engine on one slow consumer.
	//
	// gw may be nil: unlike clients, gateways are dropped on disconnect while
	// the client's resting orders stay in the book and keep filling.
	// enqueueResponse is nil-safe, so those fills settle and are logged with
	// nobody listening, which is the intended behaviour.
	gw.enqueueResponse(Response{
		Success: true,
		Data: &FillNotification{
			OrderID:       side.orderID,
			ClientID:      side.clientID,
			TradeID:       tradeID,
			Symbol:        ctx.book.Symbol,
			Qty:           ctx.exec.Qty,
			Price:         ctx.exec.Price,
			Side:          side.side,
			PositionSide:  side.posSide,
			IsFull:        side.isFull(),
			FeeAmount:     side.fee.Amount,
			FeeAsset:      side.fee.Asset,
			RealizedPnL:   side.realizedPnL,
			NewSize:       side.delta.NewSize,
			NewEntryPrice: side.delta.NewEntryPrice,
			Timestamp:     ctx.exec.Timestamp,
		},
	})
}
