package analysis

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestCompressedEventLogPreservesScanAndArtifactHash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "venues", "north", "spot", "ABC-USD.jsonl.zst")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	lines := []byte("{\"sim_ts\":1,\"client_id\":7,\"event\":\"Trade\",\"data\":{\"venue_id\":\"north\",\"sequence\":1,\"payload\":{\"price\":100}}}\n" +
		"{\"sim_ts\":2,\"client_id\":8,\"event\":\"OrderFill\",\"data\":{\"venue_id\":\"north\",\"sequence\":2,\"payload\":{\"symbol\":\"ABC/USD\",\"payload\":{\"qty\":3}}}}\n")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	encoder, err := zstd.NewWriter(file, zstd.WithEncoderConcurrency(1))
	if err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if _, err := encoder.Write(lines); err != nil {
		encoder.Close()
		_ = file.Close()
		t.Fatal(err)
	}
	if err := encoder.Close(); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	run := &Run{Dir: dir, files: []string{path}}
	var got []Event
	if err := run.Scan(ScanOptions{Workers: 1}, func(event Event) { got = append(got, event) }); err != nil {
		t.Fatalf("scan compressed route: %v", err)
	}
	if len(got) != 2 || got[0].Name != "Trade" || got[1].Symbol != "ABC/USD" || got[1].Sequence != 2 {
		t.Fatalf("scanned events = %+v", got)
	}
	if symbolFromPath(path) != "ABC-USD" || symbolFromSpotFile(path) != "ABC/USD" {
		t.Fatalf("compressed route symbol recovery failed: path=%q spot=%q", symbolFromPath(path), symbolFromSpotFile(path))
	}

	want := artifactSum256{}
	want.add(bytes.TrimSuffix(bytes.Split(lines, []byte("\n"))[0], []byte("\r")))
	want.add(bytes.TrimSuffix(bytes.Split(lines, []byte("\n"))[1], []byte("\r")))
	result, err := run.MeasureEvidenceArtifactHash()
	if err != nil {
		t.Fatalf("hash compressed route: %v", err)
	}
	if result.Events != 2 || result.Digest != want.hex() {
		t.Fatalf("compressed artifact hash = %+v, want events=2 digest=%s", result, want.hex())
	}
}

func TestCompressedEventLogRejectsCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.jsonl.zst")
	if err := os.WriteFile(path, []byte("not-a-zstd-stream"), 0644); err != nil {
		t.Fatal(err)
	}
	run := &Run{files: []string{path}}
	if err := run.Scan(ScanOptions{Workers: 1}, func(Event) {}); err == nil {
		t.Fatal("corrupt compressed evidence was accepted")
	}
}
