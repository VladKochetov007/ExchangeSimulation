package multivenue

import (
	"bufio"
	"container/heap"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// renderSidecarCursor keeps only the next LogEvidenceOnly record from one
// route. The binary stream is ordered per venue sequence, so a small heap of
// these cursors is enough to merge sidecars without retaining a whole run.
type renderSidecarCursor struct {
	key     renderRouteKey
	file    *os.File
	scanner *bufio.Scanner
	current renderRecord
	ready   bool
	done    bool
}

type renderSidecarHeap []*renderSidecarCursor

func (h renderSidecarHeap) Len() int { return len(h) }

func (h renderSidecarHeap) Less(left, right int) bool {
	leftRecord, rightRecord := h[left].current, h[right].current
	if leftRecord.sequence != rightRecord.sequence {
		return leftRecord.sequence < rightRecord.sequence
	}
	return h[left].key.route < h[right].key.route
}

func (h renderSidecarHeap) Swap(left, right int) { h[left], h[right] = h[right], h[left] }

func (h *renderSidecarHeap) Push(value any) { *h = append(*h, value.(*renderSidecarCursor)) }

func (h *renderSidecarHeap) Pop() any {
	items := *h
	last := len(items) - 1
	value := items[last]
	items[last] = nil
	*h = items[:last]
	return value
}

type renderSidecars struct {
	byVenue map[string]*renderSidecarHeap
	all     []*renderSidecarCursor
	digest  renderArtifactDigest
}

func openRenderSidecars(venuesDir string) (*renderSidecars, error) {
	sidecars := &renderSidecars{byVenue: make(map[string]*renderSidecarHeap)}
	venuesInfo, err := os.Lstat(venuesDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return sidecars, nil
		}
		return nil, fmt.Errorf("multivenue: inspect venue evidence: %w", err)
	}
	if venuesInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("multivenue: venue evidence directory is a symlink")
	}
	if !venuesInfo.IsDir() {
		return nil, fmt.Errorf("multivenue: venue evidence path is not a directory")
	}
	err = filepath.WalkDir(venuesDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("multivenue: reject symlink in venue evidence %q", path)
		}
		if entry.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		relative, err := filepath.Rel(venuesDir, path)
		if err != nil {
			return err
		}
		parts := splitRenderSidecarPath(relative)
		if len(parts) < 2 || parts[0] == "" {
			return fmt.Errorf("multivenue: sidecar path %q is not venue-qualified", relative)
		}
		venue, route := parts[0], strings.Join(parts[1:], "/")
		if err := validateRenderVenue(venue); err != nil {
			return fmt.Errorf("multivenue: sidecar %q: %w", relative, err)
		}
		if err := validateRoute(route); err != nil {
			return fmt.Errorf("multivenue: sidecar %q: %w", relative, err)
		}
		file, err := openRenderRegularFile(path, fmt.Sprintf("open sidecar %q", relative))
		if err != nil {
			return err
		}
		cursor := &renderSidecarCursor{
			key:     renderRouteKey{venue: venue, route: route},
			file:    file,
			scanner: bufio.NewScanner(file),
		}
		cursor.scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
		if err := cursor.advance(&sidecars.digest); err != nil {
			_ = file.Close()
			return err
		}
		sidecars.all = append(sidecars.all, cursor)
		if cursor.ready {
			sidecars.heapForVenue(venue)
			heap.Push(sidecars.byVenue[venue], cursor)
		}
		return nil
	})
	if err != nil {
		_ = sidecars.close()
		return nil, err
	}
	for _, venueHeap := range sidecars.byVenue {
		heap.Init(venueHeap)
	}
	return sidecars, nil
}

func splitRenderSidecarPath(relative string) []string {
	clean := filepath.ToSlash(filepath.Clean(relative))
	return strings.Split(clean, "/")
}

func validateRenderVenue(venue string) error {
	if venue == "" || venue == "." || venue == ".." || filepath.Base(filepath.FromSlash(venue)) != venue {
		return fmt.Errorf("unsafe venue %q", venue)
	}
	return nil
}

func (s *renderSidecars) heapForVenue(venue string) *renderSidecarHeap {
	venueHeap := s.byVenue[venue]
	if venueHeap == nil {
		venueHeap = &renderSidecarHeap{}
		s.byVenue[venue] = venueHeap
	}
	return venueHeap
}

func (c *renderSidecarCursor) advance(digest *renderArtifactDigest) error {
	if c.done {
		return nil
	}
	if !c.scanner.Scan() {
		if err := c.scanner.Err(); err != nil {
			return fmt.Errorf("multivenue: read sidecar %s/%s: %w", c.key.venue, c.key.route, err)
		}
		c.done = true
		c.ready = false
		return nil
	}
	raw := append([]byte(nil), c.scanner.Bytes()...)
	var event renderPersistedEvent
	if err := unmarshalRenderSidecar(raw, &event); err != nil {
		return fmt.Errorf("multivenue: sidecar %s/%s malformed JSON: %w", c.key.venue, c.key.route, err)
	}
	if event.Event == "" || event.Data.VenueID != c.key.venue || event.Data.Sequence == 0 || len(event.Data.Payload) == 0 {
		return fmt.Errorf("multivenue: sidecar %s/%s has incomplete persisted event", c.key.venue, c.key.route)
	}
	c.current = renderRecord{sequence: event.Data.Sequence, raw: raw}
	c.ready = true
	digest.add(raw)
	return nil
}

func unmarshalRenderSidecar(raw []byte, event *renderPersistedEvent) error {
	return json.Unmarshal(raw, event)
}

func (s *renderSidecars) top(venue string) *renderSidecarCursor {
	venueHeap := s.byVenue[venue]
	if venueHeap == nil || venueHeap.Len() == 0 {
		return nil
	}
	return (*venueHeap)[0]
}

func (s *renderSidecars) pop(venue string) (renderRouteKey, renderRecord, bool, error) {
	venueHeap := s.byVenue[venue]
	if venueHeap == nil || venueHeap.Len() == 0 {
		return renderRouteKey{}, renderRecord{}, false, nil
	}
	cursor := heap.Pop(venueHeap).(*renderSidecarCursor)
	key, record := cursor.key, cursor.current
	if err := cursor.advance(&s.digest); err != nil {
		return renderRouteKey{}, renderRecord{}, false, err
	}
	if cursor.ready {
		heap.Push(venueHeap, cursor)
	}
	return key, record, true, nil
}

func (s *renderSidecars) flushBefore(venue string, sequence uint64, output *renderOutput) error {
	for {
		cursor := s.top(venue)
		if cursor == nil || cursor.current.sequence >= sequence {
			return nil
		}
		key, record, ok, err := s.pop(venue)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		if err := output.append(key, record); err != nil {
			return err
		}
	}
}

func (s *renderSidecars) rejectDuplicate(venue string, sequence uint64) error {
	cursor := s.top(venue)
	if cursor != nil && cursor.current.sequence == sequence {
		return fmt.Errorf("multivenue: canonical reconstruction stream has duplicate venue sequence %s#%d", venue, sequence)
	}
	return nil
}

func (s *renderSidecars) flushAll(output *renderOutput) error {
	venues := make([]string, 0, len(s.byVenue))
	for venue := range s.byVenue {
		venues = append(venues, venue)
	}
	sort.Strings(venues)
	for _, venue := range venues {
		for s.top(venue) != nil {
			key, record, ok, err := s.pop(venue)
			if err != nil {
				return err
			}
			if !ok {
				break
			}
			if err := output.append(key, record); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *renderSidecars) close() error {
	var closeErr error
	for _, cursor := range s.all {
		if err := cursor.file.Close(); err != nil {
			closeErr = errors.Join(closeErr, err)
		}
	}
	return closeErr
}

type renderRouteOutput struct {
	file   *os.File
	writer *bufio.Writer
}

type renderOutput struct {
	outputDir    string
	stageDir     string
	routes       map[renderRouteKey]*renderRouteOutput
	nextVenueSeq map[string]uint64
	closed       bool
	committed    bool
}

func newRenderOutput(outputDir string) (*renderOutput, error) {
	stageDir, err := os.MkdirTemp(filepath.Dir(outputDir), "."+filepath.Base(outputDir)+"-render-")
	if err != nil {
		return nil, fmt.Errorf("multivenue: create render staging directory: %w", err)
	}
	return &renderOutput{
		outputDir:    outputDir,
		stageDir:     stageDir,
		routes:       make(map[renderRouteKey]*renderRouteOutput),
		nextVenueSeq: make(map[string]uint64),
	}, nil
}

func (o *renderOutput) append(key renderRouteKey, record renderRecord) error {
	if err := validateRenderVenue(key.venue); err != nil {
		return fmt.Errorf("multivenue: rendered route: %w", err)
	}
	if err := validateRoute(key.route); err != nil {
		return fmt.Errorf("multivenue: rendered route %s/%s: %w", key.venue, key.route, err)
	}
	expected := o.nextVenueSeq[key.venue]
	if expected == 0 {
		expected = 1
	}
	if record.sequence != expected {
		kind := "missing"
		if record.sequence < expected {
			kind = "duplicate or out-of-order"
		}
		return fmt.Errorf("multivenue: canonical reconstruction stream has %s venue sequence %s#%d (expected %d)", kind, key.venue, record.sequence, expected)
	}
	routeOutput, ok := o.routes[key]
	if !ok {
		path := filepath.Join(o.stageDir, "venues", key.venue, filepath.FromSlash(key.route))
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			if os.IsNotExist(err) {
				if mkdirErr := os.MkdirAll(filepath.Dir(path), 0755); mkdirErr != nil {
					return fmt.Errorf("multivenue: create rendered route directory %q: %w", key.route, mkdirErr)
				}
				file, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
			}
			if err != nil {
				return fmt.Errorf("multivenue: create rendered route %q: %w", key.route, err)
			}
		}
		routeOutput = &renderRouteOutput{file: file, writer: bufio.NewWriterSize(file, 64*1024)}
		o.routes[key] = routeOutput
	}
	if _, err := routeOutput.writer.Write(record.raw); err != nil {
		return fmt.Errorf("multivenue: write rendered route %q: %w", key.route, err)
	}
	if err := routeOutput.writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("multivenue: write rendered route newline %q: %w", key.route, err)
	}
	o.nextVenueSeq[key.venue] = expected + 1
	return nil
}

func (o *renderOutput) close() error {
	if o.closed {
		return nil
	}
	o.closed = true
	keys := make([]renderRouteKey, 0, len(o.routes))
	for key := range o.routes {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool {
		if keys[left].venue != keys[right].venue {
			return keys[left].venue < keys[right].venue
		}
		return keys[left].route < keys[right].route
	})
	var closeErr error
	for _, key := range keys {
		routeOutput := o.routes[key]
		if err := routeOutput.writer.Flush(); err != nil {
			closeErr = errors.Join(closeErr, err)
		}
		if err := routeOutput.file.Close(); err != nil {
			closeErr = errors.Join(closeErr, err)
		}
	}
	return closeErr
}

func (o *renderOutput) commit() error {
	if err := o.close(); err != nil {
		return err
	}
	if err := validateRenderOutputDirectory(o.outputDir); err != nil {
		return err
	}
	venuesDir := filepath.Join(o.stageDir, "venues")
	if _, err := os.Stat(venuesDir); errors.Is(err, os.ErrNotExist) {
		o.committed = true
		return nil
	} else if err != nil {
		return fmt.Errorf("multivenue: inspect rendered staging directory: %w", err)
	}
	destination := filepath.Join(o.outputDir, "venues")
	if _, err := os.Lstat(destination); err == nil {
		return fmt.Errorf("multivenue: refusing to replace rendered evidence destination")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("multivenue: inspect rendered evidence destination: %w", err)
	}
	if err := rejectRenderPathSymlinks(venuesDir); err != nil {
		return err
	}
	if err := renameRenderDirectoryNoReplace(venuesDir, destination); err != nil {
		return fmt.Errorf("multivenue: install rendered evidence: %w", err)
	}
	o.committed = true
	return nil
}

func (o *renderOutput) cleanup() {
	if o.committed {
		_ = os.Remove(o.stageDir)
		return
	}
	_ = o.close()
	_ = os.RemoveAll(o.stageDir)
}

func (o *renderOutput) routeCount() int { return len(o.routes) }
