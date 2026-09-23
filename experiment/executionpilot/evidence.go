package executionpilot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"exchange_sim/evstream"
	"exchange_sim/exchange"
	"exchange_sim/simulations/executionlab"
)

const (
	EvidenceSchemaID    = "execution-pilot-opaque-v2"
	evidenceSchemaEpoch = 0x4d450002
)

type evidenceEnvelope struct {
	Source  string          `json:"source"`
	Name    string          `json:"name"`
	Route   string          `json:"route,omitempty"`
	Payload json.RawMessage `json:"payload"`
}

type EvidenceIdentity struct {
	SchemaID      string `json:"schema_id"`
	ExecutionHash string `json:"execution_hash"`
	FrameCount    uint64 `json:"frame_count"`
}

type Recorder struct {
	mu       sync.Mutex
	writer   *evstream.Writer
	firstErr error
	finished bool
}

func NewRecorder(output io.Writer) *Recorder {
	return &Recorder{writer: evstream.NewWriter(output, evstream.WriterOptions{
		SchemaEpoch: evidenceSchemaEpoch,
	})}
}

// Record is safe at exchange and actor callback boundaries. It serializes
// payloads immediately, before an exchange may reuse or mutate an order.
func (r *Recorder) Record(observation executionlab.EvidenceObservation) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.firstErr != nil || r.finished {
		return
	}
	if observation.Source == "" || observation.Name == "" || observation.Payload == nil {
		r.firstErr = errors.New("execution pilot: incomplete evidence observation")
		return
	}
	payload, err := json.Marshal(observation.Payload)
	if err != nil {
		r.firstErr = fmt.Errorf("execution pilot: encode %s: %w", observation.Name, err)
		return
	}
	envelope := evidenceEnvelope{Source: observation.Source, Name: observation.Name, Route: observation.Route, Payload: payload}
	if err := r.writer.AppendInterning(observation.Timestamp, observation.ClientID, 0, exchange.OpaqueJSON{Value: envelope}); err != nil {
		r.firstErr = fmt.Errorf("execution pilot: append %s: %w", observation.Name, err)
	}
}

func (r *Recorder) Finish() (EvidenceIdentity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finished {
		return EvidenceIdentity{}, errors.New("execution pilot: evidence already finished")
	}
	r.finished = true
	if r.firstErr != nil {
		return EvidenceIdentity{}, r.firstErr
	}
	if err := r.writer.Close(); err != nil {
		return EvidenceIdentity{}, fmt.Errorf("execution pilot: close evidence: %w", err)
	}
	hash := r.writer.ExecutionHash()
	return EvidenceIdentity{
		SchemaID: EvidenceSchemaID, ExecutionHash: hex.EncodeToString(hash[:]),
		FrameCount: r.writer.Count(),
	}, nil
}

type RecordedEvent struct {
	Sequence  uint64
	Timestamp int64
	ClientID  uint64
	Source    string
	Name      string
	Route     string
	Payload   json.RawMessage
}

func WalkEvidence(input io.Reader, expected EvidenceIdentity, visit func(RecordedEvent) error) error {
	if expected.SchemaID != EvidenceSchemaID || !hexDigest(expected.ExecutionHash, sha256.Size) || expected.FrameCount == 0 {
		return errors.New("execution pilot: invalid expected evidence identity")
	}
	reader, err := evstream.NewReader(input, evstream.ReaderOptions{VerifyHash: true})
	if err != nil {
		return err
	}
	if reader.SchemaEpoch() != evidenceSchemaEpoch {
		return errors.New("execution pilot: evidence schema epoch mismatch")
	}
	err = reader.Range(func(frame evstream.Frame) error {
		if frame.Header.SchemaID != evstream.SchemaOpaqueJSON || frame.Header.SchemaVersion != 1 || frame.Header.VenueRef != 0 {
			return fmt.Errorf("execution pilot: unexpected evidence frame schema %d/%d", frame.Header.SchemaID, frame.Header.SchemaVersion)
		}
		body, err := exchange.RenderPayloadJSONVersioned(frame.Header.SchemaID, frame.Header.SchemaVersion, frame.Payload, reader)
		if err != nil {
			return err
		}
		if err := rejectDuplicateJSONKeys(body); err != nil {
			return err
		}
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		var envelope evidenceEnvelope
		if err := decoder.Decode(&envelope); err != nil {
			return fmt.Errorf("execution pilot: invalid evidence envelope: %w", err)
		}
		if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
			return errors.New("execution pilot: trailing evidence envelope content")
		}
		if envelope.Source == "" || envelope.Name == "" || len(envelope.Payload) == 0 || bytes.Equal(envelope.Payload, []byte("null")) {
			return errors.New("execution pilot: incomplete evidence envelope")
		}
		return visit(RecordedEvent{
			Sequence: frame.Header.Seq, Timestamp: frame.Header.SimTS,
			ClientID: frame.Header.ClientID, Source: envelope.Source,
			Name: envelope.Name, Route: envelope.Route, Payload: envelope.Payload,
		})
	})
	if err != nil {
		return err
	}
	hash := reader.ExecutionHash()
	if expected.ExecutionHash != hex.EncodeToString(hash[:]) || expected.FrameCount != reader.Count() {
		return errors.New("execution pilot: evidence hash or frame count mismatch")
	}
	return nil
}
