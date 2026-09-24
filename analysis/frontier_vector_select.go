package analysis

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	etypes "exchange_sim/types"
)

type DecisionFrontierVectorSelection struct {
	Symbol       string
	ClientByLink map[uint32]uint64
}

type SelectedDecisionFrontierVector struct {
	DecisionID     uint64
	ActorID        uint64
	ClientID       uint64
	RequestID      uint64
	TradingLinkID  uint32
	Symbol         string
	Side           etypes.Side
	OrderType      etypes.OrderType
	TimeInForce    etypes.TimeInForce
	Price          int64
	Qty            int64
	DecisionAt     int64
	ComponentCount uint32
	Components     []SelectedDecisionFrontierComponent
}

type SelectedDecisionFrontierComponent struct {
	ClientID    uint64
	LinkID      uint32
	Ordinal     uint64
	DeliveredAt int64
	Digest      [16]byte
}

// SelectVerifiedDecisionFrontierVectors exposes typed records only after the
// complete independent scalar/vector audit passes. A second bounded sidecar
// read selects the requested links; it does not re-extract large event logs.
func SelectVerifiedDecisionFrontierVectors(dir string, selection DecisionFrontierVectorSelection) ([]SelectedDecisionFrontierVector, error) {
	if dir == "" || selection.Symbol == "" || len(selection.ClientByLink) == 0 {
		return nil, fmt.Errorf("decision vector selection: invalid request")
	}
	for linkID, clientID := range selection.ClientByLink {
		if linkID == 0 || clientID == 0 {
			return nil, fmt.Errorf("decision vector selection: invalid link or client")
		}
	}
	manifestPath := filepath.Join(dir, "market-data-frontier-vectors-v1.json")
	manifestBefore, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("decision vector selection: read manifest: %w", err)
	}
	audit, err := AuditDecisionFrontierVectors(dir)
	if err != nil {
		return nil, fmt.Errorf("decision vector selection: audit: %w", err)
	}
	if audit == nil || !audit.Valid {
		return nil, fmt.Errorf("decision vector selection: frontier evidence is invalid")
	}
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("decision vector selection: read manifest: %w", err)
	}
	if !bytes.Equal(manifestBefore, manifestRaw) {
		return nil, fmt.Errorf("decision vector selection: manifest changed during audit")
	}
	var manifest frontierVectorManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return nil, fmt.Errorf("decision vector selection: decode manifest: %w", err)
	}
	for linkID, clientID := range selection.ClientByLink {
		found := false
		for _, required := range manifest.RequiredScalarLinks {
			if required.LinkID == linkID && required.ClientID == clientID {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("decision vector selection: selected client/link is not required by audited manifest")
		}
	}
	vectorRaw, digestMatches, err := readEvidenceFile(dir, manifest.Decisions.File, decisionFrontierVectorRecordBytes, manifest.Decisions.Records, manifest.Decisions.Digest)
	if err != nil {
		return nil, fmt.Errorf("decision vector selection: read sidecar: %w", err)
	}
	if !digestMatches {
		return nil, fmt.Errorf("decision vector selection: sidecar digest changed after audit")
	}
	symbols := make(map[uint32]string, len(manifest.Symbols))
	for _, symbol := range manifest.Symbols {
		symbols[symbol.ID] = symbol.Symbol
	}
	var selected []SelectedDecisionFrontierVector
	for offset := 0; offset < len(vectorRaw); offset += decisionFrontierVectorRecordBytes {
		record := decodeVectorDecision(vectorRaw[offset : offset+decisionFrontierVectorRecordBytes])
		clientID, wanted := selection.ClientByLink[record.tradingLinkID]
		if !wanted {
			continue
		}
		if clientID != record.clientID {
			return nil, fmt.Errorf("decision vector selection: link %d belongs to unexpected client", record.tradingLinkID)
		}
		if symbols[record.symbolID] != selection.Symbol {
			return nil, fmt.Errorf("decision vector selection: selected actor link has a decision on another symbol")
		}
		selected = append(selected, SelectedDecisionFrontierVector{
			DecisionID: record.id, ActorID: record.actorID, ClientID: record.clientID,
			RequestID: record.requestID, TradingLinkID: record.tradingLinkID, Symbol: selection.Symbol,
			Side: etypes.Side(record.side), OrderType: etypes.OrderType(record.orderType), TimeInForce: etypes.TimeInForce(record.tif),
			Price: record.price, Qty: record.qty, DecisionAt: record.decisionAt, ComponentCount: record.componentCount,
		})
	}
	componentRaw, componentDigestMatches, err := readEvidenceFile(dir, manifest.Components.File, decisionFrontierComponentRecordBytes, manifest.Components.Records, manifest.Components.Digest)
	if err != nil {
		return nil, fmt.Errorf("decision vector selection: read component sidecar: %w", err)
	}
	if !componentDigestMatches {
		return nil, fmt.Errorf("decision vector selection: component digest changed after audit")
	}
	selectedByID := make(map[uint64]int, len(selected))
	for index, decision := range selected {
		selectedByID[decision.DecisionID] = index
	}
	for offset := 0; offset < len(componentRaw); offset += decisionFrontierComponentRecordBytes {
		component := decodeVectorComponent(componentRaw[offset : offset+decisionFrontierComponentRecordBytes])
		index, wanted := selectedByID[component.decisionID]
		if !wanted {
			continue
		}
		selected[index].Components = append(selected[index].Components, SelectedDecisionFrontierComponent{
			ClientID: component.clientID, LinkID: component.linkID,
			Ordinal: component.frontier.ordinal, DeliveredAt: component.frontier.deliveredAt,
			Digest: component.frontier.digest,
		})
	}
	for _, decision := range selected {
		if uint32(len(decision.Components)) != decision.ComponentCount {
			return nil, fmt.Errorf("decision vector selection: selected decision has incomplete components")
		}
	}
	return selected, nil
}
