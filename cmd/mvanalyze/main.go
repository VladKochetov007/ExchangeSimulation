// Command mvanalyze reports market-quality metrics over multivenue run logs.
//
// It is an adapter: every metric it prints is a function in the analysis
// package, and the flags only choose which to call and over which runs.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	"exchange_sim/analysis"
)

func main() {
	metric := flag.String("metric", "roles", "roles, stalls, triangular")
	venue := flag.String("venue", "north", "venue for book-level metrics")
	base := flag.String("base", "ABC-USD", "triangle base book")
	quote := flag.String("quote", "CDF-USD", "triangle quote book")
	cross := flag.String("cross", "ABC-CDF", "triangle cross book")
	crossPrecision := flag.Int64("cross-precision", 100_000_000, "cross pair price precision")
	horizon := flag.Float64("horizon", 900, "parent abandon horizon in seconds")
	desks := flag.Int("desks", 6, "execution desks, for the stall horizon denominator")
	runSeconds := flag.Float64("run-seconds", 8*3600, "run length in seconds")
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
