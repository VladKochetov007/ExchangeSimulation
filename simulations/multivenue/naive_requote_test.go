package multivenue

import (
	"context"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	etypes "exchange_sim/types"
)

func makerSnapshot(symbol string, bid, ask int64) *actor.Event {
	return &actor.Event{
		Type: actor.EventBookSnapshot,
		Data: actor.BookSnapshotEvent{Symbol: symbol, Snapshot: &etypes.BookSnapshot{
			Bids: []etypes.PriceLevel{{Price: bid, VisibleQty: 1}},
			Asks: []etypes.PriceLevel{{Price: ask, VisibleQty: 1}},
		}},
	}
}

// A maker that has been filled holds no quote on that side. Waiting for the
// mid to move before replacing it leaves the book one-sided for as long as the
// market is calm, which is exactly when nothing will move the mid — measured
// on the second spot pair, two makers quoted twice in half an hour and then
// stopped.
func TestFixedDistanceMakerReplacesAFilledSideWithoutWaitingForAMove(t *testing.T) {
	gw := newMetaGateway()
	maker := NewFixedDistanceMaker(1, gw, FixedDistanceMakerConfig{
		Symbol: "CDF/USD", SpreadBps: 8, RequoteBps: 400, QuoteQty: 100, MaxInventory: 10_000,
		TickSize: 100, QuoteInterval: time.Second,
	})
	ctx := context.Background()
	maker.onTick(time.Unix(0, 0))
	maker.HandleEvent(ctx, makerSnapshot("CDF/USD", 299_900, 300_100))
	maker.onTick(time.Unix(0, int64(time.Second)))
	first := len(gw.orders())
	if first == 0 {
		t.Fatal("the maker never quoted")
	}

	// Acknowledge both quotes, then fill the bid completely.
	var bidOrderID uint64 = 10
	for i, req := range gw.orders() {
		maker.HandleEvent(ctx, &actor.Event{Type: actor.EventOrderAccepted,
			Data: actor.OrderAcceptedEvent{OrderID: bidOrderID + uint64(i), RequestID: req.RequestID}})
	}
	maker.HandleEvent(ctx, &actor.Event{Type: actor.EventOrderFilled,
		Data: actor.OrderFillEvent{OrderID: bidOrderID, Symbol: "CDF/USD", Side: exchange.Buy, Qty: 100, IsFull: true}})

	// The mid has not moved, so the requote gate alone would hold forever.
	maker.HandleEvent(ctx, makerSnapshot("CDF/USD", 299_900, 300_100))
	maker.onTick(time.Unix(0, int64(2*time.Second)))
	if len(gw.orders()) <= first {
		t.Error("the maker left the side it had been filled on empty while the mid was calm")
	}
}

// With both quotes live and the mid unchanged, the maker must not churn.
func TestFixedDistanceMakerStillHoldsWhenBothQuotesAreLive(t *testing.T) {
	gw := newMetaGateway()
	maker := NewFixedDistanceMaker(1, gw, FixedDistanceMakerConfig{
		Symbol: "CDF/USD", SpreadBps: 8, RequoteBps: 400, QuoteQty: 100, MaxInventory: 10_000,
		TickSize: 100, QuoteInterval: time.Second,
	})
	ctx := context.Background()
	maker.onTick(time.Unix(0, 0))
	maker.HandleEvent(ctx, makerSnapshot("CDF/USD", 299_900, 300_100))
	maker.onTick(time.Unix(0, int64(time.Second)))
	for i, req := range gw.orders() {
		maker.HandleEvent(ctx, &actor.Event{Type: actor.EventOrderAccepted,
			Data: actor.OrderAcceptedEvent{OrderID: 10 + uint64(i), RequestID: req.RequestID}})
	}
	quoted := len(gw.orders())
	maker.HandleEvent(ctx, makerSnapshot("CDF/USD", 299_900, 300_100))
	maker.onTick(time.Unix(0, int64(2*time.Second)))
	if len(gw.orders()) != quoted {
		t.Errorf("the maker requoted a live pair against an unchanged mid: %d then %d", quoted, len(gw.orders()))
	}
}

func TestFixedDistanceMakerUsesExplicitPostOnlyQuotes(t *testing.T) {
	gw := newMetaGateway()
	maker := NewFixedDistanceMaker(1, gw, FixedDistanceMakerConfig{
		Symbol: "CDF/USD", SpreadBps: 8, RequoteBps: 4, QuoteQty: 100, MaxInventory: 10_000,
		TickSize: 100, QuoteInterval: time.Second, PostOnly: true,
	})
	maker.onTick(time.Unix(0, 0))
	maker.HandleEvent(context.Background(), makerSnapshot("CDF/USD", 299_900, 300_100))
	maker.onTick(time.Unix(0, int64(time.Second)))
	orders := gw.orders()
	if len(orders) != 2 {
		t.Fatalf("post-only maker requests = %d, want 2", len(orders))
	}
	for _, request := range orders {
		if !request.PostOnly || request.Type != exchange.LimitOrder || request.TimeInForce != exchange.GTC {
			t.Fatalf("quote is not explicit post-only: %+v", request)
		}
	}
}

func TestFixedDistancePostOnlyCanCancelBeforeReplacement(t *testing.T) {
	gw := newMetaGateway()
	maker := NewFixedDistanceMaker(1, gw, FixedDistanceMakerConfig{
		Symbol: "CDF/USD", SpreadBps: 8, RequoteBps: 1, QuoteQty: 100, MaxInventory: 10_000,
		TickSize: 100, QuoteInterval: time.Second, PostOnly: true, PostOnlyCancelBeforeReplace: true,
	})
	ctx := context.Background()
	now := time.Unix(0, 0)
	maker.onTick(now) // subscribes
	maker.HandleEvent(ctx, makerSnapshot("CDF/USD", 299_900, 300_100))
	now = now.Add(time.Second)
	maker.onTick(now)
	orders := gw.orders()
	if len(orders) != 2 {
		t.Fatalf("initial orders = %d, want 2", len(orders))
	}
	for i, request := range orders {
		maker.HandleEvent(ctx, &actor.Event{Type: actor.EventOrderAccepted,
			Data: actor.OrderAcceptedEvent{OrderID: 10 + uint64(i), RequestID: request.RequestID}})
	}

	maker.HandleEvent(ctx, makerSnapshot("CDF/USD", 300_900, 301_100))
	now = now.Add(time.Second)
	maker.onTick(now)
	if len(gw.requests) != 7 || gw.requests[3].Type != etypes.ReqCancelOrder || gw.requests[4].Type != etypes.ReqCancelOrder || gw.requests[5].Type != etypes.ReqPlaceOrder || gw.requests[6].Type != etypes.ReqPlaceOrder {
		t.Fatalf("cancel-before-replace requests = %+v", gw.requests)
	}
	for _, request := range gw.requests[5:7] {
		if request.OrderReq == nil || !request.OrderReq.PostOnly {
			t.Fatalf("cancel-before-replace quote lost post-only admission: %+v", request)
		}
	}
}
