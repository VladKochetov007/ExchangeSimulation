package simulation

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/analysis"
	"exchange_sim/exchange"
)

func writeDecisionVectorFixture(t *testing.T, dir string) MarketDataFrontier {
	return writeDecisionVectorFixtureOrder(t, dir, exchange.LimitOrder, 99)
}

func writeDecisionVectorFixtureOrder(t *testing.T, dir string, orderType exchange.OrderType, price int64) MarketDataFrontier {
	t.Helper()
	receipts, err := NewMarketDataReceiptRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	if linkID := receipts.RegisterLink("north", "north/spot_maker/client/11", "spot_maker"); linkID == 0 {
		t.Fatal("fixture link registration failed")
	}
	fingerprint := [16]byte{1}
	schedule := MarketDataSchedule{
		ClientID: 11, SourceVenue: "north", Link: "north/spot_maker/client/11", Symbol: "ABC/USD",
		Type: exchange.MDSnapshot, Sequence: 7, Fingerprint: fingerprint, PublishedAt: 100, ScheduledAt: 110, LinkOrdinal: 1,
	}
	if linkID := receipts.RecordSchedule(schedule); linkID == 0 {
		t.Fatal("schedule did not receive a link ID")
	}
	frontier := receipts.RecordReceipt(MarketDataReceipt{MarketDataSchedule: schedule, DeliveredAt: 110})
	if frontier.LinkID == 0 || frontier.Ordinal != 1 {
		t.Fatalf("bad fixture frontier: %+v", frontier)
	}
	receipts.RecordDecision(MarketDataDecision{
		ClientID: 11, SourceVenue: "north", Link: "north/spot_maker/client/11", Symbol: "ABC/USD", RequestID: 31,
		Side: exchange.Buy, OrderType: orderType, TimeInForce: exchange.GTC, Price: price, Qty: 3, DecisionAt: 120, Frontier: frontier,
	})
	if err := receipts.Finalize(120); err != nil {
		t.Fatal(err)
	}
	vectors, err := NewDecisionFrontierVectorRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := vectors.RequireScalarDecisionLink(11, frontier.LinkID); err != nil {
		t.Fatal(err)
	}
	vectors.Record(DecisionFrontierVector{
		ActorID: 99, ClientID: 11, TradingLinkID: frontier.LinkID, Symbol: "ABC/USD", RequestID: 31,
		Side: exchange.Buy, OrderType: orderType, TimeInForce: exchange.GTC, Price: price, Qty: 3, DecisionAt: 120,
		Components: []DecisionFrontierComponent{{ClientID: 11, Frontier: frontier}},
	})
	if err := vectors.Finalize(filepath.Join(dir, "market-data-evidence-v2.json")); err != nil {
		t.Fatal(err)
	}
	return frontier
}

func TestDecisionFrontierVectorsAuditMultiFeedDecisionContract(t *testing.T) {
	dir := t.TempDir()
	writeDecisionVectorFixture(t, dir)
	audit, err := analysis.AuditDecisionFrontierVectors(dir)
	if err != nil || !audit.Valid || audit.Decisions != 1 || audit.Components != 1 {
		t.Fatalf("valid decision frontier vector rejected: audit=%+v err=%v", audit, err)
	}
}

func TestDecisionFrontierVectorAcceptsMarketProtocolPriceZero(t *testing.T) {
	dir := t.TempDir()
	writeDecisionVectorFixtureOrder(t, dir, exchange.Market, 0)
	audit, err := analysis.AuditDecisionFrontierVectors(dir)
	if err != nil || !audit.Valid || audit.BadDecisionFields != 0 {
		t.Fatalf("market protocol price zero rejected as unavailable: audit=%+v err=%v", audit, err)
	}
}

func TestDecisionFrontierVectorRetainsSignedLimitPrices(t *testing.T) {
	for _, limit := range []int64{-99, 0, 99} {
		t.Run(fmt.Sprintf("limit_%d", limit), func(t *testing.T) {
			dir := t.TempDir()
			writeDecisionVectorFixtureOrder(t, dir, exchange.LimitOrder, limit)
			audit, err := analysis.AuditDecisionFrontierVectors(dir)
			if err != nil || !audit.Valid || audit.BadDecisionFields != 0 {
				t.Fatalf("signed limit price %d rejected by evidence contract: audit=%+v err=%v", limit, audit, err)
			}
		})
	}
}

func TestDecisionFrontierVectorAuditCatchesFutureComponentMutation(t *testing.T) {
	dir := t.TempDir()
	writeDecisionVectorFixture(t, dir)
	path := filepath.Join(dir, "market-data-frontier-components-v1.bin")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	binary.BigEndian.PutUint64(raw[32:40], 121) // component receipt after the order decision at 120
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	manifestPath := filepath.Join(dir, "market-data-frontier-vectors-v1.json")
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatal(err)
	}
	components := manifest["components"].(map[string]any)
	components["digest"] = hex.EncodeToString(digest[:])
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, append(encoded, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	audit, err := analysis.AuditDecisionFrontierVectors(dir)
	if err != nil || audit.Valid || audit.FutureComponentUse == 0 {
		t.Fatalf("future component mutation survived: audit=%+v err=%v", audit, err)
	}
}

func TestDecisionFrontierVectorAuditCatchesDroppedComponentMutation(t *testing.T) {
	dir := t.TempDir()
	writeDecisionVectorFixture(t, dir)
	path := filepath.Join(dir, "market-data-frontier-components-v1.bin")
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(nil)
	manifestPath := filepath.Join(dir, "market-data-frontier-vectors-v1.json")
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatal(err)
	}
	components := manifest["components"].(map[string]any)
	components["records"] = float64(0)
	components["digest"] = hex.EncodeToString(digest[:])
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, append(encoded, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	audit, err := analysis.AuditDecisionFrontierVectors(dir)
	if err != nil || audit.Valid || audit.MissingDecisionComponents == 0 {
		t.Fatalf("dropped component mutation survived: audit=%+v err=%v", audit, err)
	}
}

// A vector can be internally well formed yet absent for one audited scalar
// gateway decision. The required-link declaration makes that omission
// independently observable instead of treating it as an empty information
// set.
func TestDecisionFrontierVectorAuditCatchesDroppedDecisionMutation(t *testing.T) {
	dir := t.TempDir()
	writeDecisionVectorFixture(t, dir)
	for _, file := range []string{"market-data-decision-vectors-v1.bin", "market-data-frontier-components-v1.bin"} {
		if err := os.WriteFile(filepath.Join(dir, file), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}
	emptyDigest := sha256.Sum256(nil)
	manifestPath := filepath.Join(dir, "market-data-frontier-vectors-v1.json")
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"decisions", "components"} {
		artifact := manifest[name].(map[string]any)
		artifact["records"] = float64(0)
		artifact["digest"] = hex.EncodeToString(emptyDigest[:])
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, append(encoded, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	audit, err := analysis.AuditDecisionFrontierVectors(dir)
	if err != nil || audit.Valid || audit.MissingVectorDecision != 1 {
		t.Fatalf("dropped decision vector survived: audit=%+v err=%v", audit, err)
	}
}

func TestDecisionFrontierVectorRecorderRejectsEmptyObservationComponent(t *testing.T) {
	dir := t.TempDir()
	recorder, err := NewDecisionFrontierVectorRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	recorder.Record(DecisionFrontierVector{
		ActorID: 1, ClientID: 2, TradingLinkID: 3, Symbol: "ABC/USD", RequestID: 4,
		Side: exchange.Buy, OrderType: exchange.LimitOrder, TimeInForce: exchange.GTC, Price: 1, Qty: 1, DecisionAt: 1,
		Components: []DecisionFrontierComponent{{ClientID: 2, Frontier: MarketDataFrontier{LinkID: 3}}},
	})
	base := filepath.Join(dir, "market-data-evidence-v2.json")
	if err := os.WriteFile(base, []byte("{}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := recorder.Finalize(base); err == nil {
		t.Fatal("empty decision frontier component finalized successfully")
	}
}

func TestReceiptFinalizeRetainsManifestWriteFailure(t *testing.T) {
	dir := t.TempDir()
	recorder, err := NewMarketDataReceiptRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	recorder.manifestPath = filepath.Join(dir, "missing", "market-data-evidence-v2.json")
	first := recorder.Finalize(1)
	if first == nil {
		t.Fatal("Finalize succeeded despite an unwritable manifest path")
	}
	second := recorder.Finalize(2)
	if second == nil || second.Error() != first.Error() {
		t.Fatalf("repeat Finalize error = %v, want the retained first error %v", second, first)
	}
}

func TestDecisionFrontierFinalizeRetainsBaseManifestFailure(t *testing.T) {
	dir := t.TempDir()
	recorder, err := NewDecisionFrontierVectorRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "missing", "market-data-evidence-v2.json")
	first := recorder.Finalize(missing)
	if first == nil {
		t.Fatal("Finalize succeeded despite a missing base manifest")
	}
	second := recorder.Finalize(missing)
	if second == nil || second.Error() != first.Error() {
		t.Fatalf("repeat Finalize error = %v, want the retained first error %v", second, first)
	}
}

// This joins the real actor pre-send hook, delayed gateway receipt state, the
// ordinary scalar decision ledger, and the vector artifact. A hand-written
// vector fixture alone would not prove that the actor-facing integration is
// before request latency.
func TestDecisionFrontierVectorJoinsRealActorGatewayDecision(t *testing.T) {
	dir := t.TempDir()
	receipts, err := NewMarketDataReceiptRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	vectors, err := NewDecisionFrontierVectorRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	clock := NewSimulatedClock(int64(time.Second))
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	inner := &boundaryGateway{id: 9, resp: make(chan exchange.Response, 1), md: make(chan *exchange.MarketDataMsg, 1)}
	delayed := NewDelayedGateway(inner, nil, nil, NewConstantLatency(10*time.Millisecond))
	delayed.UseScheduler(scheduler, clock)
	delayed.SetMarketDataReceiptRecorder(receipts, "north", "north/spot_maker/client/9", "spot_maker")
	if err := delayed.EnableDeterministicPhases(); err != nil {
		t.Fatal(err)
	}
	delayed.Start()
	defer delayed.Stop()
	inner.md <- &exchange.MarketDataMsg{Type: exchange.MDSnapshot, Symbol: "ABC/USD", SeqNum: 1, Timestamp: clock.NowUnixNano(), Data: &exchange.BookSnapshot{}}
	if !delayed.PumpDeterministicPhase() {
		t.Fatal("receipt source was not scheduled")
	}
	clock.Advance(10 * time.Millisecond)
	if !delayed.DrainDeterministicPhaseEgress() {
		t.Fatal("receipt source was not delivered")
	}
	participant := actor.NewBaseActor(99, delayed)
	participant.SetOrderDecisionObserver(func(request exchange.Request) {
		frontier := delayed.MarketDataFrontier()
		vectors.Record(DecisionFrontierVector{
			ActorID: participant.ID(), ClientID: delayed.ID(), TradingLinkID: frontier.LinkID, Symbol: request.OrderReq.Symbol,
			RequestID: request.OrderReq.RequestID, Side: request.OrderReq.Side, OrderType: request.OrderReq.Type,
			TimeInForce: request.OrderReq.TimeInForce, Price: request.OrderReq.Price, Qty: request.OrderReq.Qty,
			DecisionAt: clock.NowUnixNano(), Components: []DecisionFrontierComponent{{ClientID: delayed.ID(), Frontier: frontier}},
		})
	})
	participant.SubmitOrder("ABC/USD", exchange.Buy, exchange.LimitOrder, 99, 3)
	if err := receipts.Finalize(clock.NowUnixNano()); err != nil {
		t.Fatal(err)
	}
	if err := vectors.Finalize(filepath.Join(dir, "market-data-evidence-v2.json")); err != nil {
		t.Fatal(err)
	}
	audit, err := analysis.AuditDecisionFrontierVectors(dir)
	if err != nil || !audit.Valid || audit.MissingScalarDecision != 0 {
		t.Fatalf("real actor/gateway vector join invalid: audit=%+v err=%v", audit, err)
	}
}
