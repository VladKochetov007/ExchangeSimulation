package analysis

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func lifecycleForcedFillWithSymbolLine(ts int64, venue string, clientID, orderID uint64, symbol string, quantity, filled, remaining int64, full bool) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"OrderFill","data":{"venue_id":%q,"payload":{"symbol":%q,"order_id":%d,"qty":%d,"filled_qty":%d,"remaining_qty":%d,"is_full":%t,"forced":true}}}`,
		ts, clientID, venue, symbol, orderID, quantity, filled, remaining, full)
}

func lifecycleStrictForcedFillLine(ts int64, venue string, clientID, orderID uint64, symbol, side, positionSide string, quantity, filled, remaining int64, full bool, liquidationID uint64) string {
	return lifecycleStrictFillLine(ts, venue, clientID, orderID, symbol, side, positionSide, quantity, filled, remaining, full, 1, "taker", true, liquidationID)
}

func lifecycleStrictFillLine(ts int64, venue string, clientID, orderID uint64, symbol, side, positionSide string, quantity, filled, remaining int64, full bool, tradeID uint64, role string, forced bool, liquidationID uint64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"OrderFill","data":{"venue_id":%q,"payload":{"symbol":%q,"order_id":%d,"qty":%d,"price":100,"filled_qty":%d,"remaining_qty":%d,"is_full":%t,"side":%q,"role":%q,"position_side":%q,"trade_id":%d,"forced":%t,"liquidation_id":%d}}}`,
		ts, clientID, venue, symbol, orderID, quantity, filled, remaining, full, side, role, positionSide, tradeID, forced, liquidationID)
}

func lifecycleTradeLine(ts int64, venue string, tradeID uint64, price, quantity int64, side string, takerOrderID, makerOrderID uint64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":0,"event":"Trade","data":{"venue_id":%q,"payload":{"trade_id":%d,"price":%d,"qty":%d,"side":%q,"taker_order_id":%d,"maker_order_id":%d}}}`,
		ts, venue, tradeID, price, quantity, side, takerOrderID, makerOrderID)
}

func lifecycleStrictLiquidationLine(ts int64, venue string, clientID uint64, symbol, positionSide string, liquidationID, forcedOrderID uint64, positionSize, attemptedQty, filledQty, remainingQty int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"liquidation","data":{"venue_id":%q,"payload":{"symbol":%q,"position_side":%q,"liquidation_id":%d,"forced_order_id":%d,"position_size":%d,"attempted_qty":%d,"filled_qty":%d,"remaining_qty":%d,"filled_notional":%d,"vwap_price":100,"fill_price":100,"base_precision":1,"remaining_debt":0}}}`,
		ts, clientID, venue, symbol, positionSide, liquidationID, forcedOrderID, positionSize, attemptedQty, filledQty, remainingQty, filledQty*100)
}

func openStrictLifecycleRun(t *testing.T, lines []string) *Run {
	t.Helper()
	dir := writeRun(t, Report{}, map[string][]string{
		"north/derivatives/ABC-PERP.jsonl": lines,
	})
	if err := os.WriteFile(filepath.Join(dir, "run-config.json"), []byte(`{"evidence_format":"evstream_v3"}`), 0o644); err != nil {
		t.Fatalf("write strict descriptor: %v", err)
	}
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open strict run: %v", err)
	}
	return run
}

func strictTradeLifecycleLines(tradeTimestamp int64) []string {
	return []string{
		lifecycleAcceptedLine(1, "north", 1, 99, "MARKET", "GTC", 10),
		lifecycleAcceptedLine(1, "north", 2, 100, "LIMIT", "GTC", 10),
		lifecycleTradeLine(tradeTimestamp, "north", 7, 100, 10, "BUY", 99, 100),
		lifecycleStrictFillLine(4, "north", 1, 99, "ABC-PERP", "BUY", "BOTH", 10, 10, 0, true, 7, "taker", false, 0),
		lifecycleStrictFillLine(4, "north", 2, 100, "ABC-PERP", "SELL", "BOTH", 10, 10, 0, true, 7, "maker", false, 0),
	}
}

func lifecycleExplicitOrdinaryFillWithSymbolLine(ts int64, venue string, clientID, orderID uint64, symbol string, quantity, filled, remaining int64, full bool) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"OrderFill","data":{"venue_id":%q,"client_id":%d,"symbol":%q,"payload":{"symbol":%q,"order_id":%d,"qty":%d,"filled_qty":%d,"remaining_qty":%d,"is_full":%t,"forced":false}}}`,
		ts, clientID, venue, clientID, symbol, symbol, orderID, quantity, filled, remaining, full)
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

func TestOrderLifecycleSuccessorRequiresExplicitForcedMarker(t *testing.T) {
	const instant = int64(1_000_000_000)
	strictReceipt := lifecycleStrictLiquidationLine(instant, "north", 7, "ABC-PERP", "BOTH", 1, 99, -10, 10, 10, 0)
	makeRun := func(t *testing.T, fill string) *Run {
		t.Helper()
		dir := writeRun(t, Report{}, map[string][]string{
			"north/derivatives/ABC-PERP.jsonl": {fill, strictReceipt},
		})
		if err := os.WriteFile(filepath.Join(dir, "run-config.json"), []byte(`{"evidence_format":"evstream_v3"}`), 0o644); err != nil {
			t.Fatalf("write successor descriptor: %v", err)
		}
		run, err := Open(dir)
		if err != nil {
			t.Fatalf("open successor: %v", err)
		}
		return run
	}

	strictFill := lifecycleStrictForcedFillLine(instant, "north", 7, 99, "ABC-PERP", "BUY", "BOTH", 10, 10, 0, true, 1)
	strictDir := writeRun(t, Report{}, map[string][]string{
		"north/derivatives/ABC-PERP.jsonl": {
			lifecycleTradeLine(instant-2, "north", 1, 100, 10, "BUY", 99, 100),
			lifecycleAcceptedLine(instant-1, "north", 8, 100, "LIMIT", "GTC", 10),
			strictFill,
			lifecycleStrictFillLine(instant, "north", 8, 100, "ABC-PERP", "SELL", "BOTH", 10, 10, 0, true, 1, "maker", false, 0),
			strictReceipt,
		},
	})
	if err := os.WriteFile(filepath.Join(strictDir, "run-config.json"), []byte(`{"evidence_format":"evstream_v3"}`), 0o644); err != nil {
		t.Fatalf("write strict descriptor: %v", err)
	}
	strictRun, err := Open(strictDir)
	if err != nil {
		t.Fatalf("open strict run: %v", err)
	}
	forced, err := strictRun.MeasureOrderLifecycle()
	if err != nil {
		t.Fatalf("measure forced successor fill: %v", err)
	}
	if forced.LiquidationFills != 1 || forced.UnlinkedFills != 0 {
		t.Fatalf("explicit forced marker was not accepted: %+v", forced)
	}
	ordinary, err := makeRun(t, lifecycleFillWithSymbolLine(instant, "north", 7, 99, "ABC-PERP", 10, 10, 0, true)).MeasureOrderLifecycle()
	if err != nil {
		t.Fatalf("measure ordinary successor fill: %v", err)
	}
	if ordinary.LiquidationFills != 0 || ordinary.UnlinkedFills != 1 {
		t.Fatalf("ordinary unknown fill was rescued by liquidation row: %+v", ordinary)
	}
}

func TestOrderLifecycleRejectsForcedIdentityOnAcceptedOrder(t *testing.T) {
	const instant = int64(1_000_000_000)
	dir := writeRun(t, Report{}, map[string][]string{
		"north/derivatives/ABC-PERP.jsonl": {
			lifecycleTradeLine(instant-2, "north", 1, 100, 10, "BUY", 99, 100),
			lifecycleAcceptedLine(instant-1, "north", 7, 99, "LIMIT", "GTC", 10),
			lifecycleAcceptedLine(instant-1, "north", 8, 100, "LIMIT", "GTC", 10),
			lifecycleStrictForcedFillLine(instant, "north", 7, 99, "ABC-PERP", "BUY", "BOTH", 10, 10, 0, true, 1),
			lifecycleStrictFillLine(instant, "north", 8, 100, "ABC-PERP", "SELL", "BOTH", 10, 10, 0, true, 1, "maker", false, 0),
		},
	})
	if err := os.WriteFile(filepath.Join(dir, "run-config.json"), []byte(`{"evidence_format":"evstream_v3"}`), 0o644); err != nil {
		t.Fatalf("write successor descriptor: %v", err)
	}
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open successor: %v", err)
	}
	result, err := run.MeasureOrderLifecycle()
	if err != nil {
		t.Fatalf("measure successor: %v", err)
	}
	if result.LiquidationIdentityFailures != 1 || result.LiquidationFills != 0 || len(result.Checks) != 1 || result.Checks[0].Failure != "accepted_order_has_forced_identity" {
		t.Fatalf("forced identity collision was not rejected: %+v", result)
	}
}

func TestOrderLifecycleCountsMissingForcedReceiptFill(t *testing.T) {
	const instant = int64(1_000_000_000)
	dir := writeRun(t, Report{}, map[string][]string{
		"north/derivatives/ABC-PERP.jsonl": {
			lifecycleStrictLiquidationLine(instant, "north", 7, "ABC-PERP", "BOTH", 1, 99, -10, 10, 10, 0),
		},
	})
	if err := os.WriteFile(filepath.Join(dir, "run-config.json"), []byte(`{"evidence_format":"evstream_v3"}`), 0o644); err != nil {
		t.Fatalf("write successor descriptor: %v", err)
	}
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open successor: %v", err)
	}
	result, err := run.MeasureOrderLifecycle()
	if err != nil {
		t.Fatalf("measure successor: %v", err)
	}
	if result.MissingForcedFills != 1 || result.LiquidationFills != 0 {
		t.Fatalf("missing forced fill was not counted: %+v", result)
	}
}

func TestOrderLifecycleStrictTradeMutationsFailClosed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]string) []string
		check  func(*testing.T, *OrderLifecycleAudit)
	}{
		{
			name:   "valid_pair",
			mutate: func(lines []string) []string { return lines },
			check: func(t *testing.T, result *OrderLifecycleAudit) {
				if result.TradeRecords != 1 || result.TradeIdentityFailures != 0 || result.TradeFieldMismatches != 0 || result.TradeCausalityFailures != 0 || result.TradeCompletenessFailures != 0 || len(result.Checks) != 0 {
					t.Fatalf("valid strict trade was rejected: %+v", result)
				}
			},
		},
		{
			name: "missing_trade",
			mutate: func(lines []string) []string {
				return []string{lines[0], lines[1], lines[3], lines[4]}
			},
			check: func(t *testing.T, result *OrderLifecycleAudit) {
				if result.TradeIdentityFailures != 2 || len(result.Checks) != 2 {
					t.Fatalf("missing trade was not rejected per fill: %+v", result)
				}
			},
		},
		{
			name: "price_mismatch",
			mutate: func(lines []string) []string {
				mutated := append([]string{}, lines...)
				mutated[3] = strings.Replace(mutated[3], `"price":100`, `"price":101`, 1)
				return mutated
			},
			check: func(t *testing.T, result *OrderLifecycleAudit) {
				if result.TradeFieldMismatches != 1 || result.TradeCompletenessFailures != 1 {
					t.Fatalf("price mutation was not rejected: %+v", result)
				}
			},
		},
		{
			name: "fill_before_trade",
			mutate: func(lines []string) []string {
				mutated := append([]string{}, lines...)
				mutated[2] = lifecycleTradeLine(5, "north", 7, 100, 10, "BUY", 99, 100)
				return mutated
			},
			check: func(t *testing.T, result *OrderLifecycleAudit) {
				if result.TradeCausalityFailures != 2 || result.TradeCompletenessFailures != 1 {
					t.Fatalf("causal mutation was not rejected: %+v", result)
				}
			},
		},
		{
			name: "role_order_mismatch",
			mutate: func(lines []string) []string {
				mutated := append([]string{}, lines...)
				mutated[3] = strings.Replace(mutated[3], `"role":"taker"`, `"role":"maker"`, 1)
				return mutated
			},
			check: func(t *testing.T, result *OrderLifecycleAudit) {
				if result.TradeIdentityFailures == 0 || result.TradeCompletenessFailures != 1 {
					t.Fatalf("role mutation was not rejected: %+v", result)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run := openStrictLifecycleRun(t, test.mutate(strictTradeLifecycleLines(3)))
			result, err := run.MeasureOrderLifecycle()
			if err != nil {
				t.Fatalf("measure strict lifecycle: %v", err)
			}
			test.check(t, result)
		})
	}
}

func TestOrderLifecycleStrictForcedReceiptMutationsFailClosed(t *testing.T) {
	validLines := []string{
		lifecycleTradeLine(1, "north", 1, 100, 10, "BUY", 99, 100),
		lifecycleAcceptedLine(2, "north", 8, 100, "LIMIT", "GTC", 10),
		lifecycleStrictForcedFillLine(3, "north", 7, 99, "ABC-PERP", "BUY", "BOTH", 10, 10, 0, true, 1),
		lifecycleStrictFillLine(3, "north", 8, 100, "ABC-PERP", "SELL", "BOTH", 10, 10, 0, true, 1, "maker", false, 0),
		lifecycleStrictLiquidationLine(3, "north", 7, "ABC-PERP", "BOTH", 1, 99, -10, 10, 10, 0),
	}
	tests := []struct {
		name   string
		mutate func([]string) []string
		check  func(*testing.T, *OrderLifecycleAudit)
	}{
		{
			name:   "valid_receipt",
			mutate: func(lines []string) []string { return lines },
			check: func(t *testing.T, result *OrderLifecycleAudit) {
				if result.LiquidationFills != 1 || result.UnlinkedFills != 0 || result.ForcedNotionalMismatches != 0 || result.ForcedReceiptOrderFailures != 0 {
					t.Fatalf("valid forced receipt was rejected: %+v", result)
				}
			},
		},
		{
			name: "notional_mismatch",
			mutate: func(lines []string) []string {
				mutated := append([]string{}, lines...)
				mutated[4] = strings.Replace(mutated[4], `"filled_notional":1000`, `"filled_notional":999`, 1)
				return mutated
			},
			check: func(t *testing.T, result *OrderLifecycleAudit) {
				if result.ForcedNotionalMismatches == 0 || result.LiquidationFills != 0 {
					t.Fatalf("notional mutation was not rejected: %+v", result)
				}
			},
		},
		{
			name: "receipt_before_fill",
			mutate: func(lines []string) []string {
				mutated := append([]string{}, lines...)
				mutated[2], mutated[4] = mutated[4], mutated[2]
				return mutated
			},
			check: func(t *testing.T, result *OrderLifecycleAudit) {
				if result.ForcedReceiptOrderFailures != 1 || result.LiquidationFills != 0 {
					t.Fatalf("receipt ordering mutation was not rejected: %+v", result)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run := openStrictLifecycleRun(t, test.mutate(validLines))
			result, err := run.MeasureOrderLifecycle()
			if err != nil {
				t.Fatalf("measure strict forced lifecycle: %v", err)
			}
			test.check(t, result)
		})
	}
}

func TestOrderLifecycleLegacyDoesNotRescueExplicitOrdinaryMarker(t *testing.T) {
	const instant = int64(1_000_000_000)
	dir := writeRun(t, Report{}, map[string][]string{
		"north/derivatives/ABC-PERP.jsonl": {
			lifecycleExplicitOrdinaryFillWithSymbolLine(instant, "north", 7, 99, "ABC-PERP", 10, 10, 0, true),
			lifecycleLiquidationLine(instant, "north", 7, "ABC-PERP"),
		},
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open legacy run: %v", err)
	}
	result, err := run.MeasureOrderLifecycle()
	if err != nil {
		t.Fatalf("measure legacy run: %v", err)
	}
	if result.LiquidationFills != 0 || result.UnlinkedFills != 1 {
		t.Fatalf("explicit ordinary marker received legacy rescue: %+v", result)
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
