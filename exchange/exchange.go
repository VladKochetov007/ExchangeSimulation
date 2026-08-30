package exchange

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	einstrument "exchange_sim/instrument"
	ematching "exchange_sim/matching"
	eprice "exchange_sim/price"
	etypes "exchange_sim/types"
)

// ExchangeBalance tracks the exchange's own accumulated revenue and safety fund.
type ExchangeBalance struct {
	FeeRevenue    map[string]int64 `json:"fee_revenue"`
	InsuranceFund map[string]int64 `json:"insurance_fund"`
}

// ErrSettlementPendingExposure is returned when an account still has a
// position in a contract whose expiry settlement is unavailable. Until that
// position is settled, admitting a new order or loan would let the account
// increase obligations while its cross-margin collateral cannot be valued.
var ErrSettlementPendingExposure = errors.New("account has settlement-pending exposure")

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
	// IndexFeedSymbols are published on the public MDIndex feed at each price
	// update, the way a venue publishes an index alongside its books.
	IndexFeedSymbols []string
	// IndexFeedProvider prices those symbols. It is separate from
	// IndexProvider because the reference a venue advertises need not be the
	// one it marks its own derivatives with. Defaults to IndexProvider.
	IndexFeedProvider PriceSource

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

	// PreExpiryHook runs synchronously after derivative marks refresh and before
	// expired instruments are cash-settled and delisted. It is for read-only
	// research/risk snapshots that must retain near-expiry state. The hook must
	// not submit orders or otherwise mutate this exchange.
	PreExpiryHook func()

	// PostDerivativeMarkHook runs synchronously after every derivative mark
	// refresh and before expiry settlement. It is read-only observability for
	// ordered simulation telemetry; callers must not mutate this exchange.
	PostDerivativeMarkHook func()
}

type phaseJobGroup uint8

const (
	phaseJobSnapshots phaseJobGroup = iota
	phaseJobBalances
	phaseJobAutomation
)

type phaseJob struct {
	group  phaseJobGroup
	ticker Ticker
	fn     func()
}

// instrumentLogEvent preserves the symbol for a dynamically listed
// instrument written to a shared fallback stream. Symbol-specific loggers keep
// their established event schema; only the mixed fallback needs this envelope.
type instrumentLogEvent struct {
	Symbol  string `json:"symbol"`
	Payload any    `json:"payload"`
}

type scopedInstrumentLogger struct {
	symbol string
	Logger
}

func (l scopedInstrumentLogger) LogEvent(simTime int64, clientID uint64, eventName string, event any) {
	l.Logger.LogEvent(simTime, clientID, eventName, instrumentLogEvent{
		Symbol:  l.symbol,
		Payload: event,
	})
}

type DefaultExchange struct {
	ID          string
	Clients     map[uint64]*Client
	Gateways    map[uint64]*ClientGateway
	Books       map[string]*OrderBook
	Instruments map[string]Instrument
	// instrumentListedAt retains the original public listing time. Reference-
	// data replays must not rewrite contract tenor as subscription time.
	instrumentListedAt map[string]int64
	Positions          PositionStore
	ExchangeBalance    *ExchangeBalance
	// conservation accumulates every recorded movement so that a balance
	// changed without one can be detected, which no audit of the log itself
	// could do: the log would be self-consistent and merely incomplete.
	conservation *conservationTracker
	// venueBalanceSequence is an exchange-local total order for venue ledger
	// movements. Logs are split by symbol, so timestamps alone cannot recover
	// the order of same-timestamp fee and insurance updates across files.
	venueBalanceSequence uint64
	// RequestPolicy meters and admits incoming requests. Nil leaves the venue
	// unmetered, which is what scenarios without a published budget expect.
	RequestPolicy                RequestPolicy
	NextOrderID                  uint64
	Matcher                      MatchingEngine
	MDPublisher                  *MDPublisher
	Clock                        Clock
	Loggers                      map[string]Logger
	instrumentLogFallback        Logger
	BorrowingMgr                 *BorrowingManager
	CollateralRate               int64
	LiquidationFeeBps            int64
	requireExactLinearAccounting bool
	autoAnchorMarks              bool
	deterministicIngress         bool
	deterministicPhases          bool
	markEMAWindow                int
	markBandBps                  int64
	autoAnchoredSymbols          map[string]bool
	requestsInFlight             atomic.Int64
	// automInFlight counts automation-loop work (mark prices, funding,
	// expiry) in progress. These loops react to the same clock the runner
	// advances, so a barrier that ignored them would move time while the
	// exchange was still repricing.
	automInFlight       atomic.Int64
	LiquidationHandler  LiquidationHandler
	tickerFactory       TickerFactory
	markPriceCalc       MarkPriceCalculator
	markPriceCalcs      map[string]MarkPriceCalculator
	indexProvider       PriceSource
	indexFeedSymbols    []string
	indexFeedProvider   PriceSource
	priceUpdateInterval time.Duration
	listingPolicies     []etypes.ListingPolicy
	// settlementPending holds the explicit post-expiry state for contracts
	// whose declared settlement source is unavailable. They remain permanently
	// halted while retries continue under the declared retry-forever policy;
	// neither a zero nor a hidden last-trade fallback can settle them.
	settlementPending       map[string]expirySettlementPending
	preExpiryHook           func()
	postDerivativeMarkHook  func()
	automCtx                context.Context
	automCancel             context.CancelFunc
	automWg                 sync.WaitGroup
	automMu                 sync.RWMutex
	phaseMu                 sync.Mutex
	phaseJobs               []phaseJob
	mu                      sync.RWMutex
	running                 bool
	closed                  bool
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

	// DeterministicIngress disables one request goroutine per client. Callers
	// drive DrainIngress at model-defined boundaries, so equal-arrival request
	// priority is not chosen by Go scheduling.
	DeterministicIngress bool

	// DeterministicPhases replaces asynchronous exchange jobs and response
	// delivery with an explicit simulation-runner pump. It requires
	// DeterministicIngress; scheduler-backed mounts must additionally opt into
	// the simulation runner's deterministic latency courier.
	DeterministicPhases bool

	// RequireExactLinearPositionAccounting rejects a custom position store that
	// cannot provide one atomic cost-basis transition and matching valuation.
	// It is the required mode for conservation-scored research runs.
	RequireExactLinearPositionAccounting bool

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
	if config.DeterministicPhases {
		// A phase runner owns every state transition. Leaving the legacy
		// request goroutine enabled would reintroduce scheduler-dependent
		// matching even though phase mode was requested.
		config.DeterministicIngress = true
	}
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
		ID:                 config.ID,
		Clients:            make(map[uint64]*Client, config.EstimatedClients),
		Gateways:           make(map[uint64]*ClientGateway, config.EstimatedClients),
		Books:              make(map[string]*OrderBook, 16),
		Instruments:        make(map[string]Instrument, 16),
		instrumentListedAt: make(map[string]int64, 16),
		Positions:          NewPositionManager(config.Clock),
		ExchangeBalance: &ExchangeBalance{
			FeeRevenue:    make(map[string]int64),
			InsuranceFund: make(map[string]int64),
		},
		conservation:                 newConservationTracker(),
		NextOrderID:                  1,
		Matcher:                      matcher,
		MDPublisher:                  NewMDPublisher(),
		Clock:                        config.Clock,
		Loggers:                      make(map[string]Logger),
		settlementPending:            make(map[string]expirySettlementPending),
		tickerFactory:                config.TickerFactory,
		deterministicIngress:         config.DeterministicIngress,
		deterministicPhases:          config.DeterministicPhases,
		requireExactLinearAccounting: config.RequireExactLinearPositionAccounting,
		running:                      false,
		shutdownCh:                   make(chan struct{}),
		snapshotStopCh:               make(chan struct{}),
		snapshotInterval:             config.SnapshotInterval,
		snapshotPollInterval:         config.SnapshotPollInterval,
		balanceSnapshotStopCh:        make(chan struct{}),
		balanceSnapshotInterval:      config.BalanceSnapshotInterval,
	}
	if policy, ok := ex.Positions.(interface{ SetRequireExactLinearPositionAccounting(bool) }); ok {
		policy.SetRequireExactLinearPositionAccounting(config.RequireExactLinearPositionAccounting)
	}
	return ex
}

// DeterministicPhasesEnabled reports whether this exchange was constructed
// for the runner's explicit synchronous phase runtime.
func (e *DefaultExchange) DeterministicPhasesEnabled() bool {
	return e.deterministicPhases
}

func (e *DefaultExchange) addDeterministicPhaseJob(group phaseJobGroup, ticker Ticker, fn func()) {
	e.phaseMu.Lock()
	e.phaseJobs = append(e.phaseJobs, phaseJob{group: group, ticker: ticker, fn: fn})
	e.phaseMu.Unlock()
}

func (e *DefaultExchange) stopDeterministicPhaseJobs(group *phaseJobGroup) {
	e.phaseMu.Lock()
	jobs := e.phaseJobs[:0]
	var stopped []Ticker
	for _, job := range e.phaseJobs {
		if group == nil || job.group == *group {
			stopped = append(stopped, job.ticker)
			continue
		}
		jobs = append(jobs, job)
	}
	e.phaseJobs = jobs
	e.phaseMu.Unlock()

	for _, ticker := range stopped {
		ticker.Stop()
		discardTickerTicks(ticker)
	}
}

// discardTickerTicks consumes work that was delivered before a periodic job
// was retired. Scheduler-backed tickers account for every delivery until it
// is acknowledged; removing the job without this drain leaves the simulation
// permanently non-idle.
func discardTickerTicks(ticker Ticker) {
	for {
		select {
		case _, ok := <-ticker.C():
			if !ok {
				return
			}
			acknowledgeTicker(ticker)
		default:
			return
		}
	}
}

// PumpDeterministicPhase runs due exchange-owned jobs in their registration
// order. Snapshot/balance jobs register when the first client connects;
// automation jobs register when StartAutomation is called. This is the only
// phase-mode path that invokes those callbacks.
func (e *DefaultExchange) PumpDeterministicPhase() bool {
	if !e.deterministicPhases {
		return false
	}

	processed := false
	for {
		e.phaseMu.Lock()
		jobs := append([]phaseJob(nil), e.phaseJobs...)
		e.phaseMu.Unlock()

		progress := false
		for _, job := range jobs {
			select {
			case <-job.ticker.C():
				e.automInFlight.Add(1)
				job.fn()
				e.automInFlight.Add(-1)
				acknowledgeTicker(job.ticker)
				processed = true
				progress = true
			default:
			}
		}
		if !progress {
			return processed
		}
	}
}

func (e *DefaultExchange) EnablePeriodicSnapshots(interval time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.running {
		if e.snapshotInterval == 0 && interval > 0 {
			ticker := e.tickerFactory.NewTicker(interval)
			if e.deterministicPhases {
				e.addDeterministicPhaseJob(phaseJobSnapshots, ticker, e.logSnapshots)
			} else {
				go e.runSnapshotLoop(ticker)
			}
		}
	}
	e.snapshotInterval = interval
}

func (e *DefaultExchange) runSnapshotLoop(ticker Ticker) {
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C():
			e.automInFlight.Add(1)
			e.logSnapshots()
			e.automInFlight.Add(-1)
			acknowledgeTicker(ticker)
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
	symbols := make([]string, 0, len(e.Books))
	for symbol := range e.Books {
		symbols = append(symbols, symbol)
	}
	slices.Sort(symbols)
	for _, symbol := range symbols {
		book := e.Books[symbol]
		// Subscribers get the displayed book; loggers keep the god view.
		e.MDPublisher.Publish(symbol, MDSnapshot, &BookSnapshot{
			Bids: book.Bids.GetPublicSnapshot(),
			Asks: book.Asks.GetPublicSnapshot(),
		}, timestamp)

		if log := e.getLogger(symbol); log != nil {
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
		if e.deterministicPhases {
			group := phaseJobBalances
			e.stopDeterministicPhaseJobs(&group)
			e.addDeterministicPhaseJob(phaseJobBalances, e.tickerFactory.NewTicker(interval), e.LogAllBalances)
		} else {
			e.balanceSnapshotStopCh = make(chan struct{})
			go e.runBalanceSnapshotLoop(interval)
		}
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
			e.automInFlight.Add(1)
			e.LogAllBalances()
			e.automInFlight.Add(-1)
			acknowledgeTicker(ticker)
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

	// Client-ID order so the emitted snapshot stream is byte-comparable
	// between runs; these logs are the measurement medium for experiments.
	snapshotClientIDs := make([]uint64, 0, len(e.Clients))
	for clientID := range e.Clients {
		snapshotClientIDs = append(snapshotClientIDs, clientID)
	}
	slices.Sort(snapshotClientIDs)

	for _, clientID := range snapshotClientIDs {
		client := e.Clients[clientID]
		spotBalances := make([]AssetBalance, 0, len(client.Balances))
		spotAssets := make([]string, 0, len(client.Balances))
		for asset := range client.Balances {
			spotAssets = append(spotAssets, asset)
		}
		slices.Sort(spotAssets)
		for _, asset := range spotAssets {
			total := client.Balances[asset]
			locked := client.Reserved[asset]
			borrowed := client.BorrowedSpotPortion(asset)
			spotBalances = append(spotBalances, AssetBalance{
				Asset:    asset,
				Free:     total - locked,
				Locked:   locked,
				Borrowed: borrowed,
				NetAsset: total - borrowed,
			})
		}

		perpBalances := make([]AssetBalance, 0, len(client.PerpBalances))
		perpAssets := make([]string, 0, len(client.PerpBalances))
		for asset := range client.PerpBalances {
			perpAssets = append(perpAssets, asset)
		}
		slices.Sort(perpAssets)
		for _, asset := range perpAssets {
			total := client.PerpBalances[asset]
			locked := client.PerpReserved[asset]
			borrowed := client.BorrowedPerpPortion(asset)
			perpBalances = append(perpBalances, AssetBalance{
				Asset:    asset,
				Free:     total - locked,
				Locked:   locked,
				Borrowed: borrowed,
				NetAsset: total - borrowed,
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

// SetInstrumentLoggerFallback records events for dynamically listed symbols
// that do not have a dedicated logger. It deliberately does not replace the
// _global logger: balance and lifecycle records must remain venue-scoped.
func (e *DefaultExchange) SetInstrumentLoggerFallback(log Logger) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.instrumentLogFallback = log
}

func (e *DefaultExchange) getLogger(symbol string) Logger {
	if log := e.Loggers[symbol]; log != nil {
		return log
	}
	if symbol != "_global" {
		if log := e.instrumentLogFallback; log != nil {
			return scopedInstrumentLogger{symbol: symbol, Logger: log}
		}
	}
	return nil
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
	if ref, ok := instrument.(etypes.UnderlyingRef); ok {
		// Settlement observations are raw fixed-point prices. Without an FX
		// conversion layer, accepting a derivative whose underlying has a
		// different base, quote, or precision silently settles in the wrong
		// denomination. Preserve existing externally-indexed instruments (no
		// local underlying book) but reject incompatible local references.
		if underlying := e.Books[ref.UnderlyingSymbol()]; underlying != nil {
			u := underlying.Instrument
			if instrument.BaseAsset() != u.BaseAsset() ||
				instrument.QuoteAsset() != u.QuoteAsset() ||
				instrument.BasePrecision() != u.BasePrecision() ||
				instrument.QuotePrecision() != u.QuotePrecision() {
				return
			}
		}
	}
	_, isLinear := instrument.(etypes.Margined)
	if e.requireExactLinearAccounting && isLinear {
		if _, ok := e.Positions.(etypes.ExactLinearPositionStore); !ok {
			panic("exchange: exact linear position accounting is required")
		}
	}
	e.Instruments[symbol] = instrument
	if isLinear {
		if registrar, ok := e.Positions.(etypes.PositionPrecisionRegistrar); ok {
			registrar.SetPositionPrecision(symbol, instrument.BasePrecision())
		}
	}
	e.instrumentListedAt[symbol] = e.Clock.NowUnixNano()
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
		e.logExchangeForcedCancellation(book, order, remainingQty, exchangeForcedLifecycleReason)
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
	if e.closed {
		gateway := NewClientGateway(clientID)
		gateway.Close()
		return gateway
	}

	timestamp := e.Clock.NowUnixNano()
	// Reconnect on a known ID reuses the existing account: overwriting it
	// would zero the ledgers while the client's resting orders keep settling
	// against them (silent conservation break). Deposits apply only on the
	// first connect — a reconnect is a new session, not a new account.
	client := e.Clients[clientID]
	if client == nil {
		client = NewClient(clientID, feePlan)
		var changes []BalanceDelta
		assets := make([]string, 0, len(initialBalances))
		for asset := range initialBalances {
			assets = append(assets, asset)
		}
		slices.Sort(assets)
		for _, asset := range assets {
			amount := initialBalances[asset]
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
	if client != nil {
		// A reconnect starts a new market-data session. Keep account state, but
		// never carry subscriptions or gateway references from the retired one.
		e.MDPublisher.UnsubscribeClient(clientID)
	}

	gateway := NewClientGateway(clientID)
	if e.deterministicPhases {
		gateway.enableDeterministicPhaseEgress()
	}
	e.Gateways[clientID] = gateway

	if !e.deterministicIngress {
		go e.HandleClientRequests(gateway)
	}

	if !e.running {
		e.running = true
		if e.snapshotInterval > 0 {
			ticker := e.tickerFactory.NewTicker(e.snapshotInterval)
			if e.deterministicPhases {
				e.addDeterministicPhaseJob(phaseJobSnapshots, ticker, e.logSnapshots)
			} else {
				go e.runSnapshotLoop(ticker)
			}
		}
		if e.balanceSnapshotInterval > 0 {
			if e.deterministicPhases {
				e.addDeterministicPhaseJob(phaseJobBalances, e.tickerFactory.NewTicker(e.balanceSnapshotInterval), e.LogAllBalances)
			} else {
				e.balanceSnapshotStopCh = make(chan struct{})
				go e.runBalanceSnapshotLoop(e.balanceSnapshotInterval)
			}
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
	e.MDPublisher.UnsubscribeClient(clientID)
}

// Idle reports whether the exchange has no client request queued or being
// processed and every gateway has drained. A deterministic runner uses this
// to decide the exchange has finished reacting before simulated time moves.
func (e *DefaultExchange) Idle() bool {
	if e.requestsInFlight.Load() != 0 || e.automInFlight.Load() != 0 {
		return false
	}
	e.mu.RLock()
	for _, gw := range e.Gateways {
		if !gw.Idle() {
			e.mu.RUnlock()
			return false
		}
	}
	e.mu.RUnlock()
	if e.deterministicPhases {
		e.phaseMu.Lock()
		defer e.phaseMu.Unlock()
		for _, job := range e.phaseJobs {
			if len(job.ticker.C()) > 0 {
				return false
			}
		}
	}
	return true
}

func (e *DefaultExchange) HandleClientRequests(gateway *ClientGateway) {
	if e.deterministicIngress {
		return
	}
	for req := range gateway.RequestCh {
		e.handleClientRequest(gateway, req)
	}
}

// DrainIngress processes queued requests in deterministic client-ID
// round-robin order. Within one client the gateway channel remains FIFO. It
// is active only when ExchangeConfig.DeterministicIngress is set.
func (e *DefaultExchange) DrainIngress() bool {
	if !e.deterministicIngress {
		return false
	}

	processed := false
	for {
		e.mu.RLock()
		clientIDs := make([]uint64, 0, len(e.Gateways))
		gateways := make(map[uint64]*ClientGateway, len(e.Gateways))
		for clientID, gateway := range e.Gateways {
			clientIDs = append(clientIDs, clientID)
			gateways[clientID] = gateway
		}
		e.mu.RUnlock()
		slices.Sort(clientIDs)

		passProcessed := false
		for _, clientID := range clientIDs {
			gateway := gateways[clientID]
			if gateway == nil {
				continue
			}
			select {
			case req, ok := <-gateway.RequestCh:
				if !ok {
					continue
				}
				e.handleClientRequest(gateway, req)
				processed = true
				passProcessed = true
			default:
			}
		}
		if !passProcessed {
			return processed
		}
	}
}

// DrainDeterministicEgress moves exchange response outboxes to gateway inboxes
// in client-ID order. It is paired with PumpDeterministicPhase; normal
// exchanges keep their asynchronous outbox deliverers.
func (e *DefaultExchange) DrainDeterministicEgress() bool {
	if !e.deterministicPhases {
		return false
	}
	e.mu.RLock()
	clientIDs := make([]uint64, 0, len(e.Gateways))
	gateways := make(map[uint64]*ClientGateway, len(e.Gateways))
	for clientID, gateway := range e.Gateways {
		clientIDs = append(clientIDs, clientID)
		gateways[clientID] = gateway
	}
	e.mu.RUnlock()
	slices.Sort(clientIDs)

	processed := false
	for _, clientID := range clientIDs {
		if gateways[clientID].DrainDeterministicEgress() {
			processed = true
		}
	}
	return processed
}

// handleClientRequest processes one request. Kept separate so the in-flight
// count is released by defer: an early return for a shut-down gateway would
// otherwise leak the count and leave a deterministic runner waiting forever
// for a system that has already settled.
func (e *DefaultExchange) handleClientRequest(gateway *ClientGateway, req Request) {
	e.requestsInFlight.Add(1)
	defer e.requestsInFlight.Add(-1)

	// Discard order and subscribe requests for shut-down gateways.
	// CancelOrder/Unsubscribe/QueryBalance are still processed so they
	// can clean up state, but new orders and subscriptions must never be
	// processed after gateway.IsRunning() returns false. Blocking ReqSubscribe
	// closes the race where a queued subscribe arrives after Unsubscribe was
	// called directly on MDPublisher during bootstrap shutdown.
	if req.Type == ReqPlaceOrder || req.Type == ReqSubscribe {
		if !gateway.IsRunning() {
			return
		}
	}

	permit, rejection, admitted := e.admitRequest(gateway.ClientID, req)
	if !admitted {
		gateway.enqueueResponse(rejection)
		return
	}
	defer e.releasePermit(permit)

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
		resp = e.Unsubscribe(gateway.ClientID, req.QueryReq, gateway)
	}

	// At-least-once delivery: a dropped accept/reject leaves the actor's
	// in-flight order state desynchronized forever. Routed through the
	// outbox so it shares one FIFO with the fills/cancels the request
	// generated.
	gateway.enqueueResponse(resp)
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
func (e *DefaultExchange) MidPrice(symbol string) (int64, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.MidPriceLocked(symbol)
}

// MidPriceLocked computes a strict midpoint while e.mu is already held by the
// caller. It implements price.LockedMidPriceProvider for the exchange's own
// locked index path; external callers must use MidPrice.
func (e *DefaultExchange) MidPriceLocked(symbol string) (int64, error) {
	book := e.Books[symbol]
	if book == nil {
		return 0, fmt.Errorf("mid price for %s: %w", symbol, ErrNoBookPrice)
	}
	price, err := book.GetMidPrice()
	if err != nil {
		return 0, fmt.Errorf("mid price for %s: %w", symbol, err)
	}
	return price, nil
}

// TwoSidedMidPrice returns a contemporaneous executable-book midpoint. Unlike
// MidPrice it never falls back to LastTrade when either side is absent, making
// it suitable for terminal mark-to-complete research metrics.
func (e *DefaultExchange) TwoSidedMidPrice(symbol string) (int64, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	book := e.Books[symbol]
	if book == nil {
		return 0, false
	}
	price, err := book.GetMidPrice()
	if err != nil {
		return 0, false
	}
	return price, true
}

func (e *DefaultExchange) ListInstruments(baseFilter, quoteFilter string) []Instrument {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Symbol order: callers discover contracts through this list and act on
	// them in the order it returns, so ranging the map would let a participant
	// quote a newly listed chain in a different order every run.
	symbols := make([]string, 0, len(e.Instruments))
	for symbol := range e.Instruments {
		symbols = append(symbols, symbol)
	}
	slices.Sort(symbols)

	result := make([]Instrument, 0, len(e.Instruments))
	for _, symbol := range symbols {
		inst := e.Instruments[symbol]
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
	e.automMu.Lock()
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		e.automMu.Unlock()
		return
	}
	e.closed = true

	if e.running {
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
	e.mu.Unlock()
	if e.deterministicPhases {
		e.stopDeterministicPhaseJobs(nil)
	}

	// Cancel before releasing automMu so StartAutomation cannot install a new
	// context after terminal shutdown. Wait outside locks because loop work may
	// be waiting for e.mu.
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
// It returns ErrNoBookPrice when the shared mark is unavailable rather than
// silently valuing positions at zero or their individual entry prices.
func (e *DefaultExchange) SettleFunding(perp *PerpFutures) error {
	if perp == nil {
		return fmt.Errorf("funding settlement: %w", ErrNoBookPrice)
	}
	e.mu.Lock()
	now := e.Clock.NowUnixNano()
	settled, settleErr := settleFunding(e.Positions, e.Clients, perp, now, buildFundingSink(e))
	settlementSnapshot := *perp.GetFundingRate()
	if settled {
		e.logFundingSettlementLocked(now, perp, settlementSnapshot)
	}
	e.mu.Unlock()
	if !settled {
		if settleErr == nil {
			settleErr = ErrNoBookPrice
		}
		err := fmt.Errorf("funding settlement for %s: %w", perp.Symbol(), settleErr)
		if errors.Is(settleErr, ErrNoBookPrice) {
			e.reportPriceUnavailable(now, perp.Symbol(), "funding_settlement", err)
		} else {
			e.reportFundingSettlementFailure(now, perp.Symbol(), settleErr)
		}
		return err
	}
	return nil
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
	e.indexFeedSymbols = slices.Clone(config.IndexFeedSymbols)
	e.indexFeedProvider = config.IndexFeedProvider
	if e.indexFeedProvider == nil {
		e.indexFeedProvider = config.IndexProvider
	}
	e.priceUpdateInterval = config.PriceUpdateInterval
	e.CollateralRate = config.CollateralRate
	e.LiquidationFeeBps = config.LiquidationFeeBps
	e.LiquidationHandler = config.LiquidationHandler
	e.listingPolicies = config.ListingPolicies
	e.preExpiryHook = config.PreExpiryHook
	e.postDerivativeMarkHook = config.PostDerivativeMarkHook
}

// StartAutomation begins automatic price updates, funding settlements, and collateral charging.
// Runs until ctx is cancelled or StopAutomation is called.
func (e *DefaultExchange) StartAutomation(ctx context.Context) {
	e.automMu.Lock()
	defer e.automMu.Unlock()

	if e.automCtx != nil {
		return
	}
	e.mu.RLock()
	closed := e.closed
	e.mu.RUnlock()
	if closed {
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
	// Allocate scheduler-backed tickers before their goroutines start. Their
	// event sequence is part of simulated time; lazy allocation inside each
	// goroutine makes equal-time automation order depend on Go scheduling.
	priceTicker := e.tickerFactory.NewTicker(e.priceUpdateInterval)
	fundingTicker := e.tickerFactory.NewTicker(time.Second)
	collateralTicker := e.tickerFactory.NewTicker(time.Minute)
	expiryTicker := e.tickerFactory.NewTicker(time.Second)
	if e.deterministicPhases {
		// The initial mark pass is part of setup, before the runner begins
		// processing actor work. Subsequent jobs are pumped in this exact
		// registration order at their scheduled timestamps.
		e.updateAllPerpPrices()
		e.addDeterministicPhaseJob(phaseJobAutomation, priceTicker, e.updateAllPerpPrices)
		e.addDeterministicPhaseJob(phaseJobAutomation, fundingTicker, e.CheckAndSettleFunding)
		e.addDeterministicPhaseJob(phaseJobAutomation, collateralTicker, e.ChargeCollateralInterest)
		e.addDeterministicPhaseJob(phaseJobAutomation, expiryTicker, func() {
			e.CheckListings()
			e.UpdateDerivativeMarks()
			e.CheckExpiries()
			e.CheckPositionMarginerLiquidations()
		})
		return
	}

	e.automWg.Add(1)
	go e.priceUpdateLoop(priceTicker)

	e.automWg.Add(1)
	go e.fundingSettlementLoop(fundingTicker)

	e.automWg.Add(1)
	go e.collateralChargeLoop(collateralTicker)

	e.automWg.Add(1)
	go e.expiryLoop(expiryTicker)
}

// StopAutomation stops all automatic operations and waits for completion.
func (e *DefaultExchange) StopAutomation() {
	e.automMu.Lock()
	if e.automCancel != nil {
		e.automCancel()
	}
	e.automMu.Unlock()

	e.automWg.Wait()
	if e.deterministicPhases {
		group := phaseJobAutomation
		e.stopDeterministicPhaseJobs(&group)
	}

	e.automMu.Lock()
	e.automCtx = nil
	e.automCancel = nil
	e.automMu.Unlock()
}

type tickerAcknowledger interface {
	Acknowledge()
}

// acknowledgeTicker is a no-op for production tickers. Scheduler-backed
// tickers use it to keep a deterministic runner from advancing during the
// interval between channel receive and callback completion.
func acknowledgeTicker(ticker Ticker) {
	if acknowledger, ok := ticker.(tickerAcknowledger); ok {
		acknowledger.Acknowledge()
	}
}

func (e *DefaultExchange) priceUpdateLoop(ticker Ticker) {
	defer e.automWg.Done()
	defer ticker.Stop()

	e.updateAllPerpPrices()

	for {
		select {
		case <-e.automCtx.Done():
			return
		case <-ticker.C():
			e.automInFlight.Add(1)
			e.updateAllPerpPrices()
			e.automInFlight.Add(-1)
			acknowledgeTicker(ticker)
		}
	}
}

func (e *DefaultExchange) fundingSettlementLoop(ticker Ticker) {
	defer e.automWg.Done()
	defer ticker.Stop()

	for {
		select {
		case <-e.automCtx.Done():
			return
		case <-ticker.C():
			e.automInFlight.Add(1)
			e.CheckAndSettleFunding()
			e.automInFlight.Add(-1)
			acknowledgeTicker(ticker)
		}
	}
}

func (e *DefaultExchange) collateralChargeLoop(ticker Ticker) {
	defer e.automWg.Done()
	defer ticker.Stop()

	for {
		select {
		case <-e.automCtx.Done():
			return
		case <-ticker.C():
			e.automInFlight.Add(1)
			e.ChargeCollateralInterest()
			e.automInFlight.Add(-1)
			acknowledgeTicker(ticker)
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
	return priceSourceFunc(func(symbol string) (int64, error) {
		price, err := e.indexPriceLocked(symbol)
		if err != nil {
			return 0, fmt.Errorf("index source %s: %w", symbol, err)
		}
		return price, nil
	})
}

// indexPriceLocked resolves a margined book's declared external reference.
// Caller must hold e.mu. It uses the explicit top-of-book reference policy
// (midpoint when two-sided, best quote when one-sided), preserving the prior
// index/mark contract without misnaming a one-sided quote as a midpoint.
func (e *DefaultExchange) indexPriceLocked(symbol string) (int64, error) {
	book := e.Books[symbol]
	if book == nil {
		return 0, fmt.Errorf("index for %s: %w", symbol, ErrNoBookPrice)
	}
	if underlying := underlyingOf(book.Instrument); underlying != "" {
		price, err := e.bookReferencePriceLocked(underlying)
		if err == nil {
			return price, nil
		}
		fallback, fallbackErr := e.configuredIndexPriceLocked(symbol)
		if fallbackErr == nil {
			return fallback, nil
		}
		return 0, fmt.Errorf("index for %s from underlying %s: %w", symbol, underlying, err)
	}
	price, err := e.configuredIndexPriceLocked(symbol)
	if err != nil {
		return 0, fmt.Errorf("index for %s: %w", symbol, err)
	}
	return price, nil
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
		book       *OrderBook
		perp       *PerpFutures
		markPrice  int64
		indexPrice int64
		hasIndex   bool
		isPerp     bool
	}
	type optionData struct {
		symbol           string
		book             *OrderBook
		option           *einstrument.EuropeanOption
		underlyingPrice  int64
		premium          int64
		updateErr        error
		skippedLifecycle bool
		ready            bool
	}
	type deferredPrice struct {
		symbol          string
		operation       string
		err             error
		perp            *PerpFutures
		isPerp          bool
		fundingSnapshot FundingRate
	}
	e.mu.RLock()
	// Symbol order: this sweep publishes marks and can call margin and
	// liquidation, so the order books are visited deterministically.
	markSymbols := make([]string, 0, len(e.Books))
	for symbol := range e.Books {
		markSymbols = append(markSymbols, symbol)
	}
	slices.Sort(markSymbols)
	candidates := make([]bookData, 0, len(e.Books))
	optionCandidates := make([]optionData, 0)
	deferred := make([]deferredPrice, 0)
	for _, symbol := range markSymbols {
		book := e.Books[symbol]
		if _, pending := e.settlementPending[symbol]; pending {
			// Expiry already halted this contract. Settlement sampling continues
			// in UpdateDerivativeMarks, but mark/funding/liquidation work must
			// not continue merely because the declared reference is delayed.
			continue
		}
		if option, ok := book.Instrument.(*einstrument.EuropeanOption); ok && timestamp < option.ExpiryNano() {
			optionCandidates = append(optionCandidates, optionData{symbol: symbol, book: book, option: option})
		}
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
		indexPrice := int64(0)
		hasIndex := false
		if underlyingOf(book.Instrument) != "" || e.indexProvider != nil {
			var err error
			indexPrice, err = e.indexPriceLocked(book.Symbol)
			if err != nil {
				// No declared underlying reference or configured external index:
				// defer this mark/funding/margin update rather than manufacture a
				// zero index.
				deferred = append(deferred, deferredPrice{symbol: book.Symbol, operation: "perp_index", err: err, perp: perp, isPerp: isPerp})
				continue
			}
			hasIndex = true
		}
		markPrice, err := calc.Calculate(book)
		if err != nil {
			deferred = append(deferred, deferredPrice{symbol: book.Symbol, operation: "perp_mark", err: err, perp: perp, isPerp: isPerp})
			continue
		}
		candidates = append(candidates, bookData{
			symbol: book.Symbol, book: book,
			perp:       perp,
			markPrice:  markPrice,
			indexPrice: indexPrice,
			hasIndex:   hasIndex,
			isPerp:     isPerp,
		})
	}
	e.mu.RUnlock()
	for index := range optionCandidates {
		candidate := &optionCandidates[index]
		candidate.underlyingPrice, candidate.updateErr = e.derivativeUnderlyingPrice(candidate.option)
		if candidate.updateErr != nil {
			continue
		}
		yearsLeft := float64(candidate.option.ExpiryNano()-timestamp) / float64(365*24*time.Hour)
		candidate.premium = eprice.Black76Premium(candidate.underlyingPrice, candidate.option.Strike, candidate.option.IV, yearsLeft, candidate.option.IsCall)
	}

	// Symbol order, not map order: all successful marks and all availability
	// clears are committed in one critical section before any risk sweep. A
	// cross-margined portfolio must be valued against one coherent mark set;
	// actors must not observe a mixture of this tick's marks and last tick's
	// marks while the batch is being installed.
	slices.SortFunc(candidates, func(a, b bookData) int { return cmp.Compare(a.symbol, b.symbol) })

	type perpUpdate struct {
		symbol          string
		perp            *PerpFutures
		markPrice       int64
		indexPrice      int64
		isPerp          bool
		ready           bool
		skippedPending  bool
		updateErr       error
		fundingSnapshot FundingRate
	}
	updates := make([]perpUpdate, 0, len(candidates))
	for _, c := range candidates {
		indexPrice := c.indexPrice
		if !c.hasIndex {
			// Genuine single-venue configuration: the perp's own book is the
			// only declared price source. This is distinct from a missing index:
			// the missing case was already recorded as a deferral above.
			indexPrice = c.markPrice
		}
		updates = append(updates, perpUpdate{
			symbol:     c.symbol,
			perp:       c.perp,
			markPrice:  c.markPrice,
			indexPrice: indexPrice,
			isPerp:     c.isPerp,
		})
	}

	e.mu.Lock()
	for i := range deferred {
		d := &deferred[i]
		// A previously valid mark is not silently retained as the current mark
		// after its declared source becomes unusable. MarkAvailable is the
		// availability contract; the numeric fields remain diagnostics only
		// when it is false.
		d.perp.ClearMarkReferences()
		d.fundingSnapshot = *d.perp.GetFundingRate()
	}
	for i := range updates {
		u := &updates[i]
		if _, pending := e.settlementPending[u.symbol]; pending ||
			e.Books[u.symbol] != candidates[i].book {
			// Expiry can win the lock after price collection but before this
			// commit. Do not resurrect a halted contract's mark in that race.
			u.skippedPending = true
			u.fundingSnapshot = *u.perp.GetFundingRate()
			continue
		}
		if u.isPerp {
			u.updateErr = u.perp.UpdateFundingRate(u.indexPrice, u.markPrice)
		} else {
			u.perp.UpdateMarkReferences(u.indexPrice, u.markPrice)
		}
		u.fundingSnapshot = *u.perp.GetFundingRate()
		u.ready = u.updateErr == nil
	}
	for index := range optionCandidates {
		candidate := &optionCandidates[index]
		if candidate.updateErr != nil {
			liveBook := e.Books[candidate.symbol]
			if liveBook == candidate.book && e.Instruments[candidate.symbol] == candidate.option {
				// Once the declared underlying is unavailable, the previous
				// premium cannot remain a valid risk mark. Clear it while the
				// candidate identity is protected by the exchange lock.
				candidate.option.ClearMarks()
			}
			continue
		}
		liveBook := e.Books[candidate.symbol]
		if _, pending := e.settlementPending[candidate.symbol]; pending ||
			liveBook == nil || liveBook != candidate.book ||
			e.Instruments[candidate.symbol] != candidate.option ||
			timestamp >= candidate.option.ExpiryNano() {
			candidate.skippedLifecycle = true
			continue
		}
		candidate.option.SetMarks(candidate.underlyingPrice, candidate.premium)
		candidate.ready = true
	}
	e.mu.Unlock()
	for _, d := range deferred {
		e.reportPriceUnavailable(timestamp, d.symbol, d.operation, d.err)
		if d.isPerp {
			e.MDPublisher.PublishFunding(d.symbol, &d.fundingSnapshot, timestamp)
		}
	}
	for _, u := range updates {
		if u.updateErr != nil && !u.skippedPending {
			e.reportPriceUnavailable(timestamp, u.symbol, "perp_funding", u.updateErr)
			if u.isPerp {
				e.MDPublisher.PublishFunding(u.symbol, &u.fundingSnapshot, timestamp)
			}
			continue
		}
	}
	for _, candidate := range optionCandidates {
		if candidate.updateErr != nil && !candidate.skippedLifecycle {
			e.reportPriceUnavailable(timestamp, candidate.symbol, "derivative_mark", candidate.updateErr)
			continue
		}
		if !candidate.ready {
			continue
		}
		if log := e.getLogger(candidate.symbol); log != nil {
			log.LogEvent(timestamp, 0, "mark_price_update", MarkPriceUpdateEvent{
				Timestamp: timestamp, Symbol: candidate.symbol,
				MarkPrice: candidate.premium, IndexPrice: candidate.underlyingPrice,
			})
		}
	}

	// Publish the completed mark set in canonical symbol order. Publication is
	// observability only; it cannot interleave with a risk decision below.
	for _, u := range updates {
		if !u.ready {
			continue
		}
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
			e.MDPublisher.PublishFunding(u.symbol, &u.fundingSnapshot, timestamp)
			if log := e.getLogger(u.symbol); log != nil {
				log.LogEvent(timestamp, 0, "funding_rate_update", FundingRateUpdateEvent{
					Timestamp:   timestamp,
					Symbol:      u.symbol,
					Rate:        u.fundingSnapshot.Rate,
					NextFunding: u.fundingSnapshot.NextFunding,
					Interval:    u.fundingSnapshot.Interval,
				})
			}
		}
	}

	// Only now evaluate risk, after every successful candidate has published
	// its mark into the shared portfolio state.
	for _, u := range updates {
		if u.ready {
			e.CheckLiquidations(u.symbol, u.perp, u.markPrice)
		}
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

// positionUPnL returns unrealized PnL for a position marked at markPrice.
func positionUPnL(pos *Position, markPrice, precision int64) int64 {
	return etypes.PriceChangeMulDiv(pos.Size, markPrice, pos.EntryPrice, precision)
}

// accountMarginProfile aggregates a client's cross-margin exposure in the
// quote asset across every margined book: equity contribution, notional, and
// maintenance/warning requirements. Futures-style positions contribute
// entry-to-mark PnL; cash-premium options contribute signed marked value
// because their entry premium already moved cash. The triggering symbol is marked at
// triggerMark; other books use their latest explicitly available stored mark,
// then the declared one-sided book-reference policy. No position-entry or
// zero-price substitute exists when both are unavailable. Caller must hold
// e.mu.
type accountMarginProfile struct {
	EquityContribution int64
	Notional           int64
	Maintenance        int64
	Warning            int64
}

func (e *DefaultExchange) buildAccountMarginProfile(clientID uint64, quote, triggerSymbol string, triggerMark int64) (accountMarginProfile, error) {
	var p accountMarginProfile
	// Cross-margin marks can fail on the first unmarked book and emit the
	// reason into the execution evidence.  A map walk here therefore made the
	// ordered execution digest depend on the process's map hash seed (the
	// failure surfaced only once the option chain had grown enough to invoke
	// this path).  Keep the same economic aggregation, but make the diagnostic
	// and any subsequent work visit books in canonical symbol order.
	symbols := make([]string, 0, len(e.Books))
	for symbol := range e.Books {
		symbols = append(symbols, symbol)
	}
	slices.Sort(symbols)
	for _, symbol := range symbols {
		book := e.Books[symbol]
		if _, pending := e.settlementPending[symbol]; pending {
			for _, side := range []PositionSide{PositionBoth, PositionLong, PositionShort} {
				position := e.Positions.GetPositionBySide(clientID, symbol, side)
				if position != nil && position.Size != 0 {
					// Retained pending exposure is not an economic zero. No
					// valid mark exists, so fail the whole account profile closed
					// instead of allowing active sibling risk to ignore it.
					return accountMarginProfile{}, fmt.Errorf("cross-margin exposure for %s is settlement-pending", symbol)
				}
			}
			continue
		}
		perp := marginCore(book.Instrument)
		if perp == nil {
			// Non-perp margined instruments (options) contribute their own
			// mark-to-market and maintenance; skipping them makes a short-vol
			// account invisible to the risk engine and unliquidatable.
			if pm, ok := book.Instrument.(PositionMarginer); ok && book.Instrument.QuoteAsset() == quote {
				if err := e.addPositionMarginerExposure(&p, clientID, symbol, book.Instrument, pm, book); err != nil {
					return accountMarginProfile{}, err
				}
			}
			continue
		}
		if perp.QuoteAsset() != quote {
			continue
		}
		positions := make([]*Position, 0, 3)
		for _, side := range []PositionSide{PositionBoth, PositionLong, PositionShort} {
			pos := e.Positions.GetPositionBySide(clientID, symbol, side)
			if pos != nil && pos.Size != 0 {
				positions = append(positions, pos)
			}
		}
		if len(positions) == 0 {
			continue
		}
		mark := triggerMark
		if symbol != triggerSymbol {
			fundingRate := perp.GetFundingRate()
			mark = fundingRate.MarkPrice
			if !fundingRate.MarkAvailable {
				var err error
				mark, err = liveBookReferencePrice(book)
				if err != nil {
					return accountMarginProfile{}, fmt.Errorf("cross-margin mark for %s: %w", symbol, err)
				}
			}
		}
		precision := perp.BasePrecision()
		for _, pos := range positions {
			p.EquityContribution += e.positionUPnL(pos, mark, precision)
			notional := etypes.AbsMulDiv(pos.Size, mark, precision)
			p.Notional += notional
			p.Maintenance += notional * perp.MaintenanceMarginRate / 10000
			p.Warning += notional * perp.WarningMarginRate / 10000
		}
	}
	return p, nil
}

// clientHasSettlementPendingExposureLocked is the account-wide lifecycle
// boundary for pending settlement. Caller must hold e.mu.Lock().
func (e *DefaultExchange) clientHasSettlementPendingExposureLocked(clientID uint64) bool {
	for symbol := range e.settlementPending {
		for _, side := range []PositionSide{PositionBoth, PositionLong, PositionShort} {
			position := e.Positions.GetPositionBySide(clientID, symbol, side)
			if position != nil && position.Size != 0 {
				return true
			}
		}
	}
	return false
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
		if _, pending := e.settlementPending[symbol]; !pending {
			if _, ok := book.Instrument.(PositionMarginer); ok {
				symbols = append(symbols, symbol)
			}
		}
	}
	slices.Sort(symbols)

	clientIDs := make([]uint64, 0, len(e.Clients))
	for clientID := range e.Clients {
		clientIDs = append(clientIDs, clientID)
	}
	slices.Sort(clientIDs)
	type profileKey struct {
		clientID uint64
		quote    string
	}
	// Marks are fixed for this sweep. A solvent account's cross-book profile is
	// therefore identical for every option symbol it holds; rebuilding it once
	// per symbol turns an expanding option chain into quadratic work. A
	// liquidation mutates balances/positions, so invalidate only that account
	// and quote before the next symbol checks it.
	profiles := make(map[profileKey]accountMarginProfile)

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

			key := profileKey{clientID: clientID, quote: quote}
			profile, ok := profiles[key]
			if !ok {
				// No trigger symbol: every book contributes its stored mark.
				var err error
				profile, err = e.buildAccountMarginProfile(clientID, quote, "", 0)
				if err != nil {
					e.reportPriceUnavailable(timestamp, symbol, "option_liquidation", err)
					continue
				}
				profiles[key] = profile
			}
			equity := client.PerpBalance(quote) - client.BorrowedPerpPortion(quote) + profile.EquityContribution
			if equity >= profile.Maintenance {
				continue
			}
			for _, pos := range positions {
				e.liquidate(clientID, client, symbol, pos, inst, timestamp)
			}
			delete(profiles, key)
		}
	}
}

// addPositionMarginerExposure folds a PositionMarginer instrument's open
// positions into the cross-margin profile: marked at the instrument's own
// mark, maintenance per its own formula. Warning reuses maintenance — these
// instruments carry no separate warning tier.
func (e *DefaultExchange) addPositionMarginerExposure(p *accountMarginProfile, clientID uint64, symbol string, inst Instrument, pm PositionMarginer, book *OrderBook) error {
	precision := inst.BasePrecision()
	for _, side := range []PositionSide{PositionBoth, PositionLong, PositionShort} {
		pos := e.Positions.GetPositionBySide(clientID, symbol, side)
		if pos == nil || pos.Size == 0 {
			continue
		}
		m, err := riskMark(inst, book)
		if err != nil {
			return fmt.Errorf("position margin mark for %s: %w", symbol, err)
		}
		// Option premiums have already moved through the perp wallet at each
		// fill. Their contribution to equity is therefore the signed current
		// premium value, unlike futures-style entry-to-mark PnL.
		p.EquityContribution += MulDiv(pos.Size, m, precision)
		p.Notional += etypes.AbsMulDiv(pos.Size, m, precision)
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
	return nil
}

// CheckLiquidations evaluates all positions for a symbol after a mark price update.
// Cross-margin account model: equity = perp balance − borrowed quote debt +
// equity contribution across EVERY margined book in the same quote asset (order
// margin is locked, not lost); the maintenance requirement likewise sums over
// all books, so the same cash can never back two symbols at once. On breach the triggering symbol's
// positions are closed; other symbols resolve on their own mark updates.
// Hedge-mode Long/Short positions are included.
func (e *DefaultExchange) CheckLiquidations(symbol string, perp *PerpFutures, markPrice int64) {
	quote := perp.QuoteAsset()

	e.mu.Lock()
	defer e.mu.Unlock()
	if _, pending := e.settlementPending[symbol]; pending {
		return
	}

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

		profile, err := e.buildAccountMarginProfile(clientID, quote, symbol, markPrice)
		if err != nil {
			e.reportPriceUnavailable(e.Clock.NowUnixNano(), symbol, "liquidation", err)
			continue
		}
		equityContribution, notional := profile.EquityContribution, profile.Notional
		// Borrowed quote is cash in the wallet but a matching liability: counting
		// it as equity would let a loan mask an undercollateralized account and
		// dodge liquidation. Net only the perp-attributed share — a spot-credited
		// loan's cash never entered this wallet, so charging it here would
		// liquidate a solvent account.
		equity := client.PerpBalance(quote) - client.BorrowedPerpPortion(quote) + equityContribution
		maintenanceMargin := profile.Maintenance
		warningMargin := profile.Warning

		timestamp := e.Clock.NowUnixNano()

		if equity < maintenanceMargin {
			if log := e.getLogger("_global"); log != nil {
				log.LogEvent(timestamp, clientID, "liquidation_check", map[string]any{
					"timestamp":                      timestamp,
					"client_id":                      clientID,
					"symbol":                         symbol,
					"mark_price":                     markPrice,
					"balance":                        client.PerpBalances[quote],
					"reserved":                       client.PerpReserved[quote],
					"derivative_equity_contribution": equityContribution,
					"equity":                         equity,
					"notional":                       notional,
					"maintenance_margin":             maintenanceMargin,
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
			liqPrice, err := e.EstimateLiquidationPrice(positions[0], clientID, perp, perp.BasePrecision())
			if err != nil {
				e.reportPriceUnavailable(timestamp, symbol, "liquidation_price_estimate", err)
				continue
			}
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
// position's equity would hit zero (maintenance requirement ignored). A
// computed zero is a legitimate estimate for a sufficiently collateralized
// long; an absent position/client/contract/precision is therefore an error,
// never a zero sentinel.
func (e *DefaultExchange) EstimateLiquidationPrice(pos *Position, clientID uint64, perp *PerpFutures, precision int64) (int64, error) {
	if pos == nil || pos.Size == 0 {
		return 0, fmt.Errorf("liquidation-price position: %w", ErrNoBookPrice)
	}
	if perp == nil || precision <= 0 {
		return 0, fmt.Errorf("liquidation-price contract: %w", ErrNoBookPrice)
	}
	client := e.Clients[clientID]
	if client == nil {
		return 0, fmt.Errorf("liquidation-price client %d: %w", clientID, ErrNoBookPrice)
	}
	// Net perp-attributed debt out of the collateral: the loan is a liability,
	// so the price at which equity hits zero is reached sooner, not later.
	balance := client.PerpBalance(perp.QuoteAsset()) - client.BorrowedPerpPortion(perp.QuoteAsset())
	if accounting, ok := e.Positions.(etypes.ExactLinearPositionStore); ok {
		if liquidationPrice, valid := accounting.PositionLiquidationPrice(*pos, balance, precision); valid {
			return liquidationPrice, nil
		}
		if e.requireExactLinearAccounting {
			return 0, fmt.Errorf("liquidation-price exact accounting: %w", ErrNoBookPrice)
		}
	}
	if pos.Size > 0 {
		return pos.EntryPrice - MulDiv(balance, precision, pos.Size), nil
	}
	return pos.EntryPrice + MulDiv(balance, precision, -pos.Size), nil
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
	fillPrice, filledQty, filled := e.forceClose(clientID, client, book, book.Instrument, closeSide, pos.PositionSide, abs(pos.Size), timestamp)
	if !filled {
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
				client.BorrowedSpot[inst.QuoteAsset()] = min(
					client.BorrowedSpot[inst.QuoteAsset()],
					client.Borrowed[inst.QuoteAsset()],
				)
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
		e.moveVenueBalance(VenueInsuranceFund, quote, -debt, timestamp, symbol, "liquidation_deficit")

		logBalanceChange(e, timestamp, clientID, symbol, "liquidation_deficit", []BalanceDelta{
			perpDelta(quote, balance, 0),
		})
	}

	// A liquidation and any insurance-fund movement it causes must be visible in
	// the event log, not only to a handler the run may not have installed. The
	// logs are the observational surface for market behaviour after the fact.
	if log := e.getLogger(symbol); log != nil {
		log.LogEvent(timestamp, clientID, "liquidation", map[string]any{
			"symbol":         symbol,
			"position_size":  pos.Size,
			"fill_price":     fillPrice,
			"remaining_debt": debt,
		})
	}
	if debt > 0 {
		if log := e.getLogger("_global"); log != nil {
			log.LogEvent(timestamp, clientID, "insurance_fund", map[string]any{
				"symbol":  symbol,
				"asset":   quote,
				"delta":   -debt,
				"balance": e.ExchangeBalance.InsuranceFund[quote],
				"reason":  "liquidation_deficit",
			})
		}
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
	// A liquidation fee is a non-negative service/risk charge. It is based on
	// exposure magnitude, not signed futures cash-flow direction; a negative
	// price must not turn it into a rebate or numeric no-price sentinel.
	fee := etypes.AbsMulDiv(closedSize, fillPrice, inst.BasePrecision()) * e.LiquidationFeeBps / 10000
	if available := client.PerpAvailable(quote); fee > available {
		fee = available
	}
	if fee <= 0 {
		return
	}

	oldBalance := client.PerpBalances[quote]
	client.PerpBalances[quote] -= fee
	e.moveVenueBalance(VenueInsuranceFund, quote, fee, timestamp, symbol, "liquidation_clearance_fee")

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
	for symbol, inst := range e.Instruments {
		if _, pending := e.settlementPending[symbol]; pending {
			continue
		}
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
			nextFunding, ok := nextFundingTimestamp(now, fundingRate.Interval)
			if !ok {
				e.mu.Unlock()
				e.reportFundingSettlementFailure(now, perp.Symbol(), ErrFundingArithmetic)
				continue
			}
			fundingRate.NextFunding = nextFunding
			e.mu.Unlock()
			continue
		}
		if now < fundingRate.NextFunding {
			e.mu.Unlock()
			continue
		}
		settlementTimestamp := e.Clock.NowUnixNano()
		settled, settleErr := settleFunding(e.Positions, e.Clients, perp, settlementTimestamp, buildFundingSink(e))
		if !settled {
			e.mu.Unlock()
			if settleErr == nil {
				settleErr = ErrNoBookPrice
			}
			if errors.Is(settleErr, ErrNoBookPrice) {
				e.reportPriceUnavailable(now, perp.Symbol(), "funding_settlement", fmt.Errorf("funding mark for %s: %w", perp.Symbol(), settleErr))
			} else {
				e.reportFundingSettlementFailure(now, perp.Symbol(), settleErr)
			}
			continue
		}
		fundingSnapshot := *fundingRate
		e.logFundingSettlementLocked(settlementTimestamp, perp, fundingSnapshot)
		e.mu.Unlock()
		e.MDPublisher.PublishFunding(perp.Symbol(), &fundingSnapshot, now)
	}
}

// logFundingSettlementLocked records the settlement marker while the same
// exchange lock still protects the balance mutation and schedule advance.
// The marker is part of the causal evidence contract, not an asynchronous
// after-the-fact notification. Caller must hold e.mu.Lock().
func (e *DefaultExchange) logFundingSettlementLocked(timestamp int64, perp *PerpFutures, funding FundingRate) {
	if perp == nil {
		return
	}
	log := e.getLogger(perp.Symbol())
	if log == nil {
		log = e.getLogger("_global")
	}
	if log == nil {
		return
	}
	log.LogEvent(timestamp, 0, "funding_settlement", FundingSettlementEvent{
		Timestamp: timestamp, Symbol: perp.Symbol(), Rate: funding.Rate,
		NextFunding: funding.NextFunding, Interval: funding.Interval,
		MarkPrice: funding.MarkPrice, BasePrecision: perp.BasePrecision(),
	})
}

func (e *DefaultExchange) reportFundingSettlementFailure(now int64, symbol string, err error) {
	if err == nil {
		return
	}
	log := e.getLogger(symbol)
	if log == nil {
		log = e.getLogger("_global")
	}
	if log != nil {
		log.LogEvent(now, 0, "funding_settlement_failed", map[string]any{
			"timestamp": now,
			"symbol":    symbol,
			"reason":    err.Error(),
		})
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
	if client != nil && e.clientHasSettlementPendingExposureLocked(clientID) {
		return ErrSettlementPendingExposure
	}
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
