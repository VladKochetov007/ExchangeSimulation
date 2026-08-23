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
	decisionFrontierVectorRecordBytes    = 72
	decisionFrontierComponentRecordBytes = 56
)

// DecisionFrontierVectorAudit independently verifies the V2-1 multi-feed
// decision contract. It imports neither the simulator writer nor actor code.
type DecisionFrontierVectorAudit struct {
	BaseEvidenceValid         bool  `json:"base_evidence_valid"`
	BaseManifestDigestMatches bool  `json:"base_manifest_digest_matches"`
	DecisionDigestMatches     bool  `json:"decision_digest_matches"`
	ComponentDigestMatches    bool  `json:"component_digest_matches"`
	Decisions                 int64 `json:"decisions"`
	Components                int64 `json:"components"`
	BadDecisionID             int64 `json:"bad_decision_id"`
	BadDecisionFields         int64 `json:"bad_decision_fields"`
	MissingScalarDecision     int64 `json:"missing_scalar_decision"`
	UnknownComponentLink      int64 `json:"unknown_component_link"`
	BadComponentOrdinal       int64 `json:"bad_component_ordinal"`
	DuplicateComponent        int64 `json:"duplicate_component"`
	BadComponentFrontier      int64 `json:"bad_component_frontier"`
	FutureComponentUse        int64 `json:"future_component_use"`
	MissingDecisionComponents int64 `json:"missing_decision_components"`
	ExtraDecisionComponents   int64 `json:"extra_decision_components"`
	NonzeroReserved           int64 `json:"nonzero_reserved"`
	Valid                     bool  `json:"valid"`
}

type frontierVectorManifest struct {
	SchemaVersion      int                   `json:"schema_version"`
	Domain             string                `json:"domain"`
	Ordering           string                `json:"ordering"`
	BaseManifest       string                `json:"base_manifest"`
	BaseManifestDigest string                `json:"base_manifest_digest"`
	Decisions          vectorFileArtifact    `json:"decisions"`
	Components         vectorFileArtifact    `json:"components"`
	Symbols            []vectorSymbolCatalog `json:"symbols"`
}

type vectorFileArtifact struct {
	File    string `json:"file"`
	Records int64  `json:"records"`
	Digest  string `json:"digest"`
}

type vectorSymbolCatalog struct {
	ID     uint32 `json:"id"`
	Symbol string `json:"symbol"`
}

type vectorDecisionRecord struct {
	id             uint64
	actorID        uint64
	clientID       uint64
	requestID      uint64
	tradingLinkID  uint32
	symbolID       uint32
	side           uint8
	orderType      uint8
	tif            uint8
	componentCount uint32
	decisionAt     int64
	price          int64
	qty            int64
}

type vectorComponentRecord struct {
	decisionID uint64
	clientID   uint64
	linkID     uint32
	ordinal    uint32
	frontier   auditedFrontier
}

type vectorFrontierKey struct {
	clientID uint64
	linkID   uint32
	ordinal  uint64
}

type scalarDecisionKey struct {
	clientID  uint64
	linkID    uint32
	requestID uint64
}

// AuditDecisionFrontierVectors checks a separately persisted vector artifact
// against the V2-0 schedule/receipt/decision evidence it names.
func AuditDecisionFrontierVectors(dir string) (*DecisionFrontierVectorAudit, error) {
	base, err := AuditMarketDataReceipts(dir)
	if err != nil {
		return nil, fmt.Errorf("audit base market-data evidence: %w", err)
	}
	vectorRaw, err := os.ReadFile(filepath.Join(dir, "market-data-frontier-vectors-v1.json"))
	if err != nil {
		return nil, fmt.Errorf("read decision frontier-vector manifest: %w", err)
	}
	var manifest frontierVectorManifest
	if err := json.Unmarshal(vectorRaw, &manifest); err != nil {
		return nil, fmt.Errorf("decode decision frontier-vector manifest: %w", err)
	}
	if manifest.SchemaVersion != 1 || manifest.Domain != "participant_information_frontier_vector_v1" ||
		manifest.Ordering != "decision_then_sorted_components" || manifest.BaseManifest != "market-data-evidence-v2.json" ||
		manifest.Decisions.File != "market-data-decision-vectors-v1.bin" || manifest.Components.File != "market-data-frontier-components-v1.bin" {
		return nil, fmt.Errorf("unsupported decision frontier-vector contract")
	}
	if len(manifest.BaseManifestDigest) != 64 {
		return nil, fmt.Errorf("invalid base market-data manifest digest")
	}
	baseManifestRaw, err := os.ReadFile(filepath.Join(dir, manifest.BaseManifest))
	if err != nil {
		return nil, fmt.Errorf("read vector base manifest: %w", err)
	}
	baseManifestDigest := sha256.Sum256(baseManifestRaw)
	decisionsRaw, decisionsDigest, err := readEvidenceFile(dir, manifest.Decisions.File, decisionFrontierVectorRecordBytes, manifest.Decisions.Records, manifest.Decisions.Digest)
	if err != nil {
		return nil, err
	}
	componentsRaw, componentsDigest, err := readEvidenceFile(dir, manifest.Components.File, decisionFrontierComponentRecordBytes, manifest.Components.Records, manifest.Components.Digest)
	if err != nil {
		return nil, err
	}
	vectorSymbols := make(map[uint32]string, len(manifest.Symbols))
	for _, symbol := range manifest.Symbols {
		if symbol.ID == 0 || symbol.Symbol == "" {
			return nil, fmt.Errorf("invalid vector symbol catalog row")
		}
		if _, exists := vectorSymbols[symbol.ID]; exists {
			return nil, fmt.Errorf("duplicate vector symbol catalog ID %d", symbol.ID)
		}
		vectorSymbols[symbol.ID] = symbol.Symbol
	}

	var baseManifest marketDataEvidenceManifest
	if err := json.Unmarshal(baseManifestRaw, &baseManifest); err != nil {
		return nil, fmt.Errorf("decode vector base manifest: %w", err)
	}
	baseLinks, _, err := validateEvidenceCatalog(baseManifest)
	if err != nil {
		return nil, err
	}
	baseSymbolNames := make(map[uint32]string, len(baseManifest.Symbols))
	for _, symbol := range baseManifest.Symbols {
		baseSymbolNames[symbol.ID] = symbol.Symbol
	}
	receiptsRaw, _, err := readEvidenceFile(dir, baseManifest.Receipts.File, marketDataReceiptRecordBytes, baseManifest.Receipts.Records, baseManifest.Receipts.Digest)
	if err != nil {
		return nil, err
	}
	scalarRaw, _, err := readEvidenceFile(dir, baseManifest.Decisions.File, marketDataDecisionRecordBytes, baseManifest.Decisions.Records, baseManifest.Decisions.Digest)
	if err != nil {
		return nil, err
	}

	result := &DecisionFrontierVectorAudit{
		BaseEvidenceValid:         base.Valid,
		BaseManifestDigestMatches: manifest.BaseManifestDigest == hex.EncodeToString(baseManifestDigest[:]),
		DecisionDigestMatches:     decisionsDigest,
		ComponentDigestMatches:    componentsDigest,
		Decisions:                 manifest.Decisions.Records,
		Components:                manifest.Components.Records,
	}
	history := reconstructReceiptHistory(receiptsRaw)
	scalar := make(map[scalarDecisionKey][]decisionRecord, baseManifest.Decisions.Records)
	for offset := 0; offset < len(scalarRaw); offset += marketDataDecisionRecordBytes {
		record := decodeDecision(scalarRaw[offset : offset+marketDataDecisionRecordBytes])
		key := scalarDecisionKey{clientID: record.clientID, linkID: record.linkID, requestID: record.requestID}
		scalar[key] = append(scalar[key], record)
	}

	componentsByDecision := make(map[uint64][]vectorComponentRecord, result.Decisions)
	for offset := 0; offset < len(componentsRaw); offset += decisionFrontierComponentRecordBytes {
		record := decodeVectorComponent(componentsRaw[offset : offset+decisionFrontierComponentRecordBytes])
		for _, value := range componentsRaw[offset+56 : offset+decisionFrontierComponentRecordBytes] {
			if value != 0 {
				result.NonzeroReserved++
				break
			}
		}
		componentsByDecision[record.decisionID] = append(componentsByDecision[record.decisionID], record)
	}
	for offset := 0; offset < len(decisionsRaw); offset += decisionFrontierVectorRecordBytes {
		record := decodeVectorDecision(decisionsRaw[offset : offset+decisionFrontierVectorRecordBytes])
		for _, value := range decisionsRaw[offset+43 : offset+44] {
			if value != 0 {
				result.NonzeroReserved++
				break
			}
		}
		wantID := uint64(offset/decisionFrontierVectorRecordBytes + 1)
		if record.id != wantID {
			result.BadDecisionID++
		}
		symbol, symbolOK := vectorSymbols[record.symbolID]
		if record.actorID == 0 || record.clientID == 0 || record.requestID == 0 || record.tradingLinkID == 0 || record.componentCount == 0 || !symbolOK ||
			record.side > 1 || record.orderType > 1 || record.tif > 2 || record.decisionAt == 0 || record.price <= 0 || record.qty <= 0 {
			result.BadDecisionFields++
		}
		matchingScalar := false
		for _, candidate := range scalar[scalarDecisionKey{clientID: record.clientID, linkID: record.tradingLinkID, requestID: record.requestID}] {
			if baseSymbolNames[candidate.symbolID] == symbol && candidate.side == record.side && candidate.orderType == record.orderType && candidate.tif == record.tif &&
				candidate.decisionAt == record.decisionAt && candidate.price == record.price && candidate.qty == record.qty {
				matchingScalar = true
				break
			}
		}
		if !matchingScalar {
			result.MissingScalarDecision++
		}
		validateVectorComponents(result, record, componentsByDecision[record.id], history, baseLinks)
	}
	for decisionID, components := range componentsByDecision {
		if decisionID == 0 || decisionID > uint64(result.Decisions) {
			result.ExtraDecisionComponents += int64(len(components))
		}
	}
	result.Valid = result.BaseEvidenceValid && result.BaseManifestDigestMatches && result.DecisionDigestMatches && result.ComponentDigestMatches &&
		result.BadDecisionID == 0 && result.BadDecisionFields == 0 && result.MissingScalarDecision == 0 && result.UnknownComponentLink == 0 &&
		result.BadComponentOrdinal == 0 && result.DuplicateComponent == 0 && result.BadComponentFrontier == 0 && result.FutureComponentUse == 0 &&
		result.MissingDecisionComponents == 0 && result.ExtraDecisionComponents == 0 && result.NonzeroReserved == 0
	return result, nil
}

func reconstructReceiptHistory(raw []byte) map[vectorFrontierKey]auditedFrontier {
	records := make([]observationRecord, 0, len(raw)/marketDataReceiptRecordBytes)
	for offset := 0; offset < len(raw); offset += marketDataReceiptRecordBytes {
		records = append(records, decodeObservation(raw[offset:offset+marketDataReceiptRecordBytes]))
	}
	sort.Slice(records, func(i, j int) bool { return records[i].eventOrdinal < records[j].eventOrdinal })
	frontiers := make(map[linkKey]auditedFrontier)
	history := make(map[vectorFrontierKey]auditedFrontier, len(records))
	for _, record := range records {
		key := linkKey{clientID: record.clientID, linkID: record.linkID}
		previous := frontiers[key]
		chain := sha256.New()
		_, _ = chain.Write(previous.digest[:])
		_, _ = chain.Write(record.raw)
		var digest [16]byte
		copy(digest[:], chain.Sum(nil))
		frontier := auditedFrontier{ordinal: record.ordinal, deliveredAt: record.deliveredAt, digest: digest}
		frontiers[key] = frontier
		history[vectorFrontierKey{clientID: record.clientID, linkID: record.linkID, ordinal: record.ordinal}] = frontier
	}
	return history
}

func validateVectorComponents(result *DecisionFrontierVectorAudit, decision vectorDecisionRecord, components []vectorComponentRecord, history map[vectorFrontierKey]auditedFrontier, links map[uint32]struct{}) {
	if len(components) == 0 {
		result.MissingDecisionComponents++
		return
	}
	if uint32(len(components)) != decision.componentCount {
		if uint32(len(components)) < decision.componentCount {
			result.MissingDecisionComponents++
		} else {
			result.ExtraDecisionComponents += int64(len(components)) - int64(decision.componentCount)
		}
	}
	sort.Slice(components, func(i, j int) bool { return components[i].ordinal < components[j].ordinal })
	seen := make(map[linkKey]struct{}, len(components))
	for index, component := range components {
		if component.ordinal != uint32(index+1) {
			result.BadComponentOrdinal++
		}
		key := linkKey{clientID: component.clientID, linkID: component.linkID}
		if component.clientID == 0 || component.linkID == 0 {
			result.UnknownComponentLink++
		}
		if _, known := links[component.linkID]; !known {
			result.UnknownComponentLink++
		}
		if _, duplicate := seen[key]; duplicate {
			result.DuplicateComponent++
		}
		seen[key] = struct{}{}
		if component.frontier.ordinal == 0 || component.frontier.deliveredAt == 0 || component.frontier.digest == ([16]byte{}) {
			result.BadComponentFrontier++
			continue
		}
		want, exists := history[vectorFrontierKey{clientID: component.clientID, linkID: component.linkID, ordinal: component.frontier.ordinal}]
		if !exists || want != component.frontier {
			result.BadComponentFrontier++
		}
		if component.frontier.deliveredAt > decision.decisionAt {
			result.FutureComponentUse++
		}
	}
}

func decodeVectorDecision(raw []byte) vectorDecisionRecord {
	return vectorDecisionRecord{
		id: binary.BigEndian.Uint64(raw[0:8]), actorID: binary.BigEndian.Uint64(raw[8:16]), clientID: binary.BigEndian.Uint64(raw[16:24]),
		requestID: binary.BigEndian.Uint64(raw[24:32]), tradingLinkID: binary.BigEndian.Uint32(raw[32:36]), symbolID: binary.BigEndian.Uint32(raw[36:40]),
		side: raw[40], orderType: raw[41], tif: raw[42], componentCount: binary.BigEndian.Uint32(raw[44:48]), decisionAt: int64(binary.BigEndian.Uint64(raw[48:56])),
		price: int64(binary.BigEndian.Uint64(raw[56:64])), qty: int64(binary.BigEndian.Uint64(raw[64:72])),
	}
}

func decodeVectorComponent(raw []byte) vectorComponentRecord {
	var digest [16]byte
	copy(digest[:], raw[40:56])
	return vectorComponentRecord{
		decisionID: binary.BigEndian.Uint64(raw[0:8]), clientID: binary.BigEndian.Uint64(raw[8:16]),
		linkID: binary.BigEndian.Uint32(raw[16:20]), ordinal: binary.BigEndian.Uint32(raw[20:24]),
		frontier: auditedFrontier{ordinal: binary.BigEndian.Uint64(raw[24:32]), deliveredAt: int64(binary.BigEndian.Uint64(raw[32:40])), digest: digest},
	}
}
