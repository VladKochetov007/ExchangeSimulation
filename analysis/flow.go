package analysis

import "sync"

// NetFlow is one participant class's traded quantity on one book.
//
// Signed net is what every trade has a counterparty for, so summing it over all
// classes must give zero. That identity is the only reliable check that the
// quantity being summed is the right one: an OrderFill carries both Qty, the
// quantity of that execution, and FilledQty, the order's cumulative filled
// quantity to date. Summing the second across an order's partial fills counts
// the early fills once per later fill, which inflates active classes, breaks
// the identity, and looks like an engine defect. It is not.
type NetFlow struct {
	Bought int64
	Sold   int64
}

// Net is bought less sold.
func (f NetFlow) Net() int64 { return f.Bought - f.Sold }

// Gross is total traded quantity.
func (f NetFlow) Gross() int64 { return f.Bought + f.Sold }

// Imbalance is net over gross, the share of a class's trading that is one-sided.
func (f NetFlow) Imbalance() float64 {
	if f.Gross() == 0 {
		return 0
	}
	return float64(f.Net()) / float64(f.Gross())
}

type fillPayload struct {
	Qty       int64  `json:"qty"`
	FilledQty int64  `json:"filled_qty"`
	Side      string `json:"side"`
	Role      string `json:"role"`
}

// NetFlowByRole totals signed traded quantity per participant class for one
// book, and reports the residual that must be zero.
//
// A non-zero residual means the sum is not counting each trade once per side,
// and every figure derived from it should be discarded rather than explained.
func (r *Run) NetFlowByRole(venueID, symbol string) (map[string]*NetFlow, int64, error) {
	files := r.BookFiles(venueID, symbol)
	var mu sync.Mutex
	table := map[string]*NetFlow{}
	var residual int64
	err := r.Scan(ScanOptions{Events: []string{"OrderFill"}, Files: files, FilesSelected: true}, func(event Event) {
		var payload fillPayload
		if event.Decode(&payload) != nil || payload.Qty <= 0 {
			return
		}
		role := r.Role(event.VenueID, event.ClientID)
		if role == "" {
			role = "unmapped"
		}
		mu.Lock()
		defer mu.Unlock()
		flow, ok := table[role]
		if !ok {
			flow = &NetFlow{}
			table[role] = flow
		}
		if payload.Side == "BUY" {
			flow.Bought += payload.Qty
			residual += payload.Qty
		} else {
			flow.Sold += payload.Qty
			residual -= payload.Qty
		}
	})
	return table, residual, err
}
