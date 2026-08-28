package exchange

import (
	"cmp"
	"fmt"
	"maps"
	"math"
	"slices"

	etypes "exchange_sim/types"
)

const (
	exchangeForcedFeeReservationReason = "EXCHANGE_FORCED_FEE_RESERVATION"
	exchangeForcedLifecycleReason      = "EXCHANGE_FORCED_LIFECYCLE"
	exchangeForcedSTPReason            = "EXCHANGE_FORCED_SELF_TRADE_PREVENTION"
	exchangeForcedBookAdmissionReason  = "EXCHANGE_FORCED_BOOK_ADMISSION"
)

// logExchangeForcedCancellation records a terminal order transition caused by
// venue mechanics rather than an actor cancel request. The absence of a
// request_id is deliberate: analyzers must use the physical event ordinal for
// these transitions, not invent an actor request that never existed.
func (e *DefaultExchange) logExchangeForcedCancellation(book *OrderBook, order *Order, remainingQty int64, reason string) {
	if book == nil || order == nil {
		return
	}
	if log := e.getLogger(book.Symbol); log != nil {
		log.LogEvent(e.Clock.NowUnixNano(), order.ClientID, "OrderCancelled", map[string]any{
			"order_id":      order.ID,
			"remaining_qty": remainingQty,
			"reason":        reason,
		})
	}
}

// acceptedOrderEvidence keeps an accepted order's established flat wire schema
// while retaining the client request that caused its admission. Order IDs alone
// cannot join a pre-submission actor decision to an accepted venue request.
// This wrapper is evidence only; RequestID is deliberately not live Order
// state because matching and settlement do not need it.
type acceptedOrderEvidence struct {
	*Order
	RequestID uint64 `json:"request_id"`
}

// rejectedOrderEvidence keeps rejection evidence tied to the attempted order,
// rather than making a failed response indistinguishable from a request that
// was never delivered. The embedded Response preserves the established flat
// response fields. This is evidence only; rejection remains non-mutating.
type rejectedOrderEvidence struct {
	Response
	Symbol      string      `json:"symbol"`
	Side        Side        `json:"side"`
	Type        OrderType   `json:"type"`
	TimeInForce TimeInForce `json:"time_in_force"`
	PostOnly    bool        `json:"post_only"`
	Price       int64       `json:"price"`
	Qty         int64       `json:"qty"`
}

func rejectedOrderEvidenceFromRequest(response Response, req *OrderRequest) rejectedOrderEvidence {
	return rejectedOrderEvidence{
		Response: response, Symbol: req.Symbol, Side: req.Side, Type: req.Type,
		TimeInForce: req.TimeInForce, PostOnly: req.PostOnly, Price: req.Price, Qty: req.Qty,
	}
}

func rejectedOrderEvidenceFromOrder(response Response, symbol string, order *Order) rejectedOrderEvidence {
	return rejectedOrderEvidence{
		Response: response, Symbol: symbol, Side: order.Side, Type: order.Type,
		TimeInForce: order.TimeInForce, PostOnly: order.PostOnly, Price: order.Price, Qty: order.Qty,
	}
}

func (e *DefaultExchange) PlaceOrder(clientID uint64, req *OrderRequest) Response {
	e.mu.Lock()
	defer e.mu.Unlock()

	if reject := e.validatePlaceOrder(clientID, req); reject != nil {
		return *reject
	}

	book, client, log := e.Books[req.Symbol], e.Clients[clientID], e.getLogger(req.Symbol)

	// Keep a candidate ID local until every price-dependent admission check
	// passes. In particular, an unavailable fee/collateral source must reject
	// before it changes even the observable order-ID sequence.
	orderID := e.NextOrderID + 1
	order := newOrderFromRequest(clientID, orderID, req, e.Clock.NowUnixNano())

	// FOK must be checked BEFORE matching: Match mutates the book, and
	// abandoning its executions would strip maker quantity without settlement.
	// Use the configured matcher against a detached copy rather than a separate
	// depth walk, so custom allocation, iceberg, and self-trade rules cannot
	// disagree with the atomicity check.
	if req.TimeInForce == FOK && !e.canPreviewFullyMatch(book, order) {
		return e.rejectOrder(order, req.RequestID, clientID, book.Symbol, RejectFOKNotFilled, log)
	}
	// Preflight every currently executable pair's fee before reservations or
	// auto-borrow. A maker's configured external fee source is just as much a
	// precondition of this client action as the incoming client's source.
	feePlan, feeFailure := e.prepareFeeExecutionPlan(book, order, nil)
	if feeFailure != nil {
		if feeFailure.err != nil {
			e.reportPriceUnavailable(e.Clock.NowUnixNano(), book.Symbol, "fee_match_preflight", feeFailure.err)
			return e.rejectOrder(order, req.RequestID, clientID, book.Symbol, RejectPriceUnavailable, log)
		}
		panic("matching engine could not produce fee preflight")
	}

	borrowSnapshot := takeSpotBorrowSnapshot(client)
	if reject := e.reserveOrderFunds(client, book, order, req.RequestID, log); reject != nil {
		borrowSnapshot.restore(e, clientID, client)
		return *reject
	}

	var spotPlan *spotExecutionPlan
	if _, settlesOwnLedger := book.Instrument.(Settleable); !settlesOwnLedger {
		excludedMakers := make(map[uint64]struct{})
		for {
			var failure *spotPlanFailure
			spotPlan, failure = e.prepareSpotExecutionPlan(book, order, excludedMakers, feePlan)
			if failure == nil {
				break
			}
			if failure.err != nil {
				releaseReserved(client, book.Instrument, order)
				borrowSnapshot.restore(e, clientID, client)
				e.reportPriceUnavailable(e.Clock.NowUnixNano(), book.Symbol, "fee_preflight", failure.err)
				return e.rejectOrder(order, req.RequestID, clientID, book.Symbol, RejectPriceUnavailable, log)
			}
			if failure.makerOrderID == 0 {
				releaseReserved(client, book.Instrument, order)
				borrowSnapshot.restore(e, clientID, client)
				return e.rejectOrder(order, req.RequestID, clientID, book.Symbol, RejectInsufficientBalance, log)
			}
			excludedMakers[failure.makerOrderID] = struct{}{}
			// The next dry run excludes a maker. Its execution frontier can
			// therefore differ from the initial preflight, so acquire a fresh
			// exact fee plan before comparing it with live matching.
			feePlan = nil
		}
		if req.TimeInForce == FOK && !spotPlan.fullyFilled {
			releaseReserved(client, book.Instrument, order)
			borrowSnapshot.restore(e, clientID, client)
			return e.rejectOrder(order, req.RequestID, clientID, book.Symbol, RejectFOKNotFilled, log)
		}
		if len(excludedMakers) > 0 {
			makerIDs := make([]uint64, 0, len(excludedMakers))
			for makerID := range excludedMakers {
				makerIDs = append(makerIDs, makerID)
			}
			slices.Sort(makerIDs)
			for _, makerID := range makerIDs {
				if !e.cancelUnfundedSpotPlanMaker(book, makerID) {
					panic("spot execution plan maker disappeared before commit")
				}
			}
			var failure *spotPlanFailure
			spotPlan, failure = e.prepareSpotExecutionPlan(book, order, nil, nil)
			if failure != nil {
				if failure.err != nil {
					releaseReserved(client, book.Instrument, order)
					borrowSnapshot.restore(e, clientID, client)
					e.reportPriceUnavailable(e.Clock.NowUnixNano(), book.Symbol, "fee_preflight", failure.err)
					return e.rejectOrder(order, req.RequestID, clientID, book.Symbol, RejectPriceUnavailable, log)
				}
				panic("spot execution plan changed during commit")
			}
		}
	} else {
		spotPlan = feePlan
		if spotPlan == nil {
			panic("derivative fee preflight unexpectedly missing")
		}
	}

	// All preflight paths above are non-mutating or have a matching rollback.
	// Committing the ID here makes client-visible admission atomic with respect
	// to unavailable configured price sources.
	e.NextOrderID = orderID
	if log != nil {
		log.LogEvent(e.Clock.NowUnixNano(), clientID, "OrderAccepted", acceptedOrderEvidence{
			Order:     order,
			RequestID: req.RequestID,
		})
	}

	result := e.Matcher.Match(book.Bids, book.Asks, order)
	if spotPlan != nil && !spotPlan.matches(result.Executions) {
		// The matcher was just run against a detached copy while the exchange
		// lock was held. Settling a different sequence would use unvalidated
		// fees and can violate reservations, so fail loudly rather than minting
		// a plausible but inconsistent ledger.
		panic("matching engine violated spot execution plan")
	}
	if req.TimeInForce == FOK && !result.FullyFilled {
		// The matcher was just run against an identical detached book while the
		// exchange lock was held. Reaching this branch means a stateful or
		// nondeterministic matcher violated the matching-engine contract. We
		// cannot honestly report a rejected FOK here: its live-book mutations
		// have already happened. Fail loudly instead of returning a response that
		// leaves liquidity and settlement ledgers inconsistent.
		panic("matching engine violated FOK preflight")
	}

	levels := collectAffectedLevels(book, result.Executions)
	e.processExecutions(book, result.Executions, order, spotPlan)
	e.removeMakerOrders(book, result.Executions)
	e.publishLevels(book, levels)
	e.restOrReleaseOrder(client, book, order, req, log)

	return Response{RequestID: req.RequestID, Success: true, Data: e.NextOrderID}
}

// spotBorrowSnapshot makes order admission transactional when auto-borrow is
// enabled. Fee-plan validation happens after the ordinary reservation path; a
// rejected order must not leave cash and debt from a loan whose only purpose
// was that rejected order.
type spotBorrowSnapshot struct {
	balances     map[string]int64
	borrowed     map[string]int64
	borrowedSpot map[string]int64
}

func takeSpotBorrowSnapshot(client *Client) spotBorrowSnapshot {
	return spotBorrowSnapshot{
		balances:     maps.Clone(client.Balances),
		borrowed:     maps.Clone(client.Borrowed),
		borrowedSpot: maps.Clone(client.BorrowedSpot),
	}
}

func (s spotBorrowSnapshot) restore(e *DefaultExchange, clientID uint64, client *Client) {
	if client == nil || (maps.Equal(s.balances, client.Balances) && maps.Equal(s.borrowed, client.Borrowed) && maps.Equal(s.borrowedSpot, client.BorrowedSpot)) {
		return
	}
	timestamp := e.Clock.NowUnixNano()
	changes := make([]BalanceDelta, 0, len(s.balances)+len(s.borrowed))
	rollbacks := make([]RepayEvent, 0, len(s.borrowed))
	assets := make(map[string]struct{}, len(s.balances)+len(client.Balances)+len(s.borrowed)+len(client.Borrowed))
	for asset := range s.balances {
		assets[asset] = struct{}{}
	}
	for asset := range client.Balances {
		assets[asset] = struct{}{}
	}
	for asset := range s.borrowed {
		assets[asset] = struct{}{}
	}
	for asset := range client.Borrowed {
		assets[asset] = struct{}{}
	}
	assetNames := make([]string, 0, len(assets))
	for asset := range assets {
		assetNames = append(assetNames, asset)
	}
	slices.Sort(assetNames)
	for _, asset := range assetNames {
		if old, next := client.Balances[asset], s.balances[asset]; old != next {
			changes = append(changes, spotDelta(asset, old, next))
		}
		if old, next := client.Borrowed[asset], s.borrowed[asset]; old != next {
			changes = append(changes, borrowedDelta(asset, old, next))
			// The only debt this spot-admission snapshot can add is an
			// auto_spot borrow. Emit its inverse so an event replay reaches the
			// same debt state as the restored live ledger.
			if old > next {
				rollbacks = append(rollbacks, RepayEvent{
					Timestamp:     timestamp,
					ClientID:      clientID,
					Asset:         asset,
					Principal:     old - next,
					RemainingDebt: next,
					Reason:        "order_admission_rollback",
				})
			}
		}
	}
	client.Balances = maps.Clone(s.balances)
	client.Borrowed = maps.Clone(s.borrowed)
	client.BorrowedSpot = maps.Clone(s.borrowedSpot)
	if len(changes) > 0 {
		logBalanceChange(e, timestamp, clientID, "", "auto_borrow_rollback", changes)
	}
	if log := e.getLogger("_global"); log != nil {
		for _, rollback := range rollbacks {
			log.LogEvent(timestamp, clientID, "repay", rollback)
		}
	}
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
		var markPrice *int64
		var markErr error
		if book != nil {
			price, err := liveBookReferencePrice(book)
			if err != nil {
				markErr = fmt.Errorf("display mark for %s: %w", pos.Symbol, err)
			} else {
				markPrice = &price
			}
		} else {
			markErr = fmt.Errorf("display mark for %s: %w", pos.Symbol, ErrNoBookPrice)
		}
		var unrealizedPnL int64
		if markPrice != nil {
			if instrument := e.Instruments[pos.Symbol]; instrument != nil {
				unrealizedPnL = positionUPnL(&pos, *markPrice, instrument.BasePrecision())
			}
		}
		markReason := ""
		if markErr != nil {
			markReason = markErr.Error()
		}
		snapshots = append(snapshots, PositionSnapshot{
			Symbol:                pos.Symbol,
			PositionSide:          pos.PositionSide,
			Size:                  pos.Size,
			EntryPrice:            pos.EntryPrice,
			MarkPrice:             markPrice,
			MarkUnavailableReason: markReason,
			UnrealizedPnL:         unrealizedPnL,
			MarginType:            CrossMargin,
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

	// The reference-data feed has no book: subscribe directly, then replay what
	// is already listed.
	//
	// Without the replay a participant only ever learns about contracts listed
	// after it connected, and never about the ones that were there when it
	// arrived. That is invisible in a short run and fatal in a long one: an
	// actor whose first tick lands a second after the opening chain was listed
	// spends the entire simulation unaware that the chain exists.
	if req.Symbol == InstrumentFeedSymbol {
		types := req.Types
		if len(types) == 0 {
			types = []MDType{MDInstrument}
		}
		e.MDPublisher.Subscribe(clientID, req.Symbol, types, gateway)
		e.replayListedInstruments(clientID, gateway)
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

	if log := e.getLogger(req.Symbol); log != nil {
		log.LogEvent(e.Clock.NowUnixNano(), clientID, "BookSnapshot", map[string]any{
			"bids": book.Bids.GetSnapshot(),
			"asks": book.Asks.GetSnapshot(),
		})
	}

	return Response{RequestID: req.RequestID, Success: true}
}

// Unsubscribe removes a market-data subscription. Requests routed through a
// gateway pass that session so a delayed request from a replaced connection
// cannot unsubscribe the current connection.
func (e *DefaultExchange) Unsubscribe(clientID uint64, req *QueryRequest, gateway ...*ClientGateway) Response {
	if len(gateway) > 0 {
		e.MDPublisher.Unsubscribe(clientID, req.Symbol, gateway[0])
	} else {
		e.MDPublisher.Unsubscribe(clientID, req.Symbol)
	}
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

	if log := e.getLogger(book.Symbol); log != nil {
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
		resp := rejectWithLog(req, clientID, reason, log, e.Clock)
		return &resp
	}
	if e.Clients[clientID] == nil {
		return reject(RejectUnknownClient)
	}
	book := e.Books[req.Symbol]
	if book == nil {
		return reject(RejectUnknownInstrument)
	}
	if exp, ok := book.Instrument.(Expirable); ok && e.Clock.NowUnixNano() >= exp.ExpiryNano() {
		return reject(RejectInstrumentExpired)
	}
	if req.PostOnly && (req.Type != LimitOrder || req.TimeInForce != GTC) {
		return reject(RejectPostOnlyInvalid)
	}
	if req.Type == LimitOrder && !book.Instrument.ValidatePrice(req.Price) {
		return reject(RejectInvalidPrice)
	}
	if !book.Instrument.ValidateQty(req.Qty) {
		return reject(RejectInvalidQty)
	}
	if _, ok := etypes.TryMulDiv(req.Qty, req.Price, book.Instrument.BasePrecision()); !ok {
		return reject(RejectInvalidQty)
	}
	switch req.Visibility {
	case Normal, Iceberg, Hidden:
		// Valid visibility modes.
	default:
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
	if e.positionExposureViolation(clientID, book, req) {
		return reject(RejectExceedsPosition)
	}
	if e.restingLevelAggregateViolation(clientID, book, req) {
		return reject(RejectInvalidQty)
	}
	// A post-only contract is evaluated against the actual book under the
	// venue lock, after request latency has elapsed, before an order ID,
	// reservation, auto-borrow, or matching mutation exists. A client-side
	// snapshot cannot make this decision: it can be stale by arrival.
	if req.PostOnly && postOnlyWouldTake(book, req) {
		return reject(RejectPostOnlyWouldTake)
	}
	return nil
}

func postOnlyWouldTake(book *OrderBook, req *OrderRequest) bool {
	if book == nil || req == nil {
		return false
	}
	if req.Side == Buy {
		return book.Asks != nil && book.Asks.Best != nil && req.Price >= book.Asks.Best.Price
	}
	return book.Bids != nil && book.Bids.Best != nil && req.Price <= book.Bids.Best.Price
}

// restingLevelAggregateViolation checks the exact residual that a GTC limit
// order would leave after matching, before reservations or live-book mutation.
// A level aggregate is part of the public book contract; allowing it to wrap
// would corrupt depth, pro-rata allocation, and mark inputs.
func (e *DefaultExchange) restingLevelAggregateViolation(clientID uint64, book *OrderBook, req *OrderRequest) bool {
	if req.Type != LimitOrder || req.TimeInForce != GTC {
		return false
	}
	side := book.Bids
	if req.Side == Sell {
		side = book.Asks
	}
	limit := side.Limits[req.Price]
	if limit == nil {
		return false
	}
	if limit.TotalQty < 0 || limit.OrderCnt < 0 || limit.OrderCnt == math.MaxInt32 {
		return true
	}
	candidate := Order{
		ClientID:     clientID,
		Side:         req.Side,
		PositionSide: req.PositionSide,
		Type:         req.Type,
		TimeInForce:  req.TimeInForce,
		PostOnly:     req.PostOnly,
		Price:        req.Price,
		Qty:          req.Qty,
		Visibility:   req.Visibility,
		IcebergQty:   req.IcebergQty,
	}
	result, ok := e.previewMatch(book, &candidate)
	if !ok {
		return true
	}
	defer releasePreviewExecutions(result.Executions)
	remaining, ok := previewRemainingQty(result.Executions, req.Qty)
	if !ok {
		return true
	}
	if remaining == 0 {
		return false
	}
	_, ok = etypes.TryAdd(limit.TotalQty, remaining)
	return !ok
}

func previewRemainingQty(executions []*Execution, totalQty int64) (int64, bool) {
	var filled int64
	for _, exec := range executions {
		if exec == nil {
			return 0, false
		}
		var ok bool
		filled, ok = etypes.TryAdd(filled, exec.Qty)
		if !ok || filled > totalQty {
			return 0, false
		}
	}
	return etypes.TrySub(totalQty, filled)
}

// positionExposureViolation reserves signed position headroom for every
// same-direction resting order, preventing a later valid match from wrapping
// position size after the matcher has already mutated book state.
func (e *DefaultExchange) positionExposureViolation(clientID uint64, book *OrderBook, req *OrderRequest) bool {
	if req.PositionSide == PositionLong && req.Side == Sell || req.PositionSide == PositionShort && req.Side == Buy {
		return false
	}
	var size int64
	if pos := e.Positions.GetPositionBySide(clientID, req.Symbol, req.PositionSide); pos != nil {
		size = pos.Size
	}
	orders := book.Bids.Orders
	if req.Side == Sell {
		orders = book.Asks.Orders
	}
	for _, order := range orders {
		if order.ClientID != clientID || order.PositionSide != req.PositionSide {
			continue
		}
		remaining := order.Qty - order.FilledQty
		var ok bool
		if req.Side == Buy {
			size, ok = etypes.TryAdd(size, remaining)
		} else {
			size, ok = etypes.TrySub(size, remaining)
		}
		if !ok {
			return true
		}
	}
	if req.Side == Buy {
		result, ok := etypes.TryAdd(size, req.Qty)
		return !ok || result == math.MinInt64
	}
	result, ok := etypes.TrySub(size, req.Qty)
	return !ok || result == math.MinInt64
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
	order.PostOnly = req.PostOnly
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

// liveBookReferencePrice returns the declared live-book reference for valuation and
// initial risk: a true midpoint, then the sole displayed ask or bid. Last
// trade is deliberately not a hidden fallback; callers that accept it must
// opt in to that policy explicitly.
func liveBookReferencePrice(book *OrderBook) (int64, error) {
	if book == nil {
		return 0, ErrNoBookPrice
	}
	if mid, err := book.GetMidPrice(); err == nil {
		return mid, nil
	}
	if ask, err := book.GetBestAsk(); err == nil {
		return ask, nil
	}
	if bid, err := book.GetBestBid(); err == nil {
		return bid, nil
	}
	return 0, ErrNoBookPrice
}

// checkMarketOrderFunds prices the exact executions that the configured
// matcher would produce against a cloned book. A top-of-book or midpoint
// reference understates deep sweeps; unchecked aggregate arithmetic then
// turns an unaffordable order into a balance underflow after matching.
func (e *DefaultExchange) checkMarketOrderFunds(client *Client, book *OrderBook, order *Order, precision int64) (bool, error) {
	executions, ok := e.previewMarketExecutions(book, order)
	if !ok {
		return false, nil
	}
	defer releasePreviewExecutions(executions)

	instrument := book.Instrument
	quote := instrument.QuoteAsset()
	if om, ok := instrument.(OrderMarginer); ok {
		required, ok, err := derivativeMarketRequirement(client.FeePlan, instrument, order.Side, executions, precision,
			func(side Side, qty, price, precision int64) (int64, error) {
				return om.MarginForMarketOrder(side, qty, price, precision), nil
			})
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
		return e.fundMarketRequirement(order.ClientID, client, quote, required, true)
	}
	if m, ok := instrument.(Margined); ok {
		required, ok, err := derivativeMarketRequirement(client.FeePlan, instrument, order.Side, executions, precision,
			func(_ Side, qty, price, precision int64) (int64, error) {
				return m.MarginForMarket(qty, price, precision)
			})
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
		return e.fundMarketRequirement(order.ClientID, client, quote, required, true)
	}

	asset := reserveAsset(instrument, order.Side)
	required, ok, err := spotMarketRequirement(client.FeePlan, instrument, order.Side, executions, precision)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	return e.fundMarketRequirement(order.ClientID, client, asset, required, false)
}

// fundMarketRequirement reports whether a market order's immediate settlement
// requirement is covered, borrowing the shortfall when auto-borrow permits it.
//
// Market orders settle instead of reserving, so they never reach
// tryReserveOrBorrow. Without this, an account with margin enabled could fund a
// limit order but not the economically identical market order, which silently
// disabled every market-order hedge for an account holding no inventory in the
// sold asset. Admission is already wrapped in a spot borrow snapshot, so a loan
// taken here is rolled back with the order if a later check rejects it.
func (e *DefaultExchange) fundMarketRequirement(clientID uint64, client *Client, asset string, required int64, isPerp bool) (bool, error) {
	available := func() int64 {
		if isPerp {
			return client.PerpAvailable(asset)
		}
		return client.GetAvailable(asset)
	}
	if available() >= required {
		return true, nil
	}
	if e.BorrowingMgr == nil {
		return false, nil
	}
	cfg := e.BorrowingMgr.Config
	if isPerp && !cfg.AutoBorrowPerp {
		return false, nil
	}
	if !isPerp && !cfg.AutoBorrowSpot {
		return false, nil
	}
	shortfall, ok := etypes.TrySub(required, available())
	if !ok || shortfall <= 0 {
		return false, nil
	}
	reason := "auto_spot"
	if isPerp {
		reason = "auto_perp"
	}
	ctx := buildBorrowContext(e, client, clientID)
	ctx.CreditSpot = !isPerp
	if err := e.BorrowingMgr.BorrowMargin(ctx, asset, shortfall, reason); err != nil {
		return false, err
	}
	return available() >= required, nil
}

// previewMarketExecutions matches a copy of the book so admission and the
// real match share exactly the same depth, self-trade, iceberg, and pro-rata
// semantics. The preview is only an affordability calculation; it never
// touches order reservations or the live book.
func (e *DefaultExchange) previewMarketExecutions(book *OrderBook, order *Order) (executions []*Execution, ok bool) {
	result, ok := e.previewMatch(book, order)
	if !ok {
		return nil, false
	}
	executions = result.Executions
	return executions, true
}

// canPreviewFullyMatch evaluates FOK against the exact matching engine on a
// detached book. The executions are pooled by matchers, so every preview path
// must return them before the live book is touched.
func (e *DefaultExchange) canPreviewFullyMatch(book *OrderBook, order *Order) bool {
	result, ok := e.previewMatch(book, order)
	if !ok {
		return false
	}
	defer releasePreviewExecutions(result.Executions)
	return result.FullyFilled
}

func (e *DefaultExchange) previewMatch(book *OrderBook, order *Order) (*MatchResult, bool) {
	return e.previewMatchExcluding(book, order, nil)
}

func (e *DefaultExchange) previewMatchExcluding(book *OrderBook, order *Order, excluded map[uint64]struct{}) (*MatchResult, bool) {
	if e.Matcher == nil || !marketDepthSaneExcluding(book, order, excluded) {
		return nil, false
	}
	bids, ok := cloneBookForPreviewExcluding(book.Bids, excluded)
	if !ok {
		return nil, false
	}
	asks, ok := cloneBookForPreviewExcluding(book.Asks, excluded)
	if !ok {
		return nil, false
	}
	incoming := *order
	incoming.Prev = nil
	incoming.Next = nil
	incoming.Parent = nil
	incoming.FeeReserved = nil

	result := e.Matcher.Match(bids, asks, &incoming)
	if result == nil {
		return nil, false
	}
	executions := result.Executions
	var filled int64
	for _, exec := range executions {
		if exec == nil || exec.Qty <= 0 || !book.Instrument.ValidatePrice(exec.Price) || exec.TakerClientID != order.ClientID {
			releasePreviewExecutions(executions)
			return nil, false
		}
		var addOK bool
		filled, addOK = etypes.TryAdd(filled, exec.Qty)
		if !addOK || filled > order.Qty {
			releasePreviewExecutions(executions)
			return nil, false
		}
	}
	if result.FullyFilled != (filled == order.Qty) {
		releasePreviewExecutions(executions)
		return nil, false
	}
	return result, true
}

func releasePreviewExecutions(executions []*Execution) {
	for _, exec := range executions {
		if exec != nil {
			PutExecution(exec)
		}
	}
}

// cloneBookForPreview copies only live queue state. Parent/next links must be
// rebuilt by AddOrder, otherwise a dry run could mutate the live queue.
func cloneBookForPreview(source *Book) (*Book, bool) {
	return cloneBookForPreviewExcluding(source, nil)
}

func cloneBookForPreviewExcluding(source *Book, excluded map[uint64]struct{}) (*Book, bool) {
	// A preview has no lifetime beyond this admission check. Give its indexes
	// only the live source cardinality rather than the long-lived venue-book
	// defaults; capacity does not enter matching or queue semantics.
	clone := newBookWithCapacity(source.Side, len(source.Orders), len(source.Limits))
	for limit := source.ActiveHead; limit != nil; limit = limit.Next {
		for order := limit.Head; order != nil; order = order.Next {
			if _, skip := excluded[order.ID]; skip {
				continue
			}
			remaining, ok := etypes.TrySub(order.Qty, order.FilledQty)
			if !ok || remaining < 0 {
				return nil, false
			}
			copy := *order
			copy.Prev = nil
			copy.Next = nil
			copy.Parent = nil
			copy.FeeReserved = nil
			if !clone.AddOrder(&copy) {
				return nil, false
			}
		}
	}
	return clone, true
}

// marketDepthSane rejects a corrupt or unrepresentable aggregate before a
// matcher can use wrapped depth. This is especially important for pro-rata,
// whose allocation denominator is the sum at a price level.
func marketDepthSane(book *OrderBook, order *Order) bool {
	return marketDepthSaneExcluding(book, order, nil)
}

func marketDepthSaneExcluding(book *OrderBook, order *Order, excluded map[uint64]struct{}) bool {
	levels := book.Asks
	if order.Side == Sell {
		levels = book.Bids
	}
	for limit := levels.ActiveHead; limit != nil; limit = limit.Next {
		var levelQty int64
		for resting := limit.Head; resting != nil; resting = resting.Next {
			if _, skip := excluded[resting.ID]; skip {
				continue
			}
			if resting.ClientID == order.ClientID {
				continue
			}
			remaining, ok := etypes.TrySub(resting.Qty, resting.FilledQty)
			if !ok || remaining < 0 {
				return false
			}
			if levelQty, ok = etypes.TryAdd(levelQty, remaining); !ok {
				return false
			}
		}
	}
	return true
}

// derivativeMarketRequirement sums margin and the actual taker quote fee for
// every executable fill. It deliberately ignores later margin releases from
// closing positions: admission must be conservative before a market sweep
// mutates positions and cannot rely on a favourable execution path.
func derivativeMarketRequirement(
	feePlan FeeModel, instrument Instrument, side Side, executions []*Execution, precision int64,
	margin func(side Side, qty, price, precision int64) (int64, error),
) (int64, bool, error) {
	var required int64
	for _, exec := range executions {
		marginRequired, err := margin(side, exec.Qty, exec.Price, precision)
		if err != nil {
			return 0, false, fmt.Errorf("market margin at price %d for quantity %d: %w", exec.Price, exec.Qty, err)
		}
		var ok bool
		required, ok = etypes.TryAdd(required, marginRequired)
		if !ok {
			return 0, false, nil
		}
		fee, err := executionFee(feePlan, instrument, exec, false, precision)
		if err != nil {
			return 0, false, err
		}
		if fee.Asset == instrument.QuoteAsset() && fee.Amount > 0 {
			required, ok = etypes.TryAdd(required, fee.Amount)
			if !ok {
				return 0, false, nil
			}
		}
	}
	return required, true, nil
}

// spotMarketRequirement sums principal plus only positive taker fees in the
// asset that must be supplied by this order. Fees are computed per execution:
// a fixed fee applies once per counterparty fill, not once per price level.
func spotMarketRequirement(feePlan FeeModel, instrument Instrument, side Side, executions []*Execution, precision int64) (int64, bool, error) {
	asset := reserveAsset(instrument, side)
	var required int64
	for _, exec := range executions {
		principal := exec.Qty
		var ok bool
		if side == Buy {
			principal, ok = etypes.TryMulDiv(exec.Qty, exec.Price, precision)
			if !ok {
				return 0, false, nil
			}
		}
		required, ok = etypes.TryAdd(required, principal)
		if !ok {
			return 0, false, nil
		}
		fee, err := executionFee(feePlan, instrument, exec, false, precision)
		if err != nil {
			return 0, false, err
		}
		if fee.Asset == asset && fee.Amount > 0 {
			required, ok = etypes.TryAdd(required, fee.Amount)
			if !ok {
				return 0, false, nil
			}
		}
	}
	return required, true, nil
}

func executionFee(feePlan FeeModel, instrument Instrument, exec *Execution, isMaker bool, precision int64) (Fee, error) {
	if feePlan == nil {
		return Fee{}, nil
	}
	return feePlan.CalculateFee(FillContext{
		Exec:       exec,
		IsMaker:    isMaker,
		BaseAsset:  instrument.BaseAsset(),
		QuoteAsset: instrument.QuoteAsset(),
		Precision:  precision,
	})
}

func (e *DefaultExchange) reserveLimitOrderFunds(client *Client, instrument Instrument, order *Order, precision int64) (bool, error) {
	base, quote := instrument.BaseAsset(), instrument.QuoteAsset()
	if om, ok := instrument.(OrderMarginer); ok {
		// Reserve margin AND the worst-case fee: the fee is debited from the same
		// quote wallet at settlement, so an order funded to the last cent of
		// margin would go insolvent the instant it fills. Reserving both here
		// lets the exchange reject the order up front instead.
		margin := om.MarginForOrder(order.Side, order.Qty, order.Price, precision)
		fee, err := quoteFeeHeadroom(client.FeePlan, base, quote, order.Qty, order.Price, precision)
		if err != nil {
			return false, err
		}
		if margin < 0 {
			return false, nil
		}
		total, ok := etypes.TryAdd(margin, fee)
		if !ok {
			return false, nil
		}
		if ok, err := e.tryReserveOrBorrow(order.ClientID, quote, total, client.ReservePerp, true); !ok {
			return false, err
		}
		order.Reserved = total
		return true, nil
	}
	if m, ok := instrument.(Margined); ok {
		margin, err := m.MarginRequired(order.Qty, order.Price, precision)
		if err != nil {
			return false, fmt.Errorf("limit-order margin for %s: %w", instrument.Symbol(), err)
		}
		fee, err := quoteFeeHeadroom(client.FeePlan, base, quote, order.Qty, order.Price, precision)
		if err != nil {
			return false, err
		}
		total, ok := etypes.TryAdd(margin, fee)
		if !ok {
			return false, nil
		}
		if ok, err := e.tryReserveOrBorrow(order.ClientID, quote, total, client.ReservePerp, true); !ok {
			return false, err
		}
		order.Reserved = total
		return true, nil
	}
	asset := reserveAsset(instrument, order.Side)
	amount, ok, err := spotOrderReservation(client.FeePlan, instrument, order.Side, order.Qty, order.Price, precision)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	if ok, err := e.tryReserveOrBorrow(order.ClientID, asset, amount, client.Reserve, false); !ok {
		return false, err
	}
	order.Reserved = amount
	return true, nil
}

// checkForeignFeeFunds reports whether the client can cover the fee portion
// not backed by the order's supplied trade leg. On spot market orders it also
// reserves the maximum running shortfall in the received asset: a fee can be
// larger than the execution proceeds, so merely netting it against that leg
// can otherwise settle the account below zero.
func (e *DefaultExchange) checkForeignFeeFunds(client *Client, book *OrderBook, order *Order, precision int64) (bool, error) {
	fees, ok, err := e.foreignFeeReservations(client.FeePlan, book, order, precision)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	for _, asset := range sortedAssetNames(fees) {
		amount := fees[asset]
		if isMarginedInstrument(book.Instrument) {
			if client.PerpAvailable(asset) < amount {
				return false, nil
			}
		} else if client.GetAvailable(asset) < amount {
			return false, nil
		}
	}
	return true, nil
}

func isMarginedInstrument(instrument Instrument) bool {
	if _, ok := instrument.(Margined); ok {
		return true
	}
	_, ok := instrument.(OrderMarginer)
	return ok
}

// foreignFeeReservations returns a per-asset reserve for fees not backed by
// the order's supplied leg. A resting order may fill as maker or taker, so
// both schedules are considered and the larger requirement per asset is
// locked. A market order is based on a detached run of the configured matcher,
// because its number and allocation of executions determine fixed fees.
func (e *DefaultExchange) foreignFeeReservations(feePlan FeeModel, book *OrderBook, order *Order, precision int64) (map[string]int64, bool, error) {
	if feePlan == nil || order.Qty <= 0 {
		return nil, true, nil
	}
	instrument := book.Instrument
	if order.Type == Market {
		executions, ok := e.previewMarketExecutions(book, order)
		if !ok {
			return nil, false, nil
		}
		defer releasePreviewExecutions(executions)
		return marketForeignFeeReservations(feePlan, instrument, order, executions, precision)
	}
	remaining, ok := etypes.TrySub(order.Qty, order.FilledQty)
	if !ok || remaining <= 0 {
		return nil, remaining == 0, nil
	}
	price := order.Price
	base, quote := instrument.BaseAsset(), instrument.QuoteAsset()
	margined := isMarginedInstrument(instrument)
	reserved := make(map[string]int64)
	for _, isMaker := range []bool{false, true} {
		probe := Execution{Price: price, Qty: remaining}
		fee, err := feePlan.CalculateFee(FillContext{
			Exec: &probe, IsMaker: isMaker, BaseAsset: base, QuoteAsset: quote, Precision: precision,
		})
		if err != nil {
			return nil, false, err
		}
		if fee.Amount <= 0 || fee.Asset == "" || fee.Asset == quote || (!margined && fee.Asset == base) {
			continue
		}
		if fee.Amount > reserved[fee.Asset] {
			reserved[fee.Asset] = fee.Amount
		}
	}
	return reserved, true, nil
}

// cancelUnfundedSpotPlanMaker removes a resting maker before any live match
// when the exact next execution batch proves it cannot settle. It deliberately
// happens before matcher mutation, unlike the older reactive fee cancellation
// path that could leave already-built iceberg executions behind.
func (e *DefaultExchange) cancelUnfundedSpotPlanMaker(book *OrderBook, orderID uint64) bool {
	order := book.FindOrder(orderID)
	if order == nil || order.Parent == nil {
		return false
	}
	client := e.Clients[order.ClientID]
	if client == nil {
		return false
	}
	remaining, ok := etypes.TrySub(order.Qty, order.FilledQty)
	if !ok || remaining < 0 {
		return false
	}
	e.logExchangeForcedCancellation(book, order, remaining, exchangeForcedFeeReservationReason)
	releaseReserved(client, book.Instrument, order)
	if order.Side == Buy {
		book.Bids.CancelOrder(order.ID)
	} else {
		book.Asks.CancelOrder(order.ID)
	}
	if order.Visibility != Hidden {
		e.publishBookUpdate(book, order.Side, order.Price)
	}
	client.RemoveOrder(order.ID)
	order.Status = Cancelled
	if gateway := e.Gateways[order.ClientID]; gateway != nil && gateway.IsRunning() {
		gateway.enqueueResponse(Response{Success: true, Data: &ForcedCancelNotification{OrderID: order.ID, RemainingQty: remaining}})
	}
	putOrder(order)
	return true
}

func marketForeignFeeReservations(feePlan FeeModel, instrument Instrument, order *Order, executions []*Execution, precision int64) (map[string]int64, bool, error) {
	base, quote := instrument.BaseAsset(), instrument.QuoteAsset()
	margined := isMarginedInstrument(instrument)
	fundingAsset := reserveAsset(instrument, order.Side)
	receivedAsset := ""
	if !margined {
		if order.Side == Buy {
			receivedAsset = base
		} else {
			receivedAsset = quote
		}
	}
	reserved := make(map[string]int64)
	var receivedNet int64
	for _, exec := range executions {
		if exec == nil || exec.Qty <= 0 || !instrument.ValidatePrice(exec.Price) {
			return nil, false, nil
		}
		if receivedAsset != "" {
			received := exec.Qty
			var ok bool
			if order.Side == Sell {
				received, ok = etypes.TryMulDiv(exec.Qty, exec.Price, precision)
				if !ok {
					return nil, false, nil
				}
			}
			receivedNet, ok = etypes.TryAdd(receivedNet, received)
			if !ok {
				return nil, false, nil
			}
		}

		fee, err := executionFee(feePlan, instrument, exec, false, precision)
		if err != nil {
			return nil, false, err
		}
		if fee.Amount <= 0 || fee.Asset == "" {
			continue
		}
		if fee.Asset == receivedAsset {
			var ok bool
			receivedNet, ok = etypes.TrySub(receivedNet, fee.Amount)
			if !ok {
				return nil, false, nil
			}
			if receivedNet < 0 {
				shortfall, ok := etypes.TrySub(0, receivedNet)
				if !ok {
					return nil, false, nil
				}
				if shortfall > reserved[fee.Asset] {
					reserved[fee.Asset] = shortfall
				}
			}
			continue
		}
		if fee.Asset == fundingAsset {
			// checkMarketOrderFunds already includes positive fees charged in
			// the asset the order must supply.
			continue
		}
		amount, ok := etypes.TryAdd(reserved[fee.Asset], fee.Amount)
		if !ok {
			return nil, false, nil
		}
		reserved[fee.Asset] = amount
	}
	return reserved, true, nil
}

func (e *DefaultExchange) reserveForeignFeeFunds(client *Client, book *OrderBook, order *Order, precision int64) (bool, error) {
	instrument := book.Instrument
	fees, ok, err := e.foreignFeeReservations(client.FeePlan, book, order, precision)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	if len(fees) == 0 {
		return true, nil
	}
	margined := isMarginedInstrument(instrument)
	for asset, amount := range fees {
		if margined {
			if !client.ReservePerp(asset, amount) {
				releaseForeignFeeReservation(client, instrument, order)
				return false, nil
			}
		} else if !client.Reserve(asset, amount) {
			releaseForeignFeeReservation(client, instrument, order)
			return false, nil
		}
		if order.FeeReserved == nil {
			order.FeeReserved = make(map[string]int64, len(fees))
		}
		order.FeeReserved[asset] = amount
	}
	return true, nil
}

func releaseForeignFeeReservation(client *Client, instrument Instrument, order *Order) {
	if len(order.FeeReserved) == 0 {
		return
	}
	for _, asset := range sortedAssetNames(order.FeeReserved) {
		amount := order.FeeReserved[asset]
		if isMarginedInstrument(instrument) {
			client.ReleasePerp(asset, amount)
		} else {
			client.Release(asset, amount)
		}
	}
	order.FeeReserved = nil
}

func (e *DefaultExchange) restoreForeignFeeReservation(book *OrderBook, order *Order, precision int64) {
	if order == nil || order.Type == Market || len(order.FeeReserved) == 0 {
		return
	}
	client := e.Clients[order.ClientID]
	if client == nil {
		return
	}
	releaseForeignFeeReservation(client, book.Instrument, order)
	if order.FilledQty < order.Qty {
		ok, err := e.reserveForeignFeeFunds(client, book, order, precision)
		if !ok {
			if isPriceUnavailable(err) {
				e.reportPriceUnavailable(e.Clock.NowUnixNano(), book.Symbol, "fee_reservation_refresh", err)
			}
			e.cancelUnfundedFeeRemainder(book, client, order)
		}
	}
}

func (e *DefaultExchange) cancelUnfundedFeeRemainder(book *OrderBook, client *Client, order *Order) {
	remaining := order.Qty - order.FilledQty
	e.logExchangeForcedCancellation(book, order, remaining, exchangeForcedFeeReservationReason)
	releaseReserved(client, book.Instrument, order)
	if order.Parent != nil {
		if order.Side == Buy {
			book.Bids.CancelOrder(order.ID)
		} else {
			book.Asks.CancelOrder(order.ID)
		}
		if order.Visibility != Hidden {
			e.publishBookUpdate(book, order.Side, order.Price)
		}
		client.RemoveOrder(order.ID)
	}
	order.Status = Cancelled
	if gateway := e.Gateways[order.ClientID]; gateway != nil && gateway.IsRunning() {
		gateway.enqueueResponse(Response{Success: true, Data: &ForcedCancelNotification{OrderID: order.ID, RemainingQty: remaining}})
	}
}

// quoteFeeHeadroom returns the largest non-negative quote fee an order that
// may rest can owe. A crossing limit is taker today but its remainder can fill
// as maker later, so reserving just the taker schedule lets a maker-heavy fee
// plan debit the perp wallet below its available balance.
func quoteFeeHeadroom(feePlan FeeModel, base, quote string, qty, price, precision int64) (int64, error) {
	if feePlan == nil || qty <= 0 {
		return 0, nil
	}
	probe := Execution{Price: price, Qty: qty}
	var headroom int64
	for _, isMaker := range []bool{false, true} {
		fee, err := feePlan.CalculateFee(FillContext{
			Exec:       &probe,
			IsMaker:    isMaker,
			BaseAsset:  base,
			QuoteAsset: quote,
			Precision:  precision,
		})
		if err != nil {
			return 0, err
		}
		if fee.Asset == quote && fee.Amount > headroom {
			headroom = fee.Amount
		}
	}
	return headroom, nil
}

func reserveAsset(instrument Instrument, side Side) string {
	if side == Buy {
		return instrument.QuoteAsset()
	}
	return instrument.BaseAsset()
}

// spotOrderReservation returns the reserve-asset amount to lock for a resting
// order: the notional (buy) or base qty (sell), plus the largest non-negative
// maker/taker fee in that same asset. The bool is false when the exact sum is
// not representable, which admission must reject rather than wrap.
func spotOrderReservation(feePlan FeeModel, instrument Instrument, side Side, qty, price, precision int64) (int64, bool, error) {
	var amount int64
	var ok bool
	if side == Buy {
		amount, ok = etypes.TryMulDiv(qty, price, precision)
		if !ok {
			return 0, false, nil
		}
	} else {
		amount = qty
	}
	if qty <= 0 || feePlan == nil {
		return max(amount, 0), true, nil
	}
	probe := Execution{Price: price, Qty: qty}
	asset := reserveAsset(instrument, side)
	var feeHeadroom int64
	for _, isMaker := range []bool{false, true} {
		fee, err := feePlan.CalculateFee(FillContext{
			Exec:       &probe,
			IsMaker:    isMaker,
			BaseAsset:  instrument.BaseAsset(),
			QuoteAsset: instrument.QuoteAsset(),
			Precision:  precision,
		})
		if err != nil {
			return 0, false, err
		}
		if fee.Asset == asset && fee.Amount > feeHeadroom {
			feeHeadroom = fee.Amount
		}
	}
	amount, ok = etypes.TryAdd(amount, feeHeadroom)
	return amount, ok, nil
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
	if ok, err := e.checkForeignFeeFunds(client, book, order, precision); !ok {
		reason := RejectInsufficientBalance
		if isPriceUnavailable(err) {
			reason = RejectPriceUnavailable
			e.reportPriceUnavailable(e.Clock.NowUnixNano(), book.Symbol, "order_fee_admission", err)
		}
		resp := e.rejectOrder(order, requestID, order.ClientID, book.Symbol, reason, log)
		return &resp
	}
	// Freeze/reserve foreign fee exposure before the trade-leg path can invoke
	// auto-borrow. This is deliberately separate from checkForeignFeeFunds:
	// the former quotes all fee price sources without mutation; this records the
	// exact reservation only after that price boundary has succeeded.
	if ok, err := e.reserveForeignFeeFunds(client, book, order, precision); !ok {
		reason := RejectInsufficientBalance
		if isPriceUnavailable(err) {
			reason = RejectPriceUnavailable
			e.reportPriceUnavailable(e.Clock.NowUnixNano(), book.Symbol, "order_fee_reservation", err)
		}
		resp := e.rejectOrder(order, requestID, order.ClientID, book.Symbol, reason, log)
		return &resp
	}
	var (
		ok           bool
		admissionErr error
	)
	switch order.Type {
	case Market:
		ok, admissionErr = e.checkMarketOrderFunds(client, book, order, precision)
	case LimitOrder:
		ok, admissionErr = e.reserveLimitOrderFunds(client, book.Instrument, order, precision)
	default:
		return nil
	}
	if !ok {
		releaseForeignFeeReservation(client, book.Instrument, order)
		reason := RejectInsufficientBalance
		if isPriceUnavailable(admissionErr) {
			reason = RejectPriceUnavailable
			e.reportPriceUnavailable(e.Clock.NowUnixNano(), book.Symbol, "order_admission", admissionErr)
		}
		resp := e.rejectOrder(order, requestID, order.ClientID, book.Symbol, reason, log)
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
	// Sorted so the delta stream is deterministic: ranging the map directly
	// would randomize the publish order of a multi-level sweep run to run,
	// making fuzz failures irreproducible from their seed.
	prices := make([]int64, 0, len(levels))
	for price := range levels {
		prices = append(prices, price)
	}
	slices.Sort(prices)
	for _, price := range prices {
		e.publishBookUpdate(book, levels[price], price)
	}
}

// restOrReleaseOrder either rests the order as a GTC limit in the book or releases its funds.
// Caller must hold e.mu.Lock().
func (e *DefaultExchange) restOrReleaseOrder(client *Client, book *OrderBook, order *Order, req *OrderRequest, log Logger) {
	if order.Status != Filled && order.Status != Cancelled && req.Type == LimitOrder && req.TimeInForce == GTC {
		e.cancelOwnCrossingQuotes(client, book, order)
		var rested bool
		if order.Side == Buy {
			rested = book.Bids.AddOrder(order)
		} else {
			rested = book.Asks.AddOrder(order)
		}
		if !rested {
			// Admission preflight makes this unreachable for a valid deterministic
			// matcher. Preserve ledger consistency if a custom matcher violates
			// that contract rather than indexing an order the book rejected.
			remainingQty := order.Qty - order.FilledQty
			e.logExchangeForcedCancellation(book, order, remainingQty, exchangeForcedBookAdmissionReason)
			order.Status = Cancelled
			if gateway := e.Gateways[order.ClientID]; gateway != nil && gateway.IsRunning() {
				gateway.enqueueResponse(Response{Success: true, Data: &ForcedCancelNotification{
					OrderID: order.ID, RemainingQty: remainingQty,
				}})
			}
			releaseReserved(client, book.Instrument, order)
			putOrder(order)
			return
		}
		if order.Visibility != Hidden {
			e.publishBookUpdate(book, order.Side, order.Price)
		}
		client.AddOrder(order.ID)
	} else {
		// A market order and an IOC order never rest. Their discarded remainder
		// is terminal state, not merely a released reservation: the actor has
		// already received an acceptance for this order request and must be told
		// to remove its active-order entry. Queue it after any fills and before
		// the request success response, preserving the exchange response FIFO.
		remainingQty := order.Qty - order.FilledQty
		if remainingQty > 0 && order.Status != Filled && order.Status != Cancelled {
			order.Status = Cancelled
			if gateway := e.Gateways[order.ClientID]; gateway != nil && gateway.IsRunning() {
				gateway.enqueueResponse(Response{Success: true, Data: &ForcedCancelNotification{
					OrderID:      order.ID,
					RemainingQty: remainingQty,
				}})
			}
			// Without this the discarded remainder leaves no trace, and an
			// order that repeatedly missed the touch is indistinguishable in
			// the log from an agent that never traded.
			if log != nil {
				log.LogEvent(e.Clock.NowUnixNano(), order.ClientID, "OrderCancelled", map[string]any{
					"order_id":      order.ID,
					"request_id":    req.RequestID,
					"remaining_qty": remainingQty,
					"reason":        expiryReason(req.TimeInForce),
				})
			}
		}
		releaseReserved(client, book.Instrument, order)
		putOrder(order)
	}
}

// expiryReason names why a non-resting order discarded its remainder, so the
// log distinguishes an immediate-or-cancel that missed from a market order that
// exhausted the book.
func expiryReason(tif TimeInForce) string {
	if tif == IOC {
		return "IOC_EXPIRED"
	}
	return "NO_LIQUIDITY"
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
	// Order IDs allocate monotonically, so this cancels in placement order —
	// and deterministically, unlike the map iteration that collected targets.
	slices.SortFunc(targets, func(a, b *Order) int { return cmp.Compare(a.ID, b.ID) })
	gw := e.Gateways[client.ID]
	for _, o := range targets {
		remainingQty := o.Qty - o.FilledQty
		orderID := o.ID
		visibility := o.Visibility
		side, price := o.Side, o.Price
		e.logExchangeForcedCancellation(book, o, remainingQty, exchangeForcedSTPReason)
		releaseReserved(client, book.Instrument, o)
		opposite.CancelOrder(orderID)
		if visibility != Hidden {
			e.publishBookUpdate(book, side, price)
		}
		client.RemoveOrder(orderID)
		putOrder(o)
		if gw != nil && gw.IsRunning() {
			gw.enqueueResponse(Response{Success: true, Data: &ForcedCancelNotification{OrderID: orderID, RemainingQty: remainingQty}})
		}
	}
}

// rejectWithLog builds a failed Response and logs the exact attempted request.
func rejectWithLog(req *OrderRequest, clientID uint64, reason RejectReason, log Logger, clock Clock) Response {
	resp := Response{RequestID: req.RequestID, Success: false, Error: reason}
	if log != nil {
		log.LogEvent(clock.NowUnixNano(), clientID, "OrderRejected", rejectedOrderEvidenceFromRequest(resp, req))
	}
	return resp
}

// rejectOrder recycles the order and returns a logged rejection Response.
// Caller must hold e.mu.Lock().
func (e *DefaultExchange) rejectOrder(order *Order, requestID uint64, clientID uint64, symbol string, reason RejectReason, log Logger) Response {
	resp := Response{RequestID: requestID, Success: false, Error: reason}
	evidence := rejectedOrderEvidenceFromOrder(resp, symbol, order)
	putOrder(order)
	if log != nil {
		log.LogEvent(e.Clock.NowUnixNano(), clientID, "OrderRejected", evidence)
	}
	return resp
}

// releaseReserved returns an order's remaining reserved funds to the client
// and zeroes the ledger. Exact by construction: it releases what was locked,
// not a recomputed approximation.
func releaseReserved(client *Client, instrument Instrument, order *Order) {
	releaseForeignFeeReservation(client, instrument, order)
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
) (bool, error) {
	if amount < 0 {
		return false, nil
	}
	if reserveFn(asset, amount) {
		return true, nil
	}
	if e.BorrowingMgr == nil {
		return false, nil
	}
	cfg := e.BorrowingMgr.Config
	if isPerp && !cfg.AutoBorrowPerp {
		return false, nil
	}
	if !isPerp && !cfg.AutoBorrowSpot {
		return false, nil
	}
	client := e.Clients[clientID]
	if client == nil {
		return false, nil
	}
	var available int64
	if isPerp {
		available = client.PerpAvailable(asset)
	} else {
		available = client.GetAvailable(asset)
	}
	if available >= amount {
		return false, nil
	}
	reason := "auto_spot"
	if isPerp {
		reason = "auto_perp"
	}
	ctx := buildBorrowContext(e, client, clientID)
	ctx.CreditSpot = !isPerp
	if err := e.BorrowingMgr.BorrowMargin(ctx, asset, amount-available, reason); err != nil {
		return false, err
	}
	return reserveFn(asset, amount), nil
}
