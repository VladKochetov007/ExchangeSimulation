package multivenue

import (
	"bytes"
	"encoding/json"
	"testing"

	"exchange_sim/evstream"
)

func TestCDFBinarySchemasRoundTripThroughRenderer(t *testing.T) {
	var output bytes.Buffer
	sink := &binaryEvidence{writer: evstream.NewWriter(&output, evstream.WriterOptions{SchemaEpoch: binaryEvidenceSchemaEpoch})}
	decision := ElasticLiquiditySupplierDecision{
		Role: "cdf_elastic_supplier_1", ClientID: 7, Symbol: "CDF/USD", DecisionTime: 100,
		DecisionPhaseOffset: 2, ObservationTime: 90, ObservationAge: 10, ObservationSequence: 12,
		ObservationLinkID: 3, ObservationOrdinal: 8, ObservationDeliveredAt: 95,
		ObservationFingerprint: "0102030405060708090a0b0c0d0e0f10", ObservationDigest: "1112131415161718191a1b1c1d1e1f20",
		BestBid: 99, BestBidQty: 10, BestAsk: 101, BestAskQty: 11, MarkPrice: 100, RiskMarkPrice: 100,
		LocalBookMode: "two_sided", QuotePriceSource: "midpoint", RiskMarkSource: "midpoint",
		ReferencePrice: 100, Position: 2, TargetPosition: 3, InventoryLimit: 10,
		InitialBaseBalance: 20, GrossInventory: 22, GrossInventoryLimit: 30,
		Action: "submit", Reason: "inventory_target", Side: "BUY", QuotePrice: 98, QuoteQty: 4,
		MinimumQualifyingQty: 2, RegisteredMinimumExecutableQty: 1, QuoteOrderID: 20,
		QuoteRequestID: 21, CancelRequestID: 22, QuoteSubmittedAt: 101, QuoteCashAvailable: 500,
		QuoteCashReserved: 100, QuoteCashRequired: 104, InitialEquityQuote: 2_000,
		EquityQuote: 1_990, PeakEquityQuote: 2_010, LossFromInitialQuote: 10, DrawdownQuote: 20,
		MaxLossQuote: 100, EquityAvailable: true, RiskLimitTriggered: false,
	}
	fill := ElasticLiquiditySupplierFill{
		Role: "cdf_elastic_supplier_1", ClientID: 7, Symbol: "CDF/USD", OrderID: 20, TradeID: 55,
		Timestamp: 110, Side: "BUY", Price: 98, Qty: 4, FeeAmount: 1, FeeAsset: "USD",
		IsFull: true, PositionBefore: 2, PositionAfter: 6,
	}
	if err := sink.record(decision.DecisionTime, decision.ClientID, "elastic_liquidity_supplier_decision", "north", decision, "general.jsonl", 1); err != nil {
		t.Fatal(err)
	}
	if err := sink.record(fill.Timestamp, fill.ClientID, "elastic_liquidity_supplier_fill", "north", fill, "general.jsonl", 2); err != nil {
		t.Fatal(err)
	}
	if err := sink.finish(); err != nil {
		t.Fatal(err)
	}

	reader, err := evstream.NewReader(bytes.NewReader(output.Bytes()), evstream.ReaderOptions{VerifyHash: true})
	if err != nil {
		t.Fatal(err)
	}
	var rendered []renderPersistedEvent
	if err := reader.Range(func(frame evstream.Frame) error {
		_, record, err := renderBinaryFrame(reader, frame)
		if err != nil {
			return err
		}
		var event renderPersistedEvent
		if err := json.Unmarshal(record.raw, &event); err != nil {
			return err
		}
		rendered = append(rendered, event)
		return nil
	}); err != nil {
		t.Fatalf("render binary CDF schemas: %v", err)
	}
	if len(rendered) != 2 || !reader.Terminated() || sink.unencodableCount() != 0 {
		t.Fatalf("rendered CDF stream = %d events, terminated=%t, unencodable=%d", len(rendered), reader.Terminated(), sink.unencodableCount())
	}
	expected := []struct {
		name    string
		payload any
	}{
		{"elastic_liquidity_supplier_decision", decision},
		{"elastic_liquidity_supplier_fill", fill},
	}
	for index, want := range expected {
		if rendered[index].Event != want.name || rendered[index].ClientID != 7 || rendered[index].Data.VenueID != "north" || rendered[index].Data.Sequence != uint64(index+1) {
			t.Fatalf("rendered CDF envelope %d = %+v", index, rendered[index])
		}
		wantPayload, err := json.Marshal(want.payload)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(rendered[index].Data.Payload, wantPayload) {
			t.Fatalf("rendered CDF payload %d = %s, want %s", index, rendered[index].Data.Payload, wantPayload)
		}
	}
}
