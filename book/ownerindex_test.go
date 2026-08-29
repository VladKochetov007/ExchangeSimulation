package book

import (
	"testing"

	etypes "exchange_sim/types"
)

func restingOrder(id, clientID uint64, price, qty int64) *etypes.Order {
	return &etypes.Order{
		ID: id, ClientID: clientID, Price: price, Qty: qty,
		Side: etypes.Buy, Type: etypes.LimitOrder, TimeInForce: etypes.GTC,
	}
}

// ownersFromScan is the answer the admission checks used to compute: every
// order on the side that belongs to the client. The index has to agree with it
// exactly, because that agreement is the entire safety argument for narrowing
// the scan.
func ownersFromScan(b *Book, clientID uint64) map[uint64]*etypes.Order {
	found := make(map[uint64]*etypes.Order)
	for id, order := range b.Orders {
		if order.ClientID == clientID {
			found[id] = order
		}
	}
	return found
}

func requireIndexAgreesWithScan(t *testing.T, b *Book, clients []uint64) {
	t.Helper()
	if !b.TracksOwners() {
		t.Fatal("book does not maintain an owner index")
	}
	for _, clientID := range clients {
		want := ownersFromScan(b, clientID)
		got := b.OrdersForClient(clientID)
		if len(got) != len(want) {
			t.Fatalf("client %d: index has %d orders, scan finds %d", clientID, len(got), len(want))
		}
		for id, order := range want {
			if got[id] != order {
				t.Fatalf("client %d: index missing or wrong order %d", clientID, id)
			}
		}
	}
}

// TestOwnerIndexAgreesWithAFullScan drives orders through add, cancel and
// fill-removal and requires the index to match a full scan after every step.
func TestOwnerIndexAgreesWithAFullScan(t *testing.T) {
	b := NewBook(etypes.Buy)
	clients := []uint64{1, 2, 3}

	orders := []*etypes.Order{
		restingOrder(10, 1, 100, 5),
		restingOrder(11, 2, 100, 5),
		restingOrder(12, 1, 99, 5),
		restingOrder(13, 3, 98, 5),
		restingOrder(14, 1, 98, 5),
		restingOrder(15, 2, 97, 5),
	}
	for _, order := range orders {
		if !b.AddOrder(order) {
			t.Fatalf("AddOrder(%d) refused", order.ID)
		}
		requireIndexAgreesWithScan(t, b, clients)
	}

	// A duplicate must change nothing.
	if b.AddOrder(restingOrder(10, 1, 100, 5)) {
		t.Fatal("AddOrder accepted a duplicate ID")
	}
	requireIndexAgreesWithScan(t, b, clients)

	for _, id := range []uint64{12, 11, 10, 14, 13, 15} {
		if b.CancelOrder(id) == nil {
			t.Fatalf("CancelOrder(%d) found nothing", id)
		}
		requireIndexAgreesWithScan(t, b, clients)
	}
	for _, clientID := range clients {
		if got := b.OrdersForClient(clientID); len(got) != 0 {
			t.Fatalf("client %d retains %d orders after cancelling everything", clientID, len(got))
		}
	}
}

// TestOwnerIndexReleasesFilledOrders covers the path where the matcher has
// already unlinked an order from its price level, so CancelOrder takes the
// Parent == nil branch.
func TestOwnerIndexReleasesFilledOrders(t *testing.T) {
	b := NewBook(etypes.Buy)
	order := restingOrder(20, 7, 100, 5)
	if !b.AddOrder(order) {
		t.Fatal("AddOrder refused")
	}
	UnlinkOrder(order)
	order.Parent = nil
	if b.RemoveFilledOrder(20) == nil {
		t.Fatal("RemoveFilledOrder found nothing")
	}
	if got := b.OrdersForClient(7); len(got) != 0 {
		t.Fatalf("filled order stayed in the owner index: %v", got)
	}
	if _, present := b.Orders[20]; present {
		t.Fatal("filled order stayed in the ID index")
	}
}

// TestDetachedBookHasNoOwnerIndex pins the preview-clone contract: a detached
// book must report that it does not track owners, so callers fall back to the
// full scan rather than reading an empty index and concluding the client has no
// resting orders.
func TestDetachedBookHasNoOwnerIndex(t *testing.T) {
	b := NewDetachedBook(etypes.Buy, 4, 4)
	if b.TracksOwners() {
		t.Fatal("a detached book must not claim to track owners")
	}
	if !b.AddOrder(restingOrder(30, 5, 100, 5)) {
		t.Fatal("AddOrder refused")
	}
	if got := b.OrdersForClient(5); got != nil {
		t.Fatalf("a detached book reported an owner index: %v", got)
	}
	if len(b.Orders) != 1 {
		t.Fatalf("detached book holds %d orders, want 1", len(b.Orders))
	}
}

// TestOwnerIndexTracksManyClients exercises index growth and removal at a size
// where a real book operates, so a bookkeeping error shows up as a mismatch
// rather than passing on a two-order fixture.
func TestOwnerIndexTracksManyClients(t *testing.T) {
	b := NewBook(etypes.Buy)
	var clients []uint64
	for clientID := uint64(1); clientID <= 25; clientID++ {
		clients = append(clients, clientID)
	}
	id := uint64(1)
	for round := 0; round < 8; round++ {
		for _, clientID := range clients {
			if !b.AddOrder(restingOrder(id, clientID, int64(100-round), 3)) {
				t.Fatalf("AddOrder(%d) refused", id)
			}
			id++
		}
	}
	requireIndexAgreesWithScan(t, b, clients)
	if len(b.Orders) != 200 {
		t.Fatalf("book holds %d orders, want 200", len(b.Orders))
	}

	// Cancel every other order and re-check.
	for cancel := uint64(1); cancel < id; cancel += 2 {
		if b.CancelOrder(cancel) == nil {
			t.Fatalf("CancelOrder(%d) found nothing", cancel)
		}
	}
	requireIndexAgreesWithScan(t, b, clients)
}
