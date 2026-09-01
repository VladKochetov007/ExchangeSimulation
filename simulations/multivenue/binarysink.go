package multivenue

import (
	"crypto/sha256"
	"hash"
	"os"
	"sync"

	"exchange_sim/evstream"
	eexchange "exchange_sim/exchange"
)

const binaryExecutionHashContract = "route_sequence_neutral_v1"

// binaryEvidence is the canonical binary sink: the same ordered event sequence
// the JSON path records, encoded as typed frames instead of marshalled JSON.
//
// It is selected by EXSIM_BINARY_EVIDENCE while the two paths are being
// compared, so a single build can be measured both ways and the comparison is
// not confounded by a second binary. The gate goes away when the format is
// promoted.
//
// What it replaces is the whole of the measured cost: profiling put
// encoding/json at 14.99 % of CPU with raw logging switched off entirely, every
// byte of it produced to feed the ordered execution-stream digest, plus 3.61 %
// hashing it. A ceiling probe put the total addressable share at 17.0-17.9 %.
type binaryEvidence struct {
	mu     sync.Mutex
	writer *evstream.Writer
	// events counts frames the sink was asked to record, which is the figure
	// comparable to the JSON path's event count.
	events      uint64
	unencodable uint64
	err         error
}

// binaryEvidenceEnabled reports whether the binary sink is selected. Read once
// at construction rather than per event.
func binaryEvidenceEnabled() bool { return os.Getenv("EXSIM_BINARY_EVIDENCE") != "" }

// binaryEvidenceDiscards reports whether the stream should be thrown away
// rather than written, which isolates encode-and-hash cost from storage cost.
func binaryEvidenceDiscards() bool { return os.Getenv("EXSIM_BINARY_EVIDENCE") == "discard" }

// binaryEvidenceReplacesRawLog is reserved for the promoted successor format.
// The benchmark-only environment override keeps the prototype from silently
// changing the historical JSON evidence contract.
func binaryEvidenceReplacesRawLog() bool { return os.Getenv("EXSIM_BINARY_EVIDENCE") == "replace" }

// newBinaryEvidence starts a binary sink writing to out.
func newBinaryEvidence(out interface{ Write([]byte) (int, error) }) *binaryEvidence {
	return &binaryEvidence{writer: evstream.NewWriter(out, evstream.WriterOptions{})}
}

func newNeutralBinaryEvidence(out interface{ Write([]byte) (int, error) }) *binaryEvidence {
	return &binaryEvidence{writer: evstream.NewWriter(out, evstream.WriterOptions{
		HashFrame: hashBinaryExecutionFrame,
	})}
}

func hashBinaryExecutionFrame(digest hash.Hash, frame []byte) {
	header, err := evstream.ParseFrameHeader(frame)
	if err != nil || header.SchemaID == evstream.SchemaDictionary || len(frame) < evstream.FrameHeaderSize+16 {
		_, _ = digest.Write(frame)
		return
	}

	// The envelope sequence is the per-route ordering needed to merge the
	// binary execution frames with LogEvidenceOnly JSON sidecars. It is
	// persistence metadata, not execution state: enabling those sidecars can
	// advance it without changing the economic event sequence. Keep it in the
	// frame for exact reconstruction, but normalize it out of the execution
	// identity.
	sequenceStart := evstream.FrameHeaderSize + 8
	sequenceEnd := sequenceStart + 8
	_, _ = digest.Write(frame[:sequenceStart])
	var zeroSequence [8]byte
	_, _ = digest.Write(zeroSequence[:])
	_, _ = digest.Write(frame[sequenceEnd:])
}

// sinkEnvelope carries the event name alongside the payload.
//
// The frame header holds sequence, time, schema, venue and client, but not the
// event name, and the name cannot be inferred from the schema id: the
// per-symbol wrapper carries nine different event names under one schema. So
// every payload is prefixed with an interned name reference — four bytes,
// because the set of event names is tiny and closed.
type sinkEnvelope struct {
	routeRef uint32
	eventRef uint32
	sequence uint64
	inner    evstream.InterningAppender
}

func (e sinkEnvelope) SchemaID() uint16      { return e.inner.SchemaID() }
func (e sinkEnvelope) SchemaVersion() uint16 { return e.inner.SchemaVersion() }

func (e sinkEnvelope) AppendPayloadInterning(dst []byte, in evstream.Interner) ([]byte, error) {
	dst = evstream.AppendUint32(dst, e.routeRef)
	dst = evstream.AppendUint32(dst, e.eventRef)
	dst = evstream.AppendUint64(dst, e.sequence)
	return e.inner.AppendPayloadInterning(dst, in)
}

// record appends one event.
//
// A payload with no typed schema is wrapped as opaque JSON rather than
// refused, so the stream is complete from the first run and coverage can be
// raised one family at a time without the sink ever being partly JSON and
// partly binary at the file level.
func (b *binaryEvidence) record(simTime int64, clientID uint64, eventName, venueID string, payload any, route string, sequence uint64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return b.err
	}

	eventRef, err := b.writer.Intern(eventName)
	if err != nil {
		b.err = err
		return err
	}
	venueRef := uint32(0)
	if venueID != "" {
		if venueRef, err = b.writer.Intern(venueID); err != nil {
			b.err = err
			return err
		}
	}
	routeRef := uint32(0)
	if route != "" {
		if routeRef, err = b.writer.Intern(route); err != nil {
			b.err = err
			return err
		}
	}

	inner, typed := payload.(evstream.InterningAppender)
	if !typed {
		inner = eexchange.OpaqueJSON{Value: payload}
	}
	frame := sinkEnvelope{routeRef: routeRef, eventRef: eventRef, sequence: sequence, inner: inner}
	if err := b.writer.AppendInterning(simTime, clientID, venueRef, frame); err != nil {
		// Preserve the event slot when a payload cannot be encoded. The
		// substitute is itself canonical and keeps sequence continuity; the
		// failed payload is counted so the run cannot hide its information loss.
		frame.inner = eexchange.OpaqueJSON{Value: "unencodable"}
		if retryErr := b.writer.AppendInterning(simTime, clientID, venueRef, frame); retryErr != nil {
			b.err = retryErr
			return retryErr
		}
		b.unencodable++
	}
	b.events++
	return nil
}

// executionHash returns the digest over the canonical uncompressed frames. It
// is the binary path's counterpart to the JSON path's ordered-stream hash, and
// like it, it is independent of any storage compression applied afterwards.
func (b *binaryEvidence) executionHash() [sha256.Size]byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.writer.ExecutionHash()
}

// finish seals the stream with its completion trailer.
func (b *binaryEvidence) finish() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return b.err
	}
	return b.writer.Close()
}

// flush closes the open block so the bytes written so far are readable.
func (b *binaryEvidence) flush() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return b.err
	}
	return b.writer.Flush()
}

// count returns the number of events recorded.
func (b *binaryEvidence) count() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.events
}

func (b *binaryEvidence) unencodableCount() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.unencodable
}
