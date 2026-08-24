package analysis

import "testing"

func TestPerpSignalAuditRetainsPresentZeroAndVariation(t *testing.T) {
	dir := writeRun(t, Report{}, map[string][]string{
		"north/derivatives.jsonl": {
			`{"sim_ts":1,"data":{"venue_id":"north","payload":{"symbol":"ABC-PERP","payload":{"mark_price":0,"index_price":0}}},"event":"mark_price_update"}`,
			`{"sim_ts":2,"data":{"venue_id":"north","payload":{"symbol":"ABC-PERP","payload":{"mark_price":5,"index_price":0}}},"event":"mark_price_update"}`,
			`{"sim_ts":3,"data":{"venue_id":"north","payload":{"symbol":"ABC-PERP","payload":{"rate":0}}},"event":"funding_rate_update"}`,
			`{"sim_ts":4,"data":{"venue_id":"north","payload":{"symbol":"ABC-PERP","payload":{"rate":2}}},"event":"funding_rate_update"}`,
		},
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasurePerpSignals(PerpSignalOptions{Symbol: "ABC-PERP", RequiredVenues: []string{"north"}})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if !result.Valid || !result.Ready || len(result.Venues) != 1 {
		t.Fatalf("audit = %+v", result)
	}
	venue := result.Venues[0]
	if venue.FirstMark == nil || *venue.FirstMark != 0 || venue.FirstFundingRate == nil || *venue.FirstFundingRate != 0 {
		t.Fatalf("present zeros became unavailable: %+v", venue)
	}
	if venue.DistinctMarkIndexPairs != 2 || venue.DistinctFundingRates != 2 || venue.FundingSettlementEvents != 0 {
		t.Fatalf("variation/settlement accounting = %+v", venue)
	}
}

func TestPerpSignalAuditRejectsMissingRequiredScalarAndTracksMissingVenue(t *testing.T) {
	dir := writeRun(t, Report{}, map[string][]string{
		"north/derivatives.jsonl": {
			`{"sim_ts":1,"data":{"venue_id":"north","payload":{"symbol":"ABC-PERP","payload":{"index_price":10}}},"event":"mark_price_update"}`,
			`{"sim_ts":2,"data":{"venue_id":"north","payload":{"symbol":"ABC-PERP","payload":{}}},"event":"funding_rate_update"}`,
		},
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasurePerpSignals(PerpSignalOptions{Symbol: "ABC-PERP", RequiredVenues: []string{"north", "south"}})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if result.Valid || result.Ready || result.InvalidMarkRecords != 1 || result.InvalidFundingRecords != 1 {
		t.Fatalf("missing scalar passed: %+v", result)
	}
	if len(result.MissingRequiredVenues) != 2 || result.MissingRequiredVenues[0] != "north" || result.MissingRequiredVenues[1] != "south" {
		t.Fatalf("missing venues = %+v", result.MissingRequiredVenues)
	}
}

func TestPerpSignalAuditCountsFundingSettlementWithoutCallingItSignalFailure(t *testing.T) {
	dir := writeRun(t, Report{}, map[string][]string{
		"north/derivatives.jsonl": {
			`{"sim_ts":1,"data":{"venue_id":"north","payload":{"symbol":"ABC-PERP","payload":{"mark_price":10,"index_price":10}}},"event":"mark_price_update"}`,
			`{"sim_ts":2,"data":{"venue_id":"north","payload":{"symbol":"ABC-PERP","payload":{"mark_price":11,"index_price":10}}},"event":"mark_price_update"}`,
			`{"sim_ts":3,"data":{"venue_id":"north","payload":{"symbol":"ABC-PERP","payload":{"rate":1}}},"event":"funding_rate_update"}`,
			`{"sim_ts":4,"data":{"venue_id":"north","payload":{"symbol":"ABC-PERP","payload":{"rate":2}}},"event":"funding_rate_update"}`,
			`{"sim_ts":5,"data":{"venue_id":"north","payload":{"symbol":"ABC-PERP","payload":{"symbol":"ABC-PERP","reason":"funding_settlement","changes":[]}}},"event":"balance_change"}`,
			`{"sim_ts":6,"data":{"venue_id":"north","payload":{"symbol":"ABC-PERP","payload":{"symbol":"ABC-PERP","reason":"trade_settlement","changes":[]}}},"event":"balance_change"}`,
		},
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := run.MeasurePerpSignals(PerpSignalOptions{Symbol: "ABC-PERP", RequiredVenues: []string{"north"}})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if !result.Ready || result.Venues[0].FundingSettlementEvents != 1 {
		t.Fatalf("settlement count changed readiness: %+v", result)
	}
}

func TestPerpSignalReadinessCatchesDroppedUniquePublicObservation(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
	}{
		{
			name: "dropped mark variation",
			lines: []string{
				`{"sim_ts":1,"data":{"venue_id":"north","payload":{"symbol":"ABC-PERP","payload":{"mark_price":10,"index_price":10}}},"event":"mark_price_update"}`,
				`{"sim_ts":2,"data":{"venue_id":"north","payload":{"symbol":"ABC-PERP","payload":{"rate":1}}},"event":"funding_rate_update"}`,
				`{"sim_ts":3,"data":{"venue_id":"north","payload":{"symbol":"ABC-PERP","payload":{"rate":2}}},"event":"funding_rate_update"}`,
			},
		},
		{
			name: "dropped funding variation",
			lines: []string{
				`{"sim_ts":1,"data":{"venue_id":"north","payload":{"symbol":"ABC-PERP","payload":{"mark_price":10,"index_price":10}}},"event":"mark_price_update"}`,
				`{"sim_ts":2,"data":{"venue_id":"north","payload":{"symbol":"ABC-PERP","payload":{"mark_price":11,"index_price":10}}},"event":"mark_price_update"}`,
				`{"sim_ts":3,"data":{"venue_id":"north","payload":{"symbol":"ABC-PERP","payload":{"rate":1}}},"event":"funding_rate_update"}`,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := writeRun(t, Report{}, map[string][]string{"north/derivatives.jsonl": test.lines})
			run, err := Open(dir)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			result, err := run.MeasurePerpSignals(PerpSignalOptions{Symbol: "ABC-PERP", RequiredVenues: []string{"north"}})
			if err != nil {
				t.Fatalf("measure: %v", err)
			}
			if !result.Valid || result.Ready {
				t.Fatalf("dropped unique public observation passed readiness: %+v", result)
			}
		})
	}
}
