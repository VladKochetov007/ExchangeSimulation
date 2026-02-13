package exchange

import (
	"errors"
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

// ExchangeBalance tracks the exchange's own accumulated revenue and safety fund.
type ExchangeBalance struct {
	FeeRevenue    map[string]int64 `json:"fee_revenue"`
	InsuranceFund map[string]int64 `json:"insurance_fund"`
}

type Exchange struct {
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
	// EstimatedClients pre-allocates capacity for client maps (default: 10)
	EstimatedClients int

	// Clock provides time abstraction (default: RealClock)
	Clock Clock

	// SnapshotInterval is how often to publish market data snapshots (default: 100ms)
	SnapshotInterval time.Duration

	// SnapshotPollInterval is how often to check if snapshot is due (default: 1ms)
	// Lower values = more responsive to simulation time jumps but higher CPU usage
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
		return (bestBid + bestAsk) / 2
	}

	// Fallback to last trade price
	return ob.GetLastPrice()
}

type RealClock struct{}

func (c *RealClock) NowUnixNano() int64 { return time.Now().UnixNano() }
func (c *RealClock) NowUnix() int64     { return time.Now().Unix() }

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
	if config.EstimatedClients <= 0 {
		config.EstimatedClients = 10
	}
	if config.Clock == nil {
		config.Clock = &RealClock{}
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
		// If already running, start the loop now
		if e.snapshotInterval == 0 && interval > 0 {
			go e.runSnapshotLoop(interval)
		}
	}
	e.snapshotInterval = interval
}

func (e *Exchange) runSnapshotLoop(interval time.Duration) {
	lastSnapshotTime := e.Clock.NowUnixNano()
	intervalNanos := interval.Nanoseconds()

	// Poll at configured interval (real-time) to avoid busy-waiting
	// But publish based on simulation time elapsed
	// Lower poll interval = more responsive to sim time jumps but higher CPU
	ticker := time.NewTicker(e.snapshotPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Check if enough SIMULATION time has elapsed
			// Handle time jumps by publishing multiple snapshots if needed
			now := e.Clock.NowUnixNano()
			for now-lastSnapshotTime >= intervalNanos {
				e.logSnapshots()
				lastSnapshotTime += intervalNanos
			}
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

		snapshot := BalanceSnapshotComplete{
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
	gateway.Running = true
	e.Gateways[clientID] = gateway

	go e.handleClientRequests(gateway)

	if !e.running {
		e.running = true
		if e.snapshotInterval > 0 {
			go e.runSnapshotLoop(e.snapshotInterval)
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
		if instrument.IsPerp() {
			perp := instrument.(*PerpFutures)
			refPrice := book.GetMidPrice()
			if refPrice == 0 && book.Asks.Best != nil {
				refPrice = book.Asks.Best.Price
			}
			if refPrice == 0 && book.Bids.Best != nil {
				refPrice = book.Bids.Best.Price
			}
			if refPrice > 0 {
				estMargin := (req.Qty * refPrice / precision) * perp.MarginRate / 10000
				if client.PerpAvailable(instrument.QuoteAsset()) < estMargin {
					putOrder(order)
					resp := Response{RequestID: req.RequestID, Success: false, Error: RejectInsufficientBalance}
					if log != nil {
						log.LogEvent(e.Clock.NowUnixNano(), clientID, "OrderRejected", resp)
					}
					return resp
				}
			}
		} else if req.Side == Buy {
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
			if client.GetAvailable(instrument.BaseAsset()) < req.Qty {
				putOrder(order)
				resp := Response{RequestID: req.RequestID, Success: false, Error: RejectInsufficientBalance}
				if log != nil {
					log.LogEvent(e.Clock.NowUnixNano(), clientID, "OrderRejected", resp)
				}
				return resp
			}
		}
	case LimitOrder:
		if instrument.IsPerp() {
			perp := instrument.(*PerpFutures)
			initialMargin := (req.Qty * req.Price / precision) * perp.MarginRate / 10000
			if !client.ReservePerp(instrument.QuoteAsset(), initialMargin) {
				if e.BorrowingMgr != nil {
					e.mu.Unlock()
					borrowed, _ := e.BorrowingMgr.AutoBorrowForPerpTrade(clientID, instrument.QuoteAsset(), initialMargin)
					e.mu.Lock()
					if borrowed && client.ReservePerp(instrument.QuoteAsset(), initialMargin) {
						// Successfully borrowed and reserved
					} else {
						putOrder(order)
						resp := Response{RequestID: req.RequestID, Success: false, Error: RejectInsufficientBalance}
						if log != nil {
							log.LogEvent(e.Clock.NowUnixNano(), clientID, "OrderRejected", resp)
						}
						return resp
					}
				} else {
					putOrder(order)
					resp := Response{RequestID: req.RequestID, Success: false, Error: RejectInsufficientBalance}
					if log != nil {
						log.LogEvent(e.Clock.NowUnixNano(), clientID, "OrderRejected", resp)
					}
					return resp
				}
			}
		} else if req.Side == Buy {
			amount := (req.Qty * req.Price) / precision
			if !client.Reserve(instrument.QuoteAsset(), amount) {
				if e.BorrowingMgr != nil {
					e.mu.Unlock()
					borrowed, _ := e.BorrowingMgr.AutoBorrowForSpotTrade(clientID, instrument.QuoteAsset(), amount)
					e.mu.Lock()
					if borrowed && client.Reserve(instrument.QuoteAsset(), amount) {
						// Successfully borrowed and reserved
					} else {
						putOrder(order)
						resp := Response{RequestID: req.RequestID, Success: false, Error: RejectInsufficientBalance}
						if log != nil {
							log.LogEvent(e.Clock.NowUnixNano(), clientID, "OrderRejected", resp)
						}
						return resp
					}
				} else {
					putOrder(order)
					resp := Response{RequestID: req.RequestID, Success: false, Error: RejectInsufficientBalance}
					if log != nil {
						log.LogEvent(e.Clock.NowUnixNano(), clientID, "OrderRejected", resp)
					}
					return resp
				}
			}
		} else {
			if !client.Reserve(instrument.BaseAsset(), req.Qty) {
				if e.BorrowingMgr != nil {
					e.mu.Unlock()
					borrowed, _ := e.BorrowingMgr.AutoBorrowForSpotTrade(clientID, instrument.BaseAsset(), req.Qty)
					e.mu.Lock()
					if borrowed && client.Reserve(instrument.BaseAsset(), req.Qty) {
						// Successfully borrowed and reserved
					} else {
						putOrder(order)
						resp := Response{RequestID: req.RequestID, Success: false, Error: RejectInsufficientBalance}
						if log != nil {
							log.LogEvent(e.Clock.NowUnixNano(), clientID, "OrderRejected", resp)
						}
						return resp
					}
				} else {
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
			if instrument.IsPerp() {
				perp := instrument.(*PerpFutures)
				initMargin := (req.Qty * req.Price / precision) * perp.MarginRate / 10000
				client.ReleasePerp(instrument.QuoteAsset(), initMargin)
			} else if req.Side == Buy {
				client.Release(instrument.QuoteAsset(), (req.Qty*req.Price)/precision)
			} else {
				client.Release(instrument.BaseAsset(), req.Qty)
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
			remaining := order.Qty - order.FilledQty
			if instrument.IsPerp() {
				perp := instrument.(*PerpFutures)
				remainingMargin := (remaining * order.Price / precision) * perp.MarginRate / 10000
				client.ReleasePerp(instrument.QuoteAsset(), remainingMargin)
			} else if order.Side == Buy {
				remainingNotional := (remaining * order.Price) / precision
				client.Release(instrument.QuoteAsset(), remainingNotional)
			} else {
				client.Release(instrument.BaseAsset(), remaining)
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
	if instrument.IsPerp() {
		perp := instrument.(*PerpFutures)
		remainingMargin := (remainingQty * order.Price / precision) * perp.MarginRate / 10000
		client.ReleasePerp(instrument.QuoteAsset(), remainingMargin)
	} else if order.Side == Buy {
		client.Release(instrument.QuoteAsset(), (remainingQty*order.Price)/precision)
	} else {
		client.Release(instrument.BaseAsset(), remainingQty)
	}
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

		if instrument.IsPerp() {
			perp := instrument.(*PerpFutures)
			quote := instrument.QuoteAsset()

			// Snapshot old positions before update for PnL calculation
			takerDelta := e.Positions.UpdatePositionWithDelta(exec.TakerClientID, book.Symbol, exec.Qty, exec.Price, takerOrder.Side, e, "trade")
			makerDelta := e.Positions.UpdatePositionWithDelta(exec.MakerClientID, book.Symbol, exec.Qty, exec.Price, makerSide, e, "trade")

			// Calculate closed quantities
			takerClosedQty := calculateClosedQty(takerDelta.OldSize, exec.Qty, takerOrder.Side)
			makerClosedQty := calculateClosedQty(makerDelta.OldSize, exec.Qty, makerSide)

			// Handle margin for taker
			if takerOrder.Type == Market {
				// Market orders: reserve margin for opened portion only
				takerOpenedQty := exec.Qty - takerClosedQty
				if takerOpenedQty > 0 {
					marginToReserve := (takerOpenedQty * exec.Price / precision) * perp.MarginRate / 10000
					taker.ReservePerp(quote, marginToReserve)
				}
			} else {
				// Limit orders: order margin was pre-reserved
				// For closing portion, release order margin
				if takerClosedQty > 0 {
					orderMargin := (takerClosedQty * takerOrder.Price / precision) * perp.MarginRate / 10000
					taker.ReleasePerp(quote, orderMargin)
				}
				// For opening portion, margin stays reserved (transfers from order to position)
			}

			// Release position margin for closed portion (use entry price, not execution price)
			if takerClosedQty > 0 && takerDelta.OldSize != 0 {
				posMargin := (takerClosedQty * takerDelta.OldEntryPrice / precision) * perp.MarginRate / 10000
				taker.ReleasePerp(quote, posMargin)
			}

			// Handle margin for maker (always limit orders)
			if makerClosedQty > 0 {
				// Release order margin for closing portion
				makerOrder := book.Bids.Orders[exec.MakerOrderID]
				if makerOrder == nil {
					makerOrder = book.Asks.Orders[exec.MakerOrderID]
				}
				if makerOrder != nil {
					orderMargin := (makerClosedQty * makerOrder.Price / precision) * perp.MarginRate / 10000
					maker.ReleasePerp(quote, orderMargin)
				}
				// Release position margin (use entry price, not execution price)
				if makerDelta.OldSize != 0 {
					posMargin := (makerClosedQty * makerDelta.OldEntryPrice / precision) * perp.MarginRate / 10000
					maker.ReleasePerp(quote, posMargin)
				}
			}
			// For opening portion, margin stays reserved (transfers from order to position)

			// Realize PnL for closing trades
			basePrecision := perp.BasePrecision()
			takerPnL := realizedPerpPnL(takerDelta.OldSize, takerDelta.OldEntryPrice, exec.Qty, exec.Price, takerOrder.Side, basePrecision)
			makerPnL := realizedPerpPnL(makerDelta.OldSize, makerDelta.OldEntryPrice, exec.Qty, exec.Price, makerSide, basePrecision)

			// Log realized PnL if position was closed/reduced
			if takerPnL != 0 {
				if log := e.getLogger("_global"); log != nil {
					log.LogEvent(timestamp, exec.TakerClientID, "realized_pnl", RealizedPnLEvent{
						Timestamp:  timestamp,
						ClientID:   exec.TakerClientID,
						Symbol:     book.Symbol,
						TradeID:    book.SeqNum,
						ClosedQty:  calculateClosedQty(takerDelta.OldSize, exec.Qty, takerOrder.Side),
						EntryPrice: takerDelta.OldEntryPrice,
						ExitPrice:  exec.Price,
						PnL:        takerPnL,
						Side:       takerOrder.Side.String(),
					})
				}
			}
			if makerPnL != 0 {
				if log := e.getLogger("_global"); log != nil {
					log.LogEvent(timestamp, exec.MakerClientID, "realized_pnl", RealizedPnLEvent{
						Timestamp:  timestamp,
						ClientID:   exec.MakerClientID,
						Symbol:     book.Symbol,
						TradeID:    book.SeqNum,
						ClosedQty:  calculateClosedQty(makerDelta.OldSize, exec.Qty, makerSide),
						EntryPrice: makerDelta.OldEntryPrice,
						ExitPrice:  exec.Price,
						PnL:        makerPnL,
						Side:       makerSide.String(),
					})
				}
			}

			oldTakerBalance := taker.PerpBalances[quote]
			oldMakerBalance := maker.PerpBalances[quote]
			taker.PerpBalances[quote] += takerPnL - takerFee.Amount
			maker.PerpBalances[quote] += makerPnL - makerFee.Amount

			e.balanceTracker.LogBalanceChange(timestamp, exec.TakerClientID, book.Symbol, "trade_settlement", []BalanceDelta{
				perpDelta(quote, oldTakerBalance, taker.PerpBalances[quote]),
			})
			e.balanceTracker.LogBalanceChange(timestamp, exec.MakerClientID, book.Symbol, "trade_settlement", []BalanceDelta{
				perpDelta(quote, oldMakerBalance, maker.PerpBalances[quote]),
			})

			e.ExchangeBalance.FeeRevenue[quote] += takerFee.Amount + makerFee.Amount

			// Log fee revenue per trade (real exchanges track this)
			if globalLog := e.getLogger("_global"); globalLog != nil {
				globalLog.LogEvent(timestamp, 0, "fee_revenue", FeeRevenueEvent{
					Timestamp: timestamp,
					Symbol:    book.Symbol,
					TradeID:   book.SeqNum,
					TakerFee:  takerFee.Amount,
					MakerFee:  makerFee.Amount,
					Asset:     quote,
				})
			}

			positionChanged = true
		} else if takerOrder.Side == Buy {
			taker.Release(instrument.QuoteAsset(), notional)
			oldTakerQuote := taker.Balances[instrument.QuoteAsset()]
			oldTakerBase := taker.Balances[instrument.BaseAsset()]
			taker.Balances[instrument.QuoteAsset()] -= notional + takerFee.Amount
			taker.Balances[instrument.BaseAsset()] += exec.Qty
			e.balanceTracker.LogBalanceChange(timestamp, exec.TakerClientID, book.Symbol, "trade_settlement", []BalanceDelta{
				spotDelta(instrument.QuoteAsset(), oldTakerQuote, taker.Balances[instrument.QuoteAsset()]),
				spotDelta(instrument.BaseAsset(), oldTakerBase, taker.Balances[instrument.BaseAsset()]),
			})

			maker.Release(instrument.BaseAsset(), exec.Qty)
			oldMakerQuote := maker.Balances[instrument.QuoteAsset()]
			oldMakerBase := maker.Balances[instrument.BaseAsset()]
			maker.Balances[instrument.QuoteAsset()] += notional - makerFee.Amount
			maker.Balances[instrument.BaseAsset()] -= exec.Qty
			e.balanceTracker.LogBalanceChange(timestamp, exec.MakerClientID, book.Symbol, "trade_settlement", []BalanceDelta{
				spotDelta(instrument.QuoteAsset(), oldMakerQuote, maker.Balances[instrument.QuoteAsset()]),
				spotDelta(instrument.BaseAsset(), oldMakerBase, maker.Balances[instrument.BaseAsset()]),
			})

			e.ExchangeBalance.FeeRevenue[instrument.QuoteAsset()] += takerFee.Amount + makerFee.Amount

			// Log fee revenue for spot buy trades
			if globalLog := e.getLogger("_global"); globalLog != nil {
				globalLog.LogEvent(timestamp, 0, "fee_revenue", FeeRevenueEvent{
					Timestamp: timestamp,
					Symbol:    book.Symbol,
					TradeID:   book.SeqNum,
					TakerFee:  takerFee.Amount,
					MakerFee:  makerFee.Amount,
					Asset:     instrument.QuoteAsset(),
				})
			}
		} else {
			taker.Release(instrument.BaseAsset(), exec.Qty)
			oldTakerBase := taker.Balances[instrument.BaseAsset()]
			oldTakerQuote := taker.Balances[instrument.QuoteAsset()]
			taker.Balances[instrument.BaseAsset()] -= exec.Qty
			taker.Balances[instrument.QuoteAsset()] += notional - takerFee.Amount
			e.balanceTracker.LogBalanceChange(timestamp, exec.TakerClientID, book.Symbol, "trade_settlement", []BalanceDelta{
				spotDelta(instrument.BaseAsset(), oldTakerBase, taker.Balances[instrument.BaseAsset()]),
				spotDelta(instrument.QuoteAsset(), oldTakerQuote, taker.Balances[instrument.QuoteAsset()]),
			})

			maker.Release(instrument.QuoteAsset(), notional)
			oldMakerBase := maker.Balances[instrument.BaseAsset()]
			oldMakerQuote := maker.Balances[instrument.QuoteAsset()]
			maker.Balances[instrument.BaseAsset()] += exec.Qty
			maker.Balances[instrument.QuoteAsset()] -= notional + makerFee.Amount
			e.balanceTracker.LogBalanceChange(timestamp, exec.MakerClientID, book.Symbol, "trade_settlement", []BalanceDelta{
				spotDelta(instrument.BaseAsset(), oldMakerBase, maker.Balances[instrument.BaseAsset()]),
				spotDelta(instrument.QuoteAsset(), oldMakerQuote, maker.Balances[instrument.QuoteAsset()]),
			})

			e.ExchangeBalance.FeeRevenue[instrument.QuoteAsset()] += takerFee.Amount + makerFee.Amount

			// Log fee revenue for spot sell trades
			if globalLog := e.getLogger("_global"); globalLog != nil {
				globalLog.LogEvent(timestamp, 0, "fee_revenue", FeeRevenueEvent{
					Timestamp: timestamp,
					Symbol:    book.Symbol,
					TradeID:   book.SeqNum,
					TakerFee:  takerFee.Amount,
					MakerFee:  makerFee.Amount,
					Asset:     instrument.QuoteAsset(),
				})
			}
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
