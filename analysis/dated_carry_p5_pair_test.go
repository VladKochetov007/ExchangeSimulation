package analysis

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestP5BasisWindowUsesWholeMinutesCoverageAndDirection(t *testing.T) {
	const minute = p5BasisMinuteNanos
	t0 := int64(65_000_000_000)
	expiry := int64(20)*minute + p5BasisTailNanos
	series := map[p5BasisKey][]p4Quote{
		{"north", "ABC/USD"}: {},
		{"north", "ABC-FUT"}: {},
	}
	for at := int64(2) * minute; at < expiry-p5BasisTailNanos; at += minute {
		series[p5BasisKey{"north", "ABC/USD"}] = append(series[p5BasisKey{"north", "ABC/USD"}], p4Quote{at: at, mid: 100})
		series[p5BasisKey{"north", "ABC-FUT"}] = append(series[p5BasisKey{"north", "ABC-FUT"}], p4Quote{at: at, mid: 102})
	}
	window := measureP5BasisWindow(series, "north", "ABC-FUT", t0, expiry, 1)
	if !window.Measurable || window.ExpectedSamples != 18 || window.ObservedSamples != 18 || window.MeanExact != "200" || window.MeanBps == nil || *window.MeanBps != 200 {
		t.Fatalf("registered rich-future basis = %+v", window)
	}
	reversed := measureP5BasisWindow(series, "north", "ABC-FUT", t0, expiry, -1)
	if !reversed.Measurable || reversed.MeanExact != "-200" {
		t.Fatalf("registered orientation = %+v", reversed)
	}
}

func TestP5BasisWindowFailsClosedForStaleMissingAndZeroSpot(t *testing.T) {
	const minute = p5BasisMinuteNanos
	t0, expiry := int64(2)*minute, int64(13)*minute+p5BasisTailNanos
	series := map[p5BasisKey][]p4Quote{
		{"north", "ABC/USD"}: {{at: t0 - p5BasisMaxAgeNanos - 1, mid: 100}},
		{"north", "ABC-FUT"}: {{at: t0, mid: 102}},
	}
	stale := measureP5BasisWindow(series, "north", "ABC-FUT", t0, expiry, 1)
	if stale.Measurable || stale.MeanBps != nil || stale.StaleOrMissing == 0 {
		t.Fatalf("stale evidence became a statistic: %+v", stale)
	}
	series[p5BasisKey{"north", "ABC/USD"}] = []p4Quote{{at: t0, mid: 0}}
	zero := measureP5BasisWindow(series, "north", "ABC-FUT", t0, expiry, 1)
	if zero.Measurable || zero.MeanBps != nil || zero.UndefinedSpot == 0 {
		t.Fatalf("zero denominator became a ratio: %+v", zero)
	}
}

func TestP5PairConfigRequiresTradePermissionAsSoleDelta(t *testing.T) {
	write := func(name string, trade bool, mutate func(map[string]any)) string {
		t.Helper()
		dir := t.TempDir()
		config := map[string]any{
			"seed":                       float64(117),
			"dated_term_carry_allocator": map[string]any{"enabled": true, "trade_enabled": trade, "lot_qty": float64(10)},
			"snapshot_interval":          float64(1),
			"experiment_id":              name,
			"description":                name,
		}
		if mutate != nil {
			mutate(config)
		}
		raw, err := json.Marshal(map[string]any{"build": map[string]any{"revision": "same-revision"}, "config": config})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0o600); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	a := write("A", false, nil)
	b := write("B", true, nil)
	valid, sameRevision, err := p5ComparableConfigs(a, b)
	if err != nil || !valid || !sameRevision {
		t.Fatalf("registered sole delta = %t/%t, %v", valid, sameRevision, err)
	}
	c := write("B", true, func(config map[string]any) { config["snapshot_interval"] = float64(2) })
	valid, sameRevision, err = p5ComparableConfigs(a, c)
	if err != nil || valid {
		t.Fatalf("economic/config mutation survived = %t, %v", valid, err)
	}
}
