package ratelimit

import "time"

// Schemes assembled from what exchanges publish. They are ordinary
// compositions of the parts in this package, not special cases: a venue with a
// different schedule is a different composition, written by the caller.

// BinanceSpotLike is the documented spot scheme: a request-weight budget per
// minute and an order-count budget per ten seconds and per day, all scoped to
// the connection's address rather than the account.
//
// Weights differ per endpoint; the values here are the documented order of
// magnitude rather than a full endpoint table, which belongs to the caller.
func BinanceSpotLike() []Meter {
	weight := StaticCost{
		KindPlaceOrder:      1,
		KindPlaceReduceOnly: 1,
		KindCancelOrder:     1,
		KindCancelAll:       1,
		KindQueryOrder:      4,
		KindQueryBalance:    20,
		KindQueryAccount:    20,
		KindSubscribe:       2,
		KindUnsubscribe:     2,
		DefaultCost:         1,
	}
	// Only placements count against the order budgets. Cancels deliberately do
	// not, so a client throttled on new orders can still withdraw the ones it
	// has: refusing that would trap a client in its own position.
	placements := StaticCost{
		KindPlaceOrder:      1,
		KindPlaceReduceOnly: 1,
		DefaultCost:         0,
	}
	return []Meter{
		{Limiter: NewFixedWindow("request_weight_1m", 6000, int64(time.Minute)), Cost: weight},
		{Limiter: NewFixedWindow("orders_10s", 100, int64(10*time.Second)), Cost: placements},
		{Limiter: NewFixedWindow("orders_1d", 200_000, int64(24*time.Hour)), Cost: placements},
	}
}

// SmoothedBudget is the other published shape: one continuously refilling
// allowance rather than a quota that resets on a boundary. A client that paces
// itself is never refused, where a fixed window lets it spend everything in the
// first instant and wait out the rest.
func SmoothedBudget(name string, capacity, perSecond int64, cost CostModel) Meter {
	return Meter{
		Limiter: NewTokenBucket(name, capacity, perSecond, int64(time.Second)),
		Cost:    cost,
	}
}

// StreamLimits are the documented websocket constraints: a cap on streams per
// connection, and a cap on control messages a connection may send per second.
// They are separate from the request budgets because they are enforced per
// connection rather than per account or address.
type StreamLimits struct {
	MaxStreamsPerConnection int   `json:"max_streams_per_connection"`
	ControlMessagesPerSec   int64 `json:"control_messages_per_sec"`
}

// BinanceStreamLike reflects the published figures: 1024 streams on one
// connection, and five control messages a second before the connection is cut.
func BinanceStreamLike() StreamLimits {
	return StreamLimits{MaxStreamsPerConnection: 1024, ControlMessagesPerSec: 5}
}
