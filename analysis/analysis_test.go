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

func TestMetaorderVWAPAvailabilitySurvivesReportDecode(t *testing.T) {
	price := int64(5_000_000_000)
	dir := writeRun(t, Report{Metaorders: []Metaorder{
		{ID: 1, FilledQty: 1, VWAP: &price},
		{ID: 2, FilledQty: 0},
	}}, nil)
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got := run.Report.Metaorders[0].VWAP; got == nil || *got != price {
		t.Fatalf("filled metaorder VWAP = %v, want %d", got, price)
	}
	if got := run.Report.Metaorders[1].VWAP; got != nil {
		t.Fatalf("unfilled metaorder VWAP = %d, want unavailable", *got)
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

// A random walk has unpredictable returns, no persistence in their magnitudes,
// and near-Gaussian tails. The panel must report that, or it cannot be trusted
// to report the opposite for a real tape.
func TestFactsOnARandomWalkShowNoStructure(t *testing.T) {
	const n = 20000
	price := int64(50_000 * 100_000)
	priceState, signState := uint64(12345), uint64(98765)
	// Two independent streams: drawing the sign from a low bit of the same
	// state that drives the price makes the two perfectly dependent, which is
	// a property of the generator rather than of the market.
	next := func(state *uint64) uint64 {
		*state = *state*6364136223846793005 + 1442695040888963407
		return *state >> 33
	}
	tape := &TradeTape{}
	for i := 0; i < n; i++ {
		// A sum of draws, so the step distribution is bell shaped rather than
		// uniform: a uniform step has excess kurtosis of exactly -1.2, which
		// would fail a near-Gaussian expectation for the right reason.
		step := int64(next(&priceState)%101) + int64(next(&priceState)%101) +
			int64(next(&priceState)%101) - 150
		price += step * 1000
		if price < 1 {
			price = 1
		}
		sign := int8(1)
		if next(&signState)%2 == 0 {
			sign = -1
		}
		tape.Timestamps = append(tape.Timestamps, int64(i)*1e9)
		tape.Prices = append(tape.Prices, price)
		tape.Qtys = append(tape.Qtys, 1)
		tape.Signs = append(tape.Signs, sign)
	}
	facts := tape.Facts(50)
	if facts.Trades != n {
		t.Fatalf("trades = %d, want %d", facts.Trades, n)
	}
	if math.Abs(facts.ReturnACF1) > 0.05 {
		t.Errorf("random-walk return ACF(1) = %.3f, want near zero", facts.ReturnACF1)
	}
	if math.Abs(facts.AbsReturnACF10) > 0.05 {
		t.Errorf("random-walk |return| ACF(10) = %.3f, want near zero", facts.AbsReturnACF10)
	}
	if math.Abs(facts.SignACF10) > 0.05 {
		t.Errorf("coin-flip sign ACF(10) = %.3f, want near zero", facts.SignACF10)
	}
	if math.Abs(facts.ExcessKurtosis) > 0.5 {
		t.Errorf("bell-shaped-step excess kurtosis = %.2f, want near zero", facts.ExcessKurtosis)
	}
}

// Autocorrelation must find structure that is there: an alternating series is
// perfectly anticorrelated at odd lags and correlated at even ones.
func TestAutocorrelationDetectsAlternation(t *testing.T) {
	series := make([]float64, 1000)
	for i := range series {
		if i%2 == 0 {
			series[i] = 1
		} else {
			series[i] = -1
		}
	}
	acf := Autocorrelation(series, 4)
	if acf[0] > -0.9 {
		t.Errorf("ACF(1) = %.3f, want about -1", acf[0])
	}
	if acf[1] < 0.9 {
		t.Errorf("ACF(2) = %.3f, want about +1", acf[1])
	}
	if flat := Autocorrelation([]float64{2, 2, 2, 2}, 2); flat[0] != 0 {
		t.Errorf("constant series ACF = %v, want zeros", flat)
	}
}

// A persistent sign series is what order splitting produces, and it must show
// long-lag correlation rather than dying immediately.
func TestSignAutocorrelationSurvivesAtLongLags(t *testing.T) {
	// Independently signed runs, which is what order splitting produces. A
	// strictly alternating block pattern would instead be periodic, and its
	// autocorrelation would go negative at a lag half a period away.
	tape := &TradeTape{}
	state := uint64(4242)
	for block := 0; block < 400; block++ {
		state = state*6364136223846793005 + 1442695040888963407
		sign := int8(1)
		if (state>>33)%2 == 0 {
			sign = -1
		}
		for i := 0; i < 60; i++ {
			tape.Signs = append(tape.Signs, sign)
		}
	}
	acf := Autocorrelation(tape.SignSeries(), 50)
	if acf[0] < 0.9 {
		t.Errorf("sign ACF(1) = %.3f, want near one for blocked flow", acf[0])
	}
	if acf[49] < 0.1 {
		t.Errorf("sign ACF(50) = %.3f, want still positive for blocked flow", acf[49])
	}
}

func TestHillTailIndexIsLowerForFatterTails(t *testing.T) {
	state := uint64(99)
	uniform := func() float64 {
		state = state*6364136223846793005 + 1442695040888963407
		return float64(state>>11) / float64(1<<53)
	}
	pareto := func(alpha float64) []float64 {
		out := make([]float64, 5000)
		for i := range out {
			u := uniform()
			if u <= 0 {
				u = 1e-9
			}
			out[i] = math.Pow(u, -1/alpha)
		}
		return out
	}
	fat := HillTailIndex(pareto(2), 0.05)
	thin := HillTailIndex(pareto(5), 0.05)
	if !(fat < thin) {
		t.Fatalf("tail index did not order the samples: alpha 2 gave %.2f, alpha 5 gave %.2f", fat, thin)
	}
	if math.Abs(fat-2) > 0.8 {
		t.Errorf("tail index for alpha 2 = %.2f, want about 2", fat)
	}
}

// A Hill estimate is only meaningful where it is stable in the number of tail
// observations used. On a bounded lattice-valued sample the estimator still
// returns a number — it returned 39.5 on a reference tape whose returns lived
// in a 7% wide band — so stability has to be checked before the value is
// reported as a measurement.
func TestHillStabilitySeparatesRealTailsFromNone(t *testing.T) {
	state := uint64(31337)
	uniform := func() float64 {
		state = state*6364136223846793005 + 1442695040888963407
		return float64(state>>11) / float64(1<<53)
	}
	pareto := make([]float64, 40000)
	for i := range pareto {
		u := uniform()
		if u <= 0 {
			u = 1e-9
		}
		pareto[i] = math.Pow(u, -1/3.0)
	}
	median, spread := HillStability(pareto)
	if math.Abs(median-3) > 0.7 {
		t.Errorf("Pareto(3) median estimate = %.2f, want about 3", median)
	}
	if spread > 0.5 {
		t.Errorf("Pareto sample spread = %.2f, want a plateau below 0.5", spread)
	}

	// A bounded sample on a coarse lattice, which is what a tick-constrained
	// price series produces when no order ever walks the book.
	lattice := make([]float64, 40000)
	for i := range lattice {
		lattice[i] = 0.5 + float64(int(uniform()*4))*0.1
	}
	latticeMedian, latticeSpread := HillStability(lattice)
	if latticeSpread <= spread {
		t.Errorf("lattice sample spread %.2f did not exceed the Pareto plateau %.2f", latticeSpread, spread)
	}
	if latticeSpread < 1.0 {
		t.Errorf("lattice spread = %.2f (median %.2f), want a clearly unstable estimate", latticeSpread, latticeMedian)
	}
}

// Trade-indexed returns are dominated by the bid-ask bounce at lag one, so they
// cannot be compared against empirical values measured on time-sampled returns.
func TestStridedReturnsRemoveTheBounce(t *testing.T) {
	tape := &TradeTape{}
	price := int64(50_000_00000)
	for i := 0; i < 5000; i++ {
		// A drifting mid with a one-tick bounce on alternate trades, which is
		// the structure a quoted market produces.
		price += 1000
		observed := price
		if i%2 == 1 {
			observed = price + 40000
		}
		tape.Prices = append(tape.Prices, observed)
		tape.Timestamps = append(tape.Timestamps, int64(i)*1e9)
		tape.Signs = append(tape.Signs, 1)
	}
	trade := Autocorrelation(tape.LogReturns(), 1)
	strided := Autocorrelation(tape.StridedLogReturns(20), 1)
	if trade[0] > -0.5 {
		t.Fatalf("trade-indexed ACF(1) = %.3f, expected a strong negative bounce", trade[0])
	}
	if strided[0] < trade[0] {
		t.Errorf("striding did not reduce the bounce: trade %.3f, strided %.3f", trade[0], strided[0])
	}
	if len(tape.StridedLogReturns(0)) != 0 || len(tape.StridedLogReturns(999999)) != 0 {
		t.Error("invalid strides must return no samples")
	}
}

// Trade time and calendar time are different clocks and a panel scored against
// published empirical values must use the second. In trade time the absolute
// return is the pinned half-spread and is memoryless whatever the price process
// does, so a clustering measurement taken there reports zero by construction.
func TestTimeSampledReturnsSeeClusteringThatTradeTimeCannot(t *testing.T) {
	// A tape whose trade RATE clusters while each print is a fixed one-tick
	// hop: bursts of many trades, then quiet. Trade time cannot distinguish
	// the two regimes; calendar time must.
	tape := &TradeTape{}
	price := int64(50_000_00000)
	state := uint64(7)
	now := int64(0)
	busy := true
	for burst := 0; burst < 600; burst++ {
		state = state*6364136223846793005 + 1442695040888963407
		// A persistent regime rather than an alternating one: busy seconds
		// follow busy seconds. Strict alternation would give a negative lag-one
		// autocorrelation, which is periodicity, not clustering.
		if (state>>33)%100 < 8 {
			busy = !busy
		}
		trades := 2
		if busy {
			trades = 60
		}
		for i := 0; i < trades; i++ {
			state = state*6364136223846793005 + 1442695040888963407
			// Print sizes vary, so absolute return has real variance, but the
			// variation is independent from print to print: in trade time there
			// is no structure to find. The structure is in the arrival rate.
			ticks := int64((state>>33)%4 + 1)
			if (state>>40)%2 == 0 {
				price += 1000 * ticks
			} else {
				price -= 1000 * ticks
			}
			tape.Prices = append(tape.Prices, price)
			tape.Timestamps = append(tape.Timestamps, now)
			tape.Signs = append(tape.Signs, 1)
		}
		now += int64(1e9)
	}
	tradeAbs := Autocorrelation(Abs(tape.LogReturns()), 1)
	timeAbs := Autocorrelation(Abs(tape.TimeSampledLogReturns(1e9)), 1)
	if math.Abs(tradeAbs[0]) > 0.1 {
		t.Fatalf("trade-time |return| ACF(1) = %.3f, expected near zero by construction", tradeAbs[0])
	}
	if timeAbs[0] <= tradeAbs[0]+0.05 {
		t.Errorf("time sampling did not reveal the rate clustering: trade %.3f, one second %.3f",
			tradeAbs[0], timeAbs[0])
	}
	if len(tape.TimeSampledLogReturns(0)) != 0 {
		t.Error("a non-positive bucket must yield no samples")
	}
}

// Dozens of trades share one timestamp at this clock granularity, so the sort
// restoring time order has to be stable: an unstable one permutes within-second
// batches and the differenced series is no longer the sequence the matcher
// produced.
func TestTapeOrderingIsStableWithinATimestamp(t *testing.T) {
	dir := writeRun(t, Report{}, map[string][]string{
		"north/spot/ABC-USD.jsonl": {
			`{"sim_ts":1000000000,"data":{"venue_id":"north","payload":{"price":100,"qty":1,"side":"BUY"}},"event":"Trade"}`,
			`{"sim_ts":1000000000,"data":{"venue_id":"north","payload":{"price":200,"qty":1,"side":"BUY"}},"event":"Trade"}`,
			`{"sim_ts":1000000000,"data":{"venue_id":"north","payload":{"price":300,"qty":1,"side":"BUY"}},"event":"Trade"}`,
			`{"sim_ts":1000000000,"data":{"venue_id":"north","payload":{"price":400,"qty":1,"side":"BUY"}},"event":"Trade"}`,
		},
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	tape, err := run.Tape("north", "ABC-USD")
	if err != nil {
		t.Fatalf("Tape: %v", err)
	}
	want := []int64{100, 200, 300, 400}
	for i, price := range want {
		if tape.Prices[i] != price {
			t.Fatalf("tape order = %v, want file order %v", tape.Prices, want)
		}
	}
}

// A selection that matches no file must scan nothing. Treating an empty
// selection as the whole run turns a mistyped venue into a blend of every book,
// which produced a full-looking tape of 1.79 million trades for a venue that
// does not exist.
func TestTapeForAnUnknownVenueIsEmpty(t *testing.T) {
	dir := writeRun(t, Report{}, map[string][]string{
		"north/spot/ABC-USD.jsonl": {`{"sim_ts":1000000000,"data":{"venue_id":"north","payload":{"price":100,"qty":1,"side":"BUY"}},"event":"Trade"}`},
		"north/spot/CDF-USD.jsonl": {`{"sim_ts":1000000000,"data":{"venue_id":"north","payload":{"price":7,"qty":1,"side":"BUY"}},"event":"Trade"}`},
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if tape, err := run.Tape("nrth", "ABC-USD"); err != nil || len(tape.Prices) != 0 {
		t.Fatalf("unknown venue produced %d trades, want none", len(tape.Prices))
	}
	if tape, err := run.Tape("north", "ABC/USD"); err != nil || len(tape.Prices) != 0 {
		t.Fatalf("wrong symbol form produced %d trades, want none", len(tape.Prices))
	}
	tape, err := run.Tape("north", "ABC-USD")
	if err != nil || len(tape.Prices) != 1 {
		t.Fatalf("correct selection produced %d trades, want 1", len(tape.Prices))
	}
}

// An OrderFill carries the execution's quantity and the order's cumulative
// filled quantity. Summing the second across partial fills counts early fills
// repeatedly, which inflates whichever class trades most, breaks the identity
// that signed flow sums to zero, and reads as an engine defect. The residual is
// the guard against that.
func TestNetFlowByRoleSumsToZeroAndUsesExecutionQuantity(t *testing.T) {
	// One order filled in two parts against one counterparty. Cumulative
	// filled_qty is 30 then 100, while the executions are 30 and 70.
	dir := writeRun(t, Report{TerminalAccounts: []AccountRow{
		{VenueID: "north", ClientID: 1, Role: "noise_flow_1"},
		{VenueID: "north", ClientID: 2, Role: "spot_maker_1"},
	}}, map[string][]string{
		"north/spot/ABC-USD.jsonl": {
			`{"sim_ts":1,"client_id":1,"data":{"venue_id":"north","payload":{"qty":30,"filled_qty":30,"side":"BUY","role":"taker"}},"event":"OrderFill"}`,
			`{"sim_ts":1,"client_id":2,"data":{"venue_id":"north","payload":{"qty":30,"filled_qty":30,"side":"SELL","role":"maker"}},"event":"OrderFill"}`,
			`{"sim_ts":2,"client_id":1,"data":{"venue_id":"north","payload":{"qty":70,"filled_qty":100,"side":"BUY","role":"taker"}},"event":"OrderFill"}`,
			`{"sim_ts":2,"client_id":2,"data":{"venue_id":"north","payload":{"qty":70,"filled_qty":70,"side":"SELL","role":"maker"}},"event":"OrderFill"}`,
		},
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	table, residual, err := run.NetFlowByRole("north", "ABC-USD")
	if err != nil {
		t.Fatalf("NetFlowByRole: %v", err)
	}
	if residual != 0 {
		t.Fatalf("residual = %d, want zero: the sum is not counting each trade once per side", residual)
	}
	if got := table["noise_flow"].Net(); got != 100 {
		t.Errorf("taker net = %d, want 100 from executions of 30 and 70 (170 would be the cumulative-field error)", got)
	}
	if got := table["spot_maker"].Net(); got != -100 {
		t.Errorf("maker net = %d, want -100", got)
	}
	if got := table["noise_flow"].Imbalance(); got != 1 {
		t.Errorf("one-sided taker imbalance = %f, want 1", got)
	}
	if empty := (NetFlow{}); empty.Imbalance() != 0 {
		t.Error("an empty flow must report zero imbalance rather than dividing by zero")
	}
}

// One lag cannot say whether a level wanders or slides. A series can have a
// first-lag autocorrelation of essentially zero while every longer lag is
// strongly positive, which is a trending level: one arm reported 0.007 at lag
// one with lags two through twenty between 0.11 and 0.27, summing to 4.97.
func TestVarianceRatioSeesTrendingThatTheFirstLagMisses(t *testing.T) {
	// A moving average carrying only a lag-two term: its first autocorrelation
	// is zero by construction while its second is positive, so a variance ratio
	// sees persistence that lag one cannot. One arm in this campaign reported
	// 0.007 at lag one with lags two through twenty between 0.11 and 0.27.
	n := 20000
	noise := make([]float64, n+2)
	state := uint64(2024)
	for i := range noise {
		state = state*6364136223846793005 + 1442695040888963407
		noise[i] = float64(state>>33)/float64(1<<31) - 0.5
	}
	returns := make([]float64, n)
	for i := range returns {
		returns[i] = noise[i+2] + noise[i]
	}
	lags := Autocorrelation(returns, 30)
	if math.Abs(lags[0]) > 0.05 {
		t.Fatalf("first lag is %.3f, expected about zero by construction", lags[0])
	}
	if lags[1] < 0.3 {
		t.Fatalf("second lag is %.3f, expected about 0.5", lags[1])
	}
	weighted := 0.0
	for index, value := range lags[:29] {
		weighted += (1 - float64(index+1)/30) * value
	}
	if ratio := 1 + 2*weighted; ratio < 1.5 {
		t.Errorf("variance ratio %.2f did not reveal the persistence the first lag missed", ratio)
	}
}

// The exponent of the price response to trade size distinguishes a book whose
// depth is displayed and fixed, where impact is proportional to size, from one
// where much of the liquidity is latent and impact grows as roughly the square
// root. The distinction matters because proportional impact makes a large
// return and a moved level the same event.
func TestImpactRecoversAKnownExponent(t *testing.T) {
	for _, want := range []float64{0.5, 1.0} {
		tape := &TradeTape{}
		price := 1e9
		state := uint64(11)
		for i := 0; i < 20000; i++ {
			state = state*6364136223846793005 + 1442695040888963407
			size := float64(state>>40) + 1
			// Construct the response to follow the target exponent exactly, so
			// the estimator is measured against a known answer.
			response := 0.001 * math.Pow(size, want)
			sign := int8(1)
			if (state>>20)%2 == 0 {
				sign = -1
			}
			// The constructed response is the move from before this trade to
			// after it, which is what the estimator must recover.
			tape.Prices = append(tape.Prices, int64(price))
			// The constructed series is a mid series: the response is now
			// measured mid-to-mid, so the terminal must be a mid too.
			tape.PreMid = append(tape.PreMid, int64(price))
			tape.Qtys = append(tape.Qtys, int64(size))
			tape.Signs = append(tape.Signs, sign)
			price *= math.Exp(float64(sign) * response / 1e4)
		}
		// One trade ahead, so the response measured is the one constructed.
		// Horizon zero would compare the pre-trade price with itself, so one
		// trade ahead spans exactly the constructed move.
		curve := tape.Impact(ImpactOptions{HorizonTrades: 1, Buckets: 10})
		if curve.R2 < 0.9 {
			t.Errorf("exponent %.1f: fit R2 = %.2f, too low to quote", want, curve.R2)
		}
		if math.Abs(curve.Exponent-want) > 0.15 {
			t.Errorf("recovered exponent %.3f, want %.1f (R2 %.2f)", curve.Exponent, want, curve.R2)
		}
	}
	// Too little data must yield no curve rather than a fitted number.
	if empty := (&TradeTape{}).Impact(ImpactOptions{}); empty.Exponent != 0 || len(empty.Buckets) != 0 {
		t.Errorf("an empty tape produced a curve: %+v", empty)
	}
}

func TestTradeTapeSeparatesZeroMidpointFromNoSnapshot(t *testing.T) {
	tape := &TradeTape{
		PreMid:          []int64{0, 0, -10},
		PreMidAvailable: []bool{true, false, true},
	}
	if price, ok := tape.preMidAt(0); !ok || price != 0 {
		t.Fatalf("present zero midpoint = (%d, %t), want (0, true)", price, ok)
	}
	if price, ok := tape.preMidAt(1); ok || price != 0 {
		t.Fatalf("missing midpoint = (%d, %t), want (0, false)", price, ok)
	}
	if price, ok := tape.preMidAt(2); !ok || price != -10 {
		t.Fatalf("present negative midpoint = (%d, %t), want (-10, true)", price, ok)
	}
}

// Impact pooled over every participant measures who trades at each size rather
// than what a trade does. Conditioning on one class is what gives the curve a
// single meaning.
func TestImpactConditionsOnParticipantClass(t *testing.T) {
	tape := &TradeTape{}
	price := 1e9
	for i := 0; i < 4000; i++ {
		role := "noise_flow"
		// One class pushes the price with its trades, the other trades against
		// the move. Pooled they cancel; separated they are opposite.
		sign := int8(1)
		move := 1.0
		if i%2 == 1 {
			role = "arb"
			move = -1.0
		}
		tape.Prices = append(tape.Prices, int64(price))
		tape.PreMid = append(tape.PreMid, int64(price))
		tape.Qtys = append(tape.Qtys, int64(1000+i%50))
		tape.Signs = append(tape.Signs, sign)
		tape.Roles = append(tape.Roles, role)
		price *= math.Exp(move * 0.5 / 1e4)
	}
	// One trade ahead, so each observation spans exactly the move that trade
	// made. A longer horizon here would span one push and one opposing move and
	// net to zero for both classes.
	pooled := tape.Impact(ImpactOptions{HorizonTrades: 1, Buckets: 4})
	noise := tape.Impact(ImpactOptions{HorizonTrades: 1, Buckets: 4, Role: "noise_flow"})
	arb := tape.Impact(ImpactOptions{HorizonTrades: 1, Buckets: 4, Role: "arb"})

	if noise.N == 0 || arb.N == 0 {
		t.Fatalf("conditioning found no trades: noise %d, arb %d", noise.N, arb.N)
	}
	if noise.N+arb.N != pooled.N {
		t.Errorf("classes sum to %d against a pooled %d", noise.N+arb.N, pooled.N)
	}
	meanOf := func(curve ImpactCurve) float64 {
		var total float64
		for _, bucket := range curve.Buckets {
			total += bucket.MeanResponse
		}
		return total / float64(len(curve.Buckets))
	}
	if meanOf(noise) <= 0 {
		t.Errorf("the pushing class has a mean response of %.3f, want positive", meanOf(noise))
	}
	if meanOf(arb) >= 0 {
		t.Errorf("the opposing class has a mean response of %.3f, want negative", meanOf(arb))
	}
	if unknown := tape.Impact(ImpactOptions{Role: "absent"}); unknown.N != 0 {
		t.Errorf("an absent class produced %d observations", unknown.N)
	}
}
