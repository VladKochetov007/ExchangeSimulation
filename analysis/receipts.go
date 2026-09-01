package analysis

import (
	"bufio"
	"bytes"
	"container/heap"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"
)

const (
	marketDataScheduleRecordBytes = 88
	marketDataReceiptRecordBytes  = 88
	marketDataDecisionRecordBytes = 96
)

// MarketDataReceiptAudit is an independent V2-0 information-boundary audit.
// It does not import simulation or its writer: it decodes raw sidecars and
// reconciles courier admission, actual inbox receipt, and order decisions.
type MarketDataReceiptAudit struct {
	SchemaVersion int    `json:"schema_version"`
	Domain        string `json:"domain"`
	Ordering      string `json:"ordering"`
	TerminalAt    int64  `json:"terminal_at"`

	Schedules int64 `json:"schedules"`
	Receipts  int64 `json:"receipts"`
	Decisions int64 `json:"decisions"`

	ScheduleDigestMatches bool `json:"schedule_digest_matches"`
	ReceiptDigestMatches  bool `json:"receipt_digest_matches"`
	DecisionDigestMatches bool `json:"decision_digest_matches"`

	UnknownLinkID          int64 `json:"unknown_link_id"`
	UnknownSymbolID        int64 `json:"unknown_symbol_id"`
	UnknownType            int64 `json:"unknown_type"`
	NonzeroReserved        int64 `json:"nonzero_reserved"`
	ScheduledBeforePub     int64 `json:"scheduled_before_publication"`
	DeliveredBeforePlan    int64 `json:"delivered_before_scheduled"`
	BadScheduleOrdinal     int64 `json:"bad_schedule_ordinal"`
	BadReceiptOrdinal      int64 `json:"bad_receipt_ordinal"`
	DuplicateSource        int64 `json:"duplicate_source_identity"`
	ReceiptWithoutSchedule int64 `json:"receipt_without_schedule"`
	ScheduleMismatch       int64 `json:"schedule_receipt_mismatch"`
	MissingDueReceipt      int64 `json:"missing_due_receipt"`
	BadEventOrder          int64 `json:"bad_global_event_order"`
	DecisionWithoutLink    int64 `json:"decision_without_link"`
	BadDecisionFrontier    int64 `json:"bad_decision_frontier"`
	FutureDecisionUse      int64 `json:"future_decision_use"`

	// LinkActivity makes the declared feed topology testable as activity rather
	// than as configuration alone. A catalog entry with zero receipts is an
	// unactivated information path, not proof that a cache received data.
	LinkActivity []MarketDataLinkActivity `json:"link_activity"`

	Valid bool `json:"valid"`
}

// MarketDataLinkActivity is independent evidence for one declared public-feed
// link. It intentionally counts scalar decisions separately: a feed-only
// remote link should receive observations but never originate an order.
type MarketDataLinkActivity struct {
	LinkID      uint32 `json:"link_id"`
	SourceVenue string `json:"source_venue"`
	Link        string `json:"link"`
	Role        string `json:"role"`
	Schedules   int64  `json:"schedules"`
	Receipts    int64  `json:"receipts"`
	Decisions   int64  `json:"decisions"`
}

type marketDataEvidenceManifest struct {
	SchemaVersion int    `json:"schema_version"`
	Domain        string `json:"domain"`
	Ordering      string `json:"ordering"`
	TerminalAt    int64  `json:"terminal_at"`
	Schedules     struct {
		File    string `json:"file"`
		Records int64  `json:"records"`
		Digest  string `json:"digest"`
	} `json:"schedules"`
	Receipts struct {
		File    string `json:"file"`
		Records int64  `json:"records"`
		Digest  string `json:"digest"`
	} `json:"receipts"`
	Decisions struct {
		File    string `json:"file"`
		Records int64  `json:"records"`
		Digest  string `json:"digest"`
	} `json:"decisions"`
	Links []struct {
		ID          uint32 `json:"id"`
		SourceVenue string `json:"source_venue"`
		Link        string `json:"link"`
		Role        string `json:"role"`
	} `json:"links"`
	Symbols []struct {
		ID     uint32 `json:"id"`
		Symbol string `json:"symbol"`
	} `json:"symbols"`
}

type observationRecord struct {
	clientID     uint64
	linkID       uint32
	symbolID     uint32
	mdType       uint8
	sequence     uint64
	fingerprint  [16]byte
	publishedAt  int64
	scheduledAt  int64
	deliveredAt  int64
	ordinal      uint64
	eventOrdinal uint64
	raw          []byte
}

type decisionRecord struct {
	clientID            uint64
	linkID              uint32
	symbolID            uint32
	side                uint8
	orderType           uint8
	tif                 uint8
	requestID           uint64
	decisionAt          int64
	frontierOrdinal     uint64
	frontierDeliveredAt int64
	frontierDigest      [16]byte
	price               int64
	qty                 int64
	eventOrdinal        uint64
}

type eventKind uint8

const (
	eventSchedule eventKind = iota
	eventReceipt
	eventDecision
)

type informationEvent struct {
	ordinal     uint64
	kind        eventKind
	observation observationRecord
	decision    decisionRecord
}

type linkKey struct {
	clientID uint64
	linkID   uint32
}

type scheduleKey struct {
	linkKey
	ordinal uint64
}

type sourceKey struct {
	linkKey
	sequence    uint64
	fingerprint [16]byte
}

type auditedFrontier struct {
	ordinal     uint64
	deliveredAt int64
	digest      [16]byte
}

// AuditMarketDataReceipts independently validates the complete V2-0 evidence
// contract. Matching a file checksum is insufficient: all mutations below
// deliberately remain detectable even when their attacker recomputes digests.
func AuditMarketDataReceipts(dir string) (*MarketDataReceiptAudit, error) {
	return auditMarketDataReceiptsStreaming(dir)
}

// auditMarketDataReceiptsBuffered is retained temporarily as a review oracle
// while the production path moves to bounded streaming. It is not called by
// the analyzer; keeping it during the transition makes semantic comparison
// against the prior implementation straightforward in tests and review.
func auditMarketDataReceiptsBuffered(dir string) (*MarketDataReceiptAudit, error) {
	manifestRaw, err := os.ReadFile(filepath.Join(dir, "market-data-evidence-v2.json"))
	if err != nil {
		return nil, fmt.Errorf("read market-data evidence manifest: %w", err)
	}
	var manifest marketDataEvidenceManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return nil, fmt.Errorf("decode market-data evidence manifest: %w", err)
	}
	if manifest.SchemaVersion != 2 || manifest.Domain != "participant_information_boundary_v2" ||
		manifest.Ordering != "per_link_fifo_schedule_receipt_decision" {
		return nil, fmt.Errorf("unsupported market-data evidence contract: schema=%d domain=%q ordering=%q", manifest.SchemaVersion, manifest.Domain, manifest.Ordering)
	}
	if manifest.Schedules.File != "market-data-schedules-v2.bin" || manifest.Receipts.File != "market-data-receipts-v2.bin" || manifest.Decisions.File != "market-data-decisions-v2.bin" {
		return nil, fmt.Errorf("unsupported market-data evidence file names")
	}
	schedulesRaw, schedulesDigest, err := readEvidenceFile(dir, manifest.Schedules.File, marketDataScheduleRecordBytes, manifest.Schedules.Records, manifest.Schedules.Digest)
	if err != nil {
		return nil, err
	}
	receiptsRaw, receiptsDigest, err := readEvidenceFile(dir, manifest.Receipts.File, marketDataReceiptRecordBytes, manifest.Receipts.Records, manifest.Receipts.Digest)
	if err != nil {
		return nil, err
	}
	decisionsRaw, decisionsDigest, err := readEvidenceFile(dir, manifest.Decisions.File, marketDataDecisionRecordBytes, manifest.Decisions.Records, manifest.Decisions.Digest)
	if err != nil {
		return nil, err
	}
	links, symbols, err := validateEvidenceCatalog(manifest)
	if err != nil {
		return nil, err
	}
	result := &MarketDataReceiptAudit{
		SchemaVersion:         manifest.SchemaVersion,
		Domain:                manifest.Domain,
		Ordering:              manifest.Ordering,
		TerminalAt:            manifest.TerminalAt,
		Schedules:             manifest.Schedules.Records,
		Receipts:              manifest.Receipts.Records,
		Decisions:             manifest.Decisions.Records,
		ScheduleDigestMatches: schedulesDigest,
		ReceiptDigestMatches:  receiptsDigest,
		DecisionDigestMatches: decisionsDigest,
	}
	linkActivity := make(map[uint32]*MarketDataLinkActivity, len(manifest.Links))
	for _, row := range manifest.Links {
		linkActivity[row.ID] = &MarketDataLinkActivity{
			LinkID: row.ID, SourceVenue: row.SourceVenue, Link: row.Link, Role: row.Role,
		}
	}

	events := make([]informationEvent, 0, result.Schedules+result.Receipts+result.Decisions)
	for offset := 0; offset < len(schedulesRaw); offset += marketDataScheduleRecordBytes {
		record := decodeObservation(schedulesRaw[offset : offset+marketDataScheduleRecordBytes])
		validateObservation(result, record, links, symbols, false)
		if activity := linkActivity[record.linkID]; activity != nil {
			activity.Schedules++
		}
		events = append(events, informationEvent{ordinal: record.eventOrdinal, kind: eventSchedule, observation: record})
	}
	for offset := 0; offset < len(receiptsRaw); offset += marketDataReceiptRecordBytes {
		record := decodeObservation(receiptsRaw[offset : offset+marketDataReceiptRecordBytes])
		validateObservation(result, record, links, symbols, true)
		if activity := linkActivity[record.linkID]; activity != nil {
			activity.Receipts++
		}
		events = append(events, informationEvent{ordinal: record.eventOrdinal, kind: eventReceipt, observation: record})
	}
	for offset := 0; offset < len(decisionsRaw); offset += marketDataDecisionRecordBytes {
		record := decodeDecision(decisionsRaw[offset : offset+marketDataDecisionRecordBytes])
		if _, ok := links[record.linkID]; !ok {
			result.DecisionWithoutLink++
		}
		if _, ok := symbols[record.symbolID]; !ok || record.symbolID == 0 {
			result.UnknownSymbolID++
		}
		if record.side > 1 || record.orderType > 1 || record.tif > 2 {
			result.BadDecisionFrontier++
		}
		if activity := linkActivity[record.linkID]; activity != nil {
			activity.Decisions++
		}
		if decisionsRaw[offset+19] > 1 {
			result.NonzeroReserved++
		}
		for _, value := range decisionsRaw[offset+20 : offset+24] {
			if value != 0 {
				result.NonzeroReserved++
				break
			}
		}
		events = append(events, informationEvent{ordinal: record.eventOrdinal, kind: eventDecision, decision: record})
	}

	sort.Slice(events, func(i, j int) bool { return events[i].ordinal < events[j].ordinal })
	schedules := make(map[scheduleKey]observationRecord, result.Schedules)
	sources := make(map[sourceKey]struct{}, result.Schedules)
	frontiers := make(map[linkKey]auditedFrontier)
	lastSchedule := make(map[linkKey]uint64)
	lastReceipt := make(map[linkKey]uint64)
	lastEvent := uint64(0)
	for _, event := range events {
		if event.ordinal == 0 || event.ordinal != lastEvent+1 {
			result.BadEventOrder++
		}
		if event.ordinal > lastEvent {
			lastEvent = event.ordinal
		}
		switch event.kind {
		case eventSchedule:
			record := event.observation
			key := linkKey{record.clientID, record.linkID}
			if record.ordinal != lastSchedule[key]+1 {
				result.BadScheduleOrdinal++
			}
			lastSchedule[key] = record.ordinal
			scheduleKey := scheduleKey{linkKey: key, ordinal: record.ordinal}
			if _, exists := schedules[scheduleKey]; exists {
				result.BadScheduleOrdinal++
			}
			schedules[scheduleKey] = record
			sourceKey := sourceKey{linkKey: key, sequence: record.sequence, fingerprint: record.fingerprint}
			if _, exists := sources[sourceKey]; exists {
				result.DuplicateSource++
			}
			sources[sourceKey] = struct{}{}
		case eventReceipt:
			record := event.observation
			key := linkKey{record.clientID, record.linkID}
			if record.ordinal != lastReceipt[key]+1 {
				result.BadReceiptOrdinal++
			}
			lastReceipt[key] = record.ordinal
			schedule, exists := schedules[scheduleKey{linkKey: key, ordinal: record.ordinal}]
			if !exists {
				result.ReceiptWithoutSchedule++
			} else if !sameObservation(schedule, record) {
				result.ScheduleMismatch++
			}
			previous := frontiers[key]
			chain := sha256.New()
			_, _ = chain.Write(previous.digest[:])
			_, _ = chain.Write(record.raw)
			var digest [16]byte
			copy(digest[:], chain.Sum(nil))
			frontiers[key] = auditedFrontier{ordinal: record.ordinal, deliveredAt: record.deliveredAt, digest: digest}
		case eventDecision:
			record := event.decision
			key := linkKey{record.clientID, record.linkID}
			frontier := frontiers[key]
			if frontier.ordinal != record.frontierOrdinal || frontier.deliveredAt != record.frontierDeliveredAt || frontier.digest != record.frontierDigest {
				result.BadDecisionFrontier++
			}
			if record.frontierOrdinal > 0 && record.frontierDeliveredAt > record.decisionAt {
				result.FutureDecisionUse++
			}
		}
	}
	for key, schedule := range schedules {
		if schedule.scheduledAt <= manifest.TerminalAt && lastReceipt[key.linkKey] < key.ordinal {
			result.MissingDueReceipt++
		}
	}
	result.LinkActivity = make([]MarketDataLinkActivity, 0, len(linkActivity))
	for _, activity := range linkActivity {
		result.LinkActivity = append(result.LinkActivity, *activity)
	}
	sort.Slice(result.LinkActivity, func(i, j int) bool {
		return result.LinkActivity[i].LinkID < result.LinkActivity[j].LinkID
	})
	result.Valid = result.ScheduleDigestMatches && result.ReceiptDigestMatches && result.DecisionDigestMatches &&
		result.UnknownLinkID == 0 && result.UnknownSymbolID == 0 && result.UnknownType == 0 && result.NonzeroReserved == 0 &&
		result.ScheduledBeforePub == 0 && result.DeliveredBeforePlan == 0 && result.BadScheduleOrdinal == 0 &&
		result.BadReceiptOrdinal == 0 && result.DuplicateSource == 0 && result.ReceiptWithoutSchedule == 0 &&
		result.ScheduleMismatch == 0 && result.MissingDueReceipt == 0 && result.BadEventOrder == 0 &&
		result.DecisionWithoutLink == 0 && result.BadDecisionFrontier == 0 && result.FutureDecisionUse == 0
	return result, nil
}

// evidenceRecordStream reads one fixed-width sidecar without materializing the
// file. The recorder appends each sidecar in the same global event order, so
// three streams can be merged while retaining the exact semantic checks.
type evidenceRecordStream struct {
	file        *os.File
	path        string
	recordBytes int
	expected    int64
	read        int64
	wantDigest  string
	hasher      hash.Hash
	record      []byte
	finished    bool
	digestMatch bool
}

func openEvidenceRecordStream(dir, name string, recordBytes int, records int64, wantDigest string) (*evidenceRecordStream, error) {
	if recordBytes <= 0 || records < 0 || len(wantDigest) != 64 {
		return nil, fmt.Errorf("invalid evidence stream contract for %s", name)
	}
	path := filepath.Join(dir, name)
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open market-data evidence %s: %w", name, err)
	}
	return &evidenceRecordStream{
		file: file, path: path, recordBytes: recordBytes, expected: records,
		wantDigest: wantDigest, hasher: sha256.New(), record: make([]byte, recordBytes), digestMatch: true,
	}, nil
}

func (s *evidenceRecordStream) close() error {
	return s.file.Close()
}

func (s *evidenceRecordStream) next() (bool, error) {
	if s.finished {
		return false, nil
	}
	if s.read == s.expected {
		var extra [1]byte
		n, err := s.file.Read(extra[:])
		if n != 0 {
			return false, fmt.Errorf("market-data evidence %s has records beyond manifest count", s.path)
		}
		if err != io.EOF {
			if err == nil {
				return false, fmt.Errorf("market-data evidence %s did not reach EOF", s.path)
			}
			return false, fmt.Errorf("read market-data evidence %s: %w", s.path, err)
		}
		got := hex.EncodeToString(s.hasher.Sum(nil))
		s.digestMatch = got == s.wantDigest
		s.finished = true
		return false, nil
	}
	n, err := io.ReadFull(s.file, s.record)
	if err != nil {
		return false, fmt.Errorf("read market-data evidence %s record %d: %w", s.path, s.read+1, err)
	}
	if n != s.recordBytes {
		return false, fmt.Errorf("short market-data evidence %s record %d", s.path, s.read+1)
	}
	if _, err := s.hasher.Write(s.record); err != nil {
		return false, fmt.Errorf("hash market-data evidence %s: %w", s.path, err)
	}
	s.read++
	return true, nil
}

type evidenceEventStream struct {
	kind         eventKind
	records      *evidenceRecordStream
	available    bool
	eventOrdinal uint64
}

type scheduleStoreFile struct {
	file   *os.File
	writer *bufio.Writer
	count  uint64
}

type scheduleStore struct {
	dir   string
	files map[linkKey]*scheduleStoreFile
}

func newScheduleStore(dir string) *scheduleStore {
	return &scheduleStore{dir: dir, files: make(map[linkKey]*scheduleStoreFile)}
}

func (s *scheduleStore) fileFor(key linkKey) (*scheduleStoreFile, error) {
	if stored, ok := s.files[key]; ok {
		return stored, nil
	}
	path := filepath.Join(s.dir, fmt.Sprintf("schedule-%d-%d.bin", key.clientID, key.linkID))
	file, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create schedule spill %s: %w", path, err)
	}
	stored := &scheduleStoreFile{file: file, writer: bufio.NewWriterSize(file, 1<<20)}
	s.files[key] = stored
	return stored, nil
}

func (s *scheduleStore) append(key linkKey, raw []byte) error {
	stored, err := s.fileFor(key)
	if err != nil {
		return err
	}
	if _, err := stored.writer.Write(raw); err != nil {
		return fmt.Errorf("spill schedule client %d link %d: %w", key.clientID, key.linkID, err)
	}
	stored.count++
	return nil
}

func (s *scheduleStore) read(key linkKey, ordinal uint64) ([]byte, bool, error) {
	stored, ok := s.files[key]
	if !ok || ordinal == 0 || ordinal > stored.count {
		return nil, false, nil
	}
	if err := stored.writer.Flush(); err != nil {
		return nil, false, fmt.Errorf("flush schedule client %d link %d: %w", key.clientID, key.linkID, err)
	}
	if ordinal > uint64(^uint64(0)>>1)/marketDataScheduleRecordBytes {
		return nil, false, nil
	}
	raw := make([]byte, marketDataScheduleRecordBytes)
	offset := int64((ordinal - 1) * marketDataScheduleRecordBytes)
	read, err := stored.file.ReadAt(raw, offset)
	if err != nil {
		return nil, false, fmt.Errorf("read spilled schedule client %d link %d ordinal %d: %w", key.clientID, key.linkID, ordinal, err)
	}
	if read != len(raw) {
		return nil, false, fmt.Errorf("short spilled schedule client %d link %d ordinal %d", key.clientID, key.linkID, ordinal)
	}
	return raw, true, nil
}

func (s *scheduleStore) missingDue(terminalAt int64, lastReceipt map[linkKey]uint64) (int64, error) {
	var missing int64
	for key, stored := range s.files {
		if err := stored.writer.Flush(); err != nil {
			return 0, fmt.Errorf("flush schedule client %d link %d: %w", key.clientID, key.linkID, err)
		}
		if _, err := stored.file.Seek(0, io.SeekStart); err != nil {
			return 0, fmt.Errorf("seek spilled schedules client %d link %d: %w", key.clientID, key.linkID, err)
		}
		var raw [marketDataScheduleRecordBytes]byte
		for ordinal := uint64(1); ordinal <= stored.count; ordinal++ {
			if _, err := io.ReadFull(stored.file, raw[:]); err != nil {
				return 0, fmt.Errorf("read spilled schedules client %d link %d: %w", key.clientID, key.linkID, err)
			}
			record := decodeObservationView(raw[:])
			if record.scheduledAt <= terminalAt && lastReceipt[key] < ordinal {
				missing++
			}
		}
	}
	return missing, nil
}

func (s *scheduleStore) close() error {
	var first error
	for key, stored := range s.files {
		if err := stored.writer.Flush(); err != nil && first == nil {
			first = fmt.Errorf("flush schedule client %d link %d: %w", key.clientID, key.linkID, err)
		}
		if err := stored.file.Close(); err != nil && first == nil {
			first = fmt.Errorf("close schedule client %d link %d: %w", key.clientID, key.linkID, err)
		}
	}
	return first
}

type sourceIdentity struct {
	clientID    uint64
	linkID      uint32
	sequence    uint64
	fingerprint [16]byte
}

const sourceIdentityRecordBytes = 36
const sourceIdentityChunkSize = 1 << 20

func compareSourceIdentity(left, right sourceIdentity) int {
	if left.clientID < right.clientID {
		return -1
	}
	if left.clientID > right.clientID {
		return 1
	}
	if left.linkID < right.linkID {
		return -1
	}
	if left.linkID > right.linkID {
		return 1
	}
	if left.sequence < right.sequence {
		return -1
	}
	if left.sequence > right.sequence {
		return 1
	}
	return bytes.Compare(left.fingerprint[:], right.fingerprint[:])
}

func encodeSourceIdentity(raw []byte, identity sourceIdentity) {
	binary.BigEndian.PutUint64(raw[0:8], identity.clientID)
	binary.BigEndian.PutUint32(raw[8:12], identity.linkID)
	binary.BigEndian.PutUint64(raw[12:20], identity.sequence)
	copy(raw[20:36], identity.fingerprint[:])
}

func decodeSourceIdentity(raw []byte) sourceIdentity {
	var fingerprint [16]byte
	copy(fingerprint[:], raw[20:36])
	return sourceIdentity{
		clientID: binary.BigEndian.Uint64(raw[0:8]), linkID: binary.BigEndian.Uint32(raw[8:12]),
		sequence: binary.BigEndian.Uint64(raw[12:20]), fingerprint: fingerprint,
	}
}

type sourceRunManager struct {
	dir       string
	chunk     []sourceIdentity
	runPaths  []string
	nextRunID int
}

func newSourceRunManager(dir string) *sourceRunManager {
	return &sourceRunManager{dir: dir, chunk: make([]sourceIdentity, 0, sourceIdentityChunkSize)}
}

func (m *sourceRunManager) add(identity sourceIdentity) error {
	m.chunk = append(m.chunk, identity)
	if len(m.chunk) < sourceIdentityChunkSize {
		return nil
	}
	return m.flush()
}

func (m *sourceRunManager) flush() error {
	if len(m.chunk) == 0 {
		return nil
	}
	sort.Slice(m.chunk, func(i, j int) bool { return compareSourceIdentity(m.chunk[i], m.chunk[j]) < 0 })
	path := filepath.Join(m.dir, fmt.Sprintf("source-run-%06d.bin", m.nextRunID))
	m.nextRunID++
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create source duplicate run %s: %w", path, err)
	}
	var raw [sourceIdentityRecordBytes]byte
	for _, identity := range m.chunk {
		encodeSourceIdentity(raw[:], identity)
		if _, err := file.Write(raw[:]); err != nil {
			_ = file.Close()
			return fmt.Errorf("write source duplicate run %s: %w", path, err)
		}
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close source duplicate run %s: %w", path, err)
	}
	m.runPaths = append(m.runPaths, path)
	m.chunk = m.chunk[:0]
	return nil
}

type sourceRunReader struct {
	file      *os.File
	current   sourceIdentity
	available bool
}

func (r *sourceRunReader) next() error {
	var raw [sourceIdentityRecordBytes]byte
	read, err := io.ReadFull(r.file, raw[:])
	if err == io.EOF && read == 0 {
		r.available = false
		return nil
	}
	if err != nil {
		return fmt.Errorf("read source duplicate run: %w", err)
	}
	r.current = decodeSourceIdentity(raw[:])
	r.available = true
	return nil
}

type sourceRunHeap []*sourceRunReader

func (h sourceRunHeap) Len() int { return len(h) }
func (h sourceRunHeap) Less(i, j int) bool {
	return compareSourceIdentity(h[i].current, h[j].current) < 0
}
func (h sourceRunHeap) Swap(i, j int)   { h[i], h[j] = h[j], h[i] }
func (h *sourceRunHeap) Push(value any) { *h = append(*h, value.(*sourceRunReader)) }
func (h *sourceRunHeap) Pop() any {
	old := *h
	last := len(old) - 1
	value := old[last]
	*h = old[:last]
	return value
}

func (m *sourceRunManager) duplicateCount() (int64, error) {
	if err := m.flush(); err != nil {
		return 0, err
	}
	readers := make([]*sourceRunReader, 0, len(m.runPaths))
	for _, path := range m.runPaths {
		file, err := os.Open(path)
		if err != nil {
			return 0, fmt.Errorf("open source duplicate run %s: %w", path, err)
		}
		reader := &sourceRunReader{file: file}
		if err := reader.next(); err != nil {
			_ = file.Close()
			return 0, err
		}
		if reader.available {
			readers = append(readers, reader)
		} else {
			_ = file.Close()
		}
	}
	heapQueue := sourceRunHeap(readers)
	heap.Init(&heapQueue)
	var previous sourceIdentity
	havePrevious := false
	var duplicates int64
	for len(heapQueue) > 0 {
		reader := heap.Pop(&heapQueue).(*sourceRunReader)
		if havePrevious && compareSourceIdentity(previous, reader.current) == 0 {
			duplicates++
		}
		previous = reader.current
		havePrevious = true
		if err := reader.next(); err != nil {
			return 0, err
		}
		if reader.available {
			heap.Push(&heapQueue, reader)
		} else if err := reader.file.Close(); err != nil {
			return 0, fmt.Errorf("close source duplicate run: %w", err)
		}
	}
	return duplicates, nil
}

func (m *sourceRunManager) cleanup() {
	for _, path := range m.runPaths {
		_ = os.Remove(path)
	}
}

func (s *evidenceEventStream) advance() error {
	available, err := s.records.next()
	if err != nil {
		return err
	}
	s.available = available
	if !available {
		return nil
	}
	if s.kind == eventDecision {
		s.eventOrdinal = binary.BigEndian.Uint64(s.records.record[88:96])
	} else {
		s.eventOrdinal = binary.BigEndian.Uint64(s.records.record[76:84])
	}
	return nil
}

func auditMarketDataReceiptsStreaming(dir string) (*MarketDataReceiptAudit, error) {
	manifestRaw, err := os.ReadFile(filepath.Join(dir, "market-data-evidence-v2.json"))
	if err != nil {
		return nil, fmt.Errorf("read market-data evidence manifest: %w", err)
	}
	var manifest marketDataEvidenceManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return nil, fmt.Errorf("decode market-data evidence manifest: %w", err)
	}
	if manifest.SchemaVersion != 2 || manifest.Domain != "participant_information_boundary_v2" ||
		manifest.Ordering != "per_link_fifo_schedule_receipt_decision" {
		return nil, fmt.Errorf("unsupported market-data evidence contract: schema=%d domain=%q ordering=%q", manifest.SchemaVersion, manifest.Domain, manifest.Ordering)
	}
	if manifest.Schedules.File != "market-data-schedules-v2.bin" || manifest.Receipts.File != "market-data-receipts-v2.bin" || manifest.Decisions.File != "market-data-decisions-v2.bin" {
		return nil, fmt.Errorf("unsupported market-data evidence file names")
	}
	links, symbols, err := validateEvidenceCatalog(manifest)
	if err != nil {
		return nil, err
	}
	schedules, err := openEvidenceRecordStream(dir, manifest.Schedules.File, marketDataScheduleRecordBytes, manifest.Schedules.Records, manifest.Schedules.Digest)
	if err != nil {
		return nil, err
	}
	receipts, err := openEvidenceRecordStream(dir, manifest.Receipts.File, marketDataReceiptRecordBytes, manifest.Receipts.Records, manifest.Receipts.Digest)
	if err != nil {
		_ = schedules.close()
		return nil, err
	}
	decisions, err := openEvidenceRecordStream(dir, manifest.Decisions.File, marketDataDecisionRecordBytes, manifest.Decisions.Records, manifest.Decisions.Digest)
	if err != nil {
		_ = schedules.close()
		_ = receipts.close()
		return nil, err
	}
	defer schedules.close()
	defer receipts.close()
	defer decisions.close()

	result := &MarketDataReceiptAudit{
		SchemaVersion: manifest.SchemaVersion, Domain: manifest.Domain, Ordering: manifest.Ordering,
		TerminalAt: manifest.TerminalAt, Schedules: manifest.Schedules.Records,
		Receipts: manifest.Receipts.Records, Decisions: manifest.Decisions.Records,
		ScheduleDigestMatches: true, ReceiptDigestMatches: true, DecisionDigestMatches: true,
	}
	linkActivity := make(map[uint32]*MarketDataLinkActivity, len(manifest.Links))
	for _, row := range manifest.Links {
		linkActivity[row.ID] = &MarketDataLinkActivity{LinkID: row.ID, SourceVenue: row.SourceVenue, Link: row.Link, Role: row.Role}
	}
	spillDir, err := os.MkdirTemp("", "v2-market-data-audit-")
	if err != nil {
		return nil, fmt.Errorf("create market-data audit spill directory: %w", err)
	}
	defer os.RemoveAll(spillDir)
	scheduleSpill := newScheduleStore(spillDir)
	sourceRuns := newSourceRunManager(spillDir)
	defer sourceRuns.cleanup()
	defer scheduleSpill.close()

	events := []*evidenceEventStream{
		{kind: eventSchedule, records: schedules},
		{kind: eventReceipt, records: receipts},
		{kind: eventDecision, records: decisions},
	}
	for _, stream := range events {
		if err := stream.advance(); err != nil {
			return nil, err
		}
	}
	frontiers := make(map[linkKey]auditedFrontier)
	lastSchedule := make(map[linkKey]uint64)
	lastReceipt := make(map[linkKey]uint64)
	lastEvent := uint64(0)
	for {
		var selected *evidenceEventStream
		for _, stream := range events {
			if !stream.available || (selected != nil && stream.eventOrdinal >= selected.eventOrdinal) {
				continue
			}
			selected = stream
		}
		if selected == nil {
			break
		}
		raw := selected.records.record
		if selected.eventOrdinal == 0 || selected.eventOrdinal != lastEvent+1 {
			result.BadEventOrder++
		}
		if selected.eventOrdinal > lastEvent {
			lastEvent = selected.eventOrdinal
		}
		switch selected.kind {
		case eventSchedule:
			record := decodeObservationView(raw)
			record.raw = raw
			validateObservation(result, record, links, symbols, false)
			record.raw = nil
			if activity := linkActivity[record.linkID]; activity != nil {
				activity.Schedules++
			}
			key := linkKey{record.clientID, record.linkID}
			if record.ordinal != lastSchedule[key]+1 {
				result.BadScheduleOrdinal++
			}
			if record.ordinal == lastSchedule[key]+1 {
				if err := scheduleSpill.append(key, raw); err != nil {
					return nil, err
				}
			}
			lastSchedule[key] = record.ordinal
			if err := sourceRuns.add(sourceIdentity{clientID: record.clientID, linkID: record.linkID, sequence: record.sequence, fingerprint: record.fingerprint}); err != nil {
				return nil, err
			}
		case eventReceipt:
			record := decodeObservationView(raw)
			record.raw = raw
			validateObservation(result, record, links, symbols, true)
			record.raw = nil
			if activity := linkActivity[record.linkID]; activity != nil {
				activity.Receipts++
			}
			key := linkKey{record.clientID, record.linkID}
			if record.ordinal != lastReceipt[key]+1 {
				result.BadReceiptOrdinal++
			}
			lastReceipt[key] = record.ordinal
			scheduleRaw, exists, err := scheduleSpill.read(key, record.ordinal)
			if err != nil {
				return nil, err
			}
			if !exists {
				result.ReceiptWithoutSchedule++
			} else if !sameObservation(decodeObservationView(scheduleRaw), record) {
				result.ScheduleMismatch++
			}
			previous := frontiers[key]
			chain := sha256.New()
			_, _ = chain.Write(previous.digest[:])
			_, _ = chain.Write(raw)
			var digest [16]byte
			copy(digest[:], chain.Sum(nil))
			frontiers[key] = auditedFrontier{ordinal: record.ordinal, deliveredAt: record.deliveredAt, digest: digest}
		case eventDecision:
			record := decodeDecision(raw)
			if _, exists := links[record.linkID]; !exists {
				result.DecisionWithoutLink++
			}
			if _, exists := symbols[record.symbolID]; !exists || record.symbolID == 0 {
				result.UnknownSymbolID++
			}
			if record.side > 1 || record.orderType > 1 || record.tif > 2 {
				result.BadDecisionFrontier++
			}
			if activity := linkActivity[record.linkID]; activity != nil {
				activity.Decisions++
			}
			if raw[19] > 1 {
				result.NonzeroReserved++
			}
			for _, value := range raw[20:24] {
				if value != 0 {
					result.NonzeroReserved++
					break
				}
			}
			frontier := frontiers[linkKey{record.clientID, record.linkID}]
			if frontier.ordinal != record.frontierOrdinal || frontier.deliveredAt != record.frontierDeliveredAt || frontier.digest != record.frontierDigest {
				result.BadDecisionFrontier++
			}
			if record.frontierOrdinal > 0 && record.frontierDeliveredAt > record.decisionAt {
				result.FutureDecisionUse++
			}
		}
		if err := selected.advance(); err != nil {
			return nil, err
		}
	}
	result.ScheduleDigestMatches = schedules.digestMatch
	result.ReceiptDigestMatches = receipts.digestMatch
	result.DecisionDigestMatches = decisions.digestMatch
	missingDue, err := scheduleSpill.missingDue(manifest.TerminalAt, lastReceipt)
	if err != nil {
		return nil, err
	}
	result.MissingDueReceipt = missingDue
	duplicates, err := sourceRuns.duplicateCount()
	if err != nil {
		return nil, err
	}
	result.DuplicateSource += duplicates
	result.LinkActivity = make([]MarketDataLinkActivity, 0, len(linkActivity))
	for _, activity := range linkActivity {
		result.LinkActivity = append(result.LinkActivity, *activity)
	}
	sort.Slice(result.LinkActivity, func(i, j int) bool { return result.LinkActivity[i].LinkID < result.LinkActivity[j].LinkID })
	result.Valid = result.ScheduleDigestMatches && result.ReceiptDigestMatches && result.DecisionDigestMatches &&
		result.UnknownLinkID == 0 && result.UnknownSymbolID == 0 && result.UnknownType == 0 && result.NonzeroReserved == 0 &&
		result.ScheduledBeforePub == 0 && result.DeliveredBeforePlan == 0 && result.BadScheduleOrdinal == 0 &&
		result.BadReceiptOrdinal == 0 && result.DuplicateSource == 0 && result.ReceiptWithoutSchedule == 0 &&
		result.ScheduleMismatch == 0 && result.MissingDueReceipt == 0 && result.BadEventOrder == 0 &&
		result.DecisionWithoutLink == 0 && result.BadDecisionFrontier == 0 && result.FutureDecisionUse == 0
	return result, nil
}

func readEvidenceFile(dir, name string, recordBytes int, records int64, wantDigest string) ([]byte, bool, error) {
	if len(wantDigest) != 64 {
		return nil, false, fmt.Errorf("invalid evidence digest for %s", name)
	}
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return nil, false, fmt.Errorf("read market-data evidence %s: %w", name, err)
	}
	if len(raw)%recordBytes != 0 || int64(len(raw)/recordBytes) != records {
		return nil, false, fmt.Errorf("market-data evidence %s record count disagrees with manifest", name)
	}
	digest := sha256.Sum256(raw)
	return raw, wantDigest == hex.EncodeToString(digest[:]), nil
}

func validateEvidenceCatalog(manifest marketDataEvidenceManifest) (map[uint32]struct{}, map[uint32]struct{}, error) {
	links := make(map[uint32]struct{}, len(manifest.Links))
	for _, row := range manifest.Links {
		if row.ID == 0 || row.SourceVenue == "" || row.Link == "" || row.Role == "" {
			return nil, nil, fmt.Errorf("invalid market-data link catalog row")
		}
		if _, exists := links[row.ID]; exists {
			return nil, nil, fmt.Errorf("duplicate market-data link catalog ID %d", row.ID)
		}
		links[row.ID] = struct{}{}
	}
	symbols := make(map[uint32]struct{}, len(manifest.Symbols))
	for _, row := range manifest.Symbols {
		if row.ID == 0 || row.Symbol == "" {
			return nil, nil, fmt.Errorf("invalid market-data symbol catalog row")
		}
		if _, exists := symbols[row.ID]; exists {
			return nil, nil, fmt.Errorf("duplicate market-data symbol catalog ID %d", row.ID)
		}
		symbols[row.ID] = struct{}{}
	}
	return links, symbols, nil
}

func decodeObservation(raw []byte) observationRecord {
	record := decodeObservationView(raw)
	record.raw = append([]byte(nil), raw...)
	return record
}

func decodeObservationView(raw []byte) observationRecord {
	var fingerprint [16]byte
	copy(fingerprint[:], raw[28:44])
	return observationRecord{
		clientID:     binary.BigEndian.Uint64(raw[0:8]),
		linkID:       binary.BigEndian.Uint32(raw[8:12]),
		symbolID:     binary.BigEndian.Uint32(raw[12:16]),
		mdType:       raw[16],
		sequence:     binary.BigEndian.Uint64(raw[20:28]),
		fingerprint:  fingerprint,
		publishedAt:  int64(binary.BigEndian.Uint64(raw[44:52])),
		scheduledAt:  int64(binary.BigEndian.Uint64(raw[52:60])),
		deliveredAt:  int64(binary.BigEndian.Uint64(raw[60:68])),
		ordinal:      binary.BigEndian.Uint64(raw[68:76]),
		eventOrdinal: binary.BigEndian.Uint64(raw[76:84]),
	}
}

func decodeDecision(raw []byte) decisionRecord {
	var digest [16]byte
	copy(digest[:], raw[56:72])
	return decisionRecord{
		clientID: binary.BigEndian.Uint64(raw[0:8]), linkID: binary.BigEndian.Uint32(raw[8:12]), symbolID: binary.BigEndian.Uint32(raw[12:16]),
		side: raw[16], orderType: raw[17], tif: raw[18], requestID: binary.BigEndian.Uint64(raw[24:32]),
		decisionAt: int64(binary.BigEndian.Uint64(raw[32:40])), frontierOrdinal: binary.BigEndian.Uint64(raw[40:48]),
		frontierDeliveredAt: int64(binary.BigEndian.Uint64(raw[48:56])), frontierDigest: digest,
		price: int64(binary.BigEndian.Uint64(raw[72:80])), qty: int64(binary.BigEndian.Uint64(raw[80:88])), eventOrdinal: binary.BigEndian.Uint64(raw[88:96]),
	}
}

func validateObservation(result *MarketDataReceiptAudit, record observationRecord, links, symbols map[uint32]struct{}, receipt bool) {
	if _, exists := links[record.linkID]; !exists || record.linkID == 0 {
		result.UnknownLinkID++
	}
	if _, exists := symbols[record.symbolID]; !exists || record.symbolID == 0 {
		result.UnknownSymbolID++
	}
	if record.mdType > 6 {
		result.UnknownType++
	}
	for _, value := range record.raw[17:20] {
		if value != 0 {
			result.NonzeroReserved++
			break
		}
	}
	for _, value := range record.raw[84:88] {
		if value != 0 {
			result.NonzeroReserved++
			break
		}
	}
	if record.fingerprint == ([16]byte{}) {
		result.DuplicateSource++
	}
	if record.scheduledAt < record.publishedAt {
		result.ScheduledBeforePub++
	}
	if receipt {
		if record.deliveredAt < record.scheduledAt {
			result.DeliveredBeforePlan++
		}
	} else if record.deliveredAt != 0 {
		result.NonzeroReserved++
	}
}

func sameObservation(schedule, receipt observationRecord) bool {
	return schedule.clientID == receipt.clientID && schedule.linkID == receipt.linkID && schedule.symbolID == receipt.symbolID &&
		schedule.mdType == receipt.mdType && schedule.sequence == receipt.sequence && schedule.fingerprint == receipt.fingerprint &&
		schedule.publishedAt == receipt.publishedAt && schedule.scheduledAt == receipt.scheduledAt && schedule.ordinal == receipt.ordinal
}
