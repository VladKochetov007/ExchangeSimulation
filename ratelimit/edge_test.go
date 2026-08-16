package ratelimit

import (
	"math"
	"testing"
	"time"
)

// A simulator replays and reorders timestamps. A limiter that trusts time to
// move forward will hand out free budget when it does not.
func TestTimeGoingBackwardsDoesNotRefillOrReset(t *testing.T) {
	bucket := NewTokenBucket("orders", 10, 10, second)
	bucket.Admit("acct", 10, 10*second)
	if decision := bucket.Admit("acct", 1, 9*second); decision.Allowed {
		t.Fatal("token bucket refilled when time moved backwards")
	}

	window := NewFixedWindow("weight", 10, minute)
	window.Admit("ip", 10, 10*minute)
	if decision := window.Admit("ip", 1, 10*minute+1); decision.Allowed {
		t.Fatal("fixed window admitted beyond its budget inside the window")
	}
	// Stepping back into an earlier window is a different window, so a reset
	// there is correct. What must not happen is the later window forgetting.
	window.Admit("ip", 10, 9*minute)
	if decision := window.Admit("ip", 1, 10*minute+2); decision.Allowed {
		t.Fatal("revisiting an earlier window cleared the later window's usage")
	}
}

// Costs and elapsed times are attacker-sized in a fuzz or a misconfiguration.
// Silent wraparound would turn a rejection into an admission.
func TestLargeValuesDoNotWrapAround(t *testing.T) {
	bucket := NewTokenBucket("orders", math.MaxInt64/4, 10, second)
	if decision := bucket.Admit("acct", math.MaxInt64/8, 0); decision.Allowed {
		t.Fatal("token bucket admitted a cost whose scaling overflows")
	}

	// A long idle gap must not overflow while computing the refill.
	slow := NewTokenBucket("orders", 10, 1, int64(time.Hour))
	slow.Admit("acct", 10, 0)
	if decision := slow.Admit("acct", 10, math.MaxInt64/2); !decision.Allowed {
		t.Fatalf("a long idle gap did not refill the bucket: %+v", decision)
	}

	window := NewFixedWindow("weight", 100, minute)
	if decision := window.Admit("ip", math.MaxInt64, 0); decision.Allowed {
		t.Fatal("fixed window admitted a cost larger than any budget")
	}
	if decision := window.Admit("ip", 1, math.MaxInt64-1); !decision.Allowed {
		t.Fatalf("a timestamp near the maximum broke window alignment: %+v", decision)
	}
}

// A budget of zero must refuse everything rather than admit everything, which
// is what an unchecked comparison would do.
func TestDegenerateConfigurationsRefuseRatherThanAdmit(t *testing.T) {
	if decision := NewFixedWindow("weight", 0, minute).Admit("ip", 1, 0); decision.Allowed {
		t.Fatal("a zero budget admitted a request")
	}
	if decision := NewTokenBucket("orders", 0, 10, second).Admit("acct", 1, 0); decision.Allowed {
		t.Fatal("a zero capacity admitted a request")
	}
	// A zero interval cannot refill or roll over. It must not divide by zero
	// and must not admit beyond the initial allowance.
	frozen := NewTokenBucket("orders", 1, 10, 0)
	if decision := frozen.Admit("acct", 1, 0); !decision.Allowed {
		t.Fatal("a zero interval refused the initial allowance")
	}
	if decision := frozen.Admit("acct", 1, second); decision.Allowed {
		t.Fatal("a zero-interval bucket refilled")
	}
	if decision := NewFixedWindow("weight", 10, 0).Admit("ip", 1, 0); !decision.Allowed {
		t.Fatal("a zero-interval window refused its first request")
	}
}

// Would and Admit must never disagree, or the gate charges budgets for a
// request it then refuses. Drive two identical limiters through the same
// history and compare the probe against the charge at every step.
func TestWouldAgreesWithAdmit(t *testing.T) {
	history := []struct{ cost, now int64 }{
		{3, 0}, {3, 0}, {3, 0}, {3, 0}, {11, 0}, {0, 0},
		{1, second}, {5, second}, {2, 2 * second}, {10, 10 * second}, {1, 9 * second},
	}
	for name, build := range map[string]func() Limiter{
		"fixed window": func() Limiter { return NewFixedWindow("w", 10, minute) },
		"token bucket": func() Limiter { return NewTokenBucket("o", 10, 5, second) },
	} {
		limiter := build()
		dry := limiter.(dryRunLimiter)
		for step, event := range history {
			probe := dry.Would("scope", event.cost, event.now)
			charge := limiter.Admit("scope", event.cost, event.now)
			if probe.Allowed != charge.Allowed {
				t.Fatalf("%s step %d: Would said allowed=%v, Admit said allowed=%v",
					name, step, probe.Allowed, charge.Allowed)
			}
			if probe.Impossible != charge.Impossible || probe.Limit != charge.Limit {
				t.Fatalf("%s step %d: probe %+v disagrees with charge %+v", name, step, probe, charge)
			}
			if !probe.Allowed && probe.RetryAfter != charge.RetryAfter {
				t.Fatalf("%s step %d: retry-after %d vs %d", name, step, probe.RetryAfter, charge.RetryAfter)
			}
		}
	}
}

// Completing a lane the caller never offered to must not manufacture capacity
// in the other lane.
func TestCompletingTheWrongLaneCannotCorruptCounts(t *testing.T) {
	queue := NewAdmissionQueue(AdmissionConfig{PriorityDepth: 1, SecondaryDepth: 1})
	queue.Offer(KindPlaceOrder) // fills the secondary lane

	// A caller that mistakenly completes a cancel must not free the order slot.
	queue.Complete(KindCancelOrder)
	if decision := queue.Offer(KindPlaceOrder); decision.Allowed {
		t.Fatal("completing the priority lane freed a secondary slot")
	}
	if priority, secondary := queue.Depth(); priority != 0 || secondary != 1 {
		t.Fatalf("depths = (%d, %d), want (0, 1): a stray completion moved a count", priority, secondary)
	}
}

func TestReduceOnlyPlacementKeepsThePriorityLaneEvenWhenOrdersAreRefused(t *testing.T) {
	queue := NewAdmissionQueue(AdmissionConfig{PriorityDepth: 4, SecondaryDepth: 1})
	queue.Offer(KindPlaceOrder)
	if decision := queue.Offer(KindPlaceOrder); !decision.Overloaded {
		t.Fatalf("expected the secondary lane to be saturated: %+v", decision)
	}
	// Closing risk is a placement, and must still be accepted.
	if decision := queue.Offer(KindPlaceReduceOnly); !decision.Allowed {
		t.Fatalf("a reduce-only placement was refused while only new risk was blocked: %+v", decision)
	}
}
