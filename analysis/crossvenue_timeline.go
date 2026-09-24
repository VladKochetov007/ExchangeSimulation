package analysis

import "fmt"

type CrossVenueEpisodeObservation struct {
	Episode            CrossVenueEdgeEpisode
	Evaluations        int
	AlignedEvaluations int
	Submissions        int
}

type CrossVenueOpportunityObservation struct {
	Generation         uint64
	EvaluationFrame    uint64
	EvaluatedAt        int64
	Reason             string
	PublicEpisodeIndex int
	PublicBuyVenue     string
	PublicSellVenue    string
	LocalStatus        string
	LocalBuyVenue      string
	LocalSellVenue     string
	Relation           string
	SourceFrameByVenue map[string]uint64
}

type CrossVenueOpportunityTimeline struct {
	Episodes    []CrossVenueEpisodeObservation
	Evaluations []CrossVenueOpportunityObservation
}

// ReconstructCrossVenueOpportunityTimeline requires the complete audited
// callback/receipt/source chain. Public opportunity and actor-local quote
// state are separate; neither proves balance feasibility or venue execution.
func ReconstructCrossVenueOpportunityTimeline(evaluations []CrossVenueEvaluationRecord, publicEvents []Event, evidenceDir string, venues [2]string, symbol string, horizonNano, lotQty, basePrecision, feeBps int64) (*CrossVenueOpportunityTimeline, error) {
	if evidenceDir == "" || symbol == "" || horizonNano <= 0 || lotQty <= 0 || basePrecision <= 0 || feeBps < 0 ||
		venues[0] == "" || venues[1] == "" || venues[0] == venues[1] {
		return nil, fmt.Errorf("cross-venue opportunity timeline: invalid contract")
	}
	if err := VerifyCrossVenueEvaluationReceipts(evaluations, evidenceDir, symbol); err != nil {
		return nil, err
	}
	sources, err := MatchCrossVenueEvaluationSources(evaluations, publicEvents, symbol, lotQty, basePrecision, feeBps)
	if err != nil {
		return nil, err
	}
	replay, err := ReplayCrossVenuePublicEvents(publicEvents, CrossVenuePublicReplayOptions{Venues: venues, Symbol: symbol, InitiallyEmpty: true})
	if err != nil {
		return nil, err
	}
	episodes, err := ReconstructCrossVenueEdgeEpisodes(venues, replay.Transitions, horizonNano, lotQty, basePrecision, feeBps)
	if err != nil {
		return nil, err
	}
	return joinCrossVenueOpportunityTimeline(evaluations, sources, episodes, venues, horizonNano, lotQty, basePrecision, feeBps)
}

func joinCrossVenueOpportunityTimeline(evaluations []CrossVenueEvaluationRecord, sources []CrossVenueConsumedSource, episodes []CrossVenueEdgeEpisode, venues [2]string, horizonNano, lotQty, basePrecision, feeBps int64) (*CrossVenueOpportunityTimeline, error) {
	if len(evaluations) != len(sources) || venues[0] == "" || venues[1] == "" || venues[0] == venues[1] || horizonNano <= 0 || lotQty <= 0 || basePrecision <= 0 || feeBps < 0 {
		return nil, fmt.Errorf("cross-venue opportunity timeline: incomplete source or invalid convention")
	}
	timeline := &CrossVenueOpportunityTimeline{
		Episodes:    make([]CrossVenueEpisodeObservation, len(episodes)),
		Evaluations: make([]CrossVenueOpportunityObservation, 0, len(evaluations)),
	}
	for index, episode := range episodes {
		if episode.StartSequence == 0 || episode.StartTS < 0 || episode.StartTS > horizonNano || episode.EndTS < episode.StartTS || episode.EndTS > horizonNano ||
			episode.BuyVenue == episode.SellVenue || !containsCrossVenue(venues, episode.BuyVenue) || !containsCrossVenue(venues, episode.SellVenue) ||
			!episode.Censored && episode.EndSequence <= episode.StartSequence || episode.Censored && (episode.EndSequence != 0 || episode.EndTS != horizonNano) ||
			index > 0 && (episodes[index-1].Censored || episode.StartSequence < episodes[index-1].EndSequence) {
			return nil, fmt.Errorf("cross-venue opportunity timeline: malformed or overlapping public episode")
		}
		timeline.Episodes[index].Episode = episode
	}
	lastSourceFrame := map[string]uint64{venues[0]: 0, venues[1]: 0}
	var previousEvaluationFrame uint64
	for index, evaluation := range evaluations {
		source := sources[index]
		row := evaluation.Payload
		if evaluation.Event.GlobalSequence == 0 || evaluation.Event.GlobalSequence <= previousEvaluationFrame ||
			evaluation.Event.SimTS < 0 || evaluation.Event.SimTS > horizonNano || source.Generation != row.Generation || source.Reason != row.Reason ||
			source.VenueID != row.TriggerVenueID || source.SourceSequence != row.TriggerSequence ||
			source.PublishedAt != row.TriggerPublishedAt || source.EvaluatedAt != evaluation.Event.SimTS ||
			source.PublicationFrame == 0 || source.PublicationFrame >= evaluation.Event.GlobalSequence ||
			source.PublishedAt > evaluation.Event.SimTS || !containsCrossVenue(venues, source.VenueID) {
			return nil, fmt.Errorf("cross-venue opportunity timeline: evaluation %d contradicts consumed source", index)
		}
		previousEvaluationFrame = evaluation.Event.GlobalSequence
		if source.PublicationFrame <= lastSourceFrame[source.VenueID] {
			return nil, fmt.Errorf("cross-venue opportunity timeline: source frame did not advance")
		}
		lastSourceFrame[source.VenueID] = source.PublicationFrame
		books, err := crossVenueEvaluationTouches(row.Books, venues)
		if err != nil {
			return nil, err
		}
		local := EvaluateCrossVenueOneLotEdge(venues, books, lotQty, basePrecision, feeBps, true)
		observation := CrossVenueOpportunityObservation{
			Generation: row.Generation, EvaluationFrame: evaluation.Event.GlobalSequence,
			EvaluatedAt: evaluation.Event.SimTS, Reason: row.Reason,
			PublicEpisodeIndex: -1, LocalStatus: local.Status,
			LocalBuyVenue: local.BuyVenue, LocalSellVenue: local.SellVenue,
			SourceFrameByVenue: map[string]uint64{venues[0]: lastSourceFrame[venues[0]], venues[1]: lastSourceFrame[venues[1]]},
		}
		for episodeIndex, episode := range episodes {
			if evaluation.Event.GlobalSequence < episode.StartSequence || !episode.Censored && evaluation.Event.GlobalSequence >= episode.EndSequence {
				continue
			}
			if observation.PublicEpisodeIndex >= 0 {
				return nil, fmt.Errorf("cross-venue opportunity timeline: evaluation lies in overlapping episodes")
			}
			observation.PublicEpisodeIndex = episodeIndex
			observation.PublicBuyVenue, observation.PublicSellVenue = episode.BuyVenue, episode.SellVenue
		}
		observation.Relation = crossVenueOpportunityRelation(observation, local)
		if observation.PublicEpisodeIndex >= 0 {
			observedEpisode := &timeline.Episodes[observation.PublicEpisodeIndex]
			observedEpisode.Evaluations++
			if observation.Relation == "PUBLIC_ACTIVE_LOCAL_ALIGNED" {
				observedEpisode.AlignedEvaluations++
			}
			if row.Reason == "SUBMIT" {
				observedEpisode.Submissions++
			}
		}
		timeline.Evaluations = append(timeline.Evaluations, observation)
	}
	return timeline, nil
}

func crossVenueEvaluationTouches(observed []CrossVenueEvaluationBook, venues [2]string) ([2]CrossVenueTouch, error) {
	var books [2]CrossVenueTouch
	if len(observed) != 2 {
		return books, fmt.Errorf("cross-venue opportunity timeline: incomplete local books")
	}
	seen := [2]bool{}
	for _, row := range observed {
		venueIndex := -1
		for index, venue := range venues {
			if row.VenueID == venue {
				venueIndex = index
				break
			}
		}
		if venueIndex < 0 || seen[venueIndex] {
			return books, fmt.Errorf("cross-venue opportunity timeline: duplicate or unknown local venue")
		}
		seen[venueIndex] = true
		books[venueIndex] = CrossVenueTouch{
			Bid: row.Bid, BidQty: row.BidQty, HasBid: row.HasBid,
			Ask: row.Ask, AskQty: row.AskQty, HasAsk: row.HasAsk,
		}
	}
	return books, nil
}

func crossVenueOpportunityRelation(observation CrossVenueOpportunityObservation, local CrossVenueOneLotEdge) string {
	if observation.PublicEpisodeIndex < 0 {
		if local.Status == "POSITIVE_EDGE" {
			return "LOCAL_POSITIVE_PUBLIC_INACTIVE"
		}
		return "NO_PUBLIC_EDGE_LOCAL_INACTIVE"
	}
	if local.Status != "POSITIVE_EDGE" {
		return "PUBLIC_ACTIVE_LOCAL_INACTIVE"
	}
	if local.BuyVenue == observation.PublicBuyVenue && local.SellVenue == observation.PublicSellVenue {
		return "PUBLIC_ACTIVE_LOCAL_ALIGNED"
	}
	return "PUBLIC_ACTIVE_DIFFERENT_ROUTE"
}

func containsCrossVenue(venues [2]string, venue string) bool {
	return venues[0] == venue || venues[1] == venue
}
