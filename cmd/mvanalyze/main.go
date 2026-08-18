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
	"strconv"
	"strings"

	"exchange_sim/analysis"
)

func main() {
	metric := flag.String("metric", "roles", "roles, stalls, triangular, stylized, flow, impact, bookshape, sweep, sweepimpact")
	venue := flag.String("venue", "north", "venue for book-level metrics")
	base := flag.String("base", "ABC-USD", "triangle base book")
	quote := flag.String("quote", "CDF-USD", "triangle quote book")
	cross := flag.String("cross", "ABC-CDF", "triangle cross book")
	crossPrecision := flag.Int64("cross-precision", 100_000_000, "cross pair price precision")
	horizon := flag.Float64("horizon", 900, "parent abandon horizon in seconds")
	desks := flag.Int("desks", 6, "execution desks, for the stall horizon denominator")
	runSeconds := flag.Float64("run-seconds", 8*3600, "run length in seconds")
	horizonTrades := flag.Int("horizon-trades", 10, "trades ahead over which impact is measured")
	impactRole := flag.String("impact-role", "", "restrict impact to one participant class")
	tickSize := flag.Int64("tick", 10_000, "book tick size, for the spread in ticks")
	walkSizes := flag.String("walk-sizes", "", "comma-separated order sizes in base units, for the walkable fraction")
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
			curve := tape.Impact(analysis.ImpactOptions{HorizonTrades: *horizonTrades, Role: *impactRole})
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
		case "bookshape":
			files := run.BookFiles(*venue, *base)
			if len(files) == 0 {
				fmt.Fprintf(os.Stderr, "%s: no book log for %s at venue %s\n", dir, *base, *venue)
				os.Exit(1)
			}
			opts := analysis.BookShapeOptions{Files: files, TickSize: *tickSize}
			shape, err := run.MeasureBookShape(opts)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			if shape.Snapshots == 0 {
				fmt.Fprintf(os.Stderr, "%s: no book snapshots for %s at venue %s\n", dir, *base, *venue)
				os.Exit(1)
			}
			var walkable []analysis.WalkableFraction
			if *walkSizes != "" {
				sizes, parseErr := parseSizes(*walkSizes)
				if parseErr != nil {
					fmt.Fprintf(os.Stderr, "%s: %v\n", dir, parseErr)
					os.Exit(2)
				}
				if walkable, err = run.MeasureWalkable(opts, sizes); err != nil {
					fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
					os.Exit(1)
				}
			}
			emit(dir, map[string]any{"shape": shape, "walkable": walkable}, *asJSON, func() {
				emptyShare := 100 * float64(shape.OneSideEmpty+shape.BothSidesEmpty) / float64(shape.Snapshots)
				fmt.Printf("%-20s snaps %6d  empty %5.1f%%  levels bid %4.1f ask %4.1f (p90 %4.1f/%4.1f)  "+
					"touch-share med %5.3f p90 %5.3f  touch %8.2f  beyond %8.2f  spread %5.1f ticks  hidden %5.3f\n",
					dir, shape.Snapshots, emptyShare,
					shape.BidLevels.Median, shape.AskLevels.Median, shape.BidLevels.P90, shape.AskLevels.P90,
					shape.TouchShare.Median, shape.TouchShare.P90,
					shape.TouchDepth.Median/1e8, shape.BeyondTouchDepth.Median/1e8,
					shape.SpreadTicks.Median, shape.HiddenShare.Mean)
				for _, fraction := range walkable {
					fmt.Printf("    size %8.2f  walks past touch %5.1f%%  exhausts book %5.1f%%\n",
						float64(fraction.SizeBase)/1e8, 100*fraction.ExceedsTouch, 100*fraction.ExceedsBook)
				}
			})
		case "sweep":
			files := run.BookFiles(*venue, *base)
			if len(files) == 0 {
				fmt.Fprintf(os.Stderr, "%s: no book log for %s at venue %s\n", dir, *base, *venue)
				os.Exit(1)
			}
			sweep, err := run.MeasureSweep(analysis.BookShapeOptions{Files: files})
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			if sweep.Orders == 0 {
				fmt.Fprintf(os.Stderr, "%s: no taker orders for %s at venue %s\n", dir, *base, *venue)
				os.Exit(1)
			}
			emit(dir, sweep, *asJSON, func() {
				fmt.Printf("%-20s orders %7d  multi-price %5.2f%%  prices/order med %3.1f p99 %4.1f  "+
					"fills/order med %3.1f p99 %4.1f  span-when-multi med %5.2f p90 %5.2f max %6.2f bps\n",
					dir, sweep.Orders, 100*sweep.MultiPriceFraction(),
					sweep.PricesPerOrder.Median, sweep.PricesPerOrder.P99,
					sweep.FillsPerOrder.Median, sweep.FillsPerOrder.P99,
					sweep.SweepBpsWhenMulti.Median, sweep.SweepBpsWhenMulti.P90, sweep.SweepBpsWhenMulti.Max)
			})
		case "sweepimpact":
			tape, err := run.Tape(*venue, *base)
			if err != nil || len(tape.Prices) == 0 {
				fmt.Fprintf(os.Stderr, "%s: no trades for %s at venue %s\n", dir, *base, *venue)
				os.Exit(1)
			}
			result := tape.MeasureSweepImpact(analysis.ImpactOptions{HorizonTrades: *horizonTrades, Role: *impactRole})
			if result.BucketsCompared == 0 {
				fmt.Fprintf(os.Stderr, "%s: no size bucket held both classes; sweeping is not separable from size here\n", dir)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-20s swept %6d single %6d  buckets %2d/%2d favour swept  mean gap %+6.3f bps\n",
					dir, result.SweptN, result.SingleN, result.BucketsFavouringSwept, result.BucketsCompared, result.MeanGapBps)
				for _, bucket := range result.Buckets {
					if bucket.SweptN == 0 || bucket.SingleN == 0 {
						continue
					}
					fmt.Printf("    size %8.3f  swept %+6.3f (%5d)  single %+6.3f (%5d)  gap %+6.3f\n",
						bucket.MeanSize/1e8, bucket.SweptResponse, bucket.SweptN,
						bucket.SingleResp, bucket.SingleN, bucket.GapBps)
				}
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

// parseSizes reads the walkable-fraction sizes, in base units.
func parseSizes(spec string) ([]int64, error) {
	var sizes []int64
	for _, field := range strings.Split(spec, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		size, err := strconv.ParseInt(field, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("walk size %q: %w", field, err)
		}
		if size <= 0 {
			return nil, fmt.Errorf("walk size %q is not positive", field)
		}
		sizes = append(sizes, size)
	}
	if len(sizes) == 0 {
		return nil, fmt.Errorf("no walk sizes in %q", spec)
	}
	return sizes, nil
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
