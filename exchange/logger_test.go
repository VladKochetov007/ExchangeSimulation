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
