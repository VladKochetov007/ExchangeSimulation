package ratelimit

import (
	"testing"
	"time"
)

// Published websocket limits are per connection, not per account: one client
// with two connections gets two allowances, and one connection shared by two
// clients gets one.
func TestStreamsAreCountedPerConnection(t *testing.T) {
	guard := NewStreamGuard(StreamLimits{MaxStreamsPerConnection: 2, ControlMessagesPerSec: 100})

	for i := 0; i < 2; i++ {
		if decision := guard.Subscribe("conn-a", 0); !decision.Allowed {
			t.Fatalf("subscription %d refused below the cap: %+v", i, decision)
		}
	}
	decision := guard.Subscribe("conn-a", 0)
	if decision.Allowed {
		t.Fatal("subscribed beyond the per-connection cap")
	}
	if decision.Limit != "streams_per_connection" {
		t.Fatalf("rejection named %q", decision.Limit)
	}
	if decision.Impossible {
		t.Fatal("a full connection is not impossible: unsubscribing makes room")
	}
	if guard.Subscribe("conn-b", 0).Allowed != true {
		t.Fatal("a second connection was refused its own allowance")
	}
}

func TestUnsubscribingFreesAStreamSlot(t *testing.T) {
	guard := NewStreamGuard(StreamLimits{MaxStreamsPerConnection: 1, ControlMessagesPerSec: 100})
	guard.Subscribe("conn", 0)
	if guard.Subscribe("conn", 0).Allowed {
		t.Fatal("subscribed beyond the cap")
	}
	guard.Unsubscribe("conn")
	if decision := guard.Subscribe("conn", 0); !decision.Allowed {
		t.Fatalf("unsubscribing did not free a slot: %+v", decision)
	}
	// Unsubscribing more than was subscribed must not manufacture capacity.
	guard.Unsubscribe("conn")
	guard.Unsubscribe("conn")
	guard.Subscribe("conn", 0)
	if guard.Subscribe("conn", 0).Allowed {
		t.Fatal("over-unsubscribing manufactured stream capacity")
	}
}

// A connection is also limited in how fast it may send control messages, and
// exceeding that is documented as grounds for disconnection rather than a
// simple refusal.
func TestControlMessageRateIsEnforcedPerConnection(t *testing.T) {
	guard := NewStreamGuard(StreamLimits{MaxStreamsPerConnection: 100, ControlMessagesPerSec: 2})

	for i := 0; i < 2; i++ {
		if decision := guard.Control("conn", 0); !decision.Allowed {
			t.Fatalf("control message %d refused below the rate: %+v", i, decision)
		}
	}
	decision := guard.Control("conn", 0)
	if decision.Allowed {
		t.Fatal("control messages exceeded the documented rate")
	}
	if decision.Limit != "control_messages" {
		t.Fatalf("rejection named %q", decision.Limit)
	}
	// A second later the allowance is back.
	if decision := guard.Control("conn", int64(time.Second)); !decision.Allowed {
		t.Fatalf("control allowance did not refill: %+v", decision)
	}
}

func TestZeroLimitsMeanUnlimitedStreams(t *testing.T) {
	guard := NewStreamGuard(StreamLimits{})
	for i := 0; i < 5000; i++ {
		if decision := guard.Subscribe("conn", 0); !decision.Allowed {
			t.Fatalf("unconfigured guard refused subscription %d", i)
		}
		if decision := guard.Control("conn", 0); !decision.Allowed {
			t.Fatalf("unconfigured guard refused control message %d", i)
		}
	}
}

func TestPublishedStreamLimitsMatchTheDocumentedFigures(t *testing.T) {
	limits := BinanceStreamLike()
	if limits.MaxStreamsPerConnection != 1024 {
		t.Fatalf("streams per connection = %d, want the documented 1024", limits.MaxStreamsPerConnection)
	}
	if limits.ControlMessagesPerSec != 5 {
		t.Fatalf("control messages per second = %d, want the documented 5", limits.ControlMessagesPerSec)
	}
}
