package analysis

import "testing"

func TestBasisRetainsOutOfDomainSignedEvidenceAsExplicitUndefined(t *testing.T) {
	lines := []string{
		`{"sim_ts":1,"client_id":0,"event":"mark_price_update","data":{"venue_id":"north","payload":{"timestamp":1,"symbol":"ABC-PERP","mark_price":110,"index_price":100}}}`,
		`{"sim_ts":2,"client_id":0,"event":"mark_price_update","data":{"venue_id":"north","payload":{"timestamp":2,"symbol":"ABC-PERP","mark_price":0,"index_price":100}}}`,
		`{"sim_ts":3,"client_id":0,"event":"mark_price_update","data":{"venue_id":"north","payload":{"timestamp":3,"symbol":"ABC-PERP","mark_price":-10,"index_price":100}}}`,
		`{"sim_ts":4,"client_id":0,"event":"mark_price_update","data":{"venue_id":"north","payload":{"timestamp":4,"symbol":"ABC-PERP","mark_price":100,"index_price":0}}}`,
	}
	run, err := Open(writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": lines}))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasureBasis(BasisOptions{})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if len(result.Perp) != 1 {
		t.Fatalf("perp series = %#v", result.Perp)
	}
	stats := result.Perp[0]
	if stats.Observations != 1 || stats.UndefinedDomain != 3 || result.UndefinedDomainObservations != 3 {
		t.Fatalf("basis domain accounting = %+v total=%d", stats, result.UndefinedDomainObservations)
	}
	if stats.MeanBps != 1000 || result.PerpPooled.UndefinedDomain != 3 {
		t.Fatalf("valid basis or pooled undefined count changed: %+v pooled=%+v", stats, result.PerpPooled)
	}
}
