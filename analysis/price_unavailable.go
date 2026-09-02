package analysis

import "fmt"

// PriceUnavailableRejectionAudit is the typed audit for client-facing order
// rejections caused by an unavailable executable price. It deliberately reads
// OrderRejected evidence rather than the separate price_unavailable diagnostic
// event: the two events answer different questions and must not be conflated.
type PriceUnavailableRejectionAudit struct {
	OrderRejectedCount              int64 `json:"order_rejected_count"`
	PriceUnavailableOrderRejections int64 `json:"price_unavailable_order_rejections"`
	MalformedOrderRejectedCount     int64 `json:"malformed_order_rejected_count"`
	Valid                           bool  `json:"valid"`
}

// MeasurePriceUnavailableOrderRejections counts exact PRICE_UNAVAILABLE order
// outcomes across the rendered evidence routes. Every OrderRejected payload
// must carry its error field; silently treating a missing field as a different
// rejection would make the zero-count activation predicate non-auditable.
func (r *Run) MeasurePriceUnavailableOrderRejections() (*PriceUnavailableRejectionAudit, error) {
	if r == nil {
		return nil, fmt.Errorf("price unavailable: nil run")
	}
	result := &PriceUnavailableRejectionAudit{Valid: true}
	err := r.Scan(ScanOptions{Events: []string{"OrderRejected"}, Workers: 1}, func(event Event) {
		result.OrderRejectedCount++
		var rejection struct {
			Error string `json:"error"`
		}
		if err := decodeRequiredJSON(event.Raw(), &rejection, "error"); err != nil || rejection.Error == "" {
			result.MalformedOrderRejectedCount++
			result.Valid = false
			return
		}
		if rejection.Error == "PRICE_UNAVAILABLE" {
			result.PriceUnavailableOrderRejections++
		}
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
