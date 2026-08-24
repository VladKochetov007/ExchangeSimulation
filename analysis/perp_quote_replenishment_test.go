package analysis

import (
	"strings"
	"testing"
)

func TestPerpQuoteReplenishmentReplaysConfirmedResidualAndRefresh(t *testing.T) {
	run := p3ReplenishmentTestRun(t, p3ReplenishmentValidLines())
	result, err := run.MeasurePerpQuoteReplenishment()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.Decisions != 1 || result.EnabledDecisions != 1 || result.RefreshDue != 1 || result.Accepted != 2 || result.Rejected != 0 || result.LifecycleRows != 3 || result.ThresholdMismatches != 0 || result.LifecycleMismatches != 0 || result.MissingOutcomes != 0 || len(result.Checks) != 0 {
		t.Fatalf("valid P3 replenishment replay = %+v", result)
	}
}

func TestPerpQuoteReplenishmentUsesTradeIDForSameTimestampFills(t *testing.T) {
	lifecycle := &perpQuoteReplenishmentLifecycle{
		ExchangeTimestamp: 3,
		ObservedAt:        4,
		Side:              "BUY",
		Qty:               25,
		TradeID:           71,
	}
	rows := []perpQuoteVenueFill{
		{At: 3, OrderID: 10, TradeID: 71, Symbol: "ABC-PERP", Side: "BUY", Qty: 25},
		{At: 3, OrderID: 10, TradeID: 72, Symbol: "ABC-PERP", Side: "BUY", Qty: 25},
	}
	if !perpQuoteHasVenueFill(rows, lifecycle, perpQuoteParticipant{venue: "north", client: 7}) {
		t.Fatal("distinct same-timestamp pro-rata fill did not join its trade identity")
	}
	lifecycle.TradeID = 73
	if perpQuoteHasVenueFill(rows, lifecycle, perpQuoteParticipant{venue: "north", client: 7}) {
		t.Fatal("unknown trade identity joined a same-timestamp fill")
	}
}

func TestPerpQuoteReplenishmentAcceptsZeroValuedFirstTradeID(t *testing.T) {
	lines := replaceP3EventField(p3ReplenishmentValidLines(), "OrderFill", `"trade_id":71`, `"trade_id":0`)
	lines = replaceP3LifecycleTransitionField(lines, "PARTIAL_FILL", `"trade_id":71`, `"trade_id":0`)
	result, err := p3ReplenishmentTestRun(t, lines).MeasurePerpQuoteReplenishment()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid {
		t.Fatalf("zero-valued first trade ID became unavailable: %+v", result)
	}
}

func TestPerpQuoteReplenishmentCatchesLifecycleAndThresholdMutations(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func([]string) []string
		caught func(*PerpQuoteReplenishmentAudit) bool
	}{
		{
			name: "suppressed residual decrement",
			mutate: func(lines []string) []string {
				return replaceP3Line(lines, `"known_resting_qty":49`, `"known_resting_qty":100`)
			},
			caught: func(result *PerpQuoteReplenishmentAudit) bool { return result.LifecycleMismatches > 0 },
		},
		{
			name: "side swapped partial fill",
			mutate: func(lines []string) []string {
				return replaceP3LifecycleTransitionField(lines, "PARTIAL_FILL", `"side":"BUY"`, `"side":"SELL"`)
			},
			caught: func(result *PerpQuoteReplenishmentAudit) bool { return result.LifecycleMismatches > 0 },
		},
		{
			name: "non strict threshold mutation",
			mutate: func(lines []string) []string {
				lines = replaceP3EventField(lines, "perp_quote_replenishment_decision", `"refresh_due":true`, `"refresh_due":false`)
				return replaceP3EventField(lines, "perp_quote_replenishment_decision", `"reason":"BID_BELOW_THRESHOLD"`, `"reason":"ABOVE_THRESHOLD"`)
			},
			caught: func(result *PerpQuoteReplenishmentAudit) bool { return result.ThresholdMismatches > 0 },
		},
		{
			name: "duplicate venue fill",
			mutate: func(lines []string) []string {
				return append(lines, p3FillLine(3, 10, "BUY", 51, 71))
			},
			caught: func(result *PerpQuoteReplenishmentAudit) bool { return result.LifecycleMismatches > 0 },
		},
		{
			name: "wrong trade identity",
			mutate: func(lines []string) []string {
				return replaceP3LifecycleTransitionField(lines, "PARTIAL_FILL", `"trade_id":71`, `"trade_id":72`)
			},
			caught: func(result *PerpQuoteReplenishmentAudit) bool { return result.LifecycleMismatches > 0 },
		},
		{
			name: "policy disabled fake refresh",
			mutate: func(lines []string) []string {
				lines = replaceP3EventField(lines, "perp_quote_replenishment_decision", `"enabled":true`, `"enabled":false`)
				lines = replaceP3EventField(lines, "perp_quote_replenishment_decision", `"threshold_bps":5000`, `"threshold_bps":0`)
				return lines
			},
			caught: func(result *PerpQuoteReplenishmentAudit) bool { return result.ThresholdMismatches > 0 },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := p3ReplenishmentTestRun(t, tc.mutate(p3ReplenishmentValidLines()))
			result, err := run.MeasurePerpQuoteReplenishment()
			if err != nil {
				t.Fatal(err)
			}
			if result.Valid || !tc.caught(result) {
				t.Fatalf("mutation survived: %+v", result)
			}
		})
	}
}

func TestPerpQuoteReplenishmentRejectsFutureActorReceipt(t *testing.T) {
	lines := replaceP3LifecycleTransitionField(p3ReplenishmentValidLines(), "PARTIAL_FILL", `"observed_at":4`, `"observed_at":2`)
	run := p3ReplenishmentTestRun(t, lines)
	result, err := run.MeasurePerpQuoteReplenishment()
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid || result.LifecycleMismatches == 0 {
		t.Fatalf("future actor receipt survived: %+v", result)
	}
}

func p3ReplenishmentTestRun(t *testing.T, lines []string) *Run {
	t.Helper()
	return &Run{
		files: []string{writeLog(t, lines)},
		roles: map[Participant]string{{VenueID: "north", ClientID: 7}: "perp_maker"},
	}
}

func p3ReplenishmentValidLines() []string {
	return []string{
		p3OrderLine(1, 1, 10, "BUY", 99, 100),
		p3OrderLine(1, 2, 11, "SELL", 101, 100),
		logLine(2, 7, "perp_quote_replenishment_lifecycle", map[string]any{
			"venue_id": "north", "maker": "perp_maker", "client_id": 7, "symbol": "ABC-PERP", "observed_at": 2, "exchange_timestamp": 0,
			"transition": "ACKNOWLEDGED", "side": "BUY", "request_id": 1, "order_id": 10, "qty": 0, "target_qty": 100, "known_resting_qty": 100,
		}),
		logLine(2, 7, "perp_quote_replenishment_lifecycle", map[string]any{
			"venue_id": "north", "maker": "perp_maker", "client_id": 7, "symbol": "ABC-PERP", "observed_at": 2, "exchange_timestamp": 0,
			"transition": "ACKNOWLEDGED", "side": "SELL", "request_id": 2, "order_id": 11, "qty": 0, "target_qty": 100, "known_resting_qty": 100,
		}),
		p3FillLine(3, 10, "BUY", 51, 71),
		logLine(4, 7, "perp_quote_replenishment_lifecycle", map[string]any{
			"venue_id": "north", "maker": "perp_maker", "client_id": 7, "symbol": "ABC-PERP", "observed_at": 4, "exchange_timestamp": 3,
			"transition": "PARTIAL_FILL", "side": "BUY", "request_id": 0, "order_id": 10, "trade_id": 71, "qty": 51, "target_qty": 100, "known_resting_qty": 49,
		}),
		logLine(5, 7, "perp_quote_replenishment_decision", map[string]any{
			"venue_id": "north", "maker": "perp_maker", "client_id": 7, "symbol": "ABC-PERP", "decision_time": 5,
			"enabled": true, "threshold_bps": 5000, "bid_order_id": 10, "ask_order_id": 11,
			"bid_target_qty": 100, "ask_target_qty": 100, "bid_known_resting_qty": 49, "ask_known_resting_qty": 100,
			"bid_replenishment_due": true, "ask_replenishment_due": false, "refresh_due": true, "reason": "BID_BELOW_THRESHOLD",
			"bid_price": 99, "ask_price": 101, "bid_request_id": 30, "ask_request_id": 31,
			"outcome_expectation": "VENUE_OUTCOME_REQUIRED",
		}),
		p3OrderLine(6, 30, 20, "BUY", 99, 100),
		p3OrderLine(6, 31, 21, "SELL", 101, 100),
	}
}

func p3OrderLine(ts int64, requestID, orderID uint64, side string, price, qty int64) string {
	return logLine(ts, 7, "OrderAccepted", map[string]any{
		"request_id": requestID, "order_id": orderID, "client_id": 7, "symbol": "ABC-PERP", "side": side,
		"type": "LIMIT", "time_in_force": "GTC", "post_only": false, "price": price, "qty": qty,
	})
}

func p3FillLine(ts int64, orderID uint64, side string, qty int64, tradeID uint64) string {
	return logLine(ts, 7, "OrderFill", map[string]any{
		"order_id": orderID, "trade_id": tradeID, "symbol": "ABC-PERP", "side": side, "qty": qty,
	})
}

func replaceP3Line(lines []string, old, new string) []string {
	result := append([]string(nil), lines...)
	for index, line := range result {
		result[index] = strings.Replace(line, old, new, 1)
	}
	return result
}

func replaceP3EventField(lines []string, event, old, new string) []string {
	result := append([]string(nil), lines...)
	for index, line := range result {
		if strings.Contains(line, `"event":"`+event+`"`) {
			result[index] = strings.Replace(line, old, new, 1)
		}
	}
	return result
}

func replaceP3LifecycleTransitionField(lines []string, transition, old, new string) []string {
	result := append([]string(nil), lines...)
	for index, line := range result {
		if strings.Contains(line, `"event":"perp_quote_replenishment_lifecycle"`) && strings.Contains(line, `"transition":"`+transition+`"`) {
			result[index] = strings.Replace(line, old, new, 1)
		}
	}
	return result
}
