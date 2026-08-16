package ratelimit

// RequestKind classifies what a client is asking the venue to do. Cost and
// queue lane both follow from it, which is why it is one type rather than a
// weight table and a separate priority flag that can disagree.
type RequestKind uint8

const (
	KindUnknown RequestKind = iota
	KindPlaceOrder
	// KindPlaceReduceOnly is an order that can only shrink a position. Venues
	// treat it as risk-reducing even though it is a placement.
	KindPlaceReduceOnly
	KindCancelOrder
	KindCancelAll
	KindQueryBalance
	KindQueryAccount
	KindQueryOrder
	KindSubscribe
	KindUnsubscribe
)

// RiskReducing reports whether a request can only reduce the client's exposure.
// These take the priority lane, so a saturated venue can refuse new risk while
// still letting clients out of what they already hold. Refusing a cancel under
// load is what turns a busy venue into an unsafe one.
func (k RequestKind) RiskReducing() bool {
	switch k {
	case KindCancelOrder, KindCancelAll, KindPlaceReduceOnly:
		return true
	default:
		return false
	}
}

func (k RequestKind) String() string {
	switch k {
	case KindPlaceOrder:
		return "place_order"
	case KindPlaceReduceOnly:
		return "place_reduce_only"
	case KindCancelOrder:
		return "cancel_order"
	case KindCancelAll:
		return "cancel_all"
	case KindQueryBalance:
		return "query_balance"
	case KindQueryAccount:
		return "query_account"
	case KindQueryOrder:
		return "query_order"
	case KindSubscribe:
		return "subscribe"
	case KindUnsubscribe:
		return "unsubscribe"
	default:
		return "unknown"
	}
}

// CostModel prices a request. Weight-based venues charge more for expensive
// reads than for a placement, which a flat per-request count cannot express.
type CostModel interface {
	Cost(RequestKind) int64
}

// StaticCost is a table with an explicit fallback. The fallback is a field
// rather than a reserved key, because RequestKind is a uint8 that callers
// extend with their own constants: any sentinel value inside the map would sit
// in the space they extend into and could collide silently.
type StaticCost struct {
	Table map[RequestKind]int64
	// Default is charged for kinds absent from the table, including zero, which
	// is how a venue expresses that a request costs nothing against a budget.
	Default int64
}

func (c StaticCost) Cost(kind RequestKind) int64 {
	if cost, ok := c.Table[kind]; ok {
		return cost
	}
	return c.Default
}

// AdmissionConfig sizes the two lanes and decides which requests are
// risk-reducing.
//
// Depths are pointers so an absent field is distinguishable from a configured
// zero. Nil means unlimited; a configured zero means the lane is closed, which
// is what a venue shedding all new load would do. A plain int could not express
// both, and silently reading a missing field as unlimited would disable the
// overload modelling a run was built to study.
type AdmissionConfig struct {
	PriorityDepth  *int `json:"priority_depth"`
	SecondaryDepth *int `json:"secondary_depth"`
	// RiskReducing overrides which kinds take the priority lane. A caller with
	// its own request kinds needs this: the built-in classification cannot know
	// about them, and requiring an edit here to add one would make the package
	// closed to extension.
	RiskReducing func(RequestKind) bool `json:"-"`
}

// AdmissionQueue models the venue's own execution backlog. Splitting it in two
// is what lets a venue reject new orders with a clear reason while continuing
// to accept cancels: the lane carrying new risk saturates first, by design.
type AdmissionQueue struct {
	cfg       AdmissionConfig
	priority  int
	secondary int
}

func NewAdmissionQueue(cfg AdmissionConfig) *AdmissionQueue {
	if cfg.RiskReducing == nil {
		cfg.RiskReducing = RequestKind.RiskReducing
	}
	return &AdmissionQueue{cfg: cfg}
}

// Unlimited is a convenience for a lane with no bound.
func Unlimited() *int { return nil }

// Depth sizes a lane explicitly, including zero to close it.
func Depth(n int) *int { return &n }

func (q *AdmissionQueue) full(depth *int, occupied int) bool {
	return depth != nil && occupied >= *depth
}

// Slot is what Offer hands out and Complete takes back. It records the lane the
// request actually entered, so a caller cannot return a slot to the wrong lane
// by passing a different kind than it offered — a mismatch would permanently
// lose a slot in one lane and manufacture one in the other, silently, for the
// rest of the run.
type Slot struct {
	priority bool
	held     bool
}

// Offer asks for a slot in the lane the request belongs to.
func (q *AdmissionQueue) Offer(kind RequestKind) (Decision, Slot) {
	if q.cfg.RiskReducing(kind) {
		if q.full(q.cfg.PriorityDepth, q.priority) {
			return Decision{Limit: "queue_priority", Overloaded: true}, Slot{}
		}
		q.priority++
		return Allow(), Slot{priority: true, held: true}
	}
	if q.full(q.cfg.SecondaryDepth, q.secondary) {
		return Decision{Limit: "queue_secondary", Overloaded: true}, Slot{}
	}
	q.secondary++
	return Allow(), Slot{held: true}
}

// Complete releases a slot once the engine has finished the work. Releasing a
// slot that was never held does nothing.
func (q *AdmissionQueue) Complete(slot Slot) {
	if !slot.held {
		return
	}
	if slot.priority {
		if q.priority > 0 {
			q.priority--
		}
		return
	}
	if q.secondary > 0 {
		q.secondary--
	}
}

// Depth reports the current backlog in each lane, for telemetry.
func (q *AdmissionQueue) Depth() (priority, secondary int) {
	return q.priority, q.secondary
}

// Meter pairs a budget with the cost model that charges it. Budgets are
// denominated in different currencies: a venue's weight budget and its order
// count budget both see the same placement, and charge it 10 and 1. One cost
// per request cannot express that, so each budget carries its own.
type Meter struct {
	Limiter Limiter
	Cost    CostModel
}

// Gate composes metered budgets and an optional queue into the single decision
// a venue makes when a request arrives.
type Gate struct {
	meters   []Meter
	queue    *AdmissionQueue
	lastSlot Slot
}

// Queue exposes the backlog so a caller that admitted through the gate can
// release the slot it took. Without this the slot would be unreachable.
func (g *Gate) Queue() *AdmissionQueue { return g.queue }

// LastSlot is the slot taken by the most recent successful Admit.
func (g *Gate) LastSlot() Slot { return g.lastSlot }

func NewGate(meters []Meter, queue *AdmissionQueue) *Gate {
	return &Gate{meters: meters, queue: queue}
}

// Admit charges every budget and takes a queue slot, or reports the first
// refusal without charging anything.
//
// Costs are summed per limiter before probing, because two meters may share one
// limiter and probing them separately would let the pair overspend a budget
// neither exceeds alone. The venue's own queue is checked before any budget is
// charged: an overloaded engine never saw the request, so the client should not
// pay for it.
func (g *Gate) Admit(scope string, kind RequestKind, now int64) Decision {
	type charge struct {
		limiter Limiter
		cost    int64
	}
	charges := make([]charge, 0, len(g.meters))
	for _, meter := range g.meters {
		cost := meter.Cost.Cost(kind)
		existing := -1
		for i := range charges {
			if charges[i].limiter == meter.Limiter {
				existing = i
				break
			}
		}
		if existing >= 0 {
			charges[existing].cost += cost
			continue
		}
		charges = append(charges, charge{limiter: meter.Limiter, cost: cost})
	}

	for _, c := range charges {
		if decision := g.probe(c.limiter, scope, c.cost, now); !decision.Allowed {
			return decision
		}
	}
	if g.queue != nil {
		decision, slot := g.queue.Offer(kind)
		if !decision.Allowed {
			return decision
		}
		g.lastSlot = slot
	}
	for _, c := range charges {
		c.limiter.Admit(scope, c.cost, now)
	}
	return Allow()
}

// probe asks a limiter whether a cost would fit without charging it, so a
// request refused by one budget does not consume another.
func (g *Gate) probe(limiter Limiter, scope string, cost, now int64) Decision {
	if dry, ok := limiter.(dryRunLimiter); ok {
		return dry.Would(scope, cost, now)
	}
	return limiter.Admit(scope, cost, now)
}

// dryRunLimiter is implemented by limiters that can answer without charging.
type dryRunLimiter interface {
	Would(scope string, cost, now int64) Decision
}
