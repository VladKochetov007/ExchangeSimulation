package repeatedspot

import (
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
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
	ActorID                  uint64                  `json:"actor_id"`
	BeforeMeasurement        map[string]int64        `json:"before_measurement"`
	DuringMeasurement        map[string]int64        `json:"during_measurement"`
	AfterMeasurement         map[string]int64        `json:"after_measurement"`
	FirstNoUsableQuote       *LiquidityDecisionPoint `json:"first_no_usable_quote,omitempty"`
	FirstUnusableCancel      *LiquidityDecisionPoint `json:"first_unusable_cancel,omitempty"`
	LastPlacementEvaluation  *LiquidityDecisionPoint `json:"last_placement_evaluation,omitempty"`
	ShadowEligibleDecisions  int64                   `json:"shadow_eligible_decisions_in_measurement,omitempty"`
	ShadowWaitingDecisions   int64                   `json:"shadow_waiting_decisions_in_measurement,omitempty"`
	ShadowEvaluations        int64                   `json:"shadow_placement_evaluations_in_measurement,omitempty"`
	CachedReferenceDecisions int64                   `json:"cached_reference_decisions_in_measurement,omitempty"`
	CachedReferenceAgeSum    int64                   `json:"cached_reference_age_sum_ns_in_measurement,omitempty"`
	CachedReferenceAgeMax    int64                   `json:"cached_reference_age_max_ns_in_measurement,omitempty"`
}

type BookStateDurations struct {
	HorizonNanos  int64 `json:"horizon_ns"`
	TwoSidedNanos int64 `json:"two_sided_ns"`
	BidOnlyNanos  int64 `json:"bid_only_ns"`
	AskOnlyNanos  int64 `json:"ask_only_ns"`
	EmptyNanos    int64 `json:"empty_ns"`
}

type E0LiquidityDiagnostic struct {
	Evidence                        EvidenceIdentity        `json:"evidence"`
	MeasurementStartNanos           int64                   `json:"measurement_start_ns"`
	MeasurementEndNanos             int64                   `json:"measurement_end_ns"`
	WorldEndNanos                   int64                   `json:"world_end_ns"`
	FirstPermanentEmptyNanos        *int64                  `json:"first_permanent_empty_ns,omitempty"`
	LastTwoSidedEndNanos            *int64                  `json:"last_two_sided_end_ns,omitempty"`
	FirstTradeNanos                 *int64                  `json:"first_trade_ns,omitempty"`
	LastTradeNanos                  *int64                  `json:"last_trade_ns,omitempty"`
	TradeCount                      int64                   `json:"trade_count"`
	BookDurations                   BookStateDurations      `json:"book_durations"`
	MeasurementBookDurations        BookStateDurations      `json:"measurement_book_durations"`
	MeasurementSpreadPriceUnitNanos string                  `json:"measurement_spread_price_unit_ns"`
	FirstWindowTwoSidedMidPrice     *int64                  `json:"first_window_two_sided_mid_price_units,omitempty"`
	LastWindowTwoSidedMidPrice      *int64                  `json:"last_window_two_sided_mid_price_units,omitempty"`
	ShadowReferenceMaxAgeNanos      int64                   `json:"shadow_reference_max_age_ns,omitempty"`
	Makers                          []MakerLiquidityActions `json:"makers"`
}

// DiagnoseE0LiquidityEvidence reconstructs exchange book-state durations and
// maker decisions from retained evidence. It does not substitute for strict
// economic replay or infer why a maker selected a quote rule.
func DiagnoseE0LiquidityEvidence(input io.Reader, identity EvidenceIdentity, window MeasurementWindow, tickSize int64) (E0LiquidityDiagnostic, error) {
	return DiagnoseE0LiquidityEvidenceWithReference(input, identity, window, tickSize, 0)
}

func DiagnoseE0LiquidityEvidenceWithReference(input io.Reader, identity EvidenceIdentity, window MeasurementWindow, tickSize, shadowAgeNanos int64) (E0LiquidityDiagnostic, error) {
	if window.StartAt < 0 || window.EndAt <= window.StartAt || tickSize <= 0 || shadowAgeNanos < 0 {
		return E0LiquidityDiagnostic{}, errors.New("repeated spot: invalid liquidity diagnostic contract")
	}
	result := E0LiquidityDiagnostic{Evidence: identity, MeasurementStartNanos: window.StartAt,
		MeasurementEndNanos: window.EndAt, ShadowReferenceMaxAgeNanos: shadowAgeNanos,
		MeasurementBookDurations: BookStateDurations{HorizonNanos: window.EndAt - window.StartAt}}
	makers := make(map[uint64]*MakerLiquidityActions)
	lastTwoSided := make(map[uint64]worldspot.MakerObservation)
	latestSourceAt := make(map[uint64]int64)
	var series *publicBookSeries
	worldEnded := false
	var lastWindowAt int64
	var measurementSpread big.Int
	accrueWindow := func(at int64) error {
		if at < lastWindowAt {
			return errors.New("repeated spot: diagnostic event time regressed")
		}
		from := max(lastWindowAt, window.StartAt)
		until := min(at, window.EndAt)
		lastWindowAt = at
		if until <= from {
			return nil
		}
		duration := until - from
		bid, _ := bestVisible(series.bids, true)
		ask, _ := bestVisible(series.asks, false)
		switch publicBookState(series) {
		case "two_sided":
			if bid >= ask {
				return errors.New("repeated spot: crossed public book in measurement window")
			}
			result.MeasurementBookDurations.TwoSidedNanos += duration
			measurementSpread.Add(&measurementSpread, new(big.Int).Mul(big.NewInt(ask-bid), big.NewInt(duration)))
			mid := bid + (ask-bid)/2
			if result.FirstWindowTwoSidedMidPrice == nil {
				result.FirstWindowTwoSidedMidPrice = int64Pointer(mid)
			}
			result.LastWindowTwoSidedMidPrice = int64Pointer(mid)
		case "bid_only":
			result.MeasurementBookDurations.BidOnlyNanos += duration
		case "ask_only":
			result.MeasurementBookDurations.AskOnlyNanos += duration
		default:
			result.MeasurementBookDurations.EmptyNanos += duration
		}
		return nil
	}
	visit := func(event Event) error {
		if series != nil {
			if err := accrueWindow(event.Timestamp); err != nil {
				return err
			}
		}
		switch {
		case event.Source == "control" && event.Name == "world_begin":
			if series != nil {
				return errors.New("repeated spot: duplicate world begin in diagnostic")
			}
			series = newPublicBookSeries(event.Timestamp, tickSize)
			lastWindowAt = event.Timestamp
			result.FirstPermanentEmptyNanos = int64Pointer(event.Timestamp)
		case event.Source == "control" && event.Name == "world_run_end":
			if series == nil || worldEnded || event.Timestamp != window.EndAt {
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
		case event.Source == "actor" && event.Name == "maker_observation":
			if series == nil || worldEnded {
				return errors.New("repeated spot: out-of-world maker observation in diagnostic")
			}
			var observation worldspot.MakerObservation
			if err := decodePayload(event.Payload, &observation); err != nil {
				return err
			}
			if observation.Kind == "snapshot" {
				previous, seen := latestSourceAt[observation.ActorID]
				if observation.ActorID != event.ClientID || observation.ProcessedAt != event.Timestamp ||
					observation.SourceAt > observation.ProcessedAt || seen && observation.SourceAt < previous {
					return errors.New("repeated spot: invalid diagnostic maker snapshot ordering")
				}
				latestSourceAt[observation.ActorID] = observation.SourceAt
				if observation.BestBid > 0 && observation.BestAsk > observation.BestBid {
					lastTwoSided[observation.ActorID] = observation
				}
			}
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
				maker = &MakerLiquidityActions{ActorID: decision.ActorID, BeforeMeasurement: make(map[string]int64),
					DuringMeasurement: make(map[string]int64), AfterMeasurement: make(map[string]int64)}
				makers[decision.ActorID] = maker
			}
			if event.Timestamp < window.StartAt {
				maker.BeforeMeasurement[decision.Action]++
			} else if event.Timestamp < window.EndAt {
				maker.DuringMeasurement[decision.Action]++
				if decision.ReferenceMode == "cached_two_sided" {
					age := decision.DecisionAt - decision.ReferenceSourceAt
					if shadowAgeNanos <= 0 || age < 0 || age >= shadowAgeNanos ||
						maker.CachedReferenceAgeSum > math.MaxInt64-age {
						return errors.New("repeated spot: invalid cached-reference age in diagnostic")
					}
					maker.CachedReferenceDecisions++
					maker.CachedReferenceAgeSum += age
					maker.CachedReferenceAgeMax = max(maker.CachedReferenceAgeMax, age)
				}
				if shadowAgeNanos > 0 && (decision.BestBid <= 0 || decision.BestAsk <= 0) {
					prior, known := lastTwoSided[decision.ActorID]
					if known && decision.DecisionAt >= prior.SourceAt &&
						decision.DecisionAt-prior.SourceAt < shadowAgeNanos {
						maker.ShadowEligibleDecisions++
						if decision.Action == "await_response" {
							maker.ShadowWaitingDecisions++
						}
						if decision.Action == "evaluate_placements" {
							maker.ShadowEvaluations++
						}
					}
				}
			} else {
				maker.AfterMeasurement[decision.Action]++
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
	measured := result.MeasurementBookDurations
	result.MeasurementSpreadPriceUnitNanos = measurementSpread.String()
	if measured.TwoSidedNanos+measured.BidOnlyNanos+measured.AskOnlyNanos+measured.EmptyNanos != measured.HorizonNanos {
		return E0LiquidityDiagnostic{}, errors.New("repeated spot: incomplete measurement book-state duration")
	}
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
