package simulation

import (
	"testing"

	"exchange_sim/exchange"
)

func TestDeterministicResponseReceiptObserverOnlyRecordsDeliveredMessages(t *testing.T) {
	gateway := blockedGateway(t)
	var received []uint64
	if err := gateway.SetDeterministicResponseReceiptObserver(func(response exchange.Response, receivedAt int64) {
		if receivedAt != 0 {
			t.Errorf("receipt time = %d, want simulated time zero", receivedAt)
		}
		received = append(received, response.RequestID)
	}); err != nil {
		t.Fatal(err)
	}
	gateway.responseCh <- exchange.Response{RequestID: 1}
	gateway.phaseResp = append(gateway.phaseResp, exchange.Response{RequestID: 2}, exchange.Response{RequestID: 3})
	if gateway.DrainDeterministicPhaseEgress() {
		t.Fatal("full actor inbox was reported as successful delivery")
	}
	if len(received) != 0 {
		t.Fatalf("recorded undelivered response: %v", received)
	}
	<-gateway.responseCh
	if !gateway.DrainDeterministicPhaseEgress() {
		t.Fatal("first response was not delivered")
	}
	if len(received) != 1 || received[0] != 2 {
		t.Fatalf("first receipt = %v", received)
	}
	<-gateway.responseCh
	if !gateway.DrainDeterministicPhaseEgress() {
		t.Fatal("second response was not delivered")
	}
	if len(received) != 2 || received[1] != 3 {
		t.Fatalf("receipts = %v", received)
	}
	if gateway.DrainDeterministicPhaseEgress() || len(received) != 2 {
		t.Fatalf("duplicate response receipt: %v", received)
	}
}

func TestResponseReceiptObserverRejectsNondeterministicGateway(t *testing.T) {
	gateway := NewDelayedGateway(nil, nil, nil, nil)
	if err := gateway.SetDeterministicResponseReceiptObserver(func(exchange.Response, int64) {}); err == nil {
		t.Fatal("accepted observer without a deterministic clock")
	}
}

func blockedGateway(t *testing.T) *DelayedGateway {
	t.Helper()
	gateway := &DelayedGateway{
		responseCh:   make(chan exchange.Response, 1),
		marketDataCh: make(chan *exchange.MarketDataMsg, 1),
	}
	gateway.running.Store(true)
	gateway.phaseMode.Store(true)
	gateway.scheduler = NewEventScheduler(NewSimulatedClock(0))
	gateway.clock = NewSimulatedClock(0)
	return gateway
}

// A gateway whose actor inbox is full is a slow consumer, not a deadlock: the
// messages are delivered as soon as the actor drains. Reporting that as a
// stall kills a run whose only fault is a participant on a slow link.
func TestEgressBlockedDistinguishesAFullInboxFromAStall(t *testing.T) {
	gateway := blockedGateway(t)
	if gateway.EgressBlocked() {
		t.Error("an empty gateway reported backpressure")
	}

	gateway.phaseResp = append(gateway.phaseResp, exchange.Response{})
	if gateway.EgressBlocked() {
		t.Error("a gateway with room in the inbox reported backpressure")
	}

	gateway.responseCh <- exchange.Response{}
	if !gateway.EgressBlocked() {
		t.Error("a gateway holding a response against a full inbox did not report backpressure")
	}

	<-gateway.responseCh
	if gateway.EgressBlocked() {
		t.Error("backpressure persisted after the inbox drained")
	}
}

// The market-data channel has to count too: most of what a delayed gateway
// carries is book publications, not responses.
func TestEgressBlockedCountsMarketData(t *testing.T) {
	gateway := blockedGateway(t)
	gateway.phaseMD = append(gateway.phaseMD, phaseMarketData{message: &exchange.MarketDataMsg{}})
	gateway.marketDataCh <- &exchange.MarketDataMsg{}
	if !gateway.EgressBlocked() {
		t.Error("a full market-data inbox did not report backpressure")
	}
}

// A gateway that is not running, or not in deterministic phases, has no phase
// queues to report on and must never claim backpressure.
func TestEgressBlockedIsFalseOutsideDeterministicPhases(t *testing.T) {
	gateway := blockedGateway(t)
	gateway.phaseResp = append(gateway.phaseResp, exchange.Response{})
	gateway.responseCh <- exchange.Response{}
	gateway.phaseMode.Store(false)
	if gateway.EgressBlocked() {
		t.Error("a gateway outside deterministic phases reported backpressure")
	}
	gateway.phaseMode.Store(true)
	gateway.running.Store(false)
	if gateway.EgressBlocked() {
		t.Error("a stopped gateway reported backpressure")
	}
}
