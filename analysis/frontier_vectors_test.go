package analysis

import (
	"path/filepath"
	"reflect"
	"testing"

	"exchange_sim/exchange"
	"exchange_sim/simulation"
)

func TestDecisionFrontierVectorsStreamingMatchesBufferedOracle(t *testing.T) {
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
