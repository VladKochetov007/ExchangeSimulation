package marketdata

import (
	"slices"
	"testing"

	etypes "exchange_sim/types"
)

// recordingSubscriber captures the order in which messages arrive.
type recordingSubscriber struct {
	id      uint64
	channel chan *etypes.MarketDataMsg
	running bool
}

func newRecordingSubscriber(id uint64) *recordingSubscriber {
	return &recordingSubscriber{
		id:      id,
		channel: make(chan *etypes.MarketDataMsg, 64),
		running: true,
	}
}

func (r *recordingSubscriber) ID() uint64                                 { return r.id }
func (r *recordingSubscriber) IsRunning() bool                            { return r.running }
func (r *recordingSubscriber) MarketDataChan() chan *etypes.MarketDataMsg { return r.channel }

// publishOrder records which subscriber received each message first, by
// draining the channels in the order the publisher would have filled them. The
// sequence numbers are what reveal fan-out order.
func publishOrder(t *testing.T, subscribers []*recordingSubscriber) []uint64 {
	t.Helper()
	type arrival struct {
		clientID uint64
		seqNum   uint64
	}
	var arrivals []arrival
	for _, subscriber := range subscribers {
		for {
			select {
			case msg := <-subscriber.channel:
				arrivals = append(arrivals, arrival{subscriber.id, msg.SeqNum})
			default:
			}
			break
		}
	}
	slices.SortStableFunc(arrivals, func(a, b arrival) int {
		switch {
		case a.seqNum < b.seqNum:
			return -1
		case a.seqNum > b.seqNum:
			return 1
		default:
			return 0
		}
	})
	order := make([]uint64, 0, len(arrivals))
	for _, item := range arrivals {
		order = append(order, item.clientID)
	}
	return order
}

// TestPublishFansOutInClientIDOrder pins the property the subscriber-order
// cache must preserve. Fan-out order decides which subscriber reacts first,
// which is exactly what a latency experiment measures, so it is an economic
// input rather than an implementation detail.
func TestPublishFansOutInClientIDOrder(t *testing.T) {
	publisher := NewMDPublisher()
	ids := []uint64{40, 3, 900, 1, 17}
	subscribers := make([]*recordingSubscriber, 0, len(ids))
	for _, id := range ids {
		subscriber := newRecordingSubscriber(id)
		subscribers = append(subscribers, subscriber)
		publisher.Subscribe(id, "ABC/USD", []etypes.MDType{etypes.MDTrade}, subscriber)
	}

	// Publish repeatedly: the first call populates the cache and the rest must
	// be served from it in the same order.
	for round := 0; round < 5; round++ {
		publisher.Publish("ABC/USD", etypes.MDTrade, &etypes.Trade{}, int64(round))
	}

	want := []uint64{1, 3, 17, 40, 900}
	if got := publisher.subscribersInClientOrder("ABC/USD", publisher.Subscriptions["ABC/USD"]); !slices.Equal(got, want) {
		t.Fatalf("subscriber order = %v, want %v", got, want)
	}
	for _, subscriber := range subscribers {
		if len(subscriber.channel) != 5 {
			t.Fatalf("subscriber %d received %d messages, want 5", subscriber.id, len(subscriber.channel))
		}
	}
}

// TestSubscriberOrderTracksSubscriptionChanges requires the cache to follow
// subscribe and unsubscribe rather than serving a stale list.
func TestSubscriberOrderTracksSubscriptionChanges(t *testing.T) {
	publisher := NewMDPublisher()
	for _, id := range []uint64{5, 2, 9} {
		publisher.Subscribe(id, "ABC/USD", []etypes.MDType{etypes.MDTrade}, newRecordingSubscriber(id))
	}
	if got := publisher.subscribersInClientOrder("ABC/USD", publisher.Subscriptions["ABC/USD"]); !slices.Equal(got, []uint64{2, 5, 9}) {
		t.Fatalf("initial order = %v, want [2 5 9]", got)
	}

	publisher.Subscribe(1, "ABC/USD", []etypes.MDType{etypes.MDTrade}, newRecordingSubscriber(1))
	if got := publisher.subscribersInClientOrder("ABC/USD", publisher.Subscriptions["ABC/USD"]); !slices.Equal(got, []uint64{1, 2, 5, 9}) {
		t.Fatalf("after subscribe order = %v, want [1 2 5 9]", got)
	}

	publisher.Unsubscribe(5, "ABC/USD")
	if got := publisher.subscribersInClientOrder("ABC/USD", publisher.Subscriptions["ABC/USD"]); !slices.Equal(got, []uint64{1, 2, 9}) {
		t.Fatalf("after unsubscribe order = %v, want [1 2 9]", got)
	}

	publisher.UnsubscribeClient(2)
	if got := publisher.subscribersInClientOrder("ABC/USD", publisher.Subscriptions["ABC/USD"]); !slices.Equal(got, []uint64{1, 9}) {
		t.Fatalf("after client unsubscribe order = %v, want [1 9]", got)
	}

	// Every reported subscriber must still be subscribed: the publisher
	// dereferences the gateway for each one.
	for _, clientID := range publisher.subscribersInClientOrder("ABC/USD", publisher.Subscriptions["ABC/USD"]) {
		if publisher.Subscriptions["ABC/USD"][clientID] == nil {
			t.Fatalf("cache reported client %d with no subscription", clientID)
		}
	}
}

// TestSubscriberOrderSurvivesAMissedGenerationBump covers the guard, so a
// future mutation site that forgets to invalidate cannot hand Publish a client
// that no longer subscribes.
func TestSubscriberOrderSurvivesAMissedGenerationBump(t *testing.T) {
	publisher := NewMDPublisher()
	for _, id := range []uint64{4, 8} {
		publisher.Subscribe(id, "ABC/USD", []etypes.MDType{etypes.MDTrade}, newRecordingSubscriber(id))
	}
	_ = publisher.subscribersInClientOrder("ABC/USD", publisher.Subscriptions["ABC/USD"])

	// Mutate without bumping the generation, the way a new call site might.
	delete(publisher.Subscriptions["ABC/USD"], 8)
	got := publisher.subscribersInClientOrder("ABC/USD", publisher.Subscriptions["ABC/USD"])
	for _, clientID := range got {
		if publisher.Subscriptions["ABC/USD"][clientID] == nil {
			t.Fatalf("stale client %d survived a missed generation bump: %v", clientID, got)
		}
	}
}

// TestSubscriberOrderIsPerSymbol guards against one symbol's cache being served
// for another.
func TestSubscriberOrderIsPerSymbol(t *testing.T) {
	publisher := NewMDPublisher()
	publisher.Subscribe(7, "ABC/USD", []etypes.MDType{etypes.MDTrade}, newRecordingSubscriber(7))
	publisher.Subscribe(2, "CDF/USD", []etypes.MDType{etypes.MDTrade}, newRecordingSubscriber(2))
	publisher.Subscribe(9, "CDF/USD", []etypes.MDType{etypes.MDTrade}, newRecordingSubscriber(9))

	if got := publisher.subscribersInClientOrder("ABC/USD", publisher.Subscriptions["ABC/USD"]); !slices.Equal(got, []uint64{7}) {
		t.Fatalf("ABC/USD order = %v, want [7]", got)
	}
	if got := publisher.subscribersInClientOrder("CDF/USD", publisher.Subscriptions["CDF/USD"]); !slices.Equal(got, []uint64{2, 9}) {
		t.Fatalf("CDF/USD order = %v, want [2 9]", got)
	}
}
