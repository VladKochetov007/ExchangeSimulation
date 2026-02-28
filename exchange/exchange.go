package exchange

import (
	"context"
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

	// IndexProvider provides index prices for perpetuals (required for price updates)
	IndexProvider PriceSource

	// PriceUpdateInterval is how often to update funding rates (default: 3s)
	PriceUpdateInterval time.Duration

	// CollateralRate is annual interest rate on borrowed amounts in bps (default: 500 = 5%)
	CollateralRate int64

	// LiquidationHandler receives liquidation events (optional)
	LiquidationHandler LiquidationHandler
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
	LiquidationHandler      LiquidationHandler
	tickerFactory           TickerFactory
	markPriceCalc           MarkPriceCalculator
	indexProvider           PriceSource
	priceUpdateInterval     time.Duration
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

func (e *DefaultExchange) EnableBalanceSnapshots(snapshotInterval time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.balanceSnapshotInterval = snapshotInterval
	if e.running && snapshotInterval > 0 {
		e.balanceSnapshotStopCh = make(chan struct{})
		go e.runBalanceSnapshotLoop(snapshotInterval)
	}
}

func (e *DefaultExchange) runBalanceSnapshotLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-e.balanceSnapshotStopCh:
			return
		case <-e.shutdownCh:
			return
		case <-ticker.C:
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

func (e *DefaultExchange) ConnectClient(clientID uint64, initialBalances map[string]int64, feePlan FeeModel) Gateway {
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
		case ReqSubscribe:
			resp = e.Subscribe(gateway.ClientID, req.QueryReq, gateway)
		case ReqUnsubscribe:
			resp = e.Unsubscribe(gateway.ClientID, req.QueryReq)
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

// PublishSnapshot publishes a full order book snapshot to all subscribers.
// Caller must hold e.mu lock.
func (e *DefaultExchange) PublishSnapshot(symbol string, timestamp int64) {
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
	if config.MarkPriceCalc == nil {
		config.MarkPriceCalc = NewMidPriceCalculator()
	}
	if config.PriceUpdateInterval == 0 {
		config.PriceUpdateInterval = 3 * time.Second
	}
	if config.CollateralRate == 0 {
		config.CollateralRate = 500
	}
	e.markPriceCalc = config.MarkPriceCalc
	e.indexProvider = config.IndexProvider
	e.priceUpdateInterval = config.PriceUpdateInterval
	e.CollateralRate = config.CollateralRate
	e.LiquidationHandler = config.LiquidationHandler
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
	}

	e.automCtx, e.automCancel = context.WithCancel(ctx)

	e.automWg.Add(1)
	go e.priceUpdateLoop()

	e.automWg.Add(1)
	go e.fundingSettlementLoop()

	e.automWg.Add(1)
	go e.collateralChargeLoop()
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
func (e *DefaultExchange) updateAllPerpPrices() {
	timestamp := e.Clock.NowUnixNano()

	// Collect mark prices under read lock. Price() must be called outside
	// the lock because MidPriceOracle.Price also acquires e.mu.RLock;
	// calling it while already holding e.mu.RLock deadlocks when a writer waits.
	type bookData struct {
		symbol    string
		perp      *PerpFutures
		markPrice int64
	}
	e.mu.RLock()
	candidates := make([]bookData, 0, len(e.Books))
	for _, book := range e.Books {
		if !book.Instrument.IsPerp() {
			continue
		}
		markPrice := e.markPriceCalc.Calculate(book)
		if markPrice == 0 {
			continue
		}
		candidates = append(candidates, bookData{
			symbol:    book.Symbol,
			perp:      book.Instrument.(*PerpFutures),
			markPrice: markPrice,
		})
	}
	e.mu.RUnlock()

	type perpUpdate struct {
		symbol     string
		perp       *PerpFutures
		markPrice  int64
		indexPrice int64
	}
	updates := make([]perpUpdate, 0, len(candidates))
	for _, c := range candidates {
		indexPrice := e.indexProvider.Price(c.symbol)
		if indexPrice == 0 {
			continue
		}
		updates = append(updates, perpUpdate{
			symbol:     c.symbol,
			perp:       c.perp,
			markPrice:  c.markPrice,
			indexPrice: indexPrice,
		})
	}

	for _, u := range updates {
		u.perp.UpdateFundingRate(u.indexPrice, u.markPrice)
		e.MDPublisher.PublishFunding(u.symbol, u.perp.GetFundingRate(), timestamp)

		if log := e.getLogger(u.symbol); log != nil {
			log.LogEvent(timestamp, 0, "mark_price_update", MarkPriceUpdateEvent{
				Timestamp:  timestamp,
				Symbol:     u.symbol,
				MarkPrice:  u.markPrice,
				IndexPrice: u.indexPrice,
			})

			fundingRate := u.perp.GetFundingRate()
			log.LogEvent(timestamp, 0, "funding_rate_update", FundingRateUpdateEvent{
				Timestamp:   timestamp,
				Symbol:      u.symbol,
				Rate:        fundingRate.Rate,
				NextFunding: fundingRate.NextFunding,
			})
		}

		e.CheckLiquidations(u.symbol, u.perp, u.markPrice)
	}
}

// ChargeCollateralInterest charges interest on borrowed amounts (one minute of time).
func (e *DefaultExchange) ChargeCollateralInterest() {
	e.mu.Lock()
	defer e.mu.Unlock()

	const dtSeconds = 60
	const secondsPerYear = 365 * 24 * 3600
	timestamp := e.Clock.NowUnixNano()

	for _, client := range e.Clients {
		for asset, borrowed := range client.Borrowed {
			if borrowed <= 0 {
				continue
			}
			interest := borrowed * e.CollateralRate * dtSeconds / (int64(secondsPerYear) * 10000)
			if interest > 0 {
				oldBalance := client.PerpBalances[asset]
				client.PerpBalances[asset] -= interest
				e.ExchangeBalance.FeeRevenue[asset] += interest

				logBalanceChange(e, timestamp, client.ID, "", "interest_charge", []BalanceDelta{
					perpDelta(asset, oldBalance, client.PerpBalances[asset]),
				})

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

// CheckLiquidations evaluates all positions for a symbol after a mark price update.
func (e *DefaultExchange) CheckLiquidations(symbol string, perp *PerpFutures, markPrice int64) {
	if markPrice == 0 {
		return
	}
	precision := perp.BasePrecision()

	e.mu.Lock()
	defer e.mu.Unlock()

	for clientID, client := range e.Clients {
		pos := e.Positions.GetPosition(clientID, symbol)
		if pos == nil || pos.Size == 0 {
			continue
		}

		sign := int64(1)
		if pos.Size < 0 {
			sign = -1
		}
		unrealizedPnL := abs(pos.Size) * sign * (markPrice - pos.EntryPrice) / precision

		// Divide by precision first to avoid int64 overflow at real BTC prices.
		notional := abs(pos.Size) * pos.EntryPrice / precision
		initMargin := notional * perp.MarginRate / 10000
		if initMargin == 0 {
			continue
		}

		equity := client.PerpAvailable(perp.QuoteAsset()) + unrealizedPnL
		marginRatio := equity * 10000 / initMargin

		timestamp := e.Clock.NowUnixNano()

		if marginRatio < perp.MaintenanceMarginRate {
			if log := e.getLogger("_global"); log != nil {
				log.LogEvent(timestamp, clientID, "liquidation_check", map[string]any{
					"timestamp":      timestamp,
					"client_id":      clientID,
					"symbol":         symbol,
					"position_size":  pos.Size,
					"entry_price":    pos.EntryPrice,
					"mark_price":     markPrice,
					"balance":        client.PerpBalances[perp.QuoteAsset()],
					"reserved":       client.PerpReserved[perp.QuoteAsset()],
					"available":      client.PerpAvailable(perp.QuoteAsset()),
					"unrealized_pnl": unrealizedPnL,
					"equity":         equity,
					"init_margin":    initMargin,
					"margin_ratio":   marginRatio,
					"threshold":      perp.MaintenanceMarginRate,
				})
			}
			e.liquidate(clientID, client, symbol, pos, perp, timestamp)
		} else if marginRatio < perp.WarningMarginRate && e.LiquidationHandler != nil {
			liqPrice := e.EstimateLiquidationPrice(pos, clientID, perp, precision)
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

// EstimateLiquidationPrice returns the price at which the position would be liquidated.
func (e *DefaultExchange) EstimateLiquidationPrice(pos *Position, clientID uint64, perp *PerpFutures, precision int64) int64 {
	client := e.Clients[clientID]
	if client == nil || pos.Size == 0 {
		return 0
	}
	available := client.PerpAvailable(perp.QuoteAsset())
	if pos.Size > 0 {
		return pos.EntryPrice - available*precision/pos.Size
	}
	return pos.EntryPrice + available*precision/(-pos.Size)
}

// liquidate forcibly closes a position via market order when maintenance margin is breached.
// Caller must hold e.mu.Lock().
func (e *DefaultExchange) liquidate(clientID uint64, client *Client, symbol string, pos *Position, perp *PerpFutures, timestamp int64) {
	book := e.Books[symbol]
	if book == nil {
		return
	}

	closeSide := Sell
	if pos.Size < 0 {
		closeSide = Buy
	}
	fillPrice := e.forceClose(clientID, client, book, book.Instrument, closeSide, abs(pos.Size), timestamp)

	if e.BorrowingMgr != nil {
		borrowed := client.Borrowed[perp.QuoteAsset()]
		if borrowed > 0 {
			availableForRepay := client.PerpAvailable(perp.QuoteAsset())
			if availableForRepay > 0 {
				repayAmount := min(borrowed, availableForRepay)

				oldBorrowed := client.Borrowed[perp.QuoteAsset()]
				oldPerp := client.PerpBalances[perp.QuoteAsset()]
				client.Borrowed[perp.QuoteAsset()] -= repayAmount
				client.PerpBalances[perp.QuoteAsset()] -= repayAmount

				logBalanceChange(e, timestamp, clientID, symbol, "liquidation_repay", []BalanceDelta{
					perpDelta(perp.QuoteAsset(), oldPerp, client.PerpBalances[perp.QuoteAsset()]),
					borrowedDelta(perp.QuoteAsset(), oldBorrowed, client.Borrowed[perp.QuoteAsset()]),
				})

				if log := e.getLogger("_global"); log != nil {
					log.LogEvent(timestamp, clientID, "repay", RepayEvent{
						Timestamp:     timestamp,
						ClientID:      clientID,
						Asset:         perp.QuoteAsset(),
						Principal:     repayAmount,
						Interest:      0,
						RemainingDebt: client.Borrowed[perp.QuoteAsset()],
					})
				}
			}
		}
	}

	remainingEquity := client.PerpAvailable(perp.QuoteAsset())
	debt := int64(0)
	if remainingEquity < 0 {
		debt = -remainingEquity
		oldBalance := client.PerpBalances[perp.QuoteAsset()]
		oldReserved := client.PerpReserved[perp.QuoteAsset()]
		client.PerpBalances[perp.QuoteAsset()] = 0
		client.PerpReserved[perp.QuoteAsset()] = 0
		e.ExchangeBalance.InsuranceFund[perp.QuoteAsset()] -= debt

		logBalanceChange(e, timestamp, clientID, symbol, "liquidation_deficit", []BalanceDelta{
			perpDelta(perp.QuoteAsset(), oldBalance, 0),
			reservedPerpDelta(perp.QuoteAsset(), oldReserved, 0),
		})
	} else if remainingEquity > 0 {
		oldReserved := client.PerpReserved[perp.QuoteAsset()]
		client.PerpReserved[perp.QuoteAsset()] = 0

		logBalanceChange(e, timestamp, clientID, symbol, "liquidation_surplus", []BalanceDelta{
			reservedPerpDelta(perp.QuoteAsset(), oldReserved, 0),
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
		if debt > 0 || remainingEquity > 0 {
			e.LiquidationHandler.OnInsuranceFund(&InsuranceFundEvent{
				Timestamp: timestamp,
				Symbol:    symbol,
				Delta:     remainingEquity - debt,
				Balance:   e.ExchangeBalance.InsuranceFund[perp.QuoteAsset()],
			})
		}
	}
}

// CheckAndSettleFunding checks if any perpetuals need funding settlement.
func (e *DefaultExchange) CheckAndSettleFunding() {
	e.mu.RLock()
	perps := make([]*PerpFutures, 0, len(e.Instruments))
	for _, inst := range e.Instruments {
		if inst.IsPerp() {
			perps = append(perps, inst.(*PerpFutures))
		}
	}
	e.mu.RUnlock()

	now := e.Clock.NowUnixNano()

	for _, perp := range perps {
		fundingRate := perp.GetFundingRate()
		if now >= fundingRate.NextFunding {
			e.mu.Lock()
			settleFunding(e.Positions, e.Clients, perp, e.Clock, buildFundingSink(e))
			e.mu.Unlock()

			e.MDPublisher.PublishFunding(perp.Symbol(), perp.GetFundingRate(), now)
		}
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
