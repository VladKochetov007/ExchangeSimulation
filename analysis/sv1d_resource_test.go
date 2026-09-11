package analysis

import (
	"context"
	"os"
	"path/filepath"
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
