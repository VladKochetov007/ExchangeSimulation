package exchange

import (
	"sync"
	"sync/atomic"
	"time"
)

type Clock interface {
	NowUnixNano() int64
	NowUnix() int64
}

type Logger interface {
	LogEvent(simTime int64, clientID uint64, eventName string, event any)
}

type Exchange struct {
	Clients     map[uint64]*Client
	Gateways    map[uint64]*ClientGateway
	Books       map[string]*OrderBook
	Instruments map[string]Instrument
	Positions   *PositionManager
	NextOrderID uint64
	Matcher     MatchingEngine
	MDPublisher *MDPublisher
	Clock       Clock
	Loggers     map[string]Logger
	mu          sync.RWMutex
	running          bool
	shutdownCh       chan struct{}
	snapshotInterval time.Duration
	snapshotStopCh   chan struct{}
}

type OrderBook struct {
	Symbol     string
	Instrument Instrument
	Bids       *Book
	Asks       *Book
	LastTrade  *Trade
	SeqNum     uint64
}

// GetLastPrice returns the price of the last trade, or 0 if no trades
func (ob *OrderBook) GetLastPrice() int64 {
	if ob.LastTrade != nil {
		return ob.LastTrade.Price
	}
	return 0
}

// GetBestBid returns the best bid price, or 0 if no bids
func (ob *OrderBook) GetBestBid() int64 {
	if ob.Bids.Best != nil {
		return ob.Bids.Best.Price
	}
	return 0
}

// GetBestAsk returns the best ask price, or 0 if no asks
func (ob *OrderBook) GetBestAsk() int64 {
	if ob.Asks.Best != nil {
		return ob.Asks.Best.Price
	}
	return 0
}

// GetMidPrice returns the mid price between best bid and ask
// Falls back to last price if order book is empty
func (ob *OrderBook) GetMidPrice() int64 {
	bestBid := ob.GetBestBid()
	bestAsk := ob.GetBestAsk()

	if bestBid > 0 && bestAsk > 0 {
		return (bestBid + bestAsk) / 2
	}

	// Fallback to last trade price
	return ob.GetLastPrice()
}

type RealClock struct{}

func (c *RealClock) NowUnixNano() int64 { return time.Now().UnixNano() }
func (c *RealClock) NowUnix() int64     { return time.Now().Unix() }

func NewExchange(estimatedClients int, clock Clock) *Exchange {
	if clock == nil {
		clock = &RealClock{}
	}
	matcher := NewDefaultMatcher()
	matcher.clock = clock
	return &Exchange{
		Clients:     make(map[uint64]*Client, estimatedClients),
		Gateways:    make(map[uint64]*ClientGateway, estimatedClients),
		Books:       make(map[string]*OrderBook, 16),
		Instruments: make(map[string]Instrument, 16),
		Positions:   NewPositionManager(clock),
		NextOrderID: 1,
		Matcher:     matcher,
		MDPublisher: NewMDPublisher(),
		Clock:       clock,
		Loggers:     make(map[string]Logger),
		running:          false,
		shutdownCh:       make(chan struct{}),
		snapshotStopCh:   make(chan struct{}),
		snapshotInterval: 0,
	}
}

func (e *Exchange) EnablePeriodicSnapshots(interval time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.running {
		// If already running, start the loop now
		if e.snapshotInterval == 0 && interval > 0 {
			go e.runSnapshotLoop(interval)
		}
	}
	e.snapshotInterval = interval
}

func (e *Exchange) runSnapshotLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.logSnapshots()
		case <-e.snapshotStopCh:
			return
		case <-e.shutdownCh:
			return
		}
	}
}

func (e *Exchange) logSnapshots() {
	e.mu.RLock()
	defer e.mu.RUnlock()

	timestamp := e.Clock.NowUnixNano()
	for symbol, book := range e.Books {
		// Log snapshot to file if logger exists
		if log := e.Loggers[symbol]; log != nil {
			// Create snapshot
			snapshot := &BookSnapshot{
				Bids: book.Bids.getSnapshot(),
				Asks: book.Asks.getSnapshot(),
			}

			snapshotLog := map[string]any{
				"bids": snapshot.Bids,
				"asks": snapshot.Asks,
			}
			log.LogEvent(timestamp, 0, "BookSnapshot", snapshotLog)
		}
	}
}

func (e *Exchange) SetLogger(symbol string, log Logger) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Loggers[symbol] = log
}

func (e *Exchange) getLogger(symbol string) Logger {
	return e.Loggers[symbol]
}

func (e *Exchange) AddInstrument(instrument Instrument) {
	e.mu.Lock()
	defer e.mu.Unlock()

	symbol := instrument.Symbol()
	e.Instruments[symbol] = instrument
	e.Books[symbol] = &OrderBook{
		Symbol:     symbol,
		Instrument: instrument,
		Bids:       newBook(Buy),
		Asks:       newBook(Sell),
		LastTrade:  nil,
		SeqNum:     0,
	}
}

func (e *Exchange) ConnectClient(clientID uint64, initialBalances map[string]int64, feePlan FeeModel) *ClientGateway {
	e.mu.Lock()
	defer e.mu.Unlock()

	client := NewClient(clientID, feePlan)
	for asset, amount := range initialBalances {
		client.AddBalance(asset, amount)
	}
	e.Clients[clientID] = client

	gateway := NewClientGateway(clientID)
	gateway.Running = true
	e.Gateways[clientID] = gateway

	go e.handleClientRequests(gateway)

	if !e.running {
		e.running = true
		if e.snapshotInterval > 0 {
			go e.runSnapshotLoop(e.snapshotInterval)
		}
	}

	return gateway
}

func (e *Exchange) DisconnectClient(clientID uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if gateway := e.Gateways[clientID]; gateway != nil {
		gateway.Close()
		delete(e.Gateways, clientID)
	}
}

func (e *Exchange) handleClientRequests(gateway *ClientGateway) {
	for req := range gateway.RequestCh {
		var resp Response
		switch req.Type {
		case ReqPlaceOrder:
			resp = e.placeOrder(gateway.ClientID, req.OrderReq)
		case ReqCancelOrder:
			resp = e.cancelOrder(gateway.ClientID, req.CancelReq)
		case ReqQueryBalance:
			resp = e.queryBalance(gateway.ClientID, req.QueryReq)
		case ReqSubscribe:
			resp = e.subscribe(gateway.ClientID, req.QueryReq, gateway)
		case ReqUnsubscribe:
			resp = e.unsubscribe(gateway.ClientID, req.QueryReq)
		}
		
		// Send response only if gateway is still running
		gateway.Mu.Lock()
		if gateway.Running {
			select {
			case gateway.ResponseCh <- resp:
			default:
			}
		}
		gateway.Mu.Unlock()
	}
}

func (e *Exchange) placeOrder(clientID uint64, req *OrderRequest) Response {
	e.mu.Lock()
	defer e.mu.Unlock()

	client := e.Clients[clientID]
	if client == nil {
		resp := Response{RequestID: req.RequestID, Success: false, Error: RejectUnknownClient}
		if log := e.getLogger(req.Symbol); log != nil {
			log.LogEvent(e.Clock.NowUnixNano(), clientID, "OrderRejected", resp)
		}
		return resp
	}

	book := e.Books[req.Symbol]
	if book == nil {
		resp := Response{RequestID: req.RequestID, Success: false, Error: RejectUnknownInstrument}
		if log := e.getLogger(req.Symbol); log != nil {
			log.LogEvent(e.Clock.NowUnixNano(), clientID, "OrderRejected", resp)
		}
		return resp
	}

	instrument := book.Instrument
	precision := instrument.BasePrecision()
	log := e.getLogger(req.Symbol)

	if req.Type == LimitOrder && !instrument.ValidatePrice(req.Price) {
		resp := Response{RequestID: req.RequestID, Success: false, Error: RejectInvalidPrice}
		if log != nil {
			log.LogEvent(e.Clock.NowUnixNano(), clientID, "OrderRejected", resp)
		}
		return resp
	}
	if !instrument.ValidateQty(req.Qty) {
		resp := Response{RequestID: req.RequestID, Success: false, Error: RejectInvalidQty}
		if log != nil {
			log.LogEvent(e.Clock.NowUnixNano(), clientID, "OrderRejected", resp)
		}
		return resp
	}

	orderID := atomic.AddUint64(&e.NextOrderID, 1)
	order := getOrder()
	order.ID = orderID
	order.ClientID = clientID
	order.Side = req.Side
	order.Type = req.Type
	order.TimeInForce = req.TimeInForce
	order.Price = req.Price
	order.Qty = req.Qty
	order.Visibility = req.Visibility
	order.IcebergQty = req.IcebergQty
	order.Status = Open
	order.Timestamp = e.Clock.NowUnixNano()

	switch req.Type {
	case Market:
		if req.Side == Buy {
			if book.Asks.Best != nil {
				maxCost := (req.Qty * book.Asks.Best.Price) / precision
				if client.GetAvailable(instrument.QuoteAsset()) < maxCost {
					putOrder(order)
					resp := Response{RequestID: req.RequestID, Success: false, Error: RejectInsufficientBalance}
					if log != nil {
						log.LogEvent(e.Clock.NowUnixNano(), clientID, "OrderRejected", resp)
					}
					return resp
				}
			}
		} else {
			// Spot requires base asset, perp does not (can short)
			if !instrument.IsPerp() {
				if client.GetAvailable(instrument.BaseAsset()) < req.Qty {
					putOrder(order)
					resp := Response{RequestID: req.RequestID, Success: false, Error: RejectInsufficientBalance}
					if log != nil {
						log.LogEvent(e.Clock.NowUnixNano(), clientID, "OrderRejected", resp)
					}
					return resp
				}
			}
		}
	case LimitOrder:
		asset := instrument.QuoteAsset()
		amount := (req.Qty * req.Price) / precision
		if req.Side == Buy {
			if !client.Reserve(asset, amount) {
				putOrder(order)
				resp := Response{RequestID: req.RequestID, Success: false, Error: RejectInsufficientBalance}
				if log != nil {
					log.LogEvent(e.Clock.NowUnixNano(), clientID, "OrderRejected", resp)
				}
				return resp
			}
		} else {
			// Spot requires base asset reservation, perp does not
			if !instrument.IsPerp() {
				asset = instrument.BaseAsset()
				if !client.Reserve(asset, req.Qty) {
					putOrder(order)
					resp := Response{RequestID: req.RequestID, Success: false, Error: RejectInsufficientBalance}
					if log != nil {
						log.LogEvent(e.Clock.NowUnixNano(), clientID, "OrderRejected", resp)
					}
					return resp
				}
			}
		}
	}

	if log != nil {
		log.LogEvent(e.Clock.NowUnixNano(), clientID, "OrderAccepted", order)
	}

	result := e.Matcher.Match(book.Bids, book.Asks, order)

	if req.TimeInForce == FOK && !result.FullyFilled {
		if req.Type == LimitOrder {
			if req.Side == Buy {
				client.Release(instrument.QuoteAsset(), (req.Qty*req.Price)/precision)
			} else {
				if !instrument.IsPerp() {
					client.Release(instrument.BaseAsset(), req.Qty)
				}
			}
		}
		putOrder(order)
		resp := Response{RequestID: req.RequestID, Success: false, Error: RejectFOKNotFilled}
		if log != nil {
			log.LogEvent(e.Clock.NowUnixNano(), clientID, "OrderRejected", resp)
		}
		return resp
	}

	affectedLevels := make(map[int64]Side)
	for _, exec := range result.Executions {
		makerOrder := book.Bids.Orders[exec.MakerOrderID]
		if makerOrder == nil {
			makerOrder = book.Asks.Orders[exec.MakerOrderID]
		}
		if makerOrder != nil {
			affectedLevels[makerOrder.Price] = makerOrder.Side
		}
	}

	e.processExecutions(book, result.Executions, order)

	for _, exec := range result.Executions {
		makerOrder := book.Bids.Orders[exec.MakerOrderID]
		if makerOrder == nil {
			makerOrder = book.Asks.Orders[exec.MakerOrderID]
		}
		if makerOrder != nil && makerOrder.Status == Filled {
			if makerOrder.Side == Buy {
				book.Bids.cancelOrder(exec.MakerOrderID)
			} else {
				book.Asks.cancelOrder(exec.MakerOrderID)
			}
			e.Clients[exec.MakerClientID].RemoveOrder(exec.MakerOrderID)
			putOrder(makerOrder)
		}
	}

	for price, side := range affectedLevels {
		e.publishBookUpdate(book, side, price)
	}

	if order.Status != Filled && req.Type == LimitOrder && req.TimeInForce == GTC {
		if order.Side == Buy {
			book.Bids.addOrder(order)
			e.publishBookUpdate(book, Buy, order.Price)
		} else {
			book.Asks.addOrder(order)
			e.publishBookUpdate(book, Sell, order.Price)
		}
		client.AddOrder(orderID)
	} else {
		if order.FilledQty < order.Qty {
			if order.Side == Buy {
				remainingNotional := ((order.Qty - order.FilledQty) * order.Price) / precision
				client.Release(instrument.QuoteAsset(), remainingNotional)
			} else {
				if !instrument.IsPerp() {
					client.Release(instrument.BaseAsset(), order.Qty-order.FilledQty)
				}
			}
		}
		putOrder(order)
	}

	return Response{RequestID: req.RequestID, Success: true, Data: orderID}
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
		if o := b.Bids.Orders[req.OrderID]; o != nil {
			order = o
			book = b
			break
		}
		if o := b.Asks.Orders[req.OrderID]; o != nil {
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
	precision := instrument.BasePrecision()
	remainingQty := order.Qty - order.FilledQty
	if order.Side == Buy {
		amount := (remainingQty * order.Price) / precision
		client.Release(instrument.QuoteAsset(), amount)
		book.Bids.cancelOrder(req.OrderID)
		e.publishBookUpdate(book, Buy, order.Price)
	} else {
		if !instrument.IsPerp() {
			client.Release(instrument.BaseAsset(), remainingQty)
		}
		book.Asks.cancelOrder(req.OrderID)
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

	types := []MDType{MDSnapshot, MDDelta, MDTrade}
	e.MDPublisher.Subscribe(clientID, req.Symbol, types, gateway)

	snapshot := &BookSnapshot{
		Bids: book.Bids.getSnapshot(),
		Asks: book.Asks.getSnapshot(),
	}
	e.MDPublisher.Publish(req.Symbol, MDSnapshot, snapshot, e.Clock.NowUnixNano())

	// Log snapshot to file
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

func (e *Exchange) processExecutions(book *OrderBook, executions []*Execution, takerOrder *Order) {
	instrument := book.Instrument
	timestamp := e.Clock.NowUnixNano()
	precision := instrument.TickSize()
	positionChanged := false
	log := e.getLogger(book.Symbol)

	for _, exec := range executions {
		taker := e.Clients[exec.TakerClientID]
		maker := e.Clients[exec.MakerClientID]

		takerFee := taker.FeePlan.CalculateFee(exec, takerOrder.Side, false, instrument.BaseAsset(), instrument.QuoteAsset(), precision)
		makerSide := Sell
		if takerOrder.Side == Sell {
			makerSide = Buy
		}
		makerFee := maker.FeePlan.CalculateFee(exec, makerSide, true, instrument.BaseAsset(), instrument.QuoteAsset(), precision)

		notional := (exec.Price * exec.Qty) / precision

		if takerOrder.Side == Buy {
			taker.Release(instrument.QuoteAsset(), notional)
			taker.Balances[instrument.QuoteAsset()] -= notional + takerFee.Amount
			taker.Balances[instrument.BaseAsset()] += exec.Qty
			maker.Release(instrument.BaseAsset(), exec.Qty)
			maker.Balances[instrument.QuoteAsset()] += notional - makerFee.Amount
			maker.Balances[instrument.BaseAsset()] -= exec.Qty
		} else {
			taker.Release(instrument.BaseAsset(), exec.Qty)
			taker.Balances[instrument.BaseAsset()] -= exec.Qty
			taker.Balances[instrument.QuoteAsset()] += notional - takerFee.Amount
			maker.Release(instrument.QuoteAsset(), notional)
			maker.Balances[instrument.BaseAsset()] += exec.Qty
			maker.Balances[instrument.QuoteAsset()] -= notional + makerFee.Amount
		}

		taker.TakerVolume += notional
		maker.MakerVolume += notional

		if instrument.IsPerp() {
			e.Positions.UpdatePosition(exec.TakerClientID, book.Symbol, exec.Qty, exec.Price, takerOrder.Side)
			e.Positions.UpdatePosition(exec.MakerClientID, book.Symbol, exec.Qty, exec.Price, makerSide)
			positionChanged = true
		}

		trade := &Trade{
			TradeID:      book.SeqNum,
			Price:        exec.Price,
			Qty:          exec.Qty,
			Side:         takerOrder.Side,
			TakerOrderID: exec.TakerOrderID,
			MakerOrderID: exec.MakerOrderID,
		}
		book.SeqNum++
		book.LastTrade = trade

		if log != nil {
			log.LogEvent(timestamp, 0, "Trade", trade)
		}

		e.MDPublisher.PublishTrade(book.Symbol, trade, timestamp)

		takerGw := e.Gateways[exec.TakerClientID]
		if takerGw != nil {
			takerFillIsFull := takerOrder.FilledQty >= takerOrder.Qty
			takerGw.ResponseCh <- Response{
				Success: true,
				Data: &FillNotification{
					OrderID:   exec.TakerOrderID,
					ClientID:  exec.TakerClientID,
					TradeID:   book.SeqNum - 1,
					Qty:       exec.Qty,
					Price:     exec.Price,
					Side:      takerOrder.Side,
					IsFull:    takerFillIsFull,
					FeeAmount: takerFee.Amount,
					FeeAsset:  takerFee.Asset,
				},
			}
		}

		if log != nil {
			takerFill := map[string]any{
				"order_id":      exec.TakerOrderID,
				"qty":           exec.Qty,
				"price":         exec.Price,
				"side":          takerOrder.Side.String(),
				"filled_qty":    takerOrder.FilledQty,
				"remaining_qty": takerOrder.Qty - takerOrder.FilledQty,
				"is_full":       takerOrder.FilledQty >= takerOrder.Qty,
				"trade_id":      book.SeqNum - 1,
				"role":          "taker",
				"fee_amount":    takerFee.Amount,
				"fee_asset":     takerFee.Asset,
			}
			log.LogEvent(timestamp, exec.TakerClientID, "OrderFill", takerFill)
		}

		makerGw := e.Gateways[exec.MakerClientID]
		if makerGw != nil {
			makerOrder := book.Bids.Orders[exec.MakerOrderID]
			if makerOrder == nil {
				makerOrder = book.Asks.Orders[exec.MakerOrderID]
			}
			makerFillIsFull := makerOrder != nil && makerOrder.FilledQty >= makerOrder.Qty
			makerGw.ResponseCh <- Response{
				Success: true,
				Data: &FillNotification{
					OrderID:   exec.MakerOrderID,
					ClientID:  exec.MakerClientID,
					TradeID:   book.SeqNum - 1,
					Qty:       exec.Qty,
					Price:     exec.Price,
					Side:      makerSide,
					IsFull:    makerFillIsFull,
					FeeAmount: makerFee.Amount,
					FeeAsset:  makerFee.Asset,
				},
			}
		}

		if log != nil {
			makerOrder := book.Bids.Orders[exec.MakerOrderID]
			if makerOrder == nil {
				makerOrder = book.Asks.Orders[exec.MakerOrderID]
			}
			if makerOrder != nil {
				makerFill := map[string]any{
					"order_id":      exec.MakerOrderID,
					"qty":           exec.Qty,
					"price":         exec.Price,
					"side":          makerSide.String(),
					"filled_qty":    makerOrder.FilledQty,
					"remaining_qty": makerOrder.Qty - makerOrder.FilledQty,
					"is_full":       makerOrder.FilledQty >= makerOrder.Qty,
					"trade_id":      book.SeqNum - 1,
					"role":          "maker",
					"fee_amount":    makerFee.Amount,
					"fee_asset":     makerFee.Asset,
				}
				log.LogEvent(timestamp, exec.MakerClientID, "OrderFill", makerFill)
			}
		}
	}

	// Publish open interest if positions changed
	if positionChanged {
		totalOI := e.Positions.CalculateOpenInterest(book.Symbol)
		oi := &OpenInterest{
			Symbol:         book.Symbol,
			TotalContracts: totalOI,
			Timestamp:      timestamp,
		}
		e.MDPublisher.PublishOpenInterest(book.Symbol, oi, timestamp)
	}
}

// publishBookUpdate publishes a delta update for a specific price level
// Caller must hold e.mu lock
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
	// If limit is nil, qty is 0, which means delete level

	delta := &BookDelta{
		Side:       side,
		Price:      price,
		VisibleQty: visible,
		HiddenQty:  hidden,
	}
	e.MDPublisher.Publish(book.Symbol, MDDelta, delta, e.Clock.NowUnixNano())

	// Log delta to file
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

type InstrumentInfo struct {
	Symbol     string
	BaseAsset  string
	QuoteAsset string
	TickSize   int64
	MinSize    int64
	IsPerp     bool
}

func (e *Exchange) ListInstruments(baseFilter, quoteFilter string) []InstrumentInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]InstrumentInfo, 0, len(e.Instruments))
	for _, inst := range e.Instruments {
		if baseFilter != "" && inst.BaseAsset() != baseFilter {
			continue
		}
		if quoteFilter != "" && inst.QuoteAsset() != quoteFilter {
			continue
		}
		result = append(result, InstrumentInfo{
			Symbol:     inst.Symbol(),
			BaseAsset:  inst.BaseAsset(),
			QuoteAsset: inst.QuoteAsset(),
			TickSize:   inst.TickSize(),
			MinSize:    inst.MinOrderSize(),
			IsPerp:     inst.IsPerp(),
		})
	}
	return result
}

// publishSnapshot publishes a full order book snapshot to all subscribers
// Caller must hold e.mu lock
func (e *Exchange) publishSnapshot(symbol string, timestamp int64) {
	book := e.Books[symbol]
	if book == nil {
		return
	}
	snapshot := &BookSnapshot{
		Bids: book.Bids.getSnapshot(),
		Asks: book.Asks.getSnapshot(),
	}
	e.MDPublisher.Publish(symbol, MDSnapshot, snapshot, timestamp)
}

func (e *Exchange) Shutdown() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.running {
		return
	}

	close(e.shutdownCh)
	close(e.snapshotStopCh)
	for _, gateway := range e.Gateways {
		gateway.Close()
	}
	e.running = false
}
