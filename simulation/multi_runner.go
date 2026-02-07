package simulation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/logger"
)

// MultiExchangeRunner manages a multi-exchange simulation
type MultiExchangeRunner struct {
	exchanges   map[VenueID]*exchange.Exchange
	actors      []actor.Actor
	logFiles    []*os.File
	loggers     map[string]*logger.Logger // "venue:symbol" -> logger
	config      MultiSimConfig
	clock       Clock
	mu          sync.Mutex
}

// NewMultiExchangeRunner creates a new multi-exchange simulation runner
func NewMultiExchangeRunner(config MultiSimConfig) (*MultiExchangeRunner, error) {
	runner := &MultiExchangeRunner{
		exchanges: make(map[VenueID]*exchange.Exchange),
		actors:    make([]actor.Actor, 0),
		logFiles:  make([]*os.File, 0),
		loggers:   make(map[string]*logger.Logger),
		config:    config,
		clock:     &RealClock{},
	}

	// Create exchanges
	for _, exConfig := range config.Exchanges {
		ex := exchange.NewExchange(100, runner.clock)
		runner.exchanges[VenueID(exConfig.Name)] = ex
	}

	// Generate symbol sets with overlap
	assetSets := GenerateSymbolsWithOverlap(
		config.GlobalAssets,
		len(config.Exchanges),
		config.OverlapRatio,
	)

	bootstrapPrices := GetBootstrapPrices()

	// Calculate total actors across all exchanges to divide balances
	totalActors := 0
	for i := range config.Exchanges {
		assets := assetSets[i]
		instruments := GenerateInstruments(assets, config.QuoteAsset, config.SpotToFuturesRatio)
		numSymbols := len(instruments)

		// LPs and MMs are per exchange, Takers are per symbol
		actorsForExchange := config.LPsPerSymbol + config.MMsPerSymbol + (numSymbols * config.TakersPerSymbol)
		totalActors += actorsForExchange
	}

	// Divide initial balances among all actors
	dividedBalances := make(map[string]int64)
	for asset, amount := range config.InitialBalances {
		dividedBalances[asset] = amount / int64(totalActors)
	}

	// Setup each exchange
	for i, exConfig := range config.Exchanges {
		venueID := VenueID(exConfig.Name)
		ex := runner.exchanges[venueID]
		assets := assetSets[i]

		// Generate instruments for this exchange
		instruments := GenerateInstruments(assets, config.QuoteAsset, config.SpotToFuturesRatio)

		// Create log directories
		spotDir := filepath.Join(config.LogDir, exConfig.Name, "spot")
		perpDir := filepath.Join(config.LogDir, exConfig.Name, "perp")
		if err := os.MkdirAll(spotDir, 0755); err != nil {
			runner.Close()
			return nil, fmt.Errorf("create spot log dir: %w", err)
		}
		if err := os.MkdirAll(perpDir, 0755); err != nil {
			runner.Close()
			return nil, fmt.Errorf("create perp log dir: %w", err)
		}

		// Add instruments and setup logging
		for symbol, inst := range instruments {
			ex.AddInstrument(inst)

			// Determine log directory based on instrument type
			var logDir string
			if _, ok := inst.(*exchange.SpotInstrument); ok {
				logDir = spotDir
			} else {
				logDir = perpDir
			}

			// Create log file
			logPath := filepath.Join(logDir, symbol+".log")
			logFile, err := os.Create(logPath)
			if err != nil {
				runner.Close()
				return nil, fmt.Errorf("create log file %s: %w", logPath, err)
			}
			runner.logFiles = append(runner.logFiles, logFile)

			lg := logger.New(logFile)
			runner.loggers[exConfig.Name+":"+symbol] = lg
			ex.SetLogger(symbol, lg)
		}

		// Create actors for this exchange
		if err := runner.createActorsForExchange(venueID, ex, instruments, bootstrapPrices, dividedBalances); err != nil {
			runner.Close()
			return nil, err
		}
	}

	return runner, nil
}

func (r *MultiExchangeRunner) createActorsForExchange(
	venueID VenueID,
	ex *exchange.Exchange,
	instruments map[string]exchange.Instrument,
	bootstrapPrices map[string]int64,
	dividedBalances map[string]int64,
) error {
	symbols := make([]string, 0, len(instruments))
	for symbol := range instruments {
		symbols = append(symbols, symbol)
	}

	var actorID uint64 = 1

	feePlan := &exchange.PercentageFee{
		MakerBps: 2,
		TakerBps: 5,
		InQuote:  true,
	}

	// Create composite LP actors (one per exchange, managing all symbols)
	lpConfig := actor.MultiSymbolLPConfig{
		Symbols:           symbols,
		Instruments:       instruments,
		SpreadBps:         r.config.LPSpreadBps,
		BootstrapPrices:   bootstrapPrices,
		LiquidityMultiple: 10,
		MinExitSize:       50 * exchange.SATOSHI, // 0.5 BTC minimum position before considering exit
	}
	if r.config.SimSpeedup > 0 {
		lpConfig.MonitorInterval = time.Duration(float64(100*time.Millisecond) / r.config.SimSpeedup)
	} else {
		lpConfig.MonitorInterval = 100 * time.Millisecond
	}

	for i := 0; i < r.config.LPsPerSymbol; i++ {
		gateway := ex.ConnectClient(actorID, dividedBalances, feePlan)

		lp := actor.NewMultiSymbolLP(actorID, gateway, lpConfig)

		// Set initial balances
		baseBalances := make(map[string]int64)
		for asset, balance := range dividedBalances {
			if asset != r.config.QuoteAsset {
				baseBalances[asset] = balance
			}
		}
		lp.SetBalances(baseBalances, dividedBalances[r.config.QuoteAsset])

		r.actors = append(r.actors, lp)
		actorID++
	}

	// Create composite MM actors with varying spreads
	// Multiple MMs with different spreads enable price discovery and depth
	mmSpreads := []int64{5, 10, 20, 30} // Tight to wide spreads (bps)

	for i := 0; i < r.config.MMsPerSymbol; i++ {
		spread := mmSpreads[i%len(mmSpreads)]

		mmConfig := actor.MultiSymbolMMConfig{
			Symbols:          symbols,
			Instruments:      instruments,
			SpreadBps:        spread,
			QuoteSize:        exchange.BTCAmount(0.05), // Reduced to 0.05 BTC to fit limited balances
			MaxInventory:     exchange.BTCAmount(1),    // Reduced to 1 BTC
			RequoteThreshold: 5,
		}

		if r.config.SimSpeedup > 0 {
			mmConfig.MonitorInterval = time.Duration(float64(50*time.Millisecond) / r.config.SimSpeedup)
		} else {
			mmConfig.MonitorInterval = 50 * time.Millisecond
		}

		gateway := ex.ConnectClient(actorID, dividedBalances, feePlan)
		mm := actor.NewMultiSymbolMM(actorID, gateway, mmConfig)

		r.actors = append(r.actors, mm)
		actorID++
	}

	// Create random takers
	takerInterval := r.config.TakerInterval
	if r.config.SimSpeedup > 0 {
		takerInterval = time.Duration(float64(takerInterval) / r.config.SimSpeedup)
	}

	// Find configuration for this exchange to get latency
	var exConfig ExchangeConfig
	for _, cfg := range r.config.Exchanges {
		if VenueID(cfg.Name) == venueID {
			exConfig = cfg
			break
		}
	}

	latency := time.Duration(exConfig.LatencyMs) * time.Millisecond
	if r.config.SimSpeedup > 0 {
		latency = time.Duration(float64(latency) / r.config.SimSpeedup)
	}

	// Configure delayed gateway
	latencyProvider := NewConstantLatency(latency)
	mktLatencyConfig := LatencyConfig{
		MarketDataLatency: latencyProvider,
		Mode:              LatencyMarketData,
	}
	orderLatencyConfig := LatencyConfig{
		RequestLatency:  latencyProvider,
		ResponseLatency: latencyProvider,
		Mode:            LatencyRequest | LatencyResponse,
	}

	for symbol := range instruments {
		for i := 0; i < r.config.TakersPerSymbol; i++ {
			baseGateway := ex.ConnectClient(actorID, dividedBalances, feePlan)

			delayedMktData := NewDelayedGateway(baseGateway, mktLatencyConfig)
			delayedOrderEntry := NewDelayedGateway(baseGateway, orderLatencyConfig)
			delayedMktData.Start()
			delayedOrderEntry.Start()

			// Combine into a usable ClientGateway
			gw := &exchange.ClientGateway{
				ClientID:   actorID,
				RequestCh:  delayedOrderEntry.ToClientGateway().RequestCh,
				ResponseCh: delayedOrderEntry.ToClientGateway().ResponseCh,
				MarketData: delayedMktData.ToClientGateway().MarketData,
				Running:    true,
			}

			taker := actor.NewRandomizedTaker(actorID, gw, actor.RandomizedTakerConfig{
				Symbol:   symbol,
				Interval: takerInterval,
				MinQty:   exchange.BTCAmount(0.01),
				MaxQty:   exchange.BTCAmount(0.1),
			})

			r.actors = append(r.actors, taker)
			actorID++
		}
	}

	return nil
}


// Run starts the simulation
func (r *MultiExchangeRunner) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start all actors
	for _, a := range r.actors {
		if err := a.Start(ctx); err != nil {
			return fmt.Errorf("start actor %d: %w", a.ID(), err)
		}
	}

	// Wait for duration or context cancellation
	if r.config.Duration > 0 {
		timer := time.NewTimer(r.config.Duration)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
		}
	} else {
		<-ctx.Done()
	}

	// Stop all actors
	for _, a := range r.actors {
		a.Stop()
	}

	// Shutdown exchanges
	for _, ex := range r.exchanges {
		ex.Shutdown()
	}

	return nil
}

// Close cleans up resources
func (r *MultiExchangeRunner) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, f := range r.logFiles {
		f.Close()
	}
	r.logFiles = nil

	return nil
}

// GetExchange returns an exchange by venue ID
func (r *MultiExchangeRunner) GetExchange(venue VenueID) *exchange.Exchange {
	return r.exchanges[venue]
}

// ActorCount returns the total number of actors
func (r *MultiExchangeRunner) ActorCount() int {
	return len(r.actors)
}
