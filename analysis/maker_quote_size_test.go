package analysis

import "testing"

func TestMakerQuoteSizeAuditJoinsExactRequestEvidence(t *testing.T) {
	lines := []string{
		logLine(1, 7, "maker_quote_size_decision", map[string]any{
			"maker": "spot_maker_1", "client_id": 7, "symbol": "ABC/USD", "bid_request_id": 10, "ask_request_id": 11,
			"base_volatility_size": 100, "risk_position": 50, "inventory_limit": 100, "size_skew_bps": 5000,
			"full_adjustment": 50, "adjustment": 25, "bid_price": 99, "ask_price": 101, "bid_qty": 75, "ask_qty": 125, "post_only": true, "cancel_before_replace": true, "outcome_expectation": "VENUE_OUTCOME_REQUIRED",
		}),
		logLine(2, 7, "maker_quote_size_decision", map[string]any{
			"maker": "spot_maker_1", "client_id": 7, "symbol": "ABC/USD", "bid_request_id": 12, "ask_request_id": 13,
			"base_volatility_size": 100, "risk_position": 0, "inventory_limit": 100, "size_skew_bps": 0,
			"full_adjustment": 0, "adjustment": 0, "bid_price": 99, "ask_price": 101, "bid_qty": 100, "ask_qty": 100, "post_only": true, "cancel_before_replace": true, "outcome_expectation": "VENUE_OUTCOME_REQUIRED",
		}),
		p1OrderLine(3, 7, 10, "BUY", 75, ""),
		p1OrderLine(3, 7, 11, "SELL", 125, "POST_ONLY_WOULD_TAKE"),
		p1OrderLine(4, 7, 12, "BUY", 100, ""),
		p1OrderLine(4, 7, 13, "SELL", 100, ""),
	}
	run := p1TestRun(t, lines)
	result, err := run.MeasureMakerQuoteSize(MakerQuoteSizeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decisions != 2 || result.DecisionSides != 4 || result.Accepted != 3 || result.Rejected != 1 ||
		result.MissingOutcomes != 0 || result.DuplicateOutcomes != 0 || result.DecisionFieldMismatches != 0 || result.OutcomeFieldMismatches != 0 || result.InvalidDecisionRecords != 0 || len(result.Checks) != 0 {
		t.Fatalf("P1 exact join audit = %+v", result)
	}
	if len(result.SkewBuckets) != 2 || result.SkewBuckets[0].SizeSkewBps != 0 || result.SkewBuckets[1].SizeSkewBps != 5_000 || result.SkewBuckets[1].Adjusted != 1 {
		t.Fatalf("P1 skew buckets = %+v", result.SkewBuckets)
	}
}

func TestMakerQuoteSizeAuditCatchesPolicyAndEvidenceMutations(t *testing.T) {
	tests := []struct {
		name         string
		decision     map[string]any
		bidQty       int64
		outcomeQty   int64
		wantDecision bool
		wantOutcome  bool
	}{
		{
			name: "side swap mutation", bidQty: 125, outcomeQty: 75, wantDecision: true,
			decision: map[string]any{"base_volatility_size": 100, "risk_position": 50, "inventory_limit": 100, "size_skew_bps": 5000, "full_adjustment": 50, "adjustment": 25, "bid_price": 99, "ask_price": 101, "bid_qty": 125, "ask_qty": 75},
		},
		{
			name: "coefficient strip mutation", bidQty: 100, outcomeQty: 100, wantDecision: true,
			decision: map[string]any{"base_volatility_size": 100, "risk_position": 50, "inventory_limit": 100, "size_skew_bps": 5000, "full_adjustment": 0, "adjustment": 0, "bid_price": 99, "ask_price": 101, "bid_qty": 100, "ask_qty": 100},
		},
		{
			name: "rejected quantity mutation", bidQty: 75, outcomeQty: 124, wantOutcome: true,
			decision: map[string]any{"base_volatility_size": 100, "risk_position": 50, "inventory_limit": 100, "size_skew_bps": 5000, "full_adjustment": 50, "adjustment": 25, "bid_price": 99, "ask_price": 101, "bid_qty": 75, "ask_qty": 125},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			decision := map[string]any{
				"maker": "spot_maker_1", "client_id": 7, "symbol": "ABC/USD", "bid_request_id": 10, "ask_request_id": 11,
				"post_only": true, "cancel_before_replace": true, "outcome_expectation": "VENUE_OUTCOME_REQUIRED",
			}
			for field, value := range tc.decision {
				decision[field] = value
			}
			lines := []string{
				logLine(1, 7, "maker_quote_size_decision", decision),
				p1OrderLine(2, 7, 10, "BUY", tc.bidQty, ""),
				p1OrderLine(2, 7, 11, "SELL", tc.outcomeQty, "POST_ONLY_WOULD_TAKE"),
			}
			run := p1TestRun(t, lines)
			result, err := run.MeasureMakerQuoteSize(MakerQuoteSizeOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if (result.DecisionFieldMismatches > 0) != tc.wantDecision || (result.OutcomeFieldMismatches > 0) != tc.wantOutcome {
				t.Fatalf("mutation audit = %+v, want decision=%t outcome=%t", result, tc.wantDecision, tc.wantOutcome)
			}
		})
	}
}

func TestMakerQuoteSizeAuditRefusesMissingOutcome(t *testing.T) {
	decision := map[string]any{
		"maker": "spot_maker_1", "client_id": 7, "symbol": "ABC/USD", "bid_request_id": 10, "ask_request_id": 11,
		"base_volatility_size": 100, "risk_position": 50, "inventory_limit": 100, "size_skew_bps": 5000,
		"full_adjustment": 50, "adjustment": 25, "bid_price": 99, "ask_price": 101, "bid_qty": 75, "ask_qty": 125, "post_only": true, "cancel_before_replace": true, "outcome_expectation": "VENUE_OUTCOME_REQUIRED",
	}
	run := p1TestRun(t, []string{
		logLine(1, 7, "maker_quote_size_decision", decision),
		p1OrderLine(2, 7, 10, "BUY", 75, ""),
	})
	result, err := run.MeasureMakerQuoteSize(MakerQuoteSizeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.MissingOutcomes != 1 || len(result.Checks) != 1 || result.Checks[0].Failure != "missing_request_outcome" {
		t.Fatalf("missing outcome survived P1 audit: %+v", result)
	}
}

func TestMakerQuoteSizeAuditClassifiesExplicitTerminalCensoring(t *testing.T) {
	decision := map[string]any{
		"maker": "spot_maker_1", "client_id": 7, "symbol": "ABC/USD", "decision_time": 10, "bid_request_id": 10, "ask_request_id": 11,
		"base_volatility_size": 100, "risk_position": 50, "inventory_limit": 100, "size_skew_bps": 5000,
		"full_adjustment": 50, "adjustment": 25, "bid_price": 99, "ask_price": 101, "bid_qty": 75, "ask_qty": 125, "post_only": true, "cancel_before_replace": true,
		"outcome_expectation": "SIMULATION_HORIZON_CENSORED", "censor_reason": "terminal_horizon_before_venue_ingress",
	}
	t.Run("valid censor has no missing venue outcome", func(t *testing.T) {
		run := p1TestRun(t, []string{logLine(10, 7, "maker_quote_size_decision", decision)})
		result, err := run.MeasureMakerQuoteSize(MakerQuoteSizeOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if result.HorizonCensoredSides != 2 || result.MissingOutcomes != 0 || result.CensoredOutcomeDeliveries != 0 || result.InvalidCensorRecords != 0 || len(result.Checks) != 0 {
			t.Fatalf("terminal censor audit = %+v", result)
		}
	})
	t.Run("delivered terminal-censored request is caught", func(t *testing.T) {
		run := p1TestRun(t, []string{
			logLine(10, 7, "maker_quote_size_decision", decision),
			p1OrderLine(11, 7, 10, "BUY", 75, ""),
		})
		result, err := run.MeasureMakerQuoteSize(MakerQuoteSizeOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if result.HorizonCensoredSides != 1 || result.CensoredOutcomeDeliveries != 1 || len(result.Checks) != 1 || result.Checks[0].Failure != "terminal_censored_request_delivered" {
			t.Fatalf("delivered censored request survived P1 audit: %+v", result)
		}
	})
}

func p1OrderLine(ts int64, clientID, requestID uint64, side string, qty int64, reject string) string {
	event := "OrderAccepted"
	if reject != "" {
		event = "OrderRejected"
	}
	price := int64(99)
	if side == "SELL" {
		price = 101
	}
	payload := map[string]any{
		"request_id": requestID, "symbol": "ABC/USD", "side": side, "type": "LIMIT", "time_in_force": "GTC",
		"post_only": true, "price": price, "qty": qty,
	}
	if reject != "" {
		payload["error"] = reject
	}
	return logLine(ts, clientID, event, payload)
}

func p1TestRun(t *testing.T, lines []string) *Run {
	t.Helper()
	return &Run{
		files: []string{writeLog(t, lines)},
		roles: map[Participant]string{{VenueID: "north", ClientID: 7}: "spot_maker"},
	}
}
