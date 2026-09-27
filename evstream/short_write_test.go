package evstream_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"exchange_sim/evstream"
)

type shortWriteAtCall struct {
	output bytes.Buffer
	calls  int
	short  int
}

func (writer *shortWriteAtCall) Write(data []byte) (int, error) {
	writer.calls++
	if writer.calls == writer.short {
		if len(data) == 0 {
			return 0, nil
		}
		return writer.output.Write(data[:len(data)-1])
	}
	return writer.output.Write(data)
}

func TestWriterRejectsShortWritesAtEveryStreamBoundary(t *testing.T) {
	for _, test := range []struct {
		name      string
		shortCall int
	}{
		{"stream-header", 1},
		{"block-header", 2},
		{"block-payload", 3},
		{"completion-trailer", 4},
	} {
		t.Run(test.name, func(t *testing.T) {
			output := &shortWriteAtCall{short: test.shortCall}
			writer := evstream.NewWriter(output, evstream.WriterOptions{BlockBytes: 1})
			appendErr := writer.Append(1, 1, 0, corruptionProbe{value: 7})
			if test.shortCall < 4 && !errors.Is(appendErr, io.ErrShortWrite) {
				t.Fatalf("short append write accepted: %v", appendErr)
			}
			if test.shortCall == 4 && appendErr != nil {
				t.Fatalf("trailer-targeted append failed early: %v", appendErr)
			}
			if err := writer.Close(); !errors.Is(err, io.ErrShortWrite) {
				t.Fatalf("short write produced a successful seal: %v", err)
			}
			if output.calls != test.shortCall {
				t.Fatalf("writer continued after failed call: %d, want %d", output.calls, test.shortCall)
			}
		})
	}
}
