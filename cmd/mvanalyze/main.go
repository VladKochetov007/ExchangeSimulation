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
	metric := flag.String("metric", "roles", "roles, stalls, triangular, stylized, flow, impact, bookshape, sweep, sweepimpact, mechanical, spacing, resting, viability")
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
	horizonSeconds := flag.Float64("horizon-seconds", 0, "mechanical horizon in simulated seconds; overrides -horizon-trades")
	exhausted := flag.String("exhausted", "drop", "how orders that clear the whole visible side are priced: drop or deepest")
	viabilityWindow := flag.Float64("viability-window", 900, "viability window length in simulated seconds")
	minTradesPerWindow := flag.Int("viability-min-trades", 1, "fewest taker trades a window may have and stay viable")
	minTakerClasses := flag.Int("viability-min-taker-classes", 2, "fewest distinct taker classes a viable window needs")
	minMakerClasses := flag.Int("viability-min-maker-classes", 1, "fewest distinct maker classes a viable window needs")
	maxRoleShare := flag.Float64("viability-max-role-share", 0.9, "largest share of a window's volume one taker class may hold")
	maxSpreadTicks := flag.Float64("viability-max-spread-ticks", 0, "widest median spread in ticks a viable window may show; zero disables")
	maxEmptySideShare := flag.Float64("viability-max-empty-side-share", 0.02, "largest share of publications a viable window may have with a side missing")
	minTouchDepth := flag.Float64("viability-min-touch-depth", 0, "smallest median touch depth in base units a viable window may show; zero disables")
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
					"touch-share med %5.3f p90 %5.3f  touch %8.2f  beyond %8.2f  spread %5.1f ticks  hidden %5.3f  trades/snap %5.2f\n",
					dir, shape.Snapshots, emptyShare,
					shape.BidLevels.Median, shape.AskLevels.Median, shape.BidLevels.P90, shape.AskLevels.P90,
					shape.TouchShare.Median, shape.TouchShare.P90,
					shape.TouchDepth.Median/1e8, shape.BeyondTouchDepth.Median/1e8,
					shape.SpreadTicks.Median, shape.HiddenShare.Mean, shape.TradesPerSnapshot)
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
			sweep, err := run.MeasureSweep(analysis.BookShapeOptions{Files: files, TickSize: *tickSize})
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			if sweep.Orders == 0 {
				fmt.Fprintf(os.Stderr, "%s: no taker orders for %s at venue %s\n", dir, *base, *venue)
				os.Exit(1)
			}
			emit(dir, sweep, *asJSON, func() {
				// The mean span over ALL orders is the quantity to multiply
				// when asking what sweeping contributes; the conditional
				// median is not an estimator of anything.
				fmt.Printf("%-20s orders %7d  multi-price %5.2f%%  fills/order mean %4.2f  "+
					"mean-span %6.4f bps | when multi: med %5.2f mean %5.2f p90 %5.2f max %6.2f bps, med %4.1f ticks, elapsed med %5.2f p99 %6.2f s\n",
					dir, sweep.Orders, 100*sweep.MultiPriceFraction(),
					sweep.FillsPerOrder.Mean, sweep.MeanSpanBps(),
					sweep.SweepBpsWhenMulti.Median, sweep.SweepBpsWhenMulti.Mean,
					sweep.SweepBpsWhenMulti.P90, sweep.SweepBpsWhenMulti.Max,
					sweep.SweepTicksWhenMulti.Median,
					sweep.ElapsedSecondsWhenMulti.Median, sweep.ElapsedSecondsWhenMulti.P99)
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
			// The horizon multiplier is the sign-autocorrelation sum, not the
			// horizon: it says how much of a per-trade displacement survives
			// into the measured response.
			multiplier := tape.SignACFSum(*horizonTrades)
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-20s swept %6d single %6d  buckets %2d/%2d favour swept  mean gap %+6.3f bps  sign-acf-sum %5.2f (horizon %d)\n",
					dir, result.SweptN, result.SingleN, result.BucketsFavouringSwept,
					result.BucketsCompared, result.MeanGapBps, multiplier, *horizonTrades)
				for _, bucket := range result.Buckets {
					if bucket.SweptN == 0 || bucket.SingleN == 0 {
						continue
					}
					fmt.Printf("    size %8.3f  swept %+6.3f (%5d)  single %+6.3f (%5d)  gap %+6.3f\n",
						bucket.MeanSize/1e8, bucket.SweptResponse, bucket.SweptN,
						bucket.SingleResp, bucket.SingleN, bucket.GapBps)
				}
			})
		case "mechanical":
			files := run.BookFiles(*venue, *base)
			if len(files) != 1 {
				// The replay depends on event order, which holds within a file
				// and not across concurrently written ones.
				fmt.Fprintf(os.Stderr, "%s: %s at venue %s resolves to %d files, want exactly one\n",
					dir, *base, *venue, len(files))
				os.Exit(1)
			}
			opts := analysis.MechanicalOptions{HorizonTrades: *horizonTrades}
			switch *exhausted {
			case "drop":
			case "deepest":
				opts.ExhaustedPrice = analysis.ExhaustedAtDeepestVisible
			default:
				fmt.Fprintf(os.Stderr, "unknown -exhausted %q: want drop or deepest\n", *exhausted)
				os.Exit(2)
			}
			if *horizonSeconds > 0 {
				opts.HorizonTrades = 0
				opts.HorizonNanos = int64(*horizonSeconds * 1e9)
			}
			result, err := analysis.MeasureMechanicalImpact(files[0], opts)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			// A reconstruction that disagrees with the engine's own snapshots
			// is not a measurement, so it is refused rather than reported.
			if result.Drift.Mismatches > 0 {
				fmt.Fprintf(os.Stderr, "%s: replayed book diverged from %d of %d snapshots; the decomposition is unusable\n",
					dir, result.Drift.Mismatches, result.Drift.Checks)
				os.Exit(1)
			}
			horizonLabel := fmt.Sprintf("%dtr", *horizonTrades)
			if *horizonSeconds > 0 {
				horizonLabel = fmt.Sprintf("%.2fs", *horizonSeconds)
			}
			emit(dir, result, *asJSON, func() {
				// The absolute share is the headline, not a squared statistic:
				// most orders have a mechanical move of exactly zero, so any
				// R2 reports the size of that mass rather than a market fact.
				fmt.Printf("%-14s h %7s  orders %6d (zero %6d moved %5d unmeas %4d)  "+
					"|mech| %7.5f |rev| %7.5f share %5.3f  |  zero-subsample |actual| %7.5f  "+
					"slope %+6.3f  walk-agree %5.3f  drift %d/%d\n",
					dir, horizonLabel, result.Orders, result.ZeroMechanical, result.MovedOrders,
					result.UnmeasurableOrders,
					result.MeanAbsMechanicalBps, result.MeanAbsRevisionBps, result.AbsMechanicalShare,
					result.ZeroSubsampleMeanAbsBps, result.Slope, result.WalkAgreement,
					result.Drift.Mismatches, result.Drift.Checks)
			})
		case "viability":
			// The rules are assembled here, in the adapter, because what counts
			// as a living market is the caller's judgement and not a property
			// of the measurement. The library measures; the thresholds are
			// configuration.
			rules := []analysis.ViabilityRule{
				{Name: "thin_volume", Breached: func(w analysis.MarketWindow) bool {
					return w.Trades < *minTradesPerWindow
				}},
				{Name: "few_taker_classes", Breached: func(w analysis.MarketWindow) bool {
					return w.TakerRoles < *minTakerClasses
				}},
				{Name: "few_maker_classes", Breached: func(w analysis.MarketWindow) bool {
					return w.MakerRoles < *minMakerClasses
				}},
				// A single one-sided publication is a book between requotes,
				// not a dead market. What matters is the share of the window a
				// taker had nothing to trade against.
				{Name: "one_sided_book", Breached: func(w analysis.MarketWindow) bool {
					if w.Snapshots == 0 {
						return false
					}
					return float64(w.EmptySideSnapshots)/float64(w.Snapshots) > *maxEmptySideShare
				}},
				{Name: "concentrated_flow", Breached: func(w analysis.MarketWindow) bool {
					return w.TopRoleVolumeShare > *maxRoleShare
				}},
			}
			if *maxSpreadTicks > 0 {
				rules = append(rules, analysis.ViabilityRule{Name: "wide_spread", Breached: func(w analysis.MarketWindow) bool {
					return w.SpreadTicks.N > 0 && w.SpreadTicks.Median > *maxSpreadTicks
				}})
			}
			if *minTouchDepth > 0 {
				rules = append(rules, analysis.ViabilityRule{Name: "thin_depth", Breached: func(w analysis.MarketWindow) bool {
					return w.TouchDepth.N > 0 && w.TouchDepth.Median < *minTouchDepth
				}})
			}
			result, err := run.MeasureViability(analysis.ViabilityOptions{
				WindowNanos: int64(*viabilityWindow * 1e9),
				TickSize:    *tickSize,
				Rules:       rules,
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-14s books %2d  windows %4d viable %4d breached %4d\n",
					dir, result.Books, len(result.Windows), result.ViableWindows, result.BreachedWindows)
				for _, rule := range sortedRuleNames(result.BreachesByRule) {
					fmt.Printf("    %-20s %4d windows\n", rule, result.BreachesByRule[rule])
				}
				for _, book := range result.DeadBooks {
					fmt.Printf("    dead book: %s\n", book)
				}
			})
		case "spacing":
			files := run.BookFiles(*venue, *base)
			if len(files) != 1 {
				fmt.Fprintf(os.Stderr, "%s: %s at venue %s resolves to %d files, want exactly one\n",
					dir, *base, *venue, len(files))
				os.Exit(1)
			}
			result, err := analysis.MeasureLevelSpacing(files[0], analysis.SpacingOptions{TickSize: *tickSize, SampleEvery: 10})
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			if result.Drift.Mismatches > 0 {
				fmt.Fprintf(os.Stderr, "%s: replayed book diverged from %d of %d snapshots\n",
					dir, result.Drift.Mismatches, result.Drift.Checks)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-14s obs %6d  levels/side med %3.1f p90 %3.1f  first-gap med %5.1f mean %6.2f  "+
					"all-gaps med %5.1f mean %6.2f p90 %6.1f  spread med %5.1f  one-tick-gaps %5.3f\n",
					dir, result.Observations,
					result.LevelsPerSide.Median, result.LevelsPerSide.P90,
					result.FirstGapTicks.Median, result.FirstGapTicks.Mean,
					result.AllGapsTicks.Median, result.AllGapsTicks.Mean, result.AllGapsTicks.P90,
					result.SpreadTicks.Median, result.SingleTickGapShare)
			})
		case "resting":
			files := run.BookFiles(*venue, *base)
			if len(files) != 1 {
				fmt.Fprintf(os.Stderr, "%s: %s at venue %s resolves to %d files, want exactly one\n",
					dir, *base, *venue, len(files))
				os.Exit(1)
			}
			result, err := analysis.MeasureRestingPlacement(files[0], analysis.RestingOptions{
				TickSize: *tickSize,
				Role:     func(clientID uint64) string { return analysis.RoleGroup(run.Role(*venue, clientID)) },
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%s  marketable %d  unattributed %d\n", dir, result.Marketable, result.Unattributed)
				for _, role := range result.RolesByDistance() {
					stats := result.ByRole[role]
					fmt.Printf("    %-22s orders %7d  distance med %7.1f p75 %7.1f p90 %8.1f ticks  qty med %8.2f\n",
						role, stats.Orders, stats.DistanceTicks.Median, stats.DistanceTicks.P75,
						stats.DistanceTicks.P90, stats.Qty.Median/1e8)
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

// sortedRuleNames orders a breach tally so the report is deterministic.
func sortedRuleNames(breaches map[string]int) []string {
	names := make([]string, 0, len(breaches))
	for name := range breaches {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
