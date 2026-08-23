package multivenue

import (
	"testing"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/simulation"
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

// The router may compare three delayed venue feeds only after every declared
// venue has delivered a real prefix. This distinguishes an absent third feed
// from a knowingly observed third market with no opportunity, and gives the
// V2 frontier recorder an exact full information set for both legs.
func TestInstrumentedCrossVenueRouterRequiresFullDeliveredFrontier(t *testing.T) {
	var decisions []CrossVenueArbDecision
	frontiers := []simulation.MarketDataFrontier{
		{LinkID: 11, Ordinal: 1, DeliveredAt: 100, Digest: [16]byte{1}},
		{LinkID: 12, Ordinal: 1, DeliveredAt: 100, Digest: [16]byte{2}},
		{},
	}
	legs := make([]CrossVenueArbLegConfig, 0, 3)
	for index, venueID := range []string{"alpha", "bravo", "charlie"} {
		gateway := &routerFrontierGateway{ClientGateway: exchange.NewClientGateway(1), frontier: &frontiers[index]}
		legs = append(legs, CrossVenueArbLegConfig{VenueID: venueID, ClientID: 1, ActorID: uint64(index + 1), Gateway: gateway})
	}
	router, err := NewCrossVenueArb(1, CrossVenueArbConfig{
		Symbol: "ABC/USD", LotQty: 1, BasePrecision: 1, MaxAttempts: 1,
		RequireCompleteFeedFrontier: true,
		DecisionObserver:            func(decision CrossVenueArbDecision) { decisions = append(decisions, decision) },
	}, legs)
	if err != nil {
		t.Fatalf("NewCrossVenueArb: %v", err)
	}
	setRouterBook(router.legs[0], 100, 101)
	setRouterBook(router.legs[1], 105, 106)
	setRouterBook(router.legs[2], 99, 110)

	router.onQuote(router.legs[1])
	if len(router.groups) != 0 || len(decisions) != 0 {
		t.Fatalf("router traded before all declared feeds delivered: groups=%d decisions=%d", len(router.groups), len(decisions))
	}
	frontiers[2] = simulation.MarketDataFrontier{LinkID: 13, Ordinal: 1, DeliveredAt: 100, Digest: [16]byte{3}}
	router.onQuote(router.legs[1])
	if len(router.groups) != 1 || len(decisions) != 2 {
		t.Fatalf("router after full frontier = groups=%d decisions=%d, want 1/2", len(router.groups), len(decisions))
	}
	for _, decision := range decisions {
		if decision.TradingLinkID == 0 || len(decision.Components) != 3 {
			t.Fatalf("incomplete router decision frontier: %#v", decision)
		}
		for _, component := range decision.Components {
			if component.Frontier.Ordinal == 0 || component.Frontier.DeliveredAt > 100 || component.Frontier.Digest == ([16]byte{}) {
				t.Fatalf("invalid delayed router component: %#v", component)
			}
		}
		wantLink := uint32(12)
		if decision.Request.OrderReq.Side == exchange.Buy {
			wantLink = 11
		}
		if decision.TradingLinkID != wantLink {
			t.Fatalf("router side %s bound to link %d, want %d", decision.Request.OrderReq.Side, decision.TradingLinkID, wantLink)
		}
	}
}

type routerFrontierGateway struct {
	*exchange.ClientGateway
	frontier *simulation.MarketDataFrontier
}

func (g *routerFrontierGateway) MarketDataFrontier() simulation.MarketDataFrontier {
	return *g.frontier
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
