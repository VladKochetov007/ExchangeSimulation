package multivenue

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestConfigAcceptsDocumentedSnakeCaseJSON(t *testing.T) {
	var cfg Config
	if err := json.Unmarshal([]byte(`{"log_dir":"ignored","seed":99,"dealer_hedge_mode":"off","short_option_tenor":7200000000000}`), &cfg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if cfg.Seed != 99 || cfg.DealerHedgeMode != "off" || cfg.ShortOptionTenor != 2*time.Hour {
		t.Fatalf("snake-case config was not decoded: %+v", cfg)
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
