package exchange

import (
	"bytes"
	"encoding/json"
	"math"
	"math/rand"
	"testing"

	"exchange_sim/evstream"
	etypes "exchange_sim/types"
)

// These payloads feed the ordered execution-stream digest, so the property that
// matters is not that they decode but that nothing is lost on the way.
//
// Each case goes the full route the migration takes — the original value, a
// binary frame, a decoded value, and canonical JSON — and requires the JSON at
// the end to be byte-identical to the JSON at the start. That catches a dropped
// field, a reordered one, a widened integer, and the nil-versus-empty
// distinction that a length prefix alone cannot preserve.

// roundTripFrame writes one payload to a real stream and hands back the decoded
// frame, so framing, interning and the dictionary are exercised rather than
// bypassed.
func roundTripFrame(t *testing.T, payload evstream.InterningAppender) (evstream.Frame, *evstream.Reader) {
	t.Helper()
	var buf bytes.Buffer
	writer := evstream.NewWriter(&buf, evstream.WriterOptions{BlockBytes: 512})
	if err := writer.AppendInterning(1, 2, 0, payload); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	reader, err := evstream.NewReader(bytes.NewReader(buf.Bytes()), evstream.ReaderOptions{VerifyHash: true})
	if err != nil {
		t.Fatalf("reader: %v", err)
	}
	var got evstream.Frame
	seen := 0
	if err := reader.Range(func(frame evstream.Frame) error {
		got = frame
		got.Payload = append([]byte(nil), frame.Payload...)
		seen++
		return nil
	}); err != nil {
		t.Fatalf("range: %v", err)
	}
	if seen != 1 {
		t.Fatalf("expected one event frame, saw %d", seen)
	}
	if reader.ExecutionHash() != writer.ExecutionHash() {
		t.Fatal("reader digest diverged from writer digest")
	}
	return got, reader
}

// requireJSONPreserved compares the canonical JSON before and after the trip.
func requireJSONPreserved(t *testing.T, original, decoded any) {
	t.Helper()
	before, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal original: %v", err)
	}
	after, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("marshal decoded: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("round trip lost information\n  before: %s\n  after : %s", before, after)
	}
}

func TestFillEvidenceRoundTrip(t *testing.T) {
	cases := []fillEvidence{
		{},
		{FeeAmount: 1, FeeAsset: "USD", FilledQty: 2, IsFull: true, NewEntryPrice: 3,
			NewSize: 4, OrderID: 5, PositionSide: "BOTH", Price: 6, Qty: 7,
			RealizedPnL: -8, RemainingQty: 9, Role: "taker", Side: "BUY",
			Symbol: "ABC-PERP", TradeID: 10},
		{FeeAmount: math.MinInt64, FilledQty: math.MaxInt64, NewEntryPrice: math.MinInt64,
			NewSize: math.MaxInt64, OrderID: math.MaxUint64, Price: math.MaxInt64,
			Qty: math.MinInt64, RealizedPnL: math.MaxInt64, RemainingQty: math.MinInt64,
			TradeID: math.MaxUint64, IsFull: false},
	}
	for _, original := range cases {
		frame, reader := roundTripFrame(t, original)
		var decoded fillEvidence
		if err := DecodeFillEvidence(frame.Payload, reader, &decoded); err != nil {
			t.Fatalf("decode: %v", err)
		}
		requireJSONPreserved(t, original, decoded)
	}
}

func TestBookDeltaEvidenceRoundTrip(t *testing.T) {
	for _, original := range []bookDeltaEvidence{
		{},
		{HiddenQty: 1, Price: 2, Side: "BUY", TotalQty: 3, VisibleQty: 4},
		{HiddenQty: math.MinInt64, Price: math.MaxInt64, Side: "SELL",
			TotalQty: math.MinInt64, VisibleQty: math.MaxInt64},
	} {
		frame, reader := roundTripFrame(t, original)
		var decoded bookDeltaEvidence
		if err := DecodeBookDelta(frame.Payload, reader, &decoded); err != nil {
			t.Fatalf("decode: %v", err)
		}
		requireJSONPreserved(t, original, decoded)
	}
}

// TestBookSnapshotEvidenceRoundTrip covers the nil-versus-empty distinction on
// both sides independently, which is where a length-prefixed encoding without
// presence bits silently loses information.
func TestBookSnapshotEvidenceRoundTrip(t *testing.T) {
	for _, original := range []bookSnapshotEvidence{
		{},
		{Asks: []PriceLevel{}, Bids: nil},
		{Asks: nil, Bids: []PriceLevel{}},
		{Asks: []PriceLevel{}, Bids: []PriceLevel{}},
		{
			Asks: []PriceLevel{{Price: 1, VisibleQty: 2, HiddenQty: 3}},
			Bids: []PriceLevel{{Price: math.MinInt64}, {Price: math.MaxInt64, VisibleQty: -1}},
		},
	} {
		frame, _ := roundTripFrame(t, original)
		var decoded bookSnapshotEvidence
		if err := DecodeBookSnapshot(frame.Payload, &decoded); err != nil {
			t.Fatalf("decode: %v", err)
		}
		requireJSONPreserved(t, original, decoded)
	}
}

// TestVenueBalanceRoundTrip covers omitempty on Symbol, where emitting the
// field anyway would still be valid JSON and still break the digest.
func TestVenueBalanceRoundTrip(t *testing.T) {
	for _, original := range []VenueBalanceEvent{
		{Bucket: VenueFeeRevenue, Asset: "USD", Reason: "taker_fee"},
		{Timestamp: 7, Sequence: 11, TradeID: 12, Bucket: VenueInsuranceFund, Asset: "USD", Symbol: "ABC-PERP",
			Reason: "clearance", OldBalance: math.MinInt64, NewBalance: math.MaxInt64, Delta: -1},
	} {
		frame, reader := roundTripFrame(t, original)
		var decoded VenueBalanceEvent
		if err := DecodeVenueBalance(frame.Payload, reader, &decoded); err != nil {
			t.Fatalf("decode: %v", err)
		}
		requireJSONPreserved(t, original, decoded)
	}
}

func TestBalanceChangeRoundTrip(t *testing.T) {
	for _, original := range []etypes.BalanceChangeEvent{
		{},
		{Timestamp: 1, ClientID: 2, Symbol: "ABC/USD", Reason: "fill", Changes: nil},
		{Timestamp: 1, ClientID: 2, Symbol: "ABC/USD", Reason: "fill",
			Changes: []etypes.BalanceDelta{}},
		{Timestamp: -1, ClientID: math.MaxUint64, Symbol: "ABC-PERP", PositionSide: "BOTH",
			Reason: "funding", Changes: []etypes.BalanceDelta{
				{Asset: "USD", Wallet: "perp", OldBalance: math.MinInt64,
					NewBalance: math.MaxInt64, Delta: -1},
				{Asset: "CDF", Wallet: "spot"},
			}},
	} {
		frame, reader := roundTripFrame(t, original)
		var decoded etypes.BalanceChangeEvent
		if err := etypes.DecodeBalanceChange(frame.Payload, reader, &decoded); err != nil {
			t.Fatalf("decode: %v", err)
		}
		requireJSONPreserved(t, original, decoded)
	}
}

func TestFeeRevenueAndTradeRoundTrip(t *testing.T) {
	fee := etypes.FeeRevenueEvent{Timestamp: 3, Symbol: "ABC/USD", TradeID: 9,
		TakerFee: math.MaxInt64, MakerFee: math.MinInt64, Asset: "USD"}
	frame, reader := roundTripFrame(t, fee)
	var decodedFee etypes.FeeRevenueEvent
	if err := etypes.DecodeFeeRevenue(frame.Payload, reader, &decodedFee); err != nil {
		t.Fatalf("decode fee: %v", err)
	}
	requireJSONPreserved(t, fee, decodedFee)

	trade := &etypes.Trade{TradeID: 1, Price: math.MaxInt64, Qty: math.MinInt64,
		Side: Sell, TakerOrderID: 2, MakerOrderID: 3}
	frame, _ = roundTripFrame(t, trade)
	var decodedTrade etypes.Trade
	if err := etypes.DecodeTrade(frame.Payload, &decodedTrade); err != nil {
		t.Fatalf("decode trade: %v", err)
	}
	requireJSONPreserved(t, trade, &decodedTrade)
}

// TestInstrumentLogWrapperCarriesInnerSchema pins the wrapper contract: the
// inner schema id travels in the payload, and a family with no typed schema
// rides as opaque JSON rather than blocking the wrapper.
func TestInstrumentLogWrapperCarriesInnerSchema(t *testing.T) {
	typed := instrumentLogEvent{Symbol: "ABC/USD",
		Payload: bookDeltaEvidence{HiddenQty: 1, Price: 2, Side: "BUY", TotalQty: 3, VisibleQty: 4}}
	frame, reader := roundTripFrame(t, typed)
	cursor := evstream.NewCursor(frame.Payload)
	symbolRef := cursor.Uint32()
	innerID, innerVersion := cursor.Uint16(), cursor.Uint16()
	if err := cursor.Err(); err != nil {
		t.Fatalf("wrapper header: %v", err)
	}
	if symbol, ok := reader.Lookup(symbolRef); !ok || symbol != "ABC/USD" {
		t.Fatalf("wrapper symbol resolved to %q (ok=%v)", symbol, ok)
	}
	if innerID != SchemaBookDelta || innerVersion != 1 {
		t.Fatalf("inner schema = %d v%d, want %d v1", innerID, innerVersion, SchemaBookDelta)
	}

	// A payload with no typed schema must still travel, as opaque JSON.
	opaque := instrumentLogEvent{Symbol: "ABC/USD", Payload: map[string]any{"b": 2, "a": 1}}
	frame, reader = roundTripFrame(t, opaque)
	cursor = evstream.NewCursor(frame.Payload)
	cursor.Uint32()
	innerID, _ = cursor.Uint16(), cursor.Uint16()
	if innerID != evstream.SchemaOpaqueJSON {
		t.Fatalf("untyped inner payload got schema %d, want opaque %d",
			innerID, evstream.SchemaOpaqueJSON)
	}
	body := cursor.Bytes()
	if err := cursor.Err(); err != nil {
		t.Fatalf("opaque body: %v", err)
	}
	want, _ := json.Marshal(opaque.Payload)
	if !bytes.Equal(body, want) {
		t.Fatalf("opaque body %s, want %s", body, want)
	}
}

// TestSchemaIDsAreDistinct guards the one mistake that silently corrupts a
// stream: two families claiming the same id decode as each other.
func TestSchemaIDsAreDistinct(t *testing.T) {
	ids := map[uint16]string{
		evstream.SchemaDictionary:  "dictionary",
		evstream.SchemaOpaqueJSON:  "opaque",
		etypes.SchemaBalanceChange: "balance_change",
		etypes.SchemaFeeRevenue:    "fee_revenue",
		etypes.SchemaTrade:         "trade",
		SchemaFillEvidence:         "fill",
		SchemaBookDelta:            "book_delta",
		SchemaBookSnapshot:         "book_snapshot",
		SchemaVenueBalance:         "venue_balance",
		SchemaInstrumentLog:        "instrument_log",
	}
	if len(ids) != 10 {
		t.Fatalf("two schemas share an id; only %d distinct ids among 10 families", len(ids))
	}
}

func TestSchemaRoundTripRandomised(t *testing.T) {
	random := rand.New(rand.NewSource(20260905))
	assets := []string{"USD", "ABC", "CDF", ""}
	ints := []int64{0, 1, -1, 987654321, math.MaxInt64, math.MinInt64}
	pick := func() int64 { return ints[random.Intn(len(ints))] }

	for range 4000 {
		original := fillEvidence{
			FeeAmount: pick(), FeeAsset: assets[random.Intn(len(assets))],
			FilledQty: pick(), IsFull: random.Intn(2) == 0,
			NewEntryPrice: pick(), NewSize: pick(), OrderID: uint64(random.Int63()),
			PositionSide: assets[random.Intn(len(assets))], Price: pick(), Qty: pick(),
			RealizedPnL: pick(), RemainingQty: pick(),
			Role: assets[random.Intn(len(assets))], Side: assets[random.Intn(len(assets))],
			Symbol: assets[random.Intn(len(assets))], TradeID: uint64(random.Int63()),
		}
		frame, reader := roundTripFrame(t, original)
		var decoded fillEvidence
		if err := DecodeFillEvidence(frame.Payload, reader, &decoded); err != nil {
			t.Fatalf("decode: %v", err)
		}
		requireJSONPreserved(t, original, decoded)
	}
}

func TestRenderPayloadJSONPreservesTypedAndOpaquePayloads(t *testing.T) {
	cases := []evstream.InterningAppender{
		fillEvidence{FeeAmount: 1, FeeAsset: "USD", FilledQty: 2, IsFull: true,
			OrderID: 3, PositionSide: "BOTH", Price: 4, Qty: 5, RealizedPnL: -6,
			RemainingQty: 7, Role: "maker", Side: "SELL", Symbol: "ABC-PERP", TradeID: 8},
		bookDeltaEvidence{HiddenQty: 1, Price: 2, Side: "BUY", TotalQty: 3, VisibleQty: 4},
		bookSnapshotEvidence{Asks: []PriceLevel{{Price: 1, VisibleQty: 2}}, Bids: nil},
		VenueBalanceEvent{Timestamp: 1, Sequence: 2, TradeID: 3, Bucket: VenueFeeRevenue,
			Asset: "USD", Reason: "taker_fee"},
		etypes.BalanceChangeEvent{Timestamp: 1, ClientID: 2, Symbol: "ABC/USD", Reason: "fill",
			Changes: []etypes.BalanceDelta{{Asset: "USD", Wallet: "spot", Delta: 3}}},
		etypes.FeeRevenueEvent{Timestamp: 1, Symbol: "ABC/USD", TradeID: 2,
			TakerFee: 3, MakerFee: 4, Asset: "USD"},
		etypes.Trade{TradeID: 1, Price: 2, Qty: 3, Side: Buy, TakerOrderID: 4, MakerOrderID: 5},
		instrumentLogEvent{Symbol: "ABC/USD", Payload: bookDeltaEvidence{
			HiddenQty: 9, Price: 8, Side: "SELL", TotalQty: 7, VisibleQty: 6}},
		instrumentLogEvent{Symbol: "ABC/USD", Payload: map[string]any{"a": 1, "b": "two"}},
	}
	for index, original := range cases {
		frame, reader := roundTripFrame(t, original)
		rendered, err := RenderPayloadJSONVersioned(frame.Header.SchemaID, frame.Header.SchemaVersion, frame.Payload, reader)
		if err != nil {
			t.Fatalf("case %d render: %v", index, err)
		}
		want, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("case %d marshal: %v", index, err)
		}
		if !bytes.Equal(rendered, want) {
			t.Fatalf("case %d rendered %s, want %s", index, rendered, want)
		}
	}
}

func TestRenderPayloadJSONRejectsUnknownSchemaVersion(t *testing.T) {
	if _, err := RenderPayloadJSONVersioned(SchemaBookDelta, 99, nil, nil); err == nil {
		t.Fatal("unknown schema version was rendered")
	}
}

func TestHighVolumeTypedPayloadsPreserveLegacyJSONShape(t *testing.T) {
	typedCases := []struct {
		name  string
		typed any
		old   any
	}{
		{
			name: "book snapshot",
			typed: bookSnapshotEvidence{
				Asks: []PriceLevel{{Price: 101, VisibleQty: 7, HiddenQty: 2}},
				Bids: []PriceLevel{},
			},
			old: map[string]any{
				"bids": []PriceLevel{},
				"asks": []PriceLevel{{Price: 101, VisibleQty: 7, HiddenQty: 2}},
			},
		},
		{
			name:  "book delta",
			typed: bookDeltaEvidence{Side: "BUY", Price: 101, VisibleQty: 7, HiddenQty: 2, TotalQty: 9},
			old: map[string]any{
				"side": "BUY", "price": int64(101), "visible_qty": int64(7),
				"hidden_qty": int64(2), "total_qty": int64(9),
			},
		},
		{
			name: "order fill",
			typed: fillEvidence{
				OrderID: 1, Symbol: "ABC/USD", Qty: 2, Price: 101, Side: "BUY",
				PositionSide: "BOTH", FilledQty: 2, RemainingQty: 0, IsFull: true,
				TradeID: 3, Role: "maker", FeeAmount: 4, FeeAsset: "USD",
				RealizedPnL: 5, NewSize: 6, NewEntryPrice: 101,
			},
			old: map[string]any{
				"order_id": uint64(1), "symbol": "ABC/USD", "qty": int64(2), "price": int64(101),
				"side": "BUY", "position_side": "BOTH", "filled_qty": int64(2),
				"remaining_qty": int64(0), "is_full": true, "trade_id": uint64(3),
				"role": "maker", "fee_amount": int64(4), "fee_asset": "USD",
				"realized_pnl": int64(5), "new_size": int64(6), "new_entry_price": int64(101),
			},
		},
	}
	for _, testCase := range typedCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := json.Marshal(testCase.typed)
			if err != nil {
				t.Fatal(err)
			}
			want, err := json.Marshal(testCase.old)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("typed payload changed JSON shape:\n got %s\nwant %s", got, want)
			}
		})
	}
}
