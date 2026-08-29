package analysis

import (
	"fmt"
	"os"
	"sync/atomic"
)

// Scan instrumentation is an offline measurement aid. It counts the physical
// work an extraction performs so multi-pass cost can be quantified without
// guessing from call sites. It never changes scan results, ordering, or
// failure behavior, and it is inert unless MVANALYZE_SCAN_STATS is set.
var (
	scanStatsEnabled = os.Getenv("MVANALYZE_SCAN_STATS") != ""

	scanCalls     atomic.Int64
	scanFiles     atomic.Int64
	scanBytes     atomic.Int64
	scanLines     atomic.Int64
	scanPrefilter atomic.Int64
	scanEnvelopes atomic.Int64
	scanVisits    atomic.Int64
)

// ReportScanStats writes accumulated scan counters to stderr when
// instrumentation is enabled.
func ReportScanStats() {
	if !scanStatsEnabled {
		return
	}
	fmt.Fprintf(os.Stderr,
		"scanstats calls=%d files=%d bytes=%d lines=%d prefiltered_out=%d envelopes_decoded=%d visits=%d\n",
		scanCalls.Load(), scanFiles.Load(), scanBytes.Load(), scanLines.Load(),
		scanPrefilter.Load(), scanEnvelopes.Load(), scanVisits.Load())
}
