package exchange

import (
	"errors"
	"maps"
	"sync"
	"time"

	ematching "exchange_sim/matching"
)

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
	BorrowingMgr            *BorrowingManager
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

// NewExchange creates an exchange with default configuration
func NewExchange(estimatedClients int, clock Clock) *Exchange {
	return NewExchangeWithConfig(ExchangeConfig{
		EstimatedClients: estimatedClients,
		Clock:            clock,
	})
}

// NewExchangeWithConfig creates an exchange with custom configuration
func NewExchangeWithConfig(config ExchangeConfig) *Exchange {
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

	matcher := ematching.NewDefaultMatcher(config.Clock)
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
		snapshot := &BookSnapshot{
			Bids: book.Bids.GetSnapshot(),
			Asks: book.Asks.GetSnapshot(),
		}
		e.MDPublisher.Publish(symbol, MDSnapshot, snapshot, timestamp)

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
		maps.Copy(borrowed, client.Borrowed)

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
			book.Bids.CancelOrder(order.ID)
			e.publishBookUpdate(book, Buy, order.Price)
		} else {
			book.Asks.CancelOrder(order.ID)
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
		logBalanceChange(e, timestamp, clientID, "", "initial_deposit", changes)
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
		logBalanceChange(e, timestamp, clientID, "", "initial_deposit", []BalanceDelta{
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
	logBalanceChange(e, timestamp, clientID, "", "transfer", changes)

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

		if gateway.IsRunning() {
			select {
			case gateway.ResponseCh <- resp:
			default:
			}
		}
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

// GetBook returns the OrderBook for symbol, acquiring a read lock.
// Implements price.BookProvider for MidPriceOracle.
func (e *Exchange) GetBook(symbol string) *OrderBook {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.Books[symbol]
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

// publishSnapshot publishes a full order book snapshot to all subscribers.
// Caller must hold e.mu lock.
func (e *Exchange) publishSnapshot(symbol string, timestamp int64) {
	book := e.Books[symbol]
	if book == nil {
		return
	}
	snapshot := &BookSnapshot{
		Bids: book.Bids.GetSnapshot(),
		Asks: book.Asks.GetSnapshot(),
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

// Lock acquires the exchange write lock. Required for tests that directly mutate exchange state.
func (e *Exchange) Lock() { e.mu.Lock() }

// Unlock releases the exchange write lock.
func (e *Exchange) Unlock() { e.mu.Unlock() }

// RLock acquires the exchange read lock.
func (e *Exchange) RLock() { e.mu.RLock() }

// RUnlock releases the exchange read lock.
func (e *Exchange) RUnlock() { e.mu.RUnlock() }

// PlaceOrder is the public test-accessible wrapper for placeOrder.
func (e *Exchange) PlaceOrder(clientID uint64, req *OrderRequest) Response {
	return e.placeOrder(clientID, req)
}

// CancelOrder is the public test-accessible wrapper for cancelOrder.
func (e *Exchange) CancelOrder(clientID uint64, req *CancelRequest) Response {
	return e.cancelOrder(clientID, req)
}

// QueryBalance is the public test-accessible wrapper for queryBalance.
func (e *Exchange) QueryBalance(clientID uint64, req *QueryRequest) Response {
	return e.queryBalance(clientID, req)
}

// Subscribe is the public test-accessible wrapper for subscribe.
func (e *Exchange) Subscribe(clientID uint64, req *QueryRequest, gateway *ClientGateway) Response {
	return e.subscribe(clientID, req, gateway)
}

// Unsubscribe is the public test-accessible wrapper for unsubscribe.
func (e *Exchange) Unsubscribe(clientID uint64, req *QueryRequest) Response {
	return e.unsubscribe(clientID, req)
}

// HandleClientRequests is the public test-accessible wrapper for handleClientRequests.
func (e *Exchange) HandleClientRequests(gateway *ClientGateway) {
	e.handleClientRequests(gateway)
}

// PublishSnapshot is the public test-accessible wrapper for publishSnapshot.
func (e *Exchange) PublishSnapshot(symbol string, timestamp int64) {
	e.publishSnapshot(symbol, timestamp)
}

// LogAllBalances is the public test-accessible wrapper for logAllBalances.
func (e *Exchange) LogAllBalances() {
	e.logAllBalances()
}

// IsRunning returns whether the exchange is currently running.
func (e *Exchange) IsRunning() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.running
}

// SetRunning directly sets the running state. Used by tests to simulate state.
func (e *Exchange) SetRunning(v bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.running = v
}
