package main

import (
	"strings"

	"exchange_sim/analysis"
)

// This file holds the option values for every metric that both extraction paths
// compute: the single-metric switch in main.go and the fused driver.
//
// Duplicating them is the one way the two paths could silently disagree while
// both still produced well-formed artifacts — a metric run one way would answer
// a different question than the same metric run the other way, and nothing in
// the output would say so. Defining each set once removes that possibility by
// construction rather than by review.
//
// Adding a metric to the fused driver means adding its options here and calling
// the same function from both paths.

// metricSettings carries the flag-derived values the shared option sets need.
type metricSettings struct {
	basePrecision           int64
	quotePrecision          int64
	requireExactReplay      bool
	deliveryFeePolicy       string
	fundingIntervalSeconds  int64
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
	// fundingIntervals is loaded from the run directory and is consulted only
	// by the derivative audit. It is nil when that metric was not selected.
	fundingIntervals map[string]int64
}

func postOnlyOptions(s metricSettings) analysis.PostOnlyActivityOptions {
	return analysis.PostOnlyActivityOptions{
		Roles:   strings.Split(s.postOnlyRoles, ","),
		Symbols: strings.Split(s.postOnlySymbols, ","),
	}
}

func hedgingOptions(s metricSettings) analysis.HedgingOptions {
	return analysis.HedgingOptions{
		Symbol: s.hedgeSymbol,
		Roles:  []string{"option_dealer", "vanna_volga_desk"},
	}
}

func perpSignalOptions(s metricSettings) analysis.PerpSignalOptions {
	return analysis.PerpSignalOptions{
		Symbol: s.perpSignalSymbol, RequiredVenues: splitNonEmpty(s.perpSignalVenues),
	}
}

func exposureOptions(metricSettings) analysis.ExposureOptions {
	return analysis.ExposureOptions{Roles: []string{"option_dealer"}}
}

func optionSurfaceOptions(s metricSettings) analysis.SurfaceOptions {
	return analysis.SurfaceOptions{QuotePrecision: s.quotePrecision}
}

func positionOptions(s metricSettings) analysis.PositionOptions {
	return analysis.PositionOptions{
		BasePrecision: s.basePrecision, RequireExactReplay: s.requireExactReplay,
	}
}

func settlementOptions(s metricSettings) analysis.SettlementAuditOptions {
	return analysis.SettlementAuditOptions{
		BasePrecision: s.basePrecision, RequireExactReplay: s.requireExactReplay,
		DeliveryFeePolicy: s.deliveryFeePolicy,
	}
}

// derivativeOptions takes the funding intervals separately because loading them
// reads a file and can fail, and only this metric needs them. Building them
// eagerly for every metric would turn an unrelated read failure into a failure
// of metrics that never consult the value.
func derivativeOptions(s metricSettings, fundingIntervals map[string]int64) analysis.DerivativeAuditOptions {
	return analysis.DerivativeAuditOptions{
		BasePrecision: s.basePrecision, RequireExactReplay: s.requireExactReplay,
		ExpectedFundingIntervalSeconds: s.fundingIntervalSeconds,
		ExpectedFundingIntervals:       fundingIntervals,
	}
}

func arbitrageOptions(s metricSettings) analysis.ArbitrageOptions {
	return analysis.ArbitrageOptions{
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
	}
}

func crossVenueOptions(s metricSettings) analysis.CrossVenueDispersionOptions {
	return analysis.CrossVenueDispersionOptions{
		Symbol: s.crossVenueSymbol, StalenessNanos: int64(s.arbStaleness * 1e9),
		MinVenues:                       s.crossVenueMin,
		CapturePositiveObservationTimes: s.crossVenuePositiveTimes,
	}
}
