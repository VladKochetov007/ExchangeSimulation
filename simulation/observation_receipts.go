package simulation

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"exchange_sim/types"
)

// V2-0 evidence is deliberately three linked, fixed-width ledgers:
//
//	courier schedules -> actor-inbox receipts -> order decisions.
//
// The auditor owns a separate decoder. These constants are part of the
// evidence contract, not an internal storage detail.
const (
	MarketDataScheduleRecordBytes = 88
	MarketDataReceiptRecordBytes  = 88
	MarketDataDecisionRecordBytes = 96
)

const (
	marketDataEvidenceDomain   = "participant_information_boundary_v2"
	marketDataEvidenceOrdering = "per_link_fifo_schedule_receipt_decision"
)

// MarketDataSchedule is emitted when a source message has entered a delayed
// courier. It establishes the finite set of messages which may later reach
// this participant. A payload fingerprint distinguishes directed lifecycle
// messages whose legacy SeqNum is zero.
type MarketDataSchedule struct {
	ClientID    uint64
	SourceVenue string
	Link        string
	Symbol      string
	Type        types.MDType
	Sequence    uint64
	Fingerprint [16]byte
	PublishedAt int64
	ScheduledAt int64
	LinkOrdinal uint64
}

// MarketDataReceipt is emitted only after the courier successfully inserts
// the matching message into the actor-facing inbox.
type MarketDataReceipt struct {
	MarketDataSchedule
	DeliveredAt int64
}

// MarketDataFrontier identifies exactly the prefix of one delayed public feed
// that reached an actor inbox before an audited order decision.
type MarketDataFrontier struct {
	LinkID      uint32
	Ordinal     uint64
	DeliveredAt int64
	Digest      [16]byte
	// Fingerprint identifies the last delivered public message itself. Digest
	// authenticates the complete prefix; Fingerprint lets an evidence-only
	// actor decision bind its reported observation to that exact message.
	Fingerprint [16]byte
}

// MarketDataDecision records an attempted order decision at the actor-facing
// gateway boundary, before modeled request latency. It does not claim the
// venue later accepted the request. Quotes are ordinary limit-order decisions.
type MarketDataDecision struct {
	ClientID    uint64
	SourceVenue string
	Link        string
	Symbol      string
	RequestID   uint64
	Side        types.Side
	OrderType   types.OrderType
	TimeInForce types.TimeInForce
	Price       int64
	Qty         int64
	DecisionAt  int64
	Frontier    MarketDataFrontier
}

type receiptLinkKey struct {
	SourceVenue string
	Link        string
}

type receiptLinkCatalog struct {
	ID          uint32 `json:"id"`
	SourceVenue string `json:"source_venue"`
	Link        string `json:"link"`
	Role        string `json:"role"`
}

type receiptSymbolCatalog struct {
	ID     uint32 `json:"id"`
	Symbol string `json:"symbol"`
}

type evidenceFileArtifact struct {
	File    string `json:"file"`
	Records int64  `json:"records"`
	Digest  string `json:"digest"`
}

type receiptArtifact struct {
	SchemaVersion int                    `json:"schema_version"`
	Domain        string                 `json:"domain"`
	Ordering      string                 `json:"ordering"`
	TerminalAt    int64                  `json:"terminal_at"`
	Schedules     evidenceFileArtifact   `json:"schedules"`
	Receipts      evidenceFileArtifact   `json:"receipts"`
	Decisions     evidenceFileArtifact   `json:"decisions"`
	Links         []receiptLinkCatalog   `json:"links"`
	Symbols       []receiptSymbolCatalog `json:"symbols"`
}

type evidenceWriter struct {
	file    *os.File
	writer  *bufio.Writer
	hasher  hash.Hash
	records int64
}

func newEvidenceWriter(path string) (*evidenceWriter, error) {
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &evidenceWriter{file: file, writer: bufio.NewWriterSize(file, 1<<20), hasher: sha256.New()}, nil
}

func (w *evidenceWriter) write(raw []byte) error {
	if _, err := w.writer.Write(raw); err != nil {
		return err
	}
	if _, err := w.hasher.Write(raw); err != nil {
		return err
	}
	w.records++
	return nil
}

func (w *evidenceWriter) close() error {
	if err := w.writer.Flush(); err != nil {
		_ = w.file.Close()
		return err
	}
	return w.file.Close()
}

func (w *evidenceWriter) artifact(file string) evidenceFileArtifact {
	return evidenceFileArtifact{File: file, Records: w.records, Digest: hex.EncodeToString(w.hasher.Sum(nil))}
}

// MarketDataReceiptRecorder owns compact V2-0 evidence. It is write-only from
// the model: no latency provider, scheduler, actor, matcher, or risk rule may
// read it. Finalization happens after the simulation's final fixed point.
type MarketDataReceiptRecorder struct {
	mu sync.Mutex

	manifestPath string
	schedules    *evidenceWriter
	receipts     *evidenceWriter
	decisions    *evidenceWriter

	links      map[receiptLinkKey]uint32
	linkRows   []receiptLinkCatalog
	symbols    map[string]uint32
	symbolRows []receiptSymbolCatalog
	frontiers  map[uint32]MarketDataFrontier
	nextEvent  uint64

	writeErr  error
	finalized bool
}

// NewMarketDataReceiptRecorder creates the V2-0 sidecars. It owns no
// goroutine and never schedules work; final files are binary to keep campaigns
// compact without retaining JSON duplicates of public-feed messages.
func NewMarketDataReceiptRecorder(dir string) (*MarketDataReceiptRecorder, error) {
	schedules, err := newEvidenceWriter(filepath.Join(dir, "market-data-schedules-v2.bin"))
	if err != nil {
		return nil, fmt.Errorf("create market-data schedule sidecar: %w", err)
	}
	receipts, err := newEvidenceWriter(filepath.Join(dir, "market-data-receipts-v2.bin"))
	if err != nil {
		_ = schedules.close()
		return nil, fmt.Errorf("create market-data receipt sidecar: %w", err)
	}
	decisions, err := newEvidenceWriter(filepath.Join(dir, "market-data-decisions-v2.bin"))
	if err != nil {
		_ = schedules.close()
		_ = receipts.close()
		return nil, fmt.Errorf("create market-data decision sidecar: %w", err)
	}
	return &MarketDataReceiptRecorder{
		manifestPath: filepath.Join(dir, "market-data-evidence-v2.json"),
		schedules:    schedules,
		receipts:     receipts,
		decisions:    decisions,
		links:        make(map[receiptLinkKey]uint32),
		symbols:      make(map[string]uint32),
		frontiers:    make(map[uint32]MarketDataFrontier),
	}, nil
}

// RegisterLink allocates immutable catalog identity during participant setup.
// It lets a decision with an empty observation frontier still name its feed.
func (r *MarketDataReceiptRecorder) RegisterLink(sourceVenue, link, role string) uint32 {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finalized || r.writeErr != nil || sourceVenue == "" || link == "" || role == "" {
		return 0
	}
	return r.linkIDLocked(sourceVenue, link, role)
}

// RegisteredRoleCount lets the scenario controller reject an evidence request
// that never reached an instrumented participant link (for example a custom
// router mount that bypasses the ordinary role mount factory).
func (r *MarketDataReceiptRecorder) RegisteredRoleCount(role string) int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, row := range r.linkRows {
		if row.Role == role {
			count++
		}
	}
	return count
}

// Fail retains an evidence-encoding failure until finalization can return it
// through the normal simulation controller. It is intentionally not exposed
// to actors or model code.
func (r *MarketDataReceiptRecorder) Fail(err error) {
	if r == nil || err == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.writeErr == nil {
		r.writeErr = err
	}
}

// RecordSchedule emits source-to-courier admission before the arrival callback
// is registered, so a dropped tail cannot disappear merely because no later
// sequence exposes a gap.
func (r *MarketDataReceiptRecorder) RecordSchedule(schedule MarketDataSchedule) uint32 {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finalized || r.writeErr != nil {
		return 0
	}
	linkID, symbolID, ok := r.recordIDsLocked(schedule.ClientID, schedule.SourceVenue, schedule.Link, schedule.Symbol)
	if !ok {
		return 0
	}
	if schedule.Fingerprint == ([16]byte{}) {
		r.writeErr = fmt.Errorf("market-data schedule has empty message fingerprint")
		return 0
	}
	r.nextEvent++
	var raw [MarketDataScheduleRecordBytes]byte
	encodeMarketDataRecord(raw[:], schedule, linkID, symbolID, 0, r.nextEvent)
	if err := r.schedules.write(raw[:]); err != nil {
		r.writeErr = fmt.Errorf("write market-data schedule: %w", err)
		return 0
	}
	return linkID
}

// RecordReceipt emits actual inbox insertion and returns the new local frontier.
func (r *MarketDataReceiptRecorder) RecordReceipt(receipt MarketDataReceipt) MarketDataFrontier {
	if r == nil {
		return MarketDataFrontier{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finalized || r.writeErr != nil {
		return MarketDataFrontier{}
	}
	linkID, symbolID, ok := r.recordIDsLocked(receipt.ClientID, receipt.SourceVenue, receipt.Link, receipt.Symbol)
	if !ok {
		return MarketDataFrontier{}
	}
	r.nextEvent++
	var raw [MarketDataReceiptRecordBytes]byte
	encodeMarketDataRecord(raw[:], receipt.MarketDataSchedule, linkID, symbolID, receipt.DeliveredAt, r.nextEvent)
	if err := r.receipts.write(raw[:]); err != nil {
		r.writeErr = fmt.Errorf("write market-data receipt: %w", err)
		return MarketDataFrontier{}
	}
	previous := r.frontiers[linkID]
	chain := sha256.New()
	_, _ = chain.Write(previous.Digest[:])
	_, _ = chain.Write(raw[:])
	var digest [16]byte
	copy(digest[:], chain.Sum(nil))
	frontier := MarketDataFrontier{LinkID: linkID, Ordinal: receipt.LinkOrdinal, DeliveredAt: receipt.DeliveredAt, Digest: digest, Fingerprint: receipt.Fingerprint}
	r.frontiers[linkID] = frontier
	return frontier
}

// RecordDecision appends an order decision before request latency. The
// independent auditor reconstructs the supplied prefix from receipt rows.
func (r *MarketDataReceiptRecorder) RecordDecision(decision MarketDataDecision) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finalized || r.writeErr != nil {
		return
	}
	linkID, symbolID, ok := r.recordIDsLocked(decision.ClientID, decision.SourceVenue, decision.Link, decision.Symbol)
	if !ok {
		return
	}
	if decision.Frontier.LinkID != 0 && decision.Frontier.LinkID != linkID {
		r.writeErr = fmt.Errorf("market-data decision frontier link %d does not match decision link %d", decision.Frontier.LinkID, linkID)
		return
	}
	if decision.Frontier.LinkID == 0 {
		decision.Frontier.LinkID = linkID
	}
	r.nextEvent++
	var raw [MarketDataDecisionRecordBytes]byte
	binary.BigEndian.PutUint64(raw[0:8], decision.ClientID)
	binary.BigEndian.PutUint32(raw[8:12], linkID)
	binary.BigEndian.PutUint32(raw[12:16], symbolID)
	raw[16] = byte(decision.Side)
	raw[17] = byte(decision.OrderType)
	raw[18] = byte(decision.TimeInForce)
	binary.BigEndian.PutUint64(raw[24:32], decision.RequestID)
	binary.BigEndian.PutUint64(raw[32:40], uint64(decision.DecisionAt))
	binary.BigEndian.PutUint64(raw[40:48], decision.Frontier.Ordinal)
	binary.BigEndian.PutUint64(raw[48:56], uint64(decision.Frontier.DeliveredAt))
	copy(raw[56:72], decision.Frontier.Digest[:])
	binary.BigEndian.PutUint64(raw[72:80], uint64(decision.Price))
	binary.BigEndian.PutUint64(raw[80:88], uint64(decision.Qty))
	binary.BigEndian.PutUint64(raw[88:96], r.nextEvent)
	if err := r.decisions.write(raw[:]); err != nil {
		r.writeErr = fmt.Errorf("write market-data decision: %w", err)
	}
}

func (r *MarketDataReceiptRecorder) recordIDsLocked(clientID uint64, sourceVenue, link, symbol string) (uint32, uint32, bool) {
	if clientID == 0 || sourceVenue == "" || link == "" || symbol == "" {
		r.writeErr = fmt.Errorf("market-data evidence has empty client, source, link, or symbol")
		return 0, 0, false
	}
	return r.linkIDLocked(sourceVenue, link, ""), r.symbolIDLocked(symbol), true
}

func (r *MarketDataReceiptRecorder) linkIDLocked(sourceVenue, link, role string) uint32 {
	key := receiptLinkKey{SourceVenue: sourceVenue, Link: link}
	if id, exists := r.links[key]; exists {
		if role != "" && r.linkRows[id-1].Role != role {
			r.writeErr = fmt.Errorf("market-data link %q registered with conflicting role %q", link, role)
		}
		return id
	}
	id := uint32(len(r.linkRows) + 1)
	r.links[key] = id
	r.linkRows = append(r.linkRows, receiptLinkCatalog{ID: id, SourceVenue: sourceVenue, Link: link, Role: role})
	return id
}

func (r *MarketDataReceiptRecorder) symbolIDLocked(symbol string) uint32 {
	if id, exists := r.symbols[symbol]; exists {
		return id
	}
	id := uint32(len(r.symbolRows) + 1)
	r.symbols[symbol] = id
	r.symbolRows = append(r.symbolRows, receiptSymbolCatalog{ID: id, Symbol: symbol})
	return id
}

func encodeMarketDataRecord(raw []byte, record MarketDataSchedule, linkID, symbolID uint32, deliveredAt int64, eventOrdinal uint64) {
	binary.BigEndian.PutUint64(raw[0:8], record.ClientID)
	binary.BigEndian.PutUint32(raw[8:12], linkID)
	binary.BigEndian.PutUint32(raw[12:16], symbolID)
	raw[16] = byte(record.Type)
	binary.BigEndian.PutUint64(raw[20:28], record.Sequence)
	copy(raw[28:44], record.Fingerprint[:])
	binary.BigEndian.PutUint64(raw[44:52], uint64(record.PublishedAt))
	binary.BigEndian.PutUint64(raw[52:60], uint64(record.ScheduledAt))
	binary.BigEndian.PutUint64(raw[60:68], uint64(deliveredAt))
	binary.BigEndian.PutUint64(raw[68:76], record.LinkOrdinal)
	binary.BigEndian.PutUint64(raw[76:84], eventOrdinal)
}

// Finalize flushes after the final simulation fixed point. TerminalAt lets the
// auditor distinguish a future-horizon schedule from a missing due delivery.
func (r *MarketDataReceiptRecorder) Finalize(terminalAt int64) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finalized {
		return r.writeErr
	}
	r.finalized = true
	for _, writer := range []*evidenceWriter{r.schedules, r.receipts, r.decisions} {
		if err := writer.close(); err != nil && r.writeErr == nil {
			r.writeErr = fmt.Errorf("close market-data evidence sidecar: %w", err)
		}
	}
	if r.writeErr != nil {
		return r.writeErr
	}
	links := append([]receiptLinkCatalog(nil), r.linkRows...)
	symbols := append([]receiptSymbolCatalog(nil), r.symbolRows...)
	sort.Slice(links, func(i, j int) bool { return links[i].ID < links[j].ID })
	sort.Slice(symbols, func(i, j int) bool { return symbols[i].ID < symbols[j].ID })
	artifact := receiptArtifact{
		SchemaVersion: 2,
		Domain:        marketDataEvidenceDomain,
		Ordering:      marketDataEvidenceOrdering,
		TerminalAt:    terminalAt,
		Schedules:     r.schedules.artifact("market-data-schedules-v2.bin"),
		Receipts:      r.receipts.artifact("market-data-receipts-v2.bin"),
		Decisions:     r.decisions.artifact("market-data-decisions-v2.bin"),
		Links:         links,
		Symbols:       symbols,
	}
	raw, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		r.writeErr = fmt.Errorf("marshal market-data evidence manifest: %w", err)
		return r.writeErr
	}
	if err := os.WriteFile(r.manifestPath, append(raw, '\n'), 0644); err != nil {
		r.writeErr = fmt.Errorf("write market-data evidence manifest: %w", err)
		return r.writeErr
	}
	return nil
}
