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
