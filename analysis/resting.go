package analysis

import "sort"

// RestingPlacement describes where each participant class puts its resting
// orders, measured from the midpoint standing when the order arrived.
//
// It exists to identify what sets the book's width. The spacing of the makers'
// own quotes is governed by their inventory skew, but the outer envelope of the
// book proved invariant to a large manipulation of that skew, so something else
// puts orders out there. Nothing else in this analysis attributes depth to an
// owner: a book delta carries a price and a size and no identity.
type RestingPlacement struct {
	ByRole map[string]*PlacementStats `json:"by_role"`
	// Unattributed counts accepted orders whose client has no known role.
	Unattributed int `json:"unattributed"`
	// Marketable counts orders priced through the opposing touch, which are
	// taking liquidity rather than resting it and are excluded.
	Marketable int         `json:"marketable"`
	Drift      ReplayDrift `json:"drift"`
}

// PlacementStats summarises one class's resting behaviour.
type PlacementStats struct {
	Orders int `json:"orders"`
	// DistanceTicks is how far from the midpoint the order rested, positive
	// away from the mid on the order's own side, so bids and asks are directly
	// comparable.
	DistanceTicks Distribution `json:"distance_ticks"`
	// Qty is the resting size in base units.
	Qty Distribution `json:"qty"`

	distances []float64
	qtys      []float64
}

// RestingOptions configures the measurement.
type RestingOptions struct {
	TickSize int64
	// Role resolves a client to its participant class.
	Role func(clientID uint64) string
}

// MeasureRestingPlacement replays a book and attributes each resting order.
func MeasureRestingPlacement(path string, opts RestingOptions) (*RestingPlacement, error) {
	if opts.TickSize <= 0 {
		opts.TickSize = 1
	}
	result := &RestingPlacement{ByRole: map[string]*PlacementStats{}}

	drift, err := ReplayFileWith(path, nil, func(_ int64, accepted acceptedPayload, book *ReplayedBook) {
		if accepted.Qty <= 0 {
			return
		}
		mid, midOK := book.Mid()
		if !midOK {
			return
		}
		// Distance is signed away from the mid on the order's own side, so a
		// bid ten ticks below and an ask ten ticks above both read as ten.
		var distance float64
		if accepted.Side == "BUY" {
			distance = float64(mid) - float64(accepted.Price)
		} else {
			distance = float64(accepted.Price) - float64(mid)
		}
		if distance < 0 {
			// Priced through the mid: this order is crossing, not resting.
			result.Marketable++
			return
		}
		role := ""
		if opts.Role != nil {
			role = opts.Role(accepted.ClientID)
		}
		if role == "" {
			result.Unattributed++
			return
		}
		stats := result.ByRole[role]
		if stats == nil {
			stats = &PlacementStats{}
			result.ByRole[role] = stats
		}
		stats.Orders++
		stats.distances = append(stats.distances, distance/float64(opts.TickSize))
		stats.qtys = append(stats.qtys, float64(accepted.Qty))
	})
	if err != nil {
		return nil, err
	}
	for _, stats := range result.ByRole {
		stats.DistanceTicks = Describe(stats.distances)
		stats.Qty = Describe(stats.qtys)
		stats.distances, stats.qtys = nil, nil
	}
	result.Drift = *drift
	return result, nil
}

// RolesByDistance lists the classes furthest from the touch first, which is
// the order in which they set the book's outer width.
func (r *RestingPlacement) RolesByDistance() []string {
	names := make([]string, 0, len(r.ByRole))
	for name := range r.ByRole {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		return r.ByRole[names[i]].DistanceTicks.Median > r.ByRole[names[j]].DistanceTicks.Median
	})
	return names
}
