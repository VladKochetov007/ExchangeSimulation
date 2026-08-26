package feesim

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

// EvidenceDigest identifies the exact set of JSON records persisted by one or
// more JSONLinesLoggers. Logs are partitioned across writers, so there is no
// model-defined global file-write order. Each record is hashed and those
// hashes are added as a 256-bit integer; this makes the digest independent of
// writer interleaving while retaining multiplicity (unlike XOR).
//
// This is deliberately an artifact digest, not an execution-stream hash.
// Execution ordering is recorded separately by multivenue's checkpoint sink.
type EvidenceDigest struct {
	Events int64
	limbs  [4]uint64
}

func (d *EvidenceDigest) addRecord(record []byte) {
	hash := sha256.Sum256(record)
	var carry uint64
	for i := 3; i >= 0; i-- {
		limb := binary.BigEndian.Uint64(hash[i*8 : i*8+8])
		sum := d.limbs[i] + limb
		newCarry := uint64(0)
		if sum < d.limbs[i] {
			newCarry = 1
		}
		sum += carry
		if sum < carry {
			newCarry = 1
		}
		d.limbs[i] = sum
		carry = newCarry
	}
	d.Events++
}

// Add combines independently accumulated logger digests.
func (d *EvidenceDigest) Add(other EvidenceDigest) {
	var carry uint64
	for i := 3; i >= 0; i-- {
		sum := d.limbs[i] + other.limbs[i]
		newCarry := uint64(0)
		if sum < d.limbs[i] {
			newCarry = 1
		}
		sum += carry
		if sum < carry {
			newCarry = 1
		}
		d.limbs[i] = sum
		carry = newCarry
	}
	d.Events += other.Events
}

// Hex returns the digest in the same big-endian form used by offline
// evidence-artifact validation.
func (d EvidenceDigest) Hex() string {
	var out [32]byte
	for i := range d.limbs {
		binary.BigEndian.PutUint64(out[i*8:i*8+8], d.limbs[i])
	}
	return hex.EncodeToString(out[:])
}

type JSONLinesLogger struct {
	mu       sync.Mutex
	w        *bufio.Writer
	f        io.WriteCloser
	evidence EvidenceDigest
	err      error
	closed   bool
}

// persistedEvent preserves the exact key order produced by encoding/json for
// the former map[string]any envelope (lexicographic: client_id, data, event,
// sim_ts) without allocating and sorting a four-entry map per log record.
// Its literal byte stream is part of the evidence-artifact contract.
type persistedEvent struct {
	ClientID uint64 `json:"client_id"`
	Data     any    `json:"data"`
	Event    string `json:"event"`
	SimTS    int64  `json:"sim_ts"`
}

func NewJSONLinesLogger(path string) (*JSONLinesLogger, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return newJSONLinesLogger(f, 64*1024), nil
}

// newJSONLinesLogger is kept separate from path creation so transport failures
// can be exercised without filesystem timing or capacity assumptions.
func newJSONLinesLogger(f io.WriteCloser, bufferSize int) *JSONLinesLogger {
	return &JSONLinesLogger{f: f, w: bufio.NewWriterSize(f, bufferSize)}
}

func (l *JSONLinesLogger) LogEvent(simTime int64, clientID uint64, eventName string, event any) {
	b, err := json.Marshal(persistedEvent{ClientID: clientID, Data: event, Event: eventName, SimTS: simTime})
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err != nil || l.closed {
		return
	}
	if err != nil {
		l.fail(fmt.Errorf("marshal persisted event %q: %w", eventName, err))
		return
	}
	if n, err := l.w.Write(b); err != nil {
		l.fail(fmt.Errorf("write persisted event %q: %w", eventName, err))
		return
	} else if n != len(b) {
		l.fail(fmt.Errorf("write persisted event %q: %w", eventName, io.ErrShortWrite))
		return
	}
	if err := l.w.WriteByte('\n'); err != nil {
		l.fail(fmt.Errorf("write persisted event newline %q: %w", eventName, err))
		return
	}
	l.evidence.addRecord(b)
}

func (l *JSONLinesLogger) fail(err error) {
	if err != nil {
		l.err = errors.Join(l.err, err)
	}
}

// EvidenceDigest returns the records accepted for persistence only when no
// transport failure has been observed. A caller must close all loggers before
// treating this as an evidence artifact.
func (l *JSONLinesLogger) EvidenceDigest() (EvidenceDigest, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err != nil {
		return EvidenceDigest{}, l.err
	}
	return l.evidence, nil
}

// Flush persists all buffered bytes and retains any transport failure for
// Close, so callers that only have a Close boundary cannot lose it.
func (l *JSONLinesLogger) Flush() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return l.err
	}
	if err := l.w.Flush(); err != nil {
		l.fail(fmt.Errorf("flush persisted events: %w", err))
	}
	return l.err
}

// Close flushes and closes even after an earlier write failure. It reports all
// observed transport errors so a failed evidence stream cannot look complete.
func (l *JSONLinesLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return l.err
	}
	l.closed = true
	if err := l.w.Flush(); err != nil {
		l.fail(fmt.Errorf("flush persisted events: %w", err))
	}
	if err := l.f.Close(); err != nil {
		l.fail(fmt.Errorf("close persisted events: %w", err))
	}
	return l.err
}

// CloseLoggers closes every required evidence logger before returning its
// aggregate digest. Any transport error returns a zero digest, which prevents
// callers from emitting a success attestation for partial raw evidence.
func CloseLoggers(loggers []*JSONLinesLogger) (EvidenceDigest, error) {
	var closeErr error
	for _, logger := range loggers {
		if logger == nil {
			continue
		}
		if err := logger.Close(); err != nil {
			closeErr = errors.Join(closeErr, err)
		}
	}
	if closeErr != nil {
		return EvidenceDigest{}, closeErr
	}
	var evidence EvidenceDigest
	for _, logger := range loggers {
		if logger == nil {
			continue
		}
		digest, err := logger.EvidenceDigest()
		if err != nil {
			return EvidenceDigest{}, err
		}
		evidence.Add(digest)
	}
	return evidence, nil
}
