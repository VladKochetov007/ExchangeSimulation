package multivenue

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

	routes, err := readEvidenceOnlySidecars(filepath.Join(inputAbs, "venues"))
	if err != nil {
		return BinaryRenderReport{}, err
	}
	eventsFile, err := os.Open(filepath.Join(inputAbs, "events.evs"))
	if err != nil {
		return BinaryRenderReport{}, fmt.Errorf("multivenue: open binary evidence: %w", err)
	}
	defer eventsFile.Close()
	reader, err := evstream.NewReader(eventsFile, evstream.ReaderOptions{VerifyHash: true})
	if err != nil {
		return BinaryRenderReport{}, fmt.Errorf("multivenue: binary evidence reader: %w", err)
	}

	var eventFrames uint64
	if err := reader.Range(func(frame evstream.Frame) error {
		key, record, err := renderBinaryFrame(reader, frame)
		if err != nil {
			return err
		}
		if err := addRenderRecord(routes, key, record); err != nil {
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
	if err := validateRenderRecords(routes); err != nil {
		return BinaryRenderReport{}, err
	}
	if err := writeRenderedRoutes(outAbs, routes); err != nil {
		return BinaryRenderReport{}, err
	}
	digest := reader.ExecutionHash()
	return BinaryRenderReport{
		EventFrames:      eventFrames,
		DictionaryFrames: reader.Count() - eventFrames,
		Routes:           len(routes),
		ExecutionHash:    hex.EncodeToString(digest[:]),
	}, nil
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

func readEvidenceOnlySidecars(venuesDir string) (map[renderRouteKey][]renderRecord, error) {
	routes := make(map[renderRouteKey][]renderRecord)
	if _, err := os.Stat(venuesDir); err != nil {
		if os.IsNotExist(err) {
			return routes, nil
		}
		return nil, fmt.Errorf("multivenue: inspect venue evidence: %w", err)
	}
	err := filepath.WalkDir(venuesDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".jsonl" {
			return nil
		}
		relative, err := filepath.Rel(venuesDir, path)
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(relative), "/")
		if len(parts) < 2 || parts[0] == "" {
			return fmt.Errorf("multivenue: sidecar path %q is not venue-qualified", relative)
		}
		venue := parts[0]
		route := strings.Join(parts[1:], "/")
		if err := validateRoute(route); err != nil {
			return fmt.Errorf("multivenue: sidecar %q: %w", relative, err)
		}
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("multivenue: open sidecar %q: %w", relative, err)
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for scanner.Scan() {
			raw := append([]byte(nil), scanner.Bytes()...)
			var event renderPersistedEvent
			if err := json.Unmarshal(raw, &event); err != nil {
				return fmt.Errorf("multivenue: sidecar %q malformed JSON: %w", relative, err)
			}
			if event.Event == "" || event.Data.VenueID != venue || event.Data.Sequence == 0 || len(event.Data.Payload) == 0 {
				return fmt.Errorf("multivenue: sidecar %q has incomplete persisted event", relative)
			}
			key := renderRouteKey{venue: venue, route: route}
			if err := addRenderRecord(routes, key, renderRecord{sequence: event.Data.Sequence, raw: raw}); err != nil {
				return err
			}
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("multivenue: read sidecar %q: %w", relative, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return routes, nil
}

func addRenderRecord(routes map[renderRouteKey][]renderRecord, key renderRouteKey, record renderRecord) error {
	if key.venue == "" || record.sequence == 0 {
		return fmt.Errorf("multivenue: incomplete rendered evidence record")
	}
	for _, existing := range routes[key] {
		if existing.sequence == record.sequence {
			return fmt.Errorf("multivenue: duplicate venue sequence %s/%s#%d", key.venue, key.route, record.sequence)
		}
	}
	routes[key] = append(routes[key], record)
	return nil
}

func validateRenderRecords(routes map[renderRouteKey][]renderRecord) error {
	byVenue := make(map[string]map[uint64]struct{})
	for key, records := range routes {
		if err := validateRoute(key.route); err != nil {
			return fmt.Errorf("multivenue: rendered route %s/%s: %w", key.venue, key.route, err)
		}
		seen := byVenue[key.venue]
		if seen == nil {
			seen = make(map[uint64]struct{})
			byVenue[key.venue] = seen
		}
		for _, record := range records {
			if _, exists := seen[record.sequence]; exists {
				return fmt.Errorf("multivenue: duplicate venue sequence %s#%d across routes", key.venue, record.sequence)
			}
			seen[record.sequence] = struct{}{}
		}
	}
	for venue, seen := range byVenue {
		var highest uint64
		for sequence := range seen {
			if sequence > highest {
				highest = sequence
			}
		}
		for sequence := uint64(1); sequence <= highest; sequence++ {
			if _, ok := seen[sequence]; !ok {
				return fmt.Errorf("multivenue: missing venue sequence %s#%d", venue, sequence)
			}
		}
	}
	return nil
}

func writeRenderedRoutes(outDir string, routes map[renderRouteKey][]renderRecord) error {
	keys := make([]renderRouteKey, 0, len(routes))
	for key := range routes {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].venue != keys[j].venue {
			return keys[i].venue < keys[j].venue
		}
		return keys[i].route < keys[j].route
	})
	for _, key := range keys {
		records := routes[key]
		sort.Slice(records, func(i, j int) bool { return records[i].sequence < records[j].sequence })
		path := filepath.Join(outDir, "venues", key.venue, filepath.FromSlash(key.route))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return fmt.Errorf("multivenue: create rendered route %q: %w", key.route, err)
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return fmt.Errorf("multivenue: create rendered route %q: %w", key.route, err)
		}
		writer := bufio.NewWriterSize(file, 64*1024)
		for _, record := range records {
			if _, err := writer.Write(record.raw); err != nil {
				file.Close()
				return fmt.Errorf("multivenue: write rendered route %q: %w", key.route, err)
			}
			if err := writer.WriteByte('\n'); err != nil {
				file.Close()
				return fmt.Errorf("multivenue: write rendered route newline %q: %w", key.route, err)
			}
		}
		if err := writer.Flush(); err != nil {
			file.Close()
			return fmt.Errorf("multivenue: flush rendered route %q: %w", key.route, err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("multivenue: close rendered route %q: %w", key.route, err)
		}
	}
	return nil
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
