package analysis

import "sync"

// RoleStats counts what one participant class did.
type RoleStats struct {
	Accepted   int64
	Fills      int64
	FilledQty  int64
	Rejected   int64
	Cancelled  int64
	IOCExpired int64
}

// Conversion is fills per accepted order, the share of a desk's orders that
// traded. A desk pricing at the touch converts poorly not because it is
// rejected but because the touch moves before it arrives, which is invisible
// unless the expiry of an unfilled immediate-or-cancel order is logged.
func (s RoleStats) Conversion() float64 {
	if s.Accepted == 0 {
		return 0
	}
	return float64(s.Fills) / float64(s.Accepted)
}

type orderPayload struct {
	TimeInForce string `json:"time_in_force"`
	Side        string `json:"side"`
	Price       int64  `json:"price"`
	Qty         int64  `json:"qty"`
	FilledQty   int64  `json:"filled_qty"`
	Role        string `json:"role"`
	Reason      string `json:"reason"`
	Error       string `json:"error"`
	OrderID     uint64 `json:"order_id"`
	Symbol      string `json:"symbol"`
}

// RoleTable counts orders, fills, rejections and immediate-or-cancel expiries
// for every participant class in the run.
func (r *Run) RoleTable() (map[string]*RoleStats, error) {
	var mu sync.Mutex
	table := map[string]*RoleStats{}
	entry := func(role string) *RoleStats {
		stats, ok := table[role]
		if !ok {
			stats = &RoleStats{}
			table[role] = stats
		}
		return stats
	}
	err := r.Scan(ScanOptions{Events: []string{"OrderAccepted", "OrderFill", "OrderRejected", "OrderCancelled"}}, func(event Event) {
		role := r.Role(event.VenueID, event.ClientID)
		if role == "" {
			return
		}
		var payload orderPayload
		if event.Decode(&payload) != nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		stats := entry(role)
		switch event.Name {
		case "OrderAccepted":
			stats.Accepted++
		case "OrderFill":
			stats.Fills++
			stats.FilledQty += payload.FilledQty
		case "OrderRejected":
			stats.Rejected++
		case "OrderCancelled":
			stats.Cancelled++
			if payload.Reason == "IOC_EXPIRED" {
				stats.IOCExpired++
			}
		}
	})
	return table, err
}
