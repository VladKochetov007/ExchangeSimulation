package analysis

import "testing"

func TestCrossVenueRouterCountersRequireOneInstrumentedReport(t *testing.T) {
	row := CrossVenueRouterEvidenceCounters{RouterID: 7, EvaluationEvidenceEnabled: true}
	run := &Run{Report: Report{RouterReports: []CrossVenueRouterEvidenceCounters{row}}}
	if got, err := run.CrossVenueRouterCounters(7); err != nil || got != row {
		t.Fatalf("zero-count instrumented report = %#v, %v", got, err)
	}
	if _, err := run.CrossVenueRouterCounters(8); err == nil {
		t.Fatal("missing router report accepted")
	}
	run.Report.RouterReports = append(run.Report.RouterReports, row)
	if _, err := run.CrossVenueRouterCounters(7); err == nil {
		t.Fatal("duplicate router report accepted")
	}
	run.Report.RouterReports = []CrossVenueRouterEvidenceCounters{{RouterID: 7}}
	if _, err := run.CrossVenueRouterCounters(7); err == nil {
		t.Fatal("unobserved router report accepted")
	}
}
