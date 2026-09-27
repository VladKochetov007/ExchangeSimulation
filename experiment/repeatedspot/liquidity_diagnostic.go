package repeatedspot

import (
	"errors"
	"fmt"
	"io"
	"sort"

	worldspot "exchange_sim/simulations/repeatedspot"
)

type LiquidityDecisionPoint struct {
	DecisionAtNanos       int64  `json:"decision_at_ns"`
	LatestBookSourceNanos int64  `json:"latest_book_source_ns"`
	BestBid               int64  `json:"best_bid"`
	BestAsk               int64  `json:"best_ask"`
	Action                string `json:"action"`
}

type MakerLiquidityActions struct {
	ActorID                 uint64                  `json:"actor_id"`
	BeforeMeasurement       map[string]int64        `json:"before_measurement"`
	DuringMeasurement       map[string]int64        `json:"during_measurement"`
	FirstNoUsableQuote      *LiquidityDecisionPoint `json:"first_no_usable_quote,omitempty"`
	FirstUnusableCancel     *LiquidityDecisionPoint `json:"first_unusable_cancel,omitempty"`
	LastPlacementEvaluation *LiquidityDecisionPoint `json:"last_placement_evaluation,omitempty"`
}

type BookStateDurations struct {
	HorizonNanos  int64 `json:"horizon_ns"`
	TwoSidedNanos int64 `json:"two_sided_ns"`
	BidOnlyNanos  int64 `json:"bid_only_ns"`
	AskOnlyNanos  int64 `json:"ask_only_ns"`
	EmptyNanos    int64 `json:"empty_ns"`
}

type E0LiquidityDiagnostic struct {
	Evidence                 EvidenceIdentity        `json:"evidence"`
	MeasurementStartNanos    int64                   `json:"measurement_start_ns"`
	WorldEndNanos            int64                   `json:"world_end_ns"`
	FirstPermanentEmptyNanos *int64                  `json:"first_permanent_empty_ns,omitempty"`
	LastTwoSidedEndNanos     *int64                  `json:"last_two_sided_end_ns,omitempty"`
	FirstTradeNanos          *int64                  `json:"first_trade_ns,omitempty"`
	LastTradeNanos           *int64                  `json:"last_trade_ns,omitempty"`
	TradeCount               int64                   `json:"trade_count"`
	BookDurations            BookStateDurations      `json:"book_durations"`
	Makers                   []MakerLiquidityActions `json:"makers"`
}

// DiagnoseE0LiquidityEvidence reconstructs exchange book-state durations and
// maker decisions from retained evidence. It does not substitute for strict
// economic replay or infer why a maker selected a quote rule.
func DiagnoseE0LiquidityEvidence(input io.Reader, identity EvidenceIdentity, measurementStartNanos, tickSize int64) (E0LiquidityDiagnostic, error) {
	if measurementStartNanos < 0 || tickSize <= 0 {
		return E0LiquidityDiagnostic{}, errors.New("repeated spot: invalid liquidity diagnostic contract")
	}
	result := E0LiquidityDiagnostic{Evidence: identity, MeasurementStartNanos: measurementStartNanos}
	makers := make(map[uint64]*MakerLiquidityActions)
	var series *publicBookSeries
	worldEnded := false
	visit := func(event Event) error {
		switch {
		case event.Source == "control" && event.Name == "world_begin":
			if series != nil {
				return errors.New("repeated spot: duplicate world begin in diagnostic")
			}
			series = newPublicBookSeries(event.Timestamp, tickSize)
			result.FirstPermanentEmptyNanos = int64Pointer(event.Timestamp)
		case event.Source == "control" && event.Name == "world_run_end":
			if series == nil || worldEnded || event.Timestamp < measurementStartNanos {
				return errors.New("repeated spot: invalid world end in diagnostic")
			}
			if err := series.accrue(event.Timestamp); err != nil {
				return err
			}
			worldEnded = true
			result.WorldEndNanos = event.Timestamp
		case event.Source == "exchange" && event.Name == "BookDelta":
			if series == nil || worldEnded {
				return errors.New("repeated spot: out-of-world book delta in diagnostic")
			}
			before := publicBookState(series)
			var delta struct {
				Side       string `json:"side"`
				Price      int64  `json:"price"`
				VisibleQty int64  `json:"visible_qty"`
				HiddenQty  int64  `json:"hidden_qty"`
				TotalQty   int64  `json:"total_qty"`
			}
			if err := decodePayload(event.Payload, &delta); err != nil {
				return err
			}
			if err := series.delta(event.Timestamp, delta.Side, delta.Price, delta.VisibleQty, delta.HiddenQty, delta.TotalQty); err != nil {
				return err
			}
			after := publicBookState(series)
			if before == "two_sided" && after != "two_sided" {
				result.LastTwoSidedEndNanos = int64Pointer(event.Timestamp)
			}
			if after == "two_sided" {
				result.LastTwoSidedEndNanos = nil
			}
			if before != "empty" && after == "empty" {
				result.FirstPermanentEmptyNanos = int64Pointer(event.Timestamp)
			}
			if after != "empty" {
				result.FirstPermanentEmptyNanos = nil
			}
		case event.Source == "exchange" && event.Name == "BookSnapshot":
			if series == nil || worldEnded {
				return errors.New("repeated spot: out-of-world book snapshot in diagnostic")
			}
			if err := series.accrue(event.Timestamp); err != nil {
				return err
			}
		case event.Source == "exchange" && event.Name == "Trade":
			if series == nil || worldEnded {
				return errors.New("repeated spot: out-of-world trade in diagnostic")
			}
			if result.FirstTradeNanos == nil {
				result.FirstTradeNanos = int64Pointer(event.Timestamp)
			}
			result.LastTradeNanos = int64Pointer(event.Timestamp)
			result.TradeCount++
		case event.Source == "actor" && event.Name == "maker_decision":
			if series == nil || worldEnded {
				return errors.New("repeated spot: out-of-world maker decision in diagnostic")
			}
			var decision worldspot.MakerDecision
			if err := decodePayload(event.Payload, &decision); err != nil {
				return err
			}
			if decision.ActorID != event.ClientID || decision.DecisionAt != event.Timestamp || !validMakerAction(decision.Action) {
				return fmt.Errorf("repeated spot: inconsistent maker decision at sequence %d", event.Sequence)
			}
			maker := makers[decision.ActorID]
			if maker == nil {
				maker = &MakerLiquidityActions{ActorID: decision.ActorID, BeforeMeasurement: make(map[string]int64), DuringMeasurement: make(map[string]int64)}
				makers[decision.ActorID] = maker
			}
			if event.Timestamp < measurementStartNanos {
				maker.BeforeMeasurement[decision.Action]++
			} else {
				maker.DuringMeasurement[decision.Action]++
			}
			point := &LiquidityDecisionPoint{DecisionAtNanos: decision.DecisionAt,
				LatestBookSourceNanos: decision.LatestBookSourceAt, BestBid: decision.BestBid,
				BestAsk: decision.BestAsk, Action: decision.Action}
			if decision.Action == "no_usable_quote" && maker.FirstNoUsableQuote == nil {
				maker.FirstNoUsableQuote = point
			}
			if decision.Action == "cancel_quotes" &&
				(decision.BestBid <= 0 || decision.BestAsk <= decision.BestBid) && maker.FirstUnusableCancel == nil {
				maker.FirstUnusableCancel = point
			}
			if decision.Action == "evaluate_placements" {
				maker.LastPlacementEvaluation = point
			}
		}
		return nil
	}
	if err := WalkEvidence(input, identity, visit); err != nil {
		return E0LiquidityDiagnostic{}, err
	}
	if series == nil || !worldEnded {
		return E0LiquidityDiagnostic{}, errors.New("repeated spot: incomplete diagnostic world")
	}
	market := series.summary()
	result.BookDurations = BookStateDurations{HorizonNanos: market.HorizonNanos,
		TwoSidedNanos: market.TwoSidedNanos, BidOnlyNanos: market.BidOnlyNanos,
		AskOnlyNanos: market.AskOnlyNanos, EmptyNanos: market.EmptyNanos}
	for _, maker := range makers {
		result.Makers = append(result.Makers, *maker)
	}
	sort.Slice(result.Makers, func(i, j int) bool { return result.Makers[i].ActorID < result.Makers[j].ActorID })
	return result, nil
}

func publicBookState(series *publicBookSeries) string {
	bid, _ := bestVisible(series.bids, true)
	ask, _ := bestVisible(series.asks, false)
	switch {
	case bid > 0 && ask > 0:
		return "two_sided"
	case bid > 0:
		return "bid_only"
	case ask > 0:
		return "ask_only"
	default:
		return "empty"
	}
}

func int64Pointer(value int64) *int64 { return &value }
