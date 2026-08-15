package derivsim

import (
	"fmt"
	"math"
)

// GreekSummary is a compact risk result for one dealer's ordered snapshots.
// The full Profiles slice remains the source of truth: aggregate statistics
// are deliberately not used for tenor- or contract-level attribution.
type GreekSummary struct {
	HasSamples      bool         `json:"has_samples"`
	Samples         int          `json:"samples"`
	FirstTimestamp  int64        `json:"first_timestamp,omitempty"`
	LastTimestamp   int64        `json:"last_timestamp,omitempty"`
	MaxAbsNetDelta  float64      `json:"max_abs_net_delta"`
	MeanAbsNetDelta float64      `json:"mean_abs_net_delta"`
	MaxAbsGamma     float64      `json:"max_abs_gamma"`
	MeanAbsGamma    float64      `json:"mean_abs_gamma"`
	MaxAbsVega      float64      `json:"max_abs_vega"`
	MeanAbsVega     float64      `json:"mean_abs_vega"`
	Initial         GreekProfile `json:"initial,omitempty"`
	Final           GreekProfile `json:"final,omitempty"`
}

// GreekReport is the portable output written after a derivative simulation.
// It makes the model and scheduling assumptions machine-readable alongside
// the measurements, rather than allowing a downstream plot to imply a richer
// volatility surface or post-fill hedge state than the simulator produced.
type GreekReport struct {
	Model         string         `json:"model"`
	ForwardSource string         `json:"forward_source"`
	SamplingPhase string         `json:"sampling_phase"`
	Caveats       []string       `json:"caveats"`
	Summary       GreekSummary   `json:"summary"`
	Profiles      []GreekProfile `json:"profiles"`
}

// BuildGreekReport validates and copies the profile stream before deriving
// summary metrics. Refusing NaN/Inf prevents JSON serialization from silently
// dropping a corrupted risk observation.
func BuildGreekReport(profiles []GreekProfile) (GreekReport, error) {
	for i, profile := range profiles {
		if !finiteGreek(profile.OptionDelta) || !finiteGreek(profile.HedgeDelta) ||
			!finiteGreek(profile.NetDelta) || !finiteGreek(profile.Gamma) || !finiteGreek(profile.Vega) ||
			!finiteGreek(profile.ImpliedVolatility) {
			return GreekReport{}, fmt.Errorf("non-finite Greek profile at index %d", i)
		}
	}

	report := GreekReport{
		Model:         "black76_forward",
		ForwardSource: "spot_mid_proxy",
		SamplingPhase: "post_quote_pre_hedge_fill",
		Caveats: []string{
			"Forward is the underlying spot midpoint proxy; maturity-matched forward curves are not modeled.",
			"Flat implied volatility is static, so vega is local sensitivity rather than realized volatility PnL.",
			"Snapshots are pre-hedge-fill; hedge orders submitted in the same phase may fill later.",
			"The final periodic sample precedes expiry; this actor report does not replace an exchange-owned pre-expiry risk snapshot.",
		},
		Profiles: append([]GreekProfile(nil), profiles...),
	}
	if len(profiles) == 0 {
		return report, nil
	}

	summary := GreekSummary{
		HasSamples:     true,
		Samples:        len(profiles),
		FirstTimestamp: profiles[0].Timestamp,
		LastTimestamp:  profiles[len(profiles)-1].Timestamp,
		Initial:        profiles[0],
		Final:          profiles[len(profiles)-1],
	}
	for _, profile := range profiles {
		absDelta := math.Abs(profile.NetDelta)
		absGamma := math.Abs(profile.Gamma)
		absVega := math.Abs(profile.Vega)
		summary.MeanAbsNetDelta += absDelta
		summary.MeanAbsGamma += absGamma
		summary.MeanAbsVega += absVega
		summary.MaxAbsNetDelta = max(summary.MaxAbsNetDelta, absDelta)
		summary.MaxAbsGamma = max(summary.MaxAbsGamma, absGamma)
		summary.MaxAbsVega = max(summary.MaxAbsVega, absVega)
	}
	n := float64(len(profiles))
	summary.MeanAbsNetDelta /= n
	summary.MeanAbsGamma /= n
	summary.MeanAbsVega /= n
	report.Summary = summary
	return report, nil
}

func finiteGreek(x float64) bool {
	return !math.IsNaN(x) && !math.IsInf(x, 0)
}
