package exchange

import (
	"testing"
	"time"

	"exchange_sim/ratelimit"
)

func tierSpec(weightBudget int64, secondaryDepth int) RequestTier {
	spec := RequestTier{
		Meters: []ratelimit.Meter{{
			Limiter: ratelimit.NewFixedWindow("weight", weightBudget, int64(time.Minute)),
			Cost:    ratelimit.StaticCost{Default: 1},
		}},
	}
	if secondaryDepth > 0 {
		spec.Queue = ratelimit.AdmissionConfig{SecondaryDepth: ratelimit.Depth(secondaryDepth)}
		spec.Queued = true
	}
	return spec
}

// Different participants get different budgets, which is the point: a venue
// gives a market maker a larger allowance than an anonymous taker.
func TestTiersGiveParticipantsDifferentBudgets(t *testing.T) {
	policy := NewTieredRequestPolicy(map[string]RequestTier{
		"maker": tierSpec(3, 0),
		"taker": tierSpec(1, 0),
	}, func(clientID uint64) string {
		if clientID == 1 {
			return "maker"
		}
		return "taker"
	})

	for i := 0; i < 3; i++ {
		if _, _, ok := policy.Admit(1, ratelimit.KindPlaceOrder, 0); !ok {
			t.Fatalf("maker refused request %d inside its budget", i)
		}
	}
	if _, _, ok := policy.Admit(1, ratelimit.KindPlaceOrder, 0); ok {
		t.Fatal("maker admitted beyond its budget")
	}

	if _, _, ok := policy.Admit(2, ratelimit.KindPlaceOrder, 0); !ok {
		t.Fatal("taker refused its first request")
	}
	if _, _, ok := policy.Admit(2, ratelimit.KindPlaceOrder, 0); ok {
		t.Fatal("taker admitted beyond its smaller budget")
	}
}

// One participant exhausting its budget must not affect another's.
func TestBudgetsAreIsolatedPerParticipant(t *testing.T) {
	policy := NewTieredRequestPolicy(map[string]RequestTier{
		"taker": tierSpec(1, 0),
	}, func(uint64) string { return "taker" })

	policy.Admit(1, ratelimit.KindPlaceOrder, 0)
	if _, _, ok := policy.Admit(2, ratelimit.KindPlaceOrder, 0); !ok {
		t.Fatal("one client's usage consumed another's budget")
	}
}

func TestBudgetRejectionsSayRateLimitedAndCarryTheWait(t *testing.T) {
	policy := NewTieredRequestPolicy(map[string]RequestTier{
		"taker": tierSpec(1, 0),
	}, func(uint64) string { return "taker" })

	policy.Admit(1, ratelimit.KindPlaceOrder, 0)
	_, rejection, ok := policy.Admit(1, ratelimit.KindPlaceOrder, 0)
	if ok {
		t.Fatal("request admitted beyond the budget")
	}
	if rejection.Error != RejectRateLimited {
		t.Fatalf("reason = %q, want %q", rejection.Error, RejectRateLimited)
	}
	advice, hasAdvice := rejection.Data.(RetryAdvice)
	if !hasAdvice {
		t.Fatalf("rejection carried no retry advice: %#v", rejection.Data)
	}
	if advice.RetryAfterNanos != int64(time.Minute) {
		t.Fatalf("retry-after = %d, want a minute", advice.RetryAfterNanos)
	}
}

// A saturated queue is a different apology, and cancels must still get through.
func TestQueueSaturationSaysOverloadedAndSparesCancels(t *testing.T) {
	policy := NewTieredRequestPolicy(map[string]RequestTier{
		"taker": tierSpec(1000, 1),
	}, func(uint64) string { return "taker" })

	permit, _, ok := policy.Admit(1, ratelimit.KindPlaceOrder, 0)
	if !ok {
		t.Fatal("first placement refused")
	}
	_, rejection, ok := policy.Admit(1, ratelimit.KindPlaceOrder, 0)
	if ok {
		t.Fatal("second placement admitted with the lane full")
	}
	if rejection.Error != RejectOverloaded {
		t.Fatalf("reason = %q, want %q", rejection.Error, RejectOverloaded)
	}
	if _, _, ok := policy.Admit(1, ratelimit.KindCancelOrder, 0); !ok {
		t.Fatal("a cancel was refused while only the order lane was saturated")
	}

	// Releasing the permit frees the lane again.
	policy.Release(permit)
	if _, _, ok := policy.Admit(1, ratelimit.KindPlaceOrder, 0); !ok {
		t.Fatal("releasing a permit did not free the lane")
	}
}

func TestAnUnknownTierIsUnmetered(t *testing.T) {
	policy := NewTieredRequestPolicy(map[string]RequestTier{
		"taker": tierSpec(1, 0),
	}, func(uint64) string { return "nonexistent" })
	for i := 0; i < 100; i++ {
		if _, _, ok := policy.Admit(1, ratelimit.KindPlaceOrder, 0); !ok {
			t.Fatalf("a client with no matching tier was refused at %d", i)
		}
	}
}

// Client identifiers beyond the Unicode range must not collide into one budget.
func TestLargeClientIdentifiersDoNotShareABudget(t *testing.T) {
	policy := NewTieredRequestPolicy(map[string]RequestTier{
		"taker": tierSpec(1, 0),
	}, func(uint64) string { return "taker" })

	const first, second = uint64(1 << 40), uint64(1<<40) + 1
	if _, _, ok := policy.Admit(first, ratelimit.KindPlaceOrder, 0); !ok {
		t.Fatal("first client refused")
	}
	if _, _, ok := policy.Admit(second, ratelimit.KindPlaceOrder, 0); !ok {
		t.Fatal("a distinct large client identifier shared the first client's budget")
	}
}

// Without rejection counts a run cannot tell a participant that chose not to
// trade from one the venue refused, which is the confound that made earlier
// payoff comparisons unreadable.
func TestPolicyCountsAdmissionsAndRefusalsPerClient(t *testing.T) {
	policy := NewTieredRequestPolicy(map[string]RequestTier{
		"taker": tierSpec(2, 1),
	}, func(uint64) string { return "taker" })

	permit, _, _ := policy.Admit(1, ratelimit.KindPlaceOrder, 0) // admitted, fills lane
	policy.Admit(1, ratelimit.KindPlaceOrder, 0)                 // lane full: overloaded
	policy.Release(permit)
	policy.Admit(1, ratelimit.KindPlaceOrder, 0) // weight budget now exhausted
	policy.Admit(1, ratelimit.KindPlaceOrder, 0) // rate limited

	stats := policy.Stats(1)
	if stats.Admitted != 2 {
		t.Fatalf("admitted = %d, want 2", stats.Admitted)
	}
	if stats.Overloaded != 1 {
		t.Fatalf("overloaded = %d, want 1", stats.Overloaded)
	}
	if stats.RateLimited != 1 {
		t.Fatalf("rate limited = %d, want 1", stats.RateLimited)
	}
	if stats.ByKind[ratelimit.KindPlaceOrder] != 4 {
		t.Fatalf("placements seen = %d, want 4", stats.ByKind[ratelimit.KindPlaceOrder])
	}
}

func TestStatsForAnUnmeteredClientAreEmptyRatherThanMissing(t *testing.T) {
	policy := NewTieredRequestPolicy(nil, func(uint64) string { return "none" })
	policy.Admit(1, ratelimit.KindPlaceOrder, 0)
	if stats := policy.Stats(1); stats.Admitted != 1 || stats.RateLimited != 0 {
		t.Fatalf("unmetered client stats = %+v", stats)
	}
}
