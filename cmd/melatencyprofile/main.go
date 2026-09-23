// Command melatencyprofile profiles only retained, verified ME-001 evidence.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"exchange_sim/experiment/executionpilot"
)

type namedProfile struct {
	Run     string                               `json:"run"`
	Profile executionpilot.RetainedTimingProfile `json:"profile"`
}

func main() {
	cutoffNS := flag.Int64("cutoff-ns", 1_000_000_000, "pre-intervention cutoff in simulated nanoseconds")
	symbol := flag.String("symbol", "ABC/USD", "retained spot symbol")
	clientID := flag.Uint64("client", 13, "focal actor client ID")
	targetsCSV := flag.String("targets", "50000000,500000000", "comma-separated fixed-point target quantities")
	outputPath := flag.String("out", "", "new output JSON path; stdout when omitted")
	flag.Parse()
	if flag.NArg() == 0 {
		fail(errors.New("at least one retained run directory is required"))
	}
	targets, err := parseTargets(*targetsCSV)
	if err != nil {
		fail(err)
	}
	profiles := make([]namedProfile, 0, flag.NArg())
	for _, run := range flag.Args() {
		profile, err := profileRun(run, *symbol, *clientID, *cutoffNS, targets)
		if err != nil {
			fail(fmt.Errorf("%s: %w", run, err))
		}
		profiles = append(profiles, namedProfile{Run: run, Profile: profile})
	}
	output := os.Stdout
	var created *os.File
	if *outputPath != "" {
		created, err = os.OpenFile(*outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			fail(err)
		}
		output = created
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(profiles); err != nil {
		fail(err)
	}
	if created != nil {
		if err := created.Sync(); err != nil {
			fail(err)
		}
		if err := created.Close(); err != nil {
			fail(err)
		}
	}
}

func parseTargets(input string) ([]int64, error) {
	parts := strings.Split(input, ",")
	targets := make([]int64, 0, len(parts))
	for _, part := range parts {
		quantity, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || quantity <= 0 {
			return nil, errors.New("invalid fixed-point target list")
		}
		targets = append(targets, quantity)
	}
	return targets, nil
}

func profileRun(run, symbol string, clientID uint64, cutoffNS int64, targets []int64) (executionpilot.RetainedTimingProfile, error) {
	var manifest executionpilot.RunManifest
	raw, err := os.ReadFile(filepath.Join(run, executionpilot.ManifestFilename))
	if err != nil {
		return executionpilot.RetainedTimingProfile{}, err
	}
	if err := json.Unmarshal(raw, &manifest); err != nil || manifest.SchemaVersion != 1 || manifest.Evidence.SchemaID != executionpilot.EvidenceSchemaID {
		return executionpilot.RetainedTimingProfile{}, errors.New("invalid retained run manifest")
	}
	evidence, err := os.Open(filepath.Join(run, executionpilot.EvidenceFilename))
	if err != nil {
		return executionpilot.RetainedTimingProfile{}, err
	}
	defer evidence.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, evidence); err != nil {
		return executionpilot.RetainedTimingProfile{}, err
	}
	if hex.EncodeToString(digest.Sum(nil)) != manifest.EvidenceFileSHA256 {
		return executionpilot.RetainedTimingProfile{}, errors.New("retained evidence file hash mismatch")
	}
	if _, err := evidence.Seek(0, io.SeekStart); err != nil {
		return executionpilot.RetainedTimingProfile{}, err
	}
	return executionpilot.ProfileRetainedTiming(evidence, manifest.Evidence, symbol, clientID, cutoffNS, targets)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "melatencyprofile:", err)
	os.Exit(1)
}
