# Rejection Handling - Complete Guide

## Overview

Rejections are a **normal part** of trading simulations. The system has comprehensive rejection handling built-in.

## Event Types for Rejections

✅ **Implemented in `actor/events.go`**

1. `EventOrderRejected` - Order submission rejected
2. `EventOrderCancelRejected` - Order cancellation rejected

## All Rejection Reasons (10 total)

**Defined in `exchange/types.go`:**

```go
type RejectReason uint8

const (
    RejectInsufficientBalance RejectReason = iota  // 0
    RejectInvalidPrice                             // 1
    RejectInvalidQty                               // 2
    RejectUnknownClient                            // 3
    RejectUnknownInstrument                        // 4
    RejectSelfTrade                                // 5
    RejectDuplicateOrderID                         // 6
    RejectOrderNotFound                            // 7 (cancel)
    RejectOrderNotOwned                            // 8 (cancel)
    RejectOrderAlreadyFilled                       // 9 (cancel)
)
```

## Order Submission Rejections

### 1. RejectInsufficientBalance
**When:** Not enough available balance for order
**Actor response:** Query balance, reduce order size, retry
**Example:**
```go
case actor.EventOrderRejected:
    rejection := event.Data.(actor.OrderRejectedEvent)
    if rejection.Reason == exchange.RejectInsufficientBalance {
        a.QueryBalance()
        // Reduce size by 50% and retry
        newQty := originalQty / 2
        a.SubmitOrder(symbol, side, orderType, price, newQty)
    }
```

### 2. RejectInvalidPrice
**When:** Price not a multiple of tick size
**Actor response:** Round to tick size, resubmit
**Example:**
```go
if rejection.Reason == exchange.RejectInvalidPrice {
    tickSize := instrument.TickSize()
    roundedPrice := (price / tickSize) * tickSize
    a.SubmitOrder(symbol, side, orderType, roundedPrice, qty)
}
```

### 3. RejectInvalidQty
**When:** Quantity below minimum order size
**Actor response:** Increase qty or skip order
**Example:**
```go
if rejection.Reason == exchange.RejectInvalidQty {
    minSize := instrument.MinOrderSize()
    if qty < minSize {
        qty = minSize
        a.SubmitOrder(symbol, side, orderType, price, qty)
    }
}
```

### 4. RejectUnknownClient
**When:** Client ID not registered with exchange
**Actor response:** Fatal error, should not happen in normal operation
**Example:**
```go
if rejection.Reason == exchange.RejectUnknownClient {
    // This indicates serious configuration error
    panic("Client not connected to exchange")
}
```

### 5. RejectUnknownInstrument
**When:** Symbol doesn't exist on exchange
**Actor response:** Query available instruments
**Example:**
```go
if rejection.Reason == exchange.RejectUnknownInstrument {
    instruments := exchange.ListInstruments("", "USD")
    // Pick valid symbol and retry
}
```

### 6. RejectSelfTrade
**When:** Order would immediately match actor's own resting order
**Actor response:** Adjust price slightly, wait, retry
**Example:**
```go
if rejection.Reason == exchange.RejectSelfTrade {
    time.Sleep(100 * time.Millisecond)
    // Adjust price to avoid self-trade
    if side == exchange.Buy {
        price -= tickSize  // Lower bid
    } else {
        price += tickSize  // Raise ask
    }
    a.SubmitOrder(symbol, side, orderType, price, qty)
}
```

### 7. RejectDuplicateOrderID
**When:** OrderID collision (extremely rare)
**Actor response:** Retry, exchange will assign new ID
**Example:**
```go
if rejection.Reason == exchange.RejectDuplicateOrderID {
    // Just retry - exchange will assign new ID
    a.SubmitOrder(symbol, side, orderType, price, qty)
}
```

## Order Cancellation Rejections

### 8. RejectOrderNotFound
**When:** Order doesn't exist (already filled, cancelled, or never existed)
**Actor response:** Remove from local tracking
**Example:**
```go
case actor.EventOrderCancelRejected:
    rejection := event.Data.(actor.OrderCancelRejectedEvent)
    if rejection.Reason == exchange.RejectOrderNotFound {
        // Order already gone - clean up tracking
        delete(a.activeOrders, rejection.OrderID)
    }
```

### 9. RejectOrderNotOwned
**When:** Trying to cancel another client's order
**Actor response:** Logic error, indicates bug in order tracking
**Example:**
```go
if rejection.Reason == exchange.RejectOrderNotOwned {
    // This should NEVER happen - indicates bug
    // Order tracking is corrupted
    a.rebuildOrderTracking()
}
```

### 10. RejectOrderAlreadyFilled
**When:** Order fully filled before cancel request processed
**Actor response:** Wait for fill event, update position
**Example:**
```go
if rejection.Reason == exchange.RejectOrderAlreadyFilled {
    // Order completed successfully
    // EventOrderFilled should arrive shortly
    // Don't take action until fill event received
}
```

## Implementation in Code

### BaseActor Handles Rejections

**File:** `actor/actor.go:77-108`

```go
func (a *BaseActor) handleResponse(resp exchange.Response) {
    if !resp.Success {
        a.eventCh <- &Event{
            Type: EventOrderRejected,
            Data: OrderRejectedEvent{
                RequestID: resp.RequestID,
                Reason:    resp.Error,
            },
        }
        return
    }
    // ... handle success cases
}
```

### Exchange Validates and Rejects

**Order submission validation** (`exchange/exchange.go:113-220`):
- Client exists
- Instrument exists
- Price valid (tick size)
- Quantity valid (min size)
- Sufficient balance
- Self-trade check (during matching)

**Cancel validation** (`exchange/exchange.go:222-266`):
- Order exists
- Client owns order
- Order not already filled

## Complete Event Flow

### Successful Order
```
Actor → SubmitOrder → Exchange → Validate ✓ → Response(Success=true, OrderID)
      → handleResponse → EventOrderAccepted → OnEvent
```

### Rejected Order
```
Actor → SubmitOrder → Exchange → Validate ✗ → Response(Success=false, Error=Reason)
      → handleResponse → EventOrderRejected → OnEvent → Handle & Retry
```

### Successful Cancel
```
Actor → CancelOrder → Exchange → Validate ✓ → Response(Success=true, RemainingQty)
      → handleResponse → EventOrderCancelled → OnEvent
```

### Rejected Cancel
```
Actor → CancelOrder → Exchange → Validate ✗ → Response(Success=false, Error=Reason)
      → handleResponse → EventOrderCancelRejected → OnEvent → Handle
```

## Testing Coverage

**Tests in `actor/actor_test.go`:**
- ✅ `TestBaseActorHandleResponseRejection` - Order rejection handling
- ✅ `TestBaseActorHandleResponseSuccess` - Order acceptance
- ✅ `TestBaseActorHandleResponseCancelled` - Cancel success

**Tests in `exchange/exchange_instrument_test.go`:**
- ✅ `TestCancelOrderValidationNotFound` - RejectOrderNotFound
- ✅ `TestCancelOrderValidationNotOwned` - RejectOrderNotOwned
- ✅ `TestCancelOrderValidationAfterPartialFill` - Partial fill cancel

## Actor Best Practices

### 1. Always Handle Rejections
```go
func (a *MyActor) OnEvent(event *Event) {
    switch event.Type {
    case EventOrderRejected:
        // REQUIRED - handle all rejection cases
        rejection := event.Data.(OrderRejectedEvent)
        a.handleRejection(rejection)

    case EventOrderCancelRejected:
        // REQUIRED - handle cancel failures
        cancelRej := event.Data.(OrderCancelRejectedEvent)
        a.handleCancelRejection(cancelRej)
    }
}
```

### 2. Track Pending Orders
```go
type MyActor struct {
    *BaseActor
    pendingOrders map[uint64]*OrderRequest  // RequestID → Request
    activeOrders  map[uint64]*OrderInfo      // OrderID → Info
}
```

### 3. Implement Retry Logic
```go
func (a *MyActor) handleRejection(rejection OrderRejectedEvent) {
    req, ok := a.pendingOrders[rejection.RequestID]
    if !ok {
        return
    }

    if req.retries >= maxRetries {
        delete(a.pendingOrders, rejection.RequestID)
        return
    }

    switch rejection.Reason {
    case exchange.RejectInsufficientBalance:
        req.qty = req.qty * 3 / 4  // Reduce 25%
        req.retries++
        a.SubmitOrder(req.symbol, req.side, req.orderType, req.price, req.qty)

    case exchange.RejectSelfTrade:
        time.Sleep(100 * time.Millisecond)
        req.price += tickSize  // Adjust price
        req.retries++
        a.SubmitOrder(req.symbol, req.side, req.orderType, req.price, req.qty)

    default:
        delete(a.pendingOrders, rejection.RequestID)
    }
}
```

### 4. Set Retry Limits
```go
const (
    maxRetries = 3
    retryBackoffMs = 100
)
```

### 5. Log Rejections
```go
func (a *MyActor) handleRejection(rejection OrderRejectedEvent) {
    log.Printf("Order rejected: RequestID=%d Reason=%v",
               rejection.RequestID, rejection.Reason)
    // ... handle rejection
}
```

## Summary

✅ **10 rejection reasons** fully defined and implemented
✅ **2 rejection event types** (OrderRejected, OrderCancelRejected)
✅ **BaseActor** converts failures to events automatically
✅ **Exchange** validates all requests and returns rejection reasons
✅ **Tests** verify rejection handling works correctly
✅ **Documentation** shows how to handle each rejection type

**Rejections are expected and normal** - actors must handle them to be robust!
