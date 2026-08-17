package actor

import (
	"testing"

	"exchange_sim/exchange"
)

// decodeRejection exercises the response decoding path an actor's gateway uses.
func decodeRejection(resp exchange.Response) []*Event {
	return NewBaseActor(1, nil).decodeResponse(resp)
}

// A venue that refuses a request tells the client when to come back. An actor
// that ignores that advice hammers the venue and stays refused, which is both
// unrealistic and a way to make a rate-limit experiment measure nothing.
func TestRejectionCarriesTheVenuesRetryAdvice(t *testing.T) {
	events := decodeRejection(exchange.Response{
		RequestID: 7,
		Success:   false,
		Error:     exchange.RejectRateLimited,
		Data:      exchange.RetryAdvice{RetryAfterNanos: 1_500, Limit: "weight"},
	})
	if len(events) != 1 || events[0].Type != EventOrderRejected {
		t.Fatalf("expected one rejection event, got %#v", events)
	}
	rejection := events[0].Data.(OrderRejectedEvent)
	if rejection.RetryAfterNanos != 1_500 {
		t.Fatalf("retry-after = %d, want 1500", rejection.RetryAfterNanos)
	}
	if rejection.Limit != "weight" {
		t.Fatalf("limit = %q, want weight", rejection.Limit)
	}
}

func TestRejectionWithoutAdviceStillReportsItsReason(t *testing.T) {
	events := decodeRejection(exchange.Response{
		RequestID: 8, Success: false, Error: exchange.RejectInvalidPrice,
	})
	rejection := events[0].Data.(OrderRejectedEvent)
	if rejection.Reason != exchange.RejectInvalidPrice {
		t.Fatalf("reason = %q", rejection.Reason)
	}
	if rejection.RetryAfterNanos != 0 {
		t.Fatalf("a rejection with no advice invented a wait of %d", rejection.RetryAfterNanos)
	}
}

// The backoff itself: once told to wait, an actor holds off until the deadline
// and resumes afterwards.
func TestBackoffHoldsUntilTheDeadlineThenResumes(t *testing.T) {
	var backoff RequestBackoff

	if !backoff.Ready(100) {
		t.Fatal("a fresh backoff refused to send")
	}
	backoff.Observe(OrderRejectedEvent{
		Reason: exchange.RejectRateLimited, RetryAfterNanos: 50,
	}, 100)

	if backoff.Ready(149) {
		t.Fatal("sent before the venue's deadline")
	}
	if !backoff.Ready(150) {
		t.Fatal("did not resume at the deadline")
	}
}

// An overload is not the client's fault, but hammering still makes it worse.
// With no advice attached, back off by a default rather than not at all.
func TestOverloadWithoutAdviceStillBacksOff(t *testing.T) {
	backoff := RequestBackoff{DefaultWaitNanos: 25}
	backoff.Observe(OrderRejectedEvent{Reason: exchange.RejectOverloaded}, 0)
	if backoff.Ready(24) {
		t.Fatal("did not back off from an overload carrying no advice")
	}
	if !backoff.Ready(25) {
		t.Fatal("stayed backed off past the default wait")
	}
}

// Rejections that are the actor's own fault carry no wait: retrying the same
// bad price later will fail identically, so pausing would hide the defect.
func TestOrdinaryRejectionsDoNotBackOff(t *testing.T) {
	var backoff RequestBackoff
	backoff.Observe(OrderRejectedEvent{Reason: exchange.RejectInvalidPrice}, 0)
	if !backoff.Ready(0) {
		t.Fatal("backed off from a rejection the venue did not ask us to wait on")
	}
}

// A later deadline extends the wait; an earlier one must not shorten it.
func TestTheLongestOutstandingWaitWins(t *testing.T) {
	var backoff RequestBackoff
	backoff.Observe(OrderRejectedEvent{Reason: exchange.RejectRateLimited, RetryAfterNanos: 100}, 0)
	backoff.Observe(OrderRejectedEvent{Reason: exchange.RejectRateLimited, RetryAfterNanos: 10}, 0)
	if backoff.Ready(50) {
		t.Fatal("a shorter later wait cut an outstanding backoff short")
	}
	if !backoff.Ready(100) {
		t.Fatal("did not resume at the longest deadline")
	}
}
