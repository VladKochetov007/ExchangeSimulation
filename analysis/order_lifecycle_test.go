package analysis

import (
	"fmt"
	"testing"
)

func lifecycleAcceptedLine(ts int64, venue string, clientID, orderID uint64, typ, tif string, quantity int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"OrderAccepted","data":{"venue_id":%q,"payload":{"order_id":%d,"client_id":%d,"type":%q,"time_in_force":%q,"qty":%d}}}`,
		ts, clientID, venue, orderID, clientID, typ, tif, quantity)
}

func lifecycleFillLine(ts int64, venue string, clientID, orderID uint64, quantity, filled, remaining int64, full bool) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"OrderFill","data":{"venue_id":%q,"payload":{"order_id":%d,"qty":%d,"filled_qty":%d,"remaining_qty":%d,"is_full":%t}}}`,
		ts, clientID, venue, orderID, quantity, filled, remaining, full)
}

func lifecycleCancelLine(ts int64, venue string, clientID, orderID uint64, remaining int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"OrderCancelled","data":{"venue_id":%q,"payload":{"order_id":%d,"remaining_qty":%d}}}`,
		ts, clientID, venue, orderID, remaining)
}

func lifecycleLiquidationLine(ts int64, venue string, clientID uint64, symbol string) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"liquidation","data":{"venue_id":%q,"payload":{"symbol":%q,"position_size":-10,"fill_price":100,"remaining_debt":0}}}`,
		ts, clientID, venue, symbol)
}

func lifecycleFillWithSymbolLine(ts int64, venue string, clientID, orderID uint64, symbol string, quantity, filled, remaining int64, full bool) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"OrderFill","data":{"venue_id":%q,"payload":{"symbol":%q,"order_id":%d,"qty":%d,"filled_qty":%d,"remaining_qty":%d,"is_full":%t}}}`,
		ts, clientID, venue, symbol, orderID, quantity, filled, remaining, full)
}

func TestOrderLifecycleAudit(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		check func(*testing.T, *OrderLifecycleAudit)
	}{
		{
			name: "accepts_terminal_immediate_and_open_gtc",
			lines: []string{
				lifecycleAcceptedLine(1, "north", 1, 10, "MARKET", "GTC", 10),
				lifecycleCancelLine(2, "north", 1, 10, 10),
				lifecycleAcceptedLine(3, "north", 2, 11, "LIMIT", "GTC", 10),
			},
			check: func(t *testing.T, got *OrderLifecycleAudit) {
				t.Helper()
				if got.MissingImmediateTerminal != 0 || got.RequiredImmediateTerminal != 1 || len(got.Checks) != 0 {
					t.Fatalf("unexpected audit: %+v", got)
				}
			},
		},
		{
			name: "catches_missing_immediate_cancellation",
			lines: []string{
				lifecycleAcceptedLine(1, "north", 1, 10, "IOC", "IOC", 10),
			},
			check: func(t *testing.T, got *OrderLifecycleAudit) {
				t.Helper()
				if got.MissingImmediateTerminal != 1 || len(got.Checks) != 1 || got.Checks[0].Failure != "missing_immediate_terminal" {
					t.Fatalf("unexpected audit: %+v", got)
				}
			},
		},
		{
			name: "catches_fill_after_cancel",
			lines: []string{
				lifecycleAcceptedLine(1, "north", 1, 10, "LIMIT", "GTC", 10),
				lifecycleCancelLine(2, "north", 1, 10, 10),
				lifecycleFillLine(3, "north", 1, 10, 1, 1, 9, false),
			},
			check: func(t *testing.T, got *OrderLifecycleAudit) {
				t.Helper()
				if got.FillsAfterTerminal != 1 || len(got.Checks) != 1 || got.Checks[0].Failure != "fill_after_terminal" {
					t.Fatalf("unexpected audit: %+v", got)
				}
			},
		},
		{
			name: "catches_cumulative_fill_mismatch",
			lines: []string{
				lifecycleAcceptedLine(1, "north", 1, 10, "LIMIT", "GTC", 10),
				lifecycleFillLine(2, "north", 1, 10, 4, 5, 6, false),
			},
			check: func(t *testing.T, got *OrderLifecycleAudit) {
				t.Helper()
				if got.FillQuantityMismatches != 1 || len(got.Checks) != 1 || got.Checks[0].Failure != "fill_quantity_mismatch" {
					t.Fatalf("unexpected audit: %+v", got)
				}
			},
		},
		{
			name: "requires_full_flag_at_quantity_terminal",
			lines: []string{
				lifecycleAcceptedLine(1, "north", 1, 10, "LIMIT", "GTC", 10),
				lifecycleFillLine(2, "north", 1, 10, 10, 10, 0, false),
			},
			check: func(t *testing.T, got *OrderLifecycleAudit) {
				t.Helper()
				if got.FillQuantityMismatches != 1 || len(got.Checks) != 1 || got.Checks[0].Failure != "fill_quantity_mismatch" {
					t.Fatalf("a quantity-terminal fill without is_full was accepted: %+v", got)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := writeRun(t, Report{}, map[string][]string{"north/spot/ABC-USD.jsonl": test.lines})
			run, err := Open(dir)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			got, err := run.MeasureOrderLifecycle()
			if err != nil {
				t.Fatalf("measure: %v", err)
			}
			test.check(t, got)
		})
	}
}

func TestOrderLifecycleSeparatesBooksWithReusedOrderIDs(t *testing.T) {
	// The exchange allocates order IDs per venue, while independent book logs
	// can reuse an ID. The lifecycle key must therefore include the source
	// file; otherwise a fill in one book can be paired with an acceptance in
	// another (or appear to be an unknown fill when Scan visits files out of
	// order).
	dir := writeRun(t, Report{}, map[string][]string{
		"north/spot/ABC-USD.jsonl": {
			lifecycleAcceptedLine(1, "north", 1, 10, "LIMIT", "GTC", 10),
			lifecycleFillLine(2, "north", 1, 10, 10, 10, 0, true),
		},
		"north/derivatives/ABC-PERP.jsonl": {
			lifecycleAcceptedLine(1, "north", 2, 10, "LIMIT", "GTC", 10),
			lifecycleFillLine(2, "north", 2, 10, 10, 10, 0, true),
		},
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	got, err := run.MeasureOrderLifecycle()
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if got.Accepted != 2 || got.FillRecords != 2 || got.UnknownFills != 0 ||
		got.DuplicateAcceptances != 0 || len(got.Checks) != 0 {
		t.Fatalf("reused book order ID was not separated: %+v", got)
	}
}

func TestOrderLifecycleLinksForcedCloseFills(t *testing.T) {
	const instant = int64(1_000_000_000)
	dir := writeRun(t, Report{}, map[string][]string{
		"north/derivatives/ABC-PERP.jsonl": {
			lifecycleFillWithSymbolLine(instant, "north", 7, 99, "ABC-PERP", 10, 10, 0, true),
			lifecycleLiquidationLine(instant, "north", 7, "ABC-PERP"),
		},
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	got, err := run.MeasureOrderLifecycle()
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if got.UnknownFills != 1 || got.LiquidationFills != 1 || got.UnlinkedFills != 0 || len(got.Checks) != 0 {
		t.Fatalf("forced-close fill was not linked: %+v", got)
	}

	dir = writeRun(t, Report{}, map[string][]string{
		"north/derivatives/ABC-PERP.jsonl": {
			lifecycleFillWithSymbolLine(instant, "north", 7, 99, "ABC-PERP", 10, 10, 0, true),
		},
	})
	run, _ = Open(dir)
	got, err = run.MeasureOrderLifecycle()
	if err != nil {
		t.Fatalf("measure missing liquidation: %v", err)
	}
	if got.UnknownFills != 1 || got.LiquidationFills != 0 || got.UnlinkedFills != 1 || len(got.Checks) != 1 {
		t.Fatalf("unlinked forced-close mutation was not rejected: %+v", got)
	}
}

func TestOrderLifecycleCountsMalformedTerminalEvidence(t *testing.T) {
	lines := []string{
		`{"sim_ts":1,"client_id":1,"event":"OrderAccepted","data":{"venue_id":"north","payload":{"order_id":10,"qty":2}}}`,
		`{"sim_ts":2,"client_id":1,"event":"OrderFill","data":{"venue_id":"north","payload":"broken"}}`,
		`{"sim_ts":3,"client_id":1,"event":"OrderCancelled","data":{"venue_id":"north","payload":{"order_id":10}}}`,
		`{"sim_ts":4,"client_id":1,"event":"liquidation","data":{"venue_id":"north","payload":{}}}`,
	}
	run, err := Open(writeRun(t, Report{}, map[string][]string{"north/spot/ABC-USD.jsonl": lines}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureOrderLifecycle()
	if err != nil {
		t.Fatal(err)
	}
	if result.MalformedAcceptedRecords != 1 || result.MalformedFillRecords != 1 || result.MalformedCancelRecords != 1 || result.MalformedLiquidations != 1 {
		t.Fatalf("malformed lifecycle evidence disappeared: %+v", result)
	}
}
