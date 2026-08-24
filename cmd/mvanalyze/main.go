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
	"runtime"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"

	"exchange_sim/analysis"
)

// analyzerProfiles bounds process profiling to offline analysis. It does not
// participate in evidence decoding or alter any metric result.
type analyzerProfiles struct {
	cpu   *os.File
	alloc *os.File
	mutex *os.File
	block *os.File
}

func startAnalyzerProfiles(cpuPath, allocPath, mutexPath, blockPath string) (*analyzerProfiles, error) {
	profiles := &analyzerProfiles{}
	var err error
	if cpuPath != "" {
		if profiles.cpu, err = os.Create(cpuPath); err != nil {
			return nil, err
		}
		if err := pprof.StartCPUProfile(profiles.cpu); err != nil {
			_ = profiles.cpu.Close()
			return nil, err
		}
	}
	if allocPath != "" {
		if profiles.alloc, err = os.Create(allocPath); err != nil {
			profiles.Stop()
			return nil, err
		}
		// Sample allocations at Go's production-like default rate. Recording
		// every allocation would make a replay profile describe the profiler.
		runtime.MemProfileRate = 512 * 1024
	}
	if mutexPath != "" {
		if profiles.mutex, err = os.Create(mutexPath); err != nil {
			profiles.Stop()
			return nil, err
		}
		runtime.SetMutexProfileFraction(1)
	}
	if blockPath != "" {
		if profiles.block, err = os.Create(blockPath); err != nil {
			profiles.Stop()
			return nil, err
		}
		runtime.SetBlockProfileRate(1)
	}
	return profiles, nil
}

func (p *analyzerProfiles) Stop() {
	if p == nil {
		return
	}
	if p.cpu != nil {
		pprof.StopCPUProfile()
		_ = p.cpu.Close()
		p.cpu = nil
	}
	if p.alloc != nil {
		_ = pprof.Lookup("allocs").WriteTo(p.alloc, 0)
		_ = p.alloc.Close()
		p.alloc = nil
	}
	if p.mutex != nil {
		_ = pprof.Lookup("mutex").WriteTo(p.mutex, 0)
		_ = p.mutex.Close()
		p.mutex = nil
		runtime.SetMutexProfileFraction(0)
	}
	if p.block != nil {
		_ = pprof.Lookup("block").WriteTo(p.block, 0)
		_ = p.block.Close()
		p.block = nil
		runtime.SetBlockProfileRate(0)
	}
}

func main() {
	metric := flag.String("metric", "roles", "roles, postonly, makerquotesize, makerrebalance, liabilityhedger, noiseflowphase, stalls, triangular, stylized, flow, impact, bookshape, sweep, sweepimpact, mechanical, spacing, resting, viability, lifecycle, hedging, conservation, positions, fillpositions, settlements, expiryfills, orderlifecycle, arbitrage, crossvenue, roleaudit, ecology, liquidations, marginchecks, derivatives, observationreceipts, frontiervectors")
	postOnlyRoles := flag.String("post-only-roles", "", "comma-separated participant role groups for post-only activity")
	postOnlySymbols := flag.String("post-only-symbols", "", "comma-separated symbols for post-only activity")
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
	arbFeeBps := flag.Float64("arb-fee-bps", 2, "taker fee charged on every leg of an arbitrage cycle")
	arbStaleness := flag.Float64("arb-staleness", 2, "how many seconds old a quote may be and still count as executable")
	crossVenueSymbol := flag.String("cross-venue-symbol", "ABC-USD", "same asset book to compare across venues")
	crossVenueMin := flag.Int("cross-venue-min-venues", 3, "fewest fresh two-sided venues required for a midpoint-dispersion observation")
	crossVenuePositiveTimes := flag.Bool("cross-venue-positive-times", false, "include positive sampled cross-venue dispersion times for activation-window joins")
	conservationBook := flag.String("conservation-book", "", "restrict the conservation audit to one book, e.g. ABC/USD")
	basePrecision := flag.Int64("base-precision", 100_000_000, "base-asset precision, for converting position sizes into contracts")
	quotePrecision := flag.Int64("quote-precision", 100_000, "quote-asset precision, for converting logged prices into currency units")
	viabilityWindow := flag.Float64("viability-window", 900, "viability window length in simulated seconds")
	viabilityStart := flag.Float64("viability-start", 0, "exclude viability evidence before this simulated-second boundary")
	minTradesPerWindow := flag.Int("viability-min-trades", 1, "fewest taker trades a window may have and stay viable")
	minTakerClasses := flag.Int("viability-min-taker-classes", 2, "fewest distinct taker classes a viable window needs")
	minMakerClasses := flag.Int("viability-min-maker-classes", 1, "fewest distinct maker classes a viable window needs")
	maxRoleShare := flag.Float64("viability-max-role-share", 0.9, "largest share of a window's volume one taker class may hold")
	maxSpreadTicks := flag.Float64("viability-max-spread-ticks", 0, "widest median spread in ticks a viable window may show; zero disables")
	maxEmptySideShare := flag.Float64("viability-max-empty-side-share", 0.02, "largest share of publications a viable window may have with a side missing")
	viabilityThresholds := flag.String("viability-thresholds", "", "path to a JSON list of per-book viability thresholds, matched by symbol glob in order")
	judgeLifeEdges := flag.Bool("viability-judge-life-edges", false, "judge the windows a book lists and settles in, which are partial by construction")
	minTouchDepth := flag.Float64("viability-min-touch-depth", 0, "smallest median touch depth in base units a viable window may show; zero disables")
	tickSize := flag.Int64("tick", 10_000, "book tick size, for the spread in ticks")
	walkSizes := flag.String("walk-sizes", "", "comma-separated order sizes in base units, for the walkable fraction")
	asJSON := flag.Bool("json", false, "emit JSON instead of a table")
	cpuProfile := flag.String("cpuprofile", "", "write CPU profile for offline analysis")
	allocProfile := flag.String("allocprofile", "", "write sampled allocation profile after offline analysis")
	mutexProfile := flag.String("mutexprofile", "", "write mutex profile after offline analysis")
	blockProfile := flag.String("blockprofile", "", "write block profile after offline analysis")
	flag.Parse()

	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: mvanalyze [flags] <run dir>...")
		os.Exit(2)
	}
	profiles, err := startAnalyzerProfiles(*cpuProfile, *allocProfile, *mutexProfile, *blockProfile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start analysis profiles: %v\n", err)
		os.Exit(1)
	}
	defer profiles.Stop()
	for _, dir := range flag.Args() {
		// These compact V2 evidence artifacts are intentionally analyzable
		// without raw JSON logs, Greek reports, or a Run wrapper. Opening a Run
		// first would turn a successful information-boundary audit into a false
		// failure merely because unrelated market reports were not retained.
		if handled, err := emitStandaloneEvidenceMetric(*metric, dir, *asJSON); handled {
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			continue
		}
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
		case "postonly":
			result, err := run.MeasurePostOnlyActivity(analysis.PostOnlyActivityOptions{
				Roles:   strings.Split(*postOnlyRoles, ","),
				Symbols: strings.Split(*postOnlySymbols, ","),
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			if result.Events == 0 {
				fmt.Fprintf(os.Stderr, "%s: no selected post-only evidence events\n", dir)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-22s events %6d accepted post/regular %6d/%6d fills %6d qty %10d rejects take/invalid %5d/%5d unmatched-fills %d\n",
					dir, result.Events, result.AcceptedPostOnly, result.AcceptedRegular,
					result.PostOnlyFills, result.PostOnlyFilledQty, result.RejectedWouldTake,
					result.RejectedInvalid, result.UnmatchedFillOrders)
			})
		case "makerquotesize":
			result, err := run.MeasureMakerQuoteSize(analysis.MakerQuoteSizeOptions{})
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			if result.Decisions == 0 {
				fmt.Fprintf(os.Stderr, "%s: no P1 maker quote-size decision evidence\n", dir)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-22s decisions %6d sides %6d risk zero/nonzero %6d/%6d adjustment %6d signed long+/short-/zero %6d/%6d/%6d accepted/rejected %6d/%6d censored/delivered %d/%d missing/duplicate %d/%d policy/request/censor/direction mismatches %d/%d/%d/%d\n",
					dir, result.Decisions, result.DecisionSides, result.ZeroRiskDecisions, result.NonzeroRiskDecisions,
					result.NonzeroAdjustments, result.LongPositiveSizeSkew, result.ShortNegativeSizeSkew, result.ZeroRiskSymmetric,
					result.Accepted, result.Rejected, result.HorizonCensoredSides, result.CensoredOutcomeDeliveries,
					result.MissingOutcomes, result.DuplicateOutcomes, result.DecisionFieldMismatches, result.OutcomeFieldMismatches, result.InvalidCensorRecords, result.WrongDirectionSizeSkew)
				for _, bucket := range result.SkewBuckets {
					fmt.Printf("    size-skew %5d bps decisions %6d zero/nonzero %6d/%6d adjusted %6d\n",
						bucket.SizeSkewBps, bucket.Decisions, bucket.ZeroRisk, bucket.NonzeroRisk, bucket.Adjusted)
				}
				for _, maker := range result.MakerBuckets {
					fmt.Printf("    maker %-8s %-24s %-8s decisions %6d accepted/rejected %6d/%6d censored %6d\n",
						maker.VenueID, maker.Maker, maker.Symbol, maker.Decisions, maker.Accepted, maker.Rejected, maker.HorizonCensoredSides)
				}
			})
		case "makerrebalance":
			result, err := run.MeasureMakerInventoryRebalance()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			if result.Decisions == 0 {
				fmt.Fprintf(os.Stderr, "%s: no P2 maker inventory-rebalance decision evidence\n", dir)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-22s decisions %6d enabled/disabled %6d/%6d submitted/accepted/rejected/censored %6d/%6d/%6d/%6d fills %6d qty %10d receipt ok/missing/mismatch/future %t/%d/%d/%d field/outcome/missing/duplicate %d/%d/%d/%d fee/self/nonreduce %d/%d/%d valid %t\n",
					dir, result.Decisions, result.EnabledDecisions, result.DisabledDecisions, result.Submitted, result.Accepted, result.Rejected, result.HorizonCensored,
					result.Fills, result.FilledQty, result.ReceiptAuditValid, result.MissingReceipts, result.ReceiptMismatches, result.FutureReceiptUse,
					result.DecisionFieldMismatches, result.OutcomeFieldMismatches, result.MissingOutcomes, result.DuplicateOutcomes,
					result.FeeMismatches, result.SelfFills, result.NonReducingFills, result.Valid)
			})
		case "liabilityhedger":
			result, err := run.MeasureLiabilityHedger()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			if result.Decisions == 0 {
				fmt.Fprintf(os.Stderr, "%s: no L0 liability-hedger decision evidence\n", dir)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-22s decisions %6d enabled/disabled %6d/%6d updates %6d submitted/accepted/rejected/censored %6d/%6d/%6d/%6d fills %6d qty %10d receipt ok/missing/mismatch/future %t/%d/%d/%d field/outcome/missing/duplicate %d/%d/%d/%d fee/self/nonreduce %d/%d/%d valid %t\n",
					dir, result.Decisions, result.EnabledDecisions, result.DisabledDecisions, result.StateUpdates,
					result.Submitted, result.Accepted, result.Rejected, result.HorizonCensored, result.Fills, result.FilledQty,
					result.ReceiptAuditValid, result.MissingReceipts, result.ReceiptMismatches, result.FutureReceiptUse,
					result.DecisionFieldMismatches, result.OutcomeFieldMismatches, result.MissingOutcomes, result.DuplicateOutcomes,
					result.FeeMismatches, result.SelfFills, result.NonReducingFills, result.Valid)
			})
		case "noiseflowphase":
			result, err := run.MeasureNoiseFlowPhase()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			if result.Decisions == 0 {
				fmt.Fprintf(os.Stderr, "%s: no L1-P2 noise-flow phase decision evidence\n", dir)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-22s phase %d interval %d decisions %6d subscribe/evaluate %6d/%6d submitted %6d expected actors/ticks %d/%d receipt %t missing/duplicate/offphase/phase/action/extra %d/%d/%d/%d/%d/%d valid %t\n",
					dir, result.DecisionPhaseOffsetNanos, result.NoiseIntervalNanos, result.Decisions, result.SubscribeDecisions, result.EvaluateDecisions, result.SubmittedRequests,
					result.ExpectedParticipants, result.ExpectedTicks, result.ReceiptAuditValid, result.MissingTicks, result.DuplicateTicks, result.OffPhaseTicks, result.PhaseMismatches, result.ActionMismatches, result.ExtraTicks, result.Valid)
			})
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
		case "hedging":
			result, err := run.MeasureHedging(analysis.HedgingOptions{
				Symbol: *base,
				Roles:  []string{"option_dealer", "vanna_volga_desk"},
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-22s hedging in %s\n", dir, *base)
				for _, profile := range result.Profiles {
					fmt.Printf("    %-8s %-18s trades %6d  qty %14d  gap %7.1fs (spread %7.1fs)  buys %4.2f\n",
						profile.VenueID, profile.Role, profile.Trades, profile.Qty,
						profile.MedianGapSeconds, profile.GapSpreadSeconds, profile.BuyShare)
				}
			})
		case "fillpositions":
			result, err := run.MeasureFillPositions()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-22s linear fills %d, position updates %d, matched %d, missing %d, unexpected %d, chain failures %d/%d\n",
					dir, result.LinearFills, result.TradePositionUpdates, result.Matched,
					result.MissingPositionUpdate, result.UnexpectedPositionUpdate,
					result.PositionChainFailures, result.PositionChainChecks)
			})
		case "streamhash", "evidencehash":
			result, err := run.MeasureStreamHash(analysis.StreamHashOptions{PerEvent: true})
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-22s evidence events %d  unordered digest %s\n", dir, result.Events, result.Digest)
				for _, row := range result.ByVenue {
					fmt.Printf("    venue %-10s %10d  %s\n", row.Event, row.Count, row.Digest)
				}
				for _, row := range result.ByEvent {
					fmt.Printf("    %-22s %10d  %s\n", row.Event, row.Count, row.Digest)
				}
			})
		case "evidenceartifacthash":
			result, err := run.MeasureEvidenceArtifactHash()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-22s evidence artifact records %d  unordered digest %s\n", dir, result.Events, result.Digest)
			})
		case "basis":
			result, err := run.MeasureBasis(analysis.BasisOptions{})
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-22s perp basis pooled: mean %.2f bps, |mean| %.2f, sd %.2f, AR1 %.4f, half-life %.1fs (defined %t)\n",
					dir, result.PerpPooled.MeanBps, result.PerpPooled.MeanAbsBps, result.PerpPooled.StdDevBps,
					result.PerpPooled.AR1, result.PerpPooled.HalfLifeSeconds, result.PerpPooled.HalfLifeDefined)
				fmt.Printf("    dated pooled: |mean| %.2f bps, convergence slope %.4f bps/hour over %d observations\n",
					result.DatedPooled.MeanAbsBps, result.ConvergenceSlopeBpsPerHour, result.ConvergenceObservations)
				for _, bucket := range result.Convergence {
					fmt.Printf("      tte %8.0f-%8.0fs  n %7d  |basis| %8.2f bps\n",
						bucket.LowerSeconds, bucket.UpperSeconds, bucket.Observations, bucket.MeanAbsBps)
				}
			})
		case "optionsurface":
			result, err := run.MeasureOptionSurface(analysis.SurfaceOptions{QuotePrecision: *quotePrecision})
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-22s option trades %d, priced %d, unpriceable %d, fitted expiries %d\n",
					dir, result.Trades, result.Priced, result.Skipped, result.FittedExpiries)
				fmt.Printf("    pooled ATM %.4f  slope %.4f  curvature %.4f  dispersion %.4f\n",
					result.PooledATMVol, result.PooledSlope, result.PooledCurvature, result.PooledDispersion)
				for _, term := range result.TermStructure {
					fmt.Printf("      tte %.6f yr  expiries %3d  trades %7d  ATM %.4f\n",
						term.MeanTTEYears, term.Expiries, term.Trades, term.ATMVol)
				}
			})
		case "exposure":
			result, err := run.MeasureExposure(analysis.ExposureOptions{
				Roles: []string{"option_dealer"},
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-22s dealer risk samples %d; mean |net delta| %.4f, max %.4f, hedge ratio %.4f, drift %.6f/hour\n",
					dir, result.RiskSamples, result.PooledMeanAbsNetDelta, result.PooledMaxAbsNetDelta,
					result.PooledHedgeRatio, result.PooledNetDeltaDrift)
				fmt.Printf("    option-underlying volume correlation %.4f\n", result.PooledCorrelation)
				for _, flow := range result.HedgeFlows {
					fmt.Printf("      %-8s %-16s taker fills %7d (qty %14d)  option fills %7d\n",
						flow.VenueID, flow.Role, flow.TakerFills, flow.TakerVolume, flow.OptionFills)
				}
			})
		case "reaction":
			result, err := run.MeasureReaction(analysis.ReactionOptions{})
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-22s reaction lag: n %d, mean %.4fs, p50 %.4fs, min %.4fs\n",
					dir, result.Observations, result.PooledMeanSeconds, result.PooledP50Seconds, result.PooledMinSeconds)
				fmt.Printf("    maker markout pooled %.3f bps\n", result.PooledMarkoutBps)
				for _, row := range result.Adverse {
					fmt.Printf("      %-8s %-20s fills %8d  markout %8.3f bps  picked off %.3f\n",
						row.VenueID, row.Role, row.Fills, row.MeanMarkoutBps, row.PickedOffShare)
				}
			})
		case "derivatives":
			result, err := run.MeasureDerivativeSemantics(analysis.DerivativeAuditOptions{BasePrecision: *basePrecision})
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-22s funding instants %3d (residual non-zero %d, direction inconsistent %d)   option expiries %4d (payout mismatch %d, worthless paid %d, holders mispaid %d)\n",
					dir, len(result.Funding), result.FundingBroken, result.FundingSignWrong,
					len(result.Exercises), result.ExerciseBroken, result.WorthlessPaid, result.HoldersMispaid)
				for _, check := range result.Funding {
					if check.Residual == 0 && check.SignConsistent {
						continue
					}
					fmt.Printf("    funding %-8s %d  rate %4d  payers %3d receivers %3d  paid %14d received %14d  residual %10d\n",
						check.VenueID, check.Timestamp, check.Rate, check.Payers, check.Receivers,
						check.Paid, check.Received, check.Residual)
				}
				shown := 0
				for _, check := range result.Exercises {
					if check.Residual == 0 && !check.OutOfMoneyPaid {
						continue
					}
					if shown++; shown > 10 {
						break
					}
					fmt.Printf("    option  %-8s %-26s strike %10d settle %10d intrinsic %10d  holders %3d net %12d  expected %14d paid %14d residual %12d\n",
						check.VenueID, check.Symbol, check.Strike, check.SettlementPrice, check.Intrinsic,
						check.Holders, check.NetSize, check.ExpectedPayout, check.PaidOut, check.Residual)
				}
			})
		case "roleaudit":
			result, err := run.MeasureRoles(analysis.RoleAuditOptions{})
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-22s %-22s %6s %9s %9s %7s %5s %8s %8s %9s\n",
					dir, "role", "n", "makerfil", "takerfil", "taker%", "books", "top-book", "signed%", "rejected")
				for _, row := range result.Roles {
					fmt.Printf("%-22s %-22s %6d %9d %9d %6.1f%% %5d %7.2f %8.3f %9d\n",
						"", row.Role, row.Participants, row.MakerFills, row.TakerFills,
						100*row.TakerShare, row.Symbols, row.TopSymbolShare, row.SignedShare, row.Rejected)
				}
			})
		case "ecology":
			result, err := run.MeasureEcology()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-18s initial %d terminal %d HHI %.4f -> %.4f classes %d\n",
					dir, result.InitialEquity, result.TerminalEquity,
					result.InitialConcentrationHHI, result.TerminalConcentrationHHI, len(result.Roles))
			})
		case "liquidations":
			result, err := run.MeasureLiquidations()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-18s liquidations %d accounts %d checks %d deficit %d insurance residual %d balance residual %d\n",
					dir, result.Liquidations, result.AffectedAccounts, result.LiquidationChecks,
					result.TotalDeficit, result.DeficitInsuranceResidual, result.DeficitBalanceResidual)
			})
		case "marginchecks":
			result, err := run.MeasureMarginChecks(analysis.DefaultMarginCheckOptions())
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-18s eligible %d/%d marks %d active %d expected %d observed %d missing %d unexpected %d fields %d/%d exclusions %d\n",
					dir, result.EligibleCandidates, result.Candidates, result.MarkUpdates, result.ActiveMarkChecks,
					result.ExpectedBreaches, result.ObservedChecks, result.MissingChecks, result.UnexpectedChecks,
					result.FieldMismatches, result.FieldChecks, result.ExcludedCandidates)
			})
		case "arbitrage":
			result, err := run.MeasureArbitrage(analysis.ArbitrageOptions{
				TakerFeeBps:      *arbFeeBps,
				StalenessNanos:   int64(*arbStaleness * 1e9),
				BaseSymbol:       *base,
				QuoteSymbol:      *quote,
				CrossSymbol:      *cross,
				CrossPrecision:   *crossPrecision,
				CrossVenueSymbol: *base,
				PerpSymbol:       "ABC-PERP",
				SpotSymbol:       *base,
				ParityUnderlying: *base,
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-22s arbitrage at %.1f bps taker fee, %.1fs staleness\n",
					dir, result.FeeBps, float64(result.StalenessNanos)/1e9)
				for _, cycle := range result.Cycles {
					fmt.Printf("    %-24s obs %7d  domain-undefined %6d  profitable %6d (%5.2f%%)  mean %+7.2f bps  max %+8.2f bps  mean-all %+7.2f bps  longest run %6.1fs\n",
						cycle.Cycle, cycle.Observations, cycle.UndefinedDomainObservations, cycle.Profitable, 100*cycle.ProfitableShare,
						cycle.MeanEdgeBps, cycle.MaxEdgeBps, cycle.MeanAllBps,
						float64(cycle.LongestRunNanos)/1e9)
				}
			})
		case "crossvenue":
			result, err := run.MeasureCrossVenueDispersion(analysis.CrossVenueDispersionOptions{
				Symbol: *crossVenueSymbol, StalenessNanos: int64(*arbStaleness * 1e9), MinVenues: *crossVenueMin,
				CapturePositiveObservationTimes: *crossVenuePositiveTimes,
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			if result.Evaluated == 0 {
				fmt.Fprintf(os.Stderr, "%s: no fresh %d-venue two-sided observations for %s\n", dir, *crossVenueMin, *crossVenueSymbol)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-20s observations %6d  range bps mean %7.3f med %7.3f p90 %7.3f max %7.3f  longest-positive %6.2fs\n",
					dir, result.Evaluated, result.MidpointRangeBps.Mean, result.MidpointRangeBps.Median,
					result.MidpointRangeBps.P90, result.MidpointRangeBps.Max,
					float64(result.LongestPositiveRunNanos)/1e9)
			})
		case "settlements":
			result, err := run.MeasureSettlements(analysis.SettlementAuditOptions{BasePrecision: *basePrecision})
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-22s settlements %3d  payout mismatches %d  holder/payee mismatches %d  fills after expiry %d\n",
					dir, len(result.Checks), result.Mismatched, result.Unpaid, result.TotalTradesAfterExpiry)
				for _, check := range result.Checks {
					if check.Residual == 0 && check.PaidAccounts == check.Holders && check.TradesAfterExpiry == 0 {
						continue
					}
					fmt.Printf("    %-8s %-26s settle %10d  holders %3d/%3d  net %12d  expected %14d paid %14d residual %12d  after-expiry fills %d\n",
						check.VenueID, check.Symbol, check.SettlementPrice, check.PaidAccounts, check.Holders,
						check.NetSize, check.ExpectedPayout, check.PaidOut, check.Residual, check.TradesAfterExpiry)
				}
			})
		case "expiryfills":
			result, err := run.MeasureExpiryFills()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-22s contracts %d (expired %d, settled %d, futures %d options %d), after-expiry fills %d, post-expiry snapshots %d (nonempty %d), expired-unsettled %d, metadata defects %d\n",
					dir, result.Contracts, result.ExpiredContracts, result.SettledContracts,
					result.Futures, result.Options, result.FillsAfterExpiry,
					result.SnapshotRecordsAfterExpiry, result.NonEmptySnapshotsAfterExpiry,
					result.ExpiredUnsettledContracts, result.MissingExpiryMetadata+result.SettlementWithoutListing+result.MetadataMismatches)
			})
		case "orderlifecycle":
			result, err := run.MeasureOrderLifecycle()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-22s accepted %d immediate-required %d missing-terminal %d fills-after-terminal %d quantity-mismatches %d/%d\n",
					dir, result.Accepted, result.RequiredImmediateTerminal, result.MissingImmediateTerminal,
					result.FillsAfterTerminal, result.FillQuantityMismatches, result.CancelQuantityMismatches)
			})
		case "positions":
			result, err := run.MeasurePositions(analysis.PositionOptions{BasePrecision: *basePrecision})
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-22s contracts %4d  net-non-zero %d  open linear value %16d (reported %16d, gap %12d)\n",
					dir, len(result.Contracts), result.NonZeroNetContracts,
					result.OpenLinearValue, result.ReportedLinearValue, result.Disagreement)
				for _, contract := range result.Contracts {
					if contract.NetSize == 0 {
						continue
					}
					fmt.Printf("    %-8s %-28s net %14d gross %14d holders %d\n",
						contract.VenueID, contract.Symbol, contract.NetSize, contract.GrossSize, contract.Holders)
				}
			})
		case "conservation":
			// Restricting to one book localises a residual to the mechanism
			// that produced it: a spot book's cash legs must cancel against
			// its fees exactly, while a derivative book's need not.
			opts := analysis.ConservationOptions{}
			if *conservationBook != "" {
				opts.Files = run.BookFiles(*venue, *conservationBook)
				opts.FilesSelected = true
			}
			result, err := run.MeasureConservation(opts)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-22s balance deltas checked %d, self-inconsistent %d (worst %d); chain links %d, broken %d (worst %d); decode failures %d\n",
					dir, result.Deltas.Checked, result.Deltas.Mismatched, result.Deltas.WorstGap,
					result.Deltas.ChainChecked, result.Deltas.ChainBroken, result.Deltas.WorstChain,
					result.Deltas.DecodeFailures)
				for _, flow := range result.Flows {
					fmt.Printf("    %-20s %-5s credits %18d  debits %18d  net %14d  (%d records)\n",
						flow.Reason, flow.Asset, flow.Credits, flow.Debits, flow.Net, flow.Records)
				}
				if worst, ok := analysis.WorstResidual(result.FundingInstants); ok {
					fmt.Printf("    funding: %d instants, worst residual %d at %s %d\n",
						len(result.FundingInstants), worst.Net, worst.VenueID, worst.Timestamp)
				}
				if worst, ok := analysis.WorstResidual(result.ExpiryInstants); ok {
					// Not an error: a settlement pays each holder against its
					// own entry price, so the instant nets to zero only if
					// every holder entered at the same price.
					fmt.Printf("    expiry:  %d instants, largest net %d at %s %d (not required to be zero)\n",
						len(result.ExpiryInstants), worst.Net, worst.VenueID, worst.Timestamp)
				}
				// A book-restricted audit sees only that book's movements,
				// while the exchange take and open positions are the whole
				// run's, so the identity is not closable and is not printed.
				if *conservationBook != "" {
					return
				}
				for _, identity := range result.VenueIdentities {
					if identity.Residual == 0 {
						continue
					}
					fmt.Printf("    venue %-8s %-4s residual %14d  reasons %v\n",
						identity.VenueID, identity.Asset, identity.Residual, identity.ByReason)
				}
				fmt.Printf("    fees on the event stream: %v\n", result.FeesLogged)
				if worst, ok := analysis.WorstResidual(result.OptionExpiryInstants); ok {
					fmt.Printf("    option expiry: %d instants, worst net %d at %s %d (must be zero: payoff does not depend on entry price)\n",
						len(result.OptionExpiryInstants), worst.Net, worst.VenueID, worst.Timestamp)
				}
				for _, identity := range result.Identities {
					recorded, hasStream := result.VenueRecorded[identity.Asset]
					if hasStream {
						if gap := identity.ExchangeTake - recorded; gap != 0 {
							fmt.Printf("    venue take for %-4s disagrees with its own movement stream by %14d\n",
								identity.Asset, gap)
						} else {
							fmt.Printf("    venue take for %-4s reconstructs exactly from its movement stream (%d)\n",
								identity.Asset, recorded)
						}
						continue
					}
					if gap := identity.ExchangeTake - result.FeesLogged[identity.Asset]; gap != 0 {
						fmt.Printf("    venue take for %-4s exceeds the fee stream by %14d — revenue taken without an event (no movement stream in this run)\n",
							identity.Asset, gap)
					}
				}
				for _, class := range sortedRuleNames(map[string]int{"spot": 0, "perp": 0, "dated": 0, "option": 0, "none": 0}) {
					if net := result.ClassNet[class]; net != nil {
						fmt.Printf("    class %-7s net %v  (%d records)\n", class, net, result.ClassRecords[class])
					}
				}
				for _, identity := range result.Identities {
					fmt.Printf("    identity %-4s external %20d  internal %18d  exchange %14d  open %16d  residual %12d (%.2e)\n",
						identity.Asset, identity.ExternalIn, identity.InternalNet, identity.ExchangeTake,
						identity.OpenLinearValue, identity.Residual, identity.ResidualRelative)
				}
			})
		case "lifecycle":
			result, err := run.MeasureLifecycle(analysis.LifecycleOptions{})
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
				os.Exit(1)
			}
			emit(dir, result, *asJSON, func() {
				fmt.Printf("%-22s listings %v  settlements %v\n", dir, result.Listings, result.Settlements)
				for _, venue := range sortedRuleNames(result.SettlementRoundsByVenue) {
					fmt.Printf("    %-9s %2d listing rounds, %2d settlement rounds\n",
						venue, result.ListingRoundsByVenue[venue], result.SettlementRoundsByVenue[venue])
				}
				for _, schedule := range result.Funding {
					fmt.Printf("    funding %-9s %3d settlements, period %6.0fs\n",
						schedule.VenueID, schedule.Settlements, schedule.PeriodSeconds)
				}
				for venues := len(result.Funding); venues >= 2; venues-- {
					if instants := result.FundingIntersections[venues]; instants > 0 {
						fmt.Printf("    %d venues settled together at %d instants\n", venues, instants)
					}
				}
			})
		case "viability":
			classes, err := loadViabilityClasses(*viabilityThresholds)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(2)
			}
			thresholdsFor := func(symbol string) viabilityClass {
				for _, class := range classes {
					if matchSymbol(class.Pattern, symbol) {
						return class
					}
				}
				return viabilityClass{
					MinTrades:         *minTradesPerWindow,
					MinTakerClasses:   *minTakerClasses,
					MinMakerClasses:   *minMakerClasses,
					MaxRoleShare:      *maxRoleShare,
					MaxEmptySideShare: *maxEmptySideShare,
					MaxSpreadTicks:    *maxSpreadTicks,
					MinTouchDepth:     *minTouchDepth,
				}
			}
			// The rules are assembled here, in the adapter, because what counts
			// as a living market is the caller's judgement and not a property
			// of the measurement. The library measures; the thresholds are
			// configuration.
			// A rule is only asked about a window that covers a whole slice of
			// the book's life. The listing and settlement windows are partial,
			// and judging them reports every expiry as a market that died.
			judged := func(rule analysis.ViabilityRule) analysis.ViabilityRule {
				inner := rule.Breached
				return analysis.ViabilityRule{Name: rule.Name, Breached: func(w analysis.MarketWindow) bool {
					if !*judgeLifeEdges && (w.FirstForBook || w.LastForBook) {
						return false
					}
					return inner(w)
				}}
			}
			rules := []analysis.ViabilityRule{
				{Name: "thin_volume", Breached: func(w analysis.MarketWindow) bool {
					return w.Trades < thresholdsFor(w.Symbol).MinTrades
				}},
				{Name: "few_taker_classes", Breached: func(w analysis.MarketWindow) bool {
					return w.TakerRoles < thresholdsFor(w.Symbol).MinTakerClasses
				}},
				{Name: "few_maker_classes", Breached: func(w analysis.MarketWindow) bool {
					return w.MakerRoles < thresholdsFor(w.Symbol).MinMakerClasses
				}},
				// A single one-sided publication is a book between requotes,
				// not a dead market. What matters is the share of the window a
				// taker had nothing to trade against.
				{Name: "one_sided_book", Breached: func(w analysis.MarketWindow) bool {
					if w.Snapshots == 0 {
						return false
					}
					return float64(w.EmptySideSnapshots)/float64(w.Snapshots) > thresholdsFor(w.Symbol).MaxEmptySideShare
				}},
				{Name: "concentrated_flow", Breached: func(w analysis.MarketWindow) bool {
					return w.TopRoleVolumeShare > thresholdsFor(w.Symbol).MaxRoleShare
				}},
				{Name: "wide_spread", Breached: func(w analysis.MarketWindow) bool {
					limit := thresholdsFor(w.Symbol).MaxSpreadTicks
					return limit > 0 && w.SpreadTicks.N > 0 && w.SpreadTicks.Median > limit
				}},
				{Name: "thin_depth", Breached: func(w analysis.MarketWindow) bool {
					floor := thresholdsFor(w.Symbol).MinTouchDepth
					return floor > 0 && w.TouchDepth.N > 0 && w.TouchDepth.Median < floor
				}},
			}
			for i, rule := range rules {
				rules[i] = judged(rule)
			}
			result, err := run.MeasureViability(analysis.ViabilityOptions{
				WindowNanos: int64(*viabilityWindow * 1e9),
				StartNanos:  int64(*viabilityStart * 1e9),
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
				alive, died, neverAlive := 0, 0, 0
				for _, book := range result.BookSummaries {
					switch {
					case book.Viable == book.Windows:
						alive++
					case book.Viable == 0:
						neverAlive++
					default:
						died++
					}
				}
				fmt.Printf("    books viable throughout %3d  partly viable %3d  never viable %3d\n",
					alive, died, neverAlive)
				for _, book := range result.BookSummaries {
					if book.Viable == book.Windows || book.Viable == 0 {
						continue
					}
					fmt.Printf("    %-10s %-28s viable %2d/%2d  last viable window %2d  trades %8d\n",
						book.VenueID, book.Symbol, book.Viable, book.Windows, book.LastViableWindow, book.Trades)
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

// emitStandaloneEvidenceMetric handles compact evidence artifacts without
// constructing analysis.Run, whose raw-log and report prerequisites are
// intentionally unrelated to the evidence contract. The bool reports whether
// metric was one of those standalone contracts.
func emitStandaloneEvidenceMetric(metric, dir string, asJSON bool) (bool, error) {
	switch metric {
	case "observationreceipts":
		result, err := analysis.AuditMarketDataReceipts(dir)
		if err != nil {
			return true, err
		}
		emit(dir, result, asJSON, func() {
			fmt.Printf("%-22s schedules/receipts/decisions %d/%d/%d digests %t/%t/%t valid %t future %d/%d missing %d frontier %d\n",
				dir, result.Schedules, result.Receipts, result.Decisions,
				result.ScheduleDigestMatches, result.ReceiptDigestMatches, result.DecisionDigestMatches, result.Valid,
				result.ScheduledBeforePub, result.FutureDecisionUse, result.MissingDueReceipt, result.BadDecisionFrontier)
		})
		return true, nil
	case "frontiervectors":
		result, err := analysis.AuditDecisionFrontierVectors(dir)
		if err != nil {
			return true, err
		}
		emit(dir, result, asJSON, func() {
			fmt.Printf("%-22s vector decisions/components %d/%d base %t valid %t future %d missing-components %d missing-vectors %d\n",
				dir, result.Decisions, result.Components, result.BaseEvidenceValid, result.Valid,
				result.FutureComponentUse, result.MissingDecisionComponents, result.MissingVectorDecision)
		})
		return true, nil
	default:
		return false, nil
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

// viabilityClass is one book pattern's corridor. A chain that trades once an
// hour is healthy and a spot book that does is dead, so a single set of
// thresholds across every instrument answers the wrong question for most of
// them.
type viabilityClass struct {
	Pattern           string  `json:"pattern"`
	MinTrades         int     `json:"min_trades"`
	MinTakerClasses   int     `json:"min_taker_classes"`
	MinMakerClasses   int     `json:"min_maker_classes"`
	MaxRoleShare      float64 `json:"max_role_share"`
	MaxEmptySideShare float64 `json:"max_empty_side_share"`
	MaxSpreadTicks    float64 `json:"max_spread_ticks"`
	MinTouchDepth     float64 `json:"min_touch_depth"`
}

// loadViabilityClasses reads the per-book thresholds, which are matched in
// file order so a specific pattern can precede a general one.
func loadViabilityClasses(path string) ([]viabilityClass, error) {
	if path == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read viability thresholds: %w", err)
	}
	var classes []viabilityClass
	if err := json.Unmarshal(raw, &classes); err != nil {
		return nil, fmt.Errorf("decode viability thresholds: %w", err)
	}
	for _, class := range classes {
		if class.Pattern == "" {
			return nil, fmt.Errorf("viability thresholds: every entry needs a symbol pattern")
		}
		if strings.Count(class.Pattern, "*") > 8 {
			return nil, fmt.Errorf("viability thresholds: pattern %q has too many wildcards to match cheaply", class.Pattern)
		}
	}
	return classes, nil
}

// matchSymbol matches a book pattern against a symbol.
//
// It is written out rather than delegating to filepath.Match because a symbol
// is not a path: "ABC/USD" contains a separator, and filepath's "*" refuses to
// cross one, so the catch-all pattern silently failed to match every spot book
// and those books were judged against whatever the flags happened to say.
func matchSymbol(pattern, symbol string) bool {
	if pattern == "*" {
		return true
	}
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == symbol
	}
	if !strings.HasPrefix(symbol, parts[0]) {
		return false
	}
	rest := symbol[len(parts[0]):]
	for i := 1; i < len(parts)-1; i++ {
		index := strings.Index(rest, parts[i])
		if index < 0 {
			return false
		}
		rest = rest[index+len(parts[i]):]
	}
	return strings.HasSuffix(rest, parts[len(parts)-1])
}
