package derivsim

import (
	"context"
	"crypto/sha256"
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

func digestDerivSimLogs(t *testing.T, dir string) string {
	t.Helper()
	var files []string
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("WalkDir(%q): %v", dir, err)
	}
	sort.Strings(files)

	hash := sha256.New()
	for _, path := range files {
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			t.Fatalf("Rel(%q): %v", path, err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", path, err)
		}
		fmt.Fprintf(hash, "%s\x00", rel)
		hash.Write(data)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func TestDerivSimLogsDynamicallyListedDerivatives(t *testing.T) {
	logDir := t.TempDir()
	sim, err := NewSim(2*time.Second, SimConfig{LogDir: logDir})
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	sim.Exchange().StartAutomation(ctx)
	sim.Runner.SetShutdownHook(func() {
		cancel()
		sim.Exchange().StopAutomation()
	})
	if err := sim.Runner.Run(ctx); err != nil {
		t.Fatalf("Runner.Run: %v", err)
	}
	sim.Close()

	data, err := os.ReadFile(filepath.Join(logDir, "derivatives.jsonl"))
	if err != nil {
		t.Fatalf("read derivatives log: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `"event":"BookSnapshot"`) {
		t.Fatalf("derivatives log has no book snapshots:\n%s", text)
	}
	optionSymbol := regexp.MustCompile(`"symbol":"ABC-[0-9]+-[0-9]+-(?:C|P)"`)
	if !strings.Contains(text, "ABC-FUT-") || !optionSymbol.MatchString(text) {
		t.Fatalf("derivatives log does not include futures and options:\n%s", text)
	}
}

// Derivsim combines listings, expiry automation, and multiple actor classes.
// It must use the same ordered runtime as randomwalk before its basis, option,
// or hedging measurements can be interpreted as simulation results.
func TestDerivSimDeterministicPhaseDigestAcrossGOMAXPROCS(t *testing.T) {
	run := func(procs int) string {
		t.Helper()
		previous := runtime.GOMAXPROCS(procs)
		defer runtime.GOMAXPROCS(previous)

		logDir := t.TempDir()
		sim, err := NewSim(5*time.Second, SimConfig{LogDir: logDir})
		if err != nil {
			t.Fatalf("NewSim: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		sim.Exchange().StartAutomation(ctx)
		sim.Runner.SetShutdownHook(func() {
			cancel()
			sim.Exchange().StopAutomation()
		})
		if err := sim.Runner.Run(ctx); err != nil {
			t.Fatalf("Runner.Run with GOMAXPROCS=%d: %v", procs, err)
		}
		sim.Close()
		return digestDerivSimLogs(t, logDir)
	}

	one := run(1)
	many := run(14)
	if one != many {
		t.Fatalf("derivsim log digest differs: GOMAXPROCS=1 %s, GOMAXPROCS=14 %s", one, many)
	}
}
