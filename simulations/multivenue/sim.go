package multivenue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/instrument"
	"exchange_sim/matching"
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
	mvCDFBootstrap   = 3_000 * mvQuotePrecision
)

const (
	MatchingPriceTime = "price_time"
	MatchingProRata   = "pro_rata"
)

// VenueRule contains the venue-specific market mechanism settings that are
// currently admissible in the multivenue control. The scenario uses a map
// keyed by venue ID, but NewSim traverses VenueIDs so map iteration never
// chooses a causal order.
type VenueRule struct {
	MatchingRule string `json:"matching_rule"`
	// FundingIntervalSeconds overrides the population's funding period for this
	// venue alone. Real venues settle perpetual funding on different clocks,
	// and a carry desk that holds the same exposure on two of them faces a
	// different payment schedule on each: some intervals coincide and most do
	// not. A single shared period makes every venue pay at the same instant,
	// which is the one schedule under which cross-venue funding arbitrage has
	// nothing to trade. Zero keeps the population default.
	FundingIntervalSeconds int64 `json:"funding_interval_seconds"`
}

// Config creates exactly three separately funded direct venues on one
// deterministic simulated clock. The one-second default is intentional: all
// configured actor and venue timers are at least one second, making hour/day
// experiments feasible without changing their event semantics through tick
// coalescing.
// Provenance carries campaign bookkeeping into a run so a log directory can be
// traced back to the hypothesis it was testing. None of it affects behaviour;
// it is declared so that strict decoding accepts annotated configurations.
type Provenance struct {
	ExperimentID string `json:"experiment_id"`
	HypothesisID string `json:"hypothesis_id"`
	Date         string `json:"date"`
	Status       string `json:"status"`
	Description  string `json:"description"`
}

type Config struct {
	LogDir string `json:"log_dir,omitempty"`
	// LogMode controls raw venue-event persistence. "full" is the default
	// evidence mode; "none" retains deterministic in-memory risk telemetry and
	// greeks.json while avoiding large JSONL output for replicated treatments.
	Provenance

	LogMode string `json:"log_mode"`
	// CheckpointIntervalSeconds writes a rolling digest of the event stream at
	// each simulated-time boundary, so two runs of one seed can be compared
	// without retaining their logs. Zero disables it.
	CheckpointIntervalSeconds int `json:"checkpoint_interval_seconds,omitempty"`
	// TraceFromNano and TraceToNano dump a compact per-event trace for one
	// window only, which is how the first divergent event is identified once
	// the checkpoints have bracketed it. An empty window disables the trace.
	TraceFromNano int64 `json:"trace_from_nano,omitempty"`
	TraceToNano   int64 `json:"trace_to_nano,omitempty"`

	Seed     int64    `json:"seed"`
	VenueIDs []string `json:"venue_ids"`
	// StrictPopulationAccounting requires a complete initial and terminal USD
	// marked account for every connected participant. It is the required mode
	// for FFA fitness experiments; legacy mechanism controls may leave it off.
	StrictPopulationAccounting bool `json:"strict_population_accounting"`
	// VenueRules selects the exact matching policy for each venue. Omitted
	// entries preserve the established price-time control.
	VenueRules map[string]VenueRule `json:"venue_rules"`
	// CrossAssetSpotGraph adds the direct USD marks and cross pair required for
	// the smallest FFA asset graph: ABC/USD, CDF/USD, and ABC/CDF. It is opt-in
	// so retained one-underlying derivative controls remain unchanged.
	CrossAssetSpotGraph bool `json:"cross_asset_spot_graph"`

	Step               time.Duration `json:"step"`
	SnapshotInterval   time.Duration `json:"snapshot_interval"`
	AutomationInterval time.Duration `json:"automation_interval"`
	QuoteInterval      time.Duration `json:"quote_interval"`
	NoiseInterval      time.Duration `json:"noise_interval"`
	GreekInterval      time.Duration `json:"greek_interval"`
	// NoiseTraderCount and OptionFlowCount control independently funded,
	// independently seeded flow participants on every venue. A zero value
	// preserves the historical one-each baseline.
	NoiseTraderCount int `json:"noise_trader_count"`
	OptionFlowCount  int `json:"option_flow_count"`

	// StoikovMaxVarianceMultiple caps a maker's volatility estimate at this
	// multiple of its initial value.
	StoikovMaxVarianceMultiple float64 `json:"stoikov_max_variance_multiple"`

	// StoikovVolatilitySampleInterval is the minimum spacing between the trade
	// prices a maker uses to estimate volatility.
	StoikovVolatilitySampleInterval time.Duration `json:"stoikov_volatility_sample_interval"`

	// SpotTickQuoteUnits is the ABC/USD tick in quote-precision units. The
	// spread cannot be finer than one tick, so a coarse tick pins it at the
	// floor and makes it insensitive to volatility and to maker risk; that
	// degeneracy is the large-tick regime.
	SpotTickQuoteUnits int64 `json:"spot_tick_quote_units"`

	// MakerAnchor selects what the spot makers quote around: "own_mid" is each
	// maker's own book midpoint, and "consensus" is the average of the venues'
	// midpoints published as each venue's index. Both are endogenous: they are
	// computed from what participants did.
	MakerAnchor string `json:"maker_anchor"`
	// RoundTripTraderCount adds participants whose demand mean-reverts in
	// quantity: they open a position and unwind it after RoundTripHold. Pure
	// random-side flow mean-reverts in price but not in quantity, which leaves
	// market makers holding a position that never returns.
	RoundTripTraderCount int           `json:"round_trip_trader_count"`
	RoundTripHold        time.Duration `json:"round_trip_hold"`
	RoundTripLotQty      int64         `json:"round_trip_lot_qty"`
	// NoiseImbalanceCoupling tilts uninformed order side toward the visible book
	// imbalance, and NoiseExciteAlpha with NoiseExciteBetaPerSec make the flow
	// self-exciting: each observed trade raises an arrival rate that decays at
	// the given rate.
	//
	// Both default to zero, which is flow arriving on a fixed clock with an
	// independent side. That produces a market with no volatility clustering,
	// no fat tails and no order-flow memory — measured on the reference
	// population, excess kurtosis was -0.60 and the sign autocorrelation at lag
	// 50 was 0.013, where traded markets show strong positive values for both.
	// NoiseOrderQty is the size of one uninformed order. It has to be
	// configurable and it has to be set against MakerQuoteQty: an order worth a
	// fraction of the quoted depth is absorbed whole and never moves a price,
	// so the return series collapses to the bid-ask bounce. The reference
	// population quoted 5 ABC and traded 0.01, a ratio of five hundred to one.
	// MakerQuoteSizeVolElasticity withdraws maker depth as its volatility
	// estimate rises, and MinQuoteSizeFraction floors that withdrawal. Constant
	// depth is what denies this market volatility clustering.
	// MakerForwardHalfLife smooths the maker's view of its reference book. Zero
	// takes the instantaneous midpoint, which makes price impact permanent.
	MakerForwardHalfLife time.Duration `json:"maker_forward_half_life"`

	MakerQuoteSizeVolElasticity float64 `json:"maker_quote_size_vol_elasticity"`
	MakerMinQuoteSizeFraction   float64 `json:"maker_min_quote_size_fraction"`

	// ElasticSupplierReferenceHalfLife is how quickly the price-elastic
	// participant revises its reference toward observed prices. Zero holds its
	// seed forever, which is an exogenous anchor.
	ElasticSupplierReferenceHalfLife time.Duration `json:"elastic_supplier_reference_half_life"`

	NoiseOrderQty int64 `json:"noise_order_qty"`
	// NoiseTargetQtyBySymbol overrides the uninformed order size per book.
	// One size across every book means the flow is sized for the main pair and
	// negligible everywhere else, which leaves the other books to whichever
	// arbitrageur trades them in size — measured on the second spot pair, one
	// class held ninety-six percent of the volume in every window.
	NoiseTargetQtyBySymbol map[string]int64 `json:"noise_target_qty_by_symbol"`

	// NoiseFundingLots is how many orders' worth of each asset an uninformed
	// trader starts with. Underfunding it makes the flow directional rather
	// than random, because only one side gets rejected.
	NoiseFundingLots int64 `json:"noise_funding_lots"`
	// NoiseSizeParetoAlpha draws order size from a Pareto tail when positive,
	// so that an occasional order walks the book instead of every order being
	// absorbed at the touch.
	NoiseSizeParetoAlpha float64 `json:"noise_size_pareto_alpha"`
	NoiseSizeCapMultiple float64 `json:"noise_size_cap_multiple"`

	NoiseImbalanceCoupling float64 `json:"noise_imbalance_coupling"`
	NoiseExciteAlpha       float64 `json:"noise_excite_alpha"`
	NoiseExciteBetaPerSec  float64 `json:"noise_excite_beta_per_sec"`

	// RoundTripInventoryLots is how many lots of balance each round-trip desk
	// is funded with on each side. Funding it below one lot of quote currency
	// makes every long open fail, which turns symmetric flow into a persistent
	// seller.
	RoundTripInventoryLots int64 `json:"round_trip_inventory_lots"`

	// ElasticSupplierCount adds participants with a downward-sloping demand
	// curve, which is what absorbs a persistent drift: they sell as the price
	// rises and buy back as it falls. Without one, market makers hold the
	// mirror of the accumulated price drift.
	ElasticSupplierCount int `json:"elastic_supplier_count"`
	// ElasticSupplierUnitsPerPercent is how many base units each supplier's
	// target position falls for every percent the price rises.
	ElasticSupplierUnitsPerPercent int64 `json:"elastic_supplier_units_per_percent"`

	// CarryArbitrageurCount adds delta-neutral participants that hold offsetting
	// spot and perpetual positions and earn the basis. They are the only
	// participant here that can take the other side of directional flow without
	// accumulating direction themselves.
	CarryArbitrageurCount int   `json:"carry_arbitrageur_count"`
	CarryEntryBps         int64 `json:"carry_entry_bps"`
	CarryExitBps          int64 `json:"carry_exit_bps"`
	CarryMaxPosition      int64 `json:"carry_max_position"`
	// CarryLotQty is how much a carry participant adds per tick, which bounds
	// how fast it can deploy capital into a basis that mean-reverts.
	CarryLotQty int64 `json:"carry_lot_qty"`

	// PerpMakerInventoryLimit overrides the risk budget for the perpetual
	// maker alone. The shared limit moves the spot and perpetual skews
	// together, which changes the basis between the two books for reasons that
	// have nothing to do with the perpetual; separating them isolates the
	// premium.
	PerpMakerInventoryLimit int64 `json:"perp_maker_inventory_limit"`

	// FundingIntervalSeconds is how often perpetual funding settles. It matters
	// beyond bookkeeping: the ranking between market making and carry
	// arbitrage flips across a funding payment, so this parameter sets the
	// horizon at which one strategy overtakes the other.
	FundingIntervalSeconds int64 `json:"funding_interval_seconds"`

	// FundingMaxRateBps caps the perpetual funding rate per interval. The cap
	// matters because it bounds the basis funding can close: if the market's
	// equilibrium premium exceeds it, funding saturates and the premium
	// persists indefinitely.
	FundingMaxRateBps int64 `json:"funding_max_rate_bps"`

	// OptionDealerCount and FuturesMakerCount set how many independent dealers
	// compete on the option chain and the dated ladder. With one of each the
	// derivative markets are one against many; competing dealers are what makes
	// them many against many.
	OptionDealerCount int `json:"option_dealer_count"`

	// DatedCarryArbCount and ParityArbCount populate the two classes that take
	// the other side of the derivative dealers: cash-and-carry desks that trade
	// dated futures against spot, and put-call parity desks. Without them the
	// dated ladder is quoted but has no counterparty flow.
	DatedCarryArbCount    int   `json:"dated_carry_arb_count"`
	ParityArbCount        int   `json:"parity_arb_count"`
	DatedCarryEdgeBps     int64 `json:"dated_carry_edge_bps"`
	DatedCarrySlippageBps int64 `json:"dated_carry_slippage_bps"`
	// DatedCarryCheckInterval defaults to QuoteInterval. Sharing the makers'
	// cadence phase-locks the desk to the instant the makers have cancelled and
	// not yet replaced, so it repeatedly takes against an empty book.
	DatedCarryCheckInterval time.Duration `json:"dated_carry_check_interval"`
	ParityEdgeBps           int64         `json:"parity_edge_bps"`
	// DatedCarryScaleEdge demands less edge as settlement risk shrinks into
	// expiry, which is what produces a square-root convergence envelope.
	DatedCarryScaleEdge bool `json:"dated_carry_scale_edge"`
	FuturesMakerCount   int  `json:"futures_maker_count"`

	// FuturesMakerSelfAnchored lets each dated book price itself from its own
	// last trade instead of pinning its mid to the spot mid. Pinned to spot the
	// basis is identically zero and there is no term structure for a carry desk
	// to trade, so the dated ladder cannot be economically alive.
	FuturesMakerSelfAnchored bool `json:"futures_maker_self_anchored"`

	// SpotMakerCount is how many market makers quote the main spot pair on each
	// venue, so the carrying capacity of market making can be measured the same
	// way as that of any other strategy.

	// DegradedIndex models transport and measurement error on the published
	// consensus index, which every participant then observes equally.
	DegradedIndex *DegradedIndexConfig `json:"degraded_index"`

	// TakerFeeBps is the fee a taker pays on each leg. It was hardcoded at five
	// basis points, which made the round-trip cost that bounds cross-venue
	// dispersion impossible to vary and therefore impossible to test.
	TakerFeeBps int64 `json:"taker_fee_bps"`

	// RateLimitTiers gives named classes of participant their own request
	// budgets, as venues publish different allowances for different clients.
	// An empty map leaves every participant unmetered, which is what every
	// scenario before request gating assumed.
	RateLimitTiers map[string]RateLimitTier `json:"rate_limit_tiers"`

	// Naive strategy counts. These are the textbook strategies from public
	// trading newsletters, present so they compete against the inventory
	// managing makers rather than being assumed away.
	FixedDistanceMakerCount int                       `json:"fixed_distance_maker_count"`
	FixedDistanceMaker      *FixedDistanceMakerConfig `json:"fixed_distance_maker"`
	ImbalanceMakerCount     int                       `json:"imbalance_maker_count"`
	ImbalanceMaker          *ImbalanceMakerConfig     `json:"imbalance_maker"`
	TriangleArbCount        int                       `json:"triangle_arb_count"`
	TriangleArb             *TriangleArbConfig        `json:"triangle_arb"`

	// BootstrapDepthCount rests passive ladders that never reprice, so a run can
	// ask whether takers fail for lack of depth or for lack of depth at their
	// scheduled instant. BootstrapDepth.Withdraw retires the scaffold mid-run.
	BootstrapDepthCount int                   `json:"bootstrap_depth_count"`
	BootstrapDepth      *BootstrapDepthConfig `json:"bootstrap_depth"`

	// SpotMakerRequoteBps stops a maker replacing quotes until its target has
	// moved this far, which desynchronises a population that otherwise requotes
	// in lockstep every step.
	SpotMakerRequoteBps int64 `json:"spot_maker_requote_bps"`
	// SpotMakerRequoteBpsTiers gives makers different thresholds, cycled across
	// them. One shared threshold makes every book go stale at the same instant,
	// which removes the dislocations cross-market arbitrage trades; several
	// leave the books drifting apart.
	SpotMakerRequoteBpsTiers []int64 `json:"spot_maker_requote_bps_tiers"`

	// SpotMakerSubmitBeforeCancel makes the spot makers replace quotes without
	// emptying the book. Cancel-then-replace leaves both sides empty for the
	// rest of the phase in nearly every step.
	SpotMakerSubmitBeforeCancel bool `json:"spot_maker_submit_before_cancel"`

	SpotMakerCount int `json:"spot_maker_count"`

	// MakerQuoteQty is the size each spot and perpetual maker displays at its
	// quote. It bounds how fast any taker, including a delta-neutral absorber,
	// can transfer risk: an immediate-or-cancel order cannot take more than is
	// displayed.
	MakerQuoteQty int64 `json:"maker_quote_qty"`

	// MakerHedgeSymbol, when set, is where the ABC/USD makers offset the
	// inventory they take on. Real makers quote one instrument and move the
	// delta to another rather than running flat by holding the asset.
	MakerHedgeSymbol      string `json:"maker_hedge_symbol"`
	MakerHedgeBandQty     int64  `json:"maker_hedge_band_qty"`
	MakerHedgeSlippageBps int64  `json:"maker_hedge_slippage_bps"`

	// MakerInventoryLimit is the position a spot maker treats as its full risk
	// budget, in base units.
	MakerInventoryLimit int64 `json:"maker_inventory_limit"`
	// MakerMinHalfSpreadTicks is the floor a spot maker puts under its own
	// half-spread. It has to be configurable because it is the quantity the
	// inventory skew competes against: when the skew separating two makers'
	// reservations exceeds their quoted spread, their quotes cross by
	// construction and the makers trade with each other instead of resting
	// depth for anyone else.
	MakerMinHalfSpreadTicks int64 `json:"maker_min_half_spread_ticks"`
	// MakerHedgeInterval gives the spot maker's hedge its own cadence. Zero
	// leaves it inside the quote cycle, which stops hedging whenever the market
	// calms enough to suppress requoting.
	MakerHedgeInterval time.Duration `json:"maker_hedge_interval"`
	// MakerInventorySkewBps sets the maker's reservation shift at its full
	// inventory limit, in basis points. Zero keeps the textbook
	// variance-derived skew.
	MakerInventorySkewBps int64 `json:"maker_inventory_skew_bps"`
	// MakerIndexWeight blends the index with the maker's own midpoint.
	MakerIndexWeight float64 `json:"maker_index_weight"`

	// LatentLiquidityCount adds participants holding unexpressed intentions
	// whose reservation prices diffuse and become orders on crossing. The
	// square-root impact law is attributed to exactly this: latent liquidity
	// that vanishes near the transaction price.
	LatentLiquidityCount int                    `json:"latent_liquidity_count"`
	LatentLiquidity      *LatentLiquidityConfig `json:"latent_liquidity"`

	// MetaorderTraderCount is how many execution agents each venue runs.
	// Impact is a statistical measurement over many parent orders, so a single
	// agent per venue cannot produce a usable sample in a reasonable run.
	MetaorderTraderCount int `json:"metaorder_trader_count"`
	// MetaorderTraders configures execution agents that split large parent
	// orders. Their signs are independent of the price path, so what
	// they measure is the mechanical impact of execution rather than a trading
	// signal.
	MetaorderTraders *MetaorderTraderConfig `json:"metaorder_traders"`

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

	// OptionBuyProbability is nil when omitted, in which case normalize applies
	// the baseline buy bias. A non-nil zero deliberately creates all-sell flow.
	OptionBuyProbability *float64 `json:"option_buy_probability"`

	// FutureFlowCount adds participants that take only dated futures. The
	// mixed option-and-futures flow draws uniformly from what is listed, so
	// beside a thirty-strike chain the two dated books receive a twentieth of
	// it and trade about once an hour; a book with one maker class and almost
	// no demand is dead however long the run is.
	FutureFlowCount int `json:"future_flow_count"`
	// FutureFlowLotQty and FutureFlowInterval size that flow. Zero uses the
	// option flow's lot and the uninformed flow's cadence.
	FutureFlowLotQty   int64         `json:"future_flow_lot_qty"`
	FutureFlowInterval time.Duration `json:"future_flow_interval"`

	// VannaVolgaDeskCount adds desks that take the option dealers' second-order
	// risk off them by trading options: vega, vanna and volga cannot be hedged
	// in the underlying at any size, so a population with only delta hedgers
	// has nowhere for that risk to go and it accumulates on the dealers until
	// they stop quoting.
	VannaVolgaDeskCount int `json:"vanna_volga_desk_count"`
	// VannaVolgaTolerances are the exposures a desk carries before it trades,
	// and VannaVolgaLotQty and VannaVolgaMaxContracts bound each hedge and the
	// position it may build.
	VannaVolgaVegaTolerance  float64       `json:"vanna_volga_vega_tolerance"`
	VannaVolgaVannaTolerance float64       `json:"vanna_volga_vanna_tolerance"`
	VannaVolgaVolgaTolerance float64       `json:"vanna_volga_volga_tolerance"`
	VannaVolgaLotQty         int64         `json:"vanna_volga_lot_qty"`
	VannaVolgaMaxContracts   int64         `json:"vanna_volga_max_contracts"`
	VannaVolgaInterval       time.Duration `json:"vanna_volga_interval"`
	// VannaVolgaVol is the desk's own view, configured like the dealers'. A
	// desk hedging with the dealer's volatility would price the risk it is
	// buying at exactly what the dealer thinks it is worth.
	VannaVolgaVol OptionDealerVolConfig `json:"vanna_volga_vol"`

	// LatencyProfiles gives participant classes different links to the venue,
	// keyed by the role prefix a participant is connected under
	// ("spot_maker", "noise_flow", "carry_arb", and so on). A market where
	// everyone reaches the matching engine in the same time has no reason for
	// anyone to be picked off, which removes the cost that makes speed worth
	// paying for. The empty map connects every participant directly, which is
	// what the population did before.
	LatencyProfiles map[string]LatencyProfile `json:"latency_profiles"`
	// DefaultLatencyProfile applies to roles LatencyProfiles does not name.
	DefaultLatencyProfile *LatencyProfile `json:"default_latency_profile"`

	// ElasticSupplierSymbols places the price-elastic participants on books
	// other than ABC/USD, assigned in order and cycled. Empty keeps them all
	// on ABC/USD, which is where they were when the second spot book turned
	// out to have nobody who cared about its level.
	ElasticSupplierSymbols []string `json:"elastic_supplier_symbols"`

	// FixedDistanceMakerSymbols and ImbalanceMakerSymbols place those maker
	// classes on books other than ABC/USD, assigning participants entries in
	// order and cycling. Empty keeps every one of them on ABC/USD, which is
	// how the population came to have three maker classes on one book and one
	// on every other: a book quoted by a single class has a single point of
	// failure whatever its volume.
	FixedDistanceMakerSymbols []string `json:"fixed_distance_maker_symbols"`
	ImbalanceMakerSymbols     []string `json:"imbalance_maker_symbols"`

	// OptionDealerVol configures what each option dealer prices with. Dealers
	// are given consecutive entries from HalfLives and Premiums, cycling if
	// there are more dealers than entries, so that a population's dealers
	// disagree without any of them being told to quote differently. Leaving it
	// empty prices every dealer at OptionIV, which is one opinion held by
	// everybody.
	OptionDealerVol OptionDealerVolConfig `json:"option_dealer_vol"`

	// OptionDealerHedgePolicies names the hedge each dealer runs, taken in
	// order and cycled: "banded", "static", "timed", or "none". Empty leaves
	// every dealer on the banded hedge that DealerHedgeMode switches.
	OptionDealerHedgePolicies []string `json:"option_dealer_hedge_policies"`
	// OptionDealerHedgeIntervalSeconds is the rebalancing period for dealers
	// assigned the timed policy.
	OptionDealerHedgeIntervalSeconds int64 `json:"option_dealer_hedge_interval_seconds"`

	// OptionValueTakerCount adds participants that value every listed option
	// with their own volatility model and trade only where a dealer's quote
	// disagrees with them by more than OptionValueTakerEdgeBps. They are what
	// makes a badly quoted strike likelier to trade than a well quoted one.
	OptionValueTakerCount       int           `json:"option_value_taker_count"`
	OptionValueTakerEdgeBps     int64         `json:"option_value_taker_edge_bps"`
	OptionValueTakerLotQty      int64         `json:"option_value_taker_lot_qty"`
	OptionValueTakerMaxPosition int64         `json:"option_value_taker_max_position"`
	OptionValueTakerInterval    time.Duration `json:"option_value_taker_interval"`
	// OptionValueTakerVol configures their views the same way the dealers'
	// are configured. Takers and dealers drawing from different premiums is
	// what gives the two sides a reason to trade with each other.
	OptionValueTakerVol OptionDealerVolConfig `json:"option_value_taker_vol"`

	// DealerHedgeMode selects the option dealer treatment arm: "on" uses the
	// stateful spot hedge policy, "off" leaves filled-option delta unhedged.
	// It is explicit rather than a bool so an omitted JSON field keeps the
	// historical hedged baseline while a false-like zero value is not confused
	// with an experiment arm.
	DealerHedgeMode string `json:"dealer_hedge_mode"`

	// CrossVenueArbTiers enables independently prefunded spot routers. Each
	// router has one account per venue and uses non-atomic FOK legs; no asset
	// transfer or shared wallet is implied. Empty keeps the venue-local
	// baseline unchanged.
	CrossVenueArbTiers       []float64     `json:"cross_venue_arb_tiers"`
	CrossVenueBaseLatency    time.Duration `json:"cross_venue_base_latency"`
	CrossVenueArbLotQty      int64         `json:"cross_venue_arb_lot_qty"`
	CrossVenueArbMaxAttempts int           `json:"cross_venue_arb_max_attempts"`
}

// DecodeConfig reads a scenario configuration and rejects unknown fields. A
// config written for a newer binary would otherwise run silently as the
// default scenario, which is indistinguishable from a control arm.
func DecodeConfig(raw []byte) (Config, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	cfg := Config{}
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode multivenue config: %w", err)
	}
	return cfg, nil
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
	switch c.MakerAnchor {
	case "", "own_mid", "consensus":
	default:
		return fmt.Errorf("multivenue: maker anchor must be own_mid or consensus, got %q", c.MakerAnchor)
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
	for id, rule := range c.VenueRules {
		if _, exists := seen[id]; !exists {
			return fmt.Errorf("multivenue: venue rule references unknown venue %q", id)
		}
		if rule.FundingIntervalSeconds < 0 {
			return fmt.Errorf("multivenue: venue %q has negative funding interval %d", id, rule.FundingIntervalSeconds)
		}
		if rule.MatchingRule == "" {
			continue
		}
		if rule.MatchingRule != MatchingPriceTime && rule.MatchingRule != MatchingProRata {
			return fmt.Errorf("multivenue: venue %q has unsupported matching rule %q", id, rule.MatchingRule)
		}
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
	if c.NoiseTraderCount == 0 {
		c.NoiseTraderCount = 1
	}
	if c.OptionFlowCount == 0 {
		c.OptionFlowCount = 1
	}
	if c.StoikovMaxVarianceMultiple == 0 {
		c.StoikovMaxVarianceMultiple = 25
	}
	if c.StoikovVolatilitySampleInterval == 0 {
		c.StoikovVolatilitySampleInterval = 30 * time.Second
	}
	if c.SpotTickQuoteUnits == 0 {
		c.SpotTickQuoteUnits = 10 * mvQuotePrecision
	}
	if c.MakerAnchor == "" {
		c.MakerAnchor = "own_mid"
	}
	if c.RoundTripHold == 0 {
		c.RoundTripHold = 5 * time.Minute
	}
	if c.RoundTripInventoryLots == 0 {
		c.RoundTripInventoryLots = 20
	}
	if c.RoundTripLotQty == 0 {
		c.RoundTripLotQty = mvBasePrecision / 10
	}
	if c.ElasticSupplierUnitsPerPercent == 0 {
		c.ElasticSupplierUnitsPerPercent = 50 * mvBasePrecision
	}
	if c.CarryEntryBps == 0 {
		c.CarryEntryBps = 20
	}
	if c.CarryExitBps == 0 {
		c.CarryExitBps = 5
	}
	if c.CarryMaxPosition == 0 {
		c.CarryMaxPosition = 500 * mvBasePrecision
	}
	if c.FundingIntervalSeconds == 0 {
		c.FundingIntervalSeconds = 28800
	}
	if c.FundingMaxRateBps == 0 {
		c.FundingMaxRateBps = 75
	}
	if c.TakerFeeBps == 0 {
		c.TakerFeeBps = 5
	}
	if c.DatedCarrySlippageBps == 0 {
		c.DatedCarrySlippageBps = 15
	}
	if c.DatedCarryEdgeBps == 0 {
		c.DatedCarryEdgeBps = 10
	}
	if c.ParityEdgeBps == 0 {
		c.ParityEdgeBps = 20
	}
	if c.OptionDealerCount == 0 {
		c.OptionDealerCount = 1
	}
	if c.VannaVolgaDeskCount > 0 {
		if c.VannaVolgaLotQty <= 0 {
			c.VannaVolgaLotQty = mvBasePrecision / 20
		}
		if c.VannaVolgaMaxContracts <= 0 {
			c.VannaVolgaMaxContracts = 5 * mvBasePrecision
		}
		if c.VannaVolgaInterval <= 0 {
			c.VannaVolgaInterval = c.QuoteInterval
		}
		if c.VannaVolgaVegaTolerance <= 0 || c.VannaVolgaVannaTolerance <= 0 || c.VannaVolgaVolgaTolerance <= 0 {
			return errors.New("multivenue: vanna-volga desks need positive vega, vanna and volga tolerances, or they trade on every tick")
		}
		if err := c.VannaVolgaVol.validate("vanna_volga_vol"); err != nil {
			return err
		}
	}
	if c.OptionValueTakerCount > 0 {
		if c.OptionValueTakerLotQty <= 0 {
			c.OptionValueTakerLotQty = mvBasePrecision / 100
		}
		if c.OptionValueTakerMaxPosition <= 0 {
			c.OptionValueTakerMaxPosition = mvBasePrecision
		}
		if c.OptionValueTakerInterval <= 0 {
			c.OptionValueTakerInterval = c.NoiseInterval
		}
		if c.OptionValueTakerEdgeBps <= 0 {
			return errors.New("multivenue: option value takers need a positive edge requirement, or they cross every spread")
		}
	}
	for symbol, qty := range c.NoiseTargetQtyBySymbol {
		if qty <= 0 {
			return fmt.Errorf("multivenue: noise target quantity for %q must be positive, got %d", symbol, qty)
		}
		switch symbol {
		case "ABC/USD", "ABC-PERP":
		case "CDF/USD", "ABC/CDF":
			if !c.CrossAssetSpotGraph {
				return fmt.Errorf("multivenue: noise target quantity for %q requires the cross-asset spot graph", symbol)
			}
		default:
			return fmt.Errorf("multivenue: no book named %q for a noise target quantity", symbol)
		}
	}
	for role, profile := range c.LatencyProfiles {
		if err := profile.validate(role); err != nil {
			return err
		}
	}
	if c.DefaultLatencyProfile != nil {
		if err := c.DefaultLatencyProfile.validate("default"); err != nil {
			return err
		}
	}
	for _, symbols := range [][]string{c.FixedDistanceMakerSymbols, c.ImbalanceMakerSymbols, c.ElasticSupplierSymbols} {
		for _, symbol := range symbols {
			switch symbol {
			case "ABC/USD", "ABC-PERP":
			case "CDF/USD", "ABC/CDF":
				if !c.CrossAssetSpotGraph {
					return fmt.Errorf("multivenue: maker symbol %q requires the cross-asset spot graph", symbol)
				}
			default:
				return fmt.Errorf("multivenue: unsupported maker symbol %q", symbol)
			}
		}
	}
	for _, policy := range c.OptionDealerHedgePolicies {
		switch policy {
		case "banded", "static", "timed", "none":
		default:
			return fmt.Errorf("multivenue: unsupported dealer hedge policy %q", policy)
		}
	}
	if err := c.OptionDealerVol.validate("option_dealer_vol"); err != nil {
		return err
	}
	if err := c.OptionValueTakerVol.validate("option_value_taker_vol"); err != nil {
		return err
	}
	if c.FuturesMakerCount == 0 {
		c.FuturesMakerCount = 1
	}
	if c.SpotMakerCount == 0 {
		c.SpotMakerCount = 2
	}
	if c.MakerQuoteQty == 0 {
		c.MakerQuoteQty = mvBasePrecision / 5
	}
	if c.CarryLotQty == 0 {
		c.CarryLotQty = mvBasePrecision / 5
	}
	if c.MakerHedgeSlippageBps == 0 {
		c.MakerHedgeSlippageBps = 50
	}
	if c.MakerHedgeBandQty == 0 {
		c.MakerHedgeBandQty = mvBasePrecision
	}
	if c.MakerMinHalfSpreadTicks == 0 {
		c.MakerMinHalfSpreadTicks = 1
	}
	if c.MakerInventoryLimit == 0 {
		c.MakerInventoryLimit = 100 * mvBasePrecision
	}
	if c.MakerIndexWeight == 0 {
		c.MakerIndexWeight = 1
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
		// The control depends on the product of risk aversion and inventory
		// horizon, so only that product is meaningful. This value was chosen
		// with a live forward feed: at 1e-4 the market diverges, and below
		// about 2e-6 the spread reaches the tick floor and further reduction
		// changes nothing. An earlier and much larger value was calibrated
		// while the makers' snapshot feed was silently disabled, which froze
		// their forward and made the market artificially stable.
		c.StoikovRiskAversion = 0.000002
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
	if c.OptionBuyProbability == nil {
		defaultOptionBuyProbability := 0.65
		c.OptionBuyProbability = &defaultOptionBuyProbability
	}
	if c.DealerHedgeMode == "" {
		c.DealerHedgeMode = "on"
	}
	if len(c.CrossVenueArbTiers) > 0 {
		if c.CrossVenueBaseLatency <= 0 {
			return errors.New("multivenue: cross-venue base latency is required when routers are enabled")
		}
		if c.CrossVenueArbLotQty == 0 {
			c.CrossVenueArbLotQty = mvBasePrecision / 100
		}
		if c.CrossVenueArbMaxAttempts == 0 {
			c.CrossVenueArbMaxAttempts = 10
		}
		seenTiers := make(map[float64]struct{}, len(c.CrossVenueArbTiers))
		for _, tier := range c.CrossVenueArbTiers {
			if tier <= 0 {
				return errors.New("multivenue: cross-venue latency tiers must be positive")
			}
			if _, exists := seenTiers[tier]; exists {
				return fmt.Errorf("multivenue: duplicate cross-venue latency tier %g", tier)
			}
			seenTiers[tier] = struct{}{}
			delay := time.Duration(float64(c.CrossVenueBaseLatency) * tier)
			if delay <= 0 || c.Step > delay {
				return fmt.Errorf("multivenue: step %s exceeds cross-venue tier %g latency %s", c.Step, tier, delay)
			}
		}
	}
	if c.Step <= 0 || c.SnapshotInterval <= 0 || c.AutomationInterval <= 0 || c.QuoteInterval <= 0 ||
		c.NoiseInterval <= 0 || c.GreekInterval <= 0 || c.ShortOptionTenor <= 0 || c.LongOptionTenor <= 0 ||
		c.ShortFutureTenor <= 0 || c.LongFutureTenor <= 0 || c.StrikesPerSide < 0 || c.StrikeStepUSD <= 0 ||
		c.OptionMaxStrikesPerExpiry <= 0 || c.NoiseTraderCount < 1 || c.OptionFlowCount < 1 ||
		c.StoikovMaxVarianceMultiple <= 0 || c.StoikovVolatilitySampleInterval < 0 || c.SpotTickQuoteUnits <= 0 || c.MakerIndexWeight <= 0 || c.MakerIndexWeight > 1 || c.MakerInventoryLimit <= 0 || c.MakerMinHalfSpreadTicks <= 0 || c.RoundTripTraderCount < 0 || c.RoundTripHold <= 0 || c.RoundTripLotQty <= 0 || c.RoundTripInventoryLots <= 0 || c.ElasticSupplierCount < 0 || c.ElasticSupplierUnitsPerPercent <= 0 || c.CarryArbitrageurCount < 0 ||
		c.CarryEntryBps <= 0 || c.CarryExitBps < 0 || c.CarryMaxPosition <= 0 || c.CarryLotQty <= 0 || c.MakerQuoteQty <= 0 || c.SpotMakerCount < 1 || c.OptionDealerCount < 1 || c.DatedCarryArbCount < 0 || c.ParityArbCount < 0 || c.FuturesMakerCount < 1 || c.FundingMaxRateBps <= 0 || c.FundingIntervalSeconds <= 0 || c.LatentLiquidityCount < 0 ||
		c.CrossVenueArbLotQty < 0 || c.CrossVenueArbMaxAttempts < 0 ||
		c.OptionIV <= 0 || c.StoikovRiskAversion <= 0 || c.StoikovFillDecay <= 0 || c.StoikovVariancePerSecond < 0 ||
		c.StoikovInventoryHorizon <= 0 || c.StoikovVolatilityHalfLife <= 0 || *c.OptionBuyProbability < 0 || *c.OptionBuyProbability > 1 {
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
	ID           string
	MatchingRule string
	Exchange     *exchange.Exchange
	Mount        *simulation.Mount
	// latencyMounts holds one mount per participant class, so that a class
	// with its own link reaches the same exchange through a delayed gateway
	// while the rest connect directly.
	latencyMounts map[string]*simulation.Mount
	// latencyMountOrder keeps the mounts in creation order. Draining them in
	// map order would make a run's event interleaving depend on Go's map
	// iteration, which is randomised per process and would destroy replay.
	latencyMountOrder []*simulation.Mount
	// registerMount hands a newly created latency mount to the runner at the
	// moment it exists. Registering them in a sweep after the venue was built
	// missed every mount created later — the execution desks and the routers
	// connect after that point — and a mount the runner does not know about
	// never has deterministic phases enabled, so its courier goroutines keep
	// running and the run stops being reproducible.
	registerMount        func(*simulation.Mount)
	latencyConfig        Config
	latencySeed          int64
	scheduler            *simulation.EventScheduler
	clock                *simulation.SimulatedClock
	Participants         []Participant
	RequestPolicy        *exchange.TieredRequestPolicy
	SpotMakers           []*StoikovMarketMaker
	PerpMaker            *StoikovMarketMaker
	FuturesMaker         *derivsim.FuturesMarketMaker
	FuturesMakers        []*derivsim.FuturesMarketMaker
	OptionDealer         *derivsim.OptionMarketMaker
	OptionDealers        []*derivsim.OptionMarketMaker
	FixedDistanceMakers  []*FixedDistanceMaker
	ImbalanceMakers      []*ImbalanceMaker
	TriangleArbs         []*TriangleArbTaker
	BootstrapDepths      []*BootstrapDepth
	DatedCarryArbs       []*derivsim.CashCarryArb
	ParityArbs           []*derivsim.ParityArb
	OptionDealerClientID uint64
	// Singular fields retain the baseline participant for callers written
	// before configurable rosters. All actors live in the corresponding slice.
	makerStateLog     venueLogger
	NoiseTrader       *feesim.RandomTaker
	NoiseTraders      []*feesim.RandomTaker
	RoundTripTraders  []*RoundTripTrader
	Suppliers         []*ElasticSupplier
	CarryArbs         []*CarryArbitrageur
	LatentLiquidity   []*LatentLiquidity
	MetaorderTraders  []*MetaorderTrader
	lastTwoSided      map[string]twoSidedMark
	Microstructure    *MicrostructureStats
	OptionFlow        *derivsim.OptionTaker
	OptionFlows       []*derivsim.OptionTaker
	OptionValueTakers []*derivsim.OptionValueTaker
	FutureFlows       []*derivsim.OptionTaker
	VannaVolgaDesks   []*derivsim.VannaVolgaHedger
	InitialRisk       *VenueRiskSnapshot
	RiskTimeline      []VenueRiskSnapshot
	PreExpiryRisk     []VenueRiskSnapshot
	TerminalRisk      *VenueRiskSnapshot
	riskErr           error
	riskLastNano      int64
	optionListedNano  map[string]int64
	nextClient        uint64
}

// Participant identifies one independently funded account. It is recorded by
// the simulation controller and is never exposed to an actor as opponent
// state.
type Participant struct {
	VenueID  string `json:"venue_id"`
	ClientID uint64 `json:"client_id"`
	Role     string `json:"role"`
}

// ParticipantAccountSnapshot is a strict marked account for one participant
// at a lifecycle boundary. MarkSource makes the initial bootstrap valuation
// distinguishable from an executable terminal two-sided mark.
type ParticipantAccountSnapshot struct {
	Participant
	Phase      string `json:"phase"`
	MarkSource string `json:"mark_source"`
	// Marks are the asset prices this row was valued at, in report-asset units
	// per whole asset. Without them a reader cannot separate trading result
	// from inventory revaluation: every participant here is net long the base
	// asset, so a price rise lifts all of their marked equity at once.
	Marks   map[string]int64             `json:"marks"`
	Account etypes.MarkedAccountSnapshot `json:"account"`
}

// fundingInterval reports the funding period a venue settles on, in seconds.
func (c Config) fundingInterval(venueID string) int64 {
	if rule, exists := c.VenueRules[venueID]; exists && rule.FundingIntervalSeconds > 0 {
		return rule.FundingIntervalSeconds
	}
	return c.FundingIntervalSeconds
}

func (c Config) matchingRule(venueID string) string {
	if rule, exists := c.VenueRules[venueID]; exists && rule.MatchingRule != "" {
		return rule.MatchingRule
	}
	return MatchingPriceTime
}

// LatencyProfile describes one participant class's link to a venue.
//
// The model matters beyond its mean. A normal draw has no tail, so a
// participant modelled with one is never badly late; a lognormal link is late
// exactly as often as a measured one, and a spiky link is fast until it
// stalls. Those three populations behave differently under the same average.
type LatencyProfile struct {
	// Model is "constant", "uniform", "normal", "lognormal" or "spiky".
	Model string `json:"model"`
	// Delay is the constant delay, the median for lognormal, or the mean for
	// normal. Min and Max bound a uniform draw.
	Delay time.Duration `json:"delay"`
	Min   time.Duration `json:"min"`
	Max   time.Duration `json:"max"`
	// StdDev is the normal spread; Sigma is the lognormal one, which sets how
	// heavy the tail is rather than how wide the body is.
	StdDev time.Duration `json:"std_dev"`
	Sigma  float64       `json:"sigma"`
	// Cap truncates a lognormal tail, modelling a client that gives up rather
	// than waiting.
	Cap time.Duration `json:"cap"`
	// SpikeDelay and SpikeProbability describe a fast link that occasionally
	// stalls. A maker on one is picked off during exactly those stalls.
	SpikeDelay       time.Duration `json:"spike_delay"`
	SpikeProbability float64       `json:"spike_probability"`
	// ResponseDelay and MarketDataDelay scale the outbound path relative to
	// the inbound one. Zero uses the same provider for all three channels,
	// since a link is usually symmetric.
	ResponseScale   float64 `json:"response_scale"`
	MarketDataScale float64 `json:"market_data_scale"`
}

// zero reports whether the profile asks for no delay at all, in which case the
// participant is connected directly rather than through a delayed gateway.
func (p LatencyProfile) zero() bool {
	return p.Delay <= 0 && p.Min <= 0 && p.Max <= 0 && p.SpikeDelay <= 0
}

// provider builds one channel's latency source. Seed keeps runs reproducible
// across roles and venues.
func (p LatencyProfile) provider(seed int64, scale float64) simulation.LatencyProvider {
	if scale <= 0 {
		scale = 1
	}
	scaled := func(d time.Duration) time.Duration { return time.Duration(float64(d) * scale) }
	switch p.Model {
	case "", "constant":
		return simulation.NewConstantLatency(scaled(p.Delay))
	case "uniform":
		return simulation.NewUniformRandomLatency(scaled(p.Min), scaled(p.Max), seed)
	case "normal":
		return simulation.NewNormalLatency(scaled(p.Delay), scaled(p.StdDev), seed)
	case "lognormal":
		return simulation.NewLognormalLatency(scaled(p.Delay), p.Sigma, scaled(p.Cap), seed)
	case "spiky":
		return simulation.NewSpikyLatency(
			simulation.NewConstantLatency(scaled(p.Delay)),
			simulation.NewConstantLatency(scaled(p.SpikeDelay)),
			p.SpikeProbability, seed)
	}
	return nil
}

// validate refuses a profile that cannot be built.
func (p LatencyProfile) validate(role string) error {
	switch p.Model {
	case "", "constant", "normal", "lognormal", "spiky":
	case "uniform":
		if p.Max < p.Min {
			return fmt.Errorf("multivenue: latency profile %q has max below min", role)
		}
	default:
		return fmt.Errorf("multivenue: latency profile %q has unsupported model %q", role, p.Model)
	}
	if p.Delay < 0 || p.Min < 0 || p.Max < 0 || p.StdDev < 0 || p.Cap < 0 || p.SpikeDelay < 0 {
		return fmt.Errorf("multivenue: latency profile %q has a negative duration", role)
	}
	if p.SpikeProbability < 0 || p.SpikeProbability > 1 {
		return fmt.Errorf("multivenue: latency profile %q has spike probability %v outside [0,1]", role, p.SpikeProbability)
	}
	return nil
}

// latencyProfileFor resolves the profile a role connects under. Roles are
// suffixed with a participant number, so the lookup is on the prefix.
func (c Config) latencyProfileFor(role string) (LatencyProfile, bool) {
	if profile, exists := c.LatencyProfiles[roleClass(role)]; exists {
		return profile, true
	}
	if profile, exists := c.LatencyProfiles[role]; exists {
		return profile, true
	}
	if c.DefaultLatencyProfile != nil {
		return *c.DefaultLatencyProfile, true
	}
	return LatencyProfile{}, false
}

// roleClass strips the participant number a role is connected under.
func roleClass(role string) string {
	index := strings.LastIndex(role, "_")
	if index <= 0 {
		return role
	}
	suffix := role[index+1:]
	for _, digit := range suffix {
		if digit < '0' || digit > '9' {
			return role
		}
	}
	if suffix == "" {
		return role
	}
	return role[:index]
}

// makerSymbol picks the book the maker at index i quotes, cycling the roster.
func makerSymbol(symbols []string, i int) string {
	if len(symbols) == 0 {
		return "ABC/USD"
	}
	return symbols[i%len(symbols)]
}

// tickFor is the tick of one of the population's spot books. Every book a
// maker can be placed on has to resolve, or the maker quotes prices the
// exchange rejects.
func tickFor(symbol string, spotTick int64) int64 {
	switch symbol {
	case "CDF/USD":
		return int64(mvQuotePrecision)
	case "ABC/CDF":
		return int64(mvBasePrecision / 1_000)
	default:
		// The perpetual is listed at the spot tick.
		return spotTick
	}
}

// hedgePolicyFor selects the hedge the dealer at index i runs. Nil keeps the
// banded hedge the population ran before hedging was a strategy.
func (c Config) hedgePolicyFor(i int) derivsim.HedgePolicy {
	if len(c.OptionDealerHedgePolicies) == 0 {
		return nil
	}
	switch c.OptionDealerHedgePolicies[i%len(c.OptionDealerHedgePolicies)] {
	case "static":
		return derivsim.StaticDeltaHedge{}
	case "timed":
		return derivsim.TimedDeltaHedge{IntervalNanos: c.OptionDealerHedgeIntervalSeconds * int64(time.Second)}
	case "none":
		return derivsim.NoHedge{}
	case "banded":
		return derivsim.BandedDeltaHedge{}
	}
	return nil
}

// OptionDealerVolConfig describes a roster of volatility opinions.
//
// A population needs its option pricers to differ, and the honest way to
// differ is in what they estimate and what they charge for carrying the risk,
// not in a spread handed to each of them. Every participant built from this
// runs the same estimator on the same price path; they disagree because they
// forget at different rates and demand different premiums over what they
// measure.
type OptionDealerVolConfig struct {
	// Model is "flat", "realized" or "sabr". Empty keeps the single configured
	// level.
	//
	// A participant on "sabr" quotes a smile because its model has one. That
	// makes it useful as a counterparty that disagrees with a flat-volatility
	// dealer in a structured way, and useless as evidence that a population
	// produces a smile: any smile measured while it is enabled is an
	// assumption travelling through the book.
	Model string `json:"model"`
	// SABR carries the model's parameters when Model is "sabr". Alphas are
	// assigned to participants in order and cycled, like the half-lives.
	SABRAlphas []float64 `json:"sabr_alphas"`
	SABRBeta   float64   `json:"sabr_beta"`
	SABRRho    float64   `json:"sabr_rho"`
	SABRNu     float64   `json:"sabr_nu"`
	// HalfLifeSeconds and Premiums are assigned to participants in order and
	// cycled. A single entry gives every participant the same opinion, which
	// is the degenerate case worth being able to configure deliberately.
	HalfLifeSeconds []float64 `json:"half_life_seconds"`
	Premiums        []float64 `json:"premiums"`
	// Floor and Ceiling bound every estimate, so no participant quotes a zero
	// premium in a quiet stretch or an unbounded one after a jump.
	Floor   float64 `json:"floor"`
	Ceiling float64 `json:"ceiling"`
	// InventoryVegaAversion raises the volatility a participant quotes on the
	// strikes it is short, which is how a smile can arise from where the flow
	// went rather than from a parameter. Zero leaves the estimate alone.
	InventoryVegaAversion  float64 `json:"inventory_vega_aversion"`
	InventoryMaxAdjustment float64 `json:"inventory_max_adjustment"`
}

// validate refuses a volatility roster that cannot be built, rather than
// silently handing every participant the fallback.
func (c OptionDealerVolConfig) validate(field string) error {
	switch c.Model {
	case "", "flat", "realized":
	case "sabr":
		if c.SABRBeta < 0 || c.SABRBeta > 1 {
			return fmt.Errorf("multivenue: %s beta must lie in [0,1], got %v", field, c.SABRBeta)
		}
		if c.SABRRho <= -1 || c.SABRRho >= 1 {
			return fmt.Errorf("multivenue: %s rho must lie strictly inside (-1,1), got %v", field, c.SABRRho)
		}
		if c.SABRNu < 0 {
			return fmt.Errorf("multivenue: %s vol-of-vol must not be negative, got %v", field, c.SABRNu)
		}
		for _, alpha := range c.SABRAlphas {
			if alpha <= 0 {
				return fmt.Errorf("multivenue: %s alphas must be positive, got %v", field, alpha)
			}
		}
	default:
		return fmt.Errorf("multivenue: %s model must be flat, realized or sabr, got %q", field, c.Model)
	}
	for _, halfLife := range c.HalfLifeSeconds {
		if halfLife <= 0 {
			return fmt.Errorf("multivenue: %s half-lives must be positive, got %v", field, halfLife)
		}
	}
	for _, premium := range c.Premiums {
		if premium <= 0 {
			return fmt.Errorf("multivenue: %s premiums must be positive, got %v", field, premium)
		}
	}
	if c.Floor < 0 || c.Ceiling < 0 || (c.Ceiling > 0 && c.Floor > c.Ceiling) {
		return fmt.Errorf("multivenue: %s bounds are inconsistent: floor %v ceiling %v", field, c.Floor, c.Ceiling)
	}
	return nil
}

// modelFor builds the volatility model for the participant at index i.
func (c OptionDealerVolConfig) modelFor(i int, fallbackIV float64) eprice.VolatilityModel {
	switch c.Model {
	case "", "flat":
		if c.Model == "" {
			return nil
		}
		return eprice.FlatVolatility(fallbackIV)
	case "realized":
		halfLife := pickFloat(c.HalfLifeSeconds, i, 600)
		premium := pickFloat(c.Premiums, i, 1)
		return eprice.NewRealizedVolatility(fallbackIV, halfLife, premium, c.Floor, c.Ceiling)
	case "sabr":
		return eprice.SABRVolatility{
			Alpha: pickFloat(c.SABRAlphas, i, fallbackIV),
			Beta:  c.SABRBeta, Rho: c.SABRRho, Nu: c.SABRNu,
		}
	}
	return nil
}

// pickFloat cycles a roster, so a population with more participants than
// entries repeats opinions instead of failing to build.
func pickFloat(values []float64, i int, fallback float64) float64 {
	if len(values) == 0 {
		return fallback
	}
	return values[i%len(values)]
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
	Config           Config
	Runner           *simulation.Runner
	Venues           []*Venue
	Routers          []*CrossVenueArb
	SpotIndex        *spotIndexProvider
	InitialAccounts  []ParticipantAccountSnapshot
	TerminalAccounts []ParticipantAccountSnapshot
	loggers          []*feesim.JSONLinesLogger
	checkpoints      *checkpointSink
}

// Run starts all venue automation under one context and drives the common
// runner. Venue-local participants use direct mounts; optional cross-venue
// routers use the phase-owned scheduled courier for modeled latency.
func (s *Sim) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Request budgets are attached once every participant is connected, since a
	// tier is chosen from a participant's role.
	for _, venue := range s.Venues {
		if policy := buildRequestPolicy(s.Config.RateLimitTiers, venue.Participants); policy != nil {
			venue.Exchange.RequestPolicy = policy
			venue.RequestPolicy = policy
		}
	}
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
		if riskErr == nil && s.Config.StrictPopulationAccounting {
			s.TerminalAccounts, riskErr = s.capturePopulationAccounts("terminal_post_mark")
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
		venue.Microstructure.finalize()
	}
	for _, venue := range s.Venues {
		if venue.riskErr != nil {
			return venue.riskErr
		}
	}
	return riskErr
}

func (s *Sim) Close() {
	s.checkpoints.close()
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
	// sink observes every event for the divergence locator. It runs whatever
	// the log mode is, because a run with logging off still has to be
	// comparable against another run of the same seed.
	sink *checkpointSink
}

func (l venueLogger) LogEvent(simTime int64, clientID uint64, eventName string, event any) {
	l.sink.observe(simTime, clientID, eventName, l.venueID, event)
	if l.inner == nil {
		return
	}
	l.inner.LogEvent(simTime, clientID, eventName, venueLogEvent{VenueID: l.venueID, Payload: event})
}

type manifest struct {
	SchemaVersion int       `json:"schema_version"`
	VenueIDs      []string  `json:"venue_ids"`
	Config        Config    `json:"config"`
	Build         BuildInfo `json:"build"`
	Notes         []string  `json:"notes"`
}

// BuildInfo records which build produced a run. Three experiments in this
// campaign were run against a binary compiled before the fix under test, and
// each time the result was indistinguishable from a real null until something
// else exposed it. A run that cannot say which source it came from cannot be
// trusted after the fact.
type BuildInfo struct {
	Revision string `json:"revision"`
	Time     string `json:"time"`
	// Modified reports that the working tree had uncommitted changes when the
	// binary was built, so the revision alone does not identify the source.
	Modified bool `json:"modified"`
}

// currentBuild reads the version-control stamp Go embeds at build time.
func currentBuild() BuildInfo {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return BuildInfo{Revision: "unknown"}
	}
	build := BuildInfo{Revision: "unknown"}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			build.Revision = setting.Value
		case "vcs.time":
			build.Time = setting.Value
		case "vcs.modified":
			build.Modified = setting.Value == "true"
		}
	}
	return build
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
	routingNote := "No cross-venue routing, collateral transfer, or shared wallet is enabled."
	latencyNote := "Venue-local participants use direct deterministic mounts; no latency experiment is enabled."
	if len(cfg.CrossVenueArbTiers) > 0 {
		routingNote = "Cross-venue spot routers use independent prefunded accounts and non-atomic FOK legs; no collateral transfer or shared wallet exists."
		latencyNote = "Cross-venue router request, response, and market-data delay is delivered by the deterministic phase-owned courier."
	}
	manifestConfig := cfg
	// Output location is an artifact sink, not a market mechanism. Excluding it
	// makes a manifest hash identify the scenario across independently chosen
	// persistent log directories.
	manifestConfig.LogDir = ""
	manifestBytes, err := json.MarshalIndent(manifest{
		SchemaVersion: 2,
		VenueIDs:      slices.Clone(cfg.VenueIDs),
		Config:        manifestConfig,
		Build:         currentBuild(),
		Notes: []string{
			"Each venue has independent prefunded accounts and local spot-margin borrowing.",
			latencyNote,
			routingNote,
			"Option dealer begins with zero ABC; spot hedge sells borrow ABC against USD collateral when required.",
			"Noise and option-flow rosters are independently seeded per venue and participant index.",
			"cross_asset_spot_graph adds ABC/USD, CDF/USD, and ABC/CDF spot books; it does not add a triangular-arbitrage actor.",
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

	sim := &Sim{Config: cfg, Runner: runner,
		SpotIndex: newSpotIndexProvider(cfg.MakerAnchor, "ABC/USD", "ABC-PERP", "CDF/USD", "ABC/CDF"), Venues: make([]*Venue, 0, len(cfg.VenueIDs))}
	// Built before any venue, because every venue logger carries it.
	sink, err := newCheckpointSink(cfg.LogDir, cfg.CheckpointIntervalSeconds, cfg.TraceFromNano, cfg.TraceToNano)
	if err != nil {
		return nil, err
	}
	sim.checkpoints = sink
	// Seed the reference before the first quote: until the first automation
	// tick the index would otherwise be empty, leaving makers to fall back to
	// their own midpoint exactly when the book is thinnest.
	actorID := uint64(0)
	for venueIndex, id := range cfg.VenueIDs {
		venue, err := sim.addVenue(id, venueIndex, clock, scheduler, timers, &actorID)
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
		// Every class with its own link reaches the exchange through a mount of
		// its own, and a mount the runner does not know about never has its
		// egress drained nor its deterministic phases enabled. The mounts
		// created while this venue was being built are registered here; the
		// ones created later, when the execution desks and routers connect,
		// register themselves through the hook set below.
		for _, mount := range venue.latencyMountOrder {
			runner.AddMount(mount)
		}
		venue.registerMount = runner.AddMount
		for _, maker := range venue.SpotMakers {
			runner.AddActor(maker)
		}
		runner.AddActor(venue.PerpMaker)
		for _, maker := range venue.FuturesMakers {
			runner.AddActor(maker)
		}
		for _, maker := range venue.FixedDistanceMakers {
			runner.AddActor(maker)
		}
		for _, maker := range venue.ImbalanceMakers {
			runner.AddActor(maker)
		}
		for _, desk := range venue.TriangleArbs {
			runner.AddActor(desk)
		}
		for _, ladder := range venue.BootstrapDepths {
			runner.AddActor(ladder)
		}
		for _, desk := range venue.DatedCarryArbs {
			runner.AddActor(desk)
		}
		for _, desk := range venue.ParityArbs {
			runner.AddActor(desk)
		}
		for _, dealer := range venue.OptionDealers {
			runner.AddActor(dealer)
		}
		for _, desk := range venue.VannaVolgaDesks {
			runner.AddActor(desk)
		}
		for _, taker := range venue.OptionValueTakers {
			runner.AddActor(taker)
		}
		for _, flow := range venue.FutureFlows {
			runner.AddActor(flow)
		}
		for _, noise := range venue.NoiseTraders {
			runner.AddActor(noise)
		}
		for _, flow := range venue.OptionFlows {
			runner.AddActor(flow)
		}

		for _, trader := range venue.RoundTripTraders {
			runner.AddActor(trader)
		}
		for _, supplier := range venue.Suppliers {
			runner.AddActor(supplier)
		}
		for _, arb := range venue.CarryArbs {
			runner.AddActor(arb)
		}
		for _, latent := range venue.LatentLiquidity {
			runner.AddActor(latent)
		}
	}
	if err := sim.addMetaorderTraders(timers, &actorID); err != nil {
		sim.Close()
		return nil, err
	}
	if err := sim.addCrossVenueRouters(clock, scheduler, timers, &actorID); err != nil {
		sim.Close()
		return nil, err
	}
	if cfg.StrictPopulationAccounting {
		sim.InitialAccounts, err = sim.capturePopulationAccounts("initial")
		if err != nil {
			sim.Close()
			return nil, err
		}
	}
	return sim, nil
}

func (s *Sim) addVenue(id string, venueIndex int, clock *simulation.SimulatedClock, scheduler *simulation.EventScheduler, timers *simulation.SimTimerFactory, actorID *uint64) (*Venue, error) {
	logDir := filepath.Join(s.Config.LogDir, "venues", id)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}
	// With raw persistence off the wrapper is still installed, carrying only
	// the divergence sink: a run that writes no logs must still be comparable
	// against another run of the same seed, and that comparison is the whole
	// point of running with logs off.
	newLogger := func(name string) (venueLogger, error) {
		if s.Config.LogMode != "full" {
			return venueLogger{venueID: id, sink: s.checkpoints}, nil
		}
		logger, err := feesim.NewJSONLinesLogger(filepath.Join(logDir, name))
		if err != nil {
			return venueLogger{}, err
		}
		s.loggers = append(s.loggers, logger)
		return venueLogger{venueID: id, inner: logger, sink: s.checkpoints}, nil
	}
	estimatedClients := 5 + s.Config.NoiseTraderCount + s.Config.OptionFlowCount + len(s.Config.CrossVenueArbTiers)
	if s.Config.CrossAssetSpotGraph {
		estimatedClients += 4
	}
	ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		ID:                      id,
		EstimatedClients:        estimatedClients,
		Clock:                   clock,
		TickerFactory:           timers,
		DeterministicIngress:    true,
		DeterministicPhases:     true,
		SnapshotInterval:        s.Config.SnapshotInterval,
		BalanceSnapshotInterval: time.Minute,
	})
	matchingRule := s.Config.matchingRule(id)
	if matchingRule == MatchingProRata {
		ex.Matcher = matching.NewProRataMatcher(clock)
	}
	var makerStateLog venueLogger
	if s.Config.LogMode == "full" || s.checkpoints != nil {
		if s.Config.LogMode == "full" {
			if err := os.MkdirAll(filepath.Join(logDir, "spot"), 0755); err != nil {
				return nil, err
			}
		}
		globalLog, err := newLogger("general.jsonl")
		if err != nil {
			return nil, err
		}
		makerStateLog = venueLogger{inner: globalLog, venueID: id, sink: s.checkpoints}
		derivativeLog, err := newLogger("derivatives.jsonl")
		if err != nil {
			return nil, err
		}
		ex.SetLogger("_global", globalLog)
		spotSymbols := []string{"ABC/USD"}
		if s.Config.CrossAssetSpotGraph {
			spotSymbols = append(spotSymbols, "CDF/USD", "ABC/CDF")
		}
		for _, symbol := range spotSymbols {
			spotLog, err := newLogger(filepath.Join("spot", strings.ReplaceAll(symbol, "/", "-")+".jsonl"))
			if err != nil {
				return nil, err
			}
			ex.SetLogger(symbol, spotLog)
		}
		ex.SetInstrumentLoggerFallback(derivativeLog)
	}

	tick := s.Config.SpotTickQuoteUnits
	spot := exchange.NewSpotInstrument("ABC/USD", "ABC", "USD", mvBasePrecision, mvQuotePrecision, tick, mvBasePrecision/1_000)
	perp := exchange.NewPerpFutures("ABC-PERP", "ABC", "USD", mvBasePrecision, mvQuotePrecision, tick, mvBasePrecision/1_000)
	perp.GetFundingRate().Interval = s.Config.fundingInterval(id)
	perp.SetFundingCalculator(&instrument.SimpleFundingCalc{
		BaseRate: 1, Damping: 100, MaxRate: s.Config.FundingMaxRateBps,
	})
	ex.AddInstrument(spot)
	ex.AddInstrument(perp)
	if s.Config.CrossAssetSpotGraph {
		cdfTick := int64(mvQuotePrecision)
		crossTick := int64(mvBasePrecision / 1_000)
		ex.AddInstrument(exchange.NewSpotInstrument("CDF/USD", "CDF", "USD", mvBasePrecision, mvQuotePrecision, cdfTick, mvBasePrecision/1_000))
		ex.AddInstrument(exchange.NewSpotInstrument("ABC/CDF", "ABC", "CDF", mvBasePrecision, mvBasePrecision, crossTick, mvBasePrecision/1_000))
	}

	index := exchange.NewMidPriceOracle(ex)
	index.MapSymbol("ABC-PERP", "ABC/USD")
	spec := instrument.ContractSpec{
		Base: "ABC", Quote: "USD", BasePrecision: mvBasePrecision, QuotePrecision: mvQuotePrecision,
		TickSize: tick, MinOrderSize: mvBasePrecision / 1_000,
	}
	optionSpec := spec
	optionSpec.TickSize = mvQuotePrecision // one USD premium tick
	mount := simulation.NewMount(ex, simulation.LatencyConfig{})
	venue := &Venue{ID: id, MatchingRule: matchingRule, Exchange: ex, Mount: mount,
		latencyMounts:    make(map[string]*simulation.Mount),
		latencyConfig:    s.Config,
		latencySeed:      s.Config.Seed + int64(venueIndex+1)*1_000,
		scheduler:        scheduler,
		clock:            clock,
		makerStateLog:    makerStateLog,
		optionListedNano: make(map[string]int64),
		Microstructure:   newMicrostructureStats(id, "ABC/USD", tick, s.Config.AutomationInterval.Seconds())}
	// The venue advertises the scenario's reference price while still marking
	// its own derivatives from its own book. With own_mid anchoring there is
	// nothing to advertise: the makers already observe the book directly.
	var indexFeed PriceSource = s.SpotIndex
	if s.Config.DegradedIndex != nil {
		indexFeed = newDegradedIndex(s.SpotIndex, *s.Config.DegradedIndex)
	}
	indexFeedSymbols := []string(nil)
	if s.Config.MakerAnchor != "own_mid" {
		indexFeedSymbols = []string{"ABC/USD", "ABC-PERP"}
		if s.Config.CrossAssetSpotGraph {
			indexFeedSymbols = append(indexFeedSymbols, "CDF/USD", "ABC/CDF")
		}
	}
	ex.ConfigureAutomation(exchange.AutomationConfig{
		IndexProvider:       index,
		IndexFeedSymbols:    indexFeedSymbols,
		IndexFeedProvider:   indexFeed,
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
			now := venue.Exchange.Clock.NowUnixNano()
			venue.recordTwoSidedMarks(valuedSpotSymbols(s.Config.CrossAssetSpotGraph), now)
			for _, symbol := range []string{"ABC/USD", "ABC-PERP", "CDF/USD", "ABC/CDF"} {
				if mid, ok := venue.Exchange.TwoSidedMidPrice(symbol); ok {
					s.SpotIndex.observeVenueMid(symbol, venue.ID, mid)
				}
			}
			venue.verifyConservation(now)
			venue.logMakerState(now)
			venue.observeMicrostructure()
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

	connect := func(role string, balances map[string]int64, perpUSD int64, fee exchange.FeeModel) actor.Gateway {
		_, gw := venue.connectParticipant(mount, role, balances, perpUSD, fee)
		return gw
	}
	mmBalances := map[string]int64{"ABC": 10_000 * mvBasePrecision, "USD": 500_000_000 * mvQuotePrecision}
	if s.Config.CrossAssetSpotGraph {
		mmBalances["CDF"] = 10_000 * mvBasePrecision
	}
	zeroFee := &exchange.FixedFee{}
	nextActor := func() uint64 {
		*actorID++
		return *actorID
	}
	// The configured Stoikov parameters are USD-denominated at the ABC/USD
	// reference price. Convert them once into the scale-free form the control
	// law uses, so every book — including one quoted in CDF — is described by
	// the same relative parameters.
	referencePrice := float64(mvBootstrapPrice) / float64(mvQuotePrecision)
	relativeRiskAversion := s.Config.StoikovRiskAversion * referencePrice
	relativeFillDecay := s.Config.StoikovFillDecay * referencePrice
	relativeLogVariance := s.Config.StoikovVariancePerSecond / (referencePrice * referencePrice)
	// baseValueScale converts a quantity expressed in ABC into the quantity of
	// another book's base asset carrying the same value.
	//
	// The risk parameters below are already scale free, but quote size and the
	// inventory limit were passed through as raw base units to every book. The
	// base assets differ in price by a factor of sixteen, so the same number
	// quoted a book with a twenty-eight times thinner book in value terms:
	// measured median depth was 848,935 on ABC/USD against 29,840 on CDF/USD.
	// The thin book is the one that dislocated, peaking 763% above its opening
	// price while ABC/USD fell 18%.
	baseValueScale := func(baseUSDPrice int64) int64 {
		if baseUSDPrice <= 0 {
			return 1
		}
		return baseUSDPrice
	}
	scaleQty := func(qty, baseUSDPrice int64) int64 {
		scaled := etypes.MulDiv(qty, int64(mvBootstrapPrice), baseValueScale(baseUSDPrice))
		if scaled <= 0 {
			return qty
		}
		return scaled
	}
	stoikovConfig := func(symbol, reference string, bootstrapPrice, quotePrecision, tickSize int64) StoikovMMConfig {
		// The base asset's USD price, which is what quote size and inventory
		// have to be denominated against. ABC/CDF is quoted in CDF but its base
		// is still ABC, so it scales like the ABC books.
		baseUSD := int64(mvBootstrapPrice)
		if symbol == "CDF/USD" {
			baseUSD = int64(mvCDFBootstrap)
		}
		return StoikovMMConfig{
			Symbol: symbol, ReferenceSymbol: reference, BootstrapPrice: bootstrapPrice,
			BasePrecision: mvBasePrecision, QuotePrecision: quotePrecision, TickSize: tickSize,
			QuoteQty:      scaleQty(s.Config.MakerQuoteQty, baseUSD),
			QuoteInterval: s.Config.QuoteInterval, VolatilityHalfLife: s.Config.StoikovVolatilityHalfLife,
			InitialLogVariancePerSec: relativeLogVariance,
			MaxLogVarianceMultiple:   s.Config.StoikovMaxVarianceMultiple,
			VolatilitySampleInterval: s.Config.StoikovVolatilitySampleInterval,
			InventoryHorizon:         s.Config.StoikovInventoryHorizon,
			RelativeRiskAversion:     relativeRiskAversion,
			RelativeFillDecay:        relativeFillDecay,
			MinHalfSpreadTicks:       s.Config.MakerMinHalfSpreadTicks,
			ForwardHalfLife:          s.Config.MakerForwardHalfLife,
			QuoteSizeVolElasticity:   s.Config.MakerQuoteSizeVolElasticity,
			MinQuoteSizeFraction:     s.Config.MakerMinQuoteSizeFraction,
			InventoryLimit:           scaleQty(s.Config.MakerInventoryLimit, baseUSD),
			InventorySkewBps:         s.Config.MakerInventorySkewBps,
			SubmitBeforeCancel:       s.Config.SpotMakerSubmitBeforeCancel,
			RequoteBps:               s.Config.SpotMakerRequoteBps,
			HedgeSymbol:              hedgeSymbol(symbol, s.Config.MakerHedgeSymbol),
			HedgeBandQty:             s.Config.MakerHedgeBandQty,
			HedgeSlippageBps:         s.Config.MakerHedgeSlippageBps,
			HedgeInterval:            s.Config.MakerHedgeInterval,
			HedgeTickSize:            tick,
			AnchorToIndex:            s.Config.MakerAnchor != "own_mid",
			IndexWeight:              s.Config.MakerIndexWeight,
		}
	}
	for i := 0; i < s.Config.SpotMakerCount; i++ {
		makerConfig := stoikovConfig("ABC/USD", "ABC/USD", mvBootstrapPrice, mvQuotePrecision, tick)
		makerConfig.RequoteBps = requoteThresholdFor(s.Config.SpotMakerRequoteBpsTiers, s.Config.SpotMakerRequoteBps, i)
		maker := NewStoikovMarketMaker(nextActor(), connect(fmt.Sprintf("spot_maker_%d", i+1), mmBalances, 100_000_000*mvQuotePrecision, zeroFee), makerConfig)
		maker.SetTickerFactory(timers)
		venue.SpotMakers = append(venue.SpotMakers, maker)
	}
	if s.Config.CrossAssetSpotGraph {
		cdfTick := int64(mvQuotePrecision)
		crossTick := int64(mvBasePrecision / 1_000)
		crossBootstrap := etypes.MulDiv(mvBootstrapPrice, mvBasePrecision, mvCDFBootstrap)
		for i := 0; i < 2; i++ {
			cdfMaker := NewStoikovMarketMaker(nextActor(), connect(fmt.Sprintf("cdf_spot_maker_%d", i+1), mmBalances, 100_000_000*mvQuotePrecision, zeroFee), stoikovConfig("CDF/USD", "CDF/USD", mvCDFBootstrap, mvQuotePrecision, cdfTick))
			cdfMaker.SetTickerFactory(timers)
			venue.SpotMakers = append(venue.SpotMakers, cdfMaker)
			crossMaker := NewStoikovMarketMaker(nextActor(), connect(fmt.Sprintf("abc_cdf_spot_maker_%d", i+1), mmBalances, 100_000_000*mvQuotePrecision, zeroFee), stoikovConfig("ABC/CDF", "ABC/CDF", crossBootstrap, mvBasePrecision, crossTick))
			crossMaker.SetTickerFactory(timers)
			venue.SpotMakers = append(venue.SpotMakers, crossMaker)
		}
	}
	perpMakerConfig := stoikovConfig("ABC-PERP", "ABC-PERP", mvBootstrapPrice, mvQuotePrecision, tick)
	if s.Config.PerpMakerInventoryLimit > 0 {
		perpMakerConfig.InventoryLimit = s.Config.PerpMakerInventoryLimit
	}
	venue.PerpMaker = NewStoikovMarketMaker(nextActor(), connect("perp_maker", mmBalances, 100_000_000*mvQuotePrecision, zeroFee), perpMakerConfig)
	venue.PerpMaker.SetTickerFactory(timers)

	for i := 0; i < s.Config.FuturesMakerCount; i++ {
		futuresMaker := derivsim.NewFuturesMarketMaker(nextActor(), connect(fmt.Sprintf("futures_maker_%d", i+1), mmBalances, 100_000_000*mvQuotePrecision, zeroFee), derivsim.FuturesMMConfig{
			Underlying: "ABC/USD", SpreadBps: 6, QuoteQty: mvBasePrecision / 5, Tick: tick, QuoteInterval: s.Config.QuoteInterval,
			SelfAnchored: s.Config.FuturesMakerSelfAnchored,
		})
		futuresMaker.SetTickerFactory(timers)
		venue.FuturesMakers = append(venue.FuturesMakers, futuresMaker)
	}
	venue.FuturesMaker = venue.FuturesMakers[0]
	dealerBalances := map[string]int64{"USD": 500_000_000 * mvQuotePrecision}
	for i := 0; i < s.Config.OptionDealerCount; i++ {
		dealerClientID, dealerGateway := venue.connectParticipant(mount, fmt.Sprintf("option_dealer_%d", i+1), dealerBalances, 150_000_000*mvQuotePrecision, zeroFee)
		if i == 0 {
			venue.OptionDealerClientID = dealerClientID
		}
		dealer := derivsim.NewOptionMarketMaker(nextActor(), dealerGateway, derivsim.OptionMMConfig{
			Underlying: "ABC/USD", IV: s.Config.OptionIV, SpreadBps: 30, SkewPerLotBps: 5,
			VolModel: s.Config.OptionDealerVol.modelFor(i, s.Config.OptionIV),
			QuoteQty: mvBasePrecision / 10, LotQty: mvBasePrecision / 20, PremiumTick: mvQuotePrecision,
			QuoteInterval: s.Config.QuoteInterval, HedgeEnabled: s.Config.DealerHedgeMode == "on", HedgeInterval: s.Config.QuoteInterval,
			HedgePolicy:  s.Config.hedgePolicyFor(i),
			HedgeBandQty: mvBasePrecision / 100, GreekInterval: s.Config.GreekInterval, BasePrecision: mvBasePrecision,
		})
		dealer.SetTickerFactory(timers)
		venue.OptionDealers = append(venue.OptionDealers, dealer)
	}
	venue.OptionDealer = venue.OptionDealers[0]

	for participant := 0; participant < s.Config.VannaVolgaDeskCount; participant++ {
		// Each desk takes one dealer's book, cycling if there are more desks
		// than dealers, so the risk being laid off belongs to somebody.
		dealer := venue.OptionDealers[participant%len(venue.OptionDealers)]
		desk := derivsim.NewVannaVolgaHedger(nextActor(),
			connect(fmt.Sprintf("vanna_volga_desk_%d", participant+1), dealerBalances, 100_000_000*mvQuotePrecision, zeroFee),
			derivsim.VannaVolgaHedgerConfig{
				Underlying:     "ABC/USD",
				VolModel:       s.Config.VannaVolgaVol.modelFor(participant, s.Config.OptionIV),
				VegaTolerance:  s.Config.VannaVolgaVegaTolerance,
				VannaTolerance: s.Config.VannaVolgaVannaTolerance,
				VolgaTolerance: s.Config.VannaVolgaVolgaTolerance,
				LotQty:         s.Config.VannaVolgaLotQty,
				MaxContracts:   s.Config.VannaVolgaMaxContracts,
				Interval:       s.Config.VannaVolgaInterval,
				BasePrecision:  mvBasePrecision,
				Exposure:       dealer.Exposures,
			})
		desk.SetTickerFactory(timers)
		venue.VannaVolgaDesks = append(venue.VannaVolgaDesks, desk)
	}

	noiseQty := s.Config.NoiseOrderQty
	if noiseQty <= 0 {
		noiseQty = mvBasePrecision / 100
	}
	// Uninformed flow has to be funded for the size it trades, or it stops being
	// uninformed: a taker that cannot afford to buy is rejected on its buys and
	// filled on its sells, and the population's "random" flow acquires a
	// direction. Measured at 0.5 ABC orders against the previous 100 USD
	// balance, noise flow ran 5.2% net short at 43 standard deviations with
	// 12,671 insufficient-balance rejections, and the makers absorbing that
	// one-sided flow drove a 17% price decline.
	noiseFunding := s.Config.NoiseFundingLots
	if noiseFunding <= 0 {
		noiseFunding = 200
	}
	noiseBase := noiseQty * noiseFunding
	noiseQuote := noiseQty * mvBootstrapPrice / mvBasePrecision * noiseFunding
	noiseBalances := map[string]int64{"ABC": noiseBase, "USD": noiseQuote}
	noiseSymbols := []string{"ABC/USD", "ABC-PERP"}
	noiseTargetQtys := map[string]int64{"ABC/USD": noiseQty, "ABC-PERP": noiseQty}
	if s.Config.CrossAssetSpotGraph {
		noiseBalances["CDF"] = noiseBase
		noiseSymbols = append(noiseSymbols, "CDF/USD", "ABC/CDF")
		noiseTargetQtys["CDF/USD"] = noiseQty
		noiseTargetQtys["ABC/CDF"] = noiseQty
	}
	for symbol, qty := range s.Config.NoiseTargetQtyBySymbol {
		if _, quoted := noiseTargetQtys[symbol]; quoted && qty > 0 {
			noiseTargetQtys[symbol] = qty
		}
	}
	noiseFee := &exchange.PercentageFee{MakerBps: 0, TakerBps: s.Config.TakerFeeBps, InQuote: true}
	for participant := 0; participant < s.Config.NoiseTraderCount; participant++ {
		noise := feesim.NewRandomTaker(nextActor(), connect(fmt.Sprintf("noise_flow_%d", participant+1), noiseBalances, 10_000_000*mvQuotePrecision, noiseFee), feesim.TakerConfig{
			Symbols: noiseSymbols, TargetQtys: noiseTargetQtys,
			TakeInterval: s.Config.NoiseInterval, Seed: flowSeed(s.Config.Seed, venueIndex, participant, 1),
			SizeParetoAlpha:   s.Config.NoiseSizeParetoAlpha,
			SizeCapMultiple:   s.Config.NoiseSizeCapMultiple,
			ImbalanceCoupling: s.Config.NoiseImbalanceCoupling,
			ExciteAlpha:       s.Config.NoiseExciteAlpha,
			ExciteBetaPerSec:  s.Config.NoiseExciteBetaPerSec,
		})
		noise.SetTickerFactory(timers)
		venue.NoiseTraders = append(venue.NoiseTraders, noise)
	}
	venue.NoiseTrader = venue.NoiseTraders[0]
	for participant := 0; participant < s.Config.OptionFlowCount; participant++ {
		flow := derivsim.NewOptionTaker(nextActor(), connect(fmt.Sprintf("option_flow_%d", participant+1), noiseBalances, 10_000_000*mvQuotePrecision, noiseFee), derivsim.OptionTakerConfig{
			Underlying: "ABC/USD", PBuy: *s.Config.OptionBuyProbability, LotQty: mvBasePrecision / 100,
			Interval: s.Config.NoiseInterval, Seed: flowSeed(s.Config.Seed, venueIndex, participant, 2), IncludeFutures: true,
		})
		flow.SetTickerFactory(timers)
		venue.OptionFlows = append(venue.OptionFlows, flow)
	}
	venue.OptionFlow = venue.OptionFlows[0]

	futureLot := s.Config.FutureFlowLotQty
	if futureLot <= 0 {
		futureLot = mvBasePrecision / 100
	}
	futureInterval := s.Config.FutureFlowInterval
	if futureInterval <= 0 {
		futureInterval = s.Config.NoiseInterval
	}
	for participant := 0; participant < s.Config.FutureFlowCount; participant++ {
		flow := derivsim.NewOptionTaker(nextActor(),
			connect(fmt.Sprintf("future_flow_%d", participant+1), noiseBalances, 10_000_000*mvQuotePrecision, noiseFee),
			derivsim.OptionTakerConfig{
				Underlying: "ABC/USD", PBuy: *s.Config.OptionBuyProbability, LotQty: futureLot,
				Interval: futureInterval, Seed: flowSeed(s.Config.Seed, venueIndex, participant, 5),
				ContractTypes: []string{"FUTURE"},
			})
		flow.SetTickerFactory(timers)
		venue.FutureFlows = append(venue.FutureFlows, flow)
	}

	for participant := 0; participant < s.Config.OptionValueTakerCount; participant++ {
		taker := derivsim.NewOptionValueTaker(nextActor(),
			connect(fmt.Sprintf("option_value_taker_%d", participant+1), noiseBalances, 20_000_000*mvQuotePrecision, noiseFee),
			derivsim.OptionValueTakerConfig{
				Underlying:    "ABC/USD",
				VolModel:      s.Config.OptionValueTakerVol.modelFor(participant, s.Config.OptionIV),
				EdgeBps:       s.Config.OptionValueTakerEdgeBps,
				LotQty:        s.Config.OptionValueTakerLotQty,
				MaxPosition:   s.Config.OptionValueTakerMaxPosition,
				Interval:      s.Config.OptionValueTakerInterval,
				BasePrecision: mvBasePrecision,
			})
		taker.SetTickerFactory(timers)
		venue.OptionValueTakers = append(venue.OptionValueTakers, taker)
	}

	naiveBalances := map[string]int64{"ABC": 2_000 * mvBasePrecision, "USD": 200_000_000 * mvQuotePrecision}
	if s.Config.CrossAssetSpotGraph {
		naiveBalances["CDF"] = 20_000 * mvBasePrecision
	}
	for participant := 0; participant < s.Config.FixedDistanceMakerCount; participant++ {
		cfg := FixedDistanceMakerConfig{SpreadBps: 8, RequoteBps: 4, QuoteQty: mvBasePrecision / 4, MaxInventory: 200 * mvBasePrecision}
		if s.Config.FixedDistanceMaker != nil {
			cfg = *s.Config.FixedDistanceMaker
		}
		cfg.Symbol = makerSymbol(s.Config.FixedDistanceMakerSymbols, participant)
		cfg.TickSize = tickFor(cfg.Symbol, tick)
		if cfg.QuoteInterval <= 0 {
			cfg.QuoteInterval = s.Config.QuoteInterval
		}
		maker := NewFixedDistanceMaker(nextActor(), connect(fmt.Sprintf("fixed_distance_maker_%d", participant+1), naiveBalances, 10_000_000*mvQuotePrecision, zeroFee), cfg)
		maker.SetTickerFactory(timers)
		venue.FixedDistanceMakers = append(venue.FixedDistanceMakers, maker)
	}
	for participant := 0; participant < s.Config.ImbalanceMakerCount; participant++ {
		cfg := ImbalanceMakerConfig{FixedDistanceMakerConfig: FixedDistanceMakerConfig{SpreadBps: 8, RequoteBps: 4, QuoteQty: mvBasePrecision / 4, MaxInventory: 200 * mvBasePrecision}, LeanBps: 4}
		if s.Config.ImbalanceMaker != nil {
			cfg = *s.Config.ImbalanceMaker
		}
		cfg.Symbol = makerSymbol(s.Config.ImbalanceMakerSymbols, participant)
		cfg.TickSize = tickFor(cfg.Symbol, tick)
		if cfg.QuoteInterval <= 0 {
			cfg.QuoteInterval = s.Config.QuoteInterval
		}
		maker := NewImbalanceMaker(nextActor(), connect(fmt.Sprintf("imbalance_maker_%d", participant+1), naiveBalances, 10_000_000*mvQuotePrecision, zeroFee), cfg)
		maker.SetTickerFactory(timers)
		venue.ImbalanceMakers = append(venue.ImbalanceMakers, maker)
	}
	if s.Config.CrossAssetSpotGraph {
		for participant := 0; participant < s.Config.TriangleArbCount; participant++ {
			cfg := TriangleArbConfig{EdgeBps: 20, LotQty: mvBasePrecision / 20}
			if s.Config.TriangleArb != nil {
				cfg = *s.Config.TriangleArb
			}
			cfg.BaseQuote, cfg.CrossQuote, cfg.BaseCross = "ABC/USD", "CDF/USD", "ABC/CDF"
			// The desk pays the venue's taker fee on every leg it crosses, so
			// its entry test has to know what the venue charges.
			if cfg.TakerFeeBps == 0 {
				cfg.TakerFeeBps = s.Config.TakerFeeBps
			}
			if cfg.CheckInterval <= 0 {
				cfg.CheckInterval = s.Config.QuoteInterval
			}
			desk := NewTriangleArbTaker(nextActor(), connect(fmt.Sprintf("triangle_arb_%d", participant+1), naiveBalances, 10_000_000*mvQuotePrecision, noiseFee), cfg)
			desk.SetTickerFactory(timers)
			venue.TriangleArbs = append(venue.TriangleArbs, desk)
		}
	}

	depthBalances := map[string]int64{"ABC": 200_000 * mvBasePrecision, "USD": 20_000_000_000 * mvQuotePrecision}
	for participant := 0; participant < s.Config.BootstrapDepthCount; participant++ {
		cfg := BootstrapDepthConfig{Levels: 10, QtyPerLevel: mvBasePrecision, SpacingBps: 5}
		if s.Config.BootstrapDepth != nil {
			cfg = *s.Config.BootstrapDepth
		}
		if cfg.Symbol == "" {
			cfg.Symbol = "ABC/USD"
		}
		if cfg.Interval <= 0 {
			cfg.Interval = s.Config.QuoteInterval
		}
		cfg.TickSize = tick
		ladder := NewBootstrapDepth(nextActor(), connect(fmt.Sprintf("bootstrap_depth_%d", participant+1), depthBalances, 0, zeroFee), cfg)
		ladder.SetTickerFactory(timers)
		venue.BootstrapDepths = append(venue.BootstrapDepths, ladder)
	}

	datedCarryInterval := s.Config.DatedCarryCheckInterval
	if datedCarryInterval <= 0 {
		datedCarryInterval = s.Config.QuoteInterval
	}
	derivArbBalances := map[string]int64{"ABC": 5_000 * mvBasePrecision, "USD": 500_000_000 * mvQuotePrecision}
	carryTenor := int64(0)
	if s.Config.DatedCarryScaleEdge {
		carryTenor = s.Config.ShortFutureTenor.Nanoseconds()
	}
	for participant := 0; participant < s.Config.DatedCarryArbCount; participant++ {
		desk := derivsim.NewCashCarryArb(nextActor(), connect(fmt.Sprintf("dated_carry_arb_%d", participant+1), derivArbBalances, 100_000_000*mvQuotePrecision, zeroFee), derivsim.CarryArbConfig{
			Underlying: "ABC/USD", EdgeBps: s.Config.DatedCarryEdgeBps, LotQty: mvBasePrecision / 10,
			MaxPosPerSym: 5 * mvBasePrecision, CheckInterval: datedCarryInterval, TenorNano: carryTenor,
			TickSize: tick, SlippageBps: s.Config.DatedCarrySlippageBps,
		})
		desk.SetTickerFactory(timers)
		venue.DatedCarryArbs = append(venue.DatedCarryArbs, desk)
	}
	for participant := 0; participant < s.Config.ParityArbCount; participant++ {
		desk := derivsim.NewParityArb(nextActor(), connect(fmt.Sprintf("parity_arb_%d", participant+1), derivArbBalances, 100_000_000*mvQuotePrecision, zeroFee), derivsim.ParityArbConfig{
			Underlying: "ABC/USD", EdgeBps: s.Config.ParityEdgeBps, LotQty: mvBasePrecision / 20,
			MaxTrades: 100_000, CheckInterval: s.Config.QuoteInterval,
		})
		desk.SetTickerFactory(timers)
		venue.ParityArbs = append(venue.ParityArbs, desk)
	}

	latentBalances := map[string]int64{"ABC": 50_000 * mvBasePrecision, "USD": 5_000_000_000 * mvQuotePrecision}
	if s.Config.CrossAssetSpotGraph {
		latentBalances["CDF"] = 1_000 * mvBasePrecision
	}
	for participant := 0; participant < s.Config.LatentLiquidityCount; participant++ {
		cfg := LatentLiquidityConfig{
			Interval: s.Config.NoiseInterval, DepositsPerTick: 20, CancelProbability: 0.02,
			DiffusionBps: 5, SpreadBps: 200, IntentionQty: mvBasePrecision / 10, MaxIntentions: 5_000,
		}
		if s.Config.LatentLiquidity != nil {
			cfg = *s.Config.LatentLiquidity
		}
		cfg.Symbol, cfg.BasePrecision, cfg.TickSize = "ABC/USD", mvBasePrecision, tick
		if cfg.Interval <= 0 {
			cfg.Interval = s.Config.NoiseInterval
		}
		cfg.Seed = flowSeed(s.Config.Seed, venueIndex, participant, 13)
		latent := NewLatentLiquidity(nextActor(), connect(fmt.Sprintf("latent_liquidity_%d", participant+1), latentBalances, 0, noiseFee), cfg)
		latent.SetTickerFactory(timers)
		venue.LatentLiquidity = append(venue.LatentLiquidity, latent)
	}

	carryBalances := map[string]int64{"ABC": 2_000 * mvBasePrecision, "USD": 200_000_000 * mvQuotePrecision}
	if s.Config.CrossAssetSpotGraph {
		carryBalances["CDF"] = 1_000 * mvBasePrecision
	}
	for participant := 0; participant < s.Config.CarryArbitrageurCount; participant++ {
		arb := NewCarryArbitrageur(nextActor(), connect(fmt.Sprintf("carry_arb_%d", participant+1), carryBalances, 100_000_000*mvQuotePrecision, noiseFee), CarryArbitrageurConfig{
			SpotSymbol: "ABC/USD", PerpSymbol: "ABC-PERP", BasePrecision: mvBasePrecision,
			Interval: s.Config.NoiseInterval, EntryBps: s.Config.CarryEntryBps, ExitBps: s.Config.CarryExitBps,
			MaxPosition: s.Config.CarryMaxPosition, LotQty: s.Config.CarryLotQty,
			SpotTick: tick, PerpTick: tick,
			MinOrderSize: mvBasePrecision / 1_000,
		})
		arb.SetTickerFactory(timers)
		venue.CarryArbs = append(venue.CarryArbs, arb)
	}

	supplierBalances := map[string]int64{"ABC": 20_000 * mvBasePrecision, "USD": 500_000_000 * mvQuotePrecision}
	if s.Config.CrossAssetSpotGraph {
		supplierBalances["CDF"] = 1_000 * mvBasePrecision
	}
	for participant := 0; participant < s.Config.ElasticSupplierCount; participant++ {
		symbol := makerSymbol(s.Config.ElasticSupplierSymbols, participant)
		reference := int64(mvBootstrapPrice)
		if symbol == "CDF/USD" {
			reference = int64(mvCDFBootstrap)
		}
		supplier := NewElasticSupplier(nextActor(), connect(fmt.Sprintf("elastic_supplier_%d", participant+1), supplierBalances, 0, noiseFee), ElasticSupplierConfig{
			Symbol: symbol, BasePrecision: mvBasePrecision, Interval: s.Config.NoiseInterval,
			// Seeded at the opening price and revised toward what it observes,
			// so the participant holds a private belief rather than a standing
			// instruction about the correct level.
			ReferencePrice: reference, BaseHolding: 0,
			ReferenceHalfLife:    s.Config.ElasticSupplierReferenceHalfLife,
			ElasticityPerPercent: s.Config.ElasticSupplierUnitsPerPercent,
			MaxPosition:          10_000 * mvBasePrecision, RebalanceLot: mvBasePrecision / 2,
		})
		supplier.SetTickerFactory(timers)
		venue.Suppliers = append(venue.Suppliers, supplier)
	}

	for participant := 0; participant < s.Config.RoundTripTraderCount; participant++ {
		var crossAssets []string
		if s.Config.CrossAssetSpotGraph {
			crossAssets = []string{"CDF"}
		}
		roundTripFunding := roundTripBalances(s.Config.RoundTripLotQty, mvBootstrapPrice, mvBasePrecision, s.Config.RoundTripInventoryLots, crossAssets)
		trader := NewRoundTripTrader(nextActor(), connect(fmt.Sprintf("round_trip_%d", participant+1), roundTripFunding, 10_000_000*mvQuotePrecision, noiseFee), RoundTripTraderConfig{
			Symbol: "ABC/USD", BasePrecision: mvBasePrecision, LotQty: s.Config.RoundTripLotQty,
			Interval: s.Config.NoiseInterval, HoldDuration: s.Config.RoundTripHold,
			OpenProbability: 0.5, Seed: flowSeed(s.Config.Seed, venueIndex, participant, 11),
			MinOrderSize: mvBasePrecision / 1_000,
		})
		trader.SetTickerFactory(timers)
		venue.RoundTripTraders = append(venue.RoundTripTraders, trader)
	}

	valueBalances := map[string]int64{"ABC": 1_000 * mvBasePrecision, "USD": 50_000_000 * mvQuotePrecision}
	if s.Config.CrossAssetSpotGraph {
		valueBalances["CDF"] = 1_000 * mvBasePrecision
	}
	return venue, nil
}

// connectParticipant allocates a venue-local account ID. Router legs call it
// on a dedicated delayed mount, so every venue account remains explicit and
// no collateral can pass between exchanges through shared Go state.
func (v *Venue) connectParticipant(mount *simulation.Mount, role string, balances map[string]int64, perpUSD int64, fee exchange.FeeModel) (uint64, actor.Gateway) {
	if role == "" {
		panic("multivenue: participant role is required")
	}
	v.nextClient++
	clientID := v.nextClient
	// A caller that supplied its own mount has already chosen a link — the
	// cross-venue routers are defined by theirs — so only the venue's own
	// mount is replaced by the role's.
	if mount == v.Mount {
		mount = v.mountForRole(role)
	}
	gw := mount.ConnectNewClient(clientID, balances, fee)
	if perpUSD > 0 {
		v.Exchange.AddPerpBalance(clientID, "USD", perpUSD)
	}
	v.Participants = append(v.Participants, Participant{VenueID: v.ID, ClientID: clientID, Role: role})
	return clientID, gw
}

// mountForRole returns the venue mount carrying the link a role connects
// through, building it once per role class.
func (v *Venue) mountForRole(role string) *simulation.Mount {
	if v.latencyMounts == nil {
		return v.Mount
	}
	class := roleClass(role)
	if mount, exists := v.latencyMounts[class]; exists {
		return mount
	}
	profile, configured := v.latencyConfig.latencyProfileFor(role)
	if !configured || profile.zero() {
		v.latencyMounts[class] = v.Mount
		return v.Mount
	}
	// The seed mixes the venue and the role class so two classes on one venue
	// draw independently while a run stays reproducible.
	seed := v.latencySeed + int64(len(class)) + int64(class[0])<<8
	request := profile.provider(seed, 1)
	if request == nil {
		v.latencyMounts[class] = v.Mount
		return v.Mount
	}
	mount := simulation.NewMount(v.Exchange, simulation.LatencyConfig{
		Request:    request,
		Response:   profile.provider(seed+1, profile.ResponseScale),
		MarketData: profile.provider(seed+2, profile.MarketDataScale),
		// Each participant draws its own sample path from the link's
		// distribution. Sharing one stream made a client's delays depend on
		// how many messages its neighbours had drawn first, which is a
		// coupling through the random number generator rather than through
		// the market, and it made runs of the same seed diverge.
		PerClient: func(clientID uint64) (simulation.LatencyProvider, simulation.LatencyProvider, simulation.LatencyProvider) {
			clientSeed := seed + int64(clientID)*0x9E3779B1
			return profile.provider(clientSeed, 1),
				profile.provider(clientSeed+1, profile.ResponseScale),
				profile.provider(clientSeed+2, profile.MarketDataScale)
		},
		Scheduler: v.scheduler,
		Clock:     v.clock,
	})
	v.latencyMounts[class] = mount
	v.latencyMountOrder = append(v.latencyMountOrder, mount)
	if v.registerMount != nil {
		v.registerMount(mount)
	}
	return mount
}

func (s *Sim) addCrossVenueRouters(clock *simulation.SimulatedClock, scheduler *simulation.EventScheduler, timers *simulation.SimTimerFactory, actorID *uint64) error {
	if len(s.Config.CrossVenueArbTiers) == 0 {
		return nil
	}
	balances := map[string]int64{
		"ABC": 1_000 * mvBasePrecision,
		"USD": 100_000_000 * mvQuotePrecision,
	}
	fee := &exchange.PercentageFee{MakerBps: 0, TakerBps: s.Config.TakerFeeBps, InQuote: true}
	for _, tier := range s.Config.CrossVenueArbTiers {
		delay := time.Duration(float64(s.Config.CrossVenueBaseLatency) * tier)
		legs := make([]CrossVenueArbLegConfig, 0, len(s.Venues))
		mounts := make([]*simulation.Mount, 0, len(s.Venues))
		for _, venue := range s.Venues {
			mount := simulation.NewMount(venue.Exchange, simulation.LatencyConfig{
				Request:    simulation.NewConstantLatency(delay),
				Response:   simulation.NewConstantLatency(delay),
				MarketData: simulation.NewConstantLatency(delay),
				Scheduler:  scheduler,
				Clock:      clock,
			})
			clientID, gw := venue.connectParticipant(mount, fmt.Sprintf("cross_venue_router_tier_%g", tier), balances, 0, fee)
			*actorID++
			legs = append(legs, CrossVenueArbLegConfig{
				VenueID: venue.ID, ClientID: clientID, ActorID: *actorID, Gateway: gw,
			})
			mounts = append(mounts, mount)
		}
		router, err := NewCrossVenueArb(tier, CrossVenueArbConfig{
			Symbol: "ABC/USD", LotQty: s.Config.CrossVenueArbLotQty,
			BasePrecision: mvBasePrecision, TakerFeeBps: s.Config.TakerFeeBps, MaxAttempts: s.Config.CrossVenueArbMaxAttempts,
		}, legs)
		if err != nil {
			return err
		}
		router.SetTickerFactory(timers)
		s.Routers = append(s.Routers, router)
		for _, mount := range mounts {
			s.Runner.AddMount(mount)
		}
		for _, leg := range router.Actors() {
			s.Runner.AddActor(leg)
		}
	}
	return nil
}

// flowSeed creates a stable stream per flow class, venue, and participant.
// It avoids mutable/shared random state, which would otherwise make adding a
// participant change an existing actor's draws or host scheduling observable.
func flowSeed(master int64, venueIndex, participant, flowClass int) int64 {
	value := uint64(master) + 0x9e3779b97f4a7c15
	value ^= uint64(venueIndex+1) * 0xbf58476d1ce4e5b9
	value ^= uint64(participant+1) * 0x94d049bb133111eb
	value ^= uint64(flowClass) * 0xd6e8feb86659fd93
	value ^= value >> 30
	value *= 0xbf58476d1ce4e5b9
	value ^= value >> 27
	value *= 0x94d049bb133111eb
	value ^= value >> 31
	return int64(value & ((uint64(1) << 63) - 1))
}

// capturePopulationAccounts obtains controller-only valuation rows after all
// participants have been connected (initial) or after the runner's final
// fixed point (terminal). It never runs in an actor callback.
func (s *Sim) capturePopulationAccounts(phase string) ([]ParticipantAccountSnapshot, error) {
	rows := make([]ParticipantAccountSnapshot, 0)
	// A momentary one-sided book must not invalidate a whole run, but a durably
	// one-sided one still must. The window is a few automation ticks.
	maxStaleness := int64(5 * s.Config.AutomationInterval)
	for _, venue := range s.Venues {
		now := venue.Exchange.Clock.NowUnixNano()
		venue.recordTwoSidedMarks(valuedSpotSymbols(s.Config.CrossAssetSpotGraph), now)
		spec, markSource, err := populationValuationSpec(venue, phase, s.Config.CrossAssetSpotGraph, now, maxStaleness)
		if err != nil {
			return nil, err
		}
		marks := make(map[string]int64, len(spec.AssetMarks))
		for asset, mark := range spec.AssetMarks {
			marks[asset] = mark.Price
		}
		for _, participant := range venue.Participants {
			account, err := venue.Exchange.MarkedAccount(participant.ClientID, spec)
			if err != nil {
				return nil, fmt.Errorf("multivenue: %s participant %s/%d marked account: %w", phase, venue.ID, participant.ClientID, err)
			}
			rows = append(rows, ParticipantAccountSnapshot{
				Participant: participant,
				Phase:       phase,
				MarkSource:  markSource,
				Marks:       marks,
				Account:     account,
			})
		}
	}
	return rows, nil
}

// venueRiskMarkStaleness bounds how old a cached midpoint may be when valuing
// dealer risk. It is generous relative to the automation cadence because this
// snapshot is diagnostic, while population accounts use the stricter window.
const venueRiskMarkStaleness = int64(60 * time.Second)

// logMakerState records what each spot maker believes, which is the only way
// to tell a price move driven by inventory from one driven by its volatility
// estimate.
func (s *Sim) addMetaorderTraders(timers *simulation.SimTimerFactory, actorID *uint64) error {
	cfg := s.Config.MetaorderTraders
	if cfg == nil {
		return nil
	}
	if err := cfg.validate(); err != nil {
		return err
	}
	fee := &exchange.PercentageFee{MakerBps: 0, TakerBps: s.Config.TakerFeeBps, InQuote: true}
	balances := map[string]int64{"ABC": 1_000 * mvBasePrecision, "USD": 50_000_000 * mvQuotePrecision}
	if s.Config.CrossAssetSpotGraph {
		balances["CDF"] = 1_000 * mvBasePrecision
	}
	count := s.Config.MetaorderTraderCount
	if count <= 0 {
		count = 1
	}
	for venueIndex, venue := range s.Venues {
		for participant := 0; participant < count; participant++ {
			local := *cfg
			local.Symbol = "ABC/USD"
			local.BasePrecision = mvBasePrecision
			local.TickSize = s.Config.SpotTickQuoteUnits
			if local.MinOrderSize <= 0 {
				local.MinOrderSize = mvBasePrecision / 1_000
			}
			local.Seed = flowSeed(s.Config.Seed, venueIndex, participant, 9)
			_, gw := venue.connectParticipant(venue.Mount, fmt.Sprintf("metaorder_trader_%d", participant+1), balances, 0, fee)
			*actorID++
			trader := NewMetaorderTrader(*actorID, gw, venue.ID, local)
			trader.SetTickerFactory(timers)
			venue.MetaorderTraders = append(venue.MetaorderTraders, trader)
			s.Runner.AddActor(trader)
		}
	}
	return nil
}

// verifyConservation checks, once per automation tick, that every unit of
// every asset the venue holds is explained by a recorded movement.
//
// A balance changed without a logged movement is invisible to any audit of the
// log, so the check has to run inside the exchange while it still has both the
// record and the reality. A violation is logged rather than fatal: the run
// continues so that the rest of the evidence is preserved, and the event is
// what a reader has to notice.
func (v *Venue) verifyConservation(now int64) {
	violations := v.Exchange.VerifyConservation()
	if len(violations) == 0 {
		return
	}
	log := v.makerStateLog
	if log.inner == nil {
		return
	}
	for _, violation := range violations {
		log.LogEvent(now, 0, "conservation_violation", violation)
	}
}

// makerStateName labels a maker by the book it quotes, so the state log can be
// read without knowing the construction order of the roster.
func makerStateName(symbol string) string {
	switch symbol {
	case "ABC/USD":
		return "spot"
	case "CDF/USD":
		return "cdf"
	case "ABC/CDF":
		return "cross"
	default:
		return symbol
	}
}

func (v *Venue) logMakerState(timestamp int64) {
	if v.makerStateLog.inner == nil {
		return
	}
	record := func(name string, maker *StoikovMarketMaker) {
		v.makerStateLog.LogEvent(timestamp, 0, "maker_state", map[string]any{
			"maker":          name,
			"forward":        maker.forward,
			"index":          maker.indexPrice,
			"inventory":      maker.inventory,
			"net_delta":      maker.NetDelta(),
			"log_variance":   maker.logVariancePerSec,
			"bid":            maker.bidPrice,
			"ask":            maker.askPrice,
			"hedge_position": maker.hedgePosition,
		})
	}
	// Every spot maker is recorded, not only the ones on the main book. The
	// makers that were omitted are the ones whose books later turned out to
	// misbehave, and a state that is not logged cannot be audited.
	for index, maker := range v.SpotMakers {
		record(fmt.Sprintf("%s_%d", makerStateName(maker.cfg.Symbol), index), maker)
	}
	// The perpetual maker is the counterparty that absorbs one-sided
	// derivative flow, so its position is what a premium would be paying for.
	if v.PerpMaker != nil {
		record("perp", v.PerpMaker)
	}
}

// observeMicrostructure samples the spot book's top of book and cumulative
// trade count. The book's sequence number advances once per trade, so it is
// already the trade counter this measurement needs.
func (v *Venue) observeMicrostructure() {
	if v.Microstructure == nil {
		return
	}
	book := v.Exchange.Books[v.Microstructure.Symbol]
	if book == nil {
		return
	}
	bestBid, bestAsk := int64(0), int64(0)
	if levels := book.Bids.GetPublicSnapshot(); len(levels) > 0 {
		bestBid = levels[0].Price
	}
	if levels := book.Asks.GetPublicSnapshot(); len(levels) > 0 {
		bestAsk = levels[0].Price
	}
	v.Microstructure.observe(bestBid, bestAsk, int64(book.SeqNum))
	for _, maker := range v.SpotMakers {
		if maker.cfg.Symbol == v.Microstructure.Symbol {
			// Record the risk the maker actually carries. With hedging on, the
			// spot position is offset elsewhere and says nothing about exposure.
			v.Microstructure.observeInventory(maker.NetDelta(), mvBasePrecision)
		}
	}
}

// hedgeSymbol applies the configured hedge instrument only to the spot book it
// is meant for, so a perpetual maker does not try to hedge itself.
func hedgeSymbol(quotedSymbol, configured string) string {
	if configured == "" || quotedSymbol != "ABC/USD" {
		return ""
	}
	return configured
}

// valuedSpotSymbols lists the books whose midpoints price participant wealth.
func valuedSpotSymbols(crossAssetSpotGraph bool) []string {
	if crossAssetSpotGraph {
		return []string{"ABC/USD", "CDF/USD"}
	}
	return []string{"ABC/USD"}
}

// twoSidedMark remembers the last midpoint observed while a book had both
// sides, so a valuation that lands in the instant between a maker's cancel and
// its replacement is not treated as an unpriceable market.
type twoSidedMark struct {
	price     int64
	timestamp int64
}

// recordTwoSidedMarks caches the current midpoint of every valued book. It runs
// on the venue's automation tick, which is also the cadence at which marks and
// risk are refreshed.
func (v *Venue) recordTwoSidedMarks(symbols []string, timestamp int64) {
	if v.lastTwoSided == nil {
		v.lastTwoSided = make(map[string]twoSidedMark, len(symbols))
	}
	for _, symbol := range symbols {
		if mid, ok := v.Exchange.TwoSidedMidPrice(symbol); ok && mid > 0 {
			v.lastTwoSided[symbol] = twoSidedMark{price: mid, timestamp: timestamp}
		}
	}
}

// valuationMark returns a two-sided midpoint, falling back to the most recent
// one within maxStaleness. A book that is durably one-sided still fails: the
// fallback covers a momentary gap in a live market, not a broken one.
func (v *Venue) valuationMark(symbol string, now, maxStaleness int64) (int64, bool, bool) {
	if mid, ok := v.Exchange.TwoSidedMidPrice(symbol); ok && mid > 0 {
		return mid, true, false
	}
	cached, ok := v.lastTwoSided[symbol]
	if !ok || cached.price <= 0 {
		return 0, false, false
	}
	if maxStaleness > 0 && now-cached.timestamp > maxStaleness {
		return 0, false, false
	}
	return cached.price, true, true
}

func populationValuationSpec(venue *Venue, phase string, crossAssetSpotGraph bool, now, maxStaleness int64) (etypes.AccountValuationSpec, string, error) {
	if venue == nil || venue.Exchange == nil {
		return etypes.AccountValuationSpec{}, "", errors.New("multivenue: missing venue for population valuation")
	}
	marks := map[string]etypes.AssetValuationMark{
		"USD": {Price: mvQuotePrecision, Precision: mvQuotePrecision},
	}
	markSource := "bootstrap_manifest"
	spotMark := int64(mvBootstrapPrice)
	if phase != "initial" {
		var ok bool
		var stale bool
		spotMark, ok, stale = venue.valuationMark("ABC/USD", now, maxStaleness)
		if !ok || spotMark <= 0 {
			return etypes.AccountValuationSpec{}, "", fmt.Errorf("multivenue: %s participant valuation requires two-sided ABC/USD mark on venue %s", phase, venue.ID)
		}
		markSource = "two_sided_ABC_USD_mid"
		if stale {
			markSource = "recent_two_sided_ABC_USD_mid"
		}
	}
	marks["ABC"] = etypes.AssetValuationMark{Price: spotMark, Precision: mvBasePrecision}
	if crossAssetSpotGraph {
		cdfMark := int64(mvCDFBootstrap)
		if phase != "initial" {
			var ok bool
			var cdfStale bool
			cdfMark, ok, cdfStale = venue.valuationMark("CDF/USD", now, maxStaleness)
			if !ok || cdfMark <= 0 {
				return etypes.AccountValuationSpec{}, "", fmt.Errorf("multivenue: %s participant valuation requires two-sided CDF/USD mark on venue %s", phase, venue.ID)
			}
			markSource = "two_sided_ABC_USD_and_CDF_USD_mid"
			if cdfStale || markSource == "recent_two_sided_ABC_USD_mid" {
				markSource = "recent_two_sided_ABC_USD_and_CDF_USD_mid"
			}
		}
		marks["CDF"] = etypes.AssetValuationMark{Price: cdfMark, Precision: mvBasePrecision}
	}
	return etypes.AccountValuationSpec{
		ReportAsset: "USD", ReportPrecision: mvQuotePrecision, AssetMarks: marks,
	}, markSource, nil
}

func captureVenueRisk(venue *Venue, phase string) (*VenueRiskSnapshot, error) {
	if venue == nil || venue.Exchange == nil || venue.OptionDealer == nil || venue.OptionDealerClientID == 0 {
		return nil, errors.New("multivenue: incomplete venue risk capture")
	}
	// Dealer risk is captured on the automation tick and at shutdown, so it can
	// land in the instant between a maker's cancel and its replacement. Reuse
	// the same bounded-staleness mark the population accounts use rather than
	// silently valuing ABC at zero.
	now := venue.Exchange.Clock.NowUnixNano()
	spotMid, ok, _ := venue.valuationMark("ABC/USD", now, venueRiskMarkStaleness)
	if !ok || spotMid <= 0 {
		spotMid = 0
	}
	marks := map[string]etypes.AssetValuationMark{
		"USD": {Price: mvQuotePrecision, Precision: mvQuotePrecision},
	}
	if spotMid > 0 {
		marks["ABC"] = etypes.AssetValuationMark{Price: spotMid, Precision: mvBasePrecision}
	}
	if venue.lastTwoSided != nil {
		if cdf, cdfOK, _ := venue.valuationMark("CDF/USD", now, venueRiskMarkStaleness); cdfOK && cdf > 0 {
			marks["CDF"] = etypes.AssetValuationMark{Price: cdf, Precision: mvBasePrecision}
		}
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
		// The exchange marks with the instrument's own volatility, not with
		// the dealer's. Once dealers hold their own models the two views
		// diverge, and reporting the dealer's book at the venue's mark while
		// labelling it the dealer's implied volatility invites reading a
		// dealer's opinion off a number it had no part in.
		markVol := option.IV
		if venue.OptionDealer != nil {
			if dealerVol := venue.OptionDealer.PricingVolatility(option.Strike, yearsLeft, option.IsCall); dealerVol > 0 {
				markVol = dealerVol
			}
		}
		sensitivity, ok := eprice.Black76Sensitivities(forward, option.Strike, markVol, yearsLeft, option.IsCall)
		if !ok {
			return derivsim.GreekProfile{}, nil, fmt.Errorf("multivenue: venue %s option %s has invalid exchange Greek mark", venue.ID, option.Symbol())
		}
		contracts := float64(pos.Size) / float64(mvBasePrecision)
		position := derivsim.GreekPosition{
			Timestamp: account.Timestamp, Phase: phase, Symbol: option.Symbol(), Underlying: option.UnderlyingSymbol(),
			ListedNano: venue.optionListedNano[option.Symbol()], ExpiryNano: option.ExpiryNano(), Strike: option.Strike,
			IsCall: option.IsCall, Position: pos.Size, TimeToExpiryNano: timeToExpiry,
			SpotMid: forward, ModelForward: forward, ForwardSource: "option_underlying_mark", ImpliedVolatility: markVol,
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
			profile.ImpliedVolatility = markVol
		}
	}
	profile.NetDelta = profile.OptionDelta + profile.HedgeDelta
	return profile, positions, nil
}

// VenueLedger reports the exchange's own balances. Without it a report cannot
// verify conservation: a bankrupt participant's negative balance is zeroed and
// charged to the insurance fund, which writes off their debt and therefore
// raises the population's summed result above the fees it paid.
type VenueLedger struct {
	VenueID       string           `json:"venue_id"`
	FeeRevenue    map[string]int64 `json:"fee_revenue"`
	InsuranceFund map[string]int64 `json:"insurance_fund"`
}

// CaptureVenueLedgers snapshots every venue's own balances.
func (s *Sim) CaptureVenueLedgers() []VenueLedger {
	ledgers := make([]VenueLedger, 0, len(s.Venues))
	for _, venue := range s.Venues {
		balance := venue.Exchange.ExchangeBalance
		if balance == nil {
			continue
		}
		ledger := VenueLedger{
			VenueID:       venue.ID,
			FeeRevenue:    make(map[string]int64, len(balance.FeeRevenue)),
			InsuranceFund: make(map[string]int64, len(balance.InsuranceFund)),
		}
		for asset, amount := range balance.FeeRevenue {
			ledger.FeeRevenue[asset] = amount
		}
		for asset, amount := range balance.InsuranceFund {
			ledger.InsuranceFund[asset] = amount
		}
		ledgers = append(ledgers, ledger)
	}
	return ledgers
}
