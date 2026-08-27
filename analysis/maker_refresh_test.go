package analysis

import "testing"

func TestMakerPassiveRefreshOrderingReplaysCancellationBeforeReplacement(t *testing.T) {
	decision := func(ts int64, bid, ask uint64) string {
		return logLine(ts, 7, "maker_quote_size_decision", map[string]any{
			"maker": "spot_maker_1", "client_id": uint64(7), "symbol": "ABC/USD",
			"bid_request_id": bid, "ask_request_id": ask, "post_only": true,
			"cancel_before_replace": true, "outcome_expectation": "VENUE_OUTCOME_REQUIRED",
		})
	}
	accepted := func(ts int64, orderID, requestID uint64, side string) string {
		price := int64(99)
		if side == "SELL" {
			price = 101
		}
		return logLine(ts, 7, "OrderAccepted", map[string]any{
			"order_id": orderID, "client_id": uint64(7), "request_id": requestID,
			"symbol": "ABC/USD", "side": side, "type": "LIMIT", "time_in_force": "GTC",
			"post_only": true, "price": price, "qty": int64(100),
		})
	}
	cancel := func(ts int64, orderID, requestID uint64) string {
		return logLine(ts, 7, "OrderCancelled", map[string]any{
			"order_id": orderID, "request_id": requestID, "remaining_qty": int64(100),
		})
	}

	run, err := Open(writeRun(t, Report{TerminalAccounts: []AccountRow{{VenueID: "north", ClientID: 7, Role: "spot_maker_1"}}}, map[string][]string{
		"north/general.jsonl": {
			decision(1, 10, 11),
			decision(5, 20, 21),
		},
		"north/spot/ABC-USD.jsonl": {
			accepted(2, 101, 10, "BUY"),
			accepted(2, 102, 11, "SELL"),
			cancel(3, 101, 12),
			cancel(3, 102, 12),
			accepted(6, 201, 20, "BUY"),
			accepted(6, 202, 21, "SELL"),
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureMakerPassiveRefreshOrdering(MakerQuoteSizeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.Decisions != 2 || result.DecisionSides != 4 ||
		result.InitialOrNoPrior != 2 || result.Checked != 2 || result.AcceptedOutcomes != 4 ||
		result.CancellationsObserved != 2 || result.Missing != 0 || result.Duplicate != 0 ||
		result.Late != 0 || result.OutOfOrderCancellations != 0 || len(result.Checks) != 0 {
		t.Fatalf("refresh replay = %+v", result)
	}
	if result.LineageRows != result.DecisionSides || result.LineageDigest == "" {
		t.Fatalf("lineage = %+v", result)
	}
}

func TestMakerPassiveRefreshOrderingCatchesOrderingAndQuantityMutations(t *testing.T) {
	tests := []struct {
		name          string
		cancelRequest uint64
		fillQty       int64
		fillRemaining int64
		fillFull      bool
		wantInvalid   bool
		wantLate      bool
	}{
		{name: "replacement accepted before cancellation", cancelRequest: 12, wantLate: true},
		{name: "cancellation request follows replacement", cancelRequest: 20, wantInvalid: true},
		{name: "forged fill quantity", cancelRequest: 12, fillQty: 60, fillRemaining: 0, fillFull: true, wantInvalid: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			decision := func(ts int64, bid, ask uint64) string {
				return logLine(ts, 7, "maker_quote_size_decision", map[string]any{
					"maker": "spot_maker_1", "client_id": uint64(7), "symbol": "ABC/USD",
					"bid_request_id": bid, "ask_request_id": ask, "post_only": true,
					"cancel_before_replace": true, "outcome_expectation": "VENUE_OUTCOME_REQUIRED",
				})
			}
			accepted := func(ts int64, orderID, requestID uint64, side string) string {
				return logLine(ts, 7, "OrderAccepted", map[string]any{
					"order_id": orderID, "client_id": uint64(7), "request_id": requestID,
					"symbol": "ABC/USD", "side": side, "type": "LIMIT", "time_in_force": "GTC",
					"post_only": true, "price": int64(99), "qty": int64(100),
				})
			}
			rejected := func(ts int64, requestID uint64, side string) string {
				return logLine(ts, 7, "OrderRejected", map[string]any{
					"client_id": uint64(7), "request_id": requestID, "side": side,
					"error": "POST_ONLY_WOULD_TAKE",
				})
			}
			lines := []string{decision(1, 10, 11), decision(5, 20, 21), accepted(2, 101, 10, "BUY"), rejected(2, 11, "SELL"), rejected(6, 21, "SELL")}
			if tc.fillQty != 0 {
				lines = append(lines, logLine(3, 7, "OrderFill", map[string]any{
					"order_id": uint64(101), "qty": tc.fillQty, "filled_qty": tc.fillQty,
					"remaining_qty": tc.fillRemaining, "is_full": tc.fillFull,
				}))
			}
			if tc.name == "replacement accepted before cancellation" {
				lines = append(lines, accepted(6, 201, 20, "BUY"), logLine(7, 7, "OrderCancelled", map[string]any{
					"order_id": uint64(101), "request_id": tc.cancelRequest, "remaining_qty": int64(100),
				}))
			} else {
				lines = append(lines, logLine(6, 7, "OrderCancelled", map[string]any{
					"order_id": uint64(101), "request_id": tc.cancelRequest, "remaining_qty": int64(100),
				}), accepted(7, 201, 20, "BUY"))
			}
			run, err := Open(writeRun(t, Report{TerminalAccounts: []AccountRow{{VenueID: "north", ClientID: 7, Role: "spot_maker_1"}}}, map[string][]string{
				"north/general.jsonl":      {decision(1, 10, 11), decision(5, 20, 21)},
				"north/spot/ABC-USD.jsonl": lines[2:],
			}))
			if err != nil {
				t.Fatal(err)
			}
			result, err := run.MeasureMakerPassiveRefreshOrdering(MakerQuoteSizeOptions{})
			if err != nil {
				t.Fatal(err)
			}
			expectedValid := !tc.wantInvalid && !tc.wantLate
			if result.Valid != expectedValid || (result.Late > 0) != tc.wantLate {
				t.Fatalf("mutation result = %+v, want invalid=%t late=%t", result, tc.wantInvalid, tc.wantLate)
			}
			if tc.fillQty != 0 && result.FillQuantityMismatches == 0 {
				t.Fatalf("forged fill survived: %+v", result)
			}
		})
	}
}
