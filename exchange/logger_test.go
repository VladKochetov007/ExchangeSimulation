package exchange

import (
	"encoding/json"
	"testing"
)

type recordingLogger struct {
	records []logRecord
}

type typedRecordingLogger struct {
	recordingLogger
	typed []logRecord
}

type logRecord struct {
	event string
	data  any
}

func (l *recordingLogger) LogEvent(_ int64, _ uint64, event string, data any) {
	l.records = append(l.records, logRecord{event: event, data: data})
}

func (l *typedRecordingLogger) LogTypedEvent(_ int64, _ uint64, event string, typedData, _ any) {
	l.typed = append(l.typed, logRecord{event: event, data: typedData})
}

func TestLogFillUsesTypedEvidenceWhenLoggerSupportsIt(t *testing.T) {
	log := &typedRecordingLogger{}
	ctx := executionContext{
		book:      &OrderBook{Symbol: "ABC-PERP"},
		exec:      &Execution{Qty: 7, Price: 101},
		timestamp: 99, log: log, forced: true, liquidationID: 44,
	}
	side := fillSide{
		clientID: 12, orderID: 13, side: Buy, posSide: PositionBoth,
		fee: Fee{Amount: 3, Asset: "USD"}, filledQty: 7, totalQty: 7,
		delta:       PositionDelta{NewSize: -5, NewEntryPrice: 100},
		realizedPnL: -2, role: "taker",
	}

	logFill(ctx, 14, side)

	if len(log.records) != 0 {
		t.Fatalf("typed logger received compatibility event: %#v", log.records)
	}
	if len(log.typed) != 1 {
		t.Fatalf("typed events = %#v, want one", log.typed)
	}
	payload, ok := log.typed[0].data.(fillEvidence)
	if !ok {
		t.Fatalf("typed payload = %T, want fillEvidence", log.typed[0].data)
	}
	if payload.OrderID != 13 || payload.TradeID != 14 || payload.Symbol != "ABC-PERP" ||
		payload.Qty != 7 || payload.Price != 101 || payload.Role != "taker" ||
		!payload.Forced || payload.LiquidationID != 44 || payload.NewSize != -5 {
		t.Fatalf("typed fill payload = %#v", payload)
	}
}

func TestLogFillPreservesCompatibilityPayloadForLegacyLogger(t *testing.T) {
	log := &recordingLogger{}
	ctx := executionContext{
		book: &OrderBook{Symbol: "ABC/USD"}, exec: &Execution{Qty: 2, Price: 99},
		timestamp: 100, log: log,
	}
	logFill(ctx, 8, fillSide{
		clientID: 4, orderID: 5, side: Sell, posSide: PositionBoth,
		fee: Fee{Amount: 1, Asset: "USD"}, filledQty: 1, totalQty: 2,
		delta: PositionDelta{NewSize: 3, NewEntryPrice: 98}, realizedPnL: 6, role: "maker",
	})

	if len(log.records) != 1 {
		t.Fatalf("legacy records = %#v, want one", log.records)
	}
	payload, ok := log.records[0].data.(map[string]any)
	if !ok {
		t.Fatalf("legacy payload = %T, want map[string]any", log.records[0].data)
	}
	if payload["order_id"] != uint64(5) || payload["trade_id"] != uint64(8) ||
		payload["symbol"] != "ABC/USD" || payload["role"] != "maker" || payload["is_full"] != false {
		t.Fatalf("legacy fill payload = %#v", payload)
	}
	if _, forced := payload["forced"]; forced {
		t.Fatalf("ordinary legacy fill acquired forced marker: %#v", payload)
	}
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

func TestExchangeForcedCancellationCallSitesRetainTheirReason(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		check  func(*DefaultExchange, *OrderBook, *Client, Logger)
	}{
		{
			name:   "cancel all",
			reason: exchangeForcedLifecycleReason,
			check: func(ex *DefaultExchange, _ *OrderBook, _ *Client, _ Logger) {
				if got := ex.CancelAllClientOrders(1); got != 1 {
					t.Fatalf("cancel-all count = %d, want 1", got)
				}
			},
		},
		{
			name:   "lifecycle",
			reason: exchangeForcedLifecycleReason,
			check: func(ex *DefaultExchange, book *OrderBook, client *Client, _ Logger) {
				ex.cancelClientOrdersOnBook(client, book, book.Instrument)
			},
		},
		{
			name:   "self-trade prevention",
			reason: exchangeForcedSTPReason,
			check: func(ex *DefaultExchange, book *OrderBook, client *Client, _ Logger) {
				ex.cancelOwnCrossingQuotes(client, book, &Order{ClientID: client.ID, Side: Buy, Price: 100})
			},
		},
		{
			name:   "book admission",
			reason: exchangeForcedBookAdmissionReason,
			check: func(ex *DefaultExchange, book *OrderBook, client *Client, log Logger) {
				order := getOrder()
				order.ID, order.ClientID = 99, client.ID
				order.Side, order.Type, order.TimeInForce = Buy, LimitOrder, GTC
				order.Price, order.Qty, order.Status = 98, 2, Open
				order.Parent = &Limit{}
				ex.restOrReleaseOrder(client, book, order, &OrderRequest{
					RequestID: 99, Symbol: book.Symbol, Side: Buy, Type: LimitOrder,
					Price: 98, Qty: 2, TimeInForce: GTC,
				}, log)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ex := newPostOnlyTestExchange(t)
			log := &recordingLogger{}
			ex.SetLogger("ABC/USD", log)
			response := ex.PlaceOrder(1, &OrderRequest{
				RequestID: 1, Symbol: "ABC/USD", Side: Sell, Type: LimitOrder,
				Price: 100, Qty: 2, TimeInForce: GTC, Visibility: Normal,
			})
			if !response.Success {
				t.Fatalf("seed order rejected: %+v", response)
			}
			book := ex.Books["ABC/USD"]
			client := ex.Clients[1]
			tc.check(ex, book, client, log)
			for _, record := range log.records {
				if record.event != "OrderCancelled" {
					continue
				}
				payload, ok := record.data.(map[string]any)
				if ok && payload["reason"] == tc.reason {
					return
				}
			}
			t.Fatalf("no %s forced-cancellation reason in %#v", tc.reason, log.records)
		})
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
