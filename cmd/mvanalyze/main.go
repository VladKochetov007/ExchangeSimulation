// Command mvanalyze reports market-quality metrics over multivenue run logs.
//
// It is an adapter: every metric it prints is a function in the analysis
// package, and the flags only choose which to call and over which runs.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"

	"exchange_sim/analysis"
)

func main() {
	metric := flag.String("metric", "roles", "roles, stalls, triangular, stylized, flow, impact")
	venue := flag.String("venue", "north", "venue for book-level metrics")
	base := flag.String("base", "ABC-USD", "triangle base book")
	quote := flag.String("quote", "CDF-USD", "triangle quote book")
	cross := flag.String("cross", "ABC-CDF", "triangle cross book")
	crossPrecision := flag.Int64("cross-precision", 100_000_000, "cross pair price precision")
	horizon := flag.Float64("horizon", 900, "parent abandon horizon in seconds")
	desks := flag.Int("desks", 6, "execution desks, for the stall horizon denominator")
	runSeconds := flag.Float64("run-seconds", 8*3600, "run length in seconds")
	horizonTrades := flag.Int("horizon-trades", 10, "trades ahead over which impact is measured")
	asJSON := flag.Bool("json", false, "emit JSON instead of a table")
	flag.Parse()

	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: mvanalyze [flags] <run dir>...")
		os.Exit(2)
	}
	for _, dir := range flag.Args() {
		run, err := analysis.Open(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
			os.Exit(1)
		}
		switch *metric {
		case "roles":
			table, err := run.RoleTable()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			emitRoles(dir, table, *asJSON)
		case "stalls":
			stats := run.Stalls(analysis.StallOptions{HorizonSeconds: *horizon, Desks: *desks, RunSeconds: *runSeconds})
			emit(dir, stats, *asJSON, func() {
				fmt.Printf("%-24s parents %5d filled %5d stalled %4d zero-fill %4d stall-horizon %5.1f%% sides %v\n",
					dir, stats.Parents, stats.Filled, stats.Stalled, stats.ZeroFill, 100*stats.StallFraction(), stats.Sides)
			})
		case "flow":
			table, residual, err := run.NetFlowByRole(*venue, *base)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			// The residual must be zero: every trade has two sides. A non-zero
			// value means the sum is wrong and the per-class numbers below it
			// should be discarded rather than interpreted.
			if residual != 0 {
				fmt.Fprintf(os.Stderr, "%s: signed flow residual is %d, not zero; per-class figures are unusable\n", dir, residual)
				os.Exit(1)
			}
			names := make([]string, 0, len(table))
			for name := range table {
				names = append(names, name)
			}
			sort.Slice(names, func(i, j int) bool { return table[names[i]].Net() < table[names[j]].Net() })
			fmt.Printf("%s\n", dir)
			for _, name := range names {
				flow := table[name]
				fmt.Printf("  %-22s net %+12.0f  gross %12.0f  imbalance %+6.1f%%\n",
					name, float64(flow.Net())/1e8, float64(flow.Gross())/1e8, 100*flow.Imbalance())
			}
		case "impact":
			tape, err := run.Tape(*venue, *base)
			if err != nil || len(tape.Prices) == 0 {
				fmt.Fprintf(os.Stderr, "%s: no trades for %s at venue %s\n", dir, *base, *venue)
				os.Exit(1)
			}
			curve := tape.Impact(analysis.ImpactOptions{HorizonTrades: *horizonTrades})
			emit(dir, curve, *asJSON, func() {
				// An exponent from a poor fit is a number without a meaning, so
				// it is withheld rather than printed beside its own R2.
				exponent := fmt.Sprintf("%5.2f", curve.Exponent)
				if curve.R2 < 0.7 {
					exponent = "  n/a"
				}
				fmt.Printf("%-18s n %7d  exponent %s (R2 %4.2f)  smallest %8.1f -> %+6.2f bps  largest %8.1f -> %+6.2f bps\n",
					dir, curve.N, exponent, curve.R2,
					curve.Buckets[0].MeanSize/1e8, curve.Buckets[0].MeanResponse,
					curve.Buckets[len(curve.Buckets)-1].MeanSize/1e8, curve.Buckets[len(curve.Buckets)-1].MeanResponse)
			})
		case "stylized":
			tape, err := run.Tape(*venue, *base)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			facts := tape.Facts(50)
			// A tape the run does not contain is an error, not a panel of
			// zeroes: a mistyped venue or a pruned log directory would
			// otherwise print a plausible-looking row straight into a record.
			if facts.Trades == 0 {
				fmt.Fprintf(os.Stderr, "%s: no trades for %s at venue %s\n", dir, *base, *venue)
				os.Exit(1)
			}
			emit(dir, facts, *asJSON, func() {
				tail := fmt.Sprintf("%5.2f", facts.TailIndex)
				// Above roughly one the Hill estimate has no plateau and is not
				// a measurement of anything.
				if facts.TailSpread > 1 || math.IsNaN(facts.TailIndex) {
					tail = "  n/a"
				}
				// Trade-time and calendar-time numbers are reported side by
				// side because only the second is comparable to published
				// empirical values, and reporting only the first is how this
				// campaign concluded the market had no volatility clustering.
				fmt.Printf("%-16s n %6d | trade: ret %+6.3f |ret| %+6.3f sign1 %+6.3f sign50 %+6.3f | "+
					"1s: ret %+6.3f |ret| %+6.3f |ret|10 %+6.3f kurt %7.2f | 60s: ret %+6.3f |ret| %+6.3f vr %6.2f | tail %s\n",
					dir, facts.Trades, facts.ReturnACF1, facts.AbsReturnACF1, facts.SignACF1, facts.SignACF50,
					facts.Sec1ReturnACF1, facts.Sec1AbsReturnACF1, facts.Sec1AbsReturnACF10, facts.Sec1Kurtosis,
					facts.Sec60ReturnACF1, facts.Sec60AbsReturnACF1, facts.Sec60VarianceRatio, tail)
			})
		case "triangular":
			deviations, err := run.TriangularDeviation(analysis.TriangularConfig{
				VenueID: *venue, BaseSymbol: *base, QuotePair: *quote, CrossPair: *cross,
				CrossPrecision: *crossPrecision,
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			summary := analysis.Describe(analysis.Abs(deviations))
			emit(dir, summary, *asJSON, func() {
				fmt.Printf("%-24s n %6d median %6.1f p90 %6.1f p99 %6.1f max %6.1f bps\n",
					dir, summary.N, summary.Median, summary.P90, summary.P99, summary.Max)
			})
		default:
			fmt.Fprintf(os.Stderr, "unknown metric %q\n", *metric)
			os.Exit(2)
		}
	}
}

func emit(dir string, value any, asJSON bool, table func()) {
	if !asJSON {
		table()
		return
	}
	raw, _ := json.Marshal(map[string]any{"run": dir, "result": value})
	fmt.Println(string(raw))
}

func emitRoles(dir string, table map[string]*analysis.RoleStats, asJSON bool) {
	if asJSON {
		raw, _ := json.Marshal(map[string]any{"run": dir, "roles": table})
		fmt.Println(string(raw))
		return
	}
	names := make([]string, 0, len(table))
	for name := range table {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return table[names[i]].Accepted > table[names[j]].Accepted })
	fmt.Printf("%s\n", dir)
	fmt.Printf("  %-22s %10s %9s %8s %10s %9s\n", "role", "accepted", "fills", "conv", "ioc-missed", "rejected")
	for _, name := range names {
		stats := table[name]
		fmt.Printf("  %-22s %10d %9d %7.1f%% %10d %9d\n",
			name, stats.Accepted, stats.Fills, 100*stats.Conversion(), stats.IOCExpired, stats.Rejected)
	}
}
