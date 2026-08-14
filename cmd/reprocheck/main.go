// Command reprocheck runs fixed-config derivsim replicas and records whether
// their canonical analyzer output agrees. It intentionally preserves every
// run directory so a divergence can be inspected rather than overwritten.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

type replica struct {
	Index       int    `json:"index"`
	LogDir      string `json:"log_dir"`
	MetricsPath string `json:"metrics_path"`
	MetricsHash string `json:"metrics_hash"`
}

type manifest struct {
	Config       string    `json:"config"`
	Duration     string    `json:"duration"`
	GOMAXPROCS   int       `json:"gomaxprocs"`
	Replicas     []replica `json:"replicas"`
	Reproducible bool      `json:"reproducible"`
}

func main() {
	config := flag.String("config", "research/derivsim-active.json", "derivsim config path")
	duration := flag.Duration("duration", 15*time.Second, "simulated duration per replica")
	runs := flag.Int("runs", 3, "number of sequential replicas")
	out := flag.String("out", "", "new directory for preserved logs and manifest (required)")
	workdir := flag.String("workdir", ".", "repository working directory")
	gomaxprocs := flag.Int("gomaxprocs", 0, "GOMAXPROCS for child commands (required when nonzero)")
	flag.Parse()

	if *runs < 2 {
		fatalf("-runs must be at least 2")
	}
	if *out == "" {
		fatalf("-out is required so existing evidence is never overwritten")
	}
	if err := os.Mkdir(*out, 0o755); err != nil {
		fatalf("create output directory: %v", err)
	}

	m := manifest{Config: *config, Duration: duration.String(), GOMAXPROCS: *gomaxprocs, Replicas: make([]replica, 0, *runs)}
	for i := 0; i < *runs; i++ {
		logDir := filepath.Join(*out, fmt.Sprintf("run-%03d", i+1))
		if err := os.Mkdir(logDir, 0o755); err != nil {
			fatalf("create run directory: %v", err)
		}
		run(*workdir, *gomaxprocs, "go", "run", "./cmd/derivsim", "-config="+*config, "-duration="+duration.String(), "-logdir="+logDir)
		metricsPath := filepath.Join(logDir, "metrics.json")
		run(*workdir, *gomaxprocs, "go", "run", "./cmd/loganalyzer", "-dir="+logDir, "-out="+metricsPath)
		data, err := os.ReadFile(metricsPath)
		if err != nil {
			fatalf("read metrics for run %d: %v", i+1, err)
		}
		digest := sha256.Sum256(data)
		m.Replicas = append(m.Replicas, replica{
			Index:       i + 1,
			LogDir:      logDir,
			MetricsPath: metricsPath,
			MetricsHash: hex.EncodeToString(digest[:]),
		})
	}
	m.Reproducible = true
	for _, r := range m.Replicas[1:] {
		if r.MetricsHash != m.Replicas[0].MetricsHash {
			m.Reproducible = false
			break
		}
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(*out, "manifest.json"), append(data, '\n'), 0o644); err != nil {
		fatalf("write manifest: %v", err)
	}
	for _, r := range m.Replicas {
		fmt.Printf("run=%d metrics_hash=%s logs=%s\n", r.Index, r.MetricsHash, r.LogDir)
	}
	fmt.Printf("reproducible=%t manifest=%s\n", m.Reproducible, filepath.Join(*out, "manifest.json"))
}

func run(workdir string, gomaxprocs int, name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Dir = workdir
	if gomaxprocs > 0 {
		cmd.Env = append(os.Environ(), "GOMAXPROCS="+strconv.Itoa(gomaxprocs))
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		fatalf("%s %v failed: %v\n%s", name, args, err, output)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "reprocheck: "+format+"\n", args...)
	os.Exit(1)
}
