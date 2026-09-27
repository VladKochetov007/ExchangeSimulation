package repeatedspot

import (
	"errors"
	"fmt"

	worldspot "exchange_sim/simulations/repeatedspot"
)

type MakerEnvelopeSummary struct {
	WindowNanos                  int64 `json:"window_ns"`
	UpperAtLimitNanos            int64 `json:"upper_at_limit_ns"`
	LowerAtLimitNanos            int64 `json:"lower_at_limit_ns"`
	EitherAtLimitNanos           int64 `json:"either_at_limit_ns"`
	PlacementEvaluationDecisions int64 `json:"placement_evaluation_decisions"`
	BidTargetDecisions           int64 `json:"bid_target_decisions"`
	AskTargetDecisions           int64 `json:"ask_target_decisions"`
	BidResourceDeniedDecisions   int64 `json:"bid_resource_denied_decisions"`
	AskResourceDeniedDecisions   int64 `json:"ask_resource_denied_decisions"`
	BidCapClippedDecisions       int64 `json:"bid_cap_clipped_decisions"`
	AskCapClippedDecisions       int64 `json:"ask_cap_clipped_decisions"`
	TerminalLowerBaseUnits       int64 `json:"terminal_lower_base_units"`
	TerminalUpperBaseUnits       int64 `json:"terminal_upper_base_units"`
}

type envelopeOrder struct {
	requestID uint64
	orderID   uint64
	side      string
	remaining int64
}

type makerEnvelopeReplay struct {
	window        MeasurementWindow
	lastAt        int64
	limit         int64
	filled        int64
	byRequest     map[uint64]*envelopeOrder
	byOrder       map[uint64]*envelopeOrder
	upperAtLimit  int64
	lowerAtLimit  int64
	eitherAtLimit int64
	preCaptured   bool
	preLower      int64
	preUpper      int64
	preAt         int64
	bidDenied     int64
	askDenied     int64
	evaluations   int64
	bidTargets    int64
	askTargets    int64
	bidClipped    int64
	askClipped    int64
}

func newMakerEnvelopeReplay(startAt int64, window MeasurementWindow, limit int64) *makerEnvelopeReplay {
	return &makerEnvelopeReplay{window: window, lastAt: startAt, limit: limit,
		byRequest: make(map[uint64]*envelopeOrder), byOrder: make(map[uint64]*envelopeOrder)}
}

func (replay *makerEnvelopeReplay) bounds() (int64, int64, error) {
	lower, upper := replay.filled, replay.filled
	for _, order := range replay.byRequest {
		var ok bool
		if order.side == "BUY" {
			upper, ok = checkedAdd(upper, order.remaining)
		} else if order.side == "SELL" {
			lower, ok = checkedAdd(lower, -order.remaining)
		} else {
			return 0, 0, errors.New("repeated spot: invalid maker envelope side")
		}
		if !ok {
			return 0, 0, errors.New("repeated spot: maker envelope arithmetic overflows")
		}
	}
	if lower < -replay.limit || upper > replay.limit || lower > replay.filled || upper < replay.filled {
		return 0, 0, errors.New("repeated spot: pending-inclusive maker exposure breached its limit")
	}
	return lower, upper, nil
}

func (replay *makerEnvelopeReplay) advance(at int64) error {
	if at < replay.lastAt {
		return errors.New("repeated spot: maker envelope time regressed")
	}
	lower, upper, err := replay.bounds()
	if err != nil {
		return err
	}
	from := max(replay.lastAt, replay.window.StartAt)
	until := min(at, replay.window.EndAt)
	replay.lastAt = at
	if until <= from {
		return nil
	}
	duration := until - from
	upperBound := upper >= replay.limit
	lowerBound := lower <= -replay.limit
	if upperBound {
		replay.upperAtLimit += duration
	}
	if lowerBound {
		replay.lowerAtLimit += duration
	}
	if upperBound || lowerBound {
		replay.eitherAtLimit += duration
	}
	return nil
}

func (replay *makerEnvelopeReplay) sent(at int64, request sentRequest) error {
	if err := replay.advance(at); err != nil {
		return err
	}
	if !replay.preCaptured {
		var err error
		replay.preLower, replay.preUpper, err = replay.bounds()
		if err != nil {
			return err
		}
		replay.preAt, replay.preCaptured = at, true
	} else if replay.preAt != at {
		return errors.New("repeated spot: maker sent a new request before its preceding decision record")
	}
	if request.CancelReq != nil {
		if replay.byOrder[request.CancelReq.OrderID] == nil {
			return errors.New("repeated spot: maker cancelled an order absent from its local working set")
		}
		return nil
	}
	order := request.OrderReq
	if order == nil || order.RequestID == 0 || order.Qty <= 0 || replay.byRequest[order.RequestID] != nil ||
		(order.Side != "BUY" && order.Side != "SELL") {
		return errors.New("repeated spot: malformed maker reservation")
	}
	replay.byRequest[order.RequestID] = &envelopeOrder{requestID: order.RequestID, side: order.Side, remaining: order.Qty}
	_, _, err := replay.bounds()
	return err
}

func (replay *makerEnvelopeReplay) processed(at int64, response worldspot.MakerProcessedResponse) error {
	if replay.preCaptured {
		return errors.New("repeated spot: maker response interleaved within a synchronous decision")
	}
	if err := replay.advance(at); err != nil {
		return err
	}
	switch response.Kind {
	case "accepted":
		order := replay.byRequest[response.RequestID]
		if order == nil || order.orderID != 0 || response.OrderID == 0 || replay.byOrder[response.OrderID] != nil {
			return errors.New("repeated spot: maker accepted an unknown or duplicated local placement")
		}
		order.orderID = response.OrderID
		replay.byOrder[response.OrderID] = order
	case "order_rejected":
		order := replay.byRequest[response.RequestID]
		if order == nil || order.orderID != 0 {
			return errors.New("repeated spot: maker released an unknown or accepted placement")
		}
		delete(replay.byRequest, response.RequestID)
	case "fill":
		order := replay.byOrder[response.OrderID]
		if order == nil || order.side != response.Side || response.Qty <= 0 || response.Qty > order.remaining {
			return errors.New("repeated spot: maker fill exceeds local working order")
		}
		delta := response.Qty
		if order.side == "SELL" {
			delta = -delta
		}
		updated, ok := checkedAdd(replay.filled, delta)
		if !ok || updated < -replay.limit || updated > replay.limit {
			return errors.New("repeated spot: maker fill breaches working inventory limit")
		}
		replay.filled = updated
		order.remaining -= response.Qty
		if response.IsFull != (order.remaining == 0) {
			return errors.New("repeated spot: maker fill full flag contradicts local remainder")
		}
		if order.remaining == 0 {
			replay.remove(order)
		}
	case "cancelled", "forced_cancel":
		order := replay.byOrder[response.OrderID]
		if order == nil || order.remaining != response.RemainingQty {
			return errors.New("repeated spot: maker cancellation contradicts local remainder")
		}
		replay.remove(order)
	case "cancel_rejected":
		// A rejection does not release either side's still-executable exposure.
	default:
		return errors.New("repeated spot: unsupported processed maker response")
	}
	_, _, err := replay.bounds()
	return err
}

func (replay *makerEnvelopeReplay) remove(order *envelopeOrder) {
	delete(replay.byRequest, order.requestID)
	delete(replay.byOrder, order.orderID)
}

func (replay *makerEnvelopeReplay) decision(at int64, decision worldspot.MakerDecision, sends []*sentOrder, maker *makerParameters) error {
	if err := replay.advance(at); err != nil {
		return err
	}
	lower, upper, err := replay.bounds()
	if err != nil || lower != decision.WorkingLower || upper != decision.WorkingUpper || replay.filled != decision.FilledInventory {
		return fmt.Errorf("repeated spot: maker decision envelope differs from independently replayed pending/live orders")
	}
	preLower, preUpper := lower, upper
	if replay.preCaptured {
		if replay.preAt != at {
			return errors.New("repeated spot: maker outbound request has a different decision time")
		}
		preLower, preUpper = replay.preLower, replay.preUpper
	}
	if decision.Action == "evaluate_placements" {
		buyCapacity, buyOK := checkedAdd(replay.limit, -preUpper)
		sellCapacity, sellOK := checkedAdd(replay.limit, preLower)
		if !buyOK || !sellOK {
			return errors.New("repeated spot: maker placement capacity overflows")
		}
		inWindow := at >= replay.window.StartAt && at < replay.window.EndAt
		if inWindow {
			replay.evaluations++
		}
		for _, side := range []struct {
			name     string
			target   int64
			capacity int64
		}{
			{"BUY", decision.TargetBid, buyCapacity},
			{"SELL", decision.TargetAsk, sellCapacity},
		} {
			expectedQty := int64(0)
			if side.target > 0 {
				expectedQty = min(maker.quoteQty, side.capacity)
			}
			if expectedQty < maker.minQuoteQty {
				expectedQty = 0
				if inWindow && side.target > 0 {
					if side.name == "BUY" {
						replay.bidDenied++
					} else {
						replay.askDenied++
					}
				}
			}
			actualQty := int64(0)
			if inWindow && side.target > 0 {
				if side.name == "BUY" {
					replay.bidTargets++
					if side.capacity < maker.quoteQty {
						replay.bidClipped++
					}
				} else {
					replay.askTargets++
					if side.capacity < maker.quoteQty {
						replay.askClipped++
					}
				}
			}
			for _, sent := range sends {
				if sent.request.OrderReq != nil && sent.request.OrderReq.Side == side.name {
					actualQty = sent.request.OrderReq.Qty
				}
			}
			if actualQty != expectedQty {
				return errors.New("repeated spot: maker placement size differs from pre-decision inventory capacity")
			}
		}
	}
	replay.preCaptured = false
	return nil
}

func (replay *makerEnvelopeReplay) finish(at int64) (MakerEnvelopeSummary, error) {
	if replay.preCaptured {
		return MakerEnvelopeSummary{}, errors.New("repeated spot: maker outbound request lacks a decision record")
	}
	if err := replay.advance(at); err != nil {
		return MakerEnvelopeSummary{}, err
	}
	lower, upper, err := replay.bounds()
	if err != nil {
		return MakerEnvelopeSummary{}, err
	}
	return MakerEnvelopeSummary{WindowNanos: replay.window.EndAt - replay.window.StartAt,
		UpperAtLimitNanos: replay.upperAtLimit, LowerAtLimitNanos: replay.lowerAtLimit,
		EitherAtLimitNanos: replay.eitherAtLimit, PlacementEvaluationDecisions: replay.evaluations,
		BidTargetDecisions: replay.bidTargets, AskTargetDecisions: replay.askTargets,
		BidResourceDeniedDecisions: replay.bidDenied, AskResourceDeniedDecisions: replay.askDenied,
		BidCapClippedDecisions: replay.bidClipped, AskCapClippedDecisions: replay.askClipped,
		TerminalLowerBaseUnits: lower,
		TerminalUpperBaseUnits: upper}, nil
}
