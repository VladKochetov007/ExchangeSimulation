package analysis

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMeasureSV1DCommandRetainsResourceTraceAndCompletion(t *testing.T) {
	root := t.TempDir()
	payload := filepath.Join(root, "payload.bin")
	measurement, err := MeasureSV1DCommand(context.Background(), SV1DResourceOptions{
		Command:                  []string{"/bin/sh", "-c", "printf payload > \"$1\"; sleep 0.08", "sh", payload},
		OutputParent:             root,
		MeasurementRoot:          root,
		SampleInterval:           250 * time.Millisecond,
		RequireFiniteCgroupLimit: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !measurement.Complete || measurement.ExitStatus != 0 {
		t.Fatalf("measurement completion = %t/%d: %+v", measurement.Complete, measurement.ExitStatus, measurement)
	}
	if measurement.SampleCount != uint64(len(measurement.Samples)) || measurement.SampleCount < 2 {
		t.Fatalf("sample count = %d/%d", measurement.SampleCount, len(measurement.Samples))
	}
	if measurement.SamplesSHA256 == "" || measurement.PeakApparentBytes == 0 || measurement.PeakAllocatedBytes == 0 || measurement.PeakProcessTreeRSSBytes == 0 {
		t.Fatalf("resource trace omitted a measured quantity: %+v", measurement)
	}
	if measurement.FinalAvailableBytes == 0 || measurement.MinimumHostMemAvailableBytes == 0 {
		t.Fatalf("host/filesystem measurements are incomplete: %+v", measurement)
	}
	if err := ValidateSV1DResourceMeasurement(measurement, false); err != nil {
		t.Fatal(err)
	}
	if data, readErr := os.ReadFile(payload); readErr != nil || string(data) != "payload" {
		t.Fatalf("measured command did not write payload: %q/%v", data, readErr)
	}
}

func TestMeasureSV1DCommandRecordsFailedChildWithoutCertifyingIt(t *testing.T) {
	root := t.TempDir()
	measurement, err := MeasureSV1DCommand(context.Background(), SV1DResourceOptions{
		Command:                  []string{"/bin/sh", "-c", "exit 7"},
		OutputParent:             root,
		MeasurementRoot:          root,
		SampleInterval:           10 * time.Millisecond,
		RequireFiniteCgroupLimit: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if measurement.Complete || measurement.ExitStatus != 7 {
		t.Fatalf("failed command was certified: %+v", measurement)
	}
	if measurement.SampleCount < 2 || measurement.SamplesSHA256 == "" {
		t.Fatalf("failed command did not retain its trace: %+v", measurement)
	}
}

func TestMeasureSV1DCommandProvidesParentBoundChildHandoff(t *testing.T) {
	root := t.TempDir()
	handoffPath := filepath.Join(root, "handoff.txt")
	measurement, err := MeasureSV1DCommand(context.Background(), SV1DResourceOptions{
		Command:      []string{"/bin/sh", "-c", "IFS= read -r handoff <&3; printf '%s' \"$handoff\" > \"$1\"; sleep 0.03", "sh", handoffPath},
		OutputParent: root, MeasurementRoot: root, SampleInterval: 10 * time.Millisecond,
		RequireFiniteCgroupLimit: false, RequireChildHandoff: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !measurement.Complete || measurement.ExitStatus != 0 {
		t.Fatalf("handoff command completion = %t/%d: %+v", measurement.Complete, measurement.ExitStatus, measurement)
	}
	handoff, err := os.ReadFile(handoffPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(handoff), sv1DResourceChildHandoffPrefix+":") {
		t.Fatalf("child handoff = %q", handoff)
	}
}

func TestResourceTreeFootprintRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte("target"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := resourceTreeFootprint(root); err == nil {
		t.Fatal("resource tree followed a symlink")
	}
}

func TestReadResourceCountersRejectsMissingAndDuplicateRequiredFields(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "memory.events")
	cases := []struct {
		name string
		body string
	}{
		{name: "missing oom kill", body: "oom 0\n"},
		{name: "duplicate oom", body: "oom 0\noom 1\noom_kill 0\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(tc.body), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := readResourceCounters(path); err == nil {
				t.Fatalf("accepted malformed counters: %s", tc.body)
			}
		})
	}
}

func TestReadResourceCountersRetainsRequiredAndExtensionFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.events")
	if err := os.WriteFile(path, []byte("low 0\noom 0\nfoo 3\noom_kill 0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	counters, err := readResourceCounters(path)
	if err != nil {
		t.Fatal(err)
	}
	if counters["oom"] != 0 || counters["oom_kill"] != 0 || counters["foo"] != 3 {
		t.Fatalf("counters = %#v", counters)
	}
}

func TestParseHostMemorySnapshotRequiresUniqueCompleteCounters(t *testing.T) {
	valid := "MemTotal: 100 kB\nMemAvailable: 50 kB\nSwapTotal: 20 kB\nSwapFree: 20 kB\n"
	available, swapUsed, err := parseHostMemorySnapshot(valid)
	if err != nil || available != 50*1024 || swapUsed != 0 {
		t.Fatalf("valid snapshot = %d/%d/%v", available, swapUsed, err)
	}
	cases := []string{
		"MemAvailable: 50 kB\nSwapTotal: 20 kB\n",
		"MemAvailable: 50 kB\nMemAvailable: 40 kB\nSwapTotal: 20 kB\nSwapFree: 20 kB\n",
		"MemAvailable: 50 MB\nSwapTotal: 20 kB\nSwapFree: 20 kB\n",
	}
	for _, raw := range cases {
		if _, _, err := parseHostMemorySnapshot(raw); err == nil {
			t.Fatalf("accepted malformed host memory snapshot: %q", raw)
		}
	}
}
