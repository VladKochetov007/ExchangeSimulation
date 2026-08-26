package analysis

import (
	"sort"
	"sync"
)

// OrderLifecycleAudit independently reconstructs the terminal state of every
// accepted order from the persisted acceptance, fill, and cancellation records.
// It deliberately treats only market and non-GTC orders as required to have a
// terminal event: a GTC order can legitimately remain resting when a finite run
// stops, and an unlogged cancel request cannot be inferred from the evidence.
type OrderLifecycleAudit struct {
	Accepted                  int                   `json:"accepted"`
	FillRecords               int                   `json:"fill_records"`
	Cancelled                 int                   `json:"cancelled"`
	FullyFilled               int                   `json:"fully_filled"`
	RequiredImmediateTerminal int                   `json:"required_immediate_terminal"`
	MissingImmediateTerminal  int                   `json:"missing_immediate_terminal"`
	UnknownFills              int                   `json:"unknown_fills"`
	UnknownCancellations      int                   `json:"unknown_cancellations"`
	DuplicateAcceptances      int                   `json:"duplicate_acceptances"`
	DuplicateTerminals        int                   `json:"duplicate_terminals"`
	FillsAfterTerminal        int                   `json:"fills_after_terminal"`
	FillQuantityMismatches    int                   `json:"fill_quantity_mismatches"`
	CancelQuantityMismatches  int                   `json:"cancel_quantity_mismatches"`
	ClientMismatches          int                   `json:"client_mismatches"`
	Checks                    []OrderLifecycleCheck `json:"checks,omitempty"`
}

// OrderLifecycleCheck names a broken order-level evidence contract. Checks are
// emitted in deterministic venue/order order so artifacts remain comparable.
type OrderLifecycleCheck struct {
	VenueID string `json:"venue_id"`
	// File identifies the instrument/book log. Order IDs are allocated by a
	// venue but may be reused across independent books, so venue+order ID is
	// not a sufficient lifecycle key.
	File    string `json:"file"`
	OrderID uint64 `json:"order_id"`
	Failure string `json:"failure"`
}

type orderLifecycleKey struct {
	venueID string
	file    string
	orderID uint64
}

type orderLifecycleState struct {
	clientID  uint64
	quantity  int64
	filled    int64
	immediate bool
	terminal  bool
	failed    map[string]bool
}

// MeasureOrderLifecycle reconstructs accepted orders from their persisted
// lifecycle records. It is an evidence-contract audit, not a replacement for
// the matcher: it catches missing terminal records and invalid ordered fill
// transitions that accounting identities can leave invisible.
func (r *Run) MeasureOrderLifecycle() (*OrderLifecycleAudit, error) {
	type acceptedPayload struct {
		OrderID     uint64 `json:"order_id"`
		ClientID    uint64 `json:"client_id"`
		Type        string `json:"type"`
		TimeInForce string `json:"time_in_force"`
		Qty         int64  `json:"qty"`
	}
	type fillPayload struct {
		OrderID      uint64 `json:"order_id"`
		Qty          int64  `json:"qty"`
		FilledQty    int64  `json:"filled_qty"`
		RemainingQty int64  `json:"remaining_qty"`
		IsFull       bool   `json:"is_full"`
	}
	type cancelPayload struct {
		OrderID      uint64 `json:"order_id"`
		RemainingQty int64  `json:"remaining_qty"`
	}

	result := &OrderLifecycleAudit{}
	states := make(map[orderLifecycleKey]*orderLifecycleState)
	var mu sync.Mutex
	addFailure := func(key orderLifecycleKey, state *orderLifecycleState, failure string) {
		if state != nil {
			if state.failed == nil {
				state.failed = make(map[string]bool)
			}
			if state.failed[failure] {
				return
			}
			state.failed[failure] = true
		}
		result.Checks = append(result.Checks, OrderLifecycleCheck{VenueID: key.venueID, File: key.file, OrderID: key.orderID, Failure: failure})
	}

	// Lifecycle state is order-sensitive within each book. A single worker
	// preserves file order; the file component of the key still prevents
	// cross-book order-ID reuse from colliding.
	scan := ScanOptions{Events: []string{"OrderAccepted", "OrderFill", "OrderCancelled"}, Workers: 1}
	if err := r.Scan(scan, func(event Event) {
		mu.Lock()
		defer mu.Unlock()
		switch event.Name {
		case "OrderAccepted":
			var payload acceptedPayload
			if event.Decode(&payload) != nil || payload.OrderID == 0 || payload.Qty <= 0 {
				return
			}
			key := orderLifecycleKey{venueID: event.VenueID, file: event.File, orderID: payload.OrderID}
			if states[key] != nil {
				result.DuplicateAcceptances++
				addFailure(key, states[key], "duplicate_acceptance")
				return
			}
			clientID := payload.ClientID
			if clientID == 0 {
				clientID = event.ClientID
			}
			states[key] = &orderLifecycleState{
				clientID:  clientID,
				quantity:  payload.Qty,
				immediate: payload.Type != "LIMIT" || payload.TimeInForce != "GTC",
			}
			result.Accepted++
		case "OrderFill":
			var payload fillPayload
			if event.Decode(&payload) != nil || payload.OrderID == 0 || payload.Qty <= 0 {
				return
			}
			result.FillRecords++
			key := orderLifecycleKey{venueID: event.VenueID, file: event.File, orderID: payload.OrderID}
			state := states[key]
			if state == nil {
				result.UnknownFills++
				addFailure(key, nil, "fill_without_acceptance")
				return
			}
			if state.clientID != 0 && event.ClientID != state.clientID {
				result.ClientMismatches++
				addFailure(key, state, "fill_client_mismatch")
			}
			if state.terminal {
				result.FillsAfterTerminal++
				addFailure(key, state, "fill_after_terminal")
			}
			state.filled += payload.Qty
			if state.filled != payload.FilledQty || payload.RemainingQty != state.quantity-state.filled || state.filled > state.quantity || (payload.IsFull && state.filled != state.quantity) {
				result.FillQuantityMismatches++
				addFailure(key, state, "fill_quantity_mismatch")
			}
			if state.filled == state.quantity || payload.IsFull {
				if state.terminal {
					result.DuplicateTerminals++
					addFailure(key, state, "duplicate_terminal")
				} else {
					state.terminal = true
					result.FullyFilled++
				}
			}
		case "OrderCancelled":
			var payload cancelPayload
			if event.Decode(&payload) != nil || payload.OrderID == 0 {
				return
			}
			result.Cancelled++
			key := orderLifecycleKey{venueID: event.VenueID, file: event.File, orderID: payload.OrderID}
			state := states[key]
			if state == nil {
				result.UnknownCancellations++
				addFailure(key, nil, "cancellation_without_acceptance")
				return
			}
			if state.clientID != 0 && event.ClientID != state.clientID {
				result.ClientMismatches++
				addFailure(key, state, "cancellation_client_mismatch")
			}
			if payload.RemainingQty != state.quantity-state.filled {
				result.CancelQuantityMismatches++
				addFailure(key, state, "cancellation_quantity_mismatch")
			}
			if state.terminal {
				result.DuplicateTerminals++
				addFailure(key, state, "duplicate_terminal")
			} else {
				state.terminal = true
			}
		}
	}); err != nil {
		return nil, err
	}

	for key, state := range states {
		if state.immediate {
			result.RequiredImmediateTerminal++
			if !state.terminal {
				result.MissingImmediateTerminal++
				addFailure(key, state, "missing_immediate_terminal")
			}
		}
	}
	sort.Slice(result.Checks, func(i, j int) bool {
		if result.Checks[i].VenueID != result.Checks[j].VenueID {
			return result.Checks[i].VenueID < result.Checks[j].VenueID
		}
		if result.Checks[i].File != result.Checks[j].File {
			return result.Checks[i].File < result.Checks[j].File
		}
		if result.Checks[i].OrderID != result.Checks[j].OrderID {
			return result.Checks[i].OrderID < result.Checks[j].OrderID
		}
		return result.Checks[i].Failure < result.Checks[j].Failure
	})
	return result, nil
}
