package ratelimit

import (
	"testing"
	"time"
)

const (
	second = int64(time.Second)
	minute = int64(time.Minute)
)

// A fixed window is the scheme Binance documents: a budget of request weight
// per interval, reset when the interval rolls over rather than refilled
// continuously.
func TestFixedWindowAdmitsUpToTheBudgetAndRejectsTheRequestThatWouldExceedIt(t *testing.T) {
	window := NewFixedWindow("weight", 100, minute)

	if decision := window.Admit("ip", 60, 0); !decision.Allowed {
		t.Fatalf("first request of 60 against a budget of 100 was rejected: %+v", decision)
	}
	// Exactly reaching the budget must be allowed; only exceeding it must not.
	if decision := window.Admit("ip", 40, 0); !decision.Allowed {
		t.Fatalf("request landing exactly on the budget was rejected: %+v", decision)
	}
	decision := window.Admit("ip", 1, 0)
	if decision.Allowed {
		t.Fatal("request exceeding the budget by one was admitted")
	}
	if decision.Limit != "weight" {
		t.Fatalf("rejection did not name the limit it violated: %+v", decision)
	}
	if decision.RetryAfter != minute {
		t.Fatalf("retry-after = %d, want the remaining window %d", decision.RetryAfter, minute)
	}
}

func TestFixedWindowResetsOnlyWhenTheIntervalRollsOver(t *testing.T) {
	window := NewFixedWindow("weight", 10, minute)
	window.Admit("ip", 10, 0)

	if decision := window.Admit("ip", 1, minute-1); decision.Allowed {
		t.Fatal("budget reset before the interval elapsed")
	}
	if decision := window.Admit("ip", 10, minute); !decision.Allowed {
		t.Fatalf("budget did not reset when the interval rolled over: %+v", decision)
	}
	// A gap of many windows must not accumulate credit.
	window.Admit("ip", 10, 10*minute)
	if decision := window.Admit("ip", 1, 10*minute); decision.Allowed {
		t.Fatal("idle windows accumulated credit")
	}
}

func TestFixedWindowRetryAfterCountsDownWithinTheWindow(t *testing.T) {
	window := NewFixedWindow("weight", 10, minute)
	window.Admit("ip", 10, 0)

	decision := window.Admit("ip", 1, minute/4)
	if decision.Allowed {
		t.Fatal("exhausted budget admitted a request")
	}
	if want := minute - minute/4; decision.RetryAfter != want {
		t.Fatalf("retry-after = %d, want %d", decision.RetryAfter, want)
	}
}

func TestScopesAreIsolated(t *testing.T) {
	window := NewFixedWindow("weight", 10, minute)
	window.Admit("first", 10, 0)

	if decision := window.Admit("second", 10, 0); !decision.Allowed {
		t.Fatalf("one scope's usage consumed another's budget: %+v", decision)
	}
}

// A token bucket refills continuously, which is how a venue smooths bursts
// rather than resetting on a boundary.
func TestTokenBucketRefillsContinuouslyAndNeverExceedsCapacity(t *testing.T) {
	bucket := NewTokenBucket("orders", 10, 10, second)

	if decision := bucket.Admit("account", 10, 0); !decision.Allowed {
		t.Fatal("full bucket rejected a request equal to capacity")
	}
	if decision := bucket.Admit("account", 1, 0); decision.Allowed {
		t.Fatal("empty bucket admitted a request")
	}
	// Half a second at ten tokens a second is five tokens.
	if decision := bucket.Admit("account", 5, second/2); !decision.Allowed {
		t.Fatalf("bucket did not refill proportionally: %+v", decision)
	}
	// Idling far longer than the fill time must not overfill it.
	bucket.Admit("account", 0, 100*second)
	if decision := bucket.Admit("account", 11, 100*second); decision.Allowed {
		t.Fatal("bucket refilled beyond its capacity")
	}
}

func TestTokenBucketRetryAfterIsTheTimeToAccrueTheShortfall(t *testing.T) {
	bucket := NewTokenBucket("orders", 10, 10, second)
	bucket.Admit("account", 10, 0)

	decision := bucket.Admit("account", 2, 0)
	if decision.Allowed {
		t.Fatal("empty bucket admitted a request")
	}
	// Two tokens at ten a second is 200ms.
	if want := int64(200 * time.Millisecond); decision.RetryAfter != want {
		t.Fatalf("retry-after = %d, want %d", decision.RetryAfter, want)
	}
}

// A request that can never fit must be rejected outright rather than promising
// a retry that will also fail.
func TestRequestLargerThanCapacityIsRejectedAsImpossible(t *testing.T) {
	for name, limiter := range map[string]Limiter{
		"fixed window": NewFixedWindow("weight", 10, minute),
		"token bucket": NewTokenBucket("orders", 10, 10, second),
	} {
		decision := limiter.Admit("scope", 11, 0)
		if decision.Allowed {
			t.Fatalf("%s admitted a request larger than its capacity", name)
		}
		if !decision.Impossible {
			t.Fatalf("%s did not mark an unsatisfiable request as impossible: %+v", name, decision)
		}
	}
}

func TestZeroCostRequestsAreAlwaysAdmitted(t *testing.T) {
	window := NewFixedWindow("weight", 1, minute)
	window.Admit("ip", 1, 0)
	if decision := window.Admit("ip", 0, 0); !decision.Allowed {
		t.Fatalf("a request costing nothing was rejected: %+v", decision)
	}
}
