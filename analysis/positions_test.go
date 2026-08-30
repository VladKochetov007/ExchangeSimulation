package analysis

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPositionReconstructionRetainsZeroMarkAsPresent(t *testing.T) {
	dir := writeRun(t, Report{}, map[string][]string{
		"north/derivatives.jsonl": {
			`{"sim_ts":1,"client_id":1,"event":"position_update","data":{"venue_id":"north","payload":{"timestamp":1,"client_id":1,"symbol":"OIL-FUT","new_size":1,"new_entry_price":20}}}`,
			`{"sim_ts":1,"client_id":2,"event":"position_update","data":{"venue_id":"north","payload":{"timestamp":1,"client_id":2,"symbol":"OIL-FUT","new_size":-1,"new_entry_price":20}}}`,
			`{"sim_ts":2,"client_id":0,"event":"mark_price_update","data":{"venue_id":"north","payload":{"timestamp":2,"symbol":"OIL-FUT","mark_price":0}}}`,
		},
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	reconstruction, err := run.MeasurePositions(PositionOptions{BasePrecision: 1})
	if err != nil {
		t.Fatalf("measure positions: %v", err)
	}
	if len(reconstruction.Contracts) != 1 {
		t.Fatalf("contracts = %#v, want one OIL-FUT contract", reconstruction.Contracts)
	}
	contract := reconstruction.Contracts[0]
	if !contract.MarkAvailable || contract.MarkPrice != 0 {
		t.Fatalf("zero mark contract = %#v, want MarkAvailable at zero", contract)
	}
	if contract.OpenValue != 0 || reconstruction.UnrepresentableOpenValues != 0 {
		t.Fatalf("zero-mark reconstructed PnL = open=%d unrepresentable=%d, want 0/0", contract.OpenValue, reconstruction.UnrepresentableOpenValues)
	}
}

func TestPositionReconstructionUsesExactReplayAndKeepsDisplayGapDiagnostic(t *testing.T) {
	const precision = int64(1)
	lines := []string{
		`{"sim_ts":1,"client_id":0,"event":"instrument_listed","data":{"venue_id":"north","payload":{"action":"listed","symbol":"ABC-PERP","instrument_type":"PERPETUAL","base_precision":1,"timestamp":1}}}`,
		`{"sim_ts":1,"client_id":1,"event":"position_update","data":{"venue_id":"north","symbol":"ABC-PERP","payload":{"timestamp":1,"client_id":1,"symbol":"ABC-PERP","position_side":"BOTH","base_precision":1,"old_size":0,"old_entry_price":0,"new_size":3,"new_entry_price":100,"trade_qty":3,"trade_price":100,"trade_side":"BUY","reason":"trade"}}}`,
		`{"sim_ts":2,"client_id":1,"event":"position_update","data":{"venue_id":"north","symbol":"ABC-PERP","payload":{"timestamp":2,"client_id":1,"symbol":"ABC-PERP","position_side":"BOTH","base_precision":1,"old_size":3,"old_entry_price":100,"new_size":5,"new_entry_price":100,"trade_qty":2,"trade_price":101,"trade_side":"BUY","reason":"trade"}}}`,
		`{"sim_ts":3,"client_id":1,"event":"position_update","data":{"venue_id":"north","symbol":"ABC-PERP","payload":{"timestamp":3,"client_id":1,"symbol":"ABC-PERP","position_side":"BOTH","base_precision":1,"old_size":5,"old_entry_price":100,"new_size":3,"new_entry_price":100,"trade_qty":2,"trade_price":102,"trade_side":"SELL","reason":"trade"}}}`,
		`{"sim_ts":3,"client_id":1,"event":"realized_pnl","data":{"venue_id":"north","payload":{"symbol":"ABC-PERP","payload":{"timestamp":3,"client_id":1,"symbol":"ABC-PERP","closed_qty":2,"entry_price":100,"exit_price":102,"pnl":3,"side":"SELL"}}}}`,
		// Option producer records are audited from option fills and exercise
		// payouts, not from the linear position replay.
		`{"sim_ts":3,"client_id":1,"event":"realized_pnl","data":{"venue_id":"north","payload":{"symbol":"ABC-1-C","payload":{"timestamp":3,"client_id":1,"symbol":"ABC-1-C","closed_qty":1,"entry_price":100,"exit_price":102,"pnl":2,"side":"SELL"}}}}`,
		`{"sim_ts":4,"client_id":0,"event":"mark_price_update","data":{"venue_id":"north","symbol":"ABC-PERP","payload":{"timestamp":4,"symbol":"ABC-PERP","mark_price":103}}}`,
	}
	terminalMark := int64(103)
	report := Report{TerminalAccounts: []AccountRow{{VenueID: "north", ClientID: 1, Account: Account{Timestamp: 4, Positions: []Position{{Symbol: "ABC-PERP", PositionSide: json.RawMessage(`0`), Size: 3, EntryPrice: 100, MarkPrice: &terminalMark, UnrealizedPnL: 8}}}}}}
	dir := writeRun(t, report, map[string][]string{"north/derivatives.jsonl": lines})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasurePositions(PositionOptions{BasePrecision: precision, RequireExactReplay: true})
	if err != nil {
		t.Fatalf("measure positions: %v", err)
	}
	if result.ExactReplayChecks != 3 || result.ExactReplayFailures != 0 {
		t.Fatalf("exact replay status = (%d, %d), want (3, 0)", result.ExactReplayChecks, result.ExactReplayFailures)
	}
	if result.RealizedPnLChecks != 1 || result.RealizedPnLFailures != 0 {
		t.Fatalf("realized PnL binding = (%d, %d), want (1, 0)", result.RealizedPnLChecks, result.RealizedPnLFailures)
	}
	if result.OpenLinearValue != 8 || result.ReportedLinearValue != 8 || result.Disagreement != 0 {
		t.Fatalf("exact position values = open %d reported %d disagreement %d, want 8/8/0", result.OpenLinearValue, result.ReportedLinearValue, result.Disagreement)
	}
	if result.DisplayLinearValue != 9 || result.DisplayFormulaGap != -1 {
		t.Fatalf("display diagnostic = (%d, %d), want (9, -1)", result.DisplayLinearValue, result.DisplayFormulaGap)
	}
}

func TestStrictPositionReconstructionRejectsCompensatingTerminalPnLErrors(t *testing.T) {
	lines := []string{
		`{"sim_ts":1,"client_id":0,"event":"instrument_listed","data":{"venue_id":"north","payload":{"action":"listed","symbol":"ABC-PERP","instrument_type":"PERPETUAL","base_precision":1,"timestamp":1}}}`,
		`{"sim_ts":1,"client_id":1,"event":"position_update","data":{"venue_id":"north","symbol":"ABC-PERP","payload":{"timestamp":1,"client_id":1,"symbol":"ABC-PERP","position_side":"BOTH","base_precision":1,"old_size":0,"old_entry_price":0,"new_size":1,"new_entry_price":100,"trade_qty":1,"trade_price":100,"trade_side":"BUY","reason":"trade"}}}`,
		`{"sim_ts":1,"client_id":2,"event":"position_update","data":{"venue_id":"north","symbol":"ABC-PERP","payload":{"timestamp":1,"client_id":2,"symbol":"ABC-PERP","position_side":"BOTH","base_precision":1,"old_size":0,"old_entry_price":0,"new_size":-1,"new_entry_price":100,"trade_qty":1,"trade_price":100,"trade_side":"SELL","reason":"trade"}}}`,
		`{"sim_ts":2,"client_id":0,"event":"mark_price_update","data":{"venue_id":"north","symbol":"ABC-PERP","payload":{"timestamp":2,"symbol":"ABC-PERP","mark_price":105}}}`,
	}
	terminalMark := int64(105)
	report := Report{TerminalAccounts: []AccountRow{
		{VenueID: "north", ClientID: 1, Account: Account{Timestamp: 2, Positions: []Position{{Symbol: "ABC-PERP", PositionSide: json.RawMessage(`0`), Size: 1, EntryPrice: 100, MarkPrice: &terminalMark, UnrealizedPnL: 4}}}},
		{VenueID: "north", ClientID: 2, Account: Account{Timestamp: 2, Positions: []Position{{Symbol: "ABC-PERP", PositionSide: json.RawMessage(`0`), Size: -1, EntryPrice: 100, MarkPrice: &terminalMark, UnrealizedPnL: -4}}}},
	}}
	run, err := Open(writeRun(t, report, map[string][]string{"north/derivatives.jsonl": lines}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasurePositions(PositionOptions{BasePrecision: 1, RequireExactReplay: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.TerminalPositionMismatches != 2 {
		t.Fatalf("compensating terminal errors = %+v, want two per-holder mismatches", result)
	}
}

func TestStrictPositionReconstructionRejectsMissingZeroTerminalPnL(t *testing.T) {
	lines := []string{
		`{"sim_ts":1,"client_id":0,"event":"instrument_listed","data":{"venue_id":"north","payload":{"action":"listed","symbol":"ABC-PERP","instrument_type":"PERPETUAL","base_precision":1,"timestamp":1}}}`,
		`{"sim_ts":1,"client_id":1,"event":"position_update","data":{"venue_id":"north","symbol":"ABC-PERP","payload":{"timestamp":1,"client_id":1,"symbol":"ABC-PERP","position_side":"BOTH","base_precision":1,"old_size":0,"old_entry_price":0,"new_size":1,"new_entry_price":100,"trade_qty":1,"trade_price":100,"trade_side":"BUY","reason":"trade"}}}`,
		`{"sim_ts":2,"client_id":0,"event":"mark_price_update","data":{"venue_id":"north","symbol":"ABC-PERP","payload":{"timestamp":2,"symbol":"ABC-PERP","mark_price":100}}}`,
	}
	terminalMark := int64(100)
	report := Report{TerminalAccounts: []AccountRow{{VenueID: "north", ClientID: 1, Account: Account{Timestamp: 2, Positions: []Position{{Symbol: "ABC-PERP", PositionSide: json.RawMessage(`0`), Size: 1, EntryPrice: 100, MarkPrice: &terminalMark, UnrealizedPnL: 0}}}}}}
	dir := writeRun(t, report, map[string][]string{"north/derivatives.jsonl": lines})
	reportPath := filepath.Join(dir, "greeks.json")
	raw, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	raw = bytes.Replace(raw, []byte(`,"unrealized_pnl":0`), nil, 1)
	if err := os.WriteFile(reportPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	run, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasurePositions(PositionOptions{BasePrecision: 1, RequireExactReplay: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.TerminalPositionMismatches != 1 {
		t.Fatalf("missing zero PnL = %+v, want one terminal mismatch", result)
	}
}

func TestStrictPositionReconstructionRejectsPostTerminalMutation(t *testing.T) {
	lines := []string{
		`{"sim_ts":1,"client_id":0,"event":"instrument_listed","data":{"venue_id":"north","payload":{"action":"listed","symbol":"ABC-PERP","instrument_type":"PERPETUAL","base_precision":1,"timestamp":1}}}`,
		`{"sim_ts":1,"client_id":1,"event":"position_update","data":{"venue_id":"north","symbol":"ABC-PERP","payload":{"timestamp":1,"client_id":1,"symbol":"ABC-PERP","position_side":"BOTH","base_precision":1,"old_size":0,"old_entry_price":0,"new_size":1,"new_entry_price":100,"trade_qty":1,"trade_price":100,"trade_side":"BUY","reason":"trade"}}}`,
		`{"sim_ts":2,"client_id":0,"event":"mark_price_update","data":{"venue_id":"north","symbol":"ABC-PERP","payload":{"timestamp":2,"symbol":"ABC-PERP","mark_price":101}}}`,
		`{"sim_ts":3,"client_id":1,"event":"position_update","data":{"venue_id":"north","symbol":"ABC-PERP","payload":{"timestamp":3,"client_id":1,"symbol":"ABC-PERP","position_side":"BOTH","base_precision":1,"old_size":1,"old_entry_price":100,"new_size":2,"new_entry_price":100,"trade_qty":1,"trade_price":100,"trade_side":"BUY","reason":"trade"}}}`,
	}
	terminalMark := int64(101)
	report := Report{TerminalAccounts: []AccountRow{{VenueID: "north", ClientID: 1, Account: Account{Timestamp: 2, Positions: []Position{{Symbol: "ABC-PERP", PositionSide: json.RawMessage(`0`), Size: 1, EntryPrice: 100, MarkPrice: &terminalMark, UnrealizedPnL: 1}}}}}}
	run, err := Open(writeRun(t, report, map[string][]string{"north/derivatives.jsonl": lines}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasurePositions(PositionOptions{BasePrecision: 1, RequireExactReplay: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.PostTerminalPositionUpdates != 1 || result.ExactReplayFailures != 1 {
		t.Fatalf("post-terminal mutation = %+v, want one fail-closed mutation", result)
	}
}

func TestStrictPositionReconstructionRejectsUseBeforeListing(t *testing.T) {
	lines := []string{
		`{"sim_ts":1,"client_id":1,"event":"position_update","data":{"venue_id":"north","symbol":"ABC-PERP","payload":{"timestamp":1,"client_id":1,"symbol":"ABC-PERP","position_side":"BOTH","base_precision":1,"old_size":0,"old_entry_price":0,"new_size":1,"new_entry_price":100,"trade_qty":1,"trade_price":100,"trade_side":"BUY","reason":"trade"}}}`,
		`{"sim_ts":2,"client_id":0,"event":"instrument_listed","data":{"venue_id":"north","payload":{"action":"listed","symbol":"ABC-PERP","instrument_type":"PERPETUAL","base_precision":1,"timestamp":2}}}`,
	}
	run, err := Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasurePositions(PositionOptions{BasePrecision: 1, RequireExactReplay: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExactReplayFailures == 0 || result.EvidenceFailures == 0 {
		t.Fatalf("position use before listing was accepted: %+v", result)
	}
}

func TestPositionReconstructionRejectsUnpinnedPrecision(t *testing.T) {
	dir := writeRun(t, Report{}, map[string][]string{})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := run.MeasurePositions(PositionOptions{}); err == nil {
		t.Fatal("position reconstruction accepted nonpositive precision")
	}
}

func TestStrictPositionReconstructionRejectsMissingTerminalMark(t *testing.T) {
	lines := []string{
		`{"sim_ts":1,"client_id":0,"event":"instrument_listed","data":{"venue_id":"north","payload":{"action":"listed","symbol":"ABC-PERP","instrument_type":"PERPETUAL","base_precision":1,"timestamp":1}}}`,
		`{"sim_ts":1,"client_id":1,"event":"position_update","data":{"venue_id":"north","symbol":"ABC-PERP","payload":{"timestamp":1,"client_id":1,"symbol":"ABC-PERP","position_side":"BOTH","base_precision":1,"old_size":0,"old_entry_price":0,"new_size":1,"new_entry_price":100,"trade_qty":1,"trade_price":100,"trade_side":"BUY","reason":"trade"}}}`,
	}
	report := Report{TerminalAccounts: []AccountRow{{VenueID: "north", ClientID: 1, Account: Account{Timestamp: 2, Positions: []Position{{Symbol: "ABC-PERP", PositionSide: json.RawMessage(`0`), Size: 1, EntryPrice: 100}}}}}}
	dir := writeRun(t, report, map[string][]string{"north/derivatives.jsonl": lines})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasurePositions(PositionOptions{BasePrecision: 1, RequireExactReplay: true})
	if err != nil {
		t.Fatalf("measure positions: %v", err)
	}
	if result.MissingMarks != 1 || result.MarkIdentityFailures != 0 {
		t.Fatalf("missing terminal mark status = %+v, want one missing mark and no identity failure", result)
	}
}

func TestStrictPositionReconstructionRejectsMissingTerminalPosition(t *testing.T) {
	lines := []string{
		`{"sim_ts":1,"client_id":0,"event":"instrument_listed","data":{"venue_id":"north","payload":{"action":"listed","symbol":"ABC-PERP","instrument_type":"PERPETUAL","base_precision":1,"timestamp":1}}}`,
		`{"sim_ts":1,"client_id":1,"event":"position_update","data":{"venue_id":"north","symbol":"ABC-PERP","payload":{"timestamp":1,"client_id":1,"symbol":"ABC-PERP","position_side":"BOTH","base_precision":1,"old_size":0,"old_entry_price":0,"new_size":1,"new_entry_price":100,"trade_qty":1,"trade_price":100,"trade_side":"BUY","reason":"trade"}}}`,
		`{"sim_ts":2,"client_id":0,"event":"mark_price_update","data":{"venue_id":"north","symbol":"ABC-PERP","payload":{"timestamp":2,"symbol":"ABC-PERP","mark_price":101}}}`,
	}
	report := Report{TerminalAccounts: []AccountRow{{VenueID: "north", ClientID: 1, Account: Account{Timestamp: 2}}}}
	dir := writeRun(t, report, map[string][]string{"north/derivatives.jsonl": lines})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasurePositions(PositionOptions{BasePrecision: 1, RequireExactReplay: true})
	if err != nil {
		t.Fatalf("measure positions: %v", err)
	}
	if result.MissingTerminalPositions != 1 || result.UnexpectedTerminalPositions != 0 {
		t.Fatalf("missing terminal position status = %+v, want one missing position", result)
	}
}
