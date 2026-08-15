// greekreport summarizes a multi-venue Greek report by actual option listing
// generation. It is deliberately position-only: a dealer's aggregate hedge
// cannot be allocated to individual expiries without an explicit hedge policy.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"time"

	"exchange_sim/simulations/derivsim"
)

type multiVenueReport struct {
	Venues map[string]derivsim.GreekReport `json:"venues"`
}

type optionRisk struct {
	Timestamp        int64   `json:"timestamp"`
	TimeToExpiryNano int64   `json:"time_to_expiry_nano"`
	OptionDelta      float64 `json:"option_delta"`
	Gamma            float64 `json:"gamma"`
	Vega             float64 `json:"vega"`
	Positions        int     `json:"positions"`
}

// maturityRisk is an aggregate across contracts that share a remaining-time
// band but not necessarily one exact expiry. Its min/max fields prevent a
// mixed-expiry row from inheriting a misleading time to expiry from the first
// contract processed.
type maturityRisk struct {
	Timestamp           int64   `json:"timestamp"`
	MinTimeToExpiryNano int64   `json:"min_time_to_expiry_nano"`
	MaxTimeToExpiryNano int64   `json:"max_time_to_expiry_nano"`
	OptionDelta         float64 `json:"option_delta"`
	Gamma               float64 `json:"gamma"`
	Vega                float64 `json:"vega"`
	Positions           int     `json:"positions"`
}

// TenorSummary aggregates a single rolling option generation, identified by
// its exchange listing timestamp and expiry. Means are equally weighted over
// observed sampling timestamps, not volume- or time-weighted PnL measures.
type TenorSummary struct {
	VenueID          string     `json:"venue_id"`
	ListedNano       int64      `json:"listed_nano"`
	ExpiryNano       int64      `json:"expiry_nano"`
	ListingTenorNano int64      `json:"listing_tenor_nano"`
	Samples          int        `json:"samples"`
	Symbols          int        `json:"symbols"`
	First            optionRisk `json:"first"`
	LastPreExpiry    optionRisk `json:"last_pre_expiry"`
	MaxAbsDelta      float64    `json:"max_abs_option_delta"`
	MeanAbsDelta     float64    `json:"mean_abs_option_delta"`
	MaxAbsGamma      float64    `json:"max_abs_gamma"`
	MeanAbsGamma     float64    `json:"mean_abs_gamma"`
	MaxAbsVega       float64    `json:"max_abs_vega"`
	MeanAbsVega      float64    `json:"mean_abs_vega"`
}

type analysis struct {
	SchemaVersion       int                     `json:"schema_version"`
	Tenors              []TenorSummary          `json:"tenors"`
	RemainingMaturities []RemainingTenorSummary `json:"remaining_maturities"`
	Caveats             []string                `json:"caveats"`
}

// MaturityBand classifies risk by remaining lifetime, which differs from the
// original listing generation once an option ages toward expiry.
type MaturityBand struct {
	Name     string `json:"name"`
	MinNanos int64  `json:"min_nanos"` // inclusive
	MaxNanos int64  `json:"max_nanos"` // inclusive; zero means unbounded
}

// RemainingTenorSummary measures a live maturity band. It does not include
// dealer hedges because those are held at portfolio level and cannot be
// attributed to one expiry without an explicit allocation policy.
type RemainingTenorSummary struct {
	VenueID      string       `json:"venue_id"`
	Band         MaturityBand `json:"band"`
	Samples      int          `json:"samples"`
	First        maturityRisk `json:"first"`
	Last         maturityRisk `json:"last"`
	MaxAbsDelta  float64      `json:"max_abs_option_delta"`
	MeanAbsDelta float64      `json:"mean_abs_option_delta"`
	MaxAbsGamma  float64      `json:"max_abs_gamma"`
	MeanAbsGamma float64      `json:"mean_abs_gamma"`
	MaxAbsVega   float64      `json:"max_abs_vega"`
	MeanAbsVega  float64      `json:"mean_abs_vega"`
}

type bucket struct {
	venue       string
	listing     int64
	expiry      int64
	byTimestamp map[int64]*optionRisk
	symbols     map[string]struct{}
}

func main() {
	input := flag.String("input", "greeks.json", "multi-venue greeks JSON")
	output := flag.String("output", "", "analysis JSON; default stdout")
	shortMax := flag.Duration("short-max", 6*time.Hour, "maximum remaining maturity for the short bucket")
	longMin := flag.Duration("long-min", 24*time.Hour, "minimum remaining maturity for the long bucket")
	flag.Parse()
	if *shortMax <= 0 || *longMin <= 0 || *longMin <= *shortMax {
		fatal(fmt.Errorf("maturity bands require 0 < short-max < long-min"))
	}

	raw, err := os.ReadFile(*input)
	if err != nil {
		fatal(err)
	}
	var reports multiVenueReport
	if err := json.Unmarshal(raw, &reports); err != nil {
		fatal(err)
	}
	result, err := SummarizeGreekReports(reports.Venues)
	if err != nil {
		fatal(err)
	}
	result.RemainingMaturities, err = SummarizeRemainingMaturities(reports.Venues, []MaturityBand{
		{Name: "short", MinNanos: 1, MaxNanos: shortMax.Nanoseconds()},
		{Name: "long", MinNanos: longMin.Nanoseconds()},
	})
	if err != nil {
		fatal(err)
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fatal(err)
	}
	if *output == "" {
		fmt.Println(string(encoded))
		return
	}
	if err := os.WriteFile(*output, encoded, 0644); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "greekreport:", err)
	os.Exit(1)
}

// SummarizeGreekReports is exported for tests and research tooling.
func SummarizeGreekReports(reports map[string]derivsim.GreekReport) (analysis, error) {
	buckets := make(map[string]*bucket)
	for venue, report := range reports {
		for _, row := range report.PositionProfiles {
			if row.ListedNano <= 0 || row.ExpiryNano <= row.ListedNano {
				return analysis{}, fmt.Errorf("venue %q position %q lacks valid listing generation", venue, row.Symbol)
			}
			if !finite(row.Delta) || !finite(row.Gamma) || !finite(row.Vega) {
				return analysis{}, fmt.Errorf("venue %q position %q has non-finite sensitivity", venue, row.Symbol)
			}
			key := fmt.Sprintf("%s/%d/%d", venue, row.ListedNano, row.ExpiryNano)
			b := buckets[key]
			if b == nil {
				b = &bucket{venue: venue, listing: row.ListedNano, expiry: row.ExpiryNano, byTimestamp: make(map[int64]*optionRisk), symbols: make(map[string]struct{})}
				buckets[key] = b
			}
			snapshot := b.byTimestamp[row.Timestamp]
			if snapshot == nil {
				snapshot = &optionRisk{Timestamp: row.Timestamp, TimeToExpiryNano: row.TimeToExpiryNano}
				b.byTimestamp[row.Timestamp] = snapshot
			}
			if snapshot.TimeToExpiryNano != row.TimeToExpiryNano {
				return analysis{}, fmt.Errorf("venue %q expiry %d has inconsistent time-to-expiry at %d", venue, row.ExpiryNano, row.Timestamp)
			}
			snapshot.OptionDelta += row.Delta
			snapshot.Gamma += row.Gamma
			snapshot.Vega += row.Vega
			snapshot.Positions++
			b.symbols[row.Symbol] = struct{}{}
		}
	}

	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := analysis{
		SchemaVersion: 1,
		Caveats: []string{
			"Rows aggregate only filled option inventory. Aggregate dealer hedges are intentionally not allocated across expiries.",
			"Means are equally weighted observed timestamps, not PnL or time-weighted risk.",
			"Static IV and the spot-mid forward proxy make vega a local sensitivity, not realized volatility PnL.",
		},
		Tenors: make([]TenorSummary, 0, len(keys)),
	}
	for _, key := range keys {
		b := buckets[key]
		timestamps := make([]int64, 0, len(b.byTimestamp))
		for timestamp := range b.byTimestamp {
			timestamps = append(timestamps, timestamp)
		}
		sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })
		summary := TenorSummary{
			VenueID: b.venue, ListedNano: b.listing, ExpiryNano: b.expiry, ListingTenorNano: b.expiry - b.listing,
			Samples: len(timestamps), Symbols: len(b.symbols), First: *b.byTimestamp[timestamps[0]],
		}
		for _, timestamp := range timestamps {
			snapshot := *b.byTimestamp[timestamp]
			if snapshot.TimeToExpiryNano > 0 {
				summary.LastPreExpiry = snapshot
			}
			absDelta := math.Abs(snapshot.OptionDelta)
			absGamma := math.Abs(snapshot.Gamma)
			absVega := math.Abs(snapshot.Vega)
			summary.MaxAbsDelta = max(summary.MaxAbsDelta, absDelta)
			summary.MaxAbsGamma = max(summary.MaxAbsGamma, absGamma)
			summary.MaxAbsVega = max(summary.MaxAbsVega, absVega)
			summary.MeanAbsDelta += absDelta
			summary.MeanAbsGamma += absGamma
			summary.MeanAbsVega += absVega
		}
		n := float64(summary.Samples)
		summary.MeanAbsDelta /= n
		summary.MeanAbsGamma /= n
		summary.MeanAbsVega /= n
		result.Tenors = append(result.Tenors, summary)
	}
	return result, nil
}

// SummarizeRemainingMaturities derives exposure moments from actual time to
// expiry. A row belongs to every requested band whose inclusive bounds match;
// callers should therefore choose disjoint bands when comparing regimes.
func SummarizeRemainingMaturities(reports map[string]derivsim.GreekReport, bands []MaturityBand) ([]RemainingTenorSummary, error) {
	type key struct{ venue, band string }
	byBand := make(map[key]map[int64]*maturityRisk)
	for _, band := range bands {
		if band.Name == "" || band.MinNanos < 0 || (band.MaxNanos != 0 && band.MaxNanos < band.MinNanos) {
			return nil, fmt.Errorf("invalid maturity band %+v", band)
		}
	}
	for venue, report := range reports {
		for _, row := range report.PositionProfiles {
			if row.TimeToExpiryNano <= 0 || !finite(row.Delta) || !finite(row.Gamma) || !finite(row.Vega) {
				continue
			}
			for _, band := range bands {
				if row.TimeToExpiryNano < band.MinNanos || (band.MaxNanos != 0 && row.TimeToExpiryNano > band.MaxNanos) {
					continue
				}
				k := key{venue: venue, band: band.Name}
				if byBand[k] == nil {
					byBand[k] = make(map[int64]*maturityRisk)
				}
				snapshot := byBand[k][row.Timestamp]
				if snapshot == nil {
					snapshot = &maturityRisk{
						Timestamp:           row.Timestamp,
						MinTimeToExpiryNano: row.TimeToExpiryNano,
						MaxTimeToExpiryNano: row.TimeToExpiryNano,
					}
					byBand[k][row.Timestamp] = snapshot
				}
				snapshot.MinTimeToExpiryNano = min(snapshot.MinTimeToExpiryNano, row.TimeToExpiryNano)
				snapshot.MaxTimeToExpiryNano = max(snapshot.MaxTimeToExpiryNano, row.TimeToExpiryNano)
				snapshot.OptionDelta += row.Delta
				snapshot.Gamma += row.Gamma
				snapshot.Vega += row.Vega
				snapshot.Positions++
			}
		}
	}

	result := make([]RemainingTenorSummary, 0, len(byBand))
	for _, band := range bands {
		venues := make([]string, 0, len(reports))
		for venue := range reports {
			venues = append(venues, venue)
		}
		sort.Strings(venues)
		for _, venue := range venues {
			snapshots := byBand[key{venue: venue, band: band.Name}]
			if len(snapshots) == 0 {
				continue
			}
			timestamps := make([]int64, 0, len(snapshots))
			for timestamp := range snapshots {
				timestamps = append(timestamps, timestamp)
			}
			sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })
			summary := RemainingTenorSummary{VenueID: venue, Band: band, Samples: len(timestamps), First: *snapshots[timestamps[0]], Last: *snapshots[timestamps[len(timestamps)-1]]}
			for _, timestamp := range timestamps {
				snapshot := snapshots[timestamp]
				absDelta := math.Abs(snapshot.OptionDelta)
				absGamma := math.Abs(snapshot.Gamma)
				absVega := math.Abs(snapshot.Vega)
				summary.MaxAbsDelta = max(summary.MaxAbsDelta, absDelta)
				summary.MaxAbsGamma = max(summary.MaxAbsGamma, absGamma)
				summary.MaxAbsVega = max(summary.MaxAbsVega, absVega)
				summary.MeanAbsDelta += absDelta
				summary.MeanAbsGamma += absGamma
				summary.MeanAbsVega += absVega
			}
			n := float64(summary.Samples)
			summary.MeanAbsDelta /= n
			summary.MeanAbsGamma /= n
			summary.MeanAbsVega /= n
			result = append(result, summary)
		}
	}
	return result, nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
