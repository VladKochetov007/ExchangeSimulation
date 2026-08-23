package simulation

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"exchange_sim/types"
)

// Decision-frontier vectors extend, rather than alter, the V2-0 scalar
// receipt contract. A participant with multiple public feeds cannot honestly
// reduce its information set to the trading link's one scalar frontier.
const (
	DecisionFrontierVectorRecordBytes    = 72
	DecisionFrontierComponentRecordBytes = 56
)

const (
	decisionFrontierVectorDomain   = "participant_information_frontier_vector_v1"
	decisionFrontierVectorOrdering = "decision_then_sorted_components"
)

// DecisionFrontierComponent is one feed prefix available to an actor at an
// order decision. ClientID is feed-account identity; it can differ from the
// trading account when an actor owns a remote public-feed session.
type DecisionFrontierComponent struct {
	ClientID uint64
	Frontier MarketDataFrontier
}

// DecisionFrontierVector ties one emitted order to all locally available feed
// prefixes. The scalar V2-0 ledger separately attests the actual gateway send;
// the vector sidecar binds a multi-feed actor's decision to its cache inputs.
type DecisionFrontierVector struct {
	ActorID       uint64
	ClientID      uint64
	TradingLinkID uint32
	Symbol        string
	RequestID     uint64
	Side          types.Side
	OrderType     types.OrderType
	TimeInForce   types.TimeInForce
	Price         int64
	Qty           int64
	DecisionAt    int64
	Components    []DecisionFrontierComponent
}

type decisionFrontierVectorArtifact struct {
	SchemaVersion       int                            `json:"schema_version"`
	Domain              string                         `json:"domain"`
	Ordering            string                         `json:"ordering"`
	BaseManifest        string                         `json:"base_manifest"`
	BaseManifestDigest  string                         `json:"base_manifest_digest"`
	RequiredScalarLinks []decisionFrontierRequiredLink `json:"required_scalar_decision_links"`
	Decisions           evidenceFileArtifact           `json:"decisions"`
	Components          evidenceFileArtifact           `json:"components"`
	Symbols             []receiptSymbolCatalog         `json:"symbols"`
}

// decisionFrontierRequiredLink declares a scalar V2-0 gateway link whose
// every persisted order decision must be represented by exactly one V3
// vector. Without this inverse declaration a missing vector could look like a
// merely absent observation rather than dropped decision-side evidence.
type decisionFrontierRequiredLink struct {
	ClientID uint64 `json:"client_id"`
	LinkID   uint32 `json:"link_id"`
}

// DecisionFrontierVectorRecorder is evidence-only. It creates no scheduler
// events, consumes no RNG, and has no read path into the simulation.
type DecisionFrontierVectorRecorder struct {
	mu sync.Mutex

	dir          string
	decisions    *evidenceWriter
	components   *evidenceWriter
	symbols      map[string]uint32
	symbolRows   []receiptSymbolCatalog
	required     map[decisionFrontierRequiredLink]struct{}
	nextDecision uint64
	writeErr     error
	finalized    bool
}

func NewDecisionFrontierVectorRecorder(dir string) (*DecisionFrontierVectorRecorder, error) {
	decisions, err := newEvidenceWriter(filepath.Join(dir, "market-data-decision-vectors-v1.bin"))
	if err != nil {
		return nil, fmt.Errorf("create decision frontier-vector sidecar: %w", err)
	}
	components, err := newEvidenceWriter(filepath.Join(dir, "market-data-frontier-components-v1.bin"))
	if err != nil {
		_ = decisions.close()
		return nil, fmt.Errorf("create decision frontier-component sidecar: %w", err)
	}
	return &DecisionFrontierVectorRecorder{
		dir: dir, decisions: decisions, components: components,
		symbols: make(map[string]uint32), required: make(map[decisionFrontierRequiredLink]struct{}),
	}, nil
}

// RequireScalarDecisionLink declares that every V2-0 scalar decision emitted
// by this trading client/link must have one V3 vector. It is setup-only and
// does not affect actor or exchange state.
func (r *DecisionFrontierVectorRecorder) RequireScalarDecisionLink(clientID uint64, linkID uint32) error {
	if r == nil {
		return fmt.Errorf("decision frontier-vector recorder is nil")
	}
	if clientID == 0 || linkID == 0 {
		return fmt.Errorf("invalid required scalar decision link")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finalized || r.writeErr != nil {
		return fmt.Errorf("decision frontier-vector recorder is unavailable")
	}
	r.required[decisionFrontierRequiredLink{ClientID: clientID, LinkID: linkID}] = struct{}{}
	return nil
}

// Record records an already constructed order immediately before it enters a
// gateway. Components are copied and sorted by feed account/link, so a caller
// cannot accidentally make evidence order depend on map iteration.
func (r *DecisionFrontierVectorRecorder) Record(decision DecisionFrontierVector) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finalized || r.writeErr != nil {
		return
	}
	if decision.ActorID == 0 || decision.ClientID == 0 || decision.TradingLinkID == 0 || decision.Symbol == "" ||
		decision.RequestID == 0 || decision.DecisionAt == 0 || !validDecisionOrderPrice(decision.OrderType, decision.Price) || decision.Qty <= 0 || len(decision.Components) == 0 {
		r.writeErr = fmt.Errorf("invalid decision frontier vector")
		return
	}
	components := append([]DecisionFrontierComponent(nil), decision.Components...)
	sort.Slice(components, func(i, j int) bool {
		if components[i].ClientID != components[j].ClientID {
			return components[i].ClientID < components[j].ClientID
		}
		return components[i].Frontier.LinkID < components[j].Frontier.LinkID
	})
	for index, component := range components {
		if component.ClientID == 0 || component.Frontier.LinkID == 0 || component.Frontier.Ordinal == 0 ||
			component.Frontier.DeliveredAt == 0 || component.Frontier.Digest == ([16]byte{}) {
			r.writeErr = fmt.Errorf("invalid decision frontier component %d", index+1)
			return
		}
		if index > 0 && component.ClientID == components[index-1].ClientID && component.Frontier.LinkID == components[index-1].Frontier.LinkID {
			r.writeErr = fmt.Errorf("duplicate decision frontier component for client %d link %d", component.ClientID, component.Frontier.LinkID)
			return
		}
	}
	symbolID := r.symbolIDLocked(decision.Symbol)
	r.nextDecision++
	decisionID := r.nextDecision
	var raw [DecisionFrontierVectorRecordBytes]byte
	binary.BigEndian.PutUint64(raw[0:8], decisionID)
	binary.BigEndian.PutUint64(raw[8:16], decision.ActorID)
	binary.BigEndian.PutUint64(raw[16:24], decision.ClientID)
	binary.BigEndian.PutUint64(raw[24:32], decision.RequestID)
	binary.BigEndian.PutUint32(raw[32:36], decision.TradingLinkID)
	binary.BigEndian.PutUint32(raw[36:40], symbolID)
	raw[40] = byte(decision.Side)
	raw[41] = byte(decision.OrderType)
	raw[42] = byte(decision.TimeInForce)
	binary.BigEndian.PutUint32(raw[44:48], uint32(len(components)))
	binary.BigEndian.PutUint64(raw[48:56], uint64(decision.DecisionAt))
	binary.BigEndian.PutUint64(raw[56:64], uint64(decision.Price))
	binary.BigEndian.PutUint64(raw[64:72], uint64(decision.Qty))
	if err := r.decisions.write(raw[:]); err != nil {
		r.writeErr = fmt.Errorf("write decision frontier vector: %w", err)
		return
	}
	for index, component := range components {
		var componentRaw [DecisionFrontierComponentRecordBytes]byte
		binary.BigEndian.PutUint64(componentRaw[0:8], decisionID)
		binary.BigEndian.PutUint64(componentRaw[8:16], component.ClientID)
		binary.BigEndian.PutUint32(componentRaw[16:20], component.Frontier.LinkID)
		binary.BigEndian.PutUint32(componentRaw[20:24], uint32(index+1))
		binary.BigEndian.PutUint64(componentRaw[24:32], component.Frontier.Ordinal)
		binary.BigEndian.PutUint64(componentRaw[32:40], uint64(component.Frontier.DeliveredAt))
		copy(componentRaw[40:56], component.Frontier.Digest[:])
		if err := r.components.write(componentRaw[:]); err != nil {
			r.writeErr = fmt.Errorf("write decision frontier component: %w", err)
			return
		}
	}
}

// validDecisionOrderPrice distinguishes a market request's protocol price of
// zero from an unavailable market reference. Limit decisions need a positive
// limit; Market requests deliberately carry zero because their price is
// determined by later executable book levels, not this request field.
func validDecisionOrderPrice(orderType types.OrderType, price int64) bool {
	switch orderType {
	case types.Market:
		return price == 0
	case types.LimitOrder:
		return price > 0
	default:
		return false
	}
}

func (r *DecisionFrontierVectorRecorder) symbolIDLocked(symbol string) uint32 {
	if id, exists := r.symbols[symbol]; exists {
		return id
	}
	id := uint32(len(r.symbolRows) + 1)
	r.symbols[symbol] = id
	r.symbolRows = append(r.symbolRows, receiptSymbolCatalog{ID: id, Symbol: symbol})
	return id
}

// Finalize binds the vector artifact to the immutable V2-0 receipt manifest.
// The base manifest must already be finalized, normally after the simulation's
// terminal fixed point.
func (r *DecisionFrontierVectorRecorder) Finalize(baseManifestPath string) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finalized {
		return r.writeErr
	}
	r.finalized = true
	for _, writer := range []*evidenceWriter{r.decisions, r.components} {
		if err := writer.close(); err != nil && r.writeErr == nil {
			r.writeErr = fmt.Errorf("close decision frontier-vector sidecar: %w", err)
		}
	}
	if r.writeErr != nil {
		return r.writeErr
	}
	baseRaw, err := os.ReadFile(baseManifestPath)
	if err != nil {
		return fmt.Errorf("read base market-data evidence manifest: %w", err)
	}
	baseDigest := sha256.Sum256(baseRaw)
	symbols := append([]receiptSymbolCatalog(nil), r.symbolRows...)
	sort.Slice(symbols, func(i, j int) bool { return symbols[i].ID < symbols[j].ID })
	required := make([]decisionFrontierRequiredLink, 0, len(r.required))
	for link := range r.required {
		required = append(required, link)
	}
	sort.Slice(required, func(i, j int) bool {
		if required[i].ClientID != required[j].ClientID {
			return required[i].ClientID < required[j].ClientID
		}
		return required[i].LinkID < required[j].LinkID
	})
	artifact := decisionFrontierVectorArtifact{
		SchemaVersion: 1, Domain: decisionFrontierVectorDomain, Ordering: decisionFrontierVectorOrdering,
		BaseManifest: filepath.Base(baseManifestPath), BaseManifestDigest: hex.EncodeToString(baseDigest[:]),
		RequiredScalarLinks: required,
		Decisions:           r.decisions.artifact("market-data-decision-vectors-v1.bin"),
		Components:          r.components.artifact("market-data-frontier-components-v1.bin"), Symbols: symbols,
	}
	raw, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal decision frontier-vector manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(r.dir, "market-data-frontier-vectors-v1.json"), append(raw, '\n'), 0644); err != nil {
		return fmt.Errorf("write decision frontier-vector manifest: %w", err)
	}
	return nil
}
