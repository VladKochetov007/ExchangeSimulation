package analysis

import (
	"bufio"
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
	MissingVectorDecision     int64 `json:"missing_vector_decision"`
	DuplicateVectorDecision   int64 `json:"duplicate_vector_decision"`
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
	SchemaVersion       int                          `json:"schema_version"`
	Domain              string                       `json:"domain"`
	Ordering            string                       `json:"ordering"`
	BaseManifest        string                       `json:"base_manifest"`
	BaseManifestDigest  string                       `json:"base_manifest_digest"`
	RequiredScalarLinks []requiredScalarDecisionLink `json:"required_scalar_decision_links"`
	Decisions           vectorFileArtifact           `json:"decisions"`
	Components          vectorFileArtifact           `json:"components"`
	Symbols             []vectorSymbolCatalog        `json:"symbols"`
}

type requiredScalarDecisionLink struct {
	ClientID uint64 `json:"client_id"`
	LinkID   uint32 `json:"link_id"`
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

type frontierScalarRecord struct {
	key        scalarDecisionKey
	symbolID   uint32
	side       uint8
	orderType  uint8
	tif        uint8
	decisionAt int64
	price      int64
	qty        int64
}

type frontierHistoryFile struct {
	file          *os.File
	writer        *bufio.Writer
	count         uint64
	nonSequential bool
	byOrdinal     map[uint64]auditedFrontier
}

type frontierHistoryStore struct {
	dir               string
	files             map[linkKey]*frontierHistoryFile
	frontiers         map[linkKey]auditedFrontier
	nonSequentialRows uint64
}

const (
	maxFrontierVectorRecords     = 2_000_000
	maxFrontierScalarRecords     = 5_000_000
	maxFrontierHistoryRecords    = 100_000_000
	maxFrontierHistoryLinks      = 4_096
	maxFrontierNonSequentialRows = 1_000_000
	frontierHistoryRecordBytes   = 32
	frontierHistoryWriterBytes   = 64 << 10
)

// AuditDecisionFrontierVectors checks a separately persisted vector artifact
// against the V2-0 schedule/receipt/decision evidence it names.
func AuditDecisionFrontierVectors(dir string) (*DecisionFrontierVectorAudit, error) {
	return auditDecisionFrontierVectorsStreaming(dir)
}

// auditDecisionFrontierVectorsBuffered is retained as a review oracle while
// the production path moves to bounded streaming and disk-backed history.
func auditDecisionFrontierVectorsBuffered(dir string) (*DecisionFrontierVectorAudit, error) {
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
	requiredLinks := make(map[linkKey]struct{}, len(manifest.RequiredScalarLinks))
	for _, required := range manifest.RequiredScalarLinks {
		key := linkKey{clientID: required.ClientID, linkID: required.LinkID}
		if required.ClientID == 0 || required.LinkID == 0 {
			return nil, fmt.Errorf("invalid required scalar decision link")
		}
		if _, known := baseLinks[required.LinkID]; !known {
			return nil, fmt.Errorf("unknown required scalar decision link %d", required.LinkID)
		}
		if _, duplicate := requiredLinks[key]; duplicate {
			return nil, fmt.Errorf("duplicate required scalar decision link client %d link %d", required.ClientID, required.LinkID)
		}
		requiredLinks[key] = struct{}{}
	}
	requiredScalar := make(map[scalarDecisionKey][]decisionRecord)
	for offset := 0; offset < len(scalarRaw); offset += marketDataDecisionRecordBytes {
		record := decodeDecision(scalarRaw[offset : offset+marketDataDecisionRecordBytes])
		key := scalarDecisionKey{clientID: record.clientID, linkID: record.linkID, requestID: record.requestID}
		scalar[key] = append(scalar[key], record)
		if _, required := requiredLinks[linkKey{clientID: record.clientID, linkID: record.linkID}]; required {
			requiredScalar[key] = append(requiredScalar[key], record)
		}
	}
	matchedRequired := make(map[scalarDecisionKey]int, len(requiredScalar))

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
			record.side > 1 || !validVectorOrderPrice(record.orderType, record.price) || record.tif > 2 || record.decisionAt == 0 || record.qty <= 0 {
			result.BadDecisionFields++
		}
		key := scalarDecisionKey{clientID: record.clientID, linkID: record.tradingLinkID, requestID: record.requestID}
		matchingScalar := false
		for _, candidate := range scalar[key] {
			if baseSymbolNames[candidate.symbolID] == symbol && candidate.side == record.side && candidate.orderType == record.orderType && candidate.tif == record.tif &&
				candidate.decisionAt == record.decisionAt && candidate.price == record.price && candidate.qty == record.qty {
				matchingScalar = true
				break
			}
		}
		if !matchingScalar {
			result.MissingScalarDecision++
		} else if _, required := requiredScalar[key]; required {
			matchedRequired[key]++
		}
		validateVectorComponents(result, record, componentsByDecision[record.id], history, baseLinks)
	}
	for key, scalarDecisions := range requiredScalar {
		matches := matchedRequired[key]
		if matches < len(scalarDecisions) {
			result.MissingVectorDecision += int64(len(scalarDecisions) - matches)
		}
		if matches > len(scalarDecisions) {
			result.DuplicateVectorDecision += int64(matches - len(scalarDecisions))
		}
	}
	for decisionID, components := range componentsByDecision {
		if decisionID == 0 || decisionID > uint64(result.Decisions) {
			result.ExtraDecisionComponents += int64(len(components))
		}
	}
	result.Valid = result.BaseEvidenceValid && result.BaseManifestDigestMatches && result.DecisionDigestMatches && result.ComponentDigestMatches &&
		result.BadDecisionID == 0 && result.BadDecisionFields == 0 && result.MissingScalarDecision == 0 && result.MissingVectorDecision == 0 && result.DuplicateVectorDecision == 0 && result.UnknownComponentLink == 0 &&
		result.BadComponentOrdinal == 0 && result.DuplicateComponent == 0 && result.BadComponentFrontier == 0 && result.FutureComponentUse == 0 &&
		result.MissingDecisionComponents == 0 && result.ExtraDecisionComponents == 0 && result.NonzeroReserved == 0
	return result, nil
}

func auditDecisionFrontierVectorsStreaming(dir string) (result *DecisionFrontierVectorAudit, err error) {
	base, err := AuditMarketDataReceipts(dir)
	if err != nil {
		return nil, fmt.Errorf("audit base market-data evidence: %w", err)
	}
	vectorManifestRaw, err := os.ReadFile(filepath.Join(dir, "market-data-frontier-vectors-v1.json"))
	if err != nil {
		return nil, fmt.Errorf("read decision frontier-vector manifest: %w", err)
	}
	var manifest frontierVectorManifest
	if err := json.Unmarshal(vectorManifestRaw, &manifest); err != nil {
		return nil, fmt.Errorf("decode decision frontier-vector manifest: %w", err)
	}
	if manifest.SchemaVersion != 1 || manifest.Domain != "participant_information_frontier_vector_v1" ||
		manifest.Ordering != "decision_then_sorted_components" || manifest.BaseManifest != "market-data-evidence-v2.json" ||
		manifest.Decisions.File != "market-data-decision-vectors-v1.bin" || manifest.Components.File != "market-data-frontier-components-v1.bin" {
		return nil, fmt.Errorf("unsupported decision frontier-vector contract")
	}
	if len(manifest.BaseManifestDigest) != 64 || manifest.Decisions.Records < 0 || manifest.Decisions.Records > maxFrontierVectorRecords ||
		manifest.Components.Records < 0 || manifest.Components.Records > maxFrontierVectorRecords {
		return nil, fmt.Errorf("invalid or oversized decision frontier-vector artifact")
	}
	baseManifestRaw, err := os.ReadFile(filepath.Join(dir, manifest.BaseManifest))
	if err != nil {
		return nil, fmt.Errorf("read vector base manifest: %w", err)
	}
	baseManifestDigest := sha256.Sum256(baseManifestRaw)
	var baseManifest marketDataEvidenceManifest
	if err := json.Unmarshal(baseManifestRaw, &baseManifest); err != nil {
		return nil, fmt.Errorf("decode vector base manifest: %w", err)
	}
	baseLinks, _, err := validateEvidenceCatalog(baseManifest)
	if err != nil {
		return nil, err
	}
	if baseManifest.Decisions.Records < 0 || baseManifest.Decisions.Records > maxFrontierScalarRecords {
		return nil, fmt.Errorf("oversized scalar decision evidence: %d records", baseManifest.Decisions.Records)
	}
	baseSymbolNames := make(map[uint32]string, len(baseManifest.Symbols))
	for _, symbol := range baseManifest.Symbols {
		baseSymbolNames[symbol.ID] = symbol.Symbol
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
	requiredLinks := make(map[linkKey]struct{}, len(manifest.RequiredScalarLinks))
	for _, required := range manifest.RequiredScalarLinks {
		key := linkKey{clientID: required.ClientID, linkID: required.LinkID}
		if required.ClientID == 0 || required.LinkID == 0 {
			return nil, fmt.Errorf("invalid required scalar decision link")
		}
		if _, known := baseLinks[required.LinkID]; !known {
			return nil, fmt.Errorf("unknown required scalar decision link %d", required.LinkID)
		}
		if _, duplicate := requiredLinks[key]; duplicate {
			return nil, fmt.Errorf("duplicate required scalar decision link client %d link %d", required.ClientID, required.LinkID)
		}
		requiredLinks[key] = struct{}{}
	}

	vectorRaw, decisionsDigest, err := readEvidenceFile(dir, manifest.Decisions.File, decisionFrontierVectorRecordBytes, manifest.Decisions.Records, manifest.Decisions.Digest)
	if err != nil {
		return nil, err
	}
	vectorRecords := make([]vectorDecisionRecord, 0, int(manifest.Decisions.Records))
	result = &DecisionFrontierVectorAudit{
		BaseEvidenceValid:         base.Valid,
		BaseManifestDigestMatches: manifest.BaseManifestDigest == hex.EncodeToString(baseManifestDigest[:]),
		DecisionDigestMatches:     decisionsDigest,
		Decisions:                 manifest.Decisions.Records,
		Components:                manifest.Components.Records,
	}
	for offset := 0; offset < len(vectorRaw); offset += decisionFrontierVectorRecordBytes {
		if vectorRaw[offset+43] != 0 {
			result.NonzeroReserved++
		}
		vectorRecords = append(vectorRecords, decodeVectorDecision(vectorRaw[offset:offset+decisionFrontierVectorRecordBytes]))
	}
	vectorRaw = nil

	componentsRaw, componentsDigest, err := readEvidenceFile(dir, manifest.Components.File, decisionFrontierComponentRecordBytes, manifest.Components.Records, manifest.Components.Digest)
	if err != nil {
		return nil, err
	}
	result.ComponentDigestMatches = componentsDigest
	componentsByDecision := make(map[uint64][]vectorComponentRecord)
	for offset := 0; offset < len(componentsRaw); offset += decisionFrontierComponentRecordBytes {
		record := decodeVectorComponent(componentsRaw[offset : offset+decisionFrontierComponentRecordBytes])
		componentsByDecision[record.decisionID] = append(componentsByDecision[record.decisionID], record)
	}
	componentsRaw = nil

	scalarRecords, scalarDigestMatches, err := readFrontierScalarRecords(dir, baseManifest)
	if err != nil {
		return nil, err
	}
	result.BaseEvidenceValid = result.BaseEvidenceValid && scalarDigestMatches
	sort.Slice(scalarRecords, func(i, j int) bool {
		return compareScalarDecisionKey(scalarRecords[i].key, scalarRecords[j].key) < 0
	})
	requiredScalarCounts := make(map[scalarDecisionKey]int)
	for _, scalar := range scalarRecords {
		if _, required := requiredLinks[linkKey{clientID: scalar.key.clientID, linkID: scalar.key.linkID}]; required {
			requiredScalarCounts[scalar.key]++
		}
	}

	vectorsByKey := append([]vectorDecisionRecord(nil), vectorRecords...)
	sort.Slice(vectorsByKey, func(i, j int) bool {
		return compareScalarDecisionKey(frontierVectorScalarKey(vectorsByKey[i]), frontierVectorScalarKey(vectorsByKey[j])) < 0
	})
	matchedRequired := make(map[scalarDecisionKey]int)
	scalarIndex := 0
	for vectorStart := 0; vectorStart < len(vectorsByKey); {
		key := frontierVectorScalarKey(vectorsByKey[vectorStart])
		vectorEnd := vectorStart + 1
		for vectorEnd < len(vectorsByKey) && frontierVectorScalarKey(vectorsByKey[vectorEnd]) == key {
			vectorEnd++
		}
		for scalarIndex < len(scalarRecords) && compareScalarDecisionKey(scalarRecords[scalarIndex].key, key) < 0 {
			scalarIndex++
		}
		scalarStart := scalarIndex
		for scalarIndex < len(scalarRecords) && scalarRecords[scalarIndex].key == key {
			scalarIndex++
		}
		for _, vector := range vectorsByKey[vectorStart:vectorEnd] {
			symbol, symbolOK := vectorSymbols[vector.symbolID]
			matchingScalar := false
			if symbolOK {
				for _, scalar := range scalarRecords[scalarStart:scalarIndex] {
					if baseSymbolNames[scalar.symbolID] == symbol && scalar.side == vector.side && scalar.orderType == vector.orderType && scalar.tif == vector.tif &&
						scalar.decisionAt == vector.decisionAt && scalar.price == vector.price && scalar.qty == vector.qty {
						matchingScalar = true
						break
					}
				}
			}
			if !matchingScalar {
				result.MissingScalarDecision++
			} else if _, required := requiredLinks[linkKey{clientID: key.clientID, linkID: key.linkID}]; required {
				matchedRequired[key]++
			}
		}
		vectorStart = vectorEnd
	}
	for key, scalarCount := range requiredScalarCounts {
		matches := matchedRequired[key]
		if matches < scalarCount {
			result.MissingVectorDecision += int64(scalarCount - matches)
		}
		if matches > scalarCount {
			result.DuplicateVectorDecision += int64(matches - scalarCount)
		}
	}

	history, err := buildFrontierHistoryStore(dir, baseManifest)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := history.close(); closeErr != nil && err == nil {
			result = nil
			err = fmt.Errorf("close decision frontier history: %w", closeErr)
		}
	}()
	for index, record := range vectorRecords {
		if record.id != uint64(index+1) {
			result.BadDecisionID++
		}
		_, symbolOK := vectorSymbols[record.symbolID]
		if record.actorID == 0 || record.clientID == 0 || record.requestID == 0 || record.tradingLinkID == 0 || record.componentCount == 0 || !symbolOK ||
			record.side > 1 || !validVectorOrderPrice(record.orderType, record.price) || record.tif > 2 || record.decisionAt == 0 || record.qty <= 0 {
			result.BadDecisionFields++
		}
		validateVectorComponentsStored(result, record, componentsByDecision[record.id], history, baseLinks)
	}
	for decisionID, components := range componentsByDecision {
		if decisionID == 0 || decisionID > uint64(result.Decisions) {
			result.ExtraDecisionComponents += int64(len(components))
		}
	}
	result.Valid = result.BaseEvidenceValid && result.BaseManifestDigestMatches && result.DecisionDigestMatches && result.ComponentDigestMatches &&
		result.BadDecisionID == 0 && result.BadDecisionFields == 0 && result.MissingScalarDecision == 0 && result.MissingVectorDecision == 0 && result.DuplicateVectorDecision == 0 && result.UnknownComponentLink == 0 &&
		result.BadComponentOrdinal == 0 && result.DuplicateComponent == 0 && result.BadComponentFrontier == 0 && result.FutureComponentUse == 0 &&
		result.MissingDecisionComponents == 0 && result.ExtraDecisionComponents == 0 && result.NonzeroReserved == 0
	return result, nil
}

func readFrontierScalarRecords(dir string, manifest marketDataEvidenceManifest) ([]frontierScalarRecord, bool, error) {
	stream, err := openEvidenceRecordStream(dir, manifest.Decisions.File, marketDataDecisionRecordBytes, manifest.Decisions.Records, manifest.Decisions.Digest)
	if err != nil {
		return nil, false, err
	}
	defer stream.close()
	records := make([]frontierScalarRecord, 0, int(manifest.Decisions.Records))
	for {
		available, err := stream.next()
		if err != nil {
			return nil, false, err
		}
		if !available {
			break
		}
		if len(records) >= maxFrontierScalarRecords {
			return nil, false, fmt.Errorf("decision frontier scalar evidence exceeds %d records", maxFrontierScalarRecords)
		}
		decision := decodeDecision(stream.record)
		records = append(records, frontierScalarRecord{
			key:      scalarDecisionKey{clientID: decision.clientID, linkID: decision.linkID, requestID: decision.requestID},
			symbolID: decision.symbolID, side: decision.side, orderType: decision.orderType, tif: decision.tif,
			decisionAt: decision.decisionAt, price: decision.price, qty: decision.qty,
		})
	}
	return records, stream.digestMatch, nil
}

func compareScalarDecisionKey(left, right scalarDecisionKey) int {
	if left.clientID != right.clientID {
		if left.clientID < right.clientID {
			return -1
		}
		return 1
	}
	if left.linkID != right.linkID {
		if left.linkID < right.linkID {
			return -1
		}
		return 1
	}
	if left.requestID < right.requestID {
		return -1
	}
	if left.requestID > right.requestID {
		return 1
	}
	return 0
}

func frontierVectorScalarKey(record vectorDecisionRecord) scalarDecisionKey {
	return scalarDecisionKey{clientID: record.clientID, linkID: record.tradingLinkID, requestID: record.requestID}
}

func buildFrontierHistoryStore(dir string, manifest marketDataEvidenceManifest) (*frontierHistoryStore, error) {
	history, err := newFrontierHistoryStore()
	if err != nil {
		return nil, err
	}
	if manifest.Receipts.Records < 0 || manifest.Receipts.Records > maxFrontierHistoryRecords {
		history.close()
		return nil, fmt.Errorf("oversized receipt frontier history: %d records", manifest.Receipts.Records)
	}
	stream, err := openEvidenceRecordStream(dir, manifest.Receipts.File, marketDataReceiptRecordBytes, manifest.Receipts.Records, manifest.Receipts.Digest)
	if err != nil {
		history.close()
		return nil, err
	}
	defer stream.close()
	for {
		available, err := stream.next()
		if err != nil {
			history.close()
			return nil, err
		}
		if !available {
			break
		}
		record := decodeObservationView(stream.record)
		record.raw = stream.record
		if err := history.append(record); err != nil {
			history.close()
			return nil, err
		}
	}
	if err := history.prepare(); err != nil {
		history.close()
		return nil, err
	}
	return history, nil
}

func newFrontierHistoryStore() (*frontierHistoryStore, error) {
	dir, err := os.MkdirTemp("", "v2-frontier-history-")
	if err != nil {
		return nil, fmt.Errorf("create decision frontier history spill directory: %w", err)
	}
	return &frontierHistoryStore{dir: dir, files: make(map[linkKey]*frontierHistoryFile), frontiers: make(map[linkKey]auditedFrontier)}, nil
}

func (s *frontierHistoryStore) fileFor(key linkKey) (*frontierHistoryFile, error) {
	if stored, ok := s.files[key]; ok {
		return stored, nil
	}
	if len(s.files) >= maxFrontierHistoryLinks {
		return nil, fmt.Errorf("decision frontier history exceeds %d links", maxFrontierHistoryLinks)
	}
	file, err := os.Create(filepath.Join(s.dir, fmt.Sprintf("frontier-%d-%d.bin", key.clientID, key.linkID)))
	if err != nil {
		return nil, fmt.Errorf("create decision frontier history file: %w", err)
	}
	stored := &frontierHistoryFile{file: file, writer: bufio.NewWriterSize(file, frontierHistoryWriterBytes)}
	s.files[key] = stored
	return stored, nil
}

func (s *frontierHistoryStore) append(record observationRecord) error {
	key := linkKey{clientID: record.clientID, linkID: record.linkID}
	stored, err := s.fileFor(key)
	if err != nil {
		return err
	}
	if stored.count == ^uint64(0) {
		return fmt.Errorf("decision frontier history exceeds uint64 row count")
	}
	if record.ordinal != stored.count+1 && !stored.nonSequential {
		if s.nonSequentialRows+stored.count > maxFrontierNonSequentialRows {
			return fmt.Errorf("nonsequential decision frontier history exceeds %d rows", maxFrontierNonSequentialRows)
		}
		if err := stored.writer.Flush(); err != nil {
			return fmt.Errorf("flush nonsequential decision frontier history: %w", err)
		}
		stored.byOrdinal = make(map[uint64]auditedFrontier, stored.count)
		for index := uint64(0); index < stored.count; index++ {
			frontier, err := readFrontierHistoryRecord(stored.file, index)
			if err != nil {
				return err
			}
			stored.byOrdinal[frontier.ordinal] = frontier
		}
		stored.nonSequential = true
		s.nonSequentialRows += stored.count
	}
	previous := s.frontiers[key]
	chain := sha256.New()
	_, _ = chain.Write(previous.digest[:])
	_, _ = chain.Write(record.raw)
	var digest [16]byte
	copy(digest[:], chain.Sum(nil))
	frontier := auditedFrontier{ordinal: record.ordinal, deliveredAt: record.deliveredAt, digest: digest}
	s.frontiers[key] = frontier
	var raw [frontierHistoryRecordBytes]byte
	binary.BigEndian.PutUint64(raw[0:8], record.ordinal)
	binary.BigEndian.PutUint64(raw[8:16], uint64(record.deliveredAt))
	copy(raw[16:32], digest[:])
	if _, err := stored.writer.Write(raw[:]); err != nil {
		return fmt.Errorf("spill decision frontier history: %w", err)
	}
	if stored.nonSequential {
		if _, exists := stored.byOrdinal[frontier.ordinal]; !exists {
			if s.nonSequentialRows >= maxFrontierNonSequentialRows {
				return fmt.Errorf("nonsequential decision frontier history exceeds %d rows", maxFrontierNonSequentialRows)
			}
			s.nonSequentialRows++
		}
		stored.byOrdinal[frontier.ordinal] = frontier
	}
	stored.count++
	return nil
}

func (s *frontierHistoryStore) prepare() error {
	for key, stored := range s.files {
		if err := stored.writer.Flush(); err != nil {
			return fmt.Errorf("flush decision frontier history client %d link %d: %w", key.clientID, key.linkID, err)
		}
	}
	return nil
}

func (s *frontierHistoryStore) lookup(key vectorFrontierKey) (auditedFrontier, bool) {
	stored, ok := s.files[linkKey{clientID: key.clientID, linkID: key.linkID}]
	if !ok || key.ordinal == 0 || key.ordinal-1 > uint64(^uint64(0)>>1)/frontierHistoryRecordBytes {
		return auditedFrontier{}, false
	}
	if stored.nonSequential {
		frontier, exists := stored.byOrdinal[key.ordinal]
		return frontier, exists
	}
	if key.ordinal > stored.count {
		return auditedFrontier{}, false
	}
	frontier, err := readFrontierHistoryRecord(stored.file, key.ordinal-1)
	if err != nil {
		return auditedFrontier{}, false
	}
	return frontier, frontier.ordinal == key.ordinal
}

func readFrontierHistoryRecord(file *os.File, index uint64) (auditedFrontier, error) {
	if index > uint64(^uint64(0)>>1)/frontierHistoryRecordBytes {
		return auditedFrontier{}, fmt.Errorf("decision frontier history offset overflows int64")
	}
	var raw [frontierHistoryRecordBytes]byte
	offset := int64(index * frontierHistoryRecordBytes)
	read, err := file.ReadAt(raw[:], offset)
	if err != nil {
		return auditedFrontier{}, fmt.Errorf("read decision frontier history record %d: %w", index, err)
	}
	if read != len(raw) {
		return auditedFrontier{}, fmt.Errorf("short decision frontier history record %d", index)
	}
	var digest [16]byte
	copy(digest[:], raw[16:32])
	frontier := auditedFrontier{ordinal: binary.BigEndian.Uint64(raw[0:8]), deliveredAt: int64(binary.BigEndian.Uint64(raw[8:16])), digest: digest}
	return frontier, nil
}

func (s *frontierHistoryStore) close() error {
	var first error
	for key, stored := range s.files {
		if err := stored.writer.Flush(); err != nil && first == nil {
			first = fmt.Errorf("flush decision frontier history client %d link %d: %w", key.clientID, key.linkID, err)
		}
		if err := stored.file.Close(); err != nil && first == nil {
			first = fmt.Errorf("close decision frontier history client %d link %d: %w", key.clientID, key.linkID, err)
		}
	}
	if err := os.RemoveAll(s.dir); err != nil && first == nil {
		first = err
	}
	return first
}

func validateVectorComponentsStored(result *DecisionFrontierVectorAudit, decision vectorDecisionRecord, components []vectorComponentRecord, history *frontierHistoryStore, links map[uint32]struct{}) {
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
		want, exists := history.lookup(vectorFrontierKey{clientID: component.clientID, linkID: component.linkID, ordinal: component.frontier.ordinal})
		if !exists || want != component.frontier {
			result.BadComponentFrontier++
		}
		if component.frontier.deliveredAt > decision.decisionAt {
			result.FutureComponentUse++
		}
	}
}

// validVectorOrderPrice mirrors the persisted request protocol without
// importing simulator code. Market requests intentionally encode their
// unspecified limit as zero. A limit request carries an explicit signed
// numeric price: this information-boundary auditor does not reimplement the
// receiving instrument's domain policy. Neither form is an availability
// sentinel.
func validVectorOrderPrice(orderType uint8, price int64) bool {
	switch orderType {
	case 0: // types.Market
		return price == 0
	case 1: // types.LimitOrder
		return true
	default:
		return false
	}
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
