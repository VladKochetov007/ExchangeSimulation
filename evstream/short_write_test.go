package evstream_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"exchange_sim/evstream"
)

type shortWriteAtCall struct {
	output  bytes.Buffer
	calls   int
	short   int
	failure error
}

func (writer *shortWriteAtCall) Write(data []byte) (int, error) {
	writer.calls++
	if writer.calls == writer.short {
		if writer.failure != nil {
			return 0, writer.failure
		}
		if len(data) == 0 {
			return 0, nil
		}
		return writer.output.Write(data[:len(data)-1])
	}
	return writer.output.Write(data)
}

func TestWriterPreservesUnderlyingWriteError(t *testing.T) {
	want := errors.New("injected block payload write failure")
	output := &shortWriteAtCall{short: 3, failure: want}
	writer := evstream.NewWriter(output, evstream.WriterOptions{BlockBytes: 1})
	if err := writer.Append(1, 1, 0, corruptionProbe{value: 7}); !errors.Is(err, want) {
		t.Fatalf("underlying writer failure was replaced: %v", err)
	}
	if err := writer.Close(); !errors.Is(err, want) || output.calls != 3 {
		t.Fatalf("failed writer was retried or sealed: calls=%d err=%v", output.calls, err)
	}
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
