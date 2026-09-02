package analysis

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func writeCompressedRoute(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	encoder, err := zstd.NewWriter(file, zstd.WithEncoderConcurrency(1))
	if err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if _, err := encoder.Write(content); err != nil {
		_ = encoder.Close()
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
}

func TestMeasurePriceUnavailableOrderRejectionsReadsCompressedRenderedRoute(t *testing.T) {
	path := filepath.Join(t.TempDir(), "venues", "central", "general.jsonl.zst")
	content := []byte(
		`{"sim_ts":1,"client_id":7,"event":"OrderRejected","data":{"venue_id":"central","sequence":1,"payload":{"error":"PRICE_UNAVAILABLE","symbol":"CDF/USD"}}}` + "\n" +
			`{"sim_ts":2,"client_id":8,"event":"OrderRejected","data":{"venue_id":"central","sequence":2,"payload":{"error":"INVALID_PRICE","symbol":"CDF/USD"}}}` + "\n")
	writeCompressedRoute(t, path, content)

	result, err := (&Run{files: []string{path}}).MeasurePriceUnavailableOrderRejections()
	if err != nil {
		t.Fatalf("measure price-unavailable rejections: %v", err)
	}
	if !result.Valid || result.OrderRejectedCount != 2 || result.PriceUnavailableOrderRejections != 1 || result.MalformedOrderRejectedCount != 0 {
		t.Fatalf("unexpected audit: %+v", result)
	}
}

func TestMeasurePriceUnavailableOrderRejectionsFailsClosedOnMissingError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "venues", "central", "general.jsonl")
	content := []byte(`{"sim_ts":1,"client_id":7,"event":"OrderRejected","data":{"venue_id":"central","sequence":1,"payload":{"symbol":"CDF/USD"}}}` + "\n")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := (&Run{files: []string{path}}).MeasurePriceUnavailableOrderRejections()
	if err != nil {
		t.Fatalf("measure malformed rejection: %v", err)
	}
	if result.Valid || result.OrderRejectedCount != 1 || result.MalformedOrderRejectedCount != 1 {
		t.Fatalf("missing error field was not rejected: %+v", result)
	}
}
