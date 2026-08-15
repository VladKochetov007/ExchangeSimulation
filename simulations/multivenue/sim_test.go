package multivenue

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"exchange_sim/exchange"
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

		sim, err := NewSim(3*time.Second, Config{LogDir: t.TempDir(), Seed: 27})
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
