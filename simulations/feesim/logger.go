package feesim

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	f        *os.File
	evidence EvidenceDigest
}

func NewJSONLinesLogger(path string) (*JSONLinesLogger, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &JSONLinesLogger{f: f, w: bufio.NewWriterSize(f, 64*1024)}, nil
}

func (l *JSONLinesLogger) LogEvent(simTime int64, clientID uint64, eventName string, event any) {
	b, err := json.Marshal(map[string]any{
		"sim_ts":    simTime,
		"client_id": clientID,
		"event":     eventName,
		"data":      event,
	})
	if err != nil {
		panic(fmt.Sprintf("feesim: marshal persisted event %q: %v", eventName, err))
	}
	l.mu.Lock()
	l.w.Write(b)
	l.w.WriteByte('\n')
	l.evidence.addRecord(b)
	l.mu.Unlock()
}

// EvidenceDigest returns a snapshot of the records that have been accepted
// for persistence by this logger. It remains available after Close.
func (l *JSONLinesLogger) EvidenceDigest() EvidenceDigest {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.evidence
}

func (l *JSONLinesLogger) Flush() {
	l.mu.Lock()
	l.w.Flush()
	l.mu.Unlock()
}

func (l *JSONLinesLogger) Close() {
	l.Flush()
	l.f.Close()
}
