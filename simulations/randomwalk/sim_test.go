package randomwalk

import (
	"context"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	etypes "exchange_sim/types"
)

type randomwalkStubGateway struct {
	requests []etypes.Request
	respCh   chan etypes.Response
	mdCh     chan *etypes.MarketDataMsg
}

func newRandomwalkStubGateway() *randomwalkStubGateway {
	return &randomwalkStubGateway{
		respCh: make(chan etypes.Response, 16),
		mdCh:   make(chan *etypes.MarketDataMsg, 16),
	}
}

func (g *randomwalkStubGateway) ID() uint64                                 { return 1 }
func (g *randomwalkStubGateway) Send(req etypes.Request)                    { g.requests = append(g.requests, req) }
func (g *randomwalkStubGateway) Responses() <-chan etypes.Response          { return g.respCh }
func (g *randomwalkStubGateway) MarketDataCh() <-chan *etypes.MarketDataMsg { return g.mdCh }
func (g *randomwalkStubGateway) IsRunning() bool                            { return true }

func (g *randomwalkStubGateway) placedOrders() []*etypes.OrderRequest {
	var orders []*etypes.OrderRequest
	for _, request := range g.requests {
		if request.Type == etypes.ReqPlaceOrder {
			orders = append(orders, request.OrderReq)
		}
	}
	return orders
}

func orderForSide(t *testing.T, orders []*etypes.OrderRequest, side exchange.Side) *etypes.OrderRequest {
	t.Helper()
	for _, order := range orders {
		if order.Side == side {
			return order
		}
	}
	t.Fatalf("missing %s order", side)
	return nil
}

func TestMarketMakerDepthIsNotionalNormalized(t *testing.T) {
	for _, asset := range assets {
		qty := quantityForUSDNotional(marketMakerLevelNotional, asset.price)
		notional, ok := etypes.TryMulDiv(qty, asset.price, btcPrecision)
		if !ok {
			t.Fatalf("%s: level notional overflow", asset.name)
		}
		tolerance := asset.price / btcPrecision
		if tolerance < 1 {
			tolerance = 1
		}
		if diff := notional - marketMakerLevelNotional; diff < -tolerance || diff > tolerance {
			t.Fatalf("%s: level notional = %d, want approximately %d", asset.name, notional, marketMakerLevelNotional)
		}
	}
}

func TestRandomWalkMaintainsTwoSidedQuotes(t *testing.T) {
	sim, err := NewSimWithConfig(10*time.Second, SimConfig{LogDir: t.TempDir(), SnapshotOnly: true})
	if err != nil {
		t.Fatalf("NewSimWithConfig: %v", err)
	}
	defer sim.Close()

	ctx := context.Background()
	sim.Exchange().StartAutomation(ctx)
	defer sim.Exchange().StopAutomation()
	var emptyAt []time.Duration
	sim.Runner.SetProgressCallback(1_000, func(done, _ int) {
		for _, mm := range sim.MMs {
			for _, symbol := range mm.Symbols() {
				bidQty, askQty := sim.Exchange().GetBestLiquidity(symbol)
				if bidQty == 0 || askQty == 0 {
					emptyAt = append(emptyAt, time.Duration(done)*time.Millisecond)
					return
				}
			}
		}
	})
	if err := sim.Runner.Run(ctx); err != nil {
		t.Fatalf("Runner.Run: %v", err)
	}
	if len(emptyAt) > 0 {
		t.Fatalf("one or more quiescent snapshots had an empty book: %v", emptyAt)
	}

	for _, mm := range sim.MMs {
		for _, symbol := range mm.Symbols() {
			bidQty, askQty := sim.Exchange().GetBestLiquidity(symbol)
			if bidQty == 0 || askQty == 0 {
				var pending, requests int
				for ref, orderIDs := range mm.pending {
					if ref.symbol == symbol {
						pending += len(orderIDs)
					}
				}
				for _, ref := range mm.reqToQuote {
					if ref.symbol == symbol {
						requests++
					}
				}
				t.Errorf("%s has empty final book: bid=%d ask=%d pending=%d requests=%d", symbol, bidQty, askQty, pending, requests)
			}
		}
	}
}

func TestTriArbRunsOncePerCoherentSnapshotTimestamp(t *testing.T) {
	gw := exchange.NewClientGateway(12)
	arb := NewTriArbActor(12, gw, TriArbConfig{
		CrossSymbol:    "DEF-ABC",
		BaseUSDSymbol:  "DEF-USD",
		QuoteUSDSymbol: "ABC-USD",
		TargetNotional: 1_000,
		MinProfitBps:   1,
		BasePrecision:  1,
		CheckInterval:  100 * time.Millisecond,
	})

	// DEF is materially cheaper through ABC, so each coherent snapshot would
	// otherwise trigger an execution.
	updateBooks := func(timestamp int64, symbols ...string) {
		t.Helper()
		for _, symbol := range symbols {
			bid, ask := int64(0), int64(0)
			switch symbol {
			case "DEF-ABC":
				bid, ask = 9, 10
			case "DEF-USD":
				bid, ask = 250, 251
			case "ABC-USD":
				bid, ask = 20, 21
			default:
				t.Fatalf("unknown symbol %q", symbol)
			}
			arb.onSnapshot(actor.BookSnapshotEvent{
				Symbol:    symbol,
				Timestamp: timestamp,
				Snapshot: &exchange.BookSnapshot{
					Bids: []exchange.PriceLevel{{Price: bid, VisibleQty: 1_000}},
					Asks: []exchange.PriceLevel{{Price: ask, VisibleQty: 1_000}},
				},
			})
		}
	}

	nextRequest := func() exchange.Request {
		t.Helper()
		select {
		case req := <-gw.RequestCh:
			return req
		default:
			t.Fatal("expected triangular-arbitrage order request")
			return exchange.Request{}
		}
	}
	completeCycle := func(first exchange.Request) {
		t.Helper()
		requests := [3]exchange.Request{first}
		for leg := range requests {
			orderID := uint64(100 + leg)
			arb.onAccepted(actor.OrderAcceptedEvent{
				RequestID: requests[leg].OrderReq.RequestID,
				OrderID:   orderID,
			})
			arb.onFilled(actor.OrderFillEvent{OrderID: orderID, IsFull: true})
			if leg < len(requests)-1 {
				requests[leg+1] = nextRequest()
			}
		}
		if arb.executing {
			t.Fatal("triangular-arbitrage cycle did not return to idle")
		}
	}

	allSymbols := []string{"DEF-ABC", "DEF-USD", "ABC-USD"}
	updateBooks(1_000, allSymbols...)
	arb.onTick(time.Unix(0, 0))
	completeCycle(nextRequest())

	// The actor is idle again, but unchanged cached books are stale: no repeat
	// execution may be launched by later 100 ms checks.
	for i := 0; i < 5; i++ {
		arb.onTick(time.Unix(0, int64(i+1)*int64(100*time.Millisecond)))
	}
	select {
	case req := <-gw.RequestCh:
		t.Fatalf("stale snapshot submitted duplicate cycle: %+v", req)
	default:
	}

	// Two refreshed legs are insufficient; a new execution can start only when
	// all three observations carry the same new timestamp.
	updateBooks(2_000, "DEF-ABC", "DEF-USD")
	arb.onTick(time.Unix(0, 600*int64(time.Millisecond)))
	select {
	case req := <-gw.RequestCh:
		t.Fatalf("incoherent snapshots submitted cycle: %+v", req)
	default:
	}
	updateBooks(2_000, "ABC-USD")
	arb.onTick(time.Unix(0, 700*int64(time.Millisecond)))
	completeCycle(nextRequest())
}

func TestMarketMakerRequotesMissingSideWithoutReplacingLiveQuote(t *testing.T) {
	gw := newRandomwalkStubGateway()
	mm := NewMarketMaker(1, gw, MMConfig{
		Symbols: []string{"ABC-USD"}, BootstrapPrice: 100,
		Levels: 1, LevelSpacing: 1, LevelSize: 1, TickSize: 1,
		RefreshInterval: time.Hour,
	})
	const symbol = "ABC-USD"
	mm.quote(symbol)
	orders := gw.placedOrders()
	if len(orders) != 2 {
		t.Fatalf("initial orders = %d, want 2", len(orders))
	}
	bid := orderForSide(t, orders, exchange.Buy)
	ask := orderForSide(t, orders, exchange.Sell)

	mm.onAccepted(actor.OrderAcceptedEvent{OrderID: 11, RequestID: bid.RequestID})
	mm.onRejected(actor.OrderRejectedEvent{RequestID: ask.RequestID, Reason: exchange.RejectSelfTrade})
	mm.ensureQuoted(symbol)

	orders = gw.placedOrders()
	if len(orders) != 3 {
		t.Fatalf("one missing ask must produce one replacement, got %d total orders", len(orders))
	}
	if got := orders[2].Side; got != exchange.Sell {
		t.Fatalf("replacement side = %s, want sell", got)
	}
	mm.ensureQuoted(symbol)
	if got := len(gw.placedOrders()); got != 3 {
		t.Fatalf("side-aware in-flight tracking stacked orders: got %d, want 3", got)
	}
}

func TestMarketMakerKeepsInsufficientSideWithdrawnUntilOppositeFill(t *testing.T) {
	gw := newRandomwalkStubGateway()
	mm := NewMarketMaker(1, gw, MMConfig{
		Symbols: []string{"ABC-USD"}, BootstrapPrice: 100,
		Levels: 1, LevelSpacing: 1, LevelSize: 1, TickSize: 1,
		RefreshInterval: time.Hour,
	})
	const symbol = "ABC-USD"
	mm.quote(symbol)
	orders := gw.placedOrders()
	bid := orderForSide(t, orders, exchange.Buy)
	ask := orderForSide(t, orders, exchange.Sell)

	mm.onAccepted(actor.OrderAcceptedEvent{OrderID: 11, RequestID: bid.RequestID})
	mm.onRejected(actor.OrderRejectedEvent{RequestID: ask.RequestID, Reason: exchange.RejectInsufficientBalance})
	mm.ensureQuoted(symbol)
	mm.ensureQuoted(symbol)
	if got := len(gw.placedOrders()); got != 2 {
		t.Fatalf("insufficient sell inventory must withdraw ask without retry spam, got %d orders", got)
	}

	mm.onFilled(actor.OrderFillEvent{OrderID: 11, Price: 100, IsFull: true})
	orders = gw.placedOrders()
	if len(orders) != 4 {
		t.Fatalf("buy fill must re-enable the withdrawn ask, got %d total orders", len(orders))
	}
	if got := orders[3].Side; got != exchange.Sell {
		t.Fatalf("re-enabled side = %s, want sell", got)
	}
}

func TestCrossPairMMRequotesMissingSideWithoutReplacingLiveQuote(t *testing.T) {
	gw := newRandomwalkStubGateway()
	mm := NewCrossPairMM(1, gw, CrossPairMMConfig{
		CrossSymbols:   []string{"DEF-ABC"},
		BaseUSDSymbols: []string{"DEF-USD"},
		QuoteUSDSymbol: "ABC-USD", QuotePrecision: 1,
		TickSizes:  map[string]int64{"DEF-ABC": 1},
		LevelSizes: map[string]int64{"DEF-ABC": 1},
		Levels:     1, LevelSpacing: 1, RefreshInterval: time.Hour,
	})
	const symbol = "DEF-ABC"
	mm.usdMids["ABC-USD"] = 100
	mm.usdMids["DEF-USD"] = 200
	mm.recomputeMids()
	mm.quote(symbol)
	orders := gw.placedOrders()
	bid := orderForSide(t, orders, exchange.Buy)
	ask := orderForSide(t, orders, exchange.Sell)

	mm.onAccepted(actor.OrderAcceptedEvent{OrderID: 11, RequestID: bid.RequestID})
	mm.onRejected(actor.OrderRejectedEvent{RequestID: ask.RequestID, Reason: exchange.RejectSelfTrade})
	mm.onTick(time.Now())

	orders = gw.placedOrders()
	if len(orders) != 3 {
		t.Fatalf("one missing cross ask must produce one replacement, got %d total orders", len(orders))
	}
	if got := orders[2].Side; got != exchange.Sell {
		t.Fatalf("replacement side = %s, want sell", got)
	}
	mm.onTick(time.Now())
	if got := len(gw.placedOrders()); got != 3 {
		t.Fatalf("cross maker stacked orders after side replacement: got %d, want 3", got)
	}
}

func TestCrossPairMMKeepsInsufficientSideWithdrawnUntilOppositeFill(t *testing.T) {
	gw := newRandomwalkStubGateway()
	mm := NewCrossPairMM(1, gw, CrossPairMMConfig{
		CrossSymbols:   []string{"DEF-ABC"},
		BaseUSDSymbols: []string{"DEF-USD"},
		QuoteUSDSymbol: "ABC-USD", QuotePrecision: 1,
		TickSizes:  map[string]int64{"DEF-ABC": 1},
		LevelSizes: map[string]int64{"DEF-ABC": 1},
		Levels:     1, LevelSpacing: 1, RefreshInterval: time.Hour,
	})
	const symbol = "DEF-ABC"
	mm.usdMids["ABC-USD"] = 100
	mm.usdMids["DEF-USD"] = 200
	mm.recomputeMids()
	mm.quote(symbol)
	orders := gw.placedOrders()
	bid := orderForSide(t, orders, exchange.Buy)
	ask := orderForSide(t, orders, exchange.Sell)

	mm.onAccepted(actor.OrderAcceptedEvent{OrderID: 11, RequestID: bid.RequestID})
	mm.onRejected(actor.OrderRejectedEvent{RequestID: ask.RequestID, Reason: exchange.RejectInsufficientBalance})
	mm.onTick(time.Now())
	mm.onTick(time.Now())
	if got := len(gw.placedOrders()); got != 2 {
		t.Fatalf("insufficient cross inventory must withdraw ask without retry spam, got %d orders", got)
	}

	mm.onFilled(actor.OrderFillEvent{OrderID: 11, IsFull: true})
	orders = gw.placedOrders()
	if len(orders) != 4 {
		t.Fatalf("cross buy fill must re-enable withdrawn ask, got %d total orders", len(orders))
	}
	if got := orders[3].Side; got != exchange.Sell {
		t.Fatalf("re-enabled side = %s, want sell", got)
	}
}
