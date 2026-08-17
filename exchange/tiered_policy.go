package exchange

import (
	"slices"
	"strconv"

	"exchange_sim/ratelimit"
)

// RequestTier is the budget a class of participant is given. Venues publish
// different allowances for different clients, so the tier is the unit a
// scenario configures rather than one global limit.
type RequestTier struct {
	// Meters are the budgets charged, each with its own cost model. An empty
	// slice meters nothing.
	Meters []ratelimit.Meter
	// Queue sizes this tier's share of the venue's execution backlog. A zero
	// value leaves the tier unqueued, so it can only be refused by a budget.
	Queue ratelimit.AdmissionConfig
	// Queued must be set for Queue to take effect, so a tier that deliberately
	// closes a lane with depth zero is distinguishable from one that never
	// configured a queue at all.
	Queued bool
}

// RetryAdvice travels with a rejection so a client can back off by the amount
// the venue actually wants, rather than guessing.
type RetryAdvice struct {
	RetryAfterNanos int64  `json:"retry_after_nanos"`
	Limit           string `json:"limit"`
}

// RequestStats records what a participant asked for and what the venue did
// about it. A payoff table alone cannot distinguish a strategy that declined to
// trade from one that was refused, so these counts are what make a rate-limit
// experiment readable.
type RequestStats struct {
	Admitted    int64                           `json:"admitted"`
	RateLimited int64                           `json:"rate_limited"`
	Overloaded  int64                           `json:"overloaded"`
	ByKind      map[ratelimit.RequestKind]int64 `json:"-"`
	// ByLimit counts refusals against the budget that bound, which says which
	// limit is actually shaping behaviour rather than merely being configured.
	ByLimit map[string]int64 `json:"by_limit,omitempty"`
}

// TieredRequestPolicy meters each participant against the tier it belongs to.
// Budgets and queues are per client, so one participant exhausting its
// allowance cannot refuse another's requests.
type TieredRequestPolicy struct {
	tiers    map[string]RequestTier
	classify func(clientID uint64) string
	gates    map[uint64]*ratelimit.Gate
	queues   map[uint64]*ratelimit.AdmissionQueue
	stats    map[uint64]*RequestStats
}

// NewTieredRequestPolicy builds a policy from named tiers and a classifier that
// says which tier a client belongs to. A client whose tier is not configured is
// left unmetered rather than refused: an unknown participant should not be
// silently unable to trade.
func NewTieredRequestPolicy(tiers map[string]RequestTier, classify func(clientID uint64) string) *TieredRequestPolicy {
	return &TieredRequestPolicy{
		tiers:    tiers,
		classify: classify,
		gates:    make(map[uint64]*ratelimit.Gate),
		queues:   make(map[uint64]*ratelimit.AdmissionQueue),
		stats:    make(map[uint64]*RequestStats),
	}
}

// Admit meters the request against the client's own gate.
func (p *TieredRequestPolicy) Admit(clientID uint64, kind ratelimit.RequestKind, now int64) (RequestPermit, Response, bool) {
	stats := p.statsFor(clientID)
	stats.ByKind[kind]++

	gate := p.gateFor(clientID)
	if gate == nil {
		stats.Admitted++
		return RequestPermit{}, Response{}, true
	}
	decision, slot := gate.Admit(scopeFor(clientID), kind, now)
	if decision.Allowed {
		stats.Admitted++
		return RequestPermit{Held: true, Slot: slot, ClientID: clientID}, Response{}, true
	}
	if decision.Overloaded {
		stats.Overloaded++
	} else {
		stats.RateLimited++
	}
	if decision.Limit != "" {
		stats.ByLimit[decision.Limit]++
	}
	return RequestPermit{}, rejectionFor(decision), false
}

// Release returns the queue slot a permitted request took.
func (p *TieredRequestPolicy) Release(permit RequestPermit) {
	if !permit.Held {
		return
	}
	if queue := p.queues[permit.ClientID]; queue != nil {
		queue.Complete(permit.Slot)
	}
}

// Stats reports what a participant asked for and how the venue answered.
func (p *TieredRequestPolicy) Stats(clientID uint64) RequestStats {
	return *p.statsFor(clientID)
}

// Clients lists every participant the policy has seen, so a run can report
// every metered class without knowing the roster in advance.
func (p *TieredRequestPolicy) Clients() []uint64 {
	ids := make([]uint64, 0, len(p.stats))
	for clientID := range p.stats {
		ids = append(ids, clientID)
	}
	slices.Sort(ids)
	return ids
}

func (p *TieredRequestPolicy) statsFor(clientID uint64) *RequestStats {
	stats, seen := p.stats[clientID]
	if !seen {
		stats = &RequestStats{
			ByKind:  make(map[ratelimit.RequestKind]int64),
			ByLimit: make(map[string]int64),
		}
		p.stats[clientID] = stats
	}
	return stats
}

// Depth reports a client's backlog, for telemetry.
func (p *TieredRequestPolicy) Depth(clientID uint64) (priority, secondary int) {
	if queue := p.queues[clientID]; queue != nil {
		return queue.Depth()
	}
	return 0, 0
}

func (p *TieredRequestPolicy) gateFor(clientID uint64) *ratelimit.Gate {
	if gate, built := p.gates[clientID]; built {
		return gate
	}
	name := ""
	if p.classify != nil {
		name = p.classify(clientID)
	}
	tier, configured := p.tiers[name]
	if !configured {
		p.gates[clientID] = nil
		return nil
	}
	var queue *ratelimit.AdmissionQueue
	if tier.Queued {
		queue = ratelimit.NewAdmissionQueue(tier.Queue)
		p.queues[clientID] = queue
	}
	gate := ratelimit.NewGate(tier.Meters, queue)
	p.gates[clientID] = gate
	return gate
}

// rejectionFor turns a refusal into the response the client sees. The two
// reasons say different things: a budget refusal is the client's own doing and
// waiting fixes it, while an overload is the venue's and waiting may not.
func rejectionFor(decision ratelimit.Decision) Response {
	reason := RejectRateLimited
	if decision.Overloaded {
		reason = RejectOverloaded
	}
	return Response{
		Success: false,
		Error:   reason,
		Data:    RetryAdvice{RetryAfterNanos: decision.RetryAfter, Limit: decision.Limit},
	}
}

// scopeFor keys a client's budgets. Formatting the id rather than converting it
// to a rune matters: a rune conversion collides above the Unicode range and
// would silently merge two participants' budgets.
func scopeFor(clientID uint64) string {
	return strconv.FormatUint(clientID, 10)
}
