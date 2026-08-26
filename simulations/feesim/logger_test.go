package feesim

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"
)

type injectedWriteCloser struct {
	bytes.Buffer
	write func([]byte) (int, error)
	close error
}

func (w *injectedWriteCloser) Write(p []byte) (int, error) {
	if w.write != nil {
		return w.write(p)
	}
	return w.Buffer.Write(p)
}

func (w *injectedWriteCloser) Close() error { return w.close }

func TestPersistedEventPreservesLegacyCanonicalBytes(t *testing.T) {
	tests := []struct {
		name   string
		time   int64
		client uint64
		event  string
		data   any
		want   string
	}{
		{
			name: "map payload", time: 1, client: 7, event: "one", data: map[string]int{"n": 1},
			want: `{"client_id":7,"data":{"n":1},"event":"one","sim_ts":1}`,
		},
		{
			name: "struct payload", time: -2, client: 9, event: "fill", data: struct {
				Price int64 `json:"price"`
				Qty   int64 `json:"qty"`
			}{Price: 50_000, Qty: 3},
			want: `{"client_id":9,"data":{"price":50000,"qty":3},"event":"fill","sim_ts":-2}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := json.Marshal(persistedEvent{ClientID: test.client, Data: test.data, Event: test.event, SimTS: test.time})
			if err != nil {
				t.Fatal(err)
			}
			legacy, err := json.Marshal(map[string]any{"sim_ts": test.time, "client_id": test.client, "event": test.event, "data": test.data})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, legacy) || string(got) != test.want {
				t.Fatalf("persisted bytes changed: got %s legacy %s want %s", got, legacy, test.want)
			}
		})
	}
}

func BenchmarkPersistedEventEnvelope(b *testing.B) {
	payload := map[string]any{"price": int64(50_000), "qty": int64(3), "side": "BUY"}
	b.Run("legacy_map", func(b *testing.B) {
		for range b.N {
			if _, err := json.Marshal(map[string]any{"sim_ts": int64(1), "client_id": uint64(7), "event": "fill", "data": payload}); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("struct_envelope", func(b *testing.B) {
		for range b.N {
			if _, err := json.Marshal(persistedEvent{ClientID: 7, Data: payload, Event: "fill", SimTS: 1}); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestJSONLinesLoggerEvidenceDigestCountsPersistedRecordsOnce(t *testing.T) {
	path := t.TempDir() + "/evidence.jsonl"
	logger, err := NewJSONLinesLogger(path)
	if err != nil {
		t.Fatal(err)
	}
	logger.LogEvent(1, 7, "one", map[string]int{"n": 1})
	logger.LogEvent(2, 8, "two", map[string]int{"n": 2})
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := logger.EvidenceDigest()
	if err != nil {
		t.Fatal(err)
	}
	if got.Events != 2 {
		t.Fatalf("runtime record count = %d, want 2", got.Events)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var limbs [4]uint64
	scanner := bufio.NewScanner(file)
	count := int64(0)
	for scanner.Scan() {
		hash := sha256.Sum256(scanner.Bytes())
		var carry uint64
		for i := 3; i >= 0; i-- {
			limb := binary.BigEndian.Uint64(hash[i*8 : i*8+8])
			sum := limbs[i] + limb
			newCarry := uint64(0)
			if sum < limbs[i] {
				newCarry = 1
			}
			sum += carry
			if sum < carry {
				newCarry = 1
			}
			limbs[i] = sum
			carry = newCarry
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	var raw [32]byte
	for i := range limbs {
		binary.BigEndian.PutUint64(raw[i*8:i*8+8], limbs[i])
	}
	want := hex.EncodeToString(raw[:])
	if count != got.Events || want != got.Hex() {
		t.Fatalf("offline evidence %d/%s, runtime %d/%s", count, want, got.Events, got.Hex())
	}
}

func TestJSONLinesLoggerFailsClosedOnWriteFlushAndCloseErrors(t *testing.T) {
	writeErr := errors.New("injected write failure")
	flushErr := errors.New("injected flush failure")
	closeErr := errors.New("injected close failure")
	tests := []struct {
		name   string
		logger *JSONLinesLogger
		want   error
		flush  bool
	}{
		{
			name: "write",
			logger: newJSONLinesLogger(&injectedWriteCloser{write: func([]byte) (int, error) {
				return 0, writeErr
			}}, 1),
			want: writeErr,
		},
		{
			name: "flush",
			logger: newJSONLinesLogger(&injectedWriteCloser{write: func([]byte) (int, error) {
				return 0, flushErr
			}}, 1024),
			want:  flushErr,
			flush: true,
		},
		{
			name:   "close",
			logger: newJSONLinesLogger(&injectedWriteCloser{close: closeErr}, 1024),
			want:   closeErr,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.logger.LogEvent(1, 1, "event", map[string]int{"n": 1})
			if test.flush {
				if err := test.logger.Flush(); !errors.Is(err, test.want) {
					t.Fatalf("Flush error = %v, want %v", err, test.want)
				}
			}
			if err := test.logger.Close(); !errors.Is(err, test.want) {
				t.Fatalf("Close error = %v, want %v", err, test.want)
			}
			if _, err := test.logger.EvidenceDigest(); !errors.Is(err, test.want) {
				t.Fatalf("EvidenceDigest error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestJSONLinesLoggerRejectsPartialFinalEventWrite(t *testing.T) {
	partialErr := errors.New("injected partial write")
	logger := newJSONLinesLogger(&injectedWriteCloser{write: func(p []byte) (int, error) {
		return len(p) - 1, partialErr
	}}, 1)
	logger.LogEvent(1, 1, "event", map[string]int{"n": 1})
	if err := logger.Close(); !errors.Is(err, partialErr) {
		t.Fatalf("Close error = %v, want %v", err, partialErr)
	}
	if digest, err := logger.EvidenceDigest(); !errors.Is(err, partialErr) || digest.Events != 0 {
		t.Fatalf("partial record produced digest=%+v err=%v", digest, err)
	}
}

func TestSimCloseReportsEvidenceFailureAndClosesEveryLogger(t *testing.T) {
	writeErr := errors.New("injected write failure")
	failed := newJSONLinesLogger(&injectedWriteCloser{write: func([]byte) (int, error) {
		return 0, writeErr
	}}, 1)
	closed := newJSONLinesLogger(&injectedWriteCloser{}, 1024)
	failed.LogEvent(1, 1, "failed", nil)
	closed.LogEvent(1, 1, "surviving", nil)

	sim := &Sim{Loggers: []*JSONLinesLogger{failed, closed}}
	if err := sim.Close(); !errors.Is(err, writeErr) {
		t.Fatalf("Close error = %v, want %v", err, writeErr)
	}
	if _, err := closed.EvidenceDigest(); err != nil {
		t.Fatalf("healthy logger was not closed cleanly: %v", err)
	}
	if err := sim.Close(); !errors.Is(err, writeErr) {
		t.Fatalf("repeat Close error = %v, want %v", err, writeErr)
	}
}

var _ io.WriteCloser = (*injectedWriteCloser)(nil)
