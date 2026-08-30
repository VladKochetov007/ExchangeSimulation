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
// values come from the shared constructors in metricoptions.go, which the
// single-metric switch also uses, so the two paths cannot silently diverge on
// configuration.

// fusedMetric computes one metric and returns the value that is serialized
// under "result", or an error that reproduces the single-metric failure.
type fusedMetric func(*analysis.Run, metricSettings) (any, error)

func fusedMetrics() map[string]fusedMetric {
	return map[string]fusedMetric{
		"conservation": func(run *analysis.Run, _ metricSettings) (any, error) {
			return run.MeasureConservation(analysis.ConservationOptions{})
		},
		"positions": func(run *analysis.Run, s metricSettings) (any, error) {
			return run.MeasurePositions(positionOptions(s))
		},
		"fillpositions": func(run *analysis.Run, _ metricSettings) (any, error) {
			return run.MeasureFillPositions()
		},
		"orderlifecycle": func(run *analysis.Run, _ metricSettings) (any, error) {
			return run.MeasureOrderLifecycle()
		},
		"lifecycle": func(run *analysis.Run, _ metricSettings) (any, error) {
			return run.MeasureLifecycle(analysis.LifecycleOptions{})
		},
		"settlements": func(run *analysis.Run, s metricSettings) (any, error) {
			return run.MeasureSettlements(settlementOptions(s))
		},
		"expiryfills": func(run *analysis.Run, _ metricSettings) (any, error) {
			return run.MeasureExpiryFills()
		},
		"streamhash": func(run *analysis.Run, _ metricSettings) (any, error) {
			return run.MeasureStreamHash(analysis.StreamHashOptions{PerEvent: true})
		},
		"arbitrage": func(run *analysis.Run, s metricSettings) (any, error) {
			return run.MeasureArbitrage(arbitrageOptions(s))
		},
		"crossvenue": func(run *analysis.Run, s metricSettings) (any, error) {
			result, err := run.MeasureCrossVenueDispersion(crossVenueOptions(s))
			if err != nil {
				return nil, err
			}
			if result.Evaluated == 0 {
				return nil, fmt.Errorf("no fresh %d-venue two-sided observations for %s",
					s.crossVenueMin, s.crossVenueSymbol)
			}
			return result, nil
		},
		"roleaudit": func(run *analysis.Run, _ metricSettings) (any, error) {
			return run.MeasureRoles(analysis.RoleAuditOptions{})
		},
		"derivatives": func(run *analysis.Run, s metricSettings) (any, error) {
			return run.MeasureDerivativeSemantics(derivativeOptions(s, s.fundingIntervals))
		},
		"liquidations": func(run *analysis.Run, _ metricSettings) (any, error) {
			return run.MeasureLiquidations()
		},
		"marginchecks": func(run *analysis.Run, _ metricSettings) (any, error) {
			return run.MeasureMarginChecks(analysis.DefaultMarginCheckOptions())
		},
		"optionsurface": func(run *analysis.Run, s metricSettings) (any, error) {
			return run.MeasureOptionSurface(optionSurfaceOptions(s))
		},
		"exposure": func(run *analysis.Run, s metricSettings) (any, error) {
			return run.MeasureExposure(exposureOptions(s))
		},
		"hedging": func(run *analysis.Run, s metricSettings) (any, error) {
			return run.MeasureHedging(hedgingOptions(s))
		},
		"postonly": func(run *analysis.Run, s metricSettings) (any, error) {
			result, err := run.MeasurePostOnlyActivity(postOnlyOptions(s))
			if err != nil {
				return nil, err
			}
			if result.Events == 0 {
				return nil, fmt.Errorf("no selected post-only evidence events")
			}
			return result, nil
		},
		"perpsignals": func(run *analysis.Run, s metricSettings) (any, error) {
			return run.MeasurePerpSignals(perpSignalOptions(s))
		},
	}
}

// runFusedExtraction computes the selected metrics over shared physical passes
// and writes one artifact per metric.
func runFusedExtraction(run *analysis.Run, dir, outDir, set string, workers int, settings metricSettings) error {
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
