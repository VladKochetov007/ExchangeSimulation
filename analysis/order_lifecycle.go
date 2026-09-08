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
	Accepted                  int `json:"accepted"`
	FillRecords               int `json:"fill_records"`
	Cancelled                 int `json:"cancelled"`
	FullyFilled               int `json:"fully_filled"`
	RequiredImmediateTerminal int `json:"required_immediate_terminal"`
	MissingImmediateTerminal  int `json:"missing_immediate_terminal"`
	UnknownFills              int `json:"unknown_fills"`
	// LiquidationFills are forced-close fills emitted by the exchange without
	// an ordinary OrderAccepted event. They are valid only when linked to a
	// same-file, same-time, same-client liquidation record; UnlinkedFills are
	// evidence-contract failures.
	LiquidationFills            int                   `json:"liquidation_fills"`
	UnlinkedFills               int                   `json:"unlinked_fills"`
	LiquidationIdentityFailures int                   `json:"liquidation_identity_failures"`
	MissingForcedFills          int                   `json:"missing_forced_fills"`
	UnknownCancellations        int                   `json:"unknown_cancellations"`
	DuplicateAcceptances        int                   `json:"duplicate_acceptances"`
	DuplicateTerminals          int                   `json:"duplicate_terminals"`
	FillsAfterTerminal          int                   `json:"fills_after_terminal"`
	FillQuantityMismatches      int                   `json:"fill_quantity_mismatches"`
	CancelQuantityMismatches    int                   `json:"cancel_quantity_mismatches"`
	ClientMismatches            int                   `json:"client_mismatches"`
	MalformedAcceptedRecords    int                   `json:"malformed_accepted_records"`
	MalformedFillRecords        int                   `json:"malformed_fill_records"`
	MalformedCancelRecords      int                   `json:"malformed_cancel_records"`
	MalformedLiquidations       int                   `json:"malformed_liquidation_records"`
	Checks                      []OrderLifecycleCheck `json:"checks,omitempty"`
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

type orderLifecycleLiquidationKey struct {
	file          string
	timestamp     int64
	clientID      uint64
	symbol        string
	positionSide  string
	liquidationID uint64
}

type orderLifecycleLiquidationReceipt struct {
	forcedOrderID uint64
	positionSide  string
	positionSize  int64
	attemptedQty  int64
	filledQty     int64
	remainingQty  int64
}

type orderLifecycleUnknownFill struct {
	key           orderLifecycleKey
	timestamp     int64
	ordinal       int64
	clientID      uint64
	symbol        string
	orderID       uint64
	quantity      int64
	filledQty     int64
	remainingQty  int64
	isFull        bool
	side          string
	positionSide  string
	forced        *bool
	liquidationID *uint64
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
		OrderID       uint64  `json:"order_id"`
		Symbol        string  `json:"symbol"`
		Qty           int64   `json:"qty"`
		FilledQty     int64   `json:"filled_qty"`
		RemainingQty  int64   `json:"remaining_qty"`
		IsFull        bool    `json:"is_full"`
		Side          string  `json:"side"`
		PositionSide  string  `json:"position_side"`
		Forced        *bool   `json:"forced"`
		LiquidationID *uint64 `json:"liquidation_id"`
	}
	type liquidationPayload struct {
		Symbol        string `json:"symbol"`
		PositionSide  string `json:"position_side"`
		LiquidationID uint64 `json:"liquidation_id"`
		ForcedOrderID uint64 `json:"forced_order_id"`
		PositionSize  int64  `json:"position_size"`
		AttemptedQty  int64  `json:"attempted_qty"`
		FilledQty     int64  `json:"filled_qty"`
		RemainingQty  int64  `json:"remaining_qty"`
	}
	type cancelPayload struct {
		OrderID      uint64 `json:"order_id"`
		RemainingQty int64  `json:"remaining_qty"`
	}

	result := &OrderLifecycleAudit{}
	states := make(map[orderLifecycleKey]*orderLifecycleState)
	liquidations := make(map[orderLifecycleLiquidationKey]orderLifecycleLiquidationReceipt)
	unknownFills := make([]orderLifecycleUnknownFill, 0)
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
	scan := ScanOptions{Events: []string{"OrderAccepted", "OrderFill", "OrderCancelled", "liquidation"}, Workers: 1}
	if err := r.Scan(scan, func(event Event) {
		mu.Lock()
		defer mu.Unlock()
		switch event.Name {
		case "OrderAccepted":
			var payload acceptedPayload
			if err := decodeRequiredJSON(event.Raw(), &payload, "order_id", "type", "time_in_force", "qty"); err != nil || payload.OrderID == 0 || payload.Type == "" || payload.TimeInForce == "" || payload.Qty <= 0 {
				result.MalformedAcceptedRecords++
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
			if err := decodeRequiredJSON(event.Raw(), &payload, "order_id", "qty", "filled_qty", "remaining_qty", "is_full"); err != nil || payload.OrderID == 0 || payload.Qty <= 0 {
				result.MalformedFillRecords++
				return
			}
			result.FillRecords++
			key := orderLifecycleKey{venueID: event.VenueID, file: event.File, orderID: payload.OrderID}
			state := states[key]
			if state == nil {
				result.UnknownFills++
				symbol := payload.Symbol
				if symbol == "" {
					symbol = event.Symbol
				}
				unknownFills = append(unknownFills, orderLifecycleUnknownFill{
					key: key, timestamp: event.SimTS, ordinal: event.Ordinal,
					clientID: event.ClientID, symbol: symbol, orderID: payload.OrderID,
					quantity: payload.Qty, filledQty: payload.FilledQty,
					remainingQty: payload.RemainingQty, isFull: payload.IsFull,
					side: payload.Side, positionSide: payload.PositionSide,
					forced: payload.Forced, liquidationID: payload.LiquidationID,
				})
				return
			}
			if r.strictLifecycleIdentity && ((payload.Forced != nil && *payload.Forced) || (payload.LiquidationID != nil && *payload.LiquidationID != 0)) {
				// A synthetic liquidation order is intentionally absent from the
				// acceptance stream. If its identity collides with an accepted order,
				// accepting the row as an ordinary fill would make a corrupted stream
				// look valid while mutating the wrong lifecycle state.
				result.LiquidationIdentityFailures++
				addFailure(key, state, "accepted_order_has_forced_identity")
			}
			if state.clientID != 0 && event.ClientID != state.clientID {
				result.ClientMismatches++
				addFailure(key, state, "fill_client_mismatch")
			}
			if state.terminal {
				result.FillsAfterTerminal++
				addFailure(key, state, "fill_after_terminal")
			}
			filled, arithmeticOK := exactAdd(state.filled, payload.Qty)
			remaining, remainingOK := exactSub(state.quantity, filled)
			if !arithmeticOK || !remainingOK {
				result.FillQuantityMismatches++
				addFailure(key, state, "fill_quantity_mismatch")
				return
			}
			state.filled = filled
			if state.filled != payload.FilledQty || payload.RemainingQty != remaining || state.filled > state.quantity || (payload.IsFull && state.filled != state.quantity) || (state.filled == state.quantity && !payload.IsFull) {
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
			if err := decodeRequiredJSON(event.Raw(), &payload, "order_id", "remaining_qty"); err != nil || payload.OrderID == 0 {
				result.MalformedCancelRecords++
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
		case "liquidation":
			var payload liquidationPayload
			required := []string{"symbol"}
			if r.strictLifecycleIdentity {
				required = []string{"symbol", "position_side", "liquidation_id", "forced_order_id", "position_size", "attempted_qty", "filled_qty", "remaining_qty"}
			}
			if err := decodeRequiredJSON(event.Raw(), &payload, required...); err != nil {
				result.MalformedLiquidations++
				return
			}
			symbol := payload.Symbol
			if symbol == "" {
				symbol = event.Symbol
			}
			if symbol == "" || event.ClientID == 0 {
				result.MalformedLiquidations++
				return
			}
			if r.strictLifecycleIdentity && (payload.LiquidationID == 0 || payload.ForcedOrderID == 0 || payload.PositionSide == "" || payload.PositionSize == 0 || payload.AttemptedQty <= 0 || payload.FilledQty <= 0 || payload.FilledQty > payload.AttemptedQty || payload.RemainingQty != payload.AttemptedQty-payload.FilledQty) {
				result.MalformedLiquidations++
				return
			}
			liquidationKey := orderLifecycleLiquidationKey{
				file: event.File, timestamp: event.SimTS, clientID: event.ClientID, symbol: symbol,
			}
			receipt := orderLifecycleLiquidationReceipt{}
			if r.strictLifecycleIdentity {
				liquidationKey.positionSide = payload.PositionSide
				liquidationKey.liquidationID = payload.LiquidationID
				receipt = orderLifecycleLiquidationReceipt{
					forcedOrderID: payload.ForcedOrderID,
					positionSide:  payload.PositionSide,
					positionSize:  payload.PositionSize,
					attemptedQty:  payload.AttemptedQty,
					filledQty:     payload.FilledQty,
					remainingQty:  payload.RemainingQty,
				}
			}
			if _, exists := liquidations[liquidationKey]; exists {
				result.MalformedLiquidations++
				return
			}
			liquidations[liquidationKey] = receipt
		}
	}); err != nil {
		return nil, err
	}
	if r.strictLifecycleIdentity {
		type forcedFillGroup struct {
			key     orderLifecycleLiquidationKey
			receipt orderLifecycleLiquidationReceipt
			fills   []orderLifecycleUnknownFill
		}
		groups := make(map[orderLifecycleLiquidationKey]*forcedFillGroup)
		for _, unknown := range unknownFills {
			if unknown.forced == nil || !*unknown.forced || unknown.liquidationID == nil || *unknown.liquidationID == 0 || unknown.symbol == "" || unknown.positionSide == "" || unknown.side == "" {
				result.UnlinkedFills++
				result.LiquidationIdentityFailures++
				addFailure(unknown.key, nil, "fill_without_exact_liquidation_identity")
				continue
			}
			liquidationKey := orderLifecycleLiquidationKey{
				file: unknown.key.file, timestamp: unknown.timestamp, clientID: unknown.clientID,
				symbol: unknown.symbol, positionSide: unknown.positionSide, liquidationID: *unknown.liquidationID,
			}
			receipt, ok := liquidations[liquidationKey]
			if !ok || receipt.forcedOrderID != unknown.orderID {
				result.UnlinkedFills++
				result.LiquidationIdentityFailures++
				addFailure(unknown.key, nil, "fill_without_exact_liquidation_identity")
				continue
			}
			group := groups[liquidationKey]
			if group == nil {
				group = &forcedFillGroup{key: liquidationKey, receipt: receipt}
				groups[liquidationKey] = group
			}
			group.fills = append(group.fills, unknown)
		}
		for _, group := range groups {
			sort.SliceStable(group.fills, func(i, j int) bool { return group.fills[i].ordinal < group.fills[j].ordinal })
			valid := group.receipt.positionSide != "" && group.receipt.positionSize != 0
			expectedSide := ""
			if group.receipt.positionSize > 0 {
				expectedSide = "SELL"
			} else {
				expectedSide = "BUY"
			}
			var cumulative int64
			for _, fill := range group.fills {
				var ok bool
				cumulative, ok = exactAdd(cumulative, fill.quantity)
				if !ok || fill.quantity <= 0 || fill.side != expectedSide || fill.positionSide != group.receipt.positionSide || fill.filledQty != cumulative || fill.remainingQty != group.receipt.attemptedQty-cumulative || fill.isFull != (cumulative == group.receipt.attemptedQty) {
					valid = false
				}
			}
			valid = valid && cumulative == group.receipt.filledQty && group.receipt.remainingQty == group.receipt.attemptedQty-cumulative
			if !valid {
				result.LiquidationIdentityFailures++
				for _, fill := range group.fills {
					result.UnlinkedFills++
					addFailure(fill.key, nil, "liquidation_fill_mismatch")
				}
				continue
			}
			result.LiquidationFills += len(group.fills)
		}
		for key := range liquidations {
			if _, matched := groups[key]; !matched {
				result.MissingForcedFills++
			}
		}
	} else {
		for _, unknown := range unknownFills {
			liquidationKey := orderLifecycleLiquidationKey{
				file: unknown.key.file, timestamp: unknown.timestamp,
				clientID: unknown.clientID, symbol: unknown.symbol,
			}
			if unknown.forced == nil {
				// Legacy records predate the explicit marker and may use the
				// liquidation tuple as their only join key. An explicit false or
				// true marker is not eligible for this compatibility rescue.
				if _, ok := liquidations[liquidationKey]; ok {
					result.LiquidationFills++
					continue
				}
			}
			result.UnlinkedFills++
			addFailure(unknown.key, nil, "fill_without_acceptance")
		}
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
