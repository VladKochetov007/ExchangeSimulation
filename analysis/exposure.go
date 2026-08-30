package analysis

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"exchange_sim/price"
)

// Dealer exposure and option-to-underlying transmission.
//
// Two questions live here, both preregistered against the delta-hedge and
// vanna-volga arms. Does removing hedging leave the dealer's delta growing
// without bound, and does option flow stop reaching the underlying?
//
// The exposure half reads the run's own risk timeline, which records each
// participant's option delta, the delta they have hedged, their net, and their
// vega once a minute. The transmission half is computed from trades alone.

// ExposureOptions selects inputs.
type ExposureOptions struct {
	Files         []string
	FilesSelected bool
	// BucketSeconds is the width of the volume buckets the option-to-
	// underlying correlation is computed over.
	BucketSeconds int64
	// Roles restricts the exposure summary; empty means option dealers.
	Roles []string
}

// riskRow is one sample of one participant's risk, as the run recorded it.
type riskRow struct {
	VenueID  string `json:"venue_id"`
	ClientID uint64 `json:"client_id"`
	Profile  struct {
		Timestamp   int64   `json:"timestamp"`
		SpotMid     int64   `json:"spot_mid"`
		OptionDelta float64 `json:"option_delta"`
		HedgeDelta  float64 `json:"hedge_delta"`
		NetDelta    float64 `json:"net_delta"`
		Gamma       float64 `json:"gamma"`
		Vega        float64 `json:"vega"`
		Contracts   int64   `json:"contracts"`
	} `json:"greek_profile"`
	GreekPositions *[]riskGreekPosition `json:"greek_positions"`
}

// riskGreekPosition is the exchange-owned per-contract position snapshot used
// to reconstruct second-order risks. It deliberately does not consume an
// actor's own vanna-volga model or exposure cache.
type riskGreekPosition struct {
	Position          int64   `json:"position"`
	TimeToExpiryNano  int64   `json:"time_to_expiry_nano"`
	ModelForward      int64   `json:"model_forward"`
	Strike            int64   `json:"strike"`
	ImpliedVolatility float64 `json:"implied_volatility"`
}

const multivenueContractPrecision = 100_000_000

// ExposureSeries summarises one participant's risk through the run.
type ExposureSeries struct {
	VenueID  string `json:"venue_id"`
	ClientID uint64 `json:"client_id"`
	Role     string `json:"role"`
	Samples  int    `json:"samples"`
	// MeanAbsNetDelta is the average size of the unhedged position and
	// MaxAbsNetDelta its worst moment. A dealer that hedges keeps both small
	// relative to its option delta.
	MeanAbsOptionDelta float64 `json:"mean_abs_option_delta"`
	MeanAbsHedgeDelta  float64 `json:"mean_abs_hedge_delta"`
	MeanAbsNetDelta    float64 `json:"mean_abs_net_delta"`
	MaxAbsNetDelta     float64 `json:"max_abs_net_delta"`
	FinalNetDelta      float64 `json:"final_net_delta"`
	// NetDeltaDriftPerHour is the slope of net delta against time. Unbounded
	// growth shows up here as a slope that does not sit near zero.
	NetDeltaDriftPerHour float64 `json:"net_delta_drift_per_hour"`
	// HedgeRatio is mean |hedge delta| over mean |option delta|: one means
	// every unit of option delta was laid off, zero means none was.
	HedgeRatio      float64 `json:"hedge_ratio"`
	MeanAbsVega     float64 `json:"mean_abs_vega"`
	MaxAbsVega      float64 `json:"max_abs_vega"`
	FinalVega       float64 `json:"final_vega"`
	VegaDriftPerHer float64 `json:"vega_drift_per_hour"`
	// Vanna and Volga are reconstructed independently from the persisted
	// exchange-owned position rows. The original timeline recorded only
	// aggregate delta, gamma, and vega, which was insufficient to score the
	// vanna-volga intervention on its stated mechanism.
	MeanAbsVanna      float64 `json:"mean_abs_vanna"`
	MaxAbsVanna       float64 `json:"max_abs_vanna"`
	FinalVanna        float64 `json:"final_vanna"`
	VannaDriftPerHour float64 `json:"vanna_drift_per_hour"`
	MeanAbsVolga      float64 `json:"mean_abs_volga"`
	MaxAbsVolga       float64 `json:"max_abs_volga"`
	FinalVolga        float64 `json:"final_volga"`
	VolgaDriftPerHour float64 `json:"volga_drift_per_hour"`
}

// TransmissionStats is the correlation between option and underlying activity.
type TransmissionStats struct {
	VenueID string `json:"venue_id"`
	Buckets int    `json:"buckets"`
	// Correlation is Pearson between option volume and underlying volume in
	// the same bucket. This is the preregistered transmission measure and it
	// shares a clock with everything else in the run, so a common driver can
	// produce it without any causal link.
	Correlation float64 `json:"correlation"`
	// LaggedCorrelation puts option volume one bucket ahead of underlying
	// volume, which is the direction hedging would work in.
	LaggedCorrelation float64 `json:"lagged_correlation"`
	OptionVolume      int64   `json:"option_volume"`
	UnderlyingVolume  int64   `json:"underlying_volume"`
}

// HedgeFlow is the dealer's own trading in the underlying. Not preregistered:
// reported as a diagnostic beside the transmission correlation.
type HedgeFlow struct {
	VenueID string `json:"venue_id"`
	Role    string `json:"role"`
	// TakerFills and TakerVolume are the dealer lifting the underlying, which
	// is what a hedge looks like from the log.
	TakerFills  int   `json:"taker_fills"`
	TakerVolume int64 `json:"taker_volume"`
	MakerFills  int   `json:"maker_fills"`
	OptionFills int   `json:"option_fills"`
}

// Exposure is the whole measurement.
type Exposure struct {
	Series []ExposureSeries `json:"series"`
	// Pooled aggregates the selected roles across venues.
	PooledMeanAbsNetDelta float64             `json:"pooled_mean_abs_net_delta"`
	PooledMaxAbsNetDelta  float64             `json:"pooled_max_abs_net_delta"`
	PooledHedgeRatio      float64             `json:"pooled_hedge_ratio"`
	PooledMeanAbsVega     float64             `json:"pooled_mean_abs_vega"`
	PooledMeanAbsVanna    float64             `json:"pooled_mean_abs_vanna"`
	PooledMeanAbsVolga    float64             `json:"pooled_mean_abs_volga"`
	PooledNetDeltaDrift   float64             `json:"pooled_net_delta_drift_per_hour"`
	Transmission          []TransmissionStats `json:"transmission"`
	PooledCorrelation     float64             `json:"pooled_correlation"`
	HedgeFlows            []HedgeFlow         `json:"hedge_flows"`
	RiskSamples           int                 `json:"risk_samples"`
	// SecondOrderSamples is the number of selected risk snapshots for which
	// vanna and volga were reconstructed. It must equal RiskSamples whenever
	// the current Greek-position evidence contract is present.
	SecondOrderSamples int `json:"second_order_samples"`
}

// secondOrderExposure reconstructs a snapshot's aggregate vanna and volga
// from the persisted marked contract positions. The multivenue simulator's
// contract unit is fixed at 1e8 base units; this is the same public quantity
// used by the position and settlement analyzers. A missing position field is
// refused rather than silently treated as zero exposure.
func secondOrderExposure(positions *[]riskGreekPosition) (float64, float64, error) {
	if positions == nil {
		return 0, 0, fmt.Errorf("analysis: Greek position evidence missing")
	}
	var vanna, volga float64
	for _, position := range *positions {
		if position.Position == 0 {
			continue
		}
		if position.TimeToExpiryNano <= 0 {
			return 0, 0, fmt.Errorf("analysis: non-positive option horizon in Greek position evidence")
		}
		years := float64(position.TimeToExpiryNano) / float64(365*24*time.Hour)
		if _, ok := price.Black76Sensitivities(position.ModelForward, position.Strike, position.ImpliedVolatility, years, true); !ok {
			return 0, 0, fmt.Errorf("analysis: invalid Black-76 inputs in Greek position evidence")
		}
		contracts := float64(position.Position) / multivenueContractPrecision
		vanna += contracts * price.Black76Vanna(position.ModelForward, position.Strike, position.ImpliedVolatility, years)
		volga += contracts * price.Black76Volga(position.ModelForward, position.Strike, position.ImpliedVolatility, years)
	}
	if math.IsNaN(vanna) || math.IsInf(vanna, 0) || math.IsNaN(volga) || math.IsInf(volga, 0) {
		return 0, 0, fmt.Errorf("analysis: non-finite reconstructed second-order exposure")
	}
	return vanna, volga, nil
}

// isOptionSymbolName reports whether a symbol names an option contract.
func isOptionSymbolName(symbol string) bool {
	_, _, _, ok := optionTerms(symbol, 100000)
	return ok
}

// MeasureExposure reads the risk timeline and the trade stream.
func (r *Run) MeasureExposure(opts ExposureOptions) (*Exposure, error) {
	wanted := map[string]bool{}
	for _, role := range opts.Roles {
		wanted[role] = true
	}
	if len(wanted) == 0 {
		wanted["option_dealer"] = true
	}
	bucketSeconds := opts.BucketSeconds
	if bucketSeconds <= 0 {
		bucketSeconds = 60
	}

	result := &Exposure{}

	// The risk timeline lives in the run report rather than the event stream,
	// because it is a periodic snapshot rather than an event.
	raw, err := os.ReadFile(filepath.Join(r.Dir, "greeks.json"))
	if err != nil {
		return nil, err
	}
	var envelope struct {
		RiskTimeline map[string][]riskRow `json:"risk_timeline"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}

	type seriesKey struct {
		venue  string
		client uint64
	}
	type accumulator struct {
		samples                         int
		sumOption, sumHedge, sumNet     float64
		maxNet, finalNet                float64
		sumVega, maxVega, finalVega     float64
		sumVanna, maxVanna, finalVanna  float64
		sumVolga, maxVolga, finalVolga  float64
		firstAt, lastAt                 int64
		sumT, sumTT, sumTNet, sumNetRaw float64
		sumTVega, sumVegaRaw            float64
		sumTVanna, sumVannaRaw          float64
		sumTVolga, sumVolgaRaw          float64
	}
	accumulators := make(map[seriesKey]*accumulator)
	for venue, rows := range envelope.RiskTimeline {
		for _, row := range rows {
			role := r.Role(venue, row.ClientID)
			if !wanted[role] {
				continue
			}
			vanna, volga, err := secondOrderExposure(row.GreekPositions)
			if err != nil {
				return nil, fmt.Errorf("analysis: reconstruct second-order exposure for %s client %d at %d: %w", venue, row.ClientID, row.Profile.Timestamp, err)
			}
			result.RiskSamples++
			result.SecondOrderSamples++
			key := seriesKey{venue, row.ClientID}
			acc := accumulators[key]
			if acc == nil {
				acc = &accumulator{firstAt: row.Profile.Timestamp}
				accumulators[key] = acc
			}
			profile := row.Profile
			acc.samples++
			acc.sumOption += math.Abs(profile.OptionDelta)
			acc.sumHedge += math.Abs(profile.HedgeDelta)
			acc.sumNet += math.Abs(profile.NetDelta)
			acc.maxNet = math.Max(acc.maxNet, math.Abs(profile.NetDelta))
			acc.sumVega += math.Abs(profile.Vega)
			acc.maxVega = math.Max(acc.maxVega, math.Abs(profile.Vega))
			acc.sumVanna += math.Abs(vanna)
			acc.maxVanna = math.Max(acc.maxVanna, math.Abs(vanna))
			acc.sumVolga += math.Abs(volga)
			acc.maxVolga = math.Max(acc.maxVolga, math.Abs(volga))
			if profile.Timestamp >= acc.lastAt {
				acc.lastAt = profile.Timestamp
				acc.finalNet = profile.NetDelta
				acc.finalVega = profile.Vega
				acc.finalVanna = vanna
				acc.finalVolga = volga
			}
			if profile.Timestamp < acc.firstAt || acc.firstAt == 0 {
				acc.firstAt = profile.Timestamp
			}
			hours := float64(profile.Timestamp) / 1e9 / 3600
			acc.sumT += hours
			acc.sumTT += hours * hours
			acc.sumTNet += hours * profile.NetDelta
			acc.sumNetRaw += profile.NetDelta
			acc.sumTVega += hours * profile.Vega
			acc.sumVegaRaw += profile.Vega
			acc.sumTVanna += hours * vanna
			acc.sumVannaRaw += vanna
			acc.sumTVolga += hours * volga
			acc.sumVolgaRaw += volga
		}
	}

	keys := make([]seriesKey, 0, len(accumulators))
	for key := range accumulators {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].venue != keys[j].venue {
			return keys[i].venue < keys[j].venue
		}
		return keys[i].client < keys[j].client
	})

	var poolNet, poolMaxNet, poolOption, poolHedge, poolVega, poolVanna, poolVolga, poolDrift, poolWeight float64
	for _, key := range keys {
		acc := accumulators[key]
		n := float64(acc.samples)
		series := ExposureSeries{
			VenueID: key.venue, ClientID: key.client, Role: r.Role(key.venue, key.client),
			Samples:            acc.samples,
			MeanAbsOptionDelta: acc.sumOption / n,
			MeanAbsHedgeDelta:  acc.sumHedge / n,
			MeanAbsNetDelta:    acc.sumNet / n,
			MaxAbsNetDelta:     acc.maxNet,
			FinalNetDelta:      acc.finalNet,
			MeanAbsVega:        acc.sumVega / n,
			MaxAbsVega:         acc.maxVega,
			FinalVega:          acc.finalVega,
			MeanAbsVanna:       acc.sumVanna / n,
			MaxAbsVanna:        acc.maxVanna,
			FinalVanna:         acc.finalVanna,
			MeanAbsVolga:       acc.sumVolga / n,
			MaxAbsVolga:        acc.maxVolga,
			FinalVolga:         acc.finalVolga,
		}
		if series.MeanAbsOptionDelta > 0 {
			series.HedgeRatio = series.MeanAbsHedgeDelta / series.MeanAbsOptionDelta
		}
		if acc.samples > 2 {
			denominator := n*acc.sumTT - acc.sumT*acc.sumT
			if denominator != 0 {
				series.NetDeltaDriftPerHour = (n*acc.sumTNet - acc.sumT*acc.sumNetRaw) / denominator
				series.VegaDriftPerHer = (n*acc.sumTVega - acc.sumT*acc.sumVegaRaw) / denominator
				series.VannaDriftPerHour = (n*acc.sumTVanna - acc.sumT*acc.sumVannaRaw) / denominator
				series.VolgaDriftPerHour = (n*acc.sumTVolga - acc.sumT*acc.sumVolgaRaw) / denominator
			}
		}
		result.Series = append(result.Series, series)
		poolWeight += n
		poolNet += series.MeanAbsNetDelta * n
		poolOption += series.MeanAbsOptionDelta * n
		poolHedge += series.MeanAbsHedgeDelta * n
		poolVega += series.MeanAbsVega * n
		poolVanna += series.MeanAbsVanna * n
		poolVolga += series.MeanAbsVolga * n
		poolDrift += series.NetDeltaDriftPerHour * n
		poolMaxNet = math.Max(poolMaxNet, series.MaxAbsNetDelta)
	}
	if poolWeight > 0 {
		result.PooledMeanAbsNetDelta = poolNet / poolWeight
		result.PooledMeanAbsVega = poolVega / poolWeight
		result.PooledMeanAbsVanna = poolVanna / poolWeight
		result.PooledMeanAbsVolga = poolVolga / poolWeight
		result.PooledNetDeltaDrift = poolDrift / poolWeight
		if poolOption > 0 {
			result.PooledHedgeRatio = poolHedge / poolOption
		}
	}
	result.PooledMaxAbsNetDelta = poolMaxNet

	// Transmission and hedge flow from the trade and fill streams.
	type volumeKey struct {
		venue  string
		bucket int64
	}
	var mu sync.Mutex
	optionVolume := make(map[volumeKey]int64)
	underlyingVolume := make(map[volumeKey]int64)
	type flowKey struct {
		venue string
		role  string
	}
	flows := make(map[flowKey]*HedgeFlow)

	type tradePayload struct {
		Qty int64 `json:"qty"`
	}
	type fillPayload struct {
		Symbol string `json:"symbol"`
		Qty    int64  `json:"qty"`
		Role   string `json:"role"`
	}
	scan := ScanOptions{
		Events:        []string{"Trade", "OrderFill"},
		Files:         opts.Files,
		FilesSelected: opts.FilesSelected,
	}
	if err := r.Scan(scan, func(event Event) {
		switch event.Name {
		case "Trade":
			var payload tradePayload
			if event.Decode(&payload) != nil || payload.Qty <= 0 {
				return
			}
			key := volumeKey{event.VenueID, event.SimTS / (bucketSeconds * 1e9)}
			mu.Lock()
			if isOptionSymbolName(event.Symbol) {
				optionVolume[key] += payload.Qty
			} else if strings.Contains(event.Symbol, "PERP") || strings.HasPrefix(event.Symbol, "ABC-USD") {
				underlyingVolume[key] += payload.Qty
			}
			mu.Unlock()
		case "OrderFill":
			var payload fillPayload
			if event.Decode(&payload) != nil || payload.Qty <= 0 {
				return
			}
			role := r.Role(event.VenueID, event.ClientID)
			if !wanted[role] {
				return
			}
			symbol := event.Symbol
			if symbol == "" {
				symbol = payload.Symbol
			}
			key := flowKey{event.VenueID, role}
			mu.Lock()
			flow := flows[key]
			if flow == nil {
				flow = &HedgeFlow{VenueID: event.VenueID, Role: role}
				flows[key] = flow
			}
			if isOptionSymbolName(symbol) {
				flow.OptionFills++
			} else if payload.Role == "taker" {
				flow.TakerFills++
				flow.TakerVolume += payload.Qty
			} else {
				flow.MakerFills++
			}
			mu.Unlock()
		}
	}); err != nil {
		return nil, err
	}

	venues := map[string]bool{}
	for key := range optionVolume {
		venues[key.venue] = true
	}
	for key := range underlyingVolume {
		venues[key.venue] = true
	}
	names := make([]string, 0, len(venues))
	for venue := range venues {
		names = append(names, venue)
	}
	sort.Strings(names)
	var corrWeight, corrSum float64
	for _, venue := range names {
		buckets := map[int64]bool{}
		for key := range optionVolume {
			if key.venue == venue {
				buckets[key.bucket] = true
			}
		}
		for key := range underlyingVolume {
			if key.venue == venue {
				buckets[key.bucket] = true
			}
		}
		ordered := make([]int64, 0, len(buckets))
		for bucket := range buckets {
			ordered = append(ordered, bucket)
		}
		sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
		xs := make([]float64, 0, len(ordered))
		ys := make([]float64, 0, len(ordered))
		stats := TransmissionStats{VenueID: venue, Buckets: len(ordered)}
		for _, bucket := range ordered {
			x := float64(optionVolume[volumeKey{venue, bucket}])
			y := float64(underlyingVolume[volumeKey{venue, bucket}])
			xs = append(xs, x)
			ys = append(ys, y)
			stats.OptionVolume += optionVolume[volumeKey{venue, bucket}]
			stats.UnderlyingVolume += underlyingVolume[volumeKey{venue, bucket}]
		}
		stats.Correlation = pearson(xs, ys)
		if len(xs) > 2 {
			stats.LaggedCorrelation = pearson(xs[:len(xs)-1], ys[1:])
		}
		result.Transmission = append(result.Transmission, stats)
		corrWeight += float64(stats.Buckets)
		corrSum += stats.Correlation * float64(stats.Buckets)
	}
	if corrWeight > 0 {
		result.PooledCorrelation = corrSum / corrWeight
	}

	flowKeys := make([]flowKey, 0, len(flows))
	for key := range flows {
		flowKeys = append(flowKeys, key)
	}
	sort.Slice(flowKeys, func(i, j int) bool {
		if flowKeys[i].venue != flowKeys[j].venue {
			return flowKeys[i].venue < flowKeys[j].venue
		}
		return flowKeys[i].role < flowKeys[j].role
	})
	for _, key := range flowKeys {
		result.HedgeFlows = append(result.HedgeFlows, *flows[key])
	}
	return result, nil
}

// pearson is the correlation of two equal-length samples, zero when either has
// no variation.
func pearson(xs, ys []float64) float64 {
	if len(xs) != len(ys) || len(xs) < 2 {
		return 0
	}
	n := float64(len(xs))
	var sx, sy float64
	for i := range xs {
		sx += xs[i]
		sy += ys[i]
	}
	mx, my := sx/n, sy/n
	var num, dx, dy float64
	for i := range xs {
		a, b := xs[i]-mx, ys[i]-my
		num += a * b
		dx += a * a
		dy += b * b
	}
	if dx <= 0 || dy <= 0 {
		return 0
	}
	return num / math.Sqrt(dx*dy)
}
