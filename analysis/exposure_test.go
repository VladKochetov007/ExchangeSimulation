package analysis

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"exchange_sim/price"
)

func writeRiskTimelineRun(t *testing.T, report Report, timeline map[string][]riskRow) string {
	t.Helper()
	dir := t.TempDir()
	envelope := struct {
		Report
		RiskTimeline map[string][]riskRow `json:"risk_timeline"`
	}{Report: report, RiskTimeline: timeline}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal risk timeline: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "greeks.json"), raw, 0o644); err != nil {
		t.Fatalf("write risk timeline: %v", err)
	}
	return dir
}

func TestMeasureExposureReconstructsSecondOrderGreeks(t *testing.T) {
	const (
		forward = int64(50_000 * multivenueContractPrecision)
		strike  = int64(55_000 * multivenueContractPrecision)
		vol     = 0.6
	)
	horizon := int64(365 * 24 * time.Hour)
	positions := []riskGreekPosition{{
		Position:          2 * multivenueContractPrecision,
		TimeToExpiryNano:  horizon,
		ModelForward:      forward,
		Strike:            strike,
		ImpliedVolatility: vol,
	}}
	empty := make([]riskGreekPosition, 0)

	tests := []struct {
		name        string
		positions   *[]riskGreekPosition
		wantErr     bool
		wantVanna   float64
		wantVolga   float64
		wantSamples int
	}{
		{
			name:        "reconstructs from exchange-owned positions",
			positions:   &positions,
			wantVanna:   2 * price.Black76Vanna(forward, strike, vol, 1),
			wantVolga:   2 * price.Black76Volga(forward, strike, vol, 1),
			wantSamples: 1,
		},
		{
			name:      "refuses missing position evidence",
			positions: nil,
			wantErr:   true,
		},
		{
			name:        "accepts an explicit empty position snapshot",
			positions:   &empty,
			wantSamples: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			row := riskRow{VenueID: "north", ClientID: 7, GreekPositions: test.positions}
			row.Profile.Timestamp = int64(time.Hour)
			dir := writeRiskTimelineRun(t, Report{TerminalAccounts: []AccountRow{{
				VenueID: "north", ClientID: 7, Role: "option_dealer_1",
			}}}, map[string][]riskRow{"north": {row}})
			run, err := Open(dir)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			got, err := run.MeasureExposure(ExposureOptions{})
			if test.wantErr {
				if err == nil {
					t.Fatal("MeasureExposure succeeded without Greek position evidence")
				}
				return
			}
			if err != nil {
				t.Fatalf("MeasureExposure: %v", err)
			}
			if got.RiskSamples != test.wantSamples || got.SecondOrderSamples != test.wantSamples {
				t.Fatalf("samples = risk %d second-order %d, want %d", got.RiskSamples, got.SecondOrderSamples, test.wantSamples)
			}
			if math.Abs(got.PooledMeanAbsVanna-math.Abs(test.wantVanna)) > 1e-12 {
				t.Errorf("pooled |vanna| = %v, want %v", got.PooledMeanAbsVanna, math.Abs(test.wantVanna))
			}
			if math.Abs(got.PooledMeanAbsVolga-math.Abs(test.wantVolga)) > 1e-12 {
				t.Errorf("pooled |volga| = %v, want %v", got.PooledMeanAbsVolga, math.Abs(test.wantVolga))
			}
		})
	}
}
