package marketdata

import (
	"testing"

	etypes "exchange_sim/types"
)

type testSubscriber struct {
	channel chan *etypes.MarketDataMsg
	running bool
}

func (s *testSubscriber) MarketDataChan() chan *etypes.MarketDataMsg { return s.channel }
func (s *testSubscriber) IsRunning() bool                            { return s.running }

func TestPublishReturnsZeroWithoutSubscribersAndSequencesDeliveredMessages(t *testing.T) {
	publisher := NewMDPublisher()
	message := &etypes.BookSnapshot{Bids: []etypes.PriceLevel{{Price: 101, VisibleQty: 2}}}
	if sequence := publisher.Publish("CDF/USD", etypes.MDSnapshot, message, 1); sequence != 0 {
		t.Fatalf("unsubscribed publish sequence = %d, want 0", sequence)
	}

	subscriber := &testSubscriber{channel: make(chan *etypes.MarketDataMsg, 2), running: true}
	publisher.Subscribe(7, "CDF/USD", []etypes.MDType{etypes.MDSnapshot}, subscriber)
	firstSequence := publisher.Publish("CDF/USD", etypes.MDSnapshot, message, 2)
	secondSequence := publisher.Publish("CDF/USD", etypes.MDSnapshot, message, 3)
	if firstSequence == 0 || secondSequence != firstSequence+1 {
		t.Fatalf("delivered publish sequences = %d, %d", firstSequence, secondSequence)
	}
	first := <-subscriber.channel
	second := <-subscriber.channel
	if first.SeqNum != firstSequence || second.SeqNum != secondSequence {
		t.Fatalf("delivered message sequences = %d, %d", first.SeqNum, second.SeqNum)
	}
	if first.Data.(*etypes.BookSnapshot) == message {
		t.Fatal("subscriber received the publisher's mutable snapshot pointer")
	}
}

func TestPublicationObserverDistinguishesDeliveryOutcomes(t *testing.T) {
	publisher := NewMDPublisher()
	focal := &testSubscriber{channel: make(chan *etypes.MarketDataMsg, 1), running: true}
	other := &testSubscriber{channel: make(chan *etypes.MarketDataMsg, 8), running: true}
	publisher.Subscribe(7, "ABC/USD", []etypes.MDType{etypes.MDSnapshot}, other)
	publisher.Subscribe(13, "ABC/USD", []etypes.MDType{etypes.MDSnapshot}, focal)
	var outcomes []PublicationOutcome
	publisher.SetPublicationObserver(13, func(outcome PublicationOutcome) {
		outcomes = append(outcomes, outcome)
	})
	snapshot := &etypes.BookSnapshot{Asks: []etypes.PriceLevel{{Price: 101, VisibleQty: 2}}}
	publisher.Publish("ABC/USD", etypes.MDSnapshot, snapshot, 1)
	publisher.Publish("ABC/USD", etypes.MDSnapshot, snapshot, 2)
	publisher.Unsubscribe(13, "ABC/USD")
	publisher.Publish("ABC/USD", etypes.MDSnapshot, snapshot, 3)
	publisher.Subscribe(13, "ABC/USD", []etypes.MDType{etypes.MDTrade}, focal)
	publisher.Publish("ABC/USD", etypes.MDSnapshot, snapshot, 4)
	publisher.Subscribe(13, "ABC/USD", []etypes.MDType{etypes.MDSnapshot}, focal)
	focal.running = false
	publisher.Publish("ABC/USD", etypes.MDSnapshot, snapshot, 5)
	want := []PublicationStatus{
		PublicationEnqueued, PublicationDropped, PublicationNotSubscribed,
		PublicationNotInterested, PublicationGatewayStopped,
	}
	if len(outcomes) != len(want) {
		t.Fatalf("publication outcomes = %d, want %d", len(outcomes), len(want))
	}
	for index, outcome := range outcomes {
		if outcome.Status != want[index] || outcome.ClientID != 13 || outcome.Symbol != "ABC/USD" ||
			outcome.Type != etypes.MDSnapshot || outcome.Timestamp != int64(index+1) || outcome.Sequence != uint64(index+1) {
			t.Fatalf("publication %d = %+v, want status %s", index, outcome, want[index])
		}
	}
	if got := (<-focal.channel).SeqNum; got != 1 || len(focal.channel) != 0 {
		t.Fatalf("focal inbox changed by observer: first sequence=%d remaining=%d", got, len(focal.channel))
	}
	publisher.SetPublicationObserver(13, nil)
	publisher.Publish("ABC/USD", etypes.MDSnapshot, snapshot, 6)
	if len(outcomes) != len(want) {
		t.Fatal("removed publication observer continued to receive events")
	}
}
