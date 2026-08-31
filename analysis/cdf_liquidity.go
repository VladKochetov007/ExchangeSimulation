package analysis

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// CDFLiquidityRunAudit is an evidence audit for the V2-R2-SV1 participant
// class. It combines the evidence-only decision/fill stream with the rendered
// CDF/USD execution book and independent account snapshots. Valid means the
// intervention can be reconstructed without guessing; it is not a survival
// score.
type CDFLiquidityRunAudit struct {
	SupplierCount            int                         `json:"supplier_count"`
	DecisionCount            int64                       `json:"decision_count"`
	FillCount                int64                       `json:"fill_count"`
	SupplierVolumeQty        int64                       `json:"supplier_volume_qty"`
	TotalTradeCount          int64                       `json:"total_trade_count"`
	TotalTradeVolumeQty      int64                       `json:"total_trade_volume_qty"`
	SupplierVolumeShare      float64                     `json:"supplier_volume_share"`
	SnapshotCount            int64                       `json:"snapshot_count"`
	BidAbsentSnapshots       int64                       `json:"bid_absent_snapshots"`
	AskAbsentSnapshots       int64                       `json:"ask_absent_snapshots"`
	BothAbsentSnapshots      int64                       `json:"both_absent_snapshots"`
	BidAbsenceFraction       float64                     `json:"bid_absence_fraction"`
	AskAbsenceFraction       float64                     `json:"ask_absence_fraction"`
	SupplierInitialEquity    int64                       `json:"supplier_initial_equity"`
	SupplierTerminalEquity   int64                       `json:"supplier_terminal_equity"`
	SupplierPnL              int64                       `json:"supplier_pnl"`
	AcceptedQuoteCount       int64                       `json:"accepted_quote_count"`
	CompletedQuoteCount      int64                       `json:"completed_quote_count"`
	CensoredQuoteCount       int64                       `json:"censored_quote_count"`
	MeanQuoteLifetimeNs      float64                     `json:"mean_quote_lifetime_ns"`
	MaxQuoteLifetimeNs       int64                       `json:"max_quote_lifetime_ns"`
	MeanObservedTouchShare   float64                     `json:"mean_observed_touch_share"`
	MaxObservedTouchShare    float64                     `json:"max_observed_touch_share"`
	SubmitCount              int64                       `json:"submit_count"`
	RestCount                int64                       `json:"rest_count"`
	CancelCount              int64                       `json:"cancel_count"`
	WithdrawCount            int64                       `json:"withdraw_count"`
	TradingSupplierCount     int64                       `json:"trading_supplier_count"`
	PnLChangingSupplierCount int64                       `json:"pnl_changing_supplier_count"`
	Suppliers                []CDFLiquiditySupplierAudit `json:"suppliers"`
	Checks                   []CDFLiquidityCheck         `json:"checks,omitempty"`
	Valid                    bool                        `json:"valid"`
}

// CDFLiquiditySupplierAudit is the per-participant diagnostic vector required
// by the preregistration. Account equity is the PnL source; position and
// turnover are reconstructed from local evidence.
type CDFLiquiditySupplierAudit struct {
	VenueID                string  `json:"venue_id"`
	Role                   string  `json:"role"`
	ClientID               uint64  `json:"client_id"`
	DecisionCount          int64   `json:"decision_count"`
	FillCount              int64   `json:"fill_count"`
	FilledQty              int64   `json:"filled_qty"`
	BuyQty                 int64   `json:"buy_qty"`
	SellQty                int64   `json:"sell_qty"`
	InitialEquity          int64   `json:"initial_equity"`
	TerminalEquity         int64   `json:"terminal_equity"`
	PnL                    int64   `json:"pnl"`
	MinPosition            int64   `json:"min_position"`
	MaxPosition            int64   `json:"max_position"`
	TerminalPosition       int64   `json:"terminal_position"`
	InventoryLimit         int64   `json:"inventory_limit"`
	AcceptedQuoteCount     int64   `json:"accepted_quote_count"`
	CompletedQuoteCount    int64   `json:"completed_quote_count"`
	CensoredQuoteCount     int64   `json:"censored_quote_count"`
	WithdrawCount          int64   `json:"withdraw_count"`
	CancelCount            int64   `json:"cancel_count"`
	RestCount              int64   `json:"rest_count"`
	SubmitCount            int64   `json:"submit_count"`
	MeanQuoteLifetimeNs    float64 `json:"mean_quote_lifetime_ns"`
	MaxQuoteLifetimeNs     int64   `json:"max_quote_lifetime_ns"`
	MeanObservedTouchShare float64 `json:"mean_observed_touch_share"`
	MaxObservedTouchShare  float64 `json:"max_observed_touch_share"`
	MeanObservationAgeNs   float64 `json:"mean_observation_age_ns"`
	MaxObservationAgeNs    int64   `json:"max_observation_age_ns"`
	Valid                  bool    `json:"valid"`

	positionSet           bool
	lastPosition          int64
	observationAgeTotal   int64
	observationCount      int64
	touchShareTotal       float64
	touchShareCount       int64
	quoteLifetimeTotal    int64
	quoteLifetimeCount    int64
	pendingTouchByRequest map[uint64]float64
}

// CDFLiquidityCheck identifies one concrete reconstruction failure. Any check
// makes the audit invalid even when aggregate counts look plausible.
type CDFLiquidityCheck struct {
	VenueID  string `json:"venue_id"`
	Role     string `json:"role,omitempty"`
	ClientID uint64 `json:"client_id,omitempty"`
	Ordinal  int64  `json:"ordinal,omitempty"`
	Failure  string `json:"failure"`
}

// CDFLiquidityComparison contains a treatment and its paired no-CDF control.
// The control side-absence fraction is a matched development diagnostic, not a
// causal claim beyond the registered seed and horizon.
type CDFLiquidityComparison struct {
	Treatment                   *CDFLiquidityRunAudit `json:"treatment"`
	Control                     *CDFLiquidityRunAudit `json:"control"`
	ControlBidAbsenceFraction   float64               `json:"control_bid_absence_fraction"`
	ControlAskAbsenceFraction   float64               `json:"control_ask_absence_fraction"`
	TreatmentBidAbsenceFraction float64               `json:"treatment_bid_absence_fraction"`
	TreatmentAskAbsenceFraction float64               `json:"treatment_ask_absence_fraction"`
	Valid                       bool                  `json:"valid"`
}

type cdfDecisionEvidence struct {
	Role             string `json:"role"`
	ClientID         uint64 `json:"client_id"`
	Symbol           string `json:"symbol"`
	DecisionTime     int64  `json:"decision_time"`
	ObservationTime  int64  `json:"observation_time"`
	ObservationAge   int64  `json:"observation_age"`
	BestBid          int64  `json:"best_bid"`
	BestBidQty       int64  `json:"best_bid_qty"`
	BestAsk          int64  `json:"best_ask"`
	BestAskQty       int64  `json:"best_ask_qty"`
	MarkPrice        int64  `json:"mark_price"`
	ReferencePrice   int64  `json:"reference_price"`
	Position         int64  `json:"position"`
	TargetPosition   int64  `json:"target_position"`
	InventoryLimit   int64  `json:"inventory_limit"`
	Action           string `json:"action"`
	Reason           string `json:"reason"`
	Side             string `json:"side"`
	QuotePrice       int64  `json:"quote_price"`
	QuoteQty         int64  `json:"quote_qty"`
	QuoteOrderID     uint64 `json:"quote_order_id"`
	QuoteRequestID   uint64 `json:"quote_request_id"`
	QuoteSubmittedAt int64  `json:"quote_submitted_at"`
}

type cdfFillEvidence struct {
	Role           string `json:"role"`
	ClientID       uint64 `json:"client_id"`
	Symbol         string `json:"symbol"`
	OrderID        uint64 `json:"order_id"`
	TradeID        uint64 `json:"trade_id"`
	Timestamp      int64  `json:"timestamp"`
	Side           string `json:"side"`
	Price          int64  `json:"price"`
	Qty            int64  `json:"qty"`
	FeeAmount      int64  `json:"fee_amount"`
	FeeAsset       string `json:"fee_asset"`
	IsFull         bool   `json:"is_full"`
	PositionBefore int64  `json:"position_before"`
	PositionAfter  int64  `json:"position_after"`
}

type cdfAcceptedEvidence struct {
	OrderID     uint64 `json:"order_id"`
	ClientID    uint64 `json:"client_id"`
	RequestID   uint64 `json:"request_id"`
	Side        string `json:"side"`
	Type        string `json:"type"`
	TimeInForce string `json:"time_in_force"`
	PostOnly    bool   `json:"post_only"`
	Price       int64  `json:"price"`
	Qty         int64  `json:"qty"`
}

type cdfCancelledEvidence struct {
	OrderID      uint64 `json:"order_id"`
	RequestID    uint64 `json:"request_id"`
	RemainingQty int64  `json:"remaining_qty"`
}

type cdfOrderFillEvidence struct {
	OrderID      uint64 `json:"order_id"`
	TradeID      uint64 `json:"trade_id"`
	Side         string `json:"side"`
	Price        int64  `json:"price"`
	Qty          int64  `json:"qty"`
	FilledQty    int64  `json:"filled_qty"`
	RemainingQty int64  `json:"remaining_qty"`
	IsFull       bool   `json:"is_full"`
}

type cdfTradeEvidence struct {
	TradeID      uint64 `json:"trade_id"`
	Price        int64  `json:"price"`
	Qty          int64  `json:"qty"`
	Side         string `json:"side"`
	TakerOrderID uint64 `json:"taker_order_id"`
}

type cdfSnapshotEvidence struct {
	Bids []bookLevel `json:"bids"`
	Asks []bookLevel `json:"asks"`
}

type cdfParticipantKey struct {
	VenueID  string
	ClientID uint64
}

type cdfOrderKey struct {
	VenueID  string
	ClientID uint64
	OrderID  uint64
}

type cdfFillKey struct {
	VenueID  string
	ClientID uint64
	OrderID  uint64
	TradeID  uint64
}

type cdfOrderState struct {
	requestID       uint64
	acceptedAt      int64
	acceptedQty     int64
	filledQty       int64
	remainingQty    int64
	closed          bool
	closedAt        int64
	touchShare      float64
	touchShareKnown bool
}

type cdfObservedFill struct {
	fill    cdfFillEvidence
	ordinal int64
}

// MeasureCDFLiquidity reconstructs one rendered evstream run. The caller
// should first pass the original run through multivenue.RenderBinaryEvidence;
// this package depends only on the resulting public evidence layout and
// greeks.json.
func (r *Run) MeasureCDFLiquidity() (*CDFLiquidityRunAudit, error) {
	if r == nil {
		return nil, fmt.Errorf("cdf liquidity: nil run")
	}
	result := &CDFLiquidityRunAudit{}
	states := make(map[cdfParticipantKey]*CDFLiquiditySupplierAudit)
	initial := make(map[cdfParticipantKey]AccountRow)
	terminal := make(map[cdfParticipantKey]AccountRow)
	for _, row := range r.Report.InitialAccounts {
		if !isCDFSupplierRole(row.Role) {
			continue
		}
		key := cdfParticipantKey{VenueID: row.VenueID, ClientID: row.ClientID}
		if _, exists := initial[key]; exists {
			result.addCheck(CDFLiquidityCheck{VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID, Failure: "duplicate initial supplier account"})
			continue
		}
		initial[key] = row
		states[key] = &CDFLiquiditySupplierAudit{
			VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID,
			MinPosition: math.MaxInt64, MaxPosition: math.MinInt64,
			pendingTouchByRequest: make(map[uint64]float64), Valid: true,
		}
	}
	for _, row := range r.Report.TerminalAccounts {
		if !isCDFSupplierRole(row.Role) {
			continue
		}
		key := cdfParticipantKey{VenueID: row.VenueID, ClientID: row.ClientID}
		if _, exists := terminal[key]; exists {
			result.addCheck(CDFLiquidityCheck{VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID, Failure: "duplicate terminal supplier account"})
			continue
		}
		terminal[key] = row
	}
	result.SupplierCount = len(states)
	for key, row := range initial {
		terminalRow, exists := terminal[key]
		if !exists {
			result.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: row.Role, ClientID: key.ClientID, Failure: "supplier missing from terminal accounts"})
			continue
		}
		if terminalRow.Role != row.Role {
			result.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: row.Role, ClientID: key.ClientID, Failure: "supplier role changed between account snapshots"})
			continue
		}
		state := states[key]
		state.InitialEquity = row.Account.Equity
		state.TerminalEquity = terminalRow.Account.Equity
		state.PnL = terminalRow.Account.Equity - row.Account.Equity
		var ok bool
		result.SupplierInitialEquity, ok = exactAdd(result.SupplierInitialEquity, state.InitialEquity)
		if !ok {
			result.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: row.Role, ClientID: key.ClientID, Failure: "supplier initial equity overflow"})
		}
		result.SupplierTerminalEquity, ok = exactAdd(result.SupplierTerminalEquity, state.TerminalEquity)
		if !ok {
			result.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: row.Role, ClientID: key.ClientID, Failure: "supplier terminal equity overflow"})
		}
	}
	for key, row := range terminal {
		if _, exists := initial[key]; !exists {
			result.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: row.Role, ClientID: key.ClientID, Failure: "terminal supplier absent from initial accounts"})
		}
	}
	result.SupplierPnL = result.SupplierTerminalEquity - result.SupplierInitialEquity

	generalFiles, bookFiles := make([]string, 0), make([]string, 0)
	for _, path := range r.Files() {
		if filepath.Base(path) == "general.jsonl" {
			generalFiles = append(generalFiles, path)
		}
		if filepath.Base(path) == "CDF-USD.jsonl" {
			bookFiles = append(bookFiles, path)
		}
	}
	orders := make(map[cdfOrderKey]*cdfOrderState)
	actualFills := make(map[cdfFillKey]cdfOrderFillEvidence)
	observedFills := make(map[cdfFillKey]cdfObservedFill)
	if len(generalFiles) > 0 {
		err := r.Scan(ScanOptions{
			Events: []string{"elastic_liquidity_supplier_decision", "elastic_liquidity_supplier_fill"},
			Files:  generalFiles, FilesSelected: true, Workers: 1,
		}, func(event Event) {
			switch event.Name {
			case "elastic_liquidity_supplier_decision":
				result.processDecision(event, states)
			case "elastic_liquidity_supplier_fill":
				result.processSupplierFill(event, states, observedFills)
			}
		})
		if err != nil {
			return nil, fmt.Errorf("cdf liquidity: scan supplier evidence: %w", err)
		}
	}
	for _, path := range bookFiles {
		err := r.Scan(ScanOptions{
			Events: []string{"Trade", "BookSnapshot", "OrderAccepted", "OrderCancelled", "OrderFill"},
			Files:  []string{path}, FilesSelected: true, Workers: 1,
		}, func(event Event) {
			result.processBookEvent(event, states, orders, actualFills)
		})
		if err != nil {
			return nil, fmt.Errorf("cdf liquidity: scan CDF/USD book %s: %w", path, err)
		}
	}
	if len(bookFiles) == 0 {
		result.addCheck(CDFLiquidityCheck{Failure: "no rendered CDF-USD book evidence"})
	}
	result.reconcileFills(observedFills, actualFills, states)
	result.finalizeOrders(orders, states)
	result.finalizeSuppliers(states)
	if result.TotalTradeVolumeQty > 0 {
		result.SupplierVolumeShare = float64(result.SupplierVolumeQty) / float64(result.TotalTradeVolumeQty)
	}
	if result.SnapshotCount > 0 {
		result.BidAbsenceFraction = float64(result.BidAbsentSnapshots) / float64(result.SnapshotCount)
		result.AskAbsenceFraction = float64(result.AskAbsentSnapshots) / float64(result.SnapshotCount)
	}
	result.Valid = len(result.Checks) == 0
	return result, nil
}

// CompareCDFLiquidityRuns applies the paired-control contract without
// interpreting the market-survival outcome. Treatment must contain the
// separate CDF roster, while control must contain none.
func CompareCDFLiquidityRuns(treatment, control *Run) (*CDFLiquidityComparison, error) {
	treatmentAudit, err := treatment.MeasureCDFLiquidity()
	if err != nil {
		return nil, err
	}
	controlAudit, err := control.MeasureCDFLiquidity()
	if err != nil {
		return nil, err
	}
	comparison := &CDFLiquidityComparison{
		Treatment: treatmentAudit, Control: controlAudit,
		ControlBidAbsenceFraction:   controlAudit.BidAbsenceFraction,
		ControlAskAbsenceFraction:   controlAudit.AskAbsenceFraction,
		TreatmentBidAbsenceFraction: treatmentAudit.BidAbsenceFraction,
		TreatmentAskAbsenceFraction: treatmentAudit.AskAbsenceFraction,
	}
	comparison.Valid = treatmentAudit.Valid && controlAudit.Valid && treatmentAudit.SupplierCount > 0 && controlAudit.SupplierCount == 0
	return comparison, nil
}

func (r *CDFLiquidityRunAudit) addCheck(check CDFLiquidityCheck) {
	r.Checks = append(r.Checks, check)
}

func (r *CDFLiquidityRunAudit) processDecision(event Event, states map[cdfParticipantKey]*CDFLiquiditySupplierAudit) {
	var decision cdfDecisionEvidence
	required := []string{"role", "client_id", "symbol", "decision_time", "observation_time", "observation_age", "best_bid", "best_bid_qty", "best_ask", "best_ask_qty", "mark_price", "reference_price", "position", "target_position", "inventory_limit", "action", "reason"}
	if err := decodeRequiredJSON(event.Raw(), &decision, required...); err != nil {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "malformed supplier decision: " + err.Error()})
		return
	}
	key := cdfParticipantKey{VenueID: event.VenueID, ClientID: decision.ClientID}
	state := states[key]
	if state == nil {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "decision from unregistered supplier"})
		return
	}
	r.DecisionCount++
	state.DecisionCount++
	if event.ClientID != decision.ClientID || decision.Role != state.Role || decision.Symbol != "CDF/USD" || decision.DecisionTime != event.SimTS {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "decision identity or timestamp mismatch"})
	}
	if decision.ObservationAge < 0 || decision.ObservationTime > decision.DecisionTime || decision.ReferencePrice <= 0 || decision.InventoryLimit <= 0 {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "invalid decision bounds or observation time"})
	}
	if decision.Position < -decision.InventoryLimit || decision.Position > decision.InventoryLimit || decision.TargetPosition < -decision.InventoryLimit || decision.TargetPosition > decision.InventoryLimit {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "decision exceeds inventory limit"})
	}
	if state.positionSet && state.lastPosition != decision.Position {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "decision position does not follow prior fill"})
	}
	state.positionSet, state.lastPosition = true, decision.Position
	if state.InventoryLimit != 0 && state.InventoryLimit != decision.InventoryLimit {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "inventory limit changed during run"})
	}
	state.InventoryLimit = decision.InventoryLimit
	if decision.ObservationAge > state.MaxObservationAgeNs {
		state.MaxObservationAgeNs = decision.ObservationAge
	}
	state.observationAgeTotal, _ = exactAdd(state.observationAgeTotal, decision.ObservationAge)
	state.observationCount++
	switch decision.Action {
	case "submit":
		state.SubmitCount++
		r.SubmitCount++
		if decision.QuoteRequestID == 0 || decision.QuoteOrderID != 0 || !validSide(decision.Side) || decision.QuotePrice <= 0 || decision.QuoteQty <= 0 {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "submit decision has incomplete quote identity"})
			break
		}
		if share, ok := decisionTouchShare(decision); ok {
			state.pendingTouchByRequest[decision.QuoteRequestID] = share
			state.touchShareTotal += share
			state.touchShareCount++
			if share > state.MaxObservedTouchShare {
				state.MaxObservedTouchShare = share
			}
		} else {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "submitted quote has no positive observed touch depth"})
		}
	case "rest":
		state.RestCount++
		r.RestCount++
		if decision.QuoteOrderID == 0 || !validSide(decision.Side) || decision.QuotePrice <= 0 || decision.QuoteQty <= 0 {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "rest decision has incomplete quote identity"})
		}
		if share, ok := decisionTouchShare(decision); ok {
			state.touchShareTotal += share
			state.touchShareCount++
			if share > state.MaxObservedTouchShare {
				state.MaxObservedTouchShare = share
			}
		}
	case "cancel", "withdraw":
		if decision.Action == "cancel" {
			state.CancelCount++
			r.CancelCount++
		} else {
			state.WithdrawCount++
			r.WithdrawCount++
		}
		if decision.QuoteOrderID == 0 {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "withdrawal decision has no order identity"})
		}
	case "wait":
	default:
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "unknown supplier action " + decision.Action})
	}
}

func (r *CDFLiquidityRunAudit) processSupplierFill(event Event, states map[cdfParticipantKey]*CDFLiquiditySupplierAudit, observed map[cdfFillKey]cdfObservedFill) {
	var fill cdfFillEvidence
	required := []string{"role", "client_id", "symbol", "order_id", "trade_id", "timestamp", "side", "price", "qty", "fee_amount", "fee_asset", "is_full", "position_before", "position_after"}
	if err := decodeRequiredJSON(event.Raw(), &fill, required...); err != nil {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "malformed supplier fill: " + err.Error()})
		return
	}
	key := cdfParticipantKey{VenueID: event.VenueID, ClientID: fill.ClientID}
	state := states[key]
	if state == nil {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: fill.Role, ClientID: fill.ClientID, Ordinal: event.Ordinal, Failure: "fill from unregistered supplier"})
		return
	}
	r.FillCount++
	state.FillCount++
	if event.ClientID != fill.ClientID || fill.Role != state.Role || fill.Symbol != "CDF/USD" || fill.Timestamp != event.SimTS || fill.OrderID == 0 || fill.Qty <= 0 || !validSide(fill.Side) {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: fill.Role, ClientID: fill.ClientID, Ordinal: event.Ordinal, Failure: "supplier fill identity or bounds mismatch"})
	}
	if !state.positionSet {
		state.positionSet, state.lastPosition = true, fill.PositionBefore
	}
	if state.lastPosition != fill.PositionBefore {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: fill.Role, ClientID: fill.ClientID, Ordinal: event.Ordinal, Failure: "supplier fill position-before mismatch"})
	}
	expectedAfter := fill.PositionBefore
	if fill.Side == "BUY" {
		expectedAfter, _ = exactAdd(expectedAfter, fill.Qty)
		state.BuyQty, _ = exactAdd(state.BuyQty, fill.Qty)
	} else {
		expectedAfter, _ = exactAdd(expectedAfter, -fill.Qty)
		state.SellQty, _ = exactAdd(state.SellQty, fill.Qty)
	}
	if expectedAfter != fill.PositionAfter || (state.InventoryLimit > 0 && (fill.PositionAfter < -state.InventoryLimit || fill.PositionAfter > state.InventoryLimit)) {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: fill.Role, ClientID: fill.ClientID, Ordinal: event.Ordinal, Failure: "supplier fill position-after violates inventory transition"})
	}
	state.lastPosition = fill.PositionAfter
	if fill.PositionAfter < state.MinPosition {
		state.MinPosition = fill.PositionAfter
	}
	if fill.PositionAfter > state.MaxPosition {
		state.MaxPosition = fill.PositionAfter
	}
	state.FilledQty, _ = exactAdd(state.FilledQty, fill.Qty)
	fillKey := cdfFillKey{VenueID: event.VenueID, ClientID: fill.ClientID, OrderID: fill.OrderID, TradeID: fill.TradeID}
	if _, exists := observed[fillKey]; exists {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: fill.Role, ClientID: fill.ClientID, Ordinal: event.Ordinal, Failure: "duplicate supplier fill evidence"})
	} else {
		observed[fillKey] = cdfObservedFill{fill: fill, ordinal: event.Ordinal}
	}
}

func (r *CDFLiquidityRunAudit) processBookEvent(event Event, states map[cdfParticipantKey]*CDFLiquiditySupplierAudit, orders map[cdfOrderKey]*cdfOrderState, actual map[cdfFillKey]cdfOrderFillEvidence) {
	switch event.Name {
	case "Trade":
		var trade cdfTradeEvidence
		if err := decodeRequiredJSON(event.Raw(), &trade, "trade_id", "price", "qty", "side", "taker_order_id"); err != nil || trade.Qty <= 0 || trade.Price <= 0 || !validSide(trade.Side) || trade.TakerOrderID == 0 {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "malformed CDF trade evidence"})
			return
		}
		r.TotalTradeCount++
		var ok bool
		r.TotalTradeVolumeQty, ok = exactAdd(r.TotalTradeVolumeQty, trade.Qty)
		if !ok {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "CDF trade volume overflow"})
		}
	case "BookSnapshot":
		if event.ClientID != 0 {
			return
		}
		var snapshot cdfSnapshotEvidence
		if err := decodeRequiredJSON(event.Raw(), &snapshot, "bids", "asks"); err != nil {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "malformed CDF book snapshot: " + err.Error()})
			return
		}
		r.SnapshotCount++
		if len(snapshot.Bids) == 0 {
			r.BidAbsentSnapshots++
		}
		if len(snapshot.Asks) == 0 {
			r.AskAbsentSnapshots++
		}
		if len(snapshot.Bids) == 0 && len(snapshot.Asks) == 0 {
			r.BothAbsentSnapshots++
		}
	case "OrderAccepted":
		key := cdfParticipantKey{VenueID: event.VenueID, ClientID: event.ClientID}
		state := states[key]
		if state == nil {
			return
		}
		var accepted cdfAcceptedEvidence
		if err := decodeRequiredJSON(event.Raw(), &accepted, "order_id", "client_id", "request_id", "side", "type", "time_in_force", "post_only", "price", "qty"); err != nil {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "malformed supplier order acceptance: " + err.Error()})
			return
		}
		if accepted.ClientID != event.ClientID || accepted.OrderID == 0 || accepted.Qty <= 0 || !validSide(accepted.Side) || accepted.Type != "LIMIT" || accepted.TimeInForce != "GTC" || !accepted.PostOnly {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier order acceptance violates passive bounded quote contract"})
			return
		}
		orderKey := cdfOrderKey{VenueID: event.VenueID, ClientID: event.ClientID, OrderID: accepted.OrderID}
		if _, exists := orders[orderKey]; exists {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "duplicate supplier order acceptance"})
			return
		}
		order := &cdfOrderState{requestID: accepted.RequestID, acceptedAt: event.SimTS, acceptedQty: accepted.Qty, remainingQty: accepted.Qty}
		if share, ok := state.pendingTouchByRequest[accepted.RequestID]; ok {
			order.touchShare, order.touchShareKnown = share, true
			delete(state.pendingTouchByRequest, accepted.RequestID)
		}
		orders[orderKey] = order
	case "OrderFill":
		key := cdfParticipantKey{VenueID: event.VenueID, ClientID: event.ClientID}
		state := states[key]
		if state == nil {
			return
		}
		var fill cdfOrderFillEvidence
		if err := decodeRequiredJSON(event.Raw(), &fill, "order_id", "trade_id", "side", "price", "qty", "filled_qty", "remaining_qty", "is_full"); err != nil {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "malformed supplier order fill: " + err.Error()})
			return
		}
		orderKey := cdfOrderKey{VenueID: event.VenueID, ClientID: event.ClientID, OrderID: fill.OrderID}
		order := orders[orderKey]
		if order == nil {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier fill has no accepted order"})
			return
		}
		if order.closed || fill.Qty <= 0 || fill.FilledQty <= 0 || fill.RemainingQty < 0 || (fill.IsFull != (fill.RemainingQty == 0)) {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "invalid supplier order fill lifecycle"})
			return
		}
		expectedFilledTotal, ok := exactAdd(order.filledQty, fill.Qty)
		if !ok || expectedFilledTotal != fill.FilledQty || fill.FilledQty+fill.RemainingQty != order.acceptedQty {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier fill quantity does not reconcile to accepted order"})
		}
		order.filledQty, order.remainingQty = fill.FilledQty, fill.RemainingQty
		fillKey := cdfFillKey{VenueID: event.VenueID, ClientID: event.ClientID, OrderID: fill.OrderID, TradeID: fill.TradeID}
		if _, exists := actual[fillKey]; exists {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "duplicate supplier order fill"})
		} else {
			actual[fillKey] = fill
		}
		if fill.IsFull {
			order.closed, order.closedAt = true, event.SimTS
		}
	case "OrderCancelled":
		state := states[cdfParticipantKey{VenueID: event.VenueID, ClientID: event.ClientID}]
		if state == nil {
			return
		}
		var cancelled cdfCancelledEvidence
		if err := decodeRequiredJSON(event.Raw(), &cancelled, "order_id", "request_id", "remaining_qty"); err != nil {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "malformed supplier cancellation: " + err.Error()})
			return
		}
		key := cdfOrderKey{VenueID: event.VenueID, ClientID: event.ClientID, OrderID: cancelled.OrderID}
		order := orders[key]
		if order == nil {
			return
		}
		if order.closed || cancelled.RemainingQty != order.remainingQty {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier cancellation does not reconcile order state"})
			return
		}
		order.closed, order.closedAt = true, event.SimTS
	}
}

func (r *CDFLiquidityRunAudit) reconcileFills(observed map[cdfFillKey]cdfObservedFill, actual map[cdfFillKey]cdfOrderFillEvidence, states map[cdfParticipantKey]*CDFLiquiditySupplierAudit) {
	for key, observedFill := range observed {
		actualFill, exists := actual[key]
		if !exists {
			r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, ClientID: key.ClientID, Ordinal: observedFill.ordinal, Failure: "supplier fill evidence has no matching order fill"})
			continue
		}
		fill := observedFill.fill
		if actualFill.Side != fill.Side || actualFill.Price != fill.Price || actualFill.Qty != fill.Qty || actualFill.IsFull != fill.IsFull {
			r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: fill.Role, ClientID: key.ClientID, Ordinal: observedFill.ordinal, Failure: "supplier fill evidence disagrees with order fill"})
		}
	}
	for key := range actual {
		if _, exists := observed[key]; !exists {
			if state := states[cdfParticipantKey{VenueID: key.VenueID, ClientID: key.ClientID}]; state != nil {
				r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: state.Role, ClientID: key.ClientID, Failure: "supplier order fill lacks local fill evidence"})
			}
		}
	}
}

func (r *CDFLiquidityRunAudit) finalizeOrders(orders map[cdfOrderKey]*cdfOrderState, states map[cdfParticipantKey]*CDFLiquiditySupplierAudit) {
	keys := make([]cdfOrderKey, 0, len(orders))
	for key := range orders {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].VenueID != keys[j].VenueID {
			return keys[i].VenueID < keys[j].VenueID
		}
		if keys[i].ClientID != keys[j].ClientID {
			return keys[i].ClientID < keys[j].ClientID
		}
		return keys[i].OrderID < keys[j].OrderID
	})
	var lifetimeTotal int64
	var lifetimeCount int64
	for _, key := range keys {
		order := orders[key]
		state := states[cdfParticipantKey{VenueID: key.VenueID, ClientID: key.ClientID}]
		if state == nil {
			continue
		}
		state.AcceptedQuoteCount++
		r.AcceptedQuoteCount++
		if !order.closed {
			state.CensoredQuoteCount++
			r.CensoredQuoteCount++
			continue
		}
		state.CompletedQuoteCount++
		r.CompletedQuoteCount++
		lifetime := order.closedAt - order.acceptedAt
		if lifetime < 0 {
			r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: state.Role, ClientID: key.ClientID, Failure: "negative quote lifetime"})
			continue
		}
		state.quoteLifetimeTotal, _ = exactAdd(state.quoteLifetimeTotal, lifetime)
		state.quoteLifetimeCount++
		if lifetime > state.MaxQuoteLifetimeNs {
			state.MaxQuoteLifetimeNs = lifetime
		}
		if lifetime > r.MaxQuoteLifetimeNs {
			r.MaxQuoteLifetimeNs = lifetime
		}
		lifetimeTotal, _ = exactAdd(lifetimeTotal, lifetime)
		lifetimeCount++
	}
	if lifetimeCount > 0 {
		r.MeanQuoteLifetimeNs = float64(lifetimeTotal) / float64(lifetimeCount)
	}
}

func (r *CDFLiquidityRunAudit) finalizeSuppliers(states map[cdfParticipantKey]*CDFLiquiditySupplierAudit) {
	keys := make([]cdfParticipantKey, 0, len(states))
	for key := range states {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].VenueID != keys[j].VenueID {
			return keys[i].VenueID < keys[j].VenueID
		}
		return keys[i].ClientID < keys[j].ClientID
	})
	var touchTotal float64
	var touchCount int64
	for _, key := range keys {
		state := states[key]
		if state.MinPosition == math.MaxInt64 {
			state.MinPosition = state.lastPosition
		}
		if state.MaxPosition == math.MinInt64 {
			state.MaxPosition = state.lastPosition
		}
		state.TerminalPosition = state.lastPosition
		if state.observationCount > 0 {
			state.MeanObservationAgeNs = float64(state.observationAgeTotal) / float64(state.observationCount)
		}
		if state.touchShareCount > 0 {
			state.MeanObservedTouchShare = state.touchShareTotal / float64(state.touchShareCount)
			touchTotal += state.touchShareTotal
			touchCount += state.touchShareCount
		}
		if state.quoteLifetimeCount > 0 {
			state.MeanQuoteLifetimeNs = float64(state.quoteLifetimeTotal) / float64(state.quoteLifetimeCount)
		}
		if state.FillCount > 0 {
			r.TradingSupplierCount++
		}
		if state.PnL != 0 {
			r.PnLChangingSupplierCount++
		}
		state.Valid = state.DecisionCount > 0 && state.FillCount > 0 && state.AcceptedQuoteCount > 0 && state.CompletedQuoteCount > 0
		if state.DecisionCount == 0 {
			r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: state.Role, ClientID: key.ClientID, Failure: "supplier has no decision evidence"})
		}
		if state.InventoryLimit <= 0 {
			r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: state.Role, ClientID: key.ClientID, Failure: "supplier has no positive inventory limit"})
		}
		r.SupplierVolumeQty, _ = exactAdd(r.SupplierVolumeQty, state.FilledQty)
		if state.MaxObservedTouchShare > r.MaxObservedTouchShare {
			r.MaxObservedTouchShare = state.MaxObservedTouchShare
		}
		r.Suppliers = append(r.Suppliers, *state)
	}
	if touchCount > 0 {
		r.MeanObservedTouchShare = touchTotal / float64(touchCount)
	}
}

func decisionTouchShare(decision cdfDecisionEvidence) (float64, bool) {
	if decision.QuoteQty <= 0 || !validSide(decision.Side) {
		return 0, false
	}
	depth := decision.BestBidQty
	if decision.Side == "SELL" {
		depth = decision.BestAskQty
	}
	if depth <= 0 {
		return 0, false
	}
	return float64(decision.QuoteQty) / float64(depth), true
}

func validSide(side string) bool { return side == "BUY" || side == "SELL" }

func isCDFSupplierRole(role string) bool {
	const prefix = "cdf_elastic_supplier_"
	if !strings.HasPrefix(role, prefix) {
		return false
	}
	_, err := strconv.ParseUint(strings.TrimPrefix(role, prefix), 10, 32)
	return err == nil
}
