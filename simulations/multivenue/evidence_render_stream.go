package multivenue

import (
	"bufio"
	"bytes"
	"container/heap"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// renderSidecarCursor keeps only the next LogEvidenceOnly record from one
// route. A small heap of these cursors is enough to merge sidecars without
// retaining a whole run.
type renderSidecarCursor struct {
	key                renderRouteKey
	file               *os.File
	scanner            *bufio.Scanner
	current            renderRecord
	lastGlobalSequence uint64
	ready              bool
	done               bool
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

type renderGlobalSidecarHeap []*renderSidecarCursor

func (h renderGlobalSidecarHeap) Len() int { return len(h) }

func (h renderGlobalSidecarHeap) Less(left, right int) bool {
	leftRecord, rightRecord := h[left].current, h[right].current
	if leftRecord.globalSequence != rightRecord.globalSequence {
		return leftRecord.globalSequence < rightRecord.globalSequence
	}
	if h[left].key.venue != h[right].key.venue {
		return h[left].key.venue < h[right].key.venue
	}
	return h[left].key.route < h[right].key.route
}

func (h renderGlobalSidecarHeap) Swap(left, right int) { h[left], h[right] = h[right], h[left] }

func (h *renderGlobalSidecarHeap) Push(value any) { *h = append(*h, value.(*renderSidecarCursor)) }

func (h *renderGlobalSidecarHeap) Pop() any {
	items := *h
	last := len(items) - 1
	value := items[last]
	items[last] = nil
	*h = items[:last]
	return value
}

type renderSidecars struct {
	byVenue        map[string]*renderSidecarHeap
	global         renderGlobalSidecarHeap
	globalOrdering bool
	all            []*renderSidecarCursor
	digest         renderArtifactDigest
}

func openRenderSidecars(venuesDir string, globalOrdering bool) (*renderSidecars, error) {
	sidecars := &renderSidecars{byVenue: make(map[string]*renderSidecarHeap), globalOrdering: globalOrdering}
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
		if err := cursor.advance(&sidecars.digest, globalOrdering); err != nil {
			_ = file.Close()
			return err
		}
		sidecars.all = append(sidecars.all, cursor)
		if cursor.ready {
			if globalOrdering {
				heap.Push(&sidecars.global, cursor)
			} else {
				sidecars.heapForVenue(venue)
				heap.Push(sidecars.byVenue[venue], cursor)
			}
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
	heap.Init(&sidecars.global)
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

func (c *renderSidecarCursor) advance(digest *renderArtifactDigest, globalOrdering bool) error {
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
	if globalOrdering {
		if bytes.Equal(bytes.TrimSpace(event.Data.Payload), []byte("null")) {
			return fmt.Errorf("multivenue: sidecar %s/%s has null payload in global binary evidence", c.key.venue, c.key.route)
		}
		if event.EventSeq == 0 || (c.lastGlobalSequence != 0 && event.EventSeq <= c.lastGlobalSequence) {
			return fmt.Errorf("multivenue: sidecar %s/%s has non-increasing global event sequence", c.key.venue, c.key.route)
		}
		c.lastGlobalSequence = event.EventSeq
	}
	c.current = renderRecord{sequence: event.Data.Sequence, globalSequence: event.EventSeq, raw: raw}
	c.ready = true
	digest.add(raw)
	return nil
}

func unmarshalRenderSidecar(raw []byte, event *renderPersistedEvent) error {
	objects, err := decodeStrictJSONObjects(bytes.NewReader(raw), "rendered sidecar")
	if err != nil {
		return err
	}
	if len(objects) != 1 {
		return fmt.Errorf("rendered sidecar has %d JSON values, want exactly 1", len(objects))
	}
	object := objects[0]
	if object == nil {
		return errors.New("rendered sidecar is not an object")
	}
	if err := requireExactRenderKeys(object, []string{"client_id", "data", "event", "sim_ts"}, "event_seq"); err != nil {
		return err
	}
	if _, err := exactUint64Field(object, "client_id"); err != nil {
		return err
	}
	if _, err := exactStringField(object, "event"); err != nil {
		return err
	}
	if _, err := exactInt64Field(object, "sim_ts"); err != nil {
		return err
	}
	if _, present := object["event_seq"]; present {
		if _, err := exactUint64Field(object, "event_seq"); err != nil {
			return err
		}
	}
	data, ok := object["data"].(map[string]any)
	if !ok {
		return errors.New("rendered sidecar data is not an object")
	}
	if err := requireExactRenderKeys(data, []string{"venue_id", "sequence", "payload"}); err != nil {
		return err
	}
	if _, err := exactStringField(data, "venue_id"); err != nil {
		return err
	}
	if _, err := exactUint64Field(data, "sequence"); err != nil {
		return err
	}
	return json.Unmarshal(raw, event)
}

func requireExactRenderKeys(object map[string]any, required []string, optional ...string) error {
	allowedKeys := make(map[string]struct{}, len(required)+len(optional))
	for _, key := range required {
		if _, present := object[key]; !present {
			return fmt.Errorf("missing JSON field %q", key)
		}
		allowedKeys[key] = struct{}{}
	}
	for _, key := range optional {
		allowedKeys[key] = struct{}{}
	}
	for key := range object {
		if _, ok := allowedKeys[key]; !ok {
			return fmt.Errorf("unexpected JSON field %q", key)
		}
	}
	return nil
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
	if err := cursor.advance(&s.digest, s.globalOrdering); err != nil {
		return renderRouteKey{}, renderRecord{}, false, err
	}
	if cursor.ready {
		heap.Push(venueHeap, cursor)
	}
	return key, record, true, nil
}

func (s *renderSidecars) globalTop() *renderSidecarCursor {
	if len(s.global) == 0 {
		return nil
	}
	return s.global[0]
}

func (s *renderSidecars) popGlobal() (renderRouteKey, renderRecord, bool, error) {
	if len(s.global) == 0 {
		return renderRouteKey{}, renderRecord{}, false, nil
	}
	cursor := heap.Pop(&s.global).(*renderSidecarCursor)
	key, record := cursor.key, cursor.current
	if err := cursor.advance(&s.digest, s.globalOrdering); err != nil {
		return renderRouteKey{}, renderRecord{}, false, err
	}
	if cursor.ready {
		heap.Push(&s.global, cursor)
	}
	return key, record, true, nil
}

func (s *renderSidecars) flushGlobalBefore(sequence uint64, output *renderOutput) error {
	for {
		cursor := s.globalTop()
		if cursor == nil || cursor.current.globalSequence >= sequence {
			return nil
		}
		key, record, ok, err := s.popGlobal()
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

func (s *renderSidecars) flushGlobalAll(output *renderOutput) error {
	for s.globalTop() != nil {
		key, record, ok, err := s.popGlobal()
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
	return nil
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
	file       *os.File
	writer     *bufio.Writer
	compressor io.WriteCloser
}

type renderOutput struct {
	outputDir        string
	stageDir         string
	routeCompression RouteCompression
	globalOrdering   bool
	routes           map[renderRouteKey]*renderRouteOutput
	nextVenueSeq     map[string]uint64
	nextGlobalSeq    uint64
	fullEvidence     renderCanonicalDigest
	closed           bool
	committed        bool
}

func newRenderOutput(outputDir string, routeCompression RouteCompression, globalOrdering bool) (*renderOutput, error) {
	stageDir, err := os.MkdirTemp(filepath.Dir(outputDir), "."+filepath.Base(outputDir)+"-render-")
	if err != nil {
		return nil, fmt.Errorf("multivenue: create render staging directory: %w", err)
	}
	return &renderOutput{
		outputDir:        outputDir,
		stageDir:         stageDir,
		routeCompression: routeCompression,
		globalOrdering:   globalOrdering,
		routes:           make(map[renderRouteKey]*renderRouteOutput),
		nextVenueSeq:     make(map[string]uint64),
		fullEvidence:     newRenderCanonicalDigest(),
	}, nil
}

func (o *renderOutput) routePath(key renderRouteKey) string {
	route := filepath.FromSlash(key.route)
	if o.routeCompression == RouteCompressionZstd {
		route += ".zst"
	}
	return filepath.Join(o.stageDir, "venues", key.venue, route)
}

func newRenderRouteOutput(path string, compression RouteCompression) (*renderRouteOutput, error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return nil, err
	}
	output := &renderRouteOutput{file: file}
	if compression == RouteCompressionZstd {
		compressor, compressorErr := zstd.NewWriter(file,
			zstd.WithEncoderLevel(zstd.SpeedFastest),
			zstd.WithEncoderConcurrency(1),
		)
		if compressorErr != nil {
			_ = file.Close()
			return nil, compressorErr
		}
		output.compressor = compressor
		output.writer = bufio.NewWriterSize(compressor, 64*1024)
		return output, nil
	}
	output.writer = bufio.NewWriterSize(file, 64*1024)
	return output, nil
}

func (o *renderOutput) append(key renderRouteKey, record renderRecord) error {
	if err := validateRenderVenue(key.venue); err != nil {
		return fmt.Errorf("multivenue: rendered route: %w", err)
	}
	if err := validateRoute(key.route); err != nil {
		return fmt.Errorf("multivenue: rendered route %s/%s: %w", key.venue, key.route, err)
	}
	if o.globalOrdering {
		if record.globalSequence == 0 {
			return fmt.Errorf("multivenue: canonical reconstruction stream has zero global sequence for %s/%s", key.venue, key.route)
		}
		expectedGlobal := o.nextGlobalSeq + 1
		if record.globalSequence != expectedGlobal {
			kind := "missing"
			if record.globalSequence < expectedGlobal {
				kind = "duplicate or out-of-order"
			}
			return fmt.Errorf("multivenue: canonical reconstruction stream has %s global sequence %d (expected %d)", kind, record.globalSequence, expectedGlobal)
		}
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
		path := o.routePath(key)
		var err error
		routeOutput, err = newRenderRouteOutput(path, o.routeCompression)
		if err != nil {
			if os.IsNotExist(err) {
				if mkdirErr := os.MkdirAll(filepath.Dir(path), 0755); mkdirErr != nil {
					return fmt.Errorf("multivenue: create rendered route directory %q: %w", key.route, mkdirErr)
				}
				routeOutput, err = newRenderRouteOutput(path, o.routeCompression)
			}
			if err != nil {
				return fmt.Errorf("multivenue: create rendered route %q: %w", key.route, err)
			}
		}
		o.routes[key] = routeOutput
	}
	if _, err := routeOutput.writer.Write(record.raw); err != nil {
		return fmt.Errorf("multivenue: write rendered route %q: %w", key.route, err)
	}
	if err := routeOutput.writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("multivenue: write rendered route newline %q: %w", key.route, err)
	}
	o.nextVenueSeq[key.venue] = expected + 1
	if o.globalOrdering {
		o.nextGlobalSeq = record.globalSequence
		o.fullEvidence.add(key, record)
	}
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
		if routeOutput.compressor != nil {
			if err := routeOutput.compressor.Close(); err != nil {
				closeErr = errors.Join(closeErr, err)
			}
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

func (o *renderOutput) fullEvidenceHash() string {
	if !o.globalOrdering {
		return ""
	}
	return o.fullEvidence.hex()
}
