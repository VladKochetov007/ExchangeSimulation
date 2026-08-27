package analysis

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
)

// PostOnlyActivityOptions selects the passive order classes and instruments
// whose venue-arrival behavior is being audited. Empty selectors include every
// persisted order event; callers should normally name the experimental maker
// roles to avoid attributing unrelated clients' orders to a treatment.
type PostOnlyActivityOptions struct {
	Roles   []string
	Symbols []string
}

// PostOnlyActivity separates accepted passive orders from explicit
// arrival-time rejections. A post-only order can later be filled by an
// incoming taker; PostOnlyFills measures that ordinary maker outcome and does
// not imply a post-only violation.
type PostOnlyActivity struct {
	Events              int64 `json:"events"`
	Accepted            int64 `json:"accepted"`
	AcceptedPostOnly    int64 `json:"accepted_post_only"`
	AcceptedRegular     int64 `json:"accepted_regular"`
	PostOnlyFills       int64 `json:"post_only_fills"`
	PostOnlyFilledQty   int64 `json:"post_only_filled_qty"`
	RejectedWouldTake   int64 `json:"rejected_would_take"`
	RejectedInvalid     int64 `json:"rejected_invalid"`
	UnmatchedFillOrders int64 `json:"unmatched_fill_orders"`
}

type postOnlyPayload struct {
	OrderID   uint64 `json:"order_id"`
	FilledQty int64  `json:"filled_qty"`
	PostOnly  bool   `json:"post_only"`
	Error     string `json:"error"`
}

type postOnlyOrderKey struct {
	venueID string
	orderID uint64
}

// MakerPassiveRefreshOrdering is an order-level replay of the cancel-before-
// replace contract declared by maker_quote_size_decision evidence.  It is
// deliberately separate from the decision's self-reported boolean: the
// classification is based on the physical order of accepted, rejected, fill,
// and cancellation records in each persisted book log.
type MakerPassiveRefreshOrdering struct {
	Decisions                 int64                      `json:"decisions"`
	DecisionSides             int64                      `json:"decision_sides"`
	InitialOrNoPrior          int64                      `json:"initial_or_no_prior"`
	Checked                   int64                      `json:"checked"`
	Missing                   int64                      `json:"missing"`
	Late                      int64                      `json:"late"`
	Duplicate                 int64                      `json:"duplicate"`
	AcceptedOutcomes          int64                      `json:"accepted_outcomes"`
	RejectedOutcomes          int64                      `json:"rejected_outcomes"`
	CancellationsObserved     int64                      `json:"cancellations_observed"`
	InvalidDecisionRecords    int64                      `json:"invalid_decision_records"`
	DuplicateDecisionSides    int64                      `json:"duplicate_decision_sides"`
	DuplicateOrderIDs         int64                      `json:"duplicate_order_ids"`
	DuplicateCancellations    int64                      `json:"duplicate_cancellations"`
	DuplicateBookFiles        int64                      `json:"duplicate_book_files"`
	FillQuantityMismatches    int64                      `json:"fill_quantity_mismatches"`
	CancelQuantityMismatches  int64                      `json:"cancel_quantity_mismatches"`
	OutOfOrderCancellations   int64                      `json:"out_of_order_cancellations"`
	HorizonCensoredSides      int64                      `json:"horizon_censored_sides"`
	CensoredOutcomeDeliveries int64                      `json:"censored_outcome_deliveries"`
	InvalidOrderRecords       int64                      `json:"invalid_order_records"`
	BookFiles                 int64                      `json:"book_files"`
	MissingBookFiles          int64                      `json:"missing_book_files"`
	LineageRows               int64                      `json:"lineage_rows"`
	LineageDigest             string                     `json:"lineage_digest"`
	Valid                     bool                       `json:"valid"`
	Checks                    []MakerPassiveRefreshCheck `json:"checks,omitempty"`
}

// MakerPassiveRefreshCheck identifies an independently replayed contract
// failure.  PriorOrderIDs are retained for failures so a reviewer can inspect
// the exact resting order that was still live or had an ambiguous lifecycle.
type MakerPassiveRefreshCheck struct {
	VenueID       string   `json:"venue_id"`
	File          string   `json:"file"`
	ClientID      uint64   `json:"client_id"`
	Symbol        string   `json:"symbol"`
	Side          string   `json:"side"`
	RequestID     uint64   `json:"request_id"`
	Failure       string   `json:"failure"`
	PriorOrderIDs []uint64 `json:"prior_order_ids,omitempty"`
}

type makerPassiveRefreshBookKey struct {
	venueID string
	symbol  string
}

type makerPassiveRefreshRequestKey struct {
	clientID  uint64
	requestID uint64
}

type makerPassiveRefreshSideKey struct {
	clientID uint64
	side     string
}

type makerPassiveRefreshExpectedKey struct {
	clientID  uint64
	requestID uint64
	side      string
}

type makerPassiveRefreshExpected struct {
	venueID           string
	symbol            string
	file              string
	clientID          uint64
	requestID         uint64
	side              string
	decision          int64
	count             int
	outcome           string
	hadActive         bool
	priorIDs          []uint64
	terminal          string
	terminalOrdinal   int64
	terminalRequestID uint64
	outcomeOrdinal    int64
	censored          bool
}

type makerPassiveRefreshOrder struct {
	clientID  uint64
	side      string
	qty       int64
	filled    int64
	requestID uint64
}

type makerPassiveRefreshSide struct {
	active          map[uint64]*makerPassiveRefreshOrder
	priorOrderIDs   []uint64
	lastTerminal    string
	terminalOrdinal int64
	// terminalRequestID is the venue request that caused the previous order's
	// terminal transition.  It is distinct from the order's submission request
	// and lets the replay prove cancel-before-replace using canonical request
	// ordering rather than a maker self-report.
	terminalRequestID uint64
}

type makerPassiveRefreshLineage struct {
	VenueID           string   `json:"venue_id"`
	File              string   `json:"file"`
	Symbol            string   `json:"symbol"`
	ClientID          uint64   `json:"client_id"`
	Side              string   `json:"side"`
	RequestID         uint64   `json:"request_id"`
	DecisionTime      int64    `json:"decision_time"`
	Outcome           string   `json:"outcome"`
	OutcomeCount      int      `json:"outcome_count"`
	Classification    string   `json:"classification"`
	PriorOrderIDs     []uint64 `json:"prior_order_ids,omitempty"`
	HadActive         bool     `json:"had_active"`
	OutcomeOrdinal    int64    `json:"outcome_ordinal,omitempty"`
	TerminalOrdinal   int64    `json:"terminal_ordinal,omitempty"`
	TerminalRequestID uint64   `json:"terminal_request_id,omitempty"`
}

// MeasurePostOnlyActivity independently reads accepted orders, fill records,
// and rejection reasons. It never infers a passive outcome from a maker's
// configuration: the persisted accepted-order bit is the evidence.
func (r *Run) MeasurePostOnlyActivity(options PostOnlyActivityOptions) (PostOnlyActivity, error) {
	roles := selectionSet(options.Roles)
	symbols := selectionSet(options.Symbols)
	orders := make(map[postOnlyOrderKey]bool)
	var result PostOnlyActivity
	err := r.Scan(ScanOptions{
		Events:  []string{"OrderAccepted", "OrderFill", "OrderRejected"},
		Workers: 1,
	}, func(event Event) {
		role := r.Role(event.VenueID, event.ClientID)
		// Spot book logger records the book identity in its path rather than
		// redundantly in every event envelope. Do not turn an absent envelope
		// symbol into an unselected event: recover only the explicitly named
		// spot-book identity, never guess for multi-instrument logs.
		symbol := event.Symbol
		if symbol == "" {
			symbol = symbolFromSpotFile(event.File)
		}
		if !selected(role, roles) || !selected(symbol, symbols) {
			return
		}
		var payload postOnlyPayload
		if event.Decode(&payload) != nil {
			return
		}
		result.Events++
		switch event.Name {
		case "OrderAccepted":
			result.Accepted++
			orders[postOnlyOrderKey{event.VenueID, payload.OrderID}] = payload.PostOnly
			if payload.PostOnly {
				result.AcceptedPostOnly++
			} else {
				result.AcceptedRegular++
			}
		case "OrderFill":
			postOnly, found := orders[postOnlyOrderKey{event.VenueID, payload.OrderID}]
			if !found {
				result.UnmatchedFillOrders++
				return
			}
			if postOnly {
				result.PostOnlyFills++
				result.PostOnlyFilledQty += payload.FilledQty
			}
		case "OrderRejected":
			switch payload.Error {
			case "POST_ONLY_WOULD_TAKE":
				result.RejectedWouldTake++
			case "POST_ONLY_INVALID":
				result.RejectedInvalid++
			}
		}
	})
	return result, err
}

// MeasureMakerPassiveRefreshOrdering reconstructs each selected maker's quote
// requests against the canonical order of its persisted spot-book stream.  A
// request is "checked" only when the prior resting quote is represented by a
// cancellation record that appears before the replacement acceptance or
// rejection.  A fill-terminated prior order is classified as no-prior: there
// was no resting order left that required cancellation.  The method never
// trusts the cancel_before_replace field as evidence of ordering.
func (r *Run) MeasureMakerPassiveRefreshOrdering(options MakerQuoteSizeOptions) (*MakerPassiveRefreshOrdering, error) {
	roles := selectionSet(options.Roles)
	if len(roles) == 0 {
		roles = selectionSet([]string{"spot_maker", "cdf_spot_maker", "abc_cdf_spot_maker"})
	}
	result := &MakerPassiveRefreshOrdering{}
	byBook := make(map[makerPassiveRefreshBookKey]map[makerPassiveRefreshExpectedKey]*makerPassiveRefreshExpected)
	if err := r.Scan(ScanOptions{Events: []string{"maker_quote_size_decision"}, Workers: 1}, func(event Event) {
		if !selected(r.Role(event.VenueID, event.ClientID), roles) {
			return
		}
		var decision makerQuoteSizeDecision
		if event.Decode(&decision) != nil || decision.ClientID == 0 || decision.ClientID != event.ClientID ||
			decision.Symbol == "" || decision.BidRequestID == 0 || decision.AskRequestID == 0 ||
			decision.BidRequestID == decision.AskRequestID {
			result.InvalidDecisionRecords++
			return
		}
		censored, validCensor := validMakerQuoteSizeCensor(decision)
		if !validCensor {
			result.InvalidDecisionRecords++
		}
		result.Decisions++
		book := makerPassiveRefreshBookKey{venueID: event.VenueID, symbol: decision.Symbol}
		requests := byBook[book]
		if requests == nil {
			requests = make(map[makerPassiveRefreshExpectedKey]*makerPassiveRefreshExpected)
			byBook[book] = requests
		}
		for _, side := range []struct {
			name string
			id   uint64
		}{
			{name: "BUY", id: decision.BidRequestID},
			{name: "SELL", id: decision.AskRequestID},
		} {
			result.DecisionSides++
			key := makerPassiveRefreshExpectedKey{clientID: decision.ClientID, requestID: side.id, side: side.name}
			if _, duplicate := requests[key]; duplicate {
				result.DuplicateDecisionSides++
				result.Checks = append(result.Checks, MakerPassiveRefreshCheck{
					VenueID: event.VenueID, ClientID: decision.ClientID, Symbol: decision.Symbol,
					Side: side.name, RequestID: side.id, Failure: "duplicate_decision_side",
				})
				continue
			}
			requests[key] = &makerPassiveRefreshExpected{
				venueID: event.VenueID, symbol: decision.Symbol, clientID: decision.ClientID,
				requestID: side.id, side: side.name, decision: event.SimTS, censored: censored,
			}
			if !decision.PostOnly || !decision.CancelBeforeReplace {
				result.InvalidDecisionRecords++
				result.Checks = append(result.Checks, MakerPassiveRefreshCheck{
					VenueID: event.VenueID, ClientID: decision.ClientID, Symbol: decision.Symbol,
					Side: side.name, RequestID: side.id, Failure: "declared_policy_not_post_only_cancel_before_replace",
				})
			}
		}
	}); err != nil {
		return nil, err
	}

	// The maker decisions are emitted in general.jsonl, while order outcomes
	// are emitted in one spot book file per symbol.  Each file is replayed in
	// physical record order; no cross-file timestamp tie is used.
	filesByBook := make(map[makerPassiveRefreshBookKey][]string)
	for _, path := range r.Files() {
		symbol := symbolFromSpotFile(path)
		if symbol == "" {
			continue
		}
		venueID := filepath.Base(filepath.Dir(filepath.Dir(path)))
		book := makerPassiveRefreshBookKey{venueID: venueID, symbol: symbol}
		if _, wanted := byBook[book]; wanted {
			filesByBook[book] = append(filesByBook[book], path)
		}
	}

	lineage := make([]makerPassiveRefreshLineage, 0, result.DecisionSides)
	keep := map[string]bool{
		"OrderAccepted":  true,
		"OrderRejected":  true,
		"OrderFill":      true,
		"OrderCancelled": true,
	}
	needles := [][]byte{[]byte(`"OrderAccepted"`), []byte(`"OrderRejected"`), []byte(`"OrderFill"`), []byte(`"OrderCancelled"`)}

	for book, expected := range byBook {
		files := filesByBook[book]
		if len(files) == 0 {
			result.MissingBookFiles++
		}
		if len(files) > 1 {
			result.DuplicateBookFiles += int64(len(files) - 1)
		}
		// A valid run has exactly one canonical spot-book stream per venue and
		// symbol.  Do not replay duplicate files and accidentally count each
		// outcome twice; retain the duplicate diagnostic and fail closed.
		if len(files) > 1 {
			files = files[:1]
		}
		for _, path := range files {
			result.BookFiles++
			canonicalFile := filepath.ToSlash(filepath.Join("venues", book.venueID, "spot", filepath.Base(path)))
			for _, item := range expected {
				if item.file == "" {
					item.file = canonicalFile
				}
			}
			tracked := make(map[uint64]struct{})
			byRequest := make(map[makerPassiveRefreshRequestKey][]*makerPassiveRefreshExpected)
			for key, item := range expected {
				if key.clientID == item.clientID {
					tracked[item.clientID] = struct{}{}
				}
				requestKey := makerPassiveRefreshRequestKey{clientID: item.clientID, requestID: item.requestID}
				byRequest[requestKey] = append(byRequest[requestKey], item)
			}
			orders := make(map[uint64]*makerPassiveRefreshOrder)
			localStates := make(map[makerPassiveRefreshSideKey]*makerPassiveRefreshSide)
			getLocalSide := func(clientID uint64, side string) *makerPassiveRefreshSide {
				key := makerPassiveRefreshSideKey{clientID: clientID, side: side}
				state := localStates[key]
				if state == nil {
					state = &makerPassiveRefreshSide{active: make(map[uint64]*makerPassiveRefreshOrder)}
					localStates[key] = state
				}
				return state
			}

			consumeOutcome := func(event Event, clientID uint64, side string, requestID uint64, outcome string) {
				candidates := byRequest[makerPassiveRefreshRequestKey{clientID: clientID, requestID: requestID}]
				for _, item := range candidates {
					if item.side != side {
						continue
					}
					if item.count == 0 {
						state := getLocalSide(clientID, side)
						prior := make([]uint64, 0, len(state.active))
						for id := range state.active {
							prior = append(prior, id)
						}
						if len(prior) == 0 {
							prior = append(prior, state.priorOrderIDs...)
						}
						sort.Slice(prior, func(i, j int) bool { return prior[i] < prior[j] })
						item.hadActive = len(state.active) > 0
						item.priorIDs = prior
						item.terminal = state.lastTerminal
						item.terminalOrdinal = state.terminalOrdinal
						item.terminalRequestID = state.terminalRequestID
						item.outcomeOrdinal = event.Ordinal
						item.outcome = outcome
					}
					item.count++
				}
			}

			err := scanFile(path, keep, needles, func(event Event) {
				var accepted struct {
					OrderID  uint64 `json:"order_id"`
					ClientID uint64 `json:"client_id"`
					Symbol   string `json:"symbol"`
					Side     string `json:"side"`
					Type     string `json:"type"`
					TIF      string `json:"time_in_force"`
					Qty      int64  `json:"qty"`
					Request  uint64 `json:"request_id"`
				}
				var rejected struct {
					ClientID uint64 `json:"client_id"`
					Side     string `json:"side"`
					Request  uint64 `json:"request_id"`
				}
				var fill struct {
					OrderID      uint64 `json:"order_id"`
					Qty          int64  `json:"qty"`
					FilledQty    *int64 `json:"filled_qty"`
					RemainingQty *int64 `json:"remaining_qty"`
					IsFull       *bool  `json:"is_full"`
				}
				var cancel struct {
					OrderID      uint64  `json:"order_id"`
					RemainingQty *int64  `json:"remaining_qty"`
					Request      *uint64 `json:"request_id"`
				}
				switch event.Name {
				case "OrderAccepted":
					if event.Decode(&accepted) != nil {
						return
					}
					clientID := accepted.ClientID
					if clientID == 0 {
						clientID = event.ClientID
					}
					if _, ok := tracked[clientID]; !ok || accepted.OrderID == 0 || accepted.Side == "" {
						return
					}
					if (accepted.ClientID != 0 && accepted.ClientID != event.ClientID) ||
						(accepted.Symbol != "" && accepted.Symbol != book.symbol) || accepted.Request == 0 ||
						accepted.Type == "" || accepted.TIF == "" || accepted.Qty <= 0 {
						result.InvalidOrderRecords++
						result.Checks = append(result.Checks, MakerPassiveRefreshCheck{VenueID: book.venueID, File: path, ClientID: clientID, Symbol: book.symbol, Side: accepted.Side, RequestID: accepted.Request, Failure: "invalid_accepted_order_fields"})
						return
					}
					consumeOutcome(event, clientID, accepted.Side, accepted.Request, "accepted")
					if accepted.Type != "LIMIT" || accepted.TIF != "GTC" {
						return
					}
					state := getLocalSide(clientID, accepted.Side)
					if _, duplicate := orders[accepted.OrderID]; duplicate {
						result.DuplicateOrderIDs++
						result.Checks = append(result.Checks, MakerPassiveRefreshCheck{VenueID: book.venueID, File: path, ClientID: clientID, Symbol: book.symbol, Side: accepted.Side, RequestID: accepted.Request, Failure: "duplicate_order_id"})
						return
					}
					order := &makerPassiveRefreshOrder{clientID: clientID, side: accepted.Side, qty: accepted.Qty, requestID: accepted.Request}
					orders[accepted.OrderID] = order
					state.active[accepted.OrderID] = order
					state.priorOrderIDs = []uint64{accepted.OrderID}
					state.lastTerminal = ""
					state.terminalOrdinal = 0
					state.terminalRequestID = 0
				case "OrderRejected":
					if event.Decode(&rejected) != nil {
						return
					}
					clientID := rejected.ClientID
					if clientID == 0 {
						clientID = event.ClientID
					}
					if _, ok := tracked[clientID]; !ok || rejected.Request == 0 || rejected.Side == "" {
						return
					}
					consumeOutcome(event, clientID, rejected.Side, rejected.Request, "rejected")
				case "OrderFill":
					if event.Decode(&fill) != nil {
						return
					}
					order := orders[fill.OrderID]
					if order == nil {
						return
					}
					state := getLocalSide(order.clientID, order.side)
					if _, active := state.active[fill.OrderID]; !active {
						return
					}
					if fill.Qty <= 0 {
						result.FillQuantityMismatches++
						result.Checks = append(result.Checks, MakerPassiveRefreshCheck{VenueID: book.venueID, File: path, ClientID: order.clientID, Symbol: book.symbol, Side: order.side, RequestID: order.requestID, Failure: "invalid_fill_quantity"})
						return
					}
					expectedFilled := order.filled + fill.Qty
					expectedRemaining := order.qty - expectedFilled
					if fill.FilledQty == nil || fill.RemainingQty == nil || fill.IsFull == nil || *fill.FilledQty != expectedFilled || *fill.RemainingQty != expectedRemaining || expectedFilled > order.qty ||
						(*fill.IsFull != (expectedRemaining == 0)) {
						result.FillQuantityMismatches++
						result.Checks = append(result.Checks, MakerPassiveRefreshCheck{VenueID: book.venueID, File: path, ClientID: order.clientID, Symbol: book.symbol, Side: order.side, RequestID: order.requestID, Failure: "fill_quantity_mismatch"})
						return
					}
					order.filled = expectedFilled
					if expectedRemaining == 0 {
						delete(state.active, fill.OrderID)
						state.priorOrderIDs = []uint64{fill.OrderID}
						state.lastTerminal = "fill"
						state.terminalOrdinal = event.Ordinal
						state.terminalRequestID = 0
					}
				case "OrderCancelled":
					if event.Decode(&cancel) != nil {
						return
					}
					order := orders[cancel.OrderID]
					if order == nil {
						return
					}
					state := getLocalSide(order.clientID, order.side)
					if _, active := state.active[cancel.OrderID]; !active {
						result.DuplicateCancellations++
						return
					}
					if cancel.RemainingQty == nil || *cancel.RemainingQty != order.qty-order.filled {
						result.CancelQuantityMismatches++
						result.Checks = append(result.Checks, MakerPassiveRefreshCheck{VenueID: book.venueID, File: path, ClientID: order.clientID, Symbol: book.symbol, Side: order.side, RequestID: order.requestID, Failure: "cancel_quantity_mismatch"})
					}
					delete(state.active, cancel.OrderID)
					state.priorOrderIDs = []uint64{cancel.OrderID}
					state.lastTerminal = "cancel"
					state.terminalOrdinal = event.Ordinal
					if cancel.Request != nil {
						state.terminalRequestID = *cancel.Request
					} else {
						state.terminalRequestID = 0
					}
					result.CancellationsObserved++
				}
			})
			if err != nil {
				return nil, err
			}
		}

		for _, item := range expected {
			classification := "initial_or_no_prior"
			if item.count == 0 && item.censored {
				classification = "horizon_censored"
				result.HorizonCensoredSides++
			} else if item.count == 0 {
				classification = "missing"
				result.Missing++
			} else if item.count > 1 {
				classification = "duplicate"
				result.Duplicate++
			} else if item.censored {
				classification = "censored_outcome_delivered"
				result.CensoredOutcomeDeliveries++
			} else if item.hadActive {
				classification = "late"
				result.Late++
			} else if item.terminal == "cancel" {
				if item.terminalRequestID == 0 || item.terminalRequestID >= item.requestID {
					classification = "cancellation_order_violation"
					result.OutOfOrderCancellations++
				} else {
					classification = "checked"
					result.Checked++
				}
			} else {
				result.InitialOrNoPrior++
			}
			if item.count == 1 {
				if item.outcome == "accepted" {
					result.AcceptedOutcomes++
				} else if item.outcome == "rejected" {
					result.RejectedOutcomes++
				}
			}
			lineage = append(lineage, makerPassiveRefreshLineage{
				VenueID: item.venueID, File: item.file, Symbol: item.symbol,
				ClientID: item.clientID, Side: item.side, RequestID: item.requestID,
				DecisionTime: item.decision, Outcome: item.outcome, OutcomeCount: item.count,
				Classification: classification, PriorOrderIDs: append([]uint64(nil), item.priorIDs...), HadActive: item.hadActive,
				OutcomeOrdinal: item.outcomeOrdinal, TerminalOrdinal: item.terminalOrdinal, TerminalRequestID: item.terminalRequestID,
			})
			if classification == "missing" || classification == "duplicate" || classification == "late" || classification == "cancellation_order_violation" || classification == "censored_outcome_delivered" {
				result.Checks = append(result.Checks, MakerPassiveRefreshCheck{
					VenueID: item.venueID, File: item.file, ClientID: item.clientID, Symbol: item.symbol,
					Side: item.side, RequestID: item.requestID, Failure: classification, PriorOrderIDs: append([]uint64(nil), item.priorIDs...),
				})
			}
		}
	}

	result.LineageRows = int64(len(lineage))
	sort.Slice(lineage, func(i, j int) bool {
		if lineage[i].VenueID != lineage[j].VenueID {
			return lineage[i].VenueID < lineage[j].VenueID
		}
		if lineage[i].File != lineage[j].File {
			return lineage[i].File < lineage[j].File
		}
		if lineage[i].ClientID != lineage[j].ClientID {
			return lineage[i].ClientID < lineage[j].ClientID
		}
		if lineage[i].RequestID != lineage[j].RequestID {
			return lineage[i].RequestID < lineage[j].RequestID
		}
		return lineage[i].Side < lineage[j].Side
	})
	digest := sha256.New()
	for _, row := range lineage {
		raw, _ := json.Marshal(row)
		_, _ = digest.Write(raw)
		_, _ = digest.Write([]byte{'\n'})
	}
	result.LineageDigest = hex.EncodeToString(digest.Sum(nil))
	sort.Slice(result.Checks, func(i, j int) bool {
		left, right := result.Checks[i], result.Checks[j]
		if left.VenueID != right.VenueID {
			return left.VenueID < right.VenueID
		}
		if left.File != right.File {
			return left.File < right.File
		}
		if left.ClientID != right.ClientID {
			return left.ClientID < right.ClientID
		}
		if left.RequestID != right.RequestID {
			return left.RequestID < right.RequestID
		}
		if left.Side != right.Side {
			return left.Side < right.Side
		}
		return left.Failure < right.Failure
	})
	result.Valid = result.Decisions > 0 && result.InvalidDecisionRecords == 0 && result.Missing == 0 && result.Duplicate == 0 && result.Late == 0 && result.OutOfOrderCancellations == 0 && result.CensoredOutcomeDeliveries == 0 && result.MissingBookFiles == 0 && result.DuplicateBookFiles == 0 && result.DuplicateOrderIDs == 0 && result.DuplicateCancellations == 0 && result.FillQuantityMismatches == 0 && result.CancelQuantityMismatches == 0 && result.InvalidOrderRecords == 0
	return result, nil
}

func selectionSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result[value] = struct{}{}
		}
	}
	return result
}

func selected(value string, selection map[string]struct{}) bool {
	if len(selection) == 0 {
		return true
	}
	_, ok := selection[value]
	return ok
}
