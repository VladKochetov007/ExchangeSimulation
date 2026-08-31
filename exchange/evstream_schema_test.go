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
		{Timestamp: 7, Bucket: VenueInsuranceFund, Asset: "USD", Symbol: "ABC-PERP",
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

// TestOrderLifecycleRoundTrip covers the accepted/cancelled/expired/forced
// family, the largest remaining opaque group. The accepted case matters most:
// it embeds *Order, whose six enums are all uint8-backed, so a wider encoding
// would waste bytes and a narrower one would truncate silently.
func TestOrderLifecycleRoundTrip(t *testing.T) {
	order := &Order{
		ID: 7, ClientID: 9, Side: Sell, PositionSide: PositionShort,
		Type: LimitOrder, TimeInForce: GTC, PostOnly: true,
		Price: math.MaxInt64, Qty: math.MinInt64, FilledQty: 3,
		Visibility: Hidden, IcebergQty: 5, Status: PartialFill,
		Timestamp: math.MinInt64,
	}
	accepted := acceptedOrderEvidence{Order: order, RequestID: math.MaxUint64}
	frame, _ := roundTripFrame(t, accepted)
	var decodedOrder Order
	var decodedAccepted acceptedOrderEvidence
	if err := DecodeAcceptedOrder(frame.Payload, &decodedOrder, &decodedAccepted); err != nil {
		t.Fatalf("decode accepted: %v", err)
	}
	requireJSONPreserved(t, accepted, decodedAccepted)

	cancelled := cancelledOrderEvidence{OrderID: 1, RemainingQty: math.MinInt64, RequestID: 2}
	frame, _ = roundTripFrame(t, cancelled)
	var decodedCancelled cancelledOrderEvidence
	if err := DecodeCancelledOrder(frame.Payload, &decodedCancelled); err != nil {
		t.Fatalf("decode cancelled: %v", err)
	}
	requireJSONPreserved(t, cancelled, decodedCancelled)

	expired := expiredOrderEvidence{OrderID: 3, Reason: "ioc_unfilled",
		RemainingQty: math.MaxInt64, RequestID: 4}
	frame, reader := roundTripFrame(t, expired)
	var decodedExpired expiredOrderEvidence
	if err := DecodeExpiredOrder(frame.Payload, reader, &decodedExpired); err != nil {
		t.Fatalf("decode expired: %v", err)
	}
	requireJSONPreserved(t, expired, decodedExpired)

	forced := forcedCancelEvidence{OrderID: 5, Reason: "liquidation", RemainingQty: -1}
	frame, reader = roundTripFrame(t, forced)
	var decodedForced forcedCancelEvidence
	if err := DecodeForcedCancel(frame.Payload, reader, &decodedForced); err != nil {
		t.Fatalf("decode forced: %v", err)
	}
	requireJSONPreserved(t, forced, decodedForced)
}

// TestOrderEnumsSurviveTheirFullRange pins the one-byte enum encoding against
// the widest value each type can hold. A silent truncation here would produce a
// plausible order with the wrong side or status.
func TestOrderEnumsSurviveTheirFullRange(t *testing.T) {
	order := &Order{
		Side: Side(255), PositionSide: PositionSide(255), Type: OrderType(255),
		TimeInForce: TimeInForce(255), Visibility: Visibility(255), Status: OrderStatus(255),
	}
	accepted := acceptedOrderEvidence{Order: order}
	frame, _ := roundTripFrame(t, accepted)
	var decodedOrder Order
	var decoded acceptedOrderEvidence
	if err := DecodeAcceptedOrder(frame.Payload, &decodedOrder, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for name, pair := range map[string][2]uint8{
		"side":          {uint8(order.Side), uint8(decodedOrder.Side)},
		"position_side": {uint8(order.PositionSide), uint8(decodedOrder.PositionSide)},
		"type":          {uint8(order.Type), uint8(decodedOrder.Type)},
		"time_in_force": {uint8(order.TimeInForce), uint8(decodedOrder.TimeInForce)},
		"visibility":    {uint8(order.Visibility), uint8(decodedOrder.Visibility)},
		"status":        {uint8(order.Status), uint8(decodedOrder.Status)},
	} {
		if pair[0] != pair[1] {
			t.Fatalf("%s truncated: wrote %d, read %d", name, pair[0], pair[1])
		}
	}
}

// TestCancelStructsMatchTheMapsTheyReplaced is the oracle test: the structs
// must encode byte-identically to the map literals they replaced, which holds
// only while their fields stay in lexicographic order of their JSON names.
func TestCancelStructsMatchTheMapsTheyReplaced(t *testing.T) {
	cancelled := cancelledOrderEvidence{OrderID: 11, RemainingQty: 22, RequestID: 33}
	wantCancelled, _ := json.Marshal(map[string]any{
		"order_id": uint64(11), "request_id": uint64(33), "remaining_qty": int64(22),
	})
	gotCancelled, _ := json.Marshal(cancelled)
	if !bytes.Equal(gotCancelled, wantCancelled) {
		t.Fatalf("cancelled struct %s, map form %s", gotCancelled, wantCancelled)
	}

	expired := expiredOrderEvidence{OrderID: 11, Reason: "ioc", RemainingQty: 22, RequestID: 33}
	wantExpired, _ := json.Marshal(map[string]any{
		"order_id": uint64(11), "request_id": uint64(33),
		"remaining_qty": int64(22), "reason": "ioc",
	})
	gotExpired, _ := json.Marshal(expired)
	if !bytes.Equal(gotExpired, wantExpired) {
		t.Fatalf("expired struct %s, map form %s", gotExpired, wantExpired)
	}
}

// TestRenderPayloadReproducesCanonicalJSON is the analyzer-equivalence proof in
// miniature.
//
// If rendering a binary payload reproduces byte-for-byte the JSON the old
// pipeline persisted, then any analyzer reading that JSON produces identical
// output — without running a single analyzer. Running them would test the
// analyzers; this tests the only thing that could differ.
func TestRenderPayloadReproducesCanonicalJSON(t *testing.T) {
	order := &Order{ID: 7, ClientID: 9, Side: Sell, PositionSide: PositionShort,
		Type: LimitOrder, TimeInForce: GTC, PostOnly: true, Price: 100, Qty: 5,
		FilledQty: 2, Visibility: Iceberg, IcebergQty: 1, Status: PartialFill,
		Timestamp: 1234}

	cases := []evstream.InterningAppender{
		fillEvidence{FeeAmount: 1, FeeAsset: "USD", FilledQty: 2, IsFull: true,
			OrderID: 3, PositionSide: "BOTH", Price: 4, Qty: 5, RealizedPnL: -6,
			RemainingQty: 7, Role: "maker", Side: "SELL", Symbol: "ABC-PERP", TradeID: 8},
		bookDeltaEvidence{HiddenQty: 1, Price: 2, Side: "BUY", TotalQty: 3, VisibleQty: 4},
		bookSnapshotEvidence{Asks: []PriceLevel{{Price: 1, VisibleQty: 2}}, Bids: nil},
		bookSnapshotEvidence{Asks: nil, Bids: []PriceLevel{}},
		VenueBalanceEvent{Timestamp: 1, Bucket: VenueFeeRevenue, Asset: "USD",
			Reason: "taker_fee", OldBalance: 2, NewBalance: 3, Delta: 1},
		VenueBalanceEvent{Timestamp: 1, Bucket: VenueInsuranceFund, Asset: "USD",
			Symbol: "ABC-PERP", Reason: "clearance"},
		acceptedOrderEvidence{Order: order, RequestID: 11},
		cancelledOrderEvidence{OrderID: 1, RemainingQty: 2, RequestID: 3},
		expiredOrderEvidence{OrderID: 1, Reason: "IOC_EXPIRED", RemainingQty: 2, RequestID: 3},
		forcedCancelEvidence{OrderID: 1, Reason: "liquidation", RemainingQty: 2},
		etypes.BalanceChangeEvent{Timestamp: 1, ClientID: 2, Symbol: "ABC/USD",
			Reason: "fill", Changes: []etypes.BalanceDelta{{Asset: "USD", Wallet: "perp", Delta: 3}}},
		etypes.BalanceChangeEvent{Timestamp: 1, ClientID: 2, Symbol: "ABC/USD",
			PositionSide: "BOTH", Reason: "funding", Changes: nil},
		etypes.FeeRevenueEvent{Timestamp: 1, Symbol: "ABC/USD", TradeID: 2,
			TakerFee: 3, MakerFee: 4, Asset: "USD"},
		&etypes.Trade{TradeID: 1, Price: 2, Qty: 3, Side: Buy, TakerOrderID: 4, MakerOrderID: 5},
		instrumentLogEvent{Symbol: "ABC/USD",
			Payload: bookDeltaEvidence{HiddenQty: 9, Price: 8, Side: "SELL", TotalQty: 7, VisibleQty: 6}},
		instrumentLogEvent{Symbol: "ABC/USD", Payload: map[string]any{"a": 1, "b": "two"}},
	}

	for i, original := range cases {
		frame, reader := roundTripFrame(t, original)
		rendered, err := RenderPayloadJSON(frame.Header.SchemaID, frame.Payload, reader)
		if err != nil {
			t.Fatalf("case %d: render: %v", i, err)
		}
		want, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("case %d: marshal original: %v", i, err)
		}
		if !bytes.Equal(rendered, want) {
			t.Fatalf("case %d: rendered JSON differs from the original\n  rendered: %s\n  original: %s",
				i, rendered, want)
		}
	}
}

// TestRenderRejectsUnknownSchema pins the failure mode: an unrenderable frame
// must be an error, not silently empty output that a diff would call equal.
func TestRenderRejectsUnknownSchema(t *testing.T) {
	if _, err := RenderPayloadJSON(60000, []byte{0, 0, 0, 0}, nil); err == nil {
		t.Fatal("unknown schema rendered without error")
	}
}

// A pointer-receiver appender made the encoding depend on how the caller boxed
// the value: a Trade fell through to opaque JSON while a *Trade took the typed
// path, so one logical event had two encodings and two digests. For a format
// whose identity is its canonical bytes, a Go calling convention must not
// choose them.
func TestEncodingDoesNotDependOnBoxing(t *testing.T) {
	trade := etypes.Trade{
		TradeID: 77, Price: 4975000000, Qty: 125000000,
		Side: etypes.Buy, TakerOrderID: 3, MakerOrderID: 4,
	}
	byValue, _ := roundTripFrame(t, instrumentLogEvent{Symbol: "ABC/USD", Payload: trade})
	byPointer, _ := roundTripFrame(t, instrumentLogEvent{Symbol: "ABC/USD", Payload: &trade})
	if !bytes.Equal(byValue.Payload, byPointer.Payload) {
		t.Fatalf("boxing changed the canonical bytes\n value   %d bytes: %x\n pointer %d bytes: %x",
			len(byValue.Payload), byValue.Payload, len(byPointer.Payload), byPointer.Payload)
	}
}
