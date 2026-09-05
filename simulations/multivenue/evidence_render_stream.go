package multivenue

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The pieces a streaming render is made of.
//
// Rendering used to accumulate every reconstructed record in memory, sort each
// route, and write at the end. That made peak memory a function of the run:
// 909 MB at twenty minutes, 2.2 GB at an hour, and about 62 GB at the twenty-four
// hours this format exists to make affordable. Nothing required it. Frames
// arrive in per-route sequence order and each sidecar file is written in
// sequence order, so the two are already sorted and can be merged as they are
// read.

// sidecarReader is one evidence-only file, read lazily with a single record of
// lookahead so it can be merged against the frame stream.
type sidecarReader struct {
	path    string
	file    *os.File
	scanner *bufio.Scanner
	pending renderRecord
	loaded  bool
	done    bool
}

// evidenceOnlySidecars holds one reader per route plus the running digest over
// every evidence-only record. The digest is an addition of per-record hashes
// and so does not depend on the order records are folded in, which is what lets
// them be read lazily.
type evidenceOnlySidecars struct {
	readers map[renderRouteKey]*sidecarReader
	digest  renderArtifactDigest
}

func openEvidenceOnlySidecars(venuesDir string) (*evidenceOnlySidecars, error) {
	sidecars := &evidenceOnlySidecars{readers: map[renderRouteKey]*sidecarReader{}}
	if _, err := os.Stat(venuesDir); err != nil {
		if os.IsNotExist(err) {
			return sidecars, nil
		}
		return nil, fmt.Errorf("multivenue: inspect venue evidence: %w", err)
	}
	err := filepath.WalkDir(venuesDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".jsonl" {
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
		route := strings.Join(parts[1:], "/")
		if err := validateRoute(route); err != nil {
			return fmt.Errorf("multivenue: sidecar %q: %w", relative, err)
		}
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("multivenue: open sidecar %q: %w", relative, err)
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
		sidecars.readers[renderRouteKey{venue: parts[0], route: route}] = &sidecarReader{
			path: relative, file: file, scanner: scanner,
		}
		return nil
	})
	if err != nil {
		sidecars.close()
		return nil, err
	}
	return sidecars, nil
}

// peek returns the next record of one route without consuming it.
func (s *evidenceOnlySidecars) peek(reader *sidecarReader) (renderRecord, bool, error) {
	if reader.loaded {
		return reader.pending, true, nil
	}
	if reader.done {
		return renderRecord{}, false, nil
	}
	for reader.scanner.Scan() {
		raw := append([]byte(nil), reader.scanner.Bytes()...)
		if len(raw) == 0 {
			continue
		}
		var event renderPersistedEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			return renderRecord{}, false, fmt.Errorf("multivenue: sidecar %q malformed JSON: %w", reader.path, err)
		}
		if event.Event == "" || event.Data.VenueID == "" || event.Data.Sequence == 0 || len(event.Data.Payload) == 0 {
			return renderRecord{}, false, fmt.Errorf("multivenue: sidecar %q has incomplete persisted event", reader.path)
		}
		s.digest.add(raw)
		reader.pending = renderRecord{sequence: event.Data.Sequence, raw: raw}
		reader.loaded = true
		return reader.pending, true, nil
	}
	if err := reader.scanner.Err(); err != nil {
		return renderRecord{}, false, fmt.Errorf("multivenue: read sidecar %q: %w", reader.path, err)
	}
	reader.done = true
	return renderRecord{}, false, nil
}

// emitBefore writes every evidence-only record of one route that belongs ahead
// of the frame about to be written.
func (s *evidenceOnlySidecars) emitBefore(key renderRouteKey, sequence uint64, writers *routeWriters, seen *venueSequences) error {
	reader := s.readers[key]
	if reader == nil {
		return nil
	}
	for {
		record, ok, err := s.peek(reader)
		if err != nil || !ok || record.sequence >= sequence {
			return err
		}
		if err := seen.observe(key, record.sequence); err != nil {
			return err
		}
		if err := writers.write(key, record.sequence, record.raw); err != nil {
			return err
		}
		reader.loaded = false
	}
}

// drain writes whatever the merge did not reach, which is every record of a
// route the stream carries no frames for and every record past the last frame.
func (s *evidenceOnlySidecars) drain(writers *routeWriters, seen *venueSequences) error {
	keys := make([]renderRouteKey, 0, len(s.readers))
	for key := range s.readers {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].venue != keys[j].venue {
			return keys[i].venue < keys[j].venue
		}
		return keys[i].route < keys[j].route
	})
	for _, key := range keys {
		reader := s.readers[key]
		for {
			record, ok, err := s.peek(reader)
			if err != nil {
				return err
			}
			if !ok {
				break
			}
			if err := seen.observe(key, record.sequence); err != nil {
				return err
			}
			if err := writers.write(key, record.sequence, record.raw); err != nil {
				return err
			}
			reader.loaded = false
		}
	}
	return nil
}

func (s *evidenceOnlySidecars) close() {
	for _, reader := range s.readers {
		if reader.file != nil {
			reader.file.Close()
			reader.file = nil
		}
	}
}

// routeWriters holds one buffered writer per rebuilt file, created on first use
// so the rebuilt tree contains exactly the routes the run wrote.
type routeWriters struct {
	dest    string
	buffers map[renderRouteKey]*bufio.Writer
	files   map[renderRouteKey]*os.File
	// last is the sequence most recently written to each route. The merge
	// produces sorted output only because both its inputs are sorted, and that
	// is a property of the writer this reader has no control over. The buffered
	// renderer sorted each route and so could not be wrong about it; this one
	// checks instead of assuming, because a route written out of order is a
	// plausible-looking file with no error attached.
	last map[renderRouteKey]uint64
}

func newRouteWriters(dest string) *routeWriters {
	return &routeWriters{
		dest:    dest,
		buffers: map[renderRouteKey]*bufio.Writer{},
		files:   map[renderRouteKey]*os.File{},
		last:    map[renderRouteKey]uint64{},
	}
}

func (w *routeWriters) write(key renderRouteKey, sequence uint64, raw []byte) error {
	if previous, written := w.last[key]; written && sequence <= previous {
		return fmt.Errorf("multivenue: rendered route %s/%s went backwards, sequence %d after %d",
			key.venue, key.route, sequence, previous)
	}
	w.last[key] = sequence
	return w.append(key, raw)
}

func (w *routeWriters) append(key renderRouteKey, raw []byte) error {
	out, ok := w.buffers[key]
	if !ok {
		path := filepath.Join(w.dest, "venues", key.venue, filepath.FromSlash(key.route))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return fmt.Errorf("multivenue: create rendered route %q: %w", key.route, err)
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return fmt.Errorf("multivenue: create rendered route %q: %w", key.route, err)
		}
		out = bufio.NewWriterSize(file, 64*1024)
		w.buffers[key] = out
		w.files[key] = file
	}
	if _, err := out.Write(raw); err != nil {
		return fmt.Errorf("multivenue: write rendered route %q: %w", key.route, err)
	}
	if err := out.WriteByte('\n'); err != nil {
		return fmt.Errorf("multivenue: write rendered route newline %q: %w", key.route, err)
	}
	return nil
}

func (w *routeWriters) routes() int { return len(w.buffers) }

func (w *routeWriters) close() error {
	for key, out := range w.buffers {
		if err := out.Flush(); err != nil {
			return fmt.Errorf("multivenue: flush rendered route %q: %w", key.route, err)
		}
		if err := w.files[key].Close(); err != nil {
			return fmt.Errorf("multivenue: close rendered route %q: %w", key.route, err)
		}
		delete(w.files, key)
	}
	return nil
}

func (w *routeWriters) abort() {
	for _, file := range w.files {
		file.Close()
	}
	w.files = map[renderRouteKey]*os.File{}
}

// venueSequences enforces the per-venue sequence contract while the merge runs.
//
// A set of every sequence would be the obvious structure and is what the
// buffered renderer used; it costs a map entry per record. The sequences are
// dense and start at one, so a bitmap answers the same questions -- has this
// one been seen, and is the range complete -- in one bit each.
type venueSequences struct {
	bits    map[string][]uint64
	highest map[string]uint64
	count   map[string]uint64
}

func newVenueSequences() *venueSequences {
	return &venueSequences{
		bits:    map[string][]uint64{},
		highest: map[string]uint64{},
		count:   map[string]uint64{},
	}
}

func (v *venueSequences) observe(key renderRouteKey, sequence uint64) error {
	if key.venue == "" || sequence == 0 {
		return fmt.Errorf("multivenue: incomplete rendered evidence record")
	}
	word := (sequence - 1) / 64
	bit := uint64(1) << ((sequence - 1) % 64)
	words := v.bits[key.venue]
	if uint64(len(words)) <= word {
		grown := make([]uint64, word+1+word/2)
		copy(grown, words)
		words = grown
		v.bits[key.venue] = words
	}
	if words[word]&bit != 0 {
		return fmt.Errorf("multivenue: duplicate venue sequence %s#%d", key.venue, sequence)
	}
	words[word] |= bit
	v.count[key.venue]++
	if sequence > v.highest[key.venue] {
		v.highest[key.venue] = sequence
	}
	return nil
}

func (v *venueSequences) validate() error {
	venues := make([]string, 0, len(v.count))
	for venue := range v.count {
		venues = append(venues, venue)
	}
	sort.Strings(venues)
	for _, venue := range venues {
		if v.count[venue] == v.highest[venue] {
			continue
		}
		words := v.bits[venue]
		for sequence := uint64(1); sequence <= v.highest[venue]; sequence++ {
			if words[(sequence-1)/64]&(uint64(1)<<((sequence-1)%64)) == 0 {
				return fmt.Errorf("multivenue: missing venue sequence %s#%d", venue, sequence)
			}
		}
	}
	return nil
}
