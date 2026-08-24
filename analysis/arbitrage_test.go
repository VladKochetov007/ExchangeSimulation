package analysis

import (
	"fmt"
	"testing"
)

func quoteLine(ts int64, venue, file string, bid, ask int64) (string, string) {
	line := fmt.Sprintf(`{"sim_ts":%d,"client_id":0,"event":"BookSnapshot","data":{"venue_id":%q,"payload":{"bids":[{"price":%d,"visible_qty":100,"hidden_qty":0}],"asks":[{"price":%d,"visible_qty":100,"hidden_qty":0}]}}}`,
		ts, venue, bid, ask)
	return file, line
}

// A market with a consistent triangle and a spread must show no edge once fees
// are paid. If the auditor reports one here, every later result is noise.
func TestArbitrageFindsNothingInAConsistentTriangle(t *testing.T) {
	const precision = int64(100_000_000)
	// ABC/USD at 50,000; CDF/USD at 2,500; so ABC/CDF is exactly 20.
	books := map[string][]string{}
	add := func(file, line string) { books[file] = append(books[file], line) }
	for i := int64(1); i <= 5; i++ {
		ts := i * int64(1e9)
		f, l := quoteLine(ts, "north", "north/spot/ABC-USD.jsonl", 4_999_900_000, 5_000_100_000)
		add(f, l)
		f, l = quoteLine(ts, "north", "north/spot/CDF-USD.jsonl", 249_995_000, 250_005_000)
		add(f, l)
		f, l = quoteLine(ts, "north", "north/spot/ABC-CDF.jsonl", 19_999*precision/1000, 20_001*precision/1000)
		add(f, l)
	}
	dir := writeRun(t, Report{}, books)
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureArbitrage(ArbitrageOptions{
		TakerFeeBps: 2, StalenessNanos: int64(2e9),
		BaseSymbol: "ABC-USD", QuoteSymbol: "CDF-USD", CrossSymbol: "ABC-CDF",
		CrossPrecision: precision,
	})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if len(result.Cycles) != 1 {
		t.Fatalf("cycles = %d, want the triangle only: %+v", len(result.Cycles), result.Cycles)
	}
	if result.Cycles[0].Observations == 0 {
		t.Fatal("the triangle was never evaluated")
	}
	if result.Cycles[0].Profitable != 0 {
		t.Errorf("a consistent triangle showed %d profitable instants, mean %.2f bps",
			result.Cycles[0].Profitable, result.Cycles[0].MeanEdgeBps)
	}
}

// A cross rate far away from the two legs is an arbitrage, and the auditor
// exists to say so and to say for how long it lasted.
func TestArbitrageFindsAMispricedCrossAndTimesIt(t *testing.T) {
	const precision = int64(100_000_000)
	books := map[string][]string{}
	add := func(file, line string) { books[file] = append(books[file], line) }
	for i := int64(1); i <= 5; i++ {
		ts := i * int64(1e9)
		f, l := quoteLine(ts, "north", "north/spot/ABC-USD.jsonl", 4_999_900_000, 5_000_100_000)
		add(f, l)
		f, l = quoteLine(ts, "north", "north/spot/CDF-USD.jsonl", 249_995_000, 250_005_000)
		add(f, l)
		// The cross is quoted five percent above the implied twenty.
		f, l = quoteLine(ts, "north", "north/spot/ABC-CDF.jsonl", 21*precision, 21*precision+precision/1000)
		add(f, l)
	}
	dir := writeRun(t, Report{}, books)
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureArbitrage(ArbitrageOptions{
		TakerFeeBps: 2, StalenessNanos: int64(2e9),
		BaseSymbol: "ABC-USD", QuoteSymbol: "CDF-USD", CrossSymbol: "ABC-CDF",
		CrossPrecision: precision,
	})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	cycle := result.Cycles[0]
	if cycle.Profitable == 0 {
		t.Fatalf("a five percent mispricing was not detected: %+v", cycle)
	}
	if cycle.MaxEdgeBps < 400 {
		t.Errorf("edge = %.1f bps, want about five hundred", cycle.MaxEdgeBps)
	}
	if cycle.LongestRunNanos < int64(3e9) {
		t.Errorf("longest run = %d ns, want the whole persistent stretch", cycle.LongestRunNanos)
	}
}

// A quote nobody has refreshed is not executable. Treating it as live invents
// arbitrage out of timing, which is the commonest way to report a fake one.
func TestArbitrageIgnoresStaleQuotes(t *testing.T) {
	const precision = int64(100_000_000)
	books := map[string][]string{
		"north/spot/ABC-USD.jsonl": {},
		"north/spot/CDF-USD.jsonl": {},
		"north/spot/ABC-CDF.jsonl": {},
	}
	_, line := quoteLine(int64(1e9), "north", "", 4_999_900_000, 5_000_100_000)
	books["north/spot/ABC-USD.jsonl"] = append(books["north/spot/ABC-USD.jsonl"], line)
	_, line = quoteLine(int64(1e9), "north", "", 249_995_000, 250_005_000)
	books["north/spot/CDF-USD.jsonl"] = append(books["north/spot/CDF-USD.jsonl"], line)
	// The cross prints an hour later, by which time the other two are stale.
	_, line = quoteLine(int64(3_600e9), "north", "", 21*precision, 21*precision+precision/1000)
	books["north/spot/ABC-CDF.jsonl"] = append(books["north/spot/ABC-CDF.jsonl"], line)

	dir := writeRun(t, Report{}, books)
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureArbitrage(ArbitrageOptions{
		TakerFeeBps: 2, StalenessNanos: int64(2e9),
		BaseSymbol: "ABC-USD", QuoteSymbol: "CDF-USD", CrossSymbol: "ABC-CDF",
		CrossPrecision: precision,
	})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if len(result.Cycles) != 0 {
		t.Errorf("stale quotes produced %d cycles: %+v", len(result.Cycles), result.Cycles)
	}
}

// The same asset at two venues is the simplest cycle there is, and the fee has
// to bind: a gap smaller than the round trip's fees is not an arbitrage.
func TestArbitrageChargesFeesOnBothLegsAcrossVenues(t *testing.T) {
	books := map[string][]string{}
	add := func(file, line string) { books[file] = append(books[file], line) }
	for i := int64(1); i <= 3; i++ {
		ts := i * int64(1e9)
		// North bids 50,000.10 while south offers 50,000.00: a one-bp gap,
		// less than the four basis points a two-legged round trip costs.
		f, l := quoteLine(ts, "south", "south/spot/ABC-USD.jsonl", 4_999_900_000, 5_000_000_000)
		add(f, l)
		f, l = quoteLine(ts, "north", "north/spot/ABC-USD.jsonl", 5_000_010_000, 5_000_100_000)
		add(f, l)
	}
	dir := writeRun(t, Report{}, books)
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureArbitrage(ArbitrageOptions{
		TakerFeeBps: 2, StalenessNanos: int64(2e9), CrossVenueSymbol: "ABC-USD",
	})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	for _, cycle := range result.Cycles {
		if cycle.Profitable != 0 {
			t.Errorf("%s reported %d profitable instants at %.2f bps, but the gap is inside the fees",
				cycle.Cycle, cycle.Profitable, cycle.MaxEdgeBps)
		}
	}
	if len(result.Cycles) == 0 {
		t.Error("no cross-venue cycle was evaluated at all")
	}
}

// A signed or zero quote is a present book observation. The current
// cash-return arb metric cannot price it, but must report that condition rather
// than silently treating the book as missing or assigning it a zero edge.
func TestArbitrageRetainsSignedQuotesAsExplicitUndefinedDomain(t *testing.T) {
	const precision = int64(100_000_000)
	books := map[string][]string{}
	add := func(file, line string) { books[file] = append(books[file], line) }
	file, line := quoteLine(int64(1e9), "north", "north/spot/ABC-USD.jsonl", 100, 101)
	add(file, line)
	file, line = quoteLine(int64(1e9), "north", "north/spot/CDF-USD.jsonl", 0, 1)
	add(file, line)
	file, line = quoteLine(int64(1e9), "north", "north/spot/ABC-CDF.jsonl", 100*precision, 101*precision)
	add(file, line)

	dir := writeRun(t, Report{}, books)
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureArbitrage(ArbitrageOptions{
		TakerFeeBps: 2, StalenessNanos: int64(2e9),
		BaseSymbol: "ABC-USD", QuoteSymbol: "CDF-USD", CrossSymbol: "ABC-CDF",
		CrossPrecision: precision,
	})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if len(result.Cycles) != 1 {
		t.Fatalf("cycles = %d, want one undefined triangle: %+v", len(result.Cycles), result.Cycles)
	}
	cycle := result.Cycles[0]
	if cycle.Observations != 0 || cycle.UndefinedDomainObservations != 1 {
		t.Fatalf("cycle = %+v, want zero measured and one undefined-domain observation", cycle)
	}
}
