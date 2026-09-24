package analysis

import (
	"fmt"
	"sort"
	"strings"
)

type CrossVenuePlacement struct {
	Event     Event
	Kind      string
	RequestID uint64
	OrderID   uint64
	Side      string
	Qty       int64
	Reason    string
}

type crossVenuePlacementWire struct {
	RequestID   uint64 `json:"request_id"`
	OrderID     uint64 `json:"order_id"`
	Side        string `json:"side"`
	Type        string `json:"type"`
	TimeInForce string `json:"time_in_force"`
	Price       int64  `json:"price"`
	Qty         int64  `json:"qty"`
	Success     bool   `json:"success"`
	Error       string `json:"error"`
}

type crossVenuePlacementRequestKey struct {
	venueID   string
	clientID  uint64
	requestID uint64
}

type crossVenuePlacementOrderKey struct {
	venueID string
	orderID uint64
}

// CollectCrossVenuePlacements independently recovers exchange-side admission
// outcomes for dedicated, venue-local router accounts. It does not infer a
// submitted request from absence of an outcome; the decision-vector and actor
// receipt joins must establish that separate denominator.
func (r *Run) CollectCrossVenuePlacements(venues [2]string, clients map[string]uint64, symbol string, lotQty int64) ([]CrossVenuePlacement, error) {
	if r == nil || venues[0] == "" || venues[1] == "" || venues[0] == venues[1] ||
		len(clients) != 2 || clients[venues[0]] == 0 || clients[venues[1]] == 0 || symbol == "" || lotQty <= 0 {
		return nil, fmt.Errorf("cross-venue placements: invalid selection")
	}
	spotLogName := strings.ReplaceAll(symbol, "/", "-")
	files := make([]string, 0, 2)
	fileVenue := make(map[string]string, 2)
	for _, venue := range venues {
		selected := r.BookFiles(venue, spotLogName)
		if len(selected) != 1 {
			return nil, fmt.Errorf("cross-venue placements: venue %s has %d selected spot logs", venue, len(selected))
		}
		files = append(files, selected[0])
		fileVenue[selected[0]] = venue
	}
	var rows []CrossVenuePlacement
	var scanErr error
	lastByFile := make(map[string]uint64, 2)
	if err := r.Scan(ScanOptions{Events: []string{"OrderAccepted", "OrderRejected", "OrderCancelled", "OrderCancelRejected"}, Files: files, FilesSelected: true, Workers: 1}, func(event Event) {
		if scanErr != nil {
			return
		}
		if fileVenue[event.File] == "" || fileVenue[event.File] != event.VenueID {
			scanErr = fmt.Errorf("cross-venue placements: event venue disagrees with selected file")
			return
		}
		if event.ClientID != clients[event.VenueID] {
			return
		}
		if event.Name == "OrderCancelled" || event.Name == "OrderCancelRejected" {
			scanErr = fmt.Errorf("cross-venue placements: dedicated market-FOK router emitted a cancellation outcome")
			return
		}
		if event.GlobalSequence == 0 || event.GlobalSequence <= lastByFile[event.File] || event.Symbol != "" && event.Symbol != symbol {
			scanErr = fmt.Errorf("cross-venue placements: invalid book event identity")
			return
		}
		lastByFile[event.File] = event.GlobalSequence
		var wire crossVenuePlacementWire
		required := []string{"request_id", "side", "type", "time_in_force", "price", "qty"}
		if event.Name == "OrderAccepted" {
			required = append(required, "order_id")
		} else {
			required = append(required, "success", "error")
		}
		if err := decodeRequiredJSON(event.Raw(), &wire, required...); err != nil {
			scanErr = fmt.Errorf("cross-venue placements: malformed %s: %w", event.Name, err)
			return
		}
		if wire.RequestID == 0 || wire.Side != "BUY" && wire.Side != "SELL" || wire.Type != "MARKET" || wire.TimeInForce != "FOK" || wire.Price != 0 || wire.Qty != lotQty {
			scanErr = fmt.Errorf("cross-venue placements: order contradicts registered FOK lot")
			return
		}
		row := CrossVenuePlacement{Event: event, RequestID: wire.RequestID, Side: wire.Side, Qty: wire.Qty}
		if event.Name == "OrderAccepted" {
			if wire.OrderID == 0 {
				scanErr = fmt.Errorf("cross-venue placements: accepted request lacks order ID")
				return
			}
			row.Kind, row.OrderID = "ACCEPTED", wire.OrderID
		} else {
			if wire.Success || wire.Error == "" || wire.OrderID != 0 {
				scanErr = fmt.Errorf("cross-venue placements: rejected request has contradictory status")
				return
			}
			row.Kind, row.Reason = "REJECTED", wire.Error
		}
		rows = append(rows, row)
	}); err != nil {
		return nil, err
	}
	if scanErr != nil {
		return nil, scanErr
	}
	sort.Slice(rows, func(left, right int) bool { return rows[left].Event.GlobalSequence < rows[right].Event.GlobalSequence })
	requests := make(map[crossVenuePlacementRequestKey]struct{}, len(rows))
	orders := make(map[crossVenuePlacementOrderKey]struct{}, len(rows))
	for index, row := range rows {
		if index > 0 && row.Event.GlobalSequence == rows[index-1].Event.GlobalSequence {
			return nil, fmt.Errorf("cross-venue placements: duplicate global frame identity")
		}
		request := crossVenuePlacementRequestKey{row.Event.VenueID, row.Event.ClientID, row.RequestID}
		if _, duplicate := requests[request]; duplicate {
			return nil, fmt.Errorf("cross-venue placements: duplicate request outcome")
		}
		requests[request] = struct{}{}
		if row.Kind == "ACCEPTED" {
			order := crossVenuePlacementOrderKey{row.Event.VenueID, row.OrderID}
			if _, duplicate := orders[order]; duplicate {
				return nil, fmt.Errorf("cross-venue placements: duplicate accepted order ID")
			}
			orders[order] = struct{}{}
		}
	}
	return rows, nil
}
