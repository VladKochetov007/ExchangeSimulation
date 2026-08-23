package analysis

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
		for _, value := range decisionsRaw[offset+19 : offset+24] {
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
		raw:          append([]byte(nil), raw...),
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
