package multivenue

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/analysis"
	"exchange_sim/exchange"
	"exchange_sim/matching"
	eprice "exchange_sim/price"
	"exchange_sim/simulation"
	"exchange_sim/simulations/derivsim"
	"exchange_sim/simulations/feesim"
	etypes "exchange_sim/types"
)

func TestCloseSuppressesRawEvidenceHashAfterLoggerFailure(t *testing.T) {
	if _, err := os.Stat("/dev/full"); err != nil {
		t.Skipf("failure-injection device unavailable: %v", err)
	}
	logger, err := feesim.NewJSONLinesLogger("/dev/full")
	if err != nil {
		t.Fatal(err)
	}
	logger.LogEvent(1, 1, "event", nil)
	dir := t.TempDir()
	sim := &Sim{
		Config:  Config{LogMode: "full", LogDir: dir},
		loggers: []*feesim.JSONLinesLogger{logger},
	}
	if err := sim.Close(); err == nil {
		t.Fatal("Close succeeded after raw evidence write failure")
	}
	if _, err := os.Stat(filepath.Join(dir, "evidence-artifact-hash.json")); !os.IsNotExist(err) {
		t.Fatalf("runtime evidence hash exists after failed close: %v", err)
	}
}

func TestCloseSuppressesEvidenceArtifactAfterCheckpointFailure(t *testing.T) {
	want := errors.New("injected checkpoint write failure")
	logger, err := feesim.NewJSONLinesLogger(filepath.Join(t.TempDir(), "evidence.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	sim := &Sim{
		Config:      Config{LogMode: "full", LogDir: dir},
		loggers:     []*feesim.JSONLinesLogger{logger},
		checkpoints: &checkpointSink{intervalNano: 1, checkpoints: &checkpointFailWriter{err: want}, finalSimTime: 1, firstEvent: true},
	}
	if err := sim.Close(); !errors.Is(err, want) {
		t.Fatalf("Close error = %v, want %v", err, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "evidence-artifact-hash.json")); !os.IsNotExist(err) {
		t.Fatalf("runtime evidence hash exists after checkpoint failure: %v", err)
	}
}

func TestNewSimRejectsEvidenceDirectoryReuse(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stale-completion.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := NewSim(time.Second, Config{LogDir: dir, LogMode: "none", Seed: 101})
	if err == nil || !strings.Contains(err.Error(), "refusing evidence reuse") {
		t.Fatalf("non-empty evidence directory accepted: %v", err)
	}
}

func TestCloseRetainsLateSidecarFailure(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "not-a-directory-*")
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	sim := &Sim{
		Config:           Config{LogMode: "none", LogDir: file.Name()},
		latencyTelemetry: simulation.NewLatencyStats(),
	}
	first := sim.Close()
	if first == nil {
		t.Fatal("Close succeeded when latency sidecar parent was a regular file")
	}
	second := sim.Close()
	if second == nil || second.Error() != first.Error() {
		t.Fatalf("sticky close error changed: first=%v second=%v", first, second)
	}
}

func TestClosePublishesNoHashWhenLatencySidecarFails(t *testing.T) {
	root := t.TempDir()
	badPath := filepath.Join(root, "latency-parent")
	if err := os.WriteFile(badPath, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	logger, err := feesim.NewJSONLinesLogger(filepath.Join(root, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	sim := &Sim{
		Config:           Config{LogMode: "full", LogDir: badPath},
		loggers:          []*feesim.JSONLinesLogger{logger},
		latencyTelemetry: simulation.NewLatencyStats(),
	}
	if err := sim.Close(); err == nil {
		t.Fatal("Close succeeded after latency sidecar failure")
	}
	if _, err := os.Stat(filepath.Join(badPath, "evidence-artifact-hash.json")); err == nil {
		t.Fatal("runtime evidence hash exists after latency failure")
	}
}

func TestConfigAcceptsDocumentedSnakeCaseJSON(t *testing.T) {
	var cfg Config
	if err := json.Unmarshal([]byte(`{"log_dir":"ignored","log_mode":"none","seed":99,"dealer_hedge_mode":"off","short_option_tenor":7200000000000,"spot_passive_maker_post_only":true,"spot_passive_maker_cancel_before_replace":true}`), &cfg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if cfg.LogMode != "none" || cfg.Seed != 99 || cfg.DealerHedgeMode != "off" || cfg.ShortOptionTenor != 2*time.Hour || !cfg.SpotPassiveMakerPostOnly || !cfg.SpotPassiveMakerCancelBeforeReplace {
		t.Fatalf("snake-case config was not decoded: %+v", cfg)
	}
}

func TestNormalizedConfigMatchesManifestConfiguration(t *testing.T) {
	t.Setenv("EXSIM_BINARY_EVIDENCE", "")
	logDir := t.TempDir()
	input := Config{LogDir: logDir, LogMode: "none", Seed: 607}
	effective, err := NormalizeConfig(input)
	if err != nil {
		t.Fatalf("NormalizeConfig: %v", err)
	}
	effective.LogDir = ""
	sim, err := NewSim(time.Second, input)
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	if err := sim.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(logDir, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var written manifest
	if err := json.Unmarshal(raw, &written); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if !reflect.DeepEqual(written.Config, effective) {
		t.Fatalf("manifest configuration differs from normalized input:\nmanifest=%+v\neffective=%+v", written.Config, effective)
	}
}

func TestP5ConfigRetainsExplicitFalseFutureFlowAndActorClocks(t *testing.T) {
	var cfg Config
	raw := []byte(`{
		"option_flow_include_futures":false,
		"record_dated_execution_mandate_decisions":true,
		"record_dated_term_carry_decisions":true,
		"dated_future_execution_mandate":{"enabled":true,"underlying":"ABC/USD","target_tenor_nanos":28800000000000,"side":"BUY","parent_qty":200,"child_qty":10,"start_delay_nanos":600000000000,"execution_duration_nanos":7200000000000,"decision_period_nanos":300000000000,"decision_phase_offset_nanos":0,"max_market_age_nanos":10000000000,"slippage_bps":15,"tick_size":1},
		"dated_term_carry_allocator":{"enabled":true,"trade_enabled":false,"spot_symbol":"ABC/USD","target_tenor_nanos":28800000000000,"decision_period_nanos":2000000000,"decision_phase_offset_nanos":0,"max_market_age_nanos":10000000000,"min_time_to_expiry_nanos":600000000000,"taker_fee_bps":5,"long_spot_funding_bps":500,"short_spot_borrow_bps":500,"balance_sheet_bps":1,"margin_risk_bps":1,"leg_risk_bps":1,"settlement_mismatch_bps":2,"post_settlement_exit_bps":2,"min_net_carry_bps":1,"max_position":100,"lot_qty":10,"min_order_size":1,"spot_tick":1,"future_tick":1,"passive_exit_slice_qty":1,"exit_deadline_after_settlement_nanos":3600000000000}
	}`)
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.OptionFlowIncludeFutures == nil || *cfg.OptionFlowIncludeFutures || cfg.DatedFutureExecutionMandate == nil || cfg.DatedTermCarryAllocator == nil || cfg.DatedTermCarryAllocator.TradeEnabled {
		t.Fatalf("P5 explicit serialization was lost: %+v", cfg)
	}
	if cfg.DatedFutureExecutionMandate.DecisionPeriod != 5*time.Minute || cfg.DatedTermCarryAllocator.DecisionPeriod != 2*time.Second {
		t.Fatalf("P5 clocks decoded incorrectly: mandate=%s carry=%s", cfg.DatedFutureExecutionMandate.DecisionPeriod, cfg.DatedTermCarryAllocator.DecisionPeriod)
	}
}

func TestSpotStoikovInventorySizePolicyIsScopedToSpot(t *testing.T) {
	sim, err := NewSim(time.Second, Config{
		LogDir: t.TempDir(), LogMode: "none", Seed: 101,
		CrossAssetSpotGraph:                 true,
		SpotStoikovInventorySizeSkewBps:     5_000,
		SpotPassiveMakerPostOnly:            true,
		SpotPassiveMakerCancelBeforeReplace: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sim.Close(); err != nil {
			t.Error(err)
		}
	})
	for _, venue := range sim.Venues {
		if venue.PerpMaker.cfg.InventorySizeSkewBps != 0 {
			t.Fatalf("%s perpetual received scoped spot size policy: %d", venue.ID, venue.PerpMaker.cfg.InventorySizeSkewBps)
		}
		for _, maker := range venue.SpotMakers {
			if maker.cfg.InventorySizeSkewBps != 5_000 {
				t.Fatalf("%s %s size skew bps = %d, want 5000", venue.ID, maker.cfg.Symbol, maker.cfg.InventorySizeSkewBps)
			}
		}
	}
}

func TestMakerQuoteSizeEvidenceRequiresFullLogMode(t *testing.T) {
	_, err := NewSim(time.Second, Config{
		LogDir: t.TempDir(), LogMode: "none", Seed: 101,
		RecordMakerQuoteSizeDecisions: true,
	})
	if err == nil {
		t.Fatal("maker quote-size evidence was accepted without persisted raw logs")
	}
}

func TestPerpMakerReplenishmentConfigurationIsScopedAndValidated(t *testing.T) {
	sim, err := NewSim(time.Second, Config{
		LogDir: t.TempDir(), LogMode: "none", Seed: 101,
		PerpMakerReplenishBelowBps: 5_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sim.Close(); err != nil {
			t.Error(err)
		}
	})
	for _, venue := range sim.Venues {
		if venue.PerpMaker.cfg.RestingQuoteReplenishmentBelowBps != 5_000 {
			t.Fatalf("%s perp replenishment threshold = %d, want 5000", venue.ID, venue.PerpMaker.cfg.RestingQuoteReplenishmentBelowBps)
		}
		for _, maker := range venue.SpotMakers {
			if maker.cfg.RestingQuoteReplenishmentBelowBps != 0 {
				t.Fatalf("%s %s received perp replenishment policy: %d", venue.ID, maker.cfg.Symbol, maker.cfg.RestingQuoteReplenishmentBelowBps)
			}
		}
	}
	for _, bps := range []int64{-1, 10_001} {
		_, err := NewSim(time.Second, Config{LogDir: t.TempDir(), LogMode: "none", PerpMakerReplenishBelowBps: bps})
		if err == nil || !strings.Contains(err.Error(), "perp maker replenishment bps") {
			t.Fatalf("threshold %d error = %v, want explicit range rejection", bps, err)
		}
	}
	_, err = NewSim(time.Second, Config{LogDir: t.TempDir(), LogMode: "none", RecordPerpMakerReplenishmentDecisions: true})
	if err == nil || !strings.Contains(err.Error(), "perp maker replenishment decisions require full persisted evidence") {
		t.Fatalf("non-full replenishment evidence error = %v", err)
	}
}

func TestConfigRejectsCancelBeforeReplaceWithoutPostOnly(t *testing.T) {
	_, err := NewSim(time.Second, Config{
		LogDir: t.TempDir(), LogMode: "none", SpotPassiveMakerCancelBeforeReplace: true,
	})
	if err == nil || !strings.Contains(err.Error(), "requires spot passive maker post-only") {
		t.Fatalf("NewSim error = %v, want explicit post-only requirement", err)
	}
}

func TestVenueRulesSelectIndependentMatchingPolicies(t *testing.T) {
	sim, err := NewSim(time.Second, Config{
		LogDir:  t.TempDir(),
		LogMode: "none",
		VenueRules: map[string]VenueRule{
			"north":   {MatchingRule: MatchingPriceTime},
			"central": {MatchingRule: MatchingProRata},
			"south":   {MatchingRule: MatchingProRata},
		},
	})
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	defer sim.Close()

	for _, venue := range sim.Venues {
		switch venue.ID {
		case "north":
			if venue.MatchingRule != MatchingPriceTime {
				t.Fatalf("north matching rule = %q", venue.MatchingRule)
			}
			if _, ok := venue.Exchange.Matcher.(*matching.PriceTimeMatcher); !ok {
				t.Fatalf("north matcher = %T, want price-time", venue.Exchange.Matcher)
			}
		case "central", "south":
			if venue.MatchingRule != MatchingProRata {
				t.Fatalf("%s matching rule = %q", venue.ID, venue.MatchingRule)
			}
			if _, ok := venue.Exchange.Matcher.(*matching.ProRataMatcher); !ok {
				t.Fatalf("%s matcher = %T, want pro-rata", venue.ID, venue.Exchange.Matcher)
			}
		default:
			t.Fatalf("unexpected venue %q", venue.ID)
		}
	}
}

func TestVenueRulesRejectUnknownVenueAndMatchingPolicy(t *testing.T) {
	for name, rules := range map[string]map[string]VenueRule{
		"unknown venue":  {"east": {MatchingRule: MatchingPriceTime}},
		"unknown policy": {"north": {MatchingRule: "random"}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewSim(time.Second, Config{LogDir: t.TempDir(), LogMode: "none", VenueRules: rules})
			if err == nil {
				t.Fatal("NewSim accepted invalid venue rule")
			}
		})
	}
}

func TestStrictPopulationAccountingCapturesEveryParticipant(t *testing.T) {
	sim, err := NewSim(3*time.Second, Config{
		LogDir:                     t.TempDir(),
		LogMode:                    "none",
		Seed:                       71,
		NoiseTraderCount:           2,
		OptionFlowCount:            3,
		StrictPopulationAccounting: true,
	})
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	defer sim.Close()

	wantCount := 0
	for _, venue := range sim.Venues {
		wantCount += len(venue.Participants)
	}
	if len(sim.InitialAccounts) != wantCount {
		t.Fatalf("initial account rows = %d, want %d", len(sim.InitialAccounts), wantCount)
	}
	seen := make(map[string]struct{}, wantCount)
	for _, row := range sim.InitialAccounts {
		if row.Phase != "initial" || row.MarkSource != "bootstrap_manifest" || row.Account.ReportAsset != "USD" {
			t.Fatalf("invalid initial population row: %#v", row)
		}
		key := fmt.Sprintf("%s/%d", row.VenueID, row.ClientID)
		if _, duplicate := seen[key]; duplicate {
			t.Fatalf("duplicate participant row %s", key)
		}
		seen[key] = struct{}{}
	}
	if err := sim.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(sim.TerminalAccounts) != wantCount {
		t.Fatalf("terminal account rows = %d, want %d", len(sim.TerminalAccounts), wantCount)
	}
	for _, row := range sim.TerminalAccounts {
		if row.Phase != "terminal_post_mark" || row.MarkSource != "two_sided_ABC_USD_mid" || row.Account.ReportAsset != "USD" {
			t.Fatalf("invalid terminal population row: %#v", row)
		}
	}
}

func TestCrossAssetSpotGraphListsAndValuesEveryPair(t *testing.T) {
	sim, err := NewSim(3*time.Second, Config{
		LogDir:                     t.TempDir(),
		LogMode:                    "none",
		Seed:                       73,
		CrossAssetSpotGraph:        true,
		StrictPopulationAccounting: true,
	})
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	defer sim.Close()
	for _, venue := range sim.Venues {
		if len(venue.SpotMakers) != 6 {
			t.Fatalf("venue %s spot makers = %d, want 6", venue.ID, len(venue.SpotMakers))
		}
		for _, symbol := range []string{"ABC/USD", "CDF/USD", "ABC/CDF"} {
			if venue.Exchange.Books[symbol] == nil {
				t.Fatalf("venue %s missing cross-asset spot book %s", venue.ID, symbol)
			}
		}
		for _, maker := range venue.SpotMakers {
			if maker.cfg.Symbol != "ABC/CDF" {
				continue
			}
			if maker.cfg.QuotePrecision != mvBasePrecision {
				t.Fatalf("venue %s ABC/CDF quote precision = %d, want %d", venue.ID, maker.cfg.QuotePrecision, mvBasePrecision)
			}
			// The control parameters are scale free, so the cross book carries
			// exactly the same relative variance and risk parameters as the
			// USD books rather than a per-pair converted copy.
			referencePrice := float64(mvBootstrapPrice) / float64(mvQuotePrecision)
			wantVariance := sim.Config.StoikovVariancePerSecond / (referencePrice * referencePrice)
			if math.Abs(maker.cfg.InitialLogVariancePerSec-wantVariance) > 1e-18 {
				t.Fatalf("venue %s ABC/CDF log variance = %.18g, want %.18g", venue.ID, maker.cfg.InitialLogVariancePerSec, wantVariance)
			}
			forward := float64(maker.cfg.BootstrapPrice) / float64(maker.cfg.QuotePrecision)
			quote, ok := CalculateStoikovQuote(StoikovInputs{
				Forward:           forward,
				VariancePerSecond: maker.cfg.InitialLogVariancePerSec * forward * forward,
				RiskAversion:      maker.cfg.RelativeRiskAversion / forward,
				FillDecay:         maker.cfg.RelativeFillDecay / forward,
				InventoryHorizon:  maker.cfg.InventoryHorizon,
				MinHalfSpread:     float64(maker.cfg.TickSize) / float64(maker.cfg.QuotePrecision),
			})
			if !ok || quote.Bid <= 0 || quote.Ask <= quote.Bid {
				t.Fatalf("venue %s ABC/CDF bootstrap quote invalid: %#v", venue.ID, quote)
			}
		}
	}
	if err := sim.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(sim.InitialAccounts) == 0 || len(sim.InitialAccounts) != len(sim.TerminalAccounts) {
		t.Fatalf("population account row counts initial=%d terminal=%d", len(sim.InitialAccounts), len(sim.TerminalAccounts))
	}
	for _, row := range sim.TerminalAccounts {
		if row.MarkSource != "two_sided_ABC_USD_and_CDF_USD_mid" || row.Account.ReportAsset != "USD" {
			t.Fatalf("invalid terminal cross-asset account: %#v", row)
		}
	}
}

func TestSpotPassiveMakerPostOnlySelectsAllCDFRefreshFamilies(t *testing.T) {
	sim, err := NewSim(time.Second, Config{
		LogDir: t.TempDir(), LogMode: "none", CrossAssetSpotGraph: true,
		SpotPassiveMakerPostOnly:            true,
		SpotPassiveMakerCancelBeforeReplace: true,
		FixedDistanceMakerCount:             2,
		FixedDistanceMakerSymbols:           []string{"CDF/USD", "ABC-PERP"},
		ImbalanceMakerCount:                 2,
		ImbalanceMakerSymbols:               []string{"CDF/USD", "ABC-PERP"},
	})
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	defer sim.Close()
	for _, venue := range sim.Venues {
		for _, maker := range venue.SpotMakers {
			if !maker.cfg.PostOnly || !maker.cfg.PostOnlyCancelBeforeReplace {
				t.Fatalf("venue %s Stoikov maker %s is not post-only", venue.ID, maker.cfg.Symbol)
			}
		}
		for _, maker := range venue.FixedDistanceMakers {
			if maker.cfg.Symbol == "CDF/USD" && (!maker.cfg.PostOnly || !maker.cfg.PostOnlyCancelBeforeReplace) {
				t.Fatalf("venue %s fixed-distance config = %+v", venue.ID, maker.cfg)
			}
			if maker.cfg.Symbol == "ABC-PERP" && (maker.cfg.PostOnly || maker.cfg.PostOnlyCancelBeforeReplace) {
				t.Fatalf("venue %s derivative fixed-distance maker received spot-only policy: %+v", venue.ID, maker.cfg)
			}
		}
		for _, maker := range venue.ImbalanceMakers {
			if maker.cfg.Symbol == "CDF/USD" && (!maker.cfg.PostOnly || !maker.cfg.PostOnlyCancelBeforeReplace) {
				t.Fatalf("venue %s imbalance config = %+v", venue.ID, maker.cfg)
			}
			if maker.cfg.Symbol == "ABC-PERP" && (maker.cfg.PostOnly || maker.cfg.PostOnlyCancelBeforeReplace) {
				t.Fatalf("venue %s derivative imbalance maker received spot-only policy: %+v", venue.ID, maker.cfg)
			}
		}
		if venue.PerpMaker.cfg.PostOnly || venue.PerpMaker.cfg.PostOnlyCancelBeforeReplace {
			t.Fatalf("venue %s perp Stoikov maker received spot-only policy: %+v", venue.ID, venue.PerpMaker.cfg)
		}
	}
}

func TestPopulationValuationRejectsMissingTerminalTwoSidedMark(t *testing.T) {
	venue := &Venue{ID: "unpriced", Exchange: exchange.NewExchange(1, nil)}
	if _, _, err := populationValuationSpec(venue, "terminal_post_mark", false, 0, 0); !errors.Is(err, etypes.ErrNoPrice) {
		t.Fatalf("terminal population valuation error = %v, want ErrNoPrice", err)
	}
}

func TestValuationMarkPreservesSignedPresenceAndSeparatesDomain(t *testing.T) {
	venue := &Venue{
		ID:       "signed",
		Exchange: exchange.NewExchange(1, nil),
		lastTwoSided: map[string]twoSidedMark{
			"ABC/USD": {price: 0, timestamp: 100},
			"CDF/USD": {price: -20, timestamp: 100},
		},
	}
	defer venue.Exchange.Shutdown()
	for _, tc := range []struct {
		symbol string
		want   int64
	}{
		{symbol: "ABC/USD", want: 0},
		{symbol: "CDF/USD", want: -20},
	} {
		t.Run(tc.symbol, func(t *testing.T) {
			got, available, stale := venue.valuationMark(tc.symbol, 100, 0)
			if !available || !stale || got != tc.want {
				t.Fatalf("valuation mark = (%d, available=%t, stale=%t), want (%d, true, true)", got, available, stale, tc.want)
			}
		})
	}
	if err := requirePositiveUSDValuationMark("probe", "ABC/USD", venue.ID, 0, true); !errors.Is(err, etypes.ErrPriceDomain) {
		t.Fatalf("present zero valuation mark error = %v, want ErrPriceDomain", err)
	}
	if err := requirePositiveUSDValuationMark("probe", "ABC/USD", venue.ID, 0, false); !errors.Is(err, etypes.ErrNoPrice) {
		t.Fatalf("missing valuation mark error = %v, want ErrNoPrice", err)
	}
}

func TestRiskSnapshotMarksBootstrapFallbackExplicitly(t *testing.T) {
	venue := &Venue{
		ID:                   "bootstrap",
		Exchange:             exchange.NewExchange(1, nil),
		OptionDealer:         &derivsim.OptionMarketMaker{},
		OptionDealerClientID: 1,
	}
	defer venue.Exchange.Shutdown()
	venue.Exchange.ConnectNewClient(1, nil, &exchange.FixedFee{})
	risk, err := captureVenueRisk(venue, "post_derivative_mark")
	if err != nil {
		t.Fatalf("capture pre-first-mark risk: %v", err)
	}
	if risk.AccountMarkSource != "bootstrap_manifest_pre_first_two_sided_mark" {
		t.Fatalf("bootstrap risk mark source = %q", risk.AccountMarkSource)
	}
}

func TestCrossAssetCollateralMarksAreExplicitAndFinite(t *testing.T) {
	_, err := NewSim(time.Second, Config{
		LogDir: t.TempDir(), LogMode: "none", Seed: 101,
		CrossAssetCollateralMarks: true,
	})
	if err == nil || !strings.Contains(err.Error(), "require the cross-asset spot graph") {
		t.Fatalf("collateral marks without graph error = %v, want explicit graph requirement", err)
	}

	legacy, err := NewSim(time.Second, Config{
		LogDir: t.TempDir(), LogMode: "none", Seed: 101,
		CrossAssetSpotGraph: true,
	})
	if err != nil {
		t.Fatalf("legacy cross-asset NewSim: %v", err)
	}
	defer legacy.Close()
	if _, err := legacy.Venues[0].Exchange.BorrowingMgr.Config.PriceSource.Price("CDF"); err == nil {
		t.Fatal("legacy cross-asset collateral oracle unexpectedly priced CDF")
	}

	repaired, err := NewSim(time.Second, Config{
		LogDir: t.TempDir(), LogMode: "none", Seed: 101,
		CrossAssetSpotGraph: true, CrossAssetCollateralMarks: true,
	})
	if err != nil {
		t.Fatalf("repaired cross-asset NewSim: %v", err)
	}
	defer repaired.Close()
	borrowing := repaired.Venues[0].Exchange.BorrowingMgr.Config
	price, err := borrowing.PriceSource.Price("CDF")
	if err != nil || price != mvCDFBootstrap {
		t.Fatalf("CDF collateral mark = (%d, %v), want (%d, nil)", price, err, mvCDFBootstrap)
	}
	if got := borrowing.AssetPrecisions["CDF"]; got != mvBasePrecision {
		t.Fatalf("CDF collateral precision = %d, want %d", got, mvBasePrecision)
	}
	if got := borrowing.MaxBorrowPerAsset["CDF"]; got != 20_000*mvBasePrecision {
		t.Fatalf("CDF max borrow = %d, want %d", got, 20_000*mvBasePrecision)
	}
	var makerClientID uint64
	for _, participant := range repaired.Venues[0].Participants {
		if participant.Role == "cdf_spot_maker_1" {
			makerClientID = participant.ClientID
			break
		}
	}
	if makerClientID == 0 {
		t.Fatal("repaired world has no CDF maker client")
	}
	client := repaired.Venues[0].Exchange.Clients[makerClientID]
	beforeBalance, beforeDebt := client.Balances["CDF"], client.Borrowed["CDF"]
	if err := borrowingManagerBorrow(repaired.Venues[0].Exchange, makerClientID, "CDF", mvBasePrecision); err != nil {
		t.Fatalf("explicit CDF collateral borrow: %v", err)
	}
	if client.Balances["CDF"] != beforeBalance+mvBasePrecision || client.Borrowed["CDF"] != beforeDebt+mvBasePrecision {
		t.Fatalf("CDF borrow did not credit/debit one unit: balance %d debt %d", client.Balances["CDF"], client.Borrowed["CDF"])
	}
	debtAfterSmallBorrow := client.Borrowed["CDF"]
	if err := borrowingManagerBorrow(repaired.Venues[0].Exchange, makerClientID, "CDF", 20_000*mvBasePrecision); err == nil {
		t.Fatal("CDF borrow beyond finite cap unexpectedly succeeded")
	}
	if client.Borrowed["CDF"] != debtAfterSmallBorrow {
		t.Fatalf("cap rejection mutated CDF debt: got %d want %d", client.Borrowed["CDF"], debtAfterSmallBorrow)
	}
	var legacyMakerClientID uint64
	for _, participant := range legacy.Venues[0].Participants {
		if participant.Role == "cdf_spot_maker_1" {
			legacyMakerClientID = participant.ClientID
			break
		}
	}
	legacyClient := legacy.Venues[0].Exchange.Clients[legacyMakerClientID]
	legacyBalance, legacyDebt := legacyClient.Balances["CDF"], legacyClient.Borrowed["CDF"]
	if err := borrowingManagerBorrow(legacy.Venues[0].Exchange, legacyMakerClientID, "CDF", mvBasePrecision); err == nil {
		t.Fatal("legacy CDF collateral borrow unexpectedly succeeded without an oracle mark")
	}
	if legacyClient.Balances["CDF"] != legacyBalance || legacyClient.Borrowed["CDF"] != legacyDebt {
		t.Fatalf("missing CDF mark borrow mutated account: balance %d/%d debt %d/%d", legacyClient.Balances["CDF"], legacyBalance, legacyClient.Borrowed["CDF"], legacyDebt)
	}
}

func borrowingManagerBorrow(ex *exchange.Exchange, clientID uint64, asset string, amount int64) error {
	client := ex.Clients[clientID]
	return ex.BorrowingMgr.BorrowMargin(exchange.BorrowContext{
		Client: client, ClientID: clientID, Timestamp: ex.Clock.NowUnixNano(), CreditSpot: true,
	}, asset, amount, "test")
}

func TestDealerRiskDoesNotRequireUnrelatedCDFMark(t *testing.T) {
	venue := &Venue{
		ID:                   "dealer",
		Exchange:             exchange.NewExchange(1, nil),
		OptionDealer:         &derivsim.OptionMarketMaker{},
		OptionDealerClientID: 1,
		lastTwoSided: map[string]twoSidedMark{
			"CDF/USD": {price: mvCDFBootstrap, timestamp: 100},
		},
	}
	defer venue.Exchange.Shutdown()
	venue.Exchange.ConnectNewClient(1, nil, &exchange.FixedFee{})
	if _, err := captureVenueRisk(venue, "post_derivative_mark"); err != nil {
		t.Fatalf("dealer risk demanded unrelated CDF mark: %v", err)
	}
}

func TestDealerRiskRequiresCDFMarkForCDFExposure(t *testing.T) {
	venue := &Venue{
		ID:                   "cdf-dealer",
		Exchange:             exchange.NewExchange(1, nil),
		OptionDealer:         &derivsim.OptionMarketMaker{},
		OptionDealerClientID: 1,
	}
	defer venue.Exchange.Shutdown()
	venue.Exchange.ConnectNewClient(1, map[string]int64{"CDF": 1}, &exchange.FixedFee{})
	_, err := captureVenueRisk(venue, "post_derivative_mark")
	if err == nil || !errors.Is(err, etypes.ErrNoPrice) {
		t.Fatalf("CDF exposure risk error = %v, want ErrNoPrice", err)
	}
}

func TestStrictPopulationAccountsDigestAcrossGOMAXPROCS(t *testing.T) {
	run := func(procs int) ([]ParticipantAccountSnapshot, []ParticipantAccountSnapshot) {
		t.Helper()
		previous := runtime.GOMAXPROCS(procs)
		defer runtime.GOMAXPROCS(previous)
		sim, err := NewSim(3*time.Second, Config{
			LogDir:                     t.TempDir(),
			LogMode:                    "none",
			Seed:                       81,
			StrictPopulationAccounting: true,
		})
		if err != nil {
			t.Fatalf("NewSim: %v", err)
		}
		defer sim.Close()
		if err := sim.Run(context.Background()); err != nil {
			t.Fatalf("Run: %v", err)
		}
		return append([]ParticipantAccountSnapshot(nil), sim.InitialAccounts...), append([]ParticipantAccountSnapshot(nil), sim.TerminalAccounts...)
	}

	initialOne, terminalOne := run(1)
	initialMany, terminalMany := run(14)
	if !reflect.DeepEqual(initialOne, initialMany) || !reflect.DeepEqual(terminalOne, terminalMany) {
		t.Fatalf("strict population accounts differ by GOMAXPROCS:\n1=%#v\n14=%#v", struct {
			Initial  []ParticipantAccountSnapshot
			Terminal []ParticipantAccountSnapshot
		}{initialOne, terminalOne}, struct {
			Initial  []ParticipantAccountSnapshot
			Terminal []ParticipantAccountSnapshot
		}{initialMany, terminalMany})
	}
}

func TestCrossAssetPopulationAccountsDigestAcrossGOMAXPROCS(t *testing.T) {
	run := func(procs int) ([]ParticipantAccountSnapshot, []ParticipantAccountSnapshot) {
		t.Helper()
		previous := runtime.GOMAXPROCS(procs)
		defer runtime.GOMAXPROCS(previous)
		sim, err := NewSim(5*time.Minute, Config{
			LogDir:                     t.TempDir(),
			LogMode:                    "none",
			Seed:                       83,
			CrossAssetSpotGraph:        true,
			StrictPopulationAccounting: true,
			NoiseTraderCount:           2,
			OptionFlowCount:            2,
			VenueRules: map[string]VenueRule{
				"north":   {MatchingRule: MatchingPriceTime},
				"central": {MatchingRule: MatchingProRata},
				"south":   {MatchingRule: MatchingProRata},
			},
		})
		if err != nil {
			t.Fatalf("NewSim: %v", err)
		}
		defer sim.Close()
		if err := sim.Run(context.Background()); err != nil {
			t.Fatalf("Run: %v", err)
		}
		return append([]ParticipantAccountSnapshot(nil), sim.InitialAccounts...), append([]ParticipantAccountSnapshot(nil), sim.TerminalAccounts...)
	}

	initialOne, terminalOne := run(1)
	initialMany, terminalMany := run(14)
	if !reflect.DeepEqual(initialOne, initialMany) || !reflect.DeepEqual(terminalOne, terminalMany) {
		t.Fatalf("cross-asset population accounts differ by GOMAXPROCS:\n1=%#v\n14=%#v", struct {
			Initial  []ParticipantAccountSnapshot
			Terminal []ParticipantAccountSnapshot
		}{initialOne, terminalOne}, struct {
			Initial  []ParticipantAccountSnapshot
			Terminal []ParticipantAccountSnapshot
		}{initialMany, terminalMany})
	}
	if len(terminalOne) != 39 {
		t.Fatalf("cross-asset terminal account rows = %d, want 39", len(terminalOne))
	}
}

func TestManifestExcludesArtifactDirectory(t *testing.T) {
	build := func(logDir string) []byte {
		t.Helper()
		sim, err := NewSim(time.Second, Config{LogDir: logDir, LogMode: "none", Seed: 91})
		if err != nil {
			t.Fatalf("NewSim: %v", err)
		}
		defer sim.Close()
		data, err := os.ReadFile(filepath.Join(logDir, "manifest.json"))
		if err != nil {
			t.Fatalf("read manifest: %v", err)
		}
		return data
	}

	first := build(t.TempDir())
	second := build(t.TempDir())
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("manifest changes with artifact directory:\n%s\n%s", first, second)
	}
	if strings.Contains(string(first), "LogDir") || strings.Contains(string(first), "log_dir") {
		t.Fatalf("manifest leaks artifact directory: %s", first)
	}
}

func TestMixedVenueMatchingRulesDigestAcrossGOMAXPROCS(t *testing.T) {
	run := func(procs int) string {
		t.Helper()
		previous := runtime.GOMAXPROCS(procs)
		defer runtime.GOMAXPROCS(previous)

		sim, err := NewSim(3*time.Second, Config{
			LogDir: t.TempDir(),
			Seed:   63,
			VenueRules: map[string]VenueRule{
				"north":   {MatchingRule: MatchingPriceTime},
				"central": {MatchingRule: MatchingProRata},
				"south":   {MatchingRule: MatchingProRata},
			},
		})
		if err != nil {
			t.Fatalf("NewSim: %v", err)
		}
		if err := sim.Run(context.Background()); err != nil {
			sim.Close()
			t.Fatalf("Run: %v", err)
		}
		sim.Close()
		return digestVenueLogs(t, sim.Config.LogDir)
	}

	one := run(1)
	many := run(14)
	if one != many {
		t.Fatalf("mixed-matcher digest differs: GOMAXPROCS=1 %s, GOMAXPROCS=14 %s", one, many)
	}
}

func TestOptionBuyProbabilityDistinguishesOmittedAndZero(t *testing.T) {
	var allSell Config
	if err := json.Unmarshal([]byte(fmt.Sprintf(`{"log_dir":%q,"log_mode":"none","option_buy_probability":0}`, t.TempDir())), &allSell); err != nil {
		t.Fatalf("Unmarshal all-sell config: %v", err)
	}
	if err := allSell.normalize(); err != nil {
		t.Fatalf("normalize all-sell config: %v", err)
	}
	if allSell.OptionBuyProbability == nil || *allSell.OptionBuyProbability != 0 {
		t.Fatalf("all-sell probability = %#v, want explicit zero", allSell.OptionBuyProbability)
	}

	defaulted := Config{LogDir: t.TempDir(), LogMode: "none"}
	if err := defaulted.normalize(); err != nil {
		t.Fatalf("normalize default config: %v", err)
	}
	if defaulted.OptionBuyProbability == nil || *defaulted.OptionBuyProbability != 0.65 {
		t.Fatalf("default probability = %#v, want 0.65", defaulted.OptionBuyProbability)
	}
}

func TestNoLogModeRetainsRiskTelemetry(t *testing.T) {
	sim, err := NewSim(3*time.Second, Config{LogDir: t.TempDir(), LogMode: "none", Seed: 13})
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	defer sim.Close()
	if err := sim.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, venue := range sim.Venues {
		if venue.InitialRisk == nil || venue.TerminalRisk == nil || len(venue.RiskTimeline) == 0 {
			t.Fatalf("venue %s lost risk telemetry in no-log mode: %#v", venue.ID, venue)
		}
		if _, err := os.Stat(filepath.Join(sim.Config.LogDir, "venues", venue.ID, "general.jsonl")); !os.IsNotExist(err) {
			t.Fatalf("venue %s wrote raw logs in no-log mode: %v", venue.ID, err)
		}
	}
}

func TestConfigExpandsIndependentlySeededFlowRosters(t *testing.T) {
	sim, err := NewSim(3*time.Second, Config{
		LogDir:           t.TempDir(),
		LogMode:          "none",
		Seed:             91,
		NoiseTraderCount: 3,
		OptionFlowCount:  4,
	})
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	defer sim.Close()
	for _, venue := range sim.Venues {
		if len(venue.NoiseTraders) != 3 || len(venue.OptionFlows) != 4 {
			t.Fatalf("venue %s roster sizes = noise %d, option %d", venue.ID, len(venue.NoiseTraders), len(venue.OptionFlows))
		}
		if venue.NoiseTrader != venue.NoiseTraders[0] || venue.OptionFlow != venue.OptionFlows[0] {
			t.Fatalf("venue %s legacy participants do not point at roster heads", venue.ID)
		}
	}
	if err := sim.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestThreeVenueScenarioListsEveryDerivativeClass(t *testing.T) {
	sim, err := NewSim(3*time.Second, Config{LogDir: t.TempDir(), Seed: 11})
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	if len(sim.Venues) != 3 {
		sim.Close()
		t.Fatalf("venues = %d, want 3", len(sim.Venues))
	}
	if err := sim.Run(context.Background()); err != nil {
		sim.Close()
		t.Fatalf("Run: %v", err)
	}
	sim.Close()

	optionSymbol := regexp.MustCompile(`"symbol":"ABC-[0-9]+-[0-9]+-(?:C|P)"`)
	for _, venue := range sim.Venues {
		instruments := venue.Exchange.ListInstruments("", "")
		var spot, perp, future, option bool
		for _, inst := range instruments {
			switch inst.InstrumentType() {
			case "SPOT":
				spot = true
			case "PERP":
				perp = true
			case "FUTURE":
				future = true
			case "OPTION":
				option = true
			}
		}
		if !spot || !perp || !future || !option {
			t.Fatalf("venue %s instrument board incomplete: spot=%v perp=%v future=%v option=%v", venue.ID, spot, perp, future, option)
		}
		if venue.Exchange.BorrowingMgr == nil || !venue.Exchange.BorrowingMgr.Config.AutoBorrowSpot {
			t.Fatalf("venue %s does not have local spot auto-borrow enabled", venue.ID)
		}
		if venue.OptionDealerClientID == 0 || venue.InitialRisk == nil || venue.TerminalRisk == nil {
			t.Fatalf("venue %s has incomplete dealer risk telemetry: %#v", venue.ID, venue)
		}
		if venue.InitialRisk.Account.ReportAsset != "USD" || venue.TerminalRisk.Account.ReportAsset != "USD" || venue.TerminalRisk.Phase != "terminal_post_mark" {
			t.Fatalf("venue %s risk telemetry has wrong numeraire/phase: initial=%#v terminal=%#v", venue.ID, venue.InitialRisk, venue.TerminalRisk)
		}
		data, err := os.ReadFile(filepath.Join(sim.Config.LogDir, "venues", venue.ID, "derivatives.jsonl"))
		if err != nil {
			t.Fatalf("read %s derivatives log: %v", venue.ID, err)
		}
		text := string(data)
		if !strings.Contains(text, `"venue_id":"`+venue.ID+`"`) || !strings.Contains(text, "ABC-FUT-") || !optionSymbol.MatchString(text) {
			t.Fatalf("venue %s derivative log lacks provenance/classes", venue.ID)
		}
	}
}

func TestThreeVenueScenarioDigestAcrossGOMAXPROCS(t *testing.T) {
	run := func(procs int) string {
		t.Helper()
		previous := runtime.GOMAXPROCS(procs)
		defer runtime.GOMAXPROCS(previous)

		sim, err := NewSim(3*time.Second, Config{
			LogDir: t.TempDir(), Seed: 27, NoiseTraderCount: 2, OptionFlowCount: 2,
		})
		if err != nil {
			t.Fatalf("NewSim: %v", err)
		}
		if err := sim.Run(context.Background()); err != nil {
			sim.Close()
			t.Fatalf("Run: %v", err)
		}
		sim.Close()
		return digestVenueLogs(t, sim.Config.LogDir)
	}

	one := run(1)
	many := run(14)
	if one != many {
		t.Fatalf("three-venue digest differs: GOMAXPROCS=1 %s, GOMAXPROCS=14 %s", one, many)
	}
}

func TestThreeVenueCrossVenueRoutersUsePhaseOrderedIndependentLegs(t *testing.T) {
	sim, err := NewSim(6*time.Second, Config{
		LogDir:                   t.TempDir(),
		LogMode:                  "none",
		Seed:                     57,
		Step:                     time.Second,
		SnapshotInterval:         time.Second,
		AutomationInterval:       time.Second,
		QuoteInterval:            time.Second,
		NoiseInterval:            2 * time.Second,
		GreekInterval:            time.Second,
		ShortOptionTenor:         10 * time.Second,
		LongOptionTenor:          20 * time.Second,
		ShortFutureTenor:         12 * time.Second,
		LongFutureTenor:          24 * time.Second,
		CrossVenueArbTiers:       []float64{0.5, 1},
		CrossVenueBaseLatency:    2 * time.Second,
		CrossVenueArbLotQty:      mvBasePrecision / 100,
		CrossVenueArbMaxAttempts: 1,
	})
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	defer sim.Close()
	if len(sim.Routers) != 2 {
		t.Fatalf("routers = %d, want 2", len(sim.Routers))
	}
	for _, router := range sim.Routers {
		if len(router.Actors()) != 3 {
			t.Fatalf("router tier %g actors = %d, want 3", router.Tier(), len(router.Actors()))
		}
	}
	if err := sim.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, router := range sim.Routers {
		report := router.Report()
		if report.RouterID == 0 || report.PendingGroups != 0 || report.SubmittedGroups > 1 || report.CompletedGroups+report.FailedGroups != report.SubmittedGroups {
			t.Fatalf("router tier %g invalid report: %#v", router.Tier(), report)
		}
	}
}

func TestCrossVenueRouterTierOutcomesSurviveLabelSwapAndGOMAXPROCS(t *testing.T) {
	run := func(procs int, tiers []float64) map[float64]CrossVenueArbReport {
		t.Helper()
		previous := runtime.GOMAXPROCS(procs)
		defer runtime.GOMAXPROCS(previous)
		sim, err := NewSim(10*time.Minute, crossVenueRaceConfig(t.TempDir(), tiers))
		if err != nil {
			t.Fatalf("NewSim: %v", err)
		}
		defer sim.Close()
		if err := sim.Run(context.Background()); err != nil {
			t.Fatalf("Run: %v", err)
		}
		outcomes := make(map[float64]CrossVenueArbReport, len(sim.Routers))
		for _, router := range sim.Routers {
			report := normalizedCrossVenueReport(router.Report())
			outcomes[report.Tier] = report
		}
		return outcomes
	}

	forwardOne := run(1, []float64{0.5, 1})
	forwardMany := run(14, []float64{0.5, 1})
	reversed := run(14, []float64{1, 0.5})
	if !reflect.DeepEqual(forwardOne, forwardMany) {
		t.Fatalf("cross-venue outcomes differ by GOMAXPROCS:\n1=%#v\n14=%#v", forwardOne, forwardMany)
	}
	if !reflect.DeepEqual(forwardMany, reversed) {
		t.Fatalf("cross-venue outcomes differ after tier label swap:\nforward=%#v\nreversed=%#v", forwardMany, reversed)
	}
}

func crossVenueRaceConfig(logDir string, tiers []float64) Config {
	return Config{
		LogDir:                   logDir,
		LogMode:                  "none",
		Seed:                     42,
		Step:                     time.Second,
		SnapshotInterval:         time.Second,
		AutomationInterval:       time.Second,
		QuoteInterval:            time.Second,
		NoiseInterval:            2 * time.Second,
		GreekInterval:            time.Minute,
		ShortOptionTenor:         time.Hour,
		LongOptionTenor:          24 * time.Hour,
		ShortFutureTenor:         2 * time.Hour,
		LongFutureTenor:          30 * time.Hour,
		NoiseTraderCount:         4,
		OptionFlowCount:          4,
		DealerHedgeMode:          "off",
		CrossVenueArbTiers:       tiers,
		CrossVenueBaseLatency:    2 * time.Second,
		CrossVenueArbLotQty:      40_000_000,
		CrossVenueArbMaxAttempts: 1,
	}
}

func normalizedCrossVenueReport(report CrossVenueArbReport) CrossVenueArbReport {
	report.RouterID = 0
	for index := range report.Groups {
		report.Groups[index].Buy.ClientID = 0
		report.Groups[index].Buy.OrderID = 0
		report.Groups[index].Sell.ClientID = 0
		report.Groups[index].Sell.OrderID = 0
	}
	return report
}

func TestThreeVenuePreExpiryAndTerminalRiskAreDeterministic(t *testing.T) {
	run := func(procs int) []VenueRiskSnapshot {
		t.Helper()
		previous := runtime.GOMAXPROCS(procs)
		defer runtime.GOMAXPROCS(previous)
		sim, err := NewSim(4*time.Second, Config{
			LogDir:             t.TempDir(),
			Seed:               37,
			ShortOptionTenor:   2 * time.Second,
			LongOptionTenor:    8 * time.Second,
			ShortFutureTenor:   3 * time.Second,
			LongFutureTenor:    9 * time.Second,
			GreekInterval:      time.Second,
			SnapshotInterval:   time.Second,
			AutomationInterval: time.Second,
			QuoteInterval:      time.Second,
			NoiseInterval:      2 * time.Second,
		})
		if err != nil {
			t.Fatalf("NewSim: %v", err)
		}
		if err := sim.Run(context.Background()); err != nil {
			sim.Close()
			t.Fatalf("Run: %v", err)
		}
		defer sim.Close()
		result := make([]VenueRiskSnapshot, 0, len(sim.Venues)*4)
		for _, venue := range sim.Venues {
			if venue.InitialRisk == nil || venue.TerminalRisk == nil || len(venue.RiskTimeline) == 0 || len(venue.PreExpiryRisk) == 0 {
				t.Fatalf("venue %s missing lifecycle risk rows: initial=%#v timeline=%#v pre=%#v terminal=%#v", venue.ID, venue.InitialRisk, venue.RiskTimeline, venue.PreExpiryRisk, venue.TerminalRisk)
			}
			result = append(result, *venue.InitialRisk)
			result = append(result, venue.RiskTimeline...)
			result = append(result, venue.PreExpiryRisk...)
			result = append(result, *venue.TerminalRisk)
		}
		return result
	}

	one := run(1)
	many := run(14)
	if !reflect.DeepEqual(one, many) {
		t.Fatalf("risk snapshots differ by GOMAXPROCS:\n1: %#v\n14: %#v", one, many)
	}
}

func TestRiskTimelineRetainsPositiveHorizonExchangeGreeksBeforeExpiry(t *testing.T) {
	sim, err := NewSim(3*time.Second, Config{
		LogDir: t.TempDir(), Seed: 41,
		ShortOptionTenor:   8 * time.Second,
		LongOptionTenor:    12 * time.Second,
		ShortFutureTenor:   9 * time.Second,
		LongFutureTenor:    13 * time.Second,
		GreekInterval:      time.Minute,
		AutomationInterval: time.Second,
	})
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	defer sim.Close()
	venue := sim.Venues[0]
	now := venue.Exchange.Clock.NowUnixNano()
	option := exchange.NewEuropeanOption(
		"ABC-MANUAL-2-C", "ABC", "USD", "ABC/USD", mvBasePrecision, mvQuotePrecision,
		mvQuotePrecision, mvBasePrecision/100, mvBootstrapPrice, now+6*int64(time.Second), true,
	)
	option.IV = sim.Config.OptionIV
	option.SetMarks(mvBootstrapPrice, 10*mvQuotePrecision)
	venue.Exchange.AddInstrument(option)
	venue.Exchange.Positions.UpdatePosition(venue.OptionDealerClientID, option.Symbol(), mvBasePrecision/10, 10*mvQuotePrecision, exchange.Buy, exchange.PositionBoth)
	venue.Exchange.AddPerpBalance(venue.OptionDealerClientID, "USD", -mvQuotePrecision)
	var spotMakerClientID uint64
	for _, participant := range venue.Participants {
		if participant.Role == "spot_maker_1" {
			spotMakerClientID = participant.ClientID
			break
		}
	}
	if spotMakerClientID == 0 {
		t.Fatal("spot-maker fixture client is missing")
	}
	for requestID, quote := range []struct {
		side  exchange.Side
		price int64
	}{
		{side: exchange.Buy, price: mvBootstrapPrice - 10*mvQuotePrecision},
		{side: exchange.Sell, price: mvBootstrapPrice + 10*mvQuotePrecision},
	} {
		response := venue.Exchange.PlaceOrder(spotMakerClientID, &exchange.OrderRequest{
			RequestID: uint64(9_001 + requestID), Symbol: "ABC/USD", Side: quote.side,
			Type: exchange.LimitOrder, Price: quote.price, Qty: mvBasePrecision,
			TimeInForce: exchange.GTC, Visibility: exchange.Normal,
		})
		if !response.Success {
			t.Fatalf("seed declared ABC/USD reference quote: %#v", response)
		}
	}

	if err := sim.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, snapshot := range venue.RiskTimeline {
		for _, position := range snapshot.GreekPositions {
			if position.Symbol == option.Symbol() {
				if position.TimeToExpiryNano <= 0 || position.ModelForward <= 0 || position.ForwardSource != "option_underlying_mark" {
					t.Fatalf("invalid positive-horizon exchange Greeks: %#v", position)
				}
				return
			}
		}
	}
	t.Fatalf("risk timeline omitted positive-horizon Greek position for %s: %#v", option.Symbol(), venue.RiskTimeline)
}

func digestVenueLogs(t *testing.T, dir string) string {
	t.Helper()
	var files []string
	if err := filepath.WalkDir(filepath.Join(dir, "venues"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("WalkDir: %v", err)
	}
	sort.Strings(files)
	hash := sha256.New()
	for _, path := range files {
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			t.Fatalf("Rel: %v", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		fmt.Fprintf(hash, "%s\x00", rel)
		hash.Write(data)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

// Regression: every quote-denominated Avellaneda-Stoikov parameter must be
// converted when a book is quoted in a different currency, not just variance.
//
// Risk aversion and fill decay have reciprocal quote-price units. Converting
// variance alone left the ABC/CDF inventory term smaller than one tick, so the
// cross-pair maker posted its bootstrap quote once and never repriced: in a
// two-minute probe it accepted 2 orders and issued 0 cancels while the CDF/USD
// makers on the same cadence cycled ~190 quotes. The control law must be scale
// invariant: the same base inventory must produce the same *relative*
// reservation shift and half spread on both books.
func TestCrossPairStoikovControlIsQuoteScaleInvariant(t *testing.T) {
	sim, err := NewSim(time.Second, Config{
		LogDir:              t.TempDir(),
		LogMode:             "none",
		Seed:                73,
		CrossAssetSpotGraph: true,
	})
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	defer sim.Close()

	relative := func(cfg StoikovMMConfig, inventory float64) (shift, half float64) {
		forward := float64(cfg.BootstrapPrice) / float64(cfg.QuotePrecision)
		quote, ok := CalculateStoikovQuote(StoikovInputs{
			Forward:           forward,
			Inventory:         inventory,
			VariancePerSecond: cfg.InitialLogVariancePerSec * forward * forward,
			RiskAversion:      cfg.RelativeRiskAversion / forward,
			FillDecay:         cfg.RelativeFillDecay / forward,
			InventoryHorizon:  cfg.InventoryHorizon,
		})
		if !ok {
			t.Fatalf("quote invalid for %s", cfg.Symbol)
		}
		return (forward - quote.Reservation) / forward, quote.HalfSpread / forward
	}

	venue := sim.Venues[0]
	var usd, cross *StoikovMMConfig
	for _, maker := range venue.SpotMakers {
		switch maker.cfg.Symbol {
		case "ABC/USD":
			cfg := maker.cfg
			usd = &cfg
		case "ABC/CDF":
			cfg := maker.cfg
			cross = &cfg
		}
	}
	if usd == nil || cross == nil {
		t.Fatal("expected both an ABC/USD and an ABC/CDF maker")
	}

	const inventory = 1.0
	usdShift, usdHalf := relative(*usd, inventory)
	crossShift, crossHalf := relative(*cross, inventory)
	if usdShift <= 0 {
		t.Fatalf("baseline inventory shift must be positive, got %.18g", usdShift)
	}
	if math.Abs(crossShift-usdShift) > 1e-12 {
		t.Fatalf("relative inventory shift not scale invariant: ABC/USD=%.18g ABC/CDF=%.18g", usdShift, crossShift)
	}
	if math.Abs(crossHalf-usdHalf) > 1e-12 {
		t.Fatalf("relative half spread not scale invariant: ABC/USD=%.18g ABC/CDF=%.18g", usdHalf, crossHalf)
	}

	// The inventory skew must at least be positive and scale invariant. Whether
	// it clears a tick is a property of the calibration, not of the control
	// law: at the current risk aversion the skew is far below one tick, so
	// makers here are effectively fixed-spread quoters that do not price
	// inventory. That limitation is recorded in the research notes (FFA-16 and
	// FFA-17) rather than asserted here, because asserting it would make this
	// test fail for a reason it is not testing.
	if crossShift <= 0 || usdShift <= 0 {
		t.Fatalf("inventory shift must be positive: ABC/USD=%.18g ABC/CDF=%.18g", usdShift, crossShift)
	}
}

// Regression: the market-making control must not bootstrap its own volatility.
//
// The variance estimator originally measured absolute price changes of the
// maker's own reference mid. Any move widened the quote, which produced a
// larger move, which raised the variance again. In a 90-minute run the north
// ABC/USD book held ~50,000 USD for 650 seconds and then went 52,060 ->
// 41,000,000 USD in seven seconds, after which every bid was rejected for
// insufficient balance and the book stayed one-sided for the rest of the run.
//
// Log-return variance with relative risk parameters is scale invariant, so a
// price level cannot feed back into the width of the quote around it.
func TestSpotBookStaysAnchoredPastVarianceFeedbackHorizon(t *testing.T) {
	if testing.Short() {
		t.Skip("long-horizon stability run")
	}
	sim, err := NewSim(20*time.Minute, Config{
		LogDir:              t.TempDir(),
		LogMode:             "none",
		Seed:                91,
		NoiseTraderCount:    2,
		OptionFlowCount:     2,
		CrossAssetSpotGraph: true,
	})
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	defer sim.Close()
	if err := sim.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	const bootstrap = float64(mvBootstrapPrice) / float64(mvQuotePrecision)
	for _, venue := range sim.Venues {
		mid, ok := venue.Exchange.TwoSidedMidPrice("ABC/USD")
		if !ok {
			t.Fatalf("venue %s ABC/USD is not two-sided after 20 simulated minutes", venue.ID)
		}
		price := float64(mid) / float64(mvQuotePrecision)
		// A wide but finite band: this asserts the absence of runaway feedback,
		// not a particular price path.
		if price < bootstrap/2 || price > bootstrap*2 {
			t.Fatalf("venue %s ABC/USD mid %.2f left the stability band around %.2f", venue.ID, price, bootstrap)
		}
	}
}

// The tick size must be configurable and must actually reach the spot book:
// the spread cannot be finer than one tick, so a coarse tick pins it at the
// floor and makes every spread measurement insensitive to market conditions.
func TestSpotTickSizeIsConfigurableAndReachesTheBook(t *testing.T) {
	for _, tick := range []int64{mvQuotePrecision, 10 * mvQuotePrecision} {
		sim, err := NewSim(time.Second, Config{
			LogDir: t.TempDir(), LogMode: "none", Seed: 91,
			SpotTickQuoteUnits: tick,
		})
		if err != nil {
			t.Fatalf("NewSim(tick=%d): %v", tick, err)
		}
		for _, venue := range sim.Venues {
			instrument := venue.Exchange.Books["ABC/USD"].Instrument
			if got := instrument.TickSize(); got != tick {
				sim.Close()
				t.Fatalf("venue %s ABC/USD tick = %d, want %d", venue.ID, got, tick)
			}
		}
		sim.Close()
	}
}

// Microstructure sampling must report a two-sided spread, a trade count, and a
// per-trade volatility, since those three are what the spread-volatility test
// regresses.
func TestMicrostructureStatsReportSpreadAndPerTradeVolatility(t *testing.T) {
	stats := newMicrostructureStats("v", "ABC/USD", 100, 1)
	// Two-sided samples with a moving midpoint and an advancing trade count.
	stats.observe(9_900, 10_100, 0)
	stats.observe(9_950, 10_150, 4)
	stats.observe(9_900, 10_100, 8)
	// A one-sided sample is skipped and breaks the return series.
	stats.observe(0, 10_100, 10)
	stats.observe(9_900, 10_100, 12)
	stats.finalize()

	if stats.Samples != 4 {
		t.Fatalf("samples = %d, want 4 two-sided observations", stats.Samples)
	}
	if stats.Trades != 12 {
		t.Fatalf("trades = %d, want 12", stats.Trades)
	}
	if stats.MeanSpreadTicks != 2 {
		t.Fatalf("mean spread = %v ticks, want 2", stats.MeanSpreadTicks)
	}
	if stats.SigmaPerSample <= 0 {
		t.Fatal("per-sample volatility must be positive when the midpoint moves")
	}
	// Three trades per sample must scale the per-trade figure down by sqrt(3).
	want := stats.SigmaPerSample / math.Sqrt(3)
	if math.Abs(stats.SigmaPerTrade-want) > 1e-12 {
		t.Fatalf("per-trade volatility = %v, want %v", stats.SigmaPerTrade, want)
	}
}

// A metaorder must terminate. Without a horizon, a parent whose side of the
// book is empty waits forever for liquidity that never arrives, the agent
// never starts another parent, and a four-hour run produced five measurements
// instead of a hundred and seventy.
func TestMetaorderTraderAbandonsAParentItCannotComplete(t *testing.T) {
	sim, err := NewSim(4*time.Minute, Config{
		LogDir: t.TempDir(), LogMode: "none", Seed: 91,
		SpotTickQuoteUnits: 10_000,
		MetaorderTraders: &MetaorderTraderConfig{
			MinQty: 2_000_000, MaxQty: 200_000_000, ParetoAlpha: 1.5,
			ChildInterval: time.Second, ParticipationRate: 0.05,
			MinChildQty: 500_000, RestInterval: 10 * time.Second,
			MaxDuration: 30 * time.Second,
		},
	})
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	defer sim.Close()
	if err := sim.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	records := 0
	for _, venue := range sim.Venues {
		for _, trader := range venue.MetaorderTraders {
			for _, record := range trader.Records() {
				records++
				if record.EndTimestamp-record.StartTimestamp > int64(30*time.Second) {
					t.Fatalf("parent ran %v, beyond its horizon", time.Duration(record.EndTimestamp-record.StartTimestamp))
				}
				if record.ParentQty <= 0 || record.ChildCount == 0 {
					t.Fatalf("degenerate record: %+v", record)
				}
			}
		}
	}
	if records < 3 {
		t.Fatalf("expected several parents in four simulated minutes, got %d", records)
	}
}

// A metaorder configuration that cannot execute must be rejected rather than
// reaching the timer factory, where a zero interval panics.
func TestMetaorderConfigIsValidated(t *testing.T) {
	_, err := NewSim(time.Minute, Config{
		LogDir: t.TempDir(), LogMode: "none", Seed: 91,
		MetaorderTraders: &MetaorderTraderConfig{MinQty: 1, ParetoAlpha: 1.5, MinChildQty: 1},
	})
	if err == nil {
		t.Fatal("a zero child interval must be rejected")
	}
}

// The participation rate must be measured against volume traded by everyone
// else. The trade feed includes the agent's own fills, so pacing against total
// volume is self-feeding: each child enlarges the allowance for the next, and
// the realised rate runs away from the configured one.
func TestMetaorderParticipationExcludesOwnVolume(t *testing.T) {
	trader := &MetaorderTrader{cfg: MetaorderTraderConfig{
		BasePrecision: mvBasePrecision, ParticipationRate: 0.10, MinChildQty: 1,
	}}

	// 1000 units traded in total, 900 of them by this agent: only 100 are
	// external, so a 10% child is 10 units, not 100.
	trader.marketVolume = 1000
	trader.ownVolume = 900
	trader.childVolume = 0
	if got := trader.childQty(1_000_000); got != 10 {
		t.Fatalf("child quantity = %d, want 10 from 100 units of external volume", got)
	}

	// With no external volume at all the child falls back to the floor rather
	// than growing on the agent's own prints.
	trader.marketVolume, trader.ownVolume, trader.childVolume = 500, 500, 0
	if got := trader.childQty(1_000_000); got != 1 {
		t.Fatalf("child quantity = %d, want the configured floor", got)
	}
}

// The maker anchor selects what the spot makers quote around. Anchoring to a
// published index is what stops the book midpoint from reproducing itself.
func TestMakerAnchorSelectsTheQuotedReference(t *testing.T) {
	for _, testCase := range []struct {
		anchor    string
		wantIndex bool
		wantFeeds int
	}{
		{anchor: "own_mid", wantIndex: false, wantFeeds: 0},
		{anchor: "consensus", wantIndex: true, wantFeeds: 1},
	} {
		// Anchor plumbing check, not a strategy measurement, so the undegraded
		// feed is the instrument under test.
		sim, err := NewSim(time.Second, Config{
			LogDir: t.TempDir(), LogMode: "none", Seed: 91, MakerAnchor: testCase.anchor,
		})
		if err != nil {
			t.Fatalf("NewSim(%s): %v", testCase.anchor, err)
		}
		for _, venue := range sim.Venues {
			for _, maker := range venue.SpotMakers {
				if maker.cfg.Symbol != "ABC/USD" {
					continue
				}
				if maker.cfg.AnchorToIndex != testCase.wantIndex {
					sim.Close()
					t.Fatalf("%s: maker anchor to index = %v, want %v", testCase.anchor, maker.cfg.AnchorToIndex, testCase.wantIndex)
				}
			}
		}
		sim.Close()
	}

	if _, err := NewSim(time.Second, Config{LogDir: t.TempDir(), LogMode: "none", Seed: 91, MakerAnchor: "nonsense"}); err == nil {
		t.Fatal("an unknown anchor must be rejected")
	}
}

// The first V2-1 smoke mechanism is deliberately modest: each spot maker
// copies only snapshots delivered through its own delayed public link. The
// independent V2-0 auditor must then show every resulting order cites that
// link's delivered frontier. This is not yet a cross-venue composite claim.
func TestSpotMakerLocalReferenceCacheHasAuditableDelayedDecisionFrontier(t *testing.T) {
	dir := t.TempDir()
	sim, err := NewSim(20*time.Second, Config{
		LogDir: dir, LogMode: "none", Seed: 101,
		MakerAnchor:                  "own_mid",
		SpotMakerLocalReferenceCache: true,
		RecordMarketDataReceipts:     true,
		MarketDataReceiptRoles:       []string{"spot_maker"},
		LatencyProfiles: map[string]LatencyProfile{
			"spot_maker": {Model: "constant", Delay: 10 * time.Millisecond},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sim.Close()
	if err := sim.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, venue := range sim.Venues {
		for _, maker := range venue.SpotMakers {
			if maker.cfg.Symbol != "ABC/USD" {
				continue
			}
			view, ok := maker.LocalReferenceView()
			if !ok || view.SourceVenue != venue.ID || view.Symbol != "ABC/USD" || view.Updates == 0 {
				t.Fatalf("%s maker cache did not activate from its declared feed: %+v, %t", venue.ID, view, ok)
			}
		}
	}
	audit, err := analysis.AuditMarketDataReceipts(dir)
	if err != nil || !audit.Valid || audit.Receipts == 0 || audit.Decisions == 0 {
		t.Fatalf("local-cache evidence is not independently auditable: audit=%+v err=%v", audit, err)
	}
}

func TestSpotMakerLocalReferenceCacheRejectsGlobalOrUndelayedPath(t *testing.T) {
	for _, config := range []Config{
		{LogDir: t.TempDir(), LogMode: "none", MakerAnchor: "consensus", SpotMakerLocalReferenceCache: true,
			LatencyProfiles: map[string]LatencyProfile{"spot_maker": {Model: "constant", Delay: time.Millisecond}}},
		{LogDir: t.TempDir(), LogMode: "none", MakerAnchor: "own_mid", SpotMakerLocalReferenceCache: true},
	} {
		if _, err := NewSim(time.Second, config); err == nil {
			t.Fatalf("local reference cache accepted prohibited input path: %+v", config)
		}
	}
}

// V2-1c is deliberately one maker, one remote source, and one composite
// weight. Its purpose is to prove the information path before a heterogeneous
// maker roster or economic attribution is attempted. Every quoted order from
// the target maker must bind both actor-owned feed frontiers.
func TestRemoteMakerFeedProducesAuditableTwoFeedDecisionFrontier(t *testing.T) {
	dir := t.TempDir()
	sim, err := NewSim(20*time.Second, Config{
		LogDir: dir, LogMode: "none", Seed: 101,
		MakerAnchor:                   "own_mid",
		SpotMakerLocalReferenceCache:  true,
		RecordMarketDataReceipts:      true,
		RecordDecisionFrontierVectors: true,
		MarketDataReceiptRoles:        []string{"spot_maker", "v2_remote_feed"},
		LatencyProfiles: map[string]LatencyProfile{
			"spot_maker": {Model: "constant", Delay: 10 * time.Millisecond},
		},
		RemoteMakerFeed: &RemoteMakerFeedConfig{
			TargetVenue: "north", SourceVenue: "south", Symbol: "ABC/USD", Weight: 0.5,
			Latency: LatencyProfile{Model: "constant", Delay: 20 * time.Millisecond},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sim.Close()
	if err := sim.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	var north, south *Venue
	for _, venue := range sim.Venues {
		switch venue.ID {
		case "north":
			north = venue
		case "south":
			south = venue
		}
	}
	if north == nil || south == nil || len(north.SpotMakers) == 0 || len(south.FeedSessions) != 1 {
		t.Fatalf("remote feed setup north=%v south=%v source sessions=%d", north != nil, south != nil, len(south.FeedSessions))
	}
	if got := south.FeedSessions[0]; got.Role != "v2_remote_feed" || got.VenueID != "south" || got.ClientID == 0 {
		t.Fatalf("unexpected remote feed session: %+v", got)
	}
	var target *StoikovMarketMaker
	for _, maker := range north.SpotMakers {
		if maker.cfg.Symbol == "ABC/USD" {
			target = maker
			break
		}
	}
	if target == nil || target.cfg.AnchorToIndex {
		t.Fatal("target maker retained a shared index path")
	}
	remote, active := target.RemoteReferenceView()
	if !active || remote.SourceVenue != "south" || remote.Symbol != "ABC/USD" || remote.Updates == 0 {
		t.Fatalf("target remote cache did not activate: %+v active=%t", remote, active)
	}
	scalar, err := analysis.AuditMarketDataReceipts(dir)
	if err != nil || !scalar.Valid || scalar.Receipts == 0 || scalar.Decisions == 0 {
		t.Fatalf("remote scalar evidence invalid: audit=%+v err=%v", scalar, err)
	}
	vectors, err := analysis.AuditDecisionFrontierVectors(dir)
	if err != nil || !vectors.Valid || vectors.Decisions == 0 || vectors.Components != 2*vectors.Decisions {
		t.Fatalf("remote two-feed frontier invalid: audit=%+v err=%v", vectors, err)
	}
}

func TestRemoteMakerFeedRejectsIncompleteEvidenceContract(t *testing.T) {
	base := Config{
		LogDir: t.TempDir(), LogMode: "none", Seed: 101,
		MakerAnchor:                   "own_mid",
		SpotMakerLocalReferenceCache:  true,
		RecordMarketDataReceipts:      true,
		RecordDecisionFrontierVectors: true,
		MarketDataReceiptRoles:        []string{"spot_maker"},
		LatencyProfiles: map[string]LatencyProfile{
			"spot_maker": {Model: "constant", Delay: time.Millisecond},
		},
		RemoteMakerFeed: &RemoteMakerFeedConfig{
			TargetVenue: "north", SourceVenue: "south", Symbol: "ABC/USD", Weight: 0.5,
			Latency: LatencyProfile{Model: "constant", Delay: time.Millisecond},
		},
	}
	if _, err := NewSim(time.Second, base); err == nil || !strings.Contains(err.Error(), "v2_remote_feed") {
		t.Fatalf("remote feed accepted incomplete evidence contract: %v", err)
	}
}

// V2-1d uses three distinct feed policies but remains a construction smoke:
// it proves heterogeneous local information, not a price-discovery outcome.
func TestHeterogeneousRemoteMakerRosterHasAuditableLocalFrontiers(t *testing.T) {
	dir := t.TempDir()
	feeds := []RemoteMakerFeedConfig{
		{TargetVenue: "north", TargetMaker: 1, SourceVenue: "south", Symbol: "ABC/USD", Weight: 0.50, Confidence: 0.80, MaxObservationAge: 2 * time.Second, Latency: LatencyProfile{Model: "constant", Delay: 10 * time.Millisecond}},
		{TargetVenue: "central", TargetMaker: 1, SourceVenue: "north", Symbol: "ABC/USD", Weight: 0.35, Confidence: 0.90, MaxObservationAge: 4 * time.Second, Latency: LatencyProfile{Model: "constant", Delay: 20 * time.Millisecond}},
		{TargetVenue: "south", TargetMaker: 1, SourceVenue: "central", Symbol: "ABC/USD", Weight: 0.45, Confidence: 0.60, MaxObservationAge: 6 * time.Second, Latency: LatencyProfile{Model: "constant", Delay: 30 * time.Millisecond}},
	}
	sim, err := NewSim(20*time.Second, Config{
		LogDir: dir, LogMode: "none", Seed: 101, SpotMakerCount: 2,
		MakerAnchor:                   "own_mid",
		SpotMakerLocalReferenceCache:  true,
		RecordMarketDataReceipts:      true,
		RecordDecisionFrontierVectors: true,
		MarketDataReceiptRoles:        []string{"spot_maker", "v2_remote_feed"},
		LatencyProfiles: map[string]LatencyProfile{
			"spot_maker": {Model: "constant", Delay: 10 * time.Millisecond},
		},
		RemoteMakerFeeds: feeds,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sim.Close()
	if err := sim.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	wantSource := map[string]string{"north": "south", "central": "north", "south": "central"}
	feedSessions := 0
	for _, venue := range sim.Venues {
		feedSessions += len(venue.FeedSessions)
		var abcMakers []*StoikovMarketMaker
		for _, maker := range venue.SpotMakers {
			if maker.cfg.Symbol == "ABC/USD" {
				abcMakers = append(abcMakers, maker)
			}
		}
		if len(abcMakers) != 2 {
			t.Fatalf("%s ABC/USD makers = %d, want 2", venue.ID, len(abcMakers))
		}
		remote, active := abcMakers[0].RemoteReferenceView()
		if !active || remote.SourceVenue != wantSource[venue.ID] || remote.Updates == 0 || abcMakers[0].bidPrice == 0 || abcMakers[0].askPrice == 0 {
			t.Fatalf("%s informed maker did not activate/quote: remote=%+v active=%t bid/ask=%d/%d", venue.ID, remote, active, abcMakers[0].bidPrice, abcMakers[0].askPrice)
		}
		if _, active := abcMakers[1].RemoteReferenceView(); active {
			t.Fatalf("%s local-only maker unexpectedly received a remote feed", venue.ID)
		}
	}
	if feedSessions != len(feeds) {
		t.Fatalf("remote feed sessions = %d, want %d", feedSessions, len(feeds))
	}
	scalar, err := analysis.AuditMarketDataReceipts(dir)
	if err != nil || !scalar.Valid || scalar.Decisions == 0 {
		t.Fatalf("roster scalar evidence invalid: audit=%+v err=%v", scalar, err)
	}
	vectors, err := analysis.AuditDecisionFrontierVectors(dir)
	if err != nil || !vectors.Valid || vectors.Decisions == 0 || vectors.Components != 2*vectors.Decisions {
		t.Fatalf("roster vector evidence invalid: audit=%+v err=%v", vectors, err)
	}
}

func TestHeterogeneousRemoteMakerRosterRejectsDecorativePolicyFields(t *testing.T) {
	config := Config{
		LogDir: t.TempDir(), LogMode: "none", Seed: 101, SpotMakerCount: 2,
		MakerAnchor:                   "own_mid",
		SpotMakerLocalReferenceCache:  true,
		RecordMarketDataReceipts:      true,
		RecordDecisionFrontierVectors: true,
		MarketDataReceiptRoles:        []string{"spot_maker", "v2_remote_feed"},
		LatencyProfiles: map[string]LatencyProfile{
			"spot_maker": {Model: "constant", Delay: time.Millisecond},
		},
		RemoteMakerFeeds: []RemoteMakerFeedConfig{{
			TargetVenue: "north", TargetMaker: 1, SourceVenue: "south", Symbol: "ABC/USD", Weight: 0.5,
			Latency: LatencyProfile{Model: "constant", Delay: time.Millisecond},
		}},
	}
	if _, err := NewSim(time.Second, config); err == nil || !strings.Contains(err.Error(), "confidence") {
		t.Fatalf("roster accepted omitted confidence/horizon policy: %v", err)
	}
	config.RemoteMakerFeeds[0].Confidence = 0.8
	config.RemoteMakerFeeds[0].MaxObservationAge = time.Second
	config.RemoteMakerFeeds = append(config.RemoteMakerFeeds, config.RemoteMakerFeeds[0])
	if _, err := NewSim(time.Second, config); err == nil || !strings.Contains(err.Error(), "duplicate remote maker feed target") {
		t.Fatalf("roster accepted duplicate target maker: %v", err)
	}
}

// The consensus index is a median of the venues' midpoints and must ignore a
// single venue that has run away, which is the property that lets it hold a
// market that cannot hold itself.
func TestConsensusIndexIsRobustToOneRunawayVenue(t *testing.T) {
	provider := newSpotIndexProvider("consensus", "ABC/USD")
	if got, err := provider.Price("ABC/USD"); err == nil || got != 0 {
		t.Fatalf("index without observations = (%d, %v), want unavailable", got, err)
	}
	provider.observeVenueMid("ABC/USD", "north", 50_000)
	provider.observeVenueMid("ABC/USD", "central", 50_100)
	provider.observeVenueMid("ABC/USD", "south", 5_000_000)
	if got, err := provider.Price("ABC/USD"); err != nil || got != 50_100 {
		t.Fatalf("consensus = (%d, %v), want (50100, nil)", got, err)
	}
	if got, err := provider.Price("OTHER/USD"); err == nil || got != 0 {
		t.Fatalf("index for an unpublished symbol = (%d, %v), want unavailable", got, err)
	}
}

func TestConsensusIndexRetainsSignedObservedMidpoints(t *testing.T) {
	provider := newSpotIndexProvider("consensus", "OIL-FUT")
	provider.observeVenueMid("OIL-FUT", "north", -20)
	provider.observeVenueMid("OIL-FUT", "central", 0)
	provider.observeVenueMid("OIL-FUT", "south", 10)
	if got, err := provider.Price("OIL-FUT"); err != nil || got != 0 {
		t.Fatalf("signed consensus = (%d, %v), want present zero", got, err)
	}
}

// A round-trip participant must return to flat, which is the property that
// distinguishes it from random-side flow: a fresh coin flip each tick
// mean-reverts in price but leaves the participant's net position a random
// walk, and market-maker inventory is the mirror of that walk.
func TestRoundTripTraderReturnsToFlat(t *testing.T) {
	sim, err := NewSim(30*time.Minute, Config{
		LogDir: t.TempDir(), LogMode: "none", Seed: 91,
		RoundTripTraderCount: 4,
		RoundTripHold:        time.Minute,
		RoundTripLotQty:      mvBasePrecision / 10,
	})
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	defer sim.Close()
	if err := sim.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	completed := 0
	for _, venue := range sim.Venues {
		if len(venue.RoundTripTraders) != 4 {
			t.Fatalf("venue %s round-trip traders = %d, want 4", venue.ID, len(venue.RoundTripTraders))
		}
		for _, trader := range venue.RoundTripTraders {
			completed += trader.RoundTrips()
			// A position may be open at the cutoff, but it must be at most one
			// lot: anything larger means unwinding is not keeping up.
			if position := trader.Position(); position > mvBasePrecision/10 || position < -mvBasePrecision/10 {
				t.Fatalf("round-trip trader ended %d base units from flat", position)
			}
		}
	}
	if completed == 0 {
		t.Fatal("no round trips completed in thirty simulated minutes")
	}
}

// The supplier's target position must fall as the price rises, which is what
// makes it supply into a drift rather than chase it, and must stay inside its
// position limit at any price.
func TestElasticSupplierTargetSlopesDownAndIsBounded(t *testing.T) {
	supplier := &ElasticSupplier{cfg: ElasticSupplierConfig{
		ReferencePrice:       50_000 * mvQuotePrecision,
		BaseHolding:          0,
		ElasticityPerPercent: 50 * mvBasePrecision,
		MaxPosition:          10_000 * mvBasePrecision,
	}}

	atReference := supplier.TargetPosition(50_000 * mvQuotePrecision)
	if atReference != 0 {
		t.Fatalf("target at the reference price = %d, want the base holding 0", atReference)
	}
	// One percent above the reference: short exactly the elasticity.
	if got := supplier.TargetPosition(50_500 * mvQuotePrecision); got != -50*mvBasePrecision {
		t.Fatalf("target one percent above reference = %d, want %d", got, -50*mvBasePrecision)
	}
	// One percent below: long the same amount.
	if got := supplier.TargetPosition(49_500 * mvQuotePrecision); got != 50*mvBasePrecision {
		t.Fatalf("target one percent below reference = %d, want %d", got, 50*mvBasePrecision)
	}
	// Far from the reference the target saturates rather than growing without
	// bound.
	if got := supplier.TargetPosition(500_000 * mvQuotePrecision); got != -10_000*mvBasePrecision {
		t.Fatalf("target far above reference = %d, want the position limit", got)
	}
	// The long side needs no clamp: a price can only fall by one hundred
	// percent, so the target tops out at a hundred times the elasticity.
	if got := supplier.TargetPosition(1); got <= 4_999*mvBasePrecision || got > 5_000*mvBasePrecision {
		t.Fatalf("target at a near-zero price = %d, want just under %d", got, 5_000*mvBasePrecision)
	}
}

// Displayed quote size is the constraint that bounds how fast risk can move
// between participants, so it must be configurable and must reach the makers.
// An immediate-or-cancel taker cannot lift more than is displayed, which is
// why a delta-neutral absorber saturates at the book's size rather than at its
// own capital.
func TestMakerQuoteSizeIsConfigurableAndReachesTheMakers(t *testing.T) {
	for _, want := range []int64{mvBasePrecision / 5, 5 * mvBasePrecision} {
		sim, err := NewSim(time.Second, Config{
			LogDir: t.TempDir(), LogMode: "none", Seed: 91, MakerQuoteQty: want,
		})
		if err != nil {
			t.Fatalf("NewSim(%d): %v", want, err)
		}
		for _, venue := range sim.Venues {
			for _, maker := range venue.SpotMakers {
				if maker.cfg.QuoteQty != want {
					sim.Close()
					t.Fatalf("venue %s %s quote size = %d, want %d", venue.ID, maker.cfg.Symbol, maker.cfg.QuoteQty, want)
				}
			}
			if venue.PerpMaker.cfg.QuoteQty != want {
				sim.Close()
				t.Fatalf("venue %s perpetual maker quote size = %d, want %d", venue.ID, venue.PerpMaker.cfg.QuoteQty, want)
			}
		}
		sim.Close()
	}
}

// Impact is a statistical measurement over many parent orders, so the number
// of execution agents must be configurable: one per venue produced 29 usable
// metaorders in six simulated hours, which is far too few to separate impact
// from background volatility.
func TestMetaorderTraderCountIsConfigurable(t *testing.T) {
	sim, err := NewSim(2*time.Second, Config{
		LogDir: t.TempDir(), LogMode: "none", Seed: 91,
		MetaorderTraderCount: 4,
		MetaorderTraders: &MetaorderTraderConfig{
			MinQty: 2_000_000, MaxQty: 200_000_000, ParetoAlpha: 1.3,
			ChildInterval: time.Second, ParticipationRate: 0.05,
			MinChildQty: 500_000, RestInterval: 10 * time.Second, MaxDuration: time.Minute,
		},
	})
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	defer sim.Close()
	for _, venue := range sim.Venues {
		if got := len(venue.MetaorderTraders); got != 4 {
			t.Fatalf("venue %s execution agents = %d, want 4", venue.ID, got)
		}
		seeds := map[int64]struct{}{}
		for _, trader := range venue.MetaorderTraders {
			seeds[trader.cfg.Seed] = struct{}{}
		}
		if len(seeds) != 4 {
			t.Fatalf("venue %s execution agents share seeds: %d distinct of 4", venue.ID, len(seeds))
		}
	}
}

// The latent population must behave the way the reaction-diffusion picture
// describes: intentions accumulate, their reservation prices move, and one
// becomes an order only when it crosses the market.
func TestLatentLiquidityAccumulatesDiffusesAndConvertsOnCrossing(t *testing.T) {
	gw := newStoikovStubGateway()
	latent := NewLatentLiquidity(1, gw, LatentLiquidityConfig{
		Symbol: "ABC/USD", BasePrecision: mvBasePrecision, TickSize: 10 * mvQuotePrecision,
		Interval: time.Second, DepositsPerTick: 10, CancelProbability: 0,
		DiffusionBps: 0, SpreadBps: 100, IntentionQty: mvBasePrecision / 10, MaxIntentions: 100,
		Seed: 7,
	})
	now := time.Unix(0, 0)
	latent.onTick(now) // subscribes

	feed := func(bid, ask int64) {
		latent.HandleEvent(context.Background(), &actor.Event{
			Type: actor.EventBookSnapshot,
			Data: actor.BookSnapshotEvent{Symbol: "ABC/USD", Timestamp: now.UnixNano(),
				Snapshot: &exchange.BookSnapshot{
					Bids: []etypes.PriceLevel{{Price: bid, VisibleQty: mvBasePrecision}},
					Asks: []etypes.PriceLevel{{Price: ask, VisibleQty: mvBasePrecision}},
				}},
		})
	}

	// With no diffusion, intentions deposited inside a band around the price
	// never cross it, so they accumulate without trading.
	feed(50_000*mvQuotePrecision, 50_010*mvQuotePrecision)
	before := len(gw.requests)
	for range 3 {
		latent.onTick(now)
	}
	if latent.Intentions() == 0 {
		t.Fatal("no intentions accumulated")
	}
	if len(gw.requests) != before {
		t.Fatalf("intentions traded without crossing the market: %+v", gw.requests[before:])
	}

	// Move the market far below the resting buy intentions: they are now
	// crossed and must convert, one order per tick.
	feed(1*mvQuotePrecision, 2*mvQuotePrecision)
	latent.onTick(now)
	if latent.Converted() != 1 {
		t.Fatalf("converted %d intentions, want exactly one per tick", latent.Converted())
	}
	order := gw.requests[len(gw.requests)-1].OrderReq
	if order.Side != exchange.Buy || order.TimeInForce != exchange.IOC {
		t.Fatalf("unexpected converted order: %+v", order)
	}
	if order.Price%(10*mvQuotePrecision) != 0 {
		t.Fatalf("converted order price %d is off the tick grid", order.Price)
	}
}

// A revealed intention must rest as liquidity rather than cross, and must be
// pulled when its reservation price drifts away from the market. That
// distinction is the whole point of the reveal mode: an intention that crosses
// is demand, one that rests is the supply a large order trades into.
func TestLatentIntentionsRevealAsRestingLiquidity(t *testing.T) {
	gw := newStoikovStubGateway()
	latent := NewLatentLiquidity(1, gw, LatentLiquidityConfig{
		Symbol: "ABC/USD", BasePrecision: mvBasePrecision, TickSize: 10 * mvQuotePrecision,
		Interval: time.Second, DepositsPerTick: 6, CancelProbability: 0,
		DiffusionBps: 0, SpreadBps: 50, IntentionQty: mvBasePrecision / 10,
		MaxIntentions: 50, RevealBps: 40, RevealsPerTick: 10, Seed: 3,
	})
	now := time.Unix(0, 0)
	latent.onTick(now)

	feed := func(bid, ask int64) {
		latent.HandleEvent(context.Background(), &actor.Event{
			Type: actor.EventBookSnapshot,
			Data: actor.BookSnapshotEvent{Symbol: "ABC/USD", Timestamp: now.UnixNano(),
				Snapshot: &exchange.BookSnapshot{
					Bids: []etypes.PriceLevel{{Price: bid, VisibleQty: mvBasePrecision}},
					Asks: []etypes.PriceLevel{{Price: ask, VisibleQty: mvBasePrecision}},
				}},
		})
	}

	feed(50_000*mvQuotePrecision, 50_010*mvQuotePrecision)
	latent.onTick(now)
	latent.onTick(now)

	posted := 0
	for _, request := range gw.requests {
		order := request.OrderReq
		if order == nil {
			continue
		}
		posted++
		if order.TimeInForce != exchange.GTC {
			t.Fatalf("revealed intention did not rest: %+v", order)
		}
		// It must be on the passive side of the market, never marketable.
		if order.Side == exchange.Buy && order.Price >= 50_010*mvQuotePrecision {
			t.Fatalf("revealed buy crosses the ask: %+v", order)
		}
		if order.Side == exchange.Sell && order.Price <= 50_000*mvQuotePrecision {
			t.Fatalf("revealed sell crosses the bid: %+v", order)
		}
	}
	if posted == 0 {
		t.Fatal("no intention was revealed near the price")
	}
}

// A child order priced through the touch must be able to take more than the
// size displayed at the best price. Capping it at the touch made realised
// participation a property of the makers' quote size rather than of the
// configured rate, which stopped participation being a treatment at all.
func TestMetaorderChildWalksBeyondTheTouchWhenSlippageIsAllowed(t *testing.T) {
	build := func(slippageBps int64) *MetaorderTrader {
		trader := &MetaorderTrader{cfg: MetaorderTraderConfig{
			Symbol: "ABC/USD", BasePrecision: mvBasePrecision, TickSize: 10 * mvQuotePrecision,
			MinChildQty: 5 * mvBasePrecision, MaxSlippageBps: slippageBps,
		}}
		trader.BaseActor = actor.NewBaseActor(1, newStoikovStubGateway())
		trader.SetHandler(trader)
		trader.active, trader.side = true, exchange.Buy
		trader.parentQty = 100 * mvBasePrecision
		// Only a fifth of a unit is displayed at the best price.
		trader.bestAsk, trader.askQty = 50_000*mvQuotePrecision, mvBasePrecision/5
		trader.bestBid, trader.bidQty = 49_990*mvQuotePrecision, mvBasePrecision/5
		return trader
	}

	capped := build(0)
	capped.executeChild(0)
	cappedGw := capped.Gateway().(*stoikovStubGateway)
	if len(cappedGw.requests) == 0 {
		t.Fatal("no child submitted without slippage")
	}
	if got := cappedGw.requests[0].OrderReq.Qty; got != mvBasePrecision/5 {
		t.Fatalf("without slippage the child took %d, want the displayed %d", got, mvBasePrecision/5)
	}

	walking := build(30)
	walking.executeChild(0)
	walkingGw := walking.Gateway().(*stoikovStubGateway)
	if len(walkingGw.requests) == 0 {
		t.Fatal("no child submitted with slippage")
	}
	order := walkingGw.requests[0].OrderReq
	if order.Qty != 5*mvBasePrecision {
		t.Fatalf("with slippage the child took %d, want the full %d", order.Qty, 5*mvBasePrecision)
	}
	if order.Price <= 50_000*mvQuotePrecision {
		t.Fatalf("child price %d is not through the touch", order.Price)
	}
	if order.Price%(10*mvQuotePrecision) != 0 {
		t.Fatalf("child price %d is off the tick grid", order.Price)
	}
}

// Every quoted market needs a reference of its own. A market that receives no
// index falls back to its own book midpoint, which reproduces itself and
// diverges: with the cross-asset books unpublished, a twelve-hour run produced
// participant results in the tens of billions on a starting capital of about
// thirty billion.
func TestEveryQuotedMarketReceivesAnIndexOnceVenuesReportMidpoints(t *testing.T) {
	sim, err := NewSim(time.Second, Config{
		LogDir: t.TempDir(), LogMode: "none", Seed: 91,
		MakerAnchor: "consensus", CrossAssetSpotGraph: true,
	})
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	defer sim.Close()

	// The consensus index is endogenous: it exists only once venues have
	// reported midpoints from real books. Before that there is nothing to
	// publish, which is the correct behaviour rather than a gap.
	for _, symbol := range []string{"ABC/USD", "ABC-PERP", "CDF/USD", "ABC/CDF"} {
		if price, err := sim.SpotIndex.Price(symbol); err == nil || price != 0 {
			t.Fatalf("%s index before observations = (%d, %v), want unavailable", symbol, price, err)
		}
	}

	mids := map[string]int64{
		"ABC/USD":  50 * mvQuotePrecision,
		"ABC-PERP": 50 * mvQuotePrecision,
		"CDF/USD":  3 * mvQuotePrecision,
		"ABC/CDF":  16 * mvQuotePrecision,
	}
	for _, venue := range sim.Venues {
		for symbol, mid := range mids {
			sim.SpotIndex.observeVenueMid(symbol, venue.ID, mid)
		}
	}

	for symbol, mid := range mids {
		price, err := sim.SpotIndex.Price(symbol)
		if err != nil || price <= 0 {
			t.Fatalf("no index published for %s after every venue reported a midpoint: (%d, %v)", symbol, price, err)
		}
		if price != mid {
			t.Fatalf("index for %s is %d, want the consensus of the reported midpoints %d", symbol, price, mid)
		}
	}
}
func TestFundingIntervalIsConfigurableAndReachesThePerpetual(t *testing.T) {
	for _, want := range []int64{28800, 14400} {
		sim, err := NewSim(time.Second, Config{
			LogDir: t.TempDir(), LogMode: "none", Seed: 91,
			FundingIntervalSeconds: want,
		})
		if err != nil {
			t.Fatalf("NewSim(%d): %v", want, err)
		}
		for _, venue := range sim.Venues {
			book := venue.Exchange.Books["ABC-PERP"]
			if book == nil {
				sim.Close()
				t.Fatal("no perpetual book")
			}
			perp, ok := book.Instrument.(interface {
				GetFundingRate() *etypes.FundingRate
			})
			if !ok {
				sim.Close()
				t.Fatal("perpetual does not expose its funding rate")
			}
			if got := perp.GetFundingRate().Interval; got != want {
				sim.Close()
				t.Fatalf("venue %s funding interval = %d, want %d", venue.ID, got, want)
			}
		}
		sim.Close()
	}
}

// Perpetual funding settles on a different clock at every real venue, so a desk
// holding the same exposure on two of them faces two payment schedules that
// only sometimes coincide. A single shared period removes that entirely.
func TestVenueRuleOverridesTheFundingPeriodPerVenue(t *testing.T) {
	cfg := Config{
		LogDir:                 t.TempDir(),
		VenueIDs:               []string{"north", "central", "south"},
		FundingIntervalSeconds: 28800,
		VenueRules: map[string]VenueRule{
			"central": {FundingIntervalSeconds: 3600},
		},
	}
	if got := cfg.fundingInterval("north"); got != 28800 {
		t.Errorf("north funding interval = %d, want the population default 28800", got)
	}
	if got := cfg.fundingInterval("central"); got != 3600 {
		t.Errorf("central funding interval = %d, want the override 3600", got)
	}
	if err := cfg.normalize(); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	cfg.VenueRules["central"] = VenueRule{FundingIntervalSeconds: -1}
	if err := cfg.normalize(); err == nil {
		t.Error("a negative funding interval was accepted")
	}
}

// A population whose dealers all price from one number quotes one price. The
// roster has to hand consecutive participants different opinions and cycle
// when there are more participants than opinions.
func TestOptionDealerVolRosterCyclesDistinctOpinions(t *testing.T) {
	cfg := OptionDealerVolConfig{
		Model:           "realized",
		HalfLifeSeconds: []float64{60, 3600},
		Premiums:        []float64{1.0, 1.4},
	}
	first, second, third := cfg.modelFor(0, 0.8), cfg.modelFor(1, 0.8), cfg.modelFor(2, 0.8)
	firstEstimator, ok := first.(*eprice.RealizedVolatility)
	if !ok {
		t.Fatalf("model = %T, want a realized estimator", first)
	}
	secondEstimator := second.(*eprice.RealizedVolatility)
	thirdEstimator := third.(*eprice.RealizedVolatility)
	if firstEstimator.HalfLifeSeconds == secondEstimator.HalfLifeSeconds {
		t.Error("consecutive dealers were given the same half-life")
	}
	if thirdEstimator.HalfLifeSeconds != firstEstimator.HalfLifeSeconds {
		t.Error("the roster did not cycle back to its first opinion")
	}
	if firstEstimator.Premium != 1.0 || secondEstimator.Premium != 1.4 {
		t.Errorf("premiums = %v and %v, want 1.0 and 1.4", firstEstimator.Premium, secondEstimator.Premium)
	}
}

// An empty roster must leave the population exactly as it was.
func TestOptionDealerVolDefaultsToTheConfiguredLevel(t *testing.T) {
	if model := (OptionDealerVolConfig{}).modelFor(0, 0.8); model != nil {
		t.Errorf("model = %v, want nil so the dealer prices at its configured IV", model)
	}
}

func TestConfigRefusesAnUnbuildableVolatilityRoster(t *testing.T) {
	base := func() Config {
		return Config{LogDir: "x", VenueIDs: []string{"north", "central", "south"}}
	}
	cfg := base()
	cfg.OptionDealerVol = OptionDealerVolConfig{Model: "heston"}
	if err := cfg.normalize(); err == nil {
		t.Error("an unknown volatility model was accepted")
	}
	cfg = base()
	cfg.OptionValueTakerVol = OptionDealerVolConfig{Model: "realized", HalfLifeSeconds: []float64{-1}}
	if err := cfg.normalize(); err == nil {
		t.Error("a negative half-life was accepted")
	}
	cfg = base()
	cfg.OptionDealerHedgePolicies = []string{"vanna"}
	if err := cfg.normalize(); err == nil {
		t.Error("an unknown hedge policy was accepted")
	}
	cfg = base()
	cfg.OptionValueTakerCount = 2
	if err := cfg.normalize(); err == nil {
		t.Error("value takers with no edge requirement were accepted, so they cross every spread")
	}
}

// The hedge roster selects per dealer and cycles, and an empty roster leaves
// every dealer on the policy the population already ran.
func TestHedgePolicyRosterSelectsPerDealer(t *testing.T) {
	cfg := Config{OptionDealerHedgePolicies: []string{"static", "timed"}, OptionDealerHedgeIntervalSeconds: 30}
	if _, ok := cfg.hedgePolicyFor(0).(derivsim.StaticDeltaHedge); !ok {
		t.Errorf("dealer 0 = %T, want a static hedge", cfg.hedgePolicyFor(0))
	}
	timed, ok := cfg.hedgePolicyFor(1).(derivsim.TimedDeltaHedge)
	if !ok {
		t.Fatalf("dealer 1 = %T, want a timed hedge", cfg.hedgePolicyFor(1))
	}
	if timed.IntervalNanos != int64(30*time.Second) {
		t.Errorf("timed interval = %d, want 30 seconds", timed.IntervalNanos)
	}
	if _, ok := cfg.hedgePolicyFor(2).(derivsim.StaticDeltaHedge); !ok {
		t.Error("the hedge roster did not cycle")
	}
	if (Config{}).hedgePolicyFor(0) != nil {
		t.Error("an empty roster changed the dealer's hedge")
	}
}

// A book quoted by one maker class has a single point of failure whatever its
// volume, so the maker rosters have to be placeable on the other books.
func TestMakerSymbolRosterCyclesAndDefaultsToTheMainBook(t *testing.T) {
	if got := makerSymbol(nil, 3); got != "ABC/USD" {
		t.Errorf("symbol = %q, want the default ABC/USD", got)
	}
	roster := []string{"ABC/USD", "CDF/USD"}
	if got := makerSymbol(roster, 1); got != "CDF/USD" {
		t.Errorf("symbol = %q, want CDF/USD", got)
	}
	if got := makerSymbol(roster, 2); got != "ABC/USD" {
		t.Errorf("roster did not cycle: %q", got)
	}
}

// A maker quoting a book at the wrong tick has every order rejected, so each
// placeable book must resolve to its own tick.
func TestTickForResolvesEachSpotBook(t *testing.T) {
	spotTick := int64(10_000)
	if got := tickFor("ABC/USD", spotTick); got != spotTick {
		t.Errorf("ABC/USD tick = %d, want the configured %d", got, spotTick)
	}
	if got := tickFor("CDF/USD", spotTick); got != int64(mvQuotePrecision) {
		t.Errorf("CDF/USD tick = %d, want %d", got, mvQuotePrecision)
	}
	if got := tickFor("ABC/CDF", spotTick); got != int64(mvBasePrecision/1_000) {
		t.Errorf("ABC/CDF tick = %d, want %d", got, mvBasePrecision/1_000)
	}
}

// Placing a maker on a book the population did not list is a configuration
// error, not a silently idle participant.
func TestConfigRefusesMakersOnBooksThatDoNotExist(t *testing.T) {
	cfg := Config{LogDir: "x", VenueIDs: []string{"north", "central", "south"}}
	cfg.FixedDistanceMakerSymbols = []string{"CDF/USD"}
	if err := cfg.normalize(); err == nil {
		t.Error("a maker was placed on the cross-asset book with the graph switched off")
	}
	cfg.CrossAssetSpotGraph = true
	if err := cfg.normalize(); err != nil {
		t.Errorf("a listed book was refused: %v", err)
	}
	cfg.ImbalanceMakerSymbols = []string{"ABC-PERP"}
	if err := cfg.normalize(); err != nil {
		t.Errorf("the perpetual was refused as a maker book: %v", err)
	}
	cfg.ImbalanceMakerSymbols = []string{"XYZ/USD"}
	if err := cfg.normalize(); err == nil {
		t.Error("a maker was placed on a book that does not exist")
	}
}

// A role is connected under a numbered name, so the profile lookup has to
// resolve the class and leave names that merely end in a word alone.
func TestRoleClassStripsTheParticipantNumber(t *testing.T) {
	for input, want := range map[string]string{
		"spot_maker_1":          "spot_maker",
		"spot_maker":            "spot_maker",
		"option_value_taker_12": "option_value_taker",
		"cross_venue_router":    "cross_venue_router",
		"abc_cdf_maker":         "abc_cdf_maker",
	} {
		if got := roleClass(input); got != want {
			t.Errorf("roleClass(%q) = %q, want %q", input, got, want)
		}
	}
}

// A population where everyone reaches the engine in the same time gives nobody
// a reason to pay for speed, so profiles must resolve per class with a default
// for the rest.
func TestLatencyProfileResolutionPrefersTheNamedClass(t *testing.T) {
	fallback := LatencyProfile{Model: "constant", Delay: 5 * time.Millisecond}
	cfg := Config{
		LatencyProfiles:       map[string]LatencyProfile{"spot_maker": {Model: "constant", Delay: time.Millisecond}},
		DefaultLatencyProfile: &fallback,
	}
	maker, ok := cfg.latencyProfileFor("spot_maker_3")
	if !ok || maker.Delay != time.Millisecond {
		t.Errorf("spot maker profile = %+v, want the named 1ms one", maker)
	}
	other, ok := cfg.latencyProfileFor("noise_flow_1")
	if !ok || other.Delay != 5*time.Millisecond {
		t.Errorf("unnamed class = %+v, want the 5ms default", other)
	}
	if _, ok := (Config{}).latencyProfileFor("spot_maker_1"); ok {
		t.Error("a population with no profiles reported one")
	}
}

// Each model has to build, and an unbuildable one has to be refused rather
// than silently connecting the class directly at full speed.
func TestLatencyProfileBuildsEveryModelAndRefusesTheRest(t *testing.T) {
	profiles := []LatencyProfile{
		{Model: "constant", Delay: time.Millisecond},
		{Model: "uniform", Min: time.Millisecond, Max: 2 * time.Millisecond},
		{Model: "normal", Delay: time.Millisecond, StdDev: time.Millisecond / 4},
		{Model: "lognormal", Delay: time.Millisecond, Sigma: 0.5},
		{Model: "spiky", Delay: time.Millisecond, SpikeDelay: 50 * time.Millisecond, SpikeProbability: 0.01},
	}
	for _, profile := range profiles {
		if err := profile.validate("test"); err != nil {
			t.Errorf("%s refused: %v", profile.Model, err)
		}
		if provider := profile.provider(1, 1); provider == nil || provider.Delay() < 0 {
			t.Errorf("%s built no usable provider", profile.Model)
		}
	}
	for _, bad := range []LatencyProfile{
		{Model: "poisson"},
		{Model: "uniform", Min: 2 * time.Millisecond, Max: time.Millisecond},
		{Model: "constant", Delay: -time.Millisecond},
		{Model: "spiky", Delay: time.Millisecond, SpikeProbability: 2},
	} {
		if err := bad.validate("test"); err == nil {
			t.Errorf("%+v was accepted", bad)
		}
	}
}

// A profile asking for no delay must leave the participant on the direct
// mount, so that switching the mechanism on changes nothing until it is used.
func TestZeroLatencyProfileConnectsDirectly(t *testing.T) {
	if !(LatencyProfile{Model: "constant"}).zero() {
		t.Error("a profile with no delay did not report itself as direct")
	}
	if (LatencyProfile{Model: "constant", Delay: time.Millisecond}).zero() {
		t.Error("a profile with a delay reported itself as direct")
	}
}

// A dealer on the harder model has to be buildable from configuration and its
// parameters checked, since an impossible correlation prices nothing and would
// otherwise leave the dealer silently quoting its fallback.
func TestOptionDealerVolBuildsTheSABRModel(t *testing.T) {
	cfg := OptionDealerVolConfig{
		Model: "sabr", SABRAlphas: []float64{0.4, 0.7}, SABRBeta: 1, SABRRho: -0.3, SABRNu: 0.6,
	}
	first, ok := cfg.modelFor(0, 0.8).(eprice.SABRVolatility)
	if !ok {
		t.Fatalf("model = %T, want a SABR model", cfg.modelFor(0, 0.8))
	}
	second := cfg.modelFor(1, 0.8).(eprice.SABRVolatility)
	if first.Alpha != 0.4 || second.Alpha != 0.7 {
		t.Errorf("alphas = %v and %v, want 0.4 and 0.7", first.Alpha, second.Alpha)
	}
	if cfg.modelFor(2, 0.8).(eprice.SABRVolatility).Alpha != 0.4 {
		t.Error("the alpha roster did not cycle")
	}
	if err := cfg.validate("test"); err != nil {
		t.Errorf("a buildable model was refused: %v", err)
	}
	for _, bad := range []OptionDealerVolConfig{
		{Model: "sabr", SABRRho: -1},
		{Model: "sabr", SABRBeta: 2},
		{Model: "sabr", SABRNu: -0.1},
		{Model: "sabr", SABRAlphas: []float64{0}},
	} {
		if err := bad.validate("test"); err == nil {
			t.Errorf("%+v was accepted", bad)
		}
	}
}

// One order size across every book sizes the flow for the main pair and leaves
// the rest to whichever arbitrageur trades them in size, so the size has to be
// settable per book — and a book that does not exist has to be refused rather
// than silently ignored.
func TestNoiseTargetQuantityIsPerBookAndValidated(t *testing.T) {
	cfg := Config{LogDir: "x", VenueIDs: []string{"north", "central", "south"}, CrossAssetSpotGraph: true}
	cfg.NoiseTargetQtyBySymbol = map[string]int64{"CDF/USD": 5 * mvBasePrecision}
	if err := cfg.normalize(); err != nil {
		t.Fatalf("a listed book was refused: %v", err)
	}
	cfg.NoiseTargetQtyBySymbol = map[string]int64{"XYZ/USD": 1}
	if err := cfg.normalize(); err == nil {
		t.Error("a size was accepted for a book that does not exist")
	}
	cfg.NoiseTargetQtyBySymbol = map[string]int64{"ABC/USD": 0}
	if err := cfg.normalize(); err == nil {
		t.Error("a zero size was accepted")
	}
	cfg = Config{LogDir: "x", VenueIDs: []string{"north", "central", "south"}}
	cfg.NoiseTargetQtyBySymbol = map[string]int64{"CDF/USD": 1}
	if err := cfg.normalize(); err == nil {
		t.Error("a cross-asset book was accepted with the graph switched off")
	}
}

// The price-elastic participants were pinned to ABC/USD, which is why the
// second spot book had nobody who cared about its level. Placing them
// elsewhere is what the ablation needs, and a supplier on CDF/USD has to be
// seeded at CDF's opening price rather than ABC's.
func TestElasticSupplierSymbolRosterIsValidatedLikeTheMakers(t *testing.T) {
	cfg := Config{LogDir: "x", VenueIDs: []string{"north", "central", "south"}, CrossAssetSpotGraph: true}
	cfg.ElasticSupplierSymbols = []string{"ABC/USD", "CDF/USD"}
	if err := cfg.normalize(); err != nil {
		t.Fatalf("a listed book was refused: %v", err)
	}
	if got := makerSymbol(cfg.ElasticSupplierSymbols, 1); got != "CDF/USD" {
		t.Errorf("second supplier on %q, want CDF/USD", got)
	}
	cfg.ElasticSupplierSymbols = []string{"XYZ/USD"}
	if err := cfg.normalize(); err == nil {
		t.Error("a supplier was placed on a book that does not exist")
	}
	cfg = Config{LogDir: "x", VenueIDs: []string{"north", "central", "south"}}
	cfg.ElasticSupplierSymbols = []string{"CDF/USD"}
	if err := cfg.normalize(); err == nil {
		t.Error("a cross-asset book was accepted with the graph switched off")
	}
}
