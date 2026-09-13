package analysis

import (
	"bytes"
	"context"
	"errors"
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

func TestMeasureSV1DCommandMaintainsStrictGapAcrossMultipleTicks(t *testing.T) {
	root := t.TempDir()
	payload := filepath.Join(root, "payload.bin")
	const maximumSampleGap = 100 * time.Millisecond
	measurement, err := MeasureSV1DCommand(context.Background(), SV1DResourceOptions{
		Command:                  []string{"/bin/sh", "-c", "printf payload > \"$1\"; sleep 0.75", "sh", payload},
		OutputParent:             root,
		MeasurementRoot:          root,
		SampleInterval:           maximumSampleGap,
		RequireFiniteCgroupLimit: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if measurement.SampleCount < 5 {
		t.Fatalf("measurement did not cover multiple sampling deadlines: %d samples", measurement.SampleCount)
	}
	if measurement.MaximumSampleGapNano > uint64(maximumSampleGap) {
		t.Fatalf("maximum observed gap = %d ns, contract = %d ns", measurement.MaximumSampleGapNano, maximumSampleGap)
	}
	if err := ValidateSV1DResourceMeasurement(measurement, false); err != nil {
		t.Fatalf("strict resource validation rejected a multi-tick trace: %v", err)
	}
}

func TestMeasureSV1DCommandRecordsFailedChildWithoutCertifyingIt(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	measurement, err := MeasureSV1DCommand(context.Background(), SV1DResourceOptions{
		Command:                  []string{"/bin/sh", "-c", "printf 'child output'; printf 'child diagnostic' >&2; exit 7"},
		OutputParent:             root,
		MeasurementRoot:          root,
		SampleInterval:           10 * time.Millisecond,
		RequireFiniteCgroupLimit: false,
		Stdout:                   &stdout,
		Stderr:                   &stderr,
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
	if stdout.String() != "child output" || stderr.String() != "child diagnostic" || !strings.Contains(measurement.Error, "exit status 7") {
		t.Fatalf("child diagnostics = %q/%q/%q", stdout.String(), stderr.String(), measurement.Error)
	}
}

func TestMeasureSV1DCommandRejectsUnboundedCgroupBeforeChildStarts(t *testing.T) {
	cgroupPath, err := cgroupPathForPID(os.Getpid())
	if err != nil {
		t.Skipf("cgroup v2 unavailable: %v", err)
	}
	cgroup, err := readResourceCgroup(cgroupPath)
	if err != nil {
		t.Skipf("cgroup memory counters unavailable: %v", err)
	}
	if cgroup.limit != 0 {
		t.Skip("requires an unbounded cgroup; current cgroup has a finite limit")
	}
	root := t.TempDir()
	marker := filepath.Join(root, "child-started")
	measurement, err := MeasureSV1DCommand(context.Background(), SV1DResourceOptions{
		Command:                  []string{"/bin/sh", "-c", "printf started > \"$1\"", "sh", marker},
		OutputParent:             root,
		MeasurementRoot:          root,
		RequireFiniteCgroupLimit: true,
	})
	if err == nil || !strings.Contains(err.Error(), "finite cgroup memory limit before start") {
		t.Fatalf("unbounded preflight error = %v", err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("child marker exists or cannot be inspected: %v", err)
	}
	if measurement.Complete || measurement.ExitStatus != -1 || measurement.Error != err.Error() || measurement.SampleCount != 1 || measurement.SamplesSHA256 == "" {
		t.Fatalf("preflight diagnostics lost: %+v", measurement)
	}
}

func TestMeasureSV1DCommandAcceptsFiniteInheritedCgroup(t *testing.T) {
	cgroupPath, err := cgroupPathForPID(os.Getpid())
	if err != nil {
		t.Skipf("cgroup v2 unavailable: %v", err)
	}
	cgroup, err := readResourceCgroup(cgroupPath)
	if err != nil {
		t.Skipf("cgroup memory counters unavailable: %v", err)
	}
	if cgroup.limit == 0 {
		t.Skip("requires a finite inherited cgroup memory limit")
	}
	root := t.TempDir()
	measurement, err := MeasureSV1DCommand(context.Background(), SV1DResourceOptions{
		Command:      []string{"/bin/sh", "-c", "printf payload > \"$1/payload\"; sleep 0.08", "sh", root},
		OutputParent: root, MeasurementRoot: root,
		SampleInterval:           250 * time.Millisecond,
		RequireFiniteCgroupLimit: true,
	})
	if err != nil || !measurement.Complete || measurement.ExitStatus != 0 || measurement.CgroupMemoryLimitBytes != cgroup.limit {
		t.Fatalf("finite inherited cgroup = %+v/%v", measurement, err)
	}
}

func TestMeasureSV1DCommandRetainsStartFailure(t *testing.T) {
	root := t.TempDir()
	measurement, err := MeasureSV1DCommand(context.Background(), SV1DResourceOptions{
		Command:      []string{filepath.Join(root, "missing-command")},
		OutputParent: root, MeasurementRoot: root,
	})
	if err == nil || !strings.Contains(err.Error(), "start measured command") || measurement.Error != err.Error() || measurement.Complete || measurement.ExitStatus != -1 {
		t.Fatalf("start failure = %+v/%v", measurement, err)
	}
	if measurement.SampleCount != 1 || measurement.SamplesSHA256 == "" {
		t.Fatalf("start failure lost preflight trace: %+v", measurement)
	}
}

func TestMeasureSV1DCommandRetainsExitStateWhenFinalSampleFails(t *testing.T) {
	for _, exitCode := range []string{"0", "7"} {
		t.Run(exitCode, func(t *testing.T) {
			root := t.TempDir()
			var stderr bytes.Buffer
			measurement, err := MeasureSV1DCommand(context.Background(), SV1DResourceOptions{
				Command:      []string{"/bin/sh", "-c", "sleep 0.1; ln -s target \"$1/link\"; printf 'child diagnostic' >&2; exit \"$2\"", "sh", root, exitCode},
				OutputParent: root, MeasurementRoot: root,
				SampleInterval: time.Second,
				Stderr:         &stderr,
			})
			if err == nil || !strings.Contains(err.Error(), "capture final resource sample") || !strings.Contains(err.Error(), "symlink") {
				t.Fatalf("final sample error = %v", err)
			}
			if measurement.Complete || measurement.ExitStatus != int(exitCode[0]-'0') || !strings.Contains(measurement.Error, err.Error()) {
				t.Fatalf("final sample failure lost exit state: %+v", measurement)
			}
			if stderr.String() != "child diagnostic" || (exitCode == "7" && !strings.Contains(measurement.Error, "exit status 7")) {
				t.Fatalf("diagnostics = %q/%q", stderr.String(), measurement.Error)
			}
			if measurement.SampleCount < 2 || measurement.SamplesSHA256 == "" {
				t.Fatalf("partial trace lost: %+v", measurement)
			}
		})
	}
}

func TestMeasureSV1DCommandRetainsKilledChildStateOnSamplingFailure(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	measurement, err := MeasureSV1DCommand(ctx, SV1DResourceOptions{
		Command:      []string{"/bin/sh", "-c", "sleep 0.1; ln -s target \"$1/link\"; sleep 10", "sh", root},
		OutputParent: root, MeasurementRoot: root,
		SampleInterval: 50 * time.Millisecond,
	})
	if err == nil || !strings.Contains(err.Error(), "symlink") || measurement.Complete || measurement.ExitStatus != -1 {
		t.Fatalf("sampling failure = %+v/%v", measurement, err)
	}
	if !strings.Contains(measurement.Error, "signal: killed") || !strings.Contains(measurement.Error, "symlink") || measurement.SamplesSHA256 == "" {
		t.Fatalf("sampling failure diagnostics lost: %+v", measurement)
	}
}

func TestVerifySV1DResourceCgroupRejectsPlacementAndLimitMismatch(t *testing.T) {
	cgroupPath, err := cgroupPathForPID(os.Getpid())
	if err != nil {
		t.Skipf("cgroup v2 unavailable: %v", err)
	}
	if err := verifySV1DResourceCgroupPlacement(os.Getpid(), cgroupPath); err != nil {
		t.Fatal(err)
	}
	if err := verifySV1DResourceCgroupPlacement(os.Getpid(), cgroupPath+"/different"); err == nil || !strings.Contains(err.Error(), "placement mismatch") {
		t.Fatalf("placement mismatch error = %v", err)
	}
	for _, limit := range []uint64{0, 2048} {
		if err := verifySV1DResourceCgroupLimit(SV1DResourceSample{CgroupMemoryLimitBytes: limit}, 1024); err == nil {
			t.Fatalf("accepted changed limit %d", limit)
		}
	}
}

func TestMeasureSV1DCommandForwardsInheritedFileDescriptor(t *testing.T) {
	root := t.TempDir()
	lockPath := filepath.Join(root, "namespace.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer lockFile.Close()
	descriptorPath := filepath.Join(root, "descriptor.txt")
	measurement, err := MeasureSV1DCommand(context.Background(), SV1DResourceOptions{
		Command:                  []string{"/bin/sh", "-c", "readlink /proc/$$/fd/3 > \"$1\"; sleep 0.03", "sh", descriptorPath},
		OutputParent:             root,
		MeasurementRoot:          root,
		SampleInterval:           10 * time.Millisecond,
		RequireFiniteCgroupLimit: false,
		InheritedFileDescriptors: []int{int(lockFile.Fd())},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !measurement.Complete || measurement.ExitStatus != 0 {
		t.Fatalf("handoff command completion = %t/%d: %+v", measurement.Complete, measurement.ExitStatus, measurement)
	}
	descriptorTarget, err := os.ReadFile(descriptorPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(descriptorTarget)) != lockPath {
		t.Fatalf("inherited descriptor target = %q, want %q", descriptorTarget, lockPath)
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
