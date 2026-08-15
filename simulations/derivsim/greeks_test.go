package derivsim

import (
	"math"
	"testing"
)

func TestBuildGreekReportSummarizesAndCopiesProfiles(t *testing.T) {
	profiles := []GreekProfile{
		{Timestamp: 10, ModelForward: 100, ForwardSource: "spot_mid_proxy", ImpliedVolatility: 0.8, NetDelta: -2, Gamma: -3, Vega: 4, Contracts: 1},
		{Timestamp: 20, ModelForward: 100, ForwardSource: "spot_mid_proxy", ImpliedVolatility: 0.8, NetDelta: 4, Gamma: 1, Vega: -8},
	}
	report, err := BuildGreekReport(profiles)
	if err != nil {
		t.Fatalf("BuildGreekReport: %v", err)
	}
	if !report.Summary.HasSamples || report.Summary.Samples != 2 || report.Summary.FirstTimestamp != 10 || report.Summary.LastTimestamp != 20 {
		t.Fatalf("unexpected summary header: %+v", report.Summary)
	}
	if !report.Summary.HasExposure || report.Summary.LastExposure.Timestamp != 10 {
		t.Fatalf("last exposure not retained: %+v", report.Summary)
	}
	if report.Summary.MaxAbsNetDelta != 4 || report.Summary.MaxAbsGamma != 3 || report.Summary.MaxAbsVega != 8 {
		t.Fatalf("unexpected maxima: %+v", report.Summary)
	}
	if math.Abs(report.Summary.MeanAbsNetDelta-3) > 1e-12 || math.Abs(report.Summary.MeanAbsGamma-2) > 1e-12 || math.Abs(report.Summary.MeanAbsVega-6) > 1e-12 {
		t.Fatalf("unexpected means: %+v", report.Summary)
	}
	profiles[0].NetDelta = 999
	if report.Profiles[0].NetDelta == 999 {
		t.Fatal("report aliases caller profile slice")
	}
}

func TestBuildGreekReportRejectsNonFiniteValues(t *testing.T) {
	_, err := BuildGreekReport([]GreekProfile{{ImpliedVolatility: math.NaN()}})
	if err == nil {
		t.Fatal("expected non-finite profile rejection")
	}
}

func TestBuildGreekReportWithPositionsCopiesAndValidatesRows(t *testing.T) {
	positions := []GreekPosition{{Symbol: "ABC-C", ImpliedVolatility: 0.8, Delta: 1, Gamma: 2, Vega: 3}}
	report, err := BuildGreekReportWithPositions(nil, positions)
	if err != nil {
		t.Fatalf("BuildGreekReportWithPositions: %v", err)
	}
	positions[0].Delta = 99
	if report.PositionProfiles[0].Delta == 99 {
		t.Fatal("report aliases caller position rows")
	}
	_, err = BuildGreekReportWithPositions(nil, []GreekPosition{{ImpliedVolatility: 0.8, Delta: math.Inf(1)}})
	if err == nil {
		t.Fatal("expected non-finite position rejection")
	}
}
