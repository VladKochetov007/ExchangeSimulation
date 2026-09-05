package multivenue

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"exchange_sim/evstream"
	"exchange_sim/exchange"
)

// BinaryRenderReport describes a completed binary-to-JSON reconstruction.
// EventFrames excludes dictionary frames; ExecutionHash covers both, exactly
// as the writer and checkpoint attestation do.
type BinaryRenderReport struct {
	EventFrames      uint64 `json:"event_frames"`
	DictionaryFrames uint64 `json:"dictionary_frames"`
	Routes           int    `json:"routes"`
	ExecutionHash    string `json:"execution_stream_hash"`
}

type renderRouteKey struct {
	venue string
	route string
}

type renderRecord struct {
	sequence uint64
	raw      []byte
}

type renderEventData struct {
	VenueID  string          `json:"venue_id"`
	Sequence uint64          `json:"sequence"`
	Payload  json.RawMessage `json:"payload"`
}

type renderPersistedEvent struct {
	ClientID uint64          `json:"client_id"`
	Data     renderEventData `json:"data"`
	Event    string          `json:"event"`
	SimTS    int64           `json:"sim_ts"`
}

type renderRunContract struct {
	SchemaVersion int `json:"schema_version"`
	Config        struct {
		LogMode        string `json:"log_mode"`
		EvidenceFormat string `json:"evidence_format"`
	} `json:"config"`
}

type renderArtifactDigest struct {
	events int64
	limbs  [4]uint64
}

// RenderBinaryEvidence reconstructs the routed venue JSONL layout from a
// completed evstream_v3 run. The input is never modified. Existing JSONL
// files are treated as LogEvidenceOnly sidecars and merged by their shared
// per-venue sequence; missing, duplicate, or out-of-order records fail closed.
// outDir must be empty or absent and must not be inside inputDir.
func RenderBinaryEvidence(inputDir, outDir string) (BinaryRenderReport, error) {
	if inputDir == "" || outDir == "" {
		return BinaryRenderReport{}, fmt.Errorf("multivenue: input and output directories are required")
	}
	inputAbs, err := filepath.Abs(inputDir)
	if err != nil {
		return BinaryRenderReport{}, fmt.Errorf("multivenue: input path: %w", err)
	}
	outAbs, err := filepath.Abs(outDir)
	if err != nil {
		return BinaryRenderReport{}, fmt.Errorf("multivenue: output path: %w", err)
	}
	if pathContains(inputAbs, outAbs) {
		return BinaryRenderReport{}, fmt.Errorf("multivenue: output directory must not be inside input directory")
	}
	if err := prepareEmptyDirectory(outAbs); err != nil {
		return BinaryRenderReport{}, err
	}
	contract, err := readRenderRunContract(inputAbs)
	if err != nil {
		return BinaryRenderReport{}, err
	}

	sidecars, err := openEvidenceOnlySidecars(filepath.Join(inputAbs, "venues"))
	if err != nil {
		return BinaryRenderReport{}, err
	}
	defer sidecars.close()

	eventsFile, err := os.Open(filepath.Join(inputAbs, "events.evs"))
	if err != nil {
		return BinaryRenderReport{}, fmt.Errorf("multivenue: open binary evidence: %w", err)
	}
	defer eventsFile.Close()
	reader, err := evstream.NewReader(eventsFile, evstream.ReaderOptions{VerifyHash: true})
	if err != nil {
		return BinaryRenderReport{}, fmt.Errorf("multivenue: binary evidence reader: %w", err)
	}

	// Frames and sidecars both already arrive in per-route sequence order, so
	// the two are merged as they are read rather than accumulated and sorted.
	// Holding the whole rendered run made peak memory scale with the run: 909 MB
	// at twenty minutes and 2.2 GB at an hour, which is about 62 GB for the
	// twenty-four hour runs this format exists to make affordable.
	writers := newRouteWriters(outAbs)
	seen := newVenueSequences()
	failed := true
	defer func() {
		if failed {
			writers.abort()
			os.RemoveAll(filepath.Join(outAbs, "venues"))
		}
	}()

	var eventFrames uint64
	if err := reader.Range(func(frame evstream.Frame) error {
		key, record, err := renderBinaryFrame(reader, frame)
		if err != nil {
			return err
		}
		if err := sidecars.emitBefore(key, record.sequence, writers, seen); err != nil {
			return err
		}
		if err := seen.observe(key, record.sequence); err != nil {
			return err
		}
		if err := writers.write(key, record.sequence, record.raw); err != nil {
			return err
		}
		eventFrames++
		return nil
	}); err != nil {
		return BinaryRenderReport{}, fmt.Errorf("multivenue: reconstruct binary evidence: %w", err)
	}
	if err := sidecars.drain(writers, seen); err != nil {
		return BinaryRenderReport{}, err
	}
	if err := writers.close(); err != nil {
		return BinaryRenderReport{}, err
	}
	if !reader.Terminated() {
		return BinaryRenderReport{}, fmt.Errorf("multivenue: binary evidence has no completion trailer")
	}
	if err := validateBinaryAttestation(inputAbs, eventFrames, reader, sidecars.digest, contract.Config.LogMode); err != nil {
		return BinaryRenderReport{}, err
	}
	if err := seen.validate(); err != nil {
		return BinaryRenderReport{}, err
	}
	failed = false
	digest := reader.ExecutionHash()
	return BinaryRenderReport{
		EventFrames:      eventFrames,
		DictionaryFrames: reader.Count() - eventFrames,
		Routes:           writers.routes(),
		ExecutionHash:    hex.EncodeToString(digest[:]),
	}, nil
}

func readRenderRunContract(inputDir string) (renderRunContract, error) {
	raw, err := os.ReadFile(filepath.Join(inputDir, "manifest.json"))
	if err != nil {
		return renderRunContract{}, fmt.Errorf("multivenue: read binary run manifest: %w", err)
	}
	var contract renderRunContract
	if err := json.Unmarshal(raw, &contract); err != nil {
		return renderRunContract{}, fmt.Errorf("multivenue: decode binary run manifest: %w", err)
	}
	if contract.SchemaVersion < 2 || contract.Config.EvidenceFormat != binaryRepresentation {
		return renderRunContract{}, fmt.Errorf("multivenue: manifest does not declare %s evidence", binaryRepresentation)
	}
	if contract.Config.LogMode != "full" && contract.Config.LogMode != "none" {
		return renderRunContract{}, fmt.Errorf("multivenue: manifest has unsupported log mode %q", contract.Config.LogMode)
	}
	return contract, nil
}

func validateBinaryAttestation(inputDir string, eventFrames uint64, reader *evstream.Reader, sidecarDigest renderArtifactDigest, logMode string) error {
	raw, err := os.ReadFile(filepath.Join(inputDir, "binary-evidence-attestation.json"))
	if err != nil {
		return fmt.Errorf("multivenue: read binary evidence attestation: %w", err)
	}
	var attestation binaryEvidenceArtifactRecord
	if err := json.Unmarshal(raw, &attestation); err != nil {
		return fmt.Errorf("multivenue: decode binary evidence attestation: %w", err)
	}
	digest := reader.ExecutionHash()
	executionHash := hex.EncodeToString(digest[:])
	if attestation.Domain != "canonical_binary_execution_frames" || attestation.Ordering != "ordered_stream" ||
		attestation.EventFrames != eventFrames || attestation.StreamFrames != reader.Count() ||
		attestation.ExecutionStreamHash != executionHash {
		return fmt.Errorf("multivenue: binary attestation does not match reconstructed stream")
	}
	if attestation.UnencodablePayloads != 0 {
		return fmt.Errorf("multivenue: binary evidence contains %d unencodable payloads", attestation.UnencodablePayloads)
	}
	if logMode == "full" {
		raw, err := os.ReadFile(filepath.Join(inputDir, "evidence-only-artifact-hash.json"))
		if err != nil {
			return fmt.Errorf("multivenue: read evidence-only attestation: %w", err)
		}
		var artifact evidenceArtifactRecord
		if err := json.Unmarshal(raw, &artifact); err != nil {
			return fmt.Errorf("multivenue: decode evidence-only attestation: %w", err)
		}
		if artifact.Domain != "persisted_json_log_evidence_only" || artifact.Ordering != "unordered_multiset" ||
			artifact.Events != sidecarDigest.events || artifact.Digest != sidecarDigest.hex() {
			return fmt.Errorf("multivenue: evidence-only attestation does not match sidecars")
		}
	}
	return nil
}

func renderBinaryFrame(reader *evstream.Reader, frame evstream.Frame) (renderRouteKey, renderRecord, error) {
	if len(frame.Payload) < 16 {
		return renderRouteKey{}, renderRecord{}, fmt.Errorf("multivenue: frame %d payload too short for v3 envelope", frame.Header.Seq)
	}
	routeRef := binary.LittleEndian.Uint32(frame.Payload[0:4])
	eventRef := binary.LittleEndian.Uint32(frame.Payload[4:8])
	sequence := binary.LittleEndian.Uint64(frame.Payload[8:16])
	if routeRef == 0 || eventRef == 0 || sequence == 0 {
		return renderRouteKey{}, renderRecord{}, fmt.Errorf("multivenue: frame %d has incomplete route/event/sequence envelope", frame.Header.Seq)
	}
	route, ok := reader.Lookup(routeRef)
	if !ok {
		return renderRouteKey{}, renderRecord{}, fmt.Errorf("multivenue: frame %d route ref %d is undefined", frame.Header.Seq, routeRef)
	}
	eventName, ok := reader.Lookup(eventRef)
	if !ok {
		return renderRouteKey{}, renderRecord{}, fmt.Errorf("multivenue: frame %d event ref %d is undefined", frame.Header.Seq, eventRef)
	}
	if err := validateRoute(route); err != nil {
		return renderRouteKey{}, renderRecord{}, fmt.Errorf("multivenue: frame %d: %w", frame.Header.Seq, err)
	}
	if frame.Venue == "" {
		return renderRouteKey{}, renderRecord{}, fmt.Errorf("multivenue: frame %d has no venue", frame.Header.Seq)
	}
	payload, err := exchange.RenderPayloadJSONVersioned(
		frame.Header.SchemaID, frame.Header.SchemaVersion, frame.Payload[16:], reader)
	if err != nil {
		return renderRouteKey{}, renderRecord{}, fmt.Errorf("multivenue: frame %d (%s): %w", frame.Header.Seq, eventName, err)
	}
	raw, err := json.Marshal(renderPersistedEvent{
		ClientID: frame.Header.ClientID,
		Data: renderEventData{
			VenueID:  frame.Venue,
			Sequence: sequence,
			Payload:  payload,
		},
		Event: eventName,
		SimTS: frame.Header.SimTS,
	})
	if err != nil {
		return renderRouteKey{}, renderRecord{}, fmt.Errorf("multivenue: frame %d JSON rendering: %w", frame.Header.Seq, err)
	}
	return renderRouteKey{venue: frame.Venue, route: filepath.ToSlash(route)}, renderRecord{sequence: sequence, raw: raw}, nil
}

func (d *renderArtifactDigest) add(record []byte) {
	hash := sha256.Sum256(record)
	var carry uint64
	for index := 3; index >= 0; index-- {
		limb := binary.BigEndian.Uint64(hash[index*8 : index*8+8])
		sum := d.limbs[index] + limb
		newCarry := uint64(0)
		if sum < d.limbs[index] {
			newCarry = 1
		}
		sum += carry
		if sum < carry {
			newCarry = 1
		}
		d.limbs[index] = sum
		carry = newCarry
	}
	d.events++
}

func (d renderArtifactDigest) hex() string {
	var encoded [32]byte
	for index, limb := range d.limbs {
		binary.BigEndian.PutUint64(encoded[index*8:index*8+8], limb)
	}
	return hex.EncodeToString(encoded[:])
}

func prepareEmptyDirectory(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("multivenue: create render output: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("multivenue: inspect render output: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("multivenue: render output is not a directory")
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("multivenue: read render output: %w", err)
	}
	if len(entries) != 0 {
		return fmt.Errorf("multivenue: refusing to overwrite non-empty render output")
	}
	return nil
}

func pathContains(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func validateRoute(route string) error {
	if route == "" || filepath.IsAbs(filepath.FromSlash(route)) {
		return fmt.Errorf("unsafe empty or absolute route %q", route)
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(route)))
	if clean != route || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("unsafe route %q", route)
	}
	return nil
}
