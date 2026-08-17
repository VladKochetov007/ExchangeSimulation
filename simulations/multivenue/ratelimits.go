package multivenue

import (
	"sort"
	"strings"
	"time"

	"exchange_sim/exchange"
	"exchange_sim/ratelimit"
)

// RateLimitTier is the JSON shape a scenario uses to give a class of
// participant its own request budget. It is deliberately flatter than the
// ratelimit primitives: a scenario says how much a participant may spend, and
// this file assembles the meters.
type RateLimitTier struct {
	// Roles are matched as prefixes of a participant's role, so "spot_maker"
	// covers spot_maker_1 and spot_maker_2 without listing them.
	Roles []string `json:"roles"`
	// WeightPerMinute is the request-weight budget. Zero leaves it unmetered.
	WeightPerMinute int64 `json:"weight_per_minute"`
	// OrdersPer10s and OrdersPerDay bound placements only. Cancels are free
	// against them, so a throttled participant can still withdraw quotes.
	OrdersPer10s int64 `json:"orders_per_10s"`
	OrdersPerDay int64 `json:"orders_per_day"`
	// PriorityDepth and SecondaryDepth size this participant's share of the
	// venue's backlog. Nil leaves a lane unbounded.
	PriorityDepth  *int `json:"priority_depth"`
	SecondaryDepth *int `json:"secondary_depth"`
}

// buildRequestPolicy turns the scenario's tiers into a policy the exchange can
// consult. Participants whose role matches no tier are left unmetered, so a
// scenario can throttle one class without having to describe every other.
func buildRequestPolicy(tiers map[string]RateLimitTier, participants []Participant) *exchange.TieredRequestPolicy {
	if len(tiers) == 0 {
		return nil
	}
	roleOf := make(map[uint64]string, len(participants))
	for _, participant := range participants {
		roleOf[participant.ClientID] = participant.Role
	}

	specs := make(map[string]exchange.RequestTier, len(tiers))
	for name, tier := range tiers {
		specs[name] = exchange.RequestTier{
			Meters: metersFor(tier),
			Queue: ratelimit.AdmissionConfig{
				PriorityDepth:  tier.PriorityDepth,
				SecondaryDepth: tier.SecondaryDepth,
			},
			Queued: tier.PriorityDepth != nil || tier.SecondaryDepth != nil,
		}
	}

	// Tier names are visited in sorted order and the longest matching role
	// prefix wins. Ranging over the map directly would make assignment depend on
	// Go's randomised iteration order, so a participant matching two tiers would
	// get a different budget on different runs and a limit would appear to bind
	// on some seeds and not others. Preferring the longest prefix is what makes
	// a narrow tier able to override a broad one, which is why a scenario adds
	// one.
	names := make([]string, 0, len(tiers))
	for name := range tiers {
		names = append(names, name)
	}
	sort.Strings(names)

	return exchange.NewTieredRequestPolicy(specs, func(clientID uint64) string {
		role := roleOf[clientID]
		best, bestLen, bestBreadth := "", -1, 0
		for _, name := range names {
			tier := tiers[name]
			for _, prefix := range tier.Roles {
				if !strings.HasPrefix(role, prefix) {
					continue
				}
				// A longer prefix is more specific. Where two tiers name the
				// same role, the one covering fewer roles is the one written to
				// single that participant out, so it wins.
				better := len(prefix) > bestLen ||
					(len(prefix) == bestLen && len(tier.Roles) < bestBreadth)
				if best == "" || better {
					best, bestLen, bestBreadth = name, len(prefix), len(tier.Roles)
				}
			}
		}
		return best
	})
}

func metersFor(tier RateLimitTier) []ratelimit.Meter {
	weight := ratelimit.StaticCost{
		Table: map[ratelimit.RequestKind]int64{
			ratelimit.KindPlaceOrder:      1,
			ratelimit.KindPlaceReduceOnly: 1,
			ratelimit.KindCancelOrder:     1,
			ratelimit.KindQueryBalance:    20,
			ratelimit.KindQueryAccount:    20,
			ratelimit.KindSubscribe:       2,
		},
		Default: 1,
	}
	placements := ratelimit.StaticCost{
		Table: map[ratelimit.RequestKind]int64{
			ratelimit.KindPlaceOrder:      1,
			ratelimit.KindPlaceReduceOnly: 1,
		},
		Default: 0,
	}

	meters := make([]ratelimit.Meter, 0, 3)
	if tier.WeightPerMinute > 0 {
		meters = append(meters, ratelimit.Meter{
			Limiter: ratelimit.NewFixedWindow("request_weight_1m", tier.WeightPerMinute, int64(time.Minute)),
			Cost:    weight,
		})
	}
	if tier.OrdersPer10s > 0 {
		meters = append(meters, ratelimit.Meter{
			Limiter: ratelimit.NewFixedWindow("orders_10s", tier.OrdersPer10s, int64(10*time.Second)),
			Cost:    placements,
		})
	}
	if tier.OrdersPerDay > 0 {
		meters = append(meters, ratelimit.Meter{
			Limiter: ratelimit.NewFixedWindow("orders_1d", tier.OrdersPerDay, int64(24*time.Hour)),
			Cost:    placements,
		})
	}
	return meters
}

// RequestBudgetReport is what a run publishes about its request gating: per
// participant class, how many requests were admitted and how many refused, and
// which budget did the refusing. Without it a payoff table cannot distinguish a
// class that declined to trade from one the venue turned away.
type RequestBudgetReport struct {
	VenueID     string           `json:"venue_id"`
	Role        string           `json:"role"`
	Admitted    int64            `json:"admitted"`
	RateLimited int64            `json:"rate_limited"`
	Overloaded  int64            `json:"overloaded"`
	ByLimit     map[string]int64 `json:"by_limit,omitempty"`
}

// CaptureRequestBudgets summarises request gating per participant class.
func (s *Sim) CaptureRequestBudgets() []RequestBudgetReport {
	rows := make([]RequestBudgetReport, 0)
	for _, venue := range s.Venues {
		if venue.RequestPolicy == nil {
			continue
		}
		roleOf := make(map[uint64]string, len(venue.Participants))
		for _, participant := range venue.Participants {
			roleOf[participant.ClientID] = participant.Role
		}
		byClass := make(map[string]*RequestBudgetReport)
		classes := make([]string, 0)
		for _, clientID := range venue.RequestPolicy.Clients() {
			class := roleGroup(roleOf[clientID])
			row, seen := byClass[class]
			if !seen {
				row = &RequestBudgetReport{VenueID: venue.ID, Role: class, ByLimit: map[string]int64{}}
				byClass[class] = row
				classes = append(classes, class)
			}
			stats := venue.RequestPolicy.Stats(clientID)
			row.Admitted += stats.Admitted
			row.RateLimited += stats.RateLimited
			row.Overloaded += stats.Overloaded
			for limit, count := range stats.ByLimit {
				row.ByLimit[limit] += count
			}
		}
		sort.Strings(classes)
		for _, class := range classes {
			rows = append(rows, *byClass[class])
		}
	}
	return rows
}

// roleGroup collapses numbered participants of one class, matching how payoff
// reporting groups them so the two tables can be read side by side.
func roleGroup(role string) string {
	if index := strings.LastIndex(role, "_"); index > 0 {
		if suffix := role[index+1:]; suffix != "" && strings.Trim(suffix, "0123456789") == "" {
			return role[:index]
		}
	}
	return role
}

// requoteThresholdFor picks a maker's requote threshold. A scenario giving one
// value applies it to every maker, which makes the whole population requote in
// lockstep; giving several cycles them across makers so books go stale at
// different times.
func requoteThresholdFor(tiers []int64, fallback int64, makerIndex int) int64 {
	if len(tiers) == 0 {
		return fallback
	}
	return tiers[makerIndex%len(tiers)]
}
