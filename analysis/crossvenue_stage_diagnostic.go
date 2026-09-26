package analysis

import "fmt"

type CrossVenueStageExposure struct {
	DirectionalEpisodes  int   `json:"directional_episodes"`
	PositiveNanos        int64 `json:"positive_nanos"`
	ZeroDurationEpisodes int   `json:"zero_duration_episodes"`
}

type CrossVenuePublicStages struct {
	SchemaVersion        int                     `json:"schema_version"`
	EvaluatedTransitions int                     `json:"evaluated_transitions"`
	TwoSidedLegPrices    CrossVenueStageExposure `json:"two_sided_leg_prices"`
	GrossCrossing        CrossVenueStageExposure `json:"gross_crossing"`
	OneLotDepth          CrossVenueStageExposure `json:"one_lot_depth"`
	FeePositive          CrossVenueStageExposure `json:"fee_positive"`
}

type crossVenueStageTracker struct {
	active bool
	start  int64
	metric *CrossVenueStageExposure
}

func (tracker *crossVenueStageTracker) observe(at int64, qualifies bool) {
	if tracker.active && !qualifies {
		duration := at - tracker.start
		tracker.metric.PositiveNanos += duration
		if duration == 0 {
			tracker.metric.ZeroDurationEpisodes++
		}
	}
	if !tracker.active && qualifies {
		tracker.start = at
		tracker.metric.DirectionalEpisodes++
	}
	tracker.active = qualifies
}

// DiagnoseCrossVenuePublicStages decomposes the registered two-sided,
// one-lot, fee-positive public predicate without changing that predicate.
// Counts are maximal directional episodes in canonical event order; a
// same-timestamp episode may have zero duration.
func DiagnoseCrossVenuePublicStages(venues [2]string, transitions []CrossVenuePublicTransition,
	horizonNano, lotQty, basePrecision, feeBps int64) (CrossVenuePublicStages, error) {
	if venues[0] == "" || venues[1] == "" || venues[0] == venues[1] ||
		horizonNano <= 0 || lotQty <= 0 || basePrecision <= 0 || feeBps < 0 || len(transitions) == 0 {
		return CrossVenuePublicStages{}, fmt.Errorf("cross-venue stages: invalid convention or empty evidence")
	}
	result := CrossVenuePublicStages{SchemaVersion: 1}
	metrics := []*CrossVenueStageExposure{&result.TwoSidedLegPrices, &result.GrossCrossing,
		&result.OneLotDepth, &result.FeePositive}
	var trackers [2][4]crossVenueStageTracker
	for direction := range trackers {
		for stage := range trackers[direction] {
			trackers[direction][stage].metric = metrics[stage]
		}
	}
	var books [2]CrossVenueTouch
	var known [2]bool
	var previousSequence uint64
	var previousTS int64
	for index, transition := range transitions {
		if transition.GlobalSequence == 0 || index > 0 && transition.GlobalSequence <= previousSequence ||
			transition.SimTS < 0 || index > 0 && transition.SimTS < previousTS || transition.SimTS > horizonNano {
			return CrossVenuePublicStages{}, fmt.Errorf("cross-venue stages: transition %d is out of order or horizon", index)
		}
		previousSequence, previousTS = transition.GlobalSequence, transition.SimTS
		venueIndex := -1
		for candidate, venue := range venues {
			if transition.VenueID == venue {
				venueIndex = candidate
				break
			}
		}
		if venueIndex < 0 || transition.Touch.HasBid && transition.Touch.BidQty <= 0 ||
			transition.Touch.HasAsk && transition.Touch.AskQty <= 0 ||
			!transition.Touch.HasBid && (transition.Touch.Bid != 0 || transition.Touch.BidQty != 0) ||
			!transition.Touch.HasAsk && (transition.Touch.Ask != 0 || transition.Touch.AskQty != 0) {
			return CrossVenuePublicStages{}, fmt.Errorf("cross-venue stages: transition %d has malformed venue touch", index)
		}
		books[venueIndex], known[venueIndex] = transition.Touch, true
		if !known[0] || !known[1] {
			continue
		}
		result.EvaluatedTransitions++
		feeEdge := EvaluateCrossVenueOneLotEdge(venues, books, lotQty, basePrecision, feeBps, true)
		for buyIndex := range books {
			buy, sell := books[buyIndex], books[1-buyIndex]
			twoSided := buy.HasBid && buy.HasAsk && sell.HasBid && sell.HasAsk && buy.Ask > 0 && sell.Bid > 0
			gross := twoSided && sell.Bid > buy.Ask
			depth := gross && buy.AskQty >= lotQty && sell.BidQty >= lotQty
			positive := depth && feeEdge.Directions[buyIndex].Status == "POSITIVE_EDGE"
			for stage, qualifies := range [4]bool{twoSided, gross, depth, positive} {
				trackers[buyIndex][stage].observe(transition.SimTS, qualifies)
			}
		}
	}
	if !known[0] || !known[1] {
		return CrossVenuePublicStages{}, fmt.Errorf("cross-venue stages: both venue books were never observed")
	}
	for direction := range trackers {
		for stage := range trackers[direction] {
			trackers[direction][stage].observe(horizonNano, false)
		}
	}
	return result, nil
}
