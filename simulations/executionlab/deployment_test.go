package executionlab

import (
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/simulation"
)

func TestSnapshotProcessingIsDelayedOrderedAndOneSidedDoesNotEraseQuote(t *testing.T) {
	agent, err := newExecutionAgent(13, exchange.NewClientGateway(13), ParentOrderConfig{
		Symbol: "ABC/USD", Side: exchange.Buy, TargetQty: 1, BasePrecision: 1,
		QuoteAsset: "USD", Policy: Immediate, DecisionAfter: 2 * time.Second,
		PollInterval: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	agent.processingDelay = 120 * time.Millisecond
	var now int64
	agent.observationTime = func() int64 { return now }
	var completions []SnapshotProcessingComplete
	agent.observe = func(event EvidenceObservation) {
		if event.Name == "snapshot_processing_complete" {
			completions = append(completions, event.Payload.(SnapshotProcessingComplete))
		}
	}
	deliver := func(at int64, sequence uint64, ask int64) {
		now = at
		asks := []exchange.PriceLevel{}
		if ask != 0 {
			asks = append(asks, exchange.PriceLevel{Price: ask, VisibleQty: 1})
		}
		agent.HandleEvent(nil, &actor.Event{Type: actor.EventBookSnapshot, Data: actor.BookSnapshotEvent{
			Symbol: "ABC/USD", SeqNum: sequence,
			Snapshot: &exchange.BookSnapshot{Bids: []exchange.PriceLevel{{Price: 99, VisibleQty: 1}}, Asks: asks},
		}})
	}
	deliver(100_000_000, 1, 101)
	deliver(110_000_000, 2, 102)
	deliver(120_000_000, 3, 0)
	for _, step := range []int64{219_000_000, 220_000_000, 229_000_000, 230_000_000, 240_000_000} {
		agent.onTick(time.Unix(0, step))
		if step == 219_000_000 && agent.bestAsk != 0 || step == 220_000_000 && agent.bestAsk != 101 ||
			step == 229_000_000 && agent.bestAsk != 101 || step >= 230_000_000 && agent.bestAsk != 102 {
			t.Fatalf("processed ask at %d = %d", step, agent.bestAsk)
		}
	}
	if len(completions) != 3 || completions[0].SeqNum != 1 || completions[0].ProcessedAt != 220_000_000 ||
		completions[1].SeqNum != 2 || completions[1].ProcessedAt != 230_000_000 ||
		completions[2].SeqNum != 3 || completions[2].TwoSided || completions[2].ProcessedAt != 240_000_000 {
		t.Fatalf("processing completion evidence = %#v", completions)
	}
}

func TestSlowDirectedRequestCannotFillWithdrawnDisplayedAsk(t *testing.T) {
	clock := simulation.NewSimulatedClock(0)
	scheduler := simulation.NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	venue := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{Clock: clock, DeterministicIngress: true, DeterministicPhases: true})
	venue.AddInstrument(exchange.NewSpotInstrument("ABC/USD", "ABC", "USD", 1, 1, 1, 1))
	venue.ConnectNewClient(1, map[string]int64{"ABC": 10, "USD": 1000}, &exchange.FixedFee{})
	ask := venue.PlaceOrder(1, &exchange.OrderRequest{RequestID: 1, Symbol: "ABC/USD", Side: exchange.Sell,
		Type: exchange.LimitOrder, Price: 101, Qty: 1, TimeInForce: exchange.GTC, Visibility: exchange.Normal})
	if !ask.Success {
		t.Fatalf("fixture ask rejected: %+v", ask)
	}
	if bid := venue.PlaceOrder(1, &exchange.OrderRequest{RequestID: 2, Symbol: "ABC/USD", Side: exchange.Buy,
		Type: exchange.LimitOrder, Price: 99, Qty: 1, TimeInForce: exchange.GTC, Visibility: exchange.Normal}); !bid.Success {
		t.Fatalf("fixture bid rejected: %+v", bid)
	}
	link := newDirectedLatencyMount(venue, scheduler, clock, ParentDeployment{
		MarketDataLatency: 90 * time.Millisecond, RequestLatency: 90 * time.Millisecond, ResponseLatency: 90 * time.Millisecond})
	buyer := link.ConnectNewClient(13, map[string]int64{"ABC": 0, "USD": 1000}, &exchange.FixedFee{})
	defer func() {
		if stoppable, ok := buyer.(interface{ Stop() }); ok {
			stoppable.Stop()
		}
	}()
	clock.Advance(time.Second)
	if _, _, ok := venue.TwoSidedTopOfBook("ABC/USD"); !ok {
		t.Fatal("displayed two-sided quote absent at send")
	}
	buyer.Send(exchange.Request{Type: exchange.ReqPlaceOrder, OrderReq: &exchange.OrderRequest{
		RequestID: 3, Symbol: "ABC/USD", Side: exchange.Buy, Type: exchange.Market,
		Qty: 1, TimeInForce: exchange.GTC, Visibility: exchange.Normal}})
	clock.Advance(50 * time.Millisecond)
	if cancel := venue.CancelOrder(1, &exchange.CancelRequest{RequestID: 4, OrderID: ask.Data.(uint64)}); !cancel.Success {
		t.Fatalf("fixture ask cancellation failed: %+v", cancel)
	}
	clock.Advance(39 * time.Millisecond)
	select {
	case <-venue.Gateways[13].RequestCh:
		t.Fatal("slow request arrived before configured 90ms")
	default:
	}
	clock.Advance(time.Millisecond)
	var delivered exchange.Request
	select {
	case delivered = <-venue.Gateways[13].RequestCh:
	default:
		t.Fatal("slow request did not arrive at 90ms")
	}
	if clock.NowUnixNano() != int64(time.Second+90*time.Millisecond) {
		t.Fatal("wrong logical arrival timestamp")
	}
	if response := venue.PlaceOrder(13, delivered.OrderReq); !response.Success {
		t.Fatalf("empty-book market request unexpectedly rejected: %+v", response)
	}
	if venue.Clients[13].Balances["ABC"] != 0 {
		t.Fatal("slow request filled a quote withdrawn before venue arrival")
	}
}
