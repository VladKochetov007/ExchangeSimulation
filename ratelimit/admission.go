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

// StaticCost is a table with a fallback, which covers every published schedule
// this package has needed so far.
type StaticCost map[RequestKind]int64

// DefaultCost is the key used for kinds absent from the table. It is outside
// the RequestKind range so it can never collide with a real kind.
const DefaultCost RequestKind = 255

func (c StaticCost) Cost(kind RequestKind) int64 {
	if cost, ok := c[kind]; ok {
		return cost
	}
	if cost, ok := c[DefaultCost]; ok {
		return cost
	}
	return 1
}

// AdmissionConfig sizes the two lanes. A depth of zero means unlimited, so a
// venue that never overloads needs no configuration.
type AdmissionConfig struct {
	PriorityDepth  int `json:"priority_depth"`
	SecondaryDepth int `json:"secondary_depth"`
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
	return &AdmissionQueue{cfg: cfg}
}

// Offer asks for a slot in the lane the request belongs to.
func (q *AdmissionQueue) Offer(kind RequestKind) Decision {
	if kind.RiskReducing() {
		if q.cfg.PriorityDepth > 0 && q.priority >= q.cfg.PriorityDepth {
			return Decision{Limit: "queue_priority", Overloaded: true}
		}
		q.priority++
		return Allow()
	}
	if q.cfg.SecondaryDepth > 0 && q.secondary >= q.cfg.SecondaryDepth {
		return Decision{Limit: "queue_secondary", Overloaded: true}
	}
	q.secondary++
	return Allow()
}

// Complete releases a slot once the engine has finished the work.
func (q *AdmissionQueue) Complete(kind RequestKind) {
	if kind.RiskReducing() {
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
	meters []Meter
	queue  *AdmissionQueue
}

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
		if decision := g.queue.Offer(kind); !decision.Allowed {
			return decision
		}
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
