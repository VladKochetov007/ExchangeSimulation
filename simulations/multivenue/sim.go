package multivenue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/instrument"
	eprice "exchange_sim/price"
	"exchange_sim/simulation"
	"exchange_sim/simulations/derivsim"
	"exchange_sim/simulations/feesim"
	etypes "exchange_sim/types"
)

const (
	mvBasePrecision  = exchange.BTC_PRECISION
	mvQuotePrecision = exchange.USD_PRECISION
	mvBootstrapPrice = 50_000 * mvQuotePrecision
)

// Config creates exactly three separately funded direct venues on one
// deterministic simulated clock. The one-second default is intentional: all
// configured actor and venue timers are at least one second, making hour/day
// experiments feasible without changing their event semantics through tick
// coalescing.
type Config struct {
	LogDir string `json:"log_dir"`
	// LogMode controls raw venue-event persistence. "full" is the default
	// evidence mode; "none" retains deterministic in-memory risk telemetry and
	// greeks.json while avoiding large JSONL output for replicated treatments.
	LogMode  string   `json:"log_mode"`
	Seed     int64    `json:"seed"`
	VenueIDs []string `json:"venue_ids"`

	Step               time.Duration `json:"step"`
	SnapshotInterval   time.Duration `json:"snapshot_interval"`
	AutomationInterval time.Duration `json:"automation_interval"`
	QuoteInterval      time.Duration `json:"quote_interval"`
	NoiseInterval      time.Duration `json:"noise_interval"`
	GreekInterval      time.Duration `json:"greek_interval"`

	ShortOptionTenor          time.Duration `json:"short_option_tenor"`
	LongOptionTenor           time.Duration `json:"long_option_tenor"`
	ShortFutureTenor          time.Duration `json:"short_future_tenor"`
	LongFutureTenor           time.Duration `json:"long_future_tenor"`
	OptionIV                  float64       `json:"option_iv"`
	StrikesPerSide            int           `json:"strikes_per_side"`
	StrikeStepUSD             int64         `json:"strike_step_usd"`
	OptionMaxStrikesPerExpiry int           `json:"option_max_strikes_per_expiry"`

	StoikovRiskAversion       float64       `json:"stoikov_risk_aversion"`
	StoikovFillDecay          float64       `json:"stoikov_fill_decay"`
	StoikovVariancePerSecond  float64       `json:"stoikov_variance_per_second"`
	StoikovInventoryHorizon   time.Duration `json:"stoikov_inventory_horizon"`
	StoikovVolatilityHalfLife time.Duration `json:"stoikov_volatility_half_life"`

	OptionBuyProbability float64 `json:"option_buy_probability"`

	// DealerHedgeMode selects the option dealer treatment arm: "on" uses the
	// stateful spot hedge policy, "off" leaves filled-option delta unhedged.
	// It is explicit rather than a bool so an omitted JSON field keeps the
	// historical hedged baseline while a false-like zero value is not confused
	// with an experiment arm.
	DealerHedgeMode string `json:"dealer_hedge_mode"`
}

func (c *Config) normalize() error {
	if c.LogDir == "" {
		return errors.New("multivenue: LogDir is required")
	}
	if c.Seed == 0 {
		c.Seed = 42
	}
	if c.LogMode == "" {
		c.LogMode = "full"
	}
	if c.LogMode != "full" && c.LogMode != "none" {
		return fmt.Errorf("multivenue: log mode must be full or none, got %q", c.LogMode)
	}
	if len(c.VenueIDs) == 0 {
		c.VenueIDs = []string{"north", "central", "south"}
	}
	if len(c.VenueIDs) != 3 {
		return fmt.Errorf("multivenue: exactly three venue IDs required, got %d", len(c.VenueIDs))
	}
	seen := make(map[string]struct{}, len(c.VenueIDs))
	for _, id := range c.VenueIDs {
		if id == "" {
			return errors.New("multivenue: venue ID must not be empty")
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("multivenue: duplicate venue ID %q", id)
		}
		seen[id] = struct{}{}
	}
	if c.Step == 0 {
		c.Step = time.Second
	}
	if c.SnapshotInterval == 0 {
		c.SnapshotInterval = time.Second
	}
	if c.AutomationInterval == 0 {
		c.AutomationInterval = time.Second
	}
	if c.QuoteInterval == 0 {
		c.QuoteInterval = time.Second
	}
	if c.NoiseInterval == 0 {
		c.NoiseInterval = 2 * time.Second
	}
	if c.GreekInterval == 0 {
		c.GreekInterval = time.Minute
	}
	if c.ShortOptionTenor == 0 {
		c.ShortOptionTenor = 6 * time.Hour
	}
	if c.LongOptionTenor == 0 {
		c.LongOptionTenor = 48 * time.Hour
	}
	if c.ShortFutureTenor == 0 {
		c.ShortFutureTenor = 8 * time.Hour
	}
	if c.LongFutureTenor == 0 {
		c.LongFutureTenor = 72 * time.Hour
	}
	if c.OptionIV == 0 {
		c.OptionIV = 0.8
	}
	if c.StrikesPerSide == 0 {
		c.StrikesPerSide = 2
	}
	if c.StrikeStepUSD == 0 {
		c.StrikeStepUSD = 1_000
	}
	if c.OptionMaxStrikesPerExpiry == 0 {
		c.OptionMaxStrikesPerExpiry = 2*c.StrikesPerSide + 1
	}
	if c.StoikovRiskAversion == 0 {
		c.StoikovRiskAversion = 0.005
	}
	if c.StoikovFillDecay == 0 {
		c.StoikovFillDecay = 0.5
	}
	if c.StoikovVariancePerSecond == 0 {
		c.StoikovVariancePerSecond = 25
	}
	if c.StoikovInventoryHorizon == 0 {
		c.StoikovInventoryHorizon = 10 * time.Minute
	}
	if c.StoikovVolatilityHalfLife == 0 {
		c.StoikovVolatilityHalfLife = 10 * time.Minute
	}
	if c.OptionBuyProbability == 0 {
		c.OptionBuyProbability = 0.65
	}
	if c.DealerHedgeMode == "" {
		c.DealerHedgeMode = "on"
	}
	if c.Step <= 0 || c.SnapshotInterval <= 0 || c.AutomationInterval <= 0 || c.QuoteInterval <= 0 ||
		c.NoiseInterval <= 0 || c.GreekInterval <= 0 || c.ShortOptionTenor <= 0 || c.LongOptionTenor <= 0 ||
		c.ShortFutureTenor <= 0 || c.LongFutureTenor <= 0 || c.StrikesPerSide < 0 || c.StrikeStepUSD <= 0 ||
		c.OptionMaxStrikesPerExpiry <= 0 ||
		c.OptionIV <= 0 || c.StoikovRiskAversion <= 0 || c.StoikovFillDecay <= 0 || c.StoikovVariancePerSecond < 0 ||
		c.StoikovInventoryHorizon <= 0 || c.StoikovVolatilityHalfLife <= 0 || c.OptionBuyProbability < 0 || c.OptionBuyProbability > 1 {
		return errors.New("multivenue: invalid non-positive duration or model parameter")
	}
	if c.DealerHedgeMode != "on" && c.DealerHedgeMode != "off" {
		return fmt.Errorf("multivenue: dealer hedge mode must be on or off, got %q", c.DealerHedgeMode)
	}
	for _, cadence := range []time.Duration{c.SnapshotInterval, c.AutomationInterval, c.QuoteInterval, c.NoiseInterval, c.GreekInterval} {
		if c.Step > cadence {
			return fmt.Errorf("multivenue: step %s exceeds configured cadence %s", c.Step, cadence)
		}
	}
	return nil
}

// Venue holds the local exchange state and actors needed for venue-scoped
// reporting. Accounts are intentionally distinct across venues: no collateral
// transfer or synthetic shared wallet is implied by a common simulation clock.
type Venue struct {
	ID                   string
	Exchange             *exchange.Exchange
	Mount                *simulation.Mount
	SpotMakers           []*StoikovMarketMaker
	PerpMaker            *StoikovMarketMaker
	FuturesMaker         *derivsim.FuturesMarketMaker
	OptionDealer         *derivsim.OptionMarketMaker
	OptionDealerClientID uint64
	NoiseTrader          *feesim.RandomTaker
	OptionFlow           *derivsim.OptionTaker
	InitialRisk          *VenueRiskSnapshot
	RiskTimeline         []VenueRiskSnapshot
	PreExpiryRisk        []VenueRiskSnapshot
	TerminalRisk         *VenueRiskSnapshot
	riskErr              error
	riskLastNano         int64
	optionListedNano     map[string]int64
}

// VenueRiskSnapshot combines exchange-owned marked equity with an
// exchange-owned sensitivity view at a documented lifecycle point. Account
// values are the source of truth for wallet, debt, and PnL; Greeks remain model
// diagnostics and are deliberately kept separate from cash accounting.
type VenueRiskSnapshot struct {
	VenueID        string                       `json:"venue_id"`
	ClientID       uint64                       `json:"client_id"`
	Phase          string                       `json:"phase"`
	Account        etypes.MarkedAccountSnapshot `json:"account"`
	GreekProfile   derivsim.GreekProfile        `json:"greek_profile"`
	GreekPositions []derivsim.GreekPosition     `json:"greek_positions"`
}

// Sim owns the three venue ecology and every log file created for it.
type Sim struct {
	Config  Config
	Runner  *simulation.Runner
	Venues  []*Venue
	loggers []*feesim.JSONLinesLogger
}

// Run starts all venue automation under one context and drives the common
// runner. The direct mounts and deterministic phases deliberately exclude
// latency wrappers until an ordered delayed-delivery runtime exists.
func (s *Sim) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for _, venue := range s.Venues {
		venue.Exchange.StartAutomation(ctx)
	}
	var riskErr error
	s.Runner.SetShutdownHook(func() {
		// The deterministic runner reaches the final venue/actor fixed point
		// before this hook and has not shut down any mount yet. Capture risk
		// here rather than via an actor query, which would have no later phase
		// to deliver its response.
		for _, venue := range s.Venues {
			if riskErr != nil {
				break
			}
			if venue.riskErr != nil {
				riskErr = venue.riskErr
				break
			}
			venue.TerminalRisk, riskErr = captureVenueRisk(venue, "terminal_post_mark")
		}
		cancel()
		for _, venue := range s.Venues {
			venue.Exchange.StopAutomation()
		}
	})
	if err := s.Runner.Run(ctx); err != nil {
		return err
	}
	for _, venue := range s.Venues {
		if venue.riskErr != nil {
			return venue.riskErr
		}
	}
	return riskErr
}

func (s *Sim) Close() {
	for _, logger := range s.loggers {
		logger.Close()
	}
}

type venueLogEvent struct {
	VenueID string `json:"venue_id"`
	Payload any    `json:"payload"`
}

type venueLogger struct {
	venueID string
	inner   etypes.Logger
}

func (l venueLogger) LogEvent(simTime int64, clientID uint64, eventName string, event any) {
	l.inner.LogEvent(simTime, clientID, eventName, venueLogEvent{VenueID: l.venueID, Payload: event})
}

type manifest struct {
	SchemaVersion int      `json:"schema_version"`
	VenueIDs      []string `json:"venue_ids"`
	Config        Config   `json:"config"`
	Notes         []string `json:"notes"`
}

// NewSim constructs three exchanges with one local spot/perp/dated-future/
// option board each. It intentionally does not add cross-venue arbitrage yet:
// that actor must have a venue-qualified order ledger and explicit leg risk.
func NewSim(simTime time.Duration, cfg Config) (*Sim, error) {
	if err := cfg.normalize(); err != nil {
		return nil, err
	}
	if simTime <= 0 || simTime%cfg.Step != 0 {
		return nil, fmt.Errorf("multivenue: simulation duration %s must be a positive multiple of step %s", simTime, cfg.Step)
	}
	if err := os.MkdirAll(filepath.Join(cfg.LogDir, "venues"), 0755); err != nil {
		return nil, err
	}
	manifestBytes, err := json.MarshalIndent(manifest{
		SchemaVersion: 1,
		VenueIDs:      slices.Clone(cfg.VenueIDs),
		Config:        cfg,
		Notes: []string{
			"Each venue has independent prefunded accounts and local spot-margin borrowing.",
			"Direct deterministic mounts are reproducible; latency is intentionally excluded.",
			"No cross-venue routing or collateral transfer exists in schema version 1.",
			"Option dealer begins with zero ABC; spot hedge sells borrow ABC against USD collateral when required.",
			"Raw venue-event logs are controlled by log_mode; greeks.json risk telemetry is always emitted by the command.",
		},
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(cfg.LogDir, "manifest.json"), manifestBytes, 0644); err != nil {
		return nil, err
	}

	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano()
	clock := simulation.NewSimulatedClock(start)
	scheduler := simulation.NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	timers := simulation.NewSimTimerFactory(scheduler)
	runner := simulation.NewRunner(clock, simulation.RunnerConfig{
		Iterations:          int(simTime / cfg.Step),
		Step:                cfg.Step,
		Quiesce:             true,
		DeterministicPhases: true,
	})
	runner.AddIdler(timers)

	sim := &Sim{Config: cfg, Runner: runner, Venues: make([]*Venue, 0, len(cfg.VenueIDs))}
	actorID := uint64(0)
	for venueIndex, id := range cfg.VenueIDs {
		venue, err := sim.addVenue(id, venueIndex, clock, timers, &actorID)
		if err != nil {
			sim.Close()
			return nil, err
		}
		venue.InitialRisk, err = captureVenueRisk(venue, "initial")
		if err != nil {
			sim.Close()
			return nil, err
		}
		sim.Venues = append(sim.Venues, venue)
		runner.AddMount(venue.Mount)
		for _, maker := range venue.SpotMakers {
			runner.AddActor(maker)
		}
		runner.AddActor(venue.PerpMaker)
		runner.AddActor(venue.FuturesMaker)
		runner.AddActor(venue.OptionDealer)
		runner.AddActor(venue.NoiseTrader)
		runner.AddActor(venue.OptionFlow)
	}
	return sim, nil
}

func (s *Sim) addVenue(id string, venueIndex int, clock *simulation.SimulatedClock, timers *simulation.SimTimerFactory, actorID *uint64) (*Venue, error) {
	logDir := filepath.Join(s.Config.LogDir, "venues", id)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}
	newLogger := func(name string) (venueLogger, error) {
		logger, err := feesim.NewJSONLinesLogger(filepath.Join(logDir, name))
		if err != nil {
			return venueLogger{}, err
		}
		s.loggers = append(s.loggers, logger)
		return venueLogger{venueID: id, inner: logger}, nil
	}
	ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		ID:                      id,
		EstimatedClients:        16,
		Clock:                   clock,
		TickerFactory:           timers,
		DeterministicIngress:    true,
		DeterministicPhases:     true,
		SnapshotInterval:        s.Config.SnapshotInterval,
		BalanceSnapshotInterval: time.Minute,
	})
	if s.Config.LogMode == "full" {
		if err := os.MkdirAll(filepath.Join(logDir, "spot"), 0755); err != nil {
			return nil, err
		}
		globalLog, err := newLogger("general.jsonl")
		if err != nil {
			return nil, err
		}
		spotLog, err := newLogger(filepath.Join("spot", "ABC-USD.jsonl"))
		if err != nil {
			return nil, err
		}
		derivativeLog, err := newLogger("derivatives.jsonl")
		if err != nil {
			return nil, err
		}
		ex.SetLogger("_global", globalLog)
		ex.SetLogger("ABC/USD", spotLog)
		ex.SetInstrumentLoggerFallback(derivativeLog)
	}

	tick := int64(10 * mvQuotePrecision)
	spot := exchange.NewSpotInstrument("ABC/USD", "ABC", "USD", mvBasePrecision, mvQuotePrecision, tick, mvBasePrecision/1_000)
	perp := exchange.NewPerpFutures("ABC-PERP", "ABC", "USD", mvBasePrecision, mvQuotePrecision, tick, mvBasePrecision/1_000)
	ex.AddInstrument(spot)
	ex.AddInstrument(perp)

	index := exchange.NewMidPriceOracle(ex)
	index.MapSymbol("ABC-PERP", "ABC/USD")
	spec := instrument.ContractSpec{
		Base: "ABC", Quote: "USD", BasePrecision: mvBasePrecision, QuotePrecision: mvQuotePrecision,
		TickSize: tick, MinOrderSize: mvBasePrecision / 1_000,
	}
	optionSpec := spec
	optionSpec.TickSize = mvQuotePrecision // one USD premium tick
	mount := simulation.NewMount(ex, simulation.LatencyConfig{})
	venue := &Venue{ID: id, Exchange: ex, Mount: mount, optionListedNano: make(map[string]int64)}
	ex.ConfigureAutomation(exchange.AutomationConfig{
		IndexProvider:       index,
		PriceUpdateInterval: s.Config.AutomationInterval,
		ListingPolicies: []exchange.ListingPolicy{
			&instrument.DatedFuturesLister{Underlying: "ABC/USD", Spec: spec, TenorsNano: []int64{s.Config.ShortFutureTenor.Nanoseconds(), s.Config.LongFutureTenor.Nanoseconds()}},
			&instrument.OptionChainLister{
				Underlying: "ABC/USD", Spec: optionSpec,
				TenorsNano: []int64{s.Config.ShortOptionTenor.Nanoseconds(), s.Config.LongOptionTenor.Nanoseconds()},
				StrikeStep: s.Config.StrikeStepUSD * mvQuotePrecision, StrikesPerSide: s.Config.StrikesPerSide,
				MaxStrikesPerExpiry: s.Config.OptionMaxStrikesPerExpiry, IV: s.Config.OptionIV,
			},
		},
		PreExpiryHook: func() {
			if venue.riskErr != nil {
				return
			}
			risk, err := captureVenueRisk(venue, "pre_expiry")
			if err != nil {
				venue.riskErr = err
				return
			}
			venue.PreExpiryRisk = append(venue.PreExpiryRisk, *risk)
		},
		PostDerivativeMarkHook: func() {
			captureScheduledVenueRisk(venue, s.Config.GreekInterval, s.Config.AutomationInterval)
		},
	})
	if err := ex.EnableBorrowing(exchange.BorrowingConfig{
		Enabled:           true,
		AutoBorrowSpot:    true,
		DefaultMarginMode: exchange.CrossMargin,
		CollateralFactors: map[string]float64{"USD": 1},
		MaxBorrowPerAsset: map[string]int64{"USD": 20_000_000 * mvQuotePrecision, "ABC": 20_000 * mvBasePrecision},
		AssetPrecisions:   map[string]int64{"USD": mvQuotePrecision, "ABC": mvBasePrecision},
		PriceSource:       exchange.NewStaticPriceOracle(map[string]int64{"USD": mvQuotePrecision, "ABC": mvBootstrapPrice}),
	}); err != nil {
		return nil, err
	}

	nextClient := uint64(0)
	connect := func(balances map[string]int64, perpUSD int64, fee exchange.FeeModel) actor.Gateway {
		nextClient++
		gw := mount.ConnectNewClient(nextClient, balances, fee)
		if perpUSD > 0 {
			ex.AddPerpBalance(nextClient, "USD", perpUSD)
		}
		return gw
	}
	mmBalances := map[string]int64{"ABC": 10_000 * mvBasePrecision, "USD": 500_000_000 * mvQuotePrecision}
	zeroFee := &exchange.FixedFee{}
	nextActor := func() uint64 {
		*actorID++
		return *actorID
	}
	stoikovConfig := func(symbol, reference string) StoikovMMConfig {
		return StoikovMMConfig{
			Symbol: symbol, ReferenceSymbol: reference, BootstrapPrice: mvBootstrapPrice,
			BasePrecision: mvBasePrecision, QuotePrecision: mvQuotePrecision, TickSize: tick, QuoteQty: mvBasePrecision / 5,
			QuoteInterval: s.Config.QuoteInterval, VolatilityHalfLife: s.Config.StoikovVolatilityHalfLife,
			InitialVariancePerSec: s.Config.StoikovVariancePerSecond, InventoryHorizon: s.Config.StoikovInventoryHorizon,
			RiskAversion: s.Config.StoikovRiskAversion, FillDecay: s.Config.StoikovFillDecay, MinHalfSpreadTicks: 1,
		}
	}
	for i := 0; i < 2; i++ {
		maker := NewStoikovMarketMaker(nextActor(), connect(mmBalances, 100_000_000*mvQuotePrecision, zeroFee), stoikovConfig("ABC/USD", "ABC/USD"))
		maker.SetTickerFactory(timers)
		venue.SpotMakers = append(venue.SpotMakers, maker)
	}
	venue.PerpMaker = NewStoikovMarketMaker(nextActor(), connect(mmBalances, 100_000_000*mvQuotePrecision, zeroFee), stoikovConfig("ABC-PERP", "ABC/USD"))
	venue.PerpMaker.SetTickerFactory(timers)

	venue.FuturesMaker = derivsim.NewFuturesMarketMaker(nextActor(), connect(mmBalances, 100_000_000*mvQuotePrecision, zeroFee), derivsim.FuturesMMConfig{
		Underlying: "ABC/USD", SpreadBps: 6, QuoteQty: mvBasePrecision / 5, Tick: tick, QuoteInterval: s.Config.QuoteInterval,
	})
	venue.FuturesMaker.SetTickerFactory(timers)
	dealerBalances := map[string]int64{"USD": 500_000_000 * mvQuotePrecision}
	venue.OptionDealerClientID = nextClient + 1
	venue.OptionDealer = derivsim.NewOptionMarketMaker(nextActor(), connect(dealerBalances, 150_000_000*mvQuotePrecision, zeroFee), derivsim.OptionMMConfig{
		Underlying: "ABC/USD", IV: s.Config.OptionIV, SpreadBps: 30, SkewPerLotBps: 5,
		QuoteQty: mvBasePrecision / 10, LotQty: mvBasePrecision / 20, PremiumTick: mvQuotePrecision,
		QuoteInterval: s.Config.QuoteInterval, HedgeEnabled: s.Config.DealerHedgeMode == "on", HedgeInterval: s.Config.QuoteInterval,
		HedgeBandQty: mvBasePrecision / 100, GreekInterval: s.Config.GreekInterval, BasePrecision: mvBasePrecision,
	})
	venue.OptionDealer.SetTickerFactory(timers)

	noiseBalances := map[string]int64{"ABC": 100 * mvBasePrecision, "USD": 100 * mvQuotePrecision}
	noiseFee := &exchange.PercentageFee{MakerBps: 0, TakerBps: 5, InQuote: true}
	venue.NoiseTrader = feesim.NewRandomTaker(nextActor(), connect(noiseBalances, 10_000_000*mvQuotePrecision, noiseFee), feesim.TakerConfig{
		Symbols: []string{"ABC/USD", "ABC-PERP"}, TargetQtys: map[string]int64{"ABC/USD": mvBasePrecision / 100, "ABC-PERP": mvBasePrecision / 100},
		TakeInterval: s.Config.NoiseInterval, Seed: s.Config.Seed + int64(venueIndex)*1_000 + 1,
	})
	venue.NoiseTrader.SetTickerFactory(timers)
	venue.OptionFlow = derivsim.NewOptionTaker(nextActor(), connect(noiseBalances, 10_000_000*mvQuotePrecision, noiseFee), derivsim.OptionTakerConfig{
		Underlying: "ABC/USD", PBuy: s.Config.OptionBuyProbability, LotQty: mvBasePrecision / 100,
		Interval: s.Config.NoiseInterval, Seed: s.Config.Seed + int64(venueIndex)*1_000 + 2, IncludeFutures: true,
	})
	venue.OptionFlow.SetTickerFactory(timers)
	return venue, nil
}

func captureVenueRisk(venue *Venue, phase string) (*VenueRiskSnapshot, error) {
	if venue == nil || venue.Exchange == nil || venue.OptionDealer == nil || venue.OptionDealerClientID == 0 {
		return nil, errors.New("multivenue: incomplete venue risk capture")
	}
	spotMid, _ := venue.Exchange.TwoSidedMidPrice("ABC/USD")
	marks := map[string]etypes.AssetValuationMark{
		"USD": {Price: mvQuotePrecision, Precision: mvQuotePrecision},
	}
	if spotMid > 0 {
		marks["ABC"] = etypes.AssetValuationMark{Price: spotMid, Precision: mvBasePrecision}
	}
	account, err := venue.Exchange.MarkedAccount(venue.OptionDealerClientID, etypes.AccountValuationSpec{
		ReportAsset: "USD", ReportPrecision: mvQuotePrecision, AssetMarks: marks,
	})
	if err != nil {
		return nil, fmt.Errorf("multivenue: venue %s marked dealer account: %w", venue.ID, err)
	}
	profile, positions, err := exchangeGreekRisk(venue, account, phase)
	if err != nil {
		return nil, err
	}
	return &VenueRiskSnapshot{
		VenueID:        venue.ID,
		ClientID:       venue.OptionDealerClientID,
		Phase:          phase,
		Account:        account,
		GreekProfile:   profile,
		GreekPositions: positions,
	}, nil
}

func captureScheduledVenueRisk(venue *Venue, interval, automationInterval time.Duration) {
	if venue == nil || venue.Exchange == nil || venue.riskErr != nil {
		return
	}
	now := venue.Exchange.Clock.NowUnixNano()
	if now <= 0 || (venue.riskLastNano != 0 && now-venue.riskLastNano < interval.Nanoseconds() && !hasNearExpiryOption(venue, now, automationInterval)) {
		return
	}
	risk, err := captureVenueRisk(venue, "post_derivative_mark")
	if err != nil {
		venue.riskErr = err
		return
	}
	venue.riskLastNano = now
	venue.RiskTimeline = append(venue.RiskTimeline, *risk)
}

func hasNearExpiryOption(venue *Venue, now int64, cadence time.Duration) bool {
	for _, inst := range venue.Exchange.ListInstruments("", "") {
		option, ok := inst.(*exchange.EuropeanOption)
		if !ok {
			continue
		}
		remaining := option.ExpiryNano() - now
		if remaining > 0 && remaining <= cadence.Nanoseconds() {
			return true
		}
	}
	return false
}

// exchangeGreekRisk derives every sensitivity from the exchange-owned marked
// option position and the option's atomically paired underlying mark. Actor
// cache state is intentionally excluded: at an automation boundary it may not
// yet have received same-timestamp market data.
func exchangeGreekRisk(venue *Venue, account etypes.MarkedAccountSnapshot, phase string) (derivsim.GreekProfile, []derivsim.GreekPosition, error) {
	profile := derivsim.GreekProfile{Timestamp: account.Timestamp, Phase: phase, ForwardSource: "option_underlying_mark"}
	if venue.optionListedNano == nil {
		venue.optionListedNano = make(map[string]int64)
	}
	instruments := venue.Exchange.ListInstruments("", "")
	slices.SortFunc(instruments, func(a, b exchange.Instrument) int { return strings.Compare(a.Symbol(), b.Symbol()) })
	options := make(map[string]*exchange.EuropeanOption)
	for _, inst := range instruments {
		option, ok := inst.(*exchange.EuropeanOption)
		if !ok {
			continue
		}
		options[option.Symbol()] = option
		if _, listed := venue.optionListedNano[option.Symbol()]; !listed {
			venue.optionListedNano[option.Symbol()] = account.Timestamp
		}
	}

	for _, row := range account.SpotBalances {
		if row.Asset == "ABC" {
			profile.HedgeDelta = float64(row.NetAsset) / float64(mvBasePrecision)
			break
		}
	}
	positions := make([]derivsim.GreekPosition, 0)
	for _, pos := range account.Positions {
		if pos.Size == 0 {
			continue
		}
		option := options[pos.Symbol]
		if option == nil {
			continue
		}
		timeToExpiry := option.ExpiryNano() - account.Timestamp
		if timeToExpiry <= 0 {
			// Gamma and vega have no valid finite Black-76 interpretation at
			// expiry. The periodic timeline records the final positive-horizon
			// state before this lifecycle point.
			continue
		}
		forward := option.UnderlyingMark()
		yearsLeft := float64(timeToExpiry) / float64(365*24*time.Hour)
		sensitivity, ok := eprice.Black76Sensitivities(forward, option.Strike, option.IV, yearsLeft, option.IsCall)
		if !ok {
			return derivsim.GreekProfile{}, nil, fmt.Errorf("multivenue: venue %s option %s has invalid exchange Greek mark", venue.ID, option.Symbol())
		}
		contracts := float64(pos.Size) / float64(mvBasePrecision)
		position := derivsim.GreekPosition{
			Timestamp: account.Timestamp, Phase: phase, Symbol: option.Symbol(), Underlying: option.UnderlyingSymbol(),
			ListedNano: venue.optionListedNano[option.Symbol()], ExpiryNano: option.ExpiryNano(), Strike: option.Strike,
			IsCall: option.IsCall, Position: pos.Size, TimeToExpiryNano: timeToExpiry,
			SpotMid: forward, ModelForward: forward, ForwardSource: "option_underlying_mark", ImpliedVolatility: option.IV,
			Delta: contracts * sensitivity.Delta, Gamma: contracts * sensitivity.Gamma, Vega: contracts * sensitivity.Vega,
		}
		positions = append(positions, position)
		profile.OptionDelta += position.Delta
		profile.Gamma += position.Gamma
		profile.Vega += position.Vega
		profile.Contracts++
		if profile.ModelForward == 0 {
			profile.SpotMid = forward
			profile.ModelForward = forward
			profile.ImpliedVolatility = option.IV
		}
	}
	profile.NetDelta = profile.OptionDelta + profile.HedgeDelta
	return profile, positions, nil
}
