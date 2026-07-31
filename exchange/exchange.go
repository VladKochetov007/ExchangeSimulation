package exchange

import (
	"cmp"
	"context"
	"errors"
	"maps"
	"slices"
	"sync"
	"time"

	ematching "exchange_sim/matching"
	etypes "exchange_sim/types"
)

// ExchangeBalance tracks the exchange's own accumulated revenue and safety fund.
type ExchangeBalance struct {
	FeeRevenue    map[string]int64 `json:"fee_revenue"`
	InsuranceFund map[string]int64 `json:"insurance_fund"`
}

// LiquidationHandler is called when a liquidation event occurs.
type LiquidationHandler interface {
	OnMarginCall(event *MarginCallEvent)
	OnLiquidation(event *LiquidationEvent)
	OnInsuranceFund(event *InsuranceFundEvent)
}

// AutomationConfig configures automatic exchange operations.
type AutomationConfig struct {
	// MarkPriceCalc calculates mark price from order book (default: MidPriceCalculator)
	MarkPriceCalc MarkPriceCalculator

	// MarkPriceCalcs overrides MarkPriceCalc per symbol. Stateful calculators
	// (EMA, TWAP, Median) carry one symbol's state and must not be shared
	// across perp books.
	MarkPriceCalcs map[string]MarkPriceCalculator

	// IndexProvider provides index prices for perpetuals (required for price updates)
	IndexProvider PriceSource

	// PriceUpdateInterval is how often to update funding rates (default: 3s)
	PriceUpdateInterval time.Duration

	// CollateralRate is annual interest rate on borrowed amounts in bps (default: 500 = 5%)
	CollateralRate int64

	// LiquidationFeeBps is the clearance fee charged on a liquidation's closed
	// notional, credited to the insurance fund (venue pattern; e.g. 30 = 0.3%).
	// Zero disables the fee and preserves pre-fee economics.
	LiquidationFeeBps int64

	// MarkPriceEMAWindow and MarkPriceBandBps parameterize the DEFAULT
	// index-anchored mark calculator (ClampedEMA of the basis) that margined
	// books get when an index exists and no explicit calculator was injected.
	// The window is in SAMPLES of PriceUpdateInterval, not seconds: the
	// default 10 at the 3s tick gives a ~30s basis average, matching venue
	// practice (Binance 30s MA, Bybit 2.5min). The band default 600 bps
	// (±3%) matches the only published venue cap; tighter values clip
	// genuine contango and pin the mark at the band edge.
	MarkPriceEMAWindow int
	MarkPriceBandBps   int64

	// LiquidationHandler receives liquidation events (optional)
	LiquidationHandler LiquidationHandler

	// ListingPolicies generate scheduled instrument listings (dated futures
	// tenors, option chains). Polled every second by the expiry loop, which
	// also observes settlement prices and settles expired instruments.
	ListingPolicies []etypes.ListingPolicy
}

type DefaultExchange struct {
	ID                      string
	Clients                 map[uint64]*Client
	Gateways                map[uint64]*ClientGateway
	Books                   map[string]*OrderBook
	Instruments             map[string]Instrument
	Positions               PositionStore
	ExchangeBalance         *ExchangeBalance
	NextOrderID             uint64
	Matcher                 MatchingEngine
	MDPublisher             *MDPublisher
	Clock                   Clock
	Loggers                 map[string]Logger
	BorrowingMgr            *BorrowingManager
	CollateralRate          int64
	LiquidationFeeBps       int64
	autoAnchorMarks         bool
	markEMAWindow           int
	markBandBps             int64
	autoAnchoredSymbols     map[string]bool
	LiquidationHandler      LiquidationHandler
	tickerFactory           TickerFactory
	markPriceCalc           MarkPriceCalculator
	markPriceCalcs          map[string]MarkPriceCalculator
	indexProvider           PriceSource
	priceUpdateInterval     time.Duration
	listingPolicies         []etypes.ListingPolicy
	automCtx                context.Context
	automCancel             context.CancelFunc
	automWg                 sync.WaitGroup
	automMu                 sync.RWMutex
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
func NewExchange(estimatedClients int, clock Clock) *DefaultExchange {
	return NewExchangeWithConfig(ExchangeConfig{
		EstimatedClients: estimatedClients,
		Clock:            clock,
	})
}

// NewExchangeWithConfig creates an exchange with custom configuration
func NewExchangeWithConfig(config ExchangeConfig) *DefaultExchange {
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

	matcher := ematching.NewPriceTimeMatcher(config.Clock)
	ex := &DefaultExchange{
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

func (e *DefaultExchange) EnablePeriodicSnapshots(interval time.Duration) {
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

func (e *DefaultExchange) runSnapshotLoop(ticker Ticker) {
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

func (e *DefaultExchange) logSnapshots() {
	e.mu.RLock()
	defer e.mu.RUnlock()

	timestamp := e.Clock.NowUnixNano()
	for symbol, book := range e.Books {
		// Subscribers get the displayed book; loggers keep the god view.
		e.MDPublisher.Publish(symbol, MDSnapshot, &BookSnapshot{
			Bids: book.Bids.GetPublicSnapshot(),
			Asks: book.Asks.GetPublicSnapshot(),
		}, timestamp)

		if log := e.Loggers[symbol]; log != nil {
			log.LogEvent(timestamp, 0, "BookSnapshot", map[string]any{
				"bids": book.Bids.GetSnapshot(),
				"asks": book.Asks.GetSnapshot(),
			})
		}
	}
}

func (e *DefaultExchange) EnableBalanceSnapshots(interval time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.balanceSnapshotInterval = interval
	if e.running && interval > 0 {
		e.balanceSnapshotStopCh = make(chan struct{})
		go e.runBalanceSnapshotLoop(interval)
	}
}

func (e *DefaultExchange) runBalanceSnapshotLoop(interval time.Duration) {
	ticker := e.tickerFactory.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-e.balanceSnapshotStopCh:
			return
		case <-e.shutdownCh:
			return
		case <-ticker.C():
			e.LogAllBalances()
		}
	}
}

func (e *DefaultExchange) LogAllBalances() {
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
			locked := client.Reserved[asset]
			borrowed := client.Borrowed[asset]
			spotBalances = append(spotBalances, AssetBalance{
				Asset:    asset,
				Free:     total - locked,
				Locked:   locked,
				Borrowed: borrowed,
				NetAsset: total - borrowed,
			})
		}

		perpBalances := make([]AssetBalance, 0, len(client.PerpBalances))
		for asset, total := range client.PerpBalances {
			locked := client.PerpReserved[asset]
			perpBalances = append(perpBalances, AssetBalance{
				Asset:    asset,
				Free:     total - locked,
				Locked:   locked,
				NetAsset: total,
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

func (e *DefaultExchange) SetLogger(symbol string, log Logger) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Loggers[symbol] = log
}

func (e *DefaultExchange) getLogger(symbol string) Logger {
	return e.Loggers[symbol]
}

func (e *DefaultExchange) EnableBorrowing(config BorrowingConfig) error {
	if config.Enabled && config.PriceSource == nil {
		return errors.New("price source required")
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.BorrowingMgr = NewBorrowingManager(config)
	return nil
}

func (e *DefaultExchange) AddInstrument(instrument Instrument) {
	e.mu.Lock()
	defer e.mu.Unlock()

	symbol := instrument.Symbol()
	// Re-registering an existing symbol must not swap in a fresh empty book:
	// that strands every resting order — and the balance it has reserved — in
	// the old book, where no cancel or match can ever reach it again. Keep the
	// live book; listing a symbol is a one-time operation.
	if _, exists := e.Books[symbol]; exists {
		return
	}
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
func (e *DefaultExchange) CancelAllClientOrders(clientID uint64) int {
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

	// Placement order via monotonic IDs: map iteration over books and orders
	// would randomize the cancel/notification sequence run to run.
	slices.SortFunc(targets, func(a, b cancelTarget) int { return cmp.Compare(a.order.ID, b.order.ID) })

	gw := e.Gateways[clientID]
	count := 0
	for _, t := range targets {
		order := t.order
		book := t.book
		remainingQty := order.Qty - order.FilledQty
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
		orderID := order.ID
		order.Status = Cancelled
		putOrder(order)
		count++

		// Same contract as liquidation cancels: the actor must learn its
		// order is gone or its pending state blocks forever.
		gw.enqueueResponse(Response{Success: true, Data: &ForcedCancelNotification{OrderID: orderID, RemainingQty: remainingQty}})
	}
	return count
}

func (e *DefaultExchange) ConnectNewClient(clientID uint64, initialBalances map[string]int64, feePlan FeeModel) Gateway {
	e.mu.Lock()
	defer e.mu.Unlock()

	timestamp := e.Clock.NowUnixNano()
	// Reconnect on a known ID reuses the existing account: overwriting it
	// would zero the ledgers while the client's resting orders keep settling
	// against them (silent conservation break). Deposits apply only on the
	// first connect — a reconnect is a new session, not a new account.
	client := e.Clients[clientID]
	if client == nil {
		client = NewClient(clientID, feePlan)
		var changes []BalanceDelta
		for asset, amount := range initialBalances {
			client.AddBalance(asset, amount)
			changes = append(changes, spotDelta(asset, 0, amount))
		}
		e.Clients[clientID] = client
		if len(changes) > 0 {
			logBalanceChange(e, timestamp, clientID, "", "initial_deposit", changes)
		}
	} else if old := e.Gateways[clientID]; old != nil {
		old.Close()
	}

	gateway := NewClientGateway(clientID)
	e.Gateways[clientID] = gateway

	go e.HandleClientRequests(gateway)

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
func (e *DefaultExchange) AddPerpBalance(clientID uint64, asset string, amount int64) {
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
func (e *DefaultExchange) Transfer(clientID uint64, fromWallet, toWallet, asset string, amount int64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// A negative amount would reverse the transfer direction while the
	// availability check still guards the declared source wallet — letting
	// reserved margin be siphoned out of the undeclared one unchecked.
	if amount <= 0 {
		return &TransferError{"transfer amount must be positive"}
	}

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

func (e *DefaultExchange) DisconnectClient(clientID uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if gateway := e.Gateways[clientID]; gateway != nil {
		gateway.Close()
		delete(e.Gateways, clientID)
	}
}

func (e *DefaultExchange) HandleClientRequests(gateway *ClientGateway) {
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
			resp = e.PlaceOrder(gateway.ClientID, req.OrderReq)
		case ReqCancelOrder:
			resp = e.CancelOrder(gateway.ClientID, req.CancelReq)
		case ReqQueryBalance:
			resp = e.QueryBalance(gateway.ClientID, req.QueryReq)
		case ReqQueryAccount:
			resp = e.QueryAccount(gateway.ClientID, req.QueryReq)
		case ReqQueryInstruments:
			resp = e.QueryInstruments(gateway.ClientID, req.QueryReq)
		case ReqSubscribe:
			resp = e.Subscribe(gateway.ClientID, req.QueryReq, gateway)
		case ReqUnsubscribe:
			resp = e.Unsubscribe(gateway.ClientID, req.QueryReq)
		}

		// At-least-once delivery: a dropped accept/reject leaves the actor's
		// in-flight order state desynchronized forever. Routed through the
		// outbox so it shares one FIFO with the fills/cancels the request
		// generated — a direct send here could overtake them.
		gateway.enqueueResponse(resp)
	}
}

// GetBestLiquidity returns best bid qty, best ask qty for a symbol, thread-safe.
func (e *DefaultExchange) GetBestLiquidity(symbol string) (bidQty, askQty int64) {
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
func (e *DefaultExchange) GetBook(symbol string) *OrderBook {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.Books[symbol]
}

// MidPrice computes a book's mid under the exchange lock. Implements
// price.MidPriceProvider: callers must not read a *OrderBook obtained from
// GetBook after the lock is released, since order handling mutates it.
func (e *DefaultExchange) MidPrice(symbol string) int64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	book := e.Books[symbol]
	if book == nil {
		return 0
	}
	return book.GetMidPrice()
}

func (e *DefaultExchange) ListInstruments(baseFilter, quoteFilter string) []Instrument {
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

// PublishSnapshot publishes the displayed order book to all subscribers.
// Caller must hold e.mu lock.
func (e *DefaultExchange) PublishSnapshot(symbol string, timestamp int64) {
	book := e.Books[symbol]
	if book == nil {
		return
	}
	snapshot := &BookSnapshot{
		Bids: book.Bids.GetPublicSnapshot(),
		Asks: book.Asks.GetPublicSnapshot(),
	}
	e.MDPublisher.Publish(symbol, MDSnapshot, snapshot, timestamp)
}

func (e *DefaultExchange) Shutdown() {
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
func (e *DefaultExchange) Lock() { e.mu.Lock() }

// Unlock releases the exchange write lock.
func (e *DefaultExchange) Unlock() { e.mu.Unlock() }

// RLock acquires the exchange read lock.
func (e *DefaultExchange) RLock() { e.mu.RLock() }

// RUnlock releases the exchange read lock.
func (e *DefaultExchange) RUnlock() { e.mu.RUnlock() }

// IsRunning returns whether the exchange is currently running.
func (e *DefaultExchange) IsRunning() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.running
}

// SettleFunding manually triggers a funding settlement for the given perpetual.
func (e *DefaultExchange) SettleFunding(perp *PerpFutures) {
	e.mu.Lock()
	defer e.mu.Unlock()
	settleFunding(e.Positions, e.Clients, perp, e.Clock, buildFundingSink(e))
}

// ConfigureAutomation sets automation parameters. Must be called before StartAutomation.
func (e *DefaultExchange) ConfigureAutomation(config AutomationConfig) {
	// Index-anchored marking is the default whenever no explicit calculator
	// was injected: a margined book marked at its own mid lets liquidations
	// trade into the very price that triggers them (self-feeding cascade).
	// Books with no resolvable index (genuine single-venue) keep the mid.
	e.autoAnchorMarks = config.MarkPriceCalc == nil
	if config.MarkPriceCalc == nil {
		config.MarkPriceCalc = NewMidPriceCalculator()
	}
	if config.PriceUpdateInterval == 0 {
		config.PriceUpdateInterval = 3 * time.Second
	}
	if config.CollateralRate == 0 {
		config.CollateralRate = 500
	}
	if config.MarkPriceEMAWindow == 0 {
		config.MarkPriceEMAWindow = 10
	}
	if config.MarkPriceBandBps == 0 {
		config.MarkPriceBandBps = 600
	}
	e.markEMAWindow = config.MarkPriceEMAWindow
	e.markBandBps = config.MarkPriceBandBps
	e.markPriceCalc = config.MarkPriceCalc
	e.markPriceCalcs = config.MarkPriceCalcs
	if e.markPriceCalcs == nil {
		e.markPriceCalcs = make(map[string]MarkPriceCalculator)
	}
	e.indexProvider = config.IndexProvider
	e.priceUpdateInterval = config.PriceUpdateInterval
	e.CollateralRate = config.CollateralRate
	e.LiquidationFeeBps = config.LiquidationFeeBps
	e.LiquidationHandler = config.LiquidationHandler
	e.listingPolicies = config.ListingPolicies
}

// StartAutomation begins automatic price updates, funding settlements, and collateral charging.
// Runs until ctx is cancelled or StopAutomation is called.
func (e *DefaultExchange) StartAutomation(ctx context.Context) {
	e.automMu.Lock()
	defer e.automMu.Unlock()

	if e.automCtx != nil {
		return
	}

	if e.priceUpdateInterval == 0 {
		e.priceUpdateInterval = 3 * time.Second
	}
	if e.markPriceCalc == nil {
		e.markPriceCalc = NewMidPriceCalculator()
		e.autoAnchorMarks = true
	}

	e.automCtx, e.automCancel = context.WithCancel(ctx)

	e.automWg.Add(1)
	go e.priceUpdateLoop()

	e.automWg.Add(1)
	go e.fundingSettlementLoop()

	e.automWg.Add(1)
	go e.collateralChargeLoop()

	e.automWg.Add(1)
	go e.expiryLoop()
}

// StopAutomation stops all automatic operations and waits for completion.
func (e *DefaultExchange) StopAutomation() {
	e.automMu.Lock()
	if e.automCancel != nil {
		e.automCancel()
	}
	e.automMu.Unlock()

	e.automWg.Wait()

	e.automMu.Lock()
	e.automCtx = nil
	e.automCancel = nil
	e.automMu.Unlock()
}

func (e *DefaultExchange) priceUpdateLoop() {
	defer e.automWg.Done()

	ticker := e.tickerFactory.NewTicker(e.priceUpdateInterval)
	defer ticker.Stop()

	e.updateAllPerpPrices()

	for {
		select {
		case <-e.automCtx.Done():
			return
		case <-ticker.C():
			e.updateAllPerpPrices()
		}
	}
}

func (e *DefaultExchange) fundingSettlementLoop() {
	defer e.automWg.Done()

	ticker := e.tickerFactory.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-e.automCtx.Done():
			return
		case <-ticker.C():
			e.CheckAndSettleFunding()
		}
	}
}

func (e *DefaultExchange) collateralChargeLoop() {
	defer e.automWg.Done()

	ticker := e.tickerFactory.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-e.automCtx.Done():
			return
		case <-ticker.C():
			e.ChargeCollateralInterest()
		}
	}
}

// updateAllPerpPrices updates funding rates for all perpetual instruments.
// indexSourceLocked resolves a margined book's index: the underlying book's
// mid when listed, else the IndexProvider. Never the book's own price —
// anchoring a mark to itself recreates the liquidation feedback loop. The
// returned source must only be called with e.mu held (either mode); the
// IndexProvider is called under that lock and must not call back into the
// exchange.
func (e *DefaultExchange) indexSourceLocked() PriceSource {
	return priceSourceFunc(func(symbol string) int64 {
		book := e.Books[symbol]
		if book == nil {
			return 0
		}
		if u := underlyingOf(book.Instrument); u != "" {
			if mid := e.bookMidPriceLocked(u); mid != 0 {
				return mid
			}
		}
		if e.indexProvider != nil {
			return e.indexProvider.Price(symbol)
		}
		return 0
	})
}

// ensureAnchoredMarkCalcs gives every margined book with a resolvable index a
// per-symbol ClampedEMA mark calculator, unless one was injected explicitly.
// Runs every price tick so instruments listed later (listing policies) are
// picked up; existing entries are never replaced, preserving EMA state.
func (e *DefaultExchange) ensureAnchoredMarkCalcs() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.markPriceCalcs == nil {
		e.markPriceCalcs = make(map[string]MarkPriceCalculator)
	}
	window, band := e.markEMAWindow, e.markBandBps
	if window == 0 {
		window = 10
	}
	if band == 0 {
		band = 600
	}
	if e.autoAnchoredSymbols == nil {
		e.autoAnchoredSymbols = make(map[string]bool)
	}
	for symbol, book := range e.Books {
		if marginCore(book.Instrument) == nil || e.markPriceCalcs[symbol] != nil {
			continue
		}
		if underlyingOf(book.Instrument) == "" && e.indexProvider == nil {
			continue
		}
		e.markPriceCalcs[symbol] = NewClampedEMAMarkPrice(symbol, e.indexSourceLocked(), window, band)
		e.autoAnchoredSymbols[symbol] = true
	}
}

// UpdatePerpPrices runs one mark/index/funding update pass over every
// margined book — the same pass the automation price loop runs on its
// ticker. Exposed for deterministic simulations and tests.
func (e *DefaultExchange) UpdatePerpPrices() {
	e.updateAllPerpPrices()
}

func (e *DefaultExchange) updateAllPerpPrices() {
	if e.autoAnchorMarks {
		e.ensureAnchoredMarkCalcs()
	}
	timestamp := e.Clock.NowUnixNano()

	// Collect mark prices under read lock. Price() must be called outside
	// the lock because MidPriceOracle.Price also acquires e.mu.RLock;
	// calling it while already holding e.mu.RLock deadlocks when a writer waits.
	type bookData struct {
		symbol     string
		perp       *PerpFutures
		markPrice  int64
		isPerp     bool
		underlying string
	}
	e.mu.RLock()
	candidates := make([]bookData, 0, len(e.Books))
	for _, book := range e.Books {
		// Perpetuals and anything exposing the perp margin core (dated
		// futures) get mark updates, margin calls, and liquidation sweeps.
		perp := marginCore(book.Instrument)
		if perp == nil {
			continue
		}
		isPerp := book.Instrument.IsPerp()
		calc := e.markPriceCalc
		if perSymbol := e.markPriceCalcs[book.Symbol]; perSymbol != nil {
			calc = perSymbol
		}
		markPrice := calc.Calculate(book)
		if markPrice == 0 {
			continue
		}
		candidates = append(candidates, bookData{
			symbol:     book.Symbol,
			perp:       perp,
			markPrice:  markPrice,
			isPerp:     isPerp,
			underlying: underlyingOf(book.Instrument),
		})
	}
	e.mu.RUnlock()

	// Symbol order, not map order: buildAccountMarginProfile prices
	// non-trigger symbols from their last STORED mark, so whether a
	// cross-margined sibling sees this tick's fresh mark or the previous
	// tick's stale one depends on processing order — and with it, whether a
	// borderline liquidation fires. Same seed, same state, every run.
	slices.SortFunc(candidates, func(a, b bookData) int { return cmp.Compare(a.symbol, b.symbol) })

	type perpUpdate struct {
		symbol     string
		perp       *PerpFutures
		markPrice  int64
		indexPrice int64
		isPerp     bool
	}
	updates := make([]perpUpdate, 0, len(candidates))
	for _, c := range candidates {
		indexPrice := int64(0)
		if c.underlying != "" {
			indexPrice = e.bookMidPrice(c.underlying)
		}
		if indexPrice == 0 && e.indexProvider != nil {
			indexPrice = e.indexProvider.Price(c.symbol)
		}
		if indexPrice == 0 {
			// An index is supposed to exist but is unavailable or stale:
			// skip this update rather than marking the perp against itself
			// (mark-as-index makes basis identically zero and hides outages).
			if c.underlying != "" || e.indexProvider != nil {
				continue
			}
			// Genuine single-venue configuration: the perp's own book is the
			// only price there is.
			indexPrice = c.markPrice
		}
		if indexPrice == 0 {
			continue
		}
		updates = append(updates, perpUpdate{
			symbol:     c.symbol,
			perp:       c.perp,
			markPrice:  c.markPrice,
			indexPrice: indexPrice,
			isPerp:     c.isPerp,
		})
	}

	for _, u := range updates {
		// FundingRate fields are mutated under e.mu only, and subscribers get
		// a snapshot copy — publishing the live pointer would let actor
		// goroutines read fields mid-update.
		e.mu.Lock()
		u.perp.UpdateFundingRate(u.indexPrice, u.markPrice)
		fundingSnapshot := *u.perp.GetFundingRate()
		e.mu.Unlock()

		if log := e.getLogger(u.symbol); log != nil {
			log.LogEvent(timestamp, 0, "mark_price_update", MarkPriceUpdateEvent{
				Timestamp:  timestamp,
				Symbol:     u.symbol,
				MarkPrice:  u.markPrice,
				IndexPrice: u.indexPrice,
			})
		}

		// Funding is a perpetual-only concept; dated futures reuse the rate
		// struct purely as mark-price state.
		if u.isPerp {
			e.MDPublisher.PublishFunding(u.symbol, &fundingSnapshot, timestamp)
			if log := e.getLogger(u.symbol); log != nil {
				log.LogEvent(timestamp, 0, "funding_rate_update", FundingRateUpdateEvent{
					Timestamp:   timestamp,
					Symbol:      u.symbol,
					Rate:        fundingSnapshot.Rate,
					NextFunding: fundingSnapshot.NextFunding,
				})
			}
		}

		e.CheckLiquidations(u.symbol, u.perp, u.markPrice)
	}
}

// underlyingOf returns the referenced spot symbol for derivatives, "" otherwise.
func underlyingOf(inst Instrument) string {
	if ref, ok := inst.(etypes.UnderlyingRef); ok {
		return ref.UnderlyingSymbol()
	}
	return ""
}

// marginCore returns the perp margin engine behind an instrument: the
// instrument itself for perpetuals, the embedded core for dated futures,
// nil for everything else.
func marginCore(inst Instrument) *PerpFutures {
	if p, ok := inst.(*PerpFutures); ok {
		return p
	}
	if pp, ok := inst.(interface{ Perp() *PerpFutures }); ok {
		return pp.Perp()
	}
	return nil
}

// ChargeCollateralInterest charges interest on borrowed amounts (one minute of time).
func (e *DefaultExchange) ChargeCollateralInterest() {
	e.mu.Lock()
	defer e.mu.Unlock()

	const dtSeconds = 60
	const secondsPerYear = 365 * 24 * 3600
	timestamp := e.Clock.NowUnixNano()

	// Client-ID and asset order: the debits are independent, but the emitted
	// balance/interest event stream must be identical run to run.
	clientIDs := make([]uint64, 0, len(e.Clients))
	for clientID := range e.Clients {
		clientIDs = append(clientIDs, clientID)
	}
	slices.Sort(clientIDs)

	for _, clientID := range clientIDs {
		client := e.Clients[clientID]
		assets := make([]string, 0, len(client.Borrowed))
		for asset := range client.Borrowed {
			assets = append(assets, asset)
		}
		slices.Sort(assets)
		for _, asset := range assets {
			borrowed := client.Borrowed[asset]
			if borrowed <= 0 {
				continue
			}
			interest := borrowed * e.CollateralRate * dtSeconds / (int64(secondsPerYear) * 10000)
			if interest > 0 {
				// Charge each wallet its attributed share of the debt: billing a
				// spot-credited loan's interest to the perp wallet drives a
				// spot-only borrower's empty perp balance negative every sweep.
				spotShare := int64(0)
				if spotPortion := client.BorrowedSpotPortion(asset); spotPortion > 0 {
					spotShare = interest * spotPortion / borrowed
				}
				perpShare := interest - spotShare

				changes := make([]BalanceDelta, 0, 2)
				if perpShare > 0 {
					oldPerp := client.PerpBalances[asset]
					client.PerpBalances[asset] -= perpShare
					changes = append(changes, perpDelta(asset, oldPerp, client.PerpBalances[asset]))
				}
				if spotShare > 0 {
					oldSpot := client.Balances[asset]
					client.Balances[asset] -= spotShare
					changes = append(changes, spotDelta(asset, oldSpot, client.Balances[asset]))
				}
				e.ExchangeBalance.FeeRevenue[asset] += interest

				logBalanceChange(e, timestamp, client.ID, "", "interest_charge", changes)

				if log := e.getLogger("_global"); log != nil {
					log.LogEvent(timestamp, client.ID, "margin_interest", MarginInterestEvent{
						Timestamp: timestamp,
						ClientID:  client.ID,
						Asset:     asset,
						Amount:    interest,
					})
				}
			}
		}
	}
}

// positionUPnL returns unrealized PnL for a position marked at markPrice.
func positionUPnL(pos *Position, markPrice, precision int64) int64 {
	if pos.Size >= 0 {
		return MulDiv(pos.Size, markPrice-pos.EntryPrice, precision)
	}
	return MulDiv(-pos.Size, pos.EntryPrice-markPrice, precision)
}

// accountMarginProfile aggregates a client's cross-margin exposure in the
// quote asset across every margined book: unrealized PnL, notional, and the
// maintenance/warning requirements. The triggering symbol is marked at
// triggerMark; other books use their latest stored mark, falling back to the
// book reference price, then the position entry (neutral) when no mark exists.
// Caller must hold e.mu.
type accountMarginProfile struct {
	UnrealizedPnL int64
	Notional      int64
	Maintenance   int64
	Warning       int64
}

func (e *DefaultExchange) buildAccountMarginProfile(clientID uint64, quote, triggerSymbol string, triggerMark int64) accountMarginProfile {
	var p accountMarginProfile
	for symbol, book := range e.Books {
		perp := marginCore(book.Instrument)
		if perp == nil {
			// Non-perp margined instruments (options) contribute their own
			// mark-to-market and maintenance; skipping them makes a short-vol
			// account invisible to the risk engine and unliquidatable.
			if pm, ok := book.Instrument.(PositionMarginer); ok && book.Instrument.QuoteAsset() == quote {
				e.addPositionMarginerExposure(&p, clientID, symbol, book.Instrument, pm)
			}
			continue
		}
		if perp.QuoteAsset() != quote {
			continue
		}
		mark := triggerMark
		if symbol != triggerSymbol {
			mark = perp.GetFundingRate().MarkPrice
			if mark == 0 {
				mark = marketRefPrice(book)
			}
		}
		precision := perp.BasePrecision()
		for _, side := range []PositionSide{PositionBoth, PositionLong, PositionShort} {
			pos := e.Positions.GetPositionBySide(clientID, symbol, side)
			if pos == nil || pos.Size == 0 {
				continue
			}
			m := mark
			if m == 0 {
				m = pos.EntryPrice
			}
			p.UnrealizedPnL += positionUPnL(pos, m, precision)
			notional := MulDiv(abs(pos.Size), m, precision)
			p.Notional += notional
			p.Maintenance += notional * perp.MaintenanceMarginRate / 10000
			p.Warning += notional * perp.WarningMarginRate / 10000
		}
	}
	return p
}

// CheckPositionMarginerLiquidations sweeps accounts holding positions on
// PositionMarginer instruments (options). These books never enter the perp
// mark loop — marginCore is nil — so an account with option exposure but NO
// perp position would otherwise never be evaluated for liquidation at all:
// a pure short-vol account could sink arbitrarily far underwater untouched.
// Runs on the derivative mark cadence, after marks refresh.
func (e *DefaultExchange) CheckPositionMarginerLiquidations() {
	timestamp := e.Clock.NowUnixNano()

	e.mu.Lock()
	defer e.mu.Unlock()

	// Deterministic sweep order: symbols, then client IDs.
	symbols := make([]string, 0)
	for symbol, book := range e.Books {
		if _, ok := book.Instrument.(PositionMarginer); ok {
			symbols = append(symbols, symbol)
		}
	}
	slices.Sort(symbols)

	clientIDs := make([]uint64, 0, len(e.Clients))
	for clientID := range e.Clients {
		clientIDs = append(clientIDs, clientID)
	}
	slices.Sort(clientIDs)

	for _, symbol := range symbols {
		book := e.Books[symbol]
		inst := book.Instrument
		quote := inst.QuoteAsset()
		for _, clientID := range clientIDs {
			client := e.Clients[clientID]
			var positions []*Position
			for _, side := range []PositionSide{PositionBoth, PositionLong, PositionShort} {
				pos := e.Positions.GetPositionBySide(clientID, symbol, side)
				if pos == nil || pos.Size == 0 {
					continue
				}
				positions = append(positions, pos)
			}
			if len(positions) == 0 {
				continue
			}

			// No trigger symbol: every book contributes its stored mark.
			profile := e.buildAccountMarginProfile(clientID, quote, "", 0)
			equity := client.PerpBalance(quote) - client.BorrowedPerpPortion(quote) + profile.UnrealizedPnL
			if equity >= profile.Maintenance {
				continue
			}
			for _, pos := range positions {
				e.liquidate(clientID, client, symbol, pos, inst, timestamp)
			}
		}
	}
}

// addPositionMarginerExposure folds a PositionMarginer instrument's open
// positions into the cross-margin profile: marked at the instrument's own
// mark, maintenance per its own formula. Warning reuses maintenance — these
// instruments carry no separate warning tier.
func (e *DefaultExchange) addPositionMarginerExposure(p *accountMarginProfile, clientID uint64, symbol string, inst Instrument, pm PositionMarginer) {
	precision := inst.BasePrecision()
	mark := pm.PositionMark()
	for _, side := range []PositionSide{PositionBoth, PositionLong, PositionShort} {
		pos := e.Positions.GetPositionBySide(clientID, symbol, side)
		if pos == nil || pos.Size == 0 {
			continue
		}
		m := mark
		if m == 0 {
			m = pos.EntryPrice
		}
		p.UnrealizedPnL += positionUPnL(pos, m, precision)
		p.Notional += MulDiv(abs(pos.Size), m, precision)
		maintenance := pm.MaintenanceForPosition(pos.Size, precision)
		// A short with zero maintenance means the instrument has no marks yet
		// (the underlying hasn't printed): the exposure is unknown, not zero.
		// Floor at the buy-back cost at the marked (or entry) premium so the
		// window before the first mark tick cannot hide a short position.
		if maintenance == 0 && pos.Size < 0 {
			maintenance = MulDiv(-pos.Size, m, precision)
		}
		p.Maintenance += maintenance
		p.Warning += maintenance
	}
}

// CheckLiquidations evaluates all positions for a symbol after a mark price update.
// Cross-margin account model: equity = perp balance − borrowed quote debt +
// unrealized PnL across EVERY margined book in the same quote asset (order
// margin is locked, not lost); the maintenance requirement likewise sums over
// all books, so the same cash can never back two symbols at once. On breach the triggering symbol's
// positions are closed; other symbols resolve on their own mark updates.
// Hedge-mode Long/Short positions are included.
func (e *DefaultExchange) CheckLiquidations(symbol string, perp *PerpFutures, markPrice int64) {
	if markPrice == 0 {
		return
	}
	quote := perp.QuoteAsset()

	e.mu.Lock()
	defer e.mu.Unlock()

	// Client-ID order, not map order: when book liquidity covers only one of
	// two simultaneous breaches, map iteration would pick the survivor
	// randomly per run — the same seed must produce the same final state.
	clientIDs := make([]uint64, 0, len(e.Clients))
	for clientID := range e.Clients {
		clientIDs = append(clientIDs, clientID)
	}
	slices.Sort(clientIDs)

	for _, clientID := range clientIDs {
		client := e.Clients[clientID]
		var positions []*Position
		for _, side := range []PositionSide{PositionBoth, PositionLong, PositionShort} {
			pos := e.Positions.GetPositionBySide(clientID, symbol, side)
			if pos == nil || pos.Size == 0 {
				continue
			}
			positions = append(positions, pos)
		}
		if len(positions) == 0 {
			continue
		}

		profile := e.buildAccountMarginProfile(clientID, quote, symbol, markPrice)
		unrealizedPnL, notional := profile.UnrealizedPnL, profile.Notional
		// Borrowed quote is cash in the wallet but a matching liability: counting
		// it as equity would let a loan mask an undercollateralized account and
		// dodge liquidation. Net only the perp-attributed share — a spot-credited
		// loan's cash never entered this wallet, so charging it here would
		// liquidate a solvent account.
		equity := client.PerpBalance(quote) - client.BorrowedPerpPortion(quote) + unrealizedPnL
		maintenanceMargin := profile.Maintenance
		warningMargin := profile.Warning

		timestamp := e.Clock.NowUnixNano()

		if equity < maintenanceMargin {
			if log := e.getLogger("_global"); log != nil {
				log.LogEvent(timestamp, clientID, "liquidation_check", map[string]any{
					"timestamp":          timestamp,
					"client_id":          clientID,
					"symbol":             symbol,
					"mark_price":         markPrice,
					"balance":            client.PerpBalances[quote],
					"reserved":           client.PerpReserved[quote],
					"unrealized_pnl":     unrealizedPnL,
					"equity":             equity,
					"notional":           notional,
					"maintenance_margin": maintenanceMargin,
				})
			}
			for _, pos := range positions {
				e.liquidate(clientID, client, symbol, pos, perp, timestamp)
			}
		} else if equity < warningMargin && e.LiquidationHandler != nil {
			marginRatio := int64(0)
			if notional > 0 {
				marginRatio = equity * 10000 / notional
			}
			liqPrice := e.EstimateLiquidationPrice(positions[0], clientID, perp, perp.BasePrecision())
			e.LiquidationHandler.OnMarginCall(&MarginCallEvent{
				Timestamp:        timestamp,
				ClientID:         clientID,
				Symbol:           symbol,
				MarginRatioBps:   marginRatio,
				LiquidationPrice: liqPrice,
			})
		}
	}
}

// EstimateLiquidationPrice returns the approximate mark price at which the
// position's equity would hit zero (maintenance requirement ignored).
func (e *DefaultExchange) EstimateLiquidationPrice(pos *Position, clientID uint64, perp *PerpFutures, precision int64) int64 {
	client := e.Clients[clientID]
	if client == nil || pos.Size == 0 {
		return 0
	}
	// Net perp-attributed debt out of the collateral: the loan is a liability,
	// so the price at which equity hits zero is reached sooner, not later.
	balance := client.PerpBalance(perp.QuoteAsset()) - client.BorrowedPerpPortion(perp.QuoteAsset())
	if pos.Size > 0 {
		return pos.EntryPrice - MulDiv(balance, precision, pos.Size)
	}
	return pos.EntryPrice + MulDiv(balance, precision, -pos.Size)
}

// liquidate forcibly closes a position via market order when maintenance margin is breached.
// Caller must hold e.mu.Lock().
func (e *DefaultExchange) liquidate(clientID uint64, client *Client, symbol string, pos *Position, inst Instrument, timestamp int64) {
	book := e.Books[symbol]
	if book == nil {
		return
	}

	closeSide := Sell
	if pos.Size < 0 {
		closeSide = Buy
	}
	fillPrice, filledQty := e.forceClose(clientID, client, book, book.Instrument, closeSide, pos.PositionSide, abs(pos.Size), timestamp)
	if fillPrice == 0 {
		// No liquidity in the book; position stays open for retry on next mark price update.
		return
	}

	// Fee on the quantity that actually closed: a thin book can absorb only
	// part of the position, and billing the full attempted size would
	// overcharge every partial liquidation.
	e.chargeClearanceFee(clientID, client, symbol, inst, filledQty, fillPrice, timestamp)

	if e.BorrowingMgr != nil {
		borrowed := client.Borrowed[inst.QuoteAsset()]
		if borrowed > 0 {
			availableForRepay := client.PerpAvailable(inst.QuoteAsset())
			if availableForRepay > 0 {
				repayAmount := min(borrowed, availableForRepay)

				oldBorrowed := client.Borrowed[inst.QuoteAsset()]
				oldPerp := client.PerpBalances[inst.QuoteAsset()]
				client.Borrowed[inst.QuoteAsset()] -= repayAmount
				client.PerpBalances[inst.QuoteAsset()] -= repayAmount

				logBalanceChange(e, timestamp, clientID, symbol, "liquidation_repay", []BalanceDelta{
					perpDelta(inst.QuoteAsset(), oldPerp, client.PerpBalances[inst.QuoteAsset()]),
					borrowedDelta(inst.QuoteAsset(), oldBorrowed, client.Borrowed[inst.QuoteAsset()]),
				})

				if log := e.getLogger("_global"); log != nil {
					log.LogEvent(timestamp, clientID, "repay", RepayEvent{
						Timestamp:     timestamp,
						ClientID:      clientID,
						Asset:         inst.QuoteAsset(),
						Principal:     repayAmount,
						Interest:      0,
						RemainingDebt: client.Borrowed[inst.QuoteAsset()],
					})
				}
			}
		}
	}

	// Settlement already released this position's margin and the book's order
	// margin was released by cancelClientOrdersOnBook. Remaining reservations
	// back the client's orders and positions on OTHER symbols — never zero them.
	// Bankruptcy is a negative cash balance after the close.
	quote := inst.QuoteAsset()
	balance := client.PerpBalances[quote]
	debt := int64(0)
	if balance < 0 {
		debt = -balance
		client.PerpBalances[quote] = 0
		e.ExchangeBalance.InsuranceFund[quote] -= debt

		logBalanceChange(e, timestamp, clientID, symbol, "liquidation_deficit", []BalanceDelta{
			perpDelta(quote, balance, 0),
		})
	}

	if e.LiquidationHandler != nil {
		e.LiquidationHandler.OnLiquidation(&LiquidationEvent{
			Timestamp:     timestamp,
			ClientID:      clientID,
			Symbol:        symbol,
			PositionSize:  pos.Size,
			FillPrice:     fillPrice,
			RemainingDebt: debt,
		})
		if debt > 0 {
			e.LiquidationHandler.OnInsuranceFund(&InsuranceFundEvent{
				Timestamp: timestamp,
				Symbol:    symbol,
				Delta:     -debt,
				Balance:   e.ExchangeBalance.InsuranceFund[quote],
				Reason:    "liquidation_deficit",
			})
		}
	}
}

// chargeClearanceFee debits the liquidated account a fee on the closed
// notional and credits the insurance fund — the venue mechanism that lets the
// fund grow in calm regimes and absorb deficits in cascades. Clamped to the
// account's available balance: the fee must not create fresh debt or invade
// reservations backing other books. Caller must hold e.mu.Lock().
func (e *DefaultExchange) chargeClearanceFee(clientID uint64, client *Client, symbol string, inst Instrument, closedSize, fillPrice, timestamp int64) {
	if e.LiquidationFeeBps <= 0 {
		return
	}
	quote := inst.QuoteAsset()
	fee := MulDiv(closedSize, fillPrice, inst.BasePrecision()) * e.LiquidationFeeBps / 10000
	if available := client.PerpAvailable(quote); fee > available {
		fee = available
	}
	if fee <= 0 {
		return
	}

	oldBalance := client.PerpBalances[quote]
	client.PerpBalances[quote] -= fee
	e.ExchangeBalance.InsuranceFund[quote] += fee

	logBalanceChange(e, timestamp, clientID, symbol, "liquidation_clearance_fee", []BalanceDelta{
		perpDelta(quote, oldBalance, client.PerpBalances[quote]),
	})
	if e.LiquidationHandler != nil {
		e.LiquidationHandler.OnInsuranceFund(&InsuranceFundEvent{
			Timestamp: timestamp,
			Symbol:    symbol,
			Delta:     fee,
			Balance:   e.ExchangeBalance.InsuranceFund[quote],
			Reason:    "clearance_fee",
		})
	}
}

// CheckAndSettleFunding checks if any perpetuals need funding settlement.
func (e *DefaultExchange) CheckAndSettleFunding() {
	e.mu.RLock()
	perps := make([]*PerpFutures, 0, len(e.Instruments))
	for _, inst := range e.Instruments {
		// Comma-ok, not a bare assertion: a custom Instrument may report IsPerp()
		// via an embedded *PerpFutures yet have a different concrete type, and a
		// bare inst.(*PerpFutures) would panic the whole funding sweep on it.
		if p, ok := inst.(*PerpFutures); ok {
			perps = append(perps, p)
		}
	}
	e.mu.RUnlock()

	// Deterministic settlement (and funding-event) order across runs.
	slices.SortFunc(perps, func(a, b *PerpFutures) int { return cmp.Compare(a.Symbol(), b.Symbol()) })

	now := e.Clock.NowUnixNano()

	for _, perp := range perps {
		// All FundingRate reads/writes stay under e.mu; subscribers receive a
		// snapshot copy, never the live pointer.
		e.mu.Lock()
		fundingRate := perp.GetFundingRate()
		if fundingRate.NextFunding == 0 {
			// First tick after start: anchor the schedule instead of settling
			// a full interval's funding at t=0.
			fundingRate.NextFunding = now + fundingRate.Interval*1e9
			e.mu.Unlock()
			continue
		}
		if now < fundingRate.NextFunding {
			e.mu.Unlock()
			continue
		}
		settleFunding(e.Positions, e.Clients, perp, e.Clock, buildFundingSink(e))
		fundingSnapshot := *fundingRate
		e.mu.Unlock()

		e.MDPublisher.PublishFunding(perp.Symbol(), &fundingSnapshot, now)
	}
}

// BorrowMargin borrows amount of asset for clientID. Acquires exchange lock.
func (e *DefaultExchange) BorrowMargin(clientID uint64, asset string, amount int64, reason string) error {
	if e.BorrowingMgr == nil {
		return errors.New("borrowing not enabled")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	client := e.Clients[clientID]
	ctx := buildBorrowContext(e, client, clientID)
	return e.BorrowingMgr.BorrowMargin(ctx, asset, amount, reason)
}

// RepayMargin repays amount of asset for clientID. Acquires exchange lock.
func (e *DefaultExchange) RepayMargin(clientID uint64, asset string, amount int64) error {
	if e.BorrowingMgr == nil {
		return errors.New("borrowing not enabled")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	client := e.Clients[clientID]
	ctx := buildBorrowContext(e, client, clientID)
	return e.BorrowingMgr.RepayMargin(ctx, asset, amount)
}
