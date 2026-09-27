package repeatedspot

import (
	"math"
	"strings"
	"testing"

	"exchange_sim/exchange"
)

func TestWorkingInventoryIncludesPendingAndCancelInFlight(t *testing.T) {
	inventory, err := newWorkingInventory(0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if err := inventory.reserve(1, exchange.Buy, 6); err != nil {
		t.Fatal(err)
	}
	if err := inventory.reserve(2, exchange.Sell, 6); err != nil {
		t.Fatal(err)
	}
	if lower, upper, err := inventory.envelope(); err != nil || lower != -6 || upper != 6 {
		t.Fatalf("pending envelope = (%d, %d, %v), want (-6, 6, nil)", lower, upper, err)
	}
	if err := inventory.reserve(3, exchange.Buy, 5); err == nil {
		t.Fatal("pending bid must consume capacity independently of the pending ask")
	}
	if err := inventory.accepted(1, 101); err != nil {
		t.Fatal(err)
	}
	if err := inventory.accepted(2, 102); err != nil {
		t.Fatal(err)
	}
	// Sending a cancellation has no effect on the ledger. Only a reconciled
	// venue acknowledgement can release its outstanding exposure.
	if capacity, err := inventory.available(exchange.Buy); err != nil || capacity != 4 {
		t.Fatalf("cancel-pending buy capacity = %d, %v; want 4", capacity, err)
	}
	if err := inventory.filledOrder(101, 501, exchange.Buy, 2, false); err != nil {
		t.Fatal(err)
	}
	if lower, upper, err := inventory.envelope(); err != nil || lower != -4 || upper != 6 {
		t.Fatalf("partial-fill envelope = (%d, %d, %v), want (-4, 6, nil)", lower, upper, err)
	}
	if err := inventory.cancelled(101, 4); err != nil {
		t.Fatal(err)
	}
	if capacity, err := inventory.available(exchange.Buy); err != nil || capacity != 8 {
		t.Fatalf("post-cancel buy capacity = %d, %v; want 8", capacity, err)
	}
	if err := inventory.reserve(4, exchange.Buy, 8); err != nil {
		t.Fatal(err)
	}
	if lower, upper, err := inventory.envelope(); err != nil || lower != -4 || upper != 10 {
		t.Fatalf("boundary envelope = (%d, %d, %v), want (-4, 10, nil)", lower, upper, err)
	}
}

func TestWorkingInventoryRejectsContradictoryLifecycleWithoutReleasingRisk(t *testing.T) {
	inventory, err := newWorkingInventory(0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if err := inventory.reserve(1, exchange.Buy, 8); err != nil {
		t.Fatal(err)
	}
	if err := inventory.accepted(1, 101); err != nil {
		t.Fatal(err)
	}
	if err := inventory.filledOrder(101, 501, exchange.Buy, 3, false); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []struct {
		name string
		call func() error
	}{
		{"stale remainder", func() error { return inventory.cancelled(101, 8) }},
		{"duplicate trade", func() error { return inventory.filledOrder(101, 501, exchange.Buy, 1, false) }},
		{"wrong side", func() error { return inventory.filledOrder(101, 502, exchange.Sell, 1, false) }},
		{"overrun", func() error { return inventory.filledOrder(101, 502, exchange.Buy, 6, true) }},
		{"wrong full flag", func() error { return inventory.filledOrder(101, 502, exchange.Buy, 5, false) }},
		{"late rejection", func() error { return inventory.rejected(1) }},
		{"duplicate acceptance", func() error { return inventory.accepted(1, 101) }},
	} {
		t.Run(invalid.name, func(t *testing.T) {
			if err := invalid.call(); err == nil {
				t.Fatal("contradictory event was accepted")
			}
			if lower, upper, err := inventory.envelope(); err != nil || lower != 3 || upper != 8 {
				t.Fatalf("invalid event changed exposure: (%d, %d, %v)", lower, upper, err)
			}
		})
	}
	if err := inventory.cancelled(101, 5); err != nil {
		t.Fatal(err)
	}
	if err := inventory.filledOrder(101, 502, exchange.Buy, 1, false); err == nil {
		t.Fatal("post-cancellation fill has no live order anchor")
	}
	if lower, upper, err := inventory.envelope(); err != nil || lower != 3 || upper != 3 {
		t.Fatalf("terminal envelope = (%d, %d, %v), want (3, 3, nil)", lower, upper, err)
	}
}

func TestWorkingInventoryRejectionAndFullFill(t *testing.T) {
	inventory, err := newWorkingInventory(-4, 10)
	if err != nil {
		t.Fatal(err)
	}
	if err := inventory.reserve(1, exchange.Sell, 7); err == nil {
		t.Fatal("sell would breach lower bound")
	}
	if err := inventory.reserve(1, exchange.Sell, 6); err != nil {
		t.Fatal(err)
	}
	if err := inventory.rejected(1); err != nil {
		t.Fatal(err)
	}
	if err := inventory.reserve(2, exchange.Buy, 4); err != nil {
		t.Fatal(err)
	}
	if err := inventory.accepted(2, 102); err != nil {
		t.Fatal(err)
	}
	if err := inventory.filledOrder(102, 502, exchange.Buy, 4, true); err != nil {
		t.Fatal(err)
	}
	if inventory.filled != 0 || len(inventory.orders) != 0 || len(inventory.requests) != 0 {
		t.Fatalf("full fill did not settle state: %+v", inventory)
	}
}

func TestWorkingInventoryOverflowAndInvalidSideFailClosed(t *testing.T) {
	if _, err := newWorkingInventory(11, 10); err == nil {
		t.Fatal("initial position outside limit accepted")
	}
	inventory, err := newWorkingInventory(-math.MaxInt64, math.MaxInt64)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inventory.available(exchange.Buy); err == nil || !strings.Contains(err.Error(), "overflows") {
		t.Fatalf("buy capacity overflow = %v", err)
	}
	if err := inventory.reserve(1, exchange.Side(99), 1); err == nil {
		t.Fatal("invalid side accepted")
	}
}
