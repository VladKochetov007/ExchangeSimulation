package repeatedspot

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"slices"

	"exchange_sim/types"
)

type RestingDepthSummary struct {
	WindowNanos           int64  `json:"window_ns"`
	BidPresentNanos       int64  `json:"bid_present_ns"`
	AskPresentNanos       int64  `json:"ask_present_ns"`
	TwoSidedNanos         int64  `json:"two_sided_ns"`
	BidDepthBaseUnitNanos string `json:"bid_depth_base_unit_ns"`
	AskDepthBaseUnitNanos string `json:"ask_depth_base_unit_ns"`
	TerminalBidBaseUnits  int64  `json:"terminal_bid_base_units"`
	TerminalAskBaseUnits  int64  `json:"terminal_ask_base_units"`
}

type restingDepthSeries struct {
	window     MeasurementWindow
	lastAt     int64
	bidQty     int64
	askQty     int64
	bidPresent int64
	askPresent int64
	twoSided   int64
	bidDepth   big.Int
	askDepth   big.Int
}

type restingOrder struct {
	clientID uint64
	price    int64
	side     string
	qty      int64
}

type restingBook struct {
	orders map[uint64]*restingOrder
}

func newRestingDepthSeries(startAt int64, window MeasurementWindow) *restingDepthSeries {
	return &restingDepthSeries{window: window, lastAt: startAt}
}

func (series *restingDepthSeries) advance(at int64) error {
	if at < series.lastAt {
		return errors.New("repeated spot: resting depth time regressed")
	}
	from := max(series.lastAt, series.window.StartAt)
	until := min(at, series.window.EndAt)
	series.lastAt = at
	if until <= from {
		return nil
	}
	duration := until - from
	if series.bidQty > 0 {
		series.bidPresent += duration
	}
	if series.askQty > 0 {
		series.askPresent += duration
	}
	if series.bidQty > 0 && series.askQty > 0 {
		series.twoSided += duration
	}
	series.bidDepth.Add(&series.bidDepth, new(big.Int).Mul(big.NewInt(series.bidQty), big.NewInt(duration)))
	series.askDepth.Add(&series.askDepth, new(big.Int).Mul(big.NewInt(series.askQty), big.NewInt(duration)))
	return nil
}

func (series *restingDepthSeries) change(at int64, side string, delta int64) error {
	if err := series.advance(at); err != nil {
		return err
	}
	var target *int64
	switch side {
	case "BUY":
		target = &series.bidQty
	case "SELL":
		target = &series.askQty
	default:
		return errors.New("repeated spot: resting order has invalid side")
	}
	updated, ok := checkedAdd(*target, delta)
	if !ok || updated < 0 {
		return errors.New("repeated spot: resting depth is negative or overflows")
	}
	*target = updated
	return nil
}

func (series *restingDepthSeries) finish(at int64) (RestingDepthSummary, error) {
	if err := series.advance(at); err != nil {
		return RestingDepthSummary{}, err
	}
	return RestingDepthSummary{WindowNanos: series.window.EndAt - series.window.StartAt,
		BidPresentNanos: series.bidPresent, AskPresentNanos: series.askPresent,
		TwoSidedNanos: series.twoSided, BidDepthBaseUnitNanos: series.bidDepth.String(),
		AskDepthBaseUnitNanos: series.askDepth.String(), TerminalBidBaseUnits: series.bidQty,
		TerminalAskBaseUnits: series.askQty}, nil
}

func (state *replayState) restingOrderEvent(event Event) error {
	switch event.Name {
	case "OrderAccepted":
		var order acceptedOrder
		if err := json.Unmarshal(event.Payload, &order); err != nil {
			return err
		}
		if order.Type != "LIMIT" || order.TimeInForce != "GTC" || order.Visibility != "NORMAL" {
			return nil
		}
		if state.resting.orders[order.OrderID] != nil {
			return errors.New("repeated spot: repeated resting order identity")
		}
		state.resting.orders[order.OrderID] = &restingOrder{clientID: event.ClientID,
			price: order.Price, side: order.Side, qty: order.Qty}
		return state.changeRestingDepth(event.Timestamp, event.ClientID, order.Side, order.Qty)
	case "OrderFill":
		var fill recordedFill
		if err := json.Unmarshal(event.Payload, &fill); err != nil {
			return err
		}
		order := state.resting.orders[fill.OrderID]
		if order == nil {
			return nil
		}
		if order.clientID != event.ClientID || order.qty < fill.Qty {
			return errors.New("repeated spot: fill exceeds resting client order")
		}
		order.qty -= fill.Qty
		if err := state.changeRestingDepth(event.Timestamp, event.ClientID, order.side, -fill.Qty); err != nil {
			return err
		}
		if order.qty == 0 {
			delete(state.resting.orders, fill.OrderID)
		}
	case "OrderCancelled":
		var cancelled struct {
			OrderID uint64 `json:"order_id"`
		}
		if err := json.Unmarshal(event.Payload, &cancelled); err != nil {
			return err
		}
		order := state.resting.orders[cancelled.OrderID]
		if order == nil {
			return nil
		}
		if order.clientID != event.ClientID {
			return errors.New("repeated spot: cancellation owns another client's resting order")
		}
		delete(state.resting.orders, cancelled.OrderID)
		return state.changeRestingDepth(event.Timestamp, event.ClientID, order.side, -order.qty)
	}
	return nil
}

func (state *replayState) changeRestingDepth(at int64, clientID uint64, side string, delta int64) error {
	account := state.accounts[clientID]
	if account == nil {
		return errors.New("repeated spot: resting order belongs to an unknown account")
	}
	if err := account.restingDepth.change(at, side, delta); err != nil {
		return err
	}
	if err := state.publicResting.change(at, side, delta); err != nil {
		return err
	}
	if account.maker != nil {
		return state.makerResting.change(at, side, delta)
	}
	return nil
}

func (book *restingBook) verifySnapshot(bids, asks []types.PriceLevel) error {
	computedBids := make(map[int64]int64)
	computedAsks := make(map[int64]int64)
	for _, order := range book.orders {
		levels := computedBids
		if order.side == "SELL" {
			levels = computedAsks
		}
		updated, ok := checkedAdd(levels[order.price], order.qty)
		if !ok || updated <= 0 {
			return errors.New("repeated spot: resting price-level depth overflows")
		}
		levels[order.price] = updated
	}
	if !slices.Equal(bids, displayedLevels(computedBids, true)) ||
		!slices.Equal(asks, displayedLevels(computedAsks, false)) {
		return fmt.Errorf("repeated spot: public book differs from independently replayed resting client orders")
	}
	return nil
}

func (state *replayState) finishRestingDepth(terminalAt int64) error {
	for _, account := range state.accounts {
		summary, err := account.restingDepth.finish(terminalAt)
		if err != nil {
			return err
		}
		account.restingSummary = summary
	}
	var err error
	state.publicWindowDepth, err = state.publicResting.finish(terminalAt)
	if err != nil {
		return err
	}
	state.makerWindowDepth, err = state.makerResting.finish(terminalAt)
	if err != nil {
		return err
	}
	if state.makerWindowDepth.BidPresentNanos > state.publicWindowDepth.BidPresentNanos ||
		state.makerWindowDepth.AskPresentNanos > state.publicWindowDepth.AskPresentNanos ||
		state.makerWindowDepth.TwoSidedNanos > state.publicWindowDepth.TwoSidedNanos {
		return errors.New("repeated spot: maker-only depth exceeds public depth")
	}
	return nil
}
