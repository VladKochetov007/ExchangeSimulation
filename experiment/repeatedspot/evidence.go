package repeatedspot

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
	"exchange_sim/experiment/executionpilot"
)

const (
	EvidenceSchemaID          = "repeated-spot-opaque-v4"
	SignalEvidenceSchemaID    = "repeated-spot-opaque-v5"
	evidenceSchemaEpoch       = 0x45300003
	signalEvidenceSchemaEpoch = 0x45300004
)

type EvidenceIdentity struct {
	SchemaID      string `json:"schema_id"`
	ExecutionHash string `json:"execution_hash"`
	FrameCount    uint64 `json:"frame_count"`
}

type Event struct {
	Sequence  uint64          `json:"sequence"`
	Timestamp int64           `json:"timestamp"`
	ClientID  uint64          `json:"client_id"`
	Source    string          `json:"source"`
	Name      string          `json:"name"`
	Route     string          `json:"route,omitempty"`
	Payload   json.RawMessage `json:"payload"`
}

type envelope struct {
	Source  string          `json:"source"`
	Name    string          `json:"name"`
	Route   string          `json:"route,omitempty"`
	Payload json.RawMessage `json:"payload"`
}

// Recorder serializes all observed causal boundaries into one ordered stream.
// It is write-only from actors and the exchange; callers must check Finish.
type Recorder struct {
	mu       sync.Mutex
	writer   *evstream.Writer
	schemaID string
	firstErr error
	finished bool
}

func NewRecorder(output io.Writer) *Recorder {
	return &Recorder{writer: evstream.NewWriter(output, evstream.WriterOptions{SchemaEpoch: evidenceSchemaEpoch}),
		schemaID: EvidenceSchemaID}
}

func NewRecorderForSchema(output io.Writer, schemaID string) (*Recorder, error) {
	epoch, ok := repeatedSpotSchemaEpoch(schemaID)
	if !ok || output == nil {
		return nil, errors.New("repeated spot: unsupported evidence schema or nil output")
	}
	return &Recorder{writer: evstream.NewWriter(output, evstream.WriterOptions{SchemaEpoch: epoch}), schemaID: schemaID}, nil
}

func repeatedSpotSchemaEpoch(schemaID string) (uint32, bool) {
	switch schemaID {
	case EvidenceSchemaID:
		return evidenceSchemaEpoch, true
	case SignalEvidenceSchemaID:
		return signalEvidenceSchemaEpoch, true
	default:
		return 0, false
	}
}

func (recorder *Recorder) Record(timestamp int64, clientID uint64, source, name, route string, payload any) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.firstErr != nil || recorder.finished {
		return
	}
	if timestamp < 0 || source == "" || name == "" || payload == nil {
		recorder.firstErr = errors.New("repeated spot: incomplete evidence observation")
		return
	}
	encoded, err := json.Marshal(payload)
	if err != nil || bytes.Equal(encoded, []byte("null")) {
		recorder.firstErr = fmt.Errorf("repeated spot: encode %s/%s: %w", source, name, err)
		if err == nil {
			recorder.firstErr = fmt.Errorf("repeated spot: null %s/%s payload", source, name)
		}
		return
	}
	observation := envelope{Source: source, Name: name, Route: route, Payload: encoded}
	if err := recorder.writer.AppendInterning(timestamp, clientID, 0, exchange.OpaqueJSON{Value: observation}); err != nil {
		recorder.firstErr = fmt.Errorf("repeated spot: append %s/%s: %w", source, name, err)
	}
}

func (recorder *Recorder) Finish() (EvidenceIdentity, error) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.finished {
		return EvidenceIdentity{}, errors.New("repeated spot: evidence already finished")
	}
	recorder.finished = true
	closeErr := recorder.writer.Close()
	if recorder.firstErr != nil {
		return EvidenceIdentity{}, recorder.firstErr
	}
	if closeErr != nil {
		return EvidenceIdentity{}, closeErr
	}
	hash := recorder.writer.ExecutionHash()
	return EvidenceIdentity{SchemaID: recorder.schemaID, ExecutionHash: hex.EncodeToString(hash[:]),
		FrameCount: recorder.writer.Count()}, nil
}

type ExchangeLogger struct {
	Recorder *Recorder
	Route    string
}

func (logger ExchangeLogger) LogEvent(timestamp int64, clientID uint64, name string, payload any) {
	logger.Recorder.Record(timestamp, clientID, "exchange", name, logger.Route, payload)
}

func WalkEvidence(input io.Reader, expected EvidenceIdentity, visit func(Event) error) error {
	epoch, supported := repeatedSpotSchemaEpoch(expected.SchemaID)
	if !supported || len(expected.ExecutionHash) != 2*sha256.Size || expected.FrameCount == 0 || visit == nil {
		return errors.New("repeated spot: invalid evidence identity or visitor")
	}
	if _, err := hex.DecodeString(expected.ExecutionHash); err != nil {
		return fmt.Errorf("repeated spot: invalid evidence hash: %w", err)
	}
	reader, err := evstream.NewReader(input, evstream.ReaderOptions{VerifyHash: true})
	if err != nil {
		return err
	}
	if reader.SchemaEpoch() != epoch {
		return errors.New("repeated spot: evidence schema epoch mismatch")
	}
	if err := reader.Range(func(frame evstream.Frame) error {
		if frame.Header.SchemaID != evstream.SchemaOpaqueJSON || frame.Header.SchemaVersion != 1 || frame.Header.VenueRef != 0 {
			return fmt.Errorf("repeated spot: unexpected frame schema %d/%d", frame.Header.SchemaID, frame.Header.SchemaVersion)
		}
		body, err := exchange.RenderPayloadJSONVersioned(frame.Header.SchemaID, frame.Header.SchemaVersion, frame.Payload, reader)
		if err != nil {
			return err
		}
		if err := executionpilot.ValidateStrictJSON(body); err != nil {
			return fmt.Errorf("repeated spot: invalid evidence JSON: %w", err)
		}
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		var observation envelope
		if err := decoder.Decode(&observation); err != nil {
			return err
		}
		if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
			return errors.New("repeated spot: trailing envelope content")
		}
		if observation.Source == "" || observation.Name == "" || len(observation.Payload) == 0 || bytes.Equal(observation.Payload, []byte("null")) {
			return errors.New("repeated spot: incomplete evidence envelope")
		}
		return visit(Event{Sequence: frame.Header.Seq, Timestamp: frame.Header.SimTS,
			ClientID: frame.Header.ClientID, Source: observation.Source, Name: observation.Name,
			Route: observation.Route, Payload: observation.Payload})
	}); err != nil {
		return err
	}
	hash := reader.ExecutionHash()
	if expected.ExecutionHash != hex.EncodeToString(hash[:]) || expected.FrameCount != reader.Count() {
		return errors.New("repeated spot: evidence hash or frame count mismatch")
	}
	return nil
}
