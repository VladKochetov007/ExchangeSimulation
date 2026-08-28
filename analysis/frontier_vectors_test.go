package analysis

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"exchange_sim/exchange"
	"exchange_sim/simulation"
)

func TestDecisionFrontierVectorsStreamingMatchesBufferedOracle(t *testing.T) {
	dir := writeFrontierVectorOracleFixture(t)

	streaming, err := AuditDecisionFrontierVectors(dir)
	if err != nil {
		t.Fatal(err)
	}
	buffered, err := auditDecisionFrontierVectorsBuffered(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(streaming, buffered) {
		t.Fatalf("streaming frontier audit differs from buffered oracle:\nstreaming=%+v\nbuffered=%+v", streaming, buffered)
	}
}

func TestDecisionFrontierVectorsStreamingMatchesBufferedForNonsequentialReceipt(t *testing.T) {
	dir := writeFrontierVectorOracleFixture(t)
	receiptPath := filepath.Join(dir, "market-data-receipts-v2.bin")
	receiptRaw, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	binary.BigEndian.PutUint64(receiptRaw[68:76], 3)
	if err := os.WriteFile(receiptPath, receiptRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	receiptDigest := sha256.Sum256(receiptRaw)
	updateFixtureManifestDigest(t, filepath.Join(dir, "market-data-evidence-v2.json"), "receipts", hex.EncodeToString(receiptDigest[:]))

	componentPath := filepath.Join(dir, "market-data-frontier-components-v1.bin")
	componentRaw, err := os.ReadFile(componentPath)
	if err != nil {
		t.Fatal(err)
	}
	binary.BigEndian.PutUint64(componentRaw[24:32], 3)
	binary.BigEndian.PutUint64(componentRaw[32:40], 110)
	componentDigest := sha256.Sum256(receiptRaw)
	copy(componentRaw[40:56], componentDigest[:16])
	if err := os.WriteFile(componentPath, componentRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	vectorDigest := sha256.Sum256(componentRaw)
	updateFixtureManifestDigest(t, filepath.Join(dir, "market-data-frontier-vectors-v1.json"), "components", hex.EncodeToString(vectorDigest[:]))

	streaming, err := AuditDecisionFrontierVectors(dir)
	if err != nil {
		t.Fatal(err)
	}
	buffered, err := auditDecisionFrontierVectorsBuffered(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(streaming, buffered) {
		t.Fatalf("nonsequential streaming frontier audit differs from buffered oracle:\nstreaming=%+v\nbuffered=%+v", streaming, buffered)
	}
}

func writeFrontierVectorOracleFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	receipts, err := simulation.NewMarketDataReceiptRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	link := receipts.RegisterLink("north", "north/spot_maker/client/11", "spot_maker")
	schedule := simulation.MarketDataSchedule{
		ClientID: 11, SourceVenue: "north", Link: "north/spot_maker/client/11", Symbol: "ABC/USD",
		Type: exchange.MDSnapshot, Sequence: 7, Fingerprint: [16]byte{1}, PublishedAt: 100, ScheduledAt: 110, LinkOrdinal: 1,
	}
	receipts.RecordSchedule(schedule)
	frontier := receipts.RecordReceipt(simulation.MarketDataReceipt{MarketDataSchedule: schedule, DeliveredAt: 110})
	receipts.RecordDecision(simulation.MarketDataDecision{
		ClientID: 11, SourceVenue: "north", Link: schedule.Link, Symbol: schedule.Symbol, RequestID: 31,
		Side: exchange.Buy, OrderType: exchange.LimitOrder, TimeInForce: exchange.GTC, Price: 99, Qty: 3, DecisionAt: 120, Frontier: frontier,
	})
	if err := receipts.Finalize(120); err != nil {
		t.Fatal(err)
	}
	vectors, err := simulation.NewDecisionFrontierVectorRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := vectors.RequireScalarDecisionLink(11, link); err != nil {
		t.Fatal(err)
	}
	vectors.Record(simulation.DecisionFrontierVector{
		ActorID: 99, ClientID: 11, TradingLinkID: link, Symbol: "ABC/USD", RequestID: 31,
		Side: exchange.Buy, OrderType: exchange.LimitOrder, TimeInForce: exchange.GTC, Price: 99, Qty: 3, DecisionAt: 120,
		Components: []simulation.DecisionFrontierComponent{{ClientID: 11, Frontier: frontier}},
	})
	if err := vectors.Finalize(filepath.Join(dir, "market-data-evidence-v2.json")); err != nil {
		t.Fatal(err)
	}
	return dir
}

func updateFixtureManifestDigest(t *testing.T, path, field, digest string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	section, ok := manifest[field].(map[string]any)
	if !ok {
		t.Fatalf("manifest section %q missing", field)
	}
	section["digest"] = digest
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		t.Fatal(err)
	}
}
