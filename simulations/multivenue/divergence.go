package multivenue

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"exchange_sim/census"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// Divergence locator.
//
// Comparing two runs by their terminal digest says only that they differ. It
// does not say where, and a 24h run is a hundred million events, so bisecting
// by re-running whole horizons is guesswork that costs half an hour a guess.
//
// This sink sits on the path every venue event already takes and keeps a
// rolling hash in execution order. At each simulated-time boundary it writes
// one line: the instant, how many events have been seen, and the hash of all of
// them. Two runs are then compared by their checkpoint files, which are a few
// kilobytes rather than thirty gigabytes, and the last identical checkpoint
// brackets the divergence to one interval.
//
// Once the interval is known, the same sink can dump a compact per-event trace
// for that window alone -- enough to identify the first event that differs,
// without keeping the run's logs.
//
// The hash is order-sensitive on purpose. The terminal digest used elsewhere is
// deliberately order-independent, because log writers interleave; this one runs
// on the event path itself, where arrival order is execution order, and the
// question being asked is precisely whether execution order changed.

// checkpointSink records a rolling digest and, optionally, a narrow trace.
type checkpointSink struct {
	// binary, when set, replaces the JSON encode-and-hash path entirely.
	binary          *binaryEvidence
	binaryFile      *os.File
	binaryBuf       *bufio.Writer
	binaryIndexPath string
	mu              sync.Mutex

	intervalNano int64
	checkpoints  io.WriteCloser

	traceFrom int64
	traceTo   int64
	trace     io.WriteCloser

	// hasher, sumScratch and nameScratch remove four heap allocations per
	// observed event: a fresh SHA-256 state, the slice Sum(nil) returns, and the
	// two string-to-[]byte conversions the identity writes used to make. All
	// three are touched only under mu and never outlive one call. Reset leaves
	// the hash in exactly the state New returns, and SHA-256 is a stream, so
	// concatenating the two identity writes hashes the identical byte sequence.
	hasher      hash.Hash
	sumScratch  []byte
	nameScratch []byte

	rolling          [32]byte
	events           int64
	// unencodable counts payloads the JSON encoder rejected and that were
	// recorded as a substitute.
	unencodable int64
	nextBound        int64
	lastSimTime      int64
	lastCheckpointAt int64
	finalSimTime     int64
	firstEvent       bool
	err              error
	closed           bool
}

// checkpointRecord is one line of the checkpoint file.
type checkpointRecord struct {
	// Domain and Ordering distinguish this execution-attestation record from
	// the unordered persisted-evidence digests. The old rolling_hash field is
	// deliberately retained so the locator can still read the ae13f9a baseline
	// files written before V-012 clarified the contract.
	Domain              string `json:"domain"`
	Ordering            string `json:"ordering"`
	SimTime             int64  `json:"sim_time"`
	EventCount          int64  `json:"event_count"`
	ExecutionStreamHash string `json:"execution_stream_hash"`
	Rolling             string `json:"rolling_hash"`
	// Representation names the canonical bytes the hash is taken over. It is
	// omitted for the JSON path so checkpoint files stay byte-identical to
	// every one written before the binary sink existed, and the locator keeps
	// reading them unchanged. A binary-mode hash is over different canonical
	// bytes than a JSON-mode hash of the same trajectory, so comparing the two
	// is meaningless and this field is what makes that visible rather than
	// looking like a divergence.
	Representation string `json:"representation,omitempty"`
	// Unencodable counts payloads that could not be marshalled and were
	// recorded as a substitute. Both sinks silently replaced such a payload;
	// the count makes the loss visible in the evidence rather than only in a
	// log line nobody reads.
	Unencodable int64 `json:"unencodable_payloads,omitempty"`
}

// binaryRepresentation tags a checkpoint whose hash covers canonical binary
// frames rather than marshalled JSON payloads.
const binaryRepresentation = "evstream_v1"

// traceRecord is one line of the narrow trace. Sequence is the sink's own
// counter, which is the order events actually reached the log and therefore
// the order the simulation produced them.
type traceRecord struct {
	SimTime     int64  `json:"sim_time"`
	Sequence    int64  `json:"sequence"`
	Event       string `json:"event"`
	VenueID     string `json:"venue_id"`
	ClientID    uint64 `json:"client_id"`
	Symbol      string `json:"symbol,omitempty"`
	OrderID     uint64 `json:"order_id,omitempty"`
	PayloadHash string `json:"payload_hash"`
}

// newCheckpointSink opens the sink's outputs inside the run directory. A zero
// interval disables checkpoints; an empty window disables the trace.
func newCheckpointSink(dir string, intervalSeconds int, traceFrom, traceTo int64) (*checkpointSink, error) {
	if intervalSeconds <= 0 && traceFrom >= traceTo {
		return nil, nil
	}
	sink := &checkpointSink{
		hasher:       sha256.New(),
		intervalNano: int64(intervalSeconds) * 1e9,
		traceFrom:    traceFrom,
		traceTo:      traceTo,
		firstEvent:   true,
	}
	if intervalSeconds > 0 {
		file, err := os.Create(filepath.Join(dir, "checkpoints.jsonl"))
		if err != nil {
			return nil, fmt.Errorf("multivenue: checkpoint file: %w", err)
		}
		sink.checkpoints = file
	}
	if traceFrom < traceTo {
		file, err := os.Create(filepath.Join(dir, "trace.jsonl"))
		if err != nil {
			return nil, fmt.Errorf("multivenue: trace file: %w", err)
		}
		sink.trace = file
	}
	if binaryEvidenceEnabled() {
		// The trace line carries a per-event JSON payload digest and pulls the
		// symbol and order id out of the marshalled payload. The binary path
		// produces neither, so a requested trace would silently yield an empty
		// file. Refuse the run instead: a missing diagnostic that looks present
		// is worse than one that is absent.
		if sink.trace != nil {
			sink.trace.Close()
			return nil, fmt.Errorf(
				"multivenue: execution trace is not available under EXSIM_BINARY_EVIDENCE; " +
					"unset it or drop the trace window")
		}
		// "discard" isolates encode-and-hash cost from storage. On a no-log
		// config the JSON path writes no raw evidence at all, so a binary sink
		// that writes a file would be measured against a JSON path doing no
		// I/O — an unfair comparison in the binary path's disfavour. Discarding
		// makes the two paths do the same amount of writing, which is none.
		if binaryEvidenceDiscards() {
			sink.binary = newBinaryEvidence(io.Discard)
		} else {
			file, err := os.Create(filepath.Join(dir, "events.evs"))
			if err != nil {
				return nil, fmt.Errorf("multivenue: binary evidence file: %w", err)
			}
			sink.binaryFile = file
			sink.binaryIndexPath = filepath.Join(dir, "events.evx")
			sink.binaryBuf = bufio.NewWriterSize(file, 1<<20)
			sink.binary = newBinaryEvidence(sink.binaryBuf)
		}
	}
	return sink, nil
}

// observe folds one event into the rolling digest and writes a checkpoint
// whenever simulated time crosses the next boundary.
// It returns the payload encoding it produced so the raw evidence logger can
// persist those exact bytes instead of marshalling the same payload a second
// time. The result is nil whenever the caller must not reuse it: no sink, a
// closed sink, or a payload the encoder rejected, in which case the logger runs
// its own marshal and keeps its own failure handling.
func (s *checkpointSink) observe(simTime int64, clientID uint64, eventName, venueID string, payload any) json.RawMessage {
	if s == nil {
		return nil
	}
	// Binary path: the same ordered sequence encoded as typed frames. It
	// replaces the marshal and the per-event digest entirely, so when it is
	// selected the JSON encoder is never reached.
	if s.binary != nil {
		s.binary.record(simTime, clientID, eventName, venueID, payload)
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.closed {
			return nil
		}
		s.lastSimTime = simTime
		// The event counter and the checkpoint boundary are part of the
		// attestation, not part of JSON encoding, so they run in both modes.
		// Skipping them once made binary runs write a checkpoint claiming zero
		// events and an all-zero hash, which every pair of binary runs would
		// have compared as identical.
		s.events++
		if s.firstEvent && s.intervalNano > 0 {
			s.nextBound = simTime - simTime%s.intervalNano + s.intervalNano
			s.firstEvent = false
		}
		if s.intervalNano > 0 && simTime >= s.nextBound {
			s.writeCheckpointLocked(s.nextBound)
			s.nextBound = simTime - simTime%s.intervalNano + s.intervalNano
		}
		return nil
	}

	encoded, err := json.Marshal(payload)
	reusable := encoded
	unencodable := false
	if err != nil {
		encoded = []byte(`"unencodable"`)
		reusable = nil
		unencodable = true
	}
	// Every execution event is marshalled here to feed the ordered-stream
	// digest, and the profile attributes 100 % of json.Marshal to this call.
	// The census tallies it per event name, with the encoded size as quantity,
	// so a hand-written byte-identical encoder can be aimed at the few types
	// that actually dominate rather than written for all of them.
	// Guarded at the call site, not inside CountFor: Go evaluates the
	// concatenated site name before the call, so an unguarded CountFor
	// allocates one string per event even with the census disabled. That cost
	// was live in both sinks and distorted every allocation figure taken with
	// the census off.
	if census.Enabled {
		census.CountFor("checkpoint.observe["+eventName+"]",
			"marshalled to feed the ordered execution-stream hash", false, len(encoded))
	}
	payloadDigest := sha256.Sum256(encoded)

	var scratch [8]byte

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.lastSimTime = simTime
	if unencodable {
		s.unencodable++
	}

	if s.firstEvent && s.intervalNano > 0 {
		s.nextBound = simTime - simTime%s.intervalNano + s.intervalNano
		s.firstEvent = false
	}
	// Chain: the digest after n events depends on all n and on their order.
	// Lazily created so a sink built as a struct literal, which tests do, works
	// without knowing about this field.
	if s.hasher == nil {
		s.hasher = sha256.New()
	}
	hasher := s.hasher
	hasher.Reset()
	hasher.Write(s.rolling[:])
	binary.BigEndian.PutUint64(scratch[:], uint64(simTime))
	hasher.Write(scratch[:])
	binary.BigEndian.PutUint64(scratch[:], clientID)
	hasher.Write(scratch[:])
	s.nameScratch = append(append(s.nameScratch[:0], eventName...), venueID...)
	hasher.Write(s.nameScratch)
	hasher.Write(payloadDigest[:])
	s.sumScratch = hasher.Sum(s.sumScratch[:0])
	copy(s.rolling[:], s.sumScratch)
	s.events++

	if s.trace != nil && simTime >= s.traceFrom && simTime < s.traceTo {
		record := traceRecord{
			SimTime: simTime, Sequence: s.events, Event: eventName,
			VenueID: venueID, ClientID: clientID,
			PayloadHash: hex.EncodeToString(payloadDigest[:8]),
		}
		record.Symbol, record.OrderID = identifyPayload(encoded)
		if line, err := json.Marshal(record); err == nil {
			s.writeLineLocked(s.trace, line, "write execution trace")
		} else {
			s.failLocked(fmt.Errorf("marshal execution trace: %w", err))
		}
	}

	if s.intervalNano > 0 && simTime >= s.nextBound {
		s.writeCheckpointLocked(s.nextBound)
		s.nextBound = simTime - simTime%s.intervalNano + s.intervalNano
	}
	return reusable
}

func (s *checkpointSink) writeCheckpointLocked(at int64) {
	if s.checkpoints == nil {
		return
	}
	if at <= s.lastCheckpointAt {
		return
	}
	record := checkpointRecord{
		Domain:              "execution_observations",
		Ordering:            "ordered_stream",
		SimTime:             at,
		EventCount:          s.events,
		ExecutionStreamHash: hex.EncodeToString(s.rolling[:]),
		Rolling:             hex.EncodeToString(s.rolling[:]),
	}
	record.Unencodable = s.unencodable
	if s.binary != nil {
		record.Unencodable = int64(s.binary.unencodableCount())
		digest := s.binary.executionHash()
		record.ExecutionStreamHash = hex.EncodeToString(digest[:])
		record.Rolling = record.ExecutionStreamHash
		record.Representation = binaryRepresentation
	}
	line, err := json.Marshal(record)
	if err != nil {
		s.failLocked(fmt.Errorf("marshal execution checkpoint: %w", err))
		return
	}
	s.writeLineLocked(s.checkpoints, line, "write execution checkpoint")
	if s.err == nil {
		s.lastCheckpointAt = at
	}
}

func (s *checkpointSink) writeLineLocked(w io.WriteCloser, line []byte, operation string) {
	if w == nil || s.err != nil {
		return
	}
	if n, err := w.Write(line); err != nil {
		s.failLocked(fmt.Errorf("%s: %w", operation, err))
		return
	} else if n != len(line) {
		s.failLocked(fmt.Errorf("%s: %w", operation, io.ErrShortWrite))
		return
	}
	if n, err := w.Write([]byte("\n")); err != nil {
		s.failLocked(fmt.Errorf("%s newline: %w", operation, err))
	} else if n != 1 {
		s.failLocked(fmt.Errorf("%s newline: %w", operation, io.ErrShortWrite))
	}
}

func (s *checkpointSink) failLocked(err error) {
	if err != nil {
		s.err = errors.Join(s.err, err)
	}
}

// close flushes a final checkpoint so a run that ends between boundaries is
// still comparable. It reports checkpoint/trace transport failures so callers
// cannot publish a successful execution attestation for a partial file.
func (s *checkpointSink) close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return s.err
	}
	s.closed = true
	if s.binary != nil && s.binaryFile == nil {
		if err := s.binary.flush(); err != nil {
			s.failLocked(fmt.Errorf("flush binary evidence: %w", err))
		}
	}
	if s.binary != nil && s.binaryFile != nil {
		// Flush the open block, then the buffered writer, then the file: each
		// layer holds bytes the next has not seen, and skipping one truncates
		// the stream at a block boundary that looks structurally valid.
		if err := s.binary.flush(); err != nil {
			s.failLocked(fmt.Errorf("flush binary evidence: %w", err))
		}
		if err := s.binaryBuf.Flush(); err != nil {
			s.failLocked(fmt.Errorf("flush binary evidence buffer: %w", err))
		}
		if err := s.binaryFile.Close(); err != nil {
			s.failLocked(fmt.Errorf("close binary evidence: %w", err))
		}
		s.binaryFile = nil
		// The block index is a sidecar rather than a footer, so a stream
		// truncated by a crash keeps a usable index for the blocks that landed
		// and an index can be rebuilt without rewriting the evidence.
		if err := s.binary.writeIndex(s.binaryIndexPath); err != nil {
			s.failLocked(fmt.Errorf("write binary evidence index: %w", err))
		}
	}
	if s.checkpoints != nil {
		finalAt := s.finalSimTime
		if finalAt == 0 {
			finalAt = s.lastSimTime
		}
		s.writeCheckpointLocked(finalAt)
		if err := s.checkpoints.Close(); err != nil {
			s.failLocked(fmt.Errorf("close execution checkpoints: %w", err))
		}
		s.checkpoints = nil
	}
	if s.trace != nil {
		if err := s.trace.Close(); err != nil {
			s.failLocked(fmt.Errorf("close execution trace: %w", err))
		}
		s.trace = nil
	}
	return s.err
}

// identifyPayload pulls the two fields that make a trace line readable without
// storing the payload itself.
func identifyPayload(encoded []byte) (symbol string, orderID uint64) {
	var outer struct {
		Symbol  string          `json:"symbol"`
		OrderID uint64          `json:"order_id"`
		Payload json.RawMessage `json:"payload"`
	}
	if json.Unmarshal(encoded, &outer) != nil {
		return "", 0
	}
	symbol, orderID = outer.Symbol, outer.OrderID
	if len(outer.Payload) > 0 && (symbol == "" || orderID == 0) {
		var inner struct {
			Symbol  string `json:"symbol"`
			OrderID uint64 `json:"order_id"`
		}
		if json.Unmarshal(outer.Payload, &inner) == nil {
			if symbol == "" {
				symbol = inner.Symbol
			}
			if orderID == 0 {
				orderID = inner.OrderID
			}
		}
	}
	return symbol, orderID
}
