package ratelimit

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

func NewFixedWindow(name string, budget, interval int64) *FixedWindow {
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
	current := now - modFloor(now, w.interval)
	if w.windowAt[scope] != current {
		w.windowAt[scope] = current
		w.used[scope] = 0
	}
	if w.used[scope]+cost > w.budget {
		return Decision{Limit: w.name, RetryAfter: current + w.interval - now}
	}
	w.used[scope] += cost
	return Allow()
}

// Used reports the charge accumulated in the current window, which is what a
// venue publishes back to the client in a used-weight header.
func (w *FixedWindow) Used(scope string, now int64) int64 {
	if w.windowAt[scope] != now-modFloor(now, w.interval) {
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

// tokenScale keeps fractional refill exact in integer arithmetic.
const tokenScale = 1_000_000

func NewTokenBucket(name string, capacity, refill, interval int64) *TokenBucket {
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
		gained := (now - last) * b.refill * tokenScale / b.interval
		if capped := b.capacity * tokenScale; b.tokens[scope]+gained > capped {
			b.tokens[scope] = capped
		} else {
			b.tokens[scope] += gained
		}
	}
	b.lastFillAt[scope] = now
}

func (b *TokenBucket) timeToAccrue(shortfall int64) int64 {
	if b.refill <= 0 {
		return 0
	}
	// Round up: reporting the instant before the token exists would send the
	// caller back a nanosecond early, to be rejected again.
	numerator := shortfall * b.interval
	denominator := b.refill * tokenScale
	return (numerator + denominator - 1) / denominator
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
	current := now - modFloor(now, w.interval)
	used := int64(0)
	if w.windowAt[scope] == current {
		used = w.used[scope]
	}
	if used+cost > w.budget {
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
			available += (now - last) * b.refill * tokenScale / b.interval
			if capped := b.capacity * tokenScale; available > capped {
				available = capped
			}
		}
	}
	if want := cost * tokenScale; available < want {
		return Decision{Limit: b.name, RetryAfter: b.timeToAccrue(want - available)}
	}
	return Allow()
}
