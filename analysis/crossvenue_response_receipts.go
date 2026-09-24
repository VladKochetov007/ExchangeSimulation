package analysis

import (
	"fmt"
	"path/filepath"
	"sort"

	etypes "exchange_sim/types"
)

type CrossVenueResponseReceiptPayload struct {
	RouterID     uint64              `json:"router_id"`
	ActorID      uint64              `json:"actor_id"`
	ClientID     uint64              `json:"client_id"`
	VenueID      string              `json:"venue_id"`
	ReceivedAt   int64               `json:"received_at"`
	Kind         string              `json:"kind"`
	RequestID    uint64              `json:"request_id"`
	Success      bool                `json:"success"`
	Error        etypes.RejectReason `json:"error"`
	OrderID      uint64              `json:"order_id"`
	TradeID      uint64              `json:"trade_id"`
	Symbol       string              `json:"symbol"`
	Side         string              `json:"side"`
	Qty          int64               `json:"qty"`
	Price        int64               `json:"price"`
	FeeAmount    int64               `json:"fee_amount"`
	FeeAsset     string              `json:"fee_asset"`
	ExchangeAt   int64               `json:"exchange_at"`
	IsFull       bool                `json:"is_full"`
	RemainingQty int64               `json:"remaining_qty"`
}

type CrossVenueResponseReceiptRecord struct {
	Event   Event
	Payload CrossVenueResponseReceiptPayload
}

var crossVenueResponseReceiptFields = []string{
	"router_id", "actor_id", "client_id", "venue_id", "received_at", "kind",
	"request_id", "success", "error", "order_id", "trade_id", "symbol", "side",
	"qty", "price", "fee_amount", "fee_asset", "exchange_at", "is_full", "remaining_qty",
}

// CollectCrossVenueResponseReceipts verifies actor-inbox receipts against the
// terminal router count. The caller must obtain expectedCount from a verified
// run report; this function does not yet reconcile exchange execution events.
func (r *Run) CollectCrossVenueResponseReceipts(venues [2]string, routerID, expectedCount uint64) ([]CrossVenueResponseReceiptRecord, error) {
	if r == nil || routerID == 0 || venues[0] == "" || venues[1] == "" || venues[0] == venues[1] {
		return nil, fmt.Errorf("cross-venue response receipts: invalid selection")
	}
	files, err := selectCrossVenueGeneralFiles(r, venues)
	if err != nil {
		return nil, fmt.Errorf("cross-venue response receipts: %w", err)
	}
	var records []CrossVenueResponseReceiptRecord
	var callbackErr error
	if err := r.Scan(ScanOptions{Events: []string{"cross_venue_arb_response_receipt"}, Files: files, FilesSelected: true, Workers: 1}, func(event Event) {
		if callbackErr != nil {
			return
		}
		var payload CrossVenueResponseReceiptPayload
		if err := decodeRequiredJSON(event.Raw(), &payload, crossVenueResponseReceiptFields...); err != nil {
			callbackErr = fmt.Errorf("cross-venue response receipts: malformed row: %w", err)
			return
		}
		if payload.RouterID == routerID {
			records = append(records, CrossVenueResponseReceiptRecord{Event: event, Payload: payload})
		}
	}); err != nil {
		return nil, err
	}
	if callbackErr != nil {
		return nil, callbackErr
	}
	if uint64(len(records)) != expectedCount {
		return nil, fmt.Errorf("cross-venue response receipts: %d rows disagree with terminal count %d", len(records), expectedCount)
	}
	sort.Slice(records, func(left, right int) bool {
		return records[left].Event.GlobalSequence < records[right].Event.GlobalSequence
	})
	for index, record := range records {
		if record.Event.GlobalSequence == 0 || index > 0 && record.Event.GlobalSequence <= records[index-1].Event.GlobalSequence {
			return nil, fmt.Errorf("cross-venue response receipts: duplicate or missing global frame identity")
		}
		if err := validateCrossVenueResponseReceipt(record, venues, routerID); err != nil {
			return nil, fmt.Errorf("cross-venue response receipts: row %d: %w", index, err)
		}
	}
	return records, nil
}

func selectCrossVenueGeneralFiles(run *Run, venues [2]string) ([]string, error) {
	files := make([]string, 0, 2)
	for _, venue := range venues {
		selected := ""
		for _, path := range run.Files() {
			if filepath.Base(path) != "general.jsonl" || filepath.Base(filepath.Dir(path)) != venue {
				continue
			}
			if selected != "" {
				return nil, fmt.Errorf("duplicate general log for %s", venue)
			}
			selected = path
		}
		if selected == "" {
			return nil, fmt.Errorf("missing general log for %s", venue)
		}
		files = append(files, selected)
	}
	return files, nil
}

func validateCrossVenueResponseReceipt(record CrossVenueResponseReceiptRecord, venues [2]string, routerID uint64) error {
	row := record.Payload
	if row.RouterID == 0 || row.RouterID != routerID || row.ActorID == 0 || row.ClientID == 0 || row.VenueID != venues[0] && row.VenueID != venues[1] || record.Event.VenueID != row.VenueID || record.Event.ClientID != row.ClientID || record.Event.SimTS != row.ReceivedAt || row.ReceivedAt < 0 {
		return fmt.Errorf("actor, venue, router or receipt-time identity mismatch")
	}
	if row.Kind != "REJECTED" && !row.Success || row.Kind == "REJECTED" && row.Success {
		return fmt.Errorf("response status contradicts kind")
	}
	switch row.Kind {
	case "ORDER_ACCEPTED":
		if row.RequestID == 0 || row.OrderID == 0 {
			return fmt.Errorf("accepted order lacks request/order identity")
		}
	case "REJECTED":
		if row.RequestID == 0 {
			return fmt.Errorf("rejection lacks request identity")
		}
	case "FILL":
		if row.OrderID == 0 || row.Symbol == "" || row.Qty <= 0 || row.ExchangeAt < 0 || row.ExchangeAt > row.ReceivedAt || row.Side != etypes.Buy.String() && row.Side != etypes.Sell.String() {
			return fmt.Errorf("fill has invalid exchange or execution identity")
		}
	case "ORDER_CANCELLED":
		if row.RequestID == 0 || row.RemainingQty < 0 {
			return fmt.Errorf("order cancellation has invalid identity or quantity")
		}
	case "FORCED_CANCELLED":
		if row.OrderID == 0 || row.RemainingQty < 0 {
			return fmt.Errorf("forced cancellation has invalid identity or quantity")
		}
	case "OTHER":
		// Subscription responses have no order or fill identity.
	default:
		return fmt.Errorf("unsupported response kind %q", row.Kind)
	}
	return nil
}
