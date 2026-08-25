package derivsim

import (
	"context"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	etypes "exchange_sim/types"
)

func listPut(taker *OptionLiabilityTaker, symbol string, strike, expiry int64) {
	taker.HandleEvent(context.Background(), &actor.Event{
		Type: actor.EventInstrument,
		Data: actor.InstrumentEvent{Announcement: &etypes.InstrumentAnnouncement{
			Action: "listed", Symbol: symbol, InstrumentType: "OPTION", Underlying: "ABC/USD",
			Strike: strike, IsCall: false, ExpiryNano: expiry,
		}},
	})
}

func TestOptionLiabilityUserBuysOnlyObservedPutAsk(t *testing.T) {
	gw := newStubGateway()
	base := int64(100_000_000)
	decisions := make([]OptionLiabilityDecision, 0)
	u := NewOptionLiabilityTaker(7, gw, OptionLiabilityTakerConfig{
		Underlying: "ABC/USD", TargetQty: 2 * base, LotQty: base,
		TargetStrikeBps: 9_500, MaxPremium: 1_000,
		Interval: time.Second, BasePrecision: base,
		DecisionObserver: func(d OptionLiabilityDecision) { decisions = append(decisions, d) },
	})
	u.onTick(time.Unix(0, int64(time.Second))) // subscribe only
	listPut(u, "ABC-P-47500", 47_500*base, int64(30*time.Second))
	u.HandleEvent(context.Background(), snapshotEvent("ABC/USD", 50_000*base-100, 50_000*base+100))
	u.HandleEvent(context.Background(), snapshotEvent("ABC-P-47500", 400, 500))
	u.onTick(time.Unix(0, int64(2*time.Second)))
	orders := gw.placedOrders()
	if len(orders) != 1 {
		t.Fatalf("placed orders = %d, want one: %+v", len(orders), orders)
	}
	order := orders[0]
	if order.Symbol != "ABC-P-47500" || order.Side != exchange.Buy || order.Price != 500 || order.Qty != base || order.Type != exchange.LimitOrder || order.TimeInForce != exchange.IOC {
		t.Fatalf("order = %+v, want observed put ask IOC buy", order)
	}
	if len(decisions) < 2 || decisions[len(decisions)-1].Action != "SUBMIT_PUT_IOC" || decisions[len(decisions)-1].SideEvidence != "BUY" {
		t.Fatalf("decisions = %+v, want a linked buy decision", decisions)
	}

	reqID := order.RequestID
	u.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{RequestID: reqID, OrderID: 91}})
	u.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderFilled, Data: actor.OrderFillEvent{
		OrderID: 91, Symbol: order.Symbol, Side: exchange.Buy, Qty: base, Price: 500, IsFull: true,
	}})
	if got := u.Position(); got != base {
		t.Fatalf("position = %d, want %d after canonical fill", got, base)
	}
}

func TestOptionLiabilityUserDefersWithoutUnderlyingOrAsk(t *testing.T) {
	gw := newStubGateway()
	base := int64(100_000_000)
	decisions := make([]OptionLiabilityDecision, 0)
	u := NewOptionLiabilityTaker(8, gw, OptionLiabilityTakerConfig{
		Underlying: "ABC/USD", TargetQty: base, LotQty: base,
		TargetStrikeBps: 9_500, MaxPremium: 1_000,
		Interval: time.Second, BasePrecision: base,
		DecisionObserver: func(d OptionLiabilityDecision) { decisions = append(decisions, d) },
	})
	u.onTick(time.Unix(0, int64(time.Second)))
	listPut(u, "ABC-P-47500", 47_500*base, int64(30*time.Second))
	u.onTick(time.Unix(0, int64(2*time.Second)))
	if len(gw.placedOrders()) != 0 {
		t.Fatal("submitted without a delivered underlying observation")
	}
	if got := decisions[len(decisions)-1].Reason; got != "LOCAL_UNDERLYING_UNAVAILABLE" {
		t.Fatalf("reason = %q, want explicit unavailable reason", got)
	}
	u.HandleEvent(context.Background(), snapshotEvent("ABC/USD", 50_000*base-100, 50_000*base+100))
	u.onTick(time.Unix(0, int64(3*time.Second)))
	if len(gw.placedOrders()) != 0 {
		t.Fatal("submitted without an option ask observation")
	}
	if got := decisions[len(decisions)-1].Reason; got != "OPTION_ASK_UNAVAILABLE" {
		t.Fatalf("reason = %q, want explicit ask-unavailable reason", got)
	}
}

func TestOptionLiabilityUserRejectsInvalidPolicy(t *testing.T) {
	base := int64(100_000_000)
	if err := (OptionLiabilityTakerConfig{Underlying: "ABC/USD", TargetQty: base, LotQty: base, TargetStrikeBps: 10_000, MaxPremium: 1, Interval: time.Second, BasePrecision: base}).Validate(); err == nil {
		t.Fatal("accepted target strike at 10000 bps")
	}
}
