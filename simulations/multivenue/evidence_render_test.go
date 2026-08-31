package multivenue

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRenderBinaryEvidenceMergesEvidenceOnlySidecarsByVenueSequence(t *testing.T) {
	inputDir := t.TempDir()
	venueDir := filepath.Join(inputDir, "venues", "north")
	if err := os.MkdirAll(venueDir, 0755); err != nil {
		t.Fatal(err)
	}
	eventsFile, err := os.Create(filepath.Join(inputDir, "events.evs"))
	if err != nil {
		t.Fatal(err)
	}
	sink := newBinaryEvidence(eventsFile)
	if err := sink.record(10, 7, "first", "north", map[string]int{"value": 1}, "general.jsonl", 1); err != nil {
		t.Fatal(err)
	}
	if err := sink.record(30, 9, "third", "north", map[string]int{"value": 3}, "general.jsonl", 3); err != nil {
		t.Fatal(err)
	}
	if err := sink.finish(); err != nil {
		t.Fatal(err)
	}
	if err := eventsFile.Close(); err != nil {
		t.Fatal(err)
	}
	second := []byte(`{"client_id":8,"data":{"venue_id":"north","sequence":2,"payload":{"value":2}},"event":"evidence_only","sim_ts":20}`)
	if err := os.WriteFile(filepath.Join(venueDir, "general.jsonl"), append(second, '\n'), 0644); err != nil {
		t.Fatal(err)
	}

	outOne := filepath.Join(t.TempDir(), "rendered")
	report, err := RenderBinaryEvidence(inputDir, outOne)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if report.EventFrames != 2 || report.DictionaryFrames == 0 || report.Routes != 1 || report.ExecutionHash == "" {
		t.Fatalf("report = %+v", report)
	}
	want := []byte(`{"client_id":7,"data":{"venue_id":"north","sequence":1,"payload":{"value":1}},"event":"first","sim_ts":10}` + "\n")
	want = append(want, second...)
	want = append(want, '\n')
	want = append(want, []byte(`{"client_id":9,"data":{"venue_id":"north","sequence":3,"payload":{"value":3}},"event":"third","sim_ts":30}`)...)
	want = append(want, '\n')
	actual, err := os.ReadFile(filepath.Join(outOne, "venues", "north", "general.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, want) {
		t.Fatalf("rendered bytes differ:\n got %s\nwant %s", actual, want)
	}

	outTwo := filepath.Join(t.TempDir(), "rendered")
	if _, err := RenderBinaryEvidence(inputDir, outTwo); err != nil {
		t.Fatalf("second render: %v", err)
	}
	actualTwo, err := os.ReadFile(filepath.Join(outTwo, "venues", "north", "general.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, actualTwo) {
		t.Fatal("reconstruction is not deterministic")
	}
}

func TestRenderBinaryEvidenceRejectsUnterminatedStream(t *testing.T) {
	inputDir := t.TempDir()
	file, err := os.Create(filepath.Join(inputDir, "events.evs"))
	if err != nil {
		t.Fatal(err)
	}
	sink := newBinaryEvidence(file)
	if err := sink.record(1, 1, "event", "north", map[string]int{"value": 1}, "general.jsonl", 1); err != nil {
		t.Fatal(err)
	}
	if err := sink.flush(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := RenderBinaryEvidence(inputDir, filepath.Join(t.TempDir(), "rendered")); err == nil {
		t.Fatal("unterminated binary evidence accepted")
	}
}

func TestBinaryEvidenceFormatIsExplicitAndAttested(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "run")
	sim, err := NewSim(time.Second, Config{
		LogDir: dir, LogMode: "none", EvidenceFormat: binaryRepresentation, Seed: 101,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sim.checkpoints == nil || sim.checkpoints.binary == nil {
		t.Fatal("explicit binary format did not create a binary sink")
	}
	if err := sim.Close(); err != nil {
		t.Fatal(err)
	}
	manifestRaw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Config struct {
			EvidenceFormat string `json:"evidence_format"`
		} `json:"config"`
	}
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Config.EvidenceFormat != binaryRepresentation {
		t.Fatalf("manifest evidence format = %q, want %q", manifest.Config.EvidenceFormat, binaryRepresentation)
	}
	attestationRaw, err := os.ReadFile(filepath.Join(dir, "binary-evidence-attestation.json"))
	if err != nil {
		t.Fatal(err)
	}
	var attestation binaryEvidenceArtifactRecord
	if err := json.Unmarshal(attestationRaw, &attestation); err != nil {
		t.Fatal(err)
	}
	if attestation.Domain != "canonical_binary_execution_frames" || attestation.Ordering != "ordered_stream" || attestation.ExecutionStreamHash == "" {
		t.Fatalf("binary attestation = %+v", attestation)
	}
	if _, err := os.Stat(filepath.Join(dir, "evidence-artifact-hash.json")); !os.IsNotExist(err) {
		t.Fatalf("legacy JSON artifact exists for binary run: %v", err)
	}
}

func TestConfigRejectsUnknownEvidenceFormat(t *testing.T) {
	_, err := NewSim(time.Second, Config{LogDir: t.TempDir(), LogMode: "none", EvidenceFormat: "future"})
	if err == nil {
		t.Fatal("unknown evidence format accepted")
	}
}
