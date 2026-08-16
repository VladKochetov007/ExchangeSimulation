package ratelimit

import (
	"math"
	"math/bits"
)

// Decision is the outcome of one admission check. A rejection carries enough to
// answer the client honestly: which limit stopped it, and when it could succeed.
type Decision struct {
	Allowed bool
	// Limit names the budget that rejected the request, empty when allowed.
	Limit string
	// RetryAfter is nanoseconds until the request could succeed unchanged.
	RetryAfter int64
	// Overloaded marks a rejection caused by the venue's own backlog rather
	// than by the client's budget. It is a different apology: the client did
	// nothing wrong and slowing down will not necessarily help.
	Overloaded bool
	// Impossible marks a request that cannot ever fit this budget, so a caller
	// retries forever otherwise. A venue should reject it rather than queue it.
	Impossible bool
}

// Allow is the decision for a request that fits.
func Allow() Decision { return Decision{Allowed: true} }

// Limiter decides whether a cost fits a scope's budget at a moment in time.
// Time is passed in rather than read so a simulation stays deterministic.
type Limiter interface {
	// Admit charges cost to scope when it fits, and reports what happened.
	Admit(scope string, cost int64, now int64) Decision
	// Would reports what Admit would decide, without charging. A gate composing
	// several budgets needs this so a request refused by one budget does not
	// consume another. It is part of the contract rather than an optional
	// extra: a limiter that cannot answer without charging would be charged
	// twice per request, once to ask and once to commit.
	Would(scope string, cost int64, now int64) Decision
	// Name identifies the budget in rejections and telemetry.
	Name() string
}

// FixedWindow resets its budget when an interval boundary passes, which is the
// scheme published for request weight and order counts: a quota per interval
// rather than a continuously refilling allowance.
type FixedWindow struct {
	name     string
	budget   int64
	interval int64
	used     map[string]int64
	windowAt map[string]int64
}

// NewFixedWindow builds an interval quota. A non-positive interval is treated
// as a window that never rolls over: the budget is spent once and never
// refreshed. The alternative, computing a window boundary from a zero interval,
// makes every distinct timestamp its own window and silently removes the limit.
func NewFixedWindow(name string, budget, interval int64) *FixedWindow {
	if budget < 0 {
		budget = 0
	}
	return &FixedWindow{
		name: name, budget: budget, interval: interval,
		used: make(map[string]int64), windowAt: make(map[string]int64),
	}
}

func (w *FixedWindow) Name() string { return w.name }

func (w *FixedWindow) Admit(scope string, cost, now int64) Decision {
	if cost <= 0 {
		return Allow()
	}
	if cost > w.budget {
		return Decision{Limit: w.name, Impossible: true}
	}
	// Windows are anchored to the epoch so every scope rolls over together,
	// which is what makes published reset times meaningful to a client.
	current := w.windowStart(now)
	// Only a later window resets usage. A replayed or reordered timestamp from
	// an earlier window must not clear what the current one has spent, or a
	// client could reclaim its budget by sending an old timestamp.
	if stored, seen := w.windowAt[scope]; !seen || current > stored {
		w.windowAt[scope] = current
		w.used[scope] = 0
	} else if current < stored {
		current = stored
	}
	if w.used[scope]+cost > w.budget {
		if w.interval <= 0 {
			return Decision{Limit: w.name, Impossible: true}
		}
		return Decision{Limit: w.name, RetryAfter: current + w.interval - now}
	}
	w.used[scope] += cost
	return Allow()
}

// Used reports the charge accumulated in the current window, which is what a
// venue publishes back to the client in a used-weight header.
func (w *FixedWindow) Used(scope string, now int64) int64 {
	if w.windowAt[scope] != w.windowStart(now) {
		return 0
	}
	return w.used[scope]
}

// TokenBucket refills continuously, smoothing bursts instead of resetting on a
// boundary. Tokens are held scaled so a refill rate below one token per
// nanosecond does not truncate to nothing.
type TokenBucket struct {
	name       string
	capacity   int64
	refill     int64 // tokens per interval
	interval   int64
	tokens     map[string]int64 // scaled by tokenScale
	lastFillAt map[string]int64
}

// tokenScale keeps fractional refill exact in integer arithmetic. Capacity is
// clamped so the scaled arithmetic below cannot overflow: a budget larger than
// this is meaningless anyway, since no venue meters in quintillions.
const tokenScale = 1_000_000

const maxScalableTokens = math.MaxInt64 / tokenScale

func NewTokenBucket(name string, capacity, refill, interval int64) *TokenBucket {
	if capacity > maxScalableTokens {
		capacity = maxScalableTokens
	}
	if capacity < 0 {
		capacity = 0
	}
	if refill > maxScalableTokens {
		refill = maxScalableTokens
	}
	if refill < 0 {
		refill = 0
	}
	return &TokenBucket{
		name: name, capacity: capacity, refill: refill, interval: interval,
		tokens: make(map[string]int64), lastFillAt: make(map[string]int64),
	}
}

func (b *TokenBucket) Name() string { return b.name }

func (b *TokenBucket) Admit(scope string, cost, now int64) Decision {
	if cost <= 0 {
		return Allow()
	}
	if cost > b.capacity {
		return Decision{Limit: b.name, Impossible: true}
	}
	b.fill(scope, now)
	want := cost * tokenScale
	if b.tokens[scope] < want {
		if !b.refillable() {
			return Decision{Limit: b.name, Impossible: true}
		}
		return Decision{Limit: b.name, RetryAfter: b.timeToAccrue(want - b.tokens[scope])}
	}
	b.tokens[scope] -= want
	return Allow()
}

func (b *TokenBucket) fill(scope string, now int64) {
	last, seen := b.lastFillAt[scope]
	if !seen {
		b.tokens[scope] = b.capacity * tokenScale
		b.lastFillAt[scope] = now
		return
	}
	if now <= last {
		return
	}
	if b.interval > 0 && b.refill > 0 {
		b.tokens[scope] = b.accrue(b.tokens[scope], now-last)
	}
	b.lastFillAt[scope] = now
}

// accrue adds the tokens earned over an elapsed span. Whole intervals and the
// leftover fraction are counted separately so a long span never has to be
// multiplied out in one piece, and every product is checked before it is taken
// rather than approximated: rounding the span down to whole intervals would let
// a bucket grant a full capacity it had not yet earned.
func (b *TokenBucket) accrue(tokens, elapsed int64) int64 {
	capped := b.capacity * tokenScale
	if elapsed <= 0 || b.interval <= 0 || b.refill <= 0 || tokens >= capped {
		return min64(tokens, capped)
	}
	need := capped - tokens
	perInterval := b.refill * tokenScale

	gained := int64(0)
	if whole := elapsed / b.interval; whole > 0 {
		// An unrepresentable product means the true gain exceeds anything the
		// bucket can hold, since need is bounded by capacity. Returning a full
		// bucket is the arithmetic answer here, not a lenient shortcut.
		if perInterval > 0 && whole > math.MaxInt64/perInterval {
			return capped
		}
		gained = whole * perInterval
	}
	if rem := elapsed % b.interval; rem > 0 {
		// rem is below interval, so rem*perInterval/interval is below
		// perInterval and always representable. The product alone may not be,
		// so it is taken in two words: rounding this branch up to a full bucket
		// would be the largest possible over-grant, not a safe approximation.
		part := mulDiv(rem, perInterval, b.interval)
		if gained > math.MaxInt64-part {
			return capped
		}
		gained += part
	}
	if gained >= need {
		return capped
	}
	return tokens + gained
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// timeToAccrue is the wait until a shortfall has been earned, rounded up so a
// caller obeying it is not sent back a nanosecond early. Division precedes
// multiplication so a large shortfall does not wrap.
func (b *TokenBucket) timeToAccrue(shortfall int64) int64 {
	if b.refill <= 0 || b.interval <= 0 {
		return 0
	}
	denominator := b.refill * tokenScale
	whole := shortfall / denominator
	remainder := shortfall % denominator

	if whole > 0 && whole > math.MaxInt64/b.interval {
		return math.MaxInt64
	}
	wait := whole * b.interval
	if remainder > 0 {
		// remainder is below denominator, so the quotient is below interval and
		// representable even where remainder*interval is not. Rounded up so a
		// caller obeying the wait is never sent back early.
		wait += mulDivCeil(remainder, b.interval, denominator)
	}
	return wait
}

// refillable reports whether the bucket can ever earn tokens back. One that
// cannot must refuse with Impossible rather than promise a retry that will fail
// identically forever, which would spin a caller obeying RetryAfter.
func (b *TokenBucket) refillable() bool { return b.refill > 0 && b.interval > 0 }

// windowStart is the boundary of the window containing now. A non-positive
// interval has one window covering all time, so every timestamp maps to the
// same start and the budget never refreshes.
func (w *FixedWindow) windowStart(now int64) int64 {
	if w.interval <= 0 {
		return 0
	}
	return now - modFloor(now, w.interval)
}

// mulDiv computes a*b/divisor exactly, using a double-width product so a
// quotient that fits is never lost to an intermediate that does not. Callers
// must know the quotient fits, which is true wherever a < divisor.
func mulDiv(a, b, divisor int64) int64 {
	if divisor <= 0 || a <= 0 || b <= 0 {
		return 0
	}
	hi, lo := bits.Mul64(uint64(a), uint64(b))
	if hi >= uint64(divisor) {
		// The quotient would overflow; callers avoid this, but never wrap.
		return math.MaxInt64
	}
	quotient, _ := bits.Div64(hi, lo, uint64(divisor))
	if quotient > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(quotient)
}

// mulDivCeil is mulDiv rounded up.
func mulDivCeil(a, b, divisor int64) int64 {
	if divisor <= 0 || a <= 0 || b <= 0 {
		return 0
	}
	hi, lo := bits.Mul64(uint64(a), uint64(b))
	if hi >= uint64(divisor) {
		return math.MaxInt64
	}
	quotient, remainder := bits.Div64(hi, lo, uint64(divisor))
	if remainder > 0 {
		quotient++
	}
	if quotient > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(quotient)
}

// modFloor is a modulo that floors toward negative infinity, so window
// boundaries stay aligned for timestamps before the epoch.
func modFloor(value, step int64) int64 {
	if step <= 0 {
		return 0
	}
	remainder := value % step
	if remainder < 0 {
		remainder += step
	}
	return remainder
}

// Would reports what Admit would decide, without charging. A gate composing
// several budgets uses this so a request refused by one budget does not consume
// another.
func (w *FixedWindow) Would(scope string, cost, now int64) Decision {
	if cost <= 0 {
		return Allow()
	}
	if cost > w.budget {
		return Decision{Limit: w.name, Impossible: true}
	}
	current := w.windowStart(now)
	used := int64(0)
	if stored, seen := w.windowAt[scope]; seen && current <= stored {
		used = w.used[scope]
		current = stored
	}
	if used+cost > w.budget {
		if w.interval <= 0 {
			return Decision{Limit: w.name, Impossible: true}
		}
		return Decision{Limit: w.name, RetryAfter: current + w.interval - now}
	}
	return Allow()
}

// Would reports what Admit would decide, without consuming tokens.
func (b *TokenBucket) Would(scope string, cost, now int64) Decision {
	if cost <= 0 {
		return Allow()
	}
	if cost > b.capacity {
		return Decision{Limit: b.name, Impossible: true}
	}
	available := b.capacity * tokenScale
	if last, seen := b.lastFillAt[scope]; seen {
		available = b.tokens[scope]
		if now > last && b.interval > 0 && b.refill > 0 {
			available = b.accrue(available, now-last)
		}
	}
	if want := cost * tokenScale; available < want {
		if !b.refillable() {
			return Decision{Limit: b.name, Impossible: true}
		}
		return Decision{Limit: b.name, RetryAfter: b.timeToAccrue(want - available)}
	}
	return Allow()
}
