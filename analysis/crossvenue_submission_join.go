package analysis

import (
	"fmt"

	etypes "exchange_sim/types"
)

type CrossVenueSubmissionLeg struct {
	Decision  SelectedDecisionFrontierVector
	Placement CrossVenuePlacement
}

type CrossVenueSubmissionGroup struct {
	Generation uint64
	Buy        CrossVenueSubmissionLeg
	Sell       CrossVenueSubmissionLeg
}

// BindCrossVenueSubmissionOutcomes joins every submitted local-feed evaluation
// to two independently audited gateway decisions and two exchange outcomes.
// The caller must pass the complete selected vector set for the router's two
// required links and the complete selected placement set for its accounts.
func BindCrossVenueSubmissionOutcomes(evaluations []CrossVenueEvaluationRecord, decisions []SelectedDecisionFrontierVector, placements []CrossVenuePlacement, symbol string, lotQty int64, maxAttempts int) ([]CrossVenueSubmissionGroup, error) {
	if symbol == "" || lotQty <= 0 || maxAttempts <= 0 {
		return nil, fmt.Errorf("cross-venue submission join: invalid contract")
	}
	var submits []CrossVenueEvaluationRecord
	var lastEvaluationFrame uint64
	for index, evaluation := range evaluations {
		if evaluation.Event.GlobalSequence == 0 || index > 0 && evaluation.Event.GlobalSequence <= lastEvaluationFrame {
			return nil, fmt.Errorf("cross-venue submission join: evaluations out of frame order")
		}
		lastEvaluationFrame = evaluation.Event.GlobalSequence
		if evaluation.Payload.Reason == "SUBMIT" {
			submits = append(submits, evaluation)
		}
	}
	if len(submits) > maxAttempts || len(decisions) != 2*len(submits) || len(placements) != 2*len(submits) {
		return nil, fmt.Errorf("cross-venue submission join: submitted groups, decisions and exchange outcomes disagree")
	}
	placementByRequest := make(map[crossVenuePlacementRequestKey]int, len(placements))
	for index, placement := range placements {
		key := crossVenuePlacementRequestKey{placement.Event.VenueID, placement.Event.ClientID, placement.RequestID}
		if key.venueID == "" || key.clientID == 0 || key.requestID == 0 {
			return nil, fmt.Errorf("cross-venue submission join: placement lacks request identity")
		}
		if _, duplicate := placementByRequest[key]; duplicate {
			return nil, fmt.Errorf("cross-venue submission join: duplicate placement request")
		}
		placementByRequest[key] = index
	}
	claimed := make(map[int]struct{}, len(placements))
	groups := make([]CrossVenueSubmissionGroup, 0, len(submits))
	var lastDecisionID uint64
	for groupIndex, evaluation := range submits {
		row := evaluation.Payload
		if row.Generation == 0 || row.QuotedEdge <= 0 || row.SelectedBuy == "" || row.SelectedSell == "" || row.SelectedBuy == row.SelectedSell || len(row.Feeds) != 2 {
			return nil, fmt.Errorf("cross-venue submission join: invalid selected route")
		}
		feeds := make(map[string]CrossVenueEvaluationFeed, 2)
		for _, feed := range row.Feeds {
			if feed.VenueID == "" || feed.ClientID == 0 || feed.Frontier.LinkID == 0 || feed.Frontier.Ordinal == 0 ||
				feed.Frontier.DeliveredAt > evaluation.Event.SimTS || feed.Frontier.Digest == ([16]byte{}) {
				return nil, fmt.Errorf("cross-venue submission join: incomplete selected feed")
			}
			if _, duplicate := feeds[feed.VenueID]; duplicate {
				return nil, fmt.Errorf("cross-venue submission join: duplicate venue feed")
			}
			feeds[feed.VenueID] = feed
		}
		if _, exists := feeds[row.SelectedBuy]; !exists {
			return nil, fmt.Errorf("cross-venue submission join: selected buy feed missing")
		}
		if _, exists := feeds[row.SelectedSell]; !exists {
			return nil, fmt.Errorf("cross-venue submission join: selected sell feed missing")
		}
		group := CrossVenueSubmissionGroup{Generation: row.Generation}
		for legIndex, venueID := range []string{row.SelectedBuy, row.SelectedSell} {
			decision := decisions[2*groupIndex+legIndex]
			feed := feeds[venueID]
			expectedSide := etypes.Buy
			placementSide := "BUY"
			if legIndex == 1 {
				expectedSide, placementSide = etypes.Sell, "SELL"
			}
			if decision.DecisionID == 0 || decision.DecisionID <= lastDecisionID || decision.ActorID == 0 ||
				decision.ClientID != feed.ClientID || decision.TradingLinkID != feed.Frontier.LinkID || decision.RequestID == 0 ||
				decision.Symbol != symbol || decision.Side != expectedSide || decision.OrderType != etypes.Market ||
				decision.TimeInForce != etypes.FOK || decision.Price != 0 || decision.Qty != lotQty ||
				decision.DecisionAt != evaluation.Event.SimTS || decision.ComponentCount != uint32(len(feeds)) ||
				len(decision.Components) != len(feeds) {
				return nil, fmt.Errorf("cross-venue submission join: decision %d contradicts submitted FOK leg", legIndex)
			}
			lastDecisionID = decision.DecisionID
			if err := matchCrossVenueDecisionComponents(decision.Components, feeds); err != nil {
				return nil, err
			}
			key := crossVenuePlacementRequestKey{venueID, feed.ClientID, decision.RequestID}
			placementIndex, exists := placementByRequest[key]
			if _, duplicate := claimed[placementIndex]; !exists || duplicate {
				return nil, fmt.Errorf("cross-venue submission join: decision lacks unique exchange placement")
			}
			placement := placements[placementIndex]
			if placement.Event.GlobalSequence <= evaluation.Event.GlobalSequence || placement.Event.SimTS < decision.DecisionAt ||
				placement.Side != placementSide || placement.Qty != lotQty ||
				placement.Kind != "ACCEPTED" && placement.Kind != "REJECTED" {
				return nil, fmt.Errorf("cross-venue submission join: venue placement predates or contradicts decision")
			}
			claimed[placementIndex] = struct{}{}
			leg := CrossVenueSubmissionLeg{Decision: decision, Placement: placement}
			if legIndex == 0 {
				group.Buy = leg
			} else {
				group.Sell = leg
			}
		}
		groups = append(groups, group)
	}
	if len(claimed) != len(placements) {
		return nil, fmt.Errorf("cross-venue submission join: exchange placement has no submitted decision")
	}
	return groups, nil
}

func matchCrossVenueDecisionComponents(components []SelectedDecisionFrontierComponent, feeds map[string]CrossVenueEvaluationFeed) error {
	expected := make(map[crossVenueFrontierComponentKey]CrossVenueEvaluationFrontier, len(feeds))
	for _, feed := range feeds {
		key := crossVenueFrontierComponentKey{feed.ClientID, feed.Frontier.LinkID}
		if _, duplicate := expected[key]; duplicate {
			return fmt.Errorf("cross-venue submission join: duplicate feed client/link")
		}
		expected[key] = feed.Frontier
	}
	seen := make(map[crossVenueFrontierComponentKey]struct{}, len(components))
	for _, component := range components {
		key := crossVenueFrontierComponentKey{component.ClientID, component.LinkID}
		frontier, exists := expected[key]
		if _, duplicate := seen[key]; !exists || duplicate || component.Ordinal != frontier.Ordinal ||
			component.DeliveredAt != frontier.DeliveredAt || component.Digest != frontier.Digest {
			return fmt.Errorf("cross-venue submission join: decision component differs from consumed feed")
		}
		seen[key] = struct{}{}
	}
	return nil
}

type crossVenueFrontierComponentKey struct {
	clientID uint64
	linkID   uint32
}
