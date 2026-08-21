package analysis

import (
	"fmt"
	"testing"
)

func roleFill(ts int64, clientID uint64, venue, symbol, liquidityRole, side string, qty, fee int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":%d,"event":"OrderFill","data":{"venue_id":%q,"payload":{"symbol":%q,"role":%q,"side":%q,"qty":%d,"fee_amount":%d,"fee_asset":"USD"}}}`,
		ts, clientID, venue, symbol, liquidityRole, side, qty, fee)
}

// A class can be present, funded and busy while doing something other than its
// name: a maker that crosses the spread is a taker. The audit has to separate
// the two, because the configuration cannot.
func TestRoleAuditSeparatesLiquidityProvidedFromLiquidityTaken(t *testing.T) {
	report := Report{TerminalAccounts: []AccountRow{
		{VenueID: "north", ClientID: 1, Role: "spot_maker_1"},
		{VenueID: "north", ClientID: 2, Role: "noise_flow_1"},
	}}
	lines := []string{
		roleFill(1, 1, "north", "ABC/USD", "maker", "BUY", 100, 0),
		roleFill(2, 1, "north", "ABC/USD", "maker", "SELL", 100, 0),
		roleFill(3, 1, "north", "ABC/USD", "taker", "SELL", 300, 7),
		roleFill(4, 2, "north", "ABC/USD", "taker", "BUY", 50, 2),
	}
	dir := writeRun(t, report, map[string][]string{"north/spot/ABC-USD.jsonl": lines})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureRoles(RoleAuditOptions{})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	byRole := map[string]RoleBehaviour{}
	for _, row := range result.Roles {
		byRole[row.Role] = row
	}
	maker := byRole["spot_maker"]
	if maker.MakerFills != 2 || maker.TakerFills != 1 {
		t.Errorf("maker fills = %d/%d, want 2 provided and 1 taken", maker.MakerFills, maker.TakerFills)
	}
	if maker.TakerShare < 0.32 || maker.TakerShare > 0.34 {
		t.Errorf("taker share = %.3f, want a third", maker.TakerShare)
	}
	// It bought 100, sold 100 and sold 300: net short 300 on 500 gross.
	if maker.SignedQty != -300 || maker.GrossQty != 500 {
		t.Errorf("signed/gross = %d/%d, want -300 of 500", maker.SignedQty, maker.GrossQty)
	}
	if maker.SignedShare < 0.59 || maker.SignedShare > 0.61 {
		t.Errorf("signed share = %.3f, want 0.6", maker.SignedShare)
	}
	if maker.FeesPaid != 7 {
		t.Errorf("fees = %d, want 7", maker.FeesPaid)
	}
}

// A desk whose job spans several books and whose activity sits in one of them
// is not doing the job, so the concentration has to be visible.
func TestRoleAuditReportsBookConcentrationAndRejections(t *testing.T) {
	report := Report{TerminalAccounts: []AccountRow{
		{VenueID: "north", ClientID: 1, Role: "triangle_arb_1"},
	}}
	lines := []string{
		roleFill(1, 1, "north", "ABC/USD", "taker", "BUY", 100, 1),
		roleFill(2, 1, "north", "ABC/USD", "taker", "BUY", 100, 1),
		roleFill(3, 1, "north", "CDF/USD", "taker", "SELL", 100, 1),
		`{"sim_ts":4,"client_id":1,"event":"OrderRejected","data":{"venue_id":"north","payload":{"symbol":"ABC/CDF","reason":"INSUFFICIENT_BALANCE"}}}`,
	}
	dir := writeRun(t, report, map[string][]string{"north/spot/ABC-USD.jsonl": lines})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureRoles(RoleAuditOptions{})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	arb := result.Roles[0]
	if arb.Symbols != 2 {
		t.Errorf("symbols = %d, want 2", arb.Symbols)
	}
	if arb.TopSymbolShare < 0.66 || arb.TopSymbolShare > 0.67 {
		t.Errorf("top book share = %.3f, want two thirds", arb.TopSymbolShare)
	}
	if arb.Rejected != 1 {
		t.Errorf("rejections = %d, want 1: a desk that cannot afford its own strategy", arb.Rejected)
	}
}
