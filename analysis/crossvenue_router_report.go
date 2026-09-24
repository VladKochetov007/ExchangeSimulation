package analysis

import "fmt"

// CrossVenueRouterEvidenceCounters is the completeness anchor for optional
// actor-observation streams in a successfully written terminal run report.
type CrossVenueRouterEvidenceCounters struct {
	RouterID                  uint64 `json:"router_id"`
	EvaluationEvidenceEnabled bool   `json:"evaluation_evidence_enabled"`
	QuoteEvaluations          uint64 `json:"quote_evaluations"`
	ResponseReceipts          uint64 `json:"response_receipts"`
}

func (r *Run) CrossVenueRouterCounters(routerID uint64) (CrossVenueRouterEvidenceCounters, error) {
	if r == nil || routerID == 0 {
		return CrossVenueRouterEvidenceCounters{}, fmt.Errorf("cross-venue router counters: invalid run or router ID")
	}
	var selected *CrossVenueRouterEvidenceCounters
	for index := range r.Report.RouterReports {
		if r.Report.RouterReports[index].RouterID != routerID {
			continue
		}
		if selected != nil {
			return CrossVenueRouterEvidenceCounters{}, fmt.Errorf("cross-venue router counters: duplicate router %d", routerID)
		}
		selected = &r.Report.RouterReports[index]
	}
	if selected == nil || !selected.EvaluationEvidenceEnabled {
		return CrossVenueRouterEvidenceCounters{}, fmt.Errorf("cross-venue router counters: missing instrumented router %d", routerID)
	}
	return *selected, nil
}
