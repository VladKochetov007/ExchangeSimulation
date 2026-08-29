package analysis

import (
	"fmt"
	"sort"
	"sync"
)

// SettlementAuditOptions selects the contracts to audit.
type SettlementAuditOptions struct {
	Files         []string
	FilesSelected bool
	BasePrecision int64
	// RequireExactReplay selects the strict r5 evidence contract. Historical
	// streams may omit trade inputs; strict runs must reject that fallback.
	RequireExactReplay bool
	// DeliveryFeeResolver independently reconstructs the dated-future delivery
	// fee. It is the extension point for a nonzero venue fee schedule.
	DeliveryFeeResolver func(DeliveryFeeContext) (int64, error)
	// DeliveryFeePolicy names a registered built-in policy. The r5 development
	// cells explicitly pin "zero" because their dated-future lister charges no
	// delivery fee; strict replay rejects an empty policy.
	DeliveryFeePolicy string
	// Symbols, when non-empty, restricts the audit to these contracts.
	Symbols []string
}

// DeliveryFeeContext is the complete contract input supplied to a fee policy.
// A resolver is deliberately outside the analyzer's mechanics so future fee
// schedules can be injected without editing this library.
type DeliveryFeeContext struct {
	Symbol          string
	Size            int64
	SettlementPrice int64
	BasePrecision   int64
}

// SettlementCheck is one contract's settlement, recomputed from the positions
// and the published settlement price rather than read from the payouts.
type SettlementCheck struct {
	VenueID         string `json:"venue_id"`
	Symbol          string `json:"symbol"`
	SettlementPrice int64  `json:"settlement_price"`
	ExpiryNano      int64  `json:"expiry_nano"`
	// Holders and NetSize describe the book at the moment before settlement.
	Holders int   `json:"holders"`
	NetSize int64 `json:"net_size"`
	// ExpectedPayout is what the settlement price and each holder's entry
	// price imply, and PaidOut is what the venue actually credited.
	ExpectedPayout      int64 `json:"expected_payout"`
	ExpectedNetPayout   int64 `json:"expected_net_payout"`
	PaidOut             int64 `json:"paid_out"`
	DeliveryFee         int64 `json:"delivery_fee"`
	LoggedDeliveryFee   int64 `json:"logged_delivery_fee"`
	Residual            int64 `json:"residual"`
	SettlementEvents    int   `json:"settlement_events"`
	EventMismatches     int   `json:"event_mismatches"`
	ExactReplayFailures int   `json:"exact_replay_failures"`
	// PaidAccounts is how many accounts received a settlement credit, which
	// must equal Holders: a holder that is not paid has been robbed, and an
	// account paid without a position has been given money.
	PaidAccounts int `json:"paid_accounts"`
	// TradesAfterExpiry counts fills recorded after the expiry instant, which
	// must be zero.
	TradesAfterExpiry          int `json:"trades_after_expiry"`
	PositionUpdatesAfterExpiry int `json:"position_updates_after_expiry"`
	// Unrepresentable means the signed fixed-point payout cannot be represented
	// as int64. Such a contract is unresolved, never silently converted to a
	// zero payout and scored as a matching settlement.
	Unrepresentable bool `json:"unrepresentable"`
}

// SettlementAudit is the independent check of expiry semantics.
type SettlementAudit struct {
	Checks []SettlementCheck `json:"checks"`
	// Mismatched counts contracts whose recomputed payout disagrees with what
	// was paid, and Unpaid counts contracts where the number of paid accounts
	// differs from the number of holders.
	Mismatched int `json:"mismatched"`
	Unpaid     int `json:"unpaid"`
	// TotalTradesAfterExpiry is the count across all audited contracts.
	TotalTradesAfterExpiry int `json:"total_trades_after_expiry"`
	// ArithmeticFailures counts contracts for which the independent payout
	// reconstruction exceeded the artifact's int64 representation.
	ArithmeticFailures int `json:"arithmetic_failures"`
	// ExplicitUnavailableAnnouncements counts terminal settlement events whose
	// new wire contract explicitly says their numeric settlement price is not
	// available. Such an event is invalid lifecycle evidence and is never
	// reconstructed as a zero-price settlement. Legacy logs omit the field and
	// remain readable for historical provenance.
	ExplicitUnavailableAnnouncements int `json:"explicit_unavailable_announcements"`
	ExactReplayChecks                int `json:"exact_replay_checks"`
	ExactReplayFailures              int `json:"exact_replay_failures"`
	SettlementEventMismatches        int `json:"settlement_event_mismatches"`
	EvidenceFailures                 int `json:"evidence_failures"`
	DescriptorConflicts              int `json:"descriptor_conflicts"`
	SettlementTimingFailures         int `json:"settlement_timing_failures"`
	DeliveryFeeMismatches            int `json:"delivery_fee_mismatches"`
	TotalPositionUpdatesAfterExpiry  int `json:"total_position_updates_after_expiry"`
}

// settlementBalanceDelta accepts only the one ledger movement that the
// expiry contract emits: the holder's quote-asset perpetual-wallet credit
// after delivery fee. Summing arbitrary changes would let an unrelated asset
// or wallet cancel a malformed payout and make a failed settlement look paid.
func settlementBalanceDelta(record balanceChangeRecord) (int64, bool) {
	if len(record.Changes) != 1 {
		return 0, false
	}
	change := record.Changes[0]
	if change.Asset == "" || change.Wallet != "perp" {
		return 0, false
	}
	expected, ok := exactSub(change.NewBalance, change.OldBalance)
	if !ok || expected != change.Delta {
		return 0, false
	}
	return change.Delta, true
}

func resolveDeliveryFee(opts SettlementAuditOptions, context DeliveryFeeContext) (int64, bool) {
	if opts.DeliveryFeeResolver != nil {
		fee, err := opts.DeliveryFeeResolver(context)
		if err != nil || fee < 0 {
			return 0, false
		}
		return fee, true
	}
	if opts.DeliveryFeePolicy == "zero" {
		return 0, true
	}
	return 0, false
}

// MeasureSettlements recomputes every dated-future settlement.
//
// It reads three independent streams: the position updates that say who held
// what at expiry, the reference-data announcement that says what the contract
// settled at, and the balance changes that say who was paid. A venue could
// satisfy any two of those and fail the third.
//
// Two passes are needed rather than one. A holder's position is settled and
// zeroed after expiry, so the position that faced settlement is its last
// update at or before the expiry instant — and the expiry instant is only
// known once the settlement announcement has been read. A single streaming
// pass that keeps the latest update overall silently drops every holder whose
// close-out was logged after expiry, which is all of them.
func (r *Run) MeasureSettlements(opts SettlementAuditOptions) (*SettlementAudit, error) {
	if opts.BasePrecision <= 0 {
		return nil, fmt.Errorf("analysis: settlement base precision must be positive, got %d", opts.BasePrecision)
	}
	if opts.RequireExactReplay && opts.DeliveryFeeResolver == nil && opts.DeliveryFeePolicy != "zero" {
		return nil, fmt.Errorf("analysis: strict settlement audit requires an explicit delivery fee resolver or zero policy")
	}
	type instrumentPayload struct {
		Action                   string `json:"action"`
		Symbol                   string `json:"symbol"`
		InstrumentType           string `json:"instrument_type"`
		QuoteAsset               string `json:"quote_asset"`
		BasePrecision            int64  `json:"base_precision"`
		ExpiryNano               int64  `json:"expiry_nano"`
		SettlementPrice          *int64 `json:"settlement_price"`
		SettlementPriceAvailable *bool  `json:"settlement_price_available"`
		Timestamp                int64  `json:"timestamp"`
	}
	type positionPayload struct {
		Timestamp     int64  `json:"timestamp"`
		ClientID      uint64 `json:"client_id"`
		Symbol        string `json:"symbol"`
		PositionSide  string `json:"position_side"`
		BasePrecision int64  `json:"base_precision"`
		OldSize       int64  `json:"old_size"`
		OldEntryPrice int64  `json:"old_entry_price"`
		NewSize       int64  `json:"new_size"`
		NewEntryPrice int64  `json:"new_entry_price"`
		TradeQty      int64  `json:"trade_qty"`
		TradePrice    int64  `json:"trade_price"`
		TradeSide     string `json:"trade_side"`
		Reason        string `json:"reason"`
	}
	type expirySettlementPayload struct {
		Timestamp       int64  `json:"timestamp"`
		ClientID        uint64 `json:"client_id"`
		Symbol          string `json:"symbol"`
		PositionSide    string `json:"position_side"`
		BasePrecision   int64  `json:"base_precision"`
		Size            int64  `json:"size"`
		EntryPrice      int64  `json:"entry_price"`
		SettlementPrice *int64 `json:"settlement_price"`
		CashFlow        *int64 `json:"cash_flow"`
		DeliveryFee     *int64 `json:"delivery_fee"`
	}
	type fillPayload struct {
		Symbol string `json:"symbol"`
		Qty    int64  `json:"qty"`
	}
	type instrumentDescriptor struct {
		quote         string
		basePrecision int64
		expiry        int64
	}

	wanted := make(map[string]struct{}, len(opts.Symbols))
	for _, symbol := range opts.Symbols {
		wanted[symbol] = struct{}{}
	}
	interesting := func(symbol string) bool {
		if len(wanted) == 0 {
			return true
		}
		_, ok := wanted[symbol]
		return ok
	}

	var mu sync.Mutex
	type settledContract struct {
		price, expiry int64
		quote         string
		basePrecision int64
	}
	settled := make(map[markKey]settledContract)
	listings := make(map[markKey]instrumentDescriptor)
	listingTimes := make(map[markKey]int64)
	futureContracts := make(map[markKey]bool)
	quotes := make(map[markKey]string)
	basePrecisions := make(map[markKey]int64)
	type holding struct {
		size, entry, at int64
		precision       int64
		exact           *exactPositionReplay
		exactFailed     bool
	}
	holdings := make(map[positionKey]*holding)
	expiries := make(map[markKey]int64)
	paid := make(map[markKey]struct {
		amount   int64
		accounts int
	})
	type settlementEvent struct {
		size, entry, price, cash, fee, basePrecision int64
		timestamp, ordinal                           int64
		file                                         string
		count                                        int
	}
	settlementEvents := make(map[positionKey]*settlementEvent)
	type holderPayment struct {
		amount             int64
		accounts           int
		timestamp, ordinal int64
		file               string
	}
	paidByHolder := make(map[positionKey]holderPayment)
	fillTimes := make(map[markKey][]int64)
	explicitUnavailableAnnouncements := 0
	exactReplayChecks := 0
	exactReplayFailures := 0
	settlementEventMismatches := 0
	evidenceFailures := 0
	descriptorConflicts := 0
	settlementTimingFailures := 0
	deliveryFeeMismatches := 0
	latePositionUpdates := make(map[markKey]int)
	precision := opts.BasePrecision

	// First pass learns immutable lifecycle descriptors and every contract's
	// expiry instant, so the second pass can distinguish a pre-settlement
	// position from a post-settlement close-out without trusting a terminal
	// event to define its own precision.
	expiryScan := ScanOptions{
		Events:        []string{"instrument_listed", "instrument_settled"},
		Files:         opts.Files,
		FilesSelected: opts.FilesSelected,
		Workers:       1,
	}
	if err := r.Scan(expiryScan, func(event Event) {
		var payload instrumentPayload
		if err := event.Decode(&payload); err != nil {
			mu.Lock()
			evidenceFailures++
			mu.Unlock()
			return
		}
		if opts.RequireExactReplay && (payload.Timestamp == 0 || payload.Timestamp != event.SimTS) {
			mu.Lock()
			evidenceFailures++
			mu.Unlock()
			return
		}
		if payload.InstrumentType != "FUTURE" || !interesting(payload.Symbol) {
			return
		}
		if payload.Symbol == "" {
			mu.Lock()
			evidenceFailures++
			mu.Unlock()
			return
		}
		contractKey := markKey{event.VenueID, payload.Symbol}
		if event.Name == "instrument_listed" {
			if opts.RequireExactReplay && (payload.Action != "listed" || payload.QuoteAsset == "" || payload.BasePrecision <= 0 || payload.ExpiryNano <= 0) {
				mu.Lock()
				evidenceFailures++
				mu.Unlock()
				return
			}
			if payload.BasePrecision <= 0 && !opts.RequireExactReplay {
				return
			}
			mu.Lock()
			descriptor := instrumentDescriptor{quote: payload.QuoteAsset, basePrecision: payload.BasePrecision, expiry: payload.ExpiryNano}
			futureContracts[contractKey] = true
			if previous, exists := listingTimes[contractKey]; !exists || event.SimTS < previous {
				listingTimes[contractKey] = event.SimTS
			}
			if previous, exists := listings[contractKey]; exists && previous != descriptor {
				descriptorConflicts++
			}
			listings[contractKey] = descriptor
			expiries[contractKey] = descriptor.expiry
			quotes[contractKey] = descriptor.quote
			basePrecisions[contractKey] = descriptor.basePrecision
			mu.Unlock()
			return
		}
		if payload.SettlementPriceAvailable != nil && !*payload.SettlementPriceAvailable {
			return
		}
		if opts.RequireExactReplay && (payload.Action != "settled" || payload.QuoteAsset == "" || payload.BasePrecision <= 0 || payload.ExpiryNano <= 0 || payload.SettlementPrice == nil || payload.ExpiryNano != event.SimTS) {
			mu.Lock()
			evidenceFailures++
			mu.Unlock()
			return
		}
		mu.Lock()
		futureContracts[contractKey] = true
		settlementDescriptor := instrumentDescriptor{quote: payload.QuoteAsset, basePrecision: payload.BasePrecision, expiry: payload.ExpiryNano}
		if listed, exists := listings[contractKey]; !exists {
			if opts.RequireExactReplay {
				descriptorConflicts++
			}
		} else if listed != settlementDescriptor {
			descriptorConflicts++
		}
		if payload.ExpiryNano > 0 {
			expiries[contractKey] = payload.ExpiryNano
		}
		if payload.QuoteAsset != "" {
			quotes[contractKey] = payload.QuoteAsset
		}
		if payload.BasePrecision > 0 {
			basePrecisions[contractKey] = payload.BasePrecision
		}
		mu.Unlock()
	}); err != nil {
		return nil, err
	}

	scan := ScanOptions{
		Events:        []string{"instrument_settled", "position_update", "expiry_settlement", "balance_change", "OrderFill"},
		Files:         opts.Files,
		FilesSelected: opts.FilesSelected,
		Workers:       1,
	}
	if err := r.Scan(scan, func(event Event) {
		switch event.Name {
		case "instrument_settled":
			var payload instrumentPayload
			if err := event.Decode(&payload); err != nil {
				mu.Lock()
				evidenceFailures++
				mu.Unlock()
				return
			}
			if opts.RequireExactReplay && (payload.Timestamp == 0 || payload.Timestamp != event.SimTS) {
				mu.Lock()
				evidenceFailures++
				mu.Unlock()
				return
			}
			if payload.InstrumentType != "FUTURE" || !interesting(payload.Symbol) {
				return
			}
			if payload.Symbol == "" {
				mu.Lock()
				evidenceFailures++
				mu.Unlock()
				return
			}
			if payload.SettlementPriceAvailable != nil && !*payload.SettlementPriceAvailable {
				mu.Lock()
				explicitUnavailableAnnouncements++
				mu.Unlock()
				return
			}
			contractKey := markKey{event.VenueID, payload.Symbol}
			if opts.RequireExactReplay {
				listingAt, listed := listingTimes[contractKey]
				if !listed || event.SimTS < listingAt {
					descriptorConflicts++
					settlementTimingFailures++
				}
			}
			if opts.RequireExactReplay && (payload.Action != "settled" || payload.QuoteAsset == "" || payload.BasePrecision <= 0 || payload.ExpiryNano <= 0 || payload.SettlementPrice == nil || payload.ExpiryNano != event.SimTS) {
				mu.Lock()
				evidenceFailures++
				mu.Unlock()
				return
			}
			mu.Lock()
			settlementPrice := int64(0)
			if payload.SettlementPrice != nil {
				settlementPrice = *payload.SettlementPrice
			}
			if listed, exists := listings[contractKey]; opts.RequireExactReplay && (!exists || listed.quote != payload.QuoteAsset || listed.basePrecision != payload.BasePrecision || listed.expiry != payload.ExpiryNano) {
				descriptorConflicts++
			}
			settled[contractKey] = settledContract{price: settlementPrice, expiry: payload.ExpiryNano, quote: payload.QuoteAsset, basePrecision: payload.BasePrecision}
			expiries[contractKey] = payload.ExpiryNano
			quotes[contractKey] = payload.QuoteAsset
			basePrecisions[contractKey] = payload.BasePrecision
			mu.Unlock()
		case "expiry_settlement":
			var payload expirySettlementPayload
			if err := event.Decode(&payload); err != nil {
				mu.Lock()
				evidenceFailures++
				mu.Unlock()
				return
			}
			if opts.RequireExactReplay && (payload.Timestamp == 0 || payload.Timestamp != event.SimTS) {
				mu.Lock()
				evidenceFailures++
				mu.Unlock()
				return
			}
			if payload.Symbol == "" || !interesting(payload.Symbol) {
				if payload.Symbol == "" {
					mu.Lock()
					evidenceFailures++
					mu.Unlock()
				}
				return
			}
			if payload.ClientID != event.ClientID || event.Symbol != payload.Symbol {
				mu.Lock()
				settlementEventMismatches++
				mu.Unlock()
				return
			}
			if payload.PositionSide != "" && !validPositionSide(payload.PositionSide) {
				mu.Lock()
				settlementEventMismatches++
				mu.Unlock()
				return
			}
			if opts.RequireExactReplay && (payload.PositionSide == "" || payload.BasePrecision <= 0 || payload.SettlementPrice == nil || payload.CashFlow == nil || payload.DeliveryFee == nil) {
				mu.Lock()
				settlementEventMismatches++
				mu.Unlock()
				return
			}
			if opts.RequireExactReplay && payload.BasePrecision != basePrecisions[markKey{event.VenueID, payload.Symbol}] {
				mu.Lock()
				settlementEventMismatches++
				mu.Unlock()
				return
			}
			contractKey := markKey{event.VenueID, payload.Symbol}
			if opts.RequireExactReplay && (expiries[contractKey] <= 0 || event.SimTS != expiries[contractKey]) {
				mu.Lock()
				settlementTimingFailures++
				settlementEventMismatches++
				mu.Unlock()
				return
			}
			if opts.RequireExactReplay {
				listingAt, listed := listingTimes[contractKey]
				if !listed || event.SimTS < listingAt {
					mu.Lock()
					settlementTimingFailures++
					settlementEventMismatches++
					mu.Unlock()
					return
				}
			}
			settlementPrice := int64(0)
			if payload.SettlementPrice != nil {
				settlementPrice = *payload.SettlementPrice
			}
			cashFlow := int64(0)
			if payload.CashFlow != nil {
				cashFlow = *payload.CashFlow
			}
			deliveryFee := int64(0)
			if payload.DeliveryFee != nil {
				deliveryFee = *payload.DeliveryFee
			}
			if deliveryFee < 0 {
				mu.Lock()
				settlementEventMismatches++
				mu.Unlock()
				return
			}
			clientID := payload.ClientID
			key := positionKey{venue: event.VenueID, clientID: clientID, symbol: payload.Symbol, positionSide: payload.PositionSide}
			mu.Lock()
			record := settlementEvents[key]
			if record == nil {
				record = &settlementEvent{timestamp: event.SimTS, ordinal: event.Ordinal, file: event.File}
				settlementEvents[key] = record
			}
			record.size, record.entry, record.price = payload.Size, payload.EntryPrice, settlementPrice
			record.basePrecision = payload.BasePrecision
			if cash, ok := exactAdd(record.cash, cashFlow); ok {
				record.cash = cash
			} else {
				settlementEventMismatches++
			}
			if fee, ok := exactAdd(record.fee, deliveryFee); ok {
				record.fee = fee
			} else {
				settlementEventMismatches++
			}
			record.count++
			mu.Unlock()
		case "position_update":
			var payload positionPayload
			if err := event.Decode(&payload); err != nil {
				mu.Lock()
				evidenceFailures++
				mu.Unlock()
				return
			}
			if payload.Symbol == "" || !interesting(payload.Symbol) {
				if payload.Symbol == "" {
					mu.Lock()
					evidenceFailures++
					mu.Unlock()
				}
				return
			}
			contractKey := markKey{event.VenueID, payload.Symbol}
			if opts.RequireExactReplay && !futureContracts[contractKey] {
				// The settlement audit covers dated futures only. Perpetual and
				// option position updates share the derivative stream but are
				// outside this contract's holder reconstruction.
				return
			}
			if opts.RequireExactReplay && (payload.Timestamp == 0 || payload.Timestamp != event.SimTS) {
				mu.Lock()
				exactReplayFailures++
				evidenceFailures++
				mu.Unlock()
				return
			}
			tradePrecision := precision
			if expectedPrecision := basePrecisions[contractKey]; expectedPrecision > 0 {
				tradePrecision = expectedPrecision
			}
			if payload.Reason == "trade" &&
				(payload.ClientID != event.ClientID || (event.Symbol != "" && payload.Symbol != event.Symbol) ||
					(opts.RequireExactReplay && (event.Symbol == "" || !validPositionSide(payload.PositionSide) || basePrecisions[contractKey] <= 0 || payload.BasePrecision != tradePrecision)) ||
					(!opts.RequireExactReplay && payload.PositionSide != "" && !validPositionSide(payload.PositionSide))) {
				mu.Lock()
				exactReplayFailures++
				if opts.RequireExactReplay {
					evidenceFailures++
				}
				mu.Unlock()
				return
			}
			at := payload.Timestamp
			if at == 0 {
				at = event.SimTS
			}
			mu.Lock()
			// Only updates strictly before the contract's expiry describe the
			// position that faced settlement.
			expiry, known := expiries[contractKey]
			if opts.RequireExactReplay {
				listingAt, listed := listingTimes[contractKey]
				if !listed || at < listingAt {
					evidenceFailures++
					settlementEventMismatches++
					mu.Unlock()
					return
				}
			}
			isLate := known && at > expiry
			if opts.RequireExactReplay {
				isLate = known && at >= expiry
			}
			if isLate {
				latePositionUpdates[markKey{event.VenueID, payload.Symbol}]++
				mu.Unlock()
				return
			}
			key := positionKey{venue: event.VenueID, clientID: payload.ClientID, symbol: payload.Symbol, positionSide: payload.PositionSide}
			state := holdings[key]
			if state == nil {
				state = &holding{}
				holdings[key] = state
			}
			if payload.Reason == "trade" {
				if state.exact == nil {
					state.exact = &exactPositionReplay{}
				}
				if !state.exactFailed {
					exactReplayChecks++
					trade := exactPositionTrade{
						OldSize: payload.OldSize, OldEntryPrice: payload.OldEntryPrice,
						NewSize: payload.NewSize, NewEntryPrice: payload.NewEntryPrice,
						TradeQty: payload.TradeQty, TradePrice: payload.TradePrice,
						TradeSide: payload.TradeSide, PositionSide: payload.PositionSide,
					}
					if state.precision == 0 {
						state.precision = tradePrecision
					}
					if state.precision != tradePrecision {
						state.exactFailed = true
						exactReplayFailures++
					} else if _, err := state.exact.apply(trade, tradePrecision); err != nil {
						state.exactFailed = true
						exactReplayFailures++
					}
				}
			} else if state.exact != nil || opts.RequireExactReplay {
				if state.exact == nil {
					state.exact = &exactPositionReplay{}
				}
				if !state.exactFailed {
					state.exactFailed = true
					exactReplayFailures++
				}
			} else if payload.Reason != "" {
				state.exact = &exactPositionReplay{}
				state.exactFailed = true
				exactReplayFailures++
			}
			if at >= state.at {
				state.size, state.entry, state.at = payload.NewSize, payload.NewEntryPrice, at
			}
			mu.Unlock()
		case "balance_change":
			var record balanceChangeRecord
			if err := event.Decode(&record); err != nil {
				mu.Lock()
				evidenceFailures++
				mu.Unlock()
				return
			}
			if opts.RequireExactReplay && (record.Timestamp == 0 || record.Timestamp != event.SimTS) {
				mu.Lock()
				evidenceFailures++
				mu.Unlock()
				return
			}
			if record.Symbol == "" || record.Reason != "expiry_settlement" || !interesting(record.Symbol) {
				if record.Symbol == "" {
					mu.Lock()
					evidenceFailures++
					mu.Unlock()
				}
				return
			}
			total, valid := settlementBalanceDelta(record)
			mu.Lock()
			key := markKey{event.VenueID, record.Symbol}
			if opts.RequireExactReplay {
				listingAt, listed := listingTimes[key]
				if !listed || event.SimTS < listingAt {
					settlementTimingFailures++
					settlementEventMismatches++
				}
			}
			if opts.RequireExactReplay && (expiries[key] <= 0 || event.SimTS != expiries[key]) {
				settlementTimingFailures++
				settlementEventMismatches++
			}
			entry := paid[key]
			if !valid {
				settlementEventMismatches++
			}
			if (opts.RequireExactReplay && event.Symbol == "") || (event.Symbol != "" && record.Symbol != event.Symbol) {
				settlementEventMismatches++
			}
			if valid && ((opts.RequireExactReplay && quotes[key] == "") ||
				(quotes[key] != "" && record.Changes[0].Asset != quotes[key])) {
				settlementEventMismatches++
			}
			var amountOK bool
			entry.amount, amountOK = exactAdd(entry.amount, total)
			if !amountOK {
				settlementEventMismatches++
			}
			entry.accounts++
			paid[key] = entry
			holderID := record.ClientID
			if record.ClientID != event.ClientID {
				settlementEventMismatches++
			}
			if (opts.RequireExactReplay && (record.PositionSide == "" || !validPositionSide(record.PositionSide))) ||
				(!opts.RequireExactReplay && record.PositionSide != "" && !validPositionSide(record.PositionSide)) {
				settlementEventMismatches++
			}
			holderKey := positionKey{venue: event.VenueID, clientID: holderID, symbol: record.Symbol, positionSide: record.PositionSide}
			holderPayment := paidByHolder[holderKey]
			holderPayment.amount, amountOK = exactAdd(holderPayment.amount, total)
			if !amountOK {
				settlementEventMismatches++
			}
			holderPayment.accounts++
			if holderPayment.accounts == 1 {
				holderPayment.timestamp = event.SimTS
				holderPayment.ordinal = event.Ordinal
				holderPayment.file = event.File
			}
			paidByHolder[holderKey] = holderPayment
			mu.Unlock()
		case "OrderFill":
			var payload fillPayload
			if err := event.Decode(&payload); err != nil {
				mu.Lock()
				evidenceFailures++
				mu.Unlock()
				return
			}
			if payload.Qty <= 0 {
				return
			}
			symbol := event.Symbol
			if symbol == "" {
				symbol = payload.Symbol
			}
			if symbol == "" {
				mu.Lock()
				evidenceFailures++
				mu.Unlock()
				return
			}
			if !interesting(symbol) {
				return
			}
			mu.Lock()
			key := markKey{event.VenueID, symbol}
			if opts.RequireExactReplay {
				listingAt, listed := listingTimes[key]
				if !listed || event.SimTS < listingAt {
					settlementTimingFailures++
					settlementEventMismatches++
				}
			}
			fillTimes[key] = append(fillTimes[key], event.SimTS)
			mu.Unlock()
		}
	}); err != nil {
		return nil, err
	}

	result := &SettlementAudit{ExplicitUnavailableAnnouncements: explicitUnavailableAnnouncements}
	for key, contract := range settled {
		check := SettlementCheck{
			VenueID: key.venue, Symbol: key.symbol,
			SettlementPrice: contract.price, ExpiryNano: contract.expiry,
		}
		contractPrecision := contract.basePrecision
		if contractPrecision <= 0 && opts.RequireExactReplay {
			contractPrecision = precision
			check.EventMismatches++
		} else if contractPrecision <= 0 {
			contractPrecision = precision
		}
		knownHolders := make(map[positionKey]bool)
		for holderKey, state := range holdings {
			if state.exact != nil && !state.exactFailed && state.exact.size != state.size {
				state.exactFailed = true
				exactReplayFailures++
			}
			if holderKey.venue != key.venue || holderKey.symbol != key.symbol || state.size == 0 {
				continue
			}
			knownHolders[holderKey] = true
			check.Holders++
			netSize, ok := exactAdd(check.NetSize, state.size)
			if !ok {
				check.Unrepresentable = true
				continue
			}
			check.NetSize = netSize
			expected := int64(0)
			if state.exact != nil {
				if state.exactFailed {
					check.ExactReplayFailures++
					continue
				}
				expected, ok = state.exact.unrealizedPnL(contract.price, contractPrecision)
			} else {
				change, changeOK := exactSub(contract.price, state.entry)
				if changeOK {
					expected, ok = mulDiv(change, state.size, contractPrecision)
				} else {
					ok = false
				}
			}
			if !ok {
				check.Unrepresentable = true
				continue
			}
			expectedPayout, ok := exactAdd(check.ExpectedPayout, expected)
			if !ok {
				check.Unrepresentable = true
				continue
			}
			check.ExpectedPayout = expectedPayout
			event := settlementEvents[holderKey]
			if event == nil || event.count != 1 || event.size != state.size || event.entry != state.entry || event.price != contract.price || event.cash != expected {
				check.EventMismatches++
			}
			if event != nil && ((opts.RequireExactReplay && event.basePrecision != contract.basePrecision) ||
				(!opts.RequireExactReplay && event.basePrecision > 0 && event.basePrecision != contract.basePrecision)) {
				check.EventMismatches++
			}
			if event != nil {
				check.SettlementEvents += event.count
				check.LoggedDeliveryFee, ok = exactAdd(check.LoggedDeliveryFee, event.fee)
				if !ok {
					check.Unrepresentable = true
				}
			}
			payment := paidByHolder[holderKey]
			if payment.accounts != 1 {
				check.EventMismatches++
			}
			if opts.RequireExactReplay && event != nil && payment.accounts == 1 &&
				(payment.file != event.file || payment.timestamp != event.timestamp || payment.ordinal <= event.ordinal) {
				settlementTimingFailures++
				check.EventMismatches++
			}
			fee, feeOK := int64(0), true
			if opts.RequireExactReplay {
				fee, feeOK = resolveDeliveryFee(opts, DeliveryFeeContext{
					Symbol: holderKey.symbol, Size: state.size,
					SettlementPrice: contract.price, BasePrecision: contractPrecision,
				})
				if !feeOK {
					deliveryFeeMismatches++
					check.EventMismatches++
				}
			} else if event != nil {
				fee = event.fee
			}
			if feeOK {
				if opts.RequireExactReplay && (event == nil || event.fee != fee) {
					deliveryFeeMismatches++
					check.EventMismatches++
				}
				check.DeliveryFee, ok = exactAdd(check.DeliveryFee, fee)
				if !ok {
					check.Unrepresentable = true
				}
				expectedNet, netOK := exactSub(expected, fee)
				if !netOK || payment.amount != expectedNet {
					check.EventMismatches++
				}
			}
		}
		for holderKey, event := range settlementEvents {
			if holderKey.venue == key.venue && holderKey.symbol == key.symbol && event.count > 0 && !knownHolders[holderKey] {
				check.EventMismatches++
				check.SettlementEvents += event.count
			}
		}
		entry := paid[key]
		check.PaidOut = entry.amount
		check.PaidAccounts = entry.accounts
		if check.DeliveryFee != 0 {
			var netOK bool
			check.ExpectedNetPayout, netOK = exactSub(check.ExpectedPayout, check.DeliveryFee)
			if !netOK {
				check.Unrepresentable = true
			}
		} else {
			check.ExpectedNetPayout = check.ExpectedPayout
		}
		if residual, ok := exactSub(check.PaidOut, check.ExpectedNetPayout); ok {
			check.Residual = residual
		} else {
			check.Unrepresentable = true
		}
		for _, at := range fillTimes[key] {
			if at >= contract.expiry {
				check.TradesAfterExpiry++
			}
		}
		check.PositionUpdatesAfterExpiry = latePositionUpdates[key]
		if opts.RequireExactReplay && contract.quote == "" {
			check.EventMismatches++
		}
		if check.NetSize != 0 {
			// A dated future is a zero-net-supply contract. A payout can be
			// arithmetically correct for one holder while the other side of the
			// contract is missing from the evidence; never qualify that book.
			check.EventMismatches++
		}
		if check.PositionUpdatesAfterExpiry > 0 {
			check.EventMismatches += check.PositionUpdatesAfterExpiry
		}
		if check.ExactReplayFailures > 0 {
			// Exact replay failure means the expected payout is unavailable;
			// classify it before the contract result is scored.
			check.EventMismatches++
		}
		settlementEventMismatches += check.EventMismatches
		if check.Unrepresentable {
			result.ArithmeticFailures++
		} else if check.EventMismatches > 0 || check.Residual != 0 {
			result.Mismatched++
		}
		if check.PaidAccounts != check.Holders {
			result.Unpaid++
		}
		result.TotalTradesAfterExpiry += check.TradesAfterExpiry
		result.TotalPositionUpdatesAfterExpiry += check.PositionUpdatesAfterExpiry
		result.Checks = append(result.Checks, check)
	}
	result.ExactReplayChecks = exactReplayChecks
	result.ExactReplayFailures = exactReplayFailures
	result.SettlementEventMismatches = settlementEventMismatches
	result.EvidenceFailures = evidenceFailures
	result.DescriptorConflicts = descriptorConflicts
	result.SettlementTimingFailures = settlementTimingFailures
	result.DeliveryFeeMismatches = deliveryFeeMismatches
	sort.Slice(result.Checks, func(i, j int) bool {
		if result.Checks[i].VenueID != result.Checks[j].VenueID {
			return result.Checks[i].VenueID < result.Checks[j].VenueID
		}
		return result.Checks[i].Symbol < result.Checks[j].Symbol
	})
	return result, nil
}
