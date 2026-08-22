// prunegate decides whether a run's raw event logs may be deleted.
//
// The rule it enforces is the one the campaign learned the hard way: a run's
// logs were deleted after a generic set of metrics had been extracted, and the
// measurements three preregistered ablation arms actually depended on were not
// among them. Those arms became unscoreable and the runs had to be repeated.
//
// So pruning is driven by the preregistration for that specific arm, read from
// research/measurement-manifest.json, and a run reaches SAFE_TO_PRUNE only when
// every required measurement exists, parses, and contains observations.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Manifest is the required-measurements contract.
type Manifest struct {
	SchemaVersion  int                    `json:"schema_version"`
	AlwaysRequired []RequiredArtifact     `json:"always_required"`
	Arms           map[string]ArmContract `json:"arms"`
}

// RequiredArtifact is a file that must be present for every run.
type RequiredArtifact struct {
	Artifact string `json:"artifact"`
	Command  string `json:"command"`
	Source   string `json:"source"`
	Check    string `json:"check"`
}

// ArmContract is what one arm's preregistration needs measured.
type ArmContract struct {
	Preregistration string        `json:"preregistration"`
	Measurements    []Measurement `json:"measurements"`
}

// Measurement is one metric the arm's prediction rests on.
type Measurement struct {
	Metric   string `json:"metric"`
	Predicts string `json:"predicts"`
	Check    string `json:"check"`
}

// Status is the lifecycle position of one run.
type Status string

const (
	NotRun                Status = "NOT_RUN"
	RunComplete           Status = "RUN_COMPLETE"
	MeasurementIncomplete Status = "MEASUREMENT_INCOMPLETE"
	Scored                Status = "SCORED"
	SafeToPrune           Status = "SAFE_TO_PRUNE"
)

// Report is one run's evaluation.
type Report struct {
	Run        string   `json:"run"`
	Arm        string   `json:"arm"`
	Status     Status   `json:"status"`
	RawLogs    bool     `json:"raw_logs_present"`
	Missing    []string `json:"missing,omitempty"`
	Empty      []string `json:"empty,omitempty"`
	InVerdicts bool     `json:"in_verdicts"`
}

func main() {
	manifestPath := flag.String("manifest", "research/measurement-manifest.json", "required-measurements manifest")
	scoreboard := flag.String("scoreboard", "research/artifacts/scoreboard", "directory holding extracted measurements per run")
	logs := flag.String("logs", "logs", "directory holding raw run logs")
	verdicts := flag.String("verdicts", "research/artifacts/ablation-verdicts.json", "verdict artifact; an arm absent from it is not SCORED")
	prune := flag.Bool("prune", false, "delete raw logs for runs that reach SAFE_TO_PRUNE")
	asJSON := flag.Bool("json", false, "emit JSON instead of a table")
	flag.Parse()

	manifest, err := readManifest(*manifestPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	scored := readVerdictArms(*verdicts)

	names, err := runNames(*scoreboard, *logs)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	reports := make([]Report, 0, len(names))
	for _, name := range names {
		reports = append(reports, evaluate(name, manifest, *scoreboard, *logs, scored))
	}

	if *prune {
		for i := range reports {
			if reports[i].Status != SafeToPrune || !reports[i].RawLogs {
				continue
			}
			target := filepath.Join(*logs, reports[i].Run)
			if err := os.RemoveAll(target); err != nil {
				fmt.Fprintf(os.Stderr, "prune %s: %v\n", target, err)
				continue
			}
			reports[i].RawLogs = false
			fmt.Fprintf(os.Stderr, "pruned %s\n", target)
		}
	}

	if *asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(reports); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	fmt.Printf("%-38s %-30s %-24s %s\n", "run", "arm", "status", "raw logs")
	for _, report := range reports {
		fmt.Printf("%-38s %-30s %-24s %v\n", report.Run, report.Arm, report.Status, report.RawLogs)
		for _, missing := range report.Missing {
			fmt.Printf("      missing: %s\n", missing)
		}
		for _, empty := range report.Empty {
			fmt.Printf("      empty:   %s\n", empty)
		}
	}
}

func readManifest(path string) (*Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("prunegate: read manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("prunegate: parse manifest: %w", err)
	}
	if len(manifest.Arms) == 0 {
		return nil, fmt.Errorf("prunegate: manifest names no arms")
	}
	return &manifest, nil
}

// readVerdictArms lists the arms that already carry a recorded verdict. A run
// whose arm is not there has not been scored, whatever it has measured.
func readVerdictArms(path string) map[string]bool {
	scored := map[string]bool{}
	raw, err := os.ReadFile(path)
	if err != nil {
		return scored
	}
	var payload struct {
		Arms map[string]json.RawMessage `json:"arms"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return scored
	}
	for arm := range payload.Arms {
		scored[arm] = true
	}
	return scored
}

func runNames(scoreboard, logs string) ([]string, error) {
	seen := map[string]bool{}
	for _, dir := range []string{scoreboard, logs} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				seen[entry.Name()] = true
			}
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// armFor maps a run directory name onto the arm whose preregistration governs
// it. Control runs carry the control contract; anything unrecognised is
// reported rather than silently treated as generic, because treating an
// unrecognised run as generic is exactly the mistake this tool exists to stop.
func armFor(run string, manifest *Manifest) string {
	trimmed := strings.TrimPrefix(run, "r2_")
	if strings.HasPrefix(trimmed, "det") || strings.HasPrefix(trimmed, "control") {
		return "control"
	}
	best := ""
	for arm := range manifest.Arms {
		key := strings.ReplaceAll(strings.TrimPrefix(arm, "abl-"), "-", "_")
		if key == "control" {
			continue
		}
		if strings.Contains(trimmed, key) && len(key) > len(best) {
			best = arm
		}
	}
	if best == "" {
		return "unknown"
	}
	return best
}

func evaluate(run string, manifest *Manifest, scoreboard, logs string, scored map[string]bool) Report {
	report := Report{Run: run, Arm: armFor(run, manifest)}
	if _, err := os.Stat(filepath.Join(logs, run)); err == nil {
		report.RawLogs = true
	}

	extracted := filepath.Join(scoreboard, run)
	if _, err := os.Stat(extracted); err != nil {
		if report.RawLogs {
			report.Status = RunComplete
		} else {
			report.Status = NotRun
		}
		return report
	}
	report.Status = RunComplete

	contract, known := manifest.Arms[report.Arm]
	if !known {
		report.Missing = append(report.Missing, "no contract for this arm: refusing to judge it complete")
		report.Status = MeasurementIncomplete
		return report
	}

	for _, artifact := range manifest.AlwaysRequired {
		checkFile(&report, filepath.Join(extracted, artifact.Artifact), artifact.Artifact)
	}
	for _, measurement := range contract.Measurements {
		checkFile(&report, filepath.Join(extracted, measurement.Metric+".json"), measurement.Metric)
	}

	if len(report.Missing) > 0 || len(report.Empty) > 0 {
		report.Status = MeasurementIncomplete
		return report
	}
	report.InVerdicts = scored[report.Arm] || report.Arm == "control"
	if !report.InVerdicts {
		report.Status = RunComplete
		return report
	}
	report.Status = Scored
	// Everything the preregistration needs is extracted, parses, carries
	// observations, and the arm has a recorded verdict. Only now are the raw
	// logs redundant.
	report.Status = SafeToPrune
	return report
}

// checkFile records a required measurement as missing, unparseable or empty.
// "Empty" means the file parses but carries nothing to measure, which is the
// case a generic presence check would wave through.
func checkFile(report *Report, path, label string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		report.Missing = append(report.Missing, label)
		return
	}
	if len(raw) == 0 {
		report.Empty = append(report.Empty, label)
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		report.Missing = append(report.Missing, label+" (unparseable)")
		return
	}
	result, ok := payload["result"]
	if !ok {
		// manifest.json and greeks.json are copied whole rather than produced
		// by the analyzer, so they carry no result envelope.
		if len(payload) == 0 {
			report.Empty = append(report.Empty, label)
		}
		return
	}
	if !hasObservations(result) {
		report.Empty = append(report.Empty, label)
	}
}

// hasObservations reports whether a measurement carries anything at all: a
// non-empty collection, or a non-zero number somewhere in it. A result of all
// zeros and empty lists is a metric that ran and saw nothing, which is not a
// measurement.
func hasObservations(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for _, nested := range typed {
			if hasObservations(nested) {
				return true
			}
		}
		return false
	case []any:
		return len(typed) > 0
	case float64:
		return typed != 0
	case string:
		return typed != ""
	case bool:
		return typed
	case nil:
		return false
	}
	return false
}
