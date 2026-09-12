package multivenue

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
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
		QuoteRequestID: 21, CancelRequestID: 22, ReplacesOrderID: 19, QuoteSubmittedAt: 101, QuoteCashAvailable: 500,
		QuoteCashReserved: 100, QuoteCashRequired: 104, InitialEquityQuote: 2_000,
		EquityQuote: 1_990, PeakEquityQuote: 2_010, LossFromInitialQuote: 10, DrawdownQuote: 20,
		MaxLossQuote: 100, EquityAvailable: true, RiskLimitTriggered: false, RiskMarkCurrent: true,
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

func TestCDFDecisionAbsentSideUsesReservedZeroReference(t *testing.T) {
	var output bytes.Buffer
	sink := &binaryEvidence{writer: evstream.NewWriter(&output, evstream.WriterOptions{SchemaEpoch: binaryEvidenceSchemaEpoch})}
	decision := ElasticLiquiditySupplierDecision{
		Role: "cdf_elastic_supplier_1", ClientID: 7, Symbol: "CDF/USD",
		ObservationFingerprint: "fingerprint", ObservationDigest: "digest",
		LocalBookMode: "one_sided", QuotePriceSource: "none", RiskMarkSource: "none",
		Action: "withdraw", Reason: "risk_limit",
	}
	if err := sink.record(100, decision.ClientID, "elastic_liquidity_supplier_decision", "north", decision, "general.jsonl", 1); err != nil {
		t.Fatal(err)
	}
	if err := sink.finish(); err != nil {
		t.Fatal(err)
	}

	reader, err := evstream.NewReader(bytes.NewReader(output.Bytes()), evstream.ReaderOptions{VerifyHash: true})
	if err != nil {
		t.Fatal(err)
	}
	var rendered ElasticLiquiditySupplierDecision
	if err := reader.Range(func(frame evstream.Frame) error {
		if frame.Header.SchemaID != SchemaElasticLiquiditySupplierDecision {
			return nil
		}
		payload, handled, err := renderCDFPayloadJSONVersioned(frame.Header.SchemaID, frame.Header.SchemaVersion, frame.Payload[16+sha256.Size:], reader)
		if err != nil || !handled {
			return fmt.Errorf("render absent-side decision: handled=%t: %w", handled, err)
		}
		if err := json.Unmarshal(payload, &rendered); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if rendered.Side != "" {
		t.Fatalf("absent side rendered as %q", rendered.Side)
	}
	if _, ok := reader.Lookup(0); ok {
		t.Fatal("reserved dictionary id zero resolved as a value")
	}
	for ref := uint32(1); ref < 32; ref++ {
		if value, ok := reader.Lookup(ref); ok && value == "" {
			t.Fatalf("dictionary reference %d contains an empty value", ref)
		}
	}
}

type permissiveCDFResolver struct {
	delegate evstream.Resolver
}

func (r permissiveCDFResolver) Lookup(ref uint32) (string, bool) {
	if ref == 0 {
		return "", true
	}
	return r.delegate.Lookup(ref)
}

func TestCDFDecisionRequiredReferenceRejectsZero(t *testing.T) {
	var output bytes.Buffer
	sink := &binaryEvidence{writer: evstream.NewWriter(&output, evstream.WriterOptions{SchemaEpoch: binaryEvidenceSchemaEpoch})}
	decision := ElasticLiquiditySupplierDecision{
		Role: "cdf_elastic_supplier_1", ClientID: 7, Symbol: "CDF/USD",
		Action: "withdraw", Reason: "risk_limit",
	}
	if err := sink.record(100, decision.ClientID, "elastic_liquidity_supplier_decision", "north", decision, "general.jsonl", 1); err != nil {
		t.Fatal(err)
	}
	if err := sink.finish(); err != nil {
		t.Fatal(err)
	}
	reader, err := evstream.NewReader(bytes.NewReader(output.Bytes()), evstream.ReaderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var payload []byte
	if err := reader.Range(func(frame evstream.Frame) error {
		if frame.Header.SchemaID == SchemaElasticLiquiditySupplierDecision {
			payload = append(payload, frame.Payload[16+sha256.Size:]...)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(payload) < evstream.PresenceBits(cdfDecisionOptionalFields)+4 {
		t.Fatalf("decision payload length = %d, too short for role reference", len(payload))
	}
	binary.LittleEndian.PutUint32(payload[evstream.PresenceBits(cdfDecisionOptionalFields):], 0)
	var decoded ElasticLiquiditySupplierDecision
	if err := decodeElasticLiquiditySupplierDecisionVersioned(payload, permissiveCDFResolver{delegate: reader}, &decoded, 4); !errors.Is(err, evstream.ErrCorrupt) {
		t.Fatalf("zero required CDF reference error = %v, want ErrCorrupt", err)
	}
}
