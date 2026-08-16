package ratelimit

import (
	"testing"
	"time"
)

// A bucket must never grant more than its configured rate. Truncating the
// fill-span division lets a drained bucket admit a full capacity request after
// earning only part of it.
func TestBucketNeverGrantsMoreThanTheConfiguredRate(t *testing.T) {
	// Ten capacity refilling at six a second: one second after draining, six
	// tokens exist, not ten.
	bucket := NewTokenBucket("o", 10, 6, second)
	bucket.Admit("acct", 10, 0)

	if decision := bucket.Admit("acct", 10, second); decision.Allowed {
		t.Fatal("bucket admitted 10 tokens one second after draining, at a rate of 6 a second")
	}
	if decision := bucket.Admit("acct", 6, second); !decision.Allowed {
		t.Fatalf("bucket did not grant the six tokens actually earned: %+v", decision)
	}
}

// Retry-after must be the true wait. An overflow in the arithmetic sends the
// client back early, to be refused again in a loop.
func TestRetryAfterIsHonestAtLargeCapacity(t *testing.T) {
	bucket := NewTokenBucket("o", 20_000, 1, second)
	bucket.Admit("acct", 20_000, 0)

	decision := bucket.Admit("acct", 20_000, 0)
	if decision.Allowed {
		t.Fatal("drained bucket admitted a full request")
	}
	if want := int64(20_000) * second; decision.RetryAfter != want {
		t.Fatalf("retry-after = %d, want %d: the wait was computed wrong", decision.RetryAfter, want)
	}
}

// A budget that can never refill must say so, rather than promise a retry that
// will fail identically forever.
func TestABucketThatCannotRefillReportsImpossibleRatherThanRetryZero(t *testing.T) {
	for name, bucket := range map[string]*TokenBucket{
		"zero refill rate": NewTokenBucket("o", 10, 0, second),
		"zero interval":    NewTokenBucket("o", 10, 10, 0),
	} {
		bucket.Admit("acct", 10, 0)
		decision := bucket.Admit("acct", 1, 10*second)
		if decision.Allowed {
			t.Fatalf("%s: drained unrefillable bucket admitted a request", name)
		}
		if decision.RetryAfter == 0 && !decision.Impossible {
			t.Fatalf("%s: promised retry-after 0 without marking it impossible: %+v", name, decision)
		}
	}
}

// countingLimiter is the extension point a user writes. Would is part of the
// Limiter contract precisely so the gate cannot charge it twice.
type countingLimiter struct {
	name    string
	charges int
	used    int64
	budget  int64
}

func (c *countingLimiter) Name() string { return c.name }

func (c *countingLimiter) Would(_ string, cost, _ int64) Decision {
	if c.used+cost > c.budget {
		return Decision{Limit: c.name}
	}
	return Allow()
}

func (c *countingLimiter) Admit(_ string, cost, _ int64) Decision {
	c.charges++
	if c.used+cost > c.budget {
		return Decision{Limit: c.name}
	}
	c.used += cost
	return Allow()
}

func TestACustomLimiterIsChargedExactlyOnce(t *testing.T) {
	limiter := &countingLimiter{name: "custom", budget: 100}
	gate := NewGate([]Meter{{Limiter: limiter, Cost: StaticCost{Default: 10}}}, nil)

	if decision := gate.Admit("acct", KindPlaceOrder, 0); !decision.Allowed {
		t.Fatalf("custom limiter rejected a request inside its budget: %+v", decision)
	}
	if limiter.charges != 1 {
		t.Fatalf("limiter charged %d times for one request", limiter.charges)
	}
	if limiter.used != 10 {
		t.Fatalf("limiter used = %d, want 10", limiter.used)
	}
}

// The gate must be exercised with a queue, which is how the overload path stays
// honest about the client's budget.
func TestGateWithAQueueRefusesOnOverloadWithoutCharging(t *testing.T) {
	weight := NewFixedWindow("weight", 1000, minute)
	queue := NewAdmissionQueue(AdmissionConfig{SecondaryDepth: Depth(1)})
	gate := NewGate([]Meter{{Limiter: weight, Cost: StaticCost{Default: 10}}}, queue)

	gate.Admit("acct", KindPlaceOrder, 0)
	decision := gate.Admit("acct", KindPlaceOrder, 0)
	if !decision.Overloaded {
		t.Fatalf("expected overload: %+v", decision)
	}
	if used := weight.Used("acct", 0); used != 10 {
		t.Fatalf("weight used = %d, want 10", used)
	}
	// And a cancel still gets through while the order lane is saturated.
	if decision := gate.Admit("acct", KindCancelOrder, 0); !decision.Allowed {
		t.Fatalf("cancel refused while only the order lane was full: %+v", decision)
	}
}

// The load-bearing claim in the published scheme: a throttled client can still
// withdraw the orders it holds.
func TestPublishedSchemeLetsAThrottledClientStillCancel(t *testing.T) {
	gate := NewGate(BinanceSpotLike(), nil)
	now := int64(0)
	// Exhaust the ten-second order budget.
	for i := 0; i < 100; i++ {
		if decision := gate.Admit("acct", KindPlaceOrder, now); !decision.Allowed {
			t.Fatalf("placement %d refused before the budget was spent: %+v", i, decision)
		}
	}
	if decision := gate.Admit("acct", KindPlaceOrder, now); decision.Allowed {
		t.Fatal("order budget did not bind")
	}
	if decision := gate.Admit("acct", KindCancelOrder, now); !decision.Allowed {
		t.Fatalf("a client throttled on new orders could not cancel: %+v", decision)
	}
	if decision := gate.Admit("acct", KindPlaceReduceOnly, now); decision.Allowed {
		t.Fatal("reduce-only placements should still count against the order budget")
	}
}

// Capacity must actually cap accrual. The previous version of this test could
// not see the cap because it asked for more than capacity.
func TestIdleBucketFillsToCapacityAndNoFurther(t *testing.T) {
	bucket := NewTokenBucket("o", 10, 10, second)
	bucket.Admit("acct", 10, 0)
	if decision := bucket.Admit("acct", 10, 100*second); !decision.Allowed {
		t.Fatalf("long idle bucket did not refill to capacity: %+v", decision)
	}
	if decision := bucket.Admit("acct", 1, 100*second); decision.Allowed {
		t.Fatal("bucket held more than its capacity after a long idle span")
	}
}

func TestTokenBucketScopesAreIsolated(t *testing.T) {
	bucket := NewTokenBucket("o", 10, 1, second)
	bucket.Admit("first", 10, 0)
	if decision := bucket.Admit("second", 10, 0); !decision.Allowed {
		t.Fatalf("one scope drained another's bucket: %+v", decision)
	}
}

func TestUsedReportsZeroAfterTheWindowRollsOver(t *testing.T) {
	window := NewFixedWindow("weight", 10, minute)
	window.Admit("ip", 7, 0)
	if used := window.Used("ip", 0); used != 7 {
		t.Fatalf("used = %d, want 7", used)
	}
	if used := window.Used("ip", minute); used != 0 {
		t.Fatalf("used = %d after rollover, want 0", used)
	}
}

func TestImpossibleSurvivesTheGate(t *testing.T) {
	gate := NewGate([]Meter{
		{Limiter: NewFixedWindow("weight", 5, minute), Cost: StaticCost{Default: 50}},
	}, nil)
	decision := gate.Admit("acct", KindPlaceOrder, 0)
	if decision.Allowed || !decision.Impossible {
		t.Fatalf("a request larger than the budget did not surface as impossible: %+v", decision)
	}
}

// Two identical gates driven through one sequence must produce identical
// decision streams, or the simulator is not reproducible.
func TestDecisionsAreReproducible(t *testing.T) {
	build := func() *Gate {
		return NewGate(BinanceSpotLike(), NewAdmissionQueue(AdmissionConfig{PriorityDepth: Depth(4), SecondaryDepth: Depth(4)}))
	}
	kinds := []RequestKind{KindPlaceOrder, KindCancelOrder, KindQueryBalance, KindPlaceReduceOnly, KindSubscribe}
	first, second := build(), build()
	for step := 0; step < 500; step++ {
		kind := kinds[step%len(kinds)]
		now := int64(step) * int64(time.Millisecond)
		a := first.Admit("acct", kind, now)
		b := second.Admit("acct", kind, now)
		if a != b {
			t.Fatalf("step %d: %+v vs %+v", step, a, b)
		}
	}
}

// A caller's own request kinds must be able to take the priority lane without
// editing this package.
func TestCallerDefinedKindsCanBeClassifiedAsRiskReducing(t *testing.T) {
	const kindUnwindPosition RequestKind = 40
	queue := NewAdmissionQueue(AdmissionConfig{
		PriorityDepth:  Depth(2),
		SecondaryDepth: Depth(1),
		RiskReducing: func(kind RequestKind) bool {
			return kind == kindUnwindPosition || kind.RiskReducing()
		},
	})
	queue.Offer(KindPlaceOrder) // saturate the secondary lane
	if decision, _ := queue.Offer(kindUnwindPosition); !decision.Allowed {
		t.Fatalf("a caller-defined risk-reducing kind was refused: %+v", decision)
	}
	if priority, _ := queue.Depth(); priority != 1 {
		t.Fatalf("priority depth = %d, want 1: the caller's classifier was ignored", priority)
	}
}

// A configured zero closes a lane; an absent field leaves it unlimited. Reading
// a missing field as unlimited is fine, but reading a configured zero that way
// would silently disable the overload modelling.
func TestZeroDepthClosesALaneAndAbsentDepthLeavesItUnlimited(t *testing.T) {
	closed := NewAdmissionQueue(AdmissionConfig{SecondaryDepth: Depth(0)})
	if decision, _ := closed.Offer(KindPlaceOrder); decision.Allowed {
		t.Fatal("a lane configured with depth zero accepted work")
	}
	open := NewAdmissionQueue(AdmissionConfig{})
	for i := 0; i < 100; i++ {
		if decision, _ := open.Offer(KindPlaceOrder); !decision.Allowed {
			t.Fatalf("an unconfigured lane refused at %d", i)
		}
	}
}

// A slot must return to the lane it came from, whatever kind the caller quotes.
func TestSlotsReturnToTheLaneTheyCameFrom(t *testing.T) {
	queue := NewAdmissionQueue(AdmissionConfig{PriorityDepth: Depth(1), SecondaryDepth: Depth(1)})
	_, slot := queue.Offer(KindPlaceOrder)
	queue.Complete(slot)
	if priority, secondary := queue.Depth(); priority != 0 || secondary != 0 {
		t.Fatalf("depths = (%d, %d), want (0, 0)", priority, secondary)
	}
	// Returning a slot that was never held is a no-op, not a free slot.
	queue.Complete(Slot{})
	if priority, secondary := queue.Depth(); priority != 0 || secondary != 0 {
		t.Fatalf("an empty slot changed the depths to (%d, %d)", priority, secondary)
	}
}
