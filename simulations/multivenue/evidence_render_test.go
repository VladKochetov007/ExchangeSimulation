package multivenue

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"

	"exchange_sim/evstream"
	"exchange_sim/exchange"
	"exchange_sim/simulations/feesim"
	etypes "exchange_sim/types"
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
	writeRenderMetadata(t, inputDir, sink, "full", second)

	outOne := filepath.Join(t.TempDir(), "rendered")
	report, err := RenderBinaryEvidence(inputDir, outOne)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if report.EventFrames != 2 || report.DictionaryFrames == 0 || report.Routes != 1 || report.ExecutionHash == "" || report.RouteCompression != string(RouteCompressionNone) {
		t.Fatalf("report = %+v", report)
	}
	want := []byte(`{"client_id":7,"data":{"venue_id":"north","sequence":1,"payload":{"value":1}},"event":"first","sim_ts":10,"event_seq":4}` + "\n")
	want = append(want, second...)
	want = append(want, '\n')
	want = append(want, []byte(`{"client_id":9,"data":{"venue_id":"north","sequence":3,"payload":{"value":3}},"event":"third","sim_ts":30,"event_seq":6}`)...)
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

	compressedDir := filepath.Join(t.TempDir(), "rendered")
	compressedReport, err := RenderBinaryEvidenceWithOptions(inputDir, compressedDir, BinaryRenderOptions{
		RouteCompression: RouteCompressionZstd,
	})
	if err != nil {
		t.Fatalf("compressed render: %v", err)
	}
	if compressedReport.RouteCompression != string(RouteCompressionZstd) {
		t.Fatalf("compressed report = %+v", compressedReport)
	}
	compressedFile, err := os.Open(filepath.Join(compressedDir, "venues", "north", "general.jsonl.zst"))
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := zstd.NewReader(compressedFile, zstd.WithDecoderConcurrency(1))
	if err != nil {
		_ = compressedFile.Close()
		t.Fatal(err)
	}
	decoded, err := io.ReadAll(decoder)
	decoder.Close()
	_ = compressedFile.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, want) {
		t.Fatalf("compressed route decoded bytes differ:\n got %s\nwant %s", decoded, want)
	}
}

func TestRenderBinaryEvidenceMergesGlobalSidecarsByLogicalSequence(t *testing.T) {
	inputDir := t.TempDir()
	for _, venue := range []string{"north", "south"} {
		if err := os.MkdirAll(filepath.Join(inputDir, "venues", venue), 0755); err != nil {
			t.Fatal(err)
		}
	}
	eventsFile, err := os.Create(filepath.Join(inputDir, "events.evs"))
	if err != nil {
		t.Fatal(err)
	}
	sink := newGlobalNeutralBinaryEvidence(eventsFile)
	if err := sink.recordGlobal(1, 7, "binary_north", "north", map[string]int{"value": 1}, "general.jsonl", 1, 1); err != nil {
		t.Fatal(err)
	}
	if err := sink.recordGlobal(4, 9, "binary_south", "south", map[string]int{"value": 4}, "general.jsonl", 2, 4); err != nil {
		t.Fatal(err)
	}
	if err := sink.finish(); err != nil {
		t.Fatal(err)
	}
	if err := eventsFile.Close(); err != nil {
		t.Fatal(err)
	}
	northSidecar := []byte(`{"client_id":8,"data":{"venue_id":"north","sequence":2,"payload":{"value":3}},"event":"sidecar_north","sim_ts":3,"event_seq":3}`)
	southSidecar := []byte(`{"client_id":10,"data":{"venue_id":"south","sequence":1,"payload":{"value":2}},"event":"sidecar_south","sim_ts":2,"event_seq":2}`)
	if err := os.WriteFile(filepath.Join(inputDir, "venues", "north", "general.jsonl"), append(northSidecar, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "venues", "south", "general.jsonl"), append(southSidecar, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	writeRenderMetadata(t, inputDir, sink, "full", northSidecar, southSidecar)

	outDir := filepath.Join(t.TempDir(), "rendered")
	report, err := RenderBinaryEvidence(inputDir, outDir)
	if err != nil {
		t.Fatalf("render global evidence: %v", err)
	}
	if report.FullEvidenceHash == "" {
		t.Fatalf("global render omitted canonical full-evidence hash: %+v", report)
	}
	renderedNorth, err := os.ReadFile(filepath.Join(outDir, "venues", "north", "general.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(renderedNorth, []byte(`{"client_id":7,"data":{"venue_id":"north","sequence":1,"payload":{"value":1}},"event":"binary_north","sim_ts":1,"event_seq":1}`)) {
		t.Fatalf("logical global sequence was not rendered independently of transport frames: %s", renderedNorth)
	}
	// The south sidecar has global sequence 2 and must precede the south binary
	// frame at sequence 4 even though the binary stream stores that frame first.
	renderedSouth, err := os.ReadFile(filepath.Join(outDir, "venues", "south", "general.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(renderedSouth, []byte(`{"client_id":10,"data":{"venue_id":"south","sequence":1,"payload":{"value":2}},"event":"sidecar_south","sim_ts":2,"event_seq":2}`)) {
		t.Fatalf("global sidecar was not emitted before later binary frame: %s", renderedSouth)
	}
}

func TestRenderOutputRejectsGlobalSequenceMutations(t *testing.T) {
	tests := []struct {
		name         string
		firstGlobal  uint64
		secondGlobal uint64
		want         string
	}{
		{name: "zero", firstGlobal: 0, secondGlobal: 0, want: "zero global sequence"},
		{name: "duplicate", firstGlobal: 1, secondGlobal: 1, want: "duplicate or out-of-order global sequence"},
		{name: "missing", firstGlobal: 1, secondGlobal: 3, want: "missing global sequence"},
		{name: "swapped", firstGlobal: 2, secondGlobal: 1, want: "missing global sequence"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output, err := newRenderOutput(filepath.Join(t.TempDir(), "rendered"), RouteCompressionNone, true)
			if err != nil {
				t.Fatal(err)
			}
			defer output.cleanup()
			first := renderRecord{sequence: 1, globalSequence: test.firstGlobal, raw: []byte("{}")}
			if err := output.append(renderRouteKey{venue: "north", route: "general.jsonl"}, first); err != nil {
				if test.firstGlobal == 0 || test.firstGlobal == 2 {
					if !strings.Contains(err.Error(), test.want) {
						t.Fatalf("first append error = %v, want %q", err, test.want)
					}
					return
				}
				t.Fatal(err)
			}
			second := renderRecord{sequence: 2, globalSequence: test.secondGlobal, raw: []byte("{}")}
			if err := output.append(renderRouteKey{venue: "south", route: "general.jsonl"}, second); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("second append error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestBinaryEvidenceGlobalEnvelopeRequiresLogicalSequence(t *testing.T) {
	global := newGlobalNeutralBinaryEvidence(io.Discard)
	if err := global.record(1, 1, "event", "north", map[string]int{"value": 1}, "general.jsonl", 1); err == nil {
		t.Fatal("global contract accepted an envelope without a logical sequence")
	}
	legacy := newNeutralBinaryEvidence(io.Discard)
	if err := legacy.recordGlobal(1, 1, "event", "north", map[string]int{"value": 1}, "general.jsonl", 1, 1); err == nil {
		t.Fatal("legacy contract accepted a logical sequence envelope")
	}
}

func TestRenderBinaryEvidenceRejectsOutOfOrderSidecarSequence(t *testing.T) {
	inputDir := t.TempDir()
	venueDir := filepath.Join(inputDir, "venues", "north")
	if err := os.MkdirAll(venueDir, 0755); err != nil {
		t.Fatal(err)
	}
	eventsFile, err := os.Create(filepath.Join(inputDir, "events.evs"))
	if err != nil {
		t.Fatal(err)
	}
	sink := newNeutralBinaryEvidence(eventsFile)
	if err := sink.record(3, 7, "third", "north", map[string]int{"value": 3}, "general.jsonl", 3); err != nil {
		t.Fatal(err)
	}
	if err := sink.finish(); err != nil {
		t.Fatal(err)
	}
	if err := eventsFile.Close(); err != nil {
		t.Fatal(err)
	}
	second := []byte(`{"client_id":8,"data":{"venue_id":"north","sequence":2,"payload":{"value":2}},"event":"second","sim_ts":2}`)
	first := []byte(`{"client_id":9,"data":{"venue_id":"north","sequence":1,"payload":{"value":1}},"event":"first","sim_ts":1}`)
	if err := os.WriteFile(filepath.Join(venueDir, "general.jsonl"), append(append(second, '\n'), append(first, '\n')...), 0644); err != nil {
		t.Fatal(err)
	}
	writeRenderMetadata(t, inputDir, sink, "full", second, first)
	if _, err := RenderBinaryEvidence(inputDir, filepath.Join(t.TempDir(), "rendered")); err == nil || !strings.Contains(err.Error(), "canonical reconstruction stream") {
		t.Fatalf("out-of-order sidecar sequence was accepted: %v", err)
	}
}

func TestRenderBinaryEvidenceRejectsSymlinkedOutputAlias(t *testing.T) {
	inputDir := t.TempDir()
	outputParent := t.TempDir()
	outputAlias := filepath.Join(outputParent, "rendered")
	if err := os.Symlink(inputDir, outputAlias); err != nil {
		t.Fatal(err)
	}
	if _, err := RenderBinaryEvidence(inputDir, outputAlias); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlinked output alias was accepted: %v", err)
	}
}

func TestRenderBinaryEvidenceRejectsSymlinkedOutputParent(t *testing.T) {
	inputDir := t.TempDir()
	outputTarget := t.TempDir()
	outputParent := filepath.Join(t.TempDir(), "parent")
	if err := os.Symlink(outputTarget, outputParent); err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(outputParent, "rendered")
	if _, err := RenderBinaryEvidence(inputDir, outputDir); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlinked output parent was accepted: %v", err)
	}
}

func TestRenderBinaryEvidenceRejectsSymlinkedSidecar(t *testing.T) {
	inputDir := t.TempDir()
	venueDir := filepath.Join(inputDir, "venues", "north")
	if err := os.MkdirAll(venueDir, 0755); err != nil {
		t.Fatal(err)
	}
	eventsFile, err := os.Create(filepath.Join(inputDir, "events.evs"))
	if err != nil {
		t.Fatal(err)
	}
	sink := newNeutralBinaryEvidence(eventsFile)
	if err := sink.record(1, 7, "event", "north", map[string]int{"value": 1}, "general.jsonl", 1); err != nil {
		t.Fatal(err)
	}
	if err := sink.finish(); err != nil {
		t.Fatal(err)
	}
	if err := eventsFile.Close(); err != nil {
		t.Fatal(err)
	}
	writeRenderMetadata(t, inputDir, sink, "none")
	externalSidecar := filepath.Join(t.TempDir(), "general.jsonl")
	if err := os.WriteFile(externalSidecar, []byte("not used\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(externalSidecar, filepath.Join(venueDir, "general.jsonl")); err != nil {
		t.Fatal(err)
	}
	if _, err := RenderBinaryEvidence(inputDir, filepath.Join(t.TempDir(), "rendered")); err == nil || !strings.Contains(err.Error(), "reject symlink") {
		t.Fatalf("symlinked sidecar was accepted: %v", err)
	}
}

func TestRenderDirectoryPublicationDoesNotReplaceExistingDestination(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("renameat2 no-replacement contract is registered for linux/amd64")
	}
	parentDir := t.TempDir()
	sourceDir := filepath.Join(parentDir, "source")
	destinationDir := filepath.Join(parentDir, "destination")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destinationDir, 0755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(destinationDir, "sentinel")
	if err := os.WriteFile(sentinel, []byte("preserve"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "rendered"), []byte("source"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := renameRenderDirectoryNoReplace(sourceDir, destinationDir); err == nil {
		t.Fatal("publication replaced an existing destination")
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("existing destination was not preserved: %v", err)
	}
	if _, err := os.Stat(sourceDir); err != nil {
		t.Fatalf("source directory disappeared after rejected publication: %v", err)
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

func TestRenderBinaryEvidenceRejectsDuplicateProvenanceKeys(t *testing.T) {
	t.Run("manifest", func(t *testing.T) {
		inputDir := t.TempDir()
		manifest := []byte(`{"schema_version":2,"schema_version":2,"config":{"log_mode":"none","evidence_format":"evstream_v3"}}`)
		if err := os.WriteFile(filepath.Join(inputDir, "manifest.json"), append(manifest, '\n'), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := RenderBinaryEvidence(inputDir, filepath.Join(t.TempDir(), "rendered")); err == nil || !strings.Contains(err.Error(), "duplicate object key") {
			t.Fatalf("duplicate manifest key was accepted or misclassified: %v", err)
		}
	})

	t.Run("binary attestation", func(t *testing.T) {
		inputDir := minimalRenderInput(t, "none", nil)
		path := filepath.Join(inputDir, "binary-evidence-attestation.json")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		duplicate, err := duplicateJSONField(raw, "domain")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, duplicate, 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := RenderBinaryEvidence(inputDir, filepath.Join(t.TempDir(), "rendered")); err == nil || !strings.Contains(err.Error(), "duplicate object key") {
			t.Fatalf("duplicate binary attestation key was accepted or misclassified: %v", err)
		}
	})

	t.Run("evidence-only attestation", func(t *testing.T) {
		sidecar := []byte(`{"client_id":7,"data":{"venue_id":"north","sequence":2,"payload":{"value":1}},"event":"evidence_only","sim_ts":2}`)
		inputDir := minimalRenderInput(t, "full", sidecar)
		path := filepath.Join(inputDir, "evidence-only-artifact-hash.json")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		duplicate, err := duplicateJSONField(raw, "domain")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, duplicate, 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := RenderBinaryEvidence(inputDir, filepath.Join(t.TempDir(), "rendered")); err == nil || !strings.Contains(err.Error(), "duplicate object key") {
			t.Fatalf("duplicate evidence-only attestation key was accepted or misclassified: %v", err)
		}
	})
}

func minimalRenderInput(t *testing.T, logMode string, sidecar []byte) string {
	t.Helper()
	inputDir := t.TempDir()
	venueDir := filepath.Join(inputDir, "venues", "north")
	if err := os.MkdirAll(venueDir, 0755); err != nil {
		t.Fatal(err)
	}
	eventsFile, err := os.Create(filepath.Join(inputDir, "events.evs"))
	if err != nil {
		t.Fatal(err)
	}
	sink := newNeutralBinaryEvidence(eventsFile)
	if err := sink.record(1, 7, "event", "north", map[string]int{"value": 1}, "general.jsonl", 1); err != nil {
		t.Fatal(err)
	}
	if err := sink.finish(); err != nil {
		t.Fatal(err)
	}
	if err := eventsFile.Close(); err != nil {
		t.Fatal(err)
	}
	if logMode == "full" {
		if err := os.WriteFile(filepath.Join(venueDir, "general.jsonl"), append(sidecar, '\n'), 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeRenderMetadata(t, inputDir, sink, logMode, sidecar)
	return inputDir
}

func TestRenderBinaryEvidenceRejectsDuplicateKeysInSidecarsAndOpaquePayloads(t *testing.T) {
	duplicateSidecar := []byte(`{"client_id":8,"data":{"venue_id":"north","sequence":2,"payload":{"value":1,"value":2}},"event":"sidecar","sim_ts":2}`)
	inputDir := minimalRenderInput(t, "full", duplicateSidecar)
	if _, err := RenderBinaryEvidence(inputDir, filepath.Join(t.TempDir(), "rendered")); err == nil || !strings.Contains(err.Error(), "duplicate object key") {
		t.Fatalf("duplicate nested sidecar key was accepted or misclassified: %v", err)
	}

	inputDir = t.TempDir()
	if err := os.MkdirAll(filepath.Join(inputDir, "venues", "north"), 0755); err != nil {
		t.Fatal(err)
	}
	eventsFile, err := os.Create(filepath.Join(inputDir, "events.evs"))
	if err != nil {
		t.Fatal(err)
	}
	sink := newBinaryEvidence(eventsFile)
	opaque := exchange.OpaqueJSON{Value: json.RawMessage(`{"value":1,"value":2}`)}
	if err := sink.record(1, 7, "opaque", "north", opaque, "general.jsonl", 1); err != nil {
		t.Fatal(err)
	}
	if err := sink.finish(); err != nil {
		t.Fatal(err)
	}
	if err := eventsFile.Close(); err != nil {
		t.Fatal(err)
	}
	writeRenderMetadata(t, inputDir, sink, "none")
	if _, err := RenderBinaryEvidence(inputDir, filepath.Join(t.TempDir(), "rendered")); err == nil || !strings.Contains(err.Error(), "duplicate object key") {
		t.Fatalf("duplicate opaque payload key was accepted or misclassified: %v", err)
	}
}

func duplicateJSONField(raw []byte, field string) ([]byte, error) {
	marker := []byte(`"` + field + `"`)
	fieldStart := bytes.Index(raw, marker)
	if fieldStart < 0 {
		return nil, fmt.Errorf("JSON field %q not found", field)
	}
	lineStart := bytes.LastIndex(raw[:fieldStart], []byte{'\n'}) + 1
	lineEndOffset := bytes.IndexByte(raw[fieldStart:], '\n')
	if lineEndOffset < 0 {
		return nil, fmt.Errorf("JSON field %q is not pretty-printed", field)
	}
	lineEnd := fieldStart + lineEndOffset
	line := append([]byte(nil), raw[lineStart:lineEnd]...)
	if !bytes.HasSuffix(bytes.TrimSpace(line), []byte{','}) {
		line = append(line, ',')
	}
	result := append([]byte(nil), raw[:lineStart]...)
	result = append(result, line...)
	result = append(result, '\n')
	result = append(result, raw[lineStart:]...)
	return result, nil
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
	if attestation.Domain != "canonical_binary_execution_frames" || attestation.Ordering != "ordered_stream" ||
		attestation.Hashing != binaryGlobalExecutionHashContract || attestation.ExecutionStreamHash == "" || attestation.CanonicalExecutionStreamHash == "" {
		t.Fatalf("binary attestation = %+v", attestation)
	}
	if attestation.PersistedEventRecords == nil || attestation.FinalGlobalSequence == nil ||
		*attestation.PersistedEventRecords != 0 || *attestation.FinalGlobalSequence != attestation.EventFrames {
		t.Fatalf("binary global ordinal attestation = %+v", attestation)
	}
	if _, err := os.Stat(filepath.Join(dir, "evidence-artifact-hash.json")); !os.IsNotExist(err) {
		t.Fatalf("legacy JSON artifact exists for binary run: %v", err)
	}
}

func TestRenderBinaryEvidenceRequiresGlobalFinalOrdinalAttestation(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "missing persisted records", mutate: func(attestation map[string]any) {
			delete(attestation, "persisted_event_records")
		}},
		{name: "missing final sequence", mutate: func(attestation map[string]any) {
			delete(attestation, "final_global_sequence")
		}},
		{name: "wrong final sequence", mutate: func(attestation map[string]any) {
			attestation["final_global_sequence"] = float64(2)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			inputDir := t.TempDir()
			eventsFile, err := os.Create(filepath.Join(inputDir, "events.evs"))
			if err != nil {
				t.Fatal(err)
			}
			sink := newGlobalNeutralBinaryEvidence(eventsFile)
			if err := sink.recordGlobal(1, 7, "event", "north", map[string]int{"value": 1}, "general.jsonl", 1, 1); err != nil {
				t.Fatal(err)
			}
			if err := sink.finish(); err != nil {
				t.Fatal(err)
			}
			if err := eventsFile.Close(); err != nil {
				t.Fatal(err)
			}
			writeRenderMetadata(t, inputDir, sink, "none")
			attestationPath := filepath.Join(inputDir, "binary-evidence-attestation.json")
			attestationRaw, err := os.ReadFile(attestationPath)
			if err != nil {
				t.Fatal(err)
			}
			var attestation map[string]any
			if err := json.Unmarshal(attestationRaw, &attestation); err != nil {
				t.Fatal(err)
			}
			test.mutate(attestation)
			mutated, err := json.MarshalIndent(attestation, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(attestationPath, append(mutated, '\n'), 0644); err != nil {
				t.Fatal(err)
			}
			if _, err := RenderBinaryEvidence(inputDir, filepath.Join(t.TempDir(), "rendered")); err == nil || !strings.Contains(err.Error(), "final evidence ordinal") && !strings.Contains(err.Error(), "final global evidence sequence") {
				t.Fatalf("invalid global ordinal attestation was accepted: %v", err)
			}
		})
	}
}

func TestRenderSidecarRequiresMandatoryFieldsAndExactTypes(t *testing.T) {
	base := `{"client_id":7,"data":{"venue_id":"north","sequence":1,"payload":{"value":1}},"event":"sidecar","sim_ts":1,"event_seq":1}`
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "missing client", mutate: func(object map[string]any) { delete(object, "client_id") }},
		{name: "missing event", mutate: func(object map[string]any) { delete(object, "event") }},
		{name: "missing timestamp", mutate: func(object map[string]any) { delete(object, "sim_ts") }},
		{name: "missing venue", mutate: func(object map[string]any) { delete(object["data"].(map[string]any), "venue_id") }},
		{name: "missing sequence", mutate: func(object map[string]any) { delete(object["data"].(map[string]any), "sequence") }},
		{name: "missing payload", mutate: func(object map[string]any) { delete(object["data"].(map[string]any), "payload") }},
		{name: "client is string", mutate: func(object map[string]any) { object["client_id"] = "7" }},
		{name: "timestamp is fractional", mutate: func(object map[string]any) { object["sim_ts"] = 1.5 }},
		{name: "sequence is string", mutate: func(object map[string]any) { object["data"].(map[string]any)["sequence"] = "1" }},
		{name: "event sequence is string", mutate: func(object map[string]any) { object["event_seq"] = "1" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var object map[string]any
			if err := json.Unmarshal([]byte(base), &object); err != nil {
				t.Fatal(err)
			}
			test.mutate(object)
			raw, err := json.Marshal(object)
			if err != nil {
				t.Fatal(err)
			}
			var event renderPersistedEvent
			if err := unmarshalRenderSidecar(raw, &event); err == nil {
				t.Fatal("malformed sidecar was accepted")
			}
		})
	}
}

func TestRenderGlobalSidecarRejectsNullPayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "general.jsonl")
	raw := []byte(`{"client_id":7,"data":{"venue_id":"north","sequence":1,"payload":null},"event":"sidecar","sim_ts":1,"event_seq":1}`)
	if err := os.WriteFile(path, append(raw, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	cursor := &renderSidecarCursor{
		key:     renderRouteKey{venue: "north", route: "general.jsonl"},
		file:    file,
		scanner: bufio.NewScanner(file),
	}
	var digest renderArtifactDigest
	if err := cursor.advance(&digest, true); err == nil || !strings.Contains(err.Error(), "null payload") {
		t.Fatalf("global null payload was accepted: %v", err)
	}
}

func TestConfigRejectsUnknownEvidenceFormat(t *testing.T) {
	_, err := NewSim(time.Second, Config{LogDir: t.TempDir(), LogMode: "none", EvidenceFormat: "future"})
	if err == nil {
		t.Fatal("unknown evidence format accepted")
	}
}

func TestBinaryEvidenceProductionPathRunsAndRenders(t *testing.T) {
	inputDir := filepath.Join(t.TempDir(), "run")
	sim, err := NewSim(3*time.Second, Config{
		LogDir: inputDir, LogMode: "full", EvidenceFormat: binaryRepresentation,
		CheckpointIntervalSeconds: 1, Seed: 101,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sim.Run(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}
	if err := sim.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	outDir := filepath.Join(t.TempDir(), "rendered")
	report, err := RenderBinaryEvidence(inputDir, outDir)
	if err != nil {
		t.Fatalf("render production path: %v", err)
	}
	if report.EventFrames == 0 || report.Routes == 0 || report.ExecutionHash == "" || report.FullEvidenceHash == "" {
		t.Fatalf("production render report = %+v", report)
	}
}

func TestBinaryEvidenceExecutionStreamIsLogModeNeutral(t *testing.T) {
	root := t.TempDir()
	for _, logMode := range []string{"full", "none"} {
		dir := filepath.Join(root, logMode)
		sim, err := NewSim(3*time.Second, Config{
			LogDir: dir, LogMode: logMode, EvidenceFormat: binaryRepresentation,
			CheckpointIntervalSeconds: 1, Seed: 101,
		})
		if err != nil {
			t.Fatalf("create %s simulation: %v", logMode, err)
		}
		if err := sim.Run(context.Background()); err != nil {
			t.Fatalf("run %s simulation: %v", logMode, err)
		}
		if err := sim.Close(); err != nil {
			t.Fatalf("close %s simulation: %v", logMode, err)
		}
	}
	fullReport, err := RenderBinaryEvidence(filepath.Join(root, "full"), filepath.Join(t.TempDir(), "full-rendered"))
	if err != nil {
		t.Fatalf("render full evidence: %v", err)
	}
	noneReport, err := RenderBinaryEvidence(filepath.Join(root, "none"), filepath.Join(t.TempDir(), "none-rendered"))
	if err != nil {
		t.Fatalf("render none evidence: %v", err)
	}
	if fullReport.ExecutionHash != noneReport.ExecutionHash {
		t.Fatalf("binary execution hash changed when JSON sidecars were disabled: full=%s none=%s", fullReport.ExecutionHash, noneReport.ExecutionHash)
	}
}

func TestRenderBinaryEvidenceRejectsRouteSequenceSwapWithValidNormalizedHash(t *testing.T) {
	inputDir := t.TempDir()
	eventsPath := filepath.Join(inputDir, "events.evs")
	eventsFile, err := os.Create(eventsPath)
	if err != nil {
		t.Fatal(err)
	}
	sink := newNeutralBinaryEvidence(eventsFile)
	for index := uint64(1); index <= 2; index++ {
		if err := sink.record(int64(index), 7, "event", "north", map[string]int{"value": int(index)}, "general.jsonl", index); err != nil {
			t.Fatal(err)
		}
	}
	if err := sink.finish(); err != nil {
		t.Fatal(err)
	}
	if err := eventsFile.Close(); err != nil {
		t.Fatal(err)
	}
	writeRenderMetadata(t, inputDir, sink, "none")
	if err := swapRouteSequencesAndRepairCRC(eventsPath); err != nil {
		t.Fatal(err)
	}
	if _, err := RenderBinaryEvidence(inputDir, filepath.Join(t.TempDir(), "rendered")); err == nil || !strings.Contains(err.Error(), "canonical reconstruction stream") {
		t.Fatalf("route-sequence mutation was accepted: %v", err)
	}
}

func swapRouteSequencesAndRepairCRC(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var frameOffsets []int
	offset := evstream.StreamHeaderSize
	for offset+evstream.BlockHeaderSize <= len(raw) {
		magic := binary.LittleEndian.Uint32(raw[offset : offset+4])
		if magic == evstream.TrailerMagic {
			break
		}
		if magic != evstream.BlockMagic {
			return fmt.Errorf("unexpected stream block magic at %d", offset)
		}
		uncompressedLength := int(binary.LittleEndian.Uint32(raw[offset+4 : offset+8]))
		storedLength := int(binary.LittleEndian.Uint32(raw[offset+8 : offset+12]))
		if storedLength != uncompressedLength || offset+evstream.BlockHeaderSize+storedLength > len(raw) {
			return fmt.Errorf("unexpected block layout at %d", offset)
		}
		blockStart := offset + evstream.BlockHeaderSize
		blockEnd := blockStart + uncompressedLength
		for frameOffset := blockStart; frameOffset < blockEnd; {
			header, err := evstream.ParseFrameHeader(raw[frameOffset:blockEnd])
			if err != nil {
				return err
			}
			frameEnd := frameOffset + int(header.Length)
			if frameEnd > blockEnd {
				return fmt.Errorf("frame overruns block")
			}
			if header.SchemaID != evstream.SchemaDictionary && frameEnd-frameOffset >= evstream.FrameHeaderSize+16 {
				frameOffsets = append(frameOffsets, frameOffset+evstream.FrameHeaderSize+8)
			}
			frameOffset = frameEnd
		}
		crc := crc32.Checksum(raw[blockStart:blockEnd], crc32.MakeTable(crc32.Castagnoli))
		binary.LittleEndian.PutUint32(raw[offset+16:offset+20], crc)
		offset = blockEnd
	}
	if len(frameOffsets) != 2 {
		return fmt.Errorf("found %d event frames, want 2", len(frameOffsets))
	}
	firstSequence := append([]byte(nil), raw[frameOffsets[0]:frameOffsets[0]+8]...)
	copy(raw[frameOffsets[0]:frameOffsets[0]+8], raw[frameOffsets[1]:frameOffsets[1]+8])
	copy(raw[frameOffsets[1]:frameOffsets[1]+8], firstSequence)
	// Recompute CRCs after the payload mutation. There is one block for this
	// fixture, but scanning keeps the helper correct if the writer changes its
	// default block target later.
	offset = evstream.StreamHeaderSize
	for offset+evstream.BlockHeaderSize <= len(raw) {
		if binary.LittleEndian.Uint32(raw[offset:offset+4]) == evstream.TrailerMagic {
			break
		}
		length := int(binary.LittleEndian.Uint32(raw[offset+4 : offset+8]))
		blockStart := offset + evstream.BlockHeaderSize
		blockEnd := blockStart + length
		crc := crc32.Checksum(raw[blockStart:blockEnd], crc32.MakeTable(crc32.Castagnoli))
		binary.LittleEndian.PutUint32(raw[offset+16:offset+20], crc)
		offset = blockEnd
	}
	return os.WriteFile(path, raw, 0644)
}

func TestBinaryEvidenceDifferentiallyPreservesScientificJSONPayloads(t *testing.T) {
	inputDir := filepath.Join(t.TempDir(), "binary")
	if err := os.MkdirAll(filepath.Join(inputDir, "venues", "north"), 0755); err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{"schema_version":2,"config":{"log_mode":"none","evidence_format":"evstream_v3"}}`)
	if err := os.WriteFile(filepath.Join(inputDir, "manifest.json"), append(manifest, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	legacyDir := t.TempDir()
	legacyLogger, err := feesim.NewJSONLinesLogger(filepath.Join(legacyDir, "general.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	eventsFile, err := os.Create(filepath.Join(inputDir, "events.evs"))
	if err != nil {
		t.Fatal(err)
	}
	binarySink := newBinaryEvidence(eventsFile)
	cases := []struct {
		name    string
		simTime int64
		client  uint64
		payload any
	}{
		{name: "opaque", simTime: 1, client: 7, payload: map[string]any{"z": 3, "a": "one"}},
		{name: "balance", simTime: 2, client: 8, payload: etypes.BalanceChangeEvent{Timestamp: 2, ClientID: 8, Reason: "fill", Changes: []etypes.BalanceDelta{{Asset: "USD", Wallet: "spot", Delta: 4}}}},
		{name: "fee", simTime: 3, client: 0, payload: etypes.FeeRevenueEvent{Timestamp: 3, Symbol: "ABC/USD", TradeID: 9, TakerFee: 2, MakerFee: 1, Asset: "USD"}},
		{name: "trade", simTime: 4, client: 0, payload: etypes.Trade{TradeID: 10, Price: 101, Qty: 2, Side: etypes.Buy, TakerOrderID: 11, MakerOrderID: 12}},
		{name: "venue_balance", simTime: 5, client: 0, payload: exchange.VenueBalanceEvent{Timestamp: 5, Sequence: 13, TradeID: 14, Bucket: exchange.VenueFeeRevenue, Asset: "USD", Reason: "fee", Symbol: "ABC/USD", OldBalance: 1, NewBalance: 3, Delta: 2}},
	}
	for index, testCase := range cases {
		eventName := "event_" + testCase.name
		legacyLogger.LogEvent(testCase.simTime, testCase.client, eventName, venueLogEvent{VenueID: "north", Payload: testCase.payload})
		if err := binarySink.record(testCase.simTime, testCase.client, eventName, "north", testCase.payload, "general.jsonl", uint64(index+1)); err != nil {
			t.Fatal(err)
		}
	}
	if err := legacyLogger.Close(); err != nil {
		t.Fatal(err)
	}
	if err := binarySink.finish(); err != nil {
		t.Fatal(err)
	}
	if err := eventsFile.Close(); err != nil {
		t.Fatal(err)
	}
	digest := binarySink.executionHash()
	attestation, err := json.MarshalIndent(binaryEvidenceArtifactRecord{
		Domain: "canonical_binary_execution_frames", Ordering: "ordered_stream",
		EventFrames: binarySink.count(), StreamFrames: binarySink.writer.Count(),
		ExecutionStreamHash: hex.EncodeToString(digest[:]), UnencodablePayloads: binarySink.unencodableCount(),
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "binary-evidence-attestation.json"), append(attestation, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(t.TempDir(), "rendered")
	if _, err := RenderBinaryEvidence(inputDir, outDir); err != nil {
		t.Fatal(err)
	}
	legacyRaw, err := os.ReadFile(filepath.Join(legacyDir, "general.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	renderedRaw, err := os.ReadFile(filepath.Join(outDir, "venues", "north", "general.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var legacyRecords, renderedRecords []struct {
		ClientID uint64          `json:"client_id"`
		Data     json.RawMessage `json:"data"`
		Event    string          `json:"event"`
		SimTS    int64           `json:"sim_ts"`
	}
	if err := decodeJSONLines(legacyRaw, &legacyRecords); err != nil {
		t.Fatal(err)
	}
	if err := decodeJSONLines(renderedRaw, &renderedRecords); err != nil {
		t.Fatal(err)
	}
	if len(legacyRecords) != len(renderedRecords) {
		t.Fatalf("legacy records=%d rendered records=%d", len(legacyRecords), len(renderedRecords))
	}
	for index := range legacyRecords {
		var oldData, newData struct {
			VenueID string          `json:"venue_id"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(legacyRecords[index].Data, &oldData); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(renderedRecords[index].Data, &newData); err != nil {
			t.Fatal(err)
		}
		if legacyRecords[index].ClientID != renderedRecords[index].ClientID || legacyRecords[index].Event != renderedRecords[index].Event || legacyRecords[index].SimTS != renderedRecords[index].SimTS || oldData.VenueID != newData.VenueID || !bytes.Equal(oldData.Payload, newData.Payload) {
			t.Fatalf("record %d differs: old=%s new=%s", index, legacyRecords[index].Data, renderedRecords[index].Data)
		}
	}
}

func decodeJSONLines(raw []byte, target any) error {
	lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
	encoded := make([][]byte, len(lines))
	for index, line := range lines {
		encoded[index] = line
	}
	return json.Unmarshal([]byte("["+string(bytes.Join(encoded, []byte(",")))+"]"), target)
}

func writeRenderMetadata(t *testing.T, inputDir string, sink *binaryEvidence, logMode string, sidecarRecords ...[]byte) {
	t.Helper()
	manifest := []byte(`{"schema_version":2,"config":{"log_mode":"` + logMode + `","evidence_format":"evstream_v3"}}`)
	if err := os.WriteFile(filepath.Join(inputDir, "manifest.json"), append(manifest, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	var sidecarDigest renderArtifactDigest
	for _, record := range sidecarRecords {
		sidecarDigest.add(record)
	}
	digest := sink.executionHash()
	rawDigest := sink.rawExecutionHash()
	binaryArtifact := binaryEvidenceArtifactRecord{
		Domain: "canonical_binary_execution_frames", Ordering: "ordered_stream",
		Hashing:     sink.hashing,
		EventFrames: sink.count(), StreamFrames: sink.writer.Count(),
		ExecutionStreamHash: hex.EncodeToString(digest[:]), CanonicalExecutionStreamHash: hex.EncodeToString(rawDigest[:]), UnencodablePayloads: sink.unencodableCount(),
	}
	if sink.hashing == binaryGlobalExecutionHashContract {
		persistedRecords := uint64(sidecarDigest.events)
		finalGlobalSequence := binaryArtifact.EventFrames + persistedRecords
		binaryArtifact.PersistedEventRecords = &persistedRecords
		binaryArtifact.FinalGlobalSequence = &finalGlobalSequence
	}
	attestation, err := json.MarshalIndent(binaryArtifact, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "binary-evidence-attestation.json"), append(attestation, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	if logMode == "full" {
		artifact, err := json.MarshalIndent(evidenceArtifactRecord{
			Domain: "persisted_json_log_evidence_only", Ordering: "unordered_multiset",
			Events: sidecarDigest.events, Digest: sidecarDigest.hex(),
		}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(inputDir, "evidence-only-artifact-hash.json"), append(artifact, '\n'), 0644); err != nil {
			t.Fatal(err)
		}
	}
}
