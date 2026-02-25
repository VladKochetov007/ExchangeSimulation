package exchange

import (
	"errors"
	"sync"
	"time"
)

type Clock interface {
	NowUnixNano() int64
	NowUnix() int64
}

// Ticker interface matches the relevant parts of time.Ticker
type Ticker interface {
	C() <-chan time.Time
	Stop()
}

// TickerFactory creates tickers that work with either real-time or simulation time
type TickerFactory interface {
	NewTicker(d time.Duration) Ticker
}

type Logger interface {
	LogEvent(simTime int64, clientID uint64, eventName string, event any)
}

// ExchangeBalance tracks the exchange's own accumulated revenue and safety fund.
type ExchangeBalance struct {
	FeeRevenue    map[string]int64 `json:"fee_revenue"`
	InsuranceFund map[string]int64 `json:"insurance_fund"`
}

type Exchange struct {
	ID                      string
	Clients                 map[uint64]*Client
	Gateways                map[uint64]*ClientGateway
	Books                   map[string]*OrderBook
	Instruments             map[string]Instrument
	Positions               *PositionManager
	ExchangeBalance         *ExchangeBalance
	NextOrderID             uint64
	Matcher                 MatchingEngine
	MDPublisher             *MDPublisher
	Clock                   Clock
	Loggers                 map[string]Logger
	balanceTracker          *BalanceChangeTracker
	BorrowingMgr            *BorrowingManager
	MarginModeMgr           *MarginModeManager
	tickerFactory           TickerFactory
	mu                      sync.RWMutex
	running                 bool
	shutdownCh              chan struct{}
	snapshotInterval        time.Duration
	snapshotPollInterval    time.Duration
	snapshotStopCh          chan struct{}
	balanceSnapshotInterval time.Duration
	balanceSnapshotStopCh   chan struct{}
}

// ExchangeConfig configures exchange behavior
type ExchangeConfig struct {
	// ID identifies the exchange for logging (default: "exchange")
	ID string

	// EstimatedClients pre-allocates capacity for client maps (default: 10)
	EstimatedClients int

	// Clock provides time abstraction (default: RealClock)
	Clock Clock

	// TickerFactory creates tickers for periodic operations (default: RealTickerFactory)
	TickerFactory TickerFactory

	// SnapshotInterval is how often to publish market data snapshots (default: 100ms)
	SnapshotInterval time.Duration

	// SnapshotPollInterval is how often to check if snapshot is due (default: 1ms)
	// Lower values = more responsive to simulation time jumps but higher CPU usage
	// DEPRECATED: Use TickerFactory instead for proper simulation time support
	SnapshotPollInterval time.Duration

	// BalanceSnapshotInterval is how often to log balance snapshots (default: 0 = disabled)
	BalanceSnapshotInterval time.Duration
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
		return bestBid + (bestAsk-bestBid)/2
	}

	// Fallback to last trade price
	return ob.GetLastPrice()
}

type RealClock struct{}

func (c *RealClock) NowUnixNano() int64 { return time.Now().UnixNano() }
func (c *RealClock) NowUnix() int64     { return time.Now().Unix() }

// RealTickerFactory creates real-time tickers for production use
type RealTickerFactory struct{}

func (f *RealTickerFactory) NewTicker(d time.Duration) Ticker {
	return &realTicker{ticker: time.NewTicker(d)}
}

type realTicker struct {
	ticker *time.Ticker
}

func (t *realTicker) C() <-chan time.Time { return t.ticker.C }
func (t *realTicker) Stop()               { t.ticker.Stop() }

// NewExchange creates an exchange with default configuration
func NewExchange(estimatedClients int, clock Clock) *Exchange {
	return NewExchangeWithConfig(ExchangeConfig{
		EstimatedClients: estimatedClients,
		Clock:            clock,
	})
}

// NewExchangeWithConfig creates an exchange with custom configuration
func NewExchangeWithConfig(config ExchangeConfig) *Exchange {
	// Apply defaults
	if config.ID == "" {
		config.ID = "exchange"
	}
	if config.EstimatedClients <= 0 {
		config.EstimatedClients = 10
	}
	if config.Clock == nil {
		config.Clock = &RealClock{}
	}
	if config.TickerFactory == nil {
		config.TickerFactory = &RealTickerFactory{}
	}
	if config.SnapshotInterval == 0 {
		config.SnapshotInterval = 100 * time.Millisecond
	}
	if config.SnapshotPollInterval == 0 {
		config.SnapshotPollInterval = 1 * time.Millisecond
	}

	matcher := NewDefaultMatcher()
	matcher.clock = config.Clock
	ex := &Exchange{
		ID:          config.ID,
		Clients:     make(map[uint64]*Client, config.EstimatedClients),
		Gateways:    make(map[uint64]*ClientGateway, config.EstimatedClients),
		Books:       make(map[string]*OrderBook, 16),
		Instruments: make(map[string]Instrument, 16),
		Positions:   NewPositionManager(config.Clock),
		ExchangeBalance: &ExchangeBalance{
			FeeRevenue:    make(map[string]int64),
			InsuranceFund: make(map[string]int64),
		},
		NextOrderID:             1,
		Matcher:                 matcher,
		MDPublisher:             NewMDPublisher(),
		Clock:                   config.Clock,
		Loggers:                 make(map[string]Logger),
		tickerFactory:           config.TickerFactory,
		running:                 false,
		shutdownCh:              make(chan struct{}),
		snapshotStopCh:          make(chan struct{}),
		snapshotInterval:        config.SnapshotInterval,
		snapshotPollInterval:    config.SnapshotPollInterval,
		balanceSnapshotStopCh:   make(chan struct{}),
		balanceSnapshotInterval: config.BalanceSnapshotInterval,
	}
	ex.balanceTracker = &BalanceChangeTracker{exchange: ex}
	return ex
}

func (e *Exchange) EnablePeriodicSnapshots(interval time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.running {
		if e.snapshotInterval == 0 && interval > 0 {
			ticker := e.tickerFactory.NewTicker(interval)
			go e.runSnapshotLoop(ticker)
		}
	}
	e.snapshotInterval = interval
}

func (e *Exchange) runSnapshotLoop(ticker Ticker) {
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C():
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
		// Create snapshot
		snapshot := &BookSnapshot{
			Bids: book.Bids.getSnapshot(),
			Asks: book.Asks.getSnapshot(),
		}

		// Publish to all subscribers (WebSocket-style streaming)
		e.MDPublisher.Publish(symbol, MDSnapshot, snapshot, timestamp)

		// Also log snapshot to file if logger exists
		if log := e.Loggers[symbol]; log != nil {
			snapshotLog := map[string]any{
				"bids": snapshot.Bids,
				"asks": snapshot.Asks,
			}
			log.LogEvent(timestamp, 0, "BookSnapshot", snapshotLog)
		}
	}
}

func (e *Exchange) EnableBalanceSnapshots(snapshotInterval time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.balanceSnapshotInterval = snapshotInterval
	if e.running && snapshotInterval > 0 {
		e.balanceSnapshotStopCh = make(chan struct{})
		go e.runBalanceSnapshotLoop(snapshotInterval)
	}
}

func (e *Exchange) runBalanceSnapshotLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-e.balanceSnapshotStopCh:
			return
		case <-e.shutdownCh:
			return
		case <-ticker.C:
			e.logAllBalances()
		}
	}
}

func (e *Exchange) logAllBalances() {
	e.mu.RLock()
	defer e.mu.RUnlock()

	timestamp := e.Clock.NowUnixNano()
	log := e.getLogger("_global")
	if log == nil {
		return
	}

	for clientID, client := range e.Clients {
		spotBalances := make([]AssetBalance, 0, len(client.Balances))
		for asset, total := range client.Balances {
			reserved := client.Reserved[asset]
			spotBalances = append(spotBalances, AssetBalance{
				Asset:     asset,
				Total:     total,
				Available: total - reserved,
				Reserved:  reserved,
			})
		}

		perpBalances := make([]AssetBalance, 0, len(client.PerpBalances))
		for asset, total := range client.PerpBalances {
			reserved := client.PerpReserved[asset]
			perpBalances = append(perpBalances, AssetBalance{
				Asset:     asset,
				Total:     total,
				Available: total - reserved,
				Reserved:  reserved,
			})
		}

		borrowed := make(map[string]int64, len(client.Borrowed))
		for asset, amount := range client.Borrowed {
			borrowed[asset] = amount
		}

		snapshot := BalanceSnapshot{
			Timestamp:    timestamp,
			ClientID:     clientID,
			SpotBalances: spotBalances,
			PerpBalances: perpBalances,
			Borrowed:     borrowed,
		}
		log.LogEvent(timestamp, clientID, "balance_snapshot", snapshot)
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

func (e *Exchange) EnableBorrowing(config BorrowingConfig) error {
	if config.Enabled && config.PriceOracle == nil {
		return errors.New("price oracle required")
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.BorrowingMgr = NewBorrowingManager(e, config)
	e.MarginModeMgr = NewMarginModeManager(e)
	return nil
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

// CancelAllClientOrders atomically cancels all resting orders for clientID across all books.
// Scans books directly by order.ClientID rather than relying on client.OrderIDs, which can
// be momentarily empty if the actor is mid-cycle (cancel+resubmit in-flight).
// Releases reserved balances and publishes book updates. Safe to call concurrently.
// Returns the number of orders cancelled.
func (e *Exchange) CancelAllClientOrders(clientID uint64) int {
	e.mu.Lock()
	defer e.mu.Unlock()

	client := e.Clients[clientID]
	if client == nil {
		return 0
	}

	type cancelTarget struct {
		order *Order
		book  *OrderBook
	}
	var targets []cancelTarget
	for _, b := range e.Books {
		for _, order := range b.Bids.Orders {
			if order.ClientID == clientID {
				targets = append(targets, cancelTarget{order, b})
			}
		}
		for _, order := range b.Asks.Orders {
			if order.ClientID == clientID {
				targets = append(targets, cancelTarget{order, b})
			}
		}
	}

	count := 0
	for _, t := range targets {
		order := t.order
		book := t.book
		remainingQty := order.Qty - order.FilledQty
		releaseOrderFunds(client, book.Instrument, order.Side, remainingQty, order.Price)

		if order.Side == Buy {
			book.Bids.cancelOrder(order.ID)
			e.publishBookUpdate(book, Buy, order.Price)
		} else {
			book.Asks.cancelOrder(order.ID)
			e.publishBookUpdate(book, Sell, order.Price)
		}

		client.RemoveOrder(order.ID)
		order.Status = Cancelled
		putOrder(order)
		count++
	}
	return count
}

func (e *Exchange) ConnectClient(clientID uint64, initialBalances map[string]int64, feePlan FeeModel) *ClientGateway {
	e.mu.Lock()
	defer e.mu.Unlock()

	client := NewClient(clientID, feePlan)
	timestamp := e.Clock.NowUnixNano()
	var changes []BalanceDelta
	for asset, amount := range initialBalances {
		client.AddBalance(asset, amount)
		changes = append(changes, spotDelta(asset, 0, amount))
	}
	e.Clients[clientID] = client
	if len(changes) > 0 {
		e.balanceTracker.LogBalanceChange(timestamp, clientID, "", "initial_deposit", changes)
	}

	gateway := NewClientGateway(clientID)
	e.Gateways[clientID] = gateway

	go e.handleClientRequests(gateway)

	if !e.running {
		e.running = true
		if e.snapshotInterval > 0 {
			ticker := e.tickerFactory.NewTicker(e.snapshotInterval)
			go e.runSnapshotLoop(ticker)
		}
		if e.balanceSnapshotInterval > 0 {
			e.balanceSnapshotStopCh = make(chan struct{})
			go e.runBalanceSnapshotLoop(e.balanceSnapshotInterval)
		}
	}

	return gateway
}

// AddPerpBalance adds initial perp wallet balance for a client.
func (e *Exchange) AddPerpBalance(clientID uint64, asset string, amount int64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if client := e.Clients[clientID]; client != nil {
		oldBalance := client.PerpBalances[asset]
		client.PerpBalances[asset] += amount
		timestamp := e.Clock.NowUnixNano()
		e.balanceTracker.LogBalanceChange(timestamp, clientID, "", "initial_deposit", []BalanceDelta{
			perpDelta(asset, oldBalance, client.PerpBalances[asset]),
		})
	}
}

// Transfer moves funds between a client's spot and perp wallets.
func (e *Exchange) Transfer(clientID uint64, fromWallet, toWallet, asset string, amount int64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	client := e.Clients[clientID]
	if client == nil {
		return &TransferError{"unknown client"}
	}

	timestamp := e.Clock.NowUnixNano()
	var changes []BalanceDelta

	switch {
	case fromWallet == "spot" && toWallet == "perp":
		if client.GetAvailable(asset) < amount {
			return &TransferError{"insufficient spot balance"}
		}
		oldSpot := client.Balances[asset]
		oldPerp := client.PerpBalances[asset]
		client.Balances[asset] -= amount
		client.PerpBalances[asset] += amount
		changes = []BalanceDelta{
			spotDelta(asset, oldSpot, client.Balances[asset]),
			perpDelta(asset, oldPerp, client.PerpBalances[asset]),
		}
	case fromWallet == "perp" && toWallet == "spot":
		if client.PerpAvailable(asset) < amount {
			return &TransferError{"insufficient perp balance"}
		}
		oldPerp := client.PerpBalances[asset]
		oldSpot := client.Balances[asset]
		client.PerpBalances[asset] -= amount
		client.Balances[asset] += amount
		changes = []BalanceDelta{
			perpDelta(asset, oldPerp, client.PerpBalances[asset]),
			spotDelta(asset, oldSpot, client.Balances[asset]),
		}
	default:
		return &TransferError{"invalid wallet type"}
	}

	if log := e.getLogger("_global"); log != nil {
		log.LogEvent(timestamp, clientID, "transfer", TransferEvent{
			Timestamp:  timestamp,
			ClientID:   clientID,
			FromWallet: fromWallet,
			ToWallet:   toWallet,
			Asset:      asset,
			Amount:     amount,
		})
	}
	e.balanceTracker.LogBalanceChange(timestamp, clientID, "", "transfer", changes)

	return nil
}

type TransferError struct{ msg string }

func (e *TransferError) Error() string { return e.msg }

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
		// Discard order and subscribe requests for shut-down gateways.
		// CancelOrder/Unsubscribe/QueryBalance are still processed so they
		// can clean up state, but new orders and subscriptions must never be
		// processed after gateway.IsRunning() returns false. Blocking ReqSubscribe
		// closes the race where a queued subscribe arrives after Unsubscribe was
		// called directly on MDPublisher during bootstrap shutdown.
		if req.Type == ReqPlaceOrder || req.Type == ReqSubscribe {
			if !gateway.IsRunning() {
				continue
			}
		}

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
		if gateway.IsRunning() {
			select {
			case gateway.ResponseCh <- resp:
			default:
			}
		}
	}
}

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
	remainingQty := order.Qty - order.FilledQty
	releaseOrderFunds(client, instrument, order.Side, remainingQty, order.Price)
	if order.Side == Buy {
		book.Bids.cancelOrder(req.OrderID)
		e.publishBookUpdate(book, Buy, order.Price)
	} else {
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
	basePrecision := instrument.BasePrecision()
	positionChanged := false
	log := e.getLogger(book.Symbol)

	for _, exec := range executions {
		taker := e.Clients[exec.TakerClientID]
		maker := e.Clients[exec.MakerClientID]

		takerFee := taker.FeePlan.CalculateFee(exec, takerOrder.Side, false, instrument.BaseAsset(), instrument.QuoteAsset(), basePrecision)
		makerFee := maker.FeePlan.CalculateFee(exec, exec.MakerSide, true, instrument.BaseAsset(), instrument.QuoteAsset(), basePrecision)

		notional := (exec.Price * exec.Qty) / basePrecision

		if instrument.IsPerp() {
			e.processPerpExecution(book, exec, takerOrder, taker, maker, takerFee, makerFee, basePrecision, timestamp)
			positionChanged = true
		} else {
			e.settleSpotExecution(book, exec, takerOrder, taker, maker, takerFee, makerFee, notional, timestamp)
		}

		taker.TakerVolume += notional
		maker.MakerVolume += notional

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

		tradeID := book.SeqNum - 1
		sendFillNotification(e.Gateways[exec.TakerClientID], exec.TakerOrderID, exec.TakerClientID, tradeID, exec, takerOrder.Side, takerFee, takerOrder.FilledQty >= takerOrder.Qty)
		logFill(log, timestamp, exec.TakerClientID, exec.TakerOrderID, exec, takerOrder.Side, takerOrder.FilledQty, takerOrder.Qty, tradeID, takerFee, "taker")

		makerOrder := findOrderInBook(book, exec.MakerOrderID)
		makerFull := makerOrder != nil && makerOrder.FilledQty >= makerOrder.Qty
		sendFillNotification(e.Gateways[exec.MakerClientID], exec.MakerOrderID, exec.MakerClientID, tradeID, exec, exec.MakerSide, makerFee, makerFull)
		logFill(log, timestamp, exec.MakerClientID, exec.MakerOrderID, exec, exec.MakerSide, exec.MakerFilledQty, exec.MakerTotalQty, tradeID, makerFee, "maker")
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

// GetBestLiquidity returns best bid qty, best ask qty for a symbol, thread-safe.
func (e *Exchange) GetBestLiquidity(symbol string) (bidQty, askQty int64) {
	e.mu.RLock()
	book := e.Books[symbol]
	if book == nil {
		e.mu.RUnlock()
		return 0, 0
	}
	if book.Bids.Best != nil {
		bidQty = book.Bids.Best.TotalQty
	}
	if book.Asks.Best != nil {
		askQty = book.Asks.Best.TotalQty
	}
	e.mu.RUnlock()
	return bidQty, askQty
}

func (e *Exchange) ListInstruments(baseFilter, quoteFilter string) []Instrument {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]Instrument, 0, len(e.Instruments))
	for _, inst := range e.Instruments {
		if baseFilter != "" && inst.BaseAsset() != baseFilter {
			continue
		}
		if quoteFilter != "" && inst.QuoteAsset() != quoteFilter {
			continue
		}
		result = append(result, inst)
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
	if e.balanceSnapshotStopCh != nil {
		close(e.balanceSnapshotStopCh)
	}
	for _, gateway := range e.Gateways {
		gateway.Close()
	}
	e.running = false
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
	closedQty := abs(deltaSize)
	if closedQty > abs(oldSize) {
		closedQty = abs(oldSize)
	}
	return closedQty
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

func findOrderInBook(book *OrderBook, orderID uint64) *Order {
	if o := book.Bids.Orders[orderID]; o != nil {
		return o
	}
	return book.Asks.Orders[orderID]
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
		if makerOrder := findOrderInBook(book, exec.MakerOrderID); makerOrder != nil {
			levels[makerOrder.Price] = makerOrder.Side
		}
	}
	return levels
}

// removeMakerOrders removes fully filled maker orders from the book.
// Caller must hold e.mu.Lock().
func (e *Exchange) removeMakerOrders(book *OrderBook, executions []*Execution) {
	for _, exec := range executions {
		makerOrder := findOrderInBook(book, exec.MakerOrderID)
		if makerOrder == nil || makerOrder.Status != Filled {
			continue
		}
		if makerOrder.Side == Buy {
			book.Bids.cancelOrder(exec.MakerOrderID)
		} else {
			book.Asks.cancelOrder(exec.MakerOrderID)
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
			book.Bids.addOrder(order)
			e.publishBookUpdate(book, Buy, order.Price)
		} else {
			book.Asks.addOrder(order)
			e.publishBookUpdate(book, Sell, order.Price)
		}
		client.AddOrder(order.ID)
	} else {
		releaseOrderFunds(client, book.Instrument, order.Side, order.Qty-order.FilledQty, order.Price)
		putOrder(order)
	}
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
