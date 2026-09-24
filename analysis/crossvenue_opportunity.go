package analysis

import (
	"fmt"

	etypes "exchange_sim/types"
)

// CrossVenueTouch is a public, displayed top-of-book state. HasBid/HasAsk
// distinguish absent sides from numeric zero, which is outside this positive
// spot router's cashflow domain.
type CrossVenueTouch struct {
	Bid, BidQty int64
	Ask, AskQty int64
	HasBid      bool
	HasAsk      bool
}

type CrossVenueOneLotEdge struct {
	Status     string
	BuyVenue   string
	SellVenue  string
	Edge       int64
	Directions [2]CrossVenueDirectionEdge
}

type CrossVenueDirectionEdge struct {
	BuyVenue  string
	SellVenue string
	Status    string
	Edge      int64
}

// EvaluateCrossVenueOneLotEdge checks both directions using a fixed lot and
// quote-denominated taker fees. requireTwoSided reproduces the current router's
// extra eligibility rule; false is the broader leg-side diagnostic. This is a
// public-state estimator, never the actor's delayed information set.
func EvaluateCrossVenueOneLotEdge(venues [2]string, books [2]CrossVenueTouch, lotQty, basePrecision, feeBps int64, requireTwoSided bool) CrossVenueOneLotEdge {
	result := CrossVenueOneLotEdge{Status: "MISSING_SIDE"}
	if venues[0] == "" || venues[1] == "" || venues[0] == venues[1] || lotQty <= 0 || basePrecision <= 0 || feeBps < 0 {
		result.Status = "INVALID_CONVENTION"
		return result
	}
	statusRank := map[string]int{"MISSING_SIDE": 0, "INSUFFICIENT_DEPTH": 1, "OUT_OF_DOMAIN": 2, "ARITHMETIC_OVERFLOW": 3, "NONPOSITIVE_EDGE": 4, "POSITIVE_EDGE": 5}
	for buyIndex := range books {
		sellIndex := 1 - buyIndex
		buy, sell := books[buyIndex], books[sellIndex]
		direction := CrossVenueDirectionEdge{BuyVenue: venues[buyIndex], SellVenue: venues[sellIndex]}
		status := "NONPOSITIVE_EDGE"
		switch {
		case !buy.HasAsk || !sell.HasBid || requireTwoSided && (!buy.HasBid || !sell.HasAsk):
			status = "MISSING_SIDE"
		case buy.Ask <= 0 || sell.Bid <= 0:
			status = "OUT_OF_DOMAIN"
		case buy.AskQty < lotQty || sell.BidQty < lotQty:
			status = "INSUFFICIENT_DEPTH"
		default:
			sellNotional, sellOK := etypes.TryMulDiv(lotQty, sell.Bid, basePrecision)
			buyNotional, buyOK := etypes.TryMulDiv(lotQty, buy.Ask, basePrecision)
			if !sellOK || !buyOK {
				status = "ARITHMETIC_OVERFLOW"
				break
			}
			sellFee, sellOK := etypes.TryMulBps(sellNotional, feeBps)
			buyFee, buyOK := etypes.TryMulBps(buyNotional, feeBps)
			if !sellOK || !buyOK {
				status = "ARITHMETIC_OVERFLOW"
				break
			}
			proceeds, proceedsOK := etypes.TrySub(sellNotional, sellFee)
			cost, costOK := etypes.TryAdd(buyNotional, buyFee)
			if !proceedsOK || !costOK {
				status = "ARITHMETIC_OVERFLOW"
				break
			}
			edge, edgeOK := etypes.TrySub(proceeds, cost)
			if !edgeOK {
				status = "ARITHMETIC_OVERFLOW"
				break
			}
			if edge > 0 {
				status = "POSITIVE_EDGE"
				direction.Edge = edge
				if result.Status != status || edge > result.Edge || edge == result.Edge && (venues[buyIndex] < result.BuyVenue || venues[buyIndex] == result.BuyVenue && venues[sellIndex] < result.SellVenue) {
					result.Status, result.BuyVenue, result.SellVenue, result.Edge = status, venues[buyIndex], venues[sellIndex], edge
				}
				direction.Status = status
				result.Directions[buyIndex] = direction
				continue
			}
			direction.Edge = edge
		}
		direction.Status = status
		result.Directions[buyIndex] = direction
		if statusRank[status] > statusRank[result.Status] {
			result.Status = status
		}
	}
	return result
}

type CrossVenuePublicTransition struct {
	SimTS          int64
	GlobalSequence uint64
	VenueID        string
	Touch          CrossVenueTouch
}

// CrossVenueEdgeEpisode is half-open in simulated time. A same-timestamp
// positive state can therefore have zero duration while still being an actual
// event-ordered public opportunity. Censored means it persisted to horizon.
type CrossVenueEdgeEpisode struct {
	StartTS       int64
	StartSequence uint64
	EndTS         int64
	EndSequence   uint64
	Censored      bool
	BuyVenue      string
	SellVenue     string
	InitialEdge   int64
	EndStatus     string
}

// ReconstructCrossVenueEdgeEpisodes requires transitions already ordered by
// canonical global frame sequence. A sample-clock midpoint tape cannot be
// substituted for these public book transitions.
func ReconstructCrossVenueEdgeEpisodes(venues [2]string, transitions []CrossVenuePublicTransition, horizonNano int64, lotQty, basePrecision, feeBps int64) ([]CrossVenueEdgeEpisode, error) {
	return reconstructCrossVenueEdgeEpisodes(venues, transitions, horizonNano, lotQty, basePrecision, feeBps, true)
}

// ReconstructCrossVenueLegSideEdgeEpisodes measures the broader executable
// leg-side opportunity set without changing the router-policy denominator.
func ReconstructCrossVenueLegSideEdgeEpisodes(venues [2]string, transitions []CrossVenuePublicTransition, horizonNano int64, lotQty, basePrecision, feeBps int64) ([]CrossVenueEdgeEpisode, error) {
	return reconstructCrossVenueEdgeEpisodes(venues, transitions, horizonNano, lotQty, basePrecision, feeBps, false)
}

func reconstructCrossVenueEdgeEpisodes(venues [2]string, transitions []CrossVenuePublicTransition, horizonNano int64, lotQty, basePrecision, feeBps int64, requireTwoSided bool) ([]CrossVenueEdgeEpisode, error) {
	if venues[0] == "" || venues[1] == "" || venues[0] == venues[1] || lotQty <= 0 || basePrecision <= 0 || feeBps < 0 || horizonNano <= 0 {
		return nil, fmt.Errorf("cross-venue episodes: invalid convention")
	}
	var books [2]CrossVenueTouch
	var known [2]bool
	var episodes []CrossVenueEdgeEpisode
	var open *CrossVenueEdgeEpisode
	var previousSequence uint64
	var previousTS int64
	for index, transition := range transitions {
		if transition.GlobalSequence == 0 || index > 0 && transition.GlobalSequence <= previousSequence || transition.SimTS < 0 || index > 0 && transition.SimTS < previousTS || transition.SimTS > horizonNano {
			return nil, fmt.Errorf("cross-venue episodes: transition %d is out of order or horizon", index)
		}
		previousSequence, previousTS = transition.GlobalSequence, transition.SimTS
		venueIndex := -1
		for candidate, venue := range venues {
			if transition.VenueID == venue {
				venueIndex = candidate
				break
			}
		}
		if venueIndex < 0 {
			return nil, fmt.Errorf("cross-venue episodes: unknown venue %q", transition.VenueID)
		}
		books[venueIndex], known[venueIndex] = transition.Touch, true
		if !known[0] || !known[1] {
			continue
		}
		state := EvaluateCrossVenueOneLotEdge(venues, books, lotQty, basePrecision, feeBps, requireTwoSided)
		if state.Status == "INVALID_CONVENTION" {
			return nil, fmt.Errorf("cross-venue episodes: invalid book convention")
		}
		if open != nil && (state.Status != "POSITIVE_EDGE" || state.BuyVenue != open.BuyVenue || state.SellVenue != open.SellVenue) {
			open.EndTS, open.EndSequence, open.EndStatus = transition.SimTS, transition.GlobalSequence, state.Status
			episodes = append(episodes, *open)
			open = nil
		}
		if state.Status == "POSITIVE_EDGE" && open == nil {
			open = &CrossVenueEdgeEpisode{StartTS: transition.SimTS, StartSequence: transition.GlobalSequence, BuyVenue: state.BuyVenue, SellVenue: state.SellVenue, InitialEdge: state.Edge}
		}
	}
	if open != nil {
		open.EndTS, open.Censored, open.EndStatus = horizonNano, true, "HORIZON_CENSORED"
		episodes = append(episodes, *open)
	}
	return episodes, nil
}
