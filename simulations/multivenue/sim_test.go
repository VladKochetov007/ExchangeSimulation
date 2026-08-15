package multivenue

import (
	"context"
	"crypto/sha256"
	"encoding/json"
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
	"exchange_sim/exchange"
	etypes "exchange_sim/types"
	"exchange_sim/matching"
)

func TestConfigAcceptsDocumentedSnakeCaseJSON(t *testing.T) {
	var cfg Config
	if err := json.Unmarshal([]byte(`{"log_dir":"ignored","log_mode":"none","seed":99,"dealer_hedge_mode":"off","short_option_tenor":7200000000000}`), &cfg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if cfg.LogMode != "none" || cfg.Seed != 99 || cfg.DealerHedgeMode != "off" || cfg.ShortOptionTenor != 2*time.Hour {
		t.Fatalf("snake-case config was not decoded: %+v", cfg)
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

func TestPopulationValuationRejectsMissingTerminalTwoSidedMark(t *testing.T) {
	venue := &Venue{ID: "unpriced", Exchange: exchange.NewExchange(1, nil)}
	if _, _, err := populationValuationSpec(venue, "terminal_post_mark", false, 0, 0); err == nil {
		t.Fatal("terminal population valuation accepted a venue without a two-sided ABC/USD mark")
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
	if len(terminalOne) != 45 {
		t.Fatalf("cross-asset terminal account rows = %d, want 45", len(terminalOne))
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
		mvQuotePrecision, mvBasePrecision/100, mvBootstrapPrice, now+2*int64(time.Second), true,
	)
	option.IV = sim.Config.OptionIV
	option.SetMarks(mvBootstrapPrice, 10*mvQuotePrecision)
	venue.Exchange.AddInstrument(option)
	venue.Exchange.Positions.UpdatePosition(venue.OptionDealerClientID, option.Symbol(), mvBasePrecision/10, 10*mvQuotePrecision, exchange.Buy, exchange.PositionBoth)
	venue.Exchange.AddPerpBalance(venue.OptionDealerClientID, "USD", -mvQuotePrecision)

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

	// Inventory must be able to move the cross quote within a small number of
	// lots. One lot alone need not clear a tick — the cross tick is 0.6bps of
	// the cross price, finer than the USD book — but a maker that needs a large
	// position before its quote can move at all is effectively inventory blind.
	const maxLotsToMoveATick = 5
	lot := float64(cross.QuoteQty) / float64(cross.BasePrecision)
	crossPrice := float64(cross.BootstrapPrice) / float64(cross.QuotePrecision)
	tick := float64(cross.TickSize) / float64(cross.QuotePrecision)
	if got := crossShift * crossPrice * lot * maxLotsToMoveATick; got < tick {
		t.Fatalf("inventory skew over %d lots is %.18g CDF, below one tick %.18g: maker cannot reprice",
			maxLotsToMoveATick, got, tick)
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

// The fundamental value is a function of simulated time alone, so every
// participant that consults it at one timestamp sees the same value no matter
// which of them asks first.
func TestFundamentalValueIsPathDeterministic(t *testing.T) {
	const step = int64(time.Second)
	build := func() *FundamentalValue {
		return NewFundamentalValue(7, 0, 50_000*mvQuotePrecision, step, mvQuotePrecision, 0.001)
	}

	forward := build()
	var inOrder []int64
	for i := range 200 {
		inOrder = append(inOrder, forward.Value(int64(i)*step))
	}

	// Jumping straight to the end must extend the same path, not a new one.
	skipped := build()
	if got, want := skipped.Value(199*step), inOrder[199]; got != want {
		t.Fatalf("value after a jump = %d, want %d", got, want)
	}
	// Re-reading an earlier timestamp must not redraw it.
	if got, want := skipped.Value(50*step), inOrder[50]; got != want {
		t.Fatalf("value after rewind = %d, want %d", got, want)
	}
	if inOrder[0] != 50_000*mvQuotePrecision {
		t.Fatalf("path must start at the bootstrap price, got %d", inOrder[0])
	}
	moved := false
	for _, v := range inOrder {
		if v != inOrder[0] {
			moved = true
			break
		}
	}
	if !moved {
		t.Fatal("fundamental value never moved")
	}
}

// A value trader must only cross when the visible price is beyond its edge from
// fundamental value, and must respect its inventory bound.
func TestValueTraderTradesAgainstFundamentalWithinInventoryBound(t *testing.T) {
	value := NewFundamentalValue(1, 0, 50_000*mvQuotePrecision, int64(time.Second), mvQuotePrecision, 0)
	gw := newStoikovStubGateway()
	vt := NewValueTrader(1, gw, value, ValueTraderConfig{
		Symbol: "ABC/USD", BasePrecision: mvBasePrecision, TickSize: 10 * mvQuotePrecision,
		EdgeBps: 10, LotQty: mvBasePrecision / 10, MaxInventory: mvBasePrecision / 10,
		TradeInterval: time.Second,
	})
	now := time.Unix(0, 0)
	vt.onTick(now) // subscribes

	feed := func(bid, ask int64) {
		vt.HandleEvent(context.Background(), &actor.Event{
			Type: actor.EventBookSnapshot,
			Data: actor.BookSnapshotEvent{
				Symbol: "ABC/USD", Timestamp: now.UnixNano(),
				Snapshot: &exchange.BookSnapshot{
					Bids: []etypes.PriceLevel{{Price: bid, VisibleQty: mvBasePrecision}},
					Asks: []etypes.PriceLevel{{Price: ask, VisibleQty: mvBasePrecision}},
				},
			},
		})
	}

	// Inside the edge: no trade.
	feed(49_995*mvQuotePrecision, 50_005*mvQuotePrecision)
	before := len(gw.requests)
	vt.onTick(now)
	if len(gw.requests) != before {
		t.Fatalf("traded inside the edge band: %+v", gw.requests[before:])
	}

	// Offer well below fundamental value: buy it.
	feed(49_000*mvQuotePrecision, 49_100*mvQuotePrecision)
	vt.onTick(now)
	if len(gw.requests) != before+1 {
		t.Fatalf("did not lift a cheap offer, requests=%d", len(gw.requests))
	}
	order := gw.requests[len(gw.requests)-1].OrderReq
	if order.Side != exchange.Buy || order.TimeInForce != exchange.IOC || order.Price != 49_100*mvQuotePrecision {
		t.Fatalf("unexpected order %+v", order)
	}

	// Once the fill takes it to its inventory bound it must stop buying.
	vt.HandleEvent(context.Background(), &actor.Event{
		Type: actor.EventOrderFilled,
		Data: actor.OrderFillEvent{Symbol: "ABC/USD", Side: exchange.Buy, Qty: mvBasePrecision / 10, IsFull: true},
	})
	before = len(gw.requests)
	vt.onTick(now)
	if len(gw.requests) != before {
		t.Fatalf("kept buying past the inventory bound: %+v", gw.requests[before:])
	}
}
