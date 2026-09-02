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
	RouteCompression string `json:"route_compression"`
	ExecutionHash    string `json:"execution_stream_hash"`
	CanonicalHash    string `json:"canonical_execution_stream_hash"`
}

// RouteCompression controls how reconstructed route files are stored. It is a
// storage choice only: the renderer validates and hashes the same JSON records
// before they are compressed, and the analysis package can read either form.
type RouteCompression string

const (
	RouteCompressionNone RouteCompression = "none"
	RouteCompressionZstd RouteCompression = "zstd"
)

// BinaryRenderOptions makes renderer storage policy explicit at the adapter
// boundary. The default remains uncompressed JSONL for compatibility with
// existing callers and fixtures.
type BinaryRenderOptions struct {
	RouteCompression RouteCompression
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
	return RenderBinaryEvidenceWithOptions(inputDir, outDir, BinaryRenderOptions{
		RouteCompression: RouteCompressionNone,
	})
}

// RenderBinaryEvidenceWithOptions reconstructs routed evidence using the
// requested route storage policy. RouteCompressionZstd writes one independent
// zstd stream per route with a .jsonl.zst suffix; the logical route remains
// the original .jsonl path for sequence and symbol semantics.
func RenderBinaryEvidenceWithOptions(inputDir, outDir string, options BinaryRenderOptions) (BinaryRenderReport, error) {
	if inputDir == "" || outDir == "" {
		return BinaryRenderReport{}, fmt.Errorf("multivenue: input and output directories are required")
	}
	if options.RouteCompression == "" {
		options.RouteCompression = RouteCompressionNone
	}
	if options.RouteCompression != RouteCompressionNone && options.RouteCompression != RouteCompressionZstd {
		return BinaryRenderReport{}, fmt.Errorf("multivenue: unsupported route compression %q", options.RouteCompression)
	}
	inputAbs, err := canonicalRenderPath(inputDir, false)
	if err != nil {
		return BinaryRenderReport{}, fmt.Errorf("multivenue: input path: %w", err)
	}
	outAbs, err := canonicalRenderPath(outDir, true)
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

	sidecars, err := openRenderSidecars(filepath.Join(inputAbs, "venues"))
	if err != nil {
		return BinaryRenderReport{}, err
	}
	defer func() { _ = sidecars.close() }()
	rendered, err := newRenderOutput(outAbs, options.RouteCompression)
	if err != nil {
		return BinaryRenderReport{}, err
	}
	defer rendered.cleanup()
	eventsFile, err := openRenderRegularFile(filepath.Join(inputAbs, "events.evs"), "open binary evidence")
	if err != nil {
		return BinaryRenderReport{}, err
	}
	defer eventsFile.Close()
	attestation, err := readBinaryAttestation(inputAbs)
	if err != nil {
		return BinaryRenderReport{}, err
	}
	hashFrame, err := binaryHashFrameForContract(attestation.Hashing)
	if err != nil {
		return BinaryRenderReport{}, err
	}
	reader, err := evstream.NewReader(eventsFile, evstream.ReaderOptions{
		VerifyHash: true,
		HashFrame:  hashFrame,
	})
	if err != nil {
		return BinaryRenderReport{}, fmt.Errorf("multivenue: binary evidence reader: %w", err)
	}

	var eventFrames uint64
	if err := reader.Range(func(frame evstream.Frame) error {
		key, record, err := renderBinaryFrame(reader, frame)
		if err != nil {
			return err
		}
		if err := sidecars.flushBefore(key.venue, record.sequence, rendered); err != nil {
			return err
		}
		if err := sidecars.rejectDuplicate(key.venue, record.sequence); err != nil {
			return err
		}
		if err := rendered.append(key, record); err != nil {
			return err
		}
		eventFrames++
		return nil
	}); err != nil {
		return BinaryRenderReport{}, fmt.Errorf("multivenue: reconstruct binary evidence: %w", err)
	}
	if !reader.Terminated() {
		return BinaryRenderReport{}, fmt.Errorf("multivenue: binary evidence has no completion trailer")
	}
	if err := sidecars.flushAll(rendered); err != nil {
		return BinaryRenderReport{}, err
	}
	if err := validateBinaryAttestation(inputAbs, attestation, eventFrames, reader, sidecars.digest, contract.Config.LogMode); err != nil {
		return BinaryRenderReport{}, err
	}
	if err := rendered.commit(); err != nil {
		return BinaryRenderReport{}, err
	}
	digest := reader.ExecutionHash()
	rawDigest := reader.RawExecutionHash()
	return BinaryRenderReport{
		EventFrames:      eventFrames,
		DictionaryFrames: reader.Count() - eventFrames,
		Routes:           rendered.routeCount(),
		RouteCompression: string(options.RouteCompression),
		ExecutionHash:    hex.EncodeToString(digest[:]),
		CanonicalHash:    hex.EncodeToString(rawDigest[:]),
	}, nil
}

func readRenderRunContract(inputDir string) (renderRunContract, error) {
	raw, err := readRenderRegularFile(filepath.Join(inputDir, "manifest.json"), "read binary run manifest")
	if err != nil {
		return renderRunContract{}, err
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

func readBinaryAttestation(inputDir string) (binaryEvidenceArtifactRecord, error) {
	raw, err := readRenderRegularFile(filepath.Join(inputDir, "binary-evidence-attestation.json"), "read binary evidence attestation")
	if err != nil {
		return binaryEvidenceArtifactRecord{}, err
	}
	var attestation binaryEvidenceArtifactRecord
	if err := json.Unmarshal(raw, &attestation); err != nil {
		return binaryEvidenceArtifactRecord{}, fmt.Errorf("multivenue: decode binary evidence attestation: %w", err)
	}
	return attestation, nil
}

func binaryHashFrameForContract(contract string) (evstream.FrameHasher, error) {
	switch contract {
	case "":
		// Streams written before the explicit hash contract used raw frame
		// bytes, and remain renderable as historical fixtures.
		return nil, nil
	case binaryExecutionHashContract:
		return hashBinaryExecutionFrame, nil
	default:
		return nil, fmt.Errorf("multivenue: unsupported binary execution hash contract %q", contract)
	}
}

func validateBinaryAttestation(inputDir string, attestation binaryEvidenceArtifactRecord, eventFrames uint64, reader *evstream.Reader, sidecarDigest renderArtifactDigest, logMode string) error {
	digest := reader.ExecutionHash()
	executionHash := hex.EncodeToString(digest[:])
	if attestation.Domain != "canonical_binary_execution_frames" || attestation.Ordering != "ordered_stream" ||
		attestation.EventFrames != eventFrames || attestation.StreamFrames != reader.Count() ||
		attestation.ExecutionStreamHash != executionHash {
		return fmt.Errorf("multivenue: binary attestation does not match reconstructed stream")
	}
	if attestation.CanonicalExecutionStreamHash != "" {
		rawDigest := reader.RawExecutionHash()
		canonicalHash := hex.EncodeToString(rawDigest[:])
		if attestation.CanonicalExecutionStreamHash != canonicalHash {
			return fmt.Errorf("multivenue: binary attestation does not match canonical reconstruction stream")
		}
	} else if attestation.Hashing != "" {
		return fmt.Errorf("multivenue: binary attestation lacks canonical reconstruction identity")
	}
	if attestation.UnencodablePayloads != 0 {
		return fmt.Errorf("multivenue: binary evidence contains %d unencodable payloads", attestation.UnencodablePayloads)
	}
	if logMode == "full" {
		raw, err := readRenderRegularFile(filepath.Join(inputDir, "evidence-only-artifact-hash.json"), "read evidence-only attestation")
		if err != nil {
			return err
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
	if err := rejectRenderPathSymlinks(path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("multivenue: create render output: %w", err)
		}
		if err := validateRenderOutputDirectory(path); err != nil {
			return err
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("multivenue: inspect render output: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("multivenue: render output is a symlink")
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
