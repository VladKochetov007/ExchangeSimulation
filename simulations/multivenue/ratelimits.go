package multivenue

import (
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

	return exchange.NewTieredRequestPolicy(specs, func(clientID uint64) string {
		role := roleOf[clientID]
		for name, tier := range tiers {
			for _, prefix := range tier.Roles {
				if strings.HasPrefix(role, prefix) {
					return name
				}
			}
		}
		return ""
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
