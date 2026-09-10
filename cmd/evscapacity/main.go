// Command evscapacity produces the preregistered outcome-neutral binary
// evidence capacity workload. It never invokes the market simulator.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"exchange_sim/evstream/synthetic"
)

type evidenceAttestation struct {
	SchemaVersion       int              `json:"schema_version"`
	Contract            string           `json:"contract"`
	Domain              string           `json:"domain"`
	Ordering            string           `json:"ordering"`
	Hashing             string           `json:"hashing"`
	OutcomeNeutral      bool             `json:"outcome_neutral"`
	SimulatorInvoked    bool             `json:"simulator_invoked"`
	HoldoutsConsumed    bool             `json:"holdouts_consumed"`
	ReadbackVerified    bool             `json:"readback_verified"`
	UnencodablePayloads uint64           `json:"unencodable_payloads"`
	Report              synthetic.Report `json:"report"`
}

type profileDocument struct {
	SchemaVersion int               `json:"schema_version"`
	Contract      string            `json:"contract"`
	Profile       synthetic.Profile `json:"profile"`
}

func main() {
	outputPath := flag.String("out", "", "path for events.evs")
	reportPath := flag.String("report", "", "path for synthetic-capacity-report.json")
	attestationPath := flag.String("attestation", "", "path for binary-evidence-attestation.json")
	profilePath := flag.String("profile-out", "", "path for workload-profile.json")
	profileName := flag.String("profile", synthetic.DefaultProfileName, "registered workload profile")
	seed := flag.Uint64("seed", synthetic.DefaultWorkloadSeed, "synthetic workload seed")
	eventCount := flag.Uint64("event-count", synthetic.ProductionProfile().EventCount, "synthetic event count")
	startNano := flag.Int64("start-nano", synthetic.DefaultStartNano, "synthetic timestamp start")
	endNano := flag.Int64("end-nano", synthetic.DefaultEndNano, "synthetic timestamp end")
	flag.Parse()

	if *outputPath == "" || *reportPath == "" || *attestationPath == "" || *profilePath == "" {
		fmt.Fprintln(os.Stderr, "evscapacity: -out, -report, -attestation, and -profile-out are required")
		os.Exit(2)
	}
	profile := synthetic.NewProfile(*eventCount)
	profile.Name = *profileName
	profile.WorkloadSeed = *seed
	profile.StartNano = *startNano
	profile.EndNano = *endNano
	if err := writeWorkload(profile, *outputPath, *reportPath, *attestationPath, *profilePath); err != nil {
		fmt.Fprintf(os.Stderr, "evscapacity: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("synthetic capacity workload complete: events=%d stream=%s\n", profile.EventCount, *outputPath)
}

func writeWorkload(profile synthetic.Profile, outputPath, reportPath, attestationPath, profilePath string) error {
	if err := profile.Validate(); err != nil {
		return err
	}
	output, err := createExclusive(outputPath)
	if err != nil {
		return fmt.Errorf("create stream: %w", err)
	}
	report, writeErr := synthetic.Write(output, profile)
	closeErr := output.Close()
	if writeErr != nil {
		return fmt.Errorf("write stream: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close stream: %w", closeErr)
	}
	input, err := os.Open(outputPath)
	if err != nil {
		return fmt.Errorf("open stream for readback: %w", err)
	}
	verifyErr := synthetic.Verify(input, report)
	readCloseErr := input.Close()
	if verifyErr != nil {
		return fmt.Errorf("verify stream: %w", verifyErr)
	}
	if readCloseErr != nil {
		return fmt.Errorf("close readback stream: %w", readCloseErr)
	}
	report.ReadbackVerified = true
	if err := writeJSONExclusive(profilePath, profileDocument{
		SchemaVersion: 1,
		Contract:      synthetic.Contract,
		Profile:       profile,
	}); err != nil {
		return fmt.Errorf("write profile: %w", err)
	}
	if err := writeJSONExclusive(reportPath, report); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	attestation := evidenceAttestation{
		SchemaVersion:       1,
		Contract:            "v2-r2-sv1d-synthetic-capacity-evidence-v1",
		Domain:              "canonical_binary_execution_frames",
		Ordering:            report.Ordering,
		Hashing:             report.Hashing,
		OutcomeNeutral:      true,
		SimulatorInvoked:    false,
		HoldoutsConsumed:    false,
		ReadbackVerified:    report.ReadbackVerified,
		UnencodablePayloads: report.UnencodablePayloads,
		Report:              report,
	}
	if err := writeJSONExclusive(attestationPath, attestation); err != nil {
		return fmt.Errorf("write attestation: %w", err)
	}
	return nil
}

func createExclusive(path string) (*os.File, error) {
	if path == "" || filepath.Clean(path) != path || filepath.IsAbs(path) == false {
		return nil, errors.New("output paths must be clean absolute paths")
	}
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
}

func writeJSONExclusive(path string, value any) error {
	file, err := createExclusive(path)
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err == nil {
		encoded = append(encoded, '\n')
		_, err = file.Write(encoded)
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return nil
}
