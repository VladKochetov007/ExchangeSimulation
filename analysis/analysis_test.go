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
