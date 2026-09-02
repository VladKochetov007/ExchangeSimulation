package analysis

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// eventLogReader is the common sequential input adapter for routed evidence.
// Compression is deliberately below the analyzer contract: callers still see
// the same JSON records, in the same file order, with the same path identity.
type eventLogReader struct {
	file    *os.File
	decoder *zstd.Decoder
}

func (r *eventLogReader) Read(target []byte) (int, error) {
	if r.decoder != nil {
		return r.decoder.Read(target)
	}
	return r.file.Read(target)
}

func (r *eventLogReader) Close() error {
	if r.decoder != nil {
		r.decoder.Close()
	}
	return r.file.Close()
}

func openEventLog(path string) (io.ReadCloser, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(path, ".jsonl.zst") {
		return file, nil
	}
	decoder, err := zstd.NewReader(file, zstd.WithDecoderConcurrency(1))
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return &eventLogReader{file: file, decoder: decoder}, nil
}

func isEventLogPath(path string) bool {
	name := filepath.Base(path)
	return strings.HasSuffix(name, ".jsonl") || strings.HasSuffix(name, ".jsonl.zst")
}

// logicalEventLogName removes storage compression while preserving the route
// name used by the JSONL evidence contract.
func logicalEventLogName(path string) string {
	name := filepath.Base(path)
	name = strings.TrimSuffix(name, ".zst")
	return name
}
