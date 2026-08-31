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
	requiredFreeze := flag.String("simulator-freeze", "ae13f9aa6e5fd23539637a8c4a3d2d4f4c3ad107", "simulator freeze required in the verdict artifact before an arm may be pruned")
	prune := flag.Bool("prune", false, "delete raw logs for runs that reach SAFE_TO_PRUNE")
	asJSON := flag.Bool("json", false, "emit JSON instead of a table")
	flag.Parse()

	manifest, err := readManifest(*manifestPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	scored := readVerdictArms(*verdicts, *requiredFreeze)

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
func readVerdictArms(path, requiredFreeze string) map[string]bool {
	scored := map[string]bool{}
	raw, err := os.ReadFile(path)
	if err != nil {
		return scored
	}
	var payload struct {
		SimulatorFreeze string                     `json:"simulator_freeze"`
		Arms            map[string]json.RawMessage `json:"arms"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return scored
	}
	if payload.SimulatorFreeze != requiredFreeze {
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
	if strings.HasPrefix(trimmed, "det") || strings.HasPrefix(trimmed, "control") || strings.HasPrefix(trimmed, "f2_baseline_") {
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

	// A run whose evidence was written in a format the analyzers cannot read
	// must never reach SAFE_TO_PRUNE. Its artifacts are produced and non-empty,
	// so every requirement below passes on numbers derived from a fraction of
	// the events — `streamhash` counts 28,193 instead of 1,597,303 and still
	// satisfies "events > 0". Certifying that run would authorise deleting the
	// only readable copy of evidence the measurements never actually covered.
	if format := evidenceFormat(extracted); format != "" {
		report.Missing = append(report.Missing,
			"evidence is stored as "+format+", which the analyzers cannot read: "+
				"refusing to certify measurements taken over a fraction of the events")
		report.Status = MeasurementIncomplete
		return report
	}

	contract, known := manifest.Arms[report.Arm]
	if !known {
		report.Missing = append(report.Missing, "no contract for this arm: refusing to judge it complete")
		report.Status = MeasurementIncomplete
		return report
	}

	for _, artifact := range manifest.AlwaysRequired {
		checkFile(&report, filepath.Join(extracted, artifact.Artifact), artifact.Artifact, artifact.Check)
	}
	for _, measurement := range contract.Measurements {
		checkFile(&report, filepath.Join(extracted, measurement.Metric+".json"), measurement.Metric, measurement.Check)
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
func checkFile(report *Report, path, label, requirement string) {
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
	if err := checkRequirement(payload, requirement); err != nil {
		report.Empty = append(report.Empty, label+" ("+err.Error()+")")
	}
}

// evidenceFormat reports a run's declared non-default evidence format, or "" for
// the JSONL default. A run with no manifest, or an unparseable one, is reported
// as default here; the artifact checks below are what catch those.
func evidenceFormat(dir string) string {
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return ""
	}
	var manifest struct {
		EvidenceFormat string `json:"evidence_format"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return ""
	}
	return manifest.EvidenceFormat
}

// checkRequirement evaluates the small, deliberately explicit expression
// vocabulary used in measurement-manifest.json. The old generic recursive
// nonzero scan could certify a required primary field solely because an
// unrelated diagnostic happened to be nonzero.
func checkRequirement(payload map[string]any, expression string) error {
	for _, clause := range strings.Split(expression, " && ") {
		clause = strings.TrimSpace(clause)
		switch {
		case strings.HasSuffix(clause, " > 0"):
			path := strings.TrimSuffix(clause, " > 0")
			value, found := lookupPath(payload, path)
			number, numeric := value.(float64)
			if !found || !numeric || number <= 0 {
				return fmt.Errorf("requires %s", clause)
			}
		case strings.HasSuffix(clause, " present"):
			path := strings.TrimSuffix(clause, " present")
			if _, found := lookupPath(payload, path); !found {
				return fmt.Errorf("requires %s", clause)
			}
		case strings.HasSuffix(clause, " non-empty"):
			path := strings.TrimSuffix(clause, " non-empty")
			value, found := lookupPath(payload, path)
			if !found || !nonEmpty(value) {
				return fmt.Errorf("requires %s", clause)
			}
		default:
			return fmt.Errorf("unknown manifest check %q", clause)
		}
	}
	return nil
}

func lookupPath(payload map[string]any, path string) (any, bool) {
	var current any = payload
	for _, component := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[component]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func nonEmpty(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		return len(typed) > 0
	case []any:
		return len(typed) > 0
	case string:
		return typed != ""
	case float64:
		return typed != 0
	case bool:
		return typed
	default:
		return value != nil
	}
}
