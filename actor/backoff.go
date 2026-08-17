package actor

import "exchange_sim/exchange"

// RequestBackoff holds an actor off after a venue has refused it for a budget
// or a backlog. Embedding it costs nothing until a venue actually refuses
// something, and an actor that skips it will simply be refused repeatedly,
// which is realistic but measures the actor's persistence rather than its
// strategy.
type RequestBackoff struct {
	// DefaultWaitNanos is used when a venue refuses without saying for how
	// long, which happens with an overload: the venue does not know when it
	// will drain. Zero means resume immediately.
	DefaultWaitNanos int64

	readyAt int64
}

// Observe records a rejection. Only refusals the venue asked us to wait on
// cause a backoff: an invalid price will fail identically later, so pausing
// would hide the defect rather than fix it.
func (b *RequestBackoff) Observe(rejection OrderRejectedEvent, now int64) {
	switch rejection.Reason {
	case exchange.RejectRateLimited, exchange.RejectOverloaded:
	default:
		return
	}
	wait := rejection.RetryAfterNanos
	if wait <= 0 {
		wait = b.DefaultWaitNanos
	}
	// The longest outstanding deadline wins: a later, shorter wait must not cut
	// an earlier one short, or an actor refused by a daily budget would resume
	// on a per-second budget's advice.
	if deadline := now + wait; deadline > b.readyAt {
		b.readyAt = deadline
	}
}

// Ready reports whether the actor may send again.
func (b *RequestBackoff) Ready(now int64) bool { return now >= b.readyAt }

// ReadyAt is the deadline being waited on, for telemetry.
func (b *RequestBackoff) ReadyAt() int64 { return b.readyAt }
