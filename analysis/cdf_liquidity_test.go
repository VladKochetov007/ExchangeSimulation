package analysis

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMeasureCDFLiquidityReconstructsBoundedSupplier(t *testing.T) {
	run := writeCDFLiquidityFixture(t, true, false)
	audit, err := run.MeasureCDFLiquidity()
	if err != nil {
		t.Fatalf("MeasureCDFLiquidity: %v", err)
	}
	if !audit.Valid {
		t.Fatalf("audit invalid: %+v", audit.Checks)
	}
	if audit.SupplierCount != 1 || audit.DecisionCount != 3 || audit.FillCount != 1 || audit.AcceptedQuoteCount != 2 || audit.CompletedQuoteCount != 2 {
		t.Fatalf("audit counts = %+v", audit)
	}
	if audit.SupplierVolumeQty != 5 || audit.TotalTradeVolumeQty != 25 || audit.SupplierVolumeShare != .2 {
		t.Fatalf("audit volume = %+v", audit)
	}
	if audit.Suppliers[0].PnL != 1 || audit.Suppliers[0].TerminalPosition != 5 || audit.Suppliers[0].MaxObservedTouchShare != .5 {
		t.Fatalf("supplier diagnostics = %+v", audit.Suppliers[0])
	}
}

func TestMeasureCDFLiquidityFailsClosedOnMissingFillField(t *testing.T) {
	run := writeCDFLiquidityFixture(t, true, true)
	audit, err := run.MeasureCDFLiquidity()
	if err != nil {
		t.Fatalf("MeasureCDFLiquidity: %v", err)
	}
	if audit.Valid || !hasCDFCheck(audit.Checks, "malformed supplier fill") {
		t.Fatalf("audit = %+v, want malformed fill rejection", audit)
	}
}

func TestCompareCDFLiquidityRunsRequiresSeparateRoster(t *testing.T) {
	treatment := writeCDFLiquidityFixture(t, true, false)
	control := writeCDFLiquidityFixture(t, false, false)
	comparison, err := CompareCDFLiquidityRuns(treatment, control)
	if err != nil {
		t.Fatalf("CompareCDFLiquidityRuns: %v", err)
	}
	if !comparison.Valid || comparison.Control.SupplierCount != 0 || comparison.Treatment.SupplierCount != 1 {
		t.Fatalf("comparison = %+v", comparison)
	}
}

func TestCompareCDFLiquidityRunsRejectsInactiveTreatment(t *testing.T) {
	treatment := writeCDFLiquidityFixture(t, true, false)
	generalPath := filepath.Join(treatment.Dir, "venues", "north", "general.jsonl")
	if err := os.WriteFile(generalPath, []byte(cdfFixtureLine(1, 2, "elastic_liquidity_supplier_decision", `{"role":"cdf_elastic_supplier_1","client_id":2,"symbol":"CDF/USD","decision_time":1,"observation_time":1,"observation_age":0,"observation_sequence":1,"best_bid":99,"best_bid_qty":10,"best_ask":101,"best_ask_qty":10,"mark_price":100,"reference_price":100,"position":0,"target_position":5,"inventory_limit":10,"action":"submit","reason":"inventory_target_gap","side":"BUY","quote_price":99,"quote_qty":5,"quote_request_id":1,"quote_submitted_at":1}`)+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	bookPath := filepath.Join(treatment.Dir, "venues", "north", "spot", "CDF-USD.jsonl")
	book := cdfFixtureLine(1, 0, "BookSnapshot", `{"bids":[{"price":99,"visible_qty":10,"hidden_qty":0}],"asks":[{"price":101,"visible_qty":10,"hidden_qty":0}]}`) + "\n"
	book += cdfFixtureLine(2, 0, "Trade", `{"trade_id":1,"price":99,"qty":5,"side":"SELL","taker_order_id":9}`) + "\n"
	if err := os.WriteFile(bookPath, []byte(book), 0644); err != nil {
		t.Fatal(err)
	}
	audit, err := treatment.MeasureCDFLiquidity()
	if err != nil {
		t.Fatal(err)
	}
	if audit.Valid || !hasCDFCheck(audit.Checks, "supplier activation contract is incomplete") {
		t.Fatalf("inactive treatment audit = %+v, want fail-closed activation rejection", audit)
	}
}

func TestMeasureCDFLiquidityRejectsConfiguredQuoteLimitMutation(t *testing.T) {
	run := writeCDFLiquidityFixture(t, true, false)
	manifestPath := filepath.Join(run.Dir, "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	raw = []byte(strings.Replace(string(raw), `"max_quote_qty":5`, `"max_quote_qty":4`, 1))
	if err := os.WriteFile(manifestPath, raw, 0644); err != nil {
		t.Fatal(err)
	}
	audit, err := run.MeasureCDFLiquidity()
	if err != nil {
		t.Fatal(err)
	}
	if audit.Valid || !hasCDFCheck(audit.Checks, "submitted quote exceeds registered maximum quantity") {
		t.Fatalf("quote-limit mutation audit = %+v, want fail-closed limit rejection", audit)
	}
}

func writeCDFLiquidityFixture(t *testing.T, supplier, malformedFill bool) *Run {
	t.Helper()
	dir := t.TempDir()
	initialAccounts := `[{"venue_id":"north","client_id":1,"role":"elastic_supplier_1","account":{"equity":10}}]`
	terminalAccounts := `[{"venue_id":"north","client_id":1,"role":"elastic_supplier_1","account":{"equity":10}}]`
	general := ""
	if supplier {
		initialAccounts = `[{"venue_id":"north","client_id":1,"role":"elastic_supplier_1","account":{"equity":10}},{"venue_id":"north","client_id":2,"role":"cdf_elastic_supplier_1","marks":{"CDF":100},"account":{"equity":1000,"spot_balances":[{"asset":"CDF","net_asset":100,"borrowed":0},{"asset":"USD","net_asset":1000,"borrowed":0}]}}]`
		terminalAccounts = `[{"venue_id":"north","client_id":1,"role":"elastic_supplier_1","account":{"equity":10}},{"venue_id":"north","client_id":2,"role":"cdf_elastic_supplier_1","marks":{"CDF":100},"account":{"equity":1001,"spot_balances":[{"asset":"CDF","net_asset":105,"borrowed":0},{"asset":"USD","net_asset":500,"borrowed":0}]}}]`
		general = cdfFixtureLine(1, 2, "elastic_liquidity_supplier_decision", `{"role":"cdf_elastic_supplier_1","client_id":2,"symbol":"CDF/USD","decision_time":1,"observation_time":1,"observation_age":0,"observation_sequence":1,"best_bid":99,"best_bid_qty":10,"best_ask":101,"best_ask_qty":10,"mark_price":100,"reference_price":100,"position":0,"target_position":5,"inventory_limit":10,"action":"submit","reason":"inventory_target_gap","side":"BUY","quote_price":99,"quote_qty":5,"quote_request_id":1,"quote_submitted_at":1}`) + "\n"
		fillFields := `"role":"cdf_elastic_supplier_1","client_id":2,"symbol":"CDF/USD","order_id":7,"trade_id":1,"timestamp":3,"side":"BUY","price":99,"qty":5,"fee_amount":0,"fee_asset":"","is_full":true,"position_before":0,"position_after":5`
		if malformedFill {
			fillFields = strings.Replace(fillFields, `,"is_full":true`, "", 1)
		}
		general += cdfFixtureLine(3, 2, "elastic_liquidity_supplier_fill", "{"+fillFields+"}") + "\n"
		general += cdfFixtureLine(4, 2, "elastic_liquidity_supplier_decision", `{"role":"cdf_elastic_supplier_1","client_id":2,"symbol":"CDF/USD","decision_time":4,"observation_time":1,"observation_age":3,"observation_sequence":1,"best_bid":99,"best_bid_qty":10,"best_ask":101,"best_ask_qty":10,"mark_price":100,"reference_price":100,"position":5,"target_position":9,"inventory_limit":10,"action":"submit","reason":"inventory_target_gap","side":"BUY","quote_price":99,"quote_qty":4,"quote_request_id":2,"quote_submitted_at":4}`) + "\n"
		general += cdfFixtureLine(6, 2, "elastic_liquidity_supplier_decision", `{"role":"cdf_elastic_supplier_1","client_id":2,"symbol":"CDF/USD","decision_time":6,"observation_time":1,"observation_age":5,"best_bid":99,"best_bid_qty":10,"best_ask":101,"best_ask_qty":10,"mark_price":100,"reference_price":100,"position":5,"target_position":9,"inventory_limit":10,"action":"cancel","reason":"reprice_for_inventory_or_touch","quote_order_id":8}`) + "\n"
	}
	greeks := fmt.Sprintf(`{"initial_accounts":%s,"terminal_accounts":%s}`, initialAccounts, terminalAccounts)
	if err := os.WriteFile(filepath.Join(dir, "greeks.json"), []byte(greeks), 0644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"config":{"elastic_supplier_count":1,"record_market_data_receipts":false,"elastic_liquidity_suppliers":[]}}`
	if supplier {
		manifest = `{"config":{"elastic_supplier_count":1,"record_market_data_receipts":false,"elastic_liquidity_suppliers":[{"role":"cdf_elastic_supplier_1","symbol":"CDF/USD","base_asset":"CDF","quote_asset":"USD","base_precision":1,"initial_base_balance":100,"initial_quote_balance":1000,"max_position":10,"max_quote_qty":5,"max_observation_age":10}]}}`
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	venueDir := filepath.Join(dir, "venues", "north", "spot")
	if err := os.MkdirAll(venueDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "venues", "north", "general.jsonl"), []byte(general), 0644); err != nil {
		t.Fatal(err)
	}
	book := cdfFixtureLine(1, 0, "BookSnapshot", `{"bids":[{"price":99,"visible_qty":10,"hidden_qty":0}],"asks":[{"price":101,"visible_qty":10,"hidden_qty":0}]}`) + "\n"
	book += cdfFixtureLine(2, 2, "OrderAccepted", `{"order_id":7,"client_id":2,"request_id":1,"side":"BUY","type":"LIMIT","time_in_force":"GTC","post_only":true,"price":99,"qty":5}`) + "\n"
	book += cdfFixtureLine(3, 0, "Trade", `{"trade_id":1,"price":99,"qty":5,"side":"SELL","taker_order_id":9}`) + "\n"
	book += cdfFixtureLine(3, 2, "OrderFill", `{"order_id":7,"trade_id":1,"side":"BUY","price":99,"qty":5,"filled_qty":5,"remaining_qty":0,"is_full":true}`) + "\n"
	book += cdfFixtureLine(4, 0, "Trade", `{"trade_id":2,"price":99,"qty":20,"side":"SELL","taker_order_id":10}`) + "\n"
	book += cdfFixtureLine(5, 2, "OrderAccepted", `{"order_id":8,"client_id":2,"request_id":2,"side":"BUY","type":"LIMIT","time_in_force":"GTC","post_only":true,"price":99,"qty":4}`) + "\n"
	book += cdfFixtureLine(6, 2, "OrderCancelled", `{"order_id":8,"request_id":2,"remaining_qty":4}`) + "\n"
	if err := os.WriteFile(filepath.Join(venueDir, "CDF-USD.jsonl"), []byte(book), 0644); err != nil {
		t.Fatal(err)
	}
	run, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func cdfFixtureLine(sequence uint64, clientID uint64, event, payload string) string {
	return fmt.Sprintf(`{"client_id":%d,"data":{"venue_id":"north","sequence":%d,"payload":%s},"event":"%s","sim_ts":%d}`, clientID, sequence, payload, event, sequence)
}

func hasCDFCheck(checks []CDFLiquidityCheck, prefix string) bool {
	for _, check := range checks {
		if strings.HasPrefix(check.Failure, prefix) {
			return true
		}
	}
	return false
}
