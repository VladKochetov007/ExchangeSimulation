package analysis

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func writeRun(t *testing.T, report Report, books map[string][]string) string {
	t.Helper()
	dir := t.TempDir()
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "greeks.json"), raw, 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}
	for rel, lines := range books {
		path := filepath.Join(dir, "venues", rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		content := ""
		for _, line := range lines {
			content += line + "\n"
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write book: %v", err)
		}
	}
	return dir
}

func TestRoleGroupCollapsesNumberedParticipants(t *testing.T) {
	for input, want := range map[string]string{
		"spot_maker_3":  "spot_maker",
		"spot_maker":    "spot_maker",
		"carry_arb_12":  "carry_arb",
		"round_trip_1":  "round_trip",
		"abc_cdf_maker": "abc_cdf_maker",
		"trailing_":     "trailing_",
		"_1":            "_1",
		"metaorder_x1":  "metaorder_x1",
	} {
		if got := RoleGroup(input); got != want {
			t.Errorf("RoleGroup(%q) = %q, want %q", input, got, want)
		}
	}
}

// A derivative record nests its fields one level deeper than a spot record and
// carries the symbol beside them. Reading only the outer level yields zero for
// every derivative field, which is how an entire class of fills once went
// uncounted.
func TestScanUnwrapsTheDerivativeNesting(t *testing.T) {
	dir := writeRun(t, Report{
		TerminalAccounts: []AccountRow{{VenueID: "north", ClientID: 7, Role: "spot_maker_1"}},
	}, map[string][]string{
		"north/spot/ABC-USD.jsonl": {
			`{"sim_ts":1,"client_id":7,"data":{"venue_id":"north","payload":{"filled_qty":11,"role":"maker"}},"event":"OrderFill"}`,
		},
		"north/derivatives.jsonl": {
			`{"sim_ts":2,"client_id":7,"data":{"venue_id":"north","payload":{"symbol":"ABC-PERP","payload":{"filled_qty":22,"role":"taker"}}},"event":"OrderFill"}`,
		},
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	var mu sync.Mutex
	got := map[string]int64{}
	if err := run.Scan(ScanOptions{Events: []string{"OrderFill"}}, func(event Event) {
		var payload orderPayload
		if err := event.Decode(&payload); err != nil {
			t.Errorf("decode: %v", err)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		got[payload.Role] = payload.FilledQty
		if event.Symbol != "" {
			got["symbol:"+event.Symbol] = payload.FilledQty
		}
	}); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if got["maker"] != 11 {
		t.Errorf("spot fill qty = %d, want 11", got["maker"])
	}
	if got["taker"] != 22 {
		t.Errorf("derivative fill qty = %d, want 22 (outer payload read instead of inner)", got["taker"])
	}
	if got["symbol:ABC-PERP"] != 22 {
		t.Errorf("derivative symbol not carried through the unwrap")
	}
}

func TestRoleTableCountsExpiriesSeparatelyFromCancels(t *testing.T) {
	dir := writeRun(t, Report{
		TerminalAccounts: []AccountRow{{VenueID: "north", ClientID: 1, Role: "carry_arb_1"}},
	}, map[string][]string{
		"north/spot/ABC-USD.jsonl": {
			`{"sim_ts":1,"client_id":1,"data":{"venue_id":"north","payload":{}},"event":"OrderAccepted"}`,
			`{"sim_ts":2,"client_id":1,"data":{"venue_id":"north","payload":{}},"event":"OrderAccepted"}`,
			`{"sim_ts":3,"client_id":1,"data":{"venue_id":"north","payload":{"filled_qty":5}},"event":"OrderFill"}`,
			`{"sim_ts":4,"client_id":1,"data":{"venue_id":"north","payload":{"reason":"IOC_EXPIRED"}},"event":"OrderCancelled"}`,
			`{"sim_ts":5,"client_id":1,"data":{"venue_id":"north","payload":{}},"event":"OrderCancelled"}`,
		},
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	table, err := run.RoleTable()
	if err != nil {
		t.Fatalf("RoleTable: %v", err)
	}
	stats := table["carry_arb"]
	if stats == nil {
		t.Fatal("carry_arb missing from the table")
	}
	if stats.Accepted != 2 || stats.Fills != 1 || stats.FilledQty != 5 {
		t.Errorf("counts = %+v", *stats)
	}
	if stats.Cancelled != 2 || stats.IOCExpired != 1 {
		t.Errorf("expiry must be counted separately from ordinary cancels: %+v", *stats)
	}
	if math.Abs(stats.Conversion()-0.5) > 1e-9 {
		t.Errorf("conversion = %f, want 0.5", stats.Conversion())
	}
}

// A parent abandoned at its horizon is recorded a fraction under it, because
// the check runs on the desk's tick. Counting only durations at or above the
// exact horizon misses every stall.
func TestStallsCountParentsJustUnderTheHorizon(t *testing.T) {
	second := int64(1e9)
	dir := writeRun(t, Report{Metaorders: []Metaorder{
		{Side: "BUY", StartTimestamp: 0, EndTimestamp: 899 * second, FilledQty: 0},
		{Side: "SELL", StartTimestamp: 0, EndTimestamp: 900 * second, FilledQty: 0},
		{Side: "BUY", StartTimestamp: 0, EndTimestamp: 3 * second, FilledQty: 10},
	}}, nil)
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	stats := run.Stalls(StallOptions{HorizonSeconds: 900, Desks: 6, RunSeconds: 8 * 3600})
	if stats.Parents != 3 || stats.Filled != 1 || stats.ZeroFill != 2 {
		t.Errorf("parent counts = %+v", stats)
	}
	if stats.Stalled != 2 {
		t.Errorf("stalled = %d, want 2 including the parent recorded at 899s", stats.Stalled)
	}
	if stats.Sides["BUY"] != 1 || stats.Sides["SELL"] != 1 {
		t.Errorf("stall sides = %v", stats.Sides)
	}
	wantFraction := (899.0 + 900.0) / (6 * 8 * 3600)
	if math.Abs(stats.StallFraction()-wantFraction) > 1e-9 {
		t.Errorf("stall fraction = %f, want %f", stats.StallFraction(), wantFraction)
	}
}

func TestTriangularDeviationIsZeroAtParity(t *testing.T) {
	// ABC/USD 50000, CDF/USD 300, so the implied cross is 166.66 ABC per CDF
	// scaled by the cross precision.
	const crossPrecision = 100_000_000
	base, quote := int64(50_000), int64(300)
	cross := int64(float64(base) / float64(quote) * crossPrecision)
	dir := writeRun(t, Report{}, map[string][]string{
		"north/spot/ABC-USD.jsonl": {`{"sim_ts":1000000000,"data":{"venue_id":"north","payload":{"price":50000}},"event":"Trade"}`},
		"north/spot/CDF-USD.jsonl": {`{"sim_ts":1000000000,"data":{"venue_id":"north","payload":{"price":300}},"event":"Trade"}`},
		"north/spot/ABC-CDF.jsonl": {`{"sim_ts":1000000000,"data":{"venue_id":"north","payload":{"price":` + itoa(cross) + `}},"event":"Trade"}`},
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	deviations, err := run.TriangularDeviation(TriangularConfig{
		VenueID: "north", BaseSymbol: "ABC-USD", QuotePair: "CDF-USD", CrossPair: "ABC-CDF",
		CrossPrecision: crossPrecision,
	})
	if err != nil {
		t.Fatalf("TriangularDeviation: %v", err)
	}
	if len(deviations) != 1 {
		t.Fatalf("deviations = %v, want one observation", deviations)
	}
	if math.Abs(deviations[0]) > 1 {
		t.Errorf("deviation at parity = %f bps, want about zero", deviations[0])
	}
}

func TestDescribeReportsOrderStatistics(t *testing.T) {
	got := Describe([]float64{5, 1, 4, 2, 3})
	if got.N != 5 || got.Median != 3 || got.Max != 5 {
		t.Errorf("distribution = %+v", got)
	}
	if math.Abs(got.Mean-3) > 1e-9 {
		t.Errorf("mean = %f, want 3", got.Mean)
	}
	if empty := Describe(nil); empty.N != 0 {
		t.Errorf("empty sample = %+v", empty)
	}
}

func itoa(value int64) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}
