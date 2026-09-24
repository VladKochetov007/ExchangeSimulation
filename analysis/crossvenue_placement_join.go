package analysis

import (
	"fmt"

	etypes "exchange_sim/types"
)

type CrossVenuePlacementResult struct {
	VenueID    string
	ClientID   uint64
	RequestID  uint64
	OrderID    uint64
	Kind       string
	Reason     string
	Side       string
	ExchangeAt int64
	InboxAt    *int64
	InboxFrame uint64
	FilledQty  int64
}

// ReconcileCrossVenuePlacementReceipts checks the exchange-side FOK outcome,
// settled fills and actor-inbox acknowledgement as distinct timelines. A
// missing inbox acknowledgement is retained as pending delivery, not turned
// into a failed exchange execution. The caller must separately prove the
// submitted-request vectors and receipt/fill stream completeness.
func ReconcileCrossVenuePlacementReceipts(placements []CrossVenuePlacement, receipts []CrossVenueResponseReceiptRecord, fills []CrossVenueExchangeFill, horizonNano, lotQty int64) ([]CrossVenuePlacementResult, error) {
	if horizonNano <= 0 || lotQty <= 0 {
		return nil, fmt.Errorf("cross-venue placement join: invalid horizon or lot")
	}
	results := make([]CrossVenuePlacementResult, len(placements))
	requests := make(map[crossVenuePlacementRequestKey]int, len(placements))
	orders := make(map[crossVenuePlacementOrderKey]int, len(placements))
	var previousSequence uint64
	for index, placement := range placements {
		if placement.Event.GlobalSequence == 0 || placement.Event.GlobalSequence <= previousSequence || placement.Event.SimTS < 0 ||
			placement.Event.SimTS > horizonNano || placement.Event.VenueID == "" || placement.Event.ClientID == 0 || placement.RequestID == 0 ||
			placement.Qty != lotQty || placement.Side != "BUY" && placement.Side != "SELL" {
			return nil, fmt.Errorf("cross-venue placement join: invalid exchange placement %d", index)
		}
		previousSequence = placement.Event.GlobalSequence
		key := crossVenuePlacementRequestKey{placement.Event.VenueID, placement.Event.ClientID, placement.RequestID}
		if _, duplicate := requests[key]; duplicate {
			return nil, fmt.Errorf("cross-venue placement join: duplicate request identity")
		}
		requests[key] = index
		result := CrossVenuePlacementResult{
			VenueID: placement.Event.VenueID, ClientID: placement.Event.ClientID, RequestID: placement.RequestID,
			OrderID: placement.OrderID, Kind: placement.Kind, Reason: placement.Reason, Side: placement.Side,
			ExchangeAt: placement.Event.SimTS,
		}
		switch placement.Kind {
		case "ACCEPTED":
			if placement.OrderID == 0 || placement.Reason != "" {
				return nil, fmt.Errorf("cross-venue placement join: invalid accepted order")
			}
			order := crossVenuePlacementOrderKey{placement.Event.VenueID, placement.OrderID}
			if _, duplicate := orders[order]; duplicate {
				return nil, fmt.Errorf("cross-venue placement join: duplicate accepted order")
			}
			orders[order] = index
		case "REJECTED":
			if placement.OrderID != 0 || placement.Reason == "" {
				return nil, fmt.Errorf("cross-venue placement join: invalid rejected order")
			}
		default:
			return nil, fmt.Errorf("cross-venue placement join: unsupported outcome %q", placement.Kind)
		}
		results[index] = result
	}
	seenReceipts := make(map[crossVenuePlacementRequestKey]struct{}, len(receipts))
	for _, receipt := range receipts {
		if receipt.Payload.Kind == "FILL" || receipt.Payload.Kind == "OTHER" {
			continue
		}
		if receipt.Payload.Kind != "ORDER_ACCEPTED" && receipt.Payload.Kind != "REJECTED" {
			return nil, fmt.Errorf("cross-venue placement join: unexpected order response %q", receipt.Payload.Kind)
		}
		key := crossVenuePlacementRequestKey{receipt.Payload.VenueID, receipt.Payload.ClientID, receipt.Payload.RequestID}
		index, exists := requests[key]
		if _, duplicate := seenReceipts[key]; !exists || duplicate {
			return nil, fmt.Errorf("cross-venue placement join: unanchored or duplicate acknowledgement")
		}
		seenReceipts[key] = struct{}{}
		placement := placements[index]
		if receipt.Event.VenueID != key.venueID || receipt.Event.ClientID != key.clientID || receipt.Event.GlobalSequence <= placement.Event.GlobalSequence ||
			receipt.Event.SimTS != receipt.Payload.ReceivedAt || receipt.Payload.ReceivedAt < placement.Event.SimTS || receipt.Payload.ReceivedAt > horizonNano {
			return nil, fmt.Errorf("cross-venue placement join: acknowledgement violates exchange/inbox order")
		}
		if placement.Kind == "ACCEPTED" && (receipt.Payload.Kind != "ORDER_ACCEPTED" || receipt.Payload.OrderID != placement.OrderID || !receipt.Payload.Success) ||
			placement.Kind == "REJECTED" && (receipt.Payload.Kind != "REJECTED" || receipt.Payload.Error != etypes.RejectReason(placement.Reason) || receipt.Payload.Success) {
			return nil, fmt.Errorf("cross-venue placement join: acknowledgement contradicts exchange outcome")
		}
		receivedAt := receipt.Payload.ReceivedAt
		results[index].InboxAt = &receivedAt
		results[index].InboxFrame = receipt.Event.GlobalSequence
	}
	for _, fill := range fills {
		key := crossVenuePlacementOrderKey{fill.Event.VenueID, fill.OrderID}
		index, exists := orders[key]
		if !exists || fill.Event.ClientID != placements[index].Event.ClientID || fill.Event.GlobalSequence <= placements[index].Event.GlobalSequence ||
			fill.Event.SimTS < placements[index].Event.SimTS || fill.Event.SimTS > horizonNano || fill.Qty <= 0 ||
			fill.Side != placements[index].Side || fill.Role != "taker" {
			return nil, fmt.Errorf("cross-venue placement join: fill has no prior accepted FOK leg")
		}
		quantity, ok := etypes.TryAdd(results[index].FilledQty, fill.Qty)
		if !ok || quantity > lotQty {
			return nil, fmt.Errorf("cross-venue placement join: FOK fill quantity overrun")
		}
		results[index].FilledQty = quantity
	}
	for _, result := range results {
		if result.Kind == "ACCEPTED" && result.FilledQty != lotQty {
			return nil, fmt.Errorf("cross-venue placement join: accepted FOK lacks full settled fill")
		}
	}
	return results, nil
}
