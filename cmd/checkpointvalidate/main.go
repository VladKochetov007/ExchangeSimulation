package main

import (
	"flag"
	"fmt"
	"os"

	"exchange_sim/simulations/multivenue"
)

func main() {
	checkpointsPath := flag.String("checkpoints", "", "JSONL checkpoint stream to validate")
	simulationStartNano := flag.Int64("start", 0, "inclusive simulation start timestamp in nanoseconds")
	simulationEndNano := flag.Int64("end", 0, "inclusive simulation end timestamp in nanoseconds")
	attestationPath := flag.String("attestation", "", "optional binary-evidence-attestation.json to bind")
	flag.Parse()
	if *checkpointsPath == "" {
		fail("-checkpoints is required")
	}

	checkpoints, err := os.Open(*checkpointsPath)
	if err != nil {
		fail("open checkpoints: %v", err)
	}
	defer checkpoints.Close()

	var attestation *multivenue.BinaryCheckpointAttestation
	if *attestationPath != "" {
		attestationFile, err := os.Open(*attestationPath)
		if err != nil {
			fail("open attestation: %v", err)
		}
		parsedAttestation, err := multivenue.ReadBinaryCheckpointAttestation(attestationFile)
		closeErr := attestationFile.Close()
		if err != nil {
			fail("read attestation: %v", err)
		}
		if closeErr != nil {
			fail("close attestation: %v", closeErr)
		}
		attestation = &parsedAttestation
	}

	if err := multivenue.ValidateBinaryCheckpointStream(
		checkpoints,
		*simulationStartNano,
		*simulationEndNano,
		attestation,
	); err != nil {
		fail("validate checkpoints: %v", err)
	}
}

func fail(format string, arguments ...any) {
	if len(arguments) == 0 {
		fmt.Fprintln(os.Stderr, format)
	} else {
		fmt.Fprintf(os.Stderr, format+"\n", arguments...)
	}
	os.Exit(1)
}
