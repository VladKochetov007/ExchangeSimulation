package multivenue

import (
	"testing"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

func TestCrossVenueRouterRequiresAllInExecutableEdge(t *testing.T) {
	router := testCrossVenueRouter(t, 100) // 1% fee per leg in integer units.
	setRouterBook(router.legs[0], 100, 101)
	setRouterBook(router.legs[1], 102, 103)
	setRouterBook(router.legs[2], 98, 110)

	// Midpoints at venue 1 and 2 differ, but selling at 102 then paying a
	// one-unit fee cannot fund buying at 101 plus its fee.
	if _, _, edge, ok := router.bestOpportunity(); ok || edge != 0 {
		t.Fatalf("midpoint-only edge selected: edge=%d ok=%v", edge, ok)
	}

	setRouterBook(router.legs[1], 105, 106)
	buy, sell, edge, ok := router.bestOpportunity()
	if !ok || buy.venueID != "alpha" || sell.venueID != "bravo" || edge != 2 {
		t.Fatalf("best all-in opportunity = buy=%v sell=%v edge=%d ok=%v", venueID(buy), venueID(sell), edge, ok)
	}
}

func TestCrossVenueRouterReportsOneLegFailureAsResidual(t *testing.T) {
	router := testCrossVenueRouter(t, 0)
	setRouterBook(router.legs[0], 100, 101)
	setRouterBook(router.legs[1], 105, 106)

	buy, sell, edge, ok := router.bestOpportunity()
	if !ok {
		t.Fatal("missing positive all-in opportunity")
	}
	router.openGroup(buy, sell, edge)
	if len(router.groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(router.groups))
	}
	group := router.groups[0]
	assertFOKRequest(t, buy.Gateway(), group.buy.RequestID, exchange.Buy)
	assertFOKRequest(t, sell.Gateway(), group.sell.RequestID, exchange.Sell)

	router.onAccepted(buy, actor.OrderAcceptedEvent{RequestID: group.buy.RequestID, OrderID: 11})
	router.onFill(buy, actor.OrderFillEvent{OrderID: 11, Qty: 1, Price: 101, Side: exchange.Buy, IsFull: true})
	router.onRejected(sell, actor.OrderRejectedEvent{RequestID: group.sell.RequestID, Reason: exchange.RejectInsufficientBalance})

	report := router.Report()
	if report.CompletedGroups != 0 || report.FailedGroups != 1 || report.PendingGroups != 0 || report.ResidualBaseQty != 1 {
		t.Fatalf("non-atomic route report = %#v", report)
	}
	if len(report.Groups) != 1 || !report.Groups[0].Failed || report.Groups[0].Complete || report.Groups[0].Sell.RejectReason != exchange.RejectInsufficientBalance {
		t.Fatalf("group detail = %#v", report.Groups)
	}
}

func TestCrossVenueRouterRequiresDisplayedLotAtEachTouch(t *testing.T) {
	router := testCrossVenueRouter(t, 0)
	router.cfg.LotQty = 2
	setRouterBookQty(router.legs[0], 100, 1, 101, 1)
	setRouterBookQty(router.legs[1], 105, 1, 106, 1)
	setRouterBookQty(router.legs[2], 99, 2, 110, 2)

	if _, _, edge, ok := router.bestOpportunity(); ok || edge != 0 {
		t.Fatalf("under-depth touch selected: edge=%d ok=%v", edge, ok)
	}
	setRouterBookQty(router.legs[1], 105, 2, 106, 2)
	setRouterBookQty(router.legs[0], 100, 2, 101, 2)
	if buy, sell, edge, ok := router.bestOpportunity(); !ok || buy.venueID != "alpha" || sell.venueID != "bravo" || edge != 8 {
		t.Fatalf("full-depth opportunity = buy=%v sell=%v edge=%d ok=%v", venueID(buy), venueID(sell), edge, ok)
	}
}

func testCrossVenueRouter(t *testing.T, feeBps int64) *CrossVenueArb {
	t.Helper()
	legs := make([]CrossVenueArbLegConfig, 0, 3)
	for index, venueID := range []string{"alpha", "bravo", "charlie"} {
		clientID := uint64(index + 1)
		legs = append(legs, CrossVenueArbLegConfig{
			VenueID: venueID, ClientID: clientID, ActorID: clientID,
			Gateway: exchange.NewClientGateway(clientID),
		})
	}
	router, err := NewCrossVenueArb(1, CrossVenueArbConfig{
		Symbol: "ABC/USD", LotQty: 1, BasePrecision: 1, TakerFeeBps: feeBps, MaxAttempts: 1,
	}, legs)
	if err != nil {
		t.Fatalf("NewCrossVenueArb: %v", err)
	}
	return router
}

func setRouterBook(leg *crossVenueArbLeg, bid, ask int64) {
	setRouterBookQty(leg, bid, 1, ask, 1)
}

func setRouterBookQty(leg *crossVenueArbLeg, bid, bidQty, ask, askQty int64) {
	leg.book.reset(&exchange.BookSnapshot{
		Bids: []exchange.PriceLevel{{Price: bid, VisibleQty: bidQty}},
		Asks: []exchange.PriceLevel{{Price: ask, VisibleQty: askQty}},
	})
}

func assertFOKRequest(t *testing.T, gateway actor.Gateway, requestID uint64, side exchange.Side) {
	t.Helper()
	client, ok := gateway.(*exchange.ClientGateway)
	if !ok {
		t.Fatalf("gateway type = %T", gateway)
	}
	request := <-client.RequestCh
	if request.OrderReq == nil || request.OrderReq.RequestID != requestID || request.OrderReq.Side != side || request.OrderReq.TimeInForce != exchange.FOK {
		t.Fatalf("route request = %#v, want FOK %s request %d", request, side, requestID)
	}
}

func venueID(leg *crossVenueArbLeg) string {
	if leg == nil {
		return ""
	}
	return leg.venueID
}
