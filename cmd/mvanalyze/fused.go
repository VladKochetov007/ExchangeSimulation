package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"exchange_sim/analysis"
)

// Fused extraction is a research mode, not a replacement for the registered
// one-process-per-metric extraction.
//
// It exists to measure what the shared-envelope architecture is worth: the
// registered set re-reads and re-prefilters the same evidence about twenty-six
// times, and this mode collapses that into one physical pass per scan round
// while leaving every metric's own reducer, options and verdict untouched.
//
// Each metric writes exactly the bytes the single-metric path writes for the
// same run, which is the property the differential harness checks. The option
// values come from the same flag variables as the single-metric switch, so the
// two paths cannot silently diverge on configuration.

// fusedSettings carries the flag-derived options the fused metric set needs.
type fusedSettings struct {
	basePrecision           int64
	quotePrecision          int64
	requireExactReplay      bool
	deliveryFeePolicy       string
	fundingIntervalSeconds  int64
	fundingIntervals        map[string]int64
	arbFeeBps               float64
	arbStaleness            float64
	base                    string
	quote                   string
	cross                   string
	crossPrecision          int64
	crossVenueSymbol        string
	crossVenueMin           int
	crossVenuePositiveTimes bool
	perpSignalSymbol        string
	perpSignalVenues        string
	postOnlyRoles           string
	postOnlySymbols         string
	hedgeSymbol             string
}

// fusedMetric computes one metric and returns the value that is serialized
// under "result", or an error that reproduces the single-metric failure.
type fusedMetric func(*analysis.Run, fusedSettings) (any, error)

func fusedMetrics() map[string]fusedMetric {
	return map[string]fusedMetric{
		"conservation": func(run *analysis.Run, _ fusedSettings) (any, error) {
			return run.MeasureConservation(analysis.ConservationOptions{})
		},
		"positions": func(run *analysis.Run, s fusedSettings) (any, error) {
			return run.MeasurePositions(analysis.PositionOptions{
				BasePrecision: s.basePrecision, RequireExactReplay: s.requireExactReplay})
		},
		"fillpositions": func(run *analysis.Run, _ fusedSettings) (any, error) {
			return run.MeasureFillPositions()
		},
		"orderlifecycle": func(run *analysis.Run, _ fusedSettings) (any, error) {
			return run.MeasureOrderLifecycle()
		},
		"lifecycle": func(run *analysis.Run, _ fusedSettings) (any, error) {
			return run.MeasureLifecycle(analysis.LifecycleOptions{})
		},
		"settlements": func(run *analysis.Run, s fusedSettings) (any, error) {
			return run.MeasureSettlements(analysis.SettlementAuditOptions{
				BasePrecision: s.basePrecision, RequireExactReplay: s.requireExactReplay,
				DeliveryFeePolicy: s.deliveryFeePolicy})
		},
		"expiryfills": func(run *analysis.Run, _ fusedSettings) (any, error) {
			return run.MeasureExpiryFills()
		},
		"streamhash": func(run *analysis.Run, _ fusedSettings) (any, error) {
			return run.MeasureStreamHash(analysis.StreamHashOptions{PerEvent: true})
		},
		"arbitrage": func(run *analysis.Run, s fusedSettings) (any, error) {
			return run.MeasureArbitrage(analysis.ArbitrageOptions{
				TakerFeeBps:      s.arbFeeBps,
				StalenessNanos:   int64(s.arbStaleness * 1e9),
				BaseSymbol:       s.base,
				QuoteSymbol:      s.quote,
				CrossSymbol:      s.cross,
				CrossPrecision:   s.crossPrecision,
				CrossVenueSymbol: s.base,
				PerpSymbol:       "ABC-PERP",
				SpotSymbol:       s.base,
				ParityUnderlying: s.base,
			})
		},
		"crossvenue": func(run *analysis.Run, s fusedSettings) (any, error) {
			result, err := run.MeasureCrossVenueDispersion(analysis.CrossVenueDispersionOptions{
				Symbol: s.crossVenueSymbol, StalenessNanos: int64(s.arbStaleness * 1e9),
				MinVenues: s.crossVenueMin, CapturePositiveObservationTimes: s.crossVenuePositiveTimes})
			if err != nil {
				return nil, err
			}
			if result.Evaluated == 0 {
				return nil, fmt.Errorf("no fresh %d-venue two-sided observations for %s",
					s.crossVenueMin, s.crossVenueSymbol)
			}
			return result, nil
		},
		"roleaudit": func(run *analysis.Run, _ fusedSettings) (any, error) {
			return run.MeasureRoles(analysis.RoleAuditOptions{})
		},
		"derivatives": func(run *analysis.Run, s fusedSettings) (any, error) {
			return run.MeasureDerivativeSemantics(analysis.DerivativeAuditOptions{
				BasePrecision: s.basePrecision, RequireExactReplay: s.requireExactReplay,
				ExpectedFundingIntervalSeconds: s.fundingIntervalSeconds,
				ExpectedFundingIntervals:       s.fundingIntervals})
		},
		"liquidations": func(run *analysis.Run, _ fusedSettings) (any, error) {
			return run.MeasureLiquidations()
		},
		"marginchecks": func(run *analysis.Run, _ fusedSettings) (any, error) {
			return run.MeasureMarginChecks(analysis.DefaultMarginCheckOptions())
		},
		"optionsurface": func(run *analysis.Run, s fusedSettings) (any, error) {
			return run.MeasureOptionSurface(analysis.SurfaceOptions{QuotePrecision: s.quotePrecision})
		},
		"exposure": func(run *analysis.Run, _ fusedSettings) (any, error) {
			return run.MeasureExposure(analysis.ExposureOptions{Roles: []string{"option_dealer"}})
		},
		"hedging": func(run *analysis.Run, s fusedSettings) (any, error) {
			return run.MeasureHedging(analysis.HedgingOptions{
				Symbol: s.hedgeSymbol, Roles: []string{"option_dealer", "vanna_volga_desk"}})
		},
		"postonly": func(run *analysis.Run, s fusedSettings) (any, error) {
			result, err := run.MeasurePostOnlyActivity(analysis.PostOnlyActivityOptions{
				Roles:   strings.Split(s.postOnlyRoles, ","),
				Symbols: strings.Split(s.postOnlySymbols, ","),
			})
			if err != nil {
				return nil, err
			}
			if result.Events == 0 {
				return nil, fmt.Errorf("no selected post-only evidence events")
			}
			return result, nil
		},
		"perpsignals": func(run *analysis.Run, s fusedSettings) (any, error) {
			return run.MeasurePerpSignals(analysis.PerpSignalOptions{
				Symbol: s.perpSignalSymbol, RequiredVenues: splitNonEmpty(s.perpSignalVenues)})
		},
	}
}

// runFusedExtraction computes the selected metrics over shared physical passes
// and writes one artifact per metric.
func runFusedExtraction(run *analysis.Run, dir, outDir, set string, workers int, settings fusedSettings) error {
	available := fusedMetrics()
	var names []string
	if set == "" {
		for name := range available {
			names = append(names, name)
		}
		sort.Strings(names)
	} else {
		names = splitNonEmpty(set)
	}
	tasks := make([]analysis.FusedTask, 0, len(names))
	results := make([]any, len(names))
	for i, name := range names {
		compute, known := available[name]
		if !known {
			return fmt.Errorf("metric %q is not available in fused mode", name)
		}
		index := i
		metric := compute
		tasks = append(tasks, analysis.FusedTask{Name: name, Compute: func(r *analysis.Run) error {
			value, err := metric(r, settings)
			if err != nil {
				return err
			}
			results[index] = value
			return nil
		}})
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	errs := run.RunFused(tasks, workers)
	var failed []string
	for i, name := range names {
		if errs[i] != nil {
			// Reproduce the single-metric stderr line, and record the failure
			// as the extraction script's write_metric would: no artifact.
			fmt.Fprintf(os.Stderr, "%s: %v\n", dir, errs[i])
			if err := os.WriteFile(filepath.Join(outDir, name+".err"),
				[]byte(fmt.Sprintf("%s: %v\n", dir, errs[i])), 0o644); err != nil {
				return err
			}
			failed = append(failed, name)
			continue
		}
		raw, err := json.Marshal(map[string]any{"run": dir, "result": results[i]})
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(outDir, name+".json"),
			append(raw, '\n'), 0o644); err != nil {
			return err
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("metrics failed: %s", strings.Join(failed, ","))
	}
	return nil
}
