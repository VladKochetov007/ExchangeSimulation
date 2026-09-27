package repeatedspot

import (
	"fmt"

	"exchange_sim/exchange"
	"exchange_sim/types"
)

// workingInventory tracks actor-known filled inventory and every request that
// could still fill. A sent cancellation does not release capacity.
type workingInventory struct {
	limit    int64
	filled   int64
	requests map[uint64]*workingOrder
	orders   map[uint64]*workingOrder
	tradeIDs map[uint64]struct{}
}

type workingOrder struct {
	requestID uint64
	orderID   uint64
	side      exchange.Side
	remaining int64
}

func newWorkingInventory(initial, limit int64) (*workingInventory, error) {
	if limit <= 0 || initial < -limit || initial > limit {
		return nil, fmt.Errorf("repeatedspot: invalid working inventory bounds")
	}
	return &workingInventory{limit: limit, filled: initial,
		requests: make(map[uint64]*workingOrder), orders: make(map[uint64]*workingOrder),
		tradeIDs: make(map[uint64]struct{})}, nil
}

// envelope is the range reachable if all remaining orders on just one side
// execute. Bids and asks cannot offset each other in this risk calculation.
func (inventory *workingInventory) envelope() (lower, upper int64, err error) {
	lower, upper = inventory.filled, inventory.filled
	for _, order := range inventory.requests {
		if order.side == exchange.Buy {
			var ok bool
			upper, ok = types.TryAdd(upper, order.remaining)
			if !ok || upper > inventory.limit {
				return 0, 0, fmt.Errorf("repeatedspot: working buy exposure exceeds limit")
			}
		} else {
			var ok bool
			lower, ok = types.TrySub(lower, order.remaining)
			if !ok || lower < -inventory.limit {
				return 0, 0, fmt.Errorf("repeatedspot: working sell exposure exceeds limit")
			}
		}
	}
	return lower, upper, nil
}

func (inventory *workingInventory) available(side exchange.Side) (int64, error) {
	lower, upper, err := inventory.envelope()
	if err != nil {
		return 0, err
	}
	switch side {
	case exchange.Buy:
		capacity, ok := types.TrySub(inventory.limit, upper)
		if !ok {
			return 0, fmt.Errorf("repeatedspot: buy capacity overflows")
		}
		return capacity, nil
	case exchange.Sell:
		capacity, ok := types.TryAdd(inventory.limit, lower)
		if !ok {
			return 0, fmt.Errorf("repeatedspot: sell capacity overflows")
		}
		return capacity, nil
	default:
		return 0, fmt.Errorf("repeatedspot: unknown working order side")
	}
}

func (inventory *workingInventory) reserve(requestID uint64, side exchange.Side, quantity int64) error {
	if requestID == 0 || quantity <= 0 {
		return fmt.Errorf("repeatedspot: invalid working request")
	}
	if _, exists := inventory.requests[requestID]; exists {
		return fmt.Errorf("repeatedspot: duplicate working request %d", requestID)
	}
	capacity, err := inventory.available(side)
	if err != nil {
		return err
	}
	if quantity > capacity {
		return fmt.Errorf("repeatedspot: working order exceeds %v capacity: qty=%d available=%d", side, quantity, capacity)
	}
	inventory.requests[requestID] = &workingOrder{requestID: requestID, side: side, remaining: quantity}
	return nil
}

func (inventory *workingInventory) accepted(requestID, orderID uint64) error {
	order := inventory.requests[requestID]
	if order == nil || order.orderID != 0 || orderID == 0 {
		return fmt.Errorf("repeatedspot: acceptance has no pending request %d", requestID)
	}
	if _, exists := inventory.orders[orderID]; exists {
		return fmt.Errorf("repeatedspot: duplicate working order %d", orderID)
	}
	order.orderID = orderID
	inventory.orders[orderID] = order
	return nil
}

func (inventory *workingInventory) rejected(requestID uint64) error {
	order := inventory.requests[requestID]
	if order == nil || order.orderID != 0 {
		return fmt.Errorf("repeatedspot: rejection has no pending request %d", requestID)
	}
	delete(inventory.requests, requestID)
	return nil
}

func (inventory *workingInventory) filledOrder(orderID, tradeID uint64, side exchange.Side, quantity int64, full bool) error {
	order := inventory.orders[orderID]
	if order == nil || order.side != side || quantity <= 0 || quantity > order.remaining {
		return fmt.Errorf("repeatedspot: unanchored or overrun working fill %d/%d", orderID, tradeID)
	}
	if _, exists := inventory.tradeIDs[tradeID]; exists {
		return fmt.Errorf("repeatedspot: duplicate working trade %d", tradeID)
	}
	remaining := order.remaining - quantity
	if full != (remaining == 0) {
		return fmt.Errorf("repeatedspot: contradictory full-fill flag for order %d", orderID)
	}
	position := inventory.filled
	var ok bool
	if side == exchange.Buy {
		position, ok = types.TryAdd(position, quantity)
	} else {
		position, ok = types.TrySub(position, quantity)
	}
	if !ok || position < -inventory.limit || position > inventory.limit {
		return fmt.Errorf("repeatedspot: fill exceeds working position limit")
	}
	inventory.tradeIDs[tradeID] = struct{}{}
	inventory.filled = position
	order.remaining = remaining
	if full {
		inventory.remove(order)
	}
	return nil
}

func (inventory *workingInventory) cancelled(orderID uint64, remaining int64) error {
	order := inventory.orders[orderID]
	if order == nil || remaining != order.remaining {
		return fmt.Errorf("repeatedspot: cancellation remainder does not reconcile for order %d", orderID)
	}
	inventory.remove(order)
	return nil
}

func (inventory *workingInventory) remove(order *workingOrder) {
	delete(inventory.requests, order.requestID)
	delete(inventory.orders, order.orderID)
}
