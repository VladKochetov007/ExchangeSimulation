package analysis

import (
	"strings"
)

// PostOnlyActivityOptions selects the passive order classes and instruments
// whose venue-arrival behavior is being audited. Empty selectors include every
// persisted order event; callers should normally name the experimental maker
// roles to avoid attributing unrelated clients' orders to a treatment.
type PostOnlyActivityOptions struct {
	Roles   []string
	Symbols []string
}

// PostOnlyActivity separates accepted passive orders from explicit
// arrival-time rejections. A post-only order can later be filled by an
// incoming taker; PostOnlyFills measures that ordinary maker outcome and does
// not imply a post-only violation.
type PostOnlyActivity struct {
	Events              int64 `json:"events"`
	Accepted            int64 `json:"accepted"`
	AcceptedPostOnly    int64 `json:"accepted_post_only"`
	AcceptedRegular     int64 `json:"accepted_regular"`
	PostOnlyFills       int64 `json:"post_only_fills"`
	PostOnlyFilledQty   int64 `json:"post_only_filled_qty"`
	RejectedWouldTake   int64 `json:"rejected_would_take"`
	RejectedInvalid     int64 `json:"rejected_invalid"`
	UnmatchedFillOrders int64 `json:"unmatched_fill_orders"`
}

type postOnlyPayload struct {
	OrderID   uint64 `json:"order_id"`
	FilledQty int64  `json:"filled_qty"`
	PostOnly  bool   `json:"post_only"`
	Error     string `json:"error"`
}

type postOnlyOrderKey struct {
	venueID string
	orderID uint64
}

// MeasurePostOnlyActivity independently reads accepted orders, fill records,
// and rejection reasons. It never infers a passive outcome from a maker's
// configuration: the persisted accepted-order bit is the evidence.
func (r *Run) MeasurePostOnlyActivity(options PostOnlyActivityOptions) (PostOnlyActivity, error) {
	roles := selectionSet(options.Roles)
	symbols := selectionSet(options.Symbols)
	orders := make(map[postOnlyOrderKey]bool)
	var result PostOnlyActivity
	err := r.Scan(ScanOptions{
		Events:  []string{"OrderAccepted", "OrderFill", "OrderRejected"},
		Workers: 1,
	}, func(event Event) {
		role := r.Role(event.VenueID, event.ClientID)
		if !selected(role, roles) || !selected(event.Symbol, symbols) {
			return
		}
		var payload postOnlyPayload
		if event.Decode(&payload) != nil {
			return
		}
		result.Events++
		switch event.Name {
		case "OrderAccepted":
			result.Accepted++
			orders[postOnlyOrderKey{event.VenueID, payload.OrderID}] = payload.PostOnly
			if payload.PostOnly {
				result.AcceptedPostOnly++
			} else {
				result.AcceptedRegular++
			}
		case "OrderFill":
			postOnly, found := orders[postOnlyOrderKey{event.VenueID, payload.OrderID}]
			if !found {
				result.UnmatchedFillOrders++
				return
			}
			if postOnly {
				result.PostOnlyFills++
				result.PostOnlyFilledQty += payload.FilledQty
			}
		case "OrderRejected":
			switch payload.Error {
			case "POST_ONLY_WOULD_TAKE":
				result.RejectedWouldTake++
			case "POST_ONLY_INVALID":
				result.RejectedInvalid++
			}
		}
	})
	return result, err
}

func selectionSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result[value] = struct{}{}
		}
	}
	return result
}

func selected(value string, selection map[string]struct{}) bool {
	if len(selection) == 0 {
		return true
	}
	_, ok := selection[value]
	return ok
}
