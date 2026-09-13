package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"exchange_sim/analysis"
)

func TestSV1DResourceCLIHelperProcess(t *testing.T) {
	if os.Getenv("SV1D_RESOURCE_CLI_TEST_HELPER") != "1" {
		return
	}
	os.Args = append([]string{os.Args[0]}, os.Args[3:]...)
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	main()
}

func TestCLIForwardsChildDiagnosticsAndPublishesExitCode(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "measurement.json")
	command := exec.Command(os.Args[0], "-test.run=^TestSV1DResourceCLIHelperProcess$", "--",
		"-out", output, "-output-parent", root, "-measurement-root", root,
		"-require-finite-cgroup=false", "--", "/bin/sh", "-c",
		"printf 'child output'; printf 'child diagnostic' >&2; exit 7")
	command.Env = append(os.Environ(), "SV1D_RESOURCE_CLI_TEST_HELPER=1")
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err == nil || command.ProcessState == nil || command.ProcessState.ExitCode() != 1 {
		t.Fatalf("CLI failure state = %v/%v", command.ProcessState, err)
	}
	if stdout.String() != "child output" || !strings.Contains(stderr.String(), "child diagnostic") || !strings.Contains(stderr.String(), "exit status 7") {
		t.Fatalf("CLI diagnostics = %q/%q", stdout.String(), stderr.String())
	}
	raw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var measurement analysis.SV1DResourceMeasurement
	if err := json.Unmarshal(raw, &measurement); err != nil {
		t.Fatal(err)
	}
	if measurement.Complete || measurement.ExitStatus != 7 || !strings.Contains(measurement.Error, "exit status 7") || measurement.SamplesSHA256 == "" {
		t.Fatalf("CLI lost failed child state: %+v", measurement)
	}
}

func TestValidateCommandMeasurementReportsChildExitBeforeContractError(t *testing.T) {
	measurement := analysis.SV1DResourceMeasurement{ExitStatus: 7}
	for _, requireFiniteCgroup := range []bool{false, true} {
		err := validateCommandMeasurement(measurement, requireFiniteCgroup)
		if err == nil || err.Error() != "measured command was incomplete: exit status 7" {
			t.Fatalf("require finite cgroup %t: diagnostic = %v", requireFiniteCgroup, err)
		}
	}
	measurement.Complete = true
	measurement.ExitStatus = 0
	if err := validateCommandMeasurement(measurement, true); err == nil || !strings.Contains(err.Error(), "invalid contract") {
		t.Fatalf("completed measurement bypassed aggregate validation: %v", err)
	}
}

func TestPublishMeasurementRefusesOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "measurement.json")
	measurement := analysis.SV1DResourceMeasurement{SchemaVersion: 1, Contract: analysis.SV1DResourceMeasurementContract}
	if err := publishMeasurement(path, measurement); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := publishMeasurement(path, measurement); err == nil {
		t.Fatal("measurement publisher overwrote an existing record")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("existing measurement record changed")
	}
}
