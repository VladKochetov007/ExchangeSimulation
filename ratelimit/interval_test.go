package ratelimit

import (
	"testing"
	"time"
)

// Every bucket test in this package used a one-second interval, which is why a
// defect affecting every minute, hour and day bucket shipped. This walks the
// interval and rate space and checks accrual against a computed reference
// rather than a hand-picked constant.
func TestAccrualMatchesTheConfiguredRateAtEveryInterval(t *testing.T) {
	intervals := map[string]int64{
		"millisecond": int64(time.Millisecond),
		"second":      int64(time.Second),
		"minute":      int64(time.Minute),
		"hour":        int64(time.Hour),
		"day":         int64(24 * time.Hour),
	}
	capacities := []int64{10, 1_000, 20_000}
	refills := []int64{1, 10, 999}
	elapsedSpans := []int64{1, 1_000, int64(time.Second), int64(time.Minute), int64(time.Hour)}

	for name, interval := range intervals {
		for _, capacity := range capacities {
			for _, refill := range refills {
				for _, elapsed := range elapsedSpans {
					bucket := NewTokenBucket("b", capacity, refill, interval)
					bucket.Admit("scope", capacity, 0) // drain
					// Tokens genuinely earned over the span, floored.
					want := elapsed / interval * refill
					if part := elapsed % interval * refill / interval; part > 0 {
						want += part
					}
					if want > capacity {
						want = capacity
					}
					got := largestAdmissible(bucket, "scope", capacity, elapsed)
					if got != want {
						t.Fatalf("interval=%s capacity=%d refill=%d elapsed=%d: admitted %d, earned %d",
							name, capacity, refill, elapsed, got, want)
					}
				}
			}
		}
	}
}

// largestAdmissible binary-searches the biggest cost the bucket will accept,
// which is what it believes it has accrued.
func largestAdmissible(bucket *TokenBucket, scope string, capacity, now int64) int64 {
	low, high := int64(0), capacity
	for low < high {
		mid := (low + high + 1) / 2
		if bucket.Would(scope, mid, now).Allowed {
			low = mid
		} else {
			high = mid - 1
		}
	}
	return low
}

// A wait must be the true wait at any interval, not a saturated placeholder
// that idles a client for centuries.
func TestRetryAfterIsTheTrueWaitAtLongIntervals(t *testing.T) {
	bucket := NewTokenBucket("y", 1_000, 10, int64(time.Hour))
	bucket.Admit("scope", 1_000, 0)

	decision := bucket.Admit("scope", 5, 0)
	if decision.Allowed {
		t.Fatal("drained bucket admitted a request")
	}
	// Five tokens at ten an hour is half an hour.
	if want := int64(30 * time.Minute); decision.RetryAfter != want {
		t.Fatalf("retry-after = %d, want %d", decision.RetryAfter, want)
	}
}

// A gate hands out slots that are outstanding concurrently in time. Parking the
// most recent one in a field on the gate loses all but the last.
func TestEveryAdmitReturnsItsOwnSlot(t *testing.T) {
	queue := NewAdmissionQueue(AdmissionConfig{PriorityDepth: Depth(4), SecondaryDepth: Depth(4)})
	gate := NewGate(BinanceSpotLike(), queue)

	_, firstSlot := gate.Admit("acct", KindCancelOrder, 0)
	_, secondSlot := gate.Admit("acct", KindPlaceOrder, 0)
	if priority, secondary := queue.Depth(); priority != 1 || secondary != 1 {
		t.Fatalf("depths = (%d, %d), want (1, 1)", priority, secondary)
	}

	queue.Complete(firstSlot)
	queue.Complete(secondSlot)
	if priority, secondary := queue.Depth(); priority != 0 || secondary != 0 {
		t.Fatalf("depths = (%d, %d) after releasing both, want (0, 0)", priority, secondary)
	}
}

// A slot is a receipt for one unit of work. Completing it twice must not
// manufacture capacity.
func TestASlotCannotBeReplayed(t *testing.T) {
	queue := NewAdmissionQueue(AdmissionConfig{SecondaryDepth: Depth(2)})
	_, slot := queue.Offer(KindPlaceOrder)
	queue.Offer(KindPlaceOrder)

	queue.Complete(slot)
	queue.Complete(slot)
	if _, secondary := queue.Depth(); secondary != 1 {
		t.Fatalf("secondary depth = %d after replaying one release, want 1", secondary)
	}
}

// A limiter that is a value type holding a map is legal Go and satisfies the
// interface. The gate must not panic comparing it.
type uncomparableLimiter struct {
	name string
	seen map[string]int64
}

func (u uncomparableLimiter) Name() string { return u.name }

func (u uncomparableLimiter) Would(string, int64, int64) Decision { return Allow() }

func (u uncomparableLimiter) Admit(scope string, cost, _ int64) Decision {
	u.seen[scope] += cost
	return Allow()
}

func TestAnUncomparableLimiterDoesNotPanic(t *testing.T) {
	limiter := uncomparableLimiter{name: "custom", seen: map[string]int64{}}
	gate := NewGate([]Meter{{Limiter: limiter, Cost: StaticCost{Default: 1}}}, nil)
	if decision, _ := gate.Admit("acct", KindPlaceOrder, 0); !decision.Allowed {
		t.Fatalf("uncomparable limiter refused: %+v", decision)
	}
}
