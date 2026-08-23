package analysis

import "testing"

func TestMeasurePostOnlyActivityUsesPersistedAcceptanceAndRejectionEvidence(t *testing.T) {
	dir := writeRun(t, Report{TerminalAccounts: []AccountRow{
		{VenueID: "north", ClientID: 1, Role: "spot_maker_1"},
		{VenueID: "north", ClientID: 2, Role: "noise_flow_1"},
	}}, map[string][]string{
		"north/spot/ABC-USD.jsonl": {
			`{"sim_ts":1,"client_id":1,"event":"OrderAccepted","data":{"venue_id":"north","symbol":"ABC/USD","payload":{"order_id":11,"post_only":true}}}`,
			`{"sim_ts":2,"client_id":1,"event":"OrderAccepted","data":{"venue_id":"north","symbol":"ABC/USD","payload":{"order_id":12,"post_only":false}}}`,
			`{"sim_ts":3,"client_id":1,"event":"OrderFill","data":{"venue_id":"north","symbol":"ABC/USD","payload":{"order_id":11,"filled_qty":7}}}`,
			`{"sim_ts":4,"client_id":1,"event":"OrderRejected","data":{"venue_id":"north","symbol":"ABC/USD","payload":{"error":"POST_ONLY_WOULD_TAKE"}}}`,
			`{"sim_ts":5,"client_id":1,"event":"OrderRejected","data":{"venue_id":"north","symbol":"ABC/USD","payload":{"error":"POST_ONLY_INVALID"}}}`,
			`{"sim_ts":6,"client_id":2,"event":"OrderAccepted","data":{"venue_id":"north","symbol":"ABC/USD","payload":{"order_id":13,"post_only":false}}}`,
		},
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasurePostOnlyActivity(PostOnlyActivityOptions{Roles: []string{"spot_maker"}, Symbols: []string{"ABC/USD"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Accepted != 2 || result.AcceptedPostOnly != 1 || result.AcceptedRegular != 1 || result.PostOnlyFills != 1 || result.PostOnlyFilledQty != 7 || result.RejectedWouldTake != 1 || result.RejectedInvalid != 1 || result.UnmatchedFillOrders != 0 {
		t.Fatalf("post-only activity = %+v", result)
	}
}
