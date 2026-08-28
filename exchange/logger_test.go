package exchange

import (
	"encoding/json"
	"testing"
)

type recordingLogger struct {
	records []logRecord
}

type logRecord struct {
	event string
	data  any
}

func (l *recordingLogger) LogEvent(_ int64, _ uint64, event string, data any) {
	l.records = append(l.records, logRecord{event: event, data: data})
}

func TestInstrumentLoggerFallbackDoesNotReplaceGlobalLogger(t *testing.T) {
	ex := NewExchangeWithConfig(ExchangeConfig{})
	global := &recordingLogger{}
	fallback := &recordingLogger{}
	ex.SetLogger("_global", global)
	ex.SetInstrumentLoggerFallback(fallback)

	if got := ex.getLogger("_global"); got != global {
		t.Fatal("global logger was replaced by fallback")
	}
	dynamic := ex.getLogger("DYNAMIC-OPTION")
	if dynamic == nil {
		t.Fatal("dynamic instrument did not use fallback logger")
	}
	dynamic.LogEvent(1, 2, "dynamic_event", nil)
	if len(fallback.records) != 1 || fallback.records[0].event != "dynamic_event" {
		t.Fatalf("fallback did not receive tagged event: %+v", fallback.records)
	}
	tagged, ok := fallback.records[0].data.(instrumentLogEvent)
	if !ok || tagged.Symbol != "DYNAMIC-OPTION" {
		t.Fatalf("fallback record has no source symbol: %+v", fallback.records[0])
	}

	specific := &recordingLogger{}
	ex.SetLogger("DYNAMIC-OPTION", specific)
	if got := ex.getLogger("DYNAMIC-OPTION"); got != specific {
		t.Fatal("symbol-specific logger did not take precedence")
	}

}

func TestAcceptedOrderEvidenceRetainsRequestIDAndFlatOrderFields(t *testing.T) {
	ex := newPostOnlyTestExchange(t)
	log := &recordingLogger{}
	ex.SetLogger("ABC/USD", log)

	const requestID = 73
	response := ex.PlaceOrder(1, &OrderRequest{
		RequestID:   requestID,
		Symbol:      "ABC/USD",
		Side:        Buy,
		Type:        LimitOrder,
		Price:       99,
		Qty:         2,
		TimeInForce: GTC,
		Visibility:  Normal,
	})
	if !response.Success {
		t.Fatalf("resting order rejected: %+v", response)
	}
	var accepted any
	for _, record := range log.records {
		if record.event == "OrderAccepted" {
			accepted = record.data
			break
		}
	}
	if accepted == nil {
		t.Fatalf("accepted evidence = %#v", log.records)
	}
	evidence, ok := accepted.(acceptedOrderEvidence)
	if !ok {
		t.Fatalf("accepted evidence type = %T, want acceptedOrderEvidence", accepted)
	}
	if evidence.RequestID != requestID || evidence.Order == nil || evidence.Price != 99 || evidence.Qty != 2 || evidence.ClientID != 1 {
		t.Fatalf("accepted evidence = %#v", evidence)
	}

	raw, err := json.Marshal(evidence)
	if err != nil {
		t.Fatalf("marshal accepted evidence: %v", err)
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("decode accepted evidence: %v", err)
	}
	for _, field := range []string{"request_id", "order_id", "client_id", "price", "qty", "post_only"} {
		if _, ok := wire[field]; !ok {
			t.Fatalf("flat accepted evidence missing %q: %s", field, raw)
		}
	}
	if _, nested := wire["order"]; nested {
		t.Fatalf("accepted evidence unexpectedly nested order: %s", raw)
	}
}

func TestExchangeForcedCancellationIsLoggedWithoutActorRequest(t *testing.T) {
	ex := newPostOnlyTestExchange(t)
	log := &recordingLogger{}
	ex.SetLogger("ABC/USD", log)
	response := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 73, Symbol: "ABC/USD", Side: Buy, Type: LimitOrder,
		Price: 99, Qty: 2, TimeInForce: GTC, Visibility: Normal,
	})
	if !response.Success {
		t.Fatalf("resting order rejected: %+v", response)
	}
	orderID, ok := response.Data.(uint64)
	if !ok {
		t.Fatalf("accepted order ID = %#v", response.Data)
	}
	if !ex.cancelUnfundedSpotPlanMaker(ex.Books["ABC/USD"], orderID) {
		t.Fatalf("forced cancellation did not find order %d", orderID)
	}

	var cancellation map[string]any
	for _, record := range log.records {
		if record.event == "OrderCancelled" {
			cancellation, ok = record.data.(map[string]any)
			if ok {
				break
			}
		}
	}
	if cancellation == nil {
		t.Fatalf("forced cancellation evidence = %#v", log.records)
	}
	if cancellation["order_id"] != orderID || cancellation["remaining_qty"] != int64(2) ||
		cancellation["reason"] != exchangeForcedFeeReservationReason {
		t.Fatalf("forced cancellation evidence = %#v", cancellation)
	}
	if _, found := cancellation["request_id"]; found {
		t.Fatalf("forced cancellation fabricated actor request: %#v", cancellation)
	}
}

func TestRejectedOrderEvidenceRetainsAttemptedRequestFields(t *testing.T) {
	ex := newPostOnlyTestExchange(t)
	log := &recordingLogger{}
	ex.SetLogger("ABC/USD", log)
	if response := ex.PlaceOrder(2, &OrderRequest{
		RequestID: 1, Symbol: "ABC/USD", Side: Sell, Type: LimitOrder,
		Price: 100, Qty: 3, TimeInForce: GTC, Visibility: Normal,
	}); !response.Success {
		t.Fatalf("seed ask rejected: %+v", response)
	}
	const requestID = 74
	response := ex.PlaceOrder(1, &OrderRequest{
		RequestID: requestID, Symbol: "ABC/USD", Side: Buy, Type: LimitOrder,
		Price: 100, Qty: 2, TimeInForce: GTC, Visibility: Normal, PostOnly: true,
	})
	if response.Success || response.Error != RejectPostOnlyWouldTake {
		t.Fatalf("post-only rejection = %+v", response)
	}
	var rejected any
	for _, record := range log.records {
		if record.event == "OrderRejected" {
			rejected = record.data
			break
		}
	}
	if rejected == nil {
		t.Fatalf("rejected evidence = %#v", log.records)
	}
	evidence, ok := rejected.(rejectedOrderEvidence)
	if !ok {
		t.Fatalf("rejected evidence type = %T, want rejectedOrderEvidence", rejected)
	}
	if evidence.RequestID != requestID || evidence.Error != RejectPostOnlyWouldTake || evidence.Symbol != "ABC/USD" ||
		evidence.Side != Buy || evidence.Type != LimitOrder || evidence.TimeInForce != GTC || !evidence.PostOnly || evidence.Price != 100 || evidence.Qty != 2 {
		t.Fatalf("rejected evidence = %#v", evidence)
	}

	raw, err := json.Marshal(evidence)
	if err != nil {
		t.Fatalf("marshal rejected evidence: %v", err)
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("decode rejected evidence: %v", err)
	}
	for _, field := range []string{"request_id", "error", "symbol", "side", "type", "time_in_force", "post_only", "price", "qty"} {
		if _, ok := wire[field]; !ok {
			t.Fatalf("flat rejected evidence missing %q: %s", field, raw)
		}
	}
	if _, nested := wire["response"]; nested {
		t.Fatalf("rejected evidence unexpectedly nested response: %s", raw)
	}
}
