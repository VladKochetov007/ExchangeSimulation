package executionlab

import (
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
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
