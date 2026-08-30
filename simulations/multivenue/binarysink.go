package multivenue

import (
	"crypto/sha256"
	"fmt"

	"exchange_sim/census"
	"os"
	"sync"

	"exchange_sim/evstream"
	"exchange_sim/evstream/codecs"
	eexchange "exchange_sim/exchange"
)

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
	events uint64
	// unencodable counts payloads replaced by a substitute, so a run reports
	// how much detail it lost rather than only that it lost some.
	unencodable uint64
	err         error
}

// binaryEvidenceEnabled reports whether the binary sink is selected. Read once
// at construction rather than per event.
func binaryEvidenceEnabled() bool { return os.Getenv("EXSIM_BINARY_EVIDENCE") != "" }

// binaryEvidenceDiscards reports whether the stream should be thrown away
// rather than written, which isolates encode-and-hash cost from storage cost.
func binaryEvidenceDiscards() bool { return os.Getenv("EXSIM_BINARY_EVIDENCE") == "discard" }

// newBinaryEvidence starts a binary sink writing to out.
//
// The codec is a storage decision and nothing else: the execution hash covers
// uncompressed canonical frames, so a run's scientific identity is the same
// whichever codec is chosen, including none. That is what makes it safe to
// expose as configuration rather than fixing it here.
func newBinaryEvidence(out interface{ Write([]byte) (int, error) }) (*binaryEvidence, error) {
	compressor, err := binaryEvidenceCodec()
	if err != nil {
		return nil, err
	}
	return &binaryEvidence{writer: evstream.NewWriter(out,
		evstream.WriterOptions{Compressor: compressor})}, nil
}

// binaryEvidenceCodec resolves EXSIM_BINARY_CODEC. An unrecognised name is an
// error rather than a silent fallback to no compression: a run that was asked
// for a codec and quietly wrote raw blocks would misreport its own storage
// cost.
func binaryEvidenceCodec() (evstream.BlockCompressor, error) {
	switch name := os.Getenv("EXSIM_BINARY_CODEC"); name {
	case "", "none":
		return nil, nil
	case "lz4":
		return codecs.NewLZ4(), nil
	case "s2":
		return codecs.NewS2(), nil
	case "zstd":
		return codecs.NewZstdFastest()
	default:
		return nil, fmt.Errorf("multivenue: unknown EXSIM_BINARY_CODEC %q", name)
	}
}

// sinkEnvelope carries the event name alongside the payload.
//
// The frame header holds sequence, time, schema, venue and client, but not the
// event name, and the name cannot be inferred from the schema id: the
// per-symbol wrapper carries nine different event names under one schema. So
// every payload is prefixed with an interned name reference — four bytes,
// because the set of event names is tiny and closed.
type sinkEnvelope struct {
	eventRef uint32
	inner    evstream.InterningAppender
}

func (e sinkEnvelope) SchemaID() uint16      { return e.inner.SchemaID() }
func (e sinkEnvelope) SchemaVersion() uint16 { return e.inner.SchemaVersion() }

func (e sinkEnvelope) AppendPayloadInterning(dst []byte, in evstream.Interner) ([]byte, error) {
	dst = evstream.AppendUint32(dst, e.eventRef)
	return e.inner.AppendPayloadInterning(dst, in)
}

// record appends one event.
//
// A payload with no typed schema is wrapped as opaque JSON rather than
// refused, so the stream is complete from the first run and coverage can be
// raised one family at a time without the sink ever being partly JSON and
// partly binary at the file level.
func (b *binaryEvidence) record(simTime int64, clientID uint64, eventName, venueID string, payload any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return
	}

	eventRef, err := b.writer.Intern(eventName)
	if err != nil {
		b.err = err
		return
	}
	venueRef := uint32(0)
	if venueID != "" {
		if venueRef, err = b.writer.Intern(venueID); err != nil {
			b.err = err
			return
		}
	}

	inner, typed := payload.(evstream.InterningAppender)
	if !typed {
		inner = eexchange.OpaqueJSON{Value: payload}
	}
	// Coverage census: which families still ride as opaque JSON. The gap
	// between the measured speedup and the 17.9 % ceiling is bounded by this,
	// so it says directly how much is left and where.
	// Guarded at the call site, not inside CountFor: Go evaluates the
	// concatenated site name before the call, so an unguarded CountFor
	// allocates one string per event even with the census disabled. That cost
	// was live in both sinks and distorted every allocation figure taken with
	// the census off.
	if census.Enabled {
		census.CountFor("binary.sink["+eventName+"]",
			"still opaque JSON rather than a typed schema", !typed, 0)
	}
	frame := sinkEnvelope{eventRef: eventRef, inner: inner}
	if err := b.writer.AppendInterning(simTime, clientID, venueRef, frame); err != nil {
		// A payload that cannot be encoded must not end the stream. The JSON
		// path substitutes "unencodable" and keeps folding events, so a single
		// bad payload costs one event's detail; the binary path used to stop
		// accumulating and drop every event after it while the simulation ran
		// on, which is a far larger loss reported only at close.
		//
		// The writer encodes into scratch and commits only on success, so a
		// failed append leaves no partial frame behind and the substitute
		// takes its place at the same sequence number.
		frame.inner = eexchange.OpaqueJSON{Value: "unencodable"}
		if retry := b.writer.AppendInterning(simTime, clientID, venueRef, frame); retry != nil {
			// The substitute always encodes, so a failure here is the
			// underlying writer, not the payload.
			b.err = retry
			return
		}
		b.unencodable++
	}
	b.events++
}

// executionHash returns the digest over the canonical uncompressed frames. It
// is the binary path's counterpart to the JSON path's ordered-stream hash, and
// like it, it is independent of any storage compression applied afterwards.
func (b *binaryEvidence) executionHash() [sha256.Size]byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.writer.ExecutionHash()
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

// unencodableCount returns how many payloads were replaced by a substitute.
func (b *binaryEvidence) unencodableCount() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.unencodable
}

// count returns the number of events recorded.
func (b *binaryEvidence) count() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.events
}

// writeIndex persists the block directory beside the stream.
//
// It is written after the stream is closed rather than interleaved with it, so
// the evidence file is never interrupted by index bytes, and a reader that only
// wants to scan sequentially never has to know the index exists.
func (b *binaryEvidence) writeIndex(path string) error {
	if path == "" {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	if _, err := b.writer.Index().WriteTo(file); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}
