package analysis

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	cdfMarketDataRecordBytes   = 88
	cdfMarketDataDecisionBytes = 96
)

type cdfEvidenceFile struct {
	File    string `json:"file"`
	Records int64  `json:"records"`
	Digest  string `json:"digest"`
}

type cdfReceiptManifest struct {
	Schedules cdfEvidenceFile    `json:"schedules"`
	Receipts  cdfEvidenceFile    `json:"receipts"`
	Decisions cdfEvidenceFile    `json:"decisions"`
	Links     []cdfReceiptLink   `json:"links"`
	Symbols   []cdfReceiptSymbol `json:"symbols"`
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
}

type cdfMarketDataDecisionRecord struct {
	ClientID          uint64
	LinkID            uint32
	SymbolID          uint32
	RequestID         uint64
	DecisionAt        int64
	FrontierOrdinal   uint64
	FrontierDelivered int64
	Price             int64
	Qty               int64
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

type cdfMarketDataEvidence struct {
	links            map[uint32]cdfReceiptLink
	symbols          map[uint32]cdfReceiptSymbol
	receipts         map[cdfReceiptLinkOrdinal]cdfMarketDataRecord
	decisions        map[cdfReceiptDecisionKey]cdfMarketDataDecisionRecord
	receiptsByClient map[uint64]int64
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
	evidence := &cdfMarketDataEvidence{
		links:            make(map[uint32]cdfReceiptLink, len(manifest.Links)),
		symbols:          make(map[uint32]cdfReceiptSymbol, len(manifest.Symbols)),
		receipts:         make(map[cdfReceiptLinkOrdinal]cdfMarketDataRecord),
		decisions:        make(map[cdfReceiptDecisionKey]cdfMarketDataDecisionRecord),
		receiptsByClient: make(map[uint64]int64),
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
	scheduled := make(map[cdfReceiptLinkOrdinal]cdfMarketDataRecord, len(schedules)/cdfMarketDataRecordBytes)
	for offset := 0; offset < len(schedules); offset += cdfMarketDataRecordBytes {
		record := decodeCDFMarketDataRecord(schedules[offset:offset+cdfMarketDataRecordBytes], false)
		if err := validateCDFMarketDataRecord(record, evidence); err != nil {
			return nil, fmt.Errorf("schedule record %d: %w", offset/cdfMarketDataRecordBytes, err)
		}
		key := cdfReceiptLinkOrdinal{LinkID: record.LinkID, LinkOrdinal: record.LinkOrdinal}
		if _, exists := scheduled[key]; exists {
			return nil, fmt.Errorf("duplicate schedule link ordinal")
		}
		scheduled[key] = record
	}
	for offset := 0; offset < len(receipts); offset += cdfMarketDataRecordBytes {
		record := decodeCDFMarketDataRecord(receipts[offset:offset+cdfMarketDataRecordBytes], true)
		if err := validateCDFMarketDataRecord(record, evidence); err != nil {
			return nil, fmt.Errorf("receipt record %d: %w", offset/cdfMarketDataRecordBytes, err)
		}
		if record.DeliveredAt < record.ScheduledAt || record.DeliveredAt <= 0 {
			return nil, fmt.Errorf("receipt delivery precedes schedule")
		}
		key := cdfReceiptLinkOrdinal{LinkID: record.LinkID, LinkOrdinal: record.LinkOrdinal}
		schedule, exists := scheduled[key]
		if !exists || !sameCDFMarketDataSchedule(schedule, record) {
			return nil, fmt.Errorf("receipt has no matching schedule")
		}
		if _, exists := evidence.receipts[key]; exists {
			return nil, fmt.Errorf("duplicate receipt link ordinal")
		}
		evidence.receipts[key] = record
		evidence.receiptsByClient[record.ClientID]++
	}
	for offset := 0; offset < len(decisions); offset += cdfMarketDataDecisionBytes {
		record := decodeCDFMarketDataDecision(decisions[offset : offset+cdfMarketDataDecisionBytes])
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
		if !exists || receipt.DeliveredAt != record.FrontierDelivered || receipt.DeliveredAt > record.DecisionAt {
			return nil, fmt.Errorf("decision frontier does not resolve to a delivered receipt")
		}
		evidence.decisions[key] = record
	}
	return evidence, nil
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
	return cdfMarketDataDecisionRecord{
		ClientID:          binary.BigEndian.Uint64(raw[0:8]),
		LinkID:            binary.BigEndian.Uint32(raw[8:12]),
		SymbolID:          binary.BigEndian.Uint32(raw[12:16]),
		RequestID:         binary.BigEndian.Uint64(raw[24:32]),
		DecisionAt:        int64(binary.BigEndian.Uint64(raw[32:40])),
		FrontierOrdinal:   binary.BigEndian.Uint64(raw[40:48]),
		FrontierDelivered: int64(binary.BigEndian.Uint64(raw[48:56])),
		Price:             int64(binary.BigEndian.Uint64(raw[72:80])),
		Qty:               int64(binary.BigEndian.Uint64(raw[80:88])),
	}
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
