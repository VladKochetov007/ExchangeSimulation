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
	cdfMarketDataRecordBytes   = 88
	cdfMarketDataDecisionBytes = 96
	cdfMarketDataActionBytes   = 112
)

type cdfEvidenceFile struct {
	File    string `json:"file"`
	Records int64  `json:"records"`
	Digest  string `json:"digest"`
}

type cdfReceiptManifest struct {
	SchemaVersion int                `json:"schema_version"`
	Domain        string             `json:"domain"`
	Ordering      string             `json:"ordering"`
	TerminalAt    int64              `json:"terminal_at"`
	Schedules     cdfEvidenceFile    `json:"schedules"`
	Receipts      cdfEvidenceFile    `json:"receipts"`
	Decisions     cdfEvidenceFile    `json:"decisions"`
	Actions       cdfEvidenceFile    `json:"actions"`
	Links         []cdfReceiptLink   `json:"links"`
	Symbols       []cdfReceiptSymbol `json:"symbols"`
}

type cdfReceiptLink struct {
	ID          uint32 `json:"id"`
	SourceVenue string `json:"source_venue"`
	Link        string `json:"link"`
	Role        string `json:"role"`
}

type cdfReceiptSymbol struct {
	ID     uint32 `json:"id"`
	Symbol string `json:"symbol"`
}

type cdfMarketDataRecord struct {
	ClientID     uint64
	LinkID       uint32
	SymbolID     uint32
	Type         uint8
	Sequence     uint64
	Fingerprint  [16]byte
	PublishedAt  int64
	ScheduledAt  int64
	DeliveredAt  int64
	LinkOrdinal  uint64
	EventOrdinal uint64
	Digest       [16]byte
}

type cdfMarketDataDecisionRecord struct {
	ClientID          uint64
	LinkID            uint32
	SymbolID          uint32
	Side              uint8
	OrderType         uint8
	TimeInForce       uint8
	PostOnly          bool
	RequestID         uint64
	DecisionAt        int64
	FrontierOrdinal   uint64
	FrontierDelivered int64
	FrontierDigest    [16]byte
	Price             int64
	Qty               int64
	EventOrdinal      uint64
}

type cdfMarketDataActionRecord struct {
	ClientID          uint64
	LinkID            uint32
	SymbolID          uint32
	RequestType       uint8
	Side              uint8
	OrderType         uint8
	TimeInForce       uint8
	RequestID         uint64
	DecisionAt        int64
	OrderID           uint64
	Price             int64
	Qty               int64
	FrontierOrdinal   uint64
	FrontierDelivered int64
	FrontierDigest    [16]byte
	PostOnly          bool
	EventOrdinal      uint64
}

type cdfReceiptLinkOrdinal struct {
	LinkID      uint32
	LinkOrdinal uint64
}

type cdfReceiptDecisionKey struct {
	ClientID  uint64
	LinkID    uint32
	RequestID uint64
}

type cdfReceiptActionKey struct {
	ClientID  uint64
	LinkID    uint32
	RequestID uint64
}

type cdfMarketDataEvidence struct {
	links            map[uint32]cdfReceiptLink
	symbols          map[uint32]cdfReceiptSymbol
	schedules        map[cdfReceiptLinkOrdinal]cdfMarketDataRecord
	receipts         map[cdfReceiptLinkOrdinal]cdfMarketDataRecord
	decisions        map[cdfReceiptDecisionKey]cdfMarketDataDecisionRecord
	actions          map[cdfReceiptActionKey]cdfMarketDataActionRecord
	receiptsByClient map[uint64]int64
	terminalAt       int64
}

type cdfReplayEvent struct {
	ordinal     uint64
	kind        string
	linkOrdinal cdfReceiptLinkOrdinal
	frontier    cdfReceiptLinkOrdinal
}

func readCDFMarketDataEvidence(runDir string) (*cdfMarketDataEvidence, error) {
	manifestRaw, err := os.ReadFile(filepath.Join(runDir, "market-data-evidence-v2.json"))
	if err != nil {
		return nil, fmt.Errorf("read market-data evidence manifest: %w", err)
	}
	var manifest cdfReceiptManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return nil, fmt.Errorf("decode market-data evidence manifest: %w", err)
	}
	if manifest.SchemaVersion != 2 || manifest.Domain != "participant_information_boundary_v2" || manifest.Ordering != "per_link_fifo_schedule_receipt_decision" || manifest.TerminalAt <= 0 {
		return nil, fmt.Errorf("market-data evidence manifest violates schema contract")
	}
	evidence := &cdfMarketDataEvidence{
		links:            make(map[uint32]cdfReceiptLink, len(manifest.Links)),
		symbols:          make(map[uint32]cdfReceiptSymbol, len(manifest.Symbols)),
		schedules:        make(map[cdfReceiptLinkOrdinal]cdfMarketDataRecord),
		receipts:         make(map[cdfReceiptLinkOrdinal]cdfMarketDataRecord),
		decisions:        make(map[cdfReceiptDecisionKey]cdfMarketDataDecisionRecord),
		actions:          make(map[cdfReceiptActionKey]cdfMarketDataActionRecord),
		receiptsByClient: make(map[uint64]int64),
		terminalAt:       manifest.TerminalAt,
	}
	for _, link := range manifest.Links {
		if link.ID == 0 || link.SourceVenue == "" || link.Link == "" || link.Role == "" {
			return nil, fmt.Errorf("invalid market-data link catalog row")
		}
		if _, exists := evidence.links[link.ID]; exists {
			return nil, fmt.Errorf("duplicate market-data link catalog id %d", link.ID)
		}
		evidence.links[link.ID] = link
	}
	for _, symbol := range manifest.Symbols {
		if symbol.ID == 0 || symbol.Symbol == "" {
			return nil, fmt.Errorf("invalid market-data symbol catalog row")
		}
		if _, exists := evidence.symbols[symbol.ID]; exists {
			return nil, fmt.Errorf("duplicate market-data symbol catalog id %d", symbol.ID)
		}
		evidence.symbols[symbol.ID] = symbol
	}
	schedules, err := readCDFEvidenceFile(runDir, manifest.Schedules, cdfMarketDataRecordBytes)
	if err != nil {
		return nil, fmt.Errorf("read market-data schedules: %w", err)
	}
	receipts, err := readCDFEvidenceFile(runDir, manifest.Receipts, cdfMarketDataRecordBytes)
	if err != nil {
		return nil, fmt.Errorf("read market-data receipts: %w", err)
	}
	decisions, err := readCDFEvidenceFile(runDir, manifest.Decisions, cdfMarketDataDecisionBytes)
	if err != nil {
		return nil, fmt.Errorf("read market-data decisions: %w", err)
	}
	actions, err := readCDFEvidenceFile(runDir, manifest.Actions, cdfMarketDataActionBytes)
	if err != nil {
		return nil, fmt.Errorf("read market-data actions: %w", err)
	}
	eventOrdinals := make(map[uint64]string, manifest.Schedules.Records+manifest.Receipts.Records+manifest.Decisions.Records)
	var lastScheduleEventOrdinal, lastReceiptEventOrdinal, lastDecisionEventOrdinal uint64
	for offset := 0; offset < len(schedules); offset += cdfMarketDataRecordBytes {
		record := decodeCDFMarketDataRecord(schedules[offset:offset+cdfMarketDataRecordBytes], false)
		if err := validateCDFMarketDataRecord(record, evidence); err != nil {
			return nil, fmt.Errorf("schedule record %d: %w", offset/cdfMarketDataRecordBytes, err)
		}
		if err := registerCDFEventOrdinal(eventOrdinals, record.EventOrdinal, "schedule"); err != nil {
			return nil, err
		}
		if record.EventOrdinal <= lastScheduleEventOrdinal {
			return nil, fmt.Errorf("schedule event ordinal is not strictly increasing")
		}
		lastScheduleEventOrdinal = record.EventOrdinal
		key := cdfReceiptLinkOrdinal{LinkID: record.LinkID, LinkOrdinal: record.LinkOrdinal}
		if _, exists := evidence.schedules[key]; exists {
			return nil, fmt.Errorf("duplicate schedule link ordinal")
		}
		evidence.schedules[key] = record
	}
	for offset := 0; offset < len(receipts); offset += cdfMarketDataRecordBytes {
		record := decodeCDFMarketDataRecord(receipts[offset:offset+cdfMarketDataRecordBytes], true)
		if err := validateCDFMarketDataRecord(record, evidence); err != nil {
			return nil, fmt.Errorf("receipt record %d: %w", offset/cdfMarketDataRecordBytes, err)
		}
		if err := registerCDFEventOrdinal(eventOrdinals, record.EventOrdinal, "receipt"); err != nil {
			return nil, err
		}
		if record.DeliveredAt < record.ScheduledAt || record.DeliveredAt <= 0 {
			return nil, fmt.Errorf("receipt delivery precedes schedule")
		}
		if record.DeliveredAt > manifest.TerminalAt {
			return nil, fmt.Errorf("receipt delivery occurs after terminal time")
		}
		if record.EventOrdinal <= lastReceiptEventOrdinal {
			return nil, fmt.Errorf("receipt event ordinal is not strictly increasing")
		}
		lastReceiptEventOrdinal = record.EventOrdinal
		key := cdfReceiptLinkOrdinal{LinkID: record.LinkID, LinkOrdinal: record.LinkOrdinal}
		schedule, exists := evidence.schedules[key]
		if !exists || !sameCDFMarketDataSchedule(schedule, record) {
			return nil, fmt.Errorf("receipt has no matching schedule")
		}
		if _, exists := evidence.receipts[key]; exists {
			return nil, fmt.Errorf("duplicate receipt link ordinal")
		}
		evidence.receipts[key] = record
		evidence.receiptsByClient[record.ClientID]++
	}
	for key, schedule := range evidence.schedules {
		if schedule.ScheduledAt <= manifest.TerminalAt {
			if _, exists := evidence.receipts[key]; !exists {
				return nil, fmt.Errorf("schedule link %d ordinal %d is due by terminal time without a receipt", key.LinkID, key.LinkOrdinal)
			}
		}
	}
	if err := reconstructCDFReceiptDigests(evidence); err != nil {
		return nil, fmt.Errorf("reconstruct market-data receipt frontiers: %w", err)
	}
	var lastActionOrdinal uint64
	for offset := 0; offset < len(actions); offset += cdfMarketDataActionBytes {
		raw := actions[offset : offset+cdfMarketDataActionBytes]
		if raw[92] > 1 || hasNonZeroBytes(raw[93:104]) {
			return nil, fmt.Errorf("action record %d has invalid post-only/reserved bytes", offset/cdfMarketDataActionBytes)
		}
		record := decodeCDFMarketDataAction(raw)
		if record.ClientID == 0 || record.LinkID == 0 || record.RequestID == 0 || record.DecisionAt <= 0 || record.RequestType < 1 || record.RequestType > 4 || record.EventOrdinal != lastActionOrdinal+1 {
			return nil, fmt.Errorf("action record %d has invalid identity or bounds", offset/cdfMarketDataActionBytes)
		}
		lastActionOrdinal = record.EventOrdinal
		if _, exists := evidence.links[record.LinkID]; !exists {
			return nil, fmt.Errorf("action record %d references unknown link", offset/cdfMarketDataActionBytes)
		}
		if record.SymbolID != 0 {
			if _, exists := evidence.symbols[record.SymbolID]; !exists {
				return nil, fmt.Errorf("action record %d references unknown symbol", offset/cdfMarketDataActionBytes)
			}
		}
		switch record.RequestType {
		case 1, 4:
			if record.SymbolID == 0 {
				return nil, fmt.Errorf("action record %d has no subscription symbol", offset/cdfMarketDataActionBytes)
			}
		case 2:
			if record.SymbolID == 0 || record.Price <= 0 || record.Qty <= 0 {
				return nil, fmt.Errorf("action record %d has incomplete order fields", offset/cdfMarketDataActionBytes)
			}
		case 3:
			if record.OrderID == 0 {
				return nil, fmt.Errorf("action record %d has no cancellation order", offset/cdfMarketDataActionBytes)
			}
		}
		if record.FrontierOrdinal == 0 {
			if record.FrontierDelivered != 0 || record.FrontierDigest != ([16]byte{}) {
				return nil, fmt.Errorf("action record %d has an incomplete empty frontier", offset/cdfMarketDataActionBytes)
			}
		} else {
			receipt, exists := evidence.receipts[cdfReceiptLinkOrdinal{LinkID: record.LinkID, LinkOrdinal: record.FrontierOrdinal}]
			latest, latestExists := latestCDFReceiptBefore(evidence.receipts, record.LinkID, record.DecisionAt)
			if !exists || !latestExists || latest.LinkOrdinal != record.FrontierOrdinal || receipt.DeliveredAt != record.FrontierDelivered || receipt.DeliveredAt > record.DecisionAt || record.FrontierDigest == ([16]byte{}) || receipt.Digest != record.FrontierDigest {
				return nil, fmt.Errorf("action record %d frontier does not resolve to the latest delivered receipt", offset/cdfMarketDataActionBytes)
			}
		}
		key := cdfReceiptActionKey{ClientID: record.ClientID, LinkID: record.LinkID, RequestID: record.RequestID}
		if _, exists := evidence.actions[key]; exists {
			return nil, fmt.Errorf("duplicate market-data action request")
		}
		evidence.actions[key] = record
	}
	for offset := 0; offset < len(decisions); offset += cdfMarketDataDecisionBytes {
		raw := decisions[offset : offset+cdfMarketDataDecisionBytes]
		if raw[19] > 1 || hasNonZeroBytes(raw[20:24]) {
			return nil, fmt.Errorf("decision record %d has invalid post-only/reserved bytes", offset/cdfMarketDataDecisionBytes)
		}
		record := decodeCDFMarketDataDecision(raw)
		if err := registerCDFEventOrdinal(eventOrdinals, record.EventOrdinal, "decision"); err != nil {
			return nil, err
		}
		if record.EventOrdinal <= lastDecisionEventOrdinal {
			return nil, fmt.Errorf("decision event ordinal is not strictly increasing")
		}
		lastDecisionEventOrdinal = record.EventOrdinal
		if record.ClientID == 0 || record.LinkID == 0 || record.SymbolID == 0 || record.RequestID == 0 || record.DecisionAt <= 0 || record.FrontierOrdinal == 0 || record.FrontierDelivered <= 0 || record.Price <= 0 || record.Qty <= 0 {
			return nil, fmt.Errorf("decision record %d has invalid identity or bounds", offset/cdfMarketDataDecisionBytes)
		}
		if _, exists := evidence.links[record.LinkID]; !exists {
			return nil, fmt.Errorf("decision record %d references unknown link", offset/cdfMarketDataDecisionBytes)
		}
		if _, exists := evidence.symbols[record.SymbolID]; !exists {
			return nil, fmt.Errorf("decision record %d references unknown symbol", offset/cdfMarketDataDecisionBytes)
		}
		key := cdfReceiptDecisionKey{ClientID: record.ClientID, LinkID: record.LinkID, RequestID: record.RequestID}
		if _, exists := evidence.decisions[key]; exists {
			return nil, fmt.Errorf("duplicate decision request")
		}
		receipt, exists := evidence.receipts[cdfReceiptLinkOrdinal{LinkID: record.LinkID, LinkOrdinal: record.FrontierOrdinal}]
		latest, latestExists := latestCDFReceiptBefore(evidence.receipts, record.LinkID, record.DecisionAt)
		if !exists || !latestExists || latest.LinkOrdinal != record.FrontierOrdinal || receipt.DeliveredAt != record.FrontierDelivered || receipt.DeliveredAt > record.DecisionAt || record.FrontierDigest == ([16]byte{}) || receipt.Digest != record.FrontierDigest {
			return nil, fmt.Errorf("decision frontier does not resolve to a delivered receipt")
		}
		evidence.decisions[key] = record
	}
	if err := validateCDFEventReplay(evidence, eventOrdinals); err != nil {
		return nil, err
	}
	return evidence, nil
}

func registerCDFEventOrdinal(ordinals map[uint64]string, ordinal uint64, kind string) error {
	if ordinal == 0 {
		return fmt.Errorf("%s has empty global event ordinal", kind)
	}
	if previous, exists := ordinals[ordinal]; exists {
		return fmt.Errorf("global event ordinal %d is duplicated by %s and %s", ordinal, previous, kind)
	}
	ordinals[ordinal] = kind
	return nil
}

func validateCDFEventReplay(evidence *cdfMarketDataEvidence, ordinals map[uint64]string) error {
	events := make([]cdfReplayEvent, 0, len(ordinals))
	for key, record := range evidence.schedules {
		events = append(events, cdfReplayEvent{ordinal: record.EventOrdinal, kind: "schedule", linkOrdinal: key})
	}
	for key, record := range evidence.receipts {
		events = append(events, cdfReplayEvent{ordinal: record.EventOrdinal, kind: "receipt", linkOrdinal: key})
	}
	for _, record := range evidence.decisions {
		events = append(events, cdfReplayEvent{
			ordinal:  record.EventOrdinal,
			kind:     "decision",
			frontier: cdfReceiptLinkOrdinal{LinkID: record.LinkID, LinkOrdinal: record.FrontierOrdinal},
		})
	}
	sort.Slice(events, func(i, j int) bool { return events[i].ordinal < events[j].ordinal })
	scheduled := make(map[cdfReceiptLinkOrdinal]uint64, len(evidence.schedules))
	received := make(map[cdfReceiptLinkOrdinal]uint64, len(evidence.receipts))
	for index, event := range events {
		expectedOrdinal := uint64(index + 1)
		if event.ordinal != expectedOrdinal {
			return fmt.Errorf("global event ordinal stream has a gap at %d", expectedOrdinal)
		}
		switch event.kind {
		case "schedule":
			scheduled[event.linkOrdinal] = event.ordinal
		case "receipt":
			scheduleOrdinal, exists := scheduled[event.linkOrdinal]
			if !exists || scheduleOrdinal >= event.ordinal {
				return fmt.Errorf("receipt event ordinal %d replays before its schedule", event.ordinal)
			}
			received[event.linkOrdinal] = event.ordinal
		case "decision":
			receiptOrdinal, exists := received[event.frontier]
			if !exists || receiptOrdinal >= event.ordinal {
				return fmt.Errorf("decision event ordinal %d replays before its observation receipt", event.ordinal)
			}
		default:
			return fmt.Errorf("unknown global event kind %q", event.kind)
		}
	}
	if len(events) != len(ordinals) {
		return fmt.Errorf("global event replay count differs from ordinal catalog")
	}
	return nil
}

func readCDFEvidenceFile(runDir string, file cdfEvidenceFile, recordBytes int) ([]byte, error) {
	if file.File == "" || file.Records < 0 {
		return nil, fmt.Errorf("invalid file declaration")
	}
	relative, err := filepath.Rel(runDir, filepath.Join(runDir, file.File))
	if err != nil || relative == ".." || len(relative) >= 3 && relative[:3] == ".."+string(filepath.Separator) {
		return nil, fmt.Errorf("evidence file escapes run directory")
	}
	raw, err := os.ReadFile(filepath.Join(runDir, file.File))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) != file.Records*int64(recordBytes) {
		return nil, fmt.Errorf("record count does not match byte length")
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != file.Digest {
		return nil, fmt.Errorf("digest mismatch")
	}
	return raw, nil
}

func decodeCDFMarketDataRecord(raw []byte, withDelivery bool) cdfMarketDataRecord {
	record := cdfMarketDataRecord{
		ClientID:     binary.BigEndian.Uint64(raw[0:8]),
		LinkID:       binary.BigEndian.Uint32(raw[8:12]),
		SymbolID:     binary.BigEndian.Uint32(raw[12:16]),
		Type:         raw[16],
		Sequence:     binary.BigEndian.Uint64(raw[20:28]),
		PublishedAt:  int64(binary.BigEndian.Uint64(raw[44:52])),
		ScheduledAt:  int64(binary.BigEndian.Uint64(raw[52:60])),
		LinkOrdinal:  binary.BigEndian.Uint64(raw[68:76]),
		EventOrdinal: binary.BigEndian.Uint64(raw[76:84]),
	}
	copy(record.Fingerprint[:], raw[28:44])
	if withDelivery {
		record.DeliveredAt = int64(binary.BigEndian.Uint64(raw[60:68]))
	}
	return record
}

func decodeCDFMarketDataDecision(raw []byte) cdfMarketDataDecisionRecord {
	record := cdfMarketDataDecisionRecord{
		ClientID:          binary.BigEndian.Uint64(raw[0:8]),
		LinkID:            binary.BigEndian.Uint32(raw[8:12]),
		SymbolID:          binary.BigEndian.Uint32(raw[12:16]),
		Side:              raw[16],
		OrderType:         raw[17],
		TimeInForce:       raw[18],
		PostOnly:          raw[19] == 1,
		RequestID:         binary.BigEndian.Uint64(raw[24:32]),
		DecisionAt:        int64(binary.BigEndian.Uint64(raw[32:40])),
		FrontierOrdinal:   binary.BigEndian.Uint64(raw[40:48]),
		FrontierDelivered: int64(binary.BigEndian.Uint64(raw[48:56])),
		EventOrdinal:      binary.BigEndian.Uint64(raw[88:96]),
		Price:             int64(binary.BigEndian.Uint64(raw[72:80])),
		Qty:               int64(binary.BigEndian.Uint64(raw[80:88])),
	}
	copy(record.FrontierDigest[:], raw[56:72])
	return record
}

func decodeCDFMarketDataAction(raw []byte) cdfMarketDataActionRecord {
	record := cdfMarketDataActionRecord{
		ClientID:          binary.BigEndian.Uint64(raw[0:8]),
		LinkID:            binary.BigEndian.Uint32(raw[8:12]),
		SymbolID:          binary.BigEndian.Uint32(raw[12:16]),
		RequestType:       raw[16],
		Side:              raw[17],
		OrderType:         raw[18],
		TimeInForce:       raw[19],
		RequestID:         binary.BigEndian.Uint64(raw[20:28]),
		DecisionAt:        int64(binary.BigEndian.Uint64(raw[28:36])),
		OrderID:           binary.BigEndian.Uint64(raw[36:44]),
		Price:             int64(binary.BigEndian.Uint64(raw[44:52])),
		Qty:               int64(binary.BigEndian.Uint64(raw[52:60])),
		FrontierOrdinal:   binary.BigEndian.Uint64(raw[60:68]),
		FrontierDelivered: int64(binary.BigEndian.Uint64(raw[68:76])),
		PostOnly:          raw[92] == 1,
		EventOrdinal:      binary.BigEndian.Uint64(raw[104:112]),
	}
	copy(record.FrontierDigest[:], raw[76:92])
	return record
}

func hasNonZeroBytes(raw []byte) bool {
	for _, value := range raw {
		if value != 0 {
			return true
		}
	}
	return false
}

func reconstructCDFReceiptDigests(evidence *cdfMarketDataEvidence) error {
	receiptsByLink := make(map[uint32][]cdfMarketDataRecord)
	for _, receipt := range evidence.receipts {
		receiptsByLink[receipt.LinkID] = append(receiptsByLink[receipt.LinkID], receipt)
	}
	for linkID, receipts := range receiptsByLink {
		sort.Slice(receipts, func(i, j int) bool { return receipts[i].LinkOrdinal < receipts[j].LinkOrdinal })
		var previous [16]byte
		for index := range receipts {
			receipt := receipts[index]
			if receipt.LinkOrdinal != uint64(index+1) {
				return fmt.Errorf("link %d receipt ordinal has a gap at %d", linkID, receipt.LinkOrdinal)
			}
			if index > 0 && receipt.DeliveredAt < receipts[index-1].DeliveredAt {
				return fmt.Errorf("link %d receipt delivery is not FIFO", linkID)
			}
			raw := encodeCDFMarketDataRecord(receipt)
			chain := sha256.New()
			_, _ = chain.Write(previous[:])
			_, _ = chain.Write(raw[:])
			copy(receipt.Digest[:], chain.Sum(nil))
			evidence.receipts[cdfReceiptLinkOrdinal{LinkID: linkID, LinkOrdinal: receipt.LinkOrdinal}] = receipt
			previous = receipt.Digest
		}
	}
	return nil
}

func latestCDFReceiptBefore(receipts map[cdfReceiptLinkOrdinal]cdfMarketDataRecord, linkID uint32, decisionAt int64) (cdfMarketDataRecord, bool) {
	var latest cdfMarketDataRecord
	found := false
	for key, receipt := range receipts {
		if key.LinkID != linkID || receipt.DeliveredAt > decisionAt {
			continue
		}
		if !found || receipt.LinkOrdinal > latest.LinkOrdinal {
			latest, found = receipt, true
		}
	}
	return latest, found
}

func encodeCDFMarketDataRecord(record cdfMarketDataRecord) [cdfMarketDataRecordBytes]byte {
	var raw [cdfMarketDataRecordBytes]byte
	binary.BigEndian.PutUint64(raw[0:8], record.ClientID)
	binary.BigEndian.PutUint32(raw[8:12], record.LinkID)
	binary.BigEndian.PutUint32(raw[12:16], record.SymbolID)
	raw[16] = record.Type
	binary.BigEndian.PutUint64(raw[20:28], record.Sequence)
	copy(raw[28:44], record.Fingerprint[:])
	binary.BigEndian.PutUint64(raw[44:52], uint64(record.PublishedAt))
	binary.BigEndian.PutUint64(raw[52:60], uint64(record.ScheduledAt))
	binary.BigEndian.PutUint64(raw[60:68], uint64(record.DeliveredAt))
	binary.BigEndian.PutUint64(raw[68:76], record.LinkOrdinal)
	binary.BigEndian.PutUint64(raw[76:84], record.EventOrdinal)
	return raw
}

func decodeFrontierDigest(value string) ([16]byte, error) {
	var digest [16]byte
	raw, err := hex.DecodeString(value)
	if err != nil || len(raw) != len(digest) {
		return digest, fmt.Errorf("frontier digest must be 16 bytes of hex")
	}
	copy(digest[:], raw)
	return digest, nil
}

func validateCDFMarketDataRecord(record cdfMarketDataRecord, evidence *cdfMarketDataEvidence) error {
	if record.ClientID == 0 || record.LinkID == 0 || record.SymbolID == 0 || record.Type > 6 || record.LinkOrdinal == 0 || record.EventOrdinal == 0 || record.PublishedAt <= 0 || record.ScheduledAt <= 0 {
		return fmt.Errorf("invalid identity or timestamps")
	}
	if _, exists := evidence.links[record.LinkID]; !exists {
		return fmt.Errorf("unknown link")
	}
	if _, exists := evidence.symbols[record.SymbolID]; !exists {
		return fmt.Errorf("unknown symbol")
	}
	return nil
}

func sameCDFMarketDataSchedule(schedule, receipt cdfMarketDataRecord) bool {
	return schedule.ClientID == receipt.ClientID && schedule.LinkID == receipt.LinkID && schedule.SymbolID == receipt.SymbolID && schedule.Type == receipt.Type && schedule.Sequence == receipt.Sequence && schedule.Fingerprint == receipt.Fingerprint && schedule.PublishedAt == receipt.PublishedAt && schedule.ScheduledAt == receipt.ScheduledAt
}
